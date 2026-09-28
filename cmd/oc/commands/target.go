package commands

// Naming the heading a command is about.
//
// Every write verb - `orgs todo`, `orgs sched`, `orgs tag`, `orgs rm` - has the
// same first problem: which heading? The read commands answer questions and can
// afford to answer about everything they found; a write has to land on exactly
// what was meant, and landing on the wrong heading is worse than not landing at
// all. So the resolution is written once, here, and every verb gets the same
// four ways in:
//
//	orgs todo DONE                       a picker, when there is somebody to pick
//	orgs todo DONE 'IsStatus("NEXT")'    a query, the same language /search takes
//	orgs todo DONE hOpOB7vIg6oiYz5sMVS=  one heading, by hash
//	… | orgs todo DONE -                 hashes on stdin, one per line
//
// The last is the one that makes the rest of the tool worth more than the sum
// of it: the read half already prints hashes, so
//
//	orgs search 'IsStatus("NEXT") && OlderThan("30d")' -json \
//	  | jq -r .Hash | orgs tag +stale -
//
// is a sentence rather than a program. Nothing had to be added to `search` for
// that to work.
//
// Two rules hold the whole thing up, and both are about not writing to the
// wrong place:
//
//  1. **More than one match is a question, not an assumption.** With a person
//     there, the matches go to a picker (multi-select, so "these four" is one
//     press). With nothing but a pipe, it refuses and says how many it found -
//     because a query somebody expected to match one heading and which matched
//     nine is a mistake being made at speed.
//  2. **-all is how you say you meant all of them**, and it is the caller's
//     word rather than something inferred from the shape of the answer.

import (
	"bufio"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// TargetOpts is what a verb tells the resolver about itself. Everything has a
// default that is right for most of them.
type TargetOpts struct {
	// What the picker says above the list. "Mark done> ", "Delete> ".
	Prompt string
	// The query the picker offers when no words were given at all. A verb
	// about tasks wants `IsTask()`; one that could be about any heading leaves
	// this empty and gets everything.
	Query string
	// Whether this verb can sensibly act on more than one heading. A rename
	// cannot - the new text would be the same for all of them - so it says so
	// and the resolver refuses two rather than writing the same headline over
	// nine of them.
	Multi bool
	// Say what was resolved, one line per heading, before writing. On by
	// default for a verb; off for anything that prints its own listing.
	Quiet bool
}

// TargetFlags are the flags a verb grows for free by taking its targets
// through here. A verb registers them in SetupParameters and passes the struct
// back to Resolve.
type TargetFlags struct {
	Hash string
	All  bool
	At   int
	Yes  bool
}

// AddTargetFlags gives a verb the four flags every verb has. As with the
// global flags, a name the command has already taken is left alone.
func AddTargetFlags(fset *flag.FlagSet, t *TargetFlags) {
	if fset.Lookup("hash") == nil {
		fset.StringVar(&t.Hash, "hash", "", "the heading to act on, by hash")
	}
	if fset.Lookup("all") == nil {
		fset.BoolVar(&t.All, "all", false, "act on every heading that matched, without asking")
	}
	if fset.Lookup("at") == nil {
		fset.IntVar(&t.At, "at", 0, "act on the nth match (1 based)")
	}
	if fset.Lookup("yes") == nil {
		fset.BoolVar(&t.Yes, "yes", false, "do not ask before writing")
	}
}

// LooksLikeHash reports whether a word is a heading hash rather than a query.
//
// A hash is base64 of a sha1 (`orgdb.go` makes them), so it is 28 characters
// ending in `=`. That is specific enough to tell from a query expression -
// every one of those has a bracket or an operator in it - and the test is on
// the shape rather than on a flag, because having to say `-hash` for the thing
// the tool itself printed is a papercut on every pipeline.
func LooksLikeHash(s string) bool {
	if len(s) != 28 || !strings.HasSuffix(s, "=") {
		return false
	}
	if _, err := base64.StdEncoding.DecodeString(s); err != nil {
		return false
	}
	return true
}

// StdinHashes reads hashes from stdin, one per line.
//
// It takes the first whitespace-delimited field of each line rather than the
// whole line, so the output of `-format '{{.Hash}} {{.Headline}}'` works as
// well as a bare `jq -r .Hash`. A hash is base64 and can hold `+` and `/` but
// never a space, so the first field is always the whole of it. Blank lines and
// anything that is not a hash are skipped rather than refused - a header line
// or a trailing newline from some other tool should not stop the write.
func StdinHashes() []string {
	out := []string{}
	sc := bufio.NewScanner(os.Stdin)
	// A line of json from `-json` is long; the default 64k is not enough for a
	// pasted list either. Same reason filesearch.go raises its scanner.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		word := strings.Fields(line)[0]
		// `-json` output is an array of objects; pick the hashes out of it
		// rather than making the caller run jq for the common case.
		if strings.HasPrefix(word, "{") || strings.HasPrefix(word, "[") ||
			strings.HasPrefix(word, `"Hash"`) {
			if h := hashInJsonLine(line); h != "" {
				out = append(out, h)
			}
			continue
		}
		if LooksLikeHash(word) {
			out = append(out, word)
		}
	}
	return out
}

