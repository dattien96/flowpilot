package runner

import (
	"fmt"
	"strings"

	"flowpilot-runner/internal/agentpack"
)

func vibeRequirementFace() FlowControlFace {
	if face, ok, err := agentpack.LoadBuiltinToolFace("vibe-requirement-outcome"); err == nil && ok && len(face.StatusMap) > 0 {
		return FlowControlFace{Tool: face.ID, Map: face.StatusMap}
	}
	return FlowControlFace{
		Tool: "vibe-requirement-outcome",
		Map: map[string]string{
			"aligned":            "done",
			"drift_fixable":      "continue",
			"requirement_change": "escalate",
		},
	}
}

type vibeRequirementInput struct {
	Verdict string
	Summary string
}

func parseVibeRequirementInput(args map[string]any) (vibeRequirementInput, error) {
	in := vibeRequirementInput{}
	if args == nil {
		return in, fmt.Errorf("vibe-requirement-outcome: missing arguments")
	}
	verdict, _ := args["verdict"].(string)
	in.Verdict = strings.TrimSpace(verdict)
	if in.Verdict == "" {
		return in, fmt.Errorf("vibe-requirement-outcome: verdict is required")
	}
	face := vibeRequirementFace()
	if _, ok := resolveFaceStatus(face, in.Verdict); !ok {
		return in, fmt.Errorf("vibe-requirement-outcome: verdict must be aligned|drift_fixable|requirement_change, got %q", in.Verdict)
	}
	in.Summary, _ = args["summary"].(string)
	return in, nil
}

func vibeRequirementToFlowControl(in vibeRequirementInput) (FlowControlInput, error) {
	face := vibeRequirementFace()
	generic, ok := resolveFaceStatus(face, in.Verdict)
	if !ok {
		return FlowControlInput{}, fmt.Errorf("vibeRequirementToFlowControl: unknown verdict %q", in.Verdict)
	}
	return FlowControlInput{Status: generic, Summary: strings.TrimSpace(in.Summary)}, nil
}
