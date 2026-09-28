package gitorg

// orgs diff and orgs blame - git, read as org rather than as text.
//
//	orgs diff                    what changed since the last commit
//	orgs diff HEAD~5             since five commits ago
//	orgs diff -staged            what is about to be committed
//	orgs blame 'the migration'   when that heading became what it is
//
// A text diff of an outline is close to unreadable: refiling one heading moves
// forty lines, and the one thing you want to know - *which headings changed, and
// how* - is what a line diff is least able to say. Nothing else in the org
// ecosystem does this well, and the parser is right here.
//
// Two things make it worth having rather than clever:
//
//  1. **It is about headings, not lines.** A heading that moved has not changed;
//     a heading whose keyword went from NEXT to DONE has, and that is one line
//     of output rather than a hunk. What it reports is what you would tell
//     somebody: three finished, one new, one rescheduled.
//  2. **It needs no server.** Both revisions are parsed here, so this works in a
//     pre-commit hook, in CI, and on a checkout of somebody else's org files -
//     none of which has a server watching them.

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// One heading, flattened out of a parsed document, keyed by its outline path.
// The path is the identity rather than the line or a hash: a heading that moved
// is the same heading, which is the whole point of reading a diff this way.
type heading struct {
	Path     string
	Headline string
	Status   string
	Priority string
	Tags     []string
	Props    map[string]string
	Sched    string
	Deadline string
	Line     int
	// A cheap summary of the body, so "the text under this changed" can be
	// reported without diffing it.
	Body string
}

// ---------------------------------------------------------------------------
// orgs diff
// ---------------------------------------------------------------------------

type Diff struct {
	Staged bool
	Stat   bool
}

func (self *Diff) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Diff) StartPlugin(m *common.PluginManager)       {}
func (self *Diff) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Staged, "staged", false, "what is staged, rather than the working tree")
	fset.BoolVar(&self.Stat, "stat", false, "just the counts")
}

// NeedsNoServer: both revisions are parsed here.
func (self *Diff) NeedsNoServer() bool { return true }

// A change to one heading. Kind is what happened; the rest says what to.
type Change struct {
	Kind     string // added, removed, status, scheduled, deadline, tags, renamed, body
	File     string
	Path     string
	Headline string
	From     string `json:",omitempty"`
	To       string `json:",omitempty"`
	Line     int
}

func (self *Diff) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("diff").Flags)
	rev := "HEAD"
	if len(words) > 0 {
		rev = words[0]
	}
	if !inGitRepo() {
		commands.Fail("orgs diff: not in a git repository")
	}

	files := changedOrgFiles(rev, self.Staged)
	if len(files) == 0 {
		if !commands.Machine() {
			fmt.Printf("%sno org files changed%s\n", commands.C(commands.AnsiDim),
				commands.C(commands.AnsiReset))
		}
		commands.Render([]Change{}, nil)
		return
	}

	changes := []Change{}
	for _, f := range files {
		before := parseRev(rev, f)
		after := parseWorking(f, self.Staged)
		changes = append(changes, compare(f, before, after)...)
	}

	if commands.Render(changes, nil) {
		return
	}
	self.print(changes, rev)
}

