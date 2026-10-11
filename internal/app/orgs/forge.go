package orgs

// The github and gitlab pollers (plugs/github) from outside: poll now rather
// than at the next tick, and say whether the token works. Both answer with one
// result per configured poller, since a server can follow several.

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

func registerForgeRoutes(api *mux.Router) {
	api.HandleFunc("/forge/sync", PostForgeSync).Methods("POST")
	api.HandleFunc("/forge/check", RequestForgeCheck).Methods("GET")
}

// What a forge poller offers besides polling. Asked for by method rather than
// by type, so this file does not have to know the plugin.
type forgePoller interface {
	Sync(db common.ODb) common.ForgeSync
	Check() common.ForgeSync
}

func forgePollers() []forgePoller {
	var out []forgePoller
	if Conf().Server == nil {
		return out
	}
	for _, pd := range Conf().Server.Plugins {
		if f, ok := pd.Plugin.(forgePoller); ok {
			out = append(out, f)
		}
	}
	return out
}

func answerForge(w http.ResponseWriter, each func(forgePoller) common.ForgeSync) {
	reply := common.ForgeReply{Ok: true}
	for _, f := range forgePollers() {
		res := each(f)
		reply.Ok = reply.Ok && res.Ok
		reply.Results = append(reply.Results, res)
	}
	if len(reply.Results) == 0 {
		reply.Ok = false
		reply.Msg = "no github or gitlab poller in server.plugins"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

/* SDOC: API
* POST /forge/sync — Poll GitHub and GitLab Now

	Runs every =github= and =gitlab= poller in =server.plugins= once, now,
	instead of waiting for its next tick: new items written, changed ones
	updated, headings marked DONE closed upstream. =orgs forge sync= calls it.

	*Method:* =POST=, no body.

	*Response:* A =ForgeReply= ={Ok, Msg, Results}=, a =ForgeSync= per poller:
	| Field      | Meaning                                                     |
	|------------+-------------------------------------------------------------|
	| =Name=     | =github= or =gitlab=                                        |
	| =File=, =Url= | The org file it writes and the api it reads              |
	| =Ok=, =Msg= | Whether the poll worked, and why not (a rate limit, a bad token) |
	| =Added=, =Updated= | Headings written for new items; headings changed   |
	| =Closed=, =Reopened=, =Gone= | Keyword changes that came from upstream |
	| =Pushed=   | Closes and reopens sent upstream                            |
	| =Notes=    | A line per thing worth telling a person                     |
EDOC */
func PostForgeSync(w http.ResponseWriter, r *http.Request) {
	answerForge(w, func(f forgePoller) common.ForgeSync { return f.Sync(db) })
}

/* SDOC: API
* GET /forge/check — Whether the GitHub and GitLab Tokens Work

	Asks each =github= and =gitlab= poller's api who its token belongs to,
	writing nothing. =orgs forge check= calls it.

	*Method:* =GET=

	*Response:* A =ForgeReply= as for =POST /forge/sync=, each result's =User=
	the login the token belongs to, or =Ok= false and the api's refusal in =Msg=.
EDOC */
func RequestForgeCheck(w http.ResponseWriter, r *http.Request) {
	answerForge(w, func(f forgePoller) common.ForgeSync { return f.Check() })
}
