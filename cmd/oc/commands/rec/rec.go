package rec

// Records, from the terminal - a record being one heading that stands for one
// thing: a person, a laptop, a playing card.
//
//	orgs rec                           every record there is
//	orgs rec ls -type equipment        one collection
//	orgs rec find thinkpad             the ones matching
//	orgs rec show thinkpad             one, in full
//	orgs rec add -type equipment       asked for, field by field
//	orgs rec set thinkpad WARRANTY=2027
//	orgs rec fields -type equipment    what that collection uses
//	orgs rec collections               what is being kept
//	orgs rec new-collection wine -file cellar.org
//	orgs rec birthdays
//	orgs rec ls -json | jq '.[].Props.EMAIL'
//
// `orgs contact` is the same engine with the address book's manners on: it
// defaults to the contact collection and lays a record out the way a person
// is worth laying out. This one defaults to *every* collection and shows a
// record as the fields it has, which is the only thing that can be said about
// a collection nobody has designed a view for.
//
// The one rule underneath all of it: a field's kind is worked out from its
// name, never declared. EMAIL_WORK is an email labelled work, BOUGHT_DATE is a
// date called bought. That is what lets a contact and a guitar pedal share one
// command, and it is why nothing here names a property.

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

type Rec struct {
	fset *flag.FlagSet
	// What this command is called on the command line. The pane is a second
	// run of this binary and has to be told which name to call back into -
	// `orgs rec preview` and `orgs contact preview` are different commands.
	verb string

	Type   string
	Query  string
	Name   string
	File   string
	Icon   string
	Parent string
	Days   int
	Hash   string
	All    bool
	Notes  bool
	Open   bool
	Fields stringList
}

// A -set flag that may be given more than once: -set EMAIL=a@b -set PHONE=555.
type stringList []string

func (self *stringList) String() string { return strings.Join(*self, ", ") }
func (self *stringList) Set(v string) error {
	*self = append(*self, v)
	return nil
}

func (self *Rec) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Rec) StartPlugin(manager *common.PluginManager)         {}

func (self *Rec) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Type, "type", "", "which collection, or every one when empty")
	fset.StringVar(&self.Query, "q", "", "text to search for")
	fset.StringVar(&self.Name, "name", "", "the name of the record to add")
	fset.StringVar(&self.File, "file", "", "org file to write to when the collection has no home yet")
	fset.StringVar(&self.Icon, "icon", "", "emoji for a new collection")
	fset.StringVar(&self.Parent, "parent", "", "hash of the heading to file a new collection under")
	fset.IntVar(&self.Days, "days", 90, "how far ahead birthdays looks")
	fset.StringVar(&self.Hash, "hash", "", "one record by its hash, for `preview` and `show`")
	fset.BoolVar(&self.All, "all", false, "every field and the whole history, not the useful part")
	fset.BoolVar(&self.Notes, "notes", false, "include the notes in a listing")
	fset.BoolVar(&self.Open, "open", false, "open the record in your editor")
	fset.Var(&self.Fields, "set", "NAME=value to write (repeatable)")
}

func (self *Rec) Exec(core *commands.Core) {
	// Taken once. FreeArgs consumes the arguments as it parses, so the
	// subcommand has to be read off this slice rather than asked of the flag
	// set afterwards - which would get nothing.
	words := commands.FreeArgs(self.fset)
	sub := ""
	if len(words) > 0 {
		sub = strings.ToLower(words[0])
		words = words[1:]
	}
	free := strings.TrimSpace(strings.Join(words, " "))

	switch sub {
	case "":
		// Bare `orgs rec` is the picker, the way bare `orgs code` is.
		self.pick(core, "")
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
	case "show", "info", "view", "cat":
		self.show(core, firstNonEmpty(free, self.Query))
	case "add", "new":
		self.add(core, firstNonEmpty(free, self.Name))
	case "set", "edit", "update":
		self.set(core, free)
	case "fields":
		self.fields(core)
	case "birthdays", "bday", "birthday":
		self.birthdays(core)
	case "collections", "types":
		self.collections(core)
	case "new-collection", "newcollection", "mkcollection":
		self.newCollection(core, firstNonEmpty(free, self.Type))
	default:
		// `orgs rec jane` should search rather than complain: the first word
		// is only a subcommand when it is one.
		self.list(core, strings.TrimSpace(sub+" "+free))
	}
}

