package pres

// The running slideshow: keys, transitions, overlays, the presenter's view, and
// following the file as it is edited.

import (
	"crypto/sha1"
	hexenc "encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/go-org/org"
	"github.com/mattn/go-runewidth"
)

var hexInGradient = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)

type show struct {
	scr   tcell.Screen
	deck  *deck
	path  string
	load  func() (*deck, error)
	// edit opens the file at a line. A terminal editor takes the screen
	// until it exits; a gui editor returns at once and the save is noticed
	// by the reload.
	edit     func(file string, line int)
	termEdit bool
	files *finder

	pal       *palette
	themes    []string
	themeAt   int
	maxWidth  int
	trans     string
	presenter bool

	idx, step, steps int
	scroll, overflow int

	footer, notes, help, black, timerOn bool
	overview                            bool
	ovSel                               int
	jump                                string

	start   time.Time
	msg     string
	msgTill time.Time
	modTime time.Time

	// The page last put on the screen, which a transition starts from.
	shown *canvas
	anim  *anim

	sync     string
	syncMod  time.Time
	lastTick time.Time
}

type anim struct {
	from, to *canvas
	kind     string
	dir      int
	start    time.Time
	dur      time.Duration
}

func (s *show) view(w int) view {
	v := view{pal: s.pal, files: s.files, footer: s.footer, scroll: s.scroll, maxWidth: s.maxWidth, idx: s.idx}
	if s.timerOn {
		v.timer = elapsed(time.Since(s.start))
	}
	if time.Now().Before(s.msgTill) {
		v.msg = s.msg
	}
	return v
}

func elapsed(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, sec := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%02d:%02d", m, sec)
}

func (s *show) say(msg string) {
	s.msg = msg
	s.msgTill = time.Now().Add(2 * time.Second)
}

// frame draws the current state into a canvas: the page, then whatever is
// open over it.
func (s *show) frame() *canvas {
	w, h := s.scr.Size()
	if s.presenter {
		return s.presenterView(w, h)
	}
	if s.black {
		return newCanvas(w, h, tcell.StyleDefault.Background(tcell.ColorBlack))
	}
	c := s.deck.compose(s.idx, s.step, w, h, s.view(w))
	s.steps, s.overflow = c.steps, c.overflow
	cv := c.c
	if s.notes {
		s.notesPanel(cv)
	}
	if s.overview {
		s.overviewPanel(cv)
	}
	if s.help {
		s.helpPanel(cv)
	}
	return cv
}

func (s *show) draw() {
	cv := s.frame()
	s.blit(cv)
	s.shown = cv
}

func (s *show) blit(cv *canvas) {
	for y := 0; y < cv.h; y++ {
		for x := 0; x < cv.w; x++ {
			c := cv.cells[y*cv.w+x]
			if c.cont {
				continue
			}
			s.scr.SetContent(x, y, c.r, c.comb, c.st)
		}
	}
	s.scr.Show()
}

// ── Moving ──────────────────────────────────────────────────────────────────

func (s *show) goTo(idx, step int, animate bool) {
	idx = max(0, min(idx, len(s.deck.pages)-1))
	if idx == s.idx && step == s.step {
		return
	}
	from := s.shown
	dir := 1
	if idx < s.idx || (idx == s.idx && step < s.step) {
		dir = -1
	}
	sameSlide := idx == s.idx
	s.idx, s.step, s.scroll = idx, step, 0
	if step < 0 {
		// Coming back to a slide shows it as it was left: every step taken.
		s.step = 0
		w, h := s.scr.Size()
		s.step = s.deck.compose(idx, 0, w, h, s.view(w)).steps
	}
	s.writeSync()
	to := s.frame()
	kind := s.trans
	if sameSlide {
		kind = "fade"
	} else if pg := s.deck.pages[idx]; pg.s != nil {
		kind = strings.ToLower(s.deck.conf.Str(pg.s.Props(), "TRANSITION", kind))
	}
	if !animate || from == nil || kind == "none" || s.trans == "none" || from.w != to.w || from.h != to.h {
		s.blit(to)
		s.shown = to
		return
	}
	dur := 260 * time.Millisecond
	switch kind {
	case "fade":
		dur = 200 * time.Millisecond
		if sameSlide {
			dur = 160 * time.Millisecond
		}
	case "slide", "convex", "concave", "zoom", "cube", "page":
		kind = "slide"
	default:
		kind = "slide"
	}
	s.anim = &anim{from: from, to: to, kind: kind, dir: dir, start: time.Now(), dur: dur}
	s.tickAnim()
}

