package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// CP-70 Task-402/403 gap closure: provider-registry integration points that
// have complete code but no direct coverage — prompt-execution args, model
// cache, account discovery, compat defaults, and the stdio MCP shim. Additive
// — codex/grok/opencode parity branches are asserted unchanged inline
// (safe-fix-contract R2).

// ── prompt execution adapter (summarizer path, Task-403) ─────────────────────

func TestDevinResolvePromptExecutionAdapterOneShot(t *testing.T) {
	req := PromptExecutionRequest{
		ProviderKey: "devin",
		ModelName:   "devin/swe-2-high",
		Prompt:      "summarize",
		YoloMode:    true,
	}
	bin, args, resolved, err := resolvePromptExecutionAdapter(req, "/tmp/out.txt", "/tmp/ws")
	if err != nil {
		t.Fatalf("resolvePromptExecutionAdapter: %v", err)
	}
	if bin != devinBinaryName() {
		t.Fatalf("binary = %q, want %q", bin, devinBinaryName())
	}
	if resolved != "devin" {
		t.Fatalf("resolved provider = %q, want devin", resolved)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--respect-workspace-trust false", "--model swe-2-high", "--permission-mode dangerous"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q must contain %q", joined, want)
		}
	}
	// -p stays last: the prompt is appended as its inline value by ExecutePrompt.
	if args[len(args)-1] != "-p" {
		t.Fatalf("args %v must end with -p (prompt is its inline value)", args)
	}
	// The devin/ prefix must be stripped before hitting the CLI.
	if strings.Contains(joined, "devin/swe-2-high") {
		t.Fatalf("args %q must carry the bare model id, not the devin/ prefix", joined)
	}
}

func TestDevinResolvePromptExecutionAdapterAllowWrite(t *testing.T) {
	req := PromptExecutionRequest{ProviderKey: "devin", ModelName: "devin/swe-2-high", Prompt: "s", AllowWrite: true}
	_, args, _, err := resolvePromptExecutionAdapter(req, "/tmp/o", "/tmp/ws")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--permission-mode accept-edits") {
		t.Fatalf("AllowWrite must map to accept-edits, got %q", joined)
	}
	if strings.Contains(joined, "dangerous") {
		t.Fatalf("AllowWrite must not escalate to dangerous, got %q", joined)
	}
}

func TestDevinResolvePromptExecutionAdapterMinimal(t *testing.T) {
	req := PromptExecutionRequest{ProviderKey: "devin", ModelName: "devin/swe-2-high", Prompt: "s"}
	_, args, resolved, err := resolvePromptExecutionAdapter(req, "/tmp/o", "/tmp/ws")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != "devin" {
		t.Fatalf("resolved = %q, want devin", resolved)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--permission-mode") {
		t.Fatalf("no write/yolo must omit --permission-mode, got %q", joined)
	}
	if !strings.Contains(joined, "--respect-workspace-trust false") {
		t.Fatalf("non-interactive devin must always skip workspace trust, got %q", joined)
	}
}

func TestDevinResolvePromptExecutionAdapterPrefixWinsAndParity(t *testing.T) {
	// devin/ model prefix routes to devin even when another provider is supplied
	// (BUG-171 model-wins routing; mandatory because bare devin aliases like
	// opus/codex/gpt collide with other providers' namespaces).
	req := PromptExecutionRequest{ProviderKey: "claude", ModelName: "devin/swe-2-high", Prompt: "s"}
	_, _, resolved, err := resolvePromptExecutionAdapter(req, "/tmp/o", "/tmp/ws")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != "devin" {
		t.Fatalf("model prefix must win over supplied provider, got %q", resolved)
	}

	// Parity: codex/opencode branches keep their prior shape (first args element).
	codexReq := PromptExecutionRequest{ProviderKey: "codex", ModelName: "gpt-5.4", Prompt: "s"}
	codexBin, codexArgs, codexResolved, err := resolvePromptExecutionAdapter(codexReq, "/tmp/o", "/tmp/ws")
	if err != nil || codexBin != "codex" || codexResolved != "codex" {
		t.Fatalf("codex branch drift: bin=%q resolved=%q err=%v", codexBin, codexResolved, err)
	}
	if !reflect.DeepEqual(codexArgs[:2], []string{"--sandbox", "read-only"}) {
		t.Fatalf("codex branch shape drift: %v", codexArgs)
	}
	opReq := PromptExecutionRequest{ProviderKey: "opencode", ModelName: "opencode/muse-spark-1.2-contributor-free", Prompt: "s"}
	opBin, _, opResolved, err := resolvePromptExecutionAdapter(opReq, "/tmp/o", "/tmp/ws")
	if err != nil || opBin != opencodeBinaryName() || opResolved != "opencode" {
		t.Fatalf("opencode branch drift: bin=%q resolved=%q err=%v", opBin, opResolved, err)
	}
}

