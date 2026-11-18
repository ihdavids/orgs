package orgs

// One heading's lines, kept measured while a request edits them.
//
// Every write here splices lines rather than writing a parsed document back
// (**Traps: line edits**), and a request that makes several edits to one
// heading has to find the heading's extent again after each edit that adds or
// removes a line. Measuring once and reusing the numbers writes the next edit
// into the wrong heading, and the write still reports success. That used to be
// a `reread` call after each such edit, which every caller had to remember.
//
// headingLines makes it impossible to forget: the only way to hand it new lines
// is Set, and Set measures again every time, whether or not the count changed.
// Measuring is one pass from the headline to the next heading at its level or
// above - nothing next to a file read and write.
//
// An edit may change anything from the headline down to the end of the subtree.
// It must not add or remove lines above the headline: From is the headline's
// row and is not looked for again.

import (
	"fmt"
	"os"
	"strings"

	"github.com/ihdavids/go-org/org"
)

type headingLines struct {
	File  string
	Lines []string
	// From is the headline's row, End the last row of its subtree (the line
	// before the next heading at the same level or above, or the file's last).
	From, End int
	Lvl       int
}

// readHeadingLines reads a heading's file off disk and measures the heading.
func readHeadingLines(filename string, sec *org.Section) (*headingLines, error) {
	if filename == "" || sec == nil || sec.Headline == nil {
		return nil, fmt.Errorf("no heading to edit")
	}
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	h := newHeadingLines(filename, lines, sec.Headline.Pos.Row, sec.Headline.Lvl)
	if h == nil {
		return nil, fmt.Errorf("%s has no line %d", filename, sec.Headline.Pos.Row+1)
	}
	return h, nil
}

// newHeadingLines measures the heading at row in lines already read, or nil
// when the row is not in them.
func newHeadingLines(filename string, lines []string, row, lvl int) *headingLines {
	if row < 0 || row >= len(lines) {
		return nil
	}
	h := &headingLines{File: filename, Lvl: lvl, From: row}
	h.Set(lines)
	return h
}

// Set takes the lines after an edit and measures the heading again.
func (h *headingLines) Set(lines []string) {
	h.Lines = lines
	h.End = subtreeEndRow(lines, h.From, h.Lvl, h.From)
	if h.End >= len(lines) {
		h.End = len(lines) - 1
	}
}

// Write saves the lines and reads the file back into the database.
func (h *headingLines) Write() error {
	return writeLines(h.File, h.Lines)
}
