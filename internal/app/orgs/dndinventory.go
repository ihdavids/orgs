//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Inventory
//
// The html character sheet edits the equipment table on the org file itself:
// picking something up, using it, dropping it or packing it into a container
// all rewrite the sheet and add a line to its Inventory History section. There
// is no server side inventory state - the file is the state - so a sheet
// exported yesterday and a sheet exported now see the same bag.
// ----------------------------------------------------------------------------

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	dndplug "github.com/ihdavids/orgs/internal/app/orgs/plugs/dnd"
	"github.com/ihdavids/orgs/internal/common/dnd"
)

// dndInvLock serialises the read/modify/write cycle on a character sheet, so
// two quick clicks cannot lose one of the changes.
var dndInvLock sync.Mutex

// dndFindCharacter locates a character sheet by filename or by DND_ID and
// parses it. A sheet exported to disk knows both, but only the id survives
// being moved to another machine, so either will do.
func dndFindCharacter(filename, id string) (*dnd.Character, *dnd.Ruleset, string, error) {
	if strings.TrimSpace(filename) != "" {
		c, rs, err := dndplug.LoadCharacter(db, filename)
		if err == nil {
			return c, rs, dndCharacterPath(filename, c), nil
		}
		if strings.TrimSpace(id) == "" {
			return nil, nil, "", err
		}
	}
	if strings.TrimSpace(id) == "" {
		return nil, nil, "", errNoCharacter(filename, id)
	}
	for _, fname := range GetDb().GetFiles() {
		data, err := os.ReadFile(fname)
		if err != nil || !strings.Contains(string(data), id) {
			continue
		}
		c, rs, err := dndplug.ParseCharacter(string(data))
		if err != nil {
			continue
		}
		if dnd.CharacterId(c) != id {
			continue
		}
		c.Filename = fname
		return c, rs, fname, nil
	}
	return nil, nil, "", errNoCharacter(filename, id)
}

func errNoCharacter(filename, id string) error {
	which := filename
	if which == "" {
		which = id
	}
	return fmt.Errorf("no character sheet found for %q", which)
}

// dndCharacterPath is the path to write a sheet back to. The org database
// knows the full path for a file referred to by its basename.
func dndCharacterPath(filename string, c *dnd.Character) string {
	if c != nil && c.Filename != "" {
		return c.Filename
	}
	if f := GetDb().GetFile(filename); f != nil && f.Filename != "" {
		return f.Filename
	}
	return filename
}

// dndInventoryState is what every inventory call answers with.
func dndInventoryState(c *dnd.Character, rs *dnd.Ruleset, path, msg string) dnd.InventoryState {
	sheet := dnd.Compute(c, rs)
	history := c.InventoryLog
	if history == nil {
		history = []dnd.InventoryEvent{}
	}
	id := rs.Id
	return dnd.InventoryState{
		Id: dnd.CharacterId(c), Name: c.Name, Filename: path, Ruleset: id,
		Inventory: sheet.Inventory, History: history, Money: c.Money, Msg: msg,
	}
}

// dndWriteCharacter rewrites a sheet and tells the database about it.
func dndWriteCharacter(c *dnd.Character, rs *dnd.Ruleset, path string) error {
	org := dnd.RenderOrg(c, rs)
	if err := os.WriteFile(path, []byte(org), 0644); err != nil {
		return err
	}
	GetDb().ReloadFile(path)
	return nil
}

