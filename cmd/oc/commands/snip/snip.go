package snip

// `orgs snip` — the command lines you keep, kept in your notes.
//
//	orgs snip                       find one, fill in its parameters, run it
//	orgs snip docker logs           the same, starting on those words
//	orgs snip print [words]         ... and print it instead (for a shell key)
//	orgs snip copy [words]          ... and put it on the clipboard
//	orgs snip run backup -set dest=/mnt -y     no questions, for scripts
//	orgs snip new 'rsync -av <src> <dest>'     save one
//	orgs snip ls [-t docker]        list them
//	orgs snip edit [words]          open one where it is written
//	orgs snip shell zsh             the ctrl-s key and `snip-prev`, for your shell
//
// pet (github.com/knqyf263/pet) keeps commands in a TOML file of its own. Here
// a snippet is an org source block - a heading says what it does, its tags
// file it, the block is the command - so a command written down in the middle
// of a project's notes six months ago is found the same way as one saved on
// purpose. That is the half of pet's job it could never do: finding the command
// you forgot you had.
//
// Every shell block (sh, bash, zsh, fish) in the org directories is a snippet;
// `-all` takes every language. `orgs snip new` files under "Snippets" in
// snippets.org through the server's built-in Snippet capture template, which
// `orgs cap` and worg's capture dialog offer too. A capture template of your own
// called Snippet puts them somewhere else.
//
// Parameters are pet's `<name=default>` holes and babel's `:var` header
// arguments (params.go); the form that fills them is form.go. Commands run
// here, in your shell, never on the server: a snippet is for the machine you
// are typing at.

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type Snip struct {
	fset *flag.FlagSet

	Tags   string
	Query  string
	Sets   setFlags
	Yes    bool
	All    bool
	Raw    bool
	Desc   string
	Lang   string
	Force  bool
	Silent bool
}

type setFlags []string

func (s *setFlags) String() string     { return strings.Join(*s, ",") }
func (s *setFlags) Set(v string) error { *s = append(*s, v); return nil }

func (self *Snip) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Snip) StartPlugin(manager *common.PluginManager)         {}
func (self *Snip) HelpGroup() string                                 { return "Files and code" }

func (self *Snip) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Tags, "t", "", "only snippets with one of these tags (comma separated)")
	fset.StringVar(&self.Query, "q", "", "start the search on this")
	fset.Var(&self.Sets, "set", "a parameter's value, name=value (repeatable); skips the form for it")
	fset.BoolVar(&self.Yes, "y", false, "run without asking")
	fset.BoolVar(&self.All, "all", false, "every language, not only shell blocks")
	fset.BoolVar(&self.Raw, "raw", false, "print the command as written, holes and all")
	fset.StringVar(&self.Desc, "d", "", "new: what the command does")
	fset.StringVar(&self.Lang, "lang", "sh", "new: the shell it is for")
	fset.BoolVar(&self.Force, "force", false, "new: save it even if the same command is already saved")
	fset.BoolVar(&self.Silent, "s", false, "run: do not echo the command first")
}

var verbs = map[string]bool{
	"run": true, "print": true, "copy": true, "new": true, "ls": true, "list": true,
	"edit": true, "shell": true, "preview": true, "show": true,
}

func (self *Snip) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 && verbs[words[0]] {
		verb, words = words[0], words[1:]
	}
	switch verb {
	case "new":
		self.new(core, words)
	case "shell":
		self.shell(words)
	case "preview":
		self.preview(core, words)
	case "ls", "list":
		self.list(core, words)
	case "show":
		if s := self.choose(core, words, "show"); s != nil {
			self.show(*s)
		}
	case "edit":
		if s := self.choose(core, words, "edit"); s != nil {
			edit(core, s.Filename, s.Line+1)
		}
	case "print", "copy":
		self.finish(core, words, verb)
	default:
		self.finish(core, words, "run")
	}
}

// ── Finding snippets ────────────────────────────────────────────────────────

// Snippet is one source block seen as a command line.
type Snippet struct {
	common.CodeBlock
	Title  string
	Params []Param
}

var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "fish": true, "shell": true, "ksh": true, "dash": true}