func (s *show) next(skipSteps bool) {
	if !skipSteps && s.step < s.steps {
		s.goTo(s.idx, s.step+1, true)
		return
	}
	if s.idx < len(s.deck.pages)-1 {
		s.goTo(s.idx+1, 0, true)
	}
}

func (s *show) prev(skipSteps bool) {
	if !skipSteps && s.step > 0 {
		s.goTo(s.idx, s.step-1, true)
		return
	}
	if s.idx > 0 {
		if skipSteps {
			s.goTo(s.idx-1, 0, true)
		} else {
			s.goTo(s.idx-1, -1, true)
		}
	}
}

// tickAnim draws the next frame of a transition, and reports whether one is
// still running.
func (s *show) tickAnim() bool {
	a := s.anim
	if a == nil {
		return false
	}
	t := float64(time.Since(a.start)) / float64(a.dur)
	if t >= 1 {
		s.anim = nil
		s.blit(a.to)
		s.shown = a.to
		return false
	}
	// Ease out: quick to leave, gentle to arrive.
	e := 1 - (1-t)*(1-t)*(1-t)
	var cv *canvas
	switch a.kind {
	case "fade":
		cv = crossfade(a.from, a.to, e)
	default:
		cv = slideOver(a.from, a.to, e, a.dir)
	}
	s.blit(cv)
	return true
}

// slideOver is two pages side by side, moved along by t of the screen width.
func slideOver(from, to *canvas, t float64, dir int) *canvas {
	w, h := to.w, to.h
	cv := &canvas{w: w, h: h, cells: make([]cell, w*h)}
	off := int(t * float64(w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var src *canvas
			sx := 0
			if dir > 0 {
				sx = x + off
				src = from
				if sx >= w {
					sx -= w
					src = to
				}
			} else {
				sx = x - off
				src = from
				if sx < 0 {
					sx += w
					src = to
				}
			}
			c := src.cells[y*w+sx]
			// Half a wide character at the seam is drawn as a space.
			if c.cont || (runewidth.RuneWidth(c.r) == 2 && x == w-1) {
				c = cell{r: ' ', st: c.st}
			}
			cv.cells[y*w+x] = c
		}
	}
	return cv
}

// crossfade dissolves one page into the other: the old ink sinks into its
// ground, then the new ink rises out of its. Cells the same on both are left
// alone, which is what makes a revealed bullet fade in without the rest of
// the slide flickering.
func crossfade(from, to *canvas, t float64) *canvas {
	cv := &canvas{w: to.w, h: to.h, cells: make([]cell, len(to.cells))}
	for i := range to.cells {
		a, b := from.cells[i], to.cells[i]
		if a.r == b.r && a.st == b.st && len(a.comb) == len(b.comb) {
			cv.cells[i] = b
			continue
		}
		af, ab, aa := a.st.Decompose()
		bf, bb, _ := b.st.Decompose()
		bg := lerp(ab, bb, t)
		if t < 0.5 {
			c := a
			c.st = tcell.StyleDefault.Attributes(aa).Background(bg).Foreground(lerp(af, ab, t*2))
			cv.cells[i] = c
		} else {
			c := b
			c.st = b.st.Background(bg).Foreground(lerp(bb, bf, (t-0.5)*2))
			cv.cells[i] = c
		}
	}
	return cv
}

