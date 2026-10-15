package mdorg

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// conv converts one note.
type conv struct {
	ix       *index
	from     string // the note's path in the vault
	assets   map[string]bool
	tags     map[string]bool
	stats    *Stats
	warnings []string
	// [ref]: url definitions, for [text][ref] links.
	refs map[string]string
	// Shift every heading up by this many levels (the note's lone H1 became
	// its title).
	shift int
}

var (
	atxRe      = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	setext1Re  = regexp.MustCompile(`^ {0,3}=+[ \t]*$`)
	setext2Re  = regexp.MustCompile(`^ {0,3}-+[ \t]*$`)
	fenceRe    = regexp.MustCompile("^([ \\t]*)(`{3,}|~{3,})[ \\t]*([^`\\s]*)?.*$")
	hrRe       = regexp.MustCompile(`^ {0,3}((\*[ \t]*){3,}|(-[ \t]*){3,}|(_[ \t]*){3,})$`)
	bulletRe   = regexp.MustCompile(`^([ \t]*)([-*+])([ \t]+)(.*)$`)
	orderedRe  = regexp.MustCompile(`^([ \t]*)(\d{1,9})([.)])([ \t]+)(.*)$`)
	taskRe     = regexp.MustCompile(`^\[([ xX/\-><!?*])\][ \t]+`)
	quoteRe    = regexp.MustCompile(`^ {0,3}>[ ]?(.*)$`)
	calloutRe  = regexp.MustCompile(`^\[!([A-Za-z-]+)\]([+-]?)[ \t]*(.*)$`)
	refDefRe   = regexp.MustCompile(`^ {0,3}\[([^\]^][^\]]*)\]:[ \t]*<?(\S+?)>?(?:[ \t]+["'(].*["')])?[ \t]*$`)
	fnDefRe    = regexp.MustCompile(`^\[\^([^\]]+)\]:[ \t]?(.*)$`)
	tableSepRe = regexp.MustCompile(`^[ \t]*\|?[ \t]*:?-{1,}:?[ \t]*(\|[ \t]*:?-{1,}:?[ \t]*)*\|?[ \t]*$`)
	mathFence  = regexp.MustCompile(`^[ \t]*\$\$[ \t]*$`)
	commentRe  = regexp.MustCompile(`^[ \t]*%%[ \t]*$`)
	htmlComRe  = regexp.MustCompile(`^[ \t]*<!--`)
	embedLine  = regexp.MustCompile(`^[ \t]*!\[\[([^\]]+)\]\][ \t]*$`)
)

// headingsOf is a note's heading titles, for resolving #heading links.
func headingsOf(text string) []string {
	out := []string{}
	fence := ""
	for _, l := range strings.Split(text, "\n") {
		if m := fenceRe.FindStringSubmatch(l); m != nil {
			if fence == "" {
				fence = m[2][:1]
			} else if strings.HasPrefix(strings.TrimSpace(l), strings.Repeat(fence, 3)) {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		if m := atxRe.FindStringSubmatch(l); m != nil && m[2] != "" {
			out = append(out, strings.TrimSpace(stripInline(m[2])))
		}
	}
	return out
}

// stripInline is a heading's text as org will title it: emphasis and links
// gone, which is what a [[file:x.org::*Title]] search has to match.
func stripInline(s string) string {
	s = wikiRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := wikiRe.FindStringSubmatch(m)
		if sm[4] != "" {
			return sm[4]
		}
		return sm[2]
	})
	s = mdLinkRe.ReplaceAllString(s, "$2")
	return strings.NewReplacer("**", "", "__", "", "*", "", "`", "", "==", "", "~~", "").Replace(s)
}

// frontMatter splits off a leading --- yaml block.
func frontMatter(text string) (map[string]any, []string, string) {
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return nil, nil, text
	}
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		t := strings.TrimRight(lines[i], "\r")
		if t == "---" || t == "..." {
			fm := map[string]any{}
			node := yaml.Node{}
			src := strings.Join(lines[1:i], "\n")
			if err := yaml.Unmarshal([]byte(src), &fm); err != nil {
				return nil, nil, text
			}
			// Keys in the order written, which yaml into a map loses.
			order := []string{}
			if yaml.Unmarshal([]byte(src), &node) == nil && len(node.Content) > 0 {
				for j := 0; j+1 < len(node.Content[0].Content); j += 2 {
					order = append(order, node.Content[0].Content[j].Value)
				}
			}
			return fm, order, strings.Join(lines[i+1:], "\n")
		}
	}
	return nil, nil, text
}

