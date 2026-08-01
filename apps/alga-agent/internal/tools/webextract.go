package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	gohtml "html"

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
	URLs []string `json:"urls" desc:"One or more URLs to extract content from."`
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
		client:   newSSRFSafeClient(30 * time.Second),
		maxChars: maxChars,
	}
}

// newSSRFSafeClient returns an HTTP client whose dialer validates that every
// connection target resolves only to public addresses. Validation happens at
// dial time (so DNS-rebinding cannot swap in a private address after the initial
// check) and therefore applies to every redirect hop as well.
func newSSRFSafeClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		DialContext:         safeDialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("unsupported redirect scheme %q", req.URL.Scheme)
			}
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

// safeDialContext resolves the target host and refuses to connect unless every
// resolved address is an allowed public address. It then dials the first allowed
// address directly, so the validated IP is exactly the one connected to.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("host %q did not resolve", host)
	}
	for _, ipAddr := range ips {
		if !isAllowedIP(ipAddr.IP) {
			return nil, fmt.Errorf("host %q resolves to a disallowed address", host)
		}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

var disallowedRanges = func() []*net.IPNet {
	cidrs := []string{
		"100.64.0.0/10", // RFC 6598 shared address space
		"0.0.0.0/8",     // "this" network
		"192.0.0.0/24",  // IETF protocol assignments
		"198.18.0.0/15", // benchmarking
		"240.0.0.0/4",   // reserved
		"64:ff9b::/96",  // NAT64
		"2002::/16",     // 6to4
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, _ := net.ParseCIDR(c)
		nets = append(nets, n)
	}
	return nets
}()

// isAllowedIP reports whether ip is a routable public address. Loopback,
// link-local (incl. the 169.254.169.254 cloud-metadata endpoint), private
// (RFC 1918/4193), unspecified, multicast, shared (RFC 6598), benchmarking,
// reserved, NAT64, and 6to4 addresses are all rejected.
func isAllowedIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	if ip.IsPrivate() {
		return false
	}
	if ip.Equal(net.IPv4(169, 254, 169, 254)) {
		return false
	}
	check := ip
	if v4 := ip.To4(); v4 != nil {
		check = v4
	}
	for _, n := range disallowedRanges {
		if n.Contains(check) {
			return false
		}
	}
	return true
}

func (w *WebExtractTool) extract(ctx context.Context, in webExtractInput) Result[webExtractOutput] {
	if len(in.URLs) == 0 {
		return ErrMsg[webExtractOutput]("at least one URL is required")
	}
	if len(in.URLs) > 5 {
		return ErrMsg[webExtractOutput]("maximum 5 URLs per call")
	}

	// Bound the whole batch to roughly one client timeout so a slow URL cannot
	// stall the others indefinitely; each goroutine writes only its own index.
	batchTimeout := w.client.Timeout
	if batchTimeout <= 0 {
		batchTimeout = 30 * time.Second
	}
	batchCtx, cancel := context.WithTimeout(ctx, batchTimeout)
	defer cancel()

	results := make([]webExtractResult, len(in.URLs))
	var wg sync.WaitGroup
	for i, rawURL := range in.URLs {
		wg.Add(1)
		go func(i int, rawURL string) {
			defer wg.Done()
			res := webExtractResult{URL: rawURL}
			content, title, err := w.fetchOne(batchCtx, rawURL)
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Content = content
				res.Title = title
				res.Chars = len(content)
			}
			results[i] = res
		}(i, rawURL)
	}
	wg.Wait()
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
		"Fetch and extract readable content from one or more URLs. Returns page plain text. Use after web_search to read full articles.",
		t.extract, WithCategory[webExtractInput, webExtractOutput]("System")))
}

var (
	titleRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	scriptRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	noscriptRe = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	commentRe  = regexp.MustCompile(`(?s)<!--.*?-->`)
	tagRe      = regexp.MustCompile(`<[^>]*>`)
	wsRe       = regexp.MustCompile(`[ \t]+`)
	multiNlRe  = regexp.MustCompile(`\n{3,}`)
)

func extractTitle(html string) string {
	m := titleRe.FindStringSubmatch(html)
	if len(m) > 1 {
		return strings.TrimSpace(gohtml.UnescapeString(m[1]))
	}
	return ""
}

func htmlToText(html string) string {
	text := scriptRe.ReplaceAllString(html, "")
	text = styleRe.ReplaceAllString(text, "")
	text = noscriptRe.ReplaceAllString(text, "")
	// Strip HTML comments before tag stripping so a ">" inside a comment does
	// not terminate the comment early and leak into the extracted text.
	text = commentRe.ReplaceAllString(text, "")
	text = tagRe.ReplaceAllString(text, "\n")
	text = gohtml.UnescapeString(text)
	text = wsRe.ReplaceAllString(text, " ")
	text = multiNlRe.ReplaceAllString(text, "\n\n")
	return text
}
