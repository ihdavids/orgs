package mediadb

// The catalogues. Each answers a search with candidates and an id with an
// item; every request goes through the caller's Getter.
//
//	openlibrary  books (and ISBNs)       no key
//	googlebooks  books                   key optional (keyless is a small shared daily quota)
//	tmdb         films and TV shows      key (v3 api key or v4 read token)
//	wikidata     films, shows, games, albums, books - the fallback   no key
//	rawg         games                   key
//	igdb         games                   a Twitch client id and secret
//	musicbrainz  albums, covers from the Cover Art Archive   no key, 1 request a second
//	wikipedia    anything else           no key

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Request is one http request a source wants made.
type Request struct {
	Method string // "" is GET
	URL    string
	Body   string
	Header map[string]string
}

// Getter makes a request and answers with the body of a 2xx answer.
type Getter func(Request) ([]byte, error)

// NotFound is what a Getter answers a 404 with, so a source can tell "no such
// thing" from "the network is down".
type NotFound struct{ URL string }

func (e NotFound) Error() string { return "not found: " + e.URL }

// Config is what the sources need from the yaml, keys already read.
type Config struct {
	Keys     map[string]string // tmdb, googlebooks, rawg, igdbClientId, igdbClientSecret
	Urls     map[string]string // a service's base url, to point it elsewhere
	Language string            // "en"
}

func (c *Config) key(name string) string { return strings.TrimSpace(c.Keys[name]) }

func (c *Config) lang() string {
	if l := strings.TrimSpace(c.Language); l != "" {
		return strings.ToLower(strings.SplitN(strings.SplitN(l, "-", 2)[0], "_", 2)[0])
	}
	return "en"
}

var defaultUrls = map[string]string{
	"openlibrary":   "https://openlibrary.org",
	"olcovers":      "https://covers.openlibrary.org",
	"googlebooks":   "https://www.googleapis.com/books/v1",
	"tmdb":          "https://api.themoviedb.org/3",
	"tmdbimages":    "https://image.tmdb.org/t/p",
	"tmdbsite":      "https://www.themoviedb.org",
	"wikidata":      "https://www.wikidata.org",
	"wikipedia":     "https://{lang}.wikipedia.org",
	"commons":       "https://commons.wikimedia.org",
	"rawg":          "https://api.rawg.io/api",
	"igdb":          "https://api.igdb.com/v4",
	"twitch":        "https://id.twitch.tv",
	"musicbrainz":   "https://musicbrainz.org/ws/2",
	"coverart":      "https://coverartarchive.org",
	"musicbrainzui": "https://musicbrainz.org",
}

// ServiceNames is every base url the yaml can override.
func ServiceNames() []string {
	m := map[string]bool{}
	for k := range defaultUrls {
		m[k] = true
	}
	return sortedKeys(m)
}

func (c *Config) url(name string) string {
	u := strings.TrimRight(strings.TrimSpace(c.Urls[name]), "/")
	if u == "" {
		u = defaultUrls[name]
	}
	return strings.ReplaceAll(u, "{lang}", c.lang())
}

// Source is one catalogue.
type Source struct {
	Name        string
	Label       string
	Kinds       []string
	Keys        []string // the keys it needs, all of them
	OptionalKey bool     // works without, worse
	Site        string
	Attribution string
	search      func(c *Config, get Getter, kind, q string) ([]Candidate, error)
	details     func(c *Config, get Getter, kind, id string) (*Item, error)
}

// Sources is every catalogue, in the order they are tried for a kind.
var Sources = []*Source{
	{Name: "openlibrary", Label: "Open Library", Kinds: []string{Book}, Site: "https://openlibrary.org",
		Attribution: "Book data from Open Library (openlibrary.org)", search: olSearch, details: olDetails},
	{Name: "googlebooks", Label: "Google Books", Kinds: []string{Book}, Keys: []string{"googlebooks"}, OptionalKey: true,
		Site: "https://books.google.com", Attribution: "Book data from Google Books", search: gbSearch, details: gbDetails},
	{Name: "tmdb", Label: "TMDB", Kinds: []string{Film, Show}, Keys: []string{"tmdb"}, Site: "https://www.themoviedb.org",
		Attribution: "This product uses the TMDB API but is not endorsed or certified by TMDB.", search: tmdbSearch, details: tmdbDetails},
	{Name: "igdb", Label: "IGDB", Kinds: []string{Game}, Keys: []string{"igdbClientId", "igdbClientSecret"}, Site: "https://www.igdb.com",
		Attribution: "Game data from IGDB.com", search: igdbSearch, details: igdbDetails},
	{Name: "rawg", Label: "RAWG", Kinds: []string{Game}, Keys: []string{"rawg"}, Site: "https://rawg.io",
		Attribution: "Game data from RAWG (rawg.io)", search: rawgSearch, details: rawgDetails},
	{Name: "musicbrainz", Label: "MusicBrainz", Kinds: []string{Album}, Site: "https://musicbrainz.org",
		Attribution: "Album data from MusicBrainz, covers from the Cover Art Archive", search: mbSearch, details: mbDetails},
	{Name: "wikidata", Label: "Wikidata", Kinds: []string{Film, Show, Game, Album, Book}, Site: "https://www.wikidata.org",
		Attribution: "Data from Wikidata (CC0) and text from Wikipedia (CC BY-SA)", search: wdSearch, details: wdDetails},
	{Name: "wikipedia", Label: "Wikipedia", Kinds: []string{Other}, Site: "https://www.wikipedia.org",
		Attribution: "Text from Wikipedia (CC BY-SA)", search: wpSearch, details: wpDetails},
}

// SourceNamed is a source by name, or nil.
func SourceNamed(name string) *Source {
	for _, s := range Sources {
		if strings.EqualFold(s.Name, name) {
			return s
		}
	}
	return nil
}

