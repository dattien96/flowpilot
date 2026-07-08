package contextsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewEngineStoreCreatesSubdirs(t *testing.T) {
	base := t.TempDir()
	store, err := NewEngineStore(base)
	if err != nil {
		t.Fatalf("NewEngineStore error: %v", err)
	}
	if store.DotFlowpilotDir != base {
		t.Errorf("DotFlowpilotDir = %q, want %q", store.DotFlowpilotDir, base)
	}
	for _, sub := range []string{"ledger", "catalog", "settings", "guard", "structure"} {
		p := filepath.Join(base, sub)
		if info, err := os.Stat(p); err != nil || !info.IsDir() {
			t.Errorf("expected subdir %s to exist", sub)
		}
	}
}

func TestSharedFilesReturnsSharedPaths(t *testing.T) {
	base := t.TempDir()
	store, _ := NewEngineStore(base)
	shared := store.SharedFiles()
	if len(shared) != 5 {
		t.Fatalf("SharedFiles() len = %d, want 5", len(shared))
	}
	// Verify each expected path is present
	found := map[string]bool{}
	for _, p := range shared {
		found[filepath.ToSlash(p)] = true
	}
	for _, want := range []string{
		filepath.ToSlash(filepath.Join(base, LedgerFile)),
		filepath.ToSlash(filepath.Join(base, ChatSummaryFile)),
		filepath.ToSlash(filepath.Join(base, CatalogFile)),
		filepath.ToSlash(filepath.Join(base, FlowRulesFile)),
		filepath.ToSlash(filepath.Join(base, ApprovalAllowlistFile)),
	} {
		if !found[want] {
			t.Errorf("SharedFiles() missing %q", want)
		}
	}
}

func TestIsLocalOnly(t *testing.T) {
	base := t.TempDir()
	store, _ := NewEngineStore(base)

	guardPath := filepath.Join(base, "guard", "something.json")
	if !store.IsLocalOnly(guardPath) {
		t.Errorf("guard path should be local-only")
	}

	toolingPath := store.ToolingPath()
	if !store.IsLocalOnly(toolingPath) {
		t.Errorf("tooling path should be local-only")
	}

	structurePath := filepath.Join(base, "structure", "repo.json")
	if !store.IsLocalOnly(structurePath) {
		t.Errorf("structure path should be local-only")
	}

	ledgerPath := store.LedgerPath()
	if store.IsLocalOnly(ledgerPath) {
		t.Errorf("ledger path should NOT be local-only")
	}

	catalogPath := store.CatalogPath()
	if store.IsLocalOnly(catalogPath) {
		t.Errorf("catalog path should NOT be local-only")
	}
}

func TestComputeSHA256(t *testing.T) {
	content := []byte("hello contextsync")
	f, err := os.CreateTemp(t.TempDir(), "sha256test")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(content)
	f.Close()

	got, err := ComputeSHA256(f.Name())
	if err != nil {
		t.Fatalf("ComputeSHA256 error: %v", err)
	}
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("ComputeSHA256 = %q, want %q", got, want)
	}
}

func TestWriteManifest(t *testing.T) {
	base := t.TempDir()
	store, _ := NewEngineStore(base)

	// Write content to ledger and catalog; leave FlowRules absent.
	ledgerContent := []byte(`{"feature":"test"}`)
	if err := os.WriteFile(store.LedgerPath(), ledgerContent, 0o644); err != nil {
		t.Fatal(err)
	}
	chatSummaryContent := []byte(`{"feature_key":"test","summary":"discussion"}`)
	if err := os.WriteFile(store.ChatSummaryPath(), chatSummaryContent, 0o644); err != nil {
		t.Fatal(err)
	}
	catalogContent := []byte(`{"id":"feat-1"}`)
	if err := os.WriteFile(store.CatalogPath(), catalogContent, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteManifest(store); err != nil {
		t.Fatalf("WriteManifest error: %v", err)
	}

	manifestPath := filepath.Join(base, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading manifest.json: %v", err)
	}

	var entries []ManifestEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	// FlowRulesFile does not exist, so only 3 entries expected.
	if len(entries) != 3 {
		t.Fatalf("manifest entries = %d, want 3", len(entries))
	}

	for _, e := range entries {
		if e.SHA256 == "" {
			t.Errorf("entry %q has empty SHA256", e.Path)
		}
		if e.Size <= 0 {
			t.Errorf("entry %q has non-positive size", e.Path)
		}
	}
}

func TestSyncSharedFilesNilSyncer(t *testing.T) {
	base := t.TempDir()
	store, _ := NewEngineStore(base)

	// Create the ledger file so it exists (others absent).
	os.WriteFile(store.LedgerPath(), []byte("x"), 0o644)

	result := SyncSharedFiles(context.Background(), store, nil)

	if len(result.Synced) != 0 {
		t.Errorf("expected 0 synced, got %d", len(result.Synced))
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}
	// All 5 shared files should be skipped (1 exists but syncer nil, 4 don't exist).
	if len(result.Skipped) != 5 {
		t.Errorf("expected 5 skipped, got %d: %v", len(result.Skipped), result.Skipped)
	}
}

func TestSyncSharedFilesWithSyncer(t *testing.T) {
	base := t.TempDir()
	store, _ := NewEngineStore(base)

	os.WriteFile(store.LedgerPath(), []byte("ledger data"), 0o644)
	os.WriteFile(store.ChatSummaryPath(), []byte("chat summary data"), 0o644)
	os.WriteFile(store.CatalogPath(), []byte("catalog data"), 0o644)

	var synced []string
	syncer := &mockSyncer{syncFn: func(ctx context.Context, localPath, remoteName string) error {
		synced = append(synced, remoteName)
		return nil
	}}

	result := SyncSharedFiles(context.Background(), store, syncer)

	if len(result.Synced) != 3 {
		t.Errorf("expected 3 synced, got %d: %v", len(result.Synced), result.Synced)
	}
	if len(result.Skipped) != 2 {
		t.Errorf("expected 2 skipped (FlowRules + ApprovalAllowlist absent), got %d", len(result.Skipped))
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %v", result.Errors)
	}
	for _, name := range synced {
		if strings.Contains(name, "/") || strings.Contains(name, "\\") {
			t.Errorf("remoteName should be base filename, got %q", name)
		}
	}
}

type mockSyncer struct {
	syncFn func(ctx context.Context, localPath, remoteName string) error
}

func (m *mockSyncer) SyncFile(ctx context.Context, localPath, remoteName string) error {
	return m.syncFn(ctx, localPath, remoteName)
}
