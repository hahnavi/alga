package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TodoTool provides per-session task tracking for the agent. It maintains an
// in-memory list of tasks that the agent can create, update, and list during an
// investigation. Ported from hermes-agent's todo_tool.py, simplified to
// in-memory storage scoped to the agent process lifetime.
type TodoTool struct {
	mu    sync.Mutex
	items []todoItem
	seq   int
}

type todoItem struct {
	ID        int       `json:"id"`
	Task      string    `json:"task"`
	Status    string    `json:"status"`
	Priority  string    `json:"priority,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	DoneAt    time.Time `json:"done_at,omitempty"`
}

type todoInput struct {
	Action   string `json:"action" desc:"Action: \"add\", \"done\", \"list\", \"remove\", or \"clear\"."`
	Task     string `json:"task,omitempty" desc:"Task description (for add)."`
	ID       int    `json:"id,omitempty" desc:"Task ID (for done/remove)."`
	Priority string `json:"priority,omitempty" desc:"Priority: high, medium, low (for add)."`
}

type todoOutput struct {
	Items   []todoItem `json:"items,omitempty"`
	Message string     `json:"message,omitempty"`
}

// NewTodoTool constructs a TodoTool.
func NewTodoTool() *TodoTool {
	return &TodoTool{}
}

func (t *TodoTool) handle(ctx context.Context, in todoInput) Result[todoOutput] {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch in.Action {
	case "add":
		if in.Task == "" {
			return ErrMsg[todoOutput]("task is required for add")
		}
		t.seq++
		priority := in.Priority
		if priority == "" {
			priority = "medium"
		}
		item := todoItem{
			ID:        t.seq,
			Task:      in.Task,
			Status:    "pending",
			Priority:  priority,
			CreatedAt: time.Now(),
		}
		t.items = append(t.items, item)
		return OK(todoOutput{Message: fmt.Sprintf("added task #%d: %s", item.ID, item.Task), Items: t.items})

	case "done":
		if in.ID <= 0 {
			return ErrMsg[todoOutput]("id is required for done")
		}
		for i := range t.items {
			if t.items[i].ID == in.ID {
				t.items[i].Status = "done"
				t.items[i].DoneAt = time.Now()
				return OK(todoOutput{Message: fmt.Sprintf("task #%d marked done", in.ID), Items: t.items})
			}
		}
		return ErrMsg[todoOutput](fmt.Sprintf("task #%d not found", in.ID))

	case "remove":
		if in.ID <= 0 {
			return ErrMsg[todoOutput]("id is required for remove")
		}
		for i := range t.items {
			if t.items[i].ID == in.ID {
				t.items = append(t.items[:i], t.items[i+1:]...)
				return OK(todoOutput{Message: fmt.Sprintf("removed task #%d", in.ID), Items: t.items})
			}
		}
		return ErrMsg[todoOutput](fmt.Sprintf("task #%d not found", in.ID))

	case "list":
		return OK(todoOutput{Items: t.items})

	case "clear":
		t.items = nil
		t.seq = 0
		return OK(todoOutput{Message: "all tasks cleared"})

	default:
		return ErrMsg[todoOutput](fmt.Sprintf("unknown action %q (use add, done, list, remove, clear)", in.Action))
	}
}

// RegisterTodoTool registers the todo tool.
func RegisterTodoTool(reg *Registry) {
	t := NewTodoTool()
	reg.Register(NewTypedTool("todo",
		"Track investigation tasks. Actions: add (new task), done (mark complete), list (show all), remove (delete one), clear (delete all).",
		t.handle, WithCategory[todoInput, todoOutput]("System")))
}
