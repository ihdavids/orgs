package github

// One poll: what upstream says, what the file says, and bringing them level.
//
// The rule that keeps the two sides from chasing each other is a three-way
// comparison. Each heading carries <P>_STATE, the upstream state as of the
// last poll. Upstream differing from it means upstream changed, and the
// heading follows; upstream agreeing with it while the heading's keyword
// disagrees means a person changed the heading, and upstream follows. Every
// write the poller makes sets the keyword and the state together, so its own
// writes always agree with themselves and can never look like a person's.
//
// The order the writes happen in is forced as well:
//  1. Upstream first (closes and reopens), with nothing written here yet.
//  2. Keyword changes that come from upstream, one at a time through the
//     server's state change, so CLOSED and the logbook follow the file's
//     settings. Each re-reads the file and finds the heading by its id again,
//     since the one before may have added lines above it.
//  3. Titles, tags and properties, bottom of the file first so an edit never
//     moves a heading still to be edited, then new items; one write.
// A state is only recorded in step 3 for a heading whose keyword change in
// step 2 worked. Recording it anyway would leave keyword and state disagreeing,
// which the next poll would read as a person's change and push upstream.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// What a heading needs done to it in this poll.
type plan struct {
	item   Item
	fresh  bool   // item holds upstream's current fields, to update the heading from
	state  string // the state to record; "" leaves the property alone
	status string // a keyword change that came from upstream
}

// Sync polls once. Safe to call from the ticker and the sync endpoint at once.
func (self *Forge) Sync(db common.ODb) common.ForgeSync {
	self.mu.Lock()
	defer self.mu.Unlock()
	res := common.ForgeSync{Name: self.provider, File: self.File, Url: self.baseURL()}
	if self.File == "" {
		res.Msg = "no file: in its yaml"
		return res
	}
	if self.token() == "" {
		res.Msg = fmt.Sprintf("no token: put one in the keyring with `orgs forge token %s`, or set %s",
			self.provider, strings.Join(self.tokenEnvs(), " or "))
		return res
	}
	items, err := self.open()
	if err != nil {
		// Nothing is written on a failed fetch: a partial answer would read as
		// every missing item having gone.
		res.Msg = err.Error()
		return res
	}

	path := self.path()
	lines, _ := readLines(path)
	kw := self.keywordsFor(db, path, lines)
	idKey, stateKey := self.prefix()+"ID", self.prefix()+"STATE"

	plans := map[string]*plan{}
	var order []string
	limited := false
	for _, h := range scan(lines, kw) {
		id := h.props[idKey]
		if id == "" || plans[id] != nil {
			continue
		}
		rec := h.props[stateKey]
		done := kw.done[h.keyword]
		it, inQuery := items[id]
		p := &plan{item: it, fresh: inQuery}
		if !inQuery {
			// Not in any query: closed, merged, or no longer yours. Only asked
			// about when the answer could change something.
			if !(rec == "" || rec == "open" || (rec == "closed" && !done)) || limited {
				continue
			}
			got, err := self.one(id)
			var rl *rateLimited
			switch {
			case errors.Is(err, errNotFound):
				it = Item{ID: id, State: "gone"}
			case errors.As(err, &rl):
				limited = true
				self.logf("%v; the rest waits for the next poll", err)
				continue
			case err != nil:
				self.logf("%s: %v", id, err)
				continue
			default:
				it, p.fresh = got, true
				if it.State == "open" {
					// Still open, but nothing asked for brought it back.
					it.State, p.fresh = "gone", false
				}
			}
			p.item = it
		}
		up := it.State

		switch {
		case rec == "" || up != rec:
			// Upstream changed (or this heading is new to the plugin): the
			// keyword follows it.
			want := self.keyword(stateName(it))
			if want != "" && kw.done[want] != done {
				p.status = want
			}
			p.state = up
		case up == "open" && done:
			// Marked done here.
			if !self.mayClose(it.Kind) || !it.Mine || limited {
				break
			}
			if err := self.pushState(it, false, &res, &limited); err == nil {
				p.state = "closed"
			}
		case up == "closed" && !done && rec == "closed":
			// Moved back to a live keyword here.
			if it.Kind != "issue" || !self.mayClose(it.Kind) || limited {
				break
			}
			if err := self.pushState(it, true, &res, &limited); err == nil {
				p.state = "open"
			}
		}
		plans[id] = p
		order = append(order, id)
	}

	// 2. Keyword changes that came from upstream.
	for _, id := range order {
		p := plans[id]
		if p.status == "" {
			continue
		}
		if !self.changeStatus(path, kw, idKey, id, p.status) {
			p.state = ""
			continue
		}
		switch {
		case p.item.State == "gone":
			res.Gone++
		case kw.done[p.status]:
			res.Closed++
			res.Notes = append(res.Notes, fmt.Sprintf("%s is %s upstream: %s", id, p.item.State, p.status))
		default:
			res.Reopened++
			res.Notes = append(res.Notes, fmt.Sprintf("%s was reopened upstream: %s", id, p.status))
		}
	}

	// 3. Fields, then new items.
	before, existed := readLines(path)
	out := append([]string(nil), before...)
	hs := scan(out, kw)
	edited := map[string]bool{}
	for i := len(hs) - 1; i >= 0; i-- {
		h := hs[i]
		id := h.props[idKey]
		p := plans[id]
		if p == nil || edited[id] {
			continue
		}
		edited[id] = true
		var changed bool
		out, changed = self.updateHeading(out, h, kw, p)
		if changed {
			res.Updated++
		}
	}
	var fresh []Item
	for id, it := range items {
		if plans[id] == nil {
			fresh = append(fresh, it)
		}
	}
	if len(fresh) > 0 {
		if !existed && len(out) == 0 {
			out = self.header(kw)
		}
		out = self.addItems(out, kw, fresh)
		res.Added = len(fresh)
	}
	if !sameStrings(out, before) || (!existed && len(out) > 0) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			res.Msg = err.Error()
			return res
		}
		if err := self.write(path, out); err != nil {
			res.Msg = err.Error()
			return res
		}
	}
	res.Ok = true
	if limited {
		res.Msg = "rate limited: some items wait for the next poll"
	}
	return res
}

