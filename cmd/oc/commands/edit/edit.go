package edit

// The verbs that change a heading.
//
//	orgs todo DONE 'IsStatus("NEXT")'   the keyword
//	orgs sched tomorrow                 SCHEDULED
//	orgs deadline fri                   DEADLINE
//	orgs tag +work -someday             the tags
//	orgs prop EFFORT=2h                 the property drawer
//	orgs rename 'a better title'        the headline text
//	orgs note 'rang back, no answer'    a line onto the body
//	orgs check 2                        a checkbox
//	orgs archive                        to the archive file
//	orgs rm                             gone
//
// Every one of these endpoints already existed and every one was already
// reachable - from `orgs mcp`, and from inside the `tui` and `agenda` screens.
// None of them was reachable from a prompt, which meant an agent had better
// write access to somebody's org files than they did, and that anything a
// person wanted to do to fifty headings they did by hand.
//
// They are one package rather than ten because what makes them worth having is
// the part they share: `commands.Resolve` decides which heading, identically for
// all of them, so a query that names the right headings for `orgs todo` names
// them for `orgs rm` too. Ten packages would be ten chances for that to drift.
//
// Two things every verb here does, and neither is optional:
//
//  1. **It says what it wrote to.** A write with no feedback cannot be told from
//     one that silently landed on nothing.
//  2. **It goes through SendReceivePost**, which is where -dry-run is answered.
//     A verb cannot forget to honour it, because it never sees the flag.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// orgs todo - the keyword
// ---------------------------------------------------------------------------

type Todo struct {
	tf    commands.TargetFlags
	Cycle bool
	Done  bool
	Note  string
}

func (self *Todo) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Todo) StartPlugin(m *common.PluginManager)       {}

func (self *Todo) SetupParameters(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
	fset.BoolVar(&self.Cycle, "cycle", false,
		"move to the next keyword in this heading's own sequence")
	fset.BoolVar(&self.Done, "done", false, "the first finished keyword this heading's file has")
	// A note to keep with the change, for the keywords whose `@` cookie asks for
	// one. The server cannot prompt for it, so this is the only way one reaches
	// the file from a terminal.
	fset.StringVar(&self.Note, "note", "", "a note to keep with the state change")
}

func (self *Todo) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("todo").Flags)

	// The keyword is the first word, when the first word looks like one. A
	// keyword is upper case with no punctuation in it and a query never is, so
	// the shape tells them apart without a flag - and `orgs todo 'IsTask()'`
	// still means "pick a keyword for one of these" rather than being read as
	// a keyword called IsTask().
	keyword := ""
	if len(words) > 0 && looksLikeKeyword(words[0]) {
		keyword = strings.ToUpper(words[0])
		words = words[1:]
	}

	todos := commands.Resolve(core, &self.tf, words, commands.TargetOpts{
		Prompt: "Change keyword> ",
		Query:  "IsTodo()",
		Multi:  true,
	})

	lines := []string{}
	for _, t := range todos {
		// The keywords a heading may take are its own file's, which may declare
		// its own #+TODO line. Asking per heading rather than once is the same
		// rule worg's kanban and the tui both follow: offering a keyword the
		// file does not have is offering to write something org will not read
		// back.
		states := validStates(core, t.Hash)
		want := keyword
		switch {
		case self.Done:
			if len(states.Done) == 0 {
				commands.Fail("%s has no finished keyword to use", commands.BaseName(t.Filename))
			}
			want = states.Done[0]
		case self.Cycle:
			want = nextInSequence(states, t.Status)
		case want == "":
			want = askKeyword(t, states)
		}
		if want != "" && !hasState(states, want) {
			commands.Fail("%s does not take the keyword %q - it has %s",
				commands.BaseName(t.Filename), want, strings.Join(allStates(states), ", "))
		}

		var reply common.Result
		commands.SendReceivePost(core, "status/change",
			&common.TodoItemChange{Hash: t.Hash, Value: want, Note: self.Note}, &reply)
		if commands.DryRun {
			continue
		}
		if !reply.Ok {
			commands.Fail("could not set %s on %s", want, t.Headline)
		}
		shown := want
		if shown == "" {
			shown = "(no keyword)"
		}
		line := fmt.Sprintf("%s → %s", commands.Describe(t), keywordInk(shown))
		// A repeating heading asked for DONE comes back on a live keyword, which
		// looks like the write having failed unless it is said out loud.
		if reply.Msg != "" {
			line += " (" + reply.Msg + ")"
		}
		lines = append(lines, line)
	}
	commands.Ok(len(lines), "changed", lines)
}

