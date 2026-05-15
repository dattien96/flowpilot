package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

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

	providersCmd.AddCommand(&cobra.Command{
		Use:   "detect",
		Short: "Print installed provider CLI information as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}
			providers, err := instance.DetectProviders(context.Background())
			if err != nil {
				return err
			}
			return writeJSON(os.Stdout, providers)
		},
	})

	return providersCmd
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
		if origin == "http://127.0.0.1:3001" || origin == "http://localhost:3001" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:3001")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
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
