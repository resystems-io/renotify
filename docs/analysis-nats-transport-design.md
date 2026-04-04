# NATS Transport and Subject Design

This document defines the NATS subject catalogue, JetStream configuration,
transport security, authentication model, and connection lifecycles for
Renotify. It is the authoritative reference for how the NATS infrastructure is
configured, secured, and operated to carry the payloads defined in the [Payload
Schema Analysis](analysis-payload-schemas.md) across the system element
hierarchy established in the [Naming & Addressing
Analysis](analysis-naming-and-addressing.md).

This document addresses refinement item **A-02** (Broker Provisioning & Routing
Design). The pairing implementation belongs to Phase 3 (C-07, M-06); this
analysis captures the design decisions that inform it.

---

## 1. Subject Catalogue

All subjects use the global prefix `resystems.renotify` (R-API-05) followed by
the username for per-user scoping (R-API-06). Flow-scoped subjects use the
globally unique `flow_id` as the routing key (R-API-07), eliminating namespace
collisions without encoding workspace or daemon identity in the subject.

### 1.1 Subject Registry

| Subject Pattern | Transport | Publisher | Subscriber(s) | Payload | Persistence | Req Trace |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `resystems.renotify.{username}.flow.{flow_id}.request` | JetStream | CLI / MCP Agent | Mobile app | `NotificationRequest` | Memory, 30-min TTL | R-API-01, R-API-07 |
| `resystems.renotify.{username}.flow.{flow_id}.response` | JetStream | Mobile app | CLI / MCP Agent | `NotificationResponse` | Memory, 30-min TTL | R-API-02, R-API-07 |
| `resystems.renotify.{username}.flow.{flow_id}.lifecycle` | JetStream | CLI / MCP Agent | Daemon, Mobile app | `FlowLifecycleEvent` | Memory, 30-min TTL | R-API-10 |
| `resystems.renotify.{username}.flow.{flow_id}.interject` | JetStream | Mobile app | Daemon | `InterjectionCommand` | Memory, 30-min TTL | R-API-09 |
| `resystems.renotify.{username}.daemon.{daemon_id}.heartbeat` | Core NATS Pub/Sub | Daemon | Mobile app | `DaemonHeartbeat` | None (ephemeral) | R-CLI-14 |
| `resystems.renotify.{username}.svc.flows` | Core NATS Request-Reply | Mobile app | Daemon | `ActiveFlowsQuery` / `ActiveFlowsResult` | None (synchronous) | R-CLI-14, R-MOB-09 |
| `resystems.renotify.{username}.svc.history` | Core NATS Request-Reply | Mobile app | Daemon | `HistoryQueryRequest` / `HistoryQueryResult` | None (synchronous) | R-CLI-13, R-MOB-07 |

**Design rule:** All subjects under `resystems.renotify.{username}.flow.*` use
JetStream for durable delivery within the TTL window. All other subjects use
Core NATS (best-effort pub/sub or synchronous request-reply).

### 1.2 Subscription Patterns

| Subscriber | Subject Pattern | Transport | Purpose |
| :--- | :--- | :--- | :--- |
| Mobile app | `resystems.renotify.{username}.>` | JetStream + Core NATS | Receives all user traffic: notifications, responses, lifecycle events, heartbeats |
| Daemon (flow registry) | `resystems.renotify.{username}.flow.*.lifecycle` | JetStream consumer | Maintains the active flow registry in SQLite |
| Daemon (interjections) | `resystems.renotify.{username}.flow.*.interject` | JetStream consumer | Routes interjection commands to the correct flow handler |
| Daemon (services) | `resystems.renotify.{username}.svc.>` | Core NATS | Serves active-flows and history query endpoints |
| CLI (`ask`, blocking) | `resystems.renotify.{username}.flow.{flow_id}.response` | JetStream consumer | Waits for the human's decision on one specific flow |

---

## 2. JetStream Configuration

### 2.1 Stream Definition

The embedded broker creates a single JetStream stream to hold all flow-scoped
messages. The stream is memory-backed (R-CLI-12) and bounded by age and size.

| Parameter | Value | Rationale |
| :--- | :--- | :--- |
| Name | `RENOTIFY` | Single stream; shared broker operators may create per-user streams at their discretion |
| Subjects | `resystems.renotify.*.flow.>` | Captures all flow-scoped traffic for all users on this broker |
| Storage | `Memory` | R-CLI-12: no filesystem persistence for JetStream |
| Retention | `Limits` | Messages retained until age/size limits are hit, regardless of consumer interest |
| Max Age | 30 minutes (configurable) | R-CLI-12 default TTL; balances resilience against brief mobile disconnections with bounded memory |
| Max Bytes | 128 MB (configurable) | Bounds total stream memory consumption |
| Max Message Size | 64 KB | R-SYS-01 payload size limit |
| Max Messages Per Subject | 1,000 | Prevents a single runaway flow from consuming all stream capacity |
| Discard Policy | `Old` | When limits are reached, the oldest messages are discarded first |
| Duplicate Window | 2 minutes | Prevents accidental re-publish from CLI retry logic; `Nats-Msg-Id` header used for deduplication |
| Num Replicas | 1 | Embedded broker is single-node; shared brokers configure replication independently |

```go
// JetStreamConfig holds the configurable parameters for the RENOTIFY stream.
// Non-configurable parameters (storage: memory, retention: limits, discard: old,
// replicas: 1) are hardcoded at stream creation.
type JetStreamConfig struct {
	MaxAge         time.Duration `json:"max_age"`           // default: 30m
	MaxBytes       int64         `json:"max_bytes"`         // default: 134217728 (128 MB)
	MaxMsgSize     int32         `json:"max_msg_size"`      // default: 65536 (64 KB)
	MaxMsgsPerSubj int64         `json:"max_msgs_per_subj"` // default: 1000
	DupWindow      time.Duration `json:"dup_window"`        // default: 2m
}
```

