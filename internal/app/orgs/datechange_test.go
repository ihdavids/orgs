package orgs

import (
	"strings"
	"testing"
)

func dateEdit(t *testing.T, text, name, value string) string {
	t.Helper()
	stamp := ""
	if value != "" {
		var err error
		if stamp, err = dateChangeStamp(name, value); err != nil {
			t.Fatalf("stamp %q: %v", value, err)
		}
	}
	lines := strings.Split(text, "\n")
	out, err := changeDateLines(lines, 1, 2, name, stamp)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(out, "\n")
}

const dcHead = "* P\n** TODO A  :work:\n"
const dcNext = "** TODO B\n   SCHEDULED: <2026-10-08 Thu>"

func TestChangeDateMovesOnlyThePlanningLine(t *testing.T) {
	in := dcHead + "SCHEDULED: <2026-10-05 Mon> DEADLINE: <2026-10-20 Tue>\n:PROPERTIES:\n:EFFORT:   3d\n:END:\n" + dcNext
	want := dcHead + "SCHEDULED: <2026-10-08 Thu> DEADLINE: <2026-10-20 Tue>\n:PROPERTIES:\n:EFFORT:   3d\n:END:\n" + dcNext
	if got := dateEdit(t, in, "SCHEDULED", "<2026-10-08 Thu>"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestChangeDateAddsAPlanningLineAboveTheDrawer(t *testing.T) {
	in := dcHead + ":PROPERTIES:\n:EFFORT:   3d\n:END:\n" + dcNext
	want := dcHead + "SCHEDULED: <2026-10-08 Thu>\n:PROPERTIES:\n:EFFORT:   3d\n:END:\n" + dcNext
	if got := dateEdit(t, in, "SCHEDULED", "<2026-10-08 Thu>"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// With no drawer to copy the indent from, it gets the level's indent.
	if got := dateEdit(t, dcHead+dcNext, "DEADLINE", "2026-10-09"); got != dcHead+"   DEADLINE: <2026-10-09 Fri>\n"+dcNext {
		t.Errorf("got\n%s", got)
	}
}

func TestChangeDateAppendsToAnExistingPlanningLine(t *testing.T) {
	in := dcHead + "   DEADLINE: <2026-10-20 Tue>\n" + dcNext
	want := dcHead + "   DEADLINE: <2026-10-20 Tue> SCHEDULED: <2026-10-08 Thu>\n" + dcNext
	if got := dateEdit(t, in, "SCHEDULED", "<2026-10-08 Thu>"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestChangeDateRemoves(t *testing.T) {
	in := dcHead + "   SCHEDULED: <2026-10-05 Mon> DEADLINE: <2026-10-20 Tue>\n" + dcNext
	if got := dateEdit(t, in, "SCHEDULED", ""); got != dcHead+"   DEADLINE: <2026-10-20 Tue>\n"+dcNext {
		t.Errorf("got\n%s", got)
	}
	// The last date on the line takes the line with it.
	in = dcHead + "   SCHEDULED: <2026-10-05 Mon>\n   body\n" + dcNext
	if got := dateEdit(t, in, "SCHEDULED", ""); got != dcHead+"   body\n"+dcNext {
		t.Errorf("got\n%s", got)
	}
	// Nothing to remove changes nothing - and never the next heading's date.
	if got := dateEdit(t, dcHead+dcNext, "SCHEDULED", ""); got != dcHead+dcNext {
		t.Errorf("got\n%s", got)
	}
}

func TestChangeDateKeepsRepeaters(t *testing.T) {
	in := dcHead + "   SCHEDULED: <2026-10-05 Mon +1w>\n" + dcNext
	if got := dateEdit(t, in, "SCHEDULED", "<2026-10-12 Mon +1w>"); got != dcHead+"   SCHEDULED: <2026-10-12 Mon +1w>\n"+dcNext {
		t.Errorf("got\n%s", got)
	}
}

func TestChangeDateOwnTimestamp(t *testing.T) {
	in := dcHead + "   SCHEDULED: <2026-10-01 Thu>\n   :PROPERTIES:\n   :X: <2026-01-01 Thu>\n   :END:\n   <2026-10-05 Mon> notes\n" + dcNext
	want := dcHead + "   SCHEDULED: <2026-10-01 Thu>\n   :PROPERTIES:\n   :X: <2026-01-01 Thu>\n   :END:\n   <2026-10-08 Thu> notes\n" + dcNext
	if got := dateEdit(t, in, "TIMESTAMP", "<2026-10-08 Thu>"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	in = dcHead + "   :PROPERTIES:\n   :X: 1\n   :END:\n" + dcNext
	want = dcHead + "   :PROPERTIES:\n   :X: 1\n   :END:\n   <2026-10-08 Thu>\n" + dcNext
	if got := dateEdit(t, in, "TIMESTAMP", "<2026-10-08 Thu>"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := dateEdit(t, want, "TIMESTAMP", ""); got != in {
		t.Errorf("got\n%s\nwant\n%s", got, in)
	}
}

func TestChangeDateStamp(t *testing.T) {
	for in, want := range map[string]string{
		"2026-10-08 10:00": "<2026-10-08 Thu 10:00>",
		"<2026-10-08 Thu>": "<2026-10-08 Thu>",
	} {
		if got, err := dateChangeStamp("SCHEDULED", in); err != nil || got != want {
			t.Errorf("%q: got %q %v, want %q", in, got, err, want)
		}
	}
	if got, _ := dateChangeStamp("CLOSED", "2026-10-08"); got != "[2026-10-08 Thu]" {
		t.Errorf("closed: got %q", got)
	}
	if _, err := dateChangeStamp("SCHEDULED", "next tuesday-ish"); err == nil {
		t.Error("nonsense was accepted")
	}
}
