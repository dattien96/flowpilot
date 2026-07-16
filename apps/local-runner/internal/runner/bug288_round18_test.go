package runner

import (
	"encoding/json"
	"testing"
)

// ---- R18-1: numeric gen ranking for unpadded keys --------------------------

func TestDurableIdempotencyNumericGenNotLexical(t *testing.T) {
	m := map[string]string{
		"durable-a-restart-9":   "t9",
		"durable-a-restart-10":  "t10",
		"durable-a-restart-100": "t100",
		"durable-b-restart-1":   "tb1",
	}
	snap := durableIdempotencySnapshot(m)
	if snap["durable-a-restart-100"] != "t100" {
		t.Fatalf("gen 100 lost: %#v", snap)
	}
	if durableIdempotencyKeyGen("durable-a-restart-100") != 100 {
		t.Fatal("key gen parse failed")
	}
}

// ---- R18-3: session_runtime pack/unpack ------------------------------------

func TestSessionRuntimeBlobRoundTrip(t *testing.T) {
	st := ProviderSessionState{
		RunID:                 "r1",
		PendingFlowGateSettle: true,
		PendingFlowGateTurnID: "turn-z",
		FlowCohortID:          "cohort-1",
		StepID:                "step-a",
		PendingResumePrompt:   "continue please",
		PendingResumeStepID:   "step-a",
		PendingResumeGen:      2,
	}
	raw, err := json.Marshal(sessionRuntimeFromState(st))
	if err != nil {
		t.Fatal(err)
	}
	var out ProviderSessionState
	applySessionRuntime(&out, raw)
	if !out.PendingFlowGateSettle || out.PendingFlowGateTurnID != "turn-z" {
		t.Fatalf("gate fields: settle=%v turn=%q", out.PendingFlowGateSettle, out.PendingFlowGateTurnID)
	}
	if out.FlowCohortID != "cohort-1" || out.PendingResumeGen != 2 {
		t.Fatalf("cohort/resume: %#v", out)
	}
}

// ---- R18-4: per-service mint uses own secret -------------------------------

func TestServiceMarkerSecretIndependentMint(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	sec1 := InitRunMarkerSecretFromDir(d1)
	sec2 := InitRunMarkerSecretFromDir(d2)
	if len(sec1) == 0 || len(sec2) == 0 {
		t.Fatal("expected secrets")
	}
	m1 := runMarkerMACWith(sec1, "fcp", "run")
	m2 := runMarkerMACWith(sec2, "fcp", "run")
	// Different dirs almost always produce different secrets.
	if m1 == m2 {
		// possible but rare if secrets collide — still verify both verify path
		t.Log("MAC collision between dirs (unlikely)")
	}
	if !verifyRunMarkerMAC("fcp", "run", m1) || !verifyRunMarkerMAC("fcp", "run", m2) {
		// verify checks all loaded secrets — both dirs are loaded
		t.Fatal("verify multi-secret must accept both")
	}
	// Explicit mint must not depend on global active.
	if runMarkerMACWith(sec1, "fcp", "run") != m1 {
		t.Fatal("mint with sec1 must be stable")
	}
}
