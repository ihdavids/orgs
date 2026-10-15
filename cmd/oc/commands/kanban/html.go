package kanban

// orgs kanban <board> -html: the board as one html file.
//
//	orgs kanban sprint -html                    sprint.html, here
//	orgs kanban sprint -html -out - | mail …    to stdout
//	orgs kanban sprint -html -theme paper -open
//
// Nothing in the page talks to a server: css and a little script are inline,
// so it can be mailed, attached or put on any web host. Which column a card is
// in, its order, its colours and its checklist count are the board's own
// (layout, SortCards, HeaderColorOf, CardSwatch, LabelsOf, checklistOf) - the
// page is drawn from what the terminal board draws, not worked out again. The
// one thing repeated in the page's script is the filter, and it only asks
// whether every word typed is somewhere in the text Haystack made for the
// card, which is CardMatches.

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/tuikit"
)

// HtmlThemes is the looks -html can be given. dark and light are the default
// look held to one side of the switch.
var HtmlThemes = []string{"default", "paper", "midnight"}

type htmlChip struct {
	Text, Bg, Fg, Class string
}

type htmlCheck struct {
	Text string
	Done bool
	Pad  int
}

type htmlCard struct {
	Title       string
	Priority    string
	PriorityBg  string
	PriorityFg  string
	HeaderLight string
	HeaderDark  string
	Stripe      string
	Labels      []htmlChip
	Chips       []htmlChip
	Checks      []htmlCheck
	ChecksDone  int
	Percent     int
	Deadline    string
	Overdue     bool
	Scheduled   string
	File        string
	Body        template.HTML
	HasMore     bool
	Clocked     bool
	Search      string
	// The list layout's row: the keyword and the board's list fields.
	Status string
	Cells  []string
}

type htmlColumn struct {
	Title  string
	Color  string
	Limit  int
	Over   bool
	Folded bool
	Cards  []htmlCard
}

type htmlPage struct {
	Board     string
	Query     string
	Generated string
	Theme     string
	Force     string // "dark", "light" or "" to follow the reader
	List      bool
	Fields    []ListField
	Columns   []htmlColumn
	Total     int
	Loose     int
	Problem   string
	Css       template.CSS
}

func inkHex(bg string) string {
	return fmt.Sprintf("#%06x", tuikit.Ink(bg).Hex())
}

var (
	orgLinkRe = regexp.MustCompile(`\[\[([^\]]+)\](?:\[([^\]]+)\])?\]`)
	codeRe    = regexp.MustCompile(`(^|[\s(])[=~]([^=~\n]+?)[=~]($|[\s.,;:!?)])`)
	boldRe    = regexp.MustCompile(`(^|[\s(])\*([^*\n]+?)\*($|[\s.,;:!?)])`)
	safeURLRe = regexp.MustCompile(`(?i)^(https?://|mailto:)`)
)

// inlineHTML writes one line of a card's body: escaped, with org links as
// links when they go somewhere a browser can follow and as their words when
// they do not (a file: or id: link means nothing on someone else's machine).
func inlineHTML(s string) string {
	out := strings.Builder{}
	last := 0
	for _, m := range orgLinkRe.FindAllStringSubmatchIndex(s, -1) {
		out.WriteString(inlineMarks(template.HTMLEscapeString(s[last:m[0]])))
		target := s[m[2]:m[3]]
		desc := target
		if m[4] >= 0 {
			desc = s[m[4]:m[5]]
		}
		if safeURLRe.MatchString(target) {
			out.WriteString(fmt.Sprintf(`<a href="%s" rel="noopener">%s</a>`,
				template.HTMLEscapeString(target), template.HTMLEscapeString(desc)))
		} else {
			out.WriteString(`<span class="link">` + template.HTMLEscapeString(desc) + `</span>`)
		}
		last = m[1]
	}
	out.WriteString(inlineMarks(template.HTMLEscapeString(s[last:])))
	return out.String()
}

// Bold and code on text that is already escaped (the marks are not touched by
// escaping).
func inlineMarks(s string) string {
	s = codeRe.ReplaceAllString(s, "$1<code>$2</code>$3")
	return boldRe.ReplaceAllString(s, "$1<strong>$2</strong>$3")
}

