package common

// What the server keeps current by itself (refresh.go): GET /refresh lists it,
// POST /refresh/run runs it now.

// RefreshItem is one thing on a timer.
type RefreshItem struct {
	// "block" (a source block with :schedule), "call" (a #+CALL: with
	// :schedule), "dblock" (a dynamic block with :refresh), "file" (a file's
	// dynamic blocks and table formulas: #+REFRESH: or refresh.every).
	Kind string
	// What identifies it across edits: a block's name, else its language and
	// a hash of its code; a dynamic block's #+BEGIN: line; a file's path.
	Key  string
	File string
	// Zero based, as the file stands now.
	Line int
	// The block's #+NAME:, a dynamic block's name, or empty.
	Name string
	// How often, as written (1h, daily 07:30), and where that was said.
	Every string
	From  string
	// RFC 3339; empty when it has never run.
	Last string `json:",omitempty"`
	Next string `json:",omitempty"`
	// How the last run went: Ok, what it said, whether it changed the file,
	// and how long it took.
	Ok      bool
	Msg     string `json:",omitempty"`
	Changed bool
	Millis  int64 `json:",omitempty"`
	// Why it will not run (babel is off, scheduling is off), or empty.
	Blocked string `json:",omitempty"`
}

// RefreshList is the answer to GET /refresh.
type RefreshList struct {
	Ok    bool
	Msg   string `json:",omitempty"`
	Items []RefreshItem
	// Whether exports see refreshed blocks, and the global timer, for saying so.
	OnExport bool
	Every    string `json:",omitempty"`
}

// RefreshRun asks for things to run now: by Key, everything (All), everything
// due (Due), or every refreshable thing in one File (its dynamic blocks and
// tables, and its scheduled blocks with Blocks).
type RefreshRun struct {
	Key    string
	All    bool
	Due    bool
	File   string
	Blocks bool
}

// RefreshRan is what POST /refresh/run did.
type RefreshRan struct {
	Ok    bool
	Msg   string
	Items []RefreshItem
}
