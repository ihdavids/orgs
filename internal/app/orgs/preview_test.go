package orgs

import (
	htmlexp "github.com/ihdavids/orgs/internal/app/orgs/plugs/html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func previewOnFor(t *testing.T, dir string) {
	t.Helper()
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	old, oldDirs, oldSets := Conf().Server.Features.HoverEditor, Conf().Server.OrgDirs, Conf().Server.HoverEditor
	Conf().Server.Features.HoverEditor = true
	if dir != "" {
		Conf().Server.OrgDirs = []string{dir}
	}
	t.Cleanup(func() {
		Conf().Server.Features.HoverEditor, Conf().Server.OrgDirs, Conf().Server.HoverEditor = old, oldDirs, oldSets
		previewClearWeb()
	})
}

const previewSource = `#+TITLE: Source notes
* Projects
** TODO Budget :money:
:PROPERTIES:
:ID: budget-1
:CUSTOM_ID: budget
:END:
Spend [[*Other][less]].
*** Rent
Too much.

** Other
Not this.
`

func TestPreviewResolvesEveryHeadingLink(t *testing.T) {
	paths := loadOrg(t, map[string]string{"src.org": previewSource, "main.org": "* Plan\nsee [[id:budget-1]]\n"})
	previewOnFor(t, filepath.Dir(paths["src.org"]))
	src, main := paths["src.org"], paths["main.org"]
	for _, c := range []struct{ link, from string }{
		{"[[id:budget-1][the budget]]", main},
		{"id:budget-1", ""},
		{"[[*Budget]]", src},
		{"Budget", src},
		{"file:src.org::*Budget", main},
		{"[[file:src.org::#budget]]", main},
		{"./src.org::*Budget", main},
	} {
		p := LinkPreview(c.link, c.from)
		if !p.Ok || p.Kind != "heading" {
			t.Fatalf("%s: %+v", c.link, p)
		}
		if p.Title != "Budget" || p.Todo != "TODO" || p.File != src || p.From != 2 || p.To != 9 || p.Line != 3 {
			t.Fatalf("%s: title %q todo %q file %s rows %d-%d", c.link, p.Title, p.Todo, p.File, p.From, p.To)
		}
		if strings.Join(p.Path, "/") != "Projects" || len(p.Tags) != 1 || p.Tags[0] != "money" {
			t.Fatalf("%s: path %v tags %v", c.link, p.Path, p.Tags)
		}
		if !strings.HasPrefix(p.Text, "** TODO Budget") || !strings.HasSuffix(p.Text, "Too much.") {
			t.Fatalf("%s: text %q", c.link, p.Text)
		}
		if strings.Contains(p.Html, "PROPERTIES") || strings.Contains(p.Html, "budget-1") || !strings.Contains(p.Html, "Rent") {
			t.Fatalf("%s: html %s", c.link, p.Html)
		}
		if !p.Editable || p.Base == "" || p.Hash == "" {
			t.Fatalf("%s: editable %v base %s hash %q", c.link, p.Editable, p.Base, p.Hash)
		}
	}
	if p := LinkPreview("[[*Nowhere]]", src); p.Ok {
		t.Fatalf("a missing heading previewed: %+v", p)
	}
}

func TestPreviewFilesLinesPicturesAndOff(t *testing.T) {
	paths := loadOrg(t, map[string]string{"src.org": previewSource, "main.org": "* Plan\n"})
	dir := filepath.Dir(paths["src.org"])
	previewOnFor(t, dir)
	os.WriteFile(filepath.Join(dir, "cat.png"), []byte("\x89PNG"), 0644)
	os.WriteFile(filepath.Join(dir, "run.py"), []byte("print(1)\n"), 0644)
	main := paths["main.org"]

	p := LinkPreview("[[file:src.org]]", main)
	if !p.Ok || p.Kind != "file" || p.Title != "Source notes" || p.From != 1 || !p.Editable {
		t.Fatalf("file: %+v", p)
	}
	p = LinkPreview("file:src.org::12", main)
	if !p.Ok || p.Kind != "text" || p.From != 11 || !strings.HasPrefix(p.Text, "** Other") {
		t.Fatalf("line: %+v", p)
	}
	p = LinkPreview("[[file:cat.png]]", main)
	if !p.Ok || p.Kind != "image" || p.Url != "/images/cat.png" || p.Text != "" {
		t.Fatalf("picture: %+v", p)
	}
	p = LinkPreview("file:run.py", main)
	if !p.Ok || p.Kind != "source" || p.Lang != "py" || p.Editable || p.Text != "print(1)" {
		t.Fatalf("source: %+v", p)
	}
	if p = LinkPreview("file:/etc/hosts", main); p.Ok {
		t.Fatalf("read outside the org directories: %+v", p)
	}

	// Off: the endpoint refuses and the exporter's spans are gone.
	Conf().Server.Features.HoverEditor = false
	rec := httptest.NewRecorder()
	RequestPreview(rec, httptest.NewRequest("GET", "/preview?link=id:budget-1", nil))
	if !strings.Contains(rec.Body.String(), `"Off":true`) {
		t.Fatalf("off answered %s", rec.Body.String())
	}
	Conf().Server.Features.HoverEditor = true
	if !htmlexp.HoverLinksOn() {
		t.Fatal("the exporter does not see the feature")
	}
}

