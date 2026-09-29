package common

// Searching the text of every org file, the way ripgrep does it: a regular
// expression, and the lines that matched.
//
// This is a different question from the one /search answers. That one queries
// the *parsed* database - keywords, tags, dates, headline text - and knows what
// a heading is; this one reads the files as text and knows nothing, which is
// exactly what you want when looking for a phrase that could be anywhere: in a
// drawer, in a table, in a source block, in the middle of a paragraph.

// One matching line.
type FileSearchMatch struct {
	// Zero based, the way the server counts everywhere else, so it can be
	// handed straight to the source view.
	Line int
	Text string
	// Where in the line the match starts and ends, in bytes, so a client can
	// pick it out without running the pattern again - and without having to
	// agree with Go about what the pattern means.
	Start int
	End   int
}

// The matches in one file.
type FileSearchFile struct {
	Filename string
	// How many lines matched in this file, which is not len(Matches) when the
	// file had more than the per-file cap.
	Count     int
	Matches   []FileSearchMatch
	Truncated bool
}

// What a search found.
type FileSearchResult struct {
	// False when the pattern does not compile, with Msg saying why. That is an
	// ordinary state of a box being typed into rather than an error.
	Ok    bool
	Msg   string
	Query string
	Files []FileSearchFile
	// Files that had at least one match, and lines that matched, across the
	// whole database - so a client can say what it is showing part of.
	FileCount int
	Total     int
	// How many files were actually opened and read.
	//
	// The trigram index narrows the search to the files that could possibly
	// match, so this is usually far fewer than the database holds - and where
	// it is not, the pattern was one the index could say nothing about and
	// every file was read, exactly as before the index existed. Reported
	// because "why was that slow" and "why was that fast" should both have an
	// answer, and because an index that quietly stopped narrowing would
	// otherwise look like nothing at all.
	Scanned int
}
