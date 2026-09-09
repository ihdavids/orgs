//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Spells
//
// The html character sheet edits the spell tables on the org file itself:
// learning a spell, giving one back and preparing one all rewrite the sheet.
// As with the inventory there is no server side state - the file is the state
// - and the rules engine decides what may be taken, so the sheet cannot ask
// for a spell the character is not entitled to.
// ----------------------------------------------------------------------------

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// dndSpellState is what every spellbook call answers with.
func dndSpellState(c *dnd.Character, rs *dnd.Ruleset, path, msg string) dnd.SpellbookState {
	id := ""
	if rs != nil {
		id = rs.Id
	}
	return dnd.SpellbookState{
		Id: dnd.CharacterId(c), Name: c.Name, Filename: path, Ruleset: id,
		Book: dnd.Spellbook(c, rs), Msg: msg,
	}
}

/*
	SDOC: API

  - GET /dnd/spellbook — What A Character Has Prepared
    Returns the character's spells grouped by level, the budgets they are held
    against - cantrips known, spells known or written in a spellbook, spells
    prepared - and the spell slots behind them.

    *Method:* =GET=

    *Query Parameters:*
    | Parameter  | Type   | Required | Description                                      |
    |------------+--------+----------+--------------------------------------------------|
    | =filename= | string | no       | The org character sheet (basename or path).      |
    | =id=       | string | no       | The character's =DND_ID=, used when no filename. |

    One of the two is required.

    *Response:* A =SpellbookState=:
    #+BEGIN_SRC json
    {
    "id": "lyra-silverleaf-4c1f2a", "name": "Lyra", "filename": "/gtd/lyra.org",
    "book": {
    "isCaster": true, "mode": "spellbook", "twoStage": true,
    "classId": "wizard", "className": "Wizard", "lists": ["wizard"],
    "maxLevel": 3, "spellSaveDc": 15, "spellAttack": "+7",
    "allotments": [
    {"kind": "cantrips", "name": "Cantrips", "used": 3, "max": 4, "left": 1},
    {"kind": "known", "name": "Spellbook", "used": 10, "max": 12, "left": 2},
    {"kind": "prepared", "name": "Prepared", "used": 6, "max": 8, "left": 2}
    ],
    "levels": [{"level": 0, "name": "Cantrips", "spells": [...]}]
    }
    }
    #+END_SRC

    =mode= is =known= for a class with a fixed list of spells, =list= for one that
    prepares from the whole class list, and =spellbook= for one that writes spells
    down and prepares a subset. Only a =spellbook= caster has =twoStage= set, and
    only for those is preparing a step of its own.
    EDOC
*/
func RequestDndSpellbook(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	filename := r.URL.Query().Get("filename")
	id := r.URL.Query().Get("id")
	if strings.TrimSpace(filename) == "" && strings.TrimSpace(id) == "" {
		dndError(w, http.StatusBadRequest, "pass a filename or a character id")
		return
	}
	// The same lock as the inventory: both rewrite the same character file,
	// and a spell learned while an item is being stowed must not lose either.
	dndInvLock.Lock()
	defer dndInvLock.Unlock()
	c, rs, path, err := dndFindCharacter(filename, id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, dndSpellState(c, rs, path, ""))
}

/*
	SDOC: API

  - POST /dnd/spellbook — Learn, Give Back Or Prepare A Spell
    Applies one change to a character's spells and writes the sheet back. The rules
    engine checks it first: a spell has to be on a list the character can draw
    from, of a level they have slots for, and there has to be room left in the
    allowance it comes out of. Anything else is refused with a =400= saying why.

    *Method:* =POST=

    *Body:* =SpellbookRequest=
    | Field      | Type   | Required | Description                                                 |
    |------------+--------+----------+-------------------------------------------------------------|
    | =filename= | string | no       | The org character sheet. One of filename or id is required. |
    | =id=       | string | no       | The character's =DND_ID=.                                   |
    | =action=   | string | yes      | =learn=, =forget=, =prepare= or =unprepare=.                |
    | =spell=    | string | yes      | A spell id, or its name.                                    |

    #+BEGIN_SRC json
    {"id": "lyra-silverleaf-4c1f2a", "action": "learn", "spell": "magic-missile"}
    #+END_SRC

    *Response:* The =SpellbookState= after the change.

    =prepare= and =unprepare= are only meaningful for a class that prepares from a
    spellbook; for everyone else a spell on the sheet is a spell ready to cast, and
    the way to stop having it is =forget=. Spells a subclass granted - domain
    spells, oath spells - are always prepared and cannot be given back.
    EDOC
*/
func PostDndSpellbook(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.SpellbookRequest
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
	spell := req.Spell
	if strings.TrimSpace(spell) == "" {
		spell = req.Name
	}
	msg, err := dnd.ApplySpellChange(c, rs, req.Action, spell)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if err := dndWriteCharacter(c, rs, path); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
		return
	}
	dndJson(w, dndSpellState(c, rs, path, msg))
}

