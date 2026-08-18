package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-551 — visible Drive sync/restore feedback: a "Syncing/Restoring n/m" line
// with an animated spinner in the right sidebar / session panel while a batch
// is in flight, plus an immediate start message so /sync never looks like a
// no-op. TUI-only.

func TestDriveIndicatorLine_IdleIsEmpty(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	if got := m.driveIndicatorLine(); got != "" {
		t.Fatalf("idle indicator=%q want empty", got)
	}
}

func TestDriveIndicatorLine_SyncShowsSpinnerAndProgress(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.driveSync = &driveSyncState{total: 5, done: 2}
	m.driveSyncFrame = 3
	line := m.driveIndicatorLine()
	if !strings.Contains(line, "Syncing") || !strings.Contains(line, "2/5") {
		t.Fatalf("sync indicator=%q", line)
	}
	if !strings.Contains(line, thinkingSpinner(3, false)) {
		t.Fatalf("sync indicator must carry the spinner glyph: %q", line)
	}
}

func TestDriveIndicatorLine_RestoreShowsSpinnerAndProgress(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.restoreBatch = &restoreState{total: 3, done: 1}
	line := m.driveIndicatorLine()
	if !strings.Contains(line, "Restoring") || !strings.Contains(line, "1/3") {
		t.Fatalf("restore indicator=%q", line)
	}
}

func TestSessionPanelLines_RenderDriveStatus(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.driveSync = &driveSyncState{total: 2, done: 0}
	m.refreshSessionPanel()
	m.sessionPanel.DriveStatus = m.driveIndicatorLine()
	joined := strings.Join(m.sessionPanel.lines(), "\n")
	if !strings.Contains(joined, "Drive:") || !strings.Contains(joined, "Syncing 0/2") {
		t.Fatalf("panel lines=%q", joined)
	}
}

func TestRightSidebar_ShowsDriveLineWhileSync(t *testing.T) {
	m := sidebarFlowModel("codex", 120)
	m.width, m.height, m.fullWidth = 120, 24, 120
	m.driveSync = &driveSyncState{total: 3, done: 1}
	side := strings.Join(m.renderRightSidebar(m.height), "\n")
	if !strings.Contains(side, "Syncing 1/3") {
		t.Fatalf("sidebar must show Drive progress:\n%s", side)
	}
}

func TestStartSyncBatch_PrintsStartMessage(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.project = &client.Project{ID: "p1"}
	cmd := m.startSyncBatch([]string{"run-1"}, "p1")
	if cmd == nil || m.driveSync == nil || m.driveSync.total != 1 {
		t.Fatalf("cmd=%v driveSync=%+v", cmd, m.driveSync)
	}
	if !strings.Contains(m.View(), "Syncing run-1 to Drive") {
		t.Fatal("startSyncBatch must print a visible start message")
	}
}

func TestStartSyncBatch_AllPrintsCount(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1"}
	m.startSyncBatch([]string{"run-1", "run-2", "run-3"}, "p1")
	if !strings.Contains(m.View(), "Syncing 3 chats to Drive") {
		t.Fatal("bulk start must announce the batch size")
	}
}

func TestStartRestoreBatch_PrintsStartMessage(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1"}
	cmd := m.startRestoreBatch([]string{"m1:r1"}, "p1", "", true)
	if cmd == nil || m.restoreBatch == nil || !m.restoreBatch.openAfter {
		t.Fatalf("cmd=%v restoreBatch=%+v", cmd, m.restoreBatch)
	}
	if !strings.Contains(m.View(), "Restoring m1:r1 from Drive") {
		t.Fatal("restore start must print a visible start message")
	}
}

func TestDriveSyncTicker_StartsOnCursorTickWhileBatchActive(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.driveSync = &driveSyncState{total: 2, done: 0}
	m2, cmd := m.Update(cursorTickMsg{})
	am := m2.(*AppModel)
	if cmd == nil || !am.driveSyncTickerActive {
		t.Fatalf("cursor tick must start the drive ticker: active=%v cmd=%v", am.driveSyncTickerActive, cmd)
	}
}

func TestDriveSyncTickMsg_SelfCancelsWhenIdle(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.driveSyncTickerActive = true
	m2, cmd := m.Update(driveSyncTickMsg{})
	am := m2.(*AppModel)
	if cmd != nil || am.driveSyncTickerActive {
		t.Fatal("drive tick must self-cancel when no batch is active")
	}
}

func TestDriveSyncTickMsg_ReschedulesWhileBatchActive(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.driveSync = &driveSyncState{total: 2, done: 0}
	m2, cmd := m.Update(driveSyncTickMsg{})
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("drive tick must reschedule while a batch is active")
	}
	if am.driveSyncFrame == 0 {
		t.Fatal("drive tick must advance the spinner frame")
	}
}
