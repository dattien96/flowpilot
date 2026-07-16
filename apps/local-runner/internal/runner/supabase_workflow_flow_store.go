package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"flowpilot-runner/internal/agentpack"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func looksLikeUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// SupabaseWorkflowFlowStore is the FlowDefinitionStore backing CP-42
// Task-175/179: rather than a parallel `flow_definitions` table, built-in
// agentpack flows are mirrored directly into the existing `workflows` +
// `workflow_steps` tables (see
// supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql),
// and the existing Settings "Workflows/Steps" screen is the authoring UI —
// no separate flow-authoring screen. Follows SupabaseWorkflowStore's request
// conventions: PostgREST filters via query params, the package-level
// httpRequestFn for mockable tests, embedded workflow_steps via `select`.
type SupabaseWorkflowFlowStore struct {
	restURL string
	apiKey  string
}

// NewSupabaseWorkflowFlowStore builds the store from the workspace config +
// the resolved API key (service-role or RLS-scoped).
func NewSupabaseWorkflowFlowStore(cfg SupabaseWorkspaceConfig, apiKey string) *SupabaseWorkflowFlowStore {
	base := strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	return &SupabaseWorkflowFlowStore{restURL: base + "/rest/v1", apiKey: apiKey}
}

func (s *SupabaseWorkflowFlowStore) headers(prefer string) map[string]string {
	h := map[string]string{
		"apikey":        s.apiKey,
		"Authorization": "Bearer " + s.apiKey,
		"Content-Type":  "application/json",
	}
	if prefer != "" {
		h["Prefer"] = prefer
	}
	return h
}

type dbFlowEdgeRow struct {
	From string `json:"from"`
	To   string `json:"to"`
	When string `json:"when"`
	Kind string `json:"kind"`
}

type dbWorkflowStepRow struct {
	StepType       string              `json:"step_type"`
	OrderIndex     int                 `json:"order_index"`
	StepDefinition dbStepDefinitionRow `json:"step_definitions"`
}

type dbStepDefinitionRow struct {
	StepType          string                     `json:"step_type"`
	NodeID            *string                    `json:"node_id"`
	NodeLifecycle     *string                    `json:"node_lifecycle"`
	BehaviorID        *string                    `json:"behavior_id"`
	AgentRef          *string                    `json:"agent_ref"`
	JoinMode          *string                    `json:"join_mode"`
	Cohort            *string                    `json:"cohort"`
	PromptTemplateRef *string                    `json:"prompt_template_ref"`
	ContextRef        *string                    `json:"context_ref"`
	ContextSources    []string                   `json:"context_sources"`
	ArtifactBindings  []dbStepArtifactBindingRow `json:"step_artifact_bindings"`
}

// dbStepArtifactBindingRow mirrors one `step_artifact_bindings` row joined
// with its bound `artifact_instances` row (CP-45/SD-23 D-1/D-4). Embedding
// the instance in the same PostgREST select avoids an N+1 lookup per node —
// recordFromWorkflowRow denormalizes ArtifactTypeID/ConfigJSON directly onto
// agentpack.FlowArtifactBinding.
type dbStepArtifactBindingRow struct {
	ID                 string                    `json:"id"`
	Direction          string                    `json:"direction"`
	SlotName           string                    `json:"slot_name"`
	Required           bool                      `json:"required"`
	Position           int                       `json:"position"`
	ArtifactInstanceID string                    `json:"artifact_instance_id"`
	ArtifactInstance   *dbArtifactInstanceRefRow `json:"artifact_instances"`
}

type dbArtifactInstanceRefRow struct {
	ArtifactTypeID string         `json:"artifact_type_id"`
	ConfigJSON     map[string]any `json:"config_json"`
	Status         string         `json:"status"`
}

type dbFlowContextRow struct {
	Ref string `json:"ref"`
}

type dbWorkflowRow struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	Description      string                      `json:"description"`
	IsBuiltin        bool                        `json:"is_builtin"`
	Editable         bool                        `json:"editable"`
	Cloneable        bool                        `json:"cloneable"`
	ClonedFrom       *string                     `json:"cloned_from"`
	PackID           *string                     `json:"pack_id"`
	PackVersion      *string                     `json:"pack_version"`
	PackFlowID       *string                     `json:"pack_flow_id"`
	PackHash         *string                     `json:"pack_hash"`
	SelectableInJSON []string                    `json:"selectable_in_json"`
	ChatBaseline     bool                        `json:"chat_baseline"`
	ChatSubModesJSON []string                    `json:"chat_sub_modes_json"`
	PolicyCap        *int                        `json:"policy_cap"`
	PolicyOnCap      *string                     `json:"policy_on_cap"`
	PolicyExtendBy   *int                        `json:"policy_extend_by"`
	PolicyExtendMax  *int                        `json:"policy_extend_max"`
	EdgesJSON        []dbFlowEdgeRow             `json:"edges_json"`
	ContextsJSON     map[string]dbFlowContextRow `json:"contexts_json"`
	WorkflowSteps    []dbWorkflowStepRow         `json:"workflow_steps"`
}

