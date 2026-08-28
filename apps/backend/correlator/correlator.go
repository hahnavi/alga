package correlator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/valkey-io/valkey-go"

	"alga/logger"
	"alga/metrics"
	"alga/rabbitmq"
	"alga/store"
	vkclient "alga/valkey"
)

var errCooldownStale = errors.New("cooldown points to terminal investigation")

// delCooldownKey drops a cooldown/correlation key from Valkey. Del failures are
// not user-visible (the next sweep re-attempts), but we log them at debug so a
// persistent Valkey issue is observable. Formerly `_ = ...Error()` discards.
func delCooldownKey(ctx context.Context, c *vkclient.Client, key, reason string) {
	if c == nil {
		return
	}
	if err := c.Del(ctx, key); err != nil {
		logger.DebugCtx(ctx, "Correlator: valkey Del failed", "component", "correlator", "key", key, "reason", reason, "error", err)
	}
}

// InvestigatePublisher is the rabbitmq.Publisher subset the correlator uses.
type InvestigatePublisher interface {
	PublishInvestigation(ctx context.Context, msg rabbitmq.InvestigateMessage) error
	PublishTriage(ctx context.Context, msg rabbitmq.TriageMessage) error
}

type IncidentPublisher interface {
	PublishIncident(ctx context.Context, msg rabbitmq.IncidentMessage) error
}

// AgentNotifier forwards supplementary alert notifications to an active agent.
// Implementations must be safe for concurrent use.
type AgentNotifier interface {
	ForwardToAgent(agentIDHex, investigationID, senderID, senderName, message string) error
}

// SSEPublisher is invoked when a cooldown-merge or correlated flush should
// surface as a real-time investigation_update event. Optional.
type SSEPublisher interface {
	PublishInvestigationEvent(eventType string, data map[string]any)
}

// Config tunes the bounded correlation window.
type Config struct {
	// Window is how long the correlator buffers alerts for the same
	// CorrelationKey before flushing them as one InvestigateMessage.
	// Set to 0 to flush each alert immediately.
	Window time.Duration
	// CooldownTTL is how long a (key -> investigation_id) cooldown row
	// survives. Subsequent alerts during the cooldown are appended to the
	// existing investigation rather than opening a new window.
	CooldownTTL time.Duration
}

type cooldownEntry struct {
	InvestigationID string `json:"investigation_id"`
}

// Correlator buffers per-key alerts in Valkey LISTs, then flushes them as a
// single InvestigateMessage either when the window expires or on Stop().
type Correlator struct {
	vkClient              *vkclient.Client
	publisher             InvestigatePublisher
	incidentPublisher     IncidentPublisher
	alertInvestigationStr store.AlertInvestigationStore
	sseBroker             SSEPublisher
	agentNotifier         AgentNotifier

	cfg              Config
	triageEnabled    bool
	correlationRules map[string]CorrelationRule
	suppressionRules []SuppressionRule

	appendScript *valkey.Lua
	drainScript  *valkey.Lua

	mu          sync.Mutex
	flushTimers map[string]*time.Timer

	wg     sync.WaitGroup
	stopCh chan struct{}
}

// appendScript pushes the alert JSON onto the per-key list and, when this is
// the first element, sets the TTL = window. Returns the new list length so
// the caller knows whether it just opened a window.
//
// KEYS[1] = list key (alga:corr:<key>)
// ARGV[1] = json payload
// ARGV[2] = window seconds (only used when len == 1)
const appendScript = `local n = redis.call('RPUSH', KEYS[1], ARGV[1])
if n == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return n`

// drainScript atomically returns the full pending list and deletes it. Used
// by both the timer flush and Stop() to guarantee no other replica's flush
// timer can publish duplicates.
//
// KEYS[1] = list key
const drainScript = `local items = redis.call('LRANGE', KEYS[1], 0, -1)
redis.call('DEL', KEYS[1])
return items`

func corrListKey(k string) string  { return "alga:corr:" + k }
func cooldownKey(k string) string  { return "alga:cooldown:" + k }
func flushLockKey(k string) string { return "alga:corr-lock:" + k }

