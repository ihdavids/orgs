package orgs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs/tangle"
	"github.com/ihdavids/orgs/internal/common"
)

const literate = `#+TITLE: Setup
#+PROPERTY: header-args :comments link
* Install
  #+BEGIN_SRC sh :tangle install.sh
    ,* a star line
    brew install rg
  #+END_SRC
* Config
#+NAME: conf
#+BEGIN_SRC sh :tangle install.sh
echo configured
#+END_SRC
#+BEGIN_SRC sh :noweb-ref greet
echo hi
#+END_SRC
#+BEGIN_SRC sh :tangle greet.sh :noweb yes
<<greet>>
#+END_SRC
`

// A private ledger and home, so the tests neither read nor write the real one.
func tangleHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := Conf().PlugManager.HomeDir
	Conf().PlugManager.HomeDir = dir
	tangles = &tangleLedger{written: map[string]string{}, timers: map[string]*time.Timer{}}
	t.Cleanup(func() { Conf().PlugManager.HomeDir = old })
}

func tangleNow(t *testing.T, p string) {
	t.Helper()
	if _, err := tangle.Tangle(db, p, true); err != nil {
		t.Fatal(err)
	}
}

func TestDetangleRoundTrip(t *testing.T) {
	tangleHome(t)
	paths := loadOrg(t, map[string]string{"setup.org": literate})
	p := paths["setup.org"]
	tangleNow(t, p)
	sh := filepath.Join(filepath.Dir(p), "install.sh")
	got, _ := os.ReadFile(sh)
	want := "# [[file:setup.org::*Install][Install:1]]\n* a star line\nbrew install rg\n# Install:1 ends here\n\n" +
		"# [[file:setup.org::*Config][conf]]\necho configured\n# conf ends here\n"
	if string(got) != want {
		t.Fatalf("tangled\n%q\nwant\n%q", got, want)
	}

	// Edit the tangled file and put it back.
	edited := strings.Replace(string(got), "brew install rg", "brew install rg fd\n* another star", 1)
	edited = strings.Replace(edited, "echo configured", "echo configured twice", 1)
	dry := Detangle(&common.DetangleRequest{File: sh, Text: edited, DryRun: true})
	if !dry.Ok || len(dry.Written) != 0 {
		t.Fatalf("dry run: %+v", dry)
	}
	before, _ := os.ReadFile(p)
	if !strings.Contains(string(before), "    brew install rg\n  #+END_SRC") {
		t.Fatalf("a dry run wrote the file")
	}
	res := Detangle(&common.DetangleRequest{File: sh, Text: edited})
	if !res.Ok || len(res.Written) != 1 {
		t.Fatalf("detangle: %+v", res)
	}
	org, _ := os.ReadFile(p)
	for _, w := range []string{
		"  #+BEGIN_SRC sh :tangle install.sh\n    ,* a star line\n    brew install rg fd\n    ,* another star\n  #+END_SRC",
		"#+BEGIN_SRC sh :tangle install.sh\necho configured twice\n#+END_SRC",
	} {
		if !strings.Contains(string(org), w) {
			t.Errorf("missing\n%s\nin\n%s", w, org)
		}
	}
	// Tangling again gives back exactly the edited file.
	tangleNow(t, p)
	again, _ := os.ReadFile(sh)
	if string(again) != edited {
		t.Errorf("round trip\n%q\nwant\n%q", again, edited)
	}
	// And detangling that changes nothing.
	if res := Detangle(&common.DetangleRequest{File: sh, Text: edited}); len(res.Written) != 0 {
		t.Errorf("an unchanged detangle wrote: %+v", res)
	}
}

func TestDetangleLeavesNowebAlone(t *testing.T) {
	tangleHome(t)
	paths := loadOrg(t, map[string]string{"setup.org": literate})
	p := paths["setup.org"]
	tangleNow(t, p)
	g := filepath.Join(filepath.Dir(p), "greet.sh")
	text, _ := os.ReadFile(g)
	res := Detangle(&common.DetangleRequest{File: g, Text: strings.Replace(string(text), "echo hi", "echo HI", 1)})
	if len(res.Blocks) != 1 || res.Blocks[0].Status != "skipped" || len(res.Written) != 0 {
		t.Errorf("%+v", res)
	}
}

func TestDetangleNeedsMarkers(t *testing.T) {
	if res := Detangle(&common.DetangleRequest{File: "/tmp/x.sh", Text: "echo hi\n"}); res.Ok || !strings.Contains(res.Msg, ":comments link") {
		t.Errorf("%+v", res)
	}
	if res := Detangle(&common.DetangleRequest{File: "/etc/passwd"}); res.Ok {
		t.Errorf("read a file outside the org directories: %+v", res)
	}
}

func TestAutoTangleGuardsHandEdits(t *testing.T) {
	tangleHome(t)
	src := "#+auto_tangle: t\n* Code\n#+BEGIN_SRC sh :tangle out.sh\necho one\n#+END_SRC\n"
	paths := loadOrg(t, map[string]string{"auto.org": src})
	p := paths["auto.org"]
	out := filepath.Join(filepath.Dir(p), "out.sh")

	if r := AutoTangle(p); !r.Ok || len(r.Written) != 1 {
		t.Fatalf("first: %+v", r)
	}
	// Saved again with nothing new: nothing written.
	if r := AutoTangle(p); !r.Ok || len(r.Written) != 0 {
		t.Errorf("unchanged: %+v", r)
	}
	// The org file changes: the tangled file follows.
	os.WriteFile(p, []byte(strings.Replace(src, "echo one", "echo two", 1)), 0644)
	GetDb().ReloadFile(p)
	time.Sleep(10 * time.Millisecond)
	if r := AutoTangle(p); !r.Ok || len(r.Written) != 1 {
		t.Errorf("after an org edit: %+v", r)
	}
	// Somebody edits the tangled file by hand, then the org file changes.
	os.WriteFile(out, []byte("echo by hand\n"), 0644)
	os.WriteFile(p, []byte(strings.Replace(src, "echo one", "echo three", 1)), 0644)
	GetDb().ReloadFile(p)
	r := AutoTangle(p)
	if r.Ok || len(r.Conflicts) != 1 {
		t.Errorf("hand edit should stop it: %+v", r)
	}
	if b, _ := os.ReadFile(out); string(b) != "echo by hand\n" {
		t.Errorf("a hand edit was overwritten: %q", b)
	}
	// A file without #+auto_tangle: is never tangled by a save.
	plain := loadOrg(t, map[string]string{"plain.org": "* C\n#+BEGIN_SRC sh :tangle p.sh\necho\n#+END_SRC\n"})["plain.org"]
	autoTangleAfterLoad(GetDb().FindByFile(plain))
	time.Sleep(600 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(filepath.Dir(plain), "p.sh")); err == nil {
		t.Errorf("a file that did not ask was tangled")
	}
}
