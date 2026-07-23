package tools

import (
	"context"

	alga "github.com/alga/agent-sdk-go"
)

// --- Knowledge & memory ---

type searchKnowledgeInput struct {
	Query string `json:"query,omitempty"`
	Kind  string `json:"kind,omitempty" desc:"Knowledge kind (runbook, postmortem, note)"`
	Limit int    `json:"limit,omitempty"`
}

type searchKnowledgeOutput struct {
	Notes []alga.KnowledgeNote `json:"notes"`
	Total int                  `json:"total"`
	Count int                  `json:"count"`
}

type createKnowledgeInput struct {
	Kind         string   `json:"kind" desc:"Knowledge kind (runbook, postmortem, note)"`
	Title        string   `json:"title"`
	BodyMarkdown string   `json:"body_markdown" desc:"Markdown body"`
	Tags         []string `json:"tags,omitempty"`
}

type listMemoriesInput struct {
	MemoryType      string `json:"memory_type,omitempty"`
	InvestigationID string `json:"investigation_id,omitempty"`
	Limit           int    `json:"limit,omitempty"`
}

type createMemoryInput struct {
	Content    string            `json:"content" desc:"The memory content"`
	MemoryType string            `json:"memory_type" desc:"e.g. observation, learning, preference"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type deleteMemoryInput struct {
	MemoryID string `json:"memory_id"`
}

func knowledgeMemoryTools(c AlgaClient) []Tool {
	return []Tool{
		NewTypedTool("alga_search_knowledge",
			"Search the Alga knowledge base.",
			func(ctx context.Context, in searchKnowledgeInput) Result[searchKnowledgeOutput] {
				params := map[string]string{}
				if in.Query != "" {
					params["q"] = in.Query
				}
				if in.Kind != "" {
					params["kind"] = in.Kind
				}
				if in.Limit > 0 {
					params["limit"] = itoa(in.Limit)
				}
				resp, err := c.ListKnowledge(ctx, params)
				if err != nil {
					return Err[searchKnowledgeOutput](algaErr(err))
				}
				notes := resp.All()
				return OK(searchKnowledgeOutput{Notes: notes, Total: resp.Total, Count: len(notes)})
			},
			WithCategory[searchKnowledgeInput, searchKnowledgeOutput](algaCategory),
		),

		NewTypedTool("alga_create_knowledge",
			"Create a knowledge note in Alga (runbook, postmortem, etc.).",
			func(ctx context.Context, in createKnowledgeInput) Result[*alga.KnowledgeNote] {
				if in.Kind == "" || in.Title == "" || in.BodyMarkdown == "" {
					return ErrMsg[*alga.KnowledgeNote]("kind, title, and body_markdown are required")
				}
				params := map[string]any{
					"kind":          in.Kind,
					"title":         in.Title,
					"body_markdown": in.BodyMarkdown,
				}
				if len(in.Tags) > 0 {
					params["tags"] = in.Tags
				}
				note, err := c.CreateKnowledge(ctx, params)
				if err != nil {
					return Err[*alga.KnowledgeNote](algaErr(err))
				}
				return OK(note)
			},
			WithCategory[createKnowledgeInput, *alga.KnowledgeNote](algaCategory),
			WithCapability[createKnowledgeInput, *alga.KnowledgeNote]("investigate"),
		),

		NewTypedTool("alga_list_memories",
			"List agent memories with optional filter.",
			func(ctx context.Context, in listMemoriesInput) Result[*alga.MemoryListResponse] {
				params := map[string]string{}
				if in.MemoryType != "" {
					params["memory_type"] = in.MemoryType
				}
				id := in.InvestigationID
				if id == "" {
					if cc, ok := CallContextFrom(ctx); ok {
						id = cc.AlgaInvestigationID
					}
				}
				if id != "" {
					params["investigation_id"] = id
				}
				if in.Limit > 0 {
					params["limit"] = itoa(in.Limit)
				}
				resp, err := c.ListMemories(ctx, params)
				if err != nil {
					return Err[*alga.MemoryListResponse](algaErr(err))
				}
				return OK(resp)
			},
			WithCategory[listMemoriesInput, *alga.MemoryListResponse](algaCategory),
		),

		NewTypedTool("alga_create_memory",
			"Store a long-term memory in Alga (observation, learning, preference).",
			func(ctx context.Context, in createMemoryInput) Result[*alga.Memory] {
				if in.Content == "" || in.MemoryType == "" {
					return ErrMsg[*alga.Memory]("content and memory_type are required")
				}
				params := map[string]any{
					"content":     in.Content,
					"memory_type": in.MemoryType,
				}
				if in.Labels != nil {
					params["labels"] = in.Labels
				}
				if cc, ok := CallContextFrom(ctx); ok && cc.AlgaInvestigationID != "" {
					params["investigation_id"] = cc.AlgaInvestigationID
				}
				mem, err := c.CreateMemory(ctx, params)
				if err != nil {
					return Err[*alga.Memory](algaErr(err))
				}
				return OK(mem)
			},
			WithCategory[createMemoryInput, *alga.Memory](algaCategory),
			WithCapability[createMemoryInput, *alga.Memory]("investigate"),
		),

		NewTypedTool("alga_delete_memory",
			"Delete a memory by id.",
			func(ctx context.Context, in deleteMemoryInput) Result[struct {
				OK      bool   `json:"ok"`
				Deleted string `json:"deleted"`
			}] {
				if in.MemoryID == "" {
					return ErrMsg[struct {
						OK      bool   `json:"ok"`
						Deleted string `json:"deleted"`
					}]("memory_id is required")
				}
				if err := c.DeleteMemory(ctx, in.MemoryID); err != nil {
					return Err[struct {
						OK      bool   `json:"ok"`
						Deleted string `json:"deleted"`
					}](algaErr(err))
				}
				return OK(struct {
					OK      bool   `json:"ok"`
					Deleted string `json:"deleted"`
				}{OK: true, Deleted: in.MemoryID})
			},
			WithCategory[deleteMemoryInput, struct {
				OK      bool   `json:"ok"`
				Deleted string `json:"deleted"`
			}](algaCategory),
			WithCapability[deleteMemoryInput, struct {
				OK      bool   `json:"ok"`
				Deleted string `json:"deleted"`
			}]("investigate"),
		),
	}
}
