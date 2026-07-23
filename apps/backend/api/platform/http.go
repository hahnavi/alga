package platform

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"alga/logger"
)

// MaxRequestBodySize is the cap on inbound JSON request bodies enforced by
// DecodeJSON via http.MaxBytesReader.
const MaxRequestBodySize = 1 << 20 // 1 MiB

// MaxQueryLimit caps the `limit` query parameter returned by ParseLimitSkip.
const MaxQueryLimit = 500

// DecodeJSON decodes the request body into target capped at MaxRequestBodySize.
// On decode failure it writes a 400 and returns false; on success it returns
// true. Callers must not read r.Body directly.
func DecodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		WriteErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid request body")
		return false
	}
	return true
}

// ParseLimitSkip reads the `limit` and `skip` query parameters, clamping limit
// to MaxQueryLimit and defaulting skip to 0. `limit` defaults to defaultLimit
// when absent/invalid.
func ParseLimitSkip(r *http.Request, defaultLimit int) (limit, skip int64) {
	limit = int64(defaultLimit)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			limit = n
		}
	}
	limit = min(limit, MaxQueryLimit)
	if v := r.URL.Query().Get("skip"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			skip = n
		}
	}
	return
}

// ErrorCode is a stable, machine-readable error identifier emitted in the
// error envelope. The set is intentionally small and fixed so that API
// consumers (frontend, agent SDKs) can branch on it without parsing messages.
type ErrorCode string

// The stable error-code set. Every error response carries exactly one of these
// in `error.code`.
const (
	ErrorCodeValidationFailed ErrorCode = "validation_failed"
	ErrorCodeUnauthorized     ErrorCode = "unauthorized"
	ErrorCodeForbidden        ErrorCode = "forbidden"
	ErrorCodeNotFound         ErrorCode = "not_found"
	ErrorCodeConflict         ErrorCode = "conflict"
	ErrorCodeRateLimited      ErrorCode = "rate_limited"
	ErrorCodeInternal         ErrorCode = "internal"
)

// ErrorDetail carries field-level validation context for an error. It is empty
// for non-validation errors.
type ErrorDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// errorBody is the payload of the error envelope.
type errorBody struct {
	Code    ErrorCode     `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details"`
}

// errorEnvelope is the top-level error response shape:
// {"error":{"code":...,"message":...,"details":[...]}}.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// HTTPStatus returns the canonical HTTP status for an error code.
func (c ErrorCode) HTTPStatus() int {
	switch c {
	case ErrorCodeValidationFailed:
		return http.StatusUnprocessableEntity
	case ErrorCodeUnauthorized:
		return http.StatusUnauthorized
	case ErrorCodeForbidden:
		return http.StatusForbidden
	case ErrorCodeNotFound:
		return http.StatusNotFound
	case ErrorCodeConflict:
		return http.StatusConflict
	case ErrorCodeRateLimited:
		return http.StatusTooManyRequests
	case ErrorCodeInternal:
		return http.StatusInternalServerError
	}
	return http.StatusInternalServerError
}

// WriteError writes the standard error envelope with the canonical HTTP status
// for code. Use WriteErrorStatus to override the status (e.g. 503, 405).
func WriteError(w http.ResponseWriter, code ErrorCode, message string, details ...ErrorDetail) {
	WriteErrorStatus(w, code.HTTPStatus(), code, message, details...)
}

// WriteErrorStatus writes the standard error envelope with an explicit HTTP
// status. Prefer WriteError unless the desired status differs from the code's
// canonical status.
func WriteErrorStatus(w http.ResponseWriter, status int, code ErrorCode, message string, details ...ErrorDetail) {
	if details == nil {
		details = []ErrorDetail{}
	}
	WriteJSON(w, status, errorEnvelope{Error: errorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}

// WriteInternalError logs the underlying error and returns a generic 500 to
// the client, avoiding leakage of internal details.
func WriteInternalError(w http.ResponseWriter, err error, message string) {
	logger.Error("internal error", "error", err)
	WriteError(w, ErrorCodeInternal, message)
}

// WriteData wraps a single-resource success body in the {"data": ...} envelope.
func WriteData(w http.ResponseWriter, status int, payload any) {
	WriteJSON(w, status, map[string]any{"data": payload})
}

// WritePaginatedJSON writes a 200 with the standard list envelope
// {"data":{"items":..., "total":...}, "meta":{"total":...}} used by all list
// endpoints.
func WritePaginatedJSON(w http.ResponseWriter, items any, total int64) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items": items,
			"total": total,
		},
		"meta": map[string]any{
			"total": total,
		},
	})
}

// WriteRawJSON writes pre-marshaled JSON bytes directly to the response without
// re-encoding.
func WriteRawJSON(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// WriteJSON marshals payload as JSON, sets the Content-Type, writes the status
// code, and logs encode failures via structured logging. It is the low-level
// writer used by the envelope helpers and should not be used directly for
// success/error responses that belong in the {data}/{error} envelopes.
func WriteJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Error("JSON encode error", "error", err)
	}
}

// WriteStatus writes a 200 with a {"status": status} JSON body. Use for simple
// ok/status acknowledgements instead of repeating the map literal at every call
// site.
func WriteStatus(w http.ResponseWriter, status string) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": status})
}

// WriteConflict writes a 409 Conflict response. Use for concurrent-modification
// / invariant-conflict failures instead of repeating the status literal.
func WriteConflict(w http.ResponseWriter, message string) {
	WriteError(w, ErrorCodeConflict, message)
}

// WriteRateLimitExceeded writes a 429 Too Many Requests response with the
// Retry-After and X-RateLimit-Remaining headers used by the rate-limit
// middleware.
func WriteRateLimitExceeded(w http.ResponseWriter, retryAfter string) {
	w.Header().Set("Retry-After", retryAfter)
	w.Header().Set("X-RateLimit-Remaining", "0")
	WriteError(w, ErrorCodeRateLimited, "Rate limit exceeded. Please try again later.")
}

// EnsureSlice returns a non-nil copy of s: nil becomes an empty slice so JSON
// encodes `[]` instead of `null`.
func EnsureSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// PathID returns the path suffix after prefix, used for /collection/{id}
// dispatch on mux patterns without {id} capture.
func PathID(r *http.Request, prefix string) string {
	return strings.TrimPrefix(r.URL.Path, prefix)
}

// ParseTimeQuery parses an RFC3339 time query value. It returns the time and
// true on success, or the zero time and false when raw is empty or invalid.
func ParseTimeQuery(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	return t, err == nil
}

// IsStateChangingMethod reports whether method modifies state (POST/PUT/PATCH/
// DELETE). Used to decide whether to require CSRF validation.
func IsStateChangingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
