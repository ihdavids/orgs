package common

// The agenda over the wire: GET /agenda and GET /agenda/nodes.
//
// /search answers "which headings match"; it cannot answer "what is on
// Tuesday", because that needs the dates worked out: a repeater expanded to
// each day it comes round on, a deadline's warning period, a scheduled item
// carried forward until it is done, a range spread over the days it spans.
// Before these endpoints every client did that itself (or, like OnDate(),
// did not do it at all), so the agendas disagreed. Now the server says what
// lands on each day and a client only draws it.
//
// The rules (internal/app/orgs/agenda.go, pinned by agenda_test.go) follow
// org-mode's agenda:
//
//   - A plain active timestamp, in the headline or the heading's own body
//     (not a child's, not inside a drawer or the planning line), lands on its
//     day. A repeater (+1w, ++1w, .+1w alike) lands on its base day and every
//     interval after it, never before it.
//   - A range <a>--<b> lands on every day it spans; on the first and last day
//     with the times written there, on the days between as all day.
//   - SCHEDULED lands on its day (and each repeat). When it is not done and
//     its day has passed, it also lands on *today* (Late = days since), and
//     only today: an overdue item is a today problem, not a yesterday one.
//   - DEADLINE lands on its day (and each repeat). When it is not done it
//     also lands on today while today is inside its warning period (DueIn =
//     days left, from -Nd on the stamp, else agenda.deadlineWarningDays,
//     default 14) or past it (Late = days overdue).
//   - A finished heading (a done keyword of its own file) lands only on the
//     exact days its dates name: no carrying forward, no warnings. Done is
//     set so a client can draw it struck through or leave it out.
//   - CLOSED lands on its day only when the request asks (closed=t): the
//     logbook view.
//   - A heading in an archived tree (ARCHIVE, here or above) is left out
//     unless the request asks (archived=t), as org's agenda skips them.
//   - Every occurrence has the heading it belongs to by index into Nodes, so
//     a heading on five days is sent once.
//
// "Today" is the server's local date unless the request names one (today=),
// which tests and clients in another time zone use.

// A date as the agenda needs it. Times are local wall clock, written
// YYYY-MM-DDTHH:MM (or YYYY-MM-DD when HasTime is false), so a client never
// converts a time zone.
type AgendaDate struct {
	Start   string
	End     string // empty for a point in time
	HasTime bool
	Active  bool
	// The repeater as written: "+1w", ".+2d", "++1m", or "" for none.
	Repeat string
	// The warning period as written: "-3d", or "".
	Warn string
	// The stamp as it appears in the file, <2026-10-05 Mon 10:00 +1w>.
	Text string
}

// One clock line.
type AgendaClock struct {
	Start string // YYYY-MM-DDTHH:MM
	End   string // empty while the clock is running
	Mins  float64
}

// What a client needs to know about a heading's children without being sent
// them: enough to tell a project from a task and a stuck project from a
// moving one.
type AgendaChildren struct {
	Count int      // direct children
	Todo  int      // direct children with an active (not done) keyword
	Done  int      // direct children with a done keyword
	// The keywords of the direct children, in order, "" for none.
	States []string
}

// A heading with everything the agenda reads about it. Todo is embedded, so
// everything /search sends is here too, by the same names.
type AgendaNode struct {
	Todo
	Olp        []string // titles from the top of the file down to this heading
	Scheduled  *AgendaDate
	Deadline   *AgendaDate
	Closed     *AgendaDate
	Timestamps []AgendaDate // the heading's own active timestamps, in file order
	Clocks     []AgendaClock
	ClockMins  float64 // all of this heading's own clock lines
	IsTodo     bool    // an active keyword of its own file's #+TODO
	IsDone     bool    // a finished keyword of its own file's #+TODO
	IsArchived bool    // tagged ARCHIVE, here or above
	IsHabit    bool    // :STYLE: habit
	// The heading's tags plus every tag above it (#+FILETAGS included), the
	// way org matches a tag search. Todo.Tags is the heading's own.
	AllTags  []string
	Children AgendaChildren
	// Dates of completions recorded in the logbook (habit history), oldest
	// first, YYYY-MM-DD. Present for habits only.
	DoneDates []string
	// The file's keywords, active and finished, so a client can offer them.
	TodoKeys []string
	DoneKeys []string
}

// The kinds of occurrence.
const (
	AgendaTimestamp = "timestamp"
	AgendaScheduled = "scheduled"
	AgendaDeadline  = "deadline"
	AgendaClosed    = "closed"
)

// One heading landing on one day.
type AgendaEntry struct {
	Node int    // index into AgendaResult.Nodes
	Kind string // timestamp, scheduled, deadline, closed
	// When it is on this day: for a timed entry the day's start and end
	// (a range clipped to the day), else the day itself.
	Start   string
	End     string
	HasTime bool
	// Set when this day is a repeat of the stamp rather than its own date.
	Repeat bool
	// Days overdue: a scheduled item carried forward to today, or a deadline
	// gone past. 0 otherwise.
	Late int
	// Days until a deadline, while today is inside its warning period. -1
	// when not warning (0 is due today).
	DueIn int
	// The heading is finished (see IsDone on the node).
	Done bool
}

// One day of the agenda, entries sorted: timed ones by time, then all day
// ones by kind (deadline, scheduled, timestamp), then priority, then title.
type AgendaDay struct {
	Date    string // YYYY-MM-DD
	Entries []AgendaEntry
}

// GET /agenda?from=YYYY-MM-DD&to=YYYY-MM-DD[&query=...][&today=YYYY-MM-DD]
// [&closed=t][&archived=t]. from and to are inclusive; to defaults to from, from to today.
// query narrows the headings considered (the query language, filters
// allowed); without one every heading is. Days with nothing on them are
// still listed, so a client can draw an empty day.
type AgendaResult struct {
	Ok    bool
	Msg   string
	From  string
	To    string
	Today string
	Nodes []AgendaNode
	Days  []AgendaDay
}

// GET /agenda/nodes?query=...: the headings a query finds, with everything
// the agenda reads about them, for list views (todo lists, projects, the
// clocked view) that are not laid out by day.
type AgendaNodes struct {
	Ok    bool
	Msg   string
	Nodes []AgendaNode
}
