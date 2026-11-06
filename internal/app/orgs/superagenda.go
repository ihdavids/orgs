package orgs

// org-super-agenda (features.superAgenda): the agenda's items sorted into
// titled groups. The selectors and the yaml are in the "Grouped agendas"
// section of docs/org-packages.org. Grouping is worked out here, after the
// agenda engine has answered (and been cached), so every client - worg's
// agenda views, `orgs agenda -view`, the txt/html export - draws the same
// groups and none of them knows the rules.

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// superAgendaOther is where what no group takes goes, and its order.
const (
	superAgendaOther      = "Other items"
	superAgendaOtherOrder = 99
)

// agendaGroupsFor is the groups an agenda is sorted into: none while the
// feature is off; own when a request or a view section names some; else the
// yaml's.
func agendaGroupsFor(own []common.AgendaGroup, off bool) []common.AgendaGroup {
	if off || !featureOn("superAgenda") {
		return nil
	}
	if len(own) > 0 {
		return own
	}
	if s := Conf().Server; s != nil {
		return s.Agenda.Groups
	}
	return nil
}

// agendaGroupsParam reads a request's groups= and grouped=: its own groups,
// whether it turned grouping off, and what went wrong reading them.
func agendaGroupsParam(groups, grouped string) ([]common.AgendaGroup, bool, error) {
	switch strings.ToLower(grouped) {
	case "f", "false", "0", "no", "nil":
		return nil, true, nil
	}
	if strings.TrimSpace(groups) == "" {
		return nil, false, nil
	}
	var out []common.AgendaGroup
	if err := json.Unmarshal([]byte(groups), &out); err != nil {
		return nil, false, fmt.Errorf("groups: %v", err)
	}
	return out, false, nil
}

// One thing to sort: a heading and, on a day of the agenda, its entry.
type superAgendaItem struct {
	Node  *common.AgendaNode
	Entry *common.AgendaEntry
}

// What a grouping needs beyond the items: today, a heading's category, and
// what a query finds. Funcs so a test can answer them without a database.
type superAgendaCtx struct {
	Today    string // YYYY-MM-DD
	Category func(n *common.AgendaNode) string
	Query    func(q string) (map[string]bool, error)

	queries map[string]map[string]bool
	regexps map[string]*regexp.Regexp
	errs    []string
}

func newSuperAgendaCtx(today string) *superAgendaCtx {
	return &superAgendaCtx{
		Today:    today,
		Category: func(n *common.AgendaNode) string { return nodeCategory(*n) },
		Query: func(q string) (map[string]bool, error) {
			hits := map[string]bool{}
			err := agendaSections(expandQueryFilters(andQueries(q)), func(sec *org.Section, f *common.OrgFile) {
				hits[sec.Hash] = true
			})
			return hits, err
		},
	}
}

func (c *superAgendaCtx) fail(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	for _, e := range c.errs {
		if e == msg {
			return
		}
	}
	c.errs = append(c.errs, msg)
}

// The problems met, one sentence, or "".
func (c *superAgendaCtx) problems() string {
	if len(c.errs) == 0 {
		return ""
	}
	return "agenda groups: " + strings.Join(c.errs, "; ")
}

func (c *superAgendaCtx) re(pat string) *regexp.Regexp {
	if c.regexps == nil {
		c.regexps = map[string]*regexp.Regexp{}
	}
	if r, ok := c.regexps[pat]; ok {
		return r
	}
	r, err := regexp.Compile(pat)
	if err != nil {
		c.fail("%q is not a regexp: %v", pat, err)
	}
	c.regexps[pat] = r
	return r
}

func (c *superAgendaCtx) hits(q string) map[string]bool {
	if c.queries == nil {
		c.queries = map[string]map[string]bool{}
	}
	if h, ok := c.queries[q]; ok {
		return h
	}
	h, err := c.Query(q)
	if err != nil {
		c.fail("query %q: %v", q, err)
		h = map[string]bool{}
	}
	c.queries[q] = h
	return h
}

