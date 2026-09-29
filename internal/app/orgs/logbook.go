package orgs

// What org does when a heading is marked done, and what it writes down.
//
// `ChangeStatus` used to set `Headline.Status` and write the file, and that was
// the whole of it. Org does three more things, and every one of them is
// load-bearing for something orgs already ships:
//
//  1. **It stamps `CLOSED: [...]`**, which is the only record anywhere of when
//     a thing was actually finished. The keyword says it is done; the stamp says
//     when.
//  2. **It writes a line saying which state the heading came from.** That line
//     is what `parseHabitCompletions` reads to draw the agenda's habit graph -
//     a graph that could only ever be populated by Emacs, because nothing in
//     orgs had ever written one.
//  3. **It moves a repeating date on.** `SCHEDULED: <2026-09-28 Mon .+2d>`
//     marked done in Emacs becomes the 30th and goes back to TODO; marked done
//     anywhere else it stayed on the 28th with the keyword stuck on DONE, which
//     is a habit quietly broken by every client orgs has.
//
// Two decisions shape the whole file.
//
// **It is a line edit, not a document rewrite.** `ChangeStatus` went through
// `WriteOutOrgFile`, which serialises the parsed document through go-org - so
// ticking one task off reformatted every drawer and reflowed every table in
// whatever file it lived in. Everything else that writes in here (records,
// attachments, the checklist, voice notes) splices lines for exactly that
// reason, and marking something done had no business being the exception. It
// also means the timestamps are rewritten as text, so a date keeps the spelling
// it was written with.
//
// **Nothing is assumed about what a keyword means.** Which keywords are DONE
// ones comes from the file's own `#+TODO:` line where it has one, so a file
// declaring `| FIXED` gets its CLOSED stamp on FIXED and not on a DONE it has
// never heard of.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// The settings, and the three places they can be said
// ---------------------------------------------------------------------------

// How much is written down about something: nothing, a timestamp, or a
// timestamp with a note kept beside it. Org's three-valued `org-log-done`.
type LogMode int

const (
	LogNone LogMode = iota
	LogTime
	LogNote
)

func parseLogMode(s string, def LogMode) LogMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none", "nil", "off", "false", "no":
		return LogNone
	case "time", "t", "on", "true", "yes":
		return LogTime
	case "note":
		return LogNote
	}
	return def
}

// The resolved answer for one heading: what to write when it is closed, what to
// write when it repeats, and where those lines go.
type logSettings struct {
	done   LogMode
	repeat LogMode
	// The drawer log lines are written into, or "" to write them into the body
	// as a plain list the way org does by default.
	drawer string
	// Log every change between keywords, not only the ones into a done state.
	states        bool
	repeatToState string
}

// The defaults, which are not quite org's out-of-the-box defaults and should not
// be.
//
// Vanilla Emacs has `org-log-done` nil and `org-log-into-drawer` nil, on the
// reasoning that a fresh user should not find their files growing text they did
// not ask for. Neither default makes sense here: the whole complaint this file
// answers is that marking something done in orgs recorded nothing, and the habit
// graph orgs already draws reads its data out of `LOGBOOK` specifically - with
// the drawer off it would go on being a feature only Emacs could feed. So the
// shipped defaults are the ones nearly every org user sets by hand, and every
// one of them can be turned off in the config, in the file, or on the heading.
func defaultLogSettings() logSettings {
	return logSettings{
		done:   LogTime,
		repeat: LogTime,
		drawer: "LOGBOOK",
		states: false,
	}
}

// Everything the server was configured with.
func configuredLogSettings() logSettings {
	set := defaultLogSettings()
	c := Conf()
	if c == nil || c.Server == nil {
		return set
	}
	l := c.Server.Log
	set.done = parseLogMode(l.Done, set.done)
	set.repeat = parseLogMode(l.Repeat, set.repeat)
	set.states = l.States
	set.repeatToState = strings.TrimSpace(l.RepeatToState)
	// An empty drawer name is how "into the body" is written, so it cannot be
	// told from a setting nobody wrote. The yaml is only allowed to change it
	// when it says something.
	if d := strings.TrimSpace(l.IntoDrawer); d != "" {
		set.drawer = drawerName(d)
	}
	return set
}

