package orgs

// The three things the timeline writes, end to end through the real handlers:
// throwing a whole block off it, annotating a block, and taking either of them
// back. Each of these is a join between the engine, the journal and the org
// file on disk, and a break in any of the three shows up here.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/dnd"
	logging "gopkg.in/op/go-logging.v1"
)

// clocktable.go's init() calls Conf(), which - on a fresh process - loads a
// real configuration and calls flag.Parse while it is at it, and a test
// binary's own flags do not survive that. Package level variables are
// initialized before any init function runs, which makes this the one place a
// test in this package can get in first. Every test then points the same
// Config at its own temporary tree.
var _ = func() bool {
	config = new(Config)
	config.Server = &common.ServerSettings{}
	config.PlugManager = new(common.PluginManager)
	config.Out = logging.MustGetLogger("test")
	return true
}()

const timelineSession = `#+TITLE: Test Night
#+DATE: [2026-09-20 Sun]
#+SUMMARY: A test.
#+FILETAGS: :dnd:session:

* Characters
** Lyra
   :PROPERTIES:
   :DND_ID: lyra-test-1
   :DND_CHARACTER: Lyra
   :END:

* Notes
** 19:20
   The wagon tracks leave the road.

** 19:35
   Kael is down.

** 21:10
   A library, three floors of it.

* Rolls
#+NAME: rolls
| Time  | Character | Roll       | Formula | Result | Dice   | Notes |
|-------+-----------+------------+---------+--------+--------+-------|
| 19:21 | Lyra      | Perception | d20 +5  | 18     | d20 13 |       |
| 19:33 | Lyra      | Longsword  | d20 +7  | 21     | d20 14 |       |
| 19:34 | Lyra      | Longsword  | 1d8 +4  | 9      | d8 5   |       |
| 19:36 | Lyra      | Fireball   | cast    | 24     | 8d6    |       |
| 19:38 | Lyra      | Longsword  | d20 +7  | 27     | d20 20 |       |
`

// timelineDir stands a throwaway org tree up with one session in it and points
// the server's configuration at it.
func timelineDir(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0755); err != nil {
		t.Fatal(err)
	}
	id := "2026_09_20_test"
	file := filepath.Join(dir, "sessions", id+".org")
	if err := os.WriteFile(file, []byte(timelineSession), 0644); err != nil {
		t.Fatal(err)
	}
	config.Server = &common.ServerSettings{
		OrgDirs:        []string{dir},
		DndSessionPath: "sessions",
	}
	odb = nil
	dndJournal = nil
	t.Cleanup(func() { odb = nil; dndJournal = nil })
	return id, file
}

// call runs one handler the way the router would, and hands back the body.
func call(t *testing.T, h http.HandlerFunc, method, url string,
	vars map[string]string, body interface{}) (int, []byte) {
	t.Helper()
	var r *http.Request
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = httptest.NewRequest(method, url, strings.NewReader(string(raw)))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	r = mux.SetURLVars(r, vars)
	w := httptest.NewRecorder()
	h(w, r)
	return w.Code, w.Body.Bytes()
}

func readDetail(t *testing.T, raw []byte) dnd.SessionDetail {
	t.Helper()
	var d dnd.SessionDetail
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("%s in %.300s", err, raw)
	}
	return d
}

