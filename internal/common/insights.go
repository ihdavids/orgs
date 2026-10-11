package common

// Wire types for the Insights endpoints: how far estimates are off
// (/insights/estimates) and the health of the dependencies between headings
// (/insights/deps). The server half is internal/app/orgs/insights.go.

// One task measured: what it was meant to take against what it took.
type EstimateTask struct {
	Hash     string
	Headline string
	Filename string
	LineNum  int
	Status   string
	// Done tasks are the measurement; an open task with time on it is counted
	// only when asked for, and flagged, because its actual is still growing.
	Done       bool
	InProgress bool
	Effort     string
	EstMins    float64
	ActualMins float64
	// ActualMins / EstMins. Zero when either side is missing.
	Ratio   float64
	Closed  string `json:",omitempty"`
	Tags    []string
	Project string
	Person  string `json:",omitempty"`
}

// One row of the report: every measured task in a tag, project, file or
// person's name.
type EstimateGroup struct {
	Name     string
	Kind     string // tag, project, file, person
	Hash     string `json:",omitempty"` // the project heading, for kind project
	Filename string `json:",omitempty"`
	Tasks    int
	// How many of Tasks are still open (only with open=t).
	InProgress int
	EstMins    float64
	ActualMins float64
	// The group's total actual over its total estimate.
	Ratio float64
	// The per-task ratios: the middle one and the middle half around it. A
	// group whose median is 1 and whose spread is 0.3 to 3 is not estimated
	// well, it is estimated badly in both directions.
	Median float64
	P25    float64
	P75    float64
	// What to multiply an estimate by, read off the median. Zero when the
	// group has too few tasks to say.
	Multiplier float64
	Sentence   string
	// The group's worst misses, either way, worst first.
	Worst []EstimateTask
}

type EstimateReport struct {
	Ok    bool
	Msg   string `json:",omitempty"`
	Query string
	By    string
	From  string `json:",omitempty"`
	To    string `json:",omitempty"`
	Label string
	// Hours a day of EFFORT stands for. Org's own is 24; a working day is 8.
	DayHours float64
	// Every measured task together, as one group.
	All    EstimateGroup
	Groups []EstimateGroup
	// The worst over- and under-runs over every group, worst first.
	Over  []EstimateTask
	Under []EstimateTask
	// What could not be measured, counted rather than dropped.
	ClockNoEffort     int
	ClockNoEffortMins float64
	EffortNoClock     int
	// Headings with an EFFORT whose subtree holds other estimated headings:
	// their effort is a sum of the others, so measuring it again counts the
	// same work twice.
	Rollups int
	// How many measured estimates are written in days or weeks, which is when
	// DayHours matters.
	DayEfforts int
	// The tasks with the most time on them and no estimate at all.
	Unestimated []EstimateTask
}

// A heading taking part in a dependency problem.
type DepHeading struct {
	Hash     string
	Headline string
	Filename string
	LineNum  int
	Status   string
	Done     bool
	// How this heading waits on the one before it in the chain: "AFTER",
	// "ORDERED", or empty for the first.
	Via string `json:",omitempty"`
	// What a client needs to take the wait away (worg's unlinkOp): the AFTER
	// it holds, its NOBLOCKING, and whether an ORDERED parent would chain it
	// anyway once AFTER is gone.
	After      string `json:",omitempty"`
	NoBlocking string `json:",omitempty"`
	Chained    bool   `json:",omitempty"`
}

type DepProblem struct {
	// cycle, waiting-on-done, blocked-chain, dangling, ordered-stuck
	Kind string
	// error, warn, info
	Severity   string
	Title      string
	Suggestion string
	// In the order the work would be done: what is waited on comes first. A
	// cycle ends where it started.
	Chain []DepHeading
	// blocked-chain: the headings in the longest chain, root blocker
	// included, and how many open headings it holds up altogether.
	Length int `json:",omitempty"`
	Held   int `json:",omitempty"`
	// dangling: the AFTER that names nothing (or something archived).
	Missing string `json:",omitempty"`
}

type DepReport struct {
	Ok       bool
	Msg      string `json:",omitempty"`
	Query    string
	Checked  int
	Problems []DepProblem
	Counts   map[string]int
}
