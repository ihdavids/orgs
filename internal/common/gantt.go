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
	// One of these says where it goes. ParentHash puts it under that heading;
	// Filename puts it at the end of that file.
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
