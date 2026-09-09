//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Rests
//
// Taking a rest is the one action on the html character sheet that touches
// nearly everything at once: hit points, hit dice, spell slots and the uses
// of every feature that recharges. As with the inventory and the spellbook
// there is no server side state - the org character sheet is the state - and
// the rules engine decides what a rest gives back, so the sheet cannot ask
// for hit dice the character does not have.
//
// Spending a single use of a feature goes through the same endpoint, because
// it writes to the same drawer on the same file and wants the same lock.
// ----------------------------------------------------------------------------

import (
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// dndRestState is what every rest call answers with: both plans, freshly
// computed, so the sheet can redraw its rest menu from one round trip.
func dndRestState(c *dnd.Character, rs *dnd.Ruleset, path, msg string,
	result *dnd.RestResult) dnd.RestState {
	id := ""
	if rs != nil {
		id = rs.Id
	}
	return dnd.RestState{
		Id: dnd.CharacterId(c), Name: c.Name, Filename: path, Ruleset: id,
		Short:  dnd.RestPlan(c, rs, dnd.ShortRest),
		Long:   dnd.RestPlan(c, rs, dnd.LongRest),
		Result: result, Msg: msg,
	}
}

/*
	SDOC: API

  - GET /dnd/rest — What A Rest Would Give Back
    Returns both rest plans for a character: what a short rest and a long rest
    would ask of them, step by step, and everything each one would hand back.
    Nothing is changed - this is what the sheet's rest menu is drawn from.

    *Method:* =GET=

    *Query Parameters:*
    | Parameter  | Type   | Required | Description                                      |
    |------------+--------+----------+--------------------------------------------------|
    | =filename= | string | no       | The org character sheet (basename or path).      |
    | =id=       | string | no       | The character's =DND_ID=, used when no filename. |

    One of the two is required.

    *Response:* A =RestState=:
    #+BEGIN_SRC json
    {
    "id": "lyra-silverleaf-4c1f2a", "name": "Lyra", "filename": "/gtd/lyra.org",
    "short": {
    "kind": "short", "name": "Short Rest", "duration": "at least 1 hour",
    "hpCurrent": 14, "hpMax": 27, "hitDice": "4d6", "hitDiceLeft": 3,
    "hitDieFaces": 6, "conMod": 1,
    "steps": [
    {"id": "settle", "kind": "info", "title": "Take an hour", "text": "..."},
    {"id": "hitdice", "kind": "hitdice", "title": "Spend hit dice",
    "max": 3, "die": "d6", "mod": 1, "text": "..."},
    {"id": "recharge", "kind": "info", "title": "What comes back", "text": "..."}
    ],
    "recharges": [{"name": "Second Wind", "spent": 1, "max": 1, "kind": "feature"}]
    },
    "long": {"kind": "long", "name": "Long Rest", "...": "..."}
    }
    #+END_SRC

    A step of kind =hitdice= is the only one that asks for anything back; every
    other step is prose to read and acknowledge.
    EDOC
*/
func RequestDndRest(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	filename := r.URL.Query().Get("filename")
	id := r.URL.Query().Get("id")
	if strings.TrimSpace(filename) == "" && strings.TrimSpace(id) == "" {
		dndError(w, http.StatusBadRequest, "pass a filename or a character id")
		return
	}
	// The same lock as the inventory and the spellbook: all three rewrite the
	// same character file.
	dndInvLock.Lock()
	defer dndInvLock.Unlock()
	c, rs, path, err := dndFindCharacter(filename, id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, dndRestState(c, rs, path, "", nil))
}

/*
	SDOC: API

  - POST /dnd/rest — Take A Short Or A Long Rest
    Applies a rest to a character and writes the sheet back. A short rest spends
    the hit dice the player chose to spend, heals what those dice rolled, and
    hands back one spell slot of every level the character has spent one at; a
    long rest returns every hit point, half the hit dice, every spell slot and
    the uses of every limited feature. Asking to spend more hit dice than the
    character has left is refused with a =400=.

    The slot a short rest gives back is a house rule - by the book only a
    warlock's slots come back on one, and a warlock still gets all of theirs.

    The dice are not rolled here. The sheet rolls them in the open, on the table,
    and sends the total, so what the player watched land is what the file records.

    *Method:* =POST=

    *Body:* =RestRequest=
    | Field             | Type   | Required | Description                                                 |
    |-------------------+--------+----------+-------------------------------------------------------------|
    | =filename=        | string | no       | The org character sheet. One of filename or id is required. |
    | =id=              | string | no       | The character's =DND_ID=.                                   |
    | =kind=            | string | yes      | =short= or =long=.                                          |
    | =hitDiceSpent=    | number | no       | Hit dice spent on a short rest.                             |
    | =hitPointsHealed= | number | no       | What those dice came to, in hit points.                     |
    | =note=            | string | no       | A line to add to the session log entry.                     |

    #+BEGIN_SRC json
    {"id": "lyra-silverleaf-4c1f2a", "kind": "short",
    "hitDiceSpent": 2, "hitPointsHealed": 9}
    #+END_SRC

    *Response:* The =RestState= after the rest, with =result= filled in:
    #+BEGIN_SRC json
    {"result": {"kind": "short", "name": "Short Rest", "hpBefore": 14,
    "hpAfter": 23, "hpMax": 27, "healed": 9, "diceSpent": 2,
    "featuresBack": ["Second Wind (1 of 1)"],
    "lines": ["*Short Rest.* Hit points 23 of 27, 9 regained.",
    "- Spent 2 hit dice.", "- Recovered Second Wind (1 of 1)."]}}
    #+END_SRC

    =result.lines= is org markup, ready to be posted to a play session's notes.
    EDOC
*/
func PostDndRest(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.RestRequest
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
	result, err := dnd.ApplyRest(c, rs, req)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if err := dndWriteCharacter(c, rs, path); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
		return
	}
	dndJson(w, dndRestState(c, rs, path, result.Summary, result))
}

/*
	SDOC: API

  - POST /dnd/uses — Spend Or Recover One Use Of A Feature
    Marks one use of a limited feature or trait as spent, or hands one back, and
    writes the sheet. Which features have a limit, and how many uses they have,
    is worked out by the rules engine from the feature's own text; asking about
    one that has no limit is refused with a =400=.

    *Method:* =POST=

    *Body:* =UsesRequest=
    | Field      | Type   | Required | Description                                                 |
    |------------+--------+----------+-------------------------------------------------------------|
    | =filename= | string | no       | The org character sheet. One of filename or id is required. |
    | =id=       | string | no       | The character's =DND_ID=.                                   |
    | =action=   | string | yes      | =spend= or =recover=.                                       |
    | =feature=  | string | yes      | The feature's name, or the slug of it.                       |

    #+BEGIN_SRC json
    {"id": "lyra-silverleaf-4c1f2a", "action": "spend", "feature": "Second Wind"}
    #+END_SRC

    *Response:* The =RestState= after the change, so the sheet can redraw the
    rest menu and the feature's slots from one answer.
    EDOC
*/
func PostDndUses(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.UsesRequest
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
	msg, err := dnd.ApplyUseChange(c, rs, req.Action, req.Feature)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if err := dndWriteCharacter(c, rs, path); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
		return
	}
	dndJson(w, dndRestState(c, rs, path, msg, nil))
}
