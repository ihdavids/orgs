package orgs

// Time budgets (features.clockBudget), org-clock-budget's way: a heading with
// :BUDGET_WEEK: 5:00 (or _DAY, _MONTH, _QUARTER, _YEAR) has the time clocked
// in its subtree this week set against those five hours.
//
//   - A budget is the heading's own property, not inherited: a budget on a
//     project is the project's, and its tasks spend it.
//   - The value is H:MM, or a plain number of minutes (org-clock-budget's
//     two spellings), or 5h / 90m / 1h30m.
//   - The periods are org's: a week from Monday (clockBlockRange's thisweek),
//     calendar months, quarters from January, years. Every period gets the
//     same budget again; nothing carries over.
//   - Used time is clockMinsIn over the subtree, clipped to the period, plus
//     a clock still running: an open CLOCK line (Emacs) or orgs's own, which
//     writes nothing to the file until it stops.
//   - Pace is how far through the period now is; Expected = Budget * Pace.
//
// Clocking in under a heading that is over its budget says so in the answer's
// Msg, and a clock running across the line raises a notification (once per
// heading and period) unless clockBudget.notify is false.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/desktop"
	"github.com/ihdavids/orgs/internal/orgdate"
)

const clockBudgetFeature = "clockBudget"

func init() {
	dynGenerators["clockbudget"] = dynClockBudget
}

// budgetPeriodOf is a period name as typed: week, weekly, w, BUDGET_WEEK.
func budgetPeriodOf(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "budget_")
	switch s {
	case "d", "day", "daily", "today":
		return "day", true
	case "w", "week", "weekly", "thisweek":
		return "week", true
	case "m", "month", "monthly", "thismonth":
		return "month", true
	case "q", "quarter", "quarterly", "thisquarter":
		return "quarter", true
	case "y", "year", "yearly", "annual", "thisyear":
		return "year", true
	}
	return "", false
}

func budgetProperty(period string) string { return "BUDGET_" + strings.ToUpper(period) }

var (
	budgetHMRe   = regexp.MustCompile(`^(\d+):([0-5]\d)$`)
	budgetMinsRe = regexp.MustCompile(`^\d+(\.\d+)?$`)
	budgetSpanRe = regexp.MustCompile(`^(?:(\d+(?:\.\d+)?)\s*h)?\s*(?:(\d+)\s*m(?:in)?)?$`)
)

// parseBudget reads a budget: 5:00, 300 (minutes), 5h, 90m, 1h30m, 1.5h.
func parseBudget(s string) (float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, false
	}
	if m := budgetHMRe.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		mm, _ := strconv.Atoi(m[2])
		return float64(h*60 + mm), true
	}
	if budgetMinsRe.MatchString(s) {
		v, _ := strconv.ParseFloat(s, 64)
		return v, true
	}
	if m := budgetSpanRe.FindStringSubmatch(s); m != nil && (m[1] != "" || m[2] != "") {
		mins := 0.0
		if m[1] != "" {
			h, _ := strconv.ParseFloat(m[1], 64)
			mins += h * 60
		}
		if m[2] != "" {
			mm, _ := strconv.Atoi(m[2])
			mins += float64(mm)
		}
		return mins, true
	}
	return 0, false
}

// budgetRange is the period around ref.
func budgetRange(period string, ref time.Time) clockRange {
	spec := map[string]string{"day": "today", "week": "thisweek", "month": "thismonth", "year": "thisyear"}[period]
	if period == "quarter" {
		spec = fmt.Sprintf("%d-Q%d", ref.Year(), (int(ref.Month())-1)/3+1)
	}
	r, _ := clockBlockRange(spec, ref)
	return r
}

// budgetPace is how far through the range now is, 0 before it, 1 after.
func budgetPace(r clockRange, now time.Time) float64 {
	span := r.To.Sub(r.From)
	if span <= 0 || !now.After(r.From) {
		return 0
	}
	if !now.Before(r.To) {
		return 1
	}
	return float64(now.Sub(r.From)) / float64(span)
}

