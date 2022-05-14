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

func QueryTodos(query *common.Query) common.Todos {
	var todos common.Todos
	files := GetDb().GetFiles()
	for _, file := range files {
		f := GetDb().GetFile(file)
		for _, v := range f.doc.Outline.Children {
			project := IsProject(v)
			var hre *regexp.Regexp
			if query.HeadlineRe != "" {
				var err error
				hre, err = regexp.Compile(query.HeadlineRe)
				if err != nil {
					return nil
				}
			}
			if query.IsProject == project {
				ok := true
				ok = ok && (len(query.Status) <= 0 || StringInSlice(v.Headline.Status, query.Status))
				ok = ok && (len(query.Tags) <= 0 || AllStringsInSlice(v.Headline.Tags, query.Tags))
				ok = ok && (len(query.Priorities) <= 0 || StringInSlice(v.Headline.Priority, query.Priorities))
				if ok {
					var title string
					for _, n := range v.Headline.Title {
						title += n.String()
					}
					ok = ok && (hre == nil || hre.MatchString(title))
					if ok {
						var t common.Todo = common.Todo{Headline: title, Tags: v.Headline.Tags}
						todos = append(todos, t)
					}
				}
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
