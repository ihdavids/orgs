package orgs

// The assistant in worg's views (components/ViewAssistant.tsx): the column
// view, a gantt chart, a kanban board and search results. A turn carries
// the view (assistantReq.View): what it shows (a query, or a file for the
// column view), what it calls itself, and whether the user lets the
// assistant change headings.
//
// - chatgpt, gemini and local get tools (viewTools, an assistantToolset for
//   openAIChat): list, find (the query language), read a heading, and when
//   allowed set a keyword, property, tags, priority or date, add a heading,
//   make one wait for another, and suggest a query for the view to use.
//   claude gets the headings in its prompt and answers without changing
//   anything; claudecode and ollama are writing mode's only.
// - Headings are named to the model by ids made for the turn (h1, h2...),
//   each kept as file + outline path and looked up again at every call: a
//   write changes the hashes of the headings after it (Traps: hashes), and a
//   turn often makes several.
// - Every file a turn writes is kept as it was before the first write and
//   as the turn left it; POST /assistant/undo puts them back only if each
//   still reads as the turn left it (as /review/undo does), so an undo never
//   clobbers an edit made since.
// - Each write is reported as {"tool": what, "wrote": file}, a suggested
//   query as {"query": q, "why": ...}, and a turn that wrote ends with
//   {"done": true, "undo": token}.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/orgdate"
)

type assistantView struct {
	Kind  string // columns, gantt, kanban, search
	Title string // what the view calls what it shows: a board's name, a file
	// What it shows: a query (gantt, kanban, search), or a file (columns).
	Query string
	File  string
	// columns: the columns shown; kanban: the board's columns.
	Columns []string
	// Anything else the view knows that the assistant should: how a kanban
	// card's column is decided, the chart's lanes.
	About string
	// The user lets it change headings.
	Edit bool
	// Headings the user has picked (hashes): a search's selection, a
	// gantt chart's task, a board's card.
	Selected []string
}

// The backends a view may use, and those that get tools there.
func assistantViewOffered(name string) bool {
	return name == "claude" || name == "chatgpt" || name == "gemini" || name == "local"
}

func assistantViewTools(name string) bool {
	return (name == "chatgpt" || name == "gemini" || name == "local") && assistantHasTools(name)
}

const viewRows = 150      // headings in the prompt, and per list_headings page
const viewChars = 30000   // and at most this much of them
const viewRounds = 30     // tool rounds (openAIRounds) is the loop's own cap
const viewReadLines = 300 // read_heading

// ---------------------------------------------------------------------------
// The turn
// ---------------------------------------------------------------------------

func assistantViewTurn(w http.ResponseWriter, r *http.Request, name string, b *common.AssistantBackend, key, model string, req *assistantReq) {
	v := req.View
	switch v.Kind {
	case "columns", "gantt", "kanban", "search":
	default:
		writingJson(w, common.ResultMsg{Ok: false, Msg: "no view " + v.Kind})
		return
	}
	if v.Kind == "columns" {
		if GetDb().FindByFile(v.File) == nil {
			writingJson(w, common.ResultMsg{Ok: false, Msg: v.File + " is not an org file this server knows"})
			return
		}
	} else if strings.TrimSpace(v.Query) == "" {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "the view has no query"})
		return
	}
	send, emit := ndjsonSender(w)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	tools := assistantViewTools(name)
	vt := newViewTools(v, tools && v.Edit, send)
	snapshot, err := vt.snapshot()
	if err != nil {
		send(map[string]string{"error": err.Error()})
		return
	}
	system := viewSystemPrompt(v, tools, tools && v.Edit)
	var ts assistantToolset
	if tools {
		ts = vt
	}
	var stop string
	switch name {
	case "claude":
		stop, err = assistantClaude(ctx, key, b, model, system, snapshot, req.Messages, emit)
	case "chatgpt":
		base := strings.TrimRight(b.Url, "/")
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		stop, err = openAIChat(ctx, openAIConn{base: base, key: key, model: model}, system+"\n\n"+snapshot, req.Messages, ts, emit, send)
	case "gemini":
		base := strings.TrimRight(b.Url, "/")
		if base == "" {
			base = geminiOpenAIBase
		}
		stop, err = openAIChat(ctx, openAIConn{base: base, key: key, model: model}, system+"\n\n"+snapshot, req.Messages, ts, emit, send)
	case "local":
		var conn openAIConn
		var info *localModel
		if conn, info, err = localConn(ctx, model, req.NumCtx, send); err == nil {
			ts = localToolsFor(info, conn, ts, send)
			localContextWarn(info, model, req.NumCtx, system+snapshot, req.Messages, send)
			stop, err = openAIChat(ctx, conn, system+"\n\n"+snapshot, req.Messages, ts, emit, send)
		}
	}
	token := vt.finish(GetUsername(r))
	if err != nil {
		msg := assistantErrMsg(name, err)
		if token != "" {
			send(map[string]any{"error": msg, "undo": token})
		} else {
			send(map[string]string{"error": msg})
		}
		return
	}
	done := map[string]any{"done": true, "model": model, "stop": stop}
	if token != "" {
		done["undo"] = token
	}
	send(done)
}

