// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package netutil

import (
	"net"
	"testing"
)

func TestDiscoverIPs_ReturnsResult(t *testing.T) {
	// On any machine with networking, DiscoverIPs should not error.
	// It may return empty on headless CI without network interfaces.
	_, err := DiscoverIPs()
	if err != nil {
		t.Fatalf("DiscoverIPs: %v", err)
	}
}

func TestDiscoverIPs_NoLoopback(t *testing.T) {
	ips, err := DiscoverIPs()
	if err != nil {
		t.Fatalf("DiscoverIPs: %v", err)
	}
	for _, ip := range ips {
		if ip.IsLoopback() {
			t.Errorf("found loopback address %s", ip)
		}
	}
}

func TestDiscoverIPs_NoLinkLocal(t *testing.T) {
	ips, err := DiscoverIPs()
	if err != nil {
		t.Fatalf("DiscoverIPs: %v", err)
	}
	for _, ip := range ips {
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			t.Errorf("found link-local address %s", ip)
		}
	}
}

func TestPreferredIP_IPv4Preferred(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("fe80::1"),
		net.ParseIP("2001:db8::1"),
		net.ParseIP("192.168.1.42"),
	}
	got := PreferredIP(ips)
	if got.String() != "192.168.1.42" {
		t.Errorf("PreferredIP = %s, want 192.168.1.42", got)
	}
}

func TestPreferredIP_PrivatePreferred(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("8.8.8.8"),      // public
		net.ParseIP("192.168.1.42"), // private
	}
	got := PreferredIP(ips)
	if got.String() != "192.168.1.42" {
		t.Errorf("PreferredIP = %s, want 192.168.1.42", got)
	}
}

func TestPreferredIP_FallbackToPublicIPv4(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("8.8.8.8"),
		net.ParseIP("2001:db8::1"),
	}
	got := PreferredIP(ips)
	if got.String() != "8.8.8.8" {
		t.Errorf("PreferredIP = %s, want 8.8.8.8", got)
	}
}

func TestPreferredIP_FallbackToIPv6(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("2001:db8::1"),
	}
	got := PreferredIP(ips)
	if got.String() != "2001:db8::1" {
		t.Errorf("PreferredIP = %s, want 2001:db8::1", got)
	}
}

func TestPreferredIP_FallbackLoopback(t *testing.T) {
	got := PreferredIP(nil)
	if got.String() != "127.0.0.1" {
		t.Errorf("PreferredIP(nil) = %s, want 127.0.0.1", got)
	}
}

func TestPreferredIP_SingleIP(t *testing.T) {
	ips := []net.IP{net.ParseIP("10.0.0.5")}
	got := PreferredIP(ips)
	if got.String() != "10.0.0.5" {
		t.Errorf("PreferredIP = %s, want 10.0.0.5", got)
	}
}

func TestPreferredIP_RFC1918Ranges(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"10.x", "10.0.0.1", true},
		{"172.16.x", "172.16.0.1", true},
		{"172.31.x", "172.31.255.1", true},
		{"172.15.x", "172.15.0.1", false},
		{"172.32.x", "172.32.0.1", false},
		{"192.168.x", "192.168.0.1", true},
		{"public", "8.8.8.8", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isPrivateIPv4(net.ParseIP(tc.ip))
			if got != tc.want {
				t.Errorf("isPrivateIPv4(%s) = %v, want %v",
					tc.ip, got, tc.want)
			}
		})
	}
}
