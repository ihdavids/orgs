package orgs

// Link titles (features.autoLinkTitle, after Emacs's
// org-cliplink): a pasted web address becomes [[url][Page title]].
//
// The server fetches, because a browser cannot read another site's page
// (CORS) and VS Code and the command line have no page to read at all. What
// a page calls itself is internal/common/linktitle.go (pure, pinned by saved
// heads); this file is the fetch, the cache and the endpoints.
//
// The fetch happens on every paste, so it is careful:
//   - http and https only, every redirect checked the same way;
//   - no private addresses (127/8, 10/8, 172.16/12, 192.168/16, 169.254/16,
//     fc00::/7, fe80::/10, localhost): checked on the address actually dialled,
//     so a name that resolves to one is refused too, and a redirect to one;
//     autoLinkTitle.allowPrivate turns this off. No proxy from the environment,
//     which would dial for us and so skip the check;
//   - 8s (autoLinkTitle.timeout), the first 512kB only, one request at a time
//     per host with a short gap, so a file of fifty GitHub links is not fifty
//     requests at once;
//   - YouTube, Vimeo, SoundCloud, Spotify and Flickr through their oEmbed
//     endpoints, whose titles are the video's own and whose pages are huge.
//
// Titles are kept for autoLinkTitle.cacheFor (30 days) in memory and in
// <user cache dir>/orgs/link-titles.json, so a url pasted twice, or a
// retitle run again, is not fetched again; failures are kept ten minutes in
// memory only. Everything still works with no network: the paste just stays
// [[url]].

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

const (
	linkTitlePageCap   = 512 << 10
	linkTitleFailFor   = 10 * time.Minute
	linkTitleMaxBatch  = 200
	linkTitleHostGap   = 300 * time.Millisecond
	linkTitleCacheSize = 5000
)

func registerLinkTitleRoutes(api *mux.Router) {
	api.HandleFunc("/link/title", RequestLinkTitle).Methods("GET")
	api.HandleFunc("/link/titles", PostLinkTitles).Methods("POST")
	api.HandleFunc("/link/retitle", PostLinkRetitle).Methods("POST")
}

func linkTitleSettings() common.LinkTitleSettings {
	if s := Conf().Server; s != nil {
		return s.AutoLinkTitle
	}
	return common.LinkTitleSettings{}
}

func linkTitleRules(s common.LinkTitleSettings) []common.LinkTitleRule {
	rules := append([]common.LinkTitleRule(nil), s.Rules...)
	if !s.NoDefaultRules {
		rules = append(rules, common.DefaultLinkTitleRules...)
	}
	return rules
}

func linkTitleUserAgent(s common.LinkTitleSettings) string {
	if ua := strings.TrimSpace(s.UserAgent); ua != "" {
		return ua
	}
	return "Mozilla/5.0 (compatible; orgs/" + common.Version + "; +https://github.com/ihdavids/orgs)"
}

func linkTitleTimeout(s common.LinkTitleSettings) time.Duration {
	if d, err := common.ParseLongDuration(s.Timeout); err == nil && d > 0 {
		return d
	}
	return 8 * time.Second
}

func linkTitleCacheFor(s common.LinkTitleSettings) time.Duration {
	if d, err := common.ParseLongDuration(s.CacheFor); err == nil && d > 0 {
		return d
	}
	return 30 * 24 * time.Hour
}

// ---------------------------------------------------------------------------
// The safe fetch
// ---------------------------------------------------------------------------

var linkTitleErrPrivate = errors.New("a private address")

