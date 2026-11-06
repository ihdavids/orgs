package mediadb

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixtures answers a request with the first saved answer whose pattern is in
// the url. Nothing here touches the network.
func fixtures(t *testing.T, routes ...string) (Getter, *[]string) {
	t.Helper()
	asked := []string{}
	return func(r Request) ([]byte, error) {
		asked = append(asked, r.Method+" "+r.URL)
		for i := 0; i+1 < len(routes); i += 2 {
			if strings.Contains(r.URL, routes[i]) {
				return os.ReadFile(filepath.Join("testdata", routes[i+1]))
			}
		}
		return nil, NotFound{URL: r.URL}
	}, &asked
}

func TestOpenLibrarySearchAndWork(t *testing.T) {
	get, _ := fixtures(t, "search.json?q=dune", "ol_search.json")
	c := &Config{}
	got, err := SourceNamed("openlibrary").Search(c, get, Book, "dune")
	if err != nil || len(got) != 5 {
		t.Fatalf("%v %+v", err, got)
	}
	if g := got[0]; g.Id != "OL893414W" || g.Title != "Dune" || g.Year != "1965" || g.Creator != "Frank Herbert" ||
		g.Thumb != "https://covers.openlibrary.org/b/id/11481354-M.jpg" || g.Source != "openlibrary" || g.Kind != Book {
		t.Fatalf("%+v", g)
	}

	get, asked := fixtures(t,
		"/works/OL893415W.json", "ol_work_redirect.json", // never asked: the id is the live one
		"/works/OL893414W.json", "ol_work.json",
		"key%3A%2Fworks%2FOL893414W", "ol_workyear.json",
		"/editions.json", "ol_editions.json")
	it, err := SourceNamed("openlibrary").Details(c, get, Book, "OL893414W")
	if err != nil {
		t.Fatalf("%v %v", err, *asked)
	}
	if it.Title != "Dune" || it.Date != "1965" || !reflect.DeepEqual(it.Creators, []string{"Frank Herbert"}) || it.Isbn == "" ||
		it.Cover != "https://covers.openlibrary.org/b/id/11481354-L.jpg" || it.Url != "https://openlibrary.org/works/OL893414W" ||
		!strings.HasPrefix(it.Description, "Set on the desert planet Arrakis") {
		t.Fatalf("%+v", it)
	}
	tags := Tags(it, 5)
	if !reflect.DeepEqual(tags, []string{"fiction", "science_fiction", "american_literature"}) && (len(tags) == 0 || tags[0] != "fiction") {
		t.Fatalf("tags %q", tags)
	}
	for _, tg := range tags {
		if strings.Contains(tg, "nyt") || strings.Contains(tg, "new_york") {
			t.Fatalf("noise in tags %q", tags)
		}
	}
}

func TestOpenLibraryByIsbn(t *testing.T) {
	get, _ := fixtures(t,
		"/isbn/9780441013593.json", "ol_isbn.json",
		"/works/OL893414W.json", "ol_work.json",
		"key%3A%2Fworks%2FOL893414W", "ol_workyear.json")
	c := &Config{}
	// An ISBN-10 is the same book.
	cands, err := SourceNamed("openlibrary").Search(c, get, Book, "0-441-01359-7")
	if err != nil || len(cands) != 1 || cands[0].Id != "9780441013593" {
		t.Fatalf("%v %+v", err, cands)
	}
	it, err := SourceNamed("openlibrary").Details(c, get, Book, "9780441013593")
	if err != nil {
		t.Fatal(err)
	}
	// The edition's publisher, pages, ISBN and cover; the work's id, year
	// and description.
	if it.Id != "OL893414W" || it.Publisher != "Ace Trade" || it.Pages != 544 || it.Isbn != "9780441013593" || it.Date != "1965" ||
		it.Cover != "https://covers.openlibrary.org/b/id/14565843-L.jpg" || it.Language != "English" {
		t.Fatalf("%+v", it)
	}
	f, order := Fields(it, nil)
	if !reflect.DeepEqual(order, []string{"AUTHOR", "PUBLISHED_DATE", "PUBLISHER", "PAGES", "ISBN", "LANGUAGE", "URL", "MEDIA_ID"}) {
		t.Fatalf("order %q", order)
	}
	if f["MEDIA_ID"] != "openlibrary:OL893414W" || f["AUTHOR"] != "Frank Herbert" {
		t.Fatalf("%v", f)
	}
	// The yaml's fields: list keeps only those (and MEDIA_ID).
	_, order = Fields(it, []string{"author", "isbn"})
	if !reflect.DeepEqual(order, []string{"AUTHOR", "ISBN", "MEDIA_ID"}) {
		t.Fatalf("only %q", order)
	}
}

