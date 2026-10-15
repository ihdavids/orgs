package orgs

/* SDOC: Editing
* Keeping Blocks Current

  Some of a file is worth recomputing on its own: a source block that reports
  disk use or a CI status table, a clock table, a column view, a table's
  formulas. orgs reruns them on a timer, writes what changed with line edits
  (nothing else in the file moves), and hands an export a freshly computed copy
  so a published page is never stale.

  ** Scheduled source blocks

  A source block, or a =#+CALL:= line, with a =:schedule= header argument is run
  again on that schedule and its =#+RESULTS:= updated. It needs =babel.enable=,
  as running any block does.

  #+BEGIN_EXAMPLE
  #+NAME: disk
  #+BEGIN_SRC sh :schedule 1h :results table
  df -h / /home | awk '{print $1, $5, $6}'
  #+END_SRC

  #+CALL: ci-status(repo="orgs") :schedule 15m
  #+END_EXAMPLE

  The schedule is a length of time (=30m=, =1h=, =2d=, =1w=, =1h30m=), a word
  (=hourly=, =daily=, =weekly=), or a time of day (=daily 07:30=, =07:30=: once
  a day, at or after that time). The shortest is a minute. Like any header
  argument it can be inherited: =#+PROPERTY: header-args :schedule 1h= puts
  every block in the file on the hour.

  A run that produces what the file already says writes nothing, so a block
  checked every five minutes does not rewrite its file every five minutes.

  ** Refreshing dynamic blocks and tables

  - =:refresh 1h= on a dynamic block's =#+BEGIN:= line refreshes that block.
  - =#+REFRESH: 1h= in a file refreshes all of its dynamic blocks and runs
    every =#+TBLFM:= in it.
  - =refresh.every= in the server settings does that for every file that has
    a dynamic block or a table formula.

  A table is written only when its values change, never just to realign it.

  ** On export

  An export (=/file/html=, =orgs export=, =orgs site=, typst and beamer pdfs)
  reads a copy of the file with its dynamic blocks refreshed and its table
  formulas run, *in memory*: the page is current and the file is not written
  because somebody exported it. =refresh.onExport: false= turns this off, and
  =refresh=f= on a request skips it once.

  ** Settings

  #+BEGIN_SRC yaml
  server:
    refresh:
      every: 1h          # every file's dynamic blocks and table formulas
      onExport: true     # default
      scheduled: true    # run :schedule blocks (default; babel.enable must be on too)
      tick: 30           # seconds between looks at what is due
  #+END_SRC

  =orgs refresh= lists everything on a timer, when it last ran and how it went;
  =orgs refresh run= runs it now.
EDOC */

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// ----------------------------------------------------------------------------
// Schedules
// ----------------------------------------------------------------------------

// A schedule as written: every so long, or once a day at a time.
type refreshSchedule struct {
	Text  string
	Every time.Duration
	At    bool
	Hour  int
	Min   int
}

