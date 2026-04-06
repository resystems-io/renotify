# Renotify

Human-in-the-loop notification system for AI agents and
developer workflows. Send notifications to your phone, get
decisions back.

## What It Does

- **Notify** — Fire-and-forget alerts to your Android device
  when builds complete, tests fail, or agents need attention.
- **Ask** — Interactive prompts (approve/deny, choose, text
  input) that block until the developer responds on their
  phone.
- **Interjections** — Stop or send notes to running agents
  from the mobile dashboard without returning to the
  terminal.
- **History** — Query past notifications and responses from
  the CLI or mobile app.

## Quick Start

```bash
# Build everything (Android APK + CLI with embedded APK)
make

# Start the daemon
renotify daemon start

# Pair your phone (scan the QR code with the app)
renotify pair

# Install the app on your phone
renotify app apk serve
# → Scan the QR code to download the APK

# Send a test notification
renotify post -t "Hello from Renotify"

# Ask a question and wait for the answer
renotify ask -t "Deploy to production?" -r boolean
```

## AI Agent Integration

### Claude Code (HTTP MCP)

Add to `~/.claude/settings.json`:

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

### Antigravity, Cursor, Windsurf (stdio)

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

### Claude Code Hooks

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PermissionRequest": [
      {"command": "renotify dispatch"}
    ],
    "Notification": [
      {"command": "renotify dispatch"}
    ]
  }
}
```

## CLI Commands

| Command          | Description                                |
|:-----------------|:-------------------------------------------|
| `answer`         | Publish a response to a waiting ask        |
| `app apk extract`| Extract the embedded APK to disk           |
| `app apk serve`  | Serve the APK over HTTP with QR code       |
| `ask`            | Send an interactive notification and wait  |
| `config init`    | Generate a default settings.json           |
| `config list`    | Show all configuration parameters          |
| `daemon start`   | Start the Renotify daemon                  |
| `dispatch`       | Claude Code hook handler (stdin/stdout)    |
| `flow`           | Show details of a single active flow       |
| `flows`          | List active flows                          |
| `history`        | Query notification history                 |
| `interject`      | Send a control signal to a running flow    |
| `mcp`            | Run a stdio MCP gateway to the daemon      |
| `pair`           | Pair a mobile device via QR code           |
| `pairings`       | List paired devices                        |
| `post`           | Send a fire-and-forget notification        |
| `revoke`         | Revoke a device pairing                    |
| `silent`         | Toggle silent mode on a device             |
| `version`        | Print the build version                    |

## Architecture

See [docs/renotify-architecture.md](docs/renotify-architecture.md)
for system context, design principles, block diagrams, and
sequence diagrams.

## Guides

- [MCP Testing Playbook](cli/docs/mcp-testing.md) —
  Protocol-level testing with curl and agent integration.
- [Hook Testing Playbook](cli/docs/hooks-testing.md) —
  Claude Code hook dispatcher testing.
- [Device Testing Guide](clients/android/docs/device-testing.md) —
  Android device setup, pairing, and troubleshooting.

## Project Structure

```
renotify/
├── cli/                    Go CLI (go.resystems.io/renotify)
│   ├── cmd/renotify/       Entry point
│   ├── internal/           Packages: command, daemon, broker,
│   │                       mcpserver, ledger, registry, ...
│   └── docs/               CLI testing playbooks
├── clients/android/        Kotlin Android app
│   ├── app/src/main/       MainActivity, NatsService,
│   │                       NotificationRenderer, Dashboard
│   └── docs/               Device testing guide
├── docs/                   Architecture and analysis documents
├── lib/make/               Shared Makefile includes
└── Makefile                Root build orchestrator
```
