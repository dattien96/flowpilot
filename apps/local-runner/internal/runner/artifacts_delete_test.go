package runner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteArtifactsByWorkflowRunIDsRemovesWorkflowRunDirectories(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	deletedRunDir := filepath.Join(instance.artifactRoot(), "project-1", "run-1")
	keptRunDir := filepath.Join(instance.artifactRoot(), "project-1", "run-2")

	writeWorkflowArtifactFiles(t, filepath.Join(deletedRunDir, "test_codex_single_out_artifact"), "run-1")
	writeWorkflowArtifactFiles(t, filepath.Join(keptRunDir, "test_codex_single_out_artifact"), "run-2")

	if err := instance.DeleteArtifactsByWorkflowRunIDs([]string{"run-1"}); err != nil {
		t.Fatalf("delete artifacts: %v", err)
	}

	if _, err := os.Stat(deletedRunDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected deleted workflow run directory to be removed, got err=%v", err)
	}
	if _, err := os.Stat(keptRunDir); err != nil {
		t.Fatalf("expected other workflow run directory to stay, got err=%v", err)
	}
}

func TestDeleteArtifactsByWorkflowRunIDsRemovesPromptExecutionRunDirectories(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	deletedRunDir := filepath.Join(instance.artifactRoot(), "local", "prompt-execution", "run-1")
	keptRunDir := filepath.Join(instance.artifactRoot(), "local", "prompt-execution", "run-2")

	writePromptArtifactFiles(t, filepath.Join(deletedRunDir, "prompt_execution", "artifact-a"), "run-1")
	writePromptArtifactFiles(t, filepath.Join(keptRunDir, "prompt_execution", "artifact-b"), "run-2")

	if err := instance.DeleteArtifactsByWorkflowRunIDs([]string{"run-1"}); err != nil {
		t.Fatalf("delete artifacts: %v", err)
	}

	if _, err := os.Stat(deletedRunDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected deleted prompt run directory to be removed, got err=%v", err)
	}
	if _, err := os.Stat(keptRunDir); err != nil {
		t.Fatalf("expected other prompt run directory to stay, got err=%v", err)
	}
}

func writeWorkflowArtifactFiles(t *testing.T, stepDir string, workflowRunID string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(stepDir, ".snapshots", "artifact-a"), 0o755); err != nil {
		t.Fatalf("mkdir workflow artifact dirs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stepDir, "BusinessIdea.md"), []byte("output"), 0o644); err != nil {
		t.Fatalf("write workflow output file: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(stepDir, ".snapshots", "artifact-a", "manifest.json"),
		[]byte(`{"artifactId":"artifact-a","workflowRunId":"`+workflowRunID+`"}`),
		0o644,
	); err != nil {
		t.Fatalf("write workflow manifest: %v", err)
	}
}

func writePromptArtifactFiles(t *testing.T, artifactDir string, workflowRunID string) {
	t.Helper()

	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir prompt artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "manifest.json"), []byte(`{"artifactId":"artifact-a","workflowRunId":"`+workflowRunID+`"}`), 0o644); err != nil {
		t.Fatalf("write prompt manifest: %v", err)
	}
}
