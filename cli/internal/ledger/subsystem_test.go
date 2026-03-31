package ledger

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
)

func TestSubsystem_Name(t *testing.T) {
	s := NewSubsystem("/tmp/test.db", slog.Default())
	if s.Name() != "ledger" {
		t.Errorf("Name() = %q, want %q", s.Name(), "ledger")
	}
}

func TestSubsystem_StartStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s := NewSubsystem(path, slog.Default())

	ready := make(chan error, 1)
	if err := s.Start(context.Background(), nil, ready); err != nil {
		t.Fatal(err)
	}

	// DB should be accessible after Start.
	if s.DB() == nil {
		t.Fatal("DB() should not be nil after Start")
	}

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestSubsystem_StartSignalsReady(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s := NewSubsystem(path, slog.Default())

	ready := make(chan error, 1)
	if err := s.Start(context.Background(), nil, ready); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(context.Background())

	// Ready channel should be closed (receive zero value).
	err, ok := <-ready
	if ok && err != nil {
		t.Fatalf("ready signalled error: %v", err)
	}
}

func TestSubsystem_StartError(t *testing.T) {
	// Use an impossible path to trigger an Open error.
	s := NewSubsystem("/dev/null/impossible/test.db", slog.Default())

	ready := make(chan error, 1)
	err := s.Start(context.Background(), nil, ready)
	if err == nil {
		t.Fatal("Start should fail with impossible path")
	}

	// Ready channel should have received the error.
	select {
	case readyErr := <-ready:
		if readyErr == nil {
			t.Error("ready should signal an error")
		}
	default:
		t.Error("ready channel should have an error")
	}
}
