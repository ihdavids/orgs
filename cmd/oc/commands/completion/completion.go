package completion

// orgs completion - tab completion for zsh, bash and fish.
//
//	orgs completion zsh  > ~/.zsh/completions/_orgs
//	orgs completion bash > /etc/bash_completion.d/orgs
//	orgs completion fish > ~/.config/fish/completions/orgs.fish
//
// Forty-odd commands, and every interesting argument is something the server
// already knows: which files there are, which tags, which keywords *this file*
// allows, which queries you saved, which exporters this server was built with.
// A completion that only knew the command names would be the least valuable
// nine tenths of this.
//
// So the shell scripts are tiny and dumb, and all three do the same thing: hand
// the words typed so far to `orgs __complete` and print what comes back. That
// is one implementation of the interesting part rather than three, in a language
// that can make an http request, and a command added tomorrow completes without
// anybody editing a shell script.
//
// Two rules keep it from being annoying, which is the failure mode of dynamic
// completion:
//
//  1. **It never blocks for long.** The request has a short timeout and an
//     empty answer on failure - a server that is down or slow must degrade to
//     "no suggestions", never to a shell that has stopped responding.
//  2. **It never guesses at a value that would be destructive to get wrong.**
//     Completing a query is a convenience; completing the target of `orgs rm`
//     would be a way to delete the wrong heading quickly.

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Completion struct{}

func (self *Completion) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Completion) StartPlugin(m *common.PluginManager)       {}
func (self *Completion) SetupParameters(fset *flag.FlagSet)        {}

// NeedsNoServer: this prints a shell script out of this binary.
func (self *Completion) NeedsNoServer() bool { return true }

func (self *Completion) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("completion").Flags)
	shell := ""
	if len(words) > 0 {
		shell = strings.ToLower(words[0])
	}
	switch shell {
	case "zsh":
		fmt.Print(zshScript)
	case "bash":
		fmt.Print(bashScript)
	case "fish":
		fmt.Print(fishScript)
	default:
		commands.Fail("orgs completion <zsh|bash|fish>\n\n%s", install)
	}
}

const install = `  zsh   orgs completion zsh  > "${fpath[1]}/_orgs"
  bash  orgs completion bash > /etc/bash_completion.d/orgs
  fish  orgs completion fish > ~/.config/fish/completions/orgs.fish

  Or, to try it in this shell without installing anything:
    zsh   source <(orgs completion zsh)
    bash  source <(orgs completion bash)`

// ---------------------------------------------------------------------------
// The hidden half
// ---------------------------------------------------------------------------

// Complete is `orgs __complete <words...>`: given what has been typed, print
// one candidate per line. Everything about what completes where lives here.
type Complete struct{}

func (self *Complete) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Complete) StartPlugin(m *common.PluginManager)       {}
func (self *Complete) SetupParameters(fset *flag.FlagSet)        {}

// NeedsNoServer: it asks one when it can, and answers with nothing when it
// cannot. A shell completion must never be the thing that stops a prompt.
func (self *Complete) NeedsNoServer() bool { return true }

func (self *Complete) Exec(core *commands.Core) {
	// A completion is on the critical path of somebody pressing tab, so it is
	// given a short leash: a server that is down, asleep or on the other end of
	// a vpn must turn into "no suggestions" rather than into a prompt that has
	// stopped responding.
	core.Rest.Timeout = 900 * time.Millisecond

	// Read from os.Args rather than through FreeArgs: the words being completed
	// are a half-typed command line and may hold anything, including things
	// that look like this command's own flags.
	args := os.Args
	i := 0
	for ; i < len(args); i++ {
		if args[i] == "__complete" {
			break
		}
	}
	words := []string{}
	if i+1 < len(args) {
		words = args[i+1:]
	}

	// The last word is the one being completed, and may be empty when the
	// cursor is after a space.
	prefix := ""
	if len(words) > 0 {
		prefix = words[len(words)-1]
		words = words[:len(words)-1]
	}

	var out []string
	switch {
	case len(words) == 0:
		out = commandNames()
	case strings.HasPrefix(prefix, "-"):
		out = flagNames(words[0])
	default:
		out = self.values(core, words, prefix)
	}

	for _, c := range out {
		if prefix == "" || strings.HasPrefix(c, prefix) {
			fmt.Println(c)
		}
	}
}

