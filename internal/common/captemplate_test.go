package common

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// A Tuesday, so the day name in a timestamp is something other than the Mon in
// the layout string and a swapped format shows up.
var capNow = time.Date(2026, 9, 29, 14, 5, 0, 0, time.UTC)

func TestCapAutoValues(t *testing.T) {
	cases := []struct{ name, want string }{
		{"date", "2026-09-29"},
		{"time", "14:05"},
		{"datetime", "2026-09-29 14:05"},
		{"today", "<2026-09-29 Tue>"},
		{"active", "<2026-09-29 Tue>"},
		{"now", "[2026-09-29 Tue 14:05]"},
		{"inactive", "[2026-09-29 Tue 14:05]"},
		{"timestamp", "[2026-09-29 Tue 14:05]"},
		{"tomorrow", "<2026-09-30 Wed>"},
		{"yesterday", "<2026-09-28 Mon>"},
		{"week", "2026-W40"},
		{"month", "2026-09"},
		{"year", "2026"},
		{"day", "29"},
		{"weekday", "Tuesday"},
		{"dayname", "Tue"},
		{"epoch", "1790690700"},
		{"iso", "2026-09-29T14:05:00Z"},
		{"username", "ian"},
		{"user", "ian"},
		// A name is trimmed and case folded, so a template may be written
		// however reads best.
		{" NOW ", "[2026-09-29 Tue 14:05]"},
	}
	for _, c := range cases {
		got, ok := CapAutoValue(c.name, capNow, "ian")
		if !ok {
			t.Errorf("%q: no value", c.name)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %q want %q", c.name, got, c.want)
		}
	}
}

// The ones that have no answer. Every one of these has to come back false
// rather than empty, because false means "leave this for somebody to answer"
// and empty would mean "answered, with nothing".
func TestCapAutoValueHasNoOpinion(t *testing.T) {
	for _, name := range []string{"CONTENT", "content", "project", "source", "", "   "} {
		if v, ok := CapAutoValue(name, capNow, "ian"); ok {
			t.Errorf("%q should have no value, got %q", name, v)
		}
	}
	if v, ok := CapAutoValue("username", capNow, ""); ok {
		t.Errorf("username with no user should have no value, got %q", v)
	}
	// It is still an auto name though: what kind of placeholder it is does not
	// depend on whether today's answer happens to exist.
	if !IsCapAutoName("username") || !IsCapAutoName("uuid") || !IsCapAutoName("NOW") {
		t.Errorf("username/uuid/now should all be auto names")
	}
	if IsCapAutoName("project") || IsCapAutoName("CONTENT") {
		t.Errorf("project and CONTENT are questions, not auto names")
	}
}

const capTpl = `:PROPERTIES:
:CUSTOM_ID: {{uuid}}
:ID:        {{guid}}
:CREATED:   {{now}}
:AUTHOR:    {{username}}
:SOURCE:    {{source|Where did this come from?}}
:PROJECT:   {{project}}
:WHEN:      {{now|When did this happen}}
:END:
{{CONTENT}}`

