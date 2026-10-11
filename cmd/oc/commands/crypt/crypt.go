package crypt

// orgs crypt: encrypted headings (org-crypt) from the terminal.
//
//	orgs crypt                      the headings tagged :crypt:, locked or not
//	orgs crypt ls -plain            only the ones sitting in the clear
//	orgs crypt status               is the server holding a passphrase
//	orgs crypt show [heading]       print one heading's plaintext
//	orgs crypt encrypt [heading]    encrypt one heading (tagging it if need be)
//	orgs crypt sweep                encrypt every tagged heading in the clear
//	orgs crypt lock                 make the server forget a held passphrase
//
// A heading is named the way every write verb names one (commands.Resolve): a
// query, a hash, hashes on stdin, or a picker.
//
// Three rules, all about where a secret can end up:
//
//  1. **The passphrase is never a flag.** A flag is in the shell history and in
//     `ps`. It is asked for without echo, or read from stdin with
//     -passphrase-stdin for a script that has it in a pipe already.
//  2. **Plaintext goes only to a terminal**, unless -force or -json says
//     otherwise out loud. `orgs crypt show x > notes.txt` is how a secret
//     ends up in a file nobody remembers making, so it is refused before the
//     passphrase is even asked for.
//  3. **-dry-run never echoes a passphrase.** The usual dry run prints the
//     request body; here that body carries the passphrase, so these requests
//     say what they would do and stop before one is asked for.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type Crypt struct {
	fset   *flag.FlagSet
	target commands.TargetFlags

	Force     bool
	Keep      bool
	PassStdin bool
	Plain     bool
}

func (self *Crypt) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Crypt) StartPlugin(manager *common.PluginManager)         {}
func (self *Crypt) HelpGroup() string                                 { return "Changing a heading" }

func (self *Crypt) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Force, "force", false, "show: print plaintext even when stdout is not a terminal")
	fset.BoolVar(&self.Keep, "keep", false, "hold the passphrase on the server for crypt.unlockSeconds once it has worked")
	fset.BoolVar(&self.PassStdin, "passphrase-stdin", false, "read the passphrase from the first line of stdin rather than asking")
	fset.BoolVar(&self.Plain, "plain", false, "ls: only the headings tagged to be encrypted that are in the clear")
	commands.AddTargetFlags(fset, &self.target)
}

func (self *Crypt) Exec(core *commands.Core) {
	args := commands.FreeArgs(self.fset)
	verb := "ls"
	if len(args) > 0 {
		verb = args[0]
		args = args[1:]
	}
	switch verb {
	case "ls", "list":
		self.list(core)
	case "status", "state":
		self.status(core)
	case "show", "reveal", "cat":
		self.show(core, args)
	case "encrypt", "seal":
		self.encrypt(core, args)
	case "sweep":
		self.sweep(core)
	case "lock":
		self.lock(core)
	default:
		commands.Fail("orgs crypt: no %q; try ls, status, show, encrypt, sweep or lock", verb)
	}
}

func state(core *commands.Core) common.CryptState {
	st, err := commands.SendReceiveGetErr[common.CryptState](core, "crypt/state", nil)
	if err != nil {
		commands.Fail("could not ask the server: %v", err)
	}
	if !st.Ok && st.Msg != "" {
		commands.Fail("%s", st.Msg)
	}
	return st
}

// ---------------------------------------------------------------------------
// Asking for the passphrase
// ---------------------------------------------------------------------------

