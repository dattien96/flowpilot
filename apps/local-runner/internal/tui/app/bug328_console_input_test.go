package app

import (
	"strings"
	"testing"
)

func TestBug328_MouseTrackingOffANSI(t *testing.T) {
	if !strings.Contains(mouseTrackingOffANSI, "?1002l") {
		t.Fatal("must disable cell-motion mouse mode")
	}
	if !strings.Contains(mouseTrackingOffANSI, "?1000l") {
		t.Fatal("must disable basic mouse tracking")
	}
}

func TestBug328_EnsureConsoleInputReady_DoesNotPanic(t *testing.T) {
	ensureConsoleInputReady()
}
