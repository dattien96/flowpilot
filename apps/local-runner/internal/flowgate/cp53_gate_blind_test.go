package flowgate

import (
	"strings"
	"testing"
)

func TestClassifyGateBlindMissingBaseline(t *testing.T) {
	if got := ClassifyGateBlind(nil, OracleResult{}, ""); got != GateBlindMissingBaseline {
		t.Fatalf("got %q want missing_baseline", got)
	}
}

func TestClassifyGateBlindEnvError(t *testing.T) {
	bl := &Baseline{SuitePassed: true}
	if got := ClassifyGateBlind(bl, OracleResult{EnvError: "boom"}, ""); got != GateBlindEnvError {
		t.Fatalf("got %q want env_error", got)
	}
}

func TestClassifyGateBlindRedAtCapture(t *testing.T) {
	bl := &Baseline{SuitePassed: false, GreenTests: []string{"TestA"}}
	if got := ClassifyGateBlind(bl, OracleResult{}, ""); got != GateBlindRedAtCapture {
		t.Fatalf("got %q want red_at_capture", got)
	}
}

func TestClassifyGateBlindClearWhenGreen(t *testing.T) {
	bl := &Baseline{SuitePassed: true}
	if got := ClassifyGateBlind(bl, OracleResult{}, ""); got != "" {
		t.Fatalf("got %q want empty", got)
	}
}

func TestGateBlindMessageFormats(t *testing.T) {
	if !strings.Contains(GateBlindMessage(GateBlindMissingBaseline), "missing_baseline") {
		t.Fatal("missing message")
	}
}