func asList(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		out := []string{}
		for _, w := range strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ' ' }) {
			if w = strings.TrimSpace(w); w != "" {
				out = append(out, w)
			}
		}
		return out
	case []any:
		out := []string{}
		for _, x := range t {
			if s := strings.TrimSpace(fmt.Sprint(x)); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return []string{fmt.Sprint(v)}
}

func scalar(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return strings.Join(strings.Fields(t), " "), true
	case time.Time:
		if t.Hour() == 0 && t.Minute() == 0 {
			return t.Format("2006-01-02"), true
		}
		return t.Format("2006-01-02 15:04"), true
	case int, int64, float64, bool:
		return fmt.Sprint(t), true
	}
	return "", false
}

// orgDate writes a date from front matter as an inactive org stamp, which is
// how #+DATE: is written; anything it cannot read is kept as written.
func orgDate(s string) string {
	for _, f := range []string{"2006-01-02 15:04", "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(f, s); err == nil {
			if f == "2006-01-02" {
				return "[" + t.Format("2006-01-02 Mon") + "]"
			}
			return "[" + t.Format("2006-01-02 Mon 15:04") + "]"
		}
	}
	return s
}

// OrgTag makes an Obsidian tag (nested/tag, kebab-case) an org tag.
func OrgTag(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	return strings.Trim(badTag.ReplaceAllString(s, "_"), "_")
}

var badTag = regexp.MustCompile(`[^\p{L}\p{N}_@#%]+`)

