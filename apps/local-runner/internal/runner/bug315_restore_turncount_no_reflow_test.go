package runner

// BUG-315: a Drive-restored flow-hub chat re-ran its ENTIRE flow (coder +
// reviewers + synthesis) on the next plain follow-up turn, instead of the
// follow-up reaching the hub. Root cause: the sync manifest carried every
// flow-runtime field EXCEPT TurnCount, so a restored hub came back with
// turnCount==0. The flow's entry nodes start only on a run's genuine first
// turn, gated on turnCount==0 both in startTurn and in resolveWorkflowFlowRef
// (which re-resolves the flowRef from the still-restored workflowID) -- both
// guards were defeated by the dropped count. Confirmed live: run-53157 /
// run-46797 / run-45881 all came back turn_count=1 (0 restored + 1 follow-up)
// and each spawned a fresh coder child on the follow-up.
//
// Fix: carry TurnCount through the manifest (primary), plus a restoredFrom
// guard so a restored run never re-starts its flow even from a pre-fix
// manifest that still lacks the count.
//
// additive-tests-only: new file only.
// cross-provider-parity: the round-trip test runs Codex, Claude, and Grok.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// TestSyncRestoreCarriesTurnCountAllProviders proves the sync->restore round
// trip preserves the hub's completed-turn count for every provider, so a
// restored chat's follow-up is a plain hub turn (turnCount>0) and never
// re-enters the turnCount==0 flow-start path.
func TestSyncRestoreCarriesTurnCountAllProviders(t *testing.T) {
	svc, instance, store, api, workspace, accountHome := newChatSyncService(t)

	claudeHome := filepath.Join(filepath.Dir(accountHome), "claude-home")
	if err := os.MkdirAll(filepath.Join(claudeHome, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll claude home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeHome, ".claude", ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"tok"}}`), 0o644); err != nil {
		t.Fatalf("write claude credentials: %v", err)
	}
	grokHome := filepath.Join(filepath.Dir(accountHome), "grok-home")
	if err := os.MkdirAll(grokHome, 0o755); err != nil {
		t.Fatalf("MkdirAll grok home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(grokHome, "auth.json"), []byte(`{"issuer::user":{"refresh_token":"rt","email":"g@example.com"}}`), 0o644); err != nil {
		t.Fatalf("write grok auth: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := instance.saveProviderAccountState(providerAccountState{Accounts: []ProviderAccount{
		{ID: "acct-sync", ProviderKey: "codex", DisplayName: "Account 1", HomePath: accountHome, SlotIndex: 1, IsActive: true, AuthStatus: "connected", CreatedAt: now},
		{ID: "acct-claude", ProviderKey: "claude", DisplayName: "Claude 1", HomePath: claudeHome, SlotIndex: 1, IsActive: true, AuthStatus: "connected", CreatedAt: now},
		{ID: "acct-grok", ProviderKey: "grok", DisplayName: "Grok 1", HomePath: grokHome, SlotIndex: 1, IsActive: true, AuthStatus: "connected", CreatedAt: now},
	}}); err != nil {
		t.Fatalf("saveProviderAccountState: %v", err)
	}

	const wantTurnCount = 4
	cases := []struct {
		provider  ProviderKey
		accountID string
		runID     string
	}{
		{ProviderKeyCodex, "acct-sync", "run-tc-codex"},
		{ProviderKeyClaude, "acct-claude", "run-tc-claude"},
		{ProviderKeyGrok, "acct-grok", "run-tc-grok"},
	}
	for _, tc := range cases {
		t.Run(string(tc.provider), func(t *testing.T) {
			state := ProviderSessionState{
				RunID:             tc.runID,
				ProjectID:         "project-1",
				WorkflowID:        "wf-" + tc.runID,
				ProviderKey:       tc.provider,
				ProviderSessionID: "thread-1",
				ProviderAccountID: tc.accountID,
				WorkingDirectory:  workspace,
				Status:            RunStatusCompleted,
				LastPrompt:        "fix bug 1+1 != 2",
				LastMessage:       "hub synthesis answer",
				StartedAt:         "2026-07-22T10:00:00Z",
				UpdatedAt:         "2026-07-22T10:05:00Z",
				RunKind:           "workflow",
				// The hub genuinely took several turns (initial + reinvokes).
				TurnCount: wantTurnCount,
			}
			if err := store.UpsertProviderSession(context.Background(), state); err != nil {
				t.Fatalf("UpsertProviderSession: %v", err)
			}

			result, apiErr := svc.syncChatRunToDrive(context.Background(), tc.runID, ChatSessionSyncRequest{})
			if apiErr != nil {
				t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
			}

			// The uploaded manifest must carry the count. Parsed as raw JSON so the
			// assertion compiles on the pre-fix baseline (no new Go field referenced).
			manifestID := remoteManifestFileID(api)
			if manifestID == "" {
				t.Fatal("expected manifest upload")
			}
			var manifest map[string]any
			if err := json.Unmarshal(api.files[manifestID].Content, &manifest); err != nil {
				t.Fatalf("manifest json invalid: %v", err)
			}
			gotManifestTC, _ := manifest["turnCount"].(float64)
			if int(gotManifestTC) != wantTurnCount {
				t.Fatalf("BUG-315 (%s): manifest turnCount = %v, want %d", tc.provider, manifest["turnCount"], wantTurnCount)
			}

			// Restore onto a clean local slot and confirm the count round-trips.
			if err := store.DeleteProviderSession(context.Background(), tc.runID); err != nil {
				t.Fatalf("DeleteProviderSession: %v", err)
			}
			restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
				ProjectID:       "project-1",
				SourceMachineID: result.SourceMachineID,
				SourceRunID:     result.SourceRunID,
				Cwd:             workspace,
			})
			if apiErr != nil {
				t.Fatalf("restoreChatRunFromDrive() failed: %#v", apiErr)
			}
			got, found, err := store.GetProviderSession(context.Background(), restored.RunID)
			if err != nil || !found {
				t.Fatalf("GetProviderSession(%s): found=%v err=%v", restored.RunID, found, err)
			}
			if got.TurnCount != wantTurnCount {
				t.Fatalf("BUG-315 (%s): restored TurnCount = %d, want %d (a restored hub with turnCount==0 re-runs its whole flow on the next follow-up)", tc.provider, got.TurnCount, wantTurnCount)
			}
		})
	}
}

// TestResolveWorkflowFlowRefSkipsRestoredRun proves the restoredFrom guard: a
// Drive-restored run keeps its workflowID, so even at turnCount==0 (a chat
// synced by a pre-fix manifest, which still carries no count) the flowRef must
// NOT be re-resolved -- otherwise startTurn would re-spawn the whole flow on a
// plain follow-up. A fresh (non-restored) run with the identical setup still
// resolves, proving the guard is scoped to restored runs only.
func TestResolveWorkflowFlowRefSkipsRestoredRun(t *testing.T) {
	const customFlowRef = "supabase-user-flow/bug315-two-step"

	newSvc := func() *InteractiveService {
		reg := newProviderRegistry()
		reg.register(ProviderRegistration{
			Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
			Capabilities: ProviderCapabilities{Streaming: true},
			newAdapter: func() ProviderRuntimeAdapter {
				return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
					return nil
				})
			},
		})
		catalog := newInteractiveCatalog()
		catalog.steps[customFlowRef] = []Step{
			{ID: "drafting", WorkflowID: customFlowRef, Name: "drafting", Order: 1, Model: "gpt-5.4"},
		}
		svc := newInteractiveService(reg, catalog, newFakeWorkflowStore())
		store := newFakeFlowDefinitionStore()
		store.byRef[customFlowRef] = FlowDefinitionRecord{
			FlowRef:   customFlowRef,
			Name:      "Bug315 Custom Flow",
			Source:    "supabase_user_definition",
			Editable:  true,
			Cloneable: true,
			Definition: agentpack.FlowDefinition{
				ID: "bug315-two-step",
				Nodes: []agentpack.FlowNode{
					{ID: "drafting", Behavior: "agent.delegate", Agent: "agents/coder.md"},
					{ID: "final_check", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"drafting"}},
				},
				Edges: []agentpack.FlowEdge{
					{From: "drafting", To: "final_check", When: "done", Kind: "forward"},
				},
			},
		}
		svc.SetFlowDefinitionStore(store)
		return svc
	}

	mkRun := func(svc *InteractiveService, restoredFrom string) string {
		parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
		if err != nil {
			t.Fatalf("createRun: %v", err)
		}
		svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
		svc.mu.Lock()
		svc.runs[parent.RunID].workflowID = customFlowRef
		svc.runs[parent.RunID].restoredFrom = restoredFrom
		svc.mu.Unlock()
		return parent.RunID
	}

	// Control: a fresh, non-restored run at turnCount==0 must still resolve, so
	// this test can't pass just because resolution is broken across the board.
	freshSvc := newSvc()
	freshRun := mkRun(freshSvc, "")
	if ref, ok := freshSvc.resolveWorkflowFlowRef(context.Background(), freshRun); !ok || ref != customFlowRef {
		t.Fatalf("fresh run: resolveWorkflowFlowRef = (%q,%v), want (%q,true)", ref, ok, customFlowRef)
	}

	// A Drive-restored run with the identical workflowID must NOT resolve.
	restoredSvc := newSvc()
	restoredRun := mkRun(restoredSvc, "google_drive")
	if ref, ok := restoredSvc.resolveWorkflowFlowRef(context.Background(), restoredRun); ok {
		t.Fatalf("BUG-315: restored run resolved flowRef %q (want no resolve, so the follow-up reaches the hub instead of re-spawning the flow)", ref)
	}
}
