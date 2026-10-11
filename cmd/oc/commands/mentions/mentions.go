package mentions

// What links to a heading, and where it is written about without a link.
//
//	orgs backlinks Apollo                 links into and out of a heading
//	orgs backlinks notes.org              ... or a file
//	orgs mentions Apollo                  where "Apollo" is written but not linked
//	orgs mentions Apollo -link            pick which of them become links (fzf, tab marks)
//	orgs mentions Apollo -link -only 1,3  link the first and third, no picker
//	orgs mentions Apollo -link -yes       link every one of them
//	orgs mentions -file notes.org -line 40   the heading line 40 is under
//	orgs mentions 'IsProject()' -json
//
// A heading is named the way every write verb names one (commands.Resolve): a
// hash, a query, hashes on stdin, or a picker when nothing was said. Plain
// words that are not a query are taken as the heading's text, because
// `orgs mentions Apollo` is how anybody would type it. A single word ending in
// .org is a file.
//
// The answers are /links/heading, /mentions and /mentions/link; nothing about
// what counts as a mention is decided here.

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// Naming the target
// ---------------------------------------------------------------------------

type targetFlags struct {
	tf   commands.TargetFlags
	File string
	Line int
}

func (self *targetFlags) setup(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
	fset.StringVar(&self.File, "file", "", "a file rather than a heading")
	fset.IntVar(&self.Line, "line", 0, "with -file: the heading this line (1 based) is under")
}

var orgFileRe = regexp.MustCompile(`(?i)\.org(\.gpg)?$`)

// A query has a call or an operator in it; words that have neither are a
// heading's text, and become a query that looks for it - for a heading called
// exactly that when there is one, so `orgs mentions Apollo` means Apollo and
// not also "Apollo launch checklist", and for any heading with the words in
// it when there is not.
func asQuery(core *commands.Core, words []string) []string {
	text := strings.TrimSpace(strings.Join(words, " "))
	if text == "" || text == "-" || commands.LooksLikeHash(text) || strings.ContainsAny(text, "()&|!=<>") {
		return words
	}
	// Only letters, digits and spaces go into the pattern as they are: the
	// rest become "any character", which keeps the text out of both the
	// pattern's and the query language's escaping rules.
	pat := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return '.'
	}, text)
	exact := fmt.Sprintf(`MatchHeadline("(?i)^ *%s *$")`, pat)
	if found, err := commands.SendReceiveGetErr[common.Todos](core, "search", map[string]string{"query": exact}); err == nil && len(found) > 0 {
		return []string{exact}
	}
	return []string{fmt.Sprintf(`MatchHeadline("(?i)%s")`, pat)}
}

// The query parameters naming the target, and a word for it.
func (self *targetFlags) resolve(core *commands.Core, words []string, prompt string) map[string]string {
	if self.File == "" && len(words) == 1 && orgFileRe.MatchString(words[0]) {
		self.File = words[0]
	}
	if self.File != "" {
		ps := map[string]string{"filename": self.File}
		if self.Line > 0 {
			ps["line"] = strconv.Itoa(self.Line - 1)
		}
		return ps
	}
	todos := commands.Resolve(core, &self.tf, asQuery(core, words), commands.TargetOpts{
		Prompt: prompt, Quiet: true,
	})
	return map[string]string{"hash": todos[0].Hash}
}

// How a target reads at the top of a listing.
func targetLine(t common.LinkTarget) string {
	name := t.Title
	where := commands.BaseName(t.Filename)
	if t.Kind == "heading" {
		name = strings.Join(t.Olp, " › ")
		where = fmt.Sprintf("%s:%d", where, t.Line+1)
	}
	s := commands.C(commands.AnsiBold) + name + commands.C(commands.AnsiReset) +
		"  " + commands.C(commands.AnsiDim) + where + commands.C(commands.AnsiReset)
	return s
}

// ---------------------------------------------------------------------------
// orgs backlinks
// ---------------------------------------------------------------------------

type Backlinks struct {
	fset *flag.FlagSet
	t    targetFlags
}

func (self *Backlinks) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Backlinks) StartPlugin(manager *common.PluginManager)         {}
func (self *Backlinks) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	self.t.setup(fset)
}

// A row for -format: which way the link runs, and the link.
type linkRow struct {
	Dir string // in or out
	common.LinkInContext
}

