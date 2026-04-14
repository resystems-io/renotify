# Device Presence Analysis

This document analyses the design and implementation of device
presence tracking for the Renotify daemon. It covers the
motivation, design alternatives, the chosen approach, and a
step-by-step implementation plan.

This document addresses requirements **[R-CLI-23][r-cli-23]**
(Device Presence) and **[R-MOB-14][r-mob-14]** (Device Heartbeat),
and records design decision **D-73**.

---

## 1. Motivation

The daemon has no mechanism to determine whether a paired mobile
device is currently connected. `renotify pairings` reads
`devices.json` and shows device ID and pairing timestamp, but
cannot report connectivity status. The mobile device knows its own
connection state (via `ConnectionState` in the Android app), but
this information is not surfaced daemon-side.

When notifications are not being received, the developer needs to
distinguish between "device not connected" and "daemon or routing
misconfigured." Without a presence signal the developer must resort
to inspecting Android logcat or NATS monitoring to diagnose delivery
failures.

---

## 2. Design Constraints

The following constraints are derived from the requirements and the
existing architecture:

1. **Shared broker compatibility.** The presence mechanism must work
   identically across embedded and shared broker deployments
   ([R-CLI-23][r-cli-23]). Broker-internal mechanisms (e.g. the
   NATS `Connz` monitoring API or `$SYS` system events) are only
   available when the daemon controls the broker process.

2. **Separate interfaces.** File-based pairing data
   (`devices.json`) and query-based presence data (daemon state)
   must be served through separate, clearly delineated interfaces
   ([R-CLI-23][r-cli-23]). This prevents mixing static and dynamic
   data sources in a single command that is difficult to extend.

3. **Config-free mobile app.** The mobile app has no local
   configuration beyond the provisioning payload. Any parameters
   that govern the heartbeat (interval, etc.) must be communicated
   from the daemon, not baked into the app.

4. **Application-level signal.** The presence signal must be
   publishable over the standard NATS connection without requiring
   broker-internal features ([R-MOB-14][r-mob-14]).

5. **Configurable thresholds.** The heartbeat interval and
   staleness threshold must be configurable on the daemon side
   ([R-MOB-14][r-mob-14]).

---

## 3. Design

The chosen approach is an application-level device heartbeat. The
mobile app periodically publishes a lightweight heartbeat message to
the daemon on a per-device NATS subject. The daemon maintains an
in-memory last-seen map keyed by device ID and exposes it via a Core
NATS service endpoint (`svc.device-presence`).

This approach satisfies all five constraints from Section 2:

| Criterion      | Assessment                                        |
|:---------------|:--------------------------------------------------|
| Mobile changes | New periodic publisher in `NatsConnectionManager` |
| Daemon changes | New presence tracker subsystem + service endpoint |
| CLI changes    | New `renotify devices` command                    |
| Latency        | Eventually consistent (up to heartbeat interval)  |
| Data richness  | Device ID, timestamp (extensible)                 |
| Shared broker  | **Works** (application-level, broker-independent) |

**Known trade-offs:**
- Stale-state window (30–120s after disconnect). Acceptable for a
  diagnostic command — the user runs `renotify devices` to check,
  not to get sub-second accuracy.
- Battery impact: one small publish every 30s is negligible on top
  of existing NATS keepalive traffic.
- Daemon restart state loss: first accurate reading only arrives
  after one heartbeat interval. The Connz API may optionally be
  used as a startup seed for embedded broker deployments to
  eliminate this initial window, but it is not the authoritative
  presence source.

### 3.1 Design Alternatives Considered

Three other approaches were evaluated and rejected.

**NATS Server Connz API.** Query the embedded server's `Connz()`
method for per-device connection state. Provides rich data (IP,
start time, last activity, RTT) with zero mobile changes, but
requires a reference to `*server.Server` — only available when the
daemon controls the broker process. Fails the shared-broker
compatibility constraint.

