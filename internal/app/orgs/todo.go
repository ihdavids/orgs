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
  - *HasTagMatching* - returns true if any tag on the node, its parents or the file matches a regular expression: HasTagMatching("^M[0-9]+$")
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
  - *IsRecord* - returns true for a record heading (anything with a RECORD property); IsRecord("contact") asks about one collection
  - *IsCollection* - returns true for the container heading a collection files its records under
  - *InCollection* - InCollection("contact") is true for anything filed in that collection - the records, the container, and anything written under it
  - *HasBacklinks* - returns true when something else links at this heading; HasBacklinks(2) asks for at least two
  - *BacklinkCount* - how many links point at this heading, as a number to compare: BacklinkCount() == 0 finds the orphans
  - *HasLinks* - returns true when this heading links out at all
  - *LinksTo* - LinksTo(REGEX) is true when a link under this heading points at something matching - the target as written, its description, or the file and headline it lands on
  - *HasBrokenLinks* - returns true when a link under this heading resolves to nothing
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
	htmlexp "github.com/ihdavids/orgs/internal/app/orgs/plugs/html"
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
			fmt.Fprintf(os.Stderr, "*** HAVE CLOSED %v vs %v %s", t, p.Headline.Timestamp.Time, p.Headline.Title[0])
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

// The text written under a heading, without its child headings: what a person
// would call the contents of the node.
//
// Built from the parsed nodes rather than from the file, because this runs
// inside a query over every heading in the database and reading a file per
// heading would make a search of a big org directory unusable.
func HeadingText(p *org.Section) string {
	if p == nil || p.Headline == nil {
		return ""
	}
	var b strings.Builder
	for _, n := range p.Headline.Children {
		// A child heading is its own node and its own search result.
		if isHeadline(n) {
			break
		}
		b.WriteString(n.String())
		b.WriteString("\n")
	}
	return b.String()
}

// Whether a body node is actually a nested heading.
//
// go-org's parser builds a heading as `&Headline{...}`, so what sits in
// `Children` is a *Headline - and a type switch on the value type never fires.
// Written out here because getting it wrong is silent: a heading's "own body"
// quietly becomes its whole subtree, which reads as a search that matches too
// much rather than as a bug.
func isHeadline(n org.Node) bool {
	switch n.(type) {
	case org.Headline, *org.Headline:
		return true
	}
	return false
}

