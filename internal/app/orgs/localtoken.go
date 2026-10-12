package orgs

/* SDOC: Configuration
* The command line and a server on this machine: no login
  A token is only the server's keys applied to a user name, so a command line
  that has read the server's own configuration - =orgJWS=, =orgJWE= and
  =orgSalt= - can make one itself rather than ask for a password. It does when
  it has no token of its own that is still good and =url:= is on this machine
  (=localhost=, =127.0.0.1=, =::1=).

  This is how the server VS Code sets up is used from a terminal: its binary
  reads the =orgs.yaml= beside it, which is that server's, so =orgs agenda= just
  works. Anybody who can read that file could make tokens anyway - it is the
  password file's equal, and written readable by its owner only.
EDOC */

import (
	"net"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// LocalToken makes a token for the server this configuration describes, when
// it is on this machine and the configuration holds its keys. ok is false when
// it cannot or should not: no keys, a server elsewhere, or one with no login.
func LocalToken() (token string, expiresAt time.Time, ok bool) {
	c := Conf()
	s := c.Server
	if s == nil || s.NoAuth || s.OrgJWS == "" || s.OrgJWE == "" || s.OrgSalt == "" || !isLoopback(c.Url) {
		return "", time.Time{}, false
	}
	token, expiresAt, err := GenerateEncryptedToken(localUser())
	if err != nil {
		return "", time.Time{}, false
	}
	return token, expiresAt, true
}

func isLoopback(u string) bool {
	p, err := url.Parse(u)
	if err != nil {
		return false
	}
	host := p.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Who the token is for: the keystore's administrator (the VS Code plugin's
// "vscode"), so saved queries and boards are the same ones the editor sees.
// Then any account in it, then the user noAuth runs as.
func localUser() string {
	path := common.KeystorePath(Conf().Server.Keystore, Conf().Config)
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if ks, err := common.LoadKeystore(path, Conf().Server.OrgSalt); err == nil && len(ks.Creds) > 0 {
				names := make([]string, 0, len(ks.Creds))
				for n := range ks.Creds {
					names = append(names, n)
				}
				sort.Strings(names)
				for _, n := range names {
					if ks.Creds[n].Admin {
						return n
					}
				}
				return names[0]
			}
		}
	}
	return NoAuthUsername
}
