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

type ArtifactSummary struct {
	ArtifactID      string `json:"artifactId"`
	Title           string `json:"title"`
	SourceKind      string `json:"sourceKind"`
	ProjectID       string `json:"projectId"`
	FeatureID       string `json:"featureId"`
	WorkflowRunID   string `json:"workflowRunId"`
	WorkflowStepKey string `json:"workflowStepKey"`
	ProviderKey     string `json:"providerKey"`
	LocalPath       string `json:"localPath"`
	RemotePath      string `json:"remotePath"`
	RemoteURL       string `json:"remoteUrl"`
	SyncStatus      string `json:"syncStatus"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	ContentMarkdown string `json:"contentMarkdown"`
	PreviewMarkdown string `json:"previewMarkdown"`
}

type ArtifactDetail struct {
	ArtifactSummary
	ManifestPath string `json:"manifestPath"`
	PromptPath   string `json:"promptPath"`
	StdoutPath   string `json:"stdoutPath"`
	StderrPath   string `json:"stderrPath"`
	CommandPath  string `json:"commandPath"`
	ContentPath  string `json:"contentPath"`
	Checksum     string `json:"checksum"`
}

type StorageDriverConfig struct {
	DriverKey        string `json:"driverKey"`
	Enabled          bool   `json:"enabled"`
	RemoteRootPath   string `json:"remoteRootPath"`
	RemoteFolderName string `json:"remoteFolderName"`
	LastValidatedAt  string `json:"lastValidatedAt"`
	LastSyncedAt     string `json:"lastSyncedAt"`
	LastError        string `json:"lastError"`
	UpdatedAt        string `json:"updatedAt"`
}

type BackupRequest struct {
	Scope string `json:"scope"`
	RunID string `json:"runId"`
}

type BackupResult struct {
	BackupPath  string `json:"backupPath"`
	ArchiveName string `json:"archiveName"`
	CreatedAt   string `json:"createdAt"`
}

type PromptExecutionRequest struct {
	ProviderKey      string   `json:"providerKey"`
	Prompt           string   `json:"prompt"`
	SkillIds         []string `json:"skillIds"`
	FlowId           string   `json:"flowId"`
	ContextSourceIds []string `json:"contextSourceIds"`
	TimeoutMs        int      `json:"timeoutMs"`
	WorkingDirectory string   `json:"workingDirectory"`
}

type PromptExecutionResult struct {
	Status         string   `json:"status"`
	RunID          string   `json:"runId"`
	ProviderKey    string   `json:"providerKey"`
	Command        string   `json:"command"`
	StdoutSummary  string   `json:"stdoutSummary"`
	StderrSummary  string   `json:"stderrSummary"`
	OutputMarkdown string   `json:"outputMarkdown"`
	ArtifactPaths  []string `json:"artifactPaths"`
	StartedAt      string   `json:"startedAt"`
	CompletedAt    string   `json:"completedAt"`
	ExitCode       int      `json:"exitCode"`
	ErrorMessage   string   `json:"errorMessage"`
}
