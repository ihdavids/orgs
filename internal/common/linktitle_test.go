package common

import (
	"net/url"
	"strings"
	"testing"
)

func TestParsePageMeta(t *testing.T) {
	base, _ := url.Parse("https://example.com/a/b")
	page := `<!doctype html><html><head>
<meta charset="utf-8">
<title>  Fish &amp; Chips
  - The Kitchen </title>
<meta property="og:site_name" content="The Kitchen">
<meta name="description" content="How to fry.">
<link rel="shortcut icon" href="/fav.png">
</head><body><svg><title>not this</title></svg></body></html>`
	m := ParsePageMeta([]byte(page), base)
	if m.Title != "Fish & Chips - The Kitchen" || m.Site != "The Kitchen" || m.Description != "How to fry." || m.Icon != "https://example.com/fav.png" {
		t.Fatalf("%+v", m)
	}
	if got := CleanLinkTitle("example.com", m.BestTitle(), m.Site, nil, false); got != "Fish & Chips" {
		t.Fatalf("clean %q", got)
	}
	if got := CleanLinkTitle("example.com", m.BestTitle(), m.Site, nil, true); got != "Fish & Chips - The Kitchen" {
		t.Fatalf("keep %q", got)
	}
	// og:title wins, unless it is only the site's name.
	m = ParsePageMeta([]byte(`<head><title>T | S</title><meta property="og:title" content="S"><meta property="og:site_name" content="S"></head>`), nil)
	if m.BestTitle() != "T | S" {
		t.Fatalf("best %q", m.BestTitle())
	}
	m = ParsePageMeta([]byte(`<head><title>T | S</title><meta property="og:title" content="Real"></head>`), nil)
	if m.BestTitle() != "Real" {
		t.Fatalf("og %q", m.BestTitle())
	}
}

func TestDecodePage(t *testing.T) {
	// "Café" in latin-1, named by a meta tag only.
	b := append([]byte(`<html><head><meta charset="iso-8859-1"><title>Caf`), 0xe9, '<', '/', 't', 'i', 't', 'l', 'e', '>')
	m := ParsePageMeta(DecodePage(b, "text/html"), nil)
	if m.Title != "Café" {
		t.Fatalf("%q", m.Title)
	}
	// The header beats the meta.
	b = append([]byte(`<title>Caf`), 0xe9, '<', '/', 't', 'i', 't', 'l', 'e', '>')
	if m := ParsePageMeta(DecodePage(b, "text/html; charset=windows-1252"), nil); m.Title != "Café" {
		t.Fatalf("%q", m.Title)
	}
}

func TestCleanLinkTitleRules(t *testing.T) {
	cases := []struct{ host, title, site, want string }{
		{"www.youtube.com", "Never Gonna Give You Up - YouTube", "", "Never Gonna Give You Up"},
		{"github.com", "GitHub - ihdavids/orgs: Org mode server", "", "ihdavids/orgs: Org mode server"},
		{"github.com", "Paste titles · Issue #12 · ihdavids/orgs · GitHub", "", "Paste titles · Issue #12 · ihdavids/orgs"},
		{"en.wikipedia.org", "Org-mode - Wikipedia", "", "Org-mode"},
		{"stackoverflow.com", "python - How do I x? - Stack Overflow", "", "How do I x?"},
		{"example.com", "MDN | Array.map", "MDN", "Array.map"},
		{"example.com", "YouTube", "YouTube", "YouTube"}, // never to nothing
		{"notyoutube.com", "x - YouTube", "", "x - YouTube"},
	}
	for _, c := range cases {
		if got := CleanLinkTitle(c.host, c.title, c.site, DefaultLinkTitleRules, false); got != c.want {
			t.Errorf("%s %q: got %q want %q", c.host, c.title, got, c.want)
		}
	}
	user := []LinkTitleRule{{Host: "docs.example.com", Strip: []string{` \| Docs$`, `[`}}} // a bad regexp is skipped
	if got := CleanLinkTitle("docs.example.com", "Intro | Docs", "", user, false); got != "Intro" {
		t.Fatal(got)
	}
}

func TestTitleableLinks(t *testing.T) {
	lines := strings.Split(`* See https://a.example/x.
:PROPERTIES:
:URL: https://skip.example/
:END:
Read (https://b.example/p_(q)) and [[https://c.example/]] and [[https://d.example/][has one]].
#+begin_src sh
curl https://skip.example/
#+end_src
#+CAPTION: https://skip.example/
| https://skip.example/ |
: https://skip.example/
=https://skip.example/= <https://skip.example/>
- https://e.example/?a=1&b=2, then`, "\n")
	got := []string{}
	for _, l := range TitleableLinks(lines) {
		got = append(got, l.Kind+" "+l.Url)
	}
	want := "bare https://a.example/x|bare https://b.example/p_(q)|link https://c.example/|bare https://e.example/?a=1&b=2"
	if strings.Join(got, "|") != want {
		t.Fatalf("%v", got)
	}
	out, n := RetitleLines(lines, map[string]string{
		"https://a.example/x": "A [one]", "https://c.example/": "C", "https://e.example/?a=1&b=2": "",
	})
	if n != 2 || out[0] != "* See [[https://a.example/x][A {one}]]." || out[4] != "Read (https://b.example/p_(q)) and [[https://c.example/][C]] and [[https://d.example/][has one]]." {
		t.Fatalf("%d\n%s", n, strings.Join(out, "\n"))
	}
	for i := range lines {
		if i != 0 && i != 4 && out[i] != lines[i] {
			t.Fatalf("line %d changed", i)
		}
	}
}

// The same cases as worg's linktitle.test.ts and VS Code's link-title-model.test.ts.
func TestIsWebUrl(t *testing.T) {
	for s, want := range map[string]bool{
		"https://example.com/x?y=1": true, "  http://a.b/  ": true, "https://": false,
		"ftp://example.com": false, "example.com": false, "see https://a.b": false,
		"https://a.b\nhttps://c.d": false,
	} {
		if IsWebUrl(s) != want {
			t.Errorf("%q", s)
		}
	}
}
