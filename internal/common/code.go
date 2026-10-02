package common

// The wire types behind /code - every source block in the database, and what
// each one is written to take.
//
// A source block in org is more than the text between BEGIN_SRC and END_SRC.
// It has a language, a name it can be called by, switches, babel header
// arguments, and - the part worth drawing carefully - variables, which may be
// literals or may be *the name of something else in the file*: a table, the
// results of another block, a list. That last case is the one a reader needs
// pointing out, because it is the only way to tell that a block is a step in a
// chain rather than a thing on its own.

// One header argument of a block: ":results table" is Key "results", Value
// "table". Variables are not in here - they get CodeVar, because there may be
// several and because a variable is the interesting one.
type CodeArg struct {
	Key   string
	Value string
}

// One ":var name=value" binding.
type CodeVar struct {
	Name string
	// The value exactly as it is written, indexing and all.
	Value string

	// What the value names, when it names something in the org file rather
	// than being a literal. Empty Ref means a literal.
	Ref string
	// What that something is: "table", "src", "list", "example", "results",
	// or "" when the name resolves to nothing the database can find. A name
	// that resolves to nothing is worth saying out loud - it is a block that
	// cannot run.
	RefKind string
	// Where it lives, so a client can offer to go and look at it.
	RefFile string
	RefLine int
	// The shape of a table it points at. Zero for anything else.
	Rows int
	Cols int
	// The indexing written after the name: "[1,2]", "[,0]". Kept apart from
	// Ref so the reference still resolves.
	Index string
}

// One source block.
type CodeBlock struct {
	Filename string
	// Where it is in that file, counted in document order from zero. The same
	// way a table is addressed, and for the same reason: a block has no other
	// identity unless it has been given a name.
	Id int
	// The #+NAME: above it, when it has one. This is what another block calls
	// it by, so it is the first thing to show.
	Name string
	// The language, lowercased: "python", "emacs-lisp", "sh".
	Lang string
	// The heading it sits under, as an outline path, and the pieces of it.
	Heading string
	Olp     []string
	// The hash of that heading, for jumping to it. Empty for a block above the
	// first heading.
	Hash string
	// That heading's tags and the tags it inherits, its own first: what a
	// snippet is filtered by (`orgs snip -t docker`).
	Tags []string

	// The #+BEGIN_SRC line and the #+END_SRC line, zero based.
	Line    int
	EndLine int

	// The code itself, and how many lines of it there are.
	Code  string
	Lines int

	// The switches written before the header arguments: "-n", "-r", "+n".
	Switches []string
	// The header arguments, in the order they were written, minus the vars.
	Args []CodeArg
	Vars []CodeVar

	// What the block last produced, if a #+RESULTS: follows it, and what shape
	// that took: "table", "list", "text", "" for none.
	Result     string
	ResultKind string
}

// One language, and how many blocks are written in it - the filter the code
// tab offers before anybody has typed anything.
type CodeLang struct {
	Lang  string
	Count int
}

// Everything the code tab needs in one answer: the blocks, and the languages
// to filter them by. One request rather than two, because the second is
// derived from the first and asking twice can only disagree with itself.
type CodeIndex struct {
	Blocks []CodeBlock
	Langs  []CodeLang
	// How many blocks there are altogether, before any filter was applied, so
	// a client can say "12 of 300" honestly.
	Total int
}

// Running one block.
type CodeRun struct {
	Filename string
	Id       int
	// Code that is not in the file yet, for an editor trying an edit before
	// saving it. A run is the fastest way to find out whether a change works,
	// and making somebody save first to find out is the wrong order.
	Code    string
	SetCode bool
}

// What came back from running it.
type CodeResult struct {
	Ok  bool
	Msg string

	// What shape the result is: "table", "list", "file" or "text". Read from
	// :results where it says, and guessed from the output where it does not.
	Kind string
	// The result as org text of that shape - a table written as an org table,
	// so a client can hand it to the table view it already has rather than
	// either side inventing a second format for rows.
	Result string
	// What the program actually printed, for showing when the shape was
	// guessed wrong.
	Raw string

	// What went to standard error, the exit status, and how long it took.
	// Stderr is worth showing even on success: plenty of programs warn.
	Stderr  string
	Code    int
	Seconds float64

	// When the result is a file link - `:results file`, or a block that
	// printed one - what it points at and what is in it.
	//
	// The link is resolved here rather than in the client because only this
	// side knows where it is relative to: a block runs beside the org file it
	// lives in, so that is what the name it printed is relative to. The client
	// has neither the file's directory nor the org roots.
	File string
	// Where this server will serve it from, or "" when it is outside the org
	// directories and so not served at all.
	Url    string
	Exists bool
	Bytes  int64
	// The contents, when it is text and small enough to be worth showing. A
	// client shows this beside the link so that a block whose answer is a file
	// has its answer on screen rather than a path to it.
	Text string
	// Whether Text is only the start of the file.
	Truncated bool
	// What kind of thing it is: "image", "audio", "video", "pdf", "text" or
	// "binary". Decided here rather than by the client guessing at the
	// extension twice - this side already keeps the lists, because the html
	// exporter and the kanban cards needed them first.
	Media string
	// The language to colour Text as, from the file's extension: "python",
	// "json", "mermaid". Empty when there is nothing sensible to say.
	TextLang string
}

// Changing a block in the file it lives in.
//
// Each part is written only when its flag is set, because "" is a thing
// somebody means: clearing the name and leaving it alone are different.
type CodeUpdate struct {
	Filename string
	Id       int

	// The #+NAME: above the block. Empty with SetName takes it off.
	Name    string
	SetName bool

	// The rest of the #+BEGIN_SRC line: the language, the switches and the
	// header arguments, each of which is written back as org writes it.
	Lang      string
	Switches  []string
	Args      []CodeArg
	Vars      []CodeVar
	SetHeader bool

	// The code between the delimiters.
	Code    string
	SetCode bool
}
