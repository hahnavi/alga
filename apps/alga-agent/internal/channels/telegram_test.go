package channels

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"alga-agent/internal/config"
)

// testLogger returns a logger that discards all output, for tests that need
// a non-nil logger to avoid nil-pointer panics.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeTelegramAPI is a test double for TelegramAPI that records sends and
// returns configurable errors.
type fakeTelegramAPI struct {
	me         tg.User
	sends      []tg.Chattable
	sendErrors []error // queued errors, one per Send call
	meErr      error
	updates    tg.UpdatesChannel
	stopCalled bool
}

func (f *fakeTelegramAPI) GetMe() (tg.User, error) { return f.me, f.meErr }
func (f *fakeTelegramAPI) Send(c tg.Chattable) (tg.Message, error) {
	f.sends = append(f.sends, c)
	if len(f.sendErrors) > 0 {
		err := f.sendErrors[0]
		f.sendErrors = f.sendErrors[1:]
		return tg.Message{}, err
	}
	return tg.Message{MessageID: 42}, nil
}
func (f *fakeTelegramAPI) GetUpdatesChan(config tg.UpdateConfig) tg.UpdatesChannel { return f.updates }
func (f *fakeTelegramAPI) StopReceivingUpdates()                                   { f.stopCalled = true }
func (f *fakeTelegramAPI) Request(c tg.Chattable) (*tg.APIResponse, error)         { return nil, nil }

func newTestTelegramChannel(t *testing.T, api TelegramAPI) *TelegramChannel {
	t.Helper()
	cfg := config.TelegramConfig{
		Enabled:         true,
		BotToken:        "test",
		MinEditInterval: 0,
		WebhookAddr:     "0.0.0.0:8443",
	}
	ch, err := newTelegramChannelWithAPI(api, cfg, nil, nil)
	if err != nil {
		t.Fatalf("newTelegramChannelWithAPI: %v", err)
	}
	ch.botUsername = "testbot"
	return ch
}

func TestRetryAfterFromError_ExtractsRetryAfter(t *testing.T) {
	// tg.Error embeds ResponseParameters, so RetryAfter is a promoted field.
	err := tg.Error{Code: 429, Message: "Too Many Requests", ResponseParameters: tg.ResponseParameters{RetryAfter: 7}}
	d := retryAfterFromError(err)
	if d != 7*time.Second {
		t.Errorf("retry_after = %v, want 7s", d)
	}
}

func TestRetryAfterFromError_NoRetryAfter(t *testing.T) {
	err := tg.Error{Code: 429, Message: "Too Many Requests"}
	d := retryAfterFromError(err)
	if d != 0 {
		t.Errorf("retry_after = %v, want 0", d)
	}
}

func TestRetryAfterFromError_NilError(t *testing.T) {
	d := retryAfterFromError(nil)
	if d != 0 {
		t.Errorf("retry_after = %v, want 0 for nil", d)
	}
}

func TestRetryAfterFromError_NonTGError(t *testing.T) {
	d := retryAfterFromError(errors.New("some other error"))
	if d != 0 {
		t.Errorf("retry_after = %v, want 0 for non-tg error", d)
	}
}

func TestIsRateLimited_TGError429(t *testing.T) {
	err := tg.Error{Code: 429, Message: "Too Many Requests"}
	if !isRateLimited(err) {
		t.Error("expected 429 to be rate limited")
	}
}

func TestIsRateLimited_TGError400(t *testing.T) {
	err := tg.Error{Code: 400, Message: "Bad Request"}
	if isRateLimited(err) {
		t.Error("400 should not be rate limited")
	}
}

func TestHandleRateLimit_HonorsRetryAfter(t *testing.T) {
	st := &tgStreamState{}
	ch := &TelegramChannel{logger: testLogger()}
	// Simulate a 429 with retry_after=5s.
	err := tg.Error{Code: 429, ResponseParameters: tg.ResponseParameters{RetryAfter: 5}}
	ch.handleRateLimit(123, st, err)
	if st.rateLimitedUntil.IsZero() {
		t.Error("rateLimitedUntil should be set")
	}
	// The backoff should be ~5s, not the 10s default.
	expected := time.Now().Add(5 * time.Second)
	delta := st.rateLimitedUntil.Sub(expected)
	if delta > 500*time.Millisecond || delta < -500*time.Millisecond {
		t.Errorf("rateLimitedUntil = %v, want ~%v (delta %v)", st.rateLimitedUntil, expected, delta)
	}
}