func budgetDailyHours() float64 {
	if s := Conf().Server; s != nil && s.ClockBudget.DailyHours > 0 {
		return s.ClockBudget.DailyHours
	}
	return 12
}

func budgetNotifyOn() bool {
	s := Conf().Server
	return s == nil || s.ClockBudget.Notify == nil || *s.ClockBudget.Notify
}

// budgetUsage is what a subtree has clocked in the range, and whether a clock
// is running in it.
type budgetUsage struct {
	cache       lineCache
	activeSec   *org.Section
	activeStart time.Time
	now         time.Time
}

func newBudgetUsage(now time.Time) *budgetUsage {
	u := &budgetUsage{cache: lineCache{}, now: now}
	u.activeSec, u.activeStart = recapActiveClock()
	return u
}

func (u *budgetUsage) used(f *common.OrgFile, sec *org.Section, r clockRange) (float64, bool) {
	total, running := 0.0, false
	var walk func(s *org.Section)
	walk = func(s *org.Section) {
		if s.Headline != nil {
			total += headlineClockMins(s.Headline, r, u.now)
			starts := recapOpenClocks(u.cache.of(f.Filename), s.Headline.Pos.Row)
			if u.activeSec != nil && s == u.activeSec {
				starts = append(starts, u.activeStart)
			}
			for _, st := range starts {
				total += recapRunningMins(st, r, u.now)
				running = true
			}
		}
		for _, c := range s.Children {
			walk(c)
		}
	}
	walk(sec)
	return total, running
}

// budgetItem is a heading's budget for one period, if it has one.
func (u *budgetUsage) item(f *common.OrgFile, sec *org.Section, period string, ref time.Time) (common.BudgetItem, bool) {
	v, ok := ownProp(sec, budgetProperty(period))
	if !ok {
		return common.BudgetItem{}, false
	}
	budget, ok := parseBudget(v)
	if !ok || budget <= 0 {
		return common.BudgetItem{}, false
	}
	r := budgetRange(period, ref)
	used, running := u.used(f, sec, r)
	olp := outlineOf(sec)
	if len(olp) > 0 {
		olp = olp[:len(olp)-1]
	}
	it := common.BudgetItem{
		Hash: sec.Hash, Headline: strings.TrimSpace(headlineTitle(sec)), Filename: f.Filename,
		Line: sec.Headline.Pos.Row, Olp: olp, Status: sec.Headline.Status, Tags: sec.Headline.Tags,
		Period: period, Property: budgetProperty(period), Value: strings.TrimSpace(v),
		Budget: budget, Used: used, Remaining: budget - used, Percent: used / budget * 100,
		Pace: budgetPace(r, u.now), Running: running,
		PeriodFrom: r.From.Format("2006-01-02"), PeriodTo: r.To.Format("2006-01-02"),
	}
	it.Expected = budget * it.Pace
	it.Over = used > budget
	it.Ahead = used > it.Expected
	return it, true
}

func hasAnyBudget(sec *org.Section) bool {
	for _, p := range common.BudgetPeriods {
		if v, ok := ownProp(sec, budgetProperty(p)); ok {
			if b, ok := parseBudget(v); ok && b > 0 {
				return true
			}
		}
	}
	return false
}

func budgetSkipped(f *common.OrgFile, sec *org.Section) bool {
	return isArchiveFile(f.Filename) || ednaHasTag(sec, "ARCHIVE")
}

