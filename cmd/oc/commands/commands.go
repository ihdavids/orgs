package commands

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/gorilla/rpc/json"
	"github.com/ihdavids/orgs/internal/common"
)

type Cmd interface {
	Unmarshal(unmarshal func(interface{}) error) error
	Exec(core *Core)
	SetupParameters(*flag.FlagSet)
}

type PluginDef struct {
	Name   string
	Plugin Cmd
}

type PluginIdentifier struct {
	Name string `yaml:"name"`
}

func (self *PluginDef) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var id = PluginIdentifier{""}
	res := unmarshal(&id)
	if res != nil {
		return res
	}
	if creator, ok := CmdRegistry[id.Name]; ok {
		self.Plugin = creator.Cmd
		self.Name = id.Name
		return self.Plugin.Unmarshal(unmarshal)
	}
	return fmt.Errorf("Failed to create plugin %s", id.Name)
}

// The poller registry has the definitions of all known pollers
type CmdCreator func() Cmd
type CmdParams func()

/*
// This can be used in a yaml file to handle flag parsing.
type CmdArgs struct {
	Have   bool
	Args   string
	Params CmdParams
}

func (i *CmdArgs) String() string {
	return "arguments for the module"
}

// This is set as part of calling the --module
func (i *CmdArgs) Set(value string) error {
	i.Args = value
	i.Have = true
	if i.Params != nil {
		i.Params()
	}

	//fmt.Printf("SET: %s\n", value)
	return nil
}
*/

type CmdCreatorThunk struct {
	Name string
	Cmd  Cmd
	//Args      *CmdArgs
	Usage string
	Flags *flag.FlagSet
}

var CmdRegistry = map[string]*CmdCreatorThunk{}

func AddCmd(name string, usage string, creator CmdCreator) {
	//fmt.Printf("ADDING PLUGIN: %s\n", name)
	CmdRegistry[name] = &CmdCreatorThunk{Name: name, Cmd: creator() /*Args: new(CmdArgs),*/, Usage: usage}
	//CmdRegistry[name].Args = flag.NewFlagSet(name, flag.ExitOnError )
	//flag.BoolVar(&CmdRegistry[name].Args.Have, name, false, usage)
}

func Find(name string) *CmdCreatorThunk {
	if v, ok := CmdRegistry[name]; ok {
		return v
	}
	return nil
}

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

type StartServerThunk func(sets *common.ServerSettings)

type Core struct {
	Messages       chan string
	Send           chan []byte
	Rest           common.Rest
	EditorTemplate []string
	StartServer    StartServerThunk
	ServerSettings *common.ServerSettings
}

func NewCore(rurl string, sets *common.ServerSettings) *Core {
	core := new(Core)
	core.Rest = common.Rest{Url: rurl, Header: http.Header{}}
	// TODO: Make this configurable
	core.Rest.Insecure()
	core.ServerSettings = sets
	return core
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = common.MaxMessageSize
)

func T(format string, args ...interface{}) {
	//return
	if _, file, line, ok := runtime.Caller(1); ok {
		msg := fmt.Sprintf(format, args...)
		log.Printf("%s:%d:%s", file, line, msg)
	} else {
		log.Printf("?:?:"+format, args...)
	}
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
/*
func (self *Core) readPump() {
	defer func() {
		//self.Messages <- c
		//self.ws.Close()
		self.conn.Close()
	}()
	self.conn.SetReadLimit(maxMessageSize)
	self.conn.SetReadDeadline(time.Now().Add(pongWait))
	self.conn.SetPongHandler(func(string) error { self.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := self.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		//message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))
		//c.hub.broadcast <- message
		T("READ: %s", message)
		self.Messages <- (string)(message)
	}
}
*/
func Decode[T any](data string, obj *T) {
	json.DecodeClientResponse(strings.NewReader(data), obj)
}

func ReceiveAndDecode[RESP any](core *Core, obj *RESP) {
	select {
	case res := <-core.Messages:
		T(" GOT MESSAGE (ReceiveAndDecode)")
		json.DecodeClientResponse(strings.NewReader(res), obj)
	case <-time.After(60 * time.Second):
		T("Failed to read after 15 seconds")
		log.Panic("Failed to read after X seconds")
	}
}

func EncodeAndSend[T any](core *Core, name string, args *T) {
	r, err := json.EncodeClientRequest(name, args)
	//log.Println("REQUEST: ", string(r))
	if err != nil {
		log.Println("ERROR: ", err)
	}
	core.SendData(r)
}

func SendReceiveRpc[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	T("SEND: %s", name)
	EncodeAndSend(core, name, args)

	T("RCV: %s", name)
	ReceiveAndDecode(core, resp)
}

func SendReceiveGet[RESP any](core *Core, name string, ps map[string]string, resp *RESP) {
	*resp = common.RestGet[RESP](&core.Rest, name, ps)
}

func SendReceivePost[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	*resp, _ = common.RestPost[RESP](&core.Rest, name, args)
}

func (core *Core) LaunchEditor(filename string, line int) {
	eargs := make([]string, len(core.EditorTemplate))
	copy(eargs, core.EditorTemplate)
	for i, v := range eargs {
		eargs[i] = strings.Replace(strings.Replace(v, "{filename}", filename, -1), "{linenum}", fmt.Sprintf("%d", line), -1)
	}
	cmnd := exec.Command(eargs[0], eargs[1:]...)
	//cmnd.Run() // and wait
	cmnd.Start()
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
/*
func (self *Core) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		self.conn.Close()
	}()
	for {
		select {
		case message, ok := <-self.Send:
			self.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				self.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := self.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(self.Send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-self.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			self.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := self.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
*/

func (self *Core) SendData(data []byte) {
	self.Send <- data
}

func (self *Core) Start() {
	/*
		self.Messages = make(chan string)
		self.Send = make(chan []byte)
		go func() {
			self.readPump()
		}()
		go func() {
			self.writePump()
		}()
	*/
}