func (self *Crypt) readPass(prompt string, twice bool) string {
	if self.PassStdin {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			commands.Fail("-passphrase-stdin, but nothing on stdin")
		}
		pass := strings.TrimRight(line, "\r\n")
		if pass == "" {
			commands.Fail("an empty passphrase is not one")
		}
		return pass
	}
	// The question needs somebody at the keyboard; stdout may well be a pipe
	// (-json), which is why this is not commands.Interactive().
	if !term.IsTerminal(int(syscall.Stdin)) {
		commands.Fail("no passphrase and nobody to ask: pass -passphrase-stdin and pipe it in")
	}
	for {
		first := commands.ReadPasswordOnce(prompt + ": ")
		if first == "" {
			fmt.Fprintln(os.Stderr, "An empty passphrase is not one. Try again.")
			continue
		}
		if !twice {
			return first
		}
		if again := commands.ReadPasswordOnce("Again: "); again != first {
			fmt.Fprintln(os.Stderr, "Those do not match. Try again.")
			continue
		}
		return first
	}
}

// Straight to the server, not through SendReceivePost: that one prints the
// request body on -dry-run, and this body carries a passphrase. Callers check
// DryRun themselves before anything is asked for.
func post(core *commands.Core, path string, req common.CryptOp) common.CryptOpResult {
	res, err := common.RestPost[common.CryptOpResult](&core.Rest, path, &req)
	if err != nil {
		commands.Fail("%v", err)
	}
	return res
}

// One heading, by any of the ways a verb names one.
func (self *Crypt) one(core *commands.Core, words []string, prompt string, tag string) common.Todo {
	// Plain words are a headline, as `orgs mentions` reads them: "orgs crypt
	// show bank" means the heading called Bank, not a query called bank.
	if len(words) > 0 && !(len(words) == 1 && (words[0] == "-" || commands.LooksLikeHash(words[0]))) {
		joined := strings.Join(words, " ")
		if !strings.ContainsAny(joined, "()&|!=<>") {
			words = []string{fmt.Sprintf(`MatchHeadline("(?i)%s")`, regexpQuote(joined))}
		}
	}
	found := commands.Resolve(core, &self.target, words, commands.TargetOpts{
		Prompt: prompt,
		Query:  fmt.Sprintf(`HasTags("%s")`, tag),
		Quiet:  true,
	})
	if len(found) != 1 {
		commands.Fail("name one heading; %d matched", len(found))
	}
	return found[0]
}

// ---------------------------------------------------------------------------
// The verbs
// ---------------------------------------------------------------------------

func (self *Crypt) list(core *commands.Core) {
	ps := map[string]string{}
	if self.Plain {
		ps["plain"] = "t"
	}
	res, err := commands.SendReceiveGetErr[common.CryptHeadings](core, "crypt/headings", ps)
	if err != nil {
		commands.Fail("could not ask the server: %v", err)
	}
	commands.Render(res.Headings, func() {
		for _, h := range res.Headings {
			mark := commands.C(commands.AnsiGreen) + "locked " + commands.C(commands.AnsiReset)
			if !h.Encrypted {
				mark = commands.C(commands.AnsiGold) + "CLEAR  " + commands.C(commands.AnsiReset)
			}
			// The outline path ends with the heading itself.
			where := strings.Join(h.Olp, " › ")
			if where == "" {
				where = h.Headline
			}
			fmt.Printf("%s %s%s:%d%s  %s\n", mark, commands.C(commands.AnsiDim), base(h.Filename), h.Line+1,
				commands.C(commands.AnsiReset), where)
		}
		if res.Plain > 0 {
			fmt.Fprintf(os.Stderr, "%d tagged heading(s) in the clear; `orgs crypt sweep` encrypts them.\n", res.Plain)
		}
	})
}

func (self *Crypt) status(core *commands.Core) {
	st := state(core)
	commands.RenderOne(st, func() {
		fmt.Printf("mode      %s\n", st.Mode)
		fmt.Printf("tag       :%s:\n", st.Tag)
		switch {
		case st.Unlocked:
			fmt.Printf("unlocked  for %ds more (orgs crypt lock to forget it)\n", st.UnlockIn)
		case st.UnlockSeconds > 0:
			fmt.Printf("locked    (-keep holds a passphrase for %ds)\n", st.UnlockSeconds)
		default:
			fmt.Println("locked    (this server never holds a passphrase)")
		}
	})
}

