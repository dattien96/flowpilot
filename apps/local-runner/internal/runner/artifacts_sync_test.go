package runner

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewLoadsWorkspaceRootEnvFileWithoutOverridingExistingEnv(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir .agents: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workspace, ".env"),
		[]byte("SUPABASE_API_URL=https://from-dot-env.supabase.co\nSUPABASE_SERVICE_ROLE_KEY=from-dot-env-key\n"),
		0o644,
	); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	originalURL, hadURL := os.LookupEnv("SUPABASE_API_URL")
	originalServiceKey, hadServiceKey := os.LookupEnv("SUPABASE_SERVICE_ROLE_KEY")
	t.Cleanup(func() {
		if hadURL {
			_ = os.Setenv("SUPABASE_API_URL", originalURL)
		} else {
			_ = os.Unsetenv("SUPABASE_API_URL")
		}
		if hadServiceKey {
			_ = os.Setenv("SUPABASE_SERVICE_ROLE_KEY", originalServiceKey)
		} else {
			_ = os.Unsetenv("SUPABASE_SERVICE_ROLE_KEY")
		}
	})

	_ = os.Unsetenv("SUPABASE_API_URL")
	_ = os.Unsetenv("SUPABASE_SERVICE_ROLE_KEY")

	instance, err := New(workspace)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if instance.workspace != workspace {
		t.Fatalf("expected workspace %q, got %q", workspace, instance.workspace)
	}

	config, err := readSupabaseArtifactConfig()
	if err != nil {
		t.Fatalf("readSupabaseArtifactConfig() failed: %v", err)
	}
	if config.baseURL != "https://from-dot-env.supabase.co" {
		t.Fatalf("expected baseURL from .env, got %q", config.baseURL)
	}
	if config.serviceKey != "from-dot-env-key" {
		t.Fatalf("expected service key from .env, got %q", config.serviceKey)
	}

	t.Setenv("SUPABASE_API_URL", "https://from-process.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "from-process-key")
	if _, err := New(workspace); err != nil {
		t.Fatalf("New() with preloaded env failed: %v", err)
	}

	config, err = readSupabaseArtifactConfig()
	if err != nil {
		t.Fatalf("readSupabaseArtifactConfig() with process env failed: %v", err)
	}
	if config.baseURL != "https://from-process.supabase.co" {
		t.Fatalf("expected existing process env to win, got %q", config.baseURL)
	}
	if config.serviceKey != "from-process-key" {
		t.Fatalf("expected existing process env key to win, got %q", config.serviceKey)
	}
}

func TestWriteArtifactSyncBundleStreamsByteSafeZip(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	artifactID := writeTestArtifactFixture(t, instance.workspace, "artifact-bundle")

	var buffer bytes.Buffer
	if err := instance.WriteArtifactSyncBundle(artifactID, &buffer); err != nil {
		t.Fatalf("WriteArtifactSyncBundle() failed: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader() failed: %v", err)
	}

	files := map[string][]byte{}
	for _, file := range reader.File {
		data, err := readZipEntry(file)
		if err != nil {
			t.Fatalf("read zip entry %q: %v", file.Name, err)
		}
		files[file.Name] = data
	}

	if got := files["manifest.json"]; len(got) == 0 {
		t.Fatal("expected manifest.json to be present in sync bundle")
	}

	if got := files["payload.bin"]; !bytes.Equal(got, []byte{0x00, 0x01, 0x02, 0xff, 0x10, 0x00}) {
		t.Fatalf("expected binary payload to survive ZIP round-trip, got %v", got)
	}

	if got := files["content.md"]; string(got) != "Generated artifact content" {
		t.Fatalf("expected content.md to be preserved byte-for-byte, got %q", string(got))
	}
}

