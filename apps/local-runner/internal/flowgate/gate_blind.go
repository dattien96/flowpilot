package flowgate

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// GateBlindReason classifies why the regression oracle cannot run reliably (CP-53 P-1).
type GateBlindReason string

const (
	GateBlindMissingBaseline GateBlindReason = "missing_baseline"
	GateBlindEnvError        GateBlindReason = "env_error"
	GateBlindRedAtCapture    GateBlindReason = "red_at_capture"
)

// FlakyQuarantine lists test names excluded from coarse suite-failure blind when
// baseline was red at capture (operator-maintained).
type FlakyQuarantine struct {
	Tests []string `json:"tests,omitempty"`
}

const flakyQuarantineFile = "settings/flaky-quarantine.json"

// LoadFlakyQuarantine reads .flowpilot/settings/flaky-quarantine.json. Missing → empty.
func LoadFlakyQuarantine(dotFP string) FlakyQuarantine {
	if dotFP == "" {
		return FlakyQuarantine{}
	}
	data, err := os.ReadFile(filepath.Join(dotFP, flakyQuarantineFile))
	if err != nil {
		return FlakyQuarantine{}
	}
	var q FlakyQuarantine
	_ = json.Unmarshal(data, &q)
	return q
}

// ClassifyGateBlind returns a non-empty reason when the oracle is blind.
// Corrupt baseline is handled separately (BUG-288 fail-closed) — not classified here.
func ClassifyGateBlind(baseline *Baseline, oracle OracleResult, dotFP string) GateBlindReason {
	if oracle.EnvError != "" {
		return GateBlindEnvError
	}
	if baseline == nil {
		return GateBlindMissingBaseline
	}
	if !baseline.SuitePassed {
		// Red-at-capture: still blind at suite level unless operator quarantined all
		// failing signals (v1: no auto bypass from quarantine list — list is for
		// named-test oracle paths in later passes).
		return GateBlindRedAtCapture
	}
	return ""
}

// GateBlindMessage formats an operator-visible block/warn message.
func GateBlindMessage(reason GateBlindReason) string {
	switch reason {
	case GateBlindMissingBaseline:
		return "gate_blind (missing_baseline): no test baseline — regression checks disabled until baseline is captured"
	case GateBlindEnvError:
		return "gate_blind (env_error): test harness could not run — regression checks unavailable"
	case GateBlindRedAtCapture:
		return "gate_blind (red_at_capture): baseline was captured while suite was red — coarse regression signal unreliable"
	default:
		return "gate_blind: regression oracle unavailable"
	}
}

// BaselineHealthSummary returns human-readable baseline freshness hints (P-1 health).
func BaselineHealthSummary(baseline *Baseline) string {
	if baseline == nil {
		return "baseline: missing"
	}
	st := "green"
	if !baseline.SuitePassed {
		st = "red-at-capture"
	}
	if baseline.HeadSHA == "" {
		return "baseline: present (" + st + ", no head_sha — consider refresh)"
	}
	if baseline.Dirty {
		return "baseline: present (" + st + ", captured-dirty)"
	}
	return "baseline: present (" + st + ", head=" + baseline.HeadSHA[:min(8, len(baseline.HeadSHA))] + ")"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
