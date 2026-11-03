package common

// org-protocol links: Emacs's way for a browser to hand something to org.
//
// A bookmarklet opens `org-protocol://capture?template=w&url=…&title=…&body=…`
// and whatever the operating system has registered for the scheme gets the
// url. Emacs has three of these sub-protocols, and so do we:
//
//	capture      a capture, filled in from the page (template, url, title, body)
//	store-link   remember a link to the page, to put in somewhere later
//	open-source  open the source file a published page was made from
//
// Two spellings reach the same thing, and both are still in the wild:
//
//	org-protocol://capture?template=w&url=U&title=T&body=B    (org 9's)
//	org-protocol://capture:/w/U/T/B                            (the old one)
//
// In the old one every part is percent-escaped on its own and the parts are
// told apart by position. A first part one character long is a template key,
// anything longer is already the url - which is org's own rule, so an old
// bookmarklet with no key in it still works.
//
// This is the only reader of the grammar. `orgs protocol` (the scheme's
// handler) and the server's `/org-protocol/...` (the same links with the
// scheme swapped for the server's address) both parse here and hand worg the
// flat `/?protocol=capture&template=…` form, so worg never needs to know.

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	ProtoCapture    = "capture"
	ProtoStoreLink  = "store-link"
	ProtoOpenSource = "open-source"
)

// OrgProtocol is one parsed org-protocol link.
type OrgProtocol struct {
	Kind     string // ProtoCapture, ProtoStoreLink or ProtoOpenSource
	Template string
	Url      string
	Title    string
	Body     string
	// Any other key a new-style link carried (`&tags=x&source=y`). Org puts
	// them where a template can ask for them by name, and so does worg: a key
	// here fills the template's hole of the same name.
	Extra map[string]string
}

// The positions of an old-style link's parts, per sub-protocol.
var protoOldKeys = map[string][]string{
	ProtoCapture:    {"url", "title", "body"},
	ProtoStoreLink:  {"url", "title"},
	ProtoOpenSource: {"url"},
}

// ParseOrgProtocol reads an `org-protocol:` link in either spelling. The
// scheme may have one slash or two (`org-protocol:/capture?…` is what some
// systems hand over), and `capture:` may be followed by any number of slashes.
func ParseOrgProtocol(raw string) (OrgProtocol, error) {
	var p OrgProtocol
	s := strings.TrimSpace(raw)
	if len(s) < len("org-protocol:") || !strings.EqualFold(s[:len("org-protocol:")], "org-protocol:") {
		return p, fmt.Errorf("not an org-protocol link: %q", clipProto(raw))
	}
	s = strings.TrimLeft(s[len("org-protocol:"):], "/")
	end := strings.IndexAny(s, "?:/")
	if end < 0 {
		end = len(s)
	}
	p.Kind = strings.ToLower(s[:end])
	if _, ok := protoOldKeys[p.Kind]; !ok {
		return p, fmt.Errorf("org-protocol: %q is not one of capture, store-link or open-source", s[:end])
	}
	rest := strings.TrimLeft(s[end:], ":/")
	if strings.HasPrefix(rest, "?") {
		q, err := url.ParseQuery(rest[1:])
		if err != nil {
			return p, fmt.Errorf("org-protocol: the link's parameters do not read: %v", err)
		}
		for k, vs := range q {
			v := ""
			if len(vs) > 0 {
				v = vs[len(vs)-1]
			}
			p.set(k, v)
		}
	} else if rest != "" {
		parts := strings.Split(rest, "/")
		for i, part := range parts {
			if u, err := url.PathUnescape(part); err == nil {
				parts[i] = u
			}
		}
		keys := protoOldKeys[p.Kind]
		if p.Kind == ProtoCapture && len([]rune(parts[0])) == 1 {
			keys = append([]string{"template"}, keys...)
		}
		// A part past the last key is a slash that was never escaped: it
		// belongs to the last field rather than being thrown away.
		if len(parts) > len(keys) {
			parts = append(parts[:len(keys)-1], strings.Join(parts[len(keys)-1:], "/"))
		}
		for i, part := range parts {
			p.set(keys[i], part)
		}
	}
	if p.Kind != ProtoCapture && strings.TrimSpace(p.Url) == "" {
		return p, fmt.Errorf("org-protocol: %s needs a url", p.Kind)
	}
	return p, nil
}

