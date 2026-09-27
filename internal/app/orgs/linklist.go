package orgs

// Every link in the database, as a list rather than as a graph.
//
// `/links` and `/links/graph` answer the question "what points at this file";
// this answers "where did I put that link", which is a different question with
// a different answer. Most of what it turns up is *external* - a ticket, a
// document, a video - which the graph deliberately throws away, and which has
// no other index anywhere: a link pasted into a heading eighteen months ago is
// findable by grep and by nothing else.
//
// It is built out of the same index the graph is, so it costs nothing extra and
// cannot disagree with the graph about what a link is.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// The services worth naming, by the host they are served from. A suffix match,
// so "issues.apache.org" and "apache.org" are the same service and any
// "company.atlassian.net" is Jira.
//
// This list is not meant to be complete - it cannot be - which is why
// serviceOf falls back on the domain itself. It is here for the handful that
// are either not named after themselves (atlassian.net is Jira) or are worth
// gathering under one name (drive, docs and sheets are all Google Docs).
var knownServices = []struct {
	suffix  string
	service string
}{
	{"github.com", "GitHub"},
	{"githubusercontent.com", "GitHub"},
	{"gist.github.com", "GitHub"},
	{"gitlab.com", "GitLab"},
	{"bitbucket.org", "Bitbucket"},
	{"atlassian.net", "Jira"},
	{"jira.com", "Jira"},
	{"confluence.com", "Confluence"},
	{"docs.google.com", "Google Docs"},
	{"drive.google.com", "Google Docs"},
	{"sheets.google.com", "Google Docs"},
	{"calendar.google.com", "Google Calendar"},
	{"mail.google.com", "Gmail"},
	{"google.com", "Google"},
	{"youtube.com", "YouTube"},
	{"youtu.be", "YouTube"},
	{"stackoverflow.com", "Stack Overflow"},
	{"stackexchange.com", "Stack Exchange"},
	{"wikipedia.org", "Wikipedia"},
	{"arxiv.org", "arXiv"},
	{"news.ycombinator.com", "Hacker News"},
	{"reddit.com", "Reddit"},
	{"slack.com", "Slack"},
	{"notion.so", "Notion"},
	{"figma.com", "Figma"},
	{"linear.app", "Linear"},
	{"asana.com", "Asana"},
	{"trello.com", "Trello"},
	{"dropbox.com", "Dropbox"},
	{"sharepoint.com", "SharePoint"},
	{"office.com", "Microsoft 365"},
	{"microsoft.com", "Microsoft"},
	{"amazon.com", "Amazon"},
	{"aws.amazon.com", "AWS"},
	{"apple.com", "Apple"},
	{"developer.mozilla.org", "MDN"},
	{"orgmode.org", "Org Mode"},
	{"npmjs.com", "npm"},
	{"pkg.go.dev", "Go Packages"},
	{"golang.org", "Go"},
	{"go.dev", "Go"},
	{"crates.io", "crates.io"},
	{"pypi.org", "PyPI"},
	{"readthedocs.io", "Read the Docs"},
	{"medium.com", "Medium"},
	{"substack.com", "Substack"},
	{"linkedin.com", "LinkedIn"},
	{"twitter.com", "Twitter"},
	{"x.com", "Twitter"},
	{"mastodon.social", "Mastodon"},
	{"discord.com", "Discord"},
	{"zoom.us", "Zoom"},
	{"vimeo.com", "Vimeo"},
	{"twitch.tv", "Twitch"},
	{"spotify.com", "Spotify"},
}

// The service a host belongs to, said the way a person would say it.
//
// A host nobody has a name for is called after its own domain - "example.com"
// becomes "Example" - because the point of the name is to gather the links from
// one place together, and a domain does that perfectly well. What it must never
// do is answer "" and drop the link out of every group.
func serviceOf(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return ""
	}
	for _, k := range knownServices {
		if host == k.suffix || strings.HasSuffix(host, "."+k.suffix) {
			return k.service
		}
	}
	// The registrable part, near enough: the last two labels, or the whole
	// thing when it has fewer. Good enough to group by, and wrong only for the
	// likes of co.uk, where it groups a little too coarsely rather than
	// splitting one service into several.
	parts := strings.Split(host, ".")
	name := host
	if len(parts) >= 2 {
		name = parts[len(parts)-2]
	}
	if name == "" {
		return host
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// The scheme and host of a link, for the links that have them. A link that is
// not a url - mailto:, doi:, a bare id - keeps its scheme and has no host, and
// is grouped under the scheme instead.
func linkSchemeHost(raw string) (scheme string, host string) {
	proto, rest := splitProtocol(raw)
	// splitProtocol calls anything before a colon a protocol, which is how
	// `notes.org::*Plans` comes back as a scheme called "notes.org". Only the
	// ones the resolver itself treats as leaving the org files count here, so
	// the answer agrees with the Kind this link was given.
	if proto == "" || !externalProtocols[proto] {
		return "", ""
	}
	switch proto {
	case "http", "https", "ftp", "ftps":
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			return proto, strings.ToLower(u.Hostname())
		}
		// Unparseable, but the host is still the part up to the first slash.
		rest = strings.TrimPrefix(rest, "//")
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			rest = rest[:i]
		}
		return proto, strings.ToLower(rest)
	case "mailto":
		if i := strings.LastIndex(rest, "@"); i >= 0 {
			return proto, strings.ToLower(rest[i+1:])
		}
		return proto, ""
	}
	return proto, ""
}