// `logdrawer` and `org-log-into-drawer: t` both mean LOGBOOK; anything else is
// the name of the drawer to use.
func drawerName(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "t", "true", "yes", "on", "logbook":
		return "LOGBOOK"
	case "nil", "none", "off", "false", "no", "":
		return ""
	}
	return strings.ToUpper(strings.Trim(strings.TrimSpace(s), ":"))
}

// The `#+STARTUP:` words org defines for this, applied over whatever the server
// was configured with. A file that says nothing changes nothing.
var startupLogWords = map[string]func(*logSettings){
	"logdone":          func(s *logSettings) { s.done = LogTime },
	"nologdone":        func(s *logSettings) { s.done = LogNone },
	"lognotedone":      func(s *logSettings) { s.done = LogNote },
	"logrepeat":        func(s *logSettings) { s.repeat = LogTime },
	"nologrepeat":      func(s *logSettings) { s.repeat = LogNone },
	"lognoterepeat":    func(s *logSettings) { s.repeat = LogNote },
	"logdrawer":        func(s *logSettings) { s.drawer = "LOGBOOK" },
	"nologdrawer":      func(s *logSettings) { s.drawer = "" },
	"logstatechange":   func(s *logSettings) { s.states = true },
	"nologstatechange": func(s *logSettings) { s.states = false },
}

// What this file asks for, over what the server was configured with.
func fileLogSettings(f *common.OrgFile) logSettings {
	set := configuredLogSettings()
	if f == nil || f.Doc == nil {
		return set
	}
	for _, w := range strings.Fields(f.Doc.Get("STARTUP")) {
		if apply, ok := startupLogWords[strings.ToLower(w)]; ok {
			apply(&set)
		}
	}
	// #+PROPERTY: LOG_INTO_DRAWER NAME
	for _, p := range strings.Split(f.Doc.Get("PROPERTY"), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(p), " "); ok &&
			strings.EqualFold(k, "LOG_INTO_DRAWER") {
			set.drawer = drawerName(v)
		}
	}
	return set
}

// A heading's own `:LOGGING:` and `:LOG_INTO_DRAWER:`, over its file's.
//
// Org's `:LOGGING:` holds the same words `#+STARTUP:` does, and `nil` or `none`
// in it turns the lot off for that heading - which is the shape somebody wants
// for the one task they do not want a history of.
func applyHeadingLogProps(set logSettings, props map[string]string) logSettings {
	get := func(k string) (string, bool) {
		for key, v := range props {
			if strings.EqualFold(key, k) {
				return v, true
			}
		}
		return "", false
	}
	if v, ok := get("LOGGING"); ok {
		words := strings.Fields(v)
		if len(words) == 1 {
			switch strings.ToLower(words[0]) {
			case "nil", "none", "off":
				set.done, set.repeat, set.states = LogNone, LogNone, false
				return set
			}
		}
		for _, w := range words {
			if apply, ok := startupLogWords[strings.ToLower(w)]; ok {
				apply(&set)
			}
		}
	}
	if v, ok := get("LOG_INTO_DRAWER"); ok {
		set.drawer = drawerName(v)
	}
	return set
}

// ---------------------------------------------------------------------------
// Keyword cookies
// ---------------------------------------------------------------------------

