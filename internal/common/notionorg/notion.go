// Package notionorg turns a Notion export ("Markdown & CSV") into org files.
//
// It runs on the client (`orgs import Export.zip`) and needs nothing but the
// export's files, by path. The work is done by internal/common/mdorg: the
// export is made into a markdown vault - ids stripped from every name, links
// rewritten to the new names, databases turned into one note each - and
// converted as one, so a Notion import links and copies pictures exactly as
// a vault import does.
//
// Layout: one org file per page, in Notion's own folders. A page's sub-pages
// are files in a folder named after it (Home.org, Home/Garden.org), not child
// headings, because a Notion sub-page is a whole page - its own links,
// pictures, databases and sub-pages - and a workspace is often a wiki whose
// top page has hundreds of them: as headings that would be one enormous
// file, and every link into a sub-page would become a heading search that
// breaks when the title changes. Kept as files, a page is still one file you
// can open, and the folder tree is the one people knew in Notion.
//
// A database is one org file of its own (Reading List.org beside the page
// that held it), a heading per row: the row's page body under it, the CSV's
// columns as its property drawer, and :RECORD: <database> so the rows are a
// records collection (orgs rec, worg's records view) - which is what a
// Notion database is: things with typed fields and notes.
package notionorg

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/internal/common/mdorg"
)

// Converted is the export as org.
type Converted struct {
	// Org text by path, relative to where the export goes.
	Files map[string]string
	// The pictures and files the org links to, by the path they go to.
	Assets   map[string][]byte
	Warnings []string
	Stats    Stats
}

// Stats counts what a conversion found.
type Stats struct {
	Pages, Databases, Rows, Tasks, Links, Unresolved int
}

