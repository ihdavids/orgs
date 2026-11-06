package orgs

/* SDOC: Links
* Backlink Counts

  With =features: backlinkCounts:= on, every
  heading can say how many places link to it, and every link how many other
  places link where it goes. The numbers come from the same link index the
  backlinks use (and, when asked, the same unlinked mention search), so a
  count and the list it opens always agree.

  - A heading's count is the number of distinct *places* linking to it: a
    heading with three links to it counts once. The number of links is there
    too.
  - A link from a heading to itself is not counted, nor is a link that names
    the file rather than a heading in it (that counts for the file).
  - A link's count is how many other links in the database land on the same
    heading or file; the link itself is not counted.

  worg draws the counts as =↩ 3= beside each heading in the rendered page and
  the editor, VS Code as a code lens or inlay hint, and =orgs linkcounts= as
  an outline. =backlinkCounts.export: true= writes them into html exports.
EDOC */

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func registerBacklinkCountRoutes(api *mux.Router) {
	api.HandleFunc("/links/counts", RequestLinkCounts).Methods("GET")
	api.HandleFunc("/links/counts/text", PostLinkCountsText).Methods("POST")
	api.HandleFunc("/links/counts/top", RequestLinkCountsTop).Methods("GET")
}

// ---------------------------------------------------------------------------
// The pieces counted
// ---------------------------------------------------------------------------

// A heading of the file being counted for.
type lcHead struct {
	line  int
	sec   *org.Section // nil for a heading of an unsaved buffer the database does not have
	title string
	level int
}

// A link written in the file being counted for.
type lcLink struct {
	line, col, end int // UTF-16 columns
	raw, desc      string
	kind           string
	to             common.LinkEnd
	from           string // the place it is written in (placeKey)
}

// What a link lands on, as one string: a heading, or a file.
func targetKey(to common.LinkEnd) string {
	if to.Hash != "" {
		return "h:" + to.Hash
	}
	if to.Filename != "" {
		return "f:" + to.Filename
	}
	return ""
}

// Where a link is written: the heading, or the file's preamble.
func placeKey(from common.LinkEnd) string {
	if from.Hash != "" {
		return "h:" + from.Hash
	}
	return "f:" + from.Filename
}

func u16len(s string) int { return len(utf16.Encode([]rune(s))) }

// Where each link sits on its line, in document order: the first
// "[[raw]" on the line not already taken by an earlier link.
func placeLinkCols(lines []string, links []lcLink) {
	used := map[int][]int{}
	order := make([]int, len(links))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return links[order[a]].line < links[order[b]].line })
	for _, i := range order {
		l := &links[i]
		l.col, l.end = -1, -1
		if l.line < 0 || l.line >= len(lines) {
			continue
		}
		line := lines[l.line]
		needle := "[[" + l.raw + "]"
		from := 0
		for {
			k := strings.Index(line[from:], needle)
			if k < 0 {
				break
			}
			k += from
			taken := false
			for _, u := range used[l.line] {
				if u == k {
					taken = true
				}
			}
			if !taken {
				j := strings.Index(line[k:], "]]")
				if j < 0 {
					break
				}
				used[l.line] = append(used[l.line], k)
				l.col = u16len(line[:k])
				l.end = l.col + u16len(line[k:k+j+2])
				break
			}
			from = k + 2
		}
	}
}

// ---------------------------------------------------------------------------
// Counting
// ---------------------------------------------------------------------------

// The link index read by target, kept beside it.
type lcIndex struct {
	idx      *linkIndex
	byTarget map[string][]int
}

var (
	lcLock      sync.Mutex
	lcIdx       *lcIndex
	lcCache     = map[string]common.LinkCounts{}
)

