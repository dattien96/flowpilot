package tooling

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckToolNode(t *testing.T) {
	ts := CheckTool("node", ".")
	if ts.Tool != "node" {
		t.Fatalf("expected tool=node, got %q", ts.Tool)
	}
	if ts.CheckedAt == "" {
		t.Fatal("CheckedAt must not be empty")
	}
	// node should be available in the CI / dev environment.
	if ts.Status != "ok" {
		t.Skipf("node not found on this machine (status=%s); skipping version assertion", ts.Status)
	}
	if ts.Version == "" {
		t.Fatal("node status=ok but Version is empty")
	}
}

func TestCheckToolUnknown(t *testing.T) {
	ts := CheckTool("no_such_tool_xyz", ".")
	if ts.Status != "missing" {
		t.Fatalf("expected missing for unknown tool, got %q", ts.Status)
	}
}

func TestStatusOf(t *testing.T) {
	statuses := []ToolStatus{
		{Tool: "gitnexus", Status: "ok", Version: "1.2.3"},
		{Tool: "rtk", Status: "missing"},
		{Tool: "node", Status: "ok", Version: "v20.0.0"},
	}

	s := StatusOf(statuses, "gitnexus")
	if s.Status != "ok" || s.Version != "1.2.3" {
		t.Fatalf("unexpected gitnexus status: %+v", s)
	}

	s = StatusOf(statuses, "rtk")
	if s.Status != "missing" {
		t.Fatalf("unexpected rtk status: %+v", s)
	}

	s = StatusOf(statuses, "not_there")
	if s.Tool != "" || s.Status != "" {
		t.Fatalf("expected zero value for missing name, got %+v", s)
	}
}

func TestComputeCapabilityProfileMockStatuses(t *testing.T) {
	// Build a temp repo-like dir with a go.mod and a specs file.
	repoDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specsDir := filepath.Join(repoDir, "requirements", "05-System-Specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specsDir, "SS-01-Something.md"), []byte("# spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Add a .go file to detect language.
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	statuses := []ToolStatus{
		{Tool: "gitnexus", Status: "ok"},
		{Tool: "rtk", Status: "missing"},
		{Tool: "node", Status: "ok"},
		{Tool: "skill_pack", Status: "missing"},
	}

	p := ComputeCapabilityProfile(repoDir, statuses)

	if !p.HasGitNexus {
		t.Error("expected HasGitNexus=true")
	}
	if p.HasRTK {
		t.Error("expected HasRTK=false")
	}
	if !p.HasNode {
		t.Error("expected HasNode=true")
	}
	if !p.HasTests {
		t.Error("expected HasTests=true (go.mod present)")
	}
	if !p.HasSpecs {
		t.Error("expected HasSpecs=true (SS-01 present)")
	}
	if p.StructureTier != "gitnexus" {
		t.Errorf("expected StructureTier=gitnexus, got %q", p.StructureTier)
	}
	if p.DecisionTier != "full" {
		t.Errorf("expected DecisionTier=full, got %q", p.DecisionTier)
	}
	found := false
	for _, l := range p.Languages {
		if l == "go" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'go' in Languages, got %v", p.Languages)
	}
}

func TestComputeCapabilityProfileNoGitNexus(t *testing.T) {
	repoDir := t.TempDir()
	statuses := []ToolStatus{
		{Tool: "gitnexus", Status: "missing"},
		{Tool: "rtk", Status: "missing"},
		{Tool: "node", Status: "missing"},
		{Tool: "skill_pack", Status: "missing"},
	}
	p := ComputeCapabilityProfile(repoDir, statuses)
	if p.StructureTier != "fallback" {
		t.Errorf("expected fallback, got %q", p.StructureTier)
	}
	if p.DecisionTier != "git-only" {
		t.Errorf("expected git-only, got %q", p.DecisionTier)
	}
}

func TestCheckAllWritesToTempDir(t *testing.T) {
	repoDir := t.TempDir()
	dotFlowpilot := t.TempDir()

	statuses, err := CheckAll(repoDir, dotFlowpilot)
	if err != nil {
		t.Fatalf("CheckAll returned error: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("expected at least one ToolStatus")
	}

	// Verify the file was written.
	destPath := filepath.Join(dotFlowpilot, "tooling.json")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("tooling.json not created: %v", err)
	}

	var loaded []ToolStatus
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to parse tooling.json: %v", err)
	}
	if len(loaded) != len(statuses) {
		t.Fatalf("written %d entries, read back %d", len(statuses), len(loaded))
	}

	// Round-trip via LoadToolingStatus.
	loaded2, err := LoadToolingStatus(dotFlowpilot)
	if err != nil {
		t.Fatalf("LoadToolingStatus error: %v", err)
	}
	if len(loaded2) != len(statuses) {
		t.Fatalf("LoadToolingStatus returned %d entries, expected %d", len(loaded2), len(statuses))
	}
}

func TestCheckGlobalExcludesProjectScopedSkillPack(t *testing.T) {
	statuses := CheckGlobal()
	if len(statuses) != 3 {
		t.Fatalf("expected 3 global tool statuses, got %d", len(statuses))
	}
	for _, status := range statuses {
		if status.Tool == "skill_pack" {
			t.Fatalf("CheckGlobal must not include skill_pack: %+v", statuses)
		}
		if status.CheckedAt == "" {
			t.Fatalf("expected CheckedAt for %s", status.Tool)
		}
	}
}

func TestLoadToolingStatusMissingFile(t *testing.T) {
	_, err := LoadToolingStatus(t.TempDir())
	if err == nil {
		t.Fatal("expected error when tooling.json does not exist")
	}
}
