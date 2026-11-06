package common

import (
	"strings"
	"testing"
	"time"
)

func TestOrgQlToQuery(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	cases := []struct{ in, want string }{
		{`(todo)`, `IsTodo()`},
		{`(todo "NEXT" "WAITING")`, `(IsStatus("NEXT") || IsStatus("WAITING"))`},
		{`(and (todo "NEXT") (tags "work") (not (done)))`, `(IsStatus("NEXT") && HasTags("work") && !(IsDone()))`},
		{`(or (tags-all "a" "b") (habit))`, `((HasTags("a") && HasTags("b")) || IsHabit())`},
		{`(priority >= "B")`, `PropCompare("PRIORITY", "<=", "B")`},
		{`(priority "A" "b")`, `(IsPriority("A") || IsPriority("B"))`},
		{`(deadline :to today)`, `PropCompare("DEADLINE", "<=", "<today>")`},
		{`(scheduled :from -7 :to 7)`, `(PropCompare("SCHEDULED", ">=", "<-7d>") && PropCompare("SCHEDULED", "<=", "<+7d>"))`},
		{`(closed :on "2026-11-01")`, `PropCompare("CLOSED", "=", "<2026-11-01>")`},
		{`(closed)`, `PropCompare("CLOSED", ">", "<1900-01-01>")`},
		{`(deadline auto)`, `(DeadlinePast() || DeadlineWithin(14))`},
		{`(level 2 3)`, `(PropCompare("LEVEL", ">=", "2") && PropCompare("LEVEL", "<=", "3"))`},
		{`(level < 3)`, `PropCompare("LEVEL", "<", "3")`},
		{`(property "CLIENT" "acme.co")`, `MatchProperty("CLIENT", "^acme\\.co$")`},
		{`(property "CLIENT" :inherit t)`, `InheritedProperty("CLIENT")`},
		{`(heading "report")`, `MatchHeadline("(?i)report")`},
		{`(regexp "Ann|Bob")`, `MatchContent("Ann|Bob")`},
		{`(ts :from -1 :to today)`, `(HasAnyDate() && !DateBefore("2026 04 11") && !DateAfter("2026 05 11"))`},
		{`"garden plans"`, `MatchContent("(?i)garden plans")`},
		// org-ql's plain syntax.
		{`todo:NEXT,WAITING tags:work`, `(IsStatus("NEXT") || IsStatus("WAITING")) && HasTags("work")`},
		{`todo: !tags:someday deadline:to=today`, `IsTodo() && !(HasTags("someday")) && PropCompare("DEADLINE", "<=", "<today>")`},
		{`garden "raised beds" property:CLIENT=acme`, `MatchContent("(?i)garden") && MatchContent("(?i)raised beds") && MatchProperty("CLIENT", "^acme$")`},
		{`priority:A done:`, `IsPriority("A") && IsDone()`},
	}
	for _, c := range cases {
		got, err := OrgQlToQuery(c.in, now)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		// The plain syntax's terms are one (and ...): compare without its
		// outer brackets, and only as far as the case spells out.
		g := strings.TrimSuffix(strings.TrimPrefix(got, "("), ")")
		if got != c.want && g != c.want {
			t.Errorf("%s\n got %s\nwant %s", c.in, got, c.want)
		}
	}
}

func TestOrgQlRefuses(t *testing.T) {
	for _, in := range []string{
		``, `(and (todo)`, `(todo))`, `(ancestors (todo "PROJECT"))`, `(clocked :from -7)`,
		`(level "two")`, `(deadline :to "next tuesday")`, `todo`,
	} {
		if q, err := OrgQlToQuery(in, time.Now()); err == nil && in != "todo" {
			t.Errorf("%q read as %s", in, q)
		}
	}
	// A plain word that happens to be a predicate's name is text to find.
	if q, err := OrgQlToQuery("todo", time.Now()); err != nil || !strings.Contains(q, "MatchContent") {
		t.Errorf("todo alone: %s %v", q, err)
	}
	// A url is text, not an unknown predicate.
	if q, err := OrgQlToQuery("https://example.com", time.Now()); err != nil || !strings.Contains(q, "MatchContent") {
		t.Errorf("url: %s %v", q, err)
	}
}
