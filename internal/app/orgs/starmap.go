package orgs

// The org database as a galaxy: every file or heading a star, every link a
// thread of light between two of them.
//
// This is the data half of worg's Starmap tab, and it is an endpoint rather than
// something the client works out for three reasons, each of which the browser
// cannot answer for itself:
//
//  1. Every star has to be there. `/links/graph` is built out of the link index,
//     so a heading nothing links to is not in it - and in a real org database
//     that is most of them. A galaxy made only of the linked things is not a
//     map of the database, it is a map of its links.
//  2. A star's weight and its birthday come off the disk. Size, modification
//     time, a date in the file's name, a CREATED property - none of it is on the
//     wire anywhere else.
//  3. The outline is the relationship org is made of. Obsidian has folders and
//     links and nothing else; org has a tree, and a starmap that ignored it
//     would draw nine tenths of a database as unconnected dust. The structural
//     edges are handed over *named* as outline edges rather than mixed in with
//     the links, so a client can draw them differently and count them
//     separately - a heading under another heading has not linked to it.
//
// Archived things are never stars. They are counted, and the most recent few are
// named, because that is the black hole's diet and a client wants to say what
// went in rather than only how much.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// How many stars are handed over when the client does not say. The physics is
// O(n²) in the client, so a cap is a kindness rather than a limitation - and the
// answer always says how many there were before it was applied.
const defaultStarLimit = 4000
const maxStarLimit = 20000

// How many of the swallowed are named. Enough for a caption, not a second list
// of the database.
const eatenNamed = 12

// A folder, or a heading, whose contents are the black hole's. `.org_archive` is
// org's own convention; the rest are what people actually call these places.
var archiveDirRe = regexp.MustCompile(`(?i)^(archive|archives|archived|_archive|_to_delete|attic|trash)$`)

// A folder whose files are logs rather than notes: a journal, a day page, a
// session record. They draw as smaller ember stars, because there are a great
// many of them and none of them is what somebody opening a map of their notes is
// looking for.
//
// The distinctive words match anywhere in a folder's name - `worklog` and
// `old_worklog` are both day page folders - while bare `log` has to end the
// segment, so `logic` and `dialogs` are left alone. The configured day page and
// session folders are believed ahead of any of this: those are the server being
// told rather than the server guessing.
var logDirRe = regexp.MustCompile(`(?i)(journal|daily|dailies|diary|daypage|day-page|worklog|logbook|session)|log$|logs$`)

