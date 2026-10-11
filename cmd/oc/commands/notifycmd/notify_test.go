package notifycmd

import (
	"testing"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/desktop"
)

// A link the server made for its own machine is moved onto the address this
// client reaches it at; a link already elsewhere, or -open, is respected.
func TestRebase(t *testing.T) {
	link := "http://localhost:8010/?app=Files&open=%2Fn%2Fa.org&line=3"
	if got := rebase(link, "", "https://box.lan:8443"); got != "https://box.lan:8443/?app=Files&open=%2Fn%2Fa.org&line=3" {
		t.Fatalf("server address: %s", got)
	}
	if got := rebase(link, "http://orgs.example", "https://box.lan:8443"); got != "http://orgs.example/?app=Files&open=%2Fn%2Fa.org&line=3" {
		t.Fatalf("-open: %s", got)
	}
	if got := rebase("https://orgs.example/x", "", "https://box.lan:8443"); got != "https://orgs.example/x" {
		t.Fatalf("a real address is kept: %s", got)
	}
	if rebase("", "", "https://box") != "" {
		t.Fatal("no link stays no link")
	}
}

func TestNoteFromEvent(t *testing.T) {
	n := noteFromEvent(common.OrgEvent{Kind: "notify", Title: "Standup", Msg: "in 2 minutes\n10:00", Urgency: "critical", Sound: true, Group: "remind:x"}, "", "http://box")
	if n.Title != "Standup" || n.Body != "in 2 minutes\n10:00" || n.Urgency != desktop.Critical || !n.Sound || n.Group != "remind:x" {
		t.Fatalf("note: %+v", n)
	}
}
