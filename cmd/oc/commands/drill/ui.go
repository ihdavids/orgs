package drill

// The drill screen.
//
// org-drill's prompt line at the top - the clock, the status letter, and the
// done / failed / mature / new counts in their colours - then the card in a
// column down the middle, then what the keys do. In the question any key
// shows the answer; in the answer 0-5 rate it, each with the interval it would
// give written under it, as the worg tab and org-drill's help do.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/pres"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/mattn/go-runewidth"
)

type phase int

const (
	question phase = iota
	answer
	finished
	warning
)

var qualities = []struct {
	label, meaning string
}{
	{"Abject failure", "Completely forgot."},
	{"Failure", "Even after seeing the answer, it still took a bit to sink in."},
	{"Near miss", "After seeing the answer, you remembered it."},
	{"Hard", "It took you awhile, but you finally remembered."},
	{"Good", "After a little bit of thought you remembered."},
	{"Excellent", "You remembered the item really easily."},
}

// What a rating button says when org-drill's word for it does not fit.
var shortLabels = []string{"Forgot", "Failure", "Near miss", "Hard", "Good", "Excellent"}

type screen struct {
	core     *commands.Core
	scr      tcell.Screen
	s        *Session
	deckName string
	pal      *pres.Palette
	rng      Rng
	roots    []string
	refetch  func() common.DrillCardsReply

	phase  phase
	card   *common.DrillCard
	pres   Presentation
	typed  []rune
	note   string
	scroll int
	more   bool
	help   bool
	busy   bool
}

// run drills until the cards run out or q. It reports whether it was q.
func (u *screen) run() bool {
	scr, err := tcell.NewScreen()
	if err == nil {
		err = scr.Init()
	}
	if err != nil {
		commands.Fail("no terminal: %v", err)
		os.Exit(1)
	}
	u.scr = scr
	defer scr.Fini()
	scr.EnablePaste()

	if u.s.Current != "" && u.s.Cards[u.s.Current] != nil {
		u.show(u.s.Current)
	} else {
		u.advance()
	}

	events := make(chan tcell.Event, 8)
	stop := make(chan struct{})
	go scr.ChannelEvents(events, stop)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		u.draw()
		select {
		case <-tick.C:
		case ev := <-events:
			switch ev := ev.(type) {
			case *tcell.EventResize:
				scr.Sync()
			case *tcell.EventKey:
				switch u.key(ev) {
				case "quit":
					close(stop)
					return true
				case "done":
					close(stop)
					return false
				}
			}
		}
	}
}

func (u *screen) show(h string) {
	u.card = u.s.Cards[h]
	u.s.Current = h
	u.pres = Present(u.card, u.rng)
	u.phase, u.typed, u.scroll = question, nil, 0
}

func (u *screen) advance() {
	h := u.s.Next(time.Now(), u.rng)
	if h == "" {
		u.phase, u.card = finished, nil
		return
	}
	u.show(h)
}

// key handles one key: "quit", "done", or "" to keep going.
func (u *screen) key(ev *tcell.EventKey) string {
	k, r := ev.Key(), ev.Rune()
	if k == tcell.KeyCtrlC {
		return "quit"
	}
	if u.help {
		u.help = false
		return ""
	}
	switch u.phase {
	case finished:
		switch {
		case r == 'k' && u.s.Pending() > 0:
			u.s.KeepGoing(time.Now())
			u.advance()
		case k == tcell.KeyEnter || r == ' ' || r == 'q' || k == tcell.KeyEscape:
			if u.s.MakeReport(time.Now()).Warn {
				u.phase = warning
				return ""
			}
			return "done"
		}
		return ""
	case warning:
		return "done"
	}
	// Scrolling a card taller than the screen, in either phase.
	switch k {
	case tcell.KeyDown, tcell.KeyPgDn:
		if u.more {
			u.scroll += map[bool]int{true: 10, false: 1}[k == tcell.KeyPgDn]
		}
		return ""
	case tcell.KeyUp, tcell.KeyPgUp:
		u.scroll = max(0, u.scroll-map[bool]int{true: 10, false: 1}[k == tcell.KeyPgUp])
		return ""
	}
	typing := u.phase == question && u.s.Deck.TypedAnswers
	if typing {
		switch k {
		case tcell.KeyEnter:
			u.phase = answer
		case tcell.KeyEscape:
			return "quit"
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if len(u.typed) > 0 {
				u.typed = u.typed[:len(u.typed)-1]
			}
		case tcell.KeyCtrlU:
			u.typed = nil
		case tcell.KeyRune:
			u.typed = append(u.typed, r)
		}
		return ""
	}
	if u.phase == question {
		switch {
		case r == 'q' || k == tcell.KeyEscape:
			return "quit"
		case r == 'e':
			u.edit()
		case r == 's':
			u.s.Skip()
			u.note = "skipped"
			u.advance()
		default:
			u.phase = answer
		}
		return ""
	}
	// The answer.
	switch {
	case r >= '0' && r <= '5':
		u.rate(int(r - '0'))
	case r == '?':
		u.help = true
	case r == 'e':
		u.edit()
	case r == 'q' || k == tcell.KeyEscape:
		return "quit"
	}
	return ""
}

