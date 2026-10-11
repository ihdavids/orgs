package triage

// orgs triage - the inbox, one heading at a time.
//
//	orgs triage              each inbox item in turn: r refile, s schedule, t tag,
//	                         a archive, d done, n next, u undo, o open, q quit
//	orgs triage -someday     the someday pile: k keep, p promote, s schedule,
//	                         a drop (archive)
//	orgs triage -json        the items, listed
//
// One item on screen, one keypress per decision. That is the whole design:
// an inbox is emptied by deciding, and every extra keystroke between seeing a
// thing and deciding about it is a reason to put the decision off. So a key
// does the thing at once (refile opens the picker, schedule asks for a date
// in the words you would say - "fri", "+2w"), the next item comes up, and the
// wrong key is one `u` away.
//
// What the inbox *is* comes from the server (`review.inbox` in its config), as
// does every write: each decision is one POST /review/act, which is undoable,
// and the list is asked for again after it. Asked again rather than patched,
// because a refile out of a file changes the hash of every heading after it
// in that file; the walk keeps its place by the items it has already shown
// (file and outline path), not by position or hash.
//
// The walk itself (Walker) is exported: `orgs review week` walks its own lists
// - stuck projects, waiting items, stale ones - with the same screen and keys.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type Triage struct {
	Someday bool
}

func (self *Triage) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Triage) StartPlugin(m *common.PluginManager)       {}
func (self *Triage) HelpGroup() string                         { return "Changing a heading" }

func (self *Triage) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Someday, "someday", false, "sweep the someday pile instead of the inbox")
}

func (self *Triage) Exec(core *commands.Core) {
	commands.FreeArgs(commands.Find("triage").Flags)
	path := "review/inbox"
	if self.Someday {
		path = "review/someday"
	}
	list, err := commands.SendReceiveGetErr[common.ReviewList](core, path, nil)
	if err != nil {
		commands.Fail("orgs triage: %v", err)
	}
	if !list.Ok {
		commands.Fail("orgs triage: %s", list.Msg)
	}
	if commands.Machine() {
		commands.Render(list.Items, nil)
		return
	}
	if !commands.Interactive() {
		// A pipe gets the list, one line each: the question "what is in
		// there" has an answer even where nobody can press a key.
		for _, it := range list.Items {
			fmt.Println(Line(it))
		}
		return
	}
	w := NewWalker(core, nil)
	if self.Someday {
		w.Walk(SomedayDeck(core))
	} else {
		w.Walk(InboxDeck(core))
	}
	w.Summary()
}

func init() {
	commands.AddCmd("triage", "empty the inbox one heading at a time (-someday sweeps the someday pile)",
		func() commands.Cmd { return &Triage{} })
}

// ---------------------------------------------------------------------------
// The decks
// ---------------------------------------------------------------------------

func fetch(core *commands.Core, path string, params map[string]string) func() ([]common.ReviewItem, error) {
	return func() ([]common.ReviewItem, error) {
		list, err := commands.SendReceiveGetErr[common.ReviewList](core, path, params)
		if err != nil {
			return nil, err
		}
		if !list.Ok {
			return nil, fmt.Errorf("%s", list.Msg)
		}
		return list.Items, nil
	}
}

// InboxDeck is the inbox and what can be done with an item in it.
func InboxDeck(core *commands.Core) Deck {
	return Deck{
		Title: "Inbox",
		Fetch: fetch(core, "review/inbox", nil),
		Empty: "Inbox zero. Nothing left to decide.",
		Keys: []Key{
			{"r", "refile", func(w *Walker, it common.ReviewItem) Outcome { return w.Refile(it) }},
			{"s", "schedule", func(w *Walker, it common.ReviewItem) Outcome { return w.Schedule(it, "schedule") }},
			{"t", "tag", func(w *Walker, it common.ReviewItem) Outcome { return w.Tag(it) }},
			{"a", "archive", func(w *Walker, it common.ReviewItem) Outcome {
				return w.Act(it, common.ReviewAct{Action: "archive"}, true)
			}},
			{"d", "done", func(w *Walker, it common.ReviewItem) Outcome {
				return w.Act(it, common.ReviewAct{Action: "done"}, true)
			}},
		},
	}
}

