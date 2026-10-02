package pres

// `orgs pres` — an org file as a slideshow in the terminal.
//
//	orgs pres talk.org                 present it
//	orgs pres talk.org -presenter      the speaker's window: notes, next, time
//	orgs pres talk.org -theme linen    any slide theme, or builtin / term
//	orgs pres talk.org | less -R       every slide, printed
//	orgs pres talk.org -json           the outline, for a program
//
// The deck is the one worg's Presentations tab shows through reveal.js:
// `slides.BuildDeck` decides which headlines are slides (`#+SLIDE_LEVEL:`),
// takes the speaker notes out (`:NOTES:` drawers, `#+begin_notes`, a `Notes`
// heading), skips `:noexport:` and reads properties through the same ladder.
// So a slide's `:FRAGMENT:`, `:BACKGROUND:`, `:ALIGN:` and `:TRANSITION:` mean
// here what they mean there, `#+SLIDE_THEME:` picks the same palette, and a
// `TERM_` prefix (`#+TERM_THEME:`, `:TERM_TRANSITION:`) says something to this
// renderer only - the way `REVEAL_` does to reveal.
//
// The file is parsed here, not on the server: a talk is usually a file in
// front of you, and a slideshow that needed a daemon would not be one you
// could give from a laptop on a projector. A name that is not a path is looked
// for in the configured org directories.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/slides"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type Pres struct {
	fset *flag.FlagSet

	Theme      string
	Transition string
	Width      int
	Presenter  bool
	Print      bool
	NoFooter   bool
	Timer      bool
	Start      int
	ListThemes bool
}

func (self *Pres) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Pres) StartPlugin(manager *common.PluginManager)         {}
func (self *Pres) NeedsNoServer() bool                               { return true }
func (self *Pres) HelpGroup() string                                 { return "Files and code" }

func (self *Pres) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Theme, "theme", "", "slide theme (a templates/slides_theme_*.css name), builtin or term")
	fset.StringVar(&self.Transition, "transition", "", "slide, fade or none")
	fset.IntVar(&self.Width, "width", 0, "widest a column of text may be (default 88)")
	fset.BoolVar(&self.Presenter, "presenter", false, "the speaker's window: notes, the next slide and the time")
	fset.BoolVar(&self.Print, "print", false, "print every slide instead of presenting")
	fset.BoolVar(&self.NoFooter, "no-footer", false, "no footer or progress rule")
	fset.BoolVar(&self.Timer, "timer", false, "start with the timer showing")
	fset.IntVar(&self.Start, "at", 1, "slide to start on")
	fset.BoolVar(&self.ListThemes, "list-themes", false, "list the themes")
}

