package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// CP-58 Task-307: the harness plan artifact instances seeded by
// 20260831090000_add_harness_plan_artifact_instances.sql, keyed by the slot
// name the embedded pack YAML binds with (artifactInstanceId: plan_md /
// cp_md / task_md). Fixed literals, same pattern as
// builtinContextArtifactInstanceID, so the binding seed needs no lookup.
const (
	builtinHarnessPlanMdInstanceID = "00000000-0000-0000-0000-000000000002"
	builtinHarnessCpMdInstanceID   = "00000000-0000-0000-0000-000000000003"
	builtinHarnessTaskMdInstanceID = "00000000-0000-0000-0000-000000000004"
)

// builtinHarnessArtifactInstanceIDs maps the pack YAML slot names to the
// well-known instance UUIDs. A YAML binding whose artifactInstanceId is not
// in this map is skipped by the seed (the FS path still resolves it by its
// own id; only the DB mirror needs the UUID mapping).
var builtinHarnessArtifactInstanceIDs = map[string]string{
	"plan_md": builtinHarnessPlanMdInstanceID,
	"cp_md":   builtinHarnessCpMdInstanceID,
	"task_md": builtinHarnessTaskMdInstanceID,
}

// builtinHarnessArtifactFlowIDs are the CP-58 harness flows whose mirrored
// definitions carry node-level file_artifact bindings (Task-305/306).
var builtinHarnessArtifactFlowIDs = map[string]struct{}{
	"task-harness":     {},
	"cp-harness":       {},
	"cp-harness-smoke": {},
}

// builtinHarnessArtifactBindingSeeder is implemented by
// SupabaseWorkflowFlowStore only (CP-58 Task-307): seeds step_artifact_bindings
// for the harness plan outputs. Kept as a SEPARATE capability interface from
// builtinArtifactBindingSeeder so existing fakes implementing the Task-205
// contract keep compiling unchanged.
type builtinHarnessArtifactBindingSeeder interface {
	SeedBuiltinHarnessArtifactBindings(ctx context.Context, record FlowDefinitionRecord) error
}

// builtinArtifactBindingSeeder is implemented by SupabaseWorkflowFlowStore
// only (mirrors the existing builtinStaleMirrorReclaimer optional-capability
// pattern): a FlowDefinitionStore that cannot seed artifact bindings (e.g.
// the in-memory test fake) simply skips this step, matching how
// SyncBuiltins already treats capability-detection as best-effort.
type builtinArtifactBindingSeeder interface {
	SeedBuiltinContextArtifactBindings(ctx context.Context, record FlowDefinitionRecord) error
}

// EnsureBuiltinArtifactBindingsWithStore seeds step_artifact_bindings for
// every synced built-in flow whose store supports it (CP-45/SD-23 Task-205,
// extended by CP-58 Task-307 for the harness plan artifacts). Best-effort and
// non-fatal, matching EnsureBuiltinFlowMirrorsWithStore's own contract — a
// failure here must never block the flow mirror sync it runs after.
func EnsureBuiltinArtifactBindingsWithStore(ctx context.Context, store FlowDefinitionStore, synced []FlowDefinitionRecord) error {
	var firstErr error
	if seeder, ok := store.(builtinArtifactBindingSeeder); ok {
		for _, record := range synced {
			if record.PackFlowID != builtinContextCodingReviewSynthesisFlowID {
				continue
			}
			if err := seeder.SeedBuiltinContextArtifactBindings(ctx, record); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	if seeder, ok := store.(builtinHarnessArtifactBindingSeeder); ok {
		for _, record := range synced {
			if _, isHarness := builtinHarnessArtifactFlowIDs[record.PackFlowID]; !isHarness {
				continue
			}
			if err := seeder.SeedBuiltinHarnessArtifactBindings(ctx, record); err != nil && firstErr == nil {
				firstErr = err
			}
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

// SeedBuiltinHarnessArtifactBindings (CP-58 Task-307) seeds
// step_artifact_bindings for the harness flows' node-level file_artifact
// bindings. The synced record's Definition comes from the embedded pack
// (whose FS loader now parses node artifactBindings), so this walks the
// record's own bindings and maps each YAML slot name (plan_md/cp_md/task_md)
// to its well-known instance UUID. A binding whose slot has no seeded
// instance is skipped — the FS resolution path still owns it. Idempotent:
// upserts on the same (step_definition_id, direction, artifact_instance_id)
// unique key as the Task-205 seed.
func (s *SupabaseWorkflowFlowStore) SeedBuiltinHarnessArtifactBindings(ctx context.Context, record FlowDefinitionRecord) error {
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
		for _, b := range node.ArtifactBindings {
			instanceID, ok := builtinHarnessArtifactInstanceIDs[strings.TrimSpace(b.ArtifactInstanceID)]
			if !ok {
				continue
			}
			rows = append(rows, bindingRow{
				StepDefinitionID:   flowNodeStepType(record, record.FlowRef, node),
				Direction:          b.Direction,
				SlotName:           b.SlotName,
				ArtifactInstanceID: instanceID,
				Required:           b.Required,
				Position:           b.Position,
			})
		}
	}
	if len(rows) == 0 {
		return nil
	}

	payload, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("seed harness artifact bindings: encode: %w", err)
	}
	endpoint := s.restURL + "/step_artifact_bindings?on_conflict=step_definition_id,direction,artifact_instance_id"
	status, body, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("resolution=merge-duplicates,return=minimal"), payload)
	if err != nil {
		return fmt.Errorf("seed harness artifact bindings: %w", err)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("seed harness artifact bindings failed: status %d: %s", status, string(body))
	}
	return nil
}
