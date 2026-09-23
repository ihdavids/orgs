package dnd

/* SDOC: DnD
* Play Session Logs

  A play session is an ordinary org file that records what happened at the
  table: every die you rolled and every note you took. The html character
  sheet writes to it live, so the log is a normal org file you can read,
  edit, refile out of or archive like anything else.

  #+BEGIN_SRC org
  ,#+TITLE: Goblin Ambush
  ,#+DATE: [2025-09-05 Fri]
  ,#+SUMMARY: Ambushed on the road to Phandalin, Sildar is missing.
  ,#+FILETAGS: :dnd:session:

  ,* Characters
  ,** Lyra
     :PROPERTIES:
     :DND_ID: lyra-silverleaf-4c1f2a
     :DND_CHARACTER: Lyra
     :END:
     [[file:/gtd/dnd/lyra.org][Lyra]]

  ,* Notes
  ,** 19:32
     The wagon tracks leave the road here.
  ,* Rolls
  ,#+NAME: rolls
  | Time  | Character | Roll       | Formula | Result | Dice     | Notes          |
  |-------+-----------+------------+---------+--------+----------+----------------|
  | 19:33 | Lyra      | Perception | d20 +5  | 18     | d20 13   | adv 22, dis 18 |
  #+END_SRC

  Only three things in the file are load bearing: the =#+SUMMARY:= keyword,
  the =* Notes= heading and the table named =rolls=. Everything else in the
  file is yours. New notes are appended to the end of the notes section and
  new rolls to the end of the table, so anything you add by hand between
  sessions is left exactly where you put it.

  Each character who played gets a heading carrying their =DND_ID= and an
  ordinary file link to their sheet, so the evening and the character are one
  click apart in either direction and the link graph knows about both. A
  session written before the sheet said where it lived has the link added the
  next time that character writes to it.

  A note already in the file can be said again: =POST
  /dnd/play/session/{id}/note/{index}= rewrites that one entry in place and
  leaves everything around it alone, which is what the *Edit* button on a
  note in the character sheet's session panel does.

  A roll can be corrected the same way: =POST
  /dnd/play/session/{id}/roll/{index}= rewrites one row of the table. The
  character sheet uses it to settle a d20. Every d20 is rolled twice, so the
  tray can show the flat roll, the better of the two and the worse all at
  once; clicking one of those says which the table was actually owed, and the
  row that was already written is rewritten to match rather than a second
  roll being logged. Only the roll still on the card can be settled - once
  you roll again it stands as it is.
EDOC */

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	// SessionTableName is the org table name the roll log is appended to.
	SessionTableName = "rolls"
	// SessionNotesHeading is the section notes are appended under.
	SessionNotesHeading = "Notes"
	// SessionRollsHeading is the section the roll table is created under when
	// a session file does not have one yet.
	SessionRollsHeading = "Rolls"
	// SessionCharactersHeading is the section the characters who played are
	// listed under, one heading each, carrying the character id.
	SessionCharactersHeading = "Characters"
	// SessionSummaryKey is the file keyword holding the one line summary.
	SessionSummaryKey = "#+SUMMARY:"
	// SessionCharacterKey is the file keyword naming who played.
	SessionCharacterKey = "#+CHARACTERS:"
	// PropSessionCharacter and PropSessionCharacterId are the properties on a
	// character heading inside a session file. The id matches DND_ID on the
	// character sheet, which is what ties the two together in a query.
	PropSessionCharacter   = "DND_CHARACTER"
	PropSessionCharacterId = "DND_ID"
	// SessionNoteTimeFormat is the heading of a single note entry.
	SessionNoteTimeFormat = "15:04"
)

// sessionRollHeader is the table header written when the table is created.
var sessionRollHeader = []string{"Time", "Character", "Roll", "Formula", "Result", "Dice", "Notes"}

// SessionRoll is one die roll as recorded from a character sheet.
type SessionRoll struct {
	Time      string `json:"time"`
	Character string `json:"character"`
	Label     string `json:"label"`
	Formula   string `json:"formula"`
	Result    string `json:"result"`
	Dice      string `json:"dice"`
	Notes     string `json:"notes"`
}

// SessionNote is one timestamped note taken during play. Text is org markup.
type SessionNote struct {
	Time string `json:"time"`
	Text string `json:"text"`
}

// SessionCharacter is one character who played in a session.
type SessionCharacter struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	// File is the character's own org sheet. The session file links to it
	// under the character's heading, so the evening and the character who
	// played it are one click apart in either direction.
	File string `json:"file"`
}

// SessionInfo is the summary of a session file used by the session list.
type SessionInfo struct {
	Id         string             `json:"id"`
	Name       string             `json:"name"`
	Date       string             `json:"date"`
	File       string             `json:"file"`
	Summary    string             `json:"summary"`
	Rolls      int                `json:"rolls"`
	Notes      int                `json:"notes"`
	Characters []SessionCharacter `json:"characters"`
}

