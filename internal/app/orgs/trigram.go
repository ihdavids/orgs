package orgs

// A trigram index over the text of every org file, so that searching it is not
// reading it.
//
// `/files/search` is the box worg searches as you type, and it used to open and
// scan every file on every keystroke. On a database of five hundred files that
// is about fifteen milliseconds, which is bearable; it is flat cost per
// keystroke and linear in the database, so at ten times the size it is not.
//
// The idea is Google Code Search's. Every three-byte run in a file is a
// trigram, and a line can only match a literal if the file contains every
// trigram of that literal - so the index turns "search everything" into "search
// the handful of files that could possibly match", and the real regular
// expression still runs on those. That last part is what makes it safe: the
// index only ever *narrows* the set of files, and a wrong answer from it would
// have to be a file it wrongly excluded, which is the one thing the
// construction below makes impossible.
//
// Three decisions worth stating:
//
//  1. **The index never decides a match.** It answers "these files might", and
//     the existing scanner answers "these lines do", unchanged. A pattern whose
//     required trigrams cannot be worked out - `.` , `a|b`, anything starting
//     with a character class - falls back to scanning everything, which is
//     exactly what happens today. Nothing gets slower and nothing gets wrong.
//  2. **It is keyed per file.** A saved file costs one file's trigrams, not the
//     database's. That is the whole point of the exercise and is why `OrgDb`
//     now carries a version per file.
//  3. **It is written to disk**, so a restart costs a read rather than a walk
//     of every file - and it is checked against each file's size and
//     modification time on the way in, so a file changed while the server was
//     down is re-read rather than trusted.

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"regexp/syntax"
	"sort"
	"strings"
	"sync"
	"time"
)

// How the index file is laid out. Bumped when the shape below changes, so an
// index written by an older orgs is discarded rather than misread.
const trigramFormat = 3

// A file smaller than this is not worth an index entry - reading it is cheaper
// than the posting lists would be. In practice nothing is this small, but a
// zero length file would otherwise index as no trigrams at all and so match
// every query.
const trigramMinBytes = 3

// Above this, a query's trigrams are not worth intersecting: a literal of
// twenty characters has eighteen trigrams and the rarest two or three have
// already narrowed the answer to nothing. Every one after that is a map lookup
// and a set intersection for no gain.
const trigramMaxQuery = 8

type trigram uint32

func tri(a, b, c byte) trigram {
	return trigram(a)<<16 | trigram(b)<<8 | trigram(c)
}

// What the index holds about one file, so that a stale entry can be recognised
// without reading the file.
type trigramFile struct {
	Name    string
	Size    int64
	ModUnix int64
	// The database's own version of this file when the entry was made. Zero for
	// an entry read back from disk, which has only the size and time to go on.
	Version uint64
	// The trigrams this file holds.
	//
	// Kept so that removing the file can take its number out of exactly those
	// posting lists. The first version of this did not, on the reasoning that
	// walking a list per trigram is expensive and a dead number could simply be
	// skipped when the lists were read - which is true right up until the
	// number is handed to the next file, at which point that file answers for
	// everything its predecessor held. On a database being edited every save
	// leaves another file's worth behind, and the index converges on "every
	// file matches everything" without ever being wrong enough to notice.
	Tris []trigram
}

type trigramIndex struct {
	mu sync.RWMutex
	// Which files are indexed, by name, and where each sits in `files`.
	files []trigramFile
	at    map[string]int
	// trigram -> the file numbers holding it, ascending.
	post map[trigram][]int32
	// Files whose entry is a hole - removed, waiting to be reused.
	free []int
	// Whether anything has changed since the last write to disk.
	dirty bool
}

var (
	triIndex *trigramIndex
	triOnce  sync.Once
	triLock  sync.Mutex
)

// ---------------------------------------------------------------------------
// Building
// ---------------------------------------------------------------------------

