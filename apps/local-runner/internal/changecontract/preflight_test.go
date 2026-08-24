package changecontract

import (
	"os"
	"sync"
	"testing"
	"time"
)

// --- ParsePreflightDraft -----------------------------------------------------

func TestParsePreflightDraftAcceptsStrictJSON(t *testing.T) {
	draft, err := ParsePreflightDraft(`{"feature_key":"calc-core","intent":"fix rounding","declared_paths":["src/calc.go"],"source_doc_id":"Task-1"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if draft.FeatureKey != "calc-core" || draft.Intent != "fix rounding" || draft.SourceDocID != "Task-1" {
		t.Fatalf("got %+v", draft)
	}
	if len(draft.DeclaredPaths) != 1 || draft.DeclaredPaths[0] != "src/calc.go" {
		t.Fatalf("DeclaredPaths = %v", draft.DeclaredPaths)
	}
}

func TestParsePreflightDraftAcceptsSurroundingWhitespace(t *testing.T) {
	if _, err := ParsePreflightDraft("  \n{\"feature_key\":\"x\",\"intent\":\"y\",\"declared_paths\":[\"a.go\"]}\n  "); err != nil {
		t.Fatalf("leading/trailing whitespace must be tolerated: %v", err)
	}
}

func TestParsePreflightDraftAcceptsLeadingAndTrailingProse(t *testing.T) {
	text := "I'll locate `format.go` and `format_test.go` so the contract can list exact paths only. {\"feature_key\":\"calc-format\",\"intent\":\"add ClampChecked\",\"declared_paths\":[\"format.go\",\"format_test.go\"]}"
	draft, err := ParsePreflightDraft(text)
	if err != nil {
		t.Fatalf("unexpected error parsing prose-wrapped draft: %v", err)
	}
	if draft.FeatureKey != "calc-format" || len(draft.DeclaredPaths) != 2 {
		t.Fatalf("unexpected draft: %+v", draft)
	}
}

func TestParsePreflightDraftAcceptsMarkdownCodeFence(t *testing.T) {
	text := "Here is the contract:\n```json\n{\n  \"feature_key\": \"calc-format\",\n  \"intent\": \"add ClampChecked\",\n  \"declared_paths\": [\"format.go\"]\n}\n```\nDone."
	draft, err := ParsePreflightDraft(text)
	if err != nil {
		t.Fatalf("unexpected error parsing markdown code fence draft: %v", err)
	}
	if draft.FeatureKey != "calc-format" || len(draft.DeclaredPaths) != 1 {
		t.Fatalf("unexpected draft: %+v", draft)
	}
}

func TestParsePreflightDraftRejectsUnknownFields(t *testing.T) {
	_, err := ParsePreflightDraft(`{"feature_key":"x","intent":"y","declared_paths":["a.go"],"extra":"nope"}`)
	if err == nil {
		t.Fatal("an unknown field must be rejected")
	}
}

func TestParsePreflightDraftRejectsEmptyInput(t *testing.T) {
	if _, err := ParsePreflightDraft("   "); err == nil {
		t.Fatal("blank input must be rejected")
	}
}

func TestParsePreflightDraftRejectsMalformedJSON(t *testing.T) {
	if _, err := ParsePreflightDraft(`{"feature_key": }`); err == nil {
		t.Fatal("malformed JSON must be rejected")
	}
}

// --- ValidatePreflightDraft ---------------------------------------------------

func TestValidatePreflightDraftRequiresFeatureKey(t *testing.T) {
	_, err := ValidatePreflightDraft(PreflightContractDraft{
		Intent:        "fix",
		DeclaredPaths: []string{"a.go"},
	}, nil)
	if err == nil {
		t.Fatal("blank feature_key must be rejected")
	}
}

func TestValidatePreflightDraftRejectsUnknownFeatureKey(t *testing.T) {
	_, err := ValidatePreflightDraft(PreflightContractDraft{
		FeatureKey:    "not-a-real-key",
		Intent:        "fix",
		DeclaredPaths: []string{"a.go"},
	}, []string{"calc-core", "context-regression-engine"})
	if err == nil {
		t.Fatal("a feature_key absent from knownFeatureKeys must be rejected")
	}
}

func TestValidatePreflightDraftAcceptsKnownFeatureKey(t *testing.T) {
	got, err := ValidatePreflightDraft(PreflightContractDraft{
		FeatureKey:    "calc-core",
		Intent:        "fix",
		DeclaredPaths: []string{"a.go"},
	}, []string{"calc-core"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.FeatureKey != "calc-core" {
		t.Fatalf("got %+v", got)
	}
}

func TestValidatePreflightDraftSkipsKnownFeatureKeyCheckWhenListEmpty(t *testing.T) {
	if _, err := ValidatePreflightDraft(PreflightContractDraft{
		FeatureKey:    "anything",
		Intent:        "fix",
		DeclaredPaths: []string{"a.go"},
	}, nil); err != nil {
		t.Fatalf("an empty knownFeatureKeys must skip the membership check: %v", err)
	}
}

func TestValidatePreflightDraftRequiresIntent(t *testing.T) {
	_, err := ValidatePreflightDraft(PreflightContractDraft{
		FeatureKey:    "calc-core",
		DeclaredPaths: []string{"a.go"},
	}, nil)
	if err == nil {
		t.Fatal("blank intent must be rejected")
	}
}

func TestValidatePreflightDraftRequiresConcretePath(t *testing.T) {
	_, err := ValidatePreflightDraft(PreflightContractDraft{
		FeatureKey:    "calc-core",
		Intent:        "fix",
		DeclaredPaths: []string{"apps", "internal/*.go"},
	}, nil)
	if err == nil {
		t.Fatal("a draft with no concrete declared path must be rejected")
	}
}

func TestValidatePreflightDraftRequiresConcretePathEmptyList(t *testing.T) {
	_, err := ValidatePreflightDraft(PreflightContractDraft{
		FeatureKey: "calc-core",
		Intent:     "fix",
	}, nil)
	if err == nil {
		t.Fatal("an empty declared_paths list must be rejected")
	}
}

// --- ComputeContractID --------------------------------------------------------

func TestComputeContractIDIsDeterministic(t *testing.T) {
	draft := PreflightContractDraft{FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"a.go", "b.go"}}
	baseline1 := map[string]string{"a.go": "h1", "b.go": "h2"}
	baseline2 := map[string]string{"b.go": "h2", "a.go": "h1"} // different iteration order, same content

	id1 := ComputeContractID("run-1", "step-1", 1, draft, "sha1", baseline1)
	id2 := ComputeContractID("run-1", "step-1", 1, draft, "sha1", baseline2)
	if id1 != id2 {
		t.Fatalf("map iteration order must not affect the id: %s vs %s", id1, id2)
	}
	if id1 == "" {
		t.Fatal("id must not be empty")
	}
}

func TestComputeContractIDChangesWithVersion(t *testing.T) {
	draft := PreflightContractDraft{FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"a.go"}}
	id1 := ComputeContractID("run-1", "step-1", 1, draft, "sha1", nil)
	id2 := ComputeContractID("run-1", "step-1", 2, draft, "sha1", nil)
	if id1 == id2 {
		t.Fatal("a version bump must change the id")
	}
}

func TestComputeContractIDChangesWithBaseline(t *testing.T) {
	draft := PreflightContractDraft{FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"a.go"}}
	id1 := ComputeContractID("run-1", "step-1", 1, draft, "sha1", map[string]string{"a.go": "h1"})
	id2 := ComputeContractID("run-1", "step-1", 1, draft, "sha1", map[string]string{"a.go": "h2"})
	if id1 == id2 {
		t.Fatal("a different baseline fingerprint must change the id")
	}
}

// --- FreezeContract ------------------------------------------------------------

func TestFreezeContractNormalizesPathsAndComputesID(t *testing.T) {
	ws := t.TempDir()
	draft := PreflightContractDraft{
		FeatureKey:    "calc-core",
		Intent:        "fix rounding",
		DeclaredPaths: []string{"internal/*.go", `src\calc.go`},
	}
	rec, err := FreezeContract(ws, "run-1", "planner-step", "coder-step", draft, "sha123", nil, "", 1, time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.DeclaredPaths) != 1 || rec.DeclaredPaths[0] != "src/calc.go" {
		t.Fatalf("DeclaredPaths = %v, want [src/calc.go] (glob dropped, separators normalized)", rec.DeclaredPaths)
	}
	if rec.ContractID == "" {
		t.Fatal("ContractID must be set")
	}
	if rec.Version != 1 {
		t.Fatalf("Version = %d, want 1", rec.Version)
	}
}

func TestFreezeContractPropagatesNormalizationError(t *testing.T) {
	ws := t.TempDir()
	draft := PreflightContractDraft{
		FeatureKey:    "calc-core",
		Intent:        "fix",
		DeclaredPaths: []string{"apps", "internal/*.go"}, // all noise, nothing concrete survives
	}
	if _, err := FreezeContract(ws, "run-1", "", "coder-step", draft, "sha", nil, "", 1, time.Now()); err == nil {
		t.Fatal("an all-noise declared path list must fail freeze")
	}
}

// --- FrozenStore ---------------------------------------------------------------

func freezeTestRecord(runID, coderStepID string, version int, id string) FrozenContractRecord {
	return FrozenContractRecord{
		ContractID:    id,
		Version:       version,
		RunID:         runID,
		CoderStepID:   coderStepID,
		FeatureKey:    "calc-core",
		Intent:        "fix",
		DeclaredPaths: []string{"src/calc.go"},
		DeclaredAt:    time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
	}
}

func TestSaveFrozenRejectsMutationOfExistingContractID(t *testing.T) {
	ws := t.TempDir()
	store, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	rec := freezeTestRecord("run-1", "step-1", 1, "same-id")
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// Same id, same payload: idempotent no-op.
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatalf("idempotent re-save of identical payload must succeed: %v", err)
	}
	// Same id, different payload: rejected.
	mutated := rec
	mutated.Intent = "different intent"
	if err := store.SaveFrozen(mutated); err == nil {
		t.Fatal("mutating an existing contract_id's payload must be rejected")
	}
}

