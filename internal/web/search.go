// Package web mirrors construct-app/brain/tool/web_search.go +
// web_fetch.go so callers that don't run inside the desktop runtime
// still get the same network tool surface. Kept deliberately close to
// the brain copies so behaviour stays in sync.
package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

var searchClient = &http.Client{Timeout: 20 * time.Second}

// Search hits DuckDuckGo's HTML-lite endpoint by default (no API key
// needed). Override with WEB_SEARCH_URL to point at any DDG-compatible
// HTML responder.
func Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	endpoint := getenv("WEB_SEARCH_URL", "https://html.duckduckgo.com/html/")
	form := url.Values{"q": {query}}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")

	resp, err := searchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 500<<10))
	if err != nil {
		return nil, err
	}
	return parseDDG(string(body), limit), nil
}

var (
	reDDGTitle   = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*result__a[^"]*"[^>]+href="([^"]+)"[^>]*>(.+?)</a>`)
	reDDGSnippet = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*result__snippet[^"]*"[^>]*>(.+?)</a>`)
)

func parseDDG(body string, limit int) []SearchResult {
	titles := reDDGTitle.FindAllStringSubmatch(body, -1)
	snippets := reDDGSnippet.FindAllStringSubmatch(body, -1)
	out := make([]SearchResult, 0, len(titles))
	for i, m := range titles {
		if i >= limit {
			break
		}
		r := SearchResult{
			URL:   decodeDDGRedirect(m[1]),
			Title: strings.TrimSpace(stripTags(m[2])),
		}
		if i < len(snippets) {
			r.Snippet = strings.TrimSpace(stripTags(snippets[i][1]))
		}
		out = append(out, r)
	}
	return out
}

func decodeDDGRedirect(u string) string {
	if !strings.Contains(u, "uddg=") {
		if strings.HasPrefix(u, "//") {
			return "https:" + u
		}
		return u
	}
	if idx := strings.Index(u, "uddg="); idx >= 0 {
		tail := u[idx+len("uddg="):]
		if amp := strings.Index(tail, "&"); amp >= 0 {
			tail = tail[:amp]
		}
		if dec, err := url.QueryUnescape(tail); err == nil {
			return dec
		}
	}
	return u
}

var reAnyTag = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string {
	s = reAnyTag.ReplaceAllString(s, "")
	repl := strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'")
	return repl.Replace(s)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
