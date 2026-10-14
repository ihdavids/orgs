package orgs

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestExtractDepPhrases(t *testing.T) {
	cases := []struct {
		line    string
		verb    string
		reverse bool
		keys    []string
		links   []string
		text    string
	}{
		{"This is blocked by ABC-12 and ABC-13.", "blocked by", false, []string{"ABC-12", "ABC-13"}, nil, "ABC-12 and ABC-13"},
		{"Depends on [[file:x.org::*Database migration][the migration]]. Then more.", "depends on", false, nil, []string{"file:x.org::*Database migration"}, "the migration"},
		{"waiting for the signup form, after lunch", "waiting for", false, nil, nil, "the signup form"},
		{"This blocks the release notes", "blocks", true, nil, nil, "the release notes"},
		{"Prerequisite for [[#WEB-9]]", "prerequisite for", true, nil, []string{"#WEB-9"}, "#WEB-9"},
		{"Prerequisites: design review", "prerequisites:", false, nil, nil, "design review"},
		{"dependent on login page (see ticket)", "dependent on", false, nil, nil, "login page"},
	}
	for _, c := range cases {
		ps := extractDepPhrases(c.line)
		if len(ps) == 0 {
			t.Fatalf("%q: no phrase found", c.line)
		}
		p := ps[0]
		if p.Verb != c.verb || p.Reverse != c.reverse {
			t.Errorf("%q: verb %q reverse %v, want %q %v", c.line, p.Verb, p.Reverse, c.verb, c.reverse)
		}
		if len(c.keys) > 0 && !reflect.DeepEqual(p.Keys, c.keys) {
			t.Errorf("%q: keys %v, want %v", c.line, p.Keys, c.keys)
		}
		got := []string{}
		for _, l := range p.Links {
			got = append(got, l.Target)
		}
		if len(c.links) > 0 && !reflect.DeepEqual(got, c.links) {
			t.Errorf("%q: links %v, want %v", c.line, got, c.links)
		}
		if p.Text != c.text {
			t.Errorf("%q: text %q, want %q", c.line, p.Text, c.text)
		}
	}
	// Two phrases on one line each get their own clause.
	ps := extractDepPhrases("Needs WEB-1 but blocks WEB-2")
	if len(ps) != 2 || !reflect.DeepEqual(ps[0].Keys, []string{"WEB-1"}) || !ps[1].Reverse || !reflect.DeepEqual(ps[1].Keys, []string{"WEB-2"}) {
		t.Errorf("two phrases: %+v", ps)
	}
	// Words that only contain a verb are not one.
	if ps := extractDepPhrases("Thereafter we unblocked the afternoon"); len(ps) != 0 {
		t.Errorf("no phrase wanted, got %+v", ps)
	}
	// A dot inside a link does not end the clause.
	ps = extractDepPhrases("after [[file:notes.org::*Set up CI]] is green")
	if len(ps) != 1 || len(ps[0].Links) != 1 {
		t.Errorf("link with a dot: %+v", ps)
	}
	long := "Blocked by WEB-3 " + strings.Repeat("and a great deal of other words ", 10)
	if ev := extractDepPhrases(long)[0].Evidence; len([]rune(ev)) > 121 || !strings.HasPrefix(ev, "Blocked by WEB-3") {
		t.Errorf("evidence not trimmed from the phrase: %q", ev)
	}
}

func sgTestIndex() (*sgIndex, map[string]*sgTask) {
	mk := func(hash, title string, props map[string]string, tags ...string) *sgTask {
		if props == nil {
			props = map[string]string{}
		}
		t := &sgTask{Hash: hash, Title: title, File: "/n/proj.org", Props: props, Tags: tags, Open: true}
		t.ID = props["ID"]
		t.CustomID = props["CUSTOM_ID"]
		return t
	}
	ts := []*sgTask{
		mk("h1", "Login page", map[string]string{"CUSTOM_ID": "WEB-12", "PROJECTKEY": "WEB"}, "JIRA"),
		mk("h2", "Database migration", map[string]string{"ID": "dbmig"}),
		mk("h3", "OPS-7 Rotate the certificates", nil),
		mk("h4", "Review the pull request", nil),
		mk("h5", "Deploy", map[string]string{"LINK": "[[https://x.atlassian.net/browse/INF-4][INF-4]]"}),
	}
	by := map[string]*sgTask{}
	for _, t := range ts {
		by[t.Title] = t
	}
	return newSgIndex(ts), by
}

