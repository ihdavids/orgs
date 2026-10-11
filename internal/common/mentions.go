package common

// Wire types for the links around one heading and for unlinked mentions: the
// places a heading's title is written in plain text without a link to it.
// Shared by the server, worg's links panel and `orgs backlinks` /
// `orgs mentions`.

// What a links or mentions request was about: one heading, or a whole file.
type LinkTarget struct {
	Kind     string   // heading or file
	Filename string   // The file the heading is in, or the file itself
	Hash     string   // The heading's hash, empty for a file
	Headline string   // The headline text, empty for a file
	Olp      []string // Outline path of the heading, outermost first
	Line     int      // Zero based line of the headline, 0 for a file
	Title    string   // What it is called: the headline, or the file's #+TITLE
	Id       string   // Its :ID: property when it has one
	// What mentions are searched for: the title and every alias, cleaned of
	// markup. A term the guard refused is in Skipped instead, with why.
	Terms   []string
	Skipped []string
}

// A link, and the line it was written on - which is what tells a reader
// whether it is the one they meant, far more than the link's own text does.
type LinkInContext struct {
	OrgLink
	Context string // The line the link is written on, links shown as their text
	// The same line cut at the link: what comes before it, the link as it
	// reads, and what comes after - so a client can mark the link itself
	// rather than the first place its words happen to appear.
	Before string
	Shown  string
	After  string
}

// The links into and out of one heading or one file.
type LinksAround struct {
	Ok     bool
	Msg    string
	Target LinkTarget
	In     []LinkInContext // Written elsewhere, pointing here
	Out    []LinkInContext // Written here, pointing elsewhere
}

// One place a target's name is written in plain text.
type Mention struct {
	Filename string
	Line     int    // Zero based
	Col      int    // Byte offset of the match in the line, as the server reads it
	Text     string // The text as written, which is what a link to it keeps
	Term     string // The title or alias it matched
	// The rest of the line either side of the match, with links shown as
	// their descriptions. Long sides are cut at a word with an ellipsis.
	Before string
	After  string
	// The heading the mention is written under, empty in a file's preamble.
	Hash     string
	Headline string
	Olp      []string
	// Set when the mention is in a headline. Linking it changes that headline
	// and so the hash of that heading and every heading under it.
	InHeadline bool
}

type MentionsResult struct {
	Ok        bool
	Msg       string
	Target    LinkTarget
	Mentions  []Mention
	Total     int  // How many were found before the limit
	Truncated bool // Set when Total is more than came back
}

// One mention to turn into a link, as a mentions answer named it. Text is
// checked against the file before anything is written.
type MentionRef struct {
	Filename string
	Line     int
	Col      int
	Text     string
}

// A request to link mentions to a target: a heading by Hash, or a file by
// Filename when Hash is empty.
type MentionLinkRequest struct {
	Hash     string
	Filename string
	Mentions []MentionRef
}

type MentionLinkOutcome struct {
	MentionRef
	Ok   bool
	Msg  string
	Link string // What the text was replaced with
}

type MentionLinkResult struct {
	Ok      bool
	Msg     string
	Linked  int    // How many mentions became links
	Id      string // The id the links point at, for a heading
	NewId   bool   // Set when the heading had no :ID: and was given one
	Results []MentionLinkOutcome
}