func (p *OrgProtocol) set(k, v string) {
	switch strings.ToLower(k) {
	case "template":
		p.Template = v
	case "url":
		p.Url = v
	case "title":
		p.Title = v
	case "body":
		p.Body = v
	default:
		if k == "" {
			return
		}
		if p.Extra == nil {
			p.Extra = map[string]string{}
		}
		p.Extra[k] = v
	}
}

// WorgPath is where worg does this: `/?protocol=capture&template=…`. The
// extra keys travel as `x-<name>` so none of them can be mistaken for a
// parameter worg reads for itself (`app`, `open`, `embed`).
func (p OrgProtocol) WorgPath() string {
	q := url.Values{}
	q.Set("protocol", p.Kind)
	add := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	add("template", p.Template)
	add("url", p.Url)
	add("title", p.Title)
	add("body", p.Body)
	keys := make([]string, 0, len(p.Extra))
	for k := range p.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		add("x-"+k, p.Extra[k])
	}
	return "/?" + q.Encode()
}

// OrgLinkText writes `[[url][title]]`, or `[[url]]` with no title. A bracket in
// the url is escaped and one in the title turned into a brace, since either
// would end the link early. worg's orgprotocol.ts `orgLink` writes the same.
func OrgLinkText(link, title string) string {
	link = strings.NewReplacer("[", "%5B", "]", "%5D").Replace(strings.TrimSpace(link))
	title = strings.Join(strings.Fields(strings.NewReplacer("[", "{", "]", "}").Replace(title)), " ")
	if title == "" || title == link {
		return "[[" + link + "]]"
	}
	return "[[" + link + "][" + title + "]]"
}

func clipProto(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}

// ProtocolProject maps a published site back to the files it was made from,
// for open-source: org's `org-protocol-project-alist`, under
// `server.protocolProjects` in the yaml.
//
//	protocolProjects:
//	  - baseUrl: https://example.com/notes/
//	    workingDirectory: ~/notes
//	    onlineSuffix: .html
//	    workingSuffix: .org
type ProtocolProject struct {
	BaseUrl          string `yaml:"baseUrl" json:"baseUrl"`
	WorkingDirectory string `yaml:"workingDirectory" json:"workingDirectory"`
	OnlineSuffix     string `yaml:"onlineSuffix" json:"onlineSuffix"`
	WorkingSuffix    string `yaml:"workingSuffix" json:"workingSuffix"`
}

// ProtocolSourceRel is the path under a project's working directory that a
// published url was made from, or ok false when the url is not under the
// project's base. A url ending in a slash is its `index` page. The answer is
// slash separated and never climbs out of the working directory.
func ProtocolSourceRel(pr ProtocolProject, link string) (string, bool) {
	base := strings.TrimSpace(pr.BaseUrl)
	if base == "" || !strings.HasPrefix(link, base) {
		return "", false
	}
	rel := link[len(base):]
	if i := strings.IndexAny(rel, "?#"); i >= 0 {
		rel = rel[:i]
	}
	if u, err := url.PathUnescape(rel); err == nil {
		rel = u
	}
	if rel == "" || strings.HasSuffix(rel, "/") {
		rel += "index" + pr.OnlineSuffix
	}
	if pr.OnlineSuffix != "" && strings.HasSuffix(rel, pr.OnlineSuffix) {
		rel = strings.TrimSuffix(rel, pr.OnlineSuffix) + pr.WorkingSuffix
	}
	clean := []string{}
	for _, seg := range strings.Split(strings.ReplaceAll(rel, "\\", "/"), "/") {
		switch seg {
		case "", ".":
		case "..":
			return "", false
		default:
			clean = append(clean, seg)
		}
	}
	if len(clean) == 0 {
		return "", false
	}
	return strings.Join(clean, "/"), true
}

// A link stored by store-link, for whatever inserts links next.
type StoredLink struct {
	Url   string
	Title string
	Link  string    // `[[url][title]]`, as OrgLinkText writes it
	At    time.Time // when it was stored
}

type StoredLinkResult struct {
	Ok   bool
	Msg  string
	Link string
}

// What GET /protocol/source answers: the file a published page came from.
type ProtocolSourceResult struct {
	Ok    bool
	Msg   string
	File  string // absolute, on the server's disk
	Known bool   // an org file the server holds, so worg can open it
}
