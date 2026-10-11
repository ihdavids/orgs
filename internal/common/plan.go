package common

// Time-blocking over the wire: GET /plan/day, POST /plan/block, POST
// /plan/unblock (internal/app/orgs/planner.go).
//
// The agenda says what is on a day; the planner is the agenda you write in.
// A day is the timed blocks already on it (a SCHEDULED or plain stamp with a
// time), the open tasks that could go on it, and how full it is. Placing a
// task writes SCHEDULED: <2026-10-12 Mon 10:00-11:00> on it, as a line edit.
//
// Times are local wall clock, "HH:MM", on the day the answer is about, so a
// client never converts a time zone.

// One timed thing on the day.
type PlanBlock struct {
	Hash string
	// The headline without the stamps written in it: on a block they only
	// repeat the time it is drawn at.
	Headline string
	Filename string
	LineNum  int
	Status   string
	Priority string
	Tags     []string
	// scheduled, timestamp or deadline (the agenda's kinds).
	Kind  string
	Start string // HH:MM
	End   string // HH:MM; equal to Start for a deadline, which is a moment
	Mins  int    // End - Start
	// The stamp names no end, so End is the heading's EFFORT, else the
	// default length: what the day is assumed to lose to it.
	Implied bool
	// The planner may move, resize and unplan it: a SCHEDULED within one day,
	// not done. An appointment (a plain stamp) and a deadline are fixed;
	// they take up the day but are not the planner's to rearrange.
	Movable bool
	// This day is a repeat of the stamp rather than its own date. Moving it
	// moves the series to start here, as org does when a repeating item is
	// rescheduled.
	Repeat   bool
	Repeater string // "+1w" as written, "" for none
	Done     bool
	// The stamp as it is written in the file.
	Stamp      string
	EffortMins int // the heading's EFFORT, 0 when it has none
	// Days until a deadline (the agenda's DueIn), -1 when not warning; days
	// late for a deadline gone past.
	DueIn int
	Late  int
}

// One open task that could be placed on the day.
type PlanTask struct {
	Hash     string
	Headline string
	Filename string
	LineNum  int
	Status   string
	Priority string
	Tags     []string
	Effort   string // EFFORT as written, "" for none
	// EFFORT in minutes, 0 when there is none (a client then uses the
	// default length).
	EffortMins int
	Scheduled  string // YYYY-MM-DD when scheduled (on or before the day), else ""
	Deadline   string // YYYY-MM-DD, else ""
	// Days the SCHEDULED date is before the day asked about (0 = that day),
	// -1 when not scheduled.
	Late int
	// Days from the day asked about to the deadline (negative = past), only
	// meaningful when Deadline is set.
	DueIn    int
	Repeater string
}

// How full the day is, inside the working window.
type PlanLoad struct {
	WindowMins  int // DayEnd - DayStart
	PlannedMins int // the time inside the window covered by at least one block
	FreeMins    int // WindowMins - PlannedMins
	// Minutes covered twice or more: blocks on top of each other.
	DoubleMins int
	// Blocks wholly or partly outside the window, in minutes outside it.
	OutsideMins int
	// The EFFORT of every task still waiting to be placed, and how many of
	// them have none written.
	BacklogMins  int
	BacklogNoEst int
}

// GET /plan/day?date=YYYY-MM-DD[&query=...][&dayStart=HH:MM][&dayEnd=HH:MM][&today=YYYY-MM-DD]
type PlanDay struct {
	Ok       bool
	Msg      string
	Date     string
	Today    string
	DayStart string
	DayEnd   string
	// The query the task list came from (the default when none was asked).
	Query  string
	Blocks []PlanBlock
	// What is on the day without a time and is not a task to place: an all
	// day appointment, a deadline due or warning, a done item.
	AllDay []PlanBlock
	Tasks  []PlanTask
	Load   PlanLoad
	// Pairs of indexes into Blocks whose times overlap.
	Overlaps [][2]int
	// The length a task with no EFFORT is given, in minutes.
	DefaultMins int
}

// POST /plan/block. Start and End are HH:MM on Date. An empty End is Start
// plus the heading's EFFORT (else the default length); an empty Start is the
// first free gap long enough, from now on today or from DayStart otherwise.
type PlanBlockReq struct {
	Hash     string
	Date     string // YYYY-MM-DD; empty = today
	Start    string
	End      string
	DayStart string // the window the free-gap search uses, default 09:00
	DayEnd   string // default 17:00
}

// POST /plan/unblock. Takes SCHEDULED off the heading - or, when it repeats
// or KeepDate is set, only the time, so a series is never lost to a drag.
type PlanUnblockReq struct {
	Hash     string
	KeepDate bool
}

// The answer to a block or an unblock.
type PlanWrite struct {
	Ok    bool
	Msg   string
	Hash  string // the heading's hash after the write
	Stamp string // the SCHEDULED stamp now written, "" when removed
	Start string
	End   string
}
