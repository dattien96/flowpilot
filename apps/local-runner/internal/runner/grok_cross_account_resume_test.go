package runner

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Task-210 Option B: Grok cross-account resume (locate/relocate dir + promote
// real ACP session id + session/load) mirrors Codex/Claude SD-14 behavior.

func writeGrokSessionTree(t *testing.T, grokHome, cwd, sessionID string, files map[string]string) string {
	t.Helper()
	dir := grokSessionDirPath(grokHome, cwd, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll session dir: %v", err)
	}
	if files == nil {
		files = map[string]string{"chat_history.jsonl": `{"type":"assistant","content":"hi"}` + "\n"}
	}
	if _, ok := files["chat_history.jsonl"]; !ok {
		files["chat_history.jsonl"] = `{"type":"assistant","content":"hi"}` + "\n"
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	return dir
}

func TestLocateSessionFileGrokFindsSessionDirectory(t *testing.T) {
	home := t.TempDir()
	cwd := "/repo"
	dir := writeGrokSessionTree(t, home, cwd, "sess-a", nil)
	got, found := LocateSessionFile(ProviderKeyGrok, home, "sess-a", cwd)
	if !found {
		t.Fatal("expected Grok session directory to be found")
	}
	if got != dir {
		t.Fatalf("LocateSessionFile = %q, want %q", got, dir)
	}
}

func TestLocateSessionFileGrokRejectsSyntheticThread(t *testing.T) {
	home := t.TempDir()
	cwd := "/repo"
	writeGrokSessionTree(t, home, cwd, "thread-371", nil)
	if _, found := LocateSessionFile(ProviderKeyGrok, home, "thread-371", cwd); found {
		t.Fatal("synthetic thread-* must not resolve as a Grok session artifact")
	}
}

func TestRelocateSessionFileGrokCopiesDirectoryTree(t *testing.T) {
	srcHome := t.TempDir()
	targetHome := t.TempDir()
	cwd := "/repo"
	files := map[string]string{
		"chat_history.jsonl":  "history\n",
		"events.jsonl":        "events\n",
		"prompt_context.json": `{"k":1}`,
		"signals.json":        `{}`,
		"summary.json":        `{"s":1}`,
		"system_prompt.txt":   "sys",
		"updates.jsonl":       "u\n",
	}
	src := writeGrokSessionTree(t, srcHome, cwd, "sess-a", files)
	dst, err := RelocateSessionFile(ProviderKeyGrok, src, targetHome, "sess-a", cwd)
	if err != nil {
		t.Fatalf("RelocateSessionFile: %v", err)
	}
	want := grokSessionDirPath(targetHome, cwd, "sess-a")
	if dst != want {
		t.Fatalf("dst = %q, want %q", dst, want)
	}
	for name, body := range files {
		raw, readErr := os.ReadFile(filepath.Join(dst, name))
		if readErr != nil {
			t.Fatalf("missing relocated file %s: %v", name, readErr)
		}
		if string(raw) != body {
			t.Fatalf("file %s = %q, want %q", name, raw, body)
		}
	}
}

func TestRelocateSessionFileGrokExistingIdenticalIsNoop(t *testing.T) {
	srcHome := t.TempDir()
	targetHome := t.TempDir()
	cwd := "/repo"
	src := writeGrokSessionTree(t, srcHome, cwd, "sess-a", map[string]string{"chat_history.jsonl": "same\n"})
	// Pre-seed identical destination.
	_ = writeGrokSessionTree(t, targetHome, cwd, "sess-a", map[string]string{"chat_history.jsonl": "same\n"})
	dst, err := RelocateSessionFile(ProviderKeyGrok, src, targetHome, "sess-a", cwd)
	if err != nil {
		t.Fatalf("RelocateSessionFile identical: %v", err)
	}
	if dst != grokSessionDirPath(targetHome, cwd, "sess-a") {
		t.Fatalf("unexpected dst %q", dst)
	}
}

func TestRelocateSessionFileGrokExistingDifferentConflicts(t *testing.T) {
	srcHome := t.TempDir()
	targetHome := t.TempDir()
	cwd := "/repo"
	src := writeGrokSessionTree(t, srcHome, cwd, "sess-a", map[string]string{"chat_history.jsonl": "src\n"})
	_ = writeGrokSessionTree(t, targetHome, cwd, "sess-a", map[string]string{"chat_history.jsonl": "dst\n"})
	if _, err := RelocateSessionFile(ProviderKeyGrok, src, targetHome, "sess-a", cwd); err == nil {
		t.Fatal("expected conflict when destination differs")
	}
	raw, _ := os.ReadFile(filepath.Join(grokSessionDirPath(targetHome, cwd, "sess-a"), "chat_history.jsonl"))
	if string(raw) != "dst\n" {
		t.Fatalf("destination was overwritten: %q", raw)
	}
}

func TestEnsureProviderResumeHandleGrokPromotesLatestTurnLogSession(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	if err := store.AppendTurnLog(context.Background(), "run-1", turnLogLine{Kind: turnLogKindGrokSession, SessionID: "sess-old"}); err != nil {
		t.Fatalf("AppendTurnLog old: %v", err)
	}
	if err := store.AppendTurnLog(context.Background(), "run-1", turnLogLine{Kind: turnLogKindGrokSession, SessionID: "sess-new"}); err != nil {
		t.Fatalf("AppendTurnLog new: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id:                "run-1",
		providerKey:       ProviderKeyGrok,
		providerSessionID: "thread-371",
		workspaceCwd:      "/repo",
		runKind:           "chat",
	}
	if !svc.ensureProviderResumeHandle(rs, t.TempDir()) {
		t.Fatal("expected Grok handle promotion from turn log")
	}
	if rs.realProviderSessionID != "sess-new" {
		t.Fatalf("realProviderSessionID = %q, want sess-new", rs.realProviderSessionID)
	}
	if id := svc.resumeSessionID(rs); id != "sess-new" {
		t.Fatalf("resumeSessionID = %q, want sess-new", id)
	}
}

func TestPrepareCrossAccountResumeGrokCopiesStableAndTurnLogSessionDirs(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctAHome := filepath.Join(root, "acct-a")
	acctBHome := filepath.Join(root, "acct-b")
	cwd := "/repo"
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "grok", HomePath: acctAHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-b", ProviderKey: "grok", HomePath: acctBHome, SlotIndex: 2, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	if err := os.MkdirAll(acctAHome, 0o755); err != nil {
		t.Fatalf("MkdirAll acct-a: %v", err)
	}
	if err := os.MkdirAll(acctBHome, 0o755); err != nil {
		t.Fatalf("MkdirAll acct-b: %v", err)
	}
	writeGrokAuthFileFixture(t, acctAHome)
	writeGrokAuthFileFixture(t, acctBHome)
	writeGrokSessionTree(t, acctAHome, cwd, "sess-stable", map[string]string{
		"chat_history.jsonl": "stable\n",
		"events.jsonl":       "e1\n",
	})
	writeGrokSessionTree(t, acctAHome, cwd, "sess-extra", map[string]string{
		"chat_history.jsonl": "extra\n",
	})
	if err := store.AppendTurnLog(context.Background(), "run-1", turnLogLine{Kind: turnLogKindGrokSession, SessionID: "sess-extra"}); err != nil {
		t.Fatalf("AppendTurnLog: %v", err)
	}
	if err := store.AppendTurnLog(context.Background(), "run-1", turnLogLine{Kind: turnLogKindGrokSession, SessionID: "sess-stable"}); err != nil {
		t.Fatalf("AppendTurnLog stable: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyGrok,
		ProviderSessionID: "sess-stable",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  cwd,
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	// resumeRun seeds the run; ensureResumeReady is also used by startTurn on mismatch.
	// For modern real-id rows, resume uses skipsResumeSessionValidation only for thread-*,
	// so ensureResumeReady runs and should relocate.
	handle, apiErr := svc.resumeRun("run-1")
	if apiErr != nil {
		// resumeRun may skip validation for some cases; force ensureResumeReady via startTurn path.
		t.Logf("resumeRun returned %v; trying ensureResumeReady directly", apiErr)
	} else if handle.RunID != "run-1" {
		t.Fatalf("unexpected handle: %+v", handle)
	}

	svc.mu.Lock()
	rs := svc.runs["run-1"]
	svc.mu.Unlock()
	if rs == nil {
		// reconstruct manually if resume skipped loading somehow
		rs = &interactiveRun{
			id:                "run-1",
			providerKey:       ProviderKeyGrok,
			providerSessionID: "sess-stable",
			providerAccountID: "acct-a",
			workspaceCwd:      cwd,
			runKind:           "chat",
			status:            RunStatusCompleted,
			subs:              map[int64]chan ProviderEvent{},
			idempotency:       map[string]string{},
		}
		svc.mu.Lock()
		svc.runs[rs.id] = rs
		svc.mu.Unlock()
	}
	if apiErr := svc.ensureResumeReady(rs); apiErr != nil {
		t.Fatalf("ensureResumeReady: %#v", apiErr)
	}
	if _, found := LocateSessionFile(ProviderKeyGrok, acctBHome, "sess-stable", cwd); !found {
		t.Fatal("expected durable Grok session dir in target home")
	}
	if _, found := LocateSessionFile(ProviderKeyGrok, acctBHome, "sess-extra", cwd); !found {
		t.Fatal("expected turn-log Grok session dir in target home")
	}
	state, found, err := store.GetProviderSession(context.Background(), "run-1")
	if err != nil || !found {
		t.Fatalf("GetProviderSession = (%v, %v, %v)", state, found, err)
	}
	if state.ProviderAccountID != "acct-b" {
		t.Fatalf("ProviderAccountID = %q, want acct-b", state.ProviderAccountID)
	}
}

func TestStartTurnGrokCrossAccountLegacyThreadPromotesCopiesAndLoads(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctAHome := filepath.Join(root, "acct-a")
	acctBHome := filepath.Join(root, "acct-b")
	cwd := filepath.Join(root, "workspace")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll workspace: %v", err)
	}
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "grok", HomePath: acctAHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-b", ProviderKey: "grok", HomePath: acctBHome, SlotIndex: 2, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	if err := os.MkdirAll(acctAHome, 0o755); err != nil {
		t.Fatalf("MkdirAll acct-a: %v", err)
	}
	if err := os.MkdirAll(acctBHome, 0o755); err != nil {
		t.Fatalf("MkdirAll acct-b: %v", err)
	}
	writeGrokAuthFileFixture(t, acctAHome)
	writeGrokAuthFileFixture(t, acctBHome)
	// run-370 shape: durable handle is synthetic; real id only in turn log + disk.
	writeGrokSessionTree(t, acctAHome, cwd, "019f60f6-dca6-7bf1-9374-c419b9df7d86", map[string]string{
		"chat_history.jsonl": "acc1 history\n",
		"events.jsonl":       "events\n",
	})
	if err := store.AppendTurnLog(context.Background(), "run-370", turnLogLine{
		Kind: turnLogKindGrokSession, SessionID: "019f60f6-dca6-7bf1-9374-c419b9df7d86",
	}); err != nil {
		t.Fatalf("AppendTurnLog: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-370",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyGrok,
		ProviderSessionID: "thread-371",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  cwd,
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	var (
		mu       sync.Mutex
		gotReqID string
		started  = make(chan struct{}, 1)
	)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyGrok,
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return &keyedFakeAdapter{
				key: ProviderKeyGrok,
				fn: func(_ context.Context, req TurnRequest, b TurnBridge) error {
					mu.Lock()
					gotReqID = req.ProviderSessionID
					mu.Unlock()
					select {
					case started <- struct{}{}:
					default:
					}
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok under acc2"})
					return nil
				},
			}
		},
	})
	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"

	// Load run into memory with stored account still pointing at acct-a.
	if _, apiErr := svc.resumeRun("run-370"); apiErr != nil {
		// Synthetic thread skips ensureResumeReady on history open — reconstruct manually.
		svc.mu.Lock()
		svc.runs["run-370"] = &interactiveRun{
			id:                "run-370",
			projectID:         "project-1",
			providerKey:       ProviderKeyGrok,
			providerSessionID: "thread-371",
			providerAccountID: "acct-a",
			workspaceCwd:      cwd,
			runKind:           "chat",
			status:            RunStatusCompleted,
			createdAt:         time.Now().UTC().Format(time.RFC3339Nano),
			updatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
			subs:              map[int64]chan ProviderEvent{},
			idempotency:       map[string]string{},
		}
		svc.mu.Unlock()
	}
	// Ensure the in-memory run still looks like the legacy stored row.
	svc.mu.Lock()
	if rs := svc.runs["run-370"]; rs != nil {
		rs.providerAccountID = "acct-a"
		rs.providerSessionID = "thread-371"
		rs.realProviderSessionID = ""
		rs.workspaceCwd = cwd
		rs.runKind = "chat"
		rs.status = RunStatusIdle
		rs.turnInFlight = false
	}
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn("run-370", TurnInput{StepID: "chat", Prompt: "continue as acc2"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn after account switch: %#v", apiErr)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Grok adapter turn")
	}
	mu.Lock()
	reqID := gotReqID
	mu.Unlock()
	if reqID != "019f60f6-dca6-7bf1-9374-c419b9df7d86" {
		t.Fatalf("adapter ProviderSessionID = %q, want real ACP id (session/load path)", reqID)
	}
	if _, found := LocateSessionFile(ProviderKeyGrok, acctBHome, "019f60f6-dca6-7bf1-9374-c419b9df7d86", cwd); !found {
		t.Fatal("expected session dir copied into active Grok home")
	}
	state, found, err := store.GetProviderSession(context.Background(), "run-370")
	if err != nil || !found {
		t.Fatalf("GetProviderSession = (%v, %v, %v)", state, found, err)
	}
	if state.ProviderAccountID != "acct-b" {
		t.Fatalf("ProviderAccountID = %q, want acct-b", state.ProviderAccountID)
	}
	if state.ProviderSessionID != "019f60f6-dca6-7bf1-9374-c419b9df7d86" {
		t.Fatalf("persisted ProviderSessionID = %q, want real id", state.ProviderSessionID)
	}
}

func TestStartTurnGrokCrossAccountDoesNotRebindOnRelocationFailure(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctAHome := filepath.Join(root, "acct-a")
	acctBHome := filepath.Join(root, "acct-b")
	cwd := "/repo"
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "grok", HomePath: acctAHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-b", ProviderKey: "grok", HomePath: acctBHome, SlotIndex: 2, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	if err := os.MkdirAll(acctAHome, 0o755); err != nil {
		t.Fatalf("MkdirAll acct-a: %v", err)
	}
	if err := os.MkdirAll(acctBHome, 0o755); err != nil {
		t.Fatalf("MkdirAll acct-b: %v", err)
	}
	writeGrokAuthFileFixture(t, acctAHome)
	writeGrokAuthFileFixture(t, acctBHome)
	writeGrokSessionTree(t, acctAHome, cwd, "sess-a", map[string]string{"chat_history.jsonl": "src\n"})
	// Conflicting destination with different contents blocks relocate.
	writeGrokSessionTree(t, acctBHome, cwd, "sess-a", map[string]string{"chat_history.jsonl": "other\n"})
	if err := store.AppendTurnLog(context.Background(), "run-1", turnLogLine{Kind: turnLogKindGrokSession, SessionID: "sess-a"}); err != nil {
		t.Fatalf("AppendTurnLog: %v", err)
	}

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyGrok,
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return &keyedFakeAdapter{
				key: ProviderKeyGrok,
				fn: func(_ context.Context, _ TurnRequest, b TurnBridge) error {
					t.Error("adapter must not run when relocation fails")
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "should-not-run"})
					return nil
				},
			}
		},
	})
	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	svc.mu.Lock()
	svc.runs["run-1"] = &interactiveRun{
		id:                "run-1",
		projectID:         "p",
		providerKey:       ProviderKeyGrok,
		providerSessionID: "thread-1",
		providerAccountID: "acct-a",
		workspaceCwd:      cwd,
		runKind:           "chat",
		status:            RunStatusIdle,
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
	}
	svc.mu.Unlock()

	_, apiErr := svc.startTurn("run-1", TurnInput{StepID: "chat", Prompt: "hi"}, "", "")
	if apiErr == nil || apiErr.code != "session_unavailable" {
		t.Fatalf("startTurn error = %#v, want session_unavailable", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs["run-1"]
	acct := ""
	if rs != nil {
		acct = rs.providerAccountID
	}
	svc.mu.Unlock()
	if acct != "acct-a" {
		t.Fatalf("providerAccountID = %q, want still acct-a", acct)
	}
}

func TestStartTurnGrokWorkflowAccountSwitchStillConflicts(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyGrok,
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return &keyedFakeAdapter{key: ProviderKeyGrok, fn: func(context.Context, TurnRequest, TurnBridge) error {
				return nil
			}}
		},
	})
	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	svc.mu.Lock()
	svc.runs["run-wf"] = &interactiveRun{
		id:                "run-wf",
		providerKey:       ProviderKeyGrok,
		providerSessionID: "thread-1",
		providerAccountID: "acct-a",
		workspaceCwd:      "/repo",
		runKind:           "workflow",
		status:            RunStatusIdle,
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
	}
	svc.mu.Unlock()
	_, apiErr := svc.startTurn("run-wf", TurnInput{StepID: "step-1", Prompt: "go"}, "", "")
	if apiErr == nil || apiErr.code != "provider_account_changed" {
		t.Fatalf("startTurn error = %#v, want provider_account_changed", apiErr)
	}
}

