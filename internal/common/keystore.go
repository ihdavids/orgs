package common

// The credential store, shared by the server (which validates logins against
// it) and the `orgs user` command (which writes it). It lives here rather than
// in internal/app/orgs for the usual reason: a CLI command package cannot
// import the server package, so anything both halves need to agree about has
// to be said in one place that both can reach.
//
// Two hashes, and the distinction between them is the whole design:
//
//   ClientHash  is what a client sends *instead of* the password. The password
//               never leaves the machine somebody typed it on.
//   StoredHash  is what the keystore file holds. The server hashes what it
//               received a second time, with the server's own orgSalt mixed in.
//
// Keeping them different is what stops a stolen keystore from being a set of
// working logins: the file holds StoredHash, the wire carries ClientHash, and
// there is no way to get from the former back to the latter.
//
// What this does NOT do is make an http server safe. A ClientHash is
// password-equivalent *over the wire* - anything that can capture one can
// replay it - so this is protection for the password itself (which is reused
// elsewhere, gets typed into logs, and outlives any one session) and not a
// substitute for TLS. Run the server on https.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

/* SDOC: Settings
* Users and the keystore

	Orgs keeps its users in a keystore file, named by =keystore:= in the server
	block of your config. A relative path is taken relative to the
	configuration file itself, the same way =orgs_extensions.yaml= is.

	#+BEGIN_SRC yaml
	server:
	  keystore: "orgs_keystore.yaml"
	#+END_SRC

	*Until you set this, the only account that exists is admin/default*, which
	is built in so that a fresh server can be logged into at all. The server
	prints a large warning on startup for as long as that is true.

	Users are added with the =orgs user= command, which reads the same config
	file the server does and writes the keystore beside it:

	#+BEGIN_SRC sh
	orgs user add ian          # prompts for a password, twice
	orgs user ls               # who exists, and how their password is stored
	orgs user passwd ian       # change one
	orgs user rm ian           # remove one
	#+END_SRC

	The command edits a file on disk, so it has to be run on the machine the
	server runs on. It never talks to the server, and the server picks the
	change up the next time it starts.

** Administrators, and adding a user over the wire

	An account marked =admin: true= may add another account without having
	access to the server's disk, which is what =orgs adduser= does:

	#+BEGIN_SRC sh
	orgs adduser dana          # prompts for a password, twice
	orgs adduser sam -admin    # and this one may add users too
	#+END_SRC

	Unlike =orgs user=, this talks to the server (=POST /users/add=), so it
	works from anywhere you can log in - and the new account works at once
	rather than at the next restart, because the server writes the keystore it
	is already holding.

	The password still never travels: the client makes a salt for the new user
	and sends =ClientHash= of the password under it, the same as a login does.

	Somebody has to be an administrator before any of this can be used, and
	the only account that starts out as one is the built-in =admin=. On a
	keystore written by hand, or by a version of orgs older than this, mark
	one on the server's own machine:

	#+BEGIN_SRC sh
	orgs user admin ian        # ian may now add users
	orgs user noadmin ian      # and now may not
	orgs user add dana -admin  # added as an administrator
	#+END_SRC

	The server will not add a user when it is running on its built-in
	admin/default account, because that keystore has no file behind it and
	there would be nowhere to write. It says so rather than appearing to work.

	The file it writes looks like this - the password is a hash, and the salt
	is per user:

	#+BEGIN_SRC yaml
	creds:
	  ian:
	    password: "sha256:9f2c..."
	    salt: "b3d1..."
	logins:
	  ian: 2026-09-30T11:04:17Z
	#+END_SRC

	A =password:= written out as cleartext still works, so a keystore typed by
	hand does what it looks like it does - but the server names those accounts
	in its startup warning, and =orgs user passwd= is how to convert one.

	Passwords are never sent to the server. A client asks for the user's salt,
	hashes the password with it, and sends the hash; the server hashes that
	again with =orgSalt= before comparing. Set =requireHashedLogin: true= to
	refuse a login that arrives as cleartext at all - worth doing once every
	client talking to this server is new enough to hash, and noted in the
	startup warning until then.
EDOC */

// The prefix on a hash a client sends in place of a password.
const clientHashPrefix = "sha256c:"

// The prefix on a hash the keystore file holds.
const storedHashPrefix = "sha256:"

// The account a server with no keystore has, and the password it has. Named
// rather than spelled out at each use so that the startup warning, the tests
// and the `orgs user` command cannot drift apart about what "the default" is.
const (
	DefaultUsername = "admin"
	DefaultPassword = "default"
)

