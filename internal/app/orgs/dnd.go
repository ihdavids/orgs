//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	dndplug "github.com/ihdavids/orgs/internal/app/orgs/plugs/dnd"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/dnd"
)

// ----------------------------------------------------------------------------
// Session store
// ----------------------------------------------------------------------------

// dndSessionTTL is how long an abandoned character build is kept around.
const dndSessionTTL = 8 * time.Hour

type dndSessionStore struct {
	lock     sync.Mutex
	sessions map[string]*dnd.Session
}

var dndSessions = &dndSessionStore{sessions: map[string]*dnd.Session{}}

func (s *dndSessionStore) put(sess *dnd.Session) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.expireLocked()
	s.sessions[sess.Id] = sess
}

func (s *dndSessionStore) get(id, owner string) (*dnd.Session, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("no character build session %q, it may have expired", id)
	}
	if sess.Owner != "" && owner != "" && sess.Owner != owner {
		return nil, fmt.Errorf("session %q belongs to another user", id)
	}
	return sess, nil
}

func (s *dndSessionStore) drop(id string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.sessions, id)
}

func (s *dndSessionStore) expireLocked() {
	cutoff := time.Now().Add(-dndSessionTTL)
	for id, sess := range s.sessions {
		if sess.Updated.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
}

// DndLibrary returns the ruleset library, configuring its search path from
// the server settings the first time it is needed.
func DndLibrary() *dnd.Library {
	if len(dndplug.Paths()) == 0 {
		paths := []string{}
		if Conf().Server != nil {
			paths = append(paths, Conf().Server.DndPaths...)
			paths = append(paths, dndplug.DefaultPaths(Conf().Server.TemplatePath)...)
		} else {
			paths = dndplug.DefaultPaths("")
		}
		dndplug.SetPaths(paths)
	}
	return dndplug.Library()
}

func dndRuleset(id string) *dnd.Ruleset {
	return DndLibrary().Get(id)
}

func dndRulesetOptions() []dnd.Option {
	opts := []dnd.Option{}
	for _, info := range DndLibrary().List() {
		opts = append(opts, dnd.Option{
			Id: info.Id, Name: info.Name, Summary: info.Description,
			Meta: map[string]string{
				"races":   fmt.Sprintf("%d", info.Races),
				"classes": fmt.Sprintf("%d", info.Classes),
				"spells":  fmt.Sprintf("%d", info.Spells),
			},
			Recommended: info.Id == dnd.DefaultRuleset,
		})
	}
	return opts
}

func dndError(w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: fmt.Sprintf(format, args...)})
}

func dndJson(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func dndBody[T any](w http.ResponseWriter, r *http.Request, out *T) bool {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		dndError(w, http.StatusBadRequest, "could not read request: %s", err)
		return false
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return true
	}
	if err := json.Unmarshal(body, out); err != nil {
		dndError(w, http.StatusBadRequest, "invalid json: %s", err)
		return false
	}
	return true
}

// ----------------------------------------------------------------------------
// Handlers
// ----------------------------------------------------------------------------

/* SDOC: API
* GET /dnd/rulesets — List Loaded D&D Rulesets
	Returns every ruleset the dnd module has loaded, including the built in SRD data and
	any add on yaml modules found on the configured =dndPaths=.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* A JSON array of ruleset summaries:
	#+BEGIN_SRC json
	[
	  {
	    "id": "srd",
	    "name": "SRD 5.1 Core Rules",
	    "version": "5.1",
	    "modules": ["builtin:srd-core.yaml", "/Users/me/dnd/homebrew.yaml"],
	    "races": 9, "classes": 12, "backgrounds": 13, "spells": 319, "items": 201
	  }
	]
	#+END_SRC
	EDOC */
func RequestDndRulesets(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	dndJson(w, DndLibrary().List())
}

/* SDOC: API
* POST /dnd/reload — Reload D&D Ruleset Modules
	Rereads every yaml module on the dnd search path. Use this after editing homebrew
	content instead of restarting the server.

	*Method:* =POST=

	*Response:* The refreshed ruleset list, exactly like =GET /dnd/rulesets=.
	EDOC */
func PostDndReload(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	dndplug.SetPaths(nil)
	DndLibrary()
	dndplug.Reload()
	dndJson(w, DndLibrary().List())
}

