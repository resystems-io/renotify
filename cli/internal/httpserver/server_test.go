// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package httpserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestServer_Name(t *testing.T) {
	s := New("127.0.0.1", 0, testLogger())
	if s.Name() != "http" {
		t.Errorf("Name() = %q, want %q", s.Name(), "http")
	}
}

func TestServer_StartSignalsReady(t *testing.T) {
	s := New("127.0.0.1", 0, testLogger())
	ready := make(chan error, 1)
	err := s.Start(context.Background(), nil, ready)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop(context.Background())

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("ready error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for ready")
	}
}

func TestServer_StopClean(t *testing.T) {
	s := New("127.0.0.1", 0, testLogger())
	ready := make(chan error, 1)
	s.Start(context.Background(), nil, ready)
	<-ready

	err := s.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

func TestServer_HandleRegistration(t *testing.T) {
	s := New("127.0.0.1", 0, testLogger())
	s.Handle("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))

	ready := make(chan error, 1)
	s.Start(context.Background(), nil, ready)
	<-ready
	defer s.Stop(context.Background())

	resp, err := http.Get("http://" + s.Addr() + "/test")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestServer_Addr(t *testing.T) {
	s := New("127.0.0.1", 0, testLogger())
	ready := make(chan error, 1)
	s.Start(context.Background(), nil, ready)
	<-ready
	defer s.Stop(context.Background())

	addr := s.Addr()
	if addr == "" {
		t.Error("Addr() should return non-empty after Start")
	}
}
