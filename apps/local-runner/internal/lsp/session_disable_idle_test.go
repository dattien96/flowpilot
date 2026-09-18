package lsp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A check in another workspace may reap a crashed server before its next
// CheckFiles call. Reaping must not discard the exhausted crash budget.
func TestSessionDisableSurvivesIdleCleanup(t *testing.T) {
	set := NewServerSet(nil)
	root := t.TempDir()
	cfg := PlatformLSPConfig{Platform: "golang", Binary: filepath.Join(root, "missing-lsp")}
	key := serverKey{root: root, platform: cfg.Platform}
	set.servers[key] = &Server{
		Root: root, Config: cfg,
		Manager:  &ServerManager{disabled: true, restarts: maxAutoRestarts},
		lastUsed: time.Now().Add(-time.Hour),
	}
	t.Cleanup(set.Close)

	set.stopIdle(time.Minute)
	if set.ServerCount() != 0 {
		t.Fatal("idle server was not removed")
	}
	if !set.disabled[key] {
		t.Fatal("idle cleanup discarded the exhausted session crash budget")
	}
	for i := 0; i < 2; i++ {
		server, err := set.getOrStart(context.Background(), root, cfg)
		if server != nil || err == nil || !strings.Contains(err.Error(), "disabled for this session") {
			t.Fatalf("check %d: server=%v err=%v; want session-disabled before binary lookup", i, server, err)
		}
	}
}
