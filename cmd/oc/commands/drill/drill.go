package drill

// `orgs drill` — flashcards at the command line.
//
//	orgs drill                    pick a deck and drill it
//	orgs drill Spanish            drill the deck called Spanish
//	orgs drill Spanish -cram      everything not seen in 12 hours, nothing rescheduled
//	orgs drill -q 'HasTags("drill")'    a deck that is not saved: any query
//	orgs drill -resume            carry on where q left off
//	orgs drill ls                 the decks, and what each has due
//	orgs drill cards Spanish      every card: status, next review, ease
//
// The terminal half of worg's Flashcards tab, over the same endpoints: the
// server finds the cards and writes every rating, org-drill's way, so a deck
// can be drilled here, in worg and in Emacs' org-drill by turns. The session -
// which card next, the again pile, the limits, which clozes a card hides - is
// session.go. Cards are drawn with the slide renderer (pres.RenderOrg), so a
// table, a source block or a picture on a card looks the way it does on a
// slide, in a slide theme's colours (-theme).

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/pres"
	"github.com/ihdavids/orgs/internal/common"
)

type Drill struct {
	fset *flag.FlagSet

	Cram      bool
	Query     string
	Algorithm string
	Theme     string
	Resume    bool
	Max       int
	Minutes   int
}

func (self *Drill) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Drill) StartPlugin(manager *common.PluginManager)         {}
func (self *Drill) HelpGroup() string                                 { return "Files and code" }

func (self *Drill) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Cram, "cram", false, "cram: everything not seen lately, with no limits; nothing is rescheduled")
	fset.StringVar(&self.Query, "q", "", "drill the cards this query finds, without a saved deck")
	fset.StringVar(&self.Algorithm, "algorithm", "", "with -q: sm5 (default), sm2 or simple8")
	fset.StringVar(&self.Theme, "theme", "", "colours: a slide theme, builtin or term")
	fset.BoolVar(&self.Resume, "resume", false, "carry on with the session q left")
	fset.IntVar(&self.Max, "max", 0, "cards this session (overrides the deck)")
	fset.IntVar(&self.Minutes, "minutes", 0, "minutes this session (overrides the deck)")
}

func (self *Drill) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		switch words[0] {
		case "ls", "list", "decks", "cards", "stats", "preview":
			verb, words = words[0], words[1:]
		}
	}
	name := strings.TrimSpace(strings.Join(words, " "))
	switch verb {
	case "ls", "list", "decks":
		self.listDecks(core)
	case "cards":
		self.listCards(core, self.deckNamed(core, name))
	case "stats", "preview":
		self.stats(core, name)
	default:
		self.drill(core, name)
	}
}

// ── Decks ───────────────────────────────────────────────────────────────────

func decks(core *commands.Core) []common.DrillDeck {
	ds, err := commands.SendReceiveGetErr[[]common.DrillDeck](core, "ext/drill/decks", map[string]string{})
	if err != nil {
		commands.Fail("could not ask the server for decks: %v", err)
		os.Exit(1)
	}
	return ds
}

// deckNamed finds a deck by name - exactly, then by its start, ignoring case -
// or offers the decks to pick from. "" with -q is the unsaved deck.
func (self *Drill) deckNamed(core *commands.Core, name string) string {
	if self.Query != "" {
		return ""
	}
	ds := decks(core)
	if len(ds) == 0 {
		commands.Fail("no decks yet: make one in worg's Flashcards tab, or drill a query with -q 'HasTags(\"drill\")'")
		os.Exit(1)
	}
	if name != "" {
		for _, d := range ds {
			if d.Name == name {
				return d.Name
			}
		}
		hits := []string{}
		for _, d := range ds {
			if strings.HasPrefix(strings.ToLower(d.Name), strings.ToLower(name)) {
				hits = append(hits, d.Name)
			}
		}
		if len(hits) == 1 {
			return hits[0]
		}
		if !commands.Interactive() {
			commands.Fail("no deck called %q (orgs drill ls)", name)
			os.Exit(1)
		}
	}
	if len(ds) == 1 {
		return ds[0].Name
	}
	if !commands.Interactive() {
		commands.Fail("which deck? orgs drill ls")
		os.Exit(1)
	}
	self_, _ := commands.SelfCommand(core)
	lines := []string{}
	for _, d := range ds {
		q := d.Query
		if d.StoredQuery != "" {
			q = "saved: " + d.StoredQuery
		}
		lines = append(lines, commands.PickLine([]string{d.Name}, fmt.Sprintf("%s%s%s  %s%s%s",
			commands.C(commands.AnsiBold), d.Name, commands.C(commands.AnsiReset), commands.C(commands.AnsiDim), q, commands.C(commands.AnsiReset))))
	}
	picked := commands.Pick(commands.PickOpts{
		Lines: lines, AddressFields: 1, Prompt: "deck> ", Header: "enter drill · ctrl-/ pane",
		Preview: self_ + " drill stats {1} 2>/dev/null",
		Extra:   []string{"--query", name},
	})
	if len(picked) == 0 {
		os.Exit(130)
	}
	addr, _ := commands.Address(picked[0], 1)
	return addr[0]
}

