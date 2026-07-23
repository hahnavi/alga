package telnyx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/notification"
)

func TestEnabled(t *testing.T) {
	t.Parallel()

	if NewClient("", "", "", "", "", "").Enabled() {
		t.Fatal("client with empty credentials should not be enabled")
	}

	c := NewClient("key", "conn", "+15551234567", "", "", "")
	if !c.Enabled() {
		t.Fatal("fully configured client should be enabled")
	}

	c.SetDisabled(true)
	if c.Enabled() {
		t.Fatal("disabled client should not be enabled")
	}

	c.SetDisabled(false)
	if !c.Enabled() {
		t.Fatal("re-enabled client should be enabled")
	}

	if NewClient("key", "", "+1", "", "", "").Enabled() {
		t.Fatal("client missing connection id should not be enabled")
	}
	if NewClient("key", "conn", "", "", "", "").Enabled() {
		t.Fatal("client missing from number should not be enabled")
	}
	if NewClient("", "conn", "+1", "", "", "").Enabled() {
		t.Fatal("client missing api key should not be enabled")
	}
}

func TestProviderName(t *testing.T) {
	t.Parallel()
	c := NewClient("k", "c", "+1", "", "", "")
	if got := c.ProviderName(); got != "telnyx" {
		t.Fatalf("ProviderName = %q, want \"telnyx\"", got)
	}
}

func TestTTSDefaults(t *testing.T) {
	t.Parallel()

	// Empty TTS fields fall back to built-in defaults. Default voice is a free
	// Telnyx KokoroTTS voice; language defaults to en-US.
	c := NewClient("k", "c", "+1", "", "", "")
	if got := c.TTSVoice(); got != defaultTTSVoice {
		t.Errorf("default TTSVoice = %q, want %q", got, defaultTTSVoice)
	}
	if got := c.TTSLanguage(); got != "en-US" {
		t.Errorf("default TTSLanguage = %q, want en-US", got)
	}
	if got := c.TTSAPIKeyRef(); got != "" {
		t.Errorf("default TTSAPIKeyRef = %q, want empty", got)
	}

	// Explicit voice is preserved verbatim, including the provider prefix.
	c = NewClient("k", "c", "+1", "Polly.Brian", "en-GB", "")
	if got := c.TTSVoice(); got != "Polly.Brian" {
		t.Errorf("TTSVoice = %q, want Polly.Brian", got)
	}
	if got := c.TTSLanguage(); got != "en-GB" {
		t.Errorf("TTSLanguage = %q, want en-GB", got)
	}

	// SetTTS re-applies (and re-defaults on empty).
	c.SetTTS("", "", "")
	if got := c.TTSVoice(); got != defaultTTSVoice {
		t.Errorf("after SetTTS reset, TTSVoice = %q, want %q", got, defaultTTSVoice)
	}
}

func TestTTSPayload(t *testing.T) {
	t.Parallel()

	t.Run("non-ElevenLabs voice omits voice_settings", func(t *testing.T) {
		t.Parallel()
		c := NewClient("k", "c", "+1", "Polly.Brian", "en-US", "")
		body := c.ttsPayload("hello")
		if body["service"] != nil {
			t.Errorf("service field should be absent, got %v", body["service"])
		}
		if body["voice"] != "Polly.Brian" {
			t.Errorf("voice = %v, want Polly.Brian", body["voice"])
		}
		if body["language"] != "en-US" {
			t.Errorf("language = %v, want en-US", body["language"])
		}
		if _, ok := body["voice_settings"]; ok {
			t.Errorf("voice_settings should be absent for non-ElevenLabs voice")
		}
	})

	t.Run("ElevenLabs voice includes voice_settings when ref set", func(t *testing.T) {
		t.Parallel()
		c := NewClient("k", "c", "+1", "ElevenLabs.eleven_flash_v2_5.iP95p4xoKVk53GoZ742B", "en-US", "elevenlabs-prod")
		body := c.ttsPayload("hello")
		vs, ok := body["voice_settings"].(map[string]any)
		if !ok {
			t.Fatalf("voice_settings missing or wrong type: %T", body["voice_settings"])
		}
		if vs["api_key_ref"] != "elevenlabs-prod" {
			t.Errorf("api_key_ref = %v, want elevenlabs-prod", vs["api_key_ref"])
		}
		if body["voice"] != "ElevenLabs.eleven_flash_v2_5.iP95p4xoKVk53GoZ742B" {
			t.Errorf("voice should retain ElevenLabs prefix, got %v", body["voice"])
		}
	})

	t.Run("ElevenLabs voice without ref omits voice_settings", func(t *testing.T) {
		t.Parallel()
		// Missing api_key_ref is an operator misconfiguration, but the payload
		// builder must not panic; Telnyx will reject the call.
		c := NewClient("k", "c", "+1", "ElevenLabs.Default.cgSgspJ2msm6clMCkdW9", "", "")
		body := c.ttsPayload("hello")
		if _, ok := body["voice_settings"]; ok {
			t.Errorf("voice_settings should be omitted when api_key_ref is empty")
		}
	})
}

