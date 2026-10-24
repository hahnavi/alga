package api

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/google/uuid"
	xmlroundtripvalidator "github.com/mattermost/xml-roundtrip-validator"
	dsig "github.com/russellhaering/goxmldsig"

	"alga/logger"
	"alga/store"
)

const (
	samlMaxResponseBytes     = 1 << 20
	samlMaxMetadataBytes     = 512 << 10
	samlMaxAssertionLifetime = 15 * time.Minute
	samlMaxClockSkew         = 3 * time.Minute
)

type samlHandler struct {
	server        *Server
	providerStore store.SAMLProviderStore
	identityStore store.SAMLIdentityStore
	flowStore     samlFlowStore
}

func newSAMLHandler(server *Server, providerStore store.SAMLProviderStore, identityStore store.SAMLIdentityStore, flowStore samlFlowStore) *samlHandler {
	return &samlHandler{
		server:        server,
		providerStore: providerStore,
		identityStore: identityStore,
		flowStore:     flowStore,
	}
}

type samlPublicProvider struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func (h *samlHandler) listPublicProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.providerStore.ListEnabledProviders(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to list saml providers")
		return
	}
	items := make([]samlPublicProvider, 0, len(providers))
	for _, provider := range providers {
		if _, err := h.serviceProvider(&provider); err != nil {
			logger.WarnCtx(r.Context(), "enabled saml provider is unavailable", "component", "api", "provider_id", provider.ID, "reason", "invalid_provider_configuration")
			continue
		}
		items = append(items, samlPublicProvider{ID: provider.ID, Name: provider.Name})
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *samlHandler) metadata(w http.ResponseWriter, r *http.Request, providerID uuid.UUID) {
	provider, err := h.providerStore.GetProvider(r.Context(), providerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, ErrorCodeNotFound, "saml provider not found")
			return
		}
		writeInternalError(w, err, "failed to load saml provider")
		return
	}
	sp, err := h.serviceProviderEndpoints(provider)
	if err != nil {
		logger.WarnCtx(r.Context(), "saml metadata unavailable", "component", "api", "provider_id", provider.ID, "reason", "invalid_public_base_url")
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "saml provider endpoints are not configured correctly")
		return
	}
	metadata := sp.Metadata()
	for i := range metadata.SPSSODescriptors {
		acs := metadata.SPSSODescriptors[i].AssertionConsumerServices[:0]
		for _, endpoint := range metadata.SPSSODescriptors[i].AssertionConsumerServices {
			if endpoint.Binding == saml.HTTPPostBinding {
				acs = append(acs, endpoint)
			}
		}
		metadata.SPSSODescriptors[i].AssertionConsumerServices = acs
	}
	data, err := xml.MarshalIndent(metadata, "", "  ")
	if err != nil {
		writeInternalError(w, err, "failed to generate saml metadata")
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(append([]byte(xml.Header), data...)); err != nil {
		logger.WarnCtx(r.Context(), "failed to write saml metadata response", "component", "api", "provider_id", provider.ID, "error", err)
	}
}

