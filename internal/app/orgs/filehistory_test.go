package orgs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A throwaway repository: one org file, three commits, then an uncommitted
// edit and a staged one.
func historyRepo(t *testing.T) (dir, file string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir = t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := gitIn(dir, args...); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "Tester")
	run("config", "commit.gpgsign", "false")
	file = filepath.Join(dir, "notes.org")
	write := func(s string) {
		if err := os.WriteFile(file, []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("* TODO one\n")
	run("add", "notes.org")
	run("commit", "-q", "-m", "first")
	// A commit that does not touch the file must not be in its history.
	os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x"), 0644)
	run("add", "other.txt")
	run("commit", "-q", "-m", "unrelated")
	write("* DONE one\n* TODO two\n")
	run("commit", "-q", "-am", "second: done | two")
	write("* DONE one\n* TODO two\n* three\n")
	run("add", "notes.org")
	write("* DONE one\n* TODO two\n* three\n* four\n")
	return dir, file
}

func TestFileHistory(t *testing.T) {
	_, file := historyRepo(t)
	h := fileHistory(file, 0, false)
	if !h.Ok || !h.InRepo {
		t.Fatalf("not read: %+v", h)
	}
	if h.Path != "notes.org" || h.Branch != "main" {
		t.Errorf("path %q branch %q", h.Path, h.Branch)
	}
	if len(h.Commits) != 2 {
		t.Fatalf("want the 2 commits touching the file, got %d: %+v", len(h.Commits), h.Commits)
	}
	newest, oldest := h.Commits[0], h.Commits[1]
	if newest.Subject != "second: done | two" || oldest.Subject != "first" {
		t.Errorf("subjects %q %q", newest.Subject, oldest.Subject)
	}
	// The unrelated commit is skipped: the parent is rewritten past it.
	if len(newest.Parents) != 1 || newest.Parents[0] != oldest.Hash {
		t.Errorf("parents %v, want [%s]", newest.Parents, oldest.Hash)
	}
	if newest.Added != 2 || newest.Removed != 1 {
		t.Errorf("numstat +%d -%d, want +2 -1", newest.Added, newest.Removed)
	}
	if !strings.Contains(strings.Join(newest.Refs, ","), "main") {
		t.Errorf("refs %v", newest.Refs)
	}
	if !h.Staged || h.StagedAdd != 1 || !h.Modified || h.ModifiedAdd != 1 {
		t.Errorf("uncommitted state %+v", h)
	}
	if h.Last != newest.Hash {
		t.Errorf("last %q, want %q", h.Last, newest.Hash)
	}
	if h.Truncated {
		t.Error("truncated")
	}
	if one := fileHistory(file, 1, false); !one.Truncated || len(one.Commits) != 1 {
		t.Errorf("limit 1: %d commits, truncated %v", len(one.Commits), one.Truncated)
	}
}

func TestFileAtRev(t *testing.T) {
	_, file := historyRepo(t)
	h := fileHistory(file, 0, false)
	cases := map[string]string{
		"":                 "* DONE one\n* TODO two\n* three\n* four\n",
		revWorking:         "* DONE one\n* TODO two\n* three\n* four\n",
		revIndex:           "* DONE one\n* TODO two\n* three\n",
		"HEAD":             "* DONE one\n* TODO two\n",
		h.Commits[1].Hash:  "* TODO one\n",
		h.Commits[1].Short: "* TODO one\n",
	}
	for rev, want := range cases {
		got := fileAt(file, rev)
		if !got.Ok || got.Text != want {
			t.Errorf("rev %q: ok %v msg %q text %q, want %q", rev, got.Ok, got.Msg, got.Text, want)
		}
	}
	for _, bad := range []string{"--output=/tmp/x", "HEAD..main", "a b", "$(x)"} {
		if got := fileAt(file, bad); got.Ok {
			t.Errorf("rev %q accepted", bad)
		}
	}
}

func TestFileAtRevMissing(t *testing.T) {
	dir, _ := historyRepo(t)
	// Before the file existed: an empty side, not a failure.
	os.WriteFile(filepath.Join(dir, "late.org"), []byte("* late\n"), 0644)
	got := fileAt(filepath.Join(dir, "late.org"), "HEAD")
	if !got.Ok || !got.Missing {
		t.Errorf("a file not in HEAD: %+v", got)
	}
}

func TestFileHistoryOutsideGit(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "loose.org")
	os.WriteFile(f, []byte("* x\n"), 0644)
	h := fileHistory(f, 0, false)
	if !h.Ok || h.InRepo {
		t.Errorf("outside git: %+v", h)
	}
	if got := fileAt(f, ""); got.Text != "* x\n" {
		t.Errorf("working copy %q", got.Text)
	}
}