func (self *Backlinks) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	ps := self.t.resolve(core, words, "Links of> ")
	res, err := commands.SendReceiveGetErr[common.LinksAround](core, "links/heading", ps)
	if err != nil {
		commands.Fail("orgs backlinks: %v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	if commands.JsonOut {
		commands.RenderOne(res, nil)
		return
	}
	rows := []linkRow{}
	for _, l := range res.In {
		rows = append(rows, linkRow{"in", l})
	}
	for _, l := range res.Out {
		rows = append(rows, linkRow{"out", l})
	}
	if commands.Render(rows, nil) {
		return
	}
	fmt.Println(targetLine(res.Target))
	section := func(title string, ls []common.LinkInContext, in bool) {
		fmt.Printf("\n%s%s%s %s(%d)%s\n", commands.C(commands.AnsiBold), title, commands.C(commands.AnsiReset),
			commands.C(commands.AnsiDim), len(ls), commands.C(commands.AnsiReset))
		if len(ls) == 0 {
			fmt.Printf("  %snone%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		}
		for _, l := range ls {
			arrow, colour, end := "←", commands.AnsiBlue, l.From
			if !in {
				arrow, colour, end = "→", commands.AnsiGreen, l.To
			}
			name := end.Headline
			if name == "" {
				name = commands.BaseName(end.Filename)
			}
			if name == "" {
				name = l.Raw
			}
			where := fmt.Sprintf("%s:%d", commands.BaseName(l.From.Filename), l.From.Line+1)
			broken := ""
			if l.Broken {
				broken = " " + commands.C(commands.AnsiRed) + "broken" + commands.C(commands.AnsiReset)
			}
			fmt.Printf("  %s%s%s %s  %s%s%s%s\n", commands.C(colour), arrow, commands.C(commands.AnsiReset),
				name, commands.C(commands.AnsiDim), where, commands.C(commands.AnsiReset), broken)
			if l.Context != "" {
				fmt.Printf("      %s%s%s\n", commands.C(commands.AnsiDim), commands.Ellipsis(l.Context, 110), commands.C(commands.AnsiReset))
			}
		}
	}
	section("Linked from", res.In, true)
	section("Links out", res.Out, false)
}

// ---------------------------------------------------------------------------
// orgs mentions
// ---------------------------------------------------------------------------

type Mentions struct {
	fset   *flag.FlagSet
	t      targetFlags
	Link   bool
	Only   string
	Common bool
	Limit  int
	PaneAt int
}

func (self *Mentions) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Mentions) StartPlugin(manager *common.PluginManager)         {}
func (self *Mentions) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	self.t.setup(fset)
	fset.BoolVar(&self.Link, "link", false, "turn mentions into links: a picker, or every one with -yes")
	fset.StringVar(&self.Only, "only", "", "with -link: the mentions to link by their number, e.g. 1,3,5")
	fset.BoolVar(&self.Common, "common", false, "search for a title even when it is only common words")
	fset.IntVar(&self.Limit, "limit", 500, "at most this many mentions")
	// The picker's pane: a second run of this binary, handed the target
	// already resolved and the row to draw.
	fset.IntVar(&self.PaneAt, "pane-at", 0, "")
}

