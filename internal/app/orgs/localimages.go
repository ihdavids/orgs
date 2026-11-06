package orgs

// Keeping web pictures here (features.localImages, after Emacs's
// org-download).
//
// A note clipped from the web links its pictures where they were found:
// `[[https://site/x.png]]`, `[[https://site/x.png][a picture]]`, or the url on
// a line of its own. Those break when the site moves, and show nothing on a
// plane. This downloads each one beside the notes and points the link at the
// copy, the way org-download writes one:
//
//	#+DOWNLOADED: https://site/x.png @ 2026-11-05 10:20:33
//	#+CAPTION: kept, as is every #+ATTR_ line
//	[[file:images/x.png]]
//
// The `#+DOWNLOADED:` line is org-download's own record of where a picture
// came from (org-download-annotate-default). It goes above the link's
// affiliated keywords (#+CAPTION, #+NAME, #+ATTR_*), never between them and
// the link, which would part the caption from its picture. A link inside a
// paragraph gets no such line - a keyword line there would end the paragraph.
//
// Rules:
//   - Line edits, bottom-up: only the lines holding a link change, and the
//     annotation is inserted above, so every line still to be edited keeps its
//     index. The file is read again after the downloads (which take seconds)
//     and the edit worked out on what is there then.
//   - The bytes decide: a picture is kept only when its first bytes say it is
//     one (a png, a jpeg ...), whatever the url or the Content-Type claim, and
//     its extension is the one the bytes call for.
//   - The same bytes are the same file: a picture already in the folder,
//     under any name, is linked rather than written again, as contact photos
//     are.
//   - Outside service manners: http and https only, private and loopback
//     addresses refused at connect time (so a name that resolves to one is
//     refused too), timeouts, a size cap per picture and per run, a User-Agent
//     saying who is asking, and `localImages.ignore` hosts left alone.

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

func registerLocalImagesRoutes(api *mux.Router) {
	api.HandleFunc("/localimages/scan", RequestLocalImagesScan).Methods("GET")
	api.HandleFunc("/localimages/fetch", PostLocalImagesFetch).Methods("POST")
	api.HandleFunc("/localimages/download", PostLocalImagesDownload).Methods("POST")
	api.HandleFunc("/localimages/orphans", PostLocalImagesOrphans).Methods("POST")
	api.HandleFunc("/localimages/settings", RequestLocalImagesSettings).Methods("GET", "POST")
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func localImagesSettings() common.LocalImagesSettings {
	if s := Conf().Server; s != nil {
		return s.LocalImages
	}
	return common.LocalImagesSettings{}
}

func localImagesMaxImage() int64 {
	if n := localImagesSettings().MaxImageMB; n > 0 {
		return int64(n) << 20
	}
	return 20 << 20
}

func localImagesMaxRun() int64 {
	if n := localImagesSettings().MaxRunMB; n > 0 {
		return int64(n) << 20
	}
	return 200 << 20
}

func localImagesAnnotate() bool {
	a := localImagesSettings().Annotate
	return a == nil || *a
}

// ---------------------------------------------------------------------------
// Finding the pictures (pure: lines in, links out)
// ---------------------------------------------------------------------------

// LocalImageRemote is one picture on the web a file links.
type LocalImageRemote struct {
	Url string
	// The line it is on, counted from 1.
	Line int
	// Its description, for `[[url][desc]]`.
	Desc string `json:",omitempty"`
	// A url written on a line of its own, not in brackets.
	Bare bool `json:",omitempty"`
	// The link is all there is on its line, so it gets a #+DOWNLOADED: line.
	Alone bool `json:",omitempty"`
	// The url's extension says picture. False: the url has none, and the
	// server was asked (Type).
	Sure bool
	// What the server said it is, and how big, when asked.
	Type string `json:",omitempty"`
	Size int64  `json:",omitempty"`
	// Why it will not be fetched: an ignored host, a private address, not a
	// picture after all.
	Skip string `json:",omitempty"`

	start, end int // byte span of the link (or bare url) on its line
}

var (
	localImagesLinkRe = regexp.MustCompile(`\[\[(https?://[^\]\s]+)\](?:\[([^\]]*)\])?\]`)
	localImagesBareRe = regexp.MustCompile(`^(\s*)(https?://\S+?)\s*$`)
	// A block whose lines are not prose: a url in one is an example.
	localImagesBlockRe   = regexp.MustCompile(`(?i)^\s*#\+begin_(src|example|export|comment)\b`)
	localImagesEndRe     = regexp.MustCompile(`(?i)^\s*#\+end_(src|example|export|comment)\b`)
	localImagesDrawerRe  = regexp.MustCompile(`^\s*:[\w-]+:`)
	localImagesHeadingRe = regexp.MustCompile(`^\*+\s`)
	// Org's affiliated keywords: they belong to the element below them.
	localImagesAffRe = regexp.MustCompile(`(?i)^\s*#\+(caption|name|attr_\w+|header|label|plot|results|tblname|srcname|data)(\[[^\]]*\])?:`)
)

var localImagesRasterExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".avif": true, ".bmp": true}

// localImagesImageExt is whether a file or url extension names a picture this keeps.
func localImagesImageExt(ext string, svg bool) bool {
	ext = strings.ToLower(ext)
	return localImagesRasterExts[ext] || (svg && ext == ".svg")
}

// localImagesUrlKind says what a url's path says it is: a picture ("img"), nothing
// at all ("" - no extension, worth asking) or something else ("other").
func localImagesUrlKind(raw string, svg bool) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "other"
	}
	ext := path.Ext(u.Path)
	switch {
	case localImagesImageExt(ext, svg):
		return "img"
	case ext == "" || strings.ContainsAny(ext, "-_"):
		return ""
	}
	return "other"
}

