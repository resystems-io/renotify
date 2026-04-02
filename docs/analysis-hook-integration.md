# Claude Code Hook Integration Analysis

This document analyses the integration between Renotify and the Claude Code
hooks mechanism. It defines how the `renotify dispatch` command acts as a
hook handler, bridging Claude Code lifecycle events to mobile notifications
via the existing NATS transport.

For the hook events protocol, see the
[Claude Code Hooks Reference](https://code.claude.com/docs/en/hooks). For
the Renotify payload schemas, see the
[Payload Schema Analysis](analysis-payload-schemas.md).

This document addresses refinement item **R-CLI-19** (Hook Dispatcher).

---

## 1. Motivation

The Renotify architecture defines two ingress paths for agent-driven
notifications:

1. **CLI commands** (`renotify post`, `renotify ask`) — invoked directly by
   shell scripts and CI pipelines.
2. **MCP tools** (`post`, `ask`, `register_flow`, etc.) — invoked natively
   by AI agents connected to the daemon's MCP server.

However, modern AI coding agents such as Claude Code manage certain user
interactions *outside* the MCP tool loop. In particular:

- **Permission dialogs** — when Claude Code needs authorisation to execute a
  tool (e.g., run a shell command, edit a file), it presents an interactive
  permission prompt. This prompt is rendered in the terminal and blocks the
  agent loop until the user responds locally.
- **System notifications** — when Claude Code emits status notifications
  (e.g., "agent is idle and waiting for input", "permission needed"), these
  are delivered via the terminal bell or desktop notification and are
  invisible to a user who is away from the workstation.

Neither interaction type flows through MCP. They are part of Claude Code's
internal lifecycle, accessible only via the **hooks mechanism** — a
user-configured dispatch system that fires shell commands, HTTP requests, or
LLM prompts at specific lifecycle points.

Integrating Renotify with hooks enables a developer to approve or deny agent
tool calls from their mobile device, and to receive agent status
notifications remotely — completing the human-in-the-loop circle for
scenarios where the developer is not sitting at the terminal.

---

## 2. Relevant Hook Events

Claude Code defines over 25 hook events. Two are directly relevant to
Renotify's core value proposition:

### 2.1 PermissionRequest

**When it fires:** Each time Claude Code presents a permission dialog to
the user (e.g., "Allow Bash: `npm test`?").

**Matcher:** Filters on tool name (`Bash`, `Edit`, `Write`, `Read`,
`Agent`, `mcp__*`, etc.).

**Input fields (stdin JSON):**

| Field | Type | Description |
| :--- | :--- | :--- |
| `session_id` | string | Current Claude Code session |
| `cwd` | string | Working directory |
| `hook_event_name` | string | Always `"PermissionRequest"` |
| `tool_name` | string | Tool requesting permission |
| `tool_input` | object | Tool-specific parameters |
| `permission_suggestions` | array | "Always allow" options the user would normally see |

**Output (stdout JSON) — decision control:**

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PermissionRequest",
    "decision": {
      "behavior": "allow"
    }
  }
}
```

The `decision.behavior` field accepts `"allow"` or `"deny"`. A `"deny"`
decision accepts an additional `message` field that is shown to Claude as
the reason for denial.

**Mapping to Renotify:** This maps directly to `renotify ask` with a
boolean response type. The mobile notification presents the tool name and
input context; the user taps Allow or Deny; the response is translated
back to the hook decision JSON.

### 2.2 Notification

**When it fires:** Each time Claude Code emits a system notification
(terminal bell, desktop toast).

**Matcher:** Filters on `notification_type`: `permission_prompt`,
`idle_prompt`, `auth_success`, `elicitation_dialog`.

**Input fields (stdin JSON):**

| Field | Type | Description |
| :--- | :--- | :--- |
| `session_id` | string | Current Claude Code session |
| `cwd` | string | Working directory |
| `hook_event_name` | string | Always `"Notification"` |
| `message` | string | Notification body text |
| `title` | string | Notification title |
| `notification_type` | string | Event subtype |

**Output:** No decision control. The hook runs for side effects only.

**Mapping to Renotify:** This maps directly to `renotify post`
(fire-and-forget). The notification title and message are forwarded to the
mobile device as-is.

### 2.3 Events Deferred to Future Work

The following events are architecturally compatible but not in scope for
the initial implementation:

| Event | Potential Renotify mapping | Notes |
| :--- | :--- | :--- |
| `PreToolUse` | Auto-approve rules based on mobile-configured policy | Requires mobile UI for rule management |
| `AskUserQuestion` | Route Claude's questions to mobile via `ask` | Needs `updatedInput` with answers |
| `Elicitation` | Route MCP elicitations to mobile | Needs form schema rendering |
| `Stop` | Notify user when agent finishes | Low priority — covered by Notification |
| `StopFailure` | Alert user on API errors | Useful for unattended runs |

---

## 3. Transport Choice: Command vs HTTP

Claude Code hooks support two synchronous transport types that could carry
the Renotify dispatch logic:

### 3.1 Option A: Command Hook (`renotify dispatch` via stdio)

The hook handler is a shell command. Claude Code spawns the process, pipes
hook JSON to stdin, reads the decision from stdout, and interprets the
exit code.

**Advantages:**

- **Self-contained:** No additional daemon endpoint required. The
  `renotify` binary already knows how to connect to NATS.
- **Broker-agnostic:** Works identically with embedded and shared broker
  models. No assumption about daemon locality.
- **Simple configuration:** Hook config references a single command string.
- **Graceful degradation:** If the `renotify` binary is unavailable or
  NATS is unreachable, the process exits non-zero and Claude Code falls
  back to the normal interactive permission dialog.
- **No coupling:** Hook lifecycle is independent of daemon lifecycle. The
  command connects to NATS, does its work, and exits.

**Disadvantages:**

- **Process spawn overhead:** Each hook invocation starts a new process
  and establishes a NATS connection. For PermissionRequest this is
  negligible (human response time dominates), but Notification hooks may
  fire frequently.
- **Connection setup latency:** ~10-50ms to establish a loopback NATS TCP
  connection per invocation.

### 3.2 Option B: HTTP Hook (daemon endpoint)

The hook handler is an HTTP POST to the daemon's existing loopback HTTP
listener (port 4224).

**Advantages:**

- **No process spawn:** The daemon is already running; the HTTP handler
  reuses the existing NATS connection.
- **Lower latency:** No connection setup; the daemon publishes directly.

**Disadvantages:**

- **Daemon dependency:** The daemon must be running and healthy for hooks
  to function. If the daemon crashes, all hook dispatch fails silently
  (HTTP hooks treat connection failures as non-blocking errors).
- **Local-only:** Requires the daemon to be on the same host as Claude
  Code. This is currently always true, but the command approach is more
  portable.
- **Additional HTTP surface:** Adds hook-handling routes to the daemon's
  HTTP server, mixing MCP and hook concerns.
- **Configuration coupling:** Hook config must reference the correct
  `http://localhost:4224/hooks/...` URL.