/* SDOC: API
* GET /dnd/catalog — Browse Ruleset Content
	Lists the valid choices of one kind from a ruleset, with the summary and detail text
	the interactive builder shows. This is what powers "what are my options?" in a client.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                                                    |
	|------------+--------+----------+--------------------------------------------------------------------------------|
	| =kind=     | string | yes      | races, classes, subclasses, backgrounds, spells, items, feats, skills, languages |
	| =ruleset=  | string | no       | Ruleset id, defaults to =srd=.                                                  |
	| =id=       | string | no       | For =subclasses= the class id, for spells the class to filter by.               |
	| =filter=   | string | no       | Case insensitive substring match on the name.                                   |

	*Response:* =CatalogResponse=, an array of options each with =id=, =name=, =summary=,
	=detail=, =tags= and =meta=.
	EDOC */
func RequestDndCatalog(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	kind := strings.ToLower(r.URL.Query().Get("kind"))
	rsId := r.URL.Query().Get("ruleset")
	id := r.URL.Query().Get("id")
	filter := strings.ToLower(r.URL.Query().Get("filter"))
	rs := dndRuleset(rsId)
	if rs == nil {
		dndError(w, http.StatusNotFound, "no ruleset %q", rsId)
		return
	}
	resp := dnd.CatalogResponse{Ruleset: rs.Id, Kind: kind, Options: []dnd.Option{}}
	add := func(o dnd.Option) {
		if filter != "" && !strings.Contains(strings.ToLower(o.Name), filter) &&
			!strings.Contains(strings.ToLower(o.Id), filter) {
			return
		}
		resp.Options = append(resp.Options, o)
	}
	switch kind {
	case "", "rulesets":
		resp.Kind = "rulesets"
		for _, o := range dndRulesetOptions() {
			add(o)
		}
	case "races":
		for i := range rs.Races {
			race := &rs.Races[i]
			add(dnd.Option{Id: race.Id, Name: race.Name, Summary: race.Summary,
				Detail: race.Text, Meta: map[string]string{
					"speed": fmt.Sprintf("%d", race.Speed), "size": race.Size}})
			for j := range race.Subraces {
				sr := &race.Subraces[j]
				add(dnd.Option{Id: sr.Id, Name: race.Name + ": " + sr.Name,
					Summary: sr.Summary, Detail: sr.Text})
			}
		}
	case "classes":
		for i := range rs.Classes {
			c := &rs.Classes[i]
			add(dnd.Option{Id: c.Id, Name: c.Name, Summary: c.Summary, Detail: c.Text,
				Meta: map[string]string{"hitDie": fmt.Sprintf("d%d", c.HitDie)}})
		}
	case "subclasses":
		c := rs.Class(id)
		if c == nil {
			dndError(w, http.StatusNotFound, "no class %q, pass ?id=<class>", id)
			return
		}
		for i := range c.Subclasses {
			sc := &c.Subclasses[i]
			add(dnd.Option{Id: sc.Id, Name: sc.Name, Summary: sc.Summary, Detail: sc.Text})
		}
	case "backgrounds":
		for i := range rs.Backgrounds {
			b := &rs.Backgrounds[i]
			add(dnd.Option{Id: b.Id, Name: b.Name, Summary: b.Summary, Detail: b.Text})
		}
	case "spells":
		for _, sp := range rs.SpellsForClass(id, -1) {
			add(dnd.Option{Id: sp.Id, Name: sp.Name, Summary: sp.LevelString(),
				Detail: sp.Text, Tags: sp.Classes,
				Meta: map[string]string{
					"level": fmt.Sprintf("%d", sp.Level), "school": sp.School,
					"time": sp.CastingTime, "range": sp.Range, "duration": sp.Duration,
					"components": sp.Components}})
		}
	case "items":
		for i := range rs.Items {
			it := &rs.Items[i]
			add(dnd.Option{Id: it.Id, Name: it.Name, Summary: it.Category, Detail: it.Text,
				Meta: map[string]string{"cost": it.Cost, "damage": it.Damage,
					"weight": fmt.Sprintf("%v", it.Weight)}})
		}
	case "feats":
		for i := range rs.Feats {
			f := &rs.Feats[i]
			add(dnd.Option{Id: f.Id, Name: f.Name, Summary: f.Prerequisite, Detail: f.Text})
		}
	case "skills":
		for i := range rs.Skills {
			s := &rs.Skills[i]
			add(dnd.Option{Id: s.Id, Name: s.Name, Summary: dnd.AbilityNames[s.Ability],
				Detail: s.Text})
		}
	case "languages":
		for _, l := range rs.AllLanguages() {
			add(dnd.Option{Id: l, Name: l})
		}
	case "alignments":
		for _, a := range rs.AllAlignments() {
			add(dnd.Option{Id: a, Name: a})
		}
	default:
		dndError(w, http.StatusBadRequest, "unknown catalog kind %q", kind)
		return
	}
	sort.SliceStable(resp.Options, func(i, j int) bool {
		return resp.Options[i].Name < resp.Options[j].Name
	})
	dndJson(w, resp)
}

