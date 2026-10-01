package commands

// Asking for a password.
//
// Said once here rather than in each command that needs one. `orgs user` and
// `orgs adduser` write the same credential to the same keystore by two
// different routes, and a difference between them - one asking twice and the
// other once, one accepting an empty password - would be a difference in what
// a password is.

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/term"
)

// ReadNewPassword asks for a password that is about to be set, which means
// asking twice: a password nobody can read back is one typo away from an
// account nobody can log into.
//
// `given` is what a -password flag supplied, and short-circuits the asking. A
// command with nobody to ask and no flag has nothing to do but say so.
func ReadNewPassword(prompt, given string) string {
	if given != "" {
		return given
	}
	if !Interactive() {
		Fail("no password given and nobody to ask. Pass -password, " +
			"which does mean it lands in your shell history.")
	}
	for {
		first := ReadPasswordOnce(prompt + ": ")
		if first == "" {
			fmt.Fprintln(os.Stderr, "An empty password is not one. Try again.")
			continue
		}
		if second := ReadPasswordOnce("Again: "); first != second {
			fmt.Fprintln(os.Stderr, "Those do not match. Try again.")
			continue
		}
		return first
	}
}

// ReadPasswordOnce reads one password without echoing it. The prompt goes to
// stderr, like every other thing a command says that is not its answer.
func ReadPasswordOnce(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	pw, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		Fail("could not read the password: %v", err)
	}
	return string(pw)
}
