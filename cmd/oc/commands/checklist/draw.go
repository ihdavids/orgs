package checklist

// The look, after taskbook: a board's name underlined with its count beside
// it, each item numbered in grey with a coloured mark, and a footer saying how
// much of everything is done.
//
//	  Groceries [2/4]  inbox.org
//	    1. ✔  milk
//	    2. ☐  eggs
//	    3. …  the party
//	         4. ✔  balloons
//
//	  50% of all tasks complete.
//	  2 done · 1 in-progress · 1 pending

import (
	"fmt"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

const (
	ansiGrey      = "\033[90m"
	ansiUnderline = "\033[4m"
)

// The marks, and the colours they are drawn in.
func mark(it Item) (string, string) {
	switch {
	case it.Doing():
		return "…", commands.AnsiGold
	case it.Done():
		return "✔", commands.AnsiGreen
	}
	return "☐", commands.AnsiPurple
}

func c(code string) string { return commands.C(code) }

// header is a list's title line.
func header(l List) string {
	count := fmt.Sprintf("[%d/%d]", l.Done, l.Total)
	return fmt.Sprintf("  %s%s%s%s %s%s%s  %s%s%s", c(commands.AnsiBold), c(ansiUnderline), l.Title, c(commands.AnsiReset),
		c(ansiGrey), count, c(commands.AnsiReset), c(ansiGrey), commands.BaseName(l.File), c(commands.AnsiReset))
}

// itemLine is one numbered item, its number padded to the widest on the list.
func itemLine(it Item, width int) string {
	m, col := mark(it)
	num := fmt.Sprintf("%*d.", width, it.Index+1)
	text := it.Text
	if it.Done() {
		text = c(ansiGrey) + text + c(commands.AnsiReset)
	}
	return fmt.Sprintf("    %s%s%s%s %s%s%s  %s", strings.Repeat("  ", it.Depth), c(ansiGrey), num, c(commands.AnsiReset),
		c(col), m, c(commands.AnsiReset), text)
}

func printList(l List) {
	fmt.Println(header(l))
	width := len(fmt.Sprint(len(l.Items)))
	if len(l.Items) == 0 {
		fmt.Printf("    %sno items%s\n", c(ansiGrey), c(commands.AnsiReset))
	}
	for _, it := range l.Items {
		fmt.Println(itemLine(it, width))
	}
}

// footer is taskbook's summary over every list drawn.
func footer(lists []List) {
	done, doing, total := 0, 0, 0
	for _, l := range lists {
		done += l.Done
		doing += l.Doing
		total += l.Total
	}
	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}
	fmt.Println()
	fmt.Printf("  %s%d%%%s %sof all tasks complete.%s\n", c(commands.AnsiGreen), pct, c(commands.AnsiReset), c(ansiGrey), c(commands.AnsiReset))
	fmt.Printf("  %s%d%s %sdone · %s%s%d%s %sin-progress · %s%s%d%s %spending%s\n",
		c(commands.AnsiGreen), done, c(commands.AnsiReset), c(ansiGrey), c(commands.AnsiReset),
		c(commands.AnsiBlue), doing, c(commands.AnsiReset), c(ansiGrey), c(commands.AnsiReset),
		c(commands.AnsiPurple), total-done-doing, c(commands.AnsiReset), c(ansiGrey), c(commands.AnsiReset))
}

func printLists(lists []List) {
	fmt.Println()
	for i, l := range lists {
		if i > 0 {
			fmt.Println()
		}
		printList(l)
	}
	footer(lists)
}

// pickLine is one row of the fzf picker: count first so the lists line up,
// then the title, then where it lives, which is searched as well.
func pickLine(l List) string {
	count := fmt.Sprintf("%d/%d", l.Done, l.Total)
	col := commands.AnsiPurple
	if l.Total > 0 && l.Done == l.Total {
		col = commands.AnsiGreen
	}
	return fmt.Sprintf("%s%7s%s  %s  %s%s:%d%s", c(col), count, c(commands.AnsiReset), l.Title,
		c(ansiGrey), commands.BaseName(l.File), l.Line+1, c(commands.AnsiReset))
}