// NewCorrelator builds a correlator. Pass cfg.Window=0 to flush immediately.
func NewCorrelator(vkClient *vkclient.Client, publisher InvestigatePublisher, cfg Config) *Correlator {
	if cfg.CooldownTTL <= 0 {
		cfg.CooldownTTL = 30 * time.Minute
	}
	c := &Correlator{
		vkClient:     vkClient,
		publisher:    publisher,
		cfg:          cfg,
		flushTimers:  make(map[string]*time.Timer),
		stopCh:       make(chan struct{}),
		appendScript: vkclient.NewLuaScript(appendScript),
		drainScript:  vkclient.NewLuaScript(drainScript),
	}
	return c
}

func (c *Correlator) SetAlertInvestigationStore(s store.AlertInvestigationStore) {
	c.alertInvestigationStr = s
}

func (c *Correlator) SetIncidentPublisher(p IncidentPublisher) {
	c.incidentPublisher = p
}

// SetSSEBroker wires real-time SSE for cooldown-merge / window-flush updates.
func (c *Correlator) SetSSEBroker(b SSEPublisher) { c.sseBroker = b }

// SetAgentNotifier wires agent notification for merged alerts during active investigations.
func (c *Correlator) SetAgentNotifier(n AgentNotifier) { c.agentNotifier = n }

// SetTriageEnabled enables the triage pipeline. When enabled, flushInvestigation
// publishes to the triage exchange instead of directly to investigate.
func (c *Correlator) SetTriageEnabled(enabled bool) { c.triageEnabled = enabled }

func (c *Correlator) SetCorrelationRules(rules map[string]CorrelationRule) {
	c.correlationRules = rules
}
func (c *Correlator) SetSuppressionRules(rules []SuppressionRule) { c.suppressionRules = rules }

// Window returns the configured window duration (0 = immediate flush).
func (c *Correlator) Window() time.Duration { return c.cfg.Window }

// effectiveWindow resolves the buffering window for an alert: a matched rule
// with Window > 0 (seconds) overrides the global window for its alertname;
// the global window (possibly 0 = immediate flush) is the fallback.
func (c *Correlator) effectiveWindow(rule *CorrelationRule) time.Duration {
	if rule != nil && rule.Window > 0 {
		return time.Duration(rule.Window) * time.Second
	}
	return c.cfg.Window
}

// ProcessAlert handles a single alert. It either appends to an existing
// investigation (cooldown hit), buffers in the window, or publishes a new
// investigation immediately when the effective window is 0.
func (c *Correlator) ProcessAlert(ctx context.Context, alert rabbitmq.CorrelatedAlert) error {
	if c.vkClient == nil {
		return errors.New("valkey client not available for correlation")
	}
	metrics.CorrelatorAlertsTotal.Add(1)

	alertName := alert.Labels["alertname"]
	if IsSuppressed(alert.Labels, c.suppressionRules) {
		logger.InfoCtx(ctx, "Correlator: suppressed alert", "component", "correlator", "alertname", alertName, "fingerprint", alert.Fingerprint)
		return nil
	}

	var rule *CorrelationRule
	if c.correlationRules != nil {
		if r, ok := c.correlationRules[alertName]; ok {
			rule = &r
		}
	}
	key, _ := CorrelationKeyWithRules(alert.Labels, rule)
	window := c.effectiveWindow(rule)

	if invID, hit, err := c.checkCooldown(ctx, key); err != nil {
		metrics.CorrelatorFailClosedTotal.Add(1)
		return fmt.Errorf("cooldown lookup failed for key %s: %w", key, err)
	} else if hit {
		if err := c.appendToInvestigation(ctx, key, invID, alert); err != nil {
			if errors.Is(err, errCooldownStale) {
				if window <= 0 {
					return c.flushInvestigation(ctx, key, []rabbitmq.CorrelatedAlert{alert}, alert)
				}
				return c.bufferAlert(ctx, key, alert, window)
			}
			logger.ErrorCtx(ctx, "Correlator: failed to append alert to investigation", "component", "correlator", "fingerprint", alert.Fingerprint, "investigation_id", invID, "error", err)
			return err
		}
		return nil
	}

	if window <= 0 {
		return c.flushInvestigation(ctx, key, []rabbitmq.CorrelatedAlert{alert}, alert)
	}

	return c.bufferAlert(ctx, key, alert, window)
}