// Used for Yaml Keystores
type Cred struct {
	Password string `json:"password" yaml:"password"`
	Salt     string `json:"salt" yaml:"salt"`
	// Whether this account may administer the server - which at present means
	// one thing, adding another account through POST /users/add. Written
	// omitempty so that a keystore full of ordinary users reads as one.
	Admin bool `json:"admin,omitempty" yaml:"admin,omitempty"`
}

type KeyStore interface {
	// Validate answers whether this secret logs this user in. The secret is a
	// ClientHash from any current client, or a cleartext password from an
	// older one.
	Validate(user, secret string) bool
	// GetSalt answers with the salt a client should hash this user's password
	// with. It answers for a user that does not exist as readily as for one
	// that does - the reply is the only thing a login page gets before it has
	// any credentials, so telling the two apart would be a way to ask this
	// server for a list of its users.
	GetSalt(user string) (string, error)
	// IsAdmin answers whether this account may administer the server. It is
	// false for a user that does not exist, so a caller never has to ask
	// twice.
	IsAdmin(user string) bool
	// DefaultCredentialUsers names the accounts whose password is still the
	// built-in default, and PlaintextUsers those whose password is stored as
	// cleartext. Both are for the startup warning; both should be empty.
	DefaultCredentialUsers() []string
	PlaintextUsers() []string
}

type YamlKeystore struct {
	Creds  map[string]Cred
	Logins map[string]time.Time

	// Where this was read from, and where Save writes. Empty for the built-in
	// default keystore, which has nowhere to be saved to and does not need one.
	Path string `yaml:"-"`
	// The server's own salt, mixed into every stored hash. Not part of the
	// file: the point of it is to be somewhere other than the file, so that
	// the file on its own is not enough.
	OrgSalt string `yaml:"-"`

	// Guards Creds and Logins. The server mutates both from request handlers -
	// every login records a time, and POST /users/add writes an account - and
	// two requests arriving together would otherwise be two goroutines writing
	// the same map. Unexported, so neither yaml direction sees it.
	mu sync.Mutex
}

// ClientHash is what a client sends in place of the password.
//
// The username is part of it so that the same password on two accounts does
// not produce the same hash on the wire. That also means a rename invalidates
// the password, which is why there is no rename - remove the account and add
// it again.
func ClientHash(user, salt, password string) string {
	sum := sha256.Sum256([]byte("orgs-client-v1:" + user + ":" + salt + ":" + password))
	return clientHashPrefix + hex.EncodeToString(sum[:])
}

// StoredHash is what the keystore file holds: the hash the client sent, hashed
// again with the server's orgSalt. A keystore read off somebody's disk is
// therefore not a set of secrets that can be sent to this server.
func StoredHash(salt, orgSalt, clientHash string) string {
	sum := sha256.Sum256([]byte("orgs-stored-v1:" + salt + ":" + orgSalt + ":" + clientHash))
	return storedHashPrefix + hex.EncodeToString(sum[:])
}

// IsClientHash reports whether a login arrived hashed rather than as a
// cleartext password.
func IsClientHash(s string) bool { return strings.HasPrefix(s, clientHashPrefix) }

// IsStoredHash reports whether a keystore entry is hashed rather than being a
// cleartext password somebody typed into the file.
func IsStoredHash(s string) bool { return strings.HasPrefix(s, storedHashPrefix) }

