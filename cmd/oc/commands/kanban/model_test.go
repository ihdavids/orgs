package kanban

// The board's arithmetic, pinned against the same scenarios as
// worg/src/kanban.test.ts: a card in the wrong column still looks like a card,
// and an order value a hair out looks right until the next refresh.

import (
	"reflect"
	"testing"
)

func card(hash, status string, props map[string]string, tags ...string) Card {
	return Card{Hash: hash, Headline: hash, Status: status, Props: props, Tags: tags}
}

func f(v float64) *float64 { return &v }

// The colour a value gets has to be the colour the browser gives it. These are
// what worg's autoSwatch answers (checked with node), including the strings
// whose hash wraps past 32 bits and the ones outside the BMP.
func TestAutoSwatchMatchesTheBrowser(t *testing.T) {
	want := map[string]string{
		"work": "violet", "Acme Corp": "pink", "urgent": "violet",
		"héllo wörld": "amber",
		"a-very-long-tag-name-that-overflows-32-bits-many-times": "orange",
		"😀 emoji": "teal",
		"":        "slate",
	}
	for in, key := range want {
		if got := AutoSwatch(in).Key; got != key {
			t.Errorf("AutoSwatch(%q) = %s, want %s", in, got, key)
		}
	}
}

func TestColumnOf(t *testing.T) {
	b := NewBoard("b")
	b.GroupBy, b.GroupKey = "property", "STAGE"
	cols := []Column{{Value: "Doing"}, {Value: "Done"}}
	c := card("a", "", map[string]string{"STAGE": " doing "})
	if got := ColumnOf(&c, &b, cols); got != "Doing" {
		t.Errorf("property column %q", got)
	}

	b.GroupBy = "tag"
	tagCols := []Column{{Value: "urgent"}, {Value: "work"}}
	c = card("a", "", nil, "work", "urgent")
	if got := ColumnOf(&c, &b, tagCols); got != "urgent" {
		t.Errorf("first tag column that asks should win, got %q", got)
	}
	c = card("a", "", nil, "home")
	if got := ColumnOf(&c, &b, tagCols); got != Unset {
		t.Errorf("unclaimed card landed in %q", got)
	}
}

func TestOrder(t *testing.T) {
	b := NewBoard("b")
	cards := []Card{
		card("none", "", nil),
		card("two", "", map[string]string{"KANBAN": "2"}),
		card("one", "", map[string]string{"KANBAN": "1"}),
	}
	got := []string{}
	for _, c := range SortCards(cards, &b) {
		got = append(got, c.Hash)
	}
	if !reflect.DeepEqual(got, []string{"one", "two", "none"}) {
		t.Errorf("manual order %v", got)
	}

	b.Sort = "priority"
	pri := []Card{{Hash: "x", Headline: "x"}, {Hash: "b", Headline: "b", Priority: "B"}, {Hash: "a", Headline: "a", Priority: "A"}}
	got = got[:0]
	for _, c := range SortCards(pri, &b) {
		got = append(got, c.Hash)
	}
	if !reflect.DeepEqual(got, []string{"a", "b", "x"}) {
		t.Errorf("priority order %v", got)
	}
}

func TestOrderBetween(t *testing.T) {
	if v, ok := OrderBetween(f(1), f(2)); !ok || v != 1.5 {
		t.Errorf("midpoint %v", v)
	}
	if v, _ := OrderBetween(nil, f(5)); v != 4 {
		t.Errorf("before the first %v", v)
	}
	if v, _ := OrderBetween(f(5), nil); v != 6 {
		t.Errorf("after the last %v", v)
	}
	if v, _ := OrderBetween(nil, nil); v != 0 {
		t.Errorf("empty column %v", v)
	}
	a := 1.0
	b := a + 2.220446049250313e-16
	if _, ok := OrderBetween(&a, &b); ok {
		t.Error("a gap with no room should ask for a renumber")
	}
	if FormatOrder(1.5) != "1.5" || FormatOrder(3) != "3" || FormatOrder(1.0/3) != "0.333333" {
		t.Errorf("format %s %s %s", FormatOrder(1.5), FormatOrder(3), FormatOrder(1.0/3))
	}
	if !reflect.DeepEqual(Renumber(3), []float64{0, 100, 200}) {
		t.Error("renumber")
	}
}

func TestPlanOrder(t *testing.T) {
	b := NewBoard("b")
	list := []Card{
		card("a", "", map[string]string{"KANBAN": "0"}),
		card("b", "", map[string]string{"KANBAN": "100"}),
	}
	m := card("m", "", nil)
	if w := PlanOrder(list, 1, &m, &b); len(w) != 1 || w[0].Value != 50 {
		t.Errorf("between: %v", w)
	}
	// A card above the drop with no number of its own: renumber the column.
	list[0].Props = nil
	w := PlanOrder(list, 1, &m, &b)
	if len(w) != 3 {
		t.Errorf("renumber: %v", w)
	}
}

