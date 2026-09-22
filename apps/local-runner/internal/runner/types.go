package runner

type Health struct {
	Status        string `json:"status"`
	RunnerVersion string `json:"runnerVersion"`
	Cwd           string `json:"cwd"`
	Os            string `json:"os"`
	StartedAt     string `json:"startedAt"`
	// CP-81 additive identity (SD-28 §6.2): present only when a lifecycle
	// manager is attached (runner serve); zero-valued/omitted otherwise so
	// legacy consumers see the unchanged shape.
	RunnerInstanceID string `json:"runnerInstanceId,omitempty"`
	Generation       int    `json:"generation,omitempty"`
	ProtocolVersion  int    `json:"protocolVersion,omitempty"`
	BuildID          string `json:"buildId,omitempty"`
	LifecycleMode    string `json:"lifecycleMode,omitempty"`
	Phase            string `json:"phase,omitempty"`
}

type Provider struct {
	ID              string                      `json:"id,omitempty"`
	Key             string                      `json:"key"`
	Label           string                      `json:"label"`
	Supported       bool                        `json:"supported"`
	Installed       bool                        `json:"installed"`
	InstallStatus   string                      `json:"install_status,omitempty"`
	AuthStatus      string                      `json:"auth_status"`
	DetectedBinary  string                      `json:"detected_binary,omitempty"`
	DetectedVersion string                      `json:"detected_version,omitempty"`
	Models          []ProviderModel             `json:"models,omitempty"`
	LastError       *string                     `json:"last_error,omitempty"`
	Version         string                      `json:"version"`
	BinaryPath      string                      `json:"binaryPath"`
	InstallHint     string                      `json:"installHint"`
	Accounts        []ProviderDiscoveredAccount `json:"accounts,omitempty"`
}

type ProviderModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Available   bool   `json:"available"`
	Source      string `json:"source"`
	// SupportedReasoningEfforts/DefaultReasoningEffort/ContextWindowTokens/
	// MaxContextWindowTokens (Task-215) carry per-model capability data the
	// provider's own CLI already reports (codex debug models'
	// supported_reasoning_levels/default_reasoning_level/context_window/
	// max_context_window; Grok's models_cache.json reasoning_efforts/
	// reasoning_effort/context_window) so the desktop Reasoning control and
	// context-usage display can be model-aware instead of one static list/
	// value for every model of a provider. Empty/zero for providers or
	// models with no such data (e.g. Claude, or the static-fallback path).
	SupportedReasoningEfforts []string `json:"supported_reasoning_efforts,omitempty"`
	DefaultReasoningEffort    string   `json:"default_reasoning_effort,omitempty"`
	ContextWindowTokens       int64    `json:"context_window_tokens,omitempty"`
	MaxContextWindowTokens    int64    `json:"max_context_window_tokens,omitempty"`
	// InputImage (Task-319) mirrors the model's own `capabilities.input.image`
	// from `opencode models --verbose` (models.dev capability). Opencode image
	// support is per-MODEL, not per-provider: 16/31 opencode*/* models accept
	// image input (live-verified 2026-08-30, v1.18.25). This is the Task-318
	// unlock data — the UI/adapter gates use it instead of a blanket
	// provider-level Vision=false.
	InputImage bool `json:"input_image,omitempty"`
}

