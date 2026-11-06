package orgs

// The library (features.mediaDb):
// look a book, film, show, game or album up in a public catalogue and file it
// as a record heading - :RECORD: book, the fields named so records' fieldOf
// types them, genres as tags, the description as the body and the cover
// saved in images/<kind>/ beside the org file. The catalogues and the rules
// for turning their answers into fields are internal/common/mediadb (pure,
// pinned by saved answers); this file is the network, the shelves and the
// writes.
//
// Which shelf a heading is on (to read / reading / read / set aside) is the
// STATUS property, else its TODO keyword, else STARTED_DATE / FINISHED_DATE
// (mediadb.ShelfOf). Clients are told the shelf, so they have no twin.

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/mediadb"
)

const (
	mediadbJsonCap  = 4 << 20
	mediadbImageCap = 10 << 20
	mediadbCacheFor = time.Hour
	mediadbDefFile  = "library.org"
)

func registerMediadbRoutes(api *mux.Router) {
	api.HandleFunc("/mediadb/search", RequestMediadbSearch).Methods("GET")
	api.HandleFunc("/mediadb/details", RequestMediadbDetails).Methods("GET")
	api.HandleFunc("/mediadb/add", PostMediadbAdd).Methods("POST")
	api.HandleFunc("/mediadb/items", RequestMediadbItems).Methods("GET")
	api.HandleFunc("/mediadb/refresh", PostMediadbRefresh).Methods("POST")
	api.HandleFunc("/mediadb/set", PostMediadbSet).Methods("POST")
	api.HandleFunc("/mediadb/sources", RequestMediadbSources).Methods("GET")
}

func mediadbSettings() common.MediaDbSettings {
	if s := Conf().Server; s != nil {
		return s.MediaDb
	}
	return common.MediaDbSettings{}
}

func mediadbKindSettings(kind string) common.MediaDbKind {
	for k, v := range mediadbSettings().Kinds {
		if mediadb.KindOf(k) == kind {
			return v
		}
	}
	return common.MediaDbKind{}
}

// mediadbConfig is the sources' view of the yaml, every key read (env:,
// keyring: or as written).
func mediadbConfig() *mediadb.Config {
	s := mediadbSettings()
	c := &mediadb.Config{Keys: map[string]string{}, Urls: s.Urls, Language: s.Language}
	for k, v := range s.Keys {
		if v = strings.TrimSpace(hookSecret(strings.TrimSpace(v))); v != "" {
			c.Keys[k] = v
		}
	}
	return c
}

func mediadbUserAgent() string {
	v := common.Version
	if v == "" {
		v = "dev"
	}
	ua := "orgs/" + v + " (https://github.com/ihdavids/orgs"
	if c := strings.TrimSpace(mediadbSettings().Contact); c != "" {
		ua += "; " + c
	}
	return ua + ")"
}

// ---------------------------------------------------------------------------
// The network: one client, a cache, MusicBrainz's one request a second
// ---------------------------------------------------------------------------

var mediadbClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 8 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("redirected to a %s url", req.URL.Scheme)
		}
		return nil
	},
}

// How long to leave between requests to a host, as its usage policy asks.
var mediadbHostGap = map[string]time.Duration{"musicbrainz.org": 1100 * time.Millisecond}

var (
	mediadbGapMu   sync.Mutex
	mediadbLastHit = map[string]time.Time{}
	mediadbCacheMu sync.Mutex
	mediadbCache   = map[string]mediadbCached{}
)

type mediadbCached struct {
	body []byte
	at   time.Time
}

func mediadbWait(host string) {
	gap := mediadbHostGap[strings.TrimPrefix(host, "www.")]
	if gap == 0 {
		return
	}
	mediadbGapMu.Lock()
	defer mediadbGapMu.Unlock()
	if d := time.Until(mediadbLastHit[host].Add(gap)); d > 0 {
		time.Sleep(d)
	}
	mediadbLastHit[host] = time.Now()
}

func mediadbDo(r mediadb.Request, limit int64) ([]byte, string, error) {
	u, err := url.Parse(r.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, "", fmt.Errorf("only http and https can be asked, not %q", r.URL)
	}
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if r.Body != "" {
		body = strings.NewReader(r.Body)
	}
	req, err := http.NewRequest(method, r.URL, body)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", mediadbUserAgent())
	req.Header.Set("Accept", "application/json")
	for k, v := range r.Header {
		req.Header.Set(k, v)
	}
	mediadbWait(u.Host)
	resp, err := mediadbClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("could not reach %s: %v", u.Host, err)
	}
	// MusicBrainz says 503 when it is asked too often: once more, after a
	// pause, as its policy asks.
	if resp.StatusCode == http.StatusServiceUnavailable && mediadbHostGap[strings.TrimPrefix(u.Host, "www.")] > 0 && r.Body == "" {
		resp.Body.Close()
		time.Sleep(2 * time.Second)
		if req, err = http.NewRequest(method, r.URL, nil); err != nil {
			return nil, "", err
		}
		req.Header.Set("User-Agent", mediadbUserAgent())
		req.Header.Set("Accept", "application/json")
		for k, v := range r.Header {
			req.Header.Set(k, v)
		}
		mediadbWait(u.Host)
		if resp, err = mediadbClient.Do(req); err != nil {
			return nil, "", fmt.Errorf("could not reach %s: %v", u.Host, err)
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", mediadb.NotFound{URL: r.URL}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := resp.Status
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			msg += " - is the key right?"
		}
		if resp.StatusCode == 429 {
			msg += " - asked too often; try again in a while"
		}
		return nil, "", fmt.Errorf("%s answered %s", u.Host, msg)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > limit {
		return nil, "", fmt.Errorf("%s sent more than %d MB", u.Host, limit>>20)
	}
	return b, resp.Header.Get("Content-Type"), nil
}

// mediadbGet is the Getter the sources are given: a GET answered within the
// hour comes from memory, so paging through results is not asking again.
func mediadbGet(r mediadb.Request) ([]byte, error) {
	key := ""
	if r.Method == "" || r.Method == http.MethodGet {
		key = r.URL + "\x00" + r.Header["Authorization"]
		mediadbCacheMu.Lock()
		if c, ok := mediadbCache[key]; ok && time.Since(c.at) < mediadbCacheFor {
			mediadbCacheMu.Unlock()
			return c.body, nil
		}
		mediadbCacheMu.Unlock()
	}
	b, _, err := mediadbDo(r, mediadbJsonCap)
	if err != nil || key == "" {
		return b, err
	}
	mediadbCacheMu.Lock()
	if len(mediadbCache) > 400 {
		for k, c := range mediadbCache {
			if time.Since(c.at) > mediadbCacheFor/4 {
				delete(mediadbCache, k)
			}
		}
	}
	mediadbCache[key] = mediadbCached{b, time.Now()}
	mediadbCacheMu.Unlock()
	return b, nil
}

