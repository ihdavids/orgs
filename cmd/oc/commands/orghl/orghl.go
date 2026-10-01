// Org syntax colouring for a terminal.
//
// This is `worg/src/codehl.ts`'s sibling, for the other kind of screen, and it
// is a *scanner* for the same reasons: it does not know scope, it cannot tell a
// multiplication from a pair of bold markers, and it will occasionally give one
// word the wrong colour - which is the failure a highlighter is allowed. What
// it must never do is lose a character, and `TestSpansAreTheLine` pins exactly
// that: every span of a line, concatenated, is the line it was given.
//
// It answers in spans rather than in escape codes, so the same walk can be
// drawn two ways - `ANSI` for anything printing to a terminal, `Tview` for the
// full-screen forms - and so a caller can do something else with a span
// entirely, which is what `orgs cap` does to put a cursor inside one.
//
// A line is read in the state the lines above it left (`State`), because org's
// two multi-line constructs - a source block and a drawer - change what the
// lines inside them mean. Reading a screenful out of the middle of a file
// without that state starts every source block in the wrong place, which is the
// bug worg's highlighter documents on its own side.
package orghl

import (
	"regexp"
	"strings"
)

// Role is what a span of text turned out to be. The drawing tables are keyed on
// it, and a caller that wants its own palette only has to answer this.
type Role int

const (
	Plain Role = iota
	Stars
	Title
	TodoKeyword
	DoneKeyword
	Priority
	Tag
	DrawerName
	PropKey
	Planning
	Timestamp
	Link
	LinkDesc
	Comment
	Directive
	DirectiveValue
	BlockMarker
	Code
	Bold
	Italic
	Underline
	Strike
	Bullet
	Checkbox
	TableSep
	SrcText
	// Hole is a capture template's placeholder - not org at all, but this is
	// the one place org text is read with holes still in it.
	Hole
)

// State is what the lines above this one left behind.
type State struct {
	// Inside a #+BEGIN_ block, and which one.
	InBlock string
	// Inside a :PROPERTIES:/:LOGBOOK: drawer.
	InDrawer bool
	// The keywords this server accepts, so a heading's keyword is coloured as
	// the thing it is rather than matched against a list compiled in here. Both
	// are optional: with neither, a heading is just a heading.
	Active []string
	Done   []string
}

// Span is a run of text that is all one thing.
type Span struct {
	Text string
	Role Role
}

var (
	headingRe   = regexp.MustCompile(`^(\*+)(\s+)`)
	directiveRe = regexp.MustCompile(`^(\s*)(#\+[A-Za-z_]+:?)(.*)$`)
	blockRe     = regexp.MustCompile(`^\s*#\+(BEGIN|END)_([A-Za-z]+)`)
	drawerRe    = regexp.MustCompile(`^(\s*)(:[A-Za-z][A-Za-z0-9_-]*:)(\s*)$`)
	propRe      = regexp.MustCompile(`^(\s*)(:[A-Za-z][A-Za-z0-9_-]*:)(\s*)(.*)$`)
	planningRe  = regexp.MustCompile(`^(\s*)(SCHEDULED:|DEADLINE:|CLOSED:)`)
	bulletRe    = regexp.MustCompile(`^(\s*)([-+*]|\d+[.)])(\s+)`)
	checkboxRe  = regexp.MustCompile(`^(\[[ xX-]\])(\s*)`)
	tagsRe      = regexp.MustCompile(`\s+(:[A-Za-z0-9_@#%:]+:)$`)
	priorityRe  = regexp.MustCompile(`^(\[#[A-Za-z0-9]\])(\s*)`)
)

