package common

// Link titles (features.autoLinkTitle, after Emacs's
// org-cliplink): what a web page calls itself, cleaned of the site's
// own name, written as an org link. The pure half: reading a page's head,
// cleaning a title, and finding the bare urls in a file that could have one.
// The fetching, the cache and the endpoints are internal/app/orgs/linktitle.go.

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/net/html"
	htmlcharset "golang.org/x/net/html/charset"
)

// LinkTitle is what GET /link/title answers for one url.
type LinkTitle struct {
	Ok  bool
	Off bool   `json:",omitempty"`
	Msg string `json:",omitempty"`
	// The url asked about, and where it ended after redirects.
	Url   string
	Final string `json:",omitempty"`
	// The title, cleaned; Raw is the page's own before the cleanup.
	Title       string
	Raw         string `json:",omitempty"`
	Site        string `json:",omitempty"`
	Description string `json:",omitempty"`
	Icon        string `json:",omitempty"`
	// The org link: [[url][Title]], or [[url]] when there is no title.
	Link string
	// "page" (the html's head) or "oembed" (the site's oEmbed endpoint).
	From    string `json:",omitempty"`
	Cached  bool   `json:",omitempty"`
	Ignored bool   `json:",omitempty"` // a host in autoLinkTitle.ignore: never fetched
}

// LinkTitlesRequest is POST /link/titles.
type LinkTitlesRequest struct {
	Urls    []string
	Refresh bool `json:",omitempty"`
}

// LinkTitlesAnswer is POST /link/titles' answer, one LinkTitle per url asked, in order.
type LinkTitlesAnswer struct {
	Ok     bool
	Off    bool   `json:",omitempty"`
	Msg    string `json:",omitempty"`
	Titles []LinkTitle
}

// RetitleRequest is POST /link/retitle: the bare urls and [[url]] links of a
// file (File) or of some text (Text, never written) given their titles.
// Without Apply it is a preview. An apply sends back the preview's Hash and
// the Titles to use (a client may have dropped or edited some); it is refused
// if the file changed since.
type RetitleRequest struct {
	File    string            `json:",omitempty"`
	Text    string            `json:",omitempty"`
	Apply   bool              `json:",omitempty"`
	Hash    string            `json:",omitempty"`
	Titles  map[string]string `json:",omitempty"`
	Refresh bool              `json:",omitempty"`
}

// RetitleLink is one place in the text that could be given a title.
type RetitleLink struct {
	Url   string
	Line  int    // from 0
	Col   int    // byte offset in the line
	End   int    // byte offset just past it
	Kind  string // "bare" (a plain url) or "link" ([[url]])
	Title string `json:",omitempty"`
	Ok    bool
	Msg   string `json:",omitempty"`
}

// RetitleAnswer is POST /link/retitle's answer.
type RetitleAnswer struct {
	Ok      bool
	Off     bool   `json:",omitempty"`
	Msg     string `json:",omitempty"`
	File    string `json:",omitempty"`
	Hash    string `json:",omitempty"` // of the file as read, for the apply
	Links   []RetitleLink
	Text    string `json:",omitempty"` // the text after (a preview, or a Text request)
	Patch   string `json:",omitempty"` // a unified diff, for a terminal
	Added   int    `json:",omitempty"`
	Removed int    `json:",omitempty"`
	Changed int    // how many links were given a title
	Applied bool   `json:",omitempty"`
	Stale   bool   `json:",omitempty"` // an apply refused: the file changed since the preview
}

// ---------------------------------------------------------------------------
// Reading a page
// ---------------------------------------------------------------------------

// PageMeta is what a page's head says about it.
type PageMeta struct {
	Title        string // <title>
	OgTitle      string // og:title
	TwitterTitle string // twitter:title
	Site         string // og:site_name, else application-name
	Description  string // og:description, else description
	Icon         string // the favicon, absolute
	Oembed       string // a <link rel=alternate type=application/json+oembed>
}

// DecodePage turns a page's bytes into UTF-8, by its Content-Type header, a
// byte order mark or a <meta charset> in its first kilobyte (html5's rules).
func DecodePage(b []byte, contentType string) []byte {
	enc, name, _ := htmlcharset.DetermineEncoding(b, contentType)
	if enc == nil || name == "utf-8" {
		return b
	}
	out, err := enc.NewDecoder().Bytes(b)
	if err != nil {
		return b
	}
	return out
}

