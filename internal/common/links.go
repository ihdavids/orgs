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