func TestCallbackURL(t *testing.T) {
	t.Parallel()

	t.Run("relative path when base empty", func(t *testing.T) {
		t.Parallel()
		c := NewClient("k", "c", "+1", "", "", "")
		got := c.callbackURL(99, 3, "", "")
		if !strings.Contains(got, "incident=99") {
			t.Errorf("expected incident=99 in %q", got)
		}
		if !strings.Contains(got, "level=3") {
			t.Errorf("expected level=3 in %q", got)
		}
		if strings.Contains(got, "://") {
			t.Errorf("expected relative path, got %q", got)
		}
		if strings.Contains(got, "user=") {
			t.Errorf("expected no user param when empty, got %q", got)
		}
	})

	t.Run("uses callback base url", func(t *testing.T) {
		t.Parallel()
		c := NewClient("k", "c", "+1", "", "", "")
		c.SetCallbackBaseURL("https://alga.example.com/")
		got := c.callbackURL(7, 2, "", "https://alga.example.com")
		if !strings.HasPrefix(got, "https://alga.example.com/api/v1/telnyx/callback") {
			t.Fatalf("expected base-prefixed url, got %q", got)
		}
		if !strings.Contains(got, "incident=7") {
			t.Errorf("expected incident=7 in %q", got)
		}
		if !strings.Contains(got, "level=2") {
			t.Errorf("expected level=2 in %q", got)
		}
	})

	t.Run("embeds user id when provided", func(t *testing.T) {
		t.Parallel()
		uid := uuid.New()
		c := NewClient("k", "c", "+1", "", "", "")
		got := c.callbackURL(7, 2, uid.String(), "")
		if !strings.Contains(got, "user="+uid.String()) {
			t.Errorf("expected user=%s in %q", uid, got)
		}
	})
}

func TestCall_Success(t *testing.T) {
	t.Parallel()

	var (
		gotAuth  string
		gotCT    string
		gotPath  string
		postBody map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &postBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"call_control_id":"ccid_123"}}`))
	}))
	defer srv.Close()

	c := NewClient("secret-api-key", "conn-xyz", "+15550000000", "", "", "")
	c.baseURL = srv.URL
	c.SetCallbackBaseURL("https://example.com")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id, err := c.Call(ctx, "+15559999999", 5, 2, notification.CallOptions{})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if id != "ccid_123" {
		t.Fatalf("call control id = %q, want ccid_123", id)
	}
	if gotPath != "/v2/calls" {
		t.Errorf("request path = %q, want /v2/calls", gotPath)
	}
	if gotAuth != "Bearer secret-api-key" {
		t.Errorf("Authorization = %q, want Bearer secret-api-key", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if v, ok := postBody["connection_id"].(string); !ok || v != "conn-xyz" {
		t.Errorf("connection_id = %v, want conn-xyz", postBody["connection_id"])
	}
	if v, ok := postBody["to"].(string); !ok || v != "+15559999999" {
		t.Errorf("to = %v, want +15559999999", postBody["to"])
	}
	if v, ok := postBody["from"].(string); !ok || v != "+15550000000" {
		t.Errorf("from = %v, want +15550000000", postBody["from"])
	}
	if v, ok := postBody["webhook_api_version"].(string); !ok || v != "2" {
		t.Errorf("webhook_api_version = %v, want \"2\"", postBody["webhook_api_version"])
	}
	cb, _ := postBody["webhook_url"].(string)
	if !strings.Contains(cb, "incident=5") || !strings.Contains(cb, "level=2") {
		t.Errorf("webhook_url = %q, expected to contain incident=5 and level=2", cb)
	}
	if strings.Contains(cb, "user=") {
		t.Errorf("webhook_url = %q, expected no user param when UserID is nil", cb)
	}
}

func TestCall_UserIDInWebhookURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"call_control_id":"ccid"}}`))
	}))
	defer srv.Close()

	uid := uuid.New()
	c := NewClient("k", "c", "+15550000000", "", "", "")
	c.baseURL = srv.URL
	c.SetCallbackBaseURL("https://example.com")

	var postBody map[string]any
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &postBody)
		_, _ = w.Write([]byte(`{"data":{"call_control_id":"ccid"}}`))
	})

	if _, err := c.Call(context.Background(), "+15559999999", 5, 2, notification.CallOptions{UserID: &uid}); err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	cb, _ := postBody["webhook_url"].(string)
	if !strings.Contains(cb, "user="+uid.String()) {
		t.Errorf("webhook_url = %q, expected user=%s", cb, uid)
	}
}

