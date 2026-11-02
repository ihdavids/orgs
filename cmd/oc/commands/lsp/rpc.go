package lsp

// JSON-RPC with LSP's framing: a Content-Length header, a blank line, the
// body. Unlike `orgs mcp`'s one-object-a-line, the server here also asks the
// client things (workspace/applyEdit, window/showMessageRequest) and waits for
// the answer, so the reader hands responses to whoever is waiting on their id
// and requests run on their own goroutines.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync"
)

type message struct {
	JsonRpc string           `json:"jsonrpc"`
	Id      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
	errInternal       = -32603
	errNotInitialized = -32002
	errRequestFailed  = -32803
)

type conn struct {
	in  *bufio.Reader
	out io.Writer
	wmu sync.Mutex

	mu      sync.Mutex
	nextId  int
	waiting map[string]chan message
}

func newConn(in io.Reader, out io.Writer) *conn {
	return &conn{in: bufio.NewReaderSize(in, 1<<16), out: out, waiting: map[string]chan message{}}
}

// read is the next message, or io.EOF when the client has gone.
func (c *conn) read() (message, error) {
	var m message
	tp := textproto.NewReader(c.in)
	hdr, err := tp.ReadMIMEHeader()
	if err != nil {
		if err == io.EOF || strings.Contains(err.Error(), "EOF") {
			return m, io.EOF
		}
		return m, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(hdr.Get("Content-Length")))
	if err != nil || n < 0 {
		return m, fmt.Errorf("a message with no Content-Length")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(c.in, body); err != nil {
		return m, err
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return message{Error: &rpcError{Code: errParse, Message: err.Error()}}, nil
	}
	return m, nil
}

func (c *conn) write(m message) {
	m.JsonRpc = "2.0"
	body, err := json.Marshal(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "orgs lsp: could not encode a message:", err)
		return
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	fmt.Fprintf(c.out, "Content-Length: %d\r\n\r\n", len(body))
	c.out.Write(body)
	if f, ok := c.out.(interface{ Flush() error }); ok {
		f.Flush()
	}
}

func (c *conn) reply(id *json.RawMessage, result interface{}, err *rpcError) {
	m := message{Id: id, Error: err}
	if err == nil {
		raw, merr := json.Marshal(result)
		if merr != nil {
			m.Error = &rpcError{Code: errInternal, Message: merr.Error()}
		} else {
			m.Result = raw
		}
	}
	c.write(m)
}

func (c *conn) notify(method string, params interface{}) {
	raw, _ := json.Marshal(params)
	c.write(message{Method: method, Params: raw})
}

// call asks the client something and waits for the answer.
func (c *conn) call(method string, params interface{}, result interface{}) error {
	c.mu.Lock()
	c.nextId++
	key := "orgs-" + strconv.Itoa(c.nextId)
	ch := make(chan message, 1)
	c.waiting[strconv.Quote(key)] = ch
	c.mu.Unlock()
	raw, _ := json.Marshal(params)
	id := json.RawMessage(strconv.Quote(key))
	c.write(message{Id: &id, Method: method, Params: raw})
	m := <-ch
	if m.Error != nil {
		return fmt.Errorf("%s", m.Error.Message)
	}
	if result != nil && len(m.Result) > 0 {
		return json.Unmarshal(m.Result, result)
	}
	return nil
}

// answered hands a response to the call waiting for it, and says whether
// there was one.
func (c *conn) answered(m message) bool {
	if m.Id == nil || m.Method != "" {
		return false
	}
	c.mu.Lock()
	ch, ok := c.waiting[string(*m.Id)]
	delete(c.waiting, string(*m.Id))
	c.mu.Unlock()
	if ok {
		ch <- m
	}
	return true
}