func TestHandleRateLimit_DefaultsTo10s(t *testing.T) {
	st := &tgStreamState{}
	ch := &TelegramChannel{logger: testLogger()}
	// 429 without retry_after.
	err := tg.Error{Code: 429}
	ch.handleRateLimit(123, st, err)
	expected := time.Now().Add(10 * time.Second)
	delta := st.rateLimitedUntil.Sub(expected)
	if delta > 500*time.Millisecond || delta < -500*time.Millisecond {
		t.Errorf("rateLimitedUntil = %v, want ~%v (delta %v)", st.rateLimitedUntil, expected, delta)
	}
}

func TestHandleRateLimit_FinalizesAfter60s(t *testing.T) {
	st := &tgStreamState{rateLimitStart: time.Now().Add(-61 * time.Second)}
	ch := &TelegramChannel{logger: testLogger()}
	ch.handleRateLimit(123, st, tg.Error{Code: 429})
	if !st.finalized {
		t.Error("expected finalized=true after 60s of rate limiting")
	}
}

func TestTruncateForTelegram_RuneSafe(t *testing.T) {
	// A string with multibyte runes (emoji) near the truncation boundary.
	// We must not split a rune and produce invalid UTF-8.
	emoji := "🎉" // 4 bytes
	// Build a string longer than 4000 bytes ending mid-emoji.
	base := strings.Repeat("a", 3998)
	s := base + emoji + emoji + emoji
	got := truncateForTelegram(s)
	if !utf8Valid(got) {
		t.Errorf("truncated text is not valid UTF-8: %q", got[len(got)-5:])
	}
	if !strings.Contains(got, "truncated") {
		t.Error("expected truncation marker")
	}
}

func TestTruncateForTelegram_ShortStringUnchanged(t *testing.T) {
	s := "hello world"
	if got := truncateForTelegram(s); got != s {
		t.Errorf("short string changed: got %q", got)
	}
}

func TestSendText_FallsBackToPlainTextOnParseError(t *testing.T) {
	// First Send returns a parse error, second should be plain text (no ParseMode).
	api := &fakeTelegramAPI{
		sendErrors: []error{tg.Error{Code: 400, Message: "can't parse entities: unmatched delimiter"}},
	}
	ch := newTestTelegramChannel(t, api)

	_, err := ch.sendText(123, "some _invalid_ markdown")
	if err != nil {
		t.Fatalf("sendText: %v", err)
	}
	if len(api.sends) != 2 {
		t.Fatalf("expected 2 sends (markdown + plain fallback), got %d", len(api.sends))
	}
	// First send should have ParseMode set.
	mc, ok := api.sends[0].(tg.MessageConfig)
	if !ok {
		t.Fatalf("first send is not MessageConfig: %T", api.sends[0])
	}
	if mc.ParseMode != "Markdown" {
		t.Errorf("first send ParseMode = %q, want Markdown", mc.ParseMode)
	}
	// Second send (fallback) should have empty ParseMode.
	mc2, ok := api.sends[1].(tg.MessageConfig)
	if !ok {
		t.Fatalf("second send is not MessageConfig: %T", api.sends[1])
	}
	if mc2.ParseMode != "" {
		t.Errorf("fallback send ParseMode = %q, want empty", mc2.ParseMode)
	}
}

func TestOnThinking_CreatesPlaceholder(t *testing.T) {
	api := &fakeTelegramAPI{}
	ch := newTestTelegramChannel(t, api)
	if err := ch.OnThinking(context.Background(), "123"); err != nil {
		t.Fatalf("OnThinking: %v", err)
	}
	if len(api.sends) != 2 {
		t.Fatalf("expected 2 sends (typing + placeholder), got %d", len(api.sends))
	}
	ch.streamMu.Lock()
	st, ok := ch.streamStates[123]
	ch.streamMu.Unlock()
	if !ok {
		t.Fatal("no stream state created")
	}
	if st.messageID != 42 {
		t.Errorf("messageID = %d, want 42", st.messageID)
	}
}

// utf8Valid reports whether s is valid UTF-8.
func utf8Valid(s string) bool {
	for _, r := range s {
		if r == 0xFFFD {
			return false
		}
	}
	return true
}
