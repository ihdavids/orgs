package orgs

// Two ways a heading leaves by itself, each its own optional feature:
//
//   - org-expiry (features.expiry): a heading whose :EXPIRY: has passed. The
//     value is a date (<2026-12-01>) or a span after the heading's :CREATED:
//     (+2w), org-expiry's two spellings; expiry.wait gives every heading with
//     a CREATED and no EXPIRY one, which org-expiry does by default and orgs
//     only when asked. What happens is expiry.handler: archive (the default),
//     sibling (into the Archive sibling), tag (org-expiry's own default, the
//     ARCHIVE tag) or keyword (expiry.keyword, EXPIRED).
//   - auto archive (features.autoArchive): a heading in a done keyword closed
//     longer ago than autoArchive.after, filed by autoArchive.mode. A heading
//     or file can say :AUTO_ARCHIVE: nil to keep its own, or a span of its own.
//
// Both are looked at every expiry.every while either is on, and on demand
// (POST /expiry/run, `orgs expire`). Archives go to Move as **one batch**,
// because each one changes the hashes of the headings after it (see Traps:
// hashes): Move resolves them all to outline paths first.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// ExpiryItem is one heading due to go.
type ExpiryItem struct {
	Hash     string
	Headline string
	Filename string
	Line     int
	Feature  string // expiry or autoArchive
	Action   string // archive, sibling, tag or keyword
	Why      string
	Since    string // the date it became due, YYYY-MM-DD
}

var expirySpanRe = regexp.MustCompile(`^\+?(\d+)\s*([hdwmy])$`)

// parseSpan reads 30d, 2w, +1m, 12h.
func parseSpan(s string) (func(time.Time) time.Time, bool) {
	m := expirySpanRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(s)))
	if m == nil {
		return nil, false
	}
	n, _ := strconv.Atoi(m[1])
	return func(t time.Time) time.Time {
		switch m[2] {
		case "h":
			return t.Add(time.Duration(n) * time.Hour)
		case "d":
			return t.AddDate(0, 0, n)
		case "w":
			return t.AddDate(0, 0, 7*n)
		case "m":
			return t.AddDate(0, n, 0)
		}
		return t.AddDate(n, 0, 0)
	}, true
}

// stampIn is the first org timestamp in s.
func stampIn(s string) (time.Time, bool) {
	loc := anyStampRe.FindString(s)
	if loc == "" {
		return time.Time{}, false
	}
	d, _, _ := org.ParseTimestamp(loc)
	if d == nil {
		return time.Time{}, false
	}
	return d.Start, true
}

func expiryHandler() string {
	h := "archive"
	if s := Conf().Server; s != nil && s.Expiry.Handler != "" {
		h = strings.ToLower(strings.TrimSpace(s.Expiry.Handler))
	}
	return h
}

func expiryKeyword() string {
	if s := Conf().Server; s != nil && strings.TrimSpace(s.Expiry.Keyword) != "" {
		return strings.TrimSpace(s.Expiry.Keyword)
	}
	return "EXPIRED"
}

func autoArchiveMode() string {
	m := "archive"
	if s := Conf().Server; s != nil && s.AutoArchive.Mode != "" {
		m = strings.ToLower(strings.TrimSpace(s.AutoArchive.Mode))
	}
	return m
}

// expiresAt is when a heading expires, if it does.
func expiresAt(f *common.OrgFile, sec *org.Section) (time.Time, string, bool) {
	created, hasCreated := time.Time{}, false
	if v, ok := ownProp(sec, "CREATED"); ok {
		created, hasCreated = stampIn(v)
	}
	v, ok := ownProp(sec, "EXPIRY")
	if !ok || strings.TrimSpace(v) == "" {
		if s := Conf().Server; s != nil && s.Expiry.Wait != "" {
			v, ok = s.Expiry.Wait, true
		}
	}
	if !ok {
		return time.Time{}, "", false
	}
	v = strings.TrimSpace(v)
	if t, ok := stampIn(v); ok {
		return t, "its EXPIRY " + v + " has passed", true
	}
	if add, ok := parseSpan(v); ok && hasCreated {
		return add(created), fmt.Sprintf("created %s, expires %s after", created.Format("2006-01-02"), strings.TrimPrefix(v, "+")), true
	}
	return time.Time{}, "", false
}

