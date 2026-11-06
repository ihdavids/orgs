package common

// Wire types for related notes (features.related, internal/app/orgs/related.go):
// headings like the one being read, by their words or by embeddings.

// RelatedTarget is what a related-notes question was about.
type RelatedTarget struct {
	Kind     string   // heading, file or text
	Filename string   `json:",omitempty"`
	Hash     string   `json:",omitempty"`
	Headline string   `json:",omitempty"`
	Olp      []string `json:",omitempty"`
	Line     int      // zero-based headline row, -1 for a file
	Words    int      // how many words it was read for
}

// RelatedItem is one heading (or file, Line -1) like the target.
type RelatedItem struct {
	Kind     string // heading or file
	Filename string
	Hash     string   `json:",omitempty"`
	Headline string   `json:",omitempty"` // the title, or the file's #+TITLE
	Olp      []string `json:",omitempty"`
	Line     int      // zero-based headline row, -1 for a file
	Id       string   `json:",omitempty"`
	Score    float64  // 0-1: what it was ranked by
	Words    float64  `json:",omitempty"` // the words engine's similarity
	Embed    float64  `json:",omitempty"` // the embeddings' cosine
	Terms    []string `json:",omitempty"` // the words they share most
	Snippet  string   `json:",omitempty"` // the sentence of it most like the target
	Linked   bool     `json:",omitempty"` // a link already joins the two (either way)
}

// RelatedResult answers GET /related and POST /related/text.
type RelatedResult struct {
	Ok      bool
	Off     bool   `json:",omitempty"`
	Msg     string `json:",omitempty"`
	Engine  string // the engine that answered: words or embeddings
	Target  RelatedTarget
	Items   []RelatedItem
	Skipped int `json:",omitempty"` // already linked, left out (linked=1 keeps them)
}

// RelatedTextRequest is POST /related/text: an unsaved buffer's text.
type RelatedTextRequest struct {
	Text     string
	Filename string `json:",omitempty"` // the file it is from, to leave its own heading out
	Line     int    `json:",omitempty"` // zero-based line of the heading in that file (-1 none)
	N        int    `json:",omitempty"`
	Engine   string `json:",omitempty"`
	Linked   bool   `json:",omitempty"` // keep results already linked
}

// RelatedLinkRequest is POST /related/link: link the source to a result.
type RelatedLinkRequest struct {
	Hash       string `json:",omitempty"` // the source heading
	Filename   string `json:",omitempty"` // or a file (with Line: the heading that line is in)
	Line       *int   `json:",omitempty"`
	TargetHash string `json:",omitempty"` // the result: a heading
	TargetFile string `json:",omitempty"` // or a file (TargetLine -1 or absent) or the heading at TargetLine
	TargetLine *int   `json:",omitempty"`
}

// RelatedLinkResult says what /related/link wrote.
type RelatedLinkResult struct {
	Ok       bool
	Off      bool   `json:",omitempty"`
	Msg      string `json:",omitempty"`
	Link     string `json:",omitempty"` // the link written, [[id:…][Title]]
	Filename string `json:",omitempty"` // where
	Line     int    // zero-based line it was written on
	Minted   bool   `json:",omitempty"` // an :ID: was given to the target
}

// RelatedPair is two headings alike with no link between them.
type RelatedPair struct {
	A, B  RelatedItem
	Score float64
}

// RelatedPairs answers GET /related/pairs.
type RelatedPairs struct {
	Ok     bool
	Off    bool   `json:",omitempty"`
	Msg    string `json:",omitempty"`
	Engine string
	Pairs  []RelatedPair
}

// RelatedEmbedStatus is the embeddings engine's state.
type RelatedEmbedStatus struct {
	Configured bool
	Backend    string `json:",omitempty"`
	Model      string `json:",omitempty"`
	State      string // off, idle, embedding, waiting, error
	Msg        string `json:",omitempty"`
	Total      int    // headings with text
	Done       int    // with a vector for their current text
	Pending    int
	LastError  string `json:",omitempty"`
	Store      string `json:",omitempty"` // the vectors file
	PerMinute  int    `json:",omitempty"`
}

// RelatedStatus answers GET /related/status.
type RelatedStatus struct {
	Ok         bool
	Off        bool   `json:",omitempty"`
	Msg        string `json:",omitempty"`
	Engine     string // the engine a question gets when it names none
	Files      int
	Chunks     int // headings (and file preambles) with words
	Terms      int // distinct words
	Embeddings RelatedEmbedStatus
}
