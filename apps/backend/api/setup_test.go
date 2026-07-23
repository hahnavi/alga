package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"alga/store"
)

func TestHandleSetupStatus_NeedsSetup(t *testing.T) {
	userStore := newAuthMockUserStore()
	auditStore := newRecordingAuditStore()
	sessionStore := newAuthMockSessionStore()
	_, mux := newAuthTestServer(userStore, sessionStore, auditStore, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["needs_setup"] != true {
		t.Fatalf("expected needs_setup true, got %v", resp["needs_setup"])
	}
}

func TestHandleSetupStatus_SetupComplete(t *testing.T) {
	userStore := newAuthMockUserStore()
	userStore.addUser("existing@alga.local", "P@ssw0rd!", "admin")
	auditStore := newRecordingAuditStore()
	sessionStore := newAuthMockSessionStore()
	_, mux := newAuthTestServer(userStore, sessionStore, auditStore, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["needs_setup"] != false {
		t.Fatalf("expected needs_setup false, got %v", resp["needs_setup"])
	}
}

func TestHandleSetup_Success(t *testing.T) {
	userStore := newAuthMockUserStore()
	auditStore := newRecordingAuditStore()
	sessionStore := newAuthMockSessionStore()
	_, mux := newAuthTestServer(userStore, sessionStore, auditStore, nil)

	body := bytes.NewBufferString(`{"email":"boss@alga.local","password":"Str0ng!Pass","full_name":"Boss User"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["email"] != "boss@alga.local" {
		t.Fatalf("expected email boss@alga.local, got %v", resp["email"])
	}
	if resp["role"] != "admin" {
		t.Fatalf("expected role admin, got %v", resp["role"])
	}
	if _, ok := resp["csrf_token"]; !ok {
		t.Fatal("expected csrf_token in response")
	}

	var sessionCookie, csrfCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case "alga_session":
			sessionCookie = c
		case "alga_csrf":
			csrfCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected alga_session cookie to be set")
	}
	if csrfCookie == nil {
		t.Fatal("expected alga_csrf cookie to be set")
	}

	if !auditStore.hasEvent(store.AuditUserCreated) {
		t.Fatal("expected AuditUserCreated audit event")
	}
	if !auditStore.hasEvent(store.AuditLoginSuccess) {
		t.Fatal("expected AuditLoginSuccess audit event")
	}

	count, _ := userStore.CountUsers()
	if count != 1 {
		t.Fatalf("expected 1 user after setup, got %d", count)
	}
}

func TestHandleSetup_AlreadyCompleted(t *testing.T) {
	userStore := newAuthMockUserStore()
	userStore.addUser("existing@alga.local", "P@ssw0rd!", "admin")
	auditStore := newRecordingAuditStore()
	sessionStore := newAuthMockSessionStore()
	_, mux := newAuthTestServer(userStore, sessionStore, auditStore, nil)

	body := bytes.NewBufferString(`{"email":"second@alga.local","password":"Str0ng!Pass"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSetup_MissingFields(t *testing.T) {
	userStore := newAuthMockUserStore()
	auditStore := newRecordingAuditStore()
	sessionStore := newAuthMockSessionStore()
	_, mux := newAuthTestServer(userStore, sessionStore, auditStore, nil)

	body := bytes.NewBufferString(`{"email":"missing-pass@alga.local"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSetup_WeakPassword(t *testing.T) {
	userStore := newAuthMockUserStore()
	auditStore := newRecordingAuditStore()
	sessionStore := newAuthMockSessionStore()
	_, mux := newAuthTestServer(userStore, sessionStore, auditStore, nil)

	body := bytes.NewBufferString(`{"email":"weak@alga.local","password":"weak"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for weak password, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSetupStatus_MethodNotAllowed(t *testing.T) {
	userStore := newAuthMockUserStore()
	auditStore := newRecordingAuditStore()
	sessionStore := newAuthMockSessionStore()
	_, mux := newAuthTestServer(userStore, sessionStore, auditStore, nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
