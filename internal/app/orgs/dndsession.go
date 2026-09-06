//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common/dnd"
)

// ----------------------------------------------------------------------------
// Play session logs
//
// A play session is a dated org file in the dndSessionPath folder. The html
// character sheet posts rolls and notes to it while you play. Everything is
// stored in the file itself, there is no server side state to lose, so a
// session survives a restart and can be picked back up from any sheet.
// ----------------------------------------------------------------------------

// dndLogLock serialises the read/modify/write cycle on session files.
var dndLogLock sync.Mutex

// dndSessionDir resolves the session folder, creating it on first use.
func dndSessionDir() (string, error) {
	p := ""
	root := "."
	if Conf().Server != nil {
		p = strings.TrimSpace(Conf().Server.DndSessionPath)
		if len(Conf().Server.OrgDirs) > 0 {
			root = Conf().Server.OrgDirs[0]
		}
	}
	if p == "" {
		p = "dndsessions"
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	if err := os.MkdirAll(p, 0755); err != nil {
		return "", fmt.Errorf("could not create the dnd session folder %s: %s", p, err)
	}
	return p, nil
}

// dndSessionFile maps a session id onto its file, refusing anything that
// would escape the session folder.
func dndSessionFile(id string) (string, error) {
	if !dnd.ValidSessionId(id) {
		return "", fmt.Errorf("%q is not a valid session id", id)
	}
	dir, err := dndSessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".org"), nil
}

func dndSessionId(file string) string {
	return strings.TrimSuffix(filepath.Base(file), ".org")
}

// dndSessionText reads a session file.
func dndSessionText(id string) (string, string, error) {
	file, err := dndSessionFile(id)
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", file, fmt.Errorf("no session %q: %s", id, err)
	}
	return string(data), file, nil
}

// renderDndSession expands the session template. The file template wins when
// there is one, otherwise the built in default is used so that a fresh
// install still works.
func renderDndSession(name, summary string, dt time.Time) string {
	ctx := map[string]interface{}{
		"session_title":   dnd.SessionTitle(name, dt),
		"session_name":    strings.TrimSpace(name),
		"session_summary": strings.TrimSpace(summary),
		"org_date":        dt.Format("[2006-01-02 Mon]"),
		"date":            dt.Format("2006-01-02"),
		"weekday":         dt.Format("Mon"),
		"day":             fmt.Sprintf("%d", dt.Day()),
		"month":           fmt.Sprintf("%d", dt.Month()),
		"year":            fmt.Sprintf("%d", dt.Year()),
	}
	tempo := Conf().PlugManager.Tempo
	tpl := "dndsession.tpl"
	if Conf().Server != nil && Conf().Server.DndSessionTemplate != "" {
		tpl = Conf().Server.DndSessionTemplate
	}
	if path := tempo.ExpandTemplatePath(tpl); path != "" {
		if _, err := os.Stat(path); err == nil {
			return tempo.RenderTemplate(tpl, ctx)
		}
	}
	return tempo.RenderTemplateString(dnd.DefaultSessionTemplate, ctx)
}

// CreateDndSession starts (or reopens) a play session. Two sessions on the
// same day with the same name are the same file, so reloading a sheet mid
// game continues the log rather than starting a second one.
func CreateDndSession(name, summary string, dt time.Time, ch dnd.SessionCharacter) (*dnd.SessionInfo, error) {
	dir, err := dndSessionDir()
	if err != nil {
		return nil, err
	}
	dndLogLock.Lock()
	defer dndLogLock.Unlock()

	file := filepath.Join(dir, dnd.SessionFileName(name, dt))
	text := ""
	if data, err := os.ReadFile(file); err != nil {
		text = renderDndSession(name, summary, dt)
	} else {
		text = string(data)
		if strings.TrimSpace(summary) != "" {
			text = dnd.SetSessionSummary(text, summary)
		}
	}
	text = dnd.AddSessionCharacter(text, ch)
	if err := os.WriteFile(file, []byte(text), 0644); err != nil {
		return nil, fmt.Errorf("could not write session file %s: %s", file, err)
	}
	info := dnd.SessionInfoFromText(dndSessionId(file), file, text)
	return &info, nil
}