func (self *Diff) print(changes []Change, rev string) {
	if len(changes) == 0 {
		fmt.Printf("%sthe org files changed, but no heading did - whitespace or formatting%s\n",
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		return
	}

	counts := map[string]int{}
	for _, c := range changes {
		counts[c.Kind]++
	}
	if self.Stat {
		kinds := []string{}
		for k := range counts {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			fmt.Printf("  %s%-10s%s %d\n", commands.C(commands.AnsiBold), k,
				commands.C(commands.AnsiReset), counts[k])
		}
		return
	}

	file := ""
	for _, c := range changes {
		if c.File != file {
			file = c.File
			fmt.Printf("\n%s%s%s\n", commands.C(commands.AnsiBold), commands.BaseName(file),
				commands.C(commands.AnsiReset))
		}
		fmt.Printf("  %s%s%s %s", commands.C(ink(c.Kind)), mark(c.Kind),
			commands.C(commands.AnsiReset), c.Headline)
		switch {
		case c.From != "" && c.To != "":
			fmt.Printf("  %s%s → %s%s", commands.C(commands.AnsiDim), c.From, c.To,
				commands.C(commands.AnsiReset))
		case c.To != "":
			fmt.Printf("  %s%s%s", commands.C(commands.AnsiDim), c.To,
				commands.C(commands.AnsiReset))
		case c.From != "":
			fmt.Printf("  %swas %s%s", commands.C(commands.AnsiDim), c.From,
				commands.C(commands.AnsiReset))
		}
		fmt.Println()
	}

	// The sentence somebody would say, which is the thing a line diff cannot
	// produce: "three finished, one new".
	fmt.Printf("\n%s%s since %s%s\n", commands.C(commands.AnsiDim), summary(counts), rev,
		commands.C(commands.AnsiReset))
}

func summary(counts map[string]int) string {
	parts := []string{}
	for _, k := range []string{"added", "removed", "status", "scheduled", "deadline",
		"tags", "renamed", "body"} {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
		}
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}

// compare is the whole of the diff: two maps of outline path to heading.
func compare(file string, before, after map[string]heading) []Change {
	out := []Change{}
	paths := map[string]bool{}
	for p := range before {
		paths[p] = true
	}
	for p := range after {
		paths[p] = true
	}
	ordered := []string{}
	for p := range paths {
		ordered = append(ordered, p)
	}
	// In the order they appear in the new file, so a diff reads down the file
	// rather than alphabetically.
	sort.Slice(ordered, func(i, j int) bool {
		a, aok := after[ordered[i]]
		b, bok := after[ordered[j]]
		if aok && bok {
			return a.Line < b.Line
		}
		if aok != bok {
			return aok
		}
		return before[ordered[i]].Line < before[ordered[j]].Line
	})

	for _, p := range ordered {
		b, hadBefore := before[p]
		a, hasAfter := after[p]
		switch {
		case !hadBefore:
			out = append(out, Change{Kind: "added", File: file, Path: p,
				Headline: a.Headline, To: a.Status, Line: a.Line})
			continue
		case !hasAfter:
			out = append(out, Change{Kind: "removed", File: file, Path: p,
				Headline: b.Headline, From: b.Status, Line: b.Line})
			continue
		}
		add := func(kind, from, to string) {
			out = append(out, Change{Kind: kind, File: file, Path: p, Headline: a.Headline,
				From: from, To: to, Line: a.Line})
		}
		if b.Status != a.Status {
			add("status", orNone(b.Status), orNone(a.Status))
		}
		if b.Headline != a.Headline {
			add("renamed", b.Headline, a.Headline)
		}
		if b.Sched != a.Sched {
			add("scheduled", orNone(b.Sched), orNone(a.Sched))
		}
		if b.Deadline != a.Deadline {
			add("deadline", orNone(b.Deadline), orNone(a.Deadline))
		}
		if tagStr(b.Tags) != tagStr(a.Tags) {
			add("tags", orNone(tagStr(b.Tags)), orNone(tagStr(a.Tags)))
		}
		if b.Body != a.Body {
			// Not what changed - that is what a text diff is for - only that
			// something under the heading did.
			add("body", "", "")
		}
	}
	return out
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func tagStr(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	t := append([]string{}, tags...)
	sort.Strings(t)
	return strings.Join(t, ":")
}

// ---------------------------------------------------------------------------
// orgs blame
// ---------------------------------------------------------------------------

type Blame struct {
	File  string
	Limit int
}

func (self *Blame) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Blame) StartPlugin(m *common.PluginManager)       {}
func (self *Blame) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.File, "file", "", "the org file to look through")
	fset.IntVar(&self.Limit, "n", 40, "at most this many commits back")
}

func (self *Blame) NeedsNoServer() bool { return true }

type Moment struct {
	Commit   string
	When     string
	Who      string
	Subject  string
	Kind     string
	From     string `json:",omitempty"`
	To       string `json:",omitempty"`
	Headline string
}