const workflowSelect = "*,workflow_steps(step_type,order_index,step_definitions(step_type,node_id,node_lifecycle,behavior_id,agent_ref,join_mode,cohort,prompt_template_ref,context_ref,context_sources,step_artifact_bindings(id,direction,slot_name,required,position,artifact_instance_id,artifact_instances(artifact_type_id,config_json,status))))"

func recordFromWorkflowRow(row dbWorkflowRow) FlowDefinitionRecord {
	rec := FlowDefinitionRecord{
		FlowRef:      row.ID,
		Name:         row.Name,
		Editable:     row.Editable,
		Cloneable:    row.Cloneable,
		SelectableIn: row.SelectableInJSON,
		ChatBaseline: row.ChatBaseline,
		ChatSubModes: row.ChatSubModesJSON,
	}
	if row.IsBuiltin {
		rec.Source = "supabase_builtin_mirror"
	} else {
		rec.Source = "supabase_user_definition"
	}
	if row.PackID != nil {
		rec.PackID = *row.PackID
		// Built-ins resolve by the stable "packId/flowId" ref, not the row's
		// own UUID id, so a re-created mirror row never changes its flowRef.
		if row.PackFlowID != nil {
			rec.FlowRef = canonicalFlowRef(*row.PackID, *row.PackFlowID)
		}
	}
	if row.PackVersion != nil {
		rec.PackVersion = *row.PackVersion
	}
	if row.PackFlowID != nil {
		rec.PackFlowID = *row.PackFlowID
	}
	if row.PackHash != nil {
		rec.PackHash = *row.PackHash
	}
	if row.ClonedFrom != nil {
		rec.ClonedFrom = *row.ClonedFrom
	}

	// BUG-NOTE-CP42 #21: builtinRecordFromFlow (the embedded-pack path) keeps
	// Definition.ID as the flow's own semantic id (e.g. "review-loop"), since
	// that's literally what the YAML declares. A mirrored row used to set
	// Definition.ID to the row's own UUID instead, so the SAME built-in flow
	// exposed a different Definition.ID depending on whether it came from the
	// embedded pack or a Supabase mirror — breaking Task-175's requirement
	// that both sources return the same normalized shape. Use the pack's own
	// flow id when this is a mirrored built-in; fall back to the row's UUID
	// for a genuine user-owned flow, which has no other semantic id at all.
	defID := row.ID
	if row.PackFlowID != nil {
		defID = *row.PackFlowID
	}
	def := agentpack.FlowDefinition{
		ID:          defID,
		Description: row.Description,
	}
	if row.PolicyCap != nil {
		def.Policy.Cap = *row.PolicyCap
	}
	if row.PolicyOnCap != nil {
		def.Policy.OnCap = *row.PolicyOnCap
	}
	if row.PolicyExtendBy != nil {
		def.Policy.ExtendBy = *row.PolicyExtendBy
	}
	if row.PolicyExtendMax != nil {
		def.Policy.ExtendMax = *row.PolicyExtendMax
	}
	for _, e := range row.EdgesJSON {
		def.Edges = append(def.Edges, agentpack.FlowEdge{From: e.From, To: e.To, When: e.When, Kind: e.Kind})
	}
	if len(row.ContextsJSON) > 0 {
		def.Contexts = make(map[string]agentpack.FlowContextBinding, len(row.ContextsJSON))
		for name, binding := range row.ContextsJSON {
			def.Contexts[name] = agentpack.FlowContextBinding{Ref: binding.Ref}
		}
	}
	steps := append([]dbWorkflowStepRow(nil), row.WorkflowSteps...)
	sort.Slice(steps, func(i, j int) bool { return steps[i].OrderIndex < steps[j].OrderIndex })
	for _, st := range steps {
		defn := st.StepDefinition
		node := agentpack.FlowNode{}
		if defn.NodeID != nil {
			node.ID = *defn.NodeID
		}
		// BUG-282: derive this node's graph dependency from the flow's own
		// forward edges (workflows.edges_json — per-flow authoritative), NOT
		// from step_definitions.depends_on_json. step_definitions is a shared
		// catalog keyed by step_type, so a step reused across flows would
		// otherwise drag in the FIRST flow's node ids (absent from THIS flow)
		// and break entry/barrier resolution. Mirrors the client-side
		// computeDependsOnByStepType (WorkflowsSettings.tsx). depends_on_json
		// is no longer stored (dropped by migration).
		node.DependsOn = forwardEdgeSources(def.Edges, node.ID)
		if defn.NodeLifecycle != nil {
			node.Lifecycle = *defn.NodeLifecycle
		}
		if defn.BehaviorID != nil {
			node.Behavior = *defn.BehaviorID
		}
		if defn.AgentRef != nil {
			node.Agent = *defn.AgentRef
		}
		if defn.JoinMode != nil {
			node.Join = *defn.JoinMode
		}
		if defn.Cohort != nil {
			node.Cohort = *defn.Cohort
		}
		if defn.PromptTemplateRef != nil {
			node.PromptTemplate = *defn.PromptTemplateRef
		}
		if len(defn.ContextSources) > 0 {
			node.ContextSources = defn.ContextSources
		}
		for _, b := range defn.ArtifactBindings {
			binding := agentpack.FlowArtifactBinding{
				Direction:          b.Direction,
				SlotName:           b.SlotName,
				ArtifactInstanceID: b.ArtifactInstanceID,
				Required:           b.Required,
				Position:           b.Position,
			}
			// A binding whose joined artifact_instances row is missing (deleted
			// instance, stale FK left dangling by an out-of-band delete) is kept
			// with ArtifactTypeID left empty (SD-23 F-1) rather than dropped: an
			// empty type never matches any resolver's expected type, so
			// resolveArtifactBoundContextSources/resolveInputArtifactPrompt
			// already skip it exactly as if it were absent (a soft degrade for an
			// OPTIONAL binding), while ValidateFlowArtifactBindings
			// (context_sources_builtin.go) inspects this same empty-type sentinel
			// to fail flow-load fast for a REQUIRED one — callers must never
			// silently treat "instance gone" as "instance present."
			if b.ArtifactInstance != nil {
				binding.ArtifactTypeID = b.ArtifactInstance.ArtifactTypeID
				binding.ConfigJSON = b.ArtifactInstance.ConfigJSON
			}
			node.ArtifactBindings = append(node.ArtifactBindings, binding)
		}
		def.Nodes = append(def.Nodes, node)
	}
	rec.Definition = def
	return rec
}