### 2.2 Consumer Definitions

Four consumers operate against the `RENOTIFY` stream. Consumer names include the
username or flow ID to scope their state.

| Consumer Name | Type | Filter Subject | Ack Policy | Max Deliver | Max Ack Pending | Deliver Policy | Inactive Threshold | Purpose |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `mobile-{username}` | Durable | `resystems.renotify.{username}.flow.>` | Explicit | 3 | 256 | All | None | Mobile app: receives all flow events; persists until device revoked |
| `daemon-lifecycle-{username}` | Durable | `resystems.renotify.{username}.flow.*.lifecycle` | Explicit | 3 | 64 | All | 5 minutes | Daemon: maintains the active flow registry |
| `daemon-interject-{username}` | Durable | `resystems.renotify.{username}.flow.*.interject` | Explicit | 3 | 64 | All | 5 minutes | Daemon: routes interjections to flow handlers |
| `cli-response-{flow_id}` | Ephemeral | `resystems.renotify.{username}.flow.{flow_id}.response` | Explicit | 1 | 1 | New | N/A | CLI `ask`: blocks for exactly one response |

**Mobile consumer:** Durable push consumer with a deliver subject
(`resystems.renotify.{username}.mobile.deliver`). The deliver subject is within
the user's namespace so it falls under the mobile account's existing subscribe
ACL. Push delivery is required because the jnats client library's
`PushSubscribeOptions.bind()` expects a deliver subject to subscribe to; without
one the consumer would be pull-only and require a different subscription API.
No inactive threshold is set — the consumer persists until the device is
explicitly revoked via `renotify revoke`. An inactive threshold would cause the
consumer to be auto-deleted during prolonged disconnections (e.g. overnight),
preventing the app from rebinding on reconnect without a daemon restart.
AckExplicit
ensures the mobile app positively acknowledges each message after rendering; if
the app crashes mid-render, the message is redelivered on reconnect (up to
MaxDeliver=3). MaxAckPending=256 allows the app to process a backlog of
buffered messages after reconnection without back-pressure stalling.

**Daemon consumers:** Durable for the same ack-position persistence reason. The
5-minute inactive threshold matches the stale-flow reaping grace period
(R-CLI-18). MaxAckPending=64 is sufficient for the expected lifecycle and
interjection event rate.

**CLI response consumer:** Ephemeral because its lifetime is the CLI process
lifetime. MaxDeliver=1 because a response is consumed exactly once.
DeliverPolicy=New because the CLI only cares about responses published after it
starts waiting.

---

## 3. Delivery Guarantees

### 3.1 Per-Transport Guarantees

| Transport | Delivery Guarantee | Loss Window | Mitigation |
| :--- | :--- | :--- | :--- |
| JetStream (flow events) | At-least-once within TTL | Message expires after 30 min with no connected consumer; or MaxDeliver exhausted | Mobile app: reconnection retrieves unacked messages. CLI: ephemeral consumer lives only during blocking period. |
| Core NATS Pub/Sub (heartbeat) | At-most-once | Any moment the subscriber is disconnected | Next heartbeat (30s max) or on-change trigger supersedes. Heartbeats are idempotent snapshots. |
| Core NATS Request-Reply (services) | At-most-once, synchronous | Request times out if daemon is unreachable | Mobile app retries the query. Request-reply is inherently idempotent for read-only operations. |

### 3.2 Idempotency Analysis

| Payload | Idempotent? | Key Field | Notes |
| :--- | :--- | :--- | :--- |
| `NotificationRequest` | Yes | `id` | Mobile app deduplicates on `id` if the same message is delivered more than once (at-least-once JetStream). |
| `NotificationResponse` | Yes | `request_id` | CLI consumes exactly one response (MaxDeliver=1). Duplicate delivery to the daemon history ledger is a no-op if `request_id` is already recorded. |
| `FlowLifecycleEvent` | Yes | `flow_id` + `status` | Re-delivering a "status: active" event for an already-active flow is a no-op in the registry. |
| `InterjectionCommand` | **No** | `flow_id` | A duplicate "stop" command could trigger redundant termination logic. Mitigations: the 2-minute JetStream dedup window prevents accidental republish; the daemon tracks the most recent interjection timestamp per flow and ignores duplicates within a configurable debounce window (default: 5 seconds). |
| `DaemonHeartbeat` | Yes | (timestamp) | Latest heartbeat always supersedes all prior. |
| `ActiveFlowsQuery`/`Result` | Yes | (stateless) | Pure read operation. |
| `HistoryQueryRequest`/`Result` | Yes | (stateless) | Pure read operation. |

### 3.3 Timeout Semantics

The daemon is the sole timeout enforcer for blocking `ask`
requests (R-CLI-17). It reads the `timeout_sec` value from the
`NotificationRequest` payload (set by the CLI from its `--timeout`
flag or the configured default) and starts a server-side timer
from the moment the request is received. Mobile disconnection
and reconnection do not reset or extend the timeout.

If the timeout expires before a response arrives, the daemon:

1. Publishes an `ErrorResponse` with `code: "timeout"` to the
   `.response` subject
   (`resystems.renotify.{username}.flow.{flow_id}.response`).
   The CLI is already waiting on this subject and receives the
   error directly.
