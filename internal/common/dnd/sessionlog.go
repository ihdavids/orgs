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

// SessionDetail is everything recorded in one session.
type SessionDetail struct {
	SessionInfo
	RollLog  []SessionRoll `json:"rollLog"`
	NoteLog  []SessionNote `json:"noteLog"`
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
		cur.Text = strings.TrimRight(strings.Join(body, "\n"), "\n")
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
	for _, k := range known {
		if (ch.Id != "" && k.Id == ch.Id) || (ch.Id == "" && k.Name == ch.Name) {
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

// renderNoteBody pushes the note's own headings two levels down so they nest
// under the entry, and indents everything else to match.
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
	return out
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
	if d.RollLog == nil {
		d.RollLog = []SessionRoll{}
	}
	if d.NoteLog == nil {
		d.NoteLog = []SessionNote{}
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
