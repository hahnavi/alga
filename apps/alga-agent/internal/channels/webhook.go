package channels

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// webhookServer is a minimal HTTP server that receives Telegram webhook updates.
// It validates the secret path segment with a constant-time compare and forwards
// decoded updates to the channel.
//
// Concurrency: the *http.Server is constructed synchronously in newWebhookServer
// before any goroutine starts, so close() safely observes a fully-initialized
// server with no data race.
type webhookServer struct {
	addr     string
	path     string // includes leading "/" and secret token
	token    string
	srv      *http.Server
	onUpdate func(tg.Update)
}

// newWebhookServer constructs the server, registers its handler, and returns a
// fully-initialized webhookServer. Call listen() in a goroutine to accept
// connections; call close() to shut down.
func newWebhookServer(addr, path, token string, onUpdate func(tg.Update)) *webhookServer {
	w := &webhookServer{
		addr:     addr,
		path:     path,
		token:    token,
		onUpdate: onUpdate,
	}
	mux := http.NewServeMux()
	mux.HandleFunc(path, w.handleWebhook)
	w.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	return w
}

// handleWebhook is the HTTP handler for incoming Telegram webhook updates.
func (w *webhookServer) handleWebhook(resp http.ResponseWriter, req *http.Request) {
	// Constant-time compare on the full path (the mux already matched it, but
	// we double-check explicitly for defense in depth).
	got := req.URL.Path
	if subtle.ConstantTimeCompare([]byte(got), []byte(w.path)) != 1 {
		http.NotFound(resp, req)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		http.Error(resp, "bad request", http.StatusBadRequest)
		return
	}
	var upd tg.Update
	if err := json.Unmarshal(body, &upd); err != nil {
		http.Error(resp, "bad request", http.StatusBadRequest)
		return
	}
	w.onUpdate(upd)
	resp.WriteHeader(http.StatusOK)
}

// listen starts the HTTP server. Blocks until close() is called.
func (w *webhookServer) listen(logger *slog.Logger) {
	logger.Info("telegram webhook server listening", "addr", w.addr)
	if err := w.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("telegram webhook server error", "err", err)
	}
}

// close shuts down the webhook server with a 5s grace period.
func (w *webhookServer) close() error {
	if w.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.srv.Shutdown(ctx)
}
