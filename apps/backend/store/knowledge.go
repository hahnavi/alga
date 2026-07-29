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
	"github.com/uptrace/bun"

	"alga/config"
	"alga/db/models"
	"alga/matching"
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

// knowledgeFTSExpression is the immutable, normalized full-text expression
// indexed on knowledge_notes (title + body + tags). It MUST stay byte-identical
// between the GIN expression index and the WHERE clause queries.
const knowledgeFTSExpression = `to_tsvector('english', coalesce(title, '') || ' ' || coalesce(body_markdown, '') || ' ' || coalesce(tags::text, ''))`

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
}

func newPGKnowledgeStore(db *bun.DB) KnowledgeStore {
	return &pgKnowledgeStore{pgStoreBase{db: db}}
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

	m := &models.KnowledgeNote{
		Kind:                  note.Kind,
		Title:                 note.Title,
		BodyMarkdown:          note.BodyMarkdown,
		Tags:                  note.Tags,
		AuthorID:              note.AuthorID,
		AuthorType:            note.AuthorType,
		AuthorName:            note.AuthorName,
		SourceInvestigationID: note.SourceInvestigationID,
		Confidence:            note.Confidence,
		ExpiresAt:             note.ExpiresAt,
	}
	m.ID = models.NewUUID()
	m.CreatedAt = now
	m.UpdatedAt = now

	if note.Selectors != nil {
		m.Selectors = routeConditionsToModels(note.Selectors)
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert knowledge note: %w", err)
	}

	note.ID = m.ID
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

	upd := s.db.NewUpdate().Model((*models.KnowledgeNote)(nil)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", uid)

	if strings.TrimSpace(patch.Kind) != "" {
		if !IsValidKnowledgeKind(patch.Kind) {
			return nil, fmt.Errorf("invalid kind %q", patch.Kind)
		}
		upd = upd.Set("kind = ?", strings.ToLower(strings.TrimSpace(patch.Kind)))
	}
	if strings.TrimSpace(patch.Title) != "" {
		upd = upd.Set("title = ?", strings.TrimSpace(patch.Title))
	}
	if patch.BodyMarkdown != "" {
		upd = upd.Set("body_markdown = ?", patch.BodyMarkdown)
	}
	if patch.Tags != nil {
		upd = upd.Set("tags = ?", normalizeTags(patch.Tags))
	}
	if patch.Selectors != nil {
		upd = upd.Set("selectors = ?", routeConditionsToModels(patch.Selectors))
	}
	if patch.ExpiresAt != nil {
		upd = upd.Set("expires_at = ?", *patch.ExpiresAt)
	}
	if patch.Confidence != nil {
		upd = upd.Set("confidence = ?", *patch.Confidence)
	}

	res, err := upd.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update knowledge note: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update knowledge note: %w", err)
	}
	if n == 0 {
		return nil, errors.New("knowledge note not found")
	}

	var m models.KnowledgeNote
	err = s.db.NewSelect().Model(&m).Where("id = ?", uid).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reload knowledge note: %w", err)
	}
	return pgKnowledgeToRecord(&m), nil
}

func (s *pgKnowledgeStore) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}

	res, err := s.db.NewDelete().Model((*models.KnowledgeNote)(nil)).Where("id = ?", uid).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete knowledge note: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete knowledge note: %w", err)
	}
	if n == 0 {
		return errors.New("knowledge note not found")
	}
	return nil
}

func (s *pgKnowledgeStore) Get(ctx context.Context, id string) (*KnowledgeNote, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}

	var m models.KnowledgeNote
	err = s.db.NewSelect().Model(&m).Where("id = ?", uid).Scan(ctx)
	if err != nil {
		return handleQueryErr[*KnowledgeNote](err, "knowledge note")
	}
	return pgKnowledgeToRecord(&m), nil
}

