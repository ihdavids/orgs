package orgs

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/go-org/org"
)

// Friday 2026-10-30, the day every translation below is read on.
var shortNow = time.Date(2026, 10, 30, 9, 0, 0, 0, time.Local)

func TestShortQueryTranslation(t *testing.T) {
	cases := []struct{ in, want string }{
		// The proposal's table.
		{`todo`, `!IsArchived() && IsTodo()`},
		{`NEXT`, `!IsArchived() && IsStatus("NEXT")`},
		{`WAITING|BLOCKED`, `!IsArchived() && (IsStatus("WAITING") || IsStatus("BLOCKED"))`},
		{`project`, `!IsArchived() && IsProject()`},
		{`stuck`, `!IsArchived() && IsBlockedProject()`},
		{`task :work:urgent:`, `!IsArchived() && IsTask() && HasTags("work", "urgent")`},
		{`todo -:home:`, `!IsArchived() && IsTodo() && !HasTags("home")`},
		{`#A due:<3d`, `!IsArchived() && IsPriority("A") && DeadlineWithin(3)`},
		{`todo due:past`, `!IsArchived() && IsTodo() && DeadlinePast()`},
		{`on:today`, `!IsArchived() && OnDate("2026 30 10")`},
		{`on:fri`, `!IsArchived() && OnDate("2026 30 10")`},
		{`on:2026-11-02`, `!IsArchived() && OnDate("2026 02 11")`},
		{`client:acme`, `!IsArchived() && MatchProperty("CLIENT", "(?i)acme")`},
		{`file:work report`, `!IsArchived() && InFile("(?i)work") && MatchHeadline("(?i)report")`},
		{`habit streak:>7`, `!IsArchived() && IsHabit() && (HabitStreak() > 7)`},
		{`backlinks:0`, `!IsArchived() && (BacklinkCount() == 0)`},
		{`:/^M[0-9]+$/:`, `!IsArchived() && HasTagMatching("^M[0-9]+$")`},

		// before: and after:
		{`before:today`, `!IsArchived() && DateBefore("2026 30 10")`},
		{`after:fri`, `!IsArchived() && DateAfter("2026 30 10")`},
		{`after:mon`, `!IsArchived() && DateAfter("2026 02 11")`},
		{`before:2026-12-25`, `!IsArchived() && DateBefore("2026 25 12")`},
		{`before:-1w`, `!IsArchived() && DateBefore("2026 23 10")`},
		{`todo after:today before:+2w`, `!IsArchived() && IsTodo() && DateAfter("2026 30 10") && DateBefore("2026 13 11")`},
		{`-before:today`, `!IsArchived() && !DateBefore("2026 30 10")`},

		// Operators.
		{`todo or done`, `!IsArchived() && (IsTodo() || IsDone())`},
		{`todo | done`, `!IsArchived() && (IsTodo() || IsDone())`},
		{`todo and :work:`, `!IsArchived() && IsTodo() && HasTags("work")`},
		{`not project`, `!IsArchived() && !IsProject()`},
		{`!project`, `!IsArchived() && !IsProject()`},
		{`-(NEXT | WAITING)`, `!IsArchived() && !(IsStatus("NEXT") || IsStatus("WAITING"))`},
		{`(NEXT :work:) | (WAITING :home:)`, `!IsArchived() && (IsStatus("NEXT") && HasTags("work") || IsStatus("WAITING") && HasTags("home"))`},
		{`--todo`, `!IsArchived() && IsTodo()`},
		{`-tagged`, `!IsArchived() && NoTags()`},

		// archived turns the default off, whichever way it is asked.
		{`archived`, `IsArchived()`},
		{`-archived :work:`, `!IsArchived() && HasTags("work")`},
		{`todo | archived`, `IsTodo() || IsArchived()`},

		// Text.
		{`report`, `!IsArchived() && MatchHeadline("(?i)report")`},
		{`"weekly review"`, `!IsArchived() && MatchHeadline("(?i)weekly review")`},
		{`"project"`, `!IsArchived() && MatchHeadline("(?i)project")`},
		{`/^Call (Sam|Jo)/`, `!IsArchived() && MatchHeadline("^Call (Sam|Jo)")`},
		{`/a\d+/`, `!IsArchived() && MatchHeadline("a\\d+")`},
		{`c++`, `!IsArchived() && MatchHeadline("(?i)c\\+\\+")`},
		{`text:"needs review"`, `!IsArchived() && MatchContent("(?i)needs review")`},
		{`title:/^Q[1-4]/`, `!IsArchived() && MatchHeadline("^Q[1-4]")`},
		{`link:jira`, `!IsArchived() && LinksTo("(?i)jira")`},
		{`file:plan.org`, `!IsArchived() && InFile("(?i)plan\\.org")`},
		{`"say \"hi\""`, `!IsArchived() && MatchHeadline("(?i)say \"hi\"")`},

		// Keys.
		{`client:*`, `!IsArchived() && HasProperty("CLIENT")`},
		{`effort:`, `!IsArchived() && HasProperty("EFFORT")`},
		{`assigned:/^sam/`, `!IsArchived() && MatchProperty("ASSIGNED", "^sam")`},
		{`status:next`, `!IsArchived() && IsStatus("NEXT")`},
		{`status:`, `!IsArchived() && HasAStatus()`},
		{`priority:b`, `!IsArchived() && IsPriority("B")`},
		{`#c`, `!IsArchived() && IsPriority("C")`},
		{`tag:work`, `!IsArchived() && HasTags("work")`},
		{`group:WORK`, `!IsArchived() && InTagGroup("WORK")`},
		{`in:contact`, `!IsArchived() && InCollection("contact")`},
		{`record:contact`, `!IsArchived() && IsRecord("contact")`},
		{`record`, `!IsArchived() && IsRecord()`},
		{`left:>0`, `!IsArchived() && (ChecklistLeft() > 0)`},
		{`items:>=5`, `!IsArchived() && (ChecklistCount() >= 5)`},
		{`age:<3`, `!IsArchived() && (DaysOld() < 3)`},
		{`old:30`, `!IsArchived() && OlderThan(30)`},
		{`old:>30`, `!IsArchived() && OlderThan(31)`},
		{`old:<7`, `!IsArchived() && (DaysOld() < 7 && DaysOld() >= 0)`},
		{`orphan`, `!IsArchived() && (BacklinkCount() == 0)`},
		{`-orphan`, `!IsArchived() && !(BacklinkCount() == 0)`},

		// Dates by key.
		{`due:today`, `!IsArchived() && DeadlineWithin(0)`},
		{`due:3`, `!IsArchived() && DeadlineWithin(3)`},
		{`due:2w`, `!IsArchived() && DeadlineWithin(14)`},
		{`due:mon`, `!IsArchived() && DeadlineWithin(3)`},
		{`due:any`, `!IsArchived() && HasDeadline()`},
		{`due:none`, `!IsArchived() && !HasDeadline()`},
		{`sched:past`, `!IsArchived() && ScheduledPast()`},
		{`sched:<=1w`, `!IsArchived() && ScheduledWithin(7)`},

		// Filters stay one bracketed term, expanded afterwards.
		{`{{ WorkTasks }} :urgent:`, `!IsArchived() && ({{ WorkTasks }}) && HasTags("urgent")`},
	}
	for _, c := range cases {
		got, err := translateShort(c.in, shortNow)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q\n got  %s\n want %s", c.in, got, c.want)
		}
		// What comes out is the full form, so translating it again changes nothing.
		if isShortQuery(got) {
			t.Errorf("%q: the translation %s reads as a short query", c.in, got)
		}
	}
}