// mediadbGetter is swapped by the tests.
var mediadbGetter mediadb.Getter = mediadbGet

var mediadbImageExt = map[string]string{"image/jpeg": "jpg", "image/jpg": "jpg", "image/png": "png", "image/gif": "gif", "image/webp": "webp", "image/avif": "avif"}

func mediadbPicture(src string) ([]byte, string, error) {
	b, ct, err := mediadbDo(mediadb.Request{URL: src, Header: map[string]string{"Accept": "image/avif,image/webp,image/png,image/jpeg,image/*;q=0.8"}}, mediadbImageCap)
	if err != nil {
		return nil, "", err
	}
	mt, _, _ := mime.ParseMediaType(ct)
	ext := mediadbImageExt[strings.ToLower(mt)]
	if ext == "" {
		ext = mediadbImageExt[strings.SplitN(http.DetectContentType(b), ";", 2)[0]]
	}
	if ext == "" {
		return nil, "", fmt.Errorf("the cover is not a picture (%s)", ct)
	}
	return b, ext, nil
}

// mediadbSavePicture writes a cover to images/<kind>/ beside the org file
// and answers with the relative link; the same bytes already there are
// reused.
func mediadbSavePicture(orgFile, kind, name string, data []byte, ext string) (string, error) {
	rel := "images/" + kind
	dir := filepath.Join(filepath.Dir(orgFile), filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	slug := photoSlug(name)
	for n := 1; n < 1000; n++ {
		fname := slug + "." + ext
		if n > 1 {
			fname = fmt.Sprintf("%s-%d.%s", slug, n, ext)
		}
		path := filepath.Join(dir, fname)
		if have, err := os.ReadFile(path); err == nil {
			if samePhoto(have, data) {
				return "[[file:" + rel + "/" + fname + "]]", nil
			}
			continue
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return "", err
		}
		return "[[file:" + rel + "/" + fname + "]]", nil
	}
	return "", fmt.Errorf("too many covers called %s", slug)
}

// ---------------------------------------------------------------------------
// Searching
// ---------------------------------------------------------------------------

// MediadbHit is a search hit, and the heading it already is, if any.
type MediadbHit struct {
	mediadb.Candidate
	Have string `json:",omitempty"` // the hash of the heading it already is
}

// MediadbTried is a source that was asked, or skipped, and why.
type MediadbTried struct {
	Source string
	Label  string
	Msg    string
}

type mediadbSearchResult struct {
	Ok          bool
	Msg         string `json:",omitempty"`
	Kind        string
	Source      string `json:",omitempty"` // the one that answered
	Attribution string `json:",omitempty"`
	Candidates  []MediadbHit
	Tried       []MediadbTried `json:",omitempty"`
}

func mediadbKind(word string) (string, error) {
	if strings.TrimSpace(word) == "" {
		return mediadb.Book, nil
	}
	k := mediadb.KindOf(word)
	if k == "" {
		return "", fmt.Errorf("%q is not a kind (book, film, show, game, album, other)", word)
	}
	return k, nil
}

// mediadbSearch asks the kind's sources in order until one answers with
// something: one that needs a key it was not given is skipped, one that
// cannot be reached is passed over for the next.
func mediadbSearch(kind, q, only string) mediadbSearchResult {
	res := mediadbSearchResult{Kind: kind, Candidates: []MediadbHit{}}
	c := mediadbConfig()
	srcs := mediadb.SourcesFor(kind, mediadbKindSettings(kind).Sources)
	if only != "" {
		s := mediadb.SourceNamed(only)
		if s == nil || !s.Serves(kind) {
			res.Msg = fmt.Sprintf("%s is not a source for a %s", only, mediadb.KindLabel(kind))
			return res
		}
		srcs = []*mediadb.Source{s}
	}
	have := mediadbHaveIndex()
	for _, s := range srcs {
		if m := s.Missing(c); len(m) > 0 {
			res.Tried = append(res.Tried, MediadbTried{s.Name, s.Label, "needs mediaDb.keys." + strings.Join(m, " and ")})
			continue
		}
		hits, err := s.Search(c, mediadbGetter, kind, q)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mediaDb: %s: %v\n", s.Name, err)
			res.Tried = append(res.Tried, MediadbTried{s.Name, s.Label, err.Error()})
			continue
		}
		res.Tried = append(res.Tried, MediadbTried{s.Name, s.Label, fmt.Sprintf("%d found", len(hits))})
		if len(hits) == 0 {
			continue
		}
		res.Ok, res.Source, res.Attribution = true, s.Name, s.Attribution
		for _, h := range hits {
			hit := MediadbHit{Candidate: h}
			hit.Have = have[h.Source+":"+h.Id]
			if hit.Have == "" && h.Isbn != "" {
				hit.Have = have["isbn:"+h.Isbn]
			}
			if n, ok := mediadb.NormISBN(h.Id); ok && hit.Have == "" {
				hit.Have = have["isbn:"+n]
			}
			res.Candidates = append(res.Candidates, hit)
		}
		return res
	}
	if len(res.Tried) == 0 {
		res.Msg = "no source looks up a " + mediadb.KindLabel(kind)
		return res
	}
	// Nothing found is an answer; every source failing is not.
	msgs := []string{}
	for _, t := range res.Tried {
		if !strings.HasSuffix(t.Msg, " found") {
			msgs = append(msgs, t.Label+": "+t.Msg)
		}
	}
	for _, t := range res.Tried {
		if strings.HasSuffix(t.Msg, " found") {
			res.Ok = true
			res.Msg = "nothing found for " + strconv.Quote(q)
			if len(msgs) > 0 {
				res.Msg += " (" + strings.Join(msgs, "; ") + ")"
			}
			return res
		}
	}
	res.Msg = strings.Join(msgs, "; ")
	return res
}

func mediadbDetails(kind, source, id string) (*mediadb.Item, error) {
	if source == "" {
		// An ISBN for a book is Open Library's to answer by default.
		for _, s := range mediadb.SourcesFor(kind, mediadbKindSettings(kind).Sources) {
			if s.Usable(mediadbConfig()) {
				source = s.Name
				break
			}
		}
	}
	s := mediadb.SourceNamed(source)
	if s == nil || !s.Serves(kind) {
		return nil, fmt.Errorf("%q is not a source for a %s", source, mediadb.KindLabel(kind))
	}
	return s.Details(mediadbConfig(), mediadbGetter, kind, id)
}

