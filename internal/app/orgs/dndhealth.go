//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Hit points
//
// Damage, healing and temporary hit points, written straight into the org
// character sheet with a line added to its Health History section. A rest goes
// through /dnd/rest instead - it moves too much else at once - but everything
// that happens between rests comes through here.
// ----------------------------------------------------------------------------

import (
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// dndHealthState is what every health call answers with.
func dndHealthState(c *dnd.Character, rs *dnd.Ruleset, path, msg string) dnd.HealthState {
	history := c.HealthLog
	if history == nil {
		history = []dnd.HealthEvent{}
	}
	id := ""
	if rs != nil {
		id = rs.Id
	}
	return dnd.HealthState{
		Id: dnd.CharacterId(c), Name: c.Name, Filename: path, Ruleset: id,
		HP: dnd.ComputeHealth(dnd.Compute(c, rs)), History: history, Msg: msg,
	}
}

/* SDOC: API
* GET /dnd/hp — Where A Character Stands
	Returns the hit point line: current, maximum, the temporary hit points in front
	of them, how full the bar is, and the sheet's Health History.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                      |
	|------------+--------+----------+--------------------------------------------------|
	| =filename= | string | no       | The org character sheet (basename or path).      |
	| =id=       | string | no       | The character's =DND_ID=, used when no filename. |

	One of the two is required.

	*Response:* A =HealthState=:
	#+BEGIN_SRC json
	{
	  "id": "lyra-silverleaf-4c1f2a", "name": "Lyra", "filename": "/gtd/lyra.org",
	  "hp": {"current": 14, "max": 27, "temp": 5, "percent": 51, "tempPercent": 18,
	         "down": false, "bloodied": true, "deathSaves": ""},
	  "history": [{"date": "2025-09-05", "time": "21:40", "action": "hurt",
	               "amount": 9, "absorbed": 5, "hpBefore": 18, "hpAfter": 14,
	               "tempBefore": 5, "tempAfter": 0, "hpMax": 27,
	               "notes": "wraith's touch"}]
	}
	#+END_SRC
	EDOC */
func RequestDndHealth(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	filename := r.URL.Query().Get("filename")
	id := r.URL.Query().Get("id")
	if strings.TrimSpace(filename) == "" && strings.TrimSpace(id) == "" {
		dndError(w, http.StatusBadRequest, "pass a filename or a character id")
		return
	}
	dndInvLock.Lock()
	defer dndInvLock.Unlock()
	c, rs, path, err := dndFindCharacter(filename, id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, dndHealthState(c, rs, path, ""))
}

/* SDOC: API
* POST /dnd/hp — Hurt Or Heal A Character
	Applies one change to a character's hit points and writes the sheet back,
	adding a line to its Health History section.

	*Method:* =POST=

	*Body:* =HealthRequest=
	| Field      | Type   | Required | Description                                                 |
	|------------+--------+----------+-------------------------------------------------------------|
	| =filename= | string | no       | The org character sheet. One of filename or id is required. |
	| =id=       | string | no       | The character's =DND_ID=.                                   |
	| =action=   | string | yes      | =hurt=, =heal=, =temp=, =cleartemp= or =set=.               |
	| =amount=   | number | no       | How many hit points. Never negative.                        |
	| =notes=    | string | no       | What did it, kept in the history.                           |

	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "action": "hurt", "amount": 9,
	 "notes": "wraith's touch"}
	#+END_SRC

	*Response:* The =HealthState= after the change, with =msg= saying what it did
	in one line.

	Damage comes off the temporary hit points first and only then off the real
	ones, which is the order the rules put them in, and the answer says how much
	was soaked up. Healing never touches the temporary hit points - they are a
	buffer, not hit points that can be restored - and never carries a character
	past their maximum. Healing someone off zero clears their death saves.
	=temp= sets the temporary hit points outright rather than adding to them,
	because temporary hit points do not stack: gaining them while you already
	have some is a choice between the two, not a sum.
	EDOC */
func PostDndHealth(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.HealthRequest
	if !dndBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Filename) == "" && strings.TrimSpace(req.Id) == "" {
		dndError(w, http.StatusBadRequest, "pass a filename or a character id")
		return
	}
	dndInvLock.Lock()
	defer dndInvLock.Unlock()
	c, rs, path, err := dndFindCharacter(req.Filename, req.Id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	event, err := dnd.ApplyHealth(c, rs, req)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if err := dndWriteCharacter(c, rs, path); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
		return
	}
	dndJson(w, dndHealthState(c, rs, path, dnd.HealthEventMsg(event)))
}