// ── Following the file, and the other window ────────────────────────────────

// reload rebuilds the deck when the file has been saved, keeping the place:
// writing a talk is mostly saving and looking, so the slide on screen should
// be the one just edited, not the first.
func (s *show) reload(force bool) {
	st, err := os.Stat(s.path)
	if err != nil {
		return
	}
	if !force && !st.ModTime().After(s.modTime) {
		return
	}
	s.modTime = st.ModTime()
	d, err := s.load()
	if err != nil {
		s.say("✗ " + err.Error())
		return
	}
	s.deck = d
	s.idx = min(s.idx, len(d.pages)-1)
	s.say("reloaded")
	s.draw()
}

// The sync file lets a second window follow the first: `orgs pres -presenter
// talk.org` in another terminal shows the notes and the next slide for
// wherever the audience's window is, and either window can drive.
func syncPath(file string) string {
	abs, _ := filepath.Abs(file)
	sum := sha1.Sum([]byte(abs))
	return filepath.Join(os.TempDir(), "orgs-pres-"+hexenc.EncodeToString(sum[:])[:12]+".pos")
}

func (s *show) writeSync() {
	_ = os.WriteFile(s.sync, []byte(fmt.Sprintf("%d %d\n", s.idx, s.step)), 0o600)
	if st, err := os.Stat(s.sync); err == nil {
		s.syncMod = st.ModTime()
	}
}

func (s *show) readSync() {
	st, err := os.Stat(s.sync)
	if err != nil || !st.ModTime().After(s.syncMod) {
		return
	}
	s.syncMod = st.ModTime()
	b, err := os.ReadFile(s.sync)
	if err != nil {
		return
	}
	f := strings.Fields(string(b))
	if len(f) != 2 {
		return
	}
	idx, e1 := strconv.Atoi(f[0])
	step, e2 := strconv.Atoi(f[1])
	if e1 != nil || e2 != nil || (idx == s.idx && step == s.step) {
		return
	}
	idx = max(0, min(idx, len(s.deck.pages)-1))
	s.idx, s.step, s.scroll = idx, step, 0
	s.draw()
}

// ── Overlays ────────────────────────────────────────────────────────────────

// box draws a panel with a rounded edge and returns its inner rectangle.
func (s *show) box(cv *canvas, x, y, w, h int, title string) (int, int, int, int) {
	p := s.pal
	bg := p.base().Background(p.codeBg)
	frame := bg.Foreground(lerp(p.rule, p.soft, 0.5))
	for j := 0; j < h; j++ {
		cv.fill(x, y+j, w, bg)
	}
	cv.put(x, y, "╭"+strings.Repeat("─", w-2)+"╮", frame)
	for j := 1; j < h-1; j++ {
		cv.put(x, y+j, "│", frame)
		cv.put(x+w-1, y+j, "│", frame)
	}
	cv.put(x, y+h-1, "╰"+strings.Repeat("─", w-2)+"╯", frame)
	if title != "" {
		cv.put(x+3, y, " "+title+" ", bg.Foreground(p.accent).Bold(true))
	}
	return x + 3, y + 2, w - 6, h - 4
}

// renderNotes is the slide's speaker notes as lines for a column.
func (s *show) renderNotes(i, width int, p *palette) []line {
	pg := s.deck.pages[i]
	if pg.s == nil || len(pg.s.Notes) == 0 {
		return nil
	}
	r := newRenderer(p, s.deck.conf, s.files, 8)
	r.blocks(pg.s.Notes, 0, width, false)
	return r.out
}

func (s *show) notesPanel(cv *canvas) {
	h := max(6, cv.h/3)
	np := *s.pal
	np.bg = s.pal.codeBg
	x, y, w, ih := s.box(cv, 2, cv.h-h-3, cv.w-4, h, "notes")
	lines := s.renderNotes(s.idx, w, &np)
	if lines == nil {
		cv.put(x, y, clipStr("no notes on this slide", w), np.base().Foreground(np.soft).Italic(true))
		return
	}
	for k, l := range lines {
		if k >= ih {
			break
		}
		xx := x + l.indent
		for _, sg := range l.segs {
			xx = cv.put(xx, y+k, sg.s, sg.st)
		}
	}
}

