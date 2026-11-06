package common

// Latex Suite (features.latexSuite): quick maths typing in the
// manner of Emacs's cdlatex, for org maths. The snippet table format, the built-in set, the
// snippet engine and the maths scanner live here so the server (the merged
// set, /latex/check) and the command line (`orgs latex expand`, which works
// with no server from the built-in set) read them the same way.
//
// The engine and the scanner are twins of worg/src/latexsuite.ts (copied
// verbatim to vscode-org-everywhere/src/latex-suite-model.ts): the same cases
// are in latexsuite_test.go and latexsuite.test.ts. Offsets here are bytes
// (the TS copies count UTF-16 units); the cases are ASCII.

import (
	_ "embed"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

//go:embed latexsnippets.org
var LatexBuiltinOrg string

// LatexSnippet is one row of a snippets table.
type LatexSnippet struct {
	Trigger     string
	Replacement string
	Options     string
	Description string `json:",omitempty"`
	Priority    int    `json:",omitempty"`
	Group       string `json:",omitempty"` // the heading above its table
	File        string `json:",omitempty"` // "" for the built-in set
	Line        int    `json:",omitempty"` // 1-based, in File
}

// LatexTableProblem is a row that could not be read.
type LatexTableProblem struct {
	File string `json:",omitempty"`
	Line int
	Msg  string
}

var latexHeadRe = regexp.MustCompile(`^\*+\s+(.*?)\s*$`)

// LatexDecodeCell is a table cell as the engine reads it: =x= or ~x~ around
// the whole cell are dropped, \vert (or \vert{}) is a |, \n not before a
// letter is a new line, and \\ is left alone.
func LatexDecodeCell(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 3 && (s[0] == '=' || s[0] == '~') && s[len(s)-1] == s[0] {
		// Org's verbatim, but not a trigger made of the marks (=== or ~~).
		if in := s[1 : len(s)-1]; strings.Trim(in, s[:1]) != "" && in == strings.TrimSpace(in) {
			s = in
		}
	}
	letter := func(i int) bool {
		return i < len(s) && (s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z')
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) {
			switch {
			case s[i+1] == '\\':
				b.WriteString(`\\`)
				i += 2
				continue
			case strings.HasPrefix(s[i:], `\vert{}`):
				b.WriteByte('|')
				i += 7
				continue
			case strings.HasPrefix(s[i:], `\vert`) && !letter(i+5):
				b.WriteByte('|')
				i += 5
				continue
			case s[i+1] == 'n' && !letter(i+2):
				b.WriteByte('\n')
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// ParseLatexSnippets reads every table in an org file whose header row has a
// Trigger and a Replacement column (Options, Description and Priority are
// optional). Rows with a regex trigger that does not compile are reported.
func ParseLatexSnippets(text, file string) ([]LatexSnippet, []LatexTableProblem) {
	out := []LatexSnippet{}
	probs := []LatexTableProblem{}
	group := ""
	var cols map[string]int // nil: no table yet; empty: a table that is not snippets
	for i, raw := range strings.Split(text, "\n") {
		t := strings.TrimSpace(raw)
		if m := latexHeadRe.FindStringSubmatch(raw); m != nil {
			group, cols = m[1], nil
			continue
		}
		if !strings.HasPrefix(t, "|") {
			cols = nil
			continue
		}
		if strings.HasPrefix(t, "|-") {
			continue
		}
		cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(t, "|"), "|"), "|")
		if cols == nil {
			cols = map[string]int{}
			for j, c := range cells {
				cols[strings.ToLower(strings.TrimSpace(c))] = j
			}
			_, a := cols["trigger"]
			_, b := cols["replacement"]
			if !a || !b {
				cols = map[string]int{}
			}
			continue
		}
		if len(cols) == 0 {
			continue
		}
		cell := func(name string) string {
			if j, ok := cols[name]; ok && j < len(cells) {
				return cells[j]
			}
			return ""
		}
		s := LatexSnippet{
			Trigger:     LatexDecodeCell(cell("trigger")),
			Replacement: LatexDecodeCell(cell("replacement")),
			Options:     strings.TrimSpace(cell("options")),
			Description: LatexDecodeCell(cell("description")),
			Group:       group,
			File:        file,
			Line:        i + 1,
		}
		if p := strings.TrimSpace(cell("priority")); p != "" {
			if n, err := strconv.Atoi(p); err == nil {
				s.Priority = n
			} else {
				probs = append(probs, LatexTableProblem{file, i + 1, "priority " + p + " is not a number"})
			}
		}
		if s.Trigger == "" {
			continue
		}
		if strings.Contains(s.Options, "r") {
			if _, err := regexp.Compile("(?:" + s.Trigger + ")$"); err != nil {
				probs = append(probs, LatexTableProblem{file, i + 1, "trigger " + s.Trigger + ": " + err.Error()})
				continue
			}
		}
		out = append(out, s)
	}
	return out, probs
}

var latexBuiltinOnce sync.Once
var latexBuiltin []LatexSnippet

// LatexBuiltinSnippets is the built-in set (latexsnippets.org).
func LatexBuiltinSnippets() []LatexSnippet {
	latexBuiltinOnce.Do(func() { latexBuiltin, _ = ParseLatexSnippets(LatexBuiltinOrg, "") })
	return append([]LatexSnippet(nil), latexBuiltin...)
}

// latexKey is what makes two rows the same snippet: the trigger, the modes it
// expands in and whether it wraps a selection.
func latexKey(s LatexSnippet) string {
	k := s.Trigger + "\x00"
	for _, c := range "tmnMv" {
		if strings.ContainsRune(s.Options, c) {
			k += string(c)
		}
	}
	return k
}

// MergeLatexSnippets is the user's rows, then every built-in row the user did
// not replace. A user row with an empty replacement takes the built-in one
// away and is not itself a snippet.
func MergeLatexSnippets(user, builtin []LatexSnippet) []LatexSnippet {
	seen := map[string]bool{}
	out := []LatexSnippet{}
	for _, s := range user {
		seen[latexKey(s)] = true
		if s.Replacement != "" {
			out = append(out, s)
		}
	}
	for _, s := range builtin {
		if !seen[latexKey(s)] {
			out = append(out, s)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The engine
// ---------------------------------------------------------------------------

// LatexExpansion is what a trigger becomes. Remove is how many bytes before
// the cursor the trigger took; Body is the replacement in snippet syntax
// ($1, ${1:x}, $0), ready for LatexPlace or an editor's own snippets.
type LatexExpansion struct {
	Snippet LatexSnippet
	Remove  int
	Body    string
}

// LatexModeOK reports whether a snippet's options let it expand in a mode
// ("text", "inline" or "display").
func LatexModeOK(opts, mode string) bool {
	t, m, n, d := strings.Contains(opts, "t"), strings.Contains(opts, "m"), strings.Contains(opts, "n"), strings.Contains(opts, "M")
	if !t && !m && !n && !d {
		return true
	}
	switch mode {
	case "text":
		return t
	case "inline":
		return m || n
	case "display":
		return m || d
	}
	return false
}

// latexOrder is the order snippets are tried in: priority, then the longer
// trigger, then as listed.
func latexOrder(snips []LatexSnippet) []LatexSnippet {
	o := append([]LatexSnippet(nil), snips...)
	sort.SliceStable(o, func(i, j int) bool {
		if o[i].Priority != o[j].Priority {
			return o[i].Priority > o[j].Priority
		}
		return len(o[i].Trigger) > len(o[j].Trigger)
	})
	return o
}

var latexReCache sync.Map

func latexRe(trigger string) *regexp.Regexp {
	if r, ok := latexReCache.Load(trigger); ok {
		return r.(*regexp.Regexp)
	}
	r, err := regexp.Compile("(?:" + trigger + ")$")
	if err != nil {
		r = nil
	}
	latexReCache.Store(trigger, r)
	return r
}

var latexMacroTailRe = regexp.MustCompile(`\\[A-Za-z]+$`)
var latexCaptureRe = regexp.MustCompile(`\[\[(\d+)\]\]`)

func isLatexLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// LatexExpand finds the snippet that ends at the cursor. before is the text
// before the cursor (the line, or the maths so far, with what was just typed);
// auto picks the A snippets (as you type) or the others (TAB).
func LatexExpand(snips []LatexSnippet, before, mode string, auto bool) *LatexExpansion {
	for _, s := range latexOrder(snips) {
		if strings.Contains(s.Options, "v") || strings.Contains(s.Options, "A") != auto || !LatexModeOK(s.Options, mode) {
			continue
		}
		start := -1
		var groups []string
		if strings.Contains(s.Options, "r") {
			re := latexRe(s.Trigger)
			if re == nil {
				continue
			}
			m := re.FindStringSubmatchIndex(before)
			if m == nil || m[1] == m[0] {
				continue
			}
			start = m[0]
			for g := 2; g+1 < len(m); g += 2 {
				if m[g] < 0 {
					groups = append(groups, "")
				} else {
					groups = append(groups, before[m[g]:m[g+1]])
				}
			}
		} else {
			cands := []string{s.Trigger}
			if strings.HasPrefix(s.Trigger, "@") && len(s.Trigger) > 1 {
				cands = append(cands, "`"+s.Trigger[1:])
			}
			for _, c := range cands {
				if strings.HasSuffix(before, c) {
					start = len(before) - len(c)
					break
				}
			}
		}
		if start < 0 {
			continue
		}
		if strings.Contains(s.Options, "w") && start > 0 {
			if p := before[start-1]; isLatexLetter(p) || p == '\\' {
				continue
			}
		}
		// Typing a command's name (\sqrt) expands nothing inside it.
		if mode != "text" && !strings.HasPrefix(s.Trigger, `\`) {
			if loc := latexMacroTailRe.FindStringIndex(before); loc != nil && loc[0] < start {
				continue
			}
		}
		body := latexCaptureRe.ReplaceAllStringFunc(s.Replacement, func(x string) string {
			n, _ := strconv.Atoi(x[2 : len(x)-2])
			if n < len(groups) {
				return groups[n]
			}
			return ""
		})
		return &LatexExpansion{Snippet: s, Remove: len(before) - start, Body: body}
	}
	return nil
}

// LatexVisual is the snippet a key wraps the selection in, or nil.
func LatexVisual(snips []LatexSnippet, key, mode, selection string) *LatexExpansion {
	if selection == "" {
		return nil
	}
	for _, s := range latexOrder(snips) {
		if !strings.Contains(s.Options, "v") || s.Trigger != key || !LatexModeOK(s.Options, mode) {
			continue
		}
		sel := strings.ReplaceAll(selection, "$", `\$`)
		return &LatexExpansion{Snippet: s, Body: strings.ReplaceAll(s.Replacement, "${VISUAL}", sel)}
	}
	return nil
}

// LatexAutoFraction is a/ as \frac{a}{}: before ends with the / just typed.
// The term is the word, command, number or bracketed group before it, with
// its ^ and _; (a+b)/ loses its brackets.
func LatexAutoFraction(before string) *LatexExpansion {
	if !strings.HasSuffix(before, "/") || strings.HasSuffix(before, "//") {
		return nil
	}
	end := len(before) - 1
	j := end
	open := map[byte]byte{')': '(', ']': '[', '}': '{'}
	for j > 0 {
		c := before[j-1]
		if o, ok := open[c]; ok {
			depth, k := 0, j-1
			for ; k >= 0; k-- {
				if before[k] == c {
					depth++
				} else if before[k] == o {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			if k < 0 {
				break
			}
			j = k
			continue
		}
		if isLatexLetter(c) || c >= '0' && c <= '9' || c == '.' || c == '^' || c == '_' {
			j--
			continue
		}
		if c == '\\' && j < end && isLatexLetter(before[j]) && !(j >= 2 && before[j-2] == '\\') {
			j--
			continue
		}
		break
	}
	term := before[j:end]
	if term == "" {
		return nil
	}
	if term[0] == '(' && term[len(term)-1] == ')' && latexOneGroup(term) {
		term = term[1 : len(term)-1]
	}
	return &LatexExpansion{
		Snippet: LatexSnippet{Trigger: "/", Description: "a fraction of what is before it", Options: "mA"},
		Remove:  len(before) - j,
		Body:    `\frac{` + strings.ReplaceAll(term, "$", `\$`) + `}{$1}$0`,
	}
}

// latexOneGroup reports whether the ( at the start closes at the very end.
func latexOneGroup(s string) bool {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i < len(s)-1 {
				return false
			}
		}
	}
	return depth == 0
}

// LatexStop is one place TAB visits, From..To in the placed text.
type LatexStop struct {
	Index    int
	From, To int
}

// LatexPlaced is a body with its tab stops taken out: Stops in the order TAB
// visits them ($1, $2…, then $0 or the end; a number used twice has an entry
// for each place).
type LatexPlaced struct {
	Text  string
	Stops []LatexStop
}

type latexToken struct {
	text   string
	stop   int // -1: text
	def    string
	hasDef bool
}

// LatexPlace takes the tab stops out of a body.
func LatexPlace(body string) LatexPlaced {
	toks := []latexToken{}
	lit := strings.Builder{}
	flush := func() {
		if lit.Len() > 0 {
			toks = append(toks, latexToken{text: lit.String(), stop: -1})
			lit.Reset()
		}
	}
	digits := func(i int) int {
		j := i
		for j < len(body) && body[j] >= '0' && body[j] <= '9' {
			j++
		}
		return j
	}
	for i := 0; i < len(body); {
		c := body[i]
		if c == '\\' && i+1 < len(body) && body[i+1] == '$' {
			lit.WriteByte('$')
			i += 2
			continue
		}
		if c == '$' {
			if j := digits(i + 1); j > i+1 {
				n, _ := strconv.Atoi(body[i+1 : j])
				flush()
				toks = append(toks, latexToken{stop: n})
				i = j
				continue
			}
			if i+1 < len(body) && body[i+1] == '{' {
				if j := digits(i + 2); j > i+2 && j < len(body) && (body[j] == '}' || body[j] == ':') {
					n, _ := strconv.Atoi(body[i+2 : j])
					if body[j] == '}' {
						flush()
						toks = append(toks, latexToken{stop: n})
						i = j + 1
						continue
					}
					depth, k := 1, j+1
					for ; k < len(body); k++ {
						if body[k] == '\\' {
							k++
							continue
						}
						if body[k] == '{' {
							depth++
						} else if body[k] == '}' {
							depth--
							if depth == 0 {
								break
							}
						}
					}
					if k < len(body) {
						flush()
						toks = append(toks, latexToken{stop: n, def: body[j+1 : k], hasDef: true})
						i = k + 1
						continue
					}
				}
			}
		}
		lit.WriteByte(c)
		i++
	}
	flush()
	defs := map[int]string{}
	for _, t := range toks {
		if t.stop >= 0 && t.hasDef {
			if _, ok := defs[t.stop]; !ok {
				defs[t.stop] = t.def
			}
		}
	}
	var b strings.Builder
	type occ struct{ idx, from, to, order int }
	occs := []occ{}
	for _, t := range toks {
		if t.stop < 0 {
			b.WriteString(t.text)
			continue
		}
		d := t.def
		if !t.hasDef {
			d = defs[t.stop]
		}
		from := b.Len()
		b.WriteString(d)
		occs = append(occs, occ{t.stop, from, b.Len(), len(occs)})
	}
	sort.SliceStable(occs, func(i, j int) bool {
		a, c := occs[i].idx, occs[j].idx
		if a == 0 || c == 0 {
			return a != 0 && c == 0
		}
		return a < c
	})
	out := LatexPlaced{Text: b.String(), Stops: []LatexStop{}}
	hasZero := false
	for _, o := range occs {
		out.Stops = append(out.Stops, LatexStop{o.idx, o.from, o.to})
		hasZero = hasZero || o.idx == 0
	}
	if !hasZero {
		out.Stops = append(out.Stops, LatexStop{0, len(out.Text), len(out.Text)})
	}
	return out
}

// ---------------------------------------------------------------------------
// Finding the maths
// ---------------------------------------------------------------------------

// LatexFragment is one piece of maths: Start..End the whole of it with its
// delimiters, From..To what is inside. An unclosed one stops at a blank line
// or the end of the text.
type LatexFragment struct {
	Open, Close string
	Env         string `json:",omitempty"`
	Start, End  int
	From, To    int
	Display     bool
	Closed      bool
}

// LatexMathEnvs are the environments org reads as maths on their own.
var LatexMathEnvs = map[string]bool{}

// LatexKnownEnvs are the environments KaTeX and MathJax draw.
var LatexKnownEnvs = map[string]bool{}

func init() {
	for _, e := range []string{"equation", "align", "gather", "multline", "eqnarray", "flalign", "alignat", "math", "displaymath"} {
		LatexMathEnvs[e], LatexMathEnvs[e+"*"] = true, true
	}
	for _, e := range []string{"matrix", "pmatrix", "bmatrix", "Bmatrix", "vmatrix", "Vmatrix", "smallmatrix", "array", "darray",
		"cases", "dcases", "rcases", "drcases", "aligned", "alignedat", "gathered", "split", "subarray", "CD",
		"equation", "align", "gather", "multline", "eqnarray", "flalign", "alignat", "math", "displaymath"} {
		LatexKnownEnvs[e], LatexKnownEnvs[e+"*"] = true, true
	}
}

var latexBeginRe = regexp.MustCompile(`^\\begin\{([A-Za-z]+\*?)\}`)
var latexBlockBeginRe = regexp.MustCompile(`(?i)^\s*#\+begin_(src|example|export|verbatim|comment)\b`)
var latexBlockEndRe = regexp.MustCompile(`(?i)^\s*#\+end_(src|example|export|verbatim|comment)\b`)
var latexNoMathLineRe = regexp.MustCompile(`^\s*(#\+|#\s|#$|:\s|:$)`)

// LatexMask blanks what org never reads maths in (source, example, export and
// comment blocks, keyword and comment lines, fixed-width lines), keeping every
// offset where it was.
func LatexMask(text string) string {
	lines := strings.Split(text, "\n")
	in := false
	for i, l := range lines {
		blank := in || latexNoMathLineRe.MatchString(l)
		if !in && latexBlockBeginRe.MatchString(l) {
			in, blank = true, true
		} else if in && latexBlockEndRe.MatchString(l) {
			in = false
		}
		if blank {
			lines[i] = strings.Repeat(" ", len(l))
		}
	}
	return strings.Join(lines, "\n")
}

// latexBlankAt is the index of the blank line starting at or after i (the
// \n that ends the line before it), or -1.
func latexParaEnd(s string, from int) int {
	for j := from; j < len(s); j++ {
		if s[j] != '\n' {
			continue
		}
		k := j + 1
		for k < len(s) && (s[k] == ' ' || s[k] == '\t' || s[k] == '\r') {
			k++
		}
		if k < len(s) && s[k] == '\n' || k == len(s) && k > j+1 {
			return j
		}
	}
	return -1
}

func isLatexSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// LatexFragments finds the maths in a text (mask it first).
func LatexFragments(s string) []LatexFragment {
	out := []LatexFragment{}
	frag := func(i int, open, close string, display bool, env string) int {
		f := LatexFragment{Open: open, Close: close, Env: env, Start: i, From: i + len(open), Display: display}
		stop := latexParaEnd(s, f.From)
		if stop < 0 {
			stop = len(s)
		}
		j := f.From
		for j < stop {
			if strings.HasPrefix(s[j:], close) {
				break
			}
			if s[j] == '\\' {
				j += 2
				continue
			}
			j++
		}
		if j < stop && strings.HasPrefix(s[j:], close) {
			f.To, f.End, f.Closed = j, j+len(close), true
		} else {
			f.To, f.End = stop, stop
		}
		out = append(out, f)
		return f.End
	}
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '(':
				i = frag(i, `\(`, `\)`, false, "")
				continue
			case '[':
				i = frag(i, `\[`, `\]`, true, "")
				continue
			case 'b':
				if m := latexBeginRe.FindStringSubmatch(s[i:]); m != nil && LatexMathEnvs[m[1]] {
					i = frag(i, m[0], `\end{`+m[1]+`}`, true, m[1])
					continue
				}
			}
			i += 2
			continue
		}
		if c == '$' {
			if i+1 < len(s) && s[i+1] == '$' {
				i = frag(i, "$$", "$$", true, "")
				continue
			}
			if i+1 < len(s) && !isLatexSpace(s[i+1]) {
				// The closing $ has no space before it and no letter or digit after.
				stop := latexParaEnd(s, i+1)
				if stop < 0 {
					stop = len(s)
				}
				j := i + 1
				for j < stop {
					if s[j] == '\\' {
						j += 2
						continue
					}
					if s[j] == '$' && !isLatexSpace(s[j-1]) && (j+1 >= len(s) || !isLatexAlnum(s[j+1])) {
						break
					}
					j++
				}
				if j < stop {
					out = append(out, LatexFragment{Open: "$", Close: "$", Start: i, From: i + 1, To: j, End: j + 1, Closed: true})
					i = j + 1
					continue
				}
				// Unclosed: maths being typed, unless it is a price.
				if !(s[i+1] >= '0' && s[i+1] <= '9') {
					out = append(out, LatexFragment{Open: "$", Close: "$", Start: i, From: i + 1, To: stop, End: stop})
					i = stop
					continue
				}
			}
		}
		i++
	}
	return out
}

func isLatexAlnum(c byte) bool { return isLatexLetter(c) || c >= '0' && c <= '9' }

// LatexAt is the mode at a place in a text, and the maths it is in.
type LatexAt struct {
	Mode     string // text, inline or display
	Fragment *LatexFragment
}

var latexTextCmdRe = regexp.MustCompile(`^\\(text|textrm|textbf|textit|textsf|texttt|mbox|operatorname)\{`)

// LatexModeAt says whether offset is in maths. The scan starts at the blank
// line before it (maths never crosses one) and ends at the one after.
func LatexModeAt(text string, offset int) LatexAt {
	if offset > len(text) {
		offset = len(text)
	}
	start := offset - 20000
	if start < 0 {
		start = 0
	}
	for j := offset - 1; j > start; j-- {
		if text[j] == '\n' {
			k := j - 1
			for k > start && (text[k] == ' ' || text[k] == '\t' || text[k] == '\r') {
				k--
			}
			if text[k] == '\n' {
				start = j + 1
				break
			}
		}
	}
	end := latexParaEnd(text, offset)
	if end < 0 {
		end = len(text)
	}
	if end-offset > 20000 {
		end = offset + 20000
	}
	// Mask from the start of the text's line so a block line is recognised.
	seg := LatexMask(text[start:end])
	rel := offset - start
	for _, f := range LatexFragments(seg) {
		if rel < f.From || rel > f.To {
			continue
		}
		g := f
		g.Start, g.End, g.From, g.To = f.Start+start, f.End+start, f.From+start, f.To+start
		mode := "inline"
		if f.Display {
			mode = "display"
		}
		if latexInTextCmd(seg[f.From:rel]) {
			mode = "text"
		}
		return LatexAt{Mode: mode, Fragment: &g}
	}
	return LatexAt{Mode: "text"}
}

// latexInTextCmd reports whether s (maths up to the cursor) ends inside \text{.
func latexInTextCmd(s string) bool {
	stack := []bool{}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if m := latexTextCmdRe.FindString(s[i:]); m != "" {
				stack = append(stack, true)
				i += len(m) - 1
				continue
			}
			i++
		case '{':
			stack = append(stack, false)
		case '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	for _, t := range stack {
		if t {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Checking it
// ---------------------------------------------------------------------------

// LatexProblem is something wrong with a file's maths. Line and Col count
// from 1, Col in characters.
type LatexProblem struct {
	Line, Col int
	Severity  string // error or warning
	Rule      string
	Msg       string
}

var latexLeftRightRe = regexp.MustCompile(`\\(left|right)([^A-Za-z]|$)`)
var latexEnvRe = regexp.MustCompile(`\\(begin|end)\{([^}]*)\}`)

// LatexCheck finds unbalanced braces, \left without \right, environments that
// do not close or that KaTeX and MathJax do not know, maths that never closes
// and a $ that org will not read as maths.
func LatexCheck(text string) []LatexProblem {
	s := LatexMask(text)
	out := []LatexProblem{}
	at := func(off int, sev, rule, msg string) {
		line := strings.Count(s[:off], "\n") + 1
		ls := strings.LastIndex(s[:off], "\n") + 1
		out = append(out, LatexProblem{Line: line, Col: utf8.RuneCountInString(text[ls:off]) + 1, Severity: sev, Rule: rule, Msg: msg})
	}
	frags := LatexFragments(s)
	for _, f := range frags {
		if !f.Closed {
			if f.Open != "$" {
				at(f.Start, "error", "latex-unclosed", f.Open+" is never closed by "+f.Close+" (maths stops at a blank line)")
			}
			continue
		}
		body := s[f.From:f.To]
		// Braces.
		stack := []int{}
		for i := 0; i < len(body); i++ {
			switch body[i] {
			case '\\':
				i++
			case '{':
				stack = append(stack, i)
			case '}':
				if len(stack) == 0 {
					at(f.From+i, "error", "latex-brace", "a } with no { before it")
				} else {
					stack = stack[:len(stack)-1]
				}
			}
		}
		for _, p := range stack {
			at(f.From+p, "error", "latex-brace", "a { that is never closed")
		}
		// \left and \right.
		lr := []int{}
		for _, m := range latexLeftRightRe.FindAllStringSubmatchIndex(body, -1) {
			if body[m[2]:m[3]] == "left" {
				lr = append(lr, m[0])
			} else if len(lr) == 0 {
				at(f.From+m[0], "error", "latex-leftright", `a \right with no \left`)
			} else {
				lr = lr[:len(lr)-1]
			}
		}
		for _, p := range lr {
			at(f.From+p, "error", "latex-leftright", `a \left with no \right`)
		}
		// Environments.
		type env struct {
			name string
			off  int
		}
		envs := []env{}
		for _, m := range latexEnvRe.FindAllStringSubmatchIndex(body, -1) {
			name := body[m[4]:m[5]]
			if body[m[2]:m[3]] == "begin" {
				if !LatexKnownEnvs[name] {
					at(f.From+m[0], "warning", "latex-env", `KaTeX and MathJax do not know \begin{`+name+`}`)
				}
				envs = append(envs, env{name, m[0]})
				continue
			}
			if len(envs) == 0 {
				at(f.From+m[0], "error", "latex-env", `\end{`+name+`} with no \begin{`+name+`}`)
				continue
			}
			top := envs[len(envs)-1]
			envs = envs[:len(envs)-1]
			if top.name != name {
				at(f.From+m[0], "error", "latex-env", `\end{`+name+`} closes \begin{`+top.name+`}`)
			}
		}
		for _, e := range envs {
			at(f.From+e.off, "error", "latex-env", `\begin{`+e.name+`} is never ended`)
		}
	}
	// $ x$: a space after the opening $ makes it text to org.
	inFrag := func(i int) bool {
		for _, f := range frags {
			if i >= f.Start && i < f.End {
				return true
			}
		}
		return false
	}
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] != '$' || (i > 0 && s[i-1] == '$') || s[i+1] == '$' || (s[i+1] != ' ' && s[i+1] != '\t') || inFrag(i) {
			continue
		}
		eol := strings.IndexByte(s[i+1:], '\n')
		if eol < 0 {
			eol = len(s) - i - 1
		}
		if j := strings.IndexByte(s[i+1:i+1+eol], '$'); j > 0 {
			at(i, "warning", "latex-dollar", "org reads $ followed by a space as text, not maths: write $x$ or \\( x \\)")
			i += j + 1
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Col < out[j].Col
	})
	return out
}

// LatexSuiteOn is set by the server to featureOn("latexSuite"); nil (a test,
// a client) is off. The html exporter asks LatexSuiteActive before drawing
// maths with KaTeX.
var LatexSuiteOn func() bool

// LatexKatexBase is where an html export loads KaTeX from (the server sets it
// from latexSuite.katex); "off" loads nothing.
var LatexKatexBase = func() string { return "https://cdn.jsdelivr.net/npm/katex@0.16.11/dist" }

func LatexSuiteActive() bool { return LatexSuiteOn != nil && LatexSuiteOn() }
