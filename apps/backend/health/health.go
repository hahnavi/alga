package health

import (
	"context"
	"net/http"
	"sync"
	"time"

	"alga/api/platform"
	"alga/logger"
	"alga/pgclient"
	"alga/rabbitmq"
	"alga/valkey"
)

// Check probes a single dependency. It must be fast, non-blocking, and MUST
// NOT surface secrets, connection strings, or internal state — only a plain
// error is returned to the caller (and is never written into the probe body).
type Check func(ctx context.Context) error

// Result is the per-dependency readiness state. It intentionally contains only
// a status name and whether the dependency was actually probed; no diagnostic
// detail that could leak credentials or topology is ever exposed.
type Result struct {
	Status  string `json:"status"`  // "ok" | "degraded"
	Checked bool   `json:"checked"` // false when the dependency is not configured
}

// Response is the JSON body returned by /ready and /health probes.
type Response struct {
	Status       string            `json:"status"` // "ok" | "degraded"
	Dependencies map[string]Result `json:"dependencies,omitempty"`
}

// Handler serves liveness and readiness probes over a small net/http mux.
// It is shared by the API server and the worker side-port health server so
// both expose the same probe contract.
type Handler struct {
	timeout time.Duration
	mu      sync.RWMutex
	checks  map[string]Check
}

// NewHandler builds a probe handler. checks maps a dependency name (e.g.
// "postgres", "rabbitmq", "valkey") to its probe; a nil probe is reported as
// "checked": false and never fails readiness (the dependency is optional).
func NewHandler(timeout time.Duration, checks map[string]Check) *Handler {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Handler{timeout: timeout, checks: checks}
}

// SetChecks replaces the dependency probes (used at wiring time before serving).
func (h *Handler) SetChecks(checks map[string]Check) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks = checks
}

// Live is the liveness handler: 200 once the process is up, no dependency checks.
func (h *Handler) Live(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		platform.WriteJSON(w, http.StatusMethodNotAllowed, Response{Status: "method_not_allowed"})
		return
	}
	platform.WriteJSON(w, http.StatusOK, Response{Status: "ok"})
}

// Ready is the readiness handler: probes every configured dependency and
// returns 200 when all are healthy, or 503 with a status-only dependency map
// otherwise. Errors are logged by name only (never with secrets) and are never
// written into the response body.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		platform.WriteJSON(w, http.StatusMethodNotAllowed, Response{Status: "method_not_allowed"})
		return
	}

	h.mu.RLock()
	checks := h.checks
	h.mu.RUnlock()

	deps := make(map[string]Result, len(checks))
	allOK := true
	for name, check := range checks {
		if check == nil {
			deps[name] = Result{Status: "ok", Checked: false}
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		err := check(ctx)
		cancel()
		if err != nil {
			allOK = false
			deps[name] = Result{Status: "degraded", Checked: true}
			logger.Warn("health dependency degraded", "dependency", name)
			continue
		}
		deps[name] = Result{Status: "ok", Checked: true}
	}

	status := "ok"
	code := http.StatusOK
	if !allOK {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	platform.WriteJSON(w, code, Response{Status: status, Dependencies: deps})
}

// Mux returns a ServeMux with /live, /ready, and /health registered. /health
// is an alias of /ready, matching the Gresto probe contract.
func (h *Handler) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/live", h.Live)
	mux.HandleFunc("/ready", h.Ready)
	mux.HandleFunc("/health", h.Ready)
	return mux
}

// CheckPostgres returns a probe for a PostgreSQL client, or nil when the client
// is absent (dependency not configured).
func CheckPostgres(cli *pgclient.Client) Check {
	if cli == nil || cli.DB == nil {
		return nil
	}
	return func(ctx context.Context) error {
		return cli.DB.PingContext(ctx)
	}
}

// CheckRabbitMQ returns a probe that opens and closes an AMQP channel, or nil
// when the client is absent. It does not publish or consume.
func CheckRabbitMQ(cli *rabbitmq.Client) Check {
	if cli == nil {
		return nil
	}
	return func(ctx context.Context) error {
		ch, err := cli.Channel()
		if err != nil {
			return err
		}
		return ch.Close()
	}
}

// CheckValkey returns a probe for a Valkey client, or nil when the client is
// absent (dependency not configured).
func CheckValkey(cli *valkey.Client) Check {
	if cli == nil {
		return nil
	}
	return func(ctx context.Context) error {
		return cli.Ping(ctx)
	}
}