func TestWriteArtifactSyncBundleRejectsLimitViolations(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	artifactID := writeTestArtifactFixture(t, instance.workspace, "artifact-limits")

	originalMaxEntries := artifactSyncBundleMaxEntries
	originalMaxEntrySize := artifactSyncBundleMaxEntrySize
	originalMaxTotalSize := artifactSyncBundleMaxTotalSize
	t.Cleanup(func() {
		artifactSyncBundleMaxEntries = originalMaxEntries
		artifactSyncBundleMaxEntrySize = originalMaxEntrySize
		artifactSyncBundleMaxTotalSize = originalMaxTotalSize
	})

	t.Run("entry_count", func(t *testing.T) {
		artifactSyncBundleMaxEntries = 1
		var buffer bytes.Buffer
		err := instance.WriteArtifactSyncBundle(artifactID, &buffer)
		if err == nil || !strings.Contains(err.Error(), "entry count") {
			t.Fatalf("expected entry-count error, got %v", err)
		}
	})

	t.Run("entry_size", func(t *testing.T) {
		artifactSyncBundleMaxEntries = originalMaxEntries
		artifactSyncBundleMaxEntrySize = 2
		artifactSyncBundleMaxTotalSize = originalMaxTotalSize
		var buffer bytes.Buffer
		err := instance.WriteArtifactSyncBundle(artifactID, &buffer)
		if err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
			t.Fatalf("expected entry-size error, got %v", err)
		}
	})

	t.Run("total_size", func(t *testing.T) {
		artifactSyncBundleMaxEntries = originalMaxEntries
		artifactSyncBundleMaxEntrySize = originalMaxEntrySize
		artifactSyncBundleMaxTotalSize = 3
		var buffer bytes.Buffer
		err := instance.WriteArtifactSyncBundle(artifactID, &buffer)
		if err == nil || !strings.Contains(err.Error(), "total size") {
			t.Fatalf("expected total-size error, got %v", err)
		}
	})
}

func TestWriteArtifactSyncBundleRejectsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture is environment-dependent on windows")
	}

	instance := &Runner{workspace: t.TempDir()}
	artifactID := "artifact-symlink"
	artifactDir := writeTestArtifactBase(t, instance.workspace, artifactID)

	outsideFile := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	linkPath := filepath.Join(artifactDir, "escape.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}

	var buffer bytes.Buffer
	err := instance.WriteArtifactSyncBundle(artifactID, &buffer)
	if err == nil || !strings.Contains(err.Error(), "symlinks") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestSaveArtifactCloudSyncResultPersistsAndPreservesPreviousMetadata(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	artifactID := writeTestArtifactFixture(t, instance.workspace, "artifact-cloud")

	success, err := instance.SaveArtifactCloudSyncResult(artifactID, ArtifactCloudSyncResult{
		StorageProvider: "supabase",
		RemotePath:      "projects/local/runs/run-1/steps/prompt_execution",
		RemoteObjectID:  "object-1",
		SyncStatus:      artifactSyncStatusSynced,
	})
	if err != nil {
		t.Fatalf("SaveArtifactCloudSyncResult() success failed: %v", err)
	}

	if success.StorageProvider != "supabase" {
		t.Fatalf("expected storage provider to persist, got %q", success.StorageProvider)
	}
	if success.RemotePath != "projects/local/runs/run-1/steps/prompt_execution" {
		t.Fatalf("expected remote path to persist, got %q", success.RemotePath)
	}
	if success.RemoteObjectID != "object-1" {
		t.Fatalf("expected remote object id to persist, got %q", success.RemoteObjectID)
	}
	if success.RemoteURL != "" {
		t.Fatalf("expected remote URL to stay empty for cloud sync results, got %q", success.RemoteURL)
	}

	failed, err := instance.SaveArtifactCloudSyncResult(artifactID, ArtifactCloudSyncResult{
		StorageProvider: "google_drive",
		RemotePath:      "projects/local/runs/run-1/steps/prompt_execution/should-not-stick",
		RemoteObjectID:  "object-2",
		SyncStatus:      artifactSyncStatusFailed,
		ErrorMessage:    "provider rejected retry",
	})
	if err != nil {
		t.Fatalf("SaveArtifactCloudSyncResult() failed retry failed: %v", err)
	}

	if failed.SyncStatus != artifactSyncStatusFailed {
		t.Fatalf("expected failed status to persist, got %q", failed.SyncStatus)
	}
	if failed.StorageProvider != "supabase" {
		t.Fatalf("expected prior successful storage provider to be preserved, got %q", failed.StorageProvider)
	}
	if failed.RemotePath != "projects/local/runs/run-1/steps/prompt_execution" {
		t.Fatalf("expected prior successful remote path to be preserved, got %q", failed.RemotePath)
	}
	if failed.RemoteObjectID != "object-1" {
		t.Fatalf("expected prior successful remote object id to be preserved, got %q", failed.RemoteObjectID)
	}
	if failed.RemoteURL != "" {
		t.Fatalf("expected remote URL to remain empty, got %q", failed.RemoteURL)
	}

	reloaded, err := instance.GetArtifact(artifactID)
	if err != nil {
		t.Fatalf("GetArtifact() after save failed: %v", err)
	}
	if reloaded.StorageProvider != "supabase" || reloaded.RemotePath != "projects/local/runs/run-1/steps/prompt_execution" || reloaded.RemoteObjectID != "object-1" {
		t.Fatalf("expected persisted successful metadata to remain intact, got %#v", reloaded.ArtifactSummary)
	}
}

func TestSyncArtifactSupabaseUploadsHistoryAndCanonical(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	artifactID := writeTestArtifactFixture(t, instance.workspace, "artifact-supabase")

	t.Setenv("SUPABASE_API_URL", "https://example.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "service-role-key")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})

	uploadedPaths := make([]string, 0)
	httpRequestFn = func(
		ctx context.Context,
		method string,
		endpoint string,
		headers map[string]string,
		body []byte,
	) (int, []byte, error) {
		if ctx == nil {
			t.Fatal("expected sync requests to include a context")
		}
		switch {
		case method == "POST" && strings.Contains(endpoint, "/storage/v1/object/flowpilot-artifacts/"):
			uploadedPaths = append(uploadedPaths, endpoint)
			if headers["x-metadata"] == "" {
				t.Fatal("expected x-metadata header to be populated")
			}
			rawMetadata, err := base64.StdEncoding.DecodeString(headers["x-metadata"])
			if err != nil {
				t.Fatalf("decode x-metadata: %v", err)
			}
			var metadata map[string]string
			if err := json.Unmarshal(rawMetadata, &metadata); err != nil {
				t.Fatalf("unmarshal x-metadata: %v", err)
			}
			if metadata["artifactId"] != artifactID {
				t.Fatalf("expected artifactId metadata, got %#v", metadata)
			}
			return 200, []byte(`{"Id":"object-1","Key":"` + endpoint + `"}`), nil
		case method == "GET" && strings.Contains(endpoint, "/storage/v1/object/info/flowpilot-artifacts/projects/local/runs/run-1/steps/prompt_execution/artifacts/"+artifactID+"/content.md"):
			return 404, []byte(`{"message":"not found"}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", method, endpoint)
			return 0, nil, nil
		}
	}

	artifact, err := instance.SyncArtifact(artifactID, ArtifactSyncRequest{StorageProvider: "supabase"})
	if err != nil {
		t.Fatalf("SyncArtifact() supabase failed: %v", err)
	}

	if artifact.SyncStatus != artifactSyncStatusSynced {
		t.Fatalf("expected synced status, got %q", artifact.SyncStatus)
	}
	if artifact.StorageProvider != "supabase" {
		t.Fatalf("expected supabase provider, got %q", artifact.StorageProvider)
	}
	if artifact.RemotePath != "projects/local/runs/run-1/steps/prompt_execution/artifacts/"+artifactID+"/content.md" {
		t.Fatalf("unexpected remote path: %q", artifact.RemotePath)
	}
	if artifact.RemoteObjectID != "object-1" {
		t.Fatalf("unexpected remote object id: %q", artifact.RemoteObjectID)
	}
	if len(uploadedPaths) != 4 {
		t.Fatalf("expected 4 uploads (3 snapshots + 1 canonical), got %d", len(uploadedPaths))
	}
}

func TestSyncArtifactSupabaseTreatsWrappedNotFoundAsMissingObject(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	artifactID := writeTestArtifactFixture(t, instance.workspace, "artifact-supabase-wrapped-404")

	t.Setenv("SUPABASE_API_URL", "https://example.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "service-role-key")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})

	uploadedPaths := make([]string, 0)
	httpRequestFn = func(
		ctx context.Context,
		method string,
		endpoint string,
		headers map[string]string,
		body []byte,
	) (int, []byte, error) {
		if ctx == nil {
			t.Fatal("expected sync requests to include a context")
		}
		switch {
		case method == "POST" && strings.Contains(endpoint, "/storage/v1/object/flowpilot-artifacts/"):
			uploadedPaths = append(uploadedPaths, endpoint)
			return 200, []byte(`{"Id":"object-404","Key":"` + endpoint + `"}`), nil
		case method == "GET" && strings.Contains(endpoint, "/storage/v1/object/info/flowpilot-artifacts/projects/local/runs/run-1/steps/prompt_execution/artifacts/"+artifactID+"/content.md"):
			return 400, []byte(`{"statusCode":"404","error":"not_found","message":"Object not found"}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", method, endpoint)
			return 0, nil, nil
		}
	}

	artifact, err := instance.SyncArtifact(artifactID, ArtifactSyncRequest{StorageProvider: "supabase"})
	if err != nil {
		t.Fatalf("SyncArtifact() supabase failed: %v", err)
	}

	if artifact.SyncStatus != artifactSyncStatusSynced {
		t.Fatalf("expected synced status, got %q", artifact.SyncStatus)
	}
	if artifact.RemoteObjectID != "object-404" {
		t.Fatalf("unexpected remote object id: %q", artifact.RemoteObjectID)
	}
	if len(uploadedPaths) != 4 {
		t.Fatalf("expected 4 uploads (3 snapshots + 1 canonical), got %d", len(uploadedPaths))
	}
}

func TestBuildArtifactCanonicalPathUsesArtifactScopedPath(t *testing.T) {
	artifact := ArtifactDetail{
		ArtifactSummary: ArtifactSummary{
			ArtifactID:      "artifact-2",
			ProjectID:       "local",
			WorkflowRunID:   "run-1",
			WorkflowStepKey: "prompt_execution",
		},
		ContentPath: "content.md",
	}

	got := buildArtifactCanonicalPath(artifact)

	if got != "projects/local/runs/run-1/steps/prompt_execution/artifacts/artifact-2/content.md" {
		t.Fatalf("unexpected canonical path: %q", got)
	}
}

func TestResolveArtifactOpenURLCreatesSupabaseSignedURL(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	artifactID := writeTestArtifactFixture(t, instance.workspace, "artifact-open")

	if _, err := instance.SaveArtifactCloudSyncResult(artifactID, ArtifactCloudSyncResult{
		StorageProvider: "supabase",
		RemotePath:      "projects/local/runs/run-1/steps/prompt_execution/artifacts/" + artifactID + "/content.md",
		RemoteObjectID:  "object-123",
		SyncStatus:      artifactSyncStatusSynced,
	}); err != nil {
		t.Fatalf("seed cloud sync result: %v", err)
	}

	t.Setenv("SUPABASE_API_URL", "https://example.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "service-role-key")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})

	httpRequestFn = func(
		ctx context.Context,
		method string,
		endpoint string,
		headers map[string]string,
		body []byte,
	) (int, []byte, error) {
		if method != "POST" {
			t.Fatalf("expected POST for signed URL, got %s", method)
		}
		if !strings.Contains(endpoint, "/storage/v1/object/sign/flowpilot-artifacts/projects/local/runs/run-1/steps/prompt_execution/artifacts/"+artifactID+"/content.md") {
			t.Fatalf("unexpected signed URL endpoint: %s", endpoint)
		}
		return 200, []byte(`{"signedURL":"/storage/v1/object/sign/flowpilot-artifacts/projects/local/runs/run-1/steps/prompt_execution/artifacts/` + artifactID + `/content.md?token=abc"}`), nil
	}

	url, err := instance.ResolveArtifactOpenURL(artifactID, "")
	if err != nil {
		t.Fatalf("ResolveArtifactOpenURL() failed: %v", err)
	}

	if url != "https://example.supabase.co/storage/v1/object/sign/flowpilot-artifacts/projects/local/runs/run-1/steps/prompt_execution/artifacts/"+artifactID+"/content.md?token=abc" {
		t.Fatalf("unexpected signed URL: %q", url)
	}
}

func TestSyncArtifactGoogleDriveUploadsHistoryAndCanonical(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	artifactID := writeTestArtifactFixture(t, instance.workspace, "artifact-drive")
	if err := instance.saveGoogleDriveCredential("integration-drive", googleDriveCredential{
		RefreshToken: "refresh-token-1",
		AccountEmail: "owner@example.com",
	}); err != nil {
		t.Fatalf("seed google drive credential: %v", err)
	}

	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id-1")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret-1")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})

	api := newFakeGoogleDriveAPI("root-folder")
	httpRequestFn = api.handle

	artifact, err := instance.SyncArtifact(artifactID, ArtifactSyncRequest{
		StorageProvider:          "google_drive",
		GoogleDriveIntegrationID: "integration-drive",
		GoogleDriveFolderID:      "root-folder",
	})
	if err != nil {
		t.Fatalf("SyncArtifact() google drive failed: %v", err)
	}

	if artifact.SyncStatus != artifactSyncStatusSynced {
		t.Fatalf("expected synced status, got %q", artifact.SyncStatus)
	}
	if artifact.StorageProvider != "google_drive" {
		t.Fatalf("expected google_drive provider, got %q", artifact.StorageProvider)
	}
	if artifact.RemotePath != "projects/local/runs/run-1/steps/prompt_execution/artifacts/"+artifactID+"/content.md" {
		t.Fatalf("unexpected remote path: %q", artifact.RemotePath)
	}
	if strings.TrimSpace(artifact.RemoteObjectID) == "" {
		t.Fatal("expected remote object id to be populated")
	}

	files := api.nonFolderFiles()
	if len(files) != 4 {
		t.Fatalf("expected 4 uploaded files (3 snapshots + 1 canonical), got %d", len(files))
	}

	canonical := api.fileByID(artifact.RemoteObjectID)
	if canonical.Name != "content.md" {
		t.Fatalf("expected canonical file name content.md, got %#v", canonical)
	}
	if canonical.AppProperties["syncRole"] != "canonical" {
		t.Fatalf("expected canonical syncRole metadata, got %#v", canonical.AppProperties)
	}
}

func TestResolveArtifactOpenURLReturnsGoogleDriveViewURL(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	artifactID := writeTestArtifactFixture(t, instance.workspace, "artifact-open-drive")

	if _, err := instance.SaveArtifactCloudSyncResult(artifactID, ArtifactCloudSyncResult{
		StorageProvider: "google_drive",
		RemotePath:      "projects/local/runs/run-1/steps/prompt_execution/artifacts/" + artifactID + "/content.md",
		RemoteObjectID:  "drive-file-123",
		SyncStatus:      artifactSyncStatusSynced,
	}); err != nil {
		t.Fatalf("seed google drive sync result: %v", err)
	}

	if err := instance.saveGoogleDriveCredentialByProject("local", googleDriveCredential{
		RefreshToken: "refresh-token-1",
		AccountEmail: "owner@example.com",
	}); err != nil {
		t.Fatalf("seed google drive project credential: %v", err)
	}
	if err := instance.saveGoogleDriveCredential("integration-drive", googleDriveCredential{
		RefreshToken: "refresh-token-1",
		AccountEmail: "owner@example.com",
	}); err != nil {
		t.Fatalf("seed google drive credential: %v", err)
	}
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Connections["local"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "local",
			Status:       "connected",
			FolderID:     "root-folder",
			FolderName:   "FlowPilot Root",
			AccountEmail: "owner@example.com",
		}
	}); err != nil {
		t.Fatalf("seed google drive connection: %v", err)
	}

	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id-1")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret-1")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(
		_ context.Context,
		method string,
		endpoint string,
		headers map[string]string,
		body []byte,
	) (int, []byte, error) {
		if endpoint == "https://oauth2.googleapis.com/token" {
			if method != "POST" {
				t.Fatalf("expected POST for google drive token refresh, got %s", method)
			}
			if headers["content-type"] != "application/x-www-form-urlencoded" {
				t.Fatalf("unexpected token refresh content type: %s", headers["content-type"])
			}
			if !strings.Contains(string(body), "refresh_token=refresh-token-1") {
				t.Fatalf("unexpected token refresh body: %s", string(body))
			}
			return 200, []byte(`{"access_token":"drive-access-token"}`), nil
		}

		t.Fatalf("unexpected google drive request: %s %s", method, endpoint)
		return 500, nil, nil
	}

	targetURL, err := instance.ResolveArtifactOpenURL(artifactID, "")
	if err != nil {
		t.Fatalf("ResolveArtifactOpenURL() google drive failed: %v", err)
	}
	if targetURL != "https://drive.google.com/file/d/drive-file-123/view" {
		t.Fatalf("unexpected google drive open URL: %q", targetURL)
	}
}