// clockBudgets is every budget for the periods asked for (none: all) around
// ref, with each period's sums.
func clockBudgets(periods []string, ref, now time.Time) ([]common.BudgetItem, []common.BudgetPeriod) {
	if len(periods) == 0 {
		periods = common.BudgetPeriods
	}
	u := newBudgetUsage(now)
	items := []common.BudgetItem{}
	walkSections(func(f *common.OrgFile, sec *org.Section) {
		if budgetSkipped(f, sec) {
			return
		}
		for _, p := range periods {
			if it, ok := u.item(f, sec, p, ref); ok {
				items = append(items, it)
			}
		}
	})
	order := map[string]int{}
	for i, p := range common.BudgetPeriods {
		order[p] = i
	}
	sort.SliceStable(items, func(a, b int) bool { return order[items[a].Period] < order[items[b].Period] })
	sums := []common.BudgetPeriod{}
	for _, p := range periods {
		r := budgetRange(p, ref)
		days := float64(r.To.Sub(r.From).Hours()/24 + 0.5)
		s := common.BudgetPeriod{Name: p, Label: r.Label, From: r.From.Format("2006-01-02"), To: r.To.Format("2006-01-02"),
			Pace: budgetPace(r, now), Budgetable: float64(int(days)) * budgetDailyHours() * 60}
		for _, it := range items {
			if it.Period == p {
				s.Count++
				s.Budgeted += it.Budget
				s.Clocked += it.Used
			}
		}
		sums = append(sums, s)
	}
	return items, sums
}

// budgetsAbove are the budgets of a heading and every heading above it: what
// a clock on it is spending.
func budgetsAbove(f *common.OrgFile, sec *org.Section, now time.Time) []common.BudgetItem {
	u := newBudgetUsage(now)
	out := []common.BudgetItem{}
	for s := sec; s != nil && s.Headline != nil && s.Headline.Lvl > 0; s = s.Parent {
		for _, p := range common.BudgetPeriods {
			if it, ok := u.item(f, s, p, now); ok {
				out = append(out, it)
			}
		}
	}
	return out
}

// budgetPeriodWord is "this week", "today".
func budgetPeriodWord(p string) string {
	if p == "day" {
		return "today"
	}
	return "this " + p
}

func budgetOverText(it common.BudgetItem) string {
	return fmt.Sprintf("%s is over its budget: %s of %s %s", it.Headline, clockHM(it.Used), clockHM(it.Budget), budgetPeriodWord(it.Period))
}

// Which over-budget crossings have been told, so each is told once.
var (
	budgetToldLock sync.Mutex
	budgetTold     = map[string]bool{}
	budgetStop     chan struct{}
	budgetLock     sync.Mutex
)

func budgetKey(it common.BudgetItem) string {
	return it.Filename + "\x00" + olpString(append(append([]string{}, it.Olp...), it.Headline)) + "\x00" + it.Period + "\x00" + it.PeriodFrom
}

// clockBudgetWarning is what a clock-in answer adds when the heading (or one
// above it) is already over budget. Those count as told.
func clockBudgetWarning(f *common.OrgFile, sec *org.Section) string {
	if !featureOn(clockBudgetFeature) || f == nil || sec == nil {
		return ""
	}
	parts := []string{}
	budgetToldLock.Lock()
	defer budgetToldLock.Unlock()
	for _, it := range budgetsAbove(f, sec, time.Now()) {
		if it.Over {
			budgetTold[budgetKey(it)] = true
			parts = append(parts, budgetOverText(it))
		}
	}
	return strings.Join(parts, "; ")
}

// clockedBudgets is the running clock's heading and its budgets.
func clockedBudgets(now time.Time) common.BudgetClocked {
	res := common.BudgetClocked{Ok: true, Items: []common.BudgetItem{}}
	c := Clock()
	if !c.IsClockActive() || c.GetTarget() == nil {
		return res
	}
	t := *c.GetTarget()
	f, sec := GetDb().GetFromTarget(&t, false)
	if f == nil || sec == nil || sec.Headline == nil {
		return res
	}
	res.Active, res.Hash, res.Headline = true, sec.Hash, strings.TrimSpace(headlineTitle(sec))
	res.Items = budgetsAbove(f, sec, now)
	return res
}

