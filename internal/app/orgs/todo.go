package orgs

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Knetic/govaluate"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func HasTag(name string, p *org.Section, d *org.Document) bool {
	ftagstr := d.Get("FILETAGS")
	ftags := strings.Split(ftagstr, ":")
	nname := strings.ToLower(name)
	for _, t := range ftags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && (t == nname) {
			return true
		}
	}

	if p != nil && p.Headline != nil {
		for _, t := range p.Headline.Tags {
			t = strings.ToLower(strings.TrimSpace(t))
			if t != "" && (t == nname) {
				return true
			}
		}
	}
	return false
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
			if t.After(p.Headline.Closed.Date.Start) {
				return false
			}
		}

		if p.Headline.HasScheduled() && p.Headline.Scheduled.Date.Before(t) {
			return true
		}

		if p.Headline.HasTimestamp() {
			fmt.Printf("*** Have timestamp %v vs %v", t, p.Headline.Timestamp.Time)
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

func IsTodoStatus(n *org.Section) bool {
	if n != nil && n.Headline != nil {
		return n.Headline.Status == "TODO"
	}
	return false
}

func IsProjectByChildren(p *org.Section) bool {
	if p != nil && p.Headline != nil {
		var childHasTodo bool = false
		for _, c := range p.Children {
			childHasTodo = childHasTodo || IsTodoStatus(c)
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

func IsProject(p *org.Section) bool {
	if Conf().UseTagForProjects {
		return IsProjectByTag(p)
	} else {
		return IsProjectByChildren(p)
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

func Eval(self *common.Query, v *org.Section) bool {
	switch self.NodeType {
	case common.And:
		ok := true
		for _, x := range self.Children {
			ok = ok && Eval(&x, v)
		}
		return ok
	case common.Or:
		ok := false
		for _, x := range self.Children {
			ok = Eval(&x, v)
			if ok {
				return true
			}
		}
		return false
	case common.Not:
		if len(self.Children) > 0 {
			return !(Eval(&self.Children[0], v))
		}
		return true
	case common.Tag:
		return self.Value.String != "" && StringInSlice(self.Value.String, v.Headline.Tags)
	case common.Status:
		return self.Value.String != "" && self.Value.String == v.Headline.Status
	case common.Priority:
		return self.Value.String != "" && self.Value.String == v.Headline.Priority

	case common.HeadlineRe:
		if self.Value.String != "" {
			var title string
			for _, n := range v.Headline.Title {
				title += n.String()
			}
			if ok, err := regexp.MatchString(self.Value.String, title); err == nil && ok {
				return true
			} else {
				return false
			}
		}
		return true
	case common.IsProject:
		return IsProject(v) == self.Value.Bool
	}
	return true
}

type Expr struct {
	Expression *govaluate.EvaluableExpression
	Sec        *org.Section
	Doc        *org.Document
}

func ParseString(expString *common.StringQuery) (*Expr, error) {
	var exp *Expr = new(Expr)
	exp.Sec = nil
	exp.Doc = nil
	functions := map[string]govaluate.ExpressionFunction{
		"IsProject": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return IsProject(p), nil
		},
		"HasTags": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			ok := true
			for _, tagi := range args[1:] {
				tag := tagi.(string)
				if ok = ok && HasTag(tag, p, exp.Doc); !ok {
					break
				}
			}
			return ok, nil
		},
		"NoTags": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return len(p.Headline.Tags) <= 0, nil
		},
		"IsStatus": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			s := args[0].(string)
			return p.Headline.Status == s, nil
		},

		"IsTodo": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return IsTodoStatus(p), nil
		},
		"IsArchived": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return IsArchived(p, exp.Doc), nil
		},

		"IsPriority": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			s := args[0].(string)
			return p.Headline.Priority == s, nil
		},
		"MatchHeadline": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			s := args[0].(string)
			var title string
			for _, n := range p.Headline.Title {
				title += n.String()
			}
			if ok, err := regexp.MatchString(s, title); err == nil && ok {
				return true, nil
			} else {
				return false, nil
			}
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

func EvalString(exp *Expr, v *org.Section, d *org.Document) bool {
	parameters := make(map[string]interface{}, 8)
	parameters["section"] = v
	// This is the implicit this pointer of our expressions
	exp.Sec = v
	exp.Doc = d
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

func QueryStringTodos(query *common.StringQuery) (common.Todos, error) {
	var todos common.Todos
	files := GetDb().GetFiles()
	exp, err := ParseString(query)
	if err != nil {
		return todos, err
	}
	for _, file := range files {
		f := GetDb().GetFile(file)
		for _, v := range f.doc.Outline.Children {
			GetDb().RegisterSection(v.Hash, v)
			res := EvalString(exp, v, f.doc)
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
				var t common.Todo = common.Todo{Headline: title, Tags: v.Headline.Tags, Hash: v.Hash, Date: date}
				todos = append(todos, t)
			}
		}
	}
	return todos, nil
}

// TODO OLD API DEPRECATED
func QueryTodos(query *common.Query) common.Todos {
	var todos common.Todos
	files := GetDb().GetFiles()
	for _, file := range files {
		f := GetDb().GetFile(file)
		for _, v := range f.doc.Outline.Children {
			res := Eval(query, v)
			if res {
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

func QueryProjects() common.Todos {
	var todos common.Todos
	files := GetDb().GetFiles()
	for _, file := range files {
		f := GetDb().GetFile(file)
		for _, v := range f.doc.Outline.Children {
			if IsProject(v) {
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
