package palette

// orgs p - everything this tool can do, in one fuzzy list.
//
//	orgs p                 pick a command, a saved query, a filter or an alias
//	orgs                   the same, on a terminal (help everywhere else)
//	orgs p -print          print the chosen command line instead of running it
//	orgs p -json           the list, for a program
//
// There are about a hundred commands, and `orgs help` is a page to read rather
// than a thing to search. worg answers the same problem with its palette; this
// is that palette for the terminal, over the things a terminal can run: the
// commands, the queries saved in worg's search tab (`orgs q`), the filters in
// the yaml, and the aliases.
//
// Choosing a command asks for the rest of its line, with its usage above the
// prompt, because almost every command wants words and guessing which ones do
// would be wrong half the time. alt-enter runs it bare. ctrl-y (or -print)
// prints the line instead, which is what a shell widget wants:
//
//	orgs-palette() { LBUFFER+="$(orgs p -print </dev/tty)"; zle reset-prompt }
//	zle -N orgs-palette; bindkey '^o' orgs-palette
//
// The chosen line is run as a child of this binary with -config and -url
// passed through (SelfCommand), so under -local it reaches the server this
// process is holding open rather than starting another.

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
	"gopkg.in/yaml.v2"
)

type Palette struct {
	Print bool
}

func (self *Palette) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Palette) StartPlugin(m *common.PluginManager)       {}
func (self *Palette) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Print, "print", false, "print the chosen command line rather than running it")
}

// Entry is one row of the palette.
type Entry struct {
	// command, query, filter or alias.
	Kind  string
	Name  string
	Group string `json:",omitempty"`
	// The command's usage, the query's text, the alias's expansion.
	Usage string
	// What follows `orgs` to run it.
	Line []string
}

const (
	kindCommand = "command"
	kindQuery   = "query"
	kindFilter  = "filter"
	kindAlias   = "alias"
)

func (self *Palette) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("p").Flags)
	if len(words) > 0 && words[0] == "preview" {
		preview(core, words[1:])
		return
	}
	// Under -print stdout is the shell's $(…), not a terminal, but fzf draws on
	// the tty and there is still somebody to choose.
	interactive := commands.Interactive() ||
		(self.Print && !commands.Machine() && term.IsTerminal(int(os.Stdin.Fd())))
	if self.Print && interactive {
		commands.PickerOutput()
	}
	entries := gather(core)
	if !interactive {
		commands.Render(entries, func() {
			for _, e := range entries {
				fmt.Printf("%-8s %-20s %s\n", e.Kind, e.Name, e.Usage)
			}
		})
		return
	}
	self.pick(core, entries)
}

// ---------------------------------------------------------------------------
// What is in it
// ---------------------------------------------------------------------------

func gather(core *commands.Core) []Entry {
	out := []Entry{}

	// Yours first: an alias or a saved query is something somebody chose to
	// keep, which makes it likelier to be what they are looking for.
	aliases := readAliases(core.ConfigFile)
	names := make([]string, 0, len(aliases))
	for k := range aliases {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		out = append(out, Entry{Kind: kindAlias, Name: k, Group: "Your aliases",
			Usage: strings.Join(aliases[k], " "), Line: []string{k}})
	}

	type storedQuery struct {
		Name  string `json:"name"`
		Query string `json:"query"`
	}
	qs := commands.SendReceiveGetOr[[]storedQuery](core, "ext/queries", nil)
	sort.Slice(qs, func(i, j int) bool { return qs[i].Name < qs[j].Name })
	for _, q := range qs {
		out = append(out, Entry{Kind: kindQuery, Name: q.Name, Group: "Saved queries",
			Usage: q.Query, Line: []string{"q", q.Name}})
	}

	// A filter in the yaml is also registered as a command of its own, which
	// opens a table in tview. As a row here it is a search, the same as worg
	// runs it, and the command row is left out so it does not appear twice.
	filters := commands.SendReceiveGetOr[map[string]string](core, "filters", nil)
	fnames := make([]string, 0, len(filters))
	for k := range filters {
		fnames = append(fnames, k)
	}
	sort.Strings(fnames)
	for _, k := range fnames {
		out = append(out, Entry{Kind: kindFilter, Name: k, Group: "Filters",
			Usage: filters[k], Line: []string{"search", "{{" + k + "}}"}})
	}

	for _, r := range helpRows() {
		if _, isFilter := filters[r.Name]; isFilter {
			continue
		}
		if r.Name == "p" || r.Name == "palette" {
			continue
		}
		out = append(out, Entry{Kind: kindCommand, Name: r.Name, Group: r.Group,
			Usage: r.Usage, Line: []string{r.Name}})
	}
	return out
}

type helpRow struct {
	Name  string
	Group string
	Usage string
}