// ListDndSessions returns session files, newest first.
//
// A character id (or, for session files written before ids were recorded, a
// character name) narrows the list to the sessions that character played in,
// which is what a character sheet asks for: one player's sheet has no business
// listing the rest of the table's games. Both empty returns everything, which
// is what the search and the CLI want.
//
// A session that records no cast at all is kept either way. It names nobody,
// so it belongs to nobody in particular and excludes nobody either.
func ListDndSessions(character, name string) ([]dnd.SessionInfo, error) {
	dir, err := dndSessionDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []dnd.SessionInfo{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".org") {
			continue
		}
		file := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		info := dnd.SessionInfoFromText(dndSessionId(file), file, string(data))
		if !dndSessionWanted(&info, character, name) {
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].Id > out[j].Id
	})
	return out, nil
}

// dndSessionWanted applies the character filter for ListDndSessions.
func dndSessionWanted(info *dnd.SessionInfo, character, name string) bool {
	if strings.TrimSpace(character) == "" && strings.TrimSpace(name) == "" {
		return true
	}
	if len(info.Characters) == 0 {
		return true
	}
	return info.PlayedBy(character, name)
}

// GetDndSession reads one whole session back.
func GetDndSession(id string) (*dnd.SessionDetail, error) {
	text, file, err := dndSessionText(id)
	if err != nil {
		return nil, err
	}
	d := dnd.SessionDetailFromText(id, file, text)
	return &d, nil
}

// updateDndSession applies a change to a session file under the log lock.
func updateDndSession(id string, change func(string) string) (*dnd.SessionInfo, error) {
	dndLogLock.Lock()
	defer dndLogLock.Unlock()
	text, file, err := dndSessionText(id)
	if err != nil {
		return nil, err
	}
	out := change(text)
	if out != text {
		if err := os.WriteFile(file, []byte(out), 0644); err != nil {
			return nil, fmt.Errorf("could not write session file %s: %s", file, err)
		}
	}
	info := dnd.SessionInfoFromText(id, file, out)
	return &info, nil
}

// AppendDndRolls records dice rolls against a session.
func AppendDndRolls(id string, rolls []dnd.SessionRoll, ch dnd.SessionCharacter) (*dnd.SessionInfo, error) {
	now := time.Now().Format(dnd.SessionNoteTimeFormat)
	for i := range rolls {
		if strings.TrimSpace(rolls[i].Time) == "" {
			rolls[i].Time = now
		}
		if strings.TrimSpace(rolls[i].Character) == "" {
			rolls[i].Character = ch.Name
		}
	}
	return updateDndSession(id, func(text string) string {
		return dnd.AppendRolls(dnd.AddSessionCharacter(text, ch), rolls)
	})
}

// AppendDndNotes records notes against a session.
func AppendDndNotes(id string, notes []dnd.SessionNote, ch dnd.SessionCharacter) (*dnd.SessionInfo, error) {
	now := time.Now().Format(dnd.SessionNoteTimeFormat)
	for i := range notes {
		if strings.TrimSpace(notes[i].Time) == "" {
			notes[i].Time = now
		}
	}
	return updateDndSession(id, func(text string) string {
		return dnd.AppendNotes(dnd.AddSessionCharacter(text, ch), notes)
	})
}