func TestDevinSummarizerModelUsesAccountDefault(t *testing.T) {
	// Effort lives in the model id; empty lets `devin -p` use the account default
	// instead of guessing an unverified cheap tier.
	if got := summarizerModelFor(ProviderKeyDevin); got != "" {
		t.Fatalf("summarizerModelFor(devin) = %q, want empty (account default)", got)
	}
	// Parity: opencode keeps its pinned cheap tier.
	if got := summarizerModelFor(ProviderKeyOpencode); got != "opencode/gpt-5.4-nano" {
		t.Fatalf("summarizerModelFor(opencode) = %q, want opencode/gpt-5.4-nano", got)
	}
}

// ── reasoning effort mapping (Task-403) ──────────────────────────────────────

func TestDevinCanonicalEffort(t *testing.T) {
	cases := map[string]string{
		"low": "low", "medium": "medium", "med": "medium", "high": "high",
		"xhigh": "xhigh", "x-high": "xhigh", "extra_high": "xhigh", "max": "max",
	}
	for in, want := range cases {
		got, ok := devinCanonicalEffort(in)
		if !ok || got != want {
			t.Fatalf("devinCanonicalEffort(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "turbo", "ULTRA"} {
		if got, ok := devinCanonicalEffort(in); ok || got != "" {
			t.Fatalf("devinCanonicalEffort(%q) = %q,%v want \"\",false", in, got, ok)
		}
	}
}

func TestDevinSplitModelEffort(t *testing.T) {
	cases := []struct{ model, family, effort string }{
		{"swe-2-high", "swe-2", "high"},
		{"claude-opus-5-xhigh", "claude-opus-5", "xhigh"},
		{"gpt-5-6-sol-max-fast", "gpt-5-6-sol-max", "fast"}, // longest-suffix-first keeps -max in family
		{"adaptive", "adaptive", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		fam, eff := devinSplitModelEffort(tc.model)
		if fam != tc.family || eff != tc.effort {
			t.Fatalf("devinSplitModelEffort(%q) = (%q,%q), want (%q,%q)", tc.model, fam, eff, tc.family, tc.effort)
		}
	}
}

// ── model catalog cache (Task-402) ───────────────────────────────────────────

func TestDevinModelsCacheRoundTripAndTTL(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "devin_models_cache.json")
	t.Setenv("FLOWPILOT_DEVIN_MODELS_CACHE_PATH", cachePath)

	models := []ProviderModel{{ID: "swe-2-high", DisplayName: "SWE-2 High", Available: true, Source: "devin-acp"}}
	writeDevinModelsCache(models)
	got := readDevinModelsCache()
	if len(got) != 1 || got[0].ID != "swe-2-high" {
		t.Fatalf("cache roundtrip = %+v, want the written model", got)
	}

	// Expired entries are invisible to the strict reader but visible stale.
	var payload devinModelsCacheFile
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal cache: %v", err)
	}
	payload.FetchedAt = time.Now().UTC().Add(-2 * devinModelsCacheTTL).Format(time.RFC3339Nano)
	staleData, _ := json.Marshal(payload)
	if err := os.WriteFile(cachePath, staleData, 0o644); err != nil {
		t.Fatalf("write stale cache: %v", err)
	}
	if got := readDevinModelsCache(); got != nil {
		t.Fatalf("expired cache must read empty, got %+v", got)
	}
	if got := readDevinModelsCacheStale(true); len(got) != 1 {
		t.Fatalf("stale-tolerant read must return the entry, got %+v", got)
	}
}

func TestDevinModelsCacheEmptyWriteIsNoop(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "devin_models_cache.json")
	t.Setenv("FLOWPILOT_DEVIN_MODELS_CACHE_PATH", cachePath)
	writeDevinModelsCache(nil)
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("empty write must not create the cache file")
	}
}