var viewKindSay = map[string]string{
	"columns": "the column view of %s: a table of its headings, one row each, with the columns %s. A parent's EFFORT (a duration: 30min, 2h, 1d, 1w) and CLOCKSUM show the total of everything under it, so estimates go on the tasks themselves, not on their parents.",
	"gantt":   "a Gantt chart of the headings the query %s finds. A task's bar starts at its SCHEDULED date (or its plain timestamp) and lasts its EFFORT; its AFTER property names the one task it waits for (set_after), and the children of a heading with the property ORDERED: t run one after another. Lanes come from a SECTION or ASSIGNED property.",
	"kanban":  "a kanban board of the headings the query %s finds, one card each.",
	"search":  "the results of the search %s, one heading a row.",
}

func viewSystemPrompt(v *assistantView, tools, edit bool) string {
	var b strings.Builder
	b.WriteString("You are an assistant inside orgs, a tool for Org mode notes, tasks and projects. The user is looking at ")
	what := v.Query
	if v.Kind == "columns" {
		what = filepath.Base(v.File)
	}
	what = fmt.Sprintf("%q", what)
	if v.Title != "" {
		what += fmt.Sprintf(" (%s)", v.Title)
	}
	cols := strings.Join(v.Columns, ", ")
	if cols == "" {
		cols = "ITEM, TODO, EFFORT"
	}
	if v.Kind == "columns" {
		fmt.Fprintf(&b, viewKindSay[v.Kind], what, cols)
	} else {
		fmt.Fprintf(&b, viewKindSay[v.Kind], what)
	}
	if v.Kind == "kanban" && len(v.Columns) > 0 {
		fmt.Fprintf(&b, " Its columns are %s.", cols)
	}
	if v.About != "" {
		b.WriteString(" " + strings.TrimSpace(v.About))
	}
	b.WriteString(" They talk to you in a panel beside it.\n\n")
	b.WriteString("Each heading listed has an id like h12. Use ids in tool calls; when you talk to the user, name headings by their title, never by id. Answer briefly in plain text: short paragraphs or a short list, no Markdown headings, tables or bold. Never say a heading says something you have not seen. Dates are written YYYY-MM-DD.\n\n")
	switch {
	case !tools:
		b.WriteString("You cannot look anything up or change anything: the headings below are all you have, and they may be only the first of them. When something should change, say what, and the user will do it.\n")
	case edit:
		b.WriteString("You can look headings up with list_headings, find_headings and read_heading, and change them with set_status, set_property, set_tags, set_priority, set_date, add_heading and set_after. Change only what was asked, and look before you change. The user sees each change and can undo the whole turn. When you have changed things, say in a short list what you changed.\n")
	default:
		b.WriteString("You can look headings up with list_headings, find_headings and read_heading, but the user has not let you change anything: when something should change, say what, and they will do it (or let you, with the pencil in the panel).\n")
	}
	if tools && v.Kind != "columns" {
		b.WriteString("\nWhen the user asks for a query, or to see or find different headings, always offer one with suggest_query (find_headings only looks; it does not change the view). suggest_query checks it, and the user can put it in the view with one click. Say the query in your answer too.\n")
	}
	if tools {
		b.WriteString("\n" + queryLanguageHelp())
	}
	return b.String()
}

