//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Undo
//
// Taking back the last thing that happened to a character sheet. The html
// sheet writes every click straight into the org file, which is what makes it
// worth trusting and what makes a misplaced click expensive - so the last
// operation, and only the last one, can be reversed.
//
// The work is all in undo.go in the engine; this is the two calls the sheet
// makes: one to ask what pressing Undo would do, and one to press it.
// ----------------------------------------------------------------------------

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// dndJournalPlan is the newest journalled change said the way an UndoPlan
// says things, so the sheet draws one answer whichever mechanism produced
// it. The journal reaches things the history cannot - a line taken off the
// sheet, a note rewritten, a roll thrown away - because those deliberately
// write nothing down; see dndjournal.go.
func dndJournalPlan() (dndChange, dnd.UndoPlan, bool) {
	ch, ok := dndLastChange()
	if !ok {
		return ch, dnd.UndoPlan{}, false
	}
	what := ch.What
	if what == "" {
		what = "the last change to " + filepath.Base(ch.Path)
	} else if filepath.Ext(ch.Path) == ".org" &&
		strings.Contains(filepath.Base(ch.Path), "_") {
		// A session file, which is not the sheet the button lives on, so
		// the line says which one it was.
		what += " in " + filepath.Base(ch.Path)
	}
	return ch, dnd.UndoPlan{
		Can: true, What: what, Kind: "change",
		When: ch.When.Format("2006-01-02 15:04"),
	}, true
}

// DndUndoState is the answer to an undo. It is the inventory answer with the
// undo on top, because taking something back can move the bag, the purse and
// the hit points at once and the sheet would otherwise have to ask three more
// questions to find out which.
type DndUndoState struct {
	dnd.InventoryState
	// Undone is what was just taken back, and Undo what taking back the next
	// thing would do - so the button relabels itself from the same answer.
	Undone dnd.UndoPlan `json:"undone"`
	Undo   dnd.UndoPlan `json:"undo"`
	// HealthHistory is the Health History after the undo, since a line may
	// have come off it.
	HealthHistory []dnd.HealthEvent `json:"healthHistory"`
}

/* SDOC: API
* GET /dnd/undo — What Undo Would Take Back
	Answers what the last thing that happened to a character was, and whether it can be
	reversed. Nothing is changed; this is what the html sheet asks so its Undo button can
	say what pressing it will do rather than offering a bare "Undo".

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                      |
	|------------+--------+----------+--------------------------------------------------|
	| =filename= | string | no       | The org character sheet (basename or path).      |
	| =id=       | string | no       | The character's =DND_ID=, used when no filename. |

	One of the two is required.

	*Response:* An =UndoPlan=:
	#+BEGIN_SRC json
	{"can": true, "what": "using Potion of Healing and the hit points it moved",
	 "when": "2026-09-18 19:32", "kind": "used"}
	#+END_SRC

	When =can= is false, =why= says what is in the way in the words the sheet shows -
	"the hit points have moved since", "nothing has happened yet".
	EDOC */
func RequestDndUndo(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	filename := r.URL.Query().Get("filename")
	id := r.URL.Query().Get("id")
	if strings.TrimSpace(filename) == "" && strings.TrimSpace(id) == "" {
		dndError(w, http.StatusBadRequest, "pass a filename or a character id")
		return
	}
	dndInvLock.Lock()
	defer dndInvLock.Unlock()
	c, rs, _, err := dndFindCharacter(filename, id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	// What this server did last, if it still stands, beats what the history
	// says: it is newer by definition, and it covers the changes the
	// history never hears about.
	if _, plan, ok := dndJournalPlan(); ok {
		dndJson(w, plan)
		return
	}
	dndJson(w, dnd.LastUndo(c, rs))
}

/* SDOC: API
* POST /dnd/undo — Take The Last Thing Back
	Reverses the last operation on a character sheet and writes the sheet back with the
	lines it wrote taken out of the history. An accident leaves no trace rather than
	leaving a correction.

	*Method:* =POST=

	*Body:*
	| Field      | Type   | Required | Description                                                 |
	|------------+--------+----------+-------------------------------------------------------------|
	| =filename= | string | no       | The org character sheet. One of filename or id is required. |
	| =id=       | string | no       | The character's =DND_ID=.                                   |

	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a"}
	#+END_SRC

	*Response:* The =InventoryState= after the undo, with =undone= saying what was taken
	back, =undo= saying what pressing it again would do, and =healthHistory= carrying the
	Health History, since a line may have come off that too.

	There are two ways it can take something back. The first is a copy of the file as it
	stood before the change, kept in memory for the last thirty changes this server made
	(see dndjournal.go); that is what reaches the changes which deliberately write nothing
	down - an inventory line taken off the sheet as a correction, a note rewritten, a roll
	thrown away - and it reaches session files as well as character ones. The second, used
	when the journal has nothing to say, works the last operation out of the character's
	own history tables; that one survives a restart and works on a file something else
	changed.

	One operation, and only ever the last one. A potion drunk puts the potion back and the
	hit points with it; a purchase puts the coin back and takes the item away again. Where
	anything has happened since - a blow taken, a rest, an edit by hand - there is nothing
	safe to reverse and the call is refused with a =400= saying so, rather than guessing.
	EDOC */
func PostDndUndo(w http.ResponseWriter, r *http.Request) {
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

	// The journal first. Putting the file back exactly as it was is both
	// more general than working the change out of the history and exact for
	// the changes that leave no history at all.
	if ch, plan, ok := dndJournalPlan(); ok {
		if err := dndPutBack(ch); err != nil {
			dndError(w, http.StatusInternalServerError,
				"could not put %s back: %s", ch.Path, err)
			return
		}
		dndDropChange()
		// Read the character again: it may be the file that just changed,
		// and even when it is not the answer should be current.
		if again, rs2, path2, err := dndFindCharacter(req.Filename, req.Id); err == nil {
			c, rs, path = again, rs2, path2
		}
		history := c.HealthLog
		if history == nil {
			history = []dnd.HealthEvent{}
		}
		_, next, _ := dndJournalPlan()
		if next.What == "" {
			next = dnd.LastUndo(c, rs)
		}
		dndJson(w, DndUndoState{
			InventoryState: dndInventoryState(c, rs, path, "undid "+plan.What),
			Undone:         plan,
			Undo:           next,
			HealthHistory:  history,
		})
		return
	}

	res, err := dnd.ApplyUndo(c, rs)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if err := dndWriteCharacter(c, rs, path); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
		return
	}
	history := c.HealthLog
	if history == nil {
		history = []dnd.HealthEvent{}
	}
	dndJson(w, DndUndoState{
		InventoryState: dndInventoryState(c, rs, path, res.Msg),
		Undone:         res.Plan,
		Undo:           dnd.LastUndo(c, rs),
		HealthHistory:  history,
	})
}
