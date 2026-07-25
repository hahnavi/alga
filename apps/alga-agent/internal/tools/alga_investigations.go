package tools

import (
	"context"

	alga "github.com/alga/agent-sdk-go"
)

// --- Investigation tools ---
//
// Investigation reads/updates flow through chat messages and inv_tool
// commands; the backend has no standalone investigation REST endpoints.

type sendMessageInput struct {
	ChatID   string   `json:"chat_id,omitempty" desc:"Target chat id (omit in Alga threads)"`
	Text     string   `json:"text" desc:"The message text"`
	Mentions []string `json:"mentions,omitempty" desc:"Usernames or agent IDs to mention"`
}

type setOutcomeInput struct {
	RootCause  string `json:"root_cause,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	ChatID     string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
}

type cancelInvestigationInput struct {
	Reason string `json:"reason" desc:"Why the investigation is being cancelled"`
	ChatID string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
}

type triageFeedbackInput struct {
	TriageResultID  string `json:"triage_result_id" desc:"The triage result being scored"`
	Agreed          bool   `json:"agreed" desc:"Whether you agree with the triage decision"`
	CorrectDecision string `json:"correct_decision,omitempty"`
	CorrectSeverity string `json:"correct_severity,omitempty"`
	Note            string `json:"note,omitempty"`
	ChatID          string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
}

type promoteToIncidentInput struct {
	Title    string `json:"title"`
	Severity string `json:"severity" desc:"Incident severity (e.g. SEV1, SEV2, SEV3)"`
	Priority string `json:"priority,omitempty"`
	ChatID   string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
}

func investigationTools(c AlgaClient) []Tool {
	return []Tool{
		NewTypedTool("alga_send_message",
			"Send a message to an Alga chat (e.g. a coordination channel).",
			func(ctx context.Context, in sendMessageInput) Result[*alga.SendMessageResponse] {
				if in.Text == "" {
					return ErrMsg[*alga.SendMessageResponse]("text is required")
				}
				chatID := in.ChatID
				if chatID == "" {
					var err error
					chatID, err = chatIDFromCtx(ctx, nil)
					if err != nil {
						return Err[*alga.SendMessageResponse](err)
					}
				}
				resp, err := c.SendMessage(ctx, chatID, in.Text, in.Mentions)
				if err != nil {
					return Err[*alga.SendMessageResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[sendMessageInput, *alga.SendMessageResponse](algaCategory),
			WithCapability[sendMessageInput, *alga.SendMessageResponse]("communicate"),
		),

		NewTypedTool("alga_set_outcome",
			"Set the outcome (root cause and resolution) of the current investigation.",
			func(ctx context.Context, in setOutcomeInput) Result[*alga.CommandResponse] {
				var rc, res *string
				if in.RootCause != "" {
					rc = alga.StrPtr(in.RootCause)
				}
				if in.Resolution != "" {
					res = alga.StrPtr(in.Resolution)
				}
				chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
				if err != nil {
					return Err[*alga.CommandResponse](err)
				}
				resp, err := c.SendCommand(ctx, chatID, alga.SetOutcome(rc, res))
				if err != nil {
					return Err[*alga.CommandResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[setOutcomeInput, *alga.CommandResponse](algaCategory),
			WithCapability[setOutcomeInput, *alga.CommandResponse]("investigate"),
		),

		NewTypedTool("alga_cancel_investigation",
			"Cancel the current investigation with a reason.",
			func(ctx context.Context, in cancelInvestigationInput) Result[*alga.CommandResponse] {
				if in.Reason == "" {
					return ErrMsg[*alga.CommandResponse]("reason is required")
				}
				chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
				if err != nil {
					return Err[*alga.CommandResponse](err)
				}
				resp, err := c.SendCommand(ctx, chatID, alga.CancelInvestigation(in.Reason))
				if err != nil {
					return Err[*alga.CommandResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[cancelInvestigationInput, *alga.CommandResponse](algaCategory),
			WithCapability[cancelInvestigationInput, *alga.CommandResponse]("investigate"),
		),

		NewTypedTool("alga_triage_feedback",
			"Submit triage feedback for an investigation.",
			func(ctx context.Context, in triageFeedbackInput) Result[*alga.CommandResponse] {
				if in.TriageResultID == "" {
					return ErrMsg[*alga.CommandResponse]("triage_result_id is required")
				}
				chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
				if err != nil {
					return Err[*alga.CommandResponse](err)
				}
				resp, err := c.SendCommand(ctx, chatID, alga.TriageFeedback(in.TriageResultID, in.Agreed, in.CorrectDecision, in.CorrectSeverity, in.Note))
				if err != nil {
					return Err[*alga.CommandResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[triageFeedbackInput, *alga.CommandResponse](algaCategory),
			WithCapability[triageFeedbackInput, *alga.CommandResponse]("investigate"),
		),

		NewTypedTool("alga_promote_to_incident",
			"Promote the current investigation to an incident.",
			func(ctx context.Context, in promoteToIncidentInput) Result[*alga.CommandResponse] {
				if in.Title == "" || in.Severity == "" {
					return ErrMsg[*alga.CommandResponse]("title and severity are required")
				}
				chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
				if err != nil {
					return Err[*alga.CommandResponse](err)
				}
				resp, err := c.SendCommand(ctx, chatID, alga.PromoteToIncident(in.Title, in.Severity, in.Priority))
				if err != nil {
					return Err[*alga.CommandResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[promoteToIncidentInput, *alga.CommandResponse](algaCategory),
			WithCapability[promoteToIncidentInput, *alga.CommandResponse]("investigate"),
		),
	}
}