// SomedayDeck is the someday pile: keep it, bring it back, give it a date, or
// let it go.
func SomedayDeck(core *commands.Core) Deck {
	return Deck{
		Title: "Someday",
		Fetch: fetch(core, "review/someday", nil),
		Empty: "Swept. Everything on the someday list has been looked at.",
		Keys: []Key{
			{"k", "keep", func(w *Walker, it common.ReviewItem) Outcome { return Next }},
			{"p", "promote", func(w *Walker, it common.ReviewItem) Outcome {
				kw := w.Choose("promote to", []string{"n NEXT", "t TODO"})
				switch kw {
				case "n":
					return w.Act(it, common.ReviewAct{Action: "promote", Keyword: "NEXT"}, true)
				case "t":
					return w.Act(it, common.ReviewAct{Action: "promote", Keyword: "TODO"}, true)
				}
				return Stay
			}},
			{"s", "schedule", func(w *Walker, it common.ReviewItem) Outcome { return w.Schedule(it, "schedule") }},
			{"a", "drop", func(w *Walker, it common.ReviewItem) Outcome {
				return w.Act(it, common.ReviewAct{Action: "archive"}, true)
			}},
		},
	}
}

// ---------------------------------------------------------------------------
// The walk
// ---------------------------------------------------------------------------

// What a key did to the walk.
type Outcome int

const (
	// The item stays on screen, read again (its tags or date changed).
	Stay Outcome = iota
	// On to the next one.
	Next
	// Leave this deck.
	Done
	// Leave everything: q in a wizard is "stop for now", not "skip this step".
	Quit
)

type Key struct {
	Key   string
	Label string
	Do    func(w *Walker, it common.ReviewItem) Outcome
}

// A Deck is a list to walk and what each key does to an item of it.
type Deck struct {
	Title string
	Fetch func() ([]common.ReviewItem, error)
	// Extra lines under the heading: how long it has waited, its open tasks.
	Extra func(it common.ReviewItem) []string
	Keys  []Key
	// Keys not in Keys (the digits that pick one of a project's tasks, say).
	// A Key whose Do is nil is only drawn in the legend, for these.
	Other func(w *Walker, it common.ReviewItem, key string) Outcome
	// Said when the list is empty, or has been walked to the end.
	Empty string
}

// A Walker walks decks: it draws, reads keys, sends the writes and keeps the
// undo tokens and a tally of what was done.
type Walker struct {
	// Wait for a key on the "all done" screen before leaving a deck, so a
	// wizard's next step does not wipe it straight away.
	Pause bool
	core  *commands.Core
	in    *bufio.Reader
	Tally map[string]int
	undo  []undone
	note  string
	tags  []string
}

type undone struct {
	token  string
	key    string
	action string
}

// NewWalker; tally may be shared between walks so a wizard can say at the end
// what the whole review did.
func NewWalker(core *commands.Core, tally map[string]int) *Walker {
	if tally == nil {
		tally = map[string]int{}
	}
	return &Walker{core: core, in: bufio.NewReader(os.Stdin), Tally: tally}
}

func itemKey(it common.ReviewItem) string {
	return it.Filename + "\x00" + strings.Join(it.Olp, "\x00") + "\x00" + it.Headline
}

