package common

// AgendaView is a custom agenda: one or more sections shown together, as
// org's org-agenda-custom-commands and its block agendas
// (internal/app/orgs/agendaview.go).
type AgendaView struct {
	Name string `yaml:"name" json:"Name"`
	// The letter it is reached by, as in org's dispatcher (orgs agenda -view n).
	Key      string              `yaml:"key,omitempty" json:"Key,omitempty"`
	Sections []AgendaViewSection `yaml:"sections" json:"Sections"`
	// Built in (org's a, t, n, #) or from the server's yaml: not the
	// person's to change.
	ReadOnly bool `yaml:"-" json:"ReadOnly,omitempty"`
}

// AgendaViewSection is one block of a view.
type AgendaViewSection struct {
	// agenda (days), todo (open tasks), tags (a query: full, short or an org
	// match string), search (text in headings and bodies), stuck (projects
	// with nothing next).
	Kind  string `yaml:"kind" json:"Kind"`
	Title string `yaml:"title,omitempty" json:"Title,omitempty"`
	// Narrows the section (for tags, it is the section).
	Query string `yaml:"query,omitempty" json:"Query,omitempty"`
	// agenda: how many days (default 1), from today or, with start: monday,
	// from the start of the week.
	Days  int    `yaml:"days,omitempty" json:"Days,omitempty"`
	Start string `yaml:"start,omitempty" json:"Start,omitempty"`
	// todo: only these keywords, "NEXT|WAITING" (org's t with a keyword).
	Keywords string `yaml:"keywords,omitempty" json:"Keywords,omitempty"`
}

// AgendaViewPart is one section's answer: days for an agenda section, nodes
// for the rest.
type AgendaViewPart struct {
	Title  string
	Kind   string
	Agenda *AgendaResult `json:",omitempty"`
	Nodes  []AgendaNode  `json:",omitempty"`
	Msg    string        `json:",omitempty"`
}

type AgendaViewResult struct {
	Ok    bool
	Msg   string `json:",omitempty"`
	Name  string
	Parts []AgendaViewPart
}
