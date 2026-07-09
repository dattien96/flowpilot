package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"flowpilot-runner/internal/agentpack"
)

// FlowDefinitionRecord is the normalized runtime shape a resolved flow ref
// produces, whether it came from a built-in pack, a mirrored built-in row, or
// a user-owned definition. CP-42 P-3 requires all three sources to collapse
// to this one shape so the executor never branches on where a flow came from.
//
// Backed by the `workflows`/`workflow_steps` tables (CP-42/Task-175/179):
// built-in flows are mirrored into read-only Workflow rows, and the existing
// Settings "Workflows/Steps" screen is reused for authoring instead of a
// parallel table/UI. FlowRef is opaque and has two possible shapes: a stable
// "packId/flowId" string for built-ins (resolved by pack identity, since a
// mirror can be re-created with a new row id), or the workflow's own UUID
// `id` for a user-owned row (there is no separate per-user "owner" concept in
// this schema — a flow belongs to a project, or is workspace-global).
type FlowDefinitionRecord struct {
	FlowRef      string
	Name         string // Workflow.Name — human display name, distinct from Definition.Description
	Source       string // "builtin_pack" | "supabase_builtin_mirror" | "supabase_user_definition"
	Editable     bool
	Cloneable    bool
	PackID       string
	PackVersion  string
	PackFlowID   string
	PackHash     string
	SelectableIn []string
	ChatBaseline bool
	ChatSubModes []string
	ClonedFrom   string // non-empty flowRef this record was cloned from, for audit/debugging
	Definition   agentpack.FlowDefinition
}

// FlowDefinitionStore persists built-in mirror rows and user-owned flow
// definitions. Resolution and mirror sync only depend on this contract, never
// on a specific backing store — see SupabaseWorkflowFlowStore for the
// concrete implementation backed by `workflows`/`workflow_steps`.
type FlowDefinitionStore interface {
	// GetByPackFlow looks up a mirrored built-in row by pack identity.
	GetByPackFlow(ctx context.Context, packID, packFlowID string) (FlowDefinitionRecord, bool, error)
	// GetByRef looks up any record (built-in mirror or user-owned) by its
	// canonical flowRef.
	GetByRef(ctx context.Context, flowRef string) (FlowDefinitionRecord, bool, error)
	// Upsert inserts or updates a record and returns the persisted row,
	// whose FlowRef may differ from the input (e.g. a brand-new user-owned
	// record with FlowRef=="" gets a store-assigned id). Implementations
	// must reject an attempt to overwrite a record with Editable=false
	// through any path other than the mirror-sync service.
	Upsert(ctx context.Context, record FlowDefinitionRecord) (FlowDefinitionRecord, error)
	// ListAll returns every stored record (built-in mirrors and user-owned
	// flows alike), for a flow catalog/list UI. Order is unspecified;
	// callers sort as needed.
	ListAll(ctx context.Context) ([]FlowDefinitionRecord, error)
}

// ErrDefinitionNotEditable is returned when a caller attempts to modify a
// read-only built-in mirror row directly instead of cloning it first.
var ErrDefinitionNotEditable = fmt.Errorf("flow definition is read-only; clone it before editing")

// ErrFlowDefinitionInvalid wraps a validation failure (agentpack.ValidateFlowDefinition,
// ValidateFlowContextSources, ValidateFlowArtifactBindings) for a flow
// definition row that WAS found — as opposed to a flowRef/workflowID that
// simply doesn't resolve to any flow at all (a plain admin workflow, or no
// matching row). BUG-270: resolveWorkflowFlowRef's "safe bail on any error"
// contract treated both cases identically, so a genuine data problem (e.g.
// BUG-269's unknown context-source id) silently fell back to a normal chat
// turn with no indication anything was wrong — the exact opposite of
// CP-44/Task-194 T-2's "fails flow-load fast" guarantee the validation
// itself provides. Callers use errors.As to distinguish this from a
// legitimate "not a flow" bail and surface it to the user instead.
type ErrFlowDefinitionInvalid struct {
	FlowRef string
	err     error
}

func (e *ErrFlowDefinitionInvalid) Error() string { return e.err.Error() }
func (e *ErrFlowDefinitionInvalid) Unwrap() error  { return e.err }

