package flowgate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckDogfoodSuiteMissingGoBaselineBlocks(t *testing.T) {
	repo := t.TempDir()
	dot := filepath.Join(repo, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(dot, "guard"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := CheckDogfoodSuite(context.Background(), repo, dot, GoBaselineFile, "go", false)
	if err == nil {
		t.Fatal("expected error when Go baseline missing")
	}
}

func TestCheckDogfoodSuiteMissingTSBaselineWarnOnly(t *testing.T) {
	repo := t.TempDir()
	dot := filepath.Join(repo, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(dot, "guard"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := CheckDogfoodSuite(context.Background(), repo, dot, TSBaselineFile, "ts", true)
	if err != nil {
		t.Fatalf("warn-only missing TS baseline: %v", err)
	}
	if !res.Skipped {
		t.Fatalf("expected skipped warn-only result, got %+v", res)
	}
}

func TestLoadSaveBaselineFileRoundTrip(t *testing.T) {
	repo := t.TempDir()
	dot := filepath.Join(repo, ".flowpilot")
	bl := &Baseline{
		CapturedAt:  "2026-08-11T00:00:00Z",
		GreenTests:  []string{"TestFoo"},
		TestCmd:     "go test ./...",
		TestDir:     "apps/local-runner",
		SuitePassed: true,
	}
	if err := SaveBaselineFile(dot, GoBaselineFile, bl); err != nil {
		t.Fatal(err)
	}
	got, err := LoadBaselineFile(dot, GoBaselineFile)
	if err != nil || got == nil || got.TestCmd != bl.TestCmd {
		t.Fatalf("round-trip: got=%#v err=%v", got, err)
	}
}