// SearchDndSessions greps every session file for a term.
func SearchDndSessions(term string, limit int) ([]dnd.SessionMatch, error) {
	sessions, err := ListDndSessions("", "")
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	out := []dnd.SessionMatch{}
	for _, info := range sessions {
		data, err := os.ReadFile(info.File)
		if err != nil {
			continue
		}
		hits := dnd.SearchSession(info, string(data), term, limit-len(out))
		out = append(out, hits...)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ----------------------------------------------------------------------------
// Handlers
// ----------------------------------------------------------------------------

type dndSessionRequest struct {
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	Date        string `json:"date"`
	Character   string `json:"character"`
	CharacterId string `json:"characterId"`
}

type dndLogRequest struct {
	Character   string            `json:"character"`
	CharacterId string            `json:"characterId"`
	Rolls       []dnd.SessionRoll `json:"rolls"`
	Notes       []dnd.SessionNote `json:"notes"`
}

// character is who the request says is playing.
func (r *dndLogRequest) character() dnd.SessionCharacter {
	return dnd.SessionCharacter{Id: r.CharacterId, Name: r.Character}
}

/*
		SDOC: API

	  - POST /dnd/play/session — Start Or Reopen A Play Session
	    Creates a dated org file in the configured =dndSessionPath= and returns the session
	    it represents. Starting a session that already exists (same day, same name) reopens
	    it instead of creating a second file, so reloading a character sheet mid game
	    continues the same log.

	    *Method:* =POST=

	    *Body:*
	    | Field         | Type   | Required | Description                                              |
	    |---------------+--------+----------+----------------------------------------------------------|
	    | =name=        | string | no       | Session name. Without one the file is named for the date. |
	    | =summary=     | string | no       | One line summary, stored as =#+SUMMARY:=.                 |
	    | =date=        | string | no       | =YYYY-MM-DD=, defaults to today.                          |
	    | =character=   | string | no       | Character name, added to =#+CHARACTERS:= and the =* Characters= section. |
	    | =characterId= | string | no       | The character's =DND_ID=, stamped on their heading in the session file. |

	    *Response:* A session record:
	    #+BEGIN_SRC json
	    {
	    "id": "2025_09_05_Goblin_Ambush",
	    "name": "Goblin Ambush",
	    "date": "2025-09-05",
	    "file": "/Users/me/dev/gtd/dndsessions/2025_09_05_Goblin_Ambush.org",
	    "summary": "Ambushed on the road to Phandalin.",
	    "rolls": 0, "notes": 0
	    }
	    #+END_SRC
	    EDOC
*/
func PostDndPlaySession(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dndSessionRequest
	if !dndBody(w, r, &req) {
		return
	}
	dt := time.Now()
	if strings.TrimSpace(req.Date) != "" {
		if parsed, err := time.Parse("2006-01-02", strings.TrimSpace(req.Date)); err == nil {
			dt = parsed
		} else {
			dndError(w, http.StatusBadRequest, "could not read date %q, expected YYYY-MM-DD", req.Date)
			return
		}
	}
	info, err := CreateDndSession(req.Name, req.Summary, dt,
		dnd.SessionCharacter{Id: req.CharacterId, Name: req.Character})
	if err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	dndJson(w, info)
}

/*
		SDOC: API

	  - GET /dnd/play/sessions — List Play Sessions
	    Lists play session logs, newest first, each with the one line summary shown
	    beside it in the session picker on a character sheet.

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter    | Type   | Description                                                   |
	    |--------------+--------+---------------------------------------------------------------|
	    | =character=  | string | Only sessions this character id played in. Omit for all.       |
	    | =name=       | string | Character name, used for sessions logged before ids existed.   |

	    A character sheet passes its own =DND_ID= so that it lists its own games
	    rather than the whole table's. Sessions that record no cast are always listed.

	    *Response:* An array of session records, exactly like =POST /dnd/play/session= returns.
	    EDOC
*/
func RequestDndPlaySessions(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	q := r.URL.Query()
	list, err := ListDndSessions(q.Get("character"), q.Get("name"))
	if err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	dndJson(w, list)
}

/*
		SDOC: API

	  - GET /dnd/play/session/{id} — Read One Play Session
	    Returns everything recorded in a session: the roll table and every note, parsed
	    back out of the org file. This is what the sheet shows when you pick a session
	    out of the list.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                          |
	    |-----------+--------+------------------------------------------------------|
	    | ={id}=    | string | Session id, which is the file name without the .org. |

	    *Response:* The session record plus =rollLog= and =noteLog= arrays.
	    EDOC
*/
func RequestDndPlaySession(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	id := mux.Vars(r)["id"]
	detail, err := GetDndSession(id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, detail)
}

/*
		SDOC: API

	  - POST /dnd/play/session/{id}/roll — Record Dice Rolls
	    Appends rows to the roll table of a session file. The html character sheet calls
	    this for every roll while a session is running, batching anything it could not
	    send at the time.

	    *Method:* =POST=

	    *Body:*
	    | Field         | Type   | Description                                              |
	    |---------------+--------+----------------------------------------------------------|
	    | =character=   | string | Name used for rolls that do not carry one.                |
	    | =characterId= | string | The character's =DND_ID=, so the session records who was there. |
	    | =rolls=       | array  | Rolls, each =time=, =character=, =label=, =formula=, =result=, =dice=, =notes=. |

	    *Response:* The updated session record.
	    EDOC
*/
func PostDndPlayRoll(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dndLogRequest
	if !dndBody(w, r, &req) {
		return
	}
	info, err := AppendDndRolls(mux.Vars(r)["id"], req.Rolls, req.character())
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, info)
}

/*
		SDOC: API

	  - POST /dnd/play/session/{id}/note — Record Session Notes
	    Appends notes to the =* Notes= section of a session file. Note text is org markup
	    and is stored as typed: headings inside a note are pushed down so that they nest
	    under the timestamped entry, everything else is left alone.

	    *Method:* =POST=

	    *Body:*
	    | Field         | Type   | Description                                   |
	    |---------------+--------+-----------------------------------------------|
	    | =notes=       | array  | Notes, each with =time= (HH:MM) and =text=.   |
	    | =character=   | string | Character name, recorded in the session file. |
	    | =characterId= | string | The character's =DND_ID=.                     |

	    *Response:* The updated session record.
	    EDOC
*/
func PostDndPlayNote(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dndLogRequest
	if !dndBody(w, r, &req) {
		return
	}
	info, err := AppendDndNotes(mux.Vars(r)["id"], req.Notes, req.character())
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, info)
}

