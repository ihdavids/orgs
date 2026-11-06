package jcanvas

// What a node shows, once the server has looked at what it names. Wire type
// of GET /canvas (Cards, by node id) and the input to the SVG and the
// outline.

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
)

type Card struct {
	// heading | org (a whole org file) | image | file | link | text | group |
	// missing (a file or heading that is not there)
	Kind     string
	Title    string   `json:",omitempty"`
	Keyword  string   `json:",omitempty"`
	Done     bool     `json:",omitempty"` // the keyword is a done one
	Priority string   `json:",omitempty"`
	Tags     []string `json:",omitempty"`
	// The body, rendered by the html exporter (headings, org files, text).
	Html string `json:",omitempty"`
	// The body as plain lines, for the SVG and the CLI.
	Plain string `json:",omitempty"`
	// Where it is: an org heading's hash, file (absolute) and line (from 0).
	Hash     string   `json:",omitempty"`
	Filename string   `json:",omitempty"`
	Line     int      `json:",omitempty"`
	Olp      []string `json:",omitempty"`
	// A picture's url (a path under /images), or the file's when it is served.
	Src string `json:",omitempty"`
	// Found by its id in another file than the node says: the node's file is
	// out of date (a save from worg puts the new one in).
	Moved bool   `json:",omitempty"`
	File  string `json:",omitempty"` // the file the node should say, when Moved
	Msg   string `json:",omitempty"`
}

var mdMarkRe = regexp.MustCompile("(\\*\\*|__|~~|==|`)")
var mdLinkRe = regexp.MustCompile(`!?\[([^\]]*)\]\(([^)]*)\)`)
var orgLinkRe = regexp.MustCompile(`\[\[([^\]]*)\](?:\[([^\]]*)\])?\]`)
var wikiLinkRe = regexp.MustCompile(`\[\[([^\]|]*)\|([^\]]*)\]\]`)
var headingLineRe = regexp.MustCompile(`^(#{1,6}|\*+)\s+`)

// org emphasis: *bold* /italic/ =verbatim= ~code~ +strike+ _under_
var orgEmphRe = regexp.MustCompile(`(^|[\s('"])([*/=~+_])([^\s*/=~+_](?:[^\n]*?[^\s])?)([*/=~+_])([\s.,;:!?)'"]|$)`)

// PlainText takes a card's text (markdown or org) down to lines a person can
// read without either: links become their words, emphasis marks go, a
// heading's # or * goes. Good enough for a drawing and a terminal; worg and
// the exporters render the real thing.
func PlainText(s string) []string {
	out := []string{}
	inFence := false
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		t := strings.TrimRight(l, " \t")
		low := strings.ToLower(strings.TrimSpace(t))
		if strings.HasPrefix(low, "```") || strings.HasPrefix(low, "#+begin_") || strings.HasPrefix(low, "#+end_") {
			inFence = !strings.HasPrefix(low, "#+end_") && !(inFence && strings.HasPrefix(low, "```"))
			continue
		}
		if !inFence {
			if strings.HasPrefix(strings.TrimSpace(t), "#+") || strings.HasPrefix(strings.TrimSpace(t), ":") && strings.HasSuffix(strings.TrimSpace(t), ":") {
				continue
			}
			t = headingLineRe.ReplaceAllString(t, "")
			t = wikiLinkRe.ReplaceAllString(t, "$2")
			t = orgLinkRe.ReplaceAllStringFunc(t, func(m string) string {
				p := orgLinkRe.FindStringSubmatch(m)
				if p[2] != "" {
					return p[2]
				}
				// A bare file link reads as the file's name.
				if strings.HasPrefix(p[1], "file:") {
					return path.Base(strings.TrimPrefix(p[1], "file:"))
				}
				return p[1]
			})
			t = mdLinkRe.ReplaceAllString(t, "$1")
			t = mdMarkRe.ReplaceAllString(t, "")
			// Twice: a match takes the space after it, which the next needs.
			for k := 0; k < 2; k++ {
				t = orgEmphRe.ReplaceAllStringFunc(t, func(m string) string {
					p := orgEmphRe.FindStringSubmatch(m)
					if p[2] != p[4] {
						return m
					}
					return p[1] + p[3] + p[5]
				})
			}
		}
		out = append(out, t)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	return out
}

