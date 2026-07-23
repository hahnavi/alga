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
				svcs, err := c.ListServices(ctx)
				if err != nil {
					return Err[listServicesOutput](algaErr(err))
				}
				return OK(listServicesOutput{Services: svcs, Count: len(svcs)})
			},
			WithCategory[struct{}, listServicesOutput](algaCategory),
		),

		NewTypedTool("alga_who_is_on_call",
			"Get the current on-call information.",
			func(ctx context.Context, _ struct{}) Result[map[string]any] {
				oncall, err := c.WhoIsOnCall(ctx)
				if err != nil {
					return Err[map[string]any](algaErr(err))
				}
				return OK(oncall)
			},
			WithCategory[struct{}, map[string]any](algaCategory),
		),
	}
}
