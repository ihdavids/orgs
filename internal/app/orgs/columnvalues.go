//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"net/http"
	"sort"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// ColumnValues gathers every property a file's headings use and the values
// each takes. Properties are read off the file's lines (propsOfSection), as
// the column view reads them, so a drawer in column zero counts; the keyword,
// tags and priority come in under the names the column view gives them (TODO,
// TAGS, PRIORITY) so they can be listed and offered like any property.
func ColumnValues(filename string) (common.ColumnValuesResult, error) {
	res := common.ColumnValuesResult{File: filename}
	f := GetDb().FindByFile(filename)
	if f == nil || f.Doc == nil {
		return res, errNoFile(filename)
	}
	res.Props = collectColumnValues(f.Doc.Outline.Children, fileLines(filename))
	res.Ok = true
	return res, nil
}

// The counting, apart from the database so it can be tested on a parsed
// document.
func collectColumnValues(top []*org.Section, lines []string) []common.ColumnPropValues {
	// name -> value -> count, and name -> headings that have it.
	counts := map[string]map[string]int{}
	headings := map[string]int{}
	add := func(name, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if counts[name] == nil {
			counts[name] = map[string]int{}
		}
		counts[name][value]++
	}
	var walk func(secs []*org.Section)
	walk = func(secs []*org.Section) {
		for _, s := range secs {
			if s == nil || s.Headline == nil {
				continue
			}
			seen := map[string]bool{}
			note := func(name string) {
				if !seen[name] {
					seen[name] = true
					headings[name]++
				}
			}
			for k, v := range propsOfSection(lines, s) {
				name := strings.ToUpper(k)
				if strings.TrimSpace(v) == "" {
					continue
				}
				add(name, v)
				note(name)
			}
			if s.Headline.Status != "" {
				add("TODO", s.Headline.Status)
				note("TODO")
			}
			if s.Headline.Priority != "" {
				add("PRIORITY", s.Headline.Priority)
				note("PRIORITY")
			}
			for _, t := range s.Headline.Tags {
				add("TAGS", t)
				note("TAGS")
			}
			walk(s.Children)
		}
	}
	walk(top)

	var out []common.ColumnPropValues
	for name, vals := range counts {
		p := common.ColumnPropValues{Name: name, Count: headings[name]}
		for v, n := range vals {
			p.Values = append(p.Values, common.ColumnValueCount{Value: v, Count: n})
		}
		// Most used first - what is offered first while typing - then by value.
		sort.Slice(p.Values, func(a, b int) bool {
			if p.Values[a].Count != p.Values[b].Count {
				return p.Values[a].Count > p.Values[b].Count
			}
			return p.Values[a].Value < p.Values[b].Value
		})
		out = append(out, p)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

type noFileError string

func (e noFileError) Error() string { return "no file called " + string(e) }
func errNoFile(name string) error   { return noFileError(name) }

/* SDOC: API
* GET /columns/values — Every Property in a File, With Its Values

	Every property the file's headings use, on how many headings, and the
	unique values each takes with how often - most used first. The keyword,
	tags and priority come in as =TODO=, =TAGS= and =PRIORITY=. Behind the
	column view's value listing, its type-ahead while a cell is edited, and its
	property-name suggestions while a columns line is written.

	*Method:* =GET=

	*Query:* =file= - the org file.

	*Response:* A =ColumnValuesResult=: =Ok=, =Msg=, =File=, and =Props=, a
	list of ={Name, Count, Values: [{Value, Count}]}=.
	EDOC */
func RequestColumnValues(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("file")
	if strings.TrimSpace(filename) == "" {
		columnJson(w, common.ColumnValuesResult{Ok: false, Msg: "no file given"})
		return
	}
	res, err := ColumnValues(filename)
	if err != nil {
		res.Ok = false
		res.Msg = err.Error()
	}
	columnJson(w, res)
}