**NATS System Events ($SYS).** Subscribe to
`$SYS.ACCOUNT.*.CONNECT` / `$SYS.ACCOUNT.*.DISCONNECT` for
event-driven connect/disconnect detection. Requires system account
configuration not currently in place, and state recovery on daemon
restart still requires Connz, making this strictly more complex than
the chosen approach for no additional benefit.

**JetStream Consumer Info.** Check whether each device's durable
consumer has active subscribers via `ConsumerInfo`. The push
consumer's binding state does not cleanly distinguish "device
connected and subscribed" from "consumer exists but device offline."
Provides no timestamps or connection metadata.

---

## 4. Communicating Parameters to the Mobile App

The mobile app is config-free — it has no `settings.json` and no
user-facing configuration surface. All operational parameters must
be communicated from the daemon.

### 4.1 Delivery Mechanism: Daemon Heartbeat

The daemon already publishes a periodic `DaemonHeartbeat` message
(D-09, D-54) containing structural context (`grace_period`,
workspaces, flows). The Android app already parses this payload via
`DaemonHeartbeat.fromJson()` and extracts Go duration strings via
`parseGoDuration()`.

Adding a `device_heartbeat_interval` field to the `DaemonHeartbeat`
payload follows this established pattern exactly:

**Go side** (`heartbeat/payload.go`):
```go
type DaemonHeartbeat struct {
    // ... existing fields ...
    DeviceHeartbeatInterval string `json:"device_heartbeat_interval,omitempty"`
}
```

**Android side** (`DaemonHeartbeat.kt`):
```kotlin
data class DaemonHeartbeat(
    // ... existing fields ...
    val deviceHeartbeatIntervalMs: Long,
)
```

The mobile app reads the interval from the first received daemon
heartbeat and uses it to configure its own heartbeat publisher
period. If the field is absent or zero, the app uses a compiled
default (30 seconds).

### 4.2 Why Not the Provisioning QR Code

The provisioning payload (`ProvisioningPayload`) is written once at
pairing time and persists in the mobile app's encrypted storage
across daemon upgrades. Adding operational parameters to it has
several drawbacks:

- **Stale values.** If the daemon's `stale_threshold` or heartbeat
  interval changes, all previously paired devices retain the old
  value until re-paired.
- **Payload size pressure.** The QR code is already near the
  practical scanning limit for terminal rendering at EC level L.
  Every additional byte reduces reliability.
- **Wrong semantic layer.** The provisioning payload carries
  identity and security credentials (host, port, token, cert
  fingerprint). Operational tuning parameters belong in the runtime
  configuration channel, not the one-time credential exchange.

The daemon heartbeat is the correct delivery mechanism: it is
periodic, it is parsed by every connected device, and changes take
effect within one heartbeat interval — no re-pairing required.

### 4.3 Default and Fallback Behaviour

| Scenario                          | Mobile behaviour                  |
|:----------------------------------|:----------------------------------|
| Daemon heartbeat received         | Use `device_heartbeat_interval`   |
| Field absent or zero              | Use compiled default (30 seconds) |
| Daemon heartbeat not yet received | Use compiled default (30 seconds) |
| Daemon stops sending heartbeats   | Keep last known interval          |

The mobile app starts publishing heartbeats immediately on connect
using the compiled default. When the first daemon heartbeat arrives
(typically within 30 seconds), the app adjusts its interval if it
differs.

---

## 5. NATS Subject Design

### 5.1 Device Heartbeat Subject

```
resystems.renotify.{username}.device.{device_id}.heartbeat
```

- **Transport:** Core NATS Pub/Sub (ephemeral, not JetStream)
- **Direction:** Mobile → Daemon
- **Publisher:** Mobile app (`NatsConnectionManager`)
- **Subscriber:** Daemon presence tracker (wildcard:
  `resystems.renotify.{username}.device.*.heartbeat`)

This reuses the existing `device.{device_id}.*` namespace alongside
the existing `device.{device_id}.control` subject (C-16).

### 5.2 Device Presence Service Endpoint

```
resystems.renotify.{username}.svc.device-presence
```

