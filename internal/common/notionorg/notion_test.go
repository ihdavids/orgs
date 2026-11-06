package notionorg

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fixture reads testdata/export, a hand-made export in Notion's shape.
func fixture(t *testing.T) map[string][]byte {
	t.Helper()
	root := filepath.Join("testdata", "export")
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func want(t *testing.T, name, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("%s missing\n%s\n---- in ----\n%s", name, w, got)
		}
	}
}

func TestConvert(t *testing.T) {
	out := Convert(fixture(t))
	names := []string{}
	for k := range out.Files {
		names = append(names, k)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "Home.org,Home/Garden.org,Home/Reading List.org,Notes 2.org,Notes.org" {
		t.Errorf("files %s", got)
	}
	want(t, "Home.org", out.Files["Home.org"],
		"#+TITLE: Home\n\nWelcome",
		"The [[file:./Home/Garden.org][Garden]] has",
		"in the [[file:./Home/Reading List.org][Reading List]],",
		"and [[file:./Home/Reading List.org::*Dune][Dune]] is",
		"A link by url: [[file:./Home/Reading List.org::*Neuromancer][Neuromancer]].",
		"See also [[file:./Notes.org][Notes]] and [[file:./Notes 2.org][the other notes]].",
		"[[https://go.dev][Go]]",
		// The body's own level-one heading stays a heading, not the title.
		"\n* Plans\n", "\n** Later\n")
	// A block id after # is dropped: org has no way to name it.
	want(t, "Notes.org", out.Files["Notes.org"], "#+TITLE: Notes\n", "Back [[file:./Home.org][home]].")
	want(t, "Garden.org", out.Files["Home/Garden.org"], "[[file:./Garden/tomato.png]]", "[[file:../Home.org][Home]]")

	rl := out.Files["Home/Reading List.org"]
	want(t, "Reading List.org", rl,
		"#+TITLE: Reading List\n\n* DONE Dune :sci_fi:classic:\nDEADLINE: <2026-10-03 Sat>\n:PROPERTIES:\n:RECORD: reading-list\n:STATUS: Done\n",
		// The _all CSV is the one read: the plain one has no Author.
		":READ_ON: <2026-10-03 Sat>--<2026-10-05 Mon>\n:AUTHOR: Frank Herbert\n",
		":RELATED: [[*Neuromancer][Neuromancer]]\n",
		":CREATED: [2026-10-01 Thu 09:15]\n",
		":COVER: [[file:./Reading List/Dune/cover.png]]\n",
		// A key the row page has and the CSV lacks.
		":MOOD: epic\n:END:\n\nMy notes, next to [[*Neuromancer][Neuromancer]].",
		"[[file:./Reading List/Dune/cover.png]]\n\n** Quotes\n\nFear is the mind-killer.",
		// A range is not a deadline; it stays a property.
		"* TODO Neuromancer :cyberpunk:\n:PROPERTIES:\n:RECORD: reading-list\n:STATUS: In progress\n:DUE: <2026-11-01 Sun 10:00-11:30>\n",
		":RELATED: [[*Dune][Dune]]\n",
		"\nThe sky above the port.\n",
		"* TODO Hyperion\n:PROPERTIES:\n:RECORD: reading-list\n:STATUS: Not started\n:AUTHOR: Dan Simmons\n")
	// The row page's own property lines are not left in its body.
	if strings.Contains(rl, "Status: Done") || strings.Contains(rl, "Author: William") {
		t.Errorf("property lines left in a body:\n%s", rl)
	}
	assets := []string{}
	for a := range out.Assets {
		assets = append(assets, a)
	}
	sort.Strings(assets)
	if strings.Join(assets, ",") != "Home/Garden/tomato.png,Home/Reading List/Dune/cover.png" || string(out.Assets["Home/Garden/tomato.png"]) != "PNGtomato" {
		t.Errorf("assets %v", assets)
	}
	if out.Stats.Pages != 4 || out.Stats.Databases != 1 || out.Stats.Rows != 3 || out.Stats.Tasks != 3 || out.Stats.Unresolved != 0 {
		t.Errorf("stats %+v", out.Stats)
	}
}

func TestIsExport(t *testing.T) {
	for _, c := range []struct {
		paths []string
		want  bool
	}{
		{[]string{"Home 0123456789abcdef0123456789abcdef.md"}, true},
		{[]string{"x/Tasks 0123456789abcdef0123456789abcdef_all.csv"}, true},
		{[]string{"Home0123456789abcdef0123456789abcdef.md"}, true},
		{[]string{"Home 01234567-89ab-cdef-0123-456789abcdef.md"}, true},
		{[]string{"Home.md", "notes/Garden.md"}, false},
		{[]string{"pic 0123456789abcdef0123456789abcdef.png"}, false},
	} {
		if got := IsExport(c.paths); got != c.want {
			t.Errorf("IsExport(%v) = %v", c.paths, got)
		}
	}
}

func TestOrgDate(t *testing.T) {
	for _, c := range []struct {
		in       string
		inactive bool
		want     string
	}{
		{"October 3, 2026", false, "<2026-10-03 Sat>"},
		{"@October 3, 2026", false, "<2026-10-03 Sat>"},
		{"October 3, 2026 10:00 AM", false, "<2026-10-03 Sat 10:00>"},
		{"October 3, 2026 14:30", true, "[2026-10-03 Sat 14:30]"},
		{"October 3, 2026 → October 5, 2026", false, "<2026-10-03 Sat>--<2026-10-05 Mon>"},
		{"October 3, 2026 10:00 AM → 11:30 AM", false, "<2026-10-03 Sat 10:00-11:30>"},
		{"October 3, 2026 10:00 AM → October 3, 2026 1:00 PM", false, "<2026-10-03 Sat 10:00-13:00>"},
		{"October 3, 2026 9:00 PM (GMT-7)", false, "<2026-10-03 Sat 21:00>"},
		{"2026-10-03", false, "<2026-10-03 Sat>"},
		{"Yes", false, ""},
		{"Frank Herbert", false, ""},
	} {
		got, ok := OrgDate(c.in, c.inactive)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("OrgDate(%q) = %q, %v; want %q", c.in, got, ok, c.want)
		}
	}
}

