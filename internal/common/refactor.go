package common

// Structural refactors: renaming a tag everywhere, splitting a heading out to
// a file of its own, merging one heading into another, and turning a list into
// headings. Every one is answered twice - a preview that writes nothing and
// says, per file, what the text would become, and an apply that does it - so a
// client can show the change as a diff before anybody agrees to it.
//
// The server is internal/app/orgs/refactor.go; the clients are worg's
// Refactor tab and cmd/oc/commands/refactor.

// RefactorApply is what turns a preview request into an apply: the Token the
// preview answered with, and the hash each file had when it was read. An apply
// works the whole change out again from the files as they are now and refuses
// unless it comes to exactly what the preview showed.
//
// Embedded in every request, so a client sends the request it previewed with
// these two filled in.
type RefactorApply struct {
	Token  string            `json:",omitempty"`
	Expect map[string]string `json:",omitempty"`
}

// RefactorFile is one file a refactor touches.
type RefactorFile struct {
	Filename string
	// A file the refactor makes (a split's new file). Before is empty.
	Created bool `json:",omitempty"`
	Before  string
	After   string
	// The hash of Before, as read. An apply sends these back in Expect.
	Hash string
	// The change as a unified diff, three lines of context, for a terminal.
	Patch   string
	Added   int
	Removed int
}

// RefactorMention is something that names what is being changed but is not
// changed by it - a saved query, a filter or a tag group that mentions a tag
// being renamed. Reported so a person can fix it, never rewritten behind them.
type RefactorMention struct {
	Kind string // "saved query", "filter", "tag group"
	Name string
	Text string
	User string `json:",omitempty"`
}

// RefactorPlan is the answer to both a preview and an apply.
type RefactorPlan struct {
	Ok      bool
	Msg     string
	Op      string
	Summary string
	// Things the refactor could not do for you, or did in a way worth knowing.
	Warnings []string          `json:",omitempty"`
	Mentions []RefactorMention `json:",omitempty"`
	Files    []RefactorFile
	Token    string
	// An ID the refactor made for a heading that had none. A client sends it
	// back as NewId on the apply, or the apply would make a different one and
	// no longer match the preview.
	NewId string `json:",omitempty"`
	// The file a split makes.
	NewFile string `json:",omitempty"`
	Applied bool   `json:",omitempty"`
	// Set when an apply refused because a file changed since the preview.
	Stale bool `json:",omitempty"`
}

// TagRenameRequest renames Old to New, or with Delete takes Old off. A New tag
// already on a heading is a merge: Old goes and New is not written twice.
//
// Scope: every file, or the Files named, or with a Query only the headlines
// of the headings it matches (FILETAGS and #+TAGS lines are then left alone,
// since they are about whole files).
type TagRenameRequest struct {
	Old    string
	New    string
	Delete bool     `json:",omitempty"`
	Files  []string `json:",omitempty"`
	Query  string   `json:",omitempty"`
	RefactorApply
}

// SplitRequest moves a heading's subtree into a new file. To is the new
// file's path (relative to the first org directory) or a folder to put it in;
// empty is a file named for the heading, beside the one it is in now.
type SplitRequest struct {
	Hash  string
	To    string `json:",omitempty"`
	NewId string `json:",omitempty"`
	RefactorApply
}

// MergeRequest merges the heading From into Into, both by hash.
type MergeRequest struct {
	Into string
	From string
	RefactorApply
}

// ListToHeadingsRequest turns a list in a heading's body into child headings.
// Line is a zero-based row of the file anywhere inside the list; -1 (or any
// row outside every list) means the first list in the body.
type ListToHeadingsRequest struct {
	Hash string
	Line int
	RefactorApply
}

// RefactorList is one list in a heading's body, for choosing which to convert.
type RefactorList struct {
	Line       int // zero-based row of the first item
	End        int // zero-based row of its last line
	Items      int
	First      string
	Checkboxes int
}

// RefactorLists answers GET /refactor/lists.
type RefactorLists struct {
	Ok    bool
	Msg   string
	Lists []RefactorList
}

// RefactorTag is a tag in use and where.
type RefactorTag struct {
	Tag      string
	Headings int  // headlines carrying it
	Files    int  // files with it in #+FILETAGS
	Defined  bool `json:",omitempty"` // named on a #+TAGS line
}

// RefactorTags answers GET /refactor/tags.
type RefactorTags struct {
	Ok   bool
	Msg  string
	Tags []RefactorTag
}
