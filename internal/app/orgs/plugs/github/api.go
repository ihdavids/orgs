package github

// Talking to the api: one request at a time, every page of a list, and a
// rate limit treated as "come back later" rather than as an error to retry
// straight away (which is how a poller gets its token blocked for an hour).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var errNotFound = errors.New("not found")

// rateLimited is the api saying stop; until is when it said to come back.
type rateLimited struct{ until time.Time }

func (e *rateLimited) Error() string {
	return "rate limited until " + e.until.Local().Format("15:04:05")
}

// The most pages read for one list. A person's assigned issues fit in one or
// two; this is only there so a misconfigured query cannot loop for ever.
const maxPages = 20

func (self *Forge) request(method, url string, body interface{}) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	if !strings.HasPrefix(url, "http") {
		url = self.baseURL() + url
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	tok := self.token()
	if self.provider == "gitlab" {
		if tok != "" {
			req.Header.Set("PRIVATE-TOKEN", tok)
		}
	} else {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	return req, nil
}

// call does one request and decodes the answer into out. It answers with the
// next page's url when the list has one.
func (self *Forge) call(method, url string, body, out interface{}) (string, error) {
	if wait := self.backoffUntil; time.Now().Before(wait) {
		return "", &rateLimited{until: wait}
	}
	req, err := self.request(method, url, body)
	if err != nil {
		return "", err
	}
	client := self.client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))

	if until, limited := rateLimit(resp); limited {
		self.backoffUntil = until
		return "", &rateLimited{until: until}
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", errNotFound
	case resp.StatusCode == http.StatusUnauthorized:
		// Forget a keyring token that no longer works, so a new one put there
		// is picked up without restarting the server.
		self.tok, self.triedKeyring = "", false
		return "", fmt.Errorf("%s %s: the token was refused (401) - %s", method, req.URL.Path, apiMessage(data))
	case resp.StatusCode >= 300:
		return "", fmt.Errorf("%s %s: %s - %s", method, req.URL.Path, resp.Status, apiMessage(data))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return "", fmt.Errorf("%s %s: could not read the answer: %v", method, req.URL.Path, err)
		}
	}
	return nextPage(resp), nil
}

// getAll reads every page of a list, handing each page's body to page.
func (self *Forge) getAll(url string, page func(data json.RawMessage) error) error {
	for n := 0; url != "" && n < maxPages; n++ {
		var raw json.RawMessage
		next, err := self.call("GET", url, nil, &raw)
		if err != nil {
			return err
		}
		if err := page(raw); err != nil {
			return err
		}
		url = next
	}
	return nil
}

var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// Both apis send a Link header; GitLab's keyset pagination sends only that,
// and its offset pagination X-Next-Page as well.
func nextPage(resp *http.Response) string {
	if m := linkNextRe.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
		return m[1]
	}
	if n := resp.Header.Get("X-Next-Page"); n != "" {
		u := *resp.Request.URL
		q := u.Query()
		q.Set("page", n)
		u.RawQuery = q.Encode()
		return u.String()
	}
	return ""
}

// Whether an answer is a rate limit, and when to come back. GitHub says so
// with a 403 or 429 and X-RateLimit-Remaining: 0 (secondary limits only with
// Retry-After); GitLab with a 429.
func rateLimit(resp *http.Response) (time.Time, bool) {
	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusForbidden {
		return time.Time{}, false
	}
	h := resp.Header
	exhausted := h.Get("X-RateLimit-Remaining") == "0" || h.Get("RateLimit-Remaining") == "0"
	if resp.StatusCode == http.StatusForbidden && !exhausted && h.Get("Retry-After") == "" {
		return time.Time{}, false // a real "forbidden"
	}
	if s, err := strconv.Atoi(h.Get("Retry-After")); err == nil && s > 0 {
		return time.Now().Add(time.Duration(s) * time.Second), true
	}
	for _, k := range []string{"X-RateLimit-Reset", "RateLimit-Reset"} {
		if s, err := strconv.ParseInt(h.Get(k), 10, 64); err == nil && s > 0 {
			if t := time.Unix(s, 0); t.After(time.Now()) {
				return t, true
			}
		}
	}
	return time.Now().Add(time.Minute), true
}

func apiMessage(data []byte) string {
	var m struct {
		Message interface{} `json:"message"`
		Error   string      `json:"error"`
	}
	if json.Unmarshal(data, &m) == nil {
		if m.Message != nil {
			return fmt.Sprint(m.Message)
		}
		if m.Error != "" {
			return m.Error
		}
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