// Line walks one line and advances the state. The caller owns the state and
// passes the same one down the file.
func Line(line string, st *State) []Span {
	out := []Span{}
	add := func(t string, r Role) {
		if t != "" {
			out = append(out, Span{t, r})
		}
	}

	// A block's fence, and everything inside it. The fence is read first so
	// that #+END_SRC is a fence rather than the last line of the block.
	if m := blockRe.FindStringSubmatch(line); m != nil {
		if strings.EqualFold(m[1], "BEGIN") {
			st.InBlock = strings.ToUpper(m[2])
		} else {
			st.InBlock = ""
		}
		add(line, BlockMarker)
		return out
	}
	if st.InBlock != "" {
		// The code inside a block is deliberately not coloured as code: a
		// terminal capture form is not an editor, and half-colouring by
		// guesswork reads worse than not colouring at all.
		add(line, SrcText)
		return out
	}

	// A heading.
	if m := headingRe.FindStringSubmatch(line); m != nil {
		add(m[1], Stars)
		add(m[2], Plain)
		rest := line[len(m[0]):]
		// Tags come off the end first, so a keyword search never reaches them.
		tags := ""
		if tm := tagsRe.FindStringSubmatch(rest); tm != nil {
			tags = rest[len(rest)-len(tm[0]):]
			rest = rest[:len(rest)-len(tm[0])]
		}
		if kw, role, n := leadingKeyword(rest, st); n > 0 {
			add(kw, role)
			rest = rest[n:]
			if i := spaceRun(rest); i > 0 {
				add(rest[:i], Plain)
				rest = rest[i:]
			}
		}
		if pm := priorityRe.FindStringSubmatch(rest); pm != nil {
			add(pm[1], Priority)
			add(pm[2], Plain)
			rest = rest[len(pm[0]):]
		}
		inline(rest, &out, Title)
		if tags != "" {
			cut := len(tags) - len(strings.TrimLeft(tags, " \t"))
			add(tags[:cut], Plain)
			add(tags[cut:], Tag)
		}
		return out
	}

	// A drawer on its own line opens or closes one.
	if m := drawerRe.FindStringSubmatch(line); m != nil {
		add(m[1], Plain)
		add(m[2], DrawerName)
		add(m[3], Plain)
		st.InDrawer = !strings.EqualFold(m[2], ":END:")
		return out
	}
	// A property inside a drawer: the key is the key, the value is read for
	// timestamps and links like anything else.
	if st.InDrawer {
		if m := propRe.FindStringSubmatch(line); m != nil {
			add(m[1], Plain)
			add(m[2], PropKey)
			add(m[3], Plain)
			inline(m[4], &out, Plain)
			return out
		}
	}

	// A planning line, and a comment.
	if m := planningRe.FindStringSubmatch(line); m != nil {
		add(m[1], Plain)
		add(m[2], Planning)
		inline(line[len(m[0]):], &out, Plain)
		return out
	}
	if t := strings.TrimLeft(line, " \t"); strings.HasPrefix(t, "# ") || t == "#" {
		add(line, Comment)
		return out
	}
	if m := directiveRe.FindStringSubmatch(line); m != nil {
		add(m[1], Plain)
		add(m[2], Directive)
		add(m[3], DirectiveValue)
		return out
	}
	if strings.HasPrefix(strings.TrimLeft(line, " \t"), "|") {
		table(line, &out)
		return out
	}

	// A list item, with or without a box on it.
	rest := line
	if m := bulletRe.FindStringSubmatch(rest); m != nil {
		add(m[1], Plain)
		add(m[2], Bullet)
		add(m[3], Plain)
		rest = rest[len(m[0]):]
		if cm := checkboxRe.FindStringSubmatch(rest); cm != nil {
			add(cm[1], Checkbox)
			add(cm[2], Plain)
			rest = rest[len(cm[0]):]
		}
	}
	inline(rest, &out, Plain)
	return out
}

// Lines is the whole of a passage, each line's spans in order, carrying the
// state down it.
func Lines(text string, st *State) [][]Span {
	out := [][]Span{}
	for _, l := range strings.Split(text, "\n") {
		out = append(out, Line(l, st))
	}
	return out
}

func spaceRun(s string) int {
	return len(s) - len(strings.TrimLeft(s, " \t"))
}

// leadingKeyword answers with the todo keyword a headline starts with, if the
// server said there was one. Matching is by literal prefix at a word boundary,
// the way the parser does it.
func leadingKeyword(s string, st *State) (string, Role, int) {
	try := func(words []string, role Role) (string, Role, int) {
		for _, w := range words {
			if w == "" || !strings.HasPrefix(s, w) {
				continue
			}
			if len(s) > len(w) && s[len(w)] != ' ' && s[len(w)] != '\t' {
				continue
			}
			return w, role, len(w)
		}
		return "", Plain, 0
	}
	if w, r, n := try(st.Done, DoneKeyword); n > 0 {
		return w, r, n
	}
	return try(st.Active, TodoKeyword)
}

