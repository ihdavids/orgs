package common

// org-review (features.review): headings read again on a cadence. The server
// half is internal/app/orgs/orgreview.go; this is the settings block and what
// goes over the wire, so `orgs reviews` can read it without the server.

import "strings"

// OrgReviewSettings is the `orgReview:` block. (`review:` is the weekly review
// wizard's, review.go.)
type OrgReviewSettings struct {
	// The cadence of a heading with no REVIEW_DELAY of its own or above it
	// (org-review-delay). Default +1m.
	Delay string `yaml:"delay"`
	// When marking a heading reviewed writes NEXT_REVIEW
	// (org-review-sets-next-date): always (default), only-future (only when
	// it moves the date later) or never.
	SetsNext string `yaml:"setsNext"`
	// inactive (default, [2026-11-04 Wed]) or active (<2026-11-04 Wed>, what
	// org-review's own default writes).
	Stamp string `yaml:"stamp"`
	// Whether a heading that only inherits a REVIEW_DELAY, with no review
	// dates of its own, counts as due (org-review-unreviewed). Off: only a
	// heading with LAST_REVIEW, NEXT_REVIEW or its own REVIEW_DELAY takes part.
	Unreviewed bool `yaml:"unreviewed"`
}

func (s OrgReviewSettings) DelayOr() string {
	if d := strings.TrimSpace(s.Delay); d != "" {
		return d
	}
	return "+1m"
}

func (s OrgReviewSettings) SetsNextOr() string {
	switch strings.ToLower(strings.TrimSpace(s.SetsNext)) {
	case "only-future", "onlyfuture", "future":
		return "only-future"
	case "never", "nil", "no", "false":
		return "never"
	}
	return "always"
}

// OrgReviewItem is one heading that takes part in review, with when it is due
// and why. Due, Last and Next are YYYY-MM-DD.
type OrgReviewItem struct {
	ReviewItem
	// The day it falls due; "" for a heading never reviewed (due now).
	Due string
	// Days past Due: 0 is due today, negative is still to come.
	Late  int
	IsDue bool
	Last  string `json:",omitempty"`
	Next  string `json:",omitempty"`
	// The cadence it is on, and where that came from: own, the heading it
	// inherits from (its title), file, or default.
	Delay     string
	DelayFrom string
	Why       string
}

// OrgReviewList is GET /reviews.
type OrgReviewList struct {
	Ok    bool
	Off   bool `json:",omitempty"`
	Msg   string
	Today string
	// How many are due now; Items may hold upcoming ones too (within=N).
	Due   int
	Items []OrgReviewItem
}

// OrgReviewMark is POST /reviews/mark.
type OrgReviewMark struct {
	Hash string
	// The day it was reviewed: empty is today; a typed date (yesterday, fri).
	Date string `json:",omitempty"`
	// A note to log with it, in the heading's log drawer.
	Note string `json:",omitempty"`
}

// OrgReviewSet is POST /reviews/set. A nil field is left as it is; "" takes
// the property off.
type OrgReviewSet struct {
	Hash  string
	Delay *string `json:",omitempty"`
	Next  *string `json:",omitempty"`
	// Takes the heading out of review: LAST_REVIEW, NEXT_REVIEW and
	// REVIEW_DELAY all come off.
	Clear bool `json:",omitempty"`
}

// OrgReviewSnooze is POST /reviews/snooze: NEXT_REVIEW moved to Until (+1w
// from today, a typed date, an org stamp). Empty is +1w.
type OrgReviewSnooze struct {
	Hash  string
	Until string `json:",omitempty"`
}

// OrgReviewResult answers the three writes.
type OrgReviewResult struct {
	Ok   bool
	Off  bool `json:",omitempty"`
	Msg  string
	Hash string `json:",omitempty"` // the heading's hash after the write
	Item *OrgReviewItem `json:",omitempty"`
}
