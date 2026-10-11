//lint:file-ignore ST1006 allow the use of self
package orgs

// Structural refactors: renaming a tag everywhere, splitting a heading out to a
// file of its own, merging one heading into another, and turning a list into
// headings.
//
// Every one is a set of line edits (see the root CLAUDE.md, Traps: line edits)
// worked out against the files in memory first. That one computation answers
// both halves of each endpoint: the preview hands the before and after text
// back without writing, and the apply works it out again from the files as
// they are now and writes only if it comes to exactly what the preview showed.
// "Exactly" is a token over every file's hash before and after, so a file
// edited in between - by hand, by the watcher, by another tab - is refused
// rather than written over with a change nobody saw.
//
// Wire types are internal/common/refactor.go; clients are worg's Refactor tab
// and cmd/oc/commands/refactor.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

// registerRefactorRoutes adds the endpoints for structural refactors (tag rename, split, merge, list to headings). Called from RestApi.
func registerRefactorRoutes(api *mux.Router) {
	api.HandleFunc("/refactor/tags", RequestRefactorTags).Methods("GET")
	api.HandleFunc("/refactor/lists", RequestRefactorLists).Methods("GET")
	api.HandleFunc("/refactor/tag/preview", PostTagRenamePreview).Methods("POST")
	api.HandleFunc("/refactor/tag/apply", PostTagRenameApply).Methods("POST")
	api.HandleFunc("/refactor/split/preview", PostSplitPreview).Methods("POST")
	api.HandleFunc("/refactor/split/apply", PostSplitApply).Methods("POST")
	api.HandleFunc("/refactor/merge/preview", PostMergePreview).Methods("POST")
	api.HandleFunc("/refactor/merge/apply", PostMergeApply).Methods("POST")
	api.HandleFunc("/refactor/list/preview", PostListToHeadingsPreview).Methods("POST")
	api.HandleFunc("/refactor/list/apply", PostListToHeadingsApply).Methods("POST")
}

// ---------------------------------------------------------------------------
// The plan: files read once, edited in memory, written only on apply
// ---------------------------------------------------------------------------

type refactorEdit struct {
	path    string
	raw     string   // the file as read; empty for a file the plan makes
	lines   []string // the text as it will be written
	created bool
}

type refactorPlan struct {
	op       string
	files    map[string]*refactorEdit
	order    []string
	summary  string
	warnings []string
	mentions []common.RefactorMention
	newId    string
	newFile  string
}

func newRefactorPlan(op string) *refactorPlan {
	return &refactorPlan{op: op, files: map[string]*refactorEdit{}}
}

// The lines of a file the way recordLines and writeLines see them: split on
// newlines, the final newline not an empty line of its own.
func splitOrgText(raw string) []string {
	return strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
}

// get answers with the working copy of a file's lines, reading it the first
// time. The slice is the caller's to change; put stores it back.
func (self *refactorPlan) get(path string) ([]string, error) {
	if e, ok := self.files[path]; ok {
		return append([]string(nil), e.lines...), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := string(b)
	self.files[path] = &refactorEdit{path: path, raw: raw, lines: splitOrgText(raw)}
	self.order = append(self.order, path)
	return splitOrgText(raw), nil
}

func (self *refactorPlan) put(path string, lines []string) {
	if e, ok := self.files[path]; ok {
		e.lines = lines
	}
}

// adopt records a file already read and changed by the caller, for the edits
// that look at every file and change few of them: holding every file in the
// plan just to drop the unchanged ones again would be most of the work.
func (self *refactorPlan) adopt(path, raw string, lines []string) {
	if _, ok := self.files[path]; ok {
		self.files[path].lines = lines
		return
	}
	self.files[path] = &refactorEdit{path: path, raw: raw, lines: lines}
	self.order = append(self.order, path)
}

func (self *refactorPlan) create(path string, lines []string) {
	self.files[path] = &refactorEdit{path: path, lines: lines, created: true}
	self.order = append([]string{path}, self.order...)
	self.newFile = path
}

func (self *refactorPlan) warn(format string, args ...interface{}) {
	self.warnings = append(self.warnings, fmt.Sprintf(format, args...))
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:20]
}

// changed reports whether a file's text differs from what was read. Compared
// without the final newline, so a file that lacked one is not "changed" by a
// refactor that never touched it.
func (self *refactorEdit) changed() bool {
	return self.created || strings.Join(self.lines, "\n") != strings.TrimSuffix(self.raw, "\n")
}

func (self *refactorEdit) after() string {
	return strings.Join(self.lines, "\n") + "\n"
}

// result is the plan as the wire sees it, with the token an apply must match.
func (self *refactorPlan) result() common.RefactorPlan {
	res := common.RefactorPlan{Ok: true, Op: self.op, Summary: self.summary,
		Warnings: self.warnings, Mentions: self.mentions, NewId: self.newId, NewFile: self.newFile}
	tok := sha256.New()
	fmt.Fprintf(tok, "%s\n", self.op)
	for _, path := range self.order {
		e := self.files[path]
		if !e.changed() {
			continue
		}
		after := e.after()
		before := e.raw
		f := common.RefactorFile{Filename: path, Created: e.created, Before: before, After: after, Hash: shortHash(before)}
		if e.created {
			f.Hash = ""
		}
		f.Patch, f.Added, f.Removed = unifiedPatch(displayPath(path), splitOrgTextOrNone(before, e.created), e.lines, e.created)
		res.Files = append(res.Files, f)
		fmt.Fprintf(tok, "%s\x00%s\x00%s\x00%v\n", path, f.Hash, shortHash(after), e.created)
	}
	if res.Files == nil {
		res.Files = []common.RefactorFile{}
	}
	res.Token = hex.EncodeToString(tok.Sum(nil))[:32]
	return res
}

func splitOrgTextOrNone(raw string, created bool) []string {
	if created {
		return nil
	}
	return splitOrgText(raw)
}

// A path as a person reads it in a patch header: from its org directory's
// own name down.
func displayPath(path string) string {
	for _, d := range Conf().Server.OrgDirs {
		root, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(filepath.Join(filepath.Base(root), rel))
		}
	}
	return filepath.Base(path)
}

// One apply at a time: two refactors checking the same files and then writing
// them in turn would each pass the check against what the other is about to
// overwrite.
var refactorMu sync.Mutex

// apply writes the plan, if it is the plan the preview showed.
func (self *refactorPlan) apply(args common.RefactorApply) common.RefactorPlan {
	res := self.result()
	if args.Token == "" {
		return common.RefactorPlan{Files: []common.RefactorFile{}, Ok: false, Op: self.op, Msg: "an apply needs the Token its preview answered with - preview first"}
	}
	if res.Token != args.Token {
		// Name the files that moved under the preview when the client said
		// what it saw; otherwise the change itself came out different (a
		// file that now holds the tag, a heading that has gone).
		stale := []string{}
		for path, h := range args.Expect {
			cur := ""
			if b, err := os.ReadFile(path); err == nil {
				cur = shortHash(string(b))
			}
			if h != cur {
				stale = append(stale, displayPath(path))
			}
		}
		sort.Strings(stale)
		msg := "the files changed since the preview, so the change is no longer the one you saw - preview again"
		if len(stale) > 0 {
			msg = "changed since the preview: " + strings.Join(stale, ", ") + " - preview again"
		}
		return common.RefactorPlan{Files: []common.RefactorFile{}, Ok: false, Stale: true, Op: self.op, Msg: msg}
	}
	// New files first, and made exclusively: a split's link has something to
	// land on before it is written, and a file that appeared since the
	// preview is refused before anything else has been touched.
	for _, path := range self.order {
		e := self.files[path]
		if !e.created {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return common.RefactorPlan{Files: []common.RefactorFile{}, Ok: false, Op: self.op, Msg: err.Error()}
		}
		fh, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return common.RefactorPlan{Files: []common.RefactorFile{}, Ok: false, Op: self.op, Msg: fmt.Sprintf("%s: %v - a refactor never writes over a file", displayPath(path), err)}
		}
		_, err = fh.WriteString(e.after())
		fh.Close()
		if err != nil {
			return common.RefactorPlan{Files: []common.RefactorFile{}, Ok: false, Op: self.op, Msg: err.Error()}
		}
	}
	written := 0
	for _, path := range self.order {
		e := self.files[path]
		if e.created || !e.changed() {
			continue
		}
		if err := writeLines(path, e.lines); err != nil {
			res.Ok = false
			res.Msg = fmt.Sprintf("wrote %d files, then %s failed: %v", written, displayPath(path), err)
			return res
		}
		written++
	}
	for _, path := range self.order {
		if self.files[path].created {
			GetDb().ReloadFile(path)
		}
	}
	// What was written is in the files now; an apply's answer is the record of
	// it, not a second copy of every text.
	for i := range res.Files {
		res.Files[i].Before, res.Files[i].After = "", ""
	}
	res.Applied = true
	res.Msg = self.summary
	return res
}

// ---------------------------------------------------------------------------
// Diffs, for the terminal: Myers over lines, written as a unified patch
// ---------------------------------------------------------------------------

type diffOp struct {
	kind byte // ' ', '-', '+'
	a, b int  // index in the old and new lines
}