func (u *screen) rate(q int) {
	if u.busy || u.card == nil {
		return
	}
	u.busy = true
	defer func() { u.busy = false }()
	if !u.s.Cram {
		req := common.DrillReview{Hash: u.card.Hash, Quality: q, Deck: u.s.Deck.Name, Algorithm: u.s.Deck.Algorithm}
		res, err := commands.SendReceivePostErr[common.DrillReview, common.DrillReviewReply](u.core, "drill/review", &req)
		switch {
		case err == commands.ErrDryRun:
			u.note = "dry run: nothing written"
		case err != nil:
			u.note = "not saved: " + err.Error()
			return
		case !res.Ok:
			u.note = "not saved: " + res.Msg
			return
		case res.Leech:
			u.note = res.Msg + ". This card is now a leech."
		default:
			u.note = res.Msg
		}
	} else {
		u.note = ""
	}
	u.s.Rate(u.card.Hash, q, u.rng)
	u.advance()
}

// edit opens the card where it is written, then reads the deck again so the
// card asked is the card as it now reads. The session's place is kept.
func (u *screen) edit() {
	if u.card == nil {
		return
	}
	u.scr.Suspend()
	if len(u.core.EditorTemplate) > 0 {
		u.core.LaunchEditor(u.card.File, u.card.Line)
	} else {
		ed := os.Getenv("VISUAL")
		if ed == "" {
			ed = os.Getenv("EDITOR")
		}
		if ed == "" {
			ed = "vi"
		}
		args := append(strings.Fields(ed), fmt.Sprintf("+%d", u.card.Line), u.card.File)
		c := exec.Command(args[0], args[1:]...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		_ = c.Run()
	}
	u.scr.Resume()
	r := u.refetch()
	cur := u.card.Hash
	for i := range r.Cards {
		if old := u.s.Cards[r.Cards[i].Hash]; old != nil {
			*old = r.Cards[i]
		}
	}
	if c := u.s.Cards[cur]; c != nil {
		u.card = c
		u.pres = Present(c, u.rng)
		u.note = "card read again"
	} else {
		u.note = "that card is no longer in the deck"
		u.s.Skip()
		u.advance()
	}
}

// ── Drawing ─────────────────────────────────────────────────────────────────

func (u *screen) put(x, y int, s string, st tcell.Style) int {
	w, _ := u.scr.Size()
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if x+rw > w {
			break
		}
		u.scr.SetContent(x, y, r, nil, st)
		x += rw
	}
	return x
}

func (u *screen) fill(y int, st tcell.Style) {
	w, _ := u.scr.Size()
	for x := 0; x < w; x++ {
		u.scr.SetContent(x, y, ' ', nil, st)
	}
}

