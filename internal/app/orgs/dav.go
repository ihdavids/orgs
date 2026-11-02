package orgs

// CalDAV and CardDAV: the agenda as a calendar and the contacts as an address
// book, for a phone's own apps. The protocol is emersion/go-webdav; what is in
// them is davcal.go and davcard.go. This file is who may ask.
//
// Off unless `dav.enable`. Then, on every request, in this order:
//
//  1. **Transport.** https, or a request from this machine (a reverse proxy
//     terminating TLS in front of it), or `dav.allowHttp` (a VPN). A phone
//     sending a password over plain http across a cafe's wifi is refused
//     before the password is looked at.
//  2. **Throttle.** Failed logins are counted per address; past five the
//     address waits (doubling, at most fifteen minutes) and gets 429 without
//     its password being checked at all.
//  3. **An app password.** HTTP Basic with the orgs user name and a password
//     made for this (`orgs dav password`, POST /davconfig/passwords), never the
//     login password: a phone's account settings are not where a password
//     that opens everything should live, and one device can be cut off
//     without changing anything else. Stored as sha256 of 100 random bits
//     (a hash of a random secret needs no salt or stretching), 0600, beside
//     the other state files; compared in constant time.
//  4. **Your own collections.** The user in the path must be the one who
//     logged in.
//  5. **readOnly** allows only the methods that read.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-webdav/caldav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/gorilla/mux"
)

type davCtxKey struct{}

func davUser(ctx context.Context) string {
	u, _ := ctx.Value(davCtxKey{}).(string)
	return u
}

func davSettings() (s struct {
	on, cal, card, ro, http bool
}) {
	if Conf().Server == nil {
		return
	}
	d := Conf().Server.Dav
	s.on, s.cal, s.card, s.ro, s.http = d.Enable, d.CalendarOn(), d.ContactsOn(), d.ReadOnly, d.AllowHttp
	return
}

// ---------------------------------------------------------------------------
// App passwords
// ---------------------------------------------------------------------------

type davPassword struct {
	Id       string
	Label    string
	Hash     string
	Created  time.Time
	LastUsed time.Time `json:",omitempty"`
}

type davPasswordStore struct {
	mu    sync.Mutex
	path  string
	Users map[string][]davPassword
}

var davStore *davPasswordStore
var davStoreOnce sync.Once

func davPasswords() *davPasswordStore {
	davStoreOnce.Do(func() {
		dir := ""
		if Conf().PlugManager != nil {
			dir = Conf().PlugManager.HomeDir
		}
		if dir == "" {
			dir = filepath.Dir(Conf().loadedFrom)
		}
		davStore = &davPasswordStore{path: filepath.Join(dir, "dav_passwords.json"), Users: map[string][]davPassword{}}
		if b, err := os.ReadFile(davStore.path); err == nil {
			json.Unmarshal(b, davStore)
		}
		if davStore.Users == nil {
			davStore.Users = map[string][]davPassword{}
		}
	})
	return davStore
}

func (s *davPasswordStore) save() error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".writing"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func davHash(secret string) string {
	sum := sha256.Sum256([]byte("orgs-dav-v1:" + secret))
	return hex.EncodeToString(sum[:])
}

// newDavPassword makes one for user and answers it - the only time it is ever
// seen. Four groups of five: easy to type on a phone, 100 random bits.
func (s *davPasswordStore) add(user, label string) (davPassword, string, error) {
	raw := make([]byte, 13)
	if _, err := rand.Read(raw); err != nil {
		return davPassword{}, "", err
	}
	enc := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))[:20]
	secret := enc[0:5] + "-" + enc[5:10] + "-" + enc[10:15] + "-" + enc[15:20]
	idb := make([]byte, 4)
	rand.Read(idb)
	p := davPassword{Id: hex.EncodeToString(idb), Label: strings.TrimSpace(label), Hash: davHash(secret), Created: time.Now()}
	if p.Label == "" {
		p.Label = "device " + p.Created.Format("2006-01-02")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Users[user] = append(s.Users[user], p)
	return p, secret, s.save()
}