- **Transport:** Core NATS Request-Reply
- **Direction:** CLI → Daemon (query) → CLI (response)
- **Pattern:** Follows `svc.flows`, `svc.history`, etc.

### 5.3 Mobile ACL Update

The mobile publish ACL (`broker/auth.go`) must be extended to allow
each device to publish its own heartbeat. The permission is
device-specific (not a wildcard) so a device cannot impersonate
another:

```go
prefix + ".device." + d.DeviceID + ".heartbeat",
```

The existing wildcard `prefix + ".svc.*"` already covers the
`svc.device-presence` subject for request-reply responses.

---

## 6. Payload Schemas

### 6.1 DeviceHeartbeat (Mobile → Daemon)

```json
{
  "device_id": "mb_ABCDE12345678",
  "timestamp": "2026-04-08T09:00:00Z"
}
```

| Field       | Type   | Required | Description                   |
|:------------|:-------|:---------|:------------------------------|
| `device_id` | string | yes      | Device identity from pairing  |
| `timestamp` | string | yes      | ISO 8601 / RFC 3339 timestamp |

Minimal payload (~80 bytes). Additional fields (app version, battery
level) can be added in the future without breaking existing parsers
due to `omitempty` convention.

### 6.2 DaemonHeartbeat Extension

New optional field added to the existing `DaemonHeartbeat` payload:

| Field                       | Type   | Required | Description                |
|:----------------------------|:-------|:---------|:---------------------------|
| `device_heartbeat_interval` | string | no       | Go duration (e.g. `"30s"`) |

### 6.3 DevicePresenceResult (svc.device-presence Response)

```json
{
  "devices": [
    {
      "username": "stewart",
      "device_id": "mb_ABCDE12345678",
      "paired_at": "2026-04-01T12:00:00Z",
      "online": true,
      "last_seen": "2026-04-08T09:01:00Z"
    },
    {
      "username": "stewart",
      "device_id": "mb_FGHIJ98765432",
      "paired_at": "2026-04-02T15:30:00Z",
      "online": false,
      "last_seen": "2026-04-08T08:55:00Z"
    }
  ]
}
```

| Field       | Type    | Description                                   |
|:------------|:--------|:----------------------------------------------|
| `username`  | string  | Owning daemon username                        |
| `device_id` | string  | Device identity                               |
| `paired_at` | string  | RFC 3339 pairing timestamp                    |
| `online`    | boolean | True if last heartbeat within stale threshold |
| `last_seen` | string  | RFC 3339 timestamp of last heartbeat, or null |

---

## 7. Configuration

### 7.1 New Configuration Parameters

| Key                                  | Type     | Default | Env Var                                       | Description                                             |
|:-------------------------------------|:---------|:--------|:----------------------------------------------|:--------------------------------------------------------|
| `device_presence.stale_threshold`    | duration | `2m`    | `RENOTIFY_DEVICE_PRESENCE_STALE_THRESHOLD`    | Time after last heartbeat before device is offline      |
| `device_presence.heartbeat_interval` | duration | `30s`   | `RENOTIFY_DEVICE_PRESENCE_HEARTBEAT_INTERVAL` | Heartbeat interval communicated to mobile via heartbeat |

### 7.2 Validation Constraints

- `device_presence.stale_threshold` must be at least 10 seconds.
- `device_presence.heartbeat_interval` must be at least 5 seconds.
- `stale_threshold` should be at least 2× `heartbeat_interval` to
  avoid false offline detection from a single missed heartbeat.

---

## 8. Implementation Plan

### Step 1 — Configuration

**Files:** `internal/config/config.go`, `internal/config/registry.go`

1. Add `DevicePresenceConfig` struct with `StaleThreshold` and
   `HeartbeatInterval` Duration fields.
2. Add `DevicePresence` field to `Config` struct.
3. Add defaults in `Default()`: stale threshold 2 minutes, heartbeat
   interval 30 seconds.
4. Add validation in `Validate()`.
5. Add two `ParamInfo` entries in the config registry.

