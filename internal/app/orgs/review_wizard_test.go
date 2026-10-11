package orgs

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func reviewLines(s string) []string { return strings.Split(strings.TrimSuffix(s, "\n"), "\n") }

// The body a triage card shows is the prose: the planning line and the drawers
// are not what anybody reads to decide where a thing goes, and org's
// indentation is not part of what was written.
func TestReviewBodyPreviewIsTheProse(t *testing.T) {
	lines := reviewLines(`* TODO Call the plumber
  SCHEDULED: <2026-10-12 Mon>
  :PROPERTIES:
  :CREATED: [2026-10-01 Thu]
  :END:
  :LOGBOOK:
  - State "TODO" from [2026-10-01 Thu 09:00]
  :END:

  The tap in the kitchen drips.
    - ask about the boiler

** A child
  not mine`)
	got := bodyPreview(lines, 0, ownEnd(lines, 0))
	want := "The tap in the kitchen drips.\n  - ask about the boiler"
	if got != want {
		t.Errorf("body:\n%q\nwant\n%q", got, want)
	}
}

// How long something has waited comes from org's own state line for the
// keyword it is on now - the newest, since it may have waited twice - and a
// chase restarts nothing but is reported. The child's logbook is not the
// parent's.
func TestReviewWaitingSinceReadsTheLogbook(t *testing.T) {
	lines := reviewLines(`** WAITING Quote from the builder
   :LOGBOOK:
   - Note taken on [2026-10-03 Sat 10:00] \\
     Chased: rang the office
   - State "WAITING"    from "NEXT"       [2026-09-28 Mon 09:15]
   - State "NEXT"       from "WAITING"    [2026-09-20 Sun 09:15]
   - State "WAITING"    from "TODO"       [2026-09-10 Thu 11:00]
   :END:
*** WAITING child
    - State "WAITING"    from "TODO"       [2026-10-09 Fri 11:00]`)
	end := ownEnd(lines, 0)
	since, how := waitingSince(lines, 0, end, "WAITING", nil)
	if how != "logbook" || since.Format("2006-01-02 15:04") != "2026-09-28 09:15" {
		t.Errorf("since %v (%s), want 2026-09-28 09:15 from the logbook", since, how)
	}
	if c := lastChased(lines, 0, end); c.Format("2006-01-02") != "2026-10-03" {
		t.Errorf("chased %v, want 2026-10-03", c)
	}
	if a := lastActivity(lines, 0, end); a.Format("2006-01-02") != "2026-10-03" {
		t.Errorf("last activity %v, want 2026-10-03", a)
	}

	// No state line: the CREATED property is the next best thing.
	bare := reviewLines("* WAITING Parcel\n")
	since, how = waitingSince(bare, 0, 0, "WAITING", map[string]string{"CREATED": "[2026-10-01 Thu 08:00]"})
	if how != "created" || since.Format("2006-01-02") != "2026-10-01" {
		t.Errorf("since %v (%s), want 2026-10-01 from created", since, how)
	}
	if since, how = waitingSince(bare, 0, 0, "WAITING", nil); !since.IsZero() || how != "" {
		t.Errorf("nothing to go on should say so, got %v %q", since, how)
	}
}

func TestReviewDaysAreCalendarDays(t *testing.T) {
	late := time.Date(2026, 10, 9, 23, 0, 0, 0, time.Local)
	morning := time.Date(2026, 10, 10, 7, 0, 0, 0, time.Local)
	if d := reviewDaysBetween(late, morning); d != 1 {
		t.Errorf("23:00 yesterday to 07:00 today is %d days, want 1", d)
	}
}

const reviewInbox = `#+TODO: TODO NEXT WAITING SOMEDAY | DONE CANCELLED
* TODO Buy a ladder
* Plan the party
** TODO Book the hall
** TODO Invite people
* DONE Already handled
`

// Only the outermost heading a query finds is an item: the party's tasks are
// part of the party, and go wherever it goes.
func TestReviewInboxIsOutermostHeadings(t *testing.T) {
	paths := loadOrg(t, map[string]string{"inbox.org": reviewInbox, "projects.org": "* Home\n"})
	_ = paths
	list := reviewList(common.DefaultInboxQuery)
	if !list.Ok {
		t.Fatalf("inbox: %s", list.Msg)
	}
	got := []string{}
	for _, it := range list.Items {
		got = append(got, it.Headline)
	}
	if strings.Join(got, "|") != "Buy a ladder|Plan the party" {
		t.Fatalf("inbox items %q", got)
	}
	if list.Items[1].Children != 2 {
		t.Errorf("the party has %d children, want 2", list.Items[1].Children)
	}
}