// Which keyword setting an item's state uses.
func stateName(it Item) string {
	if it.State == "open" && it.Role == "review" {
		return "review"
	}
	return it.State
}

func (self *Forge) pushState(it Item, open bool, res *common.ForgeSync, limited *bool) error {
	verb := "closed"
	if open {
		verb = "reopened"
	}
	err := self.setOpen(it.ID, it.Kind, open)
	if err != nil {
		var rl *rateLimited
		if errors.As(err, &rl) {
			*limited = true
		}
		self.logf("could not mark %s %s upstream: %v", it.ID, verb, err)
		res.Notes = append(res.Notes, fmt.Sprintf("could not mark %s %s upstream: %v", it.ID, verb, err))
		return err
	}
	res.Pushed++
	res.Notes = append(res.Notes, fmt.Sprintf("%s %s upstream", verb, it.ID))
	return nil
}

// changeStatus sets one heading's keyword: through the server, so it is the
// same state change as any other, or here when the server cannot (the file is
// not under an org directory, or there is no server).
func (self *Forge) changeStatus(path string, kw keywords, idKey, id, to string) bool {
	lines, _ := readLines(path)
	h, ok := findByID(lines, kw, idKey, id)
	if !ok {
		return false
	}
	if self.manager != nil && self.manager.ChangeStatus != nil {
		_, err := self.manager.ChangeStatus(path, h.row, to)
		if err == nil {
			return true
		}
		self.logf("%s: %v; changing the keyword here instead", id, err)
		lines, _ = readLines(path)
		if h, ok = findByID(lines, kw, idKey, id); !ok {
			return false
		}
	}
	lines = closeLocally(lines, h, kw, to, self.clock())
	if err := self.write(path, lines); err != nil {
		self.logf("%s: %v", id, err)
		return false
	}
	return true
}

