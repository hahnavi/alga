package knowledge

import (
	"context"
	"fmt"
	"strings"

	"alga/rabbitmq"
	"alga/store"
	"alga/strutil"
)

// Config controls how much shared knowledge is injected. Zero or negative
// values disable the corresponding layer.
type Config struct {
	MaxEpisodic   int
	MaxNotes      int
	MaxConcurrent int
	MaxMemories   int
}

// Aggregator is the default Service implementation. All collaborators are
// optional; missing ones simply produce empty sections.
type Aggregator struct {
	episodic   EpisodicFinder
	notes      NotesFinder
	concurrent ConcurrentFinder
	memories   MemoryFinder
	cfg        Config
}

// WithNotes returns a shallow copy of the aggregator with the given notes
// finder wired. Useful when notes support is added after initial construction
// (e.g. phase rollouts).
func (a *Aggregator) WithNotes(n NotesFinder) *Aggregator {
	if a == nil {
		return nil
	}
	cp := *a
	cp.notes = n
	return &cp
}

func (a *Aggregator) WithMemory(m MemoryFinder) *Aggregator {
	if a == nil {
		return nil
	}
	cp := *a
	cp.memories = m
	return &cp
}

// DefaultConfig returns sensible defaults used when configuration is absent.
func DefaultConfig() Config {
	return Config{
		MaxEpisodic:   3,
		MaxNotes:      3,
		MaxConcurrent: 5,
		MaxMemories:   5,
	}
}

func severityConfig(severity string) Config {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high":
		return Config{MaxEpisodic: 3, MaxNotes: 3, MaxConcurrent: 5, MaxMemories: 5}
	case "warning":
		return Config{MaxEpisodic: 1, MaxNotes: 2, MaxConcurrent: 2, MaxMemories: 2}
	default:
		return Config{MaxEpisodic: 1, MaxNotes: 1, MaxConcurrent: 0, MaxMemories: 0}
	}
}

// NewAggregator returns a Service that aggregates every wired collaborator.
// Pass nil for collaborators that are not yet implemented; they are skipped.
func NewAggregator(episodic EpisodicFinder, cfg Config) *Aggregator {
	if cfg.MaxEpisodic == 0 && cfg.MaxNotes == 0 && cfg.MaxConcurrent == 0 && cfg.MaxMemories == 0 {
		cfg = DefaultConfig()
	}
	return &Aggregator{
		episodic: episodic,
		cfg:      cfg,
	}
}

// BuildContext returns the aggregated knowledge context for the given
// investigation. Returns nil when the investigation is nil.
func (a *Aggregator) BuildContext(ctx context.Context, inv *store.AlertInvestigationRecord) *Context {
	if a == nil || inv == nil {
		return nil
	}
	cfg := a.cfg
	if override := severityConfig(rabbitmq.DetermineAlertSeverity(inv.Alerts)); override.MaxEpisodic > 0 {
		cfg = override
	}
	c := &Context{}
	if a.episodic != nil && cfg.MaxEpisodic > 0 {
		c.Past = a.episodic.Find(ctx, inv, cfg.MaxEpisodic)
	}
	if a.notes != nil && cfg.MaxNotes > 0 {
		c.Notes = a.notes.Find(ctx, inv, cfg.MaxNotes)
	}
	if a.concurrent != nil && cfg.MaxConcurrent > 0 {
		c.Concurrent = a.concurrent.Find(ctx, inv, cfg.MaxConcurrent)
	}
	if a.memories != nil && cfg.MaxMemories > 0 {
		c.Memories = a.memories.Find(ctx, inv, cfg.MaxMemories)
	}
	return c
}