// hashInJsonLine picks a hash out of a line of indented json, so piping
// `-json` straight in works without jq. It is deliberately a scan for the
// field rather than a parse: the input is a stream of lines and may be an
// array split over hundreds of them.
func hashInJsonLine(line string) string {
	const key = `"Hash":`
	i := strings.Index(line, key)
	if i < 0 {
		return ""
	}
	rest := strings.TrimSpace(line[i+len(key):])
	rest = strings.TrimPrefix(rest, `"`)
	if j := strings.IndexAny(rest, `"`); j >= 0 {
		rest = rest[:j]
	}
	if LooksLikeHash(rest) {
		return rest
	}
	return ""
}

// Resolve works out which headings a verb is about, or does not return.
//
// words is what FreeArgs gave the command *after* it has taken off whatever it
// needs itself - the keyword for `orgs todo`, the date for `orgs sched` - so
// what arrives here is only ever the target.
func Resolve(core *Core, tf *TargetFlags, words []string, opts TargetOpts) []common.Todo {
	// -hash is the explicit form and beats everything, including a query
	// somebody left on the line.
	if tf.Hash != "" {
		return []common.Todo{fetchOne(core, tf.Hash)}
	}

	// A bare `-` means stdin, and so does stdin already being a pipe with no
	// words to go on. The second is what makes `… | orgs archive` work without
	// the dash, and it is only ever read when there is nothing else to use:
	// stdin being a pipe is not permission to ignore a query somebody typed.
	fromStdin := false
	rest := []string{}
	for _, w := range words {
		if w == "-" {
			fromStdin = true
			continue
		}
		rest = append(rest, w)
	}
	words = rest
	if len(words) == 0 && !fromStdin && StdinIsPipe() {
		fromStdin = true
	}

	var found []common.Todo
	switch {
	case fromStdin:
		hashes := StdinHashes()
		if len(hashes) == 0 {
			Fail("nothing on stdin to act on")
		}
		for _, h := range hashes {
			found = append(found, fetchOne(core, h))
		}
		// Hashes were named one at a time, so each one is meant. Asking "which
		// of these nine" about a list somebody built on purpose is asking them
		// to do the filtering twice.
		tf.All = true

	case len(words) == 1 && LooksLikeHash(words[0]):
		return []common.Todo{fetchOne(core, words[0])}

	case len(words) > 0:
		found = query(core, strings.Join(words, " "))
		if len(found) == 0 {
			Fail("no heading matched %s", strings.Join(words, " "))
		}

	default:
		if !Interactive() {
			Fail("say which heading: a query, a hash, or - for hashes on stdin")
		}
		found = query(core, opts.Query)
		if len(found) == 0 {
			Fail("no headings to choose from")
		}
	}

	return narrow(core, tf, found, opts)
}

// narrow takes the matches down to the ones to write to, asking when it has to
// and refusing when it cannot ask.
func narrow(core *Core, tf *TargetFlags, found []common.Todo, opts TargetOpts) []common.Todo {
	if tf.At > 0 {
		if tf.At > len(found) {
			Fail("-at %d, but only %d matched", tf.At, len(found))
		}
		return []common.Todo{found[tf.At-1]}
	}
	if len(found) == 1 {
		return found
	}
	if tf.All {
		if !opts.Multi {
			Fail("this changes one heading at a time, and %d matched", len(found))
		}
		return found
	}
	if !Interactive() {
		// The count is the useful half of this: a query written for one
		// heading that found nine is a mistake, and saying so beats writing to
		// nine of them.
		Fail("%d headings matched. -at N for one of them, -all for every one, or narrow the query",
			len(found))
	}
	return pickHeadings(core, found, opts)
}

// pickHeadings is the fzf narrowing, with the heading's own body in the pane -
// which is the question somebody opening it is asking: is this the one I meant.
func pickHeadings(core *Core, found []common.Todo, opts TargetOpts) []common.Todo {
	lines := make([]string, len(found))
	for i, t := range found {
		// Two address fields: the row, which is how the choice is read back
		// (a hash is sha1 of the headline text, so two headings with the same
		// text share one and the row is the only unambiguous handle), and the
		// hash, which is what the pane needs.
		lines[i] = PickLine([]string{fmt.Sprintf("%d", i), t.Hash}, headingLine(t))
	}
	prompt := opts.Prompt
	if prompt == "" {
		prompt = "Heading> "
	}
	// The pane is the heading's own text, drawn by a second run of this binary
	// - the same arrangement every other picker here uses. `orgs show` is that
	// pane, and it is handed the hash rather than re-running the query, so
	// filtering the list under it cannot move what the pane is about.
	preview := ""
	if self, err := SelfCommand(core); err == nil {
		preview = self + " show -hash {2} -pane"
	}
	sel := Pick(PickOpts{
		Lines:         lines,
		AddressFields: 2,
		Prompt:        prompt,
		Header:        "tab to mark several · enter to confirm · ctrl-/ hides the pane",
		Preview:       preview,
		Extra:         []string{"--multi"},
	})
	if len(sel) == 0 {
		Fail("nothing chosen")
	}
	out := []common.Todo{}
	for _, line := range sel {
		addr, ok := Address(line, 2)
		if !ok {
			continue
		}
		var i int
		if _, err := fmt.Sscanf(addr[0], "%d", &i); err != nil || i < 0 || i >= len(found) {
			continue
		}
		out = append(out, found[i])
	}
	if len(out) == 0 {
		Fail("nothing chosen")
	}
	if len(out) > 1 && !opts.Multi {
		Fail("this changes one heading at a time, and %d were chosen", len(out))
	}
	return out
}

