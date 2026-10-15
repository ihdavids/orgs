package orgs

/* SDOC: Editing
* Detangling and Auto-Tangling

  ** Detangle

  Tangling writes code out of an org file; detangling brings edits made to the
  tangled file back into the org blocks they came from (org's
  =org-babel-detangle=). It needs the markers a tangle writes with
  =:comments link= (or =both=):

  #+BEGIN_EXAMPLE
  # [[file:setup.org::*Install][Install:1]]
  brew install ripgrep
  # Install:1 ends here
  #+END_EXAMPLE

  =orgs detangle install.sh= reads the file, finds each marked section, and
  writes its code back into that block with a line edit - the block's own
  indentation kept, org's comma escape put back where a line would read as a
  heading. =-dry-run= says what would change. A block whose code has noweb
  =<<references>>= is left alone: the tangled file holds their expansion, and
  that cannot be put back as one block's code.

  Put =:comments link= on every block with =#+PROPERTY: header-args :comments link=.

  ** Auto-tangle

  A file that says so is tangled every time it is saved:

  #+BEGIN_EXAMPLE
  #+auto_tangle: t
  #+END_EXAMPLE

  Only files that ask: nothing is tangled on its own otherwise. A tangled
  file that says the same as before is not rewritten. One that has been edited
  by hand since orgs last wrote it is *not overwritten*: the tangle stops for
  that file and says so (an event, and the server log), so detangle it, or
  tangle it by hand with =orgs tangle -w= to throw the edit away.
EDOC */

import (
	"crypto/sha256"
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
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs/tangle"
	"github.com/ihdavids/orgs/internal/common"
)

// ----------------------------------------------------------------------------
// Detangle
// ----------------------------------------------------------------------------

var srcBeginLineRe = regexp.MustCompile(`(?i)^(\s*)#\+BEGIN_SRC\b`)

// detangleOrgFile is the org file a section's link names: relative to the
// tangled file, else - the tangled file was moved, or came from another
// machine - by name among the files the server has.
func detangleOrgFile(tangledPath, link string) *common.OrgFile {
	if !filepath.IsAbs(link) && tangledPath != "" {
		if f := GetDb().FindByFile(filepath.Clean(filepath.Join(filepath.Dir(tangledPath), link))); f != nil {
			return f
		}
	}
	if f := GetDb().FindByFile(filepath.Clean(link)); f != nil {
		return f
	}
	return GetDb().FindByFile(filepath.Base(link))
}

// detangleBody is the code a block will hold: escaped, and indented the way the
// block's code already is (or under its #+BEGIN_SRC line when it is empty).
func detangleBody(lines []string, begin, end int, code []string, preserve bool) []string {
	indent := ""
	if !preserve {
		set := false
		for _, l := range lines[begin+1 : end] {
			if strings.TrimSpace(l) == "" {
				continue
			}
			n := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
			if !set || len(n) < len(indent) {
				indent, set = n, true
			}
		}
		if !set {
			if m := srcBeginLineRe.FindStringSubmatch(lines[begin]); m != nil {
				indent = m[1]
			}
		}
	}
	out := make([]string, 0, len(code))
	for _, l := range code {
		if strings.TrimSpace(l) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, indent+tangle.EscapeLine(l))
	}
	return out
}