// lineDiff is the shortest edit script between two lists of lines. The common
// ends are taken off first, which leaves the middle small for every edit a
// refactor makes; past maxD differences it gives up and calls the middle a
// replacement, which is still a correct patch, only a longer one.
func lineDiff(a, b []string) []diffOp {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	ops := make([]diffOp, 0, len(a)+8)
	for i := 0; i < pre; i++ {
		ops = append(ops, diffOp{' ', i, i})
	}
	for _, op := range myersDiff(a[pre:len(a)-suf], b[pre:len(b)-suf]) {
		op.a += pre
		op.b += pre
		ops = append(ops, op)
	}
	for i := 0; i < suf; i++ {
		ops = append(ops, diffOp{' ', len(a) - suf + i, len(b) - suf + i})
	}
	return ops
}

const maxD = 4000

func myersDiff(a, b []string) []diffOp {
	n, m := len(a), len(b)
	replace := func() []diffOp {
		ops := []diffOp{}
		for i := 0; i < n; i++ {
			ops = append(ops, diffOp{'-', i, 0})
		}
		for j := 0; j < m; j++ {
			ops = append(ops, diffOp{'+', n, j})
		}
		return ops
	}
	if n == 0 || m == 0 {
		return replace()
	}
	// trace[d] holds the furthest x on each diagonal k in -d..d after step d,
	// at index k+d. Kept per step rather than as one array as wide as the
	// files, so the memory is the square of the differences, not of the size.
	trace := [][]int{}
	prev := []int{0, 0} // a step -1 with diagonal 1 at x=0, as Myers seeds it
	at := func(v []int, d, k int) int { return v[k+d] }
	for d := 0; d <= n+m && d <= maxD; d++ {
		v := make([]int, 2*d+1)
		for k := -d; k <= d; k += 2 {
			var x int
			pd := d - 1
			if d == 0 {
				x = 0
			} else if k == -d || (k != d && at(prev, pd, k-1) < at(prev, pd, k+1)) {
				x = at(prev, pd, k+1)
			} else {
				x = at(prev, pd, k-1) + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[k+d] = x
			if x >= n && y >= m {
				trace = append(trace, v)
				return myersBacktrack(trace, n, m)
			}
		}
		trace = append(trace, v)
		prev = v
	}
	return replace()
}

func myersBacktrack(trace [][]int, n, m int) []diffOp {
	ops := []diffOp{}
	x, y := n, m
	for d := len(trace) - 1; d > 0; d-- {
		prev := trace[d-1]
		pd := d - 1
		k := x - y
		var pk int
		if k == -d || (k != d && prev[k-1+pd] < prev[k+1+pd]) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := prev[pk+pd]
		py := px - pk
		for x > px && y > py {
			x--
			y--
			ops = append(ops, diffOp{' ', x, y})
		}
		if x == px {
			y--
			ops = append(ops, diffOp{'+', x, y})
		} else {
			x--
			ops = append(ops, diffOp{'-', x, y})
		}
	}
	for x > 0 && y > 0 {
		x--
		y--
		ops = append(ops, diffOp{' ', x, y})
	}
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops
}

// unifiedPatch writes the change as `diff -u` would, with three lines of
// context, and counts what it added and took away.
func unifiedPatch(name string, a, b []string, created bool) (string, int, int) {
	ops := lineDiff(a, b)
	added, removed := 0, 0
	for _, op := range ops {
		switch op.kind {
		case '+':
			added++
		case '-':
			removed++
		}
	}
	if added == 0 && removed == 0 {
		return "", 0, 0
	}
	var sb strings.Builder
	if created {
		fmt.Fprintf(&sb, "--- /dev/null\n+++ b/%s\n", name)
	} else {
		fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", name, name)
	}
	const ctx = 3
	i := 0
	for i < len(ops) {
		for i < len(ops) && ops[i].kind == ' ' {
			i++
		}
		if i >= len(ops) {
			break
		}
		start := i - ctx
		if start < 0 {
			start = 0
		}
		// Run on while the next change is close enough to share context.
		end := i
		for j := i; j < len(ops); j++ {
			if ops[j].kind != ' ' {
				end = j
				continue
			}
			if j-end > 2*ctx {
				break
			}
		}
		stop := end + ctx + 1
		if stop > len(ops) {
			stop = len(ops)
		}
		aStart, bStart, aLen, bLen := -1, -1, 0, 0
		for _, op := range ops[start:stop] {
			if op.kind != '+' {
				if aStart < 0 {
					aStart = op.a
				}
				aLen++
			}
			if op.kind != '-' {
				if bStart < 0 {
					bStart = op.b
				}
				bLen++
			}
		}
		if aStart < 0 {
			aStart = ops[start].a
		}
		if bStart < 0 {
			bStart = ops[start].b
		}
		fmt.Fprintf(&sb, "@@ -%s +%s @@\n", hunkRange(aStart, aLen), hunkRange(bStart, bLen))
		for _, op := range ops[start:stop] {
			switch op.kind {
			case ' ':
				sb.WriteString(" " + a[op.a] + "\n")
			case '-':
				sb.WriteString("-" + a[op.a] + "\n")
			case '+':
				sb.WriteString("+" + b[op.b] + "\n")
			}
		}
		i = stop
	}
	return sb.String(), added, removed
}

func hunkRange(start, n int) string {
	if n == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	if n == 1 {
		return fmt.Sprintf("%d", start+1)
	}
	return fmt.Sprintf("%d,%d", start+1, n)
}

// ---------------------------------------------------------------------------
// Reading lines: headlines, tags, indentation
// ---------------------------------------------------------------------------

// headLevel is the level of a headline line, 0 for anything else. Stars then a
// space, as subtreeEndRow reads them: `**bold**` in column zero is not one.
func headLevel(l string) int {
	n := 0
	for n < len(l) && l[n] == '*' {
		n++
	}
	if n > 0 && n < len(l) && (l[n] == ' ' || l[n] == '\t') {
		return n
	}
	return 0
}

func blankLine(l string) bool { return strings.TrimSpace(l) == "" }

// The tags written on a headline line, in order.
func headlineTagList(line string) []string {
	m := headlineHeadRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	sub := headlineTagsRe.FindStringSubmatch(line[len(m[0]):])
	if sub == nil {
		return nil
	}
	out := []string{}
	for _, t := range strings.Split(strings.Trim(sub[1], ":"), ":") {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// The title as written on a headline line: no stars, keyword, priority or
// tags, but links and markup kept.
func rawHeadlineTitle(line, status string) string {
	m := headlineHeadRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	rest := line[len(m[0]):]
	if loc := headlineTagsRe.FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}
	if status != "" && (rest == status || strings.HasPrefix(rest, status+" ") || strings.HasPrefix(rest, status+"\t")) {
		rest = strings.TrimLeft(rest[len(status):], " \t")
	}
	if pm := headlinePrioRe.FindString(rest); pm != "" {
		rest = rest[len(pm):]
	}
	return strings.TrimSpace(rest)
}

// The last row of a subtree that is not a blank line. The blank lines after a
// heading separate it from the next one, and stay where they are when the
// heading moves.
func trimBlankEnd(lines []string, from, to int) int {
	for to > from && blankLine(lines[to]) {
		to--
	}
	return to
}

// The first child headline under the heading at from, or end+1.
func firstChildRow(lines []string, from, end int) int {
	for i := from + 1; i <= end && i < len(lines); i++ {
		if headLevel(lines[i]) > 0 {
			return i
		}
	}
	return end + 1
}

func leadingSpaces(l string) int {
	n := 0
	for n < len(l) && l[n] == ' ' {
		n++
	}
	return n
}

// The indent a new drawer gets: column zero when the file already writes its
// drawers there (what Emacs does since org 9.5), the heading's depth when it
// indents them, as the record and id writers do.
func drawerIndentFor(lines []string, lvl int) string {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.EqualFold(t, ":PROPERTIES:") || strings.EqualFold(t, ":LOGBOOK:") {
			if leadingSpaces(l) == 0 && !strings.HasPrefix(l, "\t") {
				return ""
			}
			return indentOf(lvl)
		}
	}
	return indentOf(lvl)
}

// relevel moves a run of lines by delta levels: every headline gains or loses
// that many stars (never fewer than one), and the text around them follows
// when it is indented to its heading, the way org's promote and demote do
// under org-adapt-indentation.
//
// The text moves as one piece - every line by the same amount, worked out
// from the least indented of them - so nothing inside a source block changes
// shape against its neighbours. A file written in column zero has a least
// indent of zero and nothing but the stars changes.
func relevel(lines []string, delta int) []string {
	out := append([]string(nil), lines...)
	if delta == 0 {
		return out
	}
	minInd := -1
	for _, l := range out {
		if headLevel(l) > 0 || blankLine(l) {
			continue
		}
		n := leadingSpaces(l)
		if minInd < 0 || n < minInd {
			minInd = n
		}
	}
	for i, l := range out {
		if n := headLevel(l); n > 0 {
			nl := n + delta
			if nl < 1 {
				nl = 1
			}
			out[i] = strings.Repeat("*", nl) + l[n:]
			continue
		}
		if blankLine(l) || minInd <= 0 {
			continue
		}
		if delta < 0 {
			cut := -delta
			if cut > minInd {
				cut = minInd
			}
			out[i] = l[cut:]
		} else {
			out[i] = strings.Repeat(" ", delta) + l
		}
	}
	return out
}

// The files a refactor that reads everything reads: plain org text. An
// encrypted file is a blob to a line edit.
func refactorFiles() []string {
	out := []string{}
	for _, f := range GetDb().GetFiles() {
		low := strings.ToLower(f)
		if strings.HasSuffix(low, ".org") || strings.HasSuffix(low, ".org_archive") {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

var blockBeginRe = regexp.MustCompile(`(?i)^\s*#\+begin_`)
var blockEndRe = regexp.MustCompile(`(?i)^\s*#\+end_`)

// ---------------------------------------------------------------------------
// Tag rename, merge and delete
// ---------------------------------------------------------------------------

var (
	tagNameRe      = regexp.MustCompile(`^[\p{L}\p{N}_@#%]+$`)
	fileTagsLineRe = regexp.MustCompile(`(?i)^(\s*#\+filetags:)([ \t]*)(.*?)([ \t]*)$`)
	tagsLineRe     = regexp.MustCompile(`(?i)^(\s*#\+tags:)(.*)$`)
	tokenRe        = regexp.MustCompile(`\S+`)
)

// renameTag renames old in a list of tags. With nw empty the tag goes; with nw
// already in the list it goes too, since writing nw a second time would be a
// duplicate - that is the merge.
func renameTag(tags []string, old, nw string) (out []string, hit bool, merged bool) {
	have := false
	for _, t := range tags {
		if t == old {
			hit = true
		}
		if nw != "" && t == nw {
			have = true
		}
	}
	if !hit {
		return tags, false, false
	}
	for _, t := range tags {
		if t == old {
			if nw == "" || have {
				continue
			}
			out = append(out, nw)
			continue
		}
		out = append(out, t)
	}
	return out, true, have
}

// A #+FILETAGS: value with old renamed, in the style it was written in
// (`:a:b:` or `a b`). ok is false when old is not in it.
func renameFileTags(line, old, nw string) (string, bool, bool) {
	m := fileTagsLineRe.FindStringSubmatch(line)
	if m == nil {
		return line, false, false
	}
	val := m[3]
	colon := strings.Contains(val, ":")
	var tags []string
	if colon {
		for _, t := range strings.Split(strings.Trim(val, ": \t"), ":") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
	} else {
		tags = strings.Fields(val)
	}
	out, hit, merged := renameTag(tags, old, nw)
	if !hit {
		return line, false, false
	}
	if len(out) == 0 {
		return m[1], true, merged
	}
	gap := m[2]
	if gap == "" {
		gap = " "
	}
	if colon {
		return m[1] + gap + ":" + strings.Join(out, ":") + ":", true, merged
	}
	return m[1] + gap + strings.Join(out, " "), true, merged
}

// A #+TAGS: line with old renamed. Each word is a tag with an optional fast
// key, `work(w)`, or a group marker - `{`, `}`, `[`, `]`, `:` - left alone.
// Spacing is kept: only the words that change are touched.
func renameTagsLine(line, old, nw string) (string, bool, bool) {
	m := tagsLineRe.FindStringSubmatch(line)
	if m == nil {
		return line, false, false
	}
	val := m[2]
	name := func(tok string) string {
		if i := strings.Index(tok, "("); i > 0 {
			return tok[:i]
		}
		return tok
	}
	locs := tokenRe.FindAllStringIndex(val, -1)
	hit, have := false, false
	for _, l := range locs {
		n := name(val[l[0]:l[1]])
		if n == old {
			hit = true
		}
		if nw != "" && n == nw {
			have = true
		}
	}
	if !hit {
		return line, false, false
	}
	// Right to left, so the offsets still to come stay where they were.
	for i := len(locs) - 1; i >= 0; i-- {
		l := locs[i]
		tok := val[l[0]:l[1]]
		if name(tok) != old {
			continue
		}
		if nw == "" || have {
			// The word and the space in front of it.
			s := l[0]
			for s > 0 && (val[s-1] == ' ' || val[s-1] == '\t') {
				s--
			}
			val = val[:s] + val[l[1]:]
			continue
		}
		val = val[:l[0]] + nw + tok[len(old):] + val[l[1]:]
	}
	return m[1] + val, true, have
}

// tagMentions are the saved queries, filters and tag groups that name a tag.
// Reported, never changed: a query is somebody's own text, and "Tags('home')"
// inside a longer expression can mean something a rename should not decide.
func tagMentions(tag string) []common.RefactorMention {
	re := regexp.MustCompile(`(^|[^\p{L}\p{N}_@#%])` + regexp.QuoteMeta(tag) + `($|[^\p{L}\p{N}_@#%])`)
	out := []common.RefactorMention{}
	for name, q := range Conf().Filters {
		if re.MatchString(q) {
			out = append(out, common.RefactorMention{Kind: "filter", Name: name, Text: q})
		}
	}
	for name, tags := range Conf().TagGroups {
		in := name == tag
		for _, t := range tags {
			if t == tag {
				in = true
			}
		}
		if in {
			out = append(out, common.RefactorMention{Kind: "tag group", Name: name, Text: strings.Join(tags, " ")})
		}
	}
	if ext := GetExtensions(); ext != nil {
		ext.mu.RLock()
		for user, u := range ext.Users {
			if u == nil {
				continue
			}
			for _, sq := range u.StoredQueries {
				if re.MatchString(sq.Query) {
					out = append(out, common.RefactorMention{Kind: "saved query", Name: sq.Name, Text: sq.Query, User: user})
				}
			}
		}
		ext.mu.RUnlock()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// TagRename works out a tag rename, merge or delete over the files in scope.
func TagRename(req common.TagRenameRequest) (*refactorPlan, error) {
	old := strings.Trim(strings.TrimSpace(req.Old), ":")
	nw := strings.Trim(strings.TrimSpace(req.New), ":")
	if req.Delete {
		nw = ""
	}
	if !tagNameRe.MatchString(old) {
		return nil, fmt.Errorf("%q is not a tag: letters, digits, _ @ # %% only", req.Old)
	}
	if !req.Delete {
		if nw == "" {
			return nil, fmt.Errorf("say what to rename :%s: to, or ask to delete it", old)
		}
		if !tagNameRe.MatchString(nw) {
			return nil, fmt.Errorf("%q is not a tag: letters, digits, _ @ # %% only", req.New)
		}
		if nw == old {
			return nil, fmt.Errorf(":%s: is already called that", old)
		}
	}
	db := GetDb()
	files := refactorFiles()
	// With a query only the headlines it matched change, by row.
	var rows map[string]map[int]bool
	if q := strings.TrimSpace(req.Query); q != "" {
		todos, err := QueryStringTodos(&common.StringQuery{Query: q})
		if err != nil {
			return nil, fmt.Errorf("the query: %v", err)
		}
		rows = map[string]map[int]bool{}
		if todos != nil {
			for _, t := range *todos {
				sec := db.FindByHash(t.Hash)
				if sec == nil || sec.Headline == nil {
					continue
				}
				f := db.FileFromSection(sec)
				if f == nil {
					continue
				}
				if rows[f.Filename] == nil {
					rows[f.Filename] = map[int]bool{}
				}
				rows[f.Filename][sec.Headline.Pos.Row] = true
			}
		}
		files = []string{}
		for f := range rows {
			files = append(files, f)
		}
		sort.Strings(files)
	}
	if len(req.Files) > 0 {
		want := map[string]bool{}
		for _, f := range req.Files {
			of := db.FindByFile(f)
			if of == nil {
				return nil, fmt.Errorf("no org file %s", f)
			}
			want[of.Filename] = true
		}
		// Narrowing a query's files, or naming them outright.
		keep := []string{}
		if rows != nil {
			for _, f := range files {
				if want[f] {
					keep = append(keep, f)
				}
			}
		} else {
			for f := range want {
				keep = append(keep, f)
			}
			sort.Strings(keep)
		}
		files = keep
	}

	op := "rename"
	if req.Delete {
		op = "delete"
	}
	p := newRefactorPlan("tag " + op)
	heads, fileTags, tagLines, merged := 0, 0, 0, 0
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			p.warn("could not read %s: %v", displayPath(path), err)
			continue
		}
		raw := string(b)
		if !strings.Contains(raw, old) {
			continue
		}
		lines := splitOrgText(raw)
		out := make([]string, 0, len(lines))
		changed := false
		inBlock := false
		for i, l := range lines {
			if headLevel(l) > 0 {
				inBlock = false
				if rows != nil && !rows[path][i] {
					out = append(out, l)
					continue
				}
				tags := headlineTagList(l)
				nt, hit, mg := renameTag(tags, old, nw)
				if hit {
					l = setHeadlineParts(l, "", nil, &nt)
					heads++
					changed = true
					if mg {
						merged++
					}
				}
				out = append(out, l)
				continue
			}
			if blockBeginRe.MatchString(l) {
				inBlock = true
			} else if blockEndRe.MatchString(l) {
				inBlock = false
			}
			if inBlock || rows != nil {
				out = append(out, l)
				continue
			}
			if nl, hit, mg := renameFileTags(l, old, nw); hit {
				fileTags++
				changed = true
				if mg {
					merged++
				}
				// A FILETAGS line with nothing left on it goes.
				if fileTagsLineRe.MatchString(nl) && strings.TrimSpace(fileTagsLineRe.FindStringSubmatch(nl)[3]) == "" {
					continue
				}
				out = append(out, nl)
				continue
			}
			if nl, hit, _ := renameTagsLine(l, old, nw); hit {
				tagLines++
				changed = true
				out = append(out, nl)
				continue
			}
			out = append(out, l)
		}
		if changed {
			p.adopt(path, raw, out)
		}
	}
	nFiles := len(p.order)
	what := fmt.Sprintf("Rename :%s: to :%s:", old, nw)
	if req.Delete {
		what = fmt.Sprintf("Delete :%s:", old)
	}
	parts := []string{refPlural(heads, "heading")}
	if fileTags > 0 {
		parts = append(parts, refPlural(fileTags, "FILETAGS line"))
	}
	if tagLines > 0 {
		parts = append(parts, refPlural(tagLines, "#+TAGS line"))
	}
	p.summary = fmt.Sprintf("%s: %s in %s", what, strings.Join(parts, ", "), refPlural(nFiles, "file"))
	if heads+fileTags+tagLines == 0 {
		p.summary = fmt.Sprintf("Nothing carries :%s:", old)
		if rows != nil {
			p.summary += " among the headings the query matched"
		}
	}
	if merged > 0 {
		p.warn("%s already had :%s:, so :%s: was dropped rather than written twice", refPlural(merged, "place"), nw, old)
	}
	if rows != nil {
		p.warn("limited to the headings the query matched: FILETAGS and #+TAGS lines are left as they are")
	}
	p.mentions = tagMentions(old)
	return p, nil
}

func refPlural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	if strings.HasSuffix(what, "y") {
		return fmt.Sprintf("%d %sies", n, strings.TrimSuffix(what, "y"))
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// RefactorTagCounts is every tag in use, and where.
func RefactorTagCounts() []common.RefactorTag {
	byTag := map[string]*common.RefactorTag{}
	get := func(t string) *common.RefactorTag {
		if byTag[t] == nil {
			byTag[t] = &common.RefactorTag{Tag: t}
		}
		return byTag[t]
	}
	for _, path := range refactorFiles() {
		lines, err := readOrgLines(path)
		if err != nil {
			continue
		}
		inBlock := false
		for _, l := range lines {
			if headLevel(l) > 0 {
				inBlock = false
				for _, t := range headlineTagList(l) {
					get(t).Headings++
				}
				continue
			}
			if blockBeginRe.MatchString(l) {
				inBlock = true
			} else if blockEndRe.MatchString(l) {
				inBlock = false
			}
			if inBlock {
				continue
			}
			if m := fileTagsLineRe.FindStringSubmatch(l); m != nil {
				for _, t := range strings.FieldsFunc(m[3], func(r rune) bool { return r == ':' || r == ' ' || r == '\t' }) {
					get(t).Files++
				}
			} else if m := tagsLineRe.FindStringSubmatch(l); m != nil {
				for _, tok := range strings.Fields(m[2]) {
					if i := strings.Index(tok, "("); i > 0 {
						tok = tok[:i]
					}
					if tagNameRe.MatchString(tok) {
						get(tok).Defined = true
					}
				}
			}
		}
	}
	out := []common.RefactorTag{}
	for _, t := range byTag {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		ci, cj := out[i].Headings+out[i].Files, out[j].Headings+out[j].Files
		if ci != cj {
			return ci > cj
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

// ---------------------------------------------------------------------------
// Split a heading out to a file of its own
// ---------------------------------------------------------------------------

var (
	splitKeepKeywordRe = regexp.MustCompile(`(?i)^\s*#\+(filetags|todo|seq_todo|typ_todo|tags|category):`)
	bracketLinkRe      = regexp.MustCompile(`\[\[([^\[\]]+)\](\[|\])`)
	searchLinkRe       = regexp.MustCompile(`\[\[\*([^\[\]]+)\](\[|\])`)
	validIdRe          = regexp.MustCompile(`^[0-9A-Za-z-]{8,64}$`)
	slugDropRe         = regexp.MustCompile(`[^a-z0-9]+`)
)

// A file name from a title: "Apollo: launch plan" is apollo-launch-plan.
func slugOf(title string) string {
	s := strings.Trim(slugDropRe.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	if s == "" {
		s = "split"
	}
	return s
}

// Where a split writes: To as a path (relative to the first org directory),
// To as a folder, or beside the source when To is empty. Never outside the
// org directories and never a file that is there already.
func splitDest(src, to, title string) (string, error) {
	name := slugOf(title) + ".org"
	var p string
	to = strings.TrimSpace(to)
	switch {
	case to == "":
		p = filepath.Join(filepath.Dir(src), name)
	default:
		cand := to
		if !filepath.IsAbs(cand) && len(Conf().Server.OrgDirs) > 0 {
			cand = filepath.Join(Conf().Server.OrgDirs[0], cand)
		}
		if st, err := os.Stat(cand); strings.HasSuffix(to, "/") || (err == nil && st.IsDir()) {
			p = filepath.Join(cand, name)
		} else {
			p = cand
			if !strings.HasSuffix(strings.ToLower(p), ".org") {
				p += ".org"
			}
		}
	}
	clean, ok := insideOrgDirs(p)
	if !ok {
		return "", fmt.Errorf("%s is not inside an org directory", p)
	}
	if _, err := os.Stat(clean); err == nil {
		return "", fmt.Errorf("%s already exists - a split never writes over a file; give it another name", displayPath(clean))
	}
	return clean, nil
}

// rebaseLinks rewrites the relative file links in text moving from one folder
// to another, so `[[file:../img/x.png]]` still finds the picture.
func rebaseLinks(lines []string, fromDir, toDir string) []string {
	if filepath.Clean(fromDir) == filepath.Clean(toDir) {
		return lines
	}
	out := append([]string(nil), lines...)
	for i, l := range out {
		if !strings.Contains(l, "[[") {
			continue
		}
		out[i] = bracketLinkRe.ReplaceAllStringFunc(l, func(m string) string {
			sm := bracketLinkRe.FindStringSubmatch(m)
			target, tail := sm[1], sm[2]
			prefix := ""
			path := target
			switch {
			case strings.HasPrefix(target, "file:"):
				prefix, path = "file:", target[5:]
			case strings.HasPrefix(target, "./"), strings.HasPrefix(target, "../"):
			default:
				return m
			}
			search := ""
			if i := strings.Index(path, "::"); i >= 0 {
				path, search = path[:i], path[i:]
			}
			if path == "" || filepath.IsAbs(path) || strings.HasPrefix(path, "~") {
				return m
			}
			rel, err := filepath.Rel(toDir, filepath.Join(fromDir, path))
			if err != nil {
				return m
			}
			rel = filepath.ToSlash(rel)
			if !strings.HasPrefix(rel, ".") && (prefix == "" || strings.HasPrefix(path, "./")) {
				rel = "./" + rel
			}
			return "[[" + prefix + rel + search + "]" + tail
		})
	}
	return out
}

// The tags a heading inherits from the headings above it.
func tagsAbove(lines []string, from int) []string {
	lvl := headLevel(lines[from])
	out := []string{}
	for i := from - 1; i >= 0 && lvl > 1; i-- {
		if n := headLevel(lines[i]); n > 0 && n < lvl {
			out = append(headlineTagList(lines[i]), out...)
			lvl = n
		}
	}
	return out
}

// The titles of the headlines in some lines, cleaned for comparing against
// [[*Title]] links.
func headlineTitles(lines []string) map[string]bool {
	out := map[string]bool{}
	for _, l := range lines {
		if headLevel(l) > 0 {
			out[strings.ToLower(cleanMentionTitle(rawHeadlineTitle(l, "")))] = true
			// The keyword is not known here; a title that starts with one is
			// counted both ways rather than missed.
			if f := strings.Fields(rawHeadlineTitle(l, "")); len(f) > 1 {
				out[strings.ToLower(cleanMentionTitle(strings.Join(f[1:], " ")))] = true
			}
		}
	}
	return out
}

// [[*Title]] links in lines that name one of titles (or, with outside, that
// name none of them).
func searchLinksTo(lines []string, titles map[string]bool, outside bool) []string {
	found := []string{}
	for _, l := range lines {
		for _, sm := range searchLinkRe.FindAllStringSubmatch(l, -1) {
			t := strings.ToLower(cleanMentionTitle(sm[1]))
			if titles[t] != outside {
				found = append(found, "*"+sm[1])
			}
		}
	}
	return found
}

// SplitHeading works out moving a heading's subtree into a new file.
//
// The heading keeps its properties and gets an ID if it has none, so it can be
// linked to by id: in the new file and every id: link to it - or anything
// under it - still lands. Its place in the old file becomes one headline at
// the same level whose title is that link. A headline rather than a list item
// because only a headline can stand where a heading stood: a list item there
// would become the last line of the body of whatever heading came before it,
// a sibling's subtree or the parent's.
func SplitHeading(req common.SplitRequest) (*refactorPlan, error) {
	db := GetDb()
	sec := db.FindByHash(req.Hash)
	if sec == nil || sec.Headline == nil {
		return nil, fmt.Errorf("no heading with that hash - it may have moved; refresh and try again")
	}
	f := db.FileFromSection(sec)
	if f == nil {
		return nil, fmt.Errorf("could not find the file that heading is in")
	}
	src := f.Filename
	p := newRefactorPlan("split")
	lines, err := p.get(src)
	if err != nil {
		return nil, err
	}
	from := sec.Headline.Pos.Row
	lvl := sec.Headline.Lvl
	if from < 0 || from >= len(lines) || headLevel(lines[from]) != lvl {
		return nil, fmt.Errorf("that heading is not where the database says it is - the file may be mid-edit; try again")
	}
	to := subtreeEndRow(lines, from, lvl, from)
	end := trimBlankEnd(lines, from, to)
	sub := append([]string(nil), lines[from:end+1]...)

	rawTitle := rawHeadlineTitle(lines[from], sec.Headline.Status)
	plain := cleanMentionTitle(rawTitle)
	if plain == "" {
		plain = "Untitled"
	}
	dest, err := splitDest(src, req.To, plain)
	if err != nil {
		return nil, err
	}

	// The id, given one in the new file when it has none. Made here, in the
	// copy, so the old file never gets a drawer that leaves with the heading.
	id := headingPropsIn(lines, from, to)["ID"]
	if id == "" {
		id = req.NewId
		if !validIdRe.MatchString(id) {
			id = uuid.New().String()
		}
		p.newId = id
	}
	sub = relevel(sub, 1-lvl)
	if p.newId != "" {
		ind := drawerIndentFor(lines, 1)
		var ps, pe int
		sub, ps, pe, _ = ensurePropertyDrawer(sub, 0, len(sub)-1, ind)
		sub = addProps(sub, ps, pe, ind, [][2]string{{"ID", id}})
	}
	sub = rebaseLinks(sub, filepath.Dir(src), filepath.Dir(dest))

	// The new file says what the old one said about its headings: the todo
	// keywords (or they stop being keywords), the file tags and the category
	// (or a tag search and the agenda stop finding them).
	head := []string{"#+TITLE: " + plain}
	for _, l := range lines {
		if headLevel(l) > 0 {
			break
		}
		if splitKeepKeywordRe.MatchString(l) {
			head = append(head, strings.TrimSpace(l))
		}
	}
	p.create(dest, append(append(head, ""), sub...))

	stub := strings.Repeat("*", lvl) + " [[id:" + id + "][" + linkDesc(plain) + "]]"
	out := append([]string(nil), lines[:from]...)
	out = append(out, stub)
	out = append(out, lines[end+1:]...)
	p.put(src, out)

	if inh := tagsAbove(lines, from); len(inh) > 0 {
		p.warn("it inherited :%s: from the headings above it, which stop applying in the new file", strings.Join(inh, ":"))
	}
	titles := headlineTitles(sub)
	if l := searchLinksTo(append(append([]string(nil), lines[:from]...), lines[end+1:]...), titles, false); len(l) > 0 {
		p.warn("%s in %s point into the moved text by title and will not find it: %s",
			refPlural(len(l), "link"), displayPath(src), strings.Join(l, ", "))
	}
	if l := searchLinksTo(sub, titles, true); len(l) > 0 {
		p.warn("%s in the moved text point at headings in %s by title and will look in the new file instead: %s",
			refPlural(len(l), "link"), displayPath(src), strings.Join(l, ", "))
	}
	n := 0
	for _, l := range sub {
		if headLevel(l) > 0 {
			n++
		}
	}
	p.summary = fmt.Sprintf("Split “%s” (%s, %s) out to %s", plain, refPlural(n, "heading"), refPlural(len(sub), "line"), displayPath(dest))
	if p.newId != "" {
		p.summary += "; it gets an ID so the link back finds it"
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Merge one heading into another
// ---------------------------------------------------------------------------

// The parts of a heading, read off its lines.
type headingParts struct {
	planning []string
	props    [][2]string // in the order written
	logbook  []string    // the lines inside the drawer
	logInd   string
	other    []string // any other drawer, kept as written
	body     []string
	children []string
}

func readHeadingParts(lines []string, from, end int) headingParts {
	var hp headingParts
	i := from + 1
	for i <= end {
		l := lines[i]
		if planningRe.MatchString(l) {
			hp.planning = append(hp.planning, l)
			i++
			continue
		}
		m := drawerOpenRe.FindStringSubmatch(l)
		if m == nil || strings.EqualFold(m[1], "END") {
			break
		}
		j := i + 1
		for j <= end && !drawerEndRe.MatchString(lines[j]) {
			j++
		}
		if j > end {
			break
		}
		switch strings.ToUpper(m[1]) {
		case "PROPERTIES":
			for k := i + 1; k < j; k++ {
				if pm := propLineRe.FindStringSubmatch(lines[k]); pm != nil {
					hp.props = append(hp.props, [2]string{pm[2], strings.TrimSpace(pm[4])})
				}
			}
		case "LOGBOOK":
			hp.logInd = l[:len(l)-len(strings.TrimLeft(l, " \t"))]
			hp.logbook = append(hp.logbook, lines[i+1:j]...)
		default:
			hp.other = append(hp.other, lines[i:j+1]...)
		}
		i = j + 1
	}
	child := firstChildRow(lines, from, end)
	body := append([]string(nil), hp.other...)
	body = append(body, lines[i:child]...)
	for len(body) > 0 && blankLine(body[0]) {
		body = body[1:]
	}
	for len(body) > 0 && blankLine(body[len(body)-1]) {
		body = body[:len(body)-1]
	}
	hp.body = body
	if child <= end {
		hp.children = append([]string(nil), lines[child:end+1]...)
	}
	return hp
}

func partProp(props [][2]string, key string) (string, bool) {
	for _, p := range props {
		if strings.EqualFold(p[0], key) {
			return p[1], true
		}
	}
	return "", false
}

// MergeHeadings works out merging From into Into.
//
// From's body goes after Into's body, its children after Into's children
// (re-levelled to sit under Into), its tags are added to Into's, its
// properties are added where Into has none of that name - Into wins every
// conflict, and each one is reported - and its logbook entries join Into's.
// Then From is gone.
//
// Its ID is the part that must not be lost, because links elsewhere name it.
// When Into has no ID it simply takes From's, and nothing anywhere has to
// change. When both have one, every [[id:From]] link in every file is
// re-pointed at Into - shown in the preview like any other line - and From's
// id is kept on Into as MERGED_IDS, so a link from outside the org files
// (a browser bookmark, another tool) can still be traced by searching for it.
// Re-pointing is preferred to an alias because an alias only works for a
// resolver that knows to look for it; Emacs' org-id would not.
func MergeHeadings(req common.MergeRequest) (*refactorPlan, error) {
	db := GetDb()
	secA := db.FindByHash(req.Into)
	secB := db.FindByHash(req.From)
	if secA == nil || secA.Headline == nil || secB == nil || secB.Headline == nil {
		return nil, fmt.Errorf("no heading with that hash - it may have moved; refresh and try again")
	}
	if req.Into == req.From || secA == secB {
		return nil, fmt.Errorf("a heading cannot be merged into itself")
	}
	fa, fb := db.FileFromSection(secA), db.FileFromSection(secB)
	if fa == nil || fb == nil {
		return nil, fmt.Errorf("could not find the files those headings are in")
	}
	pathA, pathB := fa.Filename, fb.Filename
	p := newRefactorPlan("merge")

	lb, err := p.get(pathB)
	if err != nil {
		return nil, err
	}
	bFrom, bLvl := secB.Headline.Pos.Row, secB.Headline.Lvl
	if bFrom < 0 || bFrom >= len(lb) || headLevel(lb[bFrom]) != bLvl {
		return nil, fmt.Errorf("the heading to merge is not where the database says it is - try again")
	}
	bEnd := trimBlankEnd(lb, bFrom, subtreeEndRow(lb, bFrom, bLvl, bFrom))
	aFrom, aLvl := secA.Headline.Pos.Row, secA.Headline.Lvl
	if pathA == pathB && aFrom > bFrom && aFrom <= bEnd {
		return nil, fmt.Errorf("the heading to merge into is inside the one being merged - merge the child into its parent instead")
	}
	bLine := lb[bFrom]
	bTitle := cleanMentionTitle(rawHeadlineTitle(bLine, secB.Headline.Status))
	bTags := headlineTagList(bLine)
	hb := readHeadingParts(lb, bFrom, bEnd)

	// Take From out. A blank line on both sides of the gap it leaves is one
	// too many.
	removed := bEnd - bFrom + 1
	nb := append([]string(nil), lb[:bFrom]...)
	nb = append(nb, lb[bEnd+1:]...)
	if bFrom > 0 && bFrom < len(nb) && blankLine(nb[bFrom]) && blankLine(nb[bFrom-1]) {
		nb = append(nb[:bFrom], nb[bFrom+1:]...)
		removed++
	}
	p.put(pathB, nb)

	la, err := p.get(pathA)
	if err != nil {
		return nil, err
	}
	if pathA == pathB && bFrom < aFrom {
		aFrom -= removed
	}
	if aFrom < 0 || aFrom >= len(la) || headLevel(la[aFrom]) != aLvl {
		return nil, fmt.Errorf("the heading to merge into is not where the database says it is - try again")
	}
	aTitle := cleanMentionTitle(rawHeadlineTitle(la[aFrom], secA.Headline.Status))
	delta := aLvl - bLvl
	end := func() int { return trimBlankEnd(la, aFrom, subtreeEndRow(la, aFrom, aLvl, aFrom)) }

	// Bottom up, so each edit leaves the rows above it where they were.
	aEnd := end()
	if len(hb.children) > 0 {
		add := relevel(hb.children, delta)
		la = splice(la, aEnd+1, add)
	}
	if len(hb.body) > 0 {
		aEnd = end()
		head := headEnd(la, aFrom, aEnd)
		at := firstChildRow(la, aFrom, aEnd)
		for at-1 >= head && at-1 > aFrom && blankLine(la[at-1]) {
			at--
		}
		add := relevel(hb.body, delta)
		if at > head {
			add = append([]string{""}, add...)
		}
		la = splice(la, at, add)
	}

	aProps := headingPropsIn(la, aFrom, end())
	type kv struct{ k, v string }
	set := []kv{}
	conflicts := []string{}
	for _, pr := range hb.props {
		key := strings.ToUpper(pr[0])
		if key == "ID" {
			continue
		}
		if av, ok := aProps[key]; ok {
			if av != pr[1] {
				conflicts = append(conflicts, fmt.Sprintf("%s (kept %q, dropped %q)", pr[0], av, pr[1]))
			}
			continue
		}
		set = append(set, kv{pr[0], pr[1]})
	}
	idA := aProps["ID"]
	idB, _ := partProp(hb.props, "ID")
	repoint := false
	switch {
	case idB == "":
	case idA == "":
		set = append(set, kv{"ID", idB})
		p.warn("“%s” takes the ID of “%s”, so links to either land on it", aTitle, bTitle)
	default:
		merged := strings.TrimSpace(aProps["MERGED_IDS"] + " " + idB)
		set = append(set, kv{"MERGED_IDS", merged})
		repoint = true
	}
	if len(set) > 0 {
		ind := drawerIndentFor(la, aLvl)
		var ps, pe int
		la, ps, pe, _ = ensurePropertyDrawer(la, aFrom, end(), ind)
		kvs := [][2]string{}
		for _, s := range set {
			kvs = append(kvs, [2]string{s.k, s.v})
		}
		la = addProps(la, ps, pe, ind, kvs)
	}
	if len(conflicts) > 0 {
		p.warn("properties both had, where “%s” wins: %s", aTitle, strings.Join(conflicts, "; "))
	}

	if len(hb.logbook) > 0 {
		aEnd = end()
		if s, e, ok := drawerAt(la, aFrom, aEnd, "LOGBOOK"); ok {
			ind := la[s][:len(la[s])-len(strings.TrimLeft(la[s], " \t"))]
			la = splice(la, e, reindentFrom(hb.logbook, hb.logInd, ind))
		} else {
			ind := drawerIndentFor(la, aLvl)
			at := aFrom + 1
			if _, pe, ok := drawerAt(la, aFrom, aEnd, "PROPERTIES"); ok {
				at = pe + 1
			} else {
				for at <= aEnd && planningRe.MatchString(la[at]) {
					at++
				}
			}
			add := []string{ind + ":LOGBOOK:"}
			add = append(add, reindentFrom(hb.logbook, hb.logInd, ind)...)
			add = append(add, ind+":END:")
			la = splice(la, at, add)
		}
	}

	if len(hb.planning) > 0 {
		if aFrom+1 < len(la) && planningRe.MatchString(la[aFrom+1]) {
			p.warn("“%s” has its own dates, so these from “%s” were dropped: %s", aTitle, bTitle,
				strings.TrimSpace(strings.Join(hb.planning, " ")))
		} else {
			// Planning lines sit where the file puts its drawers.
			ind := drawerIndentFor(la, aLvl)
			add := []string{}
			for _, l := range hb.planning {
				add = append(add, ind+strings.TrimSpace(l))
			}
			la = splice(la, aFrom+1, add)
		}
	}

	aTags := headlineTagList(la[aFrom])
	union := append([]string(nil), aTags...)
	addedTags := []string{}
	for _, t := range bTags {
		have := false
		for _, u := range union {
			if u == t {
				have = true
			}
		}
		if !have {
			union = append(union, t)
			addedTags = append(addedTags, t)
		}
	}
	if len(addedTags) > 0 {
		la[aFrom] = setHeadlineParts(la[aFrom], "", nil, &union)
	}
	if bs := secB.Headline.Status; bs != "" && bs != secA.Headline.Status {
		p.warn("“%s” was %s; “%s” keeps its own keyword", bTitle, bs, aTitle)
	}
	p.put(pathA, la)

	links := 0
	if repoint {
		re := regexp.MustCompile(`\[\[id:` + regexp.QuoteMeta(idB) + `(\]|::)`)
		for _, path := range refactorFiles() {
			var lines []string
			var raw string
			if e, ok := p.files[path]; ok {
				lines = append([]string(nil), e.lines...)
			} else {
				b, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(b), idB) {
					continue
				}
				raw = string(b)
				lines = splitOrgText(raw)
			}
			hit := false
			for i, l := range lines {
				if !strings.Contains(l, idB) {
					continue
				}
				n := len(re.FindAllStringIndex(l, -1))
				if n > 0 {
					lines[i] = re.ReplaceAllString(l, "[[id:"+idA+"$1")
					links += n
					hit = true
				}
			}
			if hit {
				if _, ok := p.files[path]; ok {
					p.put(path, lines)
				} else {
					p.adopt(path, raw, lines)
				}
			}
		}
	}
	if l := searchLinksTo(nb, map[string]bool{strings.ToLower(bTitle): true}, false); len(l) > 0 {
		p.warn("%s in %s name “%s” by title and will not find it", refPlural(len(l), "link"), displayPath(pathB), bTitle)
	}

	bits := []string{}
	if len(hb.body) > 0 {
		bits = append(bits, refPlural(len(hb.body), "line")+" of text")
	}
	if n := countHeadlines(hb.children); n > 0 {
		bits = append(bits, refPlural(n, "child heading"))
	}
	if len(addedTags) > 0 {
		bits = append(bits, "tags :"+strings.Join(addedTags, ":")+":")
	}
	if len(set) > 0 {
		bits = append(bits, refPlural(len(set), "property"))
	}
	if links > 0 {
		bits = append(bits, refPlural(links, "link")+" re-pointed")
	}
	p.summary = fmt.Sprintf("Merge “%s” into “%s”", bTitle, aTitle)
	if len(bits) > 0 {
		p.summary += ": " + strings.Join(bits, ", ")
	}
	return p, nil
}

// addProps writes properties into a drawer known to exist - a key already
// there is set, the rest are added at the end - without re-padding the lines
// already in it. setPropIn realigns the whole drawer, which is right for a
// form that owns it and wrong here: a refactor's diff should show the
// properties it added, not every line of somebody's drawer moved about.
// New lines line up with the drawer's own value column, or org's default
// (a ten wide key) in a drawer with nothing in it yet.
func addProps(lines []string, ps, pe int, ind string, kvs [][2]string) []string {
	if ps >= 0 && ps < len(lines) {
		ind = lines[ps][:len(lines[ps])-len(strings.TrimLeft(lines[ps], " \t"))]
	}
	width := 0
	for i := ps + 1; i < pe && i < len(lines); i++ {
		if m := propLineRe.FindStringSubmatch(lines[i]); m != nil && m[4] != "" {
			if w := len(m[2]) + 2 + len(m[3]); w > width {
				width = w
			}
		}
	}
	if width == 0 {
		width = 11
	}
	for _, kv := range kvs {
		set := false
		for i := ps + 1; i < pe && i < len(lines); i++ {
			if m := propLineRe.FindStringSubmatch(lines[i]); m != nil && strings.EqualFold(m[2], kv[0]) {
				gap := m[3]
				if gap == "" {
					gap = " "
				}
				lines[i] = m[1] + ":" + m[2] + ":" + gap + kv[1]
				set = true
				break
			}
		}
		if set {
			continue
		}
		k := ":" + kv[0] + ":"
		pad := " "
		if width > len(k) {
			pad = strings.Repeat(" ", width-len(k))
		}
		lines = splice(lines, pe, []string{ind + k + pad + kv[1]})
		pe++
	}
	return lines
}

func countHeadlines(lines []string) int {
	n := 0
	for _, l := range lines {
		if headLevel(l) > 0 {
			n++
		}
	}
	return n
}

// Lines moved from one drawer indent to another.
func reindentFrom(lines []string, from, to string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if strings.HasPrefix(l, from) {
			out[i] = to + l[len(from):]
		} else {
			out[i] = to + strings.TrimLeft(l, " \t")
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// A list into headings
// ---------------------------------------------------------------------------

var (
	orgListItemRe = regexp.MustCompile(`^([ \t]*)([-+*]|\d+[.)])([ \t]+|$)(.*)$`)
	checkboxRe    = regexp.MustCompile(`^\[([ xX-])\][ \t]*`)
	counterRe     = regexp.MustCompile(`^\[@\d+\][ \t]*`)
	descItemRe    = regexp.MustCompile(`^(.*?)[ \t]+::(?:[ \t]+(.*))?$`)
)

type listSpan struct {
	start, end, indent int
}

// An item line, or nil. A `*` bullet in column zero is a headline, not this.
func listItem(l string) []string {
	m := orgListItemRe.FindStringSubmatch(l)
	if m == nil || (m[2] == "*" && m[1] == "") {
		return nil
	}
	return m
}

// The lists written in rows [from, to) - a heading's own text - outside blocks
// and drawers. A list runs while its lines are items at its indent or deeper,
// or text indented past its bullets; two blank lines end it, as in org.
func findLists(lines []string, from, to int) []listSpan {
	spans := []listSpan{}
	inBlock, inDrawer := false, false
	for i := from; i < to; i++ {
		l := lines[i]
		switch {
		case inBlock:
			if blockEndRe.MatchString(l) {
				inBlock = false
			}
			continue
		case inDrawer:
			if drawerEndRe.MatchString(l) {
				inDrawer = false
			}
			continue
		case blockBeginRe.MatchString(l):
			inBlock = true
			continue
		case drawerOpenRe.MatchString(l) && !drawerEndRe.MatchString(l):
			inDrawer = true
			continue
		}
		m := listItem(l)
		if m == nil {
			continue
		}
		base := len(m[1])
		last := i
		blanks := 0
		j := i + 1
		for ; j < to; j++ {
			lj := lines[j]
			if blankLine(lj) {
				blanks++
				if blanks >= 2 {
					break
				}
				continue
			}
			ind := leadingSpaces(lj)
			if ind < base || (ind == base && listItem(lj) == nil) {
				break
			}
			blanks = 0
			last = j
		}
		spans = append(spans, listSpan{i, last, base})
		i = last
	}
	return spans
}

// The heading's own text: from below its drawers to its first child.
func headingTextRows(lines []string, from, lvl int) (int, int, int) {
	to := subtreeEndRow(lines, from, lvl, from)
	end := trimBlankEnd(lines, from, to)
	return headEnd(lines, from, end), firstChildRow(lines, from, end), end
}

// The keywords a file's headings take: its own #+TODO line's first open and
// first done, else the server's.
func fileTodoPair(lines []string) (string, string) {
	pick := func(spec string) (string, string) {
		parts := strings.SplitN(spec, "|", 2)
		clean := func(w string) string {
			if i := strings.Index(w, "("); i > 0 {
				return w[:i]
			}
			return w
		}
		open := strings.Fields(parts[0])
		if len(parts) < 2 {
			if len(open) < 2 {
				return "", ""
			}
			return clean(open[0]), clean(open[len(open)-1])
		}
		done := strings.Fields(parts[1])
		if len(open) == 0 || len(done) == 0 {
			return "", ""
		}
		return clean(open[0]), clean(done[0])
	}
	for _, l := range lines {
		if headLevel(l) > 0 {
			break
		}
		if m := mentionTodoLineRe.FindStringSubmatch(l); m != nil {
			if a, d := pick(m[1]); a != "" {
				return a, d
			}
		}
	}
	if a, d := pick(Conf().Server.DefaultTodoStates); a != "" {
		return a, d
	}
	return "TODO", "DONE"
}

type parsedItem struct {
	depth int
	check string
	title string
	body  []string
}

// parseList reads a list's items, each with how deep it is and its own text.
func parseList(lines []string, sp listSpan) []parsedItem {
	items := []parsedItem{}
	type open struct{ indent, idx int }
	stack := []open{}
	for r := sp.start; r <= sp.end; r++ {
		l := lines[r]
		if m := listItem(l); m != nil {
			ind := len(m[1])
			for len(stack) > 0 && stack[len(stack)-1].indent >= ind {
				stack = stack[:len(stack)-1]
			}
			text := m[4]
			text = counterRe.ReplaceAllString(text, "")
			it := parsedItem{depth: len(stack)}
			if cm := checkboxRe.FindStringSubmatch(text); cm != nil {
				it.check = cm[1]
				text = text[len(cm[0]):]
			}
			if dm := descItemRe.FindStringSubmatch(text); dm != nil && strings.TrimSpace(dm[1]) != "" {
				text = dm[1]
				if strings.TrimSpace(dm[2]) != "" {
					it.body = append(it.body, strings.Repeat(" ", len(m[1])+len(m[2])+len(m[3]))+strings.TrimSpace(dm[2]))
				}
			}
			it.title = strings.TrimSpace(text)
			items = append(items, it)
			stack = append(stack, open{ind, len(items) - 1})
			continue
		}
		if len(items) == 0 {
			continue
		}
		owner := len(items) - 1
		if !blankLine(l) {
			ind := leadingSpaces(l)
			for len(stack) > 1 && stack[len(stack)-1].indent >= ind {
				stack = stack[:len(stack)-1]
			}
			if len(stack) > 0 {
				owner = stack[len(stack)-1].idx
			}
		}
		items[owner].body = append(items[owner].body, l)
	}
	return items
}

// RefactorListsUnder is the lists in a heading's own text.
func RefactorListsUnder(hash string) ([]common.RefactorList, error) {
	db := GetDb()
	sec := db.FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		return nil, fmt.Errorf("no heading with that hash - it may have moved; refresh and try again")
	}
	f := db.FileFromSection(sec)
	if f == nil {
		return nil, fmt.Errorf("could not find the file that heading is in")
	}
	lines, err := readOrgLines(f.Filename)
	if err != nil {
		return nil, err
	}
	from := sec.Headline.Pos.Row
	if from < 0 || from >= len(lines) {
		return nil, fmt.Errorf("that heading is not where the database says it is")
	}
	textFrom, child, _ := headingTextRows(lines, from, sec.Headline.Lvl)
	out := []common.RefactorList{}
	for _, sp := range findLists(lines, textFrom, child) {
		items := parseList(lines, sp)
		rl := common.RefactorList{Line: sp.start, End: sp.end, Items: len(items)}
		if len(items) > 0 {
			rl.First = items[0].title
		}
		for _, it := range items {
			if it.check != "" {
				rl.Checkboxes++
			}
		}
		out = append(out, rl)
	}
	return out, nil
}

// ListToHeadings works out turning one list under a heading into child
// headings: an item is a heading one level down (a nested item further down),
// [ ] is the file's first open keyword and [X] its first done one, a
// description item's term is the title and its description the text, and an
// item's own following lines become its heading's text.
//
// The headings go at the end of the heading's own text, before its existing
// children - headings cannot sit in the middle of a body, so text written
// after the list stays above them.
func ListToHeadings(req common.ListToHeadingsRequest) (*refactorPlan, error) {
	db := GetDb()
	sec := db.FindByHash(req.Hash)
	if sec == nil || sec.Headline == nil {
		return nil, fmt.Errorf("no heading with that hash - it may have moved; refresh and try again")
	}
	f := db.FileFromSection(sec)
	if f == nil {
		return nil, fmt.Errorf("could not find the file that heading is in")
	}
	path := f.Filename
	p := newRefactorPlan("list to headings")
	lines, err := p.get(path)
	if err != nil {
		return nil, err
	}
	from, lvl := sec.Headline.Pos.Row, sec.Headline.Lvl
	if from < 0 || from >= len(lines) || headLevel(lines[from]) != lvl {
		return nil, fmt.Errorf("that heading is not where the database says it is - try again")
	}
	textFrom, child, _ := headingTextRows(lines, from, lvl)
	spans := findLists(lines, textFrom, child)
	if len(spans) == 0 {
		return nil, fmt.Errorf("there is no list in the text of that heading (lists under its children are theirs)")
	}
	sp := spans[0]
	if req.Line > 0 {
		found := false
		for _, s := range spans {
			if req.Line >= s.start && req.Line <= s.end {
				sp, found = s, true
			}
		}
		if !found {
			return nil, fmt.Errorf("line %d is not in a list in that heading's text", req.Line+1)
		}
	}
	items := parseList(lines, sp)
	open, done := fileTodoPair(lines)
	indented := sp.indent > 0

	add := []string{}
	for _, it := range items {
		hl := lvl + 1 + it.depth
		line := strings.Repeat("*", hl) + " "
		switch it.check {
		case " ", "-":
			line += open + " "
		case "x", "X":
			line += done + " "
		}
		add = append(add, strings.TrimRight(line+it.title, " "))
		body := it.body
		for len(body) > 0 && blankLine(body[0]) {
			body = body[1:]
		}
		for len(body) > 0 && blankLine(body[len(body)-1]) {
			body = body[:len(body)-1]
		}
		cut := -1
		for _, b := range body {
			if blankLine(b) {
				continue
			}
			if n := leadingSpaces(b); cut < 0 || n < cut {
				cut = n
			}
		}
		ind := ""
		if indented {
			ind = indentOf(hl)
		}
		for _, b := range body {
			if blankLine(b) {
				add = append(add, "")
				continue
			}
			add = append(add, ind+b[cut:])
		}
	}

	// Out with the list; a blank line on both sides of the hole is one too many.
	n := sp.end - sp.start + 1
	out := append([]string(nil), lines[:sp.start]...)
	rest := lines[sp.end+1:]
	if sp.start > 0 && sp.end+1 < child && len(rest) > 0 && blankLine(rest[0]) && blankLine(lines[sp.start-1]) {
		rest = rest[1:]
		n++
	}
	out = append(out, rest...)
	at := child - n
	// Text that followed the list, now between the hole and the new headings.
	textLeft := false
	for i := sp.start; i < at && i < len(out); i++ {
		if !blankLine(out[i]) {
			textLeft = true
		}
	}
	for at-1 > from && at-1 >= textFrom && blankLine(out[at-1]) {
		at--
	}
	out = splice(out, at, add)
	p.put(path, out)

	todo, closed := 0, 0
	for _, it := range items {
		switch it.check {
		case " ", "-":
			todo++
		case "x", "X":
			closed++
		}
	}
	title := cleanMentionTitle(rawHeadlineTitle(lines[from], sec.Headline.Status))
	p.summary = fmt.Sprintf("Turn the %s under “%s” into headings", refPlural(len(items), "item"), title)
	if todo+closed > 0 {
		p.summary += fmt.Sprintf(" (%d %s, %d %s)", todo, open, closed, done)
	}
	if textLeft {
		p.warn("the text after the list stays in “%s”, above the new headings", title)
	}
	if len(spans) > 1 {
		p.warn("only this list of the %d under the heading changes", len(spans))
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func refactorAnswer(w http.ResponseWriter, res common.RefactorPlan) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// servePlan reads a request, works out its plan and answers with the preview,
// or with the apply when apply is set.
func servePlan[T any](w http.ResponseWriter, r *http.Request, apply bool, op string,
	run func(T) (*refactorPlan, error), args func(T) common.RefactorApply) {
	var req T
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		answerErr(w, err)
		return
	}
	if apply {
		refactorMu.Lock()
		defer refactorMu.Unlock()
	}
	p, err := run(req)
	if err != nil {
		refactorAnswer(w, common.RefactorPlan{Ok: false, Op: op, Msg: err.Error(), Files: []common.RefactorFile{}})
		return
	}
	if apply {
		refactorAnswer(w, p.apply(args(req)))
		return
	}
	refactorAnswer(w, p.result())
}

/* SDOC: API
* GET /refactor/tags — Every Tag In Use, For Renaming

	Every tag written anywhere: on headlines, in =#+FILETAGS:= lines, and
	defined on =#+TAGS:= lines, with how many headlines and files carry it.
	Read off the files' own lines, most used first.

	*Method:* =GET=

	*Response:* ={Ok, Msg, Tags: [{Tag, Headings, Files, Defined}]}=.
EDOC */
func RequestRefactorTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.RefactorTags{Ok: true, Tags: RefactorTagCounts()})
}

/* SDOC: API
* GET /refactor/lists — The Lists In A Heading's Text

	The plain and checkbox lists written in one heading's own text (not its
	children's), for choosing which one =/refactor/list= should turn into
	headings.

	*Method:* =GET=

	*Query Parameters:* =hash=, the heading.

	*Response:* ={Ok, Msg, Lists: [{Line, End, Items, First, Checkboxes}]}=,
	=Line= and =End= zero-based rows of the file.
EDOC */
func RequestRefactorLists(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	lists, err := RefactorListsUnder(r.URL.Query().Get("hash"))
	if err != nil {
		json.NewEncoder(w).Encode(common.RefactorLists{Ok: false, Msg: err.Error(), Lists: []common.RefactorList{}})
		return
	}
	json.NewEncoder(w).Encode(common.RefactorLists{Ok: true, Lists: lists})
}

/* SDOC: API
* POST /refactor/tag/preview — Preview Renaming, Merging Or Deleting A Tag

	Works out renaming one tag to another across every file - headline tags
	(right-aligned tags stay ending at the column they ended at),
	=#+FILETAGS:= and =#+TAGS:= lines - and answers with the change, writing
	nothing. A new name some heading already carries is a merge: the old tag
	is dropped there rather than written twice. =Delete= takes the tag off
	everywhere instead.

	Saved queries, filters and tag groups that name the old tag are listed in
	=Mentions= and never changed.

	All four refactors answer the same way; this is the one place it is said.
	Each has a =preview= that writes nothing and an =apply= that does it. The
	preview's answer is a =RefactorPlan=: =Ok=, =Msg=, =Summary=, =Warnings=,
	=Files= (per file: =Filename=, =Created=, =Before= and =After= text,
	=Hash= of =Before=, a unified =Patch=, =Added=/=Removed= counts), and a
	=Token=. The apply takes the same request with =Token= (and optionally
	=Expect=, a map of filename to =Hash=) from the preview, works the change
	out again from the files as they are now, and refuses with =Stale= set
	unless it comes to exactly what the preview showed.

	*Method:* =POST=

	*Request Body:* ={Old, New, Delete, Files, Query}=. =Files= limits it to
	some files; =Query= to the headlines of the headings a query matches
	(FILETAGS and #+TAGS lines are then left alone).

	*Response:* A =RefactorPlan=.
EDOC */
func PostTagRenamePreview(w http.ResponseWriter, r *http.Request) {
	servePlan(w, r, false, "tag", TagRename, func(q common.TagRenameRequest) common.RefactorApply { return q.RefactorApply })
}

/* SDOC: API
* POST /refactor/tag/apply — Rename, Merge Or Delete A Tag

	Writes what =/refactor/tag/preview= showed, if the files still come to
	the same change. Send the preview's request again with its =Token=.

	*Method:* =POST=

	*Request Body:* ={Old, New, Delete, Files, Query, Token, Expect}=.

	*Response:* A =RefactorPlan= with =Applied= set, or =Ok= false and =Stale=
	when a file changed since the preview.
EDOC */
func PostTagRenameApply(w http.ResponseWriter, r *http.Request) {
	servePlan(w, r, true, "tag", TagRename, func(q common.TagRenameRequest) common.RefactorApply { return q.RefactorApply })
}

/* SDOC: API
* POST /refactor/split/preview — Preview Splitting A Heading Out To A File

	Works out moving a heading and everything under it into a new org file:
	levels promoted so it starts at level one, its properties kept, given an
	=ID= if it had none (answered as =NewId=), and the old file's
	=#+FILETAGS:=, =#+TODO:=, =#+TAGS:= and =#+CATEGORY:= lines copied so it
	means the same there. Relative file links are rebased when the new file
	is in another folder. In the old file the heading becomes one headline at
	the same level whose title is =[[id:…][Title]]=.

	The new file is only ever inside an org directory, and never one that
	exists.

	*Method:* =POST=

	*Request Body:* ={Hash, To}=. =To= is the new file's path (relative to the
	first org directory) or a folder; empty is a file named for the heading
	beside the one it is in.

	*Response:* A =RefactorPlan= (see =/refactor/tag/preview=) with =NewFile=,
	and =NewId= when an id was made.
EDOC */
func PostSplitPreview(w http.ResponseWriter, r *http.Request) {
	servePlan(w, r, false, "split", SplitHeading, func(q common.SplitRequest) common.RefactorApply { return q.RefactorApply })
}

/* SDOC: API
* POST /refactor/split/apply — Split A Heading Out To A File

	Writes what =/refactor/split/preview= showed. Send the preview's request
	with its =Token= and, when the preview made an id, =NewId= set to it.
	The new file is made first and exclusively, so one that appeared since
	the preview is refused before the old file is touched.

	*Method:* =POST=

	*Request Body:* ={Hash, To, NewId, Token, Expect}=.

	*Response:* A =RefactorPlan= with =Applied= set.
EDOC */
func PostSplitApply(w http.ResponseWriter, r *http.Request) {
	servePlan(w, r, true, "split", SplitHeading, func(q common.SplitRequest) common.RefactorApply { return q.RefactorApply })
}

/* SDOC: API
* POST /refactor/merge/preview — Preview Merging One Heading Into Another

	Works out merging =From= into =Into=: From's text after Into's text, its
	children after Into's children (re-levelled), tags unioned, properties
	added where Into has none of that name (Into wins every conflict, each
	reported in =Warnings=), logbook entries joined and From removed. When
	Into has no =ID= it takes From's; when both do, every =[[id:From]]= link
	is re-pointed at Into and From's id kept on Into as =MERGED_IDS=.

	*Method:* =POST=

	*Request Body:* ={Into, From}=, both heading hashes.

	*Response:* A =RefactorPlan= (see =/refactor/tag/preview=).
EDOC */
func PostMergePreview(w http.ResponseWriter, r *http.Request) {
	servePlan(w, r, false, "merge", MergeHeadings, func(q common.MergeRequest) common.RefactorApply { return q.RefactorApply })
}

/* SDOC: API
* POST /refactor/merge/apply — Merge One Heading Into Another

	Writes what =/refactor/merge/preview= showed, if the files still come to
	the same change.

	*Method:* =POST=

	*Request Body:* ={Into, From, Token, Expect}=.

	*Response:* A =RefactorPlan= with =Applied= set.
EDOC */
func PostMergeApply(w http.ResponseWriter, r *http.Request) {
	servePlan(w, r, true, "merge", MergeHeadings, func(q common.MergeRequest) common.RefactorApply { return q.RefactorApply })
}

/* SDOC: API
* POST /refactor/list/preview — Preview Turning A List Into Headings

	Works out turning a list in a heading's text into child headings: each
	item a heading one level down (nested items deeper), =[ ]= the file's
	first open keyword and =[X]= its first done one, a description item's
	term the title, and the lines under an item its heading's text. The
	headings go at the end of the heading's own text, before its children.

	*Method:* =POST=

	*Request Body:* ={Hash, Line}=. =Line= is a zero-based row inside the list
	to convert (see =/refactor/lists=); 0 or less is the first list.

	*Response:* A =RefactorPlan= (see =/refactor/tag/preview=).
EDOC */
func PostListToHeadingsPreview(w http.ResponseWriter, r *http.Request) {
	servePlan(w, r, false, "list", ListToHeadings, func(q common.ListToHeadingsRequest) common.RefactorApply { return q.RefactorApply })
}

/* SDOC: API
* POST /refactor/list/apply — Turn A List Into Headings

	Writes what =/refactor/list/preview= showed, if the file still comes to
	the same change.

	*Method:* =POST=

	*Request Body:* ={Hash, Line, Token, Expect}=.

	*Response:* A =RefactorPlan= with =Applied= set.
EDOC */
func PostListToHeadingsApply(w http.ResponseWriter, r *http.Request) {
	servePlan(w, r, true, "list", ListToHeadings, func(q common.ListToHeadingsRequest) common.RefactorApply { return q.RefactorApply })
}