// localImagesScanLines finds the web pictures in a file's lines: bracket links
// whose target is a picture by its extension, description-less links and
// bare urls with no extension at all (Sure false: the server has to be
// asked), never inside a source, example or export block, a drawer, a
// heading or a keyword line.
func localImagesScanLines(lines []string, svg bool) []LocalImageRemote {
	out := []LocalImageRemote{}
	inBlock := false
	for i, l := range lines {
		if inBlock {
			if localImagesEndRe.MatchString(l) {
				inBlock = false
			}
			continue
		}
		if localImagesBlockRe.MatchString(l) {
			inBlock = true
			continue
		}
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") || localImagesDrawerRe.MatchString(l) || localImagesHeadingRe.MatchString(l) {
			continue
		}
		if m := localImagesBareRe.FindStringSubmatchIndex(l); m != nil && !strings.Contains(l, "[[") {
			u := l[m[4]:m[5]]
			if k := localImagesUrlKind(u, svg); k != "other" {
				out = append(out, LocalImageRemote{Url: u, Line: i + 1, Bare: true, Alone: true, Sure: k == "img", start: m[4], end: m[5]})
			}
			continue
		}
		ms := localImagesLinkRe.FindAllStringSubmatchIndex(l, -1)
		for _, m := range ms {
			u := l[m[2]:m[3]]
			desc := ""
			if m[4] >= 0 {
				desc = l[m[4]:m[5]]
			}
			k := localImagesUrlKind(u, svg)
			if k == "other" || (k == "" && desc != "") {
				continue
			}
			alone := len(ms) == 1 && strings.TrimSpace(l) == l[m[0]:m[1]]
			out = append(out, LocalImageRemote{Url: u, Line: i + 1, Desc: desc, Alone: alone, Sure: k == "img", start: m[0], end: m[1]})
		}
	}
	return out
}

// localImagesStamp is the time the way org-download writes it.
func localImagesStamp(t time.Time) string { return t.Format("2006-01-02 15:04:05") }

// localImagesAnnotation is org-download's line for a picture from url.
func localImagesAnnotation(indent, u, stamp string) string {
	return indent + "#+DOWNLOADED: " + u + " @ " + stamp
}

// localImagesRewrite points every link to a picture in got (url -> the link to
// write, relative to the file) at its copy, bottom-up, and writes a
// #+DOWNLOADED: line above each that is alone on its line. It answers the new
// lines and how many links changed. The lines are scanned afresh, so the
// edit fits whatever the file says now.
func localImagesRewrite(lines []string, got map[string]string, stamp string, annotate bool) ([]string, int) {
	imgs := localImagesScanLines(lines, true)
	byLine := map[int][]LocalImageRemote{}
	for _, im := range imgs {
		if _, ok := got[im.Url]; ok {
			byLine[im.Line-1] = append(byLine[im.Line-1], im)
		}
	}
	rows := make([]int, 0, len(byLine))
	for r := range byLine {
		rows = append(rows, r)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(rows)))
	out := append([]string{}, lines...)
	n := 0
	for _, r := range rows {
		list := byLine[r]
		l := out[r]
		for j := len(list) - 1; j >= 0; j-- {
			im := list[j]
			link := "[[file:" + got[im.Url] + "]]"
			if im.Desc != "" {
				link = "[[file:" + got[im.Url] + "][" + im.Desc + "]]"
			}
			l = l[:im.start] + link + l[im.end:]
			n++
		}
		out[r] = l
		if !annotate || len(list) != 1 || !list[0].Alone {
			continue
		}
		// Above the caption and attributes, which belong to the link.
		at := r
		for at > 0 && localImagesAffRe.MatchString(out[at-1]) {
			at--
		}
		indent := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		if at > 0 && strings.HasPrefix(strings.TrimSpace(out[at-1]), "#+DOWNLOADED: "+list[0].Url+" ") {
			continue
		}
		out = append(out[:at], append([]string{localImagesAnnotation(indent, list[0].Url, stamp)}, out[at:]...)...)
	}
	return out, n
}

// ---------------------------------------------------------------------------
// Fetching, carefully
// ---------------------------------------------------------------------------

var localImagesUserAgent = "orgs/" + common.Version + " (+https://github.com/ihdavids/orgs; localImages)"

// localImagesBlockedIP is an address a server should not be made to fetch from on a
// note's say-so: this machine, the local network, link-local (cloud metadata
// lives there), carrier NAT.
func localImagesBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return cgnat.Contains(ip)
}

// localImagesDialControl refuses a private address at connect time: after the name
// has been resolved, so a public name pointing at 127.0.0.1 is refused too.
func localImagesDialControl(network, address string, c syscall.RawConn) error {
	if localImagesSettings().AllowPrivate {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if localImagesBlockedIP(net.ParseIP(host)) {
		return fmt.Errorf("%s is a private address (localImages.allowPrivate lets it be fetched)", host)
	}
	return nil
}

var localImagesClientOnce sync.Once
var localImagesClientV *http.Client

// localImagesClient is made on first use: built at package init, its dial check's
// reach into Conf() would be an initialisation cycle.
func localImagesClient() *http.Client {
	localImagesClientOnce.Do(func() { localImagesClientV = newLiClient() })
	return localImagesClientV
}

func newLiClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			// No proxy: the private-address check is on the address dialled.
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
				Control: localImagesDialControl,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			MaxIdleConnsPerHost:   2,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirected to a %s url", req.URL.Scheme)
			}
			return nil
		},
	}
}

// localImagesRefusal is why a url will not be asked for at all, or "".
func localImagesRefusal(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "not a url"
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "only http and https pictures are fetched"
	}
	if u.Host == "" {
		return "not a url"
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range localImagesSettings().Ignore {
		h = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(h), "."))
		if h != "" && (host == h || strings.HasSuffix(host, "."+h)) {
			return "an ignored host (localImages.ignore)"
		}
	}
	if !localImagesSettings().AllowPrivate {
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return "a private address (localImages.allowPrivate)"
		}
		if ip := net.ParseIP(host); ip != nil && localImagesBlockedIP(ip) {
			return "a private address (localImages.allowPrivate)"
		}
	}
	return ""
}

