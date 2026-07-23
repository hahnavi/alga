// Package platform holds the cross-cutting HTTP helpers and auth/rate-limit
// middleware that every api/* sub-package depends on. It is the shared layer
// extracted from the god package `api` as part of the Prompt D decomposition
// (see docs/refactor/api-decomposition-design.md).
//
// Everything here is exported so that domain sub-packages (api/agent,
// api/incident, ...) can import it. The legacy `package api` keeps thin
// wrapper funcs/aliases that delegate here during the transition; those
// wrappers are deleted as each domain migrates.
package platform

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"alga/store"
)

// contextKey is the unexported key type for request-scoped auth values stored
// in context.Context. It is deliberately unexported so that only this package
// can mint the canonical keys; callers set/read values via WithUser/UserFromContext
// and WithAgent/AgentFromContext.
type contextKey string

const (
	userContextKey contextKey = "user"
)

type agentCtxKeyType struct{}

var agentCtxKey = agentCtxKeyType{}

type authMethodKeyT string

const authMethodKey authMethodKeyT = "auth_method"

// AgentTokenContext is the agent identity propagated through request context
// by AgentBearerMiddleware. It mirrors the agent's token record without the
// secret material.
type AgentTokenContext struct {
	ID           uuid.UUID
	Name         string
	AgentType    string
	Capabilities []string
}

// WithUser stores the authenticated user in the context and returns the new
// context. It is the canonical setter used by the auth middleware.
func WithUser(ctx context.Context, user *store.UserRecord) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext retrieves the authenticated user from the request context,
// or nil if no user is set.
func UserFromContext(ctx context.Context) *store.UserRecord {
	u, _ := ctx.Value(userContextKey).(*store.UserRecord)
	return u
}

// WithAgent stores the agent identity in the context and returns the new
// context. It is the canonical setter used by AgentBearerMiddleware.
func WithAgent(ctx context.Context, agent *AgentTokenContext) context.Context {
	return context.WithValue(ctx, agentCtxKey, agent)
}

// AgentFromContext retrieves the agent identity from the request context, or
// nil if no agent is set.
func AgentFromContext(ctx context.Context) *AgentTokenContext {
	v, _ := ctx.Value(agentCtxKey).(*AgentTokenContext)
	return v
}

// WithAuthMethod stores the authentication method ("pat", "session", ...) in
// the context. Used by the session/PAT auth middleware.
func WithAuthMethod(ctx context.Context, method string) context.Context {
	return context.WithValue(ctx, authMethodKey, method)
}

// AuthMethodFromContext retrieves the authentication method from the request
// context, or the empty string if none is set.
func AuthMethodFromContext(ctx context.Context) string {
	m, _ := ctx.Value(authMethodKey).(string)
	return m
}

// RequireAgent retrieves the agent from the request context or writes a 401
// and returns ok=false. Convenience helper for agent handlers.
func RequireAgent(w http.ResponseWriter, r *http.Request) (*AgentTokenContext, bool) {
	agent := AgentFromContext(r.Context())
	if agent == nil {
		WriteError(w, ErrorCodeUnauthorized, "missing agent context")
		return nil, false
	}
	return agent, true
}
