package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
)

var ErrInvalidRule = errors.New("invalid automation rule")

var TriggerTypes = []string{
	"alert.created", "alert.acknowledged", "alert.resolved", "alert.reopened",
	"incident.created", "incident.status_changed",
}

type Condition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value,omitempty"`
}

type Action struct {
	Type             string `json:"type"`
	URL              string `json:"url,omitempty"`
	Message          string `json:"message,omitempty"`
	BearerToken      string `json:"bearer_token,omitempty"`
	HasBearerToken   bool   `json:"has_bearer_token,omitempty"`
	ClearBearerToken bool   `json:"clear_bearer_token,omitempty"`
}

type StoredAction struct {
	Type           string `json:"type"`
	URL            string `json:"url,omitempty"`
	Message        string `json:"message,omitempty"`
	BearerTokenEnc string `json:"bearer_token_enc,omitempty"`
}

func (a StoredAction) Public() Action {
	return Action{Type: a.Type, URL: a.URL, Message: a.Message, HasBearerToken: a.BearerTokenEnc != ""}
}

type Definition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	TriggerType string      `json:"trigger_type"`
	Conditions  []Condition `json:"conditions"`
	Actions     []Action    `json:"actions"`
	Enabled     bool        `json:"enabled"`
}

type Event struct {
	SchemaVersion  int               `json:"schema_version"`
	EventType      string            `json:"event_type"`
	EntityType     string            `json:"entity_type"`
	EntityID       string            `json:"entity_id"`
	EntityNumber   int64             `json:"entity_number"`
	Status         string            `json:"status"`
	PreviousStatus string            `json:"previous_status"`
	Severity       string            `json:"severity"`
	Priority       string            `json:"priority"`
	ServiceID      string            `json:"service_id"`
	Silenced       bool              `json:"silenced"`
	Labels         map[string]string `json:"labels"`
}

type HostPolicy struct {
	Allowed      []string
	PrivateHosts []string
}

func HostPolicyFromEnv() HostPolicy {
	return HostPolicy{Allowed: csvHosts(os.Getenv("AUTOMATION_WEBHOOK_ALLOWED_HOSTS")), PrivateHosts: csvHosts(os.Getenv("AUTOMATION_WEBHOOK_PRIVATE_HOSTS"))}
}

func csvHosts(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if host := strings.ToLower(strings.TrimSpace(part)); host != "" {
			out = append(out, host)
		}
	}
	return out
}

func (p HostPolicy) ValidateURL(raw string) (*url.URL, error) {
	if len(raw) > 2048 {
		return nil, fmt.Errorf("%w: webhook URL exceeds 2048 bytes", ErrInvalidRule)
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("%w: webhook URL must be an HTTPS URL without userinfo or fragment", ErrInvalidRule)
	}
	host := strings.ToLower(u.Hostname())
	if !slices.Contains(p.Allowed, host) {
		return nil, fmt.Errorf("%w: webhook host is not approved", ErrInvalidRule)
	}
	if net.ParseIP(host) != nil {
		return nil, fmt.Errorf("%w: webhook host must be a DNS name", ErrInvalidRule)
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("%w: invalid webhook port", ErrInvalidRule)
		}
	}
	return u, nil
}

func (p HostPolicy) PermitIP(host string, ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	if ip.IsPrivate() {
		return slices.Contains(p.PrivateHosts, strings.ToLower(host)) && slices.Contains(p.Allowed, strings.ToLower(host))
	}
	// Additional non-public allocations are not covered by IsPrivate.
	for _, prefix := range blockedWebhookPrefixes() {
		if prefix.Contains(ip.Unmap()) {
			return false
		}
	}
	return ip.IsGlobalUnicast()
}

func blockedWebhookPrefixes() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("224.0.0.0/4"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("fc00::/7"),
	}
}

func Validate(d Definition, policy HostPolicy) error {
	if len(strings.TrimSpace(d.Name)) == 0 || len(d.Name) > 120 {
		return fmt.Errorf("%w: name must be 1–120 characters", ErrInvalidRule)
	}
	if len(d.Description) > 1000 || !slices.Contains(TriggerTypes, d.TriggerType) {
		return fmt.Errorf("%w: invalid description or trigger", ErrInvalidRule)
	}
	if len(d.Conditions) > 10 || len(d.Actions) == 0 || len(d.Actions) > 5 {
		return fmt.Errorf("%w: use at most 10 conditions and 1–5 actions", ErrInvalidRule)
	}
	for _, c := range d.Conditions {
		if !validField(c.Field) || !slices.Contains([]string{"equals", "not_equals", "contains", "exists"}, c.Operator) || len(c.Value) > 500 {
			return fmt.Errorf("%w: invalid condition", ErrInvalidRule)
		}
		if c.Operator != "exists" && strings.TrimSpace(c.Value) == "" {
			return fmt.Errorf("%w: condition value is required", ErrInvalidRule)
		}
	}
	for _, a := range d.Actions {
		switch a.Type {
		case "webhook":
			if _, err := policy.ValidateURL(a.URL); err != nil {
				return err
			}
			if len(a.BearerToken) > 4096 || a.Message != "" || (a.BearerToken != "" && a.ClearBearerToken) {
				return fmt.Errorf("%w: invalid webhook action", ErrInvalidRule)
			}
		case "incident_note":
			if !strings.HasPrefix(d.TriggerType, "incident.") || strings.TrimSpace(a.Message) == "" || len(a.Message) > 1000 || a.URL != "" || a.BearerToken != "" {
				return fmt.Errorf("%w: invalid incident note action", ErrInvalidRule)
			}
		default:
			return fmt.Errorf("%w: unsupported action", ErrInvalidRule)
		}
	}
	return nil
}

func validField(field string) bool {
	if slices.Contains([]string{"status", "previous_status", "severity", "priority", "service_id", "silenced"}, field) {
		return true
	}
	key, ok := strings.CutPrefix(field, "label.")
	return ok && len(key) > 0 && len(key) <= 100 && !strings.ContainsAny(key, " \t\n")
}

func Match(payload json.RawMessage, conditions []Condition) bool {
	var event Event
	if err := json.Unmarshal(payload, &event); err != nil {
		return false
	}
	// Existing alert suppression semantics apply to automation as well:
	// silenced alerts are stored for history but must not dispatch actions.
	if event.EntityType == "alert" && event.Silenced {
		return false
	}
	for _, c := range conditions {
		var actual string
		var present bool
		switch c.Field {
		case "status":
			actual, present = event.Status, event.Status != ""
		case "previous_status":
			actual, present = event.PreviousStatus, event.PreviousStatus != ""
		case "severity":
			actual, present = event.Severity, event.Severity != ""
		case "priority":
			actual, present = event.Priority, event.Priority != ""
		case "service_id":
			actual, present = event.ServiceID, event.ServiceID != ""
		case "silenced":
			actual, present = fmt.Sprint(event.Silenced), true
		default:
			key, _ := strings.CutPrefix(c.Field, "label.")
			actual, present = event.Labels[key]
		}
		var matched bool
		switch c.Operator {
		case "exists":
			matched = present
		case "equals":
			matched = present && actual == c.Value
		case "not_equals":
			matched = !present || actual != c.Value
		case "contains":
			matched = present && strings.Contains(actual, c.Value)
		}
		if !matched {
			return false
		}
	}
	return true
}
