package platform

import (
	"net/http"
	"strings"
	"time"

	algacrypto "alga/crypto"
	"alga/logger"
	"alga/rbac"
	"alga/store"
)

// RateLimiter is the per-key rate-limit interface the middleware depends on.
// It mirrors api.RateLimiting (which stays in package api to avoid an import
// cycle with app/wire.go); both the in-memory limiter and any future impl need
// only satisfy Allow/Stop.
type RateLimiter interface {
	Allow(key string) bool
	Stop()
}

// IPExtractor abstracts trusted-proxy-aware client-IP extraction. The concrete
// implementation lives in package api; the middleware only needs the ClientIP
// method.
type IPExtractor interface {
	ClientIP(r *http.Request) string
}

// AuthDeps bundles the dependencies the session/PAT AuthMiddleware closes over.
// The store fields use the canonical store package interfaces so any concrete
// implementation satisfies them without adapter boilerplate.
type AuthDeps struct {
	UserStore                store.UserStore
	SessionStore             store.SessionStore
	PersonalAccessTokenStore store.PersonalAccessTokenStore
	AuditStore               store.AuditStore
	IPExtractor              IPExtractor
}

// AgentAuthDeps bundles the dependencies AgentBearerMiddleware closes over.
type AgentAuthDeps struct {
	AgentTokenStore store.AgentTokenStore
}

// RateLimitDeps bundles the dependencies the per-IP RateLimitMiddleware closes
// over.
type RateLimitDeps struct {
	RateLimiter RateLimiter
	IPExtractor IPExtractor
}

// AgentRateLimitDeps bundles the dependencies the per-agent
// AgentRateLimitMiddleware closes over.
type AgentRateLimitDeps struct {
	RateLimiter RateLimiter
}

// AuthMiddleware validates the session cookie or a personal access token and,
// when perms are given, requires the authenticated user's role (intersected
// with the PAT's permissions when applicable) to grant at least one of them
// (OR semantics). It enforces CSRF for state-changing methods on the cookie
// path. On success the user is stored in the request context via WithUser.
//
// This is a pure move + parameterize of the legacy *Server.authMiddleware; the
// branching, CSRF check, audit calls, and error paths are preserved verbatim.
func AuthMiddleware(deps AuthDeps, next http.HandlerFunc, perms ...rbac.Permission) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer alga_pat_") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			patRecord, err := deps.PersonalAccessTokenStore.ValidateToken(token)
			if err != nil {
				WriteInternalError(w, err, "failed to validate personal access token")
				return
			}
			if patRecord == nil {
				WriteError(w, ErrorCodeUnauthorized, "invalid or expired personal access token")
				return
			}
			user, err := deps.UserStore.GetByID(patRecord.UserID)
			if err != nil {
				WriteInternalError(w, err, "failed to load user for personal access token")
				return
			}
			if user == nil {
				WriteError(w, ErrorCodeUnauthorized, "user not found")
				return
			}
			if user.LockedUntil != nil && user.LockedUntil.After(time.Now().UTC()) {
				WriteError(w, ErrorCodeForbidden, "account is locked")
				return
			}
			effectivePerms := intersectPATPermissions(user.Role, patRecord.Permissions)
			if len(perms) > 0 {
				if !rbac.HasAnyPermission(user.Role, perms...) {
					allowed := false
					for _, p := range perms {
						for _, ep := range effectivePerms {
							if string(p) == ep {
								allowed = true
								break
							}
						}
						if allowed {
							break
						}
					}
					if !allowed {
						WriteError(w, ErrorCodeForbidden, "insufficient permissions")
						return
					}
				}
			}
			ctx := WithUser(r.Context(), user)
			ctx = WithAuthMethod(ctx, "pat")
			// Inject the authenticated user id into the context so the logger's
			// contextHandler attaches user_id to every downstream log line (W8).
			ctx = logger.WithUser(ctx, user.ID.String())
			next(w, r.WithContext(ctx))
			return
		}

		if IsStateChangingMethod(r.Method) {
			if !validateCSRFToken(r) {
				WriteError(w, ErrorCodeForbidden, "invalid csrf token")
				return
			}
		}

		cookie, err := r.Cookie("alga_session")
		if err != nil {
			WriteError(w, ErrorCodeUnauthorized, "not authenticated")
			return
		}

		session, err := deps.SessionStore.GetSession(cookie.Value)
		if err != nil || session == nil {
			WriteError(w, ErrorCodeUnauthorized, "invalid or expired session")
			return
		}

		user, err := deps.UserStore.GetByID(session.UserID)
		if err != nil || user == nil {
			WriteError(w, ErrorCodeUnauthorized, "user not found")
			return
		}

		if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
			deps.AuditStore.Log(store.AuditLoginFailed, &user.ID, user.Email, deps.IPExtractor.ClientIP(r), r.UserAgent(), false, map[string]any{
				"reason":       "account_locked",
				"locked_until": user.LockedUntil,
			})
			WriteError(w, ErrorCodeForbidden, "account is locked due to too many failed login attempts")
			return
		}

		if len(perms) > 0 && !rbac.HasAnyPermission(user.Role, perms...) {
			deps.AuditStore.Log(store.AuditLoginFailed, &user.ID, user.Email, deps.IPExtractor.ClientIP(r), r.UserAgent(), false, map[string]any{
				"reason":               "insufficient_permissions",
				"required_permissions": perms,
			})
			WriteError(w, ErrorCodeForbidden, "insufficient permissions")
			return
		}

		ctx := WithUser(r.Context(), user)
		// Inject the authenticated user id into the context so the logger's
		// contextHandler attaches user_id to every downstream log line (W8).
		ctx = logger.WithUser(ctx, user.ID.String())
		next(w, r.WithContext(ctx))
	}
}

