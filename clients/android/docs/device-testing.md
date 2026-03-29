# Device Testing Playbook

How to install and test the Renotify debug APK on a physical
Android device.

## Prerequisites

- Android device running API 26+ (Android 8.0 Oreo or later)
- USB cable connecting the device to the development machine
- `ANDROID_HOME` environment variable set

## 1. Enable Developer Options on the device

1. Open **Settings > About phone**.
2. Tap **Build number** seven times. A toast confirms "You are
   now a developer!"
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

Plug in the USB cable. On the device, accept the "Allow USB
debugging?" prompt (check "Always allow from this computer" for
convenience).

```bash
$ANDROID_HOME/platform-tools/adb devices
```

You should see your device listed as `device` (not `unauthorized`
or `offline`):

```
List of devices attached
XXXXXXXX    device
```

## 4. Build and install

From the repository root:

```bash
make -C clients/android emulator-install
```

Despite the target name, `emulator-install` installs on any
connected device (it uses `adb install` which targets the first
available device). If both an emulator and a physical device are
connected, specify the device:

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

On the development machine, start the daemon and generate a
pairing QR code:

```bash
renotify daemon start --foreground &
renotify pair
```

On the device:

1. Tap **Scan Pairing QR Code**.
2. Grant camera permission when prompted.
3. Point the camera at the terminal QR code.
4. The app should display "Paired with {ip}:{port}" and return
   to the main screen showing the connection details.

## 7. Verify stored credentials

Re-launch the app. The main screen should still show the paired
status (credentials persist in encrypted storage).

## 8. Test re-pairing

On the development machine, generate a new pairing code:

```bash
renotify pair
```

Scan the new QR code on the device. The old credentials are
replaced. The main screen shows the updated connection details.

## 9. Uninstall

```bash
$ANDROID_HOME/platform-tools/adb uninstall io.resystems.renotify
```

## Troubleshooting

**"no devices/emulators found"**
: Check the USB cable and that USB debugging is enabled. Run
  `adb kill-server && adb start-server` to reset the daemon.

**"INSTALL_FAILED_UPDATE_INCOMPATIBLE"**
: A prior install with a different signing key exists. Uninstall
  first: `adb uninstall io.resystems.renotify`

**Camera permission denied**
: The scanner activity shows an explanation and closes. Re-launch
  and grant the permission, or go to **Settings > Apps > Renotify
  > Permissions > Camera** and enable it manually.

**QR code not scanning**
: Ensure the terminal has good contrast (light background, dark
  text or vice versa). Hold the phone 20-30 cm from the screen.
  The QR code uses EC level L which requires minimal error
  recovery — avoid covering any part of the code.
