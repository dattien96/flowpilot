package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Task-302 T-13: Opencode account metadata (zen proxy, single account)

// LoadOpencodeAccountMetadata loads metadata for an Opencode account home.
// It tries `opencode providers` and `opencode stats` to get email/plan and usage.
// Returns a runner-local summary; caller (cli) converts to accountLaunchMetadata.
// Exported so cli can reuse (CP-57 Task-302 DOD-7).
func LoadOpencodeAccountMetadata(ctx context.Context, homePath string) (OpencodeAccountSummary, error) {
	return loadOpencodeAccountMetadataInternal(ctx, homePath)
}

func loadOpencodeAccountMetadata(ctx context.Context, homePath string) (OpencodeAccountSummary, error) {
	return LoadOpencodeAccountMetadata(ctx, homePath)
}

func loadOpencodeAccountMetadataInternal(ctx context.Context, homePath string) (OpencodeAccountSummary, error) {
	summary := OpencodeAccountSummary{
		ProviderKey: "opencode",
		DisplayName: "OpenCode",
		HomePath:    homePath,
		AuthStatus:  "connected",
	}
	summary.ID = deterministicProviderAccountID("opencode", homePath)
	summary.DisplayLabel = "OpenCode"

	if out, err := runOpencodeCommand(ctx, homePath, "providers"); err == nil && len(out) > 0 {
		var providersOut map[string]any
		if jsonErr := json.Unmarshal(out, &providersOut); jsonErr == nil {
			if email, ok := providersOut["email"].(string); ok && strings.TrimSpace(email) != "" {
				summary.AccountEmail = strings.TrimSpace(email)
			}
			if plan, ok := providersOut["plan"].(string); ok {
				summary.UsageSummary = plan
			}
		} else {
			text := string(out)
			for _, line := range strings.Split(text, "\n") {
				if strings.Contains(strings.ToLower(line), "email") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						summary.AccountEmail = strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}

	if out, err := runOpencodeCommand(ctx, homePath, "stats"); err == nil && len(out) > 0 {
		var stats struct {
			Cost   float64 `json:"cost"`
			Tokens int64   `json:"tokens"`
			Usage  string  `json:"usage"`
		}
		if jsonErr := json.Unmarshal(out, &stats); jsonErr == nil {
			lines := []OpencodeAccountUsageLine{}
			if stats.Cost > 0 {
				lines = append(lines, OpencodeAccountUsageLine{Label: fmt.Sprintf("cost: $%.2f", stats.Cost), RemainingPercent: 0, ResetAt: nil})
			}
			if stats.Tokens > 0 {
				lines = append(lines, OpencodeAccountUsageLine{Label: fmt.Sprintf("tokens: %d", stats.Tokens), RemainingPercent: 0, ResetAt: nil})
			}
			summary.UsageDetailLines = lines
		} else {
			text := strings.TrimSpace(string(out))
			if text != "" {
				summary.UsageDetailLines = []OpencodeAccountUsageLine{{Label: text, RemainingPercent: 0, ResetAt: nil}}
			}
		}
	}

	summary.Remaining5hPercent = nil
	summary.Remaining7dPercent = nil
	summary.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	summary.LastAuthenticatedAt = &now

	return summary, nil
}

func runOpencodeCommand(ctx context.Context, homePath, subCommand string) ([]byte, error) {
	// Managed-home isolation: strip ambient HOME/XDG/OPENCODE_CONFIG and re-inject isolated.
	baseEnv := os.Environ()
	filtered := make([]string, 0, len(baseEnv))
	for _, kv := range baseEnv {
		if strings.HasPrefix(kv, "HOME=") || strings.HasPrefix(kv, "XDG_CONFIG_HOME=") || strings.HasPrefix(kv, "OPENCODE_CONFIG=") || strings.HasPrefix(kv, "OPENCODE_HOME=") || strings.HasPrefix(kv, "USERPROFILE=") || strings.HasPrefix(kv, "APPDATA=") || strings.HasPrefix(kv, "LOCALAPPDATA=") {
			continue
		}
		filtered = append(filtered, kv)
	}
	homePath = strings.TrimSpace(homePath)
	if homePath != "" {
		filtered = append(filtered, fmt.Sprintf("OPENCODE_HOME=%s", homePath))
		filtered = append(filtered, fmt.Sprintf("HOME=%s", homePath))
		filtered = append(filtered, fmt.Sprintf("XDG_CONFIG_HOME=%s", filepath.Join(homePath, ".config")))
		filtered = append(filtered, fmt.Sprintf("OPENCODE_CONFIG=%s", filepath.Join(homePath, ".config", "opencode")))
		if runtime.GOOS == "windows" {
			filtered = append(filtered, fmt.Sprintf("USERPROFILE=%s", homePath))
			filtered = append(filtered, fmt.Sprintf("APPDATA=%s\\AppData\\Roaming", homePath))
			filtered = append(filtered, fmt.Sprintf("LOCALAPPDATA=%s\\AppData\\Local", homePath))
			if drive, path, ok := windowsHomeDriveAndPath(homePath); ok {
				filtered = append(filtered, fmt.Sprintf("HOMEDRIVE=%s", drive))
				filtered = append(filtered, fmt.Sprintf("HOMEPATH=%s", path))
			}
		}
	}
	cmd := exec.CommandContext(ctx, opencodeBinaryName(), subCommand)
	cmd.Env = filtered
	dir := homePath
	if dir == "" {
		dir = os.TempDir()
	} else {
		// Prefer workspace dir if home is not a valid dir? Use homePath itself.
		dir = homePath
	}
	_ = os.MkdirAll(dir, 0755)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// OpencodeAccountSummary is the runner-local opencode summary (renamed to avoid
// colliding with cli.ProviderAccountSummary / tui/client.ProviderAccountSummary).
type OpencodeAccountSummary struct {
	ID                  string
	ProviderKey         string
	DisplayName         string
	DisplayLabel        string
	HomePath            string
	AccountEmail        string
	UsageSummary        string
	UsageDetailLines    []OpencodeAccountUsageLine
	Remaining5hPercent  *int
	Remaining7dPercent  *int
	AuthStatus          string
	CreatedAt           string
	LastAuthenticatedAt *string
}

type OpencodeAccountUsageLine struct {
	Label            string
	RemainingPercent int
	ResetAt          *string
}