// updateHeading brings one heading's title, tags and properties up to date.
func (self *Forge) updateHeading(lines []string, h heading, kw keywords, p *plan) ([]string, bool) {
	P := self.prefix()
	vals := map[string]string{}
	keys := []string{P + "STATE"}
	if p.state != "" {
		vals[P+"STATE"] = p.state
	} else {
		vals[P+"STATE"] = h.props[P+"STATE"]
	}
	changed := false
	if p.fresh {
		it := p.item
		title := oneLine(it.Title)
		tags := h.tags
		if self.labelsAsTags() {
			labels := labelTags(it.Labels)
			tags = mergeTags(h.tags, strings.Fields(h.props[P+"LABELS"]), labels)
			keys = append(keys, P+"LABELS")
			vals[P+"LABELS"] = strings.Join(labels, " ")
		}
		if title != "" && (title != h.title || !sameStrings(tags, h.tags)) {
			lines[h.row] = setHeadline(lines[h.row], kw, title, tags)
			changed = true
		}
		keys = append(keys, P+"URL", P+"KIND", P+"AUTHOR", P+"UPDATED")
		vals[P+"URL"], vals[P+"KIND"], vals[P+"AUTHOR"], vals[P+"UPDATED"] = it.URL, it.Kind, it.Author, it.Updated
		for _, k := range keys[1:] {
			if vals[k] == "" && k != P+"LABELS" {
				vals[k] = h.props[k] // an empty answer is not a reason to forget
			}
		}
	}
	lines, propsChanged := setProps(lines, h, keys, vals)
	return lines, changed || propsChanged
}

// The heading for a new item, and the lines under it.
func (self *Forge) itemLines(it Item, lvl int) []string {
	P := self.prefix()
	head := strings.Repeat("*", lvl) + " " + self.keyword(stateName(it)) + " " + oneLine(it.Title)
	var labels []string
	if self.labelsAsTags() {
		labels = labelTags(it.Labels)
		if len(labels) > 0 {
			head += "  :" + strings.Join(labels, ":") + ":"
		}
	}
	keys := []string{P + "ID", P + "URL", P + "KIND", P + "STATE", P + "AUTHOR", P + "UPDATED", P + "LABELS"}
	vals := map[string]string{
		P + "ID": it.ID, P + "URL": it.URL, P + "KIND": it.Kind, P + "STATE": it.State,
		P + "AUTHOR": it.Author, P + "UPDATED": it.Updated, P + "LABELS": strings.Join(labels, " "),
	}
	out, _ := setProps([]string{head}, heading{row: 0, end: 0, ps: -1, pe: -1}, keys, vals)
	if self.Body > 0 {
		n := 0
		for _, l := range strings.Split(strings.ReplaceAll(it.Body, "\r", ""), "\n") {
			l = strings.TrimRight(l, " \t")
			if n == 0 && l == "" {
				continue
			}
			if n >= self.Body {
				break
			}
			// A line starting with a star would be read as a heading.
			if strings.HasPrefix(l, "*") {
				l = " " + l
			}
			out = append(out, l)
			n++
		}
	}
	return out
}

// addItems puts new items into the file: at the end, or at the end of their
// repository's heading (made when it is not there yet).
func (self *Forge) addItems(lines []string, kw keywords, items []Item) []string {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Repo != items[j].Repo {
			return items[i].Repo < items[j].Repo
		}
		_, _, a, _ := splitID(items[i].ID)
		_, _, b, _ := splitID(items[j].ID)
		return a < b
	})
	if self.layout() == "flat" {
		for _, it := range items {
			lines = append(lines, self.itemLines(it, 1)...)
		}
		return lines
	}
	for _, it := range items {
		at := -1
		for _, h := range scan(lines, kw) {
			if h.lvl == 1 && h.title == it.Repo {
				at = h.end + 1
				// Above the blank lines that end the subtree, so they stay
				// between this repository and the next.
				for at-1 > h.row && strings.TrimSpace(lines[at-1]) == "" {
					at--
				}
				break
			}
		}
		if at < 0 {
			lines = append(lines, "* "+it.Repo)
			at = len(lines)
		}
		lines = splice(lines, at, self.itemLines(it, 2))
	}
	return lines
}

