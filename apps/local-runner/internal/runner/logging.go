package runner

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// maxRunnerLogBytes bounds the active log file before a single-generation rotation.
const maxRunnerLogBytes = 10 << 20 // 10 MiB

// DefaultLogFilePath returns the runner's log file path, alongside the FlowPilot config
// dir used for provider accounts (so logs sit next to provider-accounts.json under
// %AppData%\FlowPilot on Windows). Overridable via FLOWPILOT_RUNNER_LOG_FILE.
func DefaultLogFilePath() string {
	if override := strings.TrimSpace(os.Getenv("FLOWPILOT_RUNNER_LOG_FILE")); override != "" {
		return filepath.Clean(override)
	}
	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		return filepath.Join(configDir, "FlowPilot", "logs", "runner.log")
	}
	if homeDir := preferredUserHomeDir(); homeDir != "" {
		return filepath.Join(homeDir, ".flowpilot", "logs", "runner.log")
	}
	return filepath.Join(".", ".flowpilot", "logs", "runner.log")
}

// SetupFileLogging tees the standard logger to both stderr and a log file at path so the
// runner's diagnostics ([agent-spawn], [chat-history-open], [claude-mcp], …) are captured
// to disk and not just the launching terminal. It rotates one previous generation once the
// active file exceeds maxRunnerLogBytes. Best-effort: on any error it leaves stderr-only
// logging intact and returns a no-op closer so startup never fails over logging.
func SetupFileLogging(path string) (func() error, error) {
	noop := func() error { return nil }
	if strings.TrimSpace(path) == "" {
		return noop, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return noop, err
	}
	if info, err := os.Stat(path); err == nil && info.Size() > maxRunnerLogBytes {
		_ = os.Rename(path, path+".1") // best-effort single-generation rotation
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return noop, err
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.Printf("[runner] file logging enabled path=%q", path)
	return f.Close, nil
}