func (c *Correlator) checkCooldown(ctx context.Context, key string) (string, bool, error) {
	cd, err := c.vkClient.Do(ctx, c.vkClient.Builder().Get().Key(cooldownKey(key)).Build()).AsBytes()
	if err != nil {
		if errors.Is(err, valkey.Nil) {
			return "", false, nil
		}
		return "", false, err
	}
	if len(cd) == 0 {
		return "", false, nil
	}
	var entry cooldownEntry
	if err := json.Unmarshal(cd, &entry); err != nil || entry.InvestigationID == "" {
		return "", false, nil
	}
	return entry.InvestigationID, true, nil
}

func (c *Correlator) appendToInvestigation(ctx context.Context, key, investigationID string, alert rabbitmq.CorrelatedAlert) error {
	if c.alertInvestigationStr == nil {
		metrics.CorrelatorDroppedTotal.Add(1)
		logger.Warn("Correlator: cooldown hit but no alert investigation store; dropping alert", "component", "correlator", "fingerprint", alert.Fingerprint)
		return nil
	}

	inv, err := c.alertInvestigationStr.GetAlertInvestigation(ctx, investigationID)
	if err != nil {
		logger.WarnCtx(ctx, "Correlator: failed to look up cooldown investigation", "component", "correlator", "investigation_id", investigationID, "error", err)
		return fmt.Errorf("lookup cooldown investigation %s: %w", investigationID, err)
	}
	if inv == nil {
		delCooldownKey(ctx, c.vkClient, cooldownKey(key), "cooldown-missing-investigation")
		metrics.CorrelatorDroppedTotal.Add(1)
		logger.Warn("Correlator: cooldown investigation not found; clearing cooldown", "component", "correlator", "investigation_id", investigationID, "key", key)
		return errCooldownStale
	}

	if store.IsTerminalInvestigationStatus(inv.Status) {
		delCooldownKey(ctx, c.vkClient, cooldownKey(key), "cooldown-terminal-investigation")
		logger.InfoCtx(ctx, "Correlator: cooldown investigation is terminal; clearing cooldown", "component", "correlator", "investigation_id", investigationID, "status", inv.Status, "key", key)
		return errCooldownStale
	}

	if err := c.alertInvestigationStr.AppendAlertsToAlertInvestigation(ctx, investigationID, []rabbitmq.CorrelatedAlert{alert}); err != nil {
		logger.WarnCtx(ctx, "Failed to merge alert into existing investigation", "component", "correlator", "fingerprint", alert.Fingerprint, "investigation_id", investigationID, "error", err)
		return fmt.Errorf("merge alert into investigation: %w", err)
	}
	metrics.CorrelatorMergedTotal.Add(1)
	logger.InfoCtx(ctx, "Linked alert into existing investigation", "component", "correlator", "fingerprint", alert.Fingerprint, "investigation_id", investigationID, "key", key)
	c.publishMergeSSE(investigationID, alert)
	c.notifyAgentOfMergedAlert(ctx, investigationID, alert)
	return nil
}

func (c *Correlator) publishMergeSSE(investigationID string, alert rabbitmq.CorrelatedAlert) {
	if c.sseBroker == nil {
		return
	}
	c.sseBroker.PublishInvestigationEvent("investigation_update", map[string]any{
		"alert_investigation_id": investigationID,
		"merged_alert":           alert.Fingerprint,
		"merged_at":              time.Now().UTC(),
	})
}

func (c *Correlator) notifyAgentOfMergedAlert(ctx context.Context, investigationID string, alert rabbitmq.CorrelatedAlert) {
	if c.agentNotifier == nil || c.alertInvestigationStr == nil {
		return
	}
	inv, err := c.alertInvestigationStr.GetAlertInvestigation(ctx, investigationID)
	if err != nil || inv == nil || inv.AgentID == "" {
		return
	}
	if inv.Status != "investigating" && inv.Status != "assigned" {
		return
	}
	alertName := alert.Labels["alertname"]
	if alertName == "" {
		alertName = alert.Fingerprint
	}
	var b strings.Builder
	b.WriteString("🔔 **New related alert merged into this investigation**\n\n")
	if alert.AlertNumber > 0 {
		fmt.Fprintf(&b, "### #%d %s\n", alert.AlertNumber, alertName)
		fmt.Fprintf(&b, "**Alert ID:** #%d\n", alert.AlertNumber)
	} else {
		fmt.Fprintf(&b, "### %s\n", alertName)
	}
	fmt.Fprintf(&b, "**Status:** %s\n", alert.Status)
	if alert.StartsAt != "" {
		fmt.Fprintf(&b, "**Firing since:** %s\n", alert.StartsAt)
	}
	if summary, ok := alert.Annotations["summary"]; ok && summary != "" {
		fmt.Fprintf(&b, "**Summary:** %s\n", summary)
	}
	if desc, ok := alert.Annotations["description"]; ok && desc != "" {
		fmt.Fprintf(&b, "**Description:** %s\n", desc)
	}
	b.WriteString("**Labels:**\n")
	for k, v := range alert.Labels {
		fmt.Fprintf(&b, "- %s: %s\n", k, v)
	}
	fmt.Fprintf(&b, "\nUse `alga_resolve_alert` with fingerprint `%s` when ready.", alert.Fingerprint)
	if err := c.agentNotifier.ForwardToAgent(inv.AgentID, investigationID, "system", "System", b.String()); err != nil {
		logger.WarnCtx(ctx, "Correlator: failed to notify agent of merged alert", "component", "correlator", "agent_id", inv.AgentID, "fingerprint", alert.Fingerprint, "investigation_id", investigationID, "error", err)
	}
}

