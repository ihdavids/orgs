package orgs

// Hooks: a program, a shell line or a webhook run when something happens.
//
// The yaml's `hooks: rules:` (common.HookRule) say which events and what to
// run; nothing runs unless `hooks: enable: true`. Every event on the /events
// stream can be hooked; four were added for this, each carrying the heading:
//
//	| statechange | a heading's keyword changed: from, to             |
//	| clockout    | a clock stopped: minutes (clockin too, no minutes)|
//	| capture     | a capture was filed: template, the new headline   |
//	| deadline    | an open heading's DEADLINE went past: date        |
//
// A statechange is found by comparing each file's keywords when it is read
// again (noteStates, called from LoadFile), not where orgs changes one, so a
// keyword changed in Emacs or vim and saved is a statechange too; one changed
// by worg or `orgs todo` is the same reload. A file read for the first time
// says nothing. A deadline is checked once a minute (deadlineTicker); the
// moment it passes is its time, or the end of its day, and the last check is
// kept on disk so a restart neither repeats one nor loses one.
//
// What a hook gets:
//
//   - run: [prog, args...] - no shell. `{{headline}}`, `{{to}}`, ... are
//     replaced inside each argument, so a title can never be more than one
//     argument, whatever it says.
//   - shell: 'line' - `sh -c`; the event reaches it only as ORGS_* variables
//     (ORGS_KIND, ORGS_HEADLINE, ORGS_FROM, ORGS_TO, ORGS_TAGS, ORGS_FILE,
//     ORGS_LINE, ORGS_HASH, ORGS_MINUTES, ORGS_DATE, ORGS_TEMPLATE, ORGS_AT),
//     never pasted into the line.
//   - Both: the event as json on stdin, ORGS_HOOK the rule's name.
//   - url: POST {"hook", "event"} as json, X-Orgs-Event / X-Orgs-Hook headers,
//     and with a secret X-Orgs-Signature: sha256=<hex hmac of the body>.
//     Retried twice on a network error or a 5xx.
//
// Rules are config only - nothing over the API can add one - and each runs at
// most `limit` times a minute (30), which is what stops a hook that edits an
// org file from setting itself off for ever. The last hundred runs are kept
// for GET /hooks and `orgs hooks`.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func hooksOn() bool { return Conf().Server != nil && Conf().Server.Hooks.Enable }

// ---------------------------------------------------------------------------
// The heading an event is about
// ---------------------------------------------------------------------------

// withHeading fills in what an event says about a heading.
func withHeading(e *Event, f *common.OrgFile, sec *org.Section) {
	if sec == nil || sec.Headline == nil {
		return
	}
	e.Hash = sec.Hash
	e.Headline = sectionTitle(sec)
	e.Line = sec.Headline.Pos.Row
	e.Tags = inheritedTagsOf(f, sec)
	if f != nil {
		e.File = f.Filename
	}
}

