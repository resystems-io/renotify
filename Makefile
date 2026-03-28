# Root Makefile — Renotify monorepo build orchestrator.
#
# Standard targets:
#   make              — build everything (Android APK then CLI with embedding)
#   make build-all    — same as above
#   make build-cli    — build CLI with embedded APK (requires Android build first)
#   make build-cli-dev — build CLI without APK embedding (fast iteration)
#   make build-android — build Android APK only
#   make test         — run all tests
#   make clean        — remove all build artifacts

.DEFAULT_GOAL := build

.PHONY: build build-all build-cli build-cli-dev build-android
.PHONY: test clean

build: build-all

build-all: build-android build-cli

build-android:
	$(MAKE) -C clients/android build

build-cli: build-android
	@mkdir -p cli/embed
	cp clients/android/app/build/outputs/apk/release/app-release-unsigned.apk \
		cli/embed/app-release.apk
	$(MAKE) -C cli build

build-cli-dev:
	$(MAKE) -C cli build-dev

test:
	$(MAKE) -C cli test

clean:
	$(MAKE) -C cli clean
	$(MAKE) -C clients/android clean
	rm -rf cli/embed
