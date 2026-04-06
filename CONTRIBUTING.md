# Contributing

Thank you for considering contributing to this project! We welcome contributions
from everyone.

## 1. Legal

By contributing to this repository, you agree that your contributions will be
licensed under the project's [MIT License](./LICENSE).

### Mandatory

To ensure that all code contributed to this project is legally valid, we require
all contributors to sign off on their commits.

By signing off, you adhere to the [Developer Certificate of Origin
1.1][dco].

**The Rule:**
All commits must include a `Signed-off-by` line.

```text
Signed-off-by: Alice Developer <alice@example.com>
```

To add the signed-off entry automatically use:
```sh
git commit -s -m "subsystem: add new feature"
```

[dco]:https://developercertificate.org/ "Developer Certificate of Origin (DCO)"

## 2. Prerequisites

Building from source requires:

- **Go** (see `cli/go.mod` for minimum version)
- **Android SDK** with `ANDROID_HOME` set
- **JDK 17+** (for Gradle/Kotlin compilation)
- **GNU Make**

See the [README](./README.md#quick-start) for full setup instructions.

### Running Tests

```sh
# Unit tests (Go + Android JVM) — no device required
make test

# All tests including Android instrumented tests (requires emulator)
make test-all
```

## 3. How to Contribute

* **Bugs:** Open an issue describing the bug and how to reproduce it.
* **Features:** Open an issue **first** to discuss the new feature. We want to
  make sure it fits the project goals before you write code!
* **Pull Requests:**
	1. Fork the repo and create your branch from `main`.
	2. If you've added code that should be tested, add tests.
	3. Ensure the test suite passes (`make test`).
	4. Make sure your code lints.
	5. Sign off your commits (Required: `git commit -s`).

### Commit Messages

Commit messages use an **area prefix**, not conventional commits:

```text
daemon: add health check endpoint
cli: fix timeout flag parsing
android: update reconnection backoff
docs: clarify pairing instructions
```

Common prefixes: `daemon:`, `cli:`, `android:`, `docs:`, `build:`.

## 4. Commit Signing

Optional, though encouraged, please cryptographically sign your commits using
`-S`:

```
git commit -S -s -m "subsystem: add new feature"
```

## 5. Code of Conduct

We are committed to providing a friendly, safe and welcoming environment for
all. Please be respectful to others.
