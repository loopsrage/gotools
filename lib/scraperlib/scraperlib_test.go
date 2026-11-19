package scraperlib

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScraperHTTPHTMLAndRSSAndRobots(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /private\nAllow: /private/public\n"))
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>Jobs</title><meta name="description" content="Open roles"></head><body><h1>Careers</h1><a href="/jobs/123">Go Eng</a><a href="https://ext.example/x">Ext</a><script>ignored()</script></body></html>`))
		case "/feed.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(`<rss><channel><item><title>Go Release</title><link>https://example.com/go</link><description>Fast binary</description></item></channel></rss>`))
		}
	}))
	defer srv.Close()

	sc := NewScraper("TestBot/1.0", 5*time.Second)
	if sc.Robots.Allowed(context.Background(), srv.URL+"/private/secret") {
		t.Fatal("expected /private/secret to be disallowed")
	}
	if !sc.Robots.Allowed(context.Background(), srv.URL+"/private/public/ok") {
		t.Fatal("expected /private/public/ok to be allowed")
	}

	page, err := sc.Fetch(context.Background(), srv.URL+"/page", FetchOptions{RespectRobots: true})
	if err != nil {
		t.Fatalf("Fetch page: %v", err)
	}
	if page.Title != "Jobs" || !strings.Contains(page.Text, "Careers") || strings.Contains(page.Text, "ignored") {
		t.Fatalf("unexpected page parse: %+v", page)
	}
	filtered := DiscoverLinks(page.URL, page.Links, []string{`/jobs/`}, nil, true)
	if len(filtered) != 1 || !strings.HasSuffix(filtered[0], "/jobs/123") {
		t.Fatalf("DiscoverLinks: %v", filtered)
	}

	feed, err := sc.Fetch(context.Background(), srv.URL+"/feed.xml", FetchOptions{})
	if err != nil || len(feed.FeedItems) != 1 || feed.FeedItems[0].Title != "Go Release" {
		t.Fatalf("Fetch feed: %+v err=%v", feed, err)
	}
}
