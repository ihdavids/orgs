package code

// The preview pane, and the picker that shows it.
//
// `orgs code` with no subcommand is an fzf picker with the block drawn beside
// it, the way `orgs grep` is an fzf picker with the file drawn beside it. The
// difference is what draws the pane: grep hands the job to `bat`, which knows
// how to colour a file and nothing about org. A source block is not a file - it
// is a header line, a set of variables, some code, and possibly what it last
// produced - and the only thing that can lay that out is this.
//
// So the preview is a second run of this same binary: fzf's --preview shells
// out, and what it shells out to is `orgs code preview -file F -id N`. The
// machinery for that - running the picker, finding this binary again, quoting,
// and the boxes - is `cmd/oc/commands/picker.go`, shared with the links and
// tables pickers. What is here is only what a source block is.

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// The picker
// ---------------------------------------------------------------------------

func (self *Code) pick(core *commands.Core, q string) {
	// Nothing is reading a chooser. A pipe, a -json run or a cron job gets the
	// listing, which is the same question answered in the form that caller can
	// use - rather than a full screen application it cannot see or answer.
	if !commands.Interactive() {
		self.list(core, q)
		return
	}
	blocks := self.index(core, q).Blocks
	if len(blocks) == 0 {
		fmt.Fprintln(os.Stderr, "no source blocks matched")
		return
	}

	self2, err := commands.SelfCommand(core)
	if err != nil {
		// No way to draw a pane without being able to run ourselves again. The
		// list is still worth showing, so fall back rather than refuse.
		fmt.Fprintf(os.Stderr, "orgs code: no preview (%v)\n", err)
	}

	lines := make([]string, 0, len(blocks))
	for _, b := range blocks {
		lines = append(lines, commands.PickLine([]string{b.Filename, strconv.Itoa(b.Id)}, pickLine(b)))
	}

	opts := commands.PickOpts{
		Lines:         lines,
		AddressFields: 2,
		Prompt:        "block> ",
		Header:        "enter: show · ctrl-r: run · ctrl-o: edit · ctrl-/: hide pane",
	}
	if self2 != "" {
		opts.Preview = self2 + " code preview -file {1} -id {2}"
		// Running from inside the picker, and staying there afterwards. This is
		// the one people will use: the point of a preview pane is deciding
		// whether to run the thing, and going back out to type the command
		// again loses the place in the list.
		opts.Extra = []string{
			"--bind", "ctrl-r:execute(" + self2 + " code run -file {1} -id {2} -v 2>&1;" +
				` printf '\n── press enter ──'; read -r _)`,
		}
	}

	for _, chosen := range commands.Pick(opts) {
		addr, ok := commands.Address(chosen, 2)
		if !ok {
			continue
		}
		id, cerr := strconv.Atoi(strings.TrimSpace(addr[1]))
		if cerr != nil {
			continue
		}
		b, found := byAddress(blocks, addr[0], id)
		if !found {
			continue
		}
		if self.Open {
			core.LaunchEditor(b.Filename, b.Line+1)
			continue
		}
		self.detail(b)
		self.offerRun(core, b)
	}
}

// One line of the picker. The name first, because that is what another block
// calls it by and what somebody is most likely to be looking for; then the
// language, what it reads, the heading it sits under, and the file - which is
// in the display rather than only in the address fields so that it can be
// searched for.
func pickLine(b common.CodeBlock) string {
	name := b.Name
	if name == "" {
		// An unnamed block has no handle but its position, and drawing that as
		// though it were a name would be a lie about what it is.
		name = commands.C(commands.AnsiDim) + "(unnamed)" + commands.C(commands.AnsiReset)
	} else {
		name = commands.C(commands.AnsiBold) + name + commands.C(commands.AnsiReset)
	}
	lang := b.Lang
	if lang == "" {
		lang = "?"
	}
	where := b.Heading
	if where == "" {
		where = "(preamble)"
	}
	ran := ""
	if b.Result != "" {
		ran = commands.C(commands.AnsiGreen) + " ·ran" + commands.C(commands.AnsiReset)
	}
	reads := ""
	if names := readsFrom(b); names != "" {
		reads = fmt.Sprintf("  %s← %s%s", commands.C(commands.AnsiPurple), names, commands.C(commands.AnsiReset))
	}
	return fmt.Sprintf("%s  %s[%s]%s%s  %s%s  %s%s:%d%s",
		name,
		commands.C(commands.AnsiCyan), lang, commands.C(commands.AnsiReset),
		reads,
		where, ran,
		commands.C(commands.AnsiDim), b.Filename, b.Line+1, commands.C(commands.AnsiReset))
}

