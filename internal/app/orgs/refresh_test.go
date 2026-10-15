package orgs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

func TestParseSchedule(t *testing.T) {
	cases := map[string]time.Duration{"1h": time.Hour, "30m": 30 * time.Minute, "1h30m": 90 * time.Minute,
		"2d": 48 * time.Hour, "1w": 7 * 24 * time.Hour, "hourly": time.Hour, "daily": 24 * time.Hour,
		"10s": time.Minute, "every 15m": 15 * time.Minute, `"5m"`: 5 * time.Minute}
	for in, want := range cases {
		s, err := parseSchedule(in)
		if err != nil || s.Every != want || s.At {
			t.Errorf("%q: %+v %v, want %v", in, s, err, want)
		}
	}
	for _, in := range []string{"daily 07:30", "07:30", "@7:30"} {
		s, err := parseSchedule(in)
		if err != nil || !s.At || s.Hour != 7 || s.Min != 30 {
			t.Errorf("%q: %+v %v", in, s, err)
		}
	}
	for _, in := range []string{"", "soon", "25:00", "1x"} {
		if _, err := parseSchedule(in); err == nil {
			t.Errorf("%q should not read", in)
		}
	}
}

func TestScheduleDue(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, loc)
	hourly, _ := parseSchedule("1h")
	if due, _ := hourly.due(time.Time{}, now); !due {
		t.Errorf("never run is due")
	}
	if due, next := hourly.due(now.Add(-30*time.Minute), now); due || !next.Equal(now.Add(30*time.Minute)) {
		t.Errorf("half an hour ago: due %v next %v", due, next)
	}
	if due, _ := hourly.due(now.Add(-time.Hour), now); !due {
		t.Errorf("an hour ago is due")
	}
	at, _ := parseSchedule("07:30")
	if due, _ := at.due(now.AddDate(0, 0, -1), now); !due {
		t.Errorf("not run since yesterday, past 07:30: due")
	}
	if due, next := at.due(now.Add(-time.Hour), now); due || next.Day() != 16 {
		t.Errorf("run at 08:00 today: not due, next tomorrow; got %v %v", due, next)
	}
	early := time.Date(2026, 10, 15, 6, 0, 0, 0, loc)
	if due, next := at.due(early.AddDate(0, 0, -1), early); due || next.Hour() != 7 {
		t.Errorf("before 07:30: not yet; got %v %v", due, next)
	}
}

const refreshSample = `#+TITLE: Dash
* Disk
#+NAME: disk
#+BEGIN_SRC sh :schedule 1h
echo "used 42%"
#+END_SRC

#+RESULTS: disk
: used 1%

* Work
:PROPERTIES:
:EFFORT: 1:00
:END:
** TODO Write
:PROPERTIES:
:EFFORT: 0:30
:END:
* Report
#+BEGIN: columnview :id global :format "%ITEM %EFFORT" :refresh 30m
#+END:

* Sums
| a | b | sum |
|---+---+-----|
| 1 | 2 |     |
| 3 | 4 |     |
#+TBLFM: $3=$1+$2
`

func TestRefreshFindsAndRuns(t *testing.T) {
	need(t, "bash")
	babelOn(t, nil)
	if dir, err := os.MkdirTemp("", "orgs-refresh-"); err == nil {
		old := Conf().PlugManager.HomeDir
		Conf().PlugManager.HomeDir = dir
		t.Cleanup(func() { Conf().PlugManager.HomeDir = old; os.RemoveAll(dir) })
	}
	oldRefresh := Conf().Server.Refresh
	Conf().Server.Refresh = common.RefreshSettings{Every: "2h"}
	t.Cleanup(func() { Conf().Server.Refresh = oldRefresh })
	refresher = &refreshEngine{records: map[string]refreshRecord{}}

	paths := loadOrg(t, map[string]string{"dash.org": refreshSample})
	p := paths["dash.org"]
	f := GetDb().FindByFile(p)
	lines, _ := readFileLines(p)
	kinds := map[string]refreshEntry{}
	for _, en := range refreshEntriesOf(f, lines, "2h") {
		kinds[en.Kind] = en
	}
	if b := kinds["block"]; b.Name != "disk" || b.sched.Every != time.Hour || b.Line != 3 {
		t.Errorf("block %+v", b)
	}
	if d := kinds["dblock"]; d.Name != "columnview" || d.sched.Every != 30*time.Minute {
		t.Errorf("dblock %+v", d)
	}
	if fl := kinds["file"]; fl.From != "refresh.every" || fl.sched.Every != 2*time.Hour {
		t.Errorf("file %+v", fl)
	}

	// An export sees the refreshed copy; the file is not written.
	before, _ := os.ReadFile(p)
	ef := exportFile(p)
	if ef == nil || ef == GetDb().FindByFile(p) {
		t.Fatalf("export read the file as it is")
	}
	if after, _ := os.ReadFile(p); string(after) != string(before) {
		t.Errorf("an export wrote the file")
	}

	res := RunRefresh(&common.RefreshRun{All: true})
	if !res.Ok {
		t.Fatalf("run: %s %+v", res.Msg, res.Items)
	}
	got, _ := os.ReadFile(p)
	for _, want := range []string{"#+RESULTS: disk\n: used 42%\n", "| 1 | 2 | 3   |", "| 3 | 4 | 7   |", "| Write"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	// Again: nothing changes, and nothing is written.
	st1, _ := os.Stat(p)
	time.Sleep(20 * time.Millisecond)
	res = RunRefresh(&common.RefreshRun{All: true})
	st2, _ := os.Stat(p)
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Errorf("a second run with nothing new rewrote the file: %+v", res.Items)
	}
	for _, it := range res.Items {
		if !it.Ok || it.Changed {
			t.Errorf("second run: %+v", it)
		}
	}
	// Remembered across a restart.
	if _, err := os.Stat(filepath.Join(Conf().PlugManager.HomeDir, "refresh_data.json")); err != nil {
		t.Errorf("no record kept: %v", err)
	}
}

func TestScheduledBlocksWaitForBabel(t *testing.T) {
	old := Conf().Server.Babel
	Conf().Server.Babel = common.BabelSettings{}
	t.Cleanup(func() { Conf().Server.Babel = old })
	en := refreshEntry{RefreshItem: common.RefreshItem{Kind: "block"}}
	if why := blockedReason(en); !strings.Contains(why, "babel.enable") {
		t.Errorf("got %q", why)
	}
}
