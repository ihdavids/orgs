//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Conditions and defenses
//
// The defenses tab on the html character sheet: what a character shrugs off,
// and what is currently wrong with them. Both are written straight into the
// character's own org file the way coin and inventory are, and a line is added
// to its Condition History section, so the file stays the one account of what
// state a character is in between one play session and the next.
// ----------------------------------------------------------------------------

import (
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// dndConditionState is what every conditions call answers with: everything the
// tab draws, so it can be redrawn from one round trip.
func dndConditionState(c *dnd.Character, rs *dnd.Ruleset, path, msg string) dnd.ConditionsState {
	history := c.ConditionLog
	if history == nil {
		history = []dnd.ConditionEvent{}
	}
	id := ""
	if rs != nil {
		id = rs.Id
	}
	return dnd.ConditionsState{
		Id: dnd.CharacterId(c), Name: c.Name, Filename: path, Ruleset: id,
		Conditions:  dnd.ComputeConditions(c, rs),
		Defenses:    dnd.ComputeDefenses(c, rs),
		DamageTypes: rs.DamageTypeList(),
		History:     history, Msg: msg,
	}
}

/* SDOC: API
* GET /dnd/conditions — What A Character Is Under, And What They Shrug Off
	Returns the conditions currently on a character, every condition they could be
	under, the damage they take less, none or more of, and the sheet's Condition
	History. Nothing is changed - this is what the character sheet's defenses tab
	is drawn from, including the condition picker.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                      |
	|------------+--------+----------+--------------------------------------------------|
	| =filename= | string | no       | The org character sheet (basename or path).      |
	| =id=       | string | no       | The character's =DND_ID=, used when no filename. |

	One of the two is required.

	*Response:* A =ConditionsState=:
	#+BEGIN_SRC json
	{
	  "id": "lyra-silverleaf-4c1f2a", "name": "Lyra", "filename": "/gtd/lyra.org",
	  "conditions": {
	    "active": [{"id": "frightened", "name": "Frightened", "on": true,
	                "label": "Frightened"},
	               {"id": "exhaustion", "name": "Exhaustion", "on": true, "level": 2,
	                "levels": 6, "label": "Exhaustion 2", "note": "Speed halved"}],
	    "all": [{"id": "blinded", "name": "Blinded", "icon": "blinded", "on": false,
	             "text": "A blinded creature can't see..."}],
	    "count": 2, "summary": "Frightened, Exhaustion 2"
	  },
	  "defenses": {
	    "resistances": [{"id": "fire", "name": "Fire", "kind": "resistance", "damage": true}],
	    "immunities": [], "vulnerabilities": [], "any": true,
	    "summary": "resistant to fire"
	  },
	  "damageTypes": [{"id": "acid", "name": "Acid"}],
	  "history": [{"date": "2025-09-05", "time": "21:14", "action": "gained",
	               "name": "Frightened", "notes": "the wraith's shriek"}]
	}
	#+END_SRC

	=conditions.all= is the whole catalog with =on= set, which is what the picker
	needs; =conditions.active= is the same entries filtered down to what is on.
	Only exhaustion has levels, and then =note= is the line the level in force
	carries. =damageTypes= is the ruleset's catalog, so a sheet can offer them
	without carrying its own copy.
	EDOC */
func RequestDndConditions(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	filename := r.URL.Query().Get("filename")
	id := r.URL.Query().Get("id")
	if strings.TrimSpace(filename) == "" && strings.TrimSpace(id) == "" {
		dndError(w, http.StatusBadRequest, "pass a filename or a character id")
		return
	}
	// The same lock as the inventory, the purse and the spellbook: they all
	// rewrite the same character file.
	dndInvLock.Lock()
	defer dndInvLock.Unlock()
	c, rs, path, err := dndFindCharacter(filename, id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, dndConditionState(c, rs, path, ""))
}

/* SDOC: API
* POST /dnd/conditions — Suffer Or Shake Off A Condition, Gain Or Lose A Defense
	Applies one change to what a character is under or shrugs off and writes the
	sheet back, adding a line to its Condition History section.

	*Method:* =POST=

	*Body:* =ConditionsRequest=
	| Field      | Type   | Required | Description                                                 |
	|------------+--------+----------+-------------------------------------------------------------|
	| =filename= | string | no       | The org character sheet. One of filename or id is required. |
	| =id=       | string | no       | The character's =DND_ID=.                                   |
	| =action=   | string | yes      | See the table below.                                        |
	| =name=     | string | no       | The condition, or the damage type for a defense action.     |
	| =level=    | number | no       | The exhaustion level, 1 to 6.                               |
	| =notes=    | string | no       | What brought it on, kept in the history.                    |

	| Action       | What it does                                          |
	|--------------+-------------------------------------------------------|
	| =add=        | Puts a condition on.                                  |
	| =remove=     | Takes it off.                                         |
	| =toggle=     | Whichever of the two applies - what the picker sends. |
	| =level=      | Sets a levelled condition; =0= takes it off.          |
	| =clear=      | Takes every condition off at once.                    |
	| =resist=     | Adds resistance to a damage type.                     |
	| =immune=     | Adds immunity.                                        |
	| =vulnerable= | Adds vulnerability.                                   |
	| =unprotect=  | Drops whichever of the three it is in.                |

	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "action": "toggle", "name": "frightened",
	 "notes": "the wraith's shriek"}
	#+END_SRC

	*Response:* The =ConditionsState= after the change, with =msg= saying what it
	did in one line.

	A damage type belongs to one of the three defense lists at a time: making a
	character immune to fire moves fire out of their resistances rather than
	listing it twice. A defense may name a condition or say something the rules
	have no id for, in which case it is kept as it was written.
	EDOC */
func PostDndConditions(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.ConditionsRequest
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
	event, err := dnd.ApplyConditionChange(c, rs, req)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if err := dndWriteCharacter(c, rs, path); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
		return
	}
	dndJson(w, dndConditionState(c, rs, path, dnd.ConditionEventMsg(event)))
}
