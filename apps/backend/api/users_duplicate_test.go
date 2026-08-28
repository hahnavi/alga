package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"alga/config"
	"alga/store"
)

// stubUserStore implements only what handleUsers/handleUserByID need for the
// duplicate-email paths; every other method panics via embedding of a nil
// interface — tests only exercise the create/update flows.
type stubUserStore struct {
	store.UserStore
	createErr error
	updateErr error
}

func (s *stubUserStore) CreateUser(email, password, role string) (*store.UserRecord, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &store.UserRecord{ID: uuid.New(), Email: email, Role: role}, nil
}

func (s *stubUserStore) UpdateUser(uuid.UUID, map[string]any) error { return s.updateErr }

func TestHandleUsersDuplicateEmailConflict(t *testing.T) {
	t.Parallel()

	dupErr := errors.New(`pq: duplicate key value violates unique constraint "users_email_key"`)

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		store      stubUserStore
		wantStatus int
	}{
		{
			name:       "create with existing email returns 409",
			method:     http.MethodPost,
			target:     "/api/v1/users",
			body:       `{"email":"dup@example.com","password":"Sup3rSecret!","role":"viewer"}`,
			store:      stubUserStore{createErr: dupErr},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "update to existing email returns 409",
			method:     http.MethodPut,
			target:     "/api/v1/users/" + uuid.NewString(),
			body:       `{"email":"taken@example.com"}`,
			store:      stubUserStore{updateErr: dupErr},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "non-duplicate create failure still returns 5xx",
			method:     http.MethodPost,
			target:     "/api/v1/users",
			body:       `{"email":"x@example.com","password":"Sup3rSecret!","role":"viewer"}`,
			store:      stubUserStore{createErr: errors.New("connection refused")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := &Server{
				cfg:         &config.Config{},
				userStore:   &tt.store,
				auditStore:  &recordingAuditStore{},
				ipExtractor: newIPExtractor(&config.Config{}),
			}

			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			if tt.method == http.MethodPut {
				s.handleUserByID(w, req)
			} else {
				s.handleUsers(w, req)
			}

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantStatus == http.StatusConflict {
				if got := w.Body.String(); !strings.Contains(got, "already exists") {
					t.Fatalf("body = %s, want conflict message", got)
				}
			}
		})
	}
}

// compile-time guard that the stub satisfies the interface
var _ store.UserStore = (*stubUserStore)(nil)
