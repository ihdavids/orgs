package orgs

/* SDOC: API
* GET /server/status — What the Server Is Doing
	Everything worth knowing about a running server in one answer: its version
	and uptime, the org directories and files it has loaded (and the ones it
	could not read, with why), the reload counter, every open =/events= stream
	with the address holding it, the plugins and whether they started and are
	still updating, the TLS certificate and when it runs out, what the embedded
	worg was built from, and memory.

	=orgs doctor= reads it; =orgs doctor -v= shows all of it. The same object is
	published to Go's expvar, so =GET /debug/vars= answers it too, as =orgs=,
	beside the runtime's own =memstats= and =cmdline=.

	*Method:* =GET=

	*Example:*
	#+BEGIN_SRC sh
	curl -s -H "Authorization: Bearer $TOKEN" https://localhost:8010/server/status | jq .Streams
	#+END_SRC
EDOC */

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"expvar"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/worg"
)

var serverStarted = time.Now()

// Files that could not be read or parsed, with why. Its own lock rather than
// the database's: LoadFile notes a parser panic from its recover, where the
// database lock may still be held.
var loadErrors = struct {
	sync.Mutex
	m    map[string]common.FileError
	last time.Time
}{m: map[string]common.FileError{}}

// noteLoadError records that a file failed to load, or with why == "" that it
// loaded (or is gone), which clears it.
func noteLoadError(filename, why string) {
	loadErrors.Lock()
	defer loadErrors.Unlock()
	if why == "" {
		delete(loadErrors.m, filename)
		loadErrors.last = time.Now()
		return
	}
	loadErrors.m[filename] = common.FileError{File: filename, Error: why, At: time.Now()}
}

// Org directories the watcher could not add (canFailWatch lets the server
// start anyway), with why: changes there are not seen until a restart.
var watchErrors = struct {
	sync.Mutex
	m map[string]string
}{m: map[string]string{}}

func noteWatchError(dir string, err error) {
	watchErrors.Lock()
	defer watchErrors.Unlock()
	watchErrors.m[dir] = err.Error()
}

func registerServerStatusRoutes(api *mux.Router) {
	api.HandleFunc("/server/status", RequestServerStatus).Methods("GET")
	// expvar's handler lists every published variable; the runtime's are
	// published by the package itself.
	api.Handle("/debug/vars", expvar.Handler()).Methods("GET")
}

var publishStatus sync.Once

func RequestServerStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(serverStatus())
}

func init() {
	publishStatus.Do(func() {
		expvar.Publish("orgs", expvar.Func(func() any { return serverStatus() }))
	})
}

func serverStatus() common.ServerStatus {
	st := common.ServerStatus{
		Version: common.BuildVersion(),
		Started: serverStarted,
		Uptime:  time.Since(serverStarted).Round(time.Second).String(),
		Plugins: common.PluginStates(),
		Worg:    worgBuild(worg.Content),
	}
	st.Streams, st.DroppedEvents = Streams()
	sets := Conf().Server
	if sets != nil {
		st.Files.OrgDirs = sets.OrgDirs
		for _, d := range sets.OrgDirs {
			if _, err := os.ReadDir(d); err != nil {
				st.Files.MissingDirs = append(st.Files.MissingDirs, d)
			}
		}
		st.Auth = common.AuthStatus{NoAuth: sets.NoAuth,
			TokenExpiry: sets.GetTokenExpiry().String(), Refresh: sets.GetRefreshExpiry().String()}
		if sets.AllowHttps {
			st.Cert = certStatus(sets.ServerCrt, time.Now())
		}
	}
	if db := GetDb(); db != nil {
		db.dblock.RLock()
		st.Files.Loaded = len(db.ByFile)
		st.Files.ReloadIndex = db.ReloadIndex
		st.Files.Watching = db.watcher != nil
		db.dblock.RUnlock()
	}
	watchErrors.Lock()
	for dir, why := range watchErrors.m {
		st.Files.WatchErrors = append(st.Files.WatchErrors, common.FileError{File: dir, Error: why})
	}
	watchErrors.Unlock()
	loadErrors.Lock()
	st.Files.LastReload = loadErrors.last
	for _, e := range loadErrors.m {
		st.Files.Unreadable = append(st.Files.Unreadable, e)
	}
	loadErrors.Unlock()
	sort.Slice(st.Files.Unreadable, func(a, b int) bool { return st.Files.Unreadable[a].File < st.Files.Unreadable[b].File })

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	st.Runtime = common.RuntimeStatus{Goroutines: runtime.NumGoroutine(),
		HeapMB: float64(ms.HeapAlloc) / (1 << 20), SysMB: float64(ms.Sys) / (1 << 20), NumGC: ms.NumGC}
	return st
}

// certStatus reads the certificate the https listener serves (the first one in
// the file) and says when it runs out.
func certStatus(file string, now time.Time) *common.CertStatus {
	c := &common.CertStatus{File: file}
	b, err := os.ReadFile(file)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	block, _ := pem.Decode(b)
	if block == nil {
		c.Error = "no PEM certificate in it"
		return c
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	c.Subject = cert.Subject.CommonName
	c.NotAfter = cert.NotAfter
	c.DaysLeft = int(cert.NotAfter.Sub(now).Hours() / 24)
	if cert.NotAfter.Before(now) && c.DaysLeft == 0 {
		c.DaysLeft = -1
	}
	return c
}

// worgBuild reads build.json out of the embedded worg (tools/buildworg.sh).
func worgBuild(fsys fs.FS) common.WorgBuild {
	var b common.WorgBuild
	if _, err := fs.Stat(fsys, "index.html"); err == nil {
		b.Present = true
	}
	if raw, err := fs.ReadFile(fsys, "build.json"); err == nil {
		if err := json.Unmarshal(raw, &b); err != nil {
			fmt.Fprintln(os.Stderr, "worg/build.json:", err)
		}
	}
	return b
}