// A Notion id ends every page, database and folder name: 32 hex digits after
// a space (older exports), straight after the title (some newer ones), or as
// a dashed uuid.
var idRe = regexp.MustCompile(`^(.*?) ?([0-9a-fA-F]{32}|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// splitID is a name without its id, and the id (lower case, no dashes).
func splitID(stem string) (string, string) {
	m := idRe.FindStringSubmatch(stem)
	if m == nil {
		return stem, ""
	}
	return strings.TrimSpace(m[1]), strings.ToLower(strings.ReplaceAll(m[2], "-", ""))
}

// stemOf is a file's name without its extension, and the extension; a
// database's complete CSV (Name <id>_all.csv) has the stem its other CSV has.
func stemOf(name string) (string, string) {
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if strings.EqualFold(ext, ".csv") {
		stem = strings.TrimSuffix(stem, "_all")
	}
	return stem, ext
}

// IsExport says whether a tree of files is a Notion export: some page or
// database in it is named with Notion's id.
func IsExport(paths []string) bool {
	for _, p := range paths {
		ext := strings.ToLower(path.Ext(p))
		if ext != ".md" && ext != ".csv" {
			continue
		}
		stem, _ := stemOf(path.Base(p))
		if _, id := splitID(stem); id != "" {
			return true
		}
	}
	return false
}

type db struct {
	title   string
	folder  string // the rows' folder, as exported
	csvPath string
	stub    string // the vault note the database becomes (cleaned .md path)
	page    string // a page exported with the same name, whose text heads the file
	header  []string
	rows    []*row
	// Which column (if any) is the rows' todo state.
	status, done int
}

type row struct {
	title string
	cells []string // nil for a row page the CSV does not have
	extra [][2]string
	md    string // its page, as exported; "" for a row with no page
	synth string // the vault note its body is converted as
}

type export struct {
	files  map[string][]byte
	dirs   map[string]bool
	stems  map[[2]string]string // (folder as exported, stem) → name without the id
	byID   map[string]string    // id → path as exported
	dbs    []*db
	dbOf   map[string]*db // csv, rows' folder or page path → its database
	rowOf  map[string]*row
	assets map[string]bool // cleaned paths property links point at
	stats  *Stats
}

// Convert turns an export into org files. files is every file of the export
// by its slash path, as ReadZip gives them or as read from its folder.
func Convert(files map[string][]byte) Converted {
	files = unwrap(files)
	out := Converted{Files: map[string]string{}, Assets: map[string][]byte{}}
	e := &export{files: files, dirs: map[string]bool{}, stems: map[[2]string]string{}, byID: map[string]string{},
		dbOf: map[string]*db{}, rowOf: map[string]*row{}, assets: map[string]bool{}, stats: &out.Stats}
	e.name()
	e.databases(&out)

	// The vault: every page, a note per database, a note per row body.
	v := mdorg.Vault{}
	cleanToOrig := map[string]string{}
	paths := sortedKeys(files)
	for _, p := range paths {
		switch strings.ToLower(path.Ext(p)) {
		case ".md":
			if e.rowOf[p] != nil || e.dbOf[p] != nil {
				continue
			}
			c := e.clean(p)
			v.Notes = append(v.Notes, mdorg.Note{Path: c, Text: e.page(p, c, nil)})
			out.Stats.Pages++
		case ".csv":
		default:
			c := e.clean(p)
			cleanToOrig[c] = p
			v.Files = append(v.Files, c)
		}
	}
	for _, d := range e.dbs {
		text := "# " + d.title + "\n"
		if d.page != "" {
			text = e.page(d.page, d.stub, nil)
		}
		v.Notes = append(v.Notes, mdorg.Note{Path: d.stub, Text: text})
		for _, r := range d.rows {
			if r.md != "" {
				v.Notes = append(v.Notes, mdorg.Note{Path: r.synth, Text: e.page(r.md, r.synth, r)})
			}
		}
	}
	conv := mdorg.Convert(v)

	for _, n := range v.Notes {
		org := mdorg.OrgPath(n.Path)
		if strings.Contains(n.Path, rowMark) {
			continue
		}
		out.Files[org] = conv.Files[org]
	}
	for _, d := range e.dbs {
		org := mdorg.OrgPath(d.stub)
		out.Files[org] = strings.TrimRight(out.Files[org], "\n") + "\n" + e.rowsOrg(d, conv.Files)
	}
	for _, a := range conv.Assets {
		e.assets[a] = true
	}
	for a := range e.assets {
		if o, ok := cleanToOrig[a]; ok {
			out.Assets[a] = files[o]
		}
	}
	for _, w := range conv.Warnings {
		if !strings.Contains(w, "links name a note or file") {
			out.Warnings = append(out.Warnings, w)
		}
	}
	out.Stats.Unresolved += conv.Stats.Unresolved
	if out.Stats.Unresolved > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%d links name a page or file that is not in the export; they point where it would be", out.Stats.Unresolved))
	}
	return out
}

// rowMark is in the name of the vault note a row's body is converted as.
const rowMark = ".notion-row-"

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// unwrap takes off a folder everything sits in (Notion zips the export into
// Export-<uuid>/), so the files are named from where the pages start. Even a
// page's own folder alone is no loss: its .md is not beside it to be linked.
func unwrap(files map[string][]byte) map[string][]byte {
	for {
		first := ""
		for p := range files {
			seg, _, ok := strings.Cut(p, "/")
			if !ok || first != "" && seg != first {
				return files
			}
			first = seg
		}
		if first == "" {
			return files
		}
		next := map[string][]byte{}
		for p, b := range files {
			next[strings.TrimPrefix(p, first+"/")] = b
		}
		files = next
	}
}

// name works out every file's and folder's name without its id. A page, its
// folder and (for a database) its CSVs share a stem, so they share the new
// name; two pages of one title in one folder become "Notes" and "Notes 2".
func (e *export) name() {
	keys := map[string]map[string]bool{} // folder → stems in it
	add := func(dir, stem string) {
		if keys[dir] == nil {
			keys[dir] = map[string]bool{}
		}
		keys[dir][stem] = true
	}
	for p := range e.files {
		segs := strings.Split(p, "/")
		for i, s := range segs {
			dir := strings.Join(segs[:i], "/")
			if i < len(segs)-1 {
				e.dirs[strings.Join(segs[:i+1], "/")] = true
				add(dir, s)
			} else {
				st, _ := stemOf(s)
				add(dir, st)
			}
		}
	}
	for dir, stems := range keys {
		used := map[string]bool{}
		withID := []string{}
		for s := range stems {
			if _, id := splitID(s); id == "" {
				used[strings.ToLower(s)] = true
				e.stems[[2]string{dir, s}] = s
			} else {
				withID = append(withID, s)
			}
		}
		sort.Slice(withID, func(i, j int) bool {
			a, _ := splitID(withID[i])
			b, _ := splitID(withID[j])
			if a != b {
				return a < b
			}
			return withID[i] < withID[j]
		})
		for _, s := range withID {
			t, _ := splitID(s)
			t = strings.Trim(t, " .")
			if t == "" {
				t = "Untitled"
			}
			n := t
			for i := 2; used[strings.ToLower(n)]; i++ {
				n = fmt.Sprintf("%s %d", t, i)
			}
			used[strings.ToLower(n)] = true
			e.stems[[2]string{dir, s}] = n
		}
	}
	// Ids, for links that name a page by it (notion.so urls, a moved file).
	// A page's .md wins over its folder, a database's CSV over its folder.
	rank := func(p string) int {
		switch strings.ToLower(path.Ext(p)) {
		case ".md":
			return 3
		case ".csv":
			return 2
		}
		if e.dirs[p] {
			return 1
		}
		return 0
	}
	for _, p := range append(sortedKeys(e.files), sortedKeys(e.dirs)...) {
		st, _ := stemOf(path.Base(p))
		if e.dirs[p] {
			st = path.Base(p)
		}
		if _, id := splitID(st); id != "" {
			if old, ok := e.byID[id]; !ok || rank(p) > rank(old) {
				e.byID[id] = p
			}
		}
	}
}

// clean is a path as exported, with the ids gone.
func (e *export) clean(p string) string {
	segs := strings.Split(p, "/")
	out := make([]string, len(segs))
	for i, s := range segs {
		dir := strings.Join(segs[:i], "/")
		if i < len(segs)-1 {
			out[i] = e.stems[[2]string{dir, s}]
			continue
		}
		st, ext := stemOf(s)
		out[i] = e.stems[[2]string{dir, st}] + ext
	}
	return strings.Join(out, "/")
}

// databases reads every CSV: its columns, its rows, and which row pages in
// its folder belong to which row.
func (e *export) databases(out *Converted) {
	byStem := map[string]*db{}
	for _, p := range sortedKeys(e.files) {
		if !strings.EqualFold(path.Ext(p), ".csv") {
			continue
		}
		st, _ := stemOf(path.Base(p))
		folder := path.Join(path.Dir(p), st)
		if path.Dir(p) == "." {
			folder = st
		}
		d := byStem[folder]
		all := strings.HasSuffix(strings.TrimSuffix(path.Base(p), path.Ext(p)), "_all")
		if d == nil {
			title, _ := splitID(st)
			if title == "" {
				title = "Untitled"
			}
			d = &db{title: title, folder: folder, status: -1, done: -1}
			byStem[folder] = d
			e.dbs = append(e.dbs, d)
		}
		// A link may name either CSV.
		e.dbOf[p] = d
		if d.csvPath != "" && !all {
			continue // the _all one is already in, and it is the complete one
		}
		d.csvPath = p
	}
	for _, d := range e.dbs {
		d.stub = e.clean(d.csvPath)
		d.stub = strings.TrimSuffix(d.stub, path.Ext(d.stub)) + ".md"
		if _, ok := e.files[d.folder+".md"]; ok {
			d.page = d.folder + ".md"
			e.dbOf[d.page] = d
		}
		e.dbOf[d.csvPath] = d
		e.dbOf[d.folder] = d
		out.Stats.Databases++
		recs, err := readCSV(e.files[d.csvPath])
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %v", d.csvPath, err))
		}
		if len(recs) > 0 {
			d.header = recs[0]
			for _, rec := range recs[1:] {
				cells := make([]string, len(d.header))
				copy(cells, rec)
				t := strings.TrimSpace(cells[0])
				if t == "" {
					t = "Untitled"
				}
				d.rows = append(d.rows, &row{title: t, cells: cells})
			}
		}
		// The rows' pages: matched to the CSV's rows by title, in order, so
		// two rows of one title take the two pages of it.
		for _, p := range sortedKeys(e.files) {
			if path.Dir(p) != d.folder || !strings.EqualFold(path.Ext(p), ".md") {
				continue
			}
			t := pageTitle(e.files[p])
			if t == "" {
				t, _ = splitID(strings.TrimSuffix(path.Base(p), path.Ext(p)))
			}
			var r *row
			for _, c := range d.rows {
				if c.md == "" && c.cells != nil && strings.EqualFold(c.title, t) {
					r = c
					break
				}
			}
			if r == nil {
				r = &row{title: t}
				d.rows = append(d.rows, r)
			}
			r.md = p
			e.rowOf[p] = r
		}
		base := strings.TrimSuffix(d.stub, ".md")
		for i, r := range d.rows {
			r.synth = fmt.Sprintf("%s%s%d.md", base, rowMark, i)
		}
		d.status, d.done = taskColumns(d)
		out.Stats.Rows += len(d.rows)
	}
}

func readCSV(b []byte) ([][]string, error) {
	b = bytes.TrimPrefix(b, []byte("\ufeff"))
	r := csv.NewReader(bytes.NewReader(b))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	return r.ReadAll()
}

// taskColumns finds a Status column whose every value is a task state, and
// a Done checkbox column - each -1 when there is none.
func taskColumns(d *db) (int, int) {
	status, done := -1, -1
	for i, h := range d.header {
		if i == 0 {
			continue
		}
		vals := 0
		task, box := true, true
		for _, r := range d.rows {
			if r.cells == nil {
				continue
			}
			v := strings.ToLower(strings.TrimSpace(r.cells[i]))
			if v == "" {
				continue
			}
			vals++
			if !doneWords[v] && !openWords[v] {
				task = false
			}
			if v != "yes" && v != "no" {
				box = false
			}
		}
		if vals == 0 {
			continue
		}
		if status < 0 && task && statusColumn(h) {
			status = i
		}
		if done < 0 && box && doneColumn(h) {
			done = i
		}
	}
	return status, done
}

// pageTitle is a Notion page's first line, its # Title.
func pageTitle(b []byte) string {
	for _, l := range strings.Split(string(bytes.TrimPrefix(b, []byte("\ufeff"))), "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "# ") {
			return strings.TrimSpace(l[2:])
		}
		return ""
	}
	return ""
}

var (
	fenceRe   = regexp.MustCompile("^[ \\t]*(`{3,}|~{3,})")
	headRe    = regexp.MustCompile(`^(#{1,5})([ \t])`)
	linkRe    = regexp.MustCompile(`(!?\[(?:[^\[\]]|\[[^\]]*\])*\]\(\s*)(<[^>]*>|[^)\s]+)((?:\s+"[^"]*")?\s*\))`)
	propRe    = regexp.MustCompile(`^([^:\s][^:]{0,60}):[ \t]?(.*)$`)
	notionURL = regexp.MustCompile(`^https?://(?:www\.)?notion\.(?:so|site)/.*?([0-9a-fA-F]{32})(?:[?#].*)?$`)
	hexRe     = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
)