// The index by target, and the saved answers reset, whenever the database
// moves (Traps: caches).
func linkCountIndex() *lcIndex {
	idx := getLinkIndex()
	lcLock.Lock()
	defer lcLock.Unlock()
	if lcIdx != nil && lcIdx.idx == idx {
		return lcIdx
	}
	li := &lcIndex{idx: idx, byTarget: map[string][]int{}}
	for i, l := range idx.links {
		if k := targetKey(l.To); k != "" {
			li.byTarget[k] = append(li.byTarget[k], i)
		}
	}
	lcIdx = li
	lcCache = map[string]common.LinkCounts{}
	return li
}

// mentionCount is a heading's (or with sec nil, a file's) unlinked mentions,
// through the mentions search and its guard.
func mentionCount(f *common.OrgFile, sec *org.Section) (int, string) {
	m := QueryMentions(f, sec, false, 1)
	if len(m.Target.Terms) == 0 {
		return 0, strings.Join(m.Target.Skipped, "; ")
	}
	return m.Total, ""
}

// countFile works the counts out for one file's headings and links. The
// file's own links are `own` (from the index for a saved file, from the text
// for a buffer); links from every other file come from the index.
func countFile(li *lcIndex, f *common.OrgFile, fname string, heads []lcHead, own []lcLink, mentions bool) common.LinkCounts {
	idx := li.idx
	res := common.LinkCounts{Ok: true, Filename: fname, Mentions: mentions,
		Headings: []common.LinkCountHeading{}, Links: []common.LinkCountLink{}}

	// Links from other files to a target.
	outside := func(key string) []common.OrgLink {
		var out []common.OrgLink
		for _, i := range li.byTarget[key] {
			if l := idx.links[i]; l.From.Filename != fname {
				out = append(out, l)
			}
		}
		return out
	}

	for _, h := range heads {
		c := common.LinkCountHeading{Line: h.line, Title: h.title, Level: h.level}
		if h.sec != nil {
			c.Hash = h.sec.Hash
			c.Olp = outlinePath(h.sec)
			key := "h:" + h.sec.Hash
			places := map[string]bool{}
			for _, l := range outside(key) {
				c.Links++
				places[placeKey(l.From)] = true
			}
			for _, l := range own {
				if l.to.Hash == h.sec.Hash && l.from != key {
					c.Links++
					places[l.from] = true
				}
			}
			for _, l := range own {
				if l.from == key && l.to.Hash != h.sec.Hash {
					c.Out++
				}
			}
			c.Backlinks = len(places)
			if mentions && f != nil {
				c.Mentions, c.MentionsSkipped = mentionCount(f, h.sec)
			}
		}
		res.Headings = append(res.Headings, c)
	}

	for i, l := range own {
		if l.col < 0 {
			continue
		}
		c := common.LinkCountLink{Line: l.line, Col: l.col, End: l.end, Raw: l.raw, Desc: l.desc, Kind: l.kind,
			ToFilename: l.to.Filename, ToHash: l.to.Hash, ToHeadline: l.to.Headline}
		if key := targetKey(l.to); key != "" {
			places := map[string]bool{}
			for _, o := range outside(key) {
				c.Count++
				places[placeKey(o.From)] = true
			}
			for j, o := range own {
				if j != i && targetKey(o.to) == key {
					c.Count++
					if o.from != l.from {
						places[o.from] = true
					}
				}
			}
			c.Places = len(places)
		}
		res.Links = append(res.Links, c)
	}

	// The file itself: links from elsewhere to the file, not a heading in it.
	places := map[string]bool{}
	for _, i := range idx.toFile[fname] {
		l := idx.links[i]
		if l.From.Filename == fname {
			continue
		}
		res.File.Any++
		if l.To.Hash == "" {
			res.File.Links++
			places[placeKey(l.From)] = true
		}
	}
	res.File.Backlinks = len(places)
	if mentions && f != nil {
		res.File.Mentions, res.File.MentionsSkipped = mentionCount(f, nil)
	}
	return res
}