// A keyword as org writes one: upper case letters, and the handful of other
// characters keywords use. Deliberately narrow - the question being answered is
// "is this word a keyword rather than a query", and a query always has a
// bracket, a quote or a brace in it.
var keywordRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

func looksLikeKeyword(w string) bool {
	if commands.LooksLikeHash(w) {
		return false
	}
	if !keywordRe.MatchString(w) {
		return false
	}
	// Upper case is what makes it a keyword rather than a word somebody meant
	// as a query over headlines. `orgs todo done` is common enough to allow,
	// so a single lower case word is taken as a keyword too - but only when it
	// is the whole of it.
	return w == strings.ToUpper(w) || !strings.ContainsAny(w, " ")
}

func validStates(core *commands.Core, hash string) common.TodoStatesResult {
	states, err := commands.SendReceiveGetErr[common.TodoStatesResult](core,
		"status/"+commands.HashPath(hash), nil)
	if err != nil || len(allStates(states)) == 0 {
		// Fall back to the server's own list. A file with no #+TODO line of its
		// own uses that, and an older server that does not answer the
		// per-heading question should not stop the write.
		states = commands.RestGetStates(core)
	}
	return states
}

func allStates(s common.TodoStatesResult) []string {
	return append(append([]string{}, s.Active...), s.Done...)
}

func hasState(s common.TodoStatesResult, want string) bool {
	if want == "" {
		return true // clearing the keyword is always allowed
	}
	for _, k := range allStates(s) {
		if strings.EqualFold(k, want) {
			return true
		}
	}
	return false
}

// nextInSequence is org's C-c C-t: round the file's own keywords, and off the
// end back to no keyword at all - which is the state org cycles to and the one
// a tool that stopped at DONE would never let you back out of.
func nextInSequence(s common.TodoStatesResult, current string) string {
	seq := allStates(s)
	if len(seq) == 0 {
		return ""
	}
	for i, k := range seq {
		if strings.EqualFold(k, current) {
			if i+1 < len(seq) {
				return seq[i+1]
			}
			return ""
		}
	}
	return seq[0]
}

// askKeyword offers this heading's own keywords. It is a picker rather than a
// numbered prompt because the answer is one of a short known list, which is
// exactly what a picker is for.
func askKeyword(t common.Todo, s common.TodoStatesResult) string {
	choices := allStates(s)
	if !commands.Interactive() {
		commands.Fail("say which keyword: %s", strings.Join(choices, ", "))
	}
	lines := []string{}
	for _, k := range choices {
		mark := "  "
		if strings.EqualFold(k, t.Status) {
			mark = "• "
		}
		lines = append(lines, commands.PickLine([]string{k}, mark+keywordInk(k)))
	}
	// Clearing it is one of the answers, and a picker that cannot offer it is a
	// picker you have to leave to do the thing org does with one keystroke.
	lines = append(lines, commands.PickLine([]string{""},
		"  "+commands.C(commands.AnsiDim)+"(no keyword)"+commands.C(commands.AnsiReset)))
	sel := commands.Pick(commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "Keyword> ",
		Header:        t.Headline,
	})
	if len(sel) == 0 {
		commands.Fail("nothing chosen")
	}
	addr, _ := commands.Address(sel[0], 1)
	return addr[0]
}

func keywordInk(k string) string {
	ink := commands.AnsiGold
	switch strings.ToUpper(k) {
	case "DONE", "CANCELLED", "CANCELED", "CLOSED":
		ink = commands.AnsiGreen
	case "BLOCKED", "WAITING", "HOLD":
		ink = commands.AnsiRed
	}
	return commands.C(ink) + k + commands.C(commands.AnsiReset)
}

// ---------------------------------------------------------------------------
// orgs sched / orgs deadline - the dates
// ---------------------------------------------------------------------------

type Dated struct {
	tf commands.TargetFlags
	// Which date this verb is about: SCHEDULED, DEADLINE or CLOSED.
	Which    string
	Repeat   string
	Inactive bool
	cmdName  string
}

func (self *Dated) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Dated) StartPlugin(m *common.PluginManager)       {}

func (self *Dated) SetupParameters(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
	fset.StringVar(&self.Repeat, "repeat", "",
		"a repeater cookie: +1w every week, ++1w from today, .+1w from when it was done")
	fset.BoolVar(&self.Inactive, "inactive", false,
		"write [square brackets], which the agenda does not read")
}

