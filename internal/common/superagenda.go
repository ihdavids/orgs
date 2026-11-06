package common

// AgendaGroup is one of org-super-agenda's groups: a name and the selectors
// that put an item in it. An item goes to the first group (in the order
// written) any of whose selectors matches it - several selectors in one group
// are alternatives, as in org-super-agenda; And narrows. Items no group takes
// go to "Other items" at the end.
//
// In the yaml (`agenda.groups`, or a view section's `groups`):
//
//	- name: Today
//	  timeGrid: true
//	- name: Overdue
//	  deadline: past
//	  scheduled: past
//	- name: Important
//	  priority: A
//	- auto: category
//	- name: Waiting
//	  todo: [WAITING]
//	  order: 9
//	- discard: true
//	  tag: [someday]
type AgendaGroup struct {
	Name string `yaml:"name,omitempty" json:"Name,omitempty"`
	// Groups are shown by Order, low first, then as written; Other items is 99.
	Order int `yaml:"order,omitempty" json:"Order,omitempty"`
	// The items this group takes are dropped rather than shown.
	Discard bool `yaml:"discard,omitempty" json:"Discard,omitempty"`

	// One group per value of: category, tags (each item under its first
	// tag), tag (alias), todo, priority, file, parent, outline (the path
	// above), planning (scheduled, deadline, both), dir, or property:NAME.
	// Groups are named by the value and sorted by it; items with no value go
	// on to the next group.
	Auto string `yaml:"auto,omitempty" json:"Auto,omitempty"`

	Anything  bool     `yaml:"anything,omitempty" json:"Anything,omitempty"`
	TimeGrid  bool     `yaml:"timeGrid,omitempty" json:"TimeGrid,omitempty"`   // a timed entry on its day
	Tag       []string `yaml:"tag,omitempty" json:"Tag,omitempty"`             // any of these, inherited tags included
	Todo      []string `yaml:"todo,omitempty" json:"Todo,omitempty"`           // any of these keywords; [t] any active one
	Priority  string   `yaml:"priority,omitempty" json:"Priority,omitempty"`   // A, >B (higher than B), <B, or A,B
	Deadline  string   `yaml:"deadline,omitempty" json:"Deadline,omitempty"`   // past, today, future, any, none, or a day
	Scheduled string   `yaml:"scheduled,omitempty" json:"Scheduled,omitempty"` // as Deadline
	Date      string   `yaml:"date,omitempty" json:"Date,omitempty"`           // a plain timestamp: as Deadline
	Habit     bool     `yaml:"habit,omitempty" json:"Habit,omitempty"`
	Category  []string `yaml:"category,omitempty" json:"Category,omitempty"`
	File      string   `yaml:"file,omitempty" json:"File,omitempty"`         // regexp over the file's path
	Property  []string `yaml:"property,omitempty" json:"Property,omitempty"` // NAME (has it) or NAME=value
	Regexp    string   `yaml:"regexp,omitempty" json:"Regexp,omitempty"`     // regexp over the heading and its properties
	Heading   string   `yaml:"heading,omitempty" json:"Heading,omitempty"`   // regexp over the title
	Children  string   `yaml:"children,omitempty" json:"Children,omitempty"` // any, todo, none
	EffortMin string   `yaml:"effortMin,omitempty" json:"EffortMin,omitempty"` // EFFORT at least 1:00
	EffortMax string   `yaml:"effortMax,omitempty" json:"EffortMax,omitempty"` // EFFORT under 0:30
	// An orgs query (full or short form, {{ filters }} allowed): the items
	// it finds.
	Query string `yaml:"query,omitempty" json:"Query,omitempty"`

	// Every one of these must match too (org-super-agenda's :and).
	And []AgendaGroup `yaml:"and,omitempty" json:"And,omitempty"`
	// None of these may match (:not). On its own: everything they don't take.
	Not []AgendaGroup `yaml:"not,omitempty" json:"Not,omitempty"`
}

// AgendaGroupItems is one group as answered: its name and the items in it,
// as indexes into the day's Entries (an agenda day) or the Nodes (a list).
type AgendaGroupItems struct {
	Name  string
	Items []int
}
