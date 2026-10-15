package orgs

// What a :var is once it has been looked up, and the shaping org does to it
// between the lookup and the program: indexing (data[1:3,2]), column and row
// names (:colnames, :rownames) and rules (:hlines). The binders in babel.go
// write a babelVal in the program's own syntax.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// babelVal is a variable's value: a scalar as written, a list, or a table
// (a nil row is a rule, org's hline).
type babelVal struct {
	// The variable as the block wrote it; a scalar is rendered from it.
	v common.CodeVar
	// A string the program should see as text rather than as code: a looked
	// up text result, a list item, a table cell picked by an index. A scalar
	// written in the header (:var n=2, :var s="x") is not one: it is code.
	text   bool
	isList bool
	list   []string
	isTbl  bool
	rows   [][]string
}

func scalarVal(v common.CodeVar) babelVal { return babelVal{v: v} }

func textVal(name, s string) babelVal {
	return babelVal{v: common.CodeVar{Name: name, Value: s}, text: true}
}

func tableVal(name string, rows [][]string) babelVal {
	return babelVal{v: common.CodeVar{Name: name}, isTbl: true, rows: rows}
}

func listVal(name string, items []string) babelVal {
	return babelVal{v: common.CodeVar{Name: name}, isList: true, list: items}
}

// scalarText is a scalar's value with the quotes of a string literal taken
// off, for the languages that take everything as text (shell, awk, sqlite).
func (b babelVal) scalarText() string {
	val := strings.TrimSpace(b.v.Value)
	if b.text {
		return b.v.Value
	}
	if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' || val[0] == '\'' && val[len(val)-1] == '\'') {
		return val[1 : len(val)-1]
	}
	return val
}

// isNumber says a scalar is a number as far as a program is concerned.
func (b babelVal) isNumber() bool {
	_, err := strconv.ParseFloat(strings.TrimSpace(b.scalarText()), 64)
	return err == nil && !(b.text == false && strings.HasPrefix(strings.TrimSpace(b.v.Value), `"`))
}

// ----------------------------------------------------------------------------
// Indexing: org-babel-ref-index-list
// ----------------------------------------------------------------------------

// One level of a value while it is being indexed: a leaf (a cell, a list
// item, a scalar), a rule, or a list of nodes.
type idxNode struct {
	leaf  bool
	hline bool
	s     string
	kids  []idxNode
}

func nodeOf(b babelVal) (idxNode, error) {
	switch {
	case b.isTbl:
		n := idxNode{}
		for _, r := range b.rows {
			if r == nil {
				n.kids = append(n.kids, idxNode{hline: true})
				continue
			}
			row := idxNode{}
			for _, c := range r {
				row.kids = append(row.kids, idxNode{leaf: true, s: c})
			}
			n.kids = append(n.kids, row)
		}
		return n, nil
	case b.isList:
		n := idxNode{}
		for _, c := range b.list {
			n.kids = append(n.kids, idxNode{leaf: true, s: c})
		}
		return n, nil
	}
	return idxNode{}, fmt.Errorf("%s is not a table or a list, so it cannot be indexed", b.v.Name)
}

func valOf(name string, n idxNode) babelVal {
	switch {
	case n.leaf:
		return textVal(name, n.s)
	case n.hline:
		return textVal(name, "")
	}
	// A list of lists (or of rules) is a table; a list of leaves is a list.
	table := len(n.kids) > 0
	for _, k := range n.kids {
		if k.leaf {
			table = false
		}
	}
	if table {
		rows := [][]string{}
		for _, k := range n.kids {
			if k.hline {
				rows = append(rows, nil)
				continue
			}
			row := []string{}
			for _, c := range k.kids {
				row = append(row, c.s)
			}
			rows = append(rows, row)
		}
		return tableVal(name, rows)
	}
	items := []string{}
	for _, k := range n.kids {
		items = append(items, k.s)
	}
	return listVal(name, items)
}

var idxRangeRe = regexp.MustCompile(`^(-?\d+):(-?\d+)$`)

