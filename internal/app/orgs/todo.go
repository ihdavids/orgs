//lint:file-ignore ST1006 allow the use of self
package orgs

/* SDOC: Querying
* Overview

  Many operations in orgs require you to select the nodes that the operation applies to.
  - Agendas
  - Filtered Tabular Lists
  - Various Exporters
  - Etc

  Lots of these things require a filtered list of nodes to operate. Orgs does this through
  a node filter. This is an expression that is applied to nodes in the DB and returns only those
  nodes that pass the query.

  The most common expression starts with:

   #+BEGIN_SRC cpp
   !IsArchive() && IsTodo()
   #+END_SRC

   This will select all active nodes that have an active TODO status on them throughout all of your org mode files.
   Note the negation on IsArchive() these expressions support most common operators

   Agenda views will often add a date query:

   #+BEGIN_SRC cpp
   !IsArchive() && IsTodo() && OnDate('<specific date>')
   #+END_SRC

   People who follow GTD will often want lists that follow the common patterns:

   #+BEGIN_SRC cpp
   !IsArchive() && IsProject()
   !IsArchive() && IsTodo() && IsStatus('NEXT')
   !IsArchive() && IsTodo() && ( IsStatus('WAITING') || IsStatus('BLOCKED') )
   #+END_SRC

   This represents some of your common lists that you need to review regularly:
   - Projects List
   - Next Actions List
   - Waiting On List


** Orgs Expression Methods Reference

  - *IsProject* - returns true for nodes that are defined as a project (see project definition)
  - *HasAStatus* - returns true if a node has a valid status
  - *IsPartOfProject* - returns true if a task is a subnode of a project node
  - *HasTags* - returns true if a node has any tags
  - *NoTags* - returns true if a node does not have any tags on it
  - *InTagGroup* - cheat, returns true if any tags in a tag group are applied to a node
  - *IsStatus* - returns true if a node has a given status
  - *IsTodo* - returns true if a node has an active status (the same as IsActive currently)
  - *IsActive* - returns true if the status of a node is an active status (IE not DONE)
  - *IsTask* - Syntatical sugar for the following: "!IsArchived() && IsTodo() && !IsProject()"
  - *IsNextTask* - Check if a headline has a NEXT action status. This is GTD support and uses the defaultNextStatus value and #+NEXT comment
  - *IsBlockedProject* - Check if this is a project heading and it DOES NOT have a child marked NEXT.
  - *IsArchived* - Check if a headline is in the archived state or not (in an archived file or has an ARCHIVE tag)
  - *IsPriority* - Check if the priority matches a specific value.
  - *HasProperty* - Returns true if the headline has the specific property
  - *HasTable* - Checks if the node contains a table.
  - *HasDrawer* - Checks if the node contains a drawer.
  - *HasBlock* - Checks if the node contains a block object.
  - *MatchProperty* - MatchProperty(NAME, REGEX) returns true if the property value matches the implied regex
  - *MatchHeadline* - Run an RE against each headline and check for a match
  - *OnDate* - Check if a todo is targetting a specific date
  - *Today* - returns true if a node is scheduled for today
  - *Yesterday* - returns true if a node is scheduled for yesterday
  - *ThisWeek* - returns true if a node is scheduled for sometime this week
EDOC */

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/govaluate"
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
func HasFileTagRegex(name string, d *org.Document) bool {
	ftagstr := d.Get("FILETAGS")
	ftags := strings.Split(ftagstr, ":")
	for _, t := range ftags {
		if ok, err := regexp.MatchString(name, t); err == nil && ok {
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
						kw.Value = strings.TrimSpace(kw.Value)
						if !strings.HasSuffix(kw.Value, ":") {
							kw.Value += ":"
						}
						kw.Value += name + ":"
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

func HeadlineAloneHasTagRegex(name string, p *org.Section) bool {
	if p != nil && p.Headline != nil {
		for _, t := range p.Headline.Tags {
			if ok, err := regexp.MatchString(name, t); err == nil && ok {
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

func NodeHasTagRecursiveRegex(name string, p *org.Section) bool {
	if HeadlineAloneHasTagRegex(name, p) {
		return true
	}
	if p.Parent != nil {
		return NodeHasTagRecursiveRegex(name, p.Parent)
	}
	return false

}

func getParentTags(p *org.Section, curTags []string) []string {
	if p != nil && p.Headline != nil && p.Headline.Tags != nil {
		curTags = append(curTags, p.Headline.Tags...)
	}
	if p.Parent != nil {
		curTags = getParentTags(p.Parent, curTags)
	}
	return curTags
}

func GetParentTags(p *org.Section, d *org.Document) []string {
	tgs := []string{}
	if p.Parent != nil {
		tgs = getParentTags(p.Parent, tgs)
	}
	ftagstr := strings.TrimSpace(d.Get("FILETAGS"))
	if ftagstr != "" {
		ftags := strings.Split(ftagstr, ":")
		tgs = append(tgs, ftags...)
	}
	return tgs
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

func HasTagRegex(name string, p *org.Section, d *org.Document) bool {
	if HasFileTagRegex(name, d) {
		return true
	}
	return NodeHasTagRecursiveRegex(name, p)
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

func IsTodoStatus(n *org.Section, f *common.OrgFile) bool {
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

func IsPartOfProject(p *org.Section, projectRe string, f *common.OrgFile) bool {
	if p != nil && p.Headline != nil && p.Parent != nil {
		if !IsProject(p.Parent, f) {
			return false
		}
		return HeadingMatchesRe(p.Parent, projectRe)
	}
	return false
}

// This is a GTD support method. This returns true if this is a project (as defined by the system)
// AND
// this project does not have a NEXT status task. This is part of ensuring projects are moving
// forward.
func IsBlockedProject(p *org.Section, projectRe string, f *common.OrgFile) bool {
	// We have a headline
	if p != nil && p.Headline != nil {
		// This is a project
		if IsProject(p, f) {
			// Do any of the children have a NEXT status
			var childHasNext bool = false
			for _, c := range p.Children {
				childHasNext = childHasNext || IsNextTask(c, f)
			}
			return childHasNext
		}
	}
	return false
}

func HasTable(p *org.Section, f *common.OrgFile) bool {
	// The body of this node has a table object in it.
	if p != nil && p.Headline != nil {
		return p.Headline.Tables != nil && len(p.Headline.Tables) > 0
	}
	return false
}

func HasBlock(p *org.Section, f *common.OrgFile) bool {
	// The body of this node has a block object in it.
	if p != nil && p.Headline != nil {
		return p.Headline.Blocks != nil && len(p.Headline.Blocks) > 0
	}
	return false
}

func HasDrawer(p *org.Section, f *common.OrgFile) bool {
	// The body of this node has a Drawer object in it.
	if p != nil && p.Headline != nil {
		return p.Headline.Drawers != nil && len(p.Headline.Drawers) > 0
	}
	return false
}

// Project is defined as a headline that has a headline child with
// a status entry
func IsProjectByChildren(p *org.Section, f *common.OrgFile) bool {
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
			if strings.ToLower(t) == "project" {
				return true
			}
		}
	}
	return false
}

func IsArchived(p *org.Section, d *org.Document) bool {
	return HasTag("archive", p, d) || HasTag("archived", p, d)
}

// Return true if this task has a status that is considered a NEXT actions
// status
func IsNextTask(p *org.Section, f *common.OrgFile) bool {
	if p != nil && p.Headline != nil {
		status := p.Headline.Status
		next, _ := NextStatusFromFile(f)
		return contains(next, status)
	}
	return false
}

func IsProject(p *org.Section, f *common.OrgFile) bool {
	if Conf().Server.UseTagForProjects {
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
	File       *common.OrgFile
	Tbl        *org.Table
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
		"IsNextTask": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			return IsNextTask(p, exp.File), nil
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
		"IsBlockedProject": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			//p := args[0].(*org.Section)
			return IsBlockedProject(p, args[0].(string), exp.File), nil
		},
		"HasBlock": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			return HasBlock(p, exp.File), nil
		},
		"HasDrawer": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			return HasDrawer(p, exp.File), nil
		},
		"HasTable": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			return HasTable(p, exp.File), nil
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
			return ok, nil
		},
		"InTagGroup": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			s := args[0].(string)
			ok := false
			if tags, tgok := Conf().TagGroups[s]; tgok {
				for _, tag := range tags {
					if ok = ok || HasTag(tag, p, exp.Doc); ok {
						break
					}
				}
			}
			return ok, nil
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

func EvalString(exp *Expr, v *org.Section, f *common.OrgFile) bool {
	parameters := make(map[string]interface{}, 8)
	parameters["section"] = v
	// This is the implicit this pointer of our expressions
	exp.Sec = v
	exp.Doc = f.Doc
	exp.File = f
	result, _ := exp.Expression.Evaluate(parameters)
	if result != nil {
		return result.(bool)
	}
	return false
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
		props := map[string]string{}
		if s.Headline.Properties != nil && len(s.Headline.Properties.Properties) > 0 {
			for _, p := range s.Headline.Properties.Properties {
				props[p[0]] = p[1]
			}
		}
		td.Props = props
		var contentNodes []org.Node = s.Headline.Children
		for i, n := range s.Headline.Children {
			switch n.(type) {
			case org.Headline:
				contentNodes = s.Headline.Children[0:i]
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
		props := map[string]string{}
		if s.Headline.Properties != nil && len(s.Headline.Properties.Properties) > 0 {
			for _, p := range s.Headline.Properties.Properties {
				props[p[0]] = p[1]
			}
		}
		td.Props = props
		var contentNodes []org.Node = s.Headline.Children
		for i, n := range s.Headline.Children {
			switch n.(type) {
			case org.Headline:
				contentNodes = s.Headline.Children[0:i]
			}
		}
		w := org.NewHTMLWriter()
		org.WriteNodes(w, contentNodes...)
		td.Content = w.String()
		return td, nil
	}
	return td, fmt.Errorf("failed to find todo by hash")
}

func QueryFullFileHtml(query *common.TodoHash) (common.FullTodo, error) {
	var td common.FullTodo
	if f := GetDb().FindByFile((string)(*query)); f != nil {
		w := org.NewHTMLWriter()
		org.WriteNodes(w, f.Doc.Nodes...)
		td.Content = w.String()
		return td, nil
	}
	return td, fmt.Errorf("failed to find todo by hash")
}

func SectionToTodo(v *org.Section, f *common.OrgFile) *common.Todo {
	var title string
	if f == nil {
		f = GetDb().FileFromSection(v)
		if f == nil {
			return nil
		}
	}

	for _, n := range v.Headline.Title {
		title += n.String()
	}
	var date *org.OrgDate = nil
	if v.Headline.Scheduled != nil {
		date = v.Headline.Scheduled.Date
	}
	if v.Headline.Timestamp != nil {
		date = v.Headline.Timestamp.Time
	}
	props := map[string]string{}
	if v.Headline.Properties != nil && len(v.Headline.Properties.Properties) > 0 {
		for _, p := range v.Headline.Properties.Properties {
			props[p[0]] = p[1]
		}
	}
	par := ""
	if v != nil && v.Parent != nil && v.Parent.Hash != "" {
		par = v.Parent.Hash
	}
	var t common.Todo = common.Todo{Parent: par, Headline: title, Tags: v.Headline.Tags, Hash: v.Hash, Date: date, Status: v.Headline.Status, Filename: f.Filename, LineNum: v.Headline.Pos.Row, IsActive: IsActive(v, f), Props: props, Level: v.Headline.Lvl}
	return &t
}

func FindByHash(hash *common.TodoHash) *common.Todo {
	var h string = ""
	if hash != nil {
		h = (string)(*hash)
	}
	v := GetDb().FindByHash(h)
	if v != nil {
		t := SectionToTodo(v, nil)
		return t
	}
	return nil
}

func FindByAnyId(hash *common.TodoHash) *common.Todo {
	var h string = ""
	if hash != nil {
		h = (string)(*hash)
	}
	v := GetDb().FindByAnyId(h)
	if v != nil {
		t := SectionToTodo(v, nil)
		return t
	}
	return nil
}

func NextSibling(hash *common.TodoHash) *common.Todo {
	var h string = ""
	if hash != nil {
		h = (string)(*hash)
	}
	v := GetDb().NextSibling(h)
	if v != nil {
		t := SectionToTodo(v, nil)
		return t
	}
	return nil
}

func PrevSibling(hash *common.TodoHash) *common.Todo {
	var h string = ""
	if hash != nil {
		h = (string)(*hash)
	}
	v := GetDb().PrevSibling(h)
	if v != nil {
		t := SectionToTodo(v, nil)
		return t
	}
	return nil
}

func LastChild(hash *common.TodoHash) *common.Todo {
	var h string = ""
	if hash != nil {
		h = (string)(*hash)
	}
	v := GetDb().LastChild(h)
	if v != nil {
		t := SectionToTodo(v, nil)
		return t
	}
	return nil
}

func ProcessNode(exp *Expr, v *org.Section, f *common.OrgFile, todos common.Todos) (common.Todos, error) {
	GetDb().RegisterSection(v.Hash, v, f)
	res := EvalString(exp, v, f)
	if res {
		var t *common.Todo = SectionToTodo(v, f)
		todos = append(todos, *t)
	}
	for _, c := range v.Children {
		todos, _ = ProcessNode(exp, c, f, todos)
	}
	return todos, nil
}

func GetAllTodosFromFile(v *org.Section, f *common.OrgFile, todos common.Todos) (common.Todos, error) {
	GetDb().RegisterSection(v.Hash, v, f)
	var t *common.Todo = SectionToTodo(v, f)
	todos = append(todos, *t)
	for _, c := range v.Children {
		todos, _ = GetAllTodosFromFile(c, f, todos)
	}
	return todos, nil
}

func EvalForNodes(exp *Expr, v *org.Section, f *common.OrgFile, nodes []*org.Section) ([]*org.Section, error) {
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

func QueryStringNodesOnFile(query string, file *common.OrgFile) ([]*org.Section, error) {
	var nodes []*org.Section

	// Render {{ FILTER }} in our template
	ctx := Conf().PlugManager.Tempo.GetAugmentedStandardContextFromStringMap(Conf().Filters, true)
	query = Conf().PlugManager.Tempo.ExecuteTemplateString(query, ctx)

	exp, err := ParseString(&common.StringQuery{Query: query})
	if err != nil {
		return nodes, err
	}
	for _, v := range file.Doc.Outline.Children {
		nodes, _ = EvalForNodes(exp, v, file, nodes)
	}
	return nodes, nil
}

func QueryStringTodos(query *common.StringQuery) (*common.Todos, error) {
	var todos common.Todos
	files := GetDb().GetFiles()
	fmt.Printf("    > QUERY: %s\n", query.Query)

	// Render {{ FILTER }} in our template
	ctx := Conf().PlugManager.Tempo.GetAugmentedStandardContextFromStringMap(Conf().Filters, true)
	query.Query = Conf().PlugManager.Tempo.ExecuteTemplateString(query.Query, ctx)

	fmt.Printf("    > QUERY AFTER EXPANSION: %s\n", query.Query)
	exp, err := ParseString(query)
	if err != nil {
		return &todos, err
	}
	for _, file := range files {
		f := GetDb().GetFile(file)
		for _, v := range f.Doc.Outline.Children {
			todos, _ = ProcessNode(exp, v, f, todos)
		}
	}
	return &todos, nil
}

func Grep(query string, delimeter string) ([]string, error) {
	res := []string{}
	if re, err := regexp.Compile(query); err != nil {
		fmt.Printf("ERROR: failed to compile query: %v", err)
		return res, err
	} else {
		files := GetDb().GetFiles()
		for _, file := range files {
			fh, err := os.Open(file)
			if err != nil {
				return res, fmt.Errorf("error opening file %s: %v", file, err)
			}
			defer fh.Close()
			scanner := bufio.NewScanner(fh)
			buf := make([]byte, 0, 64*1024)
			scanner.Buffer(buf, 1024*1024)
			lineNumber := 0
			for scanner.Scan() {
				lineNumber++
				line := scanner.Text()
				if re.MatchString(line) {
					res = append(res, fmt.Sprintf("%s%s%d%s%d%s%s", file, delimeter, lineNumber, delimeter, 0, delimeter, line))
				}
			}

			if err := scanner.Err(); err != nil {
				return res, fmt.Errorf("error reading file %s: %v", file, err)
			}
		}
	}
	return res, nil
}

func GetAllTodosInFile(filename string) (*common.Todos, error) {
	if filename == "" {
		files := GetDb().GetFiles()
		var todos common.Todos
		for _, file := range files {
			f := GetDb().GetFile(file)
			for _, v := range f.Doc.Outline.Children {
				todos, _ = GetAllTodosFromFile(v, f, todos)
			}
		}
		return &todos, nil
	} else {
		if f := GetDb().GetFile(filename); f != nil {
			var todos common.Todos
			for _, v := range f.Doc.Outline.Children {
				todos, _ = GetAllTodosFromFile(v, f, todos)
			}
			return &todos, nil
		}
	}
	return nil, fmt.Errorf("could not locate file: %s", filename)
}

func FindNodeFromPos(sec *org.Section, pos int, file *common.OrgFile) *org.Section {
	for _, c := range sec.Children {
		odb.RegisterSection(c.Hash, c, file)
		if pos >= c.Headline.GetPos().Row && pos <= c.Headline.GetEnd().Row {
			return FindNodeFromPos(c, pos, file)
		}
	}
	return sec
}

func FindNodeInFile(pos int, fname string) (string, error) {
	file := GetDb().FindByFile(fname)
	if file == nil {
		return "", fmt.Errorf("failed to find file: %s", fname)
	}
	for _, c := range file.Doc.Outline.Children {
		odb.RegisterSection(c.Hash, c, file)
		if pos >= c.Headline.GetPos().Row && pos <= c.Headline.GetEnd().Row {
			return FindNodeFromPos(c, pos, file).Hash, nil
		}
	}
	return "", fmt.Errorf("did not find node mapping to %v", pos)
}

func QueryProjects() common.Todos {
	var todos common.Todos
	files := GetDb().GetFiles()
	for _, file := range files {
		f := GetDb().GetFile(file)
		for _, v := range f.Doc.Outline.Children {
			if IsProject(v, f) {
				var title string
				for _, n := range v.Headline.Title {
					title += n.String()
				}
				var t common.Todo = common.Todo{Headline: title, Tags: v.Headline.Tags, Level: v.Headline.Lvl}
				todos = append(todos, t)
			}
		}
	}
	return todos
}

func WriteOutOrgFile(f *common.OrgFile) bool {
	if f == nil {
		fmt.Printf("INVALID (NIL) DOCUMENT PASSED TO WRITEOUTORGFILE, SKIPPING!")
		return false
	}
	// Need the doc to serialize and write it out.
	w := org.NewOrgWriter()
	//w.Indent = "  "
	f.Doc.Write(w)
	err := ioutil.WriteFile(f.Filename, []byte(w.String()), os.ModePerm)
	return err == nil
}

func SetThingChildren(n *org.Headline, s *org.Section, doit func(head *org.Headline) org.Headline) bool {
	for i := range n.Children {
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
func SetThing(f *common.OrgFile, s *org.Section, doit func(head *org.Headline) org.Headline) bool {
	for i := range f.Doc.Nodes {
		switch n := f.Doc.Nodes[i].(type) {
		case org.Headline:
			if n.Index == s.Headline.Index {
				f.Doc.Nodes[i] = doit(&n)
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
		return common.Result{Ok: false}, fmt.Errorf("status value is not valid for this item")
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
	return common.Result{Ok: didWrite}, nil
}

func IsPropertyNameValid(hash *common.TodoHash, name string) bool {
	return true
}

func IsPropertyValueValid(hash *common.TodoHash, val string) bool {
	return true
}

// The core lib does not have this option, we want it, eventually move this up!
func SetProperty(n *org.Headline, key string, val string) {
	props := &n.Properties.Properties
	if props == nil {
		return
	}
	for _, kvPair := range *props {
		if kvPair[0] == key {
			kvPair[1] = val
			return
		}
	}
	kvPair := []string{key, val}
	*props = append(*props, kvPair)
}

func ChangeProperty(query *common.TodoPropertyChange) (common.Result, error) {
	didWrite := true
	hh := common.TodoHash(query.Hash)
	if !IsPropertyNameValid(&hh, query.Name) {
		return common.Result{Ok: false}, fmt.Errorf("property name is not valid for this item")
	}
	if !IsPropertyValueValid(&hh, query.Value) {
		return common.Result{Ok: false}, fmt.Errorf("property value is not valid for this item")
	}
	if s, ok := GetDb().ByHash[(string)(query.Hash)]; ok {
		// Change the status
		f := GetDb().ByHashToFile[(string)(query.Hash)]
		if set := SetThing(f, s, func(n *org.Headline) org.Headline {
			SetProperty(n, query.Name, query.Value)
			return *n
		}); set {
			didWrite = WriteOutOrgFile(f)
		}
	}
	return common.Result{Ok: didWrite}, nil
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

func Reformat(query *common.FileList) (common.Result, error) {
	fmt.Printf("[REFORMAT CALLED]\n")
	didWrite := true
	for _, filename := range *query {
		fmt.Printf("  reformat: [%v]\n", filename)
		f := GetDb().FindByFile(filename)
		didWrite = didWrite && WriteOutOrgFile(f)
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

func ValidStatusFromFile(f *common.OrgFile) ([]string, []string) {
	var active []string
	var done []string
	if f != nil {
		// #+TODO: REPORT BUG KNOWNCAUSE | FIXED
		ftagstr := f.Doc.Get("TODO")
		if ftagstr != "" {
			active, done = ParseTodoStates(ftagstr)
		} else {
			active, done = ParseTodoStates(Conf().Server.DefaultTodoStates)
		}
	} else {
		active, done = ParseTodoStates(Conf().Server.DefaultTodoStates)
	}
	return active, done
}

func NextStatusFromFile(f *common.OrgFile) ([]string, []string) {
	var active []string
	var done []string
	if f != nil {
		// #+NEXT: NEXT | NEXTBACKLOG
		ftagstr := f.Doc.Get("NEXT")
		if ftagstr != "" {
			active, done = ParseTodoStates(ftagstr)
		} else {
			active, done = ParseTodoStates(Conf().Server.DefaultNextStates)
		}
	} else {
		active, done = ParseTodoStates(Conf().Server.DefaultNextStates)
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
		active, done = ParseTodoStates(Conf().Server.DefaultTodoStates)
	}
	states := common.TodoStatesResult{Active: active, Done: done}
	return states, nil
}

func IsActive(v *org.Section, f *common.OrgFile) bool {
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

func SetMarkerTag(target *common.ExclusiveTagMarker) (common.Result, error) {
	res := common.Result{}
	res.Ok = false
	// Only do anything if you have a valid target!
	if _, sec := GetDb().GetFromTarget(&target.ToId, true); sec != nil {
		query := "!IsArchived() && (HasTags('" + target.Name + "'))"
		q := common.StringQuery{Query: query}
		toggle := common.TodoItemChange{Value: target.Name}
		// Turn off today on anything that already has it.
		if reply, err := QueryStringTodos(&q); err == nil {
			if reply != nil {
				for _, t := range *reply {
					toggle.Hash = t.Hash
					ToggleTag(&toggle)
				}
			}
		}
		// Turn on today on the target. Have to RE load the target
		// as modifying tags could have invalidated our section
		_, sec = GetDb().GetFromTarget(&target.ToId, true)
		toggle.Hash = sec.Hash
		return ToggleTag(&toggle)
	}
	return res, nil
}

func GetMarkerTag(name string) (*common.Todos, error) {
	Conf().Out.Infof("GetMarkerTag: %s\n", name)
	if name == "" {
		Conf().Out.Errorf("GetMarkerTag called with empty marker")
	}
	query := "!IsArchived() && (HasTags('" + name + "'))"
	q := common.StringQuery{Query: query}
	// Turn off today on anything that already has it.
	return QueryStringTodos(&q)
}