/* SDOC: API
* GET /dnd/inventory — What A Character Is Carrying
	Returns the equipment table stacked into containers, the weight carried, where that
	lands on the encumbrance scale, and the sheet's Inventory History.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                        |
	|------------+--------+----------+----------------------------------------------------|
	| =filename= | string | no       | The org character sheet (basename or path).        |
	| =id=       | string | no       | The character's =DND_ID=, used when no filename.   |

	One of the two is required.

	*Response:* An =InventoryState=:
	#+BEGIN_SRC json
	{
	  "id": "lyra-silverleaf-4c1f2a", "name": "Lyra", "filename": "/gtd/lyra.org",
	  "inventory": {
	    "containers": [
	      {"key": "", "name": "On Person", "weight": 21, "entries": [
	        {"key": "backpack", "name": "Backpack", "qty": 1, "weight": 5, "isContainer": true}]},
	      {"key": "backpack", "name": "Backpack", "capacity": 30, "weight": 16, "entries": []}
	    ],
	    "weight": 37, "carryCapacity": 150, "encumberedAt": 50,
	    "level": "", "label": "Unencumbered"
	  },
	  "history": [{"date": "2025-09-05", "time": "19:32", "action": "added",
	               "item": "Potion of Healing", "qty": 2, "to": "Backpack"}]
	}
	#+END_SRC
	EDOC */
func RequestDndInventory(w http.ResponseWriter, r *http.Request) {
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
	dndJson(w, dndInventoryState(c, rs, path, ""))
}

/* SDOC: API
* POST /dnd/inventory — Pick Up, Use, Drop Or Stow Something
	Applies one change to a character's equipment table and writes the sheet back, adding
	a line to its Inventory History section. Identical things stack, so adding a potion to
	a container that already holds two leaves one line saying three.

	*Method:* =POST=

	*Body:* =InventoryRequest=
	| Field       | Type   | Required | Description                                                    |
	|-------------+--------+----------+----------------------------------------------------------------|
	| =filename=  | string | no       | The org character sheet. One of filename or id is required.    |
	| =id=        | string | no       | The character's =DND_ID=.                                      |
	| =action=    | string | yes      | =add=, =use=, =drop=, =move= or =equip=.                       |
	| =item=      | string | yes      | An item id, or a name for homebrew the ruleset does not know.  |
	| =name=      | string | no       | Display name for an item with no id.                           |
	| =qty=       | number | no       | How many, defaults to 1.                                       |
	| =container= | string | no       | Container key the change applies to, empty for on the person.  |
	| =to=        | string | no       | Destination container for =move=.                              |
	| =equipped=  | bool   | no       | For =equip=: worn when true, taken off when false.             |
	| =notes=     | string | no       | Note stored on the line and in the history.                    |

	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "action": "add", "item": "potion-of-healing",
	 "qty": 2, "container": "backpack"}
	#+END_SRC

	*Response:* The =InventoryState= after the change.

	Storing something in a container the character does not own is refused with a =400=:
	the containers are the container items in the inventory, nothing else.

	=equip= wears or wields a stack, or takes it off, and is what the Worn column of the
	html sheet toggles. It carries the state wanted rather than a flip, so a click that
	lands twice still ends where it was asked to. Only what can be worn or wielded -
	armour, shields, weapons, rings, wands, staffs, rods and wondrous items, plus
	homebrew the ruleset has never heard of - can be equipped, and only on the person:
	something in a backpack has to be taken out first. Every line the stack was built
	from is set together, so two daggers shown as one line of two are drawn together.
	Nothing is written to the Inventory History - drawing a sword is not gaining or
	losing one.

	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "action": "equip", "item": "chain-mail", "equipped": true}
	#+END_SRC
	EDOC */
func PostDndInventory(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.InventoryRequest
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

	var event dnd.InventoryEvent
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "add":
		event, err = dnd.InventoryAdd(c, rs, req.Item, req.Name, req.Qty, req.Container, req.Notes)
	case "use", "consume":
		event, err = dnd.InventoryRemove(c, rs, req.Item, req.Qty, req.Container, dnd.InvUsed, req.Notes)
	case "drop", "remove":
		event, err = dnd.InventoryRemove(c, rs, req.Item, req.Qty, req.Container, dnd.InvDropped, req.Notes)
	case "move", "stow":
		event, err = dnd.InventoryMove(c, rs, req.Item, req.Qty, req.Container, req.To)
	case "equip", "wear":
		event, err = dnd.InventoryEquip(c, rs, req.Item, req.Container, req.Equipped)
	default:
		dndError(w, http.StatusBadRequest,
			"unknown action %q, expected add, use, drop, move or equip", req.Action)
		return
	}
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if err := dndWriteCharacter(c, rs, path); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
		return
	}
	dndJson(w, dndInventoryState(c, rs, path, dndEventMsg(event)))
}