// inheritedTagsOf is the heading's tags, its ancestors' and the file's.
func inheritedTagsOf(f *common.OrgFile, sec *org.Section) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(ts []string) {
		for _, t := range ts {
			if t != "" && !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	for s := sec; s != nil && s.Headline != nil; s = s.Parent {
		add(s.Headline.Tags)
	}
	if f != nil && f.Doc != nil {
		add(strings.FieldsFunc(f.Doc.Get("FILETAGS"), func(r rune) bool { return r == ':' || r == ' ' }))
	}
	return out
}

// ---------------------------------------------------------------------------
// statechange: keywords compared on every reload
// ---------------------------------------------------------------------------

var hookStates = struct {
	mu   sync.Mutex
	file map[string]map[string]string
}{file: map[string]map[string]string{}}

// noteStates is called with every file read (LoadFile). The first reading of
// a file is the baseline; after that each heading whose keyword differs from
// last time is a statechange. A heading is known by its outline path and
// which of its name it is, so a rename is a new heading, not a change.
func noteStates(f *common.OrgFile) {
	if f == nil || f.Doc == nil {
		return
	}
	now := map[string]string{}
	secs := map[string]*org.Section{}
	count := map[string]int{}
	var walk func(ss []*org.Section, olp string)
	walk = func(ss []*org.Section, olp string) {
		for _, s := range ss {
			if s.Headline == nil {
				continue
			}
			p := olp + "\x00" + sectionTitle(s)
			count[p]++
			key := p + "\x00" + strconv.Itoa(count[p])
			now[key] = s.Headline.Status
			secs[key] = s
			walk(s.Children, p)
		}
	}
	walk(f.Doc.Outline.Children, "")
	hookStates.mu.Lock()
	before, had := hookStates.file[f.Filename]
	hookStates.file[f.Filename] = now
	hookStates.mu.Unlock()
	if !had {
		return
	}
	for k, to := range now {
		from, known := before[k]
		if !known || from == to {
			continue
		}
		e := Event{Kind: EventStateChange, From: from, To: to}
		withHeading(&e, f, secs[k])
		e.Msg = fmt.Sprintf("%s: %s → %s", e.Headline, orNone(from), orNone(to))
		Publish(e)
	}
}

func forgetStates(filename string) {
	hookStates.mu.Lock()
	delete(hookStates.file, filename)
	hookStates.mu.Unlock()
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// ---------------------------------------------------------------------------
// deadline: once a minute
// ---------------------------------------------------------------------------

type hookData struct {
	LastDeadlineCheck time.Time
}

func hookDataPath() string {
	dir := ""
	if Conf().PlugManager != nil {
		dir = Conf().PlugManager.HomeDir
	}
	return path.Join(dir, "hooks_data.json")
}

// wantsKind is whether any rule listens for kind.
func wantsKind(kind string) bool {
	for _, r := range Conf().Server.Hooks.Rules {
		for _, k := range r.On {
			if strings.EqualFold(k, kind) {
				return true
			}
		}
	}
	return false
}

// deadlinesPassed publishes a deadline event for every open heading whose
// deadline moment is in (since, now].
func deadlinesPassed(since, now time.Time) int {
	n := 0
	db := GetDb()
	for _, name := range db.GetFiles() {
		f := db.GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		var walk func([]*org.Section)
		walk = func(ss []*org.Section) {
			for _, s := range ss {
				h := s.Headline
				if h != nil && h.Deadline != nil && h.Deadline.Date != nil && !h.IsArchived() &&
					!(h.Status != "" && h.IsDone()) {
					d := h.Deadline.Date
					at := d.Start
					if !d.HaveTime {
						y, m, dd := at.Date()
						at = time.Date(y, m, dd, 0, 0, 0, 0, time.Local).AddDate(0, 0, 1)
					}
					if at.After(since) && !at.After(now) {
						e := Event{Kind: EventDeadline, Date: d.ToString()}
						withHeading(&e, f, s)
						e.Msg = e.Headline + " was due " + e.Date
						Publish(e)
						n++
					}
				}
				walk(s.Children)
			}
		}
		walk(f.Doc.Outline.Children)
	}
	return n
}

func deadlineTicker() {
	var data hookData
	if b, err := os.ReadFile(hookDataPath()); err == nil {
		json.Unmarshal(b, &data)
	}
	if data.LastDeadlineCheck.IsZero() || time.Since(data.LastDeadlineCheck) > 7*24*time.Hour {
		// The first run, or a server off for over a week: no flood.
		data.LastDeadlineCheck = time.Now()
	}
	for {
		now := time.Now()
		if hooksOn() && wantsKind(EventDeadline) {
			deadlinesPassed(data.LastDeadlineCheck, now)
		}
		data.LastDeadlineCheck = now
		if b, err := json.Marshal(data); err == nil {
			os.WriteFile(hookDataPath(), b, 0644)
		}
		time.Sleep(time.Minute)
	}
}

// ---------------------------------------------------------------------------
// The runner
// ---------------------------------------------------------------------------

// HookRun is one time a hook ran.
type HookRun struct {
	Hook   string
	Kind   string
	At     time.Time
	Ok     bool
	Msg    string
	Output string `json:",omitempty"`
	Millis int64
}

var hookLog = struct {
	mu   sync.Mutex
	runs []HookRun
	// When each rule last ran, for its limit.
	recent map[string][]time.Time
}{recent: map[string][]time.Time{}}

func logHookRun(r HookRun) {
	hookLog.mu.Lock()
	hookLog.runs = append(hookLog.runs, r)
	if len(hookLog.runs) > 100 {
		hookLog.runs = hookLog.runs[len(hookLog.runs)-100:]
	}
	hookLog.mu.Unlock()
	state := "ok"
	if !r.Ok {
		state = "FAILED"
	}
	fmt.Fprintf(os.Stderr, "hook %s on %s: %s %s\n", r.Hook, r.Kind, state, r.Msg)
}

// allowRun counts a run against the rule's limit a minute.
func allowRun(rule common.HookRule) bool {
	limit := rule.Limit
	if limit <= 0 {
		limit = 30
	}
	now := time.Now()
	hookLog.mu.Lock()
	defer hookLog.mu.Unlock()
	keep := []time.Time{}
	for _, t := range hookLog.recent[rule.Name] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= limit {
		hookLog.recent[rule.Name] = keep
		return false
	}
	hookLog.recent[rule.Name] = append(keep, now)
	return true
}

// hookFields is the event as name → value, what a rule matches against and
// what {{name}} and ORGS_NAME are.
func hookFields(e Event) map[string]string {
	f := map[string]string{
		"kind": e.Kind, "file": e.File, "hash": e.Hash, "headline": e.Headline, "msg": e.Msg,
		"from": e.From, "to": e.To, "tags": strings.Join(e.Tags, ":"), "template": e.Template,
		"date": e.Date, "at": e.At,
	}
	if e.Headline != "" {
		f["line"] = strconv.Itoa(e.Line)
	}
	if e.Minutes != 0 {
		f["minutes"] = strconv.FormatFloat(e.Minutes, 'f', 1, 64)
	}
	if e.Watch != "" {
		f["watch"], f["label"], f["count"] = e.Watch, e.Label, strconv.Itoa(e.Count)
	}
	if e.Title != "" {
		f["title"], f["url"] = e.Title, e.URL
	}
	return f
}

var hookMatchCache sync.Map

func hookRe(pat string) *regexp.Regexp {
	if v, ok := hookMatchCache.Load(pat); ok {
		return v.(*regexp.Regexp)
	}
	re, err := regexp.Compile(`(?i)^(?:` + pat + `)$`)
	if err != nil {
		re = regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(pat) + `$`)
	}
	hookMatchCache.Store(pat, re)
	return re
}

// ruleWants says whether a rule fires for an event.
func ruleWants(rule common.HookRule, e Event) bool {
	if e.Kind == EventHello {
		return false
	}
	on := false
	for _, k := range rule.On {
		if strings.EqualFold(k, e.Kind) || (k == "*" && e.Kind != EventReload) {
			on = true
		}
	}
	if !on {
		return false
	}
	fields := hookFields(e)
	for name, pat := range rule.Match {
		name = strings.ToLower(name)
		re := hookRe(pat)
		if name == "tags" || name == "tag" {
			any := false
			for _, t := range e.Tags {
				if re.MatchString(t) {
					any = true
				}
			}
			if !any {
				return false
			}
			continue
		}
		if !re.MatchString(fields[name]) {
			return false
		}
	}
	return true
}

var hookPlaceholderRe = regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)