// ---------------------------------------------------------------------------
// What is on the shelves
// ---------------------------------------------------------------------------

// MediadbItem is one heading in the library.
type MediadbItem struct {
	Hash       string
	Filename   string
	Line       int
	Kind       string
	Title      string
	Creator    string   `json:",omitempty"`
	Date       string   `json:",omitempty"`
	Year       string   `json:",omitempty"`
	Shelf      string   // want, doing, done, dropped
	StatusWord string   // what the shelf is called for this kind: "reading"
	Rating     float64  `json:",omitempty"`
	Cover      string   `json:",omitempty"` // a path or url a client can show
	Tags       []string `json:",omitempty"`
	MediaId    string   `json:",omitempty"`
	Isbn       string   `json:",omitempty"`
	Url        string   `json:",omitempty"`
	Keyword    string   `json:",omitempty"`
	Started    string   `json:",omitempty"`
	Finished   string   `json:",omitempty"`
	Added      string   `json:",omitempty"`
	Fields     map[string]string
	About      string `json:",omitempty"` // the heading's own text, as written
}

func mediadbProp(props map[string]string, names ...string) string {
	for _, n := range names {
		for k, v := range props {
			if strings.EqualFold(k, n) && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func mediadbRead(f *common.OrgFile, sec *org.Section, kind string) *MediadbItem {
	_, props := sectionProps(sec)
	it := &MediadbItem{Hash: sec.Hash, Filename: f.Filename, Line: sec.Headline.Pos.Row, Kind: kind, Title: sectionTitle(sec),
		Tags: sec.Headline.Tags, Fields: props, Keyword: sec.Headline.Status}
	if it.Tags == nil {
		it.Tags = []string{}
	}
	it.Creator = mediadbProp(props, mediadb.CreatorField(kind), "AUTHOR", "DIRECTOR", "ARTIST", "CREATOR", "DEVELOPER")
	it.Date = mediadbProp(props, mediadb.DateField(kind), "PUBLISHED_DATE", "RELEASE_DATE", "FIRST_AIRED_DATE", "YEAR", "DATE")
	if m := mediadbYear(it.Date); m != "" {
		it.Year = m
	}
	it.Rating = mediadb.ParseRating(mediadbProp(props, "RATING", "STARS", "SCORE"))
	if c := mediadbProp(props, "IMAGE", "COVER", "POSTER"); c != "" {
		it.Cover = fieldLink("image", "IMAGE", c, f.Filename)
		if it.Cover == "" && (strings.HasPrefix(c, "http://") || strings.HasPrefix(c, "https://")) {
			it.Cover = c
		}
	}
	it.MediaId = mediadbProp(props, "MEDIA_ID")
	if n, ok := mediadb.NormISBN(mediadbProp(props, "ISBN", "ISBN_13", "ISBN13", "ISBN_10", "ISBN10")); ok {
		it.Isbn = n
	}
	it.Url = webURL(mediadbProp(props, "URL"))
	it.Started = mediadbProp(props, "STARTED_DATE", "STARTED")
	it.Finished = mediadbProp(props, "FINISHED_DATE", "FINISHED", "WATCHED_DATE", "READ_DATE")
	it.Added = mediadbProp(props, "ADDED")
	done := false
	if it.Keyword != "" {
		_, done, _ = keywordInfo(f, it.Keyword)
	}
	it.Shelf = mediadb.ShelfOf(mediadbProp(props, "STATUS"), it.Keyword, done, it.Started, it.Finished)
	it.StatusWord = mediadb.StatusWord(kind, it.Shelf)
	return it
}

func mediadbYear(s string) string {
	for i := 0; i+4 <= len(s); i++ {
		y := s[i : i+4]
		if _, err := strconv.Atoi(y); err == nil && (y[0] == '1' || y[0] == '2') {
			return y
		}
	}
	return ""
}

// mediadbAbout is a heading's own text - not its drawers, not its children.
func mediadbAbout(it *MediadbItem) string {
	sec := GetDb().FindByHash(it.Hash)
	if sec == nil {
		return ""
	}
	lines, from, to, ok := recordLines(it.Filename, sec)
	if !ok {
		return ""
	}
	end := ownLinesEnd(lines, from, to)
	s := recordNotes(lines, from, end)
	if len(s) > 8000 {
		s = s[:8000] + "…"
	}
	return s
}

var (
	mediadbCacheAllMu sync.Mutex
	mediadbCacheAt    uint64
	mediadbCacheDb    *OrgDb
	mediadbCacheAll   []*MediadbItem
)

// mediadbAll is every record of a library kind, cached on ReloadIndex (see
// Traps: caches).
func mediadbAll() []*MediadbItem {
	mediadbCacheAllMu.Lock()
	defer mediadbCacheAllMu.Unlock()
	db := GetDb()
	if mediadbCacheAll != nil && mediadbCacheDb == db && mediadbCacheAt == db.ReloadIndex {
		return mediadbCacheAll
	}
	at := db.ReloadIndex
	out := []*MediadbItem{}
	walkSections(func(f *common.OrgFile, sec *org.Section) {
		if isArchiveFile(f.Filename) || !isRealHeading(sec) {
			return
		}
		rt := strings.ToLower(RecordTypeOf(sec))
		kind := ""
		for _, k := range mediadb.Kinds {
			if rt == k {
				kind = k
			}
		}
		if kind == "" {
			return
		}
		out = append(out, mediadbRead(f, sec, kind))
	})
	mediadbCacheAll, mediadbCacheAt, mediadbCacheDb = out, at, db
	return out
}

// mediadbHaveIndex maps source:id and isbn:N to the heading that is it.
func mediadbHaveIndex() map[string]string {
	out := map[string]string{}
	for _, it := range mediadbAll() {
		if it.MediaId != "" {
			out[it.MediaId] = it.Hash
		}
		if it.Isbn != "" {
			out["isbn:"+it.Isbn] = it.Hash
		}
	}
	return out
}

func mediadbFind(hash string) (*MediadbItem, error) {
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		return nil, fmt.Errorf("no heading with that hash - it may have moved; refresh and try again")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		return nil, fmt.Errorf("could not find the file that heading is in")
	}
	kind := mediadb.KindOf(RecordTypeOf(sec))
	if kind == "" {
		return nil, fmt.Errorf("%q is not in the library (it needs :RECORD: book, film, show, game, album or other)", sectionTitle(sec))
	}
	return mediadbRead(f, sec, kind), nil
}

// ---------------------------------------------------------------------------
// Where a new one goes
// ---------------------------------------------------------------------------

var mediadbShelfNames = map[string]string{mediadb.Book: "Books", mediadb.Film: "Films", mediadb.Show: "TV shows",
	mediadb.Game: "Games", mediadb.Album: "Albums", mediadb.Other: "Things"}
var mediadbIcons = map[string]string{mediadb.Book: "📚", mediadb.Film: "🎬", mediadb.Show: "📺", mediadb.Game: "🎮", mediadb.Album: "💿", mediadb.Other: "🔖"}

func mediadbFileByName(file string) (*common.OrgFile, error) {
	if f := GetDb().FindByFile(file); f != nil {
		return f, nil
	}
	return nil, fmt.Errorf("no file called %q", file)
}

// mediadbUnder is a heading in a file by title, outline path or hash.
func mediadbUnder(file, under string) (importTargetAt, error) {
	if sec := GetDb().FindByHash(under); sec != nil && sec.Headline != nil {
		return importTarget("", under, "")
	}
	if file == "" {
		hits := []*org.Section{}
		walkSections(func(f *common.OrgFile, sec *org.Section) {
			if !isArchiveFile(f.Filename) && strings.EqualFold(sectionTitle(sec), under) {
				hits = append(hits, sec)
			}
		})
		if len(hits) == 1 {
			return importTarget("", hits[0].Hash, "")
		}
		if len(hits) > 1 {
			return importTargetAt{}, fmt.Errorf("%d headings are called %q - say which file", len(hits), under)
		}
		return importTargetAt{}, fmt.Errorf("no heading called %q", under)
	}
	f, err := mediadbFileByName(file)
	if err != nil {
		return importTargetAt{}, err
	}
	_, sec := GetDb().GetFromTarget(&common.Target{Type: "file+olp", Filename: f.Filename, Id: under}, false)
	if sec == nil || sec.Headline == nil {
		_, sec = GetDb().GetFromTarget(&common.Target{Type: "file+headline", Filename: f.Filename, Id: under}, false)
	}
	if sec == nil || sec.Headline == nil {
		return importTargetAt{}, fmt.Errorf("no heading %q in %s", under, filepath.Base(f.Filename))
	}
	return importTarget("", sec.Hash, "")
}

// mediadbTarget is where a new one of a kind goes: the file and heading
// asked for; else the kind's file/under in the yaml; else its capture
// template's target; else the kind's collection; else a new collection
// heading (Books, Films...) at the end of library.org, made if missing.
func mediadbTarget(kind, file, under string) (importTargetAt, error) {
	file, under = strings.TrimSpace(file), strings.TrimSpace(under)
	ks := mediadbKindSettings(kind)
	if file == "" && under == "" {
		file, under = strings.TrimSpace(ks.File), strings.TrimSpace(ks.Under)
	}
	if file == "" && under == "" && ks.Capture != "" {
		ct := FindCaptureTemplate(ks.Capture, "")
		if ct == nil {
			return importTargetAt{}, fmt.Errorf("no capture template called %q (mediaDb.kinds.%s.capture)", ks.Capture, kind)
		}
		t := ct.CapTarget
		if strings.TrimSpace(t.Id) != "" {
			_, sec := GetDb().GetFromTarget(&t, false)
			if sec == nil || sec.Headline == nil {
				return importTargetAt{}, fmt.Errorf("the %q capture template's target is not there", ks.Capture)
			}
			return importTarget("", sec.Hash, "")
		}
		file = t.Filename
	}
	if under != "" {
		return mediadbUnder(file, under)
	}
	if file != "" {
		if f := GetDb().FindByFile(file); f == nil {
			if err := mediadbMakeFile(file); err != nil {
				return importTargetAt{}, err
			}
		}
		f, err := mediadbFileByName(file)
		if err != nil {
			return importTargetAt{}, err
		}
		return importTarget("", "", f.Filename)
	}
	for _, c := range allCollections() {
		if strings.EqualFold(c.Type, kind) && c.Hash != "" {
			return importTarget(kind, "", "")
		}
	}
	// The first of a kind: a collection for it, so the records browser has a
	// tab for it too.
	if f := GetDb().FindByFile(mediadbDefFile); f == nil {
		if err := mediadbMakeFile(mediadbDefFile); err != nil {
			return importTargetAt{}, err
		}
	}
	f, err := mediadbFileByName(mediadbDefFile)
	if err != nil {
		return importTargetAt{}, err
	}
	if _, err := AddCollection(&common.RecordCollectionNew{Type: kind, Name: mediadbShelfNames[kind], Icon: mediadbIcons[kind],
		Fields: mediadb.FieldNames(kind), Filename: f.Filename}); err != nil {
		return importTargetAt{}, err
	}
	return importTarget(kind, "", "")
}

func mediadbMakeFile(file string) error {
	path := file
	if !filepath.IsAbs(path) && Conf().Server != nil && len(Conf().Server.OrgDirs) > 0 {
		path = filepath.Join(Conf().Server.OrgDirs[0], file)
	}
	if !strings.HasSuffix(path, ".org") {
		return fmt.Errorf("%q is not an org file", file)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte("#+TITLE: Library\n"), 0644); err != nil {
			return err
		}
	}
	GetDb().LoadFile(path, true)
	return nil
}

// ---------------------------------------------------------------------------
// Writing one
// ---------------------------------------------------------------------------

// MediadbAddRequest is POST /mediadb/add.
type MediadbAddRequest struct {
	Kind    string
	Source  string
	Id      string
	File    string
	Under   string
	Force   bool
	NoCover bool
	Status  string  // a shelf or a status word; "" is the kind's starting one
	Rating  float64 // 0: none
	Preview bool    // answer with the heading, write nothing
	Item    *mediadb.Item
}

// MediadbAddResult is what /mediadb/add wrote.
type MediadbAddResult struct {
	Ok       bool
	Msg      string `json:",omitempty"`
	Hash     string `json:",omitempty"`
	Filename string `json:",omitempty"`
	Line     int
	Existing string        `json:",omitempty"`
	Item     *mediadb.Item `json:",omitempty"`
	Org      string        `json:",omitempty"`
	Warnings []string      `json:",omitempty"`
}

// mediadbHeading is the item's fields, tags and body as recordHeadingLines
// wants them.
func mediadbHeading(it *mediadb.Item, kind string, status string, rating float64) (map[string]string, []string, []string, string) {
	ks := mediadbKindSettings(kind)
	fields, order := mediadb.Fields(it, ks.Fields)
	if it.Cover != "" {
		fields["IMAGE"] = "[[" + it.Cover + "]]"
		order = append(order, "IMAGE")
	}
	shelf := mediadb.ShelfOfWord(status)
	if shelf == "" {
		shelf = mediadb.ShelfOfWord(ks.Status)
	}
	if shelf == "" {
		shelf = mediadb.Want
	}
	fields["STATUS"] = mediadb.StatusWord(kind, shelf)
	order = append(order, "STATUS")
	now := orgStamp(time.Now())
	if shelf == mediadb.Done {
		fields["FINISHED_DATE"] = now
		order = append(order, "FINISHED_DATE")
	}
	if r := mediadb.RatingText(rating); r != "" {
		fields["RATING"] = r
		order = append(order, "RATING")
	}
	if shelf == mediadb.Doing {
		fields["STARTED_DATE"] = now
		order = append(order, "STARTED_DATE")
	}
	tags := []string{}
	s := mediadbSettings()
	if !s.NoTags {
		max := s.MaxTags
		if max <= 0 {
			max = 5
		}
		tags = mediadb.Tags(it, max)
	}
	title := it.Title
	return fields, order, tags, title
}

func mediadbAdd(req *MediadbAddRequest) (MediadbAddResult, error) {
	kind, err := mediadbKind(req.Kind)
	if err != nil {
		return MediadbAddResult{}, err
	}
	it := req.Item
	if it == nil {
		id := strings.TrimSpace(req.Id)
		source := req.Source
		if n, ok := mediadb.NormISBN(id); ok && kind == mediadb.Book {
			id = n
		}
		if it, err = mediadbDetails(kind, source, id); err != nil {
			if _, nf := err.(mediadb.NotFound); nf {
				return MediadbAddResult{}, fmt.Errorf("%s has no %s %q", mediadbFirst(source, "the catalogue"), mediadb.KindLabel(kind), req.Id)
			}
			return MediadbAddResult{}, err
		}
	}
	it.Kind = kind
	if strings.TrimSpace(it.Title) == "" {
		return MediadbAddResult{}, fmt.Errorf("nothing to add: no title")
	}
	if !req.Force {
		have := mediadbHaveIndex()
		h := have[it.MediaID()]
		if h == "" && it.Isbn != "" {
			h = have["isbn:"+it.Isbn]
		}
		if h != "" {
			for _, x := range mediadbAll() {
				if x.Hash == h {
					return MediadbAddResult{Msg: fmt.Sprintf("%q is already in %s (line %d) - add it again with Force", x.Title, filepath.Base(x.Filename), x.Line+1),
						Existing: h, Filename: x.Filename, Line: x.Line, Item: it}, nil
				}
			}
		}
	}
	fields, order, tags, title := mediadbHeading(it, kind, req.Status, req.Rating)
	body := mediadb.Body(it)
	if req.Preview {
		lines := recordHeadingLines(1, title, kind, fields, order, body, tags)
		return MediadbAddResult{Ok: true, Item: it, Org: strings.Join(lines, "\n") + "\n"}, nil
	}
	at, err := mediadbTarget(kind, req.File, req.Under)
	if err != nil {
		return MediadbAddResult{}, err
	}
	res := MediadbAddResult{Item: it, Filename: at.filename}
	if it.Cover != "" && !req.NoCover && !mediadbSettings().NoCovers {
		data, ext, err := mediadbPicture(it.Cover)
		if err == nil {
			var link string
			if link, err = mediadbSavePicture(at.filename, kind, title, data, ext); err == nil {
				fields["IMAGE"] = link
			}
		}
		if err != nil {
			if _, nf := err.(mediadb.NotFound); nf {
				// No cover at all (the Cover Art Archive has none): say nothing
				// rather than link a picture that is not there.
				delete(fields, "IMAGE")
			} else {
				res.Warnings = append(res.Warnings, "the cover was not saved: "+err.Error())
			}
		}
	}
	lines, err := ganttFileLines(at.filename)
	if err != nil {
		return MediadbAddResult{}, err
	}
	add := spaceBefore(lines, at.row, recordHeadingLines(at.lvl, title, kind, fields, order, body, tags))
	head := at.row + 1
	if len(add) > 0 && add[0] == "" {
		head++
	}
	if _, err := insertLinesAt(at.filename, at.row, add); err != nil {
		return MediadbAddResult{}, err
	}
	if f := GetDb().FindByFile(at.filename); f != nil {
		if sec := sectionAtRow(f, head); sec != nil && sec.Headline != nil && sec.Headline.Pos.Row == head {
			res.Hash = sec.Hash
		}
	}
	res.Ok, res.Line = true, head
	res.Msg = fmt.Sprintf("%s added to %s", title, filepath.Base(at.filename))
	return res, nil
}

func mediadbFirst(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Filling in again, and the shelf and stars
// ---------------------------------------------------------------------------

// MediadbRefreshResult is what /mediadb/refresh changed.
type MediadbRefreshResult struct {
	Ok       bool
	Msg      string   `json:",omitempty"`
	Hash     string   `json:",omitempty"`
	Set      []string `json:",omitempty"`
	Warnings []string `json:",omitempty"`
}

// mediadbRefresh fills in what a heading is missing: the fields, the cover,
// the genre tags and, when the heading has no text of its own, the
// description. A heading without a MEDIA_ID is looked up by its ISBN or its
// title (and creator), and the best match's MEDIA_ID written. Overwrite
// replaces fields that differ too; it never touches STATUS or RATING.
func mediadbRefresh(hash string, overwrite bool) (MediadbRefreshResult, error) {
	mine, err := mediadbFind(hash)
	if err != nil {
		return MediadbRefreshResult{}, err
	}
	kind := mine.Kind
	var it *mediadb.Item
	switch {
	case mine.MediaId != "" && strings.Contains(mine.MediaId, ":"):
		i := strings.IndexByte(mine.MediaId, ':')
		it, err = mediadbDetails(kind, mine.MediaId[:i], mine.MediaId[i+1:])
	case kind == mediadb.Book && mine.Isbn != "":
		it, err = mediadbDetails(kind, "", mine.Isbn)
	default:
		q := mine.Title
		res := mediadbSearch(kind, q, "")
		if !res.Ok || len(res.Candidates) == 0 {
			return MediadbRefreshResult{}, fmt.Errorf("could not find %q: %s", mine.Title, mediadbFirst(res.Msg, "nothing found"))
		}
		pick := res.Candidates[0]
		for _, c := range res.Candidates {
			if strings.EqualFold(c.Title, mine.Title) && (mine.Creator == "" || strings.Contains(strings.ToLower(c.Creator), strings.ToLower(strings.SplitN(mine.Creator, ",", 2)[0]))) &&
				(mine.Year == "" || c.Year == "" || c.Year == mine.Year) {
				pick = c
				break
			}
		}
		it, err = mediadbDetails(kind, pick.Source, pick.Id)
	}
	if err != nil {
		return MediadbRefreshResult{}, err
	}
	res := MediadbRefreshResult{Ok: true, Hash: hash}
	fields, _, tags, _ := mediadbHeading(it, kind, mine.StatusWord, 0)
	delete(fields, "STATUS")
	delete(fields, "STARTED_DATE")
	delete(fields, "FINISHED_DATE")
	delete(fields, "RATING")
	set := map[string]string{}
	for k, v := range fields {
		if k == "IMAGE" {
			continue
		}
		cur := mediadbProp(mine.Fields, k)
		if cur == "" || (overwrite && cur != v) {
			set[k] = v
		}
	}
	if mediadbProp(mine.Fields, "IMAGE", "COVER", "POSTER") == "" && it.Cover != "" {
		if mediadbSettings().NoCovers {
			set["IMAGE"] = "[[" + it.Cover + "]]"
		} else if data, ext, err := mediadbPicture(it.Cover); err == nil {
			if link, err := mediadbSavePicture(mine.Filename, kind, mine.Title, data, ext); err == nil {
				set["IMAGE"] = link
			} else {
				res.Warnings = append(res.Warnings, "the cover was not saved: "+err.Error())
			}
		} else if _, nf := err.(mediadb.NotFound); !nf {
			res.Warnings = append(res.Warnings, "the cover was not saved: "+err.Error())
		}
	}
	upd := &common.RecordUpdate{Hash: hash, Set: set, Via: "from " + it.Source}
	sec := GetDb().FindByHash(hash)
	if strings.TrimSpace(mediadbAbout(mine)) == "" && sec != nil && len(sec.Children) == 0 {
		if body := mediadb.Body(it); body != "" {
			upd.SetNotes, upd.Notes = true, body
			res.Set = append(res.Set, "the description")
		}
	}
	for k := range set {
		res.Set = append(res.Set, k)
	}
	sort.Strings(res.Set)
	if len(set) > 0 || upd.SetNotes {
		if _, err := UpdateRecord(upd); err != nil {
			return MediadbRefreshResult{}, err
		}
		mediadbKeepIndent(mine.Filename, mine.Line)
	}
	// Tags last, on the headline, found again by its title: the update above
	// may have changed the hash.
	if len(tags) > 0 {
		if n, err := mediadbAddTags(mine.Filename, mine.Line, tags); err == nil && n > 0 {
			res.Set = append(res.Set, fmt.Sprintf("%d tag%s", n, plural(n)))
		}
	}
	if f := GetDb().FindByFile(mine.Filename); f != nil {
		if s := sectionAtRow(f, mine.Line); s != nil && s.Headline != nil && s.Headline.Pos.Row == mine.Line {
			res.Hash = s.Hash
		}
	}
	if len(res.Set) == 0 {
		res.Msg = mine.Title + " has everything " + it.Source + " knows"
	} else {
		res.Msg = mine.Title + ": " + strings.Join(res.Set, ", ") + " from " + it.Source
	}
	return res, nil
}

// mediadbKeepIndent puts the drawer lines UpdateRecord added at the indent
// the heading's :PROPERTIES: line has: it writes new lines at the heading's
// depth, which in a file whose drawers are in column zero leaves a ragged
// drawer.
func mediadbKeepIndent(filename string, row int) {
	lines, err := ganttFileLines(filename)
	if err != nil || row < 0 || row >= len(lines) {
		return
	}
	end := subtreeEndRow(lines, row, strings.Count(strings.SplitN(lines[row], " ", 2)[0], "*"), row)
	if end >= len(lines) {
		end = len(lines) - 1
	}
	stop := headEnd(lines, row, end)
	ind := ""
	found := false
	for i := row + 1; i < stop; i++ {
		if strings.TrimSpace(lines[i]) == ":PROPERTIES:" {
			ind = lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
			found = true
			break
		}
	}
	if !found {
		return
	}
	changed := false
	for i := row + 1; i < stop; i++ {
		t := strings.TrimLeft(lines[i], " \t")
		if t == "" || planningRe.MatchString(lines[i]) {
			continue
		}
		if ind+t != lines[i] {
			lines[i] = ind + t
			changed = true
		}
	}
	if changed {
		writeLines(filename, lines)
	}
}

// mediadbAddTags puts tags the headline lacks on it.
func mediadbAddTags(filename string, row int, tags []string) (int, error) {
	lines, err := ganttFileLines(filename)
	if err != nil || row < 0 || row >= len(lines) {
		return 0, err
	}
	n := 0
	line := lines[row]
	for _, t := range tags {
		var ok bool
		if line, ok = addHeadlineTag(line, t); ok {
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	lines[row] = line
	return n, writeLines(filename, lines)
}

// MediadbSetRequest is POST /mediadb/set: a shelf, stars, or both.
type MediadbSetRequest struct {
	Hash   string
	Status string   // a shelf (want, doing, done, dropped) or a word ("reading")
	Rating *float64 // 0 clears
}

func mediadbSet(req *MediadbSetRequest) (MediadbItem, error) {
	mine, err := mediadbFind(req.Hash)
	if err != nil {
		return MediadbItem{}, err
	}
	set := map[string]string{}
	if strings.TrimSpace(req.Status) != "" {
		shelf := mediadb.ShelfOfWord(req.Status)
		if shelf == "" {
			return MediadbItem{}, fmt.Errorf("%q is not a status (want, doing, done, dropped - or %q)", req.Status, mediadb.StatusWord(mine.Kind, mediadb.Done))
		}
		set["STATUS"] = mediadb.StatusWord(mine.Kind, shelf)
		now := orgStamp(time.Now())
		if shelf == mediadb.Doing && mediadbProp(mine.Fields, "STARTED_DATE") == "" {
			set["STARTED_DATE"] = now
		}
		if shelf == mediadb.Done && mediadbProp(mine.Fields, "FINISHED_DATE") == "" {
			set["FINISHED_DATE"] = now
		}
	}
	if req.Rating != nil {
		set["RATING"] = mediadb.RatingText(*req.Rating)
	}
	if len(set) == 0 {
		return *mine, nil
	}
	if _, err := UpdateRecord(&common.RecordUpdate{Hash: req.Hash, Set: set, Via: "in the library"}); err != nil {
		return MediadbItem{}, err
	}
	mediadbKeepIndent(mine.Filename, mine.Line)
	if f := GetDb().FindByFile(mine.Filename); f != nil {
		if s := sectionAtRow(f, mine.Line); s != nil && s.Headline != nil && s.Headline.Pos.Row == mine.Line {
			return *mediadbRead(f, s, mine.Kind), nil
		}
	}
	return *mine, nil
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

type mediadbAnswer struct {
	Ok  bool
	Msg string `json:",omitempty"`
}

func mediadbBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err := json.Unmarshal(body, v); err != nil {
		dynJson(w, mediadbAnswer{Msg: err.Error()})
		return false
	}
	return true
}

/* SDOC: API
* GET /mediadb/search — Look A Book, Film, Show, Game Or Album Up
* GET /mediadb/details — One In Full
* POST /mediadb/add — File One As A Heading
* GET /mediadb/items — What Is On The Shelves
* POST /mediadb/refresh — Fill In What A Heading Is Missing
* POST /mediadb/set — Its Shelf And Stars
* GET /mediadb/sources — Which Catalogues Can Be Asked

	#+BEGIN_EXAMPLE
	GET  /mediadb/search?kind=book&q=dune[&source=googlebooks]
	     {"Ok": true, "Kind": "book", "Source": "openlibrary", "Attribution": "...",
	      "Candidates": [{Kind, Source, Id, Title, Year, Creator, Thumb, Blurb, Isbn, Have}],
	      "Tried": [{Source, Label, Msg}]}
	GET  /mediadb/details?kind=film&source=tmdb&id=movie/329865
	     {"Ok": true, "Item": {Kind, Source, Id, Title, Creators, Cast, Date, Genres, Description, Cover, Url, ...}}
	POST /mediadb/add  {"Kind": "book", "Source": "openlibrary", "Id": "OL893414W", "File": "", "Under": "",
	                    "Status": "reading", "Rating": 4, "Force": false, "NoCover": false, "Preview": false}
	     {"Ok": true, "Hash", "Filename", "Line", "Item", "Org" (with Preview), "Existing" (when refused), "Warnings"}
	GET  /mediadb/items?kind=book[&shelf=doing][&about=true]
	     {"Ok": true, "Items": [{Hash, Filename, Line, Kind, Title, Creator, Year, Shelf, StatusWord, Rating,
	                             Cover, Tags, MediaId, Isbn, Url, Started, Finished, Fields, About}]}
	POST /mediadb/refresh {"Hash": "...", "Overwrite": false}   {"Ok": true, "Hash", "Set": [...], "Msg"}
	POST /mediadb/set     {"Hash": "...", "Status": "done", "Rating": 4.5}   {"Ok": true, "Item": {...}}
	GET  /mediadb/sources
	     {"Ok": true, "Sources": [{Name, Label, Kinds, Usable, Missing, Site, Attribution}],
	      "Kinds": [{Kind, Label, Sources, Statuses: {want, doing, done, dropped}}]}
	#+END_EXAMPLE

	A kind is =book=, =film=, =show= (TV), =game=, =album= or =other= (a
	Wikipedia entry); =movie=, =tv= and =music= are taken too. Books come
	from Open Library (an ISBN, with or without hyphens, is looked up
	directly) and Google Books; films and shows from TMDB with a key, else
	Wikidata; games from IGDB or RAWG with keys, else Wikidata; albums from
	MusicBrainz (one request a second) with covers from the Cover Art
	Archive. The kind's sources are tried in order: one without its key is
	skipped, one that cannot be reached is passed over, and =Tried= says what
	each answered. Answers are kept for an hour. Requests are http(s) only,
	15 seconds, 4 MB (10 MB for a cover), with a User-Agent naming orgs and
	=mediaDb.contact=.

	=/mediadb/add= writes a record heading - =:RECORD: book= (=film=,
	=show=...), =ID=, =ADDED=, the kind's fields (=AUTHOR=, =PUBLISHED_DATE=,
	=PUBLISHER=, =PAGES=, =ISBN=; =DIRECTOR=, =CAST=, =RELEASE_DATE=,
	=RUNTIME=; =CREATOR=, =NETWORK=, =SEASONS=; =DEVELOPER=, =PLATFORMS=;
	=ARTIST=, =LABEL=, =TRACKS=), =URL=, =IMAGE=, =MEDIA_ID= (=source:id=),
	=STATUS= - with the genres as tags and the description (an album's
	tracks too) as its text. The cover is saved in =images/<kind>/= beside
	the org file and linked relatively. It goes under =Under= in =File=;
	else the kind's =file=/=under= or =capture= template in the yaml; else
	the kind's collection; else a new collection heading (Books, Films...)
	in =library.org=. The same =MEDIA_ID= or ISBN already filed is refused
	unless =Force=.

	A heading is on a shelf (=want=, =doing=, =done=, =dropped=; for a
	book "to read", "reading", "read", "set aside") by its =STATUS=
	property, else its TODO keyword (done keywords are done, =TODO= want,
	any other doing), else =FINISHED_DATE= / =STARTED_DATE=. =/mediadb/set=
	writes =STATUS= (and =STARTED_DATE= on starting, =FINISHED_DATE= on
	finishing, when they are not there) and =RATING= (0 to 5, halves
	allowed; 0 clears). =/mediadb/refresh= looks the heading up again (by
	=MEDIA_ID=, else ISBN, else title) and fills in the fields, cover, tags
	and - when it has none - text it is missing; =Overwrite= also replaces
	fields that changed. Needs =features: mediaDb:=.
EDOC */
func RequestMediadbSearch(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaDb") {
		return
	}
	q := r.URL.Query()
	kind, err := mediadbKind(q.Get("kind"))
	if err != nil {
		dynJson(w, mediadbAnswer{Msg: err.Error()})
		return
	}
	if strings.TrimSpace(q.Get("q")) == "" {
		dynJson(w, mediadbAnswer{Msg: "nothing to search for"})
		return
	}
	dynJson(w, mediadbSearch(kind, q.Get("q"), q.Get("source")))
}

func RequestMediadbDetails(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaDb") {
		return
	}
	q := r.URL.Query()
	kind, err := mediadbKind(q.Get("kind"))
	if err == nil {
		var it *mediadb.Item
		if it, err = mediadbDetails(kind, q.Get("source"), q.Get("id")); err == nil {
			dynJson(w, map[string]interface{}{"Ok": true, "Item": it, "Body": mediadb.Body(it)})
			return
		}
	}
	dynJson(w, mediadbAnswer{Msg: err.Error()})
}

func PostMediadbAdd(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaDb") {
		return
	}
	var req MediadbAddRequest
	if !mediadbBody(w, r, &req) {
		return
	}
	res, err := mediadbAdd(&req)
	if err != nil {
		dynJson(w, mediadbAnswer{Msg: err.Error()})
		return
	}
	dynJson(w, res)
}

func RequestMediadbItems(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaDb") {
		return
	}
	q := r.URL.Query()
	kind := ""
	if k := q.Get("kind"); k != "" && k != "all" {
		var err error
		if kind, err = mediadbKind(k); err != nil {
			dynJson(w, mediadbAnswer{Msg: err.Error()})
			return
		}
	}
	shelf := mediadb.ShelfOfWord(q.Get("shelf"))
	about := q.Get("about") == "true" || q.Get("hash") != ""
	out := []MediadbItem{}
	for _, it := range mediadbAll() {
		if (kind != "" && it.Kind != kind) || (shelf != "" && it.Shelf != shelf) || (q.Get("hash") != "" && it.Hash != q.Get("hash")) {
			continue
		}
		c := *it
		if about {
			c.About = mediadbAbout(it)
		}
		out = append(out, c)
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Items": out})
}

func PostMediadbRefresh(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaDb") {
		return
	}
	var req struct {
		Hash      string
		Overwrite bool
	}
	if !mediadbBody(w, r, &req) {
		return
	}
	res, err := mediadbRefresh(req.Hash, req.Overwrite)
	if err != nil {
		dynJson(w, mediadbAnswer{Msg: err.Error()})
		return
	}
	dynJson(w, res)
}

