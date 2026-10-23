package templates

// What a file template can say, for an editor to offer as it is typed: every
// value with what it would be right now, the functions, and the filters with
// what they do. worg's template editor (TemplateEditor.tsx) completes and
// explains from this, so a name added to the standard context shows up there
// without anybody touching the browser.

import (
	"os"
	"sort"
	"strings"
	"time"

	"github.com/flosch/pongo2/v5"
)

// Word is one thing a template can name.
//
// Kind is "value" (a name, {{date}}), "function" ({{ when("fri") }}), "map"
// (env, read as {{env.HOME}}) or "filter" ({{ title|upper }}). Value is what a
// value would be written as now; Alias names the value it is another spelling
// of; Arg says what a filter or function takes, "" for nothing.
type Word struct {
	Name  string
	Kind  string
	Doc   string
	Value string `json:",omitempty"`
	Alias string `json:",omitempty"`
	Arg   string `json:",omitempty"`
	Group string
}

// What each standard value is, said once. A name with no line here still
// completes, it just explains nothing.
var valueDocs = map[string]string{
	"date":       "Today, as 2026-10-03",
	"time":       "The time, as 09:07",
	"datetime":   "Date and time, as 2026-10-03 09:07",
	"today":      "Today as an active timestamp: shows on the agenda",
	"now":        "Now as an inactive timestamp: a record, not a reminder",
	"tomorrow":   "Tomorrow as an active timestamp - what a SCHEDULED line wants",
	"yesterday":  "Yesterday as an active timestamp",
	"week":       "The ISO week, as 2026-W40",
	"week_start": "Monday of this week, as an active timestamp",
	"week_end":   "Sunday of this week, as an active timestamp",
	"month":      "The month, as 2026-10",
	"year":       "The year, as 2026",
	"day":        "The day of the month, two digits",
	"weekday":    "The day's name, as Saturday",
	"dayname":    "The day's short name, as Sat",
	"epoch":      "Seconds since 1970 - a sort key or a file name",
	"iso":        "Now in RFC 3339, for a field a program reads",
	"uuid":       "A fresh v4 uuid, new each time the template is used",
	"shortuid":   "Eight characters out of the hat, safe in a file name",
	"hostname":   "The machine holding the org files",
	"username":   "The account running the server",
	"author":     "author: from the config, else the account's full name",
	"email":      "email: from the config",
	"orgdir":     "The first org directory",
	"title":      "The title typed in the New File dialog, else one made from the file name",
	"filename":   "The path of the file being made",
	"basename":   "The file's name without its folder or .org",
	"folder":     "The name of the folder the file is going into - clients/acme gives acme",
	"daypage":    "Today's day page, as a path from the new file - [[file:{{daypage}}][today]]",
}

// The values a new file gets beyond the standard context (CreateOrgFileFromTemplate).
var fileValues = []string{"title", "filename", "basename", "folder", "daypage"}

type filterDoc struct{ name, arg, doc string }

// The filters a template can use: pongo2's own, then the ones orgs registers
// in Initialize. Only those a file template might plausibly want are listed;
// a filter missing here still works.
var filterDocs = []filterDoc{
	{"upper", "", "UPPER CASE"},
	{"lower", "", "lower case"},
	{"title", "", "Each Word Capitalised"},
	{"capfirst", "", "First letter capitalised"},
	{"default", `"text"`, "This, unless it is empty"},
	{"default_if_none", `"text"`, "This, unless it is missing altogether"},
	{"cut", `" "`, "Takes every copy of the argument out"},
	{"truncatechars", "20", "At most this many characters, with …"},
	{"truncatewords", "5", "At most this many words, with …"},
	{"wordwrap", "70", "Wrapped at this many words a line"},
	{"ljust", "20", "Padded on the right to this width"},
	{"rjust", "20", "Padded on the left to this width"},
	{"center", "20", "Centred in this width"},
	{"length", "", "How long it is"},
	{"first", "", "The first item or character"},
	{"last", "", "The last item or character"},
	{"join", `", "`, "A list joined with the argument"},
	{"split", `","`, "Split into a list on the argument"},
	{"slice", `"0:3"`, "Part of it, python style"},
	{"date", `"Mon 2 Jan"`, "A time written in Go's layout"},
	{"pluralize", "", "s when the number is not one"},
	{"yesno", `"yes,no"`, "One word for true, one for false"},
	{"stringformat", `"%05d"`, "Written with a printf format"},
	{"wordcount", "", "How many words"},
	{"linenumbers", "", "Each line numbered"},
	{"striptags", "", "Html tags taken out"},
	{"urlencode", "", "Escaped for a url"},
	{"escape", "", "Html escaped (org files do not want this)"},
	{"safe", "", "Not escaped"},
	{"trim", "", "Spaces taken off both ends (orgs)"},
	{"fit", "20", "Padded or cut to exactly this width (orgs)"},
	{"orgIndent", "2", "Dedented, then indented under a heading of this level (orgs)"},
	{"orgWordWrap", "70", "Wrapped at this many columns (orgs)"},
	{"orgCleanup", "", "Spaces trimmed, non-breaking spaces made new lines (orgs)"},
	{"commaList", "", "A list written a, b, c (orgs)"},
	{"sepList", `"|"`, "A list written with the argument between (orgs)"},
	{"age", "", "How long ago a time was, as 3 days (orgs)"},
	{"plus", `"1w"`, "A date moved on - 3d, 2w, -1m, 1y, 2h - written the way it came (orgs)"},
	{"format", `"date"`, "A date written as one of the clock values: date, today, now, weekday, week, month (orgs)"},
	{"slug", "", "Lower case with dashes, for an id or a file name: Q4 Plans → q4-plans (orgs)"},
	{"tags", "", "A list, or words split at commas or spaces, as org tags: :work:q4_plans: (orgs)"},
}