// Walk shows each item of the deck in turn until the list is walked, emptied
// or left. It reports whether the person asked to stop altogether.
func (self *Walker) Walk(deck Deck) (quit bool) {
	seen := map[string]bool{}
	self.note = ""
	for {
		items, err := deck.Fetch()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return true
		}
		var cur *common.ReviewItem
		left := 0
		for i := range items {
			if !seen[itemKey(items[i])] {
				if cur == nil {
					cur = &items[i]
				}
				left++
			}
		}
		if cur == nil {
			self.clear()
			self.header(deck.Title, "")
			if len(items) == 0 {
				fmt.Printf("\n  %s✓ %s%s\n\n", commands.C(commands.AnsiGreen), deck.Empty, commands.C(commands.AnsiReset))
			} else {
				fmt.Printf("\n  %s✓ Walked all %d. %s%s\n\n", commands.C(commands.AnsiGreen), len(items),
					"What is left is what you chose to leave.", commands.C(commands.AnsiReset))
			}
			if self.note != "" {
				fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), self.note, commands.C(commands.AnsiReset))
			}
			if len(self.undo) > 0 || self.Pause {
				// The last decision can still be taken back from here.
				hint := "any key goes on"
				if len(self.undo) > 0 {
					hint = "u undoes the last one · any other key goes on"
				}
				fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), hint, commands.C(commands.AnsiReset))
				switch self.ReadKey() {
				case "u":
					if len(self.undo) > 0 {
						if k := self.Undo(); k != "" {
							delete(seen, k)
						}
						continue
					}
				case "q", "ctrl-c":
					return true
				}
			}
			return false
		}
		pos := len(seen) + 1
		self.draw(deck, *cur, pos, len(seen)+left)
		key := self.ReadKey()
		out := Stay
		switch key {
		case "q", "ctrl-c", "esc":
			return true
		case "n", " ", "right", "enter":
			out = Next
		case "u":
			if k := self.Undo(); k != "" {
				delete(seen, k)
			}
			continue
		case "o":
			self.core.LaunchEditor(cur.Filename, cur.LineNum+1)
			continue
		case "?":
			self.note = "keys: " + legend(deck)
			continue
		default:
			handled := false
			for _, k := range deck.Keys {
				if k.Key == key && k.Do != nil {
					out, handled = k.Do(self, *cur), true
					break
				}
			}
			if !handled && deck.Other != nil {
				out = deck.Other(self, *cur, key)
			}
		}
		switch out {
		case Next:
			seen[itemKey(*cur)] = true
		case Done:
			return false
		case Quit:
			return true
		}
	}
}

func legend(deck Deck) string {
	parts := []string{}
	for _, k := range deck.Keys {
		parts = append(parts, k.Key+" "+k.Label)
	}
	parts = append(parts, "n next", "u undo", "o open", "q quit")
	return strings.Join(parts, "  ")
}

func (self *Walker) clear() {
	if commands.Colour() {
		fmt.Print("\033[H\033[2J")
	} else {
		fmt.Println()
	}
}

func width() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 20 {
		if w > 100 {
			return 100
		}
		return w
	}
	return 80
}

func (self *Walker) header(title, counter string) {
	w := width()
	gap := w - 4 - commands.RuneLen(title) - commands.RuneLen(counter)
	if gap < 1 {
		gap = 1
	}
	fmt.Printf("  %s%s%s%s%s%s\n", commands.C(commands.AnsiBold), title, commands.C(commands.AnsiReset),
		strings.Repeat(" ", gap), commands.C(commands.AnsiDim)+counter, commands.C(commands.AnsiReset))
	fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), strings.Repeat("─", w-4), commands.C(commands.AnsiReset))
}

func (self *Walker) draw(deck Deck, it common.ReviewItem, pos, total int) {
	self.clear()
	self.header(deck.Title, fmt.Sprintf("%d of %d", pos, total))
	fmt.Println()
	kw := ""
	if it.Status != "" {
		kw = commands.C(commands.AnsiGold) + it.Status + commands.C(commands.AnsiReset) + " "
	}
	prio := ""
	if it.Priority != "" {
		prio = commands.C(commands.AnsiRed) + "[#" + it.Priority + "] " + commands.C(commands.AnsiReset)
	}
	fmt.Printf("  %s%s%s%s%s\n", kw, prio, commands.C(commands.AnsiBold), it.Headline, commands.C(commands.AnsiReset))
	where := commands.BaseName(it.Filename)
	if len(it.Olp) > 0 {
		where += " › " + strings.Join(it.Olp, " › ")
	}
	meta := []string{where}
	if len(it.Tags) > 0 {
		meta = append(meta, commands.C(commands.AnsiCyan)+":"+strings.Join(it.Tags, ":")+":"+commands.C(commands.AnsiReset)+commands.C(commands.AnsiDim))
	}
	if it.Scheduled != "" {
		meta = append(meta, "scheduled "+it.Scheduled)
	}
	if it.Effort != "" {
		meta = append(meta, "effort "+it.Effort)
	}
	fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), strings.Join(meta, "  ·  "), commands.C(commands.AnsiReset))
	if deck.Extra != nil {
		for _, l := range deck.Extra(it) {
			fmt.Printf("  %s\n", l)
		}
	}
	if strings.TrimSpace(it.Body) != "" {
		fmt.Println()
		lines := strings.Split(it.Body, "\n")
		if len(lines) > 14 {
			lines = append(lines[:14], "…")
		}
		for _, l := range lines {
			fmt.Printf("    %s\n", commands.Ellipsis(l, width()-6))
		}
	}
	if it.Children > 0 {
		fmt.Printf("\n  %s+ %d heading%s under it, which go%s with it%s\n", commands.C(commands.AnsiDim), it.Children,
			plural(it.Children), map[bool]string{true: "es", false: ""}[it.Children == 1], commands.C(commands.AnsiReset))
	}
	fmt.Println()
	fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), strings.Repeat("─", width()-4), commands.C(commands.AnsiReset))
	fmt.Printf("  %s\n", keyLegend(deck))
	if self.note != "" {
		fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiGreen), self.note, commands.C(commands.AnsiReset))
		self.note = ""
	}
}

