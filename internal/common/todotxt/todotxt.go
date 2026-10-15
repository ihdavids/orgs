// Package todotxt reads and writes the todo.txt format
// (https://github.com/todotxt/todo.txt), and turns its tasks into org.
//
//	x (A) 2026-10-12 2026-10-01 Call the plumber +house @phone due:2026-10-15
//
// One task a line: `x` and a completion date when done, a (A)-(Z) priority,
// a creation date, then the text, in which +project, @context and key:value
// words may appear anywhere.
//
// Shared by `orgs import` (todo.txt into org, on the client) and the
// `todotxt` exporter (org into todo.txt, on the server), so a list that goes
// out and comes back again is read by the same rules it was written by.
package todotxt

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const dateFmt = "2006-01-02"

// Task is one line of a todo.txt file.
type Task struct {
	Done      bool
	Priority  string // "A".."Z", or ""
	Completed time.Time
	Created   time.Time
	// The words that are not projects, contexts or key:values, in order.
	Text     string
	Projects []string // without the +
	Contexts []string // without the @
	// key:value pairs in the order they were written; keys as written.
	Extra [][2]string
}

// Get is a key:value's value, "" when the task has none.
func (t *Task) Get(key string) string {
	for _, kv := range t.Extra {
		if strings.EqualFold(kv[0], key) {
			return kv[1]
		}
	}
	return ""
}

// Set replaces a key:value, or adds it at the end.
func (t *Task) Set(key, val string) {
	for i, kv := range t.Extra {
		if strings.EqualFold(kv[0], key) {
			t.Extra[i][1] = val
			return
		}
	}
	t.Extra = append(t.Extra, [2]string{key, val})
}

var (
	priRe  = regexp.MustCompile(`^\(([A-Z])\)$`)
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	// key:value, where neither side is empty and the key has no colon. A url
	// (https://...) is not one: its value starts with //.
	kvRe = regexp.MustCompile(`^([^\s:]+):(\S+)$`)
)

func date(s string) (time.Time, bool) {
	if !dateRe.MatchString(s) {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(dateFmt, s, time.Local)
	return t, err == nil
}

// ParseLine reads one task. ok is false for a blank line.
func ParseLine(line string) (Task, bool) {
	words := strings.Fields(line)
	t := Task{}
	if len(words) == 0 {
		return t, false
	}
	i := 0
	if words[0] == "x" {
		t.Done = true
		i++
		if i < len(words) {
			if d, ok := date(words[i]); ok {
				t.Completed = d
				i++
			}
		}
	}
	if i < len(words) {
		if m := priRe.FindStringSubmatch(words[i]); m != nil {
			t.Priority = m[1]
			i++
		}
	}
	if i < len(words) {
		if d, ok := date(words[i]); ok {
			t.Created = d
			i++
		}
	}
	text := []string{}
	for _, w := range words[i:] {
		switch {
		case len(w) > 1 && w[0] == '+':
			t.Projects = append(t.Projects, w[1:])
		case len(w) > 1 && w[0] == '@':
			t.Contexts = append(t.Contexts, w[1:])
		default:
			if m := kvRe.FindStringSubmatch(w); m != nil && !strings.HasPrefix(m[2], "//") {
				t.Extra = append(t.Extra, [2]string{m[1], m[2]})
			} else {
				text = append(text, w)
			}
		}
	}
	t.Text = strings.Join(text, " ")
	// Done tasks lose their (A); the convention keeps it as pri:A.
	if t.Done && t.Priority == "" {
		if p := strings.ToUpper(t.Get("pri")); len(p) == 1 && p[0] >= 'A' && p[0] <= 'Z' {
			t.Priority = p
			t.Extra = without(t.Extra, "pri")
		}
	}
	return t, true
}

func without(kvs [][2]string, key string) [][2]string {
	out := [][2]string{}
	for _, kv := range kvs {
		if !strings.EqualFold(kv[0], key) {
			out = append(out, kv)
		}
	}
	return out
}

// Parse reads a whole file, skipping blank lines.
func Parse(text string) []Task {
	out := []Task{}
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if t, ok := ParseLine(l); ok {
			out = append(out, t)
		}
	}
	return out
}

