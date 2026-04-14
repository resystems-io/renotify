// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package broker

import "testing"

func TestServiceSubjects(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) string
		want string
	}{
		{"flows", ServiceFlowsSubject,
			"resystems.renotify.alice.svc.flows"},
		{"history", ServiceHistorySubject,
			"resystems.renotify.alice.svc.history"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn("alice")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDeviceControlSubject(t *testing.T) {
	got := DeviceControlSubject("alice", "mb_DEV01")
	want := "resystems.renotify.alice.device.mb_DEV01.control"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeviceHeartbeatSubject(t *testing.T) {
	got := DeviceHeartbeatSubject("alice", "mb_DEV01")
	want := "resystems.renotify.alice.device.mb_DEV01.heartbeat"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestServiceDevicePresenceSubject(t *testing.T) {
	got := ServiceDevicePresenceSubject("alice")
	want := "resystems.renotify.alice.svc.device-presence"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMCPSubjects(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"c2s", MCPClientToServerSubject("alice", "ms_SESS01"),
			"resystems.renotify.alice.mcp.ms_SESS01.c2s"},
		{"s2c", MCPServerToClientSubject("alice", "ms_SESS01"),
			"resystems.renotify.alice.mcp.ms_SESS01.s2c"},
		{"open", MCPSessionOpenSubject("alice"),
			"resystems.renotify.alice.mcp.open"},
		{"close", MCPSessionCloseSubject("alice"),
			"resystems.renotify.alice.mcp.close"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestFlowSubjects(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string, string) string
		want string
	}{
		{
			"request",
			FlowRequestSubject,
			"resystems.renotify.alice.flow.fl_TEST.request",
		},
		{
			"response",
			FlowResponseSubject,
			"resystems.renotify.alice.flow.fl_TEST.response",
		},
		{
			"lifecycle",
			FlowLifecycleSubject,
			"resystems.renotify.alice.flow.fl_TEST.lifecycle",
		},
		{
			"interject",
			FlowInterjectSubject,
			"resystems.renotify.alice.flow.fl_TEST.interject",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn("alice", "fl_TEST")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
