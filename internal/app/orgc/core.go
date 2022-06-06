package orgc

import (
	"net/rpc"
	"unicode"
	"time"
	//"bytes"
	"strings"
	"log"
	"runtime"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/gorilla/websocket"
	"github.com/gorilla/rpc/json"
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

type Core struct {
	app              *tview.Application
	layout, contents *tview.Flex

	ws          *rpc.Client
	conn        *websocket.Conn
	statusBar   *StatusBar
	projectPane *ProjectPane
	taskPane    *TaskPane
	Messages    chan string
	Send        chan []byte

	//taskDetailPane    *TaskDetailPane
	//projectDetailPane *ProjectDetailPane
}

func NewCore(c *rpc.Client, ws *websocket.Conn) *Core {
	core := new(Core)
	core.app = tview.NewApplication()
	core.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(makeTitleBar(), 2, 1, false).
		AddItem(prepareContentPages(core), 0, 2, true).
		AddItem(prepareStatusBar(core), 1, 1, false)
	//core.ws = c
	core.conn = ws
	setKeyboardShortcuts(core)

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
	maxMessageSize = 1024*64
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
		T("READ: %s",message)
		self.Messages <- (string)(message)
	}
}

func Decode[T any](data string, obj *T) {
	json.DecodeClientResponse(strings.NewReader(data), obj)
}

func ReceiveAndDecode[RESP any](core *Core, obj *RESP) {
    select {
    case res := <-core.Messages:
		T(" GOT MESSAGE (ReceiveAndDecode)")
		json.DecodeClientResponse(strings.NewReader(res), obj)
    case <-time.After(15 * time.Second):
		T("Failed to read after 15 seconds")
		log.Panic("Failed to read after X seconds")
    }
}

func EncodeAndSend[T any](core *Core, name string, args *T) {
	r,err := json.EncodeClientRequest(name, args)
	if err != nil {
		log.Println("ERROR: ",err)
	}
	core.SendData(r)
}

func SendReceiveRpc[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	T("SEND: %s",name)
	EncodeAndSend(core, name, args)

	T("RCV: %s",name)
	ReceiveAndDecode(core, resp)
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
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

func (self *Core) SendData(data []byte) {
	self.Send <- data
}

func (self *Core) Start() {
	self.Messages = make(chan string)
	self.Send = make(chan []byte)
	go func () {
		self.readPump()
	}()
	go func() {
		self.writePump()
	}()
	if err := self.app.SetRoot(self.layout, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func makeTitleBar() *tview.Flex {
	titleText := tview.NewTextView().SetText("[lime::b]OrgC [::-]- Org Cli").SetDynamicColors(true)
	versionInfo := tview.NewTextView().SetText("[::d]Version: 0.0.1").SetTextAlign(tview.AlignRight).SetDynamicColors(true)

	return tview.NewFlex().
		AddItem(titleText, 0, 2, false).
		AddItem(versionInfo, 0, 1, false)
}

func prepareContentPages(core *Core) *tview.Flex {
	core.projectPane = NewProjectPane(core)
	core.taskPane = NewTaskPane(core)
	//core.projectDetailPane = NewProjectDetailPane()
	//core.taskDetailPane = NewTaskDetailPane(taskRepo)

	core.contents = tview.NewFlex().
		AddItem(core.projectPane, 25, 1, true).
		AddItem(core.taskPane, 0, 2, false)

	return core.contents
}

func (self *Core) AskYesNo(text string, f func()) {

	activePane := self.app.GetFocus()
	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Yes" {
				f()
			}
			self.app.SetRoot(self.layout, true).EnableMouse(true)
			self.app.SetFocus(activePane)
		})

	pages := tview.NewPages().
		AddPage("background", self.layout, true, true).
		AddPage("modal", modal, true, true)
	_ = self.app.SetRoot(pages, true).EnableMouse(true)
}

func setKeyboardShortcuts(core *Core) *tview.Application {
	return core.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if ignoreKeyEvt(core) {
			return event
		}

		// Global shortcuts
		switch unicode.ToLower(event.Rune()) {
		case 'p':
			core.app.SetFocus(core.projectPane)
			//contents.RemoveItem(taskDetailPane)
			return nil
		case 'q':
		case 't':
			core.app.SetFocus(core.taskPane)
			//contents.RemoveItem(taskDetailPane)
			return nil
		case ':':
			core.statusBar.commandPalette()
			return nil
		}

		// Handle based on current focus. Handlers may modify event
		switch {
		case core.projectPane.HasFocus():
			event = core.projectPane.handleShortcuts(event)
		case core.taskPane.HasFocus():
			//event = core.taskPane.handleShortcuts(event)
			/*
				if event != nil && projectDetailPane.isShowing() {
					event = projectDetailPane.handleShortcuts(event)
				}
			*/
		}
		return event
	})
}
