// Package version exposes the build version as a single source of truth so
// both the CLI entrypoint and internal packages (e.g. the setup wizard's model
// probe) can report a consistent identity.
package version

// Version is the build version, injected at build time via
// -ldflags "-X alga-agent/internal/version.Version=<v>". Defaults to "dev".
var Version = "dev"

// UserAgent returns an "alga-agent/<version>" identifier for outbound HTTP
// requests. Some providers sit behind a WAF that rejects Go's default
// User-Agent, so requests should identify themselves explicitly.
func UserAgent() string {
	return "alga-agent/" + Version
}