func commandNames() []string {
	out := []string{}
	for name := range commands.CmdRegistry {
		// Not itself: nobody types __complete.
		if strings.HasPrefix(name, "__") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func flagNames(cmd string) []string {
	thunk := commands.Find(cmd)
	if thunk == nil || thunk.Flags == nil {
		return nil
	}
	out := []string{}
	thunk.Flags.VisitAll(func(f *flag.Flag) { out = append(out, "-"+f.Name) })
	sort.Strings(out)
	return out
}

// values is the interesting half: what this command's positional arguments
// could be, asked of the server.
func (self *Complete) values(core *commands.Core, words []string, prefix string) []string {
	cmd := words[0]
	// Which positional argument is being typed, ignoring flags and their values
	// - which cannot be told apart here without knowing every flag's type, so a
	// word beginning with a dash is skipped and nothing else is.
	pos := 0
	for _, w := range words[1:] {
		if !strings.HasPrefix(w, "-") {
			pos++
		}
	}

	// A flag immediately before the cursor is being given its value.
	if len(words) > 1 {
		if last := words[len(words)-1]; strings.HasPrefix(last, "-") {
			if v := self.flagValues(core, cmd, strings.TrimLeft(last, "-")); v != nil {
				return v
			}
		}
	}

	switch cmd {
	case "todo":
		if pos == 0 {
			return keywords(core)
		}
		return savedQueries(core)
	case "tag":
		return tags(core)
	case "q":
		if pos == 0 {
			return append([]string{"ls", "save", "rm", "edit"}, savedQueryNames(core)...)
		}
		return nil
	case "fmt", "tangle", "outline", "export", "new":
		return files(core)
	case "sched", "deadline":
		if pos == 0 {
			// The words a date can start with. Not a list of dates: nobody
			// wants their shell offering three hundred of them.
			return []string{"today", "tomorrow", "yesterday", "mon", "tue", "wed", "thu",
				"fri", "sat", "sun", "eow", "eom", "eoy", "clear"}
		}
		return savedQueries(core)
	case "rec", "record", "contact":
		return collections(core)
	case "filter":
		return filters(core)
	case "completion":
		return []string{"zsh", "bash", "fish"}
	case "search", "show", "note", "prop", "rename", "archive", "rm", "check", "log":
		// A query, and the saved ones are the queries worth offering. The
		// heading itself is deliberately not completed for the destructive
		// ones: a tab that picks the wrong heading for `orgs rm` is a way to
		// delete something quickly.
		return savedQueries(core)
	case "watch":
		return files(core)
	}
	return nil
}

// flagValues is what a particular flag takes, where the server knows.
func (self *Complete) flagValues(core *commands.Core, cmd, flagName string) []string {
	switch flagName {
	case "f", "format":
		if cmd == "export" {
			return exporters(core)
		}
	case "theme":
		return themes(core)
	case "sort":
		return []string{"date", "deadline", "priority", "file", "status"}
	case "kinds":
		return []string{"reload", "clockin", "clockout"}
	case "file", "filename", "out", "o":
		return files(core)
	case "hash":
		// A hash is 28 characters of base64 and completing it is not a service
		// to anybody typing one.
		return nil
	}
	return nil
}

// ---------------------------------------------------------------------------
// What the server knows
// ---------------------------------------------------------------------------

func files(core *commands.Core) []string {
	// The base names as well as the full paths, because that is what people
	// type and every command here resolves one.
	out := []string{}
	seen := map[string]bool{}
	for _, f := range commands.SendReceiveGetOr[common.FileList](core, "files", nil) {
		base := commands.BaseName(f)
		if !seen[base] {
			seen[base] = true
			out = append(out, base)
		}
	}
	sort.Strings(out)
	return out
}

// Each of these matches the shape its endpoint actually answers with, which is
// different for nearly every one of them - a bare array here, an {Ok, Names}
// object there, a map for the filters. Getting one wrong is silent: the
// unmarshal fails and the completion is simply empty, which looks exactly like
// a database with no tags in it.
func tags(core *commands.Core) []string {
	out := []string{}
	for _, t := range commands.SendReceiveGetOr[[]string](core, "alltags", nil) {
		// Offered as `+tag`, which is the form that adds one - and the form
		// that does not collide with the flag parser.
		out = append(out, "+"+t)
	}
	sort.Strings(out)
	return out
}

func keywords(core *commands.Core) []string {
	s := commands.SendReceiveGetOr[common.TodoStatesResult](core, "status", nil)
	return append(append([]string{}, s.Active...), s.Done...)
}

type storedQuery struct {
	Name  string `json:"name"`
	Query string `json:"query"`
}

func savedQueryNames(core *commands.Core) []string {
	out := []string{}
	for _, q := range commands.SendReceiveGetOr[[]storedQuery](core, "ext/queries", nil) {
		out = append(out, q.Name)
	}
	sort.Strings(out)
	return out
}

// savedQueries offers the *text* of each saved query, because that is what goes
// where a query goes - a name there would be a query for headings called that.
func savedQueries(core *commands.Core) []string {
	out := []string{}
	for _, q := range commands.SendReceiveGetOr[[]storedQuery](core, "ext/queries", nil) {
		out = append(out, q.Query)
	}
	// The filters too, in the handlebars form a query takes them in.
	out = append(out, filters(core)...)
	sort.Strings(out)
	return out
}

// /filters answers with a map of name to expression.
func filters(core *commands.Core) []string {
	out := []string{}
	for f := range commands.SendReceiveGetOr[map[string]string](core, "filters", nil) {
		out = append(out, "{{ "+f+" }}")
	}
	sort.Strings(out)
	return out
}

func exporters(core *commands.Core) []string {
	out := commands.SendReceiveGetOr[struct {
		Ok    bool
		Names []string
	}](core, "exporters", nil).Names
	sort.Strings(out)
	return out
}

func themes(core *commands.Core) []string {
	out := commands.SendReceiveGetOr[struct {
		Ok     bool
		Themes []string
	}](core, "html/themes", nil).Themes
	sort.Strings(out)
	return out
}

func collections(core *commands.Core) []string {
	out := commands.SendReceiveGetOr[[]string](core, "records/collections", nil)
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// The scripts
// ---------------------------------------------------------------------------

// All three do the same thing and are as small as they can be: the decisions
// are in `orgs __complete`, so none of them has a list of commands or flags in
// it to go stale.

const zshScript = `#compdef orgs
# zsh completion for orgs. Generated by ` + "`orgs completion zsh`" + `.
#
# Everything interesting is asked of the running server through
# ` + "`orgs __complete`" + `, so this file does not go stale when a command is
# added - and answers with nothing when no server is reachable.

_orgs() {
  local -a candidates
  local IFS=$'\n'
  # ${words[1,CURRENT]} is everything up to and including the word being
  # completed; the last of those may be empty, which is what tells __complete
  # that the cursor is after a space.
  candidates=($(orgs __complete "${(@)words[2,CURRENT]}" 2>/dev/null))
  if (( ${#candidates} )); then
    compadd -- ${candidates}
  else
    _files
  fi
}

compdef _orgs orgs
`

const bashScript = `# bash completion for orgs. Generated by ` + "`orgs completion bash`" + `.
#
# Everything interesting is asked of the running server through
# ` + "`orgs __complete`" + `, so this file does not go stale when a command is
# added - and answers with nothing when no server is reachable.

_orgs() {
  local IFS=$'\n'
  local candidates
  # COMP_WORDS[1..COMP_CWORD] is everything after the program name up to and
  # including the word being completed.
  candidates=$(orgs __complete "${COMP_WORDS[@]:1:COMP_CWORD}" 2>/dev/null)
  if [[ -n "$candidates" ]]; then
    COMPREPLY=($(compgen -W "$candidates" -- "${COMP_WORDS[COMP_CWORD]}"))
  else
    COMPREPLY=($(compgen -f -- "${COMP_WORDS[COMP_CWORD]}"))
  fi
}

complete -o default -F _orgs orgs
`

const fishScript = `# fish completion for orgs. Generated by ` + "`orgs completion fish`" + `.
#
# Everything interesting is asked of the running server through
# ` + "`orgs __complete`" + `, so this file does not go stale when a command is
# added - and answers with nothing when no server is reachable.

function __orgs_complete
    set -l tokens (commandline -opc)
    # Drop the program name, and add the partial word so __complete knows
    # whether the cursor is on a word or after a space.
    set -e tokens[1]
    orgs __complete $tokens (commandline -ct) 2>/dev/null
end

complete -c orgs -f -a '(__orgs_complete)'
`

func init() {
	commands.AddCmd("completion", "print a tab completion script for zsh, bash or fish",
		func() commands.Cmd { return &Completion{} })
	commands.AddCmd("__complete", "what could come next on the command line (used by the shell)",
		func() commands.Cmd { return &Complete{} })
}