// ParsePageMeta reads the head of an html page (already UTF-8). base resolves
// relative icon and oEmbed links; nil leaves them as written. Entities are
// decoded; reading stops at <body> once there is a title.
func ParsePageMeta(page []byte, base *url.URL) PageMeta {
	var m PageMeta
	var appName, metaDesc, ogDesc, icon, touchIcon string
	z := html.NewTokenizer(bytes.NewReader(page))
	inTitle, sawTitle := false, false
	var title strings.Builder
	abs := func(ref string) string {
		ref = strings.TrimSpace(ref)
		if ref == "" || base == nil {
			return ref
		}
		u, err := base.Parse(ref)
		if err != nil {
			return ""
		}
		return u.String()
	}
loop:
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			break loop
		case html.TextToken:
			if inTitle {
				title.Write(z.Text())
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if inTitle && string(name) == "title" {
				inTitle, sawTitle = false, true
			}
			if string(name) == "head" && (sawTitle || m.OgTitle != "") {
				break loop
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			attrs := map[string]string{}
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				attrs[strings.ToLower(string(k))] = string(v)
			}
			switch string(name) {
			case "title":
				if !sawTitle && tt == html.StartTagToken {
					inTitle = true
				}
			case "body":
				if sawTitle || m.OgTitle != "" {
					break loop
				}
			case "meta":
				key := strings.ToLower(attrs["property"])
				if key == "" {
					key = strings.ToLower(attrs["name"])
				}
				val := strings.TrimSpace(attrs["content"])
				if val == "" {
					continue
				}
				switch key {
				case "og:title":
					if m.OgTitle == "" {
						m.OgTitle = val
					}
				case "twitter:title":
					if m.TwitterTitle == "" {
						m.TwitterTitle = val
					}
				case "og:site_name":
					if m.Site == "" {
						m.Site = val
					}
				case "application-name":
					appName = val
				case "og:description":
					ogDesc = val
				case "description", "twitter:description":
					if metaDesc == "" {
						metaDesc = val
					}
				}
			case "link":
				rel := " " + strings.ToLower(attrs["rel"]) + " "
				href := attrs["href"]
				if href == "" {
					continue
				}
				switch {
				case strings.Contains(rel, " alternate ") && strings.EqualFold(attrs["type"], "application/json+oembed"):
					if m.Oembed == "" {
						m.Oembed = abs(href)
					}
				case strings.Contains(rel, " apple-touch-icon"):
					if touchIcon == "" {
						touchIcon = abs(href)
					}
				case strings.Contains(rel, " icon "):
					if icon == "" {
						icon = abs(href)
					}
				}
			}
		}
	}
	m.Title = squash(title.String())
	m.OgTitle, m.TwitterTitle = squash(m.OgTitle), squash(m.TwitterTitle)
	if m.Site == "" {
		m.Site = appName
	}
	m.Site = squash(m.Site)
	m.Description = ogDesc
	if m.Description == "" {
		m.Description = metaDesc
	}
	m.Description = clipRunes(squash(m.Description), 300)
	m.Icon = icon
	if m.Icon == "" {
		m.Icon = touchIcon
	}
	if m.Icon == "" && base != nil && (base.Scheme == "http" || base.Scheme == "https") {
		m.Icon = base.Scheme + "://" + base.Host + "/favicon.ico"
	}
	return m
}

// BestTitle is the title to use: og:title, else twitter:title, else
// <title> - unless the og or twitter one is only the site's name, as some
// sites put on every page.
func (m PageMeta) BestTitle() string {
	for _, t := range []string{m.OgTitle, m.TwitterTitle} {
		if t != "" && !strings.EqualFold(t, m.Site) {
			return t
		}
	}
	if m.Title != "" {
		return m.Title
	}
	if m.OgTitle != "" {
		return m.OgTitle
	}
	return m.TwitterTitle
}

func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n])) + "…"
}

// ---------------------------------------------------------------------------
// Cleaning a title
// ---------------------------------------------------------------------------

