package orgs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"
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
			s.Save()
			return true
		}
	}
	return false
}

func (s *YamlKeystore) Save() error {
	if out, err := yaml.Marshal(s); err == nil {
		if Conf().Keystore != "" {
			if filepath.Ext(Conf().Keystore) == ".yaml" {
				return os.WriteFile(Conf().Keystore, out, os.ModePerm)
			} else {
				return fmt.Errorf("keystore path is not a yaml file: %s")
			}
		} else {
			return fmt.Errorf("keystore path not set, cannot save")
		}
	} else {
		return err
	}
}

func DefaultKeystore() {
	// Give us A keystore when we start up at least.
	currentKeystore = &YamlKeystore{Creds: map[string]Cred{
		"admin": Cred{Password: "default", Salt: kBAD_SALT},
	}, Logins: map[string]time.Time{}}
}

var currentKeystore KeyStore = nil