// ---------------------------------------------------------------------------
// The picker
// ---------------------------------------------------------------------------

// Bare `orgs rec` is a list with the record drawn beside it. Everything about
// it - the line, the pane, the bindings - is `commands/recview`, shared with
// `orgs contact`: the two are one engine with different manners, and a record
// that looked different in the two of them would be two records as far as
// anybody using both is concerned.
func (self *Rec) pick(core *commands.Core, q string) {
	// Nothing is reading a chooser: a pipe, a -json run or a cron job gets the
	// listing instead, which is the same question in a form it can use.
	if !commands.Interactive() {
		self.list(core, q)
		return
	}
	recs := self.fetch(core, q)
	if len(recs) == 0 {
		self.sayNothing(q)
		return
	}
	for _, r := range recview.Choose(core, recs, recview.PickOptions{
		Verb:     self.name(),
		Prompt:   self.prompt(),
		WithType: self.Type == "",
		All:      self.All,
	}) {
		if self.Open {
			core.LaunchEditor(r.Filename, r.LineNum+1)
			continue
		}
		recview.RenderPane(r, commands.PaneWidth(), self.All)
	}
}

func (self *Rec) name() string {
	if self.verb != "" {
		return self.verb
	}
	return "rec"
}

func (self *Rec) prompt() string {
	if self.Type != "" {
		return self.Type + "> "
	}
	return "record> "
}

func (self *Rec) sayNothing(q string) {
	switch {
	case q != "":
		fmt.Fprintf(os.Stderr, "nothing matches %q\n", q)
	case self.Type != "":
		fmt.Fprintf(os.Stderr, "there is nothing in %s yet — `orgs %s add -type %s` starts it off\n",
			self.Type, self.name(), self.Type)
	default:
		fmt.Fprintln(os.Stderr, "no records yet — see docs/records.org for what one looks like")
	}
}

// The pane fzf asked for.
func (self *Rec) preview(core *commands.Core) {
	// Drawn for fzf rather than for a terminal, so the colour has to be said
	// explicitly - stdout here is a pipe.
	commands.PickerOutput()
	if self.Hash == "" {
		commands.Fail("orgs %s preview: -hash says which record", self.name())
	}
	recview.Preview(core, self.Hash, self.All)
}

// `open -hash H`, which the picker binds to ctrl-o.
func (self *Rec) openAt(core *commands.Core) {
	r, ok := recview.ByHash(core, self.Hash)
	if !ok {
		return
	}
	core.LaunchEditor(r.Filename, r.LineNum+1)
}

// ---------------------------------------------------------------------------
// Talking to the server
// ---------------------------------------------------------------------------

