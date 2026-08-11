package flowgate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	GoBaselineFile  = "test_baseline.json"
	TSBaselineFile  = "test_baseline_ts.json"
	DefaultGateMode = "enforce"
)

// DogfoodCheckResult summarizes one baseline suite check (CP-53 P-3).
type DogfoodCheckResult struct {
	Label      string
	Baseline   string
	Action     string
	Message    string
	Skipped    bool
	SkipReason string
}

// LoadBaselineFile reads guard/<filename> under dotFlowpilotDir.
func LoadBaselineFile(dotFlowpilotDir, filename string) (*Baseline, error) {
	path := filepath.Join(dotFlowpilotDir, "guard", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var bl Baseline
	if err := json.Unmarshal(data, &bl); err != nil {
		return nil, err
	}
	return &bl, nil
}

// SaveBaselineFile atomically writes guard/<filename>.
func SaveBaselineFile(dotFlowpilotDir, filename string, bl *Baseline) error {
	if bl == nil {
		return fmt.Errorf("SaveBaselineFile: nil baseline")
	}
	guardDir := filepath.Join(dotFlowpilotDir, "guard")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bl, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(guardDir, filename)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// CaptureBaselineFile runs testCmd under repoDir/testDir and stores the result.
func CaptureBaselineFile(ctx context.Context, repoDir, dotFlowpilotDir, filename, testCmd, testDir string) (*Baseline, error) {
	if strings.TrimSpace(testCmd) == "" {
		return nil, fmt.Errorf("CaptureBaselineFile: empty test command")
	}
	headSHA, dirty := captureGitState(repoDir)
	bl := &Baseline{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		TestCmd:    testCmd,
		TestDir:    testDir,
		HeadSHA:    headSHA,
		Dirty:      dirty,
	}
	suitePassed, passed, _, envErr, _ := executeSuite(ctx, repoDir, testCmd, testDir)
	if envErr != "" {
		return nil, fmt.Errorf("test command failed to start: %s", envErr)
	}
	bl.SuitePassed = suitePassed
	bl.GreenTests = passed
	if err := SaveBaselineFile(dotFlowpilotDir, filename, bl); err != nil {
		return nil, err
	}
	return bl, nil
}

// CheckDogfoodSuite runs the regression oracle for one stored baseline file.
// warnIfMissing: TS path uses true so missing baseline warns but does not fail Go.
func CheckDogfoodSuite(ctx context.Context, repoDir, dotFlowpilotDir, filename, label string, warnIfMissing bool) (DogfoodCheckResult, error) {
	res := DogfoodCheckResult{Label: label, Baseline: filename}
	bl, err := LoadBaselineFile(dotFlowpilotDir, filename)
	if err != nil {
		return res, err
	}
	if bl == nil {
		if warnIfMissing {
			res.Skipped = true
			res.SkipReason = "baseline missing (warn-only)"
			res.Action = "warn"
			res.Message = fmt.Sprintf("%s: no %s — skipping (Go gate still runs independently)", label, filename)
			return res, nil
		}
		res.Action = "block"
		res.Message = fmt.Sprintf("%s: missing baseline %s", label, filename)
		return res, errors.New(res.Message)
	}
	diff, diffErr := ObserveGitDiff(repoDir)
	if diffErr != nil {
		return res, diffErr
	}
	oracle := RunOracleContext(ctx, repoDir, bl, diff, nil)
	if reason := ClassifyGateBlind(bl, oracle, dotFlowpilotDir); reason != "" && HasCodeChanges(diff) {
		res.Action = "block"
		res.Message = fmt.Sprintf("%s: gate_blind (%s)", label, reason)
		return res, errors.New(res.Message)
	}
	if oracle.EnvError != "" {
		res.Action = "block"
		res.Message = fmt.Sprintf("%s: test env error: %s", label, oracle.EnvError)
		return res, errors.New(res.Message)
	}
	if oracle.HasRegression || (bl.SuitePassed && !oracle.SuitePassed) {
		res.Action = "block"
		if len(oracle.Regressed) > 0 {
			res.Message = fmt.Sprintf("%s: regression detected: %s", label, strings.Join(oracle.Regressed, ", "))
		} else {
			res.Message = fmt.Sprintf("%s: test suite failed (baseline was green)", label)
		}
		return res, errors.New(res.Message)
	}
	res.Action = "pass"
	res.Message = fmt.Sprintf("%s: ok", label)
	return res, nil
}