// pick is one level's index: "" or "*" every element, "n" the nth (negative
// from the end), "a:b" a to b inclusive.
func pick(n int, spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	wrap := func(i int) int {
		if i < 0 {
			return n + i
		}
		return i
	}
	check := func(i int) error {
		if i < 0 || i >= n {
			return fmt.Errorf("index %s is out of range (there are %d)", spec, n)
		}
		return nil
	}
	if spec == "" || spec == "*" {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out, nil
	}
	if m := idxRangeRe.FindStringSubmatch(spec); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		a, b = wrap(a), wrap(b)
		if err := check(a); err != nil {
			return nil, err
		}
		if err := check(b); err != nil {
			return nil, err
		}
		out := []int{}
		if a <= b {
			for i := a; i <= b; i++ {
				out = append(out, i)
			}
		} else {
			for i := a; i >= b; i-- {
				out = append(out, i)
			}
		}
		return out, nil
	}
	i, err := strconv.Atoi(spec)
	if err != nil {
		return nil, fmt.Errorf("%q is not an index (n, a:b or *)", spec)
	}
	i = wrap(i)
	if err := check(i); err != nil {
		return nil, err
	}
	return []int{i}, nil
}

// index applies the specs level by level. As in org, a level that comes out
// one element long is that element (so data[1,2] is a cell and data[,0] a
// column), and a rule passes through any level below it untouched.
func index(n idxNode, specs []string) (idxNode, error) {
	if len(specs) == 0 || n.leaf || n.hline {
		return n, nil
	}
	ids, err := pick(len(n.kids), specs[0])
	if err != nil {
		return n, err
	}
	out := idxNode{}
	for _, i := range ids {
		k, err := index(n.kids[i], specs[1:])
		if err != nil {
			return n, err
		}
		out.kids = append(out.kids, k)
	}
	if len(out.kids) == 1 {
		return out.kids[0], nil
	}
	return out, nil
}

// applyIndex is data[1:3,2] applied to a value.
func applyIndex(b babelVal, idx string) (babelVal, error) {
	idx = strings.TrimSpace(idx)
	if idx == "" {
		return b, nil
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(idx, "["), "]")
	n, err := nodeOf(b)
	if err != nil {
		return b, err
	}
	out, err := index(n, strings.Split(inner, ","))
	if err != nil {
		return b, fmt.Errorf("%s%s: %v", b.v.Name, idx, err)
	}
	r := valOf(b.v.Name, out)
	r.v.Name = b.v.Name
	return r, nil
}

// ----------------------------------------------------------------------------
// :colnames, :rownames, :hlines
// ----------------------------------------------------------------------------

// tableShape is what a block says about the tables it is handed, and what was
// taken off them to put back on its result.
type tableShape struct {
	colnames string // "", "yes", "no", or a list: (a b c)
	rownames string // "", "yes", "no"
	hlines   string // "", "yes", "no"
	// From the first table variable, for reassembling the result.
	header   []string
	rowNames []string
	taken    bool
}

func (j *babelJob) shape() *tableShape {
	s := &tableShape{}
	s.colnames, _ = j.arg("colnames")
	s.rownames, _ = j.arg("rownames")
	s.hlines, _ = j.arg("hlines")
	s.colnames = strings.ToLower(strings.TrimSpace(s.colnames))
	s.rownames = strings.ToLower(strings.TrimSpace(s.rownames))
	s.hlines = strings.ToLower(strings.TrimSpace(s.hlines))
	return s
}

func explicitNames(s string) ([]string, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "'")
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return nil, false
	}
	return shellWords(s[1 : len(s)-1]), true
}

// prepare shapes one table variable the way org-babel-disassemble-tables
// does: the header off (a first row with a rule under it, or any first row
// with :colnames yes), the first column off with :rownames yes, and the rules
// out unless :hlines yes. What came off the first table is kept to put back.
func (s *tableShape) prepare(b babelVal) babelVal {
	out, _ := s.prepareWithHeader(b)
	return out
}

