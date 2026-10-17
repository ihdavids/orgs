package orgs

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	dndplug "github.com/ihdavids/orgs/internal/app/orgs/plugs/dnd"
	"github.com/ihdavids/orgs/internal/common/dnd"
	"github.com/ihdavids/orgs/internal/common/dnd/randgen"
)

// The initiative tracker's server half: the monster list and its stat
// blocks, and the party - kept in an org file of the player's choosing, a
// heading per character, so it can be read and edited like any other notes.
// The fight itself is kept with the dungeon (dungeonState.Combat, in
// dnddungeon.go); worg runs it.

type monsterSummary struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	CR      string `json:"cr"`
	XP      int    `json:"xp"`
	AC      int    `json:"ac"`
	HP      int    `json:"hp"`
	HitDice string `json:"hitDice"`
	Type    string `json:"type"`
	Size    string `json:"size"`
	Init    int    `json:"init"`
}

func monsterSummaryOf(m *randgen.Monster) monsterSummary {
	return monsterSummary{Id: m.Id, Name: m.Name, CR: m.CR, XP: m.XP, AC: m.AC, HP: m.HP, HitDice: m.HitDice,
		Type: m.Type, Size: m.Size, Init: m.Init()}
}

/* SDOC: API
* GET /dnd/monsters — Find Monsters
	SRD monsters whose names hold every word of =q=, those starting with it first, each with
	what an initiative tracker needs to add one: armour class, hit points, initiative bonus.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                  |
	|-----------+--------+----------+------------------------------|
	| =q=       | string | no       | Words of the name.           |
	| =limit=   | int    | no       | At most this many (40).      |

	*Response:* ={ok, monsters: [{id, name, cr, xp, ac, hp, hitDice, type, size, init}]}=
	EDOC */
func RequestDndMonsters(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 40
	}
	out := []monsterSummary{}
	for _, m := range randgen.SearchMonsters(r.URL.Query().Get("q"), limit) {
		out = append(out, monsterSummaryOf(&m))
	}
	dndJson(w, map[string]interface{}{"ok": true, "monsters": out})
}

/* SDOC: API
* GET /dnd/monster — A Monster's Stat Block
	One SRD monster by name, as written in a room or a wandering monster table ("Goblins" and
	"goblin" both find the goblin): its stat block - ability scores, speed, saves, senses and
	the like, traits, actions, reactions and legendary actions - and its initiative bonus.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description        |
	|-----------+--------+----------+--------------------|
	| =name=    | string | yes      | The monster's name.|

	*Response:* ={ok, monster, init}=
	EDOC */
func RequestDndMonster(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	m := randgen.FindMonster(r.URL.Query().Get("name"))
	if m == nil {
		dndError(w, http.StatusNotFound, "no monster called %q", r.URL.Query().Get("name"))
		return
	}
	dndJson(w, map[string]interface{}{"ok": true, "monster": m, "init": m.Init()})
}

// ---- the party -------------------------------------------------------------

type partyMember struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Player  string `json:"player"`
	AC      int    `json:"ac"`
	HP      int    `json:"hp"`
	Init    int    `json:"init"`
	Level   int    `json:"level"`
	Class   string `json:"class"`
	Passive int    `json:"passive"`
	// A character sheet this member is kept to: its numbers are read from
	// the sheet each time the party is, so they are never stale.
	Sheet     string `json:"sheet"`
	FromSheet bool   `json:"fromSheet"`
}

const partyHeader = `#+TITLE: The Party

The party the initiative tracker plays with: a heading per character. Change the numbers here
or in worg. A character with a =SHEET= is kept to that character sheet.

`

// partyHeadings finds the members' headings: top level headings whose
// drawer has a DND_PC.
type partyHeading struct {
	from, to int
	m        partyMember
}

