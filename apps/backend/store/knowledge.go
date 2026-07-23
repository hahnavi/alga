package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"alga/config"
	"alga/ent"
	"alga/ent/knowledgenote"
	"alga/ent/predicate"
	"alga/matching"
	"alga/pgclient"

	entsql "entgo.io/ent/dialect/sql"
)

const (
	KnowledgeKindRunbook      = "runbook"
	KnowledgeKindKnownIssue   = "known_issue"
	KnowledgeKindServiceOwner = "service_owner"
	KnowledgeKindFact         = "fact"
)

const (
	KnowledgeAuthorUser  = "user"
	KnowledgeAuthorAgent = "agent"
)

type KnowledgeNote struct {
	ID                    uuid.UUID               `json:"id"`
	Kind                  string                  `json:"kind"`
	Title                 string                  `json:"title"`
	BodyMarkdown          string                  `json:"body_markdown"`
	Tags                  []string                `json:"tags,omitempty"`
	Selectors             []config.RouteCondition `json:"selectors,omitempty"`
	AuthorID              *uuid.UUID              `json:"author_id,omitempty"`
	AuthorType            string                  `json:"author_type"`
	AuthorName            string                  `json:"author_name,omitempty"`
	SourceInvestigationID string                  `json:"source_investigation_id,omitempty"`
	Confidence            *float64                `json:"confidence,omitempty"`
	ExpiresAt             *time.Time              `json:"expires_at,omitempty"`
	CreatedAt             time.Time               `json:"created_at"`
	UpdatedAt             time.Time               `json:"updated_at"`
}

type KnowledgeQuery struct {
	Kind       string
	Tag        string
	Text       string
	AuthorType string
	Limit      int
	Skip       int
}

func IsValidKnowledgeKind(s string) bool {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case KnowledgeKindRunbook, KnowledgeKindKnownIssue, KnowledgeKindServiceOwner, KnowledgeKindFact:
		return true
	default:
		return false
	}
}

