package github

// What each api calls things, turned into one Item. Everything past this file
// is the same for GitHub and GitLab.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Item is one issue, pull request or merge request, as the org file needs it.
type Item struct {
	ID      string // owner/repo#12, or GitLab's group/project!12 for a merge request
	Repo    string
	Kind    string // issue, pr or mr
	Title   string
	URL     string
	State   string // open, closed or merged; gone is decided by the sync
	Author  string
	Updated string
	Labels  []string
	Body    string
	Role    string // assigned, authored or review
	// Assigned to you or opened by you, in any of the queries. Only such a
	// pull request is ever closed from org: one you were only asked to review
	// is somebody else's, and DONE on it means you reviewed it.
	Mine bool
}

// How much a role says about what to do with an item, when it turns up in
// more than one query: a review is somebody waiting on you.
var rolePriority = map[string]int{"review": 3, "assigned": 2, "authored": 1}

// open is every open item the configured queries find, keyed by id.
func (self *Forge) open() (map[string]Item, error) {
	if self.provider == "gitlab" {
		return self.gitlabOpen()
	}
	return self.githubOpen()
}

func (self *Forge) one(id string) (Item, error) {
	if self.provider == "gitlab" {
		return self.gitlabOne(id)
	}
	return self.githubOne(id)
}

func (self *Forge) setOpen(id, kind string, open bool) error {
	if self.provider == "gitlab" {
		return self.gitlabSetOpen(id, kind, open)
	}
	return self.githubSetOpen(id, open)
}

// whoami is the login the token belongs to.
func (self *Forge) whoami() (string, error) {
	var u struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	}
	if _, err := self.call("GET", "/user", nil, &u); err != nil {
		return "", err
	}
	if u.Login != "" {
		return u.Login, nil
	}
	return u.Username, nil
}

// Whether the yaml's repos list lets this repository in: an exact
// owner/repo, or an owner (or GitLab group, at any depth) for all of it.
func (self *Forge) wanted(repo string) bool {
	if len(self.Repos) == 0 {
		return true
	}
	r := strings.ToLower(repo)
	for _, w := range self.Repos {
		w = strings.ToLower(strings.Trim(strings.TrimSpace(w), "/"))
		if w != "" && (r == w || strings.HasPrefix(r, w+"/")) {
			return true
		}
	}
	return false
}

func addItem(into map[string]Item, it Item) {
	it.Mine = it.Role == "assigned" || it.Role == "authored"
	if old, ok := into[it.ID]; ok {
		mine := old.Mine || it.Mine
		if rolePriority[old.Role] >= rolePriority[it.Role] {
			it = old
		}
		it.Mine = mine
	}
	into[it.ID] = it
}

// splitID is owner/repo#12 as its repository, separator and number.
func splitID(id string) (repo, sep string, num int, ok bool) {
	i := strings.LastIndexAny(id, "#!")
	if i <= 0 {
		return "", "", 0, false
	}
	n, err := strconv.Atoi(id[i+1:])
	if err != nil {
		return "", "", 0, false
	}
	return id[:i], id[i : i+1], n, true
}

// ---------------------------------------------------------------------------
// GitHub
// ---------------------------------------------------------------------------

type ghIssue struct {
	Number        int    `json:"number"`
	Title         string `json:"title"`
	HTMLURL       string `json:"html_url"`
	State         string `json:"state"`
	Body          string `json:"body"`
	UpdatedAt     string `json:"updated_at"`
	RepositoryURL string `json:"repository_url"`
	User          struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	PullRequest *struct {
		MergedAt *string `json:"merged_at"`
	} `json:"pull_request"`
}

func (g ghIssue) item(role string) Item {
	repo := g.RepositoryURL
	if i := strings.Index(repo, "/repos/"); i >= 0 {
		repo = repo[i+len("/repos/"):]
	}
	it := Item{
		ID: fmt.Sprintf("%s#%d", repo, g.Number), Repo: repo, Kind: "issue",
		Title: g.Title, URL: g.HTMLURL, State: g.State, Author: g.User.Login,
		Updated: g.UpdatedAt, Body: g.Body, Role: role,
	}
	if g.PullRequest != nil {
		it.Kind = "pr"
		if g.PullRequest.MergedAt != nil && *g.PullRequest.MergedAt != "" {
			it.State = "merged"
		}
	}
	for _, l := range g.Labels {
		it.Labels = append(it.Labels, l.Name)
	}
	return it
}

