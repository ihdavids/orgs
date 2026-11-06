package orgs

// org-review (features.review): headings that come back to be read again on
// a cadence. Not the weekly review wizard (review_wizard.go, GET /review/*),
// which is a different thing that happens to share a word; these routes are
// /reviews/*, the settings block is `orgReview:`, and the wizard only offers
// what is due here while the feature is on.
//
// org-review's properties, as it spells them:
//
//	:LAST_REVIEW:  [2026-10-04 Sun]   when it was last read (own only)
//	:NEXT_REVIEW:  [2026-11-04 Wed]   when it is next due (own only)
//	:REVIEW_DELAY: +1m                the cadence; inherited, then
//	                                  #+PROPERTY:, then orgReview.delay (+1m)
//
// Due (org-review-toreview-p): NEXT_REVIEW when it is set, else LAST_REVIEW +
// the delay. A heading with neither but a REVIEW_DELAY of its own has never
// been reviewed and is due now; one that only inherits a delay takes part
// only with orgReview.unreviewed (org-review-unreviewed: everything under the
// delay is flagged). Days are day numbers on the calendar, never instants
// (see the agenda's note in CLAUDE.md), so a review due today is due all day
// in every time zone.
//
// Marking reviewed (org-review-insert-last-review) writes LAST_REVIEW and,
// per orgReview.setsNext, NEXT_REVIEW = that day + the delay. Line edits, one
// write per request.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/orgdate"
)

const (
	reviewLastProp  = "LAST_REVIEW"
	reviewNextProp  = "NEXT_REVIEW"
	reviewDelayProp = "REVIEW_DELAY"
)

// ---------------------------------------------------------------------------
// The arithmetic: pure, in day numbers (agendaDayNum)
// ---------------------------------------------------------------------------

var (
	reviewDayRe  = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
	reviewSpanRe = regexp.MustCompile(`^\+?(\d+)\s*([dwmy]?)$`)
)

// reviewDayOf reads the day out of a property value: an org stamp, active or
// inactive, with or without a time, or a bare 2026-11-04.
func reviewDayOf(v string) (int, bool) {
	m := reviewDayRe.FindStringSubmatch(v)
	if m == nil {
		return 0, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return 0, false
	}
	return agendaDayYMD(y, time.Month(mo), d), true
}

// reviewSpanOk is whether s is a delay org-read-date takes as one: +3d, +2w,
// +1m, +1y (a bare number is days).
func reviewSpanOk(s string) bool {
	return reviewSpanRe.MatchString(strings.ToLower(strings.TrimSpace(s)))
}

// reviewAddSpan is day + span. Months and years are Emacs's encode-time
// normalising (Jan 31 +1m is Mar 3 in a common year), which is Go's AddDate.
func reviewAddSpan(day int, span string) (int, bool) {
	m := reviewSpanRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(span)))
	if m == nil {
		return day, false
	}
	n, _ := strconv.Atoi(m[1])
	t := agendaDayTime(day)
	switch m[2] {
	case "", "d":
		t = t.AddDate(0, 0, n)
	case "w":
		t = t.AddDate(0, 0, 7*n)
	case "m":
		t = t.AddDate(0, n, 0)
	case "y":
		t = t.AddDate(n, 0, 0)
	}
	return agendaDayNum(t), true
}

// reviewProps is what a heading says about its review.
type reviewProps struct {
	Last, Next string // own LAST_REVIEW, NEXT_REVIEW, as written
	Delay      string // the cadence in force
	DelayFrom  string // own, file, default, or the title it is inherited from
	OwnDelay   bool
	// A delay from a heading above or the file (not the default).
	InheritedDelay bool
}

// reviewDueAt is when a heading is due for review.
type reviewDueAt struct {
	Due   int  // day number
	Never bool // never reviewed: due from the day it is asked
	Last  int
	Next  int
	From  string // NEXT_REVIEW, LAST_REVIEW or never
}