// canonicalFlowRef formats the canonical "packId/flowId" flowRef.
func canonicalFlowRef(packID, flowID string) string {
	return packID + "/" + flowID
}

// splitFlowRef parses a canonical "packId/flowId" flowRef. It intentionally
// treats the ref as opaque beyond that split — the resolver never inspects
// semantic flow names like "review-loop" or "rag-harness" (Task-175 T-3).
func splitFlowRef(flowRef string) (packID, flowID string, ok bool) {
	idx := strings.LastIndex(flowRef, "/")
	if idx <= 0 || idx == len(flowRef)-1 {
		return "", "", false
	}
	return flowRef[:idx], flowRef[idx+1:], true
}

// hashFlowContent returns a stable content hash for mirror staleness checks.
func hashFlowContent(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// FlowDefinitionResolver resolves an opaque flowRef to the normalized
// FlowDefinitionRecord shape. It never special-cases a flow by semantic name;
// callers pass "flowpilot-core-flow-pack/review-loop" or any other pack/flow
// pair and get back the same kind of record.
type FlowDefinitionResolver struct {
	store FlowDefinitionStore // may be nil: falls back to the embedded pack only
}

// NewFlowDefinitionResolver returns a resolver backed by store. Passing a nil
// store is valid and makes every resolution fall back to the embedded pack
// (Task-175 T-7: local-only fallback when no definition store is configured).
func NewFlowDefinitionResolver(store FlowDefinitionStore) *FlowDefinitionResolver {
	return &FlowDefinitionResolver{store: store}
}

// ResolveFlowRef resolves any canonical flowRef: a mirrored/user row from the
// store when available, otherwise a direct built-in pack lookup.
func (r *FlowDefinitionResolver) ResolveFlowRef(ctx context.Context, flowRef string) (FlowDefinitionRecord, error) {
	flowRef = strings.TrimSpace(flowRef)
	if flowRef == "" {
		return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: empty flowRef")
	}
	if r.store != nil {
		if record, ok, err := r.store.GetByRef(ctx, flowRef); err != nil {
			return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: store lookup for %q: %w", flowRef, err)
		} else if ok {
			if err := agentpack.ValidateFlowDefinition(record.Definition); err != nil {
				return FlowDefinitionRecord{}, &ErrFlowDefinitionInvalid{FlowRef: flowRef, err: fmt.Errorf("flow definition resolver: stored definition for %q failed validation: %w", flowRef, err)}
			}
			if err := ValidateFlowContextSources(record.Definition); err != nil {
				return FlowDefinitionRecord{}, &ErrFlowDefinitionInvalid{FlowRef: flowRef, err: fmt.Errorf("flow definition resolver: stored definition for %q failed validation: %w", flowRef, err)}
			}
			if err := ValidateFlowArtifactBindings(record.Definition); err != nil {
				return FlowDefinitionRecord{}, &ErrFlowDefinitionInvalid{FlowRef: flowRef, err: fmt.Errorf("flow definition resolver: stored definition for %q failed validation: %w", flowRef, err)}
			}
			return record, nil
		}
	}
	packID, flowID, ok := splitFlowRef(flowRef)
	if !ok {
		return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: %q not found and is not a resolvable pack/flow ref", flowRef)
	}
	return r.ResolveBuiltin(ctx, packID, flowID)
}

// ResolveBuiltin resolves a built-in flow directly, preferring a mirrored row
// from the store and falling back to the embedded pack when the store has no
// row yet or is unavailable (local-only execution, Task-175 T-7).
func (r *FlowDefinitionResolver) ResolveBuiltin(ctx context.Context, packID, flowID string) (FlowDefinitionRecord, error) {
	if r.store != nil {
		if record, ok, err := r.store.GetByPackFlow(ctx, packID, flowID); err != nil {
			return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: mirror lookup for %s/%s: %w", packID, flowID, err)
		} else if ok {
			mirrorRef := canonicalFlowRef(packID, flowID)
			if err := agentpack.ValidateFlowDefinition(record.Definition); err != nil {
				return FlowDefinitionRecord{}, &ErrFlowDefinitionInvalid{FlowRef: mirrorRef, err: fmt.Errorf("flow definition resolver: mirrored definition for %s/%s failed validation: %w", packID, flowID, err)}
			}
			if err := ValidateFlowContextSources(record.Definition); err != nil {
				return FlowDefinitionRecord{}, &ErrFlowDefinitionInvalid{FlowRef: mirrorRef, err: fmt.Errorf("flow definition resolver: mirrored definition for %s/%s failed validation: %w", packID, flowID, err)}
			}
			if err := ValidateFlowArtifactBindings(record.Definition); err != nil {
				return FlowDefinitionRecord{}, &ErrFlowDefinitionInvalid{FlowRef: mirrorRef, err: fmt.Errorf("flow definition resolver: mirrored definition for %s/%s failed validation: %w", packID, flowID, err)}
			}
			return record, nil
		}
	}
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: load embedded pack: %w", err)
	}
	if pack.Manifest.ID != packID {
		return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: unknown pack %q", packID)
	}
	for i, def := range pack.Flows {
		if def.ID != flowID {
			continue
		}
		raw, err := agentpack.ReadBuiltinFlowRaw(pack.Manifest.Flows[i].Path)
		if err != nil {
			return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: read %q: %w", pack.Manifest.Flows[i].Path, err)
		}
		record := builtinRecordFromFlow(pack.Manifest, def, raw)
		// BUG-NOTE-CP42 #15: a missing mirror row (deleted, or never synced
		// because the store wasn't configured yet at startup) used to be a
		// permanent, silent fallback to the embedded pack — the mirror was
		// never recreated until the next process restart's best-effort
		// startup sync. CP-42 requires selecting a built-in whose mirror is
		// missing to trigger a recreate before the run starts. Best-effort:
		// a recreate failure falls through to the plain embedded-pack record
		// rather than failing resolution — a flow-start failure must not be
		// caused by a mirror-sync hiccup when the embedded pack alone is
		// enough to run the flow.
		if r.store != nil {
			mirrorAttempt := record
			mirrorAttempt.Source = "supabase_builtin_mirror"
			if saved, upsertErr := r.store.Upsert(ctx, mirrorAttempt); upsertErr == nil {
				return saved, nil
			}
			// Upsert failed: fall through and return the original
			// builtin_pack-sourced record below, not a record whose Source
			// claims a mirror that was never actually persisted.
		}
		return record, nil
	}
	return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: flow %q not found in pack %q", flowID, packID)
}

