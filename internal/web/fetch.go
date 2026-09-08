package web

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type FetchResult struct {
	URL         string `json:"url"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
}

// isBlockedFetchIP reports whether an address is one a server-side web fetch
// must never reach: loopback, the cloud-metadata endpoint, and the rest of the
// internal/private network. Unlike the desktop client there's no legitimate
// reason for inference to fetch loopback/LAN, so all of it is blocked.
func isBlockedFetchIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// 100.64.0.0/10 carrier-grade NAT.
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	return ip.IsLoopback() || ip.IsPrivate() || // 10/8, 172.16/12, 192.168/16, fc00::/7
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || // 169.254/16 (incl. metadata), fe80::/10
		ip.IsUnspecified() || ip.IsMulticast()
}

// fetchClient validates the ACTUAL dialed IP via the dialer Control hook, so a
// hostname that resolves (or DNS-rebinds) to an internal address — and any
// redirect hop — is rejected at connect time, not just by inspecting the URL.
var fetchClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				if isBlockedFetchIP(net.ParseIP(host)) {
					return fmt.Errorf("refusing to connect to internal/non-routable address %s (SSRF guard)", host)
				}
				return nil
			},
		}).DialContext,
	},
}

// Fetch GETs a URL. HTML bodies are stripped to plain text unless raw
// is true. Capped at 200KB so a runaway page can't blow the context.
// Only http/https schemes are accepted.
func Fetch(ctx context.Context, target string, raw bool) (*FetchResult, error) {
	if target == "" {
		return nil, fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return nil, fmt.Errorf("only http(s) URLs allowed")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "construct-inference/0.0.1 (+https://lisaos.dev)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := fetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 200<<10))
	if err != nil {
		return nil, err
	}
	ct := resp.Header.Get("Content-Type")
	text := string(body)
	if !raw && strings.Contains(ct, "html") {
		text = stripHTML(text)
	}
	return &FetchResult{
		URL:         target,
		Status:      resp.StatusCode,
		ContentType: ct,
		Body:        text,
	}, nil
}

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	reTag         = regexp.MustCompile(`(?s)<[^>]+>`)
	reWhitespace  = regexp.MustCompile(`[ \t]+`)
	reBlankLines  = regexp.MustCompile(`\n{3,}`)
)

func stripHTML(s string) string {
	s = reScriptStyle.ReplaceAllString(s, " ")
	s = reTag.ReplaceAllString(s, " ")
	repl := strings.NewReplacer(
		"&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&apos;", "'",
	)
	s = repl.Replace(s)
	s = reWhitespace.ReplaceAllString(s, " ")
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(strings.TrimSpace(line))
		b.WriteByte('\n')
	}
	return reBlankLines.ReplaceAllString(b.String(), "\n\n")
}