// page is a Notion page's markdown made ready for mdorg: its # Title kept as
// the only level-one heading (the body's headings moved down a level, so
// mdorg takes the title and moves them back up), a row's property lines
// taken off (they are the CSV's, and go in the drawer), and every link to
// another part of the export pointed at its new name.
func (e *export) page(orig, note string, r *row) string {
	text := strings.ReplaceAll(string(bytes.TrimPrefix(e.files[orig], []byte("\ufeff"))), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	title := ""
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "# ") {
		title = strings.TrimSpace(strings.TrimSpace(lines[i])[2:])
		i++
	}
	if title == "" {
		title = strings.TrimSuffix(path.Base(note), path.Ext(note))
		if r != nil {
			title = r.title
		}
	}
	lines = lines[i:]
	if r != nil {
		lines = e.rowProps(lines, r)
	}
	b := strings.Builder{}
	b.WriteString("# " + title + "\n")
	fence := ""
	for _, l := range lines {
		if m := fenceRe.FindStringSubmatch(l); m != nil {
			if fence == "" {
				fence = m[1][:1]
			} else if m[1][:1] == fence {
				fence = ""
			}
			b.WriteString(l + "\n")
			continue
		}
		if fence != "" {
			b.WriteString(l + "\n")
			continue
		}
		l = headRe.ReplaceAllString(l, "#$1$2")
		l = linkRe.ReplaceAllStringFunc(l, func(m string) string {
			sm := linkRe.FindStringSubmatch(m)
			t := strings.TrimSuffix(strings.TrimPrefix(sm[2], "<"), ">")
			if nt, ok := e.rewrite(path.Dir(orig), t, note); ok {
				return sm[1] + nt + sm[3]
			}
			return m
		})
		b.WriteString(l + "\n")
	}
	return b.String()
}