func TestCapExpandWritesADefaultAndKeepsTheName(t *testing.T) {
	got := CapExpandAutos(capTpl, capNow, "ian")

	for _, want := range []string{
		":CREATED:   {{now|=[2026-09-29 Tue 14:05]}}",
		":AUTHOR:    {{username|=ian}}",
		// A question keeps its question and gains an answer, so the client can
		// ask it with something already in the box.
		":WHEN:      {{now|When did this happen|=[2026-09-29 Tue 14:05]}}",
		// Left exactly as written: no value for either of these.
		":SOURCE:    {{source|Where did this come from?}}",
		":PROJECT:   {{project}}",
		"{{CONTENT}}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
	// Still a template: every placeholder is still a placeholder.
	if n := strings.Count(got, "{{"); n != 8 {
		t.Errorf("expected 8 placeholders, got %d:\n%s", n, got)
	}
	if len(strings.Split(got, "\n")) != len(strings.Split(capTpl, "\n")) {
		t.Errorf("the expansion changed the line count:\n%s", got)
	}
}

var uuidRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// One value per name per template, aliases counted as one name: a CUSTOM_ID and
// a link to it have to agree.
func TestCapExpandUuidIsOnePerTemplate(t *testing.T) {
	got := CapExpandAutos(":CUSTOM_ID: {{uuid}}\n:ID: {{guid}}\nSee [[id:{{uuid}}][this]]", capNow, "ian")
	ids := uuidRe.FindAllString(got, -1)
	if len(ids) != 3 {
		t.Fatalf("expected three uuids written, got %d:\n%s", len(ids), got)
	}
	for _, id := range ids {
		if id != ids[0] {
			t.Errorf("one template, two uuids:\n%s", got)
		}
	}
	// Two templates are two captures, so they are two uuids.
	other := CapExpandAutos(":CUSTOM_ID: {{uuid}}", capNow, "ian")
	if uuidRe.FindString(other) == ids[0] {
		t.Errorf("two templates got the same uuid")
	}
}

// Expanding an expanded template changes nothing: a template cannot collect a
// second answer, whichever end it goes through twice.
func TestCapExpandIsIdempotent(t *testing.T) {
	once := CapExpandAutos(capTpl, capNow, "ian")
	twice := CapExpandAutos(once, capNow.Add(time.Hour), "someone-else")
	if once != twice {
		t.Errorf("expanding twice changed it:\n%s\n---\n%s", once, twice)
	}
}

// Nothing to do is a no-op, and must not rewrite the text on the way past: a
// template is a file somebody typed.
func TestCapExpandLeavesPlainTextAlone(t *testing.T) {
	for _, tpl := range []string{
		"",
		"* TODO something\n  with a body\n",
		"a { brace } and a } stray {{",
	} {
		if got := CapExpandAutos(tpl, capNow, "ian"); got != tpl {
			t.Errorf("changed %q into %q", tpl, got)
		}
	}
}

func TestCapFields(t *testing.T) {
	fields := CapFields(CapExpandAutos(capTpl, capNow, "ian"))
	want := []CapField{
		{Key: "uuid", Prompt: "Uuid", Auto: true},
		{Key: "guid", Prompt: "Guid", Auto: true},
		{Key: "now", Prompt: "Now", Default: "[2026-09-29 Tue 14:05]", Auto: true},
		{Key: "username", Prompt: "Username", Default: "ian", Auto: true},
		{Key: "source", Prompt: "Where did this come from?"},
		{Key: "project", Prompt: "Project"},
		{Key: "CONTENT", Prompt: "Content", Content: true},
	}
	if len(fields) != len(want) {
		t.Fatalf("got %d fields, want %d: %+v", len(fields), len(want), fields)
	}
	for i, w := range want {
		g := fields[i]
		if g.Key != w.Key || g.Prompt != w.Prompt || g.Auto != w.Auto || g.Content != w.Content {
			t.Errorf("field %d: got %+v want %+v", i, g, w)
		}
		if w.Default != "" && g.Default != w.Default {
			t.Errorf("field %d default: got %q want %q", i, g.Default, w.Default)
		}
	}
	// The uuid's default is the uuid, and {{now}} asked twice is one field: the
	// question keeps the prompt it was written with the first time.
	if !uuidRe.MatchString(fields[0].Default) {
		t.Errorf("uuid default is not a uuid: %q", fields[0].Default)
	}
	// A template with no string of its own is still a body to type into.
	if f := CapFields(""); len(f) != 1 || !f[0].Content {
		t.Errorf("an empty template should still ask for a body, got %+v", f)
	}
}

func TestFillCapTemplate(t *testing.T) {
	tpl := CapExpandAutos(capTpl, capNow, "ian")
	fields := CapFields(tpl)
	values := map[string]string{}
	for _, f := range fields {
		values[f.Key] = f.Default
	}
	values["CONTENT"] = "the body\nof it"
	values["source"] = "an email"
	// project and guid are left unanswered.
	values["guid"] = ""

	got := FillCapTemplate(tpl, values)
	for _, want := range []string{
		":CREATED:   [2026-09-29 Tue 14:05]",
		":AUTHOR:    ian",
		":SOURCE:    an email",
		":WHEN:      [2026-09-29 Tue 14:05]",
		"the body\nof it",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
	// A line that was only an unanswered placeholder is dropped rather than
	// written as a property with nothing after it.
	for _, gone := range []string{":PROJECT:", ":ID:", "{{"} {
		if strings.Contains(got, gone) {
			t.Errorf("expected no %q in:\n%s", gone, got)
		}
	}
	// A line the template meant to write is never touched, however empty it
	// looks.
	if !strings.HasPrefix(got, ":PROPERTIES:\n") || !strings.Contains(got, "\n:END:\n") {
		t.Errorf("the drawer did not survive:\n%s", got)
	}
}

// A template with no string at all is a body, and nothing else.
func TestFillCapTemplateWithNoTemplate(t *testing.T) {
	if got := FillCapTemplate("", map[string]string{"CONTENT": "just this"}); got != "just this" {
		t.Errorf("got %q", got)
	}
}

var shortRe = regexp.MustCompile(`^[a-z2-7]{8}$`)

func TestCapShortUid(t *testing.T) {
	a, ok := CapAutoValue("shortuid", capNow, "")
	b, _ := CapAutoValue("shortuid", capNow, "")
	if !ok || !shortRe.MatchString(a) {
		t.Fatalf("shortuid should be 8 lowercase base32 characters, got %q", a)
	}
	if a == b {
		t.Errorf("two shortuids came out the same: %q", a)
	}
	// One per template, like uuid.
	got := CapExpandAutos("{{shortuid}} {{shortuid}}", capNow, "")
	parts := strings.Fields(got)
	if parts[0] != parts[1] {
		t.Errorf("one template, two shortuids: %s", got)
	}
}

// {{daypage}} is answered by whatever hook the server sets, and is left for
// somebody to answer when there is none.
func TestCapDayPage(t *testing.T) {
	old := CapDayPage
	defer func() { CapDayPage = old }()

	CapDayPage = nil
	if _, ok := CapAutoValue("daypage", capNow, ""); ok {
		t.Errorf("daypage with no hook should have no value")
	}
	if !IsCapAutoName("daypage") {
		t.Errorf("daypage is an auto name whether or not the hook is set")
	}
	CapDayPage = func(now time.Time) (string, bool) { return "worklog/Mon_2026_09_28.org", true }
	if got := CapExpandAutos("[[file:{{daypage}}][this week]]", capNow, ""); got != "[[file:{{daypage|=worklog/Mon_2026_09_28.org}}][this week]]" {
		t.Errorf("daypage in a template: %q", got)
	}
	// A path in a filename keeps its separators.
	if got, err := CapExpandFilename("{{daypage}}", capNow, ""); err != nil || got != "worklog/Mon_2026_09_28.org" {
		t.Errorf("daypage in a filename: %q %v", got, err)
	}
}

func TestCapExpandFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"links.org", "links.org"},
		{"notes/{{date}}.org", "notes/2026-09-29.org"},
		{"{{time}}.org", "14-05.org"},
		{"{{datetime}}.org", "2026-09-29_14-05.org"},
		{"{{now}}.org", "2026-09-29_Tue_14-05.org"},
		{"{{today}}.org", "2026-09-29_Tue.org"},
		{"{{iso}}.org", "2026-09-29T14-05-00Z.org"},
		{"{{week}}/{{weekday}}.org", "2026-W40/Tuesday.org"},
		{"{{kind|=meeting notes}}.org", "meeting_notes.org"},
	}
	for _, c := range cases {
		got, err := CapExpandFilename(c.in, capNow, "ian")
		if err != nil || got != c.want {
			t.Errorf("%q: got %q (%v) want %q", c.in, got, err, c.want)
		}
	}
	got, err := CapExpandFilename("tmp/{{date}}-{{shortuid}}-{{guid}}.org", capNow, "")
	if err != nil || !regexp.MustCompile(`^tmp/2026-09-29-[a-z2-7]{8}-[0-9a-f-]{36}\.org$`).MatchString(got) {
		t.Errorf("uuids in a filename: %q %v", got, err)
	}
	// Nobody is there to answer a question in a filename.
	if got, err := CapExpandFilename("{{project}}.org", capNow, ""); err == nil {
		t.Errorf("a question in a filename should be an error, got %q", got)
	}
}
