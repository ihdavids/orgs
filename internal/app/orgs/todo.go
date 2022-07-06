package orgs

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Knetic/govaluate"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func HasFileTag(name string, d *org.Document) bool {
	ftagstr := d.Get("FILETAGS")
	ftags := strings.Split(ftagstr, ":")
	nname := strings.ToLower(name)
	for _, t := range ftags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && (t == nname) {
			return true
		}
	}
	return false
}

func AddFileTag(name string, d *org.Document) bool {
	if !HasFileTag(name, d) {
		v, have := d.BufferSettings["FILETAGS"]
		if have {
			for i, n := range d.Nodes {
				switch kw := n.(type) {
				case org.Keyword:
					if kw.Key == "FILETAGS" {
						kw.Value += ":" + name + ":"
						d.Nodes[i] = kw
						break
					}
				}
			}
		} else {
			kw := org.Keyword{Key: "FILETAGS", Value: ":" + name + ":"}
			d.Nodes = append([]org.Node{kw}, d.Nodes...)
		}
		d.BufferSettings["FILETAGS"] = v + ":" + name + ":"
		return true
	}
	return false
}

func HeadlineAloneHasTag(name string, p *org.Section) bool {
	if p != nil && p.Headline != nil {
		for _, t := range p.Headline.Tags {
			t = strings.ToLower(strings.TrimSpace(t))
			if t != "" && (t == name) {
				return true
			}
		}
	}
	return false
}
func NodeHasTagRecursive(name string, p *org.Section) bool {
	if HeadlineAloneHasTag(name, p) {
		return true
	}
	if p.Parent != nil {
		return NodeHasTagRecursive(name, p.Parent)
	}
	return false

}

func NodeHasNoTagRecursive(p *org.Section) bool {
	if p.Headline != nil && p.Headline.Tags != nil && len(p.Headline.Tags) > 0 {
		return false
	}
	if p.Parent != nil {
		return NodeHasNoTagRecursive(p.Parent)
	}
	return true

}
func NoTags(p *org.Section, d *org.Document) bool {
	if strings.TrimSpace(d.Get("FILETAGS")) != "" {
		return false
	}

	return NodeHasNoTagRecursive(p)
}

func HasTag(name string, p *org.Section, d *org.Document) bool {
	if HasFileTag(name, d) {
		return true
	}

	// TODO: Can we cache this?
	nname := strings.ToLower(name)
	return NodeHasTagRecursive(nname, p)
}

func GetBeginOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func GetEndOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location()).AddDate(0, 0, 1)
}

func Today() time.Time {
	return GetBeginOfDay(time.Now())
}

func EndOfToday() time.Time {
	return GetEndOfDay(time.Now())
}

func Yesterday() time.Time {
	return GetBeginOfDay(time.Now().AddDate(0, 0, -1))
}

func TheDayBefore(from time.Time) time.Time {
	return from.AddDate(0, 0, -1)
}

func AWeekAgo() time.Time {
	return GetBeginOfDay(time.Now().AddDate(0, 0, -7))
}

func AWeekAgoFrom(from time.Time) time.Time {
	return from.AddDate(0, 0, -7)
}

func IsOn(p *org.Section, t time.Time) bool {
	if p != nil && p.Headline != nil {
		// If we are closed we do not show up after the close date
		if p.Headline.HasClosed() {
			fmt.Printf("*** HAVE CLOSED %v vs %v %s", t, p.Headline.Timestamp.Time, p.Headline.Title[0])
			if t.After(p.Headline.Closed.Date.Start) {
				return false
			}
		}

		if p.Headline.HasScheduled() && p.Headline.Scheduled.Date.Before(t) {
			return true
		}

		if p.Headline.HasTimestamp() && p.Headline.Timestamp.Time.OnDay(t) {
			return true
		}

		// TODO: Handle deadlines in here properly.
	}
	return false
}