// localImagesPictureExt is what the bytes are, as an extension, or "" when they are
// not a picture this keeps. The bytes, not the url or the Content-Type.
func localImagesPictureExt(b []byte, svg bool) string {
	if e := pictureExt(b); e != "" {
		return e
	}
	switch {
	case len(b) > 12 && string(b[4:8]) == "ftyp" && (string(b[8:12]) == "avif" || string(b[8:12]) == "avis"):
		return ".avif"
	case len(b) > 14 && b[0] == 'B' && b[1] == 'M':
		return ".bmp"
	}
	if svg {
		head := b
		if len(head) > 2048 {
			head = head[:2048]
		}
		h := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(string(head), "\ufeff")))
		if (strings.HasPrefix(h, "<svg") || strings.HasPrefix(h, "<?xml") || strings.HasPrefix(h, "<!--")) && strings.Contains(h, "<svg") {
			return ".svg"
		}
	}
	return ""
}

type localImagesProbeResult struct {
	Type string
	Size int64
	Err  string
	When time.Time
}

var localImagesProbes = struct {
	sync.Mutex
	m map[string]localImagesProbeResult
}{m: map[string]localImagesProbeResult{}}

// localImagesProbe asks a server what a url is without downloading it: a HEAD, then
// the first bytes when HEAD is refused. Remembered for an hour.
func localImagesProbe(raw string) localImagesProbeResult {
	localImagesProbes.Lock()
	if p, ok := localImagesProbes.m[raw]; ok && time.Since(p.When) < time.Hour {
		localImagesProbes.Unlock()
		return p
	}
	localImagesProbes.Unlock()
	res := localImagesProbeResult{When: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ask := func(method string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, raw, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", localImagesUserAgent)
		req.Header.Set("Accept", "image/avif,image/webp,image/png,image/*;q=0.8,*/*;q=0.5")
		if method == http.MethodGet {
			req.Header.Set("Range", "bytes=0-1023")
		}
		return localImagesClient().Do(req)
	}
	resp, err := ask(http.MethodHead)
	if err == nil && (resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotImplemented) {
		resp.Body.Close()
		resp, err = ask(http.MethodGet)
	} else if err == nil && resp.StatusCode < 400 {
		// A server that says "bytes" (octet-stream, or nothing) about a
		// picture is common enough: look at the first bytes.
		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		if !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "text/") && !strings.Contains(ct, "json") {
			size := resp.ContentLength
			resp.Body.Close()
			if r2, err2 := ask(http.MethodGet); err2 == nil {
				resp = r2
				if size > 0 {
					res.Size = size
				}
			}
		}
	}
	if err != nil {
		res.Err = err.Error()
	} else {
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			res.Err = resp.Status
		} else {
			res.Type = strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0])
			if resp.ContentLength > 0 && resp.Request.Method == http.MethodHead {
				res.Size = resp.ContentLength
			}
			if resp.Request.Method == http.MethodGet {
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
				if e := localImagesPictureExt(b, true); e != "" {
					res.Type = "image/" + strings.TrimPrefix(strings.Replace(e, "jpg", "jpeg", 1), ".")
					if e == ".svg" {
						res.Type = "image/svg+xml"
					}
				}
			}
		}
	}
	localImagesProbes.Lock()
	localImagesProbes.m[raw] = res
	localImagesProbes.Unlock()
	return res
}

// localImagesDownload fetches one picture, capped, and answers its bytes and the
// extension they call for.
func localImagesDownload(raw string, limit int64) ([]byte, string, error) {
	if why := localImagesRefusal(raw); why != "" {
		return nil, "", fmt.Errorf("%s", why)
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", localImagesUserAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/*;q=0.8,*/*;q=0.5")
	resp, err := localImagesClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", fmt.Errorf("%s answered %s", resp.Request.URL.Host, resp.Status)
	}
	if resp.ContentLength > limit {
		return nil, "", fmt.Errorf("%s is bigger than %d MB (localImages.maxImageMB)", localImagesSize(resp.ContentLength), limit>>20)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > limit {
		return nil, "", fmt.Errorf("bigger than %d MB (localImages.maxImageMB)", limit>>20)
	}
	ext := localImagesPictureExt(b, localImagesSettings().Svg)
	if ext == "" {
		if localImagesPictureExt(b, true) == ".svg" {
			return nil, "", fmt.Errorf("an svg, which is not kept unless localImages.svg is on")
		}
		ct := http.DetectContentType(b)
		return nil, "", fmt.Errorf("not a picture (%s)", strings.SplitN(ct, ";", 2)[0])
	}
	return b, ext, nil
}

// localImagesSize is a size the way a person reads one.
func localImagesSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

// Failures remembered, so the download on save does not ask a dead site
// again on every save (and its own write, which is a save, cannot loop).
var localImagesFailures = struct {
	sync.Mutex
	m map[string]time.Time
}{m: map[string]time.Time{}}

func localImagesFailedRecently(u string) bool {
	localImagesFailures.Lock()
	defer localImagesFailures.Unlock()
	t, ok := localImagesFailures.m[u]
	return ok && time.Since(t) < time.Hour
}

func localImagesNoteFailure(u string) {
	localImagesFailures.Lock()
	localImagesFailures.m[u] = time.Now()
	localImagesFailures.Unlock()
}

// ---------------------------------------------------------------------------
// Where they go
// ---------------------------------------------------------------------------

var localImagesNameRe = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

func localImagesSlug(s string) string {
	s = strings.Trim(localImagesNameRe.ReplaceAllString(s, "-"), "-.")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-.")
	}
	return s
}

// localImagesBaseName is the name a picture from url is kept under, without its
// extension: the url's own file name, or a hash of the url when it has none
// worth keeping.
func localImagesBaseName(raw string) string {
	u, err := url.Parse(raw)
	name := ""
	if err == nil {
		b := path.Base(u.Path)
		if un, err := url.PathUnescape(b); err == nil {
			b = un
		}
		name = localImagesSlug(strings.TrimSuffix(b, path.Ext(b)))
	}
	if name == "" || name == "/" || name == "image" || name == "img" || name == "download" || name == "index" {
		sum := sha1.Sum([]byte(raw))
		if name == "" || name == "/" {
			name = "image"
		}
		name += "-" + hex.EncodeToString(sum[:])[:8]
	}
	return name
}

