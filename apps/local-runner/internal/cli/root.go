package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

			mux := http.NewServeMux()
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

					artifact, err := instance.SyncArtifact(artifactID)
					if err != nil {
						writeHTTPError(w, http.StatusBadRequest, err)
						return
					}

					writeHTTPJSON(w, artifact)
					return
				}

				http.NotFound(w, r)
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
					writeHTTPError(w, http.StatusBadRequest, err)
					return
				}
				writeHTTPJSON(w, result)
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
			fmt.Fprintf(os.Stdout, "FlowPilot runner listening on http://%s\n", addr)
			return http.ListenAndServe(addr, withCORS(mux))
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
	http.Error(w, err.Error(), status)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://127.0.0.1:3002" || origin == "http://localhost:3002" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:3002")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

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
