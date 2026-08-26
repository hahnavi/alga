package store

import "errors"

var (
	ErrNotFound                    = errors.New("not found")
	ErrAlertNotFound               = errors.New("alert not found")
	ErrAlertNotFiring              = errors.New("alert not found or not firing")
	ErrAlertNotResolved            = errors.New("alert not found or not resolved")
	ErrInvestigationNotFound       = errors.New("investigation not found")
	ErrAlertInvestigationNotFound  = errors.New("alert investigation not found")
	ErrTokenNotFound               = errors.New("token not found")
	ErrUserNotFound                = errors.New("user not found")
	ErrNotificationNotFound        = errors.New("notification not found")
	ErrAgentNotFoundInactive       = errors.New("agent not found or inactive")
	ErrAgentCapabilityMismatch     = errors.New("agent does not have required capability")
	ErrServiceNotFound             = errors.New("service not found")
	ErrInvalidAgentType            = errors.New("invalid agent_type")
	ErrIncidentNotFound            = errors.New("incident not found")
	ErrHeartbeatNotFound           = errors.New("heartbeat not found")
	ErrSharedSecretNotFound        = errors.New("shared secret not found")
	ErrStatusPageNotFound          = errors.New("status page not found")
	ErrStatusPageComponentNotFound = errors.New("status page component not found")
	ErrICSRoleNotFound             = errors.New("ICS role assignment not found")
	ErrCredentialProviderNotFound  = errors.New("credential provider not found")
	// ErrCredentialProviderInUse maps the FK RESTRICT violation when a
	// provider still owns shared secrets (WP-B6): handlers translate it to a
	// 409 instead of a raw 500.
	ErrCredentialProviderInUse  = errors.New("credential provider has dependent secrets and cannot be removed")
	ErrSystemCredentialProvider = errors.New("credential provider is a system default and cannot be removed or reconfigured")
	ErrOpenAlertExists          = errors.New("an open alert already exists for this fingerprint")
)
