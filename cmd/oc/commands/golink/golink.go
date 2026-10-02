package golink

// `orgs go <name>`: follow a link by what it is called.
//
//	orgs go                  a picker over every link with a description
//	orgs go go spec          the link whose description starts "go spec"
//	orgs go jira -print      say where it goes rather than going there
//	orgs go jira -json       the matches, for a program
//
// A link written `[[https://go.dev/ref/spec][Go spec]]` anywhere in the org
// files is a bookmark called "Go spec", so the description is the name.
// Only links that have one are offered: a bare url has no name to type.
//
// The name is a case-insensitive prefix of the description. One target
// matching it is followed straight away; more than one is a question, so the
// picker opens on just those. The same link pasted in three places is still
// one target, and a description that is the name exactly wins over the ones
// it is merely the start of, so `orgs go go` is not stopped by "go spec".
//
// Following means what the link means: a url goes to the desktop's opener
// (browser, mail client), a link into the org files goes to the editor at that
// heading, a `file:` link to a file on this machine goes to whatever opens
// that kind of file. Package golink because `go` is a keyword.
//
// A protocol defined under `linkProtocols` in the yaml (`jira:ABC-123`) goes
// where the yaml says: its url to the desktop's opener, or its command run
// here, in this terminal. A link like that can also be typed straight in -
// `orgs go jira:ABC-123` - without it being written in any org file.

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/links"
	"github.com/ihdavids/orgs/internal/common"
)

type Go struct {
	fset *flag.FlagSet

	Print bool
	// For the picker's pane, which is a second run of this command. A flag
	// rather than a subcommand word so that a link called "preview" can still
	// be gone to.
	PaneAt int
}

func (self *Go) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Go) StartPlugin(manager *common.PluginManager)         {}

func (self *Go) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Print, "print", false, "print where the link goes instead of following it")
	fset.IntVar(&self.PaneAt, "pane-at", -1, "draw the preview pane for this row (used by the picker)")
}

func (self *Go) Exec(core *commands.Core) {
	name := strings.TrimSpace(strings.Join(commands.FreeArgs(self.fset), " "))
	if self.PaneAt < 0 && self.direct(core, name) {
		return
	}
	rows := matches(core, name)

	switch {
	case self.PaneAt >= 0:
		commands.PickerOutput()
		if self.PaneAt >= len(rows) {
			fmt.Println("that link is not in the list any more")
			return
		}
		links.RenderPane(core, rows[self.PaneAt], commands.PaneWidth())
		return
	}

	if commands.JsonOut || commands.FormatOut != "" {
		commands.Render(rows, nil)
		return
	}
	if len(rows) == 0 {
		if name == "" {
			commands.Fail("orgs go: no link in the org files has a description")
		}
		commands.Fail("orgs go: no link has a description starting %q", name)
	}
	if name != "" {
		if one, ok := single(rows, name); ok {
			self.follow(core, one)
			return
		}
	}
	if !commands.Interactive() {
		// More than one, and nobody to ask.
		for _, l := range rows {
			fmt.Printf("%s\t%s\n", l.Desc, l.Raw)
		}
		commands.Fail("orgs go: %d links match %q; say more of the name", len(rows), name)
	}
	self.pick(core, rows, name)
}

// The links with a description starting with name, in the order the server
// listed them. The pane is a second process indexing into this same answer,
// so it must come out the same way every time it is asked.
func matches(core *commands.Core, name string) []common.LinkEntry {
	list, err := commands.SendReceiveGetErr[common.LinkList](core, "links/all", nil)
	if err != nil {
		commands.Fail("orgs go: %v", err)
	}
	want := strings.ToLower(name)
	out := []common.LinkEntry{}
	for _, l := range list.Links {
		d := strings.TrimSpace(l.Desc)
		if d == "" || d == l.Raw {
			continue
		}
		if strings.HasPrefix(strings.ToLower(d), want) {
			out = append(out, l)
		}
	}
	return out
}

// single is the one target the matches agree on, if they do: every match
// going to the same place, or exactly one place whose name is the name.
func single(rows []common.LinkEntry, name string) (common.LinkEntry, bool) {
	if l, ok := oneTarget(rows); ok {
		return l, true
	}
	exact := []common.LinkEntry{}
	for _, l := range rows {
		if strings.EqualFold(strings.TrimSpace(l.Desc), name) {
			exact = append(exact, l)
		}
	}
	return oneTarget(exact)
}

func oneTarget(rows []common.LinkEntry) (common.LinkEntry, bool) {
	if len(rows) == 0 {
		return common.LinkEntry{}, false
	}
	for _, l := range rows[1:] {
		if l.Raw != rows[0].Raw {
			return common.LinkEntry{}, false
		}
	}
	return rows[0], true
}

