package user

// `orgs user` - who may log into this server.
//
// This is the one command that edits a file on disk rather than asking the
// server to do something, and it is deliberate: the keystore is what decides
// who may talk to the server at all, so an endpoint that wrote it would be an
// endpoint that grants access, reachable by anybody who already has some. The
// consequence is the one worth knowing before reaching for it - it has to be
// run on the machine the server runs on, and the server reads the file on
// startup, so a change takes effect when it is restarted.
//
// It reads the same configuration the server does, so `keystore:` names the
// same file for both, and the hashing is common.Keystore's so the two cannot
// disagree about what a password is.
//
// `orgs adduser` is the other way to add an account: over the wire, to a
// running server, as an administrator. This command is where an administrator
// comes from - `orgs user admin <name>` is the one thing that cannot be done
// remotely, because being able to would mean anybody who could add a user
// could promote themselves.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type UserCmd struct {
	Password string
	Force    bool
	Admin    bool
	fset     *flag.FlagSet
}

// A row of `orgs user ls`. What it says about a password is whether it is
// stored as a hash, still cleartext, or still the built-in default - which are
// the three states the server's startup warning is about.
type userRow struct {
	Name      string
	Admin     bool
	Password  string
	LastLogin string
}

func (self *UserCmd) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *UserCmd) StartPlugin(manager *common.PluginManager) {
}

// Nothing here talks to a server, and the machine being set up for the first
// time is exactly the one with no server running and no token to send it.
func (self *UserCmd) NeedsNoServer() bool { return true }

func (self *UserCmd) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Password, "password", "",
		"the password, instead of being prompted for it (it will be in your shell history)")
	fset.BoolVar(&self.Force, "force", false, "do not ask before removing a user")
	fset.BoolVar(&self.Admin, "admin", false,
		"with `add`, make the new account an administrator (it may then add users itself)")
}

func (self *UserCmd) Exec(core *commands.Core) {
	// FreeArgs consumes the arguments as it parses them, so the words have to
	// be taken once and the subcommand read off that slice - asking the flag
	// set afterwards gets nothing.
	words := commands.FreeArgs(self.fset)
	sub := "ls"
	if len(words) > 0 {
		sub = words[0]
		words = words[1:]
	}

	if core.ServerSettings == nil {
		commands.Fail("user: this config file has no server: block, so there is no keystore to edit")
	}
	path := common.KeystorePath(core.ServerSettings.Keystore, core.ConfigFile)
	if path == "" {
		commands.Fail("user: no keystore is configured, so there is nowhere to keep a user.\n"+
			"Add this under server: in %s and run this again:\n\n    keystore: \"orgs_keystore.yaml\"\n",
			core.ConfigFile)
	}

	ks, existed := openKeystore(path, core.ServerSettings.OrgSalt)

	switch sub {
	case "ls", "list":
		self.list(ks)
	case "add":
		self.add(ks, path, existed, words)
	case "passwd", "password":
		self.passwd(ks, path, words)
	case "rm", "remove", "del", "delete":
		self.remove(ks, path, words)
	case "admin":
		self.setAdmin(ks, path, words, true)
	case "noadmin":
		self.setAdmin(ks, path, words, false)
	default:
		commands.Fail("user: unknown subcommand %q. One of: ls, add, passwd, rm, admin, noadmin", sub)
	}
}

// openKeystore reads the keystore, or hands back an empty one at that path for
// `add` to write. A keystore that is not there yet is the ordinary state of a
// server nobody has added a user to.
func openKeystore(path, orgSalt string) (*common.YamlKeystore, bool) {
	ks, err := common.LoadKeystore(path, orgSalt)
	if err == nil {
		return ks, true
	}
	if !os.IsNotExist(err) {
		commands.Fail("user: %v", err)
	}
	return common.NewKeystore(path, orgSalt), false
}

