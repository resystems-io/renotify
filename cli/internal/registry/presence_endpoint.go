// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package registry

import (
	"encoding/json"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/statesvc"
)

// subscribeDevicePresenceEndpoint subscribes to the
// svc.device-presence subject and handles incoming device
// presence queries (R-CLI-23).
func (s *Service) subscribeDevicePresenceEndpoint() (*nats.Subscription, error) {
	subject := broker.ServiceDevicePresenceSubject(s.username)
	sub, err := s.nc.Subscribe(subject, s.handleDevicePresence)
	if err != nil {
		return nil, err
	}
	s.logger.Info("svc.device-presence endpoint ready",
		"subject", subject)
	return sub, nil
}

// handleDevicePresence processes a single svc.device-presence
// request. Returns an empty device list if no presence tracker
// is configured.
func (s *Service) handleDevicePresence(msg *nats.Msg) {
	var result statesvc.DevicePresenceResult
	if s.presence != nil {
		result.Devices = s.presence.DevicePresence()
	}
	if result.Devices == nil {
		result.Devices = []statesvc.DeviceStatus{}
	}
	data, _ := json.Marshal(result)
	msg.Respond(data)
}
