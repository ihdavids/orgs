package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testOrgSalt = "a server's own salt"

func TestHashedLoginRoundTrip(t *testing.T) {
	ks := NewKeystore("", testOrgSalt)
	if err := ks.SetPassword("ian", "correct horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	salt, err := ks.GetSalt("ian")
	if err != nil {
		t.Fatalf("GetSalt: %v", err)
	}
	if !ks.Validate("ian", ClientHash("ian", salt, "correct horse")) {
		t.Error("a client hash made with the salt the server gave out did not validate")
	}
	if ks.Validate("ian", ClientHash("ian", salt, "wrong horse")) {
		t.Error("the wrong password validated")
	}
}

// The whole point of the two hashes: what is in the file is not what the wire
// carries, so a keystore somebody read off a disk is not a working login.
func TestStoredHashIsNotTheWireSecret(t *testing.T) {
	ks := NewKeystore("", testOrgSalt)
	if err := ks.SetPassword("ian", "correct horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	stored := ks.Creds["ian"].Password
	if !IsStoredHash(stored) {
		t.Fatalf("stored password is not a stored hash: %q", stored)
	}
	if strings.Contains(stored, "correct horse") {
		t.Error("the password is in the file")
	}
	if ks.Validate("ian", stored) {
		t.Error("the value held in the keystore file logged in as the user")
	}
}

// A password typed on this machine must be enough to log in without the
// machine ever sending it - and the value it does send must not be the
// password with a prefix on it.
func TestClientHashDoesNotCarryThePassword(t *testing.T) {
	h := ClientHash("ian", "some salt", "correct horse")
	if !IsClientHash(h) {
		t.Fatalf("not recognised as a client hash: %q", h)
	}
	if strings.Contains(h, "correct horse") {
		t.Error("the password is in the hash")
	}
	// The username is part of it, so one account's hash is not another's.
	if h == ClientHash("jane", "some salt", "correct horse") {
		t.Error("two users with the same password hash to the same wire secret")
	}
	// And so is the salt.
	if h == ClientHash("ian", "other salt", "correct horse") {
		t.Error("the salt does not affect the hash")
	}
}

// A keystore written by hand, or the built-in default, holds a cleartext
// password. It has to keep working - and a current client still never sends
// the password for it.
func TestCleartextKeystoreEntry(t *testing.T) {
	ks := &YamlKeystore{
		Creds:   map[string]Cred{"ian": {Password: "typed in by hand", Salt: "somesalt"}},
		OrgSalt: testOrgSalt,
	}
	salt, _ := ks.GetSalt("ian")
	if !ks.Validate("ian", ClientHash("ian", salt, "typed in by hand")) {
		t.Error("a hashed login against a cleartext keystore entry failed")
	}
	// An older client sending the password itself is still accepted.
	if !ks.Validate("ian", "typed in by hand") {
		t.Error("a cleartext login against a cleartext keystore entry failed")
	}
	if ks.Validate("ian", "something else") {
		t.Error("the wrong password validated")
	}
	if got := ks.PlaintextUsers(); len(got) != 1 || got[0] != "ian" {
		t.Errorf("PlaintextUsers() = %v, want [ian]", got)
	}
}

// A client older than the hashing sends the password. That has to keep working
// against a hashed entry too, or hashing the keystore locks those clients out.
func TestCleartextLoginAgainstHashedEntry(t *testing.T) {
	ks := NewKeystore("", testOrgSalt)
	if err := ks.SetPassword("ian", "correct horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if !ks.Validate("ian", "correct horse") {
		t.Error("a cleartext login against a hashed entry failed")
	}
	if ks.Validate("ian", "wrong horse") {
		t.Error("the wrong password validated")
	}
}

// The startup warning's whole job. It has to fire for the built-in default and
// for an account somebody hashed the default password into.
func TestDefaultCredentialUsers(t *testing.T) {
	ks := DefaultKeystore(testOrgSalt)
	if got := ks.DefaultCredentialUsers(); len(got) != 1 || got[0] != DefaultUsername {
		t.Errorf("built-in keystore: DefaultCredentialUsers() = %v, want [%s]", got, DefaultUsername)
	}

	// Hashed, but hashed from the default password: still the default password.
	hashed := NewKeystore("", testOrgSalt)
	if err := hashed.SetPassword("admin", DefaultPassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if got := hashed.DefaultCredentialUsers(); len(got) != 1 || got[0] != "admin" {
		t.Errorf("hashed default: DefaultCredentialUsers() = %v, want [admin]", got)
	}
	if got := hashed.PlaintextUsers(); len(got) != 0 {
		t.Errorf("hashed default: PlaintextUsers() = %v, want none", got)
	}

	// And it has to stay quiet once the password is a real one, or the warning
	// is noise and gets ignored.
	if err := hashed.SetPassword("admin", "something nobody will guess"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if got := hashed.DefaultCredentialUsers(); len(got) != 0 {
		t.Errorf("after a password change: DefaultCredentialUsers() = %v, want none", got)
	}
}

// A login form asking for a salt must not be a way to ask which accounts
// exist, so an unknown user gets an answer of the same shape - and the same
// one every time, since a salt that changed per request would say just as much.
func TestSaltForUnknownUserIsStableAndPlausible(t *testing.T) {
	ks := NewKeystore("", testOrgSalt)
	if err := ks.SetPassword("ian", "correct horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	real, _ := ks.GetSalt("ian")
	decoy, err := ks.GetSalt("nobody")
	if err != nil {
		t.Fatalf("GetSalt for an unknown user answered with an error: %v", err)
	}
	if len(decoy) != len(real) {
		t.Errorf("decoy salt is %d characters, a real one is %d", len(decoy), len(real))
	}
	again, _ := ks.GetSalt("nobody")
	if decoy != again {
		t.Error("two requests for the same unknown user got different salts")
	}
	if other, _ := ks.GetSalt("somebodyelse"); other == decoy {
		t.Error("every unknown user gets the same salt")
	}
	// Logging in as a user who does not exist still fails.
	if ks.Validate("nobody", ClientHash("nobody", decoy, "anything")) {
		t.Error("a user who does not exist logged in")
	}
}

func TestKeystoreSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.yaml")

	ks := NewKeystore(path, testOrgSalt)
	if err := ks.SetPassword("ian", "correct horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := ks.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// It holds hashes: nobody else's business.
	if st, err := os.Stat(path); err != nil {
		t.Fatalf("Stat: %v", err)
	} else if st.Mode().Perm() != 0600 {
		t.Errorf("keystore mode is %v, want 0600", st.Mode().Perm())
	}

	back, err := LoadKeystore(path, testOrgSalt)
	if err != nil {
		t.Fatalf("LoadKeystore: %v", err)
	}
	salt, _ := back.GetSalt("ian")
	if !back.Validate("ian", ClientHash("ian", salt, "correct horse")) {
		t.Error("a password set before the save did not validate after the load")
	}

	// The orgSalt is not in the file, which is the point of it: the same file
	// read with a different server salt must not validate.
	other, err := LoadKeystore(path, "a different server")
	if err != nil {
		t.Fatalf("LoadKeystore: %v", err)
	}
	if other.Validate("ian", ClientHash("ian", salt, "correct horse")) {
		t.Error("the keystore validated under a different orgSalt")
	}
}

func TestLoadKeystoreMissingFile(t *testing.T) {
	_, err := LoadKeystore(filepath.Join(t.TempDir(), "nope.yaml"), testOrgSalt)
	if !os.IsNotExist(err) {
		t.Errorf("LoadKeystore of a missing file: err = %v, want os.ErrNotExist", err)
	}
}

func TestRemoveUser(t *testing.T) {
	ks := NewKeystore("", testOrgSalt)
	if err := ks.SetPassword("ian", "x"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if !ks.Remove("ian") {
		t.Error("Remove of an existing user answered false")
	}
	if ks.Remove("ian") {
		t.Error("Remove of a user who is gone answered true")
	}
	if ks.Validate("ian", "x") {
		t.Error("a removed user logged in")
	}
}

func TestKeystorePath(t *testing.T) {
	// A relative keystore sits beside the config that named it, not in
	// whatever directory the command happened to be run from.
	if got, want := KeystorePath("keys.yaml", "/etc/orgs/orgs.yaml"), "/etc/orgs/keys.yaml"; got != want {
		t.Errorf("KeystorePath = %q, want %q", got, want)
	}
	if got, want := KeystorePath("/abs/keys.yaml", "/etc/orgs/orgs.yaml"), "/abs/keys.yaml"; got != want {
		t.Errorf("KeystorePath of an absolute path = %q, want %q", got, want)
	}
	if got := KeystorePath("", "/etc/orgs/orgs.yaml"); got != "" {
		t.Errorf("KeystorePath of no keystore = %q, want empty", got)
	}
}

// The browser hashes a password too (worg/src/clienthash.ts), and it has to
// arrive at the same answer as this does or nobody can log into worg. The two
// implementations share nothing but this vector, so it is what catches one of
// them being changed on its own - clienthash.test.ts pins the same one.
func TestClientHashVector(t *testing.T) {
	const want = "sha256c:ac9687aca9b1a7ea3fc4589f6aee84d7f45395e64af7e0884136119190b40541"
	if got := ClientHash("ian", "somesalt", "correct horse"); got != want {
		t.Errorf("ClientHash(ian, somesalt, correct horse):\n got %s\nwant %s\n"+
			"If this change was deliberate, worg/src/clienthash.ts has to change with it.", got, want)
	}
}

// ---------------------------------------------------------------------------
// Administrators, and adding a user over the wire
// ---------------------------------------------------------------------------

// The round trip POST /users/add is made of: a client makes a salt, hashes
// under it, sends the two, and the account it creates logs in the ordinary
// way. Nothing the client sent is the password, and nothing in the file is
// what the client sent.
func TestAddUserRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.yaml")
	ks := NewKeystore(path, testOrgSalt)

	salt, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	if err := ks.AddUser("dana", salt, ClientHash("dana", salt, "correct horse"), false); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	// The salt the client chose is the salt the server hands out from now on,
	// or nothing that logs in afterwards would hash to the same thing.
	if got, _ := ks.GetSalt("dana"); got != salt {
		t.Errorf("GetSalt = %q, want the salt the client sent, %q", got, salt)
	}
	if !ks.Validate("dana", ClientHash("dana", salt, "correct horse")) {
		t.Error("an added user could not log in")
	}
	if ks.Validate("dana", ClientHash("dana", salt, "wrong horse")) {
		t.Error("the wrong password validated")
	}
	if ks.IsAdmin("dana") {
		t.Error("a user added without the admin flag came out an administrator")
	}

	// AddUser saves, because the point of it is that the account works without
	// anything being restarted - which means it has to survive one.
	back, err := LoadKeystore(path, testOrgSalt)
	if err != nil {
		t.Fatalf("LoadKeystore: %v", err)
	}
	if !back.Validate("dana", ClientHash("dana", salt, "correct horse")) {
		t.Error("the added user did not survive a save and load")
	}
	if strings.Contains(back.Creds["dana"].Password, "correct horse") {
		t.Error("the password is in the keystore file")
	}
	if back.Creds["dana"].Password == ClientHash("dana", salt, "correct horse") {
		t.Error("the keystore holds what the wire carried, so a stolen file would be a login")
	}
}

// An add that quietly replaced an account would be a way to take one over.
func TestAddUserWillNotReplace(t *testing.T) {
	ks := NewKeystore("", testOrgSalt)
	if err := ks.SetPassword("ian", "correct horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	salt, _ := NewSalt()
	if err := ks.AddUser("ian", salt, ClientHash("ian", salt, "hunter2"), true); err == nil {
		t.Fatal("AddUser overwrote an existing account")
	}
	oldSalt, _ := ks.GetSalt("ian")
	if !ks.Validate("ian", ClientHash("ian", oldSalt, "correct horse")) {
		t.Error("the refused add damaged the account it refused to replace")
	}
	if ks.IsAdmin("ian") {
		t.Error("the refused add promoted the account anyway")
	}
}

// The password has to arrive hashed. This endpoint is newer than the hashing,
// so unlike a login there is no old client to be kind to.
func TestAddUserRefusesCleartext(t *testing.T) {
	ks := NewKeystore("", testOrgSalt)
	salt, _ := NewSalt()
	if err := ks.AddUser("dana", salt, "correct horse", false); err == nil {
		t.Error("AddUser took a cleartext password")
	}
	if err := ks.AddUser("dana", "", ClientHash("dana", "", "correct horse"), false); err == nil {
		t.Error("AddUser took an empty salt")
	}
}

func TestAdminFlag(t *testing.T) {
	ks := NewKeystore("", testOrgSalt)
	salt, _ := NewSalt()
	if err := ks.AddUser("sam", salt, ClientHash("sam", salt, "correct horse"), true); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	if !ks.IsAdmin("sam") {
		t.Error("a user added as an administrator is not one")
	}
	if ks.IsAdmin("nobody") {
		t.Error("a user that does not exist is an administrator")
	}
	if got := ks.Admins(); len(got) != 1 || got[0] != "sam" {
		t.Errorf("Admins() = %v, want [sam]", got)
	}
	if !ks.SetAdmin("sam", false) || ks.IsAdmin("sam") {
		t.Error("SetAdmin could not take the flag off")
	}
	if ks.SetAdmin("nobody", true) {
		t.Error("SetAdmin promoted a user that does not exist")
	}
}

// Being an administrator is not a fact about a password, so changing one must
// not clear the other. It did, when SetPassword replaced the whole Cred.
func TestPasswordChangeKeepsAdmin(t *testing.T) {
	ks := NewKeystore("", testOrgSalt)
	if err := ks.SetPassword("sam", "correct horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	ks.SetAdmin("sam", true)
	if err := ks.SetPassword("sam", "a new one"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if !ks.IsAdmin("sam") {
		t.Error("changing a password took the administrator flag with it")
	}
}

// The admin flag has to survive the file, or an administrator would last
// until the next restart.
func TestAdminSurvivesSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.yaml")
	ks := NewKeystore(path, testOrgSalt)
	if err := ks.SetPassword("sam", "correct horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := ks.SetPassword("dana", "correct horse"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	ks.SetAdmin("sam", true)
	if err := ks.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	back, err := LoadKeystore(path, testOrgSalt)
	if err != nil {
		t.Fatalf("LoadKeystore: %v", err)
	}
	if !back.IsAdmin("sam") {
		t.Error("the administrator flag did not survive the file")
	}
	if back.IsAdmin("dana") {
		t.Error("an ordinary user came back an administrator")
	}
}

// The built-in account is the only administrator a server starts with, which
// is what makes the first `orgs user admin` possible without one.
func TestBuiltInAccountIsAnAdministrator(t *testing.T) {
	ks := DefaultKeystore(testOrgSalt)
	if !ks.IsAdmin(DefaultUsername) {
		t.Errorf("the built-in %q account is not an administrator, so nobody could ever become one", DefaultUsername)
	}
	// And it has no file behind it, so the server must refuse to write to it
	// rather than adding an account that vanishes at the next restart.
	if ks.Writable() {
		t.Error("the built-in keystore claims to be writable")
	}
	if !NewKeystore("/tmp/whatever.yaml", testOrgSalt).Writable() {
		t.Error("a keystore with a path does not claim to be writable")
	}
}

func TestValidUsername(t *testing.T) {
	for _, ok := range []string{"ian", "dana.roe", "sam_1", "a-b"} {
		if err := ValidUsername(ok); err != nil {
			t.Errorf("ValidUsername(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", " ", "two words", "has\ttab", "a:b", "a/b", "a?b", "a#b", "a@b", strings.Repeat("x", 65)} {
		if err := ValidUsername(bad); err == nil {
			t.Errorf("ValidUsername(%q) = nil, want a refusal", bad)
		}
	}
}
