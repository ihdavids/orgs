package orgs

import (
	"errors"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// A heading as runWatchQuery would describe it.
func wm(file string, line int, title string, props map[string]string, olp ...string) common.WatchMatch {
	if len(olp) == 0 {
		olp = []string{title}
	}
	return common.WatchMatch{Key: watchKey(props, file, olp, "h"+title), Headline: title,
		File: file, Line: line, Path: olp}
}

// An engine whose query answers whatever the test last put in `answer`.
func testEngine(t *testing.T) (*watchEngine, *[]common.WatchMatch, *error, *[]common.WatchNotice) {
	e := newWatchEngine()
	clock := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	e.now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	var answer []common.WatchMatch
	var fail error
	var delivered []common.WatchNotice
	e.run = func(string) ([]common.WatchMatch, error) { return answer, fail }
	e.deliver = func(_ string, _ common.QueryWatch, n common.WatchNotice) { delivered = append(delivered, n) }
	return e, &answer, &fail, &delivered
}

var blocked = common.QueryWatch{Name: "blocked"}

func TestWatchFirstCheckIsABaseline(t *testing.T) {
	e, answer, _, delivered := testEngine(t)
	*answer = []common.WatchMatch{wm("a.org", 1, "One", nil), wm("a.org", 5, "Two", nil)}
	if n, _ := e.check("me", blocked, "q"); n != nil {
		t.Fatalf("the first check announced %v", n)
	}
	if c, _, _ := e.status("me", "blocked"); c != 2 {
		t.Fatalf("baseline count %d, want 2", c)
	}
	// Nothing changed: nothing said.
	if n, _ := e.check("me", blocked, "q"); n != nil {
		t.Fatalf("an unchanged check announced %v", n)
	}
	*answer = append(*answer, wm("b.org", 3, "Three", nil))
	n, _ := e.check("me", blocked, "q")
	if n == nil || len(n.Gained) != 1 || n.Gained[0].Headline != "Three" || n.Count != 3 {
		t.Fatalf("gained %+v", n)
	}
	if len(*delivered) != 1 {
		t.Fatalf("delivered %d notices, want 1", len(*delivered))
	}
	if l := e.list("me", true, 0); l.Unread != 1 || len(l.Notices) != 1 {
		t.Fatalf("unread %+v", l)
	}
}

func TestWatchNewQueryTextIsANewBaseline(t *testing.T) {
	e, answer, _, _ := testEngine(t)
	*answer = []common.WatchMatch{wm("a.org", 1, "One", nil)}
	e.check("me", blocked, "IsStatus(\"BLOCKED\")")
	*answer = []common.WatchMatch{wm("a.org", 1, "One", nil), wm("a.org", 9, "Nine", nil)}
	if n, _ := e.check("me", blocked, "IsStatus(\"WAITING\")"); n != nil {
		t.Fatalf("editing the saved query announced %v", n)
	}
	// Forgetting (removing the watch, pausing it) is a new baseline too.
	e.forget("me", "blocked")
	*answer = append(*answer, wm("c.org", 2, "Later", nil))
	if n, _ := e.check("me", blocked, "IsStatus(\"WAITING\")"); n != nil {
		t.Fatalf("a re-added watch announced %v", n)
	}
}

func TestWatchErrorKeepsTheBaseline(t *testing.T) {
	e, answer, fail, delivered := testEngine(t)
	*answer = []common.WatchMatch{wm("a.org", 1, "One", nil)}
	e.check("me", blocked, "q")
	*fail = errors.New("does not parse")
	if _, err := e.check("me", blocked, "q"); err == nil {
		t.Fatal("no error")
	}
	if _, _, why := e.status("me", "blocked"); why == "" {
		t.Fatal("the error was not kept on the watch")
	}
	*fail = nil
	*answer = append(*answer, wm("a.org", 4, "Four", nil))
	n, _ := e.check("me", blocked, "q")
	if n == nil || len(n.Gained) != 1 || n.Gained[0].Headline != "Four" {
		t.Fatalf("after the error, gained %+v (delivered %d)", n, len(*delivered))
	}
	if _, _, why := e.status("me", "blocked"); why != "" {
		t.Fatalf("error not cleared: %s", why)
	}
}

func TestWatchLeftOnlyWhenAskedFor(t *testing.T) {
	e, answer, _, _ := testEngine(t)
	*answer = []common.WatchMatch{wm("a.org", 1, "One", nil), wm("b.org", 1, "Other", nil)}
	e.check("me", blocked, "q")
	*answer = []common.WatchMatch{wm("a.org", 1, "One", nil)}
	if n, _ := e.check("me", blocked, "q"); n != nil {
		t.Fatalf("a departure was announced to a watch not asking: %+v", n)
	}
	both := common.QueryWatch{Name: "both", Left: true}
	*answer = []common.WatchMatch{wm("a.org", 1, "One", nil), wm("b.org", 1, "Other", nil)}
	e.check("me", both, "q")
	*answer = []common.WatchMatch{wm("a.org", 1, "One", nil)}
	n, _ := e.check("me", both, "q")
	if n == nil || len(n.Left) != 1 || n.Left[0].Headline != "Other" || len(n.Gained) != 0 {
		t.Fatalf("left %+v", n)
	}
}

func TestWatchIdentitySurvivesEdits(t *testing.T) {
	e, answer, _, _ := testEngine(t)
	id := map[string]string{"ID": "abc-123"}
	*answer = []common.WatchMatch{
		wm("a.org", 10, "Has an id", id, "Project", "Has an id"),
		wm("a.org", 20, "Plain", nil, "Project", "Plain"),
	}
	e.check("me", blocked, "q")

	// Lines added above both (the hash of each changes), the one with an id
	// renamed and moved to another file, the plain one's parent renamed.
	*answer = []common.WatchMatch{
		wm("b.org", 3, "Renamed, with an id", id, "Elsewhere", "Renamed, with an id"),
		wm("a.org", 24, "Plain", nil, "Project (renamed)", "Plain"),
	}
	if n, _ := e.check("me", blocked, "q"); n != nil {
		t.Fatalf("edited headings were announced as new: %+v", n)
	}

	// A plain heading renamed in place: same file, same line.
	*answer = []common.WatchMatch{
		wm("b.org", 3, "Renamed, with an id", id, "Elsewhere", "Renamed, with an id"),
		wm("a.org", 24, "Plain, retitled", nil, "Project (renamed)", "Plain, retitled"),
	}
	if n, _ := e.check("me", blocked, "q"); n != nil {
		t.Fatalf("a retitled heading was announced as new: %+v", n)
	}

	// A genuinely different heading arriving while another leaves, elsewhere.
	both := common.QueryWatch{Name: "blocked", Left: true}
	*answer = []common.WatchMatch{
		wm("b.org", 3, "Renamed, with an id", id, "Elsewhere", "Renamed, with an id"),
		wm("c.org", 7, "Brand new", nil),
	}
	n, _ := e.check("me", both, "q")
	if n == nil || len(n.Gained) != 1 || n.Gained[0].Headline != "Brand new" ||
		len(n.Left) != 1 || n.Left[0].Headline != "Plain, retitled" {
		t.Fatalf("got %+v", n)
	}
}

func TestWatchTwoIdsAreTwoHeadings(t *testing.T) {
	before := matchSet([]common.WatchMatch{wm("a.org", 5, "Same", map[string]string{"ID": "1"})})
	after := matchSet([]common.WatchMatch{wm("a.org", 5, "Same", map[string]string{"ID": "2"})})
	gained, left := diffMatches(before, after)
	if len(gained) != 1 || len(left) != 1 {
		t.Fatalf("two ids at one line paired: gained %v left %v", gained, left)
	}
}

func TestWatchDuplicateTitlesAreKeptApart(t *testing.T) {
	set := matchSet([]common.WatchMatch{
		wm("a.org", 1, "Call", nil, "Inbox", "Call"),
		wm("a.org", 2, "Call", nil, "Inbox", "Call"),
	})
	if len(set) != 2 {
		t.Fatalf("two headings with one outline path became %d", len(set))
	}
	if watchKey(map[string]string{"custom_id": "x"}, "f", []string{"t"}, "h") != "cid:x" {
		t.Fatal("CUSTOM_ID is matched case-insensitively")
	}
	if watchKey(nil, "f", nil, "h") != "hash:h" {
		t.Fatal("no path falls back to the hash")
	}
}

func TestWatchNoticesReadClearAndCap(t *testing.T) {
	e, answer, _, _ := testEngine(t)
	e.check("me", blocked, "q")
	for i := 0; i < watchNoticeCap+5; i++ {
		*answer = append(*answer, wm("a.org", i+1, "H", nil, "H", string(rune('A'+i%26)), string(rune('a'+i/26))))
		if n, _ := e.check("me", blocked, "q"); n == nil {
			t.Fatalf("check %d said nothing", i)
		}
	}
	l := e.list("me", false, 0)
	if l.Total != watchNoticeCap || l.Unread != watchNoticeCap {
		t.Fatalf("kept %d (%d unread), want %d", l.Total, l.Unread, watchNoticeCap)
	}
	newest := l.Notices[0].Id
	e.markRead("me", common.NoticeAct{Ids: []string{newest}})
	if e.list("me", false, 0).Unread != watchNoticeCap-1 {
		t.Fatal("marking one read")
	}
	if got := e.list("me", true, 3); len(got.Notices) != 3 || got.Notices[0].Id == newest {
		t.Fatalf("unread with a limit: %d, first %s", len(got.Notices), got.Notices[0].Id)
	}
	e.markRead("me", common.NoticeAct{All: true})
	if e.list("me", false, 0).Unread != 0 {
		t.Fatal("marking all read")
	}
	e.clear("me", common.NoticeAct{Ids: []string{newest}})
	if e.list("me", false, 0).Total != watchNoticeCap-1 {
		t.Fatal("clearing one")
	}
	e.clear("me", common.NoticeAct{All: true})
	if e.list("me", false, 0).Total != 0 {
		t.Fatal("clearing all")
	}
	// Another user's list is theirs.
	if e.list("you", false, 0).Total != 0 {
		t.Fatal("notices leaked between users")
	}
}

func TestDateWithin(t *testing.T) {
	now := time.Date(2026, 10, 12, 23, 30, 0, 0, time.Local) // a Monday, late
	at := func(y int, m time.Month, d int) *org.OrgDate {
		return &org.OrgDate{Start: time.Date(y, m, d, 9, 0, 0, 0, time.Local)}
	}
	cases := []struct {
		d    *org.OrgDate
		n    float64
		want bool
	}{
		{at(2026, 10, 12), 0, true},   // today, at an hour already gone: still due
		{at(2026, 10, 14), 2, true},   // Wednesday is within two days of Monday
		{at(2026, 10, 15), 2, false},  // Thursday is not
		{at(2026, 10, 11), 2, false},  // yesterday is past, not within
		{nil, 2, false},
	}
	for i, c := range cases {
		if got := dateWithin(c.d, c.n, now); got != c.want {
			t.Errorf("case %d: got %v, want %v", i, got, c.want)
		}
	}
}