// A combat block is four rolls with a note written during it. Throwing it away
// takes all five lines in one write, which is what makes one press of Undo put
// the whole thing back.
func TestTimelineBlockGoesAndComesBack(t *testing.T) {
	id, file := timelineDir(t)
	before, _ := os.ReadFile(file)

	code, raw := call(t, PostDndPlayBlockDelete, "POST",
		"/dnd/play/session/"+id+"/delete", map[string]string{"id": id},
		map[string]interface{}{
			"what": "a combat block",
			"rolls": []map[string]interface{}{
				{"index": 1, "was": "Longsword"}, {"index": 2, "was": "Longsword"},
				{"index": 3, "was": "Fireball"}, {"index": 4, "was": "Longsword"},
			},
			"notes": []map[string]interface{}{{"index": 1, "was": "19:35"}},
		})
	if code != http.StatusOK {
		t.Fatalf("delete: %d %s", code, raw)
	}
	d := readDetail(t, raw)
	if len(d.RollLog) != 1 || d.RollLog[0].Label != "Perception" {
		t.Fatalf("want only the Perception roll left: %#v", d.RollLog)
	}
	if len(d.NoteLog) != 2 {
		t.Fatalf("want two notes left, got %d", len(d.NoteLog))
	}
	for _, n := range d.NoteLog {
		if n.Time == "19:35" {
			t.Fatalf("the note written during the fight should have gone with it")
		}
	}

	// One write, so one thing to take back.
	code, raw = call(t, RequestDndPlayUndo, "GET", "/dnd/play/session/"+id+"/undo",
		map[string]string{"id": id}, nil)
	if code != http.StatusOK {
		t.Fatalf("undo plan: %d %s", code, raw)
	}
	var plan dnd.UndoPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Can || !strings.Contains(plan.What, "combat block") {
		t.Fatalf("the undo offer should name the block: %#v", plan)
	}

	code, raw = call(t, PostDndPlayUndo, "POST", "/dnd/play/session/"+id+"/undo",
		map[string]string{"id": id}, nil)
	if code != http.StatusOK {
		t.Fatalf("undo: %d %s", code, raw)
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(before) {
		t.Fatalf("undo did not put the file back:\n%s", after)
	}
	// And there is nothing left to take back.
	_, raw = call(t, RequestDndPlayUndo, "GET", "/dnd/play/session/"+id+"/undo",
		map[string]string{"id": id}, nil)
	json.Unmarshal(raw, &plan)
	if plan.Can {
		t.Fatalf("undo should be spent: %#v", plan)
	}
}

// Half a fight deleted would be worse than none of it.
func TestTimelineBlockRefusesOnAStaleReading(t *testing.T) {
	id, file := timelineDir(t)
	before, _ := os.ReadFile(file)
	code, raw := call(t, PostDndPlayBlockDelete, "POST",
		"/dnd/play/session/"+id+"/delete", map[string]string{"id": id},
		map[string]interface{}{
			"rolls": []map[string]interface{}{
				{"index": 1, "was": "Longsword"}, {"index": 2, "was": "Dagger"},
			},
		})
	if code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", code, raw)
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(before) {
		t.Fatalf("a refused delete must leave the file alone")
	}
	// Nothing was written, so there is nothing to undo either.
	_, raw = call(t, RequestDndPlayUndo, "GET", "/dnd/play/session/"+id+"/undo",
		map[string]string{"id": id}, nil)
	var plan dnd.UndoPlan
	json.Unmarshal(raw, &plan)
	if plan.Can {
		t.Fatalf("a refusal is not a change: %#v", plan)
	}
}

func TestTimelineAnnotationsAreWrittenAndTakenBack(t *testing.T) {
	id, file := timelineDir(t)

	code, raw := call(t, PostDndPlayMark, "POST", "/dnd/play/session/"+id+"/mark",
		map[string]string{"id": id}, map[string]interface{}{
			"time": "19:33", "kind": "fight", "title": "Ambush at the ford",
			"note": "They came out of the reeds."})
	if code != http.StatusOK {
		t.Fatalf("mark: %d %s", code, raw)
	}
	d := readDetail(t, raw)
	if len(d.MarkLog) != 1 || d.MarkLog[0].Title != "Ambush at the ford" {
		t.Fatalf("the annotation did not come back: %#v", d.MarkLog)
	}
	text, _ := os.ReadFile(file)
	if !strings.Contains(string(text), "** 19:33 Ambush at the ford :fight:") {
		t.Fatalf("not written as org:\n%s", text)
	}
	// Nothing in the logs moved.
	if len(d.RollLog) != 5 || len(d.NoteLog) != 3 {
		t.Fatalf("annotating changed the logs: %d rolls, %d notes",
			len(d.RollLog), len(d.NoteLog))
	}

	// Renaming rewrites the one entry rather than leaving two.
	_, raw = call(t, PostDndPlayMark, "POST", "/dnd/play/session/"+id+"/mark",
		map[string]string{"id": id}, map[string]interface{}{
			"time": "19:33", "kind": "fight", "title": "The ford"})
	d = readDetail(t, raw)
	if len(d.MarkLog) != 1 || d.MarkLog[0].Title != "The ford" {
		t.Fatalf("rename left the old one behind: %#v", d.MarkLog)
	}

	// A moment nobody rolled for gets onto the timeline all the same.
	_, raw = call(t, PostDndPlayMark, "POST", "/dnd/play/session/"+id+"/mark",
		map[string]string{"id": id}, map[string]interface{}{
			"time": "22:00", "kind": "moment", "title": "The locked door",
			"note": "Iron, and warm to the touch."})
	d = readDetail(t, raw)
	if len(d.MarkLog) != 2 {
		t.Fatalf("want two annotations, got %#v", d.MarkLog)
	}

	// Taking one off leaves the other, and the block it was on.
	code, raw = call(t, DeleteDndPlayMark, "DELETE",
		"/dnd/play/session/"+id+"/mark?time=19:33&kind=fight",
		map[string]string{"id": id}, nil)
	if code != http.StatusOK {
		t.Fatalf("unmark: %d %s", code, raw)
	}
	d = readDetail(t, raw)
	if len(d.MarkLog) != 1 || d.MarkLog[0].Title != "The locked door" {
		t.Fatalf("wrong annotation removed: %#v", d.MarkLog)
	}
	if len(d.RollLog) != 5 {
		t.Fatalf("taking a name off a fight must not touch the fight")
	}

	// And that removal is itself one press of undo.
	code, raw = call(t, PostDndPlayUndo, "POST", "/dnd/play/session/"+id+"/undo",
		map[string]string{"id": id}, nil)
	if code != http.StatusOK {
		t.Fatalf("undo: %d %s", code, raw)
	}
	var back DndSessionUndoState
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.MarkLog) != 2 {
		t.Fatalf("undo did not put the annotation back: %#v", back.MarkLog)
	}
	if !back.Undo.Can {
		t.Fatalf("there are three more writes behind this one to take back")
	}
}

