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

// mintAgentToken creates an agent bearer token with the given capabilities.
// The plaintext token is returned exactly once by the API.
func (c *backendClient) mintAgentToken(name string, capabilities ...string) (id, token string, err error) {
	if len(capabilities) == 0 {
		capabilities = []string{"investigate", "communicate"}
	}
	req := map[string]any{
		"name":         name,
		"capabilities": capabilities,
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

// getInvestigationOutcome returns the root cause and resolution recorded on
// the alert's current investigation summary (set via the set_outcome tool).
func (c *backendClient) getInvestigationOutcome(alertNumber int64) (rootCause, resolution string, err error) {
	var resp struct {
		Data struct {
			AlertInvestigation struct {
				Summary struct {
					RootCause  string `json:"root_cause"`
					Resolution string `json:"resolution"`
				} `json:"summary"`
			} `json:"alert_investigation"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/alerts/%d", alertNumber)
	if _, err := c.do(http.MethodGet, path, nil, &resp); err != nil {
		return "", "", err
	}
	return resp.Data.AlertInvestigation.Summary.RootCause, resp.Data.AlertInvestigation.Summary.Resolution, nil
}

// findIncidentByTitle searches incidents and returns the incident number of
// the first item whose title contains the given marker.
func (c *backendClient) findIncidentByTitle(marker string) (int64, bool, error) {
	var resp struct {
		Data struct {
			Items []struct {
				IncidentNumber int64  `json:"incident_number"`
				Title          string `json:"title"`
			} `json:"items"`
		} `json:"data"`
	}
	path := "/api/v1/incidents?search=" + url.QueryEscape(marker)
	if _, err := c.do(http.MethodGet, path, nil, &resp); err != nil {
		return 0, false, err
	}
	for _, it := range resp.Data.Items {
		if strings.Contains(it.Title, marker) {
			return it.IncidentNumber, true, nil
		}
	}
	return 0, false, nil
}

// incidentAlertNumbers returns the alert numbers linked to an incident.
func (c *backendClient) incidentAlertNumbers(incidentNumber int64) ([]int64, error) {
	var resp struct {
		Data []struct {
			AlertNumber int64 `json:"alert_number"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/incidents/%d/alerts", incidentNumber)
	if _, err := c.do(http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	nums := make([]int64, 0, len(resp.Data))
	for _, a := range resp.Data {
		nums = append(nums, a.AlertNumber)
	}
	return nums, nil
}

func (c *backendClient) getAlertStatus(alertNumber int64) (string, error) {
	var resp struct {
		Data struct {
			Alert struct {
				Status string `json:"status"`
			} `json:"alert"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/alerts/%d", alertNumber)
	if _, err := c.do(http.MethodGet, path, nil, &resp); err != nil {
		return "", err
	}
	if resp.Data.Alert.Status == "" {
		return "", fmt.Errorf("alert %d response missing status", alertNumber)
	}
	return resp.Data.Alert.Status, nil
}

// --- Incident helpers ---

// createIncident creates an incident via the operator API and returns its
// incident number.
func (c *backendClient) createIncident(title, severity string) (int64, error) {
	req := map[string]any{
		"title":    title,
		"severity": severity,
	}
	var resp struct {
		Data struct {
			IncidentNumber int64 `json:"incident_number"`
		} `json:"data"`
	}
	if _, err := c.do(http.MethodPost, "/api/v1/incidents", req, &resp); err != nil {
		return 0, err
	}
	if resp.Data.IncidentNumber == 0 {
		return 0, fmt.Errorf("incident response missing incident_number")
	}
	return resp.Data.IncidentNumber, nil
}

// acknowledgeIncident transitions an incident from detected to active.
func (c *backendClient) acknowledgeIncident(incidentNumber int64) error {
	path := fmt.Sprintf("/api/v1/incidents/%d/acknowledge", incidentNumber)
	_, err := c.do(http.MethodPost, path, nil, nil)
	return err
}

// getIncidentStatus returns the current status of an incident.
func (c *backendClient) getIncidentStatus(incidentNumber int64) (string, error) {
	var resp struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/incidents/%d", incidentNumber)
	if _, err := c.do(http.MethodGet, path, nil, &resp); err != nil {
		return "", err
	}
	if resp.Data.Status == "" {
		return "", fmt.Errorf("incident %d response missing status", incidentNumber)
	}
	return resp.Data.Status, nil
}

// postIncidentThreadMessage posts a message into the incident's investigation
// thread. mentions must contain "agent:<token-id>" for the agent to act.
func (c *backendClient) postIncidentThreadMessage(incidentNumber int64, message string, mentions []string) error {
	req := map[string]any{"message": message}
	if len(mentions) > 0 {
		req["mentions"] = mentions
	}
	path := fmt.Sprintf("/api/v1/incidents/%d/thread/messages", incidentNumber)
	_, err := c.do(http.MethodPost, path, req, nil)
	return err
}

// getIncidentThread returns the incident's investigation thread messages.
func (c *backendClient) getIncidentThread(incidentNumber int64) ([]threadMessage, error) {
	var resp struct {
		Data struct {
			Items []threadMessage `json:"items"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/incidents/%d/thread", incidentNumber)
	status, err := c.do(http.MethodGet, path, nil, &resp)
	if status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return resp.Data.Items, nil
}

type icsRole struct {
	RoleType     string `json:"role_type"`
	AssigneeType string `json:"assignee_type"`
	AgentTokenID string `json:"agent_token_id"`
	AgentName    string `json:"agent_name"`
	Status       string `json:"status"`
}

// listICSRoles returns the ICS role assignments for an incident.
func (c *backendClient) listICSRoles(incidentNumber int64) ([]icsRole, error) {
	var resp struct {
		Data []icsRole `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/incidents/%d/ics/roles", incidentNumber)
	if _, err := c.do(http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// hasActiveRole reports whether an agent holds an active ICS role of the given
// type on the incident.
func (c *backendClient) hasActiveRole(incidentNumber int64, roleType, agentTokenID string) (bool, error) {
	roles, err := c.listICSRoles(incidentNumber)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r.Status == "active" && r.RoleType == roleType && r.AgentTokenID == agentTokenID {
			return true, nil
		}
	}
	return false, nil
}

type coordinationTask struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	AssigneeRole string         `json:"assignee_role"`
	Status       string         `json:"status"`
	Goal         string         `json:"goal"`
	Result       map[string]any `json:"result"`
}

// listCoordinationTasks returns the coordination tasks for an incident.
func (c *backendClient) listCoordinationTasks(incidentNumber int64) ([]coordinationTask, error) {
	var resp struct {
		Data []coordinationTask `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/incidents/%d/coordination/tasks", incidentNumber)
	if _, err := c.do(http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// findCoordinationTask returns the first coordination task matching the given
// status (empty matches any).
func (c *backendClient) findCoordinationTask(incidentNumber int64, status string) (*coordinationTask, error) {
	tasks, err := c.listCoordinationTasks(incidentNumber)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		if status == "" || tasks[i].Status == status {
			return &tasks[i], nil
		}
	}
	return nil, nil
}
