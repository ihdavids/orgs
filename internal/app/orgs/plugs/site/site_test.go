package site

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
)

// A small site: a home page, two dated posts, a draft, notes in a folder that
// link to each other by file, heading, id and custom id, a picture, code,
// footnotes, a heading that is not for export, and a link out of the site.
var sample = map[string]string{
	"site.yaml": `title: Field Notes
description: Things worth writing down.
author: Ada
baseURL: https://example.org/notes/
`,
	"index.org": `#+TITLE: Field Notes
Welcome. Start with [[file:notes/gardening.org][the garden]].
`,
	"posts/first-light.org": `#+TITLE: First light
#+DATE: <2026-09-01 Tue>
#+FILETAGS: :astronomy:field:
#+DESCRIPTION: The telescope, at last.

The night was clear.[fn:1] See [[file:../notes/gardening.org::*Soil][soil notes]]. It had rained for three days
before, which washes the dust out of the air, and the seeing was as steady as it gets this
close to town. I set up on the lawn at nine and let the tube cool for an hour.[fn:2]

#+BEGIN_QUOTE
The sky is the daily bread of the eyes.
#+END_QUOTE

[[file:../images/moon.png]]

* Setup
#+BEGIN_SRC go
func main() { fmt.Println("hi") }
#+END_SRC
* Observations :moon:
- [X] Moon
- [ ] Saturn

| Object | Time  | Seeing |
|--------+-------+--------|
| Moon   | 21:10 | 4/5    |
| Vega   | 21:40 | 3/5    |
** The terminator
Craters along the line between light and dark stand out in relief, with
=Copernicus= the finest of them.
* What next
Saturn, before it sets in November. Inline math too: $E = mc^2$.
* Private :noexport:
Not for the world.
* Footnotes
[fn:1] Mostly.

[fn:2] A warm mirror makes the image boil.
`,
	"posts/second.org": `#+TITLE: Second post
#+DATE: 2026-10-02
#+FILETAGS: field
Shorter. Back to [[id:garden-id][the garden]] and [[#compost][compost]].
`,
	"posts/draft.org": `#+TITLE: Not yet
#+DATE: 2026-10-05
#+DRAFT: t
Unfinished.
`,
	"notes/gardening.org": `#+TITLE: Gardening
:PROPERTIES:
:ID: garden-id
:END:
* Soil
Loam.
* TODO [#A] Compost
:PROPERTIES:
:CUSTOM_ID: compost
:END:
Turn it.
* Soil
A second heading with the same name.
* Links
[[file:secret.org][gone]] and [[https://orgmode.org][org]] and [[*Compost][up there]].
`,
	"notes/.hidden/x.org": "#+TITLE: Hidden\n",
}

func buildSample(t *testing.T, theme string, drafts bool, omit ...string) (*Result, string) {
	t.Helper()
	dir := t.TempDir()
	skip := map[string]bool{}
	for _, o := range omit {
		skip[o] = true
	}
	for name, text := range sample {
		if skip[name] {
			continue
		}
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(dir, "images"), 0755)
	img := image.NewRGBA(image.Rect(0, 0, 640, 280))
	for x := 0; x < 640; x++ {
		for y := 0; y < 280; y++ {
			d := (x-320)*(x-320) + (y-140)*(y-140)
			c := uint8(20 + y/8)
			if d < 110*110 {
				c = uint8(235 - d/90)
			}
			img.Set(x, y, color.RGBA{c, c, uint8(int(c) * 9 / 10), 255})
		}
	}
	pf, _ := os.Create(filepath.Join(dir, "images", "moon.png"))
	png.Encode(pf, img)
	pf.Close()
	conf := org.New()
	conf.DefaultSettings["TODO"] = "TODO NEXT | DONE"
	sources := []Source{}
	for name := range sample {
		if !strings.HasSuffix(name, ".org") || skip[name] {
			continue
		}
		p := filepath.Join(dir, filepath.FromSlash(name))
		f, _ := os.Open(p)
		doc := conf.Parse(f, p)
		f.Close()
		sources = append(sources, Source{Path: p, Doc: doc})
	}
	res, err := Build(sources, Options{Dir: dir, Theme: theme, Drafts: drafts, Now: time.Date(2026, 10, 14, 9, 0, 0, 0, time.Local)})
	if err != nil {
		t.Fatal(err)
	}
	return res, dir
}

func file(t *testing.T, r *Result, name string) string {
	t.Helper()
	b, ok := r.Files[name]
	if !ok {
		names := []string{}
		for n := range r.Files {
			names = append(names, n)
		}
		t.Fatalf("no %s in the site; it has %v", name, names)
	}
	return string(b)
}