// groupAgendaItems sorts items into groups, org-super-agenda's way. Each
// answer's Items are indexes into items, in their order there.
func groupAgendaItems(groups []common.AgendaGroup, items []superAgendaItem, c *superAgendaCtx) []common.AgendaGroupItems {
	type shown struct {
		order, seq int
		g          common.AgendaGroupItems
	}
	taken := make([]bool, len(items))
	out := []shown{}
	add := func(order int, name string, idx []int) {
		if len(idx) > 0 {
			out = append(out, shown{order, len(out), common.AgendaGroupItems{Name: name, Items: idx}})
		}
	}
	for gi, g := range groups {
		if g.Auto != "" {
			buckets := map[string][]int{}
			for i, it := range items {
				if taken[i] {
					continue
				}
				v := c.autoValue(g.Auto, it)
				if v == "" {
					continue
				}
				taken[i] = true
				buckets[v] = append(buckets[v], i)
			}
			if g.Discard {
				continue
			}
			names := make([]string, 0, len(buckets))
			for k := range buckets {
				names = append(names, k)
			}
			sort.Slice(names, func(a, b int) bool { return strings.ToLower(names[a]) < strings.ToLower(names[b]) })
			for _, k := range names {
				add(g.Order, k, buckets[k])
			}
			continue
		}
		idx := []int{}
		for i, it := range items {
			if !taken[i] && c.matches(g, it) {
				taken[i] = true
				idx = append(idx, i)
			}
		}
		if !g.Discard {
			add(g.Order, agendaGroupTitle(g, gi), idx)
		}
	}
	rest := []int{}
	for i := range items {
		if !taken[i] {
			rest = append(rest, i)
		}
	}
	add(superAgendaOtherOrder, superAgendaOther, rest)
	sort.SliceStable(out, func(a, b int) bool { return out[a].order < out[b].order })
	res := make([]common.AgendaGroupItems, len(out))
	for i, s := range out {
		res[i] = s.g
	}
	return res
}

// agendaGroupTitle names a group that has no name: by its first selector, as
// org-super-agenda does.
func agendaGroupTitle(g common.AgendaGroup, i int) string {
	switch {
	case g.Name != "":
		return g.Name
	case g.TimeGrid:
		return "Schedule"
	case len(g.Tag) > 0:
		return "Tagged " + strings.Join(g.Tag, ", ")
	case len(g.Todo) > 0:
		return strings.Join(g.Todo, ", ") + " items"
	case g.Priority != "":
		return "Priority " + g.Priority + " items"
	case g.Deadline != "":
		return "Deadline " + g.Deadline
	case g.Scheduled != "":
		return "Scheduled " + g.Scheduled
	case g.Habit:
		return "Habits"
	case len(g.Category) > 0:
		return strings.Join(g.Category, ", ")
	case g.Query != "":
		return g.Query
	}
	return fmt.Sprintf("Group %d", i+1)
}

