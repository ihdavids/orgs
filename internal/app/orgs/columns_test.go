package orgs

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"

	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// The #+COLUMNS: line
// ---------------------------------------------------------------------------

func TestParsingAColumnSpec(t *testing.T) {
	spec := ParseColumnSpec("%25ITEM %TODO %3PRIORITY %TAGS %EFFORT(Estimate){:} %CLOCKSUM")
	if len(spec) != 6 {
		t.Fatalf("got %d columns, want 6: %+v", len(spec), spec)
	}
	if spec[0].Property != "ITEM" || spec[0].Width != 25 || spec[0].Kind != "item" {
		t.Errorf("ITEM: %+v", spec[0])
	}
	if spec[2].Width != 3 || spec[2].Kind != "priority" {
		t.Errorf("PRIORITY: %+v", spec[2])
	}
	e := spec[4]
	if e.Property != "EFFORT" || e.Title != "Estimate" || e.Summary != ":" || e.Kind != "property" {
		t.Errorf("EFFORT: %+v", e)
	}
	if e.Numbers != "duration" {
		t.Errorf("EFFORT should be read as a duration, got %q", e.Numbers)
	}
}

// A property with no title of its own is written in capitals in the file and
// reads as shouting in a table header.
func TestAColumnWithNoTitleIsTitled(t *testing.T) {
	for line, want := range map[string]string{
		"%EFFORT":     "Effort",
		"%CLOCKSUM_T": "Clocksum T",
		"%CUSTOM_ID":  "Custom Id",
	} {
		if got := ParseColumnSpec(line)[0].Title; got != want {
			t.Errorf("%s: got %q want %q", line, got, want)
		}
	}
}

// Org's own parser is forgiving and so is this: a stray word must not stop the
// columns that were understood from being drawn.
func TestStrayTextIsIgnored(t *testing.T) {
	spec := ParseColumnSpec("oops %ITEM and %EFFORT{:} !!")
	if len(spec) != 2 || spec[0].Property != "ITEM" || spec[1].Property != "EFFORT" {
		t.Errorf("got %+v", spec)
	}
}

