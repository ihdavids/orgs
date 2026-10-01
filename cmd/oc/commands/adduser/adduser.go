package adduser

// `orgs adduser` - add an account to a running server.
//
// The sibling of `orgs user add`, and the difference between them is where
// the keystore is written. `orgs user` edits the file itself, so it has to be
// run on the server's own machine and the server picks the change up when it
// restarts. This asks the server to do it, so it works from anywhere you can
// log in and the new account works at once.
//
// What it costs is that the server has to be willing, and it is only willing
// for an account marked `admin: true` in its keystore - see
// internal/app/orgs/adduser.go for why, and for the two states it refuses in
// outright (authentication switched off, and a server running on the built-in
// account with no keystore file behind it).
//
// The password does not travel. A salt is made for the new user here and what
// goes over the wire is common.ClientHash of the password under it, the same
// as a login sends - so the server can write the keystore without ever having
// been told the password.

import (
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type AddUserCmd struct {
	Password string
	Admin    bool
	fset     *flag.FlagSet
}

func (self *AddUserCmd) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *AddUserCmd) StartPlugin(manager *common.PluginManager) {
}

func (self *AddUserCmd) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Password, "password", "",
		"the password, instead of being prompted for it (it will be in your shell history)")
	fset.BoolVar(&self.Admin, "admin", false,
		"make the new account an administrator, so it can add users too")
}

func (self *AddUserCmd) Exec(core *commands.Core) {
	// Flags and words in any order, and FreeArgs consumes them as it parses -
	// so the words are taken once and read off that slice.
	words := commands.FreeArgs(self.fset)
	if len(words) == 0 {
		commands.Fail("adduser: which user? `orgs adduser <name> [-admin]`")
	}
	if len(words) > 1 {
		commands.Fail("adduser: one user at a time, got %d", len(words))
	}
	name := words[0]
	// Asked here as well as on the server. The server is the one that decides,
	// but finding out that a name was never going to work before being asked
	// for a password twice is the difference between a refusal and an errand.
	if err := common.ValidUsername(name); err != nil {
		commands.Fail("adduser: %v", err)
	}

	pass := commands.ReadNewPassword(fmt.Sprintf("Password for %s", name), self.Password)

	// A new account has no salt yet, so this side makes one. The server keeps
	// it as it stands and answers GET /salt with it from then on, which is
	// what lets the password be hashed here rather than sent.
	salt, err := common.NewSalt()
	if err != nil {
		commands.Fail("adduser: %v", err)
	}

	req := common.NewUser{
		Username: name,
		Salt:     salt,
		Password: common.ClientHash(name, salt, pass),
		Admin:    self.Admin,
	}
	var res common.ResultMsg
	commands.SendReceivePost(core, "users/add", &req, &res)
	if commands.DryRun {
		return
	}
	if !res.Ok {
		msg := res.Msg
		if msg == "" {
			msg = "the server refused, and said nothing about why"
		}
		commands.Fail("adduser: %s", msg)
	}

	commands.RenderOne(res, func() {
		what := "user"
		if self.Admin {
			what = "administrator"
		}
		fmt.Printf("%sAdded%s the %s %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), what, name)
		fmt.Printf("They can log in now: orgs login -user %s\n", name)
	})
}

// init function is called at boot
func init() {
	commands.AddCmd("adduser", "add a user to a running server (you must be an admin)",
		func() commands.Cmd {
			return &AddUserCmd{}
		})
}