func PostMediadbSet(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaDb") {
		return
	}
	var req MediadbSetRequest
	if !mediadbBody(w, r, &req) {
		return
	}
	it, err := mediadbSet(&req)
	if err != nil {
		dynJson(w, mediadbAnswer{Msg: err.Error()})
		return
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Item": it})
}

// MediadbSourceInfo is one catalogue, and whether it can be asked.
type MediadbSourceInfo struct {
	Name, Label string
	Kinds       []string
	Usable      bool
	Missing     []string `json:",omitempty"`
	OptionalKey bool     `json:",omitempty"`
	Site        string
	Attribution string
}

// MediadbKindInfo is one kind: what it is called, its sources in order, and
// its shelves' names.
type MediadbKindInfo struct {
	Kind     string
	Label    string
	Sources  []string
	Usable   bool
	Statuses map[string]string
	Fields   []string
}

func mediadbSourcesInfo() ([]MediadbSourceInfo, []MediadbKindInfo) {
	c := mediadbConfig()
	srcs := []MediadbSourceInfo{}
	for _, s := range mediadb.Sources {
		srcs = append(srcs, MediadbSourceInfo{Name: s.Name, Label: s.Label, Kinds: s.Kinds, Usable: s.Usable(c), Missing: s.Missing(c),
			OptionalKey: s.OptionalKey, Site: s.Site, Attribution: s.Attribution})
	}
	kinds := []MediadbKindInfo{}
	for _, k := range mediadb.Kinds {
		ki := MediadbKindInfo{Kind: k, Label: mediadb.KindLabel(k), Statuses: map[string]string{}, Fields: mediadb.FieldNames(k)}
		for _, s := range mediadb.SourcesFor(k, mediadbKindSettings(k).Sources) {
			ki.Sources = append(ki.Sources, s.Name)
			if s.Usable(c) {
				ki.Usable = true
			}
		}
		for _, sh := range mediadb.Shelves {
			ki.Statuses[sh] = mediadb.StatusWord(k, sh)
		}
		kinds = append(kinds, ki)
	}
	return srcs, kinds
}

