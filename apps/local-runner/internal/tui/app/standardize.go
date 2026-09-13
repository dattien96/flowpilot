package app

// Task-333 (CP-49): TUI support for the `/standardize [scope]` command.
//
// The command is registered into knownSlashCommands here (so /help and the
// autocomplete surface list it without touching model.go) and dispatched from
// handleSlashCommand (app.go) into cmdStandardize below. The call goes to the
// runner API (POST /client/standardize) — the runner owns the whole flow:
// CP-48 conformance for scopes that already have docs, CP-49 reverse-doc +
// SS-Lock pause for brownfield scopes. While a run is parked at the
// non-bypassable SS-Lock gate, approval happens in the Desktop SS-Lock modal
// or via POST /client/workflow-runs/{runId}/confirm (same endpoint the modal
// uses); the TUI surfaces the run id and draft paths for it.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func init() {
	knownSlashCommands = append(knownSlashCommands, slashCommand{
		name:        "/standardize",
		description: "Standardize docs — /standardize [scope]; conformance scan or reverse-doc with SS-Lock",
	})
}

// standardizeHTTPTimeout bounds the synchronous runner call. The conformance
// path is milliseconds; the reverse-doc path includes bounded GitNexus/git
// subprocesses (see internal/runner/reverse_doc.go timeouts).
const standardizeHTTPTimeout = 45 * time.Second

// standardizeAPIResult mirrors the runner's StandardizeResult JSON.
type standardizeAPIResult struct {
	Mode        string `json:"mode"`
	RunID       string `json:"run_id,omitempty"`
	DraftSSPath string `json:"draft_ss_path,omitempty"`
	DraftSDPath string `json:"draft_sd_path,omitempty"`
	Status      string `json:"status"`
	ScanReport  *struct {
		TotalFilesScanned int `json:"total_files_scanned"`
		ConformingFiles   int `json:"conforming_files"`
		Issues            []struct {
			FilePath string `json:"file_path"`
			Severity string `json:"severity"`
			RuleID   string `json:"rule_id"`
			Message  string `json:"message"`
		} `json:"issues"`
	} `json:"scan_report,omitempty"`
}

// cmdStandardize executes the /standardize command synchronously and renders
// the runner result into the conversation.
func (m *AppModel) cmdStandardize(args []string) (tea.Model, tea.Cmd) {
	scope := map[string]string{}
	if len(args) > 0 {
		rest := strings.TrimSpace(strings.Join(args, " "))
		if rest != "" {
			if strings.ContainsRune(rest, '/') || strings.ContainsRune(rest, '\\') {
				scope["path"] = rest
			} else {
				scope["feature_name"] = rest
			}
		}
	}
	body, err := json.Marshal(scope)
	if err != nil {
		m.addMessage("system", "/standardize: "+err.Error(), "error")
		return m, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), standardizeHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(m.runnerURL, "/")+"/client/standardize", bytes.NewReader(body))
	if err != nil {
		m.addMessage("system", "/standardize: "+err.Error(), "error")
		return m, nil
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		m.addMessage("system", "/standardize: runner unreachable: "+err.Error(), "error")
		return m, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		m.addMessage("system", fmt.Sprintf("/standardize failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(raw))), "error")
		return m, nil
	}
	var result standardizeAPIResult
	if err := json.Unmarshal(raw, &result); err != nil {
		m.addMessage("system", "/standardize: unexpected runner response: "+err.Error(), "error")
		return m, nil
	}
	m.addMessage("system", renderStandardizeResult(result), "")
	return m, nil
}

// renderStandardizeResult turns the runner result into TUI text.
func renderStandardizeResult(result standardizeAPIResult) string {
	var sb strings.Builder
	switch result.Mode {
	case "conformance":
		sb.WriteString("[standardize] mode=conformance — CP-48 scan complete.\n")
	case "reverse_doc":
		sb.WriteString("[standardize] mode=reverse_doc — CP-49 reverse-documentation.\n")
	case "mixed":
		sb.WriteString("[standardize] mode=mixed — conformance on existing docs + reverse-doc backfill.\n")
	default:
		sb.WriteString("[standardize] mode=" + result.Mode + "\n")
	}
	if r := result.ScanReport; r != nil {
		sb.WriteString(fmt.Sprintf("  scanned %d doc(s), %d conforming, %d issue(s)\n", r.TotalFilesScanned, r.ConformingFiles, len(r.Issues)))
		for i, is := range r.Issues {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("  … and %d more issue(s)\n", len(r.Issues)-10))
				break
			}
			sb.WriteString(fmt.Sprintf("  [%s] %s: %s\n", is.Severity, is.FilePath, is.Message))
		}
	}
	if result.DraftSDPath != "" {
		sb.WriteString("  SD draft: " + result.DraftSDPath + "\n")
	}
	if result.DraftSSPath != "" {
		sb.WriteString("  SS draft: " + result.DraftSSPath + "\n")
	}
	if result.Status == "waiting_ss_lock" {
		sb.WriteString("  SS-Lock: workflow paused — review the SS draft, refine the acceptance criteria, then approve.\n")
		sb.WriteString("  Approve/reject via the Desktop SS-Lock modal or POST /client/workflow-runs/" + result.RunID + "/confirm (action approve|reject, edits = updated SS).\n")
		sb.WriteString("  The AI cannot continue past this gate until a human confirms.\n")
	} else {
		sb.WriteString("  status: " + result.Status + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
