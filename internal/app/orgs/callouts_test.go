package orgs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCalloutsOfFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.org")
	os.WriteFile(p, []byte("#+begin_info Top\nx\n#+end_info\n* Travel\n** Trains\n#+begin_warning Mind the gap :fold closed\ny\n#+end_warning\n#+begin_recipe Soup :fold open\nz\n#+end_recipe\n"), 0o644)
	got := calloutsOfFile(p)
	if len(got) != 3 {
		t.Fatalf("want 3, got %+v", got)
	}
	if got[0].Heading != "" || got[0].Shown != "Top" {
		t.Errorf("first: %+v", got[0])
	}
	if got[1].Heading != "Trains" || got[1].Type != "warning" || got[1].Fold != "closed" || got[1].Line != 6 || got[1].EndLine != 8 {
		t.Errorf("second: %+v", got[1])
	}
	if got[2].Unknown != "recipe" || got[2].Type != "note" {
		t.Errorf("third: %+v", got[2])
	}
}
