package orgs

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func TestOrgQlBlock(t *testing.T) {
	day := func(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02 Mon") }
	text := strings.Join([]string{
		"#+TODO: TODO NEXT WAITING | DONE",
		"* Lists",
		"#+BEGIN: org-ql :query \"(and (todo) (tags \\\"work\\\"))\" :columns (todo (priority \"P\") heading deadline) :sort (deadline priority)",
		"#+END:",
		"#+BEGIN: org-ql :query \"todo:NEXT,WAITING !tags:home\" :columns (heading) :take -1 :sort heading",
		"#+END:",
		"#+BEGIN: org-ql :query \"(deadline :from -7 :to today)\" :columns (heading deadline)",
		"#+END:",
		"#+BEGIN: org-ql :query \"(ancestors (todo))\"",
		"#+END:",
		"* NEXT [#B] Write the report :work:",
		"DEADLINE: <" + day(3) + ">",
		"* TODO [#A] Call the bank :work:",
		"* WAITING Parts from the shop :home:",
		"* NEXT Book the room :work:",
		":PROPERTIES:",
		":ID: room-1",
		":END:",
		"DEADLINE: <" + day(-2) + ">",
		"* DONE Old thing :work:",
	}, "\n") + "\n"
	paths := loadOrg(t, map[string]string{"ql.org": text})
	p := paths["ql.org"]

	// Off: the blocks are left alone and the refresh says how to turn it on.
	withFeatures(t, func(f *common.FeatureSettings) { f.OrgQl = false })
	res, err := refreshDynBlocks(p, func(dynBlock) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Blocks) == 0 || res.Blocks[0].Ok || !strings.Contains(res.Blocks[0].Msg, "orgs features orgQl on") {
		t.Fatalf("off: %+v", res.Blocks)
	}
	if readOrg(t, p) != strings.TrimRight(text, "\n") {
		t.Fatalf("an off feature wrote the file:\n%s", readOrg(t, p))
	}

	withFeatures(t, func(f *common.FeatureSettings) { f.OrgQl = true })
	res, err = refreshDynBlocks(p, func(dynBlock) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	got := readOrg(t, p)
	if !res.Blocks[0].Ok || !res.Blocks[1].Ok || !res.Blocks[2].Ok {
		t.Fatalf("%+v\n%s", res.Blocks, got)
	}
	if res.Blocks[3].Ok || !strings.Contains(res.Blocks[3].Msg, "ancestors") {
		t.Fatalf("ancestors should be refused by name: %+v", res.Blocks[3])
	}
	lines := strings.Split(got, "\n")
	// Block one: open work tasks, by deadline (missing last) then priority.
	rows := orgQlRows(lines, 0)
	want := []string{
		"| Todo | P | Heading | Deadline |",
		"| NEXT |   | [[id:room-1][Book the room]] | " + day(-2)[:10] + " |",
		"| NEXT | B | [[*Write the report][Write the report]] | " + day(3)[:10] + " |",
		"| TODO | A | [[*Call the bank][Call the bank]] | |",
	}
	if !orgQlSame(rows, want) {
		t.Fatalf("block one:\n%s", strings.Join(rows, "\n"))
	}
	// Block two: NEXT or WAITING, not home, sorted by heading, the last one.
	if rows := orgQlRows(lines, 1); !orgQlSame(rows, []string{"| Heading |", "| [[*Write the report][Write the report]] |"}) {
		t.Fatalf("block two:\n%s", strings.Join(rows, "\n"))
	}
	// Block three: a deadline in the last week, today included.
	if rows := orgQlRows(lines, 2); len(rows) != 2 || !strings.Contains(rows[1], "Book the room") {
		t.Fatalf("block three:\n%s", strings.Join(rows, "\n"))
	}
}

// The table lines of the nth block (from 0), rules left out.
func orgQlRows(lines []string, nth int) []string {
	begin := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "#+BEGIN:") {
			if nth == 0 {
				begin = i
				break
			}
			nth--
		}
	}
	out := []string{}
	if begin < 0 {
		return out
	}
	for _, l := range lines[begin+1:] {
		if !strings.HasPrefix(l, "|") {
			break
		}
		if !strings.HasPrefix(l, "|-") {
			out = append(out, l)
		}
	}
	return out
}

// Rows the same, ignoring how cells are padded.
func orgQlSame(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	norm := func(s string) string {
		cells := strings.Split(s, "|")
		for i, c := range cells {
			cells[i] = strings.TrimSpace(c)
		}
		return strings.Join(cells, "|")
	}
	for i := range got {
		if norm(got[i]) != norm(want[i]) {
			return false
		}
	}
	return true
}
