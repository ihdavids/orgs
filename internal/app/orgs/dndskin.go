//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Sheet skins
//
// Which look the html sheet wears, kept in the character's own org file
// (DND_SHEET_SKIN) so the sheet opens in it wherever it is exported. The
// skins themselves are css files the exporter reads (plugs/dnd/skins.go).
// ----------------------------------------------------------------------------

import (
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

/* SDOC: API
* POST /dnd/skin — Choose The Sheet's Skin
	Sets which skin a character's html sheet wears and writes it into the
	=DND_SHEET_SKIN= property. An empty skin is the classic sheet.

	*Method:* =POST=

	*Body:* =SkinRequest=
	| Field      | Type   | Required | Description                                                 |
	|------------+--------+----------+-------------------------------------------------------------|
	| =filename= | string | no       | The org character sheet. One of filename or id is required. |
	| =id=       | string | no       | The character's =DND_ID=.                                   |
	| =skin=     | string | no       | A skin's id (=wildwood=), or empty for the classic sheet.   |

	*Response:* ={ok, skin, msg}=
	EDOC */
func PostDndSkin(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.SkinRequest
	if !dndBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Filename) == "" && strings.TrimSpace(req.Id) == "" {
		dndError(w, http.StatusBadRequest, "pass a filename or a character id")
		return
	}
	skin := strings.TrimSpace(req.Skin)
	if skin == "classic" {
		skin = ""
	}
	if !dnd.ValidSkinId(skin) {
		dndError(w, http.StatusBadRequest, "%q is not a skin name", skin)
		return
	}
	dndInvLock.Lock()
	defer dndInvLock.Unlock()
	c, rs, path, err := dndFindCharacter(req.Filename, req.Id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	if c.SheetSkin != skin {
		c.SheetSkin = skin
		if err := dndWriteCharacter(c, rs, path); err != nil {
			dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
			return
		}
	}
	msg := "the classic sheet"
	if skin != "" {
		msg = "wearing " + skin
	}
	dndJson(w, map[string]interface{}{"ok": true, "skin": skin, "msg": msg})
}
