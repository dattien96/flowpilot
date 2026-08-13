package app

import "testing"

func TestTcpLocalAddrHasPort_ExactMatchOnly(t *testing.T) {
	if !tcpLocalAddrHasPort("127.0.0.1:4317", "4317") {
		t.Fatal("expected IPv4 exact port match")
	}
	if tcpLocalAddrHasPort("127.0.0.1:14317", "4317") {
		t.Fatal("14317 must not match port 4317")
	}
	if !tcpLocalAddrHasPort("[::1]:4317", "4317") {
		t.Fatal("expected IPv6 exact port match")
	}
	if tcpLocalAddrHasPort("[::1]:43170", "4317") {
		t.Fatal("43170 must not match port 4317")
	}
}