func TestSaveFrozenAllowsHigherAmendmentVersion(t *testing.T) {
	ws := t.TempDir()
	store, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	v1 := freezeTestRecord("run-1", "step-1", 1, "id-v1")
	if err := store.SaveFrozen(v1); err != nil {
		t.Fatal(err)
	}
	v2 := freezeTestRecord("run-1", "step-1", 2, "id-v2")
	v2.Supersedes = "id-v1"
	if err := store.SaveFrozen(v2); err != nil {
		t.Fatalf("a higher-version amendment must be allowed: %v", err)
	}

	versions, err := store.ListVersionsForStep("run-1", "step-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("ListVersionsForStep returned %d versions, want 2", len(versions))
	}
}

func TestGetFrozenForStepReturnsLatestActiveVersion(t *testing.T) {
	ws := t.TempDir()
	store, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	v1 := freezeTestRecord("run-1", "step-1", 1, "id-v1")
	if err := store.SaveFrozen(v1); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.GetFrozenForStep("run-1", "step-1")
	if err != nil || !ok {
		t.Fatalf("got=%v ok=%v err=%v, want v1 active", got, ok, err)
	}
	if got.ContractID != "id-v1" {
		t.Fatalf("ContractID = %q, want id-v1", got.ContractID)
	}

	// Amend: v1 is superseded by v2.
	v2 := freezeTestRecord("run-1", "step-1", 2, "id-v2")
	v2.Supersedes = "id-v1"
	if err := store.SaveFrozen(v2); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus("id-v1", ContractStatusSuperseded, "amended", time.Now()); err != nil {
		t.Fatal(err)
	}

	got, ok, err = store.GetFrozenForStep("run-1", "step-1")
	if err != nil || !ok {
		t.Fatalf("got=%v ok=%v err=%v, want v2 active", got, ok, err)
	}
	if got.ContractID != "id-v2" {
		t.Fatalf("ContractID = %q, want id-v2 (v1 superseded)", got.ContractID)
	}
}

