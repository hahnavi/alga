package api

import (
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"

	algacrypto "alga/crypto"
	"alga/logger"
	"alga/rbac"
	"alga/store"
)

var ldapAttributePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,63}$`)

type ldapProviderInput struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	BaseDN         string `json:"base_dn"`
	BindDN         string `json:"bind_dn"`
	BindPassword   string `json:"bind_password"`
	LoginAttribute string `json:"login_attribute"`
	IDAttribute    string `json:"id_attribute"`
	EmailAttribute string `json:"email_attribute"`
	StartTLS       bool   `json:"start_tls"`
	Enabled        bool   `json:"enabled"`
}

type ldapProviderView struct {
	ID                     uuid.UUID `json:"id"`
	Name                   string    `json:"name"`
	URL                    string    `json:"url"`
	BaseDN                 string    `json:"base_dn"`
	BindDN                 string    `json:"bind_dn"`
	BindPasswordConfigured bool      `json:"bind_password_configured"`
	LoginAttribute         string    `json:"login_attribute"`
	IDAttribute            string    `json:"id_attribute"`
	EmailAttribute         string    `json:"email_attribute"`
	StartTLS               bool      `json:"start_tls"`
	Enabled                bool      `json:"enabled"`
}

func ldapView(p *store.LDAPProviderRecord) ldapProviderView {
	return ldapProviderView{ID: p.ID, Name: p.Name, URL: p.URL, BaseDN: p.BaseDN, BindDN: p.BindDN,
		BindPasswordConfigured: p.BindPasswordEncrypted != "", LoginAttribute: p.LoginAttribute,
		IDAttribute: p.IDAttribute, EmailAttribute: p.EmailAttribute, StartTLS: p.StartTLS, Enabled: p.Enabled}
}

func validateLDAPProvider(p *store.LDAPProviderRecord) error {
	if p.Name == "" || len(p.Name) > 128 || p.BaseDN == "" || p.BindDN == "" || (p.Enabled && p.BindPasswordEncrypted == "") {
		return errors.New("name, base DN, and bind DN are required; enabled providers also need a bind password")
	}
	if _, err := ldap.ParseDN(p.BaseDN); err != nil {
		return errors.New("invalid search base DN")
	}
	if _, err := ldap.ParseDN(p.BindDN); err != nil {
		return errors.New("invalid bind DN")
	}
	for _, attribute := range []string{p.LoginAttribute, p.IDAttribute, p.EmailAttribute} {
		if !ldapAttributePattern.MatchString(attribute) {
			return errors.New("invalid LDAP attribute name")
		}
	}
	u, err := url.Parse(p.URL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid LDAP URL")
	}
	if u.Scheme != "ldaps" && (u.Scheme != "ldap" || !p.StartTLS) {
		return errors.New("use LDAPS or LDAP with StartTLS")
	}
	if u.Scheme == "ldaps" && p.StartTLS {
		return errors.New("StartTLS requires an ldap:// URL")
	}
	return nil
}

func (s *Server) ldapStoresReady(w http.ResponseWriter) bool {
	if s.ldapProviderStore == nil || s.ldapIdentityStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "ldap sign-in is not configured")
		return false
	}
	return true
}

func (s *Server) handleLDAPProviders(w http.ResponseWriter, r *http.Request) {
	if !s.ldapStoresReady(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.checkPermission(w, r, rbac.OIDCManage) {
			return
		}
		items, err := s.ldapProviderStore.List(r.Context(), false)
		if err != nil {
			writeInternalError(w, err, "failed to list ldap providers")
			return
		}
		views := make([]ldapProviderView, 0, len(items))
		for i := range items {
			views = append(views, ldapView(&items[i]))
		}
		writeJSON(w, http.StatusOK, views)
	case http.MethodPost:
		if !s.checkPermission(w, r, rbac.OIDCManage) {
			return
		}
		var input ldapProviderInput
		if !decodeJSON(w, r, &input) {
			return
		}
		p, err := ldapRecordFromInput(input, "")
		if err != nil {
			writeError(w, ErrorCodeValidationFailed, err.Error())
			return
		}
		created, err := s.ldapProviderStore.Create(r.Context(), p)
		if err != nil {
			s.ldapWriteError(w, err)
			return
		}
		s.auditLDAPProvider(r, store.AuditLDAPProviderCreated, created.ID)
		writeJSON(w, http.StatusCreated, ldapView(created))
	default:
		w.Header().Set("Allow", "GET, POST")
		writeMethodNotAllowed(w)
	}
}

func ldapRecordFromInput(input ldapProviderInput, existingSecret string) (*store.LDAPProviderRecord, error) {
	secret := existingSecret
	if input.BindPassword != "" {
		var err error
		secret, err = algacrypto.Default().EncryptString(input.BindPassword)
		if err != nil {
			return nil, errors.New("credential encryption is unavailable")
		}
	}
	p := &store.LDAPProviderRecord{Name: strings.TrimSpace(input.Name), URL: strings.TrimSpace(input.URL),
		BaseDN: strings.TrimSpace(input.BaseDN), BindDN: strings.TrimSpace(input.BindDN), BindPasswordEncrypted: secret,
		LoginAttribute: strings.TrimSpace(input.LoginAttribute), IDAttribute: strings.TrimSpace(input.IDAttribute),
		EmailAttribute: strings.TrimSpace(input.EmailAttribute), StartTLS: input.StartTLS, Enabled: input.Enabled}
	return p, validateLDAPProvider(p)
}

func (s *Server) handleLDAPProviderByID(w http.ResponseWriter, r *http.Request) {
	if !s.ldapStoresReady(w) || !s.checkPermission(w, r, rbac.OIDCManage) {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, ErrorCodeValidationFailed, "invalid ldap provider ID")
		return
	}
	switch r.Method {
	case http.MethodPut:
		current, err := s.ldapProviderStore.Get(r.Context(), id)
		if err != nil {
			s.ldapWriteError(w, err)
			return
		}
		var input ldapProviderInput
		if !decodeJSON(w, r, &input) {
			return
		}
		p, err := ldapRecordFromInput(input, current.BindPasswordEncrypted)
		if err != nil {
			writeError(w, ErrorCodeValidationFailed, err.Error())
			return
		}
		// A stable identity link is scoped to the search authority and ID attribute.
		if current.URL != p.URL || current.BaseDN != p.BaseDN || current.IDAttribute != p.IDAttribute {
			writeError(w, ErrorCodeValidationFailed, "directory URL, search base, and identity attribute cannot be changed; create a new provider")
			return
		}
		p.ID = id
		updated, err := s.ldapProviderStore.Update(r.Context(), p)
		if err != nil {
			s.ldapWriteError(w, err)
			return
		}
		s.auditLDAPProvider(r, store.AuditLDAPProviderUpdated, id)
		writeJSON(w, http.StatusOK, ldapView(updated))
	case http.MethodDelete:
		if err := s.ldapProviderStore.Delete(r.Context(), id); err != nil {
			s.ldapWriteError(w, err)
			return
		}
		s.auditLDAPProvider(r, store.AuditLDAPProviderDeleted, id)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "PUT, DELETE")
		writeMethodNotAllowed(w)
	}
}

func (s *Server) ldapWriteError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, ErrorCodeNotFound, "ldap provider not found")
		return
	}
	if store.IsDuplicateKey(err) {
		writeError(w, ErrorCodeConflict, "ldap provider name already exists")
		return
	}
	writeInternalError(w, err, "ldap provider operation failed")
}

func (s *Server) auditLDAPProvider(r *http.Request, event store.AuditEvent, id uuid.UUID) {
	user := userFromContext(r.Context())
	if user != nil {
		s.auditStore.Log(event, &user.ID, user.Email, s.ipExtractor.clientIP(r), r.UserAgent(), true, map[string]any{"provider_id": id})
	}
}

func (s *Server) handleLDAPPublicProviders(w http.ResponseWriter, r *http.Request) {
	if !s.ldapStoresReady(w) {
		return
	}
	items, err := s.ldapProviderStore.List(r.Context(), true)
	if err != nil {
		writeInternalError(w, err, "failed to list ldap providers")
		return
	}
	views := make([]map[string]any, 0, len(items))
	for i := range items {
		views = append(views, map[string]any{"id": items[i].ID, "name": items[i].Name})
	}
	writeJSON(w, http.StatusOK, views)
}

func ldapAuthenticate(p *store.LDAPProviderRecord, login, password string) (subject, email string, err error) {
	if password == "" || login == "" || len(login) > 256 {
		return "", "", errors.New("invalid credentials")
	}
	if err := validateLDAPProvider(p); err != nil {
		return "", "", err
	}
	u, _ := url.Parse(p.URL)
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}
	conn, err := ldap.DialURL(p.URL, ldap.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}), ldap.DialWithTLSConfig(tlsConfig))
	if err != nil {
		return "", "", err
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			logger.Warn("ldap connection close failed", "component", "api", "error", closeErr)
		}
	}()
	conn.SetTimeout(5 * time.Second)
	if p.StartTLS {
		if err := conn.StartTLS(tlsConfig); err != nil {
			return "", "", err
		}
	}
	servicePassword, err := algacrypto.Default().DecryptString(p.BindPasswordEncrypted)
	if err != nil {
		return "", "", err
	}
	if err := conn.Bind(p.BindDN, servicePassword); err != nil {
		return "", "", err
	}
	filter := "(" + p.LoginAttribute + "=" + ldap.EscapeFilter(login) + ")"
	result, err := conn.Search(ldap.NewSearchRequest(p.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		2, 5, false, filter, []string{p.IDAttribute, p.EmailAttribute}, nil))
	if err != nil {
		return "", "", err
	}
	if len(result.Entries) != 1 || len(result.Referrals) != 0 {
		return "", "", errors.New("ambiguous directory identity")
	}
	entry := result.Entries[0]
	ids := entry.GetEqualFoldRawAttributeValues(p.IDAttribute)
	emails := entry.GetEqualFoldAttributeValues(p.EmailAttribute)
	if len(ids) != 1 || len(ids[0]) == 0 || len(ids[0]) > 512 || len(emails) != 1 || strings.TrimSpace(emails[0]) == "" {
		return "", "", errors.New("directory identity attributes missing or ambiguous")
	}
	parsedEmail, err := mail.ParseAddress(emails[0])
	if err != nil || parsedEmail.Address != emails[0] || len(emails[0]) > 320 {
		return "", "", errors.New("invalid directory email")
	}
	if err := conn.Bind(entry.DN, password); err != nil {
		return "", "", err
	}
	return hex.EncodeToString(ids[0]), emails[0], nil
}

func (s *Server) handleLDAPLogin(w http.ResponseWriter, r *http.Request) {
	if !s.ldapStoresReady(w) {
		return
	}
	clientIP := s.ipExtractor.clientIP(r)
	allowed, _, lockedUntil := s.loginLimiter.CheckLoginAllowed(clientIP)
	if !allowed {
		s.auditStore.Log(store.AuditLDAPLoginFailed, nil, "", clientIP, r.UserAgent(), false,
			map[string]any{"reason": "rate_limited"})
		if lockedUntil != nil {
			w.Header().Set("Retry-After", fmt.Sprint(max(1, int(time.Until(*lockedUntil).Seconds()))))
		}
		writeError(w, ErrorCodeRateLimited, "too many login attempts. please try again later")
		return
	}
	var input struct {
		ProviderID string `json:"provider_id"`
		Login      string `json:"login"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	id, err := uuid.Parse(input.ProviderID)
	if err != nil || input.Login == "" || input.Password == "" {
		writeError(w, ErrorCodeValidationFailed, "provider, login, and password are required")
		return
	}
	provider, err := s.ldapProviderStore.Get(r.Context(), id)
	if err != nil || !provider.Enabled {
		s.failLDAPLogin(w, r, id, "provider_unavailable")
		return
	}
	subject, email, err := ldapAuthenticate(provider, input.Login, input.Password)
	if err != nil {
		s.failLDAPLogin(w, r, id, "directory_auth_failed")
		return
	}
	identity, err := s.ldapIdentityStore.Get(r.Context(), id, subject)
	var user *store.UserRecord
	if err == nil {
		user, err = s.userStore.GetByID(identity.UserID)
	} else if errors.Is(err, store.ErrNotFound) {
		user, err = s.userStore.GetByEmail(email)
		if err == nil && user != nil {
			if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
				s.failLDAPLogin(w, r, id, "account_locked")
				return
			}
			if err = s.ldapIdentityStore.Create(r.Context(), id, user.ID, subject); err == nil {
				s.auditStore.Log(store.AuditLDAPIdentityLinked, &user.ID, user.Email, clientIP, r.UserAgent(), true, map[string]any{"provider_id": id})
			}
		}
	}
	if err != nil || user == nil {
		s.failLDAPLogin(w, r, id, "account_link_failed")
		return
	}
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		s.failLDAPLogin(w, r, id, "account_locked")
		return
	}
	session, err := s.sessionStore.CreateSession(user.ID, clientIP, r.UserAgent())
	if err != nil {
		writeInternalError(w, err, "failed to create session")
		return
	}
	csrfToken, err := generateCSRFToken()
	if err != nil {
		writeInternalError(w, err, "failed to create session")
		return
	}
	if err := s.userStore.RecordSuccessfulLogin(user.ID, clientIP); err != nil {
		logger.Warn("failed to record ldap login", "component", "api", "error", err)
	}
	s.loginLimiter.Reset(clientIP)
	s.setSessionCookie(w, session.ID)
	setRefreshTokenCookie(w, s.cfg.SecureCookies, s.sessionExpiry, session.RefreshToken)
	s.setCSRFCookie(w, csrfToken)
	s.auditStore.Log(store.AuditLDAPLoginSuccess, &user.ID, user.Email, clientIP, r.UserAgent(), true, map[string]any{"provider_id": id})
	perms := rbac.AllPermissions(user.Role)
	permStrings := make([]string, len(perms))
	for i, p := range perms {
		permStrings[i] = string(p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID.String(), "email": user.Email, "full_name": user.FullName,
		"phone": user.Phone, "phone_country": user.PhoneCountry, "role": user.Role,
		"created_at": user.CreatedAt, "permissions": permStrings, "csrf_token": csrfToken})
}

func (s *Server) failLDAPLogin(w http.ResponseWriter, r *http.Request, providerID uuid.UUID, reason string) {
	s.auditStore.Log(store.AuditLDAPLoginFailed, nil, "", s.ipExtractor.clientIP(r), r.UserAgent(), false,
		map[string]any{"provider_id": providerID, "reason": reason})
	writeError(w, ErrorCodeUnauthorized, "invalid credentials")
}