// rowProps takes a row page's Key: value lines off its top. They are the
// row's properties, already in the CSV; a key the CSV lacks is kept for the
// drawer. The paragraph is only taken when one of its keys is a column, so a
// row whose body starts "Note: call first" keeps it.
func (e *export) rowProps(lines []string, r *row) []string {
	d := e.dbOf[path.Dir(r.md)]
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	j := i
	props := [][2]string{}
	known := false
	for j < len(lines) && strings.TrimSpace(lines[j]) != "" {
		m := propRe.FindStringSubmatch(lines[j])
		if m == nil {
			return lines
		}
		props = append(props, [2]string{strings.TrimSpace(m[1]), strings.TrimSpace(m[2])})
		if d != nil {
			for _, h := range d.header {
				if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(m[1])) {
					known = true
				}
			}
		}
		j++
	}
	if !known && (d == nil || len(d.header) > 0) {
		return lines
	}
	for _, kv := range props {
		in := false
		if d != nil {
			for _, h := range d.header {
				in = in || strings.EqualFold(strings.TrimSpace(h), kv[0])
			}
		}
		if !in || r.cells == nil {
			r.extra = append(r.extra, kv)
		}
	}
	return lines[j:]
}

// resolve finds what a link names, as exported: a path relative to the
// page, or anything carrying a Notion id (a notion.so url, a page that moved).
func (e *export) resolve(dir, target string) string {
	if m := notionURL.FindStringSubmatch(target); m != nil {
		return e.byID[strings.ToLower(m[1])]
	}
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") {
		return ""
	}
	un, err := url.PathUnescape(target)
	if err != nil {
		un = target
	}
	p := path.Clean(path.Join(dir, un))
	if _, ok := e.files[p]; ok {
		return p
	}
	if e.dirs[p] {
		if _, ok := e.files[p+".md"]; ok {
			return p + ".md"
		}
		if d := e.dbOf[p]; d != nil {
			return d.csvPath
		}
	}
	st, _ := stemOf(path.Base(un))
	if _, id := splitID(st); id != "" {
		return e.byID[id]
	}
	return ""
}

