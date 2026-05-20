# Incident Reporting Analysis

This document defines the architecture, data structures, and transport mechanisms for the Renotify platform's incident reporting capability. It addresses refinement items **R-OPS-01** through **R-OPS-05**.

---

## 1. Payload Schema

The system uses a strongly-typed JSON schema for incident reports. 

### 1.1 `IncidentReport`

```json
{
  "report_id": "ntf_... (Crockford Base32)",
  "device_id": "mb_... (Crockford Base32)",
  "timestamp": "2026-05-16T10:00:00Z",
  "incident_type": "managed_crash", // or "unmanaged_kill"
  "incident_details": {
    "exception_type": "java.lang.NullPointerException",
    "message": "Attempt to invoke method on null object",
    "stack_trace": "...",
    "exit_reason": 4 // If ApplicationExitInfo
  },
  "device_context": {
    "os_version": "Android 14",
    "app_version": "1.0.0",
    "battery_level": 85,
    "memory_state": "normal"
  },
  "breadcrumbs": [
    {"timestamp": "...", "message": "NATS connected"},
    {"timestamp": "...", "message": "Rendered notification"}
  ],
  "logcat_tail": "..."
}
```

## 2. Mobile Capture Mechanisms

The Android application implements a dual-capture strategy to maximise observability.

### 2.1 Managed Exceptions (JVM Crashes)
The `Application` class registers a custom `Thread.UncaughtExceptionHandler`. When a fatal JVM exception occurs, this handler intercepts the crash, generates an `IncidentReport` containing the full stack trace and thread state, and writes the JSON payload synchronously to the application's internal cache directory (`Context.getCacheDir()`) before allowing the default exception handler to terminate the process.

### 2.2 Unmanaged Terminations (API 30+)
Upon application startup, the service queries `ActivityManager.getHistoricalProcessExitReasons()`. This API provides structured details on terminations that bypass the JVM exception handler, such as ANRs (Application Not Responding), OOM (Out Of Memory) kills by the low memory killer, and user-initiated force stops.

If an unreported termination is detected, an `IncidentReport` is generated with `incident_type: "unmanaged_kill"` and populated with the `ApplicationExitInfo` details, then queued for transmission.

## 3. Transport Workflow

Transmission of incident reports is deferred and completely decoupled from the crash event itself to ensure reliability.

1. **Persistence:** The `IncidentReport` is saved to disk immediately upon capture.
2. **Scheduling:** The Android `WorkManager` (or the persistent `NatsService` on startup) checks for pending reports in the cache directory.
3. **Transmission:** The NATS client publishes the payload to the following subject:
   `resystems.renotify.{username}.device.{device_id}.telemetry.crash`
4. **Cleanup:** Only upon successful acknowledgement from the NATS broker is the local file deleted from the cache.

## 4. Subject Namespace

Telemetry data is published to a dedicated NATS subject hierarchy:
`resystems.renotify.{username}.device.{device_id}.telemetry.crash`

### 4.1 Rationale
Existing operational state events (e.g., requests, responses, interjections) are typically scoped to a specific flow via the `flow.{flow_id}` namespace. However, incident telemetry is inherently tied to the client application and device lifecycle, not an individual notification flow. 

By scoping the subject to `device.{device_id}`, the system ensures that:
- Incidents that occur outside the context of an active flow (e.g., during app startup, background synchronisation, or UI navigation) are still correctly routed and identifiable.
- The daemon can apply distinct ACLs and JetStream mapping rules for device telemetry versus flow operational data.
- The `{username}` token remains at the root to preserve multi-tenant boundary isolation.

## 5. Storage Strategy & Alternatives

### 5.1 Chosen Approach: File-Backed JetStream
The daemon will configure a dedicated JetStream stream named `RENOTIFY_TELEMETRY`. 
- **Storage:** `File`
- **Location:** `$XDG_STATE_HOME/renotify/jetstream/`
- **Retention:** 7 Days (or 100MB limit)

This approach isolates diagnostic data completely from the operational state management. The SQLite ledger (`renotify.db`) is not involved.

### 5.2 Alternatives Considered
**Alternative 1: Memory Stream + SQLite Drain**
- *Description:* Use a memory-backed JetStream (matching `RENOTIFY`) and immediately consume/insert the payloads into a new `incident_reports` SQLite table.
- *Rationale for Rejection:* While it keeps SQLite as the sole state authority (avoiding disk bloat), it introduces a small but critical window for data loss. If the daemon crashes immediately after the NATS broker receives the message in memory, the crash report is permanently lost. Telemetry requires true durability.

## 6. CLI Tooling

Because telemetry is isolated from the SQLite ledger, it requires dedicated access tooling.

- `renotify telemetry list`: Connects to the JetStream API, queries the `RENOTIFY_TELEMETRY` stream, and outputs a one-line summary per message (Timestamp, Device ID, Type, Exception).
- `renotify telemetry fetch`: Retrieves all pending/historical JSON payloads from the stream and saves them sequentially into a local directory for detailed analysis or submission to an external system.