func builtinRecordFromFlow(manifest agentpack.Manifest, def agentpack.FlowDefinition, raw []byte) FlowDefinitionRecord {
	return FlowDefinitionRecord{
		FlowRef:      canonicalFlowRef(manifest.ID, def.ID),
		Name:         flowOptionLabel(def.ID),
		Source:       "builtin_pack",
		Editable:     false,
		Cloneable:    def.Builtin.Cloneable,
		PackID:       manifest.ID,
		PackVersion:  manifest.Version,
		PackFlowID:   def.ID,
		PackHash:     hashFlowContent(raw),
		SelectableIn: append([]string(nil), def.Builtin.SelectableIn...),
		ChatBaseline: def.Builtin.ChatBaseline,
		ChatSubModes: append([]string(nil), def.Builtin.ChatSubModes...),
		Definition:   def,
	}
}

// CloneBuiltin creates an editable, user-owned copy of a built-in flow: a new
// `workflows` row (store-assigned UUID id, which becomes the clone's
// FlowRef) with a deep copy of the source's nodes/edges/policy. name becomes
// the new row's display Name; the caller supplies it (e.g. from a Settings UI
// "Clone" prompt) rather than the resolver inventing one.
func (r *FlowDefinitionResolver) CloneBuiltin(ctx context.Context, packID, flowID, name string) (FlowDefinitionRecord, error) {
	if r.store == nil {
		return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: cloning requires a definition store")
	}
	source, err := r.ResolveBuiltin(ctx, packID, flowID)
	if err != nil {
		return FlowDefinitionRecord{}, err
	}
	if !source.Cloneable {
		return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: %s/%s is not cloneable", packID, flowID)
	}
	// BUG-NOTE-CP42 #12: the migration's cloned_from column is
	// `uuid references workflows(id)`, but a built-in's FlowRef is always the
	// stable "packId/flowId" string (canonicalFlowRef), never the mirrored
	// row's actual UUID — recordFromWorkflowRow deliberately normalizes it
	// that way so a re-created mirror row never changes a built-in's FlowRef.
	// FlowDefinitionStore's interface has no way to recover the underlying
	// row UUID from a packID/flowID lookup, so inserting source.FlowRef
	// directly would violate the FK constraint for a built-in source (the
	// live desktop UI's own clone path, SupabaseAdminRepository.cloneWorkflow,
	// avoids this entirely by using the source row's real `id` directly — this
	// Go API path is not currently reachable from the UI). Only set
	// ClonedFrom when it's actually a UUID (cloning an already user-owned/
	// cloned flow); a built-in source leaves it unset rather than inserting
	// an invalid FK value — this does mean a built-in-sourced clone has no
	// cloned_from provenance today, a known limitation of the current schema,
	// not silently wrong data.
	clonedFrom := ""
	if looksLikeUUID(source.FlowRef) {
		clonedFrom = source.FlowRef
	}
	clone := FlowDefinitionRecord{
		Name:       name,
		Source:     "supabase_user_definition",
		Editable:   true,
		Cloneable:  true,
		ClonedFrom: clonedFrom,
		Definition: source.Definition,
	}
	clone.Definition.Builtin = agentpack.BuiltinMeta{}
	saved, err := r.store.Upsert(ctx, clone)
	if err != nil {
		return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: save clone: %w", err)
	}
	return saved, nil
}

