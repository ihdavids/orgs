package common

// The wire types behind the record endpoints.
//
// A record is one heading that stands for one thing - a person, a laptop, a
// playing card - and keeps what is known about it in its property drawer. The
// heading is the thing's name, the properties are its fields, and the body
// under it is whatever anybody wanted to write about it.
//
// The whole format is described in docs/records.org and in the SDOC block at
// the top of internal/app/orgs/records.go. The two things a client has to know
// are that a record is identified by its RECORD property, which names the
// collection it belongs to, and that a field's *kind* is worked out from its
// property name rather than declared - which is what lets a collection of
// guitar pedals use the same code as the contact book.

// One field of a record: one line of its property drawer, read.
type RecordField struct {
	// The property exactly as it is written: "PHONE_MOBILE".
	Key string
	// The part before the label: "PHONE". Two fields with the same Name are
	// the same kind of thing said twice, which is how a person has three
	// phone numbers.
	Name string
	// The part after it: "MOBILE", or empty for a plain "PHONE".
	Label string
	// What this field *is*, worked out from Name: "email", "tel", "url",
	// "social", "date", "image", "postal" or "text". A client draws a field by
	// its kind and never needs a list of property names.
	Kind string
	// The value as written in the file.
	Value string
	// The value made actionable: "mailto:...", "tel:...", "https://...", or a
	// path this server will serve the picture from. Empty when the value is
	// just text, which is the signal to print it rather than link it.
	Link string
	// Which social network a "social" field is, lowercased: "linkedin". Empty
	// for every other kind.
	Network string
}

// One line of a record's update history, read back out of its LOGBOOK.
type RecordChange struct {
	// When it happened, as the org timestamp was written: "2026-09-26 14:02".
	When string
	// The fields that changed, as they were written: "EMAIL_WORK, PHONE_MOBILE".
	What string
	// What kind of change it was: "added", "updated", "renamed", "noted".
	Kind string
}

// One record.
type Record struct {
	Hash     string
	Id       string
	Type     string
	Name     string
	Filename string
	LineNum  int
	Tags     []string

	// The fields, in the order they are written in the drawer, with the
	// bookkeeping ones (RECORD, ID, ADDED) left out - they are reported in
	// their own fields below rather than as things to draw.
	Fields []RecordField
	// Every property including the bookkeeping, for a client that wants the
	// raw drawer.
	Props map[string]string

	// The body under the heading, as it is written. Child headings are not
	// part of it.
	Notes string

	// The record's picture, resolved to something this server will serve.
	// Empty when it has none.
	Image string

	// When it was first written down and when it was last touched, as org
	// timestamps: "2026-09-26 09:12". Updated is the newest history line, or
	// Added when nothing has changed since.
	Added   string
	Updated string
	History []RecordChange
}

// A collection: every record sharing one RECORD value, and the heading that
// new ones are filed under.
type RecordCollection struct {
	// The value of RECORD, and so what this collection is called on the wire:
	// "contact", "equipment".
	Type string
	// What to call it on screen. The container heading's own text when there
	// is one, otherwise the type with its first letter up.
	Name string
	// An emoji for the tab strip, off the container's ICON property.
	Icon string
	// The fields a new record of this type should be offered, off the
	// container's FIELDS property. Empty means "whatever the records already
	// in it use", which the server works out.
	Fields []string
	// The container heading, when this collection has one. A collection can
	// exist without one - records are identified one at a time - but adding to
	// it needs somewhere to put them.
	Hash     string
	Filename string
	LineNum  int
	// How many records carry this type.
	Count int
}

// Creating a record.
type RecordNew struct {
	Type   string
	Name   string
	Fields map[string]string
	Notes  string
	Tags   []string
	// Where to put it. A hash files it under that heading; a filename appends
	// it to the end of that file. Both empty files it under the collection's
	// own container, which is the usual case and the reason a container is
	// worth having.
	TargetHash string
	Filename   string
}

// Changing one. Every field named in Set is written, and a field set to an
// empty string is taken off - the same rule the property endpoint follows.
type RecordUpdate struct {
	Hash  string
	Name  string
	Set   map[string]string
	Notes string
	// Notes and Name are only written when their flags are set, because "" is
	// a thing somebody might mean.
	SetNotes bool
	SetName  bool
}

// Starting a collection: a container heading in a file.
type RecordCollectionNew struct {
	Type     string
	Name     string
	Icon     string
	Fields   []string
	Filename string
	// Optional: file the container under this heading rather than at the end
	// of the file.
	ParentHash string
}

// Changing a collection's own definition: what it is called, what it is drawn
// with, what fields it offers, and what its records say they are.
//
// The flags are there because "" is a thing somebody means: clearing the icon
// is a change, and leaving the icon alone is not.
type RecordCollectionUpdate struct {
	// The container heading to change.
	Hash string

	Type   string
	Name   string
	Icon   string
	Fields []string

	SetType   bool
	SetName   bool
	SetIcon   bool
	SetFields bool
}

// What changing a collection did. Renaming the *type* rewrites the RECORD
// property of every record that carried the old one, so that renaming a
// collection keeps the things in it - Moved says how many that was.
type RecordCollectionUpdateResult struct {
	Ok    bool
	Msg   string
	Type  string
	Moved int
}

// One birthday, on one date, for the agenda.
//
// A birthday is a yearly thing and a record carries the day it started, so the
// server works out which occurrence falls in the window asked for rather than
// handing over the stored date and leaving the client to do calendar
// arithmetic.
type BirthdayEvent struct {
	Hash     string
	Name     string
	Type     string
	Filename string
	LineNum  int
	// The occurrence, as YYYY-MM-DD.
	Date string
	// What the record says, as it is written: "1990-05-15" or "--05-15".
	Birthday string
	// How old they turn on Date, or 0 when the record gives no year.
	Age int
	// Which property this came off: "BIRTHDAY", "ANNIVERSARY".
	Field string
	Image string
}

// The answer to "what fields do records of this type actually use?", which is
// what an add form offers before anybody has typed anything.
type RecordFieldUse struct {
	Key   string
	Name  string
	Label string
	Kind  string
	// How many records of the type carry it, so the form can put the common
	// ones first.
	Count int
}