2. Publishes a `FlowLifecycleEvent` with `status: failed` to
   the `.lifecycle` subject.
3. For MCP flows: updates the `DecisionResource` with
   `decided: true` and absent response fields, then emits
   `notifications/resources/updated`.

The CLI does not run its own timeout timer. It waits on
`.response` and `.interject` indefinitely; the daemon's
`ErrorResponse` publication on `.response` is the signal that
unblocks the CLI and causes it to exit with code 3 (timeout).

The mobile app may subsequently display the timed-out request
as expired when it reconnects and processes the lifecycle event.

---

## 4. Listener Configuration

### 4.1 Embedded Broker Listeners

The embedded NATS server exposes two listeners to serve the two transport
requirements (R-API-04): native TCP for co-located clients and WebSocket for
remote clients.

| Listener | Protocol | Bind Address | Default Port | TLS | Clients |
| :--- | :--- | :--- | :--- | :--- | :--- |
| Native TCP | `nats://` | `127.0.0.1` | 4222 | No (loopback only) | CLI commands, MCP server bridge |
| WebSocket | `wss://` | `0.0.0.0` | 4223 | Yes (mandatory) | Mobile app over network |

The native TCP listener binds to the loopback interface because co-located CLI
processes do not cross a network boundary and do not need TLS overhead. The
WebSocket listener binds to all interfaces because the mobile app connects over
the LAN or WAN. Port 4223 is adjacent to the standard NATS port 4222 for
discoverability. Plaintext WebSocket (`ws://`) is never permitted.

```go
// EmbeddedBrokerConfig holds the listener configuration for the
// in-process NATS server. This is a subset of the unified Config
// struct; see Configuration Schema (analysis-configuration-schema.md)
// Section 2.2 for the full definition with defaults and validation.
type EmbeddedBrokerConfig struct {
	TCPHost  string `json:"tcp_host"`  // default: "127.0.0.1"
	TCPPort  int    `json:"tcp_port"`  // default: 4222
	WSSHost  string `json:"wss_host"`  // default: "0.0.0.0"
	WSSPort  int    `json:"wss_port"`  // default: 4223
	CertFile string `json:"cert_file"` // path to TLS certificate (PEM)
	KeyFile  string `json:"key_file"`  // path to TLS private key (PEM)
}
```

### 4.2 Shared Broker Model

When the daemon connects to an external shared broker, it does not configure
listeners. It is a NATS client. The shared broker's listener configuration is
the operator's responsibility, but the following expectations must be met for
Renotify to function:

* The broker must expose a WSS listener with a valid TLS certificate (either
  CA-signed or self-signed with fingerprint provisioned during pairing).
* The broker must expose a TCP listener (or the same WSS listener) reachable by
  daemon processes.
* The broker must enforce per-user NATS auth scoped to
  `resystems.renotify.{username}.>` (see Section 7).
* The broker must have JetStream enabled with a stream configuration compatible
  with the parameters in Section 2 (or the daemon must have permission to create
  the stream).

### 4.3 `ProvisioningPayload` Port

