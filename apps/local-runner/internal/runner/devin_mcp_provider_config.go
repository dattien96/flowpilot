package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CP-70 Task-402: Devin MCP provider config (Google Drive / Jira one-click).
// Devin stores MCP servers in a dedicated `mcp_config.json` (v3000.3+, F-11):
// ~/.config/devin/mcp_config.json on Unix, %APPDATA%\devin\mcp_config.json on
// Windows. Managed homes (.devinHomeN) replicate the XDG subtree because the
// spawned process gets HOME + XDG_CONFIG_HOME pointing inside the slot.

func getDevinMcpConfigPath(home string) string {
	clean := filepath.ToSlash(filepath.Clean(home))
	if strings.HasSuffix(clean, ".config/devin") || strings.HasSuffix(clean, "/devin") {
		return filepath.Join(home, "mcp_config.json")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Roaming", "devin", "mcp_config.json")
	}
	return filepath.Join(home, ".config", "devin", "mcp_config.json")
}

func readDevinMcpConfig(path string) (map[string]interface{}, error) {
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
		return nil, fmt.Errorf("devin mcp config is not valid JSON: %w", err)
	}
	if _, ok := doc["mcpServers"]; !ok {
		doc["mcpServers"] = map[string]interface{}{}
	}
	return doc, nil
}

func writeDevinMcpConfigAtomic(path string, doc map[string]interface{}) error {
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

// ensureDevinMcpServer upserts one mcpServers entry in Devin's mcp_config.json
// and reports whether the file actually changed (deep-compare via JSON).
func ensureDevinMcpServer(home, name string, server map[string]interface{}) (bool, error) {
	path := getDevinMcpConfigPath(home)
	doc, err := readDevinMcpConfig(path)
	if err != nil {
		return false, err
	}
	mcpServers, _ := doc["mcpServers"].(map[string]interface{})
	if mcpServers == nil {
		mcpServers = map[string]interface{}{}
		doc["mcpServers"] = mcpServers
	}
	existing, exists := mcpServers[name]
	if exists {
		a, _ := json.Marshal(existing)
		b, _ := json.Marshal(server)
		if string(a) == string(b) {
			return false, nil
		}
	}
	mcpServers[name] = server
	if err := writeDevinMcpConfigAtomic(path, doc); err != nil {
		return false, err
	}
	return true, nil
}

// CheckDevinMcpConfig reports whether mcp_config.json already carries the named server.
func CheckDevinMcpConfig(home, name string) (bool, error) {
	path := getDevinMcpConfigPath(home)
	doc, err := readDevinMcpConfig(path)
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
