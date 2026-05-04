package login

// Login authenticates with the orgs server using a username and password,
// stores the token in the local config file, and prints it to stdout.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

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
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"ExpiresAt"`
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
	if err := saveTokenToConfig(core.ConfigFile, resp.Token, resp.ExpiresAt); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to save token to config: %s\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Token saved to %s (expires %s)\n", core.ConfigFile, resp.ExpiresAt.Local().Format(time.RFC822))
	}
}

func saveTokenToConfig(configFile string, token string, expiresAt time.Time) error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("reading config %s: %w", configFile, err)
	}
	lines := strings.Split(string(data), "\n")
	setLine := func(key, value string) {
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), key+":") {
				lines[i] = fmt.Sprintf("%s: %s", key, value)
				return
			}
		}
		lines = append(lines, fmt.Sprintf("%s: %s", key, value))
	}
	setLine("token", fmt.Sprintf("%q", token))
	setLine("tokenExpiry", fmt.Sprintf("%q", expiresAt.Format(time.RFC3339)))
	return os.WriteFile(configFile, []byte(strings.Join(lines, "\n")), 0600)
}

// init function is called at boot
func init() {
	commands.AddCmd("login", "authenticate with the orgs server and print the generated token",
		func() commands.Cmd {
			return &LoginCmd{}
		})
}
