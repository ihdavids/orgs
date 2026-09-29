package common

// Wire types for the starmap: the org database as a galaxy.
//
// One star per thing, one thread of light per link. What a "thing" is depends on
// the scope asked for - a file, or a heading - and that is the only real
// decision in here, because everything else follows from it: which stars group
// into a constellation, what a link has to land on, and how big a star is.
//
// This is not `/links/graph` with extra fields. The graph answers "what points
// at what", so a file or a heading nothing links to is not in it at all - and in
// an org database that is most of them. A galaxy has to hold every star,
// including the ones sitting on their own, which is why this walks the files
// rather than the link index and then asks the link index for the threads.

// One star.
//
// Id is "file:<path>" for a file and "node:<hash>" for a heading, the same two
// name spaces the link graph uses, so an id can be carried between the two
// without translation.
type StarmapNode struct {
	Id       string
	Kind     string   // file or heading
	Label    string   // Base name of the file, or the headline text
	Filename string   // Absolute path of the file the star lives in
	Hash     string   // Heading hash, empty for a file star
	Olp      []string // Outline path of the heading, outermost first
	Level    int      // Outline depth, 0 for a file star
	Line     int      // Zero based line the headline sits on, 0 for a file star
	Parent   string   // Id of the star this one hangs under, empty at the top

	// The two things a star can be grouped into a constellation by: the first
	// path segment under the org directory, and the file it lives in.
	//
	// Both are here and neither is called the constellation, because which one
	// is right depends on the database rather than on the data: one folder of
	// thirty files wants the file, four hundred day pages under `journal/` want
	// the folder, and nothing on the server can tell which somebody is looking
	// at. It is a question about reading, so the client answers it.
	Folder string // "" for a file sitting at the org root
	File   string // Base name of the file, without its extension
	// The path as a person would write it: relative to the org directory that
	// holds it, so the galaxy is not a wall of absolute paths.
	Path string
	// note or log. A file under a journal, daily or log folder - or one named
	// after a date - is a log, and draws as a smaller ember star. There are a
	// great many of them in a real database and they are not what anybody is
	// looking for when they open a map of it.
	Group string

	// How heavy the star is. Bytes and lines of the file, or of the heading's
	// own subtree.
	Size  int
	Lines int

	// When the star was born, as YYYY-MM-DD, and whether that date was
	// *written* - a CREATED property, a date in the name, a CLOSED stamp -
	// rather than read off the filesystem. A modification time moves whenever
	// anything is edited, so a bulk reformat would otherwise light up the whole
	// sky as newly born.
	Date  string
	Dated bool
	// When the file was last written, as YYYY-MM-DD.
	Touched string

	// What the heading is, for a client that wants to say so: the todo keyword
	// and the tags. Empty on a file star.
	Status string
	Tags   []string

	// How many distinct stars link in and out. Counted over the whole database
	// rather than over what survived a filter: how heavy a heading is does not
	// change because somebody excluded a folder.
	In  int
	Out int
}

// One thread of light.
//
// Kind is "link" for a link somebody wrote and "outline" for the structural
// edge from a heading to the heading (or file) above it. The outline is not a
// link and must not be counted as one - but it is the relationship org is made
// of, and a galaxy drawn without it is nine parts loose dust, so it is drawn and
// named honestly rather than left out or quietly mixed in with the links.
type StarmapEdge struct {
	From   string
	To     string
	Kind   string // link or outline
	Count  int    // Links written from From to To
	Back   int    // Links written from To back to From
	Both   bool   // Set when links run in both directions
	Broken bool   // Set when every link on this edge is broken
}

// The galaxy.
type StarmapResult struct {
	Ok    bool
	Msg   string
	Scope string // file or heading, echoed back

	Nodes []StarmapNode
	Edges []StarmapEdge

	// How many stars there were before any cap, and whether one was applied.
	Total     int
	Truncated bool

	// The black hole's diet: archived files and headings are never stars, they
	// are counted here, and the most recent few are named so a client can say
	// what went in rather than only how much.
	Swallowed int
	Eaten     []string

	// Links whose far end is not a star in this scope - a link at a whole file
	// while the stars are headings, or one pointing outside the org files.
	// Reported rather than silently dropped, because "the picture has fewer
	// threads than I have links" is otherwise unanswerable.
	SkippedLinks int
}