// closedAt is when a done heading was finished: its CLOSED stamp, else the
// newest "State DONE" line in its log.
func closedAt(f *common.OrgFile, sec *org.Section) (time.Time, bool) {
	if c := sec.Headline.Closed; c != nil && c.Date != nil {
		return c.Date.Start, true
	}
	lines, from, end, ok := recordLines(f.Filename, sec)
	if !ok {
		return time.Time{}, false
	}
	own := ownLinesEnd(lines, from, end)
	var newest time.Time
	for i := from + 1; i <= own; i++ {
		l := lines[i]
		if strings.Contains(l, `- State "`+sec.Headline.Status+`"`) {
			if t, ok := stampIn(l); ok && t.After(newest) {
				newest = t
			}
		}
	}
	return newest, !newest.IsZero()
}

// autoArchiveAfter is the span a heading waits after closing: its own or an
// ancestor's :AUTO_ARCHIVE:, the file's, then the yaml's; "" keeps it.
func autoArchiveAfter(f *common.OrgFile, sec *org.Section) string {
	for s := sec; isRealHeading(s); s = s.Parent {
		if v, ok := ownProp(s, "AUTO_ARCHIVE"); ok {
			return autoArchiveSpan(v)
		}
	}
	if v, ok := fileProperty(f, "AUTO_ARCHIVE"); ok {
		return autoArchiveSpan(v)
	}
	after := "30d"
	if s := Conf().Server; s != nil && s.AutoArchive.After != "" {
		after = s.AutoArchive.After
	}
	return after
}

func autoArchiveSpan(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || v == "nil" || v == "no" || v == "never" {
		return ""
	}
	if v == "t" || v == "yes" {
		after := "30d"
		if s := Conf().Server; s != nil && s.AutoArchive.After != "" {
			after = s.AutoArchive.After
		}
		return after
	}
	return v
}

func isArchiveFile(name string) bool {
	return strings.HasSuffix(name, "_archive") || strings.Contains(name, "_archive.")
}

// expiryDue is every heading either feature would take now.
func expiryDue(now time.Time) []ExpiryItem {
	out := []ExpiryItem{}
	doExpiry, doArchive := featureOn("expiry"), featureOn("autoArchive")
	if !doExpiry && !doArchive {
		return out
	}
	var only map[string]bool
	if s := Conf().Server; doArchive && s != nil && strings.TrimSpace(s.AutoArchive.Query) != "" {
		only = map[string]bool{}
		if todos, err := QueryStringTodos(&common.StringQuery{Query: s.AutoArchive.Query}); err == nil && todos != nil {
			for _, t := range *todos {
				only[string(t.Hash)] = true
			}
		}
	}
	handler, kw := expiryHandler(), expiryKeyword()
	walkSections(func(f *common.OrgFile, sec *org.Section) {
		if isArchiveFile(f.Filename) || ednaHasTag(sec, "ARCHIVE") {
			return
		}
		item := ExpiryItem{Hash: sec.Hash, Headline: strings.TrimSpace(headlineTitle(sec)), Filename: f.Filename, Line: sec.Headline.Pos.Row}
		if doExpiry && !(handler == "keyword" && sec.Headline.Status == kw) {
			if at, why, ok := expiresAt(f, sec); ok && !now.Before(at) {
				item.Feature, item.Action, item.Why, item.Since = "expiry", handler, why, at.Format("2006-01-02")
				out = append(out, item)
				return
			}
		}
		if !doArchive {
			return
		}
		done, _ := keywordSets(f)
		if !done[sec.Headline.Status] || (only != nil && !only[sec.Hash]) {
			return
		}
		after := autoArchiveAfter(f, sec)
		add, ok := parseSpan(after)
		if !ok {
			return
		}
		closed, ok := closedAt(f, sec)
		if !ok || now.Before(add(closed)) {
			return
		}
		item.Feature, item.Action = "autoArchive", autoArchiveMode()
		item.Why = fmt.Sprintf("%s since %s, over %s ago", sec.Headline.Status, closed.Format("2006-01-02"), after)
		item.Since = add(closed).Format("2006-01-02")
		out = append(out, item)
	})
	return out
}

