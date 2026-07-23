package telnyx

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"time"

	"alga/logger"
)

func (c *Client) VerifyWebhook(r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, false
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	c.mu.RLock()
	pubKey := c.publicKey
	c.mu.RUnlock()
	if len(pubKey) == 0 {
		logger.Warn("telnyx public key not configured", "component", "telnyx")
		return nil, false
	}

	sigHeader := r.Header.Get("Ed25519")
	timestampHeader := r.Header.Get("Timestamp")
	if sigHeader == "" || timestampHeader == "" {
		return nil, false
	}

	sig, err := base64.StdEncoding.DecodeString(sigHeader)
	if err != nil {
		return nil, false
	}

	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return nil, false
	}
	if absDiff(time.Now().Unix(), ts) > int64(5*time.Minute/time.Second) {
		return nil, false
	}

	message := append([]byte(timestampHeader), body...)
	if !ed25519.Verify(pubKey, message, sig) {
		return nil, false
	}
	return body, true
}

func absDiff(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}
