package common

// The flashcard wire types: what /drill/cards answers with and /drill/review
// takes. Shared because two clients read them - worg's Flashcards tab
// (src/drill.ts says them again in TypeScript) and `orgs drill` - and the
// server's own copy is these. The algorithms stay on the server
// (internal/app/orgs/drill.go); the cloze syntax is here (drillcloze.go)
// because the terminal client draws clozes itself.

import "strings"

// DrillDeck is one set of flashcards: which headings are cards, and how they
// are drilled. Every number is org-drill's customisation of the same name, with
// its default; zero means "the default", so a deck saved before a field existed
// keeps working.
type DrillDeck struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Query picks the cards. StoredQuery names a saved search instead, and
	// wins when both are set, so editing the saved search edits the deck.
	Query       string `yaml:"query,omitempty" json:"query,omitempty"`
	StoredQuery string `yaml:"storedQuery,omitempty" json:"storedQuery,omitempty"`
	// Algorithm is sm5 (org-drill's default), sm2 or simple8.
	Algorithm string `yaml:"algorithm,omitempty" json:"algorithm,omitempty"`
	// org-drill-maximum-items-per-session and -maximum-duration (minutes).
	// Negative is unlimited, as nil is in Emacs.
	MaxItems   int `yaml:"maxItems,omitempty" json:"maxItems,omitempty"`
	MaxMinutes int `yaml:"maxMinutes,omitempty" json:"maxMinutes,omitempty"`
	// org-drill-item-count-includes-failed-items-p.
	CountFailed bool `yaml:"countFailed,omitempty" json:"countFailed,omitempty"`
	// org-drill-failure-quality: a rating at or below it is a failure.
	FailureQuality int `yaml:"failureQuality,omitempty" json:"failureQuality,omitempty"`
	// org-drill-learn-fraction (SM5 and Simple8).
	LearnFraction float64 `yaml:"learnFraction,omitempty" json:"learnFraction,omitempty"`
	// org-drill-forgetting-index: the session report warns when more than
	// this percentage was failed.
	ForgettingIndex int `yaml:"forgettingIndex,omitempty" json:"forgettingIndex,omitempty"`
	// org-drill-leech-failure-threshold and -leech-method (skip, warn, none).
	LeechThreshold int    `yaml:"leechThreshold,omitempty" json:"leechThreshold,omitempty"`
	LeechMethod    string `yaml:"leechMethod,omitempty" json:"leechMethod,omitempty"`
	// org-drill-days-before-old and -overdue-interval-factor.
	DaysBeforeOld  int     `yaml:"daysBeforeOld,omitempty" json:"daysBeforeOld,omitempty"`
	OverdueFactor  float64 `yaml:"overdueFactor,omitempty" json:"overdueFactor,omitempty"`
	CramHours      int     `yaml:"cramHours,omitempty" json:"cramHours,omitempty"`
	SM5Initial     float64 `yaml:"sm5Initial,omitempty" json:"sm5Initial,omitempty"`
	Noise          bool    `yaml:"noise,omitempty" json:"noise,omitempty"`
	HideTitles     bool    `yaml:"hideTitles,omitempty" json:"hideTitles,omitempty"`
	TypedAnswers   bool    `yaml:"typedAnswers,omitempty" json:"typedAnswers,omitempty"`
	ClozeLengthDot bool    `yaml:"clozeLengthDots,omitempty" json:"clozeLengthDots,omitempty"`
	// LapseOverdue is org-drill--lapse-very-overdue-entries-p: a card more
	// than 90 days overdue is treated as forgotten.
	LapseOverdue bool `yaml:"lapseOverdue,omitempty" json:"lapseOverdue,omitempty"`
	// Theme is the html theme the cards are drawn in; empty follows worg.
	Theme string `yaml:"theme,omitempty" json:"theme,omitempty"`
}

// WithDefaults fills in org-drill's defaults for whatever the deck left out.
func (d DrillDeck) WithDefaults() DrillDeck {
	if d.Algorithm == "" {
		d.Algorithm = "sm5"
	}
	d.Algorithm = strings.ToLower(d.Algorithm)
	if d.MaxItems == 0 {
		d.MaxItems = 30
	}
	if d.MaxMinutes == 0 {
		d.MaxMinutes = 20
	}
	if d.FailureQuality == 0 {
		d.FailureQuality = 2
	}
	if d.LearnFraction == 0 {
		d.LearnFraction = 0.5
	}
	if d.ForgettingIndex == 0 {
		d.ForgettingIndex = 10
	}
	if d.LeechThreshold == 0 {
		d.LeechThreshold = 15
	}
	if d.LeechMethod == "" {
		d.LeechMethod = "skip"
	}
	if d.DaysBeforeOld == 0 {
		d.DaysBeforeOld = 10
	}
	if d.OverdueFactor == 0 {
		d.OverdueFactor = 1.2
	}
	if d.CramHours == 0 {
		d.CramHours = 12
	}
	if d.SM5Initial == 0 {
		d.SM5Initial = 4.0
	}
	return d
}

