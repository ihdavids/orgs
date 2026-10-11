package desktop

import (
	"strings"
	"testing"
)

// A headline with quotes or backslashes used to end the AppleScript string
// early, and the notification silently never appeared.
func TestOsascriptEscapes(t *testing.T) {
	got := osascriptScript(Note{Title: `Say "hi"`, Subtitle: `C:\temp`, Body: "two\nlines", Sound: true})
	want := `display notification "two` + "\n" + `lines" with title "Say \"hi\"" subtitle "C:\\temp" sound name "Glass"`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if strings.Contains(osascriptScript(Note{Title: "quiet", Body: "b"}), "sound") {
		t.Fatal("no sound asked for, none given")
	}
	if !strings.Contains(osascriptScript(Note{Title: "x", Urgency: Critical}), "sound") {
		t.Fatal("a critical note is never silent")
	}
}

func TestTerminalNotifierArgs(t *testing.T) {
	got := strings.Join(terminalNotifierArgs(Note{Title: "Standup", Subtitle: "in 10 minutes", Body: "work.org", URL: "http://h/?open=a.org", Group: "remind", Sound: true}), "|")
	want := "-title|Standup|-message|work.org|-subtitle|in 10 minutes|-group|orgs.remind|-open|http://h/?open=a.org|-sound|default"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if args := terminalNotifierArgs(Note{Title: "only a title"}); args[3] != " " {
		t.Fatalf("an empty message must not be sent empty: %q", args)
	}
}

// Notification servers read body markup, so < and & must be escaped, and the
// subtitle leads the body where there is no subtitle of its own.
func TestNotifySendArgs(t *testing.T) {
	got := notifySendArgs(Note{Title: "A <b>", Subtitle: "in 2 min", Body: "R&D", Urgency: Critical, Group: "remind"})
	if got[0] != "--app-name=orgs" || got[1] != "--urgency=critical" {
		t.Fatalf("flags: %q", got)
	}
	if got[len(got)-2] != "A <b>" {
		t.Fatalf("the summary is plain text: %q", got[len(got)-2])
	}
	if body := got[len(got)-1]; body != "in 2 min\nR&amp;D" {
		t.Fatalf("body: %q", body)
	}
	if !strings.Contains(strings.Join(got, " "), "x-canonical-private-synchronous:orgs.remind") {
		t.Fatalf("a group replaces: %q", got)
	}
}

func TestClipKeepsRunes(t *testing.T) {
	s := strings.Repeat("é", 500)
	c := clip(s, 10)
	if r := []rune(c); len(r) != 10 || r[9] != '…' {
		t.Fatalf("clip: %q", c)
	}
	if clip("short", 10) != "short" {
		t.Fatal("short text is left alone")
	}
}

// Send refuses an empty note rather than popping a blank one.
func TestSendRefusesEmpty(t *testing.T) {
	if err := Send(Note{}); err == nil {
		t.Fatal("an empty note should be refused")
	}
}

// Through a notifier, with the program it runs captured.
func TestNotifiersRunTheirProgram(t *testing.T) {
	var ran []string
	old := run
	run = func(name string, args ...string) error {
		ran = append(ran, name+" "+strings.Join(args, " "))
		return nil
	}
	defer func() { run = old }()
	osascriptNotifier{}.Send(Note{Title: "t", Body: "b"})
	terminalNotifier{}.Send(Note{Title: "t", Body: "b"})
	notifySendNotifier{}.Send(Note{Title: "t", Body: "b"})
	if len(ran) != 3 || !strings.HasPrefix(ran[0], "osascript -e display notification") ||
		!strings.HasPrefix(ran[1], "terminal-notifier -title t") || !strings.HasPrefix(ran[2], "notify-send --app-name=orgs") {
		t.Fatalf("ran: %q", ran)
	}
}