var (
	schedClockRe = regexp.MustCompile(`^(?:daily\s+|at\s+|@)?(\d{1,2}):(\d{2})$`)
	schedSpanRe  = regexp.MustCompile(`^(?:every\s+)?((?:\d+(?:\.\d+)?\s*(?:s|m|h|d|w)\s*)+)$`)
	schedPartRe  = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(s|m|h|d|w)`)
)

// The shortest schedule: a block run more often than this is a loop, not a
// dashboard, and a tick of the timer is half of it.
const minRefreshEvery = time.Minute

func parseSchedule(s string) (refreshSchedule, error) {
	t := strings.ToLower(strings.Trim(strings.TrimSpace(s), `"'`))
	out := refreshSchedule{Text: strings.TrimSpace(s)}
	switch t {
	case "":
		return out, fmt.Errorf("no schedule given")
	case "hourly", "@hourly":
		out.Every = time.Hour
		return out, nil
	case "daily", "@daily":
		out.Every = 24 * time.Hour
		return out, nil
	case "weekly", "@weekly":
		out.Every = 7 * 24 * time.Hour
		return out, nil
	}
	if m := schedClockRe.FindStringSubmatch(t); m != nil {
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		if h > 23 || mi > 59 {
			return out, fmt.Errorf("%q is not a time of day", s)
		}
		out.At, out.Hour, out.Min = true, h, mi
		return out, nil
	}
	if m := schedSpanRe.FindStringSubmatch(t); m != nil {
		var d time.Duration
		for _, p := range schedPartRe.FindAllStringSubmatch(m[1], -1) {
			n, _ := strconv.ParseFloat(p[1], 64)
			unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour,
				"d": 24 * time.Hour, "w": 7 * 24 * time.Hour}[p[2]]
			d += time.Duration(n * float64(unit))
		}
		if d < minRefreshEvery {
			d = minRefreshEvery
		}
		out.Every = d
		return out, nil
	}
	return out, fmt.Errorf("%q is not a schedule (try 30m, 1h, daily or 07:30)", s)
}

// due says whether something last run at last is due at now, and when it is
// next due.
func (s refreshSchedule) due(last, now time.Time) (bool, time.Time) {
	if s.At {
		today := time.Date(now.Year(), now.Month(), now.Day(), s.Hour, s.Min, 0, 0, now.Location())
		if !now.Before(today) && last.Before(today) {
			return true, today
		}
		if now.Before(today) {
			return false, today
		}
		return false, today.AddDate(0, 0, 1)
	}
	if last.IsZero() {
		return true, now
	}
	next := last.Add(s.Every)
	return !now.Before(next), next
}

// ----------------------------------------------------------------------------
// Finding what is on a timer
// ----------------------------------------------------------------------------

type refreshEntry struct {
	common.RefreshItem
	sched refreshSchedule
}

var (
	refreshKeywordRe = regexp.MustCompile(`(?i)^\s*#\+REFRESH:\s*(\S.*?)\s*$`)
	scheduleArgRe    = regexp.MustCompile(`(?i):schedule\s+("[^"]*"|daily\s+\d{1,2}:\d{2}|\S+)`)
	tblfmLineRe      = regexp.MustCompile(`(?im)^\s*#\+TBLFM:`)
)

func refreshHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:6])
}

// What one file has on a timer, read from its lines (and, for source blocks,
// from the database, which knows their inherited header arguments).
func refreshEntriesOf(f *common.OrgFile, lines []string, every string) []refreshEntry {
	out := []refreshEntry{}
	p := f.Filename
	add := func(item common.RefreshItem, when string) {
		s, err := parseSchedule(when)
		item.File, item.Every = p, when
		if err != nil {
			item.Blocked = err.Error()
		}
		out = append(out, refreshEntry{RefreshItem: item, sched: s})
	}
	text := strings.Join(lines, "\n")

	// The file as a whole: its own #+REFRESH:, else the server's every.
	hasDyn := strings.Contains(strings.ToUpper(text), "#+BEGIN:")
	hasFm := tblfmLineRe.MatchString(text)
	fileWhen, from := "", ""
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "*") {
			break
		}
		if m := refreshKeywordRe.FindStringSubmatch(l); m != nil {
			fileWhen, from = m[1], "#+REFRESH:"
		}
	}
	if fileWhen == "" && every != "" && (hasDyn || hasFm) {
		fileWhen, from = every, "refresh.every"
	}
	if fileWhen != "" {
		add(common.RefreshItem{Kind: "file", Key: "file:" + p, Line: 0, From: from, Name: filepath.Base(p)}, fileWhen)
	}

	// Dynamic blocks with :refresh of their own.
	if hasDyn {
		seen := map[string]int{}
		for _, b := range findDynBlocks(lines) {
			when := b.Params["refresh"]
			if when == "" || when == "t" {
				continue
			}
			seen[b.Header]++
			key := fmt.Sprintf("dblock:%s#%s#%d", p, b.Header, seen[b.Header])
			add(common.RefreshItem{Kind: "dblock", Key: key, Line: b.Begin, Name: b.Name, From: ":refresh"}, when)
		}
	}

	if !strings.Contains(strings.ToLower(text), ":schedule") {
		return out
	}
	// Source blocks: the job's own reading of its header arguments, so an
	// inherited :schedule counts.
	names := namedKinds(f)
	for _, ref := range collectFileBlocks(f) {
		block := readBlock(ref, names)
		job := jobForBlock(&block, f)
		when, ok := job.arg("schedule")
		if !ok || strings.TrimSpace(when) == "" {
			continue
		}
		id := block.Name
		if id == "" {
			id = block.Lang + ":" + refreshHash(block.Code)
		}
		add(common.RefreshItem{Kind: "block", Key: "block:" + p + "#" + id, Line: block.Line, Name: block.Name, From: ":schedule"}, strings.Trim(when, `"`))
	}
	// #+CALL: lines saying :schedule among their own header arguments.
	for i, l := range lines {
		m := callLineRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		sm := scheduleArgRe.FindStringSubmatch(m[3] + " " + m[5])
		if sm == nil {
			continue
		}
		key := "call:" + p + "#" + refreshHash(strings.TrimSpace(l))
		add(common.RefreshItem{Kind: "call", Key: key, Line: i, Name: m[2], From: ":schedule"}, strings.Trim(sm[1], `"`))
	}
	return out
}