func (h *samlHandler) authorize(w http.ResponseWriter, r *http.Request, providerID uuid.UUID) {
	if h.flowStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "saml sign-in state storage is unavailable")
		return
	}
	provider, err := h.providerStore.GetProvider(r.Context(), providerID)
	if err != nil || provider == nil || !provider.Enabled {
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			writeInternalError(w, err, "failed to load saml provider")
			return
		}
		h.fail(w, r, providerID, "provider_unavailable")
		return
	}
	sp, err := h.serviceProvider(provider)
	if err != nil {
		logger.WarnCtx(r.Context(), "saml sign-in could not start", "component", "api", "provider_id", provider.ID, "reason", "invalid_provider_configuration")
		h.fail(w, r, providerID, "provider_configuration_invalid")
		return
	}
	requestBinding := saml.HTTPPostBinding
	endpoint := sp.GetSSOBindingLocation(requestBinding)
	if endpoint == "" {
		requestBinding = saml.HTTPRedirectBinding
		endpoint = sp.GetSSOBindingLocation(requestBinding)
	}
	if endpoint == "" {
		h.fail(w, r, providerID, "provider_configuration_invalid")
		return
	}
	request, err := sp.MakeAuthenticationRequest(endpoint, requestBinding, saml.HTTPPostBinding)
	if err != nil {
		logger.WarnCtx(r.Context(), "saml authentication request creation failed", "component", "api", "provider_id", provider.ID, "error", err)
		h.fail(w, r, providerID, "request_creation_failed")
		return
	}
	relayState, err := newSAMLRelayState()
	if err != nil {
		writeInternalError(w, err, "failed to start saml sign-in")
		return
	}
	nextPath := safeSAMLNextPath(r.URL.Query().Get("next"))
	payload := samlLoginPayload{ProviderID: provider.ID.String(), RequestID: request.ID, NextPath: nextPath}
	if err := h.flowStore.Set(r.Context(), relayState, payload); err != nil {
		logger.ErrorCtx(r.Context(), "failed to persist saml login state", "component", "api", "provider_id", provider.ID, "error", err)
		writeInternalError(w, err, "failed to start saml sign-in")
		return
	}
	if requestBinding == saml.HTTPPostBinding {
		page, nonce, err := renderSAMLPostPage(endpoint, request.Element(), relayState)
		if err != nil {
			logger.WarnCtx(r.Context(), "saml authentication post form creation failed", "component", "api", "provider_id", provider.ID, "error", err)
			h.fail(w, r, providerID, "post_form_creation_failed")
			return
		}
		endpointURL, err := url.Parse(endpoint)
		if err != nil || endpointURL.Scheme != "https" || !isValidSAMLHostname(endpointURL.Hostname()) || !isValidSAMLPort(endpointURL.Port()) {
			logger.WarnCtx(r.Context(), "saml post destination is invalid", "component", "api", "provider_id", provider.ID, "reason", "invalid_sso_endpoint")
			h.fail(w, r, providerID, "provider_configuration_invalid")
			return
		}
		formOriginURL := url.URL{Scheme: endpointURL.Scheme, Host: endpointURL.Host}
		formOrigin := formOriginURL.String()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action "+formOrigin+"; script-src 'nonce-"+nonce+"'; base-uri 'none'; frame-ancestors 'none'")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(page); err != nil {
			logger.WarnCtx(r.Context(), "failed to write saml authentication post form", "component", "api", "provider_id", provider.ID, "error", err)
		}
		return
	}
	authURL, err := request.Redirect(relayState, sp)
	if err != nil {
		logger.WarnCtx(r.Context(), "saml authentication redirect creation failed", "component", "api", "provider_id", provider.ID, "error", err)
		h.fail(w, r, providerID, "redirect_creation_failed")
		return
	}
	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