// String writes a task as one todo.txt line.
func (t Task) String() string {
	parts := []string{}
	if t.Done {
		parts = append(parts, "x")
		if !t.Completed.IsZero() {
			parts = append(parts, t.Completed.Format(dateFmt))
		}
	} else if t.Priority != "" {
		parts = append(parts, "("+t.Priority+")")
	}
	if !t.Created.IsZero() {
		// A creation date needs a completion date before it on a done task, or
		// it would be read as the completion date.
		if !(t.Done && t.Completed.IsZero()) {
			parts = append(parts, t.Created.Format(dateFmt))
		}
	}
	if t.Text != "" {
		parts = append(parts, t.Text)
	}
	for _, p := range t.Projects {
		parts = append(parts, "+"+p)
	}
	for _, c := range t.Contexts {
		parts = append(parts, "@"+c)
	}
	for _, kv := range t.Extra {
		parts = append(parts, kv[0]+":"+kv[1])
	}
	if t.Done && t.Priority != "" {
		parts = append(parts, "pri:"+t.Priority)
	}
	return strings.Join(parts, " ")
}

// Format writes tasks one a line.
func Format(tasks []Task) string {
	b := strings.Builder{}
	for _, t := range tasks {
		b.WriteString(t.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// Word makes a todo.txt word of a heading title or tag: no spaces, nothing a
// reader would take for the start of a project or context.
func Word(s string) string {
	s = strings.Join(strings.Fields(s), "-")
	return strings.TrimLeft(s, "+@")
}

// ---------------------------------------------------------------------------
// Repeats: todo.txt's rec: (from the topydo/SimpleTask convention) and org's
// repeater cookie.
// ---------------------------------------------------------------------------

var recRe = regexp.MustCompile(`^(\+?)(\d+)([dwmyb])$`)

// RecToOrg turns rec:1w into org's cookie: `+1w` (strict, the next date from
// the last) for rec:+1w, `.+1w` (from when it was done) for rec:1w. Business
// days have no org equivalent and become days.
func RecToOrg(rec string) string {
	m := recRe.FindStringSubmatch(strings.TrimSpace(rec))
	if m == nil {
		return ""
	}
	unit := m[3]
	if unit == "b" {
		unit = "d"
	}
	if m[1] == "+" {
		return "+" + m[2] + unit
	}
	return ".+" + m[2] + unit
}

var orgRepRe = regexp.MustCompile(`^(\.\+|\+\+|\+)(\d+)([hdwmy])`)

// OrgToRec is RecToOrg the other way about; an hourly repeat has no rec:.
func OrgToRec(cookie string) string {
	m := orgRepRe.FindStringSubmatch(strings.TrimSpace(cookie))
	if m == nil || m[3] == "h" {
		return ""
	}
	if m[1] == ".+" {
		return m[2] + m[3]
	}
	return "+" + m[2] + m[3]
}

// ---------------------------------------------------------------------------
// Into org
// ---------------------------------------------------------------------------

// OrgOptions says how tasks become org.
type OrgOptions struct {
	// #+TITLE: of a new file; nothing when empty.
	Title string
	// Keywords for open and done tasks.
	Todo, Done string
	// Put each task under a heading for its first project (the default), or
	// leave them all at the top level with projects as a property.
	Flat bool
}

func orgDate(t time.Time, active bool) string {
	s := t.Format("2006-01-02 Mon")
	if active {
		return "<" + s + ">"
	}
	return "[" + s + "]"
}

// ToOrg writes tasks as an org file: one heading each, under a heading per
// project in the order projects first appear.
func ToOrg(tasks []Task, o OrgOptions) string {
	if o.Todo == "" {
		o.Todo = "TODO"
	}
	if o.Done == "" {
		o.Done = "DONE"
	}
	b := strings.Builder{}
	if o.Title != "" {
		b.WriteString("#+TITLE: " + o.Title + "\n")
		b.WriteString(fmt.Sprintf("#+TODO: %s | %s\n", o.Todo, o.Done))
		b.WriteString("\n")
	}
	groups := map[string][]Task{}
	order := []string{}
	for _, t := range tasks {
		p := ""
		if !o.Flat && len(t.Projects) > 0 {
			p = t.Projects[0]
		}
		if _, ok := groups[p]; !ok {
			order = append(order, p)
		}
		groups[p] = append(groups[p], t)
	}
	// Tasks with no project first, at the top, as they were in the list.
	sort.SliceStable(order, func(i, j int) bool { return order[i] == "" && order[j] != "" })
	for _, p := range order {
		lvl := 1
		if p != "" {
			b.WriteString("* " + p + "\n")
			lvl = 2
		}
		for _, t := range groups[p] {
			writeTask(&b, t, lvl, p, o)
		}
	}
	return b.String()
}

func writeTask(b *strings.Builder, t Task, lvl int, under string, o OrgOptions) {
	stars := strings.Repeat("*", lvl)
	kw := o.Todo
	if t.Done {
		kw = o.Done
	}
	line := stars + " " + kw
	if t.Priority != "" {
		line += " [#" + t.Priority + "]"
	}
	text := t.Text
	if text == "" {
		text = "(untitled)"
	}
	line += " " + text
	if len(t.Contexts) > 0 {
		tags := []string{}
		for _, c := range t.Contexts {
			tags = append(tags, "@"+orgTag(c))
		}
		line += " :" + strings.Join(tags, ":") + ":"
	}
	b.WriteString(line + "\n")

	indent := strings.Repeat(" ", lvl+1)
	rep := RecToOrg(t.Get("rec"))
	plan := []string{}
	if t.Done && !t.Completed.IsZero() {
		plan = append(plan, "CLOSED: "+orgDate(t.Completed, false))
	}
	due, dueOk := date(t.Get("due"))
	th, thOk := date(t.Get("t"))
	if dueOk {
		s := orgDate(due, true)
		if rep != "" {
			s = s[:len(s)-1] + " " + rep + ">"
			rep = ""
		}
		plan = append(plan, "DEADLINE: "+s)
	}
	if thOk {
		s := orgDate(th, true)
		if rep != "" {
			s = s[:len(s)-1] + " " + rep + ">"
			rep = ""
		}
		plan = append(plan, "SCHEDULED: "+s)
	}
	if len(plan) > 0 {
		b.WriteString(indent + strings.Join(plan, " ") + "\n")
	}

	props := [][2]string{}
	if !t.Created.IsZero() {
		props = append(props, [2]string{"CREATED", orgDate(t.Created, false)})
	}
	others := []string{}
	for _, p := range t.Projects {
		if p != under {
			others = append(others, "+"+p)
		}
	}
	if len(others) > 0 {
		props = append(props, [2]string{"PROJECTS", strings.Join(others, " ")})
	}
	for _, kv := range t.Extra {
		// What went into a planning line is not said twice; a due: that is
		// not a date is kept as it was.
		k := strings.ToLower(kv[0])
		if (k == "due" && dueOk) || (k == "t" && thOk) ||
			(k == "rec" && RecToOrg(kv[1]) != "" && (dueOk || thOk)) {
			continue
		}
		props = append(props, [2]string{strings.ToUpper(kv[0]), kv[1]})
	}
	if len(props) > 0 {
		b.WriteString(indent + ":PROPERTIES:\n")
		for _, kv := range props {
			b.WriteString(fmt.Sprintf("%s:%s: %s\n", indent, kv[0], kv[1]))
		}
		b.WriteString(indent + ":END:\n")
	}
}

// Org tags are letters, digits, _ @ # and %.
var badTag = regexp.MustCompile(`[^\p{L}\p{N}_@#%]`)

func orgTag(s string) string { return badTag.ReplaceAllString(s, "_") }
