//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// The timeline
//
// The timeline pane on the html sheet works the shape of an evening out of a
// session's two logs and writes nothing back - see the timeline section of
// dnd_character_html.tpl. These are the three things it does write.
//
// The first is throwing a whole block away. A combat block is a run of rolls
// with the notes taken during it hanging off it, and the table thinks of that
// as one thing; so does this, in one write, which is what makes one press of
// undo put all of it back.
//
// The second is annotating a block - naming the fight, saying what the room
// was. That is stored, because nothing about it can be derived; see
// sessionmark.go in the engine.
//
// The third is the undo for both, which is the journal narrowed to the one
// session being looked at. The character sheet's own Undo button takes back
// the last thing that happened anywhere; a drawer showing one evening should
// offer to take back the last thing that happened to that evening.
// ----------------------------------------------------------------------------

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common/dnd"
)

// dndBlockRequest is one block of the timeline being thrown away: the rolls
// and the notes it is made of, and the anchor of the annotation hung on it,
// which goes with it.
type dndBlockRequest struct {
	Rolls []dnd.SessionRef `json:"rolls"`
	Notes []dnd.SessionRef `json:"notes"`
	// Mark is the annotation to take off with the block. A block that was
	// never annotated leaves it empty, and an anchor carrying nothing is not
	// an error - the block is what is being deleted.
	Mark *dnd.SessionMark `json:"mark"`
	// What the block is, in the words the undo offer will use it in.
	What string `json:"what"`
}

