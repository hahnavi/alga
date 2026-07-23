// Code moved from http.go; see git history.

package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"alga/logger"
	"alga/store"
	"alga/twilio"
)

func (s *Server) handleTwilioCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	if s.cfg.TwilioAuthToken != "" {
		if !validateTwilioSignature(r, s.cfg.TwilioAuthToken) {
			logger.Warn("Twilio callback rejected: invalid signature")
			writeError(w, ErrorCodeForbidden, "invalid signature")
			return
		}
	} else {
		logger.Warn("Twilio callback rejected: auth token not configured")
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "twilio not configured")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid form data")
		return
	}

	ctx := r.Context()
	digits := r.FormValue("Digits")
	callSID := r.FormValue("CallSid")
	callStatus := r.FormValue("CallStatus")
	incidentStr := r.URL.Query().Get("incident")
	userStr := r.URL.Query().Get("user")
	attempt, _ := strconv.Atoi(r.URL.Query().Get("attempt"))
	if attempt < 1 {
		attempt = 1
	}

	writeXML := func(twiML string) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(twiML))
	}

	// Status callback (terminal call status, no DTMF): log and close the call.
	if callStatus != "" && digits == "" {
		logger.InfoCtx(ctx, "Twilio call status callback", "component", "twilio", "call_sid", callSID, "call_status", callStatus, "incident", incidentStr)
		writeXML(`<?xml version="1.0" encoding="UTF-8"?><Response><Say>Thank you. Goodbye.</Say></Response>`)
		return
	}

	logger.InfoCtx(ctx, "Twilio IVR callback received", "component", "twilio", "call_sid", callSID, "digits", digits, "incident", incidentStr, "attempt", attempt)

	incidentNumber, err := strconv.ParseInt(incidentStr, 10, 64)
	if err != nil || incidentNumber <= 0 {
		logger.WarnCtx(ctx, "Twilio callback: invalid incident id", "component", "twilio", "incident", incidentStr)
		writeXML(`<?xml version="1.0" encoding="UTF-8"?><Response><Say>Thank you. Goodbye.</Say></Response>`)
		return
	}

	switch digits {
	case "1":
		actorID, actorLabel := s.resolveVoiceActor(ctx, userStr)
		s.cancelEscalationForIncident(ctx, incidentStr, "acknowledged via phone")
		if actorID != nil || actorLabel != "" {
			s.recordVoiceTimelineEntry(ctx, incidentNumber, "acknowledged", "Incident acknowledged via Twilio IVR (press 1)", actorID, actorLabel)
		}
		s.auditVoiceCallback(store.AuditVoiceAck, actorID, actorLabel, "twilio", map[string]any{
			"incident_number": incidentNumber,
			"provider":        "twilio",
		})
		logger.InfoCtx(ctx, "Incident acknowledged via Twilio IVR", "component", "twilio", "incident_number", incidentNumber, "call_sid", callSID, "actor", actorLabel)
		writeXML(`<?xml version="1.0" encoding="UTF-8"?><Response><Say>Incident acknowledged. Escalation stopped. Goodbye.</Say></Response>`)
	case "2":
		actorID, actorLabel := s.resolveVoiceActor(ctx, userStr)
		if s.vkClient != nil {
			hashKey := "alga:esc:" + incidentStr
			_ = s.vkClient.HSet(ctx, hashKey, "silenced_until", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
		}
		s.recordVoiceTimelineEntry(ctx, incidentNumber, "escalation_silenced", "Escalation silenced for 1 hour via Twilio IVR (press 2)", actorID, actorLabel)
		s.auditVoiceCallback(store.AuditVoiceSilence, actorID, actorLabel, "twilio", map[string]any{
			"incident_number": incidentNumber,
			"provider":        "twilio",
		})
		logger.InfoCtx(ctx, "Escalation silenced for 1 hour via Twilio IVR", "component", "twilio", "incident_number", incidentNumber, "call_sid", callSID, "actor", actorLabel)
		writeXML(`<?xml version="1.0" encoding="UTF-8"?><Response><Say>Escalation silenced for one hour. Goodbye.</Say></Response>`)
	default:
		// No digit (or unrecognized): re-prompt up to maxIVRAttempts via a new
		// <Gather> whose action points at the next attempt. Twilio handles the
		// loop by POSTing back here on the next no-input/timeout.
		next := attempt + 1
		gatherURL := s.twilioGatherActionURL(incidentStr, next, userStr)
		logger.InfoCtx(ctx, "Twilio IVR re-prompting", "component", "twilio", "incident_number", incidentNumber, "call_sid", callSID, "attempt", next)
		writeXML(twilio.RePromptTwiML(gatherURL, next))
	}
}

// twilioGatherActionURL builds the relative action URL for a Twilio <Gather>
// re-prompt. It keeps the same host/scheme as the inbound request and carries
// the bumped attempt plus the user query param so ack/silence stays attributed.
func (s *Server) twilioGatherActionURL(incident string, attempt int, user string) string {
	path := "/api/v1/twilio/callback?incident=" + incident + "&attempt=" + strconv.Itoa(attempt)
	if user != "" {
		path += "&user=" + user
	}
	return path
}

// validateTwilioSignature validates the X-Twilio-Signature header

func validateTwilioSignature(r *http.Request, authToken string) bool {
	signature := r.Header.Get("X-Twilio-Signature")
	if signature == "" {
		return false
	}

	scheme := "https"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	url := scheme + "://" + host + r.URL.Path

	// Read the POST body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return false
	}
	// Restore body for later use
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	// Concatenate URL and form parameters (sorted by key)
	params := r.URL.Query()
	if err := r.ParseForm(); err == nil {
		for key := range r.Form {
			params.Set(key, r.FormValue(key))
		}
	}

	// Sort keys and build data string
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var builder strings.Builder
	builder.WriteString(url)
	for _, k := range keys {
		builder.WriteString(k)
		builder.WriteString(params.Get(k))
	}
	data := builder.String()

	// Compute HMAC-SHA1
	h := hmac.New(sha1.New, []byte(authToken))
	h.Write([]byte(data))
	expected := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}
