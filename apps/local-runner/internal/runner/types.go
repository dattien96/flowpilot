package runner

type Health struct {
	Status        string `json:"status"`
	RunnerVersion string `json:"runnerVersion"`
	Cwd           string `json:"cwd"`
	Os            string `json:"os"`
	StartedAt     string `json:"startedAt"`
}

type Provider struct {
	ID              string          `json:"id,omitempty"`
	Key             string          `json:"key"`
	Label           string          `json:"label"`
	Supported       bool            `json:"supported"`
	Installed       bool            `json:"installed"`
	InstallStatus   string          `json:"install_status,omitempty"`
	AuthStatus      string          `json:"auth_status"`
	DetectedBinary  string          `json:"detected_binary,omitempty"`
	DetectedVersion string          `json:"detected_version,omitempty"`
	Models          []ProviderModel `json:"models,omitempty"`
	LastError       *string         `json:"last_error,omitempty"`
	Version         string          `json:"version"`
	BinaryPath      string          `json:"binaryPath"`
	InstallHint     string          `json:"installHint"`
}

type ProviderModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Available   bool   `json:"available"`
	Source      string `json:"source"`
}

type ProviderInventory struct {
	Providers []Provider `json:"providers"`
}

type McpBackend struct {
	Key           string `json:"key"`
	ProviderType  string `json:"providerType"`
	Label         string `json:"label"`
	Transport     string `json:"transport"`
	Installed     bool   `json:"installed"`
	State         string `json:"state"`
	Launcher      string `json:"launcher"`
	BinaryPath    string `json:"binaryPath"`
	Command       string `json:"command"`
	InstallHint   string `json:"installHint"`
	Action        string `json:"action"`
	ActionLabel   string `json:"actionLabel"`
	LastCheckedAt string `json:"lastCheckedAt"`
	LastError     string `json:"lastError"`
	SecretKey     string `json:"secretKey,omitempty"`
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
	ManifestPath     string `json:"manifestPath"`
	PromptPath       string `json:"promptPath"`
	ActualPromptPath string `json:"actualPromptPath,omitempty"`
	StdoutPath       string `json:"stdoutPath"`
	StderrPath       string `json:"stderrPath"`
	CommandPath      string `json:"commandPath"`
	ContentPath      string `json:"contentPath"`
	Checksum         string `json:"checksum"`
	PromptText       string `json:"promptText,omitempty"`
	ActualPromptText string `json:"actualPromptText,omitempty"`
	StdoutText       string `json:"stdoutText,omitempty"`
	StderrText       string `json:"stderrText,omitempty"`
	CommandText      string `json:"commandText,omitempty"`
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

type DirectorySelection struct {
	Path string `json:"path"`
}

type DirectoryValidationRequest struct {
	Path string `json:"path"`
}

type DirectoryValidationResult struct {
	Path   string `json:"path"`
	Usable bool   `json:"usable"`
	Reason string `json:"reason"`
}

type BackupRequest struct {
	Scope string `json:"scope"`
	RunID string `json:"runId"`
}

type ArtifactDeletionRequest struct {
	WorkflowRunIDs []string `json:"workflowRunIds"`
}

type BackupResult struct {
	BackupPath  string `json:"backupPath"`
	ArchiveName string `json:"archiveName"`
	CreatedAt   string `json:"createdAt"`
}

type IntegrationConnectionRequest struct {
	ProjectID    string `json:"projectId"`
	ProviderType string `json:"providerType"`
	Action       string `json:"action"`
	WorkspaceURL string `json:"workspaceUrl,omitempty"`
	ProjectKey   string `json:"projectKey,omitempty"`
	BoardID      string `json:"boardId,omitempty"`
	Email        string `json:"email,omitempty"`
	ApiToken     string `json:"apiToken,omitempty"`
}

type McpBackendActionRequest struct {
	ProjectID     string `json:"projectId"`
	IntegrationID string `json:"integrationId"`
	Action        string `json:"action"`
}

type McpTestRequest struct {
	BackendKey    string `json:"backendKey"`
	ProviderType  string `json:"providerType"`
	ProjectID     string `json:"projectId"`
	IntegrationID string `json:"integrationId"`
	TemplateKey   string `json:"templateKey"`
	AllowWrite    bool   `json:"allowWrite"`
	Prompt        string `json:"prompt"`
	TimeoutMs     int    `json:"timeoutMs"`
}

type McpTestResult struct {
	Status         string   `json:"status"`
	RunID          string   `json:"runId"`
	BackendKey     string   `json:"backendKey"`
	ProviderType   string   `json:"providerType"`
	ProjectID      string   `json:"projectId"`
	IntegrationID  string   `json:"integrationId"`
	Command        string   `json:"command"`
	StdoutSummary  string   `json:"stdoutSummary"`
	StderrSummary  string   `json:"stderrSummary"`
	OutputMarkdown string   `json:"outputMarkdown"`
	ArtifactPaths  []string `json:"artifactPaths"`
	StartedAt      string   `json:"startedAt"`
	CompletedAt    string   `json:"completedAt"`
	ErrorMessage   string   `json:"errorMessage"`
}

type McpTestRunSummary struct {
	RunID         string `json:"runId"`
	BackendKey    string `json:"backendKey"`
	ProviderType  string `json:"providerType"`
	ProjectID     string `json:"projectId"`
	IntegrationID string `json:"integrationId"`
	Status        string `json:"status"`
	StartedAt     string `json:"startedAt"`
	CompletedAt   string `json:"completedAt"`
	ArtifactDir   string `json:"artifactDir"`
}

type IntegrationConnectionResult struct {
	RequestStatus     string  `json:"requestStatus"`
	IntegrationID     string  `json:"integrationId"`
	IntegrationStatus string  `json:"integrationStatus"`
	RunID             *string `json:"runId"`
	Message           *string `json:"message"`
}

type PromptExecutionRequest struct {
	ProviderKey       string            `json:"providerKey"`
	ModelName         string            `json:"modelName,omitempty"`
	ReasoningEffort   string            `json:"reasoningEffort,omitempty"`
	Prompt            string            `json:"prompt"`
	SkillIds          []string          `json:"skillIds"`
	FlowId            string            `json:"flowId"`
	ContextSourceIds  []string          `json:"contextSourceIds"`
	TimeoutMs         int               `json:"timeoutMs"`
	WorkingDirectory  string            `json:"workingDirectory"`
	AllowWrite        bool              `json:"allowWrite,omitempty"`
	ProviderAccountID string            `json:"providerAccountId,omitempty"`
	AccountHomePath   string            `json:"accountHomePath,omitempty"`
	ProxyURL          string            `json:"proxyUrl,omitempty"`
	CustomEnv         map[string]string `json:"customEnv,omitempty"`
}

type PromptExecutionResult struct {
	Status            string   `json:"status"`
	RunID             string   `json:"runId"`
	ProviderKey       string   `json:"providerKey"`
	ModelName         *string  `json:"modelName"`
	ProviderSessionID string   `json:"providerSessionId,omitempty"`
	Command           string   `json:"command"`
	StdoutSummary     string   `json:"stdoutSummary"`
	StderrSummary     string   `json:"stderrSummary"`
	OutputMarkdown    string   `json:"outputMarkdown"`
	ArtifactPaths     []string `json:"artifactPaths"`
	StartedAt         string   `json:"startedAt"`
	CompletedAt       string   `json:"completedAt"`
	ExitCode          int      `json:"exitCode"`
	ErrorMessage      string   `json:"errorMessage"`
}

type SessionStreamEvent struct {
	Type    string                 `json:"type"`
	Stream  string                 `json:"stream,omitempty"`
	Message string                 `json:"message,omitempty"`
	Result  *PromptExecutionResult `json:"result,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Code    string                 `json:"code,omitempty"`
	Details string                 `json:"details,omitempty"`
}

type SessionStreamCallback func(SessionStreamEvent)

type AiSessionStartRequest struct {
	ProviderKey             string            `json:"providerKey"`
	ModelName               string            `json:"modelName"`
	ReasoningEffort         *string           `json:"reasoningEffort"`
	WorkingDirectory        string            `json:"workingDirectory"`
	ApprovalMode            *string           `json:"approvalMode"`
	AllowWrite              bool              `json:"allowWrite"`
	IdleTTLSeconds          *int              `json:"idleTTLSeconds,omitempty"`
	ResumeProviderSessionID *string           `json:"resumeProviderSessionId,omitempty"`
	ProviderAccountID       string            `json:"providerAccountId,omitempty"`
	AccountHomePath         string            `json:"accountHomePath,omitempty"`
	ProxyURL                string            `json:"proxyUrl,omitempty"`
	CustomEnv               map[string]string `json:"customEnv,omitempty"`
}

type AiSessionHandle struct {
	TransportType     string  `json:"transportType"`
	ProviderSessionID string  `json:"providerSessionId"`
	ProcessKey        *string `json:"processKey"`
	ProcessPid        *int    `json:"processPid,omitempty"`
}

type AiSessionMessageRequest struct {
	Session          AiSessionHandle       `json:"session"`
	Prompt           string                `json:"prompt"`
	SkillIds         []string              `json:"skillIds"`
	ContextSourceIds []string              `json:"contextSourceIds"`
	IdleTTLSeconds   *int                  `json:"idleTTLSeconds,omitempty"`
	StreamCallback   SessionStreamCallback `json:"-"`
}