// ----------------------------------------------------------------------------
// The engine
// ----------------------------------------------------------------------------

// What is remembered of each thing's last run, across restarts.
type refreshRecord struct {
	Last    time.Time
	Ok      bool
	Msg     string `json:",omitempty"`
	Changed bool
	Millis  int64
}

type refreshEngine struct {
	mu      sync.Mutex
	runMu   sync.Mutex // one run at a time: the timer's and a request's
	records map[string]refreshRecord
	loaded  bool
	// The last scan and the database state it was made at.
	scanned   []refreshEntry
	scannedAt uint64
	scanOnce  bool
	started   bool
}

var refresher = &refreshEngine{records: map[string]refreshRecord{}}

func refreshDataPath() string {
	return path.Join(Conf().PlugManager.HomeDir, "refresh_data.json")
}

func (e *refreshEngine) load() {
	if e.loaded {
		return
	}
	e.loaded = true
	if b, err := os.ReadFile(refreshDataPath()); err == nil {
		json.Unmarshal(b, &e.records)
	}
}

func (e *refreshEngine) save() {
	b, err := json.MarshalIndent(e.records, "", " ")
	if err == nil {
		os.WriteFile(refreshDataPath(), b, 0644)
	}
}

// entries is everything on a timer, scanned again only when a file changed.
func (e *refreshEngine) entries() []refreshEntry {
	db := GetDb()
	e.mu.Lock()
	if e.scanOnce && e.scannedAt == db.ReloadIndex {
		out := e.scanned
		e.mu.Unlock()
		return out
	}
	e.mu.Unlock()
	every := strings.TrimSpace(Conf().Server.Refresh.Every)
	at := db.ReloadIndex
	out := []refreshEntry{}
	files := append([]string{}, db.GetFiles()...)
	sort.Strings(files)
	for _, name := range files {
		f := db.GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		lines, err := readFileLines(f.Filename)
		if err != nil {
			continue
		}
		out = append(out, refreshEntriesOf(f, lines, every)...)
	}
	e.mu.Lock()
	e.scanned, e.scannedAt, e.scanOnce = out, at, true
	e.mu.Unlock()
	return out
}