// checkClockBudget tells of a budget the running clock has just taken over.
func checkClockBudget(now time.Time) {
	if !featureOn(clockBudgetFeature) {
		return
	}
	cl := clockedBudgets(now)
	for _, it := range cl.Items {
		if !it.Over {
			continue
		}
		budgetToldLock.Lock()
		told := budgetTold[budgetKey(it)]
		budgetTold[budgetKey(it)] = true
		budgetToldLock.Unlock()
		if told || !budgetNotifyOn() {
			continue
		}
		n := desktop.Note{Title: "Over budget", Subtitle: it.Headline, Sound: true, Group: "clockbudget",
			Body: fmt.Sprintf("%s of %s %s", clockHM(it.Used), clockHM(it.Budget), budgetPeriodWord(it.Period))}
		n.URL = common.HeadingLink(notifyWorgBase(), it.Filename, it.Line, it.Headline)
		if n.URL == "" {
			n.URL = common.WorgLink(notifyWorgBase(), url.Values{"app": {"Budgets"}})
		}
		deliverNote(n, it.Filename, it.Hash, it.Headline)
	}
}

// startClockBudget starts (or stops) the once-a-minute look at the running
// clock. Called when the routes are registered and when the feature flips.
func startClockBudget() {
	budgetLock.Lock()
	defer budgetLock.Unlock()
	if budgetStop != nil {
		close(budgetStop)
		budgetStop = nil
	}
	if !featureOn(clockBudgetFeature) {
		return
	}
	stop := make(chan struct{})
	budgetStop = stop
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				checkClockBudget(time.Now())
			}
		}
	}()
}

func registerClockBudgetRoutes(api *mux.Router) {
	api.HandleFunc("/budgets", RequestBudgets).Methods("GET")
	api.HandleFunc("/budgets/clocked", RequestBudgetsClocked).Methods("GET")
	api.HandleFunc("/budgets/set", PostBudgetSet).Methods("POST")
	startClockBudget()
}

// budgetPeriodsParam reads period=week, period=week,month, period=all.
func budgetPeriodsParam(v string) ([]string, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "all" {
		return nil, nil
	}
	out := []string{}
	for _, w := range strings.Split(v, ",") {
		p, ok := budgetPeriodOf(w)
		if !ok {
			return nil, fmt.Errorf("%q is not a period: day, week, month, quarter or year", w)
		}
		out = append(out, p)
	}
	return out, nil
}

/* SDOC: API
* GET /budgets — Time Budgets Against The Clock
* GET /budgets/clocked — The Running Clock's Budgets
* POST /budgets/set — Give A Heading A Budget

	#+BEGIN_EXAMPLE
	GET  /budgets?period=week&date=2026-10-15
	     {"Ok": true, "Date": "2026-10-15", "DailyHours": 12,
	      "Periods": [{Name, Label, From, To, Pace, Count, Budgeted, Clocked, Budgetable}],
	      "Items": [{Hash, Headline, Filename, Line, Olp, Period, Property, Value,
	                 Budget, Used, Remaining, Percent, Pace, Expected,
	                 Over, Ahead, Running, PeriodFrom, PeriodTo}]}
	GET  /budgets/clocked   {"Ok": true, "Active": true, "Hash": "...", "Headline": "...", "Items": [...]}
	POST /budgets/set       {"Hash": "...", "Period": "week", "Value": "5:00"}   ("" takes it off)
	#+END_EXAMPLE

	=period= is =day=, =week=, =month=, =quarter=, =year=, a list of them
	(=week,month=) or =all= (the default). =date= (a typed date: =2026-10-15=,
	=-1w=, =last mon=) picks the periods around another day; the default is
	today. Minutes throughout: =Budget=, =Used= (the subtree's clocked time in
	the period, a running clock counted to now), =Remaining= (negative when
	over), =Expected= (=Budget= times =Pace=, how far through the period now
	is). =Ahead= is spending faster than the period passes. Each period's sums
	are org-clock-budget's summary rows: =Budgetable= is =clockBudget.dailyHours=
	(12) times the days in it. =/budgets/clocked= is every budget the running
	clock is spending - its own heading's and each budgeted heading above it.
	=/budgets/set= writes =:BUDGET_WEEK: 5:00= (any of =5:00=, =300= minutes,
	=5h=, =1h30m=, written back as H:MM). Needs =features: clockBudget:=.
EDOC */
func RequestBudgets(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, clockBudgetFeature) {
		return
	}
	now := time.Now()
	ref := now
	if v := strings.TrimSpace(r.URL.Query().Get("date")); v != "" {
		d, ok, err := orgdate.ParseDate(v, now)
		if err != nil || !ok {
			dynJson(w, common.BudgetReport{Msg: fmt.Sprintf("%q is not a date", v)})
			return
		}
		ref = d.Day.Add(12 * time.Hour)
	}
	periods, err := budgetPeriodsParam(r.URL.Query().Get("period"))
	if err != nil {
		dynJson(w, common.BudgetReport{Msg: err.Error()})
		return
	}
	items, sums := clockBudgets(periods, ref, now)
	dynJson(w, common.BudgetReport{Ok: true, Date: ref.Format("2006-01-02"), DailyHours: budgetDailyHours(), Periods: sums, Items: items})
}

