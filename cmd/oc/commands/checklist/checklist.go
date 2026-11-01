package checklist

// `orgs list` (and `orgs tb`): checklists - the `- [ ] item` lines under a
// heading - worked on the way taskbook works on its boards.
//
//	orgs list                      pick a list (fzf, the list drawn in the pane),
//	                               then check, add, edit and delete its items;
//	                               space in the picker opens the list's heading
//	                               in your editor
//	orgs list groceries            the same, narrowed to lists matching "groceries"
//	orgs list new                  asks for a title, files a new list in the
//	                               inbox (checklistsFile), then asks for items
//	orgs list new Packing for Oslo the same, title given
//	orgs list ls                   every list, taskbook's board view
//	orgs list ls -open             only the lists with something left to do
//	orgs list show groceries       one list
//	orgs list add -l groceries milk        add an item
//	orgs list check -l groceries 2 3       tick items 2 and 3 (uncheck clears)
//	orgs list edit -l groceries 2 oat milk reword item 2
//	orgs list rm -l groceries 4            delete item 4
//
// A list is any heading whose own body has boxes in it, wherever it lives, so
// a list made here and refiled somewhere else is still found, and so is every
// checklist written by hand. Items are numbered from 1 in file order, nested
// ones included - the numbers `orgs list show` prints.
//
// Every write is a line edit on the server (POST /checklist to tick,
// /checklist/edit to add, reword or delete), so nothing else in the file is
// reformatted.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/tuikit"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

// The shape of a box; the server's checkItem in checklist.go says it again.
// A `*` at column zero is a heading, not a bullet.
var checkItemRe = regexp.MustCompile(`^(\s*)([-+*]|\d+[.)])\s+\[([ xX-])\]\s?(.*)$`)

// The server's name for the template; see ChecklistTemplate in capture.go.
const checklistTemplate = "Checklist"

type Checklist struct {
	fset  *flag.FlagSet
	tf    commands.TargetFlags
	name  string
	List  string
	Open  bool
	Pane  string
	Print bool
	NoAdd bool
}

func (self *Checklist) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Checklist) StartPlugin(m *common.PluginManager)       {}

func (self *Checklist) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.List, "l", "", "the list: words from its title, a query or a hash")
	fset.BoolVar(&self.Open, "open", false, "ls, pick: only lists with something left to do")
	fset.StringVar(&self.Pane, "pane", "", "draw this list (a hash) for the picker's pane")
	fset.BoolVar(&self.Print, "print", false, "draw the list's screen once and exit")
	fset.BoolVar(&self.NoAdd, "no-add", false, "new: file the list and stop, without asking for items")
	commands.AddTargetFlags(fset, &self.tf)
}

func (self *Checklist) Exec(core *commands.Core) {
	if self.Pane != "" {
		self.pane(core, self.Pane)
		return
	}
	words := commands.FreeArgs(self.fset)
	sub := ""
	if len(words) > 0 {
		sub = strings.ToLower(words[0])
	}
	rest := []string{}
	if len(words) > 1 {
		rest = words[1:]
	}
	switch sub {
	case "new", "create", "mk":
		self.create(core, rest)
	case "ls", "list", "boards":
		self.ls(core, rest)
	case "show", "cat":
		self.show(core, rest)
	case "add", "a":
		self.add(core, rest)
	case "check", "done", "tick":
		self.check(core, rest, true)
	case "uncheck", "undo", "untick":
		self.check(core, rest, false)
	case "edit", "e":
		self.edit(core, rest)
	case "rm", "delete", "del":
		self.rm(core, rest)
	default:
		// Anything else narrows the picker: `orgs list groceries`.
		if !commands.Interactive() {
			self.ls(core, words)
			return
		}
		self.pick(core, words)
	}
}

// --- reading -----------------------------------------------------------------