**Tests to update:** Config registry count assertion.

**Compile check:** `go vet ./internal/config/...`

### Step 2 — NATS Subjects and ACLs

**Files:** `internal/broker/subjects.go`, `internal/broker/auth.go`

1. Add `DeviceHeartbeatSubject(username, deviceID)` constructor.
2. Add `ServiceDevicePresenceSubject(username)` constructor.
3. Add device-specific heartbeat publish permission to mobile ACL
   in `BuildAuthConfig`.

**Tests to update:** `broker/subjects_test.go` (new cases),
`broker/auth_test.go` (verify heartbeat permission).

**Compile check:** `go vet ./internal/broker/...`

### Step 3 — Wire Types

**File:** `internal/statesvc/statesvc.go`

1. Add `DevicePresenceQuery` struct (empty, for future filters).
2. Add `DeviceStatus` struct (username, device_id, paired_at,
   online, last_seen).
3. Add `DevicePresenceResult` struct (devices slice).

**Compile check:** `go vet ./internal/statesvc/...`

### Step 4 — Daemon Heartbeat Extension

**Files:** `internal/heartbeat/payload.go`,
`internal/heartbeat/publisher.go`

1. Add `DeviceHeartbeatInterval` string field to
   `DaemonHeartbeat` struct.
2. Pass the configured interval to the publisher via constructor.
3. Populate the field in `publish()`.

**Tests to update:** Heartbeat payload serialisation tests.

**Compile check:** `go vet ./internal/heartbeat/...`

### Step 5 — Presence Tracker Subsystem

**New package:** `internal/presence/`

**Files:** `presence.go`, `payload.go`, `presence_test.go`

1. `Tracker` struct implementing `daemon.Subsystem`.
2. `Start()` subscribes to
   `resystems.renotify.{username}.device.*.heartbeat` (wildcard).
3. `handleHeartbeat()` extracts device_id from subject, updates
   in-memory last-seen map.
4. `DevicePresence()` returns `[]statesvc.DeviceStatus` computing
   online/offline from stale threshold.
5. `ReloadDevices()` for SIGHUP hot-reload after pair/revoke.

**New tests:**
- Online after heartbeat received.
- Offline after stale threshold expires.
- Never-seen device (paired but no heartbeat yet).
- Multiple devices with mixed states.
- Reload adds/removes devices correctly.
- Device ID extraction from NATS subject.

**Compile check:** `go vet ./internal/presence/...`

### Step 6 — Registry Integration

**Files:** `internal/registry/registry.go`,
`internal/registry/endpoint.go` (or new
`presence_endpoint.go`)

1. Add `*presence.Tracker` field to registry `Service` struct.
2. Update `New()` signature to accept tracker.
3. Add `subscribeDevicePresenceEndpoint` to the endpoints slice.
4. Implement handler: calls `tracker.DevicePresence()`, marshals
   response.

**Tests to update:** `registry_test.go` and
`mcpserver/tool_test.go` — update `startRegistry` /
`startTestRegistry` helpers to pass a tracker.

**New test:** `TestDevicePresenceEndpoint` — publish heartbeat,
query endpoint, verify response.

**Compile check:** `go vet ./internal/registry/...`

### Step 7 — Daemon Wiring

**File:** `internal/command/daemon.go`

1. Load paired devices from `devices.json`.
2. Create `presence.Tracker` with config and device list.
3. Register as subsystem (after heartbeat, before registry).
4. Pass tracker to registry constructor.
5. Update SIGHUP handler to call `tracker.ReloadDevices()`.

**Startup order becomes:**
Ledger → HTTP → MCP → Heartbeat → **Presence** → Registry

**Compile check:** `go vet ./internal/command/...`

### Step 8 — CLI `renotify devices` Command

**Files:** `internal/command/devices.go`, `internal/command/root.go`

1. New `newDevicesCmd(app)` command.
2. Connects to daemon via `broker.ConnectCLI(cfg)`.
3. Queries `svc.device-presence` via NATS Request-Reply.
4. Text output via tabwriter (username, device ID, status, last
   seen, paired at).
