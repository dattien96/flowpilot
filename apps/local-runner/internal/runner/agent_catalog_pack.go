package runner

import "flowpilot-runner/internal/agentpack"

func loadBuiltinAgentDefinitionsFromPack() ([]AgentDefinition, error) {
	specs, err := agentpack.LoadBuiltinAgents()
	if err != nil {
		return nil, err
	}
	defs := make([]AgentDefinition, 0, len(specs))
	for _, spec := range specs {
		defs = append(defs, AgentDefinition{
			Name:                 spec.Name,
			Description:          spec.Description,
			Role:                 spec.Role,
			Provider:             spec.Provider,
			Model:                spec.Model,
			ModelReasoningEffort: spec.ModelReasoningEffort,
			Tools:                append([]string(nil), spec.Tools...),
			SystemPrompt:         spec.SystemPrompt,
			Source:               spec.Source,
			Path:                 spec.Path,
		})
	}
	return defs, nil
}