func (self *Pres) Exec(core *commands.Core) {
	if core.ServerSettings != nil {
		slides.SearchPath = templateDirs(core.ServerSettings.TemplatePath)
	} else {
		slides.SearchPath = templateDirs("")
	}
	words := commands.FreeArgs(self.fset)
	if self.ListThemes {
		if commands.Render(themeNames(), func() {
			for _, n := range themeNames() {
				fmt.Println(n)
			}
		}) {
			return
		}
		return
	}
	if len(words) == 0 {
		commands.Fail("which file? orgs pres talk.org")
		os.Exit(1)
	}
	var roots []string
	if core.ServerSettings != nil {
		roots = core.ServerSettings.OrgDirs
	}
	path := locate(words[0], roots)
	if path == "" {
		commands.Fail("no file %s here or in the org directories", words[0])
		os.Exit(1)
	}
	load := func() (*deck, error) { return loadDeck(path) }
	d, err := load()
	if err != nil {
		commands.Fail("%s: %v", path, err)
		os.Exit(1)
	}
	files := &finder{dir: filepath.Dir(path), roots: roots}

	themeName := self.Theme
	if themeName == "" {
		themeName = d.conf.DocStr("THEME", "")
		if themeName == "" {
			themeName = strings.TrimSpace(d.doc.Get("REVEAL_THEME"))
		}
	}
	pal, found := loadPalette(themeName)
	if !found && self.Theme != "" {
		fmt.Fprintf(os.Stderr, "no theme %q; using the built-in one (orgs pres -list-themes)\n", self.Theme)
	}
	width := self.Width
	if width <= 0 {
		width = d.conf.DocInt("WIDTH", 88)
	}
	trans := self.Transition
	if trans == "" {
		trans = strings.ToLower(d.conf.DocStr("TRANSITION", "slide"))
	}

	if commands.JsonOut {
		self.outline(d)
		return
	}
	if self.Print || !commands.Interactive() {
		self.print(d, pal, files, width)
		return
	}

	scr, err := tcell.NewScreen()
	if err == nil {
		err = scr.Init()
	}
	if err != nil {
		commands.Fail("no terminal to present on: %v", err)
		os.Exit(1)
	}
	defer scr.Fini()
	scr.HideCursor()

	s := &show{
		scr: scr, deck: d, path: path, load: load, files: files,
		pal: pal, themes: themeNames(), maxWidth: width, trans: trans,
		presenter: self.Presenter, footer: !self.NoFooter && d.conf.DocBool("FOOTER", true),
		timerOn: self.Timer || self.Presenter, sync: syncPath(path),
	}
	for i, n := range s.themes {
		if strings.EqualFold(n, pal.name) {
			s.themeAt = i
		}
	}
	s.start = nowFunc()
	if st, err := os.Stat(path); err == nil {
		s.modTime = st.ModTime()
	}
	s.idx = max(0, min(self.Start-1, len(d.pages)-1))
	s.edit, s.termEdit = editor(core)
	s.run()
}

// editor is $VISUAL or $EDITOR when one is set, since that is a program that
// wants the terminal; the configured editorTemplate otherwise.
func editor(core *commands.Core) (func(string, int), bool) {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed != "" {
		return func(file string, line int) {
			args := strings.Fields(ed)
			args = append(args, fmt.Sprintf("+%d", line), file)
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			_ = cmd.Run()
		}, true
	}
	if len(core.EditorTemplate) > 0 {
		return core.LaunchEditor, false
	}
	return nil, false
}