func queryLanguageHelp() string {
	return `Queries are expressions over each heading: functions joined with && (and), || (or), ! (not) and brackets; strings in double quotes; a {{Name}} is a saved filter. Examples:
  IsTask() && HasTags("work")
  IsTodo() && DeadlineWithin(7)
  InFile("plan") && !IsDone() && !HasProperty("EFFORT")
  MatchHeadline("(?i)release") || MatchProperty("ASSIGNED", "^sam")
  IsStatus("WAITING") && OlderThan(14)
The functions: ` + strings.Join(queryFunctionNames(), ", ") + ".\n"
}

// ---------------------------------------------------------------------------
// The tools
// ---------------------------------------------------------------------------

type viewRef struct {
	file string // as the database names it
	olp  string
}

type viewTools struct {
	v    *assistantView
	edit bool
	send func(any)
	mu   sync.Mutex
	ids  map[string]viewRef
	keys map[string]string // file\x00olp -> id
	n    int
	// Each file written: as it was before the turn's first write to it.
	before map[string][]byte
	order  []string
	did    []string
}

func newViewTools(v *assistantView, edit bool, send func(any)) *viewTools {
	return &viewTools{v: v, edit: edit, send: send, ids: map[string]viewRef{}, keys: map[string]string{}, before: map[string][]byte{}}
}

func (t *viewTools) idFor(sec *org.Section, f *common.OrgFile) string {
	key := f.Filename + "\x00" + olpString(outlineOf(sec))
	t.mu.Lock()
	defer t.mu.Unlock()
	if id, ok := t.keys[key]; ok {
		return id
	}
	t.n++
	id := fmt.Sprintf("h%d", t.n)
	t.keys[key] = id
	t.ids[id] = viewRef{file: f.Filename, olp: olpString(outlineOf(sec))}
	return id
}

// A heading by its id, as the file is now.
func (t *viewTools) resolve(id string) (*org.Section, *common.OrgFile, error) {
	t.mu.Lock()
	ref, ok := t.ids[strings.TrimSpace(id)]
	t.mu.Unlock()
	if !ok {
		return nil, nil, fmt.Errorf("no heading %q: use an id from list_headings or find_headings", id)
	}
	f, sec := GetDb().GetFromTarget(&common.Target{Type: "file+olp", Filename: ref.file, Id: ref.olp}, false)
	if f == nil || sec == nil || sec.Headline == nil {
		return nil, nil, fmt.Errorf("%s is not there any more (renamed, moved or removed)", id)
	}
	return sec, f, nil
}

// The headings the view shows, in its order.
func (t *viewTools) set() ([]*org.Section, []*common.OrgFile, error) {
	var secs []*org.Section
	var files []*common.OrgFile
	if t.v.Kind == "columns" {
		f := GetDb().FindByFile(t.v.File)
		if f == nil || f.Doc == nil || f.Doc.Outline.Section == nil {
			return nil, nil, fmt.Errorf("%s is not an org file this server knows", t.v.File)
		}
		var walk func(s *org.Section)
		walk = func(s *org.Section) {
			for _, c := range s.Children {
				if c.Headline != nil {
					secs = append(secs, c)
					files = append(files, f)
				}
				walk(c)
			}
		}
		walk(f.Doc.Outline.Section)
		return secs, files, nil
	}
	err := queryEachSection(&common.StringQuery{Query: t.v.Query}, func(s *org.Section, f *common.OrgFile) {
		if s.Headline != nil {
			secs = append(secs, s)
			files = append(files, f)
		}
	})
	return secs, files, err
}

func (t *viewTools) row(sec *org.Section, f *common.OrgFile) string {
	h := sec.Headline
	var b strings.Builder
	if t.v.Kind == "columns" && h.Lvl > 1 {
		b.WriteString(strings.Repeat("  ", h.Lvl-1))
	}
	b.WriteString(t.idFor(sec, f))
	if h.Status != "" {
		b.WriteString(" " + h.Status)
	}
	if h.Priority != "" {
		b.WriteString(" [#" + h.Priority + "]")
	}
	b.WriteString(" " + common.GetSectionTitle(sec))
	if len(h.Tags) > 0 {
		b.WriteString(" :" + strings.Join(h.Tags, ":") + ":")
	}
	if h.Scheduled != nil && h.Scheduled.Date != nil {
		b.WriteString(" | SCHEDULED " + viewDate(h.Scheduled.Date))
	}
	if h.Deadline != nil && h.Deadline.Date != nil {
		b.WriteString(" | DEADLINE " + viewDate(h.Deadline.Date))
	}
	if h.Properties != nil {
		props := []string{}
		for _, p := range h.Properties.Properties {
			if p[0] == "ID" || p[0] == "CUSTOM_ID" {
				continue
			}
			val := p[1]
			if len(val) > 80 {
				val = val[:80] + "…"
			}
			props = append(props, p[0]+"="+val)
		}
		if len(props) > 0 {
			b.WriteString(" | " + strings.Join(props, " "))
		}
	}
	if t.v.Kind != "columns" {
		b.WriteString(" | in " + filepath.Base(f.Filename))
		if sec.Parent != nil && sec.Parent.Headline != nil && h.Lvl > 1 {
			b.WriteString(" under “" + common.GetSectionTitle(sec.Parent) + "”")
		}
	}
	return b.String()
}

