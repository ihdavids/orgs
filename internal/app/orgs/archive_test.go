package orgs

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestArchiveKeepsOrgsContext(t *testing.T) {
	paths := loadOrg(t, map[string]string{"work.org": `#+FILETAGS: :office:
* Projects                                              :proj:
:PROPERTIES:
:CATEGORY: clients
:END:
** Acme
*** DONE Send invoice                                   :money:
Paid.
`})
	p := paths["work.org"]
	old, oldInfo := Conf().ArchiveDefaultTarget, Conf().ArchiveSaveContextInfo
	Conf().ArchiveDefaultTarget = "%s_archive::"
	Conf().ArchiveSaveContextInfo = []string{"time", "file", "olpath", "category", "todo", "itags"}
	t.Cleanup(func() { Conf().ArchiveDefaultTarget, Conf().ArchiveSaveContextInfo = old, oldInfo })
	res, err := Move(db, &common.MoveRequest{Op: common.MoveArchive,
		From: []common.Target{{Type: "file+olp", Filename: p, Id: "Projects::Acme::Send invoice"}}})
	if err != nil || !res.Ok {
		t.Fatalf("archive: %v %+v", err, res)
	}
	got := readOrg(t, p+"_archive")
	for _, want := range []string{
		":ARCHIVE_FILE: " + abbreviateHome(p),
		":ARCHIVE_OLPATH: Projects/Acme",
		":ARCHIVE_CATEGORY: clients",
		":ARCHIVE_TODO: DONE",
		":ARCHIVE_ITAGS: proj office",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if !regexp.MustCompile(`:ARCHIVE_TIME: +\d{4}-\d\d-\d\d \w{3} \d\d:\d\d\n`).MatchString(got) {
		t.Errorf("ARCHIVE_TIME not org's form:\n%s", got)
	}
	if strings.Contains(readOrg(t, p), "Send invoice") {
		t.Errorf("still in the file")
	}
}

func TestArchiveSiblingMakesAndReusesTheSibling(t *testing.T) {
	paths := loadOrg(t, map[string]string{"s.org": "* Project\n** DONE One\n** TODO Two\n** DONE Three\n* Other\n"})
	p := paths["s.org"]
	for _, name := range []string{"One", "Three"} {
		res, err := Move(db, &common.MoveRequest{Op: common.MoveArchiveSibling,
			From: []common.Target{{Type: "file+olp", Filename: p, Id: "Project::" + name}}})
		if err != nil || !res.Ok {
			t.Fatalf("archive %s: %v %+v", name, err, res)
		}
	}
	got := readOrg(t, p)
	if strings.Count(got, "Archive  :ARCHIVE:") != 1 {
		t.Fatalf("one archive sibling wanted:\n%s", got)
	}
	re := regexp.MustCompile(`(?s)^\* Project\n\*\* TODO Two\n\*\* Archive  :ARCHIVE:\n\*\*\* DONE One\n.*:ARCHIVE_TIME:.*\*\*\* DONE Three\n.*:ARCHIVE_TIME:.*\n\* Other$`)
	if !re.MatchString(got) {
		t.Errorf("layout:\n%s", got)
	}
}

func TestArchivePlace(t *testing.T) {
	paths := loadOrg(t, map[string]string{
		"w.org": "#+ARCHIVE: %s_old::* Done work\n* Projects\n** Acme\n:PROPERTIES:\n:ARCHIVE: elsewhere.org::\n:END:\n* Inbox\n",
	})
	p := paths["w.org"]
	old := Conf().ArchiveDefaultTarget
	Conf().ArchiveDefaultTarget = "%s_archive::"
	t.Cleanup(func() { Conf().ArchiveDefaultTarget = old })
	ask := func(q url.Values) ArchivePlace {
		return archivePlaceOf(httptest.NewRequest("GET", "/archive/where?"+q.Encode(), nil))
	}

	// Nothing archived yet: the file is named, but it is not there.
	got := ask(url.Values{"filename": {p}})
	if got.Ok || got.Exists || got.Filename != p+"_old" || got.Heading != "Done work" || got.From != "file" {
		t.Fatalf("before: %+v", got)
	}
	if err := os.WriteFile(p+"_old", []byte("#+TITLE: old\n\n* Done work  :x:\n** One\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = ask(url.Values{"filename": {p}, "line": {"7"}}) // Inbox: the file's rule
	if !got.Ok || !got.Exists || got.Line != 2 || got.SameFile {
		t.Errorf("file rule: %+v", got)
	}
	// A heading's own ARCHIVE wins, and names no heading.
	got = ask(url.Values{"filename": {p}, "line": {"2"}})
	if got.From != "heading" || got.Filename != filepath.Join(filepath.Dir(p), "elsewhere.org") || got.Heading != "" {
		t.Errorf("heading rule: %+v", got)
	}
}