// AgentBearerMiddleware validates a "Bearer <agent-token>" Authorization
// header and, on success, stores the agent identity in the request context via
// WithAgent. It does not do rate limiting; compose AgentRateLimitMiddleware
// around it at registration time.
func AgentBearerMiddleware(deps AgentAuthDeps, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(raw, prefix) {
			WriteError(w, ErrorCodeUnauthorized, "missing bearer token")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(raw, prefix))
		if token == "" {
			WriteError(w, ErrorCodeUnauthorized, "missing bearer token")
			return
		}
		rec, err := deps.AgentTokenStore.ValidateToken(token)
		if err != nil {
			WriteInternalError(w, err, "failed to validate token")
			return
		}
		if rec == nil {
			WriteError(w, ErrorCodeUnauthorized, "invalid or expired token")
			return
		}
		ctx := WithAgent(r.Context(), &AgentTokenContext{
			ID:           rec.ID,
			Name:         rec.Name,
			AgentType:    string(store.NormalizeAgentType(rec.AgentType)),
			Capabilities: rec.Capabilities,
		})
		next(w, r.WithContext(ctx))
	}
}

// RateLimitMiddleware wraps next with per-IP rate limiting.
func RateLimitMiddleware(deps RateLimitDeps, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := deps.IPExtractor.ClientIP(r)
		if !deps.RateLimiter.Allow(ip) {
			WriteRateLimitExceeded(w, "60")
			return
		}
		next(w, r)
	}
}

// AgentRateLimitMiddleware wraps next with per-agent-token rate limiting. It
// must be placed AFTER AgentBearerMiddleware so the agent context is set. If
// the limiter is nil it is a pass-through.
func AgentRateLimitMiddleware(deps AgentRateLimitDeps, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.RateLimiter == nil {
			next(w, r)
			return
		}
		act := AgentFromContext(r.Context())
		if act == nil {
			WriteError(w, ErrorCodeUnauthorized, "unauthorized")
			return
		}
		if !deps.RateLimiter.Allow(act.ID.String()) {
			WriteRateLimitExceeded(w, "60")
			return
		}
		next(w, r)
	}
}

// CheckPermission checks whether the authenticated user (read from the request
// context) has at least one of the given permissions. Returns true on success;
// writes a 401/403 and returns false otherwise. Useful for mixed-method
// handlers where different HTTP methods need different permissions.
func CheckPermission(w http.ResponseWriter, r *http.Request, perms ...rbac.Permission) bool {
	user := UserFromContext(r.Context())
	if user == nil {
		WriteError(w, ErrorCodeUnauthorized, "unauthorized")
		return false
	}
	if !rbac.HasAnyPermission(user.Role, perms...) {
		WriteError(w, ErrorCodeForbidden, "insufficient permissions")
		return false
	}
	return true
}

// ValidateCSRFToken validates the CSRF token from the request: present in the
// X-CSRF-Token header and matching the alga_csrf cookie, compared in constant
// time. Non-state-changing methods are always valid.
func ValidateCSRFToken(r *http.Request) bool {
	if !IsStateChangingMethod(r.Method) {
		return true
	}

	headerToken := r.Header.Get("X-CSRF-Token")
	if headerToken == "" {
		return false
	}

	cookie, err := r.Cookie("alga_csrf")
	if err != nil {
		return false
	}

	return algacrypto.ConstantTimeEqualString(headerToken, cookie.Value)
}

// validateCSRFToken is the unexported alias used internally by AuthMiddleware.
func validateCSRFToken(r *http.Request) bool { return ValidateCSRFToken(r) }

// IntersectPATPermissions returns the PAT-granted permissions that are also
// granted by the user's role. Used by AuthMiddleware on the PAT path.
func IntersectPATPermissions(role string, patPerms []string) []string {
	rolePerms := rbac.AllPermissions(role)
	roleSet := make(map[string]bool, len(rolePerms))
	for _, p := range rolePerms {
		roleSet[string(p)] = true
	}
	var result []string
	for _, p := range patPerms {
		if roleSet[p] {
			result = append(result, p)
		}
	}
	return result
}

// intersectPATPermissions is the unexported alias used internally by
// AuthMiddleware.
func intersectPATPermissions(role string, patPerms []string) []string {
	return IntersectPATPermissions(role, patPerms)
}
