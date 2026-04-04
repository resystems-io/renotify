# CLI Output Contract

This document defines the exit code mapping, output routing, and
per-command output format for all Renotify CLI commands. It is the
stable contract that shell scripts, CI pipelines, and AI agents
(invoking the CLI via shell execution) depend on.

For the payload schemas returned by CLI commands, see the
[Payload Schema Analysis](analysis-payload-schemas.md). For the
configuration flags and defaults, see the
[Configuration Schema](analysis-configuration-schema.md).

This document addresses refinement item **A-08** (CLI Contract:
Exit Codes & Output Format).

---

## 1. Exit Codes

Exit codes follow standard Unix conventions for codes 0-2 and
use Renotify-specific codes 3-6 for domain error conditions. Each
domain code maps 1:1 to an `ErrorResponse.code` value (see
[Payload Schemas](analysis-payload-schemas.md) `ErrorResponse`).

| Exit Code | Name          | `ErrorResponse.code` | Meaning                                                                                                                      |
|:----------|:--------------|:---------------------|:-----------------------------------------------------------------------------------------------------------------------------|
| 0         | Success       | —                    | Command completed normally                                                                                                   |
| 1         | General error | `internal`           | Unexpected failure (daemon unreachable, I/O error, config invalid)                                                           |
| 2         | Usage error   | —                    | Invalid flags, missing required arguments, malformed input. Cobra emits this automatically for argument validation failures. |
| 3         | Timeout       | `timeout`            | Blocking `ask` request expired without a human response (R-CLI-06)                                                           |
| 4         | Rate limited  | `rate_limited`       | Per-flow notification rate limit exceeded (R-CLI-16)                                                                         |
| 5         | Unroutable    | `unroutable`         | No mobile client connected to receive the notification                                                                       |
| 6         | Not found     | `not_found`          | Referenced flow, notification, or pairing token does not exist                                                               |

### 1.1 Cobra Integration

Cobra uses exit code 1 for command execution errors and exit code 2
for usage/argument errors by default. The Renotify CLI preserves
this convention and extends it with codes 3-6. The root command
sets `SilenceErrors: true` and `SilenceUsage: true` so that error
formatting is controlled by the Renotify error handler, not Cobra's
default output.

### 1.2 Exit Code Constants

```go
const (
	ExitSuccess     = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitTimeout     = 3
	ExitRateLimited = 4
	ExitUnroutable  = 5
	ExitNotFound    = 6
)
```

---

## 2. Output Routing

All commands follow the Unix convention: **stdout is data, stderr
is diagnostics**.

| Stream | Content                                       | Consumers                             |
|:-------|:----------------------------------------------|:--------------------------------------|
| stdout | Command result (JSON or human-readable text)  | Scripts, agents, pipes, terminal user |
| stderr | Error messages, warnings, progress indicators | Terminal user, log files              |

Rules:

* **Success output** always goes to stdout.
* **Error messages** always go to stderr. The exit code carries the
  machine-readable error signal; the stderr message provides
  human-readable context.
* **QR code output** (`renotify pair`) goes to stdout so it can be
  redirected if needed, but is primarily intended for terminal
  display.
* **Daemon logs** go to stderr in foreground mode and to
  `daemon.log` in background mode (per [Configuration
  Schema](analysis-configuration-schema.md) Section 2.7).

---

## 3. Output Format Flag

Commands that produce structured data support a `--format` flag
with two modes:

| Mode   | Description                                 | Consumers                        |
|:-------|:--------------------------------------------|:---------------------------------|
| `json` | Machine-parseable JSON, one object per line | Scripts, CI pipelines, AI agents |
| `text` | Human-readable formatted output             | Terminal users                   |

Each command has a sensible default:

| Command   | Default `--format` | Rationale                                               |
|:----------|:-------------------|:--------------------------------------------------------|
| `ask`     | `json`             | Primary consumer is scripts/agents parsing the response |
| `history` | `json`             | Primary consumer is scripts/agents processing records   |
| `post`    | `text`             | Fire-and-forget; no structured data to return           |
| `pair`    | `text`             | QR code is inherently visual                            |
| `revoke`  | `text`             | Confirmation message for the user                       |
| `daemon`  | —                  | No stdout output (logs to stderr or file)               |