func TestTicketKeyMatching(t *testing.T) {
	idx, by := sgTestIndex()
	if got := ticketKeys("see ABC-1, abc-2, X-3 and LONGPROJ1-42 and ELEVENCHARS-1"); !reflect.DeepEqual(got, []string{"ABC-1", "LONGPROJ1-42"}) {
		t.Errorf("ticketKeys: %v", got)
	}
	if idx.byKey["WEB-12"] != by["Login page"] || !idx.keyJira["WEB-12"] {
		t.Errorf("CUSTOM_ID of a jira task should be its jira key")
	}
	if idx.byKey["OPS-7"] != by["OPS-7 Rotate the certificates"] || idx.keyJira["OPS-7"] {
		t.Errorf("a key in a headline is a plain key")
	}
	if idx.byKey["INF-4"] != by["Deploy"] {
		t.Errorf("a key in LINK names its task")
	}
	self := by["Database migration"]
	cands := idx.phraseCands(self, extractDepPhrases("Blocked by WEB-12 and OPS-7")[0], "")
	if len(cands) != 2 || cands[0].Source != "jira" || cands[0].After != by["Login page"] || cands[1].Source != "key" {
		t.Errorf("key candidates: %+v", cands)
	}
	// Links resolve by id, custom id, heading and file::heading.
	for target, want := range map[string]string{
		"id:dbmig": "Database migration", "#WEB-12": "Login page", "*Deploy": "Deploy",
		"file:proj.org::*Login page": "Login page", "Review the pull request": "Review the pull request",
		"https://x.atlassian.net/browse/INF-4": "Deploy",
	} {
		if got := idx.resolveLink(target); got == nil || got.Title != want {
			t.Errorf("resolveLink(%q) = %v, want %s", target, got, want)
		}
	}
	if idx.resolveLink("file:other.org::*Deploy") != nil || idx.resolveLink("file:proj.org") != nil {
		t.Errorf("a link to another file's heading, or a whole file, is not a task")
	}
}

func TestTitleMatching(t *testing.T) {
	idx, by := sgTestIndex()
	if got, s := idx.matchTitle("the database migration", nil); got != by["Database migration"] || s < 0.9 {
		t.Errorf("whole title: %v %v", got, s)
	}
	// One word of a two-word title is not enough.
	if got, _ := idx.matchTitle("review", nil); got != nil {
		t.Errorf("one word matched %v", got)
	}
	if got, s := idx.matchTitle("pull request review from Sam", nil); got != by["Review the pull request"] || s < 0.6 {
		t.Errorf("words out of order: %v %v", got, s)
	}
	if got, _ := idx.matchTitle("lunch", nil); got != nil {
		t.Errorf("lunch matched %v", got)
	}
	// Excluding the task itself.
	if got, _ := idx.matchTitle("deploy", by["Deploy"]); got != nil {
		t.Errorf("matched itself")
	}
}