// matches is whether a group's selectors take an item. A group with no
// selectors takes nothing (a name alone is not "anything").
func (c *superAgendaCtx) matches(g common.AgendaGroup, it superAgendaItem) bool {
	n := it.Node
	any := false
	check := func(given bool, ok func() bool) {
		if given && !any && ok() {
			any = true
		}
	}
	check(g.Anything, func() bool { return true })
	check(g.TimeGrid, func() bool { return it.Entry != nil && it.Entry.HasTime })
	check(len(g.Tag) > 0, func() bool { return superAgendaAnyFold(g.Tag, n.AllTags) || superAgendaAnyFold(g.Tag, n.Tags) })
	check(len(g.Todo) > 0, func() bool {
		for _, k := range g.Todo {
			if (k == "t" && n.IsTodo) || (k != "t" && n.Status == k) {
				return true
			}
		}
		return false
	})
	check(g.Priority != "", func() bool { return superAgendaPriority(g.Priority, n.Priority) })
	check(g.Deadline != "", func() bool { return c.dayIs(g.Deadline, n.Deadline) })
	check(g.Scheduled != "", func() bool { return c.dayIs(g.Scheduled, n.Scheduled) })
	check(g.Date != "", func() bool {
		if len(n.Timestamps) == 0 {
			return c.dayIs(g.Date, nil)
		}
		for i := range n.Timestamps {
			if c.dayIs(g.Date, &n.Timestamps[i]) {
				return true
			}
		}
		return false
	})
	check(g.Habit, func() bool { return n.IsHabit })
	check(len(g.Category) > 0, func() bool { return superAgendaAnyFold(g.Category, []string{c.Category(n)}) })
	check(g.File != "", func() bool { r := c.re(g.File); return r != nil && r.MatchString(n.Filename) })
	check(len(g.Property) > 0, func() bool {
		for _, p := range g.Property {
			name, want, hasValue := strings.Cut(p, "=")
			v, ok := superAgendaProp(n, strings.TrimSpace(name))
			if ok && (!hasValue || strings.TrimSpace(v) == strings.TrimSpace(want)) {
				return true
			}
		}
		return false
	})
	check(g.Heading != "", func() bool { r := c.re(g.Heading); return r != nil && r.MatchString(n.Headline) })
	check(g.Regexp != "", func() bool {
		r := c.re(g.Regexp)
		if r == nil {
			return false
		}
		if r.MatchString(n.Headline) {
			return true
		}
		for _, v := range n.Props {
			if r.MatchString(v) {
				return true
			}
		}
		return false
	})
	check(g.Children != "", func() bool {
		switch strings.ToLower(g.Children) {
		case "todo":
			return n.Children.Todo > 0
		case "none", "nil":
			return n.Children.Count == 0
		}
		return n.Children.Count > 0
	})
	check(g.EffortMin != "", func() bool {
		e, ok := superAgendaEffort(n)
		return ok && e >= superAgendaMins(g.EffortMin)
	})
	check(g.EffortMax != "", func() bool {
		e, ok := superAgendaEffort(n)
		return ok && e < superAgendaMins(g.EffortMax)
	})
	check(g.Query != "", func() bool { return c.hits(g.Query)[n.Hash] })
	check(len(g.And) > 0, func() bool {
		for _, s := range g.And {
			if !c.matches(s, it) {
				return false
			}
		}
		return true
	})
	check(len(g.Not) > 0, func() bool {
		for _, s := range g.Not {
			if c.matches(s, it) {
				return false
			}
		}
		return true
	})
	return any
}

// dayIs reads past, today, future, any/t, none/nil or a YYYY-MM-DD against a
// date (nil for none).
func (c *superAgendaCtx) dayIs(want string, d *common.AgendaDate) bool {
	w := strings.ToLower(strings.TrimSpace(want))
	if d == nil || len(d.Start) < 10 {
		return w == "none" || w == "nil"
	}
	day := d.Start[:10]
	switch w {
	case "any", "t":
		return true
	case "none", "nil":
		return false
	case "past":
		return day < c.Today
	case "today":
		return day == c.Today
	case "future":
		return day > c.Today
	}
	return day == w
}

// superAgendaPriority reads A, A,B, >B or <B. No cookie counts as B, org's
// default, as the agenda sorts it.
func superAgendaPriority(want, have string) bool {
	p := agendaPriority(have)
	w := strings.ToUpper(strings.TrimSpace(want))
	switch {
	case strings.HasPrefix(w, ">"):
		return p < strings.TrimSpace(w[1:]) // A is higher than B
	case strings.HasPrefix(w, "<"):
		return p > strings.TrimSpace(w[1:])
	}
	for _, x := range strings.Split(w, ",") {
		if strings.TrimSpace(x) == strings.ToUpper(have) && have != "" {
			return true
		}
	}
	return false
}

