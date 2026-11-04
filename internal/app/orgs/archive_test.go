package orgs

import (
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
