// Package apkembed provides the embedded Android APK artefact.
// The dist/ directory is embedded at build time via go:embed.
//
// Default checkout: dist/ contains only .gitignore (no APK).
// Full build (make): Makefile copies the real APK into dist/
// before go build, so it is included in the binary.
//
// See docs/renotify-refinements.md P-02 and D-31.
package apkembed

import "embed"

//go:embed all:dist
var FS embed.FS

// APKName is the path within FS for the Android APK.
const APKName = "dist/app-release.apk"
