package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"alga/store"
)

type stubTwilioSender struct {
	enabled    bool
	lastTo     string
	lastInc    int64
	lastLevel  int
	lastUserID *uuid.UUID
	lastTitle  string
	callErr    error
	callSID    string
	calls      int
}

func (s *stubTwilioSender) Enabled() bool { return s.enabled }

func (s *stubTwilioSender) ProviderName() string { return "twilio" }

func (s *stubTwilioSender) Call(_ context.Context, to string, incidentNumber int64, level int, opts CallOptions) (string, error) {
	s.lastTo, s.lastInc, s.lastLevel = to, incidentNumber, level
	s.lastUserID = opts.UserID
	s.lastTitle = opts.Title
	s.calls++
	return s.callSID, s.callErr
}

type stubVoiceOnlyProvider struct {
	enabled    bool
	lastTo     string
	lastInc    int64
	lastLevel  int
	lastUserID *uuid.UUID
	lastTitle  string
	callErr    error
	callSID    string
	calls      int
}

func (s *stubVoiceOnlyProvider) Enabled() bool { return s.enabled }

func (s *stubVoiceOnlyProvider) ProviderName() string { return "voiceonly" }

func (s *stubVoiceOnlyProvider) Call(_ context.Context, to string, incidentNumber int64, level int, opts CallOptions) (string, error) {
	s.lastTo, s.lastInc, s.lastLevel = to, incidentNumber, level
	s.lastUserID = opts.UserID
	s.lastTitle = opts.Title
	s.calls++
	return s.callSID, s.callErr
}

type stubUserStore struct {
	store.UserStore
	user  *store.UserRecord
	err   error
	prefs map[string]any
}

func (s *stubUserStore) GetByID(_ uuid.UUID) (*store.UserRecord, error) {
	return s.user, s.err
}

func (s *stubUserStore) GetNotificationPreferences(_ context.Context, _ string) (map[string]any, error) {
	return s.prefs, nil
}

type stubDeliveryStore struct {
	store.NotificationDeliveryStore
	last *store.NotificationDeliveryRecord
}

func (s *stubDeliveryStore) Create(_ context.Context, record *store.NotificationDeliveryRecord) (*store.NotificationDeliveryRecord, error) {
	s.last = record
	return record, nil
}

func testDispatcher(twilioEnabled bool, user *store.UserRecord) (*Dispatcher, *stubTwilioSender, *stubDeliveryStore, *stubUserStore) {
	tw := &stubTwilioSender{enabled: twilioEnabled, callSID: "XSID"}
	ds := &stubDeliveryStore{}
	usr := &stubUserStore{user: user}
	return &Dispatcher{
		userStore:     usr,
		deliveryStore: ds,
		voiceProvider: tw,
	}, tw, ds, usr
}

func TestDispatchChannels_Twilio(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	hasPhone := &store.UserRecord{ID: uid, Phone: "+15551112222"}
	noPhone := &store.UserRecord{ID: uid, Phone: ""}

	tests := []struct {
		name       string
		channel    string
		enabled    bool
		user       *store.UserRecord
		callErr    error
		wantStatus string
		wantCalls  int
	}{
		{"voice delivered", "voice", true, hasPhone, nil, "delivered", 1},
		{"voice no phone", "voice", true, noPhone, nil, "skipped_no_phone", 0},
		{"voice call error", "voice", true, hasPhone, errors.New("boom"), "failed", 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, tw, ds, _ := testDispatcher(tc.enabled, tc.user)
			tw.callErr = tc.callErr

			err := d.DispatchChannels(
				context.Background(),
				uid.String(),
				"incident.escalated",
				"title",
				"message",
				"incident",
				"1",
				[]string{tc.channel},
				nil,
				7,
				3,
			)
			if err != nil {
				t.Fatalf("DispatchChannels returned error: %v", err)
			}

			if tw.calls != tc.wantCalls {
				t.Errorf("Call count = %d, want %d", tw.calls, tc.wantCalls)
			}
			if tw.calls > 0 {
				if tw.lastTo != tc.user.Phone {
					t.Errorf("Call recipient = %q, want %q", tw.lastTo, tc.user.Phone)
				}
				if tw.lastInc != 7 || tw.lastLevel != 3 {
					t.Errorf("Call args = (incident %d, level %d), want (7, 3)", tw.lastInc, tw.lastLevel)
				}
			}
			if ds.last == nil {
				t.Fatal("expected delivery record logged, got nil")
			}
			if ds.last.Channel != tc.channel {
				t.Errorf("delivery channel = %q, want %q", ds.last.Channel, tc.channel)
			}
			if ds.last.Status != tc.wantStatus {
				t.Errorf("delivery status = %q, want %q", ds.last.Status, tc.wantStatus)
			}
		})
	}
}

