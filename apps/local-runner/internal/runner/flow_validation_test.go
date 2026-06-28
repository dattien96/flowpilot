package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidationRetryPromptIncludesOriginalContextPackage(t *testing.T) {
	pkg := FlowContextPackage{
		PackageID:         "pkg-1",
		WorkflowRunID:     "run-1",
		PlanStepRunID:     "plan-1",
		FeatureKey:        "context-regression-engine",
		FeatureConfidence: FlowContextConfidenceVerified,
		Sections: []FlowContextSection{
			{Title: "History", Body: "## Prior work\n- entry"},
		},
	}
	result := FlowValidationResult{
		ValidationCommand: "go test ./...",
		StdoutSummary:     "ok",
		StderrSummary:     "compiler error: cannot build",
		ExitCode:          1,
	}
	prompt := buildValidationRetryPrompt(pkg, result)
	if strings.Count(prompt, "## Flow Context Package") != 1 {
		t.Fatalf("retry prompt should include original package once: %q", prompt)
	}
	if !strings.Contains(prompt, "Validation Feedback") || !strings.Contains(prompt, "compiler error") {
		t.Fatalf("retry prompt missing validation feedback: %q", prompt)
	}
	if !strings.Contains(prompt, "Keep the original Flow Context Package unchanged.") {
		t.Fatalf("retry prompt missing no-mutation instruction: %q", prompt)
	}
}

func TestSummarizeFlowValidationOutputIsBounded(t *testing.T) {
	text := strings.Repeat("line\n", 40)
	summary := summarizeFlowValidationOutput("go test ./...", text, text, 1)
	if strings.Count(summary, "line") > 25 {
		t.Fatalf("summary not bounded: %q", summary)
	}
}

func TestLoadFlowModeConfigDefaultsWhenMissing(t *testing.T) {
	dotFP := filepath.Join(t.TempDir(), ".flowpilot")
	cfg, err := loadFlowModeConfig(dotFP)
	if err != nil {
		t.Fatalf("loadFlowModeConfig: %v", err)
	}
	if cfg.MaxRetries != 3 {
		t.Fatalf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.MaxContextPackageBytes != flowContextDefaultMaxBytes {
		t.Fatalf("MaxContextPackageBytes = %d, want %d", cfg.MaxContextPackageBytes, flowContextDefaultMaxBytes)
	}
	if err := os.MkdirAll(filepath.Join(dotFP, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotFP, "settings", flowModeConfigFileName), []byte(`{"validationCommand":"go test ./...","maxRetries":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadFlowModeConfig(dotFP)
	if err != nil {
		t.Fatalf("loadFlowModeConfig after write: %v", err)
	}
	if cfg.ValidationCommand != "go test ./..." || cfg.MaxRetries != 3 {
		t.Fatalf("unexpected cfg after write: %+v", cfg)
	}
}