// NewSalt makes a fresh per-user salt.
func NewSalt() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("could not generate a salt: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func ctEq(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// DefaultKeystore is the keystore a server has when no keystore file is
// configured: one account, the default one, so that a server somebody has just
// built can be logged into. Every caller of this is expected to have warned
// about it.
func DefaultKeystore(orgSalt string) *YamlKeystore {
	return &YamlKeystore{
		Creds:   map[string]Cred{DefaultUsername: {Password: DefaultPassword, Salt: KBAD_SALT, Admin: true}},
		Logins:  map[string]time.Time{},
		OrgSalt: orgSalt,
	}
}

// LoadKeystore reads a keystore file. A file that is not there is not an error
// worth failing a startup over on its own - the caller decides what to do with
// an empty keystore - so it comes back as os.ErrNotExist for the caller to
// check.
func LoadKeystore(path, orgSalt string) (*YamlKeystore, error) {
	if path == "" {
		return nil, fmt.Errorf("no keystore path set")
	}
	if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
		return nil, fmt.Errorf("keystore path is not a yaml file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ks := &YamlKeystore{Path: path, OrgSalt: orgSalt}
	if err := yaml.Unmarshal(data, ks); err != nil {
		return nil, fmt.Errorf("parsing keystore %s: %w", path, err)
	}
	if ks.Creds == nil {
		ks.Creds = map[string]Cred{}
	}
	if ks.Logins == nil {
		ks.Logins = map[string]time.Time{}
	}
	// Unmarshalling into a struct that already had these set would have
	// cleared them had they been part of the file; they are not, but the
	// assignment says so rather than relying on it.
	ks.Path = path
	ks.OrgSalt = orgSalt
	return ks, nil
}

// NewKeystore is an empty keystore at a path, for `orgs user add` creating the
// first account on a server that has never had one.
func NewKeystore(path, orgSalt string) *YamlKeystore {
	return &YamlKeystore{
		Creds:   map[string]Cred{},
		Logins:  map[string]time.Time{},
		Path:    path,
		OrgSalt: orgSalt,
	}
}

// Save writes the keystore back.
//
// Written to a temporary file and renamed, because this is the file that says
// who may log in: a half-written one locks everybody out, and the window for
// it is exactly when something else went wrong. Mode 0600 - it holds password
// hashes, and the old code wrote it world-writable.
func (s *YamlKeystore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

// saveLocked is Save for a caller that already holds the lock.
func (s *YamlKeystore) saveLocked() error {
	if s.Path == "" {
		// The built-in default keystore. Recording a login into it is
		// meaningless rather than an error worth reporting on every login.
		return nil
	}
	out, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, out, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (s *YamlKeystore) GetSalt(user string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.Creds[user]; ok {
		return c.Salt, nil
	}
	// A user that is not here still gets an answer, and the same answer every
	// time, so that a login form cannot be used to find out which names exist.
	// Derived from the server's own salt, so it is not guessable either.
	mac := hmac.New(sha256.New, []byte(s.OrgSalt))
	mac.Write([]byte("orgs-salt-decoy-v1:" + user))
	return hex.EncodeToString(mac.Sum(nil))[:32], nil
}

// credMatches is the one comparison, with the two legacy directions folded
// into it before it happens: a cleartext password from an old client, and a
// cleartext entry in an old keystore. Both ends are reduced to a ClientHash,
// so whatever came in, what is compared is a hash of fixed length.
func (s *YamlKeystore) credMatches(user string, c Cred, secret string) bool {
	client := secret
	if !IsClientHash(secret) {
		// A cleartext password from a client older than the hashing. Work out
		// what a current client would have sent for it.
		client = ClientHash(user, c.Salt, secret)
	}
	if IsStoredHash(c.Password) {
		return ctEq(StoredHash(c.Salt, s.OrgSalt, client), c.Password)
	}
	// A cleartext entry in the keystore - one typed by hand, or the built-in
	// default. The stored side knows the password, so it can derive the same
	// hash the client did.
	return ctEq(client, ClientHash(user, c.Salt, c.Password))
}

func (s *YamlKeystore) Validate(user, secret string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.Creds[user]
	if !ok {
		// Do the work anyway. A login that comes back faster for a name that
		// does not exist is the same disclosure GetSalt is careful about.
		s.credMatches(user, Cred{Password: "x", Salt: "x"}, secret)
		return false
	}
	if !s.credMatches(user, c, secret) {
		return false
	}
	if s.Logins == nil {
		s.Logins = map[string]time.Time{}
	}
	s.Logins[user] = time.Now()
	if err := s.saveLocked(); err != nil {
		fmt.Fprintf(os.Stderr, "Keystore: could not record the login: %v\n", err)
	}
	return true
}

// SetPassword sets or replaces a user's password, with a fresh salt.
func (s *YamlKeystore) SetPassword(user, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	salt, err := NewSalt()
	if err != nil {
		return err
	}
	if s.Creds == nil {
		s.Creds = map[string]Cred{}
	}
	// Whether somebody is an administrator is not a fact about their
	// password, so changing one must not quietly clear the other.
	s.Creds[user] = Cred{
		Password: StoredHash(salt, s.OrgSalt, ClientHash(user, salt, password)),
		Salt:     salt,
		Admin:    s.Creds[user].Admin,
	}
	return nil
}

// AddUser writes a new account from what a client sent: a salt it generated
// and the ClientHash of the password under that salt. The password itself
// never reaches here, which is the whole point of adding a user over the wire
// rather than only by editing the file.
//
// It refuses to overwrite an existing account. Replacing somebody's password
// is what SetPassword is for, and an "add" that silently did it would be a way
// to take an account over.
func (s *YamlKeystore) AddUser(user, salt, clientHash string, admin bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !IsClientHash(clientHash) {
		return fmt.Errorf("the password for %q did not arrive hashed", user)
	}
	if salt == "" {
		return fmt.Errorf("no salt was sent for %q", user)
	}
	if s.Creds == nil {
		s.Creds = map[string]Cred{}
	}
	if _, taken := s.Creds[user]; taken {
		return fmt.Errorf("there is already a user called %q", user)
	}
	s.Creds[user] = Cred{
		Password: StoredHash(salt, s.OrgSalt, clientHash),
		Salt:     salt,
		Admin:    admin,
	}
	return s.saveLocked()
}

// SetAdmin turns a user's administrator flag on or off, and answers whether
// there was a user to do it to.
func (s *YamlKeystore) SetAdmin(user string, admin bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.Creds[user]
	if !ok {
		return false
	}
	c.Admin = admin
	s.Creds[user] = c
	return true
}

// IsAdmin answers whether this user may administer the server.
func (s *YamlKeystore) IsAdmin(user string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.Creds[user]
	return ok && c.Admin
}

// HasUser answers whether an account exists.
func (s *YamlKeystore) HasUser(user string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.Creds[user]
	return ok
}

// Writable answers whether this keystore has a file behind it. The built-in
// default one does not, so a server running on it can be logged into and
// cannot be written to - Save is a no-op there, and anything adding a user
// has to say so rather than appearing to succeed.
func (s *YamlKeystore) Writable() bool { return s.Path != "" }

// Admins names every account that may administer the server.
func (s *YamlKeystore) Admins() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for u, c := range s.Creds {
		if c.Admin {
			out = append(out, u)
		}
	}
	sort.Strings(out)
	return out
}

// Remove takes a user out. Their last login goes with them: it is a fact about
// an account that no longer exists.
func (s *YamlKeystore) Remove(user string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Creds[user]; !ok {
		return false
	}
	delete(s.Creds, user)
	delete(s.Logins, user)
	return true
}

func (s *YamlKeystore) Users() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.Creds))
	for u := range s.Creds {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

// DefaultCredentialUsers names every account whose password is still the
// built-in default, whether it is stored as cleartext or has been hashed.
func (s *YamlKeystore) DefaultCredentialUsers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for u, c := range s.Creds {
		if s.credMatches(u, c, DefaultPassword) {
			out = append(out, u)
		}
	}
	sort.Strings(out)
	return out
}

// PlaintextUsers names every account whose password is sitting in the keystore
// file as cleartext.
func (s *YamlKeystore) PlaintextUsers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for u, c := range s.Creds {
		if !IsStoredHash(c.Password) {
			out = append(out, u)
		}
	}
	sort.Strings(out)
	return out
}

