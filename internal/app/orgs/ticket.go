package orgs

/* SDOC: API
* Login tickets: worg in a browser without typing a password
  A client that is already logged in - the orgs command line, the VS Code
  plugin - can log a browser in to worg without anybody knowing the password:

  - =POST /login/ticket= (logged in) answers ={"ticket": "...", "url": "/login/ticket/..."}=
  - =GET /login/ticket/<ticket>[?app=Agenda]= (public) logs the browser in as
    the user who asked for the ticket - it sets worg's cookies - and redirects
    to worg, on the view =app= names.

  A ticket is good for one use and for a minute, so the url it is on is worth
  nothing by the time it is in a browser's history. =orgs worg= and
  =Org: Open worg in Browser= are the two ends that use it.
EDOC */

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

const ticketLife = time.Minute

type loginTicket struct {
	username string
	expires  time.Time
}

var tickets = struct {
	sync.Mutex
	m map[string]loginTicket
}{m: map[string]loginTicket{}}

// The cookie worg keeps its login in (worg/src/auth.ts COOKIE_NAME): the token
// and when it runs out, as json. Not HttpOnly, because worg reads it.
const worgCookie = "orgs_auth"

func PostLoginTicket(w http.ResponseWriter, r *http.Request) {
	user := GetUsername(r)
	if user == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	t := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now()
	tickets.Lock()
	for k, v := range tickets.m {
		if now.After(v.expires) {
			delete(tickets.m, k)
		}
	}
	tickets.m[t] = loginTicket{username: user, expires: now.Add(ticketLife)}
	tickets.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]string{"ticket": t, "url": "/login/ticket/" + t})
}

// A view name worg switches on: letters only, so nothing else rides along on
// the redirect.
var ticketAppRe = regexp.MustCompile(`^[A-Za-z]{1,40}$`)

func RedeemLoginTicket(w http.ResponseWriter, r *http.Request) {
	t := mux.Vars(r)["ticket"]
	tickets.Lock()
	tk, ok := tickets.m[t]
	delete(tickets.m, t)
	tickets.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !ok || time.Now().After(tk.expires) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family: sans-serif; padding: 2em">
<h3>This login link has been used or has run out</h3>
<p>A link from <code>orgs worg</code> or VS Code's <em>Org: Open worg in Browser</em> works once, for a minute. Run it again for a new one, or <a href="/">log in</a>.</p>
</body></html>`)
		return
	}
	token, expiresAt, err := GenerateEncryptedToken(tk.username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "login ticket: bad token: %v [%s]\n", err, tk.username)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// The server's own cookie, exactly as POST /login sets it...
	http.SetCookie(w, &http.Cookie{
		Name:     "orgstoken",
		Value:    base64.StdEncoding.EncodeToString([]byte(token)),
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	// ...and worg's, which is what worg looks for when it starts.
	worg, _ := json.Marshal(map[string]interface{}{"token": token, "expiresAt": expiresAt.UnixMilli()})
	http.SetCookie(w, &http.Cookie{
		Name:     worgCookie,
		Value:    url.QueryEscape(string(worg)),
		Path:     "/",
		Expires:  expiresAt,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	to := "/"
	if app := r.URL.Query().Get("app"); ticketAppRe.MatchString(app) {
		to = "/?app=" + app
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}