/* SDOC: API
* POST /dnd/session — Start Interactive Character Creation
	Opens a character build session and returns the first prompt. Every prompt carries the
	full list of valid choices, a short summary and a detail blurb for each one, plus advice
	based on what has been picked so far, so a client needs no rules knowledge of its own.

	*Method:* =POST=

	*Body:* =NewSessionRequest=
	#+BEGIN_SRC json
	{"ruleset": "srd", "name": "Lyra", "player": "Ian", "level": 3, "filename": "lyra.org"}
	#+END_SRC

	*Response:* A =Prompt=:
	#+BEGIN_SRC json
	{
	  "session": "6f1c...", "step": "class", "kind": "select",
	  "title": "Class", "question": "Choose your class.",
	  "options": [{"id": "wizard", "name": "Wizard", "summary": "...", "recommended": false}],
	  "progress": {"step": 2, "total": 24}
	}
	#+END_SRC

	The =kind= field tells the client what widget to use: =select=, =multiselect=, =text=,
	=longtext=, =number=, =abilities=, =fields= or =done=.
	EDOC */
func PostDndSession(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.NewSessionRequest
	if !dndBody(w, r, &req) {
		return
	}
	rs := dndRuleset(req.Ruleset)
	if rs == nil {
		dndError(w, http.StatusNotFound, "no ruleset %q", req.Ruleset)
		return
	}
	sess := dnd.NewSession(uuid.New().String(), &req, dndRulesetOptions())
	sess.Owner = GetUsername(r)
	sess.Ruleset = rs.Id
	dndSessions.put(sess)
	dndJson(w, sess.Next(rs))
}

/* SDOC: API
* GET /dnd/session/{id} — Current Character Creation Prompt
	Returns the prompt the session is waiting on, which is how a client resumes an
	interrupted character build.

	*Method:* =GET=

	*Response:* A =Prompt=, or 404 when the session has expired.
	EDOC */
func RequestDndSession(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	id := mux.Vars(r)["id"]
	sess, err := dndSessions.get(id, GetUsername(r))
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, sess.Next(dndRuleset(sess.Ruleset)))
}

/* SDOC: API
* DELETE /dnd/session/{id} — Abandon a Character Build
	Throws the in progress character away.

	*Method:* =DELETE=
	EDOC */
func DeleteDndSession(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	id := mux.Vars(r)["id"]
	dndSessions.drop(id)
	dndJson(w, common.ResultMsg{Ok: true, Msg: "session dropped"})
}

/* SDOC: API
* POST /dnd/answer — Answer a Character Creation Prompt
	Applies an answer and returns the next prompt. Answers are validated against the
	ruleset, so an invalid pick comes back as an error rather than a broken character.

	*Method:* =POST=

	*Body:* =Answer=
	| Field     | Type              | Description                                                  |
	|-----------+-------------------+--------------------------------------------------------------|
	| =session= | string            | Session id from =POST /dnd/session=.                          |
	| =step=    | string            | The step being answered.                                      |
	| =values=  | []string          | Chosen option ids (one for select, several for multiselect).  |
	| =text=    | string            | Free text for text/longtext prompts.                          |
	| =numbers= | map[string]int    | Ability scores for the =abilities= prompt.                    |
	| =random=  | bool              | Let the server roll or pick this step for you.                |
	| =skip=    | bool              | Skip an optional step.                                        |
	| =back=    | bool              | Step backwards, discarding later answers.                     |

	*Response:* The next =Prompt=. When the character is finished the prompt has
	=done: true= and carries the computed =sheet= plus the rendered =org= text.
	EDOC */