// One todo keyword as a `#+TODO:` line declares it.
//
// `NEXT(n!)` is a keyword called NEXT, reachable by pressing n, which logs a
// timestamp when a heading enters it. The cookie is how org says "log this one"
// for a single keyword, so it has to survive being read - and it did not:
// `ParseTodoStates` used to hand back the whole word, so a file written the way
// the org manual writes them had a keyword literally called `NEXT(n!)` and
// every status this server would accept was one no reader would recognise.
type TodoKeyword struct {
	Name    string
	Key     string
	OnEnter LogMode
	OnLeave LogMode
}

var keywordCookieRe = regexp.MustCompile(`^([^(]+)\(([^)]*)\)$`)

// One word of a `#+TODO:` line.
func parseTodoKeyword(word string) TodoKeyword {
	word = strings.TrimSpace(word)
	kw := TodoKeyword{Name: word}
	m := keywordCookieRe.FindStringSubmatch(word)
	if m == nil {
		return kw
	}
	kw.Name = strings.TrimSpace(m[1])
	cookie := m[2]
	// The cookie is an access key, then any of `!` and `@` for entering, then
	// optionally `/` and the same for leaving.
	enter, leave, _ := strings.Cut(cookie, "/")
	for _, part := range []struct {
		text string
		into *LogMode
	}{{enter, &kw.OnEnter}, {leave, &kw.OnLeave}} {
		for _, c := range part.text {
			switch c {
			case '!':
				if *part.into == LogNone {
					*part.into = LogTime
				}
			case '@':
				*part.into = LogNote
			default:
				if part.into == &kw.OnEnter && kw.Key == "" {
					kw.Key = string(c)
				}
			}
		}
	}
	return kw
}

// Every keyword a `#+TODO:` line declares, and which of them are done states.
func parseTodoKeywords(spec string) (active, done []TodoKeyword) {
	parts := strings.SplitN(spec, "|", 2)
	for _, w := range strings.Fields(parts[0]) {
		active = append(active, parseTodoKeyword(w))
	}
	if len(parts) > 1 {
		for _, w := range strings.Fields(parts[1]) {
			done = append(done, parseTodoKeyword(w))
		}
	}
	// Org's rule when there is no bar: the last keyword is the done one.
	if len(parts) == 1 && len(active) > 1 {
		done = append(done, active[len(active)-1])
		active = active[:len(active)-1]
	}
	return active, done
}

// The keyword spec in force for a file - its own `#+TODO:` where it has one.
func todoSpecFor(f *common.OrgFile) string {
	if f != nil && f.Doc != nil {
		if s := f.Doc.Get("TODO"); strings.TrimSpace(s) != "" {
			return s
		}
	}
	if c := Conf(); c != nil && c.Server != nil {
		return c.Server.DefaultTodoStates
	}
	return ""
}

// Whether this keyword is one of the file's done states, and the cookie it
// carries.
func keywordInfo(f *common.OrgFile, name string) (kw TodoKeyword, isDone bool, known bool) {
	active, done := parseTodoKeywords(todoSpecFor(f))
	for _, k := range done {
		if k.Name == name {
			return k, true, true
		}
	}
	for _, k := range active {
		if k.Name == name {
			return k, false, true
		}
	}
	return TodoKeyword{Name: name}, false, false
}