// A Status column of states that are not a task's is a property, and a Done
// checkbox is the keyword when there is no Status.
func TestTaskColumns(t *testing.T) {
	files := map[string][]byte{
		"Posts 0123456789abcdef0123456789abcdef.csv": []byte("Name,Status\nHello,Draft\nWorld,Published\n"),
		"Chores 1123456789abcdef0123456789abcdef.csv": []byte("Name,Done,Due Date\nDishes,Yes,\"October 3, 2026\"\nLaundry,No,\"October 4, 2026\"\n"),
	}
	out := Convert(files)
	want(t, "Posts.org", out.Files["Posts.org"], "#+TITLE: Posts\n\n* Hello\n:PROPERTIES:\n:RECORD: posts\n:STATUS: Draft\n:END:\n")
	want(t, "Chores.org", out.Files["Chores.org"],
		"* DONE Dishes\nDEADLINE: <2026-10-03 Sat>\n:PROPERTIES:\n:RECORD: chores\n:END:\n",
		"* TODO Laundry\nDEADLINE: <2026-10-04 Sun>\n")
}

// A zip of part zips, all inside Notion's Export-<uuid>/ folder, reads as
// the folder does.
func TestReadZip(t *testing.T) {
	files := fixture(t)
	names := []string{}
	for k := range files {
		names = append(names, k)
	}
	sort.Strings(names)
	part := func(ns []string) []byte {
		buf := &bytes.Buffer{}
		w := zip.NewWriter(buf)
		for _, n := range ns {
			f, _ := w.Create("Export-7f3c2a10-1111-2222-3333-444455556666/" + n)
			f.Write(files[n])
		}
		w.Close()
		return buf.Bytes()
	}
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	for i, ns := range [][]string{names[:len(names)/2], names[len(names)/2:]} {
		f, _ := w.Create("Export-7f3c2a10-1111-2222-3333-444455556666-Part-" + string(rune('1'+i)) + ".zip")
		f.Write(part(ns))
	}
	f, _ := w.Create("__MACOSX/._junk")
	f.Write([]byte("x"))
	w.Close()
	got, err := ReadZip(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	a, b := Convert(files), Convert(got)
	if len(a.Files) != len(b.Files) || len(a.Assets) != len(b.Assets) {
		t.Fatalf("zip %d files %d assets, folder %d %d", len(b.Files), len(b.Assets), len(a.Files), len(a.Assets))
	}
	for k, v := range a.Files {
		if b.Files[k] != v {
			t.Errorf("%s differs from the zip:\n%s\n----\n%s", k, v, b.Files[k])
		}
	}
}

func TestReadZipRefusesEscapes(t *testing.T) {
	for _, name := range []string{"../evil.md", "a/../../evil.md", "/etc/passwd", `..\evil.md`, "C:/x.md"} {
		buf := &bytes.Buffer{}
		w := zip.NewWriter(buf)
		f, _ := w.CreateHeader(&zip.FileHeader{Name: name})
		f.Write([]byte("x"))
		w.Close()
		if _, err := ReadZip(buf.Bytes()); err == nil {
			t.Errorf("%q was unpacked", name)
		}
	}
}
