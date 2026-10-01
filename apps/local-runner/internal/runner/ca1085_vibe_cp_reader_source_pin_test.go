package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CA-1085: live run-2232 — the user picked a CP in the armed bar and hit
// Start flow; the forward fence passed (sourceDocID pinned) and the flow
// launched cp_reader, but the spawned vibe-intake agent's prompt carried
// only the generic agent definition ("Read the raw requirement file or
// pasted idea") plus the "hi" chat transcript — no CP path. The agent
// globbed requirements/, found "no obvious intake file", and asked the
// user for the requirement it was already given. cp_reader declares no
// promptTemplate and no cp_md input binding, so
// appendResolvedVibeTemplatedInputs emits nothing even though
// rs.vibeLockedCP was stamped at launch (interactive_service.go).

// cp_reader in every CP-sourced vibe flow must compose an entry prompt
// that names the run's pinned CP file — same resolved-source seam the
// task_slicer got in BUG-469.
func TestCA1085_CpReaderEntryPromptNamesPinnedCP(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	pinned := "requirements/07-Coding-Plan/todo/CP-90-live-start-from-cp.md"
	for _, flowID := range []string{"vibe-tasks", "vibe-cp-ingest"} {
		var reader *agentpack.FlowNode
		for _, def := range pack.Flows {
			if def.ID != flowID {
				continue
			}
			for i := range def.Nodes {
				if def.Nodes[i].ID == "cp_reader" {
					reader = &def.Nodes[i]
				}
			}
		}
		if reader == nil {
			t.Fatalf("flow %q has no cp_reader node", flowID)
		}
		prompt := composeFlowNodeAgentPrompt(t.TempDir(), "hi", *reader)
		prompt = appendResolvedVibeTemplatedInputs(prompt, *reader, pinned)
		if !strings.Contains(prompt, pinned) {
			t.Fatalf("flow %q cp_reader prompt lacks the run's pinned CP path %q:\n%s", flowID, pinned, prompt)
		}
	}
}

// cp_reader must declare the cp_md file_artifact INPUT binding (the slot
// appendResolvedVibeTemplatedInputs resolves against) and a promptTemplate
// describing the read-the-pinned-CP job — the bare vibe-intake agent def
// tells the agent to convert a "raw requirement" into SS drafts, which is
// vibe-ingest's job, not this node's.
func TestCA1085_CpReaderDeclaresCpMdInputAndPromptTemplate(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	for _, flowID := range []string{"vibe-tasks", "vibe-cp-ingest"} {
		var reader *agentpack.FlowNode
		for _, def := range pack.Flows {
			if def.ID != flowID {
				continue
			}
			for i := range def.Nodes {
				if def.Nodes[i].ID == "cp_reader" {
					reader = &def.Nodes[i]
				}
			}
		}
		if reader == nil {
			t.Fatalf("flow %q has no cp_reader node", flowID)
		}
		if strings.TrimSpace(reader.PromptTemplate) == "" {
			t.Fatalf("flow %q cp_reader has no promptTemplate", flowID)
		}
		if _, ok, err := agentpack.LoadBuiltinPrompt(reader.PromptTemplate); err != nil || !ok {
			t.Fatalf("flow %q cp_reader promptTemplate %q does not resolve in the embedded pack: ok=%v err=%v",
				flowID, reader.PromptTemplate, ok, err)
		}
		hasCpInput := false
		for _, b := range reader.ArtifactBindings {
			if b.Direction == "input" && b.ArtifactInstanceID == "cp_md" && b.ArtifactTypeID == ArtifactTypeFile {
				if _, ok := b.ConfigJSON["pathTemplate"].(string); ok {
					hasCpInput = true
				}
			}
		}
		if !hasCpInput {
			t.Fatalf("flow %q cp_reader missing cp_md file_artifact input binding with pathTemplate: %#v", flowID, reader.ArtifactBindings)
		}
	}
}