func (self *Dated) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find(self.cmdName).Flags)

	// The date is the leading words, up to the first one that is a query or a
	// hash. "fri 14:00" is two words and one date, so they cannot simply be
	// counted off.
	dateWords, rest := splitDate(words)
	if len(dateWords) == 0 && len(rest) == 0 {
		commands.Fail("orgs %s: say when.\n\n%s", self.cmdName, dateUsage(self.cmdName))
	}

	value, err := commands.ParseDateToOrg(strings.Join(dateWords, " "), time.Now())
	if err != nil {
		commands.Fail("orgs %s: %v", self.cmdName, err)
	}
	// The repeater and the bracket style are flags as well as being writable
	// inside the date, because `-repeat` reads better on a line that already
	// says `tomorrow` than `tomorrow +1w` does.
	if value != "" && (self.Repeat != "" || self.Inactive) {
		d, _, derr := commands.ParseDate(strings.Join(dateWords, " "), time.Now())
		if derr == nil && d.Raw == "" {
			if self.Repeat != "" {
				d.Repeater = self.Repeat
			}
			d.Inactive = self.Inactive
			value = d.String()
		}
	}

	todos := commands.Resolve(core, &self.tf, rest, commands.TargetOpts{
		Prompt: self.Which + "> ",
		Query:  "IsTodo()",
		Multi:  true,
	})

	lines := []string{}
	for _, t := range todos {
		var reply common.Result
		commands.SendReceivePost(core, "date/change",
			&common.TodoDateChange{Hash: t.Hash, Name: self.Which, Value: value}, &reply)
		if commands.DryRun {
			continue
		}
		if !reply.Ok {
			commands.Fail("could not set %s on %s", self.Which, t.Headline)
		}
		shown := value
		if shown == "" {
			shown = commands.C(commands.AnsiDim) + "(cleared)" + commands.C(commands.AnsiReset)
		} else {
			shown = commands.C(commands.AnsiGold) + shown + commands.C(commands.AnsiReset)
		}
		lines = append(lines, fmt.Sprintf("%s %s %s", commands.Describe(t), self.Which, shown))
	}
	commands.Ok(len(lines), "dated", lines)
}

// splitDate takes the date words off the front. A word belongs to the date
// while it is not a hash and not a query - which is the same shape test the
// target resolver uses, asked the other way round.
func splitDate(words []string) (date []string, rest []string) {
	for i, w := range words {
		if w == "-" && i > 0 {
			// A bare dash after a date is stdin, not "clear".
			return words[:i], words[i:]
		}
		if commands.LooksLikeHash(w) || isQueryish(w) {
			return words[:i], words[i:]
		}
	}
	return words, nil
}

// isQueryish is "this word is part of a query expression rather than a value".
// A query in this language always has a bracket, a brace, a quote or an
// operator in it; a date, a tag and a keyword never do.
func isQueryish(w string) bool {
	return strings.ContainsAny(w, "()&|!\"'{}=<>") && !strings.HasPrefix(w, "<") &&
		!strings.HasPrefix(w, "[")
}

func dateUsage(name string) string {
	return fmt.Sprintf(`  orgs %s today                    a date, said the way people say one
  orgs %s tomorrow 14:00
  orgs %s fri                      the coming Friday
  orgs %s +2w                      two weeks out
  orgs %s mon -repeat +1w          every Monday
  orgs %s 2026-10-01 'IsTask()'    over everything a query finds
  orgs %s clear                    take it off`, name, name, name, name, name, name, name)
}

// ---------------------------------------------------------------------------
// orgs tag - the tags
// ---------------------------------------------------------------------------

type Tag struct {
	tf     commands.TargetFlags
	Toggle bool
	// Tags written with a minus in front of them, taken off the line before the
	// flag parse ever saw them. See commands.DashWords.
	dashed []string
}

// TakeDashWords is how `orgs tag -someday` reaches this command: the word is
// pulled out in main.go, because the flag package would otherwise call it an
// undefined flag and exit before Exec ran.
func (self *Tag) TakeDashWords(words []string) { self.dashed = words }

func (self *Tag) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Tag) StartPlugin(m *common.PluginManager)       {}

func (self *Tag) SetupParameters(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
	fset.BoolVar(&self.Toggle, "toggle", false,
		"flip each tag rather than reading + and - as add and remove")
}

