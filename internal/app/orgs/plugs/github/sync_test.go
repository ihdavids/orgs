package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// A fake GitHub and a fake GitLab, each holding a few items in memory.
// ---------------------------------------------------------------------------

type fakeItem struct {
	repo     string
	num      int
	pr       bool // a pull or merge request
	title    string
	state    string // open, closed, merged
	labels   []string
	body     string
	assigned bool
	authored bool
	review   bool
	updated  string
}

type fake struct {
	mu    sync.Mutex
	items []*fakeItem
	// Every write the plugin sent: "PATCH o/r#1 closed".
	writes []string
	// How many requests of any kind arrived.
	requests int
	// When set, every request is answered with this rate limit.
	limit bool
}

func (f *fake) find(repo string, num int) *fakeItem {
	for _, it := range f.items {
		if it.repo == repo && it.num == num {
			return it
		}
	}
	return nil
}

func (f *fake) openWith(role string, wantPR, wantIssue bool) []*fakeItem {
	var out []*fakeItem
	for _, it := range f.items {
		if it.state != "open" || (it.pr && !wantPR) || (!it.pr && !wantIssue) {
			continue
		}
		if (role == "assigned" && it.assigned) || (role == "authored" && it.authored) || (role == "review" && it.review) {
			out = append(out, it)
		}
	}
	return out
}

func (f *fake) rateLimited(w http.ResponseWriter) bool {
	f.requests++
	if !f.limit {
		return false
	}
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	w.WriteHeader(http.StatusForbidden)
	io.WriteString(w, `{"message":"API rate limit exceeded"}`)
	return true
}

func (it *fakeItem) github(base string) map[string]interface{} {
	m := map[string]interface{}{
		"number": it.num, "title": it.title, "state": it.state, "body": it.body,
		"html_url":       fmt.Sprintf("https://github.com/%s/issues/%d", it.repo, it.num),
		"repository_url": base + "/repos/" + it.repo,
		"updated_at":     it.updated, "user": map[string]string{"login": "someone"},
	}
	if it.state == "merged" {
		m["state"] = "closed"
	}
	var ls []map[string]string
	for _, l := range it.labels {
		ls = append(ls, map[string]string{"name": l})
	}
	m["labels"] = ls
	if it.pr {
		pr := map[string]interface{}{"merged_at": nil}
		if it.state == "merged" {
			pr["merged_at"] = "2026-10-09T00:00:00Z"
		}
		m["pull_request"] = pr
	}
	return m
}

