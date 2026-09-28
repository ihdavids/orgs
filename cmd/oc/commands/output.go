package commands

// Machine readable output, and the dry run, for every command at once.
//
// A command does not ask for these. `Config.AddCommands` calls AddGlobalFlags
// on each command's flag set *after* the command has registered its own, and
// AddGlobalFlags only defines a name that is still free - so the dnd client
// keeps its own -json (a saved D&D Beyond payload) and its own -format (html,
// latex or pdf), and every other command grows both. A command specific flag
// always wins, which is the right way round: the command is the thing being
// run.
//
// Three ways out of a command, in the order they are tried:
//
//   -json            the answer, as json, indented
//   -format TMPL     one line per row, TMPL being a Go text/template
//   neither          whatever the command prints for a person
//
// Which means a command writes its listing once, hands the rows to Render,
// and is scriptable without knowing it.

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/term"
)

var (
	// Print the answer as json rather than as a listing.
	JsonOut bool
	// A Go text/template run once per row.
	FormatOut string
	// Say what would be written, and write nothing.
	DryRun bool
	// Never emit ansi, whatever the terminal says.
	NoColor bool
)

// AddGlobalFlags gives one command the flags every command has. Each is only
// defined when the command has not already taken the name - flag panics on a
// redefinition, and the command's own meaning is the one to keep.
func AddGlobalFlags(fset *flag.FlagSet) {
	if fset.Lookup("json") == nil {
		fset.BoolVar(&JsonOut, "json", false, "print the answer as json")
	}
	if fset.Lookup("format") == nil {
		fset.StringVar(&FormatOut, "format", "",
			"print each row through a Go template, e.g. '{{.Filename}}:{{.LineNum}} {{.Headline}}'")
	}
	if fset.Lookup("dry-run") == nil {
		fset.BoolVar(&DryRun, "dry-run", false, "say what would be written and write nothing")
	}
	if fset.Lookup("no-color") == nil {
		fset.BoolVar(&NoColor, "no-color", false, "no ansi colour, whatever the terminal says")
	}
}

// Machine reports whether the caller is being read by a program rather than a
// person, which is the question a command asks before printing a banner.
func Machine() bool { return JsonOut || FormatOut != "" }

