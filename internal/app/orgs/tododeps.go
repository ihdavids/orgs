package orgs

// TODO dependencies, org's org-block-todo-from-children-or-siblings-or-parent:
// what stops a heading being marked done while enforceTodoDependencies (or
// enforceCheckboxDependencies) is on, and IsBlocked() in queries, which is the
// same rule whether or not it is enforced.
//
// Read off the parsed outline, which go-org now builds whole (see Traps:
// column-zero drawers): direct children, ancestors and their siblings.

import (
	"fmt"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func propTrue(h *org.Headline, name string) bool {
	if h == nil || h.Properties == nil {
		return false
	}
	v, ok := h.Properties.Get(name)
	v = strings.ToLower(strings.TrimSpace(v))
	return ok && v != "" && v != "nil"
}

// todoBlocker is why sec may not be finished, or "" when nothing is in the
// way: an open child heading; under an ORDERED parent, an open sibling above
// it (or above any of its ancestors, each under an ORDERED parent); and, with
// checkboxes, an unticked box in its own text.
func todoBlocker(f *common.OrgFile, sec *org.Section, checkboxes bool) string {
	if sec == nil || sec.Headline == nil || propTrue(sec.Headline, "NOBLOCKING") {
		return ""
	}
	_, active := keywordSets(f)
	for _, c := range sec.Children {
		if c.Headline != nil && active[c.Headline.Status] {
			return fmt.Sprintf("%q under it is still %s", strings.TrimSpace(headlineTitle(c)), c.Headline.Status)
		}
	}
	for s := sec; s.Parent != nil && s.Parent.Headline != nil; s = s.Parent {
		if !propTrue(s.Parent.Headline, "ORDERED") {
			continue
		}
		for _, sib := range s.Parent.Children {
			if sib == s {
				break
			}
			if sib.Headline != nil && active[sib.Headline.Status] {
				return fmt.Sprintf("%q comes first (%q is ORDERED) and is still %s",
					strings.TrimSpace(headlineTitle(sib)), strings.TrimSpace(headlineTitle(s.Parent)), sib.Headline.Status)
			}
		}
	}
	if checkboxes {
		if done, total := ChecklistCounts(sec, f); total > done {
			return fmt.Sprintf("%d of its %d checkboxes are not ticked", total-done, total)
		}
	}
	return ""
}

// statusBlocked is the refusal for changing sec to keyword to, under the
// server's settings, or "".
func statusBlocked(f *common.OrgFile, sec *org.Section, to string) string {
	s := Conf().Server
	if s == nil || (!s.EnforceTodoDependencies && !s.EnforceCheckboxDependencies) || sec == nil || sec.Headline == nil {
		return ""
	}
	done, _ := keywordSets(f)
	if !done[to] || done[sec.Headline.Status] {
		return ""
	}
	why := ""
	if s.EnforceTodoDependencies {
		why = todoBlocker(f, sec, s.EnforceCheckboxDependencies)
	} else if d, t := ChecklistCounts(sec, f); t > d {
		why = fmt.Sprintf("%d of its %d checkboxes are not ticked", t-d, t)
	}
	if why == "" {
		return ""
	}
	return fmt.Sprintf("%q cannot be %s yet: %s", strings.TrimSpace(headlineTitle(sec)), to, why)
}