var linkTitlePrivateNets = func() []*net.IPNet {
	out := []*net.IPNet{}
	for _, c := range []string{"0.0.0.0/8", "127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"169.254.0.0/16", "100.64.0.0/10", "::1/128", "::/128", "fc00::/7", "fe80::/10"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

func linkTitlePrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, n := range linkTitlePrivateNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// linkTitlePrivateHost is a host named as a private one before anything is dialled,
// for a clearer refusal than the dialler's.
func linkTitlePrivateHost(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") {
		return true
	}
	if ip := net.ParseIP(strings.Trim(h, "[]")); ip != nil {
		return linkTitlePrivateIP(ip)
	}
	return false
}

// linkTitleAllowPrivate is read on every dial, so a yaml change applies at
// once; tests set it too.
var linkTitleAllowPrivate = func() bool { return linkTitleSettings().AllowPrivate }

var linkTitleTransport = &http.Transport{
	Proxy: nil,
	DialContext: (&net.Dialer{
		Timeout: 6 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			if linkTitleAllowPrivate() {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if linkTitlePrivateIP(net.ParseIP(host)) {
				return linkTitleErrPrivate
			}
			return nil
		},
	}).DialContext,
	TLSHandshakeTimeout:   6 * time.Second,
	ResponseHeaderTimeout: 8 * time.Second,
	MaxIdleConns:          16,
	IdleConnTimeout:       60 * time.Second,
}

var linkTitleClient = &http.Client{
	Transport: linkTitleTransport,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 8 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("redirected to a %s url", req.URL.Scheme)
		}
		if !linkTitleAllowPrivate() && linkTitlePrivateHost(req.URL.Hostname()) {
			return fmt.Errorf("redirected to %s, %w", req.URL.Hostname(), linkTitleErrPrivate)
		}
		return nil
	},
}

// One request at a time to a host, a short gap between them.
var linkTitleHosts = struct {
	sync.Mutex
	locks map[string]*linkTitleHostTurn
}{locks: map[string]*linkTitleHostTurn{}}

type linkTitleHostTurn struct {
	sync.Mutex
	last time.Time
}

func linkTitleTurn(host string) func() {
	linkTitleHosts.Lock()
	t := linkTitleHosts.locks[host]
	if t == nil {
		t = &linkTitleHostTurn{}
		linkTitleHosts.locks[host] = t
	}
	linkTitleHosts.Unlock()
	t.Lock()
	if wait := linkTitleHostGap - time.Since(t.last); wait > 0 {
		time.Sleep(wait)
	}
	return func() { t.last = time.Now(); t.Unlock() }
}

func linkTitleWhy(host string, err error) string {
	if errors.Is(err, linkTitleErrPrivate) {
		return fmt.Sprintf("%s is a private address: autoLinkTitle.allowPrivate lets orgs fetch it", host)
	}
	var dns *net.DNSError
	if errors.As(err, &dns) && dns.IsNotFound {
		return fmt.Sprintf("there is no site called %s", host)
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue.Timeout() {
		return fmt.Sprintf("%s did not answer in time", host)
	}
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return fmt.Sprintf("could not read %s: %v", host, err)
}

// linkTitleGet fetches a url, the first cap bytes of it.
func linkTitleGet(s common.LinkTitleSettings, u, accept string, cap int64) ([]byte, string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), linkTitleTimeout(s))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", "", err
	}
	req.Header.Set("User-Agent", linkTitleUserAgent(s))
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "en;q=0.9,*;q=0.5")
	resp, err := linkTitleClient.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	final := resp.Request.URL.String()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, final, "", fmt.Errorf("%s answered %s", resp.Request.URL.Host, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, cap))
	if err != nil && len(b) == 0 {
		return nil, final, "", err
	}
	return b, final, resp.Header.Get("Content-Type"), nil
}

// ---------------------------------------------------------------------------
// oEmbed
// ---------------------------------------------------------------------------

var linkTitleDefaultOembed = []common.LinkTitleOembed{
	{Host: "youtube.com", Url: "https://www.youtube.com/oembed?format=json&url="},
	{Host: "youtu.be", Url: "https://www.youtube.com/oembed?format=json&url="},
	{Host: "vimeo.com", Url: "https://vimeo.com/api/oembed.json?url="},
	{Host: "soundcloud.com", Url: "https://soundcloud.com/oembed?format=json&url="},
	{Host: "open.spotify.com", Url: "https://open.spotify.com/oembed?url="},
	{Host: "flickr.com", Url: "https://www.flickr.com/services/oembed/?format=json&url="},
	{Host: "flic.kr", Url: "https://www.flickr.com/services/oembed/?format=json&url="},
}