func (s *davPasswordStore) revoke(user, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := []davPassword{}
	found := false
	for _, p := range s.Users[user] {
		if p.Id == id || id == "all" {
			found = true
			continue
		}
		keep = append(keep, p)
	}
	s.Users[user] = keep
	if !found {
		return false, nil
	}
	return true, s.save()
}

// check says whether secret opens user's collections. Every stored hash is
// compared, so the time taken does not say which one nearly matched.
func (s *davPasswordStore) check(user, secret string) bool {
	h := davHash(strings.ToLower(strings.TrimSpace(secret)))
	s.mu.Lock()
	defer s.mu.Unlock()
	ok := false
	at := -1
	for i, p := range s.Users[user] {
		if subtle.ConstantTimeCompare([]byte(p.Hash), []byte(h)) == 1 {
			ok, at = true, i
		}
	}
	// Last used, written at most once an hour per password.
	if ok && time.Since(s.Users[user][at].LastUsed) > time.Hour {
		s.Users[user][at].LastUsed = time.Now()
		s.save()
	}
	return ok
}

// ---------------------------------------------------------------------------
// Throttle
// ---------------------------------------------------------------------------

type davFails struct {
	n     int
	until time.Time
}

var davThrottle = struct {
	mu sync.Mutex
	by map[string]*davFails
}{by: map[string]*davFails{}}