func IsIn(p *org.Section, start time.Time, end time.Time) bool {
	if p != nil && p.Headline != nil {
		// If we are closed we do not show up after the close date
		if p.Headline.HasClosed() && end.After(p.Headline.Closed.Date.Start) {
			end = p.Headline.Closed.Date.Start
		}
		// Handle end before we start case
		if end.Before(start) {
			return false
		}
		// If we closed before we started
		if p.Headline.HasClosed() && start.After(p.Headline.Closed.Date.Start) {
			return false
		}

		if p.Headline.HasScheduled() && p.Headline.Scheduled.Date.Start.Before(end) {
			return true
		}

		if p.Headline.HasTimestamp() && p.Headline.Timestamp.Time.After(start) && p.Headline.Timestamp.Time.Before(end) {
			return true
		}

		// TODO: Handle deadlines in here properly.
	}
	return false
}

func IsTodoStatus(n *org.Section, f *OrgFile) bool {
	if n != nil && n.Headline != nil {
		return IsActive(n, f)
	}
	return false
}
func HeadingMatchesRe(p *org.Section, headingRe string) bool {
	var title string
	for _, n := range p.Headline.Title {
		title += n.String()
	}
	if ok, err := regexp.MatchString(headingRe, title); err == nil && ok {
		return true
	}
	return false
}

func IsPartOfProject(p *org.Section, projectRe string, f *OrgFile) bool {
	if p != nil && p.Headline != nil && p.Parent != nil {
		if !IsProject(p.Parent, f) {
			return false
		}
		return HeadingMatchesRe(p.Parent, projectRe)
	}
	return false
}

func IsProjectByChildren(p *org.Section, f *OrgFile) bool {
	if p != nil && p.Headline != nil {
		var childHasTodo bool = false
		for _, c := range p.Children {
			childHasTodo = childHasTodo || IsTodoStatus(c, f)
		}
		return childHasTodo
	}
	return false
}

func IsProjectByTag(p *org.Section) bool {
	if p != nil && p.Headline != nil {
		for _, t := range p.Headline.Tags {
			if t == "PROJECT" {
				return true
			}
		}
	}
	return false
}

func IsArchived(p *org.Section, d *org.Document) bool {
	return HasTag("archive", p, d)
}

func IsProject(p *org.Section, f *OrgFile) bool {
	if Conf().UseTagForProjects {
		return IsProjectByTag(p)
	} else {
		return IsProjectByChildren(p, f)
	}
}

func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func AllStringsInSlice(alist []string, list []string) bool {
	for _, a := range alist {
		if !StringInSlice(a, list) {
			return false
		}
	}
	return true
}

type Expr struct {
	Expression *govaluate.EvaluableExpression
	Sec        *org.Section
	Doc        *org.Document
	File       *OrgFile
}