func keyLegend(deck Deck) string {
	parts := []string{}
	k := func(key, label string) string {
		return commands.C(commands.AnsiBold) + key + commands.C(commands.AnsiReset) + " " +
			commands.C(commands.AnsiDim) + label + commands.C(commands.AnsiReset)
	}
	for _, x := range deck.Keys {
		parts = append(parts, k(x.Key, x.Label))
	}
	parts = append(parts, k("n", "next"), k("u", "undo"), k("o", "open"), k("q", "quit"))
	return strings.Join(parts, "  ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ---------------------------------------------------------------------------
// Keys and lines from the terminal
// ---------------------------------------------------------------------------

// ReadKey reads one keypress, without waiting for enter. Arrow keys come back
// as "left"/"right", ctrl-c as "ctrl-c".
func (self *Walker) ReadKey() string {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		// No raw mode (a terminal that refuses it): a line, and its first
		// letter is the key.
		line, _ := self.in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return "enter"
		}
		return line[:1]
	}
	defer term.Restore(fd, old)
	b, err := self.in.ReadByte()
	if err != nil {
		return "q"
	}
	switch b {
	case 3:
		return "ctrl-c"
	case '\r', '\n':
		return "enter"
	case 27:
		// An arrow is ESC [ C, all in one read; a lone ESC is the escape key.
		if self.in.Buffered() >= 2 {
			b1, _ := self.in.ReadByte()
			b2, _ := self.in.ReadByte()
			if b1 == '[' {
				switch b2 {
				case 'C':
					return "right"
				case 'D':
					return "left"
				case 'A':
					return "up"
				case 'B':
					return "down"
				}
			}
			return "?"
		}
		return "esc"
	}
	return string(rune(b))
}

// Ask reads a line, saying what it is for. Empty means never mind.
func (self *Walker) Ask(prompt, hint string) string {
	if hint != "" {
		fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), hint, commands.C(commands.AnsiReset))
	}
	fmt.Printf("  %s%s%s ", commands.C(commands.AnsiBold), prompt, commands.C(commands.AnsiReset))
	line, _ := self.in.ReadString('\n')
	return strings.TrimSpace(line)
}