// DrillData is org-drill's item data: (last-interval repeats failures
// total-repeats meanq ease). MeanQ and Ease are absent on a card never rated.
type DrillData struct {
	LastInterval float64  `json:"lastInterval"`
	Repeats      int      `json:"repeats"`
	Failures     int      `json:"failures"`
	TotalRepeats int      `json:"totalRepeats"`
	MeanQ        *float64 `json:"meanQ,omitempty"`
	Ease         *float64 `json:"ease,omitempty"`
}

// DrillSide is one direct child heading of a card.
type DrillSide struct {
	Name   string `json:"name"`  // its title, as text, for card types that look for one by name
	Title  string `json:"title"` // its title, as html
	Body   string `json:"body"`  // everything under it, as html
	IsCard bool   `json:"isCard"`
}

// DrillCard is one card, ready to show.
type DrillCard struct {
	Hash  string   `json:"hash"`
	File  string   `json:"file"`
	Line  int      `json:"line"`
	Name  string   `json:"name"` // the title as text
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Type  string   `json:"type"`
	// Status is new, failed, overdue, young, old, future or skipped.
	Status       string    `json:"status"`
	Due          int       `json:"due"` // days overdue; negative is days to go
	Age          float64   `json:"age"` // days since added, for ordering lapsed cards
	Leech        bool      `json:"leech"`
	Data         DrillData `json:"data"`
	LastQuality  int       `json:"lastQuality"` // -1: never rated
	LastReviewed string    `json:"lastReviewed,omitempty"`
	Scheduled    string    `json:"scheduled,omitempty"`
	Weight       float64   `json:"weight,omitempty"`
	// Next is the days until the card comes round again for each rating 0-5.
	Next   [6]float64  `json:"next"`
	Body   string      `json:"body"`   // the question text, as html, clozes wrapped
	Clozes int         `json:"clozes"` // how many clozes the body itself has
	Sides  []DrillSide `json:"sides"`
	// Explain is the body of each ancestor tagged :explain:, outermost
	// first: org-drill's explanation, shown with the answer.
	Explain []string `json:"explain,omitempty"`
	// Props are the inherited properties the generated card types read
	// (conjugate, decline_noun).
	Props map[string]string `json:"props,omitempty"`
}

// DrillCounts is the deck at a glance.
type DrillCounts struct {
	New         int `json:"new"`
	Failed      int `json:"failed"`
	Overdue     int `json:"overdue"`
	Young       int `json:"young"`
	Old         int `json:"old"`
	Future      int `json:"future"`
	Skipped     int `json:"skipped"`
	DueTomorrow int `json:"dueTomorrow"`
	Total       int `json:"total"`
}

type DrillCardsReply struct {
	Ok     bool        `json:"ok"`
	Msg    string      `json:"msg"`
	Deck   DrillDeck   `json:"deck"`
	Query  string      `json:"query"`
	Cards  []DrillCard `json:"cards"`
	Counts DrillCounts `json:"counts"`
	// Problems are cards left out, and why: an unknown card type, an empty
	// body. Said rather than silently dropped, as org-drill says them.
	Problems []string `json:"problems,omitempty"`
	Style    string   `json:"style,omitempty"`
}

// DrillReview is one rating.
type DrillReview struct {
	Hash    string `json:"hash"`
	Quality int    `json:"quality"`
	// Deck names the saved deck whose settings apply; Algorithm stands in
	// for an unsaved one.
	Deck      string `json:"deck"`
	Algorithm string `json:"algorithm"`
}

type DrillReviewReply struct {
	Ok        bool      `json:"ok"`
	Msg       string    `json:"msg"`
	Days      float64   `json:"days"`
	Scheduled string    `json:"scheduled,omitempty"`
	Data      DrillData `json:"data"`
	Leech     bool      `json:"leech"`
	Failed    bool      `json:"failed"`
}