// bodyHTML is the prose under a heading: its lines outside drawers and
// planning lines, checkboxes left to the checklist, as paragraphs and list
// items. Long bodies are cut, because a card is not the place to read a
// chapter.
func bodyHTML(text string) template.HTML {
	lines := []string{}
	for _, l := range bodyLines(text) {
		if checkRe.MatchString(l) {
			continue
		}
		lines = append(lines, l)
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	cut := false
	if len(lines) > 40 {
		lines, cut = lines[:40], true
	}
	b := strings.Builder{}
	para := []string{}
	flush := func() {
		if len(para) > 0 {
			b.WriteString("<p>" + strings.Join(para, " ") + "</p>")
			para = nil
		}
	}
	inList := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == "":
			flush()
			if inList {
				b.WriteString("</ul>")
				inList = false
			}
		case strings.HasPrefix(t, "#+"):
			// Block delimiters and keywords say nothing to a reader.
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "+ "):
			flush()
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			b.WriteString("<li>" + inlineHTML(t[2:]) + "</li>")
		default:
			if inList {
				b.WriteString("</ul>")
				inList = false
			}
			para = append(para, inlineHTML(t))
		}
	}
	flush()
	if inList {
		b.WriteString("</ul>")
	}
	if cut {
		b.WriteString(`<p class="cut">…</p>`)
	}
	return template.HTML(b.String())
}

func (a *app) htmlCard(c *Card, now time.Time) htmlCard {
	b := a.board()
	hc := htmlCard{
		Title:       c.Headline,
		HeaderLight: HeaderColorOf(c, b, false),
		HeaderDark:  HeaderColorOf(c, b, true),
		File:        baseName(c.Filename),
		Clocked:     a.clockKey != "" && a.clockKey == c.Filename+"\x00"+c.Headline,
		Search:      Haystack(c),
	}
	if hc.Title == "" {
		hc.Title = "(no headline)"
	}
	if s := CardSwatch(c, b); s != nil {
		hc.Stripe = s.Hex
	}
	if c.Priority != "" {
		hc.Priority = strings.ToUpper(c.Priority)
		hc.PriorityBg = PriorityColor[hc.Priority]
		if hc.PriorityBg == "" {
			hc.PriorityBg = "#6b7280"
		}
		hc.PriorityFg = inkHex(hc.PriorityBg)
	}
	labels := LabelsOf(c, b)
	for _, v := range labels {
		col := LabelColorOf(v, b)
		hc.Labels = append(hc.Labels, htmlChip{Text: v, Bg: col, Fg: inkHex(col)})
	}
	if c.Status != "" && b.GroupBy != "status" {
		hc.Chips = append(hc.Chips, htmlChip{Text: c.Status, Class: "kw"})
	}
	for _, name := range b.Badges {
		if v := prop(c, name); v != "" {
			hc.Chips = append(hc.Chips, htmlChip{Text: v, Class: "badge"})
		}
	}
	for _, t := range c.Tags {
		if b.LabelSource == "tags" && contains(labels, t) {
			continue
		}
		hc.Chips = append(hc.Chips, htmlChip{Text: "#" + t, Fg: AutoSwatch(t).Hex, Class: "tag"})
	}
	if bd, ok := a.bodyOf(c.Hash); ok {
		for _, it := range checklistOf(bd.Text) {
			hc.Checks = append(hc.Checks, htmlCheck{Text: it.text, Done: it.done, Pad: it.depth})
			if it.done {
				hc.ChecksDone++
			}
		}
		if n := len(hc.Checks); n > 0 {
			hc.Percent = hc.ChecksDone * 100 / n
		}
		// The first lines of the prose show on the card; a click shows the
		// rest, and the checklist the count stands for.
		hc.Body = bodyHTML(bd.Text)
		hc.HasMore = hc.Body != "" || len(hc.Checks) > 0
	}
	if IsSet(c.Deadline) {
		hc.Deadline = DateLabel(c.Deadline.Start, now)
		hc.Overdue = overdue(c.Deadline.Start, now)
	}
	if IsSet(c.Date) {
		hc.Scheduled = DateLabel(c.Date.Start, now)
	}
	return hc
}

