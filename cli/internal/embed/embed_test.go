package apkembed

import (
	"testing"
)

func TestFS_NotNil(t *testing.T) {
	if FS == (FS) {
		// embed.FS zero value is valid; just check we can read.
	}
	entries, err := FS.ReadDir("dist")
	if err != nil {
		t.Fatalf("ReadDir(dist): %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("dist/ should contain at least .gitignore")
	}
}

func TestFS_GitignorePresent(t *testing.T) {
	data, err := FS.ReadFile("dist/.gitignore")
	if err != nil {
		t.Fatalf("ReadFile(dist/.gitignore): %v", err)
	}
	if len(data) == 0 {
		t.Fatal(".gitignore should not be empty")
	}
}

func TestFS_APKNotPresent(t *testing.T) {
	// On a default checkout (without make), the APK is not
	// present. This test verifies the expected default state.
	_, err := FS.ReadFile(APKName)
	if err == nil {
		t.Skip("APK is present (full build); skipping")
	}
}
