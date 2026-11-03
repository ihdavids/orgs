//go:build darwin

package protocol

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

func appPath(s handlerSpec) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Applications", s.Name+".app"), nil
}

func bundleId(s handlerSpec) string { return "com.github.ihdavids.orgs." + s.Id }

// An AppleScript string literal.
func asString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v %s", filepath.Base(name), strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func installHandler(s handlerSpec) (string, error) {
	app, err := appPath(s)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(app), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(app); err == nil {
		exec.Command(lsregister, "-u", app).Run()
		os.RemoveAll(app)
	}
	// Where a click's output goes: nobody sees a terminal for it, and a
	// failure is a desktop notification too, but the whole story is here.
	logFile := "/dev/null"
	if cache, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(cache, "orgs")
		if os.MkdirAll(dir, 0o755) == nil {
			logFile = filepath.Join(dir, "protocol.log")
		}
	}
	words := make([]string, len(s.Argv))
	for i, a := range s.Argv {
		words[i] = shellQuote(a)
	}
	// The PATH this was installed from, so the editor template's command is
	// found: an applet's shell gets only /usr/bin:/bin:/usr/sbin:/sbin.
	head := "export PATH=" + shellQuote(os.Getenv("PATH")) + "; " + strings.Join(words, " ") + " "
	tail := " >> " + shellQuote(logFile) + " 2>&1 &"
	script := "on open location theURL\n" +
		"\tdo shell script " + asString(head) + " & quoted form of theURL & " + asString(tail) + "\n" +
		"end open location\n"
	src, err := os.CreateTemp("", "orgs-protocol-*.applescript")
	if err != nil {
		return "", err
	}
	defer os.Remove(src.Name())
	src.WriteString(script)
	src.Close()
	if err := run("osacompile", "-o", app, src.Name()); err != nil {
		return "", err
	}

	plist := filepath.Join(app, "Contents", "Info.plist")
	buddy := func(cmd string) error { return run("/usr/libexec/PlistBuddy", "-c", cmd, plist) }
	if buddy("Set :CFBundleIdentifier "+bundleId(s)) != nil {
		if err := buddy("Add :CFBundleIdentifier string " + bundleId(s)); err != nil {
			return "", err
		}
	}
	buddy("Delete :CFBundleURLTypes")
	buddy("Delete :LSUIElement")
	for _, c := range []string{
		"Add :CFBundleURLTypes array",
		"Add :CFBundleURLTypes:0 dict",
		"Add :CFBundleURLTypes:0:CFBundleURLName string " + s.Id,
		"Add :CFBundleURLTypes:0:CFBundleURLSchemes array",
		"Add :CFBundleURLTypes:0:CFBundleURLSchemes:0 string " + s.Scheme,
		// No Dock icon for something that runs for a moment per click.
		"Add :LSUIElement bool true",
	} {
		if err := buddy(c); err != nil {
			return "", err
		}
	}
	// The plist edit breaks the applet's signature; an ad hoc one is what a
	// locally made app needs to keep opening.
	notes := []string{}
	if err := run("codesign", "--force", "--deep", "-s", "-", app); err != nil {
		notes = append(notes, "could not sign it again ("+err.Error()+"); if macOS refuses to open it, run: codesign --force --deep -s - '"+app+"'")
	}
	if err := run(lsregister, "-f", app); err != nil {
		return "", err
	}
	// Registering is not enough when something else (Emacs) claims the
	// scheme already: Launch Services keeps its own choice until told.
	js := "ObjC.import('CoreServices'); $.LSSetDefaultHandlerForURLScheme($('" + s.Scheme + "'), $('" + bundleId(s) + "'))"
	if out, err := exec.Command("osascript", "-l", "JavaScript", "-e", js).CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "0" {
		notes = append(notes, fmt.Sprintf("could not make it the default handler (%v %s)", err, strings.TrimSpace(string(out))))
	}
	msg := fmt.Sprintf("Installed %s, which hands %s:// links to: %s <link>\nIts output goes to %s", app, s.Scheme, strings.Join(s.Argv, " "), logFile)
	if now, err := handlerStatus(s.Scheme); err == nil {
		msg += "\n" + now
	}
	for _, n := range notes {
		msg += "\nNote: " + n
	}
	return msg, nil
}

func uninstallHandler(s handlerSpec) (string, error) {
	app, err := appPath(s)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(app); err != nil {
		return "Nothing to remove: there is no " + app, nil
	}
	exec.Command(lsregister, "-u", app).Run()
	if err := os.RemoveAll(app); err != nil {
		return "", err
	}
	msg := "Removed " + app
	if now, err := handlerStatus(s.Scheme); err == nil {
		msg += "\n" + now
	}
	return msg, nil
}

func handlerStatus(scheme string) (string, error) {
	js := "ObjC.import('AppKit'); var u = $.NSWorkspace.sharedWorkspace.URLForApplicationToOpenURL($.NSURL.URLWithString('" +
		scheme + "://x')); u.isNil() ? '' : u.path.js"
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", js).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("asking Launch Services: %v %s", err, strings.TrimSpace(string(out)))
	}
	if app := strings.TrimSpace(string(out)); app != "" {
		return scheme + ":// links open with " + app, nil
	}
	return "Nothing handles " + scheme + ":// links (orgs protocol install)", nil
}