func TestRefreshResumeHandleGrokPersistsRealProviderSessionID(t *testing.T) {
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{
		id:                "run-1",
		providerKey:       ProviderKeyGrok,
		providerSessionID: "thread-1",
		providerAccountID: "acct-a",
		workspaceCwd:      "/repo",
		runKind:           "chat",
	}
	// Adapter-reported id only (not workspace discovery).
	adapter := &keyedFakeAdapter{key: ProviderKeyGrok, lastGrokSessionID: "sess-turn-1"}
	id := svc.refreshResumeHandleLocked(rs, adapter)
	if id != "sess-turn-1" {
		t.Fatalf("refreshResumeHandleLocked = %q, want sess-turn-1", id)
	}
	if rs.realProviderSessionID != "sess-turn-1" {
		t.Fatalf("realProviderSessionID = %q, want sess-turn-1", rs.realProviderSessionID)
	}
	snap := sessionStateOf(rs)
	if snap.ProviderSessionID != "sess-turn-1" {
		t.Fatalf("sessionStateOf.ProviderSessionID = %q, want sess-turn-1", snap.ProviderSessionID)
	}
}

// Codex review: ensureGrokProviderResumeHandle must not promote the only
// workspace session dir when the run has no turn-log ownership signal.
func TestEnsureGrokProviderResumeHandleDoesNotStealLoneWorkspaceDir(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	home := t.TempDir()
	cwd := "/repo"
	// Unrelated chat left exactly one session dir under this GROK_HOME/cwd.
	writeGrokSessionTree(t, home, cwd, "other-chat-only", nil)

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id:                "run-synthetic",
		providerKey:       ProviderKeyGrok,
		providerSessionID: "thread-1",
		workspaceCwd:      cwd,
		runKind:           "chat",
	}
	if svc.ensureGrokProviderResumeHandle(rs, home) {
		t.Fatal("expected no promotion without turn-log / real id")
	}
	if rs.realProviderSessionID != "" {
		t.Fatalf("realProviderSessionID = %q, want empty (no steal)", rs.realProviderSessionID)
	}
	if id := svc.resumeSessionID(rs); id != "thread-1" {
		t.Fatalf("resumeSessionID = %q, want thread-1", id)
	}
}

