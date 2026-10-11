package focus

// `orgs focus bar`: the session in the macOS menu bar, through xbar
// (https://xbarapp.com) or SwiftBar (https://swiftbar.app), which run a script
// on a timer and draw what it prints: the first line in the bar, the lines
// after `---` as the menu. A menu line with `bash=` runs that program when
// picked, so stop, +5 and skip call straight back into `orgs focus`.
//
// Linux status bars (waybar, polybar, i3blocks) want one line, which is
// `orgs focus status`.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

const barHelp = `orgs focus bar: the focus session in the macOS menu bar.

Install SwiftBar (brew install swiftbar) or xbar (brew install xbar), then:

    orgs focus bar install "$HOME/Library/Application Support/SwiftBar/Plugins"

or for xbar:

    orgs focus bar install "$HOME/Library/Application Support/xbar/plugins"

That writes orgs-focus.1s.sh there, a two line script that runs this binary
with the -config and -url in use now. The 1s in the name is how often the bar
redraws; rename the file to orgs-focus.5s.sh for every five seconds. The menu
stops, extends and skips through orgs focus, and opens worg's Focus tab.

For tmux, a shell prompt, waybar or polybar, use one line instead:

    set -g status-right '#(orgs focus status)'
`

// The program and the global flags the menu's commands need, as xbar
// params: bash=/path/orgs param1=-config param2=... A value with a space in it
// is quoted, which both apps accept.
func barCommand(core *commands.Core, args ...string) string {
	exe, err := os.Executable()
	if err != nil {
		exe = "orgs"
	}
	all := []string{}
	if core.ConfigFile != "" {
		all = append(all, "-config", core.ConfigFile)
	}
	if core.Rest.Url != "" {
		all = append(all, "-url", core.Rest.Url)
	}
	all = append(all, args...)
	out := "bash=" + barQuote(exe)
	for i, a := range all {
		out += fmt.Sprintf(" param%d=%s", i+1, barQuote(a))
	}
	return out + " terminal=false refresh=true"
}

func barQuote(s string) string {
	if strings.ContainsAny(s, " \t\"'|") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

// A menu line must not contain a bare `|`, which is where xbar's parameters
// start.
func barText(s string) string {
	return strings.ReplaceAll(s, "|", "¦")
}

func printBar(core *commands.Core, st common.FocusState) {
	if commands.JsonOut {
		commands.RenderOne(st, nil)
		return
	}
	out := []string{}
	add := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }

	switch st.Phase {
	case "work":
		add("%s %s", Glyph("work"), Countdown(st.Remaining))
	case "break":
		add("%s %s", Glyph("break"), Countdown(st.Remaining))
	default:
		// Something in the bar even when idle, or there is nothing to click to
		// start another one. Dimmed so it does not read as running.
		add("%s | color=#999999", Glyph("work"))
	}
	add("---")
	switch st.Phase {
	case "work":
		add("%s | length=60", barText(st.Title))
		add("Until %s · %s today | color=#888888", endClock(st), pomodoros(st.Today))
		add("---")
		add("Stop | %s", barCommand(core, "focus", "stop"))
		add("Five more minutes | %s", barCommand(core, "focus", "extend", "5"))
		add("Done, take the break | %s", barCommand(core, "focus", "skip"))
	case "break":
		add("%s until %s | color=#888888", strings.Title(phaseName(st)), endClock(st))
		add("%s today | color=#888888", pomodoros(st.Today))
		add("---")
		add("Skip the break, start the next | %s", barCommand(core, "focus", "skip"))
		add("Five more minutes | %s", barCommand(core, "focus", "extend", "5"))
		add("End the break | %s", barCommand(core, "focus", "stop"))
	default:
		add("%s today, %s | color=#888888", pomodoros(st.Today), Mins(st.TodayMinutes))
		if st.Title != "" {
			add("---")
			add("Another on %s | %s length=50", barText(st.Title), barCommand(core, "focus", "again", "-d"))
		}
	}
	add("---")
	if core.Rest.Url != "" {
		add("Open worg's Focus tab | href=%s/?app=Focus", strings.TrimRight(core.Rest.Url, "/"))
	}
	add("Refresh | refresh=true")
	fmt.Println(strings.Join(out, "\n"))
}

// installBar writes the plugin script into a SwiftBar or xbar plugin folder.
func installBar(core *commands.Core, args []string) {
	if len(args) == 0 {
		fmt.Print(barHelp)
		return
	}
	dir := args[0]
	exe, err := os.Executable()
	if err != nil {
		commands.Fail("could not find this program: %v", err)
	}
	cmd := []string{commands.Shq(exe)}
	if core.ConfigFile != "" {
		if abs, err := filepath.Abs(core.ConfigFile); err == nil {
			cmd = append(cmd, "-config", commands.Shq(abs))
		}
	}
	if core.Rest.Url != "" {
		cmd = append(cmd, "-url", commands.Shq(core.Rest.Url))
	}
	script := "#!/bin/sh\n" +
		"# <xbar.title>orgs focus</xbar.title>\n" +
		"# <xbar.desc>The orgs pomodoro timer: task, countdown, stop, +5, skip.</xbar.desc>\n" +
		"# <swiftbar.hideRunInTerminal>true</swiftbar.hideRunInTerminal>\n" +
		"exec " + strings.Join(cmd, " ") + " focus bar 2>/dev/null\n"
	path := filepath.Join(dir, "orgs-focus.1s.sh")
	if commands.Wrote("write "+path, script) {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		commands.Fail("could not make %s: %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		commands.Fail("could not write %s: %v", path, err)
	}
	fmt.Println("wrote", path)
}