// The keyword a repeating heading goes back to.
func repeatTarget(f *common.OrgFile, set logSettings, props map[string]string, from string) string {
	for key, v := range props {
		if strings.EqualFold(key, "REPEAT_TO_STATE") && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	active, _ := parseTodoKeywords(todoSpecFor(f))
	switch strings.ToLower(set.repeatToState) {
	case "":
		// Org's own default: the first keyword of the sequence.
	case "previous":
		for _, k := range active {
			if k.Name == from {
				return from
			}
		}
	default:
		return set.repeatToState
	}
	if len(active) > 0 {
		return active[0].Name
	}
	return ""
}

// ---------------------------------------------------------------------------
// Timestamps, as text
// ---------------------------------------------------------------------------

// An org timestamp, active or inactive, with its repeater and warning cookies
// kept separate so they can be put back untouched.
//
// This is deliberately a text-level match rather than a trip through go-org's
// date writer: a date is being edited inside a line that is otherwise to be left
// exactly as it was written, and reformatting somebody's timestamp because their
// task came due is not something a client asked for.
var timestampRe = regexp.MustCompile(
	`([<\[])(\d{4})-(\d{2})-(\d{2})(\s+[A-Za-z]{2,})?(\s+\d{1,2}:\d{2}(?:-\d{1,2}:\d{2})?)?` +
		// The repeater, with org-habit's optional slack half: `.+1d/3d` is every
		// day, and definitely overdue after three. Leaving it out of the pattern
		// did not make the slack be ignored - it made the whole timestamp fail to
		// match, so a habit written the way org-habit documents never repeated at
		// all and its keyword stayed on DONE. The group is carried through
		// untouched on the way out, so the spelling survives the date moving.
		`((?:\s+[.+]{1,2}\d+[hdwmy](?:/\d+[hdwmy])?)?)((?:\s+-+\d+[hdwmy])?)\s*([>\]])`)

// A parsed repeater: `.+2d` is pre ".+", every 2, unit "d".
type repeater struct {
	pre  string
	n    int
	unit string
}

var repeaterRe = regexp.MustCompile(`^\s*([.+]{1,2})(\d+)([hdwmy])(?:/(\d+)([hdwmy]))?$`)

func parseRepeater(s string) (repeater, bool) {
	m := repeaterRe.FindStringSubmatch(s)
	if m == nil {
		return repeater{}, false
	}
	n, err := strconv.Atoi(m[2])
	if err != nil || n <= 0 {
		return repeater{}, false
	}
	return repeater{pre: m[1], n: n, unit: m[3]}, true
}

// One interval on from a date.
func (self repeater) step(t time.Time) time.Time {
	switch self.unit {
	case "h":
		return t.Add(time.Duration(self.n) * time.Hour)
	case "d":
		return t.AddDate(0, 0, self.n)
	case "w":
		return t.AddDate(0, 0, 7*self.n)
	case "m":
		return t.AddDate(0, self.n, 0)
	case "y":
		return t.AddDate(self.n, 0, 0)
	}
	return t
}

// Where a repeating date goes next, which is three different sums depending on
// how the repeater was written - and the difference between them is the whole
// reason org has three spellings:
//
//   - `+2d` counts from the date that was there. Finish a week late and the new
//     date is still in the past, which is what somebody tracking "every second
//     day, and I want to see the ones I missed" is asking for.
//   - `++2d` counts from the date that was there but keeps going until it is in
//     the future - the same schedule, without the backlog.
//   - `.+2d` counts from today. "Two days after I actually did it."
func (self repeater) next(from, now time.Time) time.Time {
	switch self.pre {
	case ".+":
		// From today - but at the time of day the stamp was written with, not at
		// whatever time it happened to be ticked off. A task due at nine every
		// morning is due at nine; letting the clock follow the completion would
		// walk a daily reminder round the day over a week of being a bit late.
		// An hourly repeater is the exception, since for that the time of day is
		// the whole of what is repeating.
		base := now
		if self.unit != "h" {
			base = time.Date(now.Year(), now.Month(), now.Day(),
				from.Hour(), from.Minute(), 0, 0, now.Location())
		}
		return self.step(base)
	case "++":
		t := self.step(from)
		// A guard as well as a condition: a zero-length step would spin here,
		// and `parseRepeater` refusing n <= 0 is the only thing stopping it.
		for i := 0; i < 1000 && !t.After(now); i++ {
			t = self.step(t)
		}
		return t
	default:
		return self.step(from)
	}
}

// The date a timestamp match holds, and whether it carries a time of day.
func stampTime(m []string) (time.Time, bool, bool) {
	y, err1 := strconv.Atoi(m[2])
	mo, err2 := strconv.Atoi(m[3])
	d, err3 := strconv.Atoi(m[4])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, false, false
	}
	hh, mm := 0, 0
	haveTime := false
	if t := strings.TrimSpace(m[6]); t != "" {
		haveTime = true
		first, _, _ := strings.Cut(t, "-")
		hs, ms, _ := strings.Cut(first, ":")
		hh, _ = strconv.Atoi(strings.TrimSpace(hs))
		mm, _ = strconv.Atoi(strings.TrimSpace(ms))
	}
	return time.Date(y, time.Month(mo), d, hh, mm, 0, 0, time.Local), haveTime, true
}