// Cross-account startTurn with synthetic id and no turn-log must fail closed
// (session_unavailable), not copy an unrelated lone session dir.
func TestStartTurnGrokCrossAccountNoTurnLogDoesNotStealLoneDir(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctAHome := filepath.Join(root, "acct-a")
	acctBHome := filepath.Join(root, "acct-b")
	cwd := "/repo"
	if err := os.MkdirAll(acctAHome, 0o755); err != nil {
		t.Fatalf("MkdirAll a: %v", err)
	}
	if err := os.MkdirAll(acctBHome, 0o755); err != nil {
		t.Fatalf("MkdirAll b: %v", err)
	}
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "grok", HomePath: acctAHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-b", ProviderKey: "grok", HomePath: acctBHome, SlotIndex: 2, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeGrokAuthFileFixture(t, acctAHome)
	writeGrokAuthFileFixture(t, acctBHome)
	writeGrokSessionTree(t, acctAHome, cwd, "unrelated-sess", nil)

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyGrok,
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return &keyedFakeAdapter{
				key: ProviderKeyGrok,
				fn: func(context.Context, TurnRequest, TurnBridge) error {
					t.Error("adapter must not run when resume handle cannot be proven")
					return nil
				},
			}
		},
	})
	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	svc.mu.Lock()
	svc.runs["run-x"] = &interactiveRun{
		id:                "run-x",
		projectID:         "p",
		providerKey:       ProviderKeyGrok,
		providerSessionID: "thread-x",
		providerAccountID: "acct-a",
		workspaceCwd:      cwd,
		runKind:           "chat",
		status:            RunStatusIdle,
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
	}
	svc.mu.Unlock()

	_, apiErr := svc.startTurn("run-x", TurnInput{StepID: "chat", Prompt: "hi"}, "", "")
	if apiErr == nil || apiErr.code != "session_unavailable" {
		t.Fatalf("startTurn error = %#v, want session_unavailable", apiErr)
	}
	if _, found := LocateSessionFile(ProviderKeyGrok, acctBHome, "unrelated-sess", cwd); found {
		t.Fatal("must not copy unrelated session into target home")
	}
	svc.mu.Lock()
	rs := svc.runs["run-x"]
	acct, real := "", ""
	if rs != nil {
		acct, real = rs.providerAccountID, rs.realProviderSessionID
	}
	svc.mu.Unlock()
	if acct != "acct-a" {
		t.Fatalf("providerAccountID = %q, want still acct-a", acct)
	}
	if real != "" {
		t.Fatalf("realProviderSessionID = %q, want empty", real)
	}
}