func TestCall_ElevenLabsTTSInActionBody(t *testing.T) {
	t.Parallel()
	var gatherBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/actions/gather_using_speak") {
			gatherBody, _ = io.ReadAll(r.Body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("k", "c", "+15550000000", "ElevenLabs.eleven_flash_v2_5.iP95p4xoKVk53GoZ742B", "en-US", "elevenlabs-prod")
	c.baseURL = srv.URL

	if err := c.GatherUsingSpeak(context.Background(), "ccid_1", "press now", ""); err != nil {
		t.Fatalf("GatherUsingSpeak error: %v", err)
	}
	body := string(gatherBody)
	// No service field in the current Telnyx TTS API; provider is in voice prefix.
	if strings.Contains(body, `"service"`) {
		t.Errorf("gather body = %s, should not contain service field", body)
	}
	if !strings.Contains(body, `"voice":"ElevenLabs.eleven_flash_v2_5.iP95p4xoKVk53GoZ742B"`) {
		t.Errorf("gather body = %s, expected full ElevenLabs voice id", body)
	}
	if !strings.Contains(body, `"api_key_ref":"elevenlabs-prod"`) {
		t.Errorf("gather body = %s, expected voice_settings.api_key_ref", body)
	}
	if strings.Contains(body, `"client_state"`) {
		t.Errorf("gather body = %s, expected no client_state when empty", body)
	}

	// client_state is echoed when provided.
	if err := c.GatherUsingSpeak(context.Background(), "ccid_1", "press now", "abc"); err != nil {
		t.Fatalf("GatherUsingSpeak error: %v", err)
	}
}

func TestCall_Errors(t *testing.T) {
	t.Parallel()

	t.Run("empty recipient", func(t *testing.T) {
		t.Parallel()
		c := NewClient("k", "c", "+1", "", "", "")
		if _, err := c.Call(context.Background(), "", 1, 1, notification.CallOptions{}); err == nil {
			t.Fatal("expected error for empty recipient, got nil")
		}
	})

	t.Run("disabled client", func(t *testing.T) {
		t.Parallel()
		c := NewClient("k", "c", "+1", "", "", "")
		c.SetDisabled(true)
		if _, err := c.Call(context.Background(), "+15559999999", 1, 1, notification.CallOptions{}); err == nil {
			t.Fatal("expected error for disabled client, got nil")
		}
	})

	t.Run("non-2xx response", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errors":[{"code":"forbidden"}]}`))
		}))
		defer srv.Close()

		c := NewClient("k", "c", "+1", "", "", "")
		c.baseURL = srv.URL
		if _, err := c.Call(context.Background(), "+15559999999", 1, 1, notification.CallOptions{}); err == nil {
			t.Fatal("expected error for 403 response, got nil")
		}
	})
}

func TestCallAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		action    string
		invoke    func(c *Client, ctx context.Context) error
		wantPath  string
		checkBody func(t *testing.T, raw []byte)
	}{
		{
			name:     "answer",
			action:   "answer",
			invoke:   func(c *Client, ctx context.Context) error { return c.Answer(ctx, "ccid_1") },
			wantPath: "/v2/calls/ccid_1/actions/answer",
		},
		{
			name:     "hangup",
			action:   "hangup",
			invoke:   func(c *Client, ctx context.Context) error { return c.Hangup(ctx, "ccid_1") },
			wantPath: "/v2/calls/ccid_1/actions/hangup",
		},
		{
			name:     "speak",
			action:   "speak",
			invoke:   func(c *Client, ctx context.Context) error { return c.Speak(ctx, "ccid_1", "hello") },
			wantPath: "/v2/calls/ccid_1/actions/speak",
			checkBody: func(t *testing.T, raw []byte) {
				if !strings.Contains(string(raw), `"payload":"hello"`) {
					t.Errorf("expected payload:\"hello\" in body, got %s", raw)
				}
			},
		},
		{
			name:     "gather_using_speak",
			action:   "gather_using_speak",
			invoke:   func(c *Client, ctx context.Context) error { return c.GatherUsingSpeak(ctx, "ccid_1", "press now", "") },
			wantPath: "/v2/calls/ccid_1/actions/gather_using_speak",
			checkBody: func(t *testing.T, raw []byte) {
				body := string(raw)
				if !strings.Contains(body, `"payload":"press now"`) {
					t.Errorf("expected payload field, got %s", body)
				}
				if !strings.Contains(body, `"valid_digits":"12"`) {
					t.Errorf("expected valid_digits:\"12\", got %s", body)
				}
				if !strings.Contains(body, `"max_digits":1`) {
					t.Errorf("expected max_digits:1, got %s", body)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/success", func(t *testing.T) {
			t.Parallel()
			var gotPath string
			var gotBody []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotBody, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			c := NewClient("k", "c", "+1", "", "", "")
			c.baseURL = srv.URL
			if err := tc.invoke(c, context.Background()); err != nil {
				t.Fatalf("invoke returned error: %v", err)
			}
			if gotPath != tc.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tc.wantPath)
			}
			if tc.checkBody != nil {
				tc.checkBody(t, gotBody)
			}
		})

		t.Run(tc.name+"/error", func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"boom"}`))
			}))
			defer srv.Close()

			c := NewClient("k", "c", "+1", "", "", "")
			c.baseURL = srv.URL
			err := tc.invoke(c, context.Background())
			if err == nil {
				t.Fatalf("expected error for 500 response, got nil")
			}
			if !strings.Contains(err.Error(), tc.action) {
				t.Errorf("expected error to mention action %q, got %v", tc.action, err)
			}
			if !strings.Contains(err.Error(), "500") {
				t.Errorf("expected error to mention status 500, got %v", err)
			}
		})
	}
}
