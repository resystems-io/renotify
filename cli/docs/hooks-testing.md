# Hook Dispatcher Testing Playbook

How to test the `renotify dispatch` command — the Claude Code
hook handler that bridges permission dialogs and system
notifications to mobile via Renotify.

## Prerequisites

- Renotify daemon running (`renotify daemon start --foreground`)
- `renotify` binary on `$PATH`
- Optional: paired mobile device (for end-to-end notification
  verification)

## 1. Protocol-Level Testing with Shell Pipes

The dispatch command reads hook JSON from stdin. All testing can
be done by piping JSON directly.

### 1.1 Unsupported Event (silent no-op)

```bash
echo '{"hook_event_name":"PreToolUse"}' | renotify dispatch
echo $?  # 0
```

No stdout, no side effects. Exit 0.

### 1.2 Notification Dispatch (fire-and-forget)

```bash
echo '{
  "session_id": "test-session",
  "cwd": "/home/user/project",
  "hook_event_name": "Notification",
  "title": "Agent idle",
  "message": "Waiting for user input",
  "notification_type": "idle_prompt"
}' | renotify dispatch
echo $?  # 0
```

No stdout. The notification should appear on the paired mobile
device. Verify with daemon logs (`--foreground` mode).

### 1.3 PermissionRequest Dispatch (blocking)

This blocks until a response arrives or the timeout expires.
Run in the background or a separate terminal:

```bash
echo '{
  "session_id": "test-session",
  "cwd": "/home/user/project",
  "hook_event_name": "PermissionRequest",
  "tool_name": "Bash",
  "tool_input": {"command": "npm test"}
}' | renotify dispatch
```

The mobile device should show a notification with "Permission:
Bash" / "npm test" and Allow/Deny buttons.

**Allow response** → stdout:

```json
{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}
```

**Deny response** → stdout:

```json
{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"Denied by remote user via Renotify"}}}
```

### 1.4 Testing Without a Mobile Device

Use `renotify answer` in a separate terminal to simulate a
mobile response. First, check the daemon logs for the flow ID
and notification ID printed when dispatch publishes, then:

```bash
# Allow:
renotify answer -f <flow_id> -n <notification_id> --accepted

# Deny:
renotify answer -f <flow_id> -n <notification_id> --rejected
```

Alternatively, list active flows to find the dispatch flow:

```bash
renotify flows
renotify flow <flow_id>
```

---

## 2. Tool Input Summarisation

The dispatch command extracts a human-readable summary from the
`tool_input` JSON. Test each tool type:

```bash
# Bash → shows command
echo '{"hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"rm -rf node_modules"}}' | renotify dispatch &

# Edit → shows file_path
echo '{"hook_event_name":"PermissionRequest","tool_name":"Edit","tool_input":{"file_path":"/home/user/main.go"}}' | renotify dispatch &

# Glob → shows pattern in path
echo '{"hook_event_name":"PermissionRequest","tool_name":"Glob","tool_input":{"pattern":"**/*.ts","path":"/src"}}' | renotify dispatch &

# Agent → shows subagent_type: description
echo '{"hook_event_name":"PermissionRequest","tool_name":"Agent","tool_input":{"subagent_type":"Explore","description":"Find API endpoints"}}' | renotify dispatch &
```

Check the mobile notification body for each to verify the
summary is correct.

---

## 3. Claude Code Hook Configuration

### 3.1 Project-Local Configuration

Add to `.claude/settings.local.json` (not committed):

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

**Notes:**

- PermissionRequest has no matcher — all tool permissions are
  forwarded. Add a matcher to restrict (e.g., `"Bash|Edit"`).
- Notification uses `"matcher": "idle_prompt"` to avoid
  duplicate notifications when PermissionRequest is also
  configured.
- Notification uses `"async": true` (fire-and-forget).
- PermissionRequest is synchronous (blocks until response).

### 3.2 Selective Tool Filtering

Forward only Bash permissions to mobile:

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

### 3.3 User-Global Configuration

For all projects, add to `~/.claude/settings.json` instead of
the project-local file.

---

## 4. End-to-End Test with Claude Code

1. **Start the daemon:**
   ```bash
   renotify daemon start --foreground
   ```

2. **Configure hooks** (Section 3.1).

3. **Start Claude Code** in the project directory. The Renotify
   MCP server and hooks are both active.

4. **Trigger a permission dialog.** Ask Claude Code to run a
   shell command that requires approval (e.g., "run npm test").

5. **Verify the mobile notification** appears with:
   - Title: "Permission: Bash"
   - Body: "npm test"
   - Buttons: Allow / Deny

6. **Tap Allow** on the mobile device.

7. **Verify Claude Code proceeds** — the command executes and
   the spinner message disappears.

8. **Test Deny:** Repeat steps 4-5, tap Deny. Verify Claude
   Code falls back to the terminal prompt with the denial
   message.

9. **Test fallback:** Stop the daemon, trigger a permission.
   Verify Claude Code falls back to the normal terminal prompt
   (non-blocking error from exit code 1).

---

## 5. Troubleshooting

**Dispatch exits 1 immediately:**
The daemon is not running or NATS is unreachable. Start the
daemon and verify with `renotify flows`.

**No mobile notification:**
Check that the mobile app is paired and connected. Verify with
the persistent notification on the device.

**Dispatch blocks indefinitely:**
The mobile user hasn't responded. The dispatch will timeout
after `timeout.default_ask_timeout` (default 5 minutes). The
safety timer (timeout + grace period) will also fire if the
daemon fails to enforce the timeout.

**"parse hook input" error:**
The stdin JSON is malformed. Verify the JSON is valid and
contains `hook_event_name`.

**Hook not triggering in Claude Code:**
Verify the hook configuration in `.claude/settings.local.json`
or `~/.claude/settings.json`. Run `/hooks` in Claude Code to
list configured hooks. Check that the matcher (if any) matches
the tool name.

**Spinner shows but no notification:**
The dispatch process started but NATS publish failed. Check
daemon logs for errors. Ensure JetStream is healthy.

**Timeout in Claude Code (hook killed):**
The Claude Code hook timeout (default 600s) may expire before
the Renotify ask timeout. Ensure the Renotify timeout
(`timeout.default_ask_timeout`) is shorter than the hook
timeout.
