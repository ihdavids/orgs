package orgs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// A file, loaded into a database of its own, and the sections registered so
// they can be addressed by outline path.
func loadOrg(t *testing.T, files map[string]string) map[string]string {
	t.Helper()
	// The parser only knows a keyword the server has told it about, and
	// without one `** TODO Beta` is a heading *called* "TODO Beta" - which an
	// outline path would then have to spell out to find it.
	if Conf().Server != nil && Conf().Server.DefaultTodoStates == "" {
		Conf().Server.DefaultTodoStates = "TODO | DONE CANCELLED"
	}
	dir := t.TempDir()
	fresh := NewOrgDb()
	odb = fresh
	paths := map[string]string{}
	for name, text := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
		paths[name] = p
		fresh.LoadFile(p, true)
		if fresh.FindByFile(p) == nil {
			t.Fatalf("%s did not load", name)
		}
	}
	return paths
}

func readOrg(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return strings.TrimRight(string(b), "\n")
}

func olpTarget(path string, olp ...string) common.Target {
	return common.Target{Type: "file+olp", Filename: path, Id: strings.Join(olp, "::")}
}

func wantOrg(t *testing.T, label, got, want string) {
	t.Helper()
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Errorf("%s:\n--- got ---\n%s\n--- want ---\n%s", label, got, want)
	}
}

const refileFixture = `* Inbox
** TODO Beta
* Projects
** Kitchen
*** TODO Alpha
*** TODO Gamma
**** TODO Gamma child
* Someday
`

// Refiling a heading to a destination **above it in the same file** used to
// take the headings around it with it.
//
// `InsertSection` rewrites the file, so every row below the insertion point
// moves; and `formatHeadingAt` renumbers the source's headline in place while
// placing the copy, because `CopySection` shares the `*org.Headline`. The
// delete then measured the subtree from stale rows at the destination's depth
// and ran straight through the source's siblings. Refiling `Alpha` out of
// `Projects/Kitchen` into `Inbox` deleted `Kitchen`, `Gamma` and `Gamma child`
// as well - and reported success.
func TestRefileUpwardKeepsTheSiblingsBelowIt(t *testing.T) {
	paths := loadOrg(t, map[string]string{"a.org": refileFixture})
	p := paths["a.org"]

	from := olpTarget(p, "Projects", "Kitchen", "Alpha")
	to := olpTarget(p, "Inbox")
	res, err := Refile(db, &common.Refile{FromId: from, ToId: to}, nil, false)
	if err != nil || !res.Ok {
		t.Fatalf("refile failed: %v %q", err, res.Msg)
	}

	wantOrg(t, "upward refile", readOrg(t, p), `* Inbox
** TODO Beta
** TODO Alpha
* Projects
** Kitchen
*** TODO Gamma
**** TODO Gamma child
* Someday`)
}

// The other half of the same bug: a heading refiled into a *different* file
// still had its level changed underneath it, so the delete measured its subtree
// at the destination's depth even though no row had moved.
func TestRefileToAnotherFileLeavesTheRestOfTheSourceAlone(t *testing.T) {
	paths := loadOrg(t, map[string]string{
		"a.org": refileFixture,
		"b.org": "* Target\n* Keep me\n** TODO Do not touch\n",
	})
	a, b := paths["a.org"], paths["b.org"]

	res, err := Refile(db, &common.Refile{
		FromId: olpTarget(a, "Projects", "Kitchen", "Alpha"),
		ToId:   olpTarget(b, "Target"),
	}, nil, false)
	if err != nil || !res.Ok {
		t.Fatalf("refile failed: %v %q", err, res.Msg)
	}

	wantOrg(t, "source file", readOrg(t, a), `* Inbox
** TODO Beta
* Projects
** Kitchen
*** TODO Gamma
**** TODO Gamma child
* Someday`)
	wantOrg(t, "destination file", readOrg(t, b), `* Target
** TODO Alpha
* Keep me
** TODO Do not touch`)
}

// `formatHeading` used to recurse over `sec.Children` on top of the recursion
// `WriteHeadline` already does, so a subtree was written once per level: a
// child twice, a grandchild three times.
func TestRefileWritesASubtreeExactlyOnce(t *testing.T) {
	paths := loadOrg(t, map[string]string{"a.org": refileFixture})
	p := paths["a.org"]

	res, err := Refile(db, &common.Refile{
		FromId: olpTarget(p, "Projects", "Kitchen", "Gamma"),
		ToId:   olpTarget(p, "Inbox"),
	}, nil, false)
	if err != nil || !res.Ok {
		t.Fatalf("refile failed: %v %q", err, res.Msg)
	}

	got := readOrg(t, p)
	if n := strings.Count(got, "Gamma child"); n != 1 {
		t.Errorf("Gamma child written %d times, want 1:\n%s", n, got)
	}
	wantOrg(t, "subtree refile", got, `* Inbox
** TODO Beta
** TODO Gamma
*** TODO Gamma child
* Projects
** Kitchen
*** TODO Alpha
* Someday`)
}

