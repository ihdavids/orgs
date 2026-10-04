package common

// Org's column view, as data.
//
// A column view is a `#+COLUMNS:` line saying which properties to show, and a
// table of every heading in a file with those properties filled in - with one
// thing that makes it more than a table: a column may name a **summary
// operator**, and then a parent heading shows the total of what is under it.
// That is what turns `EFFORT` from a number somebody typed on a leaf into the
// size of a project.

// One column of a `#+COLUMNS:` line.
//
// `%25ITEM %TODO %EFFORT(Estimate){:}` is three of these: the item at 25
// characters, the keyword, and EFFORT titled "Estimate" and summed as a
// duration.
type ColumnSpec struct {
	// The property, upper-cased, or one of org's special names (ITEM, TODO,
	// PRIORITY, TAGS, CLOCKSUM, ...).
	Property string
	// What to put at the head of the column - the `(Estimate)` part, or the
	// property itself when it has none.
	Title string
	// The `%25` part. Zero means "as wide as it needs to be", which is the
	// client's business rather than the server's.
	Width int
	// The `{:}` part as written: the summary operator, or empty for a column
	// that does not roll up.
	Summary string
	// What a cell of this column holds, so the client knows how to draw and
	// edit it without a table of property names of its own: "property", "todo",
	// "item", "tags", "priority", or "derived" for one that is worked out and
	// cannot be written.
	Kind string
	// How the values are read for summing: "duration", "number", or "" for a
	// column that is not summed.
	Numbers string
	// The values this property is allowed to take, from org's own
	// `<PROPERTY>_ALL` mechanism - `#+PROPERTY: Effort_ALL 0 0:10 0:30 1:00` or
	// an `:EFFORT_ALL:` property on a heading. Empty when the file says nothing,
	// in which case anything may be typed.
	Allowed []string
}

// What one heading holds in one column.
type ColumnCell struct {
	// What to show: the heading's own value, or the rollup when this column is
	// summed and the heading has something under it.
	Value string
	// The value written on this heading itself, which is not the same thing as
	// what is shown and is what an edit changes. A project showing 12:00 of
	// effort summed from its tasks has, nearly always, no effort of its own -
	// and an editor that offered 12:00 as the current value would write the
	// total onto the parent the moment anybody touched it.
	Own string
	// Whether Value came from adding things up rather than from this heading.
	Summed bool
	// The value as a number, so the client can sort a column without having to
	// parse durations the same way the server does.
	Number float64
}

// One heading.
type ColumnRow struct {
	Hash        string
	Headline    string
	Level       int
	LineNum     int
	Status      string
	Priority    string
	Tags        []string
	HasChildren bool
	// One per spec entry, in the spec's order.
	Cells []ColumnCell
}

// The answer to GET /columns.
type ColumnsResult struct {
	Ok   bool
	Msg  string
	File string
	// The `#+COLUMNS:` line in force, and where it came from: "file" when the
	// file declared one, "heading" for a `:COLUMNS:` property, "config" for the
	// server default, "request" when the caller asked for a particular one.
	Columns string
	From    string
	Spec    []ColumnSpec
	Rows    []ColumnRow
}

// ColumnSettings is the column view's server-side configuration.
type ColumnSettings struct {
	// The `#+COLUMNS:` line used for a file that does not declare one.
	Default string `yaml:"default"`
}

// One value a property takes in a file, and on how many headings.
type ColumnValueCount struct {
	Value string
	Count int
}

// One property used in a file: on how many headings, and its values.
type ColumnPropValues struct {
	Name   string
	Count  int
	Values []ColumnValueCount
}

// The answer to GET /columns/values: every property the file's headings use,
// with the values each takes - for listing a property's values, for offering
// the values already in use while one is typed, and for suggesting property
// names while a columns line is written.
type ColumnValuesResult struct {
	Ok    bool
	Msg   string
	File  string
	Props []ColumnPropValues
}
