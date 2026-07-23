package twilio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"alga/internal/httpclient"
	"alga/logger"
	"alga/notification"
)

type Client struct {
	mu              sync.RWMutex
	accountSID      string
	authToken       string
	fromNumber      string
	httpClient      *http.Client
	baseURL         string
	callbackBaseURL string
	disabled        bool
}

func NewClient(accountSID, authToken, fromNumber string) *Client {
	return &Client{
		accountSID: accountSID,
		authToken:  authToken,
		fromNumber: fromNumber,
		httpClient: httpclient.NewTimeoutClient(15 * time.Second),
		baseURL:    "https://api.twilio.com",
	}
}

func (c *Client) SetCallbackBaseURL(base string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbackBaseURL = strings.TrimRight(base, "/")
}

func (c *Client) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.disabled && c.accountSID != "" && c.authToken != "" && c.fromNumber != ""
}

func (c *Client) ProviderName() string {
	return "twilio"
}

func (c *Client) SetDisabled(disabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disabled = disabled
}

func (c *Client) Reconfigure(accountSID, authToken, fromNumber string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accountSID = accountSID
	c.authToken = authToken
	c.fromNumber = fromNumber
}

// callbackURL builds the inbound webhook URL used both as the <Gather action>
// and the StatusCallback. attempt is the re-prompt iteration (1-based); user is
// the called user's UUID so DTMF callbacks can attribute ack/silence.
func (c *Client) callbackURL(incidentNumber int64, attempt int, user, base string) string {
	path := fmt.Sprintf("/api/v1/twilio/callback?incident=%d&attempt=%d", incidentNumber, attempt)
	if user != "" {
		path += "&user=" + user
	}
	if base == "" {
		return path
	}
	return base + path
}

func (c *Client) Call(ctx context.Context, to string, incidentNumber int64, level int, opts notification.CallOptions) (string, error) {
	if to == "" {
		return "", errors.New("twilio: empty recipient number")
	}

	// Snapshot mutable config under the read lock so concurrent Reconfigure/
	// SetCallbackBaseURL/SetDisabled cannot tear a single call.
	c.mu.RLock()
	accountSID := c.accountSID
	authToken := c.authToken
	fromNumber := c.fromNumber
	baseURL := c.baseURL
	callbackBase := c.callbackBaseURL
	httpClient := c.httpClient
	enabled := !c.disabled && accountSID != "" && authToken != "" && fromNumber != ""
	c.mu.RUnlock()

	if !enabled {
		return "", errors.New("twilio not configured")
	}

	user := ""
	if opts.UserID != nil {
		user = opts.UserID.String()
	}
	gatherAction := c.callbackURL(incidentNumber, 1, user, callbackBase)
	twiml := buildTwiML(incidentNumber, level, opts.Title, gatherAction)
	callURL := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Calls.json", baseURL, accountSID)

	data := url.Values{}
	data.Set("To", to)
	data.Set("From", fromNumber)
	data.Set("Twiml", twiml)
	// StatusCallback fires only on terminal call status; no attempt or user
	// query params since no DTMF will be present on that hit.
	data.Set("StatusCallback", c.callbackURL(incidentNumber, 0, user, callbackBase))

	logger.Info("initiating Twilio voice call", "component", "twilio", "to", to, "incident_number", incidentNumber)

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(accountSID+":"+authToken))
	headers := map[string]string{
		"Authorization": auth,
		"Content-Type":  "application/x-www-form-urlencoded",
	}

	statusCode, body, err := httpclient.DoJSON(ctx, httpClient, http.MethodPost, callURL, headers, strings.NewReader(data.Encode()))
	if err != nil {
		logger.Warn("twilio call request failed", "component", "twilio", "to", to, "incident_number", incidentNumber, "error", err)
		return "", fmt.Errorf("twilio call request failed: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		logger.Warn("twilio API returned non-success status", "component", "twilio", "to", to, "incident_number", incidentNumber, "status", statusCode)
		return "", fmt.Errorf("twilio API returned status %d", statusCode)
	}

	var result struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode twilio response: %w", err)
	}
	return result.SID, nil
}
