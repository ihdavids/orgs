package contact

// The address book from the terminal.
//
//	orgs contact                     everybody, one line each
//	orgs contact find jane           the ones matching "jane"
//	orgs contact show jane           one contact in full
//	orgs contact add                 asked for, field by field
//	orgs contact edit jane           change a field on one
//	orgs contact birthdays           who is next, and how long
//	orgs contact collections         what else is being kept
//
// Contacts are records of type "contact" and nothing else, so every one of
// these takes -type to work on another collection: `orgs contact find -type
// equipment thinkpad` searches the equipment the same way. The command is
// called contact because that is what it is mostly for.

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/recview"
	"github.com/ihdavids/orgs/internal/common"
	survey "gopkg.in/AlecAivazis/survey.v1"
)

// Minimal ansi, the way the dnd client does it, so that piping to a file or a
// terminal that does not do colour still reads.
const (
	cReset = "\033[0m"
	cBold  = "\033[1m"
	cDim   = "\033[2m"
	cCyan  = "\033[36m"
	cGreen = "\033[32m"
	cGold  = "\033[33m"
)

type Contact struct {
	fset *flag.FlagSet

	Type   string
	Query  string
	Name   string
	Field  string
	Value  string
	File   string
	Days   int
	Hash   string
	All    bool
	Notes  bool
	Plain  bool
	Open   bool
	Fields stringList
}

// A -field flag that may be given more than once: -field EMAIL=a@b -field
// PHONE_MOBILE=555.
type stringList []string

func (self *stringList) String() string { return strings.Join(*self, ", ") }
func (self *stringList) Set(v string) error {
	*self = append(*self, v)
	return nil
}

func (self *Contact) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Contact) StartPlugin(manager *common.PluginManager)         {}

func (self *Contact) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Type, "type", "contact", "which collection to work on")
	fset.StringVar(&self.Query, "q", "", "text to search for")
	fset.StringVar(&self.Name, "name", "", "the name of the record to add or change")
	fset.StringVar(&self.Field, "field", "", "field to set with -value, for a non interactive edit")
	fset.StringVar(&self.Value, "value", "", "what to set -field to (empty removes it)")
	fset.StringVar(&self.File, "file", "", "org file to write to when the collection has no home yet")
	fset.IntVar(&self.Days, "days", 90, "how far ahead birthdays looks")
	fset.StringVar(&self.Hash, "hash", "", "one contact by its hash, for `preview` and `open`")
	fset.BoolVar(&self.All, "all", false, "show every field rather than the useful ones")
	fset.BoolVar(&self.Notes, "notes", false, "include the notes in a listing")
	fset.BoolVar(&self.Plain, "plain", false, "no colour, one record per line, for piping")
	fset.BoolVar(&self.Open, "open", false, "open the record in your editor")
	fset.Var(&self.Fields, "set", "NAME=value to write when adding (repeatable)")
}

func (self *Contact) Exec(core *commands.Core) {
	// Bare `orgs contact` is the picker, the way bare `orgs code` is. The
	// listing is one word away and is what anything reading the answer gets
	// whatever was asked for.
	sub := "pick"
	words := []string{}
	if self.fset != nil {
		args := self.fset.Args()
		if len(args) > 0 {
			sub = args[0]
			args = args[1:]
		}
		// Flags may come before or after the words, the way the dnd client
		// takes them: "find -type equipment thinkpad" and "find thinkpad -type
		// equipment" mean the same thing.
		for {
			self.fset.Parse(args)
			rest := self.fset.Args()
			if len(rest) == 0 {
				break
			}
			words = append(words, rest[0])
			args = rest[1:]
		}
	}
	free := strings.TrimSpace(strings.Join(words, " "))

	switch strings.ToLower(sub) {
	case "pick":
		self.pick(core, free)
	case "preview":
		self.preview(core)
	case "open":
		self.openAt(core)
	case "list", "ls":
		self.list(core, free)
	case "find", "search", "grep":
		self.list(core, firstNonEmpty(free, self.Query))
	case "show", "info", "view":
		self.show(core, firstNonEmpty(free, self.Query))
	case "add", "new":
		self.add(core, firstNonEmpty(free, self.Name))
	case "edit", "set", "update":
		self.edit(core, firstNonEmpty(free, self.Query))
	case "birthdays", "bday", "birthday":
		self.birthdays(core)
	case "collections", "types":
		self.collections(core)
	default:
		fmt.Printf("orgs contact: no subcommand %q\n\n", sub)
		fmt.Print(usage)
	}
}

