package orgs

import (
	"net/http"
	"os"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs/dnd"
	dndc "github.com/ihdavids/orgs/internal/common/dnd"
)

/*
		SDOC: API

	  - POST /dnd/levelup — Level A Character Up
	    Walks a character already on disk up one or more levels, asking only what the
	    new levels actually ask them to decide, and writes the answers back into the
	    same org file.

	    The flow is stateless. Each call sends every answer given so far, the engine
	    replays them, and the response carries either the next question or the result.
	    So there is no session to expire, stopping halfway costs nothing, and the same
	    answers always produce the same character.

	    *Method:* =POST=

	    *Body:*
	    | Field      | Type   | Description                                                     |
	    |------------+--------+-----------------------------------------------------------------|
	    | =filename= | string | The character's org sheet. Required.                            |
	    | =levels=   | int    | How many levels to gain. Defaults to 1.                         |
	    | =class=    | string | Which class gains them. Defaults to the primary class; a class the character does not have is a multiclass and is added. |
	    | =answers=  | object | Step id to the values chosen for it, everything answered so far. |
	    | =seed=     | int    | Makes any dice the flow rolls repeatable. Send back what the response gave you. |
	    | =commit=   | bool   | Write the file. Without it nothing is written and the call only reports what would be asked, which is how a client checks a climb before starting it. |

	    *Response:* A =LevelUpPlan=. =next= is the question still to be answered, in the
	    same shape the character builder's prompts use, so a client can render it the same
	    way. =done= is true when there is nothing left to ask; only then does =commit=
	    write anything.

	    A level up never touches equipment, coin, notes or backstory - only the property
	    drawer and the sections the rules engine regenerates.
	    EDOC
*/
func PostDndLevelUp(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dndLevelUpRequest
	if !dndBody(w, r, &req) {
		return
	}
	if req.Filename == "" {
		dndError(w, http.StatusBadRequest, "filename is required")
		return
	}
	c, rs, err := dnd.LoadCharacter(db, req.Filename)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	plan, levelled, err := dndc.LevelUp(c, rs, &req.LevelUpRequest)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	// Nothing is written until every question has an answer, and not even
	// then unless the caller says to. A client walking the questions is
	// levelling a copy each time round; only the last call commits.
	if !plan.Done || !req.Commit {
		dndJson(w, plan)
		return
	}
	fname := req.Filename
	if f := GetDb().GetFile(req.Filename); f != nil && f.Filename != "" {
		fname = f.Filename
	}
	org := dndc.RenderOrg(levelled, rs)
	if err := os.WriteFile(fname, []byte(org), 0644); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", fname, err)
		return
	}
	GetDb().ReloadFile(fname)
	plan.Character = levelled.Name
	dndJson(w, plan)
}

// dndLevelUpRequest is the wire form: the engine's request plus the one thing
// only the server cares about, whether to write the file.
type dndLevelUpRequest struct {
	dndc.LevelUpRequest
	Commit bool `json:"commit"`
}
