package orgs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

type contextKey string

const contextKeyUsername contextKey = "username"

// GetUsername extracts the authenticated username from the request context.
func GetUsername(r *http.Request) string {
	if v, ok := r.Context().Value(contextKeyUsername).(string); ok {
		return v
	}
	return ""
}

// GET /salt?user=<name>
//
// The salt a client needs before it can hash a password, which is why this is
// a public route: it is asked for by somebody who has no credentials yet.
//
// It answers for a name that does not exist exactly as readily as for one that
// does (see common.YamlKeystore.GetSalt). Nothing here is secret - a salt is
// not a credential, and the point of a per-user one is that a hash cannot be
// precomputed, not that the salt cannot be read.
func salt(w http.ResponseWriter, r *http.Request) {
	user := r.URL.Query().Get("user")
	if user == "" {
		http.Error(w, "salt: a user parameter is required", http.StatusBadRequest)
		return
	}
	s, err := GetKeystore().GetSalt(user)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get a salt: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"salt": s})
}

func login(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	//body, _ := io.ReadAll(r.Body)
	//fmt.Fprintf(os.Stderr, "LOGIN REQUEST: %s\n", string(body))
	//read := io.ByteReader(body)
	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Boo: %v\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// A current client hashes the password with the user's salt and sends that
	// (GET /salt, then common.ClientHash), so the password itself never
	// reaches this server. One older than that sends the password, which still
	// works - refusing it outright would lock out every client that has not
	// been rebuilt - but it is said out loud each time, and a server can turn
	// it off once nothing it talks to needs it.
	if !common.IsClientHash(creds.Password) {
		if Conf().Server != nil && Conf().Server.RequireHashedLogin {
			fmt.Fprintf(os.Stderr, "Refused a cleartext login for %q: requireHashedLogin is set\n", creds.Username)
			http.Error(w, "this server does not accept a password sent as cleartext; upgrade the client", http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(os.Stderr, "WARNING: %q sent its password as cleartext. An up to date client asks GET /salt and sends a hash.\n", creds.Username)
	}

	if ok := GetKeystore().Validate(creds.Username, creds.Password); !ok {
		fmt.Fprintf(os.Stderr, "Failed validate\n")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	//fmt.Fprintf(os.Stderr, "Encrypted token gen\n")
	token, expiresAt, err := GenerateEncryptedToken(creds.Username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad token: %v [%s]\n", err, creds.Username)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	http.SetCookie(w, &http.Cookie{
		Name:     "orgstoken",
		Value:    encoded,
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     token,
		"ExpiresAt": expiresAt,
	})
}

func refresh(w http.ResponseWriter, r *http.Request) {
	var tokenStr string

	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		tokenStr = strings.TrimPrefix(auth, "Bearer ")
	} else if c, err := r.Cookie("orgstoken"); err == nil {
		if val, err := base64.StdEncoding.DecodeString(c.Value); err == nil {
			tokenStr = string(val)
		}
	}
	if tokenStr == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	claims := &Claims{}
	if _, err := ValidateEncryptedToken(tokenStr, claims); err != nil {
		fmt.Fprintf(os.Stderr, "Refresh: failed to validate existing token: %v\n", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	token, expiresAt, err := GenerateEncryptedToken(claims.Username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Refresh: failed to generate token: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	http.SetCookie(w, &http.Cookie{
		Name:     "orgstoken",
		Value:    encoded,
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     token,
		"ExpiresAt": expiresAt,
	})
}

// The user everything is done as when authentication is turned off. Without
// one, `noAuth: true` does not mean "no authentication" but "no authentication,
// and also no stored queries, no kanban boards and no capture templates" - every
// per-user endpoint reads the username off the request and refuses an empty one.
const NoAuthUsername = "local"

// Stands in for `authenticate` when noAuth is set: no token is asked for, and
// everybody is the same local user.
func noAuthUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), contextKeyUsername, NoAuthUsername)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
				fmt.Fprintf(os.Stderr, "Failed to decode the token str\n")
			} else {
				// The token itself is never printed. It is a bearer
				// credential: anything that can read the log can then be
				// whoever the log was about, and a server's stdout ends up in
				// terminals, journals and pasted-in bug reports.
				tokenStr = string(val)
			}
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		claims := &Claims{}
		if _, err := ValidateEncryptedToken(tokenStr, claims); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to authenticate: %v\n", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(os.Stderr, "AUTHENTICATION OKAY\n")
		ctx := context.WithValue(r.Context(), contextKeyUsername, claims.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