func TestRecordDevinModelCatalog(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "devin_models_cache.json")
	t.Setenv("FLOWPILOT_DEVIN_MODELS_CACHE_PATH", cachePath)

	recordDevinModelCatalog([]DevinConfigChoice{
		{Value: "swe-2-high", Name: "SWE-2 High", Meta: map[string]any{"cognition.ai/supportsImages": true}},
		{Value: "", Name: "skipped-empty-id"},
		{Value: "adaptive"},
	})
	got := readDevinModelsCache()
	if len(got) != 2 {
		t.Fatalf("catalog must skip empty ids, got %+v", got)
	}
	if got[0].ID != "swe-2-high" || !got[0].InputImage || got[0].Source != "devin-acp" {
		t.Fatalf("catalog entry lost metadata: %+v", got[0])
	}
	if got[1].ID != "adaptive" || got[1].DisplayName != "adaptive" {
		t.Fatalf("nameless entry must fall back to the id: %+v", got[1])
	}
}

func TestDevinPrefixedProviderModels(t *testing.T) {
	out := devinPrefixedProviderModels([]ProviderModel{
		{ID: "swe-2-high", DisplayName: "SWE-2 High"},
		{ID: "devin/already-prefixed"},
		{ID: ""},
	})
	if len(out) != 2 {
		t.Fatalf("empty ids must be dropped, got %+v", out)
	}
	if out[0].ID != "devin/swe-2-high" || out[0].DisplayName != "SWE-2 High" {
		t.Fatalf("bare id must gain the devin/ prefix: %+v", out[0])
	}
	if out[1].ID != "devin/already-prefixed" || out[1].DisplayName != "devin/already-prefixed" {
		t.Fatalf("prefixed id must pass through; display falls back to id: %+v", out[1])
	}
}

// ── account discovery (Task-402) ─────────────────────────────────────────────

func TestIsValidDevinAccountPath(t *testing.T) {
	homeDir := t.TempDir()
	if isValidDevinAccountPath(homeDir) {
		t.Fatalf("empty home must be invalid")
	}
	mustWriteTestFile(t, filepath.Join(homeDir, ".local", "share", "devin", "credentials.toml"), "[auth]")
	if !isValidDevinAccountPath(homeDir) {
		t.Fatalf("credentials.toml under XDG data dir must validate the home")
	}

	configOnly := t.TempDir()
	mustWriteTestFile(t, filepath.Join(configOnly, ".config", "devin", "mcp_config.json"), "{}")
	if !isValidDevinAccountPath(configOnly) {
		t.Fatalf("mcp_config.json under XDG config dir must validate the home")
	}
	if isValidDevinAccountPath(filepath.Join(homeDir, "does-not-exist")) {
		t.Fatalf("nonexistent path must be invalid")
	}
}

func TestIsValidDevinAccountPath_WindowsRoaming(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only credential layout")
	}
	homeDir := t.TempDir()
	mustWriteTestFile(t, filepath.Join(homeDir, "AppData", "Roaming", "devin", "credentials.toml"), "[auth]\nwindsurf_api_key = \"k\"")
	if !isValidDevinAccountPath(homeDir) {
		t.Fatalf("credentials.toml under AppData\\Roaming\\devin must validate the home on windows")
	}
}

func TestDiscoverDevinAccountHomes_DefaultAndManaged(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(homeDir, ".local", "share"))

	mustWriteTestFile(t, filepath.Join(homeDir, ".local", "share", "devin", "credentials.toml"), "[auth]")
	slot := filepath.Join(homeDir, ".devinHome2")
	mustWriteTestFile(t, filepath.Join(slot, ".config", "devin", "config.json"), "{}")

	paths, err := DiscoverProviderAccountHomes("devin")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes: %v", err)
	}
	found := map[string]bool{}
	for _, p := range paths {
		found[p] = true
	}
	if !found[filepath.Clean(homeDir)] && !found[homeDir] {
		t.Fatalf("default home not discovered: %v", paths)
	}
	if !found[slot] {
		t.Fatalf("managed .devinHome2 slot not discovered: %v", paths)
	}
}

