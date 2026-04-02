# Root Makefile — Renotify monorepo build orchestrator.
#
# Standard targets:
#   make              — build everything (Android APK then CLI with embedding)
#   make build-all    — same as above
#   make build-cli    — build CLI with embedded APK (requires Android build first)
#   make build-cli-dev — build CLI without APK embedding (fast iteration)
#   make build-android — build Android APK only
#   make test         — run all tests (unit + integration + Android JVM)
#   make test-unit    — run unit tests only (no integration tag)
#   make test-integration — run integration tests only
#   make test-all     — run all tests including Android instrumented
#                       tests (requires emulator or connected device)
#   make clean        — remove all build artifacts

.DEFAULT_GOAL := build

.PHONY: build build-all build-cli build-cli-dev build-android
.PHONY: test test-unit test-integration test-all clean

build: build-all

build-all: build-android build-cli

build-android:
	$(MAKE) -C clients/android build

build-cli: build-android
	@mkdir -p cli/embed
	cp clients/android/app/build/outputs/apk/release/app-release.apk \
		cli/embed/app-release.apk
	$(MAKE) -C cli build

build-cli-dev:
	$(MAKE) -C cli build-dev

test:
	$(MAKE) -C cli test
	$(MAKE) -C clients/android test

test-unit:
	$(MAKE) -C cli test-unit
	$(MAKE) -C clients/android test

test-integration:
	$(MAKE) -C cli test-integration

test-all: test
	$(MAKE) -C clients/android emulator-test

clean:
	$(MAKE) -C cli clean
	$(MAKE) -C clients/android clean
	rm -rf cli/embed