The `p` field in the `ProvisioningPayload` carries the WSS port (4223 for the
embedded broker, or the shared broker's WSS port). The mobile app always
connects via WebSocket. The TCP port is not provisioned because it is only used
by co-located CLI processes that read it from the daemon's local configuration.

---

## 5. TLS Certificate Management

### 5.1 Certificate Parameters

The daemon generates a self-signed TLS certificate on first pairing. The
certificate secures the WSS listener for mobile app connections.

| Parameter | Value | Rationale |
| :--- | :--- | :--- |
| Key Algorithm | ECDSA P-256 (secp256r1) | Modern, fast, compact key and cert. Go stdlib `crypto/ecdsa` + `crypto/elliptic` provide native support with no external dependencies. |
| Signature Algorithm | ECDSA with SHA-256 | Matches the key type. |
| Validity Period | 3 years (1,095 days) | Long enough to avoid operational churn for a personal dev tool; short enough to encourage periodic re-pairing. |
| Subject CN | `renotify-{daemon_id}` | Ties the certificate to a specific daemon instance for log traceability. |
| Subject Alternative Names | All discovered non-loopback IPs + `127.0.0.1` + `localhost` | Required for hostname/IP verification during TLS handshake. The mobile app connects by IP address. |
| Serial Number | Random 128-bit integer | Standard practice for self-signed certificates. |

### 5.2 Storage

All TLS artifacts are stored under the XDG state directory (R-CLI-09):

| Artifact | Path | Permissions |
| :--- | :--- | :--- |
| TLS certificate (PEM) | `$XDG_STATE_HOME/renotify/tls/cert.pem` | 0644 |
| TLS private key (PEM) | `$XDG_STATE_HOME/renotify/tls/key.pem` | 0600 |

`$XDG_STATE_HOME` defaults to `~/.local/state` per the XDG Base Directory
specification.

### 5.3 Fingerprint Computation

The certificate fingerprint transmitted in the `ProvisioningPayload` (R-API-08)
is computed as:

1. Extract the DER-encoded certificate bytes (not the PEM wrapper).
2. Compute the SHA-256 hash.
3. Hex-encode the hash (lowercase), yielding a 64-character string.

This fingerprint is the `c` field in the `ProvisioningPayload`. The private key
is never transmitted.

### 5.4 Certificate Lifecycle

| Event | Behaviour |
| :--- | :--- |
| First `renotify pair` | Generate new ECDSA P-256 key pair and self-signed certificate. Store in XDG state. |
| Subsequent `renotify pair` | Reuse existing certificate (fingerprint unchanged). Generate new auth token only. |
| `renotify pair --regenerate-cert` | Generate new key pair and certificate. Invalidates any prior mobile pairing (fingerprint changes). |
| Daemon startup | Load certificate and key from XDG state. If missing, the WSS listener cannot start; the daemon logs a warning and operates in TCP-only mode (no mobile connectivity). |

### 5.5 Android TLS Trust Bootstrap

A self-signed certificate is not trusted by Android's default TLS validation
stack, which requires certificates to chain to a system-installed CA. This
section analyses the available approaches for establishing trust on the Android
mobile client.

**Approach A: Custom X509TrustManager with fingerprint pinning (Recommended)**

The Android app implements a custom `X509TrustManager` whose
`checkServerTrusted` method computes the SHA-256 fingerprint of the server's
presented leaf certificate and compares it against the fingerprint provisioned
via the QR code. If the fingerprints match, the connection is accepted;
otherwise it is rejected with a `CertificateException`.

This approach is a Trust-on-First-Use (TOFU) model. The QR code serves as the
verified out-of-band channel — the physical act of scanning the code on the
developer's own terminal establishes trust. This is analogous to SSH's TOFU
model but strictly stronger: the QR code physically transfers the fingerprint
with zero human comparison error.

The custom TrustManager is *more restrictive* than the Android platform default.
The platform trusts approximately 150 CA certificates; this TrustManager trusts
exactly one leaf certificate. It is used to initialise an `SSLContext` which is
passed to the NATS client library (both jnats and nats.kt/Ktor accept a custom
`SSLContext`).

Android lint flags all custom `X509TrustManager` implementations with a
`CustomX509TrustManager` warning. This is a warning, not an error, and is
suppressible with `@SuppressLint("CustomX509TrustManager")`. The lint check
exists to catch implementations that accept all certificates (empty
`checkServerTrusted`); a fingerprint-checking implementation that actively
rejects non-matching certificates does not exhibit the unsafe pattern. Since the
app is sideloaded (embedded in the CLI binary per R-PKG-02) rather than
distributed via the Play Store, the Play Store publishing gate does not apply.

The certificate's Subject Alternative Names (SANs) are required for hostname
verification to succeed alongside fingerprint pinning. The daemon must include
all discovered local IPs and `localhost` as SANs (see Section 5.1).

During the TLS handshake the NATS server presents its full certificate to the
client. The app does not need the certificate beforehand — it receives it
through the standard TLS protocol and validates it by computing the fingerprint
on the fly. This is the same mechanism SSH uses: the server sends its host key
and the client checks the fingerprint.

**Approach B: Ephemeral HTTP bootstrap endpoint — Rejected**

In this approach the QR code would contain only a host, HTTP port, and one-time
nonce. The app would fetch credentials via plaintext HTTP. This is rejected
because:

* Plaintext HTTP is vulnerable to active MITM on the local network. An attacker
  can intercept the nonce request and substitute their own certificate
  fingerprint and auth token. The nonce prevents replay but does not
  authenticate the server.
* Android 9+ blocks cleartext HTTP by default. Enabling it
  (`android:usesCleartextTraffic="true"`) permanently weakens the app's network
  security posture for a one-time pairing operation.
* The approach is strictly more complex: the daemon must implement a temporary
  HTTP server with nonce management, and the app still needs the same
  TrustManager for the subsequent WSS connection.
* The QR payload savings are negligible (~60 bytes) given that the current
  provisioning JSON is well under 200 bytes and QR codes handle thousands.

**Approach C: Android Network Security Configuration — Not viable**

The `network_security_config.xml` mechanism requires certificate files as
build-time resources (`res/raw/*.pem`). It cannot reference runtime-provisioned
certificates. The `<certificates>` element accepts only `"system"`, `"user"`, or
a raw resource ID. Since the daemon's certificate is generated per-installation
by `renotify pair`, it does not exist at APK build time. Not viable for
Renotify's deployment model.

**Approach D: Disable TLS validation — Not viable**

Creating a TrustManager with an empty `checkServerTrusted` method would accept
any certificate, defeating server authentication entirely. An active MITM
attacker on the same network could intercept all traffic including auth tokens
and notification content. Android lint correctly flags this pattern, and it
represents a genuine security defect. Not viable under any circumstances.

**Post-MVP consideration:** If future credential requirements exceed QR capacity
(e.g., mutual TLS client certificates), a Secure Bootstrap Channel over the
already-pinned WSS connection can exchange additional credentials without the
cleartext HTTP vulnerability of Approach B.

---

## 6. Auth Token Design

The auth token is a shared secret that authenticates the **Android mobile
client** to the **NATS broker**. It is generated by the daemon during the
`renotify pair` ceremony (R-CLI-11), transmitted to the mobile app via the QR
code's `t` field (R-API-08), and presented by the app on every NATS `CONNECT`
handshake. The broker validates the token to grant the mobile client its scoped
publish/subscribe permissions (Section 7). On the embedded broker the daemon
manages the token's full lifecycle; on a shared broker the operator provisions
it into the broker's auth configuration.

Co-located CLI processes do not use this token — they authenticate with a
separate internal token that is never exposed outside the loopback interface
(Section 6.4).

### 6.1 Token Format

| Component | Value |
| :--- | :--- |
| Prefix | `rn_tk_` (6 characters) |
| Random body | 32 bytes from `crypto/rand`, encoded as 52 Crockford Base32 characters |
| Total length | 58 characters |
| Entropy | 256 bits |
| Example | `rn_tk_0A1B2C3D4E5F6G7H8J9K0M1N2P3Q4R5S6T7V8W9X0Y1Z2A3B4C5D` |

The `rn_tk_` prefix makes tokens grep-friendly in logs, consistent with the
project's identifier prefix convention (`dn_`, `ws_`, `fl_`, `mb_`). Crockford
Base32 encoding is case-insensitive and safe in NATS auth fields, CLI arguments,
QR codes, and log output.

### 6.2 Generation

Generated by `renotify pair` using Go `crypto/rand.Read(32)`, encoded to
Crockford Base32, and prepended with `rn_tk_`. The 256-bit entropy exceeds any
practical brute-force threshold.

### 6.3 Storage

| Artifact | Path | Permissions |
| :--- | :--- | :--- |
| Active pairing token | `$XDG_STATE_HOME/renotify/pairing/token` | 0600 |
| Associated username | `$XDG_STATE_HOME/renotify/pairing/username` | 0600 |

### 6.4 NATS Auth Integration (Embedded Broker)

The embedded NATS server is configured with a two-account model.
Both accounts use NATS username/password authentication (not token
auth):

* **Daemon account:** NATS username `daemon`, password = internal
  token. Used by the daemon's own NATS connection and co-located
  CLI processes connecting via the loopback TCP listener. Full
  publish/subscribe permissions scoped to
  `resystems.renotify.{username}.>` (see Section 7).
* **Mobile account:** NATS username `mobile`, password = pairing
  token (`rn_tk_...`). Used by the Android app connecting via
  WSS. Scoped publish/subscribe permissions within
  `resystems.renotify.{username}.>` (see Section 7).

The `{username}` in the ACL subject patterns is the daemon
operator's identity (e.g., `stewart` from `config.Username`), not
the NATS authentication username. On the embedded broker, the ACL
is baked at server startup from the daemon's configured username.
On a shared broker, the operator is responsible for creating
per-developer accounts with equivalent per-namespace ACL scoping
(see Section 6.5).

The embedded NATS server API supports runtime auth reconfiguration.
When `renotify pair` or `renotify revoke` is executed while the
daemon is running, the auth configuration is hot-reloaded without
restarting the broker.

**Internal token persistence.** The internal token is generated
once on first daemon startup using the same algorithm as the
pairing token (`rn_tk_` prefix + 52 Crockford Base32 characters,
256-bit entropy) and persisted to:

| Artifact | Path | Permissions |
| :--- | :--- | :--- |
| Internal token | `$XDG_STATE_HOME/renotify/internal_token` | 0600 |

The token is reused across daemon restarts. Co-located CLI
processes (`renotify post`, `renotify ask`, `renotify history`)
read it from this file before connecting to the loopback TCP
listener. If the file does not exist, the CLI reports an error
("daemon has not been started") and exits with code 1.

### 6.5 NATS Auth Integration (Shared Broker)

The pairing token is generated by the daemon and included in the QR
`ProvisioningPayload`, but the shared broker's administrator must provision it
into the broker's auth configuration (e.g., `nats-server.conf` or an external
auth service). The daemon does not manage shared broker auth — it only generates
the token value.