// Colour reports whether ansi is worth emitting: a terminal, not turned off,
// and not on its way into a pipe or a json document.
func Colour() bool {
	if NoColor || Machine() || os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// The minimal ansi the terminal clients share. Each is empty when colour is
// off, so a caller can concatenate without branching.
func C(code string) string {
	if !Colour() {
		return ""
	}
	return code
}

const (
	AnsiReset  = "\033[0m"
	AnsiBold   = "\033[1m"
	AnsiDim    = "\033[2m"
	AnsiRed    = "\033[31m"
	AnsiGreen  = "\033[32m"
	AnsiGold   = "\033[33m"
	AnsiBlue   = "\033[34m"
	AnsiPurple = "\033[35m"
	AnsiCyan   = "\033[36m"
)

var tmplFuncs = template.FuncMap{
	"base":  filepath.Base,
	"dir":   filepath.Dir,
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"trim":  strings.TrimSpace,
	"join":  func(sep string, v []string) string { return strings.Join(v, sep) },
	// Truncate with an ellipsis, for a column that has to stay a column.
	"cut": func(n int, s string) string {
		if n <= 1 || len(s) <= n {
			return s
		}
		return s[:n-1] + "…"
	},
	// Pad to a width, so a template can lay out columns without the caller
	// having to measure anything first.
	"pad": func(n int, s string) string {
		for len(s) < n {
			s += " "
		}
		return s
	},
	// A map's value by key, for the Props map every heading carries.
	"prop": func(m map[string]string, k string) string { return m[k] },
}

// A -format string is typed at a shell, so the escapes it carries are the
// shell's spelling rather than Go's. Turn the two that matter into what they
// stand for, and leave everything else alone.
func unescape(s string) string {
	r := strings.NewReplacer(`\t`, "\t", `\n`, "\n", `\\`, `\`)
	return r.Replace(s)
}

// Render prints rows the way the flags asked for, and reports whether it
// handled them. When it returns false the caller prints its own listing -
// which is what plain is for, if the caller would rather pass a closure than
// check the answer.
func Render[T any](rows []T, plain func()) bool {
	switch {
	case JsonOut:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			fmt.Fprintf(os.Stderr, "could not write json: %v\n", err)
		}
		return true
	case FormatOut != "":
		t, err := template.New("row").Funcs(tmplFuncs).Parse(unescape(FormatOut))
		if err != nil {
			fmt.Fprintf(os.Stderr, "-format is not a template yet: %v\n", err)
			os.Exit(1)
		}
		for _, row := range rows {
			if err := t.Execute(os.Stdout, row); err != nil {
				fmt.Fprintf(os.Stderr, "\n-format: %v\n", err)
				os.Exit(1)
			}
			fmt.Println()
		}
		return true
	}
	if plain != nil {
		plain()
	}
	return false
}

// RenderOne is Render for an answer that is one thing rather than a list. The
// template is run over it once; json prints the object rather than an array
// of one, because a caller piping into jq should not have to index past a
// wrapper this side invented.
func RenderOne[T any](row T, plain func()) bool {
	switch {
	case JsonOut:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(row); err != nil {
			fmt.Fprintf(os.Stderr, "could not write json: %v\n", err)
		}
		return true
	case FormatOut != "":
		return Render([]T{row}, nil)
	}
	if plain != nil {
		plain()
	}
	return false
}

// Fail says what went wrong the way the caller asked to be told: as json when
// something is reading json, and as a line on stderr otherwise. It does not
// return.
func Fail(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if JsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]interface{}{"Ok": false, "Msg": msg})
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", msg)
	}
	os.Exit(1)
}

// ---------------------------------------------------------------------------
// The dry run
// ---------------------------------------------------------------------------

// Every write a command makes goes out through SendReceivePost, so that is
// where -dry-run is answered rather than in each command: a command written
// tomorrow gets it without knowing about it, and one that forgets cannot
// write anyway.
//
// It is not a complete promise and says so. A handful of GET endpoints write
// as a side effect - an exporter asked to put its answer on the server's disk
// is the one that matters - so those are checked in the commands that call
// them (see Wrote).

// Wrote is what a command calls when it is about to write through something
// other than a POST. It prints what would happen and reports whether the
// caller should stop.
func Wrote(what string, detail interface{}) bool {
	if !DryRun {
		return false
	}
	fmt.Fprintf(os.Stderr, "%sdry run%s %s\n", C(AnsiGold), C(AnsiReset), what)
	if detail != nil {
		if b, err := json.MarshalIndent(detail, "  ", "  "); err == nil {
			fmt.Fprintf(os.Stderr, "  %s\n", string(b))
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Words and flags in any order
// ---------------------------------------------------------------------------

// FreeArgs returns everything on the command line that was not a flag, having
// parsed the flags wherever they were written.
//
// Go's flag package stops at the first argument that is not a flag, so
// `orgs search 'IsTask()' -json` parses no flags at all: -json ends up as a
// second word of the query and the listing comes out in the wrong format with
// nothing to say why. Since the natural way to type these commands is to put
// the thing you are looking for first, the parse is run again after each word
// is taken off - which is what the dnd and contact clients have always done.
func FreeArgs(fset *flag.FlagSet) []string {
	if fset == nil {
		return nil
	}
	words := []string{}
	args := fset.Args()
	for {
		// `--` is the shell's own way of saying "no more flags", and a re-parse
		// that did not honour it would read the words after it as flags again -
		// which is exactly what somebody writing it was protecting against.
		if i := indexOf(args, "--"); i >= 0 {
			if err := fset.Parse(args[:i]); err == nil {
				words = append(words, fset.Args()...)
			}
			return append(words, args[i+1:]...)
		}
		if err := fset.Parse(args); err != nil {
			return words
		}
		rest := fset.Args()
		if len(rest) == 0 {
			return words
		}
		words = append(words, rest[0])
		args = rest[1:]
	}
}

func indexOf(hay []string, needle string) int {
	for i, s := range hay {
		if s == needle {
			return i
		}
	}
	return -1
}

// FreeText is FreeArgs joined back up, for a command whose words are one
// thing: a query, a pattern, a name.
func FreeText(fset *flag.FlagSet) string {
	return strings.TrimSpace(strings.Join(FreeArgs(fset), " "))
}

// HashPath turns a heading's hash into the path segment the server expects.
//
// A hash arrives on a Todo as base64 already - `hOpOB7vIg6oiYz5sMVSlzGiJXic=`
// - and `GetHash` in rest.go base64-URL-*decodes* the segment before looking
// it up, so the segment is the hash encoded a second time. Written into a url
// as it stands, the `+` becomes a space and the `/` a path separator, and the
// request goes somewhere else entirely with nothing to say so: the handler
// answers "no heading with that hash", which reads like a stale hash rather
// than a mangled one.
func HashPath(hash string) string {
	return base64.URLEncoding.EncodeToString([]byte(hash))
}

// StdinIsPipe reports whether something is being piped in, which is how a
// write verb knows it was handed a list of headings rather than asked to go
// and find some. It is the opposite question to Interactive() rather than the
// same one: a cron job has neither a pipe nor a terminal.
func StdinIsPipe() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) == 0
}

// Interactive reports whether there is a person at the other end to answer a
// question. A pipe, a `-json` run and a cron job all answer no, and anything
// about to prompt has to ask first - a chooser or a "run it? [y/N]" put to
// something that cannot type is a hang, not a question.
func Interactive() bool {
	if Machine() {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
