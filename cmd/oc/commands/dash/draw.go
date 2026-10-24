package dash

// Drawing the dashboard: a header, then cards with rounded corners. Two
// columns when the terminal is wide enough for both to breathe, one when it
// is not. Every row is built from a coloured left part, a plain title and a
// coloured right part, so the title is the one thing ever cut short and the
// columns stay aligned whatever the colours cost in bytes.

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/focus"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/mattn/go-runewidth"
)

const (
	wideAt  = 110 // two columns from here
	maxWide = 150 // past this the lines get too long to read across
	maxOne  = 96
	gap     = 2
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func vis(s string) int { return runewidth.StringWidth(ansiRe.ReplaceAllString(s, "")) }

func padRight(s string, w int) string {
	if n := vis(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func padLeft(s string, w int) string {
	if n := vis(s); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return s
}

func cut(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return runewidth.Truncate(s, w, "…")
}

// Colours, named for what they mean here rather than what they are.
func dim(s string) string   { return paint(commands.AnsiDim, s) }
func bold(s string) string  { return paint(commands.AnsiBold, s) }
func good(s string) string  { return paint(commands.AnsiGreen, s) }
func warn(s string) string  { return paint(commands.AnsiGold, s) }
func bad(s string) string   { return paint(commands.AnsiRed, s) }
func timeTone(s string) string { return paint(commands.AnsiCyan, s) }
func soft(s string) string  { return paint(commands.AnsiBlue, s) }

func paint(code, s string) string {
	if s == "" || commands.C(code) == "" {
		return s
	}
	return code + s + commands.AnsiReset
}

// row lays out left, a title cut to fit, and right flush to the edge.
func row(left, title string, style func(string) string, right string, w int) string {
	room := w - vis(left) - vis(right)
	if right != "" {
		room--
	}
	t := cut(title, room)
	if style != nil {
		t = style(t)
	}
	line := left + t
	if right == "" {
		return line
	}
	return padRight(line, w-vis(right)) + right
}

// card draws a box around lines, its title set into the top edge.
func card(title, note string, lines []string, w int) []string {
	head := dim("╭─ ") + bold(title)
	if note != "" {
		head += " " + dim(note)
	}
	fill := w - vis(head) - 2
	if fill < 1 {
		fill = 1
	}
	out := []string{head + " " + dim(strings.Repeat("─", fill)+"╮")}
	inner := w - 4
	for _, l := range lines {
		if vis(l) > inner {
			l = cut(ansiRe.ReplaceAllString(l, ""), inner)
		}
		out = append(out, dim("│")+" "+padRight(l, inner)+" "+dim("│"))
	}
	return append(out, dim("╰"+strings.Repeat("─", w-2)+"╯"))
}

func render(d Dash, width int, now time.Time) string {
	b := strings.Builder{}
	w := width
	two := w >= wideAt
	if two && w > maxWide {
		w = maxWide
	}
	if !two && w > maxOne {
		w = maxOne
	}

	header(&b, d, w, now)

	if d.Unreachable != "" {
		cw := w
		if cw > maxOne {
			cw = maxOne
		}
		for _, l := range card("Not connected", "", []string{
			bad("✗") + " Can't reach " + bold(d.Unreachable),
			dim(d.Errors["server"]),
			"",
			"Start it with " + bold("orgs serve") + ", or ask " + bold("orgs doctor") + " what is wrong.",
		}, cw) {
			b.WriteString(l + "\n")
		}
		return b.String()
	}
	if two {
		lw := (w - gap) * 11 / 20
		rw := w - gap - lw
		left := stack(nowCard(d, lw, now), todayCard(d, lw, now))
		right := stack(overdueCard(d, rw), habitsCard(d, rw))
		// The last card goes under whichever column is shorter, so a quiet
		// day does not leave one column hanging far below the other.
		if len(left) <= len(right) {
			left = stack(left, waitingCard(d, lw))
		} else {
			right = stack(right, waitingCard(d, rw))
		}
		for i := 0; i < len(left) || i < len(right); i++ {
			l, r := strings.Repeat(" ", lw), ""
			if i < len(left) {
				l = padRight(left[i], lw)
			}
			if i < len(right) {
				r = right[i]
			}
			b.WriteString(strings.TrimRight(l+strings.Repeat(" ", gap)+r, " ") + "\n")
		}
	} else {
		for _, l := range stack(nowCard(d, w, now), todayCard(d, w, now), overdueCard(d, w),
			habitsCard(d, w), waitingCard(d, w)) {
			b.WriteString(l + "\n")
		}
	}
	footer(&b, d, w)
	return b.String()
}

func stack(cards ...[]string) []string {
	out := []string{}
	for i, c := range cards {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, c...)
	}
	return out
}

func header(b *strings.Builder, d Dash, w int, now time.Time) {
	day := now
	if t, err := time.ParseInLocation("2006-01-02", d.Date, time.Local); err == nil {
		day = t
	}
	_, wk := day.ISOWeek()
	title := bold(day.Format("Monday 2 January")) + dim(fmt.Sprintf("  week %d", wk))
	clock := dim(now.Format("15:04"))
	b.WriteString("\n  " + padRight(title, w-4-vis(clock)) + clock + "\n")

	s := d.Summary
	chips := []string{}
	add := func(n int, one, many string, paint func(string) string) {
		if n <= 0 {
			return
		}
		word := many
		if n == 1 {
			word = one
		}
		chips = append(chips, paint(fmt.Sprintf("%d %s", n, word)))
	}
	add(s.Appointments, "appointment", "appointments", timeTone)
	add(s.ToDo, "to do", "to do", func(x string) string { return x })
	add(s.Overdue, "overdue", "overdue", bad)
	add(s.HabitsLeft, "habit left", "habits left", warn)
	add(s.Inbox, "in the inbox", "in the inbox", soft)
	add(s.Notices, "new notice", "new notices", soft)
	add(s.Done, "done", "done", good)
	if d.Unreachable != "" {
		b.WriteString("\n")
		return
	}
	if len(chips) == 0 && len(d.Errors) == 0 {
		chips = []string{good("A clear day ✓")}
	}
	for _, l := range wrap(chips, dim(" · "), w) {
		b.WriteString(l + "\n")
	}
	b.WriteString("\n")
}

// wrap joins pieces with sep into lines indented two, breaking between pieces
// rather than inside one, so a chip or a hint is never split across lines.
func wrap(pieces []string, sep string, w int) []string {
	out := []string{}
	line := "  "
	for _, p := range pieces {
		s := sep
		if line == "  " {
			s = ""
		}
		if vis(line)+vis(s)+vis(p) > w && line != "  " {
			out = append(out, line)
			line, s = "  ", ""
		}
		line += s + p
	}
	return append(out, line)
}

func nowCard(d Dash, w int, now time.Time) []string {
	inner := w - 4
	lines := []string{}
	if f := d.Focus; f != nil {
		left := focus.Glyph(f.Phase) + " " + bold(focus.Countdown(f.Remaining)) + dim(" left") + "  "
		what := f.Title
		if f.Phase == "break" {
			what = "break"
			if f.LongBreak {
				what = "long break"
			}
		}
		lines = append(lines, row(left, what, nil, "", inner))
		bar := focus.Progress(*f, f.Remaining, inner-3)
		lines = append(lines, "   "+paint(phaseColour(f.Phase), bar))
		if f.Today > 0 {
			lines = append(lines, "   "+dim(fmt.Sprintf("%d today · %s focused", f.Today, focus.Mins(f.TodayMinutes))))
		}
	}
	if c := d.Clock; c != nil && (d.Focus == nil || d.Focus.Phase != "work") {
		left := good("●") + " " + bold(elapsed(c.Mins)) + "  "
		lines = append(lines, row(left, c.Title, nil, dim("since "+c.Started.Format("15:04")), inner))
	} else if c != nil {
		lines = append(lines, "   "+dim("clocked in since "+c.Started.Format("15:04")))
	}
	if len(lines) == 0 {
		lines = append(lines, dim("○ Not clocked in"))
	}
	if msg, ok := d.Errors["clock"]; ok {
		lines = append(lines, dim("clock: "+msg))
	}
	return card("Now", "", lines, w)
}

func phaseColour(phase string) string {
	if phase == "break" {
		return commands.AnsiGreen
	}
	return commands.AnsiRed
}

func elapsed(m float64) string {
	if m < 0 {
		m = 0
	}
	n := int(m)
	if n < 60 {
		return fmt.Sprintf("%dm", n)
	}
	return fmt.Sprintf("%dh %02dm", n/60, n%60)
}

const timeCol = 11 // "09:00–10:30"

func todayCard(d Dash, w int, now time.Time) []string {
	inner := w - 4
	hm := now.Format("15:04")
	if d.Date != now.Format("2006-01-02") {
		hm = "" // the server's today is not this machine's: no line for "now"
	}
	timed, rest := []Item{}, []Item{}
	for _, it := range d.Today {
		if it.HasTime {
			timed = append(timed, it)
		} else {
			rest = append(rest, it)
		}
	}
	lines := []string{}
	at := len(timed) + 1
	if hm != "" {
		at = nowIndex(timed, hm)
	}
	for i, it := range timed {
		if i == at {
			lines = append(lines, nowLine(hm, inner))
		}
		lines = append(lines, timedRow(it, hm, inner))
	}
	if at == len(timed) && len(timed) > 0 {
		lines = append(lines, nowLine(hm, inner))
	}
	if len(timed) > 0 && len(rest) > 0 {
		lines = append(lines, "")
	}
	for _, it := range rest {
		lines = append(lines, untimedRow(it, inner))
	}
	if msg, ok := d.Errors["today"]; ok {
		lines = append(lines, dim("couldn't read the agenda: "+msg))
	} else if len(lines) == 0 {
		lines = append(lines, good("✓ ")+dim("Nothing planned today"))
	}
	note := ""
	// Tasks only: an appointment is not something to tick off.
	if s := d.Summary; s.Done > 0 {
		note = fmt.Sprintf("%d of %d done", s.Done, s.Done+s.ToDo)
	}
	return card("Today", note, lines, w)
}

func nowLine(hm string, w int) string {
	label := " now " + hm + " "
	left := 2
	return dim(strings.Repeat("┄", left)) + good(label) + dim(strings.Repeat("┄", max(1, w-left-vis(label))))
}

func timedRow(it Item, hm string, w int) string {
	span := it.Start
	if it.End != "" && it.End != it.Start {
		span += "–" + it.End
	}
	right := keyword(it)
	switch {
	case it.Done:
		return row(dim(padRight(span, timeCol))+"  "+good("✓ "), it.Title, dim, right, w)
	case underway(it, hm):
		return row(good(padRight(span, timeCol))+"  "+good("▸ "), it.Title, bold, right, w)
	case hm != "" && it.Start < hm:
		return row(dim(padRight(span, timeCol))+"  "+dim("· "), it.Title, dim, right, w)
	}
	return row(timeTone(padRight(span, timeCol))+"  "+"· ", it.Title, titleStyle(it), right, w)
}

func untimedRow(it Item, w int) string {
	var when string
	var mark string
	switch it.Kind {
	case common.AgendaDeadline:
		mark = warn("◆ ")
		when = "due today"
		if it.DueIn > 0 {
			when = fmt.Sprintf("due in %dd", it.DueIn)
			mark = paint(commands.AnsiGold, "◇ ")
		}
		when = warn(padLeft(when, timeCol))
	case common.AgendaScheduled:
		mark = "○ "
		when = dim(padLeft("today", timeCol))
	default:
		mark = soft("◦ ")
		when = dim(padLeft("all day", timeCol))
	}
	if it.Done {
		return row(dim(ansiRe.ReplaceAllString(when, ""))+"  "+good("✓ "), it.Title, dim, keyword(it), w)
	}
	return row(when+"  "+mark, it.Title, titleStyle(it), keyword(it), w)
}

func titleStyle(it Item) func(string) string {
	if it.Priority == "A" {
		return bold
	}
	return nil
}

// The keyword, dim on the right: NEXT and WAITING say what kind of thing it is
// without stealing the eye from the title.
func keyword(it Item) string {
	k := it.Status
	if it.Priority != "" {
		k = strings.TrimSpace(k + " #" + it.Priority)
	}
	return dim(k)
}

const overdueShown = 6

func overdueCard(d Dash, w int) []string {
	inner := w - 4
	lines := []string{}
	for i, it := range d.Overdue {
		if i == overdueShown {
			lines = append(lines, dim(fmt.Sprintf("     + %d more · orgs agenda", len(d.Overdue)-overdueShown)))
			break
		}
		mark, kind := warn("○ "), "scheduled"
		if it.Kind == common.AgendaDeadline {
			mark, kind = bad("◆ "), "deadline"
		}
		lines = append(lines, row(bad(padLeft(lateness(it.Late), 4))+"  "+mark, it.Title, titleStyle(it), dim(kind), inner))
	}
	if msg, ok := d.Errors["today"]; ok {
		lines = append(lines, dim("couldn't read the agenda: "+msg))
	} else if len(lines) == 0 {
		lines = append(lines, good("✓ ")+dim("Nothing overdue"))
	}
	note := ""
	if n := len(d.Overdue); n > 0 {
		note = fmt.Sprintf("%d", n)
	}
	return card("Overdue", note, lines, w)
}

// lateness says how far behind in the unit a person thinks in.
func lateness(days int) string {
	switch {
	case days >= 60:
		return fmt.Sprintf("%dmo", days/30)
	case days >= 14:
		return fmt.Sprintf("%dw", days/7)
	}
	return fmt.Sprintf("%dd", days)
}

func habitsCard(d Dash, w int) []string {
	inner := w - 4
	lines := []string{}
	for _, h := range d.Habits {
		mark := warn("○ ")
		style := func(s string) string { return s }
		switch {
		case h.Done:
			mark, style = good("✓ "), dim
		case h.Missed:
			mark = bad("○ ")
		}
		right := strip(h.Week) + "  " + padLeft(streak(h.Streak), 4)
		lines = append(lines, row(mark, h.Title, style, right, inner))
	}
	if msg, ok := d.Errors["habits"]; ok {
		lines = append(lines, dim("couldn't read habits: "+msg))
	} else if len(lines) == 0 {
		lines = append(lines, good("✓ ")+dim("No habits due today"))
	}
	note := ""
	if d.HabitsDue > 0 {
		note = fmt.Sprintf("%d of %d done", d.HabitsDone, d.HabitsDue)
	}
	return card("Habits", note, lines, w)
}

// strip is the last seven days as dots: filled for done, red ring for a miss,
// faint for a day the habit did not ask for anything.
func strip(week string) string {
	b := strings.Builder{}
	for _, c := range week {
		switch c {
		case 'd':
			b.WriteString(good("●"))
		case 'm':
			b.WriteString(bad("○"))
		default:
			b.WriteString(dim("·"))
		}
	}
	return b.String()
}

func streak(n int) string {
	if n <= 0 {
		return dim("–")
	}
	return warn(fmt.Sprintf("%d", n)) + dim("d")
}

func waitingCard(d Dash, w int) []string {
	inner := w - 4
	lines := []string{}
	if msg, ok := d.Errors["inbox"]; ok {
		lines = append(lines, dim("couldn't read the inbox: "+msg))
	} else if d.InboxCount == 0 {
		lines = append(lines, good("✓ ")+dim("Inbox empty"))
	} else {
		lines = append(lines, row(soft("▤ "), fmt.Sprintf("%d in the inbox", d.InboxCount), bold, "", inner))
		for _, t := range d.Inbox {
			lines = append(lines, row("  "+dim("· "), t, dim, "", inner))
		}
		if d.InboxCount > len(d.Inbox) {
			lines = append(lines, "  "+dim(fmt.Sprintf("  + %d more", d.InboxCount-len(d.Inbox))))
		}
	}
	if msg, ok := d.Errors["notices"]; ok {
		lines = append(lines, dim("couldn't read notifications: "+msg))
	} else if d.NoticeCount == 0 {
		lines = append(lines, good("✓ ")+dim("No new notices"))
	} else {
		lines = append(lines, row(soft("◉ "), plural(d.NoticeCount, "new notice", "new notices"), bold, "", inner))
		for i, n := range d.Notices {
			if i == 3 {
				break
			}
			what := ""
			if n.Gained > 0 {
				what = dim(fmt.Sprintf("+%d", n.Gained))
			}
			lines = append(lines, row("  "+dim("· "), n.Label, nil, what, inner))
		}
	}
	return card("Waiting for you", "", lines, w)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// footer offers the one command for each card that wants something.
func footer(b *strings.Builder, d Dash, w int) {
	type hint struct{ cmd, why string }
	hints := []hint{}
	if len(d.Overdue) > 0 {
		hints = append(hints, hint{"orgs agenda", "reschedule the late"})
	}
	if d.InboxCount > 0 {
		hints = append(hints, hint{"orgs triage", "empty the inbox"})
	}
	if d.HabitsDue > d.HabitsDone {
		hints = append(hints, hint{"orgs habits", "tick one off"})
	}
	if d.NoticeCount > 0 {
		hints = append(hints, hint{"orgs notifications", "read them"})
	}
	switch {
	case d.Focus != nil:
		hints = append(hints, hint{"orgs focus stop", "end the session"})
	case d.Clock == nil:
		hints = append(hints, hint{"orgs focus", "start a pomodoro"})
	}
	if len(hints) == 0 {
		return
	}
	pieces := []string{}
	for _, h := range hints {
		pieces = append(pieces, dim(h.why+" →")+" "+bold(h.cmd))
	}
	b.WriteString("\n")
	for _, l := range wrap(pieces, "   ", w) {
		b.WriteString(l + "\n")
	}
}
