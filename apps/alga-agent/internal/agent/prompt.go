package agent

import (
	"os"
	"strings"
	"time"
)

// SystemPromptOptions controls system prompt assembly. All fields are optional;
// empty sections are omitted from the final prompt rather than emitting empty
// headers (SPEC §5.3).
type SystemPromptOptions struct {
	AgentName        string
	AgentDescription string
	// AlgaCtx is injected when responding inside Alga investigation threads.
	AlgaCtx AlgaContext
	// MemoryContext is the persistent-memory context retrieved via vector
	// similarity search (SPEC §7.2). Empty in MVP — slot reserved.
	MemoryContext string
	// ToolNames is the list of available tool names, for guidance.
	ToolNames []string
	// Now is injected for time-awareness; defaults to time.Now() if zero.
	Now time.Time
	// CustomPromptFile, if set, overrides the base identity section.
	CustomPromptFile string
}

// BuildSystemPrompt assembles the system prompt from base identity, Alga context,
// memory context, and tool descriptions, per SPEC §5.3. Empty sections are
// omitted. Returns the prompt string.
func BuildSystemPrompt(opts SystemPromptOptions) string {
	var b strings.Builder

	// 1. Base identity (or custom override).
	if opts.CustomPromptFile != "" {
		if data, err := os.ReadFile(opts.CustomPromptFile); err == nil {
			b.Write(data)
			b.WriteByte('\n')
		}
	}
	if b.Len() == 0 {
		name := opts.AgentName
		if name == "" {
			name = "Alga Agent"
		}
		desc := opts.AgentDescription
		if desc == "" {
			desc = "an SRE assistant for the Alga AIOps platform"
		}
		b.WriteString(baseIdentity(name, desc))
	}

	// 2. Alga context (investigation/incident/alerts).
	if ac := algaContextSection(opts.AlgaCtx); ac != "" {
		b.WriteString("\n\n## Current Context\n\n")
		b.WriteString(ac)
	}

	// 3. Memory context (slot reserved for v0.2 similarity injection).
	if strings.TrimSpace(opts.MemoryContext) != "" {
		b.WriteString("\n\n## Relevant Memories\n\n")
		b.WriteString(strings.TrimSpace(opts.MemoryContext))
	}

	// 4. Tool usage guidelines.
	if len(opts.ToolNames) > 0 {
		b.WriteString("\n\n## Available Tools\n\n")
		b.WriteString("You have access to the following tools. Use them to take ")
		b.WriteString("actions and retrieve information rather than asking the user ")
		b.WriteString("to do things you can do yourself. When you need an ID that is ")
		b.WriteString("not in the Current Context (such as an investigation or incident id), ")
		b.WriteString("ask the user to provide it in its canonical prefixed form ")
		b.WriteString("(e.g. `inv_<id>` for investigations, `inc_<id>` for incidents, ")
		b.WriteString("`alert:<fingerprint>` for alerts). Never guess or fabricate IDs.\n\n")
		// Categorize tools for readability.
		b.WriteString(toolListSection(opts.ToolNames))
	}

	// 5. Time awareness.
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	b.WriteString("\n\n## Environment\n\n")
	b.WriteString("Current time (UTC): ")
	b.WriteString(now.UTC().Format(time.RFC3339))
	b.WriteString("\nRespond concisely. Prefer Markdown for structure. ")
	b.WriteString("When taking an action, briefly state what you did.\n")

	return b.String()
}

func baseIdentity(name, desc string) string {
	return strings.NewReplacer(
		"{{NAME}}", name,
		"{{DESCRIPTION}}", desc,
	).Replace(`# {{NAME}}

You are {{DESCRIPTION}}.

## Role
You help site reliability engineers and on-call operators triage alerts,
investigate incidents, and operate production systems. You combine Alga
platform data (alerts, investigations, incidents, services, on-call info)
with shell access and web search to diagnose and resolve issues.

## Principles
- Be action-oriented: when asked to resolve an alert, resolve it; when asked
  for a status, fetch it.
- Prefer specific facts over speculation. If you are unsure, say so and use
  a tool to verify.
- Communicate clearly and concisely. Lead with the answer, then evidence.
- When operating inside an Alga investigation thread, use the IDs from the
  Current Context rather than asking the operator for them.
- Do not expose raw secrets, tokens, or credentials in your responses.
- Respect severity: do not resolve or mitigate without confirming intent
  when the action is destructive or customer-impacting.`)
}

func algaContextSection(ctx AlgaContext) string {
	if ctx.InvestigationID == "" && ctx.IncidentID == "" && len(ctx.AlertFingerprints) == 0 {
		return ""
	}
	var b strings.Builder
	if ctx.InvestigationID != "" {
		b.WriteString("- Investigation ID: ")
		b.WriteString(ctx.InvestigationID)
		b.WriteByte('\n')
	}
	if ctx.IncidentID != "" {
		b.WriteString("- Incident ID: ")
		b.WriteString(ctx.IncidentID)
		b.WriteByte('\n')
	}
	if ctx.InvestigationStatus != "" {
		b.WriteString("- Investigation status: ")
		b.WriteString(ctx.InvestigationStatus)
		b.WriteByte('\n')
	}
	if ctx.Severity != "" {
		b.WriteString("- Severity: ")
		b.WriteString(ctx.Severity)
		b.WriteByte('\n')
	}
	if len(ctx.AlertFingerprints) > 0 {
		b.WriteString("- Alert fingerprints in scope:\n")
		for _, fp := range ctx.AlertFingerprints {
			b.WriteString("  - ")
			b.WriteString(fp)
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// toolListSection renders the tool names as a categorized bullet list. Tools
// prefixed with "alga_" are grouped; others (shell, web_search) are listed
// under "System".
func toolListSection(names []string) string {
	var algaTools, systemTools []string
	for _, n := range names {
		if strings.HasPrefix(n, "alga_") {
			algaTools = append(algaTools, n)
		} else {
			systemTools = append(systemTools, n)
		}
	}
	var b strings.Builder
	if len(algaTools) > 0 {
		b.WriteString("### Alga Platform\n")
		for _, n := range algaTools {
			b.WriteString("- `")
			b.WriteString(n)
			b.WriteString("`\n")
		}
	}
	if len(systemTools) > 0 {
		b.WriteString("### System\n")
		for _, n := range systemTools {
			b.WriteString("- `")
			b.WriteString(n)
			b.WriteString("`\n")
		}
	}
	return b.String()
}
