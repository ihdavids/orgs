package orgs

// Saved-query notifications: "tell me when this query gains a match".
//
// A watch is a saved query (or an expression of its own) that the server runs
// again whenever the files change, and on a slow timer for the queries that
// change with the clock (DeadlineWithin(2) gains a heading at midnight without
// any file being touched). When the set of headings it finds grows, the owner
// hears about it: a `querymatch` event on /events, a notification in the list
// GET /notifications keeps, and a desktop popup through the notify plugin when
// the watch asks for one and the yaml has one.
//
// Rules, each about not crying wolf:
//
//  1. **The first check is a baseline, and says nothing.** After a server start,
//     after a watch is added, and after a watched query's text changes, the
//     matches are taken as what the person already knows about. Otherwise every
//     restart would announce every BLOCKED task as new.
//  2. **A heading is who it is across edits.** Matches are compared by ID or
//     CUSTOM_ID when the heading has one, else by file and outline path, and
//     only then by hash - a hash moves whenever anything above the heading is
//     edited, which would make every save "a new match". A heading that left
//     and one that arrived in the same check, in the same file, at the same line
//     or with the same title, are one heading edited (renamed, or its parent
//     renamed) and are not reported.
//  3. **Checks follow the files, debounced.** OrgDb.ReloadIndex is read once a
//     second; a check runs when it has been still for a moment, so one save (a
//     burst of filesystem events) is one check, and a half-written file is not
//     compared against a whole one.
//  4. **A query that stops parsing keeps its baseline.** The error is reported
//     on the watch; when it parses again it carries on from what it last saw.
//
// Baselines live in memory only (rule 1 makes that correct). Notifications are
// kept, fifty per user, in querywatch_data.json beside focus_data.json.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/desktop"
)

// EventQueryMatch is a watched query gaining or losing headings.
const EventQueryMatch = "querymatch"

const (
	// How many notifications a user keeps.
	watchNoticeCap = 50
	// How many headings one notification carries. A refile of a whole tree
	// into a watched file can gain hundreds; the count says how many.
	watchHeadingCap = 100
	// Quiet time after a reload before checking.
	watchDebounce = 1500 * time.Millisecond
	// The slow timer, for queries that move with the clock.
	watchSlowEvery = 5 * time.Minute
)

// ---------------------------------------------------------------------------
// Who a heading is
// ---------------------------------------------------------------------------

func propCI(props map[string]string, name string) string {
	for k, v := range props {
		if strings.EqualFold(k, name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// watchKey is a heading's identity across edits (rule 2).
func watchKey(props map[string]string, file string, olp []string, hash string) string {
	if id := propCI(props, "ID"); id != "" {
		return "id:" + id
	}
	if id := propCI(props, "CUSTOM_ID"); id != "" {
		return "cid:" + id
	}
	if len(olp) > 0 {
		return "olp:" + file + "::" + strings.Join(olp, "\x1f")
	}
	return "hash:" + hash
}

// matchSet keys a query's answer. Two headings with the same title under the
// same parent have the same outline path; the second and later get an ordinal,
// in file order, so neither is lost.
func matchSet(ms []common.WatchMatch) map[string]common.WatchMatch {
	out := make(map[string]common.WatchMatch, len(ms))
	for _, m := range ms {
		k := m.Key
		for n := 2; ; n++ {
			if _, taken := out[k]; !taken {
				break
			}
			k = fmt.Sprintf("%s#%d", m.Key, n)
		}
		m.Key = k
		out[k] = m
	}
	return out
}

func stableKey(k string) bool {
	return strings.HasPrefix(k, "id:") || strings.HasPrefix(k, "cid:")
}

// diffMatches says which headings arrived and which left between two checks,
// in file order, with an edited heading (rule 2) counted as neither.
func diffMatches(before, after map[string]common.WatchMatch) (gained, left []common.WatchMatch) {
	for k, m := range after {
		if _, ok := before[k]; !ok {
			gained = append(gained, m)
		}
	}
	for k, m := range before {
		if _, ok := after[k]; !ok {
			left = append(left, m)
		}
	}
	sortMatches(gained)
	sortMatches(left)

	// Pair an arrival with a departure that is the same heading edited. Only
	// for headings without an id: two different ids are two headings.
	usedLeft := make([]bool, len(left))
	keep := gained[:0]
	for _, g := range gained {
		paired := false
		if !stableKey(g.Key) {
			for i, l := range left {
				if usedLeft[i] || stableKey(l.Key) || l.File != g.File {
					continue
				}
				if l.Line == g.Line || l.Headline == g.Headline {
					usedLeft[i] = true
					paired = true
					break
				}
			}
		}
		if !paired {
			keep = append(keep, g)
		}
	}
	gained = keep
	var stillLeft []common.WatchMatch
	for i, l := range left {
		if !usedLeft[i] {
			stillLeft = append(stillLeft, l)
		}
	}
	return gained, stillLeft
}

func sortMatches(ms []common.WatchMatch) {
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].File != ms[j].File {
			return ms[i].File < ms[j].File
		}
		return ms[i].Line < ms[j].Line
	})
}