// htmlPageOf is the page for the board as laid out: the columns on show, in
// order, each with its cards sorted the board's way.
func (a *app) htmlPageOf(theme string, now time.Time) htmlPage {
	b := a.board()
	p := htmlPage{Generated: now.Format("Mon 2 Jan 2006, 15:04"), Theme: "default", Loose: a.loose, Problem: a.problem}
	switch theme {
	case "dark", "light":
		p.Force = theme
	case "":
	default:
		p.Theme = theme
	}
	if b == nil {
		return p
	}
	p.Board = b.Name
	p.Query = BoardQuery(b, a.stored)
	if b.StoredQuery != "" {
		p.Query = "saved: " + b.StoredQuery
	}
	p.List = b.Layout == "list"
	if p.List {
		p.Fields = ListFieldsOf(b)
	}
	for i, col := range a.cols {
		hcol := htmlColumn{Title: a.titleOf(col.Value), Color: a.colColor(col), Limit: col.Limit, Folded: a.folded(col.Value)}
		for _, c := range a.colCards(i) {
			c := c
			hc := a.htmlCard(&c, now)
			if p.List {
				hc.Status = c.Status
				for _, f := range p.Fields {
					hc.Cells = append(hc.Cells, ListFieldText(&c, f.Key))
				}
			}
			hcol.Cards = append(hcol.Cards, hc)
		}
		hcol.Over = col.Limit > 0 && len(hcol.Cards) > col.Limit
		p.Total += len(hcol.Cards)
		p.Columns = append(p.Columns, hcol)
	}
	return p
}

func themeCSS(name string) (string, bool) {
	switch name {
	case "default":
		return cssDefault, true
	case "paper":
		return cssPaper, true
	case "midnight":
		return cssMidnight, true
	}
	return "", false
}