func (self *Snip) load(core *commands.Core) []Snippet {
	idx, err := commands.SendReceiveGetErr[common.CodeIndex](core, "code", map[string]string{})
	if err != nil {
		commands.Fail("could not ask the server for source blocks: %v", err)
		os.Exit(1)
	}
	want := map[string]bool{}
	for _, t := range strings.Split(self.Tags, ",") {
		if t = strings.TrimSpace(strings.TrimPrefix(t, "#")); t != "" {
			want[strings.ToLower(t)] = true
		}
	}
	out := []Snippet{}
	perHeading := map[string]int{}
	for _, b := range idx.Blocks {
		if !self.All && !shells[b.Lang] {
			continue
		}
		if strings.TrimSpace(b.Code) == "" {
			continue
		}
		if len(want) > 0 {
			hit := false
			for _, t := range b.Tags {
				if want[strings.ToLower(t)] {
					hit = true
				}
			}
			if !hit {
				continue
			}
		}
		s := Snippet{CodeBlock: b}
		s.Title = b.Name
		if len(b.Olp) > 0 {
			s.Title = b.Olp[len(b.Olp)-1]
			if b.Name != "" {
				s.Title += " · " + b.Name
			}
		}
		if s.Title == "" {
			s.Title = commands.BaseName(b.Filename)
		}
		perHeading[b.Filename+"\x00"+b.Heading]++
		s.Params = Parse(b.Code, b.Vars)
		out = append(out, s)
	}
	// A heading with several blocks: number them, or they read as one.
	seen := map[string]int{}
	for i := range out {
		k := out[i].Filename + "\x00" + out[i].Heading
		if perHeading[k] > 1 && out[i].Name == "" {
			seen[k]++
			out[i].Title = fmt.Sprintf("%s #%d", out[i].Title, seen[k])
		}
	}
	// Most recently used first; the rest in the order they are written.
	used := loadUsage()
	sort.SliceStable(out, func(a, b int) bool { return used[out[a].key()] > used[out[b].key()] })
	return out
}

// key names a snippet across edits to the rest of its file: where it is and
// what it is called, not its position, which moves when a block is added
// above it.
func (s Snippet) key() string {
	h := sha1.Sum([]byte(s.Filename + "\x00" + s.Heading + "\x00" + s.Name))
	return hex.EncodeToString(h[:8])
}

func matches(s Snippet, words []string) bool {
	hay := strings.ToLower(s.Title + " " + s.Heading + " " + strings.Join(s.Tags, " ") + " " + s.Code)
	for _, w := range words {
		if !strings.Contains(hay, strings.ToLower(w)) {
			return false
		}
	}
	return true
}

// choose finds the one snippet meant: the only one the words match, or the one
// picked from those they do. Nil when nothing was chosen.
func (self *Snip) choose(core *commands.Core, words []string, verb string) *Snippet {
	all := self.load(core)
	if len(all) == 0 {
		commands.Fail("no snippets yet - save one with: orgs snip new 'the command'")
		os.Exit(1)
	}
	q := strings.TrimSpace(strings.Join(append([]string{self.Query}, words...), " "))
	hits := []Snippet{}
	for _, s := range all {
		if matches(s, strings.Fields(q)) {
			hits = append(hits, s)
		}
	}
	if len(hits) == 1 && len(words) > 0 {
		return &hits[0]
	}
	if !commands.Interactive() && !(verb == "print" && term.IsTerminal(int(os.Stdin.Fd()))) {
		switch len(hits) {
		case 0:
			commands.Fail("no snippet matches %q", q)
		default:
			commands.Fail("%d snippets match %q - say more, or run it at a terminal to pick one", len(hits), q)
		}
		os.Exit(1)
	}
	return self.pick(core, all, q, verb)
}

