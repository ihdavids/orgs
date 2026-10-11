package orgs

import (

	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)


func TestEffortMinsDayHours(t *testing.T) {
	cases := []struct {
		text     string
		day      float64
		want     float64
		dayUnits bool
	}{
		{"2h", 24, 120, false},
		{"1:30", 8, 90, false},
		{"1d", 24, 1440, true},
		{"1d", 8, 480, true},
		{"1d 4h", 8, 720, true},
		// A week is five days, as ParseDuration has it.
		{"1w", 8, 5 * 480, true},
		{"30min", 8, 30, false},
	}
	for _, c := range cases {
		got, du, ok := effortMins(c.text, c.day)
		if !ok || !near(got, c.want) || du != c.dayUnits {
			t.Errorf("effortMins(%q, %v) = %v, %v, %v; want %v, %v", c.text, c.day, got, du, ok, c.want, c.dayUnits)
		}
	}
	if _, _, ok := effortMins("soon", 24); ok {
		t.Errorf("effortMins read a word as a duration")
	}
}

func TestRatioStats(t *testing.T) {
	m, p25, p75 := ratioStats([]float64{4, 1, 3, 2})
	if !near(m, 2.5) || !near(p25, 1.75) || !near(p75, 3.25) {
		t.Errorf("ratioStats = %v %v %v, want 2.5 1.75 3.25", m, p25, p75)
	}
	m, p25, p75 = ratioStats([]float64{2})
	if m != 2 || p25 != 2 || p75 != 2 {
		t.Errorf("one ratio: %v %v %v", m, p25, p75)
	}
	if m, _, _ := ratioStats(nil); m != 0 {
		t.Errorf("no ratios: %v", m)
	}
}

func est(tags []string, project string, estMins, actual float64) common.EstimateTask {
	return common.EstimateTask{Headline: project, Tags: tags, Project: project, EstMins: estMins, ActualMins: actual, Ratio: actual / estMins, Done: true}
}

func TestGroupEstimates(t *testing.T) {
	tasks := []common.EstimateTask{
		est([]string{"backend"}, "API", 60, 90),
		est([]string{"backend"}, "API", 60, 120),
		est([]string{"backend", "db"}, "Store", 120, 180),
		est([]string{"docs"}, "Store", 60, 60),
	}
	groups := groupEstimates(tasks, "tag")
	by := map[string]common.EstimateGroup{}
	for _, g := range groups {
		by[g.Name] = g
	}
	be := by["backend"]
	if be.Tasks != 3 || be.EstMins != 240 || be.ActualMins != 390 {
		t.Fatalf("backend totals: %+v", be)
	}
	if !near(be.Ratio, 390.0/240) || !near(be.Median, 1.5) {
		t.Errorf("backend ratio %v median %v", be.Ratio, be.Median)
	}
	if be.Multiplier != 1.5 || !strings.Contains(be.Sentence, ":backend: takes 1.5×") {
		t.Errorf("backend multiplier %v, sentence %q", be.Multiplier, be.Sentence)
	}
	if len(be.Worst) != 3 || be.Worst[0].EstMins != 60 || be.Worst[0].ActualMins != 120 && be.Worst[0].ActualMins != 180 {
		t.Errorf("backend worst: %+v", be.Worst)
	}
	// One task is too few for a multiplier, and the sentence says so.
	if d := by["docs"]; d.Multiplier != 0 || !strings.Contains(d.Sentence, "too few") {
		t.Errorf("docs: %+v", d)
	}
	// A task with two tags counts in both.
	if by["db"].Tasks != 1 {
		t.Errorf("db: %+v", by["db"])
	}
	// Biggest actual first.
	if groups[0].Name != "backend" {
		t.Errorf("order: %s first", groups[0].Name)
	}
	proj := groupEstimates(tasks, "project")
	if len(proj) != 2 || proj[0].Name != "Store" && proj[0].Name != "API" {
		t.Errorf("projects: %+v", proj)
	}
	// About right, and padded.
	right := summarize(common.EstimateGroup{Name: "x", Kind: "tag"}, []common.EstimateTask{
		est(nil, "", 60, 60), est(nil, "", 60, 63), est(nil, "", 60, 57)})
	if !strings.Contains(right.Sentence, "about right") {
		t.Errorf("about right: %q", right.Sentence)
	}
	padded := summarize(common.EstimateGroup{Name: "x", Kind: "tag"}, []common.EstimateTask{
		est(nil, "", 100, 50), est(nil, "", 100, 60), est(nil, "", 100, 40)})
	if padded.Multiplier != 0.5 || !strings.Contains(padded.Sentence, "padded") {
		t.Errorf("padded: %v %q", padded.Multiplier, padded.Sentence)
	}
}