// A date at the front of a name - `2026-09-28 Standup` - which is how a day page
// says when it is from. Read as the star's birthday, and a *written* date rather
// than a filesystem one: a modification time moves whenever anything is edited,
// so a bulk reformat would otherwise light the whole sky up as newly born.
var namedDateRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})`)

// The properties a heading says its own birthday in, in the order they are
// believed. All of them are things somebody wrote down.
var createdProps = []string{"CREATED", "ADDED", "DATE", "CREATED_AT", "BIRTHDAY"}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

// The path as a person would write it: relative to whichever org directory holds
// it. The longest matching root wins, so nested org directories name the file
// from the nearest one rather than from the first one configured.
func orgRelPath(fname string) string {
	best := ""
	if Conf().Server != nil {
		for _, dir := range Conf().Server.OrgDirs {
			d, err := filepath.Abs(dir)
			if err != nil {
				d = dir
			}
			if !strings.HasSuffix(d, string(filepath.Separator)) {
				d += string(filepath.Separator)
			}
			if strings.HasPrefix(fname, d) && len(d) > len(best) {
				best = d
			}
		}
	}
	if best == "" {
		return filepath.Base(fname)
	}
	return filepath.ToSlash(strings.TrimPrefix(fname, best))
}

// The folder a file's constellation is named after: its first path segment under
// the org directory. A file sitting at the root has none, and belongs to the
// constellation of loose files rather than to one of its own.
func topFolder(rel string) string {
	if i := strings.Index(rel, "/"); i > 0 {
		return rel[:i]
	}
	return ""
}

func pathSegments(rel string) []string {
	return strings.Split(rel, "/")
}

// Is this file one the black hole has already eaten? A folder anywhere in its
// path saying so, org's own `.org_archive` suffix, or the file saying so itself
// with a file tag.
//
// The file tag matters more than it looks. `IsArchived` reads a heading's tags
// *and* its file's, so every heading in a file tagged `:ARCHIVE:` is archived -
// which is right, and counting each of them as a separate meal is not. A file
// that was put away is one decision, and on a real database this was the
// difference between "3547 swallowed" and the couple of hundred things that were
// actually archived.
func archivedFile(rel string, doc *org.Document) bool {
	if doc != nil && (HasFileTag("archive", doc) || HasFileTag("archived", doc)) {
		return true
	}
	return archivedPath(rel)
}

func archivedPath(rel string) bool {
	segs := pathSegments(rel)
	for i, s := range segs {
		if i == len(segs)-1 {
			break
		}
		if archiveDirRe.MatchString(s) {
			return true
		}
	}
	return strings.HasSuffix(strings.ToLower(rel), ".org_archive")
}

// A log rather than a note: the server was told where the day pages are, or
// somewhere in the path says so, or the file is named after a date.
func logFile(fname string, rel string) bool {
	if under(fname, Conf().Server.DayPagePath) || under(fname, Conf().Server.DndSessionPath) {
		return true
	}
	segs := pathSegments(rel)
	for i, s := range segs {
		if i == len(segs)-1 {
			break
		}
		if logDirRe.MatchString(s) {
			return true
		}
	}
	return namedDateRe.MatchString(filepath.Base(rel))
}

// Is this file inside that directory? The directory may be written relative to
// the first org directory, which is how the day page path is usually configured.
func under(fname string, dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	roots := []string{dir}
	if !filepath.IsAbs(dir) && Conf().Server != nil && len(Conf().Server.OrgDirs) > 0 {
		roots = append(roots, filepath.Join(Conf().Server.OrgDirs[0], dir))
	}
	for _, root := range roots {
		d, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if !strings.HasSuffix(d, string(filepath.Separator)) {
			d += string(filepath.Separator)
		}
		if strings.HasPrefix(fname, d) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Dates
// ---------------------------------------------------------------------------

func dayString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// The date written at the front of a name, if there is one.
func namedDate(name string) string {
	m := namedDateRe.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return m[1] + "-" + m[2] + "-" + m[3]
}

// What a file says its own date is - `#+DATE:` - if it says anything.
func keywordDate(doc *org.Document) string {
	if doc == nil {
		return ""
	}
	for _, k := range []string{"DATE", "CREATED"} {
		if v, ok := doc.BufferSettings[k]; ok {
			if d := namedDate(strings.TrimSpace(v)); d != "" {
				return d
			}
		}
	}
	return ""
}

// When a file was born, and whether anybody wrote that down.
//
// A date in the name is believed first because it is stable - a day page called
// `2026-09-28.org` was about that day whatever has been done to it since - then
// the file's own `#+DATE:`, and only then the filesystem, which is the answer
// that moves under you.
func fileDate(fname string, doc *org.Document, mod time.Time) (string, bool) {
	if d := namedDate(filepath.Base(fname)); d != "" {
		return d, true
	}
	if d := keywordDate(doc); d != "" {
		return d, true
	}
	return dayString(mod), false
}

// When a heading was born, falling back on its file.
//
// A property somebody wrote, then a date at the front of the headline, then a
// CLOSED stamp - all three are written down, so all three light a supernova.
// Scheduled and deadline dates are deliberately *not* used: they say when
// something is meant to happen, which is often years from when it was written,
// and a plan for next March is not a star born next March.
func sectionDate(sec *org.Section, title string, lines []string, fallback string, fallbackDated bool) (string, bool) {
	if v := GetProp(sec, createdProps...); v != "" {
		if d := namedDate(strings.TrimSpace(strings.Trim(v, "[]<>"))); d != "" {
			return d, true
		}
	}
	if d := namedDate(strings.TrimSpace(title)); d != "" {
		return d, true
	}
	if sec != nil && sec.Headline != nil && sec.Headline.Closed != nil && sec.Headline.Closed.Date != nil {
		if d := dayString(sec.Headline.Closed.Date.Start); d != "" {
			return d, true
		}
	}
	if d := earliestStamp(lines); d != "" {
		return d, true
	}
	return fallback, fallbackDated
}