func TestOrdinalSequence(t *testing.T) {
	for title, want := range map[string]int{
		"1. Design": 1, "2) Build": 2, "3 - Test": 3, "Step 4 polish": 4, "Phase II": 2, "Part iii. Docs": 3,
		"(5) Ship": 5, "iv) Fourth": 4,
	} {
		if n, ok := ordinalOf(title); !ok || n != want {
			t.Errorf("ordinalOf(%q) = %d %v, want %d", title, n, ok, want)
		}
	}
	for _, title := range []string{"2026 planning", "Design", "v2 api", "Mix it up"} {
		if n, ok := ordinalOf(title); ok {
			t.Errorf("ordinalOf(%q) = %d, want none", title, n)
		}
	}
	if got := stripOrdinal("Phase II: Build it"); got != "Build it" {
		t.Errorf("stripOrdinal: %q", got)
	}
	mk := func(h, title, parent string, line int) *sgTask {
		return &sgTask{Hash: h, Title: title, Parent: parent, File: "f", Line: line, Props: map[string]string{}}
	}
	idx := newSgIndex([]*sgTask{
		mk("c", "3. Ship", "p", 3), mk("a", "1. Design", "p", 1), mk("b1", "2. Build server", "p", 2),
		mk("b2", "2. Build client", "p", 4), mk("x", "1. Elsewhere", "q", 5), mk("y", "Notes", "p", 6),
	})
	got := map[string]string{}
	for _, c := range idx.sequenceCands() {
		got[c.Task.Hash] = c.After.Hash
		if c.Source != "sequence" || c.Conf != 0.5 {
			t.Errorf("sequence candidate %+v", c)
		}
	}
	want := map[string]string{"b1": "a", "b2": "a", "c": "b2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sequence: %v, want %v", got, want)
	}
}

func TestTransitiveReduction(t *testing.T) {
	// c waits on a by AFTER; its ORDERED chain (fixed) is c -> b -> a.
	edges := []sgEdge{
		{From: "b", To: "a"},
		{From: "c", To: "a", Removable: true},
		{From: "c", To: "b"},
	}
	got := redundantEdges(edges)
	if len(got) != 1 || got[0].Task != "c" || got[0].After != "a" || !reflect.DeepEqual(got[0].Via, []string{"b"}) || !got[0].Removable {
		t.Errorf("reduction: %+v", got)
	}
	// An AFTER saying exactly what the chain says: redundant with no Via.
	got = redundantEdges([]sgEdge{{From: "c", To: "b", Removable: true}, {From: "c", To: "b"}})
	if len(got) != 1 || len(got[0].Via) != 0 {
		t.Errorf("duplicate: %+v", got)
	}
	// Two removable links that imply each other: only one goes.
	got = redundantEdges([]sgEdge{{From: "x", To: "y", Removable: true}, {From: "x", To: "y", Removable: true}})
	if len(got) != 1 {
		t.Errorf("mutual: %+v", got)
	}
	// A fixed edge is never reported, and nothing is reported in a plain chain.
	if got := redundantEdges([]sgEdge{{From: "b", To: "a"}, {From: "c", To: "b", Removable: true}}); len(got) != 0 {
		t.Errorf("chain: %+v", got)
	}
}