func PostDndAnswer(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var ans dnd.Answer
	if !dndBody(w, r, &ans) {
		return
	}
	sess, err := dndSessions.get(ans.Session, GetUsername(r))
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	rs := dndRuleset(sess.Ruleset)
	prompt, err := sess.Apply(rs, &ans)
	if err != nil {
		// Not a server error: hand the problem back with the prompt so the
		// player can correct their answer.
		p := sess.Next(rs)
		p.Error = err.Error()
		w.WriteHeader(http.StatusBadRequest)
		dndJson(w, p)
		return
	}
	dndJson(w, prompt)
}

/* SDOC: API
* POST /dnd/random — Roll a Complete Character
	Generates a finished character without any interaction. Handy for NPCs, for filling a
	table with pregenerated characters, or for seeing what the module can do.

	*Method:* =POST=

	*Body:* =NewSessionRequest=, all fields optional:
	#+BEGIN_SRC json
	{"ruleset": "srd", "level": 5, "name": "Bandit Captain"}
	#+END_SRC

	*Response:* =SaveResponse= shaped object containing the rendered =org= text, with the
	computed sheet under =sheet=.
	EDOC */
func PostDndRandom(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.NewSessionRequest
	if !dndBody(w, r, &req) {
		return
	}
	rs := dndRuleset(req.Ruleset)
	if rs == nil {
		dndError(w, http.StatusNotFound, "no ruleset %q", req.Ruleset)
		return
	}
	c, err := dnd.RandomCharacter(rs, &req)
	if err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	dndJson(w, map[string]interface{}{
		"ok":        true,
		"character": c,
		"sheet":     dnd.Compute(c, rs),
		"org":       dnd.RenderOrg(c, rs),
	})
}

/* SDOC: API
* POST /dnd/save — Write a Character Sheet to an Org File
	Writes a finished character out as an org file and loads it into the org database.
	Either pass a =session= id (the character being built) or a complete =character=.

	*Method:* =POST=

	*Body:* =SaveRequest=
	#+BEGIN_SRC json
	{"session": "6f1c...", "filename": "characters/lyra.org", "overwrite": false}
	#+END_SRC

	A relative filename is resolved against the first configured =orgDirs= entry. Existing
	files are never clobbered unless =overwrite= is true.

	*Response:* =SaveResponse= with the absolute =filename= that was written.
	EDOC */
func PostDndSave(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.SaveRequest
	if !dndBody(w, r, &req) {
		return
	}
	var c *dnd.Character
	rsId := ""
	if req.Session != "" {
		sess, err := dndSessions.get(req.Session, GetUsername(r))
		if err != nil {
			dndError(w, http.StatusNotFound, "%s", err)
			return
		}
		rsId = sess.Ruleset
		c = sess.Finish(dndRuleset(rsId))
		if req.Filename == "" {
			req.Filename = sess.Filename
		}
	} else if req.Character != nil {
		c = req.Character
		rsId = c.Ruleset
	} else {
		dndError(w, http.StatusBadRequest, "pass either a session id or a character")
		return
	}
	rs := dndRuleset(rsId)
	fname, err := dndResolveFilename(req.Filename, c.Name)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if _, err := os.Stat(fname); err == nil && !req.Overwrite {
		dndError(w, http.StatusConflict, "%s already exists, pass overwrite to replace it", fname)
		return
	}
	org := dnd.RenderOrg(c, rs)
	if err := os.MkdirAll(filepath.Dir(fname), 0755); err != nil {
		dndError(w, http.StatusInternalServerError, "could not create directory: %s", err)
		return
	}
	if err := os.WriteFile(fname, []byte(org), 0644); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", fname, err)
		return
	}
	GetDb().ReloadFile(fname)
	if req.Session != "" {
		dndSessions.drop(req.Session)
	}
	dndJson(w, dnd.SaveResponse{Ok: true, Filename: fname, Org: org,
		Msg: fmt.Sprintf("wrote %s", fname)})
}

