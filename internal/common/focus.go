package common

// The wire types of a focus (pomodoro) session, shared by the server
// (internal/app/orgs/focus.go) and `orgs focus`. worg's copy is
// worg/src/focusApi.ts.

// FocusSettings are the lengths of a session's phases. Minutes are floats so a
// test (or somebody curious) can run a whole work, break, idle cycle in seconds
// with `Work: 0.1`.
type FocusSettings struct {
	Work  float64 // minutes of work, 25 by default
	Short float64 // a short break, 5
	Long  float64 // a long break, 15
	// Every how many pomodoros the break is a long one, 4 by default.
	LongEvery int
	// Start the next pomodoro on the same task when a break ends, rather than
	// stopping and waiting to be asked.
	AutoNext bool
}

// FocusStart is what POST /focus/start takes. Hash names the heading; Target
// is the longer form for a client that has a file and outline path instead.
// A zero length means "what I used last time", which is 25/5/15/4 until
// somebody changes it.
type FocusStart struct {
	Hash      string
	Target    *Target `json:",omitempty"`
	Work      float64
	Short     float64
	Long      float64
	LongEvery int
	AutoNext  *bool `json:",omitempty"`
}

// FocusState is what the session is doing now.
type FocusState struct {
	Ok  bool
	Msg string `json:",omitempty"`
	// work, break or idle.
	Phase string
	// The break under way is a long one.
	LongBreak bool `json:",omitempty"`
	// The task. When idle these name the last task worked on, so a client can
	// offer "another one on this".
	Hash  string `json:",omitempty"`
	Title string `json:",omitempty"`
	File  string `json:",omitempty"`
	// RFC3339. Empty when idle.
	Started string `json:",omitempty"`
	Ends    string `json:",omitempty"`
	// Seconds left and seconds in the phase, worked out on the server, so a
	// client whose own clock is wrong still counts down the right amount.
	Remaining float64
	Total     float64
	// The server's time when it answered, for the same reason.
	Now string
	// Pomodoros finished since the last long break, and today.
	Streak       int
	Today        int
	TodayMinutes float64
	Settings     FocusSettings
}

// FocusRecord is one work phase that happened: finished, or stopped early.
type FocusRecord struct {
	Hash      string
	Title     string
	File      string
	Start     string // RFC3339
	End       string
	Minutes   float64
	Completed bool
	// The break that followed it, in minutes; zero when it was stopped.
	Break     float64 `json:",omitempty"`
	LongBreak bool    `json:",omitempty"`
}

// FocusHistory is GET /focus/history: the work phases of the days asked for,
// newest first, and today's totals.
type FocusHistory struct {
	Ok           bool
	Today        int
	TodayMinutes float64
	Records      []FocusRecord
}
