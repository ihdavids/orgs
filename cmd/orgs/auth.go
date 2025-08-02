package main

import (
	"crypto/sha1"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ihdavids/orgs/internal/app/orgs"
)

func login(w http.ResponseWriter, r *http.Request) {
	var creds orgs.Credentials
	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	hsh := sha1.New()
	hsh.Write([]byte(creds.Password))

	if ok := orgs.GetKeystore().Validate(creds.Username, creds.Password); ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	/*
		if creds.Username != "admin" || creds.Password != "password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	*/

	token, err := GenerateEncryptedToken(creds.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:  "orgstoken",
		Value: token,
		// TODO: Make this configurable
		Expires: time.Now().Add(5 * time.Minute),
	})
}

func authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("orgstoken")
		if err != nil {
			if err == http.ErrNoCookie {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		claims := &Claims{}
		if _, err := ValidateEncryptedToken(c.Value, claims); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		} else {
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		}
		/*
			tokenStr := c.Value

			tkn, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
				return jwtKey, nil
			})
		*/

	})
}
