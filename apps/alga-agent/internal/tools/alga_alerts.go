package tools

import (
	"context"
	"strconv"

	alga "github.com/alga/agent-sdk-go"
)

// --- Alert tools ---

type listAlertsInput struct {
	Status   string `json:"status,omitempty" desc:"Filter by alert status (active, resolved, silenced)"`
	Severity string `json:"severity,omitempty" desc:"Filter by severity label"`
	Service  string `json:"service,omitempty" desc:"Filter by service label"`
	Limit    int    `json:"limit,omitempty" desc:"Maximum alerts to return (default 50)"`
	Skip     int    `json:"skip,omitempty" desc:"Pagination offset"`
}

type listAlertsOutput struct {
	Alerts []alga.Alert `json:"alerts"`
	Count  int          `json:"count"`
}

func alertTools(c AlgaClient) []Tool {
	return []Tool{
		NewTypedTool("alga_list_alerts",
			"List active alerts in Alga with optional filters.",
			func(ctx context.Context, in listAlertsInput) Result[listAlertsOutput] {
				params := map[string]string{}
				if in.Status != "" {
					params["status"] = in.Status
				}
				if in.Severity != "" {
					params["severity"] = in.Severity
				}
				if in.Service != "" {
					params["service"] = in.Service
				}
				if in.Limit > 0 {
					params["limit"] = itoa(in.Limit)
				}
				if in.Skip > 0 {
					params["skip"] = itoa(in.Skip)
				}
				alerts, err := c.ListAlerts(ctx, params)
				if err != nil {
					return Err[listAlertsOutput](algaErr(err))
				}
				return OK(listAlertsOutput{Alerts: alerts, Count: len(alerts)})
			},
			WithCategory[listAlertsInput, listAlertsOutput](algaCategory),
		),

		NewTypedTool("alga_get_alert",
			"Get full details for a single alert by fingerprint.",
			func(ctx context.Context, in struct {
				Fingerprint string `json:"fingerprint" desc:"The alert fingerprint (from alert:<fingerprint> or list output)"`
			}) Result[*alga.Alert] {
				if in.Fingerprint == "" {
					return ErrMsg[*alga.Alert]("fingerprint is required")
				}
				a, err := c.GetAlert(ctx, in.Fingerprint)
				if err != nil {
					return Err[*alga.Alert](algaErr(err))
				}
				return OK(a)
			},
			WithCategory[struct {
				Fingerprint string `json:"fingerprint" desc:"The alert fingerprint (from alert:<fingerprint> or list output)"`
			}, *alga.Alert](algaCategory),
		),

		NewTypedTool("alga_resolve_alert",
			"Resolve an alert by fingerprint. Use when the underlying issue is addressed.",
			func(ctx context.Context, in struct {
				Fingerprint string `json:"fingerprint" desc:"The alert fingerprint to resolve"`
				ChatID      string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
			}) Result[*alga.CommandResponse] {
				if in.Fingerprint == "" {
					return ErrMsg[*alga.CommandResponse]("fingerprint is required")
				}
				chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
				if err != nil {
					return Err[*alga.CommandResponse](err)
				}
				resp, err := c.SendCommand(ctx, chatID, alga.ResolveAlert(in.Fingerprint))
				if err != nil {
					return Err[*alga.CommandResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[struct {
				Fingerprint string `json:"fingerprint" desc:"The alert fingerprint to resolve"`
				ChatID      string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
			}, *alga.CommandResponse](algaCategory),
			WithCapability[struct {
				Fingerprint string `json:"fingerprint" desc:"The alert fingerprint to resolve"`
				ChatID      string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
			}, *alga.CommandResponse]("investigate"),
		),

		NewTypedTool("alga_reopen_alert",
			"Reopen a previously resolved alert by fingerprint.",
			func(ctx context.Context, in struct {
				Fingerprint string `json:"fingerprint" desc:"The alert fingerprint to reopen"`
				ChatID      string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
			}) Result[*alga.CommandResponse] {
				if in.Fingerprint == "" {
					return ErrMsg[*alga.CommandResponse]("fingerprint is required")
				}
				chatID, err := chatIDFromCtx(ctx, map[string]string{"chat_id": in.ChatID})
				if err != nil {
					return Err[*alga.CommandResponse](err)
				}
				resp, err := c.SendCommand(ctx, chatID, alga.ReopenAlert(in.Fingerprint))
				if err != nil {
					return Err[*alga.CommandResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[struct {
				Fingerprint string `json:"fingerprint" desc:"The alert fingerprint to reopen"`
				ChatID      string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
			}, *alga.CommandResponse](algaCategory),
			WithCapability[struct {
				Fingerprint string `json:"fingerprint" desc:"The alert fingerprint to reopen"`
				ChatID      string `json:"chat_id,omitempty" desc:"Chat id (omit in Alga threads)"`
			}, *alga.CommandResponse]("investigate"),
		),
	}
}

// itoa is a tiny local wrapper around strconv.FormatInt so tools that need
// to stuff an int into a map[string]string query parameter don't repeat the
// import in every file.
func itoa(n int) string { return strconv.FormatInt(int64(n), 10) }
