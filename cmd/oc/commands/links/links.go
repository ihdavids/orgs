package links

// Where every link goes.
//
//	orgs links                       every link, grouped by service
//	orgs links jira                  the ones matching the pattern
//	orgs links -service GitHub       one service
//	orgs links -broken               the ones that point at nothing
//	orgs links -in notes.org         what points at that file
//	orgs links -out notes.org        what that file points at
//	orgs links -stats                per file counts
//	orgs links -json | jq '.[].Raw'
//
// The links worth going back and finding are usually the external ones: a
// ticket pasted into a heading eighteen months ago is findable by grep and by
// nothing else. So this keeps all of them, where /links and /links/graph
// answer "what points at this file" and throw away everything that is not org
// to org.
//
// The pattern runs over everything about a link - target, description, host,
// service, heading, file and where it lands - because "that jira link about
// the migration" and "that link in the meeting notes" are both things people
// type.

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/koki-develop/go-fzf"
)

type Links struct {
	fset *flag.FlagSet

	Query   string
	Service string
	File    string
	In      string
	Out     string
	Broken  bool
	Stats   bool
	Group   bool
	Open    bool
	Limit   int
	At      int

	// How many links there are before any filter, for the listing's "n of m".
	total int
}

func (self *Links) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Links) StartPlugin(manager *common.PluginManager)         {}

func (self *Links) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Query, "q", "", "a regular expression over everything about a link")
	fset.StringVar(&self.Service, "service", "", "only links to this service: GitHub, Jira, Google Docs")
	fset.StringVar(&self.File, "file", "", "only links written in this file")
	fset.StringVar(&self.In, "in", "", "what points at this file")
	fset.StringVar(&self.Out, "out", "", "what this file points at")
	fset.BoolVar(&self.Broken, "broken", false, "only the ones that resolve to nothing")
	fset.BoolVar(&self.Stats, "stats", false, "per file counts rather than the links")
	fset.BoolVar(&self.Group, "group", true, "gather the links under a band per service")
	fset.BoolVar(&self.Open, "open", false, "pick one and open where it was written")
	fset.IntVar(&self.Limit, "limit", 0, "keep only the first n")
	fset.IntVar(&self.At, "at", -1, "which row of the list, for `preview` and `open`")
}

func (self *Links) Exec(core *commands.Core) {
	// The words, taken once. `FreeArgs` parses flags wherever they were written
	// and *consumes* them as it goes, so asking the flag set for its arguments
	// afterwards gets nothing - which is what silently turned `orgs links
	// preview` into a search for the word "preview".
	words := commands.FreeArgs(self.fset)

	// The first word is a subcommand only when it is one of these. `orgs links
	// jira` has always meant "find the jira ones" and still does.
	sub := ""
	if len(words) > 0 {
		switch strings.ToLower(words[0]) {
		case "ls", "list", "pick", "preview", "open":
			sub = strings.ToLower(words[0])
			words = words[1:]
		}
	}
	if free := strings.TrimSpace(strings.Join(words, " ")); free != "" && self.Query == "" {
		self.Query = free
	}

	switch {
	case sub == "preview":
		self.preview(core)
	case sub == "open":
		self.openTarget(core)
	case self.Stats:
		self.stats(core)
	case self.In != "" || self.Out != "":
		self.backlinks(core)
	case sub == "ls" || sub == "list":
		self.all(core)
	default:
		// Bare `orgs links` is the picker, the way bare `orgs code` is. A
		// listing is one word away, and is what anything reading the answer
		// gets whatever was asked for.
		self.pick(core)
	}
}

// ---------------------------------------------------------------------------