func postReview(t *testing.T, path string, body interface{}) common.ReviewActResult {
	t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", path, strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	if path == "/review/undo" {
		PostReviewUndo(w, r)
	} else {
		PostReviewAct(w, r)
	}
	var res common.ReviewActResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("%s answered %q", path, w.Body.String())
	}
	return res
}

// A refile out of the inbox is undone exactly - both files as they were -
// and an undo is refused once the file has been changed by something else,
// rather than throwing that change away.
func TestReviewActRefileAndUndo(t *testing.T) {
	paths := loadOrg(t, map[string]string{"inbox.org": reviewInbox, "projects.org": "* Home\n** TODO Paint the fence\n"})
	inbox, projects := paths["inbox.org"], paths["projects.org"]
	before, beforeP := readOrg(t, inbox), readOrg(t, projects)

	list := reviewList(common.DefaultInboxQuery)
	ladder := list.Items[0]
	res := postReview(t, "/review/act", common.ReviewAct{Hash: ladder.Hash, Action: "refile",
		To: &common.Target{Type: "file+olp", Filename: projects, Id: "Home"}})
	if !res.Ok || res.Undo == "" {
		t.Fatalf("refile: %+v", res)
	}
	if strings.Contains(readOrg(t, inbox), "ladder") || !strings.Contains(readOrg(t, projects), "** TODO Buy a ladder") {
		t.Fatalf("refile did not move it:\n%s\n---\n%s", readOrg(t, inbox), readOrg(t, projects))
	}

	undo := postReview(t, "/review/undo", common.ReviewUndo{Token: res.Undo})
	if !undo.Ok {
		t.Fatalf("undo: %+v", undo)
	}
	wantOrg(t, "inbox after undo", readOrg(t, inbox), before)
	wantOrg(t, "projects after undo", readOrg(t, projects), beforeP)

	// Again, and then somebody edits the destination: the undo must refuse.
	list = reviewList(common.DefaultInboxQuery)
	res = postReview(t, "/review/act", common.ReviewAct{Hash: list.Items[0].Hash, Action: "refile",
		To: &common.Target{Type: "file+olp", Filename: projects, Id: "Home"}})
	if !res.Ok {
		t.Fatalf("second refile: %+v", res)
	}
	writeLines(projects, append(reviewLines(readOrg(t, projects)), "* Added by hand"))
	if undo := postReview(t, "/review/undo", common.ReviewUndo{Token: res.Undo}); undo.Ok {
		t.Fatalf("undo over a later edit should be refused")
	}
	if !strings.Contains(readOrg(t, projects), "Added by hand") {
		t.Fatalf("the later edit was thrown away")
	}
}

// Tags, a date and a chase are line edits of the heading's own lines; a
// promote takes the someday tag off as well as setting the keyword.
func TestReviewActLineEdits(t *testing.T) {
	paths := loadOrg(t, map[string]string{"inbox.org": "#+TODO: TODO NEXT WAITING SOMEDAY | DONE\n* SOMEDAY Learn the cello :someday:music:\n* WAITING Parcel\n  Body text\n"})
	p := paths["inbox.org"]
	cello := reviewList(`MatchHeadline("cello")`).Items[0]
	if res := postReview(t, "/review/act", common.ReviewAct{Hash: cello.Hash, Action: "promote", Keyword: "NEXT"}); !res.Ok {
		t.Fatalf("promote: %+v", res)
	}
	if got := reviewLines(readOrg(t, p))[1]; got != "* NEXT Learn the cello :music:" {
		t.Errorf("promoted line %q", got)
	}
	parcel := reviewList(`MatchHeadline("Parcel")`).Items[0]
	if res := postReview(t, "/review/act", common.ReviewAct{Hash: parcel.Hash, Action: "chase", Text: "emailed"}); !res.Ok {
		t.Fatalf("chase: %+v", res)
	}
	text := readOrg(t, p)
	if !strings.Contains(text, "Chased: emailed") || !strings.Contains(text, "Body text") {
		t.Errorf("chase wrote:\n%s", text)
	}
	lines := reviewLines(text)
	if c := lastChased(lines, 2, ownEnd(lines, 2)); c.IsZero() {
		t.Errorf("the chase it wrote is not read back as a chase:\n%s", text)
	}
}