// note converts a whole note: front matter into the file's head, the body
// into org.
func (c *conv) note(text string) string {
	text = strings.ReplaceAll(strings.TrimPrefix(text, "\ufeff"), "\r\n", "\n")
	fm, order, body := frontMatter(text)
	title := ""
	date := ""
	desc := ""
	aliases := []string{}
	props := [][2]string{}
	for _, k := range order {
		v := fm[k]
		switch strings.ToLower(k) {
		case "title":
			title, _ = scalar(v)
		case "tags", "tag", "keywords":
			for _, t := range asList(v) {
				if t = OrgTag(t); t != "" {
					c.tags[t] = true
				}
			}
		case "aliases", "alias":
			aliases = append(aliases, asList(v)...)
		case "date", "created", "created_at", "creation date":
			if s, ok := scalar(v); ok && date == "" {
				date = orgDate(s)
			} else if ok {
				props = append(props, [2]string{strings.ToUpper(k), orgDate(s)})
			}
		case "description", "summary":
			desc, _ = scalar(v)
		default:
			if s, ok := scalar(v); ok && s != "" {
				props = append(props, [2]string{propName(k), s})
			} else if l := asList(v); len(l) > 0 {
				props = append(props, [2]string{propName(k), strings.Join(l, " ")})
			}
		}
	}

	lines := c.collectRefs(strings.Split(body, "\n"))
	// A note that opens with its one level-one heading has said its title;
	// the rest move up a level so the shallowest heading is `*`.
	subtitle := ""
	if t, rest, ok := leadingTitle(lines); ok {
		if title == "" {
			title = t
		} else if !strings.EqualFold(t, title) {
			// Front matter named the title; the heading said something else,
			// which is kept rather than dropped.
			subtitle = t
		}
		lines = rest
	}
	// The shallowest heading left is the file's `*`: a note of `##` sections
	// is a file of top level headings, not of headings under nothing.
	if m := minLevel(lines); m > 1 {
		c.shift = m - 1
	}
	if title == "" {
		base := path.Base(c.from)
		title = strings.TrimSuffix(base, path.Ext(base))
	}
	conv := c.block(lines, "")

	// Inline #tags found in the text are tags of the file.
	tagList := []string{}
	for t := range c.tags {
		tagList = append(tagList, t)
	}
	sort.Strings(tagList)

	b := strings.Builder{}
	if len(aliases) > 0 || len(props) > 0 {
		b.WriteString(":PROPERTIES:\n")
		if len(aliases) > 0 {
			q := []string{}
			for _, a := range aliases {
				q = append(q, fmt.Sprintf("%q", a))
			}
			b.WriteString(":ROAM_ALIASES: " + strings.Join(q, " ") + "\n")
		}
		for _, kv := range props {
			b.WriteString(":" + kv[0] + ": " + kv[1] + "\n")
		}
		b.WriteString(":END:\n")
	}
	b.WriteString("#+TITLE: " + title + "\n")
	if subtitle != "" {
		b.WriteString("#+SUBTITLE: " + subtitle + "\n")
	}
	if date != "" {
		b.WriteString("#+DATE: " + date + "\n")
	}
	if desc != "" {
		b.WriteString("#+DESCRIPTION: " + desc + "\n")
	}
	if len(tagList) > 0 {
		b.WriteString("#+FILETAGS: :" + strings.Join(tagList, ":") + ":\n")
	}
	if strings.TrimSpace(conv) != "" {
		b.WriteString("\n" + strings.TrimLeft(conv, "\n"))
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

var propBad = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func propName(k string) string {
	return strings.ToUpper(strings.Trim(propBad.ReplaceAllString(k, "_"), "_"))
}

// leadingTitle is a note's first line when it is its only level-one heading.
func leadingTitle(lines []string) (string, []string, bool) {
	first := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = i
			break
		}
	}
	if first < 0 {
		return "", lines, false
	}
	m := atxRe.FindStringSubmatch(lines[first])
	if m == nil || m[1] != "#" || m[2] == "" {
		return "", lines, false
	}
	fence := ""
	for _, l := range lines[first+1:] {
		if fm := fenceRe.FindStringSubmatch(l); fm != nil {
			if fence == "" {
				fence = fm[2][:1]
			} else if strings.HasPrefix(strings.TrimSpace(l), strings.Repeat(fence, 3)) {
				fence = ""
			}
		}
		if fence == "" {
			if h := atxRe.FindStringSubmatch(l); h != nil && h[1] == "#" {
				return "", lines, false
			}
		}
	}
	return strings.TrimSpace(stripInline(m[2])), lines[first+1:], true
}