const usage = `  orgs contact [list]              everybody, one line each
  orgs contact find <text>         the ones matching
  orgs contact show <text>         one, in full
  orgs contact add [name]          asked for, field by field
  orgs contact edit <text>         change a field on one
  orgs contact birthdays           who is next
  orgs contact collections         what else is being kept

  -type <name>   work on another collection (equipment, wine, ...)
  -all           show every field rather than the useful ones
  -plain         no colour, one record per line
`

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ----------------------------------------------------------------------------
// The picker
// ----------------------------------------------------------------------------

// Bare `orgs contact` is the address book as a list with the card beside it.
//
// The line, the pane and the bindings are `commands/recview`, shared with
// `orgs rec`. The two commands are one engine with different manners - this one
// defaults to the contact collection and does not label every row with it -
// and a contact that looked different in the two of them would be two contacts
// as far as anybody using both is concerned.
func (self *Contact) pick(core *commands.Core, q string) {
	// Nothing is reading a chooser: a pipe, a -json run or a cron job gets the
	// listing instead.
	if !commands.Interactive() {
		self.list(core, q)
		return
	}
	recs := self.fetch(core, q)
	if len(recs) == 0 {
		if q != "" {
			fmt.Fprintf(os.Stderr, "nothing in %s matches %q\n", self.Type, q)
		} else {
			fmt.Fprintf(os.Stderr, "there is nothing in %s yet — `orgs contact add` starts it off\n", self.Type)
		}
		return
	}
	for _, r := range recview.Choose(core, recs, recview.PickOptions{
		Verb:   "contact",
		Prompt: self.Type + "> ",
		// The address book never says which collection a row is in: every row
		// is in the same one, and a column of the same word is not a column.
		// `-type equipment` is the exception and is the reader asking for it.
		WithType: false,
		All:      self.All,
	}) {
		if self.Open {
			core.LaunchEditor(r.Filename, r.LineNum+1)
			continue
		}
		recview.RenderPane(r, commands.PaneWidth(), self.All)
	}
}

func (self *Contact) preview(core *commands.Core) {
	// Drawn for fzf rather than for a terminal, so the colour has to be said
	// explicitly - stdout here is a pipe.
	commands.PickerOutput()
	if self.Hash == "" {
		commands.Fail("orgs contact preview: -hash says which contact")
	}
	recview.Preview(core, self.Hash, self.All)
}

func (self *Contact) openAt(core *commands.Core) {
	r, ok := recview.ByHash(core, self.Hash)
	if !ok {
		return
	}
	core.LaunchEditor(r.Filename, r.LineNum+1)
}

// ----------------------------------------------------------------------------
// Talking to the server
// ----------------------------------------------------------------------------

func (self *Contact) fetch(core *commands.Core, q string) []common.Record {
	var reply []common.Record
	ps := map[string]string{"type": self.Type, "body": "1"}
	if q != "" {
		ps["q"] = q
	}
	commands.SendReceiveGet(core, "records", ps, &reply)
	return reply
}

func (self *Contact) collectionsOf(core *commands.Core) []common.RecordCollection {
	var reply []common.RecordCollection
	commands.SendReceiveGet(core, "records/collections", map[string]string{}, &reply)
	return reply
}

// ----------------------------------------------------------------------------
// Listing
// ----------------------------------------------------------------------------

