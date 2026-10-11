package core

import "net"

// isIPv4 reports whether s is a literal IPv4 address.
func isIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}
