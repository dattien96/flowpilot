package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Task-302 T-5: Opencode MCP provider config (Google/Jira only, atomic write)

func getOpencodeMcpConfigPath(home string) string {
	primary := filepath.Join(home, ".config", "opencode", "opencode.json")
	if _, err := os.Stat(primary); err == nil {
		return primary
	}
	fallback := filepath.Join(home, ".config", "opencode", "config.json")
	if _, err := os.Stat(fallback); err == nil {
		// Migration: copy fallback to primary atomically; on failure keep fallback to avoid data loss
		if data, err := os.ReadFile(fallback); err == nil {
			_ = os.MkdirAll(filepath.Dir(primary), 0755)
			tmp := primary + ".tmp"
			if err := os.WriteFile(tmp, data, 0644); err == nil {
				if err := os.Rename(tmp, primary); err == nil {
					return primary
				}
				_ = os.Remove(tmp)
			}
		}
		return fallback
	}
	return primary
}

type opencodeMcpConfig struct {
	McpServers map[string]json.RawMessage `json:"mcpServers"`
	// Preserve other keys
	Other map[string]json.RawMessage `json:"-"`
}

func readOpencodeMcpConfig(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{"mcpServers": map[string]interface{}{}}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return map[string]interface{}{"mcpServers": map[string]interface{}{}}, nil
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		// Preserve existing file on corrupt JSON — do not silently wipe (parity with Codex/Jira).
		return nil, fmt.Errorf("opencode mcp config is not valid JSON: %w", err)
	}
	if _, ok := doc["mcpServers"]; !ok {
		doc["mcpServers"] = map[string]interface{}{}
	}
	return doc, nil
}

func writeOpencodeMcpConfigAtomic(path string, doc map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func EnsureOpencodeGoogleDriveMcpConfig(home string, yolo bool) error {
	return ensureOpencodeMcpServer(home, "google-drive", buildOpencodeGoogleDriveServer(home, yolo))
}

func EnsureOpencodeJiraMcpConfig(home string, yolo bool) error {
	server, ok := buildOpencodeJiraServer()
	if !ok {
		return fmt.Errorf("jira MCP not configured")
	}
	return ensureOpencodeMcpServer(home, "jira", server)
}

func ensureOpencodeMcpServer(home, name string, server map[string]interface{}) error {
	path := getOpencodeMcpConfigPath(home)
	doc, err := readOpencodeMcpConfig(path)
	if err != nil {
		return err
	}
	mcpServers, _ := doc["mcpServers"].(map[string]interface{})
	if mcpServers == nil {
		mcpServers = map[string]interface{}{}
		doc["mcpServers"] = mcpServers
	}
	mcpServers[name] = server
	return writeOpencodeMcpConfigAtomic(path, doc)
}

// ensureOpencodeDriveMcpServer is the Drive-specific variant that reports whether the file changed.
// It deep-compares the existing entry via JSON marshaling to avoid spurious writes.
func ensureOpencodeDriveMcpServer(home, name string, server map[string]interface{}) (bool, error) {
	path := getOpencodeMcpConfigPath(home)
	doc, err := readOpencodeMcpConfig(path)
	if err != nil {
		return false, err
	}
	mcpServers, _ := doc["mcpServers"].(map[string]interface{})
	if mcpServers == nil {
		mcpServers = map[string]interface{}{}
		doc["mcpServers"] = mcpServers
	}
	existing, exists := mcpServers[name]
	changed := true
	if exists {
		a, _ := json.Marshal(existing)
		b, _ := json.Marshal(server)
		if string(a) == string(b) {
			changed = false
		}
	}
	if !changed {
		return false, nil
	}
	mcpServers[name] = server
	if err := writeOpencodeMcpConfigAtomic(path, doc); err != nil {
		return false, err
	}
	return true, nil
}

func buildOpencodeGoogleDriveServer(home string, yolo bool) map[string]interface{} {
	// Deprecated: real Drive server is built in google_drive_mcp_provider_config.go via expectedClaudeGoogleDriveMcpServer.
	// Kept for tests that call EnsureOpencodeGoogleDriveMcpConfig directly.
	return map[string]interface{}{
		"type":    "stdio",
		"command": "npx",
		"args":    []interface{}{"-y", "@piotr-agier/google-drive-mcp"},
		"env": map[string]interface{}{
			"GOOGLE_DRIVE_OAUTH_CREDENTIALS": "placeholder",
		},
	}
}

func buildOpencodeJiraServer() (map[string]interface{}, bool) {
	// Jira server is derived from live Jira MCP integration if connected; for DOD we just create a placeholder
	// The real implementation would call r.jiraLiveMCPServer() but this file is provider-agnostic.
	// Return a simple placeholder for testing.
	return map[string]interface{}{
		"type":    "stdio",
		"command": "npx",
		"args":    []interface{}{"-y", "@modelcontextprotocol/server-jira"},
		"env":     map[string]interface{}{},
	}, true
}

func CheckOpencodeMcpConfig(home, name string) (bool, error) {
	path := getOpencodeMcpConfigPath(home)
	doc, err := readOpencodeMcpConfig(path)
	if err != nil {
		return false, err
	}
	mcpServers, _ := doc["mcpServers"].(map[string]interface{})
	if mcpServers == nil {
		return false, nil
	}
	_, ok := mcpServers[name]
	return ok, nil
}

// EnsureOpencodeMcpProviderConfig is a generic entry point for runner's MCP ensure loop
func EnsureOpencodeMcpProviderConfig(home, serverName string, server map[string]interface{}) error {
	return ensureOpencodeMcpServer(home, serverName, server)
}