func (self *Drill) fetch(core *commands.Core, deck string, cram bool) common.DrillCardsReply {
	ps := map[string]string{"format": "org"}
	if deck != "" {
		ps["deck"] = deck
	} else {
		ps["query"] = self.Query
		if self.Algorithm != "" {
			ps["algorithm"] = self.Algorithm
		}
	}
	if cram {
		ps["cram"] = "1"
	}
	r, err := commands.SendReceiveGetErr[common.DrillCardsReply](core, "drill/cards", ps)
	if err != nil {
		commands.Fail("could not read the deck: %v", err)
		os.Exit(1)
	}
	if !r.Ok {
		commands.Fail("%s", r.Msg)
		os.Exit(1)
	}
	return r
}

func due(c common.DrillCounts) int { return c.New + c.Failed + c.Overdue + c.Young + c.Old }

func (self *Drill) listDecks(core *commands.Core) {
	type row struct {
		Name        string
		Description string `json:",omitempty"`
		Query       string
		Due         int
		Counts      common.DrillCounts
	}
	rows := []row{}
	for _, d := range decks(core) {
		q := d.Query
		if d.StoredQuery != "" {
			q = "saved: " + d.StoredQuery
		}
		r, err := commands.SendReceiveGetErr[common.DrillCardsReply](core, "drill/cards", map[string]string{"deck": d.Name, "format": "org"})
		row := row{Name: d.Name, Description: d.Description, Query: q}
		if err == nil && r.Ok {
			row.Counts, row.Due = r.Counts, due(r.Counts)
		}
		rows = append(rows, row)
	}
	commands.Render(rows, func() {
		w := 0
		for _, r := range rows {
			w = max(w, commands.RuneLen(r.Name))
		}
		for _, r := range rows {
			dueTxt := fmt.Sprintf("%3d due", r.Due)
			if r.Due > 0 {
				dueTxt = commands.C(commands.AnsiGreen) + dueTxt + commands.C(commands.AnsiReset)
			} else {
				dueTxt = commands.C(commands.AnsiDim) + dueTxt + commands.C(commands.AnsiReset)
			}
			fmt.Printf("%s%-*s%s  %s  %s%d new · %d failed · %d cards%s  %s%s%s\n",
				commands.C(commands.AnsiBold), w, r.Name, commands.C(commands.AnsiReset), dueTxt,
				commands.C(commands.AnsiDim), r.Counts.New, r.Counts.Failed, r.Counts.Total, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiDim), r.Query, commands.C(commands.AnsiReset))
		}
	})
}

// stats is one deck at a glance, which is also the deck picker's pane.
func (self *Drill) stats(core *commands.Core, name string) {
	deck := self.deckNamed(core, name)
	r := self.fetch(core, deck, false)
	if commands.RenderOne(r.Counts, nil) {
		return
	}
	c := r.Counts
	title := deck
	if title == "" {
		title = self.Query
	}
	fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), title, commands.C(commands.AnsiReset))
	if r.Deck.Description != "" {
		fmt.Println(r.Deck.Description)
	}
	fmt.Printf("%s%s · %s%s\n\n", commands.C(commands.AnsiDim), r.Query, strings.ToUpper(r.Deck.Algorithm), commands.C(commands.AnsiReset))
	line := func(label string, n int, colour string) {
		fmt.Printf("  %s%4d%s  %s\n", commands.C(colour), n, commands.C(commands.AnsiReset), label)
	}
	line("due now", due(c), commands.AnsiBold)
	line("new", c.New, commands.AnsiBlue)
	line("failed", c.Failed, commands.AnsiRed)
	line("overdue", c.Overdue, commands.AnsiGold)
	line("young", c.Young, commands.AnsiGreen)
	line("old", c.Old, commands.AnsiGreen)
	line("not yet due", c.Future, commands.AnsiDim)
	line("due tomorrow", c.DueTomorrow, commands.AnsiDim)
	if c.Skipped > 0 {
		line("leeches skipped", c.Skipped, commands.AnsiRed)
	}
	for _, p := range r.Problems {
		fmt.Printf("\n%sleft out: %s%s", commands.C(commands.AnsiDim), p, commands.C(commands.AnsiReset))
	}
	if len(r.Problems) > 0 {
		fmt.Println()
	}
}