func TestDispatchChannels_ExplicitVsFallback(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	user := &store.UserRecord{ID: uid, Phone: "+15551112222"}

	t.Run("explicit channels override ResolveChannels", func(t *testing.T) {
		t.Parallel()
		d, tw, ds, usr := testDispatcher(true, user)
		usr.prefs = map[string]any{"default_channel": "voice"}

		err := d.DispatchChannels(
			context.Background(), uid.String(), "incident.escalated",
			"t", "m", "incident", "1",
			[]string{"voice"}, nil, 1, 1,
		)
		if err != nil {
			t.Fatalf("DispatchChannels returned error: %v", err)
		}
		if tw.calls != 1 {
			t.Errorf("explicit voice channel should dispatch: calls=%d", tw.calls)
		}
		if ds.last == nil || ds.last.Channel != "voice" {
			t.Errorf("expected voice delivery logged, got %+v", ds.last)
		}
	})

	t.Run("nil channels fall back to ResolveChannels without panic", func(t *testing.T) {
		t.Parallel()
		d, _, ds, _ := testDispatcher(false, user)

		err := d.DispatchChannels(
			context.Background(), uid.String(), "incident.escalated",
			"t", "m", "incident", "1",
			nil, nil, 1, 1,
		)
		if err != nil {
			t.Fatalf("DispatchChannels returned error: %v", err)
		}
		if ds.last != nil {
			t.Errorf("in_app channel should not log a delivery record, got %+v", ds.last)
		}
	})
}

func TestDispatchChannels_VoiceOnlyProvider(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	user := &store.UserRecord{ID: uid, Phone: "+15551112222"}

	t.Run("voice delivered by voice-only provider", func(t *testing.T) {
		t.Parallel()
		voice := &stubVoiceOnlyProvider{enabled: true, callSID: "VCID"}
		ds := &stubDeliveryStore{}
		usr := &stubUserStore{user: user}
		d := &Dispatcher{
			userStore:     usr,
			deliveryStore: ds,
			voiceProvider: voice,
		}

		err := d.DispatchChannels(
			context.Background(), uid.String(), "incident.escalated",
			"title", "message", "incident", "1",
			[]string{"voice"}, nil, 7, 3,
		)
		if err != nil {
			t.Fatalf("DispatchChannels returned error: %v", err)
		}
		if voice.calls != 1 {
			t.Fatalf("expected 1 voice call, got %d", voice.calls)
		}
		if voice.lastTo != user.Phone {
			t.Errorf("Call recipient = %q, want %q", voice.lastTo, user.Phone)
		}
		if voice.lastInc != 7 || voice.lastLevel != 3 {
			t.Errorf("Call args = (incident %d, level %d), want (7, 3)", voice.lastInc, voice.lastLevel)
		}
		if ds.last == nil {
			t.Fatal("expected delivery record logged for voice")
		}
		if ds.last.Channel != "voice" || ds.last.Status != "delivered" {
			t.Errorf("delivery = %q/%q, want voice/delivered", ds.last.Channel, ds.last.Status)
		}
	})
}

// TestDispatchChannels_VoiceOptOut verifies that a user with VoiceOptOut=true
// is not paged even when the escalation policy forces the voice channel.
// This is the primary cost-control guard for users who do not want to be
// called for any reason.
func TestDispatchChannels_VoiceOptOut(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	optedOut := &store.UserRecord{ID: uid, Phone: "+15551112222", VoiceOptOut: true}

	d, tw, ds, _ := testDispatcher(true, optedOut)
	err := d.DispatchChannels(
		context.Background(), uid.String(), "incident.escalated",
		"title", "message", "incident", "1",
		[]string{"voice"}, nil, 7, 3,
	)
	if err != nil {
		t.Fatalf("DispatchChannels returned error: %v", err)
	}
	if tw.calls != 0 {
		t.Errorf("opted-out user should not be called: calls=%d", tw.calls)
	}
	if ds.last == nil || ds.last.Status != "skipped_opt_out" {
		t.Errorf("delivery = %+v, want channel=voice status=skipped_opt_out", ds.last)
	}
}

// TestClaimVoiceCallSlotFailsOpen verifies that when the dispatcher is built
// without a Valkey client (e.g. local dev), the dedup guard is a no-op and
// calls proceed. We must never silently swallow an escalation because a
// non-essential guard is unavailable.
func TestClaimVoiceCallSlotFailsOpen(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	user := &store.UserRecord{ID: uid, Phone: "+15551112222"}
	d, tw, _, _ := testDispatcher(true, user)
	d.vkClient = nil

	// Two consecutive dispatches to the same (incident, user, level) both
	// place a call because there is no Valkey to dedup against.
	for i := range 2 {
		err := d.DispatchChannels(
			context.Background(), uid.String(), "incident.escalated",
			"t", "m", "incident", "1",
			[]string{"voice"}, nil, 99, 1,
		)
		if err != nil {
			t.Fatalf("DispatchChannels returned error on call %d: %v", i+1, err)
		}
	}
	if tw.calls != 2 {
		t.Errorf("expected 2 calls (no Valkey => no dedup), got %d", tw.calls)
	}
}