func (u *screen) draw() {
	p := u.pal
	base := p.Base()
	u.scr.SetStyle(base)
	u.scr.Clear()
	u.scr.HideCursor()
	w, h := u.scr.Size()
	margin := max(2, w/12)
	colW := min(w-2*margin, 88)
	if colW < 20 {
		colW = max(10, w-2)
	}
	left := (w - colW) / 2

	switch u.phase {
	case finished, warning:
		u.drawReport(left, colW)
		u.scr.Show()
		return
	}

	// The prompt line.
	soft := base.Foreground(p.Soft())
	y := 1
	x := u.put(left, y, Clock(time.Since(u.s.Start))+"  ", soft)
	letter, tone := u.s.Letter(u.card)
	toneColour := map[string]tcell.Color{"new": tcell.ColorRoyalBlue, "mature": tcell.ColorGreen, "failed": tcell.ColorRed, "neutral": p.Soft()}[tone]
	x = u.put(x, y, " "+letter+" ", base.Background(toneColour).Foreground(tcell.ColorWhite).Bold(true))
	done, failed, mature, fresh := u.s.Counters()
	for _, c := range []struct {
		n   int
		col tcell.Color
		tip string
	}{{done, tcell.ColorSienna, "done"}, {failed, tcell.ColorRed, "failed"}, {mature, tcell.ColorGreen, "mature"}, {fresh, tcell.ColorRoyalBlue, "new"}} {
		x = u.put(x+2, y, fmt.Sprint(c.n), base.Foreground(c.col).Bold(true))
		x = u.put(x+1, y, c.tip, soft)
	}
	label := u.deckName
	if u.s.Cram {
		label += " · cram"
	}
	u.put(left+colW-runewidth.StringWidth(label), y, label, soft)
	// The session's progress, against whichever limit is nearer.
	y++
	frac := 0.0
	if !u.s.Cram {
		if u.s.Deck.MaxItems > 0 {
			frac = float64(len(u.s.Done)) / float64(u.s.Deck.MaxItems)
		}
		if u.s.Deck.MaxMinutes > 0 {
			frac = max(frac, time.Since(u.s.Start).Minutes()/float64(u.s.Deck.MaxMinutes))
		}
	}
	filled := int(min(1, frac) * float64(colW))
	u.put(left, y, strings.Repeat("━", filled), base.Foreground(p.Accent()))
	u.put(left+filled, y, strings.Repeat("─", colW-filled), base.Foreground(p.Rule()))

	// The card.
	lines := u.cardLines(colW)
	top := y + 2
	// Below the card: three rows of rating buttons, a line of keys, a gap,
	// and the note about the last rating.
	bottom := h - 7
	if u.card.Leech && u.s.Deck.LeechMethod == "warn" {
		warn := base.Foreground(tcell.ColorRed).Bold(true)
		u.put(left, top, "!!! LEECH ITEM !!!", warn)
		u.put(left, top+1, clip("You seem to be having a lot of trouble memorising this item. Consider reformulating it.", colW), base.Foreground(tcell.ColorRed))
		top += 3
	}
	avail := max(1, bottom-top)
	u.scroll = min(u.scroll, max(0, len(lines)-avail))
	u.more = len(lines)-u.scroll > avail
	for i := u.scroll; i < len(lines) && top+i-u.scroll < bottom; i++ {
		pres.DrawLine(u.scr, left, top+i-u.scroll, colW, lines[i])
	}
	if u.more {
		u.put(left+colW-1, bottom-1, "▾", soft)
	}
	if u.scroll > 0 {
		u.put(left+colW-1, top, "▴", soft)
	}

	// The keys.
	if u.phase == question {
		if u.s.Deck.TypedAnswers {
			u.put(left, h-5, "Your answer: ", soft)
			cx := u.put(left+13, h-5, string(u.typed), base.Underline(true))
			u.scr.ShowCursor(cx, h-5)
			u.put(left, h-3, "enter shows the answer · esc quits", soft)
		} else {
			x := u.put(left, h-5, " space ", base.Background(p.Accent()).Foreground(p.CodeBg()).Bold(true))
			u.put(x+1, h-5, "show the answer", base)
			hint := "e edit · s skip · q quit"
			u.put(left+colW-runewidth.StringWidth(hint), h-5, hint, soft)
		}
	} else {
		u.drawRatings(left, h-6, colW)
	}
	if u.note != "" {
		u.put(left, h-1, clip(u.note, colW), soft.Italic(true))
	}
	if u.help {
		u.drawHelp(left, colW)
	}
	u.scr.Show()
}

