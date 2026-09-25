package runner

// Chat SSOT substrate tests (Task-313): identity mint/self-tag/adopt, the
// FLOWPILOT_CHAT_SSOT gate, and the capture mapping. New file — additive only
// (safe-fix-contract R1/R3).

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
)

func TestNewChatIDFormat(t *testing.T) {
	pattern := regexp.MustCompile(`^cht_[0-9a-f]{12}$`)
	seen := map[string]struct{}{}
	for i := 0; i < 100; i++ {
		id := newChatID()
		if !pattern.MatchString(id) {
			t.Fatalf("newChatID() = %q, want cht_<12-hex>", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 100 {
		t.Fatalf("expected 100 distinct ids, got %d", len(seen))
	}
}

func TestEnsureChatTaggingLegacySelfTag(t *testing.T) {
	rs := &interactiveRun{id: "run-42", runKind: "chat"}
	ensureChatTagging(rs)
	if rs.chatID != "run-42" || rs.legSeq != 0 || rs.legState != LegStateActive {
		t.Fatalf("legacy self-tag = {chatID:%q legSeq:%d legState:%q}", rs.chatID, rs.legSeq, rs.legState)
	}
	// Idempotent: a second call must not change anything.
	ensureChatTagging(rs)
	if rs.chatID != "run-42" || rs.legSeq != 0 {
		t.Fatalf("self-tag not idempotent: {chatID:%q legSeq:%d}", rs.chatID, rs.legSeq)
	}
	// Workflow runs never self-tag.
	wf := &interactiveRun{id: "run-43", runKind: "workflow"}
	ensureChatTagging(wf)
	if wf.chatID != "" {
		t.Fatalf("workflow run self-tagged: chatID=%q", wf.chatID)
	}
}

func TestResolveChatIdentityMintAdoptExplicit(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}

	// 1. No chat hints → fresh mint with legSeq 0.
	chatID, legSeq, switchFrom, _ := svc.resolveChatIdentity(StartRunInput{ChatMode: "normal_chat"})
	if chatID == "" || legSeq != 0 || switchFrom != "" {
		t.Fatalf("mint path = (%q, %d, %q)", chatID, legSeq, switchFrom)
	}

	// 2. Explicit ChatID + LegSeq → trusted as-is (Task-314 phase-A contract).
	gotID, gotSeq, gotSwitch, _ := svc.resolveChatIdentity(StartRunInput{ChatID: "cht_a1", LegSeq: 3, SwitchFromRunID: "run-9"})
	if gotID != "cht_a1" || gotSeq != 3 || gotSwitch != "run-9" {
		t.Fatalf("explicit path = (%q, %d, %q)", gotID, gotSeq, gotSwitch)
	}

	// 3. Resident source leg via SwitchFromRunID → adopt chatId at legSeq+1.
	svc.runs["run-1"] = &interactiveRun{id: "run-1", runKind: "chat", chatID: "cht_b2", legSeq: 2, legState: LegStateActive}
	gotID, gotSeq, gotSwitch, _ = svc.resolveChatIdentity(StartRunInput{SwitchFromRunID: "run-1"})
	if gotID != "cht_b2" || gotSeq != 3 || gotSwitch != "run-1" {
		t.Fatalf("adopt path = (%q, %d, %q)", gotID, gotSeq, gotSwitch)
	}

	// 4. Legacy resident source (untagged) → self-tagged then adopted.
	svc.runs["run-2"] = &interactiveRun{id: "run-2", runKind: "chat"}
	gotID, gotSeq, _, _ = svc.resolveChatIdentity(StartRunInput{SwitchFromRunID: "run-2"})
	if gotID != "run-2" || gotSeq != 1 {
		t.Fatalf("legacy adopt = (%q, %d)", gotID, gotSeq)
	}

	// 5. Explicit ChatID without LegSeq → next after resident max.
	svc.runs["run-3"] = &interactiveRun{id: "run-3", runKind: "chat", chatID: "cht_b2", legSeq: 5}
	gotID, gotSeq, _, _ = svc.resolveChatIdentity(StartRunInput{ChatID: "cht_b2"})
	if gotID != "cht_b2" || gotSeq != 6 {
		t.Fatalf("explicit-no-seq = (%q, %d)", gotID, gotSeq)
	}

	// 6. Unknown SwitchFromRunID → fresh mint (caller decides; Task-317 passes
	// explicit LegSeq so this path is defensive only).
	gotID, gotSeq, _, _ = svc.resolveChatIdentity(StartRunInput{SwitchFromRunID: "run-missing"})
	if gotID == "" || gotSeq != 0 {
		t.Fatalf("missing-source = (%q, %d)", gotID, gotSeq)
	}
}

func TestChatSSOTFlagDefaultOffAndOptIn(t *testing.T) {
	// Dev branch: always ON (flag removed). Env is ignored.
	for _, v := range []string{"", "0", "1", "true", "false", "yes", "no", "TRUE"} {
		t.Setenv("FLOWPILOT_CHAT_SSOT", v)
		if !chatSSOTEnabled() {
			t.Fatalf("flag %q must still be enabled (always ON)", v)
		}
	}
}

// TestChatFieldsAdditiveJSON pins the additive-DTO contract: with the new chat
// fields unset, the JSON payload carries no chat keys at all (byte-comparable
// to the pre-CP-59 shapes for old clients).
func TestChatFieldsAdditiveJSON(t *testing.T) {
	in, err := json.Marshal(StartRunInput{ProjectID: "p1", ChatMode: "normal_chat"})
	if err != nil {
		t.Fatal(err)
	}
	if containsJSONKey(in, "chatId") || containsJSONKey(in, "switchFromRunId") || containsJSONKey(in, "legSeq") {
		t.Fatalf("StartRunInput leaked chat keys when unset: %s", in)
	}
	in, err = json.Marshal(StartRunInput{ChatID: "cht_x", LegSeq: 2, SwitchFromRunID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsJSONKey(in, "chatId") || !containsJSONKey(in, "legSeq") {
		t.Fatalf("StartRunInput missing chat keys when set: %s", in)
	}

	h, err := json.Marshal(RunHandle{RunID: "run-1", Status: RunStatusIdle})
	if err != nil {
		t.Fatal(err)
	}
	if containsJSONKey(h, "chatId") || containsJSONKey(h, "legSeq") {
		t.Fatalf("RunHandle leaked chat keys when unset: %s", h)
	}
	h, err = json.Marshal(RunHandle{RunID: "run-1", ChatID: "cht_x", LegSeq: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !containsJSONKey(h, "chatId") || !containsJSONKey(h, "legSeq") {
		t.Fatalf("RunHandle missing chat keys when set: %s", h)
	}
}

func containsJSONKey(raw []byte, key string) bool {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// TestResolveChatIdentityUsesPersistedLegSeq pins the detached-reattach rule
// (SD26 §10): after a restart the prior legs live in the persisted session
// store — an explicit ChatID without LegSeq must continue after the persisted
// max, not collide at leg 1.
func TestResolveChatIdentityUsesPersistedLegSeq(t *testing.T) {
	fws := newFakeWorkflowStore()
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: fws}
	ctx := context.Background()
	_ = fws.UpsertProviderSession(ctx, ProviderSessionState{RunID: "run-1", RunKind: "chat", ChatID: "cht_x", LegSeq: 0})
	_ = fws.UpsertProviderSession(ctx, ProviderSessionState{RunID: "run-2", RunKind: "chat", ChatID: "cht_x", LegSeq: 2})
	chatID, legSeq, switchFrom, _ := svc.resolveChatIdentity(StartRunInput{ChatID: "cht_x", SwitchFromRunID: "run-2"})
	if chatID != "cht_x" || legSeq != 3 || switchFrom != "run-2" {
		t.Fatalf("persisted max = (%q %d %q), want (cht_x 3 run-2)", chatID, legSeq, switchFrom)
	}
}