// reviewDueAtOf is org-review-toreview-p without the "is it past" half: when
// the heading falls due, and whether it takes part in review at all. today is
// only used for a heading never reviewed.
func reviewDueAtOf(p reviewProps, unreviewed bool, today int) (reviewDueAt, bool) {
	w := reviewDueAt{Last: -1, Next: -1}
	if d, ok := reviewDayOf(p.Last); ok {
		w.Last = d
	}
	if d, ok := reviewDayOf(p.Next); ok {
		w.Next = d
	}
	switch {
	case w.Next >= 0:
		w.Due, w.From = w.Next, reviewNextProp
		return w, true
	case w.Last >= 0:
		if due, ok := reviewAddSpan(w.Last, p.Delay); ok {
			w.Due, w.From = due, reviewLastProp
			return w, true
		}
		// A delay that is not one: the default, rather than never due.
		due, _ := reviewAddSpan(w.Last, "+1m")
		w.Due, w.From = due, reviewLastProp
		return w, true
	case p.OwnDelay || (unreviewed && p.InheritedDelay):
		w.Due, w.Never, w.From = today, true, "never"
		return w, true
	}
	return w, false
}

// reviewNextAfterMark is the NEXT_REVIEW marking a heading reviewed on day
// writes, and whether to write it (org-review-sets-next-date).
func reviewNextAfterMark(day int, delay, setsNext string, oldNext string) (int, bool) {
	next, ok := reviewAddSpan(day, delay)
	if !ok {
		next, _ = reviewAddSpan(day, "+1m")
	}
	switch setsNext {
	case "never":
		return next, false
	case "only-future":
		if old, ok := reviewDayOf(oldNext); ok && next <= old {
			return next, false
		}
	}
	return next, true
}

// reviewWhy says when, in words.
func reviewWhy(w reviewDueAt, p reviewProps, today int) string {
	if w.Never {
		return "never reviewed (every " + strings.TrimPrefix(p.Delay, "+") + ")"
	}
	late := today - w.Due
	var when string
	switch {
	case late == 0:
		when = "due today"
	case late == 1:
		when = "due yesterday"
	case late > 1:
		when = fmt.Sprintf("due %d days ago", late)
	case late == -1:
		when = "due tomorrow"
	default:
		when = fmt.Sprintf("due in %d days", -late)
	}
	if w.From == reviewNextProp {
		if w.Last >= 0 {
			return fmt.Sprintf("%s (last reviewed %s)", when, reviewDayString(w.Last))
		}
		return when + " (NEXT_REVIEW)"
	}
	return fmt.Sprintf("%s (last reviewed %s, every %s)", when, reviewDayString(w.Last), strings.TrimPrefix(p.Delay, "+"))
}

func reviewDayString(day int) string { return agendaDayTime(day).Format("2006-01-02") }

// ---------------------------------------------------------------------------
// Reading a heading
// ---------------------------------------------------------------------------

func orgReviewSettings() common.OrgReviewSettings {
	if s := Conf().Server; s != nil {
		return s.OrgReview
	}
	return common.OrgReviewSettings{}
}

// reviewStampOf is a day as orgReview.stamp writes it.
func reviewStampOf(day int) string {
	s := agendaDayTime(day).Format("2006-01-02 Mon")
	if strings.EqualFold(strings.TrimSpace(orgReviewSettings().Stamp), "active") {
		return "<" + s + ">"
	}
	return "[" + s + "]"
}

func reviewPropsOf(f *common.OrgFile, sec *org.Section) reviewProps {
	p := reviewProps{}
	p.Last, _ = ownProp(sec, reviewLastProp)
	p.Next, _ = ownProp(sec, reviewNextProp)
	if v, ok := ownProp(sec, reviewDelayProp); ok && strings.TrimSpace(v) != "" {
		p.Delay, p.DelayFrom, p.OwnDelay = strings.TrimSpace(v), "own", true
		return p
	}
	for s := sec.Parent; isRealHeading(s); s = s.Parent {
		if v, ok := ownProp(s, reviewDelayProp); ok && strings.TrimSpace(v) != "" {
			p.Delay, p.DelayFrom, p.InheritedDelay = strings.TrimSpace(v), strings.TrimSpace(headlineTitle(s)), true
			return p
		}
	}
	if f != nil {
		if v, ok := fileProperty(f, reviewDelayProp); ok && strings.TrimSpace(v) != "" {
			p.Delay, p.DelayFrom, p.InheritedDelay = strings.TrimSpace(v), "file", true
			return p
		}
	}
	p.Delay, p.DelayFrom = orgReviewSettings().DelayOr(), "default"
	return p
}

// reviewSkipped: archived, commented and finished headings are not reviewed.
func reviewSkipped(f *common.OrgFile, sec *org.Section) bool {
	if f == nil || sec == nil || sec.Headline == nil || isArchiveFile(f.Filename) || agendaCommented(sec) {
		return true
	}
	for s := sec; isRealHeading(s); s = s.Parent {
		if ednaHasTag(s, "ARCHIVE") {
			return true
		}
	}
	if sec.Headline.Status != "" {
		done, _ := keywordSets(f)
		if done[sec.Headline.Status] {
			return true
		}
	}
	return false
}