// helpRows is `orgs help -json`, asked in process.
//
// The groups are a table in the help command and nowhere else, and the order
// they come in is the order worth reading. Asking help for them keeps that in
// one place; the cost is borrowing stdout for the length of the call. Falls
// back to the registry, ungrouped, if that goes wrong.
func helpRows() []helpRow {
	rows := []helpRow{}
	if h := commands.Find("help"); h != nil && h.Flags != nil {
		if data, err := captureStdout(func() {
			saved := commands.JsonOut
			commands.JsonOut = true
			defer func() { commands.JsonOut = saved }()
			h.Flags.Parse(nil)
			h.Cmd.Exec(nil)
		}); err == nil && json.Unmarshal(data, &rows) == nil && len(rows) > 0 {
			return rows
		}
	}
	for name, c := range commands.CmdRegistry {
		if strings.HasPrefix(name, "__") {
			continue
		}
		rows = append(rows, helpRow{Name: name, Group: "Other", Usage: c.Usage})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

func captureStdout(fn func()) ([]byte, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	saved := os.Stdout
	os.Stdout = w
	got := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		got <- b
	}()
	func() {
		defer func() { os.Stdout = saved }()
		fn()
	}()
	w.Close()
	return <-got, nil
}

// readAliases reads `aliases:` from the config file. The command packages
// cannot import the settings (see the import cycle in CLAUDE.md), and the
// aliases are the one part of them this needs.
func readAliases(path string) map[string][]string {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg struct {
		Aliases map[string][]string `yaml:"aliases"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return nil
	}
	return cfg.Aliases
}

// ---------------------------------------------------------------------------
// The picker
// ---------------------------------------------------------------------------

var glyph = map[string]string{
	kindCommand: " ",
	kindQuery:   "?",
	kindFilter:  "⚑",
	kindAlias:   "→",
}

func (self *Palette) pick(core *commands.Core, entries []Entry) {
	width := 0
	for _, e := range entries {
		if n := commands.RuneLen(e.Name); n > width {
			width = n
		}
	}
	byAddr := map[string]Entry{}
	lines := []string{}
	for _, e := range entries {
		byAddr[e.Kind+"\x00"+e.Name] = e
		pad := strings.Repeat(" ", width-commands.RuneLen(e.Name))
		lines = append(lines, commands.PickLine([]string{e.Kind, e.Name},
			fmt.Sprintf("%s %s%s%s%s  %s%s%s  %s",
				glyph[e.Kind], commands.C(commands.AnsiBold), e.Name, commands.C(commands.AnsiReset), pad,
				commands.C(commands.AnsiDim), commands.Ellipsis(e.Group, 24), commands.C(commands.AnsiReset),
				e.Usage)))
	}
	preview := ""
	if me, err := commands.SelfCommand(core); err == nil {
		preview = me + " p preview {1} {2}"
		if commands.NoColor {
			preview += " -no-color"
		}
	}
	key, sel := commands.PickKey(commands.PickOpts{
		Lines:         lines,
		AddressFields: 2,
		Prompt:        "orgs> ",
		Header:        "enter run · alt-enter run bare · ctrl-y print the line · ctrl-/ pane",
		Preview:       preview,
	}, "alt-enter", "ctrl-y")
	if len(sel) == 0 {
		return
	}
	addr, ok := commands.Address(sel[0], 2)
	if !ok {
		return
	}
	e, ok := byAddr[addr[0]+"\x00"+addr[1]]
	if !ok {
		return
	}
	line := append([]string{}, e.Line...)

	if self.Print || key == "ctrl-y" {
		// A trailing space after a command, so the widget leaves the cursor
		// where its words go.
		s := "orgs " + shellJoin(line)
		if e.Kind == kindCommand {
			s += " "
		}
		fmt.Print(s)
		return
	}
	if e.Kind == kindCommand && key != "alt-enter" {
		more, err := askWords(e)
		if err != nil {
			return
		}
		line = append(line, more...)
	}
	run(core, line)
}

// askWords reads the rest of a command's line, with what it takes above the
// prompt.
func askWords(e Entry) ([]string, error) {
	fmt.Fprintf(os.Stderr, "%s%s%s  %s\n", commands.C(commands.AnsiBold), e.Name,
		commands.C(commands.AnsiReset), e.Usage)
	fmt.Fprintf(os.Stderr, "%sorgs help %s for its flags · enter on its own runs it bare · ctrl-d cancels%s\n",
		commands.C(commands.AnsiDim), e.Name, commands.C(commands.AnsiReset))
	fmt.Fprintf(os.Stderr, "%sorgs %s%s ", commands.C(commands.AnsiCyan), e.Name, commands.C(commands.AnsiReset))
	text, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && text == "" {
		fmt.Fprintln(os.Stderr)
		return nil, err
	}
	words, err := splitWords(text)
	if err != nil {
		commands.Fail("%v", err)
	}
	return words, nil
}

// run starts this binary again on the chosen line and leaves with its exit
// code. A child rather than an exec, because under -local this process is the
// server the child is about to talk to.
func run(core *commands.Core, line []string) {
	exe, err := os.Executable()
	if err != nil {
		commands.Fail("%v", err)
	}
	args := []string{}
	if core.ConfigFile != "" {
		args = append(args, "-config", core.ConfigFile)
	}
	if core.Rest.Url != "" {
		args = append(args, "-url", core.Rest.Url)
	}
	if commands.NoColor {
		// -no-color is a command flag, so it goes after the command.
		line = append(line, "-no-color")
	}
	args = append(args, line...)
	cmd := exec.Command(exe, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		commands.Fail("%v", err)
	}
}

// ---------------------------------------------------------------------------
// The pane
// ---------------------------------------------------------------------------

// preview draws the pane for one row: `orgs p preview <kind> <name>`.
func preview(core *commands.Core, words []string) {
	commands.PickerOutput()
	if len(words) < 2 {
		return
	}
	kind, name := words[0], words[1]
	switch kind {
	case kindCommand:
		showHelp(name)
	case kindAlias:
		exp := readAliases(core.ConfigFile)[name]
		fmt.Printf("%s%s%s  →  orgs %s\n", commands.C(commands.AnsiBold), name,
			commands.C(commands.AnsiReset), shellJoin(exp))
		if len(exp) > 0 {
			fmt.Println()
			showHelp(exp[0])
		}
	case kindQuery, kindFilter:
		query := "{{" + name + "}}"
		if kind == kindQuery {
			type storedQuery struct {
				Name  string `json:"name"`
				Query string `json:"query"`
			}
			for _, q := range commands.SendReceiveGetOr[[]storedQuery](core, "ext/queries", nil) {
				if q.Name == name {
					query = q.Query
				}
			}
		}
		fmt.Printf("%s%s%s\n\n", commands.C(commands.AnsiDim), query, commands.C(commands.AnsiReset))
		todos, err := commands.SendReceiveGetErr[common.Todos](core, "search", map[string]string{"query": query})
		if err != nil {
			fmt.Printf("could not ask the server: %v\n", err)
			return
		}
		width := commands.PaneWidth() - 2
		for i, t := range todos {
			if i == 40 {
				fmt.Printf("%s… %d more%s\n", commands.C(commands.AnsiDim), len(todos)-i, commands.C(commands.AnsiReset))
				break
			}
			status := ""
			if t.Status != "" {
				status = commands.C(commands.AnsiGold) + t.Status + commands.C(commands.AnsiReset) + " "
			}
			fmt.Printf("%s%s  %s%s%s\n", status, commands.Ellipsis(t.Headline, width/2),
				commands.C(commands.AnsiDim), commands.BaseName(t.Filename), commands.C(commands.AnsiReset))
		}
		if len(todos) == 0 {
			fmt.Println("nothing matches it right now")
		}
	}
}

func showHelp(name string) {
	h := commands.Find("help")
	if h == nil || h.Flags == nil {
		return
	}
	if commands.Find(name) == nil {
		fmt.Printf("no command called %s\n", name)
		return
	}
	h.Flags.Parse([]string{name})
	h.Cmd.Exec(nil)
}

// ---------------------------------------------------------------------------
// Words
// ---------------------------------------------------------------------------

// splitWords splits a typed line the way a shell would, near enough: single
// quotes literal, double quotes with backslash escapes, backslash outside
// quotes. A query is the usual thing typed here and is full of both.
func splitWords(s string) ([]string, error) {
	out := []string{}
	var cur strings.Builder
	have := false
	var quote rune
	esc := false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case quote == '"':
			if r == '"' {
				quote = 0
			} else if r == '\\' {
				esc = true
			} else {
				cur.WriteRune(r)
			}
		case r == '\\':
			esc, have = true, true
		case r == '\'' || r == '"':
			quote, have = r, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if have {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("a %c quote is not closed", quote)
	}
	if have {
		out = append(out, cur.String())
	}
	return out, nil
}

// shellJoin is the other direction, quoting only the words that need it so
// the printed line reads the way somebody would have typed it.
func shellJoin(words []string) string {
	parts := make([]string, len(words))
	for i, w := range words {
		if w != "" && !strings.ContainsAny(w, " \t\n'\"\\$`!*?[](){}<>|&;#~") {
			parts[i] = w
		} else {
			parts[i] = commands.Shq(w)
		}
	}
	return strings.Join(parts, " ")
}

func init() {
	mk := func() commands.Cmd { return &Palette{} }
	commands.AddCmd("p", "every command, saved query, filter and alias in one fuzzy list", mk)
}
