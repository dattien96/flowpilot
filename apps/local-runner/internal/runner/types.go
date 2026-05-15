package runner

type Health struct {
	Status        string `json:"status"`
	RunnerVersion string `json:"runnerVersion"`
	Cwd           string `json:"cwd"`
	Os            string `json:"os"`
	StartedAt     string `json:"startedAt"`
}

type Provider struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Installed   bool   `json:"installed"`
	Version     string `json:"version"`
	BinaryPath  string `json:"binaryPath"`
	AuthStatus  string `json:"authStatus"`
	InstallHint string `json:"installHint"`
}

type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	FilePath    string   `json:"filePath"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type Flow struct {
 	ID          string   `json:"id"`
 	Name        string   `json:"name"`
 	FilePath    string   `json:"filePath"`
 	Description string   `json:"description"`
 	Steps       []string `json:"steps"`
}

type PromptExecutionRequest struct {
	ProviderKey     string   `json:"providerKey"`
	Prompt          string   `json:"prompt"`
	SkillIds        []string `json:"skillIds"`
	FlowId          string   `json:"flowId"`
	ContextSourceIds []string `json:"contextSourceIds"`
	TimeoutMs       int      `json:"timeoutMs"`
	WorkingDirectory string  `json:"workingDirectory"`
}

type PromptExecutionResult struct {
	Status        string   `json:"status"`
	RunID         string   `json:"runId"`
	ProviderKey   string   `json:"providerKey"`
	Command       string   `json:"command"`
	StdoutSummary string   `json:"stdoutSummary"`
	StderrSummary string   `json:"stderrSummary"`
	OutputMarkdown string  `json:"outputMarkdown"`
	ArtifactPaths []string `json:"artifactPaths"`
	StartedAt     string   `json:"startedAt"`
	CompletedAt   string   `json:"completedAt"`
	ExitCode      int      `json:"exitCode"`
	ErrorMessage  string   `json:"errorMessage"`
}
