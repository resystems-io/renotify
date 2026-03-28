include ../../lib/make/gradle.mk

# Ensure local.properties exists with sdk.dir from ANDROID_HOME.
# This file is gitignored and machine-specific.
$(shell \
	if [ ! -f local.properties ] && [ -n "$$ANDROID_HOME" ]; then \
		echo "sdk.dir=$$ANDROID_HOME" > local.properties; \
	fi \
)
