package agentpack

import (
	"strings"
	"testing"
)

// TestValidateArtifactOutputPath (CP-58 Task-307 T-8) pins the deterministic
// path contract for file_artifact OUTPUT slots: workspace-relative, no
// traversal, no Windows drive/separator, always under requirements/.
func TestValidateArtifactOutputPath(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantErr string // empty means must pass
	}{
		{name: "task template under requirements", path: "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md"},
		{name: "cp template under requirements", path: "requirements/07-Coding-Plan/todo/CP-{{cpID}}-{{slug}}.md"},
		{name: "concrete path under requirements", path: "requirements/08-Task/todo/Task-123-slug.md"},
		{name: "parent traversal", path: "../../etc/passwd", wantErr: "must be a workspace-relative path under requirements/"},
		{name: "requirements escaping traversal", path: "requirements/../../escape.md", wantErr: "must be a workspace-relative path under requirements/"},
		{name: "windows drive path", path: `D:\out.md`, wantErr: "must be a workspace-relative path under requirements/"},
		{name: "absolute path", path: "/etc/passwd", wantErr: "must be a workspace-relative path under requirements/"},
		{name: "outside requirements", path: "apps/local-runner/out.md", wantErr: "must be under requirements/"},
		{name: "empty path", path: "   ", wantErr: "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateArtifactOutputPath(tc.path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateArtifactOutputPath(%q) = %v, want nil", tc.path, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateArtifactOutputPath(%q) = nil, want error containing %q", tc.path, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// TestValidateFlowDefinitionRejectsBadOutputBinding proves the fail-closed
// half: a flow whose file_artifact OUTPUT binding points outside requirements/
// (or has no target at all) fails ValidateFlowDefinition — the SD-23 D-11
// load-time contract — while the same flow with a valid template passes.
func TestValidateFlowDefinitionRejectsBadOutputBinding(t *testing.T) {
	baseNodes := func(config map[string]any) []FlowNode {
		return []FlowNode{
			{ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/coder.md", ArtifactBindings: []FlowArtifactBinding{{
				Direction: "output", SlotName: "plan_md", ArtifactInstanceID: "plan_md",
				ArtifactTypeID: "file_artifact.v1", Required: true, ConfigJSON: config,
			}}},
		}
	}
	if err := ValidateFlowDefinition(FlowDefinition{ID: "bad-escape", Nodes: baseNodes(map[string]any{
		"pathTemplate": "../../etc/passwd",
	})}); err == nil {
		t.Fatal("expected an error for an OUTPUT pathTemplate escaping requirements/")
	}
	if err := ValidateFlowDefinition(FlowDefinition{ID: "bad-drive", Nodes: baseNodes(map[string]any{
		"pathTemplate": `D:\out.md`,
	})}); err == nil {
		t.Fatal("expected an error for a Windows drive OUTPUT path")
	}
	if err := ValidateFlowDefinition(FlowDefinition{ID: "no-target", Nodes: baseNodes(map[string]any{
		"required": true,
	})}); err == nil {
		t.Fatal("expected an error for an OUTPUT binding with neither paths nor pathTemplate")
	}
	if err := ValidateFlowDefinition(FlowDefinition{ID: "good-template", Nodes: baseNodes(map[string]any{
		"pathTemplate": "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md",
	})}); err != nil {
		t.Fatalf("ValidateFlowDefinition(valid template) = %v, want nil", err)
	}
}

// TestTaskHarnessPlanArtifactBindings (CP-58 Task-307 CG-8) asserts the
// harness plan output bindings survive the FS loader: plan_writer declares one
// required file_artifact OUTPUT slot (plan_md) with the Task template, and
// plan_reviewer consumes it as a required INPUT.
func TestTaskHarnessPlanArtifactBindings(t *testing.T) {
	def := taskHarnessDefinition(t)

	planWriter := findNodeByID(t, def, "plan_writer")
	if len(planWriter.ArtifactBindings) != 1 {
		t.Fatalf("plan_writer binding count = %d, want 1", len(planWriter.ArtifactBindings))
	}
	out := planWriter.ArtifactBindings[0]
	if out.Direction != "output" || out.SlotName != "plan_md" || out.ArtifactInstanceID != "plan_md" {
		t.Fatalf("plan_writer binding = %+v, want output/plan_md/plan_md", out)
	}
	if out.ArtifactTypeID != "file_artifact.v1" || !out.Required {
		t.Fatalf("plan_writer binding type/required = %q/%v, want file_artifact.v1/true", out.ArtifactTypeID, out.Required)
	}
	tmpl, _ := out.ConfigJSON["pathTemplate"].(string)
	if tmpl != "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md" {
		t.Fatalf("plan_writer pathTemplate = %q, want the Task template", tmpl)
	}

	planReviewer := findNodeByID(t, def, "plan_reviewer")
	if len(planReviewer.ArtifactBindings) != 1 {
		t.Fatalf("plan_reviewer binding count = %d, want 1", len(planReviewer.ArtifactBindings))
	}
	in := planReviewer.ArtifactBindings[0]
	if in.Direction != "input" || in.SlotName != "plan_md" || !in.Required {
		t.Fatalf("plan_reviewer binding = %+v, want required input/plan_md", in)
	}
}
