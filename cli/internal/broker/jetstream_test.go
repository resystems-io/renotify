// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package broker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"go.resystems.io/renotify/cli/internal/config"
)

func defaultJSConfig() config.JetStreamConfig {
	return config.JetStreamConfig{
		MaxAge:         config.NewDuration(30 * time.Minute),
		MaxBytes:       134217728,
		MaxMsgSize:     65536,
		MaxMsgsPerSubj: 1000,
		DupWindow:      config.NewDuration(2 * time.Minute),
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startTestServer creates and starts an embedded NATS server for
// testing, returning the server and a connected client.
func startTestServer(t *testing.T) (*EmbeddedServer, *nats.Conn) {
	t.Helper()
	srv, err := NewEmbeddedServer(EmbeddedConfig{
		TCPHost:         "127.0.0.1",
		TCPPort:         -1,
		Username:        "testuser",
		InternalToken:   "testtoken",
		JetStreamMaxMem: 256 * 1024 * 1024,
	}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Shutdown(context.Background()) })

	nc, err := ConnectEmbedded(srv.Server(), "testtoken", testLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })

	return srv, nc
}

// --- Config builder tests ---

func TestStreamConfig_Defaults(t *testing.T) {
	cfg := streamConfig(defaultJSConfig())

	if cfg.Name != "RENOTIFY" {
		t.Errorf("name = %q, want RENOTIFY", cfg.Name)
	}
	if len(cfg.Subjects) != 1 || cfg.Subjects[0] != StreamSubjects {
		t.Errorf("subjects = %v, want [%s]", cfg.Subjects, StreamSubjects)
	}
	if cfg.Storage != natsjs.MemoryStorage {
		t.Errorf("storage = %v, want MemoryStorage", cfg.Storage)
	}
	if cfg.Retention != natsjs.LimitsPolicy {
		t.Errorf("retention = %v, want LimitsPolicy", cfg.Retention)
	}
	if cfg.Discard != natsjs.DiscardOld {
		t.Errorf("discard = %v, want DiscardOld", cfg.Discard)
	}
	if cfg.Replicas != 1 {
		t.Errorf("replicas = %d, want 1", cfg.Replicas)
	}
	if cfg.MaxAge != 30*time.Minute {
		t.Errorf("max_age = %v, want 30m", cfg.MaxAge)
	}
	if cfg.MaxBytes != 134217728 {
		t.Errorf("max_bytes = %d, want 134217728", cfg.MaxBytes)
	}
	if cfg.MaxMsgSize != 65536 {
		t.Errorf("max_msg_size = %d, want 65536", cfg.MaxMsgSize)
	}
	if cfg.MaxMsgsPerSubject != 1000 {
		t.Errorf("max_msgs_per_subj = %d, want 1000", cfg.MaxMsgsPerSubject)
	}
	if cfg.Duplicates != 2*time.Minute {
		t.Errorf("dup_window = %v, want 2m", cfg.Duplicates)
	}
}

func TestStreamConfig_CustomValues(t *testing.T) {
	jsCfg := config.JetStreamConfig{
		MaxAge:         config.NewDuration(1 * time.Hour),
		MaxBytes:       256 * 1024 * 1024,
		MaxMsgSize:     32768,
		MaxMsgsPerSubj: 500,
		DupWindow:      config.NewDuration(5 * time.Minute),
	}
	cfg := streamConfig(jsCfg)

	// Custom values propagate.
	if cfg.MaxAge != 1*time.Hour {
		t.Errorf("max_age = %v, want 1h", cfg.MaxAge)
	}
	if cfg.MaxBytes != 256*1024*1024 {
		t.Errorf("max_bytes = %d, want 256MB", cfg.MaxBytes)
	}

	// Hardcoded values remain fixed.
	if cfg.Storage != natsjs.MemoryStorage {
		t.Error("storage should remain MemoryStorage")
	}
	if cfg.Retention != natsjs.LimitsPolicy {
		t.Error("retention should remain LimitsPolicy")
	}
	if cfg.Discard != natsjs.DiscardOld {
		t.Error("discard should remain DiscardOld")
	}
	if cfg.Replicas != 1 {
		t.Error("replicas should remain 1")
	}
}

func TestMobileConsumerConfig(t *testing.T) {
	cc := mobileConsumerConfig("alice")

	if cc.Durable != "mobile-alice" {
		t.Errorf("durable = %q, want mobile-alice", cc.Durable)
	}
	if cc.FilterSubject != "resystems.renotify.alice.flow.>" {
		t.Errorf("filter = %q", cc.FilterSubject)
	}
	if cc.AckPolicy != natsjs.AckExplicitPolicy {
		t.Errorf("ack = %v, want explicit", cc.AckPolicy)
	}
	if cc.MaxDeliver != 3 {
		t.Errorf("max_deliver = %d, want 3", cc.MaxDeliver)
	}
	if cc.MaxAckPending != 256 {
		t.Errorf("max_ack_pending = %d, want 256", cc.MaxAckPending)
	}
	if cc.DeliverPolicy != natsjs.DeliverAllPolicy {
		t.Errorf("deliver = %v, want all", cc.DeliverPolicy)
	}
	if cc.InactiveThreshold != 0 {
		t.Errorf("inactive = %v, want 0 (no auto-deletion)",
			cc.InactiveThreshold)
	}
}