func RequestMediadbSources(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "mediaDb") {
		return
	}
	srcs, kinds := mediadbSourcesInfo()
	dynJson(w, map[string]interface{}{"Ok": true, "Sources": srcs, "Kinds": kinds})
}

// ---------------------------------------------------------------------------
// A shelf in a document: #+BEGIN: library :kind book :shelf done
// ---------------------------------------------------------------------------

func init() {
	dynGenerators["library"] = mediadbDynBlock
}

// mediadbDynBlock writes a table of what is on a shelf, each title a link to
// its heading: :kind (book, film... or all), :shelf (want, doing, done,
// dropped, or a word like "read"), :sort (title, creator, year, rating,
// added - the default), :limit.
func mediadbDynBlock(c *dynCtx) ([]string, error) {
	if !featureOn("mediaDb") {
		return nil, fmt.Errorf("the library is turned off: `orgs features mediaDb on`")
	}
	kind := ""
	if k := c.param("kind", ""); k != "" && k != "all" {
		if kind = mediadb.KindOf(k); kind == "" {
			return nil, fmt.Errorf("%q is not a kind (book, film, show, game, album, other)", k)
		}
	}
	shelf := ""
	if s := c.param("shelf", ""); s != "" && s != "all" {
		if shelf = mediadb.ShelfOfWord(s); shelf == "" {
			return nil, fmt.Errorf("%q is not a shelf (want, doing, done, dropped)", s)
		}
	}
	xs := []*MediadbItem{}
	for _, it := range mediadbAll() {
		if (kind == "" || it.Kind == kind) && (shelf == "" || it.Shelf == shelf) {
			xs = append(xs, it)
		}
	}
	key := strings.ToLower(c.param("sort", "added"))
	sort.SliceStable(xs, func(i, j int) bool {
		a, b := xs[i], xs[j]
		switch key {
		case "title":
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		case "creator", "author", "director", "artist":
			return strings.ToLower(a.Creator) < strings.ToLower(b.Creator)
		case "year":
			return a.Year > b.Year
		case "rating":
			return a.Rating > b.Rating
		}
		return a.Added > b.Added
	})
	if n, err := strconv.Atoi(c.param("limit", "")); err == nil && n > 0 && n < len(xs) {
		xs = xs[:n]
	}
	creator := "By"
	if kind != "" {
		creator = titleOf(strings.ToLower(mediadb.CreatorField(kind)))
	}
	table := [][]string{{"Title", creator, "Year", "Status", "Rating"}, nil}
	for _, it := range xs {
		title := strings.ReplaceAll(it.Title, "|", "/")
		if id := mediadbProp(it.Fields, "ID"); id != "" {
			title = "[[id:" + id + "][" + strings.ReplaceAll(strings.ReplaceAll(title, "[", "("), "]", ")") + "]]"
		}
		r := ""
		if it.Rating > 0 {
			r = strings.Repeat("★", int(it.Rating))
			if it.Rating-float64(int(it.Rating)) >= 0.5 {
				r += "½"
			}
		}
		table = append(table, []string{title, strings.ReplaceAll(it.Creator, "|", "/"), it.Year, it.StatusWord, r})
	}
	return orgTableLines(table), nil
}