type ProviderDiscoveredAccount struct {
	ID       string `json:"id"`
	HomePath string `json:"homePath"`
	Label    string `json:"label"`
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
	StorageProvider string `json:"storageProvider,omitempty"`
	RemoteObjectID  string `json:"remoteObjectId,omitempty"`
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

type ArtifactCloudSyncResult struct {
	StorageProvider string `json:"storageProvider"`
	RemotePath      string `json:"remotePath"`
	RemoteObjectID  string `json:"remoteObjectId,omitempty"`
	SyncStatus      string `json:"syncStatus"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
}

type ArtifactSyncRequest struct {
	StorageProvider          string `json:"storageProvider,omitempty"`
	GoogleDriveIntegrationID string `json:"googleDriveIntegrationId,omitempty"`
	GoogleDriveProjectID     string `json:"googleDriveProjectId,omitempty"`
	GoogleDriveFolderID      string `json:"googleDriveFolderId,omitempty"`
	GoogleDriveDriveID       string `json:"googleDriveDriveId,omitempty"`
}

type ArtifactHydrationRequest struct {
	ArtifactID            string `json:"artifactId"`
	Title                 string `json:"title"`
	SourceKind            string `json:"sourceKind,omitempty"`
	ProjectID             string `json:"projectId"`
	FeatureID             string `json:"featureId,omitempty"`
	WorkflowRunID         string `json:"workflowRunId"`
	WorkflowStepKey       string `json:"workflowStepKey"`
	ProviderKey           string `json:"providerKey,omitempty"`
	RemotePath            string `json:"remotePath"`
	RemoteObjectID        string `json:"remoteObjectId,omitempty"`
	SourceStorageProvider string `json:"sourceStorageProvider"`
	CreatedAt             string `json:"createdAt,omitempty"`
	UpdatedAt             string `json:"updatedAt,omitempty"`
}

type ArtifactStorageGoogleDriveConnectRequest struct {
	ProjectID string `json:"projectId"`
	BaseURL   string `json:"baseUrl,omitempty"`
	AccountID string `json:"accountId,omitempty"`
}

type GoogleDriveAccountConnectRequest struct {
	BaseURL   string `json:"baseUrl,omitempty"`
	AccountID string `json:"accountId,omitempty"`
}

type ArtifactStorageGoogleDriveSessionStatus string

const (
	ArtifactStorageGoogleDriveSessionPending              ArtifactStorageGoogleDriveSessionStatus = "pending"
	ArtifactStorageGoogleDriveSessionAwaitingOAuth        ArtifactStorageGoogleDriveSessionStatus = "awaiting_oauth"
	ArtifactStorageGoogleDriveSessionAwaitingFolderPicker ArtifactStorageGoogleDriveSessionStatus = "awaiting_folder_selection"
	ArtifactStorageGoogleDriveSessionConnected            ArtifactStorageGoogleDriveSessionStatus = "connected"
	ArtifactStorageGoogleDriveSessionFailed               ArtifactStorageGoogleDriveSessionStatus = "failed"
	ArtifactStorageGoogleDriveSessionExpired              ArtifactStorageGoogleDriveSessionStatus = "expired"
	artifactStorageGoogleDriveDefaultSessionTTL                                                   = 10 * 60
)

type ArtifactStorageGoogleDriveSession struct {
	SessionID    string                                  `json:"sessionId"`
	ProjectID    string                                  `json:"projectId"`
	Status       ArtifactStorageGoogleDriveSessionStatus `json:"status"`
	ConnectURL   string                                  `json:"connectUrl,omitempty"`
	ExpiresAt    string                                  `json:"expiresAt"`
	ConnectedAt  string                                  `json:"connectedAt,omitempty"`
	AccountID    string                                  `json:"accountId,omitempty"`
	AccountEmail string                                  `json:"accountEmail,omitempty"`
	FolderID     string                                  `json:"folderId,omitempty"`
	FolderName   string                                  `json:"folderName,omitempty"`
	LastError    string                                  `json:"lastError,omitempty"`
}

type ArtifactStorageGoogleDriveConnection struct {
	ProjectID       string `json:"projectId"`
	Status          string `json:"status"`
	FolderID        string `json:"folderId,omitempty"`
	FolderName      string `json:"folderName,omitempty"`
	AccountID       string `json:"accountId,omitempty"`
	AccountEmail    string `json:"accountEmail,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	LastValidatedAt string `json:"lastValidatedAt,omitempty"`
	ConnectedAt     string `json:"connectedAt,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
}

type ArtifactStorageGoogleDriveConnectionStatus struct {
	Connection ArtifactStorageGoogleDriveConnection `json:"connection"`
	Session    *ArtifactStorageGoogleDriveSession   `json:"session,omitempty"`
}

type ChatSyncGoogleDriveConnectionStatus struct {
	Connection        ArtifactStorageGoogleDriveConnection `json:"connection"`
	Session           *ArtifactStorageGoogleDriveSession   `json:"session,omitempty"`
	EffectiveSource   string                               `json:"effectiveSource"`
	Ready             bool                                 `json:"ready"`
	AvailableAccounts []GoogleDriveAccountStatus           `json:"availableAccounts,omitempty"`
}

type ArtifactStorageGoogleDrivePickerToken struct {
	AccessToken string `json:"accessToken"`
	ApiKey      string `json:"apiKey"`
}

type ArtifactStorageGoogleDriveFolderSelectionRequest struct {
	SessionID    string `json:"sessionId"`
	FolderID     string `json:"folderId"`
	FolderName   string `json:"folderName"`
	AccountEmail string `json:"accountEmail,omitempty"`
}

type ChatSyncGoogleDriveConnectRequest struct {
	BaseURL   string `json:"baseUrl,omitempty"`
	AccountID string `json:"accountId,omitempty"`
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

type SupabaseWorkspaceConfig struct {
	Version         int    `json:"version"`
	APIURL          string `json:"apiUrl"`
	AnonKey         string `json:"anonKey"`
	EdgeFunctionURL string `json:"edgeFunctionUrl"`
	ProjectRef      string `json:"projectRef,omitempty"`
	Status          string `json:"status,omitempty"`
	UpdatedAt       string `json:"updatedAt"`
}

type SupabaseWorkspaceConfigResponse struct {
	SupabaseWorkspaceConfig
	HasServiceRoleKey bool   `json:"hasServiceRoleKey"`
	ServiceRoleKey    string `json:"serviceRoleKey,omitempty"`
}

type SupabaseWorkspaceConfigRequest struct {
	SupabaseWorkspaceConfig
	ServiceRoleKey string `json:"serviceRoleKey"`
}

type SupabaseSchemaApplyRequest struct {
	APIURL      string `json:"apiUrl"`
	ProjectRef  string `json:"projectRef,omitempty"`
	AccessToken string `json:"accessToken"`
}

type SupabaseSchemaMigrationResult struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type SupabaseSchemaApplyResponse struct {
	ProjectRef   string                          `json:"projectRef"`
	AppliedCount int                             `json:"appliedCount"`
	SkippedCount int                             `json:"skippedCount"`
	Migrations   []SupabaseSchemaMigrationResult `json:"migrations"`
}

type SupabasePasswordLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SupabasePasswordLoginResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	UserID       string `json:"userId"`
	Email        string `json:"email,omitempty"`
}

type GoogleDriveWorkspaceConfigRequest struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	RedirectURI  string `json:"redirectUri"`
	PickerAPIKey string `json:"pickerApiKey"`
	MCPAccountID string `json:"mcpAccountId,omitempty"`
}

type GoogleDriveMcpOAuthUploadRequest struct {
	FileName string `json:"fileName"`
	Content  string `json:"content"`
}

type GoogleDriveArtifactSyncStatus struct {
	Status          string   `json:"status"`
	Source          string   `json:"source"`
	Configured      bool     `json:"configured"`
	ClientID        string   `json:"clientId,omitempty"`
	RedirectURI     string   `json:"redirectUri,omitempty"`
	HasClientSecret bool     `json:"hasClientSecret"`
	HasPickerAPIKey bool     `json:"hasPickerApiKey"`
	MissingFields   []string `json:"missingFields,omitempty"`
}

type GoogleDriveAccountStatus struct {
	AccountID         string   `json:"accountId"`
	AccountEmail      string   `json:"accountEmail,omitempty"`
	AccountSubject    string   `json:"accountSubject,omitempty"`
	OAuthClientID     string   `json:"oauthClientId,omitempty"`
	GrantedScopes     []string `json:"grantedScopes,omitempty"`
	MissingScopes     []string `json:"missingScopes,omitempty"`
	Status            string   `json:"status"`
	ProjectCount      int      `json:"projectCount"`
	AccountReady      bool     `json:"accountReady"`
	McpReadReady      bool     `json:"mcpReadReady"`
	McpWriteReady     bool     `json:"mcpWriteReady"`
	ReconnectRequired bool     `json:"reconnectRequired,omitempty"`
	ConnectedAt       string   `json:"connectedAt,omitempty"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
	LastError         string   `json:"lastError,omitempty"`
}

type GoogleDriveMcpStatus struct {
	Status                   string   `json:"status"`
	Configured               bool     `json:"configured"`
	ProxyMcpEnabled          bool     `json:"proxyMcpEnabled"`
	CredentialPath           string   `json:"credentialPath,omitempty"`
	TokenPath                string   `json:"tokenPath,omitempty"`
	CredentialFileExists     bool     `json:"credentialFileExists"`
	CredentialFileValid      bool     `json:"credentialFileValid"`
	TokenFileExists          bool     `json:"tokenFileExists"`
	TokenRefreshValid        bool     `json:"tokenRefreshValid"`
	NeedsAuth                bool     `json:"needsAuth"`
	BackendPackageAvailable  bool     `json:"backendPackageAvailable"`
	AccountID                string   `json:"accountId,omitempty"`
	AccountEmail             string   `json:"accountEmail,omitempty"`
	AccountSelectionRequired bool     `json:"accountSelectionRequired,omitempty"`
	GrantedScopes            []string `json:"grantedScopes,omitempty"`
	MissingScopes            []string `json:"missingScopes,omitempty"`
	AccountReady             bool     `json:"accountReady"`
	ArtifactBindingPresent   bool     `json:"artifactBindingPresent,omitempty"`
	ArtifactReady            bool     `json:"artifactReady"`
	McpReadReady             bool     `json:"mcpReadReady"`
	McpWriteReady            bool     `json:"mcpWriteReady"`
	ReconnectRequired        bool     `json:"reconnectRequired,omitempty"`
	MissingFields            []string `json:"missingFields,omitempty"`
}

type GoogleDriveWorkspaceConfigResponse struct {
	ArtifactSync    GoogleDriveArtifactSyncStatus        `json:"artifactSync"`
	MCP             GoogleDriveMcpStatus                 `json:"mcp"`
	Accounts        []GoogleDriveAccountStatus           `json:"accounts,omitempty"`
	ProviderConfigs []GoogleDriveMcpProviderConfigStatus `json:"providerConfigs,omitempty"`
	RunnerReachable bool                                 `json:"runnerReachable"`
	LastError       string                               `json:"lastError,omitempty"`
	UpdatedAt       string                               `json:"updatedAt,omitempty"`
}

type GoogleDriveValidationCheck struct {
	Key     string `json:"key"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type GoogleDriveValidationResult struct {
	Valid  bool                               `json:"valid"`
	Checks []GoogleDriveValidationCheck       `json:"checks"`
	Status GoogleDriveWorkspaceConfigResponse `json:"status"`
}

type SupabaseValidationResult struct {
	Valid             bool                      `json:"valid"`
	ProjectRef        string                    `json:"projectRef,omitempty"`
	Checks            []SupabaseValidationCheck `json:"checks"`
	BrowserSafeConfig SupabaseWorkspaceConfig   `json:"browserSafeConfig,omitempty"`
	HasServiceRoleKey bool                      `json:"hasServiceRoleKey"`
}

type SupabaseValidationCheck struct {
	Key     string `json:"key"`
	Status  string `json:"status"`
	Message string `json:"message"`
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
	ProjectID    string `json:"projectId,omitempty"`
	ProviderType string `json:"providerType"`
	Action       string `json:"action"`
	WorkspaceURL string `json:"workspaceUrl,omitempty"`
	ProjectKey   string `json:"projectKey,omitempty"`
	BoardID      string `json:"boardId,omitempty"`
	Email        string `json:"email,omitempty"`
	ApiToken     string `json:"apiToken,omitempty"`
	// FirebaseProjectID/FirebaseEnvironment/ServiceAccountJSON back the
	// Firebase connection flow (Task-230, CP-05-04). ServiceAccountJSON is
	// the raw contents of the uploaded GCP service-account key file — it is
	// sent once to the runner and stored only in the runner keyring
	// (firebaseCredential), never persisted in Supabase config_encrypted.
	FirebaseProjectID   string `json:"firebaseProjectId,omitempty"`
	FirebaseEnvironment string `json:"firebaseEnvironment,omitempty"`
	ServiceAccountJSON  string `json:"serviceAccountJson,omitempty"`
	// BotToken/ChannelID back the Telegram connection flow (Task-232,
	// CP-05-05). BotToken is stored only in the runner keyring
	// (telegramCredential), never in Supabase config_encrypted.
	BotToken  string `json:"botToken,omitempty"`
	ChannelID string `json:"channelId,omitempty"`
	// TelegramAutoApprove is a pointer so a re-Test of an already-connected
	// integration (desktop sends stripped config without this field) does not
	// wipe a previously-enabled auto-approve flag via JSON's false zero-value.
	// nil = leave existing keyring AutoApprove unchanged; non-nil = set it.
	TelegramAutoApprove *bool `json:"telegramAutoApprove,omitempty"`
}

type McpBackendActionRequest struct {
	ProjectID     string `json:"projectId,omitempty"`
	IntegrationID string `json:"integrationId"`
	Action        string `json:"action"`
}

type McpTestRequest struct {
	BackendKey       string `json:"backendKey"`
	ProviderType     string `json:"providerType"`
	ProjectID        string `json:"projectId"`
	IntegrationID    string `json:"integrationId"`
	TemplateKey      string `json:"templateKey"`
	AllowWrite       bool   `json:"allowWrite"`
	Prompt           string `json:"prompt"`
	TimeoutMs        int    `json:"timeoutMs"`
	UseProviderCLI   bool   `json:"useProviderCli"`
	AIProviderKey    string `json:"aiProviderKey,omitempty"`
	AIModelName      string `json:"aiModelName,omitempty"`
	AccountHomePath  string `json:"accountHomePath,omitempty"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`
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
	AIProviderKey  string   `json:"aiProviderKey,omitempty"`
	AIModelName    string   `json:"aiModelName,omitempty"`
	McpServerName  string   `json:"mcpServerName,omitempty"`
	McpToolUsed    string   `json:"mcpToolUsed,omitempty"`
	McpFailureCode string   `json:"mcpFailureCode,omitempty"`
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
	RequiredMcps      []string          `json:"requiredMcps,omitempty"`
	SkillIds          []string          `json:"skillIds"`
	FlowId            string            `json:"flowId"`
	ContextSourceIds  []string          `json:"contextSourceIds"`
	TimeoutMs         int               `json:"timeoutMs"`
	WorkingDirectory  string            `json:"workingDirectory"`
	AllowWrite        bool              `json:"allowWrite,omitempty"`
	YoloMode          bool              `json:"yoloMode,omitempty"`
	ProviderAccountID string            `json:"providerAccountId,omitempty"`
	AccountHomePath   string            `json:"accountHomePath,omitempty"`
	ProxyURL          string            `json:"proxyUrl,omitempty"`
	CustomEnv         map[string]string `json:"customEnv,omitempty"`
	// OnStdoutDelta streams provider stdout as it lands in the artifact file
	// (CA-916 scaffold observability). In-process only — never serialized.
	// Providers with buffered capture (Gemini Agy) deliver one delta at the end.
	OnStdoutDelta func(chunk string) `json:"-"`
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
	ActualPromptText  string   `json:"actualPromptText,omitempty"`
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
	Command           string  `json:"command,omitempty"`
}

type AiSessionMessageRequest struct {
	Session          AiSessionHandle       `json:"session"`
	Prompt           string                `json:"prompt"`
	RequiredMcps     []string              `json:"requiredMcps,omitempty"`
	SkillIds         []string              `json:"skillIds"`
	ContextSourceIds []string              `json:"contextSourceIds"`
	AllowWrite       bool                  `json:"allowWrite,omitempty"`
	YoloMode         bool                  `json:"yoloMode,omitempty"`
	AccountHomePath  string                `json:"accountHomePath,omitempty"`
	IdleTTLSeconds   *int                  `json:"idleTTLSeconds,omitempty"`
	StreamCallback   SessionStreamCallback `json:"-"`
}