func ParseString(expString *common.StringQuery) (*Expr, error) {
	var exp *Expr = new(Expr)
	exp.Sec = nil
	exp.Doc = nil
	exp.File = nil
	functions := map[string]govaluate.ExpressionFunction{
		"IsProject": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return IsProject(p, exp.File), nil
		},
		"IsActive": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return IsActive(p, exp.File), nil
		},
		"HasAStatus": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return strings.TrimSpace(p.Headline.Status) != "", nil
		},
		"IsPartOfProject": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return IsPartOfProject(p, args[0].(string), exp.File), nil
		},
		"HasTags": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			ok := true
			for _, tagi := range args {
				tag := tagi.(string)
				if ok = ok && HasTag(tag, p, exp.Doc); !ok {
					break
				}
			}
			return (ok && len(p.Headline.Tags) > 0), nil
		},
		"NoTags": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return NoTags(p, exp.Doc), nil
		},
		"IsStatus": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			s := args[0].(string)
			return p.Headline.Status == s, nil
		},
		// Checks if this headline status is present and in the active state
		"IsTodo": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return IsTodoStatus(p, exp.File), nil
		},
		// Syntatical sugar for the following:
		// !IsArchived() && IsTodo() && !IsProject()
		"IsTask": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			return (!IsArchived(p, exp.Doc) && !IsProject(p, exp.File) && IsTodoStatus(p, exp.File)), nil
		},
		// Check if a headline is in the archived state or not
		"IsArchived": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return IsArchived(p, exp.Doc), nil
		},
		// Check if the priority matches a specific value.
		"IsPriority": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			s := args[0].(string)
			return p.Headline.Priority == s, nil
		},
		// Returns true if the headline has the specific property
		"HasProperty": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			s := args[0].(string)
			if _, ok := p.Headline.Properties.Get(s); ok {
				return true, nil
			}
			return false, nil
		},
		// MatchProperty(NAME, REGEX)
		// returns true if the property value matches the implied regex
		"MatchProperty": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			name := args[0].(string)
			test := args[1].(string)
			if val, ok := p.Headline.Properties.Get(name); ok {
				if ok, err := regexp.MatchString(test, val); err == nil && ok {
					return true, nil
				}
			}
			return false, nil
		},
		// Run an RE against each headline and check for a match
		"MatchHeadline": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			s := args[0].(string)
			return HeadingMatchesRe(p, s), nil
		},

		// -----------------------------------------------
		// DATE TIME QUERIES
		// -----------------------------------------------

		// Check if a todo is targetting a specific date
		"OnDate": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			tm := args[0].(string)
			//p := args[0].(*org.Section)
			var now time.Time
			var err error
			if now, err = time.Parse("2006 02 01", tm); err != nil {
				return false, err
			}

			return IsOn(p, now), nil
		},
		"Today": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			now := Today()
			return IsOn(p, now), nil
		},
		"Yesterday": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			now := Yesterday()
			return IsOn(p, now), nil
		},
		"ThisWeek": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			start := AWeekAgo()
			now := EndOfToday()
			return IsIn(p, start, now), nil
		},
	}
	//expString := "strlen('someReallyLongInputString') <= 16"
	var err error
	exp.Expression, err = govaluate.NewEvaluableExpressionWithFunctions(expString.Query, functions)
	return exp, err
}

func EvalString(exp *Expr, v *org.Section, f *OrgFile) bool {
	parameters := make(map[string]interface{}, 8)
	parameters["section"] = v
	// This is the implicit this pointer of our expressions
	exp.Sec = v
	exp.Doc = f.doc
	exp.File = f
	result, _ := exp.Expression.Evaluate(parameters)
	return result.(bool)
}

func QueryFullTodo(query *common.TodoHash) (common.FullTodo, error) {
	var td common.FullTodo
	if s, ok := GetDb().ByHash[(string)(*query)]; ok {
		var title string
		for _, n := range s.Headline.Title {
			title += n.String()
		}
		td.Headline = title
		td.Hash = s.Hash
		td.Priority = s.Headline.Priority
		td.Tags = s.Headline.Tags
		var contentNodes []org.Node = s.Headline.Children
		for i, n := range s.Headline.Children {
			switch n.(type) {
			case org.Headline:
				contentNodes = s.Headline.Children[0:i]
				break
			}
		}
		w := org.NewOrgWriter()
		org.WriteNodes(w, contentNodes...)
		td.Content = w.String()
		return td, nil
	}
	return td, fmt.Errorf("failed to find todo by hash")
}

func QueryFullTodoHtml(query *common.TodoHash) (common.FullTodo, error) {
	var td common.FullTodo
	if s, ok := GetDb().ByHash[(string)(*query)]; ok {
		var title string
		for _, n := range s.Headline.Title {
			title += n.String()
		}
		td.Headline = title
		td.Hash = s.Hash
		td.Priority = s.Headline.Priority
		td.Tags = s.Headline.Tags
		var contentNodes []org.Node = s.Headline.Children
		for i, n := range s.Headline.Children {
			switch n.(type) {
			case org.Headline:
				contentNodes = s.Headline.Children[0:i]
				break
			}
		}
		w := org.NewHTMLWriter()
		org.WriteNodes(w, contentNodes...)
		td.Content = w.String()
		return td, nil
	}
	return td, fmt.Errorf("failed to find todo by hash")
}

