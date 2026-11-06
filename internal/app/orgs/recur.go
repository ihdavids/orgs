package orgs

// org-recur (features.recur): a recurrence written in the heading's title
// between bars, `* TODO Water plants |+2|`, `|Mon,Thu|`, `|1,15|`, `|wkdy|`.
// When the heading is marked done it is not closed: its SCHEDULED (or its
// DEADLINE, when that is the only date it has; a new SCHEDULED when it has
// neither) moves to the rule's next date after today and the keyword goes
// back, as a repeat does - logbook State line, :LAST_REPEAT:, CLOSED taken
// off. The rules are read by internal/common/recur.go.
//
// The hook is inside applyStatusChange (logbook.go, step 2), so it is part
// of the one write every keyword change makes: kanban, `orgs todo`, MCP,
// worg, VS Code.
//
// With an org repeater on the same stamp (`SCHEDULED: <… +1w> |Mon|`) the
// rule decides where the stamp goes, once; the cookie is kept as written but
// not stepped as well. Any other repeating date on the heading (a DEADLINE
// with its own +1m) moves on by org's rule as it would without org-recur.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func recurWeekdays() string {
	if s := Conf().Server; s != nil {
		return s.Recur.Weekdays
	}
	return ""
}

// recurRuleFor is the rule a headline line carries, when the feature is on.
func recurRuleFor(headline string) (common.RecurRule, bool) {
	if !featureOn("recur") {
		return common.RecurRule{}, false
	}
	return common.RecurOf(headline, recurWeekdays())
}

// isRecurring is the query function IsRecurring().
func isRecurring(sec *org.Section) bool {
	if sec == nil || sec.Headline == nil || !featureOn("recur") {
		return false
	}
	_, ok := common.RecurOf(headlineTitle(sec), recurWeekdays())
	return ok
}

// recurStamp is one timestamp match moved to d: the rule's time of day when it
// gives one, else the stamp's own (a range keeps its length).
func recurStamp(m []string, d common.RecurDate) string {
	if !d.HasTime {
		from, _, ok := stampTime(m)
		if !ok {
			from = d.Date
		}
		to := time.Date(d.Date.Year(), d.Date.Month(), d.Date.Day(), from.Hour(), from.Minute(), 0, 0, from.Location())
		return rewriteStamp(m, to)
	}
	return m[1] + d.String() + m[7] + m[8] + m[9]
}

// recurPlanning moves the heading's date by its rule, as a line edit of its
// planning line, and says what it did. added is whether a line was added (a
// SCHEDULED for a heading with no planning line), after which the caller
// must reread.
func recurPlanning(lines []string, from, end int, ind string, rule common.RecurRule, now time.Time) (out []string, msg string, added bool) {
	d := rule.Next(now)
	target := ""
	for i := from + 1; i <= end && i < len(lines); i++ {
		if !planningRe.MatchString(lines[i]) {
			break
		}
		if target != "" {
			lines[i], _ = advanceRepeaters(lines[i], now)
			continue
		}
		for _, key := range []string{"SCHEDULED", "DEADLINE"} {
			loc := planningKeyRe(key).FindStringIndex(lines[i])
			if loc == nil {
				continue
			}
			target = key
			seg := lines[i][loc[0]:loc[1]]
			done := false
			seg = timestampRe.ReplaceAllStringFunc(seg, func(st string) string {
				m := timestampRe.FindStringSubmatch(st)
				if done || m == nil {
					return st
				}
				done = true
				return recurStamp(m, d)
			})
			before, _ := advanceRepeaters(lines[i][:loc[0]], now)
			after, _ := advanceRepeaters(lines[i][loc[1]:], now)
			lines[i] = before + seg + after
			break
		}
		if target == "" {
			lines[i], _ = advanceRepeaters(lines[i], now)
		}
	}
	if target == "" {
		target = "SCHEDULED"
		before := len(lines)
		lines = setPlanning(lines, from, ownLinesEnd(lines, from, end), ind, "SCHEDULED", "<"+d.String()+">")
		added = len(lines) != before
	}
	msg = fmt.Sprintf("|%s| (%s): %s moved to <%s>", rule.Text, rule.Words(), target, d.String())
	return lines, msg, added
}