func (s *SupabaseWorkflowFlowStore) fetchOne(ctx context.Context, query string) (FlowDefinitionRecord, bool, error) {
	endpoint := fmt.Sprintf("%s/workflows?%s&select=%s&limit=1", s.restURL, query, url.QueryEscape(workflowSelect))
	status, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return FlowDefinitionRecord{}, false, err
	}
	if status < 200 || status >= 300 {
		return FlowDefinitionRecord{}, false, fmt.Errorf("supabase workflow flow fetch failed: status %d: %s", status, string(body))
	}
	var rows []dbWorkflowRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return FlowDefinitionRecord{}, false, fmt.Errorf("supabase workflow flow decode: %w", err)
	}
	if len(rows) == 0 {
		return FlowDefinitionRecord{}, false, nil
	}
	return recordFromWorkflowRow(rows[0]), true, nil
}

// GetByPackFlow looks up a mirrored built-in row by pack identity.
func (s *SupabaseWorkflowFlowStore) GetByPackFlow(ctx context.Context, packID, packFlowID string) (FlowDefinitionRecord, bool, error) {
	query := fmt.Sprintf(
		"pack_id=eq.%s&pack_flow_id=eq.%s&is_builtin=eq.true",
		url.QueryEscape(packID), url.QueryEscape(packFlowID),
	)
	return s.fetchOne(ctx, query)
}

// flowHashID pairs a pack flow's stable id with its current content hash, the
// two signals ReclaimOrRetireStaleBuiltinMirrors needs to tell "this orphaned
// row IS one of the pack's current flows, just corrupted" from "this orphaned
// row genuinely no longer corresponds to anything."
type flowHashID struct {
	FlowID string
	Hash   string
}

// builtinStaleMirrorReclaimer is implemented by FlowDefinitionStore backends
// that can look up and patch a builtin mirror row by its own storage identity
// (id), not just by (pack_id, pack_flow_id) — BUG-249. FlowMirrorSyncService
// type-asserts for this optional capability; a store that doesn't implement
// it (e.g. the in-memory test fake) simply keeps the pre-BUG-249 behavior of
// the normal per-flow Upsert inserting a fresh row whenever a mirror is
// missing.
type builtinStaleMirrorReclaimer interface {
	ReclaimOrRetireStaleBuiltinMirrors(ctx context.Context, packID string, currentFlows []flowHashID) (handled int, err error)
}