// dndResolveFilename turns a user supplied name into an absolute org path
// inside one of the configured org directories.
func dndResolveFilename(name, charName string) (string, error) {
	if strings.TrimSpace(name) == "" {
		if strings.TrimSpace(charName) == "" {
			return "", fmt.Errorf("a filename is required")
		}
		name = dnd.Slugify(charName) + ".org"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".org") {
		name += ".org"
	}
	if filepath.IsAbs(name) {
		return name, nil
	}
	dirs := []string{}
	if Conf().Server != nil {
		dirs = Conf().Server.OrgDirs
	}
	if len(dirs) == 0 {
		return filepath.Abs(name)
	}
	return filepath.Join(dirs[0], name), nil
}

/* SDOC: API
* GET /dnd/sheet — Computed Character Sheet
	Parses an org character sheet and returns everything derived from it: ability
	modifiers, saving throws, all eighteen skills, armour class, initiative, hit dice,
	attacks, spell save DC, spell slots, carrying capacity and the collected feature text.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                  |
	|------------+--------+----------+----------------------------------------------|
	| =filename= | string | yes      | The org character sheet (basename or path).  |

	*Response:* A =Sheet= object. Anything the engine could not resolve (an unknown class
	id for example) is reported in the =warnings= array rather than failing the request.
	EDOC */
func RequestDndSheet(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	fname := r.URL.Query().Get("filename")
	if fname == "" {
		dndError(w, http.StatusBadRequest, "filename is required")
		return
	}
	c, rs, err := dndplug.LoadCharacter(db, fname)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, dnd.Compute(c, rs))
}

/* SDOC: API
* POST /dnd/refresh — Regenerate the Derived Sections of a Sheet
	Reads a character sheet, recomputes everything and writes it back. Use this after
	editing the property drawer by hand: change =DND_CLASSES= to =wizard:evocation:4= and
	refresh, and the skills, attacks, hit points, spell slots and feature list all catch up.

	*Method:* =POST=

	*Body:*
	#+BEGIN_SRC json
	{"filename": "lyra.org"}
	#+END_SRC

	*Response:* =SaveResponse= containing the rewritten org text.
	EDOC */
func PostDndRefresh(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.SaveRequest
	if !dndBody(w, r, &req) {
		return
	}
	if req.Filename == "" {
		dndError(w, http.StatusBadRequest, "filename is required")
		return
	}
	c, rs, err := dndplug.LoadCharacter(db, req.Filename)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	f := GetDb().GetFile(req.Filename)
	fname := req.Filename
	if f != nil && f.Filename != "" {
		fname = f.Filename
	}
	org := dnd.RenderOrg(c, rs)
	if err := os.WriteFile(fname, []byte(org), 0644); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", fname, err)
		return
	}
	GetDb().ReloadFile(fname)
	dndJson(w, dnd.SaveResponse{Ok: true, Filename: fname, Org: org,
		Msg: fmt.Sprintf("refreshed %s", fname)})
}

/* SDOC: API
* GET /dnd/characters — List Known Character Sheets
	Scans the org database for files tagged as dnd character sheets and returns a one line
	summary of each.

	*Method:* =GET=

	*Response:*
	#+BEGIN_SRC json
	[{"filename": "/gtd/lyra.org", "name": "Lyra", "race": "High Elf",
	  "class": "Wizard (School of Evocation) 3", "level": 3, "ac": 12, "hp": 20}]
	#+END_SRC
	EDOC */
func RequestDndCharacters(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	out := []map[string]interface{}{}
	for _, fname := range GetDb().GetFiles() {
		data, err := os.ReadFile(fname)
		if err != nil || !strings.Contains(string(data), "DND_CLASSES") {
			continue
		}
		c, rs, err := dndplug.ParseCharacter(string(data))
		if err != nil {
			continue
		}
		s := dnd.Compute(c, rs)
		out = append(out, map[string]interface{}{
			"filename": fname, "name": s.Name, "race": s.RaceName, "class": s.ClassLine,
			"level": s.Level, "ac": s.AC, "hp": s.HPMax, "ruleset": c.Ruleset,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprintf("%v", out[i]["name"]) < fmt.Sprintf("%v", out[j]["name"])
	})
	dndJson(w, out)
}
