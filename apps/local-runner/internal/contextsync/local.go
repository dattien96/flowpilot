package contextsync

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	LedgerFile      = "ledger/feature_history.ndjson"
	ChatSummaryFile = "ledger/chat_summary.ndjson"
	CatalogFile     = "catalog/features.ndjson"
	FlowRulesFile   = "settings/flow-rules.json"
)

type EngineStore struct {
	DotFlowpilotDir string
}

func NewEngineStore(dotFlowpilotDir string) (*EngineStore, error) {
	subdirs := []string{"ledger", "catalog", "settings", "guard", "structure"}
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

func (s *EngineStore) GuardDir() string {
	return filepath.Join(s.DotFlowpilotDir, "guard")
}

func (s *EngineStore) ToolingPath() string {
	return filepath.Join(s.DotFlowpilotDir, "tooling.json")
}

func (s *EngineStore) SharedFiles() []string {
	return []string{s.LedgerPath(), s.ChatSummaryPath(), s.CatalogPath(), s.FlowRulesPath()}
}

func (s *EngineStore) IsLocalOnly(path string) bool {
	// Use forward slash normalisation so checks work on all platforms.
	normalized := filepath.ToSlash(path)
	tooling := filepath.ToSlash(s.ToolingPath())
	return strings.Contains(normalized, "/guard/") ||
		normalized == tooling ||
		strings.Contains(normalized, "structure/")
}
