//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Experience
//
// The DND_XP property of the character's own org file, changed from the ring
// round the portrait on the html sheet. Written the way inspiration is, so the
// ring, the level-up tray and the file never disagree.
// ----------------------------------------------------------------------------

import (
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

/* SDOC: API
* POST /dnd/xp — Add, Remove Or Set Experience
	Changes a character's experience and writes the sheet back, updating its
	=DND_XP= property. The level is never changed: crossing a threshold only
	makes =ready= true, and the level-up flow does the levelling.

	*Method:* =POST=

	*Body:* =XPRequest=
	| Field      | Type   | Required | Description                                                 |
	|------------+--------+----------+-------------------------------------------------------------|
	| =filename= | string | no       | The org character sheet. One of filename or id is required. |
	| =id=       | string | no       | The character's =DND_ID=.                                   |
	| =action=   | string | no       | =add= (the default), =remove= or =set=.                     |
	| =amount=   | int    | yes      | How much to add or remove, or what to set it to. Not negative. |

	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "action": "add", "amount": 450}
	#+END_SRC

	*Response:* An =XPState=:
	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "name": "Lyra", "filename": "/gtd/lyra.org",
	 "ruleset": "srd", "xp": 950, "level": 2, "nextLevelXp": 900, "xpPercent": 100,
	 "ready": true, "msg": "+450 XP, now 950 - ready for level 3"}
	#+END_SRC

	Removing more than the character has leaves them at nought.
	EDOC */
func PostDndXP(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.XPRequest
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
	was := c.XP
	msg, err := dnd.ApplyXP(c, strings.TrimSpace(req.Action), req.Amount)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if c.XP != was {
		if err := dndWriteCharacter(c, rs, path); err != nil {
			dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
			return
		}
	}
	state := dnd.NewXPState(c)
	state.Filename = path
	if rs != nil {
		state.Ruleset = rs.Id
	}
	state.Msg = msg
	dndJson(w, state)
}