### 3.3 Recommendation: Command Hook

The command approach is preferred for the following reasons:

1. **Human response time dominates.** For PermissionRequest, the hook
   blocks for seconds to minutes while the user decides on their phone.
   The ~20ms process spawn overhead is invisible.
2. **Graceful fallback.** A non-zero exit code causes Claude Code to fall
   back to the normal terminal permission prompt. This makes the hook
   purely additive — it never degrades the baseline experience.
3. **Simpler operational model.** The hook works if the `renotify` binary
   is on `$PATH`. No daemon health dependency for the hook path itself.
4. **Consistent with CLI design.** Renotify's CLI commands are
   short-lived processes that connect to NATS, do work, and exit. The
   dispatch command follows the same pattern.

The Notification event fires more frequently but has no decision control,
so latency is irrelevant — the command publishes fire-and-forget and exits
immediately. If profiling reveals the per-invocation NATS connection cost
is material, a future optimisation could use a Unix domain socket to the
daemon instead.

---

## 4. `renotify dispatch` Command Design

### 4.1 Invocation

```
renotify dispatch
```

No flags. The command reads the full hook input JSON from stdin, inspects
the `hook_event_name` field, and dispatches to the appropriate handler.
All context is derived from the JSON input.

The command is designed to be a universal hook handler — a single command
entry in the Claude Code hook configuration handles all supported event
types.

### 4.2 PermissionRequest Flow

```
stdin (JSON) → parse → compose notification → NATS ask → wait → translate → stdout (JSON)
```

