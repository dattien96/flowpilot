package app

import "strings"

// tcpLocalAddrHasPort reports whether a netstat local-address field (IPv4 or
// IPv6) is bound to exactly wantPort. Suffix matching (" :4317") is wrong
// because 127.0.0.1:14317 also ends with :4317.
func tcpLocalAddrHasPort(localAddr, wantPort string) bool {
	port := strings.TrimSpace(wantPort)
	addr := strings.TrimSpace(localAddr)
	if port == "" || addr == "" {
		return false
	}
	if strings.HasPrefix(addr, "[") {
		idx := strings.LastIndex(addr, "]:")
		if idx < 0 {
			return false
		}
		return addr[idx+2:] == port
	}
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return false
	}
	return addr[idx+1:] == port
}