type dbWorkflowIdentityRow struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	PackFlowID *string `json:"pack_flow_id"`
	PackHash   *string `json:"pack_hash"`
}

// staleMirrorSuffix marks a retired builtin mirror row's name so it reads as
// self-explanatory rather than a mysteriously duplicated entry. Also used to
// detect an already-retired row so a repeat sync doesn't append it twice.
const staleMirrorSuffix = " (stale mirror — pack_flow_id no longer matches any current pack flow; safe to review/delete)"

// ReclaimOrRetireStaleBuiltinMirrors implements builtinStaleMirrorReclaimer.
//
// BUG-249: a manually corrupted/renamed pack_flow_id (this is exactly CP-36
// Scenario 14's manual-corruption reproduction) orphans a builtin mirror row
// — Upsert's on_conflict=pack_id,pack_flow_id can no longer find it by its
// old identity, so the plain per-flow sync loop in SyncBuiltins would insert
// a brand-new row instead of repairing the existing one. That produced a
// visible duplicate "Built-in" entry in Settings' Definitions list and the
// Workflow Mode picker, and the fresh row lost whatever
// provider_override/model_override/reasoning_effort_override/yolo_mode the
// orphaned row had — those columns aren't part of FlowDefinitionRecord or the
// pack schema at all (they're pure per-installation admin settings), so a
// genuine INSERT falls through to the column's own Postgres default instead
// of anything meaningful.
//
// Call this BEFORE the normal per-flow sync loop, once per pack. A row whose
// pack_hash still matches one of currentFlows is reclaimed in place: only its
// pack_flow_id is patched back to the correct value, by row id — every other
// column (including the overrides) is left untouched, and the loop's own
// subsequent GetByPackFlow lookup then finds it looking like an already
// up-to-date mirror, so no separate "already reclaimed" branch is needed
// there. A row that cannot be reclaimed (content also changed, the flow was
// removed from the pack, or another row already occupies that flow's slot —
// i.e. the duplicate has already happened) is instead retired: is_builtin
// flips to false and editable to true so it stops rendering as a duplicate
// built-in, and its name gets a "(stale mirror...)" suffix. Retiring instead
// of deleting preserves any clone's `cloned_from` FK (Scenario 14 checklist
// item 3) and is non-destructive — the row and its history remain, just no
// longer masquerading as a live built-in.
func (s *SupabaseWorkflowFlowStore) ReclaimOrRetireStaleBuiltinMirrors(ctx context.Context, packID string, currentFlows []flowHashID) (int, error) {
	endpoint := fmt.Sprintf("%s/workflows?pack_id=eq.%s&is_builtin=eq.true&select=id,name,pack_flow_id,pack_hash",
		s.restURL, url.QueryEscape(packID))
	status, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return 0, err
	}
	if status < 200 || status >= 300 {
		return 0, fmt.Errorf("supabase workflow flow list builtins for %q: status %d: %s", packID, status, string(body))
	}
	var rows []dbWorkflowIdentityRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, fmt.Errorf("supabase workflow flow decode builtin identity rows: %w", err)
	}

	currentFlowIDs := make(map[string]bool, len(currentFlows))
	hashToFlowID := make(map[string]string, len(currentFlows))
	for _, f := range currentFlows {
		currentFlowIDs[f.FlowID] = true
		if f.Hash != "" {
			hashToFlowID[f.Hash] = f.FlowID
		}
	}
	// A flow slot already correctly occupied by a live row must never be
	// targeted by a reclaim patch too — that would violate the
	// unique(pack_id, pack_flow_id) index. This also covers the case where
	// the duplicate has already happened (both the orphan and a freshly
	// re-inserted correct row already exist): the orphan then falls through
	// to retirement instead of a doomed reclaim attempt.
	occupied := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.PackFlowID != nil && currentFlowIDs[*row.PackFlowID] {
			occupied[*row.PackFlowID] = true
		}
	}

	handled := 0
	for _, row := range rows {
		packFlowID := ""
		if row.PackFlowID != nil {
			packFlowID = *row.PackFlowID
		}
		if currentFlowIDs[packFlowID] {
			continue // a live, correctly-keyed mirror -- leave it alone
		}
		hash := ""
		if row.PackHash != nil {
			hash = *row.PackHash
		}
		if flowID, ok := hashToFlowID[hash]; ok && hash != "" && !occupied[flowID] {
			if err := s.patchWorkflowByID(ctx, row.ID, map[string]any{"pack_flow_id": flowID}); err != nil {
				return handled, fmt.Errorf("reclaim stale builtin mirror %q -> %q: %w", row.ID, flowID, err)
			}
			occupied[flowID] = true
			handled++
			continue
		}
		if strings.Contains(row.Name, staleMirrorSuffix) {
			continue // already retired and renamed in a prior pass
		}
		if err := s.patchWorkflowByID(ctx, row.ID, map[string]any{
			"is_builtin": false,
			"editable":   true,
			"name":       row.Name + staleMirrorSuffix,
		}); err != nil {
			return handled, fmt.Errorf("retire stale builtin mirror %q: %w", row.ID, err)
		}
		handled++
	}
	return handled, nil
}

