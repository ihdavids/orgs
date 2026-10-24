package help

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// An example line's command and what it is for, set apart by a run of spaces:
//
//	orgs habits -due                only what wants doing today
var exampleSplitRe = regexp.MustCompile(`^(.*?\S)\s{2,}(\S.*)$`)

// The Querying text opens with an "Overview" heading, which says nothing
// under a heading that already says what the section is.
func queryingText() string {
	t := bundle().Querying
	lines := strings.SplitN(t, "\n", 2)
	if len(lines) == 2 && strings.EqualFold(strings.TrimSpace(strings.TrimLeft(lines[0], "*")), "overview") {
		return lines[1]
	}
	return t
}

// showDocs is what follows a command's flags in `orgs help <command>`.
func showDocs(c *commands.CmdCreatorThunk) {
	b := bundle()
	gold := func(s string) string {
		return commands.C(commands.AnsiGold) + s + commands.C(commands.AnsiReset)
	}
	if ex := b.Examples[c.Name]; len(ex) > 0 {
		fmt.Printf("\n%s\n", gold("Examples"))
		// What they are for in one column, as the package comment lines
		// them up - unless one command is long enough to push it off.
		cmdW := 0
		for _, e := range ex {
			for _, l := range strings.Split(e, "\n") {
				if m := exampleSplitRe.FindStringSubmatch(l); m != nil && len(m[1]) > cmdW && len(m[1]) <= 48 {
					cmdW = len(m[1])
				}
			}
		}
		for _, e := range ex {
			for i, l := range strings.Split(e, "\n") {
				pad := "  "
				if i > 0 {
					pad = "    "
				}
				if m := exampleSplitRe.FindStringSubmatch(l); m != nil {
					fmt.Printf("%s%s%-*s%s  %s%s%s\n", pad, commands.C(commands.AnsiCyan), cmdW, m[1], commands.C(commands.AnsiReset),
						commands.C(commands.AnsiDim), m[2], commands.C(commands.AnsiReset))
				} else {
					fmt.Printf("%s%s%s%s\n", pad, commands.C(commands.AnsiCyan), l, commands.C(commands.AnsiReset))
				}
			}
		}
	}
	for _, s := range b.Commands[c.Name] {
		fmt.Println()
		fmt.Print(renderTerm(s.Text, 0))
	}
	if takesQuery(c) {
		fmt.Printf("\n%s\n\n", gold("The query language"))
		fmt.Print(renderTerm(queryingText(), 2))
		fns := queryFuncs()
		fmt.Printf("\n  %sFunctions%s %s(%d)%s\n", commands.C(commands.AnsiBold), commands.C(commands.AnsiReset),
			commands.C(commands.AnsiDim), len(fns), commands.C(commands.AnsiReset))
		nameW := 0
		for _, f := range fns {
			if len(f.Name) > nameW {
				nameW = len(f.Name)
			}
		}
		w := width()
		for _, f := range fns {
			b := &strings.Builder{}
			first := fmt.Sprintf("%s%-*s%s  ", commands.C(commands.AnsiCyan), nameW, f.Name, commands.C(commands.AnsiReset))
			// Wrapped under its own text, not under the name.
			words := styled(f.Doc)
			wrapHanging(b, words, 4, first, 4+nameW+2, w)
			fmt.Print(b.String())
		}
	}
}

// wrapHanging is wrap with a first part (already styled) taking hang columns.
func wrapHanging(b *strings.Builder, words []word, indent int, first string, hang int, w int) {
	b.WriteString(strings.Repeat(" ", indent) + first)
	col := hang
	for i, wd := range words {
		if i > 0 {
			if col+1+wd.n > w {
				b.WriteString("\n" + strings.Repeat(" ", hang))
				col = hang
			} else {
				b.WriteString(" ")
				col++
			}
		}
		b.WriteString(wd.shown)
		col += wd.n
	}
	b.WriteString("\n")
}

// manDate is the build's date when it has one, so pages built from one
// release say the same day.
func manDate() string {
	if len(common.BuildDate) >= 10 {
		return common.BuildDate[:10]
	}
	return time.Now().Format("2006-01-02")
}

func manHeader(name, section string) string {
	return fmt.Sprintf(".TH \"%s\" \"%s\" \"%s\" \"orgs %s\" \"Orgs Manual\"\n", strings.ToUpper(roffEscape(name)),
		section, manDate(), roffEscape(common.Version))
}