// gather reads every list matching the words, boxes and all. One request per
// list for its body, a few at a time.
func (self *Checklist) gather(core *commands.Core, words []string) []List {
	todos := search(core, listQuery(words))
	out := make([]List, len(todos))
	ok := make([]bool, len(todos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, t := range todos {
		wg.Add(1)
		go func(i int, t common.Todo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			l, err := load(core, t)
			if err == nil && l.Total > 0 {
				out[i], ok[i] = l, true
			}
		}(i, t)
	}
	wg.Wait()
	kept := []List{}
	for i, l := range out {
		if ok[i] && (!self.Open || l.Open() > 0) {
			kept = append(kept, l)
		}
	}
	return kept
}

func (self *Checklist) ls(core *commands.Core, words []string) {
	lists := self.gather(core, words)
	commands.Render(lists, func() {
		if len(lists) == 0 {
			fmt.Fprintln(os.Stderr, "no checklists matched")
			return
		}
		printLists(lists)
	})
}

// one resolves the list a command is about: -l, or the words, or a picker.
func (self *Checklist) one(core *commands.Core, words []string, prompt string) List {
	if self.List != "" {
		words = []string{self.List}
	}
	var target []string
	switch {
	case len(words) == 1 && commands.LooksLikeHash(words[0]):
		target = words
	case len(words) == 1 && words[0] == "-":
		target = words
	default:
		target = []string{listQuery(words)}
		// A list with no boxes yet - one just made and left empty - is still
		// a heading somebody can name.
		if len(words) > 0 && !strings.ContainsAny(strings.Join(words, " "), "()&|!=<>") &&
			len(search(core, target[0])) == 0 {
			target = []string{strings.TrimPrefix(listQuery(words), "HasChecklist() && ")}
		}
	}
	if self.tf.Hash == "" && len(words) == 0 && !commands.Interactive() {
		commands.Fail("orgs list: say which list: -l <words from its title>")
	}
	todos := commands.Resolve(core, &self.tf, target, commands.TargetOpts{Prompt: prompt, Quiet: true})
	l, err := load(core, todos[0])
	if err != nil {
		commands.Fail("orgs list: %v", err)
	}
	return l
}

func (self *Checklist) show(core *commands.Core, words []string) {
	l := self.one(core, words, "Show> ")
	if self.Print {
		self.printBoard(core, l)
		return
	}
	commands.RenderOne(l, func() {
		fmt.Println()
		printList(l)
		footer([]List{l})
	})
}

// pane is what the picker's preview runs: one list, drawn for fzf.
func (self *Checklist) pane(core *commands.Core, hash string) {
	commands.PickerOutput()
	l, err := load(core, commands.TodoByHash(core, hash))
	if err != nil {
		fmt.Printf("%s%v%s\n", c(ansiGrey), err, c(commands.AnsiReset))
		return
	}
	fmt.Println()
	printList(l)
	footer([]List{l})
}

func (self *Checklist) printBoard(core *commands.Core, l List) {
	w, h := 80, len(l.Items)+9
	if tw, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && tw > 40 {
		w = tw
	}
	b := &board{core: core, list: l}
	b.Th = tuikit.ThemeFor("")
	fmt.Print(b.print(w, h))
}

// --- the picker ------------------------------------------------------------------

// pick is fzf over the lists with the list drawn in the pane. Enter opens it
// on a screen of its own; leaving that comes back here, so several lists can
// be worked through without starting again. Esc leaves.
func (self *Checklist) pick(core *commands.Core, words []string) {
	self2, err := commands.SelfCommand(core)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orgs list: no preview (%v)\n", err)
	}
	last := ""
	for {
		lists := self.gather(core, words)
		if len(lists) == 0 {
			fmt.Fprintln(os.Stderr, "no checklists matched - orgs list new makes one")
			return
		}
		lines := []string{}
		// The list worked on last goes first, so coming back lands on it.
		for _, l := range lists {
			if l.Hash == last {
				lines = append(lines, commands.PickLine([]string{l.Hash}, pickLine(l)))
			}
		}
		for _, l := range lists {
			if l.Hash != last {
				lines = append(lines, commands.PickLine([]string{l.Hash}, pickLine(l)))
			}
		}
		opts := commands.PickOpts{
			Lines:         lines,
			AddressFields: 1,
			Prompt:        "list> ",
			Header:        "enter: open · space: edit in editor · ctrl-n: new list · ctrl-/: hide pane · esc: leave",
		}
		if self2 != "" {
			opts.Preview = self2 + noColor() + " list -pane {1} 2>/dev/null"
		}
		// Space closes the picker rather than typing into the query, so the
		// editor (which may want this terminal) has it to itself; the picker
		// comes back once the editor is done.
		key, sel := commands.PickKey(opts, "ctrl-n", "space")
		if key == "ctrl-n" {
			self.create(core, nil)
			return
		}
		if len(sel) == 0 {
			return
		}
		addr, ok := commands.Address(sel[0], 1)
		if !ok {
			return
		}
		last = addr[0]
		if key == "space" {
			self.jump(core, lists, addr[0])
			continue
		}
		b := newBoard(core, commands.TodoByHash(core, addr[0]))
		b.run()
	}
}

// jump opens a list's file in the editor on its heading.
func (self *Checklist) jump(core *commands.Core, lists []List, hash string) {
	edit, _ := tuikit.Editor(core)
	if edit == nil {
		fmt.Fprintln(os.Stderr, "orgs list: no editor - set $EDITOR or editorTemplate")
		return
	}
	for _, l := range lists {
		if l.Hash == hash {
			edit(l.File, l.Line+1)
			return
		}
	}
}

func noColor() string {
	if commands.NoColor {
		return " -no-color"
	}
	return ""
}

// --- writing ---------------------------------------------------------------------

// create files a new list in the inbox and opens it for items.
func (self *Checklist) create(core *commands.Core, words []string) {
	title := strings.TrimSpace(strings.Join(words, " "))
	in := bufio.NewReader(os.Stdin)
	if title == "" {
		if !commands.Interactive() {
			commands.Fail("orgs list new: no title given: orgs list new <title>")
		}
		fmt.Fprintf(os.Stderr, "%sTitle:%s ", commands.C(commands.AnsiBold), commands.C(commands.AnsiReset))
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(os.Stderr)
			os.Exit(1)
		}
		title = strings.TrimSpace(line)
	}
	if title == "" {
		commands.Fail("orgs list new: nothing made, no title")
	}

	// Items piped in become the first boxes: `pbpaste | orgs list new Groceries`.
	items := []string{}
	if commands.StdinIsPipe() {
		sc := bufio.NewScanner(in)
		for sc.Scan() {
			t := strings.TrimSpace(sc.Text())
			if m := checkItemRe.FindStringSubmatch(t); m != nil {
				t = m[4]
			}
			t = strings.TrimSpace(strings.TrimLeft(t, "-+*"))
			if t != "" {
				items = append(items, "- [ ] "+t)
			}
		}
	}

	var temps []common.CaptureTemplate
	commands.SendReceiveGet(core, "capture/templates", map[string]string{}, &temps)
	var tpl *common.CaptureTemplate
	for i := range temps {
		if temps[i].Name == checklistTemplate {
			tpl = &temps[i]
		}
	}
	if tpl == nil {
		commands.Fail("orgs list new: the server has no %s capture template; it needs restarting on a newer orgs", checklistTemplate)
	}

	values := map[string]string{}
	for _, f := range common.CapFields(tpl.Template) {
		values[f.Key] = f.Default
	}
	values["CONTENT"] = strings.Join(items, "\n")
	text := common.CapExpandAutos(tpl.Template, time.Now(), "")

	// Capture answers only that it worked, so the new heading is found as the
	// one with this title that was not there before.
	same := titleQuery(title)
	before := map[string]bool{}
	for _, t := range search(core, same) {
		before[t.Filename+"\x00"+t.Headline+"\x00"+t.Props["ID"]+"\x00"+t.Props["CREATED"]] = true
	}

	var query common.Capture
	query.Template = tpl.Name
	query.NewNode.Headline = title
	query.NewNode.Content = common.FillCapTemplate(text, values)

	reply, err := commands.SendReceivePostErr[common.Capture, common.ResultMsg](core, "capture", &query)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("orgs list new: %v", err)
	}
	if !reply.Ok {
		commands.Fail("orgs list new: %s", reply.Msg)
	}

	var found []common.Todo
	for _, t := range search(core, same) {
		if !before[t.Filename+"\x00"+t.Headline+"\x00"+t.Props["ID"]+"\x00"+t.Props["CREATED"]] {
			found = append(found, t)
		}
	}
	if len(found) == 0 {
		// Filed, but there is no telling it from another of the same name.
		commands.RenderOne(reply, func() {
			fmt.Printf("%s✔%s %s → %s / %s\n", c(commands.AnsiGreen), c(commands.AnsiReset), title,
				tpl.CapTarget.Filename, tpl.CapTarget.Id)
		})
		return
	}
	t := found[0]
	if self.NoAdd || !commands.Interactive() {
		l, _ := load(core, t)
		commands.RenderOne(l, func() {
			fmt.Println()
			printList(l)
		})
		return
	}
	b := newBoard(core, t)
	b.adding(0)
	b.run()
	l, _ := load(core, t)
	fmt.Println()
	printList(l)
}

