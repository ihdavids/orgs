package common

// One `#+BEGIN: name ...` / `#+END:` block in a file: what it is called, what
// it was asked for, where it sits, and - after an update - how that went.
type DynBlock struct {
	Name string
	// Everything after the name on the BEGIN line, as written.
	Header string
	// The header's `:key value` pairs, keys without their colon and in lower
	// case. A key written with no value reads as "t", the way org reads it.
	Params map[string]string
	// Zero-based rows of the BEGIN and END lines.
	Line    int
	EndLine int
	// The heading the block sits under, empty above the first one.
	Heading string
	// Set by an update: whether this block was rewritten, and why not.
	Ok  bool
	Msg string
}

// POST /dblocks/update. Line picks the block whose BEGIN..END span holds it;
// All updates every block in the file instead. A request with no Filename and
// All set updates every block in every file the server watches.
type DynBlockRequest struct {
	Filename string
	Line     int
	All      bool
	// Only blocks of this kind ("clocktable"), when set.
	Name string
}

// The answer to both /dblocks endpoints.
type DynBlocksResult struct {
	Ok  bool
	Msg string
	// One entry per file touched, for the every-file form.
	Files []DynBlockFile
}

type DynBlockFile struct {
	File   string
	Blocks []DynBlock
	// Whether the file was written. A refresh that produces what was already
	// there leaves the file alone, so its modification time means something.
	Written bool
}
