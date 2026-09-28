package tables

// Drawing a table, and the picker that shows one beside the list.
//
// This is worg's `OrgTableView` in a terminal, and it draws the same three
// things that view does:
//
//  1. **The rulers.** `@1` down the side and `$1` across the top, because a
//     formula is written in those coordinates and a table without them is a
//     table you have to count along with a finger to read `$3=$1*$2`.
//  2. **The rules.** A `|---+---|` line is a rule rather than a row of dashes,
//     and it does not take a row number - org does not count it and neither can
//     this, or every formula below one would be off by however many there are.
//  3. **The formulas, against the cells they fill.** The server hands back
//     `CellFormulas` keyed by "row,col", which is the part nothing else will
//     tell you: a `#+TBLFM:` line at the foot of a table says `$4=$2*$3` and
//     says nothing about where that lands.
//
// The machinery (the picker, finding this binary again, the boxes) is
// `cmd/oc/commands/picker.go`, shared with the code and links pickers.

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// The picker
// ---------------------------------------------------------------------------

func (self *Tables) pick(core *commands.Core, q string) {
	// Nothing is reading a chooser. A pipe, a -json run or a cron job gets the
	// listing, which is the same question in the form that caller can use.
	if !commands.Interactive() {
		self.list(core, q)
		return
	}
	rows := self.index(core, q)
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no tables matched")
		return
	}
	for _, t := range self.choose(core, rows, q) {
		if self.Open {
			core.LaunchEditor(t.Filename, t.Line+1)
			continue
		}
		renderPane(self.fetch(core, t), commands.PaneWidth())
	}
}

// Put the list up and hand back what was chosen. Split out from `pick` so that
// `resolve` can use it when a name matched several tables.
func (self *Tables) choose(core *commands.Core, rows []common.TableInfo, q string) []common.TableInfo {
	self2, err := commands.SelfCommand(core)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orgs tables: no preview (%v)\n", err)
	}

	lines := make([]string, 0, len(rows))
	for _, t := range rows {
		lines = append(lines,
			commands.PickLine([]string{t.Filename, strconv.Itoa(t.Id)}, pickLine(t)))
	}

	opts := commands.PickOpts{
		Lines:         lines,
		AddressFields: 2,
		Prompt:        "table> ",
		Header:        "enter: show · ctrl-e: run its formulas · ctrl-o: edit · ctrl-/: hide pane",
	}
	if self2 != "" {
		opts.Preview = self2 + " tables preview -file {1} -id {2}"
		// Running the formulas from inside the picker. This writes to the org
		// file, which is why it is a key of its own rather than something enter
		// might do: picking a row off a list is reading, not writing.
		opts.Extra = []string{
			"--bind", "ctrl-e:execute(" + self2 + " tables eval -file {1} -id {2} 2>&1;" +
				` printf '\n── press enter ──'; read -r _)`,
			"--bind", "ctrl-o:execute-silent(" + self2 + " tables show -file {1} -id {2} -open)",
		}
	}

	out := []common.TableInfo{}
	for _, chosen := range commands.Pick(opts) {
		addr, ok := commands.Address(chosen, 2)
		if !ok {
			continue
		}
		id, cerr := strconv.Atoi(strings.TrimSpace(addr[1]))
		if cerr != nil {
			continue
		}
		if t, found := byAddress(rows, addr[0], id); found {
			out = append(out, t)
		}
	}
	return out
}

// One line of the picker: the name first, because that is what another block
// calls the table by; then its shape, whether it computes anything, the heading
// it sits under, and the file - which is in the display rather than only in the
// address so that it can be searched for.
func pickLine(t common.TableInfo) string {
	name := t.Name
	if name == "" {
		name = commands.C(commands.AnsiDim) + "(unnamed)" + commands.C(commands.AnsiReset)
	} else {
		name = commands.C(commands.AnsiBold) + name + commands.C(commands.AnsiReset)
	}
	where := t.Heading
	if where == "" {
		where = "(preamble)"
	}
	fx := ""
	if t.Formulas > 0 {
		fx = fmt.Sprintf("  %sƒ%d%s", commands.C(commands.AnsiGreen), t.Formulas, commands.C(commands.AnsiReset))
	}
	return fmt.Sprintf("%s  %s%d×%d%s%s  %s  %s%s:%d%s",
		name,
		commands.C(commands.AnsiCyan), t.Rows, t.Cols, commands.C(commands.AnsiReset), fx,
		where,
		commands.C(commands.AnsiDim), t.Filename, t.Line+1, commands.C(commands.AnsiReset))
}

// ---------------------------------------------------------------------------
// The pane
// ---------------------------------------------------------------------------

func (self *Tables) preview(core *commands.Core) {
	if self.File == "" || self.Id < 0 {
		commands.Fail("orgs tables preview: -file and -id say which table")
	}
	t, ok := byAddress(self.index(core, ""), self.File, self.Id)
	if !ok {
		fmt.Printf("%s%s:%d is not a table any more%s\n",
			commands.C(commands.AnsiDim), commands.BaseName(self.File), self.Id, commands.C(commands.AnsiReset))
		return
	}
	renderPane(self.fetch(core, t), commands.PaneWidth())
}