// orgReviewOf is a heading's review, if it takes part.
func orgReviewOf(f *common.OrgFile, sec *org.Section, today int) (reviewDueAt, reviewProps, bool) {
	if reviewSkipped(f, sec) {
		return reviewDueAt{}, reviewProps{}, false
	}
	p := reviewPropsOf(f, sec)
	w, ok := reviewDueAtOf(p, orgReviewSettings().Unreviewed, today)
	return w, p, ok
}

func orgReviewItemOf(f *common.OrgFile, sec *org.Section, today int, cache lineCache) (common.OrgReviewItem, bool) {
	w, p, ok := orgReviewOf(f, sec, today)
	if !ok {
		return common.OrgReviewItem{}, false
	}
	ri := reviewItemOf(sec, f, cache)
	if ri == nil {
		return common.OrgReviewItem{}, false
	}
	it := common.OrgReviewItem{ReviewItem: *ri, Late: today - w.Due, IsDue: w.Due <= today,
		Delay: p.Delay, DelayFrom: p.DelayFrom, Why: reviewWhy(w, p, today)}
	if !w.Never {
		it.Due = reviewDayString(w.Due)
	}
	if w.Last >= 0 {
		it.Last = reviewDayString(w.Last)
	}
	if w.Next >= 0 {
		it.Next = reviewDayString(w.Next)
	}
	// The wizard's own fields, so a ReviewItem reader (the triage walker,
	// the weekly review) shows something sensible.
	it.Since, it.SinceFrom, it.Days = it.Due, it.Why, it.Late
	return it, true
}

// orgReviewItems is every heading due by today+within, most overdue first;
// all is every heading that takes part.
func orgReviewItems(now time.Time, within int, all bool) common.OrgReviewList {
	today := agendaDayNum(now)
	res := common.OrgReviewList{Ok: true, Today: reviewDayString(today), Items: []common.OrgReviewItem{}}
	cache := lineCache{}
	walkSections(func(f *common.OrgFile, sec *org.Section) {
		it, ok := orgReviewItemOf(f, sec, today, cache)
		if !ok || (!all && it.Late < -within) {
			return
		}
		if it.IsDue {
			res.Due++
		}
		res.Items = append(res.Items, it)
	})
	sort.SliceStable(res.Items, func(i, j int) bool {
		a, b := res.Items[i], res.Items[j]
		// Never reviewed first, as org-review-compare has it (its date is 0).
		if (a.Due == "") != (b.Due == "") {
			return a.Due == ""
		}
		if a.Late != b.Late {
			return a.Late > b.Late
		}
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		return a.LineNum < b.LineNum
	})
	return res
}

// reviewDue and hasReview are the query functions ReviewDue() and HasReview().
func reviewDue(f *common.OrgFile, sec *org.Section) bool {
	if !featureOn("review") {
		return false
	}
	today := agendaDayNum(time.Now())
	w, _, ok := orgReviewOf(f, sec, today)
	return ok && w.Due <= today
}

func hasReview(f *common.OrgFile, sec *org.Section) bool {
	if !featureOn("review") {
		return false
	}
	_, _, ok := orgReviewOf(f, sec, agendaDayNum(time.Now()))
	return ok
}

// ---------------------------------------------------------------------------
// Writing
// ---------------------------------------------------------------------------

// reviewDayFrom reads a day somebody typed: "" is def, +2w is from today,
// else a typed date (fri, tomorrow, 2026-12-01) or an org stamp.
func reviewDayFrom(v string, def int, now time.Time) (int, error) {
	v = strings.TrimSpace(v)
	today := agendaDayNum(now)
	if v == "" {
		return def, nil
	}
	if strings.HasPrefix(v, "+") && reviewSpanOk(v) {
		d, _ := reviewAddSpan(today, v)
		return d, nil
	}
	if d, ok := reviewDayOf(v); ok && (strings.ContainsAny(v, "<[") || reviewDayRe.MatchString(v) && len(strings.TrimSpace(v)) == 10) {
		return d, nil
	}
	od, set, err := orgdate.ParseDate(v, now)
	if err != nil {
		return 0, err
	}
	if !set {
		return 0, fmt.Errorf("no date in %q", v)
	}
	if od.Raw != "" {
		if d, ok := reviewDayOf(od.Raw); ok {
			return d, nil
		}
		return 0, fmt.Errorf("no date in %q", v)
	}
	return agendaDayNum(od.Day), nil
}