// Detangle is the whole of POST /detangle.
func Detangle(req *common.DetangleRequest) common.DetangleResult {
	res := common.DetangleResult{Ok: true, Blocks: []common.DetangledBlock{}}
	text := req.Text
	tangled := req.File
	if text == "" {
		// Read here only inside the org directories: anything else the client
		// reads and sends, so a request cannot have the server read any file.
		p, ok := insideOrgDirs(req.File)
		if !ok {
			return common.DetangleResult{Msg: "send the tangled file's text (Text): the server only reads files inside the org directories"}
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return common.DetangleResult{Msg: err.Error()}
		}
		text, tangled = string(b), p
	}
	sections := tangle.ParseMarkers(text)
	if len(sections) == 0 {
		return common.DetangleResult{Msg: "no [[file:...]] markers in it - tangle with :comments link to be able to detangle"}
	}
	// Grouped by org file, so each is read once and written once.
	type edit struct {
		begin, end int
		body       []string
		idx        int
	}
	byFile := map[string][]edit{}
	files := map[string]*common.OrgFile{}
	for _, sec := range sections {
		out := common.DetangledBlock{Label: sec.Label, TangledLine: sec.Line}
		f := detangleOrgFile(tangled, sec.File)
		if f == nil || f.Doc == nil {
			out.Status, out.Msg = "missing", fmt.Sprintf("no org file %s", sec.File)
			res.Blocks = append(res.Blocks, out)
			continue
		}
		out.File = f.Filename
		files[f.Filename] = f
		if sec.Nested {
			out.Status, out.Msg = "skipped", "it holds other blocks' markers (a noweb expansion)"
			res.Blocks = append(res.Blocks, out)
			continue
		}
		var ref *tangle.BlockRef
		dup := 0
		for _, b := range tangle.Blocks(f.Doc, f.Filename) {
			if b.Label == sec.Label {
				if ref == nil {
					bb := b
					ref = &bb
				}
				dup++
			}
		}
		if ref == nil {
			out.Status, out.Msg = "missing", fmt.Sprintf("no block %q in %s", sec.Label, filepath.Base(f.Filename))
			res.Blocks = append(res.Blocks, out)
			continue
		}
		out.Line = ref.Row
		if ref.Noweb {
			out.Status, out.Msg = "skipped", "the block has <<noweb>> references; its tangled text is their expansion"
			res.Blocks = append(res.Blocks, out)
			continue
		}
		if dup > 1 {
			out.Msg = fmt.Sprintf("%d blocks are called %q; the first was used", dup, sec.Label)
		}
		res.Blocks = append(res.Blocks, out)
		byFile[f.Filename] = append(byFile[f.Filename], edit{begin: ref.Row, body: sec.Body, idx: len(res.Blocks) - 1})
	}

	names := make([]string, 0, len(byFile))
	for n := range byFile {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		lines, err := readFileLines(name)
		if err != nil {
			res.Ok, res.Msg = false, err.Error()
			continue
		}
		edits := byFile[name]
		// Bottom up, so a block that changes length does not move the ones
		// above it.
		sort.Slice(edits, func(i, j int) bool { return edits[i].begin > edits[j].begin })
		changed := false
		for _, e := range edits {
			b := &res.Blocks[e.idx]
			begin := e.begin
			if begin < 0 || begin >= len(lines) || !srcBeginLineRe.MatchString(lines[begin]) {
				b.Status, b.Msg = "missing", "the block is not where the file has it - reload and try again"
				continue
			}
			end := begin + 1
			for end < len(lines) && !srcEndRe.MatchString(lines[end]) {
				end++
			}
			if end >= len(lines) {
				b.Status, b.Msg = "missing", "the block has no #+END_SRC"
				continue
			}
			preserve := false
			for _, r := range tangle.Blocks(files[name].Doc, name) {
				if r.Row == begin {
					preserve = r.Preserve
				}
			}
			body := detangleBody(lines, begin, end, e.body, preserve)
			if strings.Join(lines[begin+1:end], "\n") == strings.Join(body, "\n") {
				b.Status = "unchanged"
				continue
			}
			b.Status = "updated"
			out := append([]string{}, lines[:begin+1]...)
			out = append(out, body...)
			out = append(out, lines[end:]...)
			lines = out
			changed = true
		}
		if changed && !req.DryRun {
			if err := writeLines(name, lines); err != nil {
				res.Ok, res.Msg = false, err.Error()
				continue
			}
			res.Written = append(res.Written, name)
		}
	}
	updated, problems := 0, 0
	for _, b := range res.Blocks {
		switch b.Status {
		case "updated":
			updated++
		case "missing", "skipped":
			problems++
		}
	}
	if res.Msg == "" {
		res.Msg = fmt.Sprintf("%d of %d blocks changed", updated, len(res.Blocks))
		if problems > 0 {
			res.Msg += fmt.Sprintf(", %d could not be put back", problems)
		}
	}
	return res
}

/* SDOC: API
* POST /detangle — Put A Tangled File's Edits Back

	#+BEGIN_EXAMPLE
	{ "File": "/home/me/bin/install.sh", "Text": "<the tangled file>", "DryRun": false }
	#+END_EXAMPLE

	Finds the =[[file:...][Label]]= / =Label ends here= markers a tangle with
	=:comments link= writes, and puts each section's code back into the block
	it came from, as a line edit. =File= is where the tangled file is (its
	links are relative to it); =Text= is its contents, which a client sends -
	without it the server reads =File= itself, but only inside the org
	directories. =DryRun= changes nothing.

	*Response:* each block with =Status= - =updated=, =unchanged=, =skipped=
	(a noweb expansion), =missing= (no such block) - its org =File= and =Line=,
	and =Written=, the org files changed.
EDOC */
func PostDetangle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.DetangleRequest
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.DetangleResult{Msg: err.Error()})
		return
	}
	dynJson(w, Detangle(&req))
}

// ----------------------------------------------------------------------------
// Auto-tangle
// ----------------------------------------------------------------------------

// What orgs last wrote to each tangled file, as a hash, so a file somebody has
// edited by hand since is told apart from one that only needs bringing up to
// date. Kept across restarts.
type tangleLedger struct {
	mu      sync.Mutex
	loaded  bool
	written map[string]string
	timers  map[string]*time.Timer
}

