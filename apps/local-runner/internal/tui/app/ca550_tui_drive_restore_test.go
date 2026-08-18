package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// G3 (CA-550): /restore pull-from-Drive parity (Desktop Navigator "Restore
// all"). Single restore opens the restored chat; /restore all stays silent and
// refreshes both lists; cwd_remap_required retries once with the project path.

func remoteItem(machine, runID, prompt, status string) client.RemoteChatSessionSummary {
	return client.RemoteChatSessionSummary{
		SourceMachineID: machine,
		SourceRunID:     runID,
		RunID:           "local-" + runID,
		LastPrompt:      prompt,
		Status:          status,
	}
}

func TestRemoteSourceKey(t *testing.T) {
	if got := remoteSourceKey(client.RemoteChatSessionSummary{SourceMachineID: "m1", SourceRunID: "r1"}); got != "m1:r1" {
		t.Fatalf("got %q", got)
	}
}

func TestFilterRestoreSuggestions_AllRowPlusFiltering(t *testing.T) {
	remote := []client.RemoteChatSessionSummary{
		remoteItem("m1", "r1", "fix login bug", "completed"),
		remoteItem("m2", "r2", "ship android", "completed"),
	}
	all := filterRestoreSuggestions("/restore ", remote)
	if len(all) != 3 || all[0].value != "all" || all[0].kind != "restore" || all[0].slash != "/restore" {
		t.Fatalf("all row missing or miswired: %+v", all)
	}
	q := filterRestoreSuggestions("/restore ship", remote)
	if len(q) != 2 || q[0].value != "all" || q[1].value != "m2:r2" {
		t.Fatalf("query filter wrong: %+v", q)
	}
	if filterRestoreSuggestions("/sync ", remote) != nil {
		t.Fatal("non-/restore input must not open the restore picker")
	}
	if filterRestoreSuggestions("/restore ", nil) != nil {
		t.Fatal("empty remote index must return no rows")
	}
}