// PlayedBy reports whether a character took part in the session, which is what
// keeps one player's sheet from listing the whole table's sessions.
//
// The id is what ties a session file to a character sheet, so it decides
// whenever both sides have one. The name is a fallback for entries written
// before the id was recorded: those carry a name and nothing else, and hiding
// them would lose a campaign's early sessions from the very sheet that played
// them.
//
// A session that names nobody at all is left to the caller. It excludes no
// one, so there is nothing here to decide.
func (s *SessionInfo) PlayedBy(id, name string) bool {
	id, name = strings.TrimSpace(id), strings.TrimSpace(name)
	for _, c := range s.Characters {
		if id != "" && c.Id != "" {
			if strings.EqualFold(c.Id, id) {
				return true
			}
			continue
		}
		if name != "" && strings.EqualFold(strings.TrimSpace(c.Name), name) {
			return true
		}
	}
	return false
}

// SessionDetail is everything recorded in one session.
type SessionDetail struct {
	SessionInfo
	RollLog []SessionRoll `json:"rollLog"`
	NoteLog []SessionNote `json:"noteLog"`
	// MarkLog is the timeline's annotations - the blocks somebody has named
	// or written a line about. They are the one part of the timeline that is
	// stored rather than worked out; see sessionmark.go.
	MarkLog  []SessionMark `json:"markLog"`
	Warnings []string      `json:"warnings,omitempty"`
}

// SessionMatch is one hit from a search across session files.
type SessionMatch struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Date    string `json:"date"`
	Line    int    `json:"line"`
	Text    string `json:"text"`
	Context string `json:"context"`
	Kind    string `json:"kind"`
}

// row renders a roll as the cells of a table row.
func (r *SessionRoll) row() []string {
	return []string{
		cleanCell(r.Time), cleanCell(r.Character), cleanCell(r.Label),
		cleanCell(r.Formula), cleanCell(r.Result), cleanCell(r.Dice), cleanCell(r.Notes),
	}
}

// rollFromRow reads a roll back out of a table row. Short rows are tolerated
// so that a table someone has trimmed by hand still loads.
func rollFromRow(cells []string) SessionRoll {
	at := func(i int) string {
		if i < len(cells) {
			return cells[i]
		}
		return ""
	}
	return SessionRoll{Time: at(0), Character: at(1), Label: at(2), Formula: at(3),
		Result: at(4), Dice: at(5), Notes: at(6)}
}

// cleanCell makes a value safe to sit inside an org table cell.
func cleanCell(v string) string {
	v = strings.ReplaceAll(v, "|", "/")
	v = strings.Join(strings.Fields(v), " ")
	return v
}

var slugStrip = regexp.MustCompile(`[^A-Za-z0-9_-]+`)
var slugRepeat = regexp.MustCompile(`_{2,}`)
var validId = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// SessionSlug turns a session name into something usable in a file name.
func SessionSlug(name string) string {
	s := slugStrip.ReplaceAllString(strings.TrimSpace(name), "_")
	s = slugRepeat.ReplaceAllString(s, "_")
	return strings.Trim(s, "_-.")
}

// SessionFileName builds the file name for a session. The date leads so that
// a directory listing sorts chronologically, the name is appended when given.
func SessionFileName(name string, dt time.Time) string {
	base := dt.Format("2006_01_02")
	if slug := SessionSlug(name); slug != "" {
		base += "_" + slug
	}
	return base + ".org"
}

// ValidSessionId rejects anything that could walk out of the session folder.
func ValidSessionId(id string) bool {
	return id != "" && !strings.Contains(id, "..") && validId.MatchString(id)
}

// DateFromSessionId pulls the leading yyyy_mm_dd off a session id.
func DateFromSessionId(id string) string {
	if len(id) < 10 {
		return ""
	}
	if dt, err := time.Parse("2006_01_02", id[:10]); err == nil {
		return dt.Format("2006-01-02")
	}
	return ""
}

// DefaultSessionTemplate is used when no dndsession.tpl can be found. It is a
// pongo2 template and gets the same context the file template would.
const DefaultSessionTemplate = `#+TITLE: {{ session_title }}
#+DATE: {{ org_date }}
#+SUMMARY:{% if session_summary %} {{ session_summary }}{% endif %}
#+CHARACTERS:
#+STARTUP: showeverything
#+FILETAGS: :dnd:session:

* Characters

* Notes

* Rolls
#+NAME: rolls
`

// ----------------------------------------------------------------------------
// Reading a session file
// ----------------------------------------------------------------------------

// SessionName reads the #+TITLE out of a session file, falling back to the id.
func SessionName(text, id string) string {
	if v := fileKeyword(text, "#+TITLE:"); v != "" {
		return v
	}
	return id
}

// SessionSummary reads the #+SUMMARY line. When the file has none the first
// line of the first note stands in, which is almost always what you would
// have written there anyway.
func SessionSummary(text string) string {
	if v := fileKeyword(text, SessionSummaryKey); v != "" {
		return v
	}
	for _, n := range ParseNotes(text) {
		for _, line := range strings.Split(n.Text, "\n") {
			line = strings.TrimSpace(strings.TrimLeft(line, "*"))
			if line != "" {
				return firstSentence(line)
			}
		}
	}
	return ""
}

