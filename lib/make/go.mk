# go.mk — Go build, test, and vet patterns.
#
# Include this from a subsystem Makefile that builds a Go binary.
# Expects the Makefile to be in the Go module root directory
# (alongside go.mod).
#
# Standard targets:
#   make          — build the binary (default)
#   make build    — same as above
#   make build-dev — build without APK embedding (dev tag)
#   make test     — run all tests
#   make vet      — run go vet
#   make clean    — remove build artifacts
#
# NOTE ON .PHONY BUILD TARGETS
#
# The build and build-dev targets are marked .PHONY intentionally.
# Go's own build cache (GOCACHE) provides accurate staleness
# detection across all .go sources, go.mod, go.sum, and embedded
# files. When nothing has changed, `go build` short-circuits in
# ~100ms. Duplicating this in Make via a find-based source list
# would add fragile Makefile complexity for marginal gain.
#
# This is a deliberate trade-off specific to Go. Other subsystems
# (e.g., the Android Gradle build) should use Make-native file
# dependencies with constructed artefacts where build times are
# higher and the build tool's own caching is less reliable.

include $(dir $(lastword $(MAKEFILE_LIST)))common.mk

GO       ?= go
GOFLAGS  ?=
BINARY   := $(BUILD_DIR)/renotify

.DEFAULT_GOAL := build

.PHONY: build build-dev test vet clean

build: | $(ENSURE_BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/renotify

build-dev: | $(ENSURE_BUILD_DIR)
	$(GO) build $(GOFLAGS) -tags dev -o $(BINARY) ./cmd/renotify

test:
	$(GO) test ./... -v -count=1

vet:
	$(GO) vet ./...

clean:
	rm -rf $(BUILD_DIR)
