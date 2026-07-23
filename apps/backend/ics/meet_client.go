package ics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2/jwt"

	"alga/internal/httpclient"
)

const (
	meetSpaceAdminScope = "https://www.googleapis.com/auth/meet.space.admin"
	meetDefaultEndpoint = "https://meet.googleapis.com"
)

// serviceAccountCredentials holds the fields needed from a Google service-account
// JSON file to mint a JWT bearer token.
type serviceAccountCredentials struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// SpaceResult holds the fields persisted on the incident when a Meet space is created.
type SpaceResult struct {
	SpaceName   string
	MeetingURI  string
	MeetingCode string
}

// MeetSpaceCreator is the seam used by the war-room provisioner and the API handler.
// A nil client means the feature is disabled.
type MeetSpaceCreator interface {
	CreateSpace(ctx context.Context) (*SpaceResult, error)
}

// MeetClient creates Google Meet spaces using a service-account JWT bearer client.
type MeetClient struct {
	httpClient *http.Client
	endpoint   string
}

// NewMeetClient loads a service-account JSON file and returns a MeetClient whose
// HTTP client mints JWT bearer tokens for the Meet space admin scope.
func NewMeetClient(credentialsPath string) (*MeetClient, error) {
	if credentialsPath == "" {
		return nil, errors.New("google meet credentials path is empty")
	}
	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("read google meet credentials: %w", err)
	}
	var sa serviceAccountCredentials
	if err := json.Unmarshal(data, &sa); err != nil {
		return nil, fmt.Errorf("parse google meet credentials: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("google meet credentials missing client_email or private_key")
	}
	tokenURL := sa.TokenURI
	if tokenURL == "" {
		tokenURL = "https://oauth2.googleapis.com/token"
	}
	config := &jwt.Config{
		Email:      sa.ClientEmail,
		PrivateKey: []byte(sa.PrivateKey),
		Scopes:     []string{meetSpaceAdminScope},
		TokenURL:   tokenURL,
	}
	// oauth2's config.Client wraps a *http.Client with Token refresh but leaves
	// Timeout=0, so an unresponsive Google Meet API would hang the caller
	// indefinitely. Bound every request via a deadline-bearing base transport.
	oauthClient := config.Client(context.Background())
	oauthClient.Timeout = 30 * time.Second
	return &MeetClient{httpClient: oauthClient, endpoint: meetDefaultEndpoint}, nil
}

// CreateSpace creates a new Meet space via POST /v2/spaces.
func (c *MeetClient) CreateSpace(ctx context.Context) (*SpaceResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v2/spaces", nil)
	if err != nil {
		return nil, fmt.Errorf("build meet create-space request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call meet create-space: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, httpclient.MaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read meet create-space response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("meet create-space failed: status %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Name        string `json:"name"`
		MeetingCode string `json:"meetingCode"`
		MeetingURI  string `json:"meetingUri"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode meet create-space response: %w", err)
	}
	return &SpaceResult{
		SpaceName:   parsed.Name,
		MeetingURI:  parsed.MeetingURI,
		MeetingCode: parsed.MeetingCode,
	}, nil
}