func table(line string, out *[]Span) {
	add := func(t string, r Role) {
		if t != "" {
			*out = append(*out, Span{t, r})
		}
	}
	// A rule is all one thing - there is nothing in it to read.
	if strings.ContainsAny(line, "-") && strings.Trim(line, " \t|-+") == "" {
		add(line, TableSep)
		return
	}
	for len(line) > 0 {
		i := strings.IndexByte(line, '|')
		if i < 0 {
			inline(line, out, Plain)
			return
		}
		inline(line[:i], out, Plain)
		add("|", TableSep)
		line = line[i+1:]
	}
}

// The inline pass: links, timestamps, emphasis, and a capture template's holes.
func inline(s string, out *[]Span, base Role) {
	add := func(t string, r Role) {
		if t != "" {
			*out = append(*out, Span{t, r})
		}
	}
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			add(buf.String(), base)
			buf.Reset()
		}
	}
	for i := 0; i < len(s); {
		// [[target][description]] or [[target]]
		if strings.HasPrefix(s[i:], "[[") {
			if end := strings.Index(s[i:], "]]"); end > 0 {
				whole := s[i : i+end+2]
				flush()
				if mid := strings.Index(whole, "]["); mid > 0 {
					add(whole[:mid+2], Link)
					add(whole[mid+2:len(whole)-2], LinkDesc)
					add("]]", Link)
				} else {
					add(whole, Link)
				}
				i += end + 2
				continue
			}
		}
		// A capture template's hole. Not org, but this is where org text is
		// read with holes still in it.
		if strings.HasPrefix(s[i:], "{{") {
			if end := strings.Index(s[i:], "}}"); end > 0 {
				flush()
				add(s[i:i+end+2], Hole)
				i += end + 2
				continue
			}
		}
		// A timestamp: <2026-09-30 Wed> or [2026-09-30 Wed 14:05]. Both have to
		// start with a digit, or every bracket in a sentence is a timestamp.
		if c := s[i]; c == '<' || c == '[' {
			close := byte('>')
			if c == '[' {
				close = ']'
			}
			if end := strings.IndexByte(s[i:], close); end > 1 && isDigit(s[i+1]) {
				body := s[i+1 : i+end]
				if len(body) <= 48 && !strings.ContainsRune(body, rune(c)) {
					flush()
					add(s[i:i+end+1], Timestamp)
					i += end + 1
					continue
				}
			}
		}
		// Emphasis. Org wants a marker at a word edge with something that is
		// not a space just inside it; anything looser colours arithmetic.
		if r, ok := emphRole(s[i]); ok && emphStart(s, i) {
			if end := emphEnd(s, i); end > i {
				flush()
				add(s[i:end+1], r)
				i = end + 1
				continue
			}
		}
		buf.WriteByte(s[i])
		i++
	}
	flush()
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func emphRole(b byte) (Role, bool) {
	switch b {
	case '=', '~':
		return Code, true
	case '*':
		return Bold, true
	case '/':
		return Italic, true
	case '_':
		return Underline, true
	case '+':
		return Strike, true
	}
	return Plain, false
}

// A marker opens emphasis when what is before it is nothing or a space or
// punctuation, and what is after it is neither a space nor the end.
func emphStart(s string, i int) bool {
	if i > 0 {
		p := s[i-1]
		if !(p == ' ' || p == '\t' || p == '(' || p == '{' || p == '\'' || p == '"' || p == '-') {
			return false
		}
	}
	if i+1 >= len(s) || s[i+1] == ' ' || s[i+1] == '\t' || s[i+1] == s[i] {
		return false
	}
	return true
}

// ...and closes it at the matching marker, which must have something that is
// not a space just before it.
func emphEnd(s string, i int) int {
	for j := i + 1; j < len(s); j++ {
		if s[j] != s[i] {
			continue
		}
		if p := s[j-1]; p == ' ' || p == '\t' {
			continue
		}
		if j+1 < len(s) {
			n := s[j+1]
			if !(n == ' ' || n == '\t' || n == '.' || n == ',' || n == ';' || n == ':' || n == '!' || n == '?' || n == ')' || n == '}' || n == '"' || n == '\'') {
				continue
			}
		}
		return j
	}
	return -1
}
