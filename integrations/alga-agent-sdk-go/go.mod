module github.com/alga/agent-sdk-go

go 1.27.0

// Stdlib only — no external dependencies. The wire format uses string IDs
// even where the backend persists UUIDs, so callers do not need a UUID
// library to integrate.
