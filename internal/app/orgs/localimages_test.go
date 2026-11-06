package orgs

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestLocalImagesScanLines(t *testing.T) {
	lines := strings.Split(`* Trip
[[https://site.example/a.png]]
See [[https://site.example/b.JPG][the view]] and [[https://site.example/page.html][a page]].
  https://cdn.example/c.webp
https://example.com/article
[[https://cdn.example/photo-1501]]
[[https://example.com/story][A story]]
#+CAPTION: [[https://site.example/d.png]]
:IMAGE: [[https://site.example/e.png]]
#+BEGIN_SRC org
[[https://site.example/f.png]]
#+END_SRC
- [[https://site.example/g.gif]]
https://site.example/x.svg`, "\n")
	got := localImagesScanLines(lines, false)
	type w struct {
		url         string
		line        int
		alone, sure bool
		bare        bool
		desc        string
	}
	want := []w{
		{"https://site.example/a.png", 2, true, true, false, ""},
		{"https://site.example/b.JPG", 3, false, true, false, "the view"},
		{"https://cdn.example/c.webp", 4, true, true, true, ""},
		{"https://example.com/article", 5, true, false, true, ""},
		{"https://cdn.example/photo-1501", 6, true, false, false, ""},
		{"https://site.example/g.gif", 13, false, true, false, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("found %d, want %d: %+v", len(got), len(want), got)
	}
	for i, g := range got {
		x := want[i]
		if g.Url != x.url || g.Line != x.line || g.Alone != x.alone || g.Sure != x.sure || g.Bare != x.bare || g.Desc != x.desc {
			t.Errorf("%d: got %+v, want %+v", i, g, x)
		}
	}
	if n := len(localImagesScanLines([]string{"https://site.example/x.svg"}, true)); n != 1 {
		t.Errorf("svg on: %d", n)
	}
}

func TestLocalImagesRewrite(t *testing.T) {
	in := strings.Split(`* Trip
  #+CAPTION: The lake
  #+ATTR_HTML: :width 400
  [[https://site.example/a.png]]
See [[https://site.example/b.jpg][the view]] here.
https://cdn.example/c.webp
[[https://gone.example/d.png]]`, "\n")
	got := map[string]string{
		"https://site.example/a.png": "images/a.png",
		"https://site.example/b.jpg": "images/b.jpg",
		"https://cdn.example/c.webp": "images/c.webp",
	}
	out, n := localImagesRewrite(in, got, "2026-11-05 10:20:33", true)
	want := `* Trip
  #+DOWNLOADED: https://site.example/a.png @ 2026-11-05 10:20:33
  #+CAPTION: The lake
  #+ATTR_HTML: :width 400
  [[file:images/a.png]]
See [[file:images/b.jpg][the view]] here.
#+DOWNLOADED: https://cdn.example/c.webp @ 2026-11-05 10:20:33
[[file:images/c.webp]]
[[https://gone.example/d.png]]`
	if n != 3 || strings.Join(out, "\n") != want {
		t.Fatalf("n=%d\n%s", n, strings.Join(out, "\n"))
	}
	// Again: nothing remote is left to change.
	if _, n := localImagesRewrite(out, got, "x", true); n != 0 {
		t.Errorf("second pass changed %d", n)
	}
	// Without annotations.
	out, _ = localImagesRewrite(in, got, "x", false)
	if strings.Contains(strings.Join(out, "\n"), "DOWNLOADED") {
		t.Errorf("annotated with annotate off")
	}
}

func TestLocalImagesNamesAndBytes(t *testing.T) {
	for u, want := range map[string]string{
		"https://site.example/pics/Lake%20View.PNG?w=800": "Lake-View",
		"https://cdn.example/photo-1501785888041":         "photo-1501785888041",
	} {
		if g := localImagesBaseName(u); g != want {
			t.Errorf("%s: %q, want %q", u, g, want)
		}
	}
	if g := localImagesBaseName("https://site.example/"); !strings.HasPrefix(g, "image-") {
		t.Errorf("no name: %q", g)
	}
	if localImagesPictureExt([]byte("<html>hello</html>"), true) != "" {
		t.Errorf("html read as a picture")
	}
	if localImagesPictureExt([]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`), false) != "" {
		t.Errorf("svg kept with svg off")
	}
	for _, h := range []string{"127.0.0.1", "10.1.2.3", "192.168.0.4", "169.254.169.254", "100.64.0.1", "::1"} {
		if !strings.Contains(localImagesRefusal("http://"+h+"/x.png"), "private") && !strings.Contains(localImagesRefusal("http://["+h+"]/x.png"), "private") {
			t.Errorf("%s not refused", h)
		}
	}
	if localImagesRefusal("https://93.184.215.14/x.png") != "" {
		t.Errorf("a public address refused")
	}
}

func localImagesTestPng(t *testing.T, w int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, 1))
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestLocalImagesFetchEndToEnd(t *testing.T) {
	withFeatures(t, func(f *common.FeatureSettings) { f.LocalImages = true })
	pic, other := localImagesTestPng(t, 2), localImagesTestPng(t, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "orgs/") {
			t.Errorf("user agent %q", ua)
		}
		switch r.URL.Path {
		case "/a.png", "/same.png", "/photo-1":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pic)
		case "/b.png":
			w.Write(other)
		case "/lie.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("<html>not a picture</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	paths := loadOrg(t, map[string]string{"trip.org": "* Trip\n#+CAPTION: Lake\n[[" + srv.URL + "/a.png]]\n" +
		"[[" + srv.URL + "/same.png][same bytes]]\n" + srv.URL + "/b.png\n[[" + srv.URL + "/lie.png]]\n" +
		"[[" + srv.URL + "/photo-1]]\n"})
	dir := filepath.Dir(paths["trip.org"])
	Conf().Server.OrgDirs = []string{dir}

	// Loopback is refused unless allowed.
	res := localImagesLocalize("trip.org", nil, false, false, newLocalImagesRun())
	if len(res.Fetched) != 0 || len(res.Failed) == 0 || !strings.Contains(res.Failed[0].Why, "private") {
		t.Fatalf("loopback not refused: %+v", res)
	}
	Conf().Server.LocalImages.AllowPrivate = true
	Conf().Server.LocalImages.Ignore = []string{"ignored.example"}

	body, _ := json.Marshal(map[string]interface{}{"File": "trip.org"})
	rec := httptest.NewRecorder()
	PostLocalImagesFetch(rec, httptest.NewRequest("POST", "/localimages/fetch", bytes.NewReader(body)))
	var ans struct {
		Ok             bool
		Fetched, Wrote int
		Files          []LocalImagesFile
	}
	json.Unmarshal(rec.Body.Bytes(), &ans)
	if !ans.Ok || ans.Fetched != 4 || ans.Wrote != 1 {
		t.Fatalf("answer %s", rec.Body.String())
	}
	text, _ := os.ReadFile(paths["trip.org"])
	s := string(text)
	for _, want := range []string{
		"#+DOWNLOADED: " + srv.URL + "/a.png @ ",
		"#+CAPTION: Lake\n[[file:images/a.png]]\n",
		"[[file:images/a.png][same bytes]]", // same bytes, same file
		"[[file:images/b.png]]",
		"[[" + srv.URL + "/lie.png]]", // not a picture: left alone
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if strings.Index(s, "#+DOWNLOADED: "+srv.URL+"/a.png") > strings.Index(s, "#+CAPTION") {
		t.Errorf("annotation below the caption:\n%s", s)
	}
	ents, _ := os.ReadDir(filepath.Join(dir, "images"))
	if len(ents) != 2 {
		t.Errorf("want 2 pictures kept (dedupe), got %d", len(ents))
	}

	// Orphans: an unlinked picture is listed; deleting needs Confirm.
	os.WriteFile(filepath.Join(dir, "images", "old.png"), other[:len(other)-1], 0644)
	orphans, _ := localImagesOrphans()
	if len(orphans) != 1 || filepath.Base(orphans[0].Path) != "old.png" {
		t.Fatalf("orphans %+v", orphans)
	}
	Conf().Server.NoAuth = true
	body, _ = json.Marshal(map[string]interface{}{"Delete": []string{orphans[0].Path}})
	rec = httptest.NewRecorder()
	PostLocalImagesOrphans(rec, httptest.NewRequest("POST", "/localimages/orphans", bytes.NewReader(body)))
	if _, err := os.Stat(orphans[0].Path); err != nil {
		t.Fatalf("deleted without Confirm")
	}
	body, _ = json.Marshal(map[string]interface{}{"Delete": []string{orphans[0].Path, filepath.Join(dir, "images", "a.png")}, "Confirm": true})
	rec = httptest.NewRecorder()
	PostLocalImagesOrphans(rec, httptest.NewRequest("POST", "/localimages/orphans", bytes.NewReader(body)))
	if _, err := os.Stat(orphans[0].Path); err == nil {
		t.Errorf("orphan not deleted: %s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "images", "a.png")); err != nil {
		t.Errorf("a linked picture was deleted")
	}

	// Off is off.
	Conf().Server.Features.LocalImages = false
	rec = httptest.NewRecorder()
	RequestLocalImagesScan(rec, httptest.NewRequest("GET", "/localimages/scan?file=trip.org", nil))
	if !strings.Contains(rec.Body.String(), `"Off":true`) {
		t.Errorf("answered while off: %s", rec.Body.String())
	}
}

// The same cases as URL_KIND_CASES in worg/src/localimages.test.ts.
func TestLocalImagesUrlKind(t *testing.T) {
	for _, c := range [][2]string{
		{"https://site.example/a.png", "img"},
		{"https://site.example/pics/Lake%20View.JPEG?w=800", "img"},
		{"http://site.example/x.webp#frag", "img"},
		{"https://cdn.example/photo-1501785888041", ""},
		{"https://example.com/", ""},
		{"https://example.com/page.html", "other"},
		{"https://site.example/x.svg", "other"},
		{"ftp://site.example/a.png", "other"},
	} {
		if g := localImagesUrlKind(c[0], false); g != c[1] {
			t.Errorf("%s: %q, want %q", c[0], g, c[1])
		}
	}
	if localImagesUrlKind("https://site.example/x.svg", true) != "img" {
		t.Errorf("svg on")
	}
}