// Every distinct trigram in some text, lower cased.
//
// Lower cased always, whether or not the search is case sensitive. The index is
// a filter, so matching too many files is harmless - the real expression throws
// them out - while matching too few is a missed result. One case-folded index
// serves both kinds of search and cannot be wrong in the direction that matters.
func trigramsOf(data []byte) []trigram {
	if len(data) < 3 {
		return nil
	}
	seen := make(map[trigram]struct{}, len(data)/8)
	lower := func(b byte) byte {
		if b >= 'A' && b <= 'Z' {
			return b + 32
		}
		return b
	}
	a, b := lower(data[0]), lower(data[1])
	for i := 2; i < len(data); i++ {
		c := lower(data[i])
		seen[tri(a, b, c)] = struct{}{}
		a, b = b, c
	}
	out := make([]trigram, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	return out
}

// ORGS_NO_TRIGRAM turns the index off, so that "with" and "without" can be
// measured on the same binary against the same files. Not a setting: it is for
// answering "is the index actually helping", which is a question about a
// particular machine at a particular moment rather than about how to run a
// server.
func trigramDisabled() bool {
	return os.Getenv("ORGS_NO_TRIGRAM") != ""
}

func getTrigramIndex() *trigramIndex {
	triOnce.Do(func() {
		triIndex = &trigramIndex{
			at:   map[string]int{},
			post: map[trigram][]int32{},
		}
		triIndex.load()
	})
	return triIndex
}

// Bring the index up to date with the database.
//
// Only the files whose version has moved are read, which is the whole point: a
// save costs one file. A file the database no longer holds is dropped.
func (self *trigramIndex) sync() {
	versions, _ := GetDb().FileVersions()

	self.mu.RLock()
	stale := []string{}
	gone := []string{}
	for name, v := range versions {
		i, have := self.at[name]
		if !have {
			stale = append(stale, name)
			continue
		}
		e := self.files[i]
		if e.Version != v {
			// Either the database has read it again, or this entry came off
			// disk with no version at all - in which case the file's own size
			// and time decide.
			if e.Version != 0 {
				stale = append(stale, name)
				continue
			}
			if st, err := os.Stat(name); err != nil || st.Size() != e.Size || st.ModTime().Unix() != e.ModUnix {
				stale = append(stale, name)
			}
		}
	}
	for name := range self.at {
		if _, still := versions[name]; !still {
			gone = append(gone, name)
		}
	}
	self.mu.RUnlock()

	if len(stale) == 0 && len(gone) == 0 {
		return
	}
	// Read outside the lock: this is the expensive part and nothing else needs
	// to wait for it.
	type built struct {
		name string
		tris []trigram
		st   os.FileInfo
	}
	made := make([]built, 0, len(stale))
	for _, name := range stale {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		st, err := os.Stat(name)
		if err != nil {
			continue
		}
		if len(data) < trigramMinBytes {
			made = append(made, built{name: name, tris: nil, st: st})
			continue
		}
		made = append(made, built{name: name, tris: trigramsOf(data), st: st})
	}

	self.mu.Lock()
	defer self.mu.Unlock()
	for _, name := range gone {
		self.removeLocked(name)
	}
	for _, m := range made {
		self.removeLocked(m.name)
		self.addLocked(m.name, m.tris, m.st.Size(), m.st.ModTime().Unix(), versions[m.name])
	}
	self.dirty = true
}

func (self *trigramIndex) addLocked(name string, tris []trigram, size, mod int64, version uint64) {
	e := trigramFile{Name: name, Size: size, ModUnix: mod, Version: version, Tris: tris}
	id := -1
	if len(self.free) > 0 {
		id = self.free[len(self.free)-1]
		self.free = self.free[:len(self.free)-1]
		self.files[id] = e
	} else {
		id = len(self.files)
		self.files = append(self.files, e)
	}
	self.at[name] = id
	for _, t := range tris {
		self.post[t] = append(self.post[t], int32(id))
	}
	// The posting lists have to stay ascending for the intersection, and a
	// reused file number is usually lower than the ones already there.
	for _, t := range tris {
		p := self.post[t]
		if len(p) > 1 && p[len(p)-1] < p[len(p)-2] {
			sort.Slice(p, func(a, b int) bool { return p[a] < p[b] })
		}
	}
}

// Take a file out, of the posting lists as well as of the file table.
//
// Exactly, rather than by marking the slot dead and skipping it later: the slot
// is reused by the next file to arrive, and a slot carrying the previous file's
// postings makes its successor answer for text it does not contain. See the
// note on `Tris` above for why that matters more than it sounds like it does.
//
// The cost is one walk per trigram the file held, which is a few milliseconds
// on a large file and happens once per save.
func (self *trigramIndex) removeLocked(name string) {
	i, have := self.at[name]
	if !have {
		return
	}
	id := int32(i)
	for _, t := range self.files[i].Tris {
		p := self.post[t]
		for k, v := range p {
			if v == id {
				self.post[t] = append(p[:k], p[k+1:]...)
				break
			}
		}
		if len(self.post[t]) == 0 {
			delete(self.post, t)
		}
	}
	delete(self.at, name)
	self.files[i] = trigramFile{}
	self.free = append(self.free, i)
}

// ---------------------------------------------------------------------------
// Asking it
// ---------------------------------------------------------------------------

// The files that could possibly hold a match, or nil meaning "no idea, look at
// all of them".
//
// nil is not a failure and is the answer for every pattern whose required
// trigrams cannot be worked out. It is what the search did before this file
// existed, so a pattern the index cannot help with costs exactly what it used
// to.
//
// `files` is every file the caller was going to search. It is needed, rather
// than the answer being built out of the posting lists alone, because of the
// one way this can be silently and badly wrong: **a file the index has not read
// yet is in no posting list**, so an answer built only from the lists excludes
// it. That is not a slow search, it is a search that finds nothing - for every
// file, for as long as the index takes to build, and forever for any file that
// could not be read. So anything the index has no entry for is returned as a
// candidate, and the scanner looks at it.
func (self *trigramIndex) candidates(pattern string, files []string) map[string]bool {
	if trigramDisabled() {
		return nil
	}
	lits := requiredTrigrams(pattern)
	if len(lits) == 0 {
		return nil
	}
	self.mu.RLock()
	defer self.mu.RUnlock()

	// Everything the index has never heard of is searched regardless.
	out := map[string]bool{}
	known := 0
	for _, f := range files {
		if _, have := self.at[f]; have {
			known++
		} else {
			out[f] = true
		}
	}
	if known == 0 {
		// Nothing is indexed at all - a cold start. Say so plainly rather than
		// handing back a set that happens to be everything.
		return nil
	}

	var acc []int32
	empty := false
	for n, t := range lits {
		if n >= trigramMaxQuery {
			break
		}
		p := self.post[t]
		if len(p) == 0 {
			// A trigram nothing holds: no *indexed* file can match. The
			// unindexed ones already in `out` still have to be looked at.
			empty = true
			break
		}
		if acc == nil {
			acc = append([]int32{}, p...)
			continue
		}
		acc = intersect(acc, p)
		if len(acc) == 0 {
			empty = true
			break
		}
	}
	if !empty {
		for _, id := range acc {
			if int(id) < len(self.files) && self.files[id].Name != "" {
				out[self.files[id].Name] = true
			}
		}
	}
	return out
}

// Two ascending lists, and what is in both.
func intersect(a, b []int32) []int32 {
	out := a[:0]
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, a[i])
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return out
}

