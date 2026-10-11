package orgs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

func cryptOp(t *testing.T, handler http.HandlerFunc, req common.CryptOp) common.CryptOpResult {
	t.Helper()
	raw, _ := json.Marshal(req)
	r := httptest.NewRequest("POST", "/crypt/x", strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, r)
	var res common.CryptOpResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("answered %q", w.Body.String())
	}
	return res
}

// What is on disk must never hold the secret, whatever else happens.
func mustNotHold(t *testing.T, path string, secrets ...string) {
	t.Helper()
	text := readOrg(t, path)
	for _, s := range secrets {
		if strings.Contains(text, s) {
			t.Fatalf("the file holds %q in the clear:\n%s", s, text)
		}
	}
}

const cryptUiFixture = `* Accounts
:PROPERTIES:
:ID: acc-1
:END:
  Sort code 01-02-03
  Account 12345678
** Child
   pin 4321
* After
  in the clear
`

// A heading is named by its headline row, and only by it: a row inside the
// body would resolve to the heading above it, and a reveal must not guess.
func TestCryptOpTargetIsTheHeadlineRow(t *testing.T) {
	paths := loadOrg(t, map[string]string{"bank.org": cryptUiFixture})
	if _, _, err := cryptOpTarget(common.CryptOp{Filename: paths["bank.org"], Line: 0}); err != nil {
		t.Errorf("the headline row should name the heading: %v", err)
	}
	if _, _, err := cryptOpTarget(common.CryptOp{Filename: paths["bank.org"], Line: 4}); err == nil {
		t.Error("a row in the body should be refused")
	}
}

// Seal an untagged heading, read it back, refuse the wrong passphrase, edit
// it, and refuse an edit that started from something else - with the file
// holding only ciphertext for the body throughout.
func TestCryptSealRevealEdit(t *testing.T) {
	gpgHome(t)
	paths := loadOrg(t, map[string]string{"bank.org": cryptUiFixture})
	p := paths["bank.org"]
	secrets := []string{"12345678", "01-02-03", "4321"}

	// With nothing held and no passphrase, nothing is tried.
	res := cryptOp(t, PostCryptSeal, common.CryptOp{Filename: p, Line: 0})
	if res.Ok || !res.NeedPassphrase {
		t.Fatalf("a seal with no passphrase should ask for one: %+v", res)
	}

	res = cryptOp(t, PostCryptSeal, common.CryptOp{Filename: p, Line: 0, Passphrase: "hunter2"})
	if !res.Ok || !res.Encrypted {
		t.Skipf("gpg could not encrypt here: %s", res.Msg)
	}
	mustNotHold(t, p, secrets...)
	text := readOrg(t, p)
	if !strings.Contains(strings.SplitN(text, "\n", 2)[0], ":crypt:") {
		t.Errorf("the headline should have been tagged:\n%s", text)
	}
	for _, kept := range []string{":ID: acc-1", "* After", "in the clear"} {
		if !strings.Contains(text, kept) {
			t.Errorf("%q should still be in the clear:\n%s", kept, text)
		}
	}
	if res.Text != "" {
		t.Error("a seal should not echo what it hid")
	}

	res = cryptOp(t, PostCryptReveal, common.CryptOp{Hash: res.Hash})
	if res.Ok || !res.NeedPassphrase || res.Text != "" {
		t.Fatalf("a reveal with nothing held should ask: %+v", res)
	}
	res = cryptOp(t, PostCryptReveal, common.CryptOp{Filename: p, Line: 0, Passphrase: "wrong"})
	if res.Ok || !res.WrongPassphrase || res.Text != "" {
		t.Fatalf("the wrong passphrase should be refused as such: %+v", res)
	}
	res = cryptOp(t, PostCryptReveal, common.CryptOp{Filename: p, Line: 0, Passphrase: "hunter2", Html: true})
	if !res.Ok || !strings.Contains(res.Text, "Account 12345678") || !strings.Contains(res.Text, "** Child") {
		t.Fatalf("the reveal should hand back the subtree: %+v", res)
	}
	if !strings.Contains(res.Html, "12345678") {
		t.Errorf("asked for html and got %q", res.Html)
	}
	mustNotHold(t, p, secrets...)
	base := res.Base

	// An edit whose base is not what is there is refused, and writes nothing.
	before := readOrg(t, p)
	res = cryptOp(t, PostCryptEdit, common.CryptOp{Filename: p, Line: 0, Passphrase: "hunter2", Text: "  new", Base: plainSum("something else")})
	if res.Ok {
		t.Fatal("an edit from a stale base should be refused")
	}
	// So is one under a passphrase that does not open what is there.
	res = cryptOp(t, PostCryptEdit, common.CryptOp{Filename: p, Line: 0, Passphrase: "typo", Text: "  new", Base: base})
	if res.Ok || !res.WrongPassphrase {
		t.Fatalf("an edit under the wrong passphrase should be refused: %+v", res)
	}
	if readOrg(t, p) != before {
		t.Fatal("a refused edit wrote to the file")
	}

	res = cryptOp(t, PostCryptEdit, common.CryptOp{Filename: p, Line: 0, Passphrase: "hunter2", Text: "  Account 87654321\n", Base: base})
	if !res.Ok {
		t.Fatalf("edit: %s", res.Msg)
	}
	mustNotHold(t, p, append(secrets, "87654321")...)
	res2 := cryptOp(t, PostCryptReveal, common.CryptOp{Filename: p, Line: 0, Passphrase: "hunter2"})
	if res2.Text != "  Account 87654321" || res2.Base != res.Base {
		t.Errorf("the edit did not read back: %q (base %s, want %s)", res2.Text, res2.Base, res.Base)
	}
}

