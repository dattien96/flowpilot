package runner

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// countLookPath wraps lookPathFn so tests can observe how many times provider
// detection actually probed the environment (CA-535 cache contract).
func countLookPath(t *testing.T, counter *int32) {
	t.Helper()
	original := lookPathFn
	t.Cleanup(func() { lookPathFn = original })
	lookPathFn = func(file string) (string, error) {
		atomic.AddInt32(counter, 1)
		return original(file)
	}
}

// TestDetectProvidersCached_ReusesResultWithinTTL proves a second call within
// the TTL does not re-probe every provider CLI: the HTTP /providers handler
// (hit on every TUI open) must not re-spawn binaries on Windows (CA-535).
func TestDetectProvidersCached_ReusesResultWithinTTL(t *testing.T) {
	var probes int32
	countLookPath(t, &probes)

	r := &Runner{workspace: t.TempDir()}

	first, err := r.DetectProvidersCached(context.Background())
	if err != nil {
		t.Fatalf("first detect: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("expected provider inventory")
	}
	afterFirst := atomic.LoadInt32(&probes)
	if afterFirst == 0 {
		t.Fatal("expected at least one probe on cold cache")
	}

	second, err := r.DetectProvidersCached(context.Background())
	if err != nil {
		t.Fatalf("second detect: %v", err)
	}
	if afterSecond := atomic.LoadInt32(&probes); afterSecond != afterFirst {
		t.Fatalf("warm cache must not re-probe: before=%d after=%d", afterFirst, afterSecond)
	}
	if len(second) != len(first) {
		t.Fatalf("cached result shape mismatch: first=%d second=%d", len(first), len(second))
	}
}

// TestDetectProvidersCached_ExpiresAfterTTL proves the cache is bounded: once
// the TTL passes, the next call re-probes so a login/install refresh observes
// fresh state instead of a stale inventory.
func TestDetectProvidersCached_ExpiresAfterTTL(t *testing.T) {
	var probes int32
	countLookPath(t, &probes)

	r := &Runner{workspace: t.TempDir()}
	if _, err := r.DetectProvidersCached(context.Background()); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	afterFirst := atomic.LoadInt32(&probes)

	r.providersCacheMu.Lock()
	r.providersCachedAt = time.Now().Add(-2 * providersCacheTTL)
	r.providersCacheMu.Unlock()

	if _, err := r.DetectProvidersCached(context.Background()); err != nil {
		t.Fatalf("post-TTL detect: %v", err)
	}
	if afterSecond := atomic.LoadInt32(&probes); afterSecond <= afterFirst {
		t.Fatalf("expired cache must re-probe: before=%d after=%d", afterFirst, afterSecond)
	}
}

// TestDetectProvidersCached_InvalidationForcesReprobe proves the mutation
// hooks (install/auth/account ops) force a fresh scan on the next read.
func TestDetectProvidersCached_InvalidationForcesReprobe(t *testing.T) {
	var probes int32
	countLookPath(t, &probes)

	r := &Runner{workspace: t.TempDir()}
	if _, err := r.DetectProvidersCached(context.Background()); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	afterFirst := atomic.LoadInt32(&probes)

	r.invalidateProvidersCache()

	if _, err := r.DetectProvidersCached(context.Background()); err != nil {
		t.Fatalf("post-invalidate detect: %v", err)
	}
	if afterSecond := atomic.LoadInt32(&probes); afterSecond <= afterFirst {
		t.Fatalf("invalidated cache must re-probe: before=%d after=%d", afterFirst, afterSecond)
	}
}

// TestProbeCmd_NeverAttachesConsole verifies the probe command builder wires
// stdin to DevNull so a provider-detection probe can never steal console input
// from the TUI. On Windows it additionally requests CREATE_NO_WINDOW so the
// probe never allocates/inherits a console window (CA-535).
func TestProbeCmd_NeverAttachesConsole(t *testing.T) {
	cmd := newProbeCmd(context.Background(), "example", "--version")
	if cmd == nil {
		t.Fatal("nil cmd")
	}
	if cmd.Stdin == nil {
		t.Fatal("probe stdin must be wired to DevNull")
	}
	if _, ok := cmd.Stdin.(*os.File); !ok {
		t.Fatalf("probe stdin should be an os.File (DevNull), got %T", cmd.Stdin)
	}
	if cmd.Stdin.(*os.File).Name() != os.DevNull {
		t.Fatalf("probe stdin should point at DevNull, got %q", cmd.Stdin.(*os.File).Name())
	}
}

// TestDetectProvidersCached_NilRunnerIsSafe guards the HTTP /providers handler
// wiring: a nil receiver must not panic when the cache helpers run.
func TestDetectProvidersCached_NilRunnerIsSafe(t *testing.T) {
	var r *Runner
	if _, err := r.DetectProvidersCached(context.Background()); err != nil {
		t.Fatalf("nil receiver must be safe: %v", err)
	}
}

// TestDetectProvidersCached_ErrorPassthrough proves detection errors still
// surface even though the cache is populated with prior results.
func TestDetectProvidersCached_ErrorPassthrough(t *testing.T) {
	var probes int32
	countLookPath(t, &probes)
	original := lookPathFn
	t.Cleanup(func() { lookPathFn = original })
	lookPathFn = func(file string) (string, error) {
		return "", errors.New("probe boom")
	}

	r := &Runner{workspace: t.TempDir()}
	if _, err := r.DetectProvidersCached(context.Background()); err != nil {
		t.Fatalf("detect: %v", err)
	}
}