// One search per kind of thing wanted. The repos are filtered here rather
// than in the query: GitHub's search ORs repo: and org: qualifiers in ways
// that are hard to predict, and a person's own queue is small.
func (self *Forge) githubOpen() (map[string]Item, error) {
	queries := map[string]string{
		"assigned": "is:open assignee:@me archived:false",
		"authored": "is:open is:pr author:@me archived:false",
		"review":   "is:open is:pr review-requested:@me archived:false",
	}
	out := map[string]Item{}
	for _, role := range []string{"assigned", "authored", "review"} {
		if !self.pulls()[role] {
			continue
		}
		u := "/search/issues?per_page=100&q=" + url.QueryEscape(queries[role])
		err := self.getAll(u, func(data json.RawMessage) error {
			var page struct {
				Items []ghIssue `json:"items"`
			}
			if err := json.Unmarshal(data, &page); err != nil {
				return err
			}
			for _, g := range page.Items {
				if it := g.item(role); self.wanted(it.Repo) {
					addItem(out, it)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (self *Forge) githubOne(id string) (Item, error) {
	repo, _, num, ok := splitID(id)
	if !ok {
		return Item{}, fmt.Errorf("%q is not an owner/repo#number", id)
	}
	var g ghIssue
	if _, err := self.call("GET", fmt.Sprintf("/repos/%s/issues/%d", repo, num), nil, &g); err != nil {
		return Item{}, err
	}
	if g.RepositoryURL == "" {
		g.RepositoryURL = "/repos/" + repo
	}
	return g.item(""), nil
}

// The issues endpoint closes a pull request too, without merging it.
func (self *Forge) githubSetOpen(id string, open bool) error {
	repo, _, num, ok := splitID(id)
	if !ok {
		return fmt.Errorf("%q is not an owner/repo#number", id)
	}
	body := map[string]string{"state": "closed", "state_reason": "completed"}
	if open {
		body = map[string]string{"state": "open"}
	}
	_, err := self.call("PATCH", fmt.Sprintf("/repos/%s/issues/%d", repo, num), body, nil)
	return err
}

// ---------------------------------------------------------------------------
// GitLab
// ---------------------------------------------------------------------------

type glItem struct {
	IID         int      `json:"iid"`
	Title       string   `json:"title"`
	WebURL      string   `json:"web_url"`
	State       string   `json:"state"`
	Description string   `json:"description"`
	UpdatedAt   string   `json:"updated_at"`
	Labels      []string `json:"labels"`
	Author      struct {
		Username string `json:"username"`
	} `json:"author"`
	References struct {
		Full string `json:"full"`
	} `json:"references"`
}

func (g glItem) item(kind, role string) Item {
	id := g.References.Full
	repo, _, _, _ := splitID(id)
	state := g.State
	switch state {
	case "opened":
		state = "open"
	case "locked":
		state = "closed"
	}
	return Item{
		ID: id, Repo: repo, Kind: kind, Title: g.Title, URL: g.WebURL, State: state,
		Author: g.Author.Username, Updated: g.UpdatedAt, Labels: g.Labels,
		Body: g.Description, Role: role,
	}
}

func (self *Forge) gitlabOpen() (map[string]Item, error) {
	type query struct{ role, kind, path string }
	var qs []query
	if self.pulls()["assigned"] {
		qs = append(qs,
			query{"assigned", "issue", "/issues?scope=assigned_to_me&state=opened"},
			query{"assigned", "mr", "/merge_requests?scope=assigned_to_me&state=opened"})
	}
	if self.pulls()["authored"] {
		qs = append(qs, query{"authored", "mr", "/merge_requests?scope=created_by_me&state=opened"})
	}
	if self.pulls()["review"] {
		// The reviewer filter wants a name, and @me is not one here.
		if self.me == "" {
			me, err := self.whoami()
			if err != nil {
				return nil, err
			}
			self.me = me
		}
		qs = append(qs, query{"review", "mr",
			"/merge_requests?scope=all&state=opened&reviewer_username=" + url.QueryEscape(self.me)})
	}
	out := map[string]Item{}
	for _, q := range qs {
		err := self.getAll(q.path+"&per_page=100", func(data json.RawMessage) error {
			var page []glItem
			if err := json.Unmarshal(data, &page); err != nil {
				return err
			}
			for _, g := range page {
				if it := g.item(q.kind, q.role); it.ID != "" && self.wanted(it.Repo) {
					addItem(out, it)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// A project is named in the url by its path, escaped, so the id is all that
// is needed to find an item again.
func gitlabPath(id string) (string, error) {
	repo, sep, num, ok := splitID(id)
	if !ok {
		return "", fmt.Errorf("%q is not a group/project#number", id)
	}
	what := "issues"
	if sep == "!" {
		what = "merge_requests"
	}
	return fmt.Sprintf("/projects/%s/%s/%d", url.PathEscape(repo), what, num), nil
}

func (self *Forge) gitlabOne(id string) (Item, error) {
	p, err := gitlabPath(id)
	if err != nil {
		return Item{}, err
	}
	var g glItem
	if _, err := self.call("GET", p, nil, &g); err != nil {
		return Item{}, err
	}
	if g.References.Full == "" {
		g.References.Full = id
	}
	kind := "issue"
	if strings.Contains(p, "/merge_requests/") {
		kind = "mr"
	}
	return g.item(kind, ""), nil
}

func (self *Forge) gitlabSetOpen(id, kind string, open bool) error {
	p, err := gitlabPath(id)
	if err != nil {
		return err
	}
	ev := "close"
	if open {
		ev = "reopen"
	}
	_, err = self.call("PUT", p, map[string]string{"state_event": ev}, nil)
	return err
}
