package cert

// `orgs cert` makes a self-signed certificate for a server on this machine, so
// it can serve https (and, with httpsOnly, nothing else) without anybody
// knowing what openssl is. It writes the files the config's servercrt and
// serverkey name, for localhost, 127.0.0.1 and ::1 plus any -host given.
//
// It leaves a pair that is already there alone unless told -force, and says
// when that one runs out, so asking is safe and is how a client finds out
// whether it is time for a new one.
//
// 820 days, not ten years: macOS refuses a TLS certificate valid for more
// than 825 days, and somebody who adds this one to their keychain to quiet
// the browser's warning should not then be told it is invalid.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Cert struct {
	Crt   string
	Key   string
	Hosts string
	Days  int
	Force bool
}

// Answer is the -json shape.
type Answer struct {
	Crt     string    `json:"crt"`
	Key     string    `json:"key"`
	Expires time.Time `json:"expires"`
	Made    bool      `json:"made"` // false: the pair was already there and was left alone
}

func (self *Cert) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Cert) StartPlugin(manager *common.PluginManager)         {}
func (self *Cert) NeedsNoServer() bool                               { return true }
func (self *Cert) HelpGroup() string                                 { return "The server" }

func (self *Cert) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Crt, "crt", "", "where to write the certificate (default: servercrt in the config)")
	fset.StringVar(&self.Key, "key", "", "where to write its private key (default: serverkey in the config)")
	fset.StringVar(&self.Hosts, "host", "", "more names or addresses it is for, comma separated (always localhost, 127.0.0.1, ::1)")
	fset.IntVar(&self.Days, "days", 820, "how long it is good for")
	fset.BoolVar(&self.Force, "force", false, "replace a certificate that is already there")
}

func (self *Cert) Exec(core *commands.Core) {
	crt, key := self.Crt, self.Key
	if s := core.ServerSettings; s != nil {
		if crt == "" {
			crt = s.ServerCrt
		}
		if key == "" {
			key = s.ServerKey
		}
	}
	if crt == "" || key == "" {
		commands.Fail("cert: nowhere to write it - set servercrt and serverkey under server: in the config, or pass -crt and -key")
		os.Exit(1)
	}
	if !self.Force {
		if expires, err := expiry(crt); err == nil {
			if _, err := os.Stat(key); err == nil {
				commands.RenderOne(Answer{Crt: crt, Key: key, Expires: expires}, func() {
					fmt.Printf("%s is already there (good until %s); -force to replace it\n", crt, expires.Local().Format("2006-01-02"))
				})
				return
			}
		}
	}
	expires, err := write(crt, key, self.hosts(), self.Days)
	if err != nil {
		commands.Fail("cert: %v", err)
		os.Exit(1)
	}
	commands.RenderOne(Answer{Crt: crt, Key: key, Expires: expires, Made: true}, func() {
		fmt.Printf("Wrote %s and %s, good until %s.\n", crt, key, expires.Local().Format("2006-01-02"))
		fmt.Printf("Serve it with  allowHttps: true  (and httpsOnly: true for nothing but https), then restart.\n")
	})
}

func (self *Cert) hosts() []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	for _, h := range strings.Split(self.Hosts, ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

func expiry(crt string) (time.Time, error) {
	data, err := os.ReadFile(crt)
	if err != nil {
		return time.Time{}, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return time.Time{}, fmt.Errorf("%s is not a PEM certificate", crt)
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}
	return c.NotAfter, nil
}

func write(crt, key string, hosts []string, days int) (time.Time, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return time.Time{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return time.Time{}, err
	}
	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "orgs on " + hostname(), Organization: []string{"orgs (self-signed)"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Duration(days) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		// Its own issuer, so it can be trusted as itself: a client pins it as
		// the one CA it believes for this server.
		IsCA: true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return time.Time{}, err
	}
	keyDer, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return time.Time{}, err
	}
	for _, p := range []string{crt, key} {
		if dir := filepath.Dir(p); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return time.Time{}, err
			}
		}
	}
	// The key first, readable by its owner only; a certificate with no key
	// beside it is the worse half-written state.
	if err := os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDer}), 0o600); err != nil {
		return time.Time{}, err
	}
	if err := os.WriteFile(crt, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return time.Time{}, err
	}
	return tmpl.NotAfter, nil
}

func hostname() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "this machine"
}

func init() {
	commands.AddCmd("cert", "make a self-signed https certificate for a server on this machine",
		func() commands.Cmd { return &Cert{} })
}