// Vocabulary is everything a file template can name, with each value as it
// would be written now. Env variables come by name only: their values may be
// secrets, and the editor only needs to know they are there.
func (self *TemplateManager) Vocabulary() []Word {
	ctx := self.GetStandardContext()
	now := time.Now()
	out := []Word{}
	seen := map[string]bool{}
	add := func(w Word) {
		if !seen[w.Kind+":"+w.Name] {
			seen[w.Kind+":"+w.Name] = true
			out = append(out, w)
		}
	}
	for _, n := range fileValues {
		add(Word{Name: n, Kind: "value", Doc: valueDocs[n], Group: "This file"})
	}
	add(Word{Name: "day_page_title", Kind: "value", Doc: valueDocs["title"], Alias: "title", Group: "This file"})
	names := make([]string, 0, len(*ctx))
	for k := range *ctx {
		names = append(names, k)
	}
	sort.Strings(names)
	clock := map[string]bool{}
	for _, n := range AutoNames {
		clock[n] = true
	}
	for _, n := range names {
		if n == "when" || n == "env" {
			continue
		}
		if c, ok := Aliases[n]; ok {
			add(Word{Name: n, Kind: "value", Doc: valueDocs[c], Alias: c, Value: valueOf((*ctx)[n]), Group: groupOf(c, clock)})
			continue
		}
		add(Word{Name: n, Kind: "value", Doc: valueDocs[n], Value: valueOf((*ctx)[n]), Group: groupOf(n, clock)})
	}
	add(Word{
		Name: "when", Kind: "function", Arg: `"fri 14:00"`, Group: "Functions",
		Doc:   "Any date, read the way orgs sched reads one (tomorrow, +2w, fri 14:00), as an org timestamp. A second argument (\"date\") writes it as that value instead.",
		Value: when(now)("fri 14:00"),
	})
	add(Word{
		Name: "ask", Kind: "function", Arg: `"Client", ""`, Group: "Functions",
		Doc: "Asks whoever makes the file: the New File dialog shows a box for it. A second argument is what the box starts with.",
	})
	if ds := days(now)("mon", 5); len(ds) == 5 {
		add(Word{
			Name: "days", Kind: "function", Arg: `"mon", 5`, Group: "Functions",
			Doc:   "A run of days from a date (read like when, so \"mon\" is the next Monday; days(week_start, 5) is this week), for {% for d in days(\"mon\", 5) %}. Each is its timestamp, with d.weekday, d.date and every other date value.",
			Value: ds[0].String() + " … " + ds[4].String(),
		})
	}
	add(Word{Name: "env", Kind: "map", Group: "Functions", Doc: "An environment variable of the server, as env.HOME"})
	envs := []string{}
	for _, e := range os.Environ() {
		if k, _, ok := strings.Cut(e, "="); ok && k != "" {
			envs = append(envs, k)
		}
	}
	sort.Strings(envs)
	for _, k := range envs {
		add(Word{Name: "env." + k, Kind: "env", Group: "Environment", Doc: "The server's $" + k})
	}
	for _, f := range filterDocs {
		if pongo2.FilterExists(f.name) {
			add(Word{Name: f.name, Kind: "filter", Arg: f.arg, Doc: f.doc, Group: "Filters"})
		}
	}
	return out
}

func groupOf(name string, clock map[string]bool) string {
	switch name {
	case "uuid", "shortuid", "hostname", "username", "author", "email", "orgdir":
		return "Who and where"
	}
	if clock[name] {
		return "Dates and times"
	}
	return "Other"
}

func valueOf(v any) string {
	if pv, ok := v.(*pongo2.Value); ok {
		return pv.String()
	}
	return ""
}
