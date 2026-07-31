package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// backendClient is a minimal HTTP driver for the Alga backend operator API,
// with a cookie jar for the session and CSRF handling (X-CSRF-Token header
// mirroring the alga_csrf cookie on state-changing requests).
type backendClient struct {
	baseURL string
	http    *http.Client
}

func newBackendClient(baseURL string) (*backendClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &backendClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Jar: jar, Timeout: 30 * time.Second},
	}, nil
}

func (c *backendClient) csrfToken() string {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == "alga_csrf" {
			return ck.Value
		}
	}
	return ""
}

// do sends a JSON request and decodes a 2xx response body into out (when
// non-nil). It returns the HTTP status code; non-2xx responses are returned
// as errors carrying the response body for diagnostics.
func (c *backendClient) do(method, path string, body, out any) (int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		if tok := c.csrfToken(); tok != "" {
			req.Header.Set("X-CSRF-Token", tok)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("%s %s: decode response: %w", method, path, err)
		}
	}
	return resp.StatusCode, nil
}

// setupOrLogin bootstraps the initial admin via the setup wizard endpoint
// (idempotent: "setup already completed" is ignored) and then logs in.
func (c *backendClient) setupOrLogin(email, password, fullName string) error {
	setupReq := map[string]string{"email": email, "password": password, "full_name": fullName}
	if status, err := c.do(http.MethodPost, "/api/v1/setup", setupReq, nil); err != nil && status != http.StatusForbidden && status != http.StatusConflict {
		return fmt.Errorf("setup: %w", err)
	}
	loginReq := map[string]string{"email": email, "password": password}
	if _, err := c.do(http.MethodPost, "/api/v1/auth/login", loginReq, nil); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return nil
}

// mintAgentToken creates an agent bearer token with investigate+communicate
// capabilities. The plaintext token is returned exactly once by the API.
func (c *backendClient) mintAgentToken(name string) (id, token string, err error) {
	req := map[string]any{
		"name":         name,
		"capabilities": []string{"investigate", "communicate"},
	}
	var resp struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if _, err := c.do(http.MethodPost, "/api/v1/agent-tokens", req, &resp); err != nil {
		return "", "", err
	}
	if resp.ID == "" || resp.Token == "" {
		return "", "", fmt.Errorf("agent token response missing id or token")
	}
	return resp.ID, resp.Token, nil
}

// agentOnline reports whether the agent token has a live SSE connection,
// which is the scheduler's notion of dispatchability.
func (c *backendClient) agentOnline(id string) (bool, error) {
	var resp struct {
		Data []struct {
			ID     string `json:"id"`
			Online bool   `json:"online"`
		} `json:"data"`
	}
	if _, err := c.do(http.MethodGet, "/api/v1/agent-tokens", nil, &resp); err != nil {
		return false, err
	}
	for _, t := range resp.Data {
		if t.ID == id {
			return t.Online, nil
		}
	}
	return false, fmt.Errorf("agent token %s not found", id)
}

// createAlert creates a manual alert and returns its alert number and
// fingerprint. Manual alerts always get a unique "manual-<uuid>" fingerprint.
func (c *backendClient) createAlert(alertname, severity, message string) (int64, string, error) {
	req := map[string]string{
		"alertname": alertname,
		"severity":  severity,
		"message":   message,
	}
	var resp struct {
		Data struct {
			AlertNumber int64  `json:"alert_number"`
			Fingerprint string `json:"fingerprint"`
		} `json:"data"`
	}
	if _, err := c.do(http.MethodPost, "/api/v1/alerts", req, &resp); err != nil {
		return 0, "", err
	}
	if resp.Data.AlertNumber == 0 {
		return 0, "", fmt.Errorf("alert response missing alert_number")
	}
	return resp.Data.AlertNumber, resp.Data.Fingerprint, nil
}

// postThreadMessage posts a user message into the alert thread. mentions must
// contain "agent:<token-id>" for the agent to act on it (trigger "mention").
func (c *backendClient) postThreadMessage(alertNumber int64, message string, mentions []string) error {
	req := map[string]any{"message": message}
	if len(mentions) > 0 {
		req["mentions"] = mentions
	}
	path := fmt.Sprintf("/api/v1/alerts/%d/thread/messages", alertNumber)
	_, err := c.do(http.MethodPost, path, req, nil)
	return err
}

type threadMessage struct {
	Source    string    `json:"source"`
	Message   string    `json:"message"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// getThread returns the alert's investigation thread messages. A 404 (thread
// not created yet) is treated as an empty thread.
func (c *backendClient) getThread(alertNumber int64) ([]threadMessage, error) {
	var resp struct {
		Data struct {
			Items []threadMessage `json:"items"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/alerts/%d/thread", alertNumber)
	status, err := c.do(http.MethodGet, path, nil, &resp)
	if status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return resp.Data.Items, nil
}

func (c *backendClient) getAlertStatus(alertNumber int64) (string, error) {
	var resp struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/alerts/%d", alertNumber)
	if _, err := c.do(http.MethodGet, path, nil, &resp); err != nil {
		return "", err
	}
	return resp.Data.Status, nil
}
