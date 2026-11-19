package scraperlib

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
)

// RobotsChecker fetches, caches, and evaluates RFC 9309 robots.txt Disallow/Allow rules.
type RobotsChecker struct {
	mu        sync.RWMutex
	userAgent string
	client    *http.Client
	rules     map[string]robotsRules
}

type robotsRules struct {
	fetchedAt time.Time
	allow     []string
	disallow  []string
}

// NewRobotsChecker creates a RobotsChecker for userAgent.
func NewRobotsChecker(userAgent string, timeout time.Duration) *RobotsChecker {
	if userAgent == "" {
		userAgent = "AgentBrainBot/1.0"
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &RobotsChecker{
		userAgent: userAgent,
		client:    &http.Client{Timeout: timeout},
		rules:     make(map[string]robotsRules),
	}
}

// Allowed reports whether targetURL is permitted by the origin's robots.txt.
func (rc *RobotsChecker) Allowed(ctx context.Context, targetURL string) bool {
	u, err := url.Parse(targetURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	origin := u.Scheme + "://" + u.Host

	rc.mu.RLock()
	r, ok := rc.rules[origin]
	rc.mu.RUnlock()

	if !ok || time.Since(r.fetchedAt) > 15*time.Minute {
		r = rc.fetchRules(ctx, origin)
		rc.mu.Lock()
		rc.rules[origin] = r
		rc.mu.Unlock()
	}

	reqPath := u.EscapedPath()
	if reqPath == "" {
		reqPath = "/"
	}
	// Longest matching rule wins per RFC 9309
	bestAllow := -1
	for _, a := range r.allow {
		if a != "" && strings.HasPrefix(reqPath, a) && len(a) > bestAllow {
			bestAllow = len(a)
		}
	}
	bestDisallow := -1
	for _, d := range r.disallow {
		if d != "" && strings.HasPrefix(reqPath, d) && len(d) > bestDisallow {
			bestDisallow = len(d)
		}
	}
	if bestDisallow < 0 {
		return true
	}
	return bestAllow >= bestDisallow
}

func (rc *RobotsChecker) fetchRules(ctx context.Context, origin string) robotsRules {
	res := robotsRules{fetchedAt: time.Now()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/robots.txt", nil)
	if err != nil {
		return res
	}
	req.Header.Set("User-Agent", rc.userAgent)
	resp, err := rc.client.Do(req)
	if err != nil {
		return res
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return res
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	uaLower := strings.ToLower(rc.userAgent)
	active := false
	for _, line := range strings.Split(string(raw), "\n") {
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])
		switch key {
		case "user-agent":
			vl := strings.ToLower(val)
			active = vl == "*" || strings.Contains(uaLower, vl)
		case "disallow":
			if active && val != "" {
				res.disallow = append(res.disallow, val)
			}
		case "allow":
			if active && val != "" {
				res.allow = append(res.allow, val)
			}
		}
	}
	return res
}

// FetchOptions controls HTTP and optional headless Chromium scraping.
type FetchOptions struct {
	RenderJS      bool          `json:"render_js"`
	RespectRobots bool          `json:"respect_robots"`
	Timeout       time.Duration `json:"timeout"`
	WaitSelector  string        `json:"wait_selector,omitempty"`
	UserAgent     string        `json:"user_agent,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

// FeedItem represents a single parsed RSS or Atom entry.
type FeedItem struct {
	Title       string `json:"title"`
	Link        string `json:"link"`
	Description string `json:"description"`
	PubDate     string `json:"pub_date,omitempty"`
}

// PageResult holds cleaned content, metadata, links, and feed items extracted from a URL.
type PageResult struct {
	URL         string     `json:"url"`
	StatusCode  int        `json:"status_code"`
	ContentType string     `json:"content_type"`
	Renderer    string     `json:"renderer"` // "http" or "chromedp"
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Text        string     `json:"text"`
	RawHTML     string     `json:"raw_html,omitempty"`
	Links       []string   `json:"links,omitempty"`
	FeedItems   []FeedItem `json:"feed_items,omitempty"`
	FetchedAt   string     `json:"fetched_at"`
}

// Scraper fetches and cleans web pages, JSON endpoints, and RSS/Atom feeds, with optional chromedp rendering.
type Scraper struct {
	UserAgent string
	Client    *http.Client
	Robots    *RobotsChecker
}

// NewScraper constructs a Scraper with default HTTP timeout and robots.txt checker.
func NewScraper(userAgent string, timeout time.Duration) *Scraper {
	if userAgent == "" {
		userAgent = "AgentBrainScraper/1.0"
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Scraper{
		UserAgent: userAgent,
		Client:    &http.Client{Timeout: timeout},
		Robots:    NewRobotsChecker(userAgent, 5*time.Second),
	}
}

// Fetch retrieves targetURL using HTTP or headless Chromium (chromedp when RenderJS=true with HTTP fallback).
func (s *Scraper) Fetch(ctx context.Context, targetURL string, opts FetchOptions) (*PageResult, error) {
	if opts.RespectRobots && !s.Robots.Allowed(ctx, targetURL) {
		return nil, fmt.Errorf("scraperlib: blocked by robots.txt: %s", targetURL)
	}
	if opts.RenderJS {
		if res, err := s.fetchChromedp(ctx, targetURL, opts); err == nil {
			return res, nil
		}
		// Fallback to pure HTTP when Chromium binary is absent (e.g. minimal Cloud Run container).
	}
	return s.fetchHTTP(ctx, targetURL, opts)
}

func (s *Scraper) fetchHTTP(ctx context.Context, targetURL string, opts FetchOptions) (*PageResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = s.UserAgent
	}
	req.Header.Set("User-Agent", ua)
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, err
	}
	ct := resp.Header.Get("Content-Type")
	res := &PageResult{
		URL:         resp.Request.URL.String(),
		StatusCode:  resp.StatusCode,
		ContentType: ct,
		Renderer:    "http",
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	ctLow := strings.ToLower(ct)
	switch {
	case strings.Contains(ctLow, "xml") || strings.Contains(ctLow, "rss") || strings.Contains(ctLow, "atom"):
		items := ParseFeed(body)
		res.FeedItems = items
		var sb strings.Builder
		for _, it := range items {
			sb.WriteString(it.Title + "\n" + it.Description + "\n\n")
			if it.Link != "" {
				res.Links = append(res.Links, it.Link)
			}
		}
		res.Text = strings.TrimSpace(sb.String())
		if res.Text == "" {
			res.Text = string(body)
		}
	case strings.Contains(ctLow, "json") || strings.HasPrefix(ctLow, "text/plain"):
		res.Text = strings.TrimSpace(string(body))
	default:
		res.RawHTML = string(body)
		title, desc, text, links := ExtractHTML(res.URL, body)
		res.Title = title
		res.Description = desc
		res.Text = text
		res.Links = links
	}
	return res, nil
}

func (s *Scraper) fetchChromedp(ctx context.Context, targetURL string, opts FetchOptions) (*PageResult, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(tCtx, allocOpts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	if _, err := chromedp.Run(browserCtx, chromedp.Navigate(targetURL)); err != nil {
		return nil, err
	}
	if opts.WaitSelector != "" {
		if _, err := chromedp.Run(browserCtx, chromedp.WaitVisible(opts.WaitSelector)); err != nil {
			return nil, err
		}
	} else {
		if _, err := chromedp.Run(browserCtx, chromedp.WaitReady("body")); err != nil {
			return nil, err
		}
	}
	htmlStr, err := chromedp.Run(browserCtx, chromedp.OuterHTML("html"))
	if err != nil {
		return nil, err
	}
	title, desc, text, links := ExtractHTML(targetURL, []byte(htmlStr))
	return &PageResult{
		URL:         targetURL,
		StatusCode:  http.StatusOK,
		ContentType: "text/html; charset=utf-8",
		Renderer:    "chromedp",
		Title:       title,
		Description: desc,
		Text:        text,
		RawHTML:     htmlStr,
		Links:       links,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

var reWhitespace = regexp.MustCompile(`\s+`)

// ExtractHTML parses raw HTML and extracts title, meta description, cleaned visible text, and resolved links.
func ExtractHTML(baseURL string, raw []byte) (title, description, text string, links []string) {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return "", "", strings.TrimSpace(string(raw)), nil
	}
	base, _ := url.Parse(baseURL)
	seenLinks := make(map[string]struct{})
	var textBuf strings.Builder

	var walk func(n *html.Node, skip bool)
	walk = func(n *html.Node, skip bool) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "script" || tag == "style" || tag == "noscript" || tag == "svg" {
				skip = true
			}
			if tag == "title" && n.FirstChild != nil && title == "" {
				title = strings.TrimSpace(n.FirstChild.Data)
			}
			if tag == "meta" && description == "" {
				var name, content string
				for _, a := range n.Attr {
					switch strings.ToLower(a.Key) {
					case "name", "property":
						name = strings.ToLower(a.Val)
					case "content":
						content = a.Val
					}
				}
				if name == "description" || name == "og:description" {
					description = strings.TrimSpace(content)
				}
			}
			if tag == "a" {
				for _, a := range n.Attr {
					if strings.ToLower(a.Key) == "href" {
						href := strings.TrimSpace(a.Val)
						if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:") {
							continue
						}
						if u, err := url.Parse(href); err == nil {
							if base != nil {
								u = base.ResolveReference(u)
							}
							resolved := u.String()
							if _, dup := seenLinks[resolved]; !dup {
								seenLinks[resolved] = struct{}{}
								links = append(links, resolved)
							}
						}
					}
				}
			}
		} else if n.Type == html.TextNode && !skip {
			cleaned := strings.TrimSpace(reWhitespace.ReplaceAllString(n.Data, " "))
			if cleaned != "" {
				if textBuf.Len() > 0 {
					textBuf.WriteByte(' ')
				}
				textBuf.WriteString(cleaned)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, skip)
		}
	}
	walk(doc, false)
	return title, description, strings.TrimSpace(textBuf.String()), links
}

// DiscoverLinks filters extracted links by optional regex include/exclude lists and same-host restriction.
func DiscoverLinks(baseURL string, links []string, includePatterns, excludePatterns []string, sameHostOnly bool) []string {
	base, _ := url.Parse(baseURL)
	var incRes, excRes []*regexp.Regexp
	for _, p := range includePatterns {
		if r, err := regexp.Compile(p); err == nil {
			incRes = append(incRes, r)
		}
	}
	for _, p := range excludePatterns {
		if r, err := regexp.Compile(p); err == nil {
			excRes = append(excRes, r)
		}
	}
	var out []string
	for _, l := range links {
		if sameHostOnly && base != nil {
			if u, err := url.Parse(l); err != nil || !strings.EqualFold(u.Host, base.Host) {
				continue
			}
		}
		excluded := false
		for _, er := range excRes {
			if er.MatchString(l) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		if len(incRes) > 0 {
			matched := false
			for _, ir := range incRes {
				if ir.MatchString(l) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

// ParseFeed parses RSS 2.0 or Atom XML documents into FeedItems.
func ParseFeed(raw []byte) []FeedItem {
	var rss struct {
		Channel struct {
			Items []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Description string `xml:"description"`
				PubDate     string `xml:"pubDate"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(raw, &rss); err == nil && len(rss.Channel.Items) > 0 {
		out := make([]FeedItem, 0, len(rss.Channel.Items))
		for _, it := range rss.Channel.Items {
			out = append(out, FeedItem{
				Title:       strings.TrimSpace(it.Title),
				Link:        strings.TrimSpace(it.Link),
				Description: strings.TrimSpace(it.Description),
				PubDate:     strings.TrimSpace(it.PubDate),
			})
		}
		return out
	}
	var atom struct {
		Entries []struct {
			Title   string `xml:"title"`
			Summary string `xml:"summary"`
			Content string `xml:"content"`
			Updated string `xml:"updated"`
			Link    struct {
				Href string `xml:"href,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(raw, &atom); err == nil && len(atom.Entries) > 0 {
		out := make([]FeedItem, 0, len(atom.Entries))
		for _, e := range atom.Entries {
			desc := strings.TrimSpace(e.Summary)
			if desc == "" {
				desc = strings.TrimSpace(e.Content)
			}
			out = append(out, FeedItem{
				Title:       strings.TrimSpace(e.Title),
				Link:        strings.TrimSpace(e.Link.Href),
				Description: desc,
				PubDate:     strings.TrimSpace(e.Updated),
			})
		}
		return out
	}
	return nil
}