func (s *show) overviewPanel(cv *canvas) {
	p := s.pal
	w := min(cv.w-4, 72)
	h := cv.h - 4
	x, y, iw, ih := s.box(cv, (cv.w-w)/2, 2, w, h, "slides")
	n := len(s.deck.pages)
	first := max(0, min(s.ovSel-ih/2, n-ih))
	bg := p.base().Background(p.codeBg)
	for k := 0; k < ih && first+k < n; k++ {
		i := first + k
		lvl := 0
		if pg := s.deck.pages[i]; pg.s != nil {
			lvl = max(0, pg.s.Level-1)
		}
		num := fmt.Sprintf("%3d  ", i+1)
		st := bg.Foreground(p.ink)
		if pg := s.deck.pages[i]; pg.title || (pg.s != nil && len(pg.s.Body) == 0) {
			st = st.Bold(true).Foreground(p.head)
		}
		if i == s.ovSel {
			cv.fill(x-1, y+k, iw+2, bg.Background(p.accent))
			st = st.Background(p.accent).Foreground(p.codeBg)
		}
		mark := "  "
		if i == s.idx {
			mark = "▸ "
		}
		numSt := bg.Foreground(p.soft)
		if i == s.ovSel {
			numSt = st
		}
		xx := cv.put(x, y+k, mark+num, numSt)
		cv.put(xx+lvl*2, y+k, clipStr(s.deck.titleOf(i), iw-(xx-x)-lvl*2), st)
	}
}

var helpKeys = [][2]string{
	{"→  space  l  PgDn", "next step or slide"},
	{"←  h  PgUp  ⌫", "back"},
	{"N  P", "next / previous slide, skipping steps"},
	{"↓ ↑  j k", "scroll a slide too tall for the screen"},
	{"g  G", "first / last slide"},
	{"12 ⏎", "go to slide 12"},
	{"o  tab", "all slides"},
	{"n", "speaker notes"},
	{"t  r", "timer / restart it"},
	{"f", "footer and progress"},
	{"T", "next theme"},
	{"b  .", "black screen"},
	{"e", "edit this slide"},
	{"R", "reload the file"},
	{"q  esc", "quit"},
}

func (s *show) helpPanel(cv *canvas) {
	p := s.pal
	w := min(cv.w-4, 76)
	h := min(cv.h-2, len(helpKeys)+5)
	x, y, iw, _ := s.box(cv, (cv.w-w)/2, (cv.h-h)/2, w, h, "keys")
	bg := p.base().Background(p.codeBg)
	for k, kv := range helpKeys {
		if y+k >= (cv.h+h)/2-2 {
			break
		}
		cv.put(x, y+k, kv[0], bg.Foreground(p.accent))
		cv.put(x+22, y+k, clipStr(kv[1], iw-22), bg.Foreground(p.ink))
	}
	foot := "theme: " + p.name
	cv.put(x+iw-runewidth.StringWidth(foot), y+h-4, foot, bg.Foreground(p.soft))
}

