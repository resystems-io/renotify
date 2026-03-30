# gradle.mk — Gradle build, test, and clean patterns.
#
# Include this from a subsystem Makefile that builds an Android
# application via Gradle. Expects gradlew to be in the same
# directory as the including Makefile.
#
# Standard targets:
#   make                — build the release APK (default)
#   make build          — same as above
#   make build-debug    — build the debug APK
#   make test           — run JVM unit tests (no device required)
#   make connected-test — run instrumented tests (requires
#                         running emulator or connected device)
#   make clean          — run Gradle clean and remove build directory

include $(dir $(lastword $(MAKEFILE_LIST)))common.mk

GRADLEW := ./gradlew

.DEFAULT_GOAL := build

.PHONY: build build-debug test connected-test clean

build:
	$(GRADLEW) assembleRelease

build-debug:
	$(GRADLEW) assembleDebug

test:
	$(GRADLEW) test

connected-test:
	$(GRADLEW) connectedAndroidTest

clean:
	$(GRADLEW) clean 2>/dev/null || true
	rm -rf $(BUILD_DIR)
