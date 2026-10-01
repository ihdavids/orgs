package orghl

import (
	"strings"
	"testing"
)

// The one thing a highlighter is not allowed to get wrong. A colour is a
// judgement; a lost character is a bug, and in a capture form it is a bug that
// files somebody's note with a letter missing from it.
func TestSpansAreTheLine(t *testing.T) {
	lines := []string{
		"* TODO Buy milk :errand:shop:",
		"** DONE [#A] Something /emphasised/ and =verbatim=",
		":PROPERTIES:",
		":CUSTOM_ID: f81d4fae-7dec-11d0-a765-00a0c91e6bf6",
		":CREATED:   [2026-09-30 Wed 17:20]",
		":END:",
		"SCHEDULED: <2026-09-30 Wed .+2d>",
		"- [ ] a box to tick",
		"  - a plain bullet with [[file:notes.org][a link]] in it",
		"| a | table | row |",
		"|---+-------+-----|",
		"#+TITLE: A file",
		"#+BEGIN_SRC python",
		"  print('[not a tag]')",
		"#+END_SRC",
		"# a comment",
		"a line with {{a|=hole}} in it",
		"arithmetic 2*3*4 and a stray ] and [",
		"",
		"   ",
		"a*b_c/d=e~f+g",
	}
	st := &State{Active: []string{"TODO", "NEXT"}, Done: []string{"DONE"}}
	for _, l := range lines {
		var b strings.Builder
		for _, s := range Line(l, st) {
			b.WriteString(s.Text)
		}
		if b.String() != l {
			t.Errorf("line %q came back as %q", l, b.String())
		}
	}
}

// And the same for a whole passage read in order, which is the only way the
// block and drawer state is exercised.
func TestTextIsTheText(t *testing.T) {
	text := "* NEXT Thing\n:PROPERTIES:\n:ID: x\n:END:\n#+BEGIN_EXAMPLE\n* not a heading\n#+END_EXAMPLE\ndone\n"
	st := &State{Active: []string{"NEXT"}}
	if got := ANSIText(text, st, false); got != text {
		t.Errorf("got %q want %q", got, text)
	}
}

func roleOf(spans []Span, text string) (Role, bool) {
	for _, s := range spans {
		if s.Text == text {
			return s.Role, true
		}
	}
	return Plain, false
}

func wants(t *testing.T, spans []Span, text string, want Role) {
	t.Helper()
	got, ok := roleOf(spans, text)
	if !ok {
		t.Errorf("no span for %q in %+v", text, spans)
		return
	}
	if got != want {
		t.Errorf("%q: role %d, want %d", text, got, want)
	}
}

func TestHeadingParts(t *testing.T) {
	st := &State{Active: []string{"TODO", "NEXT"}, Done: []string{"DONE", "CANCELLED"}}
	spans := Line("** NEXT [#B] Call the plumber :home:phone:", st)
	wants(t, spans, "**", Stars)
	wants(t, spans, "NEXT", TodoKeyword)
	wants(t, spans, "[#B]", Priority)
	wants(t, spans, "Call the plumber", Title)
	wants(t, spans, ":home:phone:", Tag)

	// A done keyword is coloured as done, which is the whole reason the server
	// is asked for the two lists rather than one.
	wants(t, Line("* DONE Finished", st), "DONE", DoneKeyword)

	// A keyword this server does not have is just words - offering a colour for
	// it would be claiming the file has a keyword it does not.
	if r, _ := roleOf(Line("* WIBBLE Something", st), "WIBBLE Something"); r != Title {
		t.Errorf("an unknown keyword should be part of the title")
	}
	// A word that merely starts with a keyword is not the keyword.
	if _, ok := roleOf(Line("* TODOs are piling up", st), "TODO"); ok {
		t.Errorf("TODOs is not the keyword TODO")
	}
}

func TestDrawersAndTimestamps(t *testing.T) {
	st := &State{}
	open := Line(":PROPERTIES:", st)
	wants(t, open, ":PROPERTIES:", DrawerName)
	if !st.InDrawer {
		t.Fatalf("the drawer did not open")
	}
	prop := Line(":CREATED:   [2026-09-30 Wed 17:20]", st)
	wants(t, prop, ":CREATED:", PropKey)
	wants(t, prop, "[2026-09-30 Wed 17:20]", Timestamp)

	Line(":END:", st)
	if st.InDrawer {
		t.Fatalf("the drawer did not close")
	}
	// Outside a drawer the same line is not a property - it is a line of text
	// that happens to start with a colon.
	if _, ok := roleOf(Line(":CREATED:   x", st), ":CREATED:"); ok {
		t.Errorf("a property key outside a drawer should not be coloured as one")
	}

	plan := Line("SCHEDULED: <2026-09-30 Wed .+2d>", st)
	wants(t, plan, "SCHEDULED:", Planning)
	wants(t, plan, "<2026-09-30 Wed .+2d>", Timestamp)
}

func TestLinksHolesAndBlocks(t *testing.T) {
	st := &State{}
	link := Line("see [[file:notes.org][the notes]] for more", st)
	wants(t, link, "the notes", LinkDesc)
	wants(t, link, "[[file:notes.org][", Link)

	hole := Line(":SOURCE: {{source|Where from?}}", st)
	wants(t, hole, "{{source|Where from?}}", Hole)

	fence := Line("#+BEGIN_SRC python", st)
	wants(t, fence, "#+BEGIN_SRC python", BlockMarker)
	if st.InBlock != "SRC" {
		t.Fatalf("the block did not open: %q", st.InBlock)
	}
	// A heading inside a block is not a heading.
	wants(t, Line("* not a heading", st), "* not a heading", SrcText)
	Line("#+END_SRC", st)
	if st.InBlock != "" {
		t.Fatalf("the block did not close")
	}
}

// A timestamp and a link are tags as far as tview is concerned, so the escaping
// is the part that matters more than the colours.
func TestTviewEscapesBrackets(t *testing.T) {
	st := &State{}
	got := Tview(Line(":CREATED: [2026-09-30 Wed 17:20]", st))
	if !strings.Contains(got, "[2026-09-30 Wed 17:20[]") {
		t.Errorf("the timestamp was not escaped for tview: %q", got)
	}
}

// Without colour, ANSI is the plain text and nothing else: the same answer in a
// form a pipe can use.
func TestANSIPlainIsPlain(t *testing.T) {
	st := &State{Active: []string{"TODO"}}
	line := "* TODO Buy milk :errand:"
	if got := ANSI(Line(line, st), false); got != line {
		t.Errorf("got %q", got)
	}
	if got := ANSI(Line(line, st), true); !strings.Contains(got, "\033[") {
		t.Errorf("with colour on there should be escapes: %q", got)
	}
}