// titleQuery finds headings called exactly this.
func titleQuery(title string) string {
	pat := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return '.'
	}, title)
	return fmt.Sprintf(`MatchHeadline("(?i)^ *%s *$")`, pat)
}

// --- the verbs --------------------------------------------------------------------

// numbers takes the item numbers off the front of the words: `2 3 4` or `2,3`.
func numbers(words []string, l List) ([]Item, []string) {
	out := []Item{}
	i := 0
	for ; i < len(words); i++ {
		parts := strings.Split(words[i], ",")
		got := []Item{}
		for _, p := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				got = nil
				break
			}
			if n < 1 || n > len(l.Items) {
				commands.Fail("orgs list: %s has %d items, not %d", l.Title, len(l.Items), n)
			}
			got = append(got, l.Items[n-1])
		}
		if got == nil {
			break
		}
		out = append(out, got...)
	}
	return out, words[i:]
}

func (self *Checklist) add(core *commands.Core, words []string) {
	l := self.one(core, nil, "Add to> ")
	text := strings.TrimSpace(strings.Join(words, " "))
	if text == "" {
		if !commands.Interactive() {
			commands.Fail("orgs list add: nothing to add")
		}
		b := newBoard(core, commands.TodoByHash(core, l.Hash))
		b.adding(0)
		b.run()
		return
	}
	if err := add(core, l, text, 0); err != nil {
		commands.Fail("orgs list add: %v", err)
	}
	self.after(core, l, "added "+text)
}