// ExpiryRunResult is what a run did.
type ExpiryRunResult struct {
	Ok      bool
	Msg     string
	Done    int
	Failed  int
	Results []common.MoveResult
}

// runExpiry does what is due. hashes narrows it to those headings.
func runExpiry(now time.Time, hashes []string) ExpiryRunResult {
	expiryMu.Lock()
	defer expiryMu.Unlock()
	res := ExpiryRunResult{Results: []common.MoveResult{}}
	want := map[string]bool{}
	for _, h := range hashes {
		want[h] = true
	}
	due := expiryDue(now)
	moves := map[string][]common.Target{}
	type ref struct {
		r      ednaRef
		action string
	}
	edits := []ref{}
	for _, it := range due {
		if len(want) > 0 && !want[it.Hash] {
			continue
		}
		sec := GetDb().FindByHash(it.Hash)
		f := GetDb().FileFromSection(sec)
		if sec == nil || f == nil {
			continue
		}
		r := refOf(f, sec)
		switch it.Action {
		case "archive", "sibling":
			moves[it.Action] = append(moves[it.Action], common.Target{Type: "file+olp", Filename: r.file, Id: olpString(r.olp)})
		default:
			edits = append(edits, ref{r, it.Action})
		}
	}
	// Tags and keywords first: they keep the heading where it is, so the
	// archive batch after them still finds what it resolved.
	for _, e := range edits {
		f, sec := e.r.find()
		mr := common.MoveResult{Headline: e.r.headline, Filename: e.r.file}
		var err error
		switch {
		case f == nil:
			err = fmt.Errorf("could not find it again")
		case e.action == "tag":
			tags := append(append([]string{}, sec.Headline.Tags...), "ARCHIVE")
			run := &ednaRun{now: now}
			err = run.headline(f, sec, nil, &tags)
		case e.action == "keyword":
			lines, from, _, ok := recordLines(f.Filename, sec)
			if !ok {
				err = fmt.Errorf("could not read %s", f.Filename)
				break
			}
			head, ok := replaceHeadlineStatus(lines[from], sec.Headline.Status, expiryKeyword())
			if !ok {
				err = fmt.Errorf("not a heading")
				break
			}
			lines[from] = head
			err = writeLines(f.Filename, lines)
		default:
			err = fmt.Errorf("no handler called %q (archive, sibling, tag or keyword)", e.action)
		}
		mr.Ok = err == nil
		if err != nil {
			mr.Msg = err.Error()
			res.Failed++
		} else {
			res.Done++
		}
		res.Results = append(res.Results, mr)
	}
	for _, op := range []string{"archive", "sibling"} {
		if len(moves[op]) == 0 {
			continue
		}
		mop := common.MoveArchive
		if op == "sibling" {
			mop = common.MoveArchiveSibling
		}
		mres, _ := Move(db, &common.MoveRequest{Op: mop, From: moves[op]})
		res.Done += mres.Done
		res.Failed += mres.Failed
		res.Results = append(res.Results, mres.Results...)
	}
	res.Ok = res.Failed == 0
	switch {
	case res.Done == 0 && res.Failed == 0:
		res.Msg = "nothing is due"
	case res.Failed == 0:
		res.Msg = fmt.Sprintf("%d heading%s filed away", res.Done, plural(res.Done))
	default:
		res.Msg = fmt.Sprintf("%d filed away, %d could not be", res.Done, res.Failed)
	}
	return res
}

var (
	expiryMu   sync.Mutex
	expiryStop chan struct{}
	expiryLock sync.Mutex
)

func expiryEvery() time.Duration {
	if s := Conf().Server; s != nil && s.Expiry.Every != "" {
		if d, err := time.ParseDuration(s.Expiry.Every); err == nil && d >= time.Minute {
			return d
		}
		if add, ok := parseSpan(s.Expiry.Every); ok {
			now := time.Now()
			if d := add(now).Sub(now); d >= time.Minute {
				return d
			}
		}
	}
	return time.Hour
}

