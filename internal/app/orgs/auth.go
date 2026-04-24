package orgs

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func login(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	//body, _ := io.ReadAll(r.Body)
	//fmt.Printf("LOGIN REQUEST: %s\n", string(body))
	//read := io.ByteReader(body)
	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		fmt.Printf("Boo: %v\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	hsh := sha1.New()
	hsh.Write([]byte(creds.Password))

	if ok := GetKeystore().Validate(creds.Username, creds.Password); !ok {
		fmt.Printf("Failed validate\n")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	/*
		if creds.Username != "admin" || creds.Password != "password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	*/

	//fmt.Printf("Encrypted token gen\n")
	token, err := GenerateEncryptedToken(creds.Username)
	if err != nil {
		fmt.Printf("bad token: %v [%s]\n", err, creds.Username)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	//fmt.Printf("Setting as a cookie: [%s]\n", encoded)
	http.SetCookie(w, &http.Cookie{
		Name:  "orgstoken",
		Value: encoded,
		// TODO: Make this configurable
		Expires:  time.Now().Add(5 * time.Minute),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	//fmt.Printf("Returning the token\n")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tokenStr string

		// Prefer Authorization: Bearer <token> header (for API clients)
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		} else if c, err := r.Cookie("orgstoken"); err == nil {
			// Fall back to cookie (for browser clients)
			if val, err := base64.StdEncoding.DecodeString(c.Value); err != nil {
				fmt.Printf("Failed to decode the token str\n")
			} else {
				fmt.Printf("Using cookie token: %s\n", string(val))
				tokenStr = string(val)
			}
		} else {
			fmt.Printf("ERROR: %v\n", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		claims := &Claims{}
		if _, err := ValidateEncryptedToken(tokenStr, claims); err != nil {
			fmt.Printf("Failed to authenticate: %v\n", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Printf("AUTHENTICATION OKAY\n")
		next.ServeHTTP(w, r)
	})
}
