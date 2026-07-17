package runner

import (
	"context"
	"log"
	"strings"
)

// Drive layout (per project chat-sync root):
//
//	chat-sessions/dispatch/dispatch.ndjson
//
// Uploaded/downloaded alongside chat session sync so a new machine restores
// both session metadata and durable turn dispatch state.

const driveDispatchRelativePath = "chat-sessions/dispatch/dispatch.ndjson"

// syncDispatchLogToDrive uploads the local per-project dispatch.ndjson to Drive.
// Best-effort: missing local log is a no-op; failures are logged and returned as apiErr
// only when the caller wants fail-closed (we use best-effort from chat sync).
func (s *InteractiveService) syncDispatchLogToDrive(ctx context.Context, projectID, accessToken, rootFolderID string) error {
	_ = ctx
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || s == nil {
		return nil
	}
	hub, ok := s.dispatchStore.(*multiProjectDispatchStore)
	if !ok || hub == nil {
		return nil
	}
	raw, err := hub.ExportProjectLog(projectID)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	dirID, err := ensureGoogleDriveFolderPath(accessToken, rootFolderID, []string{"chat-sessions", "dispatch"})
	if err != nil {
		return err
	}
	if _, err := upsertGoogleDriveFile(accessToken, dirID, "dispatch.ndjson", raw, "application/x-ndjson", nil); err != nil {
		return err
	}
	log.Printf("[chat-sync] dispatch log uploaded project=%s bytes=%d", projectID, len(raw))
	return nil
}

// restoreDispatchLogFromDrive downloads the project dispatch log and imports it
// into .flowpilot/chats/<project_id>/dispatch.ndjson.
func (s *InteractiveService) restoreDispatchLogFromDrive(ctx context.Context, projectID, accessToken, rootFolderID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || s == nil {
		return nil
	}
	hub, ok := s.dispatchStore.(*multiProjectDispatchStore)
	if !ok || hub == nil {
		// Open hub on demand if only memory store was set in tests.
		return nil
	}
	file, err := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, driveDispatchRelativePath)
	if err != nil {
		// No remote dispatch log yet — fine for projects synced before this feature.
		log.Printf("[chat-sync] no remote dispatch log project=%s: %v", projectID, err)
		return nil
	}
	raw, err := downloadGoogleDriveFileByID(ctx, accessToken, file.ID)
	if err != nil {
		return err
	}
	if err := hub.ImportProjectLog(projectID, raw); err != nil {
		return err
	}
	log.Printf("[chat-sync] dispatch log restored project=%s bytes=%d", projectID, len(raw))
	return nil
}