func (s *pgKnowledgeStore) List(ctx context.Context, q KnowledgeQuery) ([]KnowledgeNote, int64, error) {
	// Free-text search uses a ranked PostgreSQL full-text query (token-AND,
	// quoted phrases, prefix-on-last-token, stemming) backed by the GIN index.
	// When there is no usable tsquery (empty input, or a query of only
	// punctuation/stopwords), fall through to the Bun builder path below, which
	// applies the legacy case-insensitive substring match.
	if tsq, ok := buildKnowledgeTSQuery(strings.TrimSpace(q.Text)); ok {
		return s.listByTextFTS(ctx, q, tsq)
	}

	countQ := s.db.NewSelect().Model((*models.KnowledgeNote)(nil))
	listQ := s.db.NewSelect().Model((*models.KnowledgeNote)(nil))

	if kind := strings.TrimSpace(strings.ToLower(q.Kind)); kind != "" {
		countQ = countQ.Where("kind = ?", kind)
		listQ = listQ.Where("kind = ?", kind)
	}
	if at := strings.TrimSpace(strings.ToLower(q.AuthorType)); at != "" {
		countQ = countQ.Where("author_type = ?", at)
		listQ = listQ.Where("author_type = ?", at)
	}
	if text := strings.TrimSpace(q.Text); text != "" {
		countQ = countQ.Where("(title ILIKE ? OR body_markdown ILIKE ?)", "%"+text+"%", "%"+text+"%")
		listQ = listQ.Where("(title ILIKE ? OR body_markdown ILIKE ?)", "%"+text+"%", "%"+text+"%")
	}
	if tag := strings.TrimSpace(q.Tag); tag != "" {
		tagJSON, err := json.Marshal(strings.ToLower(tag))
		if err != nil {
			return nil, 0, fmt.Errorf("encode tag filter: %w", err)
		}
		countQ = countQ.Where("tags::jsonb @> ?::jsonb", string(tagJSON))
		listQ = listQ.Where("tags::jsonb @> ?::jsonb", string(tagJSON))
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count knowledge notes: %w", err)
	}

	listQ = listQ.OrderExpr("updated_at DESC")
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}

	var notes []models.KnowledgeNote
	err = listQ.Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list knowledge notes: %w", err)
	}

	out := make([]KnowledgeNote, 0, len(notes))
	for _, n := range notes {
		out = append(out, *pgKnowledgeToRecord(&n))
	}
	return out, int64(total), nil
}

// knowledgeNonWord matches any run of characters that are not a letter or digit.
var knowledgeNonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// knowledgeQueryWords lowercases s and splits it on any run of non-letter /
// non-digit characters, returning the surviving sub-words.
func knowledgeQueryWords(s string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(knowledgeNonWord.ReplaceAllString(s, " "))) {
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// knowledgeTerm is one parsed piece of a search query.
type knowledgeTerm struct {
	phrase bool
	words  []string
}

// parseKnowledgeTerms splits a raw search string into ordered terms.
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
// to_tsquery value string.
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

// listByTextFTS runs the ranked full-text search path.
func (s *pgKnowledgeStore) listByTextFTS(ctx context.Context, q KnowledgeQuery, tsq string) ([]KnowledgeNote, int64, error) {
	expr := knowledgeFTSExpression

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
	var notes []models.KnowledgeNote
	err := s.db.NewSelect().Model(&notes).
		Where("selectors IS NOT NULL").
		Where("(expires_at IS NULL OR expires_at > ?)", now).
		OrderExpr("updated_at DESC").
		Limit(limit * 5).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query knowledge notes for match: %w", err)
	}

	matched := make([]KnowledgeNote, 0, len(notes))
	for _, n := range notes {
		rec := pgKnowledgeToRecord(&n)
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

func pgKnowledgeToRecord(n *models.KnowledgeNote) *KnowledgeNote {
	var selectors []config.RouteCondition
	if n.Selectors != nil {
		selectors = routeConditionsFromModels(n.Selectors)
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
