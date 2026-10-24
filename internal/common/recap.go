package common

// GET /recap: what happened over a span of days, and what stands in the way
// now - the raw material of `orgs standup` and `orgs recap`.

// RecapItem is one heading as a recap mentions it. Headline is the title as a
// person reads it: emphasis markers gone, a link as its description.
type RecapItem struct {
	Headline string
	Hash     string
	Filename string
	LineNum  int
	Status   string   `json:",omitempty"`
	Tags     []string `json:",omitempty"`
	// The nearest project at or above the heading, else "" (the file then
	// stands for it: see Filename).
	Project string `json:",omitempty"`
	// Closed: the day it was finished (YYYY-MM-DD); today's: the time it is
	// on (HH:MM) when it has one.
	When string `json:",omitempty"`
	// Closed: how many completions fell in the range (a repeating task or a
	// habit done on three days is one item, Count 3). Cancelled: closed on a
	// cancelling keyword.
	Count     int  `json:",omitempty"`
	Repeat    bool `json:",omitempty"`
	Habit     bool `json:",omitempty"`
	Cancelled bool `json:",omitempty"`
	// Clocked: minutes in the range. Running clock: minutes so far.
	Mins float64 `json:",omitempty"`
	// Today's: scheduled, deadline, timestamp (an appointment), overdue (a
	// past SCHEDULED carried to today), next (on a NEXT keyword, nothing
	// dated today).
	Kind string `json:",omitempty"`
	// Overdue: days late. Deadline: days until due (0 = today), -1 none.
	Late  int `json:",omitempty"`
	DueIn int `json:",omitempty"`
	// Waiting: whole days it has waited (-1 unknown), and what that was read
	// from (logbook, created, date) - the weekly review's rules.
	Days      int    `json:",omitempty"`
	SinceFrom string `json:",omitempty"`
}

// RecapGroup is the clocked time of one project (or of one file, for
// headings in no project), biggest heading first.
type RecapGroup struct {
	Name     string
	Kind     string // project, file
	Hash     string `json:",omitempty"`
	Filename string
	Mins     float64
	Items    []RecapItem
}

// RecapWriting is words written in the range, from writing mode's log: net
// words per day (a day of cutting counts against it, as worg's own sums do).
// Goal is the weekly or monthly goal when the range is that long, else the
// daily goal times the days; 0 is no goal.
type RecapWriting struct {
	Words    int
	Days     int // days with any words written
	GoalDays int // days that met the daily goal
	Goal     int
	GoalKind string `json:",omitempty"` // daily, weekly, monthly
	Files    []RecapWords
}

type RecapWords struct {
	File  string
	Words int
}

// RecapHabit is one habit's completions in the range.
type RecapHabit struct {
	Headline string
	Hash     string
	Filename string
	Done     int // distinct days completed in the range
	Days     int // days in the range
}

type Recap struct {
	Ok    bool
	Msg   string `json:",omitempty"`
	From  string // YYYY-MM-DD, inclusive
	To    string // YYYY-MM-DD, inclusive
	Today string
	// What was finished, newest first; habits are in Habits instead.
	Closed []RecapItem
	// Clocked time by project, biggest first, and the whole of it.
	Clocked     []RecapGroup
	ClockedMins float64
	// A clock with no end, wherever it is.
	Running *RecapItem `json:",omitempty"`
	// What is on today: dated today or overdue, then NEXT tasks.
	Agenda []RecapItem
	// WAITING (the review's waiting query), longest first.
	Waiting []RecapItem
	// Held-up work from /insights/deps: blocked-chain, ordered-stuck and
	// waiting-on-done problems.
	Blocked []DepProblem
	Habits  []RecapHabit
	Writing *RecapWriting `json:",omitempty"`
}