// presenterView is the second window: what the speaker needs and the room
// does not - the notes for this slide, the next slide's name, and the time.
func (s *show) presenterView(w, h int) *canvas {
	p := s.pal
	cv := newCanvas(w, h, p.base())
	margin := max(2, w/20)
	colW := w - 2*margin
	soft := p.base().Foreground(p.soft)

	steps := s.deck.compose(s.idx, s.step, max(w, 40), max(h, 20), s.view(w)).steps
	s.steps = steps
	head := fmt.Sprintf("%d / %d", s.idx+1, len(s.deck.pages))
	if steps > 0 {
		head += fmt.Sprintf("   step %d of %d", s.step, steps)
	}
	cv.put(margin, 1, head, soft)
	clock := time.Now().Format("15:04")
	if s.timerOn {
		clock = elapsed(time.Since(s.start)) + "   " + clock
	}
	cv.put(w-margin-runewidth.StringWidth(clock), 1, clock, p.base().Foreground(p.accent))

	y := 3
	for _, ws := range wrap([]seg{{s.deck.titleOf(s.idx), p.base().Foreground(p.head).Bold(true)}}, colW) {
		x := margin
		for _, sg := range ws {
			x = cv.put(x, y, sg.s, sg.st)
		}
		y++
	}
	cv.put(margin, y, "━━━━", p.base().Foreground(p.accent))
	y += 2

	bottom := h - 4
	lines := s.renderNotes(s.idx, colW, p)
	if lines == nil {
		cv.put(margin, y, "no notes on this slide", soft.Italic(true))
	}
	for _, l := range lines {
		if y >= bottom {
			cv.put(w-margin-1, bottom-1, "▾", soft)
			break
		}
		x := margin + l.indent
		if l.bandW > 0 {
			cv.fill(x, y, l.bandW, l.band)
		}
		for _, sg := range l.segs {
			x = cv.put(x, y, sg.s, sg.st)
		}
		y++
	}

	cv.put(0, h-3, strings.Repeat("─", w), p.base().Foreground(p.rule))
	next := "end of the talk"
	if s.step < steps {
		next = "next: step " + itoa(s.step+1) + " of this slide"
	} else if s.idx+1 < len(s.deck.pages) {
		next = "next ▸ " + s.deck.titleOf(s.idx+1)
	}
	cv.put(margin, h-2, clipStr(next, colW), p.base().Foreground(p.ink))
	return cv
}

// ── Keys ────────────────────────────────────────────────────────────────────

