package lsp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func lspAndroidWorkspace(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestKotlinInitOptionsIncludesGradleSettings(t *testing.T) {
	ws := lspAndroidWorkspace(t, map[string]string{
		"build.gradle.kts":  "plugins { id(\"com.android.application\") }\n",
		"gradle.properties": "org.gradle.jvmargs=-Xmx4g\nandroid.useAndroidX=true\n",
		"local.properties":  "sdk.dir=/opt/android-sdk\n",
	})
	opts, err := BuildKotlinInitOptions(ws)
	if err != nil {
		t.Fatalf("BuildKotlinInitOptions: %v", err)
	}
	if opts["androidSdkPath"] != "/opt/android-sdk" {
		t.Fatalf("androidSdkPath = %v", opts["androidSdkPath"])
	}
	props, ok := opts["gradleProperties"].(map[string]any)
	if !ok {
		t.Fatalf("gradleProperties missing: %+v", opts)
	}
	if props["android.useAndroidX"] != "true" || props["org.gradle.jvmargs"] != "-Xmx4g" {
		t.Fatalf("gradleProperties = %+v", props)
	}
	if _, ok := opts["storagePath"]; !ok {
		t.Fatalf("storagePath missing: %+v", opts)
	}
	// Full config carries the options.
	cfg, err := KotlinServerConfig(ws)
	if err != nil {
		t.Fatalf("KotlinServerConfig: %v", err)
	}
	if cfg.Binary != "kotlin-language-server" {
		t.Fatalf("binary = %q", cfg.Binary)
	}
	if cfg.InitializationOptions == nil {
		t.Fatal("android config must carry initialization options")
	}
}

func TestDetectAndroidSDKFromLocalProperties(t *testing.T) {
	ws := lspAndroidWorkspace(t, map[string]string{
		"local.properties": "# comment\nsdk.dir=/opt/android-sdk\n",
	})
	// Env must not shadow the file.
	t.Setenv("ANDROID_HOME", "/env/sdk")
	t.Setenv("ANDROID_SDK_ROOT", "")
	got, err := DetectAndroidSDK(ws)
	if err != nil {
		t.Fatalf("DetectAndroidSDK: %v", err)
	}
	if got != "/opt/android-sdk" {
		t.Fatalf("sdk = %q", got)
	}

	// Windows-escaped form.
	ws2 := lspAndroidWorkspace(t, map[string]string{
		"local.properties": "sdk.dir=C\\:\\\\Android\\\\Sdk\n",
	})
	got, err = DetectAndroidSDK(ws2)
	if err != nil {
		t.Fatalf("DetectAndroidSDK escaped: %v", err)
	}
	if got != `C:\Android\Sdk` {
		t.Fatalf("sdk = %q", got)
	}
}

func TestDetectAndroidSDKFromEnvVar(t *testing.T) {
	ws := t.TempDir() // no local.properties
	t.Setenv("ANDROID_HOME", "/env/android-sdk")
	t.Setenv("ANDROID_SDK_ROOT", "")
	got, err := DetectAndroidSDK(ws)
	if err != nil {
		t.Fatalf("DetectAndroidSDK: %v", err)
	}
	if got != "/env/android-sdk" {
		t.Fatalf("sdk = %q", got)
	}
}

func TestDetectAndroidSDKReturnsErrorWhenMissing(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")
	if _, err := DetectAndroidSDK(ws); err == nil {
		t.Fatal("expected error when no SDK source exists")
	}
}

func TestKotlinPlatformConfigInRegistry(t *testing.T) {
	cfg, ok := DefaultRegistry().Lookup("android")
	if !ok {
		t.Fatal("android missing from registry")
	}
	if cfg.Binary != "kotlin-language-server" {
		t.Fatalf("binary = %q", cfg.Binary)
	}
	foundStdio := false
	for _, a := range cfg.Args {
		if a == "--stdio" {
			foundStdio = true
		}
	}
	if !foundStdio {
		t.Fatalf("args = %v, want --stdio", cfg.Args)
	}
}

func TestKotlinFileExtensionsCorrect(t *testing.T) {
	reg := DefaultRegistry()
	for _, f := range []string{"Main.kt", "build.kts"} {
		cfg, ok := reg.ConfigForFile("android", f)
		if !ok || cfg.Platform != "android" {
			t.Fatalf("%s not owned by android", f)
		}
	}
	if _, ok := reg.ConfigForFile("android", "Main.java"); ok {
		t.Fatal("android entry must not claim .java (registry scope is .kt/.kts)")
	}
	if got := LanguageID("Main.kt"); got != "kotlin" {
		t.Fatalf("LanguageID = %q", got)
	}
}

func TestKotlinInitOptionsReachHandshake(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "init-params.json")
	dir := t.TempDir()
	m := &ServerManager{
		Env:         []string{"GO_WANT_LSP_HELPER=1", "GO_LSP_HELPER_MODE=lspserver", "GO_LSP_HELPER_CAPTURE=" + capture},
		InitOptions: map[string]any{"androidSdkPath": "/sdk", "customFlag": true},
	}
	t.Cleanup(func() { _ = m.Stop() })
	if err := m.Start(context.Background(), os.Args[0], []string{"-test.run=TestLSPHelperProcess"}, dir); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.WaitReady(ctx, 0); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read captured params: %v", err)
	}
	var params InitializeParams
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	opts, ok := params.InitializationOptions.(map[string]any)
	if !ok {
		t.Fatalf("initializationOptions = %#v", params.InitializationOptions)
	}
	if opts["androidSdkPath"] != "/sdk" || opts["customFlag"] != true {
		t.Fatalf("initializationOptions = %+v", opts)
	}
}

func TestKotlinInitOptionsForHookShape(t *testing.T) {
	ws := lspAndroidWorkspace(t, map[string]string{"build.gradle": ""})
	if got := KotlinInitOptionsFor("golang", ws); got != nil {
		t.Fatalf("non-android hook = %+v, want nil", got)
	}
	got := KotlinInitOptionsFor("android", ws)
	if got == nil {
		t.Fatal("android hook must return options")
	}
	if _, ok := got["storagePath"]; !ok {
		t.Fatalf("options = %+v", got)
	}
	// Compose heuristic: no compose mention -> flag absent.
	if _, ok := got["enableComposeCompilerPlugin"]; ok {
		t.Fatalf("compose flag must be absent without compose usage: %+v", got)
	}
	wsCompose := lspAndroidWorkspace(t, map[string]string{
		"build.gradle.kts": "implementation(libs.compose.ui)\n",
	})
	got = KotlinInitOptionsFor("android", wsCompose)
	if got["enableComposeCompilerPlugin"] != true {
		t.Fatalf("compose flag missing with compose usage: %+v", got)
	}
}
