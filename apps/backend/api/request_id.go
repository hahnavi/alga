package api

import (
	"net/http"
	"regexp"

	"github.com/google/uuid"

	"alga/logger"
)

// requestIDGrammar accepts opaque client-generated correlation IDs up to 128
// chars. Anything else (too long, control characters, path separators, ANSI
// escapes) is discarded and replaced with a server-generated UUID so forged
// values cannot smuggle content into logs or downstream SIEM views.
var requestIDGrammar = regexp.MustCompile(`^[A-Za-z0-9._\-]+$`)

const maxRequestIDLen = 128

func SanitizeRequestID(raw string) string {
	if raw != "" && len(raw) <= maxRequestIDLen && requestIDGrammar.MatchString(raw) {
		return raw
	}
	return uuid.New().String()
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := SanitizeRequestID(r.Header.Get("X-Request-ID"))
		w.Header().Set("X-Request-ID", id)
		ctx := logger.WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