// Run a regular expression over a heading's own text. The headline is included
// because "the contents" of a node, to somebody searching, is the thing they
// can see - and a match that skipped the title would be a surprise.
func ContentMatchesRe(p *org.Section, re string) bool {
	if p == nil || p.Headline == nil {
		return false
	}
	var title string
	for _, n := range p.Headline.Title {
		title += n.String()
	}
	if ok, err := regexp.MatchString(re, title+"\n"+HeadingText(p)); err == nil && ok {
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
// IsBlockedProject is a project with nothing to do next: a heading that has
// children with keywords on them, none of which is NEXT.
//
// It used to return the opposite of its own name - `childHasNext`, so it
// answered true for exactly the projects that were *not* blocked. docs.org has
// always described it the right way round ("DOES NOT have a child marked
// NEXT"), so the documentation was right and the code was wrong; this is what a
// weekly review asks first and it was answering with the healthy projects.
//
// The projectRe argument was never read and is kept only so a query that passes
// one still parses. The expression wrapper no longer requires it.
func IsBlockedProject(p *org.Section, projectRe string, f *common.OrgFile) bool {
	if p == nil || p.Headline == nil || !IsProject(p, f) {
		return false
	}
	for _, c := range p.Children {
		if IsNextTask(c, f) {
			return false
		}
	}
	return true
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
	// The parameter map handed to Evaluate, reused across sections. See
	// EvalString.
	params map[string]interface{}
	// The first bad call to a query function: the wrong number or kind of
	// arguments. It is the query's fault, so it is the same on every heading
	// and is reported once as the query's error. See guardQueryFunctions.
	callErr error
}

// guardQueryFunctions makes a query function's bad call an error rather than
// a panic.
//
// The functions read their arguments as args[0].(string) and so on, which
// panics on MatchProperty() or MatchProperty("X") - and worg searches as you
// type, so the half-written call reaches the server on every keystroke and
// each one took a request down. Recovering here covers every function,
// including ones written later, rather than an arity check in each.
func guardQueryFunctions(exp *Expr, functions map[string]govaluate.ExpressionFunction) {
	for name, fn := range functions {
		name, fn := name, fn
		functions[name] = func(args ...interface{}) (res interface{}, err error) {
			defer func() {
				if r := recover(); r != nil {
					res, err = false, fmt.Errorf("%s: wrong arguments (%v)", name, r)
				}
				if err != nil && exp.callErr == nil {
					exp.callErr = err
				}
			}()
			return fn(args...)
		}
	}
}

// stringArgs reads a function's arguments as the n strings it needs, or says
// what it wanted in usage.
func stringArgs(args []interface{}, n int, usage string) ([]string, error) {
	if len(args) < n {
		return nil, fmt.Errorf("%s", usage)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		v, ok := args[i].(string)
		if !ok {
			return nil, fmt.Errorf("%s", usage)
		}
		out[i] = v
	}
	return out, nil
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
			// The argument is optional. It was required, and unused - so
			// `IsBlockedProject()`, which is how the documentation writes it and
			// the only way anybody would think to call it, panicked on
			// args[0] and the whole query answered nothing.
			re := ""
			if len(args) > 0 {
				if s, ok := args[0].(string); ok {
					re = s
				}
			}
			return IsBlockedProject(exp.Sec, re, exp.File), nil
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
		// A heading with checkboxes on it, and how far through them it is.
		// `HasChecklist()` is what the terminal's checkbox picker offers over;
		// `ChecklistDone()` is "every box ticked", which is not the same
		// question as IsStatus("DONE") and is the one a review wants -
		// a heading whose list is finished and whose keyword nobody moved.
		"HasChecklist": func(args ...interface{}) (interface{}, error) {
			return HasChecklist(exp.Sec, exp.File), nil
		},
		"ChecklistDone": func(args ...interface{}) (interface{}, error) {
			done, total := ChecklistCounts(exp.Sec, exp.File)
			return total > 0 && done == total, nil
		},
		"ChecklistCount": func(args ...interface{}) (interface{}, error) {
			_, total := ChecklistCounts(exp.Sec, exp.File)
			return total, nil
		},
		"ChecklistLeft": func(args ...interface{}) (interface{}, error) {
			done, total := ChecklistCounts(exp.Sec, exp.File)
			return total - done, nil
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
		// HasTagMatching("^M[0-9]+$") is true when any tag on the heading, its
		// parents or the file matches the regular expression - for families of
		// tags (M1, M2, ...) that HasTags would have to name one by one. A
		// pattern that does not compile is an error rather than a quiet false.
		"HasTagMatching": func(args ...interface{}) (interface{}, error) {
			if len(args) == 0 {
				return false, fmt.Errorf("HasTagMatching needs a pattern")
			}
			for _, a := range args {
				pat, ok := a.(string)
				if !ok {
					return false, fmt.Errorf("HasTagMatching takes text patterns")
				}
				if _, err := regexp.Compile(pat); err != nil {
					return false, fmt.Errorf("HasTagMatching: %v", err)
				}
				if HasTagRegex(pat, exp.Sec, exp.Doc) {
					return true, nil
				}
			}
			return false, nil
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
		// IsRecord() is true for any heading that is a record - a contact, a
		// piece of equipment, anything with a RECORD property on it.
		// IsRecord("contact") asks about one collection.
		//
		// It is here so that a query can leave the address book out of a
		// search: a contact matches "jane" as readily as the task about
		// ringing her, and it is rarely the one being looked for.
		"IsRecord": func(args ...interface{}) (interface{}, error) {
			t := RecordTypeOf(exp.Sec)
			if t == "" {
				return false, nil
			}
			if len(args) > 0 {
				if want, ok := args[0].(string); ok && want != "" {
					return strings.EqualFold(t, want), nil
				}
			}
			return true, nil
		},
		// IsCollection() is true for the container heading a collection files
		// its records under, which is not itself a record.
		"IsCollection": func(args ...interface{}) (interface{}, error) {
			return CollectionTypeOf(exp.Sec) != "", nil
		},
		// InCollection("contact") is true for anything filed in that
		// collection: a record carrying that RECORD value, the container
		// heading that holds them, and anything written underneath it.
		//
		// IsRecord("contact") is the strict one and answers only for the
		// records themselves. This is the one to reach for when a query means
		// "leave the address book alone" - the notes under a contact are part
		// of the address book too.
		"InCollection": func(args ...interface{}) (interface{}, error) {
			want := ""
			if len(args) > 0 {
				if s, ok := args[0].(string); ok {
					want = s
				}
			}
			return InCollection(exp.Sec, want), nil
		},
		// HasBacklinks() is true for a heading something else points at.
		// HasBacklinks(2) asks for at least that many - the argument is a
		// floor, not an exact count.
		//
		// A link that names the *file* rather than a heading in it does not
		// count towards any heading in it, or every heading in a linked-to
		// file would claim a backlink it has not got.
		"HasBacklinks": func(args ...interface{}) (interface{}, error) {
			least := 1.0
			if len(args) > 0 {
				if n, ok := args[0].(float64); ok {
					least = n
				}
			}
			return float64(BacklinksTo(exp.Sec.Hash)) >= least, nil
		},
		// BacklinkCount() is the number itself, for a query that wants to
		// compare it: BacklinkCount() > 3, or == 0 for the orphans.
		"BacklinkCount": func(args ...interface{}) (interface{}, error) {
			return float64(BacklinksTo(exp.Sec.Hash)), nil
		},
		// HasLinks() is true for a heading that links out at all.
		"HasLinks": func(args ...interface{}) (interface{}, error) {
			return len(LinksOut(fileNameOf(exp.File), exp.Sec.Hash)) > 0, nil
		},
		// LinksTo(RE) is true when a link written under this heading points at
		// something matching the expression. The pattern is run over the link
		// as written, its description, and - for a link that stays inside the
		// org files - the file and headline it lands on, because "links to
		// notes.org" and "links to the migration heading" are both things
		// somebody means by it.
		"LinksTo": func(args ...interface{}) (interface{}, error) {
			if len(args) == 0 {
				return false, nil
			}
			pat, ok := args[0].(string)
			if !ok || pat == "" {
				return false, nil
			}
			re, err := regexp.Compile("(?i)" + pat)
			if err != nil {
				return false, err
			}
			for _, l := range LinksOut(fileNameOf(exp.File), exp.Sec.Hash) {
				if re.MatchString(l.Raw) || re.MatchString(l.Desc) ||
					re.MatchString(l.To.Filename) || re.MatchString(l.To.Headline) {
					return true, nil
				}
			}
			return false, nil
		},
		// HasBrokenLinks() is true for a heading holding a link that resolves
		// to nothing. What a link tidy-up query is made of.
		"HasBrokenLinks": func(args ...interface{}) (interface{}, error) {
			for _, l := range LinksOut(fileNameOf(exp.File), exp.Sec.Hash) {
				if l.Broken {
					return true, nil
				}
			}
			return false, nil
		},
		// Returns true if the headline has the specific property
		// A habit is a heading with :STYLE: habit and a repeating schedule. These
		// three read the history that marking something done now writes - see
		// habit.go - so they answer about orgs' own data rather than about
		// whatever Emacs last left in the drawer.
		"IsHabit": func(args ...interface{}) (interface{}, error) {
			return IsHabitSection(exp.Sec), nil
		},
		// How many times in a row it has been kept, as a number to compare:
		// `IsHabit() && HabitStreak() > 7`. Zero when the run is already broken,
		// because a streak is one you are currently on.
		"HabitStreak": func(args ...interface{}) (interface{}, error) {
			return (float64)(HabitStreak(exp.Sec, time.Now())), nil
		},
		// Left longer than its own repeater allows. The question a review asks.
		"MissedHabit": func(args ...interface{}) (interface{}, error) {
			return MissedHabit(exp.Sec, time.Now()), nil
		},
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
			a, err := stringArgs(args, 2, `MatchProperty needs a property name and a pattern: MatchProperty("CUSTOM_ID", "^abc")`)
			if err != nil {
				return false, err
			}
			if val, ok := exp.Sec.Headline.Properties.Get(a[0]); ok {
				if ok, err := regexp.MatchString(a[1], val); err == nil && ok {
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
		// Run an RE against the heading and everything written under it, short
		// of its child headings - which are their own results.
		"MatchContent": func(args ...interface{}) (interface{}, error) {
			p := exp.Sec
			s := args[0].(string)
			return ContentMatchesRe(p, s), nil
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
		// The planning lines, as questions rather than as values.
		//
		// The query language could ask what a heading *was* - its keyword, its
		// tags, its properties - and could ask whether a date fell on a
		// particular day, but could not ask the two questions a review is made
		// of: does this have a date at all, and has that date gone past. Every
		// one of these is a field that was already on the headline.
		"HasScheduled": func(args ...interface{}) (interface{}, error) {
			return exp.Sec != nil && exp.Sec.Headline != nil &&
				exp.Sec.Headline.Scheduled != nil, nil
		},
		"HasDeadline": func(args ...interface{}) (interface{}, error) {
			return exp.Sec != nil && exp.Sec.Headline != nil &&
				exp.Sec.Headline.Deadline != nil, nil
		},
		"HasTimestamp": func(args ...interface{}) (interface{}, error) {
			return exp.Sec != nil && exp.Sec.Headline != nil &&
				exp.Sec.Headline.Timestamp != nil, nil
		},
		"HasAnyDate": func(args ...interface{}) (interface{}, error) {
			h := exp.Sec.Headline
			return exp.Sec != nil && h != nil &&
				(h.Scheduled != nil || h.Deadline != nil || h.Timestamp != nil), nil
		},
		// Past, meaning strictly before today rather than before this instant:
		// a deadline of today is due, not overdue, and a review that called it
		// overdue at nine in the morning would be wrong for the rest of the day.
		"DeadlinePast": func(args ...interface{}) (interface{}, error) {
			return datePast(sdcDate(exp.Sec, "DEADLINE")), nil
		},
		"ScheduledPast": func(args ...interface{}) (interface{}, error) {
			return datePast(sdcDate(exp.Sec, "SCHEDULED")), nil
		},
		// Whether time has ever been booked against this heading, which is what
		// tells "still open and worked on" from "still open and forgotten".
		"HasClock": func(args ...interface{}) (interface{}, error) {
			return exp.Sec != nil && exp.Sec.Headline != nil &&
				len(exp.Sec.Headline.Clocks) > 0, nil
		},
		// How long ago the heading's own date was, in days, as a number to
		// compare: OlderThan(30) is a month of nothing happening. A heading
		// with no date at all answers -1, so it never satisfies OlderThan.
		"DaysOld": func(args ...interface{}) (interface{}, error) {
			return daysOld(exp.Sec), nil
		},
		"OlderThan": func(args ...interface{}) (interface{}, error) {
			if len(args) == 0 {
				return false, fmt.Errorf("OlderThan needs a number of days")
			}
			want, err := toFloat(args[0])
			if err != nil {
				return false, err
			}
			d := daysOld(exp.Sec)
			return d >= 0 && float64(d) >= want, nil
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
	guardQueryFunctions(exp, functions)
	//expString := "strlen('someReallyLongInputString') <= 16"
	var err error
	exp.Expression, err = govaluate.NewEvaluableExpressionWithFunctions(expString.Query, functions)
	return exp, err
}

func EvalString(exp *Expr, v *org.Section, f *common.OrgFile) bool {
	// The parameter map belongs to the expression rather than to the call.
	//
	// It was allocated fresh for every section - seven thousand maps per query
	// here - and it holds one entry that the expression only ever reads. An
	// Expr is used by one query at a time, which is what makes reusing it safe.
	if exp.params == nil {
		exp.params = make(map[string]interface{}, 1)
	}
	exp.params["section"] = v
	// This is the implicit this pointer of our expressions
	exp.Sec = v
	exp.Doc = f.Doc
	exp.File = f
	result, _ := exp.Expression.Evaluate(exp.params)
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
		// This heading's own body: everything up to the first child heading.
		// Stopping at the *first* one matters as much as recognising one at
		// all - taking the last would keep every heading before it.
		var contentNodes []org.Node = s.Headline.Children
		for i, n := range s.Headline.Children {
			if isHeadline(n) {
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

// The configured html exporter, or nil when the server has none. It is what
// knows the themes and how to turn a picture or a recording in a heading into
// something a browser can fetch, so a caller that wants a heading rendered the
// way the file view renders a page has to go through it rather than through
// go-org's plain writer.
func htmlExporter() *htmlexp.OrgHtmlExporter {
	for _, exp := range Conf().Server.Exporters {
		if exp.Name != "html" {
			continue
		}
		if h, ok := exp.Plugin.(*htmlexp.OrgHtmlExporter); ok {
			return h
		}
	}
	return nil
}

func QueryFullTodoHtml(query *common.TodoHash) (common.FullTodo, error) {
	return QueryFullTodoHtmlThemed(query, "")
}

// One heading rendered to html. A theme name renders it through the html
// exporter instead of go-org's plain writer and hands the theme's stylesheet
// back beside the fragment, so a client showing the heading on its own can
// style it the same way the exported page is styled. An empty name is the
// bare fragment every other caller already expects.
func QueryFullTodoHtmlThemed(query *common.TodoHash, theme string) (common.FullTodo, error) {
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
		// This heading's own body: everything up to the first child heading.
		// Stopping at the *first* one matters as much as recognising one at
		// all - taking the last would keep every heading before it.
		var contentNodes []org.Node = s.Headline.Children
		for i, n := range s.Headline.Children {
			if isHeadline(n) {
				contentNodes = s.Headline.Children[0:i]
				break
			}
		}
		// Asking for a theme is asking for the exporter's rendering as well
		// as its stylesheet: a themed page whose pictures are still file:
		// links would be styled and broken.
		if exp := htmlExporter(); theme != "" && exp != nil {
			td.Content = exp.RenderFragment(contentNodes...)
			td.Style = exp.ThemeStyle(theme)
			return td, nil
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

var habitDoneRe = regexp.MustCompile(`.*State\s+"DONE".*\[(\d{4}[-]\d{2}[-]\d{2})`)

// The days a habit was completed on, read out of the `State "DONE"` lines in its
// own logbook - which is where org puts them and, since ChangeStatus learnt to
// write them, where orgs puts them too.
//
// Two things here were wrong and both were silent. The drawer was matched as
// `org.Drawer`, and the parser only ever produces `*org.Drawer` - the same
// pointer trap as `*org.Headline` - so the loop never fired and every habit came
// back with no completions whatever was in its logbook. And the drawer looked for
// was `LOGBOOK` alone, which is right for the default and wrong for anybody who
// pointed `org-log-into-drawer` somewhere else: the history is wherever this
// heading's settings say it is.
func parseHabitCompletions(v *org.Section) []string {
	var completions []string
	if v == nil || v.Headline == nil {
		return nil
	}
	want := "LOGBOOK"
	if f := GetDb().FileFromSection(v); f != nil {
		if d := fileLogSettings(f).drawer; d != "" {
			want = d
		}
	}
	read := func(name, text string) {
		if !strings.EqualFold(name, want) {
			return
		}
		for _, m := range habitDoneRe.FindAllStringSubmatch(text, -1) {
			completions = append(completions, m[1])
		}
	}
	for _, n := range v.Headline.Children {
		switch d := n.(type) {
		case *org.Drawer:
			read(d.Name, d.String())
		case org.Drawer:
			read(d.Name, d.String())
		}
	}
	// A heading whose log went into the body rather than into a drawer keeps the
	// same lines as a plain list, so they are worth reading there too - otherwise
	// turning the drawer off would turn the habit graph off with it.
	if len(completions) == 0 {
		for _, n := range v.Headline.Children {
			switch l := n.(type) {
			case *org.List:
				read(want, l.String())
			case org.List:
				read(want, l.String())
			}
		}
	}
	return completions
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
	var deadline *org.OrgDate
	if v.Headline.Deadline != nil {
		deadline = v.Headline.Deadline.Date
	}
	var completions []string
	if props["STYLE"] == "habit" {
		completions = parseHabitCompletions(v)
	}
	var t common.Todo = common.Todo{Parent: par, Headline: title, Tags: v.Headline.Tags, Hash: v.Hash, Date: date, Deadline: deadline, Status: v.Headline.Status, Priority: v.Headline.Priority, Filename: f.Filename, LineNum: v.Headline.Pos.Row, IsActive: IsActive(v, f), Props: props, Level: v.Headline.Lvl, Completions: completions}
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
	// Registration used to happen here, once per heading per query. It is done
	// once per version of each file instead - see registerAllSections.
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

	// Render {{ FILTER }} in our template
	ctx := Conf().PlugManager.Tempo.GetAugmentedStandardContextFromStringMap(Conf().Filters, true)
	before := query.Query
	query.Query = Conf().PlugManager.Tempo.ExecuteTemplateString(query.Query, ctx)
	// Said once, and only where a filter actually expanded into something else.
	// It used to be two lines per query whatever happened, which on a client
	// that queries as you type is two lines of log per keystroke.
	if query.Query != before {
		fmt.Fprintf(os.Stderr, "    > QUERY: %s -> %s\n", before, query.Query)
	}
	exp, err := ParseString(query)
	if err != nil {
		return &todos, err
	}

	// Every section is registered into the hash index once per version of its
	// file rather than once per query.
	//
	// ProcessNode used to do it on the way past, which meant a query took the
	// database's *write* lock once per heading - seven thousand times here -
	// re-registering sections that were already registered, and blocking every
	// other reader while it did. See registerFileSections.
	registerAllSections()

	// The files this query could possibly match. A query that demands a keyword
	// or a tag skips every file whose summary says it has none; anything else
	// gets the whole database, exactly as before. See queryindex.go.
	files, _ := filesForQuery(query.Query)
	for _, file := range files {
		f := GetDb().GetFile(file)
		if f == nil || f.Doc == nil {
			continue
		}
		for _, v := range f.Doc.Outline.Children {
			todos, _ = ProcessNode(exp, v, f, todos)
		}
		if exp.callErr != nil {
			return &todos, exp.callErr
		}
	}
	return &todos, nil
}

// Registration, done once per version of each file.
//
// The sections of a file only change when the file is read again, so this is a
// per-file cache like the others (see fileparts.go) and costs nothing on a
// query that changes nothing.
var sectionRegistry = NewFileParts[int]()

func registerAllSections() {
	sectionRegistry.All(func(f *common.OrgFile) int {
		n := 0
		for _, sec := range flattenSections(f) {
			if sec.Hash != "" {
				GetDb().RegisterSection(sec.Hash, sec, f)
				n++
			}
		}
		return n
	})
}

func Grep(query string, delimeter string) ([]string, error) {
	res := []string{}
	if re, err := regexp.Compile(query); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to compile query: %v", err)
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

// Search for a file by filename in the db
func FindFileInDb(filename string) (string, error) {
	f := GetDb().FindByFile(filename)
	if f != nil {
		return f.Doc.Path, nil
	}
	return "", fmt.Errorf("Could not find file: %s", filename)
}

// Will return all the headings found in a particular file.
// If FILENAME is empty will return the la
func GetAllTodosInFile(filename string) (*common.Todos, error) {
	if filename == "" {
		// TODO This is broken, determine what is actually using this?
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
		if f := GetDb().FindByFile(filename); f != nil {
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
		fmt.Fprintf(os.Stderr, "INVALID (NIL) DOCUMENT PASSED TO WRITEOUTORGFILE, SKIPPING!")
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
		case *org.Headline:
			if nn.Index == s.Headline.Index {
				result := doit(nn)
				*nn = result
				return true
			}
			if SetThingChildren(nn, s, doit) {
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
		case *org.Headline:
			if n.Index == s.Headline.Index {
				result := doit(n)
				*n = result
				return true
			}
			if SetThingChildren(n, s, doit) {
				return true
			}
		}
	}
	log.Printf("Did not find headline in update\n")
	return false
}

// ChangeStatus moves a heading to another todo keyword, and does what org does
// when it lands on a done one: stamps CLOSED, writes down which state it came
// from, and moves a repeating date on rather than leaving the keyword stuck.
//
// See logbook.go for all of that and for the three places it can be configured.
// Two things about this function itself changed with it:
//
//  1. **It is a line edit.** This used to set `Headline.Status` on the parsed
//     document and write the whole thing back through go-org, so ticking off one
//     task reformatted every drawer and reflowed every table in its file. Every
//     other writer in the server splices lines; there was never a reason for
//     this one to be the exception, and there is now a positive reason not to be:
//     the timestamps being rewritten have to keep the spelling they were written
//     with.
//  2. **It looks the heading up with FindByHash.** Reading `ByHash` directly
//     answers with whatever the last parse left there, and sections are
//     registered lazily - so on a freshly started server it answered with
//     nothing, and after any edit it could answer with a section from the parse
//     before.
func ChangeStatus(query *common.TodoItemChange) (common.Result, error) {
	hh := common.TodoHash(query.Hash)
	if !IsStatusValid(&hh, query.Value) {
		return common.Result{Ok: false}, fmt.Errorf("status value is not valid for this item")
	}
	sec := GetDb().FindByHash((string)(query.Hash))
	if sec == nil || sec.Headline == nil {
		return common.Result{Ok: false}, fmt.Errorf("no heading with that hash")
	}
	f := GetDb().ByHashToFile[(string)(query.Hash)]
	if f == nil {
		return common.Result{Ok: false}, fmt.Errorf("no file for that heading")
	}
	res, err := applyStatusChange(f, sec, query.Value, query.Note, time.Now())
	if err != nil {
		return common.Result{Ok: false}, err
	}
	return common.Result{Ok: true, Msg: statusChangeMessage(res, query.Value)}, nil
}

// What to tell a client that asked for DONE and got TODO back.
//
// Said out loud because it is surprising the first time: a kanban card dragged
// into Done that reappears in Next looks like the write failed. It did not - the
// heading repeats, which is what its own date says it should do.
func statusChangeMessage(res StatusChangeResult, asked string) string {
	if res.Repeated && res.Status != asked {
		return fmt.Sprintf("%s repeats: moved on and set back to %s", asked, res.Status)
	}
	return ""
}

func RenameHeadline(query *common.TodoItemChange) (common.Result, error) {
	didWrite := true
	if s, ok := GetDb().ByHash[(string)(query.Hash)]; ok {
		f := GetDb().ByHashToFile[(string)(query.Hash)]
		if set := SetThing(f, s, func(n *org.Headline) org.Headline {
			n.Title = []org.Node{org.Text{Content: query.Value}}
			return *n
		}); set {
			didWrite = WriteOutOrgFile(f)
		}
	}
	return common.Result{Ok: didWrite}, nil
}

func ChangeBody(query *common.TodoItemChange) (common.Result, error) {
	didWrite := true
	if s, ok := GetDb().ByHash[(string)(query.Hash)]; ok {
		f := GetDb().ByHashToFile[(string)(query.Hash)]
		// Parse the new body content as org-mode text
		bodyDoc := org.New().Parse(strings.NewReader(query.Value), "./")
		if set := SetThing(f, s, func(n *org.Headline) org.Headline {
			// Replace the body, and keep everything that is not the body.
			//
			// Two things live among a headline's children that a caller
			// changing its *text* is not talking about, and both used to be
			// thrown away here:
			//
			//  1. **Child headings.** The old loop matched `case org.Headline`,
			//     which never fires - a nested heading is a `*org.Headline`,
			//     and a type switch on the value form does not match the
			//     pointer. So every child of the heading, and their whole
			//     subtrees, were deleted by any change to the parent's body.
			//     Appending one line of note to a project heading took the
			//     project's tasks with it, with a `{"status":true}` in reply.
			//  2. **The planning line.** SCHEDULED/DEADLINE/CLOSED are SDC
			//     children (ChangeDate puts them there), so a body change
			//     dropped the heading's dates as well.
			//
			// The property drawer is safe either way - it hangs off
			// n.Properties rather than off Children.
			var planning []org.Node
			var childHeadlines []org.Node
			for _, c := range n.Children {
				switch c.(type) {
				case org.Headline, *org.Headline:
					childHeadlines = append(childHeadlines, c)
				case org.SDC, *org.SDC:
					planning = append(planning, c)
				}
			}
			// The planning line comes directly under the headline, then the
			// body, then the children - which is the order org writes them and
			// the order the parser expects to read them back in.
			kept := append([]org.Node{}, planning...)
			kept = append(kept, bodyDoc.Nodes...)
			n.Children = append(kept, childHeadlines...)
			return *n
		}); set {
			didWrite = WriteOutOrgFile(f)
		}
	}
	return common.Result{Ok: didWrite}, nil
}

// removeChildByType removes the first child node matching the given type from a headline's children.
func removeSDCChild(n *org.Headline, dtype org.DateType) {
	// Iterate backwards to safely remove all matching SDC children
	for i := len(n.Children) - 1; i >= 0; i-- {
		child := n.Children[i]
		if sdc, ok := child.(org.SDC); ok && sdc.DateType == dtype {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
		} else if sdc, ok := child.(*org.SDC); ok && sdc.DateType == dtype {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
		}
	}
}

// paragraphIsOnlyTimestamp returns true if the paragraph contains only
// whitespace text and a single timestamp (i.e. it was generated by a
// previous ChangeDate insertion that got re-parsed as inline content).
func paragraphIsOnlyTimestamp(p org.Paragraph) bool {
	hasTimestamp := false
	for _, child := range p.Children {
		switch c := child.(type) {
		case org.Timestamp, *org.Timestamp:
			hasTimestamp = true
		case org.Text:
			if strings.TrimSpace(c.Content) != "" {
				return false
			}
		default:
			return false
		}
	}
	return hasTimestamp
}

func removeTimestampChild(n *org.Headline) {
	// Iterate backwards to safely remove all matching Timestamp children.
	// After a write/re-parse cycle, a standalone Timestamp child becomes a
	// Paragraph wrapping an inline Timestamp, so check Paragraphs too.
	for i := len(n.Children) - 1; i >= 0; i-- {
		child := n.Children[i]
		if _, ok := child.(org.Timestamp); ok {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
		} else if _, ok := child.(*org.Timestamp); ok {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
		} else if p, ok := child.(org.Paragraph); ok {
			if paragraphIsOnlyTimestamp(p) {
				n.Children = append(n.Children[:i], n.Children[i+1:]...)
			}
		}
	}
}

// insertSDCChild inserts an SDC node at the beginning of the children list (after properties and existing SDCs).
func insertSDCChild(n *org.Headline, sdc *org.SDC) {
	// Insert after any PropertyDrawer or existing SDC nodes at the start
	idx := 0
	for i, child := range n.Children {
		if _, ok := child.(*org.PropertyDrawer); ok {
			idx = i + 1
		} else if _, ok := child.(org.PropertyDrawer); ok {
			idx = i + 1
		} else if _, ok := child.(org.SDC); ok {
			idx = i + 1
		} else if _, ok := child.(*org.SDC); ok {
			idx = i + 1
		} else {
			break
		}
	}
	// Only insert a trailing LineBreak if the next child is not already a LineBreak
	needsLB := true
	if idx < len(n.Children) {
		if _, ok := n.Children[idx].(org.LineBreak); ok {
			needsLB = false
		} else if _, ok := n.Children[idx].(*org.LineBreak); ok {
			needsLB = false
		}
	}
	if needsLB {
		lb := org.LineBreak{Count: 1}
		n.Children = append(n.Children[:idx], append([]org.Node{*sdc, lb}, n.Children[idx:]...)...)
	} else {
		n.Children = append(n.Children[:idx], append([]org.Node{*sdc}, n.Children[idx:]...)...)
	}
}

// ChangeDate is in datechange.go.

func IsPropertyNameValid(hash *common.TodoHash, name string) bool {
	return true
}

func IsPropertyValueValid(hash *common.TodoHash, val string) bool {
	return true
}

// The core lib does not have this option, we want it, eventually move this up!
func SetProperty(n *org.Headline, key string, val string) {
	// A heading that has never had a property gets a drawer here rather than
	// being left alone: the org writer prints Properties directly under the
	// headline when it is there, so the new drawer lands where org expects it.
	// Without this a heading with no drawer is a nil dereference, which is the
	// common case for anything setting a property from outside the editor -
	// the kanban board dropping a card into a column, say.
	// Setting a property to nothing takes it off the heading, rather than
	// leaving `:AFTER:` sitting there with no value after it. Anything undoing
	// a property it set writes the old value back, and for a property that was
	// not there before, the old value is nothing - so this is the difference
	// between undo tidying up after itself and undo leaving litter.
	if val == "" {
		if n.Properties == nil {
			return
		}
		kept := [][]string{}
		for _, kvPair := range n.Properties.Properties {
			if kvPair[0] != key {
				kept = append(kept, kvPair)
			}
		}
		if len(kept) == 0 {
			// An empty drawer is litter too, and the writer leaves a heading
			// with no Properties alone.
			n.Properties = nil
			return
		}
		n.Properties.Properties = kept
		return
	}
	if n.Properties == nil {
		n.Properties = &org.PropertyDrawer{Properties: [][]string{}}
	}
	props := &n.Properties.Properties
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
	hh := common.TodoHash(query.Hash)
	if !IsPropertyNameValid(&hh, query.Name) {
		return common.Result{Ok: false}, fmt.Errorf("property name is not valid for this item")
	}
	if !IsPropertyValueValid(&hh, query.Value) {
		return common.Result{Ok: false}, fmt.Errorf("property value is not valid for this item")
	}
	// A line edit rather than a document rewrite - see setHeadingProperty in
	// columns.go for why, and FindByHash rather than a bare ByHash read because
	// sections are registered lazily and the map answers with whatever the last
	// parse left there.
	sec := GetDb().FindByHash((string)(query.Hash))
	if sec == nil || sec.Headline == nil {
		return common.Result{Ok: false}, fmt.Errorf("no heading with that hash")
	}
	f := GetDb().ByHashToFile[(string)(query.Hash)]
	if f == nil {
		return common.Result{Ok: false}, fmt.Errorf("no file for that heading")
	}
	if err := setHeadingProperty(f, sec, query.Name, query.Value); err != nil {
		return common.Result{Ok: false}, err
	}
	return common.Result{Ok: true}, nil
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
	fmt.Fprintf(os.Stderr, "TOGGLE TAG CALLED: %s\n", query.Value)
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
	return common.Result{Ok: didWrite}, nil
}

func Reformat(query *common.FileList) (common.Result, error) {
	fmt.Fprintf(os.Stderr, "[REFORMAT CALLED]\n")
	didWrite := true
	for _, filename := range *query {
		fmt.Fprintf(os.Stderr, "  reformat: [%v]\n", filename)
		f := GetDb().FindByFile(filename)
		didWrite = didWrite && WriteOutOrgFile(f)
	}
	return common.Result{Ok: didWrite}, nil
}

// ParseTodoStates splits a `#+TODO:` line the way org splits it - everything
// before the bar is a live state and everything after it a finished one - and
// hands back the keywords themselves.
//
// The keywords themselves is the part that was wrong. A `#+TODO:` line written
// the way the org manual writes them carries a cookie on each keyword:
// `TODO(t) NEXT(n!) | DONE(d!)`, where the letter is a key to press and the `!`
// or `@` says to log something on entering that state. This used to hand back
// `NEXT(n!)` as the keyword - so in any file using the standard notation every
// keyword this server would accept was one no reader would recognise,
// `IsStatus("NEXT")` matched nothing, and the kanban offered `NEXT(n!)` as a
// column to drag a card into. The cookies are read properly in logbook.go,
// where what they ask for is acted on.
func ParseTodoStates(ftagstr string) ([]string, []string) {
	active, done := parseTodoKeywords(ftagstr)
	names := func(kws []TodoKeyword) []string {
		var out []string
		for _, k := range kws {
			if k.Name != "" && !contains(out, k.Name) {
				out = append(out, k.Name)
			}
		}
		return out
	}
	return names(active), names(done)
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

// InCollection reports whether a heading is filed in a record collection:
// either it is a record of that type, or it is the collection's container
// heading, or it is written somewhere underneath that container.
//
// The walk up the outline is what separates this from IsRecord. A contact's
// notes are not a record - they have no RECORD property of their own - but
// they are part of the address book, and a query saying "not the address
// book" means them too.
func InCollection(sec *org.Section, want string) bool {
	if sec == nil {
		return false
	}
	match := func(t string) bool {
		if t == "" {
			return false
		}
		return want == "" || strings.EqualFold(t, want)
	}
	if match(RecordTypeOf(sec)) {
		return true
	}
	for s := sec; s != nil; s = s.Parent {
		if match(CollectionTypeOf(s)) {
			return true
		}
	}
	return false
}

// The file a query is being evaluated against, without assuming there is one.
// A query runs over parsed files so there always is, but Expr carries a
// pointer and a nil deref here would take the whole search down.
func fileNameOf(f *common.OrgFile) string {
	if f == nil {
		return ""
	}
	return f.Filename
}

// ---------------------------------------------------------------------------
// The date questions the query language asks
// ---------------------------------------------------------------------------

// sdcDate is one of a heading's planning dates, or nil.
func sdcDate(sec *org.Section, which string) *org.OrgDate {
	if sec == nil || sec.Headline == nil {
		return nil
	}
	var sdc *org.SDC
	switch which {
	case "SCHEDULED":
		sdc = sec.Headline.Scheduled
	case "DEADLINE":
		sdc = sec.Headline.Deadline
	case "CLOSED":
		sdc = sec.Headline.Closed
	}
	if sdc == nil {
		return nil
	}
	return sdc.Date
}

// datePast is "before today", counted in days rather than instants: a deadline
// of today is due and not overdue, and a check that said otherwise would be
// wrong from one minute past midnight.
func datePast(d *org.OrgDate) bool {
	if d == nil || d.Start.IsZero() {
		return false
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := d.Start
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0,
		start.Location()).Before(today)
}

// daysOld is how long ago the most recent of a heading's dates was, in whole
// days. A heading with no date answers -1 rather than zero, so "older than
// thirty days" is never true of a heading that has no date to be old.
func daysOld(sec *org.Section) int {
	if sec == nil || sec.Headline == nil {
		return -1
	}
	var newest time.Time
	for _, sdc := range []*org.SDC{sec.Headline.Scheduled, sec.Headline.Deadline,
		sec.Headline.Closed} {
		if sdc == nil || sdc.Date == nil || sdc.Date.Start.IsZero() {
			continue
		}
		if sdc.Date.Start.After(newest) {
			newest = sdc.Date.Start
		}
	}
	if sec.Headline.Timestamp != nil && sec.Headline.Timestamp.Time != nil &&
		sec.Headline.Timestamp.Time.Start.After(newest) {
		newest = sec.Headline.Timestamp.Time.Start
	}
	if newest.IsZero() {
		return -1
	}
	return int(time.Since(newest).Hours() / 24)
}

// toFloat reads a number out of whatever the expression evaluator handed over -
// it produces float64 for a literal and can produce an int from a function.
func toFloat(v interface{}) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(n, "%g", &f); err == nil {
			return f, nil
		}
	}
	return 0, fmt.Errorf("expected a number, got %T", v)
}