// What a link is grouped under: its service when it leaves the org files, the
// scheme when it is a url-ish thing with no host, and the kind of org link it
// is otherwise. Everything lands somewhere.
func linkGroup(e *common.LinkEntry) string {
	if e.Service != "" {
		return e.Service
	}
	if e.Scheme != "" {
		return e.Scheme
	}
	switch e.Kind {
	case "external":
		return "external"
	case "":
		return "org"
	}
	return "org"
}

/* SDOC: API
* GET /links/all — Every Link in the Database

	Answers with every link written in every org file the server holds: what it
	says, where it was written, and - for a link that leaves the org files - the
	scheme, host and service it points at.

	This is the flat sibling of =/links= and =/links/graph=. Those answer "what
	points at this file" and throw away everything that is not org to org; this
	keeps all of it, because the links worth going back and finding are usually
	the external ones.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                           |
	|------------+--------+----------+-------------------------------------------------------|
	| =service=  | string | no       | Only links belonging to this service                  |
	| =file=     | string | no       | Only links written in this file (basename or path)    |

	*Response:* A =LinkList= JSON object.
	| Field      | Type   | Description                                        |
	|------------+--------+----------------------------------------------------|
	| =Links=    | array  | The links, in file then line order                 |
	| =Services= | array  | Each service and how many links point at it        |
	| =Total=    | number | How many links there are before any filter         |

	Each entry carries =Raw=, =Desc=, =Kind=, =Broken=, =Scheme=, =Host=,
	=Service=, where it was written (=Filename=, =Heading=, =Olp=, =Hash=,
	=Line=) and, for an org link, where it lands (=ToFilename=, =ToHeadline=,
	=ToHash=).
EDOC */
func RequestAllLinks(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	wantService := strings.TrimSpace(r.URL.Query().Get("service"))
	wantFile := strings.TrimSpace(r.URL.Query().Get("file"))
	if wantFile != "" {
		if f, err := resolveRequestedFile(wantFile); err == nil {
			wantFile = f
		}
	}

	idx := getLinkIndex()
	out := common.LinkList{Links: []common.LinkEntry{}, Services: []common.LinkService{}}
	counts := map[string]int{}

	for _, l := range idx.links {
		e := common.LinkEntry{
			Raw:      l.Raw,
			Desc:     l.Desc,
			Kind:     l.Kind,
			Broken:   l.Broken,
			Filename: l.From.Filename,
			Heading:  l.From.Headline,
			Olp:      l.From.Olp,
			Hash:     l.From.Hash,
			Line:     l.From.Line,

			ToFilename: l.To.Filename,
			ToHeadline: l.To.Headline,
			ToHash:     l.To.Hash,
		}
		if l.Kind == "external" {
			e.Scheme, e.Host = linkSchemeHost(l.Raw)
			e.Service = serviceOf(e.Host)
			if e.Scheme == "" {
				// Something with a protocol the resolver does not follow -
				// `file:images/x.png` is the common one. Grouping all of those
				// under "external" would put a folder of screenshots in with
				// the things that are genuinely elsewhere, so they are grouped
				// by the protocol they name.
				if proto, _ := splitProtocol(l.Raw); proto != "" {
					e.Scheme = proto
				}
			}
			// A link at a file beside the notes is worth showing rather than
			// merely naming: the same sum the kanban cards and a source block's
			// result file do.
			if e.Host == "" {
				if _, rest := splitProtocol(l.Raw); rest != "" {
					target := strings.TrimPrefix(rest, "//")
					if url := mediaURL(target, l.From.Filename); url != "" {
						e.Url = url
						e.Media, _ = mediaKindOf(target)
					}
				}
			}
		}
		out.Total++
		counts[linkGroup(&e)]++
		if wantService != "" && linkGroup(&e) != wantService {
			continue
		}
		if wantFile != "" && e.Filename != wantFile {
			continue
		}
		out.Links = append(out.Links, e)
	}

	sort.SliceStable(out.Links, func(a, b int) bool {
		if out.Links[a].Filename != out.Links[b].Filename {
			return out.Links[a].Filename < out.Links[b].Filename
		}
		return out.Links[a].Line < out.Links[b].Line
	})

	for name, n := range counts {
		out.Services = append(out.Services, common.LinkService{Service: name, Count: n})
	}
	// Busiest first, and by name where two are the same size, so the strip is
	// stable between requests rather than reordering itself on every reload.
	sort.Slice(out.Services, func(a, b int) bool {
		if out.Services[a].Count != out.Services[b].Count {
			return out.Services[a].Count > out.Services[b].Count
		}
		return out.Services[a].Service < out.Services[b].Service
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
