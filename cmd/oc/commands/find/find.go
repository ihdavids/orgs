package find

// A regular expression over the text of every org file - emacs' swiper, or
// ripgrep pointed at the org database, from the terminal.
//
//	orgs find 'migration'
//	orgs find -i 'jira-\d+'
//	orgs find 'TODO' -file notes.org
//	orgs find 'plot' -json | jq '.[].Text'
//	orgs find 'migration' -open
//
// This is not `orgs search`, which queries the parsed database and knows what
// a heading is. This reads lines and knows nothing, which is exactly what
// finds a phrase written in a drawer, a table or a source block.
//
// It is not `orgs grep` either. That one has always done something close to
// this and answers with "file:12:text" strings, which cannot be taken apart
// again when the line holds a colon and say nothing about what was left out.
// Here a match is a file, a line, the text, and where in the text it matched -
// so the match can be coloured without running the pattern a second time, and
// the two regular expression engines never have to agree.

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/koki-develop/go-fzf"
)

type Find struct {
	fset *flag.FlagSet

	Query      string
	File       string
	IgnoreCase bool
	Max        int
	Count      bool
	FilesOnly  bool
	Open       bool
}

// One match, flattened. The server answers with a file holding lines because
// that is how a panel draws it; a terminal reads one line at a time, and a
// pipe certainly does, so -json hands over the flat form rather than making
// everything downstream walk two levels to reach a line.
type Hit struct {
	Filename string
	Line     int
	Text     string
	Start    int
	End      int
}

func (self *Find) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Find) StartPlugin(manager *common.PluginManager)         {}

func (self *Find) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Query, "q", "", "the pattern, when you would rather not put it first")
	fset.StringVar(&self.File, "file", "", "only search this file")
	fset.BoolVar(&self.IgnoreCase, "i", false, "match without regard to case")
	fset.IntVar(&self.Max, "max", 40, "matching lines kept per file")
	fset.BoolVar(&self.Count, "count", false, "how many lines matched, and nothing else")
	fset.BoolVar(&self.FilesOnly, "l", false, "the names of the files that matched, one per line")
	fset.BoolVar(&self.Open, "open", false, "pick a line and open it in your editor")
}

func (self *Find) Exec(core *commands.Core) {
	query := self.Query
	if free := commands.FreeText(self.fset); free != "" {
		query = free
	}
	if strings.TrimSpace(query) == "" {
		commands.Fail("orgs find: no pattern.\n\n%s", usage)
	}

	ps := map[string]string{"q": query, "max": strconv.Itoa(self.Max)}
	if self.IgnoreCase {
		ps["ignoreCase"] = "t"
	}
	if self.File != "" {
		ps["file"] = self.File
	}
	res, err := commands.SendReceiveGetErr[common.FileSearchResult](core, "files/search", ps)
	if err != nil {
		commands.Fail("orgs find: %v", err)
	}
	// A half typed pattern is an ordinary state of a box being typed into, and
	// the server says so with Ok rather than with a status. From a terminal it
	// is the one thing that really is an error - nobody is mid-keystroke here.
	if !res.Ok {
		commands.Fail("orgs find: %s", res.Msg)
	}

	if self.Count {
		commands.RenderOne(struct {
			Lines int
			Files int
		}{res.Total, res.FileCount}, func() {
			fmt.Printf("%d\n", res.Total)
		})
		return
	}
	if self.FilesOnly {
		names := make([]string, 0, len(res.Files))
		for _, f := range res.Files {
			names = append(names, f.Filename)
		}
		commands.Render(names, func() {
			for _, n := range names {
				fmt.Println(n)
			}
		})
		return
	}

	hits := flatten(res)
	if self.Open {
		self.open(core, hits)
		return
	}
	commands.Render(hits, func() { self.print(res) })
}

func flatten(res common.FileSearchResult) []Hit {
	hits := []Hit{}
	for _, f := range res.Files {
		for _, m := range f.Matches {
			hits = append(hits, Hit{f.Filename, m.Line, m.Text, m.Start, m.End})
		}
	}
	return hits
}

func (self *Find) print(res common.FileSearchResult) {
	if res.Total == 0 {
		fmt.Fprintln(os.Stderr, "nothing matched")
		return
	}
	for i, f := range res.Files {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), f.Filename, commands.C(commands.AnsiReset))
		for _, m := range f.Matches {
			fmt.Printf("%s%6d%s  %s\n",
				commands.C(commands.AnsiDim), m.Line+1, commands.C(commands.AnsiReset),
				highlight(m))
		}
		// Say what was left out rather than letting the listing imply it saw
		// everything. The cap is on the lines kept, never on the count.
		if f.Truncated {
			fmt.Printf("%s        … %d more in this file%s\n",
				commands.C(commands.AnsiDim), f.Count-len(f.Matches), commands.C(commands.AnsiReset))
		}
	}
	fmt.Fprintf(os.Stderr, "\n%d line(s) in %d file(s)\n", res.Total, res.FileCount)
}

// The offsets come from the server, so the match is picked out of the line
// without running the pattern again - Go's regular expressions and anything
// else's do not have to agree about what it meant.
func highlight(m common.FileSearchMatch) string {
	s := m.Text
	if !commands.Colour() || m.Start < 0 || m.End > len(s) || m.Start >= m.End {
		return strings.TrimRight(s, "\r\n")
	}
	return strings.TrimRight(
		s[:m.Start]+commands.AnsiGold+commands.AnsiBold+s[m.Start:m.End]+commands.AnsiReset+s[m.End:],
		"\r\n")
}

func (self *Find) open(core *commands.Core, hits []Hit) {
	if len(hits) == 0 {
		fmt.Fprintln(os.Stderr, "nothing matched")
		return
	}
	f, err := fzf.New(fzf.WithNoLimit(true))
	if err != nil {
		commands.Fail("%v", err)
	}
	idxs, err := f.Find(hits, func(i int) string {
		return fmt.Sprintf("%s:%d  %s", hits[i].Filename, hits[i].Line+1, strings.TrimSpace(hits[i].Text))
	})
	if err != nil {
		return
	}
	for _, i := range idxs {
		// The server counts lines from zero everywhere; an editor counts from
		// one, and this is the edge the two meet at.
		core.LaunchEditor(hits[i].Filename, hits[i].Line+1)
	}
}

const usage = `  orgs find '<re>'              lines matching, in every org file
  orgs find -i '<re>'           without regard to case
  orgs find '<re>' -file x.org  in one file
  orgs find '<re>' -l           just the names of the files
  orgs find '<re>' -open        pick a line and open it`

func init() {
	commands.AddCmd("find", "a regular expression over the text of every file",
		func() commands.Cmd { return &Find{Max: 40} })
}