func (self *Blame) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("blame").Flags)
	if len(words) == 0 {
		commands.Fail("orgs blame: say which heading.\n\n%s", blameUsage)
	}
	if !inGitRepo() {
		commands.Fail("orgs blame: not in a git repository")
	}
	want := strings.ToLower(strings.Join(words, " "))

	files := []string{self.File}
	if self.File == "" {
		files = orgFilesInRepo()
	}
	if len(files) == 0 {
		commands.Fail("no org files in this repository")
	}

	moments := []Moment{}
	for _, f := range files {
		commits := commitsTouching(f, self.Limit)
		if len(commits) == 0 {
			continue
		}
		// Walked oldest first, so each step is "what this commit did".
		var prev map[string]heading
		first := true
		for i := len(commits) - 1; i >= 0; i-- {
			c := commits[i]
			now := parseRev(c.Hash, f)
			if first {
				prev, first = now, false
				for p, h := range now {
					if matches(h, want) {
						moments = append(moments, Moment{Commit: short(c.Hash), When: c.When,
							Who: c.Who, Subject: c.Subject, Kind: "first seen",
							To: h.Status, Headline: h.Headline})
					}
					_ = p
				}
				continue
			}
			for _, ch := range compare(f, prev, now) {
				h, ok := now[ch.Path]
				if !ok {
					h = prev[ch.Path]
				}
				if !matches(h, want) {
					continue
				}
				moments = append(moments, Moment{Commit: short(c.Hash), When: c.When,
					Who: c.Who, Subject: c.Subject, Kind: ch.Kind, From: ch.From, To: ch.To,
					Headline: ch.Headline})
			}
			prev = now
		}
	}

	if len(moments) == 0 {
		commands.Fail("nothing in the history matched %q - try -file, or -n for more commits", want)
	}
	commands.Render(moments, func() {
		for _, m := range moments {
			what := m.Kind
			switch {
			case m.From != "" && m.To != "":
				what = fmt.Sprintf("%s %s%s → %s%s", m.Kind, commands.C(commands.AnsiDim),
					m.From, m.To, commands.C(commands.AnsiReset))
			case m.To != "":
				// A first sighting has nothing to have come from, and "(none) →
				// NEXT" reads as a change that did not happen.
				what = fmt.Sprintf("%s %s%s%s", m.Kind, commands.C(commands.AnsiDim), m.To,
					commands.C(commands.AnsiReset))
			case m.From != "":
				what = fmt.Sprintf("%s %swas %s%s", m.Kind, commands.C(commands.AnsiDim), m.From,
					commands.C(commands.AnsiReset))
			}
			fmt.Printf("%s%s%s  %s%-12s%s %-28s %s\n", commands.C(commands.AnsiGold), m.Commit,
				commands.C(commands.AnsiReset), commands.C(commands.AnsiDim),
				m.When, commands.C(commands.AnsiReset), what,
				commands.Ellipsis(m.Subject, 40))
		}
	})
}

func matches(h heading, want string) bool {
	return strings.Contains(strings.ToLower(h.Headline), want) ||
		strings.Contains(strings.ToLower(h.Path), want)
}

const blameUsage = `  orgs blame 'the migration'       when that heading became what it is
  orgs blame invoice -file gtd.org
  orgs blame 'the migration' -n 200  further back`

// ---------------------------------------------------------------------------
// git, and the parsing
// ---------------------------------------------------------------------------

type commit struct {
	Hash    string
	When    string
	Who     string
	Subject string
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	return string(out), err
}

func inGitRepo() bool {
	_, err := git("rev-parse", "--git-dir")
	return err == nil
}

func changedOrgFiles(rev string, staged bool) []string {
	args := []string{"diff", "--name-only", rev}
	if staged {
		args = []string{"diff", "--name-only", "--cached", rev}
	}
	out, err := git(args...)
	if err != nil {
		commands.Fail("git: %v - is %q a revision?", err, rev)
	}
	files := []string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasSuffix(line, ".org") {
			files = append(files, line)
		}
	}
	sort.Strings(files)
	return files
}

func orgFilesInRepo() []string {
	out, err := git("ls-files", "*.org")
	if err != nil {
		return nil
	}
	files := []string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasSuffix(line, ".org") {
			files = append(files, line)
		}
	}
	return files
}

func commitsTouching(file string, limit int) []commit {
	out, err := git("log", fmt.Sprintf("-%d", limit), "--format=%H%x00%ad%x00%an%x00%s",
		"--date=short", "--", file)
	if err != nil {
		return nil
	}
	commits := []commit{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.Split(line, "\x00")
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, commit{parts[0], parts[1], parts[2], parts[3]})
	}
	return commits
}

