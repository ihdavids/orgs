package orgpkgs

// The optional org packages orgs does the work of, from the command line.
//
//	orgs features                       which are on
//	orgs features edna on               turn one on (written into the server's yaml)
//	orgs edna                           a heading's TRIGGER and BLOCKER, and what done would do
//	orgs edna -check 'ids(x) !done?' -blocker   read one without a server
//	orgs edna -names                    every finder, action and condition
//	orgs noter                          every document with notes
//	orgs noter notes                    a session's notes, page by page
//	orgs noter add -page 4 'why √d'     a note at a page (-y 0.3 how far down, -quote)
//	orgs noter start reading.org papers/x.pdf   start notes on a document
//	orgs remark                         every file with highlights
//	orgs remark show notes.txt          a file's highlights, as file:line:col
//	orgs remark add notes.txt -line 7 'the words' -note 'why'
//	orgs remark note ID 'new note'      orgs remark rm ID
//	orgs expire                         what expiry and auto archive would take now
//	orgs expire -run                    take it
//	orgs expire -set +2w                give a heading an EXPIRY (+span from CREATED, or a date)
//
// Each needs its feature on (`orgs features NAME on`); `orgs check -reset`
// unticks a heading's boxes whether or not checklistReset is on.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type base struct{ fset *flag.FlagSet }

func (self *base) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *base) StartPlugin(m *common.PluginManager)       {}
func (self *base) HelpGroup() string                          { return "Org packages" }

// offAnswer is what every feature endpoint says when it is turned off.
type offAnswer struct {
	Ok  bool
	Off bool
	Msg string
}

func failIfOff(a offAnswer) {
	if !a.Ok {
		commands.Fail("%s", a.Msg)
	}
}

func dim(s string) string {
	return commands.C(commands.AnsiDim) + s + commands.C(commands.AnsiReset)
}

// ---------------------------------------------------------------------------
// orgs features
// ---------------------------------------------------------------------------

type Features struct{ base }

func (self *Features) SetupParameters(fset *flag.FlagSet) { self.fset = fset }

type feature struct {
	Name, Title, Org, About string
	Enable                  bool
}

type featureList struct {
	Ok       bool
	Msg      string
	Features []feature
}

func (self *Features) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	var res featureList
	var err error
	switch len(words) {
	case 0:
		res, err = commands.SendReceiveGetErr[featureList](core, "features", nil)
	case 2:
		on := map[string]bool{"on": true, "enable": true, "true": true, "off": false, "disable": false, "false": false}
		v, ok := on[strings.ToLower(words[1])]
		if !ok {
			commands.Fail("orgs features NAME on|off")
		}
		req := struct {
			Name   string
			Enable bool
		}{words[0], v}
		res, err = commands.SendReceivePostErr[struct {
			Name   string
			Enable bool
		}, featureList](core, "features", &req)
		if err == commands.ErrDryRun {
			return
		}
	default:
		commands.Fail("orgs features [NAME on|off]")
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	commands.Render(res.Features, func() {
		for _, f := range res.Features {
			state := dim("off")
			if f.Enable {
				state = commands.C(commands.AnsiGreen) + "on " + commands.C(commands.AnsiReset)
			}
			fmt.Printf("%s  %-15s %s %s\n", state, f.Name, f.Title, dim("("+f.Org+")"))
		}
		if res.Msg != "" {
			fmt.Println(res.Msg)
		}
	})
}

// ---------------------------------------------------------------------------
// orgs edna
// ---------------------------------------------------------------------------

type Edna struct {
	base
	tf      commands.TargetFlags
	Check   string
	Blocker bool
	Names   bool
}

func (self *Edna) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	commands.AddTargetFlags(fset, &self.tf)
	fset.StringVar(&self.Check, "check", "", "read a TRIGGER (or with -blocker a BLOCKER) and say what is wrong with it")
	fset.BoolVar(&self.Blocker, "blocker", false, "-check reads a BLOCKER")
	fset.BoolVar(&self.Names, "names", false, "every finder, action and condition there is")
}

type ednaStep struct {
	Action, Headline, Filename string
	Ok                         bool
	Msg                        string
}

type ednaExplain struct {
	Ok           bool
	Off          bool
	Msg          string
	Headline     string
	Trigger      string
	TriggerError string
	Steps        []ednaStep
	Blocker      string
	BlockerError string
	Blocked      string
}

