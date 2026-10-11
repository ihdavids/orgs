// orgs notify - how the server tells you things, and a way to hear it here.
//
//	orgs notify status          what the server does: reminders, digest, quiet
//	                            hours, how its machine shows a notification
//	orgs notify test            one notification through the server's own path
//	orgs notify test -here      one shown on this machine, no server involved
//	orgs notify listen          show the server's notifications on this machine
//
// `listen` is for a server that is somewhere else - a home server, a box under
// the desk, a VPS - whose own desktop is not where you are. It follows the
// `notify` events (reminders, the deadline digest, watches with desktop on,
// focus phases) and shows each with this machine's own notifier, so a click
// opens worg on the server.
package notifycmd

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/desktop"
)

type Notify struct {
	Here    bool
	Quiet   string
	NoSound bool
	Open    string
	Title   string
	fset    *flag.FlagSet
}

func (self *Notify) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Notify) StartPlugin(manager *common.PluginManager)       {}

func (self *Notify) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Here, "here", false, "test: show it on this machine instead of asking the server")
	fset.StringVar(&self.Quiet, "quiet", "", "listen: this machine's quiet hours, e.g. 22:00-07:00")
	fset.BoolVar(&self.NoSound, "no-sound", false, "listen: never play a sound")
	fset.StringVar(&self.Open, "open", "", "listen: worg's address for a click, when the server's own guess is wrong")
	fset.StringVar(&self.Title, "title", "", "test: the notification's title")
}

func (self *Notify) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := "status"
	if len(words) > 0 {
		verb, words = words[0], words[1:]
	}
	switch verb {
	case "status":
		self.status(core)
	case "test":
		self.test(core, strings.Join(words, " "))
	case "listen":
		self.listen(core)
	default:
		commands.Fail("orgs notify status | test [message] | listen - not %q", verb)
	}
}

func (self *Notify) status(core *commands.Core) {
	st, err := commands.SendReceiveGetErr[common.NotifyStatus](core, "notify/status", map[string]string{})
	if err != nil {
		commands.Fail("could not ask the server: %v", err)
	}
	here := desktop.Default().Name()
	if commands.RenderOne(map[string]any{"server": st, "here": here}, nil) {
		return
	}
	dim, bold, ok, warn, reset := commands.C(commands.AnsiDim), commands.C(commands.AnsiBold),
		commands.C(commands.AnsiGreen), commands.C(commands.AnsiGold), commands.C(commands.AnsiReset)
	line := func(k, v string) { fmt.Printf("  %s%-12s%s %s\n", dim, k, reset, v) }
	fmt.Printf("%sThe server%s\n", bold, reset)
	if !st.Plugin {
		fmt.Printf("  %s%s%s\n", warn, st.Msg, reset)
		fmt.Printf("  %sadd a notify plugin to server.plugins for reminders and the deadline digest%s\n", dim, reset)
	} else {
		remind := []string{}
		for _, m := range st.Remind {
			remind = append(remind, fmt.Sprintf("%d min", m))
		}
		line("reminders", strings.Join(remind, ", ")+" before a timed appointment")
		if st.Deadlines != "" {
			line("deadlines", "digest at "+st.Deadlines)
		} else {
			line("deadlines", "no digest")
		}
		pops := ok + "pops on its machine" + reset + " · " + st.Backend
		if !st.Desktop {
			pops = "events only (desktop: false)"
		}
		line("shows", pops)
		if st.Quiet != "" {
			q := st.Quiet
			if st.QuietNow {
				q += warn + "  (quiet now)" + reset
			}
			line("quiet", q)
		}
		line("sound", yesNo(st.Sound))
		line("focus", yesNo(st.Focus))
	}
	if st.Open != "" {
		line("clicks open", st.Open)
	}
	fmt.Printf("%sThis machine%s\n", bold, reset)
	line("shows with", here)
	if strings.Contains(here, "osascript") {
		fmt.Printf("  %sbrew install terminal-notifier: a click opens the heading, and reminders replace each other%s\n", dim, reset)
	}
}

