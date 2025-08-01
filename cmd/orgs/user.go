package main

import "time"

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type KeyStore interface {
	Validate(user, pass string) bool
}

func GetKeystore() KeyStore {
	return &currentKeystore
}

type YamlKeystore struct {
	Creds  map[string]string
	Logins map[string]time.Time
}

func (s *YamlKeystore) Validate(user, pass string) bool {
	if p, ok := s.Creds[user]; ok {
		valid := p == pass
		if valid {
			s.Logins[user] = time.Now()
		}
	}
	return false
}

var currentKeystore YamlKeystore
