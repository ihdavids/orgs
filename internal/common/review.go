package common

// The wire types of inbox triage, the someday sweep and the weekly review
// wizard (internal/app/orgs/review_wizard.go). The CLI (`orgs triage`,
// `orgs review week`) and worg (the Inbox and Weekly Review tabs) read the same
// answers, so the rules of what is in the inbox, how long a WAITING item has
// waited and what counts as planned time live once, on the server.

import "strings"

// ReviewSettings is the `review:` block of the server config.
//
//	review:
//	  inbox: 'InFile("(^|/)inbox[.]org$") && !IsArchived() && !IsDone()'
//	  somedayKeywords: [SOMEDAY, MAYBE]
//	  somedayTags: [someday, maybe]
//	  waiting: 'IsStatus("WAITING") && !IsArchived()'
//	  waitingDays: 7
//
// Every field may be left out; the defaults are the ones above.
type ReviewSettings struct {
	// The inbox, as a query. Only the outermost heading a query finds is an
	// item: its children are part of it, and go wherever it goes.
	Inbox string `yaml:"inbox"`
	// What makes a heading a someday item: one of these keywords, or one of
	// these tags. Two lists rather than a query because promoting an item has
	// to undo exactly what made it someday - set a live keyword and take the
	// someday tag off - and a query cannot be run backwards.
	SomedayKeywords []string `yaml:"somedayKeywords"`
	SomedayTags     []string `yaml:"somedayTags"`
	// The waiting-for items, as a query.
	Waiting string `yaml:"waiting"`
	// How long something may wait before the review asks about it.
	WaitingDays int `yaml:"waitingDays"`
}

const DefaultInboxQuery = `InFile("(^|/)inbox[.]org$") && !IsArchived() && !IsDone()`
const DefaultWaitingQuery = `(IsStatus("WAITING") || IsStatus("WAIT")) && !IsArchived()`

func (self *ReviewSettings) InboxQuery() string {
	if self == nil || strings.TrimSpace(self.Inbox) == "" {
		return DefaultInboxQuery
	}
	return self.Inbox
}

func (self *ReviewSettings) WaitingQuery() string {
	if self == nil || strings.TrimSpace(self.Waiting) == "" {
		return DefaultWaitingQuery
	}
	return self.Waiting
}

func (self *ReviewSettings) Keywords() []string {
	if self == nil || len(self.SomedayKeywords) == 0 {
		return []string{"SOMEDAY", "MAYBE"}
	}
	return self.SomedayKeywords
}

func (self *ReviewSettings) Tags() []string {
	if self == nil || len(self.SomedayTags) == 0 {
		return []string{"someday", "maybe"}
	}
	return self.SomedayTags
}

func (self *ReviewSettings) WaitDays() int {
	if self == nil || self.WaitingDays <= 0 {
		return 7
	}
	return self.WaitingDays
}

// SomedayQuery is the someday lists as a query: any of the keywords, or any
// of the tags (inherited tags count, and the outermost rule keeps a tagged
// project's tasks with it).
func (self *ReviewSettings) SomedayQuery() string {
	parts := []string{}
	for _, k := range self.Keywords() {
		parts = append(parts, `IsStatus("`+k+`")`)
	}
	for _, t := range self.Tags() {
		parts = append(parts, `HasTags("`+t+`")`)
	}
	return "(" + strings.Join(parts, " || ") + ") && !IsArchived() && !IsDone()"
}

// ReviewItem is one heading as a review shows it: the heading, and what a
// person needs to decide about it without opening the file.
type ReviewItem struct {
	Todo
	// The outline path above the heading, outermost first.
	Olp []string
	// The prose under it: planning lines and drawers taken off, cut short.
	Body string
	// How many headings are under it. They go wherever it goes.
	Children int
	Effort   string `json:",omitempty"`
	// The SCHEDULED day, YYYY-MM-DD, when there is one.
	Scheduled string `json:",omitempty"`
	// Waiting items: the day it started waiting, where that was read from
	// (logbook, created, date), and the last time it was chased.
	Since     string `json:",omitempty"`
	SinceFrom string `json:",omitempty"`
	Chased    string `json:",omitempty"`
	// Waiting: days since it started waiting or was last chased. Stale: days
	// since it was scheduled. -1 when nothing says.
	Days int
}

type ReviewList struct {
	Ok    bool
	Msg   string
	Query string
	Items []ReviewItem
}

// A project with nothing to do next, and the open tasks under it - any of
// which could be made NEXT on the spot.
type StuckProject struct {
	Project ReviewItem
	Open    []ReviewItem
}

// One row of the time comparison: a project (or a file, for clocked time with
// no project above it) or a tag.
type ReviewTimeRow struct {
	Name     string
	Kind     string // project, file or tag
	Hash     string `json:",omitempty"`
	Filename string `json:",omitempty"`
	Clocked  float64
	Planned  float64
	// Tasks planned for the week (scheduled or due in it), and how many of
	// those are finished.
	Tasks int
	Done  int
}

type ReviewTime struct {
	From  string // first day, YYYY-MM-DD
	To    string // last day, inclusive
	Label string
	// Minutes.
	Clocked  float64
	Planned  float64
	Projects []ReviewTimeRow
	Tags     []ReviewTimeRow
}

// WeeklyReview is the data for the wizard's middle steps.
type WeeklyReview struct {
	Ok          bool
	Msg         string
	Stuck       []StuckProject
	Waiting     []ReviewItem
	WaitingDays int
	Time        ReviewTime
	Stale       []ReviewItem
}

// ReviewState is what is kept per user: when the last review was finished,
// and the step a review in progress was left on.
type ReviewState struct {
	Last    string `yaml:"last,omitempty" json:"last,omitempty"`
	Step    int    `yaml:"step,omitempty" json:"step,omitempty"`
	Started string `yaml:"started,omitempty" json:"started,omitempty"`
	// What the review in progress has done so far, so a resumed review can
	// still say at the end what the whole of it did.
	Tally   map[string]int `yaml:"tally,omitempty" json:"tally,omitempty"`
	History []ReviewRecord `yaml:"history,omitempty" json:"history,omitempty"`
}

type ReviewRecord struct {
	Done    string         `yaml:"done" json:"done"`
	Summary map[string]int `yaml:"summary,omitempty" json:"summary,omitempty"`
}

type ReviewLast struct {
	Ok    bool
	Msg   string
	State ReviewState
	// Whole days since the last finished review; -1 for never.
	DaysAgo int
}

// ReviewAct is one decision about one heading.
//
// Actions: refile (To), archive, schedule / deadline (When: typed words like
// "fri" or "+2w", or an org stamp; empty clears), tags (Tags, the whole list),
// status (Keyword), promote (Keyword, and the someday tags off), next (Text: a
// new child heading on Keyword, NEXT by default), chase (Text: an optional
// note).
type ReviewAct struct {
	Hash    string
	Action  string
	To      *Target  `json:",omitempty"`
	When    string   `json:",omitempty"`
	Tags    []string `json:",omitempty"`
	Keyword string   `json:",omitempty"`
	Text    string   `json:",omitempty"`
}

type ReviewActResult struct {
	Ok  bool
	Msg string
	// Hand this to POST /review/undo to take the action back.
	Undo string `json:",omitempty"`
	// What was done, in words, for an undo button and a summary.
	Did string `json:",omitempty"`
}

type ReviewUndo struct {
	// Empty means the last action.
	Token string
}