// Choose offers a few one-key choices ("n NEXT") and answers with the key
// pressed, or "" for anything else.
func (self *Walker) Choose(prompt string, choices []string) string {
	fmt.Printf("  %s%s:%s %s  %s(esc cancels)%s\n", commands.C(commands.AnsiBold), prompt, commands.C(commands.AnsiReset),
		strings.Join(choices, "  "), commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	k := self.ReadKey()
	for _, c := range choices {
		if strings.HasPrefix(c, k+" ") {
			return k
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// The writes
// ---------------------------------------------------------------------------

// Act sends one decision. next says whether it is the end of the item (a
// refile is; a tag usually is not - you tag, then refile). A failure stays on
// the item and says why.
func (self *Walker) Act(it common.ReviewItem, req common.ReviewAct, next bool) Outcome {
	if req.Hash == "" {
		req.Hash = it.Hash
	}
	res, err := commands.SendReceivePostErr[common.ReviewAct, common.ReviewActResult](self.core, "review/act", &req)
	if err == commands.ErrDryRun {
		self.note = "dry run: nothing was written"
		if next {
			return Next
		}
		return Stay
	}
	if err != nil {
		self.note = "could not: " + err.Error()
		return Stay
	}
	if !res.Ok {
		self.note = "could not: " + res.Msg
		if res.Undo != "" {
			self.undo = append(self.undo, undone{token: res.Undo, key: itemKey(it), action: req.Action})
		}
		return Stay
	}
	self.Tally[tallyName(req.Action)]++
	if res.Undo != "" {
		self.undo = append(self.undo, undone{token: res.Undo, key: itemKey(it), action: req.Action})
	}
	self.note = "✓ " + res.Did
	if next {
		return Next
	}
	return Stay
}

// What the summary calls an action.
func tallyName(action string) string {
	switch action {
	case "refile":
		return "refiled"
	case "archive":
		return "archived"
	case "schedule", "deadline":
		return "scheduled"
	case "unschedule":
		return "unscheduled"
	case "tags":
		return "tagged"
	case "done":
		return "done"
	case "promote":
		return "promoted"
	case "status":
		return "keyword changed"
	case "next":
		return "next actions added"
	case "chase":
		return "chased"
	}
	return action
}

// Undo takes back the last decision and answers with the key of the item it
// was about, so the walk can show it again.
func (self *Walker) Undo() string {
	if len(self.undo) == 0 {
		self.note = "nothing to undo"
		return ""
	}
	last := self.undo[len(self.undo)-1]
	res, err := commands.SendReceivePostErr[common.ReviewUndo, common.ReviewActResult](self.core, "review/undo",
		&common.ReviewUndo{Token: last.token})
	if err == commands.ErrDryRun {
		return ""
	}
	if err != nil {
		self.note = "could not undo: " + err.Error()
		return ""
	}
	if !res.Ok {
		self.note = "could not undo: " + res.Msg
		return ""
	}
	self.undo = self.undo[:len(self.undo)-1]
	if n := tallyName(last.action); self.Tally[n] > 0 {
		self.Tally[n]--
	}
	self.note = "↶ " + res.Did
	return last.key
}

// Refile picks a destination in fzf and moves the item there.
func (self *Walker) Refile(it common.ReviewItem) Outcome {
	list, err := commands.SendReceiveGetErr[common.RefileTargetList](self.core, "refile/targets", map[string]string{"depth": "4"})
	if err != nil || !list.Ok {
		msg := list.Msg
		if err != nil {
			msg = err.Error()
		}
		self.note = "could not list refile targets: " + msg
		return Stay
	}
	lines := []string{}
	for i, t := range list.Targets {
		// Not into itself, or anywhere under it.
		if t.Hash == it.Hash {
			continue
		}
		label := commands.BaseName(t.Filename)
		if len(t.Olp) > 0 {
			label = strings.Join(t.Olp, " › ") + "  " + commands.C(commands.AnsiDim) + commands.BaseName(t.Filename) + commands.C(commands.AnsiReset)
		} else {
			label = commands.C(commands.AnsiBold) + label + commands.C(commands.AnsiReset) + "  " + commands.C(commands.AnsiDim) + "(end of file)" + commands.C(commands.AnsiReset)
		}
		if t.Todo != "" {
			label = commands.C(commands.AnsiGold) + t.Todo + commands.C(commands.AnsiReset) + " " + label
		}
		if len(t.Tags) > 0 {
			label += "  " + commands.C(commands.AnsiCyan) + ":" + strings.Join(t.Tags, ":") + ":" + commands.C(commands.AnsiReset)
		}
		lines = append(lines, commands.PickLine([]string{fmt.Sprint(i)}, label))
	}
	sel := commands.Pick(commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "refile to> ",
		Header:        "where does \"" + commands.Ellipsis(it.Headline, 50) + "\" belong?  ·  esc cancels",
	})
	if len(sel) == 0 {
		return Stay
	}
	addr, ok := commands.Address(sel[0], 1)
	if !ok {
		return Stay
	}
	var i int
	if _, err := fmt.Sscanf(addr[0], "%d", &i); err != nil || i < 0 || i >= len(list.Targets) {
		return Stay
	}
	t := list.Targets[i]
	to := common.Target{Type: "file", Filename: t.Filename}
	if t.Hash != "" {
		to = common.Target{Type: "hash", Filename: t.Filename, Id: t.Hash}
	}
	return self.Act(it, common.ReviewAct{Action: "refile", To: &to}, true)
}

// Schedule asks for a date the way it would be said. The server reads it
// (internal/orgdate, the same reader as `orgs sched`), so the words mean the
// same thing everywhere.
func (self *Walker) Schedule(it common.ReviewItem, action string) Outcome {
	hint := "today, tomorrow, fri, +3d, +2w, 2026-11-02, \"fri 14:00\"  ·  - takes it off  ·  enter cancels"
	when := self.Ask(map[string]string{"schedule": "schedule for:", "deadline": "due:"}[action], hint)
	switch when {
	case "":
		return Stay
	case "-":
		return self.Act(it, common.ReviewAct{Action: "unschedule"}, false)
	}
	return self.Act(it, common.ReviewAct{Action: action, When: when}, false)
}

// Tag edits the item's own tags: words starting + or - change the list, any
// other words replace it.
func (self *Walker) Tag(it common.ReviewItem) Outcome {
	if self.tags == nil {
		self.tags = commands.SendReceiveGetOr[[]string](self.core, "alltags", nil)
	}
	known := self.tags
	if len(known) > 24 {
		known = known[:24]
	}
	hint := "now: " + orNone(it.Tags) + "  ·  +tag adds, -tag removes, plain words replace  ·  known: " + strings.Join(known, " ")
	line := self.Ask("tags:", hint)
	if line == "" {
		return Stay
	}
	tags := ApplyTagEdit(it.Tags, line)
	return self.Act(it, common.ReviewAct{Action: "tags", Tags: tags}, false)
}

func orNone(tags []string) string {
	if len(tags) == 0 {
		return "(none)"
	}
	return ":" + strings.Join(tags, ":") + ":"
}

// ApplyTagEdit is what a tags line means against the tags a heading has:
// "+work -home" changes them, "work urgent" is the new list.
func ApplyTagEdit(have []string, line string) []string {
	words := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == ':' || r == ',' })
	delta := len(words) > 0
	for _, w := range words {
		if !strings.HasPrefix(w, "+") && !strings.HasPrefix(w, "-") {
			delta = false
		}
	}
	out := []string{}
	if !delta {
		seen := map[string]bool{}
		for _, w := range words {
			if !seen[w] {
				seen[w] = true
				out = append(out, w)
			}
		}
		return out
	}
	out = append(out, have...)
	for _, w := range words {
		name := w[1:]
		if name == "" {
			continue
		}
		at := -1
		for i, t := range out {
			if strings.EqualFold(t, name) {
				at = i
			}
		}
		if w[0] == '+' && at < 0 {
			out = append(out, name)
		}
		if w[0] == '-' && at >= 0 {
			out = append(out[:at], out[at+1:]...)
		}
	}
	return out
}

// Summary prints what the walk did, if anything.
func (self *Walker) Summary() {
	if line := TallyLine(self.Tally); line != "" {
		fmt.Printf("  %s%s%s\n", commands.C(commands.AnsiDim), line, commands.C(commands.AnsiReset))
	}
}

// TallyLine is a tally in words: "3 refiled, 1 archived".
func TallyLine(t map[string]int) string {
	order := []string{"refiled", "archived", "done", "scheduled", "unscheduled", "tagged", "promoted",
		"next actions added", "keyword changed", "chased"}
	parts := []string{}
	for _, k := range order {
		if t[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", t[k], k))
		}
	}
	return strings.Join(parts, ", ")
}

// Line is one item as a line of a listing.
func Line(it common.ReviewItem) string {
	kw := ""
	if it.Status != "" {
		kw = commands.C(commands.AnsiGold) + it.Status + commands.C(commands.AnsiReset) + " "
	}
	tags := ""
	if len(it.Tags) > 0 {
		tags = "  " + commands.C(commands.AnsiCyan) + ":" + strings.Join(it.Tags, ":") + ":" + commands.C(commands.AnsiReset)
	}
	return fmt.Sprintf("%s%s%s  %s%s:%d%s", kw, it.Headline, tags, commands.C(commands.AnsiDim),
		commands.BaseName(it.Filename), it.LineNum+1, commands.C(commands.AnsiReset))
}