Shared broker operators must map the token to the correct username-scoped
permissions as defined in Section 7.

### 6.6 Token Lifecycle

| Event | Action |
| :--- | :--- |
| `renotify pair` (no existing token) | Generate new token, store in XDG state, configure embedded NATS auth, include in QR payload |
| `renotify pair` (existing token) | Revoke old token (R-SEC-02: re-pairing supersedes prior token), generate new token |
| `renotify revoke` | Remove token from NATS auth configuration, send client disconnect signal, delete stored token file |
| Daemon startup | Load stored token from XDG state, configure embedded NATS auth. If no token exists, the broker starts without a mobile account (no mobile connectivity until `renotify pair`). |

### 6.7 Why Token Authentication Over NKeys

NATS also supports NKey authentication, which uses Ed25519 public/private key
pairs so that the private key never leaves the client. NKeys are the stronger
mechanism in general, but token authentication is the better fit for Renotify's
pairing model:

* **One-way provisioning.** The QR code is a one-way channel from daemon to
  mobile app. A token works naturally: the daemon generates it and the app
  presents it. NKeys would require the mobile app to generate an Ed25519 key
  pair and transmit its public key *back* to the daemon for provisioning — but
  no authenticated return channel exists at pairing time.
* **TLS already secures the channel.** The WSS connection uses certificate
  pinning (Section 5.5), so the token is never transmitted in the clear. The
  risk that NKeys mitigate — credential interception on an unencrypted channel —
  does not apply here.
* **Simpler revocation.** Token revocation is a single-step operation: remove
  the token from the NATS auth config and disconnect the client (R-SEC-01). NKey
  revocation is functionally identical (remove the public key) but adds key-pair
  management overhead on the mobile side.
* **Shared broker compatibility.** Token auth is supported by every NATS auth
  backend. NKey support depends on the operator's auth configuration, which
  Renotify does not control.

NKeys remain a viable future enhancement if mutual authentication requirements
emerge (e.g., multi-device pairing where each device proves a distinct
identity).

---

## 7. NATS Authorisation and ACL Design

### 7.1 Actor Roles

| Role | Connection Type | Auth Credential | Typical Client |
| :--- | :--- | :--- | :--- |
| Daemon (internal) | Native TCP on loopback | Internal token (auto-generated, never exposed) | Daemon process, CLI commands |
| Mobile client | WSS over network | Pairing token (`rn_tk_...`) | Android app |

### 7.2 Daemon (Internal) Permissions

The daemon account has full access within its user's namespace:

| Direction | Subject Pattern | Purpose |
| :--- | :--- | :--- |
| Publish | `resystems.renotify.{username}.>` | All subjects (heartbeat, lifecycle, service responses) |
| Subscribe | `resystems.renotify.{username}.>` | All subjects (lifecycle events, interjections, service requests) |
| Publish | `$JS.API.>` | JetStream management (stream and consumer creation) |
| Subscribe | `$JS.API.>` | JetStream management responses |

### 7.3 Mobile Client Permissions

The mobile account has scoped access — it can subscribe broadly but can only
publish to subjects that represent legitimate user actions:

| Direction | Subject Pattern | Purpose |
| :--- | :--- | :--- |
| Subscribe | `resystems.renotify.{username}.>` | Receive all user traffic (notifications, heartbeats, lifecycle) |
| Publish | `resystems.renotify.{username}.flow.*.response` | Send notification responses |
| Publish | `resystems.renotify.{username}.flow.*.interject` | Send interjection commands |
| Publish | `resystems.renotify.{username}.svc.*` | Send service requests (active flows, history queries) |
| Publish | `$JS.ACK.>` | Acknowledge JetStream messages |
| Publish | `$JS.FC.>` | JetStream flow control |
| Publish | `$JS.API.CONSUMER.INFO.>` | Look up consumer info for push subscription binding |
| Subscribe | `_INBOX.>` | Receive Core NATS request-reply responses |

**Security property:** The mobile client cannot publish to `*.request`,
`*.lifecycle`, or `*.heartbeat` subjects. This prevents a compromised mobile
device from:

* Injecting fake notifications (`.request`)
* Fabricating flow lifecycle events (`.lifecycle`)
* Impersonating a daemon (`.heartbeat`)

The mobile client can only respond to notifications, issue interjections, and
query the daemon — the legitimate actions a human developer takes.

### 7.4 Post-MVP ACL Enhancements

The following are explicitly deferred per R-SEC-03:

* Fine-grained per-workspace mobile permissions.
* Automatic token rotation on a scheduled cadence.
* Multi-device pairing support (multiple mobile accounts).

---

## 8. Connection Lifecycles

### 8.1 Daemon Startup (Embedded Broker)

1. Load daemon configuration from `$XDG_CONFIG_HOME/renotify/settings.json`.
2. Load `daemon_id` from `$XDG_STATE_HOME/renotify/daemon_id` (or generate on
   first run).
3. Load TLS certificate and private key from `$XDG_STATE_HOME/renotify/tls/`. If
   missing, log a warning and skip the WSS listener (TCP-only mode; mobile
   connectivity unavailable until `renotify pair`).
4. Load pairing token from `$XDG_STATE_HOME/renotify/pairing/token`. If missing,
   start without a mobile account.
5. Configure embedded NATS server: a. TCP listener on `127.0.0.1:4222` (no TLS).
   b. WSS listener on `0.0.0.0:4223` with TLS cert/key (if available). c. Auth
   accounts: daemon internal + mobile (if token exists).
6. Start embedded NATS server.
7. Enable JetStream with memory storage.
8. Create or verify the `RENOTIFY` stream with the configured subject filter and
   limits.
9. Create or verify durable consumers (`mobile-{username}`,
   `daemon-lifecycle-{username}`, `daemon-interject-{username}`).
10. Subscribe to lifecycle, interjection, and service subjects.
11. Load active flow registry from SQLite and reconcile against any buffered
    lifecycle events.
12. Publish an immediate `DaemonHeartbeat`. Begin the 30-second periodic
    heartbeat timer.

### 8.2 Daemon Startup (Shared Broker)

1. Load daemon configuration (includes shared broker URL and credentials).
2. Load `daemon_id` from XDG state.
3. Connect to the shared broker as a NATS client (TCP or TLS, depending on
   broker configuration).
4. Verify that the `RENOTIFY` stream exists (or create it if the daemon has
   permission).
5. Create or verify durable consumers.
6. Subscribe to lifecycle, interjection, and service subjects.
7. Load active flow registry from SQLite and reconcile.
8. Publish an immediate `DaemonHeartbeat`. Begin the 30-second periodic timer.

### 8.3 Pairing (`renotify pair`)

1. Check for an existing pairing token. If one exists, revoke it (R-SEC-02):
   remove from NATS auth, disconnect mobile client, delete stored token.
2. Check for an existing TLS certificate. If none exists, generate an ECDSA
   P-256 self-signed certificate with SANs for all discovered non-loopback IPs
   + `127.0.0.1` + `localhost`. Store in XDG state.
3. Discover the local IP address. Prefer non-loopback, non-link-local addresses.
   Allow `--ip` flag override.
4. Generate a new auth token: 32 bytes from `crypto/rand`, Crockford Base32
   encoded, prepended with `rn_tk_`.
5. Store the token in XDG state.
6. If the daemon is running, hot-reload the NATS auth configuration to add the
   mobile account with the new token.
7. Compute the certificate fingerprint (SHA-256 of DER, hex-encoded).
8. Assemble the `ProvisioningPayload`:
   `{"h":"{ip}","p":4223,"t":"{token}","c":"{fingerprint}"}`.
9. Encode as minified JSON and render as an ASCII QR code to the terminal.

### 8.4 Mobile First Connection

1. User scans the QR code. The app decodes the `ProvisioningPayload`: host, WSS
   port, auth token, and certificate fingerprint.
2. Store provisioning data in the app's encrypted local storage.
3. Initiate a WSS connection to `wss://{host}:{port}`.
4. During the TLS handshake, the NATS server presents its full certificate. The
   app's custom `X509TrustManager` computes the SHA-256 fingerprint of the
   received certificate and compares it against the stored value.