// minLevel is the shallowest heading level in a run of lines, 0 for none.
func minLevel(lines []string) int {
	min := 0
	fence := ""
	for i, l := range lines {
		if m := fenceRe.FindStringSubmatch(l); m != nil {
			if fence == "" {
				fence = m[2][:1]
			} else if strings.HasPrefix(strings.TrimSpace(l), strings.Repeat(fence, 3)) {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		lvl := 0
		if m := atxRe.FindStringSubmatch(l); m != nil && m[2] != "" {
			lvl = len(m[1])
		} else if strings.TrimSpace(l) != "" && i+1 < len(lines) && !isStructural(l) {
			if setext1Re.MatchString(lines[i+1]) {
				lvl = 1
			} else if setext2Re.MatchString(lines[i+1]) {
				lvl = 2
			}
		}
		if lvl > 0 && (min == 0 || lvl < min) {
			min = lvl
		}
	}
	return min
}

// collectRefs takes the [ref]: url definitions out, for [text][ref] links.
func (c *conv) collectRefs(lines []string) []string {
	c.refs = map[string]string{}
	out := []string{}
	fence := ""
	for _, l := range lines {
		if m := fenceRe.FindStringSubmatch(l); m != nil {
			if fence == "" {
				fence = m[2][:1]
			} else if strings.HasPrefix(strings.TrimSpace(l), strings.Repeat(fence, 3)) {
				fence = ""
			}
		}
		if fence == "" {
			if m := refDefRe.FindStringSubmatch(l); m != nil {
				c.refs[strings.ToLower(m[1])] = m[2]
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

// protectBlockLine escapes a line inside a source or example block that org
// would otherwise read as a heading or a keyword: org's own comma escape.
func protectBlockLine(l string) string {
	t := strings.TrimLeft(l, " \t")
	if strings.HasPrefix(t, "*") && l == t || strings.HasPrefix(t, "#+") || strings.HasPrefix(t, ",*") || strings.HasPrefix(t, ",#+") {
		return l[:len(l)-len(t)] + "," + t
	}
	return l
}

func expandTabs(l string) string {
	i := 0
	for i < len(l) && (l[i] == ' ' || l[i] == '\t') {
		i++
	}
	return strings.ReplaceAll(l[:i], "\t", "  ") + l[i:]
}

// block converts a run of markdown lines (a note's body, a quote's inside);
// indent is put before every line written.
func (c *conv) block(lines []string, indent string) string {
	b := strings.Builder{}
	w := func(s string) { b.WriteString(indent + s + "\n") }
	blank := func() { b.WriteString("\n") }
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		l := expandTabs(raw)
		t := strings.TrimSpace(l)

		// Fenced code: a source block, its lines escaped.
		if m := fenceRe.FindStringSubmatch(l); m != nil {
			mark := m[2]
			lang := strings.ToLower(m[3])
			pad := m[1]
			body := []string{}
			j := i + 1
			for ; j < len(lines); j++ {
				lt := strings.TrimSpace(lines[j])
				if strings.HasPrefix(lt, mark[:1]) && strings.Trim(lt, mark[:1]) == "" && len(lt) >= len(mark) {
					break
				}
				body = append(body, strings.TrimPrefix(lines[j], pad))
			}
			c.stats.Blocks++
			switch lang {
			case "":
				w(pad + "#+begin_example")
				for _, x := range body {
					w(pad + protectBlockLine(x))
				}
				w(pad + "#+end_example")
			case "math", "latex", "tex":
				if lang == "math" {
					w(pad + "\\[")
					for _, x := range body {
						w(pad + x)
					}
					w(pad + "\\]")
					break
				}
				fallthrough
			default:
				w(pad + "#+begin_src " + srcLang(lang))
				for _, x := range body {
					w(pad + protectBlockLine(x))
				}
				w(pad + "#+end_src")
			}
			i = j
			continue
		}
		// $$ display math $$
		if mathFence.MatchString(l) {
			j := i + 1
			w("\\[")
			for ; j < len(lines) && !mathFence.MatchString(lines[j]); j++ {
				w(lines[j])
			}
			w("\\]")
			i = j
			continue
		}
		if strings.HasPrefix(t, "$$") && strings.HasSuffix(t, "$$") && len(t) > 4 {
			w("\\[" + strings.TrimSpace(t[2:len(t)-2]) + "\\]")
			continue
		}
		// %% Obsidian comments %% and <!-- html comments -->
		if commentRe.MatchString(l) || (strings.HasPrefix(t, "%%") && !strings.HasSuffix(t[2:], "%%")) {
			body := []string{}
			first := strings.TrimSpace(strings.TrimPrefix(t, "%%"))
			if first != "" {
				body = append(body, first)
			}
			j := i + 1
			for ; j < len(lines); j++ {
				lt := strings.TrimSpace(lines[j])
				if strings.HasSuffix(lt, "%%") {
					if s := strings.TrimSpace(strings.TrimSuffix(lt, "%%")); s != "" {
						body = append(body, s)
					}
					break
				}
				body = append(body, lines[j])
			}
			w("#+begin_comment")
			for _, x := range body {
				w(protectBlockLine(x))
			}
			w("#+end_comment")
			i = j
			continue
		}
		if strings.HasPrefix(t, "%%") && strings.HasSuffix(t, "%%") && len(t) >= 4 {
			w("# " + strings.TrimSpace(t[2:len(t)-2]))
			continue
		}
		if htmlComRe.MatchString(l) {
			body := []string{}
			j := i
			for ; j < len(lines); j++ {
				body = append(body, lines[j])
				if strings.Contains(lines[j], "-->") {
					break
				}
			}
			text := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(strings.Join(body, "\n")), "<!--"), "-->"))
			if !strings.Contains(text, "\n") {
				w("# " + text)
			} else {
				w("#+begin_comment")
				for _, x := range strings.Split(text, "\n") {
					w(protectBlockLine(x))
				}
				w("#+end_comment")
			}
			i = j
			continue
		}
		// Headings.
		if m := atxRe.FindStringSubmatch(l); m != nil && (m[2] != "" || len(m[1]) > 0) && !strings.HasPrefix(t, "#[") {
			if m[2] == "" && i+1 < len(lines) {
				// "#" alone: an empty heading, written as one.
			}
			lvl := len(m[1]) - c.shift
			if lvl < 1 {
				lvl = 1
			}
			text, id := blockID(m[2])
			line := strings.Repeat("*", lvl) + " " + c.inline(text)
			w(strings.TrimRight(line, " "))
			if id != "" {
				w("<<" + id + ">>")
			}
			continue
		}
		// Setext headings: a line of text over === or ---.
		if t != "" && i+1 < len(lines) && !isStructural(l) {
			next := lines[i+1]
			if setext1Re.MatchString(next) || (setext2Re.MatchString(next) && strings.TrimSpace(next) != "") {
				lvl := 1
				if setext2Re.MatchString(next) && !setext1Re.MatchString(next) {
					lvl = 2
				}
				lvl -= c.shift
				if lvl < 1 {
					lvl = 1
				}
				w(strings.Repeat("*", lvl) + " " + c.inline(t))
				i++
				continue
			}
		}
		if hrRe.MatchString(l) {
			w("-----")
			continue
		}
		// Block quotes and callouts: the inside converted as a block of its own.
		if quoteRe.MatchString(l) {
			inner := []string{}
			j := i
			for ; j < len(lines); j++ {
				m := quoteRe.FindStringSubmatch(lines[j])
				if m == nil {
					break
				}
				inner = append(inner, m[1])
			}
			i = j - 1
			if len(inner) > 0 {
				if cm := calloutRe.FindStringSubmatch(strings.TrimSpace(inner[0])); cm != nil {
					c.stats.Callouts++
					kind := strings.ToLower(cm[1])
					w("#+begin_" + kind)
					if cm[3] != "" {
						w("*" + c.inline(cm[3]) + "*")
					}
					b.WriteString(c.block(inner[1:], indent))
					w("#+end_" + kind)
					continue
				}
			}
			w("#+begin_quote")
			b.WriteString(c.block(inner, indent))
			w("#+end_quote")
			continue
		}
		// Tables: a row, then a |---|---| rule.
		if strings.Contains(l, "|") && i+1 < len(lines) && tableSepRe.MatchString(lines[i+1]) && strings.Contains(lines[i+1], "-") {
			c.stats.Tables++
			w(c.row(l))
			w(ruleFor(lines[i+1]))
			j := i + 2
			for ; j < len(lines) && strings.Contains(lines[j], "|") && strings.TrimSpace(lines[j]) != ""; j++ {
				w(c.row(lines[j]))
			}
			i = j - 1
			continue
		}
		// Footnote definitions start in column zero in org.
		if m := fnDefRe.FindStringSubmatch(t); m != nil && len(l) == len(t) {
			w("[fn:" + fnName(m[1]) + "] " + c.inline(m[2]))
			continue
		}
		// An embed on its own line: a picture, or a note included in place.
		if m := embedLine.FindStringSubmatch(l); m != nil {
			if s, ok := c.embedBlock(m[1]); ok {
				w(s)
				continue
			}
		}
		// Lists.
		if m := bulletRe.FindStringSubmatch(l); m != nil {
			w(m[1] + "- " + c.item(m[4]))
			continue
		}
		if m := orderedRe.FindStringSubmatch(l); m != nil {
			w(m[1] + m[2] + "." + " " + c.item(m[5]))
			continue
		}
		if t == "" {
			blank()
			continue
		}
		text, id := blockID(l)
		out := c.inline(text)
		if id != "" {
			out += " <<" + id + ">>"
		}
		w(strings.TrimRight(out, " "))
	}
	return b.String()
}

// isStructural says a line is something other than paragraph text, so it
// cannot be the text of a setext heading.
func isStructural(l string) bool {
	return bulletRe.MatchString(l) || orderedRe.MatchString(l) || quoteRe.MatchString(l) ||
		atxRe.MatchString(l) || hrRe.MatchString(l) || strings.HasPrefix(strings.TrimSpace(l), "|")
}

var blockIDRe = regexp.MustCompile(`[ \t]+\^([A-Za-z0-9-]+)[ \t]*$`)

// blockID takes Obsidian's ^block-id off the end of a line; it becomes an
// org target, which [[file:x.org::block-id]] finds.
func blockID(l string) (string, string) {
	if m := blockIDRe.FindStringSubmatchIndex(l); m != nil {
		return l[:m[0]], l[m[2]:m[3]]
	}
	return l, ""
}

func fnName(s string) string {
	s = strings.TrimSpace(s)
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == ']' || r == ':' {
			return '-'
		}
		return r
	}, s)
}