// A tag as org allows one. No punctuation, which is what tells a tag from a
// query.
var tagRe = regexp.MustCompile(`^[+-]?[A-Za-z0-9_@#%]+$`)

func (self *Tag) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("tag").Flags)

	add, remove, flip, rest := splitTags(append(self.dashed, words...), self.Toggle)
	if len(add)+len(remove)+len(flip) == 0 {
		commands.Fail("orgs tag: say which tags.\n\n%s", tagUsage)
	}

	todos := commands.Resolve(core, &self.tf, rest, commands.TargetOpts{
		Prompt: "Tag> ",
		Multi:  true,
	})

	lines := []string{}
	for _, t := range todos {
		// The endpoint *toggles*, so doing what was asked means knowing what is
		// there already: `+work` on a heading that is already tagged work must
		// do nothing rather than take the tag off. Which is what makes this
		// safe to run over a query twice, and safe in a pipeline where nobody
		// is looking at the result.
		have := map[string]bool{}
		for _, tag := range t.Tags {
			have[strings.ToLower(tag)] = true
		}
		did := []string{}
		for _, tag := range add {
			if !have[strings.ToLower(tag)] {
				self.toggle(core, t, tag)
				did = append(did, "+"+tag)
			}
		}
		for _, tag := range remove {
			if have[strings.ToLower(tag)] {
				self.toggle(core, t, tag)
				did = append(did, "-"+tag)
			}
		}
		for _, tag := range flip {
			self.toggle(core, t, tag)
			if have[strings.ToLower(tag)] {
				did = append(did, "-"+tag)
			} else {
				did = append(did, "+"+tag)
			}
		}
		if commands.DryRun {
			continue
		}
		if len(did) == 0 {
			// Saying so beats silence: "nothing to do" and "it did not work"
			// look identical otherwise.
			lines = append(lines, fmt.Sprintf("%s %salready as asked%s", commands.Describe(t),
				commands.C(commands.AnsiDim), commands.C(commands.AnsiReset)))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s%s%s", commands.Describe(t),
			commands.C(commands.AnsiCyan), strings.Join(did, " "), commands.C(commands.AnsiReset)))
	}
	commands.Ok(len(lines), "tagged", lines)
}

func (self *Tag) toggle(core *commands.Core, t common.Todo, tag string) {
	var reply common.Result
	commands.SendReceivePost(core, "tags",
		&common.TodoItemChange{Hash: t.Hash, Value: tag}, &reply)
	if !commands.DryRun && !reply.Ok {
		commands.Fail("could not change tag %s on %s", tag, t.Headline)
	}
}

// splitTags reads the leading tag words. A `+` adds, a `-` removes, and a bare
// word adds - because "tag this work" is what somebody means by it, and -toggle
// is there for the other reading.
func splitTags(words []string, toggle bool) (add, remove, flip, rest []string) {
	for i, w := range words {
		if commands.LooksLikeHash(w) || !tagRe.MatchString(w) || w == "-" {
			return add, remove, flip, words[i:]
		}
		switch {
		case toggle:
			flip = append(flip, strings.TrimLeft(w, "+-"))
		case strings.HasPrefix(w, "-"):
			remove = append(remove, w[1:])
		default:
			add = append(add, strings.TrimPrefix(w, "+"))
		}
	}
	return add, remove, flip, nil
}

const tagUsage = `  orgs tag +work                   add a tag
  orgs tag -someday                take one off
  orgs tag +work -someday          both at once
  orgs tag work 'IsStatus("NEXT")' over everything a query finds
  orgs tag -toggle work            flip it, whichever way round it is`

// ---------------------------------------------------------------------------
// orgs prop - the property drawer
// ---------------------------------------------------------------------------

type Prop struct {
	tf commands.TargetFlags
}

func (self *Prop) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Prop) StartPlugin(m *common.PluginManager)       {}

func (self *Prop) SetupParameters(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
}