func TestGetFrozenForStepReturnsNotOkWhenEveryVersionInactive(t *testing.T) {
	ws := t.TempDir()
	store, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	rec := freezeTestRecord("run-1", "step-1", 1, "id-v1")
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus("id-v1", ContractStatusAbandoned, "cancelled", time.Now()); err != nil {
		t.Fatal(err)
	}
	_, ok, err := store.GetFrozenForStep("run-1", "step-1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("an abandoned-only step must report no active version")
	}
}

func TestAppendStatusRejectsUnknownContractID(t *testing.T) {
	ws := t.TempDir()
	store, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus("never-frozen", ContractStatusAccepted, "", time.Now()); err == nil {
		t.Fatal("appending a status for an unknown contract_id must fail")
	}
}

func TestFrozenStoreReloadsAcrossInstances(t *testing.T) {
	ws := t.TempDir()
	store1, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	rec := freezeTestRecord("run-1", "step-1", 1, "id-v1")
	if err := store1.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}
	if err := store1.AppendStatus("id-v1", ContractStatusAccepted, "done", time.Now()); err != nil {
		t.Fatal(err)
	}

	store2, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := store2.GetFrozenForStep("run-1", "step-1")
	if err != nil || !ok || got.ContractID != "id-v1" {
		t.Fatalf("a fresh store instance must reload persisted records: got=%v ok=%v err=%v", got, ok, err)
	}
}

