package common

// The wire types behind POST /contacts/import: a .vcf file in, and what was
// (or, on a dry run, would be) done with each person in it.

import (
	"strings"
	"unicode/utf8"
)

// VCardText is a .vcf file as text: UTF-8 when it is, Windows-1252 when it is
// not (an old Outlook export written in 8 bit). Done before the file is sent,
// because JSON would turn every byte that is not UTF-8 into a question mark.
func VCardText(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	return DecodeCP1252(b)
}

// The Windows-1252 characters in 0x80-0x9F, where it differs from Latin-1.
var cp1252High = [32]rune{
	'€', 0x81, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0x8D, 'Ž', 0x8F,
	0x90, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0x9D, 'ž', 'Ÿ',
}

// DecodeCP1252 reads bytes written in Windows-1252, which is also right for
// every printable character of ISO-8859-1.
func DecodeCP1252(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		switch {
		case c < 0x80:
			sb.WriteByte(c)
		case c < 0xA0:
			sb.WriteRune(cp1252High[c-0x80])
		default:
			sb.WriteRune(rune(c))
		}
	}
	return sb.String()
}

// Importing a vCard file into a collection.
type ContactImportRequest struct {
	// The .vcf file's text. Several files are just their texts one after the
	// other - a vCard file is a list of cards.
	Text string
	// "merge" (the default): add new people, and fill in only what a matched
	// one lacks. "skip": leave matched people alone. "overwrite": write the
	// card's values over a matched person's.
	Mode string
	// Answer with the plan and write nothing.
	DryRun bool
	// The collection, "contact" when empty.
	Type string
	// Where new people go: under this heading, or at the end of this file.
	// Both empty is the collection's own container, as for one added by hand.
	TargetHash string
	Filename   string
}

// One field of the plan for one person.
type ContactImportChange struct {
	Key   string
	Value string
	// What the record says now, when it says something.
	Old string
	// "add": the record has no such field (or no record yet). "replace": an
	// overwrite. "kept": the record says something else and merge never
	// overwrites - shown so a person can see why a value did not come in.
	How string
}

// What happens to one person in the file.
type ContactImportItem struct {
	// Which card in the file, from 1. When a file holds a person twice the
	// cards are folded into one and Cards says how many there were.
	Card  int
	Cards int
	Name  string
	// "add", "update", "skip" or "error".
	Action string
	// Why, in a few words: "matched by email jane@x.org", "nothing new".
	Reason string
	// How the existing record was found: "uid", "email" or "name". Empty for
	// a new one.
	Match string
	// The existing record, when there is one.
	Hash     string
	Filename string
	LineNum  int

	Changes []ContactImportChange
	// Tags the record gets that it did not have.
	Tags []string
	// The card's note is added to the record's notes.
	Notes bool
	// Anything odd about the card worth knowing.
	Warnings []string
}

// The answer.
type ContactImportResult struct {
	Ok     bool
	Msg    string
	DryRun bool
	Mode   string
	Type   string
	// Cards read from the file, and what became of the people in them.
	Cards   int
	Added   int
	Updated int
	Skipped int
	Failed  int
	// The file new people are (or would be) written to.
	Target string
	Items  []ContactImportItem
}