func TestLifecycleConsumerConfig(t *testing.T) {
	cc := lifecycleConsumerConfig("alice")

	if cc.Durable != "daemon-lifecycle-alice" {
		t.Errorf("durable = %q", cc.Durable)
	}
	if cc.FilterSubject != "resystems.renotify.alice.flow.*.lifecycle" {
		t.Errorf("filter = %q", cc.FilterSubject)
	}
	if cc.MaxAckPending != 64 {
		t.Errorf("max_ack_pending = %d, want 64", cc.MaxAckPending)
	}
	if cc.InactiveThreshold != 5*time.Minute {
		t.Errorf("inactive = %v, want 5m", cc.InactiveThreshold)
	}
}

func TestInterjectConsumerConfig(t *testing.T) {
	cc := interjectConsumerConfig("alice")

	if cc.Durable != "daemon-interject-alice" {
		t.Errorf("durable = %q", cc.Durable)
	}
	if cc.FilterSubject != "resystems.renotify.alice.flow.*.interject" {
		t.Errorf("filter = %q", cc.FilterSubject)
	}
	if cc.MaxAckPending != 64 {
		t.Errorf("max_ack_pending = %d, want 64", cc.MaxAckPending)
	}
	if cc.InactiveThreshold != 5*time.Minute {
		t.Errorf("inactive = %v, want 5m", cc.InactiveThreshold)
	}
}

func TestConsumerNames(t *testing.T) {
	if got := MobileConsumerName("bob"); got != "mobile-bob" {
		t.Errorf("MobileConsumerName = %q", got)
	}
	if got := LifecycleConsumerName("bob"); got != "daemon-lifecycle-bob" {
		t.Errorf("LifecycleConsumerName = %q", got)
	}
	if got := InterjectConsumerName("bob"); got != "daemon-interject-bob" {
		t.Errorf("InterjectConsumerName = %q", got)
	}
}

// --- Embedded server tests ---

func TestEnsureJetStream_Creates(t *testing.T) {
	_, nc := startTestServer(t)
	ctx := context.Background()

	err := EnsureJetStream(ctx, nc, "testuser", nil,
		defaultJSConfig(), testLogger())
	if err != nil {
		t.Fatalf("EnsureJetStream: %v", err)
	}

	js, _ := natsjs.New(nc)

	// Verify stream.
	stream, err := js.Stream(ctx, StreamName)
	if err != nil {
		t.Fatalf("stream not found: %v", err)
	}
	info := stream.CachedInfo()
	if info.Config.Storage != natsjs.MemoryStorage {
		t.Error("stream storage should be memory")
	}

	// Verify all 3 consumers exist. The mobile consumer is push;
	// lifecycle and interject are pull.
	if _, err := js.PushConsumer(ctx, StreamName,
		"mobile-testuser"); err != nil {
		t.Errorf("mobile consumer not found: %v", err)
	}
	if _, err := js.Consumer(ctx, StreamName,
		"daemon-lifecycle-testuser"); err != nil {
		t.Errorf("lifecycle consumer not found: %v", err)
	}
	if _, err := js.Consumer(ctx, StreamName,
		"daemon-interject-testuser"); err != nil {
		t.Errorf("interject consumer not found: %v", err)
	}
}

func TestEnsureJetStream_Idempotent(t *testing.T) {
	_, nc := startTestServer(t)
	ctx := context.Background()
	cfg := defaultJSConfig()

	if err := EnsureJetStream(ctx, nc, "testuser", nil,
		cfg, testLogger()); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := EnsureJetStream(ctx, nc, "testuser", nil,
		cfg, testLogger()); err != nil {
		t.Fatalf("second call: %v", err)
	}

	// Verify stream still exists.
	js, _ := natsjs.New(nc)
	if _, err := js.Stream(ctx, StreamName); err != nil {
		t.Fatalf("stream missing after second call: %v", err)
	}
}

func TestEnsureJetStream_UpdatesStream(t *testing.T) {
	_, nc := startTestServer(t)
	ctx := context.Background()

	cfg := defaultJSConfig()
	EnsureJetStream(ctx, nc, "testuser", nil, cfg, testLogger())

	// Update MaxAge.
	cfg.MaxAge = config.NewDuration(1 * time.Hour)
	if err := EnsureJetStream(ctx, nc, "testuser", nil,
		cfg, testLogger()); err != nil {
		t.Fatalf("update: %v", err)
	}

	js, _ := natsjs.New(nc)
	stream, _ := js.Stream(ctx, StreamName)
	info := stream.CachedInfo()
	if info.Config.MaxAge != 1*time.Hour {
		t.Errorf("max_age = %v, want 1h", info.Config.MaxAge)
	}
}

