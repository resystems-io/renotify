# Antigravity & go-sdk MCP Integration Run-Book

This document outlines the root cause and solutions for the integration failure observed when attempting to connect Google Deepmind's Antigravity assistant to a Model Context Protocol (MCP) server written using the Go SDK (`github.com/modelcontextprotocol/go-sdk`) over HTTP.

## Symptom

When defining the MCP server in `~/.gemini/antigravity/mcp_config.json` via HTTP (e.g., `http://127.0.0.1:4224/mcp`), the Antigravity client fails to load tools.

Directly polling the endpoint via `GET` reveals the underlying server response:
```http
HTTP/1.1 400 Bad Request
Bad Request: GET requires an Mcp-Session-Id header
```

If the endpoint is polled via an initial `POST` request, the server responds with a valid session:
```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Mcp-Session-Id: HSP2BTRUABWUNLNHZVQUY2NSCQ

event: message
data: {"jsonrpc":"2.0", ... }
```

## Root Cause: Transport Protocol Dialect Mismatch

The failure is caused by an incompatibility between the HTTP transport implementation expected by the client and the one provided by the server.

### 1. Antigravity's Expectation (Standard SSE Transport)
Antigravity and standard MCP clients (like Claude Desktop) strictly implement the protocol's **Standard SSE Transport**. In this protocol, all JSON-RPC traffic (requests and responses) is multiplexed over a single `POST` endpoint dictated by the server during the handshake.

**Step-by-step handshake:**
1.  **Stream Connection**: The client makes an HTTP `GET` request to the initial configuration URL (typically mounted at `GET /sse`) and holds the connection open to receive Server-Sent Events.
2.  **Endpoint Assignment**: The server immediately sends an SSE event named `endpoint`. The payload of this event contains the specific URL/URI where the client is permitted to send messages (e.g., `/message`).
3.  **JSON-RPC Messages**: The client sends all JSON-RPC payloads (like `initialize`, or `tools/list`) via `POST` requests to that newly assigned `/message` endpoint.
4.  **Asynchronous Responses**: The server processes the `POST` payload, immediately returns an HTTP `202 Accepted` to acknowledge receipt, and asynchronously fires the JSON-RPC response back down the open `/sse` stream to the client.

*Reference Documentation:*
*   [Antigravity MCP Integration Guide](https://antigravity.google/docs/mcp)
*   [Official MCP Transports Specification (SSE)](https://modelcontextprotocol.io/docs/concepts/transports#sse)

### 2. go-sdk Implementation (Streamable HTTP Transport)
The `go-sdk` provides a handler (`mcp.StreamableHTTPHandler`) that implements the **Streamable HTTP Transport**, which follows a radically different handshake pattern.
*   **Initialization**: The client must first execute a `POST` request to the server with the initial JSON-RPC payload.
*   **Handshake**: The server generates a session, returns the `Mcp-Session-Id` as an HTTP header in the response, and streams the result.
*   **Subsequent Stream**: The client must include the `Mcp-Session-Id` header on all subsequent requests, including the `GET` request responsible for maintaining the event stream.

Because Antigravity sends a `GET` request to initialize the stream, the Go SDK intercepts it and rejects it (`400 Bad Request`) due to the missing `Mcp-Session-Id` header.

## Recommended Solutions

### Option 1: Use Stdio Transport (Recommended for Local Clients)
For local integrations running alongside the development environment, Stdio transport strips away session management complexity and universally works out-of-the-box with all official MCP clients.

1.  Update the server code (e.g., in `cli/internal/mcpserver`) to use `mcp.NewStdioServer()` instead of `mcp.NewStreamableHTTPHandler()`.
2.  Update `mcp_config.json` to launch the executable directly:
    ```json
    {
      "mcpServers": {
        "renotify": {
          "command": "/path/to/renotify",
          "args": ["run-mcp"]
        }
      }
    }
    ```

### Option 2: Implement a Standard SSE Adapter (For Remote HTTP Hosts)
If the server absolutely must be hosted remotely over HTTP, and needs to be compatible with clients strictly expecting the Standard SSE handshake (like Antigravity), you can run both transports side-by-side.

Because the underlying `mcp.Server` instance handles its own concurrent state, you can attach multiple transport handlers to the **same** server instance and mount them on different routes.

*   **Streamable HTTP**: Keep `mcp.StreamableHTTPHandler` at `/mcp` for clients requesting it.
*   **Standard SSE**: Mount a custom standard SSE adapter at `/sse` and `/message` for clients expecting standard semantics.

#### Coexistence Example

Although the `go-sdk` does not provide an out-of-the-box Standard SSE adapter at this time, an implementation that creates one would allow multiple HTTP dialects to coexist seamlessly:

```go
// 1. Initialize the core MCP Server (business logic)
server := mcp.NewServer(...)

// 2. Wrap it for Streamable Transport (Existing behaviour)
streamableHandler := mcp.NewStreamableHTTPHandler(server, ...)
http.Handle("/mcp/", streamableHandler)

// 3. Wrap it for Standard SSE Transport (Custom Adapter)
// Note: You must build or import a custom Standard SSE handler
standardSSEAdapter := NewStandardSSEHandler(server)
http.Handle("/sse", standardSSEAdapter.HandleSSEConnection)
http.Handle("/message", standardSSEAdapter.HandleIncomingMessages)

// 4. (Optional) Run Stdio concurrently (for local CLI/IDE integration)
go func() {
    stdioServer := mcp.NewStdioServer(server)
    stdioServer.Run()
}()

http.ListenAndServe(":4224", nil)
```

In this architecture, while other SDK clients could connect to `/mcp`, you would configure Antigravity to hit the `/sse` endpoint by updating `~/.gemini/antigravity/mcp_config.json` as follows:

```json
{
  "mcpServers": {
    "renotify": {
      "serverUrl": "http://127.0.0.1:4224/sse"
    }
  }
}
```

This setup seamlessly allows all clients, regardless of their preferred transport handshake, to resolve their JSON-RPC calls against the very same underlying server tools and resources.