// A passphrase is held only when asked, only after it worked, and a wrong one
// never replaces a good one.
func TestCryptRevealHoldsOnlyWhatWorked(t *testing.T) {
	gpgHome(t)
	if Conf().Server == nil {
		t.Skip("no server settings to set an unlock window on")
	}
	was := Conf().Server.Crypt.UnlockSeconds
	Conf().Server.Crypt.UnlockSeconds = 60
	t.Cleanup(func() { Conf().Server.Crypt.UnlockSeconds = was; relock() })
	relock()

	paths := loadOrg(t, map[string]string{"bank.org": cryptUiFixture})
	p := paths["bank.org"]
	if res := cryptOp(t, PostCryptSeal, common.CryptOp{Filename: p, Line: 0, Passphrase: "hunter2"}); !res.Ok {
		t.Skipf("gpg could not encrypt here: %s", res.Msg)
	}
	if res := cryptOp(t, PostCryptReveal, common.CryptOp{Filename: p, Line: 0, Passphrase: "hunter2"}); !res.Ok || res.Unlocked {
		t.Fatalf("a reveal without keep should hold nothing: %+v", res)
	}
	if res := cryptOp(t, PostCryptReveal, common.CryptOp{Filename: p, Line: 0, Passphrase: "wrong", Keep: true}); res.Ok || res.Unlocked {
		t.Fatalf("a wrong passphrase should never be held: %+v", res)
	}
	if res := cryptOp(t, PostCryptReveal, common.CryptOp{Filename: p, Line: 0, Passphrase: "hunter2", Keep: true}); !res.Ok || !res.Unlocked {
		t.Fatalf("keep should hold a passphrase that worked: %+v", res)
	}
	cryptOp(t, PostCryptReveal, common.CryptOp{Filename: p, Line: 0, Passphrase: "wrong", Keep: true})
	if res := cryptOp(t, PostCryptReveal, common.CryptOp{Filename: p, Line: 0}); !res.Ok {
		t.Fatalf("the held passphrase should still open it after a wrong one: %+v", res)
	}
	relock()
	if res := cryptOp(t, PostCryptReveal, common.CryptOp{Filename: p, Line: 0}); res.Ok || !res.NeedPassphrase {
		t.Fatalf("after a lock nothing should open: %+v", res)
	}
}