// blocked is why an entry will not run, or "".
func blockedReason(en refreshEntry) string {
	if en.Blocked != "" {
		return en.Blocked
	}
	if en.Kind == "block" || en.Kind == "call" {
		if !Conf().Server.Refresh.RunScheduled() {
			return "refresh.scheduled is off"
		}
		if !Conf().Server.Babel.Enable {
			return "babel.enable is off, so source blocks are not run"
		}
	}
	return ""
}

// item is an entry with what is known of its runs filled in.
func (e *refreshEngine) item(en refreshEntry, now time.Time) common.RefreshItem {
	it := en.RefreshItem
	it.Blocked = blockedReason(en)
	e.mu.Lock()
	rec, ok := e.records[en.Key]
	e.mu.Unlock()
	if ok {
		it.Last = rec.Last.Format(time.RFC3339)
		it.Ok, it.Msg, it.Changed, it.Millis = rec.Ok, rec.Msg, rec.Changed, rec.Millis
	}
	if it.Blocked == "" {
		_, next := en.sched.due(rec.Last, now)
		it.Next = next.Format(time.RFC3339)
	}
	return it
}

// run runs one entry and remembers how it went.
func (e *refreshEngine) run(en refreshEntry) common.RefreshItem {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	start := time.Now()
	rec := refreshRecord{Last: start}
	changed, msg, err := runRefreshEntry(en)
	rec.Millis = time.Since(start).Milliseconds()
	rec.Changed = changed
	if err != nil {
		rec.Msg = err.Error()
	} else {
		rec.Ok, rec.Msg = true, msg
	}
	e.mu.Lock()
	e.records[en.Key] = rec
	e.save()
	e.mu.Unlock()
	if changed || err != nil {
		Publish(Event{Kind: EventRefreshed, File: en.File, Msg: fmt.Sprintf("%s %s: %s", en.Kind, refreshLabel(en.RefreshItem), rec.Msg)})
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "REFRESH: %s %s: %v\n", en.Kind, refreshLabel(en.RefreshItem), err)
	}
	return e.item(en, time.Now())
}

// EventRefreshed: something on a timer ran and changed its file, or failed.
const EventRefreshed = "refreshed"

func refreshLabel(it common.RefreshItem) string {
	name := it.Name
	if name == "" {
		name = fmt.Sprintf("line %d", it.Line+1)
	}
	return filepath.Base(it.File) + " " + name
}

// runRefreshEntry does the work, finding the thing again first: the line it
// was scanned at may have moved since.
func runRefreshEntry(en refreshEntry) (bool, string, error) {
	if why := blockedReason(en); why != "" {
		return false, "", fmt.Errorf("%s", why)
	}
	switch en.Kind {
	case "file":
		res, err := refreshFile(en.File, true)
		if err != nil {
			return false, "", err
		}
		return res.changed, res.summary(), res.err()
	case "dblock":
		f := GetDb().FindByFile(en.File)
		if f == nil {
			return false, "", fmt.Errorf("the file is gone")
		}
		lines, err := readFileLines(f.Filename)
		if err != nil {
			return false, "", err
		}
		// The same block by the same reading of its #+BEGIN: line as the scan.
		seen := map[string]int{}
		want := -1
		for _, b := range findDynBlocks(lines) {
			seen[b.Header]++
			if fmt.Sprintf("dblock:%s#%s#%d", f.Filename, b.Header, seen[b.Header]) == en.Key {
				want = b.Begin
			}
		}
		if want < 0 {
			return false, "", fmt.Errorf("the block is no longer in the file")
		}
		res, err := refreshDynBlocks(f.Filename, func(b dynBlock) bool { return b.Begin == want })
		if err != nil {
			return false, "", err
		}
		for _, b := range res.Blocks {
			if !b.Ok {
				return false, "", fmt.Errorf("%s", b.Msg)
			}
		}
		if res.Written {
			return true, "refreshed", nil
		}
		return false, "no change", nil
	case "block", "call":
		f := GetDb().FindByFile(en.File)
		if f == nil {
			return false, "", fmt.Errorf("the file is gone")
		}
		lines, err := readFileLines(f.Filename)
		if err != nil {
			return false, "", err
		}
		row := -1
		for _, cur := range refreshEntriesOf(f, lines, "") {
			if cur.Key == en.Key {
				row = cur.Line
			}
		}
		if row < 0 {
			return false, "", fmt.Errorf("the block is no longer in the file, or no longer scheduled")
		}
		for _, it := range babelItems(lines) {
			if it.row != row || it.kind != en.Kind {
				continue
			}
			ran, err := runItem(f.Filename, it, true)
			if err != nil {
				return false, "", err
			}
			if len(ran) == 0 {
				return false, "", fmt.Errorf("nothing ran")
			}
			r := ran[0].Result
			if !r.Ok {
				msg := r.Msg
				if msg == "" {
					msg = "the block failed"
				}
				return r.Written, "", fmt.Errorf("%s", strings.TrimSpace(msg))
			}
			switch {
			case r.Written:
				return true, "updated", nil
			case r.Unchanged:
				return false, "no change", nil
			}
			return false, "ran", nil
		}
		return false, "", fmt.Errorf("nothing runnable at line %d", row+1)
	}
	return false, "", fmt.Errorf("nothing to do for a %q", en.Kind)
}

