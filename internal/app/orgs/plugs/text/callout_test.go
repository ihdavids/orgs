package text

import (
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

func TestCalloutsInPlainText(t *testing.T) {
	src := "* A\n#+begin_warning Mind the gap :fold closed\nText here.\n#+end_warning\n#+begin_note\nSpeak.\n#+end_note\n"
	doc := org.New().Silent().Parse(strings.NewReader(src), "n.org")
	off := (&Exporter{}).Text(doc)
	if strings.Contains(off, "| WARNING") {
		t.Errorf("off: %s", off)
	}
	common.CalloutsOn = func() bool { return true }
	defer func() { common.CalloutsOn = nil }()
	on := (&Exporter{}).Text(org.New().Silent().Parse(strings.NewReader(src), "n.org"))
	for _, want := range []string{"| WARNING: Mind the gap (folded)\n|\n| Text here.\n", "| NOTE\n|\n| Speak.\n"} {
		if !strings.Contains(on, want) {
			t.Errorf("no %q in\n%s", want, on)
		}
	}
}
