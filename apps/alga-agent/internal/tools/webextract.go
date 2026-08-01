package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"alga-agent/internal/config"
)

// WebExtractTool fetches and extracts readable content from URLs. It complements
// web_search by allowing the agent to read full page content from URLs found
// during search. Ported from hermes-agent's web_extract_tool.
type WebExtractTool struct {
	client   *http.Client
	maxChars int
}

type webExtractInput struct {
	URLs   []string `json:"urls" desc:"One or more URLs to extract content from."`
	Format string   `json:"format,omitempty" desc:"Output format: \"markdown\" (default) or \"text\"."`
}

type webExtractResult struct {
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
	Chars   int    `json:"chars"`
	Error   string `json:"error,omitempty"`
}

type webExtractOutput struct {
	Results []webExtractResult `json:"results"`
}

// NewWebExtractTool constructs a WebExtractTool from config. Returns nil if disabled.
func NewWebExtractTool(cfg config.WebExtractConfig) *WebExtractTool {
	if !cfg.Enabled {
		return nil
	}
	maxChars := cfg.MaxChars
	if maxChars <= 0 {
		maxChars = 50000
	}
	return &WebExtractTool{
		client:   &http.Client{Timeout: 30 * time.Second},
		maxChars: maxChars,
	}
}

func (w *WebExtractTool) extract(ctx context.Context, in webExtractInput) Result[webExtractOutput] {
	if len(in.URLs) == 0 {
		return ErrMsg[webExtractOutput]("at least one URL is required")
	}
	if len(in.URLs) > 5 {
		return ErrMsg[webExtractOutput]("maximum 5 URLs per call")
	}

	results := make([]webExtractResult, 0, len(in.URLs))
	for _, rawURL := range in.URLs {
		res := webExtractResult{URL: rawURL}
		content, title, err := w.fetchOne(ctx, rawURL)
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Content = content
			res.Title = title
			res.Chars = len(content)
		}
		results = append(results, res)
	}
	return OK(webExtractOutput{Results: results})
}

func (w *WebExtractTool) fetchOne(ctx context.Context, rawURL string) (content, title string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "AlgaAgent/1.0 (+https://github.com/alga)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,*/*")

	resp, err := w.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", "", err
	}

	ct := resp.Header.Get("Content-Type")
	text := string(body)

	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		title = extractTitle(text)
		text = htmlToText(text)
	}

	if len(text) > w.maxChars {
		text = text[:w.maxChars] + "\n...[truncated]..."
	}
	return strings.TrimSpace(text), title, nil
}

// RegisterWebExtractTool registers the web extract tool if non-nil.
func RegisterWebExtractTool(reg *Registry, t *WebExtractTool) {
	if t == nil {
		return
	}
	reg.Register(NewTypedTool("web_extract",
		"Fetch and extract readable content from one or more URLs. Returns page text/markdown. Use after web_search to read full articles.",
		t.extract, WithCategory[webExtractInput, webExtractOutput]("System")))
}

var (
	titleRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	scriptRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	noscriptRe = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	tagRe      = regexp.MustCompile(`<[^>]*>`)
	wsRe       = regexp.MustCompile(`[ \t]+`)
	multiNlRe  = regexp.MustCompile(`\n{3,}`)
	entityRepl = strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`,
		"&#39;", "'", "&nbsp;", " ", "&#x27;", "'", "&#x2F;", "/",
	)
)

func extractTitle(html string) string {
	m := titleRe.FindStringSubmatch(html)
	if len(m) > 1 {
		return strings.TrimSpace(entityRepl.Replace(m[1]))
	}
	return ""
}

func htmlToText(html string) string {
	text := scriptRe.ReplaceAllString(html, "")
	text = styleRe.ReplaceAllString(text, "")
	text = noscriptRe.ReplaceAllString(text, "")
	text = tagRe.ReplaceAllString(text, "\n")
	text = entityRepl.Replace(text)
	text = wsRe.ReplaceAllString(text, " ")
	text = multiNlRe.ReplaceAllString(text, "\n\n")
	return text
}
