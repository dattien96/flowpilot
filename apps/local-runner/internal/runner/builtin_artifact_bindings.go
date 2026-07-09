package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// builtinContextArtifactInstanceID is the well-known, fixed UUID of the
// built-in default `context_artifact.v1` instance seeded by
// 20260709092000_add_builtin_context_artifact_instance.sql. Using a fixed
// literal (rather than gen_random_uuid()) lets this Go-side binding seed
// reference it deterministically without a lookup round-trip (CP-45/SD-23
// Task-205).
const builtinContextArtifactInstanceID = "00000000-0000-0000-0000-000000000001"

// builtinContextCodingReviewSynthesisFlowID is context-coding-review-
// synthesis.yaml's own flow id (Task-205's built-in artifact flow).
const builtinContextCodingReviewSynthesisFlowID = "context-coding-review-synthesis"

// builtinArtifactBindingSeeder is implemented by SupabaseWorkflowFlowStore
// only (mirrors the existing builtinStaleMirrorReclaimer optional-capability
// pattern): a FlowDefinitionStore that cannot seed artifact bindings (e.g.
// the in-memory test fake) simply skips this step, matching how
// SyncBuiltins already treats capability-detection as best-effort.
type builtinArtifactBindingSeeder interface {
	SeedBuiltinContextArtifactBindings(ctx context.Context, record FlowDefinitionRecord) error
}

// EnsureBuiltinArtifactBindingsWithStore seeds step_artifact_bindings for
// every synced built-in flow whose store supports it (CP-45/SD-23 Task-205).
// Best-effort and non-fatal, matching EnsureBuiltinFlowMirrorsWithStore's own
// contract — a failure here must never block the flow mirror sync it runs
// after.
func EnsureBuiltinArtifactBindingsWithStore(ctx context.Context, store FlowDefinitionStore, synced []FlowDefinitionRecord) error {
	seeder, ok := store.(builtinArtifactBindingSeeder)
	if !ok {
		return nil
	}
	var firstErr error
	for _, record := range synced {
		if record.PackFlowID != builtinContextCodingReviewSynthesisFlowID {
			continue
		}
		if err := seeder.SeedBuiltinContextArtifactBindings(ctx, record); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SeedBuiltinContextArtifactBindings wires the built-in default
// context_artifact instance (builtinContextArtifactInstanceID) as the
// OUTPUT of the flow's "context" node and the INPUT of every other node
// (coder, reviewer_correctness, reviewer_security, synthesis) — SD-23 D-5:
// "context_artifact.v1 là output của Context step, input của
// Coding/Review/Synthesis". Idempotent: upserts on the same
// (step_definition_id, direction, artifact_instance_id) unique key the
// migration declares, so re-running mirror sync never duplicates rows.
func (s *SupabaseWorkflowFlowStore) SeedBuiltinContextArtifactBindings(ctx context.Context, record FlowDefinitionRecord) error {
	type bindingRow struct {
		StepDefinitionID   string `json:"step_definition_id"`
		Direction          string `json:"direction"`
		SlotName           string `json:"slot_name"`
		ArtifactInstanceID string `json:"artifact_instance_id"`
		Required           bool   `json:"required"`
		Position           int    `json:"position"`
	}

	var rows []bindingRow
	for _, node := range record.Definition.Nodes {
		direction := "input"
		if node.ID == "context" {
			direction = "output"
		}
		// record.PackFlowID is always set here (this seeds only mirrored
		// built-in records — see EnsureBuiltinArtifactBindingsWithStore's
		// PackFlowID filter), so flowNodeStepType's workflowID fallback param
		// is unreachable; record.FlowRef is passed only to satisfy its
		// signature.
		stepType := flowNodeStepType(record, record.FlowRef, node)
		rows = append(rows, bindingRow{
			StepDefinitionID:   stepType,
			Direction:          direction,
			SlotName:           "main_context",
			ArtifactInstanceID: builtinContextArtifactInstanceID,
			Required:           direction == "output",
			Position:           0,
		})
	}
	if len(rows) == 0 {
		return nil
	}

	payload, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("seed builtin artifact bindings: encode: %w", err)
	}
	endpoint := s.restURL + "/step_artifact_bindings?on_conflict=step_definition_id,direction,artifact_instance_id"
	status, body, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("resolution=merge-duplicates,return=minimal"), payload)
	if err != nil {
		return fmt.Errorf("seed builtin artifact bindings: %w", err)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("seed builtin artifact bindings failed: status %d: %s", status, string(body))
	}
	return nil
}