// startExpiry starts the background look (or stops it, when both features
// are off). Called at startup and whenever either is turned on or off.
func startExpiry() {
	expiryLock.Lock()
	defer expiryLock.Unlock()
	if expiryStop != nil {
		close(expiryStop)
		expiryStop = nil
	}
	if !featureOn("expiry") && !featureOn("autoArchive") {
		return
	}
	stop := make(chan struct{})
	expiryStop = stop
	every := expiryEvery()
	go func() {
		// A minute in, not at once: the files are still being read at startup.
		first := time.NewTimer(time.Minute)
		defer first.Stop()
		tick := time.NewTicker(every)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-first.C:
			case <-tick.C:
			}
			res := runExpiry(time.Now(), nil)
			if res.Done > 0 || res.Failed > 0 {
				fmt.Fprintf(os.Stderr, "EXPIRY: %s\n", res.Msg)
				Publish(Event{Kind: "expiry", Msg: res.Msg})
			}
		}
	}()
}

/* SDOC: API
* GET /expiry/due — What Expiry And Auto Archive Would Take
* POST /expiry/run — Take It Now
* POST /expiry/set — Give A Heading An Expiry

	#+BEGIN_EXAMPLE
	GET  /expiry/due        {"Ok": true, "Expiry": true, "AutoArchive": false,
	                         "Items": [{Hash, Headline, Filename, Line, Feature, Action, Why, Since}]}
	POST /expiry/run        {"Hashes": [...]}   (none: everything due)
	POST /expiry/set        {"Hash": "...", "Value": "+2w"}   ("" takes it off)
	#+END_EXAMPLE

	=Feature= is =expiry= or =autoArchive=, =Action= what will happen:
	=archive=, =sibling=, =tag= or =keyword=. The run answers like =/move=,
	with a result per heading. =/expiry/set= takes a span after =CREATED=
	(=+2w=), a typed date (=fri=, =2026-12-01=) or an org timestamp, and
	writes =:CREATED:= too when a span needs one. Needs =features: expiry:=
	or =features: autoArchive:= (=/expiry/set= the first).
EDOC */
func RequestExpiryDue(w http.ResponseWriter, r *http.Request) {
	if !featureOn("expiry") && !featureOn("autoArchive") {
		featureOff(w, "expiry")
		return
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Expiry": featureOn("expiry"), "AutoArchive": featureOn("autoArchive"),
		"Items": expiryDue(time.Now())})
}

func PostExpiryRun(w http.ResponseWriter, r *http.Request) {
	if !featureOn("expiry") && !featureOn("autoArchive") {
		featureOff(w, "expiry")
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct{ Hashes []string }
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
	}
	dynJson(w, runExpiry(time.Now(), req.Hashes))
}

func PostExpirySet(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "expiry") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct{ Hash, Value string }
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
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
	v := strings.TrimSpace(req.Value)
	ref := refOf(f, sec)
	switch {
	case v == "":
	case strings.HasPrefix(v, "+"):
		if _, ok := parseSpan(v); !ok {
			dynJson(w, common.Result{Msg: fmt.Sprintf("%q is not a span: +3d, +2w, +1m", v)})
			return
		}
		// A span counts from CREATED, so a heading without one gets one.
		if _, ok := ownProp(sec, "CREATED"); !ok {
			if err := setHeadingProperty(f, sec, "CREATED", orgStamp(time.Now())); err != nil {
				dynJson(w, common.Result{Msg: err.Error()})
				return
			}
			if f, sec = ref.find(); f == nil {
				dynJson(w, common.Result{Msg: "the heading moved while it was being written"})
				return
			}
		}
	default:
		if _, ok := stampIn(v); !ok {
			stamp, err := dateChangeStamp("SCHEDULED", v)
			if err != nil {
				run := &ednaRun{now: time.Now()}
				if stamp, err = run.ednaStamp(ref, sec, "SCHEDULED", v); err != nil || stamp == "" {
					dynJson(w, common.Result{Msg: fmt.Sprintf("%q is not a date", v)})
					return
				}
			}
			v = stamp
		}
	}
	if err := setHeadingProperty(f, sec, "EXPIRY", v); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	dynJson(w, common.Result{Ok: true})
}
