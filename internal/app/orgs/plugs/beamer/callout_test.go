package beamer

import (
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestCalloutsInBeamer(t *testing.T) {
	src := "* Slide\n#+begin_warning Mind the gap\nText here.\n#+end_warning\n#+begin_tip\nA tip.\n#+end_tip\n#+begin_note\nSpeak.\n#+end_note\n"
	mustNotHave(t, render(t, src, Options{}), "alertblock")
	common.CalloutsOn = func() bool { return true }
	defer func() { common.CalloutsOn = nil }()
	tex := render(t, src, Options{})
	mustHave(t, tex, "\\begin{alertblock}{Mind the gap}", "\\begin{exampleblock}{Tip}", "\\note{")
}