// patchWorkflowByID applies a targeted column patch to a single workflows row
// by its own id, independent of the pack-identity-keyed on_conflict path
// Upsert uses.
func (s *SupabaseWorkflowFlowStore) patchWorkflowByID(ctx context.Context, id string, fields map[string]any) error {
	endpoint := s.restURL + "/workflows?id=eq." + url.QueryEscape(id)
	body, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPatch, endpoint, s.headers("return=minimal"), body)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("status %d: %s", status, string(respBody))
	}
	return nil
}

// GetByRef resolves a flowRef that is either a stable "packId/flowId" string
// (built-ins) or a workflow's own UUID id (user-owned rows).
func (s *SupabaseWorkflowFlowStore) GetByRef(ctx context.Context, flowRef string) (FlowDefinitionRecord, bool, error) {
	if looksLikeUUID(flowRef) {
		query := "id=eq." + url.QueryEscape(flowRef)
		return s.fetchOne(ctx, query)
	}
	packID, flowID, ok := splitFlowRef(flowRef)
	if !ok {
		return FlowDefinitionRecord{}, false, nil
	}
	return s.GetByPackFlow(ctx, packID, flowID)
}

// ListAll returns every workflow row (built-in mirrors and user-owned
// flows/plain admin workflows alike — a plain workflow whose joined step
// definitions have no behavior_id simply has no agent.delegate entry node, so
// it is inert to the flow executor).
func (s *SupabaseWorkflowFlowStore) ListAll(ctx context.Context) ([]FlowDefinitionRecord, error) {
	endpoint := fmt.Sprintf("%s/workflows?select=%s", s.restURL, url.QueryEscape(workflowSelect))
	status, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("supabase workflow flow list failed: status %d: %s", status, string(body))
	}
	var rows []dbWorkflowRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("supabase workflow flow decode: %w", err)
	}
	out := make([]FlowDefinitionRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, recordFromWorkflowRow(row))
	}
	return out, nil
}