// A copy leaves the original where it is, and the copy carries no `:ID:` of its
// own - two headings answering to one id is a link that resolves to whichever
// of them the database registered last.
func TestCopyLeavesTheOriginalAndTakesNoId(t *testing.T) {
	paths := loadOrg(t, map[string]string{"a.org": `* Inbox
** TODO Beta
   :PROPERTIES:
   :ID: keep-me
   :END:
* Someday
`})
	p := paths["a.org"]

	res, err := Copy(db, &common.Refile{
		FromId: olpTarget(p, "Inbox", "Beta"),
		ToId:   olpTarget(p, "Someday"),
	}, false)
	if err != nil || !res.Ok {
		t.Fatalf("copy failed: %v %q", err, res.Msg)
	}

	got := readOrg(t, p)
	if n := strings.Count(got, "TODO Beta"); n != 2 {
		t.Errorf("want the heading in both places, got %d:\n%s", n, got)
	}
	if n := strings.Count(got, "keep-me"); n != 1 {
		t.Errorf("the id was copied as well (%d occurrences):\n%s", n, got)
	}
}

// A batch addresses each heading by outline path, one at a time, re-reading
// the file in between - because a hash does not survive the file being written
// to. All three have to arrive, and nothing around them may go.
func TestMoveBatchRefilesEveryHeading(t *testing.T) {
	paths := loadOrg(t, map[string]string{"a.org": refileFixture})
	p := paths["a.org"]

	res, _ := Move(db, &common.MoveRequest{
		Op: common.MoveRefile,
		From: []common.Target{
			olpTarget(p, "Projects", "Kitchen", "Alpha"),
			olpTarget(p, "Projects", "Kitchen", "Gamma"),
			olpTarget(p, "Inbox", "Beta"),
		},
		To: olpTarget(p, "Someday"),
	})
	if res.Failed != 0 || res.Done != 3 {
		t.Fatalf("want 3 done and 0 failed, got %d/%d/%d: %+v",
			res.Done, res.Failed, res.Skipped, res.Results)
	}

	wantOrg(t, "batch refile", readOrg(t, p), `* Inbox
* Projects
** Kitchen
* Someday
** TODO Alpha
** TODO Gamma
*** TODO Gamma child
** TODO Beta`)
}

// A heading whose ancestor is in the same batch is not moved on its own: the
// ancestor takes it, and moving it as well would be moving it out of the thing
// that has just moved. That is a skip rather than a failure.
func TestMoveBatchSkipsADescendantOfAnotherSource(t *testing.T) {
	paths := loadOrg(t, map[string]string{"a.org": refileFixture})
	p := paths["a.org"]

	res, _ := Move(db, &common.MoveRequest{
		Op: common.MoveRefile,
		From: []common.Target{
			olpTarget(p, "Projects", "Kitchen", "Gamma"),
			olpTarget(p, "Projects", "Kitchen", "Gamma", "Gamma child"),
		},
		To: olpTarget(p, "Someday"),
	})
	if res.Done != 1 || res.Skipped != 1 || res.Failed != 0 {
		t.Fatalf("want 1 done, 1 skipped, 0 failed, got %d/%d/%d: %+v",
			res.Done, res.Skipped, res.Failed, res.Results)
	}

	got := readOrg(t, p)
	if n := strings.Count(got, "Gamma child"); n != 1 {
		t.Errorf("Gamma child appears %d times, want 1:\n%s", n, got)
	}
}

// An archive file sits beside the file it came out of, whatever name that
// file was loaded by. %s used to be filled in with the whole path, so a file
// loaded as arch/a.org archived into arch/arch/a.org_archive.
func TestTheArchiveFileIsBesideItsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "arch"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "arch", "a.org"), []byte("* DONE One\n"), 0644); err != nil {
		t.Fatal(err)
	}
	was, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(was)
	old := Conf().ArchiveDefaultTarget
	Conf().ArchiveDefaultTarget = "%s_archive::"
	defer func() { Conf().ArchiveDefaultTarget = old }()

	fresh := NewOrgDb()
	odb = fresh
	fresh.LoadFile("arch/a.org", true)
	got := FindArchiveTarget(db, &common.Target{Type: "file+olp", Filename: "arch/a.org", Id: "One"})
	if got == nil {
		t.Fatal("no archive target")
	}
	want, _ := filepath.Abs(filepath.Join("arch", "a.org_archive"))
	if got.Filename != want {
		t.Errorf("archive file: got %s, want %s", got.Filename, want)
	}
}