// UpdateUserFlow saves changes to a user-owned flow. It refuses to modify any
// record that is not marked Editable, so a caller can never mutate a built-in
// mirror row in place (Task-175 acceptance: "editing a mirrored built-in is
// rejected").
func (r *FlowDefinitionResolver) UpdateUserFlow(ctx context.Context, record FlowDefinitionRecord) (FlowDefinitionRecord, error) {
	if r.store == nil {
		return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: updating a user flow requires a definition store")
	}
	existing, ok, err := r.store.GetByRef(ctx, record.FlowRef)
	if err != nil {
		return FlowDefinitionRecord{}, fmt.Errorf("flow definition resolver: lookup %q: %w", record.FlowRef, err)
	}
	if ok && !existing.Editable {
		return FlowDefinitionRecord{}, ErrDefinitionNotEditable
	}
	if !record.Editable {
		return FlowDefinitionRecord{}, ErrDefinitionNotEditable
	}
	return r.store.Upsert(ctx, record)
}

// FlowMirrorSyncService mirrors embedded pack flows into a FlowDefinitionStore
// as read-only rows so built-ins can be selected/listed the same way as
// user-owned definitions. Sync is idempotent: it only writes when the pack's
// row is missing or its content hash has changed.
type FlowMirrorSyncService struct {
	store FlowDefinitionStore
}

// NewFlowMirrorSyncService returns a sync service writing into store.
func NewFlowMirrorSyncService(store FlowDefinitionStore) *FlowMirrorSyncService {
	return &FlowMirrorSyncService{store: store}
}