func locate(name string, roots []string) string {
	try := []string{name}
	if !strings.HasSuffix(name, ".org") {
		try = append(try, name+".org")
	}
	for _, t := range try {
		if st, err := os.Stat(t); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(t)
			return abs
		}
	}
	if filepath.IsAbs(name) {
		return ""
	}
	for _, root := range roots {
		for _, t := range try {
			p := filepath.Join(root, t)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

func loadDeck(path string) (*deck, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg := org.New()
	cfg.DefaultSettings["TODO"] = "TODO NEXT WAITING IN-PROGRESS BLOCKED | DONE CANCELLED"
	doc := cfg.Parse(f, path)
	if doc.Error != nil {
		return nil, doc.Error
	}
	d := newDeck(doc, path)
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("no slides: the file has no headlines and no title")
	}
	return d, nil
}

// print writes every slide, every fragment showing, as ansi when the output
// is a terminal or forced colour, and as plain text into a pipe.
func (self *Pres) print(d *deck, pal *palette, files *finder, width int) {
	w := 100
	if c, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && c > 20 {
		w = c
	} else if c := os.Getenv("COLUMNS"); c != "" {
		fmt.Sscanf(c, "%d", &w)
	}
	colour := commands.Colour()
	for i := range d.pages {
		// Tall enough for anything: the trailing ground is trimmed below.
		h := 400
		c := d.compose(i, 0, w, h, view{pal: pal, files: files, maxWidth: width, noStep: true})
		cv := c.c
		// The page's own ground, so a slide with a :BACKGROUND: of its own
		// is not taken to be all content.
		_, ground, _ := cv.cells[0].st.Decompose()
		inked := func(y int) bool {
			for x := 0; x < cv.w; x++ {
				c := cv.cells[y*cv.w+x]
				if _, bg, _ := c.st.Decompose(); c.r != ' ' || bg != ground {
					return true
				}
			}
			return false
		}
		first, last := -1, 0
		for y := 0; y < cv.h; y++ {
			if inked(y) {
				if first < 0 {
					first = y
				}
				last = y
			}
		}
		if first < 0 {
			continue
		}
		if colour {
			// A line of the slide's ground above and below, so a page
			// reads as a page.
			first, last = max(0, first-1), min(cv.h-2, last)
		} else {
			last--
		}
		for y := first; y <= last+1 && y < cv.h; y++ {
			fmt.Println(ansiRow(cv, y, colour))
		}
		if i < len(d.pages)-1 {
			if colour {
				fmt.Println()
			} else {
				fmt.Println(strings.Repeat("─", w))
			}
		}
	}
}

func ansiRow(cv *canvas, y int, colour bool) string {
	b := strings.Builder{}
	var last tcell.Style
	started := false
	for x := 0; x < cv.w; x++ {
		c := cv.cells[y*cv.w+x]
		if c.cont {
			continue
		}
		if colour && (!started || c.st != last) {
			b.WriteString(sgr(c.st))
			last, started = c.st, true
		}
		b.WriteRune(c.r)
		for _, r := range c.comb {
			b.WriteRune(r)
		}
	}
	if colour {
		b.WriteString("\033[0m")
		return b.String()
	}
	return strings.TrimRight(b.String(), " ")
}

func sgr(st tcell.Style) string {
	fg, bg, attr := st.Decompose()
	codes := []string{"0"}
	if attr&tcell.AttrBold != 0 {
		codes = append(codes, "1")
	}
	if attr&tcell.AttrDim != 0 {
		codes = append(codes, "2")
	}
	if attr&tcell.AttrItalic != 0 {
		codes = append(codes, "3")
	}
	if attr&tcell.AttrUnderline != 0 {
		codes = append(codes, "4")
	}
	if attr&tcell.AttrStrikeThrough != 0 {
		codes = append(codes, "9")
	}
	if r, g, b := fg.RGB(); r >= 0 && fg != tcell.ColorDefault {
		codes = append(codes, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	}
	if r, g, b := bg.RGB(); r >= 0 && bg != tcell.ColorDefault {
		codes = append(codes, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	}
	return "\033[" + strings.Join(codes, ";") + "m"
}

// outline is the deck for a program: what `-json` asks for.
func (self *Pres) outline(d *deck) {
	type row struct {
		N     int
		Title string
		Level int
		Line  int
		Steps int
		Notes bool
	}
	rows := []row{}
	for i, pg := range d.pages {
		r := row{N: i + 1, Title: d.titleOf(i)}
		if pg.s != nil {
			r.Level = pg.s.Level
			r.Line = slideLine(pg.s.H)
			r.Notes = len(pg.s.Notes) > 0
			r.Steps = d.compose(i, 0, 100, 60, view{pal: builtin(), maxWidth: 88}).steps
		}
		rows = append(rows, r)
	}
	out, _ := json.MarshalIndent(rows, "", "  ")
	fmt.Println(string(out))
}

func init() {
	commands.AddCmd("pres", "present an org file as a slideshow in the terminal",
		func() commands.Cmd { return &Pres{Start: 1} })
}

/* SDOC: Commands

* Pres

  =orgs pres= gives an org file as a slideshow in the terminal: the deck worg's
  Presentations tab shows through reveal.js, drawn in box characters, colour and
  half-block pictures, for the talk given over ssh, in tmux beside the code, or
  from a laptop with no browser worth trusting on the projector.

  #+BEGIN_SRC bash
  orgs pres talk.org                  # present it
  orgs pres talk.org -presenter       # in a second terminal: notes, next slide, time
  orgs pres talk.org -theme linen     # any slide theme; builtin and term too
  orgs pres talk.org -at 12           # start on slide 12
  orgs pres talk.org -print | less -R # every slide, every step, printed
  orgs pres talk.org -json            # the outline, for a program
  orgs pres -list-themes
  #+END_SRC

  A name that is not a path is looked for in the configured org directories, and
  no server is needed: the file is read where you are.

** The deck

   The same rules as the html presentation exporters, because it is the same
   code deciding them:

   - =#+SLIDE_LEVEL: 2= makes level-2 headlines the slides and level-1 headlines
     the sections; a section with nothing of its own is drawn as a divider in
     large type. Without it every headline is a slide.
   - =#+TITLE:=, =#+SUBTITLE:=, =#+AUTHOR:=, =#+DATE:= and anything written
     above the first headline make the title page.
   - Speaker notes - a =:NOTES:= drawer, a =#+begin_notes= block, or a child
     heading called Notes - never reach the slide.
   - =:noexport:= or =:noslide:= leaves a heading out.
   - =:FRAGMENT: t= on a slide reveals its top-level list one item at a time;
     =#+ATTR_SLIDE: :frag= above any element makes that element a step.
   - =:BACKGROUND: #f4efe4= colours one slide, and its text turns light or dark
     to suit, as in the browser.
   - =:ALIGN: center= centres a slide; =:TRANSITION: fade= changes how it arrives.

   Settings are read with the presentation ladder: a slide's =:TERM_FOO:=, its
   =:SLIDE_FOO:=, its =:FOO:=, then the document's =#+TERM_FOO:= and
   =#+SLIDE_FOO:=. The =TERM_= forms are for this renderer only.

   | Setting               | Meaning                                       | Default  |
   |-----------------------+-----------------------------------------------+----------|
   | =#+SLIDE_THEME:=      | palette, from =templates/slides_theme_*.css=  | built in |
   | =#+TERM_TRANSITION:=  | =slide=, =fade= or =none=                     | slide    |
   | =#+TERM_WIDTH:=       | widest a column of text gets                  | 88       |
   | =#+TERM_FOOTER: nil=  | no footer or progress rule                    | shown    |

   A theme is the same file the html decks use: its =--slide-bg=, =--slide-ink=,
   =--slide-accent= and the rest become the terminal's colours, and its
   =@hljs:= style picks the nearest code colouring. =term= uses the terminal's
   own colours, for transparent terminals.

** What is drawn

   Paragraphs with bold, italic, underline, strike, =code=, links, sub- and
   superscripts (H_{2}O as H₂O) and smart dashes; lists with checkboxes;
   tables in a rounded box with columns of numbers right-aligned; source blocks
   coloured by language, with line numbers for =-n=; quotes, verse, centred
   text, special blocks (=#+begin_tip=) as labelled cards; footnotes at the foot
   of their slide; and pictures (png, jpeg, gif) in half blocks, with their
   =#+CAPTION:=. An svg or a web image is drawn as a labelled frame.

   A slide too tall for the terminal gives up its blank lines first, then
   scrolls with the arrow keys; a marker says there is more.

** Keys

   | Key                      | Does                                      |
   |--------------------------+-------------------------------------------|
   | → space l PgDn ⏎         | next step, or next slide                  |
   | ← h PgUp ⌫               | back                                      |
   | N P                      | next or previous slide, skipping steps    |
   | ↓ ↑ j k                  | scroll a slide too tall for the screen    |
   | g G                      | first or last slide                       |
   | 12 then ⏎                | slide 12                                  |
   | o tab                    | every slide, to pick from                 |
   | n                        | speaker notes under the slide             |
   | t r                      | show the timer, restart it                |
   | f                        | footer and progress rule                  |
   | T                        | next theme                                |
   | b .                      | black screen                              |
   | e                        | edit this slide in =$EDITOR=              |
   | R                        | reload now                                |
   | ? q                      | keys, quit                                |

   The file is reloaded whenever it is saved, keeping your place, so a deck can
   be written in one window and watched in another.

** Presenting with two windows

   =orgs pres talk.org -presenter= in a second terminal shows the speaker's
   view: this slide's notes, the step you are on, what comes next, the time
   since you started and the clock. The two windows follow each other, and
   either can drive.
EDOC */
