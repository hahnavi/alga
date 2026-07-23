package twilio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/notification"
)

func TestBuildTwiML(t *testing.T) {
	t.Parallel()

	t.Run("without brief", func(t *testing.T) {
		t.Parallel()
		cb := "https://example.com/api/v1/twilio/callback?incident=42"
		out := buildTwiML(42, 3, "", cb)

		if !strings.HasPrefix(out, "<?xml") {
			t.Fatalf("expected TwiML to start with <?xml, got: %s", out)
		}
		for _, want := range []string{"42", "level 3", "<Gather", `numDigits="1"`, `action="` + cb + `"`, "Press 1 to acknowledge"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected TwiML to contain %q\nGot: %s", want, out)
			}
		}
	})

	t.Run("with brief inserts title", func(t *testing.T) {
		t.Parallel()
		cb := "https://example.com/api/v1/twilio/callback?incident=42"
		out := buildTwiML(42, 3, "Database primary unreachable", cb)

		if !strings.Contains(out, "Database primary unreachable.") {
			t.Errorf("expected TwiML to speak brief, got: %s", out)
		}
		// Menu instructions must still be present alongside the brief.
		if !strings.Contains(out, "Press 1 to acknowledge") {
			t.Errorf("expected menu instructions retained, got: %s", out)
		}
	})

	t.Run("XML-escapes brief", func(t *testing.T) {
		t.Parallel()
		cb := "https://example.com/cb"
		out := buildTwiML(1, 1, "CPU < 90% & rising", cb)
		if !strings.Contains(out, "CPU &lt; 90% &amp; rising") {
			t.Errorf("expected XML-escaped brief, got: %s", out)
		}
		if strings.Contains(out, "< 90%") {
			t.Errorf("unescaped '<' leaked into TwiML: %s", out)
		}
	})
}

func TestRePromptTwiML(t *testing.T) {
	t.Parallel()

	t.Run("re-prompts while under cap", func(t *testing.T) {
		t.Parallel()
		cb := "/api/v1/twilio/callback?incident=1&attempt=2&user=u"
		out := RePromptTwiML(cb, 2)
		if !strings.Contains(out, "<Gather") {
			t.Fatalf("attempt 2 should still gather, got: %s", out)
		}
		// Attribute is XML-escaped (& -> &amp;), so match the escaped form.
		escaped := strings.ReplaceAll(cb, "&", "&amp;")
		if !strings.Contains(out, `action="`+escaped+`"`) {
			t.Errorf("expected gather action %q in: %s", escaped, out)
		}
	})

	t.Run("goodbye once over cap", func(t *testing.T) {
		t.Parallel()
		out := RePromptTwiML("x", MaxIVRAttempts+1)
		if strings.Contains(out, "<Gather") {
			t.Fatalf("attempt > cap should not gather, got: %s", out)
		}
		if !strings.Contains(out, "Goodbye") {
			t.Errorf("expected goodbye TwiML, got: %s", out)
		}
	})
}

func TestEnabled(t *testing.T) {
	t.Parallel()

	if NewClient("", "", "").Enabled() {
		t.Fatal("client with empty credentials should not be enabled")
	}

	c := NewClient("sid", "tok", "+15551234567")
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
}

func TestCallbackURL(t *testing.T) {
	t.Parallel()

	t.Run("trims trailing slash from base and embeds attempt", func(t *testing.T) {
		t.Parallel()
		c := NewClient("s", "t", "+1")
		c.SetCallbackBaseURL("https://alga.example.com/")
		want := "https://alga.example.com/api/v1/twilio/callback?incident=99&attempt=1"
		if got := c.callbackURL(99, 1, "", "https://alga.example.com"); got != want {
			t.Fatalf("callbackURL = %q, want %q", got, want)
		}
	})

	t.Run("relative path when base empty", func(t *testing.T) {
		t.Parallel()
		c := NewClient("s", "t", "+1")
		want := "/api/v1/twilio/callback?incident=99&attempt=1"
		if got := c.callbackURL(99, 1, "", ""); got != want {
			t.Fatalf("callbackURL = %q, want %q", got, want)
		}
	})

	t.Run("includes user when provided", func(t *testing.T) {
		t.Parallel()
		uid := uuid.New()
		c := NewClient("s", "t", "+1")
		got := c.callbackURL(7, 1, uid.String(), "")
		if !strings.Contains(got, "user="+uid.String()) {
			t.Errorf("expected user=%s in %q", uid, got)
		}
	})

	t.Run("attempt 0 used for status callback", func(t *testing.T) {
		t.Parallel()
		c := NewClient("s", "t", "+1")
		got := c.callbackURL(7, 0, "", "")
		if !strings.Contains(got, "attempt=0") {
			t.Errorf("expected attempt=0 for status callback, got %q", got)
		}
	})
}

func TestCall_Success(t *testing.T) {
	t.Parallel()

	var posted url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		posted = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"sid":"CA123","status":"queued"}`))
	}))
	defer srv.Close()

	c := NewClient("ACxxx", "tok", "+15550000000")
	c.baseURL = srv.URL
	c.SetCallbackBaseURL("https://example.com")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	uid := uuid.New()
	sid, err := c.Call(ctx, "+15559999999", 5, 2, notification.CallOptions{UserID: &uid})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if sid != "CA123" {
		t.Errorf("sid = %q, want CA123", sid)
	}
	if got := posted.Get("To"); got != "+15559999999" {
		t.Errorf("To = %q, want +15559999999", got)
	}
	if got := posted.Get("From"); got != "+15550000000" {
		t.Errorf("From = %q, want +15550000000", got)
	}
	if twiml := posted.Get("Twiml"); !strings.Contains(twiml, "5") {
		t.Errorf("expected Twiml to mention incident 5, got: %s", twiml)
	}
	// The gather action URL inside TwiML should carry attempt=1 + user id.
	twiml := posted.Get("Twiml")
	if !strings.Contains(twiml, "attempt=1") {
		t.Errorf("expected Twiml gather action to carry attempt=1, got: %s", twiml)
	}
	if !strings.Contains(twiml, "user="+uid.String()) {
		t.Errorf("expected Twiml gather action to carry user id, got: %s", twiml)
	}
	if cb := posted.Get("StatusCallback"); !strings.Contains(cb, "incident=5") {
		t.Errorf("expected StatusCallback to contain incident=5, got: %s", cb)
	}
}

func TestCall_EmptyRecipient(t *testing.T) {
	t.Parallel()
	c := NewClient("ACxxx", "tok", "+15550000000")

	sid, err := c.Call(context.Background(), "", 1, 1, notification.CallOptions{})
	if err == nil {
		t.Fatal("expected error for empty recipient, got nil")
	}
	if sid != "" {
		t.Errorf("sid = %q, want empty", sid)
	}
}

func TestCall_APIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":403}`))
	}))
	defer srv.Close()

	c := NewClient("ACxxx", "tok", "+15550000000")
	c.baseURL = srv.URL

	sid, err := c.Call(context.Background(), "+15559999999", 1, 1, notification.CallOptions{})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
	if sid != "" {
		t.Errorf("sid = %q, want empty on error", sid)
	}
}