func TestSitePages(t *testing.T) {
	r, _ := buildSample(t, "paper", false)
	if r.Pages != 4 {
		t.Errorf("pages = %d, want 4 (no draft, nothing hidden)", r.Pages)
	}
	for _, n := range []string{"index.html", "posts/first-light.html", "posts/second.html", "notes/gardening.html",
		"tags/index.html", "tags/astronomy.html", "tags/moon.html", "archive.html", "404.html", "feed.xml",
		"sitemap.xml", "search.json", "assets/search.js", "assets/site.css", "assets/site.js", "images/moon.png"} {
		file(t, r, n)
	}
	if _, ok := r.Files["posts/draft.html"]; ok {
		t.Error("a draft was published")
	}
	if _, ok := r.Files["notes/.hidden/x.html"]; ok {
		t.Error("a hidden folder was published")
	}

	first := file(t, r, "posts/first-light.html")
	for _, want := range []string{
		`href="../assets/site.css"`,
		`href="../notes/gardening.html#soil"`, // file:...::*Soil
		`<img src="../images/moon.png"`,
		`id="setup"`,
		`class="chroma"`,
		`class="footnote-reference"`,
		`Mostly.`,
		`class="tag" href="../tags/astronomy.html"`,
		`href="../tags/moon.html"`, // a heading's tag
	} {
		if !strings.Contains(first, want) {
			t.Errorf("first-light.html is missing %s", want)
		}
	}
	for _, not := range []string{"Not for the world", `id="footnotes"`} {
		if strings.Contains(first, not) {
			t.Errorf("first-light.html should not have %q", not)
		}
	}

	garden := file(t, r, "notes/gardening.html")
	for _, want := range []string{
		`id="soil"`, `id="soil-2"`, // duplicate titles get their own ids
		`id="compost"`, // CUSTOM_ID wins
		`class="kw kw-todo kw-todo">TODO`,
		`class="pri pri-a">A`,
		`class="dead-link"`, // secret.org is not in the site
		`href="https://orgmode.org" class="ext"`,
		`href="#compost" class="heading-link"`,
		"Linked from", `href="../posts/first-light.html"`, `href="../posts/second.html"`, // backlinks
	} {
		if !strings.Contains(garden, want) {
			t.Errorf("gardening.html is missing %s", want)
		}
	}
	second := file(t, r, "posts/second.html")
	if !strings.Contains(second, `href="../notes/gardening.html">the garden`) {
		t.Error("an id: link to a file's own ID did not land on the page")
	}
	if !strings.Contains(second, `href="../notes/gardening.html#compost"`) {
		t.Error("a #custom-id link did not land on the heading")
	}
	if !strings.Contains(second, `Older</span>First light`) {
		t.Error("the pager does not lead to the older post")
	}

	home := file(t, r, "index.html")
	if !strings.Contains(home, "Welcome.") || !strings.Contains(home, `href="notes/gardening.html"`) {
		t.Error("index.org is not the home page")
	}
	if strings.Index(home, "Second post") > strings.Index(home, "First light") {
		t.Error("recent posts are not newest first")
	}

	feed := file(t, r, "feed.xml")
	if !strings.Contains(feed, "<link href=\"https://example.org/notes/posts/second.html\"/>") || strings.Contains(feed, "Gardening") {
		t.Errorf("feed is wrong:\n%s", feed)
	}
	if !strings.Contains(file(t, r, "404.html"), `href="https://example.org/notes/assets/site.css"`) {
		t.Error("404 page links should be absolute when there is a base url")
	}

	var idx []map[string]interface{}
	if err := json.Unmarshal(r.Files["search.json"], &idx); err != nil || len(idx) < 4 {
		t.Errorf("search index: %v, %d entries", err, len(idx))
	}
	found := false
	for _, w := range r.Warnings {
		if strings.Contains(w, "secret.org") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning for the dead link: %v", r.Warnings)
	}
}

func TestSiteDrafts(t *testing.T) {
	r, _ := buildSample(t, "paper", true)
	if !strings.Contains(file(t, r, "posts/draft.html"), "draft-badge") {
		t.Error("a draft asked for should be published, marked")
	}
}

func TestEveryThemeRenders(t *testing.T) {
	themes := Themes()
	if len(themes) < 7 {
		t.Fatalf("only %d themes: %v", len(themes), themes)
	}
	out := os.Getenv("SITE_RENDER_DIR")
	for _, th := range themes {
		r, _ := buildSample(t, th.Name, false)
		if th.Description == "" {
			t.Errorf("%s has no description", th.Name)
		}
		css := file(t, r, "assets/site.css")
		if !strings.Contains(css, "theme: "+th.Name) || !strings.Contains(css, `:root[data-theme="dark"] .chroma`) {
			t.Errorf("%s: css lacks the theme or dark code colours", th.Name)
		}
		// A home page made by the theme, for a site with no index.org.
		gen, _ := buildSample(t, th.Name, false, "index.org")
		if !strings.Contains(file(t, gen, "index.html"), "hero") && th.Name != "journal" {
			t.Errorf("%s: no generated home page", th.Name)
		}
		if out != "" {
			for suffix, res := range map[string]*Result{"": r, "-gen": gen} {
				dir := filepath.Join(out, "site-"+th.Name+suffix)
				os.RemoveAll(dir)
				for name, data := range res.Files {
					p := filepath.Join(dir, filepath.FromSlash(name))
					os.MkdirAll(filepath.Dir(p), 0755)
					os.WriteFile(p, data, 0644)
				}
			}
		}
	}
	if _, err := loadTheme("../etc"); err == nil {
		t.Error("a theme name with a path in it was accepted")
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Hello, World!": "hello-world", "  a  b ": "a-b", "C++ / Go": "c-go", "Ünïcode": "ünïcode", "!!!": "section"} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTagCalledIndex(t *testing.T) {
	if got := tagURL("Index"); got != "tags/index-tag.html" {
		t.Errorf("tagURL(Index) = %s; it would overwrite the tags index", got)
	}
}
