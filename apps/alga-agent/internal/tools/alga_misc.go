package tools

import (
	"context"

	alga "github.com/alga/agent-sdk-go"
)

// --- Catalog & on-call ---

type listServicesOutput struct {
	Services []alga.Service `json:"services"`
	Count    int            `json:"count"`
}

func miscTools(c AlgaClient) []Tool {
	return []Tool{
		NewTypedTool("alga_list_services",
			"List all registered services in Alga.",
			func(ctx context.Context, _ struct{}) Result[listServicesOutput] {
				resp, err := c.ListServices(ctx, nil)
				if err != nil {
					return Err[listServicesOutput](algaErr(err))
				}
				return OK(listServicesOutput{Services: resp.Items, Count: len(resp.Items)})
			},
			WithCategory[struct{}, listServicesOutput](algaCategory),
		),

		NewTypedTool("alga_who_is_on_call",
			"Get the current on-call information.",
			func(ctx context.Context, _ struct{}) Result[[]alga.OnCallEntry] {
				oncall, err := c.WhoIsOnCall(ctx)
				if err != nil {
					return Err[[]alga.OnCallEntry](algaErr(err))
				}
				return OK(oncall)
			},
			WithCategory[struct{}, []alga.OnCallEntry](algaCategory),
		),
	}
}