// run-536 regression: a failed first turn must NOT steal another chat's
// session directory from the same GROK_HOME/cwd just because it is newest.
func TestRefreshResumeHandleGrokDoesNotStealOtherChatSession(t *testing.T) {
	home := t.TempDir()
	cwd := "/repo"
	// Older chat's leftover session on disk for this workspace.
	writeGrokSessionTree(t, home, cwd, "other-chat-sess", nil)
	root := t.TempDir()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "grok", HomePath: home, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeGrokAuthFileFixture(t, home)

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{
		id:                "run-new",
		providerKey:       ProviderKeyGrok,
		providerSessionID: "thread-99",
		providerAccountID: "acct-a",
		workspaceCwd:      cwd,
		runKind:           "chat",
	}
	// Failed ensureSession → adapter reports no session id.
	adapter := &keyedFakeAdapter{key: ProviderKeyGrok, lastGrokSessionID: ""}
	id := svc.refreshResumeHandleLocked(rs, adapter)
	if id != "" {
		t.Fatalf("refresh returned %q, want empty (no steal)", id)
	}
	if rs.realProviderSessionID != "" {
		t.Fatalf("realProviderSessionID = %q, want empty so next turn still session/new", rs.realProviderSessionID)
	}
	snap := sessionStateOf(rs)
	if snap.ProviderSessionID != "thread-99" {
		t.Fatalf("persisted id = %q, want synthetic thread-99", snap.ProviderSessionID)
	}
}