type KnowledgeStore interface {
	Create(ctx context.Context, note *KnowledgeNote) (*KnowledgeNote, error)
	Update(ctx context.Context, id string, patch *KnowledgeNote) (*KnowledgeNote, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (*KnowledgeNote, error)
	List(ctx context.Context, q KnowledgeQuery) ([]KnowledgeNote, int64, error)
	Match(ctx context.Context, labels map[string]string, limit int) ([]KnowledgeNote, error)
}

func validateKnowledgeNote(n *KnowledgeNote) error {
	if !IsValidKnowledgeKind(n.Kind) {
		return fmt.Errorf("invalid kind %q", n.Kind)
	}
	n.Kind = strings.ToLower(strings.TrimSpace(n.Kind))
	n.Title = strings.TrimSpace(n.Title)
	if n.Title == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(n.BodyMarkdown) == "" {
		return errors.New("body_markdown is required")
	}
	if n.AuthorType != "" && n.AuthorType != KnowledgeAuthorUser && n.AuthorType != KnowledgeAuthorAgent {
		return fmt.Errorf("invalid author_type %q", n.AuthorType)
	}
	n.Tags = normalizeTags(n.Tags)
	return nil
}

func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func knowledgeConditionsMatch(conditions []config.RouteCondition, labels map[string]string) bool {
	if len(conditions) == 0 {
		return false
	}
	for _, c := range conditions {
		if !knowledgeConditionMatch(c, labels) {
			return false
		}
	}
	return true
}

func knowledgeConditionMatch(c config.RouteCondition, labels map[string]string) bool {
	field := strings.TrimSpace(c.Field)
	actual, exists := labels[field]
	op := strings.ToLower(strings.TrimSpace(c.Operator))
	value := c.Value
	switch op {
	case "exists":
		return exists && strings.TrimSpace(actual) != ""
	case "not_exists":
		return !exists || strings.TrimSpace(actual) == ""
	case "contains":
		return strings.Contains(actual, value)
	case "prefix":
		return strings.HasPrefix(actual, value)
	case "suffix":
		return strings.HasSuffix(actual, value)
	case "wildcard":
		return knowledgeWildcardMatch(value, actual)
	case "regex":
		// Compile via the shared cache; on error, fail closed (no match) so a
		// malformed rule does not block every matching note.
		re, err := matching.GetCompiledRegex(value)
		if err != nil {
			return false
		}
		return re.MatchString(actual)
	case "exact":
		fallthrough
	default:
		return actual == value
	}
}

func knowledgeWildcardMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(s, parts[i])
		if idx < 0 {
			return false
		}
		s = s[idx+len(parts[i]):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

type pgKnowledgeStore struct {
	pgStoreBase
	db *sql.DB
}

func newPGKnowledgeStore(client *ent.Client, db *sql.DB) KnowledgeStore {
	return &pgKnowledgeStore{pgStoreBase{client: client}, db}
}

func (s *pgKnowledgeStore) Create(ctx context.Context, note *KnowledgeNote) (*KnowledgeNote, error) {
	if note == nil {
		return nil, errors.New("nil note")
	}
	if err := validateKnowledgeNote(note); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	note.CreatedAt = now
	note.UpdatedAt = now
	if note.AuthorType == "" {
		note.AuthorType = KnowledgeAuthorUser
	}
	if note.Tags == nil {
		note.Tags = []string{}
	}

	b := s.client.KnowledgeNote.Create().
		SetKind(note.Kind).
		SetTitle(note.Title).
		SetBodyMarkdown(note.BodyMarkdown).
		SetTags(note.Tags).
		SetAuthorType(note.AuthorType).
		SetCreatedAt(now).
		SetUpdatedAt(now)

	if note.Selectors != nil {
		b.SetSelectors(routeConditionsToSchema(note.Selectors))
	}
	if note.AuthorID != nil {
		b.SetAuthorID(*note.AuthorID)
	}
	if note.AuthorName != "" {
		b.SetAuthorName(note.AuthorName)
	}
	if note.SourceInvestigationID != "" {
		b.SetSourceInvestigationID(note.SourceInvestigationID)
	}
	if note.Confidence != nil {
		b.SetConfidence(*note.Confidence)
	}
	if note.ExpiresAt != nil {
		b.SetExpiresAt(*note.ExpiresAt)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert knowledge note: %w", err)
	}

	note.ID = saved.ID
	return note, nil
}

func (s *pgKnowledgeStore) Update(ctx context.Context, id string, patch *KnowledgeNote) (*KnowledgeNote, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}

	b := s.client.KnowledgeNote.UpdateOneID(uid).SetUpdatedAt(time.Now().UTC())

	if strings.TrimSpace(patch.Kind) != "" {
		if !IsValidKnowledgeKind(patch.Kind) {
			return nil, fmt.Errorf("invalid kind %q", patch.Kind)
		}
		b.SetKind(strings.ToLower(strings.TrimSpace(patch.Kind)))
	}
	if strings.TrimSpace(patch.Title) != "" {
		b.SetTitle(strings.TrimSpace(patch.Title))
	}
	if patch.BodyMarkdown != "" {
		b.SetBodyMarkdown(patch.BodyMarkdown)
	}
	if patch.Tags != nil {
		b.SetTags(normalizeTags(patch.Tags))
	}
	if patch.Selectors != nil {
		b.SetSelectors(routeConditionsToSchema(patch.Selectors))
	}
	if patch.ExpiresAt != nil {
		b.SetExpiresAt(*patch.ExpiresAt)
	}
	if patch.Confidence != nil {
		b.SetConfidence(*patch.Confidence)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.New("knowledge note not found")
		}
		return nil, fmt.Errorf("failed to update knowledge note: %w", err)
	}
	return pgKnowledgeToRecord(saved), nil
}

func (s *pgKnowledgeStore) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}

	err = s.client.KnowledgeNote.DeleteOneID(uid).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errors.New("knowledge note not found")
		}
		return fmt.Errorf("failed to delete knowledge note: %w", err)
	}
	return nil
}

func (s *pgKnowledgeStore) Get(ctx context.Context, id string) (*KnowledgeNote, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}

	n, err := s.client.KnowledgeNote.Get(ctx, uid)
	if err != nil {
		return handleQueryErr[*KnowledgeNote](err, "knowledge note")
	}
	return pgKnowledgeToRecord(n), nil
}