// ---------------------------------------------------------------------------
// The endpoints
// ---------------------------------------------------------------------------

func registerRecurRoutes(api *mux.Router) {
	api.HandleFunc("/recur", RequestRecur).Methods("GET")
	api.HandleFunc("/recur/explain", RequestRecurExplain).Methods("GET")
	api.HandleFunc("/recur/set", PostRecurSet).Methods("POST")
}

// RecurItem is one heading with a |rule| in its title.
type RecurItem struct {
	Hash     string
	Headline string // the title without its rule
	Title    string // as written
	Filename string
	Line     int
	Status   string
	Rule     string // between the bars
	Words    string
	Err      string `json:",omitempty"` // the bars hold something that is not a rule
	Stamp    string // SCHEDULED or DEADLINE: the date done moves; "" for none yet (done adds a SCHEDULED)
	Current  string `json:",omitempty"` // that date as it is now
	Next     string `json:",omitempty"` // where done today moves it
	Upcoming []string
}

func recurItemOf(f *common.OrgFile, sec *org.Section, now time.Time, n int) (RecurItem, bool) {
	title := strings.TrimSpace(headlineTitle(sec))
	inner, _, _, ok := common.FindRecur(title)
	if !ok {
		return RecurItem{}, false
	}
	it := RecurItem{Hash: sec.Hash, Headline: common.StripRecur(title), Title: title, Filename: f.Filename,
		Line: sec.Headline.Pos.Row, Status: sec.Headline.Status, Rule: strings.TrimSpace(inner), Upcoming: []string{}}
	switch h := sec.Headline; {
	case h.Scheduled != nil && h.Scheduled.Date != nil:
		it.Stamp, it.Current = "SCHEDULED", h.Scheduled.Date.ToDate()
	case h.Deadline != nil && h.Deadline.Date != nil:
		it.Stamp, it.Current = "DEADLINE", h.Deadline.Date.ToDate()
	}
	rule, err := common.ParseRecur(inner, recurWeekdays())
	if err != nil {
		it.Err = err.Error()
		return it, true
	}
	it.Words = rule.Words()
	for i, d := range rule.NextN(now, n) {
		if i == 0 {
			it.Next = d.String()
		}
		it.Upcoming = append(it.Upcoming, d.String())
	}
	return it, true
}

/* SDOC: API
* GET /recur — Every Heading With An org-recur Rule

	#+BEGIN_EXAMPLE
	GET /recur?n=3
	-> {"Ok": true, "Weekdays": "mon,tue,wed,thu,fri", "Items": [{"Hash": "...",
	    "Headline": "Water plants", "Title": "Water plants |+2|", "Filename": "...",
	    "Line": 12, "Status": "TODO", "Rule": "+2", "Words": "every 2 days",
	    "Stamp": "SCHEDULED", "Current": "<2026-11-04 Wed>", "Next": "2026-11-06 Fri",
	    "Upcoming": ["2026-11-06 Fri", "2026-11-08 Sun", "2026-11-10 Tue"]}]}
	#+END_EXAMPLE

	Every heading whose title has =|...|= in it (=features: recur:=). =Next= is
	where marking it done today moves its =Stamp=; =Upcoming= the next =n=
	(default 3, at most 20) dates if it is done on each. Bars holding something
	that is not a rule come back with =Err= and no dates. Only headings in an
	open keyword (or none) are listed, unless =all=1=.
EDOC */
func RequestRecur(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "recur") {
		return
	}
	n := 3
	if v, err := strconv.Atoi(r.URL.Query().Get("n")); err == nil && v > 0 && v <= 20 {
		n = v
	}
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	now := time.Now()
	items := []RecurItem{}
	walkSections(func(f *common.OrgFile, sec *org.Section) {
		if !all && sec.Headline.Status != "" {
			if _, isDone, _ := keywordInfo(f, sec.Headline.Status); isDone {
				return
			}
		}
		if it, ok := recurItemOf(f, sec, now, n); ok {
			items = append(items, it)
		}
	})
	wd := recurWeekdays()
	if strings.TrimSpace(wd) == "" {
		wd = common.DefaultRecurWeekdays
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Weekdays": wd, "Items": items})
}