// tick runs whatever is due. Things are run one at a time, in file order.
func (e *refreshEngine) tick(now time.Time) []common.RefreshItem {
	e.mu.Lock()
	e.load()
	e.mu.Unlock()
	ran := []common.RefreshItem{}
	for _, en := range e.entries() {
		if blockedReason(en) != "" {
			continue
		}
		e.mu.Lock()
		last := e.records[en.Key].Last
		e.mu.Unlock()
		if due, _ := en.sched.due(last, now); due {
			ran = append(ran, e.run(en))
		}
	}
	return ran
}

func (e *refreshEngine) start() {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return
	}
	e.started = true
	e.mu.Unlock()
	go func() {
		// A moment for the files to load before the first look.
		time.Sleep(5 * time.Second)
		for {
			e.tick(time.Now())
			secs := Conf().Server.Refresh.Tick
			if secs <= 0 {
				secs = 30
			}
			time.Sleep(time.Duration(secs) * time.Second)
		}
	}()
}

// ----------------------------------------------------------------------------
// A file's dynamic blocks and tables
// ----------------------------------------------------------------------------

type fileRefresh struct {
	lines   []string
	doc     *org.Document
	changed bool
	blocks  int
	tables  int
	errs    []string
}

func (r *fileRefresh) summary() string {
	if !r.changed {
		return fmt.Sprintf("no change (%d blocks, %d tables)", r.blocks, r.tables)
	}
	return fmt.Sprintf("refreshed (%d blocks, %d tables)", r.blocks, r.tables)
}

func (r *fileRefresh) err() error {
	if len(r.errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(r.errs, "; "))
}

// refreshFile refreshes a file's dynamic blocks and runs its table formulas:
// written to the file with write, or only into the lines and document handed
// back without it (what an export reads).
func refreshFile(filename string, write bool) (*fileRefresh, error) {
	res, p, lines, changed, err := dynBlockLines(filename, func(dynBlock) bool { return true })
	if err != nil {
		return nil, err
	}
	out := &fileRefresh{lines: lines, changed: changed, blocks: len(res.Blocks)}
	for _, b := range res.Blocks {
		if !b.Ok {
			out.errs = append(out.errs, fmt.Sprintf("line %d: %s", b.Line+1, b.Msg))
		}
	}
	doc, tables, tchanged, terrs := recalcTableLines(p, out.lines)
	out.doc, out.tables = doc, tables
	out.errs = append(out.errs, terrs...)
	if tchanged != nil {
		out.lines, out.changed = tchanged, true
	}
	if write && out.changed {
		if err := writeLines(p, out.lines); err != nil {
			return out, err
		}
	}
	return out, nil
}