func ProcessNode(exp *Expr, v *org.Section, f *OrgFile, todos common.Todos) (common.Todos, error) {
	GetDb().RegisterSection(v.Hash, v, f)
	res := EvalString(exp, v, f)
	if res {
		var title string
		for _, n := range v.Headline.Title {
			title += n.String()
		}
		var date org.OrgDate
		if v.Headline.Scheduled != nil {
			date = *v.Headline.Scheduled.Date
		}
		if v.Headline.Timestamp != nil {
			date = *v.Headline.Timestamp.Time
		}
		var t common.Todo = common.Todo{Headline: title, Tags: v.Headline.Tags, Hash: v.Hash, Date: date, Status: v.Headline.Status, Filename: f.filename, LineNum: v.Headline.Pos.Row, IsActive: IsActive(v, f)}
		todos = append(todos, t)
	}
	for _, c := range v.Children {
		todos, _ = ProcessNode(exp, c, f, todos)
	}
	return todos, nil
}

func EvalForNodes(exp *Expr, v *org.Section, f *OrgFile, nodes []*org.Section) ([]*org.Section, error) {
	GetDb().RegisterSection(v.Hash, v, f)
	res := EvalString(exp, v, f)
	if res {
		nodes = append(nodes, v)
	}
	for _, c := range v.Children {
		nodes, _ = EvalForNodes(exp, c, f, nodes)
	}
	return nodes, nil
}

func QueryStringNodesOnFile(query string, file *OrgFile) ([]*org.Section, error) {
	var nodes []*org.Section
	exp, err := ParseString(&common.StringQuery{Query: query})
	if err != nil {
		return nodes, err
	}
	for _, v := range file.doc.Outline.Children {
		nodes, _ = EvalForNodes(exp, v, file, nodes)
	}
	return nodes, nil
}

func QueryStringTodos(query *common.StringQuery) (common.Todos, error) {
	var todos common.Todos
	files := GetDb().GetFiles()
	fmt.Printf("QUERY WAS: %s\n", query.Query)
	exp, err := ParseString(query)
	if err != nil {
		return todos, err
	}
	for _, file := range files {
		f := GetDb().GetFile(file)
		for _, v := range f.doc.Outline.Children {
			todos, _ = ProcessNode(exp, v, f, todos)
		}
	}
	return todos, nil
}

func QueryProjects() common.Todos {
	var todos common.Todos
	files := GetDb().GetFiles()
	for _, file := range files {
		f := GetDb().GetFile(file)
		for _, v := range f.doc.Outline.Children {
			if IsProject(v, f) {
				var title string
				for _, n := range v.Headline.Title {
					title += n.String()
				}
				var t common.Todo = common.Todo{Headline: title, Tags: v.Headline.Tags}
				todos = append(todos, t)
			}
		}
	}
	return todos
}

func WriteOutOrgFile(f *OrgFile) bool {
	// Need the doc to serialize and write it out.
	w := org.NewOrgWriter()
	//w.Indent = "  "
	f.doc.Write(w)
	err := ioutil.WriteFile(f.filename, []byte(w.String()), os.ModePerm)
	return err == nil
}

func SetThingChildren(n *org.Headline, s *org.Section, doit func(head *org.Headline) org.Headline) bool {
	for i, _ := range n.Children {
		switch nn := n.Children[i].(type) {
		case org.Headline:
			if nn.Index == s.Headline.Index {
				n.Children[i] = doit(&nn)
				return true
			}
			if SetThingChildren(&nn, s, doit) {
				return true
			}
		}
	}
	return false
}

// We have to set the status on the node in the chain (the core struct)
func SetThing(f *OrgFile, s *org.Section, doit func(head *org.Headline) org.Headline) bool {
	for i, _ := range f.doc.Nodes {
		switch n := f.doc.Nodes[i].(type) {
		case org.Headline:
			if n.Index == s.Headline.Index {
				f.doc.Nodes[i] = doit(&n)
				return true
			}
			if SetThingChildren(&n, s, doit) {
				return true
			}
		}
	}
	log.Printf("Did not find headline in update\n")
	return false
}

