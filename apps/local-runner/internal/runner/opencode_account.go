package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Task-302 T-13: Opencode account metadata (zen proxy, single account)

// LoadOpencodeAccountMetadata loads metadata for an Opencode account home.
// It reads auth.json for connected providers and optionally probes
// `opencode providers list` / `opencode stats` for usage.
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

	labels, email := loadOpencodeConnectedProvidersFromAuth(homePath)
	if len(labels) == 0 {
		if out, err := runOpencodeCommand(ctx, homePath, "providers", "list"); err == nil && len(out) > 0 {
			listLabels, listEmail := parseOpencodeProvidersListOutput(out)
			if len(listLabels) > 0 {
				labels = listLabels
			}
			if listEmail != "" {
				email = listEmail
			}
		}
	}
	summary.AccountEmail = strings.TrimSpace(email)
	summary.DisplayLabel = opencodeAccountDisplayLabel(labels, email)
	summary.DisplayName = summary.DisplayLabel
	summary.UsageSummary = opencodeAccountUsageSummary(labels)
	// Skip `opencode stats` on this path: live CLI takes ~8s, the caller budget
	// is 5s, and non-JSON help text was dumped into usage lines.

	summary.Remaining5hPercent = nil
	summary.Remaining7dPercent = nil
	summary.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	summary.LastAuthenticatedAt = &now

	return summary, nil
}

// opencodeAccountCommandEnv builds the isolated environment for an `opencode`
// CLI probe under an account home: host HOME/XDG/OPENCODE_* values are stripped
// and re-derived from the home. Exported for tests via opencodeAccountCommandEnv.
func opencodeAccountCommandEnv(homePath string) []string {
	baseEnv := os.Environ()
	filtered := make([]string, 0, len(baseEnv))
	for _, kv := range baseEnv {
		if strings.HasPrefix(kv, "HOME=") || strings.HasPrefix(kv, "XDG_CONFIG_HOME=") || strings.HasPrefix(kv, "XDG_DATA_HOME=") || strings.HasPrefix(kv, "OPENCODE_CONFIG=") || strings.HasPrefix(kv, "OPENCODE_HOME=") || strings.HasPrefix(kv, "OPENCODE_AUTH_PATH=") || strings.HasPrefix(kv, "USERPROFILE=") || strings.HasPrefix(kv, "APPDATA=") || strings.HasPrefix(kv, "LOCALAPPDATA=") {
			continue
		}
		filtered = append(filtered, kv)
	}
	homePath = strings.TrimSpace(homePath)
	if homePath != "" {
		filtered = append(filtered, fmt.Sprintf("OPENCODE_HOME=%s", homePath))
		filtered = append(filtered, fmt.Sprintf("HOME=%s", homePath))
		filtered = append(filtered, fmt.Sprintf("XDG_CONFIG_HOME=%s", filepath.Join(homePath, ".config")))
		filtered = append(filtered, fmt.Sprintf("XDG_DATA_HOME=%s", opencodeDataHomeForAccount(homePath)))
		// CA-679: config FILE path — a directory value crashes opencode probes.
		filtered = append(filtered, fmt.Sprintf("OPENCODE_CONFIG=%s", opencodeConfigFilePath(homePath)))
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
	return filtered
}

func runOpencodeCommand(ctx context.Context, homePath string, args ...string) ([]byte, error) {
	filtered := opencodeAccountCommandEnv(homePath)
	homePath = strings.TrimSpace(homePath)
	cmd := newProbeCmd(ctx, opencodeBinaryName(), args...)
	cmd.Env = filtered
	dir := homePath
	if dir == "" {
		dir = os.TempDir()
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
