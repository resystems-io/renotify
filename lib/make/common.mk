# common.mk — Shared build conventions for all subsystems.
#
# Every subsystem Makefile includes this file (via go.mk or
# gradle.mk). It establishes the BUILD_DIR convention. Build
# artifacts always go into a local ./build/ directory so that
# cleanup is a simple rm -rf build.

BUILD_DIR ?= build

# Ensure the build directory exists. Subsystem .mk files use
# this as an order-only prerequisite: target: | $(ENSURE_BUILD_DIR)
ENSURE_BUILD_DIR := $(BUILD_DIR)/.dir
$(ENSURE_BUILD_DIR):
	@mkdir -p $(BUILD_DIR)
	@touch $@