/* SDOC: API
* GET /recur/explain — What An org-recur Rule Means

	#+BEGIN_EXAMPLE
	GET /recur/explain?rule=|1,15|&n=4
	-> {"Ok": true, "Rule": "1,15", "Words": "the 1st and 15th of each month",
	    "Next": ["2026-11-15 Sun", "2026-12-01 Tue", "2026-12-15 Tue", "2027-01-01 Fri"]}
	#+END_EXAMPLE

	The rule's next =n= dates (default 5, at most 50) from today, each one
	where done on the one before it moves the heading. The bars are optional.
	A rule that does not read answers =Ok: false= and why. Works with
	=features: recur:= off - it only reads.
EDOC */
func RequestRecurExplain(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n := 5
	if v, err := strconv.Atoi(q.Get("n")); err == nil && v > 0 && v <= 50 {
		n = v
	}
	rule, err := common.ParseRecur(q.Get("rule"), recurWeekdays())
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	next := []string{}
	for _, d := range rule.NextN(time.Now(), n) {
		next = append(next, d.String())
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Rule": rule.Text, "Words": rule.Words(), "Next": next})
}

// headlineTitleText is the title part of a headline line: no stars,
// keyword, priority or tags (setHeadline's reading of it).
func headlineTitleText(line, status string) string {
	m := headlineHeadRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	rest := line[len(m[0]):]
	if loc := headlineTagsRe.FindStringSubmatchIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}
	if status != "" && (rest == status || strings.HasPrefix(rest, status+" ") || strings.HasPrefix(rest, status+"\t")) {
		rest = strings.TrimLeft(rest[len(status):], " \t")
	}
	if pm := headlinePrioRe.FindStringSubmatch(rest); pm != nil {
		rest = rest[len(pm[0]):]
	}
	return strings.TrimRight(rest, " \t")
}

/* SDOC: API
* POST /recur/set — Set Or Clear A Heading's org-recur Rule

	#+BEGIN_EXAMPLE
	POST /recur/set   {"Hash": "...", "Rule": "Mon,Fri"}
	-> {"Ok": true, "Msg": "|Mon,Fri|: every Monday and Friday, next 2026-11-06 Fri"}
	POST /recur/set   {"Hash": "...", "Rule": ""}
	-> {"Ok": true, "Msg": "rule taken off"}
	#+END_EXAMPLE

	Writes the rule into the heading's title (in place of the one there, else
	at the end of the title), or takes it off. The bars are optional. Only the
	headline line changes; the keyword, priority and tags stay as written,
	right-aligned tags ending where they ended. The heading's hash changes
	with its title. A rule that does not read is refused.
EDOC */
func PostRecurSet(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "recur") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct{ Hash, Rule string }
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	msg := "rule taken off"
	if strings.TrimSpace(strings.Trim(strings.TrimSpace(req.Rule), "|")) != "" {
		rule, err := common.ParseRecur(req.Rule, recurWeekdays())
		if err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
		if strings.Contains(rule.Text, "|") {
			dynJson(w, common.Result{Msg: "a rule has no | inside it"})
			return
		}
		msg = fmt.Sprintf("|%s|: %s, next %s", rule.Text, rule.Words(), rule.Next(time.Now()).String())
	}
	sec := GetDb().FindByHash(req.Hash)
	if sec == nil || sec.Headline == nil {
		dynJson(w, common.Result{Msg: "no heading with that hash - it may have moved; refresh and try again"})
		return
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		dynJson(w, common.Result{Msg: "could not find the file that heading is in"})
		return
	}
	lines, from, _, ok := recordLines(f.Filename, sec)
	if !ok {
		dynJson(w, common.Result{Msg: "could not read " + f.Filename})
		return
	}
	title := headlineTitleText(lines[from], sec.Headline.Status)
	newTitle := common.SetRecurIn(title, req.Rule)
	if newTitle == title {
		dynJson(w, common.Result{Ok: true, Msg: "nothing to change"})
		return
	}
	if strings.TrimSpace(newTitle) == "" {
		dynJson(w, common.Result{Msg: "a heading needs a title besides its rule"})
		return
	}
	lines[from] = setHeadlineTitle(lines[from], sec.Headline.Status, newTitle)
	if err := writeLines(f.Filename, lines); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	dynJson(w, common.Result{Ok: true, Msg: msg})
}