func (self *Go) pick(core *commands.Core, rows []common.LinkEntry, name string) {
	lines := make([]string, 0, len(rows))
	for i, l := range rows {
		lines = append(lines, commands.PickLine([]string{strconv.Itoa(i)}, pickLine(l)))
	}
	opts := commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "go> ",
		Header:        "enter: follow the link · ctrl-e: jump to where it is written · ctrl-/: hide pane",
	}
	if self2, err := commands.SelfCommand(core); err == nil {
		// The name goes to the children so they index into the same list.
		q := ""
		if name != "" {
			q = " " + commands.Shq(name)
		}
		opts.Preview = self2 + " go -pane-at {1}" + q
	}
	// ctrl-e closes the picker before the editor starts, so an editor that
	// wants this terminal gets it.
	key, chosen := commands.PickKey(opts, "ctrl-e")
	for _, chosen := range chosen {
		addr, ok := commands.Address(chosen, 1)
		if !ok {
			continue
		}
		i, err := strconv.Atoi(addr[0])
		if err != nil || i < 0 || i >= len(rows) {
			continue
		}
		if key == "ctrl-e" {
			core.LaunchEditor(rows[i].Filename, rows[i].Line+1)
		} else {
			self.follow(core, rows[i])
		}
	}
}

func pickLine(l common.LinkEntry) string {
	where := l.Heading
	if where == "" {
		where = "(preamble)"
	}
	return fmt.Sprintf("%s%s%s  %s%s%s  %s%s · %s%s",
		commands.C(commands.AnsiBold), commands.Ellipsis(l.Desc, 48), commands.C(commands.AnsiReset),
		commands.C(commands.AnsiCyan), commands.Ellipsis(l.Raw, 60), commands.C(commands.AnsiReset),
		commands.C(commands.AnsiDim), where, commands.BaseName(l.Filename), commands.C(commands.AnsiReset))
}

// follow goes where the link goes.
func (self *Go) follow(core *commands.Core, l common.LinkEntry) {
	if l.Broken {
		commands.Fail("orgs go: %q (%s) points at nothing; it is written in %s:%d", l.Desc, l.Raw, l.Filename, l.Line+1)
	}
	if self.Print {
		fmt.Println(destination(core, l))
		return
	}
	switch {
	// A protocol the yaml defines: its command, or its url.
	case len(l.Command) > 0:
		run(l.Command)
	case l.Open != "":
		commands.OpenInBrowser(l.Open)
	// Into the org files: the editor, at the heading.
	case l.ToHash != "":
		t := commands.TodoByHash(core, l.ToHash)
		core.LaunchEditor(t.Filename, t.LineNum+1)
	case l.ToFilename != "":
		core.LaunchEditor(l.ToFilename, 1)
	// A file on this machine, by whatever opens that kind of file.
	case localFile(l) != "":
		p := localFile(l)
		if strings.EqualFold(filepath.Ext(p), ".org") {
			core.LaunchEditor(p, 1)
		} else {
			commands.OpenInBrowser(p)
		}
	// An attachment or a file only the server can reach: the server serves it.
	case l.Url != "":
		commands.OpenInBrowser(serverUrl(core, l.Url))
	default:
		commands.OpenInBrowser(l.Raw)
	}
}

func destination(core *commands.Core, l common.LinkEntry) string {
	switch {
	case l.CommandLine != "":
		return l.CommandLine
	case l.Open != "":
		return l.Open
	case l.ToHash != "" || l.ToFilename != "":
		if l.ToFilename != "" {
			return l.ToFilename
		}
		return l.Raw
	case localFile(l) != "":
		return localFile(l)
	case l.Url != "":
		return serverUrl(core, l.Url)
	}
	return l.Raw
}

// direct follows a link typed in rather than named: `orgs go jira:ABC-123`,
// when the yaml defines jira. Anything else is a name to look up as before, so
// a description that happens to contain a colon still works.
func (self *Go) direct(core *commands.Core, name string) bool {
	if proto, _ := common.SplitLinkProtocol(name); proto == "" || strings.ContainsAny(name, " \t") {
		return false
	}
	params := map[string]string{"link": name}
	res, err := commands.SendReceiveGetErr[common.LinkResolution](core, "links/resolve", params)
	if err != nil || !res.Ok {
		return false
	}
	l := common.LinkEntry{Raw: res.Raw, Desc: res.Raw, Kind: "external", Open: res.Url, Command: res.Command, CommandLine: res.CommandLine}
	if commands.JsonOut || commands.FormatOut != "" {
		commands.RenderOne(res, nil)
		return true
	}
	self.follow(core, l)
	return true
}

// run starts a linkProtocols command here, attached to this terminal so a tool
// that wants it (a tui, a prompt) gets it, and waits for it.
func run(argv []string) {
	if commands.Wrote("run "+common.ShellJoin(argv), nil) {
		return
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		commands.Fail("orgs go: %s: %v", common.ShellJoin(argv), err)
	}
}

// localFile is the file a `file:` link names, if it is on this machine. It is
// written relative to the org file that holds it.
func localFile(l common.LinkEntry) string {
	raw := l.Raw
	if !strings.HasPrefix(raw, "file:") {
		return ""
	}
	p := strings.TrimPrefix(raw, "file:")
	if i := strings.Index(p, "::"); i >= 0 {
		p = p[:i]
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(filepath.Dir(l.Filename), p)
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// A path the server handed out (see Traps: media paths), made absolute against
// the server this command is talking to.
func serverUrl(core *commands.Core, u string) string {
	if strings.HasPrefix(u, "/") {
		return strings.TrimRight(core.Rest.Url, "/") + u
	}
	return u
}

func init() {
	commands.AddCmd("go", "follow a link by its description: orgs go <name>, or a picker",
		func() commands.Cmd { return &Go{PaneAt: -1} })
}
