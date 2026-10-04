package cols

// The same scenarios as worg/src/columns.test.ts and the quick add tests in
// ganttquick.test.ts: the column view in the terminal and in the browser are
// one view, and these are the parts of it that can be wrong without looking it.

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func crow(hash string, level int, own string, n float64, kids bool) Row {
	return Row{Hash: hash, Headline: hash, Level: level, HasChildren: kids,
		Cells: []Cell{{Value: own, Own: own, Number: n}}}
}

func outline() []Row {
	return []Row{
		crow("P", 1, "1:00", 60, true),
		crow("A", 2, "", 0, true),
		crow("a1", 3, "2:00", 120, false),
		crow("a2", 3, "0:30", 30, false),
		crow("B", 2, "1:00", 60, false),
		crow("Q", 1, "", 0, true),
		crow("q", 2, "", 0, false),
	}
}

func keys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestFolding(t *testing.T) {
	rows := outline()
	if len(HiddenRows(rows, map[string]bool{})) != 0 {
		t.Error("nothing folded hides nothing")
	}
	if got := keys(HiddenRows(rows, map[string]bool{"A": true})); !reflect.DeepEqual(got, []string{"a1", "a2"}) {
		t.Errorf("folding A hid %v", got)
	}
	if got := keys(HiddenRows(rows, map[string]bool{"P": true, "A": true})); !reflect.DeepEqual(got, []string{"A", "B", "a1", "a2"}) {
		t.Errorf("a fold inside a fold: %v", got)
	}
}

func TestSortKeepsTheOutline(t *testing.T) {
	order := func(rs []Row) []string {
		out := []string{}
		for _, r := range rs {
			out = append(out, r.Hash)
		}
		return out
	}
	got := order(SortWithinParents(outline(), 0, false, true))
	// A's value is empty so it goes after B; Q has none either, so after P.
	if !reflect.DeepEqual(got, []string{"P", "B", "A", "a2", "a1", "Q", "q"}) {
		t.Errorf("ascending %v", got)
	}
	got = order(SortWithinParents(outline(), 0, true, true))
	if !reflect.DeepEqual(got, []string{"P", "B", "A", "a1", "a2", "Q", "q"}) {
		t.Errorf("descending reverses siblings, empties still last: %v", got)
	}
}

func TestTheLine(t *testing.T) {
	if (OptionsOf("%ITEM %TODO") != Options{}) {
		t.Error("no options")
	}
	if got := OptionsOf("%ITEM +path %TODO +flat"); got != (Options{Flat: true, Path: 2}) {
		t.Errorf("options %+v", got)
	}
	if got := OptionsOf("%ITEM +path=3"); got.Path != 3 {
		t.Errorf("path=3 %+v", got)
	}
	cases := [][2]string{
		{SetOption("%ITEM %TODO", "path", 2), "%ITEM %TODO +path"},
		{SetOption("%ITEM +path %TODO", "path", 1), "%ITEM %TODO +path=1"},
		{SetOption("%ITEM +path=3 %TODO", "path", 0), "%ITEM %TODO"},
		{SetOption("%ITEM", "flat", 1), "%ITEM +flat"},
	}
	line := "%25ITEM %TODO %EFFORT(Estimate){:} +path"
	cases = append(cases,
		[2]string{MoveColumn(line, 2, 0), "%EFFORT(Estimate){:} %25ITEM %TODO +path"},
		[2]string{MoveColumn(line, 0, 2), "%TODO %EFFORT(Estimate){:} %25ITEM +path"},
		[2]string{MoveColumn(line, 1, 1), line},
		[2]string{SetWidth("%ITEM %EFFORT{:}", 1, 12), "%ITEM %12EFFORT{:}"},
		[2]string{SetWidth("%25ITEM %TODO", 0, 40), "%40ITEM %TODO"},
		[2]string{SetWidth("%25ITEM %TODO", 0, 0), "%ITEM %TODO"},
	)
	for i, c := range cases {
		if c[0] != c[1] {
			t.Errorf("case %d: %q, want %q", i, c[0], c[1])
		}
	}
	props := []string{}
	for _, tok := range Tokens(MoveColumn(line, 2, 0)) {
		props = append(props, tok.Prop)
	}
	if !reflect.DeepEqual(props, []string{"EFFORT", "ITEM", "TODO"}) {
		t.Errorf("tokens %v", props)
	}
}

