package orgs

// The server's half of the credential store. The store itself is
// internal/common/keystore.go, because `orgs user` writes the same file and a
// CLI command package cannot import this one.
//
// Two things happen here that cannot happen there: the keystore path is
// resolved out of the configuration, and the server says out loud on startup
// when the credentials it just loaded are not ones anybody should be running
// on.

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// Used for login.
type Credentials struct {
	Username string `json:"username"`
	// Either a client hash (common.ClientHash, which is what every current
	// client sends) or, from a client older than that, the password itself.
	// See the note on requireHashedLogin.
	Password string `json:"password"`
}

type KeyStore = common.KeyStore

func GetKeystore() KeyStore {
	return currentKeystore
}

// DefaultKeystore gives the process a keystore before the configuration has
// been read.
//
// It is called ahead of Conf() on purpose - the config load can fail, and a
// nil keystore turns that into a panic rather than a message - so it cannot
// look anything up. On the server, LoadKeystore replaces what this sets as
// soon as there is a configuration to read it out of.
func DefaultKeystore() {
	currentKeystore = common.DefaultKeystore("")
}

// LoadKeystore reads the keystore named by `keystore:` in the server config.
//
// Falling back to the built-in default is deliberate on both of the paths that
// do it: a server that refused to start without a keystore could not be set up
// in the first place, since `orgs user add` needs somewhere to write and the
// warning that follows is how somebody finds out they need to. What must not
// happen is doing it quietly.
func LoadKeystore() {
	salt := ""
	keystore := ""
	if Conf().Server != nil {
		salt = Conf().Server.OrgSalt
		keystore = Conf().Server.Keystore
	}
	path := common.KeystorePath(keystore, Conf().Config)
	if path == "" {
		currentKeystore = common.DefaultKeystore(salt)
		keystoreState = "no keystore configured"
		return
	}
	ks, err := common.LoadKeystore(path, salt)
	if err != nil {
		if os.IsNotExist(err) {
			keystoreState = fmt.Sprintf("keystore %s does not exist yet", path)
		} else {
			keystoreState = fmt.Sprintf("keystore %s could not be read: %v", path, err)
			fmt.Fprintf(os.Stderr, "Keystore: %v\n", err)
		}
		currentKeystore = common.DefaultKeystore(salt)
		return
	}
	if len(ks.Creds) == 0 {
		// A keystore with nobody in it would lock the server out of itself.
		keystoreState = fmt.Sprintf("keystore %s has no users in it", path)
		currentKeystore = common.DefaultKeystore(salt)
		return
	}
	currentKeystore = ks
	keystoreState = ""
	fmt.Fprintf(os.Stderr, "Keystore: loaded from %s (%d users)\n", path, len(ks.Creds))
}

// Why the keystore in force is not the one the configuration asked for, or
// empty when it is. Part of the startup warning rather than a thing to query.
var keystoreState string

var currentKeystore KeyStore = nil

// WarnOnInsecureCredentials says, unmissably, that this server can be logged
// into by anybody who has read its documentation.
//
// It is a block rather than a line because the thing it is competing with is
// three hundred lines of plugin and watcher chatter scrolling past on startup,
// and the state it describes - a server on a network with a published password
// - is one somebody needs to find out about from their own logs rather than
// from somebody else.
func WarnOnInsecureCredentials() {
	ks := GetKeystore()
	if ks == nil {
		return
	}
	dflt := ks.DefaultCredentialUsers()
	plain := ks.PlaintextUsers()
	noAuth := Conf().Server != nil && Conf().Server.NoAuth

	if len(dflt) == 0 && len(plain) == 0 && !noAuth {
		return
	}

	var lines []string
	if len(dflt) > 0 {
		who := "The account " + quoteList(dflt) + " can be logged into"
		if len(dflt) > 1 {
			who = "The accounts " + quoteList(dflt) + " can be logged into"
		}
		lines = append(lines,
			fmt.Sprintf("%s with the password %q.", who, common.DefaultPassword),
			"Anybody who can reach this server can read and write your org files.",
			"")
	}
	if keystoreState != "" {
		lines = append(lines, "Why: "+keystoreState+".", "")
	}
	if len(plain) > 0 {
		lines = append(lines,
			"Stored as cleartext, not hashed: "+quoteList(plain)+".",
			"")
	}
	if noAuth {
		lines = append(lines,
			"Authentication is switched off entirely (noAuth: true), so no",
			"password is asked for on any request regardless of the above.",
			"")
	}

	lines = append(lines, "To fix it:")
	if Conf().Server == nil || Conf().Server.Keystore == "" {
		lines = append(lines,
			"  1. add  keystore: \"orgs_keystore.yaml\"  under server: in",
			"     "+Conf().Config,
			"  2. run  orgs user add <name>",
			"  3. restart the server")
	} else {
		lines = append(lines,
			"  1. run  orgs user add <name>      to add an account",
			"  2. run  orgs user passwd <name>   to replace a default or",
			"                                    cleartext password",
			"  3. restart the server")
	}
	lines = append(lines,
		"",
		"orgs user ls  lists what this server will accept.")

	log.Printf("%s", warnBlock("INSECURE CREDENTIALS", lines))
}

func quoteList(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = fmt.Sprintf("%q", n)
	}
	switch len(q) {
	case 1:
		return q[0]
	case 2:
		return q[0] + " and " + q[1]
	default:
		return strings.Join(q[:len(q)-1], ", ") + " and " + q[len(q)-1]
	}
}

const warnWidth = 72

// warnBlock draws a block that cannot be mistaken for a log line.
func warnBlock(title string, lines []string) string {
	bar := strings.Repeat("!", warnWidth+6)
	var b strings.Builder
	b.WriteString("\n" + bar + "\n")
	row := func(s string) {
		pad := warnWidth - len(s)
		if pad < 0 {
			pad = 0
		}
		b.WriteString("!! " + s + strings.Repeat(" ", pad) + " !!\n")
	}
	row("")
	row(centre(title))
	row("")
	for _, l := range lines {
		for _, w := range wrapWarn(l, warnWidth) {
			row(w)
		}
	}
	row("")
	b.WriteString(bar + "\n")
	return b.String()
}

func centre(s string) string {
	if len(s) >= warnWidth {
		return s
	}
	return strings.Repeat(" ", (warnWidth-len(s))/2) + s
}

// wrapWarn wraps on words, and leaves a word longer than the box alone rather
// than cutting it - the long ones are paths and commands, which are worth more
// intact than the right hand edge of a box is.
func wrapWarn(s string, width int) []string {
	if s == "" {
		return []string{""}
	}
	// A line that was laid out by hand (indented, a command in it) is left as
	// it is: re-wrapping it would undo the layout.
	if strings.HasPrefix(s, " ") || len(s) <= width {
		return []string{s}
	}
	var out []string
	cur := ""
	for _, w := range strings.Fields(s) {
		switch {
		case cur == "":
			cur = w
		case len(cur)+1+len(w) <= width:
			cur += " " + w
		default:
			out = append(out, cur)
			cur = w
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