// recalcTableLines runs every #+TBLFM: in the text, giving back the document
// with the new values in it, how many tables had formulas, and the lines with
// those whose values changed spliced in (nil when none did). A table whose
// values come out the same is left as written, however it is aligned.
func recalcTableLines(p string, lines []string) (*org.Document, int, []string, []string) {
	doc := GetConfig().Silent().Parse(strings.NewReader(strings.Join(lines, "\n")+"\n"), p)
	of := &common.OrgFile{Filename: p, Doc: doc}
	refs := collectFileTables(of)
	n := 0
	errs := []string{}
	var out []string
	work := append([]string{}, lines...)
	// Bottom up, so a table that comes out a different number of lines does
	// not move the ones above it.
	for i := len(refs) - 1; i >= 0; i-- {
		ref := refs[i]
		if ref.Table.Formulas == nil {
			continue
		}
		n++
		start := ref.Table.Pos.Row
		if start < 0 || start >= len(work) {
			continue
		}
		indent := leadingWhitespace(work[start])
		before := renderTable(ref.Table, indent)
		if err := ExecuteFormula(db, ref.Sec, of, ref.Table); err != nil {
			errs = append(errs, fmt.Sprintf("the table at line %d: %v", start+1, err))
			continue
		}
		after := renderTable(ref.Table, indent)
		if after == before {
			continue
		}
		text, err := spliceTable(work, start, tableLastLine(ref.Table), after)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		work = strings.Split(text, "\n")
		out = work
	}
	return doc, n, out, errs
}

// ----------------------------------------------------------------------------
// Exports
// ----------------------------------------------------------------------------

var refreshableRe = regexp.MustCompile(`(?im)^\s*#\+(BEGIN:|TBLFM:)`)

// exportFile is the file an export should read: its dynamic blocks refreshed
// and its formulas run, in memory, when there are any and refresh.onExport is
// on; the database's own reading otherwise.
func exportFile(filename string) *common.OrgFile {
	f, _ := exportFileStamped(filename)
	return f
}

// exportFileStamped is exportFile with a stamp of the refreshed text, for a
// cache keyed on the file's state: a clock table can change while the file
// does not. The stamp is "" when the file is read as it is.
func exportFileStamped(filename string) (*common.OrgFile, string) {
	f := GetDb().FindByFile(filename)
	if f == nil || f.Doc == nil || !Conf().Server.Refresh.ExportRefresh() {
		return f, ""
	}
	b, err := os.ReadFile(f.Filename)
	if err != nil || !refreshableRe.Match(b) {
		return f, ""
	}
	r, err := refreshFile(f.Filename, false)
	if err != nil || r.doc == nil {
		return f, ""
	}
	if !r.changed {
		// Nothing came out different: the file as the database has it.
		return f, ""
	}
	// The blocks' new text read as a document, with the tables' new values
	// already in it.
	doc, _, _, _ := recalcTableLines(f.Filename, r.lines)
	return &common.OrgFile{Filename: f.Filename, Doc: doc}, refreshHash(strings.Join(r.lines, "\n"))
}

// exportView is the database with query's file read as exportFile reads it.
func exportView(base common.ODb, query string) common.ODb {
	f := GetDb().FindByFile(query)
	if f == nil {
		return base
	}
	ef := exportFile(query)
	if ef == nil || ef == f {
		return base
	}
	return &subtreeDb{ODb: base, file: ef}
}

// ----------------------------------------------------------------------------
// Endpoints
// ----------------------------------------------------------------------------

// registerRefreshRoutes adds /refresh and starts the timer. Called from RestApi.
func registerRefreshRoutes(api *mux.Router) {
	api.HandleFunc("/refresh", RequestRefresh).Methods("GET")
	api.HandleFunc("/refresh/run", PostRefreshRun).Methods("POST")
	refresher.start()
}

