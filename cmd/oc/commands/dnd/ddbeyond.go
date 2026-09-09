package dnd

// orgs dnd import - bring a character over from D&D Beyond.
//
// D&D Beyond publishes no api, so this reads the same json the character
// sheet page reads for itself. A character whose privacy is set to public
// needs no credential at all; a private one needs the browser session cookie,
// which is what this asks for when the fetch comes back refused. The cookie
// is exchanged for a short lived bearer token by D&D Beyond's own auth
// service, exactly as the site does, and never leaves this machine - the
// server is only ever handed the character json that comes back.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common/dnd"
	"github.com/tmc/keyring"
	survey "gopkg.in/AlecAivazis/survey.v1"
)

const (
	ddbCharacterApi = "https://character-service.dndbeyond.com/character/v5/character/%s"
	ddbTokenApi     = "https://auth-service.dndbeyond.com/v1/cobalt-token"
	ddbKeyringUser  = "dndbeyond-cobalt"
	ddbCookieEnv    = "ORGS_DDB_COBALT"
)

// ddbIdPatterns pull the character id out of anything the user is likely to
// paste: a sheet url, a share url, or the id on its own.
var (
	ddbUrlPattern    = regexp.MustCompile(`/characters?/(\d+)`)
	ddbDigitsPattern = regexp.MustCompile(`\d+`)
)

func (self *Dnd) runImport(core *commands.Core) {
	// "orgs dnd import <url>" reads as naturally as the flag does, and a bare
	// argument lands in Kind the way it does for the other subcommands.
	if self.Ddb == "" && self.Json == "" && self.Kind != "" {
		self.Ddb = self.Kind
	}
	payload, err := self.ddbPayload()
	if err != nil {
		fmt.Printf("%s%s%s\n", cRed, err, cReset)
		return
	}
	req := dnd.DDBImportRequest{
		Ruleset:   self.Ruleset,
		Payload:   payload,
		Filename:  orDefault(self.Out, self.File),
		Overwrite: self.Force,
		Preview:   self.Preview,
		Player:    self.Player,
	}
	// A refusal comes back either as a transport error or as a body that says
	// it did not work, depending on how far the request got, so both are
	// reported the same way.
	res, err := post[dnd.DDBImportRequest, dnd.DDBImportResponse](core, "dnd/import", &req)
	failed := ""
	if err != nil {
		failed = err.Error()
	} else if !res.Ok || res.Character == nil {
		failed = orDefault(res.Msg, "the server did not say why")
	}
	if failed != "" {
		fmt.Printf("%simport failed: %s%s\n", cRed, failed, cReset)
		if strings.Contains(failed, "already exists") {
			fmt.Printf("%spass -force to replace it, or -out <file> to write it "+
				"somewhere else.%s\n", cDim, cReset)
		}
		return
	}
	if res.Sheet != nil {
		fmt.Print(dnd.TerminalSheet(res.Sheet))
	}
	self.printWarnings(res.Warnings)
	if self.Preview {
		fmt.Printf("%spreview only, nothing was written. Drop -preview to save it.%s\n",
			cDim, cReset)
		return
	}
	fmt.Printf("%s%s written%s\n", cBold, res.Filename, cReset)
	if self.Open {
		core.LaunchEditor(res.Filename, 0)
	}
}

// printWarnings lists what did not map cleanly. An import is worth checking
// over rather than trusting, so this is deliberately loud.
func (self *Dnd) printWarnings(warnings []string) {
	if len(warnings) == 0 {
		return
	}
	noun := "things"
	if len(warnings) == 1 {
		noun = "thing"
	}
	fmt.Printf("\n%s%d %s to check on this sheet:%s\n", cGold, len(warnings), noun, cReset)
	for _, w := range warnings {
		fmt.Printf("  %s-%s %s\n", cGold, cReset, w)
	}
	fmt.Printf("%sanything unmatched is still on the sheet by name, it just carries no "+
		"rules text.%s\n", cDim, cReset)
}