func (self *Checklist) check(core *commands.Core, words []string, done bool) {
	l := self.one(core, nil, "Check on> ")
	items, _ := numbers(words, l)
	if len(items) == 0 {
		commands.Fail("orgs list: which items? orgs list check -l <list> 2 3")
	}
	for _, it := range items {
		if err := setDone(core, l, it, done); err != nil {
			commands.Fail("orgs list: item %d: %v", it.Index+1, err)
		}
	}
	verb := "checked"
	if !done {
		verb = "unchecked"
	}
	self.after(core, l, fmt.Sprintf("%s %d", verb, len(items)))
}

func (self *Checklist) edit(core *commands.Core, words []string) {
	l := self.one(core, nil, "Edit in> ")
	items, rest := numbers(words, l)
	text := strings.TrimSpace(strings.Join(rest, " "))
	if len(items) != 1 || text == "" {
		commands.Fail("orgs list edit: orgs list edit -l <list> <n> <new text>")
	}
	if err := reword(core, l, items[0], text); err != nil {
		commands.Fail("orgs list edit: %v", err)
	}
	self.after(core, l, "changed")
}

func (self *Checklist) rm(core *commands.Core, words []string) {
	l := self.one(core, nil, "Delete from> ")
	items, _ := numbers(words, l)
	if len(items) == 0 {
		commands.Fail("orgs list rm: which items? orgs list rm -l <list> 4")
	}
	// Bottom up, so taking one out does not renumber the next.
	for i := len(items) - 1; i >= 0; i-- {
		for j := 0; j < i; j++ {
			if items[j].Index > items[j+1].Index {
				items[j], items[j+1] = items[j+1], items[j]
			}
		}
	}
	for i := len(items) - 1; i >= 0; i-- {
		if err := remove(core, l, items[i]); err != nil {
			commands.Fail("orgs list rm: item %d: %v", items[i].Index+1, err)
		}
	}
	self.after(core, l, fmt.Sprintf("deleted %d", len(items)))
}

// after prints the list as it now is, which is the answer to "did that work".
func (self *Checklist) after(core *commands.Core, was List, what string) {
	if commands.DryRun {
		return
	}
	l, err := load(core, common.Todo{Hash: was.Hash, Headline: was.Title, Filename: was.File, LineNum: was.Line})
	if err != nil {
		commands.Fail("orgs list: %v", err)
	}
	commands.RenderOne(l, func() {
		fmt.Printf("%s✔%s %s\n\n", c(commands.AnsiGreen), c(commands.AnsiReset), what)
		printList(l)
	})
}

func init() {
	for _, name := range []string{"list", "tb"} {
		commands.AddCmd(name, "checklists, taskbook style: pick one and check, add, edit, delete; orgs list new makes one",
			func() commands.Cmd { return &Checklist{name: name} })
	}
}
