package orgs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// events collects what is published while f runs.
func eventsDuring(t *testing.T, kind string, f func()) []Event {
	t.Helper()
	ch := subscribe()
	defer unsubscribe(ch)
	f()
	out := []Event{}
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case e := <-ch:
			if e.Kind == kind {
				out = append(out, e)
			}
		case <-deadline:
			return out
		}
	}
}

func TestAKeywordChangedInTheFileIsAStateChange(t *testing.T) {
	paths := loadOrg(t, map[string]string{"t.org": "* Work :job:\n** TODO Ship it\n** TODO Other\n"})
	got := eventsDuring(t, EventStateChange, func() {
		os.WriteFile(paths["t.org"], []byte("* Work :job:\n** DONE Ship it\n** TODO Other\n"), 0644)
		GetDb().ReloadFile(paths["t.org"])
	})
	if len(got) != 1 {
		t.Fatalf("want one statechange, got %+v", got)
	}
	e := got[0]
	if e.Headline != "Ship it" || e.From != "TODO" || e.To != "DONE" || e.Line != 1 || strings.Join(e.Tags, ",") != "job" {
		t.Errorf("event: %+v", e)
	}
	// Renaming is not a change of keyword.
	got = eventsDuring(t, EventStateChange, func() {
		os.WriteFile(paths["t.org"], []byte("* Work :job:\n** DONE Shipped it\n** TODO Other\n"), 0644)
		GetDb().ReloadFile(paths["t.org"])
	})
	if len(got) != 0 {
		t.Errorf("a rename was a statechange: %+v", got)
	}
}

func TestRulesMatchAndArgumentsStayArguments(t *testing.T) {
	rule := common.HookRule{Name: "r", On: []string{"statechange"}, Match: map[string]string{"to": "done|cancelled", "tags": "job"}}
	e := Event{Kind: EventStateChange, Headline: `x"; rm -rf ~ #`, To: "DONE", Tags: []string{"home", "job"}}
	if !ruleWants(rule, e) {
		t.Errorf("should match")
	}
	if ruleWants(rule, Event{Kind: EventStateChange, To: "DONE", Tags: []string{"home"}}) {
		t.Errorf("matched without the tag")
	}
	if ruleWants(rule, Event{Kind: EventStateChange, To: "DONEISH", Tags: []string{"job"}}) {
		t.Errorf("a match is the whole value")
	}
	if ruleWants(rule, Event{Kind: EventCapture, To: "DONE", Tags: []string{"job"}}) {
		t.Errorf("matched another kind")
	}
	if got := hookArg("[{{headline}}]", hookFields(e)); got != `[x"; rm -rf ~ #]` {
		t.Errorf("arg: %q", got)
	}
}

func TestARunGetsItsArgumentsEnvironmentAndStdin(t *testing.T) {
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	e := Event{Kind: EventStateChange, Headline: "Ship it; echo pwned", From: "TODO", To: "DONE", At: "now"}
	run := runHook(common.HookRule{Name: "argv", Run: []string{"printf", "%s|%s", "{{headline}}", "{{to}}"}}, e)
	if !run.Ok || run.Output != "Ship it; echo pwned|DONE" {
		t.Errorf("run: %+v", run)
	}
	run = runHook(common.HookRule{Name: "sh", Shell: `printf '%s %s ' "$ORGS_HEADLINE" "$ORGS_HOOK"; cat`}, e)
	if !run.Ok || !strings.HasPrefix(run.Output, "Ship it; echo pwned sh {") || !strings.Contains(run.Output, `"to":"DONE"`) {
		t.Errorf("shell: %+v", run)
	}
	run = runHook(common.HookRule{Name: "slow", Shell: "sleep 5", Timeout: "200ms"}, e)
	if run.Ok || !strings.Contains(run.Msg, "timed out") {
		t.Errorf("timeout: %+v", run)
	}
	run = runHook(common.HookRule{Name: "fails", Shell: "echo nope; exit 3"}, e)
	if run.Ok || run.Output != "nope" {
		t.Errorf("failure: %+v", run)
	}
}

func TestAWebhookIsSigned(t *testing.T) {
	var got struct {
		Hook  string
		Event Event
	}
	sig := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		m := hmac.New(sha256.New, []byte("s3cret"))
		m.Write(b)
		if r.Header.Get("X-Orgs-Signature") == "sha256="+hex.EncodeToString(m.Sum(nil)) {
			sig = "good"
		}
		w.Write([]byte("thanks"))
	}))
	defer srv.Close()
	os.Setenv("ORGS_TEST_HOOK_SECRET", "s3cret")
	defer os.Unsetenv("ORGS_TEST_HOOK_SECRET")
	run := runHook(common.HookRule{Name: "web", Url: srv.URL, Secret: "env:ORGS_TEST_HOOK_SECRET"},
		Event{Kind: EventCapture, Headline: "Call Jo", Template: "Todo"})
	if !run.Ok || got.Hook != "web" || got.Event.Headline != "Call Jo" || sig != "good" || run.Output != "thanks" {
		t.Errorf("webhook: %+v %+v %q", run, got, sig)
	}
}

func TestADeadlineThatPassesIsAnnouncedOnce(t *testing.T) {
	loadOrg(t, map[string]string{"d.org": "* TODO Taxes\n  DEADLINE: <2026-04-30 Thu 17:00>\n* DONE Old\n  DEADLINE: <2026-04-30 Thu>\n"})
	since := time.Date(2026, 4, 30, 16, 0, 0, 0, time.Local)
	got := eventsDuring(t, EventDeadline, func() {
		deadlinesPassed(since, since.Add(2*time.Hour))
	})
	if len(got) != 1 || got[0].Headline != "Taxes" || !strings.HasPrefix(got[0].Date, "<2026-04-30") {
		t.Errorf("deadlines: %+v", got)
	}
	got = eventsDuring(t, EventDeadline, func() {
		deadlinesPassed(since.Add(2*time.Hour), since.Add(3*time.Hour))
	})
	if len(got) != 0 {
		t.Errorf("announced twice: %+v", got)
	}
}

func TestALimitStopsAHookSettingItselfOff(t *testing.T) {
	rule := common.HookRule{Name: "loop-" + time.Now().Format("150405.000"), Limit: 3}
	n := 0
	for i := 0; i < 10; i++ {
		if allowRun(rule) {
			n++
		}
	}
	if n != 3 {
		t.Errorf("ran %d times", n)
	}
}
