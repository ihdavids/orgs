package orgs

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

// listenAndServe is http.ListenAndServe(TLS) with localhostOnly applied: the
// sockets are bound to the loopback addresses rather than every interface, so
// another machine cannot even open a connection. 127.0.0.1 has to bind;
// [::1] is added when the machine has it, because a browser asked for
// localhost may try that first. It blocks like the calls it replaces, and a
// listener that fails is fatal, as it was for them.
func listenAndServe(localOnly bool, port int, crt, key string, handler http.Handler) {
	addrs := []string{fmt.Sprint(":", port)}
	if localOnly {
		addrs = []string{fmt.Sprintf("127.0.0.1:%d", port), fmt.Sprintf("[::1]:%d", port)}
	}
	var lns []net.Listener
	for i, addr := range addrs {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			if i == 0 {
				log.Fatal("Listen: ", err)
			}
			fmt.Fprintf(os.Stderr, "localhostOnly: not listening on %s: %v\n", addr, err)
			continue
		}
		lns = append(lns, ln)
	}
	serve := func(ln net.Listener) {
		var err error
		if crt != "" {
			err = http.ServeTLS(ln, handler, crt, key)
		} else {
			err = http.Serve(ln, handler)
		}
		if err != nil {
			log.Fatal("Serve: ", err)
		}
	}
	for _, ln := range lns[1:] {
		go serve(ln)
	}
	serve(lns[0])
}

// localhostOnly refuses, with a 403, a request that is not from this machine
// or that does not name this machine. Binding to loopback already keeps other
// machines out; this is the second lock, and the one that also stops DNS
// rebinding - a page on some other site whose name has been pointed at
// 127.0.0.1, which the browser would otherwise let read this server's
// answers. That request comes from loopback, but its Host is the other site's
// name.
func localhostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackAddr(r.RemoteAddr) || !isLoopbackHost(r.Host) {
			fmt.Fprintf(os.Stderr, "localhostOnly: refused %s %s from %s (host %q)\n", r.Method, r.URL.Path, r.RemoteAddr, r.Host)
			http.Error(w, "this server only answers requests from localhost", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackAddr is true for a "host:port" remote address on 127.0.0.0/8 or ::1.
func isLoopbackAddr(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isLoopbackHost is true for a Host header naming this machine: localhost, a
// name under .localhost (which resolves to loopback by definition) or a
// loopback address.
func isLoopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