// The links this command is showing, filtered the way the flags asked.
//
// One function for the listing, the picker and the pane, because the pane is a
// *second process* that has to index into the same answer: `-at 4` means the
// fifth row of this list, and if the two ever disagreed about what the list was
// the pane would quietly describe the wrong link.
func (self *Links) rows(core *commands.Core) []common.LinkEntry {
	ps := map[string]string{}
	if self.Service != "" {
		ps["service"] = self.Service
	}
	if self.File != "" {
		ps["file"] = self.File
	}
	list, err := commands.SendReceiveGetErr[common.LinkList](core, "links/all", ps)
	if err != nil {
		commands.Fail("orgs links: %v", err)
	}
	self.total = list.Total

	rows := list.Links
	if self.Query != "" {
		// A half typed pattern is a failure here rather than a state to sit
		// in: there is no box being typed into, so saying why beats matching
		// everything.
		re, cerr := regexp.Compile("(?i)" + self.Query)
		if cerr != nil {
			commands.Fail("orgs links: %q is not a pattern: %v", self.Query, cerr)
		}
		kept := rows[:0:0]
		for _, l := range rows {
			if re.MatchString(hay(l)) {
				kept = append(kept, l)
			}
		}
		rows = kept
	}
	if self.Broken {
		kept := rows[:0:0]
		for _, l := range rows {
			if l.Broken {
				kept = append(kept, l)
			}
		}
		rows = kept
	}
	if self.Limit > 0 && len(rows) > self.Limit {
		rows = rows[:self.Limit]
	}
	return rows
}

func (self *Links) all(core *commands.Core) {
	rows := self.rows(core)
	if self.Open {
		self.open(core, rows)
		return
	}
	commands.Render(rows, func() { self.print(rows, self.total) })
}

// Everything about a link, said once, so the pattern runs over the whole of it.
func hay(l common.LinkEntry) string {
	return strings.Join([]string{
		l.Raw, l.Desc, l.Host, l.Service, l.Scheme,
		l.Heading, l.Filename, l.ToFilename, l.ToHeadline,
		strings.Join(l.Olp, " "),
	}, " ")
}

func (self *Links) print(rows []common.LinkEntry, total int) {
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no links matched")
		return
	}
	if !self.Group {
		for _, l := range rows {
			fmt.Println(line(l))
		}
	} else {
		byService := map[string][]common.LinkEntry{}
		for _, l := range rows {
			byService[groupOf(l)] = append(byService[groupOf(l)], l)
		}
		names := make([]string, 0, len(byService))
		for k := range byService {
			names = append(names, k)
		}
		// The biggest group first: the strip is there to be picked from, and
		// what is worth picking is where most of the links are.
		sort.Slice(names, func(i, j int) bool {
			if len(byService[names[i]]) != len(byService[names[j]]) {
				return len(byService[names[i]]) > len(byService[names[j]])
			}
			return names[i] < names[j]
		})
		for i, n := range names {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("%s%s%s %s(%d)%s\n",
				commands.C(commands.AnsiBold), n, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiDim), len(byService[n]), commands.C(commands.AnsiReset))
			for _, l := range byService[n] {
				fmt.Printf("  %s\n", line(l))
			}
		}
	}
	if total > len(rows) {
		fmt.Fprintf(os.Stderr, "\n%d of %d\n", len(rows), total)
	}
}

// What a link is called when there is nothing better: an org to org link has
// no service, and calling it "external" would be a lie.
func groupOf(l common.LinkEntry) string {
	if l.Service != "" {
		return l.Service
	}
	if l.Scheme != "" {
		return l.Scheme
	}
	return "org"
}

func line(l common.LinkEntry) string {
	var b strings.Builder
	if l.Broken {
		fmt.Fprintf(&b, "%s✗%s ", commands.C(commands.AnsiRed), commands.C(commands.AnsiReset))
	} else {
		b.WriteString("  ")
	}
	desc := l.Desc
	if desc == "" {
		desc = l.Raw
	}
	fmt.Fprintf(&b, "%s%s%s", commands.C(commands.AnsiBold), desc, commands.C(commands.AnsiReset))
	if l.Desc != "" && l.Raw != l.Desc {
		fmt.Fprintf(&b, " %s%s%s", commands.C(commands.AnsiCyan), l.Raw, commands.C(commands.AnsiReset))
	}
	where := l.Heading
	if where == "" {
		where = "(preamble)"
	}
	fmt.Fprintf(&b, "\n      %sin %s — %s:%d%s",
		commands.C(commands.AnsiDim), where, l.Filename, l.Line+1, commands.C(commands.AnsiReset))
	return b.String()
}