// dndMarkRequest is one annotation being written.
type dndMarkRequest struct {
	Time  string `json:"time"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Note  string `json:"note"`
}

// DeleteDndBlock takes every roll and note of one timeline block out of a
// session in one write, along with the annotation hung on it. Anything the
// engine refuses refuses the lot and nothing is written.
func DeleteDndBlock(id string, req dndBlockRequest) (*dnd.SessionDetail, error) {
	said := strings.TrimSpace(req.What)
	if said == "" {
		said = "throwing a block off the timeline"
	} else {
		said = "throwing " + said + " off the timeline"
	}
	var refused error
	_, err := updateDndSession(id, func(text string) string {
		out, err := dnd.DeleteEntries(text, req.Rolls, req.Notes)
		if err != nil {
			refused = err
			return text
		}
		if req.Mark != nil {
			out = dnd.DropMark(out, req.Mark.Time, req.Mark.Kind)
		}
		return out
	}, said)
	if refused != nil {
		return nil, refused
	}
	if err != nil {
		return nil, err
	}
	return GetDndSession(id)
}

// SetDndMark writes an annotation onto one block of a session's timeline,
// replacing whatever was on that anchor.
func SetDndMark(id string, m dnd.SessionMark) (*dnd.SessionDetail, error) {
	var refused error
	_, err := updateDndSession(id, func(text string) string {
		out, err := dnd.SetMark(text, m)
		if err != nil {
			refused = err
			return text
		}
		return out
	}, "annotating the timeline")
	if refused != nil {
		return nil, refused
	}
	if err != nil {
		return nil, err
	}
	return GetDndSession(id)
}

// DeleteDndMark takes one annotation off a session's timeline.
func DeleteDndMark(id, at, kind string) (*dnd.SessionDetail, error) {
	var refused error
	_, err := updateDndSession(id, func(text string) string {
		out, err := dnd.DeleteMark(text, at, kind)
		if err != nil {
			refused = err
			return text
		}
		return out
	}, "taking an annotation off the timeline")
	if refused != nil {
		return nil, refused
	}
	if err != nil {
		return nil, err
	}
	return GetDndSession(id)
}

// dndSessionUndoPlan says what taking the last change to one session back
// would do, in the same words the character sheet's undo uses.
func dndSessionUndoPlan(id string) (dndChange, dnd.UndoPlan, error) {
	_, file, err := dndSessionText(id)
	if err != nil {
		return dndChange{}, dnd.UndoPlan{}, err
	}
	ch, ok := dndLastChangeTo(file)
	if !ok {
		return dndChange{}, dnd.UndoPlan{
			Can: false,
			Why: "nothing has changed in this session that can be taken back",
		}, nil
	}
	what := ch.What
	if what == "" {
		what = "the last change to " + filepath.Base(ch.Path)
	}
	return ch, dnd.UndoPlan{
		Can: true, What: what, Kind: "change",
		When: ch.When.Format("2006-01-02 15:04"),
	}, nil
}

/* SDOC: API
* POST /dnd/play/session/{id}/delete — Throw A Timeline Block Away
	Takes several rolls and notes out of a session in one write, which is what deleting a
	block of the timeline is: a combat block is a run of rolls with the notes taken during
	it hanging off it, and the table thinks of that as one thing.

	*Method:* =POST=

	*Body:*
	| Field    | Type   | Description                                                      |
	|----------+--------+------------------------------------------------------------------|
	| =rolls=  | array  | Rolls to remove, each =index= and the =was= the page read.       |
	| =notes=  | array  | Notes to remove, each =index= and the =was= the page read.       |
	| =mark=   | object | The annotation on the block, =time= and =kind=, removed with it. |
	| =what=   | string | What the block is, used in what undo offers to take back.        |

	#+BEGIN_SRC json
	{"what": "a combat block", "rolls": [{"index": 4, "was": "Longsword"}],
	 "notes": [{"index": 2, "was": "19:41"}], "mark": {"time": "19:32", "kind": "fight"}}
	#+END_SRC

	Entries are removed back to front so that the shifting deleting causes never reaches
	an index that has not been used yet, and every one is checked the way a single delete
	checks it. A refusal anywhere refuses the lot: half a fight deleted would be worse
	than none of it, and the =400= says which line was wrong.

	Doing it in one write is also what makes one press of undo put the whole block back.

	*Response:* The session, read back in full.
	EDOC */
func PostDndPlayBlockDelete(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dndBlockRequest
	if !dndBody(w, r, &req) {
		return
	}
	id := mux.Vars(r)["id"]
	if _, _, err := dndSessionText(id); err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	detail, err := DeleteDndBlock(id, req)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	dndJson(w, detail)
}

/* SDOC: API
* POST /dnd/play/session/{id}/mark — Annotate A Timeline Block
	Names one block of the timeline and says what happened there. The annotation is the
	one part of the timeline that is stored: everything else about a block is worked out
	from the logs each time it is drawn, and would go stale the moment a roll was
	corrected.

	*Method:* =POST=

	*Body:*
	| Field   | Type   | Description                                                     |
	|---------+--------+-----------------------------------------------------------------|
	| =time=  | string | The clock the block starts at (HH:MM). Required.                |
	| =kind=  | string | What sort of block - =fight=, =note=, =rolls=, =roll=.          |
	| =title= | string | What to call it. "Ambush at the ford".                          |
	| =note=  | string | Anything worth saying about it, org markup, as typed.           |

	The time and the kind together are the anchor, so annotating the same block twice
	rewrites the annotation rather than leaving two of them. One of =title= or =note= is
	required - an annotation with neither is a deletion, which is the =DELETE=.

	An annotation is written as an entry under a =* Timeline= heading in the session file,
	tagged with its kind, so it reads as ordinary org and can be edited there.

	An anchor that matches no block is not an error. The annotation is drawn as a beat of
	its own at the time it carries, which is what lets a moment nobody rolled for - the
	room, the door, who was waiting behind it - be written on the timeline at all.

	*Response:* The session, read back in full.
	EDOC */
func PostDndPlayMark(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dndMarkRequest
	if !dndBody(w, r, &req) {
		return
	}
	id := mux.Vars(r)["id"]
	if _, _, err := dndSessionText(id); err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	detail, err := SetDndMark(id, dnd.SessionMark{
		Time: req.Time, Kind: req.Kind, Title: req.Title, Note: req.Note})
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	dndJson(w, detail)
}

/* SDOC: API
* DELETE /dnd/play/session/{id}/mark — Take An Annotation Off
	Removes the annotation on one block of the timeline. The block itself is untouched -
	the rolls and notes it is made of stay exactly where they are.

	*Method:* =DELETE=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                  |
	|-----------+--------+----------+----------------------------------------------|
	| =time=    | string | yes      | The clock the annotation is anchored on.     |
	| =kind=    | string | no       | The kind it was written against.             |

	An anchor that carries no annotation is refused with a =400= rather than quietly
	doing nothing, so a page working from a stale reading hears about it.

	*Response:* The session, read back in full.
	EDOC */
func DeleteDndPlayMark(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	id := mux.Vars(r)["id"]
	if _, _, err := dndSessionText(id); err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	detail, err := DeleteDndMark(id, r.URL.Query().Get("time"),
		r.URL.Query().Get("kind"))
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	dndJson(w, detail)
}

/* SDOC: API
* GET /dnd/play/session/{id}/undo — What Undo Would Take Back In This Session
	Answers what the last change to one session file was, and whether it can be reversed.
	Nothing is changed; this is what the timeline asks so its Undo offer can say what
	pressing it will do.

	This is the character sheet's undo journal narrowed to one file. The sheet's own Undo
	button takes back the last thing that happened anywhere; a drawer showing one evening
	should offer to take back the last thing that happened to that evening, so that
	deleting a block and then rolling a die does not leave the timeline offering to
	un-roll the die.

	*Method:* =GET=

	*Response:* An =UndoPlan=, =can= false with a =why= when there is nothing to take back.

	The journal is memory only and holds the last thirty changes this server made, so a
	restart empties it. There is no history to fall back on here the way there is for a
	character sheet: a session file records what happened at the table, and nothing in it
	is derived from anything that could be worked backwards.
	EDOC */
func RequestDndPlayUndo(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	_, plan, err := dndSessionUndoPlan(mux.Vars(r)["id"])
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, plan)
}

// DndSessionUndoState is a session read back with the undo on top, so the
// timeline redraws and relabels its button from one answer.
type DndSessionUndoState struct {
	dnd.SessionDetail
	// Undone is what was just taken back, and Undo what pressing it again
	// would do.
	Undone dnd.UndoPlan `json:"undone"`
	Undo   dnd.UndoPlan `json:"undo"`
}

/* SDOC: API
* POST /dnd/play/session/{id}/undo — Take The Last Change To This Session Back
	Puts one session file back as it stood before the last change this server made to it.
	A whole timeline block thrown away comes back in one press, because throwing it away
	was one write.

	*Method:* =POST=

	*Response:* The session read back in full, with =undone= saying what was taken back
	and =undo= saying what pressing it again would do.

	Only the newest change to that file is ever offered, and only while the file is still
	exactly as that change left it. Anything else - another window, the command line, a
	text editor - and putting the old text back would throw that away, so the call is
	refused with a =400= rather than guessing.
	EDOC */
func PostDndPlayUndo(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	id := mux.Vars(r)["id"]
	dndLogLock.Lock()
	ch, plan, err := dndSessionUndoPlan(id)
	if err != nil {
		dndLogLock.Unlock()
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	if !plan.Can {
		dndLogLock.Unlock()
		dndError(w, http.StatusBadRequest, "%s", plan.Why)
		return
	}
	if err := dndPutBack(ch); err != nil {
		dndLogLock.Unlock()
		dndError(w, http.StatusInternalServerError,
			"could not put %s back: %s", ch.Path, err)
		return
	}
	dndDropSeq(ch.Seq)
	dndLogLock.Unlock()

	detail, err := GetDndSession(id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	_, next, _ := dndSessionUndoPlan(id)
	dndJson(w, DndSessionUndoState{SessionDetail: *detail, Undone: plan, Undo: next})
}