// An annotation on a block that is being thrown away goes with it, in the same
// write - otherwise undo would put the block back and leave the name off, or
// leave a name hanging on nothing.
func TestTimelineBlockTakesItsAnnotationWithIt(t *testing.T) {
	id, _ := timelineDir(t)
	call(t, PostDndPlayMark, "POST", "/dnd/play/session/"+id+"/mark",
		map[string]string{"id": id}, map[string]interface{}{
			"time": "19:33", "kind": "fight", "title": "Ambush at the ford"})

	code, raw := call(t, PostDndPlayBlockDelete, "POST",
		"/dnd/play/session/"+id+"/delete", map[string]string{"id": id},
		map[string]interface{}{
			"what": "a combat block",
			"rolls": []map[string]interface{}{
				{"index": 1}, {"index": 2}, {"index": 3}, {"index": 4}},
			"notes": []map[string]interface{}{{"index": 1}},
			"mark":  map[string]interface{}{"time": "19:33", "kind": "fight"},
		})
	if code != http.StatusOK {
		t.Fatalf("delete: %d %s", code, raw)
	}
	d := readDetail(t, raw)
	if len(d.MarkLog) != 0 {
		t.Fatalf("the name outlived the block: %#v", d.MarkLog)
	}

	_, raw = call(t, PostDndPlayUndo, "POST", "/dnd/play/session/"+id+"/undo",
		map[string]string{"id": id}, nil)
	var back DndSessionUndoState
	json.Unmarshal(raw, &back)
	if len(back.RollLog) != 5 || len(back.MarkLog) != 1 {
		t.Fatalf("one press should bring back the rolls and the name: %d rolls, %#v",
			len(back.RollLog), back.MarkLog)
	}
}

// The journal's guard: a file something else has changed cannot be put back
// without throwing that change away, so the offer is withdrawn rather than
// taken quietly.
func TestTimelineUndoRefusesAFileThatMoved(t *testing.T) {
	id, file := timelineDir(t)
	call(t, PostDndPlayMark, "POST", "/dnd/play/session/"+id+"/mark",
		map[string]string{"id": id}, map[string]interface{}{
			"time": "19:33", "kind": "fight", "title": "Ambush at the ford"})

	text, _ := os.ReadFile(file)
	os.WriteFile(file, append(text, []byte("\n* Afterwards\n")...), 0644)

	_, raw := call(t, RequestDndPlayUndo, "GET", "/dnd/play/session/"+id+"/undo",
		map[string]string{"id": id}, nil)
	var plan dnd.UndoPlan
	json.Unmarshal(raw, &plan)
	if plan.Can {
		t.Fatalf("undo should not offer to throw away a hand edit: %#v", plan)
	}
	code, raw := call(t, PostDndPlayUndo, "POST", "/dnd/play/session/"+id+"/undo",
		map[string]string{"id": id}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", code, raw)
	}
	now, _ := os.ReadFile(file)
	if !strings.Contains(string(now), "* Afterwards") {
		t.Fatalf("the hand edit was thrown away anyway")
	}
}

// The timeline's undo is narrowed to the session being looked at. Two sessions
// open at once must not take each other's changes back.
func TestTimelineUndoStaysInItsOwnSession(t *testing.T) {
	id, file := timelineDir(t)
	dir := filepath.Dir(file)
	other := "2026_09_21_other"
	if err := os.WriteFile(filepath.Join(dir, other+".org"),
		[]byte(timelineSession), 0644); err != nil {
		t.Fatal(err)
	}

	call(t, PostDndPlayMark, "POST", "/dnd/play/session/"+id+"/mark",
		map[string]string{"id": id}, map[string]interface{}{
			"time": "19:33", "kind": "fight", "title": "The ford"})
	// Something happens in the other session afterwards, so it is newer.
	call(t, PostDndPlayMark, "POST", "/dnd/play/session/"+other+"/mark",
		map[string]string{"id": other}, map[string]interface{}{
			"time": "21:10", "kind": "note", "title": "The library"})

	_, raw := call(t, PostDndPlayUndo, "POST", "/dnd/play/session/"+id+"/undo",
		map[string]string{"id": id}, nil)
	var back DndSessionUndoState
	json.Unmarshal(raw, &back)
	if len(back.MarkLog) != 0 {
		t.Fatalf("this session's own change should have gone: %#v", back.MarkLog)
	}
	// And the other one is untouched.
	d, err := GetDndSession(other)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.MarkLog) != 1 {
		t.Fatalf("the other session lost its annotation: %#v", d.MarkLog)
	}
}