// The top of a file the plugin makes itself.
func (self *Forge) header(kw keywords) []string {
	name := "GitHub"
	if self.provider == "gitlab" {
		name = "GitLab"
	}
	out := []string{"#+TITLE: " + name}
	// Keywords the yaml asks for that a file would not know without being told.
	base := newKeywords()
	base.parse(fallbackKeywords)
	var active, done []string
	for _, s := range []string{"open", "review", "closed", "merged", "gone"} {
		k := self.keyword(s)
		if k == "" || base.all[k] {
			continue
		}
		base.all[k] = true
		if s == "open" || s == "review" {
			active = append(active, k)
		} else {
			done = append(done, k)
		}
	}
	if len(active)+len(done) > 0 {
		line := strings.Join(append([]string{"TODO", "NEXT", "WAITING"}, active...), " ") + " | " +
			strings.Join(append([]string{"DONE", "CANCELLED"}, done...), " ")
		out = append(out, "#+TODO: "+line)
	}
	out = append(out,
		fmt.Sprintf("# Kept in step with %s by orgs. Headings are found again by %sID, so notes", self.baseURL(), self.prefix()),
		"# written under them stay; marking an issue DONE here closes it there.",
		"")
	return out
}

// Used when nothing better is known: the server's default keywords.
const fallbackKeywords = "TODO INPROGRESS IN-PROGRESS NEXT BLOCKED PAUSED WAITING PHONE MEETING BACKLOG | DONE CANCELLED"

// keywordsFor is the file's todo keywords: as the server parsed it when it
// has, else from its own #+TODO lines, else the server's defaults. The
// configured keywords are added, so a heading this plugin wrote is always
// read back with its keyword.
func (self *Forge) keywordsFor(db common.ODb, path string, lines []string) keywords {
	kw := newKeywords()
	spec := ""
	if db != nil {
		if f := db.GetFile(path); f != nil && f.Doc != nil && samePath(f.Filename, path) {
			spec = f.Doc.Get("TODO")
		}
	}
	if spec == "" {
		var own []string
		for _, l := range lines {
			if m := todoLineRe.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
				own = append(own, m[1])
			}
		}
		spec = strings.Join(own, "\n")
	}
	if spec == "" {
		spec = fallbackKeywords
	}
	kw.parse(spec)
	for _, s := range []string{"open", "review", "closed", "merged", "gone"} {
		if k := self.keyword(s); k != "" && !kw.all[k] {
			kw.all[k] = true
			if s != "open" && s != "review" {
				kw.done[k] = true
			}
		}
	}
	return kw
}

func (self *Forge) write(path string, lines []string) error {
	if self.manager != nil && self.manager.WriteLines != nil {
		return self.manager.WriteLines(path, lines)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// readLines is the file as lines, and whether it was there at all.
func readLines(path string) ([]string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	text := strings.TrimSuffix(string(b), "\n")
	if text == "" {
		return nil, true
	}
	return strings.Split(text, "\n"), true
}

func labelTags(labels []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range labels {
		if t := labelTag(l); t != "" && !seen[t] {
			out = append(out, t)
			seen[t] = true
		}
	}
	return out
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func samePath(a, b string) bool {
	norm := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		return p
	}
	return norm(a) == norm(b)
}

// Check asks the api who the token belongs to.
func (self *Forge) Check() common.ForgeSync {
	self.mu.Lock()
	defer self.mu.Unlock()
	res := common.ForgeSync{Name: self.provider, File: self.File, Url: self.baseURL()}
	if self.token() == "" {
		res.Msg = fmt.Sprintf("no token: put one in the keyring with `orgs forge token %s`, or set %s",
			self.provider, strings.Join(self.tokenEnvs(), " or "))
		return res
	}
	me, err := self.whoami()
	if err != nil {
		res.Msg = err.Error()
		return res
	}
	res.Ok, res.User = true, me
	return res
}
