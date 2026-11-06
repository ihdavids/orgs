package orgs

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

var timelineNow = time.Date(2026, 11, 4, 15, 0, 0, 0, time.Local)

func timelineSetup(t *testing.T) map[string]string {
	t.Helper()
	withFeatures(t, func(f *common.FeatureSettings) { f.Timeline = true })
	return loadOrg(t, map[string]string{
		"history.org": `#+TITLE: History
#+TIMELINE: group:tag title:"England"
* Roman Britain :era:
:PROPERTIES:
:DATE: 43 -- 410
:END:
* Battle of Hastings :england:
:PROPERTIES:
:DATE: 1066-10-14
:END:
William of Normandy defeats [[https://x.org][Harold]] Godwinson.
* Magna Carta :england:
:PROPERTIES:
:DATE: c. 1215
:END:
* Hidden
:PROPERTIES:
:DATE: 1300
:TIMELINE: no
:END:
* Caesar :rome:
:PROPERTIES:
:DATE: 44 BC
:COLOR: green
:END:
#+BEGIN_EXAMPLE
#+TIMELINE: not this one
#+END_EXAMPLE
`,
		"work.org": `* Launch
<2026-11-02 Mon>--<2026-11-05 Thu>
* TODO Plan
SCHEDULED: <2026-11-10 Tue> DEADLINE: <2026-11-12 Thu>
* DONE Ship
CLOSED: [2026-11-03 Tue 10:00]
* 2026-11-01 Sunday
** Note under a day
* Undated
`,
	})
}

func TestTimelineEvents(t *testing.T) {
	files := timelineSetup(t)
	hist := files["history.org"]
	d, _, err := timelineFor("group:tag", hist, timelineNow)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, ev := range d.Events {
		got = append(got, ev.Title+"|"+ev.Kind+"|"+ev.Label+"|"+ev.Lane)
	}
	want := []string{
		"Caesar|point|44 BC|rome",
		"Roman Britain|era|43 – 410|",
		"Battle of Hastings|point|14 Oct 1066|england",
		"Magna Carta|point|c. 1215|england",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("events:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(d.Lanes) != 2 || d.Lanes[0].Name != "rome" || d.Lanes[0].Slot != 0 {
		t.Errorf("lanes %+v", d.Lanes)
	}
	if d.Events[0].Colour != trackerHues["green"] || d.Events[0].Slot != -1 {
		t.Errorf("own colour: %+v", d.Events[0])
	}
	if !strings.HasPrefix(d.Events[2].Snippet, "William of Normandy defeats Harold Godwinson") {
		t.Errorf("snippet %q", d.Events[2].Snippet)
	}
}

func TestTimelineOrgDates(t *testing.T) {
	files := timelineSetup(t)
	d, _, err := timelineFor("", files["work.org"], timelineNow)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ev := range d.Events {
		got[ev.Title] = ev.Kind + "|" + ev.Source + "|" + ev.Label
	}
	want := map[string]string{
		"Launch":            "span|range|2 Nov 2026 – 5 Nov 2026",
		"Plan":              "span|planned|10 Nov 2026 – 12 Nov 2026",
		"Ship":              "point|closed|3 Nov 2026 10:00",
		"2026-11-01 Sunday": "point|datetree|1 Nov 2026",
		"Note under a day":  "point|datetree|1 Nov 2026",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["Undated"]; ok {
		t.Errorf("an undated heading is not an event")
	}
	// from/to: a range overlapping counts.
	d, _, _ = timelineFor("from:2026-11-04 to:2026-11-11", files["work.org"], timelineNow)
	names := []string{}
	for _, ev := range d.Events {
		names = append(names, ev.Title)
	}
	if strings.Join(names, ",") != "Launch,Plan" {
		t.Errorf("in range: %v", names)
	}
}

func TestTimelineSpecErrors(t *testing.T) {
	timelineSetup(t)
	for _, line := range []string{"group:colour", "from:sometime", "from:1900 to:1800", "height:5"} {
		if _, err := parseTimelineSpec(line, "", timelineNow); err == nil {
			t.Errorf("%q read", line)
		}
	}
}

func TestTimelineListAndExport(t *testing.T) {
	files := timelineSetup(t)
	ls := timelineList()
	if len(ls) != 1 || ls[0].Title != "England" || ls[0].Line != 1 {
		t.Fatalf("list %+v", ls)
	}
	lines := strings.Split(strings.TrimSuffix(timelineRead(t, files["history.org"]), "\n"), "\n")
	out, changed := timelineExportLines(files["history.org"], lines, "html")
	text := strings.Join(out, "\n")
	if !changed || !strings.Contains(text, `<figure class="org-timeline" data-timeline="group:tag title:&quot;England&quot;"`) || !strings.Contains(text, "Battle of Hastings") {
		t.Errorf("html copy:\n%s", text)
	}
	if !strings.Contains(text, "#+TIMELINE: not this one") {
		t.Errorf("the example block was touched")
	}
	out, _ = timelineExportLines(files["history.org"], lines, "latex")
	text = strings.Join(out, "\n")
	if !strings.Contains(text, "- 14 Oct 1066 :: Battle of Hastings (england)") || !strings.Contains(text, "- 43 – 410 :: Roman Britain (era)") {
		t.Errorf("latex copy:\n%s", text)
	}
}

func TestTimelineSvgTicksAndLabels(t *testing.T) {
	files := timelineSetup(t)
	d, _, _ := timelineFor("group:tag", files["history.org"], timelineNow)
	svg := timelineSvg(d, 800, timelineNow)
	for _, want := range []string{"Battle of Hastings", ">400<", ">1000<", `data-file="` + files["history.org"], "Roman Britain"} {
		if !strings.Contains(svg, want) {
			t.Errorf("svg lacks %q", want)
		}
	}
	if strings.Contains(svg, "Today") {
		t.Errorf("today is not in 50 BC .. 1250")
	}
}

func TestTimelineDynBlock(t *testing.T) {
	files := timelineSetup(t)
	f := GetDb().FindByFile(files["history.org"])
	c := &dynCtx{File: f, Block: dynBlock{Header: "group:tag"}, Now: timelineNow}
	lines, err := dynTimeline(c)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "| 14 Oct 1066") || !strings.Contains(text, "[[*Battle of Hastings][Battle of Hastings]]") || !strings.Contains(text, "| era") {
		t.Errorf("block:\n%s", text)
	}
	_ = filepath.Base
}

func timelineRead(t *testing.T, path string) string {
	t.Helper()
	lines, err := readFileLines(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}