// headingLine is how a heading reads in a list: the keyword, the text, and
// where it lives. The file is in the display as well as being an address,
// because what is shown is what fzf searches.
func headingLine(t common.Todo) string {
	kw := ""
	if t.Status != "" {
		kw = C(AnsiGold) + t.Status + C(AnsiReset) + " "
	}
	tags := ""
	if len(t.Tags) > 0 {
		tags = " " + C(AnsiCyan) + ":" + strings.Join(t.Tags, ":") + ":" + C(AnsiReset)
	}
	return fmt.Sprintf("%s%s%s  %s%s:%d%s", kw, t.Headline, tags,
		C(AnsiDim), BaseName(t.Filename), t.LineNum, C(AnsiReset))
}

// Describe is the line a verb prints to say what it wrote to. Every verb says
// this, because a write with no feedback is indistinguishable from one that
// silently missed.
func Describe(t common.Todo) string {
	return fmt.Sprintf("%s%s:%d%s %s", C(AnsiDim), BaseName(t.Filename), t.LineNum,
		C(AnsiReset), t.Headline)
}

// TodoByHash is the one heading a hash names, or a refusal. Exported because a
// preview pane is handed a hash and has nothing to resolve.
func TodoByHash(core *Core, hash string) common.Todo { return fetchOne(core, hash) }

func fetchOne(core *Core, hash string) common.Todo {
	t, err := SendReceiveGetErr[common.Todo](core, "hash/"+HashPath(hash), nil)
	if err != nil {
		Fail("no heading with hash %s: %v", hash, err)
	}
	if t.Hash == "" {
		// The handler answers with an empty object rather than a status for a
		// hash it cannot find, so an empty hash on the way back is the "not
		// found" and has to be read as one.
		Fail("no heading with hash %s", hash)
	}
	return t
}

func query(core *Core, expr string) []common.Todo {
	if strings.TrimSpace(expr) == "" {
		// Everything, which is what a picker with no query offered wants.
		expr = "true"
	}
	todos, err := SendReceiveGetErr[common.Todos](core, "search", map[string]string{"query": expr})
	if err != nil {
		Fail("%v", err)
	}
	return todos
}

// Confirm asks a yes or no question, and answers no when there is nobody to
// ask. -yes is how a script says it does not need asking.
func Confirm(tf *TargetFlags, format string, args ...interface{}) bool {
	if tf != nil && tf.Yes {
		return true
	}
	if !Interactive() {
		// A question put to something that cannot type is a hang. Refusing is
		// the safe reading for a destructive verb, and -yes is right there.
		Fail("%s - pass -yes to go ahead", fmt.Sprintf(format, args...))
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", fmt.Sprintf(format, args...))
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return false
	}
	a := strings.ToLower(strings.TrimSpace(sc.Text()))
	return a == "y" || a == "yes"
}

// Ok is what a verb prints when a write landed, and how it reports one to a
// program. A verb calls this instead of printing, so -json gets an object
// rather than a line of prose it has to parse.
func Ok(count int, what string, lines []string) {
	if JsonOut {
		RenderOne(map[string]interface{}{"Ok": true, "Changed": count, "What": what}, nil)
		return
	}
	if FormatOut != "" {
		return
	}
	for _, l := range lines {
		fmt.Printf("%s%s%s %s\n", C(AnsiGreen), "✓", C(AnsiReset), l)
	}
	if count > 1 {
		fmt.Printf("%s%d headings %s%s\n", C(AnsiDim), count, what, C(AnsiReset))
	}
}

// RestGetStates is the server's own keyword list, which is what a file with no
// #+TODO line of its own uses. Cached for the run: several verbs ask, and the
// answer cannot change under one command.
func RestGetStates(core *Core) common.TodoStatesResult {
	if statesCache != nil {
		return *statesCache
	}
	s, err := SendReceiveGetErr[common.TodoStatesResult](core, "status", nil)
	if err != nil {
		s = common.TodoStatesResult{}
	}
	statesCache = &s
	return s
}

var statesCache *common.TodoStatesResult