// Upsert inserts or updates the record's workflow row and replaces its
// workflow_steps. Built-in mirror rows (PackID/PackFlowID set) upsert on the
// (pack_id, pack_flow_id) unique index; user-owned rows upsert on id — a
// record with an empty FlowRef is a brand-new row and gets a store-assigned
// UUID, returned as the persisted record's FlowRef.
func (s *SupabaseWorkflowFlowStore) Upsert(ctx context.Context, record FlowDefinitionRecord) (FlowDefinitionRecord, error) {
	payload := map[string]any{
		"name":                nameForRecord(record),
		"description":         record.Definition.Description,
		"editable":            record.Editable,
		"cloneable":           record.Cloneable,
		"cloned_from":         nilIfEmpty(record.ClonedFrom),
		"selectable_in_json":  nonNilStrings(record.SelectableIn),
		"chat_baseline":       record.ChatBaseline,
		"chat_sub_modes_json": nonNilStrings(record.ChatSubModes),
		"policy_cap":          record.Definition.Policy.Cap,
		"policy_on_cap":       nilIfEmpty(record.Definition.Policy.OnCap),
		"policy_extend_by":    record.Definition.Policy.ExtendBy,
		"policy_extend_max":   record.Definition.Policy.ExtendMax,
		"edges_json":          edgesPayload(record.Definition.Edges),
		"contexts_json":       contextsPayload(record.Definition.Contexts),
	}

	var endpoint string
	if record.PackID != "" && record.PackFlowID != "" {
		payload["is_builtin"] = true
		payload["pack_id"] = record.PackID
		payload["pack_version"] = nilIfEmpty(record.PackVersion)
		payload["pack_flow_id"] = record.PackFlowID
		payload["pack_hash"] = nilIfEmpty(record.PackHash)
		endpoint = s.restURL + "/workflows?on_conflict=pack_id,pack_flow_id"
	} else {
		payload["is_builtin"] = false
		if looksLikeUUID(record.FlowRef) {
			payload["id"] = record.FlowRef
			endpoint = s.restURL + "/workflows?on_conflict=id"
		} else {
			// Brand-new user-owned row: let Postgres assign the id.
			endpoint = s.restURL + "/workflows"
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return FlowDefinitionRecord{}, fmt.Errorf("supabase workflow flow: encode: %w", err)
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("resolution=merge-duplicates,return=representation"), body)
	if err != nil {
		return FlowDefinitionRecord{}, err
	}
	if status < 200 || status >= 300 {
		return FlowDefinitionRecord{}, fmt.Errorf("supabase workflow flow upsert failed: status %d: %s", status, string(respBody))
	}
	var savedRows []dbWorkflowRow
	if err := json.Unmarshal(respBody, &savedRows); err != nil {
		return FlowDefinitionRecord{}, fmt.Errorf("supabase workflow flow decode upsert response: %w", err)
	}
	if len(savedRows) == 0 {
		return FlowDefinitionRecord{}, fmt.Errorf("supabase workflow flow upsert: no row returned")
	}
	workflowID := savedRows[0].ID

	stepTypes, err := s.upsertNodeStepDefinitions(ctx, record, workflowID, record.Definition.Nodes)
	if err != nil {
		return FlowDefinitionRecord{}, err
	}

	if err := s.replaceSteps(ctx, workflowID, stepTypes); err != nil {
		return FlowDefinitionRecord{}, err
	}

	saved, ok, err := s.fetchOne(ctx, "id=eq."+url.QueryEscape(workflowID))
	if err != nil {
		return FlowDefinitionRecord{}, err
	}
	if !ok {
		return FlowDefinitionRecord{}, fmt.Errorf("supabase workflow flow: row %q disappeared after upsert", workflowID)
	}
	return saved, nil
}

// insertOrderIndexOffset shifts a replaceSteps insert's order_index values
// out of the live 0..N-1 range so the new rows never collide with
// workflow_steps' unique(workflow_id, order_index) constraint while the old
// rows they're about to replace are still present (see replaceSteps).
const insertOrderIndexOffset = 1_000_000

// replaceSteps swaps workflowID's relation rows for stepTypes. Ordered insert-then-delete
// (BUG-NOTE-CP42 #18): the old delete-then-insert sequence left a window
// where an insert failure (FK/unique/network error) permanently lost every
// existing step, since the delete had already committed. Inserting the new
// rows first means a failed insert leaves the existing steps fully intact —
// the caller sees an error and the stored flow is unchanged, not silently
// emptied.
func (s *SupabaseWorkflowFlowStore) replaceSteps(ctx context.Context, workflowID string, stepTypes []string) error {
	if len(stepTypes) == 0 {
		// Nothing to preserve: a deliberate "save with zero steps" has no
		// insert-first ordering to get right.
		return s.deleteAllSteps(ctx, workflowID)
	}

	newIDs, err := s.insertSteps(ctx, workflowID, stepTypes, insertOrderIndexOffset)
	if err != nil {
		return fmt.Errorf("supabase workflow flow: insert new steps (existing steps left untouched): %w", err)
	}

	if err := s.deleteStepsExcept(ctx, workflowID, newIDs); err != nil {
		return fmt.Errorf("supabase workflow flow: delete superseded steps (new steps were saved; stale rows may remain — retry the save to clean up): %w", err)
	}

	// Best-effort: renormalize order_index back to 0..N-1 now that the old
	// rows are gone. Cosmetic only — recordFromWorkflowRow sorts by
	// order_index, so nodes remain correctly ordered relative to each other
	// even if this step fails or is skipped; a failure here must not be
	// treated as a replaceSteps failure.
	s.renormalizeOrderIndex(ctx, newIDs)
	return nil
}

// deleteAllSteps removes every workflow_steps row for workflowID.
func (s *SupabaseWorkflowFlowStore) deleteAllSteps(ctx context.Context, workflowID string) error {
	endpoint := s.restURL + "/workflow_steps?workflow_id=eq." + url.QueryEscape(workflowID)
	status, body, err := httpRequestFn(ctx, http.MethodDelete, endpoint, s.headers("return=minimal"), nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase workflow flow: delete existing steps failed: status %d: %s", status, string(body))
	}
	return nil
}

func (s *SupabaseWorkflowFlowStore) upsertNodeStepDefinitions(ctx context.Context, record FlowDefinitionRecord, workflowID string, nodes []agentpack.FlowNode) ([]string, error) {
	stepTypes := make([]string, 0, len(nodes))
	rows := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		stepType := flowNodeStepType(record, workflowID, node)
		stepTypes = append(stepTypes, stepType)
		rows = append(rows, map[string]any{
			"step_type":           stepType,
			"name":                flowNodeStepName(record, node),
			"description":         flowNodeStepDescription(record, node),
			"prompt_base":         flowNodePromptBase(record, node),
			"agent_type":          "standard",
			"node_id":             nilIfEmpty(node.ID),
			"node_lifecycle":      nilIfEmpty(node.Lifecycle),
			"behavior_id":         nilIfEmpty(node.Behavior),
			"agent_ref":           nilIfEmpty(node.Agent),
			"join_mode":           nilIfEmpty(node.Join),
			"cohort":              nilIfEmpty(node.Cohort),
			"prompt_template_ref": nilIfEmpty(node.PromptTemplate),
		})
	}
	if len(rows) == 0 {
		return stepTypes, nil
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("encode step definitions: %w", err)
	}
	endpoint := s.restURL + "/step_definitions?on_conflict=step_type"
	status, body, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("resolution=merge-duplicates,return=minimal"), payload)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("upsert step definitions failed: status %d: %s", status, string(body))
	}
	return stepTypes, nil
}

