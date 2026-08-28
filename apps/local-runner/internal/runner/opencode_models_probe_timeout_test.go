package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

// CA-657 / CP-57: a hanging `opencode models` (or the old `--format json` probe)
// must not stall GET /providers past the TUI 8s budget.

func TestDetectOpencodeModelsTimesOutInsteadOfStallingProviders(t *testing.T) {
	orig := runCommandFn
	t.Cleanup(func() { runCommandFn = orig })
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "--format" {
				t.Errorf("detect path must not spawn opencode models --format json, args=%v", args)
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return []byte("opencode/too-late"), nil
		}
	}

	start := time.Now()
	_, err := detectOpencodeModels(context.Background())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error from capped opencode models probe")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("opencode models probe stalled %v; want <= 2s (cap 1.2s)", elapsed)
	}
}

func TestResolveProviderModelsFallsBackWhenOpencodeProbeTimesOut(t *testing.T) {
	orig := runCommandFn
	t.Cleanup(func() { runCommandFn = orig })
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return []byte("opencode/too-late"), nil
		}
	}

	spec := providerSpec{Key: "opencode", Models: defaultOpencodeProviderModels()}
	start := time.Now()
	models := resolveProviderModels(context.Background(), spec, "opencode")
	if time.Since(start) > 2*time.Second {
		t.Fatalf("resolveProviderModels stalled %v on opencode probe timeout", time.Since(start))
	}
	want := defaultOpencodeProviderModels()
	if len(models) != len(want) {
		t.Fatalf("expected static fallback %d models, got %d: %+v", len(want), len(models), models)
	}
	if models[0].ID != "opencode/muse-spark-1.2-contributor-free" {
		t.Fatalf("expected muse-spark-free fallback, got %q", models[0].ID)
	}
}

func TestDetectOpencodeModelsParsesFastTextCatalog(t *testing.T) {
	orig := runCommandFn
	t.Cleanup(func() { runCommandFn = orig })
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if len(args) != 1 || args[0] != "models" {
			t.Fatalf("expected single `models` arg, got %v", args)
		}
		return []byte("opencode/muse-spark-1.2-contributor-free\nopencode/deepseek-v4-flash-free\nopencode/gpt-5.4-nano\n"), nil
	}

	models, err := detectOpencodeModels(context.Background())
	if err != nil {
		t.Fatalf("detectOpencodeModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models (broken default skipped), got %d: %+v", len(models), models)
	}
	joined := models[0].ID + " " + models[1].ID
	if !strings.Contains(joined, "opencode/gpt-5.4-nano") || !strings.Contains(joined, "muse-spark") {
		t.Fatalf("unexpected catalog: %s", joined)
	}
	for _, m := range models {
		if m.ID == "opencode/deepseek-v4-flash-free" {
			t.Fatal("broken default model must be skipped")
		}
	}
}

func TestDetectOpencodeModelsDoesNotTouchOtherProviderProbes(t *testing.T) {
	// Provider-agnostic guard: this helper is opencode-only. Codex/Claude/Grok
	// resolveProviderModels branches are not entered.
	orig := runCommandFn
	t.Cleanup(func() { runCommandFn = orig })
	called := 0
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		called++
		return []byte("opencode/muse-spark-1.2-contributor-free\n"), nil
	}
	_ = resolveProviderModels(context.Background(), providerSpec{Key: "codex", Models: []ProviderModel{{ID: "gpt-5.4"}}}, "codex")
	called = 0
	_ = resolveProviderModels(context.Background(), providerSpec{Key: "grok", Models: []ProviderModel{{ID: "grok-4.5"}}}, "grok")
	if called != 0 {
		t.Fatalf("grok model resolve must not spawn CLI, spawned %d times", called)
	}
}
