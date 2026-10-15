package common

// DetangleRequest asks for a tangled file's edits to go back into the org
// blocks it came from: POST /detangle.
type DetangleRequest struct {
	// Where the tangled file is: its markers' links are relative to it.
	File string
	// Its contents. Without them the server reads File, inside the org
	// directories only.
	Text   string `json:",omitempty"`
	DryRun bool
}

// DetangledBlock is what happened to one marked section.
type DetangledBlock struct {
	Label string
	// updated, unchanged, skipped (a noweb expansion), missing.
	Status string
	Msg    string `json:",omitempty"`
	// The org file and the block's #+BEGIN_SRC line (zero based).
	File string `json:",omitempty"`
	Line int
	// The begin marker's line in the tangled file.
	TangledLine int
}

// DetangleResult is the answer to POST /detangle.
type DetangleResult struct {
	Ok      bool
	Msg     string
	Blocks  []DetangledBlock
	Written []string `json:",omitempty"`
}

// AutoTangled is what one auto-tangle did.
type AutoTangled struct {
	Ok        bool
	Msg       string
	File      string
	Written   []string `json:",omitempty"`
	Conflicts []string `json:",omitempty"`
}
