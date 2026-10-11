package github

// -----------------------------------------------------------
/* SDOC: Pollers

* GitHub and GitLab

	Issues assigned to you, pull requests you opened and reviews you have been
	asked for become TODO headings in an org file. Marking one DONE in org
	closes the issue upstream; an issue closed upstream becomes DONE in org.
	The same plugin speaks to GitHub (github.com or Enterprise) and to GitLab
	(gitlab.com or self-hosted): name it =github= or =gitlab=.

	#+BEGIN_SRC yaml
	server:
	  plugins:
	    - name: github
	      freq: 300                  # seconds between polls (default 60)
	      file: ~/org/github.org     # put it under an org directory
	      repos: [ihdavids/orgs, mycompany]   # owner/repo, or a whole owner
	      pull: [assigned, authored, review]  # the default is all three
	    - name: gitlab
	      url: https://gitlab.example.com     # leave out for gitlab.com
	      file: ~/org/gitlab.org
	#+END_SRC

	Everything else has a default:

	| key        | default                               | meaning |
	|------------+---------------------------------------+---------|
	| url        | https://api.github.com, gitlab.com    | the api: an Enterprise one is https://ghe.example.com/api/v3; a GitLab host gets /api/v4 added |
	| file       | (required)                            | the org file the items live in; created if missing |
	| repos      | everything                            | owner/repo (GitLab: group/project) or a bare owner or group for all of its repositories |
	| pull       | assigned, authored, review            | assigned: issues and PRs/MRs assigned to you; authored: PRs/MRs you opened; review: ones waiting on your review |
	| layout     | repo                                  | repo: a top-level heading per repository with its items under it; flat: every item at the top level |
	| keywords   | open TODO, review NEXT, closed DONE, merged DONE, gone DONE | the keyword each upstream state gets |
	| labels     | true                                  | labels as tags |
	| body       | 0                                     | lines of the description written under a new heading |
	| close      | issues                                | what DONE in org closes upstream: issues, all (pull and merge requests too - closed, never merged), or none |
	| keyring    | orgs-github / orgs-gitlab             | the keyring entry holding the token |
	| tokenEnv   | GITHUB_TOKEN (or GH_TOKEN) / GITLAB_TOKEN | the environment variable tried when the keyring has nothing |

	*The token* is never written in the yaml. Put it in the keyring with
	=orgs forge token github= (or =gitlab=), which asks for it without echoing
	it, or set the environment variable before starting the server. GitHub
	wants a token that can read issues and pull requests (and write issues
	to close them); GitLab wants the =api= scope. =orgs forge check= says whether
	the token works and who it belongs to; =orgs forge sync= polls now.

	*Each item* is a heading like this; notes, clocks and children written
	under it are yours and are never touched:

	#+BEGIN_SRC org
	,* ihdavids/orgs
	,** TODO The agenda drops repeating deadlines   :bug:
	:PROPERTIES:
	:GITHUB_ID:      ihdavids/orgs#123
	:GITHUB_URL:     https://github.com/ihdavids/orgs/issues/123
	:GITHUB_KIND:    issue
	:GITHUB_STATE:   open
	:GITHUB_AUTHOR:  someone
	:GITHUB_UPDATED: 2026-10-09T12:00:00Z
	:GITHUB_LABELS:  bug
	:END:
	#+END_SRC

	A GitLab item says =GITLAB_= instead, and its id is written the way GitLab
	writes it: =group/project#12= for an issue, =group/project!12= for a merge
	request. Headings are found again by that id, wherever they have been moved
	to in the file, so a poll changes only the title, the keyword, the label
	tags and the properties - each as an edit of its own line. Tags you added
	yourself stay; =GITHUB_LABELS= is how the plugin tells them from labels.

	*Keeping the two sides in step.* =GITHUB_STATE= is the upstream state as of
	the last poll. On each poll:
	- upstream changed (closed, merged, reopened): the heading's keyword follows
	  it, through the same state change as any other, so CLOSED and the logbook
	  entry are written as the file's logging settings say;
	- upstream did not change but the heading did (you marked it DONE, or moved
	  a DONE one back to TODO): the issue is closed, or reopened, upstream;
	- an item that no longer turns up in any of the queries (unassigned, review
	  given) and is still open is =gone=: it gets the gone keyword and is no
	  longer followed or written back.
	Pull and merge requests are only closed with =close: all=, never merged, and
	never when they are only review requests - DONE on a review means you
	reviewed it. Reopening is for issues only. A heading deleted from the file
	comes back on the next poll while the item is still open; archive it once
	it is DONE instead.
EDOC */

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// Forge is one configured github or gitlab poller. The yaml name picks the
// provider; a server can have several of either, each with its own file.
type Forge struct {
	Url      string            `yaml:"url"`
	Token    string            `yaml:"token"` // works, but belongs in the keyring
	TokenEnv string            `yaml:"tokenEnv"`
	Keyring  string            `yaml:"keyring"`
	File     string            `yaml:"file"`
	Repos    []string          `yaml:"repos"`
	Pull     []string          `yaml:"pull"`
	Layout   string            `yaml:"layout"`
	Keywords map[string]string `yaml:"keywords"`
	Labels   *bool             `yaml:"labels"`
	Body     int               `yaml:"body"`
	Close    string            `yaml:"close"`

	provider string // "github" or "gitlab"
	manager  *common.PluginManager
	// The http client, replaced by tests. Everything upstream goes through it.
	client *http.Client
	// now, replaced by tests so a CLOSED stamp is predictable.
	now func() time.Time

	mu           sync.Mutex
	tok          string
	triedKeyring bool
	backoffUntil time.Time
	me           string
}