func (self *Snip) pick(core *commands.Core, all []Snippet, query, verb string) *Snippet {
	self_, _ := commands.SelfCommand(core)
	lines := []string{}
	byAddr := map[string]*Snippet{}
	colour := commands.Colour() || verb == "print"
	c := func(code string) string {
		if colour {
			return code
		}
		return ""
	}
	for i := range all {
		s := &all[i]
		addr := fmt.Sprintf("%d", i)
		byAddr[addr] = s
		tags := ""
		for _, t := range s.Tags {
			tags += " #" + t
		}
		first := strings.SplitN(strings.TrimSpace(s.Code), "\n", 2)[0]
		more := ""
		if strings.Count(strings.TrimSpace(s.Code), "\n") > 0 {
			more = " …"
		}
		display := fmt.Sprintf("%s%s%s%s%s%s  %s%s%s%s",
			c(commands.AnsiBold), s.Title, c(commands.AnsiReset),
			c(commands.AnsiCyan), tags, c(commands.AnsiReset),
			c(commands.AnsiDim), first, more, c(commands.AnsiReset))
		lines = append(lines, commands.PickLine([]string{addr, s.Filename, fmt.Sprint(s.Id)}, display))
	}
	enter := map[string]string{"run": "run", "print": "print", "copy": "copy", "edit": "edit", "show": "show"}[verb]
	extra := []string{"--expect", "ctrl-y,ctrl-p,ctrl-e"}
	if query != "" {
		extra = append(extra, "--query", query)
	}
	picked := commands.Pick(commands.PickOpts{
		Lines:         lines,
		AddressFields: 3,
		Prompt:        "snippet> ",
		Header:        "enter " + enter + " · ctrl-p print · ctrl-y copy · ctrl-e edit · ctrl-/ pane",
		Preview:       self_ + " snip preview {2} {3} 2>/dev/null",
		Extra:         extra,
	})
	if len(picked) < 2 {
		return nil
	}
	key, line := picked[0], picked[1]
	addr, ok := commands.Address(line, 3)
	if !ok {
		return nil
	}
	s := byAddr[addr[0]]
	switch key {
	case "ctrl-p":
		self.finishWith(core, *s, "print")
		return nil
	case "ctrl-y":
		self.finishWith(core, *s, "copy")
		return nil
	case "ctrl-e":
		edit(core, s.Filename, s.Line+1)
		return nil
	}
	return s
}

// ── Running, printing, copying ──────────────────────────────────────────────

func (self *Snip) finish(core *commands.Core, words []string, verb string) {
	s := self.choose(core, words, verb)
	if s == nil {
		return
	}
	self.finishWith(core, *s, verb)
}

func (self *Snip) presets() map[string]string {
	out := map[string]string{}
	for _, kv := range self.Sets {
		k, v, _ := strings.Cut(kv, "=")
		out[strings.TrimSpace(k)] = v
	}
	return out
}

func (self *Snip) finishWith(core *commands.Core, s Snippet, verb string) {
	code := strings.TrimRight(s.Code, "\n")
	cmd := code
	if !self.Raw {
		values := self.presets()
		ask := []Param{}
		for _, p := range s.Params {
			if _, ok := values[p.Name]; !ok {
				ask = append(ask, p)
			}
		}
		if len(ask) > 0 && canAsk() {
			got, ok := form(s.Title, code, s.Params, values, verb)
			if !ok {
				fmt.Fprintln(os.Stderr, "cancelled")
				os.Exit(130)
			}
			values = got
		}
		cmd = Fill(code, s.Params, values)
	}
	touchUsage(s.key())
	switch verb {
	case "print":
		fmt.Print(cmd)
		if term.IsTerminal(int(os.Stdout.Fd())) {
			fmt.Println()
		}
	case "copy":
		if err := copyText(cmd); err != nil {
			commands.Fail("could not reach the clipboard: %v", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "copied: %s\n", firstLine(cmd))
	default:
		os.Exit(self.run(s, cmd))
	}
}

// canAsk is whether there is a person to fill in a form: a terminal to draw on,
// which the shell key has even though its stdout is a pipe into the buffer.
func canAsk() bool {
	if commands.Machine() {
		return false
	}
	f, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	f.Close()
	return term.IsTerminal(int(os.Stdin.Fd())) || term.IsTerminal(int(os.Stderr.Fd()))
}

func firstLine(s string) string {
	l := strings.SplitN(s, "\n", 2)[0]
	if strings.Contains(s, "\n") {
		l += " …"
	}
	return l
}

// run runs a command in the shell its block is written for, in the block's
// :dir when it has one, attached to this terminal. A form filled in was the
// person saying yes; with no form, a command is shown and asked about first,
// because a picker's enter is not consent to run whatever it landed on.
func (self *Snip) run(s Snippet, cmd string) int {
	if commands.Wrote("run", cmd) {
		return 0
	}
	if !self.Yes && len(s.Params) == 0 {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			commands.Fail("not running without -y: there is nobody here to say yes")
			return 1
		}
		fmt.Fprintf(os.Stderr, "%s$ %s%s\nrun it? [Y/n] ", commands.C(commands.AnsiBold), cmd, commands.C(commands.AnsiReset))
		ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(ans)); a != "" && a != "y" && a != "yes" {
			return 1
		}
	} else if !self.Silent {
		fmt.Fprintf(os.Stderr, "%s$ %s%s\n", commands.C(commands.AnsiDim), cmd, commands.C(commands.AnsiReset))
	}
	shell := s.Lang
	switch shell {
	case "shell", "":
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "sh"
		}
	}
	if !shells[s.Lang] && s.Lang != "" {
		// -all brought it in; hand it to its interpreter if there is one.
		shell = map[string]string{"python": "python3", "ruby": "ruby", "perl": "perl", "node": "node", "js": "node", "javascript": "node"}[s.Lang]
		if shell == "" {
			commands.Fail("cannot run %s here; orgs code run %s runs it on the server", s.Lang, s.Name)
			return 1
		}
	}
	flag := "-c"
	if shell == "node" {
		flag = "-e"
	}
	c := exec.Command(shell, flag, cmd)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	for _, a := range s.Args {
		if a.Key == "dir" || a.Key == ":dir" {
			c.Dir = blockDir(s.Filename, a.Value)
		}
	}
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		commands.Fail("%v", err)
		return 1
	}
	return 0
}