// dndEventMsg is the one line the sheet shows after a change.
func dndEventMsg(e dnd.InventoryEvent) string {
	qty := strconv.Itoa(e.Qty)
	switch e.Action {
	case dnd.InvAdded:
		return "added " + qty + " " + e.Item + " to " + e.To
	case dnd.InvMoved:
		return "moved " + qty + " " + e.Item + " from " + e.From + " to " + e.To
	case dnd.InvWorn:
		return "wearing " + e.Item
	case dnd.InvRemoved:
		return "took off " + e.Item
	default:
		return e.Action + " " + qty + " " + e.Item
	}
}

/* SDOC: API
* GET /dnd/items — Fuzzy Item Search
	Searches the items of a ruleset the way the character builder's list filter does:
	every term has to match and letters may be skipped, so =pot heal= finds a Potion
	of Healing and =bkpk= finds a Backpack. This is what the add item box on the html
	character sheet asks.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter    | Type   | Required | Description                                              |
	|--------------+--------+----------+----------------------------------------------------------|
	| =q=          | string | no       | The search. Empty returns the front of the list.         |
	| =ruleset=    | string | no       | Ruleset to search, defaults to the character's or =srd=. |
	| =filename=   | string | no       | Character sheet, so hits can say how many you own.       |
	| =id=         | string | no       | The character's =DND_ID=, same purpose.                  |
	| =containers= | bool   | no       | Only return containers.                                  |
	| =filter=     | string | no       | Group to search in, see below. Defaults to =all=.        |
	| =limit=      | number | no       | Maximum hits, default 40.                                |

	The filter narrows the pool the search runs over to one group of things:
	=all=, =weapon=, =armor=, =potion=, =focus= (spellcasting foci and
	components), =gear=, =tool=, =pack=, =container= or =magic= (anything with
	a rarity). A name none of them goes by is an error rather than a silent
	=all=.

	*Response:*
	#+BEGIN_SRC json
	[{"id": "potion-of-healing", "name": "Potion of Healing", "kind": "gear",
	  "cost": "50 gp", "weight": 0.5, "rarity": "Common", "owned": 2,
	  "text": "You regain 2d4 + 2 hit points when you drink this potion."}]
	#+END_SRC
	EDOC */
func RequestDndItems(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	q := r.URL.Query()
	limit := 40
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	// The character, when we were told which one, gives us both the ruleset
	// to search and the counts of what is already owned.
	var owned map[string]int
	rsId := q.Get("ruleset")
	if strings.TrimSpace(q.Get("filename")) != "" || strings.TrimSpace(q.Get("id")) != "" {
		if c, rs, _, err := dndFindCharacter(q.Get("filename"), q.Get("id")); err == nil {
			owned = dnd.OwnedCounts(c)
			if rsId == "" && rs != nil {
				rsId = rs.Id
			}
		}
	}
	rs := dndRuleset(rsId)
	if rs == nil {
		dndError(w, http.StatusNotFound, "no ruleset %q", rsId)
		return
	}
	filter, ok := dnd.FindItemFilter(q.Get("filter"))
	if !ok {
		dndError(w, http.StatusBadRequest, "no item filter %q, expected one of %s",
			q.Get("filter"), strings.Join(dnd.ItemFilterNames(), ", "))
		return
	}
	containers := dndIsTrue(q.Get("containers"))
	dndJson(w, dnd.SearchItems(rs, q.Get("q"), limit, filter, containers, owned))
}

// dndIsTrue reads the usual spellings of yes out of a query parameter.
func dndIsTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	}
	return false
}