// Any org timestamp, in either bracket. Read off the lines rather than out of
// the parse tree because most of them are not parsed as dates at all: a state
// change in the LOGBOOK, a clock line, a date written in a sentence.
var stampRe = regexp.MustCompile(`[\[<](\d{4}-\d{2}-\d{2})`)

// The earliest date written anywhere under a heading, ignoring any in the
// future.
//
// This exists because on a real database almost nothing has a CREATED property,
// and without it every star's birthday is its file's modification time - which
// moves whenever anything is edited, so the timeline replays "recently touched"
// while claiming to replay history. The earliest stamp is not when the heading
// was written, but it is a date it demonstrably existed by, which is the honest
// version of the same answer.
//
// Future dates are dropped rather than taken as a minimum: a task written today
// and scheduled for next March has not been born next March, and letting that
// win would put it at the wrong end of the timeline.
func earliestStamp(lines []string) string {
	today := time.Now().Format("2006-01-02")
	best := ""
	for _, l := range lines {
		for _, m := range stampRe.FindAllStringSubmatch(l, -1) {
			d := m[1]
			if d > today {
				continue
			}
			if best == "" || d < best {
				best = d
			}
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// The index
// ---------------------------------------------------------------------------

// Everything one scope's galaxy is made of, before any filter. Held whole so a
// request that only wants some of it does not rebuild the rest.
type starmapBase struct {
	reload uint64
	scope  string

	nodes []common.StarmapNode
	// Every edge that could be drawn, both kinds, keyed nowhere - the filter
	// walks them and keeps the ones whose ends both survived.
	edges []common.StarmapEdge
	// The files each star lives in, so a filter can be applied without looking
	// the section up again.
	sectionOf map[string]*org.Section
	fileOf    map[string]*common.OrgFile

	swallowed int
	// The archived, newest first.
	eaten []string
	// Links whose far end was never a star in this scope at all - external, or
	// pointing at nothing. Counted here rather than per request, because no
	// filter can bring them back.
	lostLinks int
}

var (
	starmapCached [2]*starmapBase // [0] file scope, [1] heading scope
	starmapLock   sync.Mutex
)

// What one file contributes to the galaxy: its stars, its outline edges, and
// what of it the black hole has eaten.
//
// Cached per file (see fileparts.go). The stars of one file can be worked out
// without looking at any other, which is what makes this splittable at all -
// the link edges cannot, and are still built globally below out of the link
// index.
type starmapPart struct {
	nodes     []common.StarmapNode
	outline   []common.StarmapEdge
	sections  map[string]*org.Section
	file      *common.OrgFile
	swallowed int
	eaten     []eatenStar
}

type eatenStar struct {
	name string
	date string
}

var starmapFileParts [2]*FileParts[starmapPart]

func scopeSlot(scope string) int {
	if scope == "heading" {
		return 1
	}
	return 0
}

// The whole galaxy for one scope, cached against the reload counter the way the
// code and record indexes are. Walking every section of every file and stat-ing
// every one of them is not something to do twice for one screen, and a client
// that lets somebody drag a slider will ask again.
func starmapFor(scope string) *starmapBase {
	db := GetDb()
	starmapLock.Lock()
	defer starmapLock.Unlock()
	slot := scopeSlot(scope)
	if c := starmapCached[slot]; c != nil && c.reload == db.ReloadIndex {
		return c
	}
	b := buildStarmap(scope)
	b.reload = db.ReloadIndex
	starmapCached[slot] = b
	return b
}

// One file, read once: what the disk says about it and what its lines are.
type fileFacts struct {
	size    int
	mod     time.Time
	lines   []string
	lineLen []int
}

func statFile(fname string) fileFacts {
	out := fileFacts{}
	if st, err := os.Stat(fname); err == nil {
		out.size = int(st.Size())
		out.mod = st.ModTime()
	}
	if data, err := os.ReadFile(fname); err == nil {
		out.lines = strings.Split(string(data), "\n")
		out.lineLen = make([]int, len(out.lines))
		for i, l := range out.lines {
			out.lineLen[i] = len(l) + 1
		}
	}
	return out
}

// How many bytes a range of lines holds. Used for a heading's weight, which is
// the size of everything written under it rather than of the file it shares with
// two hundred others.
func (self *fileFacts) bytesIn(from int, to int) int {
	if from < 0 {
		from = 0
	}
	if to > len(self.lineLen) {
		to = len(self.lineLen)
	}
	n := 0
	for i := from; i < to; i++ {
		n += self.lineLen[i]
	}
	return n
}

func headlineTitle(sec *org.Section) string {
	if sec == nil || sec.Headline == nil {
		return ""
	}
	var title string
	for _, n := range sec.Headline.Title {
		title += n.String()
	}
	return title
}

// Is this heading the black hole's? Tagged archive, or written under a heading
// that is - an `* Archive` heading with the year's finished work under it is how
// org's own archiver files things in place.
func archivedSection(sec *org.Section, doc *org.Document, olp []string) bool {
	if IsArchived(sec, doc) {
		return true
	}
	for _, name := range olp {
		if archiveDirRe.MatchString(strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

// Build one scope's galaxy.
// The date an archived heading carries, for naming the most recent meals.
func date0(sec *org.Section, title, fdate string, fdated bool) string {
	d, _ := sectionDate(sec, title, nil, fdate, fdated)
	return d
}

func buildStarmap(scope string) *starmapBase {
	heading := scope == "heading"
	b := &starmapBase{
		scope:     scope,
		nodes:     []common.StarmapNode{},
		edges:     []common.StarmapEdge{},
		sectionOf: map[string]*org.Section{},
		fileOf:    map[string]*common.OrgFile{},
	}
	// Which ids exist, so the link pass can tell a thread with two ends from one
	// with a loose end.
	have := map[string]bool{}
	// The archived, with the date they carry, so the newest can be named.
	eaten := []eatenStar{}

	slot := scopeSlot(scope)
	if starmapFileParts[slot] == nil {
		starmapFileParts[slot] = NewFileParts[starmapPart]()
	}
	// One file's stars, cached per file. Everything below works out what one
	// file contributes without looking at any other, which is what makes it
	// splittable; the link edges cannot be and are built globally afterwards.
	parts := starmapFileParts[slot].All(func(f *common.OrgFile) starmapPart {
		db := GetDb()
		fname := f.Doc.Path
		part := starmapPart{sections: map[string]*org.Section{}, file: f}
		b := &part
		rel := orgRelPath(fname)
		facts := statFile(fname)
		fdate, fdated := fileDate(fname, f.Doc, facts.mod)
		touched := dayString(facts.mod)
		fileEaten := archivedFile(rel, f.Doc)
		isLog := logFile(fname, rel)

		// Sections are registered into the hash index lazily, as queries walk
		// them, so a freshly started server has an empty one - the same trap
		// the link index and the code index both had to work around.
		secs := flattenSections(f)
		for _, sec := range secs {
			db.RegisterSection(sec.Hash, sec, f)
		}

		if fileEaten {
			// A whole archived file goes in as one meal rather than as one per
			// heading: it is a box that was put away, not a hundred separate
			// decisions.
			b.swallowed++
			b.eaten = append(b.eaten, eatenStar{name: filepath.Base(rel), date: touched})
			return part
		}

		fileId := fileNodeId(fname)
		label := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		folder := topFolder(rel)
		group := "note"
		if isLog {
			group = "log"
		}
		// A file is a star at either scope. At heading scope it is also the hub
		// every heading in it hangs off: without it there is nothing for a link
		// naming a whole file to land on, and nothing holding a file's headings
		// together but whatever links they happen to have.
		b.nodes = append(b.nodes, common.StarmapNode{
			Id: fileId, Kind: "file", Label: label,
			Filename: fname, Folder: folder, File: label, Path: rel, Group: group,
			Size: facts.size, Lines: len(facts.lines),
			Date: fdate, Dated: fdated, Touched: touched,
		})
		if !heading {
			return part
		}

		for i, sec := range secs {
			if sec == nil || sec.Headline == nil || sec.Hash == "" {
				continue
			}
			olp := outlinePath(sec)
			title := headlineTitle(sec)
			if archivedSection(sec, f.Doc, olp) {
				b.swallowed++
				b.eaten = append(b.eaten, eatenStar{name: title, date: date0(sec, title, fdate, fdated)})
				continue
			}
			// The subtree's extent, taken off the flattened list rather than
			// from `Headline.GetEnd()`: that one under-reports for a heading
			// whose body is only a planning line and a property drawer, because
			// the drawer is kept in `Headline.Properties` rather than among the
			// body nodes it measures.
			from := headlineRow(sec)
			to := len(facts.lines)
			for j := i + 1; j < len(secs); j++ {
				if secs[j].Headline != nil && secs[j].Headline.Lvl <= sec.Headline.Lvl {
					to = headlineRow(secs[j])
					break
				}
			}
			if to < from {
				to = from
			}
			id := headNodeId(sec.Hash)
			parent := fileId
			if sec.Parent != nil && sec.Parent.Hash != "" && sec.Parent.Headline != nil {
				parent = headNodeId(sec.Parent.Hash)
			}
			body := facts.lines
			if from <= to && to <= len(body) {
				body = body[from:to]
			}
			date, dated := sectionDate(sec, title, body, fdate, fdated)
			if title == "" {
				title = "(untitled)"
			}
			b.nodes = append(b.nodes, common.StarmapNode{
				Id: id, Kind: "heading", Label: title,
				Filename: fname, Hash: sec.Hash, Olp: olp, Level: sec.Headline.Lvl, Line: from,
				Parent: parent, Folder: folder, File: label, Path: rel, Group: group,
				Size: facts.bytesIn(from, to), Lines: to - from,
				Date: date, Dated: dated, Touched: touched,
				Status: sec.Headline.Status,
				Tags:   sec.Headline.Tags,
			})
			b.sections[id] = sec
			b.outline = append(b.outline, common.StarmapEdge{From: parent, To: id, Kind: "outline"})
		}
		return part
	})

	// Join the parts. The order is the database's file order, which is what the
	// stable sort below and the client's seeding both depend on.
	for i := range parts {
		p := &parts[i]
		b.nodes = append(b.nodes, p.nodes...)
		b.edges = append(b.edges, p.outline...)
		b.swallowed += p.swallowed
		eaten = append(eaten, p.eaten...)
		for _, n := range p.nodes {
			have[n.Id] = true
			b.fileOf[n.Id] = p.file
		}
		for id, sec := range p.sections {
			b.sectionOf[id] = sec
		}
	}

	// The links, from the same index the graph is built from, so the two cannot
	// disagree about what a link is.
	inSets := map[string]map[string]bool{}
	outSets := map[string]map[string]bool{}
	links := map[edgeKey]*common.StarmapEdge{}
	for _, l := range getLinkIndex().links {
		if l.To.Filename == "" {
			b.lostLinks++
			continue
		}
		from := endNodeId(l.From, heading)
		to := endNodeId(l.To, heading)
		if from == "" || to == "" || from == to {
			b.lostLinks++
			continue
		}
		if !have[from] || !have[to] {
			// One end is archived, or in a file this server no longer holds.
			b.lostLinks++
			continue
		}
		if inSets[to] == nil {
			inSets[to] = map[string]bool{}
		}
		inSets[to][from] = true
		if outSets[from] == nil {
			outSets[from] = map[string]bool{}
		}
		outSets[from][to] = true

		key, forward := makeEdgeKey(from, to)
		ed, ok := links[key]
		if !ok {
			ed = &common.StarmapEdge{From: key.a, To: key.b, Kind: "link", Broken: true}
			links[key] = ed
		}
		if forward {
			ed.Count++
		} else {
			ed.Back++
		}
		ed.Both = ed.Count > 0 && ed.Back > 0
		if !l.Broken {
			ed.Broken = false
		}
	}
	for _, ed := range links {
		b.edges = append(b.edges, *ed)
	}
	for i := range b.nodes {
		b.nodes[i].In = len(inSets[b.nodes[i].Id])
		b.nodes[i].Out = len(outSets[b.nodes[i].Id])
	}

	// A stable order: the file, then the line inside it. The client seeds its
	// layout off the index a star arrived at, so an order that changed between
	// two requests would scramble the galaxy on every refresh.
	sort.SliceStable(b.nodes, func(x, y int) bool {
		if b.nodes[x].Filename != b.nodes[y].Filename {
			return b.nodes[x].Filename < b.nodes[y].Filename
		}
		if b.nodes[x].Kind != b.nodes[y].Kind {
			return b.nodes[x].Kind == "file"
		}
		return b.nodes[x].Level < b.nodes[y].Level
	})
	sort.SliceStable(b.edges, func(x, y int) bool {
		if b.edges[x].Kind != b.edges[y].Kind {
			return b.edges[x].Kind < b.edges[y].Kind
		}
		if b.edges[x].From != b.edges[y].From {
			return b.edges[x].From < b.edges[y].From
		}
		return b.edges[x].To < b.edges[y].To
	})

	// The newest meals first, because "what just went in" is the interesting
	// end of that list.
	sort.SliceStable(eaten, func(x, y int) bool { return eaten[x].date > eaten[y].date })
	for i, e := range eaten {
		if i >= eatenNamed {
			break
		}
		b.eaten = append(b.eaten, e.name)
	}
	return b
}

// ---------------------------------------------------------------------------
// The request
// ---------------------------------------------------------------------------

// What a filter keeps.
type starFilter struct {
	// Top-level folders (at file scope) or files (at heading scope) to leave
	// out, lower cased.
	exclude map[string]bool
	// An org query every heading has to satisfy. Nil when none was asked for.
	// File stars are never asked - the query language is about headings, and a
	// file that failed it would take its whole constellation with it.
	query *Expr
	// Leave the log stars out. A database with a day page per day for four years
	// is mostly day pages, and a map of it is worth more without them.
	noLogs bool
}

func (self *starFilter) keep(n *common.StarmapNode, sec *org.Section, f *common.OrgFile) bool {
	if self.noLogs && n.Group == "log" {
		return false
	}
	if len(self.exclude) > 0 {
		if self.exclude[strings.ToLower(n.Folder)] || self.exclude[strings.ToLower(n.File)] {
			return false
		}
		if self.exclude[strings.ToLower(filepath.Base(n.Path))] {
			return false
		}
	}
	if self.query != nil && n.Kind == "heading" && sec != nil && f != nil {
		if !EvalString(self.query, sec, f) {
			return false
		}
	}
	return true
}

/* SDOC: API
* GET /starmap — The Database as a Galaxy

	Answers with every file or heading the server holds as a star, every link
	between two of them as a thread, and - at heading scope - the outline as its
	own kind of edge.

	This is not =/links/graph= with more fields on it. The graph is built out of
	the link index, so a heading nothing links to is not in it at all, which in a
	real org database is most of them; a galaxy has to hold every star. It also
	carries what only the server can answer: how big each thing is, when it was
	born, and whether it has been archived.

	Archived files and headings are never stars. They are counted in =Swallowed=
	and the most recent few are named in =Eaten=, which is what a client draws as
	the black hole's diet.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                                          |
	|------------+--------+----------+----------------------------------------------------------------------|
	| =scope=    | string | no       | =heading= (the default) makes every heading a star, with its file as |
	|            |        |          | a hub star and the constellation it belongs to. =file= draws one star |
	|            |        |          | per file, grouped by its top level folder.                           |
	| =query=    | string | no       | An org query expression every heading star has to satisfy. File stars |
	|            |        |          | are never asked - a file that failed it would take its whole          |
	|            |        |          | constellation with it.                                               |
	| =exclude=  | string | no       | Comma separated folder or file names to leave out                    |
	| =nologs=   | string | no       | =t= to leave out journal, daily and session stars                    |
	| =limit=    | number | no       | Stars to hand over (default 4000, capped at 20000)                   |

	*Response:* A =StarmapResult= JSON object.
	| Field          | Type   | Description                                                   |
	|----------------+--------+---------------------------------------------------------------|
	| =Ok=           | bool   | False when the query does not parse                           |
	| =Msg=          | string | Why it does not parse                                         |
	| =Scope=        | string | The scope that was used                                       |
	| =Nodes=        | array  | The stars                                                     |
	| =Edges=        | array  | The threads, each =Kind= either =link= or =outline=            |
	| =Total=        | number | How many stars matched before =limit= was applied             |
	| =Truncated=    | bool   | Whether it was                                                |
	| =Swallowed=    | number | Archived files and headings, which are never stars            |
	| =Eaten=        | array  | The most recently archived, by name                           |
	| =SkippedLinks= | number | Links with no far end to draw in this scope                   |

	A star carries =Id= ("file:<path>" or "node:<hash>", the same name spaces the
	link graph uses), =Kind=, =Label=, =Filename=, =Hash=, =Olp=, =Level=,
	=Parent=, =Line=, =Folder=, =File=, =Path=, =Group= (=note= or =log=), =Size=,
	=Lines=, =Date=, =Dated=, =Touched=, =Status=, =Tags=, =In= and =Out=.

	=Folder= and =File= are the two things a constellation can be made of, and
	neither is called the constellation: which one is right depends on the
	database rather than on the data - one folder of thirty files wants the file,
	four hundred day pages under =journal/= want the folder - and nothing here
	can tell which somebody is looking at. The client decides.

	=Dated= says the date was *written* - a =CREATED= property, a date at the
	front of the name, a =CLOSED= stamp - rather than read off the filesystem. A
	modification time moves whenever anything is edited, so a client marking
	recent things has to know which it has.
EDOC */
func RequestStarmap(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	w.Header().Set("Content-Type", "application/json")

	scope := "heading"
	if strings.EqualFold(r.URL.Query().Get("scope"), "file") {
		scope = "file"
	}
	out := common.StarmapResult{
		Ok: true, Scope: scope,
		Nodes: []common.StarmapNode{}, Edges: []common.StarmapEdge{},
		Eaten: []string{},
	}

	filter := starFilter{exclude: map[string]bool{}}
	for _, x := range strings.Split(r.URL.Query().Get("exclude"), ",") {
		if x = strings.ToLower(strings.TrimSpace(x)); x != "" {
			filter.exclude[x] = true
		}
	}
	filter.noLogs = r.URL.Query().Get("nologs") == "t"
	if q := strings.TrimSpace(r.URL.Query().Get("query")); q != "" {
		// A half-typed query is an ordinary state of a box somebody is typing
		// in, so it answers 200 with Ok false and a reason - the same rule
		// /files/search follows for a half-typed pattern.
		sq := common.StringQuery{Query: q}
		exp, err := ParseString(&sq)
		if err != nil {
			out.Ok = false
			out.Msg = err.Error()
			json.NewEncoder(w).Encode(out)
			return
		}
		filter.query = exp
	}

	limit := defaultStarLimit
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = n
	}
	if limit > maxStarLimit {
		limit = maxStarLimit
	}

	base := starmapFor(scope)
	out.Swallowed = base.swallowed
	out.Eaten = append(out.Eaten, base.eaten...)
	out.SkippedLinks = base.lostLinks

	kept := map[string]bool{}
	for i := range base.nodes {
		n := &base.nodes[i]
		if !filter.keep(n, base.sectionOf[n.Id], base.fileOf[n.Id]) {
			continue
		}
		out.Total++
		if len(out.Nodes) >= limit {
			out.Truncated = true
			continue
		}
		out.Nodes = append(out.Nodes, *n)
		kept[n.Id] = true
	}
	// A star whose parent did not survive hangs off the nearest ancestor that
	// did, rather than being cut loose. A query that keeps a level-three heading
	// and not its level-two parent is the common case, not a mistake, and either
	// other answer - orphan it, or drop it too - loses the shape of the outline.
	parentOf := map[string]string{}
	for i := range base.nodes {
		parentOf[base.nodes[i].Id] = base.nodes[i].Parent
	}
	for i := range out.Nodes {
		p := out.Nodes[i].Parent
		for p != "" && !kept[p] {
			p = parentOf[p]
		}
		out.Nodes[i].Parent = p
	}

	for _, e := range base.edges {
		if e.Kind == "outline" {
			continue // rebuilt below, against the parents that survived
		}
		if !kept[e.From] || !kept[e.To] {
			out.SkippedLinks++
			continue
		}
		out.Edges = append(out.Edges, e)
	}
	for i := range out.Nodes {
		if p := out.Nodes[i].Parent; p != "" {
			out.Edges = append(out.Edges, common.StarmapEdge{From: p, To: out.Nodes[i].Id, Kind: "outline"})
		}
	}

	json.NewEncoder(w).Encode(out)
}
