// Package dnd is the command line front end for the orgs D&D module.
package dnd

/* SDOC: Commands

* Dnd

  The =dnd= command is the client side of the D&D character module. It talks to
  the server over the =/dnd/*= REST API, so the rules, the ruleset modules and
  the character sheet renderers all live on the server.

  #+BEGIN_SRC bash
  orgs dnd new                          # guided character creation
  orgs dnd new -level 3 -name Lyra      # with a head start
  orgs dnd random -level 5 -out npc.org # roll a whole character
  orgs dnd list classes                 # browse a ruleset
  orgs dnd info -kind class -id wizard  # read one entry in full
  orgs dnd characters                   # every sheet the server knows about
  orgs dnd show -file lyra.org          # print a sheet in the terminal
  orgs dnd sheet -file lyra.org -format pdf -out lyra.pdf
  orgs dnd refresh -file lyra.org       # recompute after hand editing
  orgs dnd rulesets                     # what content is loaded
  orgs dnd import -ddb 12345678         # bring a character over from d&d beyond
  #+END_SRC

  =orgs dnd new= walks you through creation the way a character builder does:
  every prompt lists the valid choices with a one line summary, marks the ones
  that suit your class with a star, and lets you ask for the full rules text,
  roll the choice randomly, or step back to change your mind.

  On a wide enough terminal the chooser is drawn in two panes: the list on the
  left, and the full rules text of whatever the cursor is on - the spell, item,
  feat or feature - on the right, so you can read before you commit. When a
  question takes several answers the header keeps a running count of what you
  have ticked ("selected 1 of 2") along with their names, and if you confirm the
  wrong number the list comes back with your picks still selected so you only
  have to correct them.

  Long lists are filtered by typing. The match is fuzzy on the name of an
  option, so =eldbl= finds Eldritch Blast, and a plain word also matches the
  summary beside it, so =fire= finds the spells that only mention it. The
  filter is editable: backspace takes a character off and the list widens
  again, =ctrl+w= takes a word, and escape clears it. The count beside the
  question says how much of the list you are looking at.

** Importing from D&D Beyond

  =orgs dnd import= converts a character you already have on D&D Beyond into an
  org character sheet, the same sheet =orgs dnd new= writes.

  #+BEGIN_SRC bash
  orgs dnd import -ddb https://www.dndbeyond.com/characters/12345678
  orgs dnd import -ddb 12345678 -out characters/lyra.org -force
  orgs dnd import -ddb 12345678 -preview      # convert and print, write nothing
  orgs dnd import -json saved.json            # from a payload you saved yourself
  orgs dnd import -forget                     # clear the saved session cookie
  #+END_SRC

  D&D Beyond has no login api, so a character whose privacy is set to public
  imports with no credential at all, and a private one asks for the
  =CobaltSession= cookie your browser is already holding - the prompt says where
  to find it. The cookie is traded for a short lived token by D&D Beyond's own
  auth service and can be kept in your system keyring so you are only asked
  once; it is never sent to your orgs server, which only ever sees the
  character json. =-cookie= and the =ORGS_DDB_COBALT= environment variable pass
  it in without a prompt for scripting.

  Everything is matched onto your ruleset by name, and anything that does not
  match - a subclass from a book the srd does not carry, a homebrew item - is
  still written onto the sheet under its own name and listed afterwards as
  something to check, so nothing is quietly dropped. Importing over a sheet
  that is already there keeps that sheet's identity and its inventory history,
  which D&D Beyond has no equivalent of.

  The appearance questions - age, height, weight, eyes, skin and hair - come
  with suggestions drawn from your race and your class, so a hill dwarf druid
  is offered warm hazel eyes and leaf tangled hair, and a tiefling warlock is
  offered eyes that are a solid orb of colour. You can still type your own, but
  it has to make sense: heights and weights are read in either imperial or
  metric and have to land somewhere your race could plausibly be, and a colour
  has to be a word rather than a number.

EDOC */

import (
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/dnd"
	survey "gopkg.in/AlecAivazis/survey.v1"
)

// menu entries that are not ruleset choices
const (
	entryDetails = "  ···  show me the details of each option"
	entryRandom  = "  ···  choose for me"
	entryBack    = "  ···  go back a step"
	entryCustom  = "  ···  write my own"
	entrySkip    = "  ···  skip this"
	entryQuit    = "  ···  quit without saving"
	entryAgain   = "  ···  let me pick again"
	entryFill    = "  ···  fill them in"
	entryBlank   = "  ···  leave this blank"
)

// simple ansi styling, kept minimal so it degrades gracefully
const (
	cReset = "\033[0m"
	cBold  = "\033[1m"
	cDim   = "\033[2m"
	cRed   = "\033[31m"
	cGold  = "\033[33m"
	cCyan  = "\033[36m"
	cGreen = "\033[32m"
)

type Dnd struct {
	fset *flag.FlagSet

	// lastPicks remembers what was chosen for each step, so that a rejected
	// answer (or a wrong count) comes back with those choices still selected
	// instead of an empty list.
	lastPicks map[string][]string

	Ruleset string
	Name    string
	Player  string
	File    string
	Out     string
	Format  string
	Kind    string
	Id      string
	Filter  string
	Level   int
	Force   bool
	Open    bool
	Local   bool

	// D&D Beyond import
	Ddb     string
	Json    string
	Cookie  string
	Dump    string
	Preview bool
	Forget  bool
}

