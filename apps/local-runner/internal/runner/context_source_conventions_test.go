package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/promptpacker"
)

// Task-343 (CP-62 P-7): conventions context source — repo-as-config, user > workspace,
// AGENTS.md fallback, Tier-1 non-droppable through the existing packer classification.

func writeConventions(t *testing.T, home, workspace, userBody, wsBody, agentsBody string) (workspaceDir string) {
	t.Helper()
	if userBody != "" {
		if err := os.MkdirAll(filepath.Join(home, ".flowpilot"), 0o755); err != nil {
			t.Fatalf("mkdir user: %v", err)
		}
		if err := os.WriteFile(filepath.Join(home, ".flowpilot", "conventions.md"), []byte(userBody), 0o644); err != nil {
			t.Fatalf("write user: %v", err)
		}
	}
	if wsBody != "" || agentsBody == "" {
		if err := os.MkdirAll(filepath.Join(workspace, ".flowpilot"), 0o755); err != nil {
			t.Fatalf("mkdir ws: %v", err)
		}
		if wsBody != "" {
			if err := os.WriteFile(filepath.Join(workspace, ".flowpilot", "conventions.md"), []byte(wsBody), 0o644); err != nil {
				t.Fatalf("write ws: %v", err)
			}
		}
	}
	if agentsBody != "" {
		if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte(agentsBody), 0o644); err != nil {
			t.Fatalf("write agents: %v", err)
		}
	}
	return workspace
}

func fetchConventions(t *testing.T, home, workspace string) FlowContextSection {
	t.Helper()
	t.Setenv("HOME", home) // isolate the user layer per test
	src := &conventionsSource{priority: 0}
	section, err := src.Fetch(context.Background(), FlowContextHints{Workspace: workspace})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	return section
}

// Scenario: Nạp và ưu tiên quy ước cấp User trước quy ước cấp Workspace
func TestConventionsSource_UserBeatsWorkspace(t *testing.T) {
	home := t.TempDir()
	ws := writeConventions(t, home, t.TempDir(), "user rule: gofmt everything", "workspace rule: keep tests table-driven", "")
	section := fetchConventions(t, home, ws)
	if !strings.Contains(section.Body, "user rule: gofmt everything") ||
		!strings.Contains(section.Body, "workspace rule: keep tests table-driven") {
		t.Fatalf("body must merge both layers: %q", section.Body)
	}
	if strings.Index(section.Body, "user rule") > strings.Index(section.Body, "workspace rule") {
		t.Fatalf("user layer must come first: %q", section.Body)
	}
	if section.SourceRef == "" {
		t.Fatalf("source ref must cite the loaded files")
	}
}

// Scenario: Khi không có .flowpilot/conventions.md, source tự động fallback sang AGENTS.md
func TestConventionsSource_FallbackToAgentsMd(t *testing.T) {
	home := t.TempDir()
	ws := writeConventions(t, home, t.TempDir(), "", "", "# Agents: use table tests")
	section := fetchConventions(t, home, ws)
	if !strings.Contains(section.Body, "# Agents: use table tests") {
		t.Fatalf("AGENTS.md fallback missing: %q", section.Body)
	}
}

// Scenario: Khi cả 2 file đều vắng mặt -> trả về rỗng, không phát sinh lỗi
func TestConventionsSource_MissingFile_EmptyWithoutError(t *testing.T) {
	home := t.TempDir()
	section := fetchConventions(t, home, t.TempDir()) // home empty, workspace empty
	if section.Body != "" || section.SourceRef != "" {
		t.Fatalf("all-missing setup must be empty, got %q / %q", section.Body, section.SourceRef)
	}
	_ = home
}

// Scenario: Section conventions không bị cắt tỉa trong Budget Packer dù vượt ngân sách
// (the "## Context" heading routes the section into the mandatory_doc kind — CP-23 R-1).
func TestConventionsSource_Tier1NonDroppableInBudgetPacker(t *testing.T) {
	home := t.TempDir()
	ws := writeConventions(t, home, t.TempDir(), "", strings.Repeat("keep tests table-driven\n", 400), "")
	section := fetchConventions(t, home, ws)
	sections := splitPromptIntoSections(section.Body + "\n\n" + strings.Repeat("filler content\n", 800))
	budget := defaultBudgetPackerBudget()
	budget.TotalMaxTokens = 100 // tiny — prunables must drop, conventions must stay
	packed, report, err := promptpacker.PackPrompt(sections, budget)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if !strings.Contains(packed, "Project Conventions") || !strings.Contains(packed, "keep tests table-driven") {
		t.Fatalf("conventions section must survive the tight budget")
	}
	for _, item := range report.DroppedItems {
		if strings.Contains(item, "Project Conventions") {
			t.Fatalf("conventions must never be dropped: %v", report.DroppedItems)
		}
	}
}

// Scenario: Flow YAML giữ nguyên không đổi khi chạy qua các môi trường repository khác nhau —
// the pack's flagship flows reference the registered conventions source in their profiles.
func TestConventionsSource_FlowYAMLInvariantAcrossProjects(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	var found int
	for i := range pack.Flows {
		if pack.Flows[i].ID != "task-harness" && pack.Flows[i].ID != "bug-plan-harness" && pack.Flows[i].ID != "vibe-sprint" {
			continue
		}
		found++
		for _, profile := range pack.Flows[i].ContextProfiles {
			has := false
			for _, id := range profile.CandidateSources {
				if id == "conventions" {
					has = true
				}
			}
			if !has {
				t.Fatalf("flow %q profile %q missing conventions source", pack.Flows[i].ID, profile.Name)
			}
		}
	}
	if found != 3 {
		t.Fatalf("flagship flows = %d want 3", found)
	}
	// Validation passes with conventions registered (no fork, no unknown id).
	for i := range pack.Flows {
		if err := ValidateFlowContextSources(pack.Flows[i]); err != nil {
			t.Fatalf("validate %q: %v", pack.Flows[i].ID, err)
		}
	}
}