/* SDOC: API
* GET /refresh — What Is Kept Current, And How It Went

	Everything on a timer: source blocks and =#+CALL:= lines with =:schedule=,
	dynamic blocks with =:refresh=, and files with =#+REFRESH:= (or every file
	with a dynamic block or a table formula, under =refresh.every=). Each item
	says what it is (=Kind=, =File=, =Line=, =Name=), how often and who said so
	(=Every=, =From=), when it last ran and will next (=Last=, =Next=), how
	that went (=Ok=, =Msg=, =Changed=, =Millis=), and why it will not run, if
	it will not (=Blocked=: babel is off, a schedule that does not read).
EDOC */
func RequestRefresh(w http.ResponseWriter, r *http.Request) {
	refresher.mu.Lock()
	refresher.load()
	refresher.mu.Unlock()
	now := time.Now()
	out := common.RefreshList{Ok: true, Items: []common.RefreshItem{},
		OnExport: Conf().Server.Refresh.ExportRefresh(), Every: Conf().Server.Refresh.Every}
	for _, en := range refresher.entries() {
		out.Items = append(out.Items, refresher.item(en, now))
	}
	dynJson(w, out)
}

/* SDOC: API
* POST /refresh/run — Run Something Now

	#+BEGIN_EXAMPLE
	{ "Key": "block:/org/dash.org#disk" }      one item, by the Key /refresh gave it
	{ "Due": true }                            whatever is due
	{ "All": true }                            everything on a timer
	{ "File": "dash.org" }                     that file's dynamic blocks and tables
	{ "File": "dash.org", "Blocks": true }     and its :schedule blocks
	#+END_EXAMPLE

	Runs it now, whether or not it is due, and answers each item as /refresh
	would, after the run. A file need not be on a timer to be refreshed by name.
EDOC */
func PostRefreshRun(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.RefreshRun
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.RefreshRan{Msg: err.Error()})
		return
	}
	dynJson(w, RunRefresh(&req))
}

// RunRefresh is the whole of POST /refresh/run.
func RunRefresh(req *common.RefreshRun) common.RefreshRan {
	refresher.mu.Lock()
	refresher.load()
	refresher.mu.Unlock()
	out := common.RefreshRan{Ok: true, Items: []common.RefreshItem{}}
	entries := refresher.entries()
	pick := []refreshEntry{}
	switch {
	case req.Key != "":
		for _, en := range entries {
			if en.Key == req.Key {
				pick = append(pick, en)
			}
		}
		if len(pick) == 0 {
			return common.RefreshRan{Msg: fmt.Sprintf("nothing on a timer is called %s", req.Key)}
		}
	case req.File != "":
		f := GetDb().FindByFile(req.File)
		if f == nil {
			return common.RefreshRan{Msg: fmt.Sprintf("no file called %s", req.File)}
		}
		// The file's blocks and tables whether or not it is on a timer.
		pick = append(pick, refreshEntry{RefreshItem: common.RefreshItem{Kind: "file", Key: "file:" + f.Filename,
			File: f.Filename, Name: filepath.Base(f.Filename), From: "request"}})
		if req.Blocks {
			for _, en := range entries {
				if en.File == f.Filename && (en.Kind == "block" || en.Kind == "call") {
					pick = append(pick, en)
				}
			}
		}
	case req.All:
		pick = entries
	case req.Due:
		return common.RefreshRan{Ok: true, Msg: "ran what was due", Items: refresher.tick(time.Now())}
	default:
		return common.RefreshRan{Msg: "say what to run: Key, File, Due or All"}
	}
	failed := 0
	for _, en := range pick {
		it := refresher.run(en)
		if !it.Ok {
			failed++
		}
		out.Items = append(out.Items, it)
	}
	out.Msg = fmt.Sprintf("%d ran", len(out.Items))
	if failed > 0 {
		out.Ok = false
		out.Msg += fmt.Sprintf(", %d failed", failed)
	}
	return out
}