1. **Read** the hook input JSON from stdin.
2. **Extract** `tool_name` and `tool_input`.
3. **Compose** a human-readable `NotificationRequest`:
   - **Title:** `"Permission: {tool_name}"` (e.g., "Permission: Bash")
   - **Body:** Tool-specific summary derived from `tool_input` (see
     §4.4).
   - **Source:** `"claude-code"` (or session-derived identifier)
   - **Response types:** `["boolean"]`
   - **Priority:** `"high"` (permission requests are blocking)
   - **Actions:** `["Allow", "Deny"]` (custom boolean labels)
4. **Publish** the `NotificationRequest` to JetStream (same path as
   `renotify ask`).
5. **Wait** for a `NotificationResponse` on the ephemeral response
   consumer.
6. **Translate** the mobile response to hook decision JSON:
   - `accepted: true` → `{"hookSpecificOutput": {"hookEventName": "PermissionRequest", "decision": {"behavior": "allow"}}}`
   - `accepted: false` → `{"hookSpecificOutput": {"hookEventName": "PermissionRequest", "decision": {"behavior": "deny", "message": "Denied by remote user via Renotify"}}}`
7. **Write** the decision JSON to stdout and exit 0.

### 4.3 Notification Flow

```
stdin (JSON) → parse → compose notification → NATS post → exit 0
```

1. **Read** the hook input JSON from stdin.
2. **Extract** `title`, `message`, and `notification_type`.
3. **Compose** a fire-and-forget `NotificationRequest`:
   - **Title:** The hook's `title` field (e.g., "Permission needed")
   - **Body:** The hook's `message` field
   - **Source:** `"claude-code"`
   - **Response types:** `["none"]`
   - **Priority:** `"normal"` (or `"high"` for `permission_prompt` type)
4. **Publish** to JetStream (same path as `renotify post`).
5. **Exit 0** immediately. No stdout output required.

### 4.4 Tool Input Summarisation

The `tool_input` object is tool-specific. The dispatch command should
extract a concise, human-readable summary for the mobile notification
body. The following table defines the extraction rules:

| Tool Name | Body Template | Example |
| :--- | :--- | :--- |
| `Bash` | `{command}` | `rm -rf node_modules` |
| `Edit` | `{file_path}` | `/home/user/src/main.go` |
| `Write` | `{file_path}` | `/home/user/src/new_file.go` |
| `Read` | `{file_path}` | `/home/user/src/config.json` |
| `Glob` | `{pattern}` in `{path}` | `**/*.ts` in `/home/user/src` |
| `Grep` | `/{pattern}/` in `{path}` | `/TODO.*fix/` in `/home/user/src` |
| `Agent` | `{subagent_type}: {description}` | `Explore: Find API endpoints` |
| `WebFetch` | `{url}` | `https://docs.example.com/api` |
| `WebSearch` | `{query}` | `react hooks best practices` |
| MCP tools | `{tool_name}` with raw input | `mcp__github__search_repositories` |
| Other | JSON-serialised `tool_input` | Fallback for unknown tools |

### 4.5 Timeout and Error Handling

**Ask timeout:** The PermissionRequest handler uses the same timeout
mechanism as `renotify ask` — the configurable `timeout.ask` setting
(default 5 minutes) plus the `timeout.ask_grace_period` safety buffer.

**Hook timeout alignment:** The Claude Code hook `timeout` field (in
seconds) should be configured to exceed the Renotify ask timeout. The
default hook timeout (600s = 10 minutes) already exceeds the default ask
timeout (300s = 5 minutes), so no special configuration is needed with
defaults.

**Graceful fallback on error:**

| Condition | Exit code | Claude Code behaviour |
| :--- | :--- | :--- |
| NATS unreachable | 1 | Non-blocking error; falls back to terminal prompt |
| Timeout (no mobile response) | 1 | Non-blocking error; falls back to terminal prompt |
| Mobile user allows | 0 | Permission granted |
| Mobile user denies | 0 | Permission denied with message |
| Stdin parse error | 1 | Non-blocking error; falls back to terminal prompt |
| Unsupported event type | 0 | Silent no-op; no stdout output |

The exit code 1 (non-blocking error) strategy is deliberate: it ensures
that hook failures never prevent Claude Code from functioning. The user
can always fall back to the local terminal prompt.

