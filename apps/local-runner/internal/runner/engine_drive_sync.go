package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/contextsync"
)

const engineContextDriveFolder = "context-engine"

// engineDriveSyncer implements contextsync.DriveSyncer using the runner's
// existing Google Drive helpers. Files are uploaded to a flat `context-engine/`
// folder inside the project's chat-sync Drive root (CP-35 P-8).
type engineDriveSyncer struct {
	accessToken     string
	contextFolderID string
}

func (d *engineDriveSyncer) SyncFile(_ context.Context, localPath, remoteName string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	_, err = upsertGoogleDriveFile(d.accessToken, d.contextFolderID, remoteName, data, driveContentTypeFor(localPath), nil)
	return err
}

func driveContentTypeFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "application/json"
	case ".ndjson":
		return "application/x-ndjson"
	default:
		return "application/octet-stream"
	}
}

// buildEngineDriveSyncer resolves the Drive root folder for the project, ensures
// the `context-engine/` sub-folder exists, and returns a ready-to-use syncer.
// Returns nil when Drive is not configured or the project has no connected folder —
// callers should treat nil as "skip Drive sync" (not an error).
func (s *InteractiveService) buildEngineDriveSyncer(projectID string) contextsync.DriveSyncer {
	rootFolderID, accessToken, apiErr := s.ensureChatSessionDriveRoot(projectID)
	if apiErr != nil {
		return nil
	}

	contextFolderID, err := ensureGoogleDriveFolderPath(accessToken, rootFolderID, []string{engineContextDriveFolder})
	if err != nil {
		return nil
	}

	return &engineDriveSyncer{
		accessToken:     accessToken,
		contextFolderID: contextFolderID,
	}
}

func (s *InteractiveService) syncContextEngineFiles(projectID string, dotFlowpilotDir string) (error, string) {
	store, storeErr := contextsync.NewEngineStore(dotFlowpilotDir)
	if storeErr != nil {
		return storeErr, filepath.Join(dotFlowpilotDir, "manifest.json")
	}
	if manifestErr := contextsync.WriteManifest(store); manifestErr != nil {
		return manifestErr, filepath.Join(dotFlowpilotDir, "manifest.json")
	}
	syncer := s.buildEngineDriveSyncer(projectID)
	result := contextsync.SyncSharedFiles(context.Background(), store, syncer)
	if len(result.Synced) > 0 {
		return nil, fmt.Sprintf("manifest ok; %d files synced to drive, %d skipped", len(result.Synced), len(result.Skipped))
	}
	return nil, fmt.Sprintf("manifest ok; %d files skipped (drive not connected)", len(result.Skipped))
}
