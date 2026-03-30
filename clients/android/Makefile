include ../../lib/make/gradle.mk

# Ensure local.properties exists with sdk.dir from ANDROID_HOME.
# This file is gitignored and machine-specific.
$(shell \
	if [ ! -f local.properties ] && [ -n "$$ANDROID_HOME" ]; then \
		echo "sdk.dir=$$ANDROID_HOME" > local.properties; \
	fi \
)

# --- Emulator management ---
#
# Emulator targets use the renotify-test AVD created during
# project setup. Override AVD_NAME to use a different device.
#
# Prerequisites (one-time setup):
#
#   # Install command-line tools (if not present):
#   #   Download from https://developer.android.com/studio#command-line-tools-only
#   #   Extract to $ANDROID_HOME/cmdline-tools/latest/
#
#   # Install a system image:
#   $ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager \
#       "system-images;android-36;google_apis;x86_64"
#
#   # Create the AVD:
#   $ANDROID_HOME/cmdline-tools/latest/bin/avdmanager create avd \
#       --name renotify-test \
#       --package "system-images;android-36;google_apis;x86_64" \
#       --device pixel_6
#
# Targets:
#   make emulator          — launch the emulator with a visible
#                            window for interactive testing
#   make emulator-start    — start the AVD headless (no-op if
#                            already running); used by CI and
#                            automated test targets
#   make emulator-stop     — shut down the emulator
#   make install            — build debug APK and install on any
#                            connected device or running emulator
#   make emulator-install   — start emulator if needed, then
#                            install
#   make emulator-test      — start headless emulator if needed,
#                            then run instrumented tests
#   make connected-test     — run instrumented tests only (assumes
#                            emulator or device already connected)

AVD_NAME    ?= renotify-test
EMULATOR    := $(ANDROID_HOME)/emulator/emulator
ADB         := $(ANDROID_HOME)/platform-tools/adb

.PHONY: emulator emulator-start emulator-stop emulator-test emulator-install install install-release uninstall

emulator:
	@echo "Launching emulator $(AVD_NAME) with visible window..."
	@echo "(Close the emulator window or run 'make emulator-stop' to shut down)"
	$(EMULATOR) -avd $(AVD_NAME)

emulator-start:
	@if $(ADB) devices 2>/dev/null | grep -q "emulator-"; then \
		echo "Emulator already running"; \
	else \
		echo "Starting emulator $(AVD_NAME)..."; \
		$(EMULATOR) -avd $(AVD_NAME) -no-window -no-audio \
			-no-boot-anim & \
		$(ADB) wait-for-device; \
		echo "Waiting for boot..."; \
		while [ "$$($(ADB) shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" != "1" ]; do \
			sleep 2; \
		done; \
		echo "Emulator ready"; \
	fi

emulator-stop:
	@if $(ADB) devices 2>/dev/null | grep -q "emulator-"; then \
		$(ADB) emu kill 2>/dev/null || true; \
		echo "Emulator stopped"; \
	else \
		echo "No emulator running"; \
	fi

emulator-test: emulator-start connected-test

install: build-debug
	$(ADB) install -r app/build/outputs/apk/debug/app-debug.apk
	@echo "Installed. Launch with: $(ADB) shell am start -n io.resystems.renotify/.MainActivity"

install-release: build
	$(ADB) install -r app/build/outputs/apk/release/app-release.apk
	@echo "Installed. Launch with: $(ADB) shell am start -n io.resystems.renotify/.MainActivity"

uninstall:
	$(ADB) uninstall io.resystems.renotify

emulator-install: emulator-start install