/*
		SDOC: API

	  - POST /dnd/play/session/{id}/summary — Set A Session Summary
	    Rewrites the =#+SUMMARY:= line of a session file, which is the one line
	    description shown beside the session in the session list.

	    *Method:* =POST=

	    *Body:* ={"summary": "Ambushed on the road to Phandalin."}=

	    *Response:* The updated session record.
	    EDOC
*/
func PostDndPlaySummary(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dndSessionRequest
	if !dndBody(w, r, &req) {
		return
	}
	info, err := updateDndSession(mux.Vars(r)["id"], func(text string) string {
		return dnd.SetSessionSummary(text, req.Summary)
	})
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, info)
}

/*
		SDOC: API

	  - GET /dnd/play/search — Search Every Play Session
	    Searches all session logs for a term and reports each hit with the heading it sits
	    under, so a result reads like "Goblin Ambush - Notes: the tracks leave the road".

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter | Type   | Required | Description                          |
	    |-----------+--------+----------+--------------------------------------|
	    | =q=       | string | yes      | Case insensitive substring to find.  |
	    | =limit=   | int    | no       | Maximum hits to return, default 100. |

	    *Response:* An array of matches with =id=, =name=, =date=, =line=, =text=,
	    =context= and =kind= (=note= or =roll=).
	    EDOC
*/
func RequestDndPlaySearch(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	term := r.URL.Query().Get("q")
	if strings.TrimSpace(term) == "" {
		dndJson(w, []dnd.SessionMatch{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	hits, err := SearchDndSessions(term, limit)
	if err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	dndJson(w, hits)
}
