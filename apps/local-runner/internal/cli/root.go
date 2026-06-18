package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"flowpilot-runner/internal/runner"

	"github.com/spf13/cobra"
)

type config struct {
	workspace string
	host      string
	port      int
}

func NewRootCommand() *cobra.Command {
	cfg := &config{host: "127.0.0.1", port: 4317}

	rootCmd := &cobra.Command{
		Use:           "flowpilot",
		Short:         "FlowPilot local runner",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&cfg.workspace, "workspace", "", "Workspace root used to discover skills and flows")
	rootCmd.PersistentFlags().StringVar(&cfg.host, "host", cfg.host, "Host used by the local HTTP server")
	rootCmd.PersistentFlags().IntVar(&cfg.port, "port", cfg.port, "Port used by the local HTTP server")

	rootCmd.AddCommand(newRunnerCommand(cfg))
	rootCmd.AddCommand(newProvidersCommand(cfg))
	rootCmd.AddCommand(newInstallProviderCommand(cfg))
	rootCmd.AddCommand(newBackendsCommand(cfg))
	rootCmd.AddCommand(newGoogleDriveMcpCommand(cfg))
	rootCmd.AddCommand(newSkillsCommand(cfg))
	rootCmd.AddCommand(newFlowsCommand(cfg))

	return rootCmd
}