func yesNo(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func (self *Notify) test(core *commands.Core, msg string) {
	title := self.Title
	if title == "" {
		title = "orgs"
	}
	if msg == "" {
		msg = "Notifications are working."
	}
	if self.Here {
		n := desktop.Note{Title: title, Body: msg, Sound: true, Group: "test"}
		if err := desktop.Send(n); err != nil {
			commands.Fail("could not show it here (%s): %v", desktop.Default().Name(), err)
		}
		if !commands.RenderOne(map[string]any{"Ok": true, "Backend": desktop.Default().Name()}, nil) {
			fmt.Printf("shown here with %s\n", desktop.Default().Name())
		}
		return
	}
	req := map[string]string{"Title": title, "Body": msg}
	res, err := commands.SendReceivePostErr[map[string]string, common.NotifyTestResult](core, "notify/test", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("could not ask the server: %v", err)
	}
	if commands.RenderOne(res, nil) {
		return
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	fmt.Printf("%s (%s)\n", res.Msg, res.Backend)
}

// listen shows every notify event with this machine's notifier until stopped.
func (self *Notify) listen(core *commands.Core) {
	quiet, err := common.ParseQuiet(self.Quiet)
	if err != nil {
		commands.Fail("%v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if !commands.Machine() {
		fmt.Fprintf(os.Stderr, "%sshowing %s's notifications here with %s · ctrl-c to stop%s\n",
			commands.C(commands.AnsiDim), core.Rest.Url, desktop.Default().Name(), commands.C(commands.AnsiReset))
	}
	// Said once when the server goes, and once when it is back - not once per
	// retry while a server restarts.
	lost := false
	err = common.Events(ctx, &core.Rest, common.EventOpts{
		Kinds: []string{"notify"},
		OnDisconnect: func(err error) {
			if lost {
				return
			}
			lost = true
			fmt.Fprintf(os.Stderr, "%slost the server (%v) - retrying%s\n",
				commands.C(commands.AnsiGold), err, commands.C(commands.AnsiReset))
		},
		OnReconnect: func() {
			if !lost {
				return
			}
			lost = false
			fmt.Fprintf(os.Stderr, "%sback%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		},
	}, func(e common.OrgEvent) bool {
		if e.Kind != "notify" {
			return true
		}
		n := noteFromEvent(e, self.Open, core.Rest.Url)
		if self.NoSound {
			n.Sound = false
		}
		shown := false
		if n.Urgency == desktop.Critical || !quiet.In(time.Now()) {
			if err := desktop.Send(n); err != nil {
				fmt.Fprintf(os.Stderr, "could not show it: %v\n", err)
			} else {
				shown = true
			}
		}
		if !commands.RenderOne(e, nil) {
			mark := commands.C(commands.AnsiGreen) + "●" + commands.C(commands.AnsiReset)
			if !shown {
				mark = commands.C(commands.AnsiDim) + "○" + commands.C(commands.AnsiReset)
			}
			fmt.Printf("%s %s  %s%s%s\n", mark, n.Title, commands.C(commands.AnsiDim),
				strings.ReplaceAll(strings.TrimSpace(n.Body), "\n", " · "), commands.C(commands.AnsiReset))
		}
		return true
	})
	if err != nil {
		commands.Fail("orgs notify listen: %v", err)
	}
}

// The note an event stands for. A link the server made for its own machine
// (localhost) is moved onto the address this client reaches the server at, or
// onto -open, so a click here opens the server's worg rather than nothing.
func noteFromEvent(e common.OrgEvent, open, serverURL string) desktop.Note {
	return desktop.Note{
		Title:   e.Title,
		Body:    e.Msg,
		URL:     rebase(e.URL, open, serverURL),
		Sound:   e.Sound,
		Urgency: common.ParseUrgency(e.Urgency),
		Group:   e.Group,
	}
}

func rebase(link, open, serverURL string) string {
	if link == "" {
		return ""
	}
	u, err := url.Parse(link)
	if err != nil {
		return link
	}
	base := open
	if base == "" {
		host := u.Hostname()
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return link
		}
		base = serverURL
	}
	b, err := url.Parse(base)
	if err != nil || b.Host == "" {
		return link
	}
	u.Scheme, u.Host = b.Scheme, b.Host
	return u.String()
}

func init() {
	commands.AddCmd("notify", "notifications: status, a test, and listen (show the server's notifications on this machine)",
		func() commands.Cmd { return &Notify{} })
}