var tangles = &tangleLedger{written: map[string]string{}, timers: map[string]*time.Timer{}}

func tangleLedgerPath() string {
	return path.Join(Conf().PlugManager.HomeDir, "tangle_data.json")
}

func hashOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:12])
}

func (t *tangleLedger) loadLocked() {
	if t.loaded {
		return
	}
	t.loaded = true
	if b, err := os.ReadFile(tangleLedgerPath()); err == nil {
		json.Unmarshal(b, &t.written)
	}
}

// record is tangle.OnWrite: every file a tangle writes, or finds already right.
func (t *tangleLedger) record(p string, content []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loadLocked()
	t.written[p] = hashOf(content)
	if b, err := json.MarshalIndent(t.written, "", " "); err == nil {
		os.WriteFile(tangleLedgerPath(), b, 0644)
	}
}

// editedByHand says the file on disk is neither what orgs last wrote to it
// nor what it is about to write: somebody changed it.
func (t *tangleLedger) editedByHand(p string, next string) bool {
	disk, err := os.ReadFile(p)
	if err != nil || string(disk) == next {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loadLocked()
	last, ok := t.written[p]
	// A file orgs never wrote is the user's first auto-tangle of it: they
	// asked for the file to follow the org one.
	return ok && last != hashOf(disk)
}

func init() {
	tangle.OnWrite = func(p string, content []byte) { tangles.record(p, content) }
	// <<name(args)>> in a tangled block is the block's result, as a #+CALL:
	// would give it: run here, where blocks can run (babel.enable decides).
	tangle.Evaluate = func(orgPath, name, args string) (string, error) {
		f := GetDb().FindByFile(orgPath)
		if f == nil {
			return "", fmt.Errorf("no file %s", orgPath)
		}
		job, _, err := callJob(name, "", args, "", f, f.Filename)
		if err != nil {
			return "", err
		}
		res, err := runJob(job)
		if err != nil {
			return "", err
		}
		if !res.Ok {
			return "", fmt.Errorf("%s: %s", name, res.Msg)
		}
		if res.Kind == "table" {
			return res.Result, nil
		}
		return strings.TrimRight(res.Raw, "\n"), nil
	}
}

func truthyKeyword(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "t", "yes", "true", "on", "1":
		return true
	}
	return false
}

// autoTangleAfterLoad is called for every re-read of a file that was already
// loaded (a save, not the server starting up): a file saying #+auto_tangle: t
// is tangled once the burst of filesystem events a save makes has settled.
func autoTangleAfterLoad(f *common.OrgFile) {
	if f == nil || f.Doc == nil || !truthyKeyword(f.Doc.Get("AUTO_TANGLE")) {
		return
	}
	name := f.Filename
	tangles.mu.Lock()
	defer tangles.mu.Unlock()
	if t, ok := tangles.timers[name]; ok {
		t.Stop()
	}
	tangles.timers[name] = time.AfterFunc(400*time.Millisecond, func() {
		tangles.mu.Lock()
		delete(tangles.timers, name)
		tangles.mu.Unlock()
		AutoTangle(name)
	})
}

// AutoTangle tangles one file the way a save does: every target written that
// needs it, none that somebody edited by hand.
func AutoTangle(name string) common.AutoTangled {
	out := common.AutoTangled{File: name}
	f := GetDb().FindByFile(name)
	if f == nil || f.Doc == nil {
		out.Msg = "no such file"
		return out
	}
	res, err := tangle.Assemble(f.Doc, f.Filename)
	if err != nil {
		out.Msg = err.Error()
		return out
	}
	for _, tf := range res.Files {
		if tangles.editedByHand(tf.Filename, tf.Content) {
			out.Conflicts = append(out.Conflicts, tf.Filename)
			continue
		}
		wrote, err := tangle.WriteFile(tf)
		if err != nil {
			out.Msg = err.Error()
			continue
		}
		if wrote {
			out.Written = append(out.Written, tf.Filename)
		}
	}
	out.Ok = out.Msg == "" && len(out.Conflicts) == 0
	switch {
	case len(out.Conflicts) > 0:
		out.Msg = fmt.Sprintf("not tangled over a hand edit: %s - detangle it (orgs detangle), or overwrite it (orgs tangle -w)", strings.Join(out.Conflicts, ", "))
		fmt.Fprintf(os.Stderr, "AUTO-TANGLE %s: %s\n", filepath.Base(name), out.Msg)
	case len(out.Written) > 0:
		out.Msg = fmt.Sprintf("wrote %s", strings.Join(out.Written, ", "))
	}
	if len(out.Written) > 0 || len(out.Conflicts) > 0 {
		Publish(Event{Kind: EventTangled, File: name, Msg: out.Msg})
	}
	return out
}

// EventTangled: an auto-tangle wrote files, or would not overwrite one.
const EventTangled = "tangled"
