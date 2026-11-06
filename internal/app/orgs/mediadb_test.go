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

// mediadbSite answers like Open Library, Wikidata and Wikipedia, from the
// saved answers in internal/common/mediadb/testdata.
func mediadbSite(t *testing.T) *httptest.Server {
	t.Helper()
	var pic bytes.Buffer
	png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 2, 3)))
	fx := func(w http.ResponseWriter, name string) {
		b, err := os.ReadFile(filepath.Join("../../common/mediadb/testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.UserAgent(), "orgs/") {
			t.Errorf("user agent %q", r.UserAgent())
		}
		p, q := r.URL.Path, r.URL.RawQuery
		switch {
		case strings.HasPrefix(p, "/b/id/"):
			w.Header().Set("Content-Type", "image/png")
			w.Write(pic.Bytes())
		case p == "/search.json" && strings.Contains(q, "key%3A"):
			fx(w, "ol_workyear.json")
		case p == "/search.json":
			fx(w, "ol_search.json")
		case p == "/works/OL893414W.json":
			fx(w, "ol_work.json")
		case p == "/works/OL893414W/editions.json":
			fx(w, "ol_editions.json")
		case p == "/isbn/9780441013593.json":
			fx(w, "ol_isbn.json")
		case p == "/w/api.php" && strings.Contains(q, "wbsearchentities"):
			fx(w, "wd_search.json")
		case p == "/w/api.php" && strings.Contains(q, "props=labels&"):
			fx(w, "wd_labels.json")
		case p == "/w/api.php":
			fx(w, "wd_entity.json")
		case strings.HasPrefix(p, "/api/rest_v1/page/summary/"):
			fx(w, "wp_summary.json")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mediadbOn(t *testing.T, srv *httptest.Server, kinds map[string]common.MediaDbKind) {
	withFeatures(t, func(f *common.FeatureSettings) { f.MediaDb = true })
	Conf().Server.MediaDb = common.MediaDbSettings{Contact: "test@example.com", Kinds: kinds,
		Urls: map[string]string{"openlibrary": srv.URL, "olcovers": srv.URL, "wikidata": srv.URL, "wikipedia": srv.URL}}
}

func TestMediadbSearchAddShelve(t *testing.T) {
	srv := mediadbSite(t)
	paths := loadOrg(t, map[string]string{"books.org": "#+TITLE: Books\n* Reading list\n* Elsewhere\n"})
	p := paths["books.org"]
	mediadbOn(t, srv, map[string]common.MediaDbKind{"books": {File: p, Under: "Reading list"}})
	Conf().Server.OrgDirs = []string{filepath.Dir(p)}

	res := mediadbSearch("book", "dune", "")
	if !res.Ok || res.Source != "openlibrary" || len(res.Candidates) != 5 || res.Candidates[0].Have != "" {
		t.Fatalf("%+v", res)
	}

	add, err := mediadbAdd(&MediadbAddRequest{Kind: "book", Source: "openlibrary", Id: "OL893414W", Status: "reading", Rating: 4})
	if err != nil || !add.Ok || add.Hash == "" {
		t.Fatalf("%v %+v", err, add)
	}
	got := readOrg(t, p)
	for _, want := range []string{
		"* Reading list\n\n** Dune  :fiction:science_fiction:",
		":RECORD:         book\n",
		":AUTHOR:         Frank Herbert\n",
		":PUBLISHED_DATE: 1965\n",
		":URL:            " + srv.URL + "/works/OL893414W\n",
		":MEDIA_ID:       openlibrary:OL893414W\n",
		":IMAGE:          [[file:images/book/dune.png]]\n",
		":STATUS:         reading\n",
		":RATING:         4\n",
		":STARTED_DATE:   [",
		"\n   Set on the desert planet Arrakis",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	if strings.Index(got, "** Dune") > strings.Index(got, "* Elsewhere") {
		t.Fatalf("not under Reading list:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p), "images/book/dune.png")); err != nil {
		t.Fatalf("cover: %v", err)
	}

	// The same work again - by its id or by an ISBN of it - is refused.
	again, err := mediadbAdd(&MediadbAddRequest{Kind: "book", Source: "openlibrary", Id: "OL893414W"})
	if err != nil || again.Ok || again.Existing == "" {
		t.Fatalf("again: %v %+v", err, again)
	}
	byIsbn, err := mediadbAdd(&MediadbAddRequest{Kind: "book", Id: "978-0-441-01359-3"})
	if err != nil || byIsbn.Ok || byIsbn.Existing == "" {
		t.Fatalf("isbn: %v %+v", err, byIsbn)
	}
	if res := mediadbSearch("book", "dune", ""); res.Candidates[0].Have == "" {
		t.Fatal("the search does not know it is on the shelf")
	}

	// Preview writes nothing.
	before := readOrg(t, p)
	pre, err := mediadbAdd(&MediadbAddRequest{Kind: "book", Id: "9780441013593", Preview: true, Force: true})
	if err != nil || !pre.Ok || !strings.HasPrefix(pre.Org, "* Dune  :") || readOrg(t, p) != before {
		t.Fatalf("%v %+v", err, pre)
	}

	items := mediadbAll()
	if len(items) != 1 || items[0].Shelf != "doing" || items[0].StatusWord != "reading" || items[0].Rating != 4 ||
		items[0].Creator != "Frank Herbert" || items[0].Year != "1965" || !strings.HasSuffix(items[0].Cover, "images/book/dune.png") {
		t.Fatalf("%+v", *items[0])
	}

	// A shelf in a document.
	os.WriteFile(filepath.Join(filepath.Dir(p), "shelf.org"), []byte("* Read this year\n#+BEGIN: library :kind book :shelf reading\n#+END:\n"), 0644)
	GetDb().LoadFile(filepath.Join(filepath.Dir(p), "shelf.org"), true)
	if _, err := refreshDynBlocks(filepath.Join(filepath.Dir(p), "shelf.org"), func(dynBlock) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if got := readOrg(t, filepath.Join(filepath.Dir(p), "shelf.org")); !strings.Contains(got, "| Title") || !strings.Contains(got, "][Dune]] | Frank Herbert | 1965 | reading | ★★★★") {
		t.Fatalf("%s", got)
	}

	r := 4.5
	it, err := mediadbSet(&MediadbSetRequest{Hash: items[0].Hash, Status: "done", Rating: &r})
	if err != nil || it.Shelf != "done" || it.Rating != 4.5 || it.Finished == "" {
		t.Fatalf("%v %+v", err, it)
	}
	if got := readOrg(t, p); !strings.Contains(got, ":STATUS:         read\n") || !strings.Contains(got, ":RATING:         4.5\n") {
		t.Fatalf("%s", got)
	}
}

func TestMediadbRefreshAndDefaultPlace(t *testing.T) {
	srv := mediadbSite(t)
	paths := loadOrg(t, map[string]string{"mine.org": "#+TITLE: Mine\n* Typed by hand\n:PROPERTIES:\n:RECORD: book\n:ISBN: 0441013597\n:STATUS: to read\n:END:\n* After\n"})
	p := paths["mine.org"]
	mediadbOn(t, srv, nil)
	Conf().Server.OrgDirs = []string{filepath.Dir(p)}

	items := mediadbAll()
	if len(items) != 1 || items[0].Shelf != "want" {
		t.Fatalf("%+v", items)
	}
	res, err := mediadbRefresh(items[0].Hash, false)
	if err != nil || !res.Ok {
		t.Fatalf("%v %+v", err, res)
	}
	got := readOrg(t, p)
	for _, want := range []string{":AUTHOR:", ":PUBLISHER: Ace Trade", ":MEDIA_ID: openlibrary:OL893414W", ":STATUS: to read", ":IMAGE: [[file:images/book/", "Set on the desert planet", "* Typed by hand  :fiction:"} {
		if !strings.Contains(strings.Join(strings.Fields(got), " "), strings.Join(strings.Fields(want), " ")) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	// A column-zero drawer stays in column zero.
	if strings.Contains(got, "\n   :") {
		t.Fatalf("ragged drawer:\n%s", got)
	}
	// The title is kept: refresh fills in, it does not rename.
	if !strings.Contains(got, "* Typed by hand") || strings.Index(got, "Set on the desert") > strings.Index(got, "* After") {
		t.Fatalf("%s", got)
	}

	// A film with nowhere configured: a Films collection in library.org.
	add, err := mediadbAdd(&MediadbAddRequest{Kind: "movie", Source: "wikidata", Id: "Q20382729", NoCover: true})
	if err != nil || !add.Ok {
		t.Fatalf("%v %+v", err, add)
	}
	lib := readOrg(t, filepath.Join(filepath.Dir(p), "library.org"))
	for _, want := range []string{"* Films\n", ":COLLECTION: film", "** Arrival", ":RECORD:", "film\n", ":DIRECTOR:", "Denis Villeneuve", ":RUNTIME:", "116 min", ":MEDIA_ID:", "wikidata:Q20382729", ":STATUS:", "to watch"} {
		if !strings.Contains(lib, want) {
			t.Fatalf("missing %q in\n%s", want, lib)
		}
	}
	// A second film goes into the same collection, not a second one.
	if _, err := mediadbAdd(&MediadbAddRequest{Kind: "film", Source: "wikidata", Id: "Q20382729", NoCover: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	if lib = readOrg(t, filepath.Join(filepath.Dir(p), "library.org")); strings.Count(lib, "* Films") != 1 || strings.Count(lib, "** Arrival") != 2 {
		t.Fatalf("%s", lib)
	}
}

func TestMediadbOffDoesNothing(t *testing.T) {
	withFeatures(t, func(f *common.FeatureSettings) { f.MediaDb = false })
	for _, h := range []func(http.ResponseWriter, *http.Request){RequestMediadbSearch, RequestMediadbItems, RequestMediadbSources} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest("GET", "/mediadb/x?q=dune", nil))
		if !strings.Contains(w.Body.String(), `"Off":true`) {
			t.Fatalf("%s", w.Body.String())
		}
	}
}

func TestMediadbSourcesSayWhatIsMissing(t *testing.T) {
	srv := mediadbSite(t)
	mediadbOn(t, srv, nil)
	Conf().Server.MediaDb.Keys = map[string]string{"tmdb": "env:MEDIADB_TEST_NOT_SET"}
	w := httptest.NewRecorder()
	RequestMediadbSources(w, httptest.NewRequest("GET", "/mediadb/sources", nil))
	var res struct {
		Sources []MediadbSourceInfo
		Kinds   []MediadbKindInfo
	}
	json.Unmarshal(w.Body.Bytes(), &res)
	for _, s := range res.Sources {
		if s.Name == "tmdb" && (s.Usable || len(s.Missing) != 1) {
			t.Fatalf("%+v", s)
		}
		if s.Name == "openlibrary" && !s.Usable {
			t.Fatalf("%+v", s)
		}
	}
	for _, k := range res.Kinds {
		if k.Kind == "film" && (!k.Usable || k.Statuses["done"] != "watched") {
			t.Fatalf("%+v", k)
		}
	}
	// The search skips TMDB and says so, then asks Wikidata.
	sr := mediadbSearch("film", "arrival", "")
	if !sr.Ok || sr.Source != "wikidata" || len(sr.Tried) != 2 || !strings.Contains(sr.Tried[0].Msg, "tmdb") {
		t.Fatalf("%+v", sr)
	}
}
