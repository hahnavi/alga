package telnyx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"alga/internal/httpclient"
	"alga/logger"
	"alga/notification"
)

const defaultBaseURL = "https://api.telnyx.com"

// Defaults applied when a TTS field is left empty. The current Telnyx TTS API
// encodes the provider in the voice prefix (e.g. "Polly.Brian",
// "ElevenLabs.eleven_flash_v2_5.<id>") and exposes no separate "service" field.
// "Telnyx.KokoroTTS.af_something" is a safe, free-tier default that doesn't
// require any BYOK integration secret.
const (
	defaultTTSVoice    = "Telnyx.KokoroTTS.af_something"
	defaultTTSLanguage = "en-US"
	elevenLabsPrefix   = "ElevenLabs."
)

type Client struct {
	mu              sync.RWMutex
	apiKey          string
	connectionID    string
	fromNumber      string
	ttsVoice        string
	ttsLanguage     string
	ttsAPIKeyRef    string
	publicKey       ed25519.PublicKey
	httpClient      *http.Client
	baseURL         string
	callbackBaseURL string
	disabled        bool
}

func NewClient(apiKey, connectionID, fromNumber, ttsVoice, ttsLanguage, ttsAPIKeyRef string) *Client {
	c := &Client{
		apiKey:       apiKey,
		connectionID: connectionID,
		fromNumber:   fromNumber,
		httpClient:   httpclient.NewTimeoutClient(15 * time.Second),
		baseURL:      defaultBaseURL,
	}
	c.applyTTS(ttsVoice, ttsLanguage, ttsAPIKeyRef)
	return c
}

// SetTTS reconfigures the TTS voice/language/api_key_ref used by GatherUsingSpeak
// and Speak. Empty values fall back to the built-in defaults.
func (c *Client) SetTTS(voice, language, apiKeyRef string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.applyTTS(voice, language, apiKeyRef)
}

func (c *Client) applyTTS(voice, language, apiKeyRef string) {
	c.ttsVoice = defaultTTSVoice
	if voice != "" {
		c.ttsVoice = voice
	}
	c.ttsLanguage = defaultTTSLanguage
	if language != "" {
		c.ttsLanguage = language
	}
	// apiKeyRef has no safe default — it's only required for ElevenLabs BYOK
	// and is the identifier of a Telnyx integration secret the operator
	// registers out-of-band. Empty means "no ref sent".
	c.ttsAPIKeyRef = apiKeyRef
}

func (c *Client) TTSVoice() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ttsVoice
}
func (c *Client) TTSLanguage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ttsLanguage
}
func (c *Client) TTSAPIKeyRef() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ttsAPIKeyRef
}

// ttsPayload builds the JSON body subset shared by speak and gather_using_speak.
// The provider is encoded in the voice prefix per the current Telnyx TTS API;
// there is no "service" field. When the voice is an ElevenLabs voice, Telnyx
// requires voice_settings.api_key_ref pointing at a registered integration
// secret holding the ElevenLabs API key.
func (c *Client) ttsPayload(text string) map[string]any {
	c.mu.RLock()
	voice := c.ttsVoice
	language := c.ttsLanguage
	apiKeyRef := c.ttsAPIKeyRef
	c.mu.RUnlock()

	body := map[string]any{
		"payload": text,
		"voice":   voice,
	}
	if language != "" {
		body["language"] = language
	}
	if strings.HasPrefix(voice, elevenLabsPrefix) && apiKeyRef != "" {
		body["voice_settings"] = map[string]any{
			"api_key_ref": apiKeyRef,
		}
	}
	return body
}

func (c *Client) SetCallbackBaseURL(base string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbackBaseURL = strings.TrimRight(base, "/")
}

func (c *Client) SetDisabled(disabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disabled = disabled
}

