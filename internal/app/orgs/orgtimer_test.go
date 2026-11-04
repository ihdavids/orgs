package orgs

import (
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// tempHome keeps a test's timer_data.json and clock_data.json out of the
// package folder.
func tempHome(t *testing.T) {
	t.Helper()
	old := Conf().PlugManager.HomeDir
	Conf().PlugManager.HomeDir = t.TempDir()
	t.Cleanup(func() { Conf().PlugManager.HomeDir = old })
}

func TestRelativeTimer(t *testing.T) {
	tempHome(t)
	tm := &orgTimer{}
	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.Local)
	if s := tm.do(TimerOp{Op: "start"}, t0); !s.Running || s.Display != "0:00:00" {
		t.Fatalf("start: %+v", s)
	}
	tm.do(TimerOp{Op: "pause"}, t0.Add(3*time.Minute+12*time.Second))
	if s := tm.state(t0.Add(10 * time.Minute)); s.Display != "0:03:12" || !s.Paused {
		t.Errorf("paused: %+v", s)
	}
	tm.do(TimerOp{Op: "continue"}, t0.Add(10*time.Minute))
	if s := tm.state(t0.Add(11 * time.Minute)); s.Display != "0:04:12" {
		t.Errorf("continued: %+v", s)
	}
	if s := tm.do(TimerOp{Op: "set", Offset: "1:00:05"}, t0); s.Display != "1:00:05" {
		t.Errorf("set: %+v", s)
	}
	if s := tm.do(TimerOp{Op: "pause"}, t0); !s.Ok {
		t.Errorf("pause after set: %+v", s)
	}
	if s := tm.do(TimerOp{Op: "pause"}, t0); s.Ok {
		t.Errorf("paused twice")
	}
	tm.do(TimerOp{Op: "stop"}, t0)
	if s := tm.do(TimerOp{Op: "bogus"}, t0); s.Ok {
		t.Errorf("a bogus op was taken")
	}
}

func TestTimerItemsMakeOneList(t *testing.T) {
	lines := strings.Split("* Meeting\nAgenda below.\n** Child", "\n")
	lines = timerItemLines(lines, 0, 2, "", "- 0:01:00 :: first")
	lines = timerItemLines(lines, 0, len(lines)-1, "", "- 0:02:30 :: second")
	if got := strings.Join(lines, "\n"); got != "* Meeting\nAgenda below.\n\n- 0:01:00 :: first\n- 0:02:30 :: second\n** Child" {
		t.Errorf("items:\n%s", got)
	}
}

func TestIdleTimeIsResolved(t *testing.T) {
	tempHome(t)
	paths := loadOrg(t, map[string]string{"c.org": "* Work\n"})
	p := paths["c.org"]
	f := GetDb().FindByFile(p)
	sec := f.Doc.Outline.Children[0]
	c := Clock()
	saved := *c
	t.Cleanup(func() { *c = saved })
	now := time.Now()
	start := now.Add(-60 * time.Minute)
	c.Time = &org.OrgDate{Start: start, HaveTime: true}
	c.Target = &common.Target{Type: "file+olp", Filename: p, Id: "Work"}
	_ = sec
	if r := resolveIdle(ClockResolve{How: "subtract", Minutes: 20}, now); !r.Ok || !c.Time.Start.Equal(start.Add(20*time.Minute)) {
		t.Errorf("subtract: %+v %v", r, c.Time.Start)
	}
	if r := resolveIdle(ClockResolve{How: "stop", Minutes: 10}, now); !r.Ok || c.IsClockActive() {
		t.Fatalf("stop: %+v", r)
	}
	// 60 minutes, 20 taken off, stopped 10 before now: 30.
	if got := readOrg(t, p); !strings.Contains(got, "=>  0:30") {
		t.Errorf("clock line:\n%s", got)
	}
}