// stubIncidentStore captures GetIncident lookups so the dispatcher's
// incidentBrief path can be exercised without a real Postgres store. Other
// IncidentStore methods are left nil — they panic if invoked, which is the
// desired signal in a voice-dispatch-only test.
type stubIncidentStore struct {
	store.IncidentStore
	byNumber map[int64]*store.IncidentRecord
	calls    int
}

func (s *stubIncidentStore) GetIncident(_ context.Context, incidentNumber int64) (*store.IncidentRecord, error) {
	s.calls++
	if inc, ok := s.byNumber[incidentNumber]; ok {
		return inc, nil
	}
	return nil, errors.New("not found")
}

// TestDispatchChannels_IncidentBrief verifies the dispatcher threads the
// incident Title (truncated, single-line) into CallOptions.Title so voice
// providers can speak it. Two cases: title present → brief populated; missing
// incident → brief empty, call still placed.
func TestDispatchChannels_IncidentBrief(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	user := &store.UserRecord{ID: uid, Phone: "+15551112222"}

	t.Run("title flows into CallOptions.Title and is truncated", func(t *testing.T) {
		t.Parallel()
		longTitle := "API latency spike across all regions, customers seeing 5xx errors, on-call engaged and investigating root cause now"
		incStore := &stubIncidentStore{byNumber: map[int64]*store.IncidentRecord{
			77: {IncidentNumber: 77, Title: longTitle},
		}}
		tw := &stubTwilioSender{enabled: true, callSID: "SID"}
		d := &Dispatcher{
			userStore:     &stubUserStore{user: user},
			deliveryStore: &stubDeliveryStore{},
			incidentStore: incStore,
			voiceProvider: tw,
		}

		err := d.DispatchChannels(
			context.Background(), uid.String(), "incident.escalated",
			"title", "message", "incident", "1",
			[]string{"voice"}, nil, 77, 1,
		)
		if err != nil {
			t.Fatalf("DispatchChannels returned error: %v", err)
		}
		if tw.calls != 1 {
			t.Fatalf("expected 1 voice call, got %d", tw.calls)
		}
		if tw.lastTitle == "" {
			t.Fatal("expected non-empty Title in CallOptions")
		}
		if len(tw.lastTitle) > incidentBriefMaxLen {
			t.Errorf("brief not truncated: len=%d, max=%d (%q)", len(tw.lastTitle), incidentBriefMaxLen, tw.lastTitle)
		}
		if !contains(tw.lastTitle, "API latency spike") {
			t.Errorf("brief lost title content: %q", tw.lastTitle)
		}
	})

	t.Run("missing incident leaves brief empty but still calls", func(t *testing.T) {
		t.Parallel()
		incStore := &stubIncidentStore{byNumber: map[int64]*store.IncidentRecord{}}
		tw := &stubTwilioSender{enabled: true, callSID: "SID"}
		d := &Dispatcher{
			userStore:     &stubUserStore{user: user},
			deliveryStore: &stubDeliveryStore{},
			incidentStore: incStore,
			voiceProvider: tw,
		}

		err := d.DispatchChannels(
			context.Background(), uid.String(), "incident.escalated",
			"title", "message", "incident", "1",
			[]string{"voice"}, nil, 999, 1,
		)
		if err != nil {
			t.Fatalf("DispatchChannels returned error: %v", err)
		}
		if tw.calls != 1 {
			t.Fatalf("expected call to still be placed when brief lookup fails, got %d", tw.calls)
		}
		if tw.lastTitle != "" {
			t.Errorf("expected empty brief on missing incident, got %q", tw.lastTitle)
		}
	})

	t.Run("nil incidentStore leaves brief empty without panic", func(t *testing.T) {
		t.Parallel()
		tw := &stubTwilioSender{enabled: true, callSID: "SID"}
		d := &Dispatcher{
			userStore:     &stubUserStore{user: user},
			deliveryStore: &stubDeliveryStore{},
			incidentStore: nil,
			voiceProvider: tw,
		}

		err := d.DispatchChannels(
			context.Background(), uid.String(), "incident.escalated",
			"title", "message", "incident", "1",
			[]string{"voice"}, nil, 7, 1,
		)
		if err != nil {
			t.Fatalf("DispatchChannels returned error: %v", err)
		}
		if tw.calls != 1 {
			t.Fatalf("expected call placed without incidentStore, got %d", tw.calls)
		}
		if tw.lastTitle != "" {
			t.Errorf("expected empty brief when incidentStore is nil, got %q", tw.lastTitle)
		}
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
