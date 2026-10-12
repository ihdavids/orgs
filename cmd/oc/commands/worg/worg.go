package worg

// Opens worg in the browser, already logged in: asks the server for a one-use
// login link (POST /login/ticket, see internal/app/orgs/ticket.go) and opens
// that, so nobody types a password - which for the server VS Code set up
// nobody knows.
//
//	orgs worg            the agenda, or whatever worg opens on
//	orgs worg Kanban     a view by the name worg switches on
//	orgs worg -print     the link instead of opening it

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Worg struct {
	Print bool
	fset  *flag.FlagSet
}

type ticketResponse struct {
	Ticket string `json:"ticket"`
	Url    string `json:"url"`
}

func (self *Worg) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Worg) StartPlugin(manager *common.PluginManager) {
}

func (self *Worg) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Print, "print", false, "print the login link (good for a minute, once) instead of opening it")
}

func (self *Worg) HelpGroup() string { return "The server" }

func (self *Worg) Exec(core *commands.Core) {
	app := ""
	if args := commands.FreeArgs(self.fset); len(args) > 0 {
		app = args[0]
	}
	resp, err := commands.SendReceivePostErr[struct{}, ticketResponse](core, "login/ticket", &struct{}{})
	if err == commands.ErrDryRun {
		return
	}
	if err != nil || resp.Url == "" {
		commands.Fail("worg: the server gave no login link (%v) - is it older than this command, or are you not logged in?", err)
		os.Exit(1)
	}
	link := strings.TrimRight(core.Rest.Url, "/") + resp.Url
	if app != "" {
		link += "?app=" + url.QueryEscape(app)
	}
	if self.Print || commands.Machine() {
		fmt.Println(link)
		return
	}
	commands.OpenInBrowser(link)
}

func init() {
	commands.AddCmd("worg", "open worg in the browser, logged in: orgs worg [View]", func() commands.Cmd {
		return &Worg{}
	})
}