// key handles one key press, and reports false when it is time to stop.
func (s *show) key(ev *tcell.EventKey) bool {
	if s.anim != nil {
		// A key during a transition finishes it first, so fast clicking
		// through a deck is never behind.
		s.anim.start = time.Time{}
		s.tickAnim()
	}
	r := ev.Rune()
	k := ev.Key()
	if k == tcell.KeyCtrlC {
		return false
	}
	if s.help {
		s.help = false
		s.draw()
		return true
	}
	if s.overview {
		switch {
		case k == tcell.KeyEscape || r == 'o' || r == 'q' || k == tcell.KeyTab:
			s.overview = false
		case k == tcell.KeyDown || r == 'j':
			s.ovSel = min(s.ovSel+1, len(s.deck.pages)-1)
		case k == tcell.KeyUp || r == 'k':
			s.ovSel = max(s.ovSel-1, 0)
		case k == tcell.KeyPgDn:
			s.ovSel = min(s.ovSel+10, len(s.deck.pages)-1)
		case k == tcell.KeyPgUp:
			s.ovSel = max(s.ovSel-10, 0)
		case r == 'g' || k == tcell.KeyHome:
			s.ovSel = 0
		case r == 'G' || k == tcell.KeyEnd:
			s.ovSel = len(s.deck.pages) - 1
		case k == tcell.KeyEnter || r == ' ':
			s.overview = false
			s.draw()
			s.goTo(s.ovSel, 0, true)
			return true
		}
		s.draw()
		return true
	}
	if r >= '0' && r <= '9' {
		s.jump += string(r)
		s.say("go to " + s.jump)
		s.draw()
		return true
	}
	if s.jump != "" && (k == tcell.KeyEnter || r == 'g') {
		n, _ := strconv.Atoi(s.jump)
		s.jump = ""
		s.msgTill = time.Time{}
		s.goTo(n-1, 0, true)
		return true
	}
	if k == tcell.KeyEscape && s.jump != "" {
		s.jump = ""
		s.msgTill = time.Time{}
		s.draw()
		return true
	}
	s.jump = ""
	if s.black && r != 'q' {
		s.black = false
		s.draw()
		return true
	}
	switch {
	case r == 'q' || k == tcell.KeyEscape:
		if s.notes {
			s.notes = false
			s.draw()
			return true
		}
		return false
	case k == tcell.KeyRight || r == ' ' || r == 'l' || k == tcell.KeyPgDn || k == tcell.KeyEnter:
		s.next(false)
	case k == tcell.KeyLeft || r == 'h' || k == tcell.KeyPgUp || k == tcell.KeyBackspace || k == tcell.KeyBackspace2:
		s.prev(false)
	case r == 'N':
		s.next(true)
	case r == 'P':
		s.prev(true)
	case k == tcell.KeyDown || r == 'j':
		if s.overflow > 0 && s.scroll < s.overflow {
			s.scroll++
			s.draw()
		} else if k == tcell.KeyDown {
			s.next(false)
		}
	case k == tcell.KeyUp || r == 'k':
		if s.scroll > 0 {
			s.scroll--
			s.draw()
		} else if k == tcell.KeyUp {
			s.prev(false)
		}
	case r == 'g' || k == tcell.KeyHome:
		s.goTo(0, 0, true)
	case r == 'G' || k == tcell.KeyEnd:
		s.goTo(len(s.deck.pages)-1, 0, true)
	case r == 'o' || k == tcell.KeyTab:
		s.overview, s.ovSel = true, s.idx
		s.draw()
	case r == 'n':
		s.notes = !s.notes
		s.draw()
	case r == 't':
		s.timerOn = !s.timerOn
		s.draw()
	case r == 'r':
		s.start = time.Now()
		s.timerOn = true
		s.draw()
	case r == 'f':
		s.footer = !s.footer
		s.draw()
	case r == 'b' || r == '.':
		s.black = true
		s.draw()
	case r == 'T':
		if len(s.themes) > 0 {
			s.themeAt = (s.themeAt + 1) % len(s.themes)
			s.pal, _ = loadPalette(s.themes[s.themeAt])
			s.say("theme: " + s.pal.name)
			s.draw()
		}
	case r == 'R':
		s.reload(true)
	case r == 'e':
		if s.edit != nil {
			line := 1
			if pg := s.deck.pages[s.idx]; pg.s != nil {
				line = slideLine(pg.s.H)
			}
			if s.termEdit {
				s.scr.Suspend()
				s.edit(s.path, line)
				s.scr.Resume()
				s.reload(true)
				s.draw()
			} else {
				s.edit(s.path, line)
				s.say("opened in the editor")
			}
		}
	case r == '?':
		s.help = true
		s.draw()
	}
	return true
}

// run is the event loop.
func (s *show) run() {
	events := make(chan tcell.Event, 16)
	quit := make(chan struct{})
	go s.scr.ChannelEvents(events, quit)
	tick := time.NewTicker(30 * time.Millisecond)
	defer tick.Stop()
	slow := time.NewTicker(300 * time.Millisecond)
	defer slow.Stop()
	s.writeSync()
	s.draw()
	lastSec := ""
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			switch ev := ev.(type) {
			case *tcell.EventKey:
				if !s.key(ev) {
					close(quit)
					return
				}
			case *tcell.EventResize:
				s.anim = nil
				s.scr.Sync()
				s.draw()
			}
		case <-tick.C:
			s.tickAnim()
		case <-slow.C:
			if s.anim != nil {
				continue
			}
			s.reload(false)
			s.readSync()
			sec := time.Now().Format("15:04:05")
			if (s.timerOn || s.presenter || s.msg != "") && sec != lastSec {
				lastSec = sec
				if s.msg != "" && time.Now().After(s.msgTill) {
					s.msg = ""
				}
				s.draw()
			}
		}
	}
}

// slideLine is where a page starts in the file, for the editor.
func slideLine(h *org.Headline) int {
	if h == nil {
		return 1
	}
	return h.Pos.Row + 1
}
var nowFunc = time.Now