func linkTitleOembedFor(s common.LinkTitleSettings, host string) string {
	if s.NoOembed {
		return ""
	}
	for _, p := range append(append([]common.LinkTitleOembed(nil), s.Oembed...), linkTitleDefaultOembed...) {
		if common.HostMatches(host, p.Host) && strings.TrimSpace(p.Url) != "" {
			return p.Url
		}
	}
	return ""
}

type linkTitleOembedAnswer struct {
	Title        string `json:"title"`
	AuthorName   string `json:"author_name"`
	ProviderName string `json:"provider_name"`
}

func linkTitleOembedGet(s common.LinkTitleSettings, endpoint string) (linkTitleOembedAnswer, error) {
	var a linkTitleOembedAnswer
	b, _, _, err := linkTitleGet(s, endpoint, "application/json", 64<<10)
	if err != nil {
		return a, err
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return a, fmt.Errorf("an oEmbed answer that is not json")
	}
	a.Title = strings.Join(strings.Fields(a.Title), " ")
	return a, nil
}

// ---------------------------------------------------------------------------
// The cache
// ---------------------------------------------------------------------------

type linkTitleEntry struct {
	T  common.LinkTitle
	At time.Time
}

var linkTitleCache = struct {
	sync.Mutex
	loaded  bool
	entries map[string]linkTitleEntry
	fails   map[string]linkTitleEntry
	save    *time.Timer
}{entries: map[string]linkTitleEntry{}, fails: map[string]linkTitleEntry{}}

// linkTitleCacheFile is where titles outlive the server; "" keeps them in
// memory only (tests).
var linkTitleCacheFile = func() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "orgs", "link-titles.json")
}

// Called with the lock held.
func linkTitleLoadCache() {
	if linkTitleCache.loaded {
		return
	}
	linkTitleCache.loaded = true
	p := linkTitleCacheFile()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var m map[string]linkTitleEntry
	if json.Unmarshal(b, &m) == nil {
		for k, v := range m {
			linkTitleCache.entries[k] = v
		}
	}
}

// Called with the lock held: written a moment later, once for a batch.
func linkTitleSaveSoon() {
	p := linkTitleCacheFile()
	if p == "" || linkTitleCache.save != nil {
		return
	}
	linkTitleCache.save = time.AfterFunc(2*time.Second, func() {
		linkTitleCache.Lock()
		linkTitleCache.save = nil
		keep := linkTitleCacheFor(linkTitleSettings())
		type kv struct {
			k string
			e linkTitleEntry
		}
		all := []kv{}
		for k, e := range linkTitleCache.entries {
			if time.Since(e.At) > keep {
				delete(linkTitleCache.entries, k)
				continue
			}
			all = append(all, kv{k, e})
		}
		if len(all) > linkTitleCacheSize {
			sort.Slice(all, func(i, j int) bool { return all[i].e.At.After(all[j].e.At) })
			for _, x := range all[linkTitleCacheSize:] {
				delete(linkTitleCache.entries, x.k)
			}
		}
		b, err := json.Marshal(linkTitleCache.entries)
		linkTitleCache.Unlock()
		if err != nil {
			return
		}
		if err := os.MkdirAll(filepath.Dir(p), 0755); err == nil {
			tmp := p + ".tmp"
			if os.WriteFile(tmp, b, 0644) == nil {
				os.Rename(tmp, p)
			}
		}
	})
}

func linkTitleCached(key string, s common.LinkTitleSettings) (common.LinkTitle, bool) {
	linkTitleCache.Lock()
	defer linkTitleCache.Unlock()
	linkTitleLoadCache()
	if e, ok := linkTitleCache.entries[key]; ok && time.Since(e.At) < linkTitleCacheFor(s) {
		e.T.Cached = true
		return e.T, true
	}
	if e, ok := linkTitleCache.fails[key]; ok && time.Since(e.At) < linkTitleFailFor {
		e.T.Cached = true
		return e.T, true
	}
	return common.LinkTitle{}, false
}