// insertSteps inserts relation rows that attach already-upserted step definitions
// to workflowID (order_index offset by orderOffset) and returns their ids.
func (s *SupabaseWorkflowFlowStore) insertSteps(ctx context.Context, workflowID string, stepTypes []string, orderOffset int) ([]string, error) {
	rows := make([]map[string]any, 0, len(stepTypes))
	for i, stepType := range stepTypes {
		rows = append(rows, map[string]any{
			"workflow_id": workflowID,
			"step_type":   stepType,
			"order_index": orderOffset + i,
		})
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("encode steps: %w", err)
	}
	endpoint := s.restURL + "/workflow_steps"
	status, body, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("return=representation"), payload)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("insert steps failed: status %d: %s", status, string(body))
	}
	var inserted []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &inserted); err != nil {
		return nil, fmt.Errorf("decode inserted steps: %w", err)
	}
	ids := make([]string, 0, len(inserted))
	for _, row := range inserted {
		ids = append(ids, row.ID)
	}
	return ids, nil
}

// deleteStepsExcept removes every workflow_steps row for workflowID whose id
// is not in keepIDs.
func (s *SupabaseWorkflowFlowStore) deleteStepsExcept(ctx context.Context, workflowID string, keepIDs []string) error {
	quoted := make([]string, len(keepIDs))
	for i, id := range keepIDs {
		quoted[i] = url.QueryEscape(id)
	}
	endpoint := fmt.Sprintf("%s/workflow_steps?workflow_id=eq.%s&id=not.in.(%s)",
		s.restURL, url.QueryEscape(workflowID), strings.Join(quoted, ","))
	status, body, err := httpRequestFn(ctx, http.MethodDelete, endpoint, s.headers("return=minimal"), nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("status %d: %s", status, string(body))
	}
	return nil
}

// renormalizeOrderIndex patches each newly-inserted step's order_index back
// to its position in ids (0..N-1), undoing insertSteps' collision-avoidance
// offset. Errors are logged, not returned: this is cosmetic, and the caller
// must not treat a renormalization failure as a replaceSteps failure (the
// steps themselves are already saved correctly).
func (s *SupabaseWorkflowFlowStore) renormalizeOrderIndex(ctx context.Context, ids []string) {
	for i, id := range ids {
		endpoint := s.restURL + "/workflow_steps?id=eq." + url.QueryEscape(id)
		body, _ := json.Marshal(map[string]any{"order_index": i})
		status, respBody, err := httpRequestFn(ctx, http.MethodPatch, endpoint, s.headers("return=minimal"), body)
		if err != nil || status < 200 || status >= 300 {
			log.Printf("[flow-store] renormalize order_index for step %q failed (cosmetic only, ordering remains correct): status=%d err=%v body=%s",
				id, status, err, string(respBody))
		}
	}
}

func nameForRecord(record FlowDefinitionRecord) string {
	if strings.TrimSpace(record.Name) != "" {
		return record.Name
	}
	if record.PackFlowID != "" {
		return flowOptionLabel(record.PackFlowID)
	}
	return "Untitled Flow"
}

func flowNodeStepType(record FlowDefinitionRecord, workflowID string, node agentpack.FlowNode) string {
	flowKey := record.PackFlowID
	if flowKey == "" {
		flowKey = record.Definition.ID
	}
	if flowKey == "" {
		flowKey = workflowID
	}
	if record.PackID != "" {
		flowKey = record.PackID + "__" + flowKey
	}
	nodeKey := node.ID
	if nodeKey == "" {
		nodeKey = flowNodeAgentName(node)
	}
	if nodeKey == "" {
		nodeKey = "node"
	}
	return sanitizeStepType(flowKey + "__" + nodeKey)
}