func (c *Correlator) bufferAlert(ctx context.Context, key string, alert rabbitmq.CorrelatedAlert, window time.Duration) error {
	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshal alert: %w", err)
	}
	winSecs := int64(window.Seconds())
	if winSecs <= 0 {
		winSecs = 1
	}
	script := c.appendScript
	resp := script.Exec(ctx, c.vkClient.Client(),
		[]string{corrListKey(key)},
		[]string{string(payload), strconv.FormatInt(winSecs, 10)})
	if err := resp.Error(); err != nil {
		return fmt.Errorf("buffer alert: %w", err)
	}
	n, err := resp.AsInt64()
	if err != nil {
		return fmt.Errorf("buffer alert: unexpected response type: %w", err)
	}
	if n == 1 {
		// First alert in this window — open the timer.
		metrics.CorrelatorWindowOpenTotal.Add(1)
		metrics.CorrelatorWindowDepth.Add(1)
		c.scheduleFlush(key, window)
	}
	return nil
}

func (c *Correlator) scheduleFlush(key string, window time.Duration) {
	c.mu.Lock()
	if _, exists := c.flushTimers[key]; exists {
		c.mu.Unlock()
		return
	}
	c.wg.Add(1)
	t := time.AfterFunc(window, func() {
		defer c.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic in correlator flush callback", "error", r)
			}
		}()
		c.mu.Lock()
		delete(c.flushTimers, key)
		c.mu.Unlock()
		select {
		case <-c.stopCh:
			return
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.flushKey(ctx, key); err != nil {
			logger.Error("Correlator timer flush failed", "component", "correlator", "key", key, "error", err)
		}
	})
	c.flushTimers[key] = t
	c.mu.Unlock()
}

// flushKey drains the pending list for a key under a SETNX lock, then
// publishes a single InvestigateMessage.
func (c *Correlator) flushKey(ctx context.Context, key string) error {
	// SETNX flush lock so concurrent timers (multiple replicas) don't both
	// publish.
	got, err := c.vkClient.Do(ctx, c.vkClient.Builder().Set().
		Key(flushLockKey(key)).Value("1").Nx().ExSeconds(60).Build()).ToString()
	if err != nil {
		if errors.Is(err, valkey.Nil) {
			// Another replica is flushing; nothing to do.
			return nil
		}
		return fmt.Errorf("acquire flush lock: %w", err)
	}
	if got != "OK" {
		return nil
	}
	defer func() {
		delCooldownKey(context.Background(), c.vkClient, flushLockKey(key), "release-flush-lock")
	}()

	script := c.drainScript
	resp := script.Exec(ctx, c.vkClient.Client(), []string{corrListKey(key)}, nil)
	if err := resp.Error(); err != nil {
		return fmt.Errorf("drain list: %w", err)
	}
	items, err := resp.AsStrSlice()
	if err != nil {
		return fmt.Errorf("decode drained list: %w", err)
	}
	if len(items) == 0 {
		return nil
	}
	metrics.CorrelatorWindowDepth.Add(-1)
	alerts := make([]rabbitmq.CorrelatedAlert, 0, len(items))
	for _, raw := range items {
		var a rabbitmq.CorrelatedAlert
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			logger.WarnCtx(ctx, "Correlator: failed to decode buffered alert", "component", "correlator", "key", key, "error", err)
			continue
		}
		alerts = append(alerts, a)
	}
	if len(alerts) == 0 {
		return nil
	}
	metrics.CorrelatorFlushTotal.Add(1)
	return c.flushInvestigation(ctx, key, alerts, alerts[0])
}

