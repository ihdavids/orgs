package main

import (
	"log"
	"net/http"
	"net/rpc"

	"net/rpc/jsonrpc"
	"path/filepath"

	"io/ioutil"

	"github.com/gorilla/websocket"
	"github.com/ihdavids/orgs/internal/common"
	yaml "gopkg.in/yaml.v2"
)

type Config struct {
	Url string `yaml:"url"`
}

func (self *Config) Defaults() {
	self.Url = "ws://localhost:8010/org"
}

func (self *Config) ParseConfig() {
	self.Defaults()
	filename, _ := filepath.Abs("orgc.yaml")
	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		err = yaml.Unmarshal(yamlFile, self)
		if err != nil {
			panic(err)
		}
	}
}

func main() {
	var c Config
	c.ParseConfig()
	ws, res, err := dialer.Dial(c.Url, http.Header{})
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
	defer ws.Close()

	log.Println(res)

	handle(ws)
}

var dialer = websocket.Dialer{
	ReadBufferSize:  common.MaxMessageSize,
	WriteBufferSize: common.MaxMessageSize,
}

func handle(ws *websocket.Conn) {
	defer func() {
		ws.Close()
	}()

	rwc := &common.ReadWriteCloser{WS: ws}
	codec := jsonrpc.NewClientCodec(rwc)
	c := rpc.NewClientWithCodec(codec)
	// c := rpc.NewClient(rwc)

	for {
		args := &common.HelloArgs{Msg: "Hello, World"}
		var reply common.HelloReply
		err := c.Call("Comm.Hello", args, &reply)
		if err != nil {
			log.Printf("%v", err)
			break
		}
		log.Printf("%v", reply)
	}
}