func TestGoogleBooks(t *testing.T) {
	get, asked := fixtures(t, "/volumes?q=", "gb_search.json", "/volumes/B1hSG45JCX4C", "gb_volume.json")
	c := &Config{Keys: map[string]string{"googlebooks": "k1"}}
	s := SourceNamed("googlebooks")
	cands, err := s.Search(c, get, Book, "9780441013593")
	if err != nil || len(cands) != 2 || cands[0].Isbn != "9780441013593" || !strings.HasPrefix(cands[0].Thumb, "https://books.google.com/") {
		t.Fatalf("%v %+v", err, cands)
	}
	if !strings.Contains((*asked)[0], "q=isbn%3A9780441013593") || !strings.Contains((*asked)[0], "key=k1") {
		t.Fatalf("asked %q", *asked)
	}
	it, err := s.Details(c, get, Book, "B1hSG45JCX4C")
	if err != nil {
		t.Fatal(err)
	}
	if it.Date != "2005-08-02" || it.Pages != 544 || it.Publisher != "Penguin" || strings.Contains(it.Cover, "edge=curl") {
		t.Fatalf("%+v", it)
	}
	if got := Body(it); got != "Set on the desert planet Arrakis, Dune is the story of the boy Paul Atreides.\n\nA stunning blend of adventure & mysticism." {
		t.Fatalf("body %q", got)
	}
	if tags := Tags(it, 5); !reflect.DeepEqual(tags, []string{"fiction", "science_fiction"}) {
		t.Fatalf("tags %q", tags)
	}
	// Without a key it still works (Google's shared quota).
	if !s.Usable(&Config{}) {
		t.Fatal("google books needs no key")
	}
}

func TestTMDB(t *testing.T) {
	s := SourceNamed("tmdb")
	if s.Usable(&Config{}) || s.Missing(&Config{})[0] != "tmdb" {
		t.Fatal("tmdb needs a key")
	}
	get, asked := fixtures(t, "/search/movie", "tmdb_search_movie.json", "/movie/329865", "tmdb_movie.json", "/tv/1396", "tmdb_tv.json")
	c := &Config{Keys: map[string]string{"tmdb": "abc"}}
	cands, err := s.Search(c, get, Film, "arrival")
	if err != nil || len(cands) != 2 || cands[0].Id != "movie/329865" || cands[0].Year != "2016" ||
		cands[0].Thumb != "https://image.tmdb.org/t/p/w185/x2FJsf1ElAgr63Y3PNPtJrcmpoe.jpg" || cands[1].Thumb != "" {
		t.Fatalf("%v %+v", err, cands)
	}
	if !strings.Contains((*asked)[0], "api_key=abc") {
		t.Fatalf("%q", *asked)
	}
	it, err := s.Details(c, get, Film, "movie/329865")
	if err != nil {
		t.Fatal(err)
	}
	f, _ := Fields(it, nil)
	if f["DIRECTOR"] != "Denis Villeneuve" || f["RUNTIME"] != "116 min" || f["RELEASE_DATE"] != "2016-11-10" ||
		f["CAST"] != "Amy Adams, Jeremy Renner, Forest Whitaker" || f["URL"] != "https://www.themoviedb.org/movie/329865" || f["MEDIA_ID"] != "tmdb:movie/329865" {
		t.Fatalf("%v", f)
	}
	// A v4 token goes in a header.
	var hdr map[string]string
	get2 := func(r Request) ([]byte, error) { hdr = r.Header; return os.ReadFile("testdata/tmdb_tv.json") }
	tv, err := s.Details(&Config{Keys: map[string]string{"tmdb": "eyJhbGciOi"}}, get2, Show, "tv/1396")
	if err != nil || hdr["Authorization"] != "Bearer eyJhbGciOi" {
		t.Fatalf("%v %v", err, hdr)
	}
	f, _ = Fields(tv, nil)
	if f["CREATOR"] != "Vince Gilligan" || f["NETWORK"] != "AMC" || f["SEASONS"] != "5" || f["EPISODES"] != "62" || f["FIRST_AIRED_DATE"] != "2008-01-20" {
		t.Fatalf("%v", f)
	}
}

