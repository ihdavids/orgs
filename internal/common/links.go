package common

// Wire types for the link graph: who links to whom across the org files the
// server holds. These are shared between the server and any client that wants
// to show backlinks or draw the graph.

// One end of a link. A file level endpoint (a link that points at a whole file,
// or one written in the preamble before the first heading) leaves Hash,
// Headline and Olp empty.
type LinkEnd struct {
	Filename string   // Absolute path of the org file
	Hash     string   // Hash of the heading, empty for a file level endpoint
	Headline string   // Headline text, empty for a file level endpoint
	Olp      []string // Outline path of the heading, outermost first
	Level    int      // Outline depth of the heading, 0 for a file level endpoint
	Line     int      // Zero based line the link was written on (source end only)
}

// A single link, resolved as far as the database allows.
type OrgLink struct {
	From   LinkEnd // Where the link was written
	To     LinkEnd // What it points at, Filename empty when nothing was found
	Raw    string  // The link target as written, e.g. "file:notes.org::*Plans"
	Desc   string  // The links description text, empty when it had none
	Kind   string  // id, custom-id, file, heading, fuzzy, external or unresolved
	Broken bool    // Set when the link names an org target that was not found
}

// Links into and out of one file.
type Backlinks struct {
	Ok       bool
	Msg      string
	Filename string
	In       []OrgLink // Links written elsewhere that point at this file or a heading in it
	Out      []OrgLink // Links written in this file that point at another file
	Internal []OrgLink // Links written in this file that point back into this file
}

// A node of the drawn graph. Id is "file:<path>" for a file and "node:<hash>"
// for a heading, so the two name spaces cannot collide.
type LinkGraphNode struct {
	Id       string
	Kind     string   // file or heading
	Label    string   // Base name of the file, or the headline text
	Filename string   // Absolute path of the file the node lives in
	Hash     string   // Heading hash, empty for a file node
	Olp      []string // Outline path of the heading
	Level    int      // Outline depth, 0 for a file node
	In       int      // Number of distinct nodes linking in
	Out      int      // Number of distinct nodes linked out to
	Center   bool     // Set on the node the graph was centered on
	Distance int      // Hops from the center node
}

// An edge of the drawn graph. Two nodes that link at each other are collapsed
// into a single edge with Both set, which is what makes a bidirectional link
// visible as one stroke rather than two overlapping ones.
type LinkGraphEdge struct {
	From   string // Id of the source node
	To     string // Id of the target node
	Count  int    // Links written from From to To
	Back   int    // Links written from To back to From
	Both   bool   // Set when links run in both directions
	Broken bool   // Set when every link on this edge is broken
}

// The graph around one file, or the whole database when no file was named.
type LinkGraphResult struct {
	Ok     bool
	Msg    string
	Center string // Id of the node the graph was centered on, empty for the whole graph
	Nodes  []LinkGraphNode
	Edges  []LinkGraphEdge
}

// Per file link counts, used to decorate the file tree.
type LinkFileStat struct {
	Filename string
	In       int // Links written elsewhere pointing into this file
	Out      int // Links written here pointing at another file
	Internal int // Links written here pointing back into this file
	Broken   int // Links written here that resolve to nothing
}

type LinkStatsResult struct {
	Ok    bool
	Msg   string
	Files []LinkFileStat
}

// One link as the links tab reads it: what it says, where it was written, and
// - for a link that leaves the org files - what it points at out there.
//
// This is the same link the graph is built from, turned inside out. The graph
// cares about org files pointing at each other and throws the rest away; this
// is mostly about the rest, because a link to a ticket, a document or a video
// is the kind a person goes looking for later and has no other index of.
type LinkEntry struct {
	// The target exactly as it was written, and the text it was written under.
	Raw  string
	Desc string
	// id, custom-id, file, heading, fuzzy, external or unresolved.
	Kind   string
	Broken bool

	// For a link that leaves the org files: the parts of it worth grouping and
	// searching by. Empty for a link from one heading to another.
	Scheme string // https, mailto, doi
	Host   string // news.ycombinator.com
	// What service that host belongs to, said the way a person would say it:
	// "GitHub", "Google Docs", "Jira". Worked out from the host, and falling
	// back to the domain itself, so a host nobody has heard of still groups
	// with the others from the same place.
	Service string

	// Where it was written.
	Filename string
	Heading  string
	Olp      []string
	Hash     string
	Line     int

	// Where it lands, for a link that stays inside the org files.
	ToFilename string
	ToHeadline string
	ToHash     string

	// For a link that names a file on disk rather than an org heading - a
	// picture, a recording, a pdf beside the notes - where this server serves
	// it from and what kind of thing it is ("image", "audio", "video", "pdf",
	// "text", "binary"). Resolved here for the same reason a source block's
	// result file is: a link is written relative to the org file that holds
	// it, and the client knows neither that directory nor the org roots.
	Url   string
	Media string
}

// One service, and how many links point at it - the filter strip the links tab
// offers before anybody has typed anything.
type LinkService struct {
	Service string
	Count   int
}

// Every link in the database, and the services to group them by. One request
// rather than two, for the same reason the code index is one: the second is
// derived from the first and asking twice can only disagree with itself.
type LinkList struct {
	Links    []LinkEntry
	Services []LinkService
	// How many links there are altogether, before any filter, so a client can
	// say "12 of 300" honestly.
	Total int
}
