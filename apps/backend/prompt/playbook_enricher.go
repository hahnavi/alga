package prompt

import (
	"context"
	"fmt"
	"strings"

	"alga/store"
)

type PlaybookEnricher struct {
	playbookStore store.PlaybookStore
}

func NewPlaybookEnricher(playbookStore store.PlaybookStore) *PlaybookEnricher {
	return &PlaybookEnricher{playbookStore: playbookStore}
}

func (e *PlaybookEnricher) Enrich(ctx context.Context, labels map[string]string) string {
	if e.playbookStore == nil {
		return ""
	}
	playbooks, err := e.playbookStore.FindMatching(ctx, labels)
	if err != nil || len(playbooks) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## Relevant Playbooks\n\n")
	for _, p := range playbooks {
		fmt.Fprintf(&b, "### %s (%s)\n", p.Title, p.Kind)
		if p.Summary != "" {
			b.WriteString(p.Summary)
			b.WriteString("\n")
		}
		_, steps, err := e.playbookStore.Get(ctx, p.ID)
		if err == nil && len(steps) > 0 {
			b.WriteString("Steps:\n")
			for _, s := range steps {
				fmt.Fprintf(&b, "%d. %s", s.StepNumber, s.Title)
				if s.ExpectedDuration != "" {
					fmt.Fprintf(&b, " (est. %s)", s.ExpectedDuration)
				}
				b.WriteString("\n")
				if s.Description != "" {
					fmt.Fprintf(&b, "   %s\n", s.Description)
				}
				if s.Command != "" {
					fmt.Fprintf(&b, "   Command: `%s`\n", s.Command)
				}
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
