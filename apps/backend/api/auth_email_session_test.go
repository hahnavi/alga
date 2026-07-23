package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alga/store"
)

// TestChangeEmailInvalidatesSessions verifies M2: after a successful email
// change, all of the user's sessions are invalidated (ASVS V3.6/V2.5),
// mirroring the existing password-change behavior.
func TestChangeEmailInvalidatesSessions(t *testing.T) {
	userStore := newAuthMockUserStore()
	testUser := userStore.addUser("old@alga.local", "correctP@ss1!", "admin")

	sessionStore := newAuthMockSessionStore()
	sessionStore.sessions["active-session"] = &store.SessionRecord{
		ID:        "active-session",
		UserID:    testUser.ID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	// A second session for the same user (different device).
	sessionStore.sessions["other-session"] = &store.SessionRecord{
		ID:        "other-session",
		UserID:    testUser.ID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	_, mux := newAuthTestServer(userStore, sessionStore, nil, nil)

	body := bytes.NewBufferString(`{"password":"correctP@ss1!","email":"new@alga.local"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-email", body)
	req.AddCookie(&http.Cookie{Name: "alga_session", Value: "active-session"})
	req.AddCookie(&http.Cookie{Name: "alga_csrf", Value: "test-csrf-token"})
	req.Header.Set("X-CSRF-Token", "test-csrf-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Both sessions must have been invalidated by DeleteAllUserSessions.
	if _, ok := sessionStore.sessions["active-session"]; ok {
		t.Fatal("active-session must be invalidated after email change")
	}
	if _, ok := sessionStore.sessions["other-session"]; ok {
		t.Fatal("other-session must be invalidated after email change")
	}
}