// Rewrite one timestamp onto a new day, keeping everything else it said - the
// brackets, the time of day, the repeater, the warning period.
//
// The weekday name is regenerated rather than kept, because it is the one part
// of a timestamp that is derived: carrying `Mon` onto a Wednesday would leave a
// date that reads wrong everywhere and that Emacs would silently correct on the
// next edit, making the file look like it had changed when it had not.
func rewriteStamp(m []string, to time.Time) string {
	out := m[1] + to.Format("2006-01-02")
	if strings.TrimSpace(m[5]) != "" {
		out += " " + to.Format("Mon")
	}
	if t := m[6]; strings.TrimSpace(t) != "" {
		out += " " + to.Format("15:04")
		// A range keeps its length: the end is moved by however much the start
		// moved, rather than being copied across. Copying is right whenever the
		// time of day did not change and silently wrong when it did, which is
		// exactly the case an hourly repeater is.
		if _, rest, isRange := strings.Cut(strings.TrimSpace(t), "-"); isRange {
			if from, _, ok := stampTime(m); ok {
				if endT, err := time.Parse("15:04", strings.TrimSpace(rest)); err == nil {
					end := time.Date(from.Year(), from.Month(), from.Day(),
						endT.Hour(), endT.Minute(), 0, 0, from.Location())
					out += "-" + end.Add(to.Sub(from)).Format("15:04")
				} else {
					out += "-" + rest
				}
			} else {
				out += "-" + rest
			}
		}
	}
	out += m[7] + m[8] + m[9]
	return out
}

// Move every repeating date in one planning line on, and say whether anything
// moved.
//
// Only a date carrying a repeater is touched. A heading can have a repeating
// schedule and a fixed deadline, and moving the deadline because the schedule
// repeated would quietly turn a real due date into a rolling one.
func advanceRepeaters(line string, now time.Time) (string, bool) {
	moved := false
	out := timestampRe.ReplaceAllStringFunc(line, func(s string) string {
		m := timestampRe.FindStringSubmatch(s)
		if m == nil {
			return s
		}
		rep, ok := parseRepeater(m[7])
		if !ok {
			return s
		}
		from, _, ok := stampTime(m)
		if !ok {
			return s
		}
		moved = true
		return rewriteStamp(m, rep.next(from, now))
	})
	return out, moved
}