func viewDate(d *org.OrgDate) string {
	s := ganttDateStr(*d)
	if s == "" {
		return d.ToString()
	}
	return s
}

// Rows from start (0-based), at most viewRows and viewChars of them.
func (t *viewTools) rows(secs []*org.Section, files []*common.OrgFile, start int) (string, int) {
	var b strings.Builder
	n := 0
	for i := start; i < len(secs) && n < viewRows; i++ {
		r := t.row(secs[i], files[i])
		if b.Len()+len(r) > viewChars && n > 0 {
			break
		}
		b.WriteString(r + "\n")
		n++
	}
	return b.String(), n
}

// What goes in the prompt: the first of the view's headings, and what the
// user has picked.
func (t *viewTools) snapshot() (string, error) {
	secs, files, err := t.set()
	if err != nil {
		return "", err
	}
	text, n := t.rows(secs, files, 0)
	var b strings.Builder
	switch {
	case len(secs) == 0:
		b.WriteString("The view shows no headings.\n")
	case n < len(secs):
		fmt.Fprintf(&b, "The view shows %d headings; these are the first %d (list_headings with start gives more):\n", len(secs), n)
	default:
		fmt.Fprintf(&b, "The view shows %d headings:\n", len(secs))
	}
	b.WriteString(text)
	picked := []string{}
	for _, h := range t.v.Selected {
		if sec := GetDb().FindByHash(h); sec != nil && sec.Headline != nil {
			if f := GetDb().FileFromSection(sec); f != nil {
				picked = append(picked, t.row(sec, f))
			}
		}
	}
	if len(picked) > 0 {
		b.WriteString("\nThe user has picked:\n" + strings.Join(picked, "\n") + "\n")
	}
	return b.String(), nil
}