func (s *pgKnowledgeStore) List(ctx context.Context, q KnowledgeQuery) ([]KnowledgeNote, int64, error) {
	// Free-text search uses a ranked PostgreSQL full-text query (token-AND,
	// quoted phrases, prefix-on-last-token, stemming) backed by the GIN index
	// created by pgclient.ApplyKnowledgeFTS. When there is no usable tsquery
	// (empty input, or a query of only punctuation/stopwords) or no raw DB
	// handle, fall through to the Ent builder path below, which applies the
	// legacy case-insensitive substring match.
	if tsq, ok := buildKnowledgeTSQuery(strings.TrimSpace(q.Text)); ok && s.db != nil {
		return s.listByTextFTS(ctx, q, tsq)
	}

	query := s.client.KnowledgeNote.Query()

	if kind := strings.TrimSpace(strings.ToLower(q.Kind)); kind != "" {
		query = query.Where(knowledgenote.Kind(kind))
	}
	if at := strings.TrimSpace(strings.ToLower(q.AuthorType)); at != "" {
		query = query.Where(knowledgenote.AuthorType(at))
	}
	if text := strings.TrimSpace(q.Text); text != "" {
		query = query.Where(knowledgenote.Or(
			knowledgenote.TitleContainsFold(text),
			knowledgenote.BodyMarkdownContainsFold(text),
		))
	}
	if tag := strings.TrimSpace(q.Tag); tag != "" {
		tagJSON, err := json.Marshal(strings.ToLower(tag))
		if err != nil {
			return nil, 0, fmt.Errorf("encode tag filter: %w", err)
		}
		query = query.Where(predicate.KnowledgeNote(func(sel *entsql.Selector) {
			sel.Where(entsql.ExprP("tags::jsonb @> ?::jsonb", string(tagJSON)))
		}))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count knowledge notes: %w", err)
	}

	query = query.Order(ent.Desc(knowledgenote.FieldUpdatedAt))
	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Skip > 0 {
		query = query.Offset(q.Skip)
	}

	notes, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list knowledge notes: %w", err)
	}

	out := make([]KnowledgeNote, 0, len(notes))
	for _, n := range notes {
		out = append(out, *pgKnowledgeToRecord(n))
	}
	return out, int64(total), nil
}

