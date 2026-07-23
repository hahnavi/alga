package telnyx

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func newSignedWebhookRequest(t *testing.T, priv ed25519.PrivateKey, body []byte, timestamp string) *http.Request {
	t.Helper()
	msg := append([]byte(timestamp), body...)
	sig := ed25519.Sign(priv, msg)
	req, err := http.NewRequest(http.MethodPost, "/api/v1/telnyx/callback", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Ed25519", base64.StdEncoding.EncodeToString(sig))
	req.Header.Set("Timestamp", timestamp)
	return req
}

func setupSignedClient(t *testing.T) (*Client, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	c := NewClient("k", "c", "+1", "", "", "")
	if err := c.SetPublicKey(base64.StdEncoding.EncodeToString(pub)); err != nil {
		t.Fatalf("SetPublicKey: %v", err)
	}
	return c, priv
}

func TestVerifyWebhook_Valid(t *testing.T) {
	t.Parallel()
	c, priv := setupSignedClient(t)

	body := []byte(`{"data":{"event_type":"call.answered"}}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req := newSignedWebhookRequest(t, priv, body, ts)

	returned, ok := c.VerifyWebhook(req)
	if !ok {
		t.Fatal("expected ok=true for valid signature")
	}
	if !bytes.Equal(returned, body) {
		t.Errorf("returned body = %q, want %q", returned, body)
	}

	restored, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("re-read body: %v", err)
	}
	if !bytes.Equal(restored, body) {
		t.Errorf("r.Body restorable but got %q, want %q", restored, body)
	}
}

func TestVerifyWebhook_TamperedBody(t *testing.T) {
	t.Parallel()
	c, priv := setupSignedClient(t)

	original := []byte(`{"data":{"event_type":"call.answered"}}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	msg := append([]byte(ts), original...)
	sig := ed25519.Sign(priv, msg)

	tampered := []byte(`{"data":{"event_type":"call.hangup"}}`)
	req, err := http.NewRequest(http.MethodPost, "/", bytes.NewReader(tampered))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Ed25519", base64.StdEncoding.EncodeToString(sig))
	req.Header.Set("Timestamp", ts)

	if _, ok := c.VerifyWebhook(req); ok {
		t.Fatal("expected ok=false for tampered body")
	}
}

func TestVerifyWebhook_BadSignature(t *testing.T) {
	t.Parallel()
	c, priv := setupSignedClient(t)

	body := []byte(`{"data":{"event_type":"call.answered"}}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req := newSignedWebhookRequest(t, priv, body, ts)
	req.Header.Set("Ed25519", base64.StdEncoding.EncodeToString([]byte("not-a-real-signature!!garbage!!")))

	if _, ok := c.VerifyWebhook(req); ok {
		t.Fatal("expected ok=false for garbage signature")
	}
}

func TestVerifyWebhook_MissingHeader(t *testing.T) {
	t.Parallel()
	c, _ := setupSignedClient(t)

	body := []byte(`{"data":{}}`)
	req, err := http.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	if _, ok := c.VerifyWebhook(req); ok {
		t.Fatal("expected ok=false when Ed25519 header missing")
	}
}

func TestVerifyWebhook_StaleTimestamp(t *testing.T) {
	t.Parallel()
	c, priv := setupSignedClient(t)

	body := []byte(`{"data":{"event_type":"call.answered"}}`)
	ts := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	req := newSignedWebhookRequest(t, priv, body, ts)

	if _, ok := c.VerifyWebhook(req); ok {
		t.Fatal("expected ok=false for timestamp older than 5 minutes")
	}
}

func TestVerifyWebhook_NoPublicKey(t *testing.T) {
	t.Parallel()
	c := NewClient("k", "c", "+1", "", "", "")

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	body := []byte(`{"data":{}}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	msg := append([]byte(ts), body...)
	req, err := http.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Ed25519", base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg)))
	req.Header.Set("Timestamp", ts)

	if _, ok := c.VerifyWebhook(req); ok {
		t.Fatal("expected ok=false when no public key configured")
	}
}

func TestSetPublicKey_Invalid(t *testing.T) {
	t.Parallel()
	c := NewClient("k", "c", "+1", "", "", "")

	if err := c.SetPublicKey("!!!not-base64!!!"); err == nil {
		t.Fatal("expected error for invalid base64")
	}
	if err := c.SetPublicKey(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("expected error for wrong length public key")
	}
	if err := c.SetPublicKey(""); err != nil {
		t.Fatalf("empty string should clear key without error, got %v", err)
	}
	if !c.Enabled() {
		t.Error("clearing public key should not affect Enabled()")
	}
}