func (self *Crypt) show(core *commands.Core, words []string) {
	// Refused before anything is asked: typing a passphrase only to be told
	// the answer cannot be printed is a passphrase typed for nothing.
	if !self.Force && !commands.JsonOut && !term.IsTerminal(int(os.Stdout.Fd())) {
		commands.Fail("stdout is not a terminal, so the plaintext is not printed. -force (or -json) if you mean it to go there.")
	}
	st := state(core)
	t := self.one(core, words, "Reveal> ", st.Tag)
	if commands.DryRun {
		fmt.Fprintf(os.Stderr, "dry run: would reveal %s (nothing is written either way)\n", t.Headline)
		return
	}
	req := common.CryptOp{Hash: t.Hash, Keep: self.Keep}
	res := common.CryptOpResult{NeedPassphrase: true}
	// A held passphrase (or a key) first, so an unlocked server does not ask.
	if !self.PassStdin && (st.Unlocked || st.Mode == "key") {
		res = post(core, "crypt/reveal", req)
	}
	if !res.Ok && (res.NeedPassphrase || res.WrongPassphrase) {
		req.Passphrase = self.readPass("Passphrase for "+t.Headline, false)
		res = post(core, "crypt/reveal", req)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	commands.RenderOne(res, func() {
		fmt.Print(res.Text)
		if !strings.HasSuffix(res.Text, "\n") {
			fmt.Println()
		}
	})
}

func (self *Crypt) encrypt(core *commands.Core, words []string) {
	st := state(core)
	if st.Mode == "off" {
		commands.Fail("this server has no gpg, so it cannot encrypt")
	}
	t := self.one(core, words, "Encrypt> ", st.Tag)
	if commands.DryRun {
		fmt.Fprintf(os.Stderr, "dry run: would encrypt %s and tag it :%s:\n", t.Headline, st.Tag)
		return
	}
	req := common.CryptOp{Hash: t.Hash, Keep: self.Keep}
	if st.Mode != "key" {
		req.Passphrase = self.readPass("New passphrase for "+t.Headline, true)
	}
	res := post(core, "crypt/seal", req)
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	commands.RenderOne(res, func() { fmt.Printf("%s: %s\n", t.Headline, res.Msg) })
}

func (self *Crypt) sweep(core *commands.Core) {
	st := state(core)
	if st.Mode == "off" {
		commands.Fail("this server has no gpg, so it cannot encrypt")
	}
	if commands.DryRun {
		fmt.Fprintln(os.Stderr, "dry run: would encrypt every heading tagged :"+st.Tag+": that is in the clear (orgs crypt ls -plain lists them)")
		return
	}
	pass := ""
	if st.Mode != "key" {
		pass = self.readPass("Passphrase for every tagged heading in the clear", true)
	}
	req := struct {
		Passphrase string `json:"passphrase"`
	}{pass}
	res, err := common.RestPost[common.CryptSweepResult](&core.Rest, "crypt/sweep", &req)
	if err != nil {
		commands.Fail("%v", err)
	}
	commands.RenderOne(res, func() {
		fmt.Println(res.Msg)
		for _, p := range res.Problems {
			fmt.Fprintln(os.Stderr, "  "+p)
		}
	})
	if !res.Ok {
		os.Exit(1)
	}
}

func (self *Crypt) lock(core *commands.Core) {
	res, err := commands.SendReceivePostErr[common.Empty, common.ResultMsg](core, "crypt/lock", &common.Empty{})
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	commands.RenderOne(res, func() { fmt.Println(res.Msg) })
}

// Typed words as a pattern. Only letters, digits and spaces go in as they
// are; the rest become "any character", which keeps the text out of both the
// pattern's and the query language's escaping rules (as `orgs mentions` does).
func regexpQuote(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return '.'
	}, s)
}

func base(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func init() {
	commands.AddCmd("crypt", "encrypted headings: ls, status, show, encrypt, sweep, lock",
		func() commands.Cmd { return &Crypt{} })
}
