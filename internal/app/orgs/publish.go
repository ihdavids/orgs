package orgs

// Publishing a page to GitHub Pages: one file committed to a folder on a
// branch through the contents api (GET for the file's sha, PUT with the new
// bytes), so the server needs a token and nothing else - no checkout, no git.
//
// The page is either made here (an exporter run over a file or a query, as
// /file/{type} does) or handed in whole (worg's gantt chart, which is drawn in
// the browser). Either way the pictures it shows from this server are put into
// it as data first, because the page is about to be read somewhere that cannot
// reach this server.

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/assets"
	"github.com/ihdavids/orgs/internal/common"
)

/* SDOC: API
* GET /publish/targets — Where Pages Can Be Published

	The targets named under =server.publish= (see *Publishing to GitHub
	Pages*): name, repository, branch, folder and the url the folder is
	served at. Nothing about the token.

	*Response:* ={"Ok": true, "Targets": [{"Name": "site", "Repo": "me/notes", "Branch": "gh-pages", "Dir": "", "Url": "https://me.github.io/notes/"}]}=
EDOC */
func RequestPublishTargets(w http.ResponseWriter, r *http.Request) {
	out := common.PublishTargets{Ok: true, Targets: []common.PublishTargetInfo{}}
	for _, t := range Conf().Server.Publish {
		out.Targets = append(out.Targets, common.PublishTargetInfo{
			Name: t.Name, Repo: t.Repo, Branch: t.Branch, Dir: t.Dir, Url: t.PageURL(),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

/* SDOC: API
* POST /publish — Publish a Page to GitHub Pages

	Commits one page to a target from =server.publish=. Either the page is in
	the request (=Html=), or the server makes it by running an exporter
	(=Format=, default =html=) over =Query= - a file, or for the query
	exporters such as =gantt= an expression - exactly as =/file/{type}= would.

	| Field    | Meaning |
	|----------+---------|
	| Target   | the target's name; may be left out when there is only one |
	| Path     | where in the target's folder (default: the file's name with .html, or index.html) |
	| Html     | the page itself, uploaded as it is |
	| Format   | the exporter, when Html is empty |
	| Query    | the file or expression the exporter is given |
	| Theme    | the html theme |
	| Message  | the commit message |
	| NoInline | leave /images/ links pointing at this server |

	*Response:* ={"Ok": true, "Url": "https://me.github.io/notes/plan.html", "Path": "plan.html", "Commit": "…", "Unchanged": false, "Bytes": 51234}=.
	=Unchanged= says the repository already had exactly this page, and no commit was made.
EDOC */
func PostPublish(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req common.PublishRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.PublishResult{Msg: "could not read the request: " + err.Error()})
		return
	}
	res := PublishPage(&req)
	json.NewEncoder(w).Encode(res)
}

// PublishPage makes (or takes) the page and commits it.
func PublishPage(req *common.PublishRequest) common.PublishResult {
	t, err := publishTarget(req.Target)
	if err != nil {
		return common.PublishResult{Msg: err.Error()}
	}
	name := req.Path
	if strings.TrimSpace(name) == "" {
		name = common.PublishName(req.Query)
	}
	rel, err := common.CleanPublishPath(name)
	if err != nil {
		return common.PublishResult{Msg: err.Error()}
	}

	page := req.Html
	if page == "" {
		if strings.TrimSpace(req.Query) == "" {
			return common.PublishResult{Msg: "nothing to publish: no page, and no file or query to export"}
		}
		format := req.Format
		if format == "" {
			format = "html"
		}
		res, _ := ExportToString(exportViewFor(db, req.Query, Conf().Server.Refresh.ExportRefresh(), format),
			&common.ExportToFile{Name: format, Query: req.Query, Props: map[string]string{"theme": req.Theme}})
		if !res.Ok {
			return common.PublishResult{Msg: strings.TrimSpace(res.Msg)}
		}
		page = res.Msg
	}

	var warnings []string
	if !req.NoInline {
		page, warnings = inlineServerMedia(page, mediaRoots())
	}

	repoPath := rel
	if d := strings.Trim(strings.ReplaceAll(t.Dir, "\\", "/"), "/"); d != "" {
		repoPath = d + "/" + rel
	}
	msg := req.Message
	if msg == "" {
		msg = t.Message
	}
	if msg == "" {
		msg = "orgs: publish {path}"
	}
	msg = strings.ReplaceAll(msg, "{path}", repoPath)

	pub := &ghPublisher{target: t, token: publishToken(t)}
	if pub.token == "" {
		return common.PublishResult{Msg: fmt.Sprintf("no GitHub token for %q: run orgs forge token github (keyring entry %s), or set %s before starting the server",
			t.Name, publishKeyring(t), strings.Join(publishTokenEnvs(t), " or "))}
	}
	commit, unchanged, err := pub.put(repoPath, []byte(page), msg)
	if err != nil {
		return common.PublishResult{Msg: err.Error(), Path: repoPath, Warnings: warnings}
	}
	return common.PublishResult{
		Ok: true, Url: t.PageURL() + rel, Path: repoPath, Commit: commit, Unchanged: unchanged,
		Bytes: len(page), Warnings: warnings,
	}
}

func publishTarget(name string) (common.PublishTarget, error) {
	ts := Conf().Server.Publish
	if len(ts) == 0 {
		return common.PublishTarget{}, errors.New("no publish targets: add one under server.publish in the yaml")
	}
	if name == "" {
		if len(ts) == 1 {
			return checkTarget(ts[0])
		}
		return common.PublishTarget{}, fmt.Errorf("say which target: %s", strings.Join(targetNames(ts), ", "))
	}
	for _, t := range ts {
		if strings.EqualFold(t.Name, name) {
			return checkTarget(t)
		}
	}
	return common.PublishTarget{}, fmt.Errorf("no publish target called %q (there is %s)", name, strings.Join(targetNames(ts), ", "))
}

func checkTarget(t common.PublishTarget) (common.PublishTarget, error) {
	if o, r, ok := strings.Cut(t.Repo, "/"); !ok || o == "" || r == "" || strings.Contains(r, "/") {
		return t, fmt.Errorf("publish target %q: repo should be owner/repo, not %q", t.Name, t.Repo)
	}
	return t, nil
}

func targetNames(ts []common.PublishTarget) []string {
	names := []string{}
	for _, t := range ts {
		names = append(names, t.Name)
	}
	return names
}

// The token, as the GitHub poller finds it: the keyring, else the environment.
// A variable so tests never ask this machine's keyring.
var publishToken = publishTokenOf

func publishTokenOf(t common.PublishTarget) string {
	if pm := Conf().PlugManager; pm != nil {
		if tok := pm.GetPass(publishKeyring(t), "keyring-nonfatal"); tok != "" {
			return tok
		}
	} else if tok, err := common.KeyringGet(publishKeyring(t)); err == nil && tok != "" {
		return tok
	}
	for _, env := range publishTokenEnvs(t) {
		if tok := os.Getenv(env); tok != "" {
			return tok
		}
	}
	return ""
}

func publishKeyring(t common.PublishTarget) string {
	if t.Keyring != "" {
		return t.Keyring
	}
	return "orgs-github"
}

func publishTokenEnvs(t common.PublishTarget) []string {
	if t.TokenEnv != "" {
		return []string{t.TokenEnv}
	}
	return []string{"GITHUB_TOKEN", "GH_TOKEN"}
}

// ---------------------------------------------------------------------------

type ghPublisher struct {
	target common.PublishTarget
	token  string
	client *http.Client
}

// Two publishes of the same file at once would each read the same sha and the
// second PUT would be refused; one at a time per file avoids that here.
var publishLocks sync.Map

func (self *ghPublisher) api() string {
	if self.target.Api != "" {
		return strings.TrimRight(self.target.Api, "/")
	}
	return "https://api.github.com"
}

func (self *ghPublisher) contentsURL(repoPath string) string {
	segs := strings.Split(repoPath, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return fmt.Sprintf("%s/repos/%s/contents/%s", self.api(), self.target.Repo, strings.Join(segs, "/"))
}

func (self *ghPublisher) do(method, u string, body interface{}, out interface{}) (int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, u, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+self.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := self.client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("github %s %s: %s - %s", method, req.URL.Path, resp.Status, ghMessage(data))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("github %s %s: could not read the answer: %v", method, req.URL.Path, err)
		}
	}
	return resp.StatusCode, nil
}

func ghMessage(data []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &m) == nil && m.Message != "" {
		return m.Message
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// The sha git gives a file's bytes, which is what the contents api calls a
// file's sha: equal means the repository already has this page.
func gitBlobSha(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// put commits data at repoPath, answering with the commit's sha, or unchanged
// when the file there already is data.
func (self *ghPublisher) put(repoPath string, data []byte, msg string) (string, bool, error) {
	key := self.target.Repo + "@" + self.target.Branch + ":" + repoPath
	mu, _ := publishLocks.LoadOrStore(key, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	u := self.contentsURL(repoPath)
	// A PUT that lost a race with another commit to the same file is refused
	// (409, or 422 for a stale sha); reading the sha again and retrying once
	// covers someone pushing by hand at the same moment.
	for try := 0; ; try++ {
		sha, err := self.currentSha(u)
		if err != nil {
			return "", false, err
		}
		if sha == gitBlobSha(data) {
			return "", true, nil
		}
		body := map[string]string{
			"message": msg,
			"content": base64.StdEncoding.EncodeToString(data),
		}
		if self.target.Branch != "" {
			body["branch"] = self.target.Branch
		}
		if sha != "" {
			body["sha"] = sha
		}
		var out struct {
			Commit struct {
				Sha string `json:"sha"`
			} `json:"commit"`
		}
		status, err := self.do("PUT", u, body, &out)
		if err == nil {
			return out.Commit.Sha, false, nil
		}
		if (status == http.StatusConflict || status == http.StatusUnprocessableEntity) && try == 0 {
			continue
		}
		return "", false, self.explain(status, err)
	}
}

// The file's sha on the branch, or "" when there is no file there yet.
func (self *ghPublisher) currentSha(u string) (string, error) {
	get := u
	if self.target.Branch != "" {
		get += "?ref=" + url.QueryEscape(self.target.Branch)
	}
	var cur struct {
		Sha  string `json:"sha"`
		Type string `json:"type"`
	}
	status, err := self.do("GET", get, nil, &cur)
	switch {
	case status == http.StatusNotFound:
		// No file there - or no such branch or repository, which the PUT
		// will say more clearly than a guess here.
		return "", nil
	case err != nil:
		return "", self.explain(status, err)
	case cur.Type != "" && cur.Type != "file":
		return "", fmt.Errorf("%s is a %s in %s, not a file", strings.TrimPrefix(u, self.api()), cur.Type, self.target.Repo)
	}
	return cur.Sha, nil
}

func (self *ghPublisher) explain(status int, err error) error {
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("the GitHub token was refused (401): store a new one with orgs forge token github - %v", err)
	case http.StatusForbidden, http.StatusNotFound:
		return fmt.Errorf("%v (does the token have Contents read and write on %s, and does branch %q exist?)", err, self.target.Repo, self.branchName())
	}
	return err
}

func (self *ghPublisher) branchName() string {
	if self.target.Branch == "" {
		return "(default)"
	}
	return self.target.Branch
}

// ---------------------------------------------------------------------------
// Pictures. A page made here points at this server for its pictures
// (/images/ is the first org directory, /orgimages/ and /orgfonts/ the
// theme's); published, those links answer nothing. Each is read and put in
// the page as a data url instead. A file that is missing, outside its folder
// or too big is left as it was and said so.

const (
	inlineMaxFile  = 10 << 20
	inlineMaxTotal = 60 << 20
)

// mediaRoots is where each server prefix reads from, in the order tried.
func mediaRoots() map[string][]string {
	roots := map[string][]string{}
	if dirs := Conf().Server.OrgDirs; len(dirs) > 0 {
		if d, err := filepath.Abs(dirs[0]); err == nil {
			roots["/images/"] = []string{d}
		}
	}
	if tp, err := filepath.Abs(Conf().TemplateImagesPath); err == nil && Conf().TemplateImagesPath != "" {
		roots["/orgimages/"] = append(roots["/orgimages/"], tp)
	}
	roots["/orgimages/"] = append(roots["/orgimages/"], filepath.Join(assets.Dir("templates"), "html_styles", "images"))
	if fp, err := filepath.Abs(Conf().TemplateFontPath); err == nil && Conf().TemplateFontPath != "" {
		roots["/orgfonts/"] = append(roots["/orgfonts/"], fp)
	}
	roots["/orgfonts/"] = append(roots["/orgfonts/"], filepath.Join(assets.Dir("templates"), "fonts"))
	return roots
}

// A server-relative link in an attribute or a css url(): src="/images/a.png",
// url('/orgfonts/x.woff2'). Only those three prefixes; anything else is left.
var serverMediaRe = regexp.MustCompile(`((?:src|href|poster|data)\s*=\s*["']|url\(\s*["']?)(/(?:images|orgimages|orgfonts)/[^"')\s?#]+)`)

func inlineServerMedia(page string, roots map[string][]string) (string, []string) {
	var warnings []string
	total := 0
	cache := map[string]string{}
	warned := map[string]bool{}
	warn := func(link, why string) {
		if !warned[link] {
			warned[link] = true
			warnings = append(warnings, link+": "+why)
		}
	}
	out := serverMediaRe.ReplaceAllStringFunc(page, func(m string) string {
		sub := serverMediaRe.FindStringSubmatch(m)
		lead, link := sub[1], sub[2]
		// An <a href> to a picture is a link to open it; making it a data
		// url is fine for a picture and useless for anything else, so only
		// things a browser shows inline are taken.
		if d, ok := cache[link]; ok {
			return lead + d
		}
		file, err := mediaFile(link, roots)
		if err != nil {
			warn(link, err.Error())
			return m
		}
		data, err := os.ReadFile(file)
		if err != nil {
			warn(link, "could not be read")
			return m
		}
		if len(data) > inlineMaxFile || total+len(data) > inlineMaxTotal {
			warn(link, fmt.Sprintf("too big to put in the page (%d KB)", len(data)>>10))
			return m
		}
		ctype := mime.TypeByExtension(strings.ToLower(filepath.Ext(file)))
		if ctype == "" {
			ctype = http.DetectContentType(data)
		}
		if i := strings.Index(ctype, ";"); i >= 0 {
			ctype = ctype[:i]
		}
		total += len(data)
		d := "data:" + ctype + ";base64," + base64.StdEncoding.EncodeToString(data)
		cache[link] = d
		return lead + d
	})
	return out, warnings
}

func mediaFile(link string, roots map[string][]string) (string, error) {
	for prefix, dirs := range roots {
		if !strings.HasPrefix(link, prefix) {
			continue
		}
		rel, err := url.PathUnescape(strings.TrimPrefix(link, prefix))
		if err != nil {
			return "", errors.New("not a readable path")
		}
		rel = strings.TrimLeft(path.Clean("/"+rel), "/")
		for _, d := range dirs {
			f := filepath.Join(d, filepath.FromSlash(rel))
			if st, err := os.Stat(f); err == nil && !st.IsDir() {
				return f, nil
			}
		}
		return "", errors.New("not found on this server")
	}
	return "", errors.New("not served by this server")
}
