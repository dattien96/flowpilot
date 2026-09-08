package runner

import (
	"os"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/workingmode"
)

const vibeTddSignaturesRel = "requirements/.flowpilot/vibe/tdd-signatures.md"

func runHasFlowNode(rs *interactiveRun, id string) bool {
	if rs == nil || strings.TrimSpace(id) == "" {
		return false
	}
	for _, n := range rs.activeFlowNodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func vibeCoderBlocked(workingMode, fromNode, coderNodeID string, tddDone, hasTestArtifact bool) bool {
	if workingMode != workingmode.Vibe || coderNodeID != "coder" {
		return false
	}
	if !tddDone {
		return true
	}
	if fromNode == "tdd" && !hasTestArtifact {
		return true
	}
	return false
}

// vibeCoderSpawnBlocked is the fail-closed spawn gate: any vibe coder needs
// the sprint tdd-signatures artifact (no glob of pre-existing tests).
func vibeCoderSpawnBlocked(workingMode, coderNodeID string, hasSignatures bool) bool {
	return workingMode == workingmode.Vibe && coderNodeID == "coder" && !hasSignatures
}

func hasVibeTddSignatures(cwd string) bool {
	if strings.TrimSpace(cwd) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(cwd, filepath.FromSlash(vibeTddSignaturesRel)))
	return err == nil
}

func hasVibeTestArtifact(cwd string) bool {
	if hasVibeTddSignatures(cwd) {
		return true
	}
	if strings.TrimSpace(cwd) == "" {
		return false
	}
	found := false
	_ = filepath.Walk(cwd, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			if info != nil && info.IsDir() {
				name := info.Name()
				if name == "vendor" || name == "node_modules" || name == ".git" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.HasSuffix(p, "_test.go") || strings.HasSuffix(p, ".test.ts") || strings.HasSuffix(p, ".test.tsx") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
