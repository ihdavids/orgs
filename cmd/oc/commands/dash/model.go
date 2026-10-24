package dash

// What the dashboard says, worked out from the answers of six endpoints.
// Pure (no requests, no terminal), so model_test.go can pin the decisions
// that are easy to get wrong: where "now" goes among the day's appointments,
// what counts as overdue, which habits are today's business.

import (
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands/clocks"
	"github.com/ihdavids/orgs/internal/common"
)

// Dash is the whole screen, and what -json prints.
type Dash struct {
	Date    string // YYYY-MM-DD, the server's today
	Time    string // HH:MM, this machine's clock when it was read
	Summary Summary

	Focus *common.FocusState `json:",omitempty"` // only while a session runs
	Clock *ClockNow          `json:",omitempty"`

	Today   []Item
	Overdue []Item

	Habits     []HabitRow
	HabitsDue  int
	HabitsDone int

	Inbox      []string // the first few titles
	InboxCount int

	Notices     []NoticeRow
	NoticeCount int

	// A section whose request failed, by name, with why. The rest still draw.
	Errors map[string]string `json:",omitempty"`
	// The server, when not one request reached it: then every card would
	// only say so, and an empty summary would read as a clear day.
	Unreachable string `json:",omitempty"`
}

type Summary struct {
	Appointments int // timed entries today
	ToDo         int // untimed, not done, not late
	Done         int // finished today
	Overdue      int
	HabitsLeft   int
	Inbox        int
	Notices      int
}

type ClockNow struct {
	Title   string
	File    string
	Started time.Time
	Mins    float64
}

// Item is one heading on today's list or the overdue pile.
type Item struct {
	Title    string
	Status   string `json:",omitempty"`
	Priority string `json:",omitempty"`
	Kind     string // timestamp, scheduled, deadline
	Start    string `json:",omitempty"` // HH:MM for a timed entry
	End      string `json:",omitempty"`
	HasTime  bool
	Late     int  `json:",omitempty"`
	DueIn    int  `json:",omitempty"`
	Done     bool `json:",omitempty"`
	Hash     string
	File     string
}

type HabitRow struct {
	Title  string
	Done   bool
	Missed bool
	Streak int
	Week   string // the last seven days, oldest first: d done, m missed, . neither
}

type NoticeRow struct {
	Label  string
	Gained int
	Count  int
	At     string
}

// The raw answers, each with its error. A failed one leaves its section empty
// and says so; it never stops the others.
type inputs struct {
	agenda    common.AgendaResult
	agendaErr error
	clock     clocks.ClockData
	clockErr  error
	focus     common.FocusState
	focusErr  error
	habits    common.HabitsResult
	habitsErr error
	inbox     common.ReviewList
	inboxErr  error
	notices   common.NoticeList
	noticeErr error
	server    string
}

func build(in inputs, now time.Time) Dash {
	d := Dash{Date: now.Format("2006-01-02"), Time: now.Format("15:04"), Errors: map[string]string{}}
	fail := func(section string, err error, ok bool, msg string) bool {
		if err != nil {
			d.Errors[section] = err.Error()
			return true
		}
		if !ok {
			if msg == "" {
				msg = "the server said no"
			}
			d.Errors[section] = msg
			return true
		}
		return false
	}

	if in.agendaErr != nil && in.clockErr != nil && in.focusErr != nil && in.habitsErr != nil &&
		in.inboxErr != nil && in.noticeErr != nil {
		d.Unreachable = in.server
		if d.Unreachable == "" {
			d.Unreachable = "the server"
		}
		d.Errors["server"] = in.agendaErr.Error()
		return d
	}
	if !fail("today", in.agendaErr, in.agenda.Ok, in.agenda.Msg) {
		if in.agenda.Today != "" {
			d.Date = in.agenda.Today
		}
		d.Today, d.Overdue = dayItems(in.agenda, d.Date)
	}
	for _, it := range d.Today {
		switch {
		case it.Done:
			d.Summary.Done++
		case it.HasTime:
			d.Summary.Appointments++
		default:
			d.Summary.ToDo++
		}
	}
	d.Summary.Overdue = len(d.Overdue)

	if in.focusErr == nil && in.focus.Ok && in.focus.Phase != "" && in.focus.Phase != "idle" {
		f := in.focus
		d.Focus = &f
	}
	if in.clockErr != nil {
		d.Errors["clock"] = in.clockErr.Error()
	} else if in.clock.Active {
		title := in.clock.Target.Id
		if title == "" {
			title = baseName(in.clock.Target.Filename)
		}
		start := in.clock.Time.Start
		d.Clock = &ClockNow{Title: lastOlp(title), File: in.clock.Target.Filename, Started: start,
			Mins: now.Sub(start).Minutes()}
	}

	if !fail("habits", in.habitsErr, in.habits.Ok, in.habits.Msg) {
		d.Habits = habitRows(in.habits.Habits)
		for _, h := range d.Habits {
			d.HabitsDue++
			if h.Done {
				d.HabitsDone++
			}
		}
		d.Summary.HabitsLeft = d.HabitsDue - d.HabitsDone
	}

	if !fail("inbox", in.inboxErr, in.inbox.Ok, in.inbox.Msg) {
		d.InboxCount = len(in.inbox.Items)
		for i, it := range in.inbox.Items {
			if i == 3 {
				break
			}
			d.Inbox = append(d.Inbox, it.Headline)
		}
		d.Summary.Inbox = d.InboxCount
	}

	if !fail("notices", in.noticeErr, in.notices.Ok, in.notices.Msg) {
		d.NoticeCount = in.notices.Unread
		for _, n := range in.notices.Notices {
			if n.Read {
				continue
			}
			d.Notices = append(d.Notices, NoticeRow{Label: n.Label, Gained: len(n.Gained), Count: n.Count, At: n.At})
		}
		if d.NoticeCount < len(d.Notices) {
			d.NoticeCount = len(d.Notices)
		}
		d.Summary.Notices = d.NoticeCount
	}
	if len(d.Errors) == 0 {
		d.Errors = nil
	}
	return d
}