// cardLines is the card as it reads in this phase.
func (u *screen) cardLines(colW int) []pres.Line {
	p := u.pal
	base := p.Base()
	c := u.card
	ans := u.phase == answer
	dots := u.s.Deck.ClozeLengthDot
	_, h := u.scr.Size()
	maxImg := max(4, h/3)
	out := []pres.Line{}
	head := base.Foreground(p.Head()).Bold(true)
	switch {
	case ans && u.pres.AnswerHeading != "":
		out = append(out, pres.RenderInline(u.pres.AnswerHeading, head, colW, p)...)
	case !(u.s.Deck.HideTitles && !ans):
		out = append(out, pres.RenderInline(ClozeOrg(c.Title, 0, u.pres.Hide, ans, dots), head, colW, p)...)
	}
	out = append(out, pres.Line{Spans: []pres.Span{{Text: "━━━━", Style: base.Foreground(p.Accent())}}}, pres.Line{})
	if u.pres.Prompt != "" && !ans {
		out = append(out, pres.RenderOrg(u.pres.Prompt, c.File, colW, maxImg, p, u.roots)...)
	} else {
		out = append(out, pres.RenderOrg(ClozeOrg(c.Body, 1, u.pres.Hide, ans, dots), c.File, colW, maxImg, p, u.roots)...)
	}
	for i, side := range c.Sides {
		open := i == u.pres.Side
		if ans {
			open = !side.IsCard
		}
		mark := "▸ "
		if open {
			mark = "▾ "
		}
		out = append(out, pres.Line{})
		t := pres.RenderInline(ClozeOrg(side.Title, 0, u.pres.Hide, ans, dots), base.Foreground(p.Accent()).Bold(true), colW-2, p)
		for j := range t {
			if j == 0 {
				t[j].Spans = append([]pres.Span{{Text: mark, Style: base.Foreground(p.Soft())}}, t[j].Spans...)
			} else {
				t[j].Indent += 2
			}
		}
		out = append(out, t...)
		if open {
			for _, l := range pres.RenderOrg(ClozeOrg(side.Body, 0, u.pres.Hide, ans, dots), c.File, colW-2, maxImg, p, u.roots) {
				l.Indent += 2
				out = append(out, l)
			}
		}
	}
	if ans && len(c.Explain) > 0 {
		out = append(out, pres.Line{}, pres.Line{Spans: []pres.Span{{Text: "Explanation", Style: base.Foreground(p.Accent()).Bold(true)}}})
		for _, e := range c.Explain {
			out = append(out, pres.RenderOrg(e, c.File, colW, maxImg, p, u.roots)...)
		}
	}
	if ans && len(u.typed) > 0 {
		out = append(out, pres.Line{}, pres.Line{Spans: []pres.Span{{Text: "Your answer: ", Style: base.Foreground(p.Soft())}, {Text: string(u.typed), Style: base.Italic(true)}}})
	}
	return out
}

func (u *screen) drawRatings(left, y, colW int) {
	p := u.pal
	base := p.Base()
	cell := max(8, colW/6)
	_, ground, _ := base.Decompose()
	for q := 0; q <= 5; q++ {
		fail := q <= u.s.Deck.FailureQuality
		bg := pres.Mix(ground, tcell.ColorGreen, 0.45)
		if fail {
			bg = pres.Mix(ground, tcell.ColorRed, 0.35)
		} else if q == 3 {
			bg = pres.Mix(ground, tcell.ColorOrange, 0.4)
		}
		st := base.Background(bg).Foreground(tcell.ColorWhite)
		x := left + q*cell
		for row := 0; row < 3; row++ {
			for i := 0; i < cell-1; i++ {
				u.scr.SetContent(x+i, y+row, ' ', nil, st)
			}
		}
		center := func(row int, s string, s2 tcell.Style) {
			s = clip(s, cell-1)
			u.put(x+max(0, (cell-1-runewidth.StringWidth(s))/2), y+row, s, s2)
		}
		center(0, fmt.Sprint(q), st.Bold(true))
		label := qualities[q].label
		if runewidth.StringWidth(label) > cell-1 {
			label = shortLabels[q]
		}
		center(1, label, st)
		when := "again"
		if !fail {
			when = Days(u.card.Next[q])
		}
		if u.s.Cram {
			when = ""
		}
		center(2, when, st)
	}
	u.put(left, y+3, "How well did you do? 0-5 · ? help · e edit · q quit", base.Foreground(p.Soft()))
}

