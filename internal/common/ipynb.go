package common

// IpynbImport is a request to turn a Jupyter notebook into an org file:
// POST /import/ipynb.
type IpynbImport struct {
	// The .ipynb file's text. Sent rather than named, because the notebook
	// is usually on the client's machine and not the server's.
	Notebook string
	// The notebook's own file name, for a title when it has none of its own.
	// Optional.
	Name string
	// Where the org file goes: relative to the first org directory, or an
	// absolute path inside one. Empty means the notebook's name in the first
	// org directory.
	Filename string
	// Replace a file (and pictures) that are already there.
	Overwrite bool
	// Convert, and say what would be written, without writing anything. The
	// answer carries the org text in Org.
	DryRun bool
}

// IpynbImported says what an import wrote, or would have.
type IpynbImported struct {
	Ok  bool
	Msg string
	// The org file, absolute.
	Filename string
	// The pictures written beside it, absolute.
	Files []string
	// The org text; only on a dry run.
	Org string `json:",omitempty"`

	Title    string
	Language string
	// The python session every block shares (header-args in the file).
	Session  string
	Nbformat int

	// Cells of each kind in the notebook, source blocks written, results
	// written under them, pictures saved, and outputs that were errors.
	Cells    int
	Markdown int
	Code     int
	Raw      int
	Blocks   int
	Outputs  int
	Images   int
	Errors   int
	Warnings []string `json:",omitempty"`
}
