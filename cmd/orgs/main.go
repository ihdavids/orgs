//lint:file-ignore ST1006 allow the use of self
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"path/filepath"
	"time"

	//"github.com/golang-jwt/jwt/v5"

	"github.com/gorilla/mux"
	"github.com/rs/cors"

	"github.com/gorilla/websocket"
	"github.com/ihdavids/orgs/internal/app/orgs"
	"github.com/ihdavids/orgs/internal/common"
)

// Used by the deprecated Websocket API I need to nuke
// "encoding/json"
var db *Db = &Db{}

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

func main() {
	// Force config parsing right up front
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
}

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