// item converts a list item's text: its checkbox, and Obsidian Tasks dates.
func (c *conv) item(s string) string {
	box := ""
	if m := taskRe.FindStringSubmatch(s); m != nil {
		c.stats.Tasks++
		switch m[1] {
		case " ":
			box = "[ ] "
		case "x", "X":
			box = "[X] "
		default:
			box = "[-] "
		}
		s = s[len(m[0]):]
	}
	return box + c.inline(taskDates(s))
}

var taskDateRe = regexp.MustCompile(`(📅|⏳|🛫|✅|➕|❌)️?[ \t]*(\d{4}-\d{2}-\d{2})`)

// taskDates writes the Obsidian Tasks plugin's dates as org stamps: due,
// scheduled and start as active ones (so the agenda sees them), done and
// created as inactive.
func taskDates(s string) string {
	return taskDateRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := taskDateRe.FindStringSubmatch(m)
		t, err := time.Parse("2006-01-02", sm[2])
		if err != nil {
			return m
		}
		d := t.Format("2006-01-02 Mon")
		switch sm[1] {
		case "📅":
			return "due <" + d + ">"
		case "⏳":
			return "scheduled <" + d + ">"
		case "🛫":
			return "starts <" + d + ">"
		case "✅":
			return "done [" + d + "]"
		case "❌":
			return "cancelled [" + d + "]"
		}
		return "created [" + d + "]"
	})
}

