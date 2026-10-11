package common

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common/desktop"
)

// What GET /notify/status answers: how this server would tell somebody
// something, and whether it can.
type NotifyStatus struct {
	Ok  bool
	Msg string
	// The way this server's own machine shows a notification ("macOS
	// (terminal-notifier)"), whether or not it is used.
	Backend string
	// A notify plugin is in server.plugins: reminders and the deadline digest
	// run, and notifications are published as `notify` events.
	Plugin bool
	// It pops them on this server's machine too (desktop: true).
	Desktop bool
	// The quiet hours ("22:00-07:00"), and whether it is in them now. Quiet
	// hours hold back popups, never the events.
	Quiet    string
	QuietNow bool
	// Minutes before a timed appointment that a reminder goes out.
	Remind []int
	// When the day's deadline digest goes out ("08:30"), "" for never.
	Deadlines string
	Sound     bool
	// A focus session's phase ending is announced.
	Focus bool
	// Where a click on a notification goes: worg, at this address.
	Open string
}

// POST /notify/test answers with what happened to a test notification.
type NotifyTestResult struct {
	Ok      bool
	Msg     string
	Backend string
	// Popped on the server's machine, or only published (no plugin, desktop
	// off, or quiet hours).
	Popped bool
}

// --- urgency, on the wire ---------------------------------------------------

func UrgencyName(u desktop.Urgency) string {
	switch u {
	case desktop.Low:
		return "low"
	case desktop.Critical:
		return "critical"
	}
	return "normal"
}

func ParseUrgency(s string) desktop.Urgency {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return desktop.Low
	case "critical", "high":
		return desktop.Critical
	}
	return desktop.Normal
}

// --- quiet hours -------------------------------------------------------------

// A stretch of the day, as minutes after midnight. To may be before From, for
// a stretch across midnight (22:00-07:00).
type QuietHours struct {
	From, To int
	Set      bool
}

// ParseQuiet reads "22:00-07:00" (or "22-7", "22:30–06:15"). Empty is none.
func ParseQuiet(s string) (QuietHours, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "–", "-"))
	if s == "" {
		return QuietHours{}, nil
	}
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return QuietHours{}, fmt.Errorf("quiet hours are a range like 22:00-07:00, not %q", s)
	}
	from, err := clockMinutes(a)
	if err != nil {
		return QuietHours{}, err
	}
	to, err := clockMinutes(b)
	if err != nil {
		return QuietHours{}, err
	}
	return QuietHours{From: from, To: to, Set: from != to}, nil
}

// clockMinutes reads "7", "07:00" or "22:30" as minutes after midnight.
func clockMinutes(s string) (int, error) {
	s = strings.TrimSpace(s)
	h, m, _ := strings.Cut(s, ":")
	hh, err := strconv.Atoi(h)
	if err != nil || hh < 0 || hh > 24 {
		return 0, fmt.Errorf("%q is not a time of day", s)
	}
	mm := 0
	if m != "" {
		if mm, err = strconv.Atoi(m); err != nil || mm < 0 || mm > 59 {
			return 0, fmt.Errorf("%q is not a time of day", s)
		}
	}
	return (hh*60 + mm) % (24 * 60), nil
}

// In says whether a moment falls in the quiet hours.
func (q QuietHours) In(t time.Time) bool {
	if !q.Set {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	if q.From < q.To {
		return m >= q.From && m < q.To
	}
	return m >= q.From || m < q.To
}

// --- where a click goes ----------------------------------------------------------

// WorgLink is worg at base, asked to open on one application with these
// parameters: `?app=Files&open=<file>&line=N` lands on a heading.
func WorgLink(base string, params url.Values) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	if len(params) == 0 {
		return base + "/"
	}
	return base + "/?" + params.Encode()
}

// HeadingLink opens a heading in worg's Files tab: by line (0-based, as the
// server counts) where known, and by its text as well, which is what the
// rendered view can find.
func HeadingLink(base, file string, line int, heading string) string {
	if file == "" {
		return WorgLink(base, nil)
	}
	p := url.Values{"app": {"Files"}, "open": {file}}
	if line >= 0 {
		p.Set("line", strconv.Itoa(line))
	}
	if heading != "" {
		p.Set("heading", heading)
	}
	return WorgLink(base, p)
}