func TestSuggestionAcceptValue_RestoreWritesSlashPlusKey(t *testing.T) {
	if got := suggestionAcceptValue(suggestItem{kind: "restore", value: "m1:r1", slash: "/restore"}); got != "/restore m1:r1" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveRestoreTarget(t *testing.T) {
	remote := []client.RemoteChatSessionSummary{
		remoteItem("m1", "r1", "fix login bug", "completed"),
		remoteItem("m2", "r2", "ship android", "completed"),
	}
	cases := []struct {
		arg  string
		want string
		err  bool
	}{
		{"1", "m1:r1", false},
		{"2", "m2:r2", false},
		{"m1:r1", "m1:r1", false},
		{"local-r2", "m2:r2", false},
		{"3", "", true},
		{"0", "", true},
		{"nomatch", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		got, err := resolveRestoreTarget([]string{tc.arg}, remote)
		if tc.err != (err != nil) || got != tc.want {
			t.Errorf("arg=%q got=%q err=%v want=%q err=%v", tc.arg, got, err, tc.want, tc.err)
		}
	}
}

func TestFormatRemoteChatList(t *testing.T) {
	if got := formatRemoteChatList(nil); !strings.Contains(got, "No Drive-backed chats") {
		t.Fatalf("empty dump=%q", got)
	}
	dump := formatRemoteChatList([]client.RemoteChatSessionSummary{remoteItem("m1", "r1", "fix login bug", "completed")})
	if !strings.Contains(dump, "fix login bug") || !strings.Contains(dump, "m1:r1") {
		t.Fatalf("dump=%q", dump)
	}
}

func TestRunRestoreDispatch_BareDumpUsesCacheWhenLoaded(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj"}
	m.remoteChatList = []client.RemoteChatSessionSummary{remoteItem("m1", "r1", "fix login bug", "completed")}
	am, cmd := m.runRestoreDispatch(nil)
	if cmd != nil {
		t.Fatal("cached bare /restore must not fetch")
	}
	if !strings.Contains(am.(*AppModel).View(), "Drive-backed chats") {
		t.Fatal("bare /restore must dump the index into the transcript")
	}
}

func TestRunRestoreDispatch_BareFetchesWhenCacheEmpty(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj"}
	am, cmd := m.runRestoreDispatch(nil)
	if cmd == nil || am == nil {
		t.Fatal("empty cache must trigger a remote fetch")
	}
}

func TestRunRestoreDispatch_SingleOpensAfterAndAllBatch(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj", Path: `C:\working\p1`}
	m.remoteChatList = []client.RemoteChatSessionSummary{
		remoteItem("m1", "r1", "fix login bug", "completed"),
		remoteItem("m2", "r2", "ship android", "completed"),
	}
	_, cmd := m.runRestoreDispatch([]string{"2"})
	if cmd == nil {
		t.Fatal("single restore must start a batch")
	}
	if m.restoreBatch == nil || !m.restoreBatch.openAfter || m.restoreBatch.total != 1 || m.restoreBatch.projectID != "p1" {
		t.Fatalf("restoreBatch=%+v", m.restoreBatch)
	}
	if m.restoreBatch.cwd != `C:\working\p1` {
		t.Fatalf("cwd not pre-bound from project path: %+v", m.restoreBatch)
	}

	_, cmd = m.runRestoreDispatch([]string{"all"})
	if cmd == nil {
		t.Fatal("restore all must start a batch")
	}
	if m.restoreBatch == nil || m.restoreBatch.openAfter || m.restoreBatch.total != 2 || len(m.restoreBatch.queue) != 1 {
		t.Fatalf("restoreBatch=%+v", m.restoreBatch)
	}
}

func TestRunRestoreDispatch_NoProjectIsHarmless(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	am, cmd := m.runRestoreDispatch(nil)
	if cmd != nil || am == nil {
		t.Fatalf("no-project /restore must be a no-op: cmd=%v", cmd)
	}
}

func TestRestoreBatch_SingleFailureDoesNotAbortBatch(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj"}
	m.restoreBatch = &restoreState{
		queue:     []string{"m2:r2"},
		projectID: "p1",
		total:     2,
		openAfter: false,
	}
	m2, cmd := m.Update(RestoreBatchMsg{SourceKey: "m1:r1", Err: &client.APIError{Status: 409, Code: "google_drive_not_connected", Message: "not connected"}})
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("batch must continue after one failure")
	}
	if am.restoreBatch == nil || am.restoreBatch.done != 1 || am.restoreBatch.failed != 1 {
		t.Fatalf("restoreBatch=%+v", am.restoreBatch)
	}
}

func TestRestoreBatch_SingleSuccessOpensChat(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj"}
	m.restoreBatch = &restoreState{
		projectID: "p1",
		total:     1,
		openAfter: true,
	}
	m2, cmd := m.Update(RestoreBatchMsg{SourceKey: "m1:r1", Result: &client.ChatSessionRestoreResult{RunID: "restored-1", ProviderKey: "openai"}})
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("single restore must open the restored chat")
	}
	if am.restoreBatch != nil {
		t.Fatal("restoreBatch must reset after single restore")
	}
}

func TestRestoreBatch_AllDoneRefreshesLists(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj"}
	m.restoreBatch = &restoreState{
		projectID: "p1",
		total:     2,
		done:      2,
		openAfter: false,
	}
	m2, cmd := m.Update(RestoreBatchMsg{SourceKey: "m1:r1", Result: &client.ChatSessionRestoreResult{RunID: "restored-1"}})
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("restore-all completion must refresh the chat + remote lists")
	}
	if am.restoreBatch != nil {
		t.Fatal("restoreBatch must reset after restore all")
	}
}

func TestFormatRestoreErr_Hints(t *testing.T) {
	cases := []struct {
		code string
		want string
	}{
		{"google_drive_not_connected", "/settings"},
		{"session_unavailable", "not available on Drive"},
		{"sync_remote_not_found", "no longer exists on Drive"},
		{"sync_integrity_failed", "failed integrity"},
	}
	for _, tc := range cases {
		if got := formatRestoreErr("m1:r1", &client.APIError{Status: 422, Code: tc.code, Message: "boom"}); !strings.Contains(got, tc.want) {
			t.Errorf("code=%s got=%q want contain %q", tc.code, got, tc.want)
		}
	}
}

func TestCmdMaybePrefetchHistory_TriggersOnRestore(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/restore "
	if m.cmdMaybePrefetchHistory() == nil {
		t.Fatal("/restore  must prefetch the chat list")
	}
}
