package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// KotlinInitializationOptions carries Gradle/Android context to
// kotlin-language-server via initialize.initializationOptions. Unknown keys
// are ignored by servers, so extra hints are safe.
type KotlinInitializationOptions struct {
	// StoragePath is the server's cache dir, scoped per workspace.
	StoragePath string `json:"storagePath,omitempty"`
	// AndroidSdkPath comes from local.properties sdk.dir or ANDROID_HOME.
	AndroidSdkPath string `json:"androidSdkPath,omitempty"`
	// GradleUserHome comes from GRADLE_USER_HOME or ~/.gradle.
	GradleUserHome string `json:"gradleUserHome,omitempty"`
	// GradleProperties mirrors gradle.properties key/values.
	GradleProperties map[string]string `json:"gradleProperties,omitempty"`
	// EnableComposeCompilerPlugin is a heuristic hint (see BuildKotlinInitOptions).
	EnableComposeCompilerPlugin bool `json:"enableComposeCompilerPlugin,omitempty"`
}

// ToMap renders the options for InitializeParams.InitializationOptions.
func (o KotlinInitializationOptions) ToMap() map[string]any {
	m := map[string]any{}
	if o.StoragePath != "" {
		m["storagePath"] = o.StoragePath
	}
	if o.AndroidSdkPath != "" {
		m["androidSdkPath"] = o.AndroidSdkPath
	}
	if o.GradleUserHome != "" {
		m["gradleUserHome"] = o.GradleUserHome
	}
	if len(o.GradleProperties) > 0 {
		props := make(map[string]any, len(o.GradleProperties))
		for k, v := range o.GradleProperties {
			props[k] = v
		}
		m["gradleProperties"] = props
	}
	if o.EnableComposeCompilerPlugin {
		m["enableComposeCompilerPlugin"] = true
	}
	return m
}

// DetectAndroidSDK resolves the Android SDK path: local.properties sdk.dir
// first, then ANDROID_HOME, then ANDROID_SDK_ROOT.
func DetectAndroidSDK(workspaceRoot string) (string, error) {
	if data, err := os.ReadFile(filepath.Join(workspaceRoot, "local.properties")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
				continue
			}
			key, value, found := strings.Cut(line, "=")
			if !found {
				continue
			}
			if strings.TrimSpace(key) == "sdk.dir" {
				if v := unescapePropertiesValue(strings.TrimSpace(value)); v != "" {
					return v, nil
				}
			}
		}
	}
	if v := strings.TrimSpace(os.Getenv("ANDROID_HOME")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv("ANDROID_SDK_ROOT")); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("lsp: android SDK not found (local.properties sdk.dir or ANDROID_HOME/ANDROID_SDK_ROOT)")
}

// unescapePropertiesValue decodes Java-properties escapes relevant to
// Windows SDK paths (sdk.dir=C\:\\Android\\Sdk).
func unescapePropertiesValue(s string) string {
	s = strings.ReplaceAll(s, `\:`, ":")
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

// readGradleProperties parses gradle.properties into key/values.
// A missing file yields an empty map, never an error.
func readGradleProperties(workspaceRoot string) map[string]string {
	out := make(map[string]string)
	data, err := os.ReadFile(filepath.Join(workspaceRoot, "gradle.properties"))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return out
}

// usesCompose heuristically detects Jetpack Compose usage for the compiler
// plugin hint: any mention of "compose" in Gradle descriptors.
func usesCompose(workspaceRoot string) bool {
	files := []string{"gradle.properties", "settings.gradle", "settings.gradle.kts", "build.gradle", "build.gradle.kts", "gradle/libs.versions.toml"}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(workspaceRoot, f))
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(string(data)), "compose") {
			return true
		}
	}
	return false
}

// defaultGradleUserHome resolves GRADLE_USER_HOME or ~/.gradle.
func defaultGradleUserHome() string {
	if v := strings.TrimSpace(os.Getenv("GRADLE_USER_HOME")); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".gradle")
	}
	return ""
}

// BuildKotlinInitOptions assembles per-workspace initialization options. A
// missing Android SDK is NOT an error — the key is simply omitted and the
// server proceeds without it (graceful degradation).
func BuildKotlinInitOptions(workspaceRoot string) (map[string]any, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil, fmt.Errorf("lsp: empty workspace root")
	}
	opts := KotlinInitializationOptions{
		StoragePath:      filepath.Join(workspaceRoot, ".flowpilot", "lsp", "kotlin"),
		GradleUserHome:   defaultGradleUserHome(),
		GradleProperties: readGradleProperties(workspaceRoot),
	}
	if sdk, err := DetectAndroidSDK(workspaceRoot); err == nil {
		opts.AndroidSdkPath = sdk
	}
	opts.EnableComposeCompilerPlugin = usesCompose(workspaceRoot)
	return opts.ToMap(), nil
}

// KotlinInitOptionsFor adapts BuildKotlinInitOptions to the ServerSet
// InitOptionsFor hook shape (per-workspace options for one platform).
func KotlinInitOptionsFor(platform, workspaceRoot string) map[string]any {
	if platform != "android" {
		return nil
	}
	m, err := BuildKotlinInitOptions(workspaceRoot)
	if err != nil {
		return nil
	}
	return m
}

// KotlinServerConfig returns the android registry entry enriched with
// per-workspace initialization options.
func KotlinServerConfig(workspaceRoot string) (PlatformLSPConfig, error) {
	cfg, ok := DefaultRegistry().Lookup("android")
	if !ok {
		return PlatformLSPConfig{}, fmt.Errorf("lsp: android platform missing from registry")
	}
	opts, err := BuildKotlinInitOptions(workspaceRoot)
	if err != nil {
		return PlatformLSPConfig{}, err
	}
	cfg.InitializationOptions = opts
	return cfg, nil
}
