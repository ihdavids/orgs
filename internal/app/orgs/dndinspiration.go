//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Inspiration
//
// One bit on the character sheet, handed out by the DM and spent by the
// player. It is written straight into the character's own org file the way hit
// points and coin are, so the marker on the html sheet and the
// DND_INSPIRATION property never disagree.
// ----------------------------------------------------------------------------

import (
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// dndInspirationState is what every inspiration call answers with.
func dndInspirationState(c *dnd.Character, rs *dnd.Ruleset, path, msg string) dnd.InspirationState {
	id := ""
	if rs != nil {
		id = rs.Id
	}
	return dnd.InspirationState{
		Id: dnd.CharacterId(c), Name: c.Name, Filename: path, Ruleset: id,
		Inspiration: c.Inspiration, Msg: msg,
	}
}

/* SDOC: API
* GET /dnd/inspiration — Whether A Character Holds Inspiration
	Returns whether a character is currently holding inspiration. Nothing is
	changed.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                      |
	|------------+--------+----------+--------------------------------------------------|
	| =filename= | string | no       | The org character sheet (basename or path).      |
	| =id=       | string | no       | The character's =DND_ID=, used when no filename. |

	One of the two is required.

	*Response:* An =InspirationState=:
	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "name": "Lyra", "filename": "/gtd/lyra.org",
	 "ruleset": "srd", "inspiration": true, "msg": ""}
	#+END_SRC
	EDOC */
func RequestDndInspiration(w http.ResponseWriter, r *http.Request) {
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
	dndJson(w, dndInspirationState(c, rs, path, ""))
}

/* SDOC: API
* POST /dnd/inspiration — Gain Or Spend Inspiration
	Sets whether a character holds inspiration and writes the sheet back, updating
	its =DND_INSPIRATION= property.

	*Method:* =POST=

	*Body:* =InspirationRequest=
	| Field      | Type   | Required | Description                                                 |
	|------------+--------+----------+-------------------------------------------------------------|
	| =filename= | string | no       | The org character sheet. One of filename or id is required. |
	| =id=       | string | no       | The character's =DND_ID=.                                   |
	| =action=   | string | no       | =toggle= (the default), =gain= or =spend=.                  |

	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "action": "spend"}
	#+END_SRC

	*Response:* The =InspirationState= after the change, with =msg= saying what it
	did in one line.

	Gaining inspiration a character already holds, or spending what they have not
	got, is not an error - it is two people at the table pressing the same button -
	so the sheet is left where it was and the message says so.
	EDOC */
func PostDndInspiration(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.InspirationRequest
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
	was := c.Inspiration
	msg, err := dnd.ApplyInspiration(c, strings.TrimSpace(req.Action))
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	// Nothing moved, so there is nothing worth rewriting the file for.
	if c.Inspiration != was {
		if err := dndWriteCharacter(c, rs, path); err != nil {
			dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
			return
		}
	}
	dndJson(w, dndInspirationState(c, rs, path, msg))
}
