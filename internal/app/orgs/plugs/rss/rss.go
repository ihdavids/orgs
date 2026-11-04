package rss

/* SDOC: Pollers

* RSS and Atom Feeds

	Org's org-feed: new items in a feed become headings in an org file, once
	each. Each poll fetches the feeds and files what has not been filed before
	under a heading (made when it is missing), as org-feed's default template
	does - the title, when it was fetched, the summary as text, a link to the
	item. An item is remembered by its id (=:RSS_ID:= on the heading, and a
	list kept in the plugin's folder), so deleting one does not bring it back.
	RSS 2.0, RSS 1.0 and Atom are read.

	#+BEGIN_SRC yaml
	server:
	  plugins:
	    - name: rss
	      freq: 3600                 # seconds between polls
	      feeds:
	        - url: "https://example.org/feed.xml"
	          file: "~/org/feeds.org"
	          heading: "Example blog"  # top level when empty
	          todo: "TODO"             # the keyword new items get; "" for none
	          tags: ["feed"]
	          max: 20                  # at most this many new items a poll
	#+END_SRC
EDOC */

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

type Feed struct {
	Url     string   `yaml:"url"`
	File    string   `yaml:"file"`
	Heading string   `yaml:"heading"`
	Todo    *string  `yaml:"todo"`
	Tags    []string `yaml:"tags"`
	Max     int      `yaml:"max"`
}

type Rss struct {
	Feeds []Feed `yaml:"feeds"`

	manager *common.PluginManager
	client  *http.Client
	mu      sync.Mutex
	// Now, for tests.
	now func() time.Time
}

func (self *Rss) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }

func (self *Rss) Startup(freq int, manager *common.PluginManager, opts *common.PluginOpts) {
	self.manager = manager
	self.client = &http.Client{Timeout: 30 * time.Second}
}

// Item is one entry of a feed, whatever it was written in.
type Item struct {
	Id      string
	Title   string
	Link    string
	Summary string
	Date    time.Time
}

type xmlLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Text string `xml:",chardata"`
}

