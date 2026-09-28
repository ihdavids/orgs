package common

// Moving headings about: refile, copy and archive, one node or many.
//
// The three are one operation with three endings. All of them find a heading,
// work out where it should go, and write it there; a refile then deletes the
// original, a copy does not, and an archive decides the destination for itself
// out of org's archive rules rather than being told. Sending them as one
// request with an `Op` rather than as three endpoints is not tidiness - it is
// so that a batch of twenty headings is *one* thing that either happened or did
// not, reported per heading, instead of twenty requests a client has to
// sequence and then explain.
//
// And it has to be one request, because of a property of the org database that
// is easy to miss until it corrupts something: **a heading's hash is not stable
// across an edit to its file.** The hash is accumulated from the document name
// and the chain of headline titles the parser has walked, so moving one heading
// out of a file changes the hash of every heading after it. A client that
// collects twenty hashes and posts twenty refiles gets the first one right and
// is addressing headings that no longer exist by the second - or, worse,
// headings that now answer to those hashes. The server resolves all of them
// before it changes anything.

// What to do with the headings.
const (
	MoveRefile  = "refile"
	MoveCopy    = "copy"
	MoveArchive = "archive"
)

type MoveRequest struct {
	// refile, copy or archive.
	Op string
	// The headings to move, usually as hashes. Resolved before anything is
	// written - see the note above.
	From []Target
	// Where they go. Ignored for an archive, which works its own destination
	// out of the `:ARCHIVE:` property, the file's `#+ARCHIVE:`, or the server's
	// default, in that order.
	To Target
	// Create the destination heading when it is not there. Off by default:
	// creating a heading because somebody mistyped the one they meant is a
	// worse outcome than being told it does not exist.
	Create bool
}

// What happened to one heading.
type MoveResult struct {
	Headline string
	Filename string
	Ok       bool
	Msg      string
	// Set when this heading was not attempted because one of its ancestors was
	// in the same request. Moving the ancestor takes it along, and moving it
	// as well would be moving it out of the thing that has just moved.
	Skipped bool
}

// What happened to all of them.
//
// `Ok` is whether *everything* worked. A batch that half worked is not a
// success and is not a failure either, which is why the per heading results are
// always here rather than only when something went wrong: the client has to be
// able to say which three of the twenty did not move.
type MoveResponse struct {
	Ok      bool
	Msg     string
	Results []MoveResult
	Done    int
	Failed  int
	Skipped int
}

// One place a heading can be moved to.
//
// This is the structured form of what `/refilefiles` answers with as
// `"file|H1|H2"` strings. The strings cannot say which of two headings with the
// same name is meant and have nowhere to put a keyword or a tag, which is most
// of what tells one candidate from another when you are looking at a list of
// three hundred.
type RefileTarget struct {
	// The heading's hash. Empty for a whole-file target, which means the end of
	// that file.
	Hash string
	// Where it lives.
	Filename string
	// The outline path, outermost first. Empty for a file target. This is what
	// a move is actually addressed by: unlike a hash it survives the file being
	// rewritten, which is exactly what a batch does between one heading and the
	// next.
	Olp []string
	// Outline depth. 0 for a file.
	Level int
	// The keyword and tags, because "which Notes heading did I mean" is usually
	// answered by what is on it rather than by its name.
	Todo string
	Tags []string
}

type RefileTargetList struct {
	Ok      bool
	Msg     string
	Targets []RefileTarget
	// How many files were looked at. "Nothing matched" means a very different
	// thing when the search covered two files than when it covered two hundred,
	// and only this side knows which.
	Files int
	// Set when the walk stopped early. The list is long enough to be worth
	// capping and a silently short list is a list that lies.
	Truncated bool
}