// reviewProp is one property to write; "" takes it off.
type reviewProp struct{ key, val string }

// reviewWrite sets (or takes off) properties of one heading and adds an
// optional log entry, as one write. The range is measured again after each
// change, because each can add or remove lines (Traps: re-measure).
func reviewWrite(f *common.OrgFile, sec *org.Section, props []reviewProp, logNote []string) error {
	lines, from, to, ok := recordLines(f.Filename, sec)
	if !ok {
		return fmt.Errorf("could not read %s", f.Filename)
	}
	lvl := sec.Headline.Lvl
	ind := indentOf(lvl)
	// A file whose drawers sit in column zero (Emacs since org 9.5) gets
	// new ones there too, not indented by heading depth.
	for _, l := range lines {
		if l == ":PROPERTIES:" || l == ":LOGBOOK:" {
			ind = ""
			break
		}
	}
	for _, p := range props {
		if strings.TrimSpace(p.val) == "" {
			ps, pe, found := drawerAt(lines, from, to, "PROPERTIES")
			if !found {
				continue
			}
			kept := 0
			for i := ps + 1; i < pe && i < len(lines); i++ {
				if m := propLineRe.FindStringSubmatch(lines[i]); m != nil && strings.EqualFold(m[2], p.key) {
					lines = append(lines[:i], lines[i+1:]...)
					pe--
					i--
					continue
				}
				kept++
			}
			if kept == 0 {
				lines = append(lines[:ps], lines[pe+1:]...)
			} else {
				alignDrawer(lines, ps, pe)
			}
		} else {
			_, _, had := drawerAt(lines, from, to, "PROPERTIES")
			var ps, pe int
			lines, ps, pe, _ = ensurePropertyDrawer(lines, from, to, ind)
			lines, pe = setPropIn(lines, ps, pe, ind, p.key, p.val)
			if !had && ind == "" {
				// setPropIn puts a new drawer's first property two in; in a
				// column-zero file it goes where the file's others are.
				for i := ps + 1; i < pe; i++ {
					lines[i] = strings.TrimLeft(lines[i], " \t")
				}
				alignDrawer(lines, ps, pe)
			}
		}
		lines, from, to = reread(lines, from, lvl)
	}
	if len(logNote) > 0 {
		set := applyHeadingLogProps(fileLogSettings(f), propsFromLines(lines, from, to))
		entry := []string{}
		for _, l := range logNote {
			entry = append(entry, ind+l)
		}
		lines = logInto(lines, from, to, ind, set.drawer, entry)
	}
	return writeLines(f.Filename, lines)
}

// reviewMark is org-review-insert-last-review: LAST_REVIEW is day, and
// NEXT_REVIEW day + the delay (orgReview.setsNext). A note goes into the log
// drawer as org's "Note taken on" entry.
func reviewMark(f *common.OrgFile, sec *org.Section, day int, note string, now time.Time) (string, error) {
	p := reviewPropsOf(f, sec)
	props := []reviewProp{{reviewLastProp, reviewStampOf(day)}}
	next, write := reviewNextAfterMark(day, p.Delay, orgReviewSettings().SetsNextOr(), p.Next)
	if write {
		props = append(props, reviewProp{reviewNextProp, reviewStampOf(next)})
	}
	var log []string
	if n := strings.TrimSpace(note); n != "" {
		log = []string{"- Note taken on " + orgStamp(now) + " \\\\", "  Reviewed: " + n}
	}
	if err := reviewWrite(f, sec, props, log); err != nil {
		return "", err
	}
	if write {
		return fmt.Sprintf("reviewed %q, next %s", headlineTitle(sec), reviewDayString(next)), nil
	}
	return fmt.Sprintf("reviewed %q", headlineTitle(sec)), nil
}

