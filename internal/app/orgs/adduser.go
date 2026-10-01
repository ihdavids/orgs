package orgs

// Adding a user over the wire.
//
// `orgs user` is the other way in, and the comment at the top of it explains
// why it edits the keystore file directly: the keystore decides who may talk
// to this server at all, so an endpoint that wrote it is an endpoint that
// grants access, reachable by anybody who already has some.
//
// That is exactly what this is, and the answer to it is that "anybody who
// already has some" is not enough here. An account has to be marked
// `admin: true` in the keystore before it can add another, and the only
// account that starts out that way is the built-in one - so the ability to
// add users is something somebody granted on the server's own disk, once,
// with `orgs user`. Every account this endpoint creates is one an
// administrator asked for by name.
//
// Three more things it will not do:
//
//  1. It will not run with `noAuth: true`. There is no authenticated user to
//     be an administrator, and an account written then would outlive the
//     setting - somebody turning authentication back on would be turning it
//     on over a login an anonymous caller had left behind.
//  2. It will not run on the built-in keystore, which has no file behind it:
//     Save is a no-op there, so the account would work until the next restart
//     and then be gone. Saying so beats appearing to work.
//  3. It will not replace an existing account. An "add" that quietly reset
//     somebody's password would be a way to take their account over.
//
// The password does not travel, the same as it does not for a login. The
// client makes a salt for the new user and sends common.ClientHash of the
// password under it; the server hashes that again with its own orgSalt before
// it reaches the file.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/ihdavids/orgs/internal/common"
)

// writable is the keystore in force, when it is one that can be written to.
func writableKeystore() (*common.YamlKeystore, error) {
	ks, ok := GetKeystore().(*common.YamlKeystore)
	if !ok || ks == nil {
		return nil, fmt.Errorf("this server has no keystore to add a user to")
	}
	if !ks.Writable() {
		return nil, fmt.Errorf("this server is running on its built-in %s account, which has no keystore file behind it. "+
			"Set  keystore: \"orgs_keystore.yaml\"  under server: in the config, run  orgs user add <name> -admin  on the "+
			"server's own machine, and restart", common.DefaultUsername)
	}
	return ks, nil
}

// mayAddUsers answers whether the user this request authenticated as may add
// an account, and says why not when they may not.
func mayAddUsers(who string) error {
	if Conf().Server != nil && Conf().Server.NoAuth {
		return fmt.Errorf("this server has authentication switched off (noAuth: true), so there is no " +
			"administrator to be. Add the user with  orgs user add <name>  on the server's own machine")
	}
	if who == "" {
		return fmt.Errorf("could not tell who is asking")
	}
	if !GetKeystore().IsAdmin(who) {
		return fmt.Errorf("%q is not an administrator of this server. Somebody with access to the server's "+
			"own machine can grant it with  orgs user admin %s  followed by a restart", who, who)
	}
	return nil
}

func userJson(w http.ResponseWriter, status int, res common.ResultMsg) {
	AccessControl(&w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(res)
}

/* SDOC: API
* POST /users/add — Add A User
	Adds an account to this server's keystore. The caller must be logged in as
	an account marked =admin: true=; the new account may be marked one too.

	The account works immediately - the server writes the keystore it is
	already holding, so unlike =orgs user add= there is nothing to restart.

	*Method:* =POST=

	*Body:* A =NewUser=. The password is not in it: the client generates a salt
	for the new user and sends =ClientHash(username, salt, password)= under it,
	exactly as a login does, so the password never reaches the server.

	| Field      | Type    | Required | Description                                       |
	|------------+---------+----------+---------------------------------------------------|
	| =username= | string  | yes      | The new account's name. No whitespace, no  =:/?#[]@= |
	| =salt=     | string  | yes      | A fresh salt the client generated for them.       |
	| =password= | string  | yes      | =ClientHash(username, salt, password)=.           |
	| =admin=    | bool    | no       | Whether the new account may add users itself.     |

	#+BEGIN_SRC json
	{"username": "dana", "salt": "b3d1...", "password": "sha256c:9f2c...", "admin": false}
	#+END_SRC

	*Response:* A =ResultMsg=. It is refused, with the reason in =msg=, when the
	caller is not an administrator (403), when authentication is switched off or
	this server has no keystore file to write (409), and when the name is not a
	usable one or is already taken (400).
	EDOC */
func PostAddUser(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var nu common.NewUser
	if err := json.Unmarshal(body, &nu); err != nil {
		userJson(w, http.StatusBadRequest, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}

	who := GetUsername(r)
	if err := mayAddUsers(who); err != nil {
		// Said out loud. Somebody trying to add themselves an account is worth
		// a line in the log whether or not they managed it.
		fmt.Fprintf(os.Stderr, "Refused to add the user %q for %q: %v\n", nu.Username, who, err)
		userJson(w, refusalStatus(), common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}

	ks, err := writableKeystore()
	if err != nil {
		userJson(w, http.StatusConflict, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}

	if err := common.ValidUsername(nu.Username); err != nil {
		userJson(w, http.StatusBadRequest, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if !common.IsClientHash(nu.Password) {
		// Never a cleartext password, however old the client is. A login has to
		// take one because refusing would lock out clients that predate the
		// hashing; nothing predates this endpoint.
		userJson(w, http.StatusBadRequest, common.ResultMsg{Ok: false,
			Msg: "the password has to arrive hashed (common.ClientHash); this client sent something else"})
		return
	}

	if err := ks.AddUser(nu.Username, nu.Salt, nu.Password, nu.Admin); err != nil {
		userJson(w, http.StatusBadRequest, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}

	what := "user"
	if nu.Admin {
		what = "administrator"
	}
	fmt.Fprintf(os.Stderr, "Keystore: %q added the %s %q\n", who, what, nu.Username)
	userJson(w, http.StatusOK, common.ResultMsg{Ok: true,
		Msg: fmt.Sprintf("added the %s %q; they can log in now", what, nu.Username)})
}

// refusalStatus tells the two refusals apart: "you may not" is the caller's
// problem and "this server cannot" is not.
func refusalStatus() int {
	if Conf().Server != nil && Conf().Server.NoAuth {
		return http.StatusConflict
	}
	return http.StatusForbidden
}
