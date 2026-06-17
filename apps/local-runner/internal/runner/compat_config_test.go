package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompatConfigFallsBackToBuiltInDefaultsWhenMissing(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	config, err := instance.LoadCompatConfig()
	if err != nil {
		t.Fatalf("LoadCompatConfig() failed: %v", err)
	}

	if config.TestedClaudeVersion != CompatTestedClaudeVersion {
		t.Fatalf("expected default Claude version %q, got %q", CompatTestedClaudeVersion, config.TestedClaudeVersion)
	}
	if config.TestedCodexVersion != CompatTestedCodexVersion {
		t.Fatalf("expected default Codex version %q, got %q", CompatTestedCodexVersion, config.TestedCodexVersion)
	}
}

func TestCompatConfigSaveCreatesWorkspaceFileAndNormalizesBlanks(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	saved, err := instance.SaveCompatConfig(CompatConfig{
		TestedClaudeVersion: " 2.1.200 ",
		TestedCodexVersion:  "",
	})
	if err != nil {
		t.Fatalf("SaveCompatConfig() failed: %v", err)
	}

	if saved.TestedClaudeVersion != "2.1.200" {
		t.Fatalf("expected trimmed Claude version, got %q", saved.TestedClaudeVersion)
	}
	if saved.TestedCodexVersion != CompatTestedCodexVersion {
		t.Fatalf("expected blank Codex version to fall back to %q, got %q", CompatTestedCodexVersion, saved.TestedCodexVersion)
	}

	raw, err := os.ReadFile(filepath.Join(instance.workspace, ".flowpilot", "settings", "compat-config.json"))
	if err != nil {
		t.Fatalf("expected compat config file to be written: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected compat config file to contain JSON")
	}

	loaded, err := instance.LoadCompatConfig()
	if err != nil {
		t.Fatalf("LoadCompatConfig() after save failed: %v", err)
	}
	if loaded != saved {
		t.Fatalf("expected loaded config %#v to match saved %#v", loaded, saved)
	}
}
