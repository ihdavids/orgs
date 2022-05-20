package orgs

import (
	"regexp"

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
