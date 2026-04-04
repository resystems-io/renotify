package broker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/state"
)

// StreamName is the single JetStream stream used by Renotify.
const StreamName = "RENOTIFY"

// StreamSubjects is the subject filter for the RENOTIFY stream.
// Captures all flow-scoped traffic for all users.
const StreamSubjects = "resystems.renotify.*.flow.>"

// MobileConsumerName returns the legacy durable consumer name
// for the mobile app (pre-multi-device).
func MobileConsumerName(username string) string {
	return "mobile-" + username
}

// MobileDeviceConsumerName returns the per-device consumer name.
func MobileDeviceConsumerName(username, deviceID string) string {
	return "mobile-" + username + "-" + deviceID
}

// LifecycleConsumerName returns the durable consumer name for the
// daemon's flow lifecycle processor.
func LifecycleConsumerName(username string) string {
	return "daemon-lifecycle-" + username
}

// InterjectConsumerName returns the durable consumer name for the
// daemon's interjection processor.
func InterjectConsumerName(username string) string {
	return "daemon-interject-" + username
}

// EnsureJetStream creates or updates the RENOTIFY JetStream stream
// and its durable consumers. It is idempotent and safe to call on
// every daemon startup. See docs/analysis-nats-transport-design.md
// Sections 2 and 8.1-8.2.
func EnsureJetStream(
	ctx context.Context,
	nc *nats.Conn,
	username string,
	devices []state.PairedDevice,
	cfg config.JetStreamConfig,
	logger *slog.Logger,
) error {
	js, err := natsjs.New(nc)
	if err != nil {
		return fmt.Errorf("create jetstream handle: %w", err)
	}

	// Create or update the RENOTIFY stream.
	sCfg := streamConfig(cfg)
	if _, err := js.CreateOrUpdateStream(ctx, sCfg); err != nil {
		// On shared brokers the daemon may lack creation
		// permissions. Fall back to verifying the stream exists.
		if isPermissionError(err) {
			logger.Info("stream creation not permitted, "+
				"verifying existing stream",
				"name", StreamName)
			if _, verifyErr := js.Stream(ctx, StreamName); verifyErr != nil {
				return fmt.Errorf("stream %s not found and "+
					"cannot create: %w", StreamName, err)
			}
			logger.Info("stream verified", "name", StreamName)
		} else {
			return fmt.Errorf("create stream %s: %w",
				StreamName, err)
		}
	} else {
		logger.Info("stream ready", "name", StreamName)
	}

	// Create or update durable consumers.
	consumers := []natsjs.ConsumerConfig{
		mobileConsumerConfig(username),
		lifecycleConsumerConfig(username),
		interjectConsumerConfig(username),
	}
	// Per-device consumers for multi-device pairing (R-MOB-11).
	for _, d := range devices {
		consumers = append(consumers,
			mobileDeviceConsumerConfig(username, d.DeviceID))
	}
	for _, cc := range consumers {
		if _, err := js.CreateOrUpdateConsumer(
			ctx, StreamName, cc); err != nil {
			return fmt.Errorf("create consumer %s: %w",
				cc.Durable, err)
		}
		logger.Info("consumer ready",
			"name", cc.Durable, "stream", StreamName)
	}

	return nil
}

// streamConfig builds the RENOTIFY stream configuration from the
// user-configurable parameters plus hardcoded design values.
func streamConfig(cfg config.JetStreamConfig) natsjs.StreamConfig {
	return natsjs.StreamConfig{
		Name:              StreamName,
		Subjects:          []string{StreamSubjects},
		Storage:           natsjs.MemoryStorage,
		Retention:         natsjs.LimitsPolicy,
		Discard:           natsjs.DiscardOld,
		Replicas:          1,
		MaxAge:            cfg.MaxAge.Duration,
		MaxBytes:          cfg.MaxBytes,
		MaxMsgSize:        cfg.MaxMsgSize,
		MaxMsgsPerSubject: cfg.MaxMsgsPerSubj,
		Duplicates:        cfg.DupWindow.Duration,
		MaxConsumers:      -1,
		MaxMsgs:           -1,
	}
}

// mobileConsumerConfig builds the mobile app's durable consumer.
// No InactiveThreshold — the consumer persists until the device
// is revoked via `renotify revoke`. Auto-deletion would break
// reconnection after prolonged disconnections (e.g. overnight).
func mobileConsumerConfig(username string) natsjs.ConsumerConfig {
	return natsjs.ConsumerConfig{
		Durable:        MobileConsumerName(username),
		DeliverSubject: "resystems.renotify." + username + ".mobile.deliver",
		FilterSubject:  "resystems.renotify." + username + ".flow.>",
		AckPolicy:      natsjs.AckExplicitPolicy,
		MaxDeliver:     3,
		MaxAckPending:  256,
		DeliverPolicy:  natsjs.DeliverAllPolicy,
	}
}

// mobileDeviceConsumerConfig builds a per-device push consumer.
// Same config as the legacy mobile consumer but with a
// device-specific name and deliver subject. No
// InactiveThreshold — lifecycle tied to device pairing.
func mobileDeviceConsumerConfig(
	username, deviceID string,
) natsjs.ConsumerConfig {
	return natsjs.ConsumerConfig{
		Durable: MobileDeviceConsumerName(username, deviceID),
		DeliverSubject: "resystems.renotify." + username +
			".mobile." + deviceID + ".deliver",
		FilterSubject: "resystems.renotify." + username + ".flow.>",
		AckPolicy:     natsjs.AckExplicitPolicy,
		MaxDeliver:    3,
		MaxAckPending: 256,
		DeliverPolicy: natsjs.DeliverAllPolicy,
	}
}

// lifecycleConsumerConfig builds the daemon's flow lifecycle
// consumer for maintaining the active flow registry.
func lifecycleConsumerConfig(username string) natsjs.ConsumerConfig {
	return natsjs.ConsumerConfig{
		Durable:           LifecycleConsumerName(username),
		FilterSubject:     "resystems.renotify." + username + ".flow.*.lifecycle",
		AckPolicy:         natsjs.AckExplicitPolicy,
		MaxDeliver:        3,
		MaxAckPending:     64,
		DeliverPolicy:     natsjs.DeliverAllPolicy,
		InactiveThreshold: 5 * time.Minute,
	}
}

// interjectConsumerConfig builds the daemon's interjection
// consumer for routing user commands to flow handlers.
func interjectConsumerConfig(username string) natsjs.ConsumerConfig {
	return natsjs.ConsumerConfig{
		Durable:           InterjectConsumerName(username),
		FilterSubject:     "resystems.renotify." + username + ".flow.*.interject",
		AckPolicy:         natsjs.AckExplicitPolicy,
		MaxDeliver:        3,
		MaxAckPending:     64,
		DeliverPolicy:     natsjs.DeliverAllPolicy,
		InactiveThreshold: 5 * time.Minute,
	}
}

// isPermissionError checks if the error is a NATS permissions or
// authorization error (HTTP 403).
func isPermissionError(err error) bool {
	var apiErr *natsjs.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == 403
	}
	return false
}
