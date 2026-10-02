# Notes on `orgs mcp`

Loaded when working in this directory. References like **Traps: line edits** point at the Traps list in the root `CLAUDE.md`.

## `orgs mcp`

The org database as MCP tools over stdin/stdout. A **client**, not a second server: each tool is one request to a running orgs server, so nothing is implemented twice. `cmd/oc/commands/mcp/mcp.go` is the transport, `tools.go` the table.

```json
{"mcpServers": {"orgs": {"command": "orgs", "args": ["mcp"]}}}
```

- **Stdout is the protocol**: one JSON object per line, nothing else (see **Traps: stdout is the answer**).
- **A notification has no id and gets no answer**; replying to `notifications/initialized` (right after the handshake) is a protocol error.
- **A failing tool answers with `isError`, not a JSON-RPC error**, so the model sees it and tries something else; transport errors never reach the model. Only an unknown method or unreadable json is a real error.
- **An empty answer is said out loud** ("(nothing matched)"): a model cannot tell blank from failure and will retry.
- **`-read-only` drops every writing tool** (smaller surface than trusting the agent). `-dry-run` works; its refusal comes back as readable text, not a retryable error.
- Every list tool takes a **`limit`** and says when it applied one, to protect the context window.
- `org_search` understands headings, keywords, tags and dates; `org_find` understands nothing and finds a phrase in a drawer, table or source block. Descriptions say which to use.

`orgs mcp -list` prints the table with writing tools marked; with `-json` it emits name, description, `Writes` and schema, not the `Tool` struct (it holds the run function and will not marshal).