func (self *Contact) list(core *commands.Core, q string) {
	recs := self.fetch(core, q)
	if commands.Render(recs, nil) {
		return
	}
	if len(recs) == 0 {
		if q != "" {
			fmt.Printf("Nothing in %s matches %q.\n", self.Type, q)
		} else {
			fmt.Printf("There is nothing in %s yet. `orgs contact add` starts it off.\n", self.Type)
		}
		return
	}
	if self.Plain {
		for _, r := range recs {
			fmt.Printf("%s\t%s\t%s\t%s:%d\n", r.Name, valueOf(r, "email"), valueOf(r, "tel"), r.Filename, r.LineNum+1)
		}
		return
	}

	width := 0
	for _, r := range recs {
		if n := len(r.Name); n > width {
			width = n
		}
	}
	if width > 34 {
		width = 34
	}
	fmt.Println()
	for _, r := range recs {
		name := r.Name
		if len(name) > width {
			name = name[:width-1] + "…"
		}
		fmt.Printf("  %s%-*s%s  %s%-30s%s %s%s%s\n",
			self.c(cBold), width, name, self.c(cReset),
			self.c(cCyan), trunc(valueOf(r, "email"), 30), self.c(cReset),
			self.c(cDim), valueOf(r, "tel"), self.c(cReset))
		if self.Notes && strings.TrimSpace(r.Notes) != "" {
			for _, line := range strings.Split(r.Notes, "\n") {
				fmt.Printf("    %s%s%s\n", self.c(cDim), line, self.c(cReset))
			}
		}
	}
	fmt.Printf("\n  %s%d in %s%s\n\n", self.c(cDim), len(recs), self.Type, self.c(cReset))
}

