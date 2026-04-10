package login

// Login authenticates with the orgs server using a username and password
// and prints the generated JWT token to the command line.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type LoginCmd struct {
	Username string
	Password string
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

func (self *LoginCmd) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *LoginCmd) StartPlugin(manager *common.PluginManager) {
}

func (self *LoginCmd) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Username, "user", "", "username for the orgs server")
	fset.StringVar(&self.Password, "password", "", "password for the orgs server (prompted if omitted)")
}

func promptUsername() string {
	fmt.Fprint(os.Stderr, "Username: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}

func promptPassword() string {
	fmt.Fprint(os.Stderr, "Password: ")
	pw, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return ""
	}
	return string(pw)
}

func (self *LoginCmd) Exec(core *commands.Core) {
	user := self.Username
	pass := self.Password
	if user == "" {
		user = promptUsername()
	}
	if pass == "" {
		pass = promptPassword()
	}
	if user == "" || pass == "" {
		fmt.Fprintln(os.Stderr, "login: username and password are required")
		os.Exit(1)
	}

	req := loginRequest{Username: user, Password: pass}
	resp, err := common.RestPost[loginResponse](&core.Rest, "login", &req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "login failed: %s\n", err)
		os.Exit(1)
	}
	if resp.Token == "" {
		fmt.Fprintln(os.Stderr, "login failed: server returned no token (check credentials)")
		os.Exit(1)
	}
	fmt.Println(resp.Token)
}

// init function is called at boot
func init() {
	commands.AddCmd("login", "authenticate with the orgs server and print the generated token",
		func() commands.Cmd {
			return &LoginCmd{}
		})
}
