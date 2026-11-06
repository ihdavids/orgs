// Package mediadb looks books, films, shows, games and albums up in public
// catalogues and turns what it finds into the fields of an org heading
// (features.mediaDb).
//
// It is pure: every request goes through a Getter the caller supplies, so the
// server adds the network (timeouts, size caps, a User-Agent, MusicBrainz's
// one request a second) and the tests add saved answers. Nothing here writes
// a file.
//
// A heading it files reads like this - a record of type book, so the records
// browser and collections pick it up, with fields named so records' fieldOf
// types them (AUTHOR text, PUBLISHED_DATE a date, URL a link, IMAGE a
// picture):
//
//	* Dune                                         :science_fiction:fiction:
//	:PROPERTIES:
//	:RECORD:         book
//	:ID:             ...
//	:ADDED:          [2026-11-05 Thu 09:12]
//	:AUTHOR:         Frank Herbert
//	:PUBLISHED_DATE: 1965
//	:PUBLISHER:      Ace Trade
//	:PAGES:          544
//	:ISBN:           9780441013593
//	:URL:            https://openlibrary.org/works/OL893414W
//	:IMAGE:          [[file:images/book/dune.jpg]]
//	:MEDIA_ID:       openlibrary:OL893414W
//	:STATUS:         to read
//	:END:
//
//	Set on the desert planet Arrakis, ...
package mediadb

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// The kinds of thing, each a record type of its own.
const (
	Book  = "book"
	Film  = "film"
	Show  = "show"
	Game  = "game"
	Album = "album"
	Other = "other"
)

// Kinds is every kind, in the order a picker lists them.
var Kinds = []string{Book, Film, Show, Game, Album, Other}

var kindAlias = map[string]string{
	"book": Book, "books": Book, "novel": Book,
	"film": Film, "films": Film, "movie": Film, "movies": Film,
	"show": Show, "shows": Show, "tv": Show, "series": Show, "tvshow": Show,
	"game": Game, "games": Game, "videogame": Game,
	"album": Album, "albums": Album, "music": Album, "record": Album,
	"other": Other, "wiki": Other, "wikipedia": Other, "thing": Other,
}

// KindOf is the kind a word names ("movie" is a film, "tv" a show), or "".
func KindOf(word string) string {
	return kindAlias[strings.ToLower(strings.TrimSpace(word))]
}

// KindLabel is what a kind is called in a sentence: "book", "TV show".
func KindLabel(kind string) string {
	switch kind {
	case Show:
		return "TV show"
	case Other:
		return "Wikipedia entry"
	}
	return kind
}

// Candidate is one search hit: enough to choose by, not enough to file.
type Candidate struct {
	Kind    string
	Source  string
	Id      string
	Title   string
	Year    string `json:",omitempty"`
	Creator string `json:",omitempty"`
	Thumb   string `json:",omitempty"` // a cover or poster url, small
	Blurb   string `json:",omitempty"` // one line about it, when the source has one
	Isbn    string `json:",omitempty"`
}

// Item is one thing in full, ready to file.
type Item struct {
	Kind        string
	Source      string
	Id          string
	Title       string
	Subtitle    string   `json:",omitempty"`
	Creators    []string `json:",omitempty"` // author, director, creator, developer, artist
	Cast        []string `json:",omitempty"`
	Date        string   `json:",omitempty"` // 1965, 1965-08 or 1965-08-01
	Publisher   string   `json:",omitempty"` // publisher, network, label
	Pages       int      `json:",omitempty"`
	Runtime     int      `json:",omitempty"` // minutes
	Seasons     int      `json:",omitempty"`
	Episodes    int      `json:",omitempty"`
	Platforms   []string `json:",omitempty"`
	Isbn        string   `json:",omitempty"` // 13 digits
	Genres      []string `json:",omitempty"`
	Description string   `json:",omitempty"`
	Cover       string   `json:",omitempty"` // a url
	Url         string   `json:",omitempty"` // the page about it at the source
	Tracks      []string `json:",omitempty"`
	Language    string   `json:",omitempty"`
}

// MediaID is what the MEDIA_ID property holds: source:id.
func (it *Item) MediaID() string { return it.Source + ":" + it.Id }

// Year is the first four digits of the date.
func (it *Item) Year() string { return yearOf(it.Date) }

var yearRe = regexp.MustCompile(`\b(1[0-9]{3}|20[0-9]{2})\b`)

func yearOf(s string) string {
	if m := yearRe.FindString(s); m != "" {
		return m
	}
	return ""
}

// ---------------------------------------------------------------------------
// The heading
// ---------------------------------------------------------------------------