func readPartyLines(lines []string) []partyHeading {
	var out []partyHeading
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "* ") {
			continue
		}
		to := len(lines) - 1
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "* ") {
				to = j - 1
				break
			}
		}
		ps, pe, ok := drawerAt(lines, i, to, "PROPERTIES")
		if !ok {
			continue
		}
		m := partyMember{Name: strings.TrimSpace(partyTitle(lines[i]))}
		for k := ps + 1; k < pe; k++ {
			p := propLineRe.FindStringSubmatch(lines[k])
			if p == nil {
				continue
			}
			v := strings.TrimSpace(p[4])
			n, _ := strconv.Atoi(v)
			switch strings.ToUpper(p[2]) {
			case "DND_PC":
				m.Id = v
			case "PLAYER":
				m.Player = v
			case "AC":
				m.AC = n
			case "HP":
				m.HP = n
			case "INIT":
				m.Init = n
			case "LEVEL":
				m.Level = n
			case "CLASS":
				m.Class = v
			case "PASSIVE":
				m.Passive = n
			case "SHEET":
				m.Sheet = v
			}
		}
		if m.Id != "" {
			out = append(out, partyHeading{from: i, to: to, m: m})
		}
	}
	return out
}

// partyTitle is a headline's text without its stars and tags.
func partyTitle(line string) string {
	t := strings.TrimLeft(line, "*")
	t = strings.TrimSpace(t)
	if i := strings.LastIndex(t, " :"); i > 0 && strings.HasSuffix(t, ":") {
		t = strings.TrimSpace(t[:i])
	}
	return t
}

// fromSheet brings a member up to its character sheet.
func fromSheet(m *partyMember) {
	if m.Sheet == "" {
		return
	}
	f, ok := insideOrgDirs(m.Sheet)
	if !ok {
		return
	}
	data, err := os.ReadFile(f)
	if err != nil {
		return
	}
	c, rs, err := dndplug.ParseCharacter(string(data))
	if err != nil {
		return
	}
	s := dnd.Compute(c, rs)
	m.AC, m.HP, m.Init, m.Level, m.Class, m.Passive = s.AC, s.HPMax, s.Initiative, s.Level, s.ClassLine, s.PassivePerception
	if m.Name == "" {
		m.Name = s.Name
	}
	m.FromSheet = true
}

func readParty(fname string) []partyMember {
	out := []partyMember{}
	for _, h := range readPartyLines(fileLines(fname)) {
		m := h.m
		fromSheet(&m)
		out = append(out, m)
	}
	return out
}

func partyProps(m partyMember) [][2]string {
	num := func(n int) string { return strconv.Itoa(n) }
	return [][2]string{{"DND_PC", m.Id}, {"PLAYER", m.Player}, {"AC", num(m.AC)}, {"HP", num(m.HP)},
		{"INIT", num(m.Init)}, {"LEVEL", num(m.Level)}, {"CLASS", m.Class}, {"PASSIVE", num(m.Passive)}, {"SHEET", m.Sheet}}
}

// writeParty brings the file's headings to the members: changed ones are
// edited in place (their notes and anything else under them kept), new ones
// added at the end, and ones no longer in the party taken out. Line edits
// throughout, the headings found again after every one (see Traps: re-measure).
func writeParty(fname string, members []partyMember) error {
	lines := fileLines(fname)
	if len(lines) == 0 {
		lines = strings.Split(strings.TrimSuffix(partyHeader, "\n"), "\n")
	}
	keep := map[string]bool{}
	for i := range members {
		if members[i].Id == "" {
			members[i].Id = uuid.NewString()[:8]
		}
		keep[members[i].Id] = true
	}
	// Out, from the bottom up so the rows above stay true.
	hs := readPartyLines(lines)
	for i := len(hs) - 1; i >= 0; i-- {
		if !keep[hs[i].m.Id] {
			lines = append(lines[:hs[i].from], lines[hs[i].to+1:]...)
		}
	}
	for _, m := range members {
		var at *partyHeading
		for _, h := range readPartyLines(lines) {
			if h.m.Id == m.Id {
				h := h
				at = &h
				break
			}
		}
		if at == nil {
			for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
				lines = lines[:len(lines)-1]
			}
			// Written with its first property, so the rest line up with it rather
			// than a level in (setPropIn indents a drawer's first property).
			lines = append(lines, "", "* "+oneLineName(m.Name), "  :PROPERTIES:", "  :DND_PC: "+m.Id, "  :END:")
			at = &partyHeading{from: len(lines) - 4, to: len(lines) - 1}
		} else if partyTitle(lines[at.from]) != m.Name {
			lines[at.from] = "* " + oneLineName(m.Name)
		}
		var ps, pe int
		lines, ps, pe, _ = ensurePropertyDrawer(lines, at.from, at.to, "  ")
		for _, kv := range partyProps(m) {
			// A blank one (no sheet, no player) is left out of the drawer.
			if kv[1] == "" {
				var dropped bool
				if lines, dropped = dropPropIn(lines, ps, pe, kv[0]); dropped {
					pe--
				}
				continue
			}
			lines, pe = setPropIn(lines, ps, pe, "  ", kv[0], kv[1])
		}
	}
	return writeLines(fname, lines)
}