func linkTitleRemember(key string, t common.LinkTitle) {
	linkTitleCache.Lock()
	defer linkTitleCache.Unlock()
	linkTitleLoadCache()
	e := linkTitleEntry{T: t, At: time.Now()}
	if t.Ok {
		linkTitleCache.entries[key] = e
		delete(linkTitleCache.fails, key)
		linkTitleSaveSoon()
	} else {
		linkTitleCache.fails[key] = e
	}
}

// ---------------------------------------------------------------------------
// One title
// ---------------------------------------------------------------------------

func linkTitleURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("no url")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%q is not a url", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http and https pages have titles to fetch, not %s", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%q has no host", raw)
	}
	return u, nil
}

// LinkTitleOf is a url's title, from the cache unless refresh.
func LinkTitleOf(raw string, refresh bool) common.LinkTitle {
	s := linkTitleSettings()
	res := common.LinkTitle{Url: strings.TrimSpace(raw)}
	u, err := linkTitleURL(raw)
	if err != nil {
		res.Msg = err.Error()
		res.Link = common.OrgLinkText(res.Url, "")
		return res
	}
	res.Url = u.String()
	res.Link = common.OrgLinkText(res.Url, "")
	host := u.Hostname()
	for _, ig := range s.Ignore {
		if common.HostMatches(host, ig) {
			res.Ignored = true
			res.Msg = host + " is in autoLinkTitle.ignore"
			return res
		}
	}
	if !linkTitleAllowPrivate() && linkTitlePrivateHost(host) {
		res.Msg = linkTitleWhy(host, linkTitleErrPrivate)
		return res
	}
	if !refresh {
		if c, ok := linkTitleCached(res.Url, s); ok {
			return c
		}
	}
	res = linkTitleFetch(s, u, res)
	linkTitleRemember(res.Url, res)
	return res
}

func linkTitleFetch(s common.LinkTitleSettings, u *url.URL, res common.LinkTitle) common.LinkTitle {
	host := u.Hostname()
	done := linkTitleTurn(strings.ToLower(host))
	defer done()
	rules := linkTitleRules(s)
	finish := func(title, final string) common.LinkTitle {
		res.Raw = title
		t := title
		if fh := linkTitleHostOf(final); fh != "" && !strings.EqualFold(fh, host) {
			t = common.CleanLinkTitle(fh, t, res.Site, rules, s.KeepSiteName)
		}
		t = common.CleanLinkTitle(host, t, res.Site, rules, s.KeepSiteName)
		res.Title = t
		if res.Raw == res.Title {
			res.Raw = ""
		}
		res.Ok = t != ""
		if !res.Ok && res.Msg == "" {
			res.Msg = "the page has no title"
		}
		res.Link = common.OrgLinkText(res.Url, t)
		return res
	}
	if ep := linkTitleOembedFor(s, host); ep != "" {
		if a, err := linkTitleOembedGet(s, ep+url.QueryEscape(res.Url)); err == nil && a.Title != "" {
			res.From, res.Site = "oembed", a.ProviderName
			if a.AuthorName != "" {
				res.Description = "by " + a.AuthorName
			}
			res.Icon = u.Scheme + "://" + u.Host + "/favicon.ico"
			return finish(a.Title, "")
		}
		// No answer from the endpoint: the page itself.
	}
	b, final, ct, err := linkTitleGet(s, res.Url, "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5", linkTitlePageCap)
	if final != "" && final != res.Url {
		res.Final = final
	}
	if err != nil {
		if strings.Contains(err.Error(), " answered ") {
			res.Msg = err.Error()
		} else {
			res.Msg = linkTitleWhy(host, err)
		}
		return res
	}
	mt, _, _ := mime.ParseMediaType(ct)
	if mt == "" {
		mt = strings.SplitN(http.DetectContentType(b), ";", 2)[0]
	}
	if mt != "text/html" && mt != "application/xhtml+xml" {
		res.Msg = fmt.Sprintf("not a web page (%s)", mt)
		return res
	}
	base, _ := url.Parse(final)
	if base == nil {
		base = u
	}
	m := common.ParsePageMeta(common.DecodePage(b, ct), base)
	res.From, res.Site, res.Description, res.Icon = "page", m.Site, m.Description, m.Icon
	title := m.BestTitle()
	if title == "" && m.Oembed != "" && !s.NoOembed {
		if p, err := url.Parse(m.Oembed); err == nil && (p.Scheme == "https" || p.Scheme == "http") {
			if a, err := linkTitleOembedGet(s, m.Oembed); err == nil && a.Title != "" {
				res.From, title = "oembed", a.Title
				if res.Site == "" {
					res.Site = a.ProviderName
				}
			}
		}
	}
	return finish(title, final)
}

func linkTitleHostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Hostname()
	}
	return ""
}

// LinkTitlesOf is many titles, four fetched at a time (one per host).
func LinkTitlesOf(urls []string, refresh bool) []common.LinkTitle {
	out := make([]common.LinkTitle, len(urls))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	seen := map[string][]int{}
	order := []string{}
	for i, u := range urls {
		k := strings.TrimSpace(u)
		if _, ok := seen[k]; !ok {
			order = append(order, k)
		}
		seen[k] = append(seen[k], i)
	}
	for _, k := range order {
		wg.Add(1)
		sem <- struct{}{}
		go func(k string) {
			defer wg.Done()
			defer func() { <-sem }()
			t := LinkTitleOf(k, refresh)
			for _, i := range seen[k] {
				out[i] = t
			}
		}(k)
	}
	wg.Wait()
	return out
}

// ---------------------------------------------------------------------------
// Bare links in a file
// ---------------------------------------------------------------------------

// linkTitleRetitle is POST /link/retitle (see common.RetitleRequest).
func linkTitleRetitle(req common.RetitleRequest) common.RetitleAnswer {
	res := common.RetitleAnswer{Links: []common.RetitleLink{}}
	var lines []string
	var path, raw string
	switch {
	case req.File != "":
		f := GetDb().FindByFile(req.File)
		if f == nil {
			res.Msg = fmt.Sprintf("no org file called %q", req.File)
			return res
		}
		path = f.Filename
		b, err := os.ReadFile(path)
		if err != nil {
			res.Msg = err.Error()
			return res
		}
		raw = string(b)
		res.File, res.Hash = path, shortHash(raw)
		lines = splitOrgText(raw)
		if req.Apply && req.Hash != res.Hash {
			res.Stale = true
			res.Msg = displayPath(path) + " changed since the preview - preview again"
			return res
		}
	case req.Apply:
		res.Msg = "an apply needs the File it writes"
		return res
	default:
		raw = req.Text
		lines = strings.Split(raw, "\n")
	}
	spots := common.TitleableLinks(lines)
	titles := req.Titles
	why := map[string]string{}
	if titles == nil {
		titles = map[string]string{}
		urls := []string{}
		for _, s := range spots {
			urls = append(urls, s.Url)
		}
		for i, t := range LinkTitlesOf(urls, req.Refresh) {
			if t.Ok {
				titles[urls[i]] = t.Title
			} else {
				why[urls[i]] = t.Msg
			}
		}
	}
	out, n := common.RetitleLines(lines, titles)
	for _, s := range spots {
		s.Title = strings.TrimSpace(titles[s.Url])
		s.Ok = s.Title != ""
		if !s.Ok {
			s.Msg = why[s.Url]
			if s.Msg == "" {
				s.Msg = "no title"
			}
		}
		res.Links = append(res.Links, s)
	}
	res.Ok, res.Changed = true, n
	name := "text"
	if path != "" {
		name = displayPath(path)
	}
	if n > 0 {
		res.Patch, res.Added, res.Removed = unifiedPatch(name, lines, out, false)
	}
	if path == "" {
		res.Text = strings.Join(out, "\n")
		return res
	}
	res.Text = strings.Join(out, "\n") + "\n"
	if req.Apply && n > 0 {
		if err := writeLines(path, out); err != nil {
			res.Ok, res.Msg = false, err.Error()
			return res
		}
		res.Applied = true
		res.Hash = shortHash(res.Text)
		res.Text = ""
	}
	return res
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /link/title?url=URL
	The page's title for a web address, as =[[url][title]]=, for
	=features: autoLinkTitle:= (org-cliplink).
	=refresh=1= skips the cache.

	#+BEGIN_SRC json
	{"Ok": true, "Url": "https://youtu.be/x", "Title": "Never Gonna Give You Up",
	 "Raw": "Never Gonna Give You Up - YouTube", "Site": "YouTube",
	 "Description": "by Rick Astley", "Icon": "https://youtu.be/favicon.ico",
	 "Link": "[[https://youtu.be/x][Never Gonna Give You Up]]", "From": "oembed"}
	#+END_SRC

	A url it could not read answers =Ok: false= with =Msg= and =Link=
	=[[url]]=; a host in =autoLinkTitle.ignore= adds =Ignored: true=.
EDOC */
func RequestLinkTitle(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "autoLinkTitle") {
		return
	}
	q := r.URL.Query()
	dynJson(w, LinkTitleOf(q.Get("url"), q.Get("refresh") == "1" || q.Get("refresh") == "true"))
}