5. If the fingerprint does not match: abort the connection, display an error.
6. If the fingerprint matches: the TLS handshake completes.
7. Authenticate with the pairing token via NATS `CONNECT` protocol.
8. Create or resume the durable JetStream consumer `mobile-{username}`.
9. Subscribe to `resystems.renotify.{username}.>` via the consumer.
10. Display connection status indicator: connected.

### 8.5 Mobile Reconnection After Network Drop

1. Connection loss detected (network change, server restart, etc.).
2. Display connection status indicator: disconnected (R-MOB-10).
3. Begin exponential backoff reconnection: 1s, 2s, 4s, 8s, 16s, 30s (capped).
4. On each attempt: repeat the TLS handshake with fingerprint verification and
   token authentication.
5. On successful reconnection: resume the durable JetStream consumer. The broker
   redelivers unacked messages from within the TTL window.
6. The app processes buffered messages. In-flight blocking requests that have
   not yet timed out server-side are re-presented to the user (R-MOB-10).
7. The app deduplicates on notification `id` as a safety net against
   at-least-once redelivery.
8. Display connection status indicator: connected.

### 8.6 CLI Command Connection (`renotify ask`)

The CLI reads `settings.json` to determine whether the embedded
broker or a shared broker is in use, then connects accordingly.
Steps 3-9 are identical in both modes — only the connection
target and credential differ.

**Step 1-2 (Embedded broker — `broker.enabled: true`):**

1. Read `broker.tcp_host` and `broker.tcp_port` from daemon
   configuration (default: `127.0.0.1:4222`).
2. Read the internal token from
   `$XDG_STATE_HOME/renotify/internal_token`. If the file does
   not exist, exit with code 1 ("daemon has not been started").
3. Connect via native TCP using the internal token (no TLS —
   loopback only).

**Step 1-2 (Shared broker — `broker.enabled: false`):**

1. Read `shared_broker.url` and credentials (`token` or
   `username`/`password`) from daemon configuration.
2. If `shared_broker.tls_enabled` is true, configure the TLS
   client using `ca_cert` (and optionally `client_cert`/
   `client_key` for mutual TLS).
3. Connect to the shared broker URL using the configured
   credentials.

**Steps 3-12 (common to both modes):**

3. Derive `workspace_id` from the current working directory:
   read `daemon_id` from XDG state, compute
   `SHA-256(daemon_id + "|" + cwd)`, truncate to 80 bits,
   encode as Crockford Base32 with `ws_` prefix. Derive
   `display_name` from `path.Base(cwd)`. See [Naming &
   Addressing](analysis-naming-and-addressing.md) Section 2.4.
4. Generate a `flow_id` (UUIDv7, Crockford Base32 encoded with
   `fl_` prefix).
5. Publish a `FlowLifecycleEvent` (`status: active`) to
   `resystems.renotify.{username}.flow.{flow_id}.lifecycle`.
6. Create an ephemeral JetStream consumer
   `cli-response-{flow_id}` filtering on
   `resystems.renotify.{username}.flow.{flow_id}.response` with
   DeliverPolicy=New.
7. Subscribe to
   `resystems.renotify.{username}.flow.{flow_id}.interject`
   via a second ephemeral consumer `cli-interject-{flow_id}`
   with DeliverPolicy=New.
8. Publish the `NotificationRequest` (including `timeout_sec`
   from the `--timeout` flag or config default) to
   `resystems.renotify.{username}.flow.{flow_id}.request`.
   The daemon reads `timeout_sec` and starts a server-side
   timer (R-CLI-17).
9. Wait concurrently on both consumers (`.response` and
   `.interject`) with no local timer. The daemon is the sole
   timeout enforcer (see Section 3.3). See Section 8.8 for
   interjection handling during this wait.
10. On `NotificationResponse` from `.response`: print the
    result, publish a `FlowLifecycleEvent`
    (`status: completed`), disconnect.
11. On `ErrorResponse` (`code: "timeout"`) from `.response`:
    print the error to stderr, exit with code 3. The daemon
    has already published `FlowLifecycleEvent` (`status: failed`).
12. On `stop` interjection from `.interject`: print "Flow
    stopped by user" to stderr, publish a `FlowLifecycleEvent`
    (`status: failed`), exit with code 1.

The same branching logic applies to `renotify post` (steps 3-8
only, no response wait or interjection subscription) and
`renotify history` (connects, sends a Core NATS request to
`svc.history`, prints result, disconnects).

### 8.7 Token Revocation (`renotify revoke`)

1. Load the stored pairing token from XDG state.
2. If the daemon is running with an embedded broker:
   - a. Remove the mobile account from the NATS auth configuration.
   - b. Signal the NATS server to disconnect any client authenticated with the
        revoked token.
   - c. Hot-reload the auth configuration.
3. If the daemon uses a shared broker:
   - Log a message that shared broker token revocation requires operator
     intervention.
   - (The daemon cannot reconfigure shared broker auth.)
4. Delete the stored token file from XDG state.
5. Confirm revocation to the user via terminal output.

### 8.8 Interjection Delivery

Interjections are out-of-band signals from the mobile user to
the originating process (CLI or MCP agent). The mobile app
publishes an `InterjectionCommand` to the `.interject` subject;
the system delivers it to the process that owns the targeted
flow.

**Daemon processing.** The daemon's `daemon-interject-{username}`
consumer receives all interjections for the user's flows. For
each interjection the daemon:

1. Checks the debounce window. `InterjectionCommand` is the only
   non-idempotent payload in the system — a duplicate `stop` from
   a rapid double-tap would trigger redundant termination logic.
   If the same `flow_id` + `action` combination was processed
   within the last 5 seconds (default, configurable via
   `interjection.debounce_window`), acknowledge the message to
   JetStream but do not process it. Different actions on the
   same flow within the window are not
   deduplicated.
2. Inserts the interjection into the `interjections` table in
   SQLite (see [SQLite Ledger](analysis-sqlite-ledger.md)).
3. Dispatches based on the action (see below).

**`stop` — Graceful termination request:**

The daemon treats `stop` as an authoritative termination signal:

1. Publish a `FlowLifecycleEvent` with `status: failed` to
   `resystems.renotify.{username}.flow.{flow_id}.lifecycle`.
2. Delete the flow from the `active_flows` table.
3. For MCP flows: update the `InterjectionResource` and emit
   `notifications/resources/updated` (see [Payload
   Schemas](analysis-payload-schemas.md) `InterjectionResource`).
4. For CLI flows: the CLI receives the `stop` directly via its
   own `.interject` subscription (Section 8.6, step 11) and
   exits independently. The daemon's lifecycle event publication
   (step 1) ensures the flow registry is updated regardless of
   whether the CLI process exits cleanly.

If the flow has already terminated (`completed` or `failed`),
the `stop` is a no-op: the daemon acknowledges the message but
takes no further action.

**`note` — Informational context:**

The daemon forwards the note without altering flow state:

1. Store the note in the `interjections` table (persisted for
   audit).
2. For MCP flows: update the `InterjectionResource` and emit
   `notifications/resources/updated`.
3. For CLI flows: the CLI receives the `note` directly via its
   `.interject` subscription and prints the `context` field to
   stderr. The CLI continues waiting for `.response`.

**`pause` — Deferred to post-MVP:**

The `pause` action requires a corresponding `resume` mechanism
that does not exist in the current architecture. For MVP, the
daemon treats `pause` as a `note` with context "Pause requested"
and logs a warning. The `InterjectionAction` enum value is
retained for forward compatibility.

**CLI interjection handling.** The CLI `ask` command subscribes
to both `.response` and `.interject` for its flow (Section 8.6,
steps 5-6). It waits concurrently on both consumers:

| Event | CLI Behaviour |
| :--- | :--- |
| `.response` arrives | Normal exit: print response, publish `FlowLifecycleEvent` (`completed`), exit 0 |
| `.interject` with `stop` arrives | Print "Flow stopped by user" to stderr, publish `FlowLifecycleEvent` (`failed`), exit 1 |
| `.interject` with `note` arrives | Print context to stderr, continue waiting |
| `ErrorResponse` (`code: "timeout"`) arrives on `.response` | Print timeout error to stderr, exit 3. Daemon has already published `FlowLifecycleEvent` (`failed`). |

The CLI `post` command does not subscribe to `.interject` because
it exits immediately after publishing (no blocking wait).

**MCP interjection handling.** The daemon mediates interjection
delivery to MCP agents via the `InterjectionResource` MCP
resource. The agent receives a `notifications/resources/updated`
event and reads the resource at
`renotify://interjections/{flow_id}` to obtain the interjection
details. This mirrors the `DecisionResource` pattern used for
`ask` responses. The agent decides how to act on the interjection
based on its own logic.

---

## 9. Deployment Model Comparison

| Aspect | Embedded Broker | Shared Broker |
| :--- | :--- | :--- |
| **NATS server** | In-process, managed by daemon | External, managed by operator |
| **Listener configuration** | Daemon configures TCP + WSS (Section 4.1) | Operator's responsibility (Section 4.2) |
| **JetStream stream** | Daemon creates `RENOTIFY` stream on startup | Operator pre-provisions or daemon creates if permitted |
| **Consumer creation** | Daemon creates all consumers | Daemon creates its consumers; mobile consumer may be operator-managed |
| **TLS certificates** | Self-signed ECDSA P-256, generated by `renotify pair` | Operator-managed (CA-signed or self-signed) |
| **Auth token** | Daemon manages via embedded NATS server API | Operator provisions into broker's auth configuration |
| **ACL enforcement** | Daemon configures two-account model via embedded API | Operator configures in `nats-server.conf` or auth callout |
| **Mobile connection target** | Daemon's IP:`4223` (WSS) | Shared broker's address and WSS port |
| **`renotify pair` output** | QR with daemon's local IP and WSS port | QR with shared broker's address and WSS port (from daemon config) |
| **`renotify revoke`** | Daemon removes token and disconnects client | Daemon deletes local token; operator must revoke on broker |
| **CLI connection target** | Loopback TCP (`127.0.0.1:4222`) | Shared broker URL (from `shared_broker.url` in config) |
| **CLI auth credential** | Internal token (from `$XDG_STATE_HOME/renotify/internal_token`) | Shared broker credentials (from `shared_broker` config section) |
| **Multi-user** | Single user (the daemon owner) | Multiple users, each with own namespace |
| **Multi-daemon** | N/A (one embedded broker per daemon) | Multiple daemons connect as clients; mobile discovers them via heartbeats |

The mobile app's behaviour is identical in both models. It connects
to whatever `host:port` was provisioned, authenticates with the
token, and subscribes to its user's namespace. The CLI's behaviour
is also identical after connection — it publishes to the same
subjects and consumes from the same consumers regardless of broker
topology. Only the connection target and credential differ. The
heartbeat messages from connected daemons provide the structural
context for the dashboard regardless of broker topology.