func TestOutlineHelpers(t *testing.T) {
	rows := outline()
	if got := AncestorsOf(rows)["a2"]; !reflect.DeepEqual(got, []string{"P", "A"}) {
		t.Errorf("ancestors %v", got)
	}
	if got := keys(RowsWithValue(rows, 0, "0:30")); !reflect.DeepEqual(got, []string{"A", "P", "a2"}) {
		t.Errorf("value filter keeps ancestors: %v", got)
	}
	tot := SubtreeTotals(rows, 0)
	if tot["A"] != 150 || tot["P"] != 270 {
		t.Errorf("totals %v", tot)
	}
	if _, ok := tot["Q"]; ok {
		t.Error("nothing under Q has a value")
	}
	if HoursMinutes(270) != "4:30" || HoursMinutes(4320) != "72:00" {
		t.Error("hours")
	}
}

func TestComplete(t *testing.T) {
	props := []common.ColumnPropValues{{Name: "OWNER", Count: 3}, {Name: "EFFORT", Count: 5}}
	vals := func(s []Suggestion) []string {
		out := []string{}
		for _, x := range s {
			out = append(out, x.Value)
		}
		return out
	}
	from, s := Complete("%ITEM %OW", props)
	if !reflect.DeepEqual(vals(s), []string{"OWNER"}) || "%ITEM %OW"[:from]+"OWNER" != "%ITEM %OWNER" {
		t.Errorf("OW: %d %v", from, vals(s))
	}
	from, s = Complete("%ITEM %25T", props)
	if !reflect.DeepEqual(vals(s), []string{"TODO", "TAGS"}) || from != 9 {
		t.Errorf("T: %d %v", from, vals(s))
	}
	if _, s = Complete("%EFFORT{", props); len(s) == 0 || s[0].Value != ":" {
		t.Errorf("operators %v", vals(s))
	}
	if _, s = Complete("%EFFORT{:m", props); !reflect.DeepEqual(vals(s), []string{":min", ":max", ":mean"}) {
		t.Errorf(":m %v", vals(s))
	}
	if _, s = Complete("%ITEM +p", props); len(s) == 0 || s[0].Value != "+path" {
		t.Errorf("+p %v", vals(s))
	}
	if _, s = Complete("%EFFORT(Est", props); len(s) != 0 {
		t.Errorf("inside a title %v", vals(s))
	}
	if _, s = Complete("%ITEM ", props); len(s) != 0 {
		t.Errorf("after a space %v", vals(s))
	}
}

func TestQuickAdd(t *testing.T) {
	now := time.Date(2026, 1, 8, 0, 0, 0, 0, time.Local) // a Thursday
	q := ParseQuick("NEXT Write the spec 3d @Ian_Davids #design ^fri >kick_off !a RISK=high", []string{"TODO", "NEXT"}, now)
	want := Quick{Headline: "Write the spec", Status: "NEXT", Tags: []string{"design"}, Person: "Ian Davids",
		Effort: "3d", Start: "2026-01-09", After: "kick off", Priority: "A", Props: map[string]string{"RISK": "high"}}
	if !reflect.DeepEqual(q, want) {
		t.Errorf("got  %+v\nwant %+v", q, want)
	}
	q = ParseQuick(`Fix @"Sam Smith" ^soon email`, nil, now)
	if q.Person != "Sam Smith" || q.Headline != "Fix ^soon email" {
		t.Errorf("quoted and not-a-date: %+v", q)
	}
}