type xmlEntry struct {
	Title       string    `xml:"title"`
	Links       []xmlLink `xml:"link"`
	Guid        string    `xml:"guid"`
	Id          string    `xml:"id"`
	About       string    `xml:"about,attr"`
	Description string    `xml:"description"`
	Summary     string    `xml:"summary"`
	Content     string    `xml:"content"`
	Encoded     string    `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	PubDate     string    `xml:"pubDate"`
	Updated     string    `xml:"updated"`
	Published   string    `xml:"published"`
	DcDate      string    `xml:"http://purl.org/dc/elements/1.1/ date"`
}

type xmlFeed struct {
	Channel struct {
		Items []xmlEntry `xml:"item"`
	} `xml:"channel"`
	Items   []xmlEntry `xml:"item"`  // RSS 1.0 puts them beside the channel
	Entries []xmlEntry `xml:"entry"` // Atom
}

var dateLayouts = []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", "2006-01-02T15:04:05Z07:00", "2006-01-02"}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Parse reads RSS 2.0, RSS 1.0 or Atom.
func Parse(data []byte) ([]Item, error) {
	var f xmlFeed
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("not a feed: %v", err)
	}
	entries := append(append(f.Channel.Items, f.Items...), f.Entries...)
	out := []Item{}
	for _, e := range entries {
		it := Item{Title: strings.TrimSpace(e.Title)}
		for _, l := range e.Links {
			switch {
			case l.Href != "" && (l.Rel == "" || l.Rel == "alternate"):
				if it.Link == "" {
					it.Link = l.Href
				}
			case strings.TrimSpace(l.Text) != "" && it.Link == "":
				it.Link = strings.TrimSpace(l.Text)
			}
		}
		it.Id = strings.TrimSpace(firstOf(e.Guid, e.Id, e.About, it.Link, it.Title))
		it.Summary = strings.TrimSpace(firstOf(e.Description, e.Summary, e.Encoded, e.Content))
		it.Date = parseDate(firstOf(e.PubDate, e.Published, e.Updated, e.DcDate))
		if it.Id != "" {
			out = append(out, it)
		}
	}
	return out, nil
}

func firstOf(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

var (
	tagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe = regexp.MustCompile(`\s+`)
)

// plain is html as a line of text, at most n runes.
func plain(s string, n int) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	if r := []rune(s); len(r) > n {
		s = string(r[:n]) + "…"
	}
	return s
}

func orgStamp(t time.Time) string { return "[" + t.Format("2006-01-02 Mon 15:04") + "]" }

// Entry is one item as org text at a heading level, org-feed's default
// template with the id kept.
func (f Feed) Entry(it Item, lvl int, fetched time.Time) []string {
	head := strings.Repeat("*", lvl) + " "
	kw := "TODO"
	if f.Todo != nil {
		kw = *f.Todo
	}
	if kw != "" {
		head += kw + " "
	}
	title := strings.ReplaceAll(plain(it.Title, 200), "\n", " ")
	if title == "" {
		title = it.Link
	}
	head += title
	if len(f.Tags) > 0 {
		head += "  :" + strings.Join(f.Tags, ":") + ":"
	}
	out := []string{head, ":PROPERTIES:", ":RSS_ID: " + it.Id}
	if !it.Date.IsZero() {
		out = append(out, ":RSS_DATE: "+orgStamp(it.Date.Local()))
	}
	out = append(out, ":END:", orgStamp(fetched))
	if s := plain(it.Summary, 1000); s != "" {
		out = append(out, s)
	}
	if it.Link != "" {
		out = append(out, "[["+it.Link+"]]")
	}
	return out
}

var rssIdRe = regexp.MustCompile(`(?i)^\s*:RSS_ID:\s*(.+?)\s*$`)

// Place is lines with the new items under the feed's heading. Items whose id
// is already in the file, or in seen, are left out.
func (f Feed) Place(lines []string, items []Item, seen map[string]bool, fetched time.Time) ([]string, []string) {
	have := map[string]bool{}
	for _, l := range lines {
		if m := rssIdRe.FindStringSubmatch(l); m != nil {
			have[m[1]] = true
		}
	}
	fresh := []Item{}
	for _, it := range items {
		if !have[it.Id] && !seen[it.Id] {
			fresh = append(fresh, it)
			have[it.Id] = true
		}
	}
	max := f.Max
	if max <= 0 {
		max = 20
	}
	if len(fresh) > max {
		fresh = fresh[:max]
	}
	if len(fresh) == 0 {
		return lines, nil
	}
	added := []string{}
	lvl, at := 1, len(lines)
	if h := strings.TrimSpace(f.Heading); h != "" {
		row := -1
		for i, l := range lines {
			if m := headRe.FindStringSubmatch(l); m != nil && len(m[1]) == 1 && strings.TrimSpace(tagsRe.ReplaceAllString(m[2], "")) == h {
				row = i
				break
			}
		}
		if row < 0 {
			for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
				lines = lines[:len(lines)-1]
			}
			lines = append(lines, "* "+h)
			row = len(lines) - 1
		}
		lvl, at = 2, len(lines)
		for i := row + 1; i < len(lines); i++ {
			if m := headRe.FindStringSubmatch(lines[i]); m != nil && len(m[1]) == 1 {
				at = i
				break
			}
		}
		for at > row+1 && strings.TrimSpace(lines[at-1]) == "" {
			at--
		}
	}
	block := []string{}
	for _, it := range fresh {
		block = append(block, f.Entry(it, lvl, fetched)...)
		added = append(added, it.Id)
	}
	out := append(append(append([]string{}, lines[:at]...), block...), lines[at:]...)
	return out, added
}

var (
	headRe = regexp.MustCompile(`^(\*+)\s+(.*)$`)
	tagsRe = regexp.MustCompile(`\s+:[\w@#%:]+:\s*$`)
)

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func (self *Rss) seenPath() string {
	dir := "."
	if self.manager != nil && self.manager.HomeDir != "" {
		dir = self.manager.HomeDir
	}
	return filepath.Join(dir, "rss_seen.json")
}

func (self *Rss) loadSeen() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	if b, err := os.ReadFile(self.seenPath()); err == nil {
		json.Unmarshal(b, &out)
	}
	return out
}

func (self *Rss) fetch(u string) ([]Item, error) {
	client := self.client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", u, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Update files each feed's new items.
func (self *Rss) Update(db common.ODb) {
	self.mu.Lock()
	defer self.mu.Unlock()
	now := time.Now()
	if self.now != nil {
		now = self.now()
	}
	seen := self.loadSeen()
	changed := false
	for _, f := range self.Feeds {
		if f.Url == "" || f.File == "" {
			continue
		}
		items, err := self.fetch(f.Url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rss: %v\n", err)
			continue
		}
		path := expandHome(f.File)
		var lines []string
		if b, err := os.ReadFile(path); err == nil {
			lines = strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
		}
		out, added := f.Place(lines, items, seen[f.Url], now)
		if len(added) == 0 {
			continue
		}
		if seen[f.Url] == nil {
			seen[f.Url] = map[string]bool{}
		}
		for _, id := range added {
			seen[f.Url][id] = true
		}
		changed = true
		os.MkdirAll(filepath.Dir(path), 0o755)
		if self.manager != nil && self.manager.WriteLines != nil {
			err = self.manager.WriteLines(path, out)
		} else {
			err = os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "rss: %s: %v\n", path, err)
		}
	}
	if changed {
		b, _ := json.MarshalIndent(seen, "", " ")
		os.WriteFile(self.seenPath(), b, 0o644)
	}
}

func init() {
	common.AddPoller("rss", func() common.Poller { return &Rss{} })
}