func (self *Edna) Exec(core *commands.Core) {
	if self.Names {
		type name struct{ Kind, Name, About string }
		rows := []name{}
		for _, k := range []struct {
			kind string
			m    map[string]string
		}{{"finder", common.EdnaFinders}, {"action", common.EdnaActions}, {"condition", common.EdnaConditions}} {
			for _, n := range common.EdnaNames() {
				if about, ok := k.m[n]; ok {
					rows = append(rows, name{k.kind, n, about})
				}
			}
		}
		commands.Render(rows, func() {
			kind := ""
			for _, r := range rows {
				if r.Kind != kind {
					kind = r.Kind
					fmt.Printf("%s%ss%s\n", commands.C(commands.AnsiBold), kind, commands.C(commands.AnsiReset))
				}
				fmt.Printf("  %-18s %s\n", r.Name, dim(r.About))
			}
		})
		return
	}
	if self.fset.Lookup("check") != nil && flagSet(self.fset, "check") {
		prop := "TRIGGER"
		if self.Blocker {
			prop = "BLOCKER"
		}
		err := common.ValidateEdna(prop, self.Check)
		type answer struct {
			Ok         bool
			Msg        string
			Start, End int
		}
		a := answer{Ok: err == nil}
		if e, ok := err.(*common.EdnaError); ok {
			a.Msg, a.Start, a.End = e.Msg, e.Start, e.End
		}
		commands.RenderOne(a, func() {
			if a.Ok {
				fmt.Printf("%s✓%s %s reads\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), prop)
				return
			}
			fmt.Println(self.Check)
			fmt.Printf("%s%s%s\n", strings.Repeat(" ", a.Start), commands.C(commands.AnsiRed)+strings.Repeat("^", max(1, a.End-a.Start)), commands.C(commands.AnsiReset))
			fmt.Println(a.Msg)
		})
		if !a.Ok {
			os.Exit(1)
		}
		return
	}
	todos := commands.Resolve(core, &self.tf, commands.FreeArgs(self.fset), commands.TargetOpts{
		Prompt: "Edna for> ",
		Query:  `HasProperty("TRIGGER") || HasProperty("BLOCKER")`,
		Quiet:  true,
	})
	rows := []ednaExplain{}
	for _, t := range todos {
		e, err := commands.SendReceivePostErr[struct{ Hash string }, ednaExplain](core, "edna/explain", &struct{ Hash string }{t.Hash})
		if err != nil && err != commands.ErrDryRun {
			commands.Fail("%v", err)
		}
		if !e.Ok {
			commands.Fail("%s", e.Msg)
		}
		rows = append(rows, e)
	}
	commands.Render(rows, func() {
		for i, e := range rows {
			fmt.Println(commands.Describe(todos[i]))
			if e.Blocker != "" {
				fmt.Printf("  BLOCKER %s\n", e.Blocker)
				switch {
				case e.BlockerError != "":
					fmt.Printf("    %s%s%s\n", commands.C(commands.AnsiRed), e.BlockerError, commands.C(commands.AnsiReset))
				case e.Blocked != "":
					fmt.Printf("    %sblocked:%s %s\n", commands.C(commands.AnsiGold), commands.C(commands.AnsiReset), e.Blocked)
				default:
					fmt.Printf("    %snot blocked%s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset))
				}
			}
			if e.Trigger != "" {
				fmt.Printf("  TRIGGER %s\n", e.Trigger)
				if e.TriggerError != "" {
					fmt.Printf("    %s%s%s\n", commands.C(commands.AnsiRed), e.TriggerError, commands.C(commands.AnsiReset))
				}
				if len(e.Steps) > 0 {
					fmt.Println(dim("    marking it done would:"))
				}
				for _, s := range e.Steps {
					fmt.Printf("    %s on %q %s\n", s.Action, s.Headline, dim(commands.BaseName(s.Filename)))
				}
			}
			if e.Trigger == "" && e.Blocker == "" {
				fmt.Println(dim("  no TRIGGER or BLOCKER"))
			}
		}
	})
}

func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// ---------------------------------------------------------------------------
// orgs noter
// ---------------------------------------------------------------------------

type Noter struct {
	base
	tf    commands.TargetFlags
	Page  int
	Y     float64
	Quote string
	Title string
}

func (self *Noter) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	commands.AddTargetFlags(fset, &self.tf)
	fset.IntVar(&self.Page, "page", 0, "add: the page the note is on (from 1)")
	fset.Float64Var(&self.Y, "y", -1, "add: how far down the page, 0 to 1")
	fset.StringVar(&self.Quote, "quote", "", "add: the passage the note is about")
	fset.StringVar(&self.Title, "title", "", "add, start: the heading's title")
}

type noterSession struct {
	Hash, Headline, Filename string
	Line                     int
	Document, Url            string
	Exists                   bool
	LastPage, Notes          int
}

type noterNote struct {
	Hash, Headline string
	Line, Page     int
	Y              float64
	Quote, Text    string
}

func (self *Noter) session(core *commands.Core, words []string) common.Todo {
	todos := commands.Resolve(core, &self.tf, words, commands.TargetOpts{
		Prompt: "Session> ",
		Query:  `HasProperty("NOTER_DOCUMENT")`,
		Quiet:  true,
	})
	return todos[0]
}

func (self *Noter) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb, words = words[0], words[1:]
	}
	switch verb {
	case "", "ls", "list":
		res, err := commands.SendReceiveGetErr[struct {
			offAnswer
			Sessions []noterSession
		}](core, "noter/sessions", nil)
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(res.offAnswer)
		commands.Render(res.Sessions, func() {
			for _, s := range res.Sessions {
				doc := s.Document
				if !s.Exists {
					doc += commands.C(commands.AnsiRed) + " (missing)" + commands.C(commands.AnsiReset)
				}
				about := fmt.Sprintf("%d note%s", s.Notes, plural(s.Notes))
				if s.LastPage > 0 {
					about += fmt.Sprintf(", read to p.%d", s.LastPage)
				}
				fmt.Printf("%s:%d %s  %s  %s\n", dim(commands.BaseName(s.Filename)), s.Line+1, s.Headline, doc, dim(about))
			}
			if len(res.Sessions) == 0 {
				fmt.Println("no headings with a NOTER_DOCUMENT (orgs noter start FILE DOCUMENT)")
			}
		})
	case "notes", "show":
		t := self.session(core, words)
		res, err := commands.SendReceiveGetErr[struct {
			offAnswer
			Session noterSession
			Notes   []noterNote
		}](core, "noter/notes", map[string]string{"hash": t.Hash})
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(res.offAnswer)
		commands.Render(res.Notes, func() {
			fmt.Printf("%s %s\n", res.Session.Headline, dim(res.Session.Document))
			for _, n := range res.Notes {
				where := fmt.Sprintf("p.%d", n.Page)
				if n.Y >= 0 {
					where += fmt.Sprintf(" %d%%", int(n.Y*100+0.5))
				}
				fmt.Printf("  %-9s %s\n", where, n.Headline)
				if n.Quote != "" {
					fmt.Printf("            %s\n", dim("“"+commands.Ellipsis(strings.Join(strings.Fields(n.Quote), " "), 70)+"”"))
				}
				if n.Text != "" {
					fmt.Printf("            %s\n", commands.Ellipsis(strings.Join(strings.Fields(n.Text), " "), 70))
				}
			}
		})
	case "add":
		if self.Page < 1 {
			commands.Fail("orgs noter add -page N [-y 0.3] [-quote TEXT] [-title T] 'the note' [session]")
		}
		text := ""
		if len(words) > 0 {
			text, words = words[0], words[1:]
		}
		t := self.session(core, words)
		req := struct {
			Hash               string
			Page               int
			Y                  *float64
			Title, Quote, Text string
		}{Hash: t.Hash, Page: self.Page, Title: self.Title, Quote: self.Quote, Text: text}
		if self.Y >= 0 {
			req.Y = &self.Y
		}
		var reply common.Result
		commands.SendReceivePost(core, "noter/note", &req, &reply)
		if commands.DryRun {
			return
		}
		if !reply.Ok {
			commands.Fail("%s", reply.Msg)
		}
		commands.Ok(1, "noted", []string{commands.Describe(t) + " " + dim(reply.Msg)})
	case "start":
		if len(words) != 2 {
			commands.Fail("orgs noter start ORGFILE DOCUMENT [-title T]")
		}
		file := commands.ResolveOrgFile(core, words[0])
		doc := words[1]
		if abs, err := filepath.Abs(doc); err == nil {
			doc = abs
		}
		var reply common.Result
		commands.SendReceivePost(core, "noter/start", &struct{ Filename, Document, Title string }{file, doc, self.Title}, &reply)
		if commands.DryRun {
			return
		}
		if !reply.Ok {
			commands.Fail("%s", reply.Msg)
		}
		commands.Ok(1, "started", []string{commands.BaseName(file) + " " + dim(reply.Msg)})
	default:
		commands.Fail("orgs noter [ls | notes | add -page N 'note' | start ORGFILE DOCUMENT]")
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ---------------------------------------------------------------------------
// orgs remark
// ---------------------------------------------------------------------------

type Remark struct {
	base
	Line  int
	Note  string
	Label string
}

func (self *Remark) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.IntVar(&self.Line, "line", 1, "add: the line (from 1) to look for the words from")
	fset.StringVar(&self.Note, "note", "", "add: a note on the highlight")
	fset.StringVar(&self.Label, "label", "", "add: org-remark's pen label")
}

