package orgs

import (
	"regexp"

	"github.com/Knetic/govaluate"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/niklasfasching/go-org/org"
)

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

func ParseString(expString *common.StringQuery) (*govaluate.EvaluableExpression, error) {
	functions := map[string]govaluate.ExpressionFunction{
		"IsProject": func(args ...interface{}) (interface{}, error) {
			p := args[0].(*org.Section)
			return IsProject(p), nil
		},
		"HasTags": func(args ...interface{}) (interface{}, error) {
			p := args[0].(*org.Section)
			ok := true
			for _, tagi := range args[1:] {
				tag := tagi.(string)
				if ok = ok && StringInSlice(tag, p.Headline.Tags); !ok {
					break
				}
			}
			return ok, nil
		},
		"IsStatus": func(args ...interface{}) (interface{}, error) {
			p := args[0].(*org.Section)
			s := args[1].(string)
			return p.Headline.Status == s, nil
		},

		"IsTodo": func(args ...interface{}) (interface{}, error) {
			p := args[0].(*org.Section)
			return IsTodoStatus(p), nil
		},

		"IsPriority": func(args ...interface{}) (interface{}, error) {
			p := args[0].(*org.Section)
			s := args[1].(string)
			return p.Headline.Priority == s, nil
		},
		"MatchHeadline": func(args ...interface{}) (interface{}, error) {
			p := args[0].(*org.Section)
			s := args[1].(string)
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
	}
	//expString := "strlen('someReallyLongInputString') <= 16"
	return govaluate.NewEvaluableExpressionWithFunctions(expString.Query, functions)
}

func EvalString(expression *govaluate.EvaluableExpression, v *org.Section) bool {
	parameters := make(map[string]interface{}, 8)
	parameters["v"] = v
	result, _ := expression.Evaluate(parameters)
	return result.(bool)
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
			res := EvalString(exp, v)
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