func TestWikidataFilm(t *testing.T) {
	s := SourceNamed("wikidata")
	get, _ := fixtures(t, "wbsearchentities", "wd_search.json", "ids=Q20382729", "wd_entity.json", "props=labels&ids=", "wd_labels.json",
		"/page/summary/Arrival_%28film%29", "wp_summary.json")
	c := &Config{}
	cands, err := s.Search(c, get, Film, "arrival")
	if err != nil {
		t.Fatal(err)
	}
	// The films only, not the albums, the track or "end".
	ids := []string{}
	for _, cd := range cands {
		ids = append(ids, cd.Id)
	}
	if !reflect.DeepEqual(ids, []string{"Q20382729", "Q540318"}) {
		t.Fatalf("%q", ids)
	}
	if cands[0].Year != "2016" || cands[0].Creator != "Denis Villeneuve" || cands[1].Creator != "David Twohy" {
		t.Fatalf("%+v", cands)
	}
	albums, _ := s.Search(c, get, Album, "arrival")
	if len(albums) != 3 || albums[0].Id != "Q2578989" || albums[1].Creator != "ABBA" {
		t.Fatalf("albums %+v", albums)
	}
	it, err := s.Details(c, get, Film, "Q20382729")
	if err != nil {
		t.Fatal(err)
	}
	if it.Title != "Arrival" || it.Date != "2016-09-01" && it.Year() != "2016" || it.Runtime != 116 ||
		len(it.Creators) != 1 || it.Creators[0] != "Denis Villeneuve" || len(it.Cast) == 0 || it.Cast[0] != "Amy Adams" ||
		it.Url != "https://en.wikipedia.org/wiki/Arrival_(film)" || !strings.HasPrefix(it.Description, "Arrival is a 2016 American") ||
		it.Cover != "https://upload.wikimedia.org/wikipedia/en/d/df/Arrival%2C_Movie_Poster.jpg" {
		t.Fatalf("%+v", it)
	}
	if len(it.Genres) == 0 {
		t.Fatalf("no genres %+v", it)
	}
}

func TestWikipedia(t *testing.T) {
	s := SourceNamed("wikipedia")
	get, _ := fixtures(t, "search/title", "wp_search.json", "/page/summary/", "wp_summary.json")
	cands, err := s.Search(&Config{}, get, Other, "eiffel tower")
	if err != nil || len(cands) == 0 || cands[0].Id != "Eiffel_Tower" || !strings.HasPrefix(cands[0].Thumb, "https://") || strings.Contains(cands[0].Thumb, "utm_") {
		t.Fatalf("%v %+v", err, cands)
	}
	it, err := s.Details(&Config{}, get, Other, "Arrival_(film)")
	if err != nil || it.Url == "" || it.Cover == "" {
		t.Fatalf("%v %+v", err, it)
	}
	if _, order := Fields(it, nil); !reflect.DeepEqual(order, []string{"URL", "MEDIA_ID"}) {
		t.Fatalf("%q", order)
	}
}

