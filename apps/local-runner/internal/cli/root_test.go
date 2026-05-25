package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProviderCommandsEmitJSONInventory(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	t.Setenv("GEMINI_API_KEY", "test-gemini-key")

	writeMockProviderBinary(t, binDir, "codex", "codex 1.2.3")
	writeMockProviderBinary(t, binDir, "claude", "claude 4.5.6")
	writeMockProviderBinary(t, binDir, "gemini", "gemini 7.8.9")

	listStdout, listStderr, listErr := executeRootCommand(t, "--workspace", workspace, "providers", "list", "--json")
	if listErr != nil {
		t.Fatalf("providers list: %v\nstderr: %s", listErr, listStderr)
	}

	var inventory struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := json.Unmarshal([]byte(listStdout), &inventory); err != nil {
		t.Fatalf("unmarshal providers list output: %v\noutput: %s", err, listStdout)
	}
	if len(inventory.Providers) != 3 {
		t.Fatalf("expected 3 providers from list output, got %d", len(inventory.Providers))
	}

	detectStdout, detectStderr, detectErr := executeRootCommand(t, "--workspace", workspace, "providers", "detect")
	if detectErr != nil {
		t.Fatalf("providers detect: %v\nstderr: %s", detectErr, detectStderr)
	}

	var detected []map[string]any
	if err := json.Unmarshal([]byte(detectStdout), &detected); err != nil {
		t.Fatalf("unmarshal providers detect output: %v\noutput: %s", err, detectStdout)
	}
	if len(detected) != 3 {
		t.Fatalf("expected 3 providers from detect output, got %d", len(detected))
	}

	installStdout, installStderr, installErr := executeRootCommand(t, "--workspace", workspace, "install-provider", "codex")
	if installErr != nil {
		t.Fatalf("install-provider codex: %v\nstderr: %s", installErr, installStderr)
	}

	var installInventory struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := json.Unmarshal([]byte(installStdout), &installInventory); err != nil {
		t.Fatalf("unmarshal install-provider output: %v\noutput: %s", err, installStdout)
	}
	codex := findProviderJSON(installInventory.Providers, "codex")
	if codex == nil {
		t.Fatal("expected codex provider in install-provider output")
	}
	if codex["install_status"] != "INSTALLED" {
		t.Fatalf("expected codex install status installed, got %#v", codex["install_status"])
	}
	if codex["detected_binary"] != "codex" {
		t.Fatalf("expected codex detected binary, got %#v", codex["detected_binary"])
	}
}

func findProviderJSON(providers []map[string]any, key string) map[string]any {
	for _, provider := range providers {
		if provider["key"] == key {
			return provider
		}
	}
	return nil
}

func executeRootCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	root := NewRootCommand()
	root.SetArgs(args)

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}

	originalStdout := os.Stdout
	originalStderr := os.Stderr
	os.Stdout = stdoutW
	os.Stderr = stderrW

	runErr := root.Execute()

	if closeErr := stdoutW.Close(); closeErr != nil {
		t.Fatalf("close stdout pipe: %v", closeErr)
	}
	if closeErr := stderrW.Close(); closeErr != nil {
		t.Fatalf("close stderr pipe: %v", closeErr)
	}

	os.Stdout = originalStdout
	os.Stderr = originalStderr

	stdoutBytes, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	stderrBytes, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	return strings.TrimSpace(string(stdoutBytes)), strings.TrimSpace(string(stderrBytes)), runErr
}

func writeMockProviderBinary(t *testing.T, dir, name, version string) string {
	t.Helper()

	switch runtime.GOOS {
	case "windows":
		path := filepath.Join(dir, name+".cmd")
		script := "@echo off\r\nif \"%~1\"==\"--version\" (\r\n  echo " + version + "\r\n  exit /b 0\r\n)\r\nif \"%~1\"==\"auth\" (\r\n  exit /b 0\r\n)\r\nexit /b 0\r\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write mock provider binary: %v", err)
		}
		return path
	default:
		path := filepath.Join(dir, name)
		script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo \"" + version + "\"\n  exit 0\nfi\nif [ \"$1\" = \"auth\" ]; then\n  exit 0\nfi\nexit 0\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write mock provider binary: %v", err)
		}
		return path
	}
}