func (self *Dnd) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Dnd) StartPlugin(manager *common.PluginManager) {}

func (self *Dnd) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Ruleset, "ruleset", "", "ruleset id to use (default srd)")
	fset.StringVar(&self.Name, "name", "", "character name")
	fset.StringVar(&self.Player, "player", "", "player name")
	fset.StringVar(&self.File, "file", "", "character sheet org file")
	fset.StringVar(&self.Out, "out", "", "output filename")
	fset.StringVar(&self.Format, "format", "html", "sheet format: html, latex or pdf")
	fset.StringVar(&self.Kind, "kind", "", "catalog kind for info/list")
	fset.StringVar(&self.Id, "id", "", "entry id for info, or class id for subclasses/spells")
	fset.StringVar(&self.Filter, "filter", "", "substring filter for list")
	fset.IntVar(&self.Level, "level", 0, "character level")
	fset.BoolVar(&self.Force, "force", false, "overwrite an existing character sheet")
	fset.BoolVar(&self.Open, "open", false, "open the sheet in your editor when done")
	fset.BoolVar(&self.Local, "local", true, "write exported sheets on the server")
	fset.StringVar(&self.Ddb, "ddb", "", "d&d beyond character url or id to import")
	fset.StringVar(&self.Json, "json", "", "import from a saved d&d beyond json file")
	fset.StringVar(&self.Cookie, "cookie", "", "d&d beyond CobaltSession cookie (asked for if needed)")
	fset.StringVar(&self.Dump, "dump", "", "save the d&d beyond payload to this file as well")
	fset.BoolVar(&self.Preview, "preview", false, "convert and print without writing a sheet")
	fset.BoolVar(&self.Forget, "forget", false, "clear the saved d&d beyond session cookie")
}

func (self *Dnd) Exec(core *commands.Core) {
	sub := "new"
	if self.fset != nil {
		args := self.fset.Args()
		if len(args) > 0 {
			sub = args[0]
			args = args[1:]
		}
		// Flags may follow the subcommand and they may follow a word after it,
		// so keep alternating between the two rather than stopping at the
		// first thing that is not a flag - "orgs dnd list spells -filter fire"
		// means the same as "orgs dnd list -filter fire spells".
		words := []string{}
		for {
			self.fset.Parse(args)
			args = self.fset.Args()
			if len(args) == 0 {
				break
			}
			words = append(words, args[0])
			args = args[1:]
		}
		if len(words) > 0 && self.Kind == "" {
			self.Kind = words[0]
		}
	}
	switch sub {
	case "new", "create":
		self.runNew(core)
	case "random", "roll":
		self.runRandom(core)
	case "list", "browse":
		self.runList(core)
	case "info", "detail":
		self.runInfo(core)
	case "characters", "chars":
		self.runCharacters(core)
	case "show", "print":
		self.runShow(core)
	case "sheet", "export":
		self.runSheet(core)
	case "refresh", "recompute":
		self.runRefresh(core)
	case "import", "ddb", "dndbeyond":
		if self.Forget {
			ddbForget()
			return
		}
		self.runImport(core)
	case "rulesets", "modules":
		self.runRulesets(core)
	case "reload":
		self.runReload(core)
	case "help", "-h", "--help":
		self.usage()
	default:
		fmt.Printf("unknown dnd subcommand %q\n\n", sub)
		self.usage()
	}
}

func (self *Dnd) usage() {
	fmt.Print(`orgs dnd - Dungeons & Dragons character sheets

  orgs dnd new [-level N] [-name X] [-player Y] [-ruleset id] [-file out.org]
      Guided character creation. Every prompt lists the valid choices.

  orgs dnd random [-level N] [-name X] [-out file.org]
      Roll a complete character with no questions asked.

  orgs dnd list <races|classes|subclasses|backgrounds|spells|items|feats|skills>
      Browse a ruleset. Use -filter to narrow it down, -id for a parent class.

  orgs dnd info -kind class -id wizard
      Print one catalog entry in full.

  orgs dnd characters
      List every character sheet the server knows about.

  orgs dnd show -file lyra.org
      Print a computed character sheet in the terminal.

  orgs dnd import -ddb <url or id> [-out lyra.org] [-preview] [-force]
      Bring a character over from D&D Beyond. Public characters need nothing;
      a private one asks for your browser's CobaltSession cookie.
      -dump <file> keeps the raw payload, -json <file> imports one back.

  orgs dnd sheet -file lyra.org [-format html|latex|pdf] [-out lyra.pdf]
      Render a character sheet.

  orgs dnd refresh -file lyra.org
      Recompute the derived sections after hand editing the property drawer.

  orgs dnd rulesets | reload
      Show or reload the loaded ruleset modules.
`)
}

// ----------------------------------------------------------------------------
// REST helpers
// ----------------------------------------------------------------------------

func post[REQ any, RESP any](core *commands.Core, path string, req *REQ) (RESP, error) {
	return common.RestPost[RESP](&core.Rest, path, req)
}

func get[RESP any](core *commands.Core, path string, params map[string]string) RESP {
	return common.RestGet[RESP](&core.Rest, path, params)
}

// ----------------------------------------------------------------------------
// Interactive creation
// ----------------------------------------------------------------------------

