//lint:file-ignore ST1006 allow the use of self
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/app/orgs"
	"github.com/ihdavids/orgs/internal/common"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

//"github.com/golang-jwt/jwt/v5"

// Used by the deprecated Websocket API I need to nuke
// "encoding/json"
//var db *Db = &Db{}

/*
func routingExample() {
    r := mux.NewRouter()

    r.HandleFunc("/login", login).Methods("POST")
    r.Handle("/books", authenticate(http.HandlerFunc(getBooks))).Methods("GET")

    fmt.Println("Server started on port :8000")
    log.Fatal(http.ListenAndServe(":8000", r))
}

curl -X POST http://localhost:8000/login -d '{"username":"admin", "password":"password"}' -H "Content-Type: application/json"
curl --cookie "token=<your_token>" http://localhost:8000/books
https://dev.to/neelp03/securing-your-go-api-with-jwt-authentication-4amj
*/

func logToFile() *os.File {
	f, err := os.OpenFile("orgs.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	//defer f.Close()
	// Stderr, not stdout. The log used to go to both stdout and the file,
	// which is invisible on a terminal and ruinous anywhere else: `orgs search
	// -json | jq` got "Loading: ..." in the middle of its json, and `orgs mcp`
	// speaks JSON-RPC over stdout, where one stray log line ends the session.
	// Nothing about a log line wants to be in the answer.
	wrt := io.MultiWriter(os.Stderr, f)
	log.SetOutput(wrt)
	//log.SetOutput(f)
	log.Println("--- [OrgS] ----------------------------------")
	return f
}

// A dash-word is a value somebody wrote with a minus in front of it to mean
// "take this off" - a tag, mostly. Deliberately narrow: it must look like a
// plain word, so a mistyped flag is still reported as a mistyped flag rather
// than being silently swallowed as a value.
var dashWordRe = regexp.MustCompile(`^-[A-Za-z0-9_@#%.]+$`)

// Whether the server for this command is running inside this process. Read by
// the token check, which has nothing to check against a server that issued no
// token.
var localMode bool

type refreshResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"ExpiresAt"`
}

// refreshToken calls POST /refresh to get a new token, updates the
// Authorization header on core.Rest, and persists the new token and expiry back
// to the config file.
//
// refreshToken renews the session and reports whether it managed to. The bool
// is what tells "your session was renewed" from "you have to log in again":
// without it an expiry in the past was treated as the end of the session even
// when the server would happily have issued a new token.
func refreshToken(core *commands.Core) bool {
	resp, err := common.RestPost[refreshResponse](&core.Rest, "refresh", &struct{}{})
	if err != nil || resp.Token == "" {
		return false
	}
	core.Rest.Header.Set("Authorization", "Bearer "+resp.Token)
	// Persist refreshed token and expiry to config file
	if data, err := os.ReadFile(core.ConfigFile); err == nil {
		lines := strings.Split(string(data), "\n")
		setLine := func(key, value string) {
			for i, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), key+":") {
					lines[i] = fmt.Sprintf("%s: %s", key, value)
					return
				}
			}
			lines = append(lines, fmt.Sprintf("%s: %s", key, value))
		}
		setLine("token", fmt.Sprintf("%q", resp.Token))
		setLine("tokenExpiry", fmt.Sprintf("%q", resp.ExpiresAt.Format(time.RFC3339)))
		os.WriteFile(core.ConfigFile, []byte(strings.Join(lines, "\n")), 0600)
	}
	return true
}

// startLocalServer runs the server inside this process for the length of one
// command, and hands back the url to talk to it on.
//
// Everything on the client side assumes a daemon is up. That is right for a
// desktop and wrong everywhere else: `orgs agenda` in a git hook, `orgs fmt
// -check` in CI, `orgs search` over ssh on a box where nobody has started
// anything. So -local makes this process the server for as long as it takes to
// answer, and leaves nothing running.
//
// Three things about it:
//
//  1. **The port is asked for rather than chosen.** Port 0 gets whatever is
//     free from the kernel, which matters because the whole point is running
//     where something else may already be on 8010 - including the user's own
//     real server, which this must not collide with or talk to by accident.
//  2. **It is http on the loopback and authentication is off.** There is
//     nothing to authenticate to: the listener is this process, reachable from
//     this machine, for one command. A token dance here would be ceremony with
//     no security in it.
//  3. **Readiness is polled, not assumed.** StartServer parses every org file
//     in the database before it answers anything, which on a large database is
//     seconds rather than milliseconds. The same rule whisperd.go follows, for
//     the same reason: "still loading" and "not there" need different words.
func startLocalServer(core *commands.Core) string {
	sets := orgs.Conf().Server
	if sets == nil {
		fmt.Fprintln(os.Stderr, "-local: no server settings to run with")
		os.Exit(1)
	}
	if dirs := orgs.Conf().LocalDirs; dirs != "" {
		sets.OrgDirs = strings.Split(dirs, ",")
		for i := range sets.OrgDirs {
			sets.OrgDirs[i] = strings.TrimSpace(sets.OrgDirs[i])
		}
	}
	if len(sets.OrgDirs) == 0 {
		fmt.Fprintln(os.Stderr, "-local: no org directories - pass -orgdir ./notes")
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "-local: could not find a free port: %v\n", err)
		os.Exit(1)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	// Handed straight back rather than passed in: StartServer calls
	// ListenAndServe itself, and holding the socket open until then would make
	// the bind it does fail.
	ln.Close()

	sets.Port = port
	// The https listener is the one that blocks, and there is no certificate to
	// serve one with here. Left on, StartServer would try and log.Fatal.
	sets.AllowHttps = false
	sets.NoAuth = true

	go orgs.StartServer(sets)

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	if !waitForServer(url, 3*time.Minute) {
		fmt.Fprintln(os.Stderr, "-local: the server did not come up")
		os.Exit(1)
	}
	return url
}

// waitForServer polls an endpoint that costs nothing until it answers. /status
// is that endpoint - it reads a list off the config and touches no file - and
// it is behind the auth middleware, so a 200 from it also says the middleware
// is in place and doing what -local expects of it.
func waitForServer(url string, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url + "/status")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

// Aliases allow for command line helpers for the orgs command line tool.
func expandAliases(args []string) []string {
	if len(args) > 0 {
		k := args[0]
		if v, ok := orgs.Conf().Aliases[k]; ok && len(v) > 0 {
			args = append(v, args[1:]...)
		}
	}
	return args
}

func main() {
	orgs.DefaultKeystore()
	f := logToFile()
	defer f.Close()
	orgs.Conf()
	core := commands.NewCore(orgs.Conf().Url, orgs.Conf().Server)
	core.StartServer = orgs.StartServer
	core.EditorTemplate = orgs.Conf().EditorTemplate
	core.ConfigFile = orgs.Conf().Config
	if orgs.Conf().Token != "" {
		core.Rest.Header.Set("Authorization", "Bearer "+orgs.Conf().Token)
	}
	core.Start()

	args := flag.Args()

	args = expandAliases(args)

	// -local (or -orgdir, which implies it) puts the server in this process.
	// Not for `serve`, which *is* the server, and not for `login`, which has
	// nothing to log in to.
	if (orgs.Conf().Local || orgs.Conf().LocalDirs != "") &&
		len(args) > 0 && args[0] != "serve" && args[0] != "login" {
		core.Rest.Url = startLocalServer(core)
		// Nothing is authenticated in local mode, so a stale token from the
		// config would be sent to a server that never issued it.
		core.Rest.Header.Del("Authorization")
		localMode = true
	}
	ran := false
	// Execute command line options.
	//
	// A map walked in map order, mutating `args` as it goes, and never stopping
	// once it has found its command: so a command whose *argument* happened to
	// be another command's name ran both of them, in whichever order the map
	// felt like that run. `orgs __complete tag ''` ran the completion and then
	// `orgs tag`; `orgs watch -file todo.org` would have been at the same risk.
	// Dispatch is one command, so it stops at one.
	for k := range commands.CmdRegistry {
		if len(args) > 0 && k == args[0] {
			// Check token expiry for commands that talk to the server.
			//
			// Two things this used to get wrong. It ran for commands with no
			// server to talk to, so `orgs completion zsh` - a shell script
			// printed from a table in this binary - refused to run because of a
			// token it was never going to send. And it exited on an expired
			// token *without trying the refresh*, so a session that could have
			// been renewed silently told people to log in again.
			_, offline := commands.CmdRegistry[k].Cmd.(commands.Offline)
			if !localMode && !offline && k != "login" && k != "serve" &&
				orgs.Conf().TokenExpiry != "" {
				if expiry, err := time.Parse(time.RFC3339, orgs.Conf().TokenExpiry); err == nil {
					// Try the renewal first and only give up if it fails: an
					// expiry in the past does not mean the session is gone, and
					// telling somebody to log in again when they did not have to
					// is the worst of the three outcomes.
					renewed := refreshToken(core)
					if !renewed && time.Now().After(expiry) {
						fmt.Fprintf(os.Stderr,
							"Token expired at %s. Run: orgs login  (or -local to run without a server)\n",
							expiry.Local().Format(time.RFC822))
						os.Exit(1)
					}
				}
			}
			v := commands.CmdRegistry[k]
			//fmt.Printf("KEY: %v -> %v\n", k, args[1:])
			mod := orgs.Conf().FindCommand(k)
			if mod == nil {
				mod = v.Cmd
				oldArgs := args
				args = []string{}
				if len(oldArgs) > 1 {
					args = oldArgs[1:]
				}
				// A command that takes dash-words - `orgs tag -someday` - gets
				// them off the line before the parse, because the flag package
				// would call one an undefined flag and exit. Only words this
				// command has not defined as a flag are taken, so -json stays
				// a flag everywhere.
				if dw, ok := mod.(commands.DashWords); ok && v.Flags != nil {
					var taken []string
					kept := []string{}
					for _, a := range args {
						if len(a) > 1 && a[0] == '-' && a != "--" &&
							v.Flags.Lookup(strings.TrimLeft(a, "-")) == nil &&
							dashWordRe.MatchString(a) {
							taken = append(taken, a)
							continue
						}
						kept = append(kept, a)
					}
					dw.TakeDashWords(taken)
					args = kept
				}
				if v.Flags != nil && nil != v.Flags.Parse(args) {
					panic(fmt.Sprintf("failed to parse arguments for: %s\n", k))
				}
			}
			mod.Exec(core)
			ran = true
			break
		}
	}

	// Nothing matched. `orgs` on its own and `orgs nonsense` are the same
	// question - "what can this do" - and both used to be answered with a
	// registry dump in map order, or with silence.
	if !ran {
		if len(args) > 0 {
			fmt.Fprintf(os.Stderr, "orgs: no command called %q\n\n", args[0])
		}
		if h := commands.Find("help"); h != nil {
			h.Cmd.Exec(core)
		}
		if len(args) > 0 {
			os.Exit(1)
		}
	}

	/*
		// Force config parsing right up front
		orgs.DefaultKeystore()
		orgs.Conf()
		orgs.GetDb().Watch()
		defer func() {
			orgs.GetDb().Close()
		}()
		fmt.Println("STARTING SERVER")
		//http.HandleFunc(orgs.Conf().ServePath, serveWs)
		//fileServer := http.FileServer(http.Dir("./web"))

		router := mux.NewRouter().StrictSlash(true)
		router.HandleFunc(orgs.Conf().ServePath, serveWs)
		// move ws up, prevent '/*' from covering '/ws' in not testing mux, httprouter has this bug.
		restApi(router)

		for i, path := range orgs.Conf().OrgDirs {
			if i == 0 {
				if fpath, err := filepath.Abs(path); err == nil {
					fmt.Printf("PREFIX: %s\n", fpath)
					fs := http.FileServer(http.Dir(fpath))
					tpath, _ := filepath.Abs(orgs.Conf().TemplateImagesPath)
					fmt.Printf("TEMP PATH: %s\n", tpath)
					internalfs := http.FileServer(http.Dir(tpath))
					tfpath, _ := filepath.Abs(orgs.Conf().TemplateFontPath)
					internalfontfs := http.FileServer(http.Dir(tfpath))
					router.PathPrefix("/images/").Handler(http.StripPrefix("/images", fs))
					router.PathPrefix("/orgimages/").Handler(http.StripPrefix("/orgimages", internalfs))
					router.PathPrefix("orgimages/").Handler(http.StripPrefix("orgimages", internalfs))
					router.PathPrefix("/orgfonts/").Handler(http.StripPrefix("/orgfonts", internalfontfs))
					router.PathPrefix("orgfonts/").Handler(http.StripPrefix("orgfonts", internalfontfs))
				}
			}
		}
		// END ROUTING TABLE PathPrefix("/") match '/*' request
		// This needs to be replaced by an org embedded mechanism so it's built in to orgs
		router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web")))

		// http.Handle(orgs.Conf().WebServePath, http.StripPrefix(orgs.Conf().WebServePath, fileServer))
		// This is annoying, I can't seem to handle binding to anything other than /
		//http.Handle("/", fileServer)
		//http.HandleFunc("/orgs", portal)
		startPlugins()

		// Allow http connections but only from localhost
		go func() {
			corsHandler := cors.Default().Handler(router)
			if orgs.Conf().AccessControl != "*" {
				corsPolicy := cors.New(cors.Options{
					AllowedOrigins:   []string{fmt.Sprintf("http://localhost:%d", orgs.Conf().Port)},
					AllowCredentials: true,
					//	// Enable Debugging for testing, consider disabling in production
					//	Debug: true,
				})
				corsHandler = corsPolicy.Handler(corsHandler)
			}
			//if orgs.Conf().AllowHttp {
			fmt.Printf("HTTP PORT: %d\n", orgs.Conf().Port)
			//fmt.Printf("WEB: %s\n", orgs.Conf().WebServePath)
			//fmt.Printf("ORG: %s\n", orgs.Conf().ServePath)
			err := http.ListenAndServe(fmt.Sprint(":", orgs.Conf().Port), corsHandler)
			if err != nil {
				log.Fatal("ListenAndServe: ", err)
			}
			//}
		}()

		corsHandler := cors.Default().Handler(router)
		if orgs.Conf().AccessControl != "*" {
			corsPolicy := cors.New(cors.Options{
				AllowedOrigins:   []string{orgs.Conf().AccessControl},
				AllowCredentials: true,
				//	// Enable Debugging for testing, consider disabling in production
				//	Debug: true,
			})
			corsHandler = corsPolicy.Handler(corsHandler)
		}
		// Allow https connections
		if orgs.Conf().AllowHttps {
			fmt.Printf("PORT: %d\n", orgs.Conf().TLSPort)
			//fmt.Printf("WEB: %s\n", orgs.Conf().WebServePath)
			servercrt := orgs.Conf().ServerCrt
			serverkey := orgs.Conf().ServerKey
			err := http.ListenAndServeTLS(fmt.Sprint(":", orgs.Conf().TLSPort), servercrt, serverkey, corsHandler)
			if err != nil {
				log.Fatal("ListenAndServeTLS: ", err)
			}
		}
		stopPlugins()
	*/
}

/*
var upgrader = websocket.Upgrader{
	ReadBufferSize:  common.MaxMessageSize,
	WriteBufferSize: common.MaxMessageSize,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func startPlugins() {
	for _, plug := range orgs.Conf().Plugins {
		plug.Start(db)
	}
}

func stopPlugins() {
	for _, plug := range orgs.Conf().Plugins {
		plug.Stop()
	}
}

// TODO: Nuke below this with the websocket API now that it is no longer useful.

func serveWs(w http.ResponseWriter, r *http.Request) {
	log.Println("serveWs")

	if r.Method != "GET" {
		log.Println("Method not allowed")
		http.Error(w, "Method not allowed", 405)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	handle(ws)
}

func wsping(ws *websocket.Conn, deadline time.Duration) error {
	return ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(deadline*time.Second))
}

func wsclose(ws *websocket.Conn, deadline time.Duration) error {
	return ws.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(deadline*time.Second))
}

func handle(ws *websocket.Conn) {
	defer func() {
		deadline := 1 * time.Second
		wsclose(ws, deadline)
		time.Sleep(deadline)
		ws.Close()
	}()

	ws.SetReadLimit(common.MaxMessageSize)
	ws.SetReadDeadline(time.Now().Add(common.PongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(common.PongWait))
		return nil
	})

	go func() {
		ticker := time.Tick(common.PongWait / 4)
		for range ticker {
			if err := wsping(ws, common.PongWait); err != nil {
				log.Println("Ping failed:", err)
				break
			}
		}
		wsclose(ws, 1)
	}()

	rwc := &common.ReadWriteCloser{WS: ws}
	s := rpc.NewServer()
	//comm := &Comm{}
	//s.Register(comm)
	s.Register(db)
	s.ServeCodec(jsonrpc.NewServerCodec(rwc))
	//s.ServeConn(rwc)
}
*/
