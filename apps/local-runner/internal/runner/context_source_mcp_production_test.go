package runner

import (
	"context"
	"strings"
	"testing"
)

// TestGoogleDriveDriverAdapterFetchesLiveDocument is Task-204's core
// end-to-end proof: given a connected Google Drive account (the exact same
// fixture shape google_drive_proxy_mcp_test.go's accessToken tests use —
// Task-204 T-3's reuse constraint), googleDriveDriverAdapter.Fetch resolves a
// real access token through resolveGoogleDriveAccessTokenForRunner and reads
// the requested file id from the Drive export endpoint.
func TestGoogleDriveDriverAdapterFetchesLiveDocument(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeProxyArtifactSyncConfig(t, runner, workspace)
	writeSingleProxyArtifactConnection(t, runner, "project-1")

	original := httpRequestFn
	t.Cleanup(func() { httpRequestFn = original })
	var readURL, readAuth string
	httpRequestFn = func(_ context.Context, _, url string, headers map[string]string, _ []byte) (int, []byte, error) {
		if strings.Contains(url, "oauth2.googleapis.com/token") {
			return 200, []byte(`{"access_token":"artifact-access-token"}`), nil
		}
		readURL = url
		readAuth = headers["Authorization"]
		return 200, []byte("driver document body"), nil
	}

	adapter := &googleDriveDriverAdapter{runner: runner}
	content, err := adapter.Fetch(context.Background(), "file-123")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if content != "driver document body" {
		t.Fatalf("content = %q, want %q", content, "driver document body")
	}
	if !strings.Contains(readURL, "/files/file-123/export") {
		t.Fatalf("read URL = %q, want it to target file-123's export endpoint", readURL)
	}
	if readAuth != "Bearer artifact-access-token" {
		t.Fatalf("Authorization header = %q, want the resolved access token", readAuth)
	}
}

// TestGoogleDriveDriverAdapterDegradesOnMissingCredential verifies Task-204's
// fail-closed constraint: no connected Google Drive account must return an
// error (which ContextSourceRegistry.Collect degrades to a warning,
// TestMcpDriverSourceAdapterErrorDegrades already covers that layer), never
// silently succeed or fall back to some other account's content.
func TestGoogleDriveDriverAdapterDegradesOnMissingCredential(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeProxyArtifactSyncConfig(t, runner, workspace)
	// Deliberately no writeSingleProxyArtifactConnection / saved credential —
	// no account is connected.

	adapter := &googleDriveDriverAdapter{runner: runner}
	if _, err := adapter.Fetch(context.Background(), "file-123"); err == nil {
		t.Fatal("expected an error with no connected Google Drive account, got nil")
	}
}

// TestGoogleDriveDriverAdapterNilRunnerDegrades verifies the adapter fails
// closed (not a panic) if it's ever constructed without a *Runner — a
// defensive guard, since SetMCPDriverAdapter is only ever called from
// AttachRunner with a non-nil r in production.
func TestGoogleDriveDriverAdapterNilRunnerDegrades(t *testing.T) {
	adapter := &googleDriveDriverAdapter{}
	if _, err := adapter.Fetch(context.Background(), "file-123"); err == nil {
		t.Fatal("expected an error with no runner configured, got nil")
	}
}

// TestSetMCPDriverAdapterWiresRegisteredSource verifies Task-204 T-4: calling
// SetMCPDriverAdapter on a registry that already has mcp.driver registered
// (as NewDefaultContextSourceRegistry always does) makes mcp.driver actually
// use the given adapter, end to end through Collect — not just store it
// somewhere inert.
func TestSetMCPDriverAdapterWiresRegisteredSource(t *testing.T) {
	r := NewDefaultContextSourceRegistry()
	r.SetMCPDriverAdapter(&fakeMCPDriverAdapter{content: "wired content unique-marker-abc"})

	sections, warnings := r.Collect(context.Background(), []string{"mcp.driver"}, FlowContextHints{
		MCPDriverRef: "driver-123",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	var found bool
	for _, s := range sections {
		if s.SourceType == "mcp.driver" && strings.Contains(s.Body, "wired content unique-marker-abc") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected mcp.driver section to use the wired adapter's content, got: %#v", sections)
	}
}

// TestSetMCPDriverAdapterNoOpWhenNotRegistered verifies the defensive no-op
// path: a registry that never registered mcp.driver (a bespoke test
// registry, unlike NewDefaultContextSourceRegistry) must not panic.
func TestSetMCPDriverAdapterNoOpWhenNotRegistered(t *testing.T) {
	r := NewContextSourceRegistry()
	r.SetMCPDriverAdapter(&fakeMCPDriverAdapter{content: "unused"})
}
