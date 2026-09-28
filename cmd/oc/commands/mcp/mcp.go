package mcp

// `orgs mcp` — the org database, as tools an agent can call.
//
//	orgs mcp                    speak MCP on stdin and stdout
//	orgs mcp -read-only         offer only the tools that read
//	orgs mcp -list              what it would offer, and exit
//
// It is a *client*, not a second server: every tool here is one request to a
// running orgs server, which is what makes this worth having rather than a
// second implementation of the org database to keep in step with the first.
// Point it at a server the way you point anything else:
//
//	orgs -url http://localhost:8010 mcp
//
// In a Claude Code config that is:
//
//	{"mcpServers": {"orgs": {"command": "orgs", "args": ["mcp"]}}}
//
// ---------------------------------------------------------------------------
// Three things about the transport
//
// 1. **Stdout is the protocol.** One JSON object per line, and nothing else
//    ever. This is why `logToFile` in cmd/orgs/main.go writes the log to
//    stderr rather than to stdout - it used to do both, and a single "Loading:
//    orgs.yaml" line at startup ends the session before it begins. Anything
//    added here that prints must print to stderr.
//
// 2. **A notification has no id and gets no answer.** `notifications/
//    initialized` arrives right after the handshake and replying to it is a
//    protocol error, not a harmless extra.
//
// 3. **A tool that fails answers with isError, not a JSON-RPC error.** The
//    model is meant to see what went wrong and try something else; a transport
//    level error is for the client library and never reaches it. Only an
//    unknown method or unreadable json is a real error here.

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// The version of the spec this speaks. A client asking for a version we know
// gets that version back; anything else gets this one, which is the honest
// answer to "I do not speak what you asked for".
const protocolVersion = "2025-06-18"

var knownVersions = map[string]bool{
	"2024-11-05": true,
	"2025-03-26": true,
	"2025-06-18": true,
}

type Mcp struct {
	fset *flag.FlagSet

	ReadOnly bool
	List     bool
	Name     string
}

func (self *Mcp) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Mcp) StartPlugin(manager *common.PluginManager)         {}

func (self *Mcp) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.ReadOnly, "read-only", false, "offer only the tools that read - nothing that writes to your org files")
	fset.BoolVar(&self.List, "list", false, "print the tools that would be offered, and exit")
	fset.StringVar(&self.Name, "name", "orgs", "what to call this server in the handshake")
}

func (self *Mcp) Exec(core *commands.Core) {
	tools := Tools(self.ReadOnly)
	if self.List {
		// A Tool holds the function that runs it, which will not marshal, so
		// -json is handed the part of a tool that a caller could act on: the
		// name, what it is for, whether it writes, and the schema a client
		// would have to fill in.
		type listed struct {
			Name        string
			Short       string
			Description string
			Writes      bool
			Schema      map[string]interface{}
		}
		rows := make([]listed, 0, len(tools))
		for _, t := range tools {
			rows = append(rows, listed{t.Name, t.Short, t.Description, t.Writes, t.Schema})
		}
		commands.Render(rows, func() {
			for _, t := range tools {
				mark := " "
				if t.Writes {
					mark = commands.C(commands.AnsiGold) + "✎" + commands.C(commands.AnsiReset)
				}
				fmt.Printf("%s %s%-22s%s %s\n",
					mark, commands.C(commands.AnsiBold), t.Name, commands.C(commands.AnsiReset), t.Short)
			}
			fmt.Fprintf(os.Stderr, "\n%s✎ writes to your org files; -read-only leaves them out%s\n",
				commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		})
		return
	}
	// -dry-run is a real thing to run this under: it lets somebody watch what
	// an agent would write without letting it. Say so on stderr, because the
	// agent has no way to be told and the person watching does.
	if commands.DryRun {
		fmt.Fprintln(os.Stderr, "orgs mcp: dry run — every write will be described and refused")
	}
	self.serve(core, tools)
}

// ---------------------------------------------------------------------------
// JSON-RPC over stdio
// ---------------------------------------------------------------------------

type request struct {
	JsonRpc string          `json:"jsonrpc"`
	Id      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JsonRpc string          `json:"jsonrpc"`
	Id      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (self *Mcp) serve(core *commands.Core, tools []Tool) {
	in := bufio.NewScanner(os.Stdin)
	// A tool result can be large - a file's worth of headings, a source block's
	// output - and so can the call that carries code to run. bufio's 64k
	// default would cut one in half and end the session with a parse error.
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	byName := map[string]Tool{}
	for _, t := range tools {
		byName[t.Name] = t
	}

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			write(out, response{JsonRpc: "2.0", Error: &rpcError{-32700, "parse error: " + err.Error()}})
			continue
		}
		// No id means a notification: act on it and say nothing back.
		notification := len(req.Id) == 0 || string(req.Id) == "null"

		result, rerr := self.dispatch(core, byName, tools, req)
		if notification {
			continue
		}
		resp := response{JsonRpc: "2.0", Id: req.Id}
		if rerr != nil {
			resp.Error = rerr
		} else {
			resp.Result = result
		}
		write(out, resp)
	}
	if err := in.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "orgs mcp: %v\n", err)
	}
}