// The first value of a kind, which is what a one line listing has room for.
// A contact with three phone numbers shows the mobile if there is one, because
// that is the one anybody would ring.
func valueOf(r common.Record, kind string) string {
	best := ""
	for _, f := range r.Fields {
		if f.Kind != kind {
			continue
		}
		if best == "" {
			best = f.Value
		}
		if kind == "tel" && (f.Label == "MOBILE" || f.Name == "MOBILE" || f.Name == "CELL") {
			return f.Value
		}
		if kind == "email" && f.Label == "" {
			return f.Value
		}
	}
	return best
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 2 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func (self *Contact) c(code string) string {
	if self.Plain {
		return ""
	}
	// -plain came first and is kept; -no-color is the one every command has,
	// and the two saying different things about the same listing would be a
	// bug waiting to be reported.
	return commands.C(code)
}

// ----------------------------------------------------------------------------
// One record in full
// ----------------------------------------------------------------------------

func (self *Contact) show(core *commands.Core, q string) {
	rec, ok := self.narrow(core, q, "Which one?")
	if !ok {
		return
	}
	if commands.RenderOne(rec, nil) {
		return
	}
	self.print(rec)
	if self.Open {
		core.LaunchEditor(rec.Filename, rec.LineNum+1)
	}
}

// pick narrows to one record, asking when the search found several. A search
// that found nothing says so rather than opening an empty chooser.
func (self *Contact) narrow(core *commands.Core, q string, message string) (common.Record, bool) {
	recs := self.fetch(core, q)
	switch len(recs) {
	case 0:
		if q == "" {
			fmt.Printf("There is nothing in %s yet.\n", self.Type)
		} else {
			fmt.Printf("Nothing in %s matches %q.\n", self.Type, q)
		}
		return common.Record{}, false
	case 1:
		return recs[0], true
	}
	// An exact name match wins outright - typing somebody's whole name should
	// not put a menu in the way.
	for _, r := range recs {
		if strings.EqualFold(strings.TrimSpace(r.Name), q) {
			return r, true
		}
	}
	labels := []string{}
	for _, r := range recs {
		label := r.Name
		if e := valueOf(r, "email"); e != "" {
			label += "  ·  " + e
		}
		labels = append(labels, label)
	}
	answer := ""
	if err := survey.AskOne(&survey.Select{
		Message:  message,
		Options:  labels,
		PageSize: 15,
	}, &answer, nil); err != nil {
		return common.Record{}, false
	}
	for i, l := range labels {
		if l == answer {
			return recs[i], true
		}
	}
	return common.Record{}, false
}

// The order fields are printed in: the ones you reach for first, then
// everything else in the order the record itself keeps them. A field kind with
// no entry here sorts last, which is right for the odds and ends a collection
// of guitar pedals grows.
var kindOrder = map[string]int{
	"tel": 1, "email": 2, "social": 3, "postal": 4, "url": 5, "date": 6, "text": 7, "image": 9,
}

func (self *Contact) print(rec common.Record) {
	fmt.Println()
	fmt.Printf("  %s%s%s", self.c(cBold), rec.Name, self.c(cReset))
	if len(rec.Tags) > 0 {
		fmt.Printf("  %s:%s:%s", self.c(cGreen), strings.Join(rec.Tags, ":"), self.c(cReset))
	}
	fmt.Println()
	fmt.Printf("  %s%s%s\n", self.c(cDim), strings.Repeat("─", maxInt(len(rec.Name), 24)), self.c(cReset))

	fields := append([]common.RecordField{}, rec.Fields...)
	if !self.All {
		kept := []common.RecordField{}
		for _, f := range fields {
			if f.Kind == "image" {
				continue
			}
			kept = append(kept, f)
		}
		fields = kept
	}
	sort.SliceStable(fields, func(a, b int) bool {
		ra, rb := kindOrder[fields[a].Kind], kindOrder[fields[b].Kind]
		if ra == 0 {
			ra = 8
		}
		if rb == 0 {
			rb = 8
		}
		return ra < rb
	})

	width := 0
	for _, f := range fields {
		if n := len(fieldLabel(f)); n > width {
			width = n
		}
	}
	for _, f := range fields {
		fmt.Printf("  %s%-*s%s  %s\n",
			self.c(cDim), width, fieldLabel(f), self.c(cReset), f.Value)
	}

	if strings.TrimSpace(rec.Notes) != "" {
		fmt.Println()
		for _, line := range strings.Split(rec.Notes, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	fmt.Println()
	// The history usually opens with the Added line, and saying it twice
	// reads as two things having happened.
	if rec.Added != "" && !hasAdded(rec.History) {
		fmt.Printf("  %sadded %s%s\n", self.c(cDim), rec.Added, self.c(cReset))
	}
	if len(rec.History) > 0 {
		shown := rec.History
		if !self.All && len(shown) > 3 {
			shown = shown[:3]
		}
		for _, h := range shown {
			what := h.What
			if what != "" {
				what = "  " + what
			}
			fmt.Printf("  %s%-8s %s%s%s\n", self.c(cDim), h.Kind, h.When, what, self.c(cReset))
		}
		if len(shown) < len(rec.History) {
			fmt.Printf("  %s… %d more, -all shows them%s\n", self.c(cDim), len(rec.History)-len(shown), self.c(cReset))
		}
	}
	fmt.Printf("  %s%s:%d%s\n\n", self.c(cDim), rec.Filename, rec.LineNum+1, self.c(cReset))
}

// What to call a field on screen: "PHONE (mobile)" reads better than
// "PHONE_MOBILE" and is the same thing.
func fieldLabel(f common.RecordField) string {
	if f.Label == "" {
		return strings.ToLower(f.Name)
	}
	return strings.ToLower(f.Name) + " (" + strings.ToLower(f.Label) + ")"
}

func hasAdded(h []common.RecordChange) bool {
	for _, c := range h {
		if c.Kind == "added" {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ----------------------------------------------------------------------------
// Adding
// ----------------------------------------------------------------------------

func (self *Contact) add(core *commands.Core, name string) {
	fields := map[string]string{}
	for _, kv := range self.Fields {
		if i := strings.Index(kv, "="); i > 0 {
			fields[strings.ToUpper(strings.TrimSpace(kv[:i]))] = strings.TrimSpace(kv[i+1:])
		}
	}

	// Everything given on the command line is an answer already given; only
	// what is missing is asked for. `orgs contact add "Jane Roe" -set
	// EMAIL=jane@x` writes without a single question.
	interactive := len(self.Fields) == 0

	if name == "" {
		if err := survey.AskOne(&survey.Input{Message: "Name:"}, &name, survey.Required); err != nil {
			return
		}
		interactive = true
	}
	notes := ""

	if interactive {
		var offered []common.RecordFieldUse
		commands.SendReceiveGet(core, "records/fields", map[string]string{"type": self.Type}, &offered)
		asked := map[string]bool{}
		for _, f := range offered {
			if asked[f.Key] {
				continue
			}
			asked[f.Key] = true
			v := ""
			prompt := &survey.Input{
				Message: fieldLabel(common.RecordField{Name: f.Name, Label: f.Label}) + ":",
				Help:    "leave it empty to skip - a field with no value is not written",
			}
			if err := survey.AskOne(prompt, &v, nil); err != nil {
				return
			}
			if v = strings.TrimSpace(v); v != "" {
				fields[f.Key] = v
			}
		}
		// Anything the collection has never seen, for as long as they keep
		// typing. This is the only way a field enters a collection for the
		// first time.
		for {
			more := false
			if err := survey.AskOne(&survey.Confirm{
				Message: "Another field?", Default: false,
			}, &more, nil); err != nil || !more {
				break
			}
			key, value := "", ""
			if err := survey.AskOne(&survey.Input{
				Message: "Field name:",
				Help:    "EMAIL, PHONE_HOME, LINKEDIN - NAME or NAME_LABEL, and the kind follows from the name",
			}, &key, nil); err != nil {
				break
			}
			if key = strings.ToUpper(strings.TrimSpace(key)); key == "" {
				continue
			}
			if err := survey.AskOne(&survey.Input{Message: key + ":"}, &value, nil); err != nil {
				break
			}
			if value = strings.TrimSpace(value); value != "" {
				fields[key] = value
			}
		}
		// One line here rather than an editor: survey's editor prompt hands
		// the terminal to $EDITOR, which is a lot to spring on somebody in
		// the middle of typing a phone number. Anything longer is a job for
		// worg's contact card or the file itself.
		if err := survey.AskOne(&survey.Input{
			Message: "Notes:",
			Help:    "a line about them - the org file takes as much as you like later",
		}, &notes, nil); err != nil {
			return
		}
	}

	req := common.RecordNew{
		Type: self.Type, Name: name, Fields: fields, Notes: notes, Filename: self.File,
	}
	var reply common.Record
	commands.SendReceivePost(core, "record", &req, &reply)
	if reply.Name == "" {
		// The server answers a refusal as a ResultMsg, which decodes into a
		// Record with nothing in it. Ask why rather than saying "done".
		var msg common.ResultMsg
		commands.SendReceivePost(core, "record", &req, &msg)
		if msg.Msg != "" {
			fmt.Printf("Could not add that: %s\n", msg.Msg)
			self.suggestCollection(core)
			return
		}
	}
	fmt.Printf("\n  %sAdded%s %s%s%s  %s%s:%d%s\n\n",
		self.c(cGreen), self.c(cReset), self.c(cBold), name, self.c(cReset),
		self.c(cDim), reply.Filename, reply.LineNum+1, self.c(cReset))
	if self.Open && reply.Filename != "" {
		core.LaunchEditor(reply.Filename, reply.LineNum+1)
	}
}

// The one refusal worth explaining: there is nowhere to put it yet.
func (self *Contact) suggestCollection(core *commands.Core) {
	fmt.Printf("\n  A collection needs a heading to file things under:\n\n")
	fmt.Printf("    * %s\n      :PROPERTIES:\n      :COLLECTION: %s\n      :END:\n\n",
		strings.ToUpper(self.Type[:1])+self.Type[1:], self.Type)
	fmt.Printf("  Put that in an org file, or add the first one straight to a file\n")
	fmt.Printf("  with  orgs contact add -file notes.org\n\n")
}

// ----------------------------------------------------------------------------
// Editing
// ----------------------------------------------------------------------------

func (self *Contact) edit(core *commands.Core, q string) {
	rec, ok := self.narrow(core, q, "Which one?")
	if !ok {
		return
	}
	set := map[string]string{}

	if self.Field != "" {
		set[strings.ToUpper(self.Field)] = self.Value
	} else {
		self.print(rec)
		labels := []string{}
		byLabel := map[string]string{}
		for _, f := range rec.Fields {
			l := fieldLabel(f) + "  ·  " + f.Value
			labels = append(labels, l)
			byLabel[l] = f.Key
		}
		const addNew = "  ···  a field it does not have yet"
		labels = append(labels, addNew)
		choice := ""
		if err := survey.AskOne(&survey.Select{
			Message: "Change what?", Options: labels, PageSize: 15,
		}, &choice, nil); err != nil {
			return
		}
		key := byLabel[choice]
		if choice == addNew {
			if err := survey.AskOne(&survey.Input{
				Message: "Field name:",
				Help:    "NAME or NAME_LABEL - EMAIL_WORK, PHONE_HOME",
			}, &key, nil); err != nil {
				return
			}
			key = strings.ToUpper(strings.TrimSpace(key))
			if key == "" {
				return
			}
		}
		value := rec.Props[key]
		if err := survey.AskOne(&survey.Input{
			Message: key + ":", Default: value,
			Help: "empty takes the field off the record",
		}, &value, nil); err != nil {
			return
		}
		set[key] = strings.TrimSpace(value)
	}

	req := common.RecordUpdate{Hash: rec.Hash, Set: set}
	var res common.ResultMsg
	commands.SendReceivePost(core, "record/update", &req, &res)
	if !res.Ok {
		fmt.Printf("Could not change that: %s\n", res.Msg)
		return
	}
	for k, v := range set {
		if v == "" {
			fmt.Printf("\n  %sTook %s off%s %s\n\n", self.c(cGold), strings.ToLower(k), self.c(cReset), rec.Name)
		} else {
			fmt.Printf("\n  %s%s%s is now %s\n\n", self.c(cGold), strings.ToLower(k), self.c(cReset), v)
		}
	}
}

// ----------------------------------------------------------------------------
// Birthdays
// ----------------------------------------------------------------------------

func (self *Contact) birthdays(core *commands.Core) {
	now := time.Now()
	from := now.Format("2006-01-02")
	to := now.AddDate(0, 0, self.Days).Format("2006-01-02")
	var reply []common.BirthdayEvent
	commands.SendReceiveGet(core, "records/birthdays", map[string]string{"from": from, "to": to}, &reply)
	if len(reply) == 0 {
		fmt.Printf("No birthdays in the next %d days.\n", self.Days)
		return
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	fmt.Println()
	for _, b := range reply {
		when, err := time.ParseInLocation("2006-01-02", b.Date, now.Location())
		away := ""
		if err == nil {
			days := int(when.Sub(today).Hours() / 24)
			switch {
			case days == 0:
				away = "today"
			case days == 1:
				away = "tomorrow"
			default:
				away = "in " + strconv.Itoa(days) + " days"
			}
		}
		age := ""
		if b.Age > 0 {
			age = " turns " + strconv.Itoa(b.Age)
		}
		fmt.Printf("  %s%-12s%s %s%-26s%s %s%s%s%s\n",
			self.c(cGold), when.Format("Mon 2 Jan"), self.c(cReset),
			self.c(cBold), trunc(b.Name, 26), self.c(cReset),
			self.c(cDim), away, age, self.c(cReset))
	}
	fmt.Println()
}

// ----------------------------------------------------------------------------
// Collections
// ----------------------------------------------------------------------------

func (self *Contact) collections(core *commands.Core) {
	cols := self.collectionsOf(core)
	if commands.Render(cols, nil) {
		return
	}
	if len(cols) == 0 {
		fmt.Println("Nothing is being kept as records yet.")
		return
	}
	fmt.Println()
	for _, c := range cols {
		home := c.Filename
		if home == "" {
			home = "no home yet - records of this type are scattered"
		}
		icon := c.Icon
		if icon == "" {
			icon = " "
		}
		fmt.Printf("  %s %s%-18s%s %s%4d%s  %s%s%s\n",
			icon, self.c(cBold), c.Type, self.c(cReset),
			self.c(cCyan), c.Count, self.c(cReset),
			self.c(cDim), home, self.c(cReset))
	}
	fmt.Printf("\n  %sorgs contact list -type <name> reads one%s\n\n", self.c(cDim), self.c(cReset))
}

func init() {
	commands.AddCmd("contact",
		"look up, add and change contacts - and any other collection of records",
		func() commands.Cmd {
			return &Contact{Type: "contact", Days: 90}
		})
	// "record" used to be a second name for this one, defaulting to the
	// contact collection - which is exactly the wrong default for a command
	// named after the general thing. It is `orgs rec` now, which starts on
	// every collection and lays a record out as the fields it has.
}
