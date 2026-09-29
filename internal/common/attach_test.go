package common

import (
	"path/filepath"
	"strings"
	"testing"
)

// The folder arithmetic is what makes an orgs attachment store readable by
// emacs and the other way round, and it is shared between the server and the
// exporters - so if it is wrong it is wrong in two places at once and they
// still agree with each other.

func TestAttachIdPathSplitsTwoDeep(t *testing.T) {
	// org-attach-id-uuid-folder-format: the first two characters, then the rest.
	got := AttachIdPath("8f3c1a20-1234-4321-9999-abcdefabcdef")
	want := filepath.Join("8f", "3c1a20-1234-4321-9999-abcdefabcdef")
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// A flat folder is what the split exists to avoid, but an id too short to split
// must still land somewhere rather than in a folder called "".
func TestAttachIdPathSurvivesAShortId(t *testing.T) {
	for _, id := range []string{"", "a", "ab", "abc"} {
		got := AttachIdPath(id)
		if strings.HasPrefix(got, string(filepath.Separator)) {
			t.Errorf("%q gave an absolute path %q", id, got)
		}
		if id == "abc" && got != filepath.Join("ab", "c") {
			t.Errorf("abc should still split, got %q", got)
		}
		if len(id) < 3 && got != id {
			t.Errorf("%q should be left alone, got %q", id, got)
		}
	}
}

func TestAttachRootDefaultsToData(t *testing.T) {
	root := AttachRootIn([]string{"/org"}, AttachSettings{})
	if filepath.Base(root) != "data" {
		t.Errorf("expected data under the org dir, got %q", root)
	}
	if !filepath.IsAbs(root) {
		t.Errorf("the root should be absolute, got %q", root)
	}
}

func TestAttachRootFollowsTheSetting(t *testing.T) {
	root := AttachRootIn([]string{"/org"}, AttachSettings{Dir: "attachments"})
	if filepath.Base(root) != "attachments" {
		t.Errorf("got %q", root)
	}
	// An absolute setting is taken as it stands - an attachment store on
	// another disk is a reasonable thing to want.
	abs := AttachRootIn([]string{"/org"}, AttachSettings{Dir: "/mnt/big/attach"})
	if abs != filepath.Clean("/mnt/big/attach") {
		t.Errorf("an absolute dir should be left alone, got %q", abs)
	}
}

// A :DIR: property wins outright and is read relative to the org file, which is
// what org does: that property is written by a person, beside the file.
func TestAttachDirPropertyWinsAndIsRelativeToTheFile(t *testing.T) {
	dir, from, id := AttachDirFrom("shed-photos", "some-id", "/org/projects/shed.org", []string{"/org"}, AttachSettings{})
	if from != "dir" {
		t.Errorf("expected the property to win, got %q", from)
	}
	if id != "" {
		t.Errorf("a heading with a :DIR: needs no id, got %q", id)
	}
	if dir != filepath.Clean("/org/projects/shed-photos") {
		t.Errorf("got %q", dir)
	}
}

// The older name for the same thing. Files written years ago still say it, and
// the caller passes whichever it found.
func TestAttachDirTakesTheOlderPropertyToo(t *testing.T) {
	dir, from, _ := AttachDirFrom("old-files", "", "/org/notes.org", []string{"/org"}, AttachSettings{})
	if from != "dir" || dir != filepath.Clean("/org/old-files") {
		t.Errorf("got %q %q", dir, from)
	}
}

func TestAttachDirFromId(t *testing.T) {
	dir, from, id := AttachDirFrom("", "8f3c1a20-plan", "/org/notes.org", []string{"/org"}, AttachSettings{})
	if from != "id" {
		t.Errorf("expected the id to answer, got %q", from)
	}
	if id != "8f3c1a20-plan" {
		t.Errorf("got %q", id)
	}
	want := filepath.Join(filepath.Clean("/org"), "data", "8f", "3c1a20-plan")
	if dir != want {
		t.Errorf("got %q want %q", dir, want)
	}
}

// A heading with neither owns nothing yet, and that is not an error - it is the
// ordinary state of every heading nobody has attached anything to.
func TestAttachDirOfAHeadingThatOwnsNothing(t *testing.T) {
	dir, from, id := AttachDirFrom("", "", "/org/notes.org", []string{"/org"}, AttachSettings{})
	if dir != "" || from != "" || id != "" {
		t.Errorf("expected nothing, got %q %q %q", dir, from, id)
	}
}

func TestAttachDirTakesAnAbsoluteProperty(t *testing.T) {
	dir, from, _ := AttachDirFrom("/mnt/scans", "", "/org/notes.org", []string{"/org"}, AttachSettings{})
	if from != "dir" || dir != filepath.Clean("/mnt/scans") {
		t.Errorf("got %q %q", dir, from)
	}
}

// ---------------------------------------------------------------------------
// The link
// ---------------------------------------------------------------------------

func TestAttachLinkName(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"attachment:report.pdf", "report.pdf", true},
		{"attachment://report.pdf", "report.pdf", true},
		{"attachment:sub/report.pdf", "sub/report.pdf", true},
		// A search part names a place inside the file, not another file.
		{"attachment:notes.org::*Plans", "notes.org", true},
		{"attachment:", "", false},
		{"file:report.pdf", "", false},
		{"https://example.com/report.pdf", "", false},
		{"report.pdf", "", false},
	}
	for _, c := range cases {
		got, ok := AttachLinkName(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%q gave (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// The name comes off a multipart upload, which is to say off whatever was on
// the other end of a share button. Nothing that climbs out of the folder may
// come back as a name.
func TestAttachLinkNameRefusesAnythingThatClimbsOut(t *testing.T) {
	for _, bad := range []string{
		"attachment:../../../etc/passwd",
		"attachment:..",
		"attachment:../secrets",
		"attachment:/etc/passwd",
		"attachment:sub/../../out.txt",
	} {
		if got, ok := AttachLinkName(bad); ok {
			t.Errorf("%q should have been refused, got %q", bad, got)
		}
	}
	// A path that merely *contains* a climb but lands inside is fine, because
	// what matters is where it ends up.
	if got, ok := AttachLinkName("attachment:sub/../report.pdf"); !ok || got != "report.pdf" {
		t.Errorf("got %q %v", got, ok)
	}
}