func newRunnerCommand(cfg *config) *cobra.Command {
	runnerCmd := &cobra.Command{
		Use:   "runner",
		Short: "Inspect and serve the local runner",
	}

	runnerCmd.AddCommand(&cobra.Command{
		Use:   "health",
		Short: "Print the runner health payload as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}
			return writeJSON(os.Stdout, instance.Health())
		},
	})

	runnerCmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Start the local HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}

			// Start the idle sweeper
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			instance.StartIdleSweeper(ctx)
			instance.StartOrphanedWorkflowArtifactCleanup(ctx)

			mux := http.NewServeMux()

			// Interactive + admin APIs (04-02). The provider registry is built for
			// this runner: when FLOWPILOT_CODEX_APPSERVER is set it backs Codex with
			// the live shared app-server adapter (04-03 registry swap); otherwise the
			// fake adapter keeps the demo/tests green without a codex binary.
			// The catalog (projects/workflows/steps) reads from Supabase when the
			// runner has a Supabase config, else serves the offline fake catalog
			// (04-08 A1).
			// Build a local file session store so run history survives app
			// restarts when Supabase is not configured (BUG-080). Fall back
			// to the default in-memory store on any filesystem error.
			var sessionStore runner.WorkflowStore
			storeDir := filepath.Join(instance.Health().Cwd, ".flowpilot", "chats")
			if fs, err := runner.NewLocalFileSessionStore(storeDir); err == nil {
				sessionStore = fs
			}
			interactive := runner.NewInteractiveServiceWithStore(
				runner.ProviderRegistryFor(instance),
				runner.CatalogStoreFor(instance),
				sessionStore,
			)
			interactive.AttachRunner(instance)
			interactive.RegisterInteractiveRoutes(mux)
			// Runner-hosted MCP server for the Claude permission/ask_user tools (07):
			// the per-turn --mcp-config URL points claude back at this route.
			mux.Handle(runner.ClaudeMCPPath, instance.ClaudeMCPHandler())

			mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				writeHTTPJSON(w, instance.Health())
			})
			mux.HandleFunc("/providers", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				providers, err := instance.DetectProviders(context.Background())
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}
				writeHTTPJSON(w, providers)
			})
			mux.HandleFunc("/providers/install", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload struct {
					ProviderName string `json:"providerName"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				inventory, err := instance.InstallProvider(r.Context(), payload.ProviderName)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				writeHTTPJSON(w, inventory)
			})
			mux.HandleFunc("/providers/auth", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload struct {
					ProviderName string `json:"providerName"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				err := instance.AuthenticateProvider(r.Context(), payload.ProviderName)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"success"}`))
			})

			mux.HandleFunc("/provider-accounts", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				accounts, err := instance.ListProviderAccounts()
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}

				writeHTTPJSON(w, map[string]any{"accounts": accounts})
			})
			mux.HandleFunc("/client/provider-accounts", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				accounts, err := instance.ListProviderAccounts()
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}

				summaries, err := buildProviderAccountSummaryResponses(accounts)
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}

				writeHTTPJSON(w, summaries)
			})
			mux.HandleFunc("/provider-accounts/allocate-slot", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload struct {
					ProviderKey   string   `json:"providerKey"`
					ExistingPaths []string `json:"existingPaths"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				homePath, slotIndex, err := runner.NextAccountHomePath(payload.ProviderKey, payload.ExistingPaths)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"homePath":  homePath,
					"slotIndex": slotIndex,
				})
			})
			mux.HandleFunc("/provider-accounts/connect", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload struct {
					ProviderKey string `json:"providerKey"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				account, err := instance.ConnectProviderAccount(payload.ProviderKey)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, account)
			})
			mux.HandleFunc("/provider-accounts/defaults", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				defaults := make([]map[string]string, 0, 3)
				for _, providerKey := range []string{"codex", "claude", "gemini"} {
					homePath, ok := runner.DetectDefaultAccountHomePath(providerKey)
					if !ok {
						continue
					}

					defaults = append(defaults, map[string]string{
						"providerKey": providerKey,
						"homePath":    homePath,
					})
				}

				writeHTTPJSON(w, map[string]any{"defaults": defaults})
			})
			mux.HandleFunc("/provider-accounts/context", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				defaults := make([]map[string]string, 0, 3)
				for _, providerKey := range []string{"codex", "claude", "gemini"} {
					homePath, ok := runner.DetectDefaultAccountHomePath(providerKey)
					if !ok {
						continue
					}

					defaults = append(defaults, map[string]string{
						"providerKey": providerKey,
						"homePath":    homePath,
					})
				}

				writeHTTPJSON(w, map[string]any{
					"defaults":       defaults,
					"runnerInstance": runner.CurrentRunnerInstanceContext(),
				})
			})
			mux.HandleFunc("/provider-accounts/verify", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload struct {
					AccountID       string `json:"accountId"`
					ProviderKey     string `json:"providerKey"`
					AccountHomePath string `json:"accountHomePath"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				if strings.TrimSpace(payload.AccountID) != "" {
					account, verified, err := instance.VerifyProviderAccount(payload.AccountID)
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}
					writeHTTPJSON(w, map[string]any{
						"verified": verified,
						"account":  account,
					})
					return
				}

				hasAuth := runner.HasLocalAuthAtPath(payload.ProviderKey, payload.AccountHomePath)
				writeHTTPJSON(w, map[string]bool{"verified": hasAuth})
			})
			mux.HandleFunc("/provider-accounts/activate", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload struct {
					AccountID string `json:"accountId"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				account, err := instance.ActivateProviderAccount(payload.AccountID)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				writeHTTPJSON(w, map[string]any{"account": account})
			})
			mux.HandleFunc("/provider-accounts/test", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload struct {
					AccountID string `json:"accountId"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				account, err := instance.TestProviderAccount(payload.AccountID)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				writeHTTPJSON(w, map[string]any{"account": account})
			})
			mux.HandleFunc("/provider-accounts/resolve", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload struct {
					ProviderKey string `json:"providerKey"`
					AccountID   string `json:"accountId"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				account, err := instance.ResolveProviderAccount(payload.ProviderKey, payload.AccountID)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				writeHTTPJSON(w, map[string]any{"account": account})
			})
			mux.HandleFunc("/provider-accounts/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				accountID := strings.TrimPrefix(r.URL.Path, "/provider-accounts/")
				if strings.TrimSpace(accountID) == "" {
					http.Error(w, "account id is required", http.StatusBadRequest)
					return
				}

				if err := instance.DeleteProviderAccount(accountID); err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				w.WriteHeader(http.StatusNoContent)
			})
			mux.HandleFunc("/directories/pick", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				selection, err := instance.PickDirectory(r.Context())
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				writeHTTPJSON(w, selection)
			})
			mux.HandleFunc("/directories/validate", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload runner.DirectoryValidationRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				writeHTTPJSON(w, instance.ValidateDirectory(payload.Path))
			})
			mux.HandleFunc("/mcp-backends", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				backends, err := instance.ListMcpBackends(r.Context())
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}

				writeHTTPJSON(w, backends)
			})
			mux.HandleFunc("/mcp-backends/", func(w http.ResponseWriter, r *http.Request) {
				trimmed := strings.TrimPrefix(r.URL.Path, "/mcp-backends/")
				parts := strings.Split(trimmed, "/")
				if len(parts) != 2 || parts[0] == "" {
					http.NotFound(w, r)
					return
				}
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				switch parts[1] {
				case "install", "action":
				default:
					http.NotFound(w, r)
					return
				}

				if parts[1] == "action" {
					var payload runner.McpBackendActionRequest
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
						return
					}
					switch payload.Action {
					case "install", "verify":
					default:
						writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("unsupported action %q", payload.Action))
						return
					}
					var backend runner.McpBackend
					var err error
					switch payload.Action {
					case "install":
						backend, err = instance.InstallMcpBackend(r.Context(), parts[0])
					case "verify":
						backend, err = instance.VerifyMcpBackend(
							r.Context(),
							parts[0],
							payload.ProjectID,
							payload.IntegrationID,
						)
					}
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}
					writeHTTPJSON(w, backend)
					return
				}

				backend, err := instance.InstallMcpBackend(r.Context(), parts[0])
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				writeHTTPJSON(w, backend)
			})
			mux.HandleFunc("/mcp-tests", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					limit := 10
					if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
						parsed, err := strconv.Atoi(rawLimit)
						if err != nil {
							writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid limit %q", rawLimit))
							return
						}
						limit = parsed
					}

					runs, err := instance.ListMcpTestRuns(
						r.Context(),
						r.URL.Query().Get("backendKey"),
						r.URL.Query().Get("projectId"),
						r.URL.Query().Get("integrationId"),
						limit,
					)
					if err != nil {
						writeHTTPError(w, http.StatusInternalServerError, err)
						return
					}
					writeHTTPJSON(w, runs)
				case http.MethodPost:
					var payload runner.McpTestRequest
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
						return
					}

					result, err := instance.RunMcpTest(r.Context(), payload)
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}
					writeHTTPJSON(w, result)
				default:
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
			})
			mux.HandleFunc("/skills", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				skills, err := instance.ListSkills()
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}
				writeHTTPJSON(w, skills)
			})
			mux.HandleFunc("/flows", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				flows, err := instance.ListFlows()
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}
				writeHTTPJSON(w, flows)
			})
			mux.HandleFunc("/artifacts", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				artifacts, err := instance.ListArtifacts()
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}

				writeHTTPJSON(w, artifacts)
			})
			mux.HandleFunc("/storage-driver", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					config, err := instance.GetStorageDriver()
					if err != nil {
						writeHTTPError(w, http.StatusInternalServerError, err)
						return
					}
					writeHTTPJSON(w, config)
				case http.MethodPut:
					var payload runner.StorageDriverConfig
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
						return
					}
					config, err := instance.SaveStorageDriver(payload)
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}
					writeHTTPJSON(w, config)
				case http.MethodPost:
					config, err := instance.ValidateStorageDriver()
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}
					writeHTTPJSON(w, config)
				default:
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
			})
			mux.HandleFunc("/supabase-config", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					var (
						config runner.SupabaseWorkspaceConfigResponse
						err    error
					)
					if r.URL.Query().Get("includeSecret") == "1" {
						config, err = instance.LoadSupabaseWorkspaceConfigWithSecret()
					} else {
						config, err = instance.LoadSupabaseWorkspaceConfig()
					}
					if err != nil {
						if errors.Is(err, os.ErrNotExist) {
							writeHTTPError(w, http.StatusNotFound, err)
							return
						}
						writeHTTPError(w, http.StatusInternalServerError, err)
						return
					}
					writeHTTPJSON(w, config)
				case http.MethodPut:
					var payload runner.SupabaseWorkspaceConfigRequest
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
						return
					}
					config, err := instance.SaveSupabaseWorkspaceConfig(payload)
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}
					writeHTTPJSON(w, config)
				case http.MethodDelete:
					if err := instance.ResetSupabaseWorkspaceConfig(); err != nil {
						writeHTTPError(w, http.StatusInternalServerError, err)
						return
					}
					writeHTTPJSON(w, map[string]string{"status": "reset"})
				default:
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
			})
			mux.HandleFunc("/supabase-config/validate", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload runner.SupabaseWorkspaceConfigRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				result, err := instance.ValidateSupabaseWorkspaceConfig(payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, result)
			})
			mux.HandleFunc("/supabase-auth/login", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload runner.SupabasePasswordLoginRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				result, err := instance.LoginSupabaseWithPassword(payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, result)
			})
			mux.HandleFunc("/google-drive-config", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					config, err := instance.LoadGoogleDriveWorkspaceConfig()
					if err != nil {
						writeHTTPError(w, http.StatusInternalServerError, err)
						return
					}
					writeHTTPJSON(w, config)
				case http.MethodPut:
					var payload runner.GoogleDriveWorkspaceConfigRequest
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
						return
					}
					config, err := instance.SaveGoogleDriveWorkspaceConfig(payload)
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}
					writeHTTPJSON(w, config)
				case http.MethodDelete:
					if err := instance.ResetGoogleDriveWorkspaceConfig(); err != nil {
						writeHTTPError(w, http.StatusInternalServerError, err)
						return
					}
					writeHTTPJSON(w, map[string]string{"status": "reset"})
				default:
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
			})
			mux.HandleFunc("/google-drive-config/validate", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload runner.GoogleDriveWorkspaceConfigRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				result, err := instance.ValidateGoogleDriveWorkspaceConfig(payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, result)
			})
			mux.HandleFunc("/google-drive-config/mcp-oauth-upload", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload runner.GoogleDriveMcpOAuthUploadRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				status, err := instance.UploadGoogleDriveMcpOAuthCredentials(payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, status)
			})
			mux.HandleFunc("/google-drive-config/mcp-provider-config/ensure", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload runner.GoogleDriveMcpProviderConfigRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				result, err := instance.EnsureGoogleDriveMcpProviderConfig(payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, result)
			})
			mux.HandleFunc("/google-drive-config/mcp-auth/start", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				if err := instance.StartGoogleDriveMcpAuth(); err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				status, err := instance.LoadGoogleDriveWorkspaceConfig()
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}
				message := "Google Drive MCP auth opened in a new terminal. Complete sign-in in the terminal/browser, then refresh MCP status."
				writeHTTPJSON(w, map[string]any{
					"status":  status.MCP.Status,
					"message": message,
					"config":  status,
				})
			})
			mux.HandleFunc("/google-drive-config/accounts", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				accounts, err := instance.ListGoogleDriveAccounts()
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}
				writeHTTPJSON(w, map[string]any{"accounts": accounts})
			})
			mux.HandleFunc("/google-drive-config/accounts/connect-sessions", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload runner.GoogleDriveAccountConnectRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				if strings.TrimSpace(payload.BaseURL) == "" {
					payload.BaseURL = requestBaseURL(r)
				}
				session, err := instance.CreateGoogleDriveAccountConnectSession(payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, session)
			})
			mux.HandleFunc("/google-drive-config/accounts/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				accountID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/google-drive-config/accounts/"))
				if accountID == "" {
					writeHTTPError(w, http.StatusBadRequest, errors.New("accountId is required"))
					return
				}
				if err := instance.DisconnectGoogleDriveAccount(accountID); err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})
			mux.HandleFunc("/google-drive-proxy-approvals", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				records, err := instance.ListGoogleDriveProxyApprovals(
					r.URL.Query().Get("workflowRunId"),
					r.URL.Query().Get("workflowStepRunId"),
					r.URL.Query().Get("status"),
				)
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}
				writeHTTPJSON(w, records)
			})
			mux.HandleFunc("/google-drive-proxy-approvals/", func(w http.ResponseWriter, r *http.Request) {
				trimmed := strings.TrimPrefix(r.URL.Path, "/google-drive-proxy-approvals/")
				parts := strings.Split(trimmed, "/")
				if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || parts[1] != "decision" {
					http.NotFound(w, r)
					return
				}
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload runner.GoogleDriveProxyApprovalDecisionRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				record, err := instance.DecideGoogleDriveProxyApproval(parts[0], payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, record)
			})
			mux.HandleFunc("/backup", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload runner.BackupRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				result, err := instance.CreateBackup(payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				writeHTTPJSON(w, result)
			})
			mux.HandleFunc("/artifacts/", func(w http.ResponseWriter, r *http.Request) {
				trimmed := strings.TrimPrefix(r.URL.Path, "/artifacts/")
				parts := strings.Split(trimmed, "/")
				if len(parts) == 0 || parts[0] == "" {
					http.NotFound(w, r)
					return
				}

				artifactID := parts[0]
				if len(parts) == 1 {
					if r.Method != http.MethodGet {
						http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
						return
					}

					artifact, err := instance.GetArtifact(artifactID)
					if err != nil {
						writeHTTPError(w, http.StatusNotFound, err)
						return
					}

					writeHTTPJSON(w, artifact)
					return
				}

				if len(parts) == 2 && parts[1] == "sync" {
					if r.Method != http.MethodPost {
						http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
						return
					}

					var payload runner.ArtifactSyncRequest
					if r.Body != nil {
						decodeErr := json.NewDecoder(r.Body).Decode(&payload)
						if decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
							writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", decodeErr))
							return
						}
					}

					artifact, err := instance.SyncArtifactWithContext(r.Context(), artifactID, payload)
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}

					writeHTTPJSON(w, artifact)
					return
				}

				if len(parts) == 2 && parts[1] == "sync-bundle" {
					if r.Method != http.MethodGet {
						http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
						return
					}

					var buffer bytes.Buffer
					if err := instance.WriteArtifactSyncBundle(artifactID, &buffer); err != nil {
						status := http.StatusBadRequest
						if errors.Is(err, os.ErrNotExist) {
							status = http.StatusNotFound
						}
						writeHTTPError(w, status, err)
						return
					}

					w.Header().Set("Content-Type", "application/zip")
					w.Header().Set(
						"Content-Disposition",
						fmt.Sprintf(`attachment; filename="%s.zip"`, artifactID),
					)
					if _, err := w.Write(buffer.Bytes()); err != nil {
						writeHTTPError(w, http.StatusInternalServerError, err)
					}
					return
				}

				if len(parts) == 2 && parts[1] == "cloud-sync-result" {
					if r.Method != http.MethodPut {
						http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
						return
					}

					var payload runner.ArtifactCloudSyncResult
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
						return
					}

					artifact, err := instance.SaveArtifactCloudSyncResult(artifactID, payload)
					if err != nil {
						status := http.StatusBadRequest
						if errors.Is(err, os.ErrNotExist) {
							status = http.StatusNotFound
						}
						writeHTTPError(w, status, err)
						return
					}

					writeHTTPJSON(w, artifact)
					return
				}

				if len(parts) == 2 && parts[1] == "hydrate-remote" {
					if r.Method != http.MethodPost {
						http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
						return
					}

					var payload runner.ArtifactHydrationRequest
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
						return
					}
					if strings.TrimSpace(payload.ArtifactID) == "" {
						payload.ArtifactID = artifactID
					}

					artifact, err := instance.HydrateArtifactFromRemote(r.Context(), payload)
					if err != nil {
						status := http.StatusBadRequest
						if errors.Is(err, os.ErrNotExist) {
							status = http.StatusNotFound
						}
						writeHTTPError(w, status, err)
						return
					}

					writeHTTPJSON(w, artifact)
					return
				}

				if len(parts) == 2 && parts[1] == "open" {
					if r.Method != http.MethodGet {
						http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
						return
					}

					targetURL, err := instance.ResolveArtifactOpenURL(artifactID, r.URL.Query().Get("file"))
					if err != nil {
						status := http.StatusBadRequest
						if errors.Is(err, os.ErrNotExist) {
							status = http.StatusNotFound
						}
						writeHTTPError(w, status, err)
						return
					}

					http.Redirect(w, r, targetURL, http.StatusTemporaryRedirect)
					return
				}

				http.NotFound(w, r)
			})
			mux.HandleFunc("/artifacts/delete-by-run-ids", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload runner.ArtifactDeletionRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				if err := instance.DeleteArtifactsByWorkflowRunIDs(payload.WorkflowRunIDs); err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}

				w.WriteHeader(http.StatusNoContent)
			})
			mux.HandleFunc("/files/read", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				requestPath := r.URL.Query().Get("path")
				if requestPath == "" {
					http.Error(w, "path is required", http.StatusBadRequest)
					return
				}
				resolvedPath := requestPath
				if !filepath.IsAbs(resolvedPath) {
					resolvedPath = filepath.Join(instance.Health().Cwd, requestPath)
				}
				content, err := os.ReadFile(resolvedPath)
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}
				writeHTTPJSON(w, map[string]string{"content": string(content)})
			})
			mux.HandleFunc("/artifact-storage/google-drive/connect-sessions", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload runner.ArtifactStorageGoogleDriveConnectRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				if strings.TrimSpace(payload.BaseURL) == "" {
					payload.BaseURL = requestBaseURL(r)
				}

				session, err := instance.CreateGoogleDriveArtifactConnectSession(payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, session)
			})
			mux.HandleFunc("/artifact-storage/google-drive/connection", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				projectID := strings.TrimSpace(r.URL.Query().Get("projectId"))
				sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
				status, err := instance.GetGoogleDriveArtifactConnectionStatus(projectID, sessionID)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, status)
			})
			mux.HandleFunc("/artifact-storage/google-drive/connect", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				targetURL, err := instance.BuildGoogleDriveArtifactConnectRedirect(
					r.URL.Query().Get("sessionId"),
					r.URL.Query().Get("token"),
				)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				http.Redirect(w, r, targetURL, http.StatusTemporaryRedirect)
			})
			mux.HandleFunc("/artifact-storage/google-drive/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				session, err := instance.HandleGoogleDriveArtifactOAuthCallback(
					r.URL.Query().Get("state"),
					r.URL.Query().Get("code"),
					r.URL.Query().Get("error"),
				)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				if strings.TrimSpace(session.ProjectID) == "" {
					http.Redirect(w, r, "/artifact-storage/google-drive/account-complete", http.StatusTemporaryRedirect)
					return
				}
				http.Redirect(w, r, fmt.Sprintf("%s?sessionId=%s", "/artifact-storage/google-drive/picker", url.QueryEscape(session.SessionID)), http.StatusTemporaryRedirect)
			})
			mux.HandleFunc("/artifact-storage/google-drive/account-complete", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if _, err := w.Write([]byte(runner.RenderGoogleDriveAccountConnectedHTML())); err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
				}
			})
			mux.HandleFunc("/artifact-storage/google-drive/picker", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
				if sessionID == "" {
					writeHTTPError(w, http.StatusBadRequest, errors.New("sessionId is required"))
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if _, err := w.Write([]byte(runner.RenderGoogleDriveArtifactPickerHTML(sessionID))); err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
				}
			})
			mux.HandleFunc("/artifact-storage/google-drive/picker-relay", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if _, err := w.Write([]byte(runner.RenderGoogleDriveArtifactPickerRelayHTML())); err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
				}
			})
			mux.HandleFunc("/artifact-storage/google-drive/picker-token", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				token, err := instance.GetGoogleDriveArtifactPickerToken(r.URL.Query().Get("sessionId"))
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, token)
			})
			mux.HandleFunc("/artifact-storage/google-drive/folder-selection", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload runner.ArtifactStorageGoogleDriveFolderSelectionRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				status, err := instance.SaveGoogleDriveArtifactFolderSelection(payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, status)
			})
			mux.HandleFunc("/integrations/", func(w http.ResponseWriter, r *http.Request) {
				trimmed := strings.TrimPrefix(r.URL.Path, "/integrations/")
				parts := strings.Split(trimmed, "/")
				if len(parts) != 2 || parts[0] == "" || parts[1] != "connection" {
					http.NotFound(w, r)
					return
				}

				switch r.Method {
				case http.MethodPost:
					var payload runner.IntegrationConnectionRequest
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
						return
					}

					result, err := instance.TriggerIntegrationConnection(r.Context(), parts[0], payload)
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}

					writeHTTPJSON(w, result)
				case http.MethodDelete:
					if err := instance.DeleteIntegrationConnection(r.Context(), parts[0]); err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
			})
			mux.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload runner.PromptExecutionRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				result, err := instance.ExecutePrompt(r.Context(), payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}

				writeHTTPJSON(w, result)
			})
			mux.HandleFunc("/sessions/start", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload runner.AiSessionStartRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				result, err := instance.StartSession(r.Context(), payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, result)
			})
			mux.HandleFunc("/sessions", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				writeHTTPJSON(w, instance.ListSessions())
			})
			mux.HandleFunc("/sessions/message", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload runner.AiSessionMessageRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				result, err := instance.SendMessage(r.Context(), payload)
				if err != nil {
					// Structured session_dead error contract
					if strings.HasPrefix(err.Error(), "session_dead:") {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						json.NewEncoder(w).Encode(map[string]string{
							"code":    "session_dead",
							"message": "session process exited or is no longer registered",
							"details": err.Error(),
						})
						return
					}
					if strings.HasPrefix(err.Error(), "session_terminated:") {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusConflict)
						json.NewEncoder(w).Encode(map[string]string{
							"code":    "session_terminated",
							"message": "session was intentionally terminated",
							"details": err.Error(),
						})
						return
					}
					if strings.HasPrefix(err.Error(), "mcp_write_approval_required:") {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusConflict)
						json.NewEncoder(w).Encode(map[string]string{
							"code":    "mcp_write_approval_required",
							"message": "manual approval is required before retrying this Google Drive write",
							"details": err.Error(),
						})
						return
					}
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, result)
			})
			mux.HandleFunc("/sessions/message/stream", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				flusher, ok := w.(http.Flusher)
				if !ok {
					writeHTTPError(w, http.StatusInternalServerError, fmt.Errorf("streaming is not supported by this response writer"))
					return
				}
				var payload runner.AiSessionMessageRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}

				w.Header().Set("Content-Type", "application/x-ndjson")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("X-Accel-Buffering", "no")
				encoder := json.NewEncoder(w)
				writeEvent := func(event runner.SessionStreamEvent) {
					_ = encoder.Encode(event)
					flusher.Flush()
				}

				result, err := instance.SendMessageWithCallback(r.Context(), payload, writeEvent)
				if err != nil {
					event := runner.SessionStreamEvent{
						Type:    "error",
						Error:   err.Error(),
						Details: err.Error(),
					}
					if strings.HasPrefix(err.Error(), "session_dead:") {
						event.Code = "session_dead"
						event.Error = "session process exited or is no longer registered"
					} else if strings.HasPrefix(err.Error(), "session_terminated:") {
						event.Code = "session_terminated"
						event.Error = "session was intentionally terminated"
					} else if strings.HasPrefix(err.Error(), "mcp_write_approval_required:") {
						event.Code = "mcp_write_approval_required"
						event.Error = "manual approval is required before retrying this Google Drive write"
					}
					writeEvent(event)
					return
				}
				writeEvent(runner.SessionStreamEvent{Type: "result", Result: &result})
			})
			mux.HandleFunc("/sessions/close", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				var payload runner.AiSessionHandle
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
					return
				}
				err := instance.CloseSession(r.Context(), payload)
				if err != nil {
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				w.WriteHeader(http.StatusOK)
			})
			mux.HandleFunc("/compat", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					writeHTTPJSON(w, instance.CompatLoadInfo(r.Context()))
				case http.MethodPost:
					writeHTTPJSON(w, instance.RunCompatCheck(r.Context()))
				default:
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
			})
			mux.HandleFunc("/compat-config", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					config, err := instance.LoadCompatConfig()
					if err != nil {
						writeHTTPError(w, http.StatusInternalServerError, err)
						return
					}
					writeHTTPJSON(w, config)
				case http.MethodPut:
					var payload runner.CompatConfig
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
						return
					}
					config, err := instance.SaveCompatConfig(payload)
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}
					writeHTTPJSON(w, config)
				default:
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
			})
			mux.HandleFunc("/compat/deep", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				writeHTTPJSON(w, instance.RunCompatDeepCheck(r.Context()))
			})
			mux.HandleFunc("/system/shutdown", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				instance.CleanupSessions()

				err := writeSupervisorCommand(instance.Health().Cwd, "shutdown")
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}

				w.WriteHeader(http.StatusAccepted)
				w.Write([]byte(`{"status":"accepted"}`))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			})
			mux.HandleFunc("/system/restart", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				instance.CleanupSessions()

				err := writeSupervisorCommand(instance.Health().Cwd, "restart")
				if err != nil {
					writeHTTPError(w, http.StatusInternalServerError, err)
					return
				}

				w.WriteHeader(http.StatusAccepted)
				w.Write([]byte(`{"status":"accepted"}`))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			})

			// Graceful shutdown on SIGINT/SIGTERM
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigChan
				fmt.Println("\nShutting down local runner. Cleaning up active sessions...")
				instance.CleanupSessions()
				os.Exit(0)
			}()

			addr := netJoinHostPort(cfg.host, cfg.port)
			// Loopback base URL the Claude adapter uses to build per-turn --mcp-config URLs (07).
			instance.SetMCPBaseURL(fmt.Sprintf("http://127.0.0.1:%v", cfg.port))
			fmt.Fprintf(os.Stdout, "FlowPilot runner listening on http://%s\n", addr)
			listenErr := http.ListenAndServe(addr, withCORS(mux))
			instance.CleanupSessions()
			return listenErr
		},
	})

	return runnerCmd
}

func newProvidersCommand(cfg *config) *cobra.Command {
	providersCmd := &cobra.Command{
		Use:   "providers",
		Short: "Inspect AI provider CLI installations",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Print discovered providers as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}
			inventory, err := instance.ListProviders(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(os.Stdout, inventory)
		},
	}
	listCmd.Flags().Bool("json", true, "Emit JSON output")
	providersCmd.AddCommand(listCmd)
	providersCmd.AddCommand(newProviderTerminalCommand(cfg))

	providersCmd.AddCommand(&cobra.Command{
		Use:   "detect",
		Short: "Print installed provider CLI information as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}
			providers, err := instance.DetectProviders(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(os.Stdout, providers)
		},
	})

	return providersCmd
}

func newInstallProviderCommand(cfg *config) *cobra.Command {
	installCmd := &cobra.Command{
		Use:   "install-provider <provider_name>",
		Short: "Install a provider CLI and refresh the provider inventory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}

			inventory, installErr := instance.InstallProvider(cmd.Context(), args[0])
			if writeErr := writeJSON(os.Stdout, inventory); writeErr != nil {
				return writeErr
			}
			if installErr != nil {
				return installErr
			}

			return nil
		},
	}
	installCmd.Flags().Bool("json", true, "Emit JSON output")

	return installCmd
}

func newBackendsCommand(cfg *config) *cobra.Command {
	backendsCmd := &cobra.Command{
		Use:   "backends",
		Short: "Inspect allowlisted MCP backend installations",
	}

	backendsCmd.AddCommand(&cobra.Command{
		Use:   "detect",
		Short: "Print allowlisted MCP backend information as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}
			backends, err := instance.ListMcpBackends(context.Background())
			if err != nil {
				return err
			}
			return writeJSON(os.Stdout, backends)
		},
	})

	return backendsCmd
}

func newGoogleDriveMcpCommand(cfg *config) *cobra.Command {
	var accountHomePath string
	var mode string
	var yoloMode bool

	cmd := &cobra.Command{
		Use:   "google-drive-mcp",
		Short: "Run the FlowPilot Google Drive proxy MCP server over stdio",
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}
			return instance.RunGoogleDriveProxyMcpServer(cmd.Context(), cfg.workspace, accountHomePath, mode, yoloMode)
		},
	}

	cmd.Flags().StringVar(&accountHomePath, "account-home", "", "Provider account home path used for discovery and diagnostics")
	cmd.Flags().StringVar(&mode, "mode", "read_only", "Proxy MCP mode: read_only or read_write")
	cmd.Flags().BoolVar(&yoloMode, "yolo-mode", false, "Enable yolo approval mode for proxy MCP approval handling")

	return cmd
}

func newSkillsCommand(cfg *config) *cobra.Command {
	skillsCmd := &cobra.Command{
		Use:   "skills",
		Short: "Inspect local skill markdown files",
	}

	skillsCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Print discovered skills as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}
			skills, err := instance.ListSkills()
			if err != nil {
				return err
			}
			return writeJSON(os.Stdout, skills)
		},
	})

	return skillsCmd
}

func newFlowsCommand(cfg *config) *cobra.Command {
	flowsCmd := &cobra.Command{
		Use:   "flows",
		Short: "Inspect local flow markdown files",
	}

	flowsCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Print discovered flows as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}
			flows, err := instance.ListFlows()
			if err != nil {
				return err
			}
			return writeJSON(os.Stdout, flows)
		},
	})

	return flowsCmd
}

func writeJSON(w interface{ Write([]byte) (int, error) }, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeHTTPJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeHTTPError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Loopback stance (04-02): reflect any localhost/127.0.0.1 origin so both the
		// admin web (:3002) and the desktop client (Electron / Vite dev server on any
		// port) can call the runner; default to the admin-web origin otherwise.
		if isLoopbackOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:3002")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, Last-Event-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func netJoinHostPort(host string, port int) string {
	return host + ":" + strconv.Itoa(port)
}

func isLoopbackOrigin(origin string) bool {
	return strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:") ||
		origin == "http://localhost" ||
		origin == "http://127.0.0.1"
}

func requestBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

func writeSupervisorCommand(workspace string, command string) error {
	cmdPath := filepath.Join(workspace, ".flowpilot", "supervisor.cmd")
	if err := os.MkdirAll(filepath.Dir(cmdPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(cmdPath, []byte(command), 0644)
}
