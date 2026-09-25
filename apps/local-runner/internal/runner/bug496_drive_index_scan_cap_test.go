package runner

// BUG-496: mergeChatSessionDriveIndex and parseChatSessionDriveIndex used
// bufio.Scanner with the default 64KiB cap and never checked Err(). A
// single oversized line in a remote-uploaded index truncated the scan —
// the merge path then lost the oversized `preserved` row AND every record
// after it, clobbering rows other devices wrote.

import (
	"strings"
	"testing"
)

func bug496IndexBlob(t *testing.T) []byte {
	t.Helper()
	fat := strings.Repeat("x", 200*1024) // >64KiB malformed line
	return []byte(
		`{"source_machine_id":"m1","source_run_id":"run-1","project_id":"p1","manifest_path":"a","synced_at":"t1"}` + "\n" +
			fat + "\n" +
			`{"source_machine_id":"m2","source_run_id":"run-2","project_id":"p2","manifest_path":"b","synced_at":"t2"}` + "\n",
	)
}

func TestBUG496_MergePreservesRowsAfterOversizedLine(t *testing.T) {
	existing := bug496IndexBlob(t)
	replacement := chatSessionDriveIndexRecord{
		SourceMachineID: "m1", SourceRunID: "run-1",
		ProjectID: "p1", ManifestPath: "new", SyncedAt: "t3",
	}
	out := mergeChatSessionDriveIndex(existing, replacement)
	// The trailing remote row must survive the merge — pre-fix it was
	// silently dropped because the scanner stopped at the fat line.
	if !strings.Contains(string(out), `"source_machine_id":"m2"`) {
		t.Fatal("remote row after an oversized line was dropped by the merge")
	}
	if !strings.Contains(string(out), strings.Repeat("x", 200*1024)) {
		t.Fatal("oversized preserved row was dropped by the merge")
	}
}

func TestBUG496_ParseSeesRecordsAfterOversizedLine(t *testing.T) {
	records := parseChatSessionDriveIndex(bug496IndexBlob(t))
	var found bool
	for _, r := range records {
		if r.SourceMachineID == "m2" && r.SourceRunID == "run-2" {
			found = true
		}
	}
	if !found {
		t.Fatal("record after an oversized line is absent from the parsed index")
	}
}