/*
	SDOC: API

  - GET /dnd/spells — The Spells A Character Could Have
    Every spell on the lists this character draws from - its class lists, plus
    anything a subclass widened them with, plus whatever is already on the sheet -
    answered with what the character has done about each one and what they are
    still allowed to do. This is what the manage spells panel on the html sheet
    asks, and it asks once: the panel groups by level or school and filters
    locally, so regrouping needs no second call.

    The optional query filters the list the way the character builder's list filter
    does, fuzzily, over the name and the school, casting time, duration and tags:
    =mag mis= finds Magic Missile and =ritual= narrows to rituals.

    *Method:* =GET=

    *Query Parameters:*
    | Parameter  | Type   | Required | Description                                       |
    |------------+--------+----------+---------------------------------------------------|
    | =filename= | string | no       | The org character sheet (basename or path).       |
    | =id=       | string | no       | The character's =DND_ID=, used when no filename.  |
    | =q=        | string | no       | Fuzzy filter. Empty returns the whole list.       |
    | =limit=    | number | no       | Maximum hits, default 500.                        |

    One of filename or id is required: which spells may be taken is a fact about a
    character, not about a ruleset.

    *Response:*
    #+BEGIN_SRC json
    [{"id": "magic-missile", "name": "Magic Missile", "level": 1,
    "levelName": "1st Level", "school": "evocation", "castingTime": "1 action",
    "range": "120 feet", "duration": "Instantaneous", "ritual": false,
    "known": true, "prepared": true, "canForget": true,
    "summary": "You create three glowing darts of magical force."},
    {"id": "fireball", "name": "Fireball", "level": 3, "known": false,
    "canLearn": false, "why": "you have no 3rd slots yet"}]
    #+END_SRC
    EDOC
*/
func RequestDndSpells(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	q := r.URL.Query()
	if strings.TrimSpace(q.Get("filename")) == "" && strings.TrimSpace(q.Get("id")) == "" {
		dndError(w, http.StatusBadRequest, "pass a filename or a character id")
		return
	}
	limit := 500
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	dndInvLock.Lock()
	defer dndInvLock.Unlock()
	c, rs, _, err := dndFindCharacter(q.Get("filename"), q.Get("id"))
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, dnd.SearchSpells(c, rs, q.Get("q"), limit))
}

/*
	SDOC: API

  - POST /dnd/slots — Spend Or Hand Back A Spell Slot
    Marks one spell slot of a level as spent, or gives one back, and writes the
    character file. This is what the html sheet posts when a spell is cast: the
    slot the cast costs is struck off the sheet and off the org file in the same
    breath.

    *Method:* =POST=

    *Body:* =SlotRequest=
    | Field      | Type   | Required | Description                                                 |
    |------------+--------+----------+-------------------------------------------------------------|
    | =filename= | string | no       | The org character sheet. One of filename or id is required. |
    | =id=       | string | no       | The character's =DND_ID=.                                   |
    | =action=   | string | yes      | =cast=, =spend= or =recover=.                               |
    | =level=    | number | yes      | The spell level of the slot, 1 to 9.                        |
    | =spell=    | string | no       | What is being cast. It only appears in the message.         |

    #+BEGIN_SRC json
    {"id": "lyra-silverleaf-4c1f2a", "action": "cast", "level": 1,
    "spell": "Magic Missile"}
    #+END_SRC

    =cast= reaches upward when the level asked for is empty - a spell may always
    be cast from a higher slot - and says so in =slot=; =spend= means the level
    named and no other, which is what clicking a slot on the sheet does. With
    nothing left to spend at that level or above it, the call is refused with a
    =400= and nothing is written.

    *Response:* The =SpellbookState= after the change, with =slot= saying where it
    landed:
    #+BEGIN_SRC json
    {"msg": "Magic Missile cast at 2nd level, 2 of 3 left",
    "slot": {"level": 2, "asked": 1, "total": 3, "used": 1, "left": 2, "up": true}}
    #+END_SRC
    EDOC
*/
func PostDndSlots(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.SlotRequest
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
	change, err := dnd.ApplySlotChange(c, rs, req.Action, req.Level, req.Spell)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if err := dndWriteCharacter(c, rs, path); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
		return
	}
	state := dndSpellState(c, rs, path, change.Msg)
	state.Slot = change
	dndJson(w, state)
}
