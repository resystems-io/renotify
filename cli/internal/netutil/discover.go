// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package netutil provides network discovery utilities for the
// Renotify daemon. See docs/analysis-nats-transport-design.md
// Section 5.1 (SANs) and Section 8.3 (IP discovery).
package netutil

import (
	"fmt"
	"net"
)

// DiscoverIPs returns all non-loopback, non-link-local IP
// addresses from active network interfaces. Both IPv4 and IPv6
// addresses are returned. The caller should add 127.0.0.1 and
// localhost to the TLS SANs separately.
func DiscoverIPs() ([]net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}

	var ips []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagRunning == 0 {
			continue // no carrier (e.g. virtual bridges, disconnected ethernet)
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
				ip.IsLinkLocalMulticast() {
				continue
			}
			ips = append(ips, ip)
		}
	}
	return ips, nil
}

// PreferredIP selects the best IP for the provisioning payload's
// host field. Preference order: IPv4 private, any IPv4, any IPv6,
// fallback to 127.0.0.1.
func PreferredIP(ips []net.IP) net.IP {
	var firstIPv4, firstIPv6 net.IP

	for _, ip := range ips {
		if ip4 := ip.To4(); ip4 != nil {
			if isPrivateIPv4(ip4) {
				return ip4
			}
			if firstIPv4 == nil {
				firstIPv4 = ip4
			}
		} else if firstIPv6 == nil {
			firstIPv6 = ip
		}
	}

	if firstIPv4 != nil {
		return firstIPv4
	}
	if firstIPv6 != nil {
		return firstIPv6
	}
	return net.ParseIP("127.0.0.1")
}

// isPrivateIPv4 checks if an IPv4 address is in a private range
// (RFC 1918: 10/8, 172.16/12, 192.168/16).
func isPrivateIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 10 ||
		(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
		(ip4[0] == 192 && ip4[1] == 168)
}
