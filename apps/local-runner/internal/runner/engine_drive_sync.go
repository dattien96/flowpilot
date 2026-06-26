package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/contextsync"
)

const engineContextDriveFolder = "context-engine"

// chatSummaryFileName is the only context-engine file restored from Drive on bind:
// feature_history.ndjson and features.ndjson are rebuilt locally from git, but the
// chat-summary timeline is generated from chat transcripts and is not in git.
const chatSummaryFileName = "chat_summary.ndjson"

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

// FetchFile downloads a file from the project's `context-engine/` Drive folder.
// found is false (with nil error) when no such file exists.
func (d *engineDriveSyncer) FetchFile(ctx context.Context, remoteName string) ([]byte, bool, error) {
	file, err := findGoogleDriveFile(d.accessToken, d.contextFolderID, remoteName)
	if err != nil {
		return nil, false, err
	}
	if file.ID == "" {
		return nil, false, nil
	}
	data, err := downloadGoogleDriveFileByID(ctx, d.accessToken, file.ID)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
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

// restoreChatSummaryFromDrive pulls the project's `context-engine/chat_summary.ndjson`
// from Drive (if present) and merges it into the local chat-summary ledger. This
// is what carries prior-discussion continuity to a fresh machine: the commit and
// feature history rebuild from git on bind, but the chat-summary timeline can only
// come from Drive (or be re-derived from restored chats by the startup scan). The
// merge is additive — only `(run_id, feature_key)` pairs not already present
// locally are imported — so it never clobbers newer local summaries and composes
// with the upsert path. Best-effort: Drive being absent is "skipped", not an error.
func (s *InteractiveService) restoreChatSummaryFromDrive(projectID, dotFlowpilotDir string) (error, string) {
	syncer := s.buildEngineDriveSyncer(projectID)
	ds, ok := syncer.(*engineDriveSyncer)
	if !ok || ds == nil {
		return nil, "drive not connected; chat summaries left as-is"
	}
	data, found, err := ds.FetchFile(context.Background(), chatSummaryFileName)
	if err != nil {
		return err, "chat-summary restore failed (drive read)"
	}
	if !found || len(bytes.TrimSpace(data)) == 0 {
		return nil, "no chat summary in drive to restore"
	}
	ledger, err := changeledger.NewChatSummaryLedger(dotFlowpilotDir)
	if err != nil {
		return err, "chat-summary restore failed (open local ledger)"
	}
	imported, err := mergeChatSummaryNDJSON(ledger, data)
	if err != nil {
		return err, "chat-summary restore failed (merge)"
	}
	return nil, fmt.Sprintf("restored %d chat-summary entries from drive", imported)
}

// mergeChatSummaryNDJSON additively merges NDJSON chat-summary rows into ledger:
// a row is imported only when its (run_id, feature_key) pair is not already
// present locally. Returns the number of rows imported.
func mergeChatSummaryNDJSON(ledger *changeledger.ChatSummaryLedger, data []byte) (int, error) {
	existing := map[string]struct{}{}
	for _, fk := range ledger.ListFeatures() {
		rows, err := ledger.GetFeatureSummaries(fk)
		if err != nil {
			return 0, err
		}
		for _, r := range rows {
			existing[r.RunID+"\x00"+r.FeatureKey] = struct{}{}
		}
	}

	imported := 0
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var entry changeledger.ChatSummaryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // skip malformed rows; restore is best-effort
		}
		if entry.FeatureKey == "" || entry.Summary == "" {
			continue
		}
		key := entry.RunID + "\x00" + entry.FeatureKey
		if _, seen := existing[key]; seen {
			continue
		}
		if err := ledger.UpsertForRun(entry); err != nil {
			return imported, err
		}
		existing[key] = struct{}{}
		imported++
	}
	return imported, sc.Err()
}