func hookArg(arg string, fields map[string]string) string {
	return hookPlaceholderRe.ReplaceAllStringFunc(arg, func(m string) string {
		return fields[strings.ToLower(hookPlaceholderRe.FindStringSubmatch(m)[1])]
	})
}

func hookTimeout(rule common.HookRule) time.Duration {
	for _, s := range []string{rule.Timeout, Conf().Server.Hooks.Timeout} {
		if d, err := time.ParseDuration(strings.TrimSpace(s)); err == nil && d > 0 {
			return d
		}
	}
	return 30 * time.Second
}

func hookSecret(s string) string {
	switch {
	case strings.HasPrefix(s, "env:"):
		return os.Getenv(strings.TrimPrefix(s, "env:"))
	case strings.HasPrefix(s, "keyring:"):
		if v, err := common.KeyringGet(strings.TrimPrefix(s, "keyring:")); err == nil {
			return v
		}
		return ""
	}
	return s
}

func tail(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return strings.TrimSpace(string(b))
}

// runHook does what a rule says for one event and logs it.
func runHook(rule common.HookRule, e Event) HookRun {
	start := time.Now()
	run := HookRun{Hook: rule.Name, Kind: e.Kind, At: start}
	fields := hookFields(e)
	body, _ := json.Marshal(e)
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout(rule))
	defer cancel()
	errs := []string{}
	out := []string{}
	if len(rule.Run) > 0 || rule.Shell != "" {
		var cmd *exec.Cmd
		if len(rule.Run) > 0 {
			args := make([]string, len(rule.Run))
			for i, a := range rule.Run {
				args[i] = hookArg(a, fields)
			}
			cmd = exec.CommandContext(ctx, args[0], args[1:]...)
		} else {
			cmd = exec.CommandContext(ctx, "sh", "-c", rule.Shell)
		}
		env := append(os.Environ(), "ORGS_HOOK="+rule.Name)
		for k, v := range fields {
			env = append(env, "ORGS_"+strings.ToUpper(k)+"="+v)
		}
		cmd.Env = env
		cmd.Dir = rule.Dir
		cmd.Stdin = bytes.NewReader(body)
		b, err := cmd.CombinedOutput()
		if s := tail(b, 2000); s != "" {
			out = append(out, s)
		}
		if ctx.Err() == context.DeadlineExceeded {
			errs = append(errs, "timed out after "+hookTimeout(rule).String())
		} else if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if rule.Url != "" {
		if err := postHook(ctx, rule, e, &out); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(rule.Run) == 0 && rule.Shell == "" && rule.Url == "" {
		errs = append(errs, "the rule has no run, shell or url")
	}
	run.Ok = len(errs) == 0
	run.Msg = strings.Join(errs, "; ")
	if run.Ok {
		run.Msg = e.Msg
	}
	run.Output = strings.Join(out, "\n")
	run.Millis = time.Since(start).Milliseconds()
	logHookRun(run)
	return run
}

func postHook(ctx context.Context, rule common.HookRule, e Event, out *[]string) error {
	body, _ := json.Marshal(map[string]interface{}{"hook": rule.Name, "event": e})
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return last
			case <-time.After(time.Duration(attempt*attempt) * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, "POST", rule.Url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "orgs-hooks")
		req.Header.Set("X-Orgs-Event", e.Kind)
		req.Header.Set("X-Orgs-Hook", rule.Name)
		for k, v := range rule.Headers {
			req.Header.Set(k, hookSecret(v))
		}
		if s := hookSecret(rule.Secret); s != "" {
			m := hmac.New(sha256.New, []byte(s))
			m.Write(body)
			req.Header.Set("X-Orgs-Signature", "sha256="+hex.EncodeToString(m.Sum(nil)))
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			last = err
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			last = fmt.Errorf("%s answered %d", rule.Url, resp.StatusCode)
			continue
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("%s answered %d: %s", rule.Url, resp.StatusCode, tail(b, 200))
		}
		if s := tail(b, 500); s != "" {
			*out = append(*out, s)
		}
		return nil
	}
	return last
}