// The properties every kind writes, in order. A field the yaml leaves out of
// a kind's fields: list is not written; MEDIA_ID and STATUS always are,
// because they are how orgs knows the heading again.
var kindFields = map[string][]string{
	Book:  {"AUTHOR", "SUBTITLE", "PUBLISHED_DATE", "PUBLISHER", "PAGES", "ISBN", "LANGUAGE", "URL"},
	Film:  {"DIRECTOR", "CAST", "RELEASE_DATE", "RUNTIME", "URL"},
	Show:  {"CREATOR", "CAST", "FIRST_AIRED_DATE", "NETWORK", "SEASONS", "EPISODES", "URL"},
	Game:  {"DEVELOPER", "PUBLISHER", "RELEASE_DATE", "PLATFORMS", "URL"},
	Album: {"ARTIST", "RELEASE_DATE", "LABEL", "TRACKS", "URL"},
	Other: {"URL"},
}

// FieldNames is every property a kind can write, for a settings page.
func FieldNames(kind string) []string {
	return append(append([]string{}, kindFields[kind]...), "IMAGE", "MEDIA_ID", "STATUS", "RATING")
}

// CreatorField is the property that names who made a kind of thing.
func CreatorField(kind string) string {
	if f := kindFields[kind]; len(f) > 0 && kind != Other {
		return f[0]
	}
	return ""
}

// DateField is the property that holds when a kind of thing came out.
func DateField(kind string) string {
	for _, f := range kindFields[kind] {
		if strings.HasSuffix(f, "_DATE") {
			return f
		}
	}
	return ""
}

func joinN(xs []string, n int) string {
	if len(xs) > n {
		xs = xs[:n]
	}
	return strings.Join(xs, ", ")
}

func itoa(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// Fields is the item as properties, in the order they are written. only, when
// given, is the yaml's list of which to keep (MEDIA_ID always stays).
func Fields(it *Item, only []string) (map[string]string, []string) {
	vals := map[string]string{}
	creators := joinN(it.Creators, 4)
	switch it.Kind {
	case Book:
		vals["AUTHOR"] = creators
		vals["SUBTITLE"] = it.Subtitle
		vals["PUBLISHED_DATE"] = it.Date
		vals["PUBLISHER"] = it.Publisher
		vals["PAGES"] = itoa(it.Pages)
		vals["ISBN"] = it.Isbn
		vals["LANGUAGE"] = it.Language
	case Film:
		vals["DIRECTOR"] = creators
		vals["CAST"] = joinN(it.Cast, 5)
		vals["RELEASE_DATE"] = it.Date
		if it.Runtime > 0 {
			vals["RUNTIME"] = fmt.Sprintf("%d min", it.Runtime)
		}
	case Show:
		vals["CREATOR"] = creators
		vals["CAST"] = joinN(it.Cast, 5)
		vals["FIRST_AIRED_DATE"] = it.Date
		vals["NETWORK"] = it.Publisher
		vals["SEASONS"] = itoa(it.Seasons)
		vals["EPISODES"] = itoa(it.Episodes)
	case Game:
		vals["DEVELOPER"] = creators
		vals["PUBLISHER"] = it.Publisher
		vals["RELEASE_DATE"] = it.Date
		vals["PLATFORMS"] = joinN(it.Platforms, 8)
	case Album:
		vals["ARTIST"] = creators
		vals["RELEASE_DATE"] = it.Date
		vals["LABEL"] = it.Publisher
		vals["TRACKS"] = itoa(len(it.Tracks))
	}
	vals["URL"] = it.Url
	vals["MEDIA_ID"] = it.MediaID()
	keep := map[string]bool{}
	for _, k := range only {
		keep[strings.ToUpper(strings.TrimSpace(k))] = true
	}
	order := []string{}
	out := map[string]string{}
	for _, k := range append(append([]string{}, kindFields[it.Kind]...), "MEDIA_ID") {
		v := strings.TrimSpace(vals[k])
		if v == "" || (len(keep) > 0 && !keep[k] && k != "MEDIA_ID") {
			continue
		}
		out[k] = oneLine(v)
		order = append(order, k)
	}
	return out, order
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Tags are the item's genres as org tags: "Science Fiction" is
// science_fiction. Catalogue noise ("nyt:hardcover=2008", "Fiction, science
// fiction, general", "Accessible book") is left out; at most max.
func Tags(it *Item, max int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, g := range it.Genres {
		// Wikidata's genres say what they are a genre of: "drama film",
		// "science fiction television series".
		low := strings.ToLower(g)
		for _, suf := range []string{" film", " television series", " television program", " video game", " music", " novel"} {
			if strings.HasSuffix(low, suf) && len(g) > len(suf) {
				g = g[:len(g)-len(suf)]
				break
			}
		}
		t := TagOf(g)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) >= max {
			break
		}
	}
	return out
}

var noiseSubjects = map[string]bool{"accessible_book": true, "protected_daisy": true, "in_library": true,
	"lending_library": true, "large_type_books": true, "new_york_times_bestseller": true,
	"new_york_times_reviewed": true, "open_library_staff_picks": true, "overdrive": true, "general": true}

// TagOf is one genre as an org tag, or "" when it should not be one.
func TagOf(g string) string {
	g = strings.TrimSpace(g)
	if g == "" || len(g) > 32 || strings.ContainsAny(g, ":=,()/[]") {
		return ""
	}
	var b strings.Builder
	under := false
	for _, r := range strings.ToLower(g) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			under = false
		case !under && b.Len() > 0:
			b.WriteByte('_')
			under = true
		}
	}
	t := strings.Trim(b.String(), "_")
	if t == "" || noiseSubjects[t] || strings.Count(t, "_") > 2 {
		return ""
	}
	return t
}

