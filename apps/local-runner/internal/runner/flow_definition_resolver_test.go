package runner

import (
	"context"
	"fmt"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// fakeFlowDefinitionStore is an in-memory FlowDefinitionStore for tests. It
// intentionally has no Supabase dependency so Task-175's mirror-sync and
// resolver contracts are provable without any real backing infrastructure.
type fakeFlowDefinitionStore struct {
	byRef map[string]FlowDefinitionRecord
}

func newFakeFlowDefinitionStore() *fakeFlowDefinitionStore {
	return &fakeFlowDefinitionStore{byRef: make(map[string]FlowDefinitionRecord)}
}

func (f *fakeFlowDefinitionStore) GetByPackFlow(_ context.Context, packID, packFlowID string) (FlowDefinitionRecord, bool, error) {
	for _, rec := range f.byRef {
		if rec.PackID == packID && rec.PackFlowID == packFlowID && rec.Source == "supabase_builtin_mirror" {
			return rec, true, nil
		}
	}
	return FlowDefinitionRecord{}, false, nil
}

func (f *fakeFlowDefinitionStore) GetByRef(_ context.Context, flowRef string) (FlowDefinitionRecord, bool, error) {
	rec, ok := f.byRef[flowRef]
	return rec, ok, nil
}

func (f *fakeFlowDefinitionStore) ListAll(_ context.Context) ([]FlowDefinitionRecord, error) {
	out := make([]FlowDefinitionRecord, 0, len(f.byRef))
	for _, rec := range f.byRef {
		out = append(out, rec)
	}
	return out, nil
}

// Upsert mimics the real store's id-assignment behavior: a record with an
// empty FlowRef (a brand-new user-owned clone) is assigned a fake generated
// id, mirroring how SupabaseWorkflowFlowStore assigns a UUID.
func (f *fakeFlowDefinitionStore) Upsert(_ context.Context, record FlowDefinitionRecord) (FlowDefinitionRecord, error) {
	if record.FlowRef == "" {
		record.FlowRef = fmt.Sprintf("fake-generated-id-%d", len(f.byRef)+1)
	}
	f.byRef[record.FlowRef] = record
	return record, nil
}

func TestFlowMirrorSyncInsertsOnFirstRun(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	svc := NewFlowMirrorSyncService(store)

	synced, err := svc.SyncBuiltins(context.Background())
	if err != nil {
		t.Fatalf("SyncBuiltins: %v", err)
	}
	if len(synced) == 0 {
		t.Fatal("expected at least one flow to be synced on first run")
	}
	for _, rec := range synced {
		if rec.Editable {
			t.Errorf("mirrored built-in %q must not be editable", rec.FlowRef)
		}
		if rec.Source != "supabase_builtin_mirror" {
			t.Errorf("mirrored built-in %q has source %q, want supabase_builtin_mirror", rec.FlowRef, rec.Source)
		}
	}
}

func TestFlowMirrorSyncSecondRunIsNoOp(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	svc := NewFlowMirrorSyncService(store)
	ctx := context.Background()

	if _, err := svc.SyncBuiltins(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	second, err := svc.SyncBuiltins(ctx)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("expected no-op second sync, got %d updated records", len(second))
	}
}

func TestFlowMirrorSyncUpdatesOnHashChange(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	svc := NewFlowMirrorSyncService(store)
	ctx := context.Background()

	if _, err := svc.SyncBuiltins(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Simulate a stale mirror row: same identity, different hash/version.
	rec, ok, err := store.GetByPackFlow(ctx, "flowpilot-core-flow-pack", "review-loop")
	if err != nil || !ok {
		t.Fatalf("expected mirrored review-loop row, ok=%v err=%v", ok, err)
	}
	rec.PackHash = "stale-hash"
	rec.PackVersion = "stale-version"
	if _, err := store.Upsert(ctx, rec); err != nil {
		t.Fatalf("seed stale row: %v", err)
	}

	updated, err := svc.SyncBuiltins(ctx)
	if err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	found := false
	for _, r := range updated {
		if r.PackFlowID == "review-loop" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected stale review-loop mirror row to be updated")
	}
}

func TestFlowMirrorSyncNeverOverwritesUserFlow(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	userRecord := FlowDefinitionRecord{
		FlowRef:  "user-flow-1",
		Source:   "supabase_user_definition",
		Editable: true,
	}
	if _, err := store.Upsert(context.Background(), userRecord); err != nil {
		t.Fatalf("seed user flow: %v", err)
	}

	svc := NewFlowMirrorSyncService(store)
	if _, err := svc.SyncBuiltins(context.Background()); err != nil {
		t.Fatalf("SyncBuiltins: %v", err)
	}

	got, ok, err := store.GetByRef(context.Background(), "user-flow-1")
	if err != nil || !ok {
		t.Fatalf("user flow disappeared: ok=%v err=%v", ok, err)
	}
	if got.Source != userRecord.Source || got.Editable != userRecord.Editable {
		t.Fatalf("user flow was mutated by builtin sync: got %#v, want %#v", got, userRecord)
	}
}

func TestFlowDefinitionResolverPrefersMirrorRowOverPack(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	svc := NewFlowMirrorSyncService(store)
	if _, err := svc.SyncBuiltins(context.Background()); err != nil {
		t.Fatalf("SyncBuiltins: %v", err)
	}

	resolver := NewFlowDefinitionResolver(store)
	record, err := resolver.ResolveFlowRef(context.Background(), "flowpilot-core-flow-pack/review-loop")
	if err != nil {
		t.Fatalf("ResolveFlowRef: %v", err)
	}
	if record.Source != "supabase_builtin_mirror" {
		t.Fatalf("source = %q, want supabase_builtin_mirror", record.Source)
	}
	if record.PackFlowID != "review-loop" {
		t.Fatalf("packFlowID = %q, want review-loop", record.PackFlowID)
	}
}

func TestFlowDefinitionResolverFallsBackToPackWithoutStore(t *testing.T) {
	resolver := NewFlowDefinitionResolver(nil)
	record, err := resolver.ResolveFlowRef(context.Background(), "flowpilot-core-flow-pack/review-loop")
	if err != nil {
		t.Fatalf("ResolveFlowRef: %v", err)
	}
	if record.Source != "builtin_pack" {
		t.Fatalf("source = %q, want builtin_pack", record.Source)
	}
	if record.Editable {
		t.Fatal("built-in fallback record must not be editable")
	}
}

// TestFlowDefinitionResolverRecreatesMissingMirror is the regression test
// for BUG-NOTE-CP42 #15: a missing mirror row (deleted, or never synced
// because the store wasn't configured yet at startup) used to be a
// permanent, silent fallback to the embedded pack — nothing ever recreated
// the mirror until the next process restart's best-effort startup sync.
// CP-42 requires selecting a built-in whose mirror is missing to trigger a
// recreate before the run starts. This proves ResolveBuiltin now does that:
// the store actually gains a row, and the returned record reflects the
// mirrored source, not just an inert embedded-pack fallback.
func TestFlowDefinitionResolverRecreatesMissingMirror(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	resolver := NewFlowDefinitionResolver(store)

	record, err := resolver.ResolveFlowRef(context.Background(), "flowpilot-core-flow-pack/rag-harness")
	if err != nil {
		t.Fatalf("ResolveFlowRef: %v", err)
	}
	if record.Source != "supabase_builtin_mirror" {
		t.Fatalf("source = %q, want supabase_builtin_mirror (mirror should have been recreated)", record.Source)
	}

	// The recreate must have actually persisted, not just been returned —
	// a second, independent lookup against the same store must find it too.
	mirrored, ok, err := store.GetByPackFlow(context.Background(), "flowpilot-core-flow-pack", "rag-harness")
	if err != nil {
		t.Fatalf("GetByPackFlow after recreate: %v", err)
	}
	if !ok {
		t.Fatal("expected the missing mirror to have been recreated in the store")
	}
	if mirrored.PackFlowID != "rag-harness" {
		t.Fatalf("recreated mirror PackFlowID = %q, want rag-harness", mirrored.PackFlowID)
	}
}

// TestFlowDefinitionResolverFallsBackToPackWhenRecreateFails proves the
// degrade-gracefully half of the same fix: a mirror-recreate failure (e.g. a
// transient Supabase error) must not block flow resolution — the embedded
// pack alone is enough to run the flow, so a mirror-sync hiccup should be
// invisible to the caller, not surfaced as a resolution failure.
func TestFlowDefinitionResolverFallsBackToPackWhenRecreateFails(t *testing.T) {
	store := &failingUpsertFlowDefinitionStore{fakeFlowDefinitionStore: newFakeFlowDefinitionStore()}
	resolver := NewFlowDefinitionResolver(store)

	record, err := resolver.ResolveFlowRef(context.Background(), "flowpilot-core-flow-pack/rag-harness")
	if err != nil {
		t.Fatalf("ResolveFlowRef: %v", err)
	}
	if record.Source != "builtin_pack" {
		t.Fatalf("source = %q, want builtin_pack (recreate failed, so this must fall back to the plain embedded-pack record)", record.Source)
	}
}

// failingUpsertFlowDefinitionStore wraps fakeFlowDefinitionStore with an
// Upsert that always fails, for TestFlowDefinitionResolverFallsBackToPackWhenRecreateFails.
type failingUpsertFlowDefinitionStore struct {
	*fakeFlowDefinitionStore
}

func (f *failingUpsertFlowDefinitionStore) Upsert(_ context.Context, _ FlowDefinitionRecord) (FlowDefinitionRecord, error) {
	return FlowDefinitionRecord{}, fmt.Errorf("simulated upsert failure")
}

func TestFlowDefinitionResolverUnknownRefFails(t *testing.T) {
	resolver := NewFlowDefinitionResolver(nil)
	if _, err := resolver.ResolveFlowRef(context.Background(), "flowpilot-core-flow-pack/does-not-exist"); err == nil {
		t.Fatal("expected error for unknown flow id")
	}
	if _, err := resolver.ResolveFlowRef(context.Background(), "not-a-valid-ref"); err == nil {
		t.Fatal("expected error for malformed flowRef")
	}
}

// TestFlowDefinitionResolverRejectsStoredDefinitionWithUnknownBehavior is the
// regression test for BUG-NOTE-CP42 #28: the embedded pack validates every
// node's behavior at load time (LoadFlowFS -> ValidateFlowDefinition), but a
// Supabase-loaded row skipped that check entirely — recordFromWorkflowRow
// copies whatever string sits in the behavior_id column straight into
// FlowNode.Behavior with no validation, and the executor silently skips a
// node it doesn't recognize (entryDelegateNodes/forwardDoneTargets just
// don't match it) instead of failing fast, exactly the "resolves successfully
// but doesn't spawn anything" class of bug this whole file exists to catch
// (see BUG#9/#15). This proves a garbage behavior_id — as free-text Settings
// UI input or DB corruption could produce — is now rejected at resolution
// time instead of silently loading.
func TestFlowDefinitionResolverRejectsStoredDefinitionWithUnknownBehavior(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	store.byRef["22222222-2222-2222-2222-222222222222"] = FlowDefinitionRecord{
		FlowRef: "22222222-2222-2222-2222-222222222222",
		Source:  "supabase_user_definition",
		Definition: agentpack.FlowDefinition{
			ID: "custom-flow",
			Nodes: []agentpack.FlowNode{
				{ID: "n1", Behavior: "totally.not.a.real.behavior", Agent: "agents/coder.md"},
			},
		},
	}
	resolver := NewFlowDefinitionResolver(store)
	if _, err := resolver.ResolveFlowRef(context.Background(), "22222222-2222-2222-2222-222222222222"); err == nil {
		t.Fatal("expected an error resolving a stored definition with an unknown behavior_id, got none")
	}
}

func TestCloneBuiltinCreatesEditableUserCopy(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	resolver := NewFlowDefinitionResolver(store)

	clone, err := resolver.CloneBuiltin(context.Background(), "flowpilot-core-flow-pack", "review-loop", "My Review Loop")
	if err != nil {
		t.Fatalf("CloneBuiltin: %v", err)
	}
	if !clone.Editable {
		t.Fatal("clone must be editable")
	}
	// BUG-NOTE-CP42 #12: cloned_from is a `uuid references workflows(id)`
	// column; a built-in's FlowRef is the stable "packId/flowId" string, not
	// a UUID, so it must NOT be written there (it would violate the FK
	// constraint against a real database) — ClonedFrom is correctly left
	// empty for a built-in source. See TestCloneBuiltinFromUUIDSourcePreservesClonedFrom
	// for the case where ClonedFrom IS populated (cloning an already
	// user-owned/UUID-identified flow).
	if clone.ClonedFrom != "" {
		t.Fatalf("clonedFrom = %q, want empty for a built-in source (not a UUID, would violate the cloned_from FK)", clone.ClonedFrom)
	}
	if clone.Name != "My Review Loop" {
		t.Fatalf("clone name = %q, want My Review Loop", clone.Name)
	}
	if clone.FlowRef == "" || clone.FlowRef == "flowpilot-core-flow-pack/review-loop" {
		t.Fatalf("clone must get a distinct, store-assigned flowRef, got %q", clone.FlowRef)
	}

	stored, ok, err := store.GetByRef(context.Background(), clone.FlowRef)
	if err != nil || !ok {
		t.Fatalf("clone not persisted: ok=%v err=%v", ok, err)
	}
	if !stored.Editable {
		t.Fatal("persisted clone must be editable")
	}
}

// TestCloneBuiltinFromUUIDSourcePreservesClonedFrom exercises the branch
// where a resolved source's FlowRef is already a real UUID (the shape a
// user-owned/already-cloned flow has) rather than a built-in's stable
// "packId/flowId" ref — CloneBuiltin must preserve ClonedFrom in that case,
// since it's a valid FK value. The production FlowDefinitionStore never
// actually returns a UUID FlowRef from GetByPackFlow (recordFromWorkflowRow
// always normalizes a built-in mirror row back to its stable pack ref), so
// this seeds the fake store directly to prove the looksLikeUUID branch
// itself behaves correctly per the FlowDefinitionStore interface contract.
func TestCloneBuiltinFromUUIDSourcePreservesClonedFrom(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	const uuidRef = "33333333-3333-3333-3333-333333333333"
	store.byRef[uuidRef] = FlowDefinitionRecord{
		FlowRef:    uuidRef,
		Source:     "supabase_builtin_mirror",
		PackID:     "flowpilot-core-flow-pack",
		PackFlowID: "review-loop",
		Cloneable:  true,
		Definition: agentpack.FlowDefinition{ID: "review-loop"},
	}
	resolver := NewFlowDefinitionResolver(store)

	clone, err := resolver.CloneBuiltin(context.Background(), "flowpilot-core-flow-pack", "review-loop", "My Clone")
	if err != nil {
		t.Fatalf("CloneBuiltin: %v", err)
	}
	if clone.ClonedFrom != uuidRef {
		t.Fatalf("clonedFrom = %q, want the UUID source ref %q preserved", clone.ClonedFrom, uuidRef)
	}
}

func TestCloneBuiltinRejectsNonCloneableFlow(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	resolver := NewFlowDefinitionResolver(store)

	// Force-seed a non-cloneable builtin mirror row and attempt to clone it.
	builtin, err := resolver.ResolveBuiltin(context.Background(), "flowpilot-core-flow-pack", "review-loop")
	if err != nil {
		t.Fatalf("ResolveBuiltin: %v", err)
	}
	builtin.Source = "supabase_builtin_mirror"
	builtin.Cloneable = false
	if _, err := store.Upsert(context.Background(), builtin); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := resolver.CloneBuiltin(context.Background(), builtin.PackID, builtin.PackFlowID, "should-fail"); err == nil {
		t.Fatal("expected error cloning a non-cloneable flow")
	}
}

func TestUpdateUserFlowRejectsReadOnlyBuiltinMirror(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	svc := NewFlowMirrorSyncService(store)
	if _, err := svc.SyncBuiltins(context.Background()); err != nil {
		t.Fatalf("SyncBuiltins: %v", err)
	}
	resolver := NewFlowDefinitionResolver(store)

	builtin, ok, err := store.GetByPackFlow(context.Background(), "flowpilot-core-flow-pack", "review-loop")
	if err != nil || !ok {
		t.Fatalf("expected mirrored row: ok=%v err=%v", ok, err)
	}
	builtin.Editable = true // attacker/bug attempts to flip the flag locally
	if _, err := resolver.UpdateUserFlow(context.Background(), builtin); err != ErrDefinitionNotEditable {
		t.Fatalf("UpdateUserFlow error = %v, want ErrDefinitionNotEditable", err)
	}
}

func TestUpdateUserFlowAllowsEditableUserRecord(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	resolver := NewFlowDefinitionResolver(store)
	clone, err := resolver.CloneBuiltin(context.Background(), "flowpilot-core-flow-pack", "review-loop", "My Review Loop")
	if err != nil {
		t.Fatalf("CloneBuiltin: %v", err)
	}

	clone.Definition.Description = "edited by alice"
	if _, err := resolver.UpdateUserFlow(context.Background(), clone); err != nil {
		t.Fatalf("UpdateUserFlow: %v", err)
	}
	stored, ok, err := store.GetByRef(context.Background(), clone.FlowRef)
	if err != nil || !ok {
		t.Fatalf("expected stored clone: ok=%v err=%v", ok, err)
	}
	if stored.Definition.Description != "edited by alice" {
		t.Fatalf("update did not persist: %#v", stored)
	}
}