The `--format` flag is accepted by all commands but only
meaningful for `ask` and `history`. For `post`, `pair`, and
`revoke`, `--format json` causes confirmation messages to be
emitted as JSON objects instead of plain text.

---

## 4. Per-Command Output

### 4.1 `renotify daemon`

Starts the daemon process. Does not produce stdout output.

**Foreground mode:**

| Stream | Content                                                   |
|:-------|:----------------------------------------------------------|
| stdout | (none)                                                    |
| stderr | Structured log lines (startup, connection events, errors) |

**Background mode:**

| Stream       | Content                    |
|:-------------|:---------------------------|
| stdout       | (none)                     |
| stderr       | (none after daemonisation) |
| `daemon.log` | Structured log lines       |

**Exit codes:**

| Condition                        | Exit Code |
|:---------------------------------|:----------|
| Normal shutdown (SIGINT/SIGTERM) | 0         |
| Configuration validation failure | 1         |
| Port already in use              | 1         |
| Missing required `username`      | 1         |

### 4.2 `renotify post`

Sends a fire-and-forget notification. Silent on success.

**Success (`--format text`, default):**

```
(no output, exit 0)
```

**Success (`--format json`):**

```json
{"status":"sent","notification_id":"ntf_0R3FABM6NQKJ71XW"}
```

**Error examples (stderr):**

```
error: rate limit exceeded (60 notifications/min for flow fl_0R3FABM7TP2XE89YWCGKN4QJ5V)
```

**Exit codes:**

| Condition                     | Exit Code |
|:------------------------------|:----------|
| Notification published        | 0         |
| Rate limited                  | 4         |
| Unroutable (no mobile client) | 5         |
| Daemon unreachable            | 1         |

### 4.3 `renotify ask`

Sends a blocking notification and waits for a human response.
This is the most important output contract — scripts and agents
parse the stdout JSON to extract the decision.

The CLI does not run its own timeout timer. The daemon is the sole
timeout enforcer (R-CLI-17): the CLI publishes the `timeout_sec`
value in the `NotificationRequest` payload, and the daemon starts
a server-side timer. On expiry the daemon publishes an
`ErrorResponse` (`code: "timeout"`) to the `.response` subject.
The CLI receives this and exits with code 3.

**Success (`--format json`, default):**

The `NotificationResponse` JSON is printed to stdout as a single
line:

```json
{"request_id":"ntf_4H7DCRW2VNPK9FMJ","accepted":true,"timestamp":"2026-03-27T14:32:15Z"}
```

Boolean rejection with explanation:

```json
{"request_id":"ntf_8X1GBEQ5STNZ3KWR","accepted":false,"text":"Wait for the security audit to close.","timestamp":"2026-03-27T14:33:42Z"}
```

Choice selection:

```json
{"request_id":"ntf_4H7DCRW2VNPK9FMJ","action":"Approve","timestamp":"2026-03-27T14:32:15Z"}
```

**Success (`--format text`):**

```
Response: Approve
```

Or for boolean with text:

```
Response: No
Comment:  Wait for the security audit to close.
```

**Timeout (stderr):**

```
error: timeout after 5m0s waiting for response to "Deploy to production?"
```

**Exit codes:**

| Condition                     | Exit Code |
|:------------------------------|:----------|
| Response received             | 0         |
| Timeout expired               | 3         |
| Rate limited                  | 4         |
| Unroutable (no mobile client) | 5         |
| Daemon unreachable            | 1         |

**Note on `accepted: false`:** A human responding "No" is a
**successful** response (exit 0). The boolean value is data, not
an error. Scripts that need to branch on the decision inspect the
`accepted` field in the JSON output, not the exit code.

### 4.4 `renotify history`

Queries the notification history ledger.

**Success (`--format json`, default):**

The `HistoryQueryResult` JSON is printed to stdout:

```json
{"records":[{"request":{"id":"ntf_4H7DCRW2VNPK9FMJ","flow_id":"fl_0R3FABM7TP2XE89YWCGKN4QJ5V","daemon_id":"dn_3G2K7V9WNFQ4J","workspace_id":"ws_5MBJR1HXNP3KQ8DW","title":"Deploy to production?","response_types":["choice"],"priority":"high","source":"cd/deploy-pipeline","actions":["Approve","Reject","Defer"],"timeout_sec":300,"timestamp":"2026-03-27T14:30:00Z"},"response":{"request_id":"ntf_4H7DCRW2VNPK9FMJ","action":"Approve","timestamp":"2026-03-27T14:32:15Z"}}],"total":1}
```