func (self *Rec) fetch(core *commands.Core, q string) []common.Record {
	ps := map[string]string{"type": self.Type, "body": "1"}
	if q != "" {
		ps["q"] = q
	}
	recs, err := commands.SendReceiveGetErr[[]common.Record](core, "records", ps)
	if err != nil {
		commands.Fail("orgs rec: %v", err)
	}
	return recs
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

func (self *Rec) list(core *commands.Core, q string) {
	recs := self.fetch(core, q)
	commands.Render(recs, func() {
		if len(recs) == 0 {
			if q != "" {
				fmt.Fprintf(os.Stderr, "nothing matches %q\n", q)
			} else if self.Type != "" {
				fmt.Fprintf(os.Stderr, "there is nothing in %s yet — `orgs rec add -type %s` starts it off\n",
					self.Type, self.Type)
			} else {
				fmt.Fprintln(os.Stderr, "no records yet — see docs/records.org for what one looks like")
			}
			return
		}
		// Banded by collection whenever there is more than one, because a
		// list of a laptop, a wine and a person reading as one list is a list
		// of nothing.
		byType := map[string][]common.Record{}
		order := []string{}
		for _, r := range recs {
			if _, seen := byType[r.Type]; !seen {
				order = append(order, r.Type)
			}
			byType[r.Type] = append(byType[r.Type], r)
		}
		sort.Strings(order)
		for i, t := range order {
			if len(order) > 1 {
				if i > 0 {
					fmt.Println()
				}
				fmt.Printf("%s%s%s %s(%d)%s\n",
					commands.C(commands.AnsiBold), t, commands.C(commands.AnsiReset),
					commands.C(commands.AnsiDim), len(byType[t]), commands.C(commands.AnsiReset))
			}
			self.listOne(byType[t], len(order) > 1)
		}
		fmt.Fprintf(os.Stderr, "\n%d record(s)\n", len(recs))
	})
}

func (self *Rec) listOne(recs []common.Record, indent bool) {
	pad := ""
	if indent {
		pad = "  "
	}
	width := 0
	for _, r := range recs {
		if n := len(r.Name); n > width && n <= 34 {
			width = n
		}
	}
	for _, r := range recs {
		// Two fields beside the name, chosen by kind rather than by property
		// name: whatever this collection happens to keep is what shows.
		fmt.Printf("%s%s%-*s%s  %s%s%s\n",
			pad, commands.C(commands.AnsiBold), width, trunc(r.Name, 34), commands.C(commands.AnsiReset),
			commands.C(commands.AnsiDim), blurb(r), commands.C(commands.AnsiReset))
		if self.Notes && strings.TrimSpace(r.Notes) != "" {
			for _, line := range strings.Split(r.Notes, "\n") {
				fmt.Printf("%s    %s%s%s\n", pad, commands.C(commands.AnsiDim), line, commands.C(commands.AnsiReset))
			}
		}
	}
}

// The two or three fields worth putting beside a name when nothing knows what
// the collection is about. The order is what a person reaches for first.
func blurb(r common.Record) string {
	parts := []string{}
	seen := map[string]bool{}
	for _, kind := range []string{"email", "tel", "url", "date", "postal", "text"} {
		for _, f := range r.Fields {
			if f.Kind != kind || seen[f.Key] || f.Value == "" {
				continue
			}
			seen[f.Key] = true
			parts = append(parts, trunc(f.Value, 34))
			break
		}
		if len(parts) >= 3 {
			break
		}
	}
	return strings.Join(parts, "  ·  ")
}

// ---------------------------------------------------------------------------
// One record
// ---------------------------------------------------------------------------

func (self *Rec) show(core *commands.Core, q string) {
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
// that found nothing says so rather than opening an empty chooser, and one
// being read by a program refuses rather than hanging on a prompt nothing can
// answer.
func (self *Rec) narrow(core *commands.Core, q string, message string) (common.Record, bool) {
	recs := self.fetch(core, q)
	switch len(recs) {
	case 0:
		if q == "" {
			commands.Fail("orgs rec: no records")
		}
		commands.Fail("orgs rec: nothing matches %q", q)
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
	if commands.Machine() {
		commands.Fail("orgs rec: %q matches %d records; say which", q, len(recs))
	}
	labels := []string{}
	for _, r := range recs {
		label := r.Name
		if b := blurb(r); b != "" {
			label += "  ·  " + b
		}
		if self.Type == "" {
			label += "  [" + r.Type + "]"
		}
		labels = append(labels, label)
	}
	answer := ""
	if err := survey.AskOne(&survey.Select{
		Message: message, Options: labels, PageSize: 15,
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

func (self *Rec) print(rec common.Record) {
	fmt.Println()
	fmt.Printf("  %s%s%s  %s%s%s", commands.C(commands.AnsiBold), rec.Name, commands.C(commands.AnsiReset),
		commands.C(commands.AnsiCyan), rec.Type, commands.C(commands.AnsiReset))
	if len(rec.Tags) > 0 {
		fmt.Printf("  %s:%s:%s", commands.C(commands.AnsiGreen), strings.Join(rec.Tags, ":"), commands.C(commands.AnsiReset))
	}
	fmt.Println()
	fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim),
		strings.Repeat("─", maxInt(len(rec.Name)+len(rec.Type)+2, 24)), commands.C(commands.AnsiReset))

	// The fields in the order the record itself keeps them. A collection
	// nobody has designed a view for has no better order than its own.
	fields := rec.Fields
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
	width := 0
	for _, f := range fields {
		if n := len(fieldLabel(f)); n > width {
			width = n
		}
	}
	for _, f := range fields {
		fmt.Printf("  %s%-*s%s  %s\n",
			commands.C(commands.AnsiDim), width, fieldLabel(f), commands.C(commands.AnsiReset), f.Value)
	}
	if strings.TrimSpace(rec.Notes) != "" {
		fmt.Println()
		for _, line := range strings.Split(rec.Notes, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}
	fmt.Println()
	if rec.Added != "" && !hasAdded(rec.History) {
		fmt.Printf("  %sadded %s%s\n", commands.C(commands.AnsiDim), rec.Added, commands.C(commands.AnsiReset))
	}
	shown := rec.History
	if !self.All && len(shown) > 3 {
		shown = shown[:3]
	}
	for _, h := range shown {
		what := h.What
		if what != "" {
			what = "  " + what
		}
		fmt.Printf("  %s%-8s %s%s%s\n", commands.C(commands.AnsiDim), h.Kind, h.When, what, commands.C(commands.AnsiReset))
	}
	if len(shown) < len(rec.History) {
		fmt.Printf("  %s… %d more, -all shows them%s\n",
			commands.C(commands.AnsiDim), len(rec.History)-len(shown), commands.C(commands.AnsiReset))
	}
	fmt.Printf("  %s%s:%d%s\n\n", commands.C(commands.AnsiDim), rec.Filename, rec.LineNum+1, commands.C(commands.AnsiReset))
}

// ---------------------------------------------------------------------------
// Adding and changing
// ---------------------------------------------------------------------------

func (self *Rec) add(core *commands.Core, name string) {
	if self.Type == "" {
		commands.Fail("orgs rec add: which collection? -type contact, -type equipment, …\n" +
			"`orgs rec collections` lists the ones you have")
	}
	fields := parseSets(self.Fields)
	// Everything given on the command line is an answer already given; only
	// what is missing is asked for. `orgs rec add -type wine "Barolo 2016"
	// -set REGION=Piedmont` writes without a single question.
	interactive := len(fields) == 0 && !commands.Machine()

	if name == "" {
		if commands.Machine() {
			commands.Fail("orgs rec add: no name")
		}
		if err := survey.AskOne(&survey.Input{Message: "Name:"}, &name, survey.Required); err != nil {
			return
		}
		interactive = true
	}
	notes := ""
	if interactive {
		offered, _ := commands.SendReceiveGetErr[[]common.RecordFieldUse](core, "records/fields",
			map[string]string{"type": self.Type})
		asked := map[string]bool{}
		for _, f := range offered {
			if asked[f.Key] {
				continue
			}
			asked[f.Key] = true
			v := ""
			if err := survey.AskOne(&survey.Input{
				Message: fieldLabel(common.RecordField{Name: f.Name, Label: f.Label}) + ":",
				Help:    "leave it empty to skip - a field with no value is not written",
			}, &v, nil); err != nil {
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
			if err := survey.AskOne(&survey.Confirm{Message: "Another field?", Default: false},
				&more, nil); err != nil || !more {
				break
			}
			key, value := "", ""
			if err := survey.AskOne(&survey.Input{
				Message: "Field name:",
				Help:    "NAME or NAME_LABEL - the kind follows from the name",
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
		if err := survey.AskOne(&survey.Input{Message: "Notes:"}, &notes, nil); err != nil {
			return
		}
	}

	req := common.RecordNew{Type: self.Type, Name: name, Fields: fields, Notes: notes, Filename: self.File}
	rec, err := commands.SendReceivePostErr[common.RecordNew, common.Record](core, "record", &req)
	if err != nil {
		if err == commands.ErrDryRun {
			return
		}
		commands.Fail("orgs rec add: %v", err)
	}
	if rec.Name == "" {
		// The server answers a refusal as a ResultMsg, which decodes into a
		// Record with nothing in it. Ask why rather than saying "done".
		var msg common.ResultMsg
		commands.SendReceivePost(core, "record", &req, &msg)
		if msg.Msg != "" {
			fmt.Fprintf(os.Stderr, "could not add that: %s\n", msg.Msg)
			self.suggestCollection()
			os.Exit(1)
		}
	}
	if commands.RenderOne(rec, nil) {
		return
	}
	fmt.Printf("\n  %sAdded%s %s%s%s  %s%s:%d%s\n\n",
		commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset),
		commands.C(commands.AnsiBold), name, commands.C(commands.AnsiReset),
		commands.C(commands.AnsiDim), rec.Filename, rec.LineNum+1, commands.C(commands.AnsiReset))
	if self.Open && rec.Filename != "" {
		core.LaunchEditor(rec.Filename, rec.LineNum+1)
	}
}

// The one refusal worth explaining: there is nowhere to put it yet.
func (self *Rec) suggestCollection() {
	fmt.Fprintf(os.Stderr, "\n  A collection needs a heading to file things under:\n\n")
	fmt.Fprintf(os.Stderr, "    * %s\n      :PROPERTIES:\n      :COLLECTION: %s\n      :END:\n\n",
		titleOf(self.Type), self.Type)
	fmt.Fprintf(os.Stderr, "  `orgs rec new-collection %s -file notes.org` writes that for you,\n", self.Type)
	fmt.Fprintf(os.Stderr, "  or add the first one straight to a file with -file notes.org\n\n")
}

// set writes fields without asking anything: `orgs rec set thinkpad
// WARRANTY_UNTIL=2027-04-01 SERIAL=`. An empty value takes the field off,
// which is the same rule the property endpoint follows.
func (self *Rec) set(core *commands.Core, free string) {
	// The words are the record and then NAME=value pairs. Everything with an
	// = in it is a field; everything before that is the name to look up.
	who, pairs := []string{}, []string{}
	for _, w := range strings.Fields(free) {
		if strings.Contains(w, "=") {
			pairs = append(pairs, w)
		} else if len(pairs) == 0 {
			who = append(who, w)
		}
	}
	set := parseSets(append(append([]string{}, self.Fields...), pairs...))
	q := firstNonEmpty(strings.Join(who, " "), self.Query)

	rec, ok := self.narrow(core, q, "Change which one?")
	if !ok {
		return
	}
	if len(set) == 0 {
		if commands.Machine() {
			commands.Fail("orgs rec set: nothing to write — NAME=value")
		}
		var err error
		if set, err = self.askField(rec); err != nil || len(set) == 0 {
			return
		}
	}

	req := common.RecordUpdate{Hash: rec.Hash, Set: set}
	res, err := commands.SendReceivePostErr[common.RecordUpdate, common.ResultMsg](core, "record/update", &req)
	if err != nil {
		if err == commands.ErrDryRun {
			return
		}
		commands.Fail("orgs rec set: %v", err)
	}
	if !res.Ok {
		commands.Fail("orgs rec set: %s", res.Msg)
	}
	if commands.RenderOne(res, nil) {
		return
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if set[k] == "" {
			fmt.Printf("  %stook %s off%s %s\n",
				commands.C(commands.AnsiGold), strings.ToLower(k), commands.C(commands.AnsiReset), rec.Name)
		} else {
			fmt.Printf("  %s%s%s is now %s\n",
				commands.C(commands.AnsiGold), strings.ToLower(k), commands.C(commands.AnsiReset), set[k])
		}
	}
}

func (self *Rec) askField(rec common.Record) (map[string]string, error) {
	self.print(rec)
	labels, byLabel := []string{}, map[string]string{}
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
		return nil, err
	}
	key := byLabel[choice]
	if choice == addNew {
		if err := survey.AskOne(&survey.Input{Message: "Field name:"}, &key, nil); err != nil {
			return nil, err
		}
		if key = strings.ToUpper(strings.TrimSpace(key)); key == "" {
			return nil, nil
		}
	}
	value := rec.Props[key]
	if err := survey.AskOne(&survey.Input{
		Message: key + ":", Default: value,
		Help: "empty takes the field off the record",
	}, &value, nil); err != nil {
		return nil, err
	}
	return map[string]string{key: strings.TrimSpace(value)}, nil
}

// ---------------------------------------------------------------------------
// Collections
// ---------------------------------------------------------------------------

func (self *Rec) collections(core *commands.Core) {
	cols, err := commands.SendReceiveGetErr[[]common.RecordCollection](core, "records/collections", nil)
	if err != nil {
		commands.Fail("orgs rec: %v", err)
	}
	commands.Render(cols, func() {
		if len(cols) == 0 {
			fmt.Fprintln(os.Stderr, "nothing is being kept as records yet")
			return
		}
		fmt.Println()
		for _, c := range cols {
			home := c.Filename
			if home == "" {
				home = "no home yet — records of this type are scattered"
			}
			icon := c.Icon
			if icon == "" {
				icon = " "
			}
			fmt.Printf("  %s %s%-18s%s %s%4d%s  %s%s%s\n",
				icon, commands.C(commands.AnsiBold), c.Type, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiCyan), c.Count, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiDim), home, commands.C(commands.AnsiReset))
		}
		fmt.Printf("\n  %sorgs rec ls -type <name> reads one%s\n\n",
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	})
}

func (self *Rec) newCollection(core *commands.Core, rtype string) {
	if rtype == "" {
		commands.Fail("orgs rec new-collection: what is it called? e.g. `orgs rec new-collection wine -file cellar.org`")
	}
	if self.File == "" {
		commands.Fail("orgs rec new-collection: -file says which org file to put the heading in")
	}
	req := common.RecordCollectionNew{
		Type:       rtype,
		Name:       firstNonEmpty(self.Name, titleOf(rtype)),
		Icon:       self.Icon,
		Fields:     keysOf(parseSets(self.Fields)),
		Filename:   self.File,
		ParentHash: self.Parent,
	}
	res, err := commands.SendReceivePostErr[common.RecordCollectionNew, common.ResultMsg](core, "records/collection", &req)
	if err != nil {
		if err == commands.ErrDryRun {
			return
		}
		commands.Fail("orgs rec new-collection: %v", err)
	}
	if !res.Ok {
		commands.Fail("orgs rec new-collection: %s", res.Msg)
	}
	commands.RenderOne(res, func() {
		fmt.Printf("  %s%s%s is a collection now, in %s\n",
			commands.C(commands.AnsiGreen), rtype, commands.C(commands.AnsiReset), self.File)
	})
}

func (self *Rec) fields(core *commands.Core) {
	if self.Type == "" {
		commands.Fail("orgs rec fields: which collection? -type contact")
	}
	use, err := commands.SendReceiveGetErr[[]common.RecordFieldUse](core, "records/fields",
		map[string]string{"type": self.Type})
	if err != nil {
		commands.Fail("orgs rec fields: %v", err)
	}
	commands.Render(use, func() {
		if len(use) == 0 {
			fmt.Fprintf(os.Stderr, "nothing in %s carries any fields yet\n", self.Type)
			return
		}
		for _, f := range use {
			fmt.Printf("  %s%-22s%s %s%-8s%s %s%d record(s)%s\n",
				commands.C(commands.AnsiBold), f.Key, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiCyan), f.Kind, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiDim), f.Count, commands.C(commands.AnsiReset))
		}
	})
}

// ---------------------------------------------------------------------------
// Birthdays
// ---------------------------------------------------------------------------

// Worked out by the server, never stored, so the agenda, the contact card and
// this all get the same answer - the 29th of February included.
func (self *Rec) birthdays(core *commands.Core) {
	now := time.Now()
	ps := map[string]string{
		"from": now.Format("2006-01-02"),
		"to":   now.AddDate(0, 0, self.Days).Format("2006-01-02"),
	}
	evs, err := commands.SendReceiveGetErr[[]common.BirthdayEvent](core, "records/birthdays", ps)
	if err != nil {
		commands.Fail("orgs rec birthdays: %v", err)
	}
	commands.Render(evs, func() {
		if len(evs) == 0 {
			fmt.Fprintf(os.Stderr, "no birthdays in the next %d days\n", self.Days)
			return
		}
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		fmt.Println()
		for _, b := range evs {
			when, perr := time.ParseInLocation("2006-01-02", b.Date, now.Location())
			away := ""
			if perr == nil {
				switch days := int(when.Sub(today).Hours() / 24); {
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
				commands.C(commands.AnsiGold), when.Format("Mon 2 Jan"), commands.C(commands.AnsiReset),
				commands.C(commands.AnsiBold), trunc(b.Name, 26), commands.C(commands.AnsiReset),
				commands.C(commands.AnsiDim), away, age, commands.C(commands.AnsiReset))
		}
		fmt.Println()
	})
}

// ---------------------------------------------------------------------------

func parseSets(kvs []string) map[string]string {
	out := map[string]string{}
	for _, kv := range kvs {
		if i := strings.Index(kv, "="); i > 0 {
			out[strings.ToUpper(strings.TrimSpace(kv[:i]))] = strings.TrimSpace(kv[i+1:])
		}
	}
	return out
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// What to call a field on screen: "phone (mobile)" reads better than
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

func titleOf(t string) string {
	if t == "" {
		return t
	}
	return strings.ToUpper(t[:1]) + t[1:]
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func init() {
	commands.AddCmd("rec", "records of any kind - contacts, equipment, anything worth a list",
		func() commands.Cmd { return &Rec{Days: 90, verb: "rec"} })
	// A second name for the same command, because "rec" is short and "record"
	// is what it is.
	commands.AddCmd("record", "the same thing as rec",
		func() commands.Cmd { return &Rec{Days: 90, verb: "record"} })
}
