package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"alga-agent/internal/llm"
)

// sessionRecord is the on-disk representation of a session. Only data fields
// are persisted; mutexes and the ring-buffer cap stay runtime-only.
type sessionRecord struct {
	ID          string        `json:"id"`
	Created     time.Time     `json:"created"`
	LastActive  time.Time     `json:"last_active"`
	AlgaCtx     AlgaContext   `json:"alga_ctx"`
	DispatchCtx string        `json:"dispatch_ctx,omitempty"`
	Messages    []llm.Message `json:"messages"`
}

// EnablePersistence turns on JSON-file persistence under dir. Sessions are
// written by Persist (called by the agent core after each turn), lazily
// reloaded by Get after a restart or idle eviction, and deleted by Clear.
func (ss *SessionStore) EnablePersistence(dir string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.persistDir = dir
}

// sessionFilename maps a session id to a safe, deterministic file name. The
// sanitized prefix keeps files recognizable (e.g. alga_12345); the hash
// suffix guarantees uniqueness and prevents path traversal via crafted ids.
func sessionFilename(id string) string {
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, id)
	const maxPrefix = 64
	if len(sanitized) > maxPrefix {
		sanitized = sanitized[:maxPrefix]
	}
	sum := sha256.Sum256([]byte(id))
	return sanitized + "-" + hex.EncodeToString(sum[:])[:8] + ".json"
}

// Persist writes the session for id to disk atomically. It is a no-op when
// persistence is disabled or the session does not exist. Files are 0600:
// conversations can contain secrets and PII.
func (ss *SessionStore) Persist(id string) error {
	ss.mu.Lock()
	dir := ss.persistDir
	s := ss.sessions[id]
	ss.mu.Unlock()
	if dir == "" || s == nil {
		return nil
	}

	s.mu.Lock()
	rec := sessionRecord{
		ID:          id,
		Created:     s.created,
		LastActive:  s.lastActive,
		AlgaCtx:     s.AlgaCtx,
		DispatchCtx: s.dispatchCtx,
		Messages:    make([]llm.Message, len(s.messages)),
	}
	copy(rec.Messages, s.messages)
	s.mu.Unlock()

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal session %s: %w", id, err)
	}
	path := filepath.Join(dir, sessionFilename(id))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	// Re-check under the store lock: Clear may have removed the session (and
	// its file) while we marshalled and wrote the tmp file. Renaming now would
	// resurrect a cleared conversation, so drop the tmp and abort. The
	// check-then-rename is atomic with respect to Clear, which also holds
	// ss.mu when deleting.
	ss.mu.Lock()
	_, present := ss.sessions[id]
	if present {
		err = os.Rename(tmp, path)
	}
	ss.mu.Unlock()
	if !present {
		_ = os.Remove(tmp)
		return nil
	}
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename session file: %w", err)
	}
	return nil
}

// loadSession reads the persisted record for id, returning nil when the file
// is missing or unreadable/corrupt — callers fall back to a fresh session,
// and the file is overwritten on the next Persist. Caller must hold ss.mu.
func (ss *SessionStore) loadSession(id string) *Session {
	if ss.persistDir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(ss.persistDir, sessionFilename(id)))
	if err != nil {
		return nil
	}
	var rec sessionRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil
	}
	s := &Session{
		maxTurns:    ss.maxTurns,
		messages:    rec.Messages,
		AlgaCtx:     rec.AlgaCtx,
		dispatchCtx: rec.DispatchCtx,
		created:     rec.Created,
		lastActive:  time.Now(),
	}
	s.trim()
	return s
}

// removeSessionFile deletes the persisted file for id, if any. A missing file
// yields os.ErrNotExist; callers decide whether that is a failure.
func (ss *SessionStore) removeSessionFile(id string) error {
	if ss.persistDir == "" {
		return nil
	}
	return os.Remove(filepath.Join(ss.persistDir, sessionFilename(id)))
}

// PruneFiles deletes persisted session files whose last modification is older
// than olderThan (retention sweep). In-memory sessions are untouched; a
// pruned session simply starts fresh on its next message.
func (ss *SessionStore) PruneFiles(olderThan time.Duration) (int, error) {
	ss.mu.Lock()
	dir := ss.persistDir
	ss.mu.Unlock()
	if dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().Add(-olderThan)
	var n int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
			n++
		}
	}
	return n, nil
}