// The three states the server's startup warning is about, worst first.
func passwordState(ks *common.YamlKeystore, name string, isDefault map[string]bool) string {
	switch {
	case isDefault[name]:
		return "THE DEFAULT PASSWORD"
	case !common.IsStoredHash(ks.Creds[name].Password):
		return "cleartext"
	default:
		return "hashed"
	}
}

func (self *UserCmd) list(ks *common.YamlKeystore) {
	names := ks.Users()
	isDefault := map[string]bool{}
	for _, n := range ks.DefaultCredentialUsers() {
		isDefault[n] = true
	}
	rows := make([]userRow, 0, len(names))
	for _, n := range names {
		last := "never"
		if t, ok := ks.LastLogin(n); ok {
			last = t.Local().Format(time.RFC822)
		}
		rows = append(rows, userRow{Name: n, Admin: ks.Creds[n].Admin,
			Password: passwordState(ks, n, isDefault), LastLogin: last})
	}
	commands.Render(rows, func() {
		if len(rows) == 0 {
			fmt.Println("No users. `orgs user add <name> -admin` adds one.")
			fmt.Println("Until there is one, the server falls back to its built-in admin/default account.")
			return
		}
		w := 4
		for _, r := range rows {
			if len(r.Name) > w {
				w = len(r.Name)
			}
		}
		fmt.Printf("%s%-*s  %-5s  %-20s  %s%s\n", commands.C(commands.AnsiBold), w, "USER", "ADMIN", "PASSWORD", "LAST LOGIN", commands.C(commands.AnsiReset))
		for _, r := range rows {
			colour := ""
			switch r.Password {
			case "THE DEFAULT PASSWORD":
				colour = commands.C(commands.AnsiRed)
			case "cleartext":
				colour = commands.C(commands.AnsiGold)
			}
			// An admin with the default password is the worst row this listing
			// can have, so the two states are shown side by side rather than
			// one of them being folded into the other.
			admin := ""
			if r.Admin {
				admin = "yes"
			}
			fmt.Printf("%-*s  %-5s  %s%-20s%s  %s\n", w, r.Name, admin, colour, r.Password, commands.C(commands.AnsiReset), r.LastLogin)
		}
	})
}

func (self *UserCmd) add(ks *common.YamlKeystore, path string, existed bool, words []string) {
	name := oneName("add", words)
	if _, taken := ks.Creds[name]; taken {
		commands.Fail("user add: %q already exists. `orgs user passwd %s` changes the password.", name, name)
	}
	pass := commands.ReadNewPassword(fmt.Sprintf("Password for %s", name), self.Password)
	if err := ks.SetPassword(name, pass); err != nil {
		commands.Fail("user add: %v", err)
	}
	if self.Admin {
		ks.SetAdmin(name, true)
	}
	what := "add"
	if self.Admin {
		what = "add as an administrator"
	}
	if commands.Wrote(fmt.Sprintf("%s %q to %s", what, name, path), nil) {
		return
	}
	save(ks, path)
	if self.Admin {
		fmt.Printf("Added %s to %s as an administrator\n", name, path)
	} else {
		fmt.Printf("Added %s to %s\n", name, path)
	}
	if !existed {
		// The point at which the built-in account stops existing, which is the
		// whole reason somebody ran this - and a bad surprise if they were
		// relying on admin/default for anything.
		fmt.Printf("\nThat is a new keystore, so the built-in %s/%s account is now gone.\n",
			common.DefaultUsername, common.DefaultPassword)
		fmt.Printf("Log in as %s from here on.\n", name)
	}
	fmt.Println("\nRestart the server to pick this up.")
}

func (self *UserCmd) passwd(ks *common.YamlKeystore, path string, words []string) {
	name := oneName("passwd", words)
	if _, ok := ks.Creds[name]; !ok {
		commands.Fail("user passwd: there is no user %q in %s", name, path)
	}
	pass := commands.ReadNewPassword(fmt.Sprintf("New password for %s", name), self.Password)
	if err := ks.SetPassword(name, pass); err != nil {
		commands.Fail("user passwd: %v", err)
	}
	if commands.Wrote(fmt.Sprintf("change the password for %q in %s", name, path), nil) {
		return
	}
	save(ks, path)
	fmt.Printf("Changed the password for %s\n", name)
	fmt.Println("Restart the server to pick this up.")
}