### 4.6 Diagnostic Output

Stderr is used for diagnostics (consistent with all other Renotify CLI
commands). In Claude Code, stderr from hooks is visible in verbose mode
(`Ctrl+O`) but does not affect hook processing.

---

## 5. Hook Configuration

### 5.1 Recommended Configuration

The following configuration handles both PermissionRequest and
Notification events. It should be placed in the project-local
`.claude/settings.local.json` (not committed to the repo) or the user's
`~/.claude/settings.json` (applies to all projects):

```json
{
  "hooks": {
    "PermissionRequest": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "renotify dispatch",
            "statusMessage": "Awaiting remote approval..."
          }
        ]
      }
    ],
    "Notification": [
      {
        "matcher": "idle_prompt",
        "hooks": [
          {
            "type": "command",
            "command": "renotify dispatch",
            "async": true
          }
        ]
      }
    ]
  }
}
```

**Design notes:**

- **PermissionRequest** has no matcher — all tool permission requests are
  forwarded to mobile. The user can add a matcher to restrict to specific
  tools (e.g., `"Bash|Edit|Write"`).
- **Notification** uses a matcher to forward only `idle_prompt`
  notifications. `permission_prompt` is excluded because the
  PermissionRequest hook already covers that case with a richer
  interactive notification (§10.4). `auth_success` and
  `elicitation_dialog` are excluded as they have low informational value
  for remote monitoring.
- **Notification** uses `async: true` because it is fire-and-forget. The
  command runs in the background without blocking Claude Code.
- **PermissionRequest** is synchronous (default) because it must block
  until the user responds.
- **`statusMessage`** customises the spinner text shown while waiting for
  the remote decision.

### 5.2 Selective Tool Filtering

For users who want remote approval only for dangerous operations:

```json
{
  "hooks": {
    "PermissionRequest": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "renotify dispatch",
            "statusMessage": "Awaiting remote approval..."
          }
        ]
      }
    ]
  }
}
```

### 5.3 Future: `renotify hooks install`

A convenience command to write the hook configuration automatically is a
candidate for future work but is not required for the initial
implementation. The configuration is static JSON and can be documented.

---

## 6. Relationship to Existing Renotify Primitives

The dispatch command does not introduce new transport primitives. It
reuses the existing infrastructure:

| Dispatch action | Renotify equivalent | Shared infrastructure |
| :--- | :--- | :--- |
| PermissionRequest | `renotify ask -r boolean` | `setupFlow`, JetStream publish, ephemeral consumer, response wait |
| Notification | `renotify post` | `setupFlow`, JetStream publish, lifecycle events |

Internally, `renotify dispatch` calls the same `flow.go` setup path and
NATS publish logic as the `ask` and `post` commands. The differences are:

1. **Input source:** stdin JSON instead of CLI flags.
2. **Output sink:** stdout JSON (hook protocol) instead of terminal text.
3. **Content derivation:** Title and body are composed from hook input
   fields, not from user-supplied `--title` and `--body` flags.
4. **Session context:** The `session_id` and `cwd` from the hook input
   are included in the notification metadata for traceability.

---

## 7. Mobile Rendering

Permission request notifications appear on the mobile device as
interactive notifications with **Allow** and **Deny** action buttons.
This uses the existing boolean response type rendering (M-03) with custom
action labels.

Example mobile notification for a Bash permission:

```
┌─────────────────────────────────────┐
│ Permission: Bash                    │
│ rm -rf node_modules                 │
│ claude-code                         │
│                                     │
│    [Allow]     [Deny]               │
└─────────────────────────────────────┘
```

Example mobile notification for an Edit permission:

```
┌─────────────────────────────────────┐
│ Permission: Edit                    │
│ /home/user/src/main.go              │
│ claude-code                         │
│                                     │
│    [Allow]     [Deny]               │
└─────────────────────────────────────┘
```

The notification priority is `high`, placing it in the urgent channel
(heads-up display) so the user notices immediately.

---

## 8. Security Considerations

### 8.1 Information Exposure

The hook input may contain file paths, shell commands, and code snippets
from the developer's workstation. These are transmitted over the existing
NATS WSS connection, which is secured by TLS with certificate pinning
(D-13, D-14). No additional security surface is introduced.

### 8.2 Denial-of-Service