// blockDir is babel's :dir, relative to the org file, ~ expanded.
func blockDir(file, dir string) string {
	dir = strings.Trim(strings.TrimSpace(dir), `"`)
	if strings.HasPrefix(dir, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, dir[1:])
		}
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(filepath.Dir(file), dir)
	}
	return dir
}

// copyText puts text on the clipboard: the platform's tool where there is
// one, and the terminal's own OSC 52 otherwise, which reaches the clipboard of
// the machine you are sitting at even over ssh.
func copyText(text string) error {
	try := [][]string{}
	switch runtime.GOOS {
	case "darwin":
		try = [][]string{{"pbcopy"}}
	case "windows":
		try = [][]string{{"clip"}}
	default:
		try = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	if os.Getenv("SSH_CONNECTION") == "" {
		for _, t := range try {
			if _, err := exec.LookPath(t[0]); err != nil {
				continue
			}
			c := exec.Command(t[0], t[1:]...)
			c.Stdin = strings.NewReader(text)
			if err := c.Run(); err == nil {
				return nil
			}
		}
	}
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer tty.Close()
	_, err = fmt.Fprintf(tty, "\033]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(text)))
	return err
}

// ── Saving one ──────────────────────────────────────────────────────────────

func (self *Snip) new(core *commands.Core, words []string) {
	cmd := strings.TrimSpace(strings.Join(words, " "))
	interactive := term.IsTerminal(int(os.Stdin.Fd()))
	if cmd == "" && !interactive {
		b, _ := readAll(os.Stdin)
		cmd = strings.TrimSpace(b)
		// What came in on stdin was the command; questions now go to the
		// terminal, if there is one.
		interactive = false
	}
	in := bufio.NewReader(os.Stdin)
	ask := func(prompt string) string {
		fmt.Fprint(os.Stderr, commands.C(commands.AnsiBold)+prompt+commands.C(commands.AnsiReset))
		s, _ := in.ReadString('\n')
		return strings.TrimSpace(s)
	}
	if cmd == "" && interactive {
		fmt.Fprintln(os.Stderr, commands.C(commands.AnsiDim)+"A hole is <name> or <name=default>; finish with an empty line."+commands.C(commands.AnsiReset))
		lines := []string{}
		for {
			l := ask(map[bool]string{true: "Command> ", false: "    ...> "}[len(lines) == 0])
			if l == "" {
				break
			}
			lines = append(lines, l)
		}
		cmd = strings.Join(lines, "\n")
	}
	if cmd == "" {
		commands.Fail("nothing to save: orgs snip new 'the command'")
		os.Exit(1)
	}
	if !self.Force {
		for _, s := range self.load(core) {
			if strings.TrimSpace(s.Code) == cmd {
				commands.Fail("already saved as %q in %s (-force saves it again)", s.Title, commands.BaseName(s.Filename))
				os.Exit(1)
			}
		}
	}
	if !interactive && self.Desc == "" {
		commands.Fail("say what it does with -d 'description'")
		os.Exit(1)
	}
	if interactive {
		fmt.Fprintf(os.Stderr, "%s$ %s%s\n", commands.C(commands.AnsiDim), cmd, commands.C(commands.AnsiReset))
	}
	desc := self.Desc
	for desc == "" {
		desc = ask("What does it do? ")
	}
	tags := self.Tags
	if tags == "" && interactive {
		tags = ask("Tags (optional)> ")
	}
	tagList := []string{}
	for _, t := range strings.FieldsFunc(tags, func(r rune) bool { return r == ',' || r == ' ' || r == ':' }) {
		tagList = append(tagList, strings.TrimPrefix(t, "#"))
	}
	lang := self.Lang
	if lang == "" {
		lang = "sh"
	}
	body := "#+begin_src " + lang + "\n" + cmd + "\n#+end_src"
	capture := common.Capture{Template: "Snippet", NewNode: common.NewNode{Headline: desc, Content: body, Tags: tagList}}
	res, err := commands.SendReceivePostErr[common.Capture, common.ResultMsg](core, "capture", &capture)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil || !res.Ok {
		msg := res.Msg
		if err != nil {
			msg = err.Error()
		}
		commands.Fail("not saved: %s", msg)
		os.Exit(1)
	}
	if commands.RenderOne(res, nil) {
		return
	}
	ps := Parse(cmd, nil)
	note := ""
	if len(ps) > 0 {
		n := []string{}
		for _, p := range ps {
			n = append(n, p.Name)
		}
		note = fmt.Sprintf(" with %d parameter%s (%s)", len(ps), map[bool]string{true: "", false: "s"}[len(ps) == 1], strings.Join(n, ", "))
	}
	fmt.Fprintf(os.Stderr, "%ssaved%s %q%s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), desc, note)
}

// ── Listing and showing ─────────────────────────────────────────────────────

type row struct {
	Title    string
	Tags     []string
	Lang     string
	Command  string
	Params   []Param
	Filename string
	Line     int
	Heading  string
}

func (self *Snip) list(core *commands.Core, words []string) {
	q := append([]string{}, words...)
	if self.Query != "" {
		q = append(q, strings.Fields(self.Query)...)
	}
	rows := []row{}
	for _, s := range self.load(core) {
		if matches(s, q) {
			rows = append(rows, row{s.Title, s.Tags, s.Lang, strings.TrimRight(s.Code, "\n"), s.Params, s.Filename, s.Line + 1, s.Heading})
		}
	}
	commands.Render(rows, func() {
		w := 0
		for _, r := range rows {
			w = max(w, commands.RuneLen(r.Title))
		}
		w = min(w, 40)
		for _, r := range rows {
			tags := ""
			for _, t := range r.Tags {
				tags += " #" + t
			}
			fmt.Printf("%s%-*s%s %s%s%s  %s\n", commands.C(commands.AnsiBold), w, commands.Ellipsis(r.Title, w), commands.C(commands.AnsiReset),
				commands.C(commands.AnsiCyan), tags, commands.C(commands.AnsiReset), firstLine(r.Command))
		}
	})
}

// preview is the picker's pane: `orgs snip preview FILE ID`.
func (self *Snip) preview(core *commands.Core, words []string) {
	if len(words) < 2 {
		return
	}
	for _, s := range self.load(core) {
		if s.Filename == words[0] && fmt.Sprint(s.Id) == words[1] {
			self.show(s)
			return
		}
	}
}

func (self *Snip) show(s Snippet) {
	width := commands.PaneWidth()
	fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), s.Title, commands.C(commands.AnsiReset))
	if len(s.Tags) > 0 {
		fmt.Printf("%s#%s%s\n", commands.C(commands.AnsiCyan), strings.Join(s.Tags, " #"), commands.C(commands.AnsiReset))
	}
	fmt.Printf("%s%s:%d  %s%s\n\n", commands.C(commands.AnsiDim), commands.BaseName(s.Filename), s.Line+1, s.Heading, commands.C(commands.AnsiReset))
	lang := s.Lang
	if lang == "sh" || lang == "shell" || lang == "zsh" {
		lang = "bash"
	}
	code := strings.TrimRight(s.Code, "\n")
	if commands.Colour() || os.Getenv("FZF_PREVIEW_COLUMNS") != "" {
		var b strings.Builder
		if err := quick.Highlight(&b, code, lang, "terminal256", "monokai"); err == nil {
			code = b.String()
		}
	}
	fmt.Println(code)
	if len(s.Params) > 0 {
		fmt.Println()
		commands.OpenBox("parameters", width)
		for _, p := range s.Params {
			v := p.Default
			switch {
			case len(p.Choices) > 1:
				v = strings.Join(p.Choices, " · ")
			case v == "" && p.Optional:
				v = commands.C(commands.AnsiDim) + "(optional)" + commands.C(commands.AnsiReset)
			case v == "":
				v = commands.C(commands.AnsiDim) + "(no default)" + commands.C(commands.AnsiReset)
			}
			from := ""
			if p.FromVar {
				from = commands.C(commands.AnsiDim) + "  :var" + commands.C(commands.AnsiReset)
			}
			commands.BoxLine(fmt.Sprintf("%s%s%s = %s%s", commands.C(commands.AnsiGold), p.Name, commands.C(commands.AnsiReset), v, from))
		}
		commands.CloseBox(width)
	}
	for _, a := range s.Args {
		if a.Key == "dir" || a.Key == ":dir" {
			fmt.Printf("\n%sruns in %s%s\n", commands.C(commands.AnsiDim), blockDir(s.Filename, a.Value), commands.C(commands.AnsiReset))
		}
	}
	if strings.TrimSpace(s.Result) != "" {
		fmt.Println()
		commands.OpenBox("last output", width)
		commands.BoxText(s.Result)
		commands.CloseBox(width)
	}
}

// ── Editing ─────────────────────────────────────────────────────────────────

func edit(core *commands.Core, file string, line int) {
	if len(core.EditorTemplate) > 0 {
		core.LaunchEditor(file, line)
		return
	}
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		ed = "vi"
	}
	args := append(strings.Fields(ed), fmt.Sprintf("+%d", line), file)
	c := exec.Command(args[0], args[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	_ = c.Run()
}

// ── Recently used ───────────────────────────────────────────────────────────

// Which snippets were used when, so the ones you reach for are at the top.
// Kept in this machine's cache, not the org files: it is about this machine.
func usagePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "orgs", "snip-usage.json")
}