/* SDOC: API
* POST /link/titles
	Many titles at once: ={"Urls": [...], "Refresh": false}= answers
	={"Ok": true, "Titles": [...]}=, one =GET /link/title= answer per url in
	the same order. At most 200 urls; four are fetched at a time, one per site.
EDOC */
func PostLinkTitles(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "autoLinkTitle") {
		return
	}
	var req common.LinkTitlesRequest
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.LinkTitlesAnswer{Msg: "that is not a list of urls: " + err.Error(), Titles: []common.LinkTitle{}})
		return
	}
	if len(req.Urls) > linkTitleMaxBatch {
		dynJson(w, common.LinkTitlesAnswer{Msg: fmt.Sprintf("at most %d urls at once", linkTitleMaxBatch), Titles: []common.LinkTitle{}})
		return
	}
	dynJson(w, common.LinkTitlesAnswer{Ok: true, Titles: LinkTitlesOf(req.Urls, req.Refresh)})
}

/* SDOC: API
* POST /link/retitle
	Gives the bare urls and =[[url]]= links of a file their pages' titles,
	as =[[url][title]]=. A preview first, as the refactors are:

	#+BEGIN_SRC json
	{"File": "notes.org"}
	#+END_SRC

	answers the places it found (=Links=: =Url=, =Line=, =Kind= bare or link,
	=Title=, =Ok=, =Msg=), the text after, a unified =Patch= and the file's
	=Hash=. Nothing is written. To write, send it back with =Apply= and the
	titles to use (drop or change any):

	#+BEGIN_SRC json
	{"File": "notes.org", "Apply": true, "Hash": "…", "Titles": {"https://…": "A title"}}
	#+END_SRC

	refused with =Stale: true= if the file changed since. Only the lines
	holding a link change. Left alone: urls in source, example, export and
	comment blocks, property drawers, keyword lines, fixed-width lines,
	tables, other links and =verbatim=. ={"Text": "..."}= instead of =File=
	answers the new =Text= and writes nothing (an editor's buffer).
EDOC */
func PostLinkRetitle(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "autoLinkTitle") {
		return
	}
	var req common.RetitleRequest
	body, _ := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.RetitleAnswer{Msg: err.Error(), Links: []common.RetitleLink{}})
		return
	}
	dynJson(w, linkTitleRetitle(req))
}
