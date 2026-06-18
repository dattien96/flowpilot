package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
)

type codexResumeAdapter struct {
	accountHome string
	promptPrep  func(TurnRequest) string
}

func newCodexResumeAdapter(accountHome string, promptPrep func(TurnRequest) string) *codexResumeAdapter {
	return &codexResumeAdapter{accountHome: accountHome, promptPrep: promptPrep}
}

func (a *codexResumeAdapter) Key() ProviderKey { return ProviderKeyCodex }

func (a *codexResumeAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
		SkillSelection: true, Mcp: true, Interrupt: true, Vision: true,
	}
}

func (a *codexResumeAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	sandbox, approvalMode := codexYoloDerive(req.YoloMode)
	prompt := req.Prompt + askUserReinforcement
	if a.promptPrep != nil {
		prompt = a.promptPrep(req)
	}

	outputFile, err := os.CreateTemp("", "flowpilot-codex-last-*.txt")
	if err != nil {
		return err
	}
	outputPath := outputFile.Name()
	_ = outputFile.Close()
	defer os.Remove(outputPath)

	imagePaths, cleanupImages, err := writeCodexImageAttachments(req.ProviderTurnID, req.Attachments)
	if err != nil {
		return err
	}
	defer cleanupImages()

	args := []string{
		"exec", "resume", req.ProviderSessionID, "--all",
		"-c", fmt.Sprintf(`sandbox_mode=%q`, sandbox),
		"-c", fmt.Sprintf(`approval_policy=%q`, approvalMode),
		"-o", outputPath,
	}
	if req.ModelName != "" {
		args = append(args, "-m", req.ModelName)
	}
	for _, imagePath := range imagePaths {
		args = append(args, "-i", imagePath)
	}
	if prompt != "" {
		args = append(args, prompt)
	}

	cmd := commandContextFn(ctx, codexBinaryName(), args...)
	cmd.Dir = req.Cwd
	cmd.Env = append(os.Environ(), "CODEX_HOME="+a.accountHome)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("codex resume failed: %s", msg)
	}

	finalMessage := readLastMessageFile(outputPath)
	if finalMessage == "" {
		finalMessage = strings.TrimSpace(stdout.String())
	}
	if finalMessage != "" {
		bridge.Emit(ProviderEvent{Type: EventMessageCompleted, Text: finalMessage})
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: finalMessage})
	return nil
}

func readLastMessageFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