// QueryLinkCounts is the counts for a saved file, cached until the database
// next changes.
func QueryLinkCounts(filename string, mentions bool) common.LinkCounts {
	fname, err := resolveRequestedFile(filename)
	if err != nil {
		return common.LinkCounts{Ok: false, Msg: err.Error(), Filename: filename}
	}
	f := GetDb().FindByFile(fname)
	if f == nil || f.Doc == nil {
		return common.LinkCounts{Ok: false, Msg: "file not found in the database: " + fname, Filename: fname}
	}
	li := linkCountIndex()
	key := fmt.Sprintf("%s\x00%v", fname, mentions)
	lcLock.Lock()
	got, ok := lcCache[key]
	ok = ok && lcIdx == li
	lcLock.Unlock()
	if ok {
		return got
	}

	heads := []lcHead{}
	for _, sec := range flattenSections(f) {
		if sec.Headline == nil {
			continue
		}
		heads = append(heads, lcHead{line: sec.Headline.Pos.Row, sec: sec,
			title: common.GetHeadlineTitle(sec.Headline), level: sec.Headline.Lvl})
	}
	own := []lcLink{}
	for _, i := range li.idx.fromFile[fname] {
		l := li.idx.links[i]
		own = append(own, lcLink{line: l.From.Line, raw: l.Raw, desc: l.Desc, kind: l.Kind, to: l.To, from: placeKey(l.From)})
	}
	lines, _ := readOrgLines(fname)
	placeLinkCols(lines, own)
	res := countFile(li, f, fname, heads, own, mentions)
	sort.SliceStable(res.Links, func(a, b int) bool {
		if res.Links[a].Line != res.Links[b].Line {
			return res.Links[a].Line < res.Links[b].Line
		}
		return res.Links[a].Col < res.Links[b].Col
	})

	lcLock.Lock()
	if lcIdx == li {
		lcCache[key] = res
	}
	lcLock.Unlock()
	return res
}

// ---------------------------------------------------------------------------
// An unsaved buffer
// ---------------------------------------------------------------------------

var (
	lcHeadRe   = regexp.MustCompile(`^(\*+)\s+(.*?)\s*$`)
	lcLinkRe   = regexp.MustCompile(`\[\[([^\]]+)\](?:\[([^\]]*)\])?\]`)
	lcKwRe     = regexp.MustCompile(`^[A-Z][A-Z_-]*\s+`)
	lcPrioRe   = regexp.MustCompile(`^\[#[A-Za-z0-9]\]\s*`)
	lcTagsRe   = regexp.MustCompile(`\s+:[\p{L}\p{N}_@#%:]+:\s*$`)
	lcBlockRe  = regexp.MustCompile(`(?i)^\s*#\+begin_(src|example|export)\b`)
	lcBlockEnd = regexp.MustCompile(`(?i)^\s*#\+end_(src|example|export)\b`)
)

// A headline's words with its stars, keyword, priority and tags off. Any
// leading upper case word is taken for a keyword, which is right for every
// keyword set and only matters for matching a buffer's heading to the saved
// one when the line itself has changed.
func lcLooseTitle(line string) string {
	m := lcHeadRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	t := m[2]
	if loc := lcTagsRe.FindStringIndex(t); loc != nil {
		t = t[:loc[0]]
	}
	t = lcKwRe.ReplaceAllString(t, "")
	t = lcPrioRe.ReplaceAllString(t, "")
	return strings.ToLower(strings.TrimSpace(t))
}

