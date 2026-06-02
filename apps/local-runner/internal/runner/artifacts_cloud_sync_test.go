package runner

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteArtifactSyncBundleIncludesArtifactFiles(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace}

	artifact, err := instance.SavePromptArtifact(
		PromptExecutionRequest{ProviderKey: "codex", Prompt: "How does CP-06-01 work?"},
		PromptExecutionResult{
			Status:         "success",
			RunID:          "run-123",
			ProviderKey:    "codex",
			Command:        "codex --prompt",
			StdoutSummary:  "stdout summary",
			StderrSummary:  "",
			OutputMarkdown: "# Artifact",
			StartedAt:      "2026-06-02T00:00:00Z",
			CompletedAt:    "2026-06-02T00:00:01Z",
		},
	)
	if err != nil {
		t.Fatalf("save prompt artifact: %v", err)
	}

	var bundle bytes.Buffer
	if err := instance.WriteArtifactSyncBundle(artifact.ArtifactID, &bundle); err != nil {
		t.Fatalf("write artifact sync bundle: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(bundle.Bytes()), int64(bundle.Len()))
	if err != nil {
		t.Fatalf("open bundle zip: %v", err)
	}

	entries := make(map[string]struct{}, len(reader.File))
	for _, file := range reader.File {
		if filepath.IsAbs(file.Name) || strings.Contains(file.Name, "..") {
			t.Fatalf("bundle entry leaked unsafe path: %q", file.Name)
		}
		entries[file.Name] = struct{}{}
	}

	for _, name := range []string{
		"manifest.json",
		"content.md",
		"prompt.md",
		"stdout.txt",
		"stderr.txt",
		"command.txt",
	} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("expected bundle to contain %q, got entries %v", name, keys(entries))
		}
	}
}

func TestSaveArtifactCloudSyncResultPreservesSuccessfulMetadataOnFailure(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace}

	artifact, err := instance.SavePromptArtifact(
		PromptExecutionRequest{ProviderKey: "codex", Prompt: "How does CP-06-01 work?"},
		PromptExecutionResult{
			Status:         "success",
			RunID:          "run-456",
			ProviderKey:    "codex",
			Command:        "codex --prompt",
			StdoutSummary:  "stdout summary",
			OutputMarkdown: "# Artifact",
			StartedAt:      "2026-06-02T00:00:00Z",
			CompletedAt:    "2026-06-02T00:00:01Z",
		},
	)
	if err != nil {
		t.Fatalf("save prompt artifact: %v", err)
	}

	synced, err := instance.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
		StorageProvider: "supabase",
		RemotePath:      "projects/project-alpha/runs/run-456/steps/prompt_execution/Artifact.md",
		RemoteObjectID:  "object-123",
		SyncStatus:      artifactSyncStatusSynced,
	})
	if err != nil {
		t.Fatalf("save synced artifact cloud result: %v", err)
	}

	if synced.StorageProvider != "supabase" {
		t.Fatalf("expected storage provider to persist, got %q", synced.StorageProvider)
	}
	if synced.RemotePath == "" || synced.RemoteObjectID != "object-123" {
		t.Fatalf("expected successful sync metadata to persist, got %#v", synced.ArtifactSummary)
	}
	if synced.SyncStatus != artifactSyncStatusSynced {
		t.Fatalf("expected synced status, got %q", synced.SyncStatus)
	}

	failed, err := instance.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
		StorageProvider: "google_drive",
		RemotePath:      "",
		RemoteObjectID:  "",
		SyncStatus:      artifactSyncStatusFailed,
		ErrorMessage:    "temporary failure",
	})
	if err != nil {
		t.Fatalf("save failed artifact cloud result: %v", err)
	}

	if failed.SyncStatus != artifactSyncStatusFailed {
		t.Fatalf("expected failed status, got %q", failed.SyncStatus)
	}
	if failed.StorageProvider != "supabase" {
		t.Fatalf("expected successful storage provider to be preserved, got %q", failed.StorageProvider)
	}
	if failed.RemotePath != synced.RemotePath {
		t.Fatalf("expected remote path to be preserved, got %q", failed.RemotePath)
	}
	if failed.RemoteObjectID != synced.RemoteObjectID {
		t.Fatalf("expected remote object id to be preserved, got %q", failed.RemoteObjectID)
	}

	manifestPath := filepath.Join(filepath.Dir(synced.ManifestPath), "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"syncStatus": "failed"`)) {
		t.Fatalf("expected manifest to persist failed sync state, got %s", string(raw))
	}
}

func keys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}
