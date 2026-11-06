package common

// Wire types for backlink counts (features.backlinkCounts): how many places link to each heading of a file, and to each
// link's target, so an editor can write the number beside it.

// One heading of the file and what points at it.
type LinkCountHeading struct {
	Line  int // zero based headline line
	Hash  string
	Title string
	Level int
	Olp   []string `json:",omitempty"`
	// Distinct places linking here: a heading, or a file's text above its
	// first heading, counts once however many links it has.
	Backlinks int
	Links     int // every link landing here
	// Unlinked mentions of the title and aliases, only when they were asked
	// for (LinkCounts.Mentions). MentionsSkipped says why a title was not
	// searched for (too short, only common words).
	Mentions        int
	MentionsSkipped string `json:",omitempty"`
	Out             int // links written under this heading (not its children) to somewhere else
}

// One link written in the file, and how many other links go where it goes.
type LinkCountLink struct {
	Line int // zero based
	// Where [[ starts and where ]] ends, in UTF-16 code units (what an editor
	// counts in), end exclusive.
	Col, End   int
	Raw        string
	Desc       string `json:",omitempty"`
	Kind       string
	ToFilename string
	ToHash     string `json:",omitempty"`
	ToHeadline string `json:",omitempty"`
	Count      int // other links to the same target, this one not counted
	Places     int // distinct headings (or file preambles) those are written in, this link's own not counted
}

// The file itself.
type LinkCountFile struct {
	Backlinks       int // distinct places elsewhere linking to the file itself (not a heading in it)
	Links           int // links from elsewhere to the file itself
	Any             int // links from elsewhere to the file or any heading in it
	Mentions        int
	MentionsSkipped string `json:",omitempty"`
}

type LinkCounts struct {
	Ok       bool
	Off      bool `json:",omitempty"`
	Msg      string
	Filename string
	Mentions bool // whether mention counts were worked out
	File     LinkCountFile
	Headings []LinkCountHeading
	Links    []LinkCountLink
}

// An unsaved buffer to count for (POST /links/counts/text).
type LinkCountsText struct {
	Filename string // the file the buffer is, to read relative links and in-file targets against
	Text     string
	Mentions bool
}

// The most linked headings in the database (GET /links/counts/top).
type LinkCountTop struct {
	Filename  string
	Hash      string
	Title     string
	Olp       []string `json:",omitempty"`
	Line      int
	Level     int
	Backlinks int
	Links     int
	Mentions  int `json:",omitempty"`
}

type LinkCountTopResult struct {
	Ok       bool
	Off      bool `json:",omitempty"`
	Msg      string
	Mentions bool
	Total    int // headings with at least one backlink
	Headings []LinkCountTop
}