// Serves says whether a source has kind among its kinds.
func (s *Source) Serves(kind string) bool {
	for _, k := range s.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Missing is the keys a source needs and has not been given ("" when it can
// be used). A source with an optional key is never missing one.
func (s *Source) Missing(c *Config) []string {
	if s.OptionalKey {
		return nil
	}
	out := []string{}
	for _, k := range s.Keys {
		if c.key(k) == "" {
			out = append(out, k)
		}
	}
	return out
}

// Usable says whether a source can be asked.
func (s *Source) Usable(c *Config) bool { return len(s.Missing(c)) == 0 }

// SourcesFor is the sources for a kind in the order to try them: the
// yaml's order when it gives one, else the default order. Unusable ones are
// left in; the caller skips them and says why.
func SourcesFor(kind string, order []string) []*Source {
	out := []*Source{}
	if len(order) > 0 {
		for _, n := range order {
			if s := SourceNamed(n); s != nil && s.Serves(kind) {
				out = append(out, s)
			}
		}
		return out
	}
	for _, s := range Sources {
		if s.Serves(kind) {
			out = append(out, s)
		}
	}
	return out
}

// Search asks one source.
func (s *Source) Search(c *Config, get Getter, kind, q string) ([]Candidate, error) {
	if m := s.Missing(c); len(m) > 0 {
		return nil, fmt.Errorf("%s needs %s (mediaDb.keys)", s.Label, strings.Join(m, " and "))
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, fmt.Errorf("nothing to search for")
	}
	out, err := s.search(c, get, kind, q)
	for i := range out {
		out[i].Kind, out[i].Source = kind, s.Name
	}
	return out, err
}

// Details asks one source for one thing in full.
func (s *Source) Details(c *Config, get Getter, kind, id string) (*Item, error) {
	if m := s.Missing(c); len(m) > 0 {
		return nil, fmt.Errorf("%s needs %s (mediaDb.keys)", s.Label, strings.Join(m, " and "))
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("no id")
	}
	it, err := s.details(c, get, kind, id)
	if err != nil {
		return nil, err
	}
	it.Kind, it.Source = kind, s.Name
	if it.Id == "" {
		it.Id = id
	}
	it.Title = strings.TrimSpace(it.Title)
	if it.Title == "" {
		return nil, fmt.Errorf("%s has no title for %s", s.Label, id)
	}
	it.Creators = uniq(it.Creators)
	it.Cast = uniq(it.Cast)
	it.Genres = uniq(it.Genres)
	it.Platforms = uniq(it.Platforms)
	return it, nil
}

// ---------------------------------------------------------------------------
// Shared
// ---------------------------------------------------------------------------

func getJSON(get Getter, r Request, v interface{}) error {
	b, err := get(r)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("an answer that is not json from %s", hostOf(r.URL))
	}
	return nil
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Host
	}
	return u
}

func esc(s string) string { return url.QueryEscape(s) }

// https makes a protocol-relative or http url https.
func https(u string) string {
	u = strings.TrimSpace(u)
	switch {
	case strings.HasPrefix(u, "//"):
		return "https:" + u
	case strings.HasPrefix(u, "http://"):
		return "https://" + strings.TrimPrefix(u, "http://")
	}
	return u
}

var dateLayouts = []string{"2006-01-02", "January 2, 2006", "Jan 2, 2006", "2 January 2006", "January 2006", "Jan 2006", "2006-01", "January, 2006"}

// isoDate is a date as 2005-08-02, 2005-08 or 2005 - however much of it
// there is - or what was given when it is none of those.
func isoDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			switch {
			case strings.Contains(l, "2,") || strings.HasPrefix(l, "2 ") || l == "2006-01-02":
				return t.Format("2006-01-02")
			default:
				return t.Format("2006-01")
			}
		}
	}
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		if _, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return s[:10]
		}
	}
	if y := yearOf(s); y != "" {
		return y
	}
	return s
}

func names(xs []struct{ Name string }) []string {
	out := []string{}
	for _, x := range xs {
		out = append(out, x.Name)
	}
	return out
}

// ---------------------------------------------------------------------------
// Open Library
// ---------------------------------------------------------------------------

type olDoc struct {
	Key              string
	Title            string
	Subtitle         string
	AuthorName       []string `json:"author_name"`
	FirstPublishYear int      `json:"first_publish_year"`
	CoverI           int      `json:"cover_i"`
}

func olCover(c *Config, id int, size string) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/b/id/%d-%s.jpg", c.url("olcovers"), id, size)
}

func olSearch(c *Config, get Getter, kind, q string) ([]Candidate, error) {
	if isbn, ok := NormISBN(q); ok {
		it, err := olDetails(c, get, kind, isbn)
		if err != nil {
			if _, nf := err.(NotFound); nf {
				return []Candidate{}, nil
			}
			return nil, err
		}
		return []Candidate{{Id: isbn, Title: it.Title, Year: it.Year(), Creator: joinN(it.Creators, 2), Thumb: it.Cover, Isbn: it.Isbn}}, nil
	}
	var res struct{ Docs []olDoc }
	u := c.url("openlibrary") + "/search.json?q=" + esc(q) + "&limit=12&fields=key,title,subtitle,author_name,first_publish_year,cover_i"
	if err := getJSON(get, Request{URL: u}, &res); err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, d := range res.Docs {
		year := ""
		if d.FirstPublishYear > 0 {
			year = strconv.Itoa(d.FirstPublishYear)
		}
		out = append(out, Candidate{Id: strings.TrimPrefix(d.Key, "/works/"), Title: d.Title, Year: year,
			Creator: joinN(d.AuthorName, 2), Thumb: olCover(c, d.CoverI, "M")})
	}
	return out, nil
}

type olText struct{ S string }

// A description is a string or {"type": "/type/text", "value": "..."}.
func (t *olText) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		t.S = s
		return nil
	}
	var o struct{ Value string }
	if err := json.Unmarshal(b, &o); err != nil {
		return nil
	}
	t.S = o.Value
	return nil
}

type olKey struct{ Key string }

type olEdition struct {
	Key           string
	Title         string
	Subtitle      string
	PublishDate   string   `json:"publish_date"`
	Publishers    []string `json:"publishers"`
	NumberOfPages int      `json:"number_of_pages"`
	Isbn13        []string `json:"isbn_13"`
	Isbn10        []string `json:"isbn_10"`
	Covers        []int
	Languages     []olKey
	Works         []olKey
	Description   olText
}

