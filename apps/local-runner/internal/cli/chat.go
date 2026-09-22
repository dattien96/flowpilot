package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"flowpilot-runner/internal/runner"
	tuiapp "flowpilot-runner/internal/tui/app"
	tuicfg "flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/desktopboot"
	"flowpilot-runner/internal/tui/runnerboot"
)

// newChatCommand creates the `flowpilot chat` Cobra command.
// It reuses the root persistent flags (--workspace, --host, --port) and adds
// chat-local flags only (CP-56 architecture: no shadowing of persistent flags).
func newChatCommand(cfg *config) *cobra.Command {
	var (
		runnerURL     string
		noStartRunner bool
		projectPath   string
		provider      string
		model         string
		reasoning     string
		yolo          bool
		printMode     bool
		prompt        string
		resumeRunID   string
		timeout       string
	)

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Open an interactive chat with the FlowPilot runner",
		Long: `Start a terminal chat session with the FlowPilot runner.

The runner is auto-started if not already running (use --no-start-runner to disable).
The workspace is resolved via --workspace flag, FLOWPILOT_WORKSPACE env, or by
walking up the directory tree to find a FlowPilot root (apps/local-runner or .agents).

Examples:
  just chat-dev --project D:/working/gate-sandbox
  just chat-dev --project D:\working\gate-sandbox
  flowpilot chat --project /Users/me/gate-sandbox
  flowpilot chat --yolo
  flowpilot chat --print "What is 2+2?"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Positional arg as prompt (for --print convenience)
			if len(args) > 0 && prompt == "" {
				prompt = args[0]
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolve cwd: %w", err)
			}

			// Resolve runner workspace: --workspace → env → walk up.
			workspace := cfg.workspace
			if workspace == "" {
				workspace = os.Getenv("FLOWPILOT_WORKSPACE")
			}
			if workspace == "" {
				workspace = runnerboot.FindWorkspaceRoot(cwd)
			}

			// --project accepts Windows/macOS/Linux paths (\, /, quoted).
			// just chat-dev always sets FLOWPILOT_CHAT_REQUIRE_PROJECT (FlowPilot checkout ≠ target).
			if strings.TrimSpace(projectPath) == "" && os.Getenv("FLOWPILOT_CHAT_REQUIRE_PROJECT") == "1" {
				return fmt.Errorf("project path is required: just chat-dev --project D:/working/gate-sandbox")
			}
			resolvedProject, err := tuicfg.ResolveProjectPath(projectPath)
			if err != nil {
				return err
			}

			// Build chat config (combines root persistent flags + chat-local flags).
			// Provider/model are optional — TUI loads the active account on connect.
			chatCfg := tuicfg.ChatConfig{
				RunnerURL:       runnerURL,
				NoStartRunner:   noStartRunner,
				RunnerWorkspace: workspace,
				RunnerHost:      cfg.host,
				RunnerPort:      cfg.port,
				DesktopHost:     "127.0.0.1",
				DesktopPort:     desktopboot.DefaultPort(),
				ProjectPath:     resolvedProject,
				Provider:        provider,
				Model:           model,
				ReasoningEffort: reasoning,
				Yolo:            yolo,
				Print:           printMode,
				Prompt:          prompt,
				ResumeRunID:     resumeRunID,
				Timeout:         timeout,
			}

			// Auto-ensure runner is online. CP-81: the expected lifecycle
			// identity lets EnsureRunner reuse compatible runners, fence
			// stale-build replacement to idle runners, and never kill foreign
			// listeners on the port.
			bootResult, err := runnerboot.EnsureRunner(cmd.Context(), runnerboot.Config{
				ExplicitURL:             runnerURL,
				NoStart:                 noStartRunner,
				Workspace:               workspace,
				Host:                    cfg.host,
				Port:                    cfg.port,
				ExpectedBuildID:         runner.EffectiveBuildID(),
				ExpectedProtocolVersion: runner.ProtocolVersion,
			})
			if err != nil {
				return fmt.Errorf("runner unavailable: %w", err)
			}

			if bootResult.Launched {
				chatCfg.OwnsRunner = true
				fmt.Fprintf(os.Stderr, "Runner started at %s (shared, live Codex enabled)\n", bootResult.RunnerURL)
			} else if bootResult.Reused {
				fmt.Fprintf(os.Stderr, "Reusing runner at %s\n", bootResult.RunnerURL)
				if bootResult.UpdatePending {
					fmt.Fprintf(os.Stderr, "Runner is busy on an older build; it will be replaced when idle.\n")
				}
				fmt.Fprintf(os.Stderr, "If Codex replies look scripted, kill that runner and restart chat (needs FLOWPILOT_CODEX_APPSERVER=1).\n")
			}

			// Launch the TUI (or headless mode when --print is set).
			return tuiapp.Run(chatCfg, bootResult.RunnerURL)
		},
	}

	// Chat-local flags only — never shadow root persistent flags.
	cmd.Flags().StringVar(&runnerURL, "runner-url", "", "Explicit runner URL (skips auto-start)")
	cmd.Flags().BoolVar(&noStartRunner, "no-start-runner", false, "Do not auto-start a runner if not running")
	cmd.Flags().StringVarP(&projectPath, "project", "P", "", "Target project directory (Windows/macOS/Linux paths; required for just chat-dev)")
	cmd.Flags().StringVar(&provider, "provider", "", "Optional provider override (default: active account / runner default)")
	cmd.Flags().StringVar(&model, "model", "", "Optional model override (default: provider default)")
	cmd.Flags().StringVar(&reasoning, "reasoning", "", "Reasoning effort (high, medium, low)")
	cmd.Flags().BoolVar(&yolo, "yolo", false, "Enable YOLO mode (auto-approve all)")
	cmd.Flags().BoolVarP(&printMode, "print", "p", false, "Headless: send prompt and print response, then exit")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Initial prompt (used with --print)")
	cmd.Flags().StringVar(&resumeRunID, "resume", "", "Resume a previous run by ID")
	cmd.Flags().StringVar(&timeout, "timeout", "", "Session timeout (e.g. 30m, 2h)")

	return cmd
}