func TestIsGrokRealSessionID(t *testing.T) {
	if isGrokRealSessionID("") || isGrokRealSessionID("thread-1") {
		t.Fatal("empty/thread ids must not be real")
	}
	if !isGrokRealSessionID("019f60f6-dca6-7bf1-9374-c419b9df7d86") {
		t.Fatal("expected real ACP id")
	}
	for _, bad := range []string{"../escape", "a/b", `a\b`, ".", "..", "/abs"} {
		if isGrokRealSessionID(bad) {
			t.Fatalf("expected path-unsafe id %q to be rejected", bad)
		}
	}
}

func TestLocateAndRelocateGrokRejectPathTraversalIDs(t *testing.T) {
	srcHome := t.TempDir()
	targetHome := t.TempDir()
	cwd := "/repo"
	// Even if a malicious dir existed, locate must reject the id before joining.
	malicious := filepath.Join(srcHome, "sessions", grokSessionsCwdDirName(cwd), "..", "escape")
	_ = os.MkdirAll(malicious, 0o755)
	_ = os.WriteFile(filepath.Join(malicious, "chat_history.jsonl"), []byte("x\n"), 0o644)
	if _, found := LocateSessionFile(ProviderKeyGrok, srcHome, "../escape", cwd); found {
		t.Fatal("LocateSessionFile must reject ../escape")
	}
	src := writeGrokSessionTree(t, srcHome, cwd, "sess-ok", nil)
	if _, err := RelocateSessionFile(ProviderKeyGrok, src, targetHome, "../escape", cwd); err == nil {
		t.Fatal("RelocateSessionFile must reject ../escape session id")
	}
	// Destination must not escape targetHome.
	escapeProbe := filepath.Join(filepath.Dir(targetHome), "escape-should-not-exist")
	if _, err := os.Stat(escapeProbe); err == nil {
		t.Fatalf("unexpected path outside target home: %s", escapeProbe)
	}
}

func TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	home := filepath.Join(root, "acct-a")
	cwd := filepath.Join(root, "workspace")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("MkdirAll home: %v", err)
	}
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll cwd: %v", err)
	}
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "grok", HomePath: home, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeGrokAuthFileFixture(t, home)
	writeGrokSessionTree(t, home, cwd, "sess-stable", nil)

	var (
		mu    sync.Mutex
		gotID string
		done  = make(chan struct{}, 1)
	)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyGrok,
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return &keyedFakeAdapter{
				key: ProviderKeyGrok,
				fn: func(_ context.Context, req TurnRequest, b TurnBridge) error {
					mu.Lock()
					gotID = req.ProviderSessionID
					mu.Unlock()
					select {
					case done <- struct{}{}:
					default:
					}
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
					return nil
				},
			}
		},
	})
	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-a"
	svc.mu.Lock()
	svc.runs["run-same"] = &interactiveRun{
		id:                    "run-same",
		projectID:             "p",
		providerKey:           ProviderKeyGrok,
		providerSessionID:     "thread-1",
		realProviderSessionID: "sess-stable",
		providerAccountID:     "acct-a",
		workspaceCwd:          cwd,
		runKind:               "chat",
		status:                RunStatusIdle,
		subs:                  map[int64]chan ProviderEvent{},
		idempotency:           map[string]string{},
	}
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn("run-same", TurnInput{StepID: "chat", Prompt: "follow up"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %#v", apiErr)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for adapter")
	}
	mu.Lock()
	id := gotID
	mu.Unlock()
	if id != "sess-stable" {
		t.Fatalf("ProviderSessionID = %q, want sess-stable for session/load", id)
	}
}