func TestPreviewSaveIsARowCheckedSplice(t *testing.T) {
	paths := loadOrg(t, map[string]string{"src.org": previewSource})
	previewOnFor(t, filepath.Dir(paths["src.org"]))
	src := paths["src.org"]
	p := LinkPreview("id:budget-1", src)
	edited := strings.Replace(p.Text, "Too much.", "Too much.\nFar too much.", 1)
	res := SavePreview(&PreviewSave{File: p.File, From: p.From, To: p.To, Base: p.Base, Text: edited + "\n"})
	if !res.Ok || res.To != p.To+1 || res.Base == p.Base || res.Text != edited {
		t.Fatalf("save: %+v", res)
	}
	want := strings.TrimRight(strings.Replace(previewSource, "Too much.\n", "Too much.\nFar too much.\n", 1), "\n")
	if got := readOrg(t, src); got != want {
		t.Fatalf("file:\n%q\nwant:\n%q", got, want)
	}
	// The old base no longer matches: refused, nothing written.
	if res2 := SavePreview(&PreviewSave{File: p.File, From: p.From, To: p.To, Base: p.Base, Text: "* gone"}); res2.Ok || !strings.Contains(res2.Msg, "changed") {
		t.Fatalf("stale save: %+v", res2)
	}
	if readOrg(t, src) != want {
		t.Fatal("a stale save wrote")
	}
	// The answer's rows and base save again.
	again := strings.Replace(edited, "Far", "Much", 1)
	if res3 := SavePreview(&PreviewSave{File: res.File, From: res.From, To: res.To, Base: res.Base, Text: again}); !res3.Ok {
		t.Fatalf("second save: %+v", res3)
	}
	if !strings.Contains(readOrg(t, src), "Much too much.") {
		t.Fatal("second save not written")
	}
	// The last heading of a file, by id: the end of the file is its neighbour.
	last := loadOrg(t, map[string]string{"end.org": "* A\n* B\n:PROPERTIES:\n:ID: last-1\n:END:\ntext\n"})["end.org"]
	q := LinkPreview("id:last-1", "")
	if r := SavePreview(&PreviewSave{File: q.File, From: q.From, To: q.To, Base: q.Base, Text: q.Text + "\nmore"}); !r.Ok {
		t.Fatalf("last heading: %+v", r)
	}
	if readOrg(t, last) != "* A\n* B\n:PROPERTIES:\n:ID: last-1\n:END:\ntext\nmore" {
		t.Fatalf("last heading wrote %q", readOrg(t, last))
	}
}

func TestPreviewWebPages(t *testing.T) {
	previewOnFor(t, "")
	page := `<html><head><title>Fallback &amp; title - Site</title>
<meta property="og:title" content="Owls &amp; owlbears">
<meta name="description" content="Plain description">
<meta property="og:description" content="All about owls.">
<meta property="og:image" content="/pics/owl.jpg">
<meta property="og:site_name" content="Birdy">
</head><body><img src="x.png"></body></html>`
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if !strings.HasPrefix(r.UserAgent(), "orgs/") {
			t.Errorf("user agent %q", r.UserAgent())
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(page))
	}))
	defer srv.Close()

	// A test server is on this machine: refused unless privateHosts.
	p := previewWeb(srv.URL + "/a")
	if !p.Ok || p.Description != "" || !strings.Contains(p.Msg, "could not reach") || hits != 0 {
		t.Fatalf("private host fetched: %+v (hits %d)", p, hits)
	}
	Conf().Server.HoverEditor.PrivateHosts = true
	p = previewWeb(srv.URL + "/b")
	if p.Title != "Owls & owlbears" || p.Description != "All about owls." || p.Image != srv.URL+"/pics/owl.jpg" || p.Site != "Birdy" {
		t.Fatalf("meta: %+v", p)
	}
	previewWeb(srv.URL + "/b")
	if hits != 1 {
		t.Fatalf("not cached: %d fetches", hits)
	}
	Conf().Server.HoverEditor.NoWeb = true
	if p = previewWeb(srv.URL + "/c"); hits != 1 || p.Title != srv.URL+"/c" {
		t.Fatalf("noWeb fetched: %+v", p)
	}
	u, _ := url.Parse("https://x.example/dir/")
	if m := previewPageMeta(`<title> Just   a title </title>`, u); m.title != "Just a title" || m.image != "" {
		t.Fatalf("plain title: %+v", m)
	}
}