func sanitizeStepType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case r == '-' || r == '_':
			if !lastUnderscore {
				b.WriteRune('_')
				lastUnderscore = true
			}
		default:
			if !lastUnderscore {
				b.WriteRune('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "flow_node"
	}
	return out
}

func flowNodeStepName(record FlowDefinitionRecord, node agentpack.FlowNode) string {
	flowName := nameForRecord(record)
	nodeName := strings.TrimSpace(node.ID)
	if nodeName == "" {
		nodeName = flowNodeAgentName(node)
	}
	if nodeName == "" {
		nodeName = "node"
	}
	return flowName + ": " + humanizeFlowNodeID(nodeName)
}

func humanizeFlowNodeID(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '_' || r == '-' })
	if len(parts) == 0 {
		return value
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func flowNodeStepDescription(record FlowDefinitionRecord, node agentpack.FlowNode) string {
	behavior := strings.TrimSpace(node.Behavior)
	if behavior == "" {
		behavior = "flow node"
	}
	return fmt.Sprintf("Builtin flow node %q for %s. Behavior: %s.", strings.TrimSpace(node.ID), nameForRecord(record), behavior)
}

func flowNodePromptBase(record FlowDefinitionRecord, node agentpack.FlowNode) string {
	return fmt.Sprintf("Execute the %q node in the %q flow.", strings.TrimSpace(node.ID), nameForRecord(record))
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func edgesPayload(edges []agentpack.FlowEdge) []map[string]string {
	out := make([]map[string]string, 0, len(edges))
	for _, e := range edges {
		out = append(out, map[string]string{"from": e.From, "to": e.To, "when": e.When, "kind": e.Kind})
	}
	return out
}

// contextsPayload flattens a flow's named context bindings
// ({name: {ref: "..."}}) into the JSON shape contexts_json stores.
func contextsPayload(contexts map[string]agentpack.FlowContextBinding) map[string]map[string]string {
	out := make(map[string]map[string]string, len(contexts))
	for name, binding := range contexts {
		out[name] = map[string]string{"ref": binding.Ref}
	}
	return out
}

// FlowDefinitionStoreFor picks the FlowDefinitionStore backing built-in
// mirror sync and flowRef resolution for a workspace, mirroring
// CatalogStoreFor's precedence (supabase_catalog_store.go): a Supabase-backed
// store when the workspace has a configured project + resolvable key,
// otherwise nil — resolution and mirror sync degrade gracefully without
// Supabase (FlowDefinitionResolver falls back to the embedded pack directly;
// mirror sync and cloning simply require Supabase to be configured, matching
// the desktop Settings UI's own requirement).
//
// BUG-NOTE-CP42 #19: this used to fall back to resp.AnonKey when the service
// role key was empty, but workflows/workflow_steps' RLS policies are scoped
// `to authenticated` — a bare anon key authenticates as Postgres role `anon`,
// not `authenticated`, so every mirror-sync write through that fallback would
// have been silently rejected by RLS (a confusing 401/403 with no clear
// "not configured" signal). The fallback was also already unreachable in
// practice: SaveSupabaseWorkspaceConfig requires a service role key to save
// the config at all ("service role key is required..."), so a configured
// workspace always has one. Require the service role key explicitly instead
// of silently degrading to a key that can't actually write these tables.
func FlowDefinitionStoreFor(r *Runner) FlowDefinitionStore {
	if r == nil {
		return nil
	}
	resp, err := r.LoadSupabaseWorkspaceConfigWithSecret()
	if err != nil {
		return nil
	}
	key := strings.TrimSpace(resp.ServiceRoleKey)
	if strings.TrimSpace(resp.APIURL) == "" || key == "" {
		return nil
	}
	return NewSupabaseWorkflowFlowStore(resp.SupabaseWorkspaceConfig, key)
}

// EnsureBuiltinFlowMirrorsWithStore mirrors every embedded pack flow into an
// arbitrary FlowDefinitionStore (nil-safe: it is simply a no-op error when
// store is nil, which cmd/flowpilot logs and ignores at startup — see
// FlowDefinitionStoreFor).
func EnsureBuiltinFlowMirrorsWithStore(ctx context.Context, store FlowDefinitionStore) ([]FlowDefinitionRecord, error) {
	if store == nil {
		return nil, nil
	}
	svc := NewFlowMirrorSyncService(store)
	return svc.SyncBuiltins(ctx)
}
