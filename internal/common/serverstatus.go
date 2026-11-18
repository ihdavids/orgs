package common

import "time"

// ServerStatus is GET /server/status: what a running server is doing, for
// orgs doctor and for looking at by hand when something is off. Several bugs
// in docs/claude/bug-history.md were found only by close inspection - a stream
// too many, a plugin gone quiet, a file that never loaded - and this is the
// one place that says all of it.
type ServerStatus struct {
	Version VersionInfo
	Started time.Time
	Uptime  string

	Files FilesStatus
	// Every open /events stream (and the server's own listeners), oldest
	// first, and the events dropped because a stream was not reading.
	Streams       []StreamInfo
	DroppedEvents uint64
	Plugins       []PluginState
	Cert          *CertStatus `json:",omitempty"`
	Worg          WorgBuild
	Auth          AuthStatus
	Runtime       RuntimeStatus
}

type FilesStatus struct {
	OrgDirs []string
	// Org directories that are not there or not readable.
	MissingDirs []string `json:",omitempty"`
	Loaded      int
	// Whether the file watcher is running (a -local run has none).
	Watching bool
	// Org directories the watcher could not add (canFailWatch on), with why.
	WatchErrors []FileError `json:",omitempty"`
	// The database's version: one more for every file read, since start.
	ReloadIndex uint64
	LastReload  time.Time `json:",omitempty"`
	// Files that could not be read or parsed the last time they were tried,
	// with why. Cleared when the file loads.
	Unreadable []FileError `json:",omitempty"`
}

type FileError struct {
	File  string
	Error string
	At    time.Time
}

// StreamInfo is one open /events subscription: who holds it and since when.
// A browser holds at most six connections to a host across all its tabs, and
// a page holding a stream too many hangs its POSTs with no error anywhere -
// this is where that shows.
type StreamInfo struct {
	// The client's address, "" for the server's own listeners (hooks).
	Addr  string
	User  string `json:",omitempty"`
	Kinds string `json:",omitempty"`
	Since time.Time
	// Events this subscriber missed because it was not reading.
	Dropped uint64
}

type CertStatus struct {
	File     string
	Subject  string    `json:",omitempty"`
	NotAfter time.Time `json:",omitempty"`
	// Days until it runs out; negative once it has.
	DaysLeft int    `json:",omitempty"`
	Error    string `json:",omitempty"`
}

// WorgBuild is worg/build.json, written by tools/buildworg.sh: what the
// embedded worg was built from. Empty commits are a build from before the
// stamp existed.
type WorgBuild struct {
	Present    bool   // index.html is in the binary at all
	WorgCommit string `json:"worgCommit"`
	WorgDirty  bool   `json:"worgDirty"`
	OrgsCommit string `json:"orgsCommit"`
	BuiltAt    string `json:"builtAt"`
}

type AuthStatus struct {
	NoAuth      bool
	TokenExpiry string
	Refresh     string
}

type RuntimeStatus struct {
	Goroutines int
	HeapMB     float64
	SysMB      float64
	NumGC      uint32
}