// ── compat defaults (Task-402) ───────────────────────────────────────────────

func TestCompatConfigSeedsDevinVersion(t *testing.T) {
	cfg := defaultCompatConfig()
	if cfg.TestedDevinVersion != CompatTestedDevinVersion {
		t.Fatalf("defaultCompatConfig.TestedDevinVersion = %q, want %q", cfg.TestedDevinVersion, CompatTestedDevinVersion)
	}
	// An empty stored value normalizes back to the seeded constant.
	got := normalizeCompatConfig(CompatConfig{})
	if got.TestedDevinVersion != CompatTestedDevinVersion {
		t.Fatalf("normalizeCompatConfig must seed TestedDevinVersion, got %q", got.TestedDevinVersion)
	}
	// Parity: the other provider seeds stay intact.
	if got.TestedOpencodeVersion != CompatTestedOpencodeVersion || got.TestedGrokVersion != CompatTestedGrokVersion {
		t.Fatalf("opencode/grok seeds drifted: %+v", got)
	}
}

// ── stdio MCP shim (Task-403) ────────────────────────────────────────────────

func TestDevinMCPShimCommandOverride(t *testing.T) {
	t.Setenv("FLOWPILOT_DEVIN_MCP_SHIM", "/bin/echo devin-mcp-stdio --flag")
	cmd, args := devinMCPShimCommand()
	if cmd != "/bin/echo" || !reflect.DeepEqual(args, []string{"devin-mcp-stdio", "--flag"}) {
		t.Fatalf("override shim = %q %v", cmd, args)
	}
}

func TestRunDevinMCPStdioRequiresURL(t *testing.T) {
	if err := RunDevinMCPStdio(context.Background(), "  ", strings.NewReader(""), &strings.Builder{}); err == nil {
		t.Fatalf("empty --url must fail")
	}
}

func TestRunDevinMCPStdioForwardsRequestAndNotification(t *testing.T) {
	var gotBodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBodies = append(gotBodies, string(raw))
		var msg map[string]any
		_ = json.Unmarshal(raw, &msg)
		if _, hasID := msg["id"]; !hasID {
			w.WriteHeader(http.StatusAccepted) // notification → no stdout line
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":7,"result":{"tools":[]}}`))
	}))
	defer srv.Close()

	stdin := strings.NewReader(
		`{"jsonrpc":"2.0","id":7,"method":"tools/list","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
	var stdout strings.Builder
	if err := RunDevinMCPStdio(context.Background(), srv.URL+"?token=turn-tok", stdin, &stdout); err != nil {
		t.Fatalf("RunDevinMCPStdio: %v", err)
	}
	if len(gotBodies) != 2 {
		t.Fatalf("expected 2 forwarded lines, got %d", len(gotBodies))
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("notification must not produce a stdout line, got %q", stdout.String())
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("stdout line is not JSON: %v", err)
	}
	if resp["id"].(float64) != 7 {
		t.Fatalf("response id = %v, want 7", resp["id"])
	}
}

func TestRunDevinMCPStdioParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("malformed line must not reach the endpoint")
	}))
	defer srv.Close()

	var stdout strings.Builder
	if err := RunDevinMCPStdio(context.Background(), srv.URL, strings.NewReader("{not json\n"), &stdout); err != nil {
		t.Fatalf("RunDevinMCPStdio: %v", err)
	}
	if !strings.Contains(stdout.String(), "-32700") {
		t.Fatalf("parse error must reply -32700, got %q", stdout.String())
	}
}

func TestRunDevinMCPStdioEndpointErrorRepliesJSONRPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	var stdout strings.Builder
	in := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{}}` + "\n"
	if err := RunDevinMCPStdio(context.Background(), srv.URL, strings.NewReader(in), &stdout); err != nil {
		t.Fatalf("RunDevinMCPStdio: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "-32000") || !strings.Contains(out, `"id":3`) {
		t.Fatalf("endpoint failure must resolve the pending call with -32000, got %q", out)
	}
}
