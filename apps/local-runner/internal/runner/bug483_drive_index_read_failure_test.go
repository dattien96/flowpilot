package runner

// BUG-483 (CP-59): the sessions.ndjson read-merge-write must never treat an
// unreadable existing index as a valid empty baseline — a transient download
// failure used to silently overwrite the remote index, dropping every other
// device's synced rows. Corrupt lines are preserved verbatim (lossless merge)
// and a concurrent writer's rows are merged forward, never clobbered.

import (
	"context"
	"strings"
	"testing"
)

func bug483IndexRecord(machine, run, prompt string) chatSessionDriveIndexRecord {
	return chatSessionDriveIndexRecord{
		RunID:           run,
		ProjectID:       "project-1",
		ProviderKey:     string(ProviderKeyCodex),
		RunKind:         "chat",
		SourceMachineID: machine,
		SourceRunID:     run,
		LastPrompt:      prompt,
		SyncedAt:        "2026-09-25T01:00:00Z",
		ManifestPath:    "chat-sessions/runs/" + machine + "/" + run + "/manifest.json",
	}
}

// plantIndexFile writes a sessions.ndjson blob into the fake Drive at the
// canonical index path and returns the file id.
func plantIndexFile(t *testing.T, api *fakeChatDriveAPI, rootID, content string) string {
	t.Helper()
	indexFolder := ""
	for _, f := range api.files {
		if f.Name == "_index" && f.MimeType == googleDriveFolderMimeType {
			indexFolder = f.ID
		}
	}
	if indexFolder == "" {
		// Ensure the folder exists the same way prod does.
		id, err := ensureGoogleDriveFolderPath("tok", rootID, []string{"chat-sessions", "_index"})
		if err != nil {
			t.Fatalf("ensure index folder: %v", err)
		}
		indexFolder = id
	}
	file, err := upsertGoogleDriveFile("tok", indexFolder, "sessions.ndjson", []byte(content), "application/x-ndjson", nil)
	if err != nil {
		t.Fatalf("plant index file: %v", err)
	}
	return file.ID
}

// A download failure on the EXISTING index must abort the merge — never
// upsert over an authority we could not read.
func TestBUG483_IndexDownloadFailureAbortsMerge(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	original := `{"source_machine_id":"mch_A","source_run_id":"run-A","project_id":"project-1","manifest_path":"p1","synced_at":"t"}` + "\n" +
		`{"source_machine_id":"mch_B","source_run_id":"run-B","project_id":"project-1","manifest_path":"p2","synced_at":"t"}` + "\n"
	plantIndexFile(t, api, "drive-root", original)

	api.mu.Lock()
	api.failDownload = true
	api.mu.Unlock()

	_, apiErr := svc.mergeAndUpsertChatSessionDriveIndexLocked(context.Background(), "tok", "drive-root",
		[]chatSessionDriveIndexRecord{bug483IndexRecord("mch_C", "run-C", "new")})
	if apiErr == nil {
		t.Fatal("merge must abort when the existing index cannot be downloaded")
	}
	// The remote index must be untouched.
	idxFile, ok := driveFileByName(api, "sessions.ndjson")
	if !ok {
		t.Fatal("index file vanished")
	}
	if string(idxFile.Content) != original {
		t.Fatalf("index was overwritten on download failure:\n%s", idxFile.Content)
	}
}

// A missing index is the ONLY case where an empty baseline is valid.
func TestBUG483_IndexNotFoundCreatesFreshIndex(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	_, apiErr := svc.mergeAndUpsertChatSessionDriveIndexLocked(context.Background(), "tok", "drive-root",
		[]chatSessionDriveIndexRecord{bug483IndexRecord("mch_A", "run-A", "p")})
	if apiErr != nil {
		t.Fatalf("fresh-index merge failed: %v", apiErr)
	}
	idxFile, ok := driveFileByName(api, "sessions.ndjson")
	if !ok {
		t.Fatal("index not created")
	}
	records := parseChatSessionDriveIndex(idxFile.Content)
	if len(records) != 1 || records[0].SourceRunID != "run-A" {
		t.Fatalf("fresh index rows wrong: %+v", records)
	}
}

// Unparseable/identity-less lines in the existing index are preserved
// verbatim through a merge — the merge must be lossless, never dropping
// remote rows it cannot interpret.
func TestBUG483_CorruptIndexLinesPreservedThroughMerge(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	original := "{corrupt-not-json\n" +
		`{"source_machine_id":"mch_A","source_run_id":"run-A","project_id":"project-1","manifest_path":"p1","synced_at":"t"}` + "\n" +
		`{"json_ok_but_no_identity":true}` + "\n"
	plantIndexFile(t, api, "drive-root", original)

	_, apiErr := svc.mergeAndUpsertChatSessionDriveIndexLocked(context.Background(), "tok", "drive-root",
		[]chatSessionDriveIndexRecord{bug483IndexRecord("mch_B", "run-B", "p")})
	if apiErr != nil {
		t.Fatalf("merge failed: %v", apiErr)
	}
	idxFile, _ := driveFileByName(api, "sessions.ndjson")
	got := string(idxFile.Content)
	if !strings.Contains(got, "{corrupt-not-json") {
		t.Fatalf("malformed line dropped by merge:\n%s", got)
	}
	if !strings.Contains(got, `"json_ok_but_no_identity":true`) {
		t.Fatalf("identity-less line dropped by merge:\n%s", got)
	}
	records := parseChatSessionDriveIndex(idxFile.Content)
	if len(records) != 2 {
		t.Fatalf("valid rows = %d want 2 (run-A + run-B)", len(records))
	}
}

// A concurrent device writing between our read and our upsert must not lose
// rows: the writer re-reads before overwriting and merges onto the fresh
// baseline instead of last-write-wins clobbering.
func TestBUG483_ConcurrentWriterMergedForward(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	original := `{"source_machine_id":"mch_A","source_run_id":"run-A","project_id":"project-1","manifest_path":"p1","synced_at":"t"}` + "\n"
	indexFileID := plantIndexFile(t, api, "drive-root", original)

	// After the first index download, a "second device" appends its row —
	// simulated by rewriting the file content inside the download hook.
	var fired bool
	api.mu.Lock()
	api.onDownload = func(id string) {
		if fired || id != indexFileID {
			return
		}
		fired = true
		f := api.files[indexFileID]
		f.Content = append(append([]byte(nil), original...),
			[]byte(`{"source_machine_id":"mch_X","source_run_id":"run-X","project_id":"project-1","manifest_path":"px","synced_at":"t2"}`+"\n")...)
		api.files[indexFileID] = f
	}
	api.mu.Unlock()

	_, apiErr := svc.mergeAndUpsertChatSessionDriveIndexLocked(context.Background(), "tok", "drive-root",
		[]chatSessionDriveIndexRecord{bug483IndexRecord("mch_B", "run-B", "ours")})
	if apiErr != nil {
		t.Fatalf("merge failed: %v", apiErr)
	}
	idxFile, _ := driveFileByName(api, "sessions.ndjson")
	records := parseChatSessionDriveIndex(idxFile.Content)
	got := map[string]bool{}
	for _, r := range records {
		got[r.SourceRunID] = true
	}
	for _, want := range []string{"run-A", "run-X", "run-B"} {
		if !got[want] {
			t.Fatalf("row %s lost — concurrent writer clobbered. index:\n%s", want, idxFile.Content)
		}
	}
}
