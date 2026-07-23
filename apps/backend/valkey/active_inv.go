package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
)

// ActiveInvestigation describes an in-flight investigation for cross-agent
// awareness. The struct is intentionally compact; it only carries what we
// want to surface in the CONCURRENT INVESTIGATIONS prompt block.
type ActiveInvestigation struct {
	InvestigationID string    `json:"investigation_id"`
	AgentID         string    `json:"agent_id,omitempty"`
	AgentType       string    `json:"agent_type,omitempty"`
	Severity        string    `json:"severity,omitempty"`
	AlertName       string    `json:"alert_name,omitempty"`
	Namespace       string    `json:"namespace,omitempty"`
	StartedAt       time.Time `json:"started_at"`
}

// PeerFinding is the payload broadcast on the shared peer-findings channel
// whenever any agent publishes a finding, so every connected agent can
// surface it as a peer_finding SSE event in near real-time.
type PeerFinding struct {
	InvestigationID string            `json:"investigation_id"`
	AgentID         string            `json:"agent_id,omitempty"`
	AgentType       string            `json:"agent_type,omitempty"`
	Text            string            `json:"text"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

// peer-findings fanout channel. All agent SSE handlers subscribe to
// this single channel and forward matching peer findings to their clients.
const peerFindingsChannel = "alga:peer-findings"

// activeInvestigationKey is the hash storing per-investigation metadata.
func activeInvestigationKey(invID string) string {
	return "alga:active-inv:" + invID
}

// activeByLabelKey is the set of investigation IDs currently active for a
// given discriminator label (e.g. namespace=prod).
func activeByLabelKey(labelKey, labelValue string) string {
	return fmt.Sprintf("alga:active-inv-by:%s:%s", labelKey, labelValue)
}

// labelsIndexKey stores the list of discriminator labels used for cleanup
// when the investigation finishes.
func labelsIndexKey(invID string) string {
	return "alga:active-inv-labels:" + invID
}

// RegisterActiveInvestigation records that invID is currently in flight and
// indexes it by each discriminator label. TTL is applied to every key so
// stale entries clear themselves if the agent crashes without unregistering.
func (c *Client) RegisterActiveInvestigation(ctx context.Context, info ActiveInvestigation, discriminators map[string]string, ttl time.Duration) error {
	if c == nil || strings.TrimSpace(info.InvestigationID) == "" {
		return nil
	}
	if info.StartedAt.IsZero() {
		info.StartedAt = time.Now().UTC()
	}
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal active investigation: %w", err)
	}
	secs := int64(ttl.Seconds())
	if secs <= 0 {
		secs = 900
	}

	key := activeInvestigationKey(info.InvestigationID)
	if err := c.client.Do(ctx, c.client.B().Set().Key(key).Value(string(data)).ExSeconds(secs).Build()).Error(); err != nil {
		return fmt.Errorf("set active investigation: %w", err)
	}

	labelList := make([]string, 0, len(discriminators))
	for k, v := range discriminators {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		labelList = append(labelList, k+"="+v)
		setKey := activeByLabelKey(k, v)
		if err := c.client.Do(ctx, c.client.B().Sadd().Key(setKey).Member(info.InvestigationID).Build()).Error(); err != nil {
			return fmt.Errorf("sadd active-by-label: %w", err)
		}
		if err := c.client.Do(ctx, c.client.B().Expire().Key(setKey).Seconds(secs).Build()).Error(); err != nil {
			return fmt.Errorf("expire active-by-label: %w", err)
		}
	}
	if len(labelList) > 0 {
		lkey := labelsIndexKey(info.InvestigationID)
		if err := c.client.Do(ctx, c.client.B().Sadd().Key(lkey).Member(labelList...).Build()).Error(); err != nil {
			return fmt.Errorf("sadd labels index: %w", err)
		}
		if err := c.client.Do(ctx, c.client.B().Expire().Key(lkey).Seconds(secs).Build()).Error(); err != nil {
			return fmt.Errorf("expire labels index: %w", err)
		}
	}
	return nil
}

// UnregisterActiveInvestigation removes all traces of an active investigation.
// Safe to call multiple times; missing keys are ignored.
func (c *Client) UnregisterActiveInvestigation(ctx context.Context, invID string) error {
	if c == nil || strings.TrimSpace(invID) == "" {
		return nil
	}

	lkey := labelsIndexKey(invID)
	members, err := c.client.Do(ctx, c.client.B().Smembers().Key(lkey).Build()).AsStrSlice()
	if err == nil {
		for _, m := range members {
			parts := strings.SplitN(m, "=", 2)
			if len(parts) != 2 {
				continue
			}
			setKey := activeByLabelKey(parts[0], parts[1])
			_ = c.client.Do(ctx, c.client.B().Srem().Key(setKey).Member(invID).Build()).Error()
		}
	}
	_ = c.client.Do(ctx, c.client.B().Del().Key(lkey).Build()).Error()
	_ = c.client.Do(ctx, c.client.B().Del().Key(activeInvestigationKey(invID)).Build()).Error()
	return nil
}

// ListActiveByDiscriminators returns up to `limit` distinct active
// investigations that share at least one discriminator label/value with the
// caller. `excludeInvID` is skipped (typically the caller's own invID).
func (c *Client) ListActiveByDiscriminators(ctx context.Context, discriminators map[string]string, excludeInvID string, limit int) ([]ActiveInvestigation, error) {
	if c == nil || len(discriminators) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	seen := make(map[string]struct{})
	out := make([]ActiveInvestigation, 0, limit)
	for k, v := range discriminators {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		ids, err := c.client.Do(ctx, c.client.B().Smembers().Key(activeByLabelKey(k, v)).Build()).AsStrSlice()
		if err != nil {
			continue
		}
		for _, id := range ids {
			if slices.Contains([]string{excludeInvID}, id) {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			raw, err := c.client.Do(ctx, c.client.B().Get().Key(activeInvestigationKey(id)).Build()).ToString()
			if err != nil {
				continue
			}
			var info ActiveInvestigation
			if err := json.Unmarshal([]byte(raw), &info); err != nil {
				continue
			}
			out = append(out, info)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// PublishPeerFinding broadcasts a peer finding to every subscribed agent SSE
// handler on this cluster via the shared Valkey pub/sub channel.
func (c *Client) PublishPeerFinding(ctx context.Context, finding PeerFinding) error {
	if c == nil {
		return nil
	}
	if finding.CreatedAt.IsZero() {
		finding.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(finding)
	if err != nil {
		return fmt.Errorf("marshal peer finding: %w", err)
	}
	return c.client.Do(ctx, c.client.B().Publish().Channel(peerFindingsChannel).Message(string(data)).Build()).Error()
}

// SubscribePeerFindings subscribes to the peer-findings channel and invokes
// the handler for every received message. Blocks until ctx is cancelled.
func (c *Client) SubscribePeerFindings(ctx context.Context, handler func(PeerFinding)) error {
	if c == nil {
		return nil
	}
	return c.client.Receive(ctx, c.client.B().Subscribe().Channel(peerFindingsChannel).Build(), func(msg valkey.PubSubMessage) {
		var finding PeerFinding
		if err := json.Unmarshal([]byte(msg.Message), &finding); err != nil {
			return
		}
		handler(finding)
	})
}
