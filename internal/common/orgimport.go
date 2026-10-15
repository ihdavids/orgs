package common

// OrgImport is org text made on the client, to be written into the org
// directories: POST /import/org.
//
// The markdown, todo.txt and iCalendar importers convert on the client (the
// file being imported is on that machine, and a vault is a folder of them)
// and send the org they made here, so writing - where a file may go, what it
// would clash with, what an append already holds - is decided once, by the
// server that owns the directories.
type OrgImport struct {
	Files []ImportFile
	// Pictures and attachments the files link to, written before the files
	// so a link never points at something that is not there yet.
	Assets []ImportAsset `json:",omitempty"`
	// Replace files (and assets) that are already there.
	Overwrite bool
	// Add to the end of a file that is already there rather than refusing.
	// The new text's own #+KEYWORD: lines are left off: the file has a head.
	Append bool
	// With Append: leave out each top level entry whose KeyProp the file
	// already holds, so importing the same calendar twice adds only what is
	// new.
	SkipKnown bool
	// The property that says which entry is which. Empty means ID.
	KeyProp string `json:",omitempty"`
	// Say what would be written, writing nothing. Each file's org comes back.
	DryRun bool
}

// ImportFile is one org file of an OrgImport.
type ImportFile struct {
	// Relative to the first org directory, or absolute inside one; ends in .org.
	Filename string
	Org      string
}

// ImportAsset is one file an imported org file links to.
type ImportAsset struct {
	// Relative to the first org directory, or absolute inside one.
	Filename string
	Data     []byte
}

// OrgImported says what an OrgImport wrote, or would have.
type OrgImported struct {
	Ok       bool
	Msg      string
	Files    []ImportedFile
	Assets   []string `json:",omitempty"`
	Warnings []string `json:",omitempty"`
}

// ImportedFile is what happened to one file.
type ImportedFile struct {
	// Absolute.
	Filename string
	// Top level headings written, and ones left out because the file had them.
	Entries int
	Skipped int
	// Added to a file that was there, rather than a new file (or a replaced one).
	Appended bool
	// What was (or would be) written: the whole file, or what an append adds.
	// Only on a dry run.
	Org string `json:",omitempty"`
}
