package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-553 — the Drive sync badge must be impossible to miss on every surface:
// the history/open/resume picker detail, the /history dump, the /sync picker,
// and the session panel right sidebar (below the Run line) for the open chat.
// The picker badge moved to the START of the detail line (right after the #N
// index) so paintRow truncation can never cut it.

func TestFilterHistorySuggestionsWithRemote_BadgeStartsDetail(t *testing.T) {
	items := []client.RunHistoryItem{
		{RunID: "run-107774", LastPrompt: "thế tóm lại flow có bị hang không", Status: "completed", RunKind: "workflow"},
	}
	remote := []client.RemoteChatSessionSummary{remoteRow("mch_c3ab", "run-107774")}
	sugg := filterHistorySuggestionsWithRemote("/open ", items, remote)
	if len(sugg) != 1 {
		t.Fatalf("suggestions=%+v", sugg)
	}
	d := sugg[0].detail
	badgeIdx := strings.Index(d, "synced")
	if badgeIdx < 0 {
		t.Fatalf("detail missing synced badge: %q", d)
	}
	// Badge must sit right after "#1 · " — before kind/status/date/title — so
	// even a heavily truncated picker line still shows it.
	if !strings.HasPrefix(d, "#1 · synced") {
		t.Fatalf("badge must start the detail right after #N: %q", d)
	}
}

func TestFilterHistorySuggestions_OpenAndResumeCarryBadge(t *testing.T) {
	items := []client.RunHistoryItem{
		{RunID: "run-107774", LastPrompt: "title one", Status: "completed", RunKind: "workflow", SyncStatus: "synced"},
		{RunID: "run-b", LastPrompt: "title two", Status: "completed", RunKind: "chat"},
	}
	for _, prefix := range []string{"/history ", "/open ", "/resume "} {
		sugg := filterHistorySuggestions(prefix, items)
		if len(sugg) != 2 {
			t.Fatalf("%s suggestions=%+v", prefix, sugg)
		}
		if !strings.Contains(sugg[0].detail, "synced") {
			t.Fatalf("%s picker missing synced badge: %q", prefix, sugg[0].detail)
		}
		if strings.Contains(sugg[1].detail, "synced") {
			t.Fatalf("%s badge leaked to unsynced row: %q", prefix, sugg[1].detail)
		}
	}
}

func TestFormatChatList_BadgeEarlyInDumpLine(t *testing.T) {
	items := []client.RunHistoryItem{
		{RunID: "run-107774", LastPrompt: "title", Status: "completed", RunKind: "workflow", SyncStatus: "synced"},
		{RunID: "run-c", LastPrompt: "local", Status: "completed", RunKind: "chat"},
	}
	dump := formatChatList(items)
	idxSynced := strings.Index(dump, "(synced)")
	idxTitle := strings.Index(dump, "title")
	if idxSynced < 0 {
		t.Fatalf("dump missing (synced):\n%s", dump)
	}
	if idxTitle < 0 || idxSynced > idxTitle {
		t.Fatalf("dump badge must precede the title so it is never wrapped away:\n%s", dump)
	}
	if strings.Contains(dump, "  ()") {
		t.Fatalf("empty badge rendered:\n%s", dump)
	}
}

func TestOpenChatDriveBadge_FromCachedHistory(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-107774", Status: "completed", RunKind: "workflow", SyncStatus: "synced"},
	}
	m.runHandle = &client.RunHandle{RunID: "run-107774"}
	if got := m.openChatDriveBadge(); got != "synced" {
		t.Fatalf("open badge got=%q want synced", got)
	}
}

func TestOpenChatDriveBadge_ReconcilesViaRemote(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{{RunID: "run-107774", Status: "completed", RunKind: "workflow"}}
	m.remoteChatList = []client.RemoteChatSessionSummary{remoteRow("mch_c3ab", "run-107774")}
	m.runHandle = &client.RunHandle{RunID: "run-107774"}
	if got := m.openChatDriveBadge(); got != "synced" {
		t.Fatalf("remote reconcile got=%q want synced", got)
	}
}

func TestOpenChatDriveBadge_EmptyForUnsyncedOrNoRun(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	if got := m.openChatDriveBadge(); got != "" {
		t.Fatalf("no runHandle got=%q", got)
	}
	m.chatList = []client.RunHistoryItem{{RunID: "run-b", Status: "completed", RunKind: "chat"}}
	m.runHandle = &client.RunHandle{RunID: "run-b"}
	if got := m.openChatDriveBadge(); got != "" {
		t.Fatalf("unsynced open chat got=%q", got)
	}
}

func TestSessionPanel_LinesShowsDriveBadgeBelowRun(t *testing.T) {
	p := sessionInfoPanel{RunID: "run-107774", DriveBadge: "synced"}
	lines := p.lines()
	var runIdx, driveIdx int = -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "Run: ") {
			runIdx = i
		}
		if l == "Drive: synced" {
			driveIdx = i
		}
	}
	if runIdx < 0 {
		t.Fatalf("missing Run line: %+v", lines)
	}
	if driveIdx != runIdx+1 {
		t.Fatalf("Drive badge must sit directly below Run: run=%d drive=%d lines=%+v", runIdx, driveIdx, lines)
	}
}

func TestSessionPanel_LinesNoDriveBadgeWhenEmpty(t *testing.T) {
	p := sessionInfoPanel{RunID: "run-b"}
	for _, l := range p.lines() {
		if strings.HasPrefix(l, "Drive: ") {
			t.Fatalf("empty badge must not render: %q", l)
		}
	}
}

func TestRenderRightSidebar_ShowsOpenChatBadge(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 120
	m.height = 30
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	enableSidebarForTest(m)
	m.chatList = []client.RunHistoryItem{{RunID: "run-107774", Status: "completed", RunKind: "workflow", SyncStatus: "synced"}}
	m.runHandle = &client.RunHandle{RunID: "run-107774"}
	m.sessionPanel.RunID = "run-107774"
	if !m.useRightSidebar() {
		t.Fatal("wide expanded panel must engage right sidebar")
	}
	out := m.renderRightSidebar(30)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "Drive: synced") {
		t.Fatalf("sidebar must show open-chat badge:\n%s", joined)
	}
	runPos := strings.Index(joined, "Run:")
	drivePos := strings.Index(joined, "Drive: synced")
	if runPos < 0 || drivePos < runPos {
		t.Fatalf("badge must appear after Run line:\n%s", joined)
	}
}