func loadUsage() map[string]int64 {
	out := map[string]int64{}
	if b, err := os.ReadFile(usagePath()); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func touchUsage(key string) {
	u := loadUsage()
	u[key] = time.Now().Unix()
	if b, err := json.Marshal(u); err == nil {
		_ = os.MkdirAll(filepath.Dir(usagePath()), 0o755)
		_ = os.WriteFile(usagePath(), b, 0o644)
	}
}

func init() {
	commands.AddCmd("snip", "find, fill in and run the command lines kept in your notes",
		func() commands.Cmd { return &Snip{Lang: "sh"} })
}

/* SDOC: Commands

* Snip

  =orgs snip= keeps the command lines worth keeping - and finds the ones you
  wrote down and forgot - in your org files. It does what [[https://github.com/knqyf263/pet][pet]]
  does, with org source blocks for snippets: a heading says what the command
  does, its tags file it, the block is the command. Every shell block in your
  notes is a snippet, so a command written into a project's notes months ago
  turns up beside the ones saved on purpose.

  #+BEGIN_SRC bash
  orgs snip                          # find one, fill in its parameters, run it
  orgs snip docker logs              # start the search on those words
  orgs snip print [words]            # print it instead (what the shell key uses)
  orgs snip copy [words]             # put it on the clipboard
  orgs snip run backup -set dest=/mnt -y   # no questions: for scripts and cron
  orgs snip new 'rsync -av <src> <dest=/backup>'   # save one
  orgs snip ls -t docker             # list, by tag (comma separated: any of them)
  orgs snip show [words]             # what it is and what it asks for
  orgs snip edit [words]             # open it where it is written
  orgs snip shell zsh                # the ctrl-s key and snip-prev
  #+END_SRC

** A snippet

   #+BEGIN_SRC org
   ,* Docker                                                     :docker:
   ,** Tail a container's logs
   ,#+begin_src bash
   docker logs -f --tail <lines=100> <container=web>
   ,#+end_src
   #+END_SRC

   Any =sh=, =bash=, =zsh= or =fish= block is a snippet (=-all= takes every
   language). Its title is its heading, plus its =#+NAME:= when it has one; its
   tags are its heading's and the ones it inherits. A =:dir= header argument is
   where it runs. Its last =#+RESULTS:= is shown beside it.

** Parameters

   | Written                          | Means                                              |
   |----------------------------------+----------------------------------------------------|
   | =<name>=                         | a value to fill in                                 |
   | =<name=default>=                 | with a default (an === in it is fine)              |
   | =<flags?>=                       | optional: left empty, it is left out               |
   | =\<b>=                           | a literal =<b>=, not a parameter                   |
   | =:var host="db.internal"=        | babel's way: a parameter where the body says =$host= |
   | =:var env='("dev" "prod")=       | babel's way, with choices                          |

   A choice is written as pet writes it, and the first is the default:

   #+BEGIN_SRC sh
   kubectl -n <ns=|_dev_||_staging_||_prod_|> get pods
   #+END_SRC

   The =:var= form keeps the block runnable with =C-c C-c= in Emacs. A name
   used twice is one parameter.

   Values are quoted for the shell as much as where they land needs: inside the
   command's own quotes they are escaped for those quotes; elsewhere a value is
   single-quoted only if it has a space or something the shell would act on.
   A glob or a =$VAR= in a value is left alone, because that is somebody asking
   for one.

** The form

   With parameters to fill in, a form shows the finished command - redrawn as
   you type, values in colour - above a field for each one, defaults already
   in. *Tab* and *shift-tab* move between fields, *↑ ↓* cycle a choice,
   *ctrl-u* clears, *enter* runs (or prints, or copies), *esc* cancels and runs
   nothing. =-set name=value= fills one in without asking.

   A snippet with no parameters is shown and asked about before it runs;
   =-y= skips the question. Commands run here, in your shell - never on the
   server.

** The picker

   Snippets you used recently come first. The pane shows the command coloured,
   its parameters and defaults, where it runs and what it last printed.
   *enter* does what the command was asked to do; *ctrl-p* prints, *ctrl-y*
   copies, *ctrl-e* edits, *ctrl-/* hides the pane.

** Saving one

   #+BEGIN_SRC bash
   orgs snip new 'du -ah <dir=.> | sort -rh | head -n <count=20>'   # asks what it does, and tags
   orgs snip new -d 'Find big files' -t disk,find 'du -ah ...'      # asks nothing
   pbpaste | orgs snip new -d 'From the clipboard'                   # the command on stdin
   orgs snip new                                                     # type it, several lines if need be
   #+END_SRC

   It is filed under "Snippets" in =snippets.org= in your first org directory,
   through the server's built-in =Snippet= capture template - which is also in
   worg's capture dialog and =orgs cap=. Define a capture template of your own
   called =Snippet= to file them somewhere else. A command already saved is
   refused (=-force= saves it again).

** In your shell

   #+BEGIN_SRC bash
   eval "$(orgs snip shell zsh)"      # in ~/.zshrc
   eval "$(orgs snip shell bash)"     # in ~/.bashrc
   orgs snip shell fish | source      # in config.fish
   #+END_SRC

   *ctrl-s* opens the picker on what you have typed so far and puts the chosen
   command, filled in, on your command line - to edit, run, and keep in your
   history like anything you typed. =ORGS_SNIP_KEY= picks another key.
   =snip-prev= saves the command you just ran (=snip-prev -d 'what' -t tags=).
EDOC */
