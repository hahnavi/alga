package ics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMeetClientCreateSpace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/v2/spaces") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"spaces/abc","meetingCode":"abc-defg-hij","meetingUri":"https://meet.google.com/abc"}`))
	}))
	defer srv.Close()

	client := &MeetClient{httpClient: srv.Client(), endpoint: srv.URL}
	res, err := client.CreateSpace(context.Background())
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if res.SpaceName != "spaces/abc" {
		t.Fatalf("SpaceName = %q", res.SpaceName)
	}
	if res.MeetingURI != "https://meet.google.com/abc" {
		t.Fatalf("MeetingURI = %q", res.MeetingURI)
	}
	if res.MeetingCode != "abc-defg-hij" {
		t.Fatalf("MeetingCode = %q", res.MeetingCode)
	}
}

func TestMeetClientCreateSpaceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &MeetClient{httpClient: srv.Client(), endpoint: srv.URL}
	if _, err := client.CreateSpace(context.Background()); err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestNewMeetClientMissingFile(t *testing.T) {
	if _, err := NewMeetClient("/nonexistent/path/creds.json"); err == nil {
		t.Fatal("expected error for missing credentials file")
	}
}