func (self *Drill) listCards(core *commands.Core, deck string) {
	r := self.fetch(core, deck, false)
	cards := r.Cards
	sort.SliceStable(cards, func(a, b int) bool { return cards[a].Scheduled < cards[b].Scheduled })
	type row struct {
		Name, Type, Status, Scheduled, File string
		Line, Reviews, Failures           int
		Ease                              *float64 `json:",omitempty"`
		Leech                             bool
	}
	rows := []row{}
	for _, c := range cards {
		rows = append(rows, row{c.Name, c.Type, c.Status, c.Scheduled, c.File, c.Line, c.Data.TotalRepeats, c.Data.Failures, c.Data.Ease, c.Leech})
	}
	colour := map[string]string{"new": commands.AnsiBlue, "failed": commands.AnsiRed, "overdue": commands.AnsiGold, "young": commands.AnsiGreen, "old": commands.AnsiGreen}
	commands.Render(rows, func() {
		w := 0
		for _, r := range rows {
			w = min(48, max(w, commands.RuneLen(r.Name)))
		}
		for _, r := range rows {
			st := r.Status
			if st == "future" {
				st = "not due"
			}
			sched := r.Scheduled
			if sched == "" {
				sched = "—"
			}
			ease := "  —"
			if r.Ease != nil {
				ease = fmt.Sprintf("%.2f", *r.Ease)
			}
			leech := ""
			if r.Leech {
				leech = commands.C(commands.AnsiRed) + " leech" + commands.C(commands.AnsiReset)
			}
			fmt.Printf("%-*s  %s%-8s%s %-10s  ease %s  %2d reviews %2d failed%s\n", w, commands.Ellipsis(r.Name, w),
				commands.C(colour[r.Status]), st, commands.C(commands.AnsiReset), sched, ease, r.Reviews, r.Failures, leech)
		}
	})
}

// ── A saved session, for -resume ────────────────────────────────────────────

// org-drill-resume carries on in the same Emacs; a command that has exited has
// to have written down where it was. The cards are read fresh on resume - the
// file may have been edited - and the queues are kept by hash.
type saved struct {
	Deck    string
	Query   string
	Algo    string
	Cram    bool
	Session *Session
	Saved   time.Time
}

func statePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "orgs", "drill-session.json")
}

func saveState(st saved) {
	st.Saved = time.Now()
	if b, err := json.Marshal(st); err == nil {
		_ = os.MkdirAll(filepath.Dir(statePath()), 0o755)
		_ = os.WriteFile(statePath(), b, 0o644)
	}
}

func loadState() *saved {
	b, err := os.ReadFile(statePath())
	if err != nil {
		return nil
	}
	var st saved
	if json.Unmarshal(b, &st) != nil || st.Session == nil {
		return nil
	}
	return &st
}

func clearState() { _ = os.Remove(statePath()) }

// ── Drilling ────────────────────────────────────────────────────────────────

func (self *Drill) drill(core *commands.Core, name string) {
	if !commands.Interactive() {
		commands.Fail("drilling needs a person at a terminal (orgs drill ls, orgs drill cards are for programs)")
		os.Exit(1)
	}
	var s *Session
	var deck string
	rng := NewRng(time.Now().UnixNano())
	if self.Resume {
		st := loadState()
		if st == nil {
			commands.Fail("no session to resume")
			os.Exit(1)
		}
		deck, self.Query, self.Algorithm = st.Deck, st.Query, st.Algo
		r := self.fetch(core, deck, st.Cram)
		s = st.Session
		s.Cards = map[string]*common.DrillCard{}
		for i := range r.Cards {
			s.Cards[r.Cards[i].Hash] = &r.Cards[i]
		}
		s.Deck = r.Deck
		// A card gone from the file - deleted, or its heading renamed - is
		// dropped from the queues rather than asked as a blank.
		s.prune()
	} else {
		deck = self.deckNamed(core, name)
		r := self.fetch(core, deck, self.Cram)
		if self.Max != 0 {
			r.Deck.MaxItems = self.Max
		}
		if self.Minutes != 0 {
			r.Deck.MaxMinutes = self.Minutes
		}
		s = NewSession(r, self.Cram, time.Now(), rng)
		if s.Pending() == 0 && self.Cram {
			fmt.Fprintf(os.Stderr, "nothing to cram in %s: every card was seen in the last %d hours\n", deckLabel(deck, self.Query), r.Deck.CramHours)
			return
		}
		if s.Pending() == 0 {
			fmt.Fprintf(os.Stderr, "nothing due in %s", deckLabel(deck, self.Query))
			if r.Counts.DueTomorrow > 0 {
				fmt.Fprintf(os.Stderr, " - %d tomorrow", r.Counts.DueTomorrow)
			}
			fmt.Fprintln(os.Stderr, " (-cram asks them anyway)")
			return
		}
	}
	if core.ServerSettings != nil {
		pres.UseTemplates(core.ServerSettings.TemplatePath)
	} else {
		pres.UseTemplates("")
	}
	pal, _ := pres.LoadPalette(self.Theme)
	var roots []string
	if core.ServerSettings != nil {
		roots = core.ServerSettings.OrgDirs
	}
	ui := &screen{
		core: core, s: s, deckName: deckLabel(deck, self.Query), pal: pal, rng: rng, roots: roots,
		refetch: func() common.DrillCardsReply { return self.fetch(core, deck, s.Cram) },
	}
	quit := ui.run()
	if quit && s.Pending() > 0 {
		saveState(saved{Deck: deck, Query: self.Query, Algo: self.Algorithm, Cram: s.Cram, Session: s})
		fmt.Fprintf(os.Stderr, "%d reviewed; your ratings are saved. orgs drill -resume carries on (%d left).\n", len(s.Done), s.Pending())
	} else {
		clearState()
	}
}