// Label is a node in a few words: the outline's and the CLI's name for it.
func Label(n Node, c *Card) string {
	switch {
	case c != nil && (c.Kind == "heading" || c.Kind == "org") && c.Title != "":
		t := c.Title
		if c.Keyword != "" {
			t = c.Keyword + " " + t
		}
		return t
	case n.Type == "text":
		for _, l := range PlainText(n.Text) {
			if strings.TrimSpace(l) != "" {
				return clip(strings.TrimSpace(l), 60)
			}
		}
		return "(empty)"
	case n.Type == "file":
		return path.Base(n.File) + n.Subpath
	case n.Type == "link":
		return n.URL
	case n.Type == "group":
		if n.Label != "" {
			return n.Label
		}
		return "(group)"
	}
	return n.Type
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Host is a link card's site, for its subtitle.
func Host(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return strings.TrimPrefix(p.Host, "www.")
	}
	return u
}

// Outline is a canvas as text: each group with what is in it (nested groups
// inside), the cards in no group, then the edges with their labels.
func Outline(c *Canvas, cards map[string]Card) string {
	var b strings.Builder
	parent := make([]int, len(c.Nodes))
	kids := map[int][]int{}
	for i := range c.Nodes {
		parent[i] = c.GroupOf(i)
		kids[parent[i]] = append(kids[parent[i]], i)
	}
	for k := range kids {
		c.Order(kids[k])
	}
	var walk func(p int, depth int)
	walk = func(p int, depth int) {
		// Groups after cards at each level, so a group's cards read as its own.
		ids := append([]int{}, kids[p]...)
		sort.SliceStable(ids, func(a, b int) bool {
			return c.Nodes[ids[a]].Type != "group" && c.Nodes[ids[b]].Type == "group"
		})
		for _, i := range ids {
			n := c.Nodes[i]
			card, ok := cards[n.ID]
			var cp *Card
			if ok {
				cp = &card
			}
			ind := strings.Repeat("  ", depth)
			col := ""
			if n.Color != "" {
				col = " (" + colorName(n.Color) + ")"
			}
			if n.Type == "group" {
				fmt.Fprintf(&b, "%s[%s]%s\n", ind, Label(n, cp), col)
				walk(i, depth+1)
				continue
			}
			label := Label(n, cp)
			if n.Type == "text" {
				label = "\"" + label + "\""
			}
			fmt.Fprintf(&b, "%s- %s%s%s\n", ind, kindMark(n, cp), label, col+tagsOf(cp)+whereOf(n, cp))
		}
	}
	walk(-1, 0)
	if len(c.Edges) > 0 {
		b.WriteString("\nEdges:\n")
		for _, e := range c.Edges {
			fmt.Fprintf(&b, "  %s\n", EdgeText(c, e, cards))
		}
	}
	return b.String()
}

// EdgeText is one edge as `A --label--> B`.
func EdgeText(c *Canvas, e Edge, cards map[string]Card) string {
	name := func(id string) string {
		n := c.Node(id)
		if n == nil {
			return "?" + id
		}
		card, ok := cards[id]
		if ok {
			return Label(*n, &card)
		}
		return Label(*n, nil)
	}
	head, tail := "", ""
	if e.ToEndOr() == "arrow" {
		head = ">"
	}
	if e.FromEndOr() == "arrow" {
		tail = "<"
	}
	mid := "--"
	if e.Label != "" {
		mid = "-- " + e.Label + " --"
	}
	col := ""
	if e.Color != "" {
		col = "  (" + colorName(e.Color) + ")"
	}
	return fmt.Sprintf("%s %s%s%s %s%s", name(e.FromNode), tail, mid, head, name(e.ToNode), col)
}

func colorName(c string) string {
	for name, n := range PresetNames {
		if n == c {
			return name
		}
	}
	return c
}

func kindMark(n Node, c *Card) string {
	switch {
	case c != nil && c.Kind == "missing":
		return "(missing) "
	case n.Type == "link":
		return "link: "
	case n.Type == "file" && (c == nil || (c.Kind != "heading" && c.Kind != "org")):
		return "file: "
	}
	return ""
}

func tagsOf(c *Card) string {
	if c == nil || len(c.Tags) == 0 {
		return ""
	}
	return "  :" + strings.Join(c.Tags, ":") + ":"
}

func whereOf(n Node, c *Card) string {
	if n.Type == "file" && c != nil && (c.Kind == "heading") {
		if c.Filename != "" {
			return "  (" + path.Base(strings.ReplaceAll(c.Filename, "\\", "/")) + ")"
		}
		return "  (" + path.Base(ParseRef(n.File, "").Path) + ")"
	}
	return ""
}
