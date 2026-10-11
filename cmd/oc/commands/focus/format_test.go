package focus

import (
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestCountdown(t *testing.T) {
	for secs, want := range map[float64]string{
		0: "0:00", 0.2: "0:01", 59.1: "1:00", 247: "4:07", 1500: "25:00", 3723: "1:02:03", -5: "0:00",
	} {
		if got := Countdown(secs); got != want {
			t.Errorf("Countdown(%v) = %q, want %q", secs, got, want)
		}
	}
}

func TestStatusLine(t *testing.T) {
	work := common.FocusState{Phase: "work", Remaining: 751, Title: "Write the report"}
	if got := StatusLine(work, false); got != "🍅 12:31 Write the report" {
		t.Errorf("work: %q", got)
	}
	brk := common.FocusState{Phase: "break", Remaining: 299, LongBreak: true}
	if got := StatusLine(brk, false); got != "☕ 4:59 long break" {
		t.Errorf("break: %q", got)
	}
	if got := StatusLine(common.FocusState{Phase: "idle", Title: "x"}, false); got != "" {
		t.Errorf("idle should be empty, got %q", got)
	}
}

func TestProgress(t *testing.T) {
	st := common.FocusState{Total: 100}
	if got := Progress(st, 50, 10); got != "━━━━━─────" {
		t.Errorf("half: %q", got)
	}
	if got := Progress(st, 0, 4); got != "━━━━" {
		t.Errorf("done: %q", got)
	}
}