func (self *Prop) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("prop").Flags)
	if len(words) == 0 {
		commands.Fail("orgs prop: say which property.\n\n%s", propUsage)
	}

	// NAME=VALUE rather than two words, because a value has spaces in it far
	// more often than not and `orgs prop OWNER 'Jane Roe' 'IsTask()'` has no
	// way to say which word is the value and which the query.
	spec := words[0]
	rest := words[1:]
	name, value, assigning := strings.Cut(spec, "=")
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		commands.Fail("orgs prop: no property name in %q", spec)
	}
	if name == "RECORD" || name == "ADDED" {
		// The same two the records endpoint refuses, for the same reason: one
		// is a record's identity and the other is when it started.
		commands.Fail("%s is not editable through a form - edit the file if you mean it", name)
	}

	todos := commands.Resolve(core, &self.tf, rest, commands.TargetOpts{
		Prompt: name + "> ",
		Multi:  assigning,
	})

	// Without an `=` this is a read, which is the natural thing to want just
	// before writing one and does not deserve a command of its own.
	if !assigning {
		rows := []propRow{}
		for _, t := range todos {
			rows = append(rows, propRow{Hash: t.Hash, Headline: t.Headline,
				Filename: t.Filename, LineNum: t.LineNum, Name: name, Value: t.Props[name]})
		}
		commands.Render(rows, func() {
			for _, r := range rows {
				if r.Value == "" {
					fmt.Printf("%s %s(not set)%s\n", commands.Describe(common.Todo{
						Filename: r.Filename, LineNum: r.LineNum, Headline: r.Headline}),
						commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
					continue
				}
				fmt.Printf("%s %s%s%s\n", commands.Describe(common.Todo{
					Filename: r.Filename, LineNum: r.LineNum, Headline: r.Headline}),
					commands.C(commands.AnsiCyan), r.Value, commands.C(commands.AnsiReset))
			}
		})
		return
	}

	lines := []string{}
	for _, t := range todos {
		var reply common.Result
		commands.SendReceivePost(core, "property",
			&common.TodoPropertyChange{Hash: t.Hash, Name: name, Value: value}, &reply)
		if commands.DryRun {
			continue
		}
		if !reply.Ok {
			commands.Fail("could not set %s on %s", name, t.Headline)
		}
		// An empty value removes the property, and the drawer with it when it
		// was the last one. Said out loud, because "set to nothing" and
		// "removed" are different things to have done.
		shown := commands.C(commands.AnsiCyan) + value + commands.C(commands.AnsiReset)
		if value == "" {
			shown = commands.C(commands.AnsiDim) + "(removed)" + commands.C(commands.AnsiReset)
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", commands.Describe(t), name, shown))
	}
	commands.Ok(len(lines), "changed", lines)
}

type propRow struct {
	Hash     string
	Headline string
	Filename string
	LineNum  int
	Name     string
	Value    string
}

const propUsage = `  orgs prop EFFORT=2h              set it
  orgs prop OWNER='Jane Roe'       a value with a space in it
  orgs prop EFFORT=                take it off
  orgs prop EFFORT                 read it back
  orgs prop EFFORT=2h 'IsTask()'   over everything a query finds`

// ---------------------------------------------------------------------------
// orgs rename - the headline text
// ---------------------------------------------------------------------------

type Rename struct {
	tf commands.TargetFlags
}

func (self *Rename) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Rename) StartPlugin(m *common.PluginManager)       {}

func (self *Rename) SetupParameters(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
}

func (self *Rename) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("rename").Flags)
	if len(words) == 0 {
		commands.Fail("orgs rename: say what to call it.\n\n  orgs rename 'a better title' [query]")
	}
	title := words[0]
	rest := words[1:]

	// One at a time. The new text is the same for every heading it landed on,
	// so a rename over nine headings makes nine headings with one name - which
	// is never what anybody meant.
	todos := commands.Resolve(core, &self.tf, rest, commands.TargetOpts{
		Prompt: "Rename> ",
		Multi:  false,
	})
	t := todos[0]

	var reply common.Result
	commands.SendReceivePost(core, "headline/change",
		&common.TodoItemChange{Hash: t.Hash, Value: title}, &reply)
	if commands.DryRun {
		return
	}
	if !reply.Ok {
		commands.Fail("could not rename %s", t.Headline)
	}
	// A hash is a sha1 over the outline path, so renaming a heading changes its
	// hash and every one of its children's. Worth saying: a script holding the
	// old hash now holds nothing, and that is not obvious from the outside.
	commands.Ok(1, "renamed", []string{fmt.Sprintf("%s → %s%s%s  %s(its hash has changed)%s",
		commands.Describe(t), commands.C(commands.AnsiBold), title,
		commands.C(commands.AnsiReset), commands.C(commands.AnsiDim),
		commands.C(commands.AnsiReset))})
}

// ---------------------------------------------------------------------------
// orgs note - a line onto the body
// ---------------------------------------------------------------------------

type Note struct {
	tf      commands.TargetFlags
	File    string
	Stamp   bool
	Prepend bool
}