// ---------------------------------------------------------------------------
// Query functions for the time-based watches
// ---------------------------------------------------------------------------

// dateWithin is true when d falls on a day from today to n days ahead,
// counted in calendar days (like DeadlinePast), so "within 2 days" on a
// Monday means Monday, Tuesday or Wednesday whatever the hour. A date already
// past is not within: DeadlinePast() asks for that.
func dateWithin(d *org.OrgDate, n float64, now time.Time) bool {
	if d == nil || d.Start.IsZero() {
		return false
	}
	ahead := agendaDayNum(d.Start) - agendaDayNum(now)
	return ahead >= 0 && float64(ahead) <= n
}

func withinFunc(exp *Expr, which string) func(args ...interface{}) (interface{}, error) {
	return func(args ...interface{}) (interface{}, error) {
		n := 0.0
		if len(args) > 0 {
			v, err := toFloat(args[0])
			if err != nil {
				return false, err
			}
			n = v
		}
		return dateWithin(sdcDate(exp.Sec, which), n, time.Now()), nil
	}
}

// ---------------------------------------------------------------------------
// The engine
// ---------------------------------------------------------------------------

// What a watch saw at its last check.
type watchBase struct {
	query   string
	set     map[string]common.WatchMatch
	checked time.Time
	err     string
}

// watchEngine is the part that decides what is new, with its clock, the query
// runner and what happens to a notice passed in, so the tests drive it without
// a database.
type watchEngine struct {
	now     func() time.Time
	run     func(query string) ([]common.WatchMatch, error)
	deliver func(user string, w common.QueryWatch, n common.WatchNotice)
	persist func(map[string][]common.WatchNotice)

	// One check at a time: a timer check and a POST /watches meeting halfway
	// would both baseline, or both report.
	checkMu sync.Mutex
	mu      sync.Mutex
	base    map[string]map[string]*watchBase
	notices map[string][]common.WatchNotice
	seq     int
}

func newWatchEngine() *watchEngine {
	return &watchEngine{
		now:     time.Now,
		base:    map[string]map[string]*watchBase{},
		notices: map[string][]common.WatchNotice{},
	}
}

// check runs one watch and answers the notice it produced, if any.
func (e *watchEngine) check(user string, w common.QueryWatch, query string) (*common.WatchNotice, error) {
	e.checkMu.Lock()
	defer e.checkMu.Unlock()

	ms, err := e.run(query)
	now := e.now()

	e.mu.Lock()
	users := e.base[user]
	if users == nil {
		users = map[string]*watchBase{}
		e.base[user] = users
	}
	b := users[w.Name]
	if err != nil {
		// Rule 4: keep what it last saw.
		if b == nil || b.query != query {
			b = &watchBase{query: query}
			users[w.Name] = b
		}
		b.err = err.Error()
		b.checked = now
		e.mu.Unlock()
		return nil, err
	}
	set := matchSet(ms)
	if b == nil || b.query != query || b.set == nil {
		// Rule 1: a baseline.
		users[w.Name] = &watchBase{query: query, set: set, checked: now}
		e.mu.Unlock()
		return nil, nil
	}
	gained, left := diffMatches(b.set, set)
	b.set, b.checked, b.err = set, now, ""
	if !w.Left {
		left = nil
	}
	if len(gained) == 0 && len(left) == 0 {
		e.mu.Unlock()
		return nil, nil
	}
	if len(gained) > watchHeadingCap {
		gained = gained[:watchHeadingCap]
	}
	if len(left) > watchHeadingCap {
		left = left[:watchHeadingCap]
	}
	e.seq++
	n := common.WatchNotice{
		Id:     fmt.Sprintf("%d-%d", now.UnixNano(), e.seq),
		Watch:  w.Name,
		Label:  w.Title(),
		At:     now.Format(time.RFC3339),
		Gained: gained,
		Left:   left,
		Count:  len(set),
	}
	list := append([]common.WatchNotice{n}, e.notices[user]...)
	if len(list) > watchNoticeCap {
		list = list[:watchNoticeCap]
	}
	e.notices[user] = list
	snapshot := e.copyNotices()
	e.mu.Unlock()

	if e.persist != nil {
		e.persist(snapshot)
	}
	if e.deliver != nil {
		e.deliver(user, w, n)
	}
	return &n, nil
}