// PromptBlock renders the knowledge context as a deterministic, agent-agnostic
// text block suitable for appending to a prompt. Returns an empty string when
// there is nothing to inject.
func (c *Context) PromptBlock() string {
	if c.IsEmpty() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n--- SHARED KNOWLEDGE (auto-injected by Alga) ---\n")

	if len(c.Past) > 0 {
		fmt.Fprintf(&b, "\nPAST INCIDENTS (%d shown):\n", len(c.Past))
		for _, e := range c.Past {
			writeEpisodic(&b, e)
		}
	}

	if len(c.Notes) > 0 {
		fmt.Fprintf(&b, "\nAPPLICABLE RUNBOOKS / KNOWN ISSUES (%d shown):\n", len(c.Notes))
		for _, n := range c.Notes {
			writeNote(&b, n)
		}
		b.WriteString("Previews are truncated. Call alga_get_knowledge with a note's id to read the full runbook and follow its steps.\n")
	}

	if len(c.Concurrent) > 0 {
		fmt.Fprintf(&b, "\nCONCURRENT INVESTIGATIONS (%d shown):\n", len(c.Concurrent))
		for _, p := range c.Concurrent {
			writeConcurrent(&b, p)
		}
	}

	if len(c.Memories) > 0 {
		fmt.Fprintf(&b, "\nAGENT MEMORIES (%d shown):\n", len(c.Memories))
		for _, m := range c.Memories {
			writeMemory(&b, m)
		}
	}

	if c.PeerAskAvailable {
		b.WriteString("\nYou can ask another agent directly via peer_ask (inv_tool op=\"peer_ask\").\n")
	}

	b.WriteString("--- END SHARED KNOWLEDGE ---\n")
	return b.String()
}

func writeEpisodic(b *strings.Builder, e EpisodicEntry) {
	id := e.InvestigationID
	if id == "" {
		id = "(unknown)"
	}
	agent := strings.TrimSpace(e.AgentType)
	if agent == "" {
		agent = "agent"
	}
	sev := strings.TrimSpace(e.Severity)
	if sev == "" {
		sev = "unknown"
	}
	fmt.Fprintf(b, "- %s (%s, %s, %s)\n", id, humanizeAge(e.CompletedAt), agent, sev)
	if e.AlertName != "" {
		if e.Namespace != "" {
			fmt.Fprintf(b, "  alert: %s (namespace=%s)\n", e.AlertName, e.Namespace)
		} else {
			fmt.Fprintf(b, "  alert: %s\n", e.AlertName)
		}
	}
	if rc := strings.TrimSpace(e.RootCause); rc != "" {
		fmt.Fprintf(b, "  root_cause: %s\n", rc)
	}
	if rs := strings.TrimSpace(e.Resolution); rs != "" {
		fmt.Fprintf(b, "  resolution: %s\n", rs)
	}
}

func writeNote(b *strings.Builder, n NoteEntry) {
	kind := strings.TrimSpace(n.Kind)
	if kind == "" {
		kind = "note"
	}
	title := strings.TrimSpace(n.Title)
	if title == "" {
		title = "(untitled)"
	}
	if len(n.Tags) > 0 {
		fmt.Fprintf(b, "- [%s] %s (tags: %s)\n", kind, title, strings.Join(n.Tags, ","))
	} else {
		fmt.Fprintf(b, "- [%s] %s\n", kind, title)
	}
	if id := strings.TrimSpace(n.ID); id != "" {
		fmt.Fprintf(b, "  id: %s\n", id)
	}
	body := strings.TrimSpace(n.Body)
	if body != "" {
		fmt.Fprintf(b, "  %s\n", strutil.TruncateOneLine(body, 240))
	}
}

func writeConcurrent(b *strings.Builder, p ConcurrentEntry) {
	id := p.InvestigationID
	if id == "" {
		id = "(unknown)"
	}
	agent := strings.TrimSpace(p.AgentType)
	if agent == "" {
		agent = "agent"
	}
	ago := ""
	if p.StartedAgo > 0 {
		ago = " (started " + humanizeDuration(p.StartedAgo) + " ago)"
	}
	if p.AlertName != "" {
		if p.Namespace != "" {
			fmt.Fprintf(b, "- %s by %s — %s on %s%s\n", id, agent, p.AlertName, p.Namespace, ago)
		} else {
			fmt.Fprintf(b, "- %s by %s — %s%s\n", id, agent, p.AlertName, ago)
		}
	} else {
		fmt.Fprintf(b, "- %s by %s%s\n", id, agent, ago)
	}
}
