package runner

import (
	"errors"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func requireGateOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
}

func requireGateCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s, got nil", code)
	}
	var we *workingmode.Error
	if !errors.As(err, &we) || we.Code != code {
		t.Fatalf("want code %s, got %v", code, err)
	}
}

// Scenario: normal user may start task-harness.
// Input: mode=dev startKind=user flowID=task-harness
// Expect: nil error
func TestFlowAllowed_DevUserTaskHarness(t *testing.T) {
	requireGateOK(t, workingmode.FlowAllowedForWorkingMode("dev", "task-harness", "user"))
}

// Scenario: normal user may start each of the five harness ids.
// Input: mode=dev startKind=user; table of the five ids
// Expect: nil error per named subtest
func TestFlowAllowed_DevUserHarnessFamily(t *testing.T) {
	for _, id := range workingmode.DevHarnessFive {
		t.Run(id, func(t *testing.T) {
			requireGateOK(t, workingmode.FlowAllowedForWorkingMode("dev", id, "user"))
		})
	}
}

// Scenario: vibe user may start vibe-ingest (bare and pack-prefixed).
// Input: mode=vibe startKind=user flowID=vibe-ingest and flowpilot-core-flow-pack/vibe-ingest
// Expect: nil error both
func TestFlowAllowed_VibeUserIngest(t *testing.T) {
	requireGateOK(t, workingmode.FlowAllowedForWorkingMode("vibe", "vibe-ingest", "user"))
	requireGateOK(t, workingmode.FlowAllowedForWorkingMode("vibe", "flowpilot-core-flow-pack/vibe-ingest", "user"))
}

// Scenario: future system child may start vibe-sprint only on a vibe run.
// Input: mode=vibe startKind=system flowID=vibe-sprint
// Expect: nil error
func TestFlowAllowed_VibeSystemSprint(t *testing.T) {
	requireGateOK(t, workingmode.FlowAllowedForWorkingMode("vibe", "vibe-sprint", "system"))
}

// Scenario: future resolver may start vibe-owner-debate only on a vibe run.
// Input: mode=vibe startKind=system flowID=vibe-owner-debate
// Expect: nil error
func TestFlowAllowed_VibeSystemOwnerDebate(t *testing.T) {
	requireGateOK(t, workingmode.FlowAllowedForWorkingMode("vibe", "vibe-owner-debate", "system"))
}

// Scenario: missing mode is dev.
// Input: mode="" startKind=user flowID=task-harness
// Expect: nil error
func TestFlowAllowed_EmptyModeDefaultsDev(t *testing.T) {
	requireGateOK(t, workingmode.FlowAllowedForWorkingMode("", "task-harness", "user"))
}

// Scenario: empty startKind is user (fail-closed).
// Input: mode=vibe startKind="" flowID=vibe-sprint
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_EmptyStartKindIsUser(t *testing.T) {
	requireGateCode(t, workingmode.FlowAllowedForWorkingMode("vibe", "vibe-sprint", ""), workingmode.CodeFlowForbidden)
}

// Scenario: UI word "normal" is not a wire value.
// Input: mode=normal startKind=user flowID=task-harness
// Expect: error code invalid_working_mode
func TestFlowAllowed_NormalAliasRejectedOnWire(t *testing.T) {
	requireGateCode(t, workingmode.FlowAllowedForWorkingMode("normal", "task-harness", "user"), workingmode.CodeInvalidMode)
}

// Scenario: unknown mode rejected.
// Input: mode=prod startKind=user flowID=task-harness
// Expect: error code invalid_working_mode
func TestFlowAllowed_UnknownModeRejected(t *testing.T) {
	requireGateCode(t, workingmode.FlowAllowedForWorkingMode("prod", "task-harness", "user"), workingmode.CodeInvalidMode)
}

// Scenario: vibe-cp-ingest is user-startable after Task-321.
// Input: mode=vibe startKind=user flowID=vibe-cp-ingest
// Expect: nil error
func TestFlowAllowed_VibeUserCpIngestForbidden(t *testing.T) {
	requireGateOK(t, workingmode.FlowAllowedForWorkingMode("vibe", "vibe-cp-ingest", "user"))
}

// Scenario: empty flow id is invalid_flow_ref.
// Input: mode=dev startKind=user flowID=""
// Expect: error code invalid_flow_ref; no panic
func TestFlowAllowed_EmptyFlowID(t *testing.T) {
	requireGateCode(t, workingmode.FlowAllowedForWorkingMode("dev", "", "user"), workingmode.CodeInvalidFlowRef)
}