// dispatch runs every rule that wants e, a few at a time.
var hookSlots = make(chan struct{}, 4)

func dispatchHooks(e Event) {
	if !hooksOn() {
		return
	}
	for _, rule := range Conf().Server.Hooks.Rules {
		if !ruleWants(rule, e) {
			continue
		}
		if !allowRun(rule) {
			logHookRun(HookRun{Hook: rule.Name, Kind: e.Kind, At: time.Now(),
				Msg: fmt.Sprintf("skipped: ran %d times in the last minute (limit)", rule.Limit)})
			continue
		}
		rule := rule
		go func() {
			hookSlots <- struct{}{}
			defer func() { <-hookSlots }()
			runHook(rule, e)
		}()
	}
}

// startHooks listens to the event hub for as long as the server runs. Called
// from StartServer; harmless with hooks off (nothing matches).
func startHooks() {
	ch := subscribe()
	go func() {
		for e := range ch {
			dispatchHooks(e)
		}
	}()
	go deadlineTicker()
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /hooks — The Hooks, And What They Did
* POST /hooks/test — Run One Now

	#+BEGIN_EXAMPLE
	GET  /hooks            {"Ok":true,"Enable":true,"Rules":[...],"Runs":[{"Hook","Kind","At","Ok","Msg","Output","Millis"}]}
	POST /hooks/test       {"Hook":"done-log","Event":{"kind":"statechange","headline":"Ship it","from":"TODO","to":"DONE"}}
	#+END_EXAMPLE

	Runs are the last hundred, oldest first. A test runs the named rule on the
	event given - whether or not its =on= and =match= would have taken it - and
	answers how it went; only an administrator may (it runs a program).
	Rules come only from the yaml: nothing here adds or changes one.
EDOC */
func RequestHooks(w http.ResponseWriter, r *http.Request) {
	type rule struct {
		Name  string
		On    []string
		Match map[string]string `json:",omitempty"`
		Does  string
	}
	out := struct {
		Ok     bool
		Enable bool
		Rules  []rule
		Runs   []HookRun
	}{Ok: true, Rules: []rule{}, Runs: []HookRun{}}
	if Conf().Server != nil {
		out.Enable = Conf().Server.Hooks.Enable
		for _, r := range Conf().Server.Hooks.Rules {
			does := []string{}
			if len(r.Run) > 0 {
				does = append(does, "run "+strings.Join(r.Run, " "))
			}
			if r.Shell != "" {
				does = append(does, "shell "+r.Shell)
			}
			if r.Url != "" {
				does = append(does, "POST "+r.Url)
			}
			out.Rules = append(out.Rules, rule{r.Name, r.On, r.Match, strings.Join(does, "; ")})
		}
	}
	hookLog.mu.Lock()
	out.Runs = append(out.Runs, hookLog.runs...)
	hookLog.mu.Unlock()
	dynJson(w, out)
}

func PostHookTest(w http.ResponseWriter, r *http.Request) {
	who := GetUsername(r)
	if !Conf().Server.NoAuth && !GetKeystore().IsAdmin(who) {
		w.WriteHeader(http.StatusForbidden)
		dynJson(w, common.ResultMsg{Msg: "only an administrator may run a hook by hand"})
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Hook  string
		Event Event
	}
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.ResultMsg{Msg: err.Error()})
		return
	}
	for _, rule := range Conf().Server.Hooks.Rules {
		if rule.Name == req.Hook {
			if req.Event.At == "" {
				req.Event.At = time.Now().Format(time.RFC3339)
			}
			dynJson(w, runHook(rule, req.Event))
			return
		}
	}
	dynJson(w, common.ResultMsg{Msg: "no hook called " + req.Hook})
}
