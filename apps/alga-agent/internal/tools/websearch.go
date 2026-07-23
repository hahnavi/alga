package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"alga-agent/internal/config"
)

// SearchResult is one web search hit.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchProvider is the interface for web search backends.
type SearchProvider interface {
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
	Name() string
}

// WebSearchTool wraps a SearchProvider behind the Tool interface.
type WebSearchTool struct {
	provider   SearchProvider
	maxResults int
}

// NewWebSearchTool constructs a WebSearchTool from config. Returns nil if
// disabled. The provider is selected by config.Provider.
func NewWebSearchTool(cfg config.WebSearchConfig) *WebSearchTool {
	if !cfg.Enabled {
		return nil
	}
	max := cfg.MaxResults
	if max <= 0 {
		max = 5
	}
	var provider SearchProvider
	switch cfg.Provider {
	case "brave":
		provider = &braveProvider{apiKey: cfg.APIKey, client: &http.Client{Timeout: 15 * time.Second}}
	case "tavily":
		provider = &tavilyProvider{apiKey: cfg.APIKey, client: &http.Client{Timeout: 15 * time.Second}}
	default:
		provider = &duckDuckGoProvider{client: &http.Client{Timeout: 15 * time.Second}}
	}
	return &WebSearchTool{provider: provider, maxResults: max}
}

func (w *WebSearchTool) Name() string { return "web_search" }
func (w *WebSearchTool) Description() string {
	return "Search the web for information. Returns the top results with title, URL, and snippet."
}
func (w *WebSearchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string", "description": "The search query."},
			"max_results": map[string]any{"type": "integer", "description": "Maximum results (default " + fmt.Sprintf("%d", w.maxResults) + ")."},
		},
		"required": []string{"query"},
	}
}

func (w *WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := DecodeArgs(args, &in); err != nil {
		return "", err
	}
	if in.Query == "" {
		return "", errors.New("query is required")
	}
	max := in.MaxResults
	if max <= 0 || max > w.maxResults {
		max = w.maxResults
	}
	results, err := w.provider.Search(ctx, in.Query, max)
	if err != nil {
		return "", fmt.Errorf("web_search (%s): %w", w.provider.Name(), err)
	}
	return JSONString(map[string]any{
		"provider": w.provider.Name(),
		"query":    in.Query,
		"results":  results,
		"count":    len(results),
	}), nil
}

// RegisterWebSearchTool registers the web search tool if non-nil.
func RegisterWebSearchTool(reg *Registry, t *WebSearchTool) {
	if t == nil {
		return
	}
	reg.Register(t)
}

// --- DuckDuckGo (HTML scraping) ---

type duckDuckGoProvider struct {
	client *http.Client
}

func (d *duckDuckGoProvider) Name() string { return "duckduckgo" }

func (d *duckDuckGoProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AlgaAgent/1.0 (+https://github.com/alga)")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("duckduckgo returned %d", resp.StatusCode)
	}
	return parseDuckDuckGoHTML(string(body), maxResults), nil
}

var (
	ddgResultLink = regexp.MustCompile(`<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	ddgSnippet    = regexp.MustCompile(`<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)
	ddgTagStrip   = regexp.MustCompile(`<[^>]*>`)
)

func parseDuckDuckGoHTML(html string, maxResults int) []SearchResult {
	linkMatches := ddgResultLink.FindAllStringSubmatch(html, -1)
	snippetMatches := ddgSnippet.FindAllStringSubmatch(html, -1)
	results := make([]SearchResult, 0, maxResults)
	n := len(linkMatches)
	if len(snippetMatches) < n {
		n = len(snippetMatches)
	}
	if n > maxResults {
		n = maxResults
	}
	for i := 0; i < n; i++ {
		rawURL := linkMatches[i][1]
		// DuckDuckGo wraps links in a redirect like //duckduckgo.com/l/?uddg=<encoded>
		if decoded := extractDDGRedirect(rawURL); decoded != "" {
			rawURL = decoded
		}
		results = append(results, SearchResult{
			Title:   cleanHTML(linkMatches[i][2]),
			URL:     rawURL,
			Snippet: cleanHTML(snippetMatches[i][1]),
		})
	}
	return results
}

func extractDDGRedirect(raw string) string {
	if strings.Contains(raw, "uddg=") {
		u, err := url.Parse(raw)
		if err == nil {
			if v := u.Query().Get("uddg"); v != "" {
				return v
			}
		}
	}
	return ""
}

func cleanHTML(s string) string {
	// Unescape entities, strip tags, collapse whitespace.
	s = ddgTagStrip.ReplaceAllString(s, "")
	s = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(s, " "))
}

// --- Brave Search API ---

type braveProvider struct {
	apiKey string
	client *http.Client
}

func (b *braveProvider) Name() string { return "brave" }

func (b *braveProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u := "https://api.search.brave.com/res/v1/web/search?q=" + url.QueryEscape(query) + fmt.Sprintf("&count=%d", maxResults)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Subscription-Token", b.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("brave returned %d: %s", resp.StatusCode, string(body))
	}
	var doc struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode brave response: %w", err)
	}
	results := make([]SearchResult, 0, len(doc.Web.Results))
	for _, r := range doc.Web.Results {
		results = append(results, SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return results, nil
}

// --- Tavily Search API ---

type tavilyProvider struct {
	apiKey string
	client *http.Client
}

func (t *tavilyProvider) Name() string { return "tavily" }

func (t *tavilyProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	payload := map[string]any{
		"api_key":     t.apiKey,
		"query":       query,
		"max_results": maxResults,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tavily returned %d: %s", resp.StatusCode, string(respBody))
	}
	var doc struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &doc); err != nil {
		return nil, fmt.Errorf("decode tavily response: %w", err)
	}
	results := make([]SearchResult, 0, len(doc.Results))
	for _, r := range doc.Results {
		snippet := r.Content
		if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}
		results = append(results, SearchResult{Title: r.Title, URL: r.URL, Snippet: snippet})
	}
	return results, nil
}
