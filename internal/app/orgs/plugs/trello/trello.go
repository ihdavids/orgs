package trello

/* SDOC: Pollers

* Trello

	Mirrors Trello boards into org files: a heading per list, a heading per
	card under it with its due date as a DEADLINE, its labels as tags, who is
	on it, its description, and its checklists as checkboxes. The files are
	written by the poller and rewritten when the board changes, so they are
	for reading, searching and putting on the agenda rather than for editing.

	#+BEGIN_SRC yaml
	server:
	  plugins:
	    - name: trello
	      key: "your api key"            # https://trello.com/power-ups/admin
	      token: "your token"            # or leave it out and keep it in the
	                                     # keyring under orgs-trello
	      doneLists: ["Done", "Shipped"] # cards in these lists are DONE
	      boards:
	        - id: "aBcD1234"             # the part of the board's url after /b/
	          file: "~/org/trello-website.org"
	#+END_SRC

	A card marked complete (its due date ticked) or in a done list is DONE;
	every other card is TODO. Archived cards and lists are left out.
EDOC */

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

type Board struct {
	Id   string `yaml:"id"`
	File string `yaml:"file"`
}

type Trello struct {
	Name      string
	Key       string   `yaml:"key"`
	Token     string   `yaml:"token"`
	Boards    []Board  `yaml:"boards"`
	DoneLists []string `yaml:"doneLists"`
	// The api's address, for a test server; Trello's own when empty.
	Endpoint string `yaml:"endpoint"`

	manager *common.PluginManager
	client  *http.Client
}

