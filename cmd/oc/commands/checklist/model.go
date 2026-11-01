package checklist

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// Item is one box. Index counts every box in the heading's own body, nested
// ones included, which is how POST /checklist and /checklist/edit count them.
type Item struct {
	Index int
	Depth int
	// " " open, "X" ticked, "-" part done (org writes it on a parent whose
	// children are partly ticked).
	State string
	Text  string
	// Where it is written, 1-based, for an editor.
	Line int
}

func (i Item) Done() bool  { return i.State != " " && i.State != "-" }
func (i Item) Doing() bool { return i.State == "-" }

// List is a heading with boxes in its own body.
type List struct {
	Hash  string
	Title string
	File  string
	Line  int
	Done  int
	Doing int
	Total int
	Items []Item
}

func (l List) Open() int { return l.Total - l.Done }

// The same shape the server matches (checkItem in checklist.go). The two have
// to agree, because this numbers the boxes and the server counts them again to
// find the line.
//
// first is the file line (1-based) the body starts on.
func parseItems(body string, first int) []Item {
	type raw struct {
		indent int
		state  string
		text   string
		line   int
	}
	rows := []raw{}
	for n, line := range strings.Split(body, "\n") {
		m := checkItemRe.FindStringSubmatch(line)
		if m == nil || (m[2] == "*" && m[1] == "") {
			continue
		}
		state := m[3]
		if state == "x" {
			state = "X"
		}
		rows = append(rows, raw{indentWidth(m[1]), state, m[4], first + n})
	}
	// Depth is the indent's rank among the indents used, so a list written two
	// spaces in and one written four in both start at depth 0.
	widths := map[int]bool{}
	for _, r := range rows {
		widths[r.indent] = true
	}
	ranks := []int{}
	for w := range widths {
		ranks = append(ranks, w)
	}
	sort.Ints(ranks)
	depth := map[int]int{}
	for i, w := range ranks {
		depth[w] = i
	}
	out := make([]Item, 0, len(rows))
	for i, r := range rows {
		out = append(out, Item{Index: i, Depth: depth[r.indent], State: r.state, Text: r.text, Line: r.line})
	}
	return out
}

func indentWidth(s string) int {
	n := 0
	for _, r := range s {
		if r == '\t' {
			n += 8
		} else {
			n++
		}
	}
	return n
}

type bodyResult struct {
	Ok   bool
	Msg  string
	Text string
}

// load reads a heading's boxes from the file as it is now.
func load(core *commands.Core, t common.Todo) (List, error) {
	l := List{Hash: t.Hash, Title: t.Headline, File: t.Filename, Line: t.LineNum}
	b, err := commands.SendReceiveGetErr[bodyResult](core, "body/"+commands.HashPath(t.Hash), nil)
	if err != nil {
		return l, err
	}
	if !b.Ok {
		return l, fmt.Errorf("%s", b.Msg)
	}
	// The body is the lines under the headline; Line is the headline's row
	// counted from 0, so the body's first line is Line+2 counted from 1.
	l.Items = parseItems(b.Text, l.Line+2)
	for _, it := range l.Items {
		l.Total++
		if it.Done() {
			l.Done++
		}
		if it.Doing() {
			l.Doing++
		}
	}
	return l, nil
}

// listQuery is the query for the lists matching what was typed: plain words
// look for a heading with them in it, anything with a call or an operator is
// a query already. Either way only headings with boxes.
func listQuery(words []string) string {
	text := strings.TrimSpace(strings.Join(words, " "))
	switch {
	case text == "":
		return "HasChecklist()"
	case strings.ContainsAny(text, "()&|!=<>"):
		return "HasChecklist() && (" + text + ")"
	}
	// Letters, digits and spaces as they are; anything else is "any
	// character", which keeps the text clear of both escaping rules.
	pat := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return '.'
	}, text)
	return fmt.Sprintf(`HasChecklist() && MatchHeadline("(?i)%s")`, pat)
}

func search(core *commands.Core, expr string) []common.Todo {
	todos, err := commands.SendReceiveGetErr[common.Todos](core, "search", map[string]string{"query": expr})
	if err != nil {
		commands.Fail("orgs list: %v", err)
	}
	return todos
}

// --- the writes ----------------------------------------------------------------

type toggle struct {
	Hash  string
	Index int
	Text  string
	Done  bool
}

type edit struct {
	Hash  string
	Op    string
	Index int
	Text  string
	New   string
	At    int
	Done  bool
}

// setDone ticks or clears one box (POST /checklist).
func setDone(core *commands.Core, l List, it Item, done bool) error {
	return post(core, "checklist", &toggle{Hash: l.Hash, Index: it.Index, Text: it.Text, Done: done})
}

// add writes a new box at position at (from 1), or at the end for 0.
func add(core *commands.Core, l List, text string, at int) error {
	return postEdit(core, &edit{Hash: l.Hash, Op: "add", New: text, At: at})
}

func reword(core *commands.Core, l List, it Item, text string) error {
	return postEdit(core, &edit{Hash: l.Hash, Op: "set", Index: it.Index, Text: it.Text, New: text})
}

func remove(core *commands.Core, l List, it Item) error {
	return postEdit(core, &edit{Hash: l.Hash, Op: "delete", Index: it.Index, Text: it.Text})
}

func postEdit(core *commands.Core, e *edit) error { return post(core, "checklist/edit", e) }

func post[T any](core *commands.Core, path string, body *T) error {
	reply, err := commands.SendReceivePostErr[T, common.ResultMsg](core, path, body)
	if err == commands.ErrDryRun {
		return nil
	}
	if err != nil {
		return err
	}
	if !reply.Ok {
		return fmt.Errorf("%s", reply.Msg)
	}
	return nil
}
