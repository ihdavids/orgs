package orgs

import (
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func TestLatexStarterReads(t *testing.T) {
	user, probs := common.ParseLatexSnippets(latexStarter, "mine.org")
	if len(probs) > 0 || len(user) != 3 {
		t.Fatalf("starter: %+v %+v", user, probs)
	}
	m := common.MergeLatexSnippets(user, common.LatexBuiltinSnippets())
	if a := latexExpandText(m, `\( a ooo`, "", true, true); a.Found {
		t.Errorf("ooo should be off: %+v", a)
	}
	if a := latexExpandText(m, `\( ip`, "", true, true); !a.Found || a.Text != `\langle ,  \rangle` || a.Mode != "inline" {
		t.Errorf("ip: %+v", a)
	}
	if a := latexExpandText(m, `\( a/`, "", true, true); !a.Found || a.Body != `\frac{a}{$1}$0` {
		t.Errorf("fraction: %+v", a)
	}
	if a := latexExpandText(m, `\( a/`, "", true, false); a.Found {
		t.Errorf("fraction=0: %+v", a)
	}
}
