package common

import (
	"time"

	"github.com/ihdavids/go-org/org"
)

type OrgFile struct {
	Filename string
	Doc      *org.Document
}

type Empty struct{}

type FileList []string

type Date string

func (self *Date) Set(dt time.Time) {
	*self = Date(dt.Format("2006-02-01"))
}

func (self *Date) Get() (time.Time, error) {
	return time.Parse("2006-02-01", string(*self))
}

type Todo struct {
	Headline string
	Tags     []string
	Props    map[string]string
	Hash     string
	Date     *org.OrgDate
	Status   string
	Filename string
	LineNum  int
	IsActive bool
	Parent   string
	Level    int
}

func (self Todo) Is(other Todo) bool {
	return self.LineNum == other.LineNum && self.Filename == other.Filename && self.Headline == other.Headline
}

type FullTodo struct {
	Headline string
	Content  string
	Tags     []string
	Props    map[string]string
	Hash     string
	Priority string
}

type TodoHash string
type TodoItemChange struct {
	Hash  string
	Value string
}

type TodoPropertyChange struct {
	Hash  string
	Name  string
	Value string
}

type Todos []Todo

type StringQuery struct {
	Query string `yaml:"query"`
}
type Result struct {
	Ok bool `yaml:"status"`
}

type ResultMsg struct {
	Ok  bool    `yaml:"status"`
	Msg string  `yaml:"msg"`
	Pos org.Pos `yaml:"pos"`
	End org.Pos `yaml:"end"`
}

type CellDimensions struct {
	Start org.Pos
	End   org.Pos
}
type TableFormulaDetails struct {
	Targets  [][]CellDimensions
	Formulas []CellDimensions
}

type ResultTableDetailsMsg struct {
	Ok      bool                `yaml:"status"`
	Msg     string              `yaml:"msg"`
	Pos     org.Pos             `yaml:"pos"`
	End     org.Pos             `yaml:"end"`
	Details TableFormulaDetails `yaml:"details"`
}

type ListResult struct {
	Vals []string
}

type TodoStatesResult struct {
	Active []string
	Done   []string
}

type ExportToFile struct {
	Name     string
	Query    string
	Filename string
	Opts     string
}

type NewNode struct {
	Headline string
	Content  string
	Tags     []string
	Props    map[string]string
	Priority string
}

// A capture can be from clipboard or have all the data itself.
// Data is inserted as per the specifications of the template
type Capture struct {
	Template string
	NewNode  NewNode
}

type Target struct {
	Filename string
	Id       string
	// File+ types use file and id fields except for line
	// id, customid and hash all just use the id field
	Type string // file+headline, id, customid, hash, file+line
	Lvl  int    // For heading matches if this is non-zero then this fixes the level we MUST match at
}

// A precise target has a target and a relative line offset within the headline.
// These are considered transient.
type PreciseTarget struct {
	Target Target
	Row    int
}

type CaptureTemplate struct {
	Name      string `yaml:"name"`     // "User Specified"
	Type      string `yaml:"type"`     // "entry"
	CapTarget Target `yaml:"target"`   // "file+headline"
	Template  string `yaml:"template"` // This is NOT used by orgs, this a suggestion for the calling program.
}

// TARGET TYPES
// file              "path/to/file"                               - Text will be placed at the beginning or end of that file.
// id                "id of existing org entry"                   - Filing as child of this entry, or in the body of the entry.
// customid          "customid of existing org entry"             - Filing as child of this entry, or in the body of the entry.
// file+headline     "filename" "node headline"                   - Fast configuration if the target heading is unique in the file.
// file+olp          "filename" "Level 1 heading" "Level 2" ...   - For non-unique headings, the full path is safer.
// file+regexp       "filename" "regexp to find location"         - Use a regular expression to position point.
// file+olp+datetree "filename" [ "Level 1 heading" ...]          - This target83 creates a heading in a date tree84 for today’s date. If the optional outline path is given, the tree will be built under the node it is pointing to, instead of at top level. Check out the :time-prompt and :tree-type properties below for additional options.
// clock                                                          - insert at position of active clock
// hash              dynamically assigned hash                    - during a run of the server nodes are dynamically assigned a hash
//                                                                  use the current dynamic hash to id the node

type Refile struct {
	FromId Target
	ToId   Target
}

// An Update is an updater plugin that takes a section and does something to it.
// Jira for instance might update jira.
type Update struct {
	Name   string
	Target Target
}

// Exclusive markers are tags that can only be on one heading at a time.
// (Unless you manually break that assertion)
// They act a little like named bookmarks
type ExclusiveTagMarker struct {
	ToId Target
	Name string
}
