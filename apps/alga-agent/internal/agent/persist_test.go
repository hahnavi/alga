package agent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"alga-agent/internal/llm"
)

func TestPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ss := NewSessionStore(20)
	ss.EnablePersistence(dir)

	id := "telegram:12345"
	s := ss.Get(id)
	s.AppendMessage(llm.Message{Role: "user", Content: "hello"})
	s.AppendMessage(llm.Message{Role: "assistant", Content: "hi there"})
	s.SetAlgaCtx(AlgaContext{InvestigationID: "inv-1", Severity: "high"})

	if err := ss.Persist(id); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, sessionFilename(id))); err != nil {
		t.Fatalf("session file missing: %v", err)
	}

	// Fresh store simulates a restart.
	ss2 := NewSessionStore(20)
	ss2.EnablePersistence(dir)
	s2 := ss2.Get(id)

	msgs := s2.Messages()
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Content != "hello" || msgs[1].Content != "hi there" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
	ctx := s2.AlgaContext()
	if ctx.InvestigationID != "inv-1" || ctx.Severity != "high" {
		t.Fatalf("unexpected alga ctx: %+v", ctx)
	}
}

func TestPersistDisabledIsNoop(t *testing.T) {
	ss := NewSessionStore(20)
	s := ss.Get("id")
	s.AppendMessage(llm.Message{Role: "user", Content: "x"})
	if err := ss.Persist("id"); err != nil {
		t.Fatalf("Persist without persistence: %v", err)
	}
}

func TestPersistUnknownSessionIsNoop(t *testing.T) {
	dir := t.TempDir()
	ss := NewSessionStore(20)
	ss.EnablePersistence(dir)
	if err := ss.Persist("never-seen"); err != nil {
		t.Fatalf("Persist unknown id: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("expected no files, got %d", len(entries))
	}
}

func TestClearRemovesFile(t *testing.T) {
	dir := t.TempDir()
	ss := NewSessionStore(20)
	ss.EnablePersistence(dir)

	id := "telegram:99"
	ss.Get(id).AppendMessage(llm.Message{Role: "user", Content: "x"})
	if err := ss.Persist(id); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	path := filepath.Join(dir, sessionFilename(id))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}

	ss.Clear(id)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be removed, stat err: %v", err)
	}
	if ss.Has(id) {
		t.Fatal("session should be gone from memory")
	}
}

// TestPersistClearNoResurrection is a regression test for the durable-deletion
// barrier: once Clear removes a session, an in-flight Persist that captured the
// session before Clear must not rename its tmp file back over the deleted path.
// Before the fix the rename was unconditional, so a Persist racing with Clear
// could resurrect a cleared conversation on disk.
func TestPersistClearNoResurrection(t *testing.T) {
	dir := t.TempDir()
	ss := NewSessionStore(20)
	ss.EnablePersistence(dir)
	id := "telegram:race"

	const rounds = 20
	for r := 0; r < rounds; r++ {
		s := ss.Get(id)
		s.AppendMessage(llm.Message{Role: "user", Content: "round"})
		if err := ss.Persist(id); err != nil {
			t.Fatalf("round %d seed Persist: %v", r, err)
		}

		// Spawn persists that capture the session before Clear fires.
		const k = 16
		var wg sync.WaitGroup
		for i := 0; i < k; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _ = ss.Persist(id) }()
		}
		// Clear concurrently; any persist surviving past Clear must not
		// resurrect the file. Persist never inserts into the map, so the
		// session stays gone after Clear.
		if err := ss.Clear(id); err != nil {
			t.Fatalf("round %d Clear: %v", r, err)
		}
		wg.Wait()

		path := filepath.Join(dir, sessionFilename(id))
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("round %d: session file resurrected after Clear (stat err=%v)", r, err)
		}
		if ss.Has(id) {
			t.Fatalf("round %d: session still in memory after Clear", r)
		}
	}
}

func TestCorruptFileFallsBackToFresh(t *testing.T) {
	dir := t.TempDir()
	id := "telegram:7"
	if err := os.WriteFile(filepath.Join(dir, sessionFilename(id)), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	ss := NewSessionStore(20)
	ss.EnablePersistence(dir)
	s := ss.Get(id)
	if got := len(s.Messages()); got != 0 {
		t.Fatalf("expected fresh session, got %d messages", got)
	}
}

func TestEvictIdleKeepsFileAndReloads(t *testing.T) {
	dir := t.TempDir()
	ss := NewSessionStore(20)
	ss.EnablePersistence(dir)

	id := "alga:abc"
	ss.Get(id).AppendMessage(llm.Message{Role: "user", Content: "keep me"})
	if err := ss.Persist(id); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	if n := ss.EvictIdle(0); n != 1 {
		t.Fatalf("evicted %d, want 1", n)
	}
	if ss.Has(id) {
		t.Fatal("session should be evicted from memory")
	}

	msgs := ss.Get(id).Messages()
	if len(msgs) != 1 || msgs[0].Content != "keep me" {
		t.Fatalf("expected reload from disk, got %+v", msgs)
	}
}

func TestSessionFilename(t *testing.T) {
	name := sessionFilename("telegram:12345")
	if strings.ContainsAny(name, ":/\\") {
		t.Fatalf("unsafe chars in %q", name)
	}
	if !strings.HasSuffix(name, ".json") {
		t.Fatalf("missing .json suffix: %q", name)
	}
	if !strings.HasPrefix(name, "telegram_12345-") {
		t.Fatalf("unexpected prefix: %q", name)
	}
	// Distinct ids that sanitize identically must still map to distinct files.
	if sessionFilename("a:b") == sessionFilename("a_b") {
		t.Fatal("colliding filenames for distinct ids")
	}
	// Deterministic.
	if sessionFilename("x") != sessionFilename("x") {
		t.Fatal("filename not deterministic")
	}
	// Traversal attempt stays a bare file name.
	if n := sessionFilename("../../etc/passwd"); filepath.Base(n) != n {
		t.Fatalf("traversal not neutralized: %q", n)
	}
	// Long ids are capped.
	if n := sessionFilename(strings.Repeat("a", 200)); len(n) > 64+1+8+5 {
		t.Fatalf("filename too long: %d", len(n))
	}
}

func TestPruneFiles(t *testing.T) {
	dir := t.TempDir()
	ss := NewSessionStore(20)
	ss.EnablePersistence(dir)

	oldFile := filepath.Join(dir, "old-00000000.json")
	newFile := filepath.Join(dir, "new-11111111.json")
	other := filepath.Join(dir, "notes.txt")
	for _, p := range []string{oldFile, newFile, other} {
		if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, past, past); err != nil {
		t.Fatal(err)
	}

	n, err := ss.PruneFiles(24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneFiles: %v", err)
	}
	if n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("old file should be pruned")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatal("new file should survive")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("non-json file should survive")
	}
}

func TestPruneFilesDisabled(t *testing.T) {
	ss := NewSessionStore(20)
	n, err := ss.PruneFiles(time.Hour)
	if err != nil || n != 0 {
		t.Fatalf("got n=%d err=%v, want 0,nil", n, err)
	}
}