// SetSessionSummary rewrites (or inserts) the #+SUMMARY keyword.
func SetSessionSummary(text, summary string) string {
	return setFileKeyword(text, SessionSummaryKey, summary)
}

func setFileKeyword(text, key, value string) string {
	value = strings.Join(strings.Fields(value), " ")
	lines := splitLines(text)
	for i, l := range lines {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(l)), key) {
			lines[i] = key + " " + value
			return strings.Join(lines, "\n")
		}
	}
	// No keyword yet: put it after the last of the leading file keywords.
	at := 0
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "#+") {
			at = i + 1
			continue
		}
		if t == "" {
			continue
		}
		break
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, key+" "+value)
	out = append(out, lines[at:]...)
	return strings.Join(out, "\n")
}

func fileKeyword(text, key string) string {
	up := strings.ToUpper(key)
	for _, l := range splitLines(text) {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(strings.ToUpper(t), up) {
			return strings.TrimSpace(t[len(key):])
		}
		if strings.HasPrefix(t, "*") {
			break
		}
	}
	return ""
}

// ParseRolls reads the roll table back out of a session file.
func ParseRolls(text string) []SessionRoll {
	lines := splitLines(text)
	start, end, _ := findRollTable(lines)
	out := []SessionRoll{}
	if start < 0 || start == end {
		return out
	}
	for i := start; i < end; i++ {
		t := strings.TrimSpace(lines[i])
		if isTableRule(t) {
			continue
		}
		cells := parseTableRow(lines[i])
		if len(cells) > 0 && strings.EqualFold(cells[0], sessionRollHeader[0]) {
			continue
		}
		out = append(out, rollFromRow(cells))
	}
	return out
}

// ParseNotes reads the note entries back out of a session file. Headings the
// note itself contains are promoted back to the level they were typed at.
func ParseNotes(text string) []SessionNote {
	lines := splitLines(text)
	start, end := findNotesSection(lines)
	out := []SessionNote{}
	if start < 0 {
		return out
	}
	var cur *SessionNote
	body := []string{}
	flush := func() {
		if cur == nil {
			return
		}
		cur.Text = strings.TrimRight(strings.Join(orgStripBreaks(body), "\n"), "\n")
		out = append(out, *cur)
		cur = nil
		body = nil
	}
	for i := start; i < end; i++ {
		line := lines[i]
		if lvl, title := headingOf(line); lvl == 2 {
			flush()
			cur = &SessionNote{Time: strings.TrimSpace(title)}
			continue
		} else if lvl > 2 {
			body = append(body, strings.Repeat("*", lvl-2)+" "+title)
			continue
		}
		if cur != nil {
			body = append(body, unindent(line))
		}
	}
	flush()
	return out
}

// ParseSessionCharacters reads the characters section back out of a session.
func ParseSessionCharacters(text string) []SessionCharacter {
	lines := splitLines(text)
	start, end := findSection(lines, SessionCharactersHeading)
	out := []SessionCharacter{}
	if start < 0 {
		return out
	}
	var cur *SessionCharacter
	flush := func() {
		if cur != nil && (cur.Name != "" || cur.Id != "") {
			out = append(out, *cur)
		}
		cur = nil
	}
	for i := start; i < end; i++ {
		if lvl, title := headingOf(lines[i]); lvl == 2 {
			flush()
			cur = &SessionCharacter{Name: stripTags(title)}
			continue
		}
		if cur == nil {
			continue
		}
		if target := fileLinkTarget(lines[i]); target != "" && cur.File == "" {
			cur.File = target
		}
		key, val := drawerProperty(lines[i])
		switch key {
		case PropSessionCharacterId:
			cur.Id = val
		case PropSessionCharacter:
			if val != "" {
				cur.Name = val
			}
		}
	}
	flush()
	return out
}

