# MCP Server Testing Playbook

How to test the Renotify MCP server with protocol-level tools
and AI agent clients.

## Prerequisites

- Renotify daemon running (`renotify daemon start --foreground`)
- `curl` and `jq` installed
- Optional: paired mobile device (for end-to-end notification
  verification)

## 1. Protocol-Level Testing with curl

The MCP server uses Streamable HTTP transport on
`http://127.0.0.1:4224/mcp`. All requests are JSON-RPC 2.0
POSTs.

### 1.1 Initialize a Session

```bash
curl -s -X POST http://127.0.0.1:4224/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-06-18",
      "capabilities": {},
      "clientInfo": {"name": "curl-test", "version": "1.0"}
    }
  }' | jq .
```

Save the `Mcp-Session-Id` header from the response for
subsequent requests:

```bash
SESSION=$(curl -s -D - -X POST http://127.0.0.1:4224/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-06-18",
      "capabilities": {},
      "clientInfo": {"name": "curl-test", "version": "1.0"}
    }
  }' 2>/dev/null | grep -i mcp-session-id | tr -d '\r' | awk '{print $2}')

echo "Session: $SESSION"
```

### 1.2 Send Initialized Notification

After initialize, send the `initialized` notification:

```bash
curl -s -X POST http://127.0.0.1:4224/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{
    "jsonrpc": "2.0",
    "method": "notifications/initialized"
  }'
```

### 1.3 List Tools

```bash
curl -s -X POST http://127.0.0.1:4224/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list"
  }' | jq '.result.tools[].name'
```

Expected output: `register_flow`, `refresh_flow`,
`terminate_flow`, `post`, `ask`.

### 1.4 Register a Flow

```bash
curl -s -X POST http://127.0.0.1:4224/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "register_flow",
      "arguments": {
        "workspace_path": "/home/user/project",
        "label": "Test Flow"
      }
    }
  }' | jq .
```

Note the `flow_id` from the response.

### 1.5 Send a Post Notification

Replace `FLOW_ID` with the flow_id from register_flow:

```bash
curl -s -X POST http://127.0.0.1:4224/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "post",
      "arguments": {
        "flow_id": "FLOW_ID",
        "title": "Build Complete",
        "body": "All 42 tests passed."
      }
    }
  }' | jq .
```

If the mobile app is paired, the notification should appear.

### 1.6 Send an Ask Notification

```bash
curl -s -X POST http://127.0.0.1:4224/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{
    "jsonrpc": "2.0",
    "id": 5,
    "method": "tools/call",
    "params": {
      "name": "ask",
      "arguments": {
        "flow_id": "FLOW_ID",
        "title": "Deploy to production?",
        "response_types": ["boolean"],
        "priority": "high"
      }
    }
  }' | jq .
```

Note the `resource_uri` from the response.

### 1.7 Read the DecisionResource

Replace `RESOURCE_URI` with the resource_uri from ask:

```bash
curl -s -X POST http://127.0.0.1:4224/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{
    "jsonrpc": "2.0",
    "id": 6,
    "method": "resources/read",
    "params": {
      "uri": "RESOURCE_URI"
    }
  }' | jq .
```

Before the user responds, `decided` is `false`. After
responding on the mobile app, repeat the read — `decided`
should be `true` with the response fields populated.

### 1.8 Terminate the Flow

```bash
curl -s -X POST http://127.0.0.1:4224/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{
    "jsonrpc": "2.0",
    "id": 7,
    "method": "tools/call",
    "params": {
      "name": "terminate_flow",
      "arguments": {
        "flow_id": "FLOW_ID",
        "status": "completed"
      }
    }
  }' | jq .
```

---

## 2. Claude Code Configuration

Add the Renotify daemon as an MCP server in Claude Code's
settings. In `~/.claude/settings.json` or
`.claude/settings.local.json`:

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

Or add via CLI:

```bash
claude mcp add --transport http renotify http://127.0.0.1:4224/mcp
```

After restarting Claude Code (or running `/mcp`), the five
Renotify tools should appear in the available tools list.

---

## 3. Agent Testing Walkthrough

With the daemon running and Claude Code configured:

1. **Start a session.** Ask Claude Code to "register a Renotify
   flow for this workspace". It should call `register_flow`
   with the current working directory.

2. **Send a notification.** Ask it to "notify me on my phone
   that the build is complete". It should call `post` with the
   flow_id.

3. **Request approval.** Ask it to "ask me on my phone whether
   to deploy to production". It should call `ask` with
   `response_types: ["boolean"]`.

4. **Respond on mobile.** Tap Allow or Deny on the phone.

5. **Verify decision.** Claude Code should read the
   DecisionResource and report the user's decision.

6. **End the session.** Ask it to "terminate the flow". It
   should call `terminate_flow` with `status: "completed"`.

---

## 4. Troubleshooting

**Connection refused:**
The daemon is not running or the MCP port is misconfigured.
Check `renotify daemon status` and verify `mcp.port` in
`settings.json` (default: 4224).

**No tools listed:**
MCP is disabled. Start the daemon without `--no-mcp`, or set
`mcp.enabled: true` in settings.json.

**Notification not received on mobile:**
Check that the mobile app is paired and connected. Verify with
`nats sub` on the WSS listener (see `renotify daemon --help`
for diagnostics). The NATS connection status is shown in the
app's persistent notification.

**DecisionResource stays undecided:**
The mobile response may not have reached the daemon. Check the
daemon logs for "decision resolved" messages. If the mobile app
shows the notification but tapping a button does nothing, check
the Android logcat for `ActionReceiver` or `NatsService` errors.

---

## 5. Google Antigravity Configuration

Antigravity (Google's AI agent) supports MCP servers via a
local config file using either `stdio` or Standard SSE transport.

However, the Renotify daemon uses Streamable HTTP transport.

See: [Antigravity MCP Integration Analysis](mcp-antigravity-integration.md)