func (self *Note) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Note) StartPlugin(m *common.PluginManager)       {}

func (self *Note) SetupParameters(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
	fset.StringVar(&self.File, "f", "", "read the note from a file, or - for stdin")
	fset.BoolVar(&self.Stamp, "stamp", false, "put an inactive timestamp in front of it")
	fset.BoolVar(&self.Prepend, "top", false, "put it at the top of the body rather than the end")
}

func (self *Note) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("note").Flags)

	text := ""
	rest := words
	switch {
	case self.File == "-":
		// Note that this is the one verb where stdin is the *note* rather than
		// a list of hashes, and only when -f - says so. Which is why it takes
		// a flag: guessing would make the two meanings of a pipe depend on
		// what else happened to be on the line.
		b, err := readAll(os.Stdin)
		if err != nil {
			commands.Fail("could not read the note: %v", err)
		}
		text = b
	case self.File != "":
		b, err := os.ReadFile(self.File)
		if err != nil {
			commands.Fail("could not read %s: %v", self.File, err)
		}
		text = string(b)
	case len(words) > 0:
		text = words[0]
		rest = words[1:]
	}
	if strings.TrimSpace(text) == "" {
		commands.Fail("orgs note: say what to write.\n\n%s", noteUsage)
	}
	if self.Stamp {
		text = fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 Mon 15:04"), text)
	}

	todos := commands.Resolve(core, &self.tf, rest, commands.TargetOpts{
		Prompt: "Note on> ",
		Multi:  true,
	})

	lines := []string{}
	for _, t := range todos {
		body := ""
		if b, err := commands.SendReceiveGetErr[bodyResult](core,
			"body/"+commands.HashPath(t.Hash), nil); err == nil && b.Ok {
			body = proseOnly(b.Text)
		}
		joined := ""
		switch {
		case strings.TrimSpace(body) == "":
			joined = text
		case self.Prepend:
			joined = text + "\n\n" + strings.TrimLeft(body, "\n")
		default:
			joined = strings.TrimRight(body, "\n") + "\n\n" + text
		}

		var reply common.Result
		commands.SendReceivePost(core, "body/change",
			&common.TodoItemChange{Hash: t.Hash, Value: joined}, &reply)
		if commands.DryRun {
			continue
		}
		if !reply.Ok {
			commands.Fail("could not write the note onto %s", t.Headline)
		}
		lines = append(lines, fmt.Sprintf("%s %s%s%s", commands.Describe(t),
			commands.C(commands.AnsiDim), commands.Ellipsis(oneLine(text), 40),
			commands.C(commands.AnsiReset)))
	}
	commands.Ok(len(lines), "noted", lines)
}

const noteUsage = `  orgs note 'rang back, no answer'
  orgs note 'saw this again today' -stamp
  orgs note -f notes.txt 'IsStatus("BLOCKED")'
  git log -1 --format=%s | orgs note -f - -hash hOpO…

  This one goes through /body/change, which parses the new body and writes the
  whole document back - so the file it lives in is re-serialised, and every
  drawer and table in it is reformatted on the way past. That is an acceptable
  price for a paragraph somebody meant to write, and the reason ticking a
  checkbox is a line edit instead.`