func (c *Client) SetPublicKey(pubKeyBase64 string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pubKeyBase64 == "" {
		c.publicKey = nil
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(pubKeyBase64)
	if err != nil {
		return fmt.Errorf("telnyx: invalid public key base64: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return fmt.Errorf("telnyx: invalid public key length %d (expected %d)", len(raw), ed25519.PublicKeySize)
	}
	c.publicKey = ed25519.PublicKey(raw)
	return nil
}

func (c *Client) Reconfigure(apiKey, connectionID, fromNumber, ttsVoice, ttsLanguage, ttsAPIKeyRef string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiKey = apiKey
	c.connectionID = connectionID
	c.fromNumber = fromNumber
	c.applyTTS(ttsVoice, ttsLanguage, ttsAPIKeyRef)
}

func (c *Client) ReconfigurePublicKey(pubKeyBase64 string) error {
	return c.SetPublicKey(pubKeyBase64)
}

func (c *Client) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.disabled && c.apiKey != "" && c.connectionID != "" && c.fromNumber != ""
}

func (c *Client) ProviderName() string {
	return "telnyx"
}

func (c *Client) callbackURL(incidentNumber int64, level int, userID, base string) string {
	path := "/api/v1/telnyx/callback?incident=" + strconv.FormatInt(incidentNumber, 10) + "&level=" + strconv.Itoa(level)
	if userID != "" {
		path += "&user=" + userID
	}
	if base == "" {
		return path
	}
	return base + path
}

func (c *Client) Call(ctx context.Context, to string, incidentNumber int64, level int, opts notification.CallOptions) (string, error) {
	if to == "" {
		return "", errors.New("telnyx: empty recipient number")
	}

	// Snapshot mutable config under the read lock so concurrent Reconfigure/
	// SetCallbackBaseURL/SetDisabled cannot tear a single call.
	c.mu.RLock()
	connectionID := c.connectionID
	fromNumber := c.fromNumber
	callbackBase := c.callbackBaseURL
	enabled := !c.disabled && c.apiKey != "" && connectionID != "" && fromNumber != ""
	c.mu.RUnlock()

	if !enabled {
		return "", errors.New("telnyx not configured")
	}

	userID := ""
	if opts.UserID != nil {
		userID = opts.UserID.String()
	}
	payload := map[string]any{
		"connection_id":       connectionID,
		"to":                  to,
		"from":                fromNumber,
		"timeout_secs":        30,
		"webhook_url":         c.callbackURL(incidentNumber, level, userID, callbackBase),
		"webhook_api_version": "2",
	}

	logger.Info("initiating Telnyx voice call", "component", "telnyx", "to", to, "incident_number", incidentNumber)

	body, statusCode, err := c.doJSON(ctx, http.MethodPost, "/v2/calls", payload)
	if err != nil {
		logger.Warn("telnyx call request failed", "component", "telnyx", "to", to, "incident_number", incidentNumber, "error", err)
		return "", fmt.Errorf("telnyx call request failed: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		logger.Warn("telnyx API returned non-success status", "component", "telnyx", "to", to, "incident_number", incidentNumber, "status", statusCode, "body", string(body))
		return "", fmt.Errorf("telnyx API returned status %d: %s", statusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Data struct {
			CallControlID string `json:"call_control_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode telnyx response: %w", err)
	}
	return result.Data.CallControlID, nil
}

func (c *Client) Answer(ctx context.Context, callControlID string) error {
	return c.callAction(ctx, callControlID, "answer", nil)
}

// GatherUsingSpeak plays text-to-speech while collecting a single DTMF digit.
// clientState is an opaque base64 token Telnyx echoes back in the
// call.gather.ended webhook, letting us carry re-prompt attempt state without
// server-side call tracking. Pass "" to omit it.
func (c *Client) GatherUsingSpeak(ctx context.Context, callControlID, text, clientState string) error {
	body := c.ttsPayload(text)
	body["valid_digits"] = "12"
	body["max_digits"] = 1
	body["timeout_millis"] = 5000
	if clientState != "" {
		body["client_state"] = clientState
	}
	return c.callAction(ctx, callControlID, "gather_using_speak", body)
}

func (c *Client) Speak(ctx context.Context, callControlID, text string) error {
	return c.callAction(ctx, callControlID, "speak", c.ttsPayload(text))
}

func (c *Client) Hangup(ctx context.Context, callControlID string) error {
	return c.callAction(ctx, callControlID, "hangup", nil)
}

func (c *Client) callAction(ctx context.Context, callControlID, action string, body any) error {
	path := "/v2/calls/" + callControlID + "/actions/" + action
	_, statusCode, err := c.doJSON(ctx, http.MethodPost, path, body)
	if err != nil {
		return fmt.Errorf("telnyx %s request failed: %w", action, err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("telnyx %s: status %d", action, statusCode)
	}
	return nil
}

// doJSON snapshots mutable config (api key, base URL, http client) under the
// read lock before issuing the request so concurrent Reconfigure calls cannot
// race with an in-flight HTTP transaction.
func (c *Client) doJSON(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	c.mu.RLock()
	apiKey := c.apiKey
	baseURL := c.baseURL
	httpClient := c.httpClient
	c.mu.RUnlock()

	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("telnyx: failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(raw)
	}

	reqURL := baseURL + path
	headers := map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Content-Type":  "application/json",
	}

	status, respBytes, err := httpclient.DoJSON(ctx, httpClient, method, reqURL, headers, reqBody)
	if err != nil {
		return respBytes, status, fmt.Errorf("telnyx: %w", err)
	}
	return respBytes, status, nil
}
