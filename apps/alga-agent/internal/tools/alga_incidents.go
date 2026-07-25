package tools

import (
	"context"

	alga "github.com/alga/agent-sdk-go"
)

// --- Incident tools ---

type getIncidentInput struct {
	IncidentNumber int64 `json:"incident_number,omitempty" desc:"Incident number (omit in Alga incident threads; the context number is used)"`
}

type addIncidentTimelineInput struct {
	IncidentNumber int64  `json:"incident_number,omitempty"`
	Message        string `json:"message"`
	EventType      string `json:"event_type" desc:"e.g. detection, mitigation, escalation, note"`
}

// incidentNumberInput is the shared schema for incident-scoped command tools.
type incidentNumberInput struct {
	IncidentNumber int64  `json:"incident_number" desc:"The incident number"`
	Reason         string `json:"reason,omitempty"`
	ChatID         string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
}

func incidentTools(c AlgaClient) []Tool {
	return []Tool{
		NewTypedTool("alga_get_incident",
			"Get incident details (record plus active role assignments) by incident number.",
			func(ctx context.Context, in getIncidentInput) Result[*alga.IncidentContext] {
				n, err := incidentNumberFromCtx(ctx, in.IncidentNumber)
				if err != nil {
					return Err[*alga.IncidentContext](err)
				}
				inc, err := c.GetIncident(ctx, n)
				if err != nil {
					return Err[*alga.IncidentContext](algaErr(err))
				}
				return OK(inc)
			},
			WithCategory[getIncidentInput, *alga.IncidentContext](algaCategory),
		),

		NewTypedTool("alga_add_incident_timeline",
			"Add a timeline entry to an incident.",
			func(ctx context.Context, in addIncidentTimelineInput) Result[struct {
				IncidentNumber int64 `json:"incident_number"`
				OK             bool  `json:"ok"`
			}] {
				if in.Message == "" || in.EventType == "" {
					return ErrMsg[struct {
						IncidentNumber int64 `json:"incident_number"`
						OK             bool  `json:"ok"`
					}]("message and event_type are required")
				}
				n, err := incidentNumberFromCtx(ctx, in.IncidentNumber)
				if err != nil {
					return Err[struct {
						IncidentNumber int64 `json:"incident_number"`
						OK             bool  `json:"ok"`
					}](err)
				}
				if err := c.AddIncidentTimeline(ctx, n, in.Message, in.EventType); err != nil {
					return Err[struct {
						IncidentNumber int64 `json:"incident_number"`
						OK             bool  `json:"ok"`
					}](algaErr(err))
				}
				return OK(struct {
					IncidentNumber int64 `json:"incident_number"`
					OK             bool  `json:"ok"`
				}{IncidentNumber: n, OK: true})
			},
			WithCategory[addIncidentTimelineInput, struct {
				IncidentNumber int64 `json:"incident_number"`
				OK             bool  `json:"ok"`
			}](algaCategory),
		),

		incidentCommandTool(c, "alga_trigger_escalation",
			"Trigger escalation for an incident.",
			false,
			func(in incidentNumberInput) alga.InvestigationCommand {
				return alga.TriggerEscalation(in.IncidentNumber)
			},
			"command",
		),
		incidentCommandTool(c, "alga_mitigate_incident",
			"Mark an incident as mitigated with a reason.",
			true,
			func(in incidentNumberInput) alga.InvestigationCommand {
				return alga.MitigateIncident(in.IncidentNumber, in.Reason)
			},
			"command",
		),
		incidentCommandTool(c, "alga_resolve_incident",
			"Resolve an incident with a reason. Commander-capable agents only.",
			true,
			func(in incidentNumberInput) alga.InvestigationCommand {
				return alga.ResolveIncident(in.IncidentNumber, in.Reason)
			},
			"command",
		),
	}
}

// incidentCommandTool builds an incident-scoped SendCommand tool. requireReason
// gates whether the reason argument is enforced; cap is the RBAC capability
// required to invoke the tool.
func incidentCommandTool(c AlgaClient, name, desc string, requireReason bool,
	build func(incidentNumberInput) alga.InvestigationCommand, cap string,
) Tool {
	return NewTypedTool(name, desc,
		func(ctx context.Context, in incidentNumberInput) Result[*alga.CommandResponse] {
			if in.IncidentNumber == 0 {
				return ErrMsg[*alga.CommandResponse]("incident_number is required")
			}
			if requireReason && in.Reason == "" {
				return ErrMsg[*alga.CommandResponse]("reason is required")
			}
			chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
			if err != nil {
				return Err[*alga.CommandResponse](err)
			}
			resp, err := c.SendCommand(ctx, chatID, build(in))
			if err != nil {
				return Err[*alga.CommandResponse](algaErr(err))
			}
			return OK(resp)
		},
		WithCategory[incidentNumberInput, *alga.CommandResponse](algaCategory),
		WithCapability[incidentNumberInput, *alga.CommandResponse](cap),
	)
}
