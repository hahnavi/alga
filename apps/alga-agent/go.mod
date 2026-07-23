module alga-agent

go 1.26.5

require (
	github.com/alga/agent-sdk-go v0.0.0-00010101000000-000000000000
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
	github.com/modelcontextprotocol/go-sdk v1.6.1
	golang.org/x/term v0.44.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
)

// Local SDK: resolved via go.work in the monorepo. The replace directive makes
// standalone builds (outside the workspace) work against the local checkout.
replace github.com/alga/agent-sdk-go => ../../integrations/alga-agent-sdk-go
