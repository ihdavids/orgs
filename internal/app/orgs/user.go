package orgs

import (
	"fmt"
	"time"
)

// Used for login.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type KeyStore interface {
	Validate(user, pass string) bool
	GetSalt(user string) (string, error)
}

func GetKeystore() KeyStore {
	return currentKeystore
}

// Used for Yaml Keystores
type Cred struct {
	Password string `json:"password"`
	Salt     string `json:"salt"`
}
type YamlKeystore struct {
	Creds  map[string]Cred
	Logins map[string]time.Time
}

func (s *YamlKeystore) GetSalt(user string) (string, error) {
	if p, ok := s.Creds[user]; ok {
		return p.Salt, nil
	}
	return "", fmt.Errorf("user not found")
}

func (s *YamlKeystore) Validate(user, pass string) bool {
	if p, ok := s.Creds[user]; ok {
		valid := p.Password == pass
		if valid {
			s.Logins[user] = time.Now()
			return true
		}
	}
	return false
}

func DefaultKeystore() {
	// Give us A keystore when we start up at least.
	currentKeystore = &YamlKeystore{Creds: map[string]Cred{
		"admin": Cred{Password: "default", Salt: kBAD_SALT},
	}, Logins: map[string]time.Time{}}
}

var currentKeystore KeyStore = nil
