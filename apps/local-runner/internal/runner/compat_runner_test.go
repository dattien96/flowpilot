package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCompatProbeBinary(t *testing.T, dir, name, script string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write compat probe binary: %v", err)
	}
	return path
}

func findCompatItem(items []CompatItem, name string) CompatItem {
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	return CompatItem{}
}

func TestRunCompatCheckIncludesPortabilityCanaries(t *testing.T) {
	workspace := t.TempDir()
	binDir := t.TempDir()
	homeDir := t.TempDir()
	codexHome := filepath.Join(homeDir, ".codex")
	rolloutDir := filepath.Join(codexHome, "sessions", "2026", "06", "18")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatalf("mkdir rollout dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, ".claude", "projects"), 0o755); err != nil {
		t.Fatalf("mkdir claude projects dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rolloutDir, "rollout-abc.jsonl"), []byte("{\"payload\":{\"id\":\"rollout-abc\",\"cwd\":\"/repo\",\"originator\":\"codex\",\"source\":\"cli\"}}\n"), 0o644); err != nil {
		t.Fatalf("write rollout meta: %v", err)
	}

	writeCompatProbeBinary(t, binDir, "claude", "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo \"2.1.179\"\n  exit 0\nfi\nif [ \"$1\" = \"--help\" ]; then\n  printf '%s\\n' '--input-format --output-format --include-partial-messages --include-hook-events --strict-mcp-config --disallowed-tools --permission-mode --mcp-config --resume --effort'\n  exit 0\nfi\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false}'\n")
	writeCompatProbeBinary(t, binDir, "codex", "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo \"0.140.0\"\n  exit 0\nfi\nif [ \"$1\" = \"app-server\" ] && [ \"$2\" = \"--help\" ]; then\n  printf '%s\\n' '--listen stdio'\n  exit 0\nfi\nif [ \"$1\" = \"exec\" ] && [ \"$2\" = \"resume\" ] && [ \"$3\" = \"--help\" ]; then\n  printf '%s\\n' 'Usage: codex exec resume SESSION_ID [PROMPT]'\n  exit 0\nfi\nexit 0\n")

	instance := &Runner{workspace: workspace}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", homeDir)
	t.Setenv("CODEX_HOME", codexHome)

	result := instance.RunCompatCheck(context.Background())

	if item := findCompatItem(result.Items, "codex exec resume surface"); item.Status != "pass" {
		t.Fatalf("expected codex exec resume surface pass, got %#v", item)
	}
	if item := findCompatItem(result.Items, "codex rollout portability metadata"); item.Status != "pass" {
		t.Fatalf("expected rollout metadata pass, got %#v", item)
	}
	if item := findCompatItem(result.Items, "claude session store layout"); item.Status != "pass" {
		t.Fatalf("expected claude session store layout pass, got %#v", item)
	}
}

func TestRunCompatCheckFailsWhenCodexPortabilityContractDrifts(t *testing.T) {
	workspace := t.TempDir()
	binDir := t.TempDir()
	homeDir := t.TempDir()
	codexHome := filepath.Join(homeDir, ".codex")
	rolloutDir := filepath.Join(codexHome, "sessions")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatalf("mkdir rollout dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rolloutDir, "rollout-abc.jsonl"), []byte("{\"payload\":{\"id\":\"rollout-abc\",\"account_id\":\"acct-a\"}}\n"), 0o644); err != nil {
		t.Fatalf("write rollout meta: %v", err)
	}

	writeCompatProbeBinary(t, binDir, "claude", "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo \"2.1.179\"\n  exit 0\nfi\nif [ \"$1\" = \"--help\" ]; then\n  printf '%s\\n' '--input-format --output-format --include-partial-messages --include-hook-events --strict-mcp-config --disallowed-tools --permission-mode --mcp-config --resume --effort'\n  exit 0\nfi\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false}'\n")
	writeCompatProbeBinary(t, binDir, "codex", "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo \"0.140.0\"\n  exit 0\nfi\nif [ \"$1\" = \"app-server\" ] && [ \"$2\" = \"--help\" ]; then\n  printf '%s\\n' '--listen stdio'\n  exit 0\nfi\nif [ \"$1\" = \"exec\" ] && [ \"$2\" = \"resume\" ] && [ \"$3\" = \"--help\" ]; then\n  printf '%s\\n' 'Usage: codex exec resume'\n  exit 0\nfi\nexit 0\n")

	instance := &Runner{workspace: workspace}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", homeDir)
	t.Setenv("CODEX_HOME", codexHome)

	result := instance.RunCompatCheck(context.Background())

	if item := findCompatItem(result.Items, "codex exec resume surface"); item.Status != "fail" || !strings.Contains(item.Detail, "resume help shape changed") {
		t.Fatalf("expected codex resume surface failure, got %#v", item)
	}
	if item := findCompatItem(result.Items, "codex rollout portability metadata"); item.Status != "fail" {
		t.Fatalf("expected rollout portability failure, got %#v", item)
	}
}
