package app

import (
	"strings"
	"testing"
)

func TestTaskkillTreeArgs_IncludesTreeFlag(t *testing.T) {
	got := taskkillTreeArgs(30268)
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "/PID 30268") {
		t.Fatalf("missing pid: %v", got)
	}
	if !strings.Contains(joined, "/T") {
		t.Fatalf("taskkill without /T orphans grok/MCP children: %v", got)
	}
	if !strings.Contains(joined, "/F") {
		t.Fatalf("missing /F: %v", got)
	}
}
