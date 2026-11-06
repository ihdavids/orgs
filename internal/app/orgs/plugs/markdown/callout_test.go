package markdown

import (
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestCalloutsBackToMarkdown(t *testing.T) {
	src := "* A\n#+begin_warning Mind the gap :fold closed\nText here.\n#+end_warning\n#+begin_note\nSpeak.\n#+end_note\n" + "#+begin_callout Run :type example :fold open\none\n#+end_callout\n"
	hasNot(t, md(t, src, nil), "[!warning]")
	common.CalloutsOn = func() bool { return true }
	defer func() { common.CalloutsOn = nil }()
	out := md(t, src, nil)
	has(t, out, "> [!warning]- Mind the gap\n> Text here.\n")
	has(t, out, "> [!note]\n> Speak.\n")
	has(t, out, "> [!example]+ Run\n> one\n")
}
