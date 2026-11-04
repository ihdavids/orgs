package rss

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

const rss2 = `<?xml version="1.0"?><rss version="2.0"><channel><title>Blog</title>
<item><title>First &amp; best</title><link>https://b.example/1</link><guid>id-1</guid>
<description>&lt;p&gt;Hello &lt;b&gt;world&lt;/b&gt;&lt;/p&gt;</description><pubDate>Mon, 02 Nov 2026 10:00:00 +0000</pubDate></item>
<item><title>Second</title><link>https://b.example/2</link><guid>id-2</guid></item>
</channel></rss>`

const atom = `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>A</title>
<entry><title>Atom one</title><link rel="alternate" href="https://a.example/1"/><id>urn:a:1</id><summary>Short</summary><updated>2026-11-01T09:00:00Z</updated></entry>
</feed>`

func TestParseFeeds(t *testing.T) {
	items, err := Parse([]byte(rss2))
	if err != nil || len(items) != 2 || items[0].Id != "id-1" || items[0].Title != "First & best" || items[0].Link != "https://b.example/1" || items[0].Date.IsZero() {
		t.Fatalf("rss: %v %+v", err, items)
	}
	items, err = Parse([]byte(atom))
	if err != nil || len(items) != 1 || items[0].Id != "urn:a:1" || items[0].Link != "https://a.example/1" || items[0].Summary != "Short" {
		t.Fatalf("atom: %v %+v", err, items)
	}
	if _, err := Parse([]byte("not xml <")); err == nil {
		t.Errorf("rubbish parsed")
	}
}

func TestItemsAreFiledOnce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(rss2)) }))
	defer srv.Close()
	dir := t.TempDir()
	file := filepath.Join(dir, "feeds.org")
	os.WriteFile(file, []byte("#+TITLE: Feeds\n* Other\nkeep\n"), 0o644)
	r := &Rss{Feeds: []Feed{{Url: srv.URL, File: file, Heading: "Blog", Tags: []string{"feed"}}},
		manager: &common.PluginManager{HomeDir: dir}, now: func() time.Time { return time.Date(2026, 11, 3, 14, 5, 0, 0, time.Local) }}
	r.Update(nil)
	r.Update(nil)
	b, _ := os.ReadFile(file)
	got := string(b)
	if strings.Count(got, ":RSS_ID: id-1") != 1 || strings.Count(got, ":RSS_ID: id-2") != 1 {
		t.Fatalf("filed twice or not at all:\n%s", got)
	}
	for _, want := range []string{"* Other\nkeep\n* Blog\n** TODO First & best  :feed:\n:PROPERTIES:\n:RSS_ID: id-1\n",
		"[2026-11-03 Tue 14:05]\nHello world\n[[https://b.example/1]]"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	// Deleted by hand, it does not come back.
	os.WriteFile(file, []byte("* Blog\n"), 0o644)
	r.Update(nil)
	if b, _ := os.ReadFile(file); strings.Contains(string(b), "RSS_ID") {
		t.Errorf("a deleted item came back:\n%s", b)
	}
}
