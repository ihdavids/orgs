package capture

import (
	"strings"
	"testing"

	"github.com/ihdavids/orgs/cmd/oc/commands/orghl"
	"github.com/ihdavids/orgs/internal/common"
)

const tpl = `:PROPERTIES:
:CUSTOM_ID: {{uuid|=f81d4fae-7dec-11d0-a765-00a0c91e6bf6}}
:SOURCE:    {{source|Where did this come from?}}
:PROJECT:   {{project}}
:END:
{{CONTENT}}`

func testForm(t *testing.T) *form {
	t.Helper()
	ct := common.CaptureTemplate{Name: "Note", Type: "entry", Template: tpl}
	f := &form{tpl: ct, fields: buildFields(ct, true), level: 2}
	f.st = orghl.State{Active: []string{"TODO", "NEXT"}, Done: []string{"DONE"}}
	return f
}

// What the form is for: the entry as it will be filed. The stars are the
// target's level plus one and the body is indented under them, which is what
// InsertEntryUsingTemplate does - a preview of something else would be worse
// than no preview.
func TestEntryIsTheEntryAsFiled(t *testing.T) {
	f := testForm(t)
	setField(f.fields, "HEADLINE", "Buy milk")
	setField(f.fields, "TAGS", "errand, shop")
	setField(f.fields, "CONTENT", "two pints\nand a paper")
	f.at = indexOfField(f.fields, "CONTENT")
	f.cur = 0

	got := strings.ReplaceAll(f.entry(), caretMark, "")
	want := []string{
		"** Buy milk  :errand:shop:",
		"   :PROPERTIES:",
		"   :CUSTOM_ID: f81d4fae-7dec-11d0-a765-00a0c91e6bf6",
		// A hole nobody has answered keeps its braces rather than becoming a
		// blank: the line is waiting, and it says so.
		"   :PROJECT:   {{project}}",
		"   :END:",
		"   two pints",
		"   and a paper",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("expected %q in:\n%s", w, got)
		}
	}
	// The question keeps its question until it is answered.
	if !strings.Contains(got, "{{source|Where did this come from?}}") {
		t.Errorf("the source question went missing:\n%s", got)
	}
}

// The caret goes where the cursor is, in the field being edited and nowhere
// else - one caret in the whole entry, however many fields there are.
func TestCaretIsWhereTheCursorIs(t *testing.T) {
	f := testForm(t)
	setField(f.fields, "HEADLINE", "Buy milk")
	f.at = indexOfField(f.fields, "HEADLINE")
	f.cur = 3

	got := f.entry()
	if n := strings.Count(got, caretMark); n != 1 {
		t.Fatalf("expected one caret, got %d:\n%s", n, got)
	}
	if !strings.HasPrefix(got, "** Buy"+caretMark+" milk") {
		t.Errorf("the caret is in the wrong place: %q", strings.SplitN(got, "\n", 2)[0])
	}

	// An empty field being edited is a caret and nothing else - which is the
	// only thing on screen saying where you are about to type.
	f.at = indexOfField(f.fields, "source")
	f.cur = 0
	got = f.entry()
	if !strings.Contains(got, ":SOURCE:    "+caretMark+"\n") {
		t.Errorf("an empty active field should be just the caret:\n%s", got)
	}
	// ...and it is no longer drawn as a hole, because it is being filled in.
	if strings.Contains(got, "{{source") {
		t.Errorf("the active field should not still be a hole:\n%s", got)
	}
}

// A template with no string of its own is still a headline and a body, which is
// what every template in the shipped config is.
func TestEntryWithNoTemplate(t *testing.T) {
	ct := common.CaptureTemplate{Name: "Quick", Type: "entry"}
	f := &form{tpl: ct, fields: buildFields(ct, true), level: 1}
	setField(f.fields, "HEADLINE", "Think about it")
	setField(f.fields, "CONTENT", "later")
	f.at = 0
	f.cur = 0
	got := strings.ReplaceAll(f.entry(), caretMark, "")
	if got != "* Think about it\n  later" {
		t.Errorf("got %q", got)
	}
}

// A capture that is not a heading has no heading: an item goes into somebody
// else's, so there are no stars and no indent to put it under.
func TestEntryWithNoHeadline(t *testing.T) {
	ct := common.CaptureTemplate{Name: "Item", Type: "item", Template: "a note about {{thing}}"}
	f := &form{tpl: ct, fields: buildFields(ct, false), level: 1}
	f.at = 0
	f.cur = 0
	got := strings.ReplaceAll(f.entry(), caretMark, "")
	if got != "a note about " {
		t.Errorf("got %q", got)
	}
	if n := len(f.fields); n != 1 {
		t.Errorf("an item should ask for one thing, got %d", n)
	}
}

// The caret is drawn by splitting the span it landed in, so the character under
// it comes out in reverse video *inside* the coloured org rather than beside it.
func TestWithCaretReversesOneCharacter(t *testing.T) {
	st := &orghl.State{}
	spans := orghl.Line(":CUSTOM_ID: ab"+caretMark+"cd", st)
	got := withCaret(spans)
	if strings.Contains(got, caretMark) {
		t.Errorf("the sentinel reached the screen: %q", got)
	}
	if !strings.Contains(got, "[::r]c[::-]") {
		t.Errorf("the character under the caret is not reversed: %q", got)
	}
	// Everything either side of it survives, and the colour of the key with it.
	if !strings.Contains(got, ":CUSTOM_ID:") || !strings.Contains(got, "ab") || !strings.Contains(got, "d") {
		t.Errorf("text was lost around the caret: %q", got)
	}
	// At the end of a field there is no character to reverse, so it is a block.
	got = withCaret(orghl.Line("abc"+caretMark, st))
	if !strings.Contains(got, "[::r] [::-]") {
		t.Errorf("a caret at the end should be a block: %q", got)
	}
}

// Tags are taken however somebody wrote them, because somebody typing into a
// one-line box types spaces, or commas, or org's own colons.
func TestTagWords(t *testing.T) {
	for _, in := range []string{"errand shop", "errand,shop", "errand, shop", ":errand:shop:"} {
		got := tagWords(in)
		if len(got) != 2 || got[0] != "errand" || got[1] != "shop" {
			t.Errorf("%q came back as %v", in, got)
		}
	}
	if tagWords("  ") != nil {
		t.Errorf("nothing should be no tags, not one empty one")
	}
}

func TestTargetText(t *testing.T) {
	cases := []struct {
		t    common.CaptureTemplate
		want string
	}{
		{common.CaptureTemplate{CapTarget: common.Target{Type: "file+headline", Filename: "/a/b/inbox.org", Id: "Inbox"}}, "inbox.org › Inbox"},
		{common.CaptureTemplate{CapTarget: common.Target{Type: "file+olp", Filename: "notes.org", Id: "One::Two"}}, "notes.org › One › Two"},
		{common.CaptureTemplate{CapTarget: common.Target{Type: "file+datetree", Filename: "journal.org"}}, "journal.org › today"},
		{common.CaptureTemplate{CapTarget: common.Target{Type: "clock"}}, "wherever the clock is running"},
	}
	for _, c := range cases {
		if got := targetText(c.t); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
}

func indexOfField(fields []*field, key string) int {
	for i, f := range fields {
		if strings.EqualFold(f.Key, key) {
			return i
		}
	}
	return 0
}
