package orgs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
)

// A token past its expiry is refused everywhere but /refresh, which takes it
// until its refresh window closes; one with no window is refused there too.
func TestRefreshWindow(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	c := func(expired, window time.Duration) *Claims {
		cl := &Claims{Username: "admin", Claims: jwt.Claims{Issuer: kORGS_ISSUER,
			Expiry: jwt.NewNumericDate(now.Add(-expired))}}
		if window != 0 {
			cl.RefreshUntil = jwt.NewNumericDate(now.Add(window))
		}
		return cl
	}
	if err := validateToken(c(-time.Hour, 0), now, false); err != nil {
		t.Errorf("a good token refused: %v", err)
	}
	if err := validateToken(c(time.Hour, 24*time.Hour), now, false); err == nil {
		t.Error("an expired token was taken for a request")
	}
	if err := validateToken(c(time.Hour, 24*time.Hour), now, true); err != nil {
		t.Errorf("an expired token inside its window refused for a refresh: %v", err)
	}
	if err := validateToken(c(time.Hour, -time.Minute), now, true); err == nil {
		t.Error("a token past its refresh window was renewed")
	}
	if err := validateToken(c(time.Hour, 0), now, true); err == nil {
		t.Error("an expired token with no window (issued before there was one) was renewed")
	}
}

func TestCertStatus(t *testing.T) {
	now := time.Now()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "orgs.local"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(10*24*time.Hour + time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "server.crt")
	os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644)
	c := certStatus(p, now)
	if c.Error != "" || c.Subject != "orgs.local" || c.DaysLeft != 10 {
		t.Errorf("cert = %+v", c)
	}
	if c := certStatus(p, now.Add(12*24*time.Hour)); c.DaysLeft >= 0 {
		t.Errorf("an expired cert has %d days left", c.DaysLeft)
	}
	if c := certStatus(filepath.Join(t.TempDir(), "none.crt"), now); c.Error == "" {
		t.Error("a missing cert reads")
	}
}

func TestWorgBuild(t *testing.T) {
	b := worgBuild(fstest.MapFS{
		"index.html": {Data: []byte("<html>")},
		"build.json": {Data: []byte(`{"worgCommit": "abc123", "worgDirty": true, "orgsCommit": "def", "builtAt": "2026-11-17T00:00:00Z"}`)},
	})
	if !b.Present || b.WorgCommit != "abc123" || !b.WorgDirty || b.OrgsCommit != "def" {
		t.Errorf("build = %+v", b)
	}
	if b := worgBuild(fstest.MapFS{}); b.Present || b.WorgCommit != "" {
		t.Errorf("nothing embedded = %+v", b)
	}
}

func TestStatusNotesUnreadableFiles(t *testing.T) {
	paths := loadOrg(t, map[string]string{"a.org": "* A\n"})
	bad := filepath.Join(filepath.Dir(paths["a.org"]), "locked.org")
	os.WriteFile(bad, []byte("* B\n"), 0000)
	defer os.Chmod(bad, 0644)
	if f, err := os.Open(bad); err == nil {
		f.Close()
		t.Skip("running as a user who can read anything")
	}
	GetDb().LoadFile(bad, true)
	st := serverStatus()
	found := false
	for _, e := range st.Files.Unreadable {
		found = found || e.File == bad
	}
	if !found {
		t.Errorf("unreadable = %+v", st.Files.Unreadable)
	}
	os.Chmod(bad, 0644)
	GetDb().LoadFile(bad, true)
	for _, e := range serverStatus().Files.Unreadable {
		if e.File == bad {
			t.Error("still unreadable after it loaded")
		}
	}
}