5. JSON output: raw `DevicePresenceResult`.
6. Register in `root.go`.

**Tests to update:** `command_test.go` — add `"devices"` to help
output and subcommand flags checks.

**Compile check:** `go vet ./internal/command/...`

### Step 9 — Android Heartbeat Publisher

**File:**
`clients/android/.../nats/NatsConnectionManager.kt`

1. After successful subscription setup, launch a coroutine that
   publishes `DeviceHeartbeat` JSON on the device-specific
   heartbeat subject.
2. Use the compiled default interval (30 seconds) initially.
3. When a daemon heartbeat arrives containing
   `device_heartbeat_interval`, adjust the publish interval.
4. Loop checks `nc.status == CONNECTED` — auto-stops on disconnect.

**File:**
`clients/android/.../dashboard/DaemonHeartbeat.kt`

1. Add `deviceHeartbeatIntervalMs` field.
2. Parse `device_heartbeat_interval` via existing
   `parseGoDuration()`.

**Tests:**
- Heartbeat subject format test.
- Heartbeat JSON payload contains required fields.
- `DaemonHeartbeat.fromJson()` parses new field.
- Missing field defaults to zero.

### Step 10 — Integration Tests

**File:** `internal/command/smoke_test.go` or dedicated
`internal/presence/integration_test.go`

1. Start embedded NATS + presence tracker with one test device.
2. Publish heartbeat on device subject.
3. Query `svc.device-presence`.
4. Verify device is online.
5. Wait past stale threshold.
6. Query again.
7. Verify device is offline with correct last_seen.

### Step 11 — Documentation

1. **renotify-refinements.md** — change log entry for
   implementation.
2. **analysis-nats-transport-design.md** — add
   `device.{device_id}.heartbeat` to subject catalogue, add
   `svc.device-presence` to service endpoint table, update mobile
   ACL table.
3. **analysis-configuration-schema.md** — add
   `device_presence.stale_threshold` and
   `device_presence.heartbeat_interval` parameters.
4. **analysis-payload-schemas.md** — add `DeviceHeartbeat` schema,
   update `DaemonHeartbeat` with new field.

---

## 9. Dependency Graph

```
Step 1 (config)
  ↓
Step 2 (subjects + ACLs)
  ↓
Step 3 (wire types)
  ↓
Step 4 (daemon heartbeat extension)
  ↓
Step 5 (presence tracker) ← depends on 1, 2, 3
  ↓
Step 6 (registry endpoint) ← depends on 5
  ↓
Step 7 (daemon wiring) ← depends on 5, 6
  ↓
Step 8 (CLI command) ← depends on 2, 3
  |
Step 9 (Android) ← depends on 2, 4
  ↓
Step 10 (integration tests) ← depends on all above
  ↓
Step 11 (documentation) ← last
```

Steps 1–4 are structural changes (types, subjects, config). Steps
5–7 form the core daemon implementation. Step 8 is the CLI. Step 9
is Android. Steps 10–11 are verification and documentation.

---

## 10. Impact on Existing Requirements

| Requirement | Impact                                              |
|:------------|:----------------------------------------------------|
| R-CLI-14    | Minor — registry gains one more service endpoint    |
| R-CLI-20    | Aligned — new endpoint follows NATS service pattern |
| R-CLI-21    | None — presence tracker is not in the MCP server    |
| R-MOB-10    | Enhanced — mobile app gains a daemon-visible signal |
| R-MOB-11    | None — per-device identity already established      |
| R-SEC-02    | Minor — mobile ACL extended with heartbeat subject  |
| R-SYS-01    | Negligible — one heartbeat per device per interval  |
| D-09        | Extended — daemon heartbeat gains new field         |
| D-16        | Updated — mobile publish ACL adds heartbeat subject |

---

[r-cli-23]: renotify-refinements.md#r-cli-23-device-presence
[r-mob-14]: renotify-refinements.md#r-mob-14-device-heartbeat
