package alga

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// AlgaAuthError is returned for 401/403 responses. Auth errors are never
// retryable — the token must be valid (not revoked, not expired) before
// retrying.
type AlgaAuthError struct {
	StatusCode int
	Message    string
}

func (e *AlgaAuthError) Error() string {
	return fmt.Sprintf("auth error %d: %s", e.StatusCode, e.Message)
}

// AlgaAPIError is returned for any non-2xx, non-auth response. RetryAfter is
// populated from the `Retry-After` header when present (notably for 429s).
type AlgaAPIError struct {
	StatusCode int
	Message    string
	RetryAfter time.Duration
}

func (e *AlgaAPIError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("api error %d: %s (retry after %s)", e.StatusCode, e.Message, e.RetryAfter)
	}
	return fmt.Sprintf("api error %d: %s", e.StatusCode, e.Message)
}

// IsRetryable reports whether the error is worth retrying per the HTTP
// status code (429, 500, 502, 503, 504). 4xx errors other than 429 are
// considered permanent.
func (e *AlgaAPIError) IsRetryable() bool {
	switch e.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// AlgaConnectionError wraps a transport-level error (DNS, TCP, TLS, timeout).
// The underlying error is retained via Unwrap so callers can use errors.As
// to detect net.Error timeouts, etc.
type AlgaConnectionError struct {
	Err error
}

func (e *AlgaConnectionError) Error() string {
	if e.Err == nil {
		return "connection error"
	}
	return fmt.Sprintf("connection error: %v", e.Err)
}

func (e *AlgaConnectionError) Unwrap() error { return e.Err }

// IsRetryable reports whether a connection error is worth retrying. Timeouts
// and transient network failures are retryable; everything else is treated as
// retryable too because the underlying TCP/TLS layer rarely surfaces
// permanent conditions through net.Error.
func (e *AlgaConnectionError) IsRetryable() bool {
	if e.Err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(e.Err, &netErr) {
		return netErr.Timeout()
	}
	return true
}

// IsRetryableError reports whether err is a retryable SDK error (an
// AlgaAPIError or AlgaConnectionError whose IsRetryable method returns true).
// Non-SDK errors are reported as non-retryable so callers don't accidentally
// retry a programming bug.
func IsRetryableError(err error) bool {
	var apiErr *AlgaAPIError
	if errors.As(err, &apiErr) {
		return apiErr.IsRetryable()
	}
	var connErr *AlgaConnectionError
	if errors.As(err, &connErr) {
		return connErr.IsRetryable()
	}
	return false
}