type olWork struct {
	Key         string
	Title       string
	Subtitle    string
	Type        olKey
	Location    string
	Description olText
	Subjects    []string
	Covers      []int
}

var olLanguages = map[string]string{"eng": "English", "fre": "French", "ger": "German", "spa": "Spanish", "ita": "Italian",
	"por": "Portuguese", "dut": "Dutch", "jpn": "Japanese", "chi": "Chinese", "rus": "Russian", "swe": "Swedish", "nor": "Norwegian",
	"dan": "Danish", "fin": "Finnish", "pol": "Polish", "kor": "Korean", "ara": "Arabic", "heb": "Hebrew", "gre": "Greek"}

func (e *olEdition) isbn() string {
	for _, x := range append(append([]string{}, e.Isbn13...), e.Isbn10...) {
		if n, ok := NormISBN(x); ok {
			return n
		}
	}
	return ""
}

func (e *olEdition) language() string {
	for _, l := range e.Languages {
		code := strings.TrimPrefix(l.Key, "/languages/")
		if n := olLanguages[code]; n != "" {
			return n
		}
		return code
	}
	return ""
}

// olDetails is a work (OL893414W) or an edition by ISBN: the work for the
// description, subjects and first year, an edition for the publisher, pages
// and ISBN - the one asked for, else the likeliest of the work's.
func olDetails(c *Config, get Getter, kind, id string) (*Item, error) {
	var ed *olEdition
	workID := strings.TrimPrefix(strings.TrimPrefix(id, "/works/"), "works/")
	if isbn, ok := NormISBN(id); ok {
		var e olEdition
		if err := getJSON(get, Request{URL: c.url("openlibrary") + "/isbn/" + isbn + ".json"}, &e); err != nil {
			return nil, err
		}
		ed = &e
		workID = ""
		if len(e.Works) > 0 {
			workID = strings.TrimPrefix(e.Works[0].Key, "/works/")
		}
	}
	it := &Item{}
	if workID != "" {
		var w olWork
		if err := getJSON(get, Request{URL: c.url("openlibrary") + "/works/" + workID + ".json"}, &w); err != nil {
			return nil, err
		}
		// A merged work is a redirect to the one it was merged into.
		if w.Type.Key == "/type/redirect" && strings.HasPrefix(w.Location, "/works/") {
			workID = strings.TrimPrefix(w.Location, "/works/")
			w = olWork{}
			if err := getJSON(get, Request{URL: c.url("openlibrary") + "/works/" + workID + ".json"}, &w); err != nil {
				return nil, err
			}
		}
		it.Id = workID
		it.Title, it.Subtitle = w.Title, w.Subtitle
		it.Description = w.Description.S
		it.Genres = w.Subjects
		for _, cv := range w.Covers {
			if cv > 0 {
				it.Cover = olCover(c, cv, "L")
				break
			}
		}
		it.Url = c.url("openlibrary") + "/works/" + workID
		// The search index has the authors' names and the first year, which
		// the work itself does not.
		var s struct{ Docs []olDoc }
		if err := getJSON(get, Request{URL: c.url("openlibrary") + "/search.json?q=" + esc("key:/works/"+workID) + "&fields=key,title,author_name,first_publish_year,cover_i"}, &s); err == nil && len(s.Docs) > 0 {
			it.Creators = s.Docs[0].AuthorName
			if s.Docs[0].FirstPublishYear > 0 {
				it.Date = strconv.Itoa(s.Docs[0].FirstPublishYear)
			}
		}
		if ed == nil {
			var eds struct{ Entries []olEdition }
			if err := getJSON(get, Request{URL: c.url("openlibrary") + "/works/" + workID + "/editions.json?limit=30"}, &eds); err == nil {
				ed = bestEdition(eds.Entries, c.lang())
			}
		}
	}
	if ed != nil {
		if it.Title == "" {
			it.Title, it.Subtitle = ed.Title, ed.Subtitle
		}
		if it.Description == "" {
			it.Description = ed.Description.S
		}
		if it.Date == "" {
			it.Date = isoDate(ed.PublishDate)
		}
		it.Publisher = first(ed.Publishers...)
		it.Pages = ed.NumberOfPages
		it.Isbn = ed.isbn()
		it.Language = ed.language()
		// The cover of the edition asked for by ISBN is the one on the
		// shelf; a work's first cover may be a translation's, so the
		// chosen edition's wins when it is in the language asked for.
		_, byIsbn := NormISBN(id)
		inLang := false
		for _, l := range ed.Languages {
			inLang = inLang || l.Key == "/languages/"+olLangCode[c.lang()]
		}
		if byIsbn || inLang {
			for _, cv := range ed.Covers {
				if cv > 0 {
					it.Cover = olCover(c, cv, "L")
					break
				}
			}
		}
		if it.Url == "" && ed.Key != "" {
			it.Url = c.url("openlibrary") + ed.Key
		}
		if it.Id == "" {
			it.Id = strings.TrimPrefix(ed.Key, "/books/")
		}
	}
	return it, nil
}

var olLangCode = map[string]string{"en": "eng", "fr": "fre", "de": "ger", "es": "spa", "it": "ita", "pt": "por", "nl": "dut", "ja": "jpn"}