// The trigrams a line must contain for this pattern to match it.
//
// Worked out from the *mandatory* literal run of the expression, which is the
// conservative half of what Code Search does: if the answer is wrong it must be
// wrong by asking for too little, never too much, because asking for too much
// hides a real result and nothing downstream could ever notice.
//
// So anything that makes a literal optional or alternative - a `?`, a `|`, a
// `*` - ends the run rather than being reasoned about. A pattern with no run of
// three literal bytes gets nothing back and is scanned in full.
func requiredTrigrams(pattern string) []trigram {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil
	}
	lit := longestLiteral(re.Simplify())
	if len(lit) < 3 {
		return nil
	}
	b := []byte(strings.ToLower(lit))
	out := make([]trigram, 0, len(b)-2)
	seen := map[trigram]struct{}{}
	for i := 0; i+2 < len(b)+0; i++ {
		if i+2 >= len(b) {
			break
		}
		t := tri(b[i], b[i+1], b[i+2])
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// The longest run of literal bytes that every match must contain.
//
// Only two shapes produce one: a literal, and a concatenation of things
// starting with literals. Everything else - a repeat, an alternation, a
// character class - could match without any particular byte, so it breaks the
// run. Case-insensitive literals are taken as they are and lower cased by the
// caller, which is why `(?i)` costs nothing here.
func longestLiteral(re *syntax.Regexp) string {
	switch re.Op {
	case syntax.OpLiteral:
		return string(re.Rune)
	case syntax.OpCapture:
		if len(re.Sub) == 1 {
			return longestLiteral(re.Sub[0])
		}
	case syntax.OpConcat:
		best, run := "", ""
		for _, sub := range re.Sub {
			s := ""
			if sub.Op == syntax.OpLiteral {
				s = string(sub.Rune)
			} else if sub.Op == syntax.OpCapture && len(sub.Sub) == 1 && sub.Sub[0].Op == syntax.OpLiteral {
				s = string(sub.Sub[0].Rune)
			}
			if s != "" {
				run += s
				if len(run) > len(best) {
					best = run
				}
				continue
			}
			// Anything else could match nothing in particular, so the run ends.
			run = ""
		}
		return best
	}
	return ""
}

// ---------------------------------------------------------------------------
// Keeping it between runs
// ---------------------------------------------------------------------------

// Where the index is kept: beside the org files, under the name a dotfile has
// so that nothing walking the database for org files trips over it.
func trigramPath() string {
	root := "."
	if Conf().Server != nil && len(Conf().Server.OrgDirs) > 0 {
		root = Conf().Server.OrgDirs[0]
	}
	p, err := filepath.Abs(filepath.Join(root, ".orgs-index"))
	if err != nil {
		return filepath.Join(root, ".orgs-index")
	}
	return p
}

type trigramDisk struct {
	Format int
	Files  []trigramFile
	// Written as pairs rather than a map of slices: a gob of
	// map[trigram][]int32 is most of the file in keys, and the pairs compress
	// into the same shape on the way back in.
	Tris  []trigram
	Lists [][]int32
}

func (self *trigramIndex) load() {
	f, err := os.Open(trigramPath())
	if err != nil {
		return
	}
	defer f.Close()
	var d trigramDisk
	if err := gob.NewDecoder(f).Decode(&d); err != nil {
		return
	}
	if d.Format != trigramFormat || len(d.Tris) != len(d.Lists) {
		return
	}
	self.mu.Lock()
	defer self.mu.Unlock()
	self.files = d.Files
	self.at = make(map[string]int, len(d.Files))
	for i, e := range d.Files {
		if e.Name == "" {
			self.free = append(self.free, i)
			continue
		}
		self.at[e.Name] = i
	}
	self.post = make(map[trigram][]int32, len(d.Tris))
	for i, t := range d.Tris {
		self.post[t] = d.Lists[i]
	}
}

func (self *trigramIndex) save() error {
	self.mu.RLock()
	if !self.dirty {
		self.mu.RUnlock()
		return nil
	}
	d := trigramDisk{Format: trigramFormat, Files: append([]trigramFile{}, self.files...)}
	d.Tris = make([]trigram, 0, len(self.post))
	d.Lists = make([][]int32, 0, len(self.post))
	for t, p := range self.post {
		d.Tris = append(d.Tris, t)
		d.Lists = append(d.Lists, p)
	}
	self.mu.RUnlock()

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(d); err != nil {
		return err
	}
	// Through a temporary file and a rename, so a server killed mid-write
	// leaves the old index rather than half of a new one - which would be read
	// back as a valid index that is missing files.
	path := trigramPath()
	tmp, err := os.CreateTemp(filepath.Dir(path), ".orgs-index-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	self.mu.Lock()
	self.dirty = false
	self.mu.Unlock()
	return nil
}

// What the index is holding, for the endpoint that reports on it.
func (self *trigramIndex) stats() (files int, trigrams int, postings int) {
	self.mu.RLock()
	defer self.mu.RUnlock()
	files = len(self.at)
	trigrams = len(self.post)
	for _, p := range self.post {
		postings += len(p)
	}
	return
}

// ---------------------------------------------------------------------------
// Starting and stopping
// ---------------------------------------------------------------------------

var triStop chan struct{}

// Build the index in the background and keep it written down.
//
// Started rather than built on the first request, because the first request is
// somebody typing into a search box and a first keystroke that costs a walk of
// the whole database is the thing this is here to stop. A search arriving
// before it is ready is not a problem: an index that does not know about a file
// yet simply does not narrow, and the file is scanned.
func StartTrigramIndex() {
	if trigramDisabled() {
		fmt.Fprintf(os.Stderr, "TRIGRAM: off (ORGS_NO_TRIGRAM)\n")
		return
	}
	idx := getTrigramIndex()
	triStop = make(chan struct{})
	go func() {
		idx.sync()
		if err := idx.save(); err != nil {
			fmt.Fprintf(os.Stderr, "TRIGRAM: could not write the index: %v\n", err)
		}
		// Written every so often rather than on every change: a save costs one
		// file's trigrams in memory and the whole index on disk, and a database
		// being edited would otherwise rewrite it on every keystroke of an
		// editor with autosave on.
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-triStop:
				idx.save()
				return
			case <-t.C:
				idx.sync()
				idx.save()
			}
		}
	}()
}

func StopTrigramIndex() {
	if triStop != nil {
		close(triStop)
		triStop = nil
	}
}
