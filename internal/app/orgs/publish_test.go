package orgs

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// fakeContents is the contents api for one repository, in memory.
type fakeContents struct {
	mu    sync.Mutex
	files map[string][]byte // "branch:path"
	puts  int
	auth  string
}

func (f *fakeContents) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = r.Header.Get("Authorization")
	p := strings.TrimPrefix(r.URL.Path, "/repos/me/notes/contents/")
	switch r.Method {
	case "GET":
		data, ok := f.files[r.URL.Query().Get("ref")+":"+p]
		if !ok {
			w.WriteHeader(404)
			io.WriteString(w, `{"message":"Not Found"}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"type": "file", "sha": gitBlobSha(data)})
	case "PUT":
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		key := body["branch"] + ":" + p
		if old, ok := f.files[key]; ok && body["sha"] != gitBlobSha(old) {
			w.WriteHeader(409)
			io.WriteString(w, `{"message":"sha does not match"}`)
			return
		}
		data, _ := base64.StdEncoding.DecodeString(body["content"])
		f.files[key] = data
		f.puts++
		json.NewEncoder(w).Encode(map[string]any{"commit": map[string]string{"sha": "c0ffee"}})
	}
}

func TestPublishPage(t *testing.T) {
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	fake := &fakeContents{files: map[string][]byte{}}
	api := httptest.NewServer(fake)
	defer api.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "dot.png"), []byte("\x89PNG\r\n\x1a\nxx"), 0o644)

	old := *Conf().Server
	t.Cleanup(func() { *Conf().Server = old })
	Conf().Server.OrgDirs = []string{dir}
	Conf().Server.Publish = []common.PublishTarget{{Name: "site", Repo: "me/notes", Branch: "gh-pages", Dir: "plans", Api: api.URL}}
	oldTok := publishToken
	publishToken = func(common.PublishTarget) string { return "tok" }
	t.Cleanup(func() { publishToken = oldTok })

	page := `<html><img src="/images/dot.png"><img src="/images/gone.png"><a href="https://x.org/images/a.png">x</a></html>`
	res := PublishPage(&common.PublishRequest{Html: page, Path: "launch"})
	if !res.Ok {
		t.Fatalf("publish: %s", res.Msg)
	}
	if res.Url != "https://me.github.io/notes/launch.html" || res.Path != "plans/launch.html" || res.Commit != "c0ffee" {
		t.Errorf("answer: %+v", res)
	}
	if fake.auth != "Bearer tok" {
		t.Errorf("auth header %q", fake.auth)
	}
	got := string(fake.files["gh-pages:plans/launch.html"])
	if !strings.Contains(got, `src="data:image/png;base64,`) {
		t.Errorf("picture not put in the page: %s", got)
	}
	if !strings.Contains(got, `src="/images/gone.png"`) || len(res.Warnings) != 1 {
		t.Errorf("a missing picture should be left and warned about: %s %v", got, res.Warnings)
	}
	if !strings.Contains(got, `href="https://x.org/images/a.png"`) {
		t.Errorf("another site's link was touched: %s", got)
	}

	// The same page again makes no commit; a changed one updates the file.
	again := PublishPage(&common.PublishRequest{Html: page, Path: "launch.html"})
	if !again.Ok || !again.Unchanged || fake.puts != 1 {
		t.Errorf("unchanged publish: %+v, %d puts", again, fake.puts)
	}
	changed := PublishPage(&common.PublishRequest{Html: page + "<!-- 2 -->", Path: "launch.html", Target: "SITE"})
	if !changed.Ok || changed.Unchanged || fake.puts != 2 {
		t.Errorf("changed publish: %+v, %d puts", changed, fake.puts)
	}

	for _, bad := range []common.PublishRequest{
		{Html: "x", Path: "../up.html"},
		{Html: "x", Target: "nope"},
		{},
	} {
		if r := PublishPage(&bad); r.Ok {
			t.Errorf("%+v should be refused", bad)
		}
	}
}

func TestPublishNames(t *testing.T) {
	for in, want := range map[string]string{
		"notes/plan.org":   "plan.html",
		"IsTask()":         "index.html",
		"":                 "index.html",
		"C:\\org\\a b.org": "a b.html",
	} {
		if got := common.PublishName(in); got != want {
			t.Errorf("PublishName(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"a": "a.html", "/x/y.htm": "x/y.htm", "./a//b": "a/b.html"} {
		if got, err := common.CleanPublishPath(in); err != nil || got != want {
			t.Errorf("CleanPublishPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if u := (common.PublishTarget{Repo: "Me/me.github.io"}).PageURL(); u != "https://me.github.io/" {
		t.Errorf("user site url %q", u)
	}
}