// proseOnly is the body without the parts /body/change will write again.
//
// `/body/{hash}` answers with the heading's lines **as written**, which includes
// the planning line and the property drawer - right for a reader, and a trap for
// a writer: `/body/change` sets the body from what it is given and the drawer
// comes back out of the headline's own properties, so handing the whole thing
// back writes the drawer twice. The first note appended to a heading with
// properties left it with two identical :PROPERTIES: blocks.
func proseOnly(text string) string {
	out := []string{}
	inDrawer := false
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		up := strings.ToUpper(t)
		switch {
		case inDrawer:
			if up == ":END:" {
				inDrawer = false
			}
			continue
		case up == ":PROPERTIES:" || up == ":LOGBOOK:":
			inDrawer = true
			continue
		case isPlanningLine(up):
			continue
		}
		out = append(out, line)
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

// A planning line is the SCHEDULED/DEADLINE/CLOSED line org writes directly
// under a headline. It comes back as part of the body and is regenerated from
// the headline's own dates, so it is dropped for the same reason the drawer is.
func isPlanningLine(up string) bool {
	for _, k := range []string{"SCHEDULED:", "DEADLINE:", "CLOSED:"} {
		if strings.HasPrefix(up, k) {
			return true
		}
	}
	return false
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

func readAll(f *os.File) (string, error) {
	var sb strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		sb.WriteString(sc.Text())
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n"), sc.Err()
}

// ---------------------------------------------------------------------------
// orgs check - a checkbox
// ---------------------------------------------------------------------------

type Check struct {
	tf    commands.TargetFlags
	On    bool
	Off   bool
	Every bool
}

func (self *Check) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Check) StartPlugin(m *common.PluginManager)       {}

func (self *Check) SetupParameters(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
	fset.BoolVar(&self.On, "on", false, "tick it, whatever it was")
	fset.BoolVar(&self.Off, "off", false, "clear it, whatever it was")
	fset.BoolVar(&self.Every, "every", false, "every box on the heading")
}

// The same shape the server matches, said again here because the client numbers
// the boxes and the server counts them again to find the line - so the two have
// to agree about what a box is. A `*` at column zero is a heading whose text
// happens to start with a box, not a bullet.
var checkItemRe = regexp.MustCompile(`^(\s*)([-+*]|\d+[.)])\s+\[([ xX-])\]\s?(.*)$`)

type checkBox struct {
	Index int
	Done  bool
	Text  string
}

func (self *Check) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("check").Flags)

	// A leading number is which box. Anything else is the target.
	which := -1
	rest := words
	if len(words) > 0 {
		if n, err := strconv.Atoi(words[0]); err == nil {
			which = n - 1
			rest = words[1:]
		}
	}

	todos := commands.Resolve(core, &self.tf, rest, commands.TargetOpts{
		Prompt: "Check on> ",
		Query:  "HasChecklist()",
		Multi:  self.Every || which >= 0,
	})

	lines := []string{}
	for _, t := range todos {
		boxes := boxesOf(core, t)
		if len(boxes) == 0 {
			commands.Fail("%s has no checkboxes", t.Headline)
		}
		want := []checkBox{}
		switch {
		case self.Every:
			want = boxes
		case which >= 0:
			if which >= len(boxes) {
				commands.Fail("%s has %d boxes, not %d", t.Headline, len(boxes), which+1)
			}
			want = []checkBox{boxes[which]}
		default:
			want = askBox(t, boxes)
		}
		for _, b := range want {
			done := !b.Done
			if self.On {
				done = true
			}
			if self.Off {
				done = false
			}
			var reply common.ResultMsg
			commands.SendReceivePost(core, "checklist", &checklistToggle{
				Hash: t.Hash, Index: b.Index, Text: b.Text, Done: done}, &reply)
			if commands.DryRun {
				continue
			}
			if !reply.Ok {
				commands.Fail("could not change box %d: %s", b.Index+1, reply.Msg)
			}
			mark := commands.C(commands.AnsiDim) + "[ ]" + commands.C(commands.AnsiReset)
			if done {
				mark = commands.C(commands.AnsiGreen) + "[x]" + commands.C(commands.AnsiReset)
			}
			lines = append(lines, fmt.Sprintf("%s %s %s", commands.Describe(t), mark, b.Text))
		}
	}
	commands.Ok(len(lines), "ticked", lines)
}

// checklistToggle is the shape POST /checklist takes. It carries both the index
// and what the client believes the item says, because the index goes stale the
// moment a line is added above it and the text alone cannot tell two identical
// items apart - both have to agree or the write is refused.
type checklistToggle struct {
	Hash  string
	Index int
	Text  string
	Done  bool
}

func boxesOf(core *commands.Core, t common.Todo) []checkBox {
	b, err := commands.SendReceiveGetErr[bodyResult](core, "body/"+commands.HashPath(t.Hash), nil)
	if err != nil || !b.Ok {
		return nil
	}
	out := []checkBox{}
	i := 0
	for _, line := range strings.Split(b.Text, "\n") {
		m := checkItemRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if m[2] == "*" && m[1] == "" {
			continue
		}
		out = append(out, checkBox{Index: i, Done: m[3] != " ", Text: m[4]})
		i++
	}
	return out
}