func TestResolveArtifactOpenURLCreatesSupabaseSignedURLForActualPrompt(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	artifactID := writeTestArtifactFixture(t, instance.workspace, "artifact-open-prompt")

	if _, err := instance.SaveArtifactCloudSyncResult(artifactID, ArtifactCloudSyncResult{
		StorageProvider: "supabase",
		RemotePath:      "projects/local/runs/run-1/steps/prompt_execution/artifacts/" + artifactID + "/content.md",
		RemoteObjectID:  "object-123",
		SyncStatus:      artifactSyncStatusSynced,
	}); err != nil {
		t.Fatalf("seed cloud sync result: %v", err)
	}

	t.Setenv("SUPABASE_API_URL", "https://example.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "service-role-key")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})

	httpRequestFn = func(
		ctx context.Context,
		method string,
		endpoint string,
		headers map[string]string,
		body []byte,
	) (int, []byte, error) {
		if method != "POST" {
			t.Fatalf("expected POST for signed URL, got %s", method)
		}
		if !strings.Contains(endpoint, "/storage/v1/object/sign/flowpilot-artifacts/projects/local/runs/run-1/steps/prompt_execution/.snapshots/"+artifactID+"/actual-prompt.md") {
			t.Fatalf("unexpected signed URL endpoint: %s", endpoint)
		}
		return 200, []byte(`{"signedURL":"/storage/v1/object/sign/flowpilot-artifacts/projects/local/runs/run-1/steps/prompt_execution/.snapshots/` + artifactID + `/actual-prompt.md?token=abc"}`), nil
	}

	url, err := instance.ResolveArtifactOpenURL(artifactID, "actual-prompt")
	if err != nil {
		t.Fatalf("ResolveArtifactOpenURL() failed: %v", err)
	}

	expected := "https://example.supabase.co/storage/v1/object/sign/flowpilot-artifacts/projects/local/runs/run-1/steps/prompt_execution/.snapshots/" + artifactID + "/actual-prompt.md?token=abc"
	if url != expected {
		t.Fatalf("unexpected signed URL: %q", url)
	}
}

func readZipEntry(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func writeTestArtifactFixture(t *testing.T, workspace, artifactID string) string {
	t.Helper()

	artifactDir := writeTestArtifactBase(t, workspace, artifactID)

	if err := os.WriteFile(filepath.Join(artifactDir, "content.md"), []byte("Generated artifact content"), 0o644); err != nil {
		t.Fatalf("write content.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "payload.bin"), []byte{0x00, 0x01, 0x02, 0xff, 0x10, 0x00}, 0o644); err != nil {
		t.Fatalf("write payload.bin: %v", err)
	}

	return artifactID
}

func writeTestArtifactBase(t *testing.T, workspace, artifactID string) string {
	t.Helper()

	artifactDir := filepath.Join(workspace, ".flowpilot", "artifacts", "local", "prompt-execution", "run-1", "prompt_execution", artifactID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}

	manifestPath := filepath.Join(artifactDir, "manifest.json")
	manifest := `{
  "artifactId": "` + artifactID + `",
  "title": "Artifact",
  "sourceKind": "prompt_execution",
  "projectId": "local",
  "featureId": "prompt-execution",
  "workflowRunId": "run-1",
  "workflowStepKey": "prompt_execution",
  "providerKey": "codex",
  "localPath": "` + filepath.ToSlash(artifactDir) + `",
  "remotePath": "",
  "remoteUrl": "",
  "syncStatus": "local_only",
  "createdAt": "2026-06-01T00:00:00Z",
  "updatedAt": "2026-06-01T00:00:01Z"
}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	return artifactDir
}

type fakeGoogleDriveAPI struct {
	nextID string
	files  map[string]fakeGoogleDriveFile
}

type fakeGoogleDriveFile struct {
	ID            string
	Name          string
	ParentID      string
	MimeType      string
	AppProperties map[string]string
}

func newFakeGoogleDriveAPI(rootFolderID string) *fakeGoogleDriveAPI {
	return &fakeGoogleDriveAPI{
		nextID: "1",
		files: map[string]fakeGoogleDriveFile{
			rootFolderID: {
				ID:       rootFolderID,
				Name:     "FlowPilot Root",
				MimeType: googleDriveFolderMimeType,
			},
		},
	}
}

func (api *fakeGoogleDriveAPI) handle(
	_ context.Context,
	method string,
	endpoint string,
	headers map[string]string,
	body []byte,
) (int, []byte, error) {
	switch {
	case endpoint == "https://oauth2.googleapis.com/token":
		if headers["content-type"] != "application/x-www-form-urlencoded" {
			return 400, []byte(`{"error":"bad content type"}`), nil
		}
		values, _ := url.ParseQuery(string(body))
		if values.Get("refresh_token") != "refresh-token-1" {
			return 401, []byte(`{"error":"bad refresh token"}`), nil
		}
		return 200, []byte(`{"access_token":"drive-access-token"}`), nil
	case method == "GET" && strings.HasPrefix(endpoint, "https://www.googleapis.com/drive/v3/files?"):
		return api.handleList(endpoint)
	case strings.HasPrefix(endpoint, "https://www.googleapis.com/upload/drive/v3/files"):
		return api.handleUpload(method, endpoint, headers, body)
	default:
		return 500, []byte(`{"error":"unexpected endpoint"}`), nil
	}
}

func (api *fakeGoogleDriveAPI) handleList(endpoint string) (int, []byte, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return 500, nil, err
	}
	query := parsed.Query().Get("q")
	name, parentID := parseGoogleDriveListQuery(query)
	for _, file := range api.files {
		if file.Name == name && file.ParentID == parentID {
			payload, err := json.Marshal(map[string]any{
				"files": []map[string]any{
					{
						"id":            file.ID,
						"name":          file.Name,
						"mimeType":      file.MimeType,
						"webViewLink":   "https://drive.google.com/file/d/" + file.ID + "/view",
						"appProperties": file.AppProperties,
					},
				},
			})
			return 200, payload, err
		}
	}
	return 200, []byte(`{"files":[]}`), nil
}

func (api *fakeGoogleDriveAPI) handleUpload(
	method string,
	endpoint string,
	headers map[string]string,
	body []byte,
) (int, []byte, error) {
	mediaType := headers["content-type"]
	boundaryIndex := strings.Index(mediaType, "boundary=")
	if boundaryIndex == -1 {
		return 400, []byte(`{"error":"missing boundary"}`), nil
	}
	boundary := mediaType[boundaryIndex+9:]
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var metadata struct {
		Name          string            `json:"name"`
		MimeType      string            `json:"mimeType"`
		Parents       []string          `json:"parents"`
		AppProperties map[string]string `json:"appProperties"`
	}
	for partIndex := 0; partIndex < 2; partIndex++ {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		data, err := io.ReadAll(part)
		if err != nil {
			return 500, nil, err
		}
		if partIndex == 0 {
			if err := json.Unmarshal(data, &metadata); err != nil {
				return 400, nil, err
			}
		}
	}

	parentID := ""
	if len(metadata.Parents) > 0 {
		parentID = metadata.Parents[0]
	}
	targetID := ""
	if method == "PATCH" {
		targetID = pathBaseWithoutQuery(endpoint)
	}
	if targetID == "" {
		targetID = api.allocID()
	}

	file := fakeGoogleDriveFile{
		ID:            targetID,
		Name:          metadata.Name,
		ParentID:      parentID,
		MimeType:      metadata.MimeType,
		AppProperties: metadata.AppProperties,
	}
	if file.MimeType == "" {
		file.MimeType = "application/octet-stream"
	}
	if existing, ok := api.files[targetID]; ok && file.ParentID == "" {
		file.ParentID = existing.ParentID
	}
	api.files[targetID] = file

	payload, err := json.Marshal(map[string]any{
		"id":            file.ID,
		"name":          file.Name,
		"mimeType":      file.MimeType,
		"webViewLink":   "https://drive.google.com/file/d/" + file.ID + "/view",
		"appProperties": file.AppProperties,
	})
	return 200, payload, err
}

func (api *fakeGoogleDriveAPI) allocID() string {
	id := "drive-id-" + api.nextID
	current := api.nextID
	switch current {
	case "1":
		api.nextID = "2"
	case "2":
		api.nextID = "3"
	case "3":
		api.nextID = "4"
	case "4":
		api.nextID = "5"
	case "5":
		api.nextID = "6"
	case "6":
		api.nextID = "7"
	case "7":
		api.nextID = "8"
	default:
		api.nextID = current + "x"
	}
	return id
}

func (api *fakeGoogleDriveAPI) nonFolderFiles() []fakeGoogleDriveFile {
	files := make([]fakeGoogleDriveFile, 0)
	for _, file := range api.files {
		if file.MimeType == googleDriveFolderMimeType || file.ID == "root-folder" {
			continue
		}
		files = append(files, file)
	}
	return files
}

func (api *fakeGoogleDriveAPI) fileByID(id string) fakeGoogleDriveFile {
	return api.files[id]
}

func parseGoogleDriveListQuery(query string) (name string, parentID string) {
	const namePrefix = "name = '"
	const parentMarker = "' in parents"
	if nameStart := strings.Index(query, namePrefix); nameStart >= 0 {
		rest := query[nameStart+len(namePrefix):]
		if end := strings.Index(rest, "'"); end >= 0 {
			name = strings.ReplaceAll(rest[:end], "\\'", "'")
		}
	}
	if marker := strings.Index(query, parentMarker); marker >= 0 {
		start := strings.LastIndex(query[:marker], "'")
		previous := query[:marker]
		if start >= 0 {
			parentID = strings.ReplaceAll(previous[start+1:], "\\'", "'")
		}
	}
	return name, parentID
}

func pathBaseWithoutQuery(endpoint string) string {
	trimmed := endpoint
	if index := strings.Index(trimmed, "?"); index >= 0 {
		trimmed = trimmed[:index]
	}
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}