// ddbPayload gets the character json, from a file when one was named and from
// D&D Beyond otherwise.
func (self *Dnd) ddbPayload() (json.RawMessage, error) {
	if self.Json != "" {
		data, err := os.ReadFile(self.Json)
		if err != nil {
			return nil, fmt.Errorf("could not read %s: %s", self.Json, err)
		}
		return json.RawMessage(data), nil
	}
	if strings.TrimSpace(self.Ddb) == "" {
		return nil, fmt.Errorf("pass the character with -ddb <url or id>, or a saved " +
			"payload with -json <file>")
	}
	id := ddbCharacterId(self.Ddb)
	if id == "" {
		return nil, fmt.Errorf("no character id in %q - it should look like "+
			"https://www.dndbeyond.com/characters/12345678, or just the number",
			self.Ddb)
	}
	fmt.Printf("%sfetching character %s from D&D Beyond...%s\n", cDim, id, cReset)
	data, status, err := ddbFetch(id, "")
	if err != nil {
		return nil, err
	}
	if status == http.StatusOK {
		return self.ddbKeep(data)
	}
	// D&D Beyond answers 404 or 409 for an id that is not a character at all,
	// and 401 or 403 for one that exists but is not public. Only the second
	// is worth a credential.
	if status == http.StatusNotFound || status == http.StatusConflict {
		return nil, fmt.Errorf("D&D Beyond has no character %s - check the url or id", id)
	}
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return nil, fmt.Errorf("D&D Beyond answered %d for character %s", status, id)
	}
	fmt.Printf("%scharacter %s is not public, so this needs your D&D Beyond session.%s\n",
		cGold, id, cReset)
	cookie, err := ddbCookie(self.Cookie)
	if err != nil {
		return nil, err
	}
	token, err := ddbToken(cookie)
	if err != nil {
		return nil, err
	}
	data, status, err = ddbFetch(id, token)
	if err != nil {
		return nil, err
	}
	if status == http.StatusOK {
		return self.ddbKeep(data)
	}
	if status == http.StatusNotFound || status == http.StatusConflict {
		return nil, fmt.Errorf("D&D Beyond has no character %s on this account", id)
	}
	return nil, fmt.Errorf("D&D Beyond answered %d for character %s even with your "+
		"session - is the character on this account?", status, id)
}

// ddbKeep writes the payload out when -dump was given, which is how a
// character that will not convert can be looked at, or imported again later
// without going back to D&D Beyond.
func (self *Dnd) ddbKeep(data []byte) (json.RawMessage, error) {
	if self.Dump != "" {
		if err := os.WriteFile(self.Dump, data, 0644); err != nil {
			fmt.Printf("%scould not write %s: %s%s\n", cGold, self.Dump, err, cReset)
		} else {
			fmt.Printf("%spayload saved to %s%s\n", cDim, self.Dump, cReset)
		}
	}
	return json.RawMessage(data), nil
}

// ddbCharacterId accepts a sheet url or a bare id. A url is read for the id
// that follows /characters/, so a profile url with a user id in front of it
// does not hand back the wrong number.
func ddbCharacterId(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if m := ddbUrlPattern.FindStringSubmatch(s); len(m) == 2 {
		return m[1]
	}
	if m := ddbDigitsPattern.FindAllString(s, -1); len(m) > 0 {
		return m[len(m)-1]
	}
	return ""
}