// dayItems splits today's entries into the day's list and the overdue pile.
// Habits are left out of both: they have their own card, and a daily habit
// carried forward would otherwise sit at the top of Overdue every morning.
func dayItems(a common.AgendaResult, date string) (today, overdue []Item) {
	for _, day := range a.Days {
		if day.Date != date {
			continue
		}
		for _, e := range day.Entries {
			if e.Node < 0 || e.Node >= len(a.Nodes) || e.Kind == common.AgendaClosed {
				continue
			}
			n := a.Nodes[e.Node]
			if n.IsHabit {
				continue
			}
			it := Item{Title: n.Headline, Status: n.Status, Priority: n.Priority, Kind: e.Kind,
				HasTime: e.HasTime, Late: e.Late, DueIn: e.DueIn, Done: e.Done || n.IsDone,
				Hash: n.Hash, File: n.Filename}
			if e.HasTime {
				it.Start, it.End = clockOf(e.Start), clockOf(e.End)
			}
			if e.Late > 0 && !it.Done {
				overdue = append(overdue, it)
				continue
			}
			today = append(today, it)
		}
	}
	// Most late first: a month-old deadline matters more than yesterday's
	// scheduled item. Deadlines before scheduled at the same age, because a
	// deadline gone by is a promise broken and a scheduled date is a plan.
	sort.SliceStable(overdue, func(i, j int) bool {
		if overdue[i].Late != overdue[j].Late {
			return overdue[i].Late > overdue[j].Late
		}
		return overdue[i].Kind == common.AgendaDeadline && overdue[j].Kind != common.AgendaDeadline
	})
	return today, overdue
}

// habitRows keeps the habits that are today's business: due, or already done
// today (so ticking one does not make it vanish, which reads as lost).
func habitRows(hs []common.Habit) []HabitRow {
	out := []HabitRow{}
	for _, h := range hs {
		if !h.Due && !h.DoneToday {
			continue
		}
		out = append(out, HabitRow{Title: h.Headline, Done: h.DoneToday, Missed: h.Missed && !h.DoneToday,
			Streak: h.Streak, Week: week(h.Days)})
	}
	// Still to do first, then done; the order the file gives them inside each.
	sort.SliceStable(out, func(i, j int) bool { return !out[i].Done && out[j].Done })
	return out
}

func week(days []common.HabitDay) string {
	if len(days) > 7 {
		days = days[len(days)-7:]
	}
	b := strings.Builder{}
	for _, d := range days {
		switch d.State {
		case "done":
			b.WriteByte('d')
		case "miss":
			b.WriteByte('m')
		default:
			b.WriteByte('.')
		}
	}
	return b.String()
}

// nowIndex is where the "now" line goes among the timed entries: before the
// first one that has not started. An appointment under way stays above it.
func nowIndex(timed []Item, now string) int {
	for i, it := range timed {
		if it.Start > now {
			return i
		}
	}
	return len(timed)
}

// underway: started and not over. An entry with no end is a moment and is
// never under way.
func underway(it Item, now string) bool {
	return it.HasTime && it.End != "" && it.Start <= now && now < it.End
}

func clockOf(s string) string {
	if i := strings.IndexByte(s, 'T'); i >= 0 && len(s) >= i+6 {
		return s[i+1 : i+6]
	}
	return ""
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// A clock's target id is often the outline path "Project/Task"; the task is
// the part a person recognises.
func lastOlp(s string) string {
	parts := strings.Split(s, "|")
	return strings.TrimSpace(parts[len(parts)-1])
}
