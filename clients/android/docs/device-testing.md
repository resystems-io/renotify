# Device Testing Playbook

How to install and test the Renotify debug APK on a physical Android device.

## Prerequisites

- Android device running API 26+ (Android 8.0 Oreo or later)
- USB cable connecting the device to the development machine
- `ANDROID_HOME` environment variable set

## 1. Enable Developer Options on the device

1. Open **Settings > About phone**.
2. Tap **Build number** seven times. A toast confirms "You are now a developer!"
3. Go back to **Settings > System > Developer options**.
4. Enable **USB debugging**.

## 2. Enable Developer udev rules on Linux

Set up permissions for `udev` to allow access to ADB devices.

```sh
sudo apt install android-sdk-platform-tools-common
```

---

Or, add specific rules for vendors, e.g.:

```sh
# Add this line for Samsung
SUBSYSTEM=="usb", ATTR{idVendor}=="04e8", MODE="0666", GROUP="plugdev"
# Add this line the LG
SUBSYSTEM=="usb", ATTR{idVendor}=="1004", MODE="0666", GROUP="plugdev"
```

Add your user to the plugev group:

```sh
sudo usermod -aG plugdev $USER
```

Then reload the udev rules:

```bash
sudo cp clients/android/99-android.rules /etc/udev/rules.d/
sudo udevadm control --reload-rules
sudo udevadm trigger
```


## 3. Connect and verify

Plug in the USB cable. On the device, accept the "Allow USB debugging?" prompt
(check "Always allow from this computer" for convenience).

```bash
$ANDROID_HOME/platform-tools/adb devices
```

You should see your device listed as `device` (not `unauthorized` or `offline`):

```
List of devices attached
XXXXXXXX    device
```

## 4. Build and install

From the repository root:

```bash
make -C clients/android emulator-install
```

Despite the target name, `emulator-install` installs on any connected device (it
uses `adb install` which targets the first available device). If both an
emulator and a physical device are connected, specify the device:

```bash
$ANDROID_HOME/platform-tools/adb -s <SERIAL> install -r \
    clients/android/app/build/outputs/apk/debug/app-debug.apk
```

## 5. Launch

The app appears in the launcher as **Renotify**. Alternatively:

```bash
$ANDROID_HOME/platform-tools/adb shell am start \
    -n io.resystems.renotify/.MainActivity
```

## 6. Test the pairing flow

On the development machine, start the daemon and generate a pairing QR code:

```bash
renotify daemon start --foreground &
renotify pair
```

On the device:

1. Tap **Scan Pairing QR Code**.
2. Grant camera permission when prompted.
3. Point the camera at the terminal QR code.
4. The app should display "Paired with {ip}:{port}" and return to the main
   screen showing the connection details.

## 7. Verify stored credentials

Re-launch the app. The main screen should still show the paired status
(credentials persist in encrypted storage).

## 8. Test re-pairing

On the development machine, generate a new pairing code:

```bash
renotify pair
```

Scan the new QR code on the device. The old credentials are replaced. The main
screen shows the updated connection details.

## 9. Uninstall

```bash
$ANDROID_HOME/platform-tools/adb uninstall io.resystems.renotify
```

## 10. Network Connectivity for NATS Testing

After pairing, the app starts a foreground service that connects to the daemon's
WSS listener. The device must be able to reach the daemon over the network.

### 10.1 Firewall rules

The daemon's WSS listener (default port 4223) binds to `0.0.0.0` but may be
blocked by the host firewall. On Ubuntu/Debian with `ufw`:

```bash
# Allow WSS from local network
sudo ufw allow from 192.168.0.0/16 to any port 4223 proto tcp

# After testing, revoke the rule:
sudo ufw delete allow from 192.168.0.0/16 to any port 4223 proto tcp
```

### 10.2 Emulator networking

The Android emulator routes `10.0.2.2` to the host's loopback interface. The WSS
listener on `0.0.0.0:4223` is reachable without firewall changes. Generate a
pairing QR with the emulator IP:

```bash
renotify pair --ip 10.0.2.2
```

### 10.3 Verifying the connection

After scanning the pairing QR code, a persistent notification appears showing
the connection state:

- **"Connecting..."** — TLS handshake and auth in progress
- **"Connected to {ip}:{port}"** — success
- **"Disconnected — reconnecting..."** — connection lost, exponential backoff
  active
- **"Error: ..."** — TLS fingerprint mismatch or auth failure

If the connection fails, check:

1. Daemon is running: `renotify daemon start --foreground`
2. WSS listener is active (look for `websocket` in daemon logs)
3. TLS cert exists: `ls ~/.local/state/renotify/tls/`
4. Firewall allows port 4223 from the device's subnet
5. Device can reach the host: ping the host IP from the device

### 10.4 Viewing device logs

The app logs connection events, errors, and state transitions via Android's
`Log` API. Use `adb logcat` to view them in real time or retrieve recent
entries.

**Real-time log stream** (filtered to Renotify tags):

```bash
$ANDROID_HOME/platform-tools/adb logcat -s \
    NatsConnectionManager:* NatsService:* ScannerActivity:*
```

**Dump recent logs** (non-blocking):

```bash
$ANDROID_HOME/platform-tools/adb logcat -d -s \
    NatsConnectionManager:* NatsService:* ScannerActivity:*
```

**All app logs** (broader filter, includes system messages):

```bash
$ANDROID_HOME/platform-tools/adb logcat -d | grep -i renotify
```

**If multiple devices are connected**, specify the target:

```bash
$ANDROID_HOME/platform-tools/adb -s <SERIAL> logcat -s \
    NatsConnectionManager:*
```

Key log messages to look for:

- `NatsConnectionManager: Connected to {ip}:{port}` — success
- `NatsConnectionManager: Connection failed: ...` — first attempt error with
  full exception message
- `NatsConnectionManager: Reconnect attempt N failed: ...` — backoff retry error
- `NatsService: No provisioning data, stopping` — store is empty

The exception message in the log typically reveals the root cause:
`SSLHandshakeException` for TLS/fingerprint issues, `IOException` for network
unreachable, or `AuthorizationException` for invalid tokens.

### 10.5 Testing reconnection

1. Verify the app shows "Connected"
2. Stop the daemon (Ctrl-C in foreground mode)
3. Verify the notification shows "Disconnected — reconnecting..."
4. Restart the daemon: `renotify daemon start --foreground`
5. Verify the app reconnects (exponential backoff: up to 30s)

## 11. Incident & Telemetry Testing

The Renotify application implements a robust dual-capture strategy for incident
and crash telemetry (capturing both managed JVM exceptions and unmanaged system
kills). Because this telemetry is saved directly to the application's secure
sandbox directory, special commands are required to inspect and download these
reports for verification.

### 11.1 Verify Debuggability

To access the secure sandbox using the `run-as` tool, the application must be
built and installed in debug mode (e.g., using `make install`). Release builds
installed on the device do not permit sandbox access.

Verify that the installed application is debuggable:

```bash
$ANDROID_HOME/platform-tools/adb shell dumpsys package io.resystems.renotify | grep flags
```

Look for the `DEBUGGABLE` flag in the output:
```
flags=[ DEBUGGABLE HAS_CODE ALLOW_CLEAR_USER_DATA ALLOW_BACKUP ]
```

### 11.2 Trigger a Test Crash

To generate an incident report and create the cache directory:

1. Launch the application on the device.
2. In the top-left of the application header, **long-press on the Resystems
   logo**.
3. The application will crash immediately and capture the JVM runtime exception.

### 11.3 List Crash Telemetry

Use the `run-as` command to execute a directory listing within the application
sandbox. The telemetry crash reports are saved as JSON files in the
`cache/telemetry/crashes/` directory:

```bash
$ANDROID_HOME/platform-tools/adb shell run-as io.resystems.renotify \
    ls -la cache/telemetry/crashes/
```

This will output the generated reports (named `ntf_<report_id>.json`):
```
total 16
drwx--S--- 2 u0_a572 u0_a572_cache 4096 2026-05-17 14:49 .
drwx--S--- 3 u0_a572 u0_a572_cache 4096 2026-05-17 14:49 ..
-rw------- 1 u0_a572 u0_a572_cache 5867 2026-05-17 14:49 ntf_706d0317fb644b66.json
```

### 11.4 View or Download Crash Reports

Since the security sandbox restricts direct file pulling (`adb pull` will fail
with permission errors), use one of the following methods to view or download
the telemetry JSON files to your development machine:

#### Method A: Direct Command Redirect (Recommended)
You can stream the contents of a report directly from the sandbox and redirect
it to a file on your development machine in a single command:

```bash
$ANDROID_HOME/platform-tools/adb shell "run-as io.resystems.renotify \
    cat cache/telemetry/crashes/ntf_<report_id>.json" > telemetry_report.json
```

#### Method B: Temporary Public Copy
Alternatively, you can copy the report to a public directory on the device
first, pull it, and then clean up:

```bash
# 1. Copy to the public temp directory from inside the sandbox
$ANDROID_HOME/platform-tools/adb shell "run-as io.resystems.renotify \
    cp cache/telemetry/crashes/ntf_<report_id>.json /data/local/tmp/"

# 2. Make the copied file readable
$ANDROID_HOME/platform-tools/adb shell \
    chmod 666 /data/local/tmp/ntf_<report_id>.json

# 3. Pull the file to your machine
$ANDROID_HOME/platform-tools/adb pull \
    /data/local/tmp/ntf_<report_id>.json ./

# 4. Clean up the temporary copy on the device
$ANDROID_HOME/platform-tools/adb shell \
    rm /data/local/tmp/ntf_<report_id>.json
```

---

## Troubleshooting

**"no devices/emulators found"** : Check the USB cable and that USB debugging is
enabled. Run `adb kill-server && adb start-server` to reset the daemon.

**"INSTALL_FAILED_UPDATE_INCOMPATIBLE"** : A prior install with a different
signing key exists. Uninstall first: `adb uninstall io.resystems.renotify`

**Camera permission denied** : The scanner activity shows an explanation and
closes. Re-launch and grant the permission, or go to **Settings > Apps >
  Renotify
  > Permissions > Camera** and enable it manually.

**QR code not scanning** : Ensure the terminal has good contrast (light
background, dark text or vice versa). Hold the phone 20-30 cm from the screen.
  The QR code uses EC level L which requires minimal error recovery — avoid
  covering any part of the code.

**"Error: Fingerprint mismatch"** : The daemon's TLS certificate has changed
since pairing (e.g., `renotify pair --regenerate-cert` was run). Generate a new
  QR code with `renotify pair` and re-scan.

**"Error: Authorization Violation"** : The pairing token has been revoked or
replaced. Generate a new QR code with `renotify pair` and re-scan.

**Notification stuck on "Connecting..."** : Check that the daemon is running and
the WSS listener is reachable. On the emulator, ensure the QR was generated with
  `--ip 10.0.2.2`. On a physical device, check firewall rules (Section 10.1).