// GitHub: search pages one item at a time, so pagination is always exercised.
func (f *fake) githubHandler(base *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.rateLimited(w) {
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"message":"Bad credentials"}`)
			return
		}
		p := r.URL.Path
		switch {
		case p == "/user":
			io.WriteString(w, `{"login":"me"}`)
		case p == "/search/issues":
			q := r.URL.Query().Get("q")
			var hits []*fakeItem
			switch {
			case strings.Contains(q, "assignee:@me"):
				hits = f.openWith("assigned", true, true)
			case strings.Contains(q, "author:@me"):
				hits = f.openWith("authored", true, false)
			case strings.Contains(q, "review-requested:@me"):
				hits = f.openWith("review", true, false)
			}
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			var items []interface{}
			if page <= len(hits) {
				items = append(items, hits[page-1].github(*base))
			}
			if page < len(hits) {
				u := *r.URL
				qs := u.Query()
				qs.Set("page", strconv.Itoa(page+1))
				u.RawQuery = qs.Encode()
				w.Header().Set("Link", fmt.Sprintf(`<%s%s>; rel="next"`, *base, u.RequestURI()))
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"total_count": len(hits), "items": items})
		case strings.HasPrefix(p, "/repos/"):
			parts := strings.Split(strings.TrimPrefix(p, "/repos/"), "/")
			if len(parts) != 4 || parts[2] != "issues" {
				http.NotFound(w, r)
				return
			}
			num, _ := strconv.Atoi(parts[3])
			it := f.find(parts[0]+"/"+parts[1], num)
			if it == nil {
				http.NotFound(w, r)
				return
			}
			if r.Method == "PATCH" {
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				it.state = body["state"]
				f.writes = append(f.writes, fmt.Sprintf("PATCH %s#%d %s", it.repo, it.num, body["state"]))
			}
			json.NewEncoder(w).Encode(it.github(*base))
		default:
			http.NotFound(w, r)
		}
	})
}

func (it *fakeItem) gitlab() map[string]interface{} {
	sep, what := "#", "issues"
	if it.pr {
		sep, what = "!", "merge_requests"
	}
	state := it.state
	if state == "open" {
		state = "opened"
	}
	return map[string]interface{}{
		"iid": it.num, "title": it.title, "state": state, "description": it.body,
		"web_url":    fmt.Sprintf("https://gitlab.example.com/%s/-/%s/%d", it.repo, what, it.num),
		"updated_at": it.updated, "labels": it.labels,
		"author":     map[string]string{"username": "someone"},
		"references": map[string]string{"full": fmt.Sprintf("%s%s%d", it.repo, sep, it.num)},
	}
}

func (f *fake) gitlabHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.rateLimited(w) {
			return
		}
		if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// The project path arrives escaped (group%2Fproject), so the raw path.
		p := strings.TrimPrefix(r.URL.EscapedPath(), "/api/v4")
		q := r.URL.Query()
		list := func(items []*fakeItem) {
			out := []interface{}{}
			for _, it := range items {
				out = append(out, it.gitlab())
			}
			json.NewEncoder(w).Encode(out)
		}
		switch {
		case p == "/user":
			io.WriteString(w, `{"username":"me","id":7}`)
		case p == "/issues" && q.Get("scope") == "assigned_to_me":
			list(f.openWith("assigned", false, true))
		case p == "/merge_requests" && q.Get("scope") == "assigned_to_me":
			list(f.openWith("assigned", true, false))
		case p == "/merge_requests" && q.Get("scope") == "created_by_me":
			list(f.openWith("authored", true, false))
		case p == "/merge_requests" && q.Get("reviewer_username") == "me":
			list(f.openWith("review", true, false))
		case strings.HasPrefix(p, "/projects/"):
			parts := strings.Split(strings.TrimPrefix(p, "/projects/"), "/")
			if len(parts) != 3 {
				http.NotFound(w, r)
				return
			}
			repo := strings.ReplaceAll(parts[0], "%2F", "/")
			num, _ := strconv.Atoi(parts[2])
			it := f.find(repo, num)
			if it == nil || it.pr != (parts[1] == "merge_requests") {
				http.NotFound(w, r)
				return
			}
			if r.Method == "PUT" {
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				if body["state_event"] == "close" {
					it.state = "closed"
				} else {
					it.state = "open"
				}
				f.writes = append(f.writes, fmt.Sprintf("PUT %s %s", r.URL.EscapedPath(), body["state_event"]))
			}
			json.NewEncoder(w).Encode(it.gitlab())
		default:
			http.NotFound(w, r)
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var fixedNow = time.Date(2026, 10, 10, 14, 3, 0, 0, time.Local)

func newGithub(t *testing.T, f *fake) (*Forge, string) {
	t.Helper()
	var base string
	srv := httptest.NewServer(f.githubHandler(&base))
	t.Cleanup(srv.Close)
	base = srv.URL
	file := filepath.Join(t.TempDir(), "github.org")
	g := &Forge{provider: "github", Url: srv.URL, Token: "test-token", File: file, client: srv.Client(),
		now: func() time.Time { return fixedNow }}
	return g, file
}

func newGitlab(t *testing.T, f *fake) (*Forge, string) {
	t.Helper()
	srv := httptest.NewServer(f.gitlabHandler())
	t.Cleanup(srv.Close)
	file := filepath.Join(t.TempDir(), "gitlab.org")
	g := &Forge{provider: "gitlab", Url: srv.URL, Token: "test-token", File: file, client: srv.Client(),
		now: func() time.Time { return fixedNow }}
	return g, file
}

func read(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func sync1(t *testing.T, g *Forge) common.ForgeSync {
	t.Helper()
	res := g.Sync(nil)
	if !res.Ok {
		t.Fatalf("sync failed: %s", res.Msg)
	}
	return res
}

// The lines of b that are not in a, and of a that are not in b, as a crude
// diff - enough to say exactly which lines a poll touched.
func diff(a, b string) (gone, added []string) {
	count := map[string]int{}
	for _, l := range strings.Split(a, "\n") {
		count[l]++
	}
	for _, l := range strings.Split(b, "\n") {
		if count[l] > 0 {
			count[l]--
		} else {
			added = append(added, l)
		}
	}
	count = map[string]int{}
	for _, l := range strings.Split(b, "\n") {
		count[l]++
	}
	for _, l := range strings.Split(a, "\n") {
		if count[l] > 0 {
			count[l]--
		} else {
			gone = append(gone, l)
		}
	}
	return gone, added
}

func sampleGithub() *fake {
	return &fake{items: []*fakeItem{
		{repo: "ihdavids/orgs", num: 12, title: "Agenda drops repeating deadlines", state: "open",
			labels: []string{"bug", "good first issue"}, assigned: true, updated: "2026-10-01T00:00:00Z",
			body: "When a deadline repeats\n* it vanishes\nthird line"},
		{repo: "ihdavids/orgs", num: 30, pr: true, title: "Gantt colours", state: "open", authored: true,
			updated: "2026-10-02T00:00:00Z"},
		{repo: "other/lib", num: 5, pr: true, title: "Fix the parser", state: "open", review: true,
			updated: "2026-10-03T00:00:00Z"},
		{repo: "elsewhere/x", num: 1, title: "Not in the repos list", state: "open", assigned: true},
	}}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestFirstPollWritesAHeadingPerItem(t *testing.T) {
	f := sampleGithub()
	g, file := newGithub(t, f)
	g.Repos = []string{"ihdavids/orgs", "other"}
	g.Body = 2
	res := sync1(t, g)
	if res.Added != 3 {
		t.Fatalf("added %d, want 3 (the elsewhere repo is not wanted): %+v", res.Added, res)
	}
	text := read(t, file)
	for _, want := range []string{
		"#+TITLE: GitHub",
		"* ihdavids/orgs\n** TODO Agenda drops repeating deadlines  :bug:good_first_issue:\n:PROPERTIES:\n:GITHUB_ID:      ihdavids/orgs#12\n",
		":GITHUB_URL:     https://github.com/ihdavids/orgs/issues/12\n",
		":GITHUB_KIND:    issue\n:GITHUB_STATE:   open\n",
		":GITHUB_LABELS:  bug good_first_issue\n:END:\nWhen a deadline repeats\n * it vanishes\n",
		"** TODO Gantt colours\n",
		":GITHUB_KIND:    pr\n",
		"* other/lib\n** NEXT Fix the parser\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	if strings.Contains(text, "third line") {
		t.Errorf("body: 2 wrote more than two lines:\n%s", text)
	}
	if strings.Contains(text, "elsewhere") {
		t.Errorf("an unwanted repository was written:\n%s", text)
	}
	if len(f.writes) != 0 {
		t.Errorf("a first poll wrote upstream: %v", f.writes)
	}
}

func TestRepollEditsOnlyTheLinesThatChanged(t *testing.T) {
	f := sampleGithub()
	g, file := newGithub(t, f)
	sync1(t, g)

	// A person works in the file: notes under the issue, a tag of their own,
	// a child heading, the property drawer indented their way.
	text := read(t, file)
	text = strings.Replace(text, "** TODO Agenda drops repeating deadlines  :bug:good_first_issue:",
		"** TODO [#A] Agenda drops repeating deadlines  :bug:good_first_issue:mine:", 1)
	text = strings.Replace(text, ":GITHUB_LABELS:  bug good_first_issue\n:END:\n",
		":GITHUB_LABELS:  bug good_first_issue\n:END:\nMy notes:\n| a | b |\n|---+---|\n*** TODO Write a test\n", 1)
	os.WriteFile(file, []byte(text), 0644)

	// Nothing upstream changed: nothing is written.
	before, _ := os.Stat(file)
	res := sync1(t, g)
	if res.Updated != 0 || res.Added != 0 || read(t, file) != text {
		t.Fatalf("an idle poll changed the file (%+v):\n%s", res, read(t, file))
	}
	after, _ := os.Stat(file)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("an idle poll wrote the file")
	}

	// Upstream: renamed, a label swapped, updated.
	it := f.find("ihdavids/orgs", 12)
	it.title = "Agenda drops repeating deadlines on Sundays"
	it.labels = []string{"bug", "agenda"}
	it.updated = "2026-10-09T00:00:00Z"
	sync1(t, g)
	got := read(t, file)
	gone, added := diff(text, got)
	wantGone := []string{
		"** TODO [#A] Agenda drops repeating deadlines  :bug:good_first_issue:mine:",
		":GITHUB_UPDATED: 2026-10-01T00:00:00Z",
		":GITHUB_LABELS:  bug good_first_issue",
	}
	wantAdded := []string{
		"** TODO [#A] Agenda drops repeating deadlines on Sundays  :bug:mine:agenda:",
		":GITHUB_UPDATED: 2026-10-09T00:00:00Z",
		":GITHUB_LABELS:  bug agenda",
	}
	if !sameStrings(gone, wantGone) || !sameStrings(added, wantAdded) {
		t.Errorf("the poll touched\n-%q\n+%q\nwant\n-%q\n+%q", gone, added, wantGone, wantAdded)
	}
	if !strings.Contains(got, "My notes:\n| a | b |\n|---+---|\n*** TODO Write a test\n") {
		t.Errorf("the notes were not kept as written:\n%s", got)
	}
}

func TestClosedUpstreamBecomesDoneAndIsNotPushedBack(t *testing.T) {
	f := sampleGithub()
	g, file := newGithub(t, f)
	sync1(t, g)
	f.find("ihdavids/orgs", 12).state = "closed"
	f.find("ihdavids/orgs", 30).state = "merged"

	res := sync1(t, g)
	if res.Closed != 2 {
		t.Fatalf("closed %d, want 2: %+v", res.Closed, res)
	}
	text := read(t, file)
	for _, want := range []string{
		"** DONE Agenda drops repeating deadlines  :bug:good_first_issue:\nCLOSED: [2026-10-10 Sat 14:03]\n:PROPERTIES:",
		":GITHUB_STATE:   closed\n",
		"** DONE Gantt colours\nCLOSED: [2026-10-10 Sat 14:03]\n",
		":GITHUB_STATE:   merged\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	// The poller's own DONE must not read as a person's next time round.
	sync1(t, g)
	sync1(t, g)
	if len(f.writes) != 0 {
		t.Errorf("the poller pushed its own change back upstream: %v", f.writes)
	}
	if read(t, file) != text {
		t.Errorf("later polls changed the file again:\n%s", read(t, file))
	}
}

func TestDoneInOrgClosesTheIssueUpstreamOnce(t *testing.T) {
	f := sampleGithub()
	g, file := newGithub(t, f)
	sync1(t, g)
	text := strings.Replace(read(t, file), "** TODO Agenda drops", "** DONE Agenda drops", 1)
	os.WriteFile(file, []byte(text), 0644)

	res := sync1(t, g)
	if res.Pushed != 1 || len(f.writes) != 1 || f.writes[0] != "PATCH ihdavids/orgs#12 closed" {
		t.Fatalf("writes %v (%+v)", f.writes, res)
	}
	if !strings.Contains(read(t, file), ":GITHUB_STATE:   closed") {
		t.Errorf("the state was not recorded:\n%s", read(t, file))
	}
	sync1(t, g)
	if len(f.writes) != 1 {
		t.Errorf("closed twice: %v", f.writes)
	}

	// Back to TODO here: reopened there, and only once.
	text = strings.Replace(read(t, file), "** DONE Agenda drops", "** TODO Agenda drops", 1)
	os.WriteFile(file, []byte(text), 0644)
	sync1(t, g)
	sync1(t, g)
	if len(f.writes) != 2 || f.writes[1] != "PATCH ihdavids/orgs#12 open" {
		t.Fatalf("writes %v", f.writes)
	}
	if !strings.Contains(read(t, file), ":GITHUB_STATE:   open") {
		t.Errorf("the reopen was not recorded:\n%s", read(t, file))
	}
}

func TestPullRequestsAreOnlyClosedWhenAskedAndNeverForAReview(t *testing.T) {
	f := sampleGithub()
	g, file := newGithub(t, f)
	sync1(t, g)
	text := read(t, file)
	text = strings.Replace(text, "** TODO Gantt colours", "** DONE Gantt colours", 1)
	text = strings.Replace(text, "** NEXT Fix the parser", "** DONE Fix the parser", 1)
	os.WriteFile(file, []byte(text), 0644)

	sync1(t, g)
	if len(f.writes) != 0 {
		t.Fatalf("a pull request was closed with the default close: issues: %v", f.writes)
	}
	g.Close = "all"
	sync1(t, g)
	if len(f.writes) != 1 || f.writes[0] != "PATCH ihdavids/orgs#30 closed" {
		t.Fatalf("close: all should close my own PR and leave the review alone: %v", f.writes)
	}
}

func TestAnItemNoLongerAskedForIsGone(t *testing.T) {
	f := sampleGithub()
	g, file := newGithub(t, f)
	sync1(t, g)
	f.find("other/lib", 5).review = false // reviewed
	res := sync1(t, g)
	if res.Gone != 1 {
		t.Fatalf("gone %d: %+v", res.Gone, res)
	}
	text := read(t, file)
	if !strings.Contains(text, "** DONE Fix the parser") || !strings.Contains(text, ":GITHUB_STATE:   gone") {
		t.Errorf("not marked gone:\n%s", text)
	}
	// Gone is not followed and never pushed: a poll now costs only the
	// searches, the same as a poll with nothing to follow up.
	n := f.requests
	sync1(t, g)
	idle := f.requests - n
	n = f.requests
	sync1(t, g)
	if len(f.writes) != 0 {
		t.Errorf("pushed a gone item: %v", f.writes)
	}
	if f.requests-n != idle || idle != 4 {
		t.Errorf("a gone item was asked about again (%d requests, %d searches)", f.requests-n, idle)
	}
	// Asked for again: it comes back to life.
	f.find("other/lib", 5).review = true
	sync1(t, g)
	if !strings.Contains(read(t, file), "** NEXT Fix the parser") {
		t.Errorf("not brought back:\n%s", read(t, file))
	}
}

func TestStateChangesGoThroughTheServer(t *testing.T) {
	f := sampleGithub()
	g, file := newGithub(t, f)
	var calls []string
	g.manager = &common.PluginManager{
		ChangeStatus: func(filename string, row int, to string) (string, error) {
			lines, _ := readLines(filename)
			calls = append(calls, fmt.Sprintf("%d %s %s", row, to, lines[row]))
			lines[row] = strings.Replace(lines[row], "TODO", to, 1)
			lines = splice(lines, row+1, []string{"CLOSED: [server]"})
			return to, os.WriteFile(filename, []byte(strings.Join(lines, "\n")+"\n"), 0644)
		},
	}
	sync1(t, g)
	f.find("ihdavids/orgs", 12).state = "closed"
	f.find("ihdavids/orgs", 30).state = "closed"
	sync1(t, g)
	if len(calls) != 2 {
		t.Fatalf("server state change called %d times: %v", len(calls), calls)
	}
	// The second call was measured after the first added lines above it.
	for _, c := range calls {
		if !strings.Contains(c, "** TODO") {
			t.Errorf("a state change was aimed at a line that is not its heading: %q", c)
		}
	}
	text := read(t, file)
	if strings.Count(text, ":GITHUB_STATE:   closed") != 2 || strings.Contains(text, "[2026-10-10") {
		t.Errorf("the server's change was not the one used:\n%s", text)
	}
}

func TestAFailedStateChangeIsNotRecorded(t *testing.T) {
	f := sampleGithub()
	g, file := newGithub(t, f)
	sync1(t, g)
	f.find("ihdavids/orgs", 12).state = "closed"
	// A write that fails: the state must not be recorded, or the next poll
	// would read the stale TODO as a person reopening the issue.
	os.Chmod(file, 0444)
	g.Sync(nil)
	os.Chmod(file, 0644)
	if strings.Contains(read(t, file), ":GITHUB_STATE:   closed") {
		t.Fatalf("recorded a state whose keyword change failed")
	}
	sync1(t, g)
	if len(f.writes) != 0 || !strings.Contains(read(t, file), "** DONE Agenda drops") {
		t.Errorf("writes %v\n%s", f.writes, read(t, file))
	}
}

func TestRateLimitWritesNothingAndBacksOff(t *testing.T) {
	f := sampleGithub()
	g, file := newGithub(t, f)
	sync1(t, g)
	text := read(t, file)
	f.limit = true
	res := g.Sync(nil)
	if res.Ok || !strings.Contains(res.Msg, "rate limited") {
		t.Fatalf("want a rate limit, got %+v", res)
	}
	n := f.requests
	g.Sync(nil)
	if f.requests != n {
		t.Errorf("asked again during the back-off")
	}
	if read(t, file) != text {
		t.Errorf("a failed fetch changed the file")
	}
}

func TestFlatLayout(t *testing.T) {
	f := sampleGithub()
	g, file := newGithub(t, f)
	g.Layout = "flat"
	g.Repos = []string{"ihdavids/orgs"}
	sync1(t, g)
	text := read(t, file)
	if !strings.Contains(text, "\n* TODO Agenda drops") || strings.Contains(text, "* ihdavids/orgs\n") {
		t.Errorf("flat layout:\n%s", text)
	}
}

func TestGitlab(t *testing.T) {
	f := &fake{items: []*fakeItem{
		{repo: "team/backend", num: 4, title: "Slow login", state: "open", assigned: true, labels: []string{"perf"}, updated: "2026-10-01T00:00:00Z"},
		{repo: "team/backend", num: 9, pr: true, title: "Cache sessions", state: "open", review: true, updated: "2026-10-01T00:00:00Z"},
		{repo: "team/sub/frontend", num: 2, pr: true, title: "Dark mode", state: "open", authored: true, updated: "2026-10-01T00:00:00Z"},
	}}
	g, file := newGitlab(t, f)
	g.Repos = []string{"team"}
	res := sync1(t, g)
	if res.Added != 3 {
		t.Fatalf("added %d: %+v", res.Added, res)
	}
	text := read(t, file)
	for _, want := range []string{
		"* team/backend\n** TODO Slow login  :perf:\n:PROPERTIES:\n:GITLAB_ID:      team/backend#4\n",
		"** NEXT Cache sessions\n:PROPERTIES:\n:GITLAB_ID:      team/backend!9\n",
		":GITLAB_KIND:    mr\n",
		"* team/sub/frontend\n** TODO Dark mode\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}

	// DONE here closes the issue there, by its escaped project path.
	os.WriteFile(file, []byte(strings.Replace(text, "** TODO Slow login", "** DONE Slow login", 1)), 0644)
	sync1(t, g)
	if len(f.writes) != 1 || f.writes[0] != "PUT /api/v4/projects/team%2Fbackend/issues/4 close" {
		t.Fatalf("writes %v", f.writes)
	}
	// A merge request merged upstream becomes DONE; nothing is sent back.
	f.find("team/sub/frontend", 2).state = "merged"
	sync1(t, g)
	sync1(t, g)
	text = read(t, file)
	if !strings.Contains(text, "** DONE Dark mode\nCLOSED:") || !strings.Contains(text, ":GITLAB_STATE:   merged") {
		t.Errorf("merge not followed:\n%s", text)
	}
	if len(f.writes) != 1 {
		t.Errorf("pushed back: %v", f.writes)
	}
}

func TestCheckSaysWhoTheTokenIs(t *testing.T) {
	f := sampleGithub()
	g, _ := newGithub(t, f)
	if res := g.Check(); !res.Ok || res.User != "me" {
		t.Fatalf("%+v", res)
	}
	g.Token = "wrong"
	if res := g.Check(); res.Ok || !strings.Contains(res.Msg, "401") {
		t.Fatalf("a bad token should say so: %+v", res)
	}
}

func TestKeywordsComeFromTheFile(t *testing.T) {
	kw := newKeywords()
	kw.parse("TODO NEXT(n!) | DONE(d@) CANCELLED\nREPORT BUG | FIXED")
	for _, k := range []string{"TODO", "NEXT", "REPORT", "BUG"} {
		if !kw.all[k] || kw.done[k] {
			t.Errorf("%s should be an active keyword", k)
		}
	}
	for _, k := range []string{"DONE", "CANCELLED", "FIXED"} {
		if !kw.done[k] {
			t.Errorf("%s should be a done keyword", k)
		}
	}
	_, title, tags := splitHeadline("WAITING Talk to Bob  :x:y:", kw)
	if title != "WAITING Talk to Bob" || len(tags) != 2 {
		t.Errorf("unknown keyword: %q %v", title, tags)
	}
}