// DefaultLinkTitleRules are the cleanups for sites everyone links to; a
// rule in the yaml for the same host is tried first, and noDefaultRules
// drops these.
var DefaultLinkTitleRules = []LinkTitleRule{
	{Host: "youtube.com", Strip: []string{` - YouTube$`}},
	{Host: "youtu.be", Strip: []string{` - YouTube$`}},
	{Host: "github.com", Strip: []string{`^GitHub - `, ` · GitHub$`}},
	{Host: "gitlab.com", Strip: []string{` · GitLab$`}},
	{Host: "wikipedia.org", Strip: []string{` [-—–] Wikipedia$`}},
	{Host: "stackoverflow.com", Strip: []string{`^[a-z0-9.+#-]+ - `, ` - Stack Overflow$`}},
	{Host: "stackexchange.com", Strip: []string{`^[a-z0-9.+#-]+ - `, ` - [^-]+ Stack Exchange$`}},
	{Host: "superuser.com", Strip: []string{` - Super User$`}},
	{Host: "serverfault.com", Strip: []string{` - Server Fault$`}},
	{Host: "reddit.com", Strip: []string{` - Reddit$`, ` : reddit\.com$`}},
	{Host: "news.ycombinator.com", Strip: []string{` \| Hacker News$`}},
	{Host: "medium.com", Strip: []string{` \| by [^|]+ \| Medium$`, ` \| Medium$`}},
	{Host: "x.com", Strip: []string{` / X$`}},
	{Host: "twitter.com", Strip: []string{` / Twitter$`, ` / X$`}},
	{Host: "amazon.com", Strip: []string{`^Amazon\.com ?: `}},
	{Host: "vimeo.com", Strip: []string{` on Vimeo$`}},
}

// HostMatches says whether a url's host is pattern or under it
// (www.youtube.com and m.youtube.com are youtube.com). A pattern that starts
// with a dot matches only under it.
func HostMatches(host, pattern string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, ".") {
		return strings.HasSuffix(host, pattern)
	}
	return host == pattern || strings.HasSuffix(host, "."+pattern)
}

var ruleCache = map[string]*regexp.Regexp{}
var ruleMu sync.Mutex

func ruleRe(s string) *regexp.Regexp {
	ruleMu.Lock()
	defer ruleMu.Unlock()
	if re, ok := ruleCache[s]; ok {
		return re
	}
	re, err := regexp.Compile(s)
	if err != nil {
		re = nil
	}
	if len(ruleCache) < 512 {
		ruleCache[s] = re
	}
	return re
}

var siteSeps = []string{" - ", " | ", " · ", " — ", " – ", " :: ", " • ", ": "}

// CleanLinkTitle takes a site's furniture off a page's title: the rules for
// its host, then (unless keepSite) the site's own name when the title ends
// or starts with it after a separator (" - The Guardian", "MDN | "). It never
// cleans a title down to nothing.
func CleanLinkTitle(host, title, site string, rules []LinkTitleRule, keepSite bool) string {
	t := squash(title)
	if t == "" {
		return ""
	}
	for _, r := range rules {
		if !HostMatches(host, r.Host) {
			continue
		}
		for _, s := range r.Strip {
			re := ruleRe(s)
			if re == nil {
				continue
			}
			if c := strings.TrimSpace(re.ReplaceAllString(t, "")); c != "" {
				t = c
			}
		}
	}
	site = squash(site)
	if !keepSite && site != "" && !strings.EqualFold(t, site) {
		low, ls := strings.ToLower(t), strings.ToLower(site)
		for _, sep := range siteSeps {
			if strings.HasSuffix(low, strings.ToLower(sep)+ls) {
				if c := strings.TrimSpace(t[:len(t)-len(sep)-len(site)]); c != "" {
					t = c
					break
				}
			}
			if strings.HasPrefix(low, ls+strings.ToLower(sep)) {
				if c := strings.TrimSpace(t[len(site)+len(sep):]); c != "" {
					t = c
					break
				}
			}
		}
	}
	return t
}

// ---------------------------------------------------------------------------
// Bare links in a file
// ---------------------------------------------------------------------------

var (
	bareUrlRe     = regexp.MustCompile(`https?://[^\s\[\]<>"'{}|\\^` + "`" + `]+`)
	untitledRe    = regexp.MustCompile(`\[\[(https?://[^\]\[]+)\]\]`)
	anyLinkRe     = regexp.MustCompile(`\[\[[^\]]*\](?:\[[^\]]*\])?\]`)
	angleLinkRe   = regexp.MustCompile(`<https?://[^>\s]+>`)
	verbatimRe    = regexp.MustCompile(`(?:^|[\s({'"])([=~][^\s=~](?:[^=~]*?[^\s=~])?[=~])(?:[\s.,;:!?')}-]|$)`)
	ltBlockBeginRe  = regexp.MustCompile(`(?i)^\s*#\+begin_(src|example|export|comment)\b`)
	ltBlockEndRe    = regexp.MustCompile(`(?i)^\s*#\+end_(src|example|export|comment)\b`)
	ltDrawerBeginRe = regexp.MustCompile(`^\s*:PROPERTIES:\s*$`)
	ltDrawerEndRe   = regexp.MustCompile(`(?i)^\s*:END:\s*$`)
)