func TestEnsureJetStream_MobileConsumerReceives(t *testing.T) {
	_, nc := startTestServer(t)
	ctx := context.Background()

	EnsureJetStream(ctx, nc, "testuser", nil, defaultJSConfig(), testLogger())

	// The mobile consumer is push-based with a deliver subject.
	// Subscribe to the deliver subject to receive messages.
	deliverSubject := "resystems.renotify.testuser.mobile.deliver"
	sub, err := nc.SubscribeSync(deliverSubject)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	nc.Flush()

	// Publish to a flow request subject.
	js, _ := natsjs.New(nc)
	_, err = js.Publish(ctx,
		"resystems.renotify.testuser.flow.f001.request",
		[]byte("hello mobile"))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Receive from the push deliver subject.
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if string(msg.Data) != "hello mobile" {
		t.Errorf("data = %q, want 'hello mobile'",
			string(msg.Data))
	}
}

func TestEnsureJetStream_LifecycleConsumerReceives(t *testing.T) {
	_, nc := startTestServer(t)
	ctx := context.Background()

	if err := EnsureJetStream(ctx, nc, "testuser", nil,
		defaultJSConfig(), testLogger()); err != nil {
		t.Fatalf("EnsureJetStream: %v", err)
	}

	js, _ := natsjs.New(nc)
	if _, err := js.Publish(ctx,
		"resystems.renotify.testuser.flow.f001.lifecycle",
		[]byte("flow started")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	consumer, err := js.Consumer(ctx, StreamName,
		LifecycleConsumerName("testuser"))
	if err != nil {
		t.Fatalf("consumer: %v", err)
	}
	msgs, err := consumer.Fetch(1, natsjs.FetchMaxWait(2*time.Second))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	var received int
	for msg := range msgs.Messages() {
		if string(msg.Data()) != "flow started" {
			t.Errorf("data = %q", string(msg.Data()))
		}
		msg.Ack()
		received++
	}
	if received != 1 {
		t.Errorf("received %d messages, want 1", received)
	}
}

func TestEnsureJetStream_InterjectConsumerReceives(t *testing.T) {
	_, nc := startTestServer(t)
	ctx := context.Background()

	if err := EnsureJetStream(ctx, nc, "testuser", nil,
		defaultJSConfig(), testLogger()); err != nil {
		t.Fatalf("EnsureJetStream: %v", err)
	}

	js, _ := natsjs.New(nc)
	if _, err := js.Publish(ctx,
		"resystems.renotify.testuser.flow.f001.interject",
		[]byte("stop")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	consumer, err := js.Consumer(ctx, StreamName,
		InterjectConsumerName("testuser"))
	if err != nil {
		t.Fatalf("consumer: %v", err)
	}
	msgs, err := consumer.Fetch(1, natsjs.FetchMaxWait(2*time.Second))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	var received int
	for msg := range msgs.Messages() {
		if string(msg.Data()) != "stop" {
			t.Errorf("data = %q", string(msg.Data()))
		}
		msg.Ack()
		received++
	}
	if received != 1 {
		t.Errorf("received %d messages, want 1", received)
	}
}

func TestEnsureJetStream_ConsumerIsolation(t *testing.T) {
	_, nc := startTestServer(t)
	ctx := context.Background()

	if err := EnsureJetStream(ctx, nc, "testuser", nil,
		defaultJSConfig(), testLogger()); err != nil {
		t.Fatalf("EnsureJetStream: %v", err)
	}

	// Publish to lifecycle subject.
	js, _ := natsjs.New(nc)
	if _, err := js.Publish(ctx,
		"resystems.renotify.testuser.flow.f001.lifecycle",
		[]byte("lifecycle event")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Mobile consumer should see it (push, filter: .flow.>).
	deliverSubject := "resystems.renotify.testuser.mobile.deliver"
	sub, err := nc.SubscribeSync(deliverSubject)
	if err != nil {
		t.Fatalf("subscribe mobile: %v", err)
	}
	nc.Flush()

	// Re-publish so the push subscriber receives it (the first
	// publish may have been delivered before we subscribed).
	js.Publish(ctx,
		"resystems.renotify.testuser.flow.f001.lifecycle",
		[]byte("lifecycle event 2"))

	msg, err := sub.NextMsg(2 * time.Second)
	mobileCount := 0
	if err == nil && msg != nil {
		mobileCount = 1
	}
	if mobileCount != 1 {
		t.Errorf("mobile received %d, want 1", mobileCount)
	}

	// Interject consumer should NOT see it (filter: .interject).
	interject, _ := js.Consumer(ctx, StreamName,
		InterjectConsumerName("testuser"))
	msgs, _ := interject.Fetch(1, natsjs.FetchMaxWait(500*time.Millisecond))
	var interjectCount int
	for msg := range msgs.Messages() {
		msg.Ack()
		interjectCount++
	}
	if interjectCount != 0 {
		t.Errorf("interject received %d, want 0", interjectCount)
	}
}