// manPage is a command's page: what it is, its flags, its examples, what the
// documentation says about it and, when it takes one, the query language.
func manPage(c *commands.CmdCreatorThunk) string {
	b := &strings.Builder{}
	b.WriteString(manHeader("orgs-"+c.Name, "1"))
	b.WriteString(".SH NAME\norgs\\-" + roffEscape(c.Name) + " \\- " + roffEscape(c.Usage) + "\n")
	b.WriteString(".SH SYNOPSIS\n.B orgs\n.RI [ global\\ flags ]\n.B " + roffEscape(c.Name) + "\n.RI [ flags ]\n.RI [ args ...]\n")
	if c.Flags != nil {
		opts := &strings.Builder{}
		c.Flags.VisitAll(func(f *flag.Flag) {
			arg, usage := flag.UnquoteUsage(f)
			opts.WriteString(".TP\n.B \\-" + roffEscape(f.Name))
			if arg != "" {
				opts.WriteString(" \" \\fI" + roffEscape(arg) + "\\fR\"")
			}
			opts.WriteString("\n" + roffLine(roffEscape(usage)))
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
				opts.WriteString(" (default: " + roffEscape(f.DefValue) + ")")
			}
			opts.WriteString("\n")
		})
		if opts.Len() > 0 {
			b.WriteString(".SH OPTIONS\n" + opts.String())
		}
	}
	bd := bundle()
	if ex := bd.Examples[c.Name]; len(ex) > 0 {
		b.WriteString(".SH EXAMPLES\n.PP\n.RS 4\n.nf\n")
		for _, e := range ex {
			for _, l := range strings.Split(e, "\n") {
				b.WriteString(roffLine(roffEscape(l)) + "\n")
			}
		}
		b.WriteString(".fi\n.RE\n")
	}
	for _, s := range bd.Commands[c.Name] {
		b.WriteString(renderRoff(s.Text, 1))
	}
	if takesQuery(c) {
		b.WriteString(".SH \"QUERY LANGUAGE\"\n")
		b.WriteString(renderRoff(queryingText(), 0))
		b.WriteString(".SS Functions\n")
		for _, f := range queryFuncs() {
			b.WriteString(".TP\n.B " + roffEscape(f.Name) + "\n")
			if f.Doc != "" {
				b.WriteString(roffLine(roffInline(f.Doc, false)) + "\n")
			} else {
				b.WriteString("(not documented yet)\n")
			}
		}
	}
	b.WriteString(".SH \"SEE ALSO\"\n.BR orgs (1)\n")
	return tidyRoff(b.String())
}

var ppAfterHeadingRe = regexp.MustCompile(`(?m)^(\.S[HS] .*)\n\.PP\n`)

// tidyRoff drops a paragraph break straight after a heading, which starts a
// paragraph anyway (mandoc -T lint warns about every one).
func tidyRoff(s string) string {
	return ppAfterHeadingRe.ReplaceAllString(s, "$1\n")
}

// manIndex is orgs(1): how a command line is put together, and every
// command under its group.
func manIndex() string {
	b := &strings.Builder{}
	b.WriteString(manHeader("orgs", "1"))
	b.WriteString(".SH NAME\norgs \\- an org mode server and the command line that talks to it\n")
	b.WriteString(".SH SYNOPSIS\n.B orgs\n[\\fB\\-url\\fR \\fIURL\\fR] [\\fB\\-config\\fR \\fIFILE\\fR] [\\fB\\-local\\fR] \\fIcommand\\fR [\\fIflags\\fR]\n")
	b.WriteString(".SH DESCRIPTION\n.PP\n" +
		"One binary is both an org mode server - it watches your org files and answers a REST api, " +
		"which worg, the editor plugins and this command line talk to - and the command line itself. " +
		"The first word picks the command; its flags come after it.\n" +
		".PP\nEach command has a page of its own, \\fBorgs\\-\\fR\\fIcommand\\fR(1), and " +
		"\\fBorgs help\\fR \\fIcommand\\fR shows the same in a terminal. " +
		"\\fBorgs \\-local\\fR \\fIcommand\\fR runs a server in the process for one command, for hooks and scripts.\n")
	grouped, _ := helpGroups()
	b.WriteString(".SH COMMANDS\n")
	for _, g := range grouped {
		b.WriteString(".SS \"" + roffEscape(g.Name) + "\"\n")
		for _, name := range g.Names {
			b.WriteString(".TP\n.B " + roffEscape(name) + "\n" + roffLine(roffEscape(commands.Find(name).Usage)) + "\n")
		}
	}
	b.WriteString(".SH \"SEE ALSO\"\n")
	names := sortedNames()
	for i, name := range names {
		sep := ","
		if i == len(names)-1 {
			sep = ""
		}
		b.WriteString(".BR orgs\\-" + roffEscape(name) + " (1)" + sep + "\n")
	}
	return tidyRoff(b.String())
}

// writeManDir writes orgs.1 and every command's page into dir.
func writeManDir(dir string) {
	names := sortedNames()
	if commands.Wrote(fmt.Sprintf("write %d man pages into %s", len(names)+1, dir), nil) {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		commands.Fail("%v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "orgs.1"), []byte(manIndex()), 0644); err != nil {
		commands.Fail("%v", err)
	}
	// A filter called Projects and the projects command are one file on a
	// disk that ignores case (macOS, Windows): the lower case one - the
	// command, not a filter from the yaml - keeps the name.
	taken := map[string]string{}
	for _, name := range names {
		if name == strings.ToLower(name) {
			taken[name] = name
		}
	}
	wrote := 1
	for _, name := range names {
		if t, ok := taken[strings.ToLower(name)]; ok && t != name {
			fmt.Fprintf(os.Stderr, "no page for %s: it would be the same file as %s's\n", name, t)
			continue
		}
		taken[strings.ToLower(name)] = name
		if err := os.WriteFile(filepath.Join(dir, "orgs-"+name+".1"), []byte(manPage(commands.Find(name))), 0644); err != nil {
			commands.Fail("%v", err)
		}
		wrote++
	}
	fmt.Printf("wrote %d man pages into %s\n", wrote, dir)
	fmt.Fprintf(os.Stderr, "%sman orgs-search%s works once %s is on MANPATH (or under a man1 folder that is)\n",
		commands.C(commands.AnsiDim), commands.C(commands.AnsiReset), dir)
}
