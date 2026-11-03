package protocol

// Registering orgs as the program a system runs for org-protocol:// links.
// Each system wants something different, in install_<os>.go:
//
//   - macOS only lets an app bundle claim a url scheme, and hands the url over
//     as an Apple event rather than an argument. So install writes a small
//     AppleScript applet (osacompile, part of every macOS) whose
//     `on open location` runs orgs with the link, adds the scheme to its
//     Info.plist, registers it with Launch Services and makes it the default.
//   - Linux: a .desktop file with MimeType=x-scheme-handler/org-protocol,
//     made the default with xdg-mime.
//   - Windows: HKCU\Software\Classes\org-protocol with a shell\open\command.
//
// Every one of them runs this binary, with this config file, so the server
// and token are the ones this install was made with.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

type handlerSpec struct {
	Scheme string
	// The command the link is appended to: this binary, its config, and
	// `protocol` (with -scheme when it is not the usual one).
	Argv []string
	// What the handler is called where a person can see it.
	Name string
	// A stable identifier: the bundle id, the .desktop file's name.
	Id string
}

// named is the handler's name and id for a scheme; a second handler under
// another scheme gets its own, so the two can live side by side.
func named(scheme string) handlerSpec {
	s := handlerSpec{Scheme: scheme, Name: "Orgs Protocol", Id: "orgs-protocol"}
	if scheme != defaultScheme {
		s.Name += " (" + scheme + ")"
		s.Id += "-" + strings.NewReplacer("+", "-", ".", "-").Replace(scheme)
	}
	return s
}

var schemeRe = regexp.MustCompile(`^[a-z][a-z0-9+.-]{1,40}$`)

func (self *Protocol) spec(core *commands.Core) handlerSpec {
	scheme := strings.ToLower(self.Scheme)
	if !schemeRe.MatchString(scheme) {
		commands.Fail("-scheme %q is not a url scheme (letters, digits, + . -, starting with a letter)", self.Scheme)
	}
	s := named(scheme)
	bin, err := os.Executable()
	if err == nil {
		if real, err2 := filepath.EvalSymlinks(bin); err2 == nil {
			bin = real
		}
	}
	if err != nil {
		commands.Fail("cannot tell where this orgs binary is: %v", err)
	}
	s.Argv = []string{bin}
	if core.ConfigFile != "" {
		if abs, err := filepath.Abs(core.ConfigFile); err == nil {
			s.Argv = append(s.Argv, "-config", abs)
		}
	}
	s.Argv = append(s.Argv, "protocol")
	if scheme != defaultScheme {
		s.Argv = append(s.Argv, "-scheme", scheme)
	}
	return s
}

func (self *Protocol) install(core *commands.Core) {
	s := self.spec(core)
	if strings.Contains(s.Argv[0], string(filepath.Separator)+"go-build") {
		commands.Fail("this orgs is a `go run` build that is about to be deleted; install with a built binary")
	}
	if commands.DryRun {
		fmt.Fprintf(os.Stderr, "would register %s:// links to run: %s <link>\n", s.Scheme, strings.Join(s.Argv, " "))
		return
	}
	msg, err := installHandler(s)
	if err != nil {
		commands.Fail("install: %v", err)
	}
	fmt.Println(msg)
	fmt.Fprintf(os.Stderr, "Try it: open 'org-protocol://capture?url=https%%3A%%2F%%2Forgmode.org&title=Org' in a browser.\n")
	fmt.Fprintf(os.Stderr, "Bookmarklets for it: orgs protocol bookmarklets\n")
}

func (self *Protocol) uninstall() {
	s := named(strings.ToLower(self.Scheme))
	if commands.DryRun {
		fmt.Fprintf(os.Stderr, "would remove the %s:// handler %s\n", s.Scheme, s.Name)
		return
	}
	msg, err := uninstallHandler(s)
	if err != nil {
		commands.Fail("uninstall: %v", err)
	}
	fmt.Println(msg)
}

func (self *Protocol) status() {
	msg, err := handlerStatus(strings.ToLower(self.Scheme))
	if err != nil {
		commands.Fail("status: %v", err)
	}
	fmt.Println(msg)
}

// shellQuote is one word for /bin/sh, whatever is in it.
func shellQuote(s string) string {
	if s != "" && regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`).MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
