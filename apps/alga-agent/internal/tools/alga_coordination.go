package tools

import (
	"context"

	alga "github.com/alga/agent-sdk-go"
)

// --- Coordination tasks ---
//
// These wrap the incident commander's task decomposition tools plus the
// responder/communicator task lifecycle (claim → complete). They are the
// modern replacement for the deprecated post_handoff handoff flow:
// dispatch_task + complete_task is the normal coordination path.

type dispatchTaskInput struct {
	IncidentNumber  int64  `json:"incident_number" desc:"The incident to decompose"`
	Kind            string `json:"kind" desc:"Task kind (investigate, communicate, verify, mitigate)"`
	Goal            string `json:"goal" desc:"Bounded, specific goal of the task"`
	AssigneeRole    string `json:"assignee_role,omitempty" desc:"Target role (responder, communicator, verifier)"`
	AssigneeAgentID string `json:"assignee_agent_id,omitempty" desc:"Target a specific agent by token id"`
	ChatID          string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
}

type claimTaskInput struct {
	TaskID string `json:"task_id" desc:"The coordination task id to claim"`
	ChatID string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
}

type completeTaskInput struct {
	TaskID string         `json:"task_id" desc:"The task id being completed"`
	Result map[string]any `json:"result" desc:"Typed result (e.g. finding, evidence, root_cause_candidate, published_status_id)"`
	ChatID string         `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
}

type synthesizeFindingsInput struct {
	IncidentNumber int64          `json:"incident_number"`
	Summary        string         `json:"summary" desc:"The synthesized incident conclusion"`
	Evidence       map[string]any `json:"evidence,omitempty" desc:"Per-investigation findings (key = investigation id, value = summary)"`
	ChatID         string         `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
}

type listIncidentTasksInput struct {
	IncidentNumber int64 `json:"incident_number" desc:"The incident to list tasks for"`
}

func coordinationTools(c AlgaClient) []Tool {
	return []Tool{
		NewTypedTool("alga_dispatch_task",
			"Commander-only: decompose an incident into a bounded task targeted at a role or agent.",
			func(ctx context.Context, in dispatchTaskInput) Result[*alga.CommandResponse] {
				if in.IncidentNumber == 0 || in.Kind == "" || in.Goal == "" {
					return ErrMsg[*alga.CommandResponse]("incident_number, kind, and goal are required")
				}
				chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
				if err != nil {
					return Err[*alga.CommandResponse](err)
				}
				var cmd alga.InvestigationCommand
				if in.AssigneeAgentID != "" {
					cmd = alga.DispatchTaskToAgent(in.IncidentNumber, in.Kind, in.Goal, in.AssigneeAgentID)
				} else {
					role := in.AssigneeRole
					if role == "" {
						role = "responder"
					}
					cmd = alga.DispatchTask(in.IncidentNumber, in.Kind, in.Goal, role)
				}
				resp, err := c.SendCommand(ctx, chatID, cmd)
				if err != nil {
					return Err[*alga.CommandResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[dispatchTaskInput, *alga.CommandResponse](algaCategory),
			WithCapability[dispatchTaskInput, *alga.CommandResponse]("command"),
		),

		NewTypedTool("alga_claim_task",
			"Claim a pending coordination task. The backend rejects the claim if the task is already claimed, completed, or past its deadline.",
			func(ctx context.Context, in claimTaskInput) Result[*alga.CommandResponse] {
				if in.TaskID == "" {
					return ErrMsg[*alga.CommandResponse]("task_id is required")
				}
				chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
				if err != nil {
					return Err[*alga.CommandResponse](err)
				}
				resp, err := c.SendCommand(ctx, chatID, alga.ClaimTask(in.TaskID))
				if err != nil {
					return Err[*alga.CommandResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[claimTaskInput, *alga.CommandResponse](algaCategory),
			WithCapability[claimTaskInput, *alga.CommandResponse]("investigate"),
		),

		NewTypedTool("alga_complete_task",
			"Mark a claimed task done and record its typed result. This is the normal path for handing work back to the commander (replaces post_handoff).",
			func(ctx context.Context, in completeTaskInput) Result[*alga.CommandResponse] {
				if in.TaskID == "" {
					return ErrMsg[*alga.CommandResponse]("task_id is required")
				}
				chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
				if err != nil {
					return Err[*alga.CommandResponse](err)
				}
				resp, err := c.SendCommand(ctx, chatID, alga.CompleteTask(in.TaskID, in.Result))
				if err != nil {
					return Err[*alga.CommandResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[completeTaskInput, *alga.CommandResponse](algaCategory),
			WithCapability[completeTaskInput, *alga.CommandResponse]("investigate"),
		),

		NewTypedTool("alga_synthesize_findings",
			"Commander-only: synthesize the conclusion of an incident from completed child investigations.",
			func(ctx context.Context, in synthesizeFindingsInput) Result[*alga.CommandResponse] {
				if in.IncidentNumber == 0 || in.Summary == "" {
					return ErrMsg[*alga.CommandResponse]("incident_number and summary are required")
				}
				chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
				if err != nil {
					return Err[*alga.CommandResponse](err)
				}
				resp, err := c.SendCommand(ctx, chatID, alga.SynthesizeFindings(in.IncidentNumber, in.Summary, in.Evidence))
				if err != nil {
					return Err[*alga.CommandResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[synthesizeFindingsInput, *alga.CommandResponse](algaCategory),
			WithCapability[synthesizeFindingsInput, *alga.CommandResponse]("command"),
		),

		NewTypedTool("alga_list_tasks",
			"List the coordination tasks for an incident (the read side of dispatch_task / claim_task / complete_task).",
			func(ctx context.Context, in listIncidentTasksInput) Result[struct {
				Tasks []alga.CoordinationTask `json:"tasks"`
				Count int                     `json:"count"`
			}] {
				if in.IncidentNumber == 0 {
					return ErrMsg[struct {
						Tasks []alga.CoordinationTask `json:"tasks"`
						Count int                     `json:"count"`
					}]("incident_number is required")
				}
				tasks, err := c.ListIncidentTasks(ctx, in.IncidentNumber, nil)
				if err != nil {
					return Err[struct {
						Tasks []alga.CoordinationTask `json:"tasks"`
						Count int                     `json:"count"`
					}](algaErr(err))
				}
				return OK(struct {
					Tasks []alga.CoordinationTask `json:"tasks"`
					Count int                     `json:"count"`
				}{Tasks: tasks, Count: len(tasks)})
			},
			WithCategory[listIncidentTasksInput, struct {
				Tasks []alga.CoordinationTask `json:"tasks"`
				Count int                     `json:"count"`
			}](algaCategory),
		),
	}
}