// ddbCookie finds the CobaltSession cookie: the flag, then the environment,
// then the keyring, and only then the user.
func ddbCookie(flagValue string) (string, error) {
	if v := strings.TrimSpace(flagValue); v != "" {
		return ddbTrimCookie(v), nil
	}
	if v := strings.TrimSpace(os.Getenv(ddbCookieEnv)); v != "" {
		return ddbTrimCookie(v), nil
	}
	if v, err := keyring.Get("orgs", ddbKeyringUser); err == nil && strings.TrimSpace(v) != "" {
		fmt.Printf("%susing the D&D Beyond session saved in your keyring.%s\n", cDim, cReset)
		return ddbTrimCookie(v), nil
	}
	fmt.Printf(`
%sD&D Beyond has no login api, so this needs the session cookie your browser
already holds. To find it:%s

  1. sign in at %shttps://www.dndbeyond.com%s in your browser
  2. open the developer tools (F12) and go to
     Application (Chrome) or Storage (Firefox) -> Cookies -> https://www.dndbeyond.com
  3. copy the value of the %sCobaltSession%s cookie

%sIt is pasted hidden, is only used to ask D&D Beyond for a short lived token,
and is never sent to your orgs server.%s

`, cBold, cReset, cCyan, cReset, cBold, cReset, cDim, cReset)
	cookie := ""
	if err := survey.AskOne(&survey.Password{Message: "CobaltSession cookie:"},
		&cookie, survey.Required); err != nil {
		return "", fmt.Errorf("cancelled")
	}
	cookie = ddbTrimCookie(cookie)
	if cookie == "" {
		return "", fmt.Errorf("no cookie given")
	}
	save := false
	if err := survey.AskOne(&survey.Confirm{
		Message: "Remember this in your system keyring so you are not asked again?",
		Default: true,
	}, &save, nil); err == nil && save {
		if err := keyring.Set("orgs", ddbKeyringUser, cookie); err != nil {
			fmt.Printf("%scould not write to the keyring: %s%s\n", cGold, err, cReset)
		}
	}
	return cookie, nil
}

// ddbTrimCookie accepts either the value on its own or the whole
// "CobaltSession=..." pair copied out of a cookie header.
func ddbTrimCookie(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, ";")
	if i := strings.Index(v, "CobaltSession="); i >= 0 {
		v = v[i+len("CobaltSession="):]
	}
	if i := strings.Index(v, ";"); i >= 0 {
		v = v[:i]
	}
	return strings.Trim(strings.TrimSpace(v), `"`)
}

// ddbToken trades the session cookie for the short lived bearer token the
// character service wants, which is what the site itself does on page load.
func ddbToken(cookie string) (string, error) {
	req, err := http.NewRequest("POST", ddbTokenApi, bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ddbUserAgent)
	req.AddCookie(&http.Cookie{Name: "CobaltSession", Value: cookie})
	res, err := ddbClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach the D&D Beyond auth service: %s", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("D&D Beyond rejected that session cookie - it expires " +
				"when you sign out, so copy a fresh one (orgs dnd import -forget clears " +
				"the saved copy)")
		}
		return "", fmt.Errorf("the D&D Beyond auth service answered %d", res.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Token == "" {
		return "", fmt.Errorf("no token came back from D&D Beyond")
	}
	return out.Token, nil
}

// ddbFetch asks the character service for a character, with the bearer token
// when there is one. The status is returned rather than turned into an error
// so the caller can tell "private" from "broken".
func ddbFetch(id, token string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf(ddbCharacterApi, id), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ddbUserAgent)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := ddbClient().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("could not reach D&D Beyond: %s", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("could not read the character: %s", err)
	}
	return body, res.StatusCode, nil
}

const ddbUserAgent = "orgs-dnd/1.0 (+https://github.com/ihdavids/orgs)"

func ddbClient() *http.Client {
	return &http.Client{Timeout: 45 * time.Second}
}

// ddbForget drops the saved session cookie.
func ddbForget() {
	if err := keyring.Set("orgs", ddbKeyringUser, ""); err != nil {
		fmt.Printf("%scould not clear the keyring entry: %s%s\n", cRed, err, cReset)
		return
	}
	fmt.Printf("%sthe saved D&D Beyond session has been cleared.%s\n", cBold, cReset)
}