func davHost(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func davBlocked(host string) (time.Duration, bool) {
	davThrottle.mu.Lock()
	defer davThrottle.mu.Unlock()
	f := davThrottle.by[host]
	if f == nil || time.Now().After(f.until) {
		return 0, false
	}
	return time.Until(f.until), true
}

func davFailed(host string) {
	davThrottle.mu.Lock()
	defer davThrottle.mu.Unlock()
	f := davThrottle.by[host]
	if f == nil {
		f = &davFails{}
		davThrottle.by[host] = f
	}
	f.n++
	if f.n > 5 {
		wait := time.Second << uint(f.n-6)
		if wait > 15*time.Minute || wait <= 0 {
			wait = 15 * time.Minute
		}
		f.until = time.Now().Add(wait)
	}
}

func davSucceeded(host string) {
	davThrottle.mu.Lock()
	delete(davThrottle.by, host)
	davThrottle.mu.Unlock()
}

func davLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ---------------------------------------------------------------------------
// The gate
// ---------------------------------------------------------------------------

var davReading = map[string]bool{"GET": true, "HEAD": true, "OPTIONS": true, "PROPFIND": true, "REPORT": true}

func davGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		set := davSettings()
		if !set.on {
			http.NotFound(w, r)
			return
		}
		host := davHost(r)
		if r.TLS == nil && !davLoopback(host) && !set.http {
			http.Error(w, "CalDAV and CardDAV are served over https only: connect to the https port, "+
				"or set dav.allowHttp for a network you trust", http.StatusForbidden)
			return
		}
		if wait, blocked := davBlocked(host); blocked {
			w.Header().Set("Retry-After", fmt.Sprint(int(wait.Seconds())+1))
			http.Error(w, "too many failed logins; try again later", http.StatusTooManyRequests)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user == "" || !davPasswords().check(user, pass) {
			if ok {
				davFailed(host)
				fmt.Fprintf(os.Stderr, "dav: refused a login as %q from %s\n", user, host)
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="orgs", charset="UTF-8"`)
			http.Error(w, "an app password is needed: make one with `orgs dav password`", http.StatusUnauthorized)
			return
		}
		davSucceeded(host)
		// Nobody else's collections.
		p := r.URL.Path
		for _, pre := range []string{"/dav/cal/", "/dav/card/"} {
			if strings.HasPrefix(p, pre) {
				rest := strings.TrimPrefix(p, pre)
				if seg, _, _ := strings.Cut(rest, "/"); seg != "" {
					if u, err := urlUnescape(seg); err != nil || u != user {
						http.Error(w, "not yours", http.StatusForbidden)
						return
					}
				}
			}
		}
		if set.ro && !davReading[r.Method] {
			http.Error(w, "this server's calendar and contacts are read only (dav.readOnly)", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), davCtxKey{}, user)))
	})
}

func urlUnescape(s string) (string, error) { return url.PathUnescape(s) }

// registerDavRoutes mounts the collections. Public routes as far as the
// token is concerned: davGate is their authentication.
func registerDavRoutes(router *mux.Router) {
	cal := &caldav.Handler{Backend: &calBackend{}, Prefix: "/dav/cal"}
	card := &carddav.Handler{Backend: &cardBackend{}, Prefix: "/dav/card"}
	calOn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !davSettings().cal {
			http.NotFound(w, r)
			return
		}
		cal.ServeHTTP(w, r)
	})
	cardOn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !davSettings().card {
			http.NotFound(w, r)
			return
		}
		card.ServeHTTP(w, r)
	})
	router.Handle("/.well-known/caldav", davGate(calOn))
	router.Handle("/.well-known/carddav", davGate(cardOn))
	router.PathPrefix("/dav/cal/").Handler(davGate(calOn))
	router.PathPrefix("/dav/card/").Handler(davGate(cardOn))
	// A client that starts at /dav/ (or the bare host) is pointed at the
	// calendar; iOS tries the root first.
	router.Handle("/dav/", davGate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dav/cal/"+url.PathEscape(davUser(r.Context()))+"/", http.StatusPermanentRedirect)
	})))
}

// ---------------------------------------------------------------------------
// Managing passwords (the token's routes)
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /davconfig — CalDAV and CardDAV: Whether They Are On, And Your App Passwords
* POST /davconfig/passwords — Make An App Password
* DELETE /davconfig/passwords — Revoke One

	#+BEGIN_EXAMPLE
	GET    /davconfig                   {"Ok":true,"Enable":true,"Calendar":"/dav/cal/ian/calendars/org/", ...,"Passwords":[{"Id","Label","Created","LastUsed"}]}
	POST   /davconfig/passwords {"Label":"phone"}   {"Ok":true,"Id":"3fa1c2d0","Password":"abcde-fghij-klmno-pqrst"}
	DELETE /davconfig/passwords?id=3fa1c2d0   (id=all revokes every one)
	#+END_EXAMPLE

	The password is in the answer once and never again: only its hash is kept.
	A phone logs in with the orgs user name and it. Making one works while the
	collections are off, so a phone can be set up before they are turned on.
EDOC */
func RequestDav(w http.ResponseWriter, r *http.Request) {
	user := GetUsername(r)
	type pw struct {
		Id       string
		Label    string
		Created  time.Time
		LastUsed time.Time `json:",omitempty"`
	}
	out := struct {
		Ok                 bool
		Enable             bool
		ReadOnly           bool
		Calendar, Contacts string `json:",omitempty"`
		User               string
		Passwords          []pw
	}{Ok: true, User: user, Passwords: []pw{}}
	set := davSettings()
	out.Enable, out.ReadOnly = set.on, set.ro
	if set.cal {
		out.Calendar = "/dav/cal/" + user + "/calendars/org/"
	}
	if set.card {
		out.Contacts = "/dav/card/" + user + "/contacts/org/"
	}
	s := davPasswords()
	s.mu.Lock()
	for _, p := range s.Users[user] {
		out.Passwords = append(out.Passwords, pw{p.Id, p.Label, p.Created, p.LastUsed})
	}
	s.mu.Unlock()
	dynJson(w, out)
}

func PostDavPassword(w http.ResponseWriter, r *http.Request) {
	user := GetUsername(r)
	if user == "" {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": "who are you?"})
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct{ Label string }
	json.Unmarshal(body, &req)
	p, secret, err := davPasswords().add(user, req.Label)
	if err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Id": p.Id, "Label": p.Label, "User": user, "Password": secret})
}

func DeleteDavPassword(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	found, err := davPasswords().revoke(GetUsername(r), id)
	switch {
	case err != nil:
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
	case !found:
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": "no app password " + id})
	default:
		dynJson(w, map[string]interface{}{"Ok": true})
	}
}