func TestFindCycles(t *testing.T) {
	got := findCycles([]sgEdge{
		{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"},
		{From: "d", To: "a"}, {From: "e", To: "e"}, {From: "f", To: "g"},
	})
	want := [][]string{{"a", "b", "c"}, {"e"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cycles %v, want %v", got, want)
	}
}

func TestAcceptDepsAvoidsCycles(t *testing.T) {
	mk := func(h string) *sgTask { return &sgTask{Hash: h, Title: h, Props: map[string]string{}} }
	a, b, c, d, e := mk("a"), mk("b"), mk("c"), mk("d"), mk("e")
	d.After = "a" // an existing link: d waits on a
	e.Parent = "a"
	idx := newSgIndex([]*sgTask{a, b, c, d, e})
	cands := []sgCand{
		{Task: b, After: a, Source: "text", Conf: 0.6},
		{Task: a, After: b, Source: "link", Conf: 0.9},     // taken first
		{Task: c, After: a, Source: "key", Conf: 0.85},     // c -> a -> b
		{Task: b, After: c, Source: "text", Conf: 0.7},     // would close b -> c -> a -> b
		{Task: c, After: b, Source: "text", Conf: 0.5},     // c already has one
		{Task: d, After: b, Source: "link", Conf: 0.9},     // d already waits on a
		{Task: a, After: a, Source: "link", Conf: 0.95},    // itself
		{Task: e, After: a, Source: "sequence", Conf: 0.5}, // a is e's parent
	}
	got := acceptDeps(cands, idx)
	pairs := []string{}
	for _, s := range got {
		pairs = append(pairs, s.Task+"<"+s.After)
	}
	if want := []string{"a<b", "c<a"}; !reflect.DeepEqual(pairs, want) {
		t.Errorf("accepted %v, want %v", pairs, want)
	}
	if got[0].AfterTitle != "b" || got[0].Confidence != 0.9 {
		t.Errorf("suggestion fields: %+v", got[0])
	}
}

func TestRoundEffort(t *testing.T) {
	cases := []struct {
		mins, day float64
		want      string
		wantMins  float64
	}{
		{5, 8, "15min", 15},
		{38, 8, "45min", 45},
		{65, 8, "1h", 60},
		{100, 8, "1.75h", 105},
		{119, 8, "2h", 120},
		{200, 8, "3.5h", 210},
		{470, 8, "1d", 480},
		{700, 8, "1.5d", 720},
		{2900, 8, "6d", 2880},
		{300, 6, "5h", 300},
		{350, 6, "1d", 360},
	}
	for _, c := range cases {
		got, m := roundEffort(c.mins, c.day)
		if got != c.want || m != c.wantMins {
			t.Errorf("roundEffort(%v, %v) = %q %v, want %q %v", c.mins, c.day, got, m, c.want, c.wantMins)
		}
	}
	if got, _ := roundEffort(0, 8); got != "" {
		t.Errorf("zero minutes: %q", got)
	}
}

func TestGuessEffort(t *testing.T) {
	hist := []sgSample{
		{Tags: []string{"web"}, Assignee: "Alice", Mins: 120, File: "a.org"},
		{Tags: []string{"web"}, Assignee: "Alice", Mins: 180, File: "a.org"},
		{Tags: []string{"web"}, Assignee: "Alice", Mins: 90, File: "a.org"},
		{Tags: []string{"web"}, Assignee: "Bob", Mins: 600, File: "a.org"},
		{Tags: []string{"db"}, Assignee: "Bob", Mins: 480, Words: sgWords("Migrate users table"), File: "b.org"},
		{Tags: []string{"db"}, Assignee: "Bob", Mins: 240, Words: sgWords("Migrate orders table"), File: "b.org"},
		{Tags: []string{"ops"}, Assignee: "Carol", Mins: 60, Words: sgWords("Migrate the logs table"), File: "c.org"},
	}
	g, ok := guessEffort(&sgTask{Hash: "t", Title: "Header", Tags: []string{"web"}, Assignee: "Alice"}, hist, nil, 8)
	if !ok || g.Effort != "2h" || g.Samples != 3 || !strings.Contains(g.Basis, "tagged :web: by Alice") {
		t.Errorf("assignee and tag: %+v", g)
	}
	g, _ = guessEffort(&sgTask{Hash: "t", Title: "Header", Tags: []string{"web"}, Assignee: "Dan"}, hist, nil, 8)
	if g.Samples != 4 || !strings.HasSuffix(g.Basis, "tagged :web:") || g.Effort != "2.5h" {
		t.Errorf("tag: %+v", g)
	}
	g, _ = guessEffort(&sgTask{Hash: "t", Title: "Migrate the payments table", File: "z.org"}, hist, nil, 8)
	if g.Samples != 3 || !strings.Contains(g.Basis, "titles like this one") || g.Effort != "4h" {
		t.Errorf("title words: %+v", g)
	}
	g, ok = guessEffort(&sgTask{Hash: "t", Title: "Something new", File: "z.org"}, hist, []float64{60, 240, 480}, 8)
	if !ok || g.Effort != "4h" || g.Confidence != 0.2 || !strings.Contains(g.Basis, "in this chart") {
		t.Errorf("chart fallback: %+v", g)
	}
	if _, ok := guessEffort(&sgTask{Hash: "t", Title: "Something new"}, nil, nil, 8); ok {
		t.Errorf("guessed with nothing to go on")
	}
}

// Groups of the given sizes, alternating name and size.
func laneGroups(spec ...interface{}) []common.GanttLaneGroup {
	out := []common.GanttLaneGroup{}
	k := 0
	for i := 0; i+1 < len(spec); i += 2 {
		tasks := []string{}
		for n := 0; n < spec[i+1].(int); n++ {
			tasks = append(tasks, string(rune('A'+k)))
			k++
		}
		out = append(out, common.GanttLaneGroup{Name: spec[i].(string), Tasks: tasks})
	}
	return out
}

func TestLaneScoring(t *testing.T) {
	even := scoreLanes(laneGroups("a", 3, "b", 3), 6, 0, 2)
	lopsided := scoreLanes(laneGroups("a", 5, "b", 1), 6, 0, 2)
	partial := scoreLanes(laneGroups("a", 2, "b", 2, sgNone, 2), 6, 0, 2)
	crossing := scoreLanes(laneGroups("a", 3, "b", 3), 6, 2, 2)
	one := scoreLanes(laneGroups("a", 6), 6, 0, 2)
	if !(even > lopsided && even > partial && even > crossing) || one != 0 {
		t.Errorf("even %v lopsided %v partial %v crossing %v one %v", even, lopsided, partial, crossing, one)
	}
	many := []interface{}{}
	for i := 0; i < 12; i++ {
		many = append(many, string(rune('a'+i)), 1)
	}
	if s := scoreLanes(laneGroups(many...), 12, 0, 0); s >= scoreLanes(laneGroups("a", 6, "b", 6), 12, 0, 0) {
		t.Errorf("twelve lanes of one scored %v", s)
	}

	mk := func(h, file, parent string, props map[string]string, tags ...string) *sgTask {
		return &sgTask{Hash: h, Title: "T" + h, File: file, ParentTitle: parent, Props: props, Tags: tags}
	}
	tasks := []*sgTask{
		mk("1", "/a.org", "P", map[string]string{"COMPONENT": "api", "ID": "x1"}, "work", "api"),
		mk("2", "/a.org", "P", map[string]string{"COMPONENT": "api", "ID": "x2"}, "work", "api"),
		mk("3", "/a.org", "Q", map[string]string{"COMPONENT": "web", "ID": "x3"}, "work", "web"),
		mk("4", "/b.org", "Q", map[string]string{"COMPONENT": "web", "ID": "x4"}, "work", "web"),
	}
	lanes := laneSuggestions(tasks, [][2]string{{"2", "1"}, {"4", "3"}})
	byKind := map[string]bool{}
	for _, l := range lanes {
		key := l.By + ":" + l.Prop
		if byKind[key] {
			t.Errorf("duplicate lane kind %s", key)
		}
		byKind[key] = true
		if l.Prop == "ID" {
			t.Errorf("ID offered as a lane")
		}
	}
	// tag (first tag on some but not all: api/web), COMPONENT and chain are
	// the same split; only one of them is kept, and it crosses no links.
	if len(lanes) == 0 || lanes[0].CrossLinks != 0 || lanes[0].Score <= 0.9 {
		t.Errorf("best lane: %+v", lanes)
	}
	splits := 0
	for _, l := range lanes {
		if l.By == "tag" || l.By == "chain" || (l.By == "property" && l.Prop == "COMPONENT") {
			splits++
		}
	}
	if splits != 1 {
		t.Errorf("the same split offered %d times: %+v", splits, lanes)
	}
}

func TestGanttIdFor(t *testing.T) {
	// The same vector as idFor in worg's ganttops.ts would give.
	if got := ganttIdFor("Database migration!", "Ab+/9xYz12=="); got != "database-migration-ab9xyz" {
		t.Errorf("ganttIdFor: %q", got)
	}
	if got := ganttIdFor("???", ""); got != "task" {
		t.Errorf("ganttIdFor empty: %q", got)
	}
	if got := ganttIdFor("A very long title that goes on and on", "zz"); got != "a-very-long-title-that-g-zz" {
		t.Errorf("ganttIdFor long: %q", got)
	}
}
