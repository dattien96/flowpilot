package runner

import "testing"

func TestLooksLikeCLIProbeError_NodeModuleDump(t *testing.T) {
	dump := "node:internal/modules/cjs/loader:1146\n  throw err;\nError: Cannot find module 'C:\\Users\\me\\AppData\\Roaming\\npm\\node_modules\\@openai\\codex\\bin.js'"
	if !looksLikeCLIProbeError(dump) {
		t.Fatal("expected node module dump to be treated as probe error")
	}
	if !looksLikeCLIProbeError("Error: Cannot find module '@openai/codex'") {
		t.Fatal("expected short cannot-find-module line")
	}
	if looksLikeCLIProbeError("codex 0.140.0") {
		t.Fatal("real version must not look like a probe error")
	}
	if looksLikeCLIProbeError("") {
		t.Fatal("empty is not a probe error")
	}
}
