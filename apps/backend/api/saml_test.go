package api

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"html"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/crewjam/saml"
	"github.com/google/uuid"
	dsig "github.com/russellhaering/goxmldsig"

	"alga/config"
	"alga/store"
)

const testSAMLIdPIssuer = "https://identity.example.test/saml"

type samlTestKeys struct {
	privateKey  *rsa.PrivateKey
	certificate *x509.Certificate
}

func newSAMLTestKeys(t *testing.T) samlTestKeys {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test SAML key: %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test IAM Identity Center"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create test SAML certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse test SAML certificate: %v", err)
	}
	return samlTestKeys{privateKey: key, certificate: certificate}
}

func (keys samlTestKeys) metadata(t *testing.T, endpoint string) string {
	return keys.metadataWithBinding(t, endpoint, saml.HTTPRedirectBinding)
}

func (keys samlTestKeys) metadataWithBinding(t *testing.T, endpoint, binding string) string {
	t.Helper()
	metadata := saml.EntityDescriptor{
		EntityID: testSAMLIdPIssuer,
		IDPSSODescriptors: []saml.IDPSSODescriptor{{
			SSODescriptor: saml.SSODescriptor{
				RoleDescriptor: saml.RoleDescriptor{
					ProtocolSupportEnumeration: "urn:oasis:names:tc:SAML:2.0:protocol",
					KeyDescriptors: []saml.KeyDescriptor{{
						Use: "signing",
						KeyInfo: saml.KeyInfo{X509Data: saml.X509Data{X509Certificates: []saml.X509Certificate{{
							Data: base64.StdEncoding.EncodeToString(keys.certificate.Raw),
						}}}},
					}},
				},
			},
			SingleSignOnServices: []saml.Endpoint{{
				Binding:  binding,
				Location: endpoint,
			}},
		}},
	}
	data, err := xml.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal test SAML metadata: %v", err)
	}
	return xml.Header + string(data)
}

func TestParseSAMLMetadataRequiresHTTPSAndSigningCertificate(t *testing.T) {
	keys := newSAMLTestKeys(t)
	valid := keys.metadata(t, "https://identity.example.test/sso")
	if _, err := parseSAMLMetadata(valid); err != nil {
		t.Fatalf("valid IdP metadata rejected: %v", err)
	}
	if _, err := parseSAMLMetadata(keys.metadata(t, "http://identity.example.test/sso")); err == nil {
		t.Fatal("metadata with an HTTP sign-in endpoint was accepted")
	}
	if _, err := parseSAMLMetadata(keys.metadataWithBinding(t, "https://identity.example.test/sso", saml.HTTPPostBinding)); err != nil {
		t.Fatalf("valid HTTP-POST SSO metadata rejected: %v", err)
	}
	if _, err := parseSAMLMetadata(strings.Repeat("x", samlMaxMetadataBytes+1)); err == nil {
		t.Fatal("oversized metadata was accepted")
	}
}

func TestSafeSAMLNextPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "/incidents/42?tab=timeline", want: "/incidents/42?tab=timeline"},
		{input: "https://attacker.example/", want: "/"},
		{input: "//attacker.example/", want: "/"},
		{input: "/%2f%2fattacker.example/", want: "/"},
		{input: "/\\attacker.example/", want: "/"},
		{input: "", want: "/"},
	}
	for _, test := range tests {
		if got := safeSAMLNextPath(test.input); got != test.want {
			t.Errorf("safeSAMLNextPath(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestMemorySAMLFlowStoreConsumesStateAndRejectsReplay(t *testing.T) {
	store := newMemorySAMLFlowStore()
	ctx := context.Background()
	state, err := newSAMLRelayState()
	if err != nil {
		t.Fatal(err)
	}
	if !validSAMLRelayState(state) {
		t.Fatalf("generated relay state is invalid: %q", state)
	}
	want := samlLoginPayload{ProviderID: uuid.NewString(), RequestID: "request-1", NextPath: "/"}
	if err := store.Set(ctx, state, want); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wins := make(chan bool, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, err := store.Consume(ctx, state)
			if err != nil {
				t.Errorf("consume state: %v", err)
			}
			wins <- ok
		}()
	}
	wg.Wait()
	close(wins)
	consumed := 0
	for ok := range wins {
		if ok {
			consumed++
		}
	}
	if consumed != 1 {
		t.Fatalf("successful consumes = %d, want exactly one", consumed)
	}
	first, err := store.MarkAssertionUsed(ctx, testSAMLIdPIssuer, "assertion-1")
	if err != nil || !first {
		t.Fatalf("first assertion use = (%v, %v), want (true, nil)", first, err)
	}
	second, err := store.MarkAssertionUsed(ctx, testSAMLIdPIssuer, "assertion-1")
	if err != nil || second {
		t.Fatalf("replayed assertion use = (%v, %v), want (false, nil)", second, err)
	}
}

func TestSAMLAuthorizeAndACSCreateSessionOnce(t *testing.T) {
	keys := newSAMLTestKeys(t)
	providerID := uuid.New()
	provider := &store.SAMLProviderRecord{
		ID:             providerID,
		Name:           "AWS IAM Identity Center",
		IDPEntityID:    testSAMLIdPIssuer,
		MetadataXML:    keys.metadata(t, "https://identity.example.test/sso"),
		EmailAttribute: "email",
		Enabled:        true,
	}
	providerStore := &samlTestProviderStore{provider: provider}
	identityStore := &samlTestIdentityStore{identities: make(map[string]*store.SAMLIdentityRecord)}
	user := &store.UserRecord{ID: uuid.New(), Email: "operator@example.test", Role: "operator"}
	userStore := &samlTestUserStore{user: user}
	sessions := &samlTestSessionStore{}
	audit := &recordingAuditStore{}
	server := &Server{
		cfg:           &config.Config{AlgaPublicURL: "https://alga.example.test", SecureCookies: true},
		userStore:     userStore,
		sessionStore:  sessions,
		auditStore:    audit,
		sessionExpiry: 24 * time.Hour,
	}
	flows := newMemorySAMLFlowStore()
	handler := newSAMLHandler(server, providerStore, identityStore, flows)
	server.samlHandler = handler

	authorizeRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/saml/"+providerID.String()+"/authorize?next=%2Fincidents", nil)
	authorizeResponse := httptest.NewRecorder()
	handler.authorize(authorizeResponse, authorizeRequest, providerID)
	if authorizeResponse.Code != http.StatusFound {
		t.Fatalf("authorize status = %d, want 302: %s", authorizeResponse.Code, authorizeResponse.Body.String())
	}
	authURL, err := url.Parse(authorizeResponse.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse IdP redirect: %v", err)
	}
	relayState := authURL.Query().Get("RelayState")
	flow, ok := flows.flows[relayState]
	if !ok {
		t.Fatal("authorize did not persist RelayState")
	}
	sp, err := handler.serviceProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	response := signedSAMLResponse(t, keys, sp, flow.payload.RequestID, sp.EntityID, "assertion-1")
	acsRequest := samlACSRequest(t, providerID, relayState, response)
	acsResponse := httptest.NewRecorder()
	handler.assertionConsumer(acsResponse, acsRequest, providerID)
	if acsResponse.Code != http.StatusFound {
		t.Fatalf("ACS status = %d, want 302: %s", acsResponse.Code, acsResponse.Body.String())
	}
	if got := acsResponse.Header().Get("Location"); !strings.Contains(got, "saml=success") || !strings.Contains(got, "redirect=%2Fincidents") {
		t.Fatalf("ACS redirect = %q, want SAML success and safe next path", got)
	}
	if sessions.created != 1 || userStore.successfulLogins != 1 || len(identityStore.identities) != 1 {
		t.Fatalf("session/login/identity counts = %d/%d/%d, want 1/1/1", sessions.created, userStore.successfulLogins, len(identityStore.identities))
	}
	if got := len(acsResponse.Result().Cookies()); got != 3 {
		t.Fatalf("session cookies = %d, want session, refresh, and CSRF cookies", got)
	}

	replayRequest := samlACSRequest(t, providerID, relayState, response)
	replayResponse := httptest.NewRecorder()
	handler.assertionConsumer(replayResponse, replayRequest, providerID)
	if sessions.created != 1 || replayResponse.Code != http.StatusFound || !strings.Contains(replayResponse.Header().Get("Location"), "saml_auth_failed") {
		t.Fatalf("replay changed session or returned unexpected response: sessions=%d status=%d location=%q", sessions.created, replayResponse.Code, replayResponse.Header().Get("Location"))
	}
}

func TestSAMLAuthorizeUsesNonceProtectedPOSTForAWSBinding(t *testing.T) {
	keys := newSAMLTestKeys(t)
	providerID := uuid.New()
	provider := &store.SAMLProviderRecord{
		ID:             providerID,
		Name:           "AWS IAM Identity Center",
		IDPEntityID:    testSAMLIdPIssuer,
		MetadataXML:    keys.metadataWithBinding(t, "https://identity.example.test/sso", saml.HTTPPostBinding),
		EmailAttribute: "email",
		Enabled:        true,
	}
	flows := newMemorySAMLFlowStore()
	handler := newSAMLHandler(&Server{cfg: &config.Config{AlgaPublicURL: "https://alga.example.test"}}, &samlTestProviderStore{provider: provider}, nil, flows)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/saml/"+providerID.String()+"/authorize?next=%2Fincidents", nil)
	response := httptest.NewRecorder()
	handler.authorize(response, request, providerID)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("POST authorize response = %d %q: %s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `action="https://identity.example.test/sso"`) ||
		!strings.Contains(response.Body.String(), `name="SAMLRequest"`) || !strings.Contains(response.Body.String(), `name="RelayState"`) {
		t.Fatalf("POST authorize page is missing its IdP form fields: %s", response.Body.String())
	}
	policy := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "form-action https://identity.example.test") || !strings.Contains(policy, "script-src 'nonce-") || strings.Contains(policy, "unsafe-inline") {
		t.Fatalf("POST authorize CSP is not nonce-restricted: %q", policy)
	}
	_, noncePolicy, ok := strings.Cut(policy, "script-src 'nonce-")
	if !ok {
		t.Fatalf("POST authorize CSP is missing a script nonce: %q", policy)
	}
	nonce, _, ok := strings.Cut(noncePolicy, "'")
	if !ok || !strings.Contains(html.UnescapeString(response.Body.String()), `<script nonce="`+nonce+`">`) {
		t.Fatalf("POST form script nonce does not match its CSP: policy=%q body=%s", policy, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("POST authorize cache policy = %q, want no-store", response.Header().Get("Cache-Control"))
	}
	encodedRequest := samlHTMLInputValue(response.Body.String(), "SAMLRequest")
	requestXML, err := base64.StdEncoding.DecodeString(encodedRequest)
	if err != nil {
		t.Fatalf("decode SAMLRequest form value: %v", err)
	}
	var authnRequest saml.AuthnRequest
	if err := xml.Unmarshal(requestXML, &authnRequest); err != nil {
		t.Fatalf("parse SAMLRequest form value: %v", err)
	}
	if authnRequest.ProtocolBinding != saml.HTTPPostBinding || authnRequest.Destination != "https://identity.example.test/sso" {
		t.Fatalf("AuthnRequest binding/destination = %q/%q", authnRequest.ProtocolBinding, authnRequest.Destination)
	}
	state := samlHTMLInputValue(response.Body.String(), "RelayState")
	flow, ok := flows.flows[state]
	if !ok || flow.payload.ProviderID != providerID.String() || flow.payload.NextPath != "/incidents" {
		t.Fatalf("POST flow state was not stored correctly: (%+v, %t)", flow, ok)
	}
}

func samlHTMLInputValue(document, name string) string {
	marker := `name="` + name + `" value="`
	_, value, ok := strings.Cut(document, marker)
	if !ok {
		return ""
	}
	value, _, _ = strings.Cut(value, `"`)
	return html.UnescapeString(value)
}

func TestSAMLACSRejectsWrongAudienceWithoutCreatingSession(t *testing.T) {
	keys := newSAMLTestKeys(t)
	providerID := uuid.New()
	provider := &store.SAMLProviderRecord{
		ID:             providerID,
		Name:           "AWS IAM Identity Center",
		IDPEntityID:    testSAMLIdPIssuer,
		MetadataXML:    keys.metadata(t, "https://identity.example.test/sso"),
		EmailAttribute: "email",
		Enabled:        true,
	}
	flows := newMemorySAMLFlowStore()
	server := &Server{
		cfg:           &config.Config{AlgaPublicURL: "https://alga.example.test", SecureCookies: true},
		userStore:     &samlTestUserStore{user: &store.UserRecord{ID: uuid.New(), Email: "operator@example.test"}},
		sessionStore:  &samlTestSessionStore{},
		auditStore:    &recordingAuditStore{},
		sessionExpiry: time.Hour,
	}
	handler := newSAMLHandler(server, &samlTestProviderStore{provider: provider}, &samlTestIdentityStore{identities: make(map[string]*store.SAMLIdentityRecord)}, flows)
	state, err := newSAMLRelayState()
	if err != nil {
		t.Fatal(err)
	}
	requestID := "request-wrong-audience"
	if err := flows.Set(context.Background(), state, samlLoginPayload{ProviderID: providerID.String(), RequestID: requestID, NextPath: "/"}); err != nil {
		t.Fatal(err)
	}
	sp, err := handler.serviceProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	response := signedSAMLResponse(t, keys, sp, requestID, "https://attacker.example/", "assertion-wrong-audience")
	recorder := httptest.NewRecorder()
	handler.assertionConsumer(recorder, samlACSRequest(t, providerID, state, response), providerID)
	if server.sessionStore.(*samlTestSessionStore).created != 0 {
		t.Fatal("wrong-audience assertion created a session")
	}
	if !strings.Contains(recorder.Header().Get("Location"), "saml_auth_failed") {
		t.Fatalf("wrong-audience response redirect = %q", recorder.Header().Get("Location"))
	}
}

func signedSAMLResponse(t *testing.T, keys samlTestKeys, sp *saml.ServiceProvider, requestID, audience, assertionID string) string {
	return signedSAMLResponseWithHash(t, keys, sp, requestID, audience, assertionID, crypto.SHA256)
}

func signedSAMLResponseWithHash(t *testing.T, keys samlTestKeys, sp *saml.ServiceProvider, requestID, audience, assertionID string, signatureHash crypto.Hash) string {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	assertion := &saml.Assertion{
		ID:           assertionID,
		IssueInstant: now,
		Version:      "2.0",
		Issuer:       saml.Issuer{Value: testSAMLIdPIssuer},
		Subject: &saml.Subject{
			NameID: &saml.NameID{Format: string(saml.PersistentNameIDFormat), Value: "user-123"},
			SubjectConfirmations: []saml.SubjectConfirmation{{
				Method: "urn:oasis:names:tc:SAML:2.0:cm:bearer",
				SubjectConfirmationData: &saml.SubjectConfirmationData{
					InResponseTo: requestID,
					NotOnOrAfter: now.Add(5 * time.Minute),
					Recipient:    sp.AcsURL.String(),
				},
			}},
		},
		Conditions: &saml.Conditions{
			NotBefore:    now.Add(-time.Minute),
			NotOnOrAfter: now.Add(5 * time.Minute),
			AudienceRestrictions: []saml.AudienceRestriction{{
				Audience: saml.Audience{Value: audience},
			}},
		},
		AttributeStatements: []saml.AttributeStatement{{
			Attributes: []saml.Attribute{{Name: "email", Values: []saml.AttributeValue{{Value: "operator@example.test"}}}},
		}},
	}
	response := &saml.Response{
		ID:           "response-" + assertionID,
		InResponseTo: requestID,
		Version:      "2.0",
		IssueInstant: now,
		Destination:  sp.AcsURL.String(),
		Issuer:       &saml.Issuer{Value: testSAMLIdPIssuer},
		Status:       saml.Status{StatusCode: saml.StatusCode{Value: saml.StatusSuccess}},
		Assertion:    assertion,
	}
	keyPair := tls.Certificate{Certificate: [][]byte{keys.certificate.Raw}, PrivateKey: keys.privateKey, Leaf: keys.certificate}
	signingContext := dsig.NewDefaultSigningContext(dsig.TLSCertKeyStore(keyPair))
	signingContext.Hash = signatureHash
	signingContext.Canonicalizer = dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList("")
	signed, err := signingContext.SignEnveloped(response.Element())
	if err != nil {
		t.Fatalf("sign test SAML response: %v", err)
	}
	document := etree.NewDocument()
	document.SetRoot(signed)
	data, err := document.WriteToBytes()
	if err != nil {
		t.Fatalf("serialize test SAML response: %v", err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func samlACSRequest(t *testing.T, providerID uuid.UUID, relayState, response string) *http.Request {
	t.Helper()
	form := url.Values{"RelayState": {relayState}, "SAMLResponse": {response}}
	request := httptest.NewRequest(http.MethodPost, "https://alga.example.test/api/v1/auth/saml/"+providerID.String()+"/acs", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetPathValue("id", providerID.String())
	request.RemoteAddr = "192.0.2.10:12345"
	return request
}

type samlTestProviderStore struct {
	store.SAMLProviderStore
	provider    *store.SAMLProviderRecord
	updateCalls int
}

func (s *samlTestProviderStore) CreateProvider(_ context.Context, provider *store.SAMLProviderRecord) (*store.SAMLProviderRecord, error) {
	copy := *provider
	if copy.ID == uuid.Nil {
		copy.ID = uuid.New()
	}
	s.provider = &copy
	return &copy, nil
}
func (s *samlTestProviderStore) UpdateProvider(_ context.Context, _ uuid.UUID, patch *store.SAMLProviderRecord) (*store.SAMLProviderRecord, error) {
	s.updateCalls++
	if s.provider == nil {
		return nil, store.ErrNotFound
	}
	copy := *s.provider
	if patch.Name != "" {
		copy.Name = patch.Name
	}
	if patch.MetadataXMLSet {
		copy.MetadataXML = patch.MetadataXML
		copy.IDPEntityID = patch.IDPEntityID
	}
	if patch.EnabledSet {
		copy.Enabled = patch.Enabled
	}
	s.provider = &copy
	return &copy, nil
}
func (s *samlTestProviderStore) DeleteProvider(context.Context, uuid.UUID) error { return nil }
func (s *samlTestProviderStore) GetProvider(_ context.Context, id uuid.UUID) (*store.SAMLProviderRecord, error) {
	if s.provider == nil || s.provider.ID != id {
		return nil, store.ErrNotFound
	}
	copy := *s.provider
	return &copy, nil
}
func (s *samlTestProviderStore) ListProviders(context.Context, store.SAMLProviderQuery) ([]store.SAMLProviderRecord, int64, error) {
	return []store.SAMLProviderRecord{*s.provider}, 1, nil
}
func (s *samlTestProviderStore) ListEnabledProviders(context.Context) ([]store.SAMLProviderRecord, error) {
	if s.provider == nil || !s.provider.Enabled {
		return []store.SAMLProviderRecord{}, nil
	}
	return []store.SAMLProviderRecord{*s.provider}, nil
}

type samlTestIdentityStore struct {
	store.SAMLIdentityStore
	identities map[string]*store.SAMLIdentityRecord
}

func (s *samlTestIdentityStore) CreateLink(_ context.Context, record *store.SAMLIdentityRecord) (*store.SAMLIdentityRecord, error) {
	copy := *record
	s.identities[record.ProviderID.String()+":"+record.Subject] = &copy
	return &copy, nil
}
func (s *samlTestIdentityStore) GetByProviderSubject(_ context.Context, providerID uuid.UUID, subject string) (*store.SAMLIdentityRecord, error) {
	record := s.identities[providerID.String()+":"+subject]
	if record == nil {
		return nil, store.ErrNotFound
	}
	copy := *record
	return &copy, nil
}

type samlTestUserStore struct {
	store.UserStore
	user             *store.UserRecord
	successfulLogins int
}

func (s *samlTestUserStore) GetByEmail(email string) (*store.UserRecord, error) {
	if s.user == nil || s.user.Email != email {
		return nil, store.ErrNotFound
	}
	copy := *s.user
	return &copy, nil
}
func (s *samlTestUserStore) GetByID(id uuid.UUID) (*store.UserRecord, error) {
	if s.user == nil || s.user.ID != id {
		return nil, store.ErrNotFound
	}
	copy := *s.user
	return &copy, nil
}
func (s *samlTestUserStore) RecordSuccessfulLogin(uuid.UUID, string) error {
	s.successfulLogins++
	return nil
}

type samlTestSessionStore struct {
	store.SessionStore
	created int
}

func (s *samlTestSessionStore) CreateSession(userID uuid.UUID, _, _ string) (*store.SessionRecord, error) {
	s.created++
	return &store.SessionRecord{ID: "session-id", UserID: userID, RefreshToken: "refresh-token"}, nil
}

var _ store.SAMLProviderStore = (*samlTestProviderStore)(nil)
var _ store.SAMLIdentityStore = (*samlTestIdentityStore)(nil)
var _ store.UserStore = (*samlTestUserStore)(nil)
var _ store.SessionStore = (*samlTestSessionStore)(nil)

func TestSAMLResponseHasOnlyOneAssertion(t *testing.T) {
	encoded := validSignedTestResponse(t)
	document := parseTestSAMLResponse(t, encoded)
	assertions := document.FindElements(".//Assertion")
	if len(assertions) != 1 {
		t.Fatalf("test response assertions = %d, want 1", len(assertions))
	}
	assertions[0].Parent().AddChild(assertions[0].Copy())
	duplicated := encodeTestSAMLResponse(t, document)
	err := requireSinglePlaintextAssertion(duplicated)
	if err == nil || !strings.Contains(err.Error(), "one unencrypted assertion") {
		t.Fatalf("response containing two assertions returned %v, want assertion multiplicity rejection", err)
	}
}

func TestSAMLResponseRejectsMultipleAudienceValuesInRestriction(t *testing.T) {
	document := parseTestSAMLResponse(t, validSignedTestResponse(t))
	restrictions := document.FindElements(".//AudienceRestriction")
	if len(restrictions) != 1 {
		t.Fatalf("test response audience restrictions = %d, want 1", len(restrictions))
	}
	audiences := restrictions[0].FindElements("./Audience")
	if len(audiences) != 1 {
		t.Fatalf("test response audience values = %d, want 1", len(audiences))
	}
	extra := audiences[0].Copy()
	extra.SetText("https://attacker.example/")
	restrictions[0].AddChild(extra)
	err := requireSinglePlaintextAssertion(encodeTestSAMLResponse(t, document))
	if err == nil || !strings.Contains(err.Error(), "exactly one audience") {
		t.Fatalf("response with ambiguous audiences returned %v, want rejection", err)
	}
}

func validSignedTestResponse(t *testing.T) string {
	t.Helper()
	keys := newSAMLTestKeys(t)
	provider := &store.SAMLProviderRecord{
		ID:             uuid.New(),
		IDPEntityID:    testSAMLIdPIssuer,
		MetadataXML:    keys.metadata(t, "https://identity.example.test/sso"),
		EmailAttribute: "email",
	}
	handler := newSAMLHandler(&Server{cfg: &config.Config{AlgaPublicURL: "https://alga.example.test"}}, nil, nil, nil)
	sp, err := handler.serviceProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	return signedSAMLResponse(t, keys, sp, "request-shape", sp.EntityID, "assertion-shape")
}

func parseTestSAMLResponse(t *testing.T, encoded string) *etree.Document {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode test SAML response: %v", err)
	}
	document := etree.NewDocument()
	if err := document.ReadFromBytes(raw); err != nil {
		t.Fatalf("parse test SAML response: %v", err)
	}
	return document
}

func encodeTestSAMLResponse(t *testing.T, document *etree.Document) string {
	t.Helper()
	raw, err := document.WriteToBytes()
	if err != nil {
		t.Fatalf("serialize test SAML response: %v", err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func TestSAMLSignatureUsesConfiguredIdPCertificate(t *testing.T) {
	keys := newSAMLTestKeys(t)
	provider := &store.SAMLProviderRecord{
		ID:             uuid.New(),
		IDPEntityID:    testSAMLIdPIssuer,
		MetadataXML:    keys.metadata(t, "https://identity.example.test/sso"),
		EmailAttribute: "email",
		Enabled:        true,
	}
	server := &Server{cfg: &config.Config{AlgaPublicURL: "https://alga.example.test"}}
	handler := newSAMLHandler(server, nil, nil, nil)
	sp, err := handler.serviceProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	encoded := signedSAMLResponse(t, keys, sp, "request-test", sp.EntityID, "assertion-test")
	response := httptest.NewRequest(http.MethodPost, sp.AcsURL.String(), nil)
	response.PostForm = url.Values{"SAMLResponse": {encoded}}
	assertion, err := sp.ParseResponse(response, []string{"request-test"})
	if err != nil {
		t.Fatalf("valid signed response rejected: %v", err)
	}
	if assertion.ID != "assertion-test" {
		t.Fatalf("assertion id = %q, want assertion-test", assertion.ID)
	}
}

func TestSAMLRejectsSHA1Signature(t *testing.T) {
	keys := newSAMLTestKeys(t)
	provider := &store.SAMLProviderRecord{
		ID:             uuid.New(),
		IDPEntityID:    testSAMLIdPIssuer,
		MetadataXML:    keys.metadata(t, "https://identity.example.test/sso"),
		EmailAttribute: "email",
	}
	handler := newSAMLHandler(&Server{cfg: &config.Config{AlgaPublicURL: "https://alga.example.test"}}, nil, nil, nil)
	sp, err := handler.serviceProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	encoded := signedSAMLResponseWithHash(t, keys, sp, "request-sha1", sp.EntityID, "assertion-sha1", crypto.SHA1)
	request := httptest.NewRequest(http.MethodPost, sp.AcsURL.String(), nil)
	request.PostForm = url.Values{"SAMLResponse": {encoded}}
	if _, err := sp.ParseResponse(request, []string{"request-sha1"}); err == nil {
		t.Fatal("SHA-1 signed SAML response was accepted")
	}
}

func TestSAMLMetadataResponseOmitsUnsupportedArtifactBinding(t *testing.T) {
	keys := newSAMLTestKeys(t)
	provider := &store.SAMLProviderRecord{ID: uuid.New(), Name: "AWS", IDPEntityID: testSAMLIdPIssuer, MetadataXML: keys.metadata(t, "https://identity.example.test/sso"), EmailAttribute: "email", Enabled: true}
	handler := newSAMLHandler(&Server{cfg: &config.Config{AlgaPublicURL: "https://alga.example.test"}}, &samlTestProviderStore{provider: provider}, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/saml/"+provider.ID.String()+"/metadata", nil)
	request.SetPathValue("id", provider.ID.String())
	response := httptest.NewRecorder()
	handler.metadata(response, request, provider.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("metadata status = %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), saml.HTTPArtifactBinding) || !strings.Contains(response.Body.String(), saml.HTTPPostBinding) {
		t.Fatalf("metadata exposes unexpected ACS bindings: %s", response.Body.String())
	}
}

func TestSAMLMetadataAndEndpointsAreAvailableForDraftProvider(t *testing.T) {
	provider := &store.SAMLProviderRecord{ID: uuid.New(), Name: "AWS IAM Identity Center", EmailAttribute: "email"}
	handler := newSAMLHandler(&Server{cfg: &config.Config{AlgaPublicURL: "https://alga.example.test"}}, &samlTestProviderStore{provider: provider}, nil, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/saml/"+provider.ID.String()+"/metadata", nil)
	handler.metadata(response, request, provider.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("draft metadata status = %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), provider.ID.String()+"/metadata") || !strings.Contains(response.Body.String(), provider.ID.String()+"/acs") {
		t.Fatalf("draft SP metadata is missing provider endpoints: %s", response.Body.String())
	}
	adminResponse := handler.providerResponse(provider)
	if adminResponse.ConfigurationValid || adminResponse.MetadataConfigured || adminResponse.ACSURL == "" || adminResponse.EntityID == "" || adminResponse.AuthorizeURL == "" {
		t.Fatalf("draft provider response does not expose setup values: %+v", adminResponse)
	}
}

func TestSAMLProviderCanBeCreatedAsDisabledDraft(t *testing.T) {
	providerStore := &samlTestProviderStore{}
	server := &Server{cfg: &config.Config{AlgaPublicURL: "https://alga.example.test"}, auditStore: &recordingAuditStore{}}
	handler := newSAMLHandler(server, providerStore, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/saml/providers", strings.NewReader(`{"name":"AWS IAM Identity Center"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.createProvider(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create draft status = %d: %s", response.Code, response.Body.String())
	}
	if providerStore.provider == nil || providerStore.provider.Enabled || providerStore.provider.MetadataXML != "" || providerStore.provider.IDPEntityID != "" {
		t.Fatalf("created provider is not a disabled draft: %+v", providerStore.provider)
	}
	if !strings.Contains(response.Body.String(), providerStore.provider.ID.String()+"/acs") {
		t.Fatalf("created draft response did not include the ACS URL: %s", response.Body.String())
	}
}

func TestSAMLProviderDraftCannotBeEnabledWithoutMetadata(t *testing.T) {
	provider := &store.SAMLProviderRecord{ID: uuid.New(), Name: "AWS IAM Identity Center"}
	providerStore := &samlTestProviderStore{provider: provider}
	server := &Server{cfg: &config.Config{AlgaPublicURL: "https://alga.example.test"}, auditStore: &recordingAuditStore{}}
	handler := newSAMLHandler(server, providerStore, nil, nil)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/saml/providers/"+provider.ID.String(), strings.NewReader(`{"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.updateProvider(response, request, provider.ID)
	if response.Code != http.StatusUnprocessableEntity || providerStore.updateCalls != 0 {
		t.Fatalf("enabling draft status/calls = %d/%d, want 422/0: %s", response.Code, providerStore.updateCalls, response.Body.String())
	}
}

func TestSAMLProviderIssuerCannotChangeAfterIdentitySourceIsSet(t *testing.T) {
	keys := newSAMLTestKeys(t)
	provider := &store.SAMLProviderRecord{
		ID:             uuid.New(),
		Name:           "AWS IAM Identity Center",
		IDPEntityID:    testSAMLIdPIssuer,
		MetadataXML:    keys.metadata(t, "https://identity.example.test/sso"),
		EmailAttribute: "email",
	}
	providerStore := &samlTestProviderStore{provider: provider}
	server := &Server{cfg: &config.Config{AlgaPublicURL: "https://alga.example.test"}, auditStore: &recordingAuditStore{}}
	handler := newSAMLHandler(server, providerStore, nil, nil)
	changedMetadata := strings.Replace(provider.MetadataXML, testSAMLIdPIssuer, "https://different-idp.example.test/saml", 1)
	body, err := json.Marshal(map[string]string{"metadata_xml": changedMetadata})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/saml/providers/"+provider.ID.String(), strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.updateProvider(response, request, provider.ID)
	if response.Code != http.StatusUnprocessableEntity || providerStore.updateCalls != 0 {
		t.Fatalf("issuer change status/calls = %d/%d, want 422/0: %s", response.Code, providerStore.updateCalls, response.Body.String())
	}
}
