package code

// The python sessions `:session` started, from the terminal.
//
//	orgs code sessions                    what is running, file by file
//	orgs code sessions stop analysis      stop that one (a notebook's restart)
//	orgs code sessions stop -file a.org   every session of a file
//	orgs code sessions stop -all          all of them
//
// A session holds its variables between blocks, which is what makes it useful
// and also what makes it go stale: a block edited above a long-running one is
// not rerun, and the kernel goes on answering with yesterday's dataframe. worg
// has a restart button for this; the terminal had nothing short of restarting
// the server.
//
// Stopping is a write of a kind (a process is killed), so it goes through
// SendReceivePostErr and honours -dry-run. Several sessions and no name is a
// question: a picker for a person, a refusal for a pipe - the same rule the
// write verbs follow, because "stop all of them" should be said, not inferred.

import (
	"fmt"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// sessionInfo mirrors BabelSessionInfo in internal/app/orgs/babelsession.go,
// which this package cannot import.
type sessionInfo struct {
	Name     string
	File     string
	Python   string
	Version  string
	Pid      int
	Runs     int
	Started  time.Time
	LastUsed time.Time
}

// stopReq is POST /babel/sessions/stop's body: a file, a file and a name, or
// nothing for everything.
type stopReq struct {
	Filename string `json:",omitempty"`
	Name     string `json:",omitempty"`
}

func (self *Code) sessions(core *commands.Core, words []string) {
	if len(words) > 0 && (words[0] == "stop" || words[0] == "kill" || words[0] == "restart") {
		self.stopSessions(core, words[1:])
		return
	}
	if len(words) > 0 && words[0] != "ls" && words[0] != "list" {
		commands.Fail("orgs code sessions [ls | stop [name] [-file f] [-all]]")
	}
	list := self.fetchSessions(core)
	commands.Render(list, func() {
		if len(list) == 0 {
			fmt.Println("no python sessions running")
			return
		}
		lastFile := ""
		for _, s := range list {
			if s.File != lastFile {
				fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), s.File, commands.C(commands.AnsiReset))
				lastFile = s.File
			}
			fmt.Printf("  %s%-16s%s %s  pid %d  %d run%s  started %s, idle %s\n",
				commands.C(commands.AnsiCyan), s.Name, commands.C(commands.AnsiReset),
				strings.TrimSpace(s.Python+" "+s.Version), s.Pid, s.Runs, plural(s.Runs),
				ago(s.Started), ago(s.LastUsed))
		}
	})
}

func (self *Code) fetchSessions(core *commands.Core) []sessionInfo {
	list, err := commands.SendReceiveGetErr[[]sessionInfo](core, "babel/sessions", nil)
	if err != nil {
		commands.Fail("could not list sessions: %v", err)
	}
	if self.File != "" {
		kept := []sessionInfo{}
		for _, s := range list {
			if sameFile(s.File, self.File) {
				kept = append(kept, s)
			}
		}
		list = kept
	}
	return list
}

// A file named on the command line is usually a base name or a relative path,
// and the server lists absolute ones. Matching on the tail is what a person
// means; the server resolves the name again on stop, so a loose match here
// cannot stop the wrong file's sessions.
func sameFile(full, given string) bool {
	return full == given || strings.HasSuffix(full, "/"+strings.TrimPrefix(given, "./"))
}

func (self *Code) stopSessions(core *commands.Core, words []string) {
	name := strings.TrimSpace(strings.Join(words, " "))
	list := self.fetchSessions(core)

	var reqs []stopReq
	switch {
	case self.All && name == "":
		if self.File != "" && len(list) == 0 {
			commands.Fail("no sessions running in %s", self.File)
		}
		// One request: the server stops all of a file, or all of everything.
		file := ""
		if self.File != "" {
			file = list[0].File
		}
		reqs = append(reqs, stopReq{Filename: file})
	case name != "":
		for _, s := range list {
			if s.Name == name {
				reqs = append(reqs, stopReq{Filename: s.File, Name: s.Name})
			}
		}
		if len(reqs) == 0 {
			commands.Fail("no session called %q running", name)
		}
		if len(reqs) > 1 && !self.All {
			reqs = self.pickSessions(list, name, len(reqs))
		}
	default:
		if len(list) == 0 {
			commands.Fail("no python sessions running")
		}
		if len(list) == 1 {
			reqs = append(reqs, stopReq{Filename: list[0].File, Name: list[0].Name})
			break
		}
		for _, s := range self.pickFrom(list, len(list)) {
			reqs = append(reqs, stopReq{Filename: s.File, Name: s.Name})
		}
	}

	stopped := 0
	msgs := []string{}
	for _, r := range reqs {
		res, err := commands.SendReceivePostErr[stopReq, common.ResultMsg](core, "babel/sessions/stop", &r)
		if err == commands.ErrDryRun {
			continue
		}
		if err != nil {
			commands.Fail("could not stop: %v", err)
		}
		var n int
		fmt.Sscanf(res.Msg, "%d", &n)
		stopped += n
		what := r.Name
		if what == "" {
			what = "every session"
			if r.Filename != "" {
				what += " of " + commands.BaseName(r.Filename)
			}
		}
		msgs = append(msgs, what)
	}
	if commands.DryRun {
		return
	}
	commands.RenderOne(common.ResultMsg{Ok: true, Msg: fmt.Sprintf("%d stopped", stopped)}, func() {
		fmt.Printf("%sstopped%s %d (%s)\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset),
			stopped, strings.Join(msgs, ", "))
	})
}

// The same name in several files: which one did they mean.
func (self *Code) pickSessions(list []sessionInfo, name string, n int) []stopReq {
	same := []sessionInfo{}
	for _, s := range list {
		if s.Name == name {
			same = append(same, s)
		}
	}
	out := []stopReq{}
	for _, s := range self.pickFrom(same, n) {
		out = append(out, stopReq{Filename: s.File, Name: s.Name})
	}
	return out
}

func (self *Code) pickFrom(list []sessionInfo, n int) []sessionInfo {
	if !commands.Interactive() {
		commands.Fail("%d sessions match; name one, add -file, or say -all", n)
	}
	lines := []string{}
	for i, s := range list {
		lines = append(lines, commands.PickLine([]string{fmt.Sprint(i)},
			fmt.Sprintf("%-16s %s  pid %d  idle %s", s.Name, commands.BaseName(s.File), s.Pid, ago(s.LastUsed))))
	}
	chosen := commands.Pick(commands.PickOpts{
		Lines: lines, AddressFields: 1, Prompt: "Stop> ",
		Header: "tab: mark several   enter: stop", Extra: []string{"--multi"},
	})
	out := []sessionInfo{}
	for _, c := range chosen {
		if addr, ok := commands.Address(c, 1); ok {
			var i int
			fmt.Sscanf(addr[0], "%d", &i)
			if i >= 0 && i < len(list) {
				out = append(out, list[i])
			}
		}
	}
	if len(out) == 0 {
		commands.Fail("nothing stopped")
	}
	return out
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t).Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
