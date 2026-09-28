package commands

// One heading, drawn for a terminal. Shared rather than written twice because
// `orgs search` and `orgs tui` are the same list read two ways, and a heading
// that reads differently in the two of them is two headings as far as anybody
// looking at both is concerned.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// StatusColour is how a keyword is drawn. The server does not say what a
// keyword means beyond active or finished, so this is the same small guess
// worg's statuslook.tsx makes: the finished ones go quiet, the ones that are
// waiting on somebody else go amber, and the one you are meant to do next is
// the one that stands out.
func StatusColour(status string) string {
	switch strings.ToUpper(status) {
	case "DONE", "CANCELLED", "CANCELED":
		return AnsiDim
	case "NEXT":
		return AnsiGreen
	case "WAITING", "BLOCKED", "HOLD":
		return AnsiGold
	case "":
		return ""
	}
	return AnsiCyan
}

// PriorityColour: A is the one worth a colour, B and C are not worth shouting
// about, and anything else is somebody's own scheme and gets the middle one.
func PriorityColour(p string) string {
	switch strings.ToUpper(p) {
	case "A":
		return AnsiRed
	case "B":
		return AnsiGold
	case "C":
		return AnsiBlue
	}
	return AnsiPurple
}

// TodoLine is one heading on one line: keyword, priority, headline, tags, and
// where it lives. Width is the terminal's, and 0 means do not trim.
func TodoLine(t common.Todo, width int) string {
	var b strings.Builder
	if t.Status != "" {
		fmt.Fprintf(&b, "%s%-8s%s ", C(StatusColour(t.Status)), t.Status, C(AnsiReset))
	} else {
		b.WriteString(strings.Repeat(" ", 9))
	}
	if t.Priority != "" {
		fmt.Fprintf(&b, "%s[#%s]%s ", C(PriorityColour(t.Priority)), t.Priority, C(AnsiReset))
	}
	b.WriteString(t.Headline)
	if len(t.Tags) > 0 {
		fmt.Fprintf(&b, " %s:%s:%s", C(AnsiPurple), strings.Join(t.Tags, ":"), C(AnsiReset))
	}
	if d := DateOf(t); d != "" {
		fmt.Fprintf(&b, " %s%s%s", C(AnsiBlue), d, C(AnsiReset))
	}
	fmt.Fprintf(&b, " %s%s:%d%s", C(AnsiDim), filepath.Base(t.Filename), t.LineNum, C(AnsiReset))
	return b.String()
}

// DateOf is the date a heading is about, said shortly: the deadline if it has
// one, otherwise whatever it is scheduled or stamped for. A heading with both
// is nearly always about the deadline, and the deadline is marked so the two
// cannot be read as the same thing.
func DateOf(t common.Todo) string {
	if t.Deadline != nil && !t.Deadline.IsZero() {
		return "!" + t.Deadline.Start.Format("2006-01-02")
	}
	if t.Date != nil && !t.Date.IsZero() {
		return t.Date.Start.Format("2006-01-02")
	}
	return ""
}