// An ordinary property is editable; the worked-out ones are not, and saying so
// on the spec is what keeps the client from needing a table of property names.
func TestWhichColumnsCanBeEdited(t *testing.T) {
	for line, want := range map[string]string{
		"%EFFORT":   "property",
		"%POINTS":   "property",
		"%ITEM":     "item",
		"%TODO":     "todo",
		"%TAGS":     "tags",
		"%CLOCKSUM": "derived",
		"%ALLTAGS":  "derived",
		"%FILE":     "derived",
	} {
		if got := ParseColumnSpec(line)[0].Kind; got != want {
			t.Errorf("%s: got %q want %q", line, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Summary operators
// ---------------------------------------------------------------------------

func TestWhichOperatorsRollUp(t *testing.T) {
	for op, want := range map[string]string{
		"+": "sum", "$": "sum", ":": "sum",
		"min": "min", ":min": "min",
		"max": "max", ":max": "max",
		"mean": "mean", ":mean": "mean",
		"+;%.1f": "sum",
		// Not implemented, and a column carrying one must simply not roll up
		// rather than roll up wrongly.
		"X": "", "X/": "", "X%": "", "@min": "", "": "",
	} {
		if got := summaryOp(op); got != want {
			t.Errorf("%q: got %q want %q", op, got, want)
		}
	}
}

// `%EFFORT` and `%EFFORT{:}` are different things. The operator is how a file
// says it wants a total, and inventing one for a column that did not ask would
// make orgs disagree with org about what the file means.
func TestAColumnWithNoOperatorDoesNotRollUp(t *testing.T) {
	spec := ParseColumnSpec("%ITEM %EFFORT")
	root := leaf("Project", "")
	root.children = []*columnNode{leaf("a", "2h"), leaf("b", "3h")}
	fill(root, spec)
	summarise(root, spec)
	if root.row.Cells[1].Value != "" {
		t.Errorf("rolled up without being asked: %q", root.row.Cells[1].Value)
	}
	spec = ParseColumnSpec("%ITEM %EFFORT{:}")
	root = leaf("Project", "")
	root.children = []*columnNode{leaf("a", "2h"), leaf("b", "3h")}
	fill(root, spec)
	summarise(root, spec)
	if root.row.Cells[1].Value != "5:00" {
		t.Errorf("got %q want 5:00", root.row.Cells[1].Value)
	}
}

// ---------------------------------------------------------------------------
// The rollup
// ---------------------------------------------------------------------------

// Effort is written in whatever units suit the task. Adding it up means reading
// all of them the same way, which is what makes the total worth anything.
func TestEffortIsSummedAcrossUnits(t *testing.T) {
	spec := ParseColumnSpec("%ITEM %EFFORT{:}")
	root := leaf("Project", "")
	root.children = []*columnNode{leaf("a", "2h"), leaf("b", "30min"), leaf("c", "1:15")}
	fill(root, spec)
	summarise(root, spec)
	if got := root.row.Cells[1].Value; got != "3:45" {
		t.Errorf("got %q want 3:45", got)
	}
}

// The total goes down the tree as well as up: a total is of everything
// underneath, not of the immediate children.
func TestTheRollupIsOfTheWholeSubtree(t *testing.T) {
	spec := ParseColumnSpec("%ITEM %EFFORT{:}")
	root := leaf("Project", "")
	mid := leaf("Phase", "")
	mid.children = []*columnNode{leaf("a", "2h"), leaf("b", "2h")}
	root.children = []*columnNode{mid, leaf("c", "1h")}
	fill(root, spec)
	summarise(root, spec)
	if got := mid.row.Cells[1].Value; got != "4:00" {
		t.Errorf("phase: got %q want 4:00", got)
	}
	if got := root.row.Cells[1].Value; got != "5:00" {
		t.Errorf("project: got %q want 5:00", got)
	}
}

// A parent's own estimate is its own overhead and is counted, not replaced. Of
// the two readings of this, it is the only one that does not quietly throw away
// a number somebody typed.
func TestAParentsOwnValueIsCounted(t *testing.T) {
	spec := ParseColumnSpec("%ITEM %EFFORT{:}")
	root := leaf("Project", "1h")
	root.children = []*columnNode{leaf("a", "2h")}
	fill(root, spec)
	summarise(root, spec)
	if got := root.row.Cells[1].Value; got != "3:00" {
		t.Errorf("got %q want 3:00", got)
	}
}

// The cell has to carry both, because they are different on every parent that
// rolls up - and an editor pre-filled with the total writes the total onto the
// parent the first time anybody opens it.
func TestACellKeepsWhatIsWrittenAsWellAsWhatIsShown(t *testing.T) {
	spec := ParseColumnSpec("%ITEM %EFFORT{:}")
	root := leaf("Project", "")
	root.children = []*columnNode{leaf("a", "2h")}
	fill(root, spec)
	summarise(root, spec)
	cell := root.row.Cells[1]
	if cell.Value != "2:00" {
		t.Errorf("shown: %q", cell.Value)
	}
	if cell.Own != "" {
		t.Errorf("the parent has no effort of its own, but Own says %q", cell.Own)
	}
	if !cell.Summed {
		t.Error("Summed should say the value was worked out")
	}
	// And on a leaf the two agree and nothing was summed.
	leafCell := root.children[0].row.Cells[1]
	if leafCell.Value != "2h" || leafCell.Own != "2h" || leafCell.Summed {
		t.Errorf("leaf: %+v", leafCell)
	}
}

// A heading with nothing under it is left exactly as it wrote itself - `2h`
// stays `2h` rather than being reformatted into `2:00`, because the column is
// showing the file rather than replacing it.
func TestALeafIsNotReformatted(t *testing.T) {
	spec := ParseColumnSpec("%ITEM %EFFORT{:}")
	n := leaf("a", "2h")
	fill(n, spec)
	summarise(n, spec)
	if n.row.Cells[1].Value != "2h" {
		t.Errorf("got %q want 2h", n.row.Cells[1].Value)
	}
}

// Three days of effort is seventy-two hours, not nought.
func TestATotalPastADayDoesNotWrap(t *testing.T) {
	if got := hoursMinutes(3 * 24 * 60); got != "72:00" {
		t.Errorf("got %q want 72:00", got)
	}
	if got := hoursMinutes(0); got != "" {
		t.Errorf("nothing clocked should be blank, got %q", got)
	}
}

// min, max and mean, and a printf format after a semicolon.
func TestTheOtherOperators(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"%ITEM %POINTS{+}", "9"},
		{"%ITEM %POINTS{min}", "2"},
		{"%ITEM %POINTS{max}", "4"},
		{"%ITEM %POINTS{mean}", "3"},
		{"%ITEM %POINTS{+;%.2f}", "9.00"},
	} {
		spec := ParseColumnSpec(c.line)
		root := leaf("Project", "")
		root.children = []*columnNode{leaf("a", "2"), leaf("b", "3"), leaf("c", "4")}
		fill(root, spec)
		summarise(root, spec)
		if got := root.row.Cells[1].Value; got != c.want {
			t.Errorf("%s: got %q want %q", c.line, got, c.want)
		}
	}
}

// A number written with something after it is still a number: `3 pages` is
// three. This is what lets a column count things nobody thought to make tidy.
func TestANumberWithWordsRoundIt(t *testing.T) {
	for _, c := range []struct {
		in   string
		kind string
		want float64
		ok   bool
	}{
		{"3 pages", "number", 3, true},
		{"$4.50", "number", 4.5, true},
		{"2h", "duration", 120, true},
		{"1:30", "duration", 90, true},
		{"", "duration", 0, false},
		{"soon", "number", 0, false},
	} {
		got, ok := cellNumber(c.in, c.kind)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%q as %s: got %v,%v want %v,%v", c.in, c.kind, got, ok, c.want, c.ok)
		}
	}
}

// A file that declares its own line gets exactly that; one that does not gets
// the configured default, and the answer says which so a view can explain why a
// column it expected is missing.
func TestWhereTheColumnsLineCameFrom(t *testing.T) {
	line, from := columnsFor(nil, "%ITEM")
	if line != "%ITEM" || from != "request" {
		t.Errorf("got %q from %q", line, from)
	}
	line, from = columnsFor(nil, "")
	if from != "config" || !strings.Contains(line, "EFFORT") {
		t.Errorf("the default should show effort: %q from %q", line, from)
	}
}

// ---------------------------------------------------------------------------

// A node standing for one heading with one property, for testing the arithmetic
// without a file behind it.
func leaf(name, effort string) *columnNode {
	n := &columnNode{props: map[string]string{}}
	if effort != "" {
		n.props["EFFORT"] = effort
		n.props["POINTS"] = effort
	}
	n.row = common.ColumnRow{Headline: name}
	return n
}

// Fill in each node's own cells, the way buildColumnNodes does, deepest first.
func fill(n *columnNode, spec []common.ColumnSpec) {
	for _, c := range n.children {
		fill(c, spec)
	}
	n.row.Cells = nil
	for _, s := range spec {
		cell := common.ColumnCell{}
		switch s.Property {
		case "ITEM":
			cell.Value = n.row.Headline
		default:
			cell.Value = propOf(n.props, s.Property)
		}
		cell.Own = cell.Value
		if s.Numbers != "" {
			if v, ok := cellNumber(cell.Value, s.Numbers); ok {
				cell.Number = v
			}
		}
		n.row.Cells = append(n.row.Cells, cell)
	}
	n.row.HasChildren = len(n.children) > 0
}

// `%CLOCKSUM` is a subtree total in org whether or not anybody wrote an
// operator - it means "the time spent under here". Showing only the time clocked
// against the parent heading itself reads as almost every project having no time
// on it.
func TestClocksumRollsUpWithoutBeingAsked(t *testing.T) {
	spec := ParseColumnSpec("%ITEM %CLOCKSUM")
	if spec[1].Numbers != "duration" {
		t.Fatalf("CLOCKSUM should be a duration column: %+v", spec[1])
	}
	if got := opFor(spec[1]); got != "sum" {
		t.Errorf("got %q want sum", got)
	}
	// And an ordinary property still needs to be asked.
	if got := opFor(ParseColumnSpec("%EFFORT")[0]); got != "" {
		t.Errorf("EFFORT with no operator should not roll up, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Writing a property
// ---------------------------------------------------------------------------

// A column view is a screen somebody sits in front of filling in estimates. At
// one whole-file rewrite per cell, every edit would show up as a hundred changed
// lines to anybody with the file open in an editor or under git.
//
// The guarantee is that an edit reaches the heading's own property drawer and
// nothing else. Inside that drawer the keys are re-aligned, which is what the
// record editor does and for the same reason - a drawer is something somebody
// opens in an editor - but a table three headings away, a CLOCK line's spacing
// and every other heading's drawer are none of its business.
func TestSettingAPropertyTouchesOnlyItsOwnDrawer(t *testing.T) {
	dir := t.TempDir()
	name := dir + "/p.org"
	before := `#+TITLE: T

* TODO Parent
  :PROPERTIES:
  :EFFORT: 2h
  :END:
** TODO Child
   :PROPERTIES:
   :EFFORT: 1d
   :POINTS: 5
   :END:
   :LOGBOOK:
   CLOCK: [2026-09-20 Sun 09:00]--[2026-09-20 Sun 12:30] =>  3:30
   :END:

| a | b |
|---+---|
| 1 | 2 |
`
	if err := os.WriteFile(name, []byte(before), 0644); err != nil {
		t.Fatal(err)
	}
	lines := fileLines(name)
	// The child heading is at row 6 and is a level 2 heading.
	sec := &org.Section{Headline: &org.Headline{Lvl: 2, Pos: org.Pos{Row: 6}}}
	f := &common.OrgFile{Filename: name}
	if err := setHeadingProperty(f, sec, "EFFORT", "2d"); err != nil {
		t.Fatal(err)
	}
	after := fileLines(name)
	if len(after) != len(lines) {
		t.Fatalf("the line count changed: %d -> %d", len(lines), len(after))
	}
	for i := range lines {
		if lines[i] == after[i] {
			continue
		}
		// The child's own drawer is rows 7..10. Anything else moving is the
		// whole-document rewrite this replaced.
		if i < 7 || i > 10 {
			t.Errorf("line %d is outside the heading being edited: %q -> %q", i, lines[i], after[i])
		}
	}
	// The drawer pads its keys to a common width, so the value is looked for
	// without insisting on how much space is in front of it.
	if !regexp.MustCompile(`:EFFORT:\s+2d`).MatchString(strings.Join(after, "\n")) {
		t.Errorf("the edit did not land:\n%s", strings.Join(after, "\n"))
	}
	// The three things a whole-document rewrite used to mangle.
	body := strings.Join(after, "\n")
	if !strings.Contains(body, "=>  3:30") {
		t.Error("the clock line lost its spacing")
	}
	if !strings.Contains(body, "|---+---|") {
		t.Error("the table was reflowed")
	}
	if !strings.Contains(body, "\n  :EFFORT: 2h\n") {
		t.Error("the parent heading's own drawer was touched")
	}
}

// Setting a property to nothing takes it off - and takes the drawer with it when
// there is nothing left in it, because an empty drawer is litter and undo
// writing back a previous value of "nothing" has to leave the heading as it
// found it.
func TestClearingAProperty(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name    string
		before  string
		gone    string
		wantOut []string
	}{
		{
			"one of several",
			"* TODO A\n  :PROPERTIES:\n  :EFFORT: 2h\n  :POINTS: 5\n  :END:\n",
			"EFFORT",
			[]string{":POINTS:", ":PROPERTIES:"},
		},
		{
			"the last one takes the drawer with it",
			"* TODO A\n  :PROPERTIES:\n  :EFFORT: 2h\n  :END:\nbody\n",
			"EFFORT",
			nil,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			name := dir + "/" + strings.ReplaceAll(c.name, " ", "_") + ".org"
			if err := os.WriteFile(name, []byte(c.before), 0644); err != nil {
				t.Fatal(err)
			}
			sec := &org.Section{Headline: &org.Headline{Lvl: 1, Pos: org.Pos{Row: 0}}}
			if err := setHeadingProperty(&common.OrgFile{Filename: name}, sec, c.gone, ""); err != nil {
				t.Fatal(err)
			}
			body := strings.Join(fileLines(name), "\n")
			if strings.Contains(body, ":"+c.gone+":") {
				t.Errorf("%s is still there:\n%s", c.gone, body)
			}
			for _, want := range c.wantOut {
				if !strings.Contains(body, want) {
					t.Errorf("%s went too:\n%s", want, body)
				}
			}
			if c.wantOut == nil && strings.Contains(body, "PROPERTIES") {
				t.Errorf("the empty drawer was left behind:\n%s", body)
			}
		})
	}
}
