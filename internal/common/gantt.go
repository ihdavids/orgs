package common

// The wire types behind /gantt/tasks and /gantt/add.
//
// The mermaid exporter answers with a picture of a schedule. These answer with
// the schedule itself - every heading the query found, what it depends on, how
// long it is meant to take, and the hash to address it by - which is what a
// client needs to *edit* a plan rather than only look at one. The dates are not
// laid out here: a task says when it starts or what it comes after, and the
// client resolves the chain, the same way mermaid's own renderer does with the
// source the exporter writes.

// One heading, as a gantt chart sees it.
type GanttTask struct {
	Hash     string
	Headline string
	Filename string
	LineNum  int
	Level    int
	Parent   string
	Status   string
	// The letter of the heading's [#A] cookie, "" when it has none.
	Priority string
	Tags     []string
	Props    map[string]string

	// The swim lane and the person, worked out the same way the mermaid
	// exporter works them out (SECTION / ASSIGNED / RID / RESOURCEID, falling
	// back to a parent tagged :project:).
	Section  string
	Resource string

	// When the heading says when it happens. Empty when it does not, which is
	// what makes a task's place in the chart depend on what it comes After.
	Start string // YYYY-MM-DD
	End   string // YYYY-MM-DD, only when the timestamp is a range

	// Which of the heading's dates Start came off, and so which one a client
	// writing a new date should write: "SCHEDULED", "TIMESTAMP", or empty for a
	// heading that has neither yet.
	DateKind string

	// EFFORT as it is written, and as a number of days. Zero days means the
	// heading has no effort on it and the chart should draw the default bar.
	Effort     string
	EffortDays float64

	// What this comes after: the hash of the heading the AFTER property or an
	// ORDERED parent puts in front of it, and the id that was written to say so.
	After   string
	AfterId string
	Ordered bool
	// An ORDERED parent would put this heading behind something if it had no
	// AFTER; NOBLOCKING is how it is taken out of that chain.
	Chained bool

	// The bar's look, all of it optional and read off the heading: PERCENTDONE,
	// GANTT_COLOR (or COLOR), and GANTT_ORDER, which is the only thing that
	// decides row order when it is set.
	Percent  int
	Color    string
	Order    float64
	HasOrder bool

	// The flags the exporter writes as mermaid tags, worked out from the
	// properties and the todo keyword together.
	Crit      bool
	Active    bool
	Done      bool
	Milestone bool
	Mark      bool

	IsActive bool

	// True for a heading the query did not find, pulled in because something
	// the query did find comes after it. It is drawn so the chain reads, but it
	// is not part of the answer to the question that was asked.
	Implied bool

	// The time actually clocked on this heading itself, one entry per CLOCK
	// line, oldest first: what was done, to draw against what was planned.
	Clocked []ClockSpan
}

// One stretch of clocked time. End is empty while the clock is still running.
type ClockSpan struct {
	Start string // YYYY-MM-DDTHH:MM, local time
	End   string
	Mins  float64
}

// The answer to /gantt/tasks.
type GanttData struct {
	Ok    bool
	Msg   string
	Query string
	Tasks []GanttTask
}

// A new heading, written under a parent or at the end of a file.
type GanttAdd struct {
	// One of these says where it goes. AfterHash puts it straight after that
	// heading (and everything under it), at the same level; ParentHash puts
	// it under that heading, last; Filename puts it at the end of that file.
	AfterHash  string
	ParentHash string
	Filename   string

	Headline string
	Status   string
	// YYYY-MM-DD, written as the kind of date asked for ("SCHEDULED" by
	// default, "TIMESTAMP" for a heading that should carry a plain stamp).
	Start    string
	DateKind string
	Effort   string
	Tags     []string
	Props    map[string]string
}

// GanttMarkerDay is one end of a marker's date: a day, and optionally a time,
// and optionally the time a span of that day ends (10:00-11:00).
type GanttMarkerDay struct {
	Date  string // YYYY-MM-DD
	Time  string // HH:MM, or empty for the whole day
	Until string // HH:MM, the end of a span within the day, or empty
}

