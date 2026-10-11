package plan

// The day as text: a bar of the working window, the blocks in time order
// with the gaps between them, then what is left to place.
//
//	Mon 12 Oct  09:00 ▕█▒▒▒████████▓▓██          ████   ▏ 17:00   3h of 8h planned · 5h free
//
//	  09:00-09:15  Standup                        appointment
//	  10:00-11:30  TODO Write report
//	  11:00-11:45  TODO Review PR                 overlaps
//	               free 11:45-15:00  3h15
//	  ...
//
// One character of the bar is a quarter hour, so an ordinary working day is
// about thirty characters and fits beside the numbers.

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// Mins is a length the way the planner says one: 45m, 2h, 1h30.
func Mins(m int) string {
	if m < 0 {
		m = 0
	}
	switch {
	case m < 60:
		return fmt.Sprintf("%dm", m)
	case m%60 == 0:
		return fmt.Sprintf("%dh", m/60)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}

func kw(status string) string {
	if status == "" {
		return ""
	}
	return commands.C(commands.AnsiGold) + status + commands.C(commands.AnsiReset) + " "
}

func taskNote(t common.PlanTask) string {
	switch {
	case t.Late > 0:
		return fmt.Sprintf("%dd late", t.Late)
	case t.Late == 0:
		return "today"
	case t.Deadline != "" && t.DueIn <= 0:
		return "due!"
	case t.Deadline != "":
		return fmt.Sprintf("due %dd", t.DueIn)
	}
	return ""
}

// The working window as one bar: ' ' free, █ planned, ▓ double-booked, ▏ a
// deadline, ▒ the past (today only).
func Bar(pd common.PlanDay, now time.Time) string {
	ds, de := clockMins(pd.DayStart), clockMins(pd.DayEnd)
	if ds < 0 || de <= ds {
		return ""
	}
	n := (de - ds + 14) / 15
	cover := make([]int, n)
	marks := make([]bool, n)
	for _, b := range pd.Blocks {
		s, e := clockMins(b.Start), clockMins(b.End)
		if b.End == "24:00" {
			e = 24 * 60
		}
		if b.Kind == common.AgendaDeadline {
			if i := (s - ds) / 15; s >= ds && i < n {
				marks[i] = true
			}
			continue
		}
		for i := 0; i < n; i++ {
			a, z := ds+i*15, ds+(i+1)*15
			if s < z && e > a {
				cover[i]++
			}
		}
	}
	past := -1
	if pd.Date == now.Format("2006-01-02") {
		past = (now.Hour()*60 + now.Minute() - ds) / 15
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		switch {
		case marks[i]:
			sb.WriteString(commands.C(commands.AnsiRed) + "▏" + commands.C(commands.AnsiReset))
		case cover[i] >= 2:
			sb.WriteString(commands.C(commands.AnsiRed) + "▓" + commands.C(commands.AnsiReset))
		case cover[i] == 1:
			sb.WriteString(commands.C(commands.AnsiBlue) + "█" + commands.C(commands.AnsiReset))
		case i < past:
			sb.WriteString(commands.C(commands.AnsiDim) + "▒" + commands.C(commands.AnsiReset))
		default:
			sb.WriteString("·")
		}
	}
	return sb.String()
}

// The gaps inside the window, from free minutes on.
func gaps(pd common.PlanDay) [][2]int {
	ds, de := clockMins(pd.DayStart), clockMins(pd.DayEnd)
	busy := make([]bool, 24*60+1)
	for _, b := range pd.Blocks {
		s, e := clockMins(b.Start), clockMins(b.End)
		if b.End == "24:00" {
			e = 24 * 60
		}
		for m := s; m < e && m < len(busy); m++ {
			busy[m] = true
		}
	}
	out := [][2]int{}
	start := -1
	for m := ds; m <= de; m++ {
		free := m < de && !busy[m]
		if free && start < 0 {
			start = m
		}
		if !free && start >= 0 {
			out = append(out, [2]int{start, m})
			start = -1
		}
	}
	return out
}

// Draw is the whole day for a person. c is commands.C, passed in so a test
// can draw without colour.
func Draw(pd common.PlanDay, now time.Time, c func(string) string) string {
	var sb strings.Builder
	day, _ := time.Parse("2006-01-02", pd.Date)
	l := pd.Load
	summary := fmt.Sprintf("%s of %s planned · %s free", Mins(l.PlannedMins), Mins(l.WindowMins), Mins(l.FreeMins))
	if l.DoubleMins > 0 {
		summary += c(commands.AnsiRed) + fmt.Sprintf(" · %s double-booked", Mins(l.DoubleMins)) + c(commands.AnsiReset)
	}
	if l.OutsideMins > 0 {
		summary += fmt.Sprintf(" · %s outside hours", Mins(l.OutsideMins))
	}
	fmt.Fprintf(&sb, "%s%s%s  %s ▕%s▏ %s   %s\n\n", c(commands.AnsiBold), day.Format("Mon 2 Jan"), c(commands.AnsiReset),
		pd.DayStart, Bar(pd, now), pd.DayEnd, summary)

	overlapping := map[int]bool{}
	for _, o := range pd.Overlaps {
		overlapping[o[0]], overlapping[o[1]] = true, true
	}
	type row struct {
		at   int
		line string
	}
	rows := []row{}
	for i, b := range pd.Blocks {
		when := b.Start + "-" + b.End
		if b.Kind == common.AgendaDeadline {
			when = b.Start + "      "
		}
		notes := []string{}
		switch b.Kind {
		case common.AgendaTimestamp:
			notes = append(notes, "appointment")
		case common.AgendaDeadline:
			notes = append(notes, c(commands.AnsiRed)+"deadline"+c(commands.AnsiReset))
		}
		if b.Implied {
			notes = append(notes, "no end written")
		}
		if b.Repeater != "" {
			notes = append(notes, "repeats "+b.Repeater)
		}
		if overlapping[i] {
			notes = append(notes, c(commands.AnsiRed)+"overlaps"+c(commands.AnsiReset))
		}
		title := kw(b.Status) + b.Headline
		if b.Done {
			title = c(commands.AnsiDim) + b.Status + " " + b.Headline + c(commands.AnsiReset)
		}
		ink := commands.AnsiBlue
		if !b.Movable {
			ink = commands.AnsiDim
		}
		rows = append(rows, row{clockMins(b.Start), fmt.Sprintf("  %s%s%s  %s  %s%s%s", c(ink), when, c(commands.AnsiReset), title,
			c(commands.AnsiDim), strings.Join(notes, " · "), c(commands.AnsiReset))})
	}
	for _, g := range gaps(pd) {
		if g[1]-g[0] < 15 {
			continue
		}
		rows = append(rows, row{g[0], fmt.Sprintf("  %s             free %s-%s  %s%s", c(commands.AnsiGreen), Clock(g[0]), Clock(g[1]),
			Mins(g[1]-g[0]), c(commands.AnsiReset))})
	}
	// Stable by time: blocks come before a gap starting at the same minute,
	// which never happens, so a plain insertion sort is enough.
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].at < rows[j-1].at; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	for _, r := range rows {
		sb.WriteString(r.line + "\n")
	}
	if len(pd.AllDay) > 0 {
		sb.WriteString("\n")
		for _, b := range pd.AllDay {
			note := b.Kind
			if b.Kind == common.AgendaDeadline {
				switch {
				case b.Late > 0:
					note = fmt.Sprintf("deadline %dd ago", b.Late)
				case b.DueIn > 0:
					note = fmt.Sprintf("deadline in %dd", b.DueIn)
				default:
					note = "deadline today"
				}
			}
			fmt.Fprintf(&sb, "  %sall day%s      %s%s  %s%s%s\n", c(commands.AnsiDim), c(commands.AnsiReset), kw(b.Status), b.Headline,
				c(commands.AnsiDim), note, c(commands.AnsiReset))
		}
	}

	if len(pd.Tasks) == 0 {
		sb.WriteString("\nNothing left to place.\n")
		return sb.String()
	}
	backlog := Mins(l.BacklogMins)
	if l.BacklogNoEst > 0 {
		backlog += fmt.Sprintf(" + %d unestimated", l.BacklogNoEst)
	}
	fmt.Fprintf(&sb, "\n%sTo place%s (%d · %s)\n", c(commands.AnsiBold), c(commands.AnsiReset), len(pd.Tasks), backlog)
	for _, t := range pd.Tasks {
		size := "  -  "
		if t.EffortMins > 0 {
			size = Mins(t.EffortMins)
		}
		note := taskNote(t)
		ink := commands.AnsiDim
		if t.Late > 0 || (t.Deadline != "" && t.DueIn <= 0) {
			ink = commands.AnsiRed
		}
		fmt.Fprintf(&sb, "  %s%-8s%s %5s  %s%s  %s%s%s\n", c(ink), note, c(commands.AnsiReset), size, kw(t.Status), t.Headline,
			c(commands.AnsiDim), filepath.Base(t.Filename), c(commands.AnsiReset))
	}
	return sb.String()
}
