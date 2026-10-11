package common

// Saved-query notifications: "tell me when this query gains a match".
//
// The wire types for /watches and /notifications, here rather than beside the
// handlers so the terminal (which cannot import internal/app/orgs) reads the
// same shapes the server writes. Keys are lowercase, like the event stream's,
// because a notice travels in a `querymatch` event as well as in a list.

// QueryWatch is one thing being watched, kept per user in the extensions file.
type QueryWatch struct {
	// What the watch is called, and how a client names it to change or remove
	// it. For a saved query it is that query's name unless said otherwise.
	Name string `yaml:"name" json:"name"`
	// What a notification calls it; the name when empty.
	Label string `yaml:"label,omitempty" json:"label,omitempty"`
	// A saved query by name, read afresh at every check so editing the saved
	// query edits what is watched. Wins over Query.
	StoredQuery string `yaml:"storedQuery,omitempty" json:"storedQuery,omitempty"`
	// An expression of its own, for a watch with no saved query behind it.
	Query string `yaml:"query,omitempty" json:"query,omitempty"`
	// Also say when a heading stops matching ("the BLOCKED task is unblocked").
	Left bool `yaml:"left,omitempty" json:"left,omitempty"`
	// Also pop a notification on the server's desktop through the notify
	// plugin, when one is configured. Events and the notification list are
	// always written.
	Desktop bool `yaml:"desktop,omitempty" json:"desktop,omitempty"`
	// Kept but not checked.
	Paused bool `yaml:"paused,omitempty" json:"paused,omitempty"`
}

// Title is what a person sees the watch called.
func (w QueryWatch) Title() string {
	if w.Label != "" {
		return w.Label
	}
	return w.Name
}

// WatchMatch is one heading a watched query found.
type WatchMatch struct {
	// Who the heading is across edits: its ID or CUSTOM_ID when it has one,
	// else its file and outline path. Not the hash, which moves whenever
	// anything above it in the file is edited.
	Key      string   `json:"key"`
	Headline string   `json:"headline"`
	Status   string   `json:"status,omitempty"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Hash     string   `json:"hash,omitempty"`
	Path     []string `json:"path,omitempty"`
}

// WatchNotice is one notification: what a watch gained (and lost) at one check.
type WatchNotice struct {
	Id    string `json:"id"`
	Watch string `json:"watch"`
	Label string `json:"label"`
	At    string `json:"at"`
	// The headings that started matching, and those that stopped (only for a
	// watch asking for them).
	Gained []WatchMatch `json:"gained,omitempty"`
	Left   []WatchMatch `json:"left,omitempty"`
	// How many headings match now.
	Count int  `json:"count"`
	Read  bool `json:"read"`
}

// WatchStatus is a watch as GET /watches describes it: what it is, and what it
// is finding.
type WatchStatus struct {
	QueryWatch
	// The expression actually run (the saved query's text, for one).
	Resolved string `json:"resolved"`
	// Matches at the last check, and when that was; -1 before the first.
	Count   int    `json:"count"`
	Checked string `json:"checked,omitempty"`
	// Why it cannot be checked: the saved query is gone, or does not parse.
	Error string `json:"error,omitempty"`
}

type WatchList struct {
	Ok      bool          `json:"ok"`
	Msg     string        `json:"msg,omitempty"`
	Watches []WatchStatus `json:"watches"`
	// A notify plugin is configured, so Desktop can do something.
	Desktop bool `json:"desktop"`
}

type NoticeList struct {
	Ok      bool          `json:"ok"`
	Msg     string        `json:"msg,omitempty"`
	Unread  int           `json:"unread"`
	Total   int           `json:"total"`
	Notices []WatchNotice `json:"notices"`
}

// NoticeAct is the body of POST /notifications/read and /notifications/clear:
// these ids, or every one.
type NoticeAct struct {
	Ids []string `json:"ids,omitempty"`
	All bool     `json:"all,omitempty"`
}