// localImagesDirFor is the folder pictures linked from orgFile are kept in:
// localImages.dir (default images/) beside the file, {file} replaced by the
// file's name. It has to be inside an org directory.
func localImagesDirFor(orgFile string) (string, error) {
	d := strings.TrimSpace(localImagesSettings().Dir)
	if d == "" {
		d = "images"
	}
	if strings.Contains(d, "{file}") {
		base := strings.TrimSuffix(filepath.Base(orgFile), filepath.Ext(orgFile))
		d = strings.ReplaceAll(d, "{file}", localImagesSlug(base))
	}
	d = filepath.FromSlash(d)
	if !filepath.IsAbs(d) {
		abs, _ := filepath.Abs(orgFile)
		d = filepath.Join(filepath.Dir(abs), d)
	}
	p, ok := insideOrgDirs(filepath.Clean(d))
	if !ok {
		return "", fmt.Errorf("%s is not inside an org directory (localImages.dir)", d)
	}
	return p, nil
}

// localImagesRootDirFor is the folder localImagesDirFor's folders sit in, for looking for
// orphans: the part of localImages.dir before {file}.
func localImagesRootDirFor(orgFile string) string {
	d := strings.TrimSpace(localImagesSettings().Dir)
	if d == "" {
		d = "images"
	}
	if i := strings.Index(d, "{file}"); i >= 0 {
		d = strings.TrimRight(d[:i], "/\\")
		if d == "" {
			d = "."
		}
	}
	d = filepath.FromSlash(d)
	if !filepath.IsAbs(d) {
		abs, _ := filepath.Abs(orgFile)
		d = filepath.Join(filepath.Dir(abs), d)
	}
	return filepath.Clean(d)
}

var localImagesStoreLock sync.Mutex

// localImagesStore keeps data in dir under base+ext, unless the same bytes are
// already there under any name, which is then the answer.
func localImagesStore(dir, base string, data []byte, ext string) (string, bool, error) {
	localImagesStoreLock.Lock()
	defer localImagesStoreLock.Unlock()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", false, err
	}
	if ents, err := os.ReadDir(dir); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil || info.Size() != int64(len(data)) {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if have, err := os.ReadFile(p); err == nil && bytes.Equal(have, data) {
				return p, true, nil
			}
		}
	}
	for n := 1; n < 1000; n++ {
		name := base + ext
		if n > 1 {
			name = fmt.Sprintf("%s-%d%s", base, n, ext)
		}
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.WriteFile(p, data, 0644); err != nil {
			return "", false, err
		}
		return p, false, nil
	}
	return "", false, fmt.Errorf("too many pictures called %s", base)
}

// ---------------------------------------------------------------------------
// One file
// ---------------------------------------------------------------------------

type LocalImageFetched struct {
	Url  string
	Link string // as written in the file, relative to it
	Src  string `json:",omitempty"` // where a browser can show it
	Line int
	Size int64
	// The same picture was already in the folder.
	Reused bool `json:",omitempty"`
}

type LocalImageFailed struct {
	Url  string
	Line int
	Why  string
}

type LocalImagesFile struct {
	File string
	Ok   bool
	Msg  string `json:",omitempty"`
	// Where its pictures go, relative to the file.
	Dir     string
	Images  []LocalImageRemote       `json:",omitempty"`
	Fetched []LocalImageFetched `json:",omitempty"`
	Failed  []LocalImageFailed  `json:",omitempty"`
	Bytes   int64
	Wrote   bool
}

func localImagesFileOf(name string) (string, error) {
	f := GetDb().FindByFile(name)
	if f == nil {
		return "", fmt.Errorf("no org file called %q", name)
	}
	return f.Filename, nil
}

// localImagesScanFile finds a file's web pictures; with probe the servers of urls
// with no extension are asked what they are (and every picture's size),
// at most probeCap of them.
func localImagesScanFile(name string, probe bool, probeCap int) LocalImagesFile {
	file, err := localImagesFileOf(name)
	if err != nil {
		return LocalImagesFile{File: name, Msg: err.Error()}
	}
	res := LocalImagesFile{File: file, Ok: true}
	if d, err := localImagesDirFor(file); err == nil {
		res.Dir = linkRelativeTo(d, file)
	} else {
		res.Ok, res.Msg = false, err.Error()
		return res
	}
	lines := fileLines(file)
	if lines == nil {
		res.Ok, res.Msg = false, "could not read "+file
		return res
	}
	imgs := localImagesScanLines(lines, localImagesSettings().Svg)
	for i := range imgs {
		imgs[i].Skip = localImagesRefusal(imgs[i].Url)
	}
	if probe {
		// Asked in parallel, four at a time, a few at most.
		var wg sync.WaitGroup
		sem := make(chan struct{}, 4)
		asked := 0
		for i := range imgs {
			if imgs[i].Skip != "" || asked >= probeCap {
				continue
			}
			asked++
			wg.Add(1)
			go func(im *LocalImageRemote) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				p := localImagesProbe(im.Url)
				im.Type, im.Size = p.Type, p.Size
				if im.Sure && (strings.HasPrefix(p.Err, "404") || strings.HasPrefix(p.Err, "410")) {
					im.Skip = "the site says it is gone (" + p.Err + ")"
				}
				if !im.Sure {
					switch {
					case p.Err != "":
						im.Skip = "could not ask: " + p.Err
					case !strings.HasPrefix(p.Type, "image/") || (p.Type == "image/svg+xml" && !localImagesSettings().Svg):
						im.Skip = "not a picture (" + p.Type + ")"
					}
				}
			}(&imgs[i])
		}
		wg.Wait()
	}
	// An extensionless url nobody asked about is left out: it is most likely
	// a page, and a picture only when its server says so.
	keep := []LocalImageRemote{}
	for _, im := range imgs {
		if !im.Sure && im.Type == "" && im.Skip == "" {
			continue
		}
		if !im.Sure && (strings.HasPrefix(im.Skip, "not a picture") || strings.HasPrefix(im.Skip, "could not ask")) {
			continue
		}
		keep = append(keep, im)
	}
	res.Images = keep
	return res
}

type localImagesRun struct {
	budget int64
	stamp  string
	cache  map[string]localImagesGot // url -> what it became, across files in one run
}