// Every query written before the short form keeps its meaning: nothing in the
// full form is taken for a short query.
func TestShortQueryDetection(t *testing.T) {
	full := []string{
		`IsTodo()`,
		`!IsArchived() && IsTodo()`,
		`IsTask() && HasTags("work")`,
		`IsProject ()`,
		`{{ AllTasks }}`,
		`{{ AllTasks }} && {{ HomeTasks }}`,
		`true`,
		`false`,
		``,
		`   `,
	}
	for _, q := range full {
		if isShortQuery(q) {
			t.Errorf("%q read as a short query", q)
		}
		if got, err := shortToFull(q); err != nil || got != q {
			t.Errorf("%q came back as %q, %v", q, got, err)
		}
	}
	short := []string{
		`todo`,
		`NEXT :work:`,
		`"Call(me)"`, // brackets in a phrase are text
		`{{ WorkTasks }} :urgent:`,
		`due:<3d`,
		`before:today`,
	}
	for _, q := range short {
		if !isShortQuery(q) {
			t.Errorf("%q read as the full form", q)
		}
	}
}

// A mistake says where it is.
func TestShortQueryErrors(t *testing.T) {
	cases := []struct{ in, want string }{
		{`todo (`, `col 7: the query ends`},
		{`(todo`, `col 1: ( with no )`},
		{`todo )`, `col 6: unexpected ")"`},
		{`todo | `, `col 7: the query ends`},
		{`| todo`, `col 1: nothing before "|"`},
		{`"open`, `col 1: " with no closing "`},
		{`/[a/`, `error parsing regexp`},
		{`on:someday`, `col 1: on: wants a day`},
		{`before:blue`, `col 1: before: wants a day`},
		{`todo after:x`, `col 6: after: wants a day`},
		{`due:2026-01-01`, `in the past - try due:past`},
		{`streak:many`, `streak: wants a number`},
		{`::`, `names no tag`},
		{`{{ Oops`, `col 1: {{ with no }}`},
	}
	for _, c := range cases {
		_, err := translateShort(c.in, shortNow)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want %q", c.in, err, c.want)
		}
	}
}