// forget drops a watch's baseline, so the next check is a baseline again.
func (e *watchEngine) forget(user, name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if users := e.base[user]; users != nil {
		delete(users, name)
	}
}

// status is what a watch last saw: matches (-1 before the first check), when,
// and why it failed.
func (e *watchEngine) status(user, name string) (int, time.Time, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.base[user][name]
	if b == nil {
		return -1, time.Time{}, ""
	}
	if b.set == nil {
		return -1, b.checked, b.err
	}
	return len(b.set), b.checked, b.err
}

// Must be called with mu held.
func (e *watchEngine) copyNotices() map[string][]common.WatchNotice {
	out := make(map[string][]common.WatchNotice, len(e.notices))
	for u, l := range e.notices {
		out[u] = append([]common.WatchNotice(nil), l...)
	}
	return out
}

func (e *watchEngine) list(user string, unreadOnly bool, limit int) common.NoticeList {
	e.mu.Lock()
	defer e.mu.Unlock()
	all := e.notices[user]
	res := common.NoticeList{Ok: true, Total: len(all), Notices: []common.WatchNotice{}}
	for _, n := range all {
		if !n.Read {
			res.Unread++
		}
		if unreadOnly && n.Read {
			continue
		}
		if limit > 0 && len(res.Notices) >= limit {
			continue
		}
		res.Notices = append(res.Notices, n)
	}
	return res
}

func (e *watchEngine) markRead(user string, act common.NoticeAct) {
	e.alter(user, func(list []common.WatchNotice) []common.WatchNotice {
		ids := idSet(act.Ids)
		for i := range list {
			if act.All || ids[list[i].Id] {
				list[i].Read = true
			}
		}
		return list
	})
}

func (e *watchEngine) clear(user string, act common.NoticeAct) {
	e.alter(user, func(list []common.WatchNotice) []common.WatchNotice {
		if act.All {
			return nil
		}
		ids := idSet(act.Ids)
		var keep []common.WatchNotice
		for _, n := range list {
			if !ids[n.Id] {
				keep = append(keep, n)
			}
		}
		return keep
	})
}

func (e *watchEngine) alter(user string, f func([]common.WatchNotice) []common.WatchNotice) {
	e.mu.Lock()
	e.notices[user] = f(e.notices[user])
	if len(e.notices[user]) == 0 {
		delete(e.notices, user)
	}
	snapshot := e.copyNotices()
	e.mu.Unlock()
	if e.persist != nil {
		e.persist(snapshot)
	}
}

func idSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// ---------------------------------------------------------------------------
// The engine wired to the server
// ---------------------------------------------------------------------------

var (
	watchesOnce    sync.Once
	theWatchEngine *watchEngine
)

func watches() *watchEngine {
	watchesOnce.Do(func() {
		e := newWatchEngine()
		e.run = runWatchQuery
		e.deliver = deliverNotice
		e.persist = saveWatchNotices
		e.notices = loadWatchNotices()
		theWatchEngine = e
	})
	return theWatchEngine
}

// runWatchQuery is /search's walk, quietly: queryEachSection logs every
// filter expansion, which on a timer would be a line every few minutes per
// watch for nothing.
func runWatchQuery(query string) ([]common.WatchMatch, error) {
	var out []common.WatchMatch
	err := evalEachSection(expandQueryFilters(query), func(v *org.Section, f *common.OrgFile) {
		t := SectionToTodo(v, f)
		if t == nil {
			return
		}
		olp := outlinePath(v)
		out = append(out, common.WatchMatch{
			Key:      watchKey(t.Props, f.Filename, olp, v.Hash),
			Headline: t.Headline,
			Status:   t.Status,
			File:     f.Filename,
			Line:     t.LineNum,
			Hash:     v.Hash,
			Path:     olp,
		})
	})
	return out, err
}

