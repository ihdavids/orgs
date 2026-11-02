package dav

// `orgs dav`: setting a phone up to see the agenda and contacts over CalDAV
// and CardDAV.
//
//	orgs dav                    whether it is on, the addresses, the app passwords
//	orgs dav password phone     make an app password for a device (shown once)
//	orgs dav revoke 3fa1c2d0    cut one device off (revoke all: every one)
//
// The server half is internal/app/orgs/dav.go: off unless `dav: enable: true`
// in the server's yaml, https only (or this machine, or dav.allowHttp), and
// only with an app password - never the login password.

import (
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Dav struct {
	fset *flag.FlagSet
}

func (self *Dav) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Dav) StartPlugin(manager *common.PluginManager)       {}
func (self *Dav) SetupParameters(fset *flag.FlagSet)              { self.fset = fset }
func (self *Dav) HelpGroup() string                                { return "Setup" }

type pw struct {
	Id       string
	Label    string
	Created  time.Time
	LastUsed time.Time
}

type status struct {
	Ok        bool
	Msg       string
	Enable    bool
	ReadOnly  bool
	Calendar  string
	Contacts  string
	User      string
	Passwords []pw
}

func (self *Dav) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb = words[0]
	}
	switch verb {
	case "", "status":
		s, err := commands.SendReceiveGetErr[status](core, "davconfig", nil)
		if err != nil {
			commands.Fail("%v", err)
		}
		commands.RenderOne(s, func() { show(core, s) })
	case "password", "passwd", "new":
		label := strings.Join(words[1:], " ")
		res, err := commands.SendReceivePostErr[struct{ Label string }, struct {
			Ok       bool
			Msg      string
			Id       string
			Label    string
			User     string
			Password string
		}](core, "davconfig/passwords", &struct{ Label string }{label})
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		if !res.Ok {
			commands.Fail("%s", res.Msg)
		}
		commands.RenderOne(res, func() {
			fmt.Printf("App password for %s%s%s (%s, id %s):\n\n    %s%s%s\n\n",
				commands.C(commands.AnsiBold), res.User, commands.C(commands.AnsiReset), res.Label, res.Id,
				commands.C(commands.AnsiBold), res.Password, commands.C(commands.AnsiReset))
			fmt.Println("It is shown once. On the phone add a CalDAV (and a CardDAV) account:")
			fmt.Printf("    server    %s\n    user      %s\n    password  the one above\n", host(core), res.User)
			fmt.Println("Revoke it with: orgs dav revoke " + res.Id)
		})
	case "revoke", "rm":
		if len(words) < 2 {
			commands.Fail("orgs dav revoke ID   (orgs dav lists them; all revokes every one)")
		}
		ps := map[string]string{"id": words[1]}
		if commands.Wrote("DELETE /davconfig/passwords", ps) {
			return
		}
		raw, err := core.Rest.Delete("davconfig/passwords", ps)
		if err != nil {
			commands.Fail("%v", err)
		}
		fmt.Println(strings.TrimSpace(string(raw)))
	default:
		commands.Fail("orgs dav [status | password LABEL | revoke ID]")
	}
}

func host(core *commands.Core) string {
	u, err := url.Parse(core.Rest.Url)
	if err != nil {
		return core.Rest.Url
	}
	return u.Host
}

func show(core *commands.Core, s status) {
	if !s.Enable {
		fmt.Println("CalDAV/CardDAV is off. Turn it on in the server's yaml:\n\n  dav:\n    enable: true")
	} else {
		mode := ""
		if s.ReadOnly {
			mode = " (read only)"
		}
		fmt.Printf("CalDAV/CardDAV is %son%s%s for %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), mode, s.User)
		base := strings.TrimSuffix(core.Rest.Url, "/")
		if s.Calendar != "" {
			fmt.Printf("  calendar  %s%s\n", base, s.Calendar)
		}
		if s.Contacts != "" {
			fmt.Printf("  contacts  %s%s\n", base, s.Contacts)
		}
		if strings.HasPrefix(base, "http://") {
			fmt.Println("  (phones must use the https port, unless dav.allowHttp is set)")
		}
	}
	if len(s.Passwords) == 0 {
		fmt.Println("No app passwords yet: orgs dav password phone")
		return
	}
	fmt.Println("App passwords:")
	for _, p := range s.Passwords {
		used := "never used"
		if !p.LastUsed.IsZero() {
			used = "last used " + p.LastUsed.Local().Format("2006-01-02 15:04")
		}
		fmt.Printf("  %s  %-20s made %s, %s\n", p.Id, p.Label, p.Created.Local().Format("2006-01-02"), used)
	}
}

func init() {
	commands.AddCmd("dav", "set a phone up for CalDAV/CardDAV: status, app passwords, revoking them",
		func() commands.Cmd { return &Dav{} })
}