type remarkSource struct {
	Source, Title string
	Url           bool
	Count         int
}

type remarkHighlight struct {
	Id                       string
	Beg, End                 int
	Label, Text, Note        string
	Line, Col                int
	EndLine, EndCol          int
	Moved, Lost              bool
	NotesLine                int
}

// remarkSourceArg is a word as a source: a url, else a file made absolute
// here, since the server and this shell may not share a working directory.
func remarkSourceArg(w string) string {
	if strings.HasPrefix(w, "http://") || strings.HasPrefix(w, "https://") {
		return w
	}
	if abs, err := filepath.Abs(w); err == nil {
		return abs
	}
	return w
}

func (self *Remark) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb, words = words[0], words[1:]
	}
	switch verb {
	case "", "ls", "list":
		res, err := commands.SendReceiveGetErr[struct {
			offAnswer
			NotesFile string
			Sources   []remarkSource
		}](core, "remark/sources", nil)
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(res.offAnswer)
		commands.Render(res.Sources, func() {
			for _, s := range res.Sources {
				fmt.Printf("%s  %s\n", s.Source, dim(fmt.Sprintf("%d highlight%s", s.Count, plural(s.Count))))
			}
			fmt.Println(dim("kept in " + res.NotesFile))
		})
	case "show":
		if len(words) != 1 {
			commands.Fail("orgs remark show FILE")
		}
		src := remarkSourceArg(words[0])
		res, err := commands.SendReceiveGetErr[struct {
			offAnswer
			Highlights []remarkHighlight
		}](core, "remark", map[string]string{"source": src})
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(res.offAnswer)
		commands.Render(res.Highlights, func() {
			// file:line:col: so an editor's quickfix list can jump to each.
			for _, h := range res.Highlights {
				state := ""
				if h.Lost {
					state = commands.C(commands.AnsiRed) + " (its words are gone)" + commands.C(commands.AnsiReset)
				}
				fmt.Printf("%s:%d:%d: %s%s %s\n", words[0], h.Line+1, h.Col+1,
					commands.Ellipsis(strings.Join(strings.Fields(h.Text), " "), 60), state, dim("["+h.Id+"]"))
				if h.Note != "" {
					fmt.Printf("    %s\n", commands.Ellipsis(strings.Join(strings.Fields(h.Note), " "), 76))
				}
			}
		})
	case "add":
		if len(words) != 2 {
			commands.Fail("orgs remark add FILE 'the words' [-line N] [-note TEXT] [-label L]")
		}
		req := struct {
			Source            string
			Beg, End, Line    int
			Text, Label, Note string
		}{Source: remarkSourceArg(words[0]), Line: max(0, self.Line-1), Text: words[1], Label: self.Label, Note: self.Note}
		res, err := commands.SendReceivePostErr[struct {
			Source            string
			Beg, End, Line    int
			Text, Label, Note string
		}, struct {
			offAnswer
			Highlight remarkHighlight
		}](core, "remark/highlight", &req)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(res.offAnswer)
		commands.Ok(1, "highlighted", []string{fmt.Sprintf("%s:%d:%d %s", words[0], res.Highlight.Line+1, res.Highlight.Col+1, dim("["+res.Highlight.Id+"]"))})
	case "note":
		if len(words) != 2 {
			commands.Fail("orgs remark note ID 'the note'")
		}
		var reply common.Result
		commands.SendReceivePost(core, "remark/note", &struct{ Id, Note string }{words[0], words[1]}, &reply)
		if commands.DryRun {
			return
		}
		if !reply.Ok {
			commands.Fail("%s", reply.Msg)
		}
		commands.Ok(1, "noted", []string{words[0]})
	case "rm", "delete":
		if len(words) != 1 {
			commands.Fail("orgs remark rm ID")
		}
		var reply common.Result
		commands.SendReceivePost(core, "remark/delete", &struct{ Id string }{words[0]}, &reply)
		if commands.DryRun {
			return
		}
		if !reply.Ok {
			commands.Fail("%s", reply.Msg)
		}
		commands.Ok(1, "removed", []string{words[0]})
	default:
		commands.Fail("orgs remark [ls | show FILE | add FILE 'words' | note ID 'note' | rm ID]")
	}
}

