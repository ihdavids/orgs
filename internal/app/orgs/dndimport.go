//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

/*
	SDOC: API

  - POST /dnd/import — Convert a D&D Beyond Character into an Org Sheet
    Takes a character-service payload from D&D Beyond, maps it onto the ruleset and
    writes it out as an org character sheet, exactly as =/dnd/save= would.

    The client does the fetching, so no D&D Beyond credential ever reaches the server -
    the body carries the json the site returned, nothing else.

    *Method:* =POST=

    *Body:* =DDBImportRequest=
    #+BEGIN_SRC json
    {"ruleset": "srd", "filename": "characters/lyra.org", "overwrite": false,
    "payload": {"success": true, "data": {"name": "Lyra", "...": "..."}}}
    #+END_SRC

    Set =preview= to convert and return the character without writing anything.

    *Response:* =DDBImportResponse=. =warnings= lists everything that did not map onto
    ruleset content - an unknown subclass, a homebrew item - so the sheet can be checked
    over rather than silently trusted.
    EDOC
*/
func PostDndImport(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.DDBImportRequest
	if !dndBody(w, r, &req) {
		return
	}
	if len(req.Payload) == 0 {
		dndError(w, http.StatusBadRequest, "no D&D Beyond character in the request")
		return
	}
	rs := dndRuleset(req.Ruleset)
	if rs == nil {
		dndError(w, http.StatusNotFound, "no ruleset %q", req.Ruleset)
		return
	}
	c, warnings, err := dnd.ImportDDB(req.Payload, rs)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if req.Player != "" {
		c.Player = req.Player
	}
	res := dnd.DDBImportResponse{Ok: true, Character: c, Warnings: warnings}
	if req.Preview {
		res.Sheet = dnd.Compute(c, rs)
		res.Org = dnd.RenderOrg(c, rs)
		res.Msg = fmt.Sprintf("%s imported, nothing written", c.Name)
		dndJson(w, res)
		return
	}
	fname, err := dndResolveFilename(req.Filename, c.Name)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	existing, statErr := os.Stat(fname)
	if statErr == nil && !req.Overwrite {
		dndError(w, http.StatusConflict,
			"%s already exists, pass overwrite to bring the D&D Beyond version over it", fname)
		return
	}
	if statErr == nil && !existing.IsDir() {
		// Re-importing an already imported character replaces the sheet, so
		// keep the parts of it that are the org file's own: the identity that
		// play session logs are stamped with, and the inventory history,
		// which D&D Beyond has no equivalent of and cannot send back.
		dndCarryOver(fname, c, rs)
	}
	if err := os.MkdirAll(filepath.Dir(fname), 0755); err != nil {
		dndError(w, http.StatusInternalServerError, "could not create directory: %s", err)
		return
	}
	org := dnd.RenderOrg(c, rs)
	if err := os.WriteFile(fname, []byte(org), 0644); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", fname, err)
		return
	}
	GetDb().ReloadFile(fname)
	res.Filename = fname
	res.Org = org
	res.Sheet = dnd.Compute(c, rs)
	res.Msg = fmt.Sprintf("wrote %s", fname)
	dndJson(w, res)
}

// dndCarryOver keeps the fields that belong to the org sheet rather than to
// D&D Beyond when an import lands on top of an existing character.
func dndCarryOver(fname string, c *dnd.Character, rs *dnd.Ruleset) {
	data, err := os.ReadFile(fname)
	if err != nil {
		return
	}
	old, err := dnd.ParseOrg(string(data), rs)
	if err != nil || old == nil {
		return
	}
	if old.Id != "" {
		c.Id = old.Id
	}
	c.InventoryLog = old.InventoryLog
	if c.Image == "" {
		c.Image = old.Image
		c.ImageFocus = old.ImageFocus
		c.ImageZoom = old.ImageZoom
	}
}
