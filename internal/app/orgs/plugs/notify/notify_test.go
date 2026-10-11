package notify

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/desktop"
)

// Only today's own timed, unfinished occurrences are announced, minutes are
// counted across the hour, and an overdue item carried to today is not an
// appointment at its old time.
func TestDueSoon(t *testing.T) {
	now := time.Date(2026, 10, 10, 9, 55, 0, 0, time.Local)
	res := common.AgendaResult{
		Nodes: []common.AgendaNode{
			{Todo: common.Todo{Headline: "Standup", Filename: "a.org", LineNum: 1}},
			{Todo: common.Todo{Headline: "Overdue", Filename: "a.org", LineNum: 5}},
			{Todo: common.Todo{Headline: "Deadline only", Filename: "a.org", LineNum: 9}},
			{Todo: common.Todo{Headline: "Later", Filename: "a.org", LineNum: 12}},
			{Todo: common.Todo{Headline: "Earlier", Filename: "a.org", LineNum: 15}},
		},
		Days: []common.AgendaDay{{Date: "2026-10-10", Entries: []common.AgendaEntry{
			{Node: 0, Kind: common.AgendaTimestamp, Start: "2026-10-10T10:00", HasTime: true, DueIn: -1},
			{Node: 1, Kind: common.AgendaScheduled, Start: "2026-10-10", Late: 3, DueIn: -1},
			{Node: 2, Kind: common.AgendaDeadline, Start: "2026-10-10", DueIn: 4},
			{Node: 3, Kind: common.AgendaTimestamp, Start: "2026-10-10T11:00", HasTime: true, DueIn: -1},
			{Node: 4, Kind: common.AgendaTimestamp, Start: "2026-10-10T09:00", HasTime: true, DueIn: -1},
		}}},
	}
	soon, started := dueSoon(res, now, 10)
	if len(soon) != 1 || soon[0].Headline != "Standup" || soon[0].Mins != 5 {
		t.Fatalf("soon = %+v, want only Standup in 5 minutes (across the hour)", soon)
	}
	if len(started) != 1 {
		t.Fatalf("started = %v, want only Earlier", started)
	}
}

// One reminder per reminder time; crossing two at once (a server started a
// minute before) sends only the later one; nothing twice.
func TestReminderDue(t *testing.T) {
	offsets := []int{10, 2}
	sent := map[int]bool{}
	was := func(o int) bool { return sent[o] }
	if _, ok := reminderDue(15, offsets, was); ok {
		t.Fatal("15 minutes out is before any reminder")
	}
	if o, ok := reminderDue(9, offsets, was); !ok || o != 10 {
		t.Fatalf("9 minutes out: the 10 minute reminder, got %d %v", o, ok)
	}
	sent[10] = true
	if _, ok := reminderDue(8, offsets, was); ok {
		t.Fatal("the 10 minute reminder went out once")
	}
	if o, ok := reminderDue(2, offsets, was); !ok || o != 2 {
		t.Fatalf("2 minutes out: the 2 minute reminder, got %d %v", o, ok)
	}
	// Started late: both crossed, only the later one.
	if o, ok := reminderDue(1, offsets, func(int) bool { return false }); !ok || o != 2 {
		t.Fatalf("crossing both sends the 2 minute one, got %d %v", o, ok)
	}
	// The later one sent: the earlier one is not sent after it.
	if _, ok := reminderDue(1, offsets, func(o int) bool { return o == 2 }); ok {
		t.Fatal("nothing after the last reminder")
	}
}

func TestWhenWords(t *testing.T) {
	for mins, want := range map[int]string{0: "now", 1: "in 1 minute", 10: "in 10 minutes", 60: "in 1 h", 75: "in 1 h 15 min"} {
		if got := whenWords(mins); got != want {
			t.Errorf("%d: %q, want %q", mins, got, want)
		}
	}
}

func TestReminderNote(t *testing.T) {
	u := upcoming{Key: "k", Headline: "Standup", Mins: 10, File: "/n/work.org", Line: 3,
		Start: time.Date(2026, 10, 11, 10, 0, 0, 0, time.Local), End: time.Date(2026, 10, 11, 10, 15, 0, 0, time.Local)}
	n := reminderNote(u, true, "http://localhost:8010")
	if n.Title != "Standup" || n.Subtitle != "in 10 minutes" || n.Body != "10:00–10:15 · work" || !n.Sound || n.Group != "remind:k" {
		t.Fatalf("note: %+v", n)
	}
	if !strings.Contains(n.URL, "app=Files") || !strings.Contains(n.URL, "line=3") {
		t.Fatalf("a click opens the heading: %s", n.URL)
	}
}

// Overdue first (most overdue first), then due today, then coming; finished
// and repeated entries once.
func TestDeadlineDigest(t *testing.T) {
	res := common.AgendaResult{
		Nodes: []common.AgendaNode{
			{Todo: common.Todo{Headline: "Taxes"}}, {Todo: common.Todo{Headline: "Report"}},
			{Todo: common.Todo{Headline: "Renew passport"}}, {Todo: common.Todo{Headline: "Done thing"}},
			{Todo: common.Todo{Headline: "Ancient"}},
		},
		Days: []common.AgendaDay{{Date: "2026-10-11", Entries: []common.AgendaEntry{
			{Node: 1, Kind: common.AgendaDeadline, DueIn: 0},
			{Node: 0, Kind: common.AgendaDeadline, Late: 2, DueIn: -1},
			{Node: 2, Kind: common.AgendaDeadline, DueIn: 3},
			{Node: 3, Kind: common.AgendaDeadline, DueIn: 0, Done: true},
			{Node: 4, Kind: common.AgendaDeadline, Late: 9, DueIn: -1},
			{Node: 1, Kind: common.AgendaDeadline, DueIn: 0},
		}}},
	}
	items := deadlinesDue(res)
	var names []string
	for _, i := range items {
		names = append(names, i.Headline)
	}
	if got := strings.Join(names, ","); got != "Ancient,Taxes,Report,Renew passport" {
		t.Fatalf("order: %s", got)
	}
	n := digestNote(items, "http://h")
	if n.Title != "Deadlines: 2 overdue, 1 due today, 1 coming up" || !n.Sound || !strings.Contains(n.URL, "app=Agenda") {
		t.Fatalf("digest: %+v", n)
	}
}

// Quiet hours hold back a popup, unless it is critical; desktop off holds
// back everything.
func TestDeliverHonoursQuietAndDesktop(t *testing.T) {
	quiet, _ := common.ParseQuiet("22:00-07:00")
	night := func() time.Time { return time.Date(2026, 10, 11, 23, 0, 0, 0, time.Local) }
	n := &Notify{quiet: quiet, now: night}
	if popped, _ := n.Popped(desktop.Note{Title: "x"}); popped {
		t.Fatal("quiet hours: no popup")
	}
	off := false
	if popped, _ := (&Notify{Desktop: &off}).Popped(desktop.Note{Title: "x", Urgency: desktop.Critical}); popped {
		t.Fatal("desktop off: no popup at all")
	}
	if got := (&Notify{NotifyBeforeMins: 5}).offsets(); len(got) != 1 || got[0] != 5 {
		t.Fatalf("the old setting is still read: %v", got)
	}
	if got := (&Notify{Remind: []int{2, 10, 2}}).offsets(); len(got) != 2 || got[0] != 10 || got[1] != 2 {
		t.Fatalf("largest first, no repeats: %v", got)
	}
}
