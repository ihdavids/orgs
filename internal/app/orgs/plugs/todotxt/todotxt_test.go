package todotxtexp

import (
	"sort"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/todotxt"
)

// A database of one file: enough for an exporter given a file name.
type oneFile struct {
	common.ODb
	doc *org.Document
}

func (d oneFile) FindByFile(string) *org.Document { return d.doc }

func parse(t *testing.T, text string) *org.Document {
	t.Helper()
	conf := org.New()
	conf.DefaultSettings["TODO"] = "TODO NEXT | DONE"
	doc := conf.Parse(strings.NewReader(text), "inbox.org")
	if doc.Error != nil {
		t.Fatal(doc.Error)
	}
	return doc
}

const sample = `#+TODO: TODO NEXT | DONE
* Not a task
* work
** TODO [#A] Write the [[https://x.org/r][report]] :@desk:urgent:
   DEADLINE: <2026-10-20 Tue .+1w>
   :PROPERTIES:
   :CREATED: [2026-10-01 Thu]
   :PROJECTS: +q4
   :ID: abc-123
   :NOTE: has spaces so stays home
   :EFFORT: 1:30
   :END:
** DONE [#B] Filed taxes
   CLOSED: [2026-10-12 Mon 09:00]
* NEXT Buy milk
  SCHEDULED: <2026-10-15 Thu>
`

func TestExport(t *testing.T) {
	e := &Exporter{}
	err, got := e.ExportToString(oneFile{doc: parse(t, sample)}, "inbox.org", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `(A) 2026-10-01 Write the report https://x.org/r +work +q4 @desk @urgent due:2026-10-20 rec:1w id:abc-123 effort:1:30
x 2026-10-12 Filed taxes +work pri:B
Buy milk t:2026-10-15
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// Out to todo.txt and back in, and the task is what it was.
func TestRoundTrip(t *testing.T) {
	e := &Exporter{}
	_, txt := e.ExportToString(oneFile{doc: parse(t, sample)}, "inbox.org", "", nil)
	back := todotxt.ToOrg(todotxt.Parse(txt), todotxt.OrgOptions{})
	_, again := e.ExportToString(oneFile{doc: parse(t, "#+TODO: TODO NEXT | DONE\n"+back)}, "inbox.org", "", nil)
	// NEXT comes back as TODO (todo.txt has open and done, nothing between),
	// which changes nothing on the todo.txt side; the importer puts tasks
	// with no project first, so the order is not kept.
	if lines(again) != lines(txt) {
		t.Errorf("round trip changed it\nfirst\n%s\nsecond\n%s\norg\n%s", txt, again, back)
	}
}

func lines(s string) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	sort.Strings(ls)
	return strings.Join(ls, "\n")
}