const estimatesOrg = `#+TODO: TODO NEXT WAITING | DONE CANCELLED
* Backend work
** DONE Write the handler :backend:
   CLOSED: [2026-09-02 Wed 17:00]
   :PROPERTIES:
   :EFFORT:   2h
   :ASSIGNEE: Jane Roe
   :END:
   :LOGBOOK:
   CLOCK: [2026-09-02 Wed 09:00]--[2026-09-02 Wed 12:00] =>  3:00
   :END:
** DONE Migrate the table :backend:db:
   CLOSED: [2026-09-03 Thu 17:00]
   :PROPERTIES:
   :EFFORT:   1h
   :END:
   :LOGBOOK:
   CLOCK: [2026-09-03 Thu 09:00]--[2026-09-03 Thu 10:00] =>  1:00
   :END:
*** Subtask with no estimate of its own
    :LOGBOOK:
    CLOCK: [2026-09-03 Thu 13:00]--[2026-09-03 Thu 14:00] =>  1:00
    :END:
** DONE Estimated, never clocked :backend:
   CLOSED: [2026-09-04 Fri 10:00]
   :PROPERTIES:
   :EFFORT:   1h
   :END:
** DONE Clocked, never estimated :backend:
   CLOSED: [2026-09-04 Fri 12:00]
   :LOGBOOK:
   CLOCK: [2026-09-04 Fri 10:00]--[2026-09-04 Fri 12:00] =>  2:00
   :END:
** DONE A rollup :backend:
   CLOSED: [2026-09-05 Sat 12:00]
   :PROPERTIES:
   :EFFORT:   1d
   :END:
*** DONE Part one
    CLOSED: [2026-09-05 Sat 11:00]
    :PROPERTIES:
    :EFFORT:   4h
    :END:
    :LOGBOOK:
    CLOCK: [2026-09-05 Sat 08:00]--[2026-09-05 Sat 11:00] =>  3:00
    :END:
** CANCELLED Dropped :backend:
   CLOSED: [2026-09-05 Sat 12:00]
   :PROPERTIES:
   :EFFORT:   1h
   :END:
   :LOGBOOK:
   CLOCK: [2026-09-05 Sat 08:00]--[2026-09-05 Sat 09:00] =>  1:00
   :END:
** NEXT Still going :backend:
   :PROPERTIES:
   :EFFORT:   2h
   :END:
   :LOGBOOK:
   CLOCK: [2026-09-06 Sun 08:00]--[2026-09-06 Sun 09:00] =>  1:00
   :END:
** DONE Long ago :backend:
   CLOSED: [2026-01-05 Mon 12:00]
   :PROPERTIES:
   :EFFORT:   1h
   :END:
   :LOGBOOK:
   CLOCK: [2026-01-05 Mon 08:00]--[2026-01-05 Mon 09:00] =>  1:00
   :END:
`

