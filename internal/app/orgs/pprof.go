package orgs

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
)

// startPprof serves Go's profiler on the address in ORGS_PPROF (say
// `localhost:6060`), for looking at where the server's memory and time go:
// `go tool pprof http://localhost:6060/debug/pprof/heap`. Off unless asked
// for, and on its own listener so it is neither behind the api's login nor
// reachable through it.
func startPprof() {
	addr := os.Getenv("ORGS_PPROF")
	if addr == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "PPROF: http://%s/debug/pprof/\n", addr)
	go func() {
		if err := http.ListenAndServe(addr, http.DefaultServeMux); err != nil {
			fmt.Fprintf(os.Stderr, "PPROF: %v\n", err)
		}
	}()
}
