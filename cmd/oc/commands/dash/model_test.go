package dash

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands/clocks"
	"github.com/ihdavids/orgs/internal/common"
)

func sampleInputs(now time.Time) inputs {
	day := now.Format("2006-01-02")
	at := func(hm string) string { return day + "T" + hm }
	node := func(title, status, prio string) common.AgendaNode {
		n := common.AgendaNode{}
		n.Headline, n.Status, n.Priority, n.Hash = title, status, prio, title
		return n
	}
	habit := node("Stretch", "TODO", "")
	habit.IsHabit = true
	done := node("Send the minutes", "DONE", "")
	done.IsDone = true
	return inputs{
		agenda: common.AgendaResult{Ok: true, Today: day,
			Nodes: []common.AgendaNode{
				node("Standup", "", ""), node("Lunch with Sam", "", ""), node("Design review: the new capture flow", "", ""),
				node("Ship the release notes", "NEXT", "A"), node("Write the migration plan", "TODO", ""),
				node("Renew passport", "TODO", ""), node("Pay the plumber", "TODO", ""), habit, done,
				node("Quarterly taxes", "TODO", "B"),
			},
			Days: []common.AgendaDay{{Date: day, Entries: []common.AgendaEntry{
				{Node: 0, Kind: "timestamp", Start: at("09:00"), End: at("09:15"), HasTime: true, DueIn: -1},
				{Node: 1, Kind: "timestamp", Start: at("12:30"), End: at("13:30"), HasTime: true, DueIn: -1},
				{Node: 2, Kind: "timestamp", Start: at("15:00"), End: at("16:00"), HasTime: true, DueIn: -1},
				{Node: 3, Kind: "deadline", Start: day, DueIn: 0},
				{Node: 9, Kind: "deadline", Start: day, DueIn: 3},
				{Node: 4, Kind: "scheduled", Start: day, DueIn: -1},
				{Node: 8, Kind: "scheduled", Start: day, DueIn: -1, Done: true},
				{Node: 5, Kind: "deadline", Start: day, Late: 3, DueIn: -1},
				{Node: 6, Kind: "scheduled", Start: day, Late: 40, DueIn: -1},
				{Node: 7, Kind: "scheduled", Start: day, Late: 1, DueIn: -1},
			}}}},
		clock: clocks.ClockData{Active: true, Target: common.Target{Id: "Work|Write the migration plan"}},
		focus: common.FocusState{Ok: true, Phase: "work", Title: "Write the migration plan", Remaining: 754, Total: 1500, Today: 3, TodayMinutes: 75},
		habits: common.HabitsResult{Ok: true, Habits: []common.Habit{
			{Headline: "Stretch", Due: true, Streak: 12, Days: days("dddd.dd")},
			{Headline: "Read 20 pages", DoneToday: true, Streak: 4, Days: days("m.ddddd")},
			{Headline: "Water the plants", Due: true, Missed: true, Days: days("d..m..m")},
			{Headline: "Call Mum", Due: false, Days: days(".......")},
		}},
		inbox:     common.ReviewList{Ok: true, Items: []common.ReviewItem{item("Idea: weekly digest email"), item("Look into the flaky test"), item("Book flights"), item("Reply to Ana")}},
		notices:   common.NoticeList{Ok: true, Unread: 1, Notices: []common.WatchNotice{{Label: "Stale NEXT actions", Gained: make([]common.WatchMatch, 2)}}},
	}
}

func days(s string) []common.HabitDay {
	out := []common.HabitDay{}
	for _, c := range s {
		st := "ok"
		switch c {
		case 'd':
			st = "done"
		case 'm':
			st = "miss"
		}
		out = append(out, common.HabitDay{State: st})
	}
	return out
}

func item(t string) common.ReviewItem {
	it := common.ReviewItem{}
	it.Headline = t
	return it
}

