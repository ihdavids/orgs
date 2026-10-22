package orgs

// Where the browser can fetch the pictures an org file links to, for worg's
// inline images (the Org view and the editor, src/inlineimages.ts). A link is
// relative to the file that names it while the file server is rooted at the
// first org directory, and only the server knows both (see Traps: media
// paths), so the page asks once per file for every picture in it.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

type mediaResolveReq struct {
	File    string
	Targets []string
}

type mediaResolved struct {
	Url string // a path under /images, or the link itself when it is a url
	Msg string // why there is none
}

/* SDOC: API
* POST /media/resolve — Where an Org File's Pictures Can Be Fetched
	Body ={File, Targets: [link, ...]}=: an org file and the targets of the
	picture links in it, as written (=file:x.png=, =./img/y.jpg=, a url).

	*Response:* ={Ok, Urls: {target: {Url, Msg}}}=. =Url= is a path under
	=/images= for a file inside the first org directory, or the link itself
	when it is already a url; otherwise =Msg= says why there is none (missing,
	or outside the folder the server shares).
EDOC */
func PostMediaResolve(w http.ResponseWriter, r *http.Request) {
	var req mediaResolveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if _, ok := insideOrgDirs(req.File); !ok {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "not a file in the org directories"})
		return
	}
	out := map[string]mediaResolved{}
	for i, t := range req.Targets {
		if i >= 500 {
			break
		}
		out[t] = resolveOne(t, req.File)
	}
	writingJson(w, map[string]any{"Ok": true, "Urls": out})
}

func resolveOne(target, fromFile string) mediaResolved {
	t := strings.TrimSpace(target)
	t = strings.TrimPrefix(t, "file://")
	t = strings.TrimPrefix(t, "file:")
	if i := strings.Index(t, "::"); i >= 0 {
		t = t[:i]
	}
	if strings.Contains(t, "://") || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "data:") {
		return mediaResolved{Url: t}
	}
	if strings.HasPrefix(t, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			t = filepath.Join(home, t[2:])
		}
	}
	abs := t
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(filepath.Dir(fromFile), t)
	}
	found := false
	if _, err := os.Stat(abs); err == nil {
		found = true
	}
	u := mediaURL(t, fromFile)
	if u == "" {
		if !found {
			return mediaResolved{Msg: "no such file: " + t}
		}
		return mediaResolved{Msg: "outside the org folder the server shares, so the browser cannot be sent it"}
	}
	if !found {
		// mediaURL also tries the org root; check where it pointed.
		rel := strings.TrimPrefix(u, "/images/")
		dirs := Conf().Server.OrgDirs
		if len(dirs) > 0 {
			root, _ := filepath.Abs(dirs[0])
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
				return mediaResolved{Msg: "no such file: " + t}
			}
		}
	}
	// A path, never a localhost url, so the page works from any host.
	parts := strings.Split(u, "/")
	for i := range parts {
		parts[i] = urlPathEscape(parts[i])
	}
	return mediaResolved{Url: strings.Join(parts, "/")}
}

func urlPathEscape(s string) string {
	r := strings.NewReplacer("%", "%25", " ", "%20", "#", "%23", "?", "%3F")
	return r.Replace(s)
}
