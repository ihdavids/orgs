package orgs

// Custom agenda views and block agendas (org manual 11.6, 11.7): a view is
// sections shown one after another - a day or a week of the agenda, open
// tasks, a tag/property match, a text search, stuck projects - as org's
// org-agenda-custom-commands builds them. Each section's answer comes from the
// engines that already answer those questions (buildAgenda, agendaSections),
// so a view and /agenda cannot disagree.
//
// Views come from three places, listed in this order: org's own dispatcher
// views (a week, t open tasks, n both, # stuck projects), agenda.views in the
// yaml, and the person's own (the extensions file, POST /ext/agenda/views).
// The first two are read only. GET /agenda/view answers one, as json or
// exported (format=txt, html or ics), as org-agenda-write does.

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/ics"
	"github.com/ihdavids/orgs/internal/common"
)

// builtinAgendaViews are org's dispatcher letters.
func builtinAgendaViews() []common.AgendaView {
	views := []common.AgendaView{
		{Name: "Week agenda", Key: "a", ReadOnly: true, Sections: []common.AgendaViewSection{{Kind: "agenda", Days: 7}}},
		{Name: "All open tasks", Key: "t", ReadOnly: true, Sections: []common.AgendaViewSection{{Kind: "todo"}}},
		{Name: "Agenda and all open tasks", Key: "n", ReadOnly: true, Sections: []common.AgendaViewSection{{Kind: "agenda", Days: 7}, {Kind: "todo"}}},
		{Name: "Stuck projects", Key: "#", ReadOnly: true, Sections: []common.AgendaViewSection{{Kind: "stuck"}}},
	}
	// org-review's agenda: today, then what is due for review (orgreview.go).
	if featureOn("review") {
		views = append(views, common.AgendaView{Name: "Today and due for review", Key: "R", ReadOnly: true,
			Sections: []common.AgendaViewSection{{Kind: "agenda", Days: 1}, {Kind: "tags", Title: "Due for review", Query: "ReviewDue()"}}})
	}
	return views
}

// agendaViewsFor is every view a person can open.
func agendaViewsFor(username string) []common.AgendaView {
	out := builtinAgendaViews()
	if s := Conf().Server; s != nil {
		for _, v := range s.Agenda.Views {
			v.ReadOnly = true
			out = append(out, v)
		}
	}
	return append(out, GetExtensions().GetAgendaViews(username)...)
}

// findAgendaView is the view by name or key.
func findAgendaView(username, which string) (common.AgendaView, bool) {
	for _, v := range agendaViewsFor(username) {
		if v.Name == which || (v.Key != "" && v.Key == which) {
			return v, true
		}
	}
	return common.AgendaView{}, false
}

func viewSectionTitle(s common.AgendaViewSection) string {
	if s.Title != "" {
		return s.Title
	}
	switch s.Kind {
	case "agenda":
		if s.Days == 7 {
			return "Week agenda"
		}
		if s.Days <= 1 {
			return "Day agenda"
		}
		return fmt.Sprintf("%d-day agenda", s.Days)
	case "todo":
		if s.Keywords != "" {
			return "Tasks: " + s.Keywords
		}
		return "Global list of open tasks"
	case "tags":
		return "Headings matching " + s.Query
	case "search":
		return "Search: " + s.Query
	case "stuck":
		return "Stuck projects"
	}
	return s.Kind
}

// and joins full-form queries; an empty one means everything.
func andQueries(qs ...string) string {
	parts := []string{}
	for _, q := range qs {
		if q = strings.TrimSpace(q); q != "" {
			full, err := shortToFull(q)
			if err != nil {
				full = q
			}
			parts = append(parts, "("+full+")")
		}
	}
	return strings.Join(parts, " && ")
}

// sectionQuery is what a non-agenda section matches.
func sectionQuery(s common.AgendaViewSection) (string, error) {
	switch s.Kind {
	case "todo":
		q := "HasAStatus() && !IsDone() && !IsArchived()"
		if kws := strings.FieldsFunc(s.Keywords, func(r rune) bool { return r == '|' || r == ' ' || r == ',' }); len(kws) > 0 {
			ors := []string{}
			for _, k := range kws {
				ors = append(ors, qcall("IsStatus", k))
			}
			q += " && (" + strings.Join(ors, " || ") + ")"
		}
		return andQueries(q, s.Query), nil
	case "tags":
		if strings.TrimSpace(s.Query) == "" {
			return "", fmt.Errorf("a tags section needs a query")
		}
		return andQueries(s.Query), nil
	case "search":
		if strings.TrimSpace(s.Query) == "" {
			return "", fmt.Errorf("a search section needs words to find")
		}
		pat := "(?i)" + regexpQuote(s.Query)
		return "!IsArchived() && (" + qcall("MatchHeadline", pat) + " || " + qcall("MatchContent", pat) + ")", nil
	case "stuck":
		return andQueries("IsBlockedProject() && !IsArchived()", s.Query), nil
	}
	return "", fmt.Errorf("no section kind %q: agenda, todo, tags, search or stuck", s.Kind)
}

func regexpQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, ".", `\.`, "+", `\+`, "*", `\*`, "?", `\?`, "(", `\(`, ")", `\)`, "[", `\[`, "]", `\]`, "{", `\{`, "}", `\}`, "^", `\^`, "$", `\$`, "|", `\|`).Replace(s)
}

// runAgendaView answers every section of a view.
func runAgendaView(v common.AgendaView, now time.Time) common.AgendaViewResult {
	res := common.AgendaViewResult{Ok: true, Name: v.Name, Parts: []common.AgendaViewPart{}}
	for _, s := range v.Sections {
		part := common.AgendaViewPart{Title: viewSectionTitle(s), Kind: s.Kind}
		if s.Kind == "agenda" {
			days := s.Days
			if days <= 0 {
				days = 1
			}
			today := agendaDayNum(now)
			from := today
			if strings.EqualFold(s.Start, "monday") || strings.EqualFold(s.Start, "week") {
				from = today - (int(now.Weekday())+6)%7
			}
			rules := agendaRules{From: from, To: from + days - 1, Today: today, WarnDays: agendaWarnDays()}
			a, err := buildAgenda(expandQueryFilters(andQueries(s.Query)), rules, now)
			if err != nil {
				part.Msg = err.Error()
			} else {
				if groups := agendaGroupsFor(s.Groups, s.NoGroups); len(groups) > 0 {
					part.Msg = groupAgendaDays(&a, groups)
				}
				part.Agenda = &a
			}
			res.Parts = append(res.Parts, part)
			continue
		}
		q, err := sectionQuery(s)
		if err != nil {
			part.Msg = err.Error()
			res.Parts = append(res.Parts, part)
			continue
		}
		nodes, err := buildAgendaNodes(expandQueryFilters(q), now)
		if err != nil {
			part.Msg = err.Error()
		}
		part.Nodes = nodes.Nodes
		if part.Nodes == nil {
			part.Nodes = []common.AgendaNode{}
		}
		if groups := agendaGroupsFor(s.Groups, s.NoGroups); len(groups) > 0 && err == nil {
			part.Groups, part.Msg = groupAgendaNodes(part.Nodes, groups, agendaDayStr(agendaDayNum(now)))
		}
		res.Parts = append(res.Parts, part)
	}
	return res
}

// ---------------------------------------------------------------------------
// Export: org-agenda-write's text, a page, a calendar
// ---------------------------------------------------------------------------

func nodeCategory(n common.AgendaNode) string {
	if sec := GetDb().FindByHash(n.Hash); sec != nil {
		if f := GetDb().FileFromSection(sec); f != nil {
			return categoryOf(f, sec)
		}
	}
	return strings.TrimSuffix(filepath.Base(n.Filename), filepath.Ext(n.Filename))
}

// agendaLine is one item as org's agenda draws it:
// "  work:       10:00-11:00 TODO Write the report  :urgent:".
func agendaLine(n common.AgendaNode, when string) string {
	cat := nodeCategory(n) + ":"
	title := strings.TrimSpace(strings.Join([]string{n.Status, priorityCookie(n.Priority), n.Headline}, " "))
	title = strings.Join(strings.Fields(title), " ")
	line := fmt.Sprintf("  %-12s%s%s", cat, when, title)
	if len(n.Tags) > 0 {
		line += "  :" + strings.Join(n.Tags, ":") + ":"
	}
	return line
}

func priorityCookie(p string) string {
	if p == "" {
		return ""
	}
	return "[#" + p + "]"
}