type localImagesGot struct {
	path   string
	size   int64
	reused bool
	err    error
}

func newLocalImagesRun() *localImagesRun {
	return &localImagesRun{budget: localImagesMaxRun(), stamp: localImagesStamp(time.Now()), cache: map[string]localImagesGot{}}
}

var localImagesWriteLock sync.Mutex

// localImagesLocalize downloads a file's web pictures (only those in only, when it
// names any) and points the links at the copies.
func localImagesLocalize(name string, only map[string]bool, dry, auto bool, run *localImagesRun) LocalImagesFile {
	res := localImagesScanFile(name, true, 40)
	if !res.Ok {
		return res
	}
	file := res.File
	dir, _ := localImagesDirFor(file)
	want := []LocalImageRemote{}
	seen := map[string]bool{}
	for _, im := range res.Images {
		if len(only) > 0 && !only[im.Url] {
			continue
		}
		if im.Skip != "" {
			res.Failed = append(res.Failed, LocalImageFailed{Url: im.Url, Line: im.Line, Why: im.Skip})
			continue
		}
		if auto && localImagesFailedRecently(im.Url) {
			continue
		}
		if seen[im.Url] {
			continue
		}
		seen[im.Url] = true
		want = append(want, im)
	}
	res.Images = nil
	if dry {
		for _, im := range want {
			ext := strings.ToLower(path.Ext(func() string { u, _ := url.Parse(im.Url); return u.Path }()))
			if !localImagesImageExt(ext, true) {
				ext = ".png"
			}
			if ext == ".jpeg" {
				ext = ".jpg"
			}
			p := filepath.Join(dir, localImagesBaseName(im.Url)+ext)
			res.Fetched = append(res.Fetched, LocalImageFetched{Url: im.Url, Line: im.Line, Link: linkRelativeTo(p, file), Size: im.Size})
		}
		return res
	}
	got := map[string]string{}
	for _, im := range want {
		g, ok := run.cache[im.Url+"\x00"+dir]
		if !ok {
			if run.budget <= 0 {
				res.Failed = append(res.Failed, LocalImageFailed{Url: im.Url, Line: im.Line, Why: fmt.Sprintf("this run has downloaded %d MB already (localImages.maxRunMB)", localImagesMaxRun()>>20)})
				continue
			}
			limit := localImagesMaxImage()
			if limit > run.budget {
				limit = run.budget
			}
			data, ext, err := localImagesDownload(im.Url, limit)
			if err == nil {
				g.path, g.reused, err = localImagesStore(dir, localImagesBaseName(im.Url), data, ext)
				g.size = int64(len(data))
				if !g.reused {
					run.budget -= g.size
				}
			}
			g.err = err
			run.cache[im.Url+"\x00"+dir] = g
		}
		if g.err != nil {
			localImagesNoteFailure(im.Url)
			res.Failed = append(res.Failed, LocalImageFailed{Url: im.Url, Line: im.Line, Why: g.err.Error()})
			continue
		}
		link := linkRelativeTo(g.path, file)
		got[im.Url] = link
		res.Fetched = append(res.Fetched, LocalImageFetched{Url: im.Url, Line: im.Line, Link: link, Src: mediaURL(link, file), Size: g.size, Reused: g.reused})
		if !g.reused {
			res.Bytes += g.size
		}
	}
	if len(got) == 0 {
		return res
	}
	localImagesWriteLock.Lock()
	defer localImagesWriteLock.Unlock()
	// Read again: the downloads took a while, and the file may have been
	// saved meanwhile. The edit is worked out on what it says now.
	lines := fileLines(file)
	if lines == nil {
		res.Ok, res.Msg = false, "could not read "+file+" again"
		return res
	}
	out, n := localImagesRewrite(lines, got, run.stamp, localImagesAnnotate())
	if n == 0 {
		return res
	}
	if err := writeLines(file, out); err != nil {
		res.Ok, res.Msg = false, "the pictures were saved but the file could not be written: "+err.Error()
		return res
	}
	res.Wrote = true
	return res
}

// ---------------------------------------------------------------------------
// On save
// ---------------------------------------------------------------------------

var localImagesTimers = struct {
	mu     sync.Mutex
	timers map[string]*time.Timer
}{timers: map[string]*time.Timer{}}

// autoLocalImagesAfterLoad is called from LoadFile for every re-read of a
// file (a save; a server starting is not one), as the TOC's update is:
// debounced, then the file's web pictures are downloaded. It cannot loop:
// its own write is a re-read too, and that run finds the links local, or
// only urls that failed within the hour, which it does not ask again.
func autoLocalImagesAfterLoad(f *common.OrgFile) {
	if f == nil || !featureOn("localImages") || !localImagesSettings().Auto {
		return
	}
	name := f.Filename
	localImagesTimers.mu.Lock()
	defer localImagesTimers.mu.Unlock()
	if t, ok := localImagesTimers.timers[name]; ok {
		t.Stop()
	}
	localImagesTimers.timers[name] = time.AfterFunc(1500*time.Millisecond, func() {
		localImagesTimers.mu.Lock()
		delete(localImagesTimers.timers, name)
		localImagesTimers.mu.Unlock()
		if !featureOn("localImages") || !localImagesSettings().Auto {
			return
		}
		// Cheap first: most saves have no web pictures.
		lines := fileLines(name)
		any := false
		for _, im := range localImagesScanLines(lines, localImagesSettings().Svg) {
			if !localImagesFailedRecently(im.Url) {
				any = true
				break
			}
		}
		if !any {
			return
		}
		res := localImagesLocalize(name, nil, false, true, newLocalImagesRun())
		if res.Wrote {
			fmt.Fprintf(os.Stderr, "LOCALIMAGES %s: kept %d web picture(s) here\n", filepath.Base(name), len(res.Fetched))
		}
		for _, f := range res.Failed {
			fmt.Fprintf(os.Stderr, "LOCALIMAGES %s:%d %s: %s\n", filepath.Base(name), f.Line, f.Url, f.Why)
		}
	})
}

