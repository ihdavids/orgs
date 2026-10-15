package kanban

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
)

// A board of three keyword columns, laid out the way the terminal lays it out.
func htmlApp() *app {
	b := NewBoard("Sprint <1>")
	b.Columns = []Column{{Value: "TODO", Color: "blue"}, {Value: "DOING", Color: "amber", Limit: 1}, {Value: "DONE", Color: "green"}}
	b.Folded = []string{"DONE"}
	due := org.OrgDate{Start: time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)}
	a := &app{boards: []Board{b}, bodies: map[string]body{}, asked: map[string]bool{}}
	a.cards = []Card{
		{Hash: "h1", Headline: `<script>alert("x")</script>`, Status: "TODO", Priority: "A", Tags: []string{"work"}, Filename: "/n/plan.org", Deadline: &due},
		{Hash: "h2", Headline: "Write the report", Status: "DOING", Filename: "/n/plan.org"},
		{Hash: "h3", Headline: "Second doing", Status: "DOING", Filename: "/n/plan.org"},
		{Hash: "h4", Headline: "Shipped", Status: "DONE", Filename: "/n/plan.org"},
	}
	a.bodies["h2"] = body{Ok: true, Text: ":PROPERTIES:\n:ID: x\n:END:\nSee [[https://example.com/a?b=1&c=2][the spec]] and [[javascript:alert(1)][this]].\n- [X] outline\n- [ ] draft <b>\n"}
	a.layout()
	return a
}

func TestHtmlBoard(t *testing.T) {
	a := htmlApp()
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.Local)
	page, err := renderHTML(a.htmlPageOf("", now))
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	// Columns in the board's order, each with its count.
	todo, doing, done := strings.Index(s, `class="col-title">TODO<`), strings.Index(s, `class="col-title">DOING<`), strings.Index(s, `class="col-title">DONE<`)
	if todo < 0 || doing < todo || done < doing {
		t.Fatalf("columns out of order: %d %d %d", todo, doing, done)
	}
	if !strings.Contains(s, `<span class="limit over">/ 1</span>`) {
		t.Error("DOING holds two cards over a limit of one and should say so")
	}
	if !strings.Contains(s, `class="column folded"`) {
		t.Error("the folded column should start folded")
	}
	// Each card under its own column.
	if i := strings.Index(s, "Write the report"); i < doing || i > done {
		t.Error("Write the report is not in DOING")
	}
	if i := strings.Index(s, "Shipped"); i < done {
		t.Error("Shipped is not in DONE")
	}
	// Hostile text is text.
	if strings.Contains(s, `<script>alert("x")`) || !strings.Contains(s, "&lt;script&gt;") {
		t.Error("the headline was not escaped")
	}
	if strings.Contains(s, "draft <b>") || !strings.Contains(s, "draft &lt;b&gt;") {
		t.Error("a checklist item was not escaped")
	}
	if !strings.Contains(s, `<a href="https://example.com/a?b=1&amp;c=2" rel="noopener">the spec</a>`) {
		t.Error("an https link should be a link")
	}
	if strings.Contains(s, `href="javascript:`) || !strings.Contains(s, `<span class="link">this</span>`) {
		t.Error("a javascript: link must not be a link")
	}
	if strings.Contains(s, ":ID:") {
		t.Error("the property drawer leaked into the body")
	}
	// Checklist count and overdue deadline, as the card draws them.
	if !strings.Contains(s, "☑ 1/2") || !strings.Contains(s, `style="width:50%"`) {
		t.Error("checklist progress missing")
	}
	if !strings.Contains(s, `class="when deadline overdue"`) || !strings.Contains(s, "⚑ Oct 1") {
		t.Error("an overdue deadline should be marked")
	}
	if !strings.Contains(s, "#"+"work") {
		t.Error("tags should be chips")
	}
}

func TestHtmlThemes(t *testing.T) {
	a := htmlApp()
	for _, th := range append(append([]string{}, HtmlThemes...), "dark", "light") {
		page, err := renderHTML(a.htmlPageOf(th, time.Now()))
		if err != nil {
			t.Fatalf("%s: %v", th, err)
		}
		if (th == "dark" || th == "light") && !strings.Contains(string(page), `data-theme="`+th+`"`) {
			t.Errorf("%s should hold the page to it", th)
		}
	}
	if _, err := renderHTML(a.htmlPageOf("nope", time.Now())); err == nil {
		t.Error("an unknown theme should be refused")
	}
}

func TestHtmlListLayout(t *testing.T) {
	a := htmlApp()
	a.board().Layout = "list"
	a.board().ListFields = []string{"priority", "deadline", "file"}
	a.layout()
	page, err := renderHTML(a.htmlPageOf("", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	if !strings.Contains(s, "<th>Deadline</th>") || !strings.Contains(s, "<td>2026-10-01</td>") || !strings.Contains(s, "<td>plan.org</td>") {
		t.Error("the list should lay out the board's fields")
	}
}

func TestHtmlFileFor(t *testing.T) {
	if got := htmlFileFor("Sprint <1>/two"); got != "Sprint-1-two.html" {
		t.Errorf("got %q", got)
	}
	if got := htmlFileFor("***"); got != "board.html" {
		t.Errorf("got %q", got)
	}
}