// LastLogin answers when a user last logged in, and whether they ever have.
func (s *YamlKeystore) LastLogin(user string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.Logins[user]
	return t, ok
}

// KeystorePath resolves the configured keystore path.
//
// A relative path is taken relative to the configuration file that named it,
// not to the working directory: the keystore is a companion to the config the
// way orgs_extensions.yaml is, and the server and the `orgs user` command are
// not necessarily run from the same place.
func KeystorePath(keystore, configFile string) string {
	if keystore == "" || filepath.IsAbs(keystore) || configFile == "" {
		return keystore
	}
	return filepath.Join(filepath.Dir(configFile), keystore)
}

// NewUser is what a client sends to POST /users/add.
//
// The password is not in it. A client adding a user makes a fresh salt for
// them, hashes the password with it exactly as a login does (ClientHash), and
// sends the two; the server hashes that again with its own orgSalt before it
// goes in the file. So the same thing is true of adding an account as of
// using one: what crosses the wire is not the password, and what lands in the
// file is not what crosses the wire.
type NewUser struct {
	Username string `json:"username"`
	// The salt the client generated for this user. It goes into the keystore
	// as it stands and is what GET /salt will answer with from now on.
	Salt string `json:"salt"`
	// ClientHash(Username, Salt, password).
	Password string `json:"password"`
	// Whether the new account may add users itself.
	Admin bool `json:"admin"`
}

// ValidUsername answers whether a name can be an account, and says why not
// when it cannot.
//
// The rules are about the name being usable rather than about taste: it is a
// yaml key, it is part of the hash of its own password, and it travels in a
// url query on GET /salt. A name with a space or a newline in it is a shape
// of bug nobody would look for.
func ValidUsername(user string) error {
	switch {
	case user == "":
		return fmt.Errorf("a user needs a name")
	case len(user) > 64:
		return fmt.Errorf("that name is longer than 64 characters")
	case strings.TrimSpace(user) != user || strings.ContainsAny(user, " \t\r\n"):
		return fmt.Errorf("a user name cannot have whitespace in it")
	case strings.ContainsAny(user, ":/?#[]@"):
		return fmt.Errorf("a user name cannot contain any of  : / ? # [ ] @")
	}
	return nil
}
