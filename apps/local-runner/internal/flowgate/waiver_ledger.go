package flowgate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultWaiverTTL     = 14 * 24 * time.Hour
	waiverLedgerFile     = "settings/waiver_ledger.json"
)

// WaiverEntry is durable debt for an accepted test override (CP-53 P-4 / Task-276).
type WaiverEntry struct {
	TestName   string `json:"test_name"`
	Reason     string `json:"reason"`
	AcceptedAt string `json:"accepted_at"`
	ExpiresAt  string `json:"expires_at"`
	RunID      string `json:"run_id,omitempty"`
	Actor      string `json:"actor,omitempty"`
}

// LoadWaiverLedger reads .flowpilot/settings/waiver_ledger.json.
func LoadWaiverLedger(dotFP string) ([]WaiverEntry, error) {
	data, err := os.ReadFile(filepath.Join(dotFP, waiverLedgerFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var entries []WaiverEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func saveWaiverLedger(dotFP string, entries []WaiverEntry) error {
	dir := filepath.Join(dotFP, "settings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "waiver_ledger.json"), data, 0o644)
}

// AppendWaiverLedger records a new waiver entry (newest first).
func AppendWaiverLedger(dotFP string, entry WaiverEntry) error {
	entries, err := LoadWaiverLedger(dotFP)
	if err != nil {
		return err
	}
	entries = append([]WaiverEntry{entry}, entries...)
	return saveWaiverLedger(dotFP, entries)
}

// ListOpenWaivers returns non-expired ledger entries.
func ListOpenWaivers(dotFP string) ([]WaiverEntry, error) {
	entries, err := LoadWaiverLedger(dotFP)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var open []WaiverEntry
	for _, e := range entries {
		if e.TestName == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, e.ExpiresAt); err == nil && !t.After(now) {
			continue
		}
		open = append(open, e)
	}
	return open, nil
}

func overrideExpired(o Override, now time.Time) bool {
	if strings.TrimSpace(o.ExpiresAt) == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, o.ExpiresAt)
	if err != nil {
		return false
	}
	return !t.After(now)
}

// PruneExpiredOverrides removes expired overrides from disk and returns active map.
func PruneExpiredOverrides(dotFP string) (map[string]Override, error) {
	return LoadOverrides(dotFP)
}

// SaveOverrideWithReason records override + waiver ledger entry. Reason required.
func SaveOverrideWithReason(dotFP string, o Override, reason, runID, actor string, ttl time.Duration) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("waiver reason is required")
	}
	if ttl <= 0 {
		ttl = DefaultWaiverTTL
	}
	now := time.Now().UTC()
	o.AgreedAt = now.Format(time.RFC3339)
	o.HumanConfirm = true
	o.Reason = reason
	o.ExpiresAt = now.Add(ttl).Format(time.RFC3339)
	o.RunID = runID
	o.Actor = actor

	overrides, err := LoadOverrides(dotFP)
	if err != nil {
		overrides = map[string]Override{}
	}
	overrides[o.TestName] = o
	if err := writeOverrides(dotFP, overrides); err != nil {
		return err
	}
	return AppendWaiverLedger(dotFP, WaiverEntry{
		TestName:   o.TestName,
		Reason:     reason,
		AcceptedAt: o.AgreedAt,
		ExpiresAt:  o.ExpiresAt,
		RunID:      runID,
		Actor:      actor,
	})
}
