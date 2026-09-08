package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
)

func TestBuiltinArm_VibeIngestOmitsBugSubMode(t *testing.T) {
	arm := builtinArm(client.BuiltinFlowOption{FlowRef: "vibe-ingest", Label: "vibe-ingest"})
	sub, fr, ct, _, ok := arm.FirstTurnExtras()
	if !ok || fr != "vibe-ingest" {
		t.Fatalf("ok=%v fr=%q", ok, fr)
	}
	if sub == "bug" || ct == "bugfix" {
		t.Fatalf("vibe arm must not send chat bug extras: sub=%q ct=%q", sub, ct)
	}
}

func TestBuiltinArm_ReviewLoopKeepsBugSubMode(t *testing.T) {
	arm := builtinArm(client.BuiltinFlowOption{FlowRef: "pack/review-loop", Label: "review-loop"})
	sub, _, ct, _, ok := arm.FirstTurnExtras()
	if !ok || sub != "bug" || ct != "bugfix" {
		t.Fatalf("review-loop extras sub=%q ct=%q ok=%v", sub, ct, ok)
	}
}
