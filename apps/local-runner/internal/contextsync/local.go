package contextsync

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	LedgerFile            = "ledger/feature_history.ndjson"
	ChatSummaryFile       = "ledger/chat_summary.ndjson"
	CatalogFile           = "catalog/features.ndjson"
	FlowRulesFile         = "settings/flow-rules.json"
	ApprovalAllowlistFile = "settings/approval-allowlist.json"
)

type EngineStore struct {
	DotFlowpilotDir string
}

func NewEngineStore(dotFlowpilotDir string) (*EngineStore, error) {
	subdirs := []string{"ledger", "catalog", "settings", "guard", "structure", "canonical"}
	for _, sub := range subdirs {
		if err := os.MkdirAll(filepath.Join(dotFlowpilotDir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	return &EngineStore{DotFlowpilotDir: dotFlowpilotDir}, nil
}

func (s *EngineStore) LedgerPath() string {
	return filepath.Join(s.DotFlowpilotDir, LedgerFile)
}

func (s *EngineStore) ChatSummaryPath() string {
	return filepath.Join(s.DotFlowpilotDir, ChatSummaryFile)
}

func (s *EngineStore) CatalogPath() string {
	return filepath.Join(s.DotFlowpilotDir, CatalogFile)
}

func (s *EngineStore) FlowRulesPath() string {
	return filepath.Join(s.DotFlowpilotDir, FlowRulesFile)
}

func (s *EngineStore) ApprovalAllowlistPath() string {
	return filepath.Join(s.DotFlowpilotDir, ApprovalAllowlistFile)
}

func (s *EngineStore) GuardDir() string {
	return filepath.Join(s.DotFlowpilotDir, "guard")
}

func (s *EngineStore) ToolingPath() string {
	return filepath.Join(s.DotFlowpilotDir, "tooling.json")
}

// CanonicalDir is where Task-186's per-feature Canonical Head JSON files live
// (`.flowpilot/canonical/<feature_key>.json`). Unlike the other shared files,
// this is an unbounded, dynamically-growing set — one file per feature — so
// SharedFiles globs it rather than naming individual paths.
func (s *EngineStore) CanonicalDir() string {
	return filepath.Join(s.DotFlowpilotDir, "canonical")
}

// SharedFiles returns every file the CP-35 P-8 Drive sync mechanism uploads
// to context-engine/. Task-188 (CP-43 P-5) adds every `canonical/*.json`
// Canonical Head to this set; `contracts/contracts.ndjson` (Task-184's local,
// per-turn Change Contract store) is deliberately never listed here — it
// lives under its own `contracts/` directory, not `canonical/`, and this
// function only ever globs the latter.
func (s *EngineStore) SharedFiles() []string {
	files := []string{s.LedgerPath(), s.ChatSummaryPath(), s.CatalogPath(), s.FlowRulesPath(), s.ApprovalAllowlistPath()}
	matches, _ := filepath.Glob(filepath.Join(s.CanonicalDir(), "*.json"))
	sort.Strings(matches)
	return append(files, matches...)
}

func (s *EngineStore) IsLocalOnly(path string) bool {
	// Use forward slash normalisation so checks work on all platforms.
	normalized := filepath.ToSlash(path)
	tooling := filepath.ToSlash(s.ToolingPath())
	return strings.Contains(normalized, "/guard/") ||
		normalized == tooling ||
		strings.Contains(normalized, "structure/")
}