// ---------------------------------------------------------------------------
// Orphans
// ---------------------------------------------------------------------------

type LocalImageOrphan struct {
	Path string
	Rel  string // to the first org directory
	Src  string `json:",omitempty"`
	Size int64
	Mod  time.Time
}

// Any picture-looking name in a text file: what keeps a picture.
var localImagesNameInText = regexp.MustCompile(`(?i)[^\s\[\]/\\:"'()<>|=]+\.(png|jpe?g|gif|webp|avif|bmp|svg)\b`)

var localImagesTextExts = map[string]bool{".org": true, ".org_archive": true, ".canvas": true, ".excalidraw": true, ".json": true, ".md": true, ".txt": true, ".html": true, ".yaml": true, ".yml": true}

// localImagesOrphans is every picture in the pictures folders whose name no text file
// in the org directories mentions. Deliberately generous about "mentions":
// any picture-like name anywhere (a link, a property, a canvas, a dungeon's
// json, a url-escaped link) keeps every picture of that name, so the list
// errs towards keeping.
func localImagesOrphans() ([]LocalImageOrphan, []string) {
	roots := map[string]bool{}
	for _, f := range GetDb().GetFiles() {
		d := localImagesRootDirFor(f)
		if p, ok := insideOrgDirs(d); ok {
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				roots[p] = true
			}
		}
	}
	names := map[string]bool{}
	for _, od := range Conf().Server.OrgDirs {
		filepath.WalkDir(od, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != od && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !localImagesTextExts[strings.ToLower(filepath.Ext(p))] {
				return nil
			}
			if info, err := d.Info(); err != nil || info.Size() > 32<<20 {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for _, m := range localImagesNameInText.FindAll(b, -1) {
				s := string(m)
				if un, err := url.PathUnescape(s); err == nil {
					s = un
				}
				names[strings.ToLower(filepath.Base(filepath.FromSlash(s)))] = true
			}
			return nil
		})
	}
	first := ""
	if len(Conf().Server.OrgDirs) > 0 {
		first, _ = filepath.Abs(Conf().Server.OrgDirs[0])
	}
	out := []LocalImageOrphan{}
	dirs := []string{}
	seen := map[string]bool{}
	for root := range roots {
		dirs = append(dirs, root)
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || seen[p] {
				return nil
			}
			seen[p] = true
			if strings.HasPrefix(d.Name(), ".") || !localImagesImageExt(filepath.Ext(p), true) {
				return nil
			}
			if names[strings.ToLower(d.Name())] {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel := p
			if first != "" {
				if r, err := filepath.Rel(first, p); err == nil && !strings.HasPrefix(r, "..") {
					rel = filepath.ToSlash(r)
				}
			}
			o := LocalImageOrphan{Path: p, Rel: rel, Size: info.Size(), Mod: info.ModTime()}
			if rel != p {
				o.Src = mediaURL(rel, filepath.Join(first, "x.org"))
			}
			out = append(out, o)
			return nil
		})
	}
	sort.Strings(dirs)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, dirs
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /localimages/scan — The Web Pictures A File Links

	#+BEGIN_EXAMPLE
	GET /localimages/scan?file=trip.org
	-> {"Ok": true, "Count": 2, "Bytes": 812345,
	    "Files": [{"File": "/notes/trip.org", "Ok": true, "Dir": "images",
	               "Images": [{"Url": "https://site/x.png", "Line": 12, "Alone": true, "Sure": true,
	                           "Type": "image/png", "Size": 812345},
	                          {"Url": "https://cdn/photo-1501", "Line": 20, "Sure": false, "Type": "image/jpeg"}]}]}
	#+END_EXAMPLE

	For =features: localImages:=. What =/localimages/fetch= would download:
	=[[https://…/x.png]]=, =[[https://…/x.png][desc]]= and a picture's url on
	a line of its own (not in source, example or export blocks, drawers,
	headings or keyword lines). A url with no extension is a picture only
	when its server says so: with one file named, servers are asked (a
	=HEAD=, at most 40), which is also where sizes come from. With no
	=file=, every file is scanned and nobody is asked unless =probe=1=.
	=Skip= says why one will not be fetched (an ignored host, a private
	address). =Line= counts from 1. Writes nothing.
EDOC */
func RequestLocalImagesScan(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "localImages") {
		return
	}
	q := r.URL.Query()
	type answer struct {
		Ok    bool
		Msg   string `json:",omitempty"`
		Count int
		Bytes int64
		Files []LocalImagesFile
	}
	res := answer{Ok: true, Files: []LocalImagesFile{}}
	file := q.Get("file")
	if file == "" {
		file = q.Get("filename")
	}
	if file != "" {
		f := localImagesScanFile(file, q.Get("probe") != "0", 40)
		res.Ok, res.Msg = f.Ok, f.Msg
		res.Files = append(res.Files, f)
	} else {
		probe := q.Get("probe") == "1"
		for _, name := range GetDb().GetFiles() {
			if !IsOrgFile(name) {
				continue
			}
			f := localImagesScanFile(name, probe, 10)
			if len(f.Images) > 0 {
				res.Files = append(res.Files, f)
			}
		}
	}
	for _, f := range res.Files {
		for _, im := range f.Images {
			if im.Skip == "" {
				res.Count++
				res.Bytes += im.Size
			}
		}
	}
	dynJson(w, res)
}

/* SDOC: API
* POST /localimages/fetch — Keep A File's Web Pictures Here

	#+BEGIN_EXAMPLE
	POST /localimages/fetch {"File": "trip.org"}                      every web picture in it
	POST /localimages/fetch {"File": "trip.org", "Links": ["https://site/x.png"]}   just these
	POST /localimages/fetch {"Files": ["a.org", "b.org"]}             several; {} for every file
	POST /localimages/fetch {"File": "trip.org", "DryRun": true}      what would be fetched, and where
	-> {"Ok": true, "Fetched": 2, "Failed": 0, "Bytes": 812345, "Wrote": 1,
	    "Files": [{"File": "/notes/trip.org", "Ok": true, "Dir": "images", "Wrote": true,
	               "Fetched": [{"Url": "https://site/x.png", "Link": "images/x.png", "Src": "/images/images/x.png",
	                            "Line": 12, "Size": 812345}],
	               "Failed": [{"Url": "https://gone/y.jpg", "Line": 30, "Why": "gone answered 404 Not Found"}]}]}
	#+END_EXAMPLE

	For =features: localImages:=. Downloads each picture (http and https
	only; private addresses refused unless =localImages.allowPrivate=; a
	picture over =maxImageMB= (20) skipped, a run stopping at =maxRunMB=
	(200)), checks the bytes really are a picture, keeps it in
	=localImages.dir= (=images/= beside the org file; ={file}= for a folder
	per file) under the url's file name (or a hash of the url), reusing a
	file with the same bytes, and rewrites the links as line edits:
	=[[file:images/x.png]]=, keeping the description, captions and
	=#+ATTR_= lines, with org-download's =#+DOWNLOADED: url @ time= above a
	link that is alone on its line. A link that failed stays as it was.
EDOC */
func PostLocalImagesFetch(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "localImages") {
		return
	}
	var req struct {
		File   string
		Files  []string
		Links  []string
		DryRun bool
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
	}
	files := req.Files
	if req.File != "" {
		files = append(files, req.File)
	}
	if len(files) == 0 {
		for _, f := range GetDb().GetFiles() {
			if IsOrgFile(f) {
				files = append(files, f)
			}
		}
	}
	only := map[string]bool{}
	for _, l := range req.Links {
		only[strings.TrimSpace(l)] = true
	}
	type answer struct {
		Ok      bool
		Msg     string `json:",omitempty"`
		Fetched int
		Failed  int
		Bytes   int64
		Wrote   int
		Files   []LocalImagesFile
	}
	res := answer{Ok: true, Files: []LocalImagesFile{}}
	run := newLocalImagesRun()
	for _, f := range files {
		fr := localImagesLocalize(f, only, req.DryRun, false, run)
		if !fr.Ok {
			if len(files) == 1 {
				res.Ok, res.Msg = false, fr.Msg
			}
		}
		if len(fr.Fetched) == 0 && len(fr.Failed) == 0 && len(files) > 1 && fr.Ok {
			continue
		}
		res.Fetched += len(fr.Fetched)
		res.Failed += len(fr.Failed)
		res.Bytes += fr.Bytes
		if fr.Wrote {
			res.Wrote++
		}
		res.Files = append(res.Files, fr)
	}
	dynJson(w, res)
}