func TestMusicBrainz(t *testing.T) {
	s := SourceNamed("musicbrainz")
	get, asked := fixtures(t, "/release-group/?", "mb_search.json", "/release-group/b1392450", "mb_rg.json", "/release/", "mb_release.json")
	c := &Config{}
	cands, err := s.Search(c, get, Album, "OK Computer by Radiohead")
	if err != nil || len(cands) != 3 || cands[0].Creator != "Radiohead" || cands[0].Year != "1997" ||
		cands[0].Thumb != "https://coverartarchive.org/release-group/b1392450-e666-3926-a536-22c65f834433/front-250" {
		t.Fatalf("%v %+v", err, cands)
	}
	if !strings.Contains((*asked)[0], "releasegroup%3A%22OK+Computer%22+AND+artist%3A%22Radiohead%22") {
		t.Fatalf("query %q", (*asked)[0])
	}
	it, err := s.Details(c, get, Album, "b1392450-e666-3926-a536-22c65f834433")
	if err != nil {
		t.Fatal(err)
	}
	if it.Publisher != "Parlophone" || len(it.Tracks) != 12 || it.Tracks[0] != "Airbag (4:44)" || it.Genres[0] != "alternative rock" {
		t.Fatalf("%+v", it)
	}
	f, _ := Fields(it, nil)
	if f["ARTIST"] != "Radiohead" || f["TRACKS"] != "12" || f["RELEASE_DATE"] != "1997-05-21" || f["LABEL"] != "Parlophone" {
		t.Fatalf("%v", f)
	}
	if b := Body(it); !strings.HasPrefix(b, "1. Airbag (4:44)\n2. Paranoid Android") {
		t.Fatalf("%q", b)
	}
	if tags := Tags(it, 3); !reflect.DeepEqual(tags, []string{"alternative_rock", "art_rock", "rock"}) {
		t.Fatalf("%q", tags)
	}
}

func TestGames(t *testing.T) {
	get, _ := fixtures(t, "/games?", "rawg_search.json", "/games/3328", "rawg_game.json")
	c := &Config{Keys: map[string]string{"rawg": "r"}}
	cands, err := SourceNamed("rawg").Search(c, get, Game, "witcher")
	if err != nil || len(cands) != 2 || !strings.Contains(cands[0].Thumb, "/media/crop/600/400/games/") {
		t.Fatalf("%v %+v", err, cands)
	}
	it, err := SourceNamed("rawg").Details(c, get, Game, "3328")
	if err != nil {
		t.Fatal(err)
	}
	f, _ := Fields(it, nil)
	if f["DEVELOPER"] != "CD PROJEKT RED" || f["PLATFORMS"] != "PC, PlayStation 4, Xbox One" || f["URL"] != "https://rawg.io/games/the-witcher-3-wild-hunt" {
		t.Fatalf("%v", f)
	}

	var bodies []string
	iget := func(r Request) ([]byte, error) {
		switch {
		case strings.Contains(r.URL, "/oauth2/token"):
			return os.ReadFile("testdata/igdb_token.json")
		case strings.HasSuffix(r.URL, "/games"):
			if r.Header["Authorization"] != "Bearer tok123" || r.Header["Client-ID"] != "cid" {
				t.Fatalf("headers %v", r.Header)
			}
			bodies = append(bodies, r.Body)
			return os.ReadFile("testdata/igdb_games.json")
		}
		return nil, NotFound{URL: r.URL}
	}
	ic := &Config{Keys: map[string]string{"igdbClientId": "cid", "igdbClientSecret": "sec"}}
	if SourceNamed("igdb").Usable(&Config{Keys: map[string]string{"igdbClientId": "cid"}}) {
		t.Fatal("igdb needs both")
	}
	cands, err = SourceNamed("igdb").Search(ic, iget, Game, `witcher "3"`)
	if err != nil || len(cands) != 1 || cands[0].Creator != "CD Projekt RED" || cands[0].Year != "2015" {
		t.Fatalf("%v %+v", err, cands)
	}
	it, err = SourceNamed("igdb").Details(ic, iget, Game, "1942")
	if err != nil || it.Publisher != "Bandai Namco Entertainment" || it.Date != "2015-05-19" || it.Cover != "https://images.igdb.com/igdb/image/upload/t_cover_big/coaarl.jpg" {
		t.Fatalf("%v %+v", err, it)
	}
	if !strings.HasPrefix(bodies[0], `search "witcher 3";`) || !strings.Contains(bodies[1], "where id = 1942;") {
		t.Fatalf("%q", bodies)
	}
}

