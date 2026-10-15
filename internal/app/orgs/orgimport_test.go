package orgs

import (
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestImportEntries(t *testing.T) {
	head, entries := importEntries("#+TITLE: Cal\n\n* One\n  :PROPERTIES:\n  :ICAL_UID: a@x\n  :END:\n** child\n  :ICAL_UID: not-mine\n* Two\nbody\n")
	if head != "#+TITLE: Cal\n\n" || len(entries) != 2 {
		t.Fatalf("head %q entries %q", head, entries)
	}
	if entryKey(entries[0], "ICAL_UID") != "a@x" || entryKey(entries[1], "ICAL_UID") != "" {
		t.Errorf("keys %q %q", entryKey(entries[0], "ICAL_UID"), entryKey(entries[1], "ICAL_UID"))
	}
	known := knownKeys("* Old\n:PROPERTIES:\n:ical_uid: a@x\n:END:\n", "ICAL_UID")
	if !known["a@x"] {
		t.Errorf("a key in a column-zero drawer, any case, is known: %v", known)
	}
}

func TestColumnsCsv(t *testing.T) {
	res := common.ColumnsResult{File: "/n/plan.org",
		Spec: []common.ColumnSpec{{Title: "Item"}, {Title: "Effort"}},
		Rows: []common.ColumnRow{
			{Level: 1, LineNum: 0, Hash: "h1", Cells: []common.ColumnCell{{Value: "Project, big", Own: "Project, big"}, {Value: "3:30", Own: ""}}},
			{Level: 2, LineNum: 3, Hash: "h2", Cells: []common.ColumnCell{{Value: `Say "hi"`, Own: `Say "hi"`}, {Value: "2:00", Own: "2:00"}}},
		}}
	got := string(ColumnsCsv(res, CsvOptions{}))
	want := "Item,Effort\n\"Project, big\",3:30\n\"Say \"\"hi\"\"\",2:00\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	got = string(ColumnsCsv(res, CsvOptions{Sep: '\t', Own: true, Meta: true, NoHeader: true}))
	if !strings.HasPrefix(got, "1\tProject, big\t\t1\th1\t/n/plan.org\n") {
		t.Errorf("meta/own/tab: %q", got)
	}
}