// A notify plugin, found by what it can do rather than by its type, so this
// package does not import the plugin.
// noticeSummary is one line saying what happened: "2 new: Call Sam, Fix the
// roof".
func noticeSummary(n common.WatchNotice) string {
	var parts []string
	say := func(word string, ms []common.WatchMatch) {
		if len(ms) == 0 {
			return
		}
		names := []string{}
		for i, m := range ms {
			if i == 3 {
				names = append(names, fmt.Sprintf("and %d more", len(ms)-3))
				break
			}
			names = append(names, m.Headline)
		}
		parts = append(parts, fmt.Sprintf("%d %s: %s", len(ms), word, strings.Join(names, ", ")))
	}
	say("new", n.Gained)
	say("gone", n.Left)
	return strings.Join(parts, "; ")
}

func deliverNotice(user string, w common.QueryWatch, n common.WatchNotice) {
	msg := noticeSummary(n)
	Publish(Event{Kind: EventQueryMatch, User: user, Watch: n.Watch, Label: n.Label,
		Count: n.Count, Notice: n.Id, Headings: n.Gained, Left: n.Left, Msg: msg})
	if w.Desktop {
		// Through the server's one way of saying something (notifydeliver.go):
		// the notify plugin's quiet hours hold, a click opens the first
		// heading found, and the note reaches `orgs notify listen` too.
		note := desktop.Note{Title: n.Label, Body: msg, Sound: len(n.Gained) > 0, Group: "watch:" + n.Watch}
		var file, hash, head string
		if len(n.Gained) > 0 {
			g := n.Gained[0]
			file, hash, head = g.File, g.Hash, g.Headline
			note.URL = common.HeadingLink(notifyWorgBase(), g.File, g.Line, g.Headline)
		} else {
			note.URL = common.WorgLink(notifyWorgBase(), url.Values{"app": {"StoredQueries"}})
		}
		deliverNote(note, file, hash, head)
	}
}

func watchDataPath() string {
	if Conf().PlugManager == nil || Conf().PlugManager.HomeDir == "" {
		return ""
	}
	return path.Join(Conf().PlugManager.HomeDir, "querywatch_data.json")
}

type watchData struct {
	Users map[string][]common.WatchNotice `json:"users"`
}

func loadWatchNotices() map[string][]common.WatchNotice {
	out := map[string][]common.WatchNotice{}
	p := watchDataPath()
	if p == "" {
		return out
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return out
	}
	var d watchData
	if err := json.Unmarshal(b, &d); err != nil {
		fmt.Fprintf(os.Stderr, "querywatch: %s: %v\n", p, err)
		return out
	}
	if d.Users != nil {
		out = d.Users
	}
	return out
}

var watchSaveMu sync.Mutex

func saveWatchNotices(all map[string][]common.WatchNotice) {
	p := watchDataPath()
	if p == "" {
		return
	}
	b, err := json.MarshalIndent(watchData{Users: all}, "", "  ")
	if err != nil {
		return
	}
	watchSaveMu.Lock()
	defer watchSaveMu.Unlock()
	if err := os.WriteFile(p, b, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "querywatch: %v\n", err)
	}
}

// ---------------------------------------------------------------------------
// The watch list, in the extensions file
// ---------------------------------------------------------------------------

func (self *ExtensionsConfig) GetQueryWatches(username string) []common.QueryWatch {
	self.mu.RLock()
	defer self.mu.RUnlock()
	u, ok := self.Users[username]
	if !ok {
		return nil
	}
	return append([]common.QueryWatch(nil), u.QueryWatches...)
}

// Every user's watches, for the checker.
func (self *ExtensionsConfig) allQueryWatches() map[string][]common.QueryWatch {
	self.mu.RLock()
	defer self.mu.RUnlock()
	out := map[string][]common.QueryWatch{}
	for name, u := range self.Users {
		if len(u.QueryWatches) > 0 {
			out[name] = append([]common.QueryWatch(nil), u.QueryWatches...)
		}
	}
	return out
}

func (self *ExtensionsConfig) SetQueryWatch(username string, w common.QueryWatch) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	u := self.getUser(username)
	for i := range u.QueryWatches {
		if u.QueryWatches[i].Name == w.Name {
			u.QueryWatches[i] = w
			return self.save()
		}
	}
	u.QueryWatches = append(u.QueryWatches, w)
	return self.save()
}