// What the board endpoint answers with, as much of it as is written out.
type apiBoard struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Url   string `json:"url"`
	Lists []struct {
		Id     string  `json:"id"`
		Name   string  `json:"name"`
		Closed bool    `json:"closed"`
		Pos    float64 `json:"pos"`
	} `json:"lists"`
	Cards []struct {
		Id          string   `json:"id"`
		Name        string   `json:"name"`
		Desc        string   `json:"desc"`
		Due         string   `json:"due"`
		DueComplete bool     `json:"dueComplete"`
		Closed      bool     `json:"closed"`
		IdList      string   `json:"idList"`
		IdMembers   []string `json:"idMembers"`
		ShortUrl    string   `json:"shortUrl"`
		Pos         float64  `json:"pos"`
		Labels      []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"labels"`
	} `json:"cards"`
	Checklists []struct {
		IdCard     string  `json:"idCard"`
		Name       string  `json:"name"`
		Pos        float64 `json:"pos"`
		CheckItems []struct {
			Name  string  `json:"name"`
			State string  `json:"state"`
			Pos   float64 `json:"pos"`
		} `json:"checkItems"`
	} `json:"checklists"`
	Members []struct {
		Id       string `json:"id"`
		Username string `json:"username"`
	} `json:"members"`
}

func (self *Trello) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Trello) Startup(freq int, manager *common.PluginManager, opts *common.PluginOpts) {
	self.manager = manager
	self.client = &http.Client{Timeout: 30 * time.Second}
}

func (self *Trello) token() string {
	if self.Token != "" || self.manager == nil {
		return self.Token
	}
	return self.manager.GetPass("orgs-trello", "keyring-nonfatal")
}

func (self *Trello) fetch(id string) (*apiBoard, error) {
	base := strings.TrimRight(self.Endpoint, "/")
	if base == "" {
		base = "https://api.trello.com"
	}
	q := url.Values{}
	q.Set("key", self.Key)
	q.Set("token", self.token())
	q.Set("fields", "name,url")
	q.Set("lists", "open")
	q.Set("cards", "open")
	q.Set("checklists", "all")
	q.Set("members", "all")
	client := self.client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Get(base + "/1/boards/" + url.PathEscape(id) + "?" + q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trello answered %s for board %s", resp.Status, id)
	}
	var b apiBoard
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		return nil, err
	}
	return &b, nil
}

// Update rewrites each board's file when the board has changed.
func (self *Trello) Update(db common.ODb) {
	for _, bd := range self.Boards {
		if bd.Id == "" || bd.File == "" {
			continue
		}
		b, err := self.fetch(bd.Id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "trello: %v\n", err)
			continue
		}
		path := expandHome(bd.File)
		text := self.Write(b)
		if old, err := os.ReadFile(path); err == nil && string(old) == text {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "trello: %v\n", err)
			continue
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "trello: %v\n", err)
		}
	}
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

var tagUnsafe = regexp.MustCompile(`[^\p{L}\p{N}_@#%]+`)

// A label as an org tag: its name, or its colour when it has none.
func labelTag(name, color string) string {
	t := strings.Trim(tagUnsafe.ReplaceAllString(strings.TrimSpace(name), "_"), "_")
	if t == "" {
		t = color
	}
	return t
}

// Write is a board as org text. Pure, so it can be tested from a fixture.
func (self *Trello) Write(b *apiBoard) string {
	done := map[string]bool{}
	for _, l := range self.DoneLists {
		done[strings.ToLower(strings.TrimSpace(l))] = true
	}
	members := map[string]string{}
	for _, m := range b.Members {
		members[m.Id] = m.Username
	}
	lists := b.Lists
	sort.SliceStable(lists, func(i, j int) bool { return lists[i].Pos < lists[j].Pos })
	cards := b.Cards
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].Pos < cards[j].Pos })
	checklists := b.Checklists
	sort.SliceStable(checklists, func(i, j int) bool { return checklists[i].Pos < checklists[j].Pos })

	var w strings.Builder
	fmt.Fprintf(&w, "#+TITLE: Trello - %s\n", b.Name)
	fmt.Fprintf(&w, "#+TRELLO: %s\n", b.Id)
	w.WriteString("# Written by orgs from Trello: a change made here is overwritten on the next poll.\n\n")
	for _, l := range lists {
		if l.Closed {
			continue
		}
		listDone := done[strings.ToLower(l.Name)]
		fmt.Fprintf(&w, "* %s\n:PROPERTIES:\n:TRELLOID: %s\n:END:\n", l.Name, l.Id)
		for _, c := range cards {
			if c.Closed || c.IdList != l.Id {
				continue
			}
			kw := "TODO"
			if listDone || c.DueComplete {
				kw = "DONE"
			}
			head := "** " + kw + " " + strings.TrimSpace(c.Name)
			tags := []string{}
			for _, lb := range c.Labels {
				if t := labelTag(lb.Name, lb.Color); t != "" {
					tags = append(tags, t)
				}
			}
			if len(tags) > 0 {
				head += " :" + strings.Join(tags, ":") + ":"
			}
			w.WriteString(head + "\n")
			if c.Due != "" {
				if t, err := time.Parse(time.RFC3339, c.Due); err == nil {
					t = t.Local()
					fmt.Fprintf(&w, "DEADLINE: <%s>\n", t.Format("2006-01-02 Mon 15:04"))
				}
			}
			w.WriteString(":PROPERTIES:\n")
			fmt.Fprintf(&w, ":TRELLOID: %s\n", c.Id)
			if c.ShortUrl != "" {
				fmt.Fprintf(&w, ":URL:      %s\n", c.ShortUrl)
			}
			who := []string{}
			for _, id := range c.IdMembers {
				if u := members[id]; u != "" {
					who = append(who, u)
				}
			}
			if len(who) > 0 {
				fmt.Fprintf(&w, ":ASSIGNED: %s\n", strings.Join(who, ", "))
			}
			w.WriteString(":END:\n")
			if d := strings.TrimSpace(c.Desc); d != "" {
				for _, line := range strings.Split(d, "\n") {
					// A line starting with a star would be read as a heading.
					if strings.HasPrefix(line, "*") {
						line = " " + line
					}
					w.WriteString(strings.TrimRight(line, " \r") + "\n")
				}
			}
			for _, cl := range checklists {
				if cl.IdCard != c.Id {
					continue
				}
				items := cl.CheckItems
				sort.SliceStable(items, func(i, j int) bool { return items[i].Pos < items[j].Pos })
				n := 0
				for _, it := range items {
					if it.State == "complete" {
						n++
					}
				}
				fmt.Fprintf(&w, "- %s [%d/%d]\n", cl.Name, n, len(items))
				for _, it := range items {
					box := " "
					if it.State == "complete" {
						box = "X"
					}
					fmt.Fprintf(&w, "  - [%s] %s\n", box, it.Name)
				}
			}
		}
	}
	return w.String()
}

func init() {
	common.AddPoller("trello", func() common.Poller {
		return &Trello{}
	})
}
