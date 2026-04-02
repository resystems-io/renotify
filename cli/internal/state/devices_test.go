package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDevices_AddAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")

	d := PairedDevice{
		DeviceID: "mb_TEST01",
		Token:    "rn_tk_TESTTOKEN",
		PairedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	}
	if err := AddDevice(path, d); err != nil {
		t.Fatal(err)
	}

	devices, err := LoadDevices(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if devices[0].DeviceID != "mb_TEST01" {
		t.Errorf("device_id = %q", devices[0].DeviceID)
	}
	if devices[0].Token != "rn_tk_TESTTOKEN" {
		t.Errorf("token = %q", devices[0].Token)
	}
}

func TestDevices_AddMultiple(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")

	AddDevice(path, PairedDevice{
		DeviceID: "mb_DEV01",
		Token:    "rn_tk_TOK01",
		PairedAt: time.Now().UTC(),
	})
	AddDevice(path, PairedDevice{
		DeviceID: "mb_DEV02",
		Token:    "rn_tk_TOK02",
		PairedAt: time.Now().UTC(),
	})

	devices, _ := LoadDevices(path)
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
}

func TestDevices_Remove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")

	AddDevice(path, PairedDevice{
		DeviceID: "mb_DEV01", Token: "rn_tk_TOK01",
		PairedAt: time.Now().UTC()})
	AddDevice(path, PairedDevice{
		DeviceID: "mb_DEV02", Token: "rn_tk_TOK02",
		PairedAt: time.Now().UTC()})

	removed, err := RemoveDevice(path, "mb_DEV01")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("expected device to be removed")
	}

	devices, _ := LoadDevices(path)
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if devices[0].DeviceID != "mb_DEV02" {
		t.Errorf("remaining device = %q", devices[0].DeviceID)
	}
}

func TestDevices_RemoveNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")

	AddDevice(path, PairedDevice{
		DeviceID: "mb_DEV01", Token: "rn_tk_TOK01",
		PairedAt: time.Now().UTC()})

	removed, err := RemoveDevice(path, "mb_NONEXISTENT")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("should not have removed anything")
	}
}

func TestDevices_Clear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")

	AddDevice(path, PairedDevice{
		DeviceID: "mb_DEV01", Token: "rn_tk_TOK01",
		PairedAt: time.Now().UTC()})

	if err := ClearDevices(path); err != nil {
		t.Fatal(err)
	}

	devices, _ := LoadDevices(path)
	if len(devices) != 0 {
		t.Errorf("got %d devices after clear, want 0",
			len(devices))
	}
}

func TestDevices_LoadEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")

	devices, err := LoadDevices(path)
	if err != nil {
		t.Fatal(err)
	}
	if devices != nil {
		t.Errorf("got %v, want nil for missing file", devices)
	}
}

func TestDevices_MigrateFromSingleToken(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	usernamePath := filepath.Join(dir, "username")
	devicesPath := filepath.Join(dir, "devices.json")

	os.WriteFile(tokenPath, []byte("rn_tk_LEGACY\n"), 0600)
	os.WriteFile(usernamePath, []byte("stewart\n"), 0600)

	if err := MigrateFromSingleToken(
		tokenPath, usernamePath, devicesPath,
	); err != nil {
		t.Fatal(err)
	}

	devices, err := LoadDevices(devicesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if devices[0].Token != "rn_tk_LEGACY" {
		t.Errorf("token = %q", devices[0].Token)
	}
	if devices[0].DeviceID == "" {
		t.Error("expected generated device_id")
	}

	// Legacy files should be removed.
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Error("legacy token file should be deleted")
	}
	if _, err := os.Stat(usernamePath); !os.IsNotExist(err) {
		t.Error("legacy username file should be deleted")
	}
}

func TestDevices_MigrateNoop_AlreadyMigrated(t *testing.T) {
	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")

	// Create existing devices.json.
	SaveDevices(devicesPath, []PairedDevice{{
		DeviceID: "mb_EXISTING",
		Token:    "rn_tk_EXIST",
		PairedAt: time.Now().UTC(),
	}})

	// Write a legacy token.
	tokenPath := filepath.Join(dir, "token")
	os.WriteFile(tokenPath, []byte("rn_tk_LEGACY\n"), 0600)

	// Migration should be a no-op.
	MigrateFromSingleToken(tokenPath, "", devicesPath)

	devices, _ := LoadDevices(devicesPath)
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if devices[0].DeviceID != "mb_EXISTING" {
		t.Errorf("device should be unchanged, got %q",
			devices[0].DeviceID)
	}
}

func TestNatsUsername(t *testing.T) {
	got := NatsUsername("mb_3G2K7V9WNFQ4J")
	want := "mobile-mb_3G2K7V9WNFQ4J"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