func TestDefaultStatusColumns(t *testing.T) {
	cols := DefaultStatusColumns([]string{"TODO", "IN-PROGRESS"}, []string{"DONE"}, []string{"INPROGRESS", "WAITING"})
	vals := []string{}
	for _, c := range cols {
		vals = append(vals, c.Value)
	}
	if !reflect.DeepEqual(vals, []string{"TODO", "IN-PROGRESS", "WAITING", "DONE"}) {
		t.Errorf("columns %v", vals)
	}
	if !reflect.DeepEqual(cols[1].Aliases, []string{"INPROGRESS"}) {
		t.Errorf("aliases %v", cols[1].Aliases)
	}
	if cols[3].Color != "green" || cols[0].Color != "blue" {
		t.Errorf("colours %s %s", cols[0].Color, cols[3].Color)
	}
	b := NewBoard("b")
	c := card("x", "inprogress", nil)
	if got := ColumnOf(&c, &b, cols); got != "IN-PROGRESS" {
		t.Errorf("alias landed in %q", got)
	}
}

func TestHeaderColour(t *testing.T) {
	b := NewBoard("b")
	c := card("a", "", map[string]string{"COLOR": "teal"})
	if HeaderColorOf(&c, &b, true) != "#12a594" {
		t.Error("COLOR teal")
	}
	c = card("a", "", map[string]string{"COLOUR": "purple"})
	if HeaderColorOf(&c, &b, true) != "#8e4ec6" {
		t.Error("purple is violet")
	}
	c = card("a", "", nil)
	if HeaderColorOf(&c, &b, true) != HeaderNoneDark || HeaderColorOf(&c, &b, false) != HeaderNoneLight {
		t.Error("nothing said")
	}
	b.HeaderKey = "CLIENT"
	b.HeaderColors = map[string]string{"Acme": "blue"}
	c = card("a", "", map[string]string{"CLIENT": "Acme", "COLOR": "red"})
	if HeaderColorOf(&c, &b, true) != "#3e63dd" {
		t.Error("the board's mapping wins, and only its key is read")
	}
	if ColorFromWord("#abc") != "#aabbcc" {
		t.Error("short hex")
	}
}

func TestLabels(t *testing.T) {
	b := NewBoard("b")
	b.LabelSource = "property"
	b.LabelKey = "labels"
	b.LabelOrder = []string{"bug", "ux"}
	c := card("a", "", map[string]string{"LABELS": "ux, perf bug"})
	if got := LabelsOf(&c, &b); !reflect.DeepEqual(got, []string{"bug", "ux", "perf"}) {
		t.Errorf("labels %v", got)
	}
}

func TestQueries(t *testing.T) {
	stored := []StoredQuery{{Name: "mine", Query: "IsTodo()"}}
	b := NewBoard("b")
	b.Query = "IsProject()"
	b.StoredQuery = "mine"
	if got := BoardQuery(&b, stored); got != "!IsArchived() && (IsTodo())" {
		t.Errorf("query %q", got)
	}
	b.IncludeArchived = true
	if got := BoardQuery(&b, stored); got != "IsTodo()" {
		t.Errorf("archived %q", got)
	}
	b.StoredQuery = "gone"
	if BoardQueryProblem(&b, stored) == "" {
		t.Error("a missing saved query is a problem")
	}
	b.StoredQuery, b.Query = "", " "
	if BoardQueryProblem(&b, stored) == "" {
		t.Error("an empty query is a problem")
	}
}

func TestSearchAndNames(t *testing.T) {
	c := card("Write the spec", "TODO", map[string]string{"OWNER": "ian"}, "draft")
	if !CardMatches(&c, SearchTerms("spec DRAFT ian")) || CardMatches(&c, SearchTerms("spec bob")) {
		t.Error("every word has to be found somewhere")
	}
	if UniqueName("Board", []string{"Board", "Board 2"}) != "Board 3" {
		t.Error("unique name")
	}
}

func TestNormalizeReadsOldFolds(t *testing.T) {
	b := Board{Columns: []Column{{Value: "A", Collapsed: true}, {Value: "B"}}}
	Normalize(&b)
	if !reflect.DeepEqual(b.Folded, []string{"A"}) || b.Sort != "manual" || b.Layout != "board" {
		t.Errorf("normalized %+v", b)
	}
}
