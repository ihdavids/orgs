package orgs

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// linkTitleTest is a site on 127.0.0.1 (so private addresses are allowed for
// the test) and an empty cache that never touches the user's cache file.
func linkTitleTest(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	withFeatures(t, func(f *common.FeatureSettings) { f.AutoLinkTitle = true })
	oldAllow, oldFile := linkTitleAllowPrivate, linkTitleCacheFile
	linkTitleAllowPrivate = func() bool { return true }
	linkTitleCacheFile = func() string { return "" }
	linkTitleCache.Lock()
	linkTitleCache.entries, linkTitleCache.fails, linkTitleCache.loaded = map[string]linkTitleEntry{}, map[string]linkTitleEntry{}, true
	linkTitleCache.Unlock()
	t.Cleanup(func() { linkTitleAllowPrivate, linkTitleCacheFile = oldAllow, oldFile })
	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
		w.Write(append([]byte("<html><head><title>Caf"), append([]byte{0xe9}, []byte(" [menu] | Bistro</title><meta property=\"og:site_name\" content=\"Bistro\"></head><body>x</body></html>")...)...))
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/page", http.StatusFound) })
	mux.HandleFunc("/pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.4"))
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("/video", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<title>page title - Tube</title>")) })
	mux.HandleFunc("/oembed", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"title":"The video (for %s)","author_name":"Someone","provider_name":"Tube"}`, r.URL.Query().Get("url")[len("http://"):][:9])
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestLinkTitleFetch(t *testing.T) {
	srv, hits := linkTitleTest(t)
	got := LinkTitleOf(srv.URL+"/moved", false)
	if !got.Ok || got.Title != "Café [menu]" || got.Site != "Bistro" || got.Final != srv.URL+"/page" {
		t.Fatalf("%+v", got)
	}
	if got.Link != "[["+srv.URL+"/moved][Café {menu}]]" {
		t.Fatalf("link %q", got.Link)
	}
	if again := LinkTitleOf(srv.URL+"/moved", false); !again.Cached || atomic.LoadInt32(hits) != 1 {
		t.Fatalf("not cached: %+v hits %d", again, *hits)
	}
	LinkTitleOf(srv.URL+"/moved", true)
	if atomic.LoadInt32(hits) != 2 {
		t.Fatal("refresh did not fetch")
	}
	for path, want := range map[string]string{"/pdf": "not a web page (application/pdf)", "/gone": "answered 404"} {
		r := LinkTitleOf(srv.URL+path, false)
		if r.Ok || !strings.Contains(r.Msg, want) || r.Link != "[["+srv.URL+path+"]]" {
			t.Fatalf("%s: %+v", path, r)
		}
	}
	if r := LinkTitleOf("ftp://x.example/", false); r.Ok || !strings.Contains(r.Msg, "only http") {
		t.Fatalf("%+v", r)
	}
	// Batches keep their order and ask once per url.
	all := LinkTitlesOf([]string{srv.URL + "/gone", srv.URL + "/page", srv.URL + "/gone"}, false)
	if len(all) != 3 || all[0].Ok || !all[1].Ok || all[2].Url != all[0].Url {
		t.Fatalf("%+v", all)
	}
}

func TestLinkTitleOembedAndIgnore(t *testing.T) {
	srv, _ := linkTitleTest(t)
	Conf().Server.AutoLinkTitle.Oembed = []common.LinkTitleOembed{{Host: "127.0.0.1", Url: srv.URL + "/oembed?url="}}
	Conf().Server.AutoLinkTitle.Rules = []common.LinkTitleRule{{Host: "127.0.0.1", Strip: []string{` \(for [^)]*\)$`}}}
	r := LinkTitleOf(srv.URL+"/video", false)
	if !r.Ok || r.From != "oembed" || r.Title != "The video" || r.Description != "by Someone" || r.Site != "Tube" {
		t.Fatalf("%+v", r)
	}
	Conf().Server.AutoLinkTitle.Ignore = []string{"127.0.0.1"}
	if r := LinkTitleOf(srv.URL+"/other", false); !r.Ignored || r.Ok {
		t.Fatalf("%+v", r)
	}
}

func TestLinkTitlePrivateRefused(t *testing.T) {
	srv, hits := linkTitleTest(t)
	linkTitleAllowPrivate = func() bool { return false }
	for _, u := range []string{srv.URL + "/page", "http://localhost:1/", "http://10.1.2.3/", "http://[::1]/", "http://169.254.169.254/latest"} {
		if r := LinkTitleOf(u, false); r.Ok || !strings.Contains(r.Msg, "private address") {
			t.Fatalf("%s: %+v", u, r)
		}
	}
	if *hits != 0 {
		t.Fatal("a private site was fetched")
	}
	if !linkTitlePrivateIP(nil) || linkTitlePrivateIP([]byte{93, 184, 216, 34}) || !linkTitlePrivateIP([]byte{172, 20, 0, 1}) {
		t.Fatal("ranges")
	}
}

func TestLinkTitleRetitle(t *testing.T) {
	srv, _ := linkTitleTest(t)
	paths := loadOrg(t, map[string]string{"links.org": "* Reading\n:PROPERTIES:\n:URL: " + srv.URL + "/page\n:END:\nSee " + srv.URL + "/page and [[" + srv.URL + "/gone]].\n| " + srv.URL + "/page |\n"})
	p := paths["links.org"]
	raw, _ := os.ReadFile(p)
	before := string(raw)
	pre := linkTitleRetitle(common.RetitleRequest{File: p})
	if !pre.Ok || pre.Changed != 1 || len(pre.Links) != 2 || pre.Links[1].Ok || pre.Hash == "" || !strings.Contains(pre.Patch, "+See [["+srv.URL+"/page][Café {menu}]] and") {
		t.Fatalf("%+v", pre)
	}
	if now, _ := os.ReadFile(p); string(now) != before {
		t.Fatal("a preview wrote")
	}
	// An apply with the titles the client chose, refused once the file moved.
	titles := map[string]string{srv.URL + "/page": "Lunch", srv.URL + "/gone": "Gone"}
	os.WriteFile(p, []byte(before+"more\n"), 0644)
	if st := linkTitleRetitle(common.RetitleRequest{File: p, Apply: true, Hash: pre.Hash, Titles: titles}); st.Ok || !st.Stale {
		t.Fatalf("%+v", st)
	}
	os.WriteFile(p, []byte(before), 0644)
	done := linkTitleRetitle(common.RetitleRequest{File: p, Apply: true, Hash: pre.Hash, Titles: titles})
	if !done.Applied || done.Changed != 2 {
		t.Fatalf("%+v", done)
	}
	want := strings.Replace(strings.Replace(before, "See "+srv.URL+"/page", "See [["+srv.URL+"/page][Lunch]]", 1), "[["+srv.URL+"/gone]]", "[["+srv.URL+"/gone][Gone]]", 1)
	if got, _ := os.ReadFile(p); string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	// Text in, text out, nothing written.
	tx := linkTitleRetitle(common.RetitleRequest{Text: "a " + srv.URL + "/page"})
	if !tx.Ok || tx.Text != "a [["+srv.URL+"/page][Café {menu}]]" {
		t.Fatalf("%+v", tx)
	}
}

func TestLinkTitleOff(t *testing.T) {
	withFeatures(t, func(f *common.FeatureSettings) { f.AutoLinkTitle = false })
	w := httptest.NewRecorder()
	RequestLinkTitle(w, httptest.NewRequest("GET", "/link/title?url=https://example.com", nil))
	if !strings.Contains(w.Body.String(), `"Off":true`) {
		t.Fatal(w.Body.String())
	}
}