func TestSourcesFor(t *testing.T) {
	names := func(ss []*Source) string {
		out := []string{}
		for _, s := range ss {
			out = append(out, s.Name)
		}
		return strings.Join(out, ",")
	}
	if got := names(SourcesFor(Film, nil)); got != "tmdb,wikidata" {
		t.Fatal(got)
	}
	if got := names(SourcesFor(Game, nil)); got != "igdb,rawg,wikidata" {
		t.Fatal(got)
	}
	if got := names(SourcesFor(Book, []string{"googlebooks", "tmdb", "openlibrary"})); got != "googlebooks,openlibrary" {
		t.Fatal(got)
	}
	if KindOf("Movie") != Film || KindOf("tv") != Show || KindOf("music") != Album || KindOf("x") != "" {
		t.Fatal("aliases")
	}
}

// The same cases are in worg's library.test.ts (the barcode scanner).
func TestNormISBN(t *testing.T) {
	for in, want := range map[string]string{
		"9780441013593":     "9780441013593",
		"978-0-441-01359-3": "9780441013593",
		"0441013597":        "9780441013593",
		"ISBN 0-441-01359-7": "9780441013593",
		"080442957X":        "9780804429573",
		"9780441013594":     "",
		"0441013598":        "",
		"1234567890123":     "",
		"dune":              "",
	} {
		got, ok := NormISBN(in)
		if got != want || ok != (want != "") {
			t.Errorf("%q: %q %v, want %q", in, got, ok, want)
		}
	}
}

func TestShelvesAndRatings(t *testing.T) {
	for _, c := range []struct {
		status, kw  string
		done        bool
		start, fin  string
		want        string
	}{
		{"reading", "", false, "", "", Doing},
		{"Watched", "TODO", false, "", "", Done},
		{"", "DONE", true, "", "", Done},
		{"", "CANCELLED", true, "", "", Dropped},
		{"", "TODO", false, "", "", Want},
		{"", "READING", false, "", "", Doing},
		{"", "NEXT", false, "", "", Doing},
		{"", "", false, "[2026-01-02]", "", Doing},
		{"", "", false, "[2026-01-02]", "[2026-02-01]", Done},
		{"", "", false, "", "", Want},
		{"DNF", "", false, "", "", Dropped},
	} {
		if got := ShelfOf(c.status, c.kw, c.done, c.start, c.fin); got != c.want {
			t.Errorf("%+v: %s", c, got)
		}
	}
	if StatusWord(Book, Done) != "read" || StatusWord(Game, Want) != "to play" || StatusWord("x", Doing) != "in progress" {
		t.Fatal("words")
	}
	for in, want := range map[string]float64{"4": 4, "4.5": 4.5, "8/10": 4, "★★★": 3, "****": 4, "": 0, "9": 5, "3.3": 3.5, "meh": 0} {
		if got := ParseRating(in); got != want {
			t.Errorf("rating %q: %v", in, got)
		}
	}
	if RatingText(4) != "4" || RatingText(3.5) != "3.5" || RatingText(0) != "" {
		t.Fatal("rating text")
	}
}

func TestCleanText(t *testing.T) {
	in := "Paul's story ([source][1])\r\n\r\n* not a heading\n\n#+TITLE: x\n\nSee [the site](https://x.example/).\n\n----------\n**Also contained in:**\n[1]: https://example.com"
	want := "Paul's story\n\n,* not a heading\n\n,#+TITLE: x\n\nSee [[https://x.example/][the site]]."
	if got := CleanText(in); got != want {
		t.Fatalf("%q", got)
	}
	if TagOf("Science Fiction") != "science_fiction" || TagOf("nyt:x=1") != "" || TagOf("Fiction, general") != "" || TagOf("Accessible book") != "" {
		t.Fatal("tags")
	}
}