// SyncBuiltins mirrors every flow declared in the embedded pack manifest.
// It returns the records that were inserted or updated; a flow whose mirror
// row already matches the current pack hash is left untouched and omitted
// from the result.
func (s *FlowMirrorSyncService) SyncBuiltins(ctx context.Context) ([]FlowDefinitionRecord, error) {
	if s.store == nil {
		return nil, fmt.Errorf("flow mirror sync: no definition store configured")
	}
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		return nil, fmt.Errorf("flow mirror sync: load embedded pack: %w", err)
	}

	// BUG-249: repair or retire any orphaned builtin mirror row (e.g. a
	// manually corrupted/renamed pack_flow_id, CP-36 Scenario 14's own
	// reproduction) before the per-flow loop below runs. Doing this first
	// means a reclaimed row is already fixed by the time the loop's own
	// GetByPackFlow lookup runs, so it is treated as a normal up-to-date
	// mirror with no separate "already reclaimed" branch needed here.
	// Best-effort and non-fatal: a store that doesn't support this optional
	// capability (e.g. the in-memory test fake) simply keeps the prior
	// behavior of inserting a fresh row when a mirror is missing.
	if reclaimer, ok := s.store.(builtinStaleMirrorReclaimer); ok && pack.Manifest.ID != "" {
		currentFlows := make([]flowHashID, 0, len(pack.Flows))
		for i, def := range pack.Flows {
			raw, err := agentpack.ReadBuiltinFlowRaw(pack.Manifest.Flows[i].Path)
			if err != nil {
				return nil, fmt.Errorf("flow mirror sync: read %q: %w", pack.Manifest.Flows[i].Path, err)
			}
			currentFlows = append(currentFlows, flowHashID{FlowID: def.ID, Hash: hashFlowContent(raw)})
		}
		if _, err := reclaimer.ReclaimOrRetireStaleBuiltinMirrors(ctx, pack.Manifest.ID, currentFlows); err != nil {
			log.Printf("[flow-mirror-sync] stale builtin mirror reclaim/retire for pack %q failed (non-fatal, per-flow sync below still runs): %v", pack.Manifest.ID, err)
		}
	}

	var synced []FlowDefinitionRecord
	for i, def := range pack.Flows {
		manifestFlow := pack.Manifest.Flows[i]
		raw, err := agentpack.ReadBuiltinFlowRaw(manifestFlow.Path)
		if err != nil {
			return synced, fmt.Errorf("flow mirror sync: read %q: %w", manifestFlow.Path, err)
		}
		record := builtinRecordFromFlow(pack.Manifest, def, raw)
		record.Source = "supabase_builtin_mirror"

		existing, ok, err := s.store.GetByPackFlow(ctx, record.PackID, record.PackFlowID)
		if err != nil {
			return synced, fmt.Errorf("flow mirror sync: lookup %s/%s: %w", record.PackID, record.PackFlowID, err)
		}
		if ok && existing.PackHash == record.PackHash && existing.PackVersion == record.PackVersion {
			// BUG-Rnd2 (Bug A): a pack hash match is not sufficient — an old DB row
			// may have been created before the node_lifecycle column existed, leaving
			// every step_definition with node_lifecycle=NULL. flowNodeLifecycle()
			// defaults a NULL lifecycle to "reinvoke", so reviewer-spawn nodes
			// silently become reinvoke nodes at runtime. Re-upsert whenever the
			// embedded pack declares a lifecycle that the stored mirror is missing.
			if !mirrorHasMissingLifecycles(existing.Definition.Nodes, record.Definition.Nodes) {
				continue // truly up to date, nothing to write
			}
			// Fall through: at least one node lifecycle is absent from the mirror.
		}
		saved, err := s.store.Upsert(ctx, record)
		if err != nil {
			return synced, fmt.Errorf("flow mirror sync: upsert %s/%s: %w", record.PackID, record.PackFlowID, err)
		}
		synced = append(synced, saved)
	}
	return synced, nil
}

// mirrorHasMissingLifecycles returns true when any node in canonical (the
// embedded pack's node list) has a non-empty Lifecycle that the corresponding
// node in stored (the DB mirror's node list, matched by node ID) is missing.
//
// Used by SyncBuiltins to detect rows created before node_lifecycle was added
// to the DB schema. Such rows would otherwise pass the hash+version freshness
// check and never be refreshed, leaving every node defaulting to "reinvoke"
// at runtime even when the pack YAML declares "spawn".
func mirrorHasMissingLifecycles(stored, canonical []agentpack.FlowNode) bool {
	storedByID := make(map[string]agentpack.FlowNode, len(stored))
	for _, n := range stored {
		if n.ID != "" {
			storedByID[n.ID] = n
		}
	}
	for _, n := range canonical {
		if strings.TrimSpace(n.Lifecycle) == "" {
			continue // pack node has no lifecycle declared; nothing to check
		}
		mirrored, ok := storedByID[n.ID]
		if !ok || strings.TrimSpace(mirrored.Lifecycle) == "" {
			return true // canonical lifecycle missing from mirror
		}
	}
	return false
}