// matchBufferHeads finds each heading of a buffer in the saved file: the same
// headline line first, then the same ID or CUSTOM_ID, then the same words.
// Each saved heading is taken once, in order.
func matchBufferHeads(lines []string, saved []string, secs []*org.Section) []lcHead {
	type sh struct {
		sec       *org.Section
		raw       string
		loose     string
		id, cid   string
		taken     bool
	}
	var have []*sh
	for _, s := range secs {
		if s.Headline == nil {
			continue
		}
		r := s.Headline.Pos.Row
		x := &sh{sec: s}
		if r >= 0 && r < len(saved) {
			x.raw = strings.TrimRight(saved[r], " \t")
			x.loose = lcLooseTitle(saved[r])
			p := headingPropsIn(saved, r, subtreeEndRow(saved, r, s.Headline.Lvl, r))
			x.id, x.cid = p["ID"], p["CUSTOM_ID"]
		}
		have = append(have, x)
	}
	var heads []lcHead
	var rows []int
	for i, l := range lines {
		if m := lcHeadRe.FindStringSubmatch(l); m != nil {
			rows = append(rows, i)
			heads = append(heads, lcHead{line: i, level: len(m[1]), title: headlineTitleText(l, "")})
		}
	}
	take := func(i int, ok func(x *sh) bool) {
		if heads[i].sec != nil {
			return
		}
		for _, x := range have {
			if !x.taken && ok(x) {
				x.taken = true
				heads[i].sec = x.sec
				heads[i].title = common.GetHeadlineTitle(x.sec.Headline)
				return
			}
		}
	}
	for i := range heads {
		raw := strings.TrimRight(lines[rows[i]], " \t")
		take(i, func(x *sh) bool { return x.raw == raw })
	}
	for i := range heads {
		end := len(lines)
		if i+1 < len(rows) {
			end = rows[i+1]
		}
		p := headingPropsIn(lines, rows[i], end)
		if p["ID"] != "" {
			take(i, func(x *sh) bool { return x.id == p["ID"] })
		}
		if p["CUSTOM_ID"] != "" {
			take(i, func(x *sh) bool { return x.cid == p["CUSTOM_ID"] })
		}
	}
	for i := range heads {
		loose := lcLooseTitle(lines[rows[i]])
		if loose != "" {
			take(i, func(x *sh) bool { return x.loose == loose })
		}
	}
	for i := range heads {
		if heads[i].sec == nil {
			// Its words, cleaned as a title is: the keyword is a guess here.
			heads[i].title = strings.TrimSpace(lcPrioRe.ReplaceAllString(lcKwRe.ReplaceAllString(heads[i].title, ""), ""))
		}
	}
	return heads
}

// QueryLinkCountsText is the counts for a buffer that may not be saved.
func QueryLinkCountsText(req common.LinkCountsText) common.LinkCounts {
	db := GetDb()
	lines := strings.Split(strings.ReplaceAll(req.Text, "\r\n", "\n"), "\n")
	fname := ""
	var f *common.OrgFile
	if strings.TrimSpace(req.Filename) != "" {
		if n, err := resolveRequestedFile(req.Filename); err == nil {
			fname = n
			f = db.FindByFile(n)
		} else {
			fname = req.Filename
		}
	}
	li := linkCountIndex()
	var secs []*org.Section
	var saved []string
	if f != nil {
		secs = flattenSections(f)
		saved, _ = readOrgLines(f.Filename)
	}
	heads := matchBufferHeads(lines, saved, secs)

	// The buffer's links, each from the heading it is under.
	own := []lcLink{}
	hi := -1
	inBlock := false
	for row, line := range lines {
		for hi+1 < len(heads) && heads[hi+1].line <= row {
			hi++
		}
		if inBlock {
			if lcBlockEnd.MatchString(line) {
				inBlock = false
			}
			continue
		}
		if lcBlockRe.MatchString(line) {
			inBlock = true
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "# ") {
			continue
		}
		from := "f:" + fname
		if hi >= 0 {
			if heads[hi].sec != nil {
				from = "h:" + heads[hi].sec.Hash
			} else {
				from = "new:" + strconv.Itoa(heads[hi].line)
			}
		}
		for _, m := range lcLinkRe.FindAllStringSubmatchIndex(line, -1) {
			raw := line[m[2]:m[3]]
			desc := ""
			if m[4] >= 0 {
				desc = line[m[4]:m[5]]
			}
			to, kind, _ := resolveLink(db, li.idx.ids, li.idx.lookup, raw, fname)
			col := u16len(line[:m[0]])
			own = append(own, lcLink{line: row, col: col, end: col + u16len(line[m[0]:m[1]]),
				raw: raw, desc: desc, kind: kind, to: to, from: from})
		}
	}
	return countFile(li, f, fname, heads, own, req.Mentions)
}

