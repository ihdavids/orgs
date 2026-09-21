//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Concentration
//
// What a caster is holding their attention on, written into the character's own
// org file as DND_CONCENTRATION the way the hit points and the conditions are.
// A spell put up here is still up when the sheet is reloaded, which is the
// point: the thing everyone forgets at the table is the one worth writing down.
//
// The saving throw a blow calls for is asked for by /dnd/hp, which knows how
// much damage landed. The roll is the sheet's - it has the dice - and comes
// back here as the save action to be settled.
// ----------------------------------------------------------------------------

import (
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// dndConcentrationState is what every concentration call answers with.
func dndConcentrationState(c *dnd.Character, rs *dnd.Ruleset, path, msg string,
	kept bool) dnd.ConcentrationState {
	id := ""
	if rs != nil {
		id = rs.Id
	}
	return dnd.ConcentrationState{
		Id: dnd.CharacterId(c), Name: c.Name, Filename: path, Ruleset: id,
		Concentration: dnd.ComputeConcentration(dnd.Compute(c, rs), rs),
		Kept:          kept, Msg: msg,
	}
}

/* SDOC: API
* GET /dnd/concentration — What A Caster Is Holding
	Returns the spell a character is concentrating on, if any, along with the
	Constitution saving throw they make to keep it. Nothing is changed.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                      |
	|------------+--------+----------+--------------------------------------------------|
	| =filename= | string | no       | The org character sheet (basename or path).      |
	| =id=       | string | no       | The character's =DND_ID=, used when no filename. |

	One of the two is required.

	*Response:* A =ConcentrationState=:
	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "name": "Lyra", "filename": "/gtd/lyra.org",
	 "ruleset": "srd",
	 "concentration": {"on": true, "spell": "haste", "name": "Haste", "level": 3,
	                   "label": "Haste (3rd level)", "duration": "1 minute",
	                   "saveMod": 2, "saveStr": "+2"}}
	#+END_SRC

	=on= is false when nothing is being held, which is the usual state: the banner
	has to be able to draw its own absence.
	EDOC */
func RequestDndConcentration(w http.ResponseWriter, r *http.Request) {
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
	dndJson(w, dndConcentrationState(c, rs, path, "", false))
}

/* SDOC: API
* POST /dnd/concentration — Hold A Spell, Let It Go, Or Save For It
	Sets what a character is concentrating on and writes the sheet back, updating
	its =DND_CONCENTRATION= property.

	*Method:* =POST=

	*Body:* =ConcentrationRequest=
	| Field      | Type   | Required | Description                                                 |
	|------------+--------+----------+-------------------------------------------------------------|
	| =filename= | string | no       | The org character sheet. One of filename or id is required. |
	| =id=       | string | no       | The character's =DND_ID=.                                   |
	| =action=   | string | yes      | =start=, =drop= or =save=.                                  |
	| =spell=    | string | no       | For =start=: the spell id or name being held.               |
	| =level=    | number | no       | For =start=: the slot level it was cast at.                 |
	| =dc=       | number | no       | For =save=: the DC the blow called for, default 10.         |
	| =roll=     | number | no       | For =save=: what the Constitution save came to.             |

	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "action": "start", "spell": "haste", "level": 3}
	#+END_SRC

	*Response:* The =ConcentrationState= after the change, with =msg= saying what it
	did in one line. For =save=, =kept= says whether the spell survived.

	A caster holds one spell at a time, so =start= on a second one lets the first go
	rather than refusing: that is what the rules do, and a sheet that argued about it
	would be wrong. The message names what was dropped. A spell that does not ask for
	concentration is refused.

	=save= settles a roll against a DC: at or above it the spell stays up, under it
	the spell is lost. The DC is =/dnd/hp='s to work out - it knows how much damage
	landed - and rides back on the answer to the blow as =save=.

	Letting go of nothing is not an error. A sheet that has just been reloaded and one
	that never had a spell up look the same, and both should be able to press the
	button.
	EDOC */
func PostDndConcentration(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.ConcentrationRequest
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

	was := dnd.ConcentrationProp(c.Concentration)
	msg, kept := "", false
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case dnd.ConcStart, "hold", "cast":
		msg, err = dnd.StartConcentration(c, rs, req.Spell, req.Level)
	case dnd.ConcDrop, "let go", "end", "stop":
		msg = dnd.DropConcentration(c, rs)
	case dnd.ConcSave, "check":
		kept, msg, err = dnd.ApplyConcentrationSave(c, rs, req.DC, req.Roll)
	default:
		dndError(w, http.StatusBadRequest,
			"unknown action %q, expected start, drop or save", req.Action)
		return
	}
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	// Nothing moved, so there is nothing worth rewriting the file for - a save
	// that was made is the common case and should cost no disk write.
	if dnd.ConcentrationProp(c.Concentration) != was {
		if err := dndWriteCharacter(c, rs, path); err != nil {
			dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
			return
		}
	}
	dndJson(w, dndConcentrationState(c, rs, path, msg, kept))
}