func (self *ExtensionsConfig) DeleteQueryWatch(username, name string) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	u, ok := self.Users[username]
	if ok {
		for i := range u.QueryWatches {
			if u.QueryWatches[i].Name == name {
				u.QueryWatches = append(u.QueryWatches[:i], u.QueryWatches[i+1:]...)
				return self.save()
			}
		}
	}
	return fmt.Errorf("nothing called %q is being watched", name)
}

// resolveWatch is the expression a watch runs, or why it has none.
func resolveWatch(user string, w common.QueryWatch) (string, error) {
	if w.StoredQuery != "" {
		sq := GetExtensions().GetStoredQuery(user, w.StoredQuery)
		if sq == nil {
			return "", fmt.Errorf("there is no saved query called %q", w.StoredQuery)
		}
		return sq.Query, nil
	}
	if strings.TrimSpace(w.Query) == "" {
		return "", fmt.Errorf("a watch needs a saved query or a query of its own")
	}
	return w.Query, nil
}

// checkUser runs one user's watches and answers what they produced.
func checkUser(user string, ws []common.QueryWatch) []common.WatchNotice {
	var out []common.WatchNotice
	for _, w := range ws {
		if w.Paused {
			// Un-pausing starts from a fresh baseline: what happened while
			// paused was asked not to be said.
			watches().forget(user, w.Name)
			continue
		}
		q, err := resolveWatch(user, w)
		if err != nil {
			watches().noteError(user, w.Name, err)
			continue
		}
		if n, _ := watches().check(user, w, q); n != nil {
			out = append(out, *n)
		}
	}
	return out
}

func (e *watchEngine) noteError(user, name string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.base[user] == nil {
		e.base[user] = map[string]*watchBase{}
	}
	b := e.base[user][name]
	if b == nil {
		b = &watchBase{}
		e.base[user][name] = b
	}
	b.err = err.Error()
	b.checked = e.now()
}

func checkAllWatches() {
	ext := GetExtensions()
	if ext == nil {
		return
	}
	for user, ws := range ext.allQueryWatches() {
		checkUser(user, ws)
	}
}