// Body is the text under the heading: the description in paragraphs, and an
// album's tracks as a numbered list.
func Body(it *Item) string {
	parts := []string{}
	if d := CleanText(it.Description); d != "" {
		parts = append(parts, d)
	}
	if len(it.Tracks) > 0 {
		ls := []string{}
		for i, t := range it.Tracks {
			ls = append(ls, fmt.Sprintf("%d. %s", i+1, t))
		}
		parts = append(parts, strings.Join(ls, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

var (
	tagRe        = regexp.MustCompile(`(?s)<[^>]*>`)
	brRe         = regexp.MustCompile(`(?i)<br\s*/?>|</p>\s*<p[^>]*>|</p>`)
	mdRefLinkRe  = regexp.MustCompile(`\[([^\]]+)\]\[\d+\]`)
	mdLinkRe     = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)]+)\)`)
	mdRefDefRe   = regexp.MustCompile(`(?m)^\s*\[\d+\]:\s*\S+\s*$`)
	mdSourceRe   = regexp.MustCompile(`\(\s*\[?[Ss]ource\]?[^)]*\)`)
	blankLinesRe = regexp.MustCompile(`\n\s*\n+`)
)

// CleanText is a description as plain paragraphs: html tags and entities
// gone, Open Library's markdown links and "----" trailers dropped, every
// paragraph on one line, and nothing left that org would read as markup at
// the start of a line.
func CleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if i := strings.Index(s, "\n----"); i >= 0 {
		s = s[:i]
	}
	s = brRe.ReplaceAllString(s, "\n\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = mdRefDefRe.ReplaceAllString(s, "")
	s = mdLinkRe.ReplaceAllString(s, "[[$2][$1]]")
	s = mdRefLinkRe.ReplaceAllString(s, "$1")
	s = mdSourceRe.ReplaceAllString(s, "")
	paras := []string{}
	for _, p := range blankLinesRe.Split(strings.TrimSpace(s), -1) {
		p = oneLine(p)
		if p == "" {
			continue
		}
		// Org markup at the start of a line - a heading, a keyword, a
		// drawer - written as text.
		if strings.HasPrefix(p, "*") || strings.HasPrefix(p, "#+") || (strings.HasPrefix(p, ":") && strings.HasSuffix(p, ":")) {
			p = "," + p
		}
		paras = append(paras, p)
	}
	return strings.Join(paras, "\n\n")
}

// ---------------------------------------------------------------------------
// Status and rating
// ---------------------------------------------------------------------------

// The four shelves: want, doing, done, dropped.
const (
	Want    = "want"
	Doing   = "doing"
	Done    = "done"
	Dropped = "dropped"
)

// Shelves is every shelf in order.
var Shelves = []string{Want, Doing, Done, Dropped}

var statusWords = map[string][4]string{
	Book:  {"to read", "reading", "read", "set aside"},
	Film:  {"to watch", "watching", "watched", "set aside"},
	Show:  {"to watch", "watching", "watched", "set aside"},
	Game:  {"to play", "playing", "played", "set aside"},
	Album: {"to listen", "listening", "listened", "set aside"},
	Other: {"to see", "in progress", "done", "set aside"},
}

// StatusWord is what a shelf is called for a kind: a book's done is "read".
func StatusWord(kind, shelf string) string {
	w, ok := statusWords[kind]
	if !ok {
		w = statusWords[Other]
	}
	for i, s := range Shelves {
		if s == shelf {
			return w[i]
		}
	}
	return ""
}

var shelfWords = map[string]string{
	"want": Want, "to read": Want, "to watch": Want, "to play": Want, "to listen": Want, "to see": Want,
	"todo": Want, "wishlist": Want, "wish": Want, "planned": Want, "plan to watch": Want, "backlog": Want, "unread": Want,
	"doing": Doing, "reading": Doing, "watching": Doing, "playing": Doing, "listening": Doing, "in progress": Doing,
	"started": Doing, "current": Doing, "currently reading": Doing, "next": Doing, "active": Doing,
	"done": Done, "read": Done, "watched": Done, "played": Done, "listened": Done, "seen": Done, "finished": Done,
	"completed": Done, "complete": Done, "beaten": Done,
	"dropped": Dropped, "set aside": Dropped, "abandoned": Dropped, "dnf": Dropped, "did not finish": Dropped,
	"on hold": Dropped, "paused": Dropped, "cancelled": Dropped, "canceled": Dropped,
}

// ShelfOfWord is the shelf a status word means ("watched" is done), or "".
func ShelfOfWord(w string) string {
	w = strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(w, "_", " ")), " "))
	if s, ok := shelfWords[w]; ok {
		return s
	}
	if s, ok := shelfWords[strings.ReplaceAll(w, "-", " ")]; ok {
		return s
	}
	return ""
}

// ShelfOf decides which shelf a heading is on. The STATUS property wins;
// else the TODO keyword (a done keyword is done, TODO is want, any other
// active one - READING, NEXT, STARTED - is doing); else a FINISHED_DATE
// means done and a STARTED_DATE doing; else want.
func ShelfOf(status, keyword string, keywordDone bool, started, finished string) string {
	if s := ShelfOfWord(status); s != "" {
		return s
	}
	if keyword != "" {
		word := ShelfOfWord(keyword)
		switch {
		case keywordDone && word == Dropped:
			return Dropped
		case keywordDone:
			return Done
		case strings.EqualFold(keyword, "TODO") || word == Want:
			return Want
		default:
			return Doing
		}
	}
	if strings.TrimSpace(finished) != "" {
		return Done
	}
	if strings.TrimSpace(started) != "" {
		return Doing
	}
	return Want
}

var ratingRe = regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?)\s*(?:/\s*([0-9]+))?`)