// agendaViewText is the view as org-agenda-write writes a .txt.
func agendaViewText(r common.AgendaViewResult) string {
	var b strings.Builder
	for i, p := range r.Parts {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p.Title + "\n")
		if p.Msg != "" {
			b.WriteString("  (" + p.Msg + ")\n")
		}
		if p.Agenda != nil {
			for _, d := range p.Agenda.Days {
				day, _ := time.Parse("2006-01-02", d.Date)
				b.WriteString(day.Format("Monday") + strings.Repeat(" ", max(1, 10-len(day.Format("Monday")))) + day.Format("2 January 2006") + "\n")
				for _, at := range agendaTextOrder(len(d.Entries), d.Groups) {
					if at.group != "" {
						b.WriteString(agendaGroupLine(at.group))
						continue
					}
					e := d.Entries[at.i]
					if e.Node < 0 || e.Node >= len(p.Agenda.Nodes) {
						continue
					}
					when := ""
					switch {
					case e.HasTime && len(e.Start) >= 16:
						when = e.Start[11:16]
						if len(e.End) >= 16 && e.End != e.Start {
							when += "-" + e.End[11:16]
						}
						when += " "
					case e.Kind == "scheduled" && e.Late > 0:
						when = fmt.Sprintf("Sched.%2dx: ", e.Late)
					case e.Kind == "scheduled":
						when = "Scheduled: "
					case e.Kind == "deadline" && e.Late > 0:
						when = fmt.Sprintf("%2d d. ago: ", e.Late)
					case e.Kind == "deadline" && e.DueIn > 0:
						when = fmt.Sprintf("In %3d d.: ", e.DueIn)
					case e.Kind == "deadline":
						when = "Deadline:  "
					}
					n := p.Agenda.Nodes[e.Node]
					if e.Text != "" {
						n.Headline = e.Text
					}
					b.WriteString(agendaLine(n, when) + "\n")
				}
			}
			continue
		}
		for _, at := range agendaTextOrder(len(p.Nodes), p.Groups) {
			if at.group != "" {
				b.WriteString(agendaGroupLine(at.group))
				continue
			}
			b.WriteString(agendaLine(p.Nodes[at.i], "") + "\n")
		}
	}
	return b.String()
}

// One line of agendaViewText's: an item (i) or a group's heading.
type agendaTextAt struct {
	i     int
	group string
}

// agendaTextOrder is the items in the order they are written: as they are,
// or group by group under each group's name. A discarded item is not in any
// group and so is not written.
func agendaTextOrder(n int, groups []common.AgendaGroupItems) []agendaTextAt {
	out := []agendaTextAt{}
	if len(groups) == 0 {
		for i := 0; i < n; i++ {
			out = append(out, agendaTextAt{i: i})
		}
		return out
	}
	for _, g := range groups {
		out = append(out, agendaTextAt{group: g.Name})
		for _, i := range g.Items {
			if i >= 0 && i < n {
				out = append(out, agendaTextAt{i: i})
			}
		}
	}
	return out
}

// agendaGroupLine is a group's heading the way org-super-agenda writes one:
// the name, a space in, between the items.
func agendaGroupLine(name string) string {
	return " " + name + "\n"
}

// agendaViewHTML is the text as a page, each item linked to worg.
func agendaViewHTML(r common.AgendaViewResult) string {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><title>" + html.EscapeString(r.Name) + "</title>\n")
	b.WriteString(`<style>body{font:14px/1.5 ui-monospace,Menlo,monospace;max-width:60rem;margin:2rem auto;padding:0 1rem}h2{font:600 1rem system-ui;margin:1.5rem 0 .25rem;border-bottom:1px solid #ccc}a{color:inherit;text-decoration:none}a:hover{text-decoration:underline}.day{font-weight:600;margin-top:.5rem}.group{font:600 .9rem system-ui;margin:.4rem 0 0 .5rem;opacity:.8}pre{margin:0}</style>` + "\n")
	b.WriteString("<h1 style=\"font:600 1.3rem system-ui\">" + html.EscapeString(r.Name) + "</h1>\n")
	for _, line := range strings.Split(strings.TrimRight(agendaViewText(r), "\n"), "\n") {
		switch {
		case line == "":
		case isWeekdayLine(line):
			b.WriteString("<div class=\"day\">" + html.EscapeString(line) + "</div>\n")
		case !strings.HasPrefix(line, " "):
			b.WriteString("<h2>" + html.EscapeString(line) + "</h2>\n")
		case !strings.HasPrefix(line, "  "): // a group's name (agendaGroupLine)
			b.WriteString("<div class=\"group\">" + html.EscapeString(strings.TrimSpace(line)) + "</div>\n")
		default:
			b.WriteString("<pre>" + html.EscapeString(line) + "</pre>\n")
		}
	}
	return b.String()
}

func isWeekdayLine(l string) bool {
	for _, d := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"} {
		if strings.HasPrefix(l, d+" ") {
			return true
		}
	}
	return false
}