func renderSAMLPostPage(destination string, request *etree.Element, relayState string) ([]byte, string, error) {
	if request == nil {
		return nil, "", errors.New("saml authentication request is empty")
	}
	requestDocument := etree.NewDocument()
	requestDocument.SetRoot(request)
	requestXML, err := requestDocument.WriteToBytes()
	if err != nil {
		return nil, "", fmt.Errorf("serialize saml authentication request: %w", err)
	}
	pageNonce := make([]byte, 32)
	if _, err := rand.Read(pageNonce); err != nil {
		return nil, "", fmt.Errorf("generate saml form nonce: %w", err)
	}
	nonce := base64.StdEncoding.EncodeToString(pageNonce)
	pageTemplate, err := template.New("saml-post-page").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Signing in</title></head>
<body>
<form id="saml-post" method="post" action="{{.Destination}}">
<input type="hidden" name="SAMLRequest" value="{{.SAMLRequest}}">
<input type="hidden" name="RelayState" value="{{.RelayState}}">
<noscript><button type="submit">Continue to sign in</button></noscript>
</form>
<script nonce="{{.Nonce}}">document.getElementById("saml-post").submit();</script>
</body></html>`)
	if err != nil {
		return nil, "", fmt.Errorf("parse saml post page template: %w", err)
	}
	var page bytes.Buffer
	data := struct {
		Destination string
		SAMLRequest string
		RelayState  string
		Nonce       string
	}{
		Destination: destination,
		SAMLRequest: base64.StdEncoding.EncodeToString(requestXML),
		RelayState:  relayState,
		Nonce:       nonce,
	}
	if err := pageTemplate.Execute(&page, data); err != nil {
		return nil, "", fmt.Errorf("render saml post page: %w", err)
	}
	return page.Bytes(), nonce, nil
}

func (h *samlHandler) assertionConsumer(w http.ResponseWriter, r *http.Request, providerID uuid.UUID) {
	if h.flowStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "saml sign-in state storage is unavailable")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, ErrorCodeMethodNotAllowed, "method not allowed")
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/x-www-form-urlencoded" {
		h.fail(w, r, providerID, "invalid_callback_content_type")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, samlMaxResponseBytes)
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, providerID, "invalid_callback_form")
		return
	}
	if len(r.PostForm["SAMLResponse"]) != 1 || len(r.PostForm["RelayState"]) != 1 ||
		r.URL.Query().Has("SAMLResponse") || r.URL.Query().Has("SAMLart") || len(r.PostForm["SAMLart"]) != 0 {
		h.fail(w, r, providerID, "invalid_callback_payload")
		return
	}
	relayState := r.PostForm.Get("RelayState")
	if !validSAMLRelayState(relayState) {
		h.fail(w, r, providerID, "invalid_relay_state")
		return
	}
	payload, ok, err := h.flowStore.Consume(r.Context(), relayState)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to consume saml login state", "component", "api", "provider_id", providerID, "error", err)
		h.fail(w, r, providerID, "state_storage_failed")
		return
	}
	if !ok || payload.ProviderID != providerID.String() || payload.RequestID == "" {
		h.fail(w, r, providerID, "invalid_or_expired_state")
		return
	}
	provider, err := h.providerStore.GetProvider(r.Context(), providerID)
	if err != nil || provider == nil || !provider.Enabled {
		h.fail(w, r, providerID, "provider_unavailable")
		return
	}
	if err := requireSinglePlaintextAssertion(r.PostForm.Get("SAMLResponse")); err != nil {
		h.fail(w, r, providerID, "invalid_assertion_count")
		return
	}
	sp, err := h.serviceProvider(provider)
	if err != nil {
		h.fail(w, r, providerID, "provider_configuration_invalid")
		return
	}
	assertion, err := sp.ParseResponse(r, []string{payload.RequestID})
	if err != nil {
		h.fail(w, r, providerID, "assertion_validation_failed")
		return
	}
	if assertion == nil || assertion.ID == "" || assertion.Subject == nil || assertion.Subject.NameID == nil ||
		assertion.Subject.NameID.Value == "" || len(assertion.Subject.NameID.Value) > 512 || assertion.Subject.NameID.Format == string(saml.TransientNameIDFormat) ||
		assertion.Conditions == nil || len(assertion.Subject.SubjectConfirmations) == 0 {
		h.fail(w, r, providerID, "required_assertion_claim_missing")
		return
	}
	for _, confirmation := range assertion.Subject.SubjectConfirmations {
		if confirmation.Method != "urn:oasis:names:tc:SAML:2.0:cm:bearer" || confirmation.SubjectConfirmationData == nil {
			h.fail(w, r, providerID, "unsupported_subject_confirmation")
			return
		}
	}
	if err := validateSAMLAssertionLifetime(assertion, time.Now()); err != nil {
		h.fail(w, r, providerID, "assertion_lifetime_invalid")
		return
	}
	email, err := samlAttribute(assertion, provider.EmailAttribute)
	if err != nil {
		h.fail(w, r, providerID, "email_attribute_missing")
		return
	}
	email = strings.TrimSpace(email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || address.Name != "" || len(email) > 320 {
		h.fail(w, r, providerID, "email_attribute_invalid")
		return
	}
	used, err := h.flowStore.MarkAssertionUsed(r.Context(), provider.IDPEntityID, assertion.ID)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to record saml assertion replay key", "component", "api", "provider_id", provider.ID, "error", err)
		h.fail(w, r, providerID, "replay_storage_failed")
		return
	}
	if !used {
		h.fail(w, r, providerID, "assertion_replayed")
		return
	}
	user, linked := h.resolveUser(r, provider, assertion.Subject.NameID.Value, email)
	if user == nil {
		reason := "no_matching_account"
		if !linked {
			reason = "identity_link_failed"
		}
		h.fail(w, r, providerID, reason)
		return
	}
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		h.fail(w, r, providerID, "account_locked")
		return
	}
	if err := h.server.userStore.RecordSuccessfulLogin(user.ID, r.RemoteAddr); err != nil {
		logger.WarnCtx(r.Context(), "failed to record saml login time", "component", "api", "user_id", user.ID, "error", err)
	}
	session, err := h.server.sessionStore.CreateSession(user.ID, r.RemoteAddr, r.UserAgent())
	if err != nil {
		logger.ErrorCtx(r.Context(), "saml session creation failed", "component", "api", "user_id", user.ID, "error", err)
		h.fail(w, r, providerID, "session_creation_failed")
		return
	}
	csrfToken, err := generateCSRFToken()
	if err != nil {
		logger.ErrorCtx(r.Context(), "saml csrf token generation failed", "component", "api", "user_id", user.ID, "error", err)
		h.fail(w, r, providerID, "session_creation_failed")
		return
	}
	h.server.setSessionCookie(w, session.ID)
	setRefreshTokenCookie(w, h.server.cfg.SecureCookies, h.server.sessionExpiry, session.RefreshToken)
	h.server.setCSRFCookie(w, csrfToken)
	h.server.auditStore.Log(store.AuditSAMLLoginSuccess, &user.ID, user.Email, r.RemoteAddr, r.UserAgent(), true, map[string]any{
		"provider_id": provider.ID,
		"provider":    provider.Name,
		"method":      "saml",
	})
	logger.InfoCtx(r.Context(), "saml login success", "component", "api", "user_id", user.ID, "provider_id", provider.ID)
	redirect := url.URL{Path: "/login"}
	query := redirect.Query()
	query.Set("saml", "success")
	query.Set("redirect", safeSAMLNextPath(payload.NextPath))
	redirect.RawQuery = query.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (h *samlHandler) resolveUser(r *http.Request, provider *store.SAMLProviderRecord, subject, email string) (*store.UserRecord, bool) {
	identity, err := h.identityStore.GetByProviderSubject(r.Context(), provider.ID, subject)
	if err == nil && identity != nil {
		user, userErr := h.server.userStore.GetByID(identity.UserID)
		if userErr == nil && user != nil {
			return user, true
		}
		if userErr != nil && !errors.Is(userErr, store.ErrNotFound) {
			logger.WarnCtx(r.Context(), "saml linked user lookup failed", "component", "api", "provider_id", provider.ID, "error", userErr)
			return nil, false
		}
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		logger.WarnCtx(r.Context(), "saml identity lookup failed", "component", "api", "provider_id", provider.ID, "error", err)
		return nil, false
	}
	user, err := h.server.userStore.GetByEmail(email)
	if err != nil || user == nil {
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			logger.WarnCtx(r.Context(), "saml email lookup failed", "component", "api", "provider_id", provider.ID, "error", err)
			return nil, false
		}
		return nil, true
	}
	_, err = h.identityStore.CreateLink(r.Context(), &store.SAMLIdentityRecord{
		UserID:     user.ID,
		ProviderID: provider.ID,
		Subject:    subject,
		Email:      email,
	})
	if err == nil {
		h.server.auditStore.Log(store.AuditSAMLIdentityLinked, &user.ID, user.Email, r.RemoteAddr, r.UserAgent(), true, map[string]any{
			"provider_id": provider.ID,
			"provider":    provider.Name,
		})
		return user, true
	}
	// A simultaneous first login can race on (provider_id, subject). Only accept
	// the link if the winning request linked this same subject to this same user.
	linked, lookupErr := h.identityStore.GetByProviderSubject(r.Context(), provider.ID, subject)
	if lookupErr == nil && linked != nil && linked.UserID == user.ID {
		return user, true
	}
	logger.WarnCtx(r.Context(), "saml identity link creation failed", "component", "api", "provider_id", provider.ID, "user_id", user.ID, "error", err)
	return nil, false
}

func (h *samlHandler) serviceProvider(provider *store.SAMLProviderRecord) (*saml.ServiceProvider, error) {
	sp, err := h.serviceProviderEndpoints(provider)
	if err != nil {
		return nil, err
	}
	metadata, err := parseSAMLMetadata(provider.MetadataXML)
	if err != nil {
		return nil, err
	}
	if metadata.EntityID != provider.IDPEntityID {
		return nil, errors.New("saml metadata issuer does not match provider configuration")
	}
	sp.IDPMetadata = metadata
	return sp, nil
}

func (h *samlHandler) serviceProviderEndpoints(provider *store.SAMLProviderRecord) (*saml.ServiceProvider, error) {
	baseURL, err := h.publicBaseURL()
	if err != nil {
		return nil, err
	}
	metadataURL := samlEndpointURL(baseURL, provider.ID, "metadata")
	acsURL := samlEndpointURL(baseURL, provider.ID, "acs")
	entityID := metadataURL.String()
	return &saml.ServiceProvider{
		EntityID:          entityID,
		MetadataURL:       metadataURL,
		AcsURL:            acsURL,
		AuthnNameIDFormat: saml.UnspecifiedNameIDFormat,
		SignatureVerifier: samlSHA2SignatureVerifier{},
		ValidateAudienceRestriction: func(assertion *saml.Assertion) error {
			if assertion.Conditions == nil || len(assertion.Conditions.AudienceRestrictions) == 0 {
				return errors.New("audience restriction is required")
			}
			for _, restriction := range assertion.Conditions.AudienceRestrictions {
				if restriction.Audience.Value != entityID {
					return errors.New("audience restriction does not match this service provider")
				}
			}
			return nil
		},
	}, nil
}

type samlSHA2SignatureVerifier struct{}

func (samlSHA2SignatureVerifier) VerifySignature(validationContext *dsig.ValidationContext, element *etree.Element) error {
	signedInfo := element.FindElement("./Signature/SignedInfo")
	if signedInfo == nil {
		return errors.New("saml signature is missing SignedInfo")
	}
	signatureMethod := signedInfo.FindElement("./SignatureMethod")
	if signatureMethod == nil {
		return errors.New("saml signature is missing SignatureMethod")
	}
	switch signatureMethod.SelectAttrValue("Algorithm", "") {
	case dsig.RSASHA256SignatureMethod, dsig.RSASHA384SignatureMethod, dsig.RSASHA512SignatureMethod,
		dsig.ECDSASHA256SignatureMethod, dsig.ECDSASHA384SignatureMethod, dsig.ECDSASHA512SignatureMethod:
	default:
		return errors.New("saml signature must use a SHA-2 signature method")
	}
	references := signedInfo.FindElements("./Reference")
	if len(references) == 0 {
		return errors.New("saml signature has no references")
	}
	for _, reference := range references {
		digestMethod := reference.FindElement("./DigestMethod")
		if digestMethod == nil {
			return errors.New("saml signature reference is missing DigestMethod")
		}
		switch digestMethod.SelectAttrValue("Algorithm", "") {
		case "http://www.w3.org/2001/04/xmlenc#sha256", "http://www.w3.org/2001/04/xmldsig-more#sha384", "http://www.w3.org/2001/04/xmlenc#sha512":
		default:
			return errors.New("saml signature must use a SHA-2 digest method")
		}
	}
	if _, err := validationContext.Validate(element); err != nil {
		return fmt.Errorf("validate saml XML signature: %w", err)
	}
	return nil
}

func (h *samlHandler) publicBaseURL() (url.URL, error) {
	raw := strings.TrimSpace(h.server.cfg.AlgaPublicURL)
	if raw == "" {
		return url.URL{}, errors.New("alga_public_url is required for saml")
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || !isValidSAMLHostname(u.Hostname()) || !isValidSAMLPort(u.Port()) || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return url.URL{}, errors.New("alga_public_url must be an absolute origin URL")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isSAMLDevelopmentHost(u.Hostname())) {
		return url.URL{}, errors.New("saml requires an https alga_public_url")
	}
	u.RawPath = ""
	u.Path = strings.TrimRight(u.Path, "/")
	return *u, nil
}

func samlEndpointURL(baseURL url.URL, providerID uuid.UUID, endpoint string) url.URL {
	baseURL.Path += "/api/v1/auth/saml/" + providerID.String() + "/" + endpoint
	baseURL.RawPath = ""
	return baseURL
}

func parseSAMLMetadata(raw string) (*saml.EntityDescriptor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > samlMaxMetadataBytes {
		return nil, errors.New("saml metadata is empty or exceeds 512 KiB")
	}
	metadata, err := samlsp.ParseMetadata([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("parse saml metadata: %w", err)
	}
	if metadata.EntityID == "" || len(metadata.EntityID) > 2048 || len(metadata.IDPSSODescriptors) == 0 {
		return nil, errors.New("saml metadata must contain an IdP entity ID and SSO descriptor")
	}
	if !metadata.ValidUntil.IsZero() && time.Now().After(metadata.ValidUntil) {
		return nil, errors.New("saml metadata has expired")
	}
	postEndpoint := ""
	redirectEndpoint := ""
	hasSigningCertificate := false
	for _, descriptor := range metadata.IDPSSODescriptors {
		for _, endpoint := range descriptor.SingleSignOnServices {
			if endpoint.Binding != saml.HTTPPostBinding && endpoint.Binding != saml.HTTPRedirectBinding {
				continue
			}
			parsedEndpoint, err := url.Parse(endpoint.Location)
			if err != nil || !parsedEndpoint.IsAbs() || !isValidSAMLHostname(parsedEndpoint.Hostname()) || !isValidSAMLPort(parsedEndpoint.Port()) || parsedEndpoint.User != nil || parsedEndpoint.Scheme != "https" || parsedEndpoint.Fragment != "" || parsedEndpoint.Opaque != "" {
				return nil, errors.New("saml SSO endpoint must be an absolute https URL")
			}
			if endpoint.Binding == saml.HTTPPostBinding && postEndpoint == "" {
				postEndpoint = endpoint.Location
			}
			if endpoint.Binding == saml.HTTPRedirectBinding && redirectEndpoint == "" {
				redirectEndpoint = endpoint.Location
			}
		}
		for _, key := range descriptor.KeyDescriptors {
			if key.Use != "" && key.Use != "signing" {
				continue
			}
			for _, cert := range key.KeyInfo.X509Data.X509Certificates {
				encoded := strings.Join(strings.Fields(cert.Data), "")
				if encoded == "" {
					continue
				}
				der, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					return nil, errors.New("saml metadata contains an invalid signing certificate")
				}
				parsedCert, err := x509.ParseCertificate(der)
				if err != nil {
					return nil, errors.New("saml metadata contains an invalid signing certificate")
				}
				now := time.Now()
				if now.Before(parsedCert.NotBefore) || !now.Before(parsedCert.NotAfter) {
					continue
				}
				if parsedCert.KeyUsage != 0 && parsedCert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
					return nil, errors.New("saml signing certificate does not allow digital signatures")
				}
				switch publicKey := parsedCert.PublicKey.(type) {
				case *rsa.PublicKey:
					if publicKey.N.BitLen() < 2048 {
						return nil, errors.New("saml signing certificate RSA key is smaller than 2048 bits")
					}
				case *ecdsa.PublicKey:
					if publicKey.Curve == nil || publicKey.Curve.Params().BitSize < 256 {
						return nil, errors.New("saml signing certificate EC key is smaller than 256 bits")
					}
				default:
					return nil, errors.New("saml signing certificate uses an unsupported public key")
				}
				hasSigningCertificate = true
			}
		}
	}
	if metadata.EntityID == "" || postEndpoint == "" && redirectEndpoint == "" || !hasSigningCertificate {
		return nil, errors.New("saml metadata must include an HTTP-POST or HTTP-Redirect SSO endpoint and signing certificate")
	}
	return metadata, nil
}

func isSAMLDevelopmentHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func isValidSAMLHostname(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			isAlphanumeric := (character >= 'a' && character <= 'z') ||
				(character >= 'A' && character <= 'Z') ||
				(character >= '0' && character <= '9')
			if !isAlphanumeric && character != '-' {
				return false
			}
		}
	}
	return true
}

func isValidSAMLPort(port string) bool {
	if port == "" {
		return true
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535
}

func safeSAMLNextPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\\\r\n") || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "/"
	}
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || u.User != nil || strings.HasPrefix(u.Path, "//") {
		return "/"
	}
	return value
}

func samlAttribute(assertion *saml.Assertion, name string) (string, error) {
	if assertion == nil || name == "" {
		return "", errors.New("saml attribute is not configured")
	}
	var found string
	for _, statement := range assertion.AttributeStatements {
		for _, attribute := range statement.Attributes {
			if attribute.Name != name {
				continue
			}
			if len(attribute.Values) != 1 || strings.TrimSpace(attribute.Values[0].Value) == "" || found != "" {
				return "", errors.New("saml attribute is ambiguous")
			}
			found = attribute.Values[0].Value
		}
	}
	if found == "" {
		return "", errors.New("saml attribute was not found")
	}
	return found, nil
}

func requireSinglePlaintextAssertion(encoded string) error {
	if len(encoded) == 0 || len(encoded) > samlMaxResponseBytes {
		return errors.New("saml response is empty or exceeds the size limit")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return errors.New("saml response is not valid base64")
	}
	if err := xmlroundtripvalidator.Validate(bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("validate saml response xml: %w", err)
	}
	var response saml.Response
	//#nosec G709 -- encoding/xml never resolves external entities; the payload is size-capped and must pass the round-trip validator before decoding.
	if err := xml.Unmarshal(raw, &response); err != nil {
		return fmt.Errorf("decode saml response structure: %w", err)
	}
	if response.Assertion == nil || response.Assertion.Subject == nil || response.Assertion.Subject.NameID == nil ||
		response.Assertion.Conditions == nil || len(response.Assertion.Subject.SubjectConfirmations) == 0 ||
		response.IssueInstant.IsZero() || response.IssueInstant.After(time.Now().Add(samlMaxClockSkew)) {
		return errors.New("saml response is missing required response or assertion fields")
	}
	for _, confirmation := range response.Assertion.Subject.SubjectConfirmations {
		if confirmation.SubjectConfirmationData == nil {
			return errors.New("saml response is missing subject confirmation data")
		}
	}
	decoder := xml.NewDecoder(io.LimitReader(bytes.NewReader(raw), samlMaxResponseBytes))
	type elementFrame struct {
		name                xml.Name
		directAudienceCount int
	}
	frames := make([]elementFrame, 0, 16)
	assertions := 0
	encryptedAssertions := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("parse saml response shape: %w", err)
		}
		switch item := token.(type) {
		case xml.StartElement:
			if len(frames) == 0 && (item.Name.Local != "Response" || item.Name.Space != "urn:oasis:names:tc:SAML:2.0:protocol") {
				return errors.New("saml response root is invalid")
			}
			if len(frames) == 1 && item.Name.Space == "urn:oasis:names:tc:SAML:2.0:assertion" {
				switch item.Name.Local {
				case "Assertion":
					assertions++
				case "EncryptedAssertion":
					encryptedAssertions++
				}
			}
			if len(frames) > 0 {
				parent := &frames[len(frames)-1]
				if parent.name.Local == "AudienceRestriction" && parent.name.Space == "urn:oasis:names:tc:SAML:2.0:assertion" &&
					item.Name.Local == "Audience" && item.Name.Space == "urn:oasis:names:tc:SAML:2.0:assertion" {
					parent.directAudienceCount++
				}
			}
			frames = append(frames, elementFrame{name: item.Name})
		case xml.EndElement:
			if len(frames) == 0 || frames[len(frames)-1].name != item.Name {
				return errors.New("saml response XML structure is invalid")
			}
			frame := frames[len(frames)-1]
			if frame.name.Local == "AudienceRestriction" && frame.name.Space == "urn:oasis:names:tc:SAML:2.0:assertion" && frame.directAudienceCount != 1 {
				return errors.New("each saml audience restriction must have exactly one audience")
			}
			frames = frames[:len(frames)-1]
		}
	}
	if len(frames) != 0 || assertions != 1 || encryptedAssertions != 0 {
		return errors.New("saml response must contain one unencrypted assertion")
	}
	return nil
}

func validateSAMLAssertionLifetime(assertion *saml.Assertion, now time.Time) error {
	if assertion == nil || assertion.Subject == nil || assertion.IssueInstant.IsZero() ||
		assertion.IssueInstant.After(now.Add(samlMaxClockSkew)) || assertion.Conditions == nil || assertion.Conditions.NotOnOrAfter.IsZero() {
		return errors.New("saml assertion has missing or future timestamps")
	}
	if assertion.Conditions.NotOnOrAfter.Sub(assertion.IssueInstant) > samlMaxAssertionLifetime {
		return errors.New("saml assertion lifetime exceeds the configured maximum")
	}
	for _, confirmation := range assertion.Subject.SubjectConfirmations {
		if confirmation.SubjectConfirmationData == nil || confirmation.SubjectConfirmationData.NotOnOrAfter.IsZero() ||
			confirmation.SubjectConfirmationData.NotOnOrAfter.Sub(assertion.IssueInstant) > samlMaxAssertionLifetime {
			return errors.New("saml subject confirmation lifetime exceeds the configured maximum")
		}
	}
	return nil
}

func (h *samlHandler) fail(w http.ResponseWriter, r *http.Request, providerID uuid.UUID, reason string) {
	h.server.auditStore.Log(store.AuditSAMLLoginFailed, nil, "", r.RemoteAddr, r.UserAgent(), false, map[string]any{
		"provider_id": providerID,
		"reason":      reason,
	})
	logger.WarnCtx(r.Context(), "saml sign-in failed", "component", "api", "provider_id", providerID, "reason", reason)
	http.Redirect(w, r, "/login?error=saml_auth_failed", http.StatusFound)
}

func (s *Server) handleSAMLOauthProviders(w http.ResponseWriter, r *http.Request) {
	if s.samlHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "saml sign-in is not configured")
		return
	}
	s.samlHandler.listPublicProviders(w, r)
}

func (s *Server) handleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	if s.samlHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "saml sign-in is not configured")
		return
	}
	id, ok := samlPathID(w, r)
	if !ok {
		return
	}
	s.samlHandler.metadata(w, r, id)
}

func samlPathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, ErrorCodeNotFound, "saml provider not found")
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) handleSAMLAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.samlHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "saml sign-in is not configured")
		return
	}
	id, ok := samlPathID(w, r)
	if !ok {
		return
	}
	s.samlHandler.authorize(w, r, id)
}

func (s *Server) handleSAMLACS(w http.ResponseWriter, r *http.Request) {
	if s.samlHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "saml sign-in is not configured")
		return
	}
	id, ok := samlPathID(w, r)
	if !ok {
		return
	}
	s.samlHandler.assertionConsumer(w, r, id)
}