// renderHTML writes the page. The theme's palette goes in front of the shared
// layout, so a theme is only colours and faces.
func renderHTML(p htmlPage) ([]byte, error) {
	css, ok := themeCSS(p.Theme)
	if !ok {
		return nil, fmt.Errorf("no html theme called %s - there is %s, and dark or light", p.Theme, strings.Join(HtmlThemes, ", "))
	}
	p.Css = template.CSS(css + cssLayout)
	var out bytes.Buffer
	if err := pageTpl.Execute(&out, p); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

var slugRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func htmlFileFor(board string) string {
	s := strings.Trim(slugRe.ReplaceAllString(board, "-"), "-")
	if s == "" {
		s = "board"
	}
	return s + ".html"
}

// html is -html: lay the board out as the terminal would, with every card's
// body read (for its checklist and prose), and write the page.
func (self *Kanban) html(a *app) {
	if a.board() == nil {
		commands.Fail("no boards yet - orgs kanban new <name> -q '<query>', or make one in worg")
	}
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err == nil {
		a.Scr = sim
		defer sim.Fini()
	}
	a.printing = true
	a.refresh()
	if a.problem != "" {
		commands.Fail("%s", a.problem)
	}
	theme := self.HtmlTheme
	if theme == "" {
		theme = self.Theme
	}
	page, err := renderHTML(a.htmlPageOf(theme, time.Now()))
	if err != nil {
		commands.Fail("%v", err)
	}
	out := self.Out
	if out == "" {
		out = htmlFileFor(a.board().Name)
	}
	type wrote struct {
		Ok      bool
		File    string
		Board   string
		Columns int
		Cards   int
		Bytes   int
	}
	cards := 0
	for i := range a.cols {
		cards += len(a.colCards(i))
	}
	res := wrote{Ok: true, File: out, Board: a.board().Name, Columns: len(a.cols), Cards: cards, Bytes: len(page)}
	if out == "-" {
		os.Stdout.Write(page)
		return
	}
	if commands.Wrote(fmt.Sprintf("write %s (%d cards, %d bytes)", out, cards, len(page)), nil) {
		return
	}
	if dir := filepath.Dir(out); dir != "." {
		os.MkdirAll(dir, 0o755)
	}
	if err := os.WriteFile(out, page, 0o644); err != nil {
		commands.Fail("could not write %s: %v", out, err)
	}
	if abs, err := filepath.Abs(out); err == nil {
		res.File = abs
	}
	if self.Open {
		commands.OpenInBrowser("file://" + filepath.ToSlash(res.File))
	}
	commands.RenderOne(res, func() {
		fmt.Fprintf(os.Stderr, "%s✓%s wrote %s %s(%d columns, %d cards)%s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset),
			out, commands.C(commands.AnsiDim), len(a.cols), cards, commands.C(commands.AnsiReset))
	})
}

var pageTpl = template.Must(template.New("board").Funcs(template.FuncMap{
	"css": func(s string) template.CSS { return template.CSS(s) },
	"count": func(n int) string {
		if n == 1 {
			return "1 card"
		}
		return fmt.Sprintf("%d cards", n)
	},
	"pad": func(n int) template.CSS { return template.CSS(fmt.Sprintf("padding-left:%.1fem", float64(n)*1.1)) },
}).Parse(pageHTML))

const pageHTML = `<!DOCTYPE html>
<html lang="en"{{if .Force}} data-theme="{{.Force}}"{{end}}>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="generator" content="orgs kanban">
<title>{{.Board}}</title>
<style>{{.Css}}</style>
</head>
<body class="theme-{{.Theme}}">
<header class="bar">
  <div class="heading">
    <h1>{{.Board}}</h1>
    <p class="meta"><span>{{count .Total}}</span> · <span>{{.Generated}}</span>{{if .Query}} · <code title="{{.Query}}">{{.Query}}</code>{{end}}</p>
  </div>
  <div class="tools">
    <input id="filter" type="search" placeholder="Filter cards…" aria-label="Filter cards" autocomplete="off">
    <button id="mode" type="button" title="Light or dark" aria-label="Switch light and dark">◐</button>
  </div>
</header>
{{if .Problem}}<p class="problem">{{.Problem}}</p>{{end}}
{{if .Loose}}<p class="note">{{count .Loose}} the query found {{if eq .Loose 1}}is{{else}}are{{end}} in no column of this board.</p>{{end}}
{{if .List}}
<main class="list">
{{range .Columns}}
  <section class="group{{if .Folded}} folded{{end}}" style="--col:{{css .Color}}">
    <h2 class="col-head" tabindex="0"><span class="dot"></span><span class="col-title">{{.Title}}</span> <span class="n">{{len .Cards}}</span>{{if .Limit}}<span class="limit{{if .Over}} over{{end}}">/ {{.Limit}}</span>{{end}}</h2>
    <table>
      <thead><tr><th>Heading</th><th>Keyword</th>{{range $.Fields}}<th>{{.Label}}</th>{{end}}</tr></thead>
      <tbody>
      {{range .Cards}}<tr class="card-row" data-search="{{.Search}}"><td class="item">{{if .Priority}}<span class="pri" style="background:{{css .PriorityBg}};color:{{css .PriorityFg}}">{{.Priority}}</span>{{end}}{{.Title}}</td><td>{{if .Status}}<span class="chip kw">{{.Status}}</span>{{end}}</td>{{range .Cells}}<td>{{.}}</td>{{end}}</tr>
      {{else}}<tr><td class="empty" colspan="2">Nothing here</td></tr>{{end}}
      </tbody>
    </table>
  </section>
{{end}}
</main>
{{else}}
<main class="board">
{{range .Columns}}
  <section class="column{{if .Folded}} folded{{end}}" style="--col:{{css .Color}}">
    <h2 class="col-head" tabindex="0" title="Fold or unfold">
      <span class="dot"></span><span class="col-title">{{.Title}}</span>
      <span class="n">{{len .Cards}}</span>{{if .Limit}}<span class="limit{{if .Over}} over{{end}}">/ {{.Limit}}</span>{{end}}
    </h2>
    <div class="cards">
    {{range .Cards}}
      <article class="card{{if .HasMore}} has-more{{end}}" tabindex="0" data-search="{{.Search}}" style="--hc-l:{{css .HeaderLight}};--hc-d:{{css .HeaderDark}}{{if .Stripe}};--stripe:{{css .Stripe}}{{end}}">
        {{if .Labels}}<div class="labels">{{range .Labels}}<span class="label" style="background:{{css .Bg}};color:{{css .Fg}}">{{.Text}}</span>{{end}}</div>{{end}}
        <h3 class="title">{{if .Clocked}}<span class="clock" title="Clocked in">◉</span>{{end}}{{if .Priority}}<span class="pri" style="background:{{css .PriorityBg}};color:{{css .PriorityFg}}">{{.Priority}}</span>{{end}}{{.Title}}</h3>
        {{if .Checks}}<div class="progress{{if eq .Percent 100}} complete{{end}}" title="{{.ChecksDone}} of {{len .Checks}} done"><span style="width:{{.Percent}}%"></span><em>☑ {{.ChecksDone}}/{{len .Checks}}</em></div>{{end}}
        {{if .Chips}}<div class="chips">{{range .Chips}}<span class="chip {{.Class}}"{{if .Fg}} style="--tag:{{css .Fg}}"{{end}}>{{.Text}}</span>{{end}}</div>{{end}}
        {{if .Body}}<div class="body">{{.Body}}</div>{{end}}
        {{if .Checks}}<ul class="checks more">{{range .Checks}}<li class="{{if .Done}}done{{end}}" style="{{pad .Pad}}"><span class="box">{{if .Done}}✓{{end}}</span>{{.Text}}</li>{{end}}</ul>{{end}}
        <footer>
          {{if .Deadline}}<span class="when deadline{{if .Overdue}} overdue{{end}}" title="Deadline">⚑ {{.Deadline}}</span>{{end}}
          {{if .Scheduled}}<span class="when" title="Scheduled">◷ {{.Scheduled}}</span>{{end}}
          <span class="file">{{.File}}</span>
        </footer>
      </article>
    {{else}}
      <p class="empty">Nothing here</p>
    {{end}}
    </div>
  </section>
{{end}}
</main>
{{end}}
<footer class="made">Made by <a href="https://github.com/ihdavids/orgs">orgs</a> from {{.Board}}.</footer>
<script>
(function () {
  var root = document.documentElement;
  try { var saved = localStorage.getItem('kanbanTheme'); if (saved && !root.dataset.theme) root.dataset.theme = saved; } catch (e) {}
  document.getElementById('mode').addEventListener('click', function () {
    var dark = root.dataset.theme ? root.dataset.theme === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches;
    root.dataset.theme = dark ? 'light' : 'dark';
    try { localStorage.setItem('kanbanTheme', root.dataset.theme); } catch (e) {}
  });
  // CardMatches: every word typed has to be somewhere in the card's text.
  var filter = document.getElementById('filter');
  filter.addEventListener('input', function () {
    var terms = filter.value.toLowerCase().split(/\s+/).filter(Boolean);
    document.querySelectorAll('.column, .group').forEach(function (col) {
      var shown = 0;
      col.querySelectorAll('[data-search]').forEach(function (card) {
        var hay = card.dataset.search;
        var ok = terms.every(function (t) { return hay.indexOf(t) >= 0; });
        card.hidden = !ok;
        if (ok) shown++;
      });
      col.querySelector('.n').textContent = shown;
    });
  });
  document.querySelectorAll('.col-head').forEach(function (h) {
    function fold() { h.parentNode.classList.toggle('folded'); }
    h.addEventListener('click', fold);
    h.addEventListener('keydown', function (e) { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); fold(); } });
  });
  document.querySelectorAll('.card.has-more').forEach(function (c) {
    function open(e) { if (e.target.closest('a')) return; c.classList.toggle('open'); }
    c.addEventListener('click', open);
    c.addEventListener('keydown', function (e) { if (e.key === 'Enter') open(e); });
  });
})();
</script>
</body>
</html>
`
