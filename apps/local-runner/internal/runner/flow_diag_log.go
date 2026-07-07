package runner

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const flowDiagFeatureKey = "agent-flow-engine"

var flowDiagWriteMu sync.Mutex

func flowDiagEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWPILOT_LOG_FLOW_DIAG"))) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func flowDiagDir() string {
	if override := strings.TrimSpace(os.Getenv("FLOWPILOT_FLOW_DIAG_DIR")); override != "" {
		return filepath.Clean(override)
	}
	if wd, err := os.Getwd(); err == nil && strings.TrimSpace(wd) != "" {
		for dir := wd; dir != "" && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "apps", "local-runner")); err == nil {
				return filepath.Join(dir, ".flowpilot", "logs", "features", flowDiagFeatureKey)
			}
		}
	}
	return filepath.Join(".flowpilot", "logs", "features", flowDiagFeatureKey)
}

func (s *InteractiveService) flowDiagLog(parentRunID, event, message string, attrs ...any) {
	if !flowDiagEnabled() || strings.TrimSpace(parentRunID) == "" {
		return
	}
	payload := make([]any, 0, len(attrs)+14)
	payload = append(payload,
		"feature_key", flowDiagFeatureKey,
		"event", event,
		"parent_run_id", parentRunID,
	)

	loop := s.agentOrchestrator.loopStateFor(parentRunID)
	payload = append(payload,
		"loop_mode", loop.Mode,
		"loop_status", loop.Status,
		"loop_round", loop.Round,
		"loop_cap", effectiveCap(loop),
		"loop_open_issues", loop.OpenIssues,
		"loop_gate_reason", loop.GateReason,
	)
	payload = append(payload, attrs...)
	writeFlowDiagEntry(parentRunID, message, payload...)
}

func writeFlowDiagEntry(parentRunID, message string, attrs ...any) {
	dir := flowDiagDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	record := make([]any, 0, len(attrs)+2)
	record = append(record, "run_file", parentRunID+".ndjson")
	record = append(record, attrs...)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))
	logger.Info(message, record...)
	line := buf.Bytes()
	if len(line) == 0 {
		return
	}
	if line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}

	runPath := filepath.Join(dir, parentRunID+".ndjson")
	latestPath := filepath.Join(dir, "latest.ndjson")
	indexPath := filepath.Join(dir, "latest-path.txt")

	flowDiagWriteMu.Lock()
	defer flowDiagWriteMu.Unlock()
	_, _ = appendFile(runPath, line)
	_, _ = appendFile(latestPath, line)
	_ = os.WriteFile(indexPath, []byte(runPath+"\n"+time.Now().UTC().Format(time.RFC3339Nano)+"\n"), 0o644)
}

func appendFile(path string, line []byte) (int, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.Write(line)
}