// bestEdition is the edition a person most likely has: one with an ISBN, in
// the configured language, with a page count, the newest of those.
func bestEdition(eds []olEdition, lang string) *olEdition {
	want := "/languages/" + olLangCode[lang]
	best, bestScore := -1, -1
	for i := range eds {
		e := &eds[i]
		score := 0
		if e.isbn() != "" {
			score += 4
		}
		for _, l := range e.Languages {
			if l.Key == want {
				score += 2
			}
		}
		if e.NumberOfPages > 0 {
			score++
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return nil
	}
	return &eds[best]
}

// ---------------------------------------------------------------------------
// Google Books
// ---------------------------------------------------------------------------

type gbVolume struct {
	Id         string
	VolumeInfo struct {
		Title               string
		Subtitle            string
		Authors             []string
		Publisher           string
		PublishedDate       string
		Description         string
		PageCount           int
		Categories          []string
		Language            string
		InfoLink            string
		CanonicalVolumeLink string
		IndustryIdentifiers []struct{ Type, Identifier string }
		ImageLinks          map[string]string
	}
}

func (c *Config) gbKey() string {
	if k := c.key("googlebooks"); k != "" {
		return "&key=" + esc(k)
	}
	return ""
}

func gbSearch(c *Config, get Getter, kind, q string) ([]Candidate, error) {
	if isbn, ok := NormISBN(q); ok {
		q = "isbn:" + isbn
	}
	var res struct{ Items []gbVolume }
	if err := getJSON(get, Request{URL: c.url("googlebooks") + "/volumes?q=" + esc(q) + "&maxResults=12&printType=books" + c.gbKey()}, &res); err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, v := range res.Items {
		vi := v.VolumeInfo
		out = append(out, Candidate{Id: v.Id, Title: vi.Title, Year: yearOf(vi.PublishedDate), Creator: joinN(vi.Authors, 2),
			Thumb: https(first(vi.ImageLinks["thumbnail"], vi.ImageLinks["smallThumbnail"])), Isbn: gbIsbn(&v)})
	}
	return out, nil
}

func gbIsbn(v *gbVolume) string {
	for _, want := range []string{"ISBN_13", "ISBN_10"} {
		for _, x := range v.VolumeInfo.IndustryIdentifiers {
			if x.Type == want {
				if n, ok := NormISBN(x.Identifier); ok {
					return n
				}
			}
		}
	}
	return ""
}

func gbDetails(c *Config, get Getter, kind, id string) (*Item, error) {
	var v gbVolume
	if err := getJSON(get, Request{URL: c.url("googlebooks") + "/volumes/" + url.PathEscape(id) + "?projection=full" + c.gbKey()}, &v); err != nil {
		return nil, err
	}
	vi := v.VolumeInfo
	it := &Item{Id: v.Id, Title: vi.Title, Subtitle: vi.Subtitle, Creators: vi.Authors, Publisher: vi.Publisher,
		Date: isoDate(vi.PublishedDate), Description: vi.Description, Pages: vi.PageCount, Isbn: gbIsbn(&v),
		Url: https(first(vi.CanonicalVolumeLink, vi.InfoLink))}
	for _, cat := range vi.Categories {
		it.Genres = append(it.Genres, strings.Split(cat, " / ")...)
	}
	if n := olLanguages[olLangCode[vi.Language]]; n != "" {
		it.Language = n
	}
	for _, size := range []string{"extraLarge", "large", "medium", "small", "thumbnail", "smallThumbnail"} {
		if u := vi.ImageLinks[size]; u != "" {
			it.Cover = https(strings.Replace(u, "&edge=curl", "", 1))
			break
		}
	}
	return it, nil
}

// ---------------------------------------------------------------------------
// TMDB
// ---------------------------------------------------------------------------

// A v4 read token is a JWT and goes in a header; a v3 key is a parameter.
func (c *Config) tmdb(path string, params string) Request {
	k := c.key("tmdb")
	r := Request{URL: c.url("tmdb") + path + "?" + params + "&language=" + esc(c.lang())}
	if strings.HasPrefix(k, "eyJ") {
		r.Header = map[string]string{"Authorization": "Bearer " + k}
	} else {
		r.URL += "&api_key=" + esc(k)
	}
	return r
}

func tmdbType(kind string) string {
	if kind == Show {
		return "tv"
	}
	return "movie"
}

func tmdbSearch(c *Config, get Getter, kind, q string) ([]Candidate, error) {
	t := tmdbType(kind)
	var res struct {
		Results []struct {
			Id                      int
			Title, Name             string
			ReleaseDate             string `json:"release_date"`
			FirstAirDate            string `json:"first_air_date"`
			PosterPath              string `json:"poster_path"`
			Overview                string
		}
	}
	if err := getJSON(get, c.tmdb("/search/"+t, "query="+esc(q)+"&include_adult=false"), &res); err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, r := range res.Results {
		cd := Candidate{Id: t + "/" + strconv.Itoa(r.Id), Title: first(r.Title, r.Name), Year: yearOf(first(r.ReleaseDate, r.FirstAirDate)), Blurb: blurb(r.Overview)}
		if r.PosterPath != "" {
			cd.Thumb = c.url("tmdbimages") + "/w185" + r.PosterPath
		}
		out = append(out, cd)
	}
	return out, nil
}

// blurb is the first sentence or so of a description, for a search hit.
func blurb(s string) string {
	s = oneLine(CleanText(s))
	if len(s) <= 140 {
		return s
	}
	cut := strings.LastIndex(s[:140], " ")
	if cut < 60 {
		cut = 140
	}
	return s[:cut] + "…"
}

func tmdbDetails(c *Config, get Getter, kind, id string) (*Item, error) {
	t := tmdbType(kind)
	num := id
	if i := strings.IndexByte(id, '/'); i >= 0 {
		t, num = id[:i], id[i+1:]
	}
	var d struct {
		Id                  int
		Title, Name         string
		Overview            string
		ReleaseDate         string              `json:"release_date"`
		FirstAirDate        string              `json:"first_air_date"`
		Runtime             int                 `json:"runtime"`
		EpisodeRunTime      []int               `json:"episode_run_time"`
		NumberOfSeasons     int                 `json:"number_of_seasons"`
		NumberOfEpisodes    int                 `json:"number_of_episodes"`
		Genres              []struct{ Name string }
		Networks            []struct{ Name string }
		CreatedBy           []struct{ Name string } `json:"created_by"`
		PosterPath          string                  `json:"poster_path"`
		OriginalLanguage    string                  `json:"original_language"`
		Credits             struct {
			Cast []struct{ Name string }
			Crew []struct{ Name, Job string }
		}
	}
	if err := getJSON(get, c.tmdb("/"+t+"/"+url.PathEscape(num), "append_to_response=credits"), &d); err != nil {
		return nil, err
	}
	it := &Item{Id: t + "/" + strconv.Itoa(d.Id), Title: first(d.Title, d.Name), Description: d.Overview,
		Date: first(d.ReleaseDate, d.FirstAirDate), Runtime: d.Runtime, Seasons: d.NumberOfSeasons, Episodes: d.NumberOfEpisodes,
		Genres: names(d.Genres), Url: c.url("tmdbsite") + "/" + t + "/" + strconv.Itoa(d.Id)}
	for _, p := range d.Credits.Cast {
		it.Cast = append(it.Cast, p.Name)
	}
	if t == "tv" {
		it.Creators = names(d.CreatedBy)
		it.Publisher = first(names(d.Networks)...)
	} else {
		for _, p := range d.Credits.Crew {
			if p.Job == "Director" {
				it.Creators = append(it.Creators, p.Name)
			}
		}
	}
	if d.PosterPath != "" {
		it.Cover = c.url("tmdbimages") + "/w500" + d.PosterPath
	}
	return it, nil
}

// ---------------------------------------------------------------------------
// Wikidata (and Wikipedia's summary for the text and picture)
// ---------------------------------------------------------------------------

// A search hit's description says what it is: "2016 film by Denis
// Villeneuve", "1997 studio album by Radiohead".
var wdKindRe = map[string]*regexp.Regexp{
	Film:  regexp.MustCompile(`(?i)\bfilm\b|\bmovie\b`),
	Show:  regexp.MustCompile(`(?i)television (series|show|program|programme|miniseries|drama|sitcom)|\bTV (series|show)|\b(anime|web|animated) series|\bsitcom\b|\bminiseries\b`),
	Game:  regexp.MustCompile(`(?i)video ?game`),
	Album: regexp.MustCompile(`(?i)\balbum\b`),
	Book:  regexp.MustCompile(`(?i)\b(novel|book|novella|short story collection|poetry collection|memoir|non-fiction)\b`),
}

var wdNotRe = map[string]*regexp.Regexp{
	Film: regexp.MustCompile(`(?i)film series|film director|film festival|film company|filmmaker|soundtrack|film score`),
	Show: regexp.MustCompile(`(?i)\bepisode\b|season of|character`),
	Game: regexp.MustCompile(`(?i)video game (series|developer|publisher|company|character)`),
	Album: regexp.MustCompile(`(?i)\btrack\b|\bsong\b|\bsingle\b`),
	Book: regexp.MustCompile(`(?i)book series|novelist|\bwriter\b|publisher|book chapter`),
}

var byRe = regexp.MustCompile(`(?i)\bby ([^;,()]+)$`)

func wdSearch(c *Config, get Getter, kind, q string) ([]Candidate, error) {
	var res struct {
		Search []struct{ Id, Label, Description string }
	}
	u := c.url("wikidata") + "/w/api.php?action=wbsearchentities&type=item&limit=30&format=json&search=" + esc(q) + "&language=" + c.lang() + "&uselang=" + c.lang()
	if err := getJSON(get, Request{URL: u}, &res); err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, r := range res.Search {
		d := r.Description
		if re := wdKindRe[kind]; re != nil && (!re.MatchString(d) || wdNotRe[kind].MatchString(d)) {
			continue
		}
		cd := Candidate{Id: r.Id, Title: r.Label, Blurb: d, Year: yearOf(d)}
		if m := byRe.FindStringSubmatch(strings.TrimSpace(d)); m != nil {
			cd.Creator = strings.TrimSpace(m[1])
		}
		out = append(out, cd)
		if len(out) >= 12 {
			break
		}
	}
	return out, nil
}

type wdSnak struct {
	Mainsnak struct {
		Snaktype  string
		Datavalue struct {
			Value json.RawMessage
			Type  string
		}
	}
	Rank string
}

type wdEntity struct {
	Id           string
	Labels       map[string]struct{ Value string }
	Descriptions map[string]struct{ Value string }
	Claims       map[string][]wdSnak
	Sitelinks    map[string]struct{ Title string }
}

func (e *wdEntity) label(lang string) string {
	if l, ok := e.Labels[lang]; ok {
		return l.Value
	}
	if l, ok := e.Labels["en"]; ok {
		return l.Value
	}
	for _, l := range e.Labels {
		return l.Value
	}
	return ""
}

func (e *wdEntity) values(prop string) []wdSnak {
	out := []wdSnak{}
	for _, s := range e.Claims[prop] {
		if s.Mainsnak.Snaktype == "value" && s.Rank != "deprecated" {
			out = append(out, s)
		}
	}
	return out
}

func (e *wdEntity) ids(prop string, max int) []string {
	out := []string{}
	for _, s := range e.values(prop) {
		var v struct{ Id string }
		if json.Unmarshal(s.Mainsnak.Datavalue.Value, &v) == nil && v.Id != "" {
			out = append(out, v.Id)
		}
		if len(out) >= max {
			break
		}
	}
	return out
}

func (e *wdEntity) str(prop string) string {
	for _, s := range e.values(prop) {
		var v string
		if json.Unmarshal(s.Mainsnak.Datavalue.Value, &v) == nil && v != "" {
			return v
		}
	}
	return ""
}

// date is the earliest value of a time property, to the precision it has.
func (e *wdEntity) date(prop string) string {
	best := ""
	for _, s := range e.values(prop) {
		var v struct {
			Time      string
			Precision int
		}
		if json.Unmarshal(s.Mainsnak.Datavalue.Value, &v) != nil || len(v.Time) < 11 {
			continue
		}
		t := strings.TrimPrefix(v.Time, "+")
		d := t[:10]
		switch {
		case v.Precision <= 9:
			d = t[:4]
		case v.Precision == 10:
			d = t[:7]
		}
		if best == "" || d < best {
			best = d
		}
	}
	return best
}

func (e *wdEntity) amount(prop string) (float64, string) {
	for _, s := range e.values(prop) {
		var v struct{ Amount, Unit string }
		if json.Unmarshal(s.Mainsnak.Datavalue.Value, &v) == nil {
			f, _ := strconv.ParseFloat(strings.TrimPrefix(v.Amount, "+"), 64)
			return f, v.Unit[strings.LastIndex(v.Unit, "/")+1:]
		}
	}
	return 0, ""
}

// Who made it, who published it, per kind.
var wdCreator = map[string]string{Film: "P57", Show: "P170", Game: "P178", Album: "P175", Book: "P50"}
var wdPublisher = map[string]string{Show: "P449", Game: "P123", Album: "P264", Book: "P123"}

func wdDetails(c *Config, get Getter, kind, id string) (*Item, error) {
	lang := c.lang()
	var res struct{ Entities map[string]wdEntity }
	u := c.url("wikidata") + "/w/api.php?action=wbgetentities&format=json&props=labels|descriptions|claims|sitelinks&ids=" + esc(id) +
		"&languages=" + lang + "|en&sitefilter=" + lang + "wiki"
	if err := getJSON(get, Request{URL: u}, &res); err != nil {
		return nil, err
	}
	e, ok := res.Entities[id]
	if !ok || e.Id == "" {
		return nil, NotFound{URL: u}
	}
	it := &Item{Id: e.Id, Title: e.label(lang), Url: c.url("wikidata") + "/wiki/" + e.Id}
	creators := e.ids(wdCreator[kind], 4)
	cast := []string{}
	if kind == Film || kind == Show {
		cast = e.ids("P161", 5)
	}
	pub := e.ids(wdPublisher[kind], 1)
	genres := e.ids("P136", 6)
	platforms := []string{}
	if kind == Game {
		platforms = e.ids("P400", 8)
	}
	want := uniq(append(append(append(append(append([]string{}, creators...), cast...), pub...), genres...), platforms...))
	labels := map[string]string{}
	if len(want) > 0 {
		var ls struct{ Entities map[string]wdEntity }
		lu := c.url("wikidata") + "/w/api.php?action=wbgetentities&format=json&props=labels&ids=" + esc(strings.Join(want, "|")) + "&languages=" + lang + "|en"
		if err := getJSON(get, Request{URL: lu}, &ls); err == nil {
			for k, v := range ls.Entities {
				labels[k] = v.label(lang)
			}
		}
	}
	lab := func(ids []string) []string {
		out := []string{}
		for _, i := range ids {
			if l := labels[i]; l != "" {
				out = append(out, l)
			}
		}
		return out
	}
	it.Creators, it.Cast, it.Genres, it.Platforms = lab(creators), lab(cast), lab(genres), lab(platforms)
	it.Publisher = first(lab(pub)...)
	it.Date = e.date("P577")
	if it.Date == "" {
		it.Date = e.date("P580")
	}
	if n, unit := e.amount("P2047"); n > 0 {
		if unit == "Q25235" { // hours
			n *= 60
		}
		it.Runtime = int(n + 0.5)
	}
	if n, _ := e.amount("P1104"); n > 0 {
		it.Pages = int(n)
	}
	if n, _ := e.amount("P2437"); n > 0 {
		it.Seasons = int(n)
	}
	if n, _ := e.amount("P1113"); n > 0 {
		it.Episodes = int(n)
	}
	if isbn, ok := NormISBN(first(e.str("P212"), e.str("P957"))); ok {
		it.Isbn = isbn
	}
	if f := first(e.str("P3383"), e.str("P154"), e.str("P18")); f != "" {
		it.Cover = c.url("commons") + "/wiki/Special:FilePath/" + url.PathEscape(strings.ReplaceAll(f, " ", "_")) + "?width=600"
	}
	if d, ok := e.Descriptions[lang]; ok {
		it.Description = d.Value
	}
	if sl, ok := e.Sitelinks[lang+"wiki"]; ok && sl.Title != "" {
		if s, err := wpSummary(c, get, sl.Title); err == nil {
			if s.Extract != "" {
				it.Description = s.Extract
			}
			// Wikipedia's lead picture is the poster or cover; Commons'
			// is often a still or a logo.
			if s.Image != "" {
				it.Cover = s.Image
			}
			if s.Url != "" {
				it.Url = s.Url
			}
		}
	}
	return it, nil
}

// ---------------------------------------------------------------------------
// Wikipedia
// ---------------------------------------------------------------------------

type wpSum struct {
	Title, Description, Extract, Image, Url string
}

func wpSummary(c *Config, get Getter, title string) (*wpSum, error) {
	var s struct {
		Title         string
		Description   string
		Extract       string
		Type          string
		Thumbnail     struct{ Source string }
		Originalimage struct{ Source string }
		ContentUrls   struct {
			Desktop struct{ Page string }
		} `json:"content_urls"`
	}
	u := c.url("wikipedia") + "/api/rest_v1/page/summary/" + url.PathEscape(strings.ReplaceAll(title, " ", "_"))
	if err := getJSON(get, Request{URL: u}, &s); err != nil {
		return nil, err
	}
	img := s.Originalimage.Source
	if img == "" {
		img = s.Thumbnail.Source
	}
	return &wpSum{Title: s.Title, Description: s.Description, Extract: s.Extract, Image: stripUtm(img), Url: s.ContentUrls.Desktop.Page}, nil
}

// stripUtm drops the tracking parameters Wikipedia puts on its image urls.
func stripUtm(u string) string {
	if i := strings.Index(u, "?utm_"); i >= 0 {
		return u[:i]
	}
	return u
}

func wpSearch(c *Config, get Getter, kind, q string) ([]Candidate, error) {
	var res struct {
		Pages []struct {
			Key, Title, Description string
			Thumbnail               *struct{ Url string }
		}
	}
	if err := getJSON(get, Request{URL: c.url("wikipedia") + "/w/rest.php/v1/search/title?limit=12&q=" + esc(q)}, &res); err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, p := range res.Pages {
		cd := Candidate{Id: p.Key, Title: p.Title, Blurb: p.Description, Year: yearOf(p.Description)}
		if p.Thumbnail != nil {
			cd.Thumb = stripUtm(https(p.Thumbnail.Url))
		}
		out = append(out, cd)
	}
	return out, nil
}

func wpDetails(c *Config, get Getter, kind, id string) (*Item, error) {
	s, err := wpSummary(c, get, id)
	if err != nil {
		return nil, err
	}
	return &Item{Id: strings.ReplaceAll(id, " ", "_"), Title: s.Title, Description: s.Extract, Cover: s.Image, Url: s.Url}, nil
}

// ---------------------------------------------------------------------------
// RAWG
// ---------------------------------------------------------------------------

type rawgGame struct {
	Id              int
	Slug            string
	Name            string
	Released        string
	BackgroundImage string `json:"background_image"`
	DescriptionRaw  string `json:"description_raw"`
	Description     string
	Website         string
	Platforms       []struct{ Platform struct{ Name string } }
	Genres          []struct{ Name string }
	Developers      []struct{ Name string }
	Publishers      []struct{ Name string }
}

func rawgSearch(c *Config, get Getter, kind, q string) ([]Candidate, error) {
	var res struct{ Results []rawgGame }
	if err := getJSON(get, Request{URL: c.url("rawg") + "/games?page_size=12&search=" + esc(q) + "&key=" + esc(c.key("rawg"))}, &res); err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, g := range res.Results {
		ps := []string{}
		for _, p := range g.Platforms {
			ps = append(ps, p.Platform.Name)
		}
		out = append(out, Candidate{Id: strconv.Itoa(g.Id), Title: g.Name, Year: yearOf(g.Released), Thumb: rawgCrop(g.BackgroundImage), Blurb: joinN(ps, 4)})
	}
	return out, nil
}

// rawgCrop is RAWG's smaller copy of a picture.
func rawgCrop(u string) string {
	return strings.Replace(u, "/media/games/", "/media/crop/600/400/games/", 1)
}

func rawgDetails(c *Config, get Getter, kind, id string) (*Item, error) {
	var g rawgGame
	if err := getJSON(get, Request{URL: c.url("rawg") + "/games/" + url.PathEscape(id) + "?key=" + esc(c.key("rawg"))}, &g); err != nil {
		return nil, err
	}
	it := &Item{Id: strconv.Itoa(g.Id), Title: g.Name, Date: g.Released, Description: first(g.DescriptionRaw, g.Description),
		Cover: g.BackgroundImage, Creators: names(g.Developers), Publisher: first(names(g.Publishers)...), Genres: names(g.Genres),
		Url: "https://rawg.io/games/" + first(g.Slug, strconv.Itoa(g.Id))}
	for _, p := range g.Platforms {
		it.Platforms = append(it.Platforms, p.Platform.Name)
	}
	return it, nil
}

// ---------------------------------------------------------------------------
// IGDB (Twitch's client-credentials token, kept until it runs out)
// ---------------------------------------------------------------------------

var igdbTok struct {
	sync.Mutex
	key   string
	token string
	until time.Time
}

func igdbToken(c *Config, get Getter) (string, error) {
	id, secret := c.key("igdbClientId"), c.key("igdbClientSecret")
	igdbTok.Lock()
	defer igdbTok.Unlock()
	if igdbTok.key == id+"\x00"+secret && igdbTok.token != "" && time.Now().Before(igdbTok.until) {
		return igdbTok.token, nil
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	u := c.url("twitch") + "/oauth2/token?client_id=" + esc(id) + "&client_secret=" + esc(secret) + "&grant_type=client_credentials"
	if err := getJSON(get, Request{Method: "POST", URL: u}, &t); err != nil {
		return "", fmt.Errorf("twitch would not give a token for IGDB: %v", err)
	}
	igdbTok.key, igdbTok.token = id+"\x00"+secret, t.AccessToken
	igdbTok.until = time.Now().Add(time.Duration(t.ExpiresIn-300) * time.Second)
	return t.AccessToken, nil
}

type igdbGame struct {
	Id               int
	Name             string
	Summary          string
	Url              string
	FirstReleaseDate int64 `json:"first_release_date"`
	Cover            struct {
		ImageId string `json:"image_id"`
	}
	Genres            []struct{ Name string }
	Platforms         []struct{ Name string }
	InvolvedCompanies []struct {
		Company   struct{ Name string }
		Developer bool
		Publisher bool
	} `json:"involved_companies"`
}

const igdbFields = "fields name,summary,url,first_release_date,cover.image_id,genres.name,platforms.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher;"

func igdbQuery(c *Config, get Getter, body string) ([]igdbGame, error) {
	tok, err := igdbToken(c, get)
	if err != nil {
		return nil, err
	}
	var out []igdbGame
	r := Request{Method: "POST", URL: c.url("igdb") + "/games", Body: body,
		Header: map[string]string{"Client-ID": c.key("igdbClientId"), "Authorization": "Bearer " + tok, "Content-Type": "text/plain"}}
	if err := getJSON(get, r, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func igdbCover(g *igdbGame, size string) string {
	if g.Cover.ImageId == "" {
		return ""
	}
	return "https://images.igdb.com/igdb/image/upload/t_" + size + "/" + g.Cover.ImageId + ".jpg"
}

func igdbDate(sec int64) string {
	if sec == 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format("2006-01-02")
}

func igdbSearch(c *Config, get Getter, kind, q string) ([]Candidate, error) {
	gs, err := igdbQuery(c, get, `search "`+strings.ReplaceAll(q, `"`, ``)+`"; `+igdbFields+` limit 12;`)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for i := range gs {
		g := &gs[i]
		dev := ""
		for _, ic := range g.InvolvedCompanies {
			if ic.Developer {
				dev = ic.Company.Name
				break
			}
		}
		out = append(out, Candidate{Id: strconv.Itoa(g.Id), Title: g.Name, Year: yearOf(igdbDate(g.FirstReleaseDate)), Creator: dev, Thumb: igdbCover(g, "cover_small")})
	}
	return out, nil
}

func igdbDetails(c *Config, get Getter, kind, id string) (*Item, error) {
	if _, err := strconv.Atoi(id); err != nil {
		return nil, fmt.Errorf("an IGDB id is a number, not %q", id)
	}
	gs, err := igdbQuery(c, get, igdbFields+" where id = "+id+";")
	if err != nil {
		return nil, err
	}
	if len(gs) == 0 {
		return nil, NotFound{URL: "igdb game " + id}
	}
	g := &gs[0]
	it := &Item{Id: strconv.Itoa(g.Id), Title: g.Name, Description: g.Summary, Date: igdbDate(g.FirstReleaseDate),
		Cover: igdbCover(g, "cover_big"), Genres: names(g.Genres), Platforms: names(g.Platforms), Url: g.Url}
	for _, ic := range g.InvolvedCompanies {
		if ic.Developer {
			it.Creators = append(it.Creators, ic.Company.Name)
		}
		if ic.Publisher && it.Publisher == "" {
			it.Publisher = ic.Company.Name
		}
	}
	return it, nil
}

// ---------------------------------------------------------------------------
// MusicBrainz and the Cover Art Archive
// ---------------------------------------------------------------------------

type mbCredit struct {
	Name       string
	Joinphrase string
}

func mbArtist(cs []mbCredit) string {
	var b strings.Builder
	for _, c := range cs {
		b.WriteString(c.Name + c.Joinphrase)
	}
	return strings.TrimSpace(b.String())
}

var luceneSpecial = regexp.MustCompile(`([+\-&|!(){}\[\]^"~*?:\\/])`)

func lucene(s string) string { return luceneSpecial.ReplaceAllString(strings.TrimSpace(s), `\$1`) }

// mbQuery reads "OK Computer by Radiohead" or "Radiohead - OK Computer" as
// a title and an artist; anything else is searched as written.
func mbQuery(q string) string {
	if i := strings.LastIndex(strings.ToLower(q), " by "); i > 0 {
		return `releasegroup:"` + lucene(q[:i]) + `" AND artist:"` + lucene(q[i+4:]) + `" AND primarytype:album`
	}
	if i := strings.Index(q, " - "); i > 0 {
		return `artist:"` + lucene(q[:i]) + `" AND releasegroup:"` + lucene(q[i+3:]) + `" AND primarytype:album`
	}
	return `(` + lucene(q) + `) AND primarytype:album`
}

func mbSearch(c *Config, get Getter, kind, q string) ([]Candidate, error) {
	var res struct {
		ReleaseGroups []struct {
			Id               string
			Title            string
			FirstReleaseDate string     `json:"first-release-date"`
			ArtistCredit     []mbCredit `json:"artist-credit"`
			SecondaryTypes   []string   `json:"secondary-types"`
		} `json:"release-groups"`
	}
	if err := getJSON(get, Request{URL: c.url("musicbrainz") + "/release-group/?fmt=json&limit=12&query=" + esc(mbQuery(q))}, &res); err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, r := range res.ReleaseGroups {
		cd := Candidate{Id: r.Id, Title: r.Title, Year: yearOf(r.FirstReleaseDate), Creator: mbArtist(r.ArtistCredit),
			Thumb: c.url("coverart") + "/release-group/" + r.Id + "/front-250"}
		if len(r.SecondaryTypes) > 0 {
			cd.Blurb = strings.Join(r.SecondaryTypes, ", ")
		}
		out = append(out, cd)
	}
	return out, nil
}

func mbLength(ms int) string {
	if ms <= 0 {
		return ""
	}
	s := (ms + 500) / 1000
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func mbDetails(c *Config, get Getter, kind, id string) (*Item, error) {
	var rg struct {
		Id               string
		Title            string
		FirstReleaseDate string     `json:"first-release-date"`
		ArtistCredit     []mbCredit `json:"artist-credit"`
		Genres           []struct {
			Name  string
			Count int
		}
		Tags []struct {
			Name  string
			Count int
		}
		Releases []struct {
			Id, Date, Status, Country string
		}
	}
	if err := getJSON(get, Request{URL: c.url("musicbrainz") + "/release-group/" + url.PathEscape(id) + "?fmt=json&inc=artist-credits+genres+tags+releases"}, &rg); err != nil {
		return nil, err
	}
	it := &Item{Id: rg.Id, Title: rg.Title, Date: rg.FirstReleaseDate, Url: c.url("musicbrainzui") + "/release-group/" + rg.Id,
		Cover: c.url("coverart") + "/release-group/" + rg.Id + "/front-500"}
	if a := mbArtist(rg.ArtistCredit); a != "" {
		it.Creators = []string{a}
	}
	gs := rg.Genres
	if len(gs) == 0 {
		gs = rg.Tags
	}
	sort.SliceStable(gs, func(i, j int) bool { return gs[i].Count > gs[j].Count })
	for _, g := range gs {
		it.Genres = append(it.Genres, g.Name)
	}
	// The first official release is the album as it came out.
	rel := ""
	relDate := ""
	for _, r := range rg.Releases {
		if r.Status != "" && r.Status != "Official" {
			continue
		}
		if rel == "" || (r.Date != "" && (relDate == "" || r.Date < relDate)) {
			rel, relDate = r.Id, r.Date
		}
	}
	if rel != "" {
		var r struct {
			LabelInfo []struct {
				Label *struct{ Name string }
			} `json:"label-info"`
			Media []struct {
				Tracks []struct {
					Title  string
					Length int
				}
			}
		}
		if err := getJSON(get, Request{URL: c.url("musicbrainz") + "/release/" + url.PathEscape(rel) + "?fmt=json&inc=recordings+labels"}, &r); err == nil {
			for _, l := range r.LabelInfo {
				if l.Label != nil && l.Label.Name != "" {
					it.Publisher = l.Label.Name
					break
				}
			}
			for _, m := range r.Media {
				for _, t := range m.Tracks {
					s := t.Title
					if l := mbLength(t.Length); l != "" {
						s += " (" + l + ")"
					}
					it.Tracks = append(it.Tracks, s)
				}
			}
		}
	}
	return it, nil
}