func short(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// parseRev is one org file as it was at a revision. A file that did not exist
// then is an empty document rather than an error - which is what makes every
// heading in a new file read as "added".
func parseRev(rev, file string) map[string]heading {
	out, err := git("show", rev+":"+file)
	if err != nil {
		return map[string]heading{}
	}
	return parseOrg(out)
}

func parseWorking(file string, staged bool) map[string]heading {
	if staged {
		out, err := git("show", ":"+file)
		if err != nil {
			return map[string]heading{}
		}
		return parseOrg(out)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return map[string]heading{}
	}
	return parseOrg(string(b))
}

// parseOrg flattens a document into headings keyed by outline path.
//
// The parsing is done here rather than asked of the server, which is what lets
// this work in a hook: a pre-commit hook runs against a checkout, not against a
// database, and the version being compared to only exists inside git.
func parseOrg(text string) map[string]heading {
	cfg := org.New()
	// go-org's default keyword list is "TODO | DONE" and nothing else, so in a
	// file with no #+TODO line of its own every other keyword was read as part
	// of the headline text: "NEXT Chase the invoice" came out as a heading
	// called that, with no status. Which made every NEXT→DONE - the transition
	// a diff of org files is most often about - look like a rename.
	//
	// A file that declares its own #+TODO still wins: BufferSettings override
	// DefaultSettings, which is what this map is.
	cfg.DefaultSettings["TODO"] = defaultKeywords
	doc := cfg.Parse(strings.NewReader(text), "./")
	out := map[string]heading{}
	walk(doc.Nodes, nil, out)
	return out
}

// The keywords to assume for a file that does not say. Deliberately generous:
// reading a word as a keyword that was meant as text costs one heading's title
// looking short, and failing to read a real keyword costs every transition of
// it in the whole history.
const defaultKeywords = "TODO NEXT STARTED INPROGRESS IN-PROGRESS BLOCKED PAUSED WAITING " +
	"HOLD PHONE MEETING BACKLOG SOMEDAY | DONE CANCELLED CANCELED SHIPPED"

func walk(nodes []org.Node, path []string, out map[string]heading) {
	for _, n := range nodes {
		// A nested heading is a *org.Headline, never an org.Headline - the trap
		// that has bitten the link index, the code index and the tangler.
		var h *org.Headline
		switch v := n.(type) {
		case *org.Headline:
			h = v
		case org.Headline:
			hh := v
			h = &hh
		default:
			continue
		}
		var tb strings.Builder
		for _, t := range h.Title {
			tb.WriteString(t.String())
		}
		title := tb.String()
		here := append(append([]string{}, path...), title)
		key := strings.Join(here, "/")
		// A duplicate outline path is possible and rare; the second one gets a
		// suffix so it is not silently merged with the first.
		if _, clash := out[key]; clash {
			for i := 2; ; i++ {
				alt := fmt.Sprintf("%s#%d", key, i)
				if _, clash := out[alt]; !clash {
					key = alt
					break
				}
			}
		}
		props := map[string]string{}
		if h.Properties != nil {
			for _, p := range h.Properties.Properties {
				props[p[0]] = p[1]
			}
		}
		out[key] = heading{
			Path:     key,
			Headline: title,
			Status:   h.Status,
			Priority: h.Priority,
			Tags:     h.Tags,
			Props:    props,
			Sched:    dateOf(h.Scheduled),
			Deadline: dateOf(h.Deadline),
			Line:     h.Pos.Row,
			Body:     bodyOf(h),
		}
		walk(h.Children, here, out)
	}
}

func dateOf(sdc *org.SDC) string {
	if sdc == nil || sdc.Date == nil || sdc.Date.Start.IsZero() {
		return ""
	}
	return sdc.Date.ToString()
}

// bodyOf is a cheap stand-in for the text under a heading: enough to tell that
// it changed, and deliberately not enough to say how. Child headings are left
// out, because a change to one of those is that heading's own change.
func bodyOf(h *org.Headline) string {
	var sb strings.Builder
	for _, c := range h.Children {
		switch c.(type) {
		case *org.Headline, org.Headline:
			// A child heading's change is that heading's own change.
			continue
		case *org.SDC, org.SDC:
			// The planning line is reported as "scheduled" or "deadline" in its
			// own right; counting it here as well made every date change report
			// twice, once usefully and once as "something under this changed".
			continue
		}
		sb.WriteString(c.String())
		sb.WriteString("\n")
	}
	return sb.String()
}

func mark(kind string) string {
	switch kind {
	case "added":
		return "+"
	case "removed":
		return "-"
	case "status":
		return "→"
	case "renamed":
		return "✎"
	case "body":
		return "¶"
	}
	return "·"
}

func ink(kind string) string {
	switch kind {
	case "added":
		return commands.AnsiGreen
	case "removed":
		return commands.AnsiRed
	case "status":
		return commands.AnsiGold
	case "renamed":
		return commands.AnsiPurple
	}
	return commands.AnsiCyan
}

func init() {
	commands.AddCmd("diff", "what changed in your org files, read as headings rather than lines",
		func() commands.Cmd { return &Diff{} })
	commands.AddCmd("blame", "when a heading became what it is",
		func() commands.Cmd { return &Blame{} })
}
