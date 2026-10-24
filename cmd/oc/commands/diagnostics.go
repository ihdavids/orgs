package commands

// One spelling for "something is wrong at this place in this file", shared by
// every command that finds problems: `lint`, `review`, `deps -check`,
// `fmt -check`, `doctor`.
//
// `-format quickfix` prints each one as
//
//	file:line:col: severity: message [rule]
//
// which is what vim's quickfix (`:cexpr system(...)`), Emacs `compile`, a VS
// Code problem matcher and most CI annotators read without being told
// anything. The name is a magic value of -format rather than a flag of its
// own, so every command already has it; a real template never says
// `quickfix`, since one with no `{{` would print the same word on every row.
// The editor needs a path it can open, which is the server's own path for
// the file - fine whenever the client shares the server's disk, which is
// every case where an editor would open it. A path under the working
// directory is printed relative to it, because a CI annotator only matches a
// file it can find in the checkout; -json keeps the absolute path.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Diagnostic is one problem at one place. Line and Col are 1 based; 0 means
// unknown, and the quickfix line leaves them out rather than pointing at a
// line that is not there.
type Diagnostic struct {
	File     string
	Line     int    `json:",omitempty"`
	Col      int    `json:",omitempty"`
	Severity string // error, warning, info
	Rule     string `json:",omitempty"`
	Msg      string
	// What to write instead, when the checker offered something.
	Fix []string `json:",omitempty"`
}

// QuickfixFormat reports whether -format asked for quickfix lines.
func QuickfixFormat() bool { return strings.EqualFold(strings.TrimSpace(FormatOut), "quickfix") }

// QuickfixLine is a diagnostic as one line of compiler output.
func QuickfixLine(d Diagnostic) string {
	var b strings.Builder
	b.WriteString(displayPath(d.File))
	if d.Line > 0 {
		fmt.Fprintf(&b, ":%d", d.Line)
		if d.Col > 0 {
			fmt.Fprintf(&b, ":%d", d.Col)
		}
	}
	sev := d.Severity
	if sev == "" {
		sev = "warning"
	}
	fmt.Fprintf(&b, ": %s: %s", sev, oneLine(d.Msg))
	if d.Rule != "" {
		fmt.Fprintf(&b, " [%s]", d.Rule)
	}
	return b.String()
}

// displayPath is a file as a person or an annotator wants to see it: relative
// to where the command was run when it is somewhere below it, as given
// otherwise.
func displayPath(p string) string {
	if !filepath.IsAbs(p) {
		return p
	}
	wd, err := os.Getwd()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(wd, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return p
	}
	return rel
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// RenderDiagnostics prints problems the way the flags asked for: quickfix
// lines, json, a template per problem, or plain() for a person. It reports
// whether it handled them, the same as Render.
func RenderDiagnostics(diags []Diagnostic, plain func()) bool {
	if QuickfixFormat() {
		for _, d := range diags {
			fmt.Fprintln(os.Stdout, QuickfixLine(d))
		}
		return true
	}
	if diags == nil {
		diags = []Diagnostic{}
	}
	return Render(diags, plain)
}

// PrintDiagnostics is the plain listing a person reads: the place dimmed, the
// severity coloured, the message, and the suggestion when there is one.
func PrintDiagnostics(diags []Diagnostic) {
	for _, d := range diags {
		place := displayPath(d.File)
		if d.Line > 0 {
			place = fmt.Sprintf("%s:%d", place, d.Line)
			if d.Col > 0 {
				place = fmt.Sprintf("%s:%d", place, d.Col)
			}
		}
		col := AnsiGold
		switch d.Severity {
		case "error":
			col = AnsiRed
		case "info":
			col = AnsiBlue
		}
		sev := d.Severity
		if sev == "" {
			sev = "warning"
		}
		fmt.Printf("%s%s%s %s%s%s %s", C(AnsiDim), place, C(AnsiReset), C(col), sev, C(AnsiReset), oneLine(d.Msg))
		if d.Rule != "" {
			fmt.Printf(" %s[%s]%s", C(AnsiDim), d.Rule, C(AnsiReset))
		}
		fmt.Println()
		if len(d.Fix) > 0 {
			fixes := make([]string, len(d.Fix))
			for i, f := range d.Fix {
				// A fix that is only spaces - "one space here, not two" -
				// would print as nothing at all after the arrow.
				if strings.TrimSpace(f) == "" {
					f = fmt.Sprintf("%q", f)
				}
				fixes[i] = f
			}
			fmt.Printf("    %s→ %s%s\n", C(AnsiGreen), strings.Join(fixes, " | "), C(AnsiReset))
		}
	}
}