// where is a part of the export as the vault knows it: the note (or file) a
// link goes to and the heading in it, for a database row.
func (e *export) where(p string) (string, string) {
	if r := e.rowOf[p]; r != nil {
		return e.dbOf[path.Dir(p)].stub, r.title
	}
	if d := e.dbOf[p]; d != nil {
		return d.stub, ""
	}
	if e.dirs[p] {
		return "", ""
	}
	return e.clean(p), ""
}

// rewrite points a markdown link at the vault's name for what it named.
func (e *export) rewrite(dir, target, note string) (string, bool) {
	t, frag, _ := strings.Cut(target, "#")
	if t == "" {
		return "", false
	}
	if notionURL.MatchString(target) {
		t, frag = target, ""
	}
	p := e.resolve(dir, t)
	if p == "" {
		return "", false // mdorg counts it, if it looks like a page or file
	}
	to, heading := e.where(p)
	if to == "" {
		return "", false
	}
	e.stats.Links++
	if hexRe.MatchString(frag) {
		frag = "" // a block's id, which org has no way to name
	}
	if heading != "" {
		frag = heading
	}
	out := escapePath(relPath(path.Dir(note), to))
	if frag != "" {
		out += "#" + url.PathEscape(frag)
	}
	return out, true
}

func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// relPath is the path from a folder to a file, both relative to the root.
func relPath(fromDir, to string) string {
	var from []string
	if fromDir != "." && fromDir != "" {
		from = strings.Split(fromDir, "/")
	}
	parts := strings.Split(to, "/")
	i := 0
	for i < len(from) && i < len(parts)-1 && from[i] == parts[i] {
		i++
	}
	return strings.Repeat("../", len(from)-i) + strings.Join(parts[i:], "/")
}