func write(out *bufio.Writer, resp response) {
	b, err := json.Marshal(resp)
	if err != nil {
		// Nothing useful to say on the wire if the answer will not marshal;
		// say it to the person instead and keep the session alive.
		fmt.Fprintf(os.Stderr, "orgs mcp: could not encode a reply: %v\n", err)
		return
	}
	out.Write(b)
	out.WriteByte('\n')
	out.Flush()
}

func (self *Mcp) dispatch(core *commands.Core, byName map[string]Tool, tools []Tool, req request) (interface{}, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		version := protocolVersion
		if knownVersions[p.ProtocolVersion] {
			version = p.ProtocolVersion
		}
		return map[string]interface{}{
			"protocolVersion": version,
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": self.Name, "version": "1"},
			"instructions": "Tools over an org-mode database served by orgs. " +
				"org_search takes the server's own query expression - IsTask(), " +
				"IsStatus(\"NEXT\"), HasProperty(\"EFFORT\"), Today(), " +
				"InCollection(\"contact\"), HasBacklinks(), LinksTo(re) - and " +
				"understands headings, keywords, tags and dates. org_find is a " +
				"regular expression over the raw text of every file and is the " +
				"one that finds a phrase written in a drawer, a table or a " +
				"source block. Headings are addressed by their Hash.",
		}, nil

	case "notifications/initialized", "notifications/cancelled":
		return nil, nil

	case "ping":
		return map[string]interface{}{}, nil

	case "tools/list":
		out := make([]map[string]interface{}, 0, len(tools))
		for _, t := range tools {
			out = append(out, map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.Schema,
			})
		}
		return map[string]interface{}{"tools": out}, nil

	case "tools/call":
		var p struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "bad params: " + err.Error()}
		}
		t, ok := byName[p.Name]
		if !ok {
			// Not knowing a tool is the model's mistake rather than the
			// client's, so it goes back in band where the model can read it.
			return toolError(fmt.Sprintf("there is no tool called %q", p.Name)), nil
		}
		if p.Arguments == nil {
			p.Arguments = map[string]interface{}{}
		}
		res, err := t.Run(core, p.Arguments)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolResult(res), nil
	}
	return nil, &rpcError{-32601, "no method called " + req.Method}
}

// A tool's answer travels as text, because that is the one content type every
// client renders. It is json text: the model reads structure better than it
// reads a table somebody laid out for a terminal, and it can be handed on to
// something else without being parsed out of prose first.
func toolResult(v interface{}) map[string]interface{} {
	text := ""
	switch s := v.(type) {
	case string:
		text = s
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return toolError("could not encode the answer: " + err.Error())
		}
		text = string(b)
	}
	if strings.TrimSpace(text) == "" || text == "null" || text == "[]" {
		// An empty answer and a failed one look identical to a model reading a
		// blank string, and it will usually assume the latter and try again.
		text = "(nothing matched)"
	}
	return map[string]interface{}{
		"content": []map[string]interface{}{{"type": "text", "text": text}},
	}
}

func toolError(msg string) map[string]interface{} {
	return map[string]interface{}{
		"isError": true,
		"content": []map[string]interface{}{{"type": "text", "text": msg}},
	}
}

func init() {
	commands.AddCmd("mcp", "serve the org database to an agent over MCP",
		func() commands.Cmd { return &Mcp{Name: "orgs"} })
}