// Does this line hold a repeating SCHEDULED or DEADLINE?
func hasRepeater(line string) bool {
	for _, m := range timestampRe.FindAllStringSubmatch(line, -1) {
		if _, ok := parseRepeater(m[7]); ok {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Writing the lines
// ---------------------------------------------------------------------------

// Org's own wording for a state change, padding and all - `State "DONE"
// from "NEXT"  [2026-09-28 Mon 14:32]`. Written the way org writes it so a
// file orgs has touched reads like one Emacs has, and so the regexes on both
// sides find the same thing.
func stateLogLine(ind, to, from string, when time.Time) string {
	entry := fmt.Sprintf(`%s- State %-12s from %-12s %s`, ind,
		`"`+to+`"`, `"`+from+`"`, orgStamp(when))
	return strings.TrimRight(entry, " ")
}

func closingNoteLine(ind string, when time.Time) string {
	return ind + "- CLOSING NOTE " + orgStamp(when)
}

// Put log entries where this heading's settings say they go: into a drawer, or
// into the body as a plain list.
//
// Newest first in both cases, which is org's default (`org-log-states-ordered`
// nil) and the order that matters: the line somebody wants is nearly always the
// last thing that happened, and a drawer that grows downwards puts it at the
// bottom of a fold.
func logInto(lines []string, from, to int, ind, drawer string, entry []string) []string {
	if len(entry) == 0 {
		return lines
	}
	if drawer == "" {
		return splice(lines, afterHead(lines, from, to), entry)
	}
	if start, _, ok := drawerAt(lines, from, to, drawer); ok {
		return splice(lines, start+1, entry)
	}
	add := append([]string{ind + ":" + drawer + ":"}, entry...)
	add = append(add, ind+":END:")
	return splice(lines, afterHead(lines, from, to), add)
}

// The first line under a heading that is neither planning nor a drawer - where
// a log entry goes when there is no drawer to put it in.
func afterHead(lines []string, from, to int) int {
	at := from + 1
	for at <= to && at < len(lines) && planningRe.MatchString(lines[at]) {
		at++
	}
	if _, pe, ok := drawerAt(lines, from, to, "PROPERTIES"); ok && pe+1 > at {
		at = pe + 1
	}
	return at
}

// ---------------------------------------------------------------------------
// The planning line
// ---------------------------------------------------------------------------

var closedOnLineRe = regexp.MustCompile(`\s*CLOSED:\s*\[[^\]]*\]`)

// Stamp `CLOSED: [...]` on the heading, putting it where org puts it: at the
// front of the planning line, which is one line holding all of a heading's
// planning.
//
// Writing it on a line of its own would have been easier and is wrong in both
// directions - Emacs writes one line and would leave a second one lying in the
// body, and a reader of these files has every right to expect the shape org
// produces.
func stampClosed(lines []string, from, to int, ind string, when time.Time) []string {
	stamp := "CLOSED: " + orgStamp(when)
	for i := from + 1; i <= to && i < len(lines); i++ {
		if !planningRe.MatchString(lines[i]) {
			break
		}
		if closedOnLineRe.MatchString(lines[i]) {
			return closedRewritten(lines, i, stamp)
		}
		trimmed := strings.TrimLeft(lines[i], " \t")
		lines[i] = ind + stamp + " " + trimmed
		return lines
	}
	return splice(lines, from+1, []string{ind + stamp})
}

func closedRewritten(lines []string, i int, stamp string) []string {
	lines[i] = closedOnLineRe.ReplaceAllString(lines[i], "")
	ind := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
	rest := strings.TrimSpace(lines[i])
	if rest == "" {
		lines[i] = ind + stamp
	} else {
		lines[i] = ind + stamp + " " + rest
	}
	return lines
}

// Take any CLOSED stamp off the heading, and the planning line with it when
// that is all it held.
//
// Two things need this: a heading moved back out of a done state, which is no
// longer closed; and a repeating heading, which org deliberately does not leave
// a closing time on - it was not closed, it came round again.
func unstampClosed(lines []string, from, to int) []string {
	for i := from + 1; i <= to && i < len(lines); i++ {
		if !planningRe.MatchString(lines[i]) {
			break
		}
		if !closedOnLineRe.MatchString(lines[i]) {
			continue
		}
		stripped := closedOnLineRe.ReplaceAllString(lines[i], "")
		if strings.TrimSpace(stripped) == "" {
			return append(lines[:i], lines[i+1:]...)
		}
		ind := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
		lines[i] = ind + strings.TrimSpace(stripped)
		return lines
	}
	return lines
}

// ---------------------------------------------------------------------------
// The heading line, and its properties
// ---------------------------------------------------------------------------

var headStarsRe = regexp.MustCompile(`^(\*+)(\s+)(.*)$`)

// Put a different keyword on a heading.
//
// The old keyword is passed in rather than guessed at, and that is the whole
// trick: a headline pattern that decides what a keyword looks like will read
// `* API redesign` as a heading whose keyword is API. Knowing what the parser
// found there means the word can be matched exactly, and a heading with no
// keyword at all is the case where one is inserted rather than replaced.
func replaceHeadlineStatus(line, from, to string) (string, bool) {
	m := headStarsRe.FindStringSubmatch(line)
	if m == nil {
		return line, false
	}
	stars, gap, rest := m[1], m[2], m[3]
	if from != "" && strings.HasPrefix(rest, from) {
		after := rest[len(from):]
		if after == "" || after[0] == ' ' || after[0] == '\t' {
			rest = strings.TrimLeft(after, " \t")
		}
	}
	if to != "" {
		if rest == "" {
			rest = to
		} else {
			rest = to + " " + rest
		}
	}
	return stars + gap + rest, true
}

// The properties written in this heading's own drawer.
//
// Read off the lines rather than from `Headline.Properties`, which go-org leaves
// nil for a drawer written in column zero - the trap that runs through this
// whole codebase. The settings this is for are exactly the ones somebody would
// write by hand, so meeting them written by hand is the common case.
func propsFromLines(lines []string, from, to int) map[string]string {
	out := map[string]string{}
	ps, pe, ok := drawerAt(lines, from, to, "PROPERTIES")
	if !ok {
		return out
	}
	for i := ps + 1; i < pe && i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(t, ":") {
			continue
		}
		key, val, found := strings.Cut(t[1:], ":")
		if !found {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return out
}

// Does this heading's planning hold a repeating date?
func planningHasRepeater(lines []string, from, to int) bool {
	for i := from + 1; i <= to && i < len(lines); i++ {
		if !planningRe.MatchString(lines[i]) {
			break
		}
		if hasRepeater(lines[i]) {
			return true
		}
	}
	return false
}

// Move every repeating date in this heading's planning on.
func advancePlanning(lines []string, from, to int, now time.Time) bool {
	moved := false
	for i := from + 1; i <= to && i < len(lines); i++ {
		if !planningRe.MatchString(lines[i]) {
			break
		}
		if out, did := advanceRepeaters(lines[i], now); did {
			lines[i] = out
			moved = true
		}
	}
	return moved
}

// The strongest of several log modes. A note says everything a timestamp says
// and more, so anything asking for one wins.
func strongest(modes ...LogMode) LogMode {
	out := LogNone
	for _, m := range modes {
		if m > out {
			out = m
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// What happens when a heading changes state
// ---------------------------------------------------------------------------

// StatusChangeResult says what was done beyond setting the keyword, so a caller
// can tell a client that its DONE became a TODO again and why.
type StatusChangeResult struct {
	// The keyword actually written, which is not the one that was asked for when
	// the heading repeats.
	Status string
	// Whether a repeating date was moved on.
	Repeated bool
	// The lines that were logged, if any.
	Logged []string
	// Whether a CLOSED stamp was written or taken off.
	Closed   bool
	Unclosed bool
}

// Apply org's own meaning of a state change to one heading, as a line edit.
//
// The order the edits are done in is the only subtle thing here, and it is
// forced: every one of them that adds or removes a line moves everything below
// it, so the heading's extent is worked out again after each. Doing the log
// entry before the CLOSED stamp - or measuring once at the top and using that
// measurement throughout - writes the second edit into the wrong heading, which
// is a corruption that reports success.
func applyStatusChange(f *common.OrgFile, sec *org.Section, to, note string, now time.Time) (StatusChangeResult, error) {
	var res StatusChangeResult
	if f == nil || sec == nil || sec.Headline == nil {
		return res, fmt.Errorf("no heading to change")
	}
	lines, from, end, ok := recordLines(f.Filename, sec)
	if !ok {
		return res, fmt.Errorf("could not read %s", f.Filename)
	}
	lvl := sec.Headline.Lvl
	ind := indentOf(lvl)
	was := sec.Headline.Status
	res.Status = to

	if was == to {
		// Nothing asked for. Writing the file anyway would bump its version and
		// invalidate every per-file index for a change nobody made.
		return res, nil
	}

	set := applyHeadingLogProps(fileLogSettings(f), propsFromLines(lines, from, end))
	_, toIsDone, _ := keywordInfo(f, to)
	_, wasDone, _ := keywordInfo(f, was)
	kwTo, _, _ := keywordInfo(f, to)
	kwWas, _, _ := keywordInfo(f, was)

	// A heading with a repeater that reaches a done state does not become done -
	// it comes round again. Org's rule, and the one that makes a habit a habit.
	repeating := toIsDone && planningHasRepeater(lines, from, end)

	// What gets written down. The per-keyword cookies are part of this and are
	// not overridden by the global switches: `NEXT(n!)` is somebody saying "log
	// this one" about one keyword.
	cookieMode := strongest(kwTo.OnEnter, kwWas.OnLeave)
	stateMode := cookieMode
	if set.states {
		stateMode = strongest(stateMode, LogTime)
	}
	switch {
	case repeating:
		stateMode = strongest(stateMode, set.repeat)
	case toIsDone:
		stateMode = strongest(stateMode, set.done)
	}

	// 1. The keyword. A repeating heading goes back to a live state instead.
	written := to
	if repeating {
		if back := repeatTarget(f, set, propsFromLines(lines, from, end), was); back != "" {
			written = back
		}
	}
	head, ok := replaceHeadlineStatus(lines[from], was, written)
	if !ok {
		return res, fmt.Errorf("%s:%d is not a heading", f.Filename, from+1)
	}
	lines[from] = head
	res.Status = written

	// 2. The dates. No line count changes here, so no re-measuring yet.
	if repeating {
		res.Repeated = advancePlanning(lines, from, end, now)
	}

	// 3. The CLOSED stamp. A repeat is explicitly not a closure, so any stamp
	//    left from a previous pass comes off - otherwise a habit would carry the
	//    date it was last completed as though it were finished for good.
	switch {
	case repeating, wasDone && !toIsDone:
		before := len(lines)
		lines = unstampClosed(lines, from, end)
		res.Unclosed = len(lines) != before
	case toIsDone && set.done != LogNone:
		before := len(lines)
		lines = stampClosed(lines, from, end, ind, now)
		res.Closed = true
		if len(lines) != before {
			lines, from, end = reread(lines, from, lvl)
		}
	}
	lines, from, end = reread(lines, from, lvl)

	// 4. LAST_REPEAT, which is how org records that a repeat happened at all -
	//    the dates afterwards say where it is going, never where it has been.
	if res.Repeated {
		var ps, pe int
		lines, ps, pe, end = ensurePropertyDrawer(lines, from, end, ind)
		lines, _ = setPropIn(lines, ps, pe, ind, "LAST_REPEAT", orgStamp(now))
		lines, from, end = reread(lines, from, lvl)
	}

	// 5. The log entry, last, because it is the only edit that does not have to
	//    find anything afterwards.
	if stateMode != LogNone {
		// One indent for the drawer and its lines alike, which is what org does
		// and what the record logbooks in here already do - a drawer indented
		// further than the property drawer above it reads as nested inside it.
		entry := []string{stateLogLine(ind, to, was, now)}
		if stateMode == LogNote && strings.TrimSpace(note) != "" {
			for _, l := range strings.Split(strings.TrimRight(note, "\n"), "\n") {
				entry = append(entry, ind+"  "+strings.TrimSpace(l))
			}
		}
		lines = logInto(lines, from, end, ind, set.drawer, entry)
		res.Logged = entry
	}

	if err := writeLines(f.Filename, lines); err != nil {
		return res, err
	}
	return res, nil
}