func (t *viewTools) toolDefs() []codeTool {
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	num := func(d string) map[string]any { return map[string]any{"type": "integer", "description": d} }
	obj := func(props map[string]any, req ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": append([]string{}, req...)}
	}
	id := str("The heading's id, like h12.")
	out := []codeTool{
		{"list_headings", fmt.Sprintf("The headings the view shows, %d at a time, as id, keyword, priority, title, tags, dates and properties.", viewRows),
			obj(map[string]any{"start": num("Where to start, from 1 (default 1).")})},
		{"find_headings", "Find headings anywhere in the user's files with a query (the language in the instructions). Gives ids to use with the other tools.",
			obj(map[string]any{"query": str("The query, like IsTask() && HasTags(\"work\").")}, "query")},
		{"read_heading", "A heading's own text, as Org: its headline, dates, drawers and body, not its children.",
			obj(map[string]any{"id": id}, "id")},
	}
	if t.v.Kind != "columns" {
		out = append(out, codeTool{"suggest_query", "Offer the user a query to show in the view instead of its own. It is run first: the answer says how many headings it finds, or why it is wrong.",
			obj(map[string]any{"query": str("The query."), "why": str("A few words on what it shows.")}, "query")})
	}
	if !t.edit {
		return out
	}
	return append(out,
		codeTool{"set_status", "Set a heading's todo keyword (TODO, NEXT, DONE, WAITING... as the file defines them), or \"\" for none. Reaching a done keyword closes it the way org does (a repeating task moves on instead).",
			obj(map[string]any{"id": id, "keyword": str("The keyword.")}, "id", "keyword")},
		codeTool{"set_property", "Set a property of a heading (EFFORT, ASSIGNED, any name); an empty value removes it.",
			obj(map[string]any{"id": id, "name": str("The property's name, like EFFORT."), "value": str("The value, like 2h.")}, "id", "name", "value")},
		codeTool{"set_tags", "Set a heading's tags: the whole list, replacing what it has.",
			obj(map[string]any{"id": id, "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "The tags, without colons."}}, "id", "tags")},
		codeTool{"set_priority", "Set a heading's priority: A, B or C, or \"\" for none.",
			obj(map[string]any{"id": id, "priority": str("The priority.")}, "id", "priority")},
		codeTool{"set_date", "Set or remove a heading's SCHEDULED or DEADLINE date.",
			obj(map[string]any{"id": id, "kind": str("SCHEDULED or DEADLINE."), "date": str("YYYY-MM-DD (or tomorrow, fri, +2w); empty removes it.")}, "id", "kind", "date")},
		codeTool{"add_heading", "Add a new heading under one (as its last child) or straight after one (at its level). Gives the new heading's id.",
			obj(map[string]any{
				"under": str("The id of the heading to put it under."), "after": str("Or the id of the heading to put it after."),
				"title": str("Its title."), "keyword": str("A todo keyword, like TODO (optional)."),
				"effort": str("An EFFORT, like 2h (optional)."), "scheduled": str("A SCHEDULED date, YYYY-MM-DD (optional)."),
				"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags (optional)."},
			}, "title")},
		codeTool{"set_after", "Make a task wait for another (its AFTER property, what a Gantt chart draws as a dependency), or with after \"\" stop it waiting.",
			obj(map[string]any{"id": id, "after": str("The id of the task it waits for, or \"\".")}, "id", "after")},
	)
}

func (t *viewTools) call(name string, args json.RawMessage) (string, error) {
	var a struct {
		Start     int      `json:"start"`
		Query     string   `json:"query"`
		Why       string   `json:"why"`
		ID        string   `json:"id"`
		Keyword   *string  `json:"keyword"`
		Name      string   `json:"name"`
		Value     string   `json:"value"`
		Tags      []string `json:"tags"`
		Priority  *string  `json:"priority"`
		Kind      string   `json:"kind"`
		Date      string   `json:"date"`
		Under     string   `json:"under"`
		After     *string  `json:"after"`
		Title     string   `json:"title"`
		Effort    string   `json:"effort"`
		Scheduled string   `json:"scheduled"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return "", err
		}
	}
	switch name {
	case "list_headings":
		secs, files, err := t.set()
		if err != nil {
			return "", err
		}
		start := a.Start - 1
		if start < 0 {
			start = 0
		}
		if start >= len(secs) {
			return fmt.Sprintf("The view shows only %d headings.", len(secs)), nil
		}
		text, n := t.rows(secs, files, start)
		head := fmt.Sprintf("Headings %d-%d of %d:\n", start+1, start+n, len(secs))
		if start+n < len(secs) {
			head = fmt.Sprintf("Headings %d-%d of %d (start %d for more):\n", start+1, start+n, len(secs), start+n+1)
		}
		t.note("Looked at the view")
		return head + text, nil
	case "find_headings":
		return t.find(a.Query)
	case "read_heading":
		return t.read(a.ID)
	case "suggest_query":
		if t.v.Kind == "columns" {
			return "", fmt.Errorf("the column view shows a file, not a query: use find_headings to look elsewhere")
		}
		return t.suggest(a.Query, a.Why)
	}
	if !t.edit {
		return "", fmt.Errorf("the user has not let you change headings: say what you would change instead")
	}
	switch name {
	case "set_status":
		if a.Keyword == nil {
			return "", fmt.Errorf("give a keyword")
		}
		kw := strings.TrimSpace(*a.Keyword)
		return t.change(a.ID, func(hash, title string) (string, error) {
			res, err := ChangeStatus(&common.TodoItemChange{Hash: hash, Value: kw})
			if err != nil {
				return "", err
			}
			did := fmt.Sprintf("Marked “%s” %s", title, kw)
			if kw == "" {
				did = fmt.Sprintf("Took the keyword off “%s”", title)
			}
			if res.Msg != "" {
				did += " (" + res.Msg + ")"
			}
			return did, nil
		})
	case "set_property":
		prop := strings.ToUpper(strings.TrimSpace(a.Name))
		if !viewPropRe.MatchString(prop) {
			return "", fmt.Errorf("%q is not a property name (letters, digits, - and _)", a.Name)
		}
		val := strings.TrimSpace(a.Value)
		if strings.ContainsAny(val, "\n\r") {
			return "", fmt.Errorf("a property's value is one line")
		}
		return t.change(a.ID, func(hash, title string) (string, error) {
			if _, err := ChangeProperty(&common.TodoPropertyChange{Hash: hash, Name: prop, Value: val}); err != nil {
				return "", err
			}
			if val == "" {
				return fmt.Sprintf("Removed %s from “%s”", prop, title), nil
			}
			return fmt.Sprintf("Set %s of “%s” to %s", prop, title, val), nil
		})
	case "set_tags":
		tags := []string{}
		for _, g := range a.Tags {
			g = strings.Trim(strings.TrimSpace(g), ":")
			if g == "" {
				continue
			}
			if !viewTagRe.MatchString(g) {
				return "", fmt.Errorf("%q is not a tag (letters, digits, _, @, #, %%)", g)
			}
			tags = append(tags, g)
		}
		return t.change(a.ID, func(hash, title string) (string, error) {
			if err := setHeadingParts(hash, nil, &tags); err != nil {
				return "", err
			}
			if len(tags) == 0 {
				return fmt.Sprintf("Took the tags off “%s”", title), nil
			}
			return fmt.Sprintf("Tagged “%s” :%s:", title, strings.Join(tags, ":")), nil
		})
	case "set_priority":
		if a.Priority == nil {
			return "", fmt.Errorf("give a priority")
		}
		p := strings.ToUpper(strings.TrimSpace(*a.Priority))
		if p != "" && !priorityRe.MatchString(p) {
			return "", fmt.Errorf("a priority is one letter or digit, like A")
		}
		return t.change(a.ID, func(hash, title string) (string, error) {
			if err := setHeadingParts(hash, &p, nil); err != nil {
				return "", err
			}
			if p == "" {
				return fmt.Sprintf("Took the priority off “%s”", title), nil
			}
			return fmt.Sprintf("Made “%s” priority %s", title, p), nil
		})
	case "set_date":
		kind := strings.ToUpper(strings.TrimSpace(a.Kind))
		if kind != "SCHEDULED" && kind != "DEADLINE" {
			return "", fmt.Errorf("kind is SCHEDULED or DEADLINE")
		}
		stamp := ""
		if d := strings.TrimSpace(a.Date); d != "" {
			s, err := orgdate.ParseDateToOrg(d, time.Now())
			if err != nil {
				return "", fmt.Errorf("%q is not a date: write YYYY-MM-DD", d)
			}
			stamp = s
		}
		return t.change(a.ID, func(hash, title string) (string, error) {
			if _, err := ChangeDate(&common.TodoDateChange{Hash: hash, Name: kind, Value: stamp}); err != nil {
				return "", err
			}
			if stamp == "" {
				return fmt.Sprintf("Removed the %s date of “%s”", strings.ToLower(kind), title), nil
			}
			return fmt.Sprintf("Set %s of “%s” to %s", kind, title, stamp), nil
		})
	case "add_heading":
		after := ""
		if a.After != nil {
			after = strings.TrimSpace(*a.After)
		}
		return t.add(strings.TrimSpace(a.Under), after, a.Title, a.Keyword, a.Effort, a.Scheduled, a.Tags)
	case "set_after":
		if a.After == nil {
			return "", fmt.Errorf("give after: an id, or \"\"")
		}
		return t.setAfter(a.ID, strings.TrimSpace(*a.After))
	}
	return "", fmt.Errorf("no tool %s", name)
}

var viewPropRe = regexp.MustCompile(`^[A-Z0-9_-]{1,60}$`)
var viewTagRe = regexp.MustCompile(`^[\pL\pN_@#%]{1,60}$`)

func (t *viewTools) note(what string) { t.send(map[string]any{"tool": what}) }

func (t *viewTools) find(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("give a query")
	}
	var secs []*org.Section
	var files []*common.OrgFile
	err := queryEachSection(&common.StringQuery{Query: query}, func(s *org.Section, f *common.OrgFile) {
		if s.Headline != nil {
			secs = append(secs, s)
			files = append(files, f)
		}
	})
	if err != nil {
		return "", fmt.Errorf("the query is wrong: %v", err)
	}
	t.note("Searched: " + query)
	if len(secs) == 0 {
		return "Nothing matches.", nil
	}
	text, n := t.rows(secs, files, 0)
	if n < len(secs) {
		return fmt.Sprintf("%d match; the first %d:\n%s", len(secs), n, text), nil
	}
	return fmt.Sprintf("%d match:\n%s", len(secs), text), nil
}

func (t *viewTools) read(id string) (string, error) {
	sec, f, err := t.resolve(id)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(f.Filename)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(raw), "\n")
	from := sec.Headline.Pos.Row
	if from < 0 || from >= len(lines) {
		return "", fmt.Errorf("%s has moved: list the headings again", id)
	}
	end := from + 1
	for end < len(lines) && !isHeadingLine(lines[end]) {
		end++
	}
	cut := ""
	if end-from > viewReadLines {
		end, cut = from+viewReadLines, fmt.Sprintf("\n(cut at %d lines)", viewReadLines)
	}
	t.note("Read “" + common.GetSectionTitle(sec) + "”")
	return strings.Join(lines[from:end], "\n") + cut, nil
}

func (t *viewTools) suggest(query, why string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("give a query")
	}
	n := 0
	titles := []string{}
	err := queryEachSection(&common.StringQuery{Query: query}, func(s *org.Section, f *common.OrgFile) {
		if s.Headline == nil {
			return
		}
		n++
		if len(titles) < 5 {
			titles = append(titles, common.GetSectionTitle(s))
		}
	})
	if err != nil {
		return "", fmt.Errorf("the query is wrong, so it was not offered: %v", err)
	}
	t.send(map[string]any{"query": query, "why": strings.TrimSpace(why), "count": n})
	if n == 0 {
		return "Offered to the user, but it finds nothing: say so, or offer a better one.", nil
	}
	return fmt.Sprintf("Offered to the user. It finds %d headings, such as: %s.", n, strings.Join(titles, "; ")), nil
}

// ---------------------------------------------------------------------------
// Writes, and undoing a turn's
// ---------------------------------------------------------------------------

// Keeps a file's text before the turn's first write to it.
func (t *viewTools) keep(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.before[path]; ok {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	t.before[path] = raw
	t.order = append(t.order, path)
}

// One write to a heading: resolved now, its file kept, then reported.
func (t *viewTools) change(id string, do func(hash, title string) (string, error)) (string, error) {
	sec, f, err := t.resolve(id)
	if err != nil {
		return "", err
	}
	t.keep(f.Filename)
	did, err := do(sec.Hash, common.GetSectionTitle(sec))
	if err != nil {
		return "", err
	}
	t.wrote(did, f.Filename)
	return did + ".", nil
}

func (t *viewTools) wrote(did, file string) {
	t.mu.Lock()
	t.did = append(t.did, did)
	t.mu.Unlock()
	t.send(map[string]any{"tool": did, "wrote": file})
}

func (t *viewTools) add(under, after, title string, keyword *string, effort, scheduled string, tags []string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || strings.ContainsAny(title, "\n\r") {
		return "", fmt.Errorf("a heading needs a title, on one line")
	}
	if (under == "") == (after == "") {
		return "", fmt.Errorf("give one of under or after")
	}
	ref := under
	if ref == "" {
		ref = after
	}
	sec, f, err := t.resolve(ref)
	if err != nil {
		return "", err
	}
	args := common.GanttAdd{Headline: title, Effort: strings.TrimSpace(effort), Tags: tags}
	if keyword != nil {
		args.Status = strings.TrimSpace(*keyword)
	}
	if d := strings.TrimSpace(scheduled); d != "" {
		od, _, err := orgdate.ParseDate(d, time.Now())
		if err != nil {
			return "", fmt.Errorf("%q is not a date: write YYYY-MM-DD", d)
		}
		args.Start = od.Day.Format("2006-01-02")
	}
	parentOlp := outlineOf(sec)
	if under != "" {
		args.ParentHash = sec.Hash
	} else {
		args.AfterHash = sec.Hash
		parentOlp = parentOlp[:len(parentOlp)-1]
	}
	t.keep(f.Filename)
	if _, err := ganttAdd(&args); err != nil {
		return "", err
	}
	where := "under “" + common.GetSectionTitle(sec) + "”"
	if after != "" {
		where = "after “" + common.GetSectionTitle(sec) + "”"
	}
	t.wrote(fmt.Sprintf("Added “%s” %s", title, where), f.Filename)
	// Its id: the new heading by its outline path.
	olp := olpString(append(append([]string{}, parentOlp...), title))
	if nf, ns := GetDb().GetFromTarget(&common.Target{Type: "file+olp", Filename: f.Filename, Id: olp}, false); nf != nil && ns != nil {
		return fmt.Sprintf("Added it as %s.", t.idFor(ns, nf)), nil
	}
	return "Added it.", nil
}

func (t *viewTools) setAfter(id, after string) (string, error) {
	sec, f, err := t.resolve(id)
	if err != nil {
		return "", err
	}
	title := common.GetSectionTitle(sec)
	req := common.GanttLinkReq{Task: sec.Hash, Remove: after == ""}
	did := fmt.Sprintf("“%s” no longer waits", title)
	other := f
	if after != "" {
		as, af, err := t.resolve(after)
		if err != nil {
			return "", err
		}
		req.After, other = as.Hash, af
		did = fmt.Sprintf("“%s” now waits for “%s”", title, common.GetSectionTitle(as))
	}
	t.keep(f.Filename)
	t.keep(other.Filename)
	res, err := ganttLink(req)
	if err != nil {
		return "", err
	}
	if !res.Ok {
		return "", fmt.Errorf("%s", res.Msg)
	}
	t.wrote(did, f.Filename)
	if other.Filename != f.Filename {
		t.send(map[string]any{"wrote": other.Filename})
	}
	return did + ".", nil
}

type viewUndoFile struct {
	path          string
	before, after []byte
}

type viewUndo struct {
	user  string
	at    time.Time
	files []viewUndoFile
	did   []string
}

var viewUndos = struct {
	sync.Mutex
	m map[string]*viewUndo
}{m: map[string]*viewUndo{}}

const viewUndoKeep = 50

// The turn's undo token, if it wrote anything.
func (t *viewTools) finish(user string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.order) == 0 {
		return ""
	}
	u := &viewUndo{user: user, at: time.Now(), did: append([]string{}, t.did...)}
	for _, p := range t.order {
		after, _ := os.ReadFile(p)
		u.files = append(u.files, viewUndoFile{path: p, before: t.before[p], after: after})
	}
	raw := make([]byte, 12)
	rand.Read(raw)
	token := hex.EncodeToString(raw)
	viewUndos.Lock()
	viewUndos.m[token] = u
	if len(viewUndos.m) > viewUndoKeep {
		type aged struct {
			k  string
			at time.Time
		}
		all := []aged{}
		for k, v := range viewUndos.m {
			all = append(all, aged{k, v.at})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
		for _, a := range all[:len(all)-viewUndoKeep] {
			delete(viewUndos.m, a.k)
		}
	}
	viewUndos.Unlock()
	return token
}

/* SDOC: API
* POST /assistant/undo — Undo What the Assistant Changed
	Body ={Token}=, the =undo= of a view assistant turn that changed
	headings (=POST /assistant/chat= with =View=). Every file the turn wrote
	is put back as it was, but only if each still reads exactly as the turn
	left it: an edit made since (by anyone) makes the undo refuse, naming the
	file, rather than lose it. *Response:* ={Ok, Msg}=.
EDOC */
func PostAssistantUndo(w http.ResponseWriter, r *http.Request) {
	var req struct{ Token string }
	json.NewDecoder(r.Body).Decode(&req)
	viewUndos.Lock()
	u := viewUndos.m[req.Token]
	viewUndos.Unlock()
	if u == nil || u.user != GetUsername(r) {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "nothing to undo: it was undone already, or the server has restarted since"})
		return
	}
	for _, f := range u.files {
		now, err := os.ReadFile(f.path)
		if err != nil || string(now) != string(f.after) {
			writingJson(w, common.ResultMsg{Ok: false, Msg: filepath.Base(f.path) + " has changed since, so nothing was undone - undo that by hand"})
			return
		}
	}
	for _, f := range u.files {
		if err := os.WriteFile(f.path, f.before, 0644); err != nil {
			writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
			return
		}
		GetDb().ReloadFile(f.path)
	}
	viewUndos.Lock()
	delete(viewUndos.m, req.Token)
	viewUndos.Unlock()
	writingJson(w, common.ResultMsg{Ok: true, Msg: fmt.Sprintf("undid %d change%s", len(u.did), plural(len(u.did)))})
}