func (self *Mentions) query(core *commands.Core, ps map[string]string) common.MentionsResult {
	if self.Common {
		ps["all"] = "t"
	}
	ps["limit"] = strconv.Itoa(self.Limit)
	res, err := commands.SendReceiveGetErr[common.MentionsResult](core, "mentions", ps)
	if err != nil {
		commands.Fail("orgs mentions: %v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	return res
}

// One mention as a line: where, the heading, and the sentence with the match
// picked out.
func mentionLine(n int, m common.Mention) string {
	// Without colour the match is bracketed instead, or two mentions on one
	// line would read as the same row twice.
	open, shut := commands.C(commands.AnsiBold+commands.AnsiGold), commands.C(commands.AnsiReset)
	if open == "" {
		open, shut = "«", "»"
	}
	head := m.Headline
	if head == "" {
		head = "(top of file)"
	}
	return fmt.Sprintf("%s%3d%s %s%s:%d%s  %s%s%s  %s%s%s%s%s",
		commands.C(commands.AnsiDim), n, commands.C(commands.AnsiReset),
		commands.C(commands.AnsiCyan), commands.BaseName(m.Filename), m.Line+1, commands.C(commands.AnsiReset),
		commands.C(commands.AnsiDim), commands.Ellipsis(head, 28), commands.C(commands.AnsiReset),
		m.Before, open, m.Text, shut, m.After)
}

func (self *Mentions) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	ps := self.t.resolve(core, words, "Mentions of> ")
	res := self.query(core, ps)

	if self.PaneAt > 0 {
		self.pane(res)
		return
	}
	if !self.Link {
		self.list(res)
		return
	}
	self.linkThem(core, ps, res)
}

func (self *Mentions) list(res common.MentionsResult) {
	if commands.JsonOut {
		commands.RenderOne(res, nil)
		return
	}
	if commands.Render(res.Mentions, nil) {
		return
	}
	fmt.Println(targetLine(res.Target))
	self.searched(res)
	if len(res.Mentions) == 0 {
		fmt.Printf("%sno unlinked mentions%s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset))
		return
	}
	for i, m := range res.Mentions {
		fmt.Println(mentionLine(i+1, m))
	}
	if res.Truncated {
		fmt.Printf("%s… %d of %d shown; -limit for more%s\n", commands.C(commands.AnsiDim), len(res.Mentions), res.Total, commands.C(commands.AnsiReset))
	}
}

// What was looked for, and what was not and why - said once under the title,
// because "no mentions" of a heading called Notes means something different
// from "no mentions" of one called Apollo.
func (self *Mentions) searched(res common.MentionsResult) {
	if len(res.Target.Terms) > 0 {
		fmt.Printf("%ssearched for: %s%s\n", commands.C(commands.AnsiDim), strings.Join(res.Target.Terms, ", "), commands.C(commands.AnsiReset))
	}
	for _, s := range res.Target.Skipped {
		fmt.Printf("%snot searched: %s (-common to search anyway)%s\n", commands.C(commands.AnsiGold), s, commands.C(commands.AnsiReset))
	}
}

// The mentions to link: a list of numbers, everything with -yes, or a picker.
func (self *Mentions) choose(core *commands.Core, ps map[string]string, res common.MentionsResult) []common.Mention {
	if self.Only != "" {
		var out []common.Mention
		for _, f := range strings.FieldsFunc(self.Only, func(r rune) bool { return r == ',' || r == ' ' }) {
			n, err := strconv.Atoi(f)
			if err != nil || n < 1 || n > len(res.Mentions) {
				commands.Fail("-only %s: there are %d mentions", f, len(res.Mentions))
			}
			out = append(out, res.Mentions[n-1])
		}
		return out
	}
	if self.t.tf.Yes {
		return res.Mentions
	}
	if !commands.Interactive() {
		commands.Fail("%d mentions. -only 1,3 to link some, -yes to link every one", len(res.Mentions))
	}
	lines := make([]string, len(res.Mentions))
	for i, m := range res.Mentions {
		lines[i] = commands.PickLine([]string{strconv.Itoa(i + 1)}, mentionLine(i+1, m))
	}
	preview := ""
	if self_, err := commands.SelfCommand(core); err == nil {
		preview = self_ + " mentions" + paneArgs(ps, self.Common, self.Limit) + " -pane-at {1}"
	}
	commands.PickerOutput()
	sel := commands.Pick(commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "Link> ",
		Header:        "tab marks several · ctrl-a marks all · enter links them",
		Preview:       preview,
		Extra:         []string{"--multi", "--bind", "ctrl-a:select-all"},
	})
	var out []common.Mention
	for _, line := range sel {
		if addr, ok := commands.Address(line, 1); ok {
			if n, err := strconv.Atoi(addr[0]); err == nil && n >= 1 && n <= len(res.Mentions) {
				out = append(out, res.Mentions[n-1])
			}
		}
	}
	return out
}

// The flags that make the pane's run of the query the same as this one.
func paneArgs(ps map[string]string, common bool, limit int) string {
	s := ""
	if h := ps["hash"]; h != "" {
		s += " -hash " + commands.Shq(h)
	} else {
		s += " -file " + commands.Shq(ps["filename"])
		if l, ok := ps["line"]; ok {
			n, _ := strconv.Atoi(l)
			s += fmt.Sprintf(" -line %d", n+1)
		}
	}
	if common {
		s += " -common"
	}
	s += fmt.Sprintf(" -limit %d", limit)
	if commands.NoColor {
		s += " -no-color"
	}
	return s
}

func (self *Mentions) linkThem(core *commands.Core, ps map[string]string, res common.MentionsResult) {
	if len(res.Mentions) == 0 {
		if commands.JsonOut {
			commands.RenderOne(common.MentionLinkResult{Ok: true, Msg: "no unlinked mentions"}, nil)
			return
		}
		fmt.Println(targetLine(res.Target))
		self.searched(res)
		fmt.Printf("%sno unlinked mentions%s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset))
		return
	}
	chosen := self.choose(core, ps, res)
	if len(chosen) == 0 {
		commands.Fail("nothing chosen")
	}
	req := common.MentionLinkRequest{Hash: res.Target.Hash}
	if res.Target.Kind == "file" {
		req.Filename = res.Target.Filename
	}
	for _, m := range chosen {
		req.Mentions = append(req.Mentions, common.MentionRef{Filename: m.Filename, Line: m.Line, Col: m.Col, Text: m.Text})
	}
	out, err := commands.SendReceivePostErr[common.MentionLinkRequest, common.MentionLinkResult](core, "mentions/link", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("orgs mentions: %v", err)
	}
	if commands.JsonOut {
		commands.RenderOne(out, nil)
		if !out.Ok {
			os.Exit(1)
		}
		return
	}
	if commands.Render(out.Results, nil) {
		return
	}
	if out.NewId {
		fmt.Printf("%s+%s gave %s an id: %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), res.Target.Title, out.Id)
	}
	for _, r := range out.Results {
		where := fmt.Sprintf("%s:%d", commands.BaseName(r.Filename), r.Line+1)
		if r.Ok {
			fmt.Printf("%s✓%s %s  %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), where, r.Link)
		} else {
			fmt.Printf("%s✗%s %s  %s\n", commands.C(commands.AnsiRed), commands.C(commands.AnsiReset), where, r.Msg)
		}
	}
	if !out.Ok {
		commands.Fail("%s", out.Msg)
	}
}

// The picker's pane: the lines around one mention, from the file here when it
// can be read, with the mention picked out.
func (self *Mentions) pane(res common.MentionsResult) {
	commands.PickerOutput()
	if self.PaneAt < 1 || self.PaneAt > len(res.Mentions) {
		fmt.Println("that mention is gone - the list has changed")
		return
	}
	m := res.Mentions[self.PaneAt-1]
	head := strings.Join(m.Olp, " › ")
	if head == "" {
		head = "(top of file)"
	}
	fmt.Printf("%s%s%s\n%s%s:%d%s\n\n", commands.C(commands.AnsiBold), head, commands.C(commands.AnsiReset),
		commands.C(commands.AnsiDim), m.Filename, m.Line+1, commands.C(commands.AnsiReset))
	b, err := os.ReadFile(m.Filename)
	if err != nil {
		// The server may be another machine. The line itself still came back.
		fmt.Println(m.Before + commands.C(commands.AnsiBold+commands.AnsiGold) + m.Text + commands.C(commands.AnsiReset) + m.After)
		return
	}
	lines := strings.Split(string(b), "\n")
	from, to := m.Line-6, m.Line+8
	if from < 0 {
		from = 0
	}
	if to > len(lines) {
		to = len(lines)
	}
	for i := from; i < to; i++ {
		l := lines[i]
		mark := "  "
		if i == m.Line && m.Col+len(m.Text) <= len(l) {
			mark = commands.C(commands.AnsiGold) + "▶ " + commands.C(commands.AnsiReset)
			l = l[:m.Col] + commands.C(commands.AnsiBold+commands.AnsiGold) + l[m.Col:m.Col+len(m.Text)] + commands.C(commands.AnsiReset) + l[m.Col+len(m.Text):]
		}
		fmt.Printf("%s%s%4d%s %s\n", mark, commands.C(commands.AnsiDim), i+1, commands.C(commands.AnsiReset), l)
	}
	if m.InHeadline {
		fmt.Printf("\n%sin a headline: linking it changes that heading's hash%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	}
}

func init() {
	commands.AddCmd("backlinks", "what links to a heading or file, and what it links to",
		func() commands.Cmd { return &Backlinks{} })
	commands.AddCmd("mentions", "where a heading or file is named but not linked; -link to link them",
		func() commands.Cmd { return &Mentions{Limit: 500} })
}