func (self *Forge) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Forge) Startup(freq int, manager *common.PluginManager, opts *common.PluginOpts) {
	self.manager = manager
	if self.client == nil {
		self.client = &http.Client{Timeout: 30 * time.Second}
	}
	if self.File == "" {
		self.logf("no file: in its yaml, so there is nowhere to put anything")
	}
}

// Update is the poll. It never panics the server and never blocks it for long:
// a failure is a line on stderr and the next tick tries again.
func (self *Forge) Update(db common.ODb) {
	res := self.Sync(db)
	if !res.Ok && res.Msg != "" {
		self.logf("%s", res.Msg)
	}
}

// Kind is the provider this poller speaks to, for the sync endpoint's answer.
func (self *Forge) Kind() string { return self.provider }

func (self *Forge) logf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, self.provider+": "+format+"\n", args...)
}

func (self *Forge) prefix() string { return strings.ToUpper(self.provider) + "_" }

func (self *Forge) path() string {
	p := self.File
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return p
}

func (self *Forge) baseURL() string {
	u := strings.TrimRight(self.Url, "/")
	if self.provider == "gitlab" {
		if u == "" {
			u = "https://gitlab.com"
		}
		if !strings.Contains(u, "/api/v") {
			u += "/api/v4"
		}
		return u
	}
	if u == "" || u == "https://github.com" {
		return "https://api.github.com"
	}
	return u
}

// The token: the yaml if somebody insisted, else the keyring, else the
// environment. The keyring is asked once; a miss there is not worth a keyring
// lookup on every poll.
func (self *Forge) token() string {
	if self.Token != "" {
		return self.Token
	}
	if self.tok != "" {
		return self.tok
	}
	if !self.triedKeyring && self.manager != nil {
		self.triedKeyring = true
		self.tok = self.manager.GetPass(self.keyringName(), "keyring-nonfatal")
		if self.tok != "" {
			return self.tok
		}
	}
	for _, env := range self.tokenEnvs() {
		if t := os.Getenv(env); t != "" {
			return t
		}
	}
	return ""
}

func (self *Forge) keyringName() string {
	if self.Keyring != "" {
		return self.Keyring
	}
	return "orgs-" + self.provider
}

func (self *Forge) tokenEnvs() []string {
	if self.TokenEnv != "" {
		return []string{self.TokenEnv}
	}
	if self.provider == "gitlab" {
		return []string{"GITLAB_TOKEN"}
	}
	return []string{"GITHUB_TOKEN", "GH_TOKEN"}
}

func (self *Forge) pulls() map[string]bool {
	want := map[string]bool{}
	for _, p := range self.Pull {
		want[strings.ToLower(strings.TrimSpace(p))] = true
	}
	if len(want) == 0 {
		want = map[string]bool{"assigned": true, "authored": true, "review": true}
	}
	return want
}

// The keyword each upstream state becomes. Review requests are the one open
// state with their own keyword, because they are somebody else waiting on you.
func (self *Forge) keyword(state string) string {
	if k, ok := self.Keywords[state]; ok {
		return strings.TrimSpace(k)
	}
	switch state {
	case "open":
		return "TODO"
	case "review":
		return "NEXT"
	}
	return "DONE"
}

func (self *Forge) labelsAsTags() bool { return self.Labels == nil || *self.Labels }

func (self *Forge) layout() string {
	if strings.EqualFold(self.Layout, "flat") {
		return "flat"
	}
	return "repo"
}

// Whether DONE in org may close this kind of item upstream.
func (self *Forge) mayClose(kind string) bool {
	switch strings.ToLower(self.Close) {
	case "none", "off", "false":
		return false
	case "all":
		return true
	}
	return kind == "issue"
}

func (self *Forge) clock() time.Time {
	if self.now != nil {
		return self.now()
	}
	return time.Now()
}

func init() {
	common.AddPoller("github", func() common.Poller {
		return &Forge{provider: "github"}
	})
	common.AddPoller("gitlab", func() common.Poller {
		return &Forge{provider: "gitlab"}
	})
}
