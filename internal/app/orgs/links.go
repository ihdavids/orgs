package orgs

/* SDOC: Links
* Backlinks And The Link Graph

  The link endpoints answer the question "what points at this?". Every
  =[[target][description]]= link in every org file the server holds is walked
  out of the parsed document, resolved against the database, and indexed in
  both directions. From that index a client can ask for the backlinks of a
  single file or for the graph of links around it.

  A link is resolved the way org mode resolves one:

  | Written as              | Resolves to                                        |
  |-------------------------+----------------------------------------------------|
  | =[[id:UUID]]=           | The heading with that =ID= or =CUSTOM_ID= property |
  | =[[file:notes.org]]=    | That file                                          |
  | =[[file:n.org::*Plans]]=| The heading "Plans" in that file                   |
  | =[[file:n.org::#tag]]=  | The heading with =CUSTOM_ID: tag= in that file      |
  | =[[*Plans]]=            | The heading "Plans" in the same file               |
  | =[[#tag]]=              | The =CUSTOM_ID= in the same file                   |
  | =[[./other.org]]=       | That file, relative to the linking file            |
  | =[[Plans]]=             | A heading of that name in the same file            |
  | =[[https://...]]=       | Nothing, it is an external link                    |

  A link that names an org target the database does not hold is reported with
  =Broken= set, so a client can show it without pretending it resolved. An
  external link is never broken.

  The index is rebuilt whenever the database reloads a file, and cached against
  the reload counter in between, so repeated requests do not rewalk every file.
EDOC */

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// A link protocol has to look like one. This keeps a fuzzy link that happens to
// contain a colon, like [[Release 1: the plan]], from being read as a link to
// the "Release 1" protocol.
var linkProtocolRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*$`)

// Protocols we know go outside the org files. Anything else with a protocol we
// cannot resolve is reported as external too, just never as broken.
var externalProtocols = map[string]bool{
	"http": true, "https": true, "ftp": true, "ftps": true, "mailto": true,
	"news": true, "doi": true, "elisp": true, "shell": true, "info": true,
	"man": true, "help": true, "irc": true, "rmail": true, "mhe": true,
	"gnus": true, "bbdb": true, "bibtex": true, "docview": true, "eww": true,
	"w3m": true, "javascript": true, "data": true, "magnet": true, "tel": true,
	"sms": true, "attachment": true, "denote": true, "roam": true,
}

// One link, plus where it was written, before resolution.
type foundLink struct {
	raw  string
	desc string
	line int
	sec  *org.Section // nil when the link sits in the files preamble
}

// Everything the link endpoints serve, built in one pass over the database.
type linkIndex struct {
	reload uint64
	links  []common.OrgLink
	// Indices into links, so the same link can be looked up from either end.
	fromFile map[string][]int // links written in this file
	toFile   map[string][]int // links pointing at this file or a heading in it
}

var (
	linkCache     *linkIndex
	linkCacheLock sync.Mutex
)

// ---------------------------------------------------------------------------
// Walking the document for links
// ---------------------------------------------------------------------------

// Collect every link in a run of nodes. Nested headlines are skipped, as the
// section they belong to is walked on its own and would otherwise be counted
// twice. GetChildren does not reach into a few node types that can hold links,
// so those are descended explicitly.
func collectLinks(nodes []org.Node, out *[]foundLink, sec *org.Section) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		switch t := n.(type) {
		case *org.Headline, org.Headline:
			// Its own section covers it.
			continue
		case org.RegularLink:
			addLink(t, out, sec)
			continue
		case *org.RegularLink:
			addLink(*t, out, sec)
			continue
		case *org.Table:
			collectTableLinks(t, out, sec)
			continue
		case org.Table:
			collectTableLinks(&t, out, sec)
			continue
		case org.NodeWithMeta:
			collectLinks([]org.Node{t.Node}, out, sec)
			collectMetaLinks(t.Meta, out, sec)
			continue
		case *org.NodeWithMeta:
			collectLinks([]org.Node{t.Node}, out, sec)
			collectMetaLinks(t.Meta, out, sec)
			continue
		case org.DescriptiveListItem:
			collectLinks(t.Term, out, sec)
			collectLinks(t.Details, out, sec)
			continue
		case *org.DescriptiveListItem:
			collectLinks(t.Term, out, sec)
			collectLinks(t.Details, out, sec)
			continue
		case org.Result:
			collectLinks([]org.Node{t.Node}, out, sec)
			continue
		case *org.Result:
			collectLinks([]org.Node{t.Node}, out, sec)
			continue
		}
		collectLinks(n.GetChildren(), out, sec)
	}
}

func addLink(l org.RegularLink, out *[]foundLink, sec *org.Section) {
	raw := strings.TrimSpace(l.URL)
	if raw == "" {
		return
	}
	desc := ""
	if l.Description != nil {
		desc = strings.TrimSpace(org.String(l.Description...))
	}
	*out = append(*out, foundLink{raw: raw, desc: desc, line: l.Pos.Row, sec: sec})
	// A description can hold a link of its own.
	collectLinks(l.Description, out, sec)
}

func collectTableLinks(t *org.Table, out *[]foundLink, sec *org.Section) {
	for _, row := range t.Rows {
		if row == nil {
			continue
		}
		for _, col := range row.Columns {
			if col == nil {
				continue
			}
			collectLinks(col.Children, out, sec)
		}
	}
}

func collectMetaLinks(m org.Metadata, out *[]foundLink, sec *org.Section) {
	for _, caption := range m.Caption {
		collectLinks(caption, out, sec)
	}
}

// Walk a section and every section under it.
func collectSectionLinks(sec *org.Section, out *[]foundLink) {
	if sec.Headline != nil {
		collectLinks(sec.Headline.Title, out, sec)
		collectLinks(sec.Headline.Children, out, sec)
		for _, d := range sec.Headline.Drawers {
			if d != nil {
				collectLinks(d.Children, out, sec)
			}
		}
		for _, t := range sec.Headline.Tables {
			if t != nil {
				collectTableLinks(t, out, sec)
			}
		}
	}
	for _, c := range sec.Children {
		collectSectionLinks(c, out)
	}
}

// Every section of a file, in document order.
func flattenSections(f *common.OrgFile) []*org.Section {
	var out []*org.Section
	if f == nil || f.Doc == nil {
		return out
	}
	var walk func(secs []*org.Section)
	walk = func(secs []*org.Section) {
		for _, s := range secs {
			out = append(out, s)
			walk(s.Children)
		}
	}
	walk(f.Doc.Outline.Children)
	sort.SliceStable(out, func(a, b int) bool {
		return headlineRow(out[a]) < headlineRow(out[b])
	})
	return out
}

func headlineRow(s *org.Section) int {
	if s == nil || s.Headline == nil {
		return -1
	}
	return s.Headline.Pos.Row
}

// Links that sit at the top level of the document. Those written before the
// first heading belong to the file itself, but the parser also ends a headlines
// body at a drawer written in column zero, which leaves the rest of that
// heading sitting at the top of the document rather than under its headline.
// Giving every top level node to the last heading that starts above it keeps
// those links attributed to the heading they were written under instead of
// losing them.
func collectTopLevelLinks(f *common.OrgFile, secs []*org.Section, out *[]foundLink) {
	for _, n := range f.Doc.Nodes {
		if n == nil {
			continue
		}
		switch n.(type) {
		case *org.Headline, org.Headline:
			continue
		}
		row := n.GetPos().Row
		var owner *org.Section
		for _, s := range secs {
			if headlineRow(s) > row {
				break
			}
			owner = s
		}
		collectLinks([]org.Node{n}, out, owner)
	}
}

func collectFileLinks(f *common.OrgFile) []foundLink {
	var out []foundLink
	if f == nil || f.Doc == nil {
		return out
	}
	collectTopLevelLinks(f, flattenSections(f), &out)
	for _, sec := range f.Doc.Outline.Children {
		collectSectionLinks(sec, &out)
	}
	return out
}

// ---------------------------------------------------------------------------
// Resolving a link target
// ---------------------------------------------------------------------------

// Lookup tables for turning a written path into a filename the database holds.
type fileLookup struct {
	byPath map[string]string   // cleaned absolute path -> filename as the db knows it
	byBase map[string][]string // base name -> filenames
}

func buildFileLookup(db *OrgDb) *fileLookup {
	res := &fileLookup{byPath: map[string]string{}, byBase: map[string][]string{}}
	for _, fname := range db.GetFiles() {
		if abs, err := filepath.Abs(fname); err == nil {
			res.byPath[filepath.Clean(abs)] = fname
		}
		res.byPath[filepath.Clean(fname)] = fname
		base := filepath.Base(fname)
		res.byBase[base] = append(res.byBase[base], fname)
	}
	return res
}

// Try hard to turn the path written in a link into a file the database holds.
// Returns "" when nothing matches.
func (self *fileLookup) find(path string, fromFile string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	var tries []string
	if filepath.IsAbs(path) {
		tries = append(tries, path)
	} else {
		if fromFile != "" {
			if abs, err := filepath.Abs(fromFile); err == nil {
				tries = append(tries, filepath.Join(filepath.Dir(abs), path))
			}
			tries = append(tries, filepath.Join(filepath.Dir(fromFile), path))
		}
		for _, dir := range Conf().Server.OrgDirs {
			tries = append(tries, filepath.Join(dir, path))
			if abs, err := filepath.Abs(dir); err == nil {
				tries = append(tries, filepath.Join(abs, path))
			}
		}
		tries = append(tries, path)
	}
	for _, t := range tries {
		if f, ok := self.byPath[filepath.Clean(t)]; ok {
			return f
		}
		if abs, err := filepath.Abs(t); err == nil {
			if f, ok := self.byPath[filepath.Clean(abs)]; ok {
				return f
			}
		}
	}
	// Last resort, a bare name. Only when it is unambiguous.
	if base := filepath.Base(path); base == path {
		if list, ok := self.byBase[base]; ok && len(list) == 1 {
			return list[0]
		}
	}
	return ""
}

// Build the endpoint for a heading.
func endForSection(db *OrgDb, sec *org.Section, line int) common.LinkEnd {
	end := common.LinkEnd{
		Hash:     sec.Hash,
		Headline: common.GetHeadlineTitle(sec.Headline),
		Olp:      outlinePath(sec),
		Line:     line,
	}
	if sec.Headline != nil {
		end.Level = sec.Headline.Lvl
		if line < 0 {
			end.Line = sec.Headline.Pos.Row
		}
	}
	if f := db.FileFromSection(sec); f != nil {
		end.Filename = f.Filename
	}
	return end
}

// The same, for a heading we already know the file of.
func endForSectionIn(db *OrgDb, sec *org.Section, f *common.OrgFile) common.LinkEnd {
	end := endForSection(db, sec, -1)
	if end.Filename == "" && f != nil {
		end.Filename = f.Filename
	}
	return end
}

// Outermost heading first, the heading itself last.
func outlinePath(sec *org.Section) []string {
	var rev []string
	for s := sec; s != nil; s = s.Parent {
		if s.Headline == nil {
			continue
		}
		t := common.GetHeadlineTitle(s.Headline)
		if t == "" {
			continue
		}
		rev = append(rev, t)
	}
	out := make([]string, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		out = append(out, rev[i])
	}
	return out
}

// Find a heading by its text in one file. Exact match wins, then a prefix, then
// a substring, so "[[*Plans]]" finds "Plans for next year" when nothing is
// called exactly "Plans".
func findHeadingByText(f *common.OrgFile, text string) *org.Section {
	text = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(text, "*")))
	if text == "" || f == nil || f.Doc == nil {
		return nil
	}
	var exact, prefix, contains *org.Section
	var walk func(secs []*org.Section)
	walk = func(secs []*org.Section) {
		for _, s := range secs {
			title := strings.ToLower(strings.TrimSpace(common.GetHeadlineTitle(s.Headline)))
			if title == text && exact == nil {
				exact = s
			} else if strings.HasPrefix(title, text) && prefix == nil {
				prefix = s
			} else if title != "" && strings.Contains(title, text) && contains == nil {
				contains = s
			}
			walk(s.Children)
		}
	}
	walk(f.Doc.Outline.Children)
	if exact != nil {
		return exact
	}
	if prefix != nil {
		return prefix
	}
	return contains
}

// An ID or CUSTOM_ID, and the heading it names.
type secRef struct {
	sec  *org.Section
	file *common.OrgFile
}

// The id lookups the link resolver uses. The database has its own, but it only
// holds sections a query has already touched, and it only sees a property
// drawer the parser managed to attach to its headline. A drawer written in
// column zero is not attached, so the ids in it are indexed here by the heading
// it was written under.
type idIndex struct {
	byId       map[string]secRef            // ID, global across files the way org treats it
	byCustomId map[string]map[string]secRef // filename -> lowercased CUSTOM_ID
}

func newIdIndex() *idIndex {
	return &idIndex{byId: map[string]secRef{}, byCustomId: map[string]map[string]secRef{}}
}

func (self *idIndex) add(f *common.OrgFile, sec *org.Section, props [][]string) {
	for _, p := range props {
		if len(p) < 2 {
			continue
		}
		key, val := strings.ToUpper(strings.TrimSpace(p[0])), strings.TrimSpace(p[1])
		if val == "" {
			continue
		}
		switch key {
		case "ID":
			if _, have := self.byId[val]; !have {
				self.byId[val] = secRef{sec, f}
			}
		case "CUSTOM_ID":
			m := self.byCustomId[f.Filename]
			if m == nil {
				m = map[string]secRef{}
				self.byCustomId[f.Filename] = m
			}
			low := strings.ToLower(val)
			if _, have := m[low]; !have {
				m[low] = secRef{sec, f}
			}
		}
	}
}

// Index every id in a file, from the drawers the parser attached and from the
// ones it left at the top of the document.
func (self *idIndex) addFile(f *common.OrgFile, secs []*org.Section) {
	for _, sec := range secs {
		if sec.Headline != nil && sec.Headline.Properties != nil {
			self.add(f, sec, sec.Headline.Properties.Properties)
		}
	}
	for _, n := range f.Doc.Nodes {
		pd, ok := n.(*org.PropertyDrawer)
		if !ok {
			if v, vok := n.(org.PropertyDrawer); vok {
				pd = &v
			} else {
				continue
			}
		}
		row := pd.Pos.Row
		var owner *org.Section
		for _, sec := range secs {
			if headlineRow(sec) > row {
				break
			}
			owner = sec
		}
		if owner != nil {
			self.add(f, owner, pd.Properties)
		}
	}
}

func (self *idIndex) findId(id string) (secRef, bool) {
	id = strings.TrimSpace(id)
	r, ok := self.byId[id]
	return r, ok
}

func (self *idIndex) findCustomId(f *common.OrgFile, id string) (secRef, bool) {
	if f == nil {
		return secRef{}, false
	}
	id = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(id, "#")))
	r, ok := self.byCustomId[f.Filename][id]
	return r, ok
}

// The section a line number falls inside, so "file.org::42" lands somewhere
// sensible rather than nowhere.
func findHeadingByLine(f *common.OrgFile, line int) *org.Section {
	if f == nil || f.Doc == nil {
		return nil
	}
	var best *org.Section
	var walk func(secs []*org.Section)
	walk = func(secs []*org.Section) {
		for _, s := range secs {
			if s.Headline != nil && s.Headline.Pos.Row <= line {
				if best == nil || s.Headline.Pos.Row >= best.Headline.Pos.Row {
					best = s
				}
			}
			walk(s.Children)
		}
	}
	walk(f.Doc.Outline.Children)
	return best
}

// Split "file.org::*Heading" into its path and its search part.
func splitSearch(rest string) (string, string) {
	if i := strings.Index(rest, "::"); i >= 0 {
		return rest[:i], rest[i+2:]
	}
	return rest, ""
}

// Pull the protocol off a link target, if it really has one.
func splitProtocol(raw string) (string, string) {
	if i := strings.Index(raw, ":"); i > 0 {
		if p := raw[:i]; linkProtocolRe.MatchString(p) {
			return strings.ToLower(p), raw[i+1:]
		}
	}
	return "", raw
}

// Apply the search part of a file link, landing on a heading when it names one.
func applySearch(db *OrgDb, ids *idIndex, f *common.OrgFile, search string, end *common.LinkEnd) string {
	search = strings.TrimSpace(search)
	if search == "" {
		return "file"
	}
	switch {
	case strings.HasPrefix(search, "*"):
		if sec := findHeadingByText(f, search); sec != nil {
			*end = endForSectionIn(db, sec, f)
			return "heading"
		}
		return ""
	case strings.HasPrefix(search, "#"):
		if r, ok := ids.findCustomId(f, search); ok {
			*end = endForSectionIn(db, r.sec, f)
			return "custom-id"
		}
		return ""
	}
	if n, err := strconv.Atoi(search); err == nil {
		if sec := findHeadingByLine(f, n-1); sec != nil {
			*end = endForSectionIn(db, sec, f)
		}
		return "file"
	}
	if sec := findHeadingByText(f, search); sec != nil {
		*end = endForSectionIn(db, sec, f)
		return "fuzzy"
	}
	// The file was found even if the search inside it was not.
	return "file"
}

var orgFileRe = regexp.MustCompile(`(?i)\.org(\.gpg)?$`)

// Does this look like a path rather than a heading name?
func looksLikePath(s string) bool {
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") {
		return true
	}
	return orgFileRe.MatchString(s) && !strings.ContainsAny(s, " \t")
}

// Resolve one written link into the thing it points at.
func resolveLink(db *OrgDb, ids *idIndex, lookup *fileLookup, raw string, fromFile string) (common.LinkEnd, string, bool) {
	var end common.LinkEnd
	proto, rest := splitProtocol(raw)

	resolveFile := func(path, search string, kind string) (common.LinkEnd, string, bool) {
		fname := lookup.find(path, fromFile)
		if fname == "" {
			// Not an org file we hold. A link to a pdf or an image is not a
			// broken org link, it just points outside the database.
			if orgFileRe.MatchString(path) || path == "" {
				return end, kind, true
			}
			return end, "external", false
		}
		f := db.FindByFile(fname)
		end.Filename = fname
		got := applySearch(db, ids, f, search, &end)
		if got == "" {
			// The file is there, the heading inside it is not.
			end = common.LinkEnd{Filename: fname}
			return end, kind, true
		}
		return end, got, false
	}

	switch {
	case proto == "id":
		id := strings.TrimSpace(rest)
		if r, ok := ids.findId(id); ok {
			return endForSectionIn(db, r.sec, r.file), "id", false
		}
		if sec := db.FindByAnyId(id); sec != nil {
			return endForSection(db, sec, -1), "id", false
		}
		return end, "id", true

	case proto == "file":
		path, search := splitSearch(rest)
		return resolveFile(path, search, "file")

	case proto != "" && externalProtocols[proto]:
		return end, "external", false

	case proto != "":
		// Some protocol we do not know how to follow.
		return end, "external", false

	case strings.HasPrefix(rest, "*"):
		f := db.FindByFile(fromFile)
		if sec := findHeadingByText(f, rest); sec != nil {
			return endForSectionIn(db, sec, f), "heading", false
		}
		return end, "heading", true

	case strings.HasPrefix(rest, "#"):
		f := db.FindByFile(fromFile)
		if r, ok := ids.findCustomId(f, rest); ok {
			return endForSectionIn(db, r.sec, f), "custom-id", false
		}
		return end, "custom-id", true

	case looksLikePath(rest):
		path, search := splitSearch(rest)
		return resolveFile(path, search, "file")
	}

	// A bare fuzzy target. Try a heading in this file, then an id anywhere.
	f := db.FindByFile(fromFile)
	if sec := findHeadingByText(f, rest); sec != nil {
		return endForSectionIn(db, sec, f), "fuzzy", false
	}
	if r, ok := ids.findId(strings.TrimSpace(rest)); ok {
		return endForSectionIn(db, r.sec, r.file), "id", false
	}
	if sec := db.FindByAnyId(strings.TrimSpace(rest)); sec != nil {
		return endForSection(db, sec, -1), "id", false
	}
	return end, "unresolved", true
}

// ---------------------------------------------------------------------------
// Building and caching the index
// ---------------------------------------------------------------------------

func buildLinkIndex(db *OrgDb) *linkIndex {
	idx := &linkIndex{
		reload:   db.ReloadIndex,
		fromFile: map[string][]int{},
		toFile:   map[string][]int{},
	}
	lookup := buildFileLookup(db)
	files := db.GetFiles()

	// Sections are registered lazily as queries touch them, so an "id:" link
	// written in the first file could not find a heading in the last one. Walk
	// every file into the registry first, then resolve.
	ids := newIdIndex()
	for _, fname := range files {
		f := db.FindByFile(fname)
		if f == nil || f.Doc == nil {
			continue
		}
		secs := flattenSections(f)
		for _, sec := range secs {
			db.RegisterSection(sec.Hash, sec, f)
		}
		ids.addFile(f, secs)
	}

	for _, fname := range files {
		f := db.FindByFile(fname)
		if f == nil || f.Doc == nil {
			continue
		}
		for _, fl := range collectFileLinks(f) {
			link := common.OrgLink{Raw: fl.raw, Desc: fl.desc}
			link.From = common.LinkEnd{Filename: fname, Line: fl.line}
			if fl.sec != nil {
				link.From = endForSection(db, fl.sec, fl.line)
				// A section whose file lookup failed still belongs to this file.
				if link.From.Filename == "" {
					link.From.Filename = fname
				}
			}
			link.To, link.Kind, link.Broken = resolveLink(db, ids, lookup, fl.raw, fname)
			i := len(idx.links)
			idx.links = append(idx.links, link)
			idx.fromFile[fname] = append(idx.fromFile[fname], i)
			if link.To.Filename != "" {
				idx.toFile[link.To.Filename] = append(idx.toFile[link.To.Filename], i)
			}
		}
	}
	return idx
}

func getLinkIndex() *linkIndex {
	db := GetDb()
	linkCacheLock.Lock()
	defer linkCacheLock.Unlock()
	if linkCache != nil && linkCache.reload == db.ReloadIndex {
		return linkCache
	}
	linkCache = buildLinkIndex(db)
	return linkCache
}

// Resolve whatever a client called the file into the name the database uses.
func resolveRequestedFile(filename string) (string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "", fmt.Errorf("no filename given")
	}
	db := GetDb()
	if f := db.FindByFile(filename); f != nil {
		return f.Filename, nil
	}
	lookup := buildFileLookup(db)
	if fname := lookup.find(filename, ""); fname != "" {
		return fname, nil
	}
	return "", fmt.Errorf("file not found in the database: %s", filename)
}

// ---------------------------------------------------------------------------
// Backlinks for one file
// ---------------------------------------------------------------------------

func QueryBacklinks(filename string) common.Backlinks {
	fname, err := resolveRequestedFile(filename)
	if err != nil {
		return common.Backlinks{Ok: false, Msg: err.Error(), Filename: filename}
	}
	idx := getLinkIndex()
	res := common.Backlinks{Ok: true, Filename: fname}
	for _, i := range idx.toFile[fname] {
		l := idx.links[i]
		if l.From.Filename == fname {
			res.Internal = append(res.Internal, l)
		} else {
			res.In = append(res.In, l)
		}
	}
	for _, i := range idx.fromFile[fname] {
		l := idx.links[i]
		if l.To.Filename != fname {
			res.Out = append(res.Out, l)
		}
	}
	sortLinks(res.In)
	sortLinks(res.Out)
	sortLinks(res.Internal)
	return res
}

func sortLinks(ls []common.OrgLink) {
	sort.SliceStable(ls, func(a, b int) bool {
		if ls[a].From.Filename != ls[b].From.Filename {
			return ls[a].From.Filename < ls[b].From.Filename
		}
		return ls[a].From.Line < ls[b].From.Line
	})
}

// ---------------------------------------------------------------------------
// The graph
// ---------------------------------------------------------------------------

func fileNodeId(fname string) string { return "file:" + fname }
func headNodeId(hash string) string  { return "node:" + hash }

// Which node of the graph does this endpoint belong to? At file scope every
// endpoint collapses onto its file; at heading scope a link that names a
// heading keeps it.
func endNodeId(e common.LinkEnd, headingScope bool) string {
	if e.Filename == "" {
		return ""
	}
	if headingScope && e.Hash != "" {
		return headNodeId(e.Hash)
	}
	return fileNodeId(e.Filename)
}

func nodeForEnd(e common.LinkEnd, headingScope bool) common.LinkGraphNode {
	if headingScope && e.Hash != "" {
		label := e.Headline
		if label == "" {
			label = filepath.Base(e.Filename)
		}
		return common.LinkGraphNode{
			Id: headNodeId(e.Hash), Kind: "heading", Label: label,
			Filename: e.Filename, Hash: e.Hash, Olp: e.Olp, Level: e.Level,
		}
	}
	return common.LinkGraphNode{
		Id: fileNodeId(e.Filename), Kind: "file",
		Label: filepath.Base(e.Filename), Filename: e.Filename,
	}
}

// An undirected pair, so two nodes that link at each other make one edge.
type edgeKey struct{ a, b string }

func makeEdgeKey(from, to string) (edgeKey, bool) {
	if from < to {
		return edgeKey{from, to}, true
	}
	return edgeKey{to, from}, false
}

// Build the graph of links. When filename is empty the whole database is
// returned; otherwise the graph is grown outward from that file for depth hops.
func QueryLinkGraph(filename string, depth int, headingScope bool) common.LinkGraphResult {
	idx := getLinkIndex()
	res := common.LinkGraphResult{Ok: true}

	center := ""
	if filename != "" {
		fname, err := resolveRequestedFile(filename)
		if err != nil {
			return common.LinkGraphResult{Ok: false, Msg: err.Error()}
		}
		center = fileNodeId(fname)
		res.Center = center
	}

	nodes := map[string]*common.LinkGraphNode{}
	edges := map[edgeKey]*common.LinkGraphEdge{}
	// Distinct neighbour sets, so a file linked at five times still counts once.
	inSets := map[string]map[string]bool{}
	outSets := map[string]map[string]bool{}
	// Adjacency, used to walk outward from the center.
	adj := map[string]map[string]bool{}

	touch := func(e common.LinkEnd) string {
		id := endNodeId(e, headingScope)
		if id == "" {
			return ""
		}
		if _, ok := nodes[id]; !ok {
			n := nodeForEnd(e, headingScope)
			nodes[id] = &n
		} else if headingScope && e.Hash != "" && nodes[id].Label == "" {
			nodes[id].Label = e.Headline
		}
		return id
	}

	for _, l := range idx.links {
		if l.To.Filename == "" {
			continue // external or broken, there is nothing to draw at the far end
		}
		from := touch(l.From)
		to := touch(l.To)
		if from == "" || to == "" || from == to {
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
		if adj[from] == nil {
			adj[from] = map[string]bool{}
		}
		if adj[to] == nil {
			adj[to] = map[string]bool{}
		}
		adj[from][to] = true
		adj[to][from] = true

		key, forward := makeEdgeKey(from, to)
		ed, ok := edges[key]
		if !ok {
			ed = &common.LinkGraphEdge{From: key.a, To: key.b, Broken: true}
			edges[key] = ed
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

	// A file the client asked about that nothing links to still deserves to be
	// drawn, alone, rather than coming back as an empty graph.
	if center != "" {
		if _, ok := nodes[center]; !ok {
			fname := strings.TrimPrefix(center, "file:")
			nodes[center] = &common.LinkGraphNode{
				Id: center, Kind: "file", Label: filepath.Base(fname), Filename: fname,
			}
		}
	}

	keep := map[string]int{}
	if center == "" {
		for id := range nodes {
			keep[id] = 0
		}
	} else {
		if depth < 0 {
			depth = 0
		}
		// The center is the file, plus - at heading scope - every heading in it,
		// since a link written under one of those headings is still a link out of
		// the file the user is looking at. Seeding the walk with all of them keeps
		// the hop counts honest, rather than bolting the headings on afterwards
		// and never expanding what they reach.
		frontier := []string{center}
		keep[center] = 0
		if headingScope {
			fname := strings.TrimPrefix(center, "file:")
			for id, n := range nodes {
				if n.Kind == "heading" && n.Filename == fname {
					if _, seen := keep[id]; !seen {
						keep[id] = 0
						frontier = append(frontier, id)
					}
				}
			}
		}
		for d := 1; d <= depth; d++ {
			var next []string
			for _, id := range frontier {
				for n := range adj[id] {
					if _, seen := keep[n]; !seen {
						keep[n] = d
						next = append(next, n)
					}
				}
			}
			frontier = next
			if len(frontier) == 0 {
				break
			}
		}
	}

	for id, d := range keep {
		n := nodes[id]
		if n == nil {
			continue
		}
		n.In = len(inSets[id])
		n.Out = len(outSets[id])
		n.Distance = d
		n.Center = id == center
		res.Nodes = append(res.Nodes, *n)
	}
	for _, ed := range edges {
		if _, ok := keep[ed.From]; !ok {
			continue
		}
		if _, ok := keep[ed.To]; !ok {
			continue
		}
		res.Edges = append(res.Edges, *ed)
	}

	// Stable output: the busiest nodes first, then by label.
	sort.SliceStable(res.Nodes, func(a, b int) bool {
		da, dbb := res.Nodes[a].In+res.Nodes[a].Out, res.Nodes[b].In+res.Nodes[b].Out
		if da != dbb {
			return da > dbb
		}
		return res.Nodes[a].Id < res.Nodes[b].Id
	})
	sort.SliceStable(res.Edges, func(a, b int) bool {
		if res.Edges[a].From != res.Edges[b].From {
			return res.Edges[a].From < res.Edges[b].From
		}
		return res.Edges[a].To < res.Edges[b].To
	})
	return res
}

// ---------------------------------------------------------------------------
// Per file counts for the tree
// ---------------------------------------------------------------------------

func QueryLinkStats() common.LinkStatsResult {
	idx := getLinkIndex()
	res := common.LinkStatsResult{Ok: true}
	for _, fname := range GetDb().GetFiles() {
		stat := common.LinkFileStat{Filename: fname}
		for _, i := range idx.fromFile[fname] {
			l := idx.links[i]
			if l.Broken {
				stat.Broken++
			}
			if l.To.Filename == fname {
				stat.Internal++
			} else if l.To.Filename != "" {
				stat.Out++
			}
		}
		for _, i := range idx.toFile[fname] {
			if idx.links[i].From.Filename != fname {
				stat.In++
			}
		}
		res.Files = append(res.Files, stat)
	}
	return res
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

/*
		SDOC: API

	  - GET /links — Backlinks For One File
	    Returns every link that points at the given file, or at any heading inside it,
	    together with the links written in that file that point somewhere else.

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter  | Type   | Required | Description                                                      |
	    |------------+--------+----------+------------------------------------------------------------------|
	    | =filename= | string | yes      | Path of the file. An absolute path, a path relative to an org    |
	    |            |        |          | directory, or an unambiguous base name all work.                 |

	    *Response:* A =Backlinks= object with three lists of links: =In= (written
	    elsewhere, pointing here), =Out= (written here, pointing elsewhere) and
	    =Internal= (written here, pointing back into this same file). Each link
	    carries both of its ends, the raw target text, its description, the kind of
	    link it is, and whether it is broken.
	    EDOC
*/
func RequestBacklinks(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	res := QueryBacklinks(filename)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

/*
		SDOC: API

	  - GET /links/graph — The Link Graph
	    Returns the graph of links between org files, or between headings when asked
	    for. Two nodes that link at each other come back as a single edge with =Both=
	    set, so a bidirectional link draws as one stroke.

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter  | Type   | Required | Description                                                      |
	    |------------+--------+----------+------------------------------------------------------------------|
	    | =filename= | string | no       | Center the graph on this file. Omit for the whole database.      |
	    | =depth=    | int    | no       | How many hops out from the center to include. Defaults to 2.     |
	    | =scope=    | string | no       | =file= (the default) collapses every link onto its file.         |
	    |            |        |          | =heading= keeps the heading a link names as its own node.        |

	    *Response:* A =LinkGraphResult= holding the nodes and edges to draw. Each
	    node carries its in and out degree and how many hops it sits from the center.
	    EDOC
*/
func RequestLinkGraph(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	depth := 2
	if d := r.URL.Query().Get("depth"); d != "" {
		if n, err := strconv.Atoi(d); err == nil {
			depth = n
		}
	}
	headingScope := strings.EqualFold(r.URL.Query().Get("scope"), "heading")
	res := QueryLinkGraph(filename, depth, headingScope)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

/*
		SDOC: API

	  - GET /links/stats — Link Counts Per File
	    Returns the number of links into, out of, and within every org file the server
	    holds, plus how many links written in each file resolve to nothing. Intended
	    for decorating a file list without asking for the whole graph.

	    *Method:* =GET=

	    *Response:* A =LinkStatsResult= holding one =LinkFileStat= per file.
	    EDOC
*/
func RequestLinkStats(w http.ResponseWriter, r *http.Request) {
	res := QueryLinkStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