**Success (`--format text`):**

```
Showing 1 of 1 records

  ntf_4H7DCRW2VNPK9FMJ  2026-03-27T14:30:00Z  high
  Deploy to production?
  Response: Approve (2026-03-27T14:32:15Z)
```

**Exit codes:**

| Condition                                | Exit Code |
|:-----------------------------------------|:----------|
| Query returned results (including empty) | 0         |
| Daemon unreachable                       | 1         |

An empty result set (`"records":[],"total":0`) is success (exit
0), not an error.

### 4.5 `renotify pair`

Generates a pairing QR code for the mobile app.

**Success (`--format text`, default):**

```
Pairing QR code for stewart@dev-laptop (wss://192.168.1.42:4223)

█████████████████████████████████
█████████████████████████████████
████ ▄▄▄▄▄ █ ▄▄▀█▀█ ▄▄▄▄▄ ████
████ █   █ █▄  ▄▀██ █   █ ████
...
(QR code rendered via mdp/qrterminal half-block characters)

Scan this code with the Renotify app to pair.
Token: rn_tk_0A1B2C3D...5D (new)
Cert:  b94d27b9...cde9 (existing)
```

The QR code encodes the minified `ProvisioningPayload` JSON.

**Success (`--format json`):**

```json
{"host":"192.168.1.42","port":4223,"token":"rn_tk_0A1B2C3D4E5F6G7H8J9K0M1N2P3Q4R5S6T7V8W9X0Y1Z2A3B4C5D","cert_fingerprint":"b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9","cert_regenerated":false}
```

This mode omits the visual QR code and outputs the provisioning
data as JSON, useful for automation or alternative pairing
mechanisms.

**Exit codes:**

| Condition                                | Exit Code |
|:-----------------------------------------|:----------|
| QR code generated (new or existing cert) | 0         |
| IP discovery failed                      | 1         |
| Certificate generation failed            | 1         |
| Daemon state directory not writable      | 1         |

### 4.6 `renotify revoke`

Revokes the active mobile pairing token.

**Success (`--format text`, default):**

```
Pairing token revoked. Mobile client disconnected.
```

Or for shared broker mode:

```
Pairing token deleted locally.
Note: shared broker token must be revoked by the operator.
```

**Success (`--format json`):**

```json
{"status":"revoked","shared_broker":false}
```

**Exit codes:**

| Condition                 | Exit Code |
|:--------------------------|:----------|
| Token revoked             | 0         |
| No active token to revoke | 6         |
| Daemon unreachable        | 1         |

---

## 5. Script Integration Examples

### 5.1 Shell Script (Blocking Approval)

```bash
#!/bin/bash
response=$(renotify ask \
  --title "Deploy to production?" \
  --actions "Approve,Reject" \
  --response-types choice \
  --timeout 5m)

if [ $? -ne 0 ]; then
  echo "No approval received" >&2
  exit 1
fi

action=$(echo "$response" | jq -r '.action')
if [ "$action" = "Approve" ]; then
  deploy_to_production
else
  echo "Deployment rejected: $action" >&2
  exit 1
fi
```

### 5.2 Shell Script (Boolean with Explanation)

```bash
#!/bin/bash
response=$(renotify ask \
  --title "Proceed with migration?" \
  --response-types boolean,text \
  --timeout 10m)
exit_code=$?

if [ $exit_code -eq 3 ]; then
  echo "Timed out waiting for approval" >&2
  exit 1
elif [ $exit_code -ne 0 ]; then
  echo "Error sending notification" >&2
  exit 1
fi

accepted=$(echo "$response" | jq -r '.accepted')
comment=$(echo "$response" | jq -r '.text // empty')

if [ "$accepted" = "true" ]; then
  run_migration
else
  echo "Migration rejected: $comment" >&2
  exit 1
fi
```

### 5.3 CI Pipeline (Fire-and-Forget)

```bash
# Notify on build completion (non-blocking)
renotify post \
  --title "Build #${BUILD_NUMBER} complete" \
  --body "All ${TEST_COUNT} tests passed in ${DURATION}." \
  --priority normal \
  --source "ci/${JOB_NAME}"
# Ignore exit code — notification failure should not fail the build
```
