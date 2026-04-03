# Antigravity & go-sdk MCP Integration Run-Book

This document outlines the root cause and resolution for
the integration failure observed when attempting to connect
Google Deepmind's Antigravity assistant to a Model Context
Protocol (MCP) server written using the Go SDK
(`github.com/modelcontextprotocol/go-sdk`) over HTTP.

## Symptom

When defining the MCP server in
`~/.gemini/antigravity/mcp_config.json` via HTTP (e.g.,
`http://127.0.0.1:4224/mcp`), the Antigravity client fails
to load tools.

Directly polling the endpoint via `GET` reveals the
underlying server response:
```http
HTTP/1.1 400 Bad Request
Bad Request: GET requires an Mcp-Session-Id header
```

If the endpoint is polled via an initial `POST` request, the
server responds with a valid session:
```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Mcp-Session-Id: HSP2BTRUABWUNLNHZVQUY2NSCQ

event: message
data: {"jsonrpc":"2.0", ... }
```

## Root Cause: Transport Protocol Dialect Mismatch

The failure is caused by an incompatibility between the HTTP
transport implementation expected by the client and the one
provided by the server.

### 1. Antigravity's Expectation (Standard SSE Transport)

Antigravity implements the **Standard SSE Transport** defined
by the 2024-11-05 version of the MCP spec.

**Step-by-step handshake:**
1. **Stream Connection**: The client makes an HTTP `GET`
   request to the initial configuration URL (e.g. `/sse`)
   and holds the connection open to receive Server-Sent
   Events.
2. **Endpoint Assignment**: The server immediately sends an
   SSE event named `endpoint`. The payload contains the
   URL where the client must send messages (e.g.,
   `/sse?sessionid=xxx`).
3. **JSON-RPC Messages**: The client sends all JSON-RPC
   payloads (like `initialize`, `tools/list`) via `POST`
   requests to that endpoint.
4. **Asynchronous Responses**: The server processes the
   `POST`, returns `202 Accepted`, and sends the JSON-RPC
   response back over the open SSE stream.

*Reference Documentation:*
- [Antigravity MCP Integration Guide](https://antigravity.google/docs/mcp)
- [Official MCP Transports Specification (SSE)](https://modelcontextprotocol.io/docs/concepts/transports#sse)

### 2. go-sdk Implementation (Streamable HTTP Transport)

The `go-sdk` default handler (`mcp.StreamableHTTPHandler`)
implements the **Streamable HTTP Transport** from the
2025-03-26 spec revision:
- The client must first `POST` to the server with the
  initial JSON-RPC payload.
- The server generates a session and returns the
  `Mcp-Session-Id` header in the response.
- The client must include the `Mcp-Session-Id` on all
  subsequent requests, including `GET` for the event stream.

Because Antigravity sends a `GET` to initialize the stream,
the Streamable handler rejects it (`400 Bad Request`) due to
the missing `Mcp-Session-Id` header.

## Resolution: Dual Transport

The go-sdk v1.4.1 provides `mcp.SSEHandler` — a complete
Standard SSE transport implementation. Both handlers share
the same `mcp.Server` instance (same tools, same resources,
zero duplication).

```go
mcpSrv := mcp.NewServer(...)

// Streamable HTTP (2025-03-26 spec) — Claude Code, etc.
streamable := mcp.NewStreamableHTTPHandler(
    func(r *http.Request) *mcp.Server { return mcpSrv },
    nil)

// Standard SSE (2024-11-05 spec) — Antigravity, etc.
sse := mcp.NewSSEHandler(
    func(r *http.Request) *mcp.Server { return mcpSrv },
    nil)

http.Handle("/mcp", streamable)
http.Handle("/sse", sse)
```

The `SSEHandler` handles both `GET` (create session, stream
events) and `POST` (relay messages via `?sessionid=`) on a
single route. No separate `/message` endpoint is needed.

### Client Configuration

**Claude Code** (`~/.claude/settings.json`):
```json
{
  "mcpServers": {
    "renotify": {
      "type": "http",
      "url": "http://127.0.0.1:4224/mcp"
    }
  }
}
```

**Antigravity** (`~/.gemini/antigravity/mcp_config.json`):
```json
{
  "mcpServers": {
    "renotify": {
      "serverUrl": "http://127.0.0.1:4224/sse"
    }
  }
}
```

### Stdio Transport (recommended for Antigravity)

Initial testing (2026-04-03) showed Antigravity sending
Streamable HTTP dialect (POST-first) rather than Standard
SSE (GET-first), and failing Accept-header validation on
both `/mcp` and `/sse`. The most reliable transport for
Antigravity is **stdio**:

```json
{
  "mcpServers": {
    "renotify": {
      "command": "renotify",
      "args": ["mcp"]
    }
  }
}
```

The `renotify mcp` command bridges stdin/stdout to the
daemon's `mcp.Server` via NATS. It is a raw byte relay —
adding new MCP tools requires no CLI changes.

**Daemon restart behaviour:** The CLI exits immediately on
NATS disconnect. Antigravity detects subprocess death and
relaunches. Future improvement: persist `ServerSessionState`
(the SDK's `ServerSessionOptions.State` accepts pre-populated
`InitializeParams`/`InitializedParams`) and rehydrate on
reconnect. This would survive brief daemon restarts without
requiring the MCP client to re-initialize.

### Implementation Note: http.Flusher

The SDK's `writeEvent()` type-asserts `w.(http.Flusher)` to
push SSE events immediately. Any middleware wrapping
`http.ResponseWriter` (e.g. logging middleware) must forward
the `Flush()` call, or SSE events will be buffered
indefinitely.
