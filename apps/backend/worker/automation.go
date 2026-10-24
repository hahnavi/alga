package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"alga/automation"
	"alga/crypto"
	"alga/logger"
	"alga/store"
)

type AutomationWorker struct {
	store   store.AutomationStore
	audit   store.AuditStore
	policy  automation.HostPolicy
	client  *http.Client
	keyring *crypto.Keyring
	ticks   int
}

func (w *AutomationWorker) SetAuditStore(audit store.AuditStore) { w.audit = audit }

func NewAutomationWorker(s store.AutomationStore, policy automation.HostPolicy) *AutomationWorker {
	w := &AutomationWorker{store: s, policy: policy, keyring: crypto.Default()}
	w.client = &http.Client{
		Timeout:       12 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{Proxy: nil, MaxIdleConns: 20, MaxIdleConnsPerHost: 4,
			IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second,
			DialContext: w.dialApproved},
	}
	return w
}

func (w *AutomationWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *AutomationWorker) tick(ctx context.Context) {
	if w.store == nil {
		return
	}
	w.ticks++
	if w.ticks%3600 == 0 {
		pruneCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		if err := w.store.PruneHistory(pruneCtx, time.Now().UTC().Add(-90*24*time.Hour)); err != nil {
			logger.Warn("automation history prune failed", "component", "automation-worker", "error", err)
		}
		cancel()
	}
	for range 100 {
		processed, err := w.store.DispatchOne(ctx)
		if err != nil {
			logger.Error("automation event dispatch failed", "component", "automation-worker", "error", err)
			break
		}
		if !processed {
			break
		}
	}
	for range 20 {
		claim, err := w.store.ClaimRun(ctx)
		if err != nil {
			logger.Error("automation run claim failed", "component", "automation-worker", "error", err)
			break
		}
		if claim == nil {
			break
		}
		w.execute(ctx, *claim)
	}
}

func (w *AutomationWorker) execute(ctx context.Context, claim store.ClaimedAutomationRun) {
	if claim.ActionIndex >= len(claim.Actions) || claim.ActionIndex < 0 {
		w.fail(ctx, claim, "invalid_action_index", nil, false)
		return
	}
	action := claim.Actions[claim.ActionIndex]
	switch action.Type {
	case "incident_note":
		if err := w.store.CompleteAction(ctx, claim, nil, true); err != nil {
			if !errors.Is(err, store.ErrAutomationClaimLost) {
				w.fail(ctx, claim, "incident_note_failed", nil, false)
			}
		} else {
			w.auditSuccess(claim, action.Type)
		}
	case "webhook":
		status, code, retry := w.sendWebhook(ctx, claim, action)
		if code != "" {
			w.fail(ctx, claim, code, status, retry)
			return
		}
		if err := w.store.CompleteAction(ctx, claim, status, false); err != nil {
			if !errors.Is(err, store.ErrAutomationClaimLost) {
				logger.Error("automation webhook outcome could not be persisted", "component", "automation-worker", "run_id", claim.ID, "error", err)
			}
		} else {
			w.auditSuccess(claim, action.Type)
		}
	default:
		w.fail(ctx, claim, "unsupported_action", nil, false)
	}
}

func (w *AutomationWorker) auditSuccess(claim store.ClaimedAutomationRun, actionType string) {
	if w.audit == nil {
		return
	}
	w.audit.Log(store.AuditAutomationActionSucceeded, nil, "automation", "", "", true,
		map[string]any{"run_id": claim.ID.String(), "rule_id": claim.RuleID.String(), "action_index": claim.ActionIndex, "action_type": actionType})
}

func (w *AutomationWorker) fail(ctx context.Context, claim store.ClaimedAutomationRun, code string, status *int, retry bool) {
	backoff := []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 30 * time.Minute}
	stage := min(max(claim.Attempt-1, 0), len(backoff)-1)
	if err := w.store.FailAction(ctx, claim, code, status, retry, time.Now().UTC().Add(backoff[stage])); err != nil {
		if !errors.Is(err, store.ErrAutomationClaimLost) {
			logger.Error("automation failure could not be persisted", "component", "automation-worker", "run_id", claim.ID, "error", err)
		}
		return
	}
	if w.audit != nil {
		w.audit.Log(store.AuditAutomationActionFailed, nil, "automation", "", "", false,
			map[string]any{"run_id": claim.ID.String(), "rule_id": claim.RuleID.String(), "action_index": claim.ActionIndex, "error_code": code, "retry": retry})
	}
	logger.Warn("automation action failed", "component", "automation-worker", "run_id", claim.ID, "action_index", claim.ActionIndex, "code", code, "retry", retry)
}

func (w *AutomationWorker) sendWebhook(ctx context.Context, claim store.ClaimedAutomationRun, action automation.StoredAction) (*int, string, bool) {
	if _, err := w.policy.ValidateURL(action.URL); err != nil {
		return nil, "destination_not_approved", false
	}
	var event automation.Event
	if err := json.Unmarshal(claim.EventPayload, &event); err != nil {
		return nil, "invalid_event_payload", false
	}
	body, err := json.Marshal(struct {
		EventID     string           `json:"event_id"`
		RunID       string           `json:"run_id"`
		RuleID      string           `json:"rule_id"`
		ActionIndex int              `json:"action_index"`
		Event       automation.Event `json:"event"`
	}{EventID: claim.EventID.String(), RunID: claim.ID.String(), RuleID: claim.RuleID.String(), ActionIndex: claim.ActionIndex, Event: event})
	if err != nil {
		return nil, "encode_failed", false
	}
	requestCtx, cancel := context.WithTimeout(ctx, 11*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, action.URL, bytes.NewReader(body))
	if err != nil {
		return nil, "invalid_destination", false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", claim.ID.String()+"-"+strconv.Itoa(claim.ActionIndex))
	req.Header.Set("User-Agent", "Alga-Automation/1")
	if action.BearerTokenEnc != "" {
		token, err := w.keyring.DecryptString(action.BearerTokenEnc)
		if err != nil {
			return nil, "credential_unavailable", false
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, "network_error", true
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("automation response close failed", "component", "automation-worker", "error", err)
		}
	}()
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)); err != nil {
		return nil, "response_read_error", true
	}
	status := resp.StatusCode
	if status >= 200 && status < 300 {
		return &status, "", false
	}
	if status == http.StatusTooManyRequests || status >= 500 {
		return &status, "remote_retryable", true
	}
	return &status, "remote_rejected", false
}

func (w *AutomationWorker) dialApproved(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("automation destination: %w", err)
	}
	// The hostname must still be configured when the connection is made; a
	// saved rule is not an authorization to use a host after policy changes.
	if _, err := w.policy.ValidateURL((&url.URL{Scheme: "https", Host: address}).String()); err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve automation destination: %w", err)
	}
	if len(addresses) == 0 {
		return nil, errors.New("automation destination has no address")
	}
	for _, ip := range addresses {
		if !w.policy.PermitIP(host, ip) {
			return nil, errors.New("automation destination resolves to a blocked address")
		}
	}
	// Pin the validated address so no second DNS lookup can change the target.
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
}