func TestBuild(t *testing.T) {
	now := time.Date(2026, 10, 23, 14, 12, 0, 0, time.Local)
	d := build(sampleInputs(now), now)
	if len(d.Overdue) != 2 || d.Overdue[0].Title != "Pay the plumber" || d.Overdue[1].Title != "Renew passport" {
		t.Errorf("overdue should be most late first and leave out the habit: %+v", d.Overdue)
	}
	if d.Summary.Appointments != 3 || d.Summary.ToDo != 3 || d.Summary.Done != 1 {
		t.Errorf("summary %+v", d.Summary)
	}
	if d.HabitsDue != 3 || d.HabitsDone != 1 || d.Habits[len(d.Habits)-1].Title != "Read 20 pages" {
		t.Errorf("habits: due %d done %d %+v", d.HabitsDue, d.HabitsDone, d.Habits)
	}
	if d.Clock == nil || d.Clock.Title != "Write the migration plan" {
		t.Errorf("clock title should be the last part of the outline path: %+v", d.Clock)
	}
	if d.InboxCount != 4 || len(d.Inbox) != 3 || d.NoticeCount != 1 {
		t.Errorf("inbox %d %v notices %d", d.InboxCount, d.Inbox, d.NoticeCount)
	}
}

func TestNowLineGoesBeforeTheNextAppointment(t *testing.T) {
	timed := []Item{{Start: "09:00", End: "09:15", HasTime: true}, {Start: "12:30", End: "13:30", HasTime: true}, {Start: "15:00", HasTime: true}}
	for now, want := range map[string]int{"08:00": 0, "09:05": 1, "13:00": 2, "15:00": 3, "18:00": 3} {
		if got := nowIndex(timed, now); got != want {
			t.Errorf("at %s the now line goes at %d, not %d", now, want, got)
		}
	}
	if !underway(timed[1], "13:00") || underway(timed[2], "15:00") {
		t.Errorf("an appointment with an end is under way inside it; a moment never is")
	}
}

func TestAFailedSectionDoesNotStopTheRest(t *testing.T) {
	now := time.Date(2026, 10, 23, 14, 12, 0, 0, time.Local)
	in := sampleInputs(now)
	in.inboxErr = errors.New("the server at x has no GET /review/inbox")
	in.habits = common.HabitsResult{Ok: false, Msg: "habits.org did not parse"}
	d := build(in, now)
	if d.Errors["inbox"] == "" || d.Errors["habits"] == "" || len(d.Today) == 0 {
		t.Errorf("errors %v, today %d", d.Errors, len(d.Today))
	}
	out := render(d, 120, now)
	if !strings.Contains(out, "couldn't read the inbox") || !strings.Contains(out, "couldn't read habits") {
		t.Errorf("a failed card should say so:\n%s", out)
	}
}

// Every line fits the width asked for, wide and narrow, so nothing wraps.
// Run with -v to see the screens.
func TestRenderFits(t *testing.T) {
	now := time.Date(2026, 10, 23, 14, 12, 0, 0, time.Local)
	d := build(sampleInputs(now), now)
	for _, w := range []int{140, 120, 100, 72, 50} {
		out := render(d, w, now)
		t.Logf("width %d:\n%s", w, out)
		for _, l := range strings.Split(out, "\n") {
			if n := vis(l); n > w {
				t.Errorf("width %d: a line is %d wide: %q", w, n, l)
			}
		}
	}
	empty := build(inputs{agenda: common.AgendaResult{Ok: true}, habits: common.HabitsResult{Ok: true},
		inbox: common.ReviewList{Ok: true}, notices: common.NoticeList{Ok: true}, focus: common.FocusState{Ok: true, Phase: "idle"}}, now)
	out := render(empty, 120, now)
	t.Logf("a clear day:\n%s", out)
	for _, want := range []string{"A clear day", "Nothing overdue", "Inbox empty", "Not clocked in"} {
		if !strings.Contains(out, want) {
			t.Errorf("an empty day should say %q", want)
		}
	}
}

// Not one request reached the server: one card says so, and the summary must
// not call it a clear day.
func TestUnreachableIsOneCardNotAClearDay(t *testing.T) {
	now := time.Date(2026, 10, 23, 9, 0, 0, 0, time.Local)
	e := errors.New("dial tcp: connection refused")
	d := build(inputs{server: "http://localhost:8010", agendaErr: e, clockErr: e, focusErr: e,
		habitsErr: e, inboxErr: e, noticeErr: e}, now)
	if d.Unreachable != "http://localhost:8010" {
		t.Fatalf("Unreachable = %q", d.Unreachable)
	}
	out := ansiRe.ReplaceAllString(render(d, 120, now), "")
	if strings.Contains(out, "clear day") || strings.Count(out, "╭") != 1 || !strings.Contains(out, "Can't reach http://localhost:8010") {
		t.Errorf("rendering:\n%s", out)
	}
}