// GanttMarkerEdit adds, changes or deletes a marker: a heading whose date the
// gantt chart draws as a line (one day) or a band (a range of days).
//
// Hash names the marker to change or delete; without it a new one is added
// under ParentHash, or at the end of Filename. Kind says which of the
// heading's dates is the marker's: "date" (its own timestamp, or SCHEDULED
// when that is all it has) or "deadline". A range is always written as the
// heading's own timestamp, <a>--<b>, which is the only place org keeps one.
type GanttMarkerEdit struct {
	Hash       string
	ParentHash string
	Filename   string
	Delete     bool

	Headline string
	// Tags replace the heading's own tags when SetTags is true (so an edit
	// that does not mention tags leaves them alone).
	Tags    []string
	SetTags bool

	Kind  string
	Start GanttMarkerDay
	// Set for a range; nil for a single day.
	End *GanttMarkerDay
	// The repeater, as org writes it ("+1w", ".+2d/4d"), put into each
	// stamp; "" for a date that does not repeat.
	Repeat string
}

// ---------------------------------------------------------------------------
// /gantt/suggest and /gantt/link: setting a chart up from a pile of tickets.
// ---------------------------------------------------------------------------

// GanttSuggestReq asks for what a chart could be told that it is not yet: the
// dependencies its tasks' own text names, the estimates history can guess, and
// the ways its tasks could be split into swim lanes.
type GanttSuggestReq struct {
	// The chart's query: the tasks are exactly what /gantt/tasks finds for it.
	Query string
	// Any of "deps", "estimates", "lanes", "ratios"; empty asks for all.
	// "deps" also answers Redundant and Cycles.
	Kinds []string
	// Hours in a day of effort. Default 8.
	DayHours float64
	// The start of the history estimates and ratios are read from, as typed
	// anywhere else (-180d, 2026-01-01). Default "-180d".
	From string
}

// A dependency the text suggests: Task waits on After.
type GanttDepSuggestion struct {
	Task       string // hash
	After      string // hash
	AfterTitle string
	// The phrase or line it came from, trimmed to about 120 characters.
	Evidence string
	// "text", "link", "key", "jira" or "sequence".
	Source     string
	Confidence float64 // 0..1
}

// An existing link that the chart would keep without: Task waits on After,
// and the path through Via already says so.
type GanttRedundantLink struct {
	Task string
	// The hash the link names.
	After string
	// The hashes of the longer path that already implies it, in wait order
	// (Task waits on Via[0], ..., the last waits on After). Empty when another
	// link says exactly the same thing (an AFTER naming the heading the
	// ORDERED parent would chain it behind anyway).
	Via []string
	// Only an AFTER can be taken off; an ORDERED chain is the outline.
	Removable bool
}

// A guess at the EFFORT of an open task that has none.
type GanttEstimateGuess struct {
	Task string // hash
	// An org effort in DayHours units: "45min", "1.5h", "2d". Always set.
	Effort string
	// What Effort comes to in working minutes, a day being DayHours: "2d"
	// at the default 8 hours is 960, not org's 2880.
	Mins float64
	// Where the number came from, as a sentence.
	Basis      string
	Samples    int
	Confidence float64
}

// One way of splitting a chart's tasks into lanes.
type GanttLaneSuggestion struct {
	// "tag", "parent", "file", "chain" or "property".
	By string
	// The property's name when By is "property" (ASSIGNED, COMPONENT...).
	Prop   string
	Label  string
	Score  float64
	Groups []GanttLaneGroup
	// Dependency edges (existing and suggested) between tasks in different
	// groups: fewer means lanes that can be read on their own.
	CrossLinks int
}

type GanttLaneGroup struct {
	// "(none)" holds the tasks with no value.
	Name  string
	Tasks []string // hashes
}

// The answer to /gantt/suggest. Every list is present (possibly empty) for
// the kinds asked for.
type GanttSuggest struct {
	Ok    bool
	Msg   string
	Query string

	Deps      []GanttDepSuggestion
	Redundant []GanttRedundantLink
	// Existing dependency cycles, each as hashes in wait order: each waits on
	// the next, and the last on the first.
	Cycles    [][]string
	Estimates []GanttEstimateGuess
	Lanes     []GanttLaneSuggestion
	// actual/estimated of every finished task in the window with both an
	// EFFORT and clocked time (absurd ones, under 0.05 or over 20, left out),
	// for a client-side forecast.
	Ratios []float64
}

// GanttLinkReq makes Task wait on After (by AFTER, naming After by its ID,
// which it is given if it has none), or with Remove, stops it waiting on it.
// AFTER holds one heading, so adding to a task whose AFTER already names
// another is refused: Remove that one first.
type GanttLinkReq struct {
	Task   string // hash
	After  string // hash
	Remove bool
}
