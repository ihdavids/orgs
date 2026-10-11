package focus

// How a session is said in a terminal. Pure, so focus_test.go can pin it.

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// Countdown is seconds as a clock face: 4:07, 25:00, 1:02:03. Rounded up, so
// the last second reads 0:01 rather than 0:00 while there is still time.
func Countdown(secs float64) string {
	n := int(math.Ceil(secs))
	if n < 0 {
		n = 0
	}
	h, m, s := n/3600, (n/60)%60, n%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// Mins is a length of time as a person says it: 25 min, 1h 40m.
func Mins(m float64) string {
	if m < 60 {
		if m == math.Trunc(m) {
			return fmt.Sprintf("%d min", int(m))
		}
		return fmt.Sprintf("%.1f min", m)
	}
	n := int(math.Round(m))
	return fmt.Sprintf("%dh %02dm", n/60, n%60)
}

func pomodoros(n int) string {
	if n == 1 {
		return "1 pomodoro"
	}
	return fmt.Sprintf("%d pomodoros", n)
}

// Glyph is the phase in one character: a tomato working, a cup on a break.
func Glyph(phase string) string {
	switch phase {
	case "work":
		return "🍅"
	case "break":
		return "☕"
	}
	return ""
}

func phaseName(st common.FocusState) string {
	if st.Phase == "break" {
		if st.LongBreak {
			return "long break"
		}
		return "break"
	}
	return st.Phase
}

// StatusLine is the one line for tmux or a prompt, empty when idle so a
// status bar shows nothing rather than a placeholder.
func StatusLine(st common.FocusState, colour bool) string {
	switch st.Phase {
	case "work":
		return fmt.Sprintf("%s %s %s", Glyph("work"), Countdown(st.Remaining), ellipsis(st.Title, 40))
	case "break":
		return fmt.Sprintf("%s %s %s", Glyph("break"), Countdown(st.Remaining), phaseName(st))
	}
	return ""
}

// Progress is a bar of width cells, filled for the part of the phase gone by.
func Progress(st common.FocusState, remaining float64, width int) string {
	if width <= 0 || st.Total <= 0 {
		return ""
	}
	done := 1 - remaining/st.Total
	if done < 0 {
		done = 0
	}
	if done > 1 {
		done = 1
	}
	fill := int(math.Round(done * float64(width)))
	return strings.Repeat("━", fill) + strings.Repeat("─", width-fill)
}

func ellipsis(s string, n int) string {
	r := []rune(s)
	if n <= 1 || len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func parseTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}
