package trello

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fixture = `{
 "id": "b1", "name": "Website", "url": "https://trello.com/b/b1",
 "lists": [
  {"id": "l2", "name": "Done", "closed": false, "pos": 2},
  {"id": "l1", "name": "Doing", "closed": false, "pos": 1}
 ],
 "cards": [
  {"id": "c1", "name": "Build the page", "desc": "Some words\n* not a heading", "due": "2026-10-06T16:00:00.000Z",
   "dueComplete": false, "closed": false, "idList": "l1", "idMembers": ["m1"], "shortUrl": "https://trello.com/c/c1",
   "pos": 1, "labels": [{"name": "front end", "color": "green"}, {"name": "", "color": "red"}]},
  {"id": "c2", "name": "Design", "idList": "l2", "pos": 1, "labels": []},
  {"id": "c3", "name": "Archived", "closed": true, "idList": "l1", "pos": 2}
 ],
 "checklists": [
  {"idCard": "c1", "name": "Steps", "pos": 1, "checkItems": [
   {"name": "second", "state": "incomplete", "pos": 2}, {"name": "first", "state": "complete", "pos": 1}]}
 ],
 "members": [{"id": "m1", "username": "ann"}]
}`

func TestABoardIsWrittenAsOrg(t *testing.T) {
	var b apiBoard
	if err := json.Unmarshal([]byte(fixture), &b); err != nil {
		t.Fatal(err)
	}
	got := (&Trello{DoneLists: []string{"done"}}).Write(&b)
	for _, want := range []string{
		"#+TITLE: Trello - Website\n",
		"* Doing\n:PROPERTIES:\n:TRELLOID: l1\n:END:\n** TODO Build the page :front_end:red:\nDEADLINE: <2026-10-06 ",
		":ASSIGNED: ann\n:END:\nSome words\n * not a heading\n- Steps [1/2]\n  - [X] first\n  - [ ] second\n",
		"* Done\n:PROPERTIES:\n:TRELLOID: l2\n:END:\n** DONE Design\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Archived") {
		t.Errorf("an archived card was written")
	}
	if strings.Index(got, "* Doing") > strings.Index(got, "* Done") {
		t.Errorf("lists are out of board order")
	}
}

func TestUpdateWritesTheFileOnlyWhenTheBoardChanged(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/1/boards/b1" || r.URL.Query().Get("key") != "k" || r.URL.Query().Get("token") != "t" {
			http.Error(w, "bad request", 400)
			return
		}
		w.Write([]byte(fixture))
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "sub", "board.org")
	tr := &Trello{Key: "k", Token: "t", Endpoint: srv.URL, Boards: []Board{{Id: "b1", File: path}}}
	tr.Update(nil)
	st1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("not written: %v", err)
	}
	os.Chtimes(path, st1.ModTime().Add(-time.Hour), st1.ModTime().Add(-time.Hour))
	before, _ := os.Stat(path)
	tr.Update(nil)
	after, _ := os.Stat(path)
	if calls != 2 {
		t.Errorf("calls: %d", calls)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("an unchanged board rewrote its file")
	}
}