// trimUrlEnd drops what ends a sentence rather than a url: a full stop or
// comma after it, and a closing bracket it did not open.
func trimUrlEnd(u string) string {
	for u != "" {
		last := u[len(u)-1]
		switch last {
		case '.', ',', ';', ':', '!', '?', '*', '_', '+':
			u = u[:len(u)-1]
			continue
		case ')':
			if strings.Count(u, "(") < strings.Count(u, ")") {
				u = u[:len(u)-1]
				continue
			}
		}
		break
	}
	return u
}

// TitleableLinks finds the places in some org lines that could take a title:
// bare http(s) urls and [[url]] links with no description. Left alone: urls
// inside other links, <angle> links and =verbatim=/~code~, and the lines of
// source, example, export and comment blocks, property drawers, keyword lines
// (#+...), fixed-width lines (: ...) and tables, whose columns a title would
// push out of line.
func TitleableLinks(lines []string) []RetitleLink {
	out := []RetitleLink{}
	inBlock, inDrawer := false, false
	for i, l := range lines {
		switch {
		case inBlock:
			if ltBlockEndRe.MatchString(l) {
				inBlock = false
			}
			continue
		case ltBlockBeginRe.MatchString(l):
			inBlock = true
			continue
		case inDrawer:
			if ltDrawerEndRe.MatchString(l) {
				inDrawer = false
			}
			continue
		case ltDrawerBeginRe.MatchString(l):
			inDrawer = true
			continue
		}
		t := strings.TrimLeft(l, " \t")
		if strings.HasPrefix(t, "#+") || strings.HasPrefix(t, "|") || t == ":" || strings.HasPrefix(t, ": ") {
			continue
		}
		if !strings.Contains(l, "http") {
			continue
		}
		masked := []byte(l)
		for _, m := range untitledRe.FindAllStringSubmatchIndex(l, -1) {
			out = append(out, RetitleLink{Url: l[m[2]:m[3]], Line: i, Col: m[0], End: m[1], Kind: "link"})
		}
		for _, re := range []*regexp.Regexp{anyLinkRe, angleLinkRe, verbatimRe} {
			for _, m := range re.FindAllStringSubmatchIndex(l, -1) {
				if len(m) >= 4 && m[2] >= 0 {
					m = m[2:4]
				}
				for k := m[0]; k < m[1]; k++ {
					masked[k] = ' '
				}
			}
		}
		for _, m := range bareUrlRe.FindAllIndex(masked, -1) {
			u := trimUrlEnd(l[m[0]:m[1]])
			if len(u) <= len("https://") {
				continue
			}
			if p, err := url.Parse(u); err != nil || p.Host == "" {
				continue
			}
			out = append(out, RetitleLink{Url: u, Line: i, Col: m[0], End: m[0] + len(u), Kind: "bare"})
		}
	}
	// In line order, and left to right within a line.
	sortRetitle(out)
	return out
}

func sortRetitle(ls []RetitleLink) {
	for i := 1; i < len(ls); i++ {
		for j := i; j > 0 && (ls[j].Line < ls[j-1].Line || ls[j].Line == ls[j-1].Line && ls[j].Col < ls[j-1].Col); j-- {
			ls[j], ls[j-1] = ls[j-1], ls[j]
		}
	}
}

// RetitleLines gives each titleable link a title from titles (by url), as
// [[url][title]]: a url with no title (or an empty one) is left as it is.
// It answers the new lines and how many links changed; only the lines that
// hold a changed link differ.
func RetitleLines(lines []string, titles map[string]string) ([]string, int) {
	out := append([]string(nil), lines...)
	spots := TitleableLinks(lines)
	n := 0
	for k := len(spots) - 1; k >= 0; k-- {
		s := spots[k]
		t := strings.TrimSpace(titles[s.Url])
		if t == "" {
			continue
		}
		link := OrgLinkText(s.Url, t)
		if !strings.Contains(link, "][") {
			continue
		}
		l := out[s.Line]
		out[s.Line] = l[:s.Col] + link + l[s.End:]
		n++
	}
	return out, n
}

// IsWebUrl is a whole string that is one http(s) url and nothing else - what
// a paste must be to be given a title. worg's linktitle.ts `pastedUrl` and
// VS Code's link-title-model.ts say the same.
func IsWebUrl(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return false
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Host != ""
}