func (self *Dnd) runNew(core *commands.Core) {
	req := dnd.NewSessionRequest{
		Ruleset: self.Ruleset, Name: self.Name, Player: self.Player,
		Level: self.Level, Filename: self.File,
	}
	prompt, err := post[dnd.NewSessionRequest, dnd.Prompt](core, "dnd/session", &req)
	if err != nil {
		fmt.Printf("%sCould not start character creation: %s%s\n", cRed, err, cReset)
		return
	}
	if prompt.Session == "" {
		fmt.Printf("%sThe server did not open a session. Is the dnd module available?%s\n", cRed, cReset)
		return
	}
	fmt.Printf("\n%s%sCharacter creation%s  %sruleset %s, session %s%s\n\n",
		cBold, cGold, cReset, cDim, orDefault(req.Ruleset, "srd"), shortId(prompt.Session), cReset)

	for {
		if prompt.Error != "" {
			fmt.Printf("%s  %s%s\n", cRed, prompt.Error, cReset)
		}
		if prompt.Done {
			break
		}
		ans, quit := self.ask(&prompt)
		if quit {
			fmt.Println("\nAbandoned. Nothing was written.")
			return
		}
		ans.Session = prompt.Session
		ans.Step = prompt.Step
		self.remember(prompt.Step, ans)
		next, err := post[dnd.Answer, dnd.Prompt](core, "dnd/answer", ans)
		if err != nil {
			fmt.Printf("%sserver error: %s%s\n", cRed, err, cReset)
			return
		}
		if next.Session == "" && next.Error == "" {
			fmt.Printf("%sthe session was lost, aborting%s\n", cRed, cReset)
			return
		}
		next.Session = prompt.Session
		prompt = next
	}

	// finished: show the sheet then offer to save it
	if prompt.Sheet != nil {
		fmt.Println()
		fmt.Print(dnd.TerminalSheet(prompt.Sheet))
	}
	for _, a := range prompt.Advice {
		fmt.Printf("%s %s%s\n", cDim, a, cReset)
	}

	filename := self.File
	if filename == "" {
		filename = prompt.Filename
	}
	if filename == "" && prompt.Sheet != nil {
		filename = dnd.Slugify(prompt.Sheet.Name) + ".org"
	}
	save := true
	survey.AskOne(&survey.Confirm{
		Message: "Save this character as an org file?", Default: true,
	}, &save, nil)
	if !save {
		fmt.Println("Not saved. The org text was:")
		fmt.Println(prompt.Org)
		return
	}
	survey.AskOne(&survey.Input{
		Message: "Filename:", Default: filename,
		Help: "Relative names are created inside your first org directory.",
	}, &filename, nil)

	res, err := post[dnd.SaveRequest, dnd.SaveResponse](core,
		"dnd/save", &dnd.SaveRequest{Session: prompt.Session, Filename: filename, Overwrite: self.Force})
	if err != nil {
		fmt.Printf("%ssave failed: %s%s\n", cRed, err, cReset)
		return
	}
	if !res.Ok {
		fmt.Printf("%s%s%s\n", cRed, orDefault(res.Msg, "save failed"), cReset)
		overwrite := false
		survey.AskOne(&survey.Confirm{Message: "Overwrite it?", Default: false}, &overwrite, nil)
		if !overwrite {
			return
		}
		res, err = post[dnd.SaveRequest, dnd.SaveResponse](core, "dnd/save",
			&dnd.SaveRequest{Session: prompt.Session, Filename: filename, Overwrite: true})
		if err != nil || !res.Ok {
			fmt.Printf("%ssave failed: %v %s%s\n", cRed, err, res.Msg, cReset)
			return
		}
	}
	fmt.Printf("\n%s%s written%s\n", cBold, res.Filename, cReset)
	fmt.Printf("%s  orgs dnd sheet -file %s -format pdf -out sheet.pdf%s\n",
		cDim, res.Filename, cReset)
	if self.Open {
		core.LaunchEditor(res.Filename, 0)
	}
}

// remember stores the choices made for a step so that they can be pre-selected
// if we end up asking the same question again.
func (self *Dnd) remember(step string, a *dnd.Answer) {
	if step == "" || a == nil || len(a.Values) == 0 {
		return
	}
	if self.lastPicks == nil {
		self.lastPicks = map[string][]string{}
	}
	self.lastPicks[step] = a.Values
}

// previous is what was chosen for this step last time round, if anything.
func (self *Dnd) previous(p *dnd.Prompt) []string {
	if len(p.Defaults) > 0 {
		return p.Defaults
	}
	return self.lastPicks[p.Step]
}

// ask turns one prompt into an answer, returning quit=true if the player bails.
func (self *Dnd) ask(p *dnd.Prompt) (*dnd.Answer, bool) {
	self.header(p)
	switch p.Kind {
	case "select":
		return self.askSelect(p)
	case "multiselect":
		return self.askMulti(p)
	case "number":
		return self.askNumber(p)
	case "abilities":
		return self.askAbilities(p)
	case "fields":
		return self.askFields(p)
	case "longtext":
		return self.askLongText(p)
	default: // text
		return self.askText(p)
	}
}

func (self *Dnd) header(p *dnd.Prompt) {
	fmt.Println()
	if p.Progress.Total > 0 {
		fmt.Printf("%s[%d/%d] %s%s%s%s\n", cDim, p.Progress.Step, p.Progress.Total,
			cReset+cBold, strings.ToUpper(p.Title), cReset, "")
	} else if p.Title != "" {
		fmt.Printf("%s%s%s\n", cBold, strings.ToUpper(p.Title), cReset)
	}
	if p.Summary != "" {
		fmt.Printf("%s%s%s\n", cDim, p.Summary, cReset)
	}
	for _, a := range p.Advice {
		fmt.Printf("%s  %s %s%s\n", cCyan, "→", a, cReset)
	}
	if p.Help != "" {
		fmt.Printf("%s  %s%s\n", cDim, p.Help, cReset)
	}
}

