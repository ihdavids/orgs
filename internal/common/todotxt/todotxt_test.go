package todotxt

import (
	"strings"
	"testing"
)

func TestParseLine(t *testing.T) {
	task, ok := ParseLine("x 2026-10-12 2026-10-01 Call the plumber +house @phone due:2026-10-15 see https://x.org/a pri:B")
	if !ok {
		t.Fatal("not read")
	}
	if !task.Done || task.Priority != "B" || task.Completed.Format(dateFmt) != "2026-10-12" || task.Created.Format(dateFmt) != "2026-10-01" {
		t.Errorf("head wrong: %+v", task)
	}
	if task.Text != "Call the plumber see https://x.org/a" {
		t.Errorf("text %q", task.Text)
	}
	if len(task.Projects) != 1 || task.Projects[0] != "house" || len(task.Contexts) != 1 || task.Contexts[0] != "phone" {
		t.Errorf("projects/contexts %v %v", task.Projects, task.Contexts)
	}
	if task.Get("due") != "2026-10-15" || task.Get("pri") != "" {
		t.Errorf("extra %v", task.Extra)
	}
}

func TestRoundTrip(t *testing.T) {
	for _, l := range []string{
		"(A) 2026-10-01 Write the report +work @desk due:2026-10-20",
		"x 2026-10-12 2026-10-01 Done thing pri:C",
		"Plain task",
		"x 2026-10-12 Finished",
	} {
		task, _ := ParseLine(l)
		if got := task.String(); got != l {
			t.Errorf("round trip\n got %q\nwant %q", got, l)
		}
	}
}

func TestRec(t *testing.T) {
	cases := map[string]string{"1w": ".+1w", "+2d": "+2d", "3b": ".+3d", "x": ""}
	for in, want := range cases {
		if got := RecToOrg(in); got != want {
			t.Errorf("RecToOrg(%q) = %q want %q", in, got, want)
		}
	}
	back := map[string]string{".+1w": "1w", "+2d": "+2d", "++1m": "+1m", "+1h": ""}
	for in, want := range back {
		if got := OrgToRec(in); got != want {
			t.Errorf("OrgToRec(%q) = %q want %q", in, got, want)
		}
	}
}

func TestToOrg(t *testing.T) {
	tasks := Parse(`(A) 2026-10-01 Write the report +work +q4 @desk due:2026-10-20 rec:1w id:abc
Buy milk @shop
x 2026-10-12 2026-10-01 Filed taxes +home
`)
	got := ToOrg(tasks, OrgOptions{Title: "Inbox"})
	for _, want := range []string{
		"#+TITLE: Inbox\n",
		"* TODO Buy milk :@shop:\n",
		"* work\n** TODO [#A] Write the report :@desk:\n   DEADLINE: <2026-10-20 Tue .+1w>\n",
		"   :CREATED: [2026-10-01 Thu]\n   :PROJECTS: +q4\n   :ID: abc\n",
		"* home\n** DONE Filed taxes\n   CLOSED: [2026-10-12 Mon]\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Index(got, "Buy milk") > strings.Index(got, "* work") {
		t.Errorf("project-less tasks should come first:\n%s", got)
	}
}