func oneLineName(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "Unnamed"
	}
	return s
}

func partyFileOf(r *http.Request) string {
	if GetExtensions() == nil {
		return ""
	}
	u := GetUsername(r)
	GetExtensions().mu.RLock()
	defer GetExtensions().mu.RUnlock()
	if x, ok := GetExtensions().Users[u]; ok {
		return x.DndParty
	}
	return ""
}

/* SDOC: API
* GET /dnd/party — The Party
	The party the initiative tracker plays with, from the org file this user chose for it
	(=POST /dnd/party/file=): a heading per character with =DND_PC=, =PLAYER=, =AC=, =HP=,
	=INIT=, =LEVEL=, =CLASS=, =PASSIVE= and =SHEET= properties. A member with a =SHEET= has
	its numbers read from that character sheet.

	*Method:* =GET=

	*Response:* ={ok, file, members: [...]}=, =file= empty when none has been chosen yet.
	EDOC */
func RequestDndParty(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	file := partyFileOf(r)
	members := []partyMember{}
	if file != "" {
		members = readParty(file)
	}
	dndJson(w, map[string]interface{}{"ok": true, "file": file, "members": members})
}

/* SDOC: API
* POST /dnd/party/file — Choose Where the Party Is Kept
	Sets the org file this user's party lives in, made with a short header if it is not
	there yet. It must be inside an org directory and end in =.org=.

	*Method:* =POST=

	*Request Body:* ={file}=

	*Response:* ={ok, file, members}=
	EDOC */
func PostDndPartyFile(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		File string `json:"file"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	f, ok := insideOrgDirs(strings.TrimSpace(req.File))
	if !ok || filepath.Ext(f) != ".org" {
		dndError(w, http.StatusBadRequest, "the party has to be kept in an org file inside one of the org directories")
		return
	}
	if _, err := os.Stat(f); err != nil {
		if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
			dndError(w, http.StatusInternalServerError, "%s", err)
			return
		}
		if err := os.WriteFile(f, []byte(partyHeader), 0644); err != nil {
			dndError(w, http.StatusInternalServerError, "%s", err)
			return
		}
		GetDb().ReloadFile(f)
	}
	ext := GetExtensions()
	if ext == nil {
		dndError(w, http.StatusInternalServerError, "no extensions file to remember the choice in")
		return
	}
	ext.mu.Lock()
	ext.getUser(GetUsername(r)).DndParty = f
	err := ext.save()
	ext.mu.Unlock()
	if err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	dndJson(w, map[string]interface{}{"ok": true, "file": f, "members": readParty(f)})
}

/* SDOC: API
* POST /dnd/party — Keep the Party
	Writes the party into its org file: changed characters are edited in place (anything
	written under them is kept), new ones added, and ones left out removed.

	*Method:* =POST=

	*Request Body:* ={members: [{id, name, player, ac, hp, init, level, class, passive, sheet}]}=

	*Response:* ={ok, file, members}=, read back from the file.
	EDOC */
func PostDndParty(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req struct {
		Members []partyMember `json:"members"`
	}
	if !dndBody(w, r, &req) {
		return
	}
	file := partyFileOf(r)
	if file == "" {
		dndError(w, http.StatusBadRequest, "choose where the party is kept first")
		return
	}
	if err := writeParty(file, req.Members); err != nil {
		dndError(w, http.StatusInternalServerError, "%s", err)
		return
	}
	dndJson(w, map[string]interface{}{"ok": true, "file": file, "members": readParty(file)})
}