// Scenario: system cannot start vibe-sprint on a dev run.
// Input: mode=dev startKind=system flowID=vibe-sprint
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_DevSystemSprintForbidden(t *testing.T) {
	requireGateCode(t, workingmode.FlowAllowedForWorkingMode("dev", "vibe-sprint", "system"), workingmode.CodeFlowForbidden)
}

// Scenario: system cannot start vibe-owner-debate on a dev run.
// Input: mode=dev startKind=system flowID=vibe-owner-debate
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_DevSystemOwnerDebateForbidden(t *testing.T) {
	requireGateCode(t, workingmode.FlowAllowedForWorkingMode("dev", "vibe-owner-debate", "system"), workingmode.CodeFlowForbidden)
}

// Scenario: system cannot start harness on a vibe run.
// Input: mode=vibe startKind=system flowID=task-harness
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_VibeSystemHarnessForbidden(t *testing.T) {
	requireGateCode(t, workingmode.FlowAllowedForWorkingMode("vibe", "task-harness", "system"), workingmode.CodeFlowForbidden)
}

// Scenario: pack-prefixed harness is still forbidden in vibe.
// Input: mode=vibe startKind=user flowID=flowpilot-core-flow-pack/task-harness
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_VibeUserPackPrefixedHarnessForbidden(t *testing.T) {
	requireGateCode(t, workingmode.FlowAllowedForWorkingMode("vibe", "flowpilot-core-flow-pack/task-harness", "user"), workingmode.CodeFlowForbidden)
}

// Scenario: pack-prefixed vibe-ingest is still forbidden in dev.
// Input: mode=dev startKind=user flowID=flowpilot-core-flow-pack/vibe-ingest
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_DevUserPackPrefixedVibeForbidden(t *testing.T) {
	requireGateCode(t, workingmode.FlowAllowedForWorkingMode("dev", "flowpilot-core-flow-pack/vibe-ingest", "user"), workingmode.CodeFlowForbidden)
}

// Scenario: vibe user cannot start task-harness.
// Input: mode=vibe startKind=user flowID=task-harness
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_VibeUserTaskHarnessForbidden(t *testing.T) {
	requireGateCode(t, workingmode.FlowAllowedForWorkingMode("vibe", "task-harness", "user"), workingmode.CodeFlowForbidden)
}

// Scenario: vibe user cannot start any other harness id.
// Input: mode=vibe startKind=user; table of the remaining four harness ids
// Expect: error code working_mode_flow_forbidden per named subtest
func TestFlowAllowed_VibeUserHarnessFamilyForbidden(t *testing.T) {
	for _, id := range workingmode.DevHarnessFive[1:] {
		t.Run(id, func(t *testing.T) {
			requireGateCode(t, workingmode.FlowAllowedForWorkingMode("vibe", id, "user"), workingmode.CodeFlowForbidden)
		})
	}
}

// Scenario: dev user cannot start any vibe-* id including unlanded cp-ingest.
// Input: mode=dev startKind=user flowID in {vibe-ingest, vibe-sprint, vibe-owner-debate, vibe-cp-ingest}
// Expect: error code working_mode_flow_forbidden each
func TestFlowAllowed_DevUserVibeFamilyForbidden(t *testing.T) {
	for _, id := range []string{"vibe-ingest", "vibe-sprint", "vibe-owner-debate", "vibe-cp-ingest"} {
		t.Run(id, func(t *testing.T) {
			requireGateCode(t, workingmode.FlowAllowedForWorkingMode("dev", id, "user"), workingmode.CodeFlowForbidden)
		})
	}
}

// Scenario: user can never start sprint or owner-debate.
// Input: startKind=user flowID in {vibe-sprint, vibe-owner-debate} mode in {dev, vibe}
// Expect: error code working_mode_flow_forbidden all four
func TestFlowAllowed_UserNeverStartsSprintOrDebate(t *testing.T) {
	for _, mode := range []string{"dev", "vibe"} {
		for _, id := range []string{"vibe-sprint", "vibe-owner-debate"} {
			t.Run(mode+"/"+id, func(t *testing.T) {
				requireGateCode(t, workingmode.FlowAllowedForWorkingMode(mode, id, "user"), workingmode.CodeFlowForbidden)
			})
		}
	}
}