func readsFrom(b common.CodeBlock) string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range b.Vars {
		if v.Ref == "" || seen[v.Ref] {
			continue
		}
		seen[v.Ref] = true
		out = append(out, v.Ref)
	}
	return strings.Join(out, ", ")
}

// Find a block by the file it is in and its position in that file.
//
// The file is matched three ways, in order: the exact path, the base name, and
// any tail of the path. The picker hands back the exact path it was given, so
// it only ever needs the first - but a person typing the address by hand types
// `-file notes.org`, and refusing that while printing the block as
// "notes.org:3" in the listing is telling somebody to use a name and then not
// accepting it.
func byAddress(blocks []common.CodeBlock, filename string, id int) (common.CodeBlock, bool) {
	want := strings.ToLower(filename)
	for _, b := range blocks {
		if b.Id == id && b.Filename == filename {
			return b, true
		}
	}
	for _, b := range blocks {
		if b.Id != id {
			continue
		}
		low := strings.ToLower(b.Filename)
		if strings.EqualFold(commands.BaseName(b.Filename), filename) || strings.HasSuffix(low, want) {
			return b, true
		}
	}
	return common.CodeBlock{}, false
}

// ---------------------------------------------------------------------------
// After the pick: offer to run it
// ---------------------------------------------------------------------------

// A block picked out of the list is usually picked in order to run it, so ask -
// but only ask. Running somebody's program because they pressed enter on a list
// is not what enter on a list means.
func (self *Code) offerRun(core *commands.Core, b common.CodeBlock) {
	if self.NoRun {
		return
	}
	if !self.Run {
		if !commands.Interactive() {
			return
		}
		fmt.Printf("\n%sRun it? [y/N] %s", commands.C(commands.AnsiBold), commands.C(commands.AnsiReset))
		var answer string
		fmt.Scanln(&answer)
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			return
		}
	}
	fmt.Println()
	self.runBlock(core, b)
}

// ---------------------------------------------------------------------------
// The pane
// ---------------------------------------------------------------------------

func (self *Code) preview(core *commands.Core) {
	if self.File == "" || self.Id < 0 {
		commands.Fail("orgs code preview: -file and -id say which block")
	}
	// Narrowed by the file name so the whole index is not walked for one block.
	// The server matches a file name inside code text as well, so the exact
	// match below is what actually picks it.
	b, ok := byAddress(self.index(core, commands.BaseName(self.File)).Blocks, self.File, self.Id)
	if !ok {
		if b, ok = byAddress(self.index(core, "").Blocks, self.File, self.Id); !ok {
			fmt.Printf("%s%s:%d is not a source block any more%s\n",
				commands.C(commands.AnsiDim), commands.BaseName(self.File), self.Id, commands.C(commands.AnsiReset))
			return
		}
	}
	renderPane(b, commands.PaneWidth())
}

// The pane: what the block is given, the code, and what it last produced.
//
// The boxes are open on the right. A box that closes has to know the display
// width of every line inside it, and a line holding a tab, a wide character or
// an escape sequence makes that a guess - a box with one ragged edge reads as a
// box, and one with the wrong edge reads as a bug.
func renderPane(b common.CodeBlock, width int) {
	// ---- the header, which is what the block is called and where it lives
	name := b.Name
	if name == "" {
		name = "(unnamed)"
	}
	fmt.Printf("%s%s%s  %s%s%s\n",
		commands.C(commands.AnsiBold), name, commands.C(commands.AnsiReset),
		commands.C(commands.AnsiCyan), b.Lang, commands.C(commands.AnsiReset))
	where := b.Heading
	if where == "" {
		where = "(preamble)"
	}
	fmt.Printf("%s%s — %s:%d%s\n\n",
		commands.C(commands.AnsiDim), where, commands.BaseName(b.Filename), b.Line+1, commands.C(commands.AnsiReset))

	// ---- inputs: the switches, the header arguments, and the variables
	inputs := inputLines(b)
	if len(inputs) > 0 {
		commands.OpenBox("inputs", width)
		for _, l := range inputs {
			commands.BoxLine(l)
		}
		commands.CloseBox(width)
		fmt.Println()
	}

	// ---- the code
	fmt.Print(colourCode(b))

	// ---- results, when the block has been run
	if strings.TrimSpace(b.Result) != "" {
		fmt.Println()
		label := "results"
		if b.ResultKind != "" {
			label = "results · " + b.ResultKind
		}
		commands.OpenBox(label, width)
		commands.BoxText(b.Result)
		commands.CloseBox(width)
	}
}