// The translations are queries govaluate takes, against the real function map.
func TestShortQueriesParse(t *testing.T) {
	for _, q := range []string{
		`todo :work: due:<3d -:home:`,
		`project stuck | WAITING old:>14`,
		`#A before:fri after:-2w`,
		`/a\d+/ client:"acme \"co\"" assigned:/^sam/`,
		`habit streak:>7 orphan -tagged`,
	} {
		if _, err := ParseString(&common.StringQuery{Query: q}); err != nil {
			t.Errorf("%q: %v", q, err)
		}
	}
	if _, err := ParseString(&common.StringQuery{Query: `todo (`}); err == nil {
		t.Errorf("a broken short query parsed")
	}
}

// End to end: short queries over a loaded file find what they say.
func TestShortQueriesFind(t *testing.T) {
	day := func(n int) string {
		return time.Now().AddDate(0, 0, n).Format("2006-01-02 Mon")
	}
	text := fmt.Sprintf(`* TODO Write the report :work:
SCHEDULED: <%s>
* TODO Fix the sink :home:
DEADLINE: <%s>
* DONE Ship release :work:
CLOSED: [%s]
* TODO Plan the offsite :work:urgent:
:PROPERTIES:
:CLIENT: Acme Corp
:END:
<%s>
* NEXT [#A] Call Sam
* Old thing :ARCHIVE:
** TODO Archived task
SCHEDULED: <%s>
* Reading notes
`, day(-3), day(2), day(-1), day(10), day(-5))

	if Conf().Server != nil {
		was := Conf().Server.DefaultTodoStates
		Conf().Server.DefaultTodoStates = "TODO NEXT | DONE CANCELLED"
		t.Cleanup(func() { Conf().Server.DefaultTodoStates = was })
	}
	loadOrg(t, map[string]string{"short.org": text})

	find := func(q string) []string {
		t.Helper()
		var got []string
		err := evalEachSection(q, func(s *org.Section, f *common.OrgFile) {
			got = append(got, strings.TrimSpace(s.Headline.Title[0].String()))
		})
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		sort.Strings(got)
		return got
	}
	cases := []struct {
		q    string
		want []string
	}{
		{`:work:`, []string{"Plan the offsite", "Ship release", "Write the report"}},
		{`:work: -done`, []string{"Plan the offsite", "Write the report"}},
		{`:work:urgent:`, []string{"Plan the offsite"}},
		{`client:acme`, []string{"Plan the offsite"}},
		{`client:*`, []string{"Plan the offsite"}},
		{`#A`, []string{"Call Sam"}},
		{`sink | report`, []string{"Fix the sink", "Write the report"}},
		{`due:<3d`, []string{"Fix the sink"}},
		{`sched:past`, []string{"Write the report"}},
		{`before:today`, []string{"Write the report"}},
		{`after:today`, []string{"Fix the sink", "Plan the offsite"}},
		{`after:+1w`, []string{"Plan the offsite"}},
		{`after:today before:+1w`, []string{"Fix the sink"}},
		{`-dated "the"`, nil},
		// The archived tree is left out unless asked for.
		{`"archived task"`, nil},
		{`archived "archived task"`, []string{"Archived task"}},
		{`archived before:today`, []string{"Archived task"}},
		{`-scheduled -deadline -dated untagged`, []string{"Call Sam", "Reading notes"}},
	}
	for _, c := range cases {
		got := find(c.q)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%q\n got  %v\n want %v", c.q, got, c.want)
		}
	}
}