func (u *screen) drawHelp(left, colW int) {
	p := u.pal
	st := p.Base().Background(p.CodeBg())
	_, h := u.scr.Size()
	lines := []string{
		"How well did you do?", "",
		fmt.Sprintf("0-%d means you have forgotten the item.", u.s.Deck.FailureQuality),
		fmt.Sprintf("%d-5 means you have remembered it.", u.s.Deck.FailureQuality+1), "",
	}
	for q, x := range qualities {
		l := fmt.Sprintf("%d - %s", q, x.meaning)
		if q > u.s.Deck.FailureQuality && !u.s.Cram {
			l += " (" + Days(u.card.Next[q]) + ")"
		}
		lines = append(lines, l)
	}
	lines = append(lines, "", "any key closes this")
	y0 := max(1, (h-len(lines)-2)/2)
	for i := -1; i <= len(lines); i++ {
		u.fillRange(left-2, y0+i, colW+4, st)
	}
	for i, l := range lines {
		ls := st
		if i == 0 {
			ls = st.Foreground(p.Accent()).Bold(true)
		}
		u.put(left, y0+i, clip(l, colW), ls)
	}
}

func (u *screen) fillRange(x, y, n int, st tcell.Style) {
	for i := 0; i < n; i++ {
		u.scr.SetContent(x+i, y, ' ', nil, st)
	}
}

func (u *screen) drawReport(left, colW int) {
	p := u.pal
	base := p.Base()
	soft := base.Foreground(p.Soft())
	r := u.s.MakeReport(time.Now())
	y := 2
	line := func(s string, st tcell.Style) {
		for _, part := range wrapPlain(s, colW) {
			u.put(left, y, part, st)
			y++
		}
	}
	if u.phase == warning {
		line("WARNING!", base.Foreground(tcell.ColorRed).Bold(true))
		y++
		line(fmt.Sprintf("You failed %d%% of the items you reviewed during this session.", 100-r.PassPercent), base)
		line(fmt.Sprintf("%d (%d%%) of all items scanned were overdue.", r.OverdueAtStart, r.OverduePercent), base)
		y++
		line("Are you keeping up with your items, and reviewing them when they are scheduled? If so, you may want to consider lowering the deck's learn fraction slightly in order to make items appear more frequently over time.", base)
		y++
		line("any key to finish", soft)
		return
	}
	line("Session finished", base.Foreground(p.Head()).Bold(true))
	u.put(left, y, "━━━━", base.Foreground(p.Accent()))
	y += 2
	line(fmt.Sprintf("%d items reviewed. Session duration %d:%02d:%02d.", r.Reviewed,
		int(r.Duration.Hours()), int(r.Duration.Minutes())%60, int(r.Duration.Seconds())%60), base)
	if len(u.s.Qualities) > 0 {
		y++
		line("Recall of reviewed items:", base.Bold(true))
		// Drawn as they are, not wrapped: the spaces are the columns.
		row := func(a string, qa int, b string, qb int) {
			u.put(left, y, clip(fmt.Sprintf(" %-18s %3d%%   |   %-20s %3d%%", a, r.Percent[qa], b, r.Percent[qb]), colW), base)
			y++
		}
		row("Excellent (5):", 5, "Near miss (2):", 2)
		row("Good (4):", 4, "Failure (1):", 1)
		row("Hard (3):", 3, "Abject failure (0):", 0)
		y++
		line(fmt.Sprintf("You successfully recalled %d%% of reviewed items (quality > %d)", r.PassPercent, r.FailureQuality), base)
	}
	line(fmt.Sprintf("%d/%d items still await review (%d failed, %d overdue, %d new, %d young, %d old).",
		r.Pending, r.PendingAll, r.Failed, r.Overdue, r.Fresh, r.Young, r.Old), base)
	line(fmt.Sprintf("Tomorrow, %d more items will become due for review.", r.DueTomorrow), base)
	y++
	hint := "enter to finish"
	if r.Pending > 0 {
		hint += fmt.Sprintf(" · k keep going (%d left)", r.Pending)
	}
	line(hint, soft)
}

func clip(s string, w int) string {
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

func wrapPlain(s string, w int) []string {
	out := []string{}
	cur := ""
	for _, word := range strings.Fields(s) {
		if cur != "" && runewidth.StringWidth(cur)+1+runewidth.StringWidth(word) > w {
			out = append(out, cur)
			cur = ""
		}
		if cur != "" {
			cur += " "
		}
		cur += word
	}
	if cur != "" || len(out) == 0 {
		out = append(out, cur)
	}
	return out
}