func TestEstimatesFromFiles(t *testing.T) {
	loadOrg(t, map[string]string{"work.org": estimatesOrg})
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.Local)
	rng, err := insightRange("", "2026-09-01", "2026-09-30", now)
	if err != nil {
		t.Fatal(err)
	}
	res, err := buildEstimates(estimateArgs{By: "tag", Rng: rng, DayHours: 24, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tk := range res.Over {
		names = append(names, tk.Headline)
	}
	for _, tk := range res.Under {
		names = append(names, tk.Headline)
	}
	sort.Strings(names)
	// Measured: the handler (3h on 2h), the migration (1h own + 1h from the
	// unestimated subtask, on 1h) and Part one (3h on 4h). Not the rollup,
	// not the cancelled one, not the open one, not the one from January.
	if strings.Join(names, "|") != "Migrate the table|Part one|Write the handler" {
		t.Fatalf("measured: %v", names)
	}
	if res.All.Tasks != 3 || res.All.EstMins != 7*60 || res.All.ActualMins != 8*60 {
		t.Errorf("all: %+v", res.All)
	}
	if res.EffortNoClock != 1 || res.ClockNoEffort != 1 || res.ClockNoEffortMins != 120 || res.Rollups != 1 {
		t.Errorf("counts: effort-no-clock %d, clock-no-effort %d (%v min), rollups %d",
			res.EffortNoClock, res.ClockNoEffort, res.ClockNoEffortMins, res.Rollups)
	}
	if len(res.Unestimated) != 1 || res.Unestimated[0].Headline != "Clocked, never estimated" {
		t.Errorf("unestimated: %+v", res.Unestimated)
	}
	var mig common.EstimateTask
	for _, tk := range res.Over {
		if tk.Headline == "Migrate the table" {
			mig = tk
		}
	}
	if mig.ActualMins != 120 || mig.Ratio != 2 || mig.Project != "Backend work" {
		t.Errorf("migration: %+v", mig)
	}

	// The open task comes in only when asked, flagged.
	res, _ = buildEstimates(estimateArgs{By: "person", Rng: rng, DayHours: 24, Open: true, Now: now})
	if res.All.Tasks != 4 || res.All.InProgress != 1 {
		t.Errorf("with open: %+v", res.All)
	}
	people := map[string]int{}
	for _, g := range res.Groups {
		people[g.Name] = g.Tasks
	}
	if people["Jane Roe"] != 1 || people["(nobody)"] != 3 {
		t.Errorf("people: %v", people)
	}
}

const depsOrg = `#+TODO: TODO NEXT WAITING | DONE CANCELLED
* Chains
** TODO Design
   :PROPERTIES:
   :ID:       design
   :END:
** TODO Build
   :PROPERTIES:
   :ID:       build
   :AFTER:    design
   :END:
** TODO Test
   :PROPERTIES:
   :ID:       test
   :AFTER:    build
   :END:
** TODO Ship
   :PROPERTIES:
   :AFTER:    test
   :END:
* Waiting
** DONE Vendor quote
   :PROPERTIES:
   :ID:       quote
   :END:
** WAITING Sign the order
   :PROPERTIES:
   :AFTER:    quote
   :END:
* Dangling
** TODO Points at nothing
   :PROPERTIES:
   :AFTER:    no-such-id
   :END:
** TODO Spelled as a link
   :PROPERTIES:
   :AFTER:    id:design
   :END:
* Loop
** TODO Egg
   :PROPERTIES:
   :ID:       egg
   :AFTER:    hen
   :END:
** TODO Hen
   :PROPERTIES:
   :ID:       hen
   :AFTER:    egg
   :END:
** TODO Narcissus
   :PROPERTIES:
   :ID:       self
   :AFTER:    self
   :END:
* TODO Release
  :PROPERTIES:
  :ORDERED:  t
  :END:
** DONE Branch
** WAITING Sign-off from legal
** TODO Tag
** TODO Publish
** DONE Announce
* TODO Parent
  :PROPERTIES:
  :ID:       parent
  :END:
** TODO Child after its own parent
   :PROPERTIES:
   :AFTER:    parent
   :END:
`

func depsByKind(res common.DepReport) map[string][]common.DepProblem {
	out := map[string][]common.DepProblem{}
	for _, p := range res.Problems {
		out[p.Kind] = append(out[p.Kind], p)
	}
	return out
}

func chainNames(p common.DepProblem) string {
	out := []string{}
	for _, h := range p.Chain {
		out = append(out, h.Headline)
	}
	return strings.Join(out, " → ")
}

func TestDependencyHealth(t *testing.T) {
	loadOrg(t, map[string]string{"deps.org": depsOrg})
	done := make(chan common.DepReport, 1)
	go func() {
		res, err := buildDeps(depArgs{MinChain: 3})
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()
	var res common.DepReport
	select {
	case res = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("dependency health hung (a cycle not caught)")
	}
	k := depsByKind(res)

	// Cycles: egg/hen once (not once per member), the self loop, and a child
	// AFTER its own parent.
	cycles := []string{}
	for _, p := range k["cycle"] {
		cycles = append(cycles, chainNames(p))
	}
	sort.Strings(cycles)
	if len(cycles) != 3 {
		t.Fatalf("cycles: %q", cycles)
	}
	if cycles[0] != "Egg → Hen → Egg" && cycles[0] != "Hen → Egg → Hen" {
		t.Errorf("egg and hen: %q", cycles[0])
	}
	if cycles[1] != "Narcissus → Narcissus" || cycles[2] != "Parent → Child after its own parent" {
		t.Errorf("cycles: %q", cycles)
	}

	// Blocked chain: Ship waits on Test on Build on Design, reported once with
	// Design as the root, in work order. Not the ORDERED steps behind the
	// WAITING sign-off: that is the ordered-stuck problem, not a second one.
	bc := k["blocked-chain"]
	if len(bc) != 1 || chainNames(bc[0]) != "Design → Build → Test → Ship" || bc[0].Length != 4 || bc[0].Held != 3 {
		t.Fatalf("blocked chains: %+v", bc)
	}
	if bc[0].Chain[0].Via != "" || bc[0].Chain[1].Via != "AFTER" || bc[0].Chain[1].After != "design" {
		t.Errorf("chain links: %+v", bc[0].Chain)
	}

	// Waiting on done.
	wd := k["waiting-on-done"]
	if len(wd) != 1 || chainNames(wd[0]) != "Vendor quote → Sign the order" {
		t.Fatalf("waiting on done: %+v", wd)
	}

	// Dangling: one names nothing, one is spelled id:design and is told so.
	dg := map[string]common.DepProblem{}
	for _, p := range k["dangling"] {
		dg[p.Missing] = p
	}
	if len(dg) != 2 || dg["no-such-id"].Severity != "warn" {
		t.Fatalf("dangling: %+v", k["dangling"])
	}
	if !strings.Contains(dg["id:design"].Suggestion, `"design"`) {
		t.Errorf("id: spelling not explained: %q", dg["id:design"].Suggestion)
	}

	// ORDERED: stuck at step 2 (WAITING), and Announce done ahead of it.
	os := k["ordered-stuck"]
	if len(os) != 2 || chainNames(os[0])+chainNames(os[1]) == "" {
		t.Fatalf("ordered: %+v", os)
	}
	titles := os[0].Title + " | " + os[1].Title
	if !strings.Contains(titles, "stuck at step 2 of 5") || !strings.Contains(titles, "Announce is DONE ahead of Sign-off from legal") {
		t.Errorf("ordered titles: %s", titles)
	}
	// Tag is chained behind the waiting step by ORDERED, so a client clearing
	// its wait needs NOBLOCKING.
	g := buildDepGraph()
	for _, n := range g.nodes {
		if insightTitle(n.sec) == "Tag" {
			if e := g.edges[n]; e.via != "ORDERED" || insightTitle(e.to.sec) != "Sign-off from legal" {
				t.Errorf("Tag waits on %+v", e)
			}
			if !g.link(n).Chained {
				t.Errorf("Tag not marked chained")
			}
		}
	}

	// Errors first.
	if res.Problems[0].Severity != "error" {
		t.Errorf("first problem is %s", res.Problems[0].Severity)
	}

	// A query keeps only problems touching what it finds.
	only, err := buildDeps(depArgs{Query: `MatchHeadline("Sign the order")`, MinChain: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(only.Problems) != 1 || only.Problems[0].Kind != "waiting-on-done" {
		t.Errorf("scoped: %+v", only.Problems)
	}
}