// knowledgeNonWord matches any run of characters that are not a letter or digit.
// Splitting on it yields clean barewords that are safe to embed in a to_tsquery
// value: such a bareword can never contain tsquery syntax characters
// (& | ! ( ) : < ->) or quotes, so user input cannot alter the query structure.
var knowledgeNonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// knowledgeQueryWords lowercases s and splits it on any run of non-letter /
// non-digit characters, returning the surviving sub-words (e.g. "PostgreSQL" ->
// ["postgresql"], "post-recovery" -> ["post", "recovery"]).
func knowledgeQueryWords(s string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(knowledgeNonWord.ReplaceAllString(s, " "))) {
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// knowledgeTerm is one parsed piece of a search query. A bare whitespace token
// is a token term; text inside double quotes is a phrase term whose words must
// appear adjacently. A single surviving word inside quotes is treated as an
// ordinary token (no adjacency constraint).
type knowledgeTerm struct {
	phrase bool
	words  []string
}

// parseKnowledgeTerms splits a raw search string into ordered terms. Double
// quotes group a phrase; single quotes are treated as ordinary characters (they
// appear inside words like "don't"). Whitespace separates bare tokens.
func parseKnowledgeTerms(s string) []knowledgeTerm {
	var terms []knowledgeTerm
	inQuote := false
	var buf strings.Builder
	flush := func(quoted bool) {
		words := knowledgeQueryWords(buf.String())
		buf.Reset()
		if len(words) == 0 {
			return
		}
		terms = append(terms, knowledgeTerm{phrase: quoted && len(words) > 1, words: words})
	}
	for _, r := range s {
		switch {
		case r == '"':
			flush(inQuote)
			inQuote = !inQuote
		case unicode.IsSpace(r):
			if inQuote {
				buf.WriteRune(r)
			} else {
				flush(false)
			}
		default:
			buf.WriteRune(r)
		}
	}
	flush(inQuote)
	return terms
}

// buildKnowledgeTSQuery parses a free-text search string into a PostgreSQL
// to_tsquery value string. Semantics:
//
//   - Bare whitespace tokens are AND-ed: "postgres recovery" -> postgres & recovery.
//   - Double-quoted segments are matched as adjacent phrases:
//     "cluster recovery" -> cluster <-> recovery.
//   - The final bare token (the one the user is still typing) is a prefix match:
//     "postgres rec" -> postgres & rec:*, so "rec" matches recovery/recovering.
//   - to_tsquery stems and stop-words each lexeme, so "recovering" matches
//     "recovery", and a query of only stopwords yields no terms.
//
// The returned string contains only sanitized barewords joined by the tsquery
// operators '&', '<->', and the prefix marker ':*'. ok is false when no usable
// lexemes survive (empty input or only punctuation/stopwords); callers should
// then fall back to the substring search.
func buildKnowledgeTSQuery(text string) (string, bool) {
	terms := parseKnowledgeTerms(text)
	if len(terms) == 0 {
		return "", false
	}
	lastTerm := len(terms) - 1
	termParts := make([]string, 0, len(terms))
	for ti, term := range terms {
		sep := " & "
		if term.phrase {
			sep = " <-> "
		}
		lastWord := len(term.words) - 1
		wordParts := make([]string, 0, len(term.words))
		for wi, w := range term.words {
			if ti == lastTerm && wi == lastWord && !term.phrase {
				w += ":*"
			}
			wordParts = append(wordParts, w)
		}
		termParts = append(termParts, strings.Join(wordParts, sep))
	}
	return strings.Join(termParts, " & "), true
}

// listByTextFTS runs the ranked full-text search path. tsq is a sanitized
// to_tsquery value string produced by buildKnowledgeTSQuery. The kind,
// author_type, and tag filters are applied in the same query so the count and
// the page stay consistent and ranking is computed before pagination. The
// tsvector expression matches pgclient.KnowledgeFTSExpression exactly so the
// GIN index is used.
func (s *pgKnowledgeStore) listByTextFTS(ctx context.Context, q KnowledgeQuery, tsq string) ([]KnowledgeNote, int64, error) {
	expr := pgclient.KnowledgeFTSExpression

	var (
		filterClauses []string
		args          []any
	)
	// $1 is reserved for the tsquery; filter parameters start at $2.
	args = append(args, tsq)
	paramIdx := 1
	addFilter := func(fragment string, value any) {
		paramIdx++
		filterClauses = append(filterClauses, fmt.Sprintf(fragment, paramIdx))
		args = append(args, value)
	}
	if k := strings.TrimSpace(strings.ToLower(q.Kind)); k != "" {
		addFilter("kind = $%d", k)
	}
	if at := strings.TrimSpace(strings.ToLower(q.AuthorType)); at != "" {
		addFilter("author_type = $%d", at)
	}
	if tag := strings.TrimSpace(q.Tag); tag != "" {
		tagJSON, err := json.Marshal(strings.ToLower(tag))
		if err != nil {
			return nil, 0, fmt.Errorf("encode tag filter: %w", err)
		}
		addFilter("tags::jsonb @> $%d::jsonb", string(tagJSON))
	}
	filterSQL := ""
	if len(filterClauses) > 0 {
		filterSQL = " AND " + strings.Join(filterClauses, " AND ")
	}

	countSQL := "SELECT COUNT(*) FROM knowledge_notes WHERE " + expr + " @@ to_tsquery('english', $1)" + filterSQL
	var total int64
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count knowledge notes (text search): %w", err)
	}

	listSQL := "SELECT id, kind, title, body_markdown, tags, selectors, author_id, author_type, " +
		"author_name, source_investigation_id, confidence, expires_at, created_at, updated_at, " +
		"ts_rank(" + expr + ", to_tsquery('english', $1)) AS rank " +
		"FROM knowledge_notes WHERE " + expr + " @@ to_tsquery('english', $1)" + filterSQL + " " +
		"ORDER BY rank DESC, updated_at DESC"

	listArgs := make([]any, len(args))
	copy(listArgs, args)
	if limit := q.Limit; limit > 0 {
		paramIdx++
		listSQL += fmt.Sprintf(" LIMIT $%d", paramIdx)
		listArgs = append(listArgs, limit)
	}
	if skip := q.Skip; skip > 0 {
		paramIdx++
		listSQL += fmt.Sprintf(" OFFSET $%d", paramIdx)
		listArgs = append(listArgs, skip)
	}

	rows, err := s.db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list knowledge notes (text search): %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]KnowledgeNote, 0)
	for rows.Next() {
		var (
			n             KnowledgeNote
			tagsJSON      sql.NullString
			selectorsJSON sql.NullString
			authorID      uuid.NullUUID
			confidence    sql.NullFloat64
			expiresAt     sql.NullTime
			rank          float64
		)
		if err := rows.Scan(
			&n.ID, &n.Kind, &n.Title, &n.BodyMarkdown, &tagsJSON, &selectorsJSON,
			&authorID, &n.AuthorType, &n.AuthorName, &n.SourceInvestigationID,
			&confidence, &expiresAt, &n.CreatedAt, &n.UpdatedAt, &rank,
		); err != nil {
			return nil, 0, fmt.Errorf("scan knowledge note (text search): %w", err)
		}
		if authorID.Valid {
			aid := authorID.UUID
			n.AuthorID = &aid
		}
		if confidence.Valid {
			c := confidence.Float64
			n.Confidence = &c
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			n.ExpiresAt = &t
		}
		n.Tags = scanStringSlice(tagsJSON)
		n.Selectors = scanRouteConditions(selectorsJSON)
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate knowledge notes (text search): %w", err)
	}
	return out, total, nil
}