// The checker (rule 3). Started once, by registerQueryWatchRoutes.
func startWatchLoop() {
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		lastIdx := GetDb().ReloadIndex
		// A check soon after start: the baseline.
		dirtyAt := time.Now()
		lastAll := time.Time{}
		lastDay := agendaDayNum(time.Now())
		for range tick.C {
			now := time.Now()
			if idx := GetDb().ReloadIndex; idx != lastIdx {
				lastIdx = idx
				dirtyAt = now
			}
			day := agendaDayNum(now)
			switch {
			case !dirtyAt.IsZero() && now.Sub(dirtyAt) >= watchDebounce,
				!lastAll.IsZero() && now.Sub(lastAll) >= watchSlowEvery,
				day != lastDay:
				checkAllWatches()
				dirtyAt = time.Time{}
				lastAll = now
				lastDay = day
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

// registerQueryWatchRoutes adds the endpoints for saved-query notifications. Called from RestApi.
func registerQueryWatchRoutes(api *mux.Router) {
	api.HandleFunc("/watches", RequestWatches).Methods("GET")
	api.HandleFunc("/watches", PostWatch).Methods("POST")
	api.HandleFunc("/watches", DeleteWatch).Methods("DELETE")
	api.HandleFunc("/watches/check", PostWatchesCheck).Methods("POST")
	api.HandleFunc("/notifications", RequestNotifications).Methods("GET")
	api.HandleFunc("/notifications/read", PostNotificationsRead).Methods("POST")
	api.HandleFunc("/notifications/clear", PostNotificationsClear).Methods("POST")
	watches()
	startWatchLoop()
}

func watchJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	json.NewEncoder(w).Encode(v)
}

func watchRefuse(w http.ResponseWriter, status int, msg string) {
	watchJSON(w, status, common.ResultMsg{Ok: false, Msg: msg})
}

func watchUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	user := GetUsername(r)
	if user == "" {
		watchRefuse(w, http.StatusUnauthorized, "not logged in")
		return "", false
	}
	if GetExtensions() == nil {
		watchRefuse(w, http.StatusServiceUnavailable, "the extensions store is not loaded")
		return "", false
	}
	return user, true
}

func watchList(user string) common.WatchList {
	res := common.WatchList{Ok: true, Watches: []common.WatchStatus{}, Desktop: len(notifyPlugins()) > 0}
	for _, w := range GetExtensions().GetQueryWatches(user) {
		st := common.WatchStatus{QueryWatch: w, Count: -1}
		q, err := resolveWatch(user, w)
		st.Resolved = q
		count, checked, why := watches().status(user, w.Name)
		st.Count = count
		if !checked.IsZero() {
			st.Checked = checked.Format(time.RFC3339)
		}
		if err != nil {
			st.Error = err.Error()
		} else if why != "" {
			st.Error = why
		}
		res.Watches = append(res.Watches, st)
	}
	return res
}

/* SDOC: API
* GET /watches — The Queries Being Watched
	Every watch the logged-in user has: a saved query (or an expression of its
	own) that the server checks whenever the org files change, and every few
	minutes besides, saying when it finds a heading it did not find before.
	See =POST /watches=.

	*Method:* =GET=

	*Response:* A =WatchList=:
	| Field     | Meaning                                                        |
	|-----------+----------------------------------------------------------------|
	| =watches= | One entry per watch, below.                                    |
	| =desktop= | A notify plugin is configured, so =desktop= on a watch works.  |

	Each watch carries what was saved (=name=, =label=, =storedQuery= or
	=query=, =left=, =desktop=, =paused=) and what it is finding: =resolved=
	(the expression run), =count= (matches at the last check, -1 before the
	first), =checked= (when) and =error= (a saved query that is gone, or an
	expression that does not parse).
EDOC */
func RequestWatches(w http.ResponseWriter, r *http.Request) {
	user, ok := watchUser(w, r)
	if !ok {
		return
	}
	watchJSON(w, http.StatusOK, watchList(user))
}

/* SDOC: API
* POST /watches — Watch a Query
	Starts watching a query, or changes a watch with the same =name=. The first
	check happens at once and is a baseline: what the query finds now is taken
	as known, and only headings found *after* that are announced. A heading is
	told apart by its =ID= or =CUSTOM_ID= when it has one, else by its file and
	outline path, so editing a file does not make its headings "new".

	*Method:* =POST=

	*Request Body (JSON):*
	| Field         | Type   | Description                                                  |
	|---------------+--------+--------------------------------------------------------------|
	| =name=        | string | What the watch is called. Defaults to =storedQuery=.         |
	| =storedQuery= | string | A saved query by name, read afresh at every check.           |
	| =query=       | string | An expression of its own, when there is no saved query.      |
	| =label=       | string | What a notification calls it; the name when empty.           |
	| =left=        | bool   | Also say when a heading stops matching.                      |
	| =desktop=     | bool   | Also pop up on the server's desktop through the notify plugin. |
	| =paused=      | bool   | Keep it, but do not check it.                                |

	Every new match is a =querymatch= event on =GET /events= (to this user's
	streams only) and an entry in =GET /notifications=.

	For queries that move with the clock, =DeadlineWithin(2)= and
	=ScheduledWithin(0)= are true of a deadline (scheduled date) from today to
	that many days ahead; the watch notices those within five minutes of
	midnight with no file touched.

	*Response:* The =WatchList= after the change; =ok= false with =msg= when
	the saved query does not exist or the expression does not parse (400).
EDOC */
func PostWatch(w http.ResponseWriter, r *http.Request) {
	user, ok := watchUser(w, r)
	if !ok {
		return
	}
	var qw common.QueryWatch
	if err := json.NewDecoder(r.Body).Decode(&qw); err != nil {
		answerErr(w, err)
		return
	}
	qw.Name = strings.TrimSpace(qw.Name)
	qw.StoredQuery = strings.TrimSpace(qw.StoredQuery)
	if qw.Name == "" {
		qw.Name = qw.StoredQuery
	}
	if qw.Name == "" {
		watchRefuse(w, http.StatusBadRequest, "a watch needs a name, or a saved query to take one from")
		return
	}
	q, err := resolveWatch(user, qw)
	if err != nil {
		watchRefuse(w, http.StatusBadRequest, err.Error())
		return
	}
	// Run before saving, so a typo is refused rather than kept as a watch
	// that can never fire.
	if _, err := runWatchQuery(q); err != nil {
		watchRefuse(w, http.StatusBadRequest, "the query does not parse: "+err.Error())
		return
	}
	if err := GetExtensions().SetQueryWatch(user, qw); err != nil {
		answerErr(w, err)
		return
	}
	if qw.Paused {
		watches().forget(user, qw.Name)
	} else {
		// The baseline, now, so the very next change is compared against the
		// moment the watch was made.
		watches().check(user, qw, q)
	}
	res := watchList(user)
	res.Msg = fmt.Sprintf("watching %s", qw.Title())
	watchJSON(w, http.StatusOK, res)
}

/* SDOC: API
* DELETE /watches — Stop Watching a Query
	Removes a watch by name. Its notifications stay in the list.

	*Method:* =DELETE=

	*Query Parameters:*
	| Parameter | Type   | Required | Description         |
	|-----------+--------+----------+---------------------|
	| =name=    | string | yes      | The watch to remove. |

	*Response:* The =WatchList= after the change; 404 when nothing has that name.
EDOC */
func DeleteWatch(w http.ResponseWriter, r *http.Request) {
	user, ok := watchUser(w, r)
	if !ok {
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		watchRefuse(w, http.StatusBadRequest, "missing name parameter")
		return
	}
	if err := GetExtensions().DeleteQueryWatch(user, name); err != nil {
		watchRefuse(w, http.StatusNotFound, err.Error())
		return
	}
	watches().forget(user, name)
	res := watchList(user)
	res.Msg = fmt.Sprintf("stopped watching %s", name)
	watchJSON(w, http.StatusOK, res)
}

/* SDOC: API
* POST /watches/check — Check the Watches Now
	Runs the user's watches at once rather than waiting for a file to change or
	the five minute timer. Notifications are made and sent exactly as a timed
	check would.

	*Method:* =POST=

	*Response:* A =NoticeList= of the notifications this check made.
EDOC */
func PostWatchesCheck(w http.ResponseWriter, r *http.Request) {
	user, ok := watchUser(w, r)
	if !ok {
		return
	}
	made := checkUser(user, GetExtensions().GetQueryWatches(user))
	res := watches().list(user, false, 0)
	if made == nil {
		made = []common.WatchNotice{}
	}
	res.Notices = made
	res.Msg = fmt.Sprintf("%d new", len(made))
	watchJSON(w, http.StatusOK, res)
}

/* SDOC: API
* GET /notifications — What the Watches Found
	The logged-in user's notifications from watched queries, newest first, the
	last fifty kept.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                     |
	|-----------+--------+----------+---------------------------------|
	| =unread=  | bool   | no       | Only those not yet marked read. |
	| =limit=   | int    | no       | At most this many.              |

	*Response:* A =NoticeList=: =unread= and =total= (counted before
	=unread=/=limit=), and =notices=, each with =id=, =watch=, =label=, =at=,
	=count= (matches now), =read=, and =gained= / =left= headings (=key=,
	=headline=, =status=, =file=, =line=, =hash=, =path=).
EDOC */
func RequestNotifications(w http.ResponseWriter, r *http.Request) {
	user, ok := watchUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	unread := q.Get("unread") == "t" || q.Get("unread") == "true" || q.Get("unread") == "1"
	limit := 0
	fmt.Sscanf(q.Get("limit"), "%d", &limit)
	watchJSON(w, http.StatusOK, watches().list(user, unread, limit))
}

func noticeAct(w http.ResponseWriter, r *http.Request, apply func(string, common.NoticeAct)) {
	user, ok := watchUser(w, r)
	if !ok {
		return
	}
	var act common.NoticeAct
	if err := json.NewDecoder(r.Body).Decode(&act); err != nil {
		answerErr(w, err)
		return
	}
	if !act.All && len(act.Ids) == 0 {
		watchRefuse(w, http.StatusBadRequest, "say which: ids, or all")
		return
	}
	apply(user, act)
	watchJSON(w, http.StatusOK, watches().list(user, false, 0))
}

/* SDOC: API
* POST /notifications/read — Mark Notifications Read
	*Method:* =POST=

	*Request Body (JSON):* ={"ids": ["..."]}= or ={"all": true}=.

	*Response:* The =NoticeList= after the change.
EDOC */
func PostNotificationsRead(w http.ResponseWriter, r *http.Request) {
	noticeAct(w, r, watches().markRead)
}

/* SDOC: API
* POST /notifications/clear — Remove Notifications
	*Method:* =POST=

	*Request Body (JSON):* ={"ids": ["..."]}= or ={"all": true}=.

	*Response:* The =NoticeList= after the change.
EDOC */
func PostNotificationsClear(w http.ResponseWriter, r *http.Request) {
	noticeAct(w, r, watches().clear)
}