// reviewSnooze moves NEXT_REVIEW and nothing else.
func reviewSnooze(f *common.OrgFile, sec *org.Section, until string, now time.Time) (string, error) {
	if strings.TrimSpace(until) == "" {
		until = "+1w"
	}
	day, err := reviewDayFrom(until, 0, now)
	if err != nil {
		return "", err
	}
	if err := reviewWrite(f, sec, []reviewProp{{reviewNextProp, reviewStampOf(day)}}, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf("%q comes back for review on %s", headlineTitle(sec), reviewDayString(day)), nil
}

// reviewSetCadence is POST /reviews/set: a new REVIEW_DELAY moves NEXT_REVIEW
// to LAST_REVIEW + it (or today + it, for a heading not yet in review), so
// "every two weeks" means it at once.
func reviewSetCadence(f *common.OrgFile, sec *org.Section, req common.OrgReviewSet, now time.Time) (string, error) {
	if req.Clear {
		err := reviewWrite(f, sec, []reviewProp{{reviewLastProp, ""}, {reviewNextProp, ""}, {reviewDelayProp, ""}}, nil)
		return fmt.Sprintf("%q is no longer reviewed", headlineTitle(sec)), err
	}
	today := agendaDayNum(now)
	p := reviewPropsOf(f, sec)
	props := []reviewProp{}
	said := []string{}
	if req.Delay != nil {
		d := strings.TrimSpace(*req.Delay)
		if d != "" {
			if !reviewSpanOk(d) {
				return "", fmt.Errorf("%q is not a delay: +3d, +2w, +1m, +1y", d)
			}
			if !strings.HasPrefix(d, "+") {
				d = "+" + d
			}
			props = append(props, reviewProp{reviewDelayProp, d})
			said = append(said, "every "+strings.TrimPrefix(d, "+"))
			if req.Next == nil && orgReviewSettings().SetsNextOr() != "never" {
				from := today
				if last, ok := reviewDayOf(p.Last); ok {
					from = last
				}
				next, _ := reviewAddSpan(from, d)
				props = append(props, reviewProp{reviewNextProp, reviewStampOf(next)})
				said = append(said, "next "+reviewDayString(next))
			}
		} else {
			props = append(props, reviewProp{reviewDelayProp, ""})
			said = append(said, "own cadence taken off")
		}
	}
	if req.Next != nil {
		if strings.TrimSpace(*req.Next) == "" {
			props = append(props, reviewProp{reviewNextProp, ""})
			said = append(said, "NEXT_REVIEW taken off")
		} else {
			day, err := reviewDayFrom(*req.Next, today, now)
			if err != nil {
				return "", err
			}
			props = append(props, reviewProp{reviewNextProp, reviewStampOf(day)})
			said = append(said, "next "+reviewDayString(day))
		}
	}
	if len(props) == 0 {
		return "", fmt.Errorf("nothing to set: give Delay, Next or Clear")
	}
	if err := reviewWrite(f, sec, props, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf("%q: %s", headlineTitle(sec), strings.Join(said, ", ")), nil
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

func registerOrgReviewRoutes(api *mux.Router) {
	api.HandleFunc("/reviews", RequestOrgReviews).Methods("GET")
	api.HandleFunc("/reviews/mark", PostOrgReviewMark).Methods("POST")
	api.HandleFunc("/reviews/set", PostOrgReviewSet).Methods("POST")
	api.HandleFunc("/reviews/snooze", PostOrgReviewSnooze).Methods("POST")
}

/* SDOC: API
* GET /reviews — Headings Due For Review (org-review)
* POST /reviews/mark — Mark A Heading Reviewed
* POST /reviews/set — Set A Heading's Review Cadence
* POST /reviews/snooze — Put A Review Off

	#+BEGIN_EXAMPLE
	GET  /reviews?within=7        {"Ok": true, "Today": "2026-11-04", "Due": 3,
	                               "Items": [{Hash, Headline, Filename, LineNum, Olp, Body, ...,
	                                          Due, Late, IsDue, Last, Next, Delay, DelayFrom, Why}]}
	POST /reviews/mark            {"Hash": "...", "Date": "", "Note": "still on track"}
	POST /reviews/set             {"Hash": "...", "Delay": "+2w"}   ("" takes it off)
	                              {"Hash": "...", "Next": "fri"}    {"Hash": "...", "Clear": true}
	POST /reviews/snooze          {"Hash": "...", "Until": "+1w"}
	#+END_EXAMPLE

	A heading takes part when it has =:LAST_REVIEW:=, =:NEXT_REVIEW:= or a
	=:REVIEW_DELAY:= of its own. It is due on its =NEXT_REVIEW=, else on
	=LAST_REVIEW= plus its delay (=REVIEW_DELAY= inherited from the headings
	above or =#+PROPERTY:=, else =orgReview.delay=, +1m). One with only a
	delay has never been reviewed and is due now. Archived, commented and
	finished headings are left out.

	=GET /reviews= answers what is due today or before, most overdue first;
	=within=N= adds what falls due in the next N days (=Late= is negative
	for those), =all=t= every heading in review. =Late= is days past =Due=;
	=Due= is empty for one never reviewed.

	=/reviews/mark= writes =LAST_REVIEW= (=Date=, default today, a typed date
	like =yesterday=) and =NEXT_REVIEW= = that day + the delay, as
	=orgReview.setsNext= says; a =Note= goes into the log drawer.
	=/reviews/set= with a =Delay= also moves =NEXT_REVIEW= to =LAST_REVIEW=
	(or today) + the delay. =/reviews/snooze= moves only =NEXT_REVIEW=
	(=+1w= by default, from today). Each answers ={Ok, Msg, Hash, Item}= with
	the heading as it now stands. All need =features: review:=.
EDOC */
func RequestOrgReviews(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "review") {
		return
	}
	q := r.URL.Query()
	within := 0
	if s := q.Get("within"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			within = n
		}
	}
	all := q.Get("all") == "t" || q.Get("all") == "true"
	dynJson(w, orgReviewItems(time.Now(), within, all))
}

// reviewTarget finds the heading a write names.
func reviewTarget(w http.ResponseWriter, r *http.Request, into interface{}, hash func() string) (*common.OrgFile, *org.Section, bool) {
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, into); err != nil {
		dynJson(w, common.OrgReviewResult{Msg: err.Error()})
		return nil, nil, false
	}
	sec := GetDb().FindByHash(hash())
	if sec == nil || sec.Headline == nil {
		dynJson(w, common.OrgReviewResult{Msg: "no heading with that hash - it may have moved; refresh and try again"})
		return nil, nil, false
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		dynJson(w, common.OrgReviewResult{Msg: "could not find the file that heading is in"})
		return nil, nil, false
	}
	return f, sec, true
}