// keyedFakeAdapter is a test double that reports an arbitrary provider key.
type keyedFakeAdapter struct {
	key               ProviderKey
	fn                func(context.Context, TurnRequest, TurnBridge) error
	lastGrokSessionID string
}

func (a *keyedFakeAdapter) Key() ProviderKey { return a.key }
func (a *keyedFakeAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true, Resume: true}
}
func (a *keyedFakeAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	if a.fn == nil {
		return nil
	}
	return a.fn(ctx, req, bridge)
}

// LastGrokSessionID mirrors grokAdapter for refreshResumeHandleLocked tests.
func (a *keyedFakeAdapter) LastGrokSessionID() string { return a.lastGrokSessionID }

// BUG-312: restoreTargetPath had no case for ProviderKeyGrok at all -- every
// Grok restore fell into the switch's default branch and failed with
// "unsupported provider session relocation". TestRestoreTargetPathRejectsTraversal
// (cross_account_resume_test.go) already covers the Codex/Claude shape; these
// cover the new Grok case, including the cross-machine cwd-encoding concern
// that makes a naive "just reuse the embedded path" fix wrong (see the
// round-trip test below for the decisive proof).
func TestRestoreTargetPathGrokAcceptsWellFormedPathAndRejectsBadInput(t *testing.T) {
	home := t.TempDir()
	const sourceCwd = "/Users/tiendat/Desktop/BE/gate-sandbox"
	const targetCwd = `D:\working\gate-sandbox`
	const sessionID = "019f8722-c5fc-7f11-8d9e-4154bf38d338"
	relPath := "sessions/" + grokSessionsCwdDirName(sourceCwd) + "/" + sessionID + "/chat_history.jsonl"

	dst, err := restoreTargetPath(ProviderKeyGrok, home, relPath, sessionID, targetCwd)
	if err != nil {
		t.Fatalf("restoreTargetPath: unexpected error: %v", err)
	}
	want := filepath.Join(grokSessionDirPath(home, targetCwd, sessionID), "chat_history.jsonl")
	if dst != want {
		t.Fatalf("restoreTargetPath = %q, want %q (recomputed from the target cwd, not the source-embedded segment)", dst, want)
	}

	reject := []struct {
		name    string
		relPath string
		session string
		cwd     string
	}{
		{"synthetic thread placeholder session id", relPath, "thread-7", targetCwd},
		{"missing cwd", relPath, sessionID, ""},
		{"wrong file suffix (sidecar, not chat_history.jsonl)", "sessions/" + grokSessionsCwdDirName(sourceCwd) + "/" + sessionID + "/sidecar.json", sessionID, targetCwd},
		{"session id path traversal", relPath, "../../etc", targetCwd},
	}
	for _, tc := range reject {
		if _, err := restoreTargetPath(ProviderKeyGrok, home, tc.relPath, tc.session, tc.cwd); err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
		}
	}
}