// AddSessionCharacter records who is playing. The heading it writes carries
// the character's DND_ID, so one query over your org files gathers the sheet
// and every session that character has been in.
func AddSessionCharacter(text string, ch SessionCharacter) string {
	ch.Name = strings.TrimSpace(ch.Name)
	ch.Id = strings.TrimSpace(ch.Id)
	if ch.Name == "" && ch.Id == "" {
		return text
	}
	known := ParseSessionCharacters(text)
	for i, k := range known {
		if (ch.Id != "" && k.Id == ch.Id) || (ch.Id == "" && k.Name == ch.Name) {
			// Already listed. A session written before the sheet knew where
			// the character lived has no link back to it, so this is where
			// one is added.
			if ch.File != "" && k.File == "" {
				return addCharacterLink(text, i, ch)
			}
			return text
		}
	}
	name := ch.Name
	if name == "" {
		name = ch.Id
	}
	entry := []string{
		"** " + name,
		"   :PROPERTIES:",
		"   :" + PropSessionCharacterId + ": " + ch.Id,
		"   :" + PropSessionCharacter + ": " + ch.Name,
		"   :END:",
	}
	if link := characterLink(ch); link != "" {
		entry = append(entry, "   "+link)
	}

	names := []string{}
	for _, k := range known {
		if k.Name != "" {
			names = append(names, k.Name)
		}
	}
	names = append(names, name)
	text = setFileKeyword(text, SessionCharacterKey, strings.Join(names, ", "))

	lines := splitLines(text)
	start, end := findSection(lines, SessionCharactersHeading)
	if start < 0 {
		// No characters section yet: put one above the notes so the file reads
		// as who, then what happened, then what was rolled.
		at := len(lines)
		for i, l := range lines {
			if lvl, title := headingOf(l); lvl == 1 && strings.EqualFold(stripTags(title), SessionNotesHeading) {
				at = i
				break
			}
		}
		out := append([]string{}, lines[:at]...)
		out = append(out, "* "+SessionCharactersHeading)
		out = append(out, entry...)
		out = append(out, "")
		out = append(out, lines[at:]...)
		return strings.Join(out, "\n")
	}
	body := trimTrailingBlanks(lines[start:end])
	out := append([]string{}, lines[:start]...)
	out = append(out, body...)
	out = append(out, entry...)
	out = append(out, "")
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// characterLink is the org link from a session back to the character sheet
// that played it. It is an ordinary file link, so it opens in org mode, is
// picked up by the backlink index, and reads as the character's name.
func characterLink(ch SessionCharacter) string {
	file := strings.TrimSpace(ch.File)
	if file == "" {
		return ""
	}
	name := strings.TrimSpace(ch.Name)
	if name == "" {
		name = strings.TrimSpace(ch.Id)
	}
	if name == "" {
		return "[[file:" + file + "]]"
	}
	return "[[file:" + file + "][" + name + "]]"
}

// fileLinkTarget reads the target out of a line holding a file link, and
// answers empty for a line that is not one.
func fileLinkTarget(line string) string {
	m := reFileLink.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

var reFileLink = regexp.MustCompile(`\[\[file:([^\]]+)\](?:\[[^\]]*\])?\]`)

// addCharacterLink puts a link to the character's sheet into an entry that
// was written without one, directly under its property drawer.
func addCharacterLink(text string, index int, ch SessionCharacter) string {
	link := characterLink(ch)
	if link == "" {
		return text
	}
	lines := splitLines(text)
	start, end := findSection(lines, SessionCharactersHeading)
	if start < 0 {
		return text
	}
	at, n := -1, -1
	for i := start; i < end; i++ {
		if lvl, _ := headingOf(lines[i]); lvl == 2 {
			n++
			if n == index {
				at = i
			} else if at >= 0 {
				end = i
				break
			}
		}
	}
	if at < 0 {
		return text
	}
	// After the drawer if the entry has one, otherwise straight under the
	// heading. Either way the link is the last line of what is already there.
	put := at + 1
	for i := at + 1; i < end; i++ {
		if strings.EqualFold(strings.TrimSpace(lines[i]), ":END:") {
			put = i + 1
			break
		}
	}
	out := append([]string{}, lines[:put]...)
	out = append(out, "   "+link)
	out = append(out, lines[put:]...)
	return strings.Join(out, "\n")
}

// drawerProperty reads a ":KEY: value" line out of a property drawer.
func drawerProperty(line string) (string, string) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, ":") || strings.HasPrefix(t, ":PROPERTIES:") || strings.HasPrefix(t, ":END:") {
		return "", ""
	}
	rest := t[1:]
	idx := strings.Index(rest, ":")
	if idx <= 0 {
		return "", ""
	}
	return strings.ToUpper(strings.TrimSpace(rest[:idx])), strings.TrimSpace(rest[idx+1:])
}

// ----------------------------------------------------------------------------
// Appending to a session file
// ----------------------------------------------------------------------------

