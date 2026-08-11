package changecontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// CanonicalHead status values (SD-21 §5 state machine).
const (
	HeadStatusSpecLess    = "spec_less"
	HeadStatusCurrent     = "current"
	HeadStatusSpecDrifted = "spec_drifted"
	HeadStatusCodeDrifted = "code_drifted"
	HeadStatusRenamed     = "renamed"
	HeadStatusMerged      = "merged"
	HeadStatusDeprecated  = "deprecated"
)

// SpecConfidence values.
const (
	SpecConfidenceSpecBacked = "spec_backed"
	SpecConfidenceSpecLess   = "spec_less"
)

// Decision (SD-21 §5, embedded in CanonicalHead.Decisions) preserves negative
// knowledge: an approach that was tried and its outcome, so a later turn does
// not silently re-attempt a known dead end (Task-187 folds these in).
type Decision struct {
	Tried        string     `json:"tried"`
	Outcome      string     `json:"outcome"` // adopted | rejected | reverted
	Reason       string     `json:"reason"`
	SupersededBy string     `json:"superseded_by,omitempty"`
	SourceDocID  string     `json:"source_doc_id,omitempty"`
	At           *time.Time `json:"at,omitempty"`
}

// Decision.Outcome values.
const (
	DecisionAdopted  = "adopted"
	DecisionRejected = "rejected"
	DecisionReverted = "reverted"
)

// CanonicalHead is the single authoritative statement of a feature's current
// intent (SD-21 §5): one per feature_key, Drive-synced (unlike per-turn
// Contracts, which stay local). It carries a reproducible IntentSignature
// over the governing spec + declared behavior — never over code bytes.
type CanonicalHead struct {
	FeatureKey         string            `json:"feature_key"`
	BehaviorStatement  string            `json:"behavior_statement"`
	GoverningDocIDs    []string          `json:"governing_doc_ids,omitempty"`
	GoverningDocHashes map[string]string `json:"governing_doc_hashes,omitempty"`
	IntentSignature    string            `json:"intent_signature"`
	HeadCommit         string            `json:"head_commit,omitempty"`
	Decisions          []Decision        `json:"decisions,omitempty"`
	UpdatedAt          time.Time         `json:"updated_at"`
	Status             string            `json:"status"`
	SpecConfidence     string            `json:"spec_confidence"`
	// SupersededBy targets a rename/merge points to (Task-187 RetireHead).
	SupersededBy []string   `json:"superseded_by,omitempty"`
	RetiredAt    *time.Time `json:"retired_at,omitempty"`
}

func headFilePath(workspace, featureKey string) string {
	return filepath.Join(workspace, ".flowpilot", "canonical", featureKey+".json")
}

// LoadHead reads the stored Head for featureKey. ok=false (no error) when no
// Head file exists yet — callers should then mint one via BuildHead.
//
// BUG-289 H3/F-3: corrupt JSON (torn SaveHead, Drive-sync conflict) returns
// ok=false with nil error so callers can rebuild rather than hard-fail the
// whole gate into a silent permanent block. Non-corrupt IO errors still fail.
func LoadHead(workspace, featureKey string) (CanonicalHead, bool, error) {
	data, err := os.ReadFile(headFilePath(workspace, featureKey))
	if err != nil {
		if os.IsNotExist(err) {
			return CanonicalHead{}, false, nil
		}
		return CanonicalHead{}, false, err
	}
	var h CanonicalHead
	if err := json.Unmarshal(data, &h); err != nil {
		// Corrupt / partial file — treat as missing so updateCanonicalHead can
		// rebuild from the new contract rather than fail-closed forever.
		return CanonicalHead{}, false, nil
	}
	return h, true, nil
}

// SaveHead persists h to <workspace>/.flowpilot/canonical/<feature_key>.json,
// creating the directory if needed. One file per feature — Drive-synced via
// contextsync (Task-188), unlike Contract's local-only contracts.ndjson.
//
// BUG-289 H3/F-3: write via temp + Rename for crash atomicity (mirror
// flow_context_handoff.go run_marker_secret path).
func SaveHead(workspace string, h CanonicalHead) error {
	tmp, final, err := StageHeadWrite(workspace, h)
	if err != nil {
		return err
	}
	return CommitHeadWrite(tmp, final)
}

// StageHeadWrite marshals h and writes it to a temp file beside its target
// Head path, without making it visible at finalPath yet — the first half of
// a two-phase commit across multiple features' Heads, so one feature's write
// failure (disk full, permission denied) can never leave a sibling feature's
// real Head file already mutated (CP-55 P-5 review finding C-2: finalizing a
// Flow's several staged Heads one at a time, writing each as it goes, let an
// earlier feature's Head become permanently live even though the Flow's
// "done" transition as a whole was refused because a later feature failed).
// Call CommitHeadWrite once every Head in the batch has staged successfully,
// or DiscardHeadWrite to abandon this one tmp file without committing it.
func StageHeadWrite(workspace string, h CanonicalHead) (tmpPath, finalPath string, err error) {
	finalPath = headFilePath(workspace, h.FeatureKey)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return "", "", err
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return "", "", err
	}
	tmpPath = finalPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return "", "", err
	}
	return tmpPath, finalPath, nil
}

// CommitHeadWrite makes a StageHeadWrite'd tmp file visible at finalPath.
func CommitHeadWrite(tmpPath, finalPath string) error {
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// DiscardHeadWrite removes a StageHeadWrite'd tmp file without committing it.
func DiscardHeadWrite(tmpPath string) {
	_ = os.Remove(tmpPath)
}
