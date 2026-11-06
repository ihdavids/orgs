package typst

import (
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestCalloutsInTypst(t *testing.T) {
	src := "* A\n#+begin_warning Mind the gap :fold closed\nText here.\n#+end_warning\n#+begin_note\nSpeak.\n#+end_note\n"
	if off := export(t, parse(t, src, "/tmp/n.org"), nil); !strings.Contains(off, "#org-special(\"warning\")") {
		t.Errorf("off: %s", off)
	}
	common.CalloutsOn = func() bool { return true }
	defer func() { common.CalloutsOn = nil }()
	on := export(t, parse(t, src, "/tmp/n.org"), nil)
	for _, want := range []string{"#block(width: 100%, inset: (x: 10pt, y: 8pt), radius: 3pt, fill: rgb(\"#ec7500\").lighten(90%)", "weight: \"bold\")[Mind the gap]", "Text here."} {
		if !strings.Contains(on, want) {
			t.Errorf("no %q in\n%s", want, on)
		}
	}
}