// AppendRolls adds rows to the roll table, creating the table when the file
// does not have one yet. The rest of the file is untouched.
func AppendRolls(text string, rolls []SessionRoll) string {
	if len(rolls) == 0 {
		return text
	}
	lines := splitLines(text)
	start, end, indent := findRollTable(lines)
	rows := [][]string{}
	if start >= 0 {
		for i := start; i < end; i++ {
			if isTableRule(strings.TrimSpace(lines[i])) {
				continue
			}
			rows = append(rows, parseTableRow(lines[i]))
		}
	}
	if len(rows) == 0 || !strings.EqualFold(strings.TrimSpace(rows[0][0]), sessionRollHeader[0]) {
		rows = append([][]string{append([]string{}, sessionRollHeader...)}, rows...)
	}
	for i := range rolls {
		rows = append(rows, rolls[i].row())
	}
	table := indentLines(strings.Split(orgTable(rows, 1), "\n"), indent)

	if start < 0 {
		out := trimTrailingBlanks(lines)
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, "* "+SessionRollsHeading, "#+NAME: "+SessionTableName)
		out = append(out, table...)
		out = append(out, "")
		return strings.Join(out, "\n")
	}
	out := append([]string{}, lines[:start]...)
	out = append(out, table...)
	// Writing a brand new table straight under its #+NAME: must not run into
	// whatever came next.
	if start == end && start < len(lines) && strings.TrimSpace(lines[start]) != "" {
		out = append(out, "")
	}
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// AppendNotes adds note entries to the notes section, creating the section
// when the file does not have one. Each note becomes a level two heading
// stamped with the time, and any headings inside the note are pushed down so
// that they stay part of that entry.
func AppendNotes(text string, notes []SessionNote) string {
	if len(notes) == 0 {
		return text
	}
	lines := splitLines(text)
	start, end := findNotesSection(lines)
	block := []string{}
	for _, n := range notes {
		if strings.TrimSpace(n.Text) == "" {
			continue
		}
		stamp := strings.TrimSpace(n.Time)
		if stamp == "" {
			stamp = time.Now().Format(SessionNoteTimeFormat)
		}
		block = append(block, "** "+stamp)
		block = append(block, renderNoteBody(n.Text)...)
		block = append(block, "")
	}
	if len(block) == 0 {
		return text
	}
	if start < 0 {
		out := trimTrailingBlanks(lines)
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, "* "+SessionNotesHeading)
		out = append(out, block...)
		return strings.Join(out, "\n")
	}
	body := trimTrailingBlanks(lines[start:end])
	out := append([]string{}, lines[:start]...)
	out = append(out, body...)
	if len(body) > 0 {
		out = append(out, "")
	}
	out = append(out, block...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// UpdateRoll rewrites one row of the roll table where it stands. index is the
// roll's position in the table, counted the way ParseRolls reads it, and was -
// when it is given - is the label that row is expected to carry: a roll edited
// against a stale reading of the file is refused rather than written over the
// wrong row.
//
// It exists so that a die already thrown can be told what it meant. The sheet
// rolls all three of a d20 - the flat roll, the better of two and the worse -
// and the player says afterwards which one the table was owed; saying so
// rewrites the one row rather than adding a second roll that never happened.
//
// Cells the edit leaves empty keep what the row already holds, so a caller
// that only has a new result need not restate the time or who rolled it.
func UpdateRoll(text string, index int, roll SessionRoll, was string) (string, error) {
	lines := splitLines(text)
	start, end, indent := findRollTable(lines)
	if start < 0 || start == end {
		return text, fmt.Errorf("this session has no rolls to edit")
	}
	rows := [][]string{}
	for i := start; i < end; i++ {
		if isTableRule(strings.TrimSpace(lines[i])) {
			continue
		}
		rows = append(rows, parseTableRow(lines[i]))
	}
	// Data rows are counted exactly as ParseRolls counts them, so the index
	// the caller read out of the session detail is the row it lands on.
	data := []int{}
	for i, cells := range rows {
		if len(cells) > 0 && strings.EqualFold(cells[0], sessionRollHeader[0]) {
			continue
		}
		data = append(data, i)
	}
	if index < 0 || index >= len(data) {
		return text, fmt.Errorf("this session has no roll %d", index+1)
	}
	at := data[index]
	old := rollFromRow(rows[at])
	if was = strings.TrimSpace(was); was != "" && !strings.EqualFold(was, strings.TrimSpace(old.Label)) {
		return text, fmt.Errorf(
			"that roll now reads %q, not %q - read the session again before editing it",
			old.Label, was)
	}
	keep := func(now, was string) string {
		if strings.TrimSpace(now) == "" {
			return was
		}
		return now
	}
	roll.Time = keep(roll.Time, old.Time)
	roll.Character = keep(roll.Character, old.Character)
	roll.Label = keep(roll.Label, old.Label)
	roll.Formula = keep(roll.Formula, old.Formula)
	roll.Result = keep(roll.Result, old.Result)
	roll.Dice = keep(roll.Dice, old.Dice)
	roll.Notes = keep(roll.Notes, old.Notes)
	rows[at] = roll.row()
	table := indentLines(strings.Split(orgTable(rows, 1), "\n"), indent)
	out := append([]string{}, lines[:start]...)
	out = append(out, table...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n"), nil
}

// DeleteRoll takes one row out of a session's roll table.
//
// It finds the row the way UpdateRoll does, and refuses on the same terms: a
// roll number that is not there, or one whose label is no longer what the
// page thought it was. The check earns its keep here in the same way it does
// for a note - deleting shifts every roll after it up by one.
//
// Taking the last row out takes the table with it, leaving the "#+NAME:"
// anchor and nothing under it, which is exactly the state a session with no
// rolls in it starts in. A lone header row would parse the same but would
// read as a table somebody had emptied rather than one nothing had been
// written to yet.
func DeleteRoll(text string, index int, was string) (string, error) {
	lines := splitLines(text)
	start, end, indent := findRollTable(lines)
	if start < 0 || start == end {
		return text, fmt.Errorf("this session has no rolls to delete")
	}
	rows := [][]string{}
	for i := start; i < end; i++ {
		if isTableRule(strings.TrimSpace(lines[i])) {
			continue
		}
		rows = append(rows, parseTableRow(lines[i]))
	}
	// Data rows are counted exactly as ParseRolls counts them, so the index
	// the caller read out of the session detail is the row it lands on.
	data := []int{}
	for i, cells := range rows {
		if len(cells) > 0 && strings.EqualFold(cells[0], sessionRollHeader[0]) {
			continue
		}
		data = append(data, i)
	}
	if index < 0 || index >= len(data) {
		return text, fmt.Errorf("this session has no roll %d", index+1)
	}
	at := data[index]
	old := rollFromRow(rows[at])
	if was = strings.TrimSpace(was); was != "" && !strings.EqualFold(was, strings.TrimSpace(old.Label)) {
		return text, fmt.Errorf(
			"that roll now reads %q, not %q - read the session again before deleting it",
			old.Label, was)
	}
	rows = append(rows[:at], rows[at+1:]...)

	table := []string{}
	if len(data) > 1 {
		table = indentLines(strings.Split(orgTable(rows, 1), "\n"), indent)
	}
	out := append([]string{}, lines[:start]...)
	out = append(out, table...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n"), nil
}

// UpdateNote rewrites one note that is already in the file, in place. index
// is the note's position in the notes section, counted the way ParseNotes
// reads them, and was - when it is given - is the time stamp that note is
// expected to carry: a note edited from a stale page is refused rather than
// written over the wrong entry.
//
// The rest of the file is untouched, including anything written between the
// notes by hand, and a note is never emptied: rubbing one out is deleting it,
// which is a thing to do in the file itself.
func UpdateNote(text string, index int, note SessionNote, was string) (string, error) {
	if strings.TrimSpace(note.Text) == "" {
		return text, fmt.Errorf("a note cannot be left empty")
	}
	lines := splitLines(text)
	start, end := findNotesSection(lines)
	if start < 0 {
		return text, fmt.Errorf("this session has no notes to edit")
	}
	heads := []int{}
	for i := start; i < end; i++ {
		if lvl, _ := headingOf(lines[i]); lvl == 2 {
			heads = append(heads, i)
		}
	}
	if index < 0 || index >= len(heads) {
		return text, fmt.Errorf("this session has no note %d", index+1)
	}
	at := heads[index]
	stop := end
	if index+1 < len(heads) {
		stop = heads[index+1]
	}
	_, title := headingOf(lines[at])
	stamp := strings.TrimSpace(title)
	if was = strings.TrimSpace(was); was != "" && !strings.EqualFold(was, stamp) {
		return text, fmt.Errorf(
			"that note now reads %q, not %q - read the session again before editing it",
			stamp, was)
	}
	if t := strings.TrimSpace(note.Time); t != "" {
		stamp = t
	}
	block := append([]string{"** " + stamp}, renderNoteBody(note.Text)...)
	block = trimTrailingBlanks(block)
	// The blank line that held this entry apart from the next one is part of
	// how the file reads, not part of the note, so it stays.
	if stop > at && strings.TrimSpace(lines[stop-1]) == "" {
		block = append(block, "")
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, block...)
	out = append(out, lines[stop:]...)
	return strings.Join(out, "\n"), nil
}

// DeleteNote takes one entry out of a session's notes altogether.
//
// It finds the entry exactly the way UpdateNote does, and refuses on the same
// terms: a note number that is not there, or one whose stamp is no longer what
// the page thought it was. That check matters more here than it does for an
// edit, because deleting shifts every note after it up by one - a page holding
// stale numbers could otherwise throw away the wrong note and be none the
// wiser.
func DeleteNote(text string, index int, was string) (string, error) {
	lines := splitLines(text)
	start, end := findNotesSection(lines)
	if start < 0 {
		return text, fmt.Errorf("this session has no notes to delete")
	}
	heads := []int{}
	for i := start; i < end; i++ {
		if lvl, _ := headingOf(lines[i]); lvl == 2 {
			heads = append(heads, i)
		}
	}
	if index < 0 || index >= len(heads) {
		return text, fmt.Errorf("this session has no note %d", index+1)
	}
	at := heads[index]
	stop := end
	if index+1 < len(heads) {
		stop = heads[index+1]
	}
	_, title := headingOf(lines[at])
	stamp := strings.TrimSpace(title)
	if was = strings.TrimSpace(was); was != "" && !strings.EqualFold(was, stamp) {
		return text, fmt.Errorf(
			"that note now reads %q, not %q - read the session again before deleting it",
			stamp, was)
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, lines[stop:]...)
	return strings.Join(out, "\n"), nil
}

// renderNoteBody pushes the note's own headings two levels down so they nest
// under the entry, indents everything else to match, and writes the line
// breaks somebody typed as line breaks org will honour - see orgBreakLines.
func renderNoteBody(text string) []string {
	out := []string{}
	for _, line := range splitLines(strings.TrimRight(text, "\n")) {
		if lvl, title := headingOf(line); lvl > 0 {
			out = append(out, strings.Repeat("*", lvl+2)+" "+title)
			continue
		}
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, "   "+strings.TrimRight(line, " \t"))
	}
	return orgBreakLines(out)
}

// ----------------------------------------------------------------------------
// Line breaks
//
// Org runs consecutive lines of prose together into one paragraph. That is
// right for prose and wrong for a session note: somebody typing four short
// lines into the notes box at the table means four lines, and getting one
// run-on sentence back out of the export is not what they wrote.
//
// So a hard line break - org's trailing "\\" - is written wherever two lines
// would otherwise be flowed into one, and taken off again when the note is
// read back, so what the editor shows is what was typed. Both directions ask
// the same question of the same lines, which is what keeps the round trip
// honest: a break is only removed where one would have been added.
// ----------------------------------------------------------------------------

// orgBullet is a list marker, which starts something new rather than
// continuing the line above it.
var orgBullet = regexp.MustCompile(`^(?:[-+*]\s|\d+[.)]\s)`)

// orgBreakLines adds the break. Nothing inside a #+BEGIN_/#+END_ block is
// touched: those are verbatim, and a backslash pair in one is content.
func orgBreakLines(lines []string) []string {
	verbatim := false
	for i := range lines {
		t := strings.TrimSpace(lines[i])
		up := strings.ToUpper(t)
		if strings.HasPrefix(up, "#+BEGIN_") {
			verbatim = true
			continue
		}
		if strings.HasPrefix(up, "#+END_") {
			verbatim = false
			continue
		}
		if verbatim || !orgFlowsOn(lines, i) || strings.HasSuffix(t, `\\`) {
			continue
		}
		lines[i] = strings.TrimRight(lines[i], " \t") + ` \\`
	}
	return lines
}

// orgStripBreaks takes it off again, and only where orgBreakLines would have
// put it - so a backslash pair somebody typed themselves at the end of a last
// line survives being read back.
func orgStripBreaks(lines []string) []string {
	verbatim := false
	for i := range lines {
		t := strings.TrimSpace(lines[i])
		up := strings.ToUpper(t)
		if strings.HasPrefix(up, "#+BEGIN_") {
			verbatim = true
			continue
		}
		if strings.HasPrefix(up, "#+END_") {
			verbatim = false
			continue
		}
		if verbatim || !orgFlowsOn(lines, i) || !strings.HasSuffix(t, `\\`) {
			continue
		}
		lines[i] = strings.TrimRight(strings.TrimSuffix(
			strings.TrimRight(lines[i], " \t"), `\\`), " \t")
	}
	return lines
}

// orgFlowsOn reports whether the line after i would be run into line i when
// the org is read.
func orgFlowsOn(lines []string, i int) bool {
	if i < 0 || i+1 >= len(lines) {
		return false
	}
	return orgFlowable(lines[i]) && orgFlowsInto(lines[i+1])
}

// orgFlowable is a line that can carry a break at all: prose, or a list item
// with more of itself on the next line. A heading, a keyword, a table row and
// a drawer line all end where they end.
func orgFlowable(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	if lvl, _ := headingOf(line); lvl > 0 {
		return false
	}
	if strings.HasPrefix(t, "#+") || strings.HasPrefix(t, "|") {
		return false
	}
	return !(strings.HasPrefix(t, ":") && strings.HasSuffix(t, ":"))
}

// orgFlowsInto is a line org runs into the one above it. A bullet, a heading,
// a table or a keyword starts something new instead.
func orgFlowsInto(line string) bool {
	return orgFlowable(line) && !orgBullet.MatchString(strings.TrimSpace(line))
}

// ----------------------------------------------------------------------------
// Searching
// ----------------------------------------------------------------------------

// SearchSession scans one session file for a term, reporting each hit with
// the heading it lives under so a result reads like something rather than
// like a line number.
func SearchSession(info SessionInfo, text, term string, max int) []SessionMatch {
	out := []SessionMatch{}
	needle := strings.ToLower(strings.TrimSpace(term))
	if needle == "" {
		return out
	}
	context := ""
	kind := "note"
	for i, line := range splitLines(text) {
		trimmed := strings.TrimSpace(line)
		if lvl, title := headingOf(line); lvl > 0 {
			context = strings.TrimSpace(title)
			if lvl == 1 {
				kind = "note"
				if strings.EqualFold(context, SessionRollsHeading) {
					kind = "roll"
				}
			}
			continue
		}
		if trimmed == "" {
			continue
		}
		// The title and the summary describe the session, so they are worth
		// finding; the rest of the file keywords are plumbing.
		if strings.HasPrefix(trimmed, "#+") {
			up := strings.ToUpper(trimmed)
			if !strings.HasPrefix(up, SessionSummaryKey) && !strings.HasPrefix(up, "#+TITLE:") {
				continue
			}
			value := strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:])
			if value == "" || !strings.Contains(strings.ToLower(value), needle) {
				continue
			}
			out = append(out, SessionMatch{Id: info.Id, Name: info.Name, Date: info.Date,
				Line: i + 1, Text: value, Context: "Summary", Kind: "summary"})
			if max > 0 && len(out) >= max {
				break
			}
			continue
		}
		if !strings.Contains(strings.ToLower(trimmed), needle) {
			continue
		}
		if isTableRule(trimmed) {
			continue
		}
		text := trimmed
		if strings.HasPrefix(text, "|") {
			text = strings.Join(parseTableRow(text), " · ")
			kind = "roll"
		}
		out = append(out, SessionMatch{Id: info.Id, Name: info.Name, Date: info.Date,
			Line: i + 1, Text: text, Context: context, Kind: kind})
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

// ----------------------------------------------------------------------------
// Small helpers
// ----------------------------------------------------------------------------

func splitLines(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

func isTableRule(trimmed string) bool {
	return strings.HasPrefix(trimmed, "|-") || strings.HasPrefix(trimmed, "|+")
}

// headingOf reports the level and title of an org heading, or 0.
func headingOf(line string) (int, string) {
	i := 0
	for i < len(line) && line[i] == '*' {
		i++
	}
	if i == 0 || i >= len(line) || (line[i] != ' ' && line[i] != '\t') {
		return 0, ""
	}
	return i, strings.TrimSpace(line[i:])
}

func unindent(line string) string {
	if strings.HasPrefix(line, "   ") {
		return line[3:]
	}
	return strings.TrimLeft(line, " \t")
}

func indentLines(lines []string, indent string) []string {
	if indent == "" {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = indent + l
	}
	return out
}

func trimTrailingBlanks(lines []string) []string {
	out := append([]string{}, lines...)
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

// findRollTable locates the roll table. It returns the half open line range
// the table occupies and the indent it is written at. start == end means the
// anchor was found but the table has not been written yet, and a start of -1
// means there is no anchor at all.
func findRollTable(lines []string) (int, int, string) {
	anchor := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(strings.ToUpper(t), "#+NAME:") &&
			strings.EqualFold(strings.TrimSpace(t[len("#+NAME:"):]), SessionTableName) {
			anchor = i
			break
		}
	}
	if anchor < 0 {
		for i, l := range lines {
			if lvl, title := headingOf(l); lvl > 0 && strings.EqualFold(stripTags(title), SessionRollsHeading) {
				anchor = i
				break
			}
		}
	}
	if anchor < 0 {
		return -1, -1, ""
	}
	indent := leadingSpace(lines[anchor])
	i := anchor + 1
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#+") || strings.HasPrefix(t, ":") {
			i++
			continue
		}
		break
	}
	if i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
		indent = leadingSpace(lines[i])
		j := i
		for j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), "|") {
			j++
		}
		return i, j, indent
	}
	// The anchor is there but nothing has been rolled yet. Write the table
	// directly under it rather than after whatever blank lines follow.
	return anchor + 1, anchor + 1, indent
}

