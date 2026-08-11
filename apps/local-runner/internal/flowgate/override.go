package flowgate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Override records that the user explicitly agreed to allow a specific test to change
// as part of an opt-2 requirement-change flow (Task-155). Persisted under
// .flowpilot/guard/test_overrides.json, machine-local, never Drive-synced (CP-35 §4.8).
type Override struct {
	TestName     string `json:"test_name"`
	AgreedAt     string `json:"agreed_at"`
	HumanConfirm bool   `json:"human_confirm"`
	// CP-53 P-4 / Task-276: time-bounded waiver debt.
	Reason    string `json:"reason,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Actor     string `json:"actor,omitempty"`
}

// LoadOverrides reads the per-project override store. Missing file → empty map (non-fatal).
// Expired waivers are pruned on load (CP-53 P-4 re-arm).
func LoadOverrides(dotFP string) (map[string]Override, error) {
	overrides, err := readOverridesFile(dotFP)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	changed := false
	for name, o := range overrides {
		if overrideExpired(o, now) {
			delete(overrides, name)
			changed = true
		}
	}
	if changed {
		_ = writeOverrides(dotFP, overrides)
	}
	return overrides, nil
}

func readOverridesFile(dotFP string) (map[string]Override, error) {
	data, err := os.ReadFile(filepath.Join(dotFP, "guard", "test_overrides.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]Override{}, nil
		}
		return nil, err
	}
	var overrides map[string]Override
	if err := json.Unmarshal(data, &overrides); err != nil {
		return map[string]Override{}, nil
	}
	return overrides, nil
}

// SaveOverride records an agreed test override. Called only on explicit user agreement
// inside the opt-2 propose→agree sub-flow. Never called for opt-1 or opt-3. (Task-155)
// Prefer SaveOverrideWithReason for CP-53 waiver ledger entries with expiry.
func SaveOverride(dotFP string, o Override) error {
	return SaveOverrideWithReason(dotFP, o, "legacy override (no reason recorded)", "", "", DefaultWaiverTTL)
}

// ClearOverride removes the override for a specific test (used in tests or manual reset).
func ClearOverride(dotFP, testName string) error {
	overrides, err := LoadOverrides(dotFP)
	if err != nil || len(overrides) == 0 {
		return nil
	}
	delete(overrides, testName)
	return writeOverrides(dotFP, overrides)
}

// ClearOverrideIfGreen removes overrides for tests that are now green — the block
// was resolved, so the sticky override is no longer needed. (Task-155 DOD-07)
func ClearOverrideIfGreen(dotFP string, greenTests []string) error {
	if len(greenTests) == 0 {
		return nil
	}
	overrides, err := LoadOverrides(dotFP)
	if err != nil || len(overrides) == 0 {
		return nil
	}
	green := make(map[string]bool, len(greenTests))
	for _, t := range greenTests {
		green[t] = true
	}
	changed := false
	for name := range overrides {
		if green[name] {
			delete(overrides, name)
			changed = true
		}
	}
	if changed {
		return writeOverrides(dotFP, overrides)
	}
	return nil
}

// IsOverridden reports whether testName has an active human-confirmed override.
func IsOverridden(overrides map[string]Override, testName string) bool {
	if overrides == nil || testName == "" {
		return false
	}
	_, ok := overrides[testName]
	if !ok {
		return false
	}
	return !overrideExpired(overrides[testName], time.Now().UTC())
}

func writeOverrides(dotFP string, overrides map[string]Override) error {
	guardDir := filepath.Join(dotFP, "guard")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(overrides, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(guardDir, "test_overrides.json"), data, 0o644)
}