// agendaViewICS is every dated heading in the view as a calendar (the ics
// exporter's events, so it reads as orgs export -f ics does).
func agendaViewICS(r common.AgendaViewResult, now time.Time) string {
	seen := map[string]bool{}
	secs := []*org.Section{}
	add := func(n common.AgendaNode) {
		if seen[n.Hash] {
			return
		}
		seen[n.Hash] = true
		if s := GetDb().FindByHash(n.Hash); s != nil {
			secs = append(secs, s)
		}
	}
	for _, p := range r.Parts {
		if p.Agenda != nil {
			for _, n := range p.Agenda.Nodes {
				add(n)
			}
		}
		for _, n := range p.Nodes {
			add(n)
		}
	}
	exp := &ics.Exporter{ScheduledAsEvents: true, DeadlinesAsEvents: true}
	return exp.Calendar(secs, now).String()
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

func (self *ExtensionsConfig) GetAgendaViews(username string) []common.AgendaView {
	if self == nil {
		return []common.AgendaView{}
	}
	self.mu.RLock()
	defer self.mu.RUnlock()
	if u, ok := self.Users[username]; ok && u.AgendaViews != nil {
		return u.AgendaViews
	}
	return []common.AgendaView{}
}

func (self *ExtensionsConfig) SetAgendaViews(username string, views []common.AgendaView) error {
	if self == nil {
		return fmt.Errorf("this server keeps no per-user settings")
	}
	self.mu.Lock()
	defer self.mu.Unlock()
	if views == nil {
		views = []common.AgendaView{}
	}
	self.getUser(username).AgendaViews = views
	return self.save()
}

/* SDOC: API
* GET /agenda/views — Every Agenda View
* POST /ext/agenda/views — Replace The Person's Own

	=GET= lists org's built-in views (=a= week, =t= open tasks, =n= both, =#=
	stuck projects), the yaml's =agenda: views:= and the person's own; the
	first two say =ReadOnly=. =POST= takes the person's whole list. A view is
	={Name, Key, Sections: [{Kind, Title, Query, Days, Start, Keywords}]}=,
	=Kind= one of =agenda= (=Days=, =Start: monday=), =todo= (=Keywords:
	"NEXT|WAITING"=), =tags= (=Query= in any form, org match strings
	included), =search= (words) and =stuck=; =Query= narrows any of them.
EDOC */
func RequestAgendaViews(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agendaViewsFor(GetUsername(r)))
}

func PostAgendaViews(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	username := GetUsername(r)
	body, _ := io.ReadAll(r.Body)
	var views []common.AgendaView
	if err := json.Unmarshal(body, &views); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Msg: err.Error()})
		return
	}
	taken := map[string]bool{}
	for _, v := range agendaViewsFor("") {
		taken[v.Name], taken["key:"+v.Key] = true, v.Key != ""
	}
	seen := map[string]bool{}
	for _, v := range views {
		msg := ""
		switch {
		case strings.TrimSpace(v.Name) == "":
			msg = "every view needs a name"
		case seen[v.Name] || taken[v.Name]:
			msg = fmt.Sprintf("there is already a view called %q", v.Name)
		case v.Key != "" && taken["key:"+v.Key]:
			msg = fmt.Sprintf("the key %q is taken by a built-in view", v.Key)
		case len(v.Sections) == 0:
			msg = fmt.Sprintf("view %q has no sections", v.Name)
		}
		for _, s := range v.Sections {
			if s.Kind == "agenda" {
				continue
			}
			if _, err := sectionQuery(s); err != nil && msg == "" {
				msg = fmt.Sprintf("view %q: %v", v.Name, err)
			}
		}
		if msg != "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(common.ResultMsg{Msg: msg})
			return
		}
		seen[v.Name] = true
	}
	if err := GetExtensions().SetAgendaViews(username, views); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.ResultMsg{Msg: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: "saved"})
}

/* SDOC: API
* GET /agenda/view — One Agenda View, Answered Or Exported

	=?name=Week agenda= or =?name=n= (a view's name or key). Answers
	={Ok, Name, Parts: [{Title, Kind, Agenda, Nodes, Msg}]}=: an =agenda=
	section's =Agenda= is =/agenda='s answer for its days, the others' =Nodes=
	the headings they match. =format=txt= is the view as org-agenda-write
	writes text, =format=html= that as a page, =format=ics= every dated
	heading in it as a calendar.
EDOC */
func RequestAgendaView(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v, ok := findAgendaView(GetUsername(r), q.Get("name"))
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		names := []string{}
		for _, v := range agendaViewsFor(GetUsername(r)) {
			names = append(names, v.Name)
		}
		sort.Strings(names)
		json.NewEncoder(w).Encode(common.AgendaViewResult{Msg: fmt.Sprintf("no agenda view %q; there are %s", q.Get("name"), strings.Join(names, ", ")), Parts: []common.AgendaViewPart{}})
		return
	}
	now := time.Now()
	res := runAgendaView(v, now)
	switch q.Get("format") {
	case "txt", "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, agendaViewText(res))
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, agendaViewHTML(res))
	case "ics":
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		io.WriteString(w, agendaViewICS(res, now))
	default:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	}
}