func (self *UserCmd) remove(ks *common.YamlKeystore, path string, words []string) {
	name := oneName("rm", words)
	if _, ok := ks.Creds[name]; !ok {
		commands.Fail("user rm: there is no user %q in %s", name, path)
	}
	// Removing the last user does not lock anybody out - the server falls back
	// to its built-in account - which is worse than being locked out and is
	// worth saying before it happens rather than after.
	last := len(ks.Creds) == 1
	if !self.Force {
		if !commands.Interactive() {
			commands.Fail("user rm: refusing to remove %q without -force, with nobody here to ask", name)
		}
		q := fmt.Sprintf("Remove %s from %s?", name, path)
		if last {
			q = fmt.Sprintf("%s is the only user. Removing them leaves the server on its\nbuilt-in %s/%s account. Remove them anyway?",
				name, common.DefaultUsername, common.DefaultPassword)
		}
		if !confirm(q) {
			fmt.Println("Left alone.")
			return
		}
	}
	ks.Remove(name)
	if commands.Wrote(fmt.Sprintf("remove %q from %s", name, path), nil) {
		return
	}
	save(ks, path)
	fmt.Printf("Removed %s\n", name)
	if last {
		fmt.Printf("\nThere are no users left, so the server will fall back to %s/%s.\n",
			common.DefaultUsername, common.DefaultPassword)
	}
	fmt.Println("Restart the server to pick this up.")
}

// setAdmin grants or removes the administrator flag.
//
// This is the one thing `orgs adduser` cannot do for itself, and deliberately:
// an administrator who could promote an account over the wire could promote
// one they had just added, so being an administrator would be one call away
// from being granted rather than something somebody decided at the server.
func (self *UserCmd) setAdmin(ks *common.YamlKeystore, path string, words []string, admin bool) {
	sub := "admin"
	if !admin {
		sub = "noadmin"
	}
	name := oneName(sub, words)
	if _, ok := ks.Creds[name]; !ok {
		commands.Fail("user %s: there is no user %q in %s", sub, name, path)
	}
	if ks.Creds[name].Admin == admin {
		if admin {
			fmt.Printf("%s is already an administrator.\n", name)
		} else {
			fmt.Printf("%s is not an administrator.\n", name)
		}
		return
	}
	// Removing the last administrator leaves nobody who can run `orgs adduser`
	// at all. It is recoverable - this command is right here - but only on this
	// machine, which is worth saying before it happens.
	if !admin && len(ks.Admins()) == 1 {
		fmt.Printf("%s is the only administrator. `orgs adduser` will refuse for everybody\n", name)
		fmt.Printf("until somebody is made one again here.\n")
	}
	ks.SetAdmin(name, admin)
	if commands.Wrote(fmt.Sprintf("%s %q in %s", sub, name, path), nil) {
		return
	}
	save(ks, path)
	if admin {
		fmt.Printf("%s is now an administrator and may run orgs adduser\n", name)
	} else {
		fmt.Printf("%s is no longer an administrator\n", name)
	}
	fmt.Println("Restart the server to pick this up.")
}

func oneName(sub string, words []string) string {
	switch len(words) {
	case 0:
		commands.Fail("user %s: which user? `orgs user %s <name>`", sub, sub)
	case 1:
		return words[0]
	}
	commands.Fail("user %s: one user at a time, got %d", sub, len(words))
	return ""
}

func save(ks *common.YamlKeystore, path string) {
	if err := ks.Save(); err != nil {
		commands.Fail("user: could not write %s: %v", path, err)
	}
}

func confirm(question string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", question)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// init function is called at boot
func init() {
	commands.AddCmd("user", "add, list, remove, re-password and promote the users that may log in",
		func() commands.Cmd {
			return &UserCmd{}
		})
}
