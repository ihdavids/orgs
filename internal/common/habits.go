package common

// Habits, and how they are going.
//
// A habit is a heading with `:STYLE: habit` and a repeating schedule. Orgs has
// been able to say whether one exists for a long time; what it could not say is
// the thing anybody actually wants to know, which is *how it is going* - and it
// could not say it because nothing in orgs wrote the history down. Marking
// something done now leaves a `State "DONE"` line in the logbook, so the history
// is there to read, and this is it read.

// One day of a habit's recent history.
//
// Three states rather than two, because "not done" means two different things.
// A daily habit not done yesterday is a broken run; a habit with three days of
// slack not done yesterday is simply a day you had in hand. Drawing them the
// same way makes a well-kept habit look like a failing one.
type HabitDay struct {
	Date string `json:"d"`
	// "done" - a completion is recorded that day.
	// "miss" - as of that day more time had passed than the habit allows.
	// "ok"   - neither: inside the grace the habit's own repeater grants.
	State string `json:"s"`
}

// One habit, and everything a tracker needs to draw it.
type Habit struct {
	Hash     string
	Headline string
	Filename string
	LineNum  int
	Status   string
	Tags     []string

	// The repeater as written (`.+1d/3d`), how many days between occurrences,
	// and how many days may pass before the run is broken - which is the slack
	// half where the habit has one and the interval itself where it does not.
	Repeater string
	Every    int
	Slack    int
	// The next time it comes round, as the file says.
	Scheduled string

	// How it is going.
	Streak    int
	Best      int
	Total     int
	DoneToday bool
	Due       bool
	Missed    bool
	LastDone  string
	// Completions in the window against what the cadence expected there, 0..1.
	Rate float64

	// The window, oldest first, one entry per day up to and including today.
	Days []HabitDay

	// The keyword to write to tick it off, from this heading's own file - so a
	// tracker can offer the button without a request per habit.
	DoneKeyword string
}

// The answer to GET /habits.
type HabitsResult struct {
	Ok   bool
	Msg  string
	Days int
	// Today as the server reckons it, so a client in another timezone draws the
	// same last column rather than one that is off by one for half the day.
	Today  string
	Habits []Habit

	// The header's summary: how many want doing today and how many are done.
	DueToday  int
	DoneToday int
	// The longest run anybody is currently on, which is the number worth putting
	// at the top of a tracker.
	BestStreak int
}

// ---------------------------------------------------------------------------
// Taking today's tick off again
// ---------------------------------------------------------------------------

// HabitUntick names the habit whose completion for today should come off.
//
// Not an undo: nothing is remembered and nothing has to have happened in this
// process, or even on this machine. It is a question asked of the file - is there
// a completion recorded for today? - and the answer either exists in the logbook
// or does not. That makes it work on a habit ticked off in Emacs an hour ago, on a
// habit ticked off twice, and after the server has been restarted, none of which
// an undo buffer can manage.
type HabitUntick struct {
	Hash string
}

// HabitUntickResult says what came off.
type HabitUntickResult struct {
	Ok  bool
	Msg string
	// The habit, so a client can name it without a second read.
	Headline string
	// How many completion lines came out. More than one is a habit ticked twice
	// in a day, which counts as one day and is cleared as one day.
	Removed int
	// Whether the repeating date was put back, and where it now stands.
	Moved     bool
	Scheduled string
	// The keyword the heading carries now.
	Status string
}
