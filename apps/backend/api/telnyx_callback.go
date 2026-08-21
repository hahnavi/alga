// Code moved from http.go; see git history.

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/store"
	"alga/strutil"
	"alga/telnyx"
)

// maxIVRGatherAttempts caps how many times a Telnyx caller is re-prompted for
// DTMF input before we give up and hang up. The first gather counts as attempt 1.
const maxIVRGatherAttempts = 2

func (s *Server) handleTelnyxCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	if s.telnyxClient == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "telnyx not configured")
		return
	}

	body, ok := s.telnyxClient.VerifyWebhook(r)
	if !ok {
		logger.Warn("Telnyx callback rejected: invalid signature")
		writeError(w, ErrorCodeForbidden, "invalid signature")
		return
	}

	var ev struct {
		Data struct {
			EventType     string `json:"event_type"`
			CallControlID string `json:"call_control_id"`
			Payload       struct {
				Digits      string `json:"digits"`
				ClientState string `json:"client_state"`
			} `json:"payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid payload")
		return
	}

	ctx := r.Context()
	incidentStr := r.URL.Query().Get("incident")
	levelStr := r.URL.Query().Get("level")
	userStr := r.URL.Query().Get("user")
	incidentNumber, err := strconv.ParseInt(incidentStr, 10, 64)
	if err != nil {
		incidentNumber = 0
	}
	level, err := strconv.Atoi(levelStr)
	if err != nil {
		level = 0
	}
	ccid := ev.Data.CallControlID

	switch ev.Data.EventType {
	case "call.initiated":
		logger.InfoCtx(ctx, "Telnyx call initiated", "component", "telnyx", "call_control_id", ccid, "incident", incidentStr)
		writeStatus(w, "initiated")
		return

	case "call.answered":
		if err := s.telnyxClient.Answer(ctx, ccid); err != nil {
			logger.WarnCtx(ctx, "Telnyx answer failed", "component", "telnyx", "call_control_id", ccid, "error", err)
		}
		// Carry (incident, level, attempt=1, user) statelessly via client_state
		// so the gather.ended webhook can re-prompt without server-side tracking.
		state := encodeIVRClientState(incidentStr, level, 1, userStr)
		brief := s.incidentBrief(ctx, incidentNumber)
		if err := s.telnyxClient.GatherUsingSpeak(ctx, ccid, telnyx.GatherText(incidentNumber, level, brief), state); err != nil {
			logger.WarnCtx(ctx, "Telnyx gather_using_speak failed", "component", "telnyx", "call_control_id", ccid, "error", err)
		}
		logger.InfoCtx(ctx, "Telnyx call answered, gathering digits", "component", "telnyx", "call_control_id", ccid, "incident_number", incidentNumber)
		writeStatus(w, "answered")
		return

	case "call.gather.ended":
		digits := ev.Data.Payload.Digits
		// client_state (if present) is the source of truth for incident/level/attempt;
		// fall back to query params for calls placed before client_state was added.
		stIncident, stLevel, attempt, stUser := decodeIVRClientState(ev.Data.Payload.ClientState)
		if stIncident != "" {
			incidentStr = stIncident
			if n, err := strconv.ParseInt(stIncident, 10, 64); err == nil {
				incidentNumber = n
			}
		}
		if stLevel != 0 {
			level = stLevel
		}
		if stUser != "" {
			userStr = stUser
		}

		switch digits {
		case "1":
			actorID, actorLabel := s.resolveVoiceActor(ctx, userStr)
			s.cancelEscalationForIncident(ctx, incidentStr, "acknowledged via phone")
			if actorID != nil || actorLabel != "" {
				s.recordVoiceTimelineEntry(ctx, incidentNumber, "acknowledged", "Incident acknowledged via Telnyx IVR (press 1)", actorID, actorLabel)
			}
			s.auditVoiceCallback(store.AuditVoiceAck, actorID, actorLabel, "telnyx", map[string]any{
				"incident_number": incidentNumber,
				"level":           level,
				"provider":        "telnyx",
			})
			logger.InfoCtx(ctx, "Incident acknowledged via Telnyx IVR", "component", "telnyx", "incident_number", incidentNumber, "call_control_id", ccid, "actor", actorLabel)
			if err := s.telnyxClient.Speak(ctx, ccid, telnyx.AcknowledgedText()); err != nil {
				logger.WarnCtx(ctx, "Telnyx speak failed", "component", "telnyx", "call_control_id", ccid, "error", err)
			}
			if err := s.telnyxClient.Hangup(ctx, ccid); err != nil {
				logger.WarnCtx(ctx, "Telnyx hangup failed", "component", "telnyx", "call_control_id", ccid, "error", err)
			}
		case "2":
			actorID, actorLabel := s.resolveVoiceActor(ctx, userStr)
			if s.vkClient != nil {
				hashKey := "alga:esc:" + incidentStr
				_ = s.vkClient.HSet(ctx, hashKey, "silenced_until", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
			}
			s.recordVoiceTimelineEntry(ctx, incidentNumber, "escalation_silenced", "Escalation silenced for 1 hour via Telnyx IVR (press 2)", actorID, actorLabel)
			s.auditVoiceCallback(store.AuditVoiceSilence, actorID, actorLabel, "telnyx", map[string]any{
				"incident_number": incidentNumber,
				"level":           level,
				"provider":        "telnyx",
			})
			logger.InfoCtx(ctx, "Escalation silenced for 1 hour via Telnyx IVR", "component", "telnyx", "incident_number", incidentNumber, "call_control_id", ccid, "actor", actorLabel)
			if err := s.telnyxClient.Speak(ctx, ccid, telnyx.SilencedText()); err != nil {
				logger.WarnCtx(ctx, "Telnyx speak failed", "component", "telnyx", "call_control_id", ccid, "error", err)
			}
			if err := s.telnyxClient.Hangup(ctx, ccid); err != nil {
				logger.WarnCtx(ctx, "Telnyx hangup failed", "component", "telnyx", "call_control_id", ccid, "error", err)
			}
		default:
			// No digit (or anything other than 1/2): re-prompt up to maxIVRGatherAttempts.
			if attempt < maxIVRGatherAttempts {
				next := attempt + 1
				state := encodeIVRClientState(incidentStr, level, next, userStr)
				if err := s.telnyxClient.GatherUsingSpeak(ctx, ccid, telnyx.PromptText(), state); err != nil {
					logger.WarnCtx(ctx, "Telnyx re-prompt gather_using_speak failed", "component", "telnyx", "call_control_id", ccid, "error", err)
					if err := s.telnyxClient.Hangup(ctx, ccid); err != nil {
						logger.WarnCtx(ctx, "Telnyx hangup failed", "component", "telnyx", "call_control_id", ccid, "error", err)
					}
				}
				logger.InfoCtx(ctx, "Telnyx IVR re-prompting", "component", "telnyx", "incident_number", incidentNumber, "call_control_id", ccid, "attempt", next)
			} else {
				logger.InfoCtx(ctx, "No action taken for Telnyx IVR after final attempt", "component", "telnyx", "incident_number", incidentNumber, "call_control_id", ccid, "digits", digits, "attempt", attempt)
				if err := s.telnyxClient.Hangup(ctx, ccid); err != nil {
					logger.WarnCtx(ctx, "Telnyx hangup failed", "component", "telnyx", "call_control_id", ccid, "error", err)
				}
			}
		}
		writeStatus(w, "gather_ended")
		return

	case "call.hangup":
		logger.InfoCtx(ctx, "Telnyx call hangup", "component", "telnyx", "call_control_id", ccid, "incident", incidentStr)
		writeStatus(w, "hangup")
		return

	default:
		logger.InfoCtx(ctx, "Telnyx unhandled event", "component", "telnyx", "event_type", ev.Data.EventType, "call_control_id", ccid)
		writeStatus(w, "unhandled")
		return
	}
}

// resolveVoiceActor maps the user query param (set by the outbound Call into
// the webhook URL) to a user ID + human-readable label for audit/timeline
// attribution. Returns zero values when the user cannot be resolved, in which
// case callers fall back to ActorType "system".
func (s *Server) resolveVoiceActor(ctx context.Context, userStr string) (*uuid.UUID, string) {
	if userStr == "" || s.userStore == nil {
		return nil, ""
	}
	uid, err := uuid.Parse(userStr)
	if err != nil {
		return nil, ""
	}
	user, err := s.userStore.GetByID(uid)
	if err != nil || user == nil {
		return &uid, ""
	}
	return &uid, user.DisplayName()
}

// incidentBrief returns a short, single-line truncation of the incident's Title
// for inclusion in the spoken IVR announcement. Returns "" when the incident
// cannot be resolved, so callers fall back to the menu-only announcement.
func (s *Server) incidentBrief(ctx context.Context, incidentNumber int64) string {
	if s.incidentStore == nil || incidentNumber <= 0 {
		return ""
	}
	inc, err := s.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil || inc == nil {
		return ""
	}
	return strutil.TruncateOneLine(inc.Title, incidentBriefMaxLen)
}

// incidentBriefMaxLen caps the spoken incident title so the announcement stays
// near a few seconds of speech (~3 words/sec → ~120 chars ≈ 6s).
const incidentBriefMaxLen = 120

// recordVoiceTimelineEntry writes an attributed timeline entry for a phone
// ack/silence. When actorID is present the entry is attributed to that user;
// otherwise it falls back to ActorType "system" with the provided label so the
// incident timeline still explains what happened.
func (s *Server) recordVoiceTimelineEntry(ctx context.Context, incidentNumber int64, eventType, message string, actorID *uuid.UUID, actorLabel string) {
	if s.incidentStore == nil || incidentNumber <= 0 {
		return
	}
	entry := &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      eventType,
		Message:        message,
	}
	if actorID != nil {
		entry.ActorID = actorID
		entry.ActorType = "user"
	} else {
		entry.ActorType = "system"
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("timeline entry goroutine panicked", "component", "telnyx-callback", "incident_number", incidentNumber, "panic", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := s.incidentStore.AddTimelineEntry(ctx, entry); err != nil {
			logger.WarnCtx(ctx, "failed to add incident timeline entry", "incident_number", incidentNumber, "event_type", eventType, "error", err)
		}
	}()
}

// encodeIVRClientState packs the IVR continuation context (incident id, level,
// attempt number, user id) into the opaque base64 token Telnyx echoes back on
// the next webhook. Plain "incident:level:attempt:user" keeps it debuggable.
func encodeIVRClientState(incident string, level, attempt int, user string) string {
	raw := incident + ":" + strconv.Itoa(level) + ":" + strconv.Itoa(attempt) + ":" + user
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// decodeIVRClientState is the inverse of encodeIVRClientState. Returns zero
// values when the token is absent or malformed.
func decodeIVRClientState(state string) (incident string, level, attempt int, user string) {
	if state == "" {
		return "", 0, 0, ""
	}
	raw, err := base64.StdEncoding.DecodeString(state)
	if err != nil {
		return "", 0, 0, ""
	}
	parts := splitClientState(string(raw))
	if len(parts) < 3 {
		return "", 0, 0, ""
	}
	incident = parts[0]
	if n, err := strconv.Atoi(parts[1]); err == nil {
		level = n
	}
	if n, err := strconv.Atoi(parts[2]); err == nil {
		attempt = n
	}
	if len(parts) >= 4 {
		user = parts[3]
	}
	return incident, level, attempt, user
}

// splitClientState splits the decoded client_state on ":". The user field is a
// UUID and never contains ":", so a plain split is safe.
func splitClientState(s string) []string {
	var parts []string
	start := 0
	for i := range len(s) {
		if s[i] == ':' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
