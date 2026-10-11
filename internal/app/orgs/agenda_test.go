package orgs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Saturday. Every rule is stated against a fixed today.
const agToday = "2026-10-10"

func agDay(t *testing.T, s string) int {
	t.Helper()
	n, err := agendaParseDay(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Loads one file into a database of its own and lays out [from, to].
func agRun(t *testing.T, text, from, to string, closed bool) common.AgendaResult {
	t.Helper()
	loadOrg(t, map[string]string{"agenda.org": text})
	r := agendaRules{From: agDay(t, from), To: agDay(t, to), Today: agDay(t, agToday), Closed: closed, WarnDays: 14}
	res, err := buildAgenda("", r, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// One line per entry: "2026-10-12 Title kind @10:00-11:00 r late=2 due=3 done".
func agLines(res common.AgendaResult) string {
	var out []string
	for _, d := range res.Days {
		for _, e := range d.Entries {
			n := res.Nodes[e.Node]
			// A stamp in the headline is part of its text, as /search says.
			title := strings.TrimSpace(strings.Split(n.Headline, "<")[0])
			s := d.Date + " " + title + " " + e.Kind
			if e.HasTime {
				s += " @" + e.Start[strings.Index(e.Start, "T")+1:]
				if e.End != "" {
					s += "-" + e.End[strings.Index(e.End, "T")+1:]
				}
			} else if e.Start != d.Date {
				s += " start=" + e.Start
			}
			if e.Repeat {
				s += " r"
			}
			if e.Late != 0 {
				s += fmt.Sprintf(" late=%d", e.Late)
			}
			if e.DueIn != -1 {
				s += fmt.Sprintf(" due=%d", e.DueIn)
			}
			if e.Done {
				s += " done"
			}
			out = append(out, s)
		}
	}
	return strings.Join(out, "\n")
}

// An archived tree stays out of the agenda unless asked for: a task filed
// away long ago must not come back as today's overdue item.
func TestAgendaSkipsArchivedTrees(t *testing.T) {
	text := "* Live\n** TODO Due soon\n   SCHEDULED: <2026-10-08 Thu>\n" +
		"* Old :ARCHIVE:\n** TODO Forgotten\n   SCHEDULED: <2026-09-01 Tue>\n"
	loadOrg(t, map[string]string{"agenda.org": text})
	r := agendaRules{From: agDay(t, agToday), To: agDay(t, agToday), Today: agDay(t, agToday), WarnDays: 14}
	res, err := buildAgenda("", r, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	agWant(t, "archived left out", res, "2026-10-10 Due soon scheduled late=2")
	r.Archived = true
	if res, err = buildAgenda("", r, time.Now()); err != nil {
		t.Fatal(err)
	}
	agWant(t, "archived asked for", res,
		"2026-10-10 Due soon scheduled late=2\n2026-10-10 Forgotten scheduled late=39")
}

// Review fixes: hourly repeaters, commented subtrees, a repeating range that
// overlaps itself, and -0d.
func TestAgendaReviewCases(t *testing.T) {
	cases := []struct{ name, text, from, to, want string }{
		{"an hourly repeater lands every day from its base", `* Ping <2026-10-10 Sat 10:00 +1h>`, "2026-10-09", "2026-10-11",
			"2026-10-10 Ping timestamp @10:00\n2026-10-11 Ping timestamp @10:00 r"},
		{"a commented subtree is not in the agenda", "* COMMENT Old\n** TODO Kid\n   SCHEDULED: <2026-10-01 Thu>\n", agToday, agToday, ``},
		{"a repeating range gives one entry a day", `* Run <2026-10-09 Fri +1d>--<2026-10-11 Sun>`, "2026-10-09", "2026-10-12",
			"2026-10-09 Run timestamp\n2026-10-10 Run timestamp\n2026-10-11 Run timestamp\n2026-10-12 Run timestamp r"},
		{"-0d does not warn", "* TODO Due\n   DEADLINE: <2026-10-11 Sun -0d>\n", agToday, "2026-10-11",
			"2026-10-11 Due deadline"},
	}
	for _, c := range cases {
		agWant(t, c.name, agRun(t, c.text, c.from, c.to, false), c.want)
	}
}

func agWant(t *testing.T, name string, res common.AgendaResult, want string) {
	t.Helper()
	got := agLines(res)
	want = strings.TrimSpace(want)
	if got != want {
		t.Errorf("%s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestAgendaOccurrences(t *testing.T) {
	cases := []struct {
		name, text, from, to string
		closed               bool
		want                 string
	}{
		{"a plain stamp lands on its day", `* Meeting <2026-10-12 Mon 10:00>`, "2026-10-10", "2026-10-14", false,
			`2026-10-12 Meeting timestamp @10:00`},
		{"a stamp in the body counts", "* Dentist\nAt the usual place <2026-10-13 Tue>\n", "2026-10-10", "2026-10-14", false,
			`2026-10-13 Dentist timestamp`},
		{"two stamps, two days", "* Two <2026-10-12 Mon>\nand again <2026-10-14 Wed>\n", "2026-10-10", "2026-10-14", false,
			"2026-10-12 Two timestamp\n2026-10-14 Two timestamp"},
		{"an inactive stamp is not an appointment", "* Note [2026-10-12 Mon]\n", "2026-10-10", "2026-10-14", false, ``},
		{"+1d repeats from its base, never before it", `* Daily <2026-10-11 Sun +1d>`, "2026-10-09", "2026-10-13", false,
			"2026-10-11 Daily timestamp\n2026-10-12 Daily timestamp r\n2026-10-13 Daily timestamp r"},
		{"++1w", `* Weekly <2026-09-24 Thu ++1w>`, "2026-10-01", "2026-10-16", false,
			"2026-10-01 Weekly timestamp r\n2026-10-08 Weekly timestamp r\n2026-10-15 Weekly timestamp r"},
		{".+2d", `* Water <2026-10-06 Tue .+2d>`, "2026-10-09", "2026-10-13", false,
			"2026-10-10 Water timestamp r\n2026-10-12 Water timestamp r"},
		{"+1m keeps to the end of a short month", `* Rent <2026-01-31 Sat +1m>`, "2026-01-01", "2026-05-31", false,
			"2026-01-31 Rent timestamp\n2026-02-28 Rent timestamp r\n2026-03-31 Rent timestamp r\n2026-04-30 Rent timestamp r\n2026-05-31 Rent timestamp r"},
		{"+1y on a leap day", `* Leap <2024-02-29 Thu +1y>`, "2027-02-27", "2027-03-01", false,
			"2027-02-28 Leap timestamp r"},
		{"+1y on a leap day, in a leap year", `* Leap <2024-02-29 Thu +1y>`, "2028-02-27", "2028-03-01", false,
			"2028-02-29 Leap timestamp r"},
		{"+2w", `* Fortnight <2026-09-14 Mon +2w>`, "2026-10-01", "2026-10-31", false,
			"2026-10-12 Fortnight timestamp r\n2026-10-26 Fortnight timestamp r"},
		{"a range spans its days, times on the ends", `* Conference <2026-10-12 Mon 10:00>--<2026-10-14 Wed 12:00>`, "2026-10-10", "2026-10-15", false,
			"2026-10-12 Conference timestamp @10:00-23:59\n2026-10-13 Conference timestamp\n2026-10-14 Conference timestamp @00:00-12:00"},
		{"a range of dates is all day", `* Holiday <2026-10-09 Fri>--<2026-10-11 Sun>`, "2026-10-10", "2026-10-15", false,
			"2026-10-10 Holiday timestamp\n2026-10-11 Holiday timestamp"},
		{"a time range within a day", `* Call <2026-10-12 Mon 10:00-11:30>`, "2026-10-12", "", false,
			`2026-10-12 Call timestamp @10:00-11:30`},
		{"scheduled in the future", "* TODO Later\nSCHEDULED: <2026-10-12 Mon>\n", "2026-10-09", "2026-10-13", false,
			`2026-10-12 Later scheduled`},
		{"scheduled today", "* TODO Now\nSCHEDULED: <2026-10-10 Sat>\n", "2026-10-09", "2026-10-13", false,
			`2026-10-10 Now scheduled`},
		{"scheduled in the past, not done: its day and today only", "* TODO Slipped\nSCHEDULED: <2026-10-05 Mon>\n", "2026-10-04", "2026-10-12", false,
			"2026-10-05 Slipped scheduled\n2026-10-10 Slipped scheduled late=5"},
		{"scheduled in the past, not done, today not asked", "* TODO Slipped\nSCHEDULED: <2026-10-05 Mon>\n", "2026-10-06", "2026-10-09", false, ``},
		{"scheduled in the past, done: its day only", "* DONE Finished\nSCHEDULED: <2026-10-05 Mon>\n", "2026-10-04", "2026-10-12", false,
			`2026-10-05 Finished scheduled done`},
		{"a repeating schedule, overdue", "* TODO Daily review\nSCHEDULED: <2026-10-08 Thu +1d>\n", "2026-10-08", "2026-10-12", false,
			"2026-10-08 Daily review scheduled\n2026-10-09 Daily review scheduled r\n2026-10-10 Daily review scheduled late=2\n2026-10-11 Daily review scheduled r\n2026-10-12 Daily review scheduled r"},
		{"a deadline warns inside the default 14 days", "* TODO Report\nDEADLINE: <2026-10-20 Tue>\n", "2026-10-10", "2026-10-20", false,
			"2026-10-10 Report deadline due=10\n2026-10-20 Report deadline due=10"},
		{"a deadline beyond the warning is only on its day", "* TODO Far\nDEADLINE: <2026-10-30 Fri>\n", "2026-10-10", "2026-10-30", false,
			`2026-10-30 Far deadline`},
		{"a deadline's own -Nd", "* TODO Short\nDEADLINE: <2026-10-20 Tue -3d>\n", "2026-10-10", "2026-10-20", false,
			`2026-10-20 Short deadline`},
		{"a deadline's own -Nd, inside it", "* TODO Soon\nDEADLINE: <2026-10-12 Mon -3d>\n", "2026-10-10", "2026-10-12", false,
			"2026-10-10 Soon deadline due=2\n2026-10-12 Soon deadline due=2"},
		{"due today", "* TODO Today\nDEADLINE: <2026-10-10 Sat>\n", "2026-10-09", "2026-10-11", false,
			`2026-10-10 Today deadline due=0`},
		{"overdue", "* TODO Overdue\nDEADLINE: <2026-10-07 Wed>\n", "2026-10-06", "2026-10-11", false,
			"2026-10-07 Overdue deadline\n2026-10-10 Overdue deadline late=3"},
		{"a done deadline neither warns nor goes overdue", "* DONE Handed in\nDEADLINE: <2026-10-12 Mon>\n* DONE Late one\nDEADLINE: <2026-10-07 Wed>\n", "2026-10-06", "2026-10-12", false,
			"2026-10-07 Late one deadline done\n2026-10-12 Handed in deadline done"},
		{"closed is left out unless asked", "* DONE Shipped\nCLOSED: [2026-10-09 Fri 14:00]\n", "2026-10-08", "2026-10-10", false, ``},
		{"closed, asked for", "* DONE Shipped\nCLOSED: [2026-10-09 Fri 14:00]\n", "2026-10-08", "2026-10-10", true,
			`2026-10-09 Shipped closed @14:00 done`},
		{"a child's stamp is the child's", "* Parent\n** Child <2026-10-12 Mon>\n", "2026-10-10", "2026-10-14", false,
			`2026-10-12 Child timestamp`},
		{"a column-zero file still attributes stamps", `* Project
** TODO Task
SCHEDULED: <2026-10-12 Mon>
:PROPERTIES:
:ID: task-1
:END:
:LOGBOOK:
- Note taken on <2026-10-13 Tue>
:END:
Body <2026-10-14 Wed>
** TODO Sibling
<2026-10-15 Thu>
`, "2026-10-10", "2026-10-16", false,
			"2026-10-12 Task scheduled\n2026-10-14 Task timestamp\n2026-10-15 Sibling timestamp"},
		{"a file's own keywords decide done", `#+TODO: OPEN WAIT | SHUT
* SHUT Old
SCHEDULED: <2026-10-01 Thu>
* OPEN New
SCHEDULED: <2026-10-01 Thu>
`, "2026-10-01", "2026-10-10", false,
			"2026-10-01 New scheduled\n2026-10-01 Old scheduled done\n2026-10-10 New scheduled late=9"},
	}
	for _, c := range cases {
		to := c.to
		if to == "" {
			to = c.from
		}
		agWant(t, c.name, agRun(t, c.text, c.from, to, c.closed), c.want)
	}
}

// Timed by time, then all day by kind (deadline, scheduled, timestamp), then
// priority, then title.
func TestAgendaSortsADay(t *testing.T) {
	res := agRun(t, `* Afternoon <2026-10-12 Mon 14:00>
* TODO Morning
SCHEDULED: <2026-10-12 Mon 09:00>
* Zed <2026-10-12 Mon>
* Alpha <2026-10-12 Mon>
* TODO Plain
SCHEDULED: <2026-10-12 Mon>
* TODO [#A] Urgent
SCHEDULED: <2026-10-12 Mon>
* TODO Due
DEADLINE: <2026-10-12 Mon -0d>
`, "2026-10-12", "2026-10-12", false)
	agWant(t, "order", res, `
2026-10-12 Morning scheduled @09:00
2026-10-12 Afternoon timestamp @14:00
2026-10-12 Due deadline
2026-10-12 Urgent scheduled
2026-10-12 Plain scheduled
2026-10-12 Alpha timestamp
2026-10-12 Zed timestamp`)
	if len(res.Days) != 1 || res.Days[0].Date != "2026-10-12" {
		t.Errorf("days %v", res.Days)
	}
}

// Every day asked about is listed, empty or not, and a heading on several
// days is one node.
func TestAgendaListsEveryDayAndEachHeadingOnce(t *testing.T) {
	res := agRun(t, "* Daily <2026-10-10 Sat +1d>\n", "2026-10-10", "2026-10-13", false)
	if len(res.Days) != 4 || len(res.Nodes) != 1 {
		t.Fatalf("%d days, %d nodes", len(res.Days), len(res.Nodes))
	}
	res = agRun(t, "* Nothing\n", "2026-10-10", "2026-10-12", false)
	if len(res.Days) != 3 || res.Days[1].Entries == nil || len(res.Days[1].Entries) != 0 {
		t.Errorf("empty days: %+v", res.Days)
	}
}

func agNode(t *testing.T, res common.AgendaNodes, title string) common.AgendaNode {
	t.Helper()
	for _, n := range res.Nodes {
		if n.Headline == title {
			return n
		}
	}
	t.Fatalf("no node %q", title)
	return common.AgendaNode{}
}

func TestAgendaNodes(t *testing.T) {
	loadOrg(t, map[string]string{"nodes.org": `#+FILETAGS: :work:
#+TODO: OPEN WAIT | SHUT
* Project :proj:
** OPEN First :x:
SCHEDULED: <2026-10-12 Mon 09:30 +1w> DEADLINE: <2026-10-20 Tue -3d>
:LOGBOOK:
CLOCK: [2026-10-09 Fri 09:00]--[2026-10-09 Fri 10:30] =>  1:30
:END:
Meet <2026-10-14 Wed 10:00-11:00> and <2026-10-15 Thu>
** SHUT Second
CLOSED: [2026-10-08 Thu 17:00]
** Third
*** OPEN Grandchild
* Old :ARCHIVE:
** OPEN Buried
* OPEN Habit
SCHEDULED: <2026-10-10 Sat .+1d>
:PROPERTIES:
:STYLE: habit
:END:
:LOGBOOK:
- State "SHUT"       from "OPEN"       [2026-10-09 Fri 08:00]
- State "DONE"       from "OPEN"       [2026-10-08 Thu 08:00]
- State "DONE"       from "OPEN"       [2026-10-07 Wed 08:00]
:END:
`})
	res, err := buildAgendaNodes("", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 8 {
		t.Errorf("%d nodes, want 8", len(res.Nodes))
	}
	p := agNode(t, res, "Project")
	if p.Children.Count != 3 || p.Children.Todo != 1 || p.Children.Done != 1 ||
		strings.Join(p.Children.States, ",") != "OPEN,SHUT," {
		t.Errorf("children %+v", p.Children)
	}
	f := agNode(t, res, "First")
	if strings.Join(f.Olp, "/") != "Project/First" {
		t.Errorf("olp %v", f.Olp)
	}
	if !f.IsTodo || f.IsDone || f.IsArchived || f.IsHabit {
		t.Errorf("flags %+v", f)
	}
	if strings.Join(f.AllTags, ",") != "x,proj,work" || strings.Join(f.Tags, ",") != "x" {
		t.Errorf("tags %v / %v", f.AllTags, f.Tags)
	}
	if f.Scheduled == nil || f.Scheduled.Start != "2026-10-12T09:30" || !f.Scheduled.HasTime ||
		f.Scheduled.Repeat != "+1w" || f.Scheduled.Text != "<2026-10-12 Mon 09:30 +1w>" || !f.Scheduled.Active {
		t.Errorf("scheduled %+v", f.Scheduled)
	}
	if f.Deadline == nil || f.Deadline.Start != "2026-10-20" || f.Deadline.Warn != "-3d" || f.Deadline.HasTime {
		t.Errorf("deadline %+v", f.Deadline)
	}
	if len(f.Timestamps) != 2 || f.Timestamps[0].Start != "2026-10-14T10:00" || f.Timestamps[0].End != "2026-10-14T11:00" ||
		f.Timestamps[0].Text != "<2026-10-14 Wed 10:00-11:00>" || f.Timestamps[1].Start != "2026-10-15" {
		t.Errorf("timestamps %+v", f.Timestamps)
	}
	if len(f.Clocks) != 1 || f.Clocks[0].Start != "2026-10-09T09:00" || f.Clocks[0].End != "2026-10-09T10:30" || f.ClockMins != 90 {
		t.Errorf("clocks %+v %v", f.Clocks, f.ClockMins)
	}
	if strings.Join(f.TodoKeys, ",") != "OPEN,WAIT" || strings.Join(f.DoneKeys, ",") != "SHUT" {
		t.Errorf("keys %v | %v", f.TodoKeys, f.DoneKeys)
	}
	s := agNode(t, res, "Second")
	if !s.IsDone || s.IsTodo || s.Closed == nil || s.Closed.Active || s.Closed.Start != "2026-10-08T17:00" {
		t.Errorf("second %+v %+v", s, s.Closed)
	}
	if b := agNode(t, res, "Buried"); !b.IsArchived {
		t.Error("an ARCHIVE tag above archives")
	}
	h := agNode(t, res, "Habit")
	if !h.IsHabit || strings.Join(h.DoneDates, ",") != "2026-10-07,2026-10-08" {
		t.Errorf("habit %v %v", h.IsHabit, h.DoneDates)
	}
	if third := agNode(t, res, "Third"); third.DoneDates != nil || third.IsTodo || third.IsDone {
		t.Errorf("third %+v", third)
	}
}

// The hashes in an answer address the heading everywhere else: /hash and the
// edit endpoints - on a fresh database, where nothing has been registered by
// a search first.
func TestAgendaHashesAreLive(t *testing.T) {
	paths := loadOrg(t, map[string]string{"h.org": "* Inbox\n** TODO Call Bob\nSCHEDULED: <2026-10-10 Sat>\n"})
	code, raw := call(t, RequestAgenda, "GET", "/agenda?from=2026-10-10&today=2026-10-10", nil, nil)
	var res common.AgendaResult
	if err := json.Unmarshal(raw, &res); err != nil || code != 200 || !res.Ok {
		t.Fatalf("%d %v %s", code, err, raw)
	}
	if len(res.Nodes) != 1 {
		t.Fatalf("nodes %s", raw)
	}
	n := res.Nodes[0]
	if sec := GetDb().FindByHash(n.Hash); sec == nil || common.GetHeadlineTitle(sec.Headline) != "Call Bob" {
		t.Fatalf("hash %q does not find the heading", n.Hash)
	}
	enc := base64.URLEncoding.EncodeToString([]byte(n.Hash))
	_, raw = call(t, RequestByHash, "GET", "/hash/"+enc, map[string]string{"hash": enc}, nil)
	var td common.Todo
	if err := json.Unmarshal(raw, &td); err != nil || td.Headline != "Call Bob" {
		t.Fatalf("/hash: %v %s", err, raw)
	}
	_, raw = call(t, PostChangeStatus, "POST", "/status/change", nil, common.TodoItemChange{Hash: n.Hash, Value: "DONE"})
	if !strings.Contains(readOrg(t, paths["h.org"]), "** DONE Call Bob") {
		t.Fatalf("status change through the agenda's hash did nothing: %s\n%s", raw, readOrg(t, paths["h.org"]))
	}
}

func TestAgendaHandler(t *testing.T) {
	loadOrg(t, map[string]string{"web.org": `* TODO Write
SCHEDULED: <2026-10-08 Thu>
* DONE Wrote
CLOSED: [2026-10-09 Fri 10:00]
* Party <2026-10-11 Sun 19:00>
`})
	get := func(u string) common.AgendaResult {
		t.Helper()
		code, raw := call(t, RequestAgenda, "GET", u, nil, nil)
		var res common.AgendaResult
		if err := json.Unmarshal(raw, &res); err != nil || code != 200 {
			t.Fatalf("%s: %d %v %s", u, code, err, raw)
		}
		return res
	}
	res := get("/agenda?from=2026-10-09&to=2026-10-11&today=2026-10-10")
	agWant(t, "all", res, `
2026-10-10 Write scheduled late=2
2026-10-11 Party timestamp @19:00`)
	if res.From != "2026-10-09" || res.To != "2026-10-11" || res.Today != "2026-10-10" {
		t.Errorf("echo %s %s %s", res.From, res.To, res.Today)
	}
	res = get("/agenda?from=2026-10-09&to=2026-10-11&today=2026-10-10&closed=t")
	agWant(t, "closed", res, `
2026-10-09 Wrote closed @10:00 done
2026-10-10 Write scheduled late=2
2026-10-11 Party timestamp @19:00`)
	res = get("/agenda?from=2026-10-09&to=2026-10-11&today=2026-10-10&query=" + url.QueryEscape("IsTodo()"))
	agWant(t, "query", res, `2026-10-10 Write scheduled late=2`)
	// to defaults to from, from to today.
	if res = get("/agenda?today=2026-10-10"); len(res.Days) != 1 || res.Days[0].Date != "2026-10-10" {
		t.Errorf("defaults %+v", res.Days)
	}
	for _, bad := range []string{"/agenda?from=10/10/2026", "/agenda?today=nope", "/agenda?from=2026-10-10&to=2026-10-01",
		"/agenda?from=2026-10-10&query=" + url.QueryEscape("IsTodo(")} {
		if res := get(bad); res.Ok || res.Msg == "" {
			t.Errorf("%s answered %+v", bad, res)
		}
	}

	code, raw := call(t, RequestAgendaNodes, "GET", "/agenda/nodes?query="+url.QueryEscape("IsTodo()"), nil, nil)
	var nodes common.AgendaNodes
	if err := json.Unmarshal(raw, &nodes); err != nil || code != 200 || !nodes.Ok || len(nodes.Nodes) != 1 || nodes.Nodes[0].Headline != "Write" {
		t.Fatalf("nodes: %v %s", err, raw)
	}
	_, raw = call(t, RequestAgendaNodes, "GET", "/agenda/nodes?query="+url.QueryEscape("IsTodo("), nil, nil)
	if err := json.Unmarshal(raw, &nodes); err != nil || nodes.Ok || nodes.Msg == "" {
		t.Fatalf("bad query: %s", raw)
	}
}

// OnDate() is the agenda for one day counted from the real today: what the
// day views built on `IsTodo() && OnDate(...)` ask.
func TestOnDateUsesTheAgenda(t *testing.T) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	stamp := func(d time.Time) string { return d.Format("2006-01-02 Mon") }
	ago := today.AddDate(0, 0, -3)
	soon := today.AddDate(0, 0, 5)
	loadOrg(t, map[string]string{"on.org": fmt.Sprintf(`* TODO Slipped
SCHEDULED: <%s>
* TODO Due soon
DEADLINE: <%s>
* Weekly <%s +1w>
* DONE Closed one
SCHEDULED: <%s>
CLOSED: [%s]
`, stamp(ago), stamp(soon), stamp(today.AddDate(0, 0, -14)), stamp(ago), stamp(ago))})
	f := GetDb().GetFile(GetDb().GetFiles()[0])
	secs := map[string]*org.Section{}
	for _, s := range flattenSections(f) {
		secs[strings.TrimSpace(strings.Split(common.GetHeadlineTitle(s.Headline), "<")[0])] = s
	}
	check := func(title string, d time.Time, want bool) {
		t.Helper()
		if got := IsOn(secs[title], f, d); got != want {
			t.Errorf("%s on %s: %v, want %v", title, d.Format("2006-01-02"), got, want)
		}
	}
	check("Slipped", today, true) // carried forward
	check("Slipped", ago, true)
	check("Slipped", today.AddDate(0, 0, -1), false)
	check("Due soon", today, true) // warning
	check("Due soon", soon, true)
	check("Due soon", today.AddDate(0, 0, 1), false)
	check("Weekly", today, true) // a repeat
	check("Weekly", today.AddDate(0, 0, 1), false)
	check("Closed one", today, false)
	check("Closed one", ago, true)

	// Through the query language, as worg, vscode and the CLI ask it.
	q := fmt.Sprintf(`!IsProject() && !IsArchived() && IsTodo() && OnDate("%s")`, today.Format("2006 02 01"))
	tds, err := QueryStringTodos(&common.StringQuery{Query: q})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, td := range *tds {
		got = append(got, td.Headline)
	}
	if strings.Join(got, ",") != "Slipped,Due soon" {
		t.Errorf("OnDate today: %v", got)
	}
	if !IsIn(secs["Slipped"], f, today.AddDate(0, 0, -7), today.AddDate(0, 0, 1)) || IsIn(secs["Due soon"], f, today.AddDate(0, 0, -7), today.AddDate(0, 0, -1)) {
		t.Error("IsIn")
	}
}