func scanStringSlice(v sql.NullString) []string {
	if !v.Valid || v.String == "" || v.String == "null" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(v.String), &out); err != nil {
		return []string{}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func scanRouteConditions(v sql.NullString) []config.RouteCondition {
	if !v.Valid || v.String == "" || v.String == "null" {
		return []config.RouteCondition{}
	}
	var out []config.RouteCondition
	if err := json.Unmarshal([]byte(v.String), &out); err != nil {
		return []config.RouteCondition{}
	}
	if out == nil {
		out = []config.RouteCondition{}
	}
	return out
}

func (s *pgKnowledgeStore) Match(ctx context.Context, labels map[string]string, limit int) ([]KnowledgeNote, error) {
	if limit <= 0 {
		limit = 10
	}

	now := time.Now().UTC()
	query := s.client.KnowledgeNote.Query().
		Where(
			knowledgenote.SelectorsNotNil(),
			knowledgenote.Or(
				knowledgenote.ExpiresAtIsNil(),
				knowledgenote.ExpiresAtGT(now),
			),
		).
		Order(ent.Desc(knowledgenote.FieldUpdatedAt)).
		Limit(limit * 5)

	notes, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query knowledge notes for match: %w", err)
	}

	matched := make([]KnowledgeNote, 0, len(notes))
	for _, n := range notes {
		rec := pgKnowledgeToRecord(n)
		if knowledgeConditionsMatch(rec.Selectors, labels) {
			matched = append(matched, *rec)
			if len(matched) >= limit {
				break
			}
		}
	}
	if matched == nil {
		matched = []KnowledgeNote{}
	}
	return matched, nil
}

func pgKnowledgeToRecord(n *ent.KnowledgeNote) *KnowledgeNote {
	var selectors []config.RouteCondition
	if n.Selectors != nil {
		selectors = routeConditionsFromSchema(n.Selectors)
	}
	if selectors == nil {
		selectors = []config.RouteCondition{}
	}

	var authorID *uuid.UUID
	if n.AuthorID != nil {
		uid := *n.AuthorID
		authorID = &uid
	}

	var tags []string
	if n.Tags != nil {
		tags = n.Tags
	} else {
		tags = []string{}
	}

	return &KnowledgeNote{
		ID:                    n.ID,
		Kind:                  n.Kind,
		Title:                 n.Title,
		BodyMarkdown:          n.BodyMarkdown,
		Tags:                  tags,
		Selectors:             selectors,
		AuthorID:              authorID,
		AuthorType:            n.AuthorType,
		AuthorName:            n.AuthorName,
		SourceInvestigationID: n.SourceInvestigationID,
		Confidence:            n.Confidence,
		ExpiresAt:             n.ExpiresAt,
		CreatedAt:             n.CreatedAt,
		UpdatedAt:             n.UpdatedAt,
	}
}