func TestFrozenStoreCorruptContractsLineFailsClosed(t *testing.T) {
	ws := t.TempDir()
	store, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(freezeTestRecord("run-1", "step-1", 1, "id-v1")); err != nil {
		t.Fatal(err)
	}

	path := store.contractsPath
	if err := appendRawLine(path, "{not valid json"); err != nil {
		t.Fatal(err)
	}

	if _, err := NewFrozenStore(ws); err == nil {
		t.Fatal("a corrupt trailing line must fail NewFrozenStore, not be silently skipped")
	}
}

func TestFrozenStoreCorruptEventsLineFailsClosed(t *testing.T) {
	ws := t.TempDir()
	store, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(freezeTestRecord("run-1", "step-1", 1, "id-v1")); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus("id-v1", ContractStatusAccepted, "", time.Now()); err != nil {
		t.Fatal(err)
	}

	if err := appendRawLine(store.eventsPath, "{also not valid"); err != nil {
		t.Fatal(err)
	}

	if _, err := NewFrozenStore(ws); err == nil {
		t.Fatal("a corrupt trailing event line must fail NewFrozenStore, not be silently skipped")
	}
}

func TestFrozenStoreConcurrentInstancesDoNotInterleave(t *testing.T) {
	ws := t.TempDir()
	const n = 20

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store, err := NewFrozenStore(ws)
			if err != nil {
				t.Errorf("NewFrozenStore: %v", err)
				return
			}
			rec := freezeTestRecord("run-1", "step-1", i+1, "id-concurrent")
			rec.ContractID = "id-concurrent-" + string(rune('A'+i))
			if err := store.SaveFrozen(rec); err != nil {
				t.Errorf("SaveFrozen: %v", err)
			}
		}(i)
	}
	wg.Wait()

	final, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatalf("final reload must not see a torn/interleaved write: %v", err)
	}
	versions, err := final.ListVersionsForStep("run-1", "step-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != n {
		t.Fatalf("got %d records, want %d — a torn write dropped or merged lines", len(versions), n)
	}
}

// --- Legacy Contract Store must stay untouched --------------------------------

func TestContractStoreLoadsLegacyRecordsWithoutNewFields(t *testing.T) {
	ws := t.TempDir()
	legacyStore, err := NewStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacyStore.Save(Contract{
		RunID:         "run-1",
		StepID:        "step-1",
		FeatureKey:    "calc-core",
		Intent:        "legacy",
		DeclaredPaths: []string{"src/calc.go"},
		Confidence:    ConfidenceDeclared,
	}); err != nil {
		t.Fatal(err)
	}

	// A FrozenStore rooted at the same workspace must not see/touch the legacy
	// contracts.ndjson file at all — separate files, separate lifecycles.
	frozenStore, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := frozenStore.GetFrozenForStep("run-1", "step-1"); ok {
		t.Fatal("FrozenStore must not read the legacy Contract store's records")
	}

	reloaded, err := NewStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.Get("run-1", "step-1")
	if !ok || got.FeatureKey != "calc-core" {
		t.Fatalf("legacy Store must still load its own records unaffected: got=%+v ok=%v", got, ok)
	}
}

func appendRawLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}