func askBox(t common.Todo, boxes []checkBox) []checkBox {
	if !commands.Interactive() {
		commands.Fail("say which box: 1 to %d", len(boxes))
	}
	lines := []string{}
	for _, b := range boxes {
		mark := commands.C(commands.AnsiDim) + "[ ]" + commands.C(commands.AnsiReset)
		if b.Done {
			mark = commands.C(commands.AnsiGreen) + "[x]" + commands.C(commands.AnsiReset)
		}
		lines = append(lines, commands.PickLine([]string{strconv.Itoa(b.Index)},
			fmt.Sprintf("%s %s", mark, b.Text)))
	}
	sel := commands.Pick(commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "Box> ",
		Header:        t.Headline + "  ·  tab to mark several",
		Extra:         []string{"--multi"},
	})
	if len(sel) == 0 {
		commands.Fail("nothing chosen")
	}
	out := []checkBox{}
	for _, line := range sel {
		addr, ok := commands.Address(line, 1)
		if !ok {
			continue
		}
		n, err := strconv.Atoi(addr[0])
		if err != nil || n < 0 || n >= len(boxes) {
			continue
		}
		out = append(out, boxes[n])
	}
	return out
}

// ---------------------------------------------------------------------------
// orgs archive / orgs rm - taking a heading out of the file
// ---------------------------------------------------------------------------

type Remove struct {
	tf commands.TargetFlags
	// Which endpoint: "archive" moves the subtree to the archive file, "delete"
	// does not.
	How     string
	cmdName string
}

func (self *Remove) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Remove) StartPlugin(m *common.PluginManager)       {}

func (self *Remove) SetupParameters(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
}

// label is the verb with a capital, for a prompt. strings.Title is deprecated
// and a table of two words does not need a unicode word segmenter.
func (self *Remove) label() string {
	if self.How == "delete" {
		return "Delete"
	}
	return "Archive"
}

func (self *Remove) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find(self.cmdName).Flags)

	todos := commands.Resolve(core, &self.tf, words, commands.TargetOpts{
		Prompt: self.label() + "> ",
		Multi:  true,
	})

	// Both of these take the whole subtree, which is the part worth saying
	// before it happens rather than after. `orgs rm` in particular has no undo
	// on this side of it.
	what := "archive"
	if self.How == "delete" {
		what = "delete"
	}
	if !commands.DryRun {
		if len(todos) == 1 {
			if !commands.Confirm(&self.tf, "%s %q and everything under it?", what, todos[0].Headline) {
				commands.Fail("nothing was %sd", what)
			}
		} else if !commands.Confirm(&self.tf, "%s %d headings and everything under them?",
			what, len(todos)) {
			commands.Fail("nothing was %sd", what)
		}
	}

	lines := []string{}
	for _, t := range todos {
		var reply common.ResultMsg
		commands.SendReceivePost(core, self.How,
			&common.Target{Id: t.Hash, Type: "hash"}, &reply)
		if commands.DryRun {
			continue
		}
		if !reply.Ok {
			commands.Fail("could not %s %s: %s", what, t.Headline, reply.Msg)
		}
		lines = append(lines, commands.Describe(t))
	}
	commands.Ok(len(lines), what+"d", lines)
}

// ---------------------------------------------------------------------------

// bodyResult is what /body/{hash} answers with. Restated rather than imported:
// it lives in internal/app/orgs, which a command package may not import.
type bodyResult struct {
	Ok     bool
	Msg    string
	Text   string
	Audio  string
	Image  string
	Images []string
}

func init() {
	commands.AddCmd("todo", "change a heading's todo keyword",
		func() commands.Cmd { return &Todo{} })
	commands.AddCmd("sched", "schedule a heading",
		func() commands.Cmd { return &Dated{Which: "SCHEDULED", cmdName: "sched"} })
	commands.AddCmd("deadline", "put a deadline on a heading",
		func() commands.Cmd { return &Dated{Which: "DEADLINE", cmdName: "deadline"} })
	commands.AddCmd("tag", "add and remove a heading's tags",
		func() commands.Cmd { return &Tag{} })
	commands.AddCmd("prop", "read and write a heading's properties",
		func() commands.Cmd { return &Prop{} })
	commands.AddCmd("rename", "change a heading's text",
		func() commands.Cmd { return &Rename{} })
	commands.AddCmd("note", "append a note to a heading's body",
		func() commands.Cmd { return &Note{} })
	commands.AddCmd("check", "tick and clear the checkboxes on a heading",
		func() commands.Cmd { return &Check{} })
	commands.AddCmd("archive", "move a heading and its subtree to the archive",
		func() commands.Cmd { return &Remove{How: "archive", cmdName: "archive"} })
	commands.AddCmd("rm", "delete a heading and its subtree",
		func() commands.Cmd { return &Remove{How: "delete", cmdName: "rm"} })
}