// prepareWithHeader is prepare, also handing back the header it took off (for
// a csv file that needs it: sqlite's .import names a new table's columns
// from the first row).
func (s *tableShape) prepareWithHeader(b babelVal) (babelVal, []string) {
	if !b.isTbl {
		return b, nil
	}
	rows := b.rows
	var header []string
	_, explicit := explicitNames(s.colnames)
	if !explicit && s.colnames != "no" && s.colnames != "nil" {
		first := -1
		for i, r := range rows {
			if r != nil {
				first = i
				break
			}
		}
		ruled := first >= 0 && first+1 < len(rows) && rows[first+1] == nil
		if first >= 0 && (s.colnames == "yes" || s.colnames == "t" || ruled) {
			header = rows[first]
			rest := rows[first+1:]
			for len(rest) > 0 && rest[0] == nil {
				rest = rest[1:]
			}
			rows = rest
		}
	}
	var names []string
	if s.rownames == "yes" || s.rownames == "t" {
		trimmed := [][]string{}
		for _, r := range rows {
			if r == nil {
				trimmed = append(trimmed, nil)
				continue
			}
			if len(r) > 0 {
				names = append(names, r[0])
				trimmed = append(trimmed, r[1:])
			} else {
				names = append(names, "")
				trimmed = append(trimmed, r)
			}
		}
		rows = trimmed
	}
	if s.hlines != "yes" && s.hlines != "t" {
		kept := [][]string{}
		for _, r := range rows {
			if r != nil {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	if !s.taken {
		s.taken = true
		s.header, s.rowNames = header, names
	}
	out := b
	out.rows = rows
	return out, header
}

// reassemble puts back on a table result what prepare took off its input:
// row names when the row count matches, column names and a rule when the
// width does (org-babel-reassemble-table); an explicit :colnames list always.
func (s *tableShape) reassemble(res *common.CodeResult) {
	if res.Kind != "table" {
		return
	}
	header := s.header
	if names, ok := explicitNames(s.colnames); ok {
		header = names
	}
	if header == nil && s.rowNames == nil {
		return
	}
	rows := babelRows(res.Result)
	data := [][]string{}
	for _, r := range rows {
		if r != nil {
			data = append(data, r)
		}
	}
	if len(data) == 0 {
		return
	}
	changed := false
	if s.rowNames != nil && len(data) == len(s.rowNames) {
		k := 0
		for i, r := range rows {
			if r == nil {
				continue
			}
			rows[i] = append([]string{s.rowNames[k]}, r...)
			k++
		}
		changed = true
	}
	width := 0
	for _, r := range rows {
		if r != nil {
			width = len(r)
			break
		}
	}
	// A result whose first row already has a rule under it (sqlite's or a
	// csv's own header) names its own columns.
	if header != nil && width == len(header) && !(len(rows) > 1 && rows[0] != nil && rows[1] == nil) {
		rows = append([][]string{header, nil}, rows...)
		changed = true
	}
	if changed {
		res.Result = orgTableText(rows)
	}
}

// ----------------------------------------------------------------------------
// Named things other than tables and blocks
// ----------------------------------------------------------------------------

// namedNode is the node a #+NAME: names in a file, unwrapped.
func namedNode(f *common.OrgFile, name string) (org.Node, bool) {
	if f == nil || f.Doc == nil {
		return nil, false
	}
	node, ok := f.Doc.NamedNodes[name]
	for ok {
		switch t := node.(type) {
		case org.NodeWithName:
			node = t.Node
		case *org.NodeWithName:
			node = t.Node
		case org.NodeWithMeta:
			node = t.Node
		case *org.NodeWithMeta:
			node = t.Node
		default:
			return node, true
		}
	}
	return nil, false
}

// listItems is a named list as org passes it: each item's text.
func listItems(node org.Node) []string {
	var l *org.List
	switch t := node.(type) {
	case org.List:
		l = &t
	case *org.List:
		l = t
	}
	out := []string{}
	if l == nil {
		return out
	}
	for _, it := range l.Items {
		var kids []org.Node
		switch t := it.(type) {
		case org.ListItem:
			kids = t.Children
		case *org.ListItem:
			kids = t.Children
		case org.DescriptiveListItem:
			kids = t.Details
		}
		parts := []string{}
		for _, k := range kids {
			if p, ok := k.(org.Paragraph); ok {
				parts = append(parts, strings.TrimSpace(org.String(p.Children...)))
			}
		}
		out = append(out, strings.Join(parts, " "))
	}
	return out
}

// exampleText is a named example block's text.
func exampleText(node org.Node) string {
	switch t := node.(type) {
	case org.Block:
		return strings.TrimRight(org.String(t.Children...), "\n")
	case *org.Block:
		return strings.TrimRight(org.String(t.Children...), "\n")
	case org.Example:
		return strings.TrimRight(org.String(t.Children...), "\n")
	}
	return strings.TrimRight(node.String(), "\n")
}

var crossFileVarRe = regexp.MustCompile(`^(.+\.org):([^:]+)$`)

// crossFile reads `other.org:name`, org's way of naming something in another
// file (relative to this one): the file, absolute, and the name.
func crossFile(target, from string) (string, string, bool) {
	m := crossFileVarRe.FindStringSubmatch(target)
	if m == nil {
		return "", "", false
	}
	p := m[1]
	if !filepath.IsAbs(p) && from != "" {
		p = filepath.Join(filepath.Dir(from), p)
	}
	return filepath.Clean(p), m[2], true
}
