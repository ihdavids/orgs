package orgs

/* SDOC: API
* org-protocol: captures, links and sources from a browser
  The server's half of Emacs's org-protocol (the parser is
  =common.ParseOrgProtocol=, shared with =orgs protocol=, the scheme's
  handler).

  - =GET /org-protocol/<link>= (public) is any org-protocol link with
    =org-protocol://= swapped for the server's address, in either spelling:
    =/org-protocol/capture?template=w&url=…= or the old
    =/org-protocol/capture:/w/URL/TITLE/BODY=. It writes nothing; it
    redirects to worg's flat form, =/?protocol=capture&template=…=, and worg
    puts up the capture (or stores the link, or opens the file) once whoever
    it is has logged in. A bookmarklet for it needs nothing installed.
  - =POST /protocol/links= ={"Url": "...", "Title": "..."}= stores a link
    (store-link) for the user, answering ={"Ok": true, "Link": "[[url][title]]"}=;
    =GET /protocol/links= lists them, newest first. Kept in memory, thirty a
    user, as org keeps its stored links for the session.
  - =GET /protocol/source?url=<published url>= is the file the
    =protocolProjects:= setting says that page was made from:
    ={"Ok", "Msg", "File", "Known"}=, =Known= when it is an org file this
    server holds (so worg can open it).
EDOC */

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// protocolFront answers /org-protocol/... before the router sees it. An old
// style link carries a whole escaped url as a path segment, and the router
// cleans the *decoded* path: `https://` collapses to `https:/` and the request
// is redirected somewhere that no longer parses. So it never gets that far.
func protocolFront(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/org-protocol/"
		raw := r.URL.EscapedPath()
		if !strings.HasPrefix(raw, prefix) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "org-protocol: GET only", http.StatusMethodNotAllowed)
			return
		}
		link := "org-protocol://" + strings.TrimPrefix(raw, prefix)
		if r.URL.RawQuery != "" {
			link += "?" + r.URL.RawQuery
		}
		p, err := common.ParseOrgProtocol(link)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `<!DOCTYPE html><html><body style="font-family: sans-serif; padding: 2em">
<h3>That org-protocol link does not read</h3><p>%s</p>
<p>The forms this server takes are <code>/org-protocol/capture?template=…&amp;url=…&amp;title=…&amp;body=…</code>,
<code>/org-protocol/store-link?url=…&amp;title=…</code> and <code>/org-protocol/open-source?url=…</code>,
or the old <code>/org-protocol/capture:/TEMPLATE/URL/TITLE/BODY</code>.</p></body></html>`, htmlEscape(err.Error()))
			return
		}
		http.Redirect(w, r, p.WorgPath(), http.StatusSeeOther)
	})
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// ── store-link ────────────────────────────────────────────────────────────

const storedLinksKept = 30

var storedLinks = struct {
	sync.Mutex
	m map[string][]common.StoredLink
}{m: map[string][]common.StoredLink{}}

func PostProtocolLink(w http.ResponseWriter, r *http.Request) {
	var req common.StoredLink
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Url) == "" {
		protocolJson(w, common.StoredLinkResult{Ok: false, Msg: "store-link wants {\"Url\": ..., \"Title\": ...}"})
		return
	}
	req.Url = strings.TrimSpace(req.Url)
	req.Title = strings.TrimSpace(req.Title)
	req.Link = common.OrgLinkText(req.Url, req.Title)
	req.At = time.Now()
	user := GetUsername(r)
	storedLinks.Lock()
	// The same page stored again moves to the top rather than being listed
	// twice.
	list := []common.StoredLink{req}
	for _, l := range storedLinks.m[user] {
		if l.Url != req.Url && len(list) < storedLinksKept {
			list = append(list, l)
		}
	}
	storedLinks.m[user] = list
	storedLinks.Unlock()
	protocolJson(w, common.StoredLinkResult{Ok: true, Link: req.Link})
}

func GetProtocolLinks(w http.ResponseWriter, r *http.Request) {
	storedLinks.Lock()
	list := append([]common.StoredLink{}, storedLinks.m[GetUsername(r)]...)
	storedLinks.Unlock()
	protocolJson(w, list)
}

// ── open-source ───────────────────────────────────────────────────────────

func GetProtocolSource(w http.ResponseWriter, r *http.Request) {
	protocolJson(w, protocolSource(r.URL.Query().Get("url")))
}

func protocolSource(link string) common.ProtocolSourceResult {
	link = strings.TrimSpace(link)
	if link == "" {
		return common.ProtocolSourceResult{Msg: "open-source needs the url of the page"}
	}
	var projects []common.ProtocolProject
	if Conf().Server != nil {
		projects = append(projects, Conf().Server.ProtocolProjects...)
	}
	if len(projects) == 0 {
		return common.ProtocolSourceResult{Msg: "no protocolProjects: in the server's yaml, so no published page has a source - see Protocol Projects in the settings"}
	}
	// The longest base wins, so a sub-site can be mapped apart from its parent.
	sort.SliceStable(projects, func(i, j int) bool { return len(projects[i].BaseUrl) > len(projects[j].BaseUrl) })
	tried := ""
	for _, pr := range projects {
		rel, ok := common.ProtocolSourceRel(pr, link)
		if !ok {
			continue
		}
		file := filepath.Join(expandHome(pr.WorkingDirectory), filepath.FromSlash(rel))
		if _, err := os.Stat(file); err != nil {
			if tried == "" {
				tried = file
			}
			continue
		}
		known := false
		if f := GetDb().FindByFile(file); f != nil && f.Filename == file {
			known = true
		}
		return common.ProtocolSourceResult{Ok: true, File: file, Known: known}
	}
	if tried != "" {
		return common.ProtocolSourceResult{Msg: "that page would come from " + tried + ", which is not there"}
	}
	return common.ProtocolSourceResult{Msg: "no protocolProjects: entry has a baseUrl that " + link + " starts with"}
}

func protocolJson(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
