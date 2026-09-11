package runner

import (
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func pickerSet(mode string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, id := range workingmode.FlowPickerOptions(mode) {
		out[id] = struct{}{}
	}
	return out
}

func requirePickerHas(t *testing.T, set map[string]struct{}, id string) {
	t.Helper()
	if _, ok := set[id]; !ok {
		t.Fatalf("picker missing %s: %v", id, set)
	}
}

func requirePickerOmit(t *testing.T, set map[string]struct{}, id string) {
	t.Helper()
	if _, ok := set[id]; ok {
		t.Fatalf("picker leaked %s", id)
	}
}

// Scenario: vibe picker is vibe-ingest then vibe-cp-ingest.
// Input: flowPickerOptions(vibe)
// Expect: ids == [vibe-ingest, vibe-cp-ingest]
func TestPicker_VibeListsOnlyIngest(t *testing.T) {
	ids := workingmode.FlowPickerOptions("vibe")
	if len(ids) != 2 || ids[0] != "vibe-ingest" || ids[1] != "vibe-cp-ingest" {
		t.Fatalf("vibe picker = %v, want [vibe-ingest vibe-cp-ingest]", ids)
	}
}

// Scenario: dev picker is exactly the five harness ids.
// Input: flowPickerOptions(dev)
// Expect: id set == five harness; count==5
func TestPicker_DevListsHarnessFamily(t *testing.T) {
	ids := workingmode.FlowPickerOptions("dev")
	if len(ids) != 5 {
		t.Fatalf("dev picker count=%d ids=%v, want 5", len(ids), ids)
	}
	set := pickerSet("dev")
	for _, id := range workingmode.DevHarnessFive {
		requirePickerHas(t, set, id)
	}
}

// Scenario: missing mode lists as dev.
// Input: flowPickerOptions("")
// Expect: same id set as flowPickerOptions(dev)
func TestPicker_EmptyModeListsDev(t *testing.T) {
	empty := workingmode.FlowPickerOptions("")
	dev := workingmode.FlowPickerOptions("dev")
	if len(empty) != len(dev) {
		t.Fatalf("empty=%v dev=%v", empty, dev)
	}
	for i := range empty {
		if empty[i] != dev[i] {
			t.Fatalf("empty=%v dev=%v", empty, dev)
		}
	}
}

// Scenario: owner-debate never listed.
func TestPicker_OwnerDebateNeverListed(t *testing.T) {
	requirePickerOmit(t, pickerSet("dev"), "vibe-owner-debate")
	requirePickerOmit(t, pickerSet("vibe"), "vibe-owner-debate")
}

// Scenario: vibe-sprint never listed.
func TestPicker_SprintNeverListed(t *testing.T) {
	requirePickerOmit(t, pickerSet("dev"), "vibe-sprint")
	requirePickerOmit(t, pickerSet("vibe"), "vibe-sprint")
}

// Scenario: vibe-cp-ingest listed in vibe, omitted in dev.
func TestPicker_CpIngestNeverListed(t *testing.T) {
	requirePickerOmit(t, pickerSet("dev"), "vibe-cp-ingest")
	requirePickerHas(t, pickerSet("vibe"), "vibe-cp-ingest")
}

// Scenario: vibe list has none of the five harness ids.
func TestPicker_VibeOmitsHarnessFamily(t *testing.T) {
	set := pickerSet("vibe")
	for _, id := range workingmode.DevHarnessFive {
		requirePickerOmit(t, set, id)
	}
}

// Scenario: dev list has no vibe-* ids.
func TestPicker_DevOmitsVibeFamily(t *testing.T) {
	set := pickerSet("dev")
	requirePickerOmit(t, set, "vibe-ingest")
	requirePickerOmit(t, set, "vibe-sprint")
	requirePickerOmit(t, set, "vibe-owner-debate")
}

// Scenario: review-loop / rag-harness / cp-harness-smoke stay hidden both modes.
func TestPicker_HiddenFlowsStayHidden(t *testing.T) {
	for _, mode := range []string{"dev", "vibe"} {
		set := pickerSet(mode)
		requirePickerOmit(t, set, "review-loop")
		requirePickerOmit(t, set, "rag-harness")
		requirePickerOmit(t, set, "cp-harness-smoke")
	}
}
