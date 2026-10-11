package notifications

// orgs notifications - what the watched queries found.
//
//	orgs notifications              the list, newest first, unread marked
//	orgs notifications -unread      only those not yet read
//	orgs notifications -read        the list, then mark them all read
//	orgs notifications -clear       remove them all
//	orgs notifications -follow      say each new one as it happens (ctrl-c ends)
//	orgs notifications -json        for a program
//
// The terminal half of GET /notifications and the `querymatch` events; the
// watches themselves are `orgs q watch`. -follow is `orgs watch -kinds
// querymatch` drawn as notifications, with -exec to hang a command off one
// (`-exec 'notify-send orgs {}'`, {} being the one-line summary).

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Notifications struct {
	Unread bool
	Read   bool
	Clear  bool
	Follow bool
	Limit  int
	OnEach string
}

func (self *Notifications) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Notifications) StartPlugin(m *common.PluginManager)       {}

func (self *Notifications) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Unread, "unread", false, "only notifications not yet read")
	fset.BoolVar(&self.Read, "read", false, "mark them all read after listing")
	fset.BoolVar(&self.Clear, "clear", false, "remove every notification")
	fset.BoolVar(&self.Follow, "follow", false, "print each new notification as it arrives")
	fset.IntVar(&self.Limit, "limit", 0, "at most this many")
	fset.StringVar(&self.OnEach, "exec", "", "with -follow: run this shell command on each; {} becomes the summary")
}

func (self *Notifications) Exec(core *commands.Core) {
	switch {
	case self.Follow:
		self.follow(core)
	case self.Clear:
		self.act(core, "notifications/clear", "cleared")
	default:
		self.list(core)
		if self.Read {
			self.act(core, "notifications/read", "")
		}
	}
}

func (self *Notifications) list(core *commands.Core) {
	ps := map[string]string{}
	if self.Unread {
		ps["unread"] = "t"
	}
	if self.Limit > 0 {
		ps["limit"] = fmt.Sprint(self.Limit)
	}
	res, err := commands.SendReceiveGetErr[common.NoticeList](core, "notifications", ps)
	if err != nil {
		commands.Fail("could not read the notifications: %v", err)
	}
	if res.Notices == nil {
		res.Notices = []common.WatchNotice{}
	}
	if commands.Render(res.Notices, nil) {
		return
	}
	if len(res.Notices) == 0 {
		if self.Unread || res.Total > 0 {
			fmt.Printf("%snothing unread%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		} else {
			fmt.Printf("%sno notifications. orgs q watch <saved query> asks for some.%s\n",
				commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		}
		return
	}
	for _, n := range res.Notices {
		printNotice(n, !n.Read)
	}
	fmt.Printf("%s%d unread of %d%s\n", commands.C(commands.AnsiDim), res.Unread, res.Total,
		commands.C(commands.AnsiReset))
}

func (self *Notifications) act(core *commands.Core, path, said string) {
	res, err := commands.SendReceivePostErr[common.NoticeAct, common.NoticeList](core, path,
		&common.NoticeAct{All: true})
	if err == commands.ErrDryRun {
		return
	}
	if err != nil || !res.Ok {
		commands.Fail("%s: %v %s", path, err, res.Msg)
	}
	if said != "" {
		if commands.RenderOne(res, nil) {
			return
		}
		fmt.Printf("%s✓%s %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), said)
	}
}

// printNotice is one notification: a line saying when and which watch, then a
// line per heading. Unread ones are bold, and gone headings are dimmed.
func printNotice(n common.WatchNotice, unread bool) {
	at := n.At
	if t, err := time.Parse(time.RFC3339, n.At); err == nil {
		at = whenSaid(t)
	}
	mark := " "
	weight := ""
	if unread {
		mark = commands.C(commands.AnsiGold) + "●" + commands.C(commands.AnsiReset)
		weight = commands.C(commands.AnsiBold)
	}
	fmt.Printf("%s %s%s%s  %s%s · %d now%s\n", mark, weight, n.Label, commands.C(commands.AnsiReset),
		commands.C(commands.AnsiDim), at, n.Count, commands.C(commands.AnsiReset))
	for _, m := range n.Gained {
		fmt.Printf("    %s+%s %s%s %s%s:%d%s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset),
			keyword(m.Status), m.Headline, commands.C(commands.AnsiDim), commands.BaseName(m.File), m.Line+1,
			commands.C(commands.AnsiReset))
	}
	for _, m := range n.Left {
		fmt.Printf("    %s- %s%s %s:%d%s\n", commands.C(commands.AnsiDim), keyword(m.Status), m.Headline,
			commands.BaseName(m.File), m.Line+1, commands.C(commands.AnsiReset))
	}
}

func keyword(s string) string {
	if s == "" {
		return ""
	}
	return commands.C(commands.AnsiGold) + s + commands.C(commands.AnsiReset) + " "
}

// whenSaid is a time the way a person reads a list of notifications: the
// clock today, the day this week, the date before that.
func whenSaid(t time.Time) string {
	now := time.Now()
	t = t.Local()
	switch {
	case t.Year() == now.Year() && t.YearDay() == now.YearDay():
		return t.Format("15:04")
	case now.Sub(t) < 6*24*time.Hour:
		return t.Format("Mon 15:04")
	}
	return t.Format("2006-01-02 15:04")
}

func (self *Notifications) follow(core *commands.Core) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if !commands.Machine() {
		fmt.Fprintf(os.Stderr, "%swaiting for watched queries to find something · ctrl-c to stop%s\n",
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	}
	err := common.Events(ctx, &core.Rest, common.EventOpts{
		Kinds: []string{"querymatch"},
		OnDisconnect: func(err error) {
			fmt.Fprintf(os.Stderr, "%slost the server (%v) - retrying%s\n",
				commands.C(commands.AnsiGold), err, commands.C(commands.AnsiReset))
		},
	}, func(e common.OrgEvent) bool {
		if e.Kind != "querymatch" {
			return true
		}
		n := common.WatchNotice{Id: e.Notice, Watch: e.Watch, Label: e.Label, At: e.At,
			Gained: e.Headings, Left: e.Left, Count: e.Count}
		if !commands.RenderOne(n, nil) {
			printNotice(n, true)
		}
		if self.OnEach != "" {
			self.run(e)
		}
		return true
	})
	if err != nil {
		commands.Fail("orgs notifications: %v", err)
	}
}

// run hands one notification to a shell, as `orgs watch -exec` does.
func (self *Notifications) run(e common.OrgEvent) {
	summary := e.Label
	if e.Msg != "" {
		summary += ": " + e.Msg
	}
	cmdline := strings.ReplaceAll(self.OnEach, "{}", "'"+strings.ReplaceAll(summary, "'", `'\''`)+"'")
	if commands.Wrote("run "+cmdline, nil) {
		return
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-c", cmdline)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s%s → %v%s\n", commands.C(commands.AnsiRed), cmdline, err,
			commands.C(commands.AnsiReset))
	}
}

func init() {
	commands.AddCmd("notifications", "what your watched queries found (orgs q watch); -follow for live",
		func() commands.Cmd { return &Notifications{} })
}
