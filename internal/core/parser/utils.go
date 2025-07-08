package parser

import (
	"fmt"
	"net"
	"strings"
)

// isLocalhostIP checks if an IP address is localhost/loopback
func isLocalhostIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.Equal(net.IPv4(127, 0, 0, 1)) || ip.Equal(net.IPv6loopback)
}

// isPrivateIP checks if an IP is in private ranges (optional additional filtering)
// RFC 5735 https://tools.ietf.org/html/rfc5735
// RFC 6890 https://tools.ietf.org/html/rfc6890
func isPrivateIP(ip net.IP) bool {
	privateRanges := []*net.IPNet{
		// IPv4
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},      // 10.0.0.0/8 (Private)
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},   // 172.16.0.0/12 (Private)
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},  // 192.168.0.0/16 (Private)
		{IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)},  // 169.254.0.0/16 (Link-local)
		{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)},   // 100.64.0.0/10 (Carrier-grade NAT)
		{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},     // 127.0.0.0/8 (Loopback)
		{IP: net.IPv4(192, 0, 0, 0), Mask: net.CIDRMask(24, 32)},    // 192.0.0.0/24 (IETF Protocol Assignments)
		{IP: net.IPv4(192, 0, 2, 0), Mask: net.CIDRMask(24, 32)},    // 192.0.2.0/24 (TEST-NET-1)
		{IP: net.IPv4(198, 18, 0, 0), Mask: net.CIDRMask(15, 32)},   // 198.18.0.0/15 (Network Interconnect Device Benchmark Testing)
		{IP: net.IPv4(198, 51, 100, 0), Mask: net.CIDRMask(24, 32)}, // 198.51.100.0/24 (TEST-NET-2)
		{IP: net.IPv4(203, 0, 113, 0), Mask: net.CIDRMask(24, 32)},  // 203.0.113.0/24 (TEST-NET-3)
		{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(8, 32)},       // 0.0.0.0/8 (Current network)

		// IPv6
		{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},       // ::1/128 (Loopback)
		{IP: net.ParseIP("::"), Mask: net.CIDRMask(128, 128)},        // ::/128 (Unspecified)
		{IP: net.ParseIP("fc00::"), Mask: net.CIDRMask(7, 128)},      // fc00::/7 (Unique Local)
		{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)},     // fe80::/10 (Link-local)
		{IP: net.ParseIP("2001:db8::"), Mask: net.CIDRMask(32, 128)}, // 2001:db8::/32 (Documentation)
		{IP: net.ParseIP("2001:10::"), Mask: net.CIDRMask(28, 128)},  // 2001:10::/28 (Deprecated ORCHID)
		{IP: net.ParseIP("2001:2::"), Mask: net.CIDRMask(48, 128)},   // 2001:2::/48 (Benchmarking)
	}

	for _, privateRange := range privateRanges {
		if privateRange.Contains(ip) {
			return true
		}
	}

	return false
}

// validateAddress checks if an address is not localhost
func validateAddress(address string) error {
	if address == "" {
		return nil
	}

	// Check for obvious localhost hostnames
	lowerAddr := strings.ToLower(address)
	if lowerAddr == "localhost" || lowerAddr == "localhost.localdomain" {
		return fmt.Errorf("%w: %s", ErrLocalhostBlocked, address)
	}

	// parse as IP first
	if ip := net.ParseIP(address); ip != nil {
		if isLocalhostIP(ip) {
			return fmt.Errorf("%w: %s", ErrLocalhostBlocked, address)
		}
		return nil
	}

	// resolve hostname
	ips, err := net.LookupIP(address)
	if err != nil {
		// If DNS resolution fails, we'll allow it and let the connection fail later
		// This prevents DNS issues from blocking valid configs
		return nil
	}

	// Check if any resolved IP is localhost
	for _, ip := range ips {
		if isLocalhostIP(ip) {
			return fmt.Errorf("%w: hostname %s resolves to localhost IP %s", ErrLocalhostBlocked, address, ip)
		}
		if isPrivateIP(ip) {
			return fmt.Errorf("%w: private IP %s", ErrLocalhostBlocked, address)
		}
	}

	return nil
}