func RequestBudgetsClocked(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, clockBudgetFeature) {
		return
	}
	dynJson(w, clockedBudgets(time.Now()))
}

func PostBudgetSet(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, clockBudgetFeature) {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req common.BudgetSet
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	period, ok := budgetPeriodOf(req.Period)
	if !ok {
		dynJson(w, common.Result{Msg: fmt.Sprintf("%q is not a period: day, week, month, quarter or year", req.Period)})
		return
	}
	v := strings.TrimSpace(req.Value)
	if v != "" {
		mins, ok := parseBudget(v)
		if !ok || mins <= 0 {
			dynJson(w, common.Result{Msg: fmt.Sprintf("%q is not a budget: 5:00, 300 (minutes), 5h, 1h30m", v)})
			return
		}
		v = clockHM(mins)
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
	if err := setHeadingProperty(f, sec, budgetProperty(period), v); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	msg := budgetProperty(period) + " taken off"
	if v != "" {
		msg = fmt.Sprintf("%s %s", budgetProperty(period), v)
	}
	dynJson(w, common.Result{Ok: true, Msg: msg})
}

// Query functions: HasBudget() / HasBudget("week"), OverBudget() /
// OverBudget("week"). Both false while the feature is off.
func queryHasBudget(sec *org.Section, args []interface{}) (bool, error) {
	if !featureOn(clockBudgetFeature) || sec == nil || sec.Headline == nil {
		return false, nil
	}
	if len(args) == 0 {
		return hasAnyBudget(sec), nil
	}
	p, ok := budgetPeriodOf(fmt.Sprint(args[0]))
	if !ok {
		return false, fmt.Errorf("HasBudget: %q is not a period", fmt.Sprint(args[0]))
	}
	v, ok := ownProp(sec, budgetProperty(p))
	if !ok {
		return false, nil
	}
	b, ok := parseBudget(v)
	return ok && b > 0, nil
}

func queryOverBudget(f *common.OrgFile, sec *org.Section, args []interface{}) (bool, error) {
	if !featureOn(clockBudgetFeature) || f == nil || sec == nil || sec.Headline == nil || !hasAnyBudget(sec) {
		return false, nil
	}
	periods := common.BudgetPeriods
	if len(args) > 0 {
		p, ok := budgetPeriodOf(fmt.Sprint(args[0]))
		if !ok {
			return false, fmt.Errorf("OverBudget: %q is not a period", fmt.Sprint(args[0]))
		}
		periods = []string{p}
	}
	now := time.Now()
	u := newBudgetUsage(now)
	for _, p := range periods {
		if it, ok := u.item(f, sec, p, now); ok && it.Over {
			return true, nil
		}
	}
	return false, nil
}

// ----------------------------------------------------------------------------
// #+BEGIN: clockbudget - org-clock-budget's report as a table
// ----------------------------------------------------------------------------

// budgetPct is C/B as org-clock-budget shows it.
func budgetPct(used, budget float64) string {
	if budget <= 0 {
		return ""
	}
	return fmt.Sprintf("%.0f%%", used/budget*100)
}

func dynClockBudget(c *dynCtx) ([]string, error) {
	if !featureOn(clockBudgetFeature) {
		return nil, fmt.Errorf("time budgets (clockBudget) are turned off: `orgs features clockBudget on`")
	}
	periods, err := budgetPeriodsParam(c.param("period", "all"))
	if err != nil {
		return nil, err
	}
	ref := c.Now
	if v := strings.TrimSpace(c.param("date", "")); v != "" && v != "nil" {
		d, ok, err := orgdate.ParseDate(v, c.Now)
		if err != nil || !ok {
			return nil, fmt.Errorf(":date %q is not a date", v)
		}
		ref = d.Day.Add(12 * time.Hour)
	}
	items, sums := clockBudgets(periods, ref, c.Now)
	scope := c.param("scope", "agenda")
	switch scope {
	case "agenda", "all", "nil":
	case "file":
		kept := items[:0]
		for _, it := range items {
			if it.Filename == c.File.Filename {
				kept = append(kept, it)
			}
		}
		items = kept
	default:
		return nil, fmt.Errorf(":scope %q is not one a budget report knows (agenda, file)", scope)
	}
	// Only the periods somebody has a budget for, unless they were asked for.
	if periods == nil {
		kept := []common.BudgetPeriod{}
		for _, s := range sums {
			for _, it := range items {
				if it.Period == s.Name {
					kept = append(kept, s)
					break
				}
			}
		}
		sums = kept
	}
	if len(sums) == 0 {
		return []string{"No headings have a budget (:BUDGET_WEEK: 5:00)."}, nil
	}
	// A row a heading, in file order, a group of three columns a period.
	type rowKey struct {
		file string
		line int
	}
	rows := map[rowKey]map[string]common.BudgetItem{}
	keys := []rowKey{}
	titles := map[rowKey]string{}
	for _, it := range items {
		k := rowKey{it.Filename, it.Line}
		if rows[k] == nil {
			rows[k] = map[string]common.BudgetItem{}
			keys = append(keys, k)
			titles[k] = it.Headline
		}
		rows[k][it.Period] = it
	}
	sort.SliceStable(keys, func(a, b int) bool {
		if keys[a].file != keys[b].file {
			return keys[a].file < keys[b].file
		}
		return keys[a].line < keys[b].line
	})
	header := []string{"Task"}
	for _, s := range sums {
		name := strings.ToUpper(s.Name[:1]) + s.Name[1:]
		header = append(header, name+" budget", "Clocked", "C/B")
	}
	table := [][]string{header, nil}
	for _, k := range keys {
		row := []string{cellText(titles[k])}
		for _, s := range sums {
			if it, ok := rows[k][s.Name]; ok {
				row = append(row, clockHM(it.Budget), clockHM(it.Used), budgetPct(it.Used, it.Budget))
			} else {
				row = append(row, "", "", "")
			}
		}
		table = append(table, row)
	}
	table = append(table, nil)
	budgeted := []string{"Budgeted/Clocked"}
	budgetable := []string{"Budgetable hours"}
	for _, s := range sums {
		budgeted = append(budgeted, clockHM(s.Budgeted), clockHM(s.Clocked), budgetPct(s.Clocked, s.Budgeted))
		budgetable = append(budgetable, clockHM(s.Budgetable), clockHM(s.Budgetable-s.Budgeted), budgetPct(s.Budgeted, s.Budgetable))
	}
	table = append(table, budgeted, budgetable)
	labels := []string{}
	for _, s := range sums {
		labels = append(labels, s.Label)
	}
	caption := "#+CAPTION: Clock budgets at [" + c.Now.Format("2006-01-02 Mon 15:04") + "], for " + strings.Join(labels, ", ") + "."
	return append([]string{caption}, orgTableLines(table)...), nil
}