// Everything the block is handed, said the way org writes it, so that what is
// in the box is what is on the #+BEGIN_SRC line rather than a summary of it.
//
// A variable that names something in the file is the one worth drawing
// carefully: `:var data=monthly` and `:var scale=2` are the same shape and mean
// entirely different things, and only the server can tell which. A name that
// resolves to nothing is a block that cannot run, and is said in danger colours
// rather than left looking ordinary.
func inputLines(b common.CodeBlock) []string {
	out := []string{}
	if len(b.Switches) > 0 {
		out = append(out, fmt.Sprintf("%sswitches%s %s",
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset), strings.Join(b.Switches, " ")))
	}
	for _, v := range b.Vars {
		switch {
		case v.Ref == "":
			out = append(out, fmt.Sprintf(":var %s%s%s=%s",
				commands.C(commands.AnsiBold), v.Name, commands.C(commands.AnsiReset), v.Value))
		case v.RefKind == "":
			out = append(out, fmt.Sprintf(":var %s%s%s=%s %s→ nothing%s",
				commands.C(commands.AnsiBold), v.Name, commands.C(commands.AnsiReset), v.Value,
				commands.C(commands.AnsiRed), commands.C(commands.AnsiReset)))
		case v.Rows > 0:
			out = append(out, fmt.Sprintf(":var %s%s%s=%s %s→ %s %d×%d in %s%s",
				commands.C(commands.AnsiBold), v.Name, commands.C(commands.AnsiReset), v.Value,
				commands.C(commands.AnsiGreen), v.RefKind, v.Rows, v.Cols,
				commands.BaseName(v.RefFile), commands.C(commands.AnsiReset)))
		default:
			out = append(out, fmt.Sprintf(":var %s%s%s=%s %s→ %s in %s%s",
				commands.C(commands.AnsiBold), v.Name, commands.C(commands.AnsiReset), v.Value,
				commands.C(commands.AnsiGreen), v.RefKind,
				commands.BaseName(v.RefFile), commands.C(commands.AnsiReset)))
		}
	}
	for _, a := range b.Args {
		out = append(out, fmt.Sprintf(":%s %s", a.Key, a.Value))
	}
	return out
}

// The code, coloured by `bat` when it is there and printed plainly when it is
// not. `orgs grep` already leans on bat for its pane, so this is not a new
// dependency - but a pane that is blank because bat is missing would be a new
// failure, so the fallback is a real one rather than an error.
func colourCode(b common.CodeBlock) string {
	code := strings.TrimRight(b.Code, "\n") + "\n"
	bat, err := exec.LookPath("bat")
	if err != nil {
		return numbered(code)
	}
	args := []string{"--style", "numbers", "--color", "always", "--paging", "never"}
	if b.Lang != "" {
		args = append(args, "--language", b.Lang)
	}
	cmd := exec.Command(bat, args...)
	cmd.Stdin = strings.NewReader(code)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		// A language bat has never heard of is an error from bat, not a reason
		// to show nothing.
		return numbered(code)
	}
	return string(out)
}

func numbered(code string) string {
	var b strings.Builder
	for i, l := range strings.Split(strings.TrimRight(code, "\n"), "\n") {
		fmt.Fprintf(&b, "%s%4d%s  %s\n",
			commands.C(commands.AnsiDim), i+1, commands.C(commands.AnsiReset), l)
	}
	return b.String()
}
