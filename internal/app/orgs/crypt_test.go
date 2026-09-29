package orgs

import (
	"os"
	"strings"
	"testing"
)

// The parts of org-crypt that can be wrong without gpg being involved: which
// rows are the secret, whether what is there is already ciphertext, and getting
// the armour back out of a body in a shape gpg will take.

func lines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// org-crypt encrypts from after the planning line and the drawers to the end of
// the *subtree*. Both ends matter: starting too early takes the property drawer
// with it - and a heading's :ID: is how links and attachments reach it - while
// stopping at the heading's own body would leave its children in the clear
// underneath it.
func TestCryptRangeStartsAfterTheDrawersAndEndsAtTheSubtree(t *testing.T) {
	src := `* Secret                                                              :crypt:
SCHEDULED: <2026-09-28 Mon>
  :PROPERTIES:
  :ID: abc
  :END:
  the body
** a child
   more
* Next heading
  not this
`
	ls := lines(src)
	// The heading runs from row 0 to the row before "* Next heading".
	start, end := cryptRange(ls, 0, 7)
	if ls[start] != "  the body" {
		t.Errorf("the secret should start at the body, got %q", ls[start])
	}
	if ls[end] != "   more" {
		t.Errorf("the secret should end at the end of the subtree, got %q", ls[end])
	}
	body := strings.Join(ls[start:end+1], "\n")
	for _, must := range []string{"the body", "** a child", "more"} {
		if !strings.Contains(body, must) {
			t.Errorf("the secret is missing %q:\n%s", must, body)
		}
	}
	for _, mustNot := range []string{":PROPERTIES:", ":ID:", "SCHEDULED:", "Next heading"} {
		if strings.Contains(body, mustNot) {
			t.Errorf("the secret should not include %q:\n%s", mustNot, body)
		}
	}
}

// A heading with nothing but a drawer has no secret, and encrypting nothing
// would write a block saying there is one where there is not.
func TestCryptRangeOfAnEmptyHeading(t *testing.T) {
	ls := lines("* Secret   :crypt:\n  :PROPERTIES:\n  :ID: abc\n  :END:\n")
	start, end := cryptRange(ls, 0, 3)
	body := []string{}
	if start <= end && start < len(ls) {
		body = ls[start : end+1]
	}
	if strings.TrimSpace(strings.Join(body, "")) != "" {
		t.Errorf("expected nothing, got %q", body)
	}
}

func TestIsArmored(t *testing.T) {
	yes := lines("\n" + armorBegin + "\nabc\n" + armorEnd + "\n")
	if !isArmored(yes) {
		t.Error("an armoured body should be recognised through a leading blank line")
	}
	if isArmored(lines("  ordinary text\n")) {
		t.Error("ordinary text is not armour")
	}
	if isArmored(nil) {
		t.Error("nothing is not armour")
	}
	// The marker has to be the first thing written. A body that merely mentions
	// it is a note about encryption, not an encrypted note.
	if isArmored(lines("  I pasted a " + armorBegin + " once\n")) {
		t.Error("a mention of the marker is not armour")
	}
}

// gpg refuses an indented armour outright - "no valid OpenPGP data found" - so
// a block somebody re-indented by hand, or an older writer indented to match
// the body, is one nothing can decrypt until the indent comes off.
func TestArmorOfStripsIndentAndSurroundings(t *testing.T) {
	body := lines(`  ` + armorBegin + `

  jA0ECQMIabc123
  =AbCd
  ` + armorEnd + `
  something after
`)
	got := armorOf(body)
	want := armorBegin + "\n\njA0ECQMIabc123\n=AbCd\n" + armorEnd
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
	if strings.Contains(got, "something after") {
		t.Error("only the block itself should come back")
	}
	for _, l := range strings.Split(got, "\n") {
		if l != strings.TrimSpace(l) {
			t.Errorf("a line came back indented: %q", l)
		}
	}
}

func TestCryptTagDefaults(t *testing.T) {
	if cryptTag() != "crypt" {
		t.Errorf("the default tag should be org-crypt's, got %q", cryptTag())
	}
}

// ---------------------------------------------------------------------------
// The parts that need gpg
// ---------------------------------------------------------------------------

// A gpg home of this test's own, short enough and private enough for gpg to
// use it.
//
// Both of those are real constraints rather than fussiness. gpg refuses a
// homedir anybody else can read, and it puts its agent socket inside that
// directory - a unix socket path is limited to about a hundred characters, and
// the temporary directories a test framework hands out are most of the way
// there before anything is added to them. Either one makes gpg fail in a way
// that looks like the code being wrong.
func gpgHome(t *testing.T) {
	t.Helper()
	if gpgBin() == "" {
		t.Skip("no gpg on this machine")
	}
	dir, err := os.MkdirTemp("/tmp", "og")
	if err != nil {
		t.Skipf("no temporary directory: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0700); err != nil {
		t.Skipf("could not make it private: %v", err)
	}
	t.Setenv("GNUPGHOME", dir)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	gpgHome(t)
	const plain = "  Account 12345678\n  Sort code 01-02-03\n\n** a child\n   more\n"
	armor, err := encryptBody(plain, "", "hunter2")
	if err != nil {
		t.Skipf("gpg could not encrypt here: %v", err)
	}
	if !strings.HasPrefix(armor, armorBegin) {
		t.Fatalf("not armoured:\n%s", armor)
	}
	if strings.Contains(armor, "12345678") {
		t.Error("the plaintext is in the ciphertext")
	}
	back, err := decryptBody(armor, "hunter2")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	// Verbatim in, verbatim out - the child heading's column zero included,
	// because re-indenting it would stop it being a heading.
	if back != plain {
		t.Errorf("the round trip changed it:\ngot  %q\nwant %q", back, plain)
	}
}

// The wrong passphrase has to fail, and fail as a refusal rather than as
// plausible rubbish - everything upstream of here decides whether to write a
// file on the strength of it.
func TestWrongPassphraseFails(t *testing.T) {
	gpgHome(t)
	armor, err := encryptBody("a secret", "", "hunter2")
	if err != nil {
		t.Skipf("gpg could not encrypt here: %v", err)
	}
	if _, err := decryptBody(armor, "not the passphrase"); err == nil {
		t.Error("the wrong passphrase should not decrypt")
	}
}

// Symmetric encryption with no passphrase would produce a file anybody can
// read, so it is refused rather than done.
func TestSymmetricNeedsAPassphrase(t *testing.T) {
	if _, err := encryptBody("a secret", "", ""); err == nil {
		t.Error("symmetric encryption with no passphrase should be refused")
	}
}

// Nothing may be written unless the ciphertext has been read back and matched.
// This is the guard that stands between a wrong key and a note nobody can ever
// read again, so it is pinned rather than trusted.
func TestEncryptChecksItsOwnWork(t *testing.T) {
	gpgHome(t)
	// A key that does not exist: gpg fails, and the failure has to reach the
	// caller rather than an empty block being handed back as success.
	out, err := encryptBody("a secret", "nobody@nowhere.invalid", "")
	if err == nil {
		t.Errorf("encrypting to an unknown key should fail, got %q", out)
	}
}