// ---------------------------------------------------------------------------
// The most linked headings
// ---------------------------------------------------------------------------

func QueryLinkCountsTop(limit int, mentions bool) common.LinkCountTopResult {
	li := linkCountIndex()
	db := GetDb()
	res := common.LinkCountTopResult{Ok: true, Mentions: mentions, Headings: []common.LinkCountTop{}}
	type acc struct {
		to     common.LinkEnd
		links  int
		places map[string]bool
	}
	by := map[string]*acc{}
	for _, l := range li.idx.links {
		if l.To.Hash == "" || l.From.Hash == l.To.Hash {
			continue
		}
		a := by[l.To.Hash]
		if a == nil {
			a = &acc{to: l.To, places: map[string]bool{}}
			by[l.To.Hash] = a
		}
		a.links++
		a.places[placeKey(l.From)] = true
	}
	for _, a := range by {
		res.Headings = append(res.Headings, common.LinkCountTop{Filename: a.to.Filename, Hash: a.to.Hash,
			Title: a.to.Headline, Olp: a.to.Olp, Line: a.to.Line, Level: a.to.Level,
			Backlinks: len(a.places), Links: a.links})
	}
	res.Total = len(res.Headings)
	sort.SliceStable(res.Headings, func(i, j int) bool {
		a, b := res.Headings[i], res.Headings[j]
		if a.Backlinks != b.Backlinks {
			return a.Backlinks > b.Backlinks
		}
		if a.Links != b.Links {
			return a.Links > b.Links
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.Filename < b.Filename
	})
	if limit > 0 && len(res.Headings) > limit {
		res.Headings = res.Headings[:limit]
	}
	if mentions {
		for i := range res.Headings {
			h := &res.Headings[i]
			if sec := db.FindByHash(h.Hash); sec != nil {
				if f := db.FileFromSection(sec); f != nil {
					h.Mentions, _ = mentionCount(f, sec)
				}
			}
		}
	}
	return res
}

// ---------------------------------------------------------------------------
// Html exports
// ---------------------------------------------------------------------------

// backlinkExportMayApply is whether an export for backend gets counts.
func backlinkExportMayApply(backend string) bool {
	s := Conf().Server
	return s != nil && s.BacklinkCounts.Export && strings.EqualFold(backend, "html") && featureOn("backlinkCounts")
}

// backlinkExportLines writes the count after every linked-to headline of
// the export copy, as an html export snippet before the tags. Headlines are
// found again in the copy by their text (transclusion may have moved them),
// the nth of the same text for the nth.
func backlinkExportLines(filename string, out []string) ([]string, bool) {
	s := Conf().Server
	res := QueryLinkCounts(filename, s != nil && s.BacklinkCounts.Mentions)
	if !res.Ok {
		return out, false
	}
	lines, err := readOrgLines(res.Filename)
	if err != nil {
		return out, false
	}
	want := map[string][]common.LinkCountHeading{}
	for _, h := range res.Headings {
		if h.Line >= 0 && h.Line < len(lines) {
			k := strings.TrimRight(lines[h.Line], " \t")
			want[k] = append(want[k], h)
		}
	}
	seen := map[string]int{}
	changed := false
	cp := append([]string{}, out...)
	for i, l := range cp {
		if !mentionHeadRe.MatchString(l) {
			continue
		}
		k := strings.TrimRight(l, " \t")
		n := seen[k]
		seen[k]++
		hs := want[k]
		if n >= len(hs) {
			continue
		}
		h := hs[n]
		badge := backlinkBadgeHtml(h)
		if badge == "" {
			continue
		}
		if loc := lcTagsRe.FindStringIndex(k); loc != nil {
			cp[i] = k[:loc[0]] + " " + badge + k[loc[0]:]
		} else {
			cp[i] = k + " " + badge
		}
		changed = true
	}
	return cp, changed
}

func backlinkBadgeHtml(h common.LinkCountHeading) string {
	parts := []string{}
	tips := []string{}
	if h.Backlinks > 0 {
		parts = append(parts, fmt.Sprintf("↩ %d", h.Backlinks))
		tips = append(tips, lcPlural(h.Backlinks, "place links here", "places link here"))
	}
	if h.Mentions > 0 {
		parts = append(parts, fmt.Sprintf("≈ %d", h.Mentions))
		tips = append(tips, lcPlural(h.Mentions, "unlinked mention", "unlinked mentions"))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf(`@@html:<span class="org-backlinks" title="%s">%s</span>@@`, strings.Join(tips, ", "), strings.Join(parts, " "))
}

func lcPlural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

func lcTruthy(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s != "" && s != "f" && s != "false" && s != "0" && s != "nil"
}

/* SDOC: API
* GET /links/counts — How Many Places Link To Each Heading Of A File

	For =features: backlinkCounts:=. Every heading of
	the file by its zero based line, with its hash and title, and how many
	places link to it (=Backlinks=: distinct headings, a file's preamble
	counting as one place; =Links=: every link), how many links it writes to
	elsewhere (=Out=) and, with =mentions=, how many unlinked mentions it has
	(the =/mentions= search and its guard; =MentionsSkipped= says why a title
	was not searched). =Links= is every link written in the file, with its
	line and =Col=/=End= (UTF-16 code units, end exclusive) and =Count=, how
	many other links in the database land on the same heading or file
	(=Places=: written in how many other headings). =File= is the links to the
	file itself (=Any=: to it or any heading in it).

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                   |
	|------------+--------+----------+-----------------------------------------------|
	| =filename= | string | yes      | The file.                                     |
	| =mentions= | bool   | no       | Count unlinked mentions too (slower).         |

	*Response:* A =LinkCounts= object. Off: ={Ok: false, Off: true, Msg}=.
EDOC */
func RequestLinkCounts(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "backlinkCounts") {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	json.NewEncoder(w).Encode(QueryLinkCounts(q.Get("filename"), lcTruthy(q.Get("mentions"))))
}

/* SDOC: API
* POST /links/counts/text — Backlink Counts For An Unsaved Buffer

	The answer =/links/counts= gives, for text an editor holds. Headings are
	read off the text and matched to the saved file's (the same headline
	line, else the same =ID= or =CUSTOM_ID=, else the same words); a heading
	the saved file does not have yet has no hash and no counts. The buffer's
	links are resolved as the link index resolves them, and counted with the
	links of every other file.

	#+BEGIN_EXAMPLE
	POST /links/counts/text  {"Filename": "notes.org", "Text": "* Plans\n...", "Mentions": false}
	#+END_EXAMPLE

	*Response:* A =LinkCounts= object.
EDOC */
func PostLinkCountsText(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "backlinkCounts") {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var req common.LinkCountsText
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(common.LinkCounts{Ok: false, Msg: "bad request: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(QueryLinkCountsText(req))
}

/* SDOC: API
* GET /links/counts/top — The Most Linked Headings

	Every heading in the database something links to, most linked first (by
	distinct places, then links). =Total= is how many there are before the
	limit.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type | Required | Description                                       |
	|------------+------+----------+---------------------------------------------------|
	| =limit=    | int  | no       | At most this many, default 20; 0 for every one.   |
	| =mentions= | bool | no       | Count each one's unlinked mentions too.           |

	*Response:* A =LinkCountTopResult=.
EDOC */
func RequestLinkCountsTop(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "backlinkCounts") {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	limit := 20
	if s := q.Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			limit = n
		}
	}
	json.NewEncoder(w).Encode(QueryLinkCountsTop(limit, lcTruthy(q.Get("mentions"))))
}