// srcLang maps markdown fence names to the names org babel uses.
func srcLang(l string) string {
	switch l {
	case "sh", "bash", "zsh", "shell", "console":
		return "sh"
	case "js", "javascript":
		return "js"
	case "ts", "typescript":
		return "typescript"
	case "py", "python3":
		return "python"
	case "yml":
		return "yaml"
	case "elisp", "emacs-lisp":
		return "emacs-lisp"
	case "dataview", "dataviewjs", "query", "tasks":
		return l
	}
	return l
}

func ruleFor(sep string) string {
	n := len(splitRow(sep))
	if n < 1 {
		n = 1
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "---"
	}
	return "|" + strings.Join(parts, "+") + "|"
}

// splitRow splits a table row at its unescaped pipes.
func splitRow(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "|")
	if strings.HasSuffix(l, "|") && !strings.HasSuffix(l, `\|`) {
		l = l[:len(l)-1]
	}
	cells := []string{}
	cur := strings.Builder{}
	code := false
	for i := 0; i < len(l); i++ {
		switch {
		case l[i] == '\\' && i+1 < len(l) && l[i+1] == '|':
			cur.WriteByte('|')
			i++
		case l[i] == '`':
			code = !code
			cur.WriteByte('`')
		case l[i] == '|' && !code:
			cells = append(cells, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(l[i])
		}
	}
	return append(cells, cur.String())
}

func (c *conv) row(l string) string {
	cells := []string{}
	for _, cell := range splitRow(l) {
		// A pipe inside a cell would split it in org; org has no escape for
		// one, so it is written as the entity.
		v := strings.ReplaceAll(c.inline(strings.TrimSpace(cell)), "|", `\vert{}`)
		cells = append(cells, v)
	}
	return "| " + strings.Join(cells, " | ") + " |"
}