// Flush iterates all pending correlation lists with SCAN (never KEYS) and
// drains them. Safe to call from shutdown paths.
func (c *Correlator) Flush(ctx context.Context) error {
	if c.vkClient == nil {
		return nil
	}
	var cursor uint64
	for {
		entry, err := c.vkClient.Do(ctx, c.vkClient.Builder().Scan().
			Cursor(cursor).Match("alga:corr:*").Count(200).Build()).AsScanEntry()
		if err != nil {
			return fmt.Errorf("scan correlation lists: %w", err)
		}
		for _, fullKey := range entry.Elements {
			if !strings.HasPrefix(fullKey, "alga:corr:") {
				continue
			}
			key := strings.TrimPrefix(fullKey, "alga:corr:")
			if err := c.flushKey(ctx, key); err != nil {
				logger.ErrorCtx(ctx, "Correlator: failed flushing on shutdown", "component", "correlator", "key", key, "error", err)
			}
		}
		cursor = entry.Cursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// Stop drains any open windows synchronously and cancels pending timers.
func (c *Correlator) Stop(ctx context.Context) {
	close(c.stopCh)
	c.mu.Lock()
	for k, t := range c.flushTimers {
		if t.Stop() {
			c.wg.Done()
		}
		delete(c.flushTimers, k)
	}
	c.mu.Unlock()
	if c.vkClient != nil {
		if err := c.Flush(ctx); err != nil {
			logger.WarnCtx(ctx, "Correlator.Stop: flush error", "component", "correlator", "error", err)
		}
	}
	c.wg.Wait()
}

func (c *Correlator) flushInvestigation(ctx context.Context, key string, alerts []rabbitmq.CorrelatedAlert, primaryAlert rabbitmq.CorrelatedAlert) error {
	severity := rabbitmq.DetermineAlertSeverity(alerts)

	// Re-check cooldown: it's possible a sibling flush wrote one between
	// our buffer-append and timer-fire. If so, merge into the existing
	// investigation instead of creating a new one.
	if invID, hit, err := c.checkCooldown(ctx, key); err == nil && hit && c.alertInvestigationStr != nil {
		inv, invErr := c.alertInvestigationStr.GetAlertInvestigation(ctx, invID)
		if invErr != nil {
			logger.WarnCtx(ctx, "Correlator flush: failed to look up cooldown investigation", "component", "correlator", "investigation_id", invID, "error", invErr)
		} else if inv == nil {
			delCooldownKey(ctx, c.vkClient, cooldownKey(key), "flush-cooldown-missing-investigation")
		} else {
			if store.IsTerminalInvestigationStatus(inv.Status) {
				delCooldownKey(ctx, c.vkClient, cooldownKey(key), "flush-cooldown-terminal-investigation")
			} else {
				if err := c.alertInvestigationStr.AppendAlertsToAlertInvestigation(ctx, invID, alerts); err != nil {
					logger.WarnCtx(ctx, "Correlator flush: append to existing investigation failed", "component", "correlator", "investigation_id", invID, "error", err)
				} else {
					metrics.CorrelatorMergedTotal.Add(int64(len(alerts)))
					c.publishMergeSSE(invID, primaryAlert)
					for _, a := range alerts {
						c.notifyAgentOfMergedAlert(ctx, invID, a)
					}
				}
				return nil
			}
		}
	}

	if c.alertInvestigationStr == nil {
		metrics.CorrelatorDroppedTotal.Add(int64(len(alerts)))
		return errors.New("alert investigation store is required to publish a new investigation")
	}

	// Belt-and-braces: even with cooldown, two replicas could both miss it
	// (race on cooldown SET) — also check the active investigation by
	// correlation_key as a final dedupe gate.
	if existing, err := c.alertInvestigationStr.GetActiveAlertInvestigationByCorrelationKey(ctx, key); err == nil && existing != nil {
		if err := c.alertInvestigationStr.AppendAlertsToAlertInvestigation(ctx, existing.AlertInvestigationID, alerts); err == nil {
			metrics.CorrelatorMergedTotal.Add(int64(len(alerts)))
			c.setCooldown(ctx, key, existing.AlertInvestigationID)
			c.publishMergeSSE(existing.AlertInvestigationID, primaryAlert)
			for _, a := range alerts {
				c.notifyAgentOfMergedAlert(ctx, existing.AlertInvestigationID, a)
			}
			return nil
		}
	}

	investigationID := uuid.New().String()

	dedupeKey := fmt.Sprintf("%s:%d", key, time.Now().UnixNano())

	if c.publisher != nil && c.triageEnabled {
		triageMsg := rabbitmq.TriageMessage{
			CorrelationKey:  key,
			Alerts:          alerts,
			Severity:        severity,
			RetryCount:      0,
			TraceID:         key,
			DedupeKey:       dedupeKey,
			InvestigationID: investigationID,
		}
		if err := c.publisher.PublishTriage(ctx, triageMsg); err != nil {
			return fmt.Errorf("publish triage: %w", err)
		}
		c.setCooldown(ctx, key, investigationID)
		metrics.CorrelatorPublishedTotal.Add(1)
		logger.InfoCtx(ctx, "Published triage for investigation", "component", "correlator",
			"investigation_id", investigationID, "alert_count", len(alerts), "severity", severity, "trace_id", key)
		return nil
	}

	msg := rabbitmq.InvestigateMessage{
		InvestigationID:         investigationID,
		InvestigationKind:       rabbitmq.InvestigationKindAlert,
		Alerts:                  alerts,
		Severity:                severity,
		CorrelationKey:          key,
		RetryCount:              0,
		TraceID:                 key,
		DedupeKey:               dedupeKey,
		PrimaryAlertFingerprint: primaryAlert.Fingerprint,
		PrimaryAlertNumber:      primaryAlert.AlertNumber,
	}

	if err := c.publisher.PublishInvestigation(ctx, msg); err != nil {
		return fmt.Errorf("publish investigation: %w", err)
	}

	if c.incidentPublisher != nil && severity == "critical" {
		incidentMsg := rabbitmq.IncidentMessage{
			DedupeKey:       dedupeKey + ":incident",
			TraceID:         key,
			InvestigationID: investigationID,
			Alerts:          alerts,
			CorrelationKey:  key,
			Severity:        severity,
			ImpactLevel:     "medium",
			TriageDecision:  "escalate",
			RetryCount:      0,
			MaxRetries:      3,
		}
		if err := c.incidentPublisher.PublishIncident(ctx, incidentMsg); err != nil {
			logger.WarnCtx(ctx, "Failed to publish incident message for investigation", "component", "correlator", "investigation_id", investigationID, "error", err)
		}
	}

	c.setCooldown(ctx, key, investigationID)
	metrics.CorrelatorPublishedTotal.Add(1)
	logger.InfoCtx(ctx, "Published investigation", "component", "correlator",
		"investigation_id", investigationID, "alert_count", len(alerts), "severity", severity, "trace_id", key)
	return nil
}

func (c *Correlator) setCooldown(ctx context.Context, key, investigationID string) {
	entry := cooldownEntry{InvestigationID: investigationID}
	data, err := json.Marshal(entry)
	if err != nil {
		logger.ErrorCtx(ctx, "Failed to marshal cooldown entry", "component", "correlator", "key", key, "error", err)
		return
	}
	ttlSec := int64(c.cfg.CooldownTTL.Seconds())
	if ttlSec <= 0 {
		ttlSec = int64((30 * time.Minute).Seconds())
	}
	if err := c.vkClient.Do(ctx, c.vkClient.Builder().Set().
		Key(cooldownKey(key)).Value(string(data)).ExSeconds(ttlSec).Build()).Error(); err != nil {
		logger.ErrorCtx(ctx, "Failed to set cooldown key", "component", "correlator", "key", cooldownKey(key), "error", err)
	}
}

// RemoveCooldown clears all correlator state for the given alert labels. Used
// when an operator manually closes a cooldown so the next firing alert opens
// a fresh investigation.
func (c *Correlator) RemoveCooldown(ctx context.Context, labels map[string]string) {
	if c == nil || c.vkClient == nil {
		return
	}
	key, _ := CorrelationKey(labels)
	for _, k := range []string{cooldownKey(key), corrListKey(key), flushLockKey(key)} {
		delCooldownKey(ctx, c.vkClient, k, "reset-keys")
	}
}
