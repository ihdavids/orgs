package commands

// The fzf picker with a pane beside it, and the drawing the pane is made of.
//
// `orgs grep` was the first of these and hands its pane to `bat`, which knows
// how to colour a file and nothing about org. Everything since - a source
// block, a link, a table - is not a file: it is a thing the server knows about,
// with parts that have to be laid out. So the pane is drawn by *this binary*,
// and fzf's `--preview` shells out to a second run of it.
//
// Four things follow from that, and each was a bug before it was a rule:
//
//  1. **The child has to reach the same server.** It is a fresh process with a
//     fresh config load, so -config and -url are passed through explicitly
//     rather than left to defaults that resolve against a working directory
//     fzf's child does not necessarily have.
//  2. **The command is a shell string**, so every path in it is single quoted
//     (`Shq`). fzf quotes the `{1}`/`{2}` it substitutes, so those are left
//     alone.
//  3. **The list is tab separated.** `orgs grep` splits on "|" and a path with
//     a pipe in it takes the line apart in the wrong place. The address fields
//     come first and are hidden; what is shown is also what is searched.
//  4. **The pane draws to whatever width it is given** - FZF_PREVIEW_COLUMNS,
//     then COLUMNS, then 80.

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	fzf "github.com/junegunn/fzf/src"
)

// The field separator in a picker line.
const Sep = "\t"

// ---------------------------------------------------------------------------
// Running the picker
// ---------------------------------------------------------------------------

type PickOpts struct {
	// One line per row: the address fields, then the text to show, joined with
	// Sep. Build them with PickLine.
	Lines []string
	// How many leading fields are the address rather than the display. They are
	// hidden from the list and handed to the preview command as {1}, {2}, …
	AddressFields int
	// What to put in front of the box.
	Prompt string
	// The line above the list saying which keys do what.
	Header string
	// The command the pane runs, with {1}/{2} for the address fields. Empty
	// means no pane.
	Preview string
	// Extra fzf arguments - key bindings, mostly.
	Extra []string
}

// PickLine joins an address and a display into one row of the list.
func PickLine(address []string, display string) string {
	return strings.Join(append(append([]string{}, address...), display), Sep)
}

// Address takes the leading fields back off a line fzf handed back.
func Address(line string, n int) ([]string, bool) {
	parts := strings.Split(line, Sep)
	if len(parts) < n+1 {
		return nil, false
	}
	return parts[:n], true
}

// Run the picker and hand back the lines that were chosen.
//
// The output channel is closed and drained before returning, because fzf's
// writer goroutine and this one are otherwise racing over the slice: reading it
// straight after `fzf.Run` sometimes gave an empty selection on a fast machine.
func Pick(opts PickOpts) []string {
	return runPick(opts, nil)
}

// PickKey is Pick that also says which key closed it: "" for enter, or one of
// keys (fzf's --expect). For a list where enter does one thing and another key
// does another, and the other thing needs the picker gone first - an editor
// in the same terminal, say.
func PickKey(opts PickOpts, keys ...string) (string, []string) {
	if len(keys) == 0 {
		return "", Pick(opts)
	}
	out := runPick(opts, []string{"--expect", strings.Join(keys, ",")})
	if len(out) == 0 {
		return "", nil
	}
	return out[0], out[1:]
}

func runPick(opts PickOpts, more []string) []string {
	if len(opts.Lines) == 0 {
		return nil
	}
	args := []string{
		"--ansi",
		"--reverse",
		"--border",
		"--delimiter", Sep,
		"--with-nth", fmt.Sprintf("%d..", opts.AddressFields+1),
		"--bind", "ctrl-/:toggle-preview",
	}
	if opts.Prompt != "" {
		args = append(args, "--prompt", opts.Prompt)
	}
	if opts.Header != "" {
		args = append(args, "--header", opts.Header)
	}
	if opts.Preview != "" {
		args = append(args,
			"--preview", opts.Preview,
			"--preview-window", "right,62%,border-left,~2")
	}
	args = append(args, opts.Extra...)
	args = append(args, more...)

	inputChan := make(chan string)
	go func() {
		for _, s := range opts.Lines {
			inputChan <- s
		}
		close(inputChan)
	}()
	out := []string{}
	outputChan := make(chan string)
	done := make(chan struct{})
	go func() {
		for s := range outputChan {
			out = append(out, s)
		}
		close(done)
	}()

	options, err := fzf.ParseOptions(true, args)
	if err != nil {
		Fail("%v", err)
	}
	options.Input = inputChan
	options.Output = outputChan
	fzf.Run(options)
	close(outputChan)
	<-done
	return out
}

// SelfCommand is how to run this binary again with enough of the global flags
// to reach the same server. Quoted, because it is about to be part of a shell
// command.
func SelfCommand(core *Core) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	parts := []string{Shq(exe)}
	if core.ConfigFile != "" {
		parts = append(parts, "-config", Shq(core.ConfigFile))
	}
	if core.Rest.Url != "" {
		parts = append(parts, "-url", Shq(core.Rest.Url))
	}
	return strings.Join(parts, " "), nil
}

func Shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ---------------------------------------------------------------------------
// Drawing the pane
// ---------------------------------------------------------------------------

// PaneWidth is what fzf said the pane is, or what the terminal said, or 80.
func PaneWidth() int {
	for _, key := range []string{"FZF_PREVIEW_COLUMNS", "COLUMNS"} {
		if v := os.Getenv(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 20 {
				return n
			}
		}
	}
	return 80
}

// The boxes a pane is made of are **open on the right**.
//
// A box that closes has to know the display width of every line inside it, and
// a line holding a tab, an east asian character or an escape sequence makes
// that a guess. A box with one ragged edge reads as a box; one with the wrong
// edge reads as a bug.
func OpenBox(label string, width int) {
	head := "┌─ " + label + " "
	fmt.Printf("%s%s%s%s\n", C(AnsiDim), head, Rule(width-RuneLen(head)), C(AnsiReset))
}

func CloseBox(width int) {
	fmt.Printf("%s└%s%s\n", C(AnsiDim), Rule(width-1), C(AnsiReset))
}

// BoxLine is one line inside a box: the bar, then whatever was given.
func BoxLine(text string) {
	fmt.Printf("%s│%s %s\n", C(AnsiDim), C(AnsiReset), text)
}

// BoxText is a run of lines inside a box, split the way they were written.
func BoxText(text string) {
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		BoxLine(l)
	}
}

func Rule(n int) string {
	if n < 0 {
		n = 0
	}
	return strings.Repeat("─", n)
}

// RuneLen is the length in characters rather than in bytes, which is what a
// rule has to be measured in or every box with an accent in its label comes out
// short.
func RuneLen(s string) int { return len([]rune(s)) }

// BaseName is the file, said the way a listing says one.
func BaseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// Ellipsis trims to n characters, counting characters rather than bytes.
func Ellipsis(s string, n int) string {
	r := []rune(s)
	if n <= 1 || len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// OpenInBrowser hands a url to whatever this desktop opens urls with.
//
// It decides nothing itself. A `doi:` or a `mailto:` has a right answer and it
// is not this program's to have an opinion about; the desktop already knows.
// Started rather than waited on, so a browser that takes four seconds to come
// up does not hold the picker.
func OpenInBrowser(url string) {
	if strings.TrimSpace(url) == "" {
		return
	}
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		name = "xdg-open"
	}
	cmd := exec.Command(name, append(args, url)...)
	cmd.Start()
}
