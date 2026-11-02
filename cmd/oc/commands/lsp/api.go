package lsp

// The orgs server, as this language server asks it things. Every call goes
// through here so that a token renewal (which rewrites the shared header) never
// runs beside a request reading it, and so lists completion wants on every
// keystroke are asked for once a minute rather than once a key.

import (
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type api struct {
	core *commands.Core
	// Read-locked by every request, write-locked by a renewal.
	mu sync.RWMutex

	cmu   sync.Mutex
	cache map[string]cached
}

type cached struct {
	at    time.Time
	value interface{}
	err   error
}

const cacheFor = time.Minute

func newApi(core *commands.Core) *api {
	return &api{core: core, cache: map[string]cached{}}
}

// renew asks for a new token. A language server lives as long as the editor,
// and a token an hour.
func (a *api) renew() bool {
	if a.core.Refresh == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.core.Refresh()
}

// unauthorized is whether an error is the server refusing the token, which a
// renewal may cure.
func unauthorized(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Unauthorized")
}

func get[T any](a *api, path string, ps map[string]string) (T, error) {
	a.mu.RLock()
	v, err := commands.SendReceiveGetErr[T](a.core, path, ps)
	a.mu.RUnlock()
	if unauthorized(err) && a.renew() {
		a.mu.RLock()
		v, err = commands.SendReceiveGetErr[T](a.core, path, ps)
		a.mu.RUnlock()
	}
	return v, err
}

// getCached is get, remembered for a minute under key. A failure is
// remembered too, so a server that is down costs one timeout a minute rather
// than one per keystroke.
func getCached[T any](a *api, key, path string, ps map[string]string) (T, error) {
	a.cmu.Lock()
	if c, ok := a.cache[key]; ok && time.Since(c.at) < cacheFor {
		a.cmu.Unlock()
		v, _ := c.value.(T)
		return v, c.err
	}
	a.cmu.Unlock()
	v, err := get[T](a, path, ps)
	a.cmu.Lock()
	a.cache[key] = cached{at: time.Now(), value: v, err: err}
	a.cmu.Unlock()
	return v, err
}

// forget drops what is cached, after this server changed something.
func (a *api) forget() {
	a.cmu.Lock()
	a.cache = map[string]cached{}
	a.cmu.Unlock()
}

// post is a POST that writes nothing on the server (the /lsp/ text edits),
// so -dry-run does not stop it. slow is for a block run, which takes as long
// as the program does.
func post[T any](a *api, path string, body interface{}, slow bool) (T, error) {
	do := func() (T, error) {
		a.mu.RLock()
		defer a.mu.RUnlock()
		rest := a.core.Rest
		if slow {
			rest.Timeout = 0
		}
		return common.RestPost[T](&rest, path, body)
	}
	v, err := do()
	if err != nil && a.renew() {
		v, err = do()
	}
	return v, err
}

// write is a POST that changes files on the server: refile, archive.
// -dry-run describes it and refuses (commands.ErrDryRun).
func write[REQ any, T any](a *api, path string, body *REQ) (T, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	core := *a.core
	core.Rest.Timeout = 0
	return commands.SendReceivePostErr[REQ, T](&core, path, body)
}