/* SDOC: API
* POST /localimages/download — One Picture, For Text Being Typed

	#+BEGIN_EXAMPLE
	POST /localimages/download {"Url": "https://site/x.png", "File": "trip.org"}
	-> {"Ok": true, "Link": "images/x.png", "Src": "/images/images/x.png", "Size": 812345,
	    "Lines": ["#+DOWNLOADED: https://site/x.png @ 2026-11-05 10:20:33", "[[file:images/x.png]]"]}
	#+END_EXAMPLE

	For =features: localImages:=. Downloads one picture into the folder
	=File='s pictures go in (with every check =/localimages/fetch= makes) and
	answers the lines to put in the text, writing nothing into the org
	file: for an editor pasting a url into a buffer that is not saved yet.
EDOC */
func PostLocalImagesDownload(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "localImages") {
		return
	}
	var req struct {
		Url  string
		File string
	}
	type answer struct {
		Ok     bool
		Msg    string `json:",omitempty"`
		Link   string
		Src    string `json:",omitempty"`
		Size   int64
		Reused bool
		Lines  []string
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		dynJson(w, answer{Msg: err.Error()})
		return
	}
	file, err := localImagesFileOf(req.File)
	if err != nil {
		dynJson(w, answer{Msg: err.Error()})
		return
	}
	dir, err := localImagesDirFor(file)
	if err != nil {
		dynJson(w, answer{Msg: err.Error()})
		return
	}
	u := strings.TrimSpace(req.Url)
	data, ext, err := localImagesDownload(u, localImagesMaxImage())
	if err != nil {
		dynJson(w, answer{Msg: err.Error()})
		return
	}
	p, reused, err := localImagesStore(dir, localImagesBaseName(u), data, ext)
	if err != nil {
		dynJson(w, answer{Msg: err.Error()})
		return
	}
	link := linkRelativeTo(p, file)
	res := answer{Ok: true, Link: link, Src: mediaURL(link, file), Size: int64(len(data)), Reused: reused}
	if localImagesAnnotate() {
		res.Lines = append(res.Lines, localImagesAnnotation("", u, localImagesStamp(time.Now())))
	}
	res.Lines = append(res.Lines, "[[file:"+link+"]]")
	dynJson(w, res)
}

