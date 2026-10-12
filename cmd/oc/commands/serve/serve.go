package serve

import (
	"flag"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Serve struct {
	ServePath string
	Port      int
	TLSPort   int
}

func (self *Serve) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Serve) StartPlugin(manager *common.PluginManager) {
}

func (self *Serve) SetupParameters(fset *flag.FlagSet) {

	// Zero means "not given": Exec only overrides the yaml's port:/tlsport:
	// with a flag that was actually passed. A literal default here (8010, 443)
	// used to win over the yaml on every run. The real defaults live in
	// ServerSettings.Init.
	fset.StringVar(&self.ServePath, "servepath", self.ServePath, "serve path")
	fset.IntVar(&self.Port, "port", 0, "serve port (default: port in the config, else 8010)")
	fset.IntVar(&self.TLSPort, "tlsport", 0, "tls serve port (default: tlsport in the config, else 443)")
}

func (self *Serve) Exec(core *commands.Core) {
	if self.ServePath != "" {
		core.ServerSettings.ServePath = self.ServePath
	}
	if self.Port != 0 {
		core.ServerSettings.Port = self.Port
	}
	if self.TLSPort != 0 {
		core.ServerSettings.TLSPort = self.TLSPort
	}
	core.ServerSettings.Validate()
	core.StartServer(core.ServerSettings)
}

/*
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  common.MaxMessageSize,
		WriteBufferSize: common.MaxMessageSize,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
*/

// TODO: Nuke below this with the websocket API now that it is no longer useful.
/*
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

// init function is called at boot
func init() {
	commands.AddCmd("serve", "Start orgs server",
		func() commands.Cmd {
			return &Serve{}
		})
}