A misbehaving agent could trigger excessive permission requests, flooding
the mobile device. This is mitigated by the existing per-flow rate limit
(R-CLI-16, default 60/min). Each dispatch invocation creates a new flow,
but the rate limit applies at the daemon level.

### 8.3 Decision Authenticity

The mobile response (allow/deny) travels over the same authenticated NATS
channel as all other Renotify responses. The ACL model (D-16) restricts
the mobile client to publishing on `.response` and `.interject` subjects
only. No additional authentication is needed.

---

## 9. Implementation Items

The hook integration adds the following items to the refinement plan:

### Requirement

**R-CLI-19: Hook Dispatcher**
**Statement:** Implement `renotify dispatch` as a universal Claude Code
hook handler. The command reads hook event JSON from stdin, discriminates
on `hook_event_name`, and dispatches to the appropriate Renotify flow:
`PermissionRequest` events are forwarded as interactive ask notifications
with boolean response; `Notification` events are forwarded as
fire-and-forget post notifications. Unsupported event types are silently
ignored. Non-zero exit on error ensures graceful fallback to the local
terminal prompt.
**Rationale:** Extends Renotify's reach to agent lifecycle events that
occur outside the MCP tool loop, enabling remote permission approval and
status monitoring for unattended agent sessions.
**Trace to Parent:** N-02, N-03
**Allocation:** Go CLI Application
**V&V Method:** Test

### Implementation Step

**C-15: Hook Dispatcher Command** — Implement `renotify dispatch` with
stdin/stdout JSON protocol, PermissionRequest→ask and Notification→post
mapping, tool input summarisation, and graceful fallback on error. Reuses
existing `setupFlow`, JetStream publish, and ephemeral consumer
infrastructure.

---

## 10. Design Decisions

### 10.1 Event Filtering: Matcher-Based, Not Dispatch-Internal

**Decision:** The `renotify dispatch` command does not filter events
internally. All filtering — which tool names trigger PermissionRequest
hooks, which `notification_type` values trigger Notification hooks — is
the responsibility of the Claude Code hook matcher configuration.

**Alternative considered:** The dispatch command could accept a
`--notification-types` flag or inspect `notification_type` internally to
skip low-value events (e.g., `auth_success`). This would provide a
second filtering layer inside the command itself.

**Rationale:** The Claude Code hook matcher already provides expressive
regex-based filtering at the configuration level (§5.1). Duplicating
this logic inside the dispatch command would create two places to
configure filtering, with subtle interaction effects. The dispatch
command should be a dumb pipe: it receives an event, translates it to a
Renotify notification, and exits. This keeps the command simple, makes
filtering behaviour fully visible in the hook configuration JSON, and
avoids the need to rebuild or reconfigure the binary when filtering
preferences change.

### 10.2 Session Identity in Source Field

**Decision:** Include the Claude Code `session_id` in the
`NotificationRequest.source` field, formatted as
`claude-code/{session_id}`. When `session_id` is absent (non-Claude-Code
usage or older hook protocol versions), fall back to `"claude-code"`.

**Rationale:** The `source` field already exists in the notification
payload and is rendered as sub-text on Android notifications. Including
the session ID enables the mobile app to visually distinguish permission
requests from concurrent Claude Code sessions without introducing new
payload fields.

### 10.3 `updatedPermissions` ("Allow and Remember")

**Decision:** Deferred to future work.

**Rationale:** Supporting "Allow and remember" requires the mobile UI to
present a three-way choice (Allow / Allow Always / Deny) and the
dispatch command to construct the `updatedPermissions` array with the
correct `destination`, `rules`, and `behavior` fields. The current
binary Allow/Deny is sufficient for the initial implementation.

### 10.4 Dual Notification Suppression

**Decision:** Handled via matcher configuration, not dispatch-internal
logic. The recommended Notification matcher (§5.1) should use
`idle_prompt` only — excluding `permission_prompt` — when the
PermissionRequest hook is also configured. This avoids duplicate mobile
notifications (one interactive from PermissionRequest, one informational
from Notification) for the same permission dialog.

**Alternative considered:** The dispatch command could track recently
dispatched PermissionRequest events and suppress Notification events
with `notification_type: "permission_prompt"` that arrive within a short
window. This was rejected because it introduces stateful logic into what
should be a stateless pipe, and the matcher configuration already
provides the necessary control.