func superAgendaAnyFold(want, have []string) bool {
	for _, w := range want {
		for _, h := range have {
			if strings.EqualFold(w, h) {
				return true
			}
		}
	}
	return false
}

// A property by name in any case: go-org keeps the case it was written in.
func superAgendaProp(n *common.AgendaNode, name string) (string, bool) {
	for k, v := range n.Props {
		if strings.EqualFold(k, name) {
			return v, true
		}
	}
	return "", false
}

func superAgendaMins(s string) float64 {
	if d := common.ParseDuration(strings.TrimSpace(s)); d != nil {
		return d.Mins
	}
	return 0
}

func superAgendaEffort(n *common.AgendaNode) (float64, bool) {
	v, ok := superAgendaProp(n, "EFFORT")
	if !ok || strings.TrimSpace(v) == "" {
		return 0, false
	}
	d := common.ParseDuration(strings.TrimSpace(v))
	if d == nil {
		return 0, false
	}
	return d.Mins, true
}

// autoValue is what an auto group names an item by, "" for nothing.
func (c *superAgendaCtx) autoValue(by string, it superAgendaItem) string {
	n := it.Node
	key := strings.ToLower(strings.TrimSpace(by))
	if strings.HasPrefix(key, "property:") {
		v, _ := superAgendaProp(n, strings.TrimSpace(by[len("property:"):]))
		return strings.TrimSpace(v)
	}
	switch key {
	case "category":
		return c.Category(n)
	case "tags", "tag":
		tags := n.AllTags
		if len(tags) == 0 {
			tags = n.Tags
		}
		return strings.Join(tags, ", ")
	case "todo", "keyword":
		return n.Status
	case "priority":
		if n.Priority == "" {
			return ""
		}
		return "Priority " + n.Priority
	case "file":
		return strings.TrimSuffix(filepath.Base(n.Filename), filepath.Ext(n.Filename))
	case "dir":
		return filepath.Base(filepath.Dir(n.Filename))
	case "parent":
		if len(n.Olp) >= 2 {
			return n.Olp[len(n.Olp)-2]
		}
	case "outline", "path":
		if len(n.Olp) >= 2 {
			return strings.Join(n.Olp[:len(n.Olp)-1], " / ")
		}
	case "planning":
		switch {
		case n.Scheduled != nil && n.Deadline != nil:
			return "Scheduled, with a deadline"
		case n.Scheduled != nil:
			return "Scheduled"
		case n.Deadline != nil:
			return "Deadline"
		}
	default:
		c.fail("no auto grouping %q (category, tags, todo, priority, file, dir, parent, outline, planning or property:NAME)", by)
	}
	return ""
}

// groupAgendaDays fills each day's Groups. The problems met, or "".
func groupAgendaDays(a *common.AgendaResult, groups []common.AgendaGroup) string {
	if len(groups) == 0 {
		return ""
	}
	c := newSuperAgendaCtx(a.Today)
	for d := range a.Days {
		day := &a.Days[d]
		// One item per entry, so an item's index is the entry's.
		items := make([]superAgendaItem, len(day.Entries))
		for i := range day.Entries {
			e := &day.Entries[i]
			n := &common.AgendaNode{}
			if e.Node >= 0 && e.Node < len(a.Nodes) {
				n = &a.Nodes[e.Node]
			}
			items[i] = superAgendaItem{Node: n, Entry: e}
		}
		day.Groups = groupAgendaItems(groups, items, c)
	}
	return c.problems()
}

// groupAgendaNodes groups a list. The problems met, or "".
func groupAgendaNodes(nodes []common.AgendaNode, groups []common.AgendaGroup, today string) ([]common.AgendaGroupItems, string) {
	if len(groups) == 0 {
		return nil, ""
	}
	c := newSuperAgendaCtx(today)
	items := make([]superAgendaItem, len(nodes))
	for i := range nodes {
		items[i] = superAgendaItem{Node: &nodes[i]}
	}
	return groupAgendaItems(groups, items, c), c.problems()
}
