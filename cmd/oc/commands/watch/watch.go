package watch

// orgs watch - say when something changes, or do something about it.
//
//	orgs watch                          print each change as it happens
//	orgs watch -exec 'make notes'       run that when anything changes
//	orgs watch -exec 'orgs tangle {}' -file lit.org
//	orgs watch -kinds clockin,clockout  only the clock
//	orgs watch -once                    wait for one change and exit
//	orgs watch -json                    one json object per line, for a program
//
// The server has watched the org files since the beginning and, until /events,
// had no way to say so. This is the plainest possible use of that: a stream you
// can read, pipe, or hang a command off.
//
// `-exec` is what makes it more than a curiosity. A file of literate config that
// has to be tangled on every save, a static site that has to be rebuilt, a
// `notify-send` when a clock has been running too long - all of them are this
// command and a shell string.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Watch struct {
	Kinds  string
	OnEach string
	File   string
	Once   bool
	Quiet  bool
}

func (self *Watch) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Watch) StartPlugin(m *common.PluginManager)       {}

func (self *Watch) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Kinds, "kinds", "",
		"only these events, comma separated: reload, clockin, clockout")
	fset.StringVar(&self.OnEach, "exec", "",
		"run this shell command on each change; {} becomes the file that changed")
	fset.StringVar(&self.File, "file", "",
		"only changes to this file - a name or a path")
	fset.BoolVar(&self.Once, "once", false, "wait for one change, then exit")
	fset.BoolVar(&self.Quiet, "quiet", false, "print nothing; useful with -exec")
}

func (self *Watch) Exec(core *commands.Core) {
	kinds := []string{}
	if self.Kinds != "" {
		for _, k := range strings.Split(self.Kinds, ",") {
			if k = strings.TrimSpace(k); k != "" {
				kinds = append(kinds, k)
			}
		}
	}

	// ctrl-c has to end this rather than killing it, so a command started by
	// -exec is not left half finished and the last line is not a stack trace.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !self.Quiet && !commands.Machine() {
		where := "everything"
		if self.File != "" {
			where = self.File
		}
		fmt.Fprintf(os.Stderr, "%swatching %s · ctrl-c to stop%s\n",
			commands.C(commands.AnsiDim), where, commands.C(commands.AnsiReset))
	}

	err := common.Events(ctx, &core.Rest, common.EventOpts{
		Kinds: kinds,
		Once:  false,
		OnDisconnect: func(err error) {
			// Said on stderr, so it never lands in the middle of -json output.
			// Worth saying at all: a watch that has quietly stopped watching
			// looks exactly like a database where nothing is happening.
			fmt.Fprintf(os.Stderr, "%slost the server (%v) - retrying%s\n",
				commands.C(commands.AnsiGold), err, commands.C(commands.AnsiReset))
		},
		OnReconnect: func() {},
	}, func(e common.OrgEvent) bool {
		// The hello is the stream saying it is connected, not a change. Acting
		// on it would run -exec once at startup for no reason.
		if e.Kind == "hello" {
			return true
		}
		if !self.matches(e) {
			return true
		}
		self.report(e)
		if self.OnEach != "" {
			self.run(e)
		}
		return !self.Once
	})
	if err != nil {
		commands.Fail("orgs watch: %v", err)
	}
}

// matches is the -file filter. Matched on the base name as well as the path,
// because nobody types an absolute path at a flag and the server always answers
// with one.
func (self *Watch) matches(e common.OrgEvent) bool {
	if self.File == "" {
		return true
	}
	if e.File == "" {
		// An event with no file (a clock) cannot match a file filter.
		return false
	}
	return e.File == self.File || filepath.Base(e.File) == self.File ||
		strings.HasSuffix(e.File, "/"+self.File)
}

func (self *Watch) report(e common.OrgEvent) {
	if self.Quiet {
		return
	}
	if commands.RenderOne(e, nil) {
		return
	}
	at := e.At
	if t, err := time.Parse(time.RFC3339, e.At); err == nil {
		at = t.Format("15:04:05")
	}
	what := e.File
	if what == "" {
		what = e.Msg
	} else {
		what = commands.BaseName(what)
	}
	fmt.Printf("%s%s%s %s%-9s%s %s\n", commands.C(commands.AnsiDim), at,
		commands.C(commands.AnsiReset), commands.C(kindInk(e.Kind)), e.Kind,
		commands.C(commands.AnsiReset), what)
}

// run is the -exec half. The command goes to a shell rather than being split
// here, because what people write is a pipeline - `orgs tangle {} -w | tee
// log` - and a splitter that handled that would be a shell.
func (self *Watch) run(e common.OrgEvent) {
	cmdline := strings.ReplaceAll(self.OnEach, "{}", shq(e.File))
	if commands.Wrote("run "+cmdline, nil) {
		return
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-c", cmdline)
	// Inherited, so the command's output is the watch's output and a person
	// watching sees what it said.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	// A failing command is reported and does not stop the watch: the next save
	// is very often the fix for it, and a watcher that exits on the first
	// compile error is a watcher you have to keep restarting.
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s%s → %v%s\n", commands.C(commands.AnsiRed), cmdline, err,
			commands.C(commands.AnsiReset))
	}
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func kindInk(kind string) string {
	switch kind {
	case "reload":
		return commands.AnsiCyan
	case "clockin":
		return commands.AnsiGreen
	case "clockout":
		return commands.AnsiGold
	}
	return commands.AnsiDim
}

func init() {
	commands.AddCmd("watch", "say when an org file changes, and run something about it",
		func() commands.Cmd { return &Watch{} })
}