// ---------------------------------------------------------------------------
// orgs expire
// ---------------------------------------------------------------------------

type Expire struct {
	base
	tf  commands.TargetFlags
	Run bool
	Set string
}

func (self *Expire) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	commands.AddTargetFlags(fset, &self.tf)
	fset.BoolVar(&self.Run, "run", false, "archive (or tag) what is due now")
	fset.StringVar(&self.Set, "set", "", "give a heading an EXPIRY: +2w from its CREATED, a date, or '' to take it off")
}

type expiryItem struct {
	Hash, Headline, Filename string
	Line                     int
	Feature, Action, Why     string
	Since                    string
}

type moveResult struct {
	Headline, Filename string
	Ok, Skipped        bool
	Msg                string
}

func (self *Expire) Exec(core *commands.Core) {
	if flagSet(self.fset, "set") {
		todos := commands.Resolve(core, &self.tf, commands.FreeArgs(self.fset), commands.TargetOpts{Prompt: "Expire> "})
		lines := []string{}
		for _, t := range todos {
			var reply common.Result
			commands.SendReceivePost(core, "expiry/set", &struct{ Hash, Value string }{t.Hash, self.Set}, &reply)
			if commands.DryRun {
				continue
			}
			if !reply.Ok {
				commands.Fail("%s: %s", t.Headline, reply.Msg)
			}
			lines = append(lines, commands.Describe(t)+" "+dim("EXPIRY "+strconv.Quote(self.Set)))
		}
		commands.Ok(len(lines), "set", lines)
		return
	}
	due, err := commands.SendReceiveGetErr[struct {
		offAnswer
		Expiry, AutoArchive bool
		Items               []expiryItem
	}](core, "expiry/due", nil)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(due.offAnswer)
	if !self.Run {
		commands.Render(due.Items, func() {
			for _, it := range due.Items {
				fmt.Printf("%s:%d %s  %s %s\n", dim(commands.BaseName(it.Filename)), it.Line+1, it.Headline,
					commands.C(commands.AnsiGold)+it.Action+commands.C(commands.AnsiReset), dim(it.Why))
			}
			if len(due.Items) == 0 {
				fmt.Println("nothing is due")
			} else {
				fmt.Println(dim("orgs expire -run to file them away"))
			}
		})
		return
	}
	if len(due.Items) == 0 {
		commands.Ok(0, "filed", []string{"nothing is due"})
		return
	}
	if !commands.Machine() && !commands.DryRun && !commands.Confirm(&self.tf, "file away %d heading%s?", len(due.Items), plural(len(due.Items))) {
		return
	}
	hashes := []string{}
	for _, it := range due.Items {
		hashes = append(hashes, it.Hash)
	}
	res, err := commands.SendReceivePostErr[struct{ Hashes []string }, struct {
		Ok           bool
		Msg          string
		Done, Failed int
		Results      []moveResult
	}](core, "expiry/run", &struct{ Hashes []string }{hashes})
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	lines := []string{}
	for _, r := range res.Results {
		l := fmt.Sprintf("%s %s", dim(commands.BaseName(r.Filename)), r.Headline)
		if !r.Ok {
			l += commands.C(commands.AnsiRed) + " " + r.Msg + commands.C(commands.AnsiReset)
		}
		lines = append(lines, l)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	commands.Ok(res.Done, "filed away", lines)
}

func init() {
	commands.AddCmd("features", "turn the optional org packages on and off (edna, noter, remark, expiry...)",
		func() commands.Cmd { return &Features{} })
	commands.AddCmd("edna", "a heading's TRIGGER and BLOCKER (org-edna), and what done would do",
		func() commands.Cmd { return &Edna{} })
	commands.AddCmd("noter", "notes on a PDF, page by page (org-noter)",
		func() commands.Cmd { return &Noter{} })
	commands.AddCmd("remark", "highlights and margin notes on any file (org-remark)",
		func() commands.Cmd { return &Remark{} })
	commands.AddCmd("expire", "archive what has expired or been done a while (org-expiry, auto archive)",
		func() commands.Cmd { return &Expire{} })
}
