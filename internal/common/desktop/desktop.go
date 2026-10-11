// Package desktop pops a native notification on whatever machine it runs on:
// macOS Notification Center, the freedesktop notification service on Linux,
// and toasts on Windows. The server uses it for reminders, saved-query
// matches and focus phases; `orgs notify listen` uses it on the machine a
// person is sitting at when the server is somewhere else.
//
// Each platform has a best way and a way that is always there, and Detect
// picks the best one present:
//
//	macOS    terminal-notifier (click opens the heading, notifications are
//	         grouped and replaced, attributed to the notifier rather than to
//	         "Script Editor"), else osascript
//	Linux    the org.freedesktop.Notifications service over D-Bus (urgency,
//	         replace in place, a click opens the heading), else notify-send
//	Windows  a toast through PowerShell, with an Open button and click to open
//
// The argument builders are plain functions in this file, so what each
// platform is asked to do is tested everywhere, not only on that platform.
package desktop

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// How much a notification wants attention. Critical stays on screen on the
// desktops that honour it (Linux), and is never sent silently.
type Urgency int

const (
	Normal Urgency = iota
	Low
	Critical
)

// One notification.
type Note struct {
	Title    string
	Subtitle string // macOS shows it under the title; elsewhere it leads the body
	Body     string
	// Opened when the notification is clicked, where the platform allows it:
	// a worg link to the heading, usually.
	URL     string
	Sound   bool
	Urgency Urgency
	// Notifications in one group replace each other rather than piling up:
	// the third "10 minutes till standup" is not three notifications.
	Group string
	// An image file shown with it, where the platform allows one.
	Icon string
}

// A way of showing notifications on this machine.
type Notifier interface {
	// What it is, for `orgs notify status`: "macOS (terminal-notifier)".
	Name() string
	Send(n Note) error
}

var (
	detected     Notifier
	detectedOnce sync.Once
)

// The best way this machine has, found once.
func Default() Notifier {
	detectedOnce.Do(func() { detected = Detect() })
	return detected
}

// Send through the best way this machine has.
func Send(n Note) error {
	if strings.TrimSpace(n.Title) == "" && strings.TrimSpace(n.Body) == "" {
		return fmt.Errorf("desktop: nothing to say")
	}
	return Default().Send(n)
}

// Every notifier keeps to these, whatever the platform: a long body is a
// paragraph nobody reads in a popup, and some servers drop it entirely.
const (
	maxTitle = 120
	maxBody  = 400
)

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// The body as one text, with the subtitle leading it, for the platforms that
// have no subtitle of their own.
func bodyWithSubtitle(n Note) string {
	if n.Subtitle == "" {
		return n.Body
	}
	if n.Body == "" {
		return n.Subtitle
	}
	return n.Subtitle + "\n" + n.Body
}

// --- running a program --------------------------------------------------------

// A notifier program that hangs (a stuck D-Bus, a PowerShell waiting on a
// prompt) must not hang the reminder loop behind it.
const runTimeout = 10 * time.Second

// What runs a program; tests replace it.
var run = func(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s: %v: %s", name, err, msg)
		}
		return fmt.Errorf("%s: %v", name, err)
	}
	return nil
}

// Whether a program is on the PATH; tests replace it.
var have = func(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// --- macOS --------------------------------------------------------------------

// An AppleScript string literal. Only the backslash and the double quote need
// escaping; a headline with either used to end the string early and the
// notification silently never appeared.
func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// The osascript program for a note: `display notification` with a real system
// sound rather than the beep it used to play before it.
func osascriptScript(n Note) string {
	var b strings.Builder
	b.WriteString("display notification ")
	b.WriteString(appleScriptString(clip(n.Body, maxBody)))
	b.WriteString(" with title ")
	b.WriteString(appleScriptString(clip(n.Title, maxTitle)))
	if n.Subtitle != "" {
		b.WriteString(" subtitle ")
		b.WriteString(appleScriptString(clip(n.Subtitle, maxTitle)))
	}
	if n.Sound || n.Urgency == Critical {
		b.WriteString(` sound name "Glass"`)
	}
	return b.String()
}

// terminal-notifier's arguments for a note.
func terminalNotifierArgs(n Note) []string {
	args := []string{"-title", clip(n.Title, maxTitle), "-message", clip(n.Body, maxBody)}
	if n.Body == "" {
		// It refuses an empty message.
		args[3] = " "
	}
	if n.Subtitle != "" {
		args = append(args, "-subtitle", clip(n.Subtitle, maxTitle))
	}
	if n.Group != "" {
		args = append(args, "-group", "orgs."+n.Group)
	}
	if n.URL != "" {
		args = append(args, "-open", n.URL)
	}
	if n.Sound || n.Urgency == Critical {
		args = append(args, "-sound", "default")
	}
	if n.Icon != "" {
		args = append(args, "-appIcon", n.Icon)
	}
	return args
}

type osascriptNotifier struct{}

func (osascriptNotifier) Name() string { return "macOS (osascript)" }
func (osascriptNotifier) Send(n Note) error {
	return run("osascript", "-e", osascriptScript(n))
}

type terminalNotifier struct{}

func (terminalNotifier) Name() string { return "macOS (terminal-notifier)" }
func (terminalNotifier) Send(n Note) error {
	return run("terminal-notifier", terminalNotifierArgs(n)...)
}

// --- Linux --------------------------------------------------------------------

// Notification servers read a little HTML in the body (the spec's "body
// markup"), so a headline with `<` or `&` came out mangled or not at all.
func markupEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func linuxUrgency(u Urgency) string {
	switch u {
	case Low:
		return "low"
	case Critical:
		return "critical"
	}
	return "normal"
}

// notify-send's arguments for a note. It has no click action that does not
// block, so a URL is left to the D-Bus path, which listens for it.
func notifySendArgs(n Note) []string {
	args := []string{"--app-name=orgs", "--urgency=" + linuxUrgency(n.Urgency)}
	if n.Icon != "" {
		args = append(args, "--icon="+n.Icon)
	}
	if n.Group != "" {
		// Replace rather than stack, on the servers that know this hint.
		args = append(args, "--hint=string:x-canonical-private-synchronous:orgs."+n.Group)
	}
	if n.Sound {
		args = append(args, "--hint=string:sound-name:message-new-instant")
	}
	args = append(args, clip(n.Title, maxTitle), markupEscape(clip(bodyWithSubtitle(n), maxBody)))
	return args
}

type notifySendNotifier struct{}

func (notifySendNotifier) Name() string { return "Linux (notify-send)" }
func (notifySendNotifier) Send(n Note) error {
	return run("notify-send", notifySendArgs(n)...)
}

// --- nowhere ------------------------------------------------------------------

type noNotifier struct{ why string }

func (n noNotifier) Name() string    { return "none (" + n.why + ")" }
func (n noNotifier) Send(Note) error { return fmt.Errorf("desktop notifications: %s", n.why) }