// TestRestoreSessionFileGrokRoundTripsAcrossDifferentCwdEncoding is the
// decisive regression proof for BUG-312: a Grok chat synced from one machine
// (Mac-shaped cwd) must be restorable under a different machine/cwd
// (Windows-shaped) and be findable again afterward via LocateSessionFile --
// not merely "RestoreSessionFile returned no error". A naive fix that reused
// the source's embedded percent-encoded cwd segment verbatim would still
// "succeed" here but write into a directory LocateSessionFile could never
// look up again under the target's own cwd (grokSessionsCwdDirName encodes
// per-cwd, and the two cwds below encode to different segments).
func TestRestoreSessionFileGrokRoundTripsAcrossDifferentCwdEncoding(t *testing.T) {
	sourceHome := t.TempDir()
	targetHome := t.TempDir()
	const sourceCwd = "/Users/tiendat/Desktop/BE/gate-sandbox"
	const targetCwd = `D:\working\gate-sandbox`
	const sessionID = "019f8722-c5fc-7f11-8d9e-4154bf38d338"
	const transcript = `{"type":"assistant","content":"hello from grok"}` + "\n"

	writeGrokSessionTree(t, sourceHome, sourceCwd, sessionID, map[string]string{"chat_history.jsonl": transcript})
	sourcePath := filepath.Join(grokSessionDirPath(sourceHome, sourceCwd, sessionID), "chat_history.jsonl")
	relativePath, err := filepath.Rel(sourceHome, sourcePath)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	relativePath = filepath.ToSlash(relativePath)

	dstPath, err := RestoreSessionFile(ProviderKeyGrok, targetHome, relativePath, sessionID, targetCwd, []byte(transcript))
	if err != nil {
		t.Fatalf("RestoreSessionFile: unexpected error: %v", err)
	}

	foundPath, found := LocateSessionFile(ProviderKeyGrok, targetHome, sessionID, targetCwd)
	if !found {
		t.Fatalf("LocateSessionFile could not find the restored session under the target's own cwd (dst=%s)", dstPath)
	}
	body, err := os.ReadFile(filepath.Join(foundPath, "chat_history.jsonl"))
	if err != nil {
		t.Fatalf("read restored transcript: %v", err)
	}
	if string(body) != transcript {
		t.Fatalf("restored transcript = %q, want %q", body, transcript)
	}

	// The destination must NOT be the source's verbatim cwd-encoded directory --
	// that would be the pre-fix-shaped bug (right bytes, wrong/unfindable folder).
	wrongDir := grokSessionDirPath(targetHome, sourceCwd, sessionID)
	if filepath.Clean(filepath.Dir(dstPath)) == filepath.Clean(wrongDir) {
		t.Fatalf("restored into the SOURCE's cwd-encoded directory (%s) instead of recomputing from the target cwd", wrongDir)
	}
}