/* SDOC: API
* POST /localimages/orphans — Pictures Nothing Links Any More

	#+BEGIN_EXAMPLE
	POST /localimages/orphans {}
	-> {"Ok": true, "Dirs": ["/notes/images"], "Bytes": 40960,
	    "Orphans": [{"Path": "/notes/images/old.png", "Rel": "images/old.png", "Src": "/images/images/old.png",
	                 "Size": 40960, "Mod": "2026-10-01T09:00:00Z"}]}
	POST /localimages/orphans {"Delete": ["/notes/images/old.png"], "Confirm": true}
	-> {"Ok": true, "Deleted": ["/notes/images/old.png"], "Orphans": [...what is left]}
	#+END_EXAMPLE

	For =features: localImages:=. The pictures in the pictures folders
	(=localImages.dir= beside each org file) whose file name no org file -
	nor a canvas, drawing or other text file in the org directories -
	mentions. Generous about "mentions": a name written anywhere keeps
	every picture of that name. Lists only, unless =Delete= names pictures
	and =Confirm= is true; then each must still be an orphan, and the
	account must be one that may delete files (=/file/delete='s rule).
EDOC */
func PostLocalImagesOrphans(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "localImages") {
		return
	}
	var req struct {
		Delete  []string
		Confirm bool
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
	}
	type answer struct {
		Ok      bool
		Msg     string `json:",omitempty"`
		Dirs    []string
		Orphans []LocalImageOrphan
		Bytes   int64
		Deleted []string          `json:",omitempty"`
		Failed  map[string]string `json:",omitempty"`
	}
	orphans, dirs := localImagesOrphans()
	res := answer{Ok: true, Dirs: dirs, Orphans: orphans}
	if len(req.Delete) > 0 {
		if !req.Confirm {
			res.Ok, res.Msg = false, "nothing was deleted: deleting needs \"Confirm\": true"
		} else if err := mayDeleteFiles(r); err != nil {
			w.WriteHeader(http.StatusForbidden)
			res.Ok, res.Msg = false, err.Error()
		} else {
			is := map[string]bool{}
			for _, o := range orphans {
				is[o.Path] = true
			}
			res.Failed = map[string]string{}
			for _, p := range req.Delete {
				clean, ok := insideOrgDirs(p)
				switch {
				case !ok || !is[clean]:
					res.Failed[p] = "not a picture nothing links"
				default:
					if err := os.Remove(clean); err != nil {
						res.Failed[p] = err.Error()
					} else {
						res.Deleted = append(res.Deleted, clean)
					}
				}
			}
			if len(res.Failed) > 0 {
				res.Ok, res.Msg = false, fmt.Sprintf("%d could not be deleted", len(res.Failed))
			} else {
				res.Failed = nil
			}
			res.Orphans, res.Dirs = localImagesOrphans()
		}
	}
	for _, o := range res.Orphans {
		res.Bytes += o.Size
	}
	dynJson(w, res)
}

/* SDOC: API
* GET /localimages/settings — Where Web Pictures Go
* POST /localimages/settings — Change It

	#+BEGIN_EXAMPLE
	GET  /localimages/settings
	-> {"Ok": true, "Dir": "images", "Auto": false, "MaxImageMB": 20, "MaxRunMB": 200,
	    "Ignore": ["example.com"], "AllowPrivate": false, "Svg": false, "Annotate": true}
	POST /localimages/settings {"Dir": "images/{file}", "Auto": true}
	#+END_EXAMPLE

	For =features: localImages:=. The POST changes =dir=, =auto=,
	=maxImageMB= and =annotate= (the fields it is sent; the others are
	the yaml's to change) at once and in the server's yaml
	(=server: localImages:=, the file kept as =<file>.before-localimages=).
	Administrators only, unless =noAuth=. A =dir= that would leave the org
	directories is refused.
EDOC */
func RequestLocalImagesSettings(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "localImages") {
		return
	}
	type answer struct {
		Ok           bool
		Msg          string `json:",omitempty"`
		Dir          string
		Auto         bool
		MaxImageMB   int
		MaxRunMB     int
		Ignore       []string
		AllowPrivate bool
		Svg          bool
		Annotate     bool
	}
	now := func(ok bool, msg string) answer {
		s := localImagesSettings()
		d := s.Dir
		if strings.TrimSpace(d) == "" {
			d = "images"
		}
		ig := s.Ignore
		if ig == nil {
			ig = []string{}
		}
		return answer{Ok: ok, Msg: msg, Dir: d, Auto: s.Auto, MaxImageMB: int(localImagesMaxImage() >> 20), MaxRunMB: int(localImagesMaxRun() >> 20),
			Ignore: ig, AllowPrivate: s.AllowPrivate, Svg: s.Svg, Annotate: localImagesAnnotate()}
	}
	if r.Method != http.MethodPost {
		dynJson(w, now(true, ""))
		return
	}
	if !Conf().Server.NoAuth && !GetKeystore().IsAdmin(GetUsername(r)) {
		w.WriteHeader(http.StatusForbidden)
		dynJson(w, now(false, "only an administrator may change where web pictures go"))
		return
	}
	var req struct {
		Dir        *string
		Auto       *bool
		MaxImageMB *int
		Annotate   *bool
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		dynJson(w, now(false, err.Error()))
		return
	}
	s := &Conf().Server.LocalImages
	if req.Dir != nil {
		d := strings.TrimSpace(*req.Dir)
		if d == "" {
			d = "images"
		}
		if filepath.IsAbs(d) {
			if _, ok := insideOrgDirs(d); !ok {
				dynJson(w, now(false, d+" is not inside an org directory"))
				return
			}
		} else if c := filepath.Clean(filepath.FromSlash(d)); c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator)) {
			dynJson(w, now(false, "the folder has to be beside the org file or below it, not above"))
			return
		}
		s.Dir = d
	}
	if req.Auto != nil {
		s.Auto = *req.Auto
	}
	if req.MaxImageMB != nil && *req.MaxImageMB > 0 {
		s.MaxImageMB = *req.MaxImageMB
	}
	if req.Annotate != nil {
		a := *req.Annotate
		s.Annotate = &a
	}
	_, _, err := editConfigFile(".before-localimages", func(lines []string) ([]string, error) {
		var err error
		at := []string{"server", "localImages"}
		if req.Dir != nil {
			if lines, err = setYamlPath(lines, append(at, "dir"), s.Dir); err != nil {
				return nil, err
			}
		}
		if req.Auto != nil {
			if lines, err = setYamlBool(lines, append(at, "auto"), s.Auto); err != nil {
				return nil, err
			}
		}
		if req.Annotate != nil {
			if lines, err = setYamlBool(lines, append(at, "annotate"), *s.Annotate); err != nil {
				return nil, err
			}
		}
		if req.MaxImageMB != nil && *req.MaxImageMB > 0 {
			if lines, err = setYamlPath(lines, append(at, "maxImageMB"), strconv.Itoa(s.MaxImageMB)); err != nil {
				return nil, err
			}
			for i, l := range lines {
				if strings.TrimSpace(l) == `maxImageMB: "`+strconv.Itoa(s.MaxImageMB)+`"` {
					lines[i] = strings.Replace(l, `"`+strconv.Itoa(s.MaxImageMB)+`"`, strconv.Itoa(s.MaxImageMB), 1)
				}
			}
		}
		return lines, nil
	})
	msg := ""
	if err != nil {
		msg = "changed until the server restarts: " + err.Error()
	}
	dynJson(w, now(true, msg))
}