func deckLabel(deck, query string) string {
	if deck != "" {
		return deck
	}
	return query
}

// prune drops cards that are no longer in the deck from every queue.
func (s *Session) prune() {
	keep := func(xs []string) []string {
		out := []string{}
		for _, h := range xs {
			if s.Cards[h] != nil {
				out = append(out, h)
			}
		}
		return out
	}
	s.Failed, s.Overdue, s.Young, s.Fresh, s.Old, s.Again = keep(s.Failed), keep(s.Overdue), keep(s.Young), keep(s.Fresh), keep(s.Old), keep(s.Again)
	if s.Current != "" && s.Cards[s.Current] == nil {
		s.Current = ""
	}
	if s.FailedHere == nil {
		s.FailedHere = map[string]bool{}
	}
}

func init() {
	commands.AddCmd("drill", "drill a deck of flashcards: spaced repetition, org-drill style",
		func() commands.Cmd { return &Drill{} })
}

/* SDOC: Commands

* Drill

  =orgs drill= drills a deck of flashcards at the command line: the decks of
  worg's Flashcards tab, the same cards, the same scheduling. Ratings are
  written by the server into the cards' =DRILL_*= properties, org-drill's
  way, so a deck can be drilled here, in worg and with Emacs' =org-drill= by
  turns. The user guide is =docs/drill.org=.

  #+BEGIN_SRC bash
  orgs drill                          # pick a deck, drill it
  orgs drill Spanish                  # a deck by name (or the start of one)
  orgs drill Spanish -cram            # everything not seen in 12 hours; nothing rescheduled
  orgs drill -q 'HasTags("drill")'    # any query, without saving a deck
  orgs drill Spanish -max 10 -minutes 5   # a shorter session than the deck's
  orgs drill -resume                  # carry on where q left off
  orgs drill ls                       # the decks and what each has due
  orgs drill stats Spanish            # one deck at a glance
  orgs drill cards Spanish            # every card: status, next review, ease, failures
  #+END_SRC

  =ls=, =stats= and =cards= take =-json= and =-format= for scripts; drilling
  needs a person at a terminal.

** The screen

   Across the top is org-drill's prompt line: the time so far, the card's
   status letter (=N= new, =Y= young, =o= old, =!= overdue, =F= failed, =C=
   cram) and how many are done, failed, mature and new. Under it, a rule shows
   how far through the session's limit you are.

   The card is drawn the way =orgs pres= draws a slide: tables, source blocks
   and pictures as they are written, in a slide theme's colours (=-theme=).
   Hidden clozes show as =[...]= or =[hint...]=; in the answer they are bold.

   | Key          | Question                   | Answer                       |
   |--------------+----------------------------+------------------------------|
   | space, any   | show the answer            |                              |
   | =0= - =5=    |                            | rate it; each button says when the card comes back |
   | =e=          | edit the card in =$EDITOR= | edit the card                |
   | =s=          | skip it                    |                              |
   | =?=          |                            | what the ratings mean        |
   | ↑ ↓ PgUp PgDn | scroll a tall card        | scroll                       |
   | =q=, esc     | stop                       | stop                         |

   A failed card comes back later in the session until you pass it. At the end
   you get org-drill's report, and its warning if too much was failed; *k*
   keeps going on what is left.

   =q= saves where you were, and =orgs drill -resume= carries on - reading the
   cards again, in case you edited them. Editing a card with =e= reads it again
   when the editor closes, so the card asked is the card as it now reads.

   A deck that asks for typed answers (in worg's deck settings) has a line to
   type into before the answer is shown, and your answer is shown beside it.
EDOC */
