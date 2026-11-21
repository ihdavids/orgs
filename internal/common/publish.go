package common

import (
	"fmt"
	"path"
	"strings"
)

// PublishTarget is one place a page can be published to: a folder on a
// branch of a GitHub repository that GitHub Pages serves (server.publish).
type PublishTarget struct {
	Name     string `yaml:"name"`
	Repo     string `yaml:"repo"`     // owner/repo
	Branch   string `yaml:"branch"`   // the branch Pages serves; the repo's default branch when empty
	Dir      string `yaml:"dir"`      // folder within the branch ("" for its root, "docs" for main/docs)
	Url      string `yaml:"url"`      // where that folder is served; worked out from repo when empty
	Api      string `yaml:"api"`      // https://api.github.com, or an Enterprise one's /api/v3
	Keyring  string `yaml:"keyring"`  // keyring entry holding the token (default orgs-github)
	TokenEnv string `yaml:"tokenEnv"` // environment variable tried when the keyring has nothing
	Message  string `yaml:"message"`  // commit message; {path} is the file's path
}

// PublishTargetInfo is a target as GET /publish/targets answers it: what a
// client shows, and nothing about the token.
type PublishTargetInfo struct {
	Name   string
	Repo   string
	Branch string
	Dir    string
	Url    string
}

type PublishTargets struct {
	Ok      bool
	Msg     string
	Targets []PublishTargetInfo
}

// PublishRequest is POST /publish. Html is a page made by the client (worg's
// gantt chart) uploaded as it is; without it the server runs Format (an
// exporter: html, gantt, revealjs...) over Query, as /file/{type} does.
type PublishRequest struct {
	Target  string
	Path    string // where in the target's folder; from Query, else index.html, when empty
	Html    string
	Format  string
	Query   string
	Theme   string
	Message string
	// Leave pictures as links to this server rather than putting them in the page.
	NoInline bool
}

type PublishResult struct {
	Ok        bool
	Msg       string
	Url       string   // where the page will be served
	Path      string   // the file's path in the repository
	Commit    string   // the commit made; empty when the file was already the same
	Unchanged bool     // the repository already had exactly this page
	Bytes     int      // the size uploaded
	Warnings  []string `json:",omitempty"`
}

// PageURL is where GitHub Pages serves this target's folder: the url the yaml
// gives, else owner.github.io/repo (or owner.github.io for a user site). The
// Dir is not part of it: Pages is pointed at a folder (main/docs), and a
// file at docs/x.html is served as /x.html. Folders under that are kept.
func (t PublishTarget) PageURL() string {
	if t.Url != "" {
		return strings.TrimRight(t.Url, "/") + "/"
	}
	owner, repo, ok := strings.Cut(t.Repo, "/")
	if !ok {
		return ""
	}
	if strings.EqualFold(repo, owner+".github.io") {
		return "https://" + strings.ToLower(owner) + ".github.io/"
	}
	return "https://" + strings.ToLower(owner) + ".github.io/" + repo + "/"
}

// CleanPublishPath is a path inside a target's folder made safe: forward
// slashes, no leading slash, nothing climbing out. A name with no extension
// gets .html.
func CleanPublishPath(p string) (string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" {
		return "", fmt.Errorf("no file name to publish as")
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", fmt.Errorf("%q climbs out of the target's folder", p)
		}
	}
	p = strings.TrimLeft(path.Clean("/"+p), "/")
	if p == "" || p == "." {
		return "", fmt.Errorf("no file name to publish as")
	}
	if path.Ext(p) == "" {
		p += ".html"
	}
	return p, nil
}

// PublishName is the name a page is published as when none was given: the
// org file's name with .html, or index.html for a query.
func PublishName(query string) string {
	q := strings.TrimSpace(query)
	if q == "" || strings.ContainsAny(q, "()&|!\"'") {
		return "index.html"
	}
	base := path.Base(strings.ReplaceAll(q, "\\", "/"))
	return strings.TrimSuffix(base, path.Ext(base)) + ".html"
}