// findNotesSection returns the half open line range of the body of the notes
// section, or -1 when the file has no notes heading.
func findNotesSection(lines []string) (int, int) {
	return findSection(lines, SessionNotesHeading)
}

// findSection returns the half open line range of the body of a named
// section, or -1 when the file does not have that heading.
func findSection(lines []string, heading string) (int, int) {
	for i, l := range lines {
		lvl, title := headingOf(l)
		if lvl == 0 || !strings.EqualFold(stripTags(title), heading) {
			continue
		}
		j := i + 1
		for j < len(lines) {
			if l2, _ := headingOf(lines[j]); l2 > 0 && l2 <= lvl {
				break
			}
			j++
		}
		return i + 1, j
	}
	return -1, -1
}

func leadingSpace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// SessionInfoFromText builds the list entry for a session file.
func SessionInfoFromText(id, file, text string) SessionInfo {
	info := SessionInfo{Id: id, File: file, Name: SessionName(text, id)}
	info.Date = plainDate(fileKeyword(text, "#+DATE:"))
	if info.Date == "" {
		info.Date = DateFromSessionId(id)
	}
	info.Summary = SessionSummary(text)
	info.Rolls = len(ParseRolls(text))
	info.Notes = len(ParseNotes(text))
	info.Characters = ParseSessionCharacters(text)
	return info
}

var orgDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

// plainDate pulls the day out of an org timestamp so that a session reads as
// "2025-09-05" rather than "[2025-09-05 Fri]".
func plainDate(v string) string {
	if m := orgDate.FindString(v); m != "" {
		return m
	}
	return strings.TrimSpace(v)
}

// SessionDetailFromText reads a whole session file.
func SessionDetailFromText(id, file, text string) SessionDetail {
	d := SessionDetail{SessionInfo: SessionInfoFromText(id, file, text)}
	d.RollLog = ParseRolls(text)
	d.NoteLog = ParseNotes(text)
	d.MarkLog = ParseMarks(text)
	if d.RollLog == nil {
		d.RollLog = []SessionRoll{}
	}
	if d.NoteLog == nil {
		d.NoteLog = []SessionNote{}
	}
	if d.MarkLog == nil {
		d.MarkLog = []SessionMark{}
	}
	return d
}

// SessionTitle is the title given to an unnamed session.
func SessionTitle(name string, dt time.Time) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return fmt.Sprintf("Session %s", dt.Format("Mon 2006-01-02"))
}
