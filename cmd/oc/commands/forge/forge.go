package forge

// orgs forge: the github and gitlab pollers from the terminal.
//
//	orgs forge                 poll now (the same as orgs forge sync)
//	orgs forge check           does each token work, and whose is it
//	orgs forge token github    put a token in the keyring, asked for unechoed
//	gh auth token | orgs forge token github     or read from a pipe
//
// The token goes into this machine's keyring under the name the server reads
// (orgs-github or orgs-gitlab, or -name for a poller with its own keyring:),
// so it is set up on the machine the server runs on. It never goes in the
// yaml, and never on the command line where the shell history would keep it.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/tmc/keyring"
)

type Forge struct {
	name string
	fset *flag.FlagSet
}

func (self *Forge) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Forge) StartPlugin(manager *common.PluginManager) {}

func (self *Forge) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.name, "name", "", "token: the keyring entry, when the poller's yaml names its own keyring:")
}

func (self *Forge) HelpGroup() string { return "The server" }

func (self *Forge) Exec(core *commands.Core) {
	args := commands.FreeArgs(self.fset)
	verb := "sync"
	if len(args) > 0 {
		verb = args[0]
	}
	switch verb {
	case "sync":
		reply, err := commands.SendReceivePostErr[common.Empty, common.ForgeReply](core, "forge/sync", &common.Empty{})
		if errors.Is(err, commands.ErrDryRun) {
			return
		}
		show(reply, err, false)
	case "check":
		reply, err := commands.SendReceiveGetErr[common.ForgeReply](core, "forge/check", nil)
		show(reply, err, true)
	case "token":
		if len(args) < 2 || (args[1] != "github" && args[1] != "gitlab") {
			commands.Fail("usage: orgs forge token github|gitlab [-name keyring-entry]")
		}
		self.storeToken(args[1])
	default:
		commands.Fail("orgs forge: sync, check or token - not %q", verb)
	}
}

func (self *Forge) storeToken(provider string) {
	name := self.name
	if name == "" {
		name = "orgs-" + provider
	}
	var tok string
	if commands.Interactive() {
		tok = commands.ReadPasswordOnce(fmt.Sprintf("%s token: ", provider))
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			commands.Fail("could not read the token: %v", err)
		}
		tok = string(b)
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		commands.Fail("no token given")
	}
	if commands.Wrote("store a token in the keyring as "+name, nil) {
		return
	}
	if err := keyring.Set("orgs", name, tok); err != nil {
		commands.Fail("could not store it in the keyring: %v", err)
	}
	fmt.Fprintf(os.Stderr, "stored as %s. `orgs forge check` once the server has restarted.\n", name)
}

func show(reply common.ForgeReply, err error, check bool) {
	if len(reply.Results) == 0 {
		if err == nil && reply.Msg != "" {
			err = errors.New(reply.Msg)
		}
		if err == nil {
			err = errors.New("no answer from the server")
		}
		commands.Fail("forge: %v", err)
	}
	commands.RenderOne(reply, func() {
		for _, r := range reply.Results {
			mark := commands.C("32") + "ok" + commands.C("0")
			if !r.Ok {
				mark = commands.C("31") + "failed" + commands.C("0")
			}
			fmt.Printf("%s %s %s (%s)\n", mark, r.Name, r.File, r.Url)
			if r.Msg != "" {
				fmt.Printf("   %s\n", r.Msg)
			}
			if check {
				if r.User != "" {
					fmt.Printf("   token belongs to %s\n", r.User)
				}
				continue
			}
			if r.Ok {
				fmt.Printf("   %d new, %d updated, %d closed, %d reopened, %d gone, %d sent upstream\n",
					r.Added, r.Updated, r.Closed, r.Reopened, r.Gone, r.Pushed)
			}
			for _, n := range r.Notes {
				fmt.Printf("   %s\n", n)
			}
		}
	})
	if !reply.Ok {
		os.Exit(1)
	}
}

func init() {
	commands.AddCmd("forge", "github / gitlab issues: sync now, check the token, or store one in the keyring",
		func() commands.Cmd {
			return &Forge{}
		})
}
