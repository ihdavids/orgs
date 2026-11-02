package common

// Edits made to text a client sends rather than to the file on disk: what an
// editor holding unsaved changes needs (`orgs lsp`). The server applies the
// same line edit it would make to the file and answers with the new text, so
// the editor can apply it to its buffer as an undoable change and nothing is
// written behind its back.

// TextDateChange sets or clears SCHEDULED, DEADLINE, CLOSED or TIMESTAMP on
// the heading that holds Line (zero based) of Text.
type TextDateChange struct {
	Filename string
	Text     string
	Line     int
	Name     string
	// A stamp as written in a file, or a bare date. Empty clears it.
	Value string
}

// TextRun runs the source block, #+CALL: line or inline code at Line (zero
// based) of Text, which is the file Filename as the editor has it.
type TextRun struct {
	Filename string
	Text     string
	Line     int
}

// TextEditResult is Text after the edit. Changed is false when there was
// nothing to change, and then Text is empty.
type TextEditResult struct {
	Ok      bool
	Msg     string
	Changed bool
	Text    string
	// What ran, for a TextRun.
	Ran []BabelRan `json:",omitempty"`
}