var orgHeadRe = regexp.MustCompile(`^\*+\s`)

// rowsOrg writes a database's rows as headings: keyword, tags, a deadline,
// the drawer of its columns, then the body its page converted to.
func (e *export) rowsOrg(d *db, conv map[string]string) string {
	b := strings.Builder{}
	self := mdorg.OrgPath(d.stub)
	selfLink := "[[file:" + mdorg.RelLink(self, self) + "::"
	coll := Collection(d.title)
	for _, r := range d.rows {
		kw, tags, planning := "", []string{}, ""
		props := [][2]string{{"RECORD", coll}}
		add := func(k, v string) {
			if v = e.value(d, k, v); v != "" {
				props = append(props, [2]string{PropName(k), v})
			}
		}
		if r.cells != nil {
			if d.status >= 0 {
				if v := strings.ToLower(strings.TrimSpace(r.cells[d.status])); v != "" {
					kw = "TODO"
					if doneWords[v] {
						kw = "DONE"
					}
				}
			}
			if kw == "" && d.done >= 0 {
				switch strings.ToLower(strings.TrimSpace(r.cells[d.done])) {
				case "yes":
					kw = "DONE"
				case "no":
					kw = "TODO"
				}
			}
			for i, h := range d.header {
				v := strings.TrimSpace(r.cells[i])
				switch {
				case i == 0, v == "":
				case tagsColumn(h):
					tags = append(tags, tagList(v)...)
				case i == d.done && kw != "":
					// the keyword says it
				case kw != "" && planning == "" && dueColumn(h):
					if s, ok := OrgDate(v, false); ok && !strings.Contains(v, "→") {
						planning = "DEADLINE: " + s
					} else {
						add(h, v)
					}
				default:
					add(h, v)
				}
			}
		}
		for _, kv := range r.extra {
			if tagsColumn(kv[0]) {
				tags = append(tags, tagList(kv[1])...)
				continue
			}
			add(kv[0], kv[1])
		}
		if kw != "" {
			e.stats.Tasks++
		}

		body := ""
		if r.md != "" {
			var more []string
			body, more = rowBody(conv[mdorg.OrgPath(r.synth)])
			tags = append(tags, more...)
			body = strings.ReplaceAll(body, selfLink, "[[")
		}
		head := "* "
		if kw != "" {
			head += kw + " "
		}
		head += r.title
		if tags = uniq(tags); len(tags) > 0 {
			head += " :" + strings.Join(tags, ":") + ":"
		}
		b.WriteString("\n" + head + "\n")
		if planning != "" {
			b.WriteString(planning + "\n")
		}
		b.WriteString(":PROPERTIES:\n")
		for _, kv := range props {
			b.WriteString(":" + kv[0] + ": " + kv[1] + "\n")
		}
		b.WriteString(":END:\n")
		if body != "" {
			b.WriteString("\n" + body + "\n")
		}
	}
	return b.String()
}