// reviewAnswer is a write's answer, with the heading read again.
func reviewAnswer(w http.ResponseWriter, ref ednaRef, msg string, err error) {
	if err != nil {
		dynJson(w, common.OrgReviewResult{Msg: err.Error()})
		return
	}
	res := common.OrgReviewResult{Ok: true, Msg: msg}
	if f, sec := ref.find(); f != nil && sec != nil {
		res.Hash = sec.Hash
		if it, ok := orgReviewItemOf(f, sec, agendaDayNum(time.Now()), lineCache{}); ok {
			res.Item = &it
		}
	}
	Publish(Event{Kind: "reviews", Msg: msg})
	dynJson(w, res)
}

func PostOrgReviewMark(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "review") {
		return
	}
	var req common.OrgReviewMark
	f, sec, ok := reviewTarget(w, r, &req, func() string { return req.Hash })
	if !ok {
		return
	}
	now := time.Now()
	day, err := reviewDayFrom(req.Date, agendaDayNum(now), now)
	if err != nil {
		dynJson(w, common.OrgReviewResult{Msg: err.Error()})
		return
	}
	ref := refOf(f, sec)
	msg, err := reviewMark(f, sec, day, req.Note, now)
	reviewAnswer(w, ref, msg, err)
}

func PostOrgReviewSet(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "review") {
		return
	}
	var req common.OrgReviewSet
	f, sec, ok := reviewTarget(w, r, &req, func() string { return req.Hash })
	if !ok {
		return
	}
	ref := refOf(f, sec)
	msg, err := reviewSetCadence(f, sec, req, time.Now())
	reviewAnswer(w, ref, msg, err)
}

func PostOrgReviewSnooze(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "review") {
		return
	}
	var req common.OrgReviewSnooze
	f, sec, ok := reviewTarget(w, r, &req, func() string { return req.Hash })
	if !ok {
		return
	}
	ref := refOf(f, sec)
	msg, err := reviewSnooze(f, sec, req.Until, time.Now())
	reviewAnswer(w, ref, msg, err)
}

// orgReviewAct is the weekly review's (POST /review/act) review actions:
// reviewed, review-snooze (When: +1w, a date) and review-every (Text: +2w).
func orgReviewAct(req *common.ReviewAct, f *common.OrgFile, sec *org.Section, now time.Time) (string, error) {
	switch strings.ToLower(req.Action) {
	case "reviewed":
		return reviewMark(f, sec, agendaDayNum(now), req.Text, now)
	case "review-snooze":
		return reviewSnooze(f, sec, req.When, now)
	}
	d := req.Text
	return reviewSetCadence(f, sec, common.OrgReviewSet{Delay: &d}, now)
}
