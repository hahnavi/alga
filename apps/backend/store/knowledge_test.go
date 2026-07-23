package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"alga/ent"
	"alga/pgclient"
)

// newKnowledgeTestStore provisions an isolated PostgreSQL schema, applies the
// ent schema, creates the GIN full-text index (pgclient.ApplyKnowledgeFTS), and
// returns a knowledge store backed by a raw *sql.DB so the ranked text-search
// path can be exercised against real PostgreSQL.
func newKnowledgeTestStore(t *testing.T) KnowledgeStore {
	t.Helper()

	baseDSN := resolveTestDSN(t)
	schema := "alga_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	if err := createTestSchema(baseDSN, schema); err != nil {
		t.Fatalf("create test schema %s: %v", schema, err)
	}
	testDSN, err := dsnWithSearchPath(baseDSN, schema)
	if err != nil {
		t.Fatalf("build test dsn: %v", err)
	}

	db, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("open test connection: %v", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Schema.Create(ctx); err != nil {
		_ = client.Close()
		_ = db.Close()
		_ = dropTestSchema(baseDSN, schema)
		t.Fatalf("apply ent schema: %v", err)
	}
	if err := pgclient.ApplyKnowledgeFTS(ctx, db); err != nil {
		_ = client.Close()
		_ = db.Close()
		_ = dropTestSchema(baseDSN, schema)
		t.Fatalf("apply knowledge fts: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
		_ = dropTestSchema(baseDSN, schema)
	})

	return newPGKnowledgeStore(client, db)
}

func mustCreateKnowledgeNote(t *testing.T, s KnowledgeStore, kind, title, body string, tags []string) *KnowledgeNote {
	t.Helper()
	n, err := s.Create(context.Background(), &KnowledgeNote{
		Kind:         kind,
		Title:        title,
		BodyMarkdown: body,
		Tags:         tags,
	})
	if err != nil {
		t.Fatalf("create knowledge note %q: %v", title, err)
	}
	return n
}

func knowledgeIDs(notes []KnowledgeNote) map[string]struct{} {
	out := make(map[string]struct{}, len(notes))
	for i := range notes {
		out[notes[i].ID.String()] = struct{}{}
	}
	return out
}

func TestBuildKnowledgeTSQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"empty", "", "", false},
		{"punctuation only", "!!! ??? ", "", false},
		{"single token gets prefix", "postgres", "postgres:*", true},
		{"two tokens AND with last prefix", "postgres recovery", "postgres & recovery:*", true},
		{"hyphenated splits into anded words", "post-recovery", "post & recovery:*", true},
		{"quoted phrase is adjacency", `"cluster recovery"`, "cluster <-> recovery", true},
		{"phrase then token", `"cluster recovery" postgres`, "cluster <-> recovery & postgres:*", true},
		{"token then phrase no prefix on phrase", `postgres "cluster recovery"`, "postgres & cluster <-> recovery", true},
		{"case insensitive", "PostgreSQL Recovery", "postgresql & recovery:*", true},
		{"strips injected tsquery operators", "a&b|c", "a & b & c:*", true},
		{"quoted single word behaves as token", `"postgres"`, "postgres:*", true},
		{"trailing space keeps prefix", "postgres ", "postgres:*", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := buildKnowledgeTSQuery(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Errorf("buildKnowledgeTSQuery(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestKnowledgeSearch(t *testing.T) {
	s := newKnowledgeTestStore(t)
	ctx := context.Background()

	pg := mustCreateKnowledgeNote(t, s, KnowledgeKindRunbook,
		"PostgreSQL recovery runbook",
		"Steps to restore a PostgreSQL cluster after a crash.",
		[]string{"postgres", "recovery"})
	redis := mustCreateKnowledgeNote(t, s, KnowledgeKindRunbook,
		"Redis failover",
		"How to fail over a Redis sentinel cluster.",
		[]string{"redis"})
	cluster := mustCreateKnowledgeNote(t, s, KnowledgeKindKnownIssue,
		"Cluster recovery notes",
		"General cluster recovery procedures.",
		[]string{})
	_ = mustCreateKnowledgeNote(t, s, KnowledgeKindFact,
		"Unrelated",
		"Sometimes the network is slow and nothing recovers.",
		[]string{})

	wantIDs := func(ids ...*KnowledgeNote) map[string]struct{} {
		out := make(map[string]struct{}, len(ids))
		for _, n := range ids {
			out[n.ID.String()] = struct{}{}
		}
		return out
	}

	list := func(text string) []KnowledgeNote {
		t.Helper()
		notes, total, err := s.List(ctx, KnowledgeQuery{Text: text, Limit: 50})
		if err != nil {
			t.Fatalf("List(%q): %v", text, err)
		}
		if int(total) != len(notes) {
			t.Fatalf("List(%q) total=%d but got %d rows", text, total, len(notes))
		}
		return notes
	}

	mustHave := func(text string, want map[string]struct{}) {
		t.Helper()
		notes := list(text)
		got := knowledgeIDs(notes)
		if len(got) != len(want) {
			t.Fatalf("List(%q) = %v, want exactly %v", text, got, want)
		}
		for id := range want {
			if _, ok := got[id]; !ok {
				t.Fatalf("List(%q) = %v, missing %s", text, got, id)
			}
		}
	}

	// Single token (prefix): "postgres" matches PostgreSQL in the runbook.
	mustHave("postgres", wantIDs(pg))

	// Token-AND: "cluster recovery" requires BOTH words. Matches pg (has both in
	// body/title) and cluster (both in title/body), but not redis/unrelated.
	mustHave("cluster recovery", wantIDs(pg, cluster))

	// Quoted phrase requires adjacency. "cluster recovery" is adjacent only in
	// the cluster note, demonstrating the phrase vs token-AND distinction.
	mustHave(`"cluster recovery"`, wantIDs(cluster))

	// Stemming (non-last token is stemmed): "restoring" -> lexeme "restor"
	// matches "restore" in the PostgreSQL runbook.
	mustHave("restoring cluster", wantIDs(pg))

	// Prefix on the last token: "postgr" -> matches the "postgresql" lexeme.
	mustHave("postgr", wantIDs(pg))

	// Combined filters: kind + text.
	mustHaveKindText := func(kind, text string, want map[string]struct{}) {
		t.Helper()
		notes, _, err := s.List(ctx, KnowledgeQuery{Kind: kind, Text: text, Limit: 50})
		if err != nil {
			t.Fatalf("List(kind=%q,text=%q): %v", kind, text, err)
		}
		got := knowledgeIDs(notes)
		if len(got) != len(want) {
			t.Fatalf("List(kind=%q,text=%q) = %v, want exactly %v", kind, text, got, want)
		}
		for id := range want {
			if _, ok := got[id]; !ok {
				t.Fatalf("List(kind=%q,text=%q) = %v, missing %s", kind, text, got, id)
			}
		}
	}
	mustHaveKindText(KnowledgeKindRunbook, "redis", wantIDs(redis))
	mustHaveKindText(KnowledgeKindKnownIssue, "cluster", wantIDs(cluster))

	// Tag filter + text.
	notesByTag, _, err := s.List(ctx, KnowledgeQuery{Tag: "redis", Text: "failover", Limit: 50})
	if err != nil {
		t.Fatalf("List(tag=redis,failover): %v", err)
	}
	if got := knowledgeIDs(notesByTag); len(got) != 1 {
		t.Fatalf("List(tag=redis,failover) = %v, want only redis note", got)
	}

	// Ranking: a title hit outranks a body hit.
	ranked, _, err := s.List(ctx, KnowledgeQuery{Text: "redis", Limit: 50})
	if err != nil {
		t.Fatalf("List(redis) ranking: %v", err)
	}
	if len(ranked) == 0 || ranked[0].ID != redis.ID {
		t.Fatalf("List(redis) ranking: expected redis title hit first, got %v", knowledgeIDs(ranked))
	}
}
