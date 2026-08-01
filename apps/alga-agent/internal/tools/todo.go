package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// maxTodoSessions bounds the number of per-session todo lists retained in
// memory. When exceeded, the least-recently-used session's list is evicted so
// the map cannot grow without bound across many distinct chats.
const maxTodoSessions = 256

// TodoTool provides per-session task tracking for the agent. Each chat/session
// gets its own in-memory list, keyed by the chat id resolved from the call
// context, so concurrent investigations do not share or clobber each other's
// tasks. Ported from hermes-agent's todo_tool.py, simplified to in-memory
// storage scoped to the agent process lifetime.
type TodoTool struct {
	mu       sync.Mutex
	sessions map[string]*todoSession
}

type todoSession struct {
	items      []todoItem
	seq        int
	lastAccess time.Time
}

type todoItem struct {
	ID        int        `json:"id"`
	Task      string     `json:"task"`
	Status    string     `json:"status"`
	Priority  string     `json:"priority,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	DoneAt    *time.Time `json:"done_at,omitempty"`
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
	return &TodoTool{sessions: make(map[string]*todoSession)}
}

// todoSessionKey resolves the per-session key from the call context, preferring
// the Alga chat id, then the session id. Returns an error when no session
// context is available so standalone turns cannot share a global todo list.
func todoSessionKey(ctx context.Context) (string, error) {
	if key, err := chatIDFromCtx(ctx, nil); err == nil && key != "" {
		return key, nil
	}
	if cc, ok := CallContextFrom(ctx); ok && cc.SessionID != "" {
		return cc.SessionID, nil
	}
	return "", fmt.Errorf("todo: no session context (requires an Alga chat or session id)")
}

// session returns the todo list for key, creating it on first use. It evicts
// the least-recently-used session when the bound is reached. Callers must hold
// t.mu.
func (t *TodoTool) session(key string) *todoSession {
	s, ok := t.sessions[key]
	if !ok {
		if len(t.sessions) >= maxTodoSessions {
			t.evictOldest()
		}
		s = &todoSession{}
		t.sessions[key] = s
	}
	s.lastAccess = time.Now()
	return s
}

// evictOldest removes the least-recently-accessed session. Callers must hold
// t.mu and have already verified the map is non-empty.
func (t *TodoTool) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, s := range t.sessions {
		if first || s.lastAccess.Before(oldestTime) {
			oldestKey, oldestTime, first = k, s.lastAccess, false
		}
	}
	delete(t.sessions, oldestKey)
}

// snapshot returns a copy of the session's items so callers never receive a
// reference to the internal slice. Callers must hold t.mu.
func (t *TodoTool) snapshot(s *todoSession) []todoItem {
	out := make([]todoItem, len(s.items))
	copy(out, s.items)
	return out
}

func (t *TodoTool) handle(ctx context.Context, in todoInput) Result[todoOutput] {
	key, err := todoSessionKey(ctx)
	if err != nil {
		return Err[todoOutput](err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	s := t.session(key)

	switch in.Action {
	case "add":
		if in.Task == "" {
			return ErrMsg[todoOutput]("task is required for add")
		}
		s.seq++
		priority := in.Priority
		if priority == "" {
			priority = "medium"
		}
		item := todoItem{
			ID:        s.seq,
			Task:      in.Task,
			Status:    "pending",
			Priority:  priority,
			CreatedAt: time.Now(),
		}
		s.items = append(s.items, item)
		return OK(todoOutput{Message: fmt.Sprintf("added task #%d: %s", item.ID, item.Task), Items: t.snapshot(s)})

	case "done":
		if in.ID <= 0 {
			return ErrMsg[todoOutput]("id is required for done")
		}
		for i := range s.items {
			if s.items[i].ID == in.ID {
				now := time.Now()
				s.items[i].Status = "done"
				s.items[i].DoneAt = &now
				return OK(todoOutput{Message: fmt.Sprintf("task #%d marked done", in.ID), Items: t.snapshot(s)})
			}
		}
		return ErrMsg[todoOutput](fmt.Sprintf("task #%d not found", in.ID))

	case "remove":
		if in.ID <= 0 {
			return ErrMsg[todoOutput]("id is required for remove")
		}
		for i := range s.items {
			if s.items[i].ID == in.ID {
				s.items = append(s.items[:i], s.items[i+1:]...)
				return OK(todoOutput{Message: fmt.Sprintf("removed task #%d", in.ID), Items: t.snapshot(s)})
			}
		}
		return ErrMsg[todoOutput](fmt.Sprintf("task #%d not found", in.ID))

	case "list":
		return OK(todoOutput{Items: t.snapshot(s)})

	case "clear":
		s.items = nil
		s.seq = 0
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
