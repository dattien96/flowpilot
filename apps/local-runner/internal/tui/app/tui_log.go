package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	tuiLogMu   sync.Mutex
	tuiLogPath string
)

func initTUILog() {
	// Called once from New() / Run() — best-effort, never blocks TUI.
	var dir string
	if runtime.GOOS == "windows" {
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			dir = filepath.Join(appData, "FlowPilot")
		}
	}
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".flowpilot")
		}
	}
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0o755)
	tuiLogPath = filepath.Join(dir, "tui.log")
	// Append, don't truncate — keeps hang evidence across restarts. Rotate if huge.
	if info, err := os.Stat(tuiLogPath); err == nil && info.Size() > 5<<20 {
		_ = os.Rename(tuiLogPath, tuiLogPath+".old")
	}
	tuiLog("=== TUI start pid=%d goos=%s time=%s runner=%s ===", os.Getpid(), runtime.GOOS, time.Now().Format(time.RFC3339), "")
	if dir != "" {
		tuiLog("log file: %s", tuiLogPath)
	}
}

func tuiLog(format string, args ...any) {
	if strings.TrimSpace(os.Getenv("FLOWPILOT_TUI_SKIP_MODE_RESTORE")) != "" {
		return
	}
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s [%d] %s\n", time.Now().Format("15:04:05.000"), os.Getpid(), msg)
	tuiLogMu.Lock()
	defer tuiLogMu.Unlock()
	if tuiLogPath == "" {
		return
	}
	// Open, append, close each time so TempDir cleanup in tests can remove the file.
	f, err := os.OpenFile(tuiLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line)
	_ = f.Close()
}

// tuiLogClose is called on quit.
func tuiLogClose() {
	tuiLog("=== TUI exit ===")
}

func tuiLogPathForUser() string {
	if tuiLogPath != "" {
		return tuiLogPath
	}
	return "(no log file)"
}
