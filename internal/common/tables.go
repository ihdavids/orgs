package common

// Wire types for the table browser and spreadsheet editor.
// These are shared between the server and any client that wants to list
// or edit the org tables held in the database.

// A single row of an org table. Kind is "data" for a normal row and "sep"
// for a horizontal rule (|---+---|). Advanced carries the org "special row"
// marker of the row ("!", "^", "_", "$", "/") when it has one, so a client
// can render name/parameter rows differently. It is derived from the first
// cell and is ignored on write.
type TableRow struct {
	Kind     string
	Advanced string
	Cells    []string
}

// Summary of one table, used by the table list.
type TableInfo struct {
	Id       int      // Ordinal of the table within its file, in document order
	Filename string   // Absolute path of the org file holding the table
	Name     string   // #+NAME: of the table, empty when it has none
	Heading  string   // Outline path of the heading the table lives under
	Olp      []string // The same outline path, split into its parts
	Line     int      // Zero based line the table starts on
	Rows     int      // Row count, separators included
	Cols     int      // Column count
	Formulas int      // Number of formulas in the tables #+TBLFM: lines
}

// A table and everything needed to edit it.
type TableData struct {
	TableInfo
	EndLine      int               // Zero based last line of the table (its last #+TBLFM: line, when it has one)
	Data         []TableRow        // The rows themselves
	FormulaList  []string          // Individual formulas, e.g. "$3=$1*$2;%.2f"
	CellFormulas map[string]string // "row,col" (zero based, raw row index) -> the formula that writes that cell
	ColNames     map[string]int    // Column names declared by a "!" row
	Params       map[string]string // Parameters declared by a "$" row
	Indent       string            // Leading whitespace the table is written with
}

// Response for the table list.
type TableListResult struct {
	Ok     bool
	Msg    string
	Tables []TableInfo
}

// Response for a single table.
type TableResult struct {
	Ok    bool
	Msg   string
	Table *TableData
}

// Request body used to write a table back to its org file.
type TableEdit struct {
	Filename string     // File holding the table
	Id       int        // Ordinal of the table within that file
	Data     []TableRow // Replacement rows
	Formulas []string   // Replacement formula list, written as a single #+TBLFM: line
	Name     string     // New #+NAME: for the table, empty removes it
	SetName  bool       // Only touch the #+NAME: line when this is set
	Eval     bool       // Run the tables formulas after saving
}