func renderPane(d *common.TableData, width int) {
	name := d.Name
	if name == "" {
		name = "(unnamed)"
	}
	fmt.Printf("%s%s%s  %s%d×%d%s\n",
		commands.C(commands.AnsiBold), name, commands.C(commands.AnsiReset),
		commands.C(commands.AnsiCyan), d.Rows, d.Cols, commands.C(commands.AnsiReset))
	where := d.Heading
	if where == "" {
		where = "(preamble)"
	}
	fmt.Printf("%s%s — %s:%d%s\n\n",
		commands.C(commands.AnsiDim), where, commands.BaseName(d.Filename), d.Line+1,
		commands.C(commands.AnsiReset))

	drawGrid(d, width)

	if len(d.FormulaList) > 0 {
		fmt.Println()
		commands.OpenBox(fmt.Sprintf("formulas · %d", len(d.FormulaList)), width)
		for _, f := range d.FormulaList {
			commands.BoxLine(commands.C(commands.AnsiGreen) + f + commands.C(commands.AnsiReset))
		}
		commands.CloseBox(width)
	}
	if len(d.Params) > 0 {
		fmt.Println()
		commands.OpenBox("parameters", width)
		for k, v := range d.Params {
			commands.BoxLine(fmt.Sprintf("%s%s%s = %s", commands.C(commands.AnsiBold), k, commands.C(commands.AnsiReset), v))
		}
		commands.CloseBox(width)
	}
}

// The grid itself: the `$n` ruler across the top, the `@n` ruler down the side,
// and a cell marked when a formula writes it.
func drawGrid(d *common.TableData, width int) {
	cols := d.Cols
	if cols <= 0 {
		for _, r := range d.Data {
			if len(r.Cells) > cols {
				cols = len(r.Cells)
			}
		}
	}
	if cols == 0 {
		fmt.Printf("%s(an empty table)%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		return
	}

	// Which cells a formula fills, by the raw row index the server keys on.
	computed := map[string]bool{}
	for key := range d.CellFormulas {
		computed[key] = true
	}

	// Org counts data rows and ignores the rules, and so does a formula. The
	// row ruler has to do the same or every `@n` below a rule points at the
	// wrong line.
	rowNo := make([]int, len(d.Data))
	n := 0
	for i, r := range d.Data {
		if r.Kind == "sep" {
			rowNo[i] = 0
			continue
		}
		n++
		rowNo[i] = n
	}

	// Column widths from the widest cell, the headers included.
	w := make([]int, cols)
	for c := 0; c < cols; c++ {
		w[c] = commands.RuneLen(fmt.Sprintf("$%d", c+1))
	}
	for _, r := range d.Data {
		if r.Kind == "sep" {
			continue
		}
		for c, cell := range r.Cells {
			if c >= cols {
				break
			}
			if l := commands.RuneLen(cell); l > w[c] {
				w[c] = l
			}
		}
	}
	// Trimmed to fit the pane rather than running off the side of it. A table
	// that is too wide is the common case, not the exception.
	gut := commands.RuneLen(fmt.Sprintf("@%d", n)) + 1
	budget := width - gut - 2
	for {
		total := 0
		for _, x := range w {
			total += x + 3
		}
		if total <= budget {
			break
		}
		// Take a character off the widest column, so what gets squeezed is
		// whatever has most room to spare.
		widest, at := 0, -1
		for c, x := range w {
			if x > widest {
				widest, at = x, c
			}
		}
		if at < 0 || widest <= 4 {
			break
		}
		w[at]--
	}

	pad := func(s string, n int) string {
		r := []rune(s)
		if len(r) > n {
			if n <= 1 {
				return string(r[:n])
			}
			return string(r[:n-1]) + "…"
		}
		return s + strings.Repeat(" ", n-len(r))
	}

	// The column ruler.
	var head strings.Builder
	fmt.Fprintf(&head, "%s%s%s", commands.C(commands.AnsiDim), strings.Repeat(" ", gut), commands.C(commands.AnsiReset))
	for c := 0; c < cols; c++ {
		fmt.Fprintf(&head, " %s%s%s ", commands.C(commands.AnsiDim), pad(fmt.Sprintf("$%d", c+1), w[c]), commands.C(commands.AnsiReset))
		if c < cols-1 {
			fmt.Fprint(&head, " ")
		}
	}
	fmt.Println(head.String())

	rule := func() string {
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", gut))
		for c := 0; c < cols; c++ {
			b.WriteString(strings.Repeat("─", w[c]+2))
			if c < cols-1 {
				b.WriteString("┼")
			}
		}
		return commands.C(commands.AnsiDim) + b.String() + commands.C(commands.AnsiReset)
	}

	for i, r := range d.Data {
		if r.Kind == "sep" {
			fmt.Println(rule())
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s%s%s", commands.C(commands.AnsiDim),
			pad(fmt.Sprintf("@%d", rowNo[i]), gut-1)+" ", commands.C(commands.AnsiReset))
		for c := 0; c < cols; c++ {
			cell := ""
			if c < len(r.Cells) {
				cell = r.Cells[c]
			}
			body := pad(cell, w[c])
			// A cell a formula writes is not a cell to edit by hand, and
			// saying which ones they are is most of why this view exists.
			if computed[fmt.Sprintf("%d,%d", i, c)] {
				body = commands.C(commands.AnsiGreen) + body + commands.C(commands.AnsiReset)
			}
			fmt.Fprintf(&b, " %s ", body)
			if c < cols-1 {
				fmt.Fprintf(&b, "%s│%s", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
			}
		}
		fmt.Println(b.String())
	}
}