func (self *Links) open(core *commands.Core, rows []common.LinkEntry) {
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no links matched")
		return
	}
	f, err := fzf.New(fzf.WithNoLimit(true))
	if err != nil {
		commands.Fail("%v", err)
	}
	idxs, err := f.Find(rows, func(i int) string {
		d := rows[i].Desc
		if d == "" {
			d = rows[i].Raw
		}
		return fmt.Sprintf("%s  %s  %s", d, rows[i].Raw, rows[i].Filename)
	})
	if err != nil {
		return
	}
	// Open where the link was *written*, not where it goes. Following it is
	// the browser's job and this is the thing the terminal can do that the
	// browser cannot: get you back to the heading you wrote it in.
	for _, i := range idxs {
		core.LaunchEditor(rows[i].Filename, rows[i].Line+1)
	}
}

// ---------------------------------------------------------------------------

func (self *Links) backlinks(core *commands.Core) {
	name := self.In
	if name == "" {
		name = self.Out
	}
	res, err := commands.SendReceiveGetErr[common.Backlinks](core, "links",
		map[string]string{"filename": name})
	if err != nil {
		commands.Fail("orgs links: %v", err)
	}
	if !res.Ok {
		commands.Fail("orgs links: %s", res.Msg)
	}
	rows := res.In
	what := "point at"
	if self.Out != "" {
		rows = append(append([]common.OrgLink{}, res.Out...), res.Internal...)
		what = "are written in"
	}
	commands.Render(rows, func() {
		if len(rows) == 0 {
			fmt.Fprintf(os.Stderr, "no links %s %s\n", what, res.Filename)
			return
		}
		fmt.Printf("%s%d link(s) %s %s%s\n\n",
			commands.C(commands.AnsiBold), len(rows), what, res.Filename, commands.C(commands.AnsiReset))
		for _, l := range rows {
			from := l.From.Headline
			if from == "" {
				from = "(preamble)"
			}
			to := l.To.Headline
			if to == "" {
				to = l.To.Filename
			}
			if to == "" {
				to = l.Raw
			}
			mark := "→"
			if l.Broken {
				mark = commands.C(commands.AnsiRed) + "✗" + commands.C(commands.AnsiReset)
			}
			fmt.Printf("%s %s %s\n    %s%s:%d%s\n",
				from, mark, to,
				commands.C(commands.AnsiDim), l.From.Filename, l.From.Line+1, commands.C(commands.AnsiReset))
		}
	})
}

func (self *Links) stats(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[common.LinkStatsResult](core, "links/stats", nil)
	if err != nil {
		commands.Fail("orgs links: %v", err)
	}
	if !res.Ok {
		commands.Fail("orgs links: %s", res.Msg)
	}
	rows := res.Files
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i].In + rows[i].Out + rows[i].Internal
		b := rows[j].In + rows[j].Out + rows[j].Internal
		if a != b {
			return a > b
		}
		return rows[i].Filename < rows[j].Filename
	})
	commands.Render(rows, func() {
		fmt.Printf("%s%6s %6s %6s %6s  %s%s\n",
			commands.C(commands.AnsiBold), "in", "out", "own", "broken", "file", commands.C(commands.AnsiReset))
		for _, f := range rows {
			broken := fmt.Sprintf("%6d", f.Broken)
			if f.Broken > 0 {
				broken = commands.C(commands.AnsiRed) + broken + commands.C(commands.AnsiReset)
			}
			fmt.Printf("%6d %6d %6d %s  %s\n", f.In, f.Out, f.Internal, broken, f.Filename)
		}
	})
}

func init() {
	commands.AddCmd("links", "every link in the database, and where it goes",
		func() commands.Cmd { return &Links{Group: true} })
}