func ChangeStatus(query *common.TodoItemChange) (common.Result, error) {
	didWrite := true
	hh := common.TodoHash(query.Hash)
	if !IsStatusValid(&hh, query.Value) {
		return common.Result{false}, fmt.Errorf("Status value is not valid for this item!")
	}
	if s, ok := GetDb().ByHash[(string)(query.Hash)]; ok {
		// Change the status
		f := GetDb().ByHashToFile[(string)(query.Hash)]
		if set := SetThing(f, s, func(n *org.Headline) org.Headline {
			n.Status = query.Value
			return *n
		}); set {
			didWrite = WriteOutOrgFile(f)
		}
	}
	return common.Result{didWrite}, nil
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func findStr(s []string, str string) int {
	for i, v := range s {
		if v == str {
			return i
		}
	}
	return -1
}

func remove(slice []string, s string) []string {
	if i := findStr(slice, s); i >= 0 {
		return append(slice[:i], slice[i+1:]...)
	}
	return slice
}

func ToggleTag(query *common.TodoItemChange) (common.Result, error) {
	fmt.Printf("TOGGLE TAG CALLED: %s\n", query.Value)
	didWrite := true
	if s, ok := GetDb().ByHash[(string)(query.Hash)]; ok {
		// Change a tag
		f := GetDb().ByHashToFile[(string)(query.Hash)]
		if set := SetThing(f, s, func(n *org.Headline) org.Headline {
			if contains(n.Tags, query.Value) {
				n.Tags = remove(n.Tags, query.Value)
			} else {
				n.Tags = append(n.Tags, query.Value)
			}
			return *n
		}); set {
			didWrite = WriteOutOrgFile(f)
		}
	}
	return common.Result{didWrite}, nil
}

func ParseTodoStates(ftagstr string) ([]string, []string) {

	var active []string
	var done []string

	ss := strings.Split(ftagstr, "|")
	if len(ss) >= 1 {
		sss := strings.Fields(ss[0])
		for _, x := range sss {
			x = strings.TrimSpace(x)
			if x != "" {
				if !contains(active, x) {
					active = append(active, x)
				}
			}
		}
	}
	if len(ss) >= 2 {
		sss := strings.Fields(ss[1])
		for _, x := range sss {
			x = strings.TrimSpace(x)
			if x != "" {
				if !contains(done, x) {
					done = append(done, x)
				}
			}
		}
	}
	return active, done
}

func ValidStatusFromFile(f *OrgFile) ([]string, []string) {
	var active []string
	var done []string
	if f != nil {
		// #+TODO: REPORT BUG KNOWNCAUSE | FIXED
		ftagstr := f.doc.Get("TODO")
		if ftagstr != "" {
			active, done = ParseTodoStates(ftagstr)
		} else {
			active, done = ParseTodoStates(Conf().DefaultTodoStates)
		}
	} else {
		active, done = ParseTodoStates(Conf().DefaultTodoStates)
	}
	return active, done
}

func ValidStatus(query *common.TodoHash) (common.TodoStatesResult, error) {
	var active []string
	var done []string
	if _, ok := GetDb().ByHash[(string)(*query)]; ok {
		f := GetDb().ByHashToFile[(string)(*query)]
		if f != nil {
			active, done = ValidStatusFromFile(f)
		}
	} else {
		active, done = ParseTodoStates(Conf().DefaultTodoStates)
	}
	states := common.TodoStatesResult{Active: active, Done: done}
	return states, nil
}

func IsActive(v *org.Section, f *OrgFile) bool {
	status := v.Headline.Status
	active, _ := ValidStatusFromFile(f)
	return contains(active, status)
}

func IsStatusValid(query *common.TodoHash, status string) bool {
	r, _ := ValidStatus(query)
	if contains(r.Active, status) || contains(r.Done, status) {
		return true
	}
	return false
}