// label renders one option the way a character builder does.
func label(o dnd.Option) string {
	star := "  "
	if o.Recommended {
		star = "★ "
	}
	line := star + o.Name
	if o.Summary != "" {
		line += "  —  " + oneLine(o.Summary)
	}
	if len(line) > 150 {
		line = line[:147] + "..."
	}
	return line
}

func (self *Dnd) menu(p *dnd.Prompt, extra ...string) ([]string, map[string]dnd.Option) {
	opts := []string{}
	byLabel := map[string]dnd.Option{}
	for _, o := range p.Options {
		if o.Disabled {
			continue
		}
		l := label(o)
		for {
			if _, dup := byLabel[l]; !dup {
				break
			}
			l += " " // two options that render identically, keep them distinct
		}
		byLabel[l] = o
		opts = append(opts, l)
	}
	opts = append(opts, extra...)
	if len(p.Options) > 0 {
		opts = append(opts, entryDetails)
	}
	if p.AllowRandom {
		opts = append(opts, entryRandom)
	}
	if p.AllowSkip {
		opts = append(opts, entrySkip)
	}
	opts = append(opts, entryBack, entryQuit)
	return opts, byLabel
}

func (self *Dnd) showDetails(p *dnd.Prompt) {
	fmt.Println()
	for _, o := range p.Options {
		star := ""
		if o.Recommended {
			star = cGold + "  ★ recommended: " + o.Reason + cReset
		}
		fmt.Printf("%s%s%s%s\n", cBold, o.Name, cReset, star)
		if o.Summary != "" {
			fmt.Printf("  %s\n", oneLine(o.Summary))
		}
		if len(o.Meta) > 0 {
			keys := []string{}
			for k := range o.Meta {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			bits := []string{}
			for _, k := range keys {
				if o.Meta[k] != "" {
					bits = append(bits, fmt.Sprintf("%s %s", k, o.Meta[k]))
				}
			}
			if len(bits) > 0 {
				fmt.Printf("  %s%s%s\n", cDim, strings.Join(bits, " · "), cReset)
			}
		}
		if o.Detail != "" {
			fmt.Printf("%s%s%s\n", cDim, indent(wrap(o.Detail, 92), "    "), cReset)
		}
		fmt.Println()
	}
}

func (self *Dnd) askSelect(p *dnd.Prompt) (*dnd.Answer, bool) {
	for {
		opts, byLabel := self.menu(p)
		choice := ""
		def := ""
		prev := self.previous(p)
		for _, o := range p.Options {
			if o.Id == p.Default || (p.Default == "" && len(prev) > 0 && o.Id == prev[0]) {
				def = label(o)
			}
		}
		err := paneAsk(&survey.Select{
			Message: p.Question, Options: opts, Default: def, Help: p.Help,
			PageSize: panePageSize(),
		}, &choice, &paneCtx{byLabel: byLabel})
		if err != nil {
			return nil, true
		}
		switch choice {
		case entryDetails:
			self.showDetails(p)
			continue
		case entryRandom:
			return &dnd.Answer{Random: true}, false
		case entrySkip:
			return &dnd.Answer{Skip: true}, false
		case entryBack:
			return &dnd.Answer{Back: true}, false
		case entryQuit:
			return nil, true
		}
		o := byLabel[choice]
		return &dnd.Answer{Values: []string{o.Id}, Text: o.Name}, false
	}
}

func (self *Dnd) askMulti(p *dnd.Prompt) (*dnd.Answer, bool) {
	wantMin, wantMax := p.Min, p.Max
	if wantMin <= 0 && wantMax > 0 {
		wantMin = wantMax
	}
	opts, byLabel := multiLabels(p.Options)
	// whatever was picked last time (or pre-selected by the server) starts ticked
	prev := self.previous(p)
	picked := []string{}
	for _, l := range opts {
		if hasStr(prev, byLabel[l].Id) {
			picked = append(picked, l)
		}
	}
	for {
		sel := []string{}
		// Default carries the previous attempt back into the list, so a wrong
		// count is corrected rather than started over from nothing.
		if err := paneAsk(&survey.MultiSelect{
			Message: p.Question, Options: opts, Default: picked,
			Help: p.Help, PageSize: panePageSize(),
		}, &sel, &paneCtx{byLabel: byLabel, multi: true, min: wantMin, max: wantMax}); err != nil {
			return nil, true
		}
		picked = sel
		if countOk(len(picked), wantMin, wantMax) {
			vals := []string{}
			for _, l := range picked {
				vals = append(vals, byLabel[l].Id)
			}
			return &dnd.Answer{Values: vals}, false
		}
		if len(picked) > 0 {
			fmt.Printf("%s  %s, you picked %d. They are still selected, adjust them.%s\n",
				cRed, needText(wantMin, wantMax), len(picked), cReset)
			continue
		}
		// Nothing selected: offer the ways out rather than asking again.
		menu := []string{entryAgain}
		if len(p.Options) > 0 {
			menu = append(menu, entryDetails)
		}
		if p.AllowRandom {
			menu = append(menu, entryRandom)
		}
		if p.AllowSkip {
			menu = append(menu, entrySkip)
		}
		menu = append(menu, entryBack, entryQuit)
		choice := ""
		if err := paneAsk(&survey.Select{
			Message: "You selected nothing. What would you like to do?",
			Options: menu, PageSize: 8,
		}, &choice, &paneCtx{}); err != nil {
			return nil, true
		}
		switch choice {
		case entryDetails:
			self.showDetails(p)
		case entryRandom:
			return &dnd.Answer{Random: true}, false
		case entrySkip:
			return &dnd.Answer{Skip: true}, false
		case entryBack:
			return &dnd.Answer{Back: true}, false
		case entryQuit:
			return nil, true
		}
	}
}

func multiLabels(options []dnd.Option) ([]string, map[string]dnd.Option) {
	opts := []string{}
	byLabel := map[string]dnd.Option{}
	for _, o := range options {
		l := label(o)
		for {
			if _, dup := byLabel[l]; !dup {
				break
			}
			l += " " // two options that render identically, keep them distinct
		}
		byLabel[l] = o
		opts = append(opts, l)
	}
	return opts, byLabel
}

func (self *Dnd) askText(p *dnd.Prompt) (*dnd.Answer, bool) {
	if len(p.Options) > 0 {
		opts, byLabel := self.menu(p, entryCustom)
		choice := ""
		if err := paneAsk(&survey.Select{
			Message: p.Question, Options: opts, Help: p.Help, PageSize: panePageSize(),
		}, &choice, &paneCtx{byLabel: byLabel}); err != nil {
			return nil, true
		}
		switch choice {
		case entryDetails:
			self.showDetails(p)
			return self.askText(p)
		case entryRandom:
			return &dnd.Answer{Random: true}, false
		case entrySkip:
			return &dnd.Answer{Skip: true}, false
		case entryBack:
			return &dnd.Answer{Back: true}, false
		case entryQuit:
			return nil, true
		case entryCustom:
			// fall through to free text
		default:
			o := byLabel[choice]
			return &dnd.Answer{Values: []string{o.Id}, Text: o.Name}, false
		}
	}
	text := ""
	if err := survey.AskOne(&survey.Input{
		Message: p.Question, Default: p.Default, Help: p.Help,
	}, &text, nil); err != nil {
		return nil, true
	}
	text = strings.TrimSpace(text)
	if text == "" && p.AllowSkip {
		return &dnd.Answer{Skip: true}, false
	}
	return &dnd.Answer{Text: text}, false
}

func (self *Dnd) askLongText(p *dnd.Prompt) (*dnd.Answer, bool) {
	write := false
	survey.AskOne(&survey.Confirm{Message: p.Question + " (opens your editor)",
		Default: false, Help: p.Help}, &write, nil)
	if !write {
		return &dnd.Answer{Skip: true}, false
	}
	text := ""
	if err := survey.AskOne(&survey.Editor{Message: p.Title, Help: p.Help}, &text, nil); err != nil {
		return &dnd.Answer{Skip: true}, false
	}
	return &dnd.Answer{Text: strings.TrimSpace(text)}, false
}

func (self *Dnd) askNumber(p *dnd.Prompt) (*dnd.Answer, bool) {
	for {
		text := ""
		if err := survey.AskOne(&survey.Input{
			Message: p.Question, Default: p.Default, Help: p.Help,
		}, &text, nil); err != nil {
			return nil, true
		}
		text = strings.TrimSpace(text)
		if text == "back" {
			return &dnd.Answer{Back: true}, false
		}
		v, err := strconv.Atoi(text)
		if err != nil {
			fmt.Printf("%s  that is not a number (type back to go back)%s\n", cRed, cReset)
			continue
		}
		if p.Min > 0 && v < p.Min || p.Max > 0 && v > p.Max {
			fmt.Printf("%s  pick a number between %d and %d%s\n", cRed, p.Min, p.Max, cReset)
			continue
		}
		return &dnd.Answer{Text: text, Numbers: map[string]int{p.Step: v}}, false
	}
}

// askAbilities handles the three ways of setting ability scores.
func (self *Dnd) askAbilities(p *dnd.Prompt) (*dnd.Answer, bool) {
	if len(p.Pool) > 0 {
		return self.assignPool(p)
	}
	return self.typeScores(p)
}

func (self *Dnd) assignPool(p *dnd.Prompt) (*dnd.Answer, bool) {
	pool := append([]int{}, p.Pool...)
	sort.Sort(sort.Reverse(sort.IntSlice(pool)))
	fmt.Printf("%s  values to assign: %v%s\n", cDim, pool, cReset)
	nums := map[string]int{}
	remaining := []dnd.Field{}
	remaining = append(remaining, p.Fields...)
	for _, v := range pool {
		if len(remaining) == 0 {
			break
		}
		opts := []string{}
		byLabel := map[string]dnd.Field{}
		for _, f := range remaining {
			l := fmt.Sprintf("%-14s", f.Name)
			if f.Bonus != 0 {
				l += fmt.Sprintf("  racial %s%d", plus(f.Bonus), abs(f.Bonus))
			}
			if f.Hint != "" {
				l += "   " + cDim + f.Hint + cReset
			}
			byLabel[l] = f
			opts = append(opts, l)
		}
		opts = append(opts, entryBack, entryQuit)
		choice := ""
		if err := survey.AskOne(&survey.Select{
			Message: fmt.Sprintf("Assign %d to which ability?", v), Options: opts, PageSize: 10,
		}, &choice, nil); err != nil {
			return nil, true
		}
		if choice == entryBack {
			return &dnd.Answer{Back: true}, false
		}
		if choice == entryQuit {
			return nil, true
		}
		f := byLabel[choice]
		nums[f.Id] = v
		next := []dnd.Field{}
		for _, r := range remaining {
			if r.Id != f.Id {
				next = append(next, r)
			}
		}
		remaining = next
	}
	self.previewScores(p, nums)
	return &dnd.Answer{Numbers: nums}, false
}

func (self *Dnd) typeScores(p *dnd.Prompt) (*dnd.Answer, bool) {
	nums := map[string]int{}
	for _, f := range p.Fields {
		for {
			msg := f.Name
			if f.Bonus != 0 {
				msg += fmt.Sprintf(" (racial %s%d)", plus(f.Bonus), abs(f.Bonus))
			}
			if f.Hint != "" {
				msg += " - " + f.Hint
			}
			text := ""
			def := ""
			if f.Value > 0 {
				def = strconv.Itoa(f.Value)
			}
			if err := survey.AskOne(&survey.Input{Message: msg + ":", Default: def}, &text, nil); err != nil {
				return nil, true
			}
			v, err := strconv.Atoi(strings.TrimSpace(text))
			if err != nil {
				fmt.Printf("%s  that is not a number%s\n", cRed, cReset)
				continue
			}
			if f.Min > 0 && v < f.Min || f.Max > 0 && v > f.Max {
				fmt.Printf("%s  %s must be between %d and %d%s\n", cRed, f.Name, f.Min, f.Max, cReset)
				continue
			}
			nums[f.Id] = v
			break
		}
	}
	self.previewScores(p, nums)
	return &dnd.Answer{Numbers: nums}, false
}

func (self *Dnd) previewScores(p *dnd.Prompt, nums map[string]int) {
	parts := []string{}
	for _, f := range p.Fields {
		total := nums[f.Id] + f.Bonus
		mod := (total - 10) / 2
		if total < 10 && (total-10)%2 != 0 {
			mod--
		}
		parts = append(parts, fmt.Sprintf("%s %d(%s%d)", strings.ToUpper(f.Id[:3]), total, plus(mod), abs(mod)))
	}
	fmt.Printf("%s  final scores: %s%s\n", cDim, strings.Join(parts, "  "), cReset)
}

// what asking for one field ended in
const (
	fieldOk = iota
	fieldBack
	fieldQuit
)

// askFields walks a multi line prompt - the appearance step - one field at a
// time. Each field carries its own suggestions and its own bounds, so the
// answers are offered rather than demanded, and checked before they are sent.
func (self *Dnd) askFields(p *dnd.Prompt) (*dnd.Answer, bool) {
	menu := []string{entryFill}
	if p.AllowRandom {
		menu = append(menu, entryRandom)
	}
	if p.AllowSkip {
		menu = append(menu, entrySkip)
	}
	menu = append(menu, entryBack, entryQuit)
	choice := ""
	if err := paneAsk(&survey.Select{
		Message: p.Question, Options: menu, Help: p.Help, PageSize: 8,
	}, &choice, &paneCtx{}); err != nil {
		return nil, true
	}
	switch choice {
	case entryRandom:
		return &dnd.Answer{Random: true}, false
	case entrySkip:
		return &dnd.Answer{Skip: true}, false
	case entryBack:
		return &dnd.Answer{Back: true}, false
	case entryQuit:
		return nil, true
	}

	values := make([]string, len(p.Fields))
	for i := 0; i < len(p.Fields); {
		// Stepping back onto a field brings the answer you already gave with
		// you, so a correction is a correction rather than a retype.
		f := p.Fields[i]
		f.Text = values[i]
		v, what := self.askField(f)
		switch what {
		case fieldQuit:
			return nil, true
		case fieldBack:
			if i == 0 {
				// backing out of the first field leaves the step entirely
				return &dnd.Answer{Back: true}, false
			}
			i--
			continue
		}
		values[i] = v
		i++
	}
	return &dnd.Answer{Values: values}, false
}

// askField asks for one line of a fields prompt: pick a suggestion, write your
// own, roll for it, or leave it blank.
func (self *Dnd) askField(f dnd.Field) (string, int) {
	message := f.Name + "?"
	if f.Hint != "" {
		message += "  (" + f.Hint + ")"
	}
	for {
		if len(f.Options) == 0 {
			return self.askFieldText(f, message)
		}
		opts, byLabel := fieldLabels(f)
		if f.AllowCustom {
			opts = append(opts, entryCustom)
		}
		opts = append(opts, entryRandom, entryBlank, entryBack, entryQuit)
		choice := ""
		if err := paneAsk(&survey.Select{
			Message: message, Options: opts, Default: labelOf(byLabel, f.Text),
			Help: f.Hint, PageSize: panePageSize(),
		}, &choice, &paneCtx{byLabel: byLabel}); err != nil {
			return "", fieldQuit
		}
		switch choice {
		case entryRandom:
			return dnd.RandomAppearanceValue(f), fieldOk
		case entryBlank:
			return "", fieldOk
		case entryBack:
			return "", fieldBack
		case entryQuit:
			return "", fieldQuit
		case entryCustom:
			return self.askFieldText(f, message)
		}
		v, err := dnd.ValidateField(f, byLabel[choice].Name)
		if err != nil {
			// A suggestion that does not validate means the tables disagree
			// with the bounds, which is a bug rather than a bad answer.
			fmt.Printf("%s  %s%s\n", cRed, err, cReset)
			continue
		}
		return v, fieldOk
	}
}

// askFieldText takes a typed value, checking it before accepting it. An empty
// line leaves the field out.
func (self *Dnd) askFieldText(f dnd.Field, message string) (string, int) {
	for {
		text := ""
		if err := survey.AskOne(&survey.Input{
			Message: message, Default: f.Text, Help: f.Hint,
		}, &text, nil); err != nil {
			return "", fieldQuit
		}
		v, err := dnd.ValidateField(f, text)
		if err != nil {
			fmt.Printf("%s  %s%s\n", cRed, err, cReset)
			continue
		}
		return v, fieldOk
	}
}

// fieldLabels renders the suggestions of one field as menu entries.
func fieldLabels(f dnd.Field) ([]string, map[string]dnd.Option) {
	opts := []string{}
	byLabel := map[string]dnd.Option{}
	for _, o := range f.Options {
		l := label(o)
		for {
			if _, dup := byLabel[l]; !dup {
				break
			}
			l += " " // two suggestions that render identically, keep them distinct
		}
		byLabel[l] = o
		opts = append(opts, l)
	}
	return opts, byLabel
}

// labelOf finds the menu entry for a value, so a field that already has one
// starts with it selected.
func labelOf(byLabel map[string]dnd.Option, value string) string {
	if value == "" {
		return ""
	}
	for l, o := range byLabel {
		if strings.EqualFold(o.Name, value) || strings.EqualFold(o.Id, value) {
			return l
		}
	}
	return ""
}

// ----------------------------------------------------------------------------
// Non interactive subcommands
// ----------------------------------------------------------------------------

func (self *Dnd) runRandom(core *commands.Core) {
	req := dnd.NewSessionRequest{Ruleset: self.Ruleset, Name: self.Name,
		Player: self.Player, Level: self.Level}
	res, err := post[dnd.NewSessionRequest, struct {
		Ok        bool           `json:"ok"`
		Character *dnd.Character `json:"character"`
		Sheet     *dnd.Sheet     `json:"sheet"`
		Org       string         `json:"org"`
		Msg       string         `json:"msg"`
	}](core, "dnd/random", &req)
	if err != nil {
		fmt.Printf("%s%s%s\n", cRed, err, cReset)
		return
	}
	if res.Sheet == nil {
		fmt.Printf("%sno character was generated%s %s\n", cRed, cReset, res.Msg)
		return
	}
	fmt.Print(dnd.TerminalSheet(res.Sheet))
	out := self.Out
	if out == "" {
		out = self.File
	}
	if out == "" {
		return
	}
	save, err := post[dnd.SaveRequest, dnd.SaveResponse](core, "dnd/save",
		&dnd.SaveRequest{Character: res.Character, Filename: out, Overwrite: self.Force})
	if err != nil || !save.Ok {
		fmt.Printf("%ssave failed: %v %s%s\n", cRed, err, save.Msg, cReset)
		return
	}
	fmt.Printf("%s%s written%s\n", cBold, save.Filename, cReset)
	if self.Open {
		core.LaunchEditor(save.Filename, 0)
	}
}

func (self *Dnd) runList(core *commands.Core) {
	kind := self.Kind
	if kind == "" {
		kind = "rulesets"
	}
	res := get[dnd.CatalogResponse](core, "dnd/catalog", map[string]string{
		"kind": kind, "ruleset": self.Ruleset, "id": self.Id, "filter": self.Filter,
	})
	if len(res.Options) == 0 {
		fmt.Printf("nothing found for kind %q\n", kind)
		return
	}
	fmt.Printf("%s%s in %s%s\n\n", cBold, strings.ToUpper(res.Kind), orDefault(res.Ruleset, "srd"), cReset)
	for _, o := range res.Options {
		fmt.Printf("  %s%-26s%s %s\n", cBold, o.Id, cReset, oneLine(o.Summary))
	}
	fmt.Printf("\n%s%d entries. orgs dnd info -kind %s -id <id> for the full text.%s\n",
		cDim, len(res.Options), kind, cReset)
}

func (self *Dnd) runInfo(core *commands.Core) {
	if self.Kind == "" || self.Id == "" {
		fmt.Println("usage: orgs dnd info -kind <races|classes|spells|...> -id <id>")
		return
	}
	res := get[dnd.CatalogResponse](core, "dnd/catalog", map[string]string{
		"kind": self.Kind, "ruleset": self.Ruleset, "id": self.Id, "filter": self.Filter,
	})
	found := false
	for _, o := range res.Options {
		if o.Id != self.Id && !strings.EqualFold(o.Name, self.Id) {
			continue
		}
		found = true
		fmt.Printf("\n%s%s%s\n", cBold, o.Name, cReset)
		if o.Summary != "" {
			fmt.Printf("%s\n", oneLine(o.Summary))
		}
		if len(o.Meta) > 0 {
			keys := []string{}
			for k := range o.Meta {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if o.Meta[k] != "" {
					fmt.Printf("  %s%-12s%s %s\n", cDim, k, cReset, o.Meta[k])
				}
			}
		}
		if o.Detail != "" {
			fmt.Printf("\n%s\n", wrap(o.Detail, 92))
		}
		fmt.Println()
	}
	if !found {
		fmt.Printf("no %s called %q, try: orgs dnd list %s\n", self.Kind, self.Id, self.Kind)
	}
}

func (self *Dnd) runCharacters(core *commands.Core) {
	list := get[[]map[string]interface{}](core, "dnd/characters", map[string]string{})
	if len(list) == 0 {
		fmt.Println("No character sheets found. Create one with: orgs dnd new")
		return
	}
	fmt.Printf("%s%-22s %-26s %-28s %s%s\n", cBold, "NAME", "RACE", "CLASS", "FILE", cReset)
	for _, c := range list {
		fmt.Printf("%-22v %-26v %-28v %v\n", c["name"], c["race"], c["class"], c["filename"])
	}
}

func (self *Dnd) runShow(core *commands.Core) {
	if self.File == "" {
		fmt.Println("usage: orgs dnd show -file <sheet.org>")
		return
	}
	sheet := get[dnd.Sheet](core, "dnd/sheet", map[string]string{"filename": self.File})
	if sheet.Name == "" {
		fmt.Printf("%scould not read a character out of %s%s\n", cRed, self.File, cReset)
		return
	}
	fmt.Print(dnd.TerminalSheet(&sheet))
}

func (self *Dnd) runSheet(core *commands.Core) {
	if self.File == "" {
		fmt.Println("usage: orgs dnd sheet -file <sheet.org> [-format html|latex|pdf] [-out file]")
		return
	}
	exporter := map[string]string{
		"html": "dndsheet", "latex": "dndlatex", "tex": "dndlatex", "pdf": "dndpdf",
	}[strings.ToLower(self.Format)]
	if exporter == "" {
		fmt.Printf("unknown format %q, use html, latex or pdf\n", self.Format)
		return
	}
	out := self.Out
	if out == "" {
		base := strings.TrimSuffix(self.File, ".org")
		ext := map[string]string{"dndsheet": ".html", "dndlatex": ".tex", "dndpdf": ".pdf"}[exporter]
		out = base + ext
	}
	local := "t"
	if !self.Local {
		local = "f"
	}
	res := get[common.ResultMsg](core, fmt.Sprintf("file/%s", exporter), map[string]string{
		"filename": out, "query": self.File, "local": local,
	})
	if !res.Ok {
		fmt.Printf("%sexport failed: %s%s\n", cRed, res.Msg, cReset)
		// The server only reports "setup in the config file" when the exporter is
		// genuinely missing. Every other failure already explains itself, so do not
		// send people off to check a config that is fine.
		if strings.Contains(res.Msg, "setup in the config file") {
			fmt.Printf("%sadd \"%s\" to server.exporters in your server config%s\n", cDim, exporter, cReset)
		}
		return
	}
	if !self.Local {
		fmt.Println(res.Msg)
		return
	}
	fmt.Printf("%s%s written%s\n", cBold, out, cReset)
}

func (self *Dnd) runRefresh(core *commands.Core) {
	if self.File == "" {
		fmt.Println("usage: orgs dnd refresh -file <sheet.org>")
		return
	}
	res, err := post[dnd.SaveRequest, dnd.SaveResponse](core, "dnd/refresh",
		&dnd.SaveRequest{Filename: self.File})
	if err != nil || !res.Ok {
		fmt.Printf("%srefresh failed: %v %s%s\n", cRed, err, res.Msg, cReset)
		return
	}
	fmt.Printf("%s%s refreshed%s\n", cBold, res.Filename, cReset)
}

func (self *Dnd) runRulesets(core *commands.Core) {
	list := get[[]dnd.RulesetInfo](core, "dnd/rulesets", map[string]string{})
	if len(list) == 0 {
		fmt.Println("no rulesets loaded")
		return
	}
	for _, r := range list {
		fmt.Printf("\n%s%s%s  %s\n", cBold, r.Id, cReset, r.Name)
		if r.Description != "" {
			fmt.Printf("  %s\n", oneLine(r.Description))
		}
		if r.Extends != "" {
			fmt.Printf("  %sextends %s%s\n", cDim, r.Extends, cReset)
		}
		fmt.Printf("  %sraces %d · classes %d · backgrounds %d · spells %d · items %d · feats %d%s\n",
			cDim, r.Races, r.Classes, r.Backgrounds, r.Spells, r.Items, r.Feats, cReset)
		for _, m := range r.Modules {
			fmt.Printf("    %s%s%s\n", cDim, m, cReset)
		}
	}
	fmt.Println()
}

func (self *Dnd) runReload(core *commands.Core) {
	list, err := post[struct{}, []dnd.RulesetInfo](core, "dnd/reload", &struct{}{})
	if err != nil {
		fmt.Printf("%sreload failed: %s%s\n", cRed, err, cReset)
		return
	}
	fmt.Printf("reloaded, %d rulesets available\n", len(list))
	self.runRulesets(core)
}

// ----------------------------------------------------------------------------
// small helpers
// ----------------------------------------------------------------------------

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		s = s[:117] + "..."
	}
	return s
}

func wrap(text string, width int) string {
	out := []string{}
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, w := range words {
			if line == "" {
				line = w
			} else if len(line)+1+len(w) <= width {
				line += " " + w
			} else {
				out = append(out, line)
				line = w
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func indent(text, pad string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func shortId(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func plus(v int) string {
	if v < 0 {
		return "-"
	}
	return "+"
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// init function is called at boot
func init() {
	commands.AddCmd("dnd", "create and render dungeons & dragons character sheets",
		func() commands.Cmd {
			return &Dnd{Format: "html", Local: true}
		})
}