// ParseRating reads a RATING: 4, 4.5, 8/10 (as 4), ★★★★ or ****. Out of 5;
// 0 is unrated.
func ParseRating(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n := strings.Count(s, "★") + strings.Count(s, "*"); n > 0 && strings.Trim(s, "★☆* ") == "" {
		if n > 5 {
			n = 5
		}
		return float64(n)
	}
	m := ratingRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	if m[2] != "" {
		if of, _ := strconv.ParseFloat(m[2], 64); of > 0 {
			v = v * 5 / of
		}
	}
	if v < 0 {
		v = 0
	}
	if v > 5 {
		v = 5
	}
	return float64(int(v*2+0.5)) / 2
}

// RatingText is how a rating is written back: "4", "3.5", "" for none.
func RatingText(v float64) string {
	if v <= 0 {
		return ""
	}
	if v > 5 {
		v = 5
	}
	v = float64(int(v*2+0.5)) / 2
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ---------------------------------------------------------------------------
// ISBNs (worg's library.ts has the same rule for the barcode scanner)
// ---------------------------------------------------------------------------

// NormISBN is an ISBN-10 or ISBN-13 as its 13 digits, if the check digit
// holds. Hyphens and spaces are ignored; "ISBN" in front is allowed.
func NormISBN(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.TrimPrefix(strings.TrimPrefix(s, "ISBN-13"), "ISBN-10")
	s = strings.TrimPrefix(s, "ISBN")
	s = strings.TrimLeft(s, ": ")
	d := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == 'X' {
			return r
		}
		if r == '-' || r == ' ' {
			return -1
		}
		return '?'
	}, s)
	switch len(d) {
	case 10:
		sum := 0
		for i := 0; i < 10; i++ {
			c := d[i]
			v := int(c - '0')
			if c == 'X' {
				if i != 9 {
					return "", false
				}
				v = 10
			} else if c < '0' || c > '9' {
				return "", false
			}
			sum += v * (10 - i)
		}
		if sum%11 != 0 {
			return "", false
		}
		return isbn13("978" + d[:9]), true
	case 13:
		if strings.ContainsAny(d, "X?") || (!strings.HasPrefix(d, "978") && !strings.HasPrefix(d, "979")) {
			return "", false
		}
		if isbn13(d[:12]) != d {
			return "", false
		}
		return d, true
	}
	return "", false
}

// isbn13 adds the check digit to twelve digits.
func isbn13(twelve string) string {
	sum := 0
	for i := 0; i < 12; i++ {
		v := int(twelve[i] - '0')
		if i%2 == 1 {
			v *= 3
		}
		sum += v
	}
	return twelve + strconv.Itoa((10-sum%10)%10)
}

// ---------------------------------------------------------------------------
// Small helpers the sources share
// ---------------------------------------------------------------------------

func uniq(xs []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, x := range xs {
		x = strings.TrimSpace(x)
		k := strings.ToLower(x)
		if x == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, x)
	}
	return out
}

func first(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return strings.TrimSpace(x)
		}
	}
	return ""
}

// sortedKeys is for deterministic output from maps.
func sortedKeys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