func tagList(v string) []string {
	out := []string{}
	for _, t := range strings.Split(v, ",") {
		if t = mdorg.OrgTag(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func uniq(s []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// rowBody is a row page's converted org without its file head (#+TITLE and
// the rest), its headings a level down to sit under the row's heading, and
// the #+FILETAGS it had as tags for the row.
func rowBody(org string) (string, []string) {
	lines := strings.Split(org, "\n")
	tags := []string{}
	i := 0
	if i < len(lines) && strings.TrimSpace(lines[i]) == ":PROPERTIES:" {
		for i < len(lines) && strings.TrimSpace(lines[i]) != ":END:" {
			i++
		}
		i++
	}
	for i < len(lines) && strings.HasPrefix(lines[i], "#+") {
		if v, ok := strings.CutPrefix(lines[i], "#+FILETAGS:"); ok {
			for _, t := range strings.Split(strings.TrimSpace(v), ":") {
				if t != "" {
					tags = append(tags, t)
				}
			}
		}
		i++
	}
	rest := lines[i:]
	for j, l := range rest {
		if orgHeadRe.MatchString(l) {
			rest[j] = "*" + l
		}
	}
	return strings.Trim(strings.Join(rest, "\n"), "\n"), tags
}

var (
	relationRe = regexp.MustCompile(`([^,()]*?)\s*\(([^()\s]+)\)`)
)

// value is one cell made fit for a property drawer: one line, a date as an
// org timestamp, a relation (Title (Page%20<id>.md), …) or a file as org
// links from the database's file.
func (e *export) value(d *db, col, v string) string {
	v = strings.Join(strings.Fields(strings.ReplaceAll(v, "\n", " ")), " ")
	if v == "" {
		return ""
	}
	if s, ok := OrgDate(v, historyColumn(col)); ok {
		return s
	}
	from := mdorg.OrgPath(d.stub)
	dir := path.Dir(d.csvPath)
	link := func(target, desc string) (string, bool) {
		// Relative to the CSV, or to the rows' folder beside it.
		p := e.resolve(dir, target)
		if p == "" {
			p = e.resolve(d.folder, target)
		}
		if p == "" {
			return "", false
		}
		to, heading := e.where(p)
		if to == "" {
			return "", false
		}
		e.stats.Links++
		if strings.EqualFold(path.Ext(to), ".md") {
			org := mdorg.OrgPath(to)
			l := "file:" + mdorg.RelLink(from, org)
			if heading != "" {
				l += "::*" + heading
				if org == from {
					l = "*" + heading
				}
				if desc == "" {
					desc = heading
				}
			}
			if desc == "" {
				desc = strings.TrimSuffix(path.Base(org), ".org")
			}
			return "[[" + l + "][" + desc + "]]", true
		}
		e.assets[to] = true
		return "[[file:" + mdorg.RelLink(from, to) + "]]", true
	}
	// Title (path), Title (path), ...: every one has to resolve, or the
	// cell is some text with brackets in it and is kept as written.
	if ms := relationRe.FindAllStringSubmatchIndex(v, -1); ms != nil {
		parts := []string{}
		rest := relationRe.ReplaceAllString(v, "")
		if strings.Trim(rest, ", ") == "" {
			ok := true
			for _, m := range ms {
				l, good := link(v[m[4]:m[5]], strings.TrimSpace(v[m[2]:m[3]]))
				if !good {
					ok = false
					break
				}
				parts = append(parts, l)
			}
			if ok {
				return strings.Join(parts, ", ")
			}
		}
	}
	// Files & media: paths with no spaces (they are url-encoded).
	if !strings.Contains(v, " ") || strings.Contains(v, "%20") {
		parts := []string{}
		ok := true
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p == "" || strings.Contains(p, " ") || !strings.Contains(p, ".") && !strings.Contains(p, "/") {
				ok = false
				break
			}
			l, good := link(p, "")
			if !good {
				ok = false
				break
			}
			parts = append(parts, l)
		}
		if ok && len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	}
	return v
}
