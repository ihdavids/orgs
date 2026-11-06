package orgs

import (
	"reflect"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func saNode(title string, set func(n *common.AgendaNode)) common.AgendaNode {
	n := common.AgendaNode{}
	n.Headline = title
	n.Hash = "h-" + title
	n.Olp = []string{"Projects", title}
	n.Filename = "/notes/work.org"
	if set != nil {
		set(&n)
	}
	return n
}

func saDate(day string) *common.AgendaDate { return &common.AgendaDate{Start: day + "T00:00"} }

// The names of the groups and the titles in each, for comparing.
func saShape(nodes []common.AgendaNode, groups []common.AgendaGroupItems) map[string][]string {
	out := map[string][]string{}
	for _, g := range groups {
		for _, i := range g.Items {
			out[g.Name] = append(out[g.Name], nodes[i].Headline)
		}
		if len(g.Items) == 0 {
			out[g.Name] = []string{}
		}
	}
	return out
}

func saNames(groups []common.AgendaGroupItems) []string {
	out := []string{}
	for _, g := range groups {
		out = append(out, g.Name)
	}
	return out
}

func saCtx() *superAgendaCtx {
	return &superAgendaCtx{
		Today:    "2026-11-05",
		Category: func(n *common.AgendaNode) string { return n.Props["CATEGORY"] },
		Query: func(q string) (map[string]bool, error) {
			if q == "IsProject()" {
				return map[string]bool{"h-Garden": true}, nil
			}
			return map[string]bool{}, nil
		},
	}
}

func TestSuperAgendaGroups(t *testing.T) {
	nodes := []common.AgendaNode{
		saNode("Report", func(n *common.AgendaNode) { n.Deadline = saDate("2026-11-01"); n.Priority = "A" }),
		saNode("Call Ann", func(n *common.AgendaNode) { n.Status = "WAITING"; n.IsTodo = true }),
		saNode("Garden", func(n *common.AgendaNode) { n.Children.Count = 3; n.Children.Todo = 1 }),
		saNode("Tax", func(n *common.AgendaNode) { n.Priority = "A"; n.Props = map[string]string{"Effort": "0:10"} }),
		saNode("Read", func(n *common.AgendaNode) { n.AllTags = []string{"someday"} }),
		saNode("Gym", func(n *common.AgendaNode) { n.IsHabit = true }),
		saNode("Shop", nil),
	}
	items := make([]superAgendaItem, len(nodes))
	for i := range nodes {
		items[i] = superAgendaItem{Node: &nodes[i]}
	}
	groups := []common.AgendaGroup{
		{Name: "Overdue", Deadline: "past", Scheduled: "past"},
		{Name: "Quick", EffortMax: "0:15"},
		{Name: "Important", Priority: "A"},
		{Name: "Waiting", Todo: []string{"WAITING"}, Order: 9},
		{Discard: true, Tag: []string{"someday"}},
		{Name: "Projects", Query: "IsProject()"},
		{Name: "Empty", Heading: "^nothing$"},
		{Habit: true},
	}
	c := saCtx()
	got := groupAgendaItems(groups, items, c)
	if c.problems() != "" {
		t.Fatal(c.problems())
	}
	// First match wins (Tax is quick before it is important); Read is
	// discarded; empty groups are not shown; Other items come last but
	// before nothing, and Waiting's order puts it after the zeros.
	wantNames := []string{"Overdue", "Quick", "Projects", "Habits", "Waiting", "Other items"}
	if !reflect.DeepEqual(saNames(got), wantNames) {
		t.Fatalf("groups %v, want %v", saNames(got), wantNames)
	}
	want := map[string][]string{
		"Overdue": {"Report"}, "Quick": {"Tax"}, "Projects": {"Garden"},
		"Habits": {"Gym"}, "Waiting": {"Call Ann"}, "Other items": {"Shop"},
	}
	if s := saShape(nodes, got); !reflect.DeepEqual(s, want) {
		t.Fatalf("got %v\nwant %v", s, want)
	}
}

func TestSuperAgendaAutoAndLogic(t *testing.T) {
	nodes := []common.AgendaNode{
		saNode("a", func(n *common.AgendaNode) { n.Props = map[string]string{"CATEGORY": "work"}; n.Status = "TODO"; n.Tags = []string{"x"} }),
		saNode("b", func(n *common.AgendaNode) { n.Props = map[string]string{"CATEGORY": "home"}; n.Status = "TODO" }),
		saNode("c", func(n *common.AgendaNode) { n.Props = map[string]string{"CATEGORY": "Admin"} }),
		saNode("d", func(n *common.AgendaNode) { n.Props = map[string]string{} }),
	}
	items := make([]superAgendaItem, len(nodes))
	for i := range nodes {
		items[i] = superAgendaItem{Node: &nodes[i]}
	}
	groups := []common.AgendaGroup{
		// TODO and tagged x, together.
		{Name: "Both", And: []common.AgendaGroup{{Todo: []string{"TODO"}}, {Tag: []string{"x"}}}},
		// Then one group per category, sorted without regard to case; d has
		// none and goes on.
		{Auto: "category"},
	}
	got := groupAgendaItems(groups, items, saCtx())
	if names := saNames(got); !reflect.DeepEqual(names, []string{"Both", "Admin", "home", "Other items"}) {
		t.Fatalf("groups %v", names)
	}
	notGroups := []common.AgendaGroup{{Name: "Not todo", Not: []common.AgendaGroup{{Todo: []string{"t"}}}}}
	nodes[0].IsTodo, nodes[1].IsTodo = true, true
	got = groupAgendaItems(notGroups, items, saCtx())
	if s := saShape(nodes, got); !reflect.DeepEqual(s["Not todo"], []string{"c", "d"}) {
		t.Fatalf("not: %v", s)
	}
}

func TestSuperAgendaSelectors(t *testing.T) {
	c := saCtx()
	cases := []struct {
		g    common.AgendaGroup
		set  func(n *common.AgendaNode)
		e    *common.AgendaEntry
		want bool
	}{
		{common.AgendaGroup{Priority: ">B"}, func(n *common.AgendaNode) { n.Priority = "A" }, nil, true},
		{common.AgendaGroup{Priority: ">B"}, nil, nil, false}, // no cookie is B
		{common.AgendaGroup{Priority: "<B"}, func(n *common.AgendaNode) { n.Priority = "C" }, nil, true},
		{common.AgendaGroup{Priority: "A,C"}, func(n *common.AgendaNode) { n.Priority = "C" }, nil, true},
		{common.AgendaGroup{Deadline: "today"}, func(n *common.AgendaNode) { n.Deadline = saDate("2026-11-05") }, nil, true},
		{common.AgendaGroup{Deadline: "future"}, func(n *common.AgendaNode) { n.Deadline = saDate("2026-11-05") }, nil, false},
		{common.AgendaGroup{Scheduled: "none"}, nil, nil, true},
		{common.AgendaGroup{Date: "2026-11-07"}, func(n *common.AgendaNode) { n.Timestamps = []common.AgendaDate{*saDate("2026-11-07")} }, nil, true},
		{common.AgendaGroup{TimeGrid: true}, nil, &common.AgendaEntry{HasTime: true}, true},
		{common.AgendaGroup{TimeGrid: true}, nil, nil, false},
		{common.AgendaGroup{Property: []string{"owner=ian"}}, func(n *common.AgendaNode) { n.Props = map[string]string{"OWNER": "ian"} }, nil, true},
		{common.AgendaGroup{Property: []string{"OWNER"}}, nil, nil, false},
		{common.AgendaGroup{EffortMin: "1:00"}, func(n *common.AgendaNode) { n.Props = map[string]string{"EFFORT": "2:00"} }, nil, true},
		{common.AgendaGroup{EffortMin: "1:00"}, nil, nil, false}, // no effort is not a match either way
		{common.AgendaGroup{Children: "none"}, nil, nil, true},
		{common.AgendaGroup{File: `work\.org$`}, nil, nil, true},
		{common.AgendaGroup{Regexp: "urgent"}, func(n *common.AgendaNode) { n.Props = map[string]string{"NOTE": "urgent"} }, nil, true},
		{common.AgendaGroup{Name: "only a name"}, nil, nil, false},
	}
	for i, tc := range cases {
		n := saNode("x", tc.set)
		if got := c.matches(tc.g, superAgendaItem{Node: &n, Entry: tc.e}); got != tc.want {
			t.Errorf("case %d %+v: got %v", i, tc.g, got)
		}
	}
	bad := saCtx()
	n := saNode("x", nil)
	bad.matches(common.AgendaGroup{Heading: "("}, superAgendaItem{Node: &n})
	bad.autoValue("colour", superAgendaItem{Node: &n})
	if bad.problems() == "" {
		t.Fatal("a bad regexp and an unknown auto were not reported")
	}
}

func TestSuperAgendaText(t *testing.T) {
	r := common.AgendaViewResult{Parts: []common.AgendaViewPart{{
		Title: "Open tasks",
		Nodes: []common.AgendaNode{saNode("one", nil), saNode("two", nil), saNode("three", nil)},
		Groups: []common.AgendaGroupItems{
			{Name: "First", Items: []int{1}},
			{Name: "Other items", Items: []int{0}}, // three was discarded
		},
	}}}
	order := agendaTextOrder(3, r.Parts[0].Groups)
	want := []agendaTextAt{{group: "First"}, {i: 1}, {group: "Other items"}, {i: 0}}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order %v", order)
	}
}
