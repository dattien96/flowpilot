package agentpack

import (
	"strings"
	"testing"
)

// CP-67 P-1 (Task-378): the scaffold + coder declared faces must load from the
// builtin pack with the exact schema contract the hub and the gate consume.
// New file — additive only; no pre-existing pack test is edited.

// findToolFace looks a declared face up in the loaded pack.
func findToolFace(t *testing.T, pack Pack, id string) ToolFace {
	t.Helper()
	for _, f := range pack.Tools {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("pack has no tool face %q", id)
	return ToolFace{}
}

// inputEnum reads the enum values of an input schema property.
func inputEnum(t *testing.T, face ToolFace, prop string) []string {
	t.Helper()
	raw, ok := face.Input["properties"]
	if !ok {
		t.Fatalf("face %q input has no properties", face.ID)
	}
	props, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("face %q input.properties is not a map", face.ID)
	}
	propRaw, ok := props[prop]
	if !ok {
		t.Fatalf("face %q input.properties missing %q", face.ID, prop)
	}
	propMap, ok := propRaw.(map[string]any)
	if !ok {
		t.Fatalf("face %q property %q is not a map", face.ID, prop)
	}
	enumRaw, ok := propMap["enum"]
	if !ok {
		t.Fatalf("face %q property %q has no enum", face.ID, prop)
	}
	enumSlice, ok := enumRaw.([]any)
	if !ok {
		t.Fatalf("face %q property %q enum is not a list", face.ID, prop)
	}
	out := make([]string, 0, len(enumSlice))
	for _, v := range enumSlice {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("face %q property %q enum value not a string: %v", face.ID, prop, v)
		}
		out = append(out, s)
	}
	return out
}

func TestSubmitScaffoldOutcomeSchemaValidation(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	face := findToolFace(t, pack, "submit_scaffold_outcome")
	if !face.ExposesTool {
		t.Fatal("submit_scaffold_outcome must expose its tool")
	}
	if face.MapsTo != "flow.control" {
		t.Fatalf("mapsTo = %q, want flow.control", face.MapsTo)
	}
	// status enum: exactly scaffold_ready|blocked (T-2).
	statuses := inputEnum(t, face, "status")
	if len(statuses) != 2 || statuses[0] != "scaffold_ready" || statuses[1] != "blocked" {
		t.Fatalf("status enum = %v, want [scaffold_ready blocked]", statuses)
	}
	// statusMap: scaffold_ready -> done, blocked -> escalate (T-2).
	if face.StatusMap["scaffold_ready"] != "done" || face.StatusMap["blocked"] != "escalate" {
		t.Fatalf("statusMap = %v, want scaffold_ready:done blocked:escalate", face.StatusMap)
	}
	// stubs[].symbols[].kind enum has exactly the 5 declaration kinds (T-4).
	kindEnum := nestedEnum(t, face, []string{"stubs", "symbols"}, "kind")
	want := []string{"function", "method", "interface", "struct", "class"}
	if len(kindEnum) != len(want) {
		t.Fatalf("symbols.kind enum = %v, want %v", kindEnum, want)
	}
	for i, w := range want {
		if kindEnum[i] != w {
			t.Fatalf("symbols.kind enum = %v, want %v", kindEnum, want)
		}
	}
	// test_suite.failure_type enum: not_implemented | assertion_failure (T-4).
	ftEnum := nestedEnum(t, face, []string{"test_suite"}, "failure_type")
	if len(ftEnum) != 2 || ftEnum[0] != "not_implemented" || ftEnum[1] != "assertion_failure" {
		t.Fatalf("failure_type enum = %v, want [not_implemented assertion_failure]", ftEnum)
	}
	// payloadMap forwards stubs + test_suite verbatim for hub reading.
	if face.PayloadMap["stubs"] != "payload.stubs" || face.PayloadMap["test_suite"] != "payload.test_suite" {
		t.Fatalf("payloadMap = %v, want stubs/test_suite forwarded to payload.*", face.PayloadMap)
	}
}

func TestSubmitCoderOutcomeBatchSignatureValidation(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	face := findToolFace(t, pack, "submit_coder_outcome")
	if !face.ExposesTool {
		t.Fatal("submit_coder_outcome must expose its tool")
	}
	if face.MapsTo != "flow.control" {
		t.Fatalf("mapsTo = %q, want flow.control", face.MapsTo)
	}
	// status enum: completed | renegotiate_signatures | blocked (T-3) — the
	// canonical renegotiation term is renegotiate_signatures (T-5).
	statuses := inputEnum(t, face, "status")
	if len(statuses) != 3 {
		t.Fatalf("status enum = %v, want 3 values", statuses)
	}
	want := map[string]bool{"completed": true, "renegotiate_signatures": true, "blocked": true}
	for _, s := range statuses {
		if !want[s] {
			t.Fatalf("unexpected status %q in enum %v", s, statuses)
		}
	}
	// statusMap: completed -> done, renegotiate_signatures -> continue, blocked -> escalate.
	if face.StatusMap["completed"] != "done" ||
		face.StatusMap["renegotiate_signatures"] != "continue" ||
		face.StatusMap["blocked"] != "escalate" {
		t.Fatalf("statusMap = %v, want completed:done renegotiate_signatures:continue blocked:escalate", face.StatusMap)
	}
	// batch_signature_requests rows require all five fields including rationale.
	rowProps := nestedProperties(t, face, []string{"batch_signature_requests"})
	for _, required := range []string{"symbol", "file", "current_signature", "proposed_signature", "rationale"} {
		if _, ok := rowProps[required]; !ok {
			t.Fatalf("batch_signature_requests properties missing %q", required)
		}
	}
	// payloadMap forwards batch + summary + progress into payload.*.
	if face.PayloadMap["batch_signature_requests"] != "payload.batch_signature_requests" ||
		face.PayloadMap["summary"] != "payload.summary" ||
		face.PayloadMap["implementation_progress"] != "payload.implementation_progress" {
		t.Fatalf("payloadMap = %v, want batch/summary/progress forwarded to payload.*", face.PayloadMap)
	}
}

// nestedEnum digs a <prop>.enum out of array-item or object properties.
func nestedEnum(t *testing.T, face ToolFace, path []string, prop string) []string {
	t.Helper()
	props := nestedProperties(t, face, path)
	propRaw, ok := props[prop]
	if !ok {
		t.Fatalf("face %q %v missing %q", face.ID, path, prop)
	}
	propMap, ok := propRaw.(map[string]any)
	if !ok {
		t.Fatalf("face %q %v.%s is not a map", face.ID, path, prop)
	}
	enumRaw, ok := propMap["enum"]
	if !ok {
		t.Fatalf("face %q %v.%s has no enum", face.ID, path, prop)
	}
	enumSlice, ok := enumRaw.([]any)
	if !ok {
		t.Fatalf("face %q %v.%s enum is not a list", face.ID, path, prop)
	}
	out := make([]string, 0, len(enumSlice))
	for _, v := range enumSlice {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out
}

// nestedProperties walks face.Input through a path of property names, diving
// into array item schemas automatically.
func nestedProperties(t *testing.T, face ToolFace, path []string) map[string]any {
	t.Helper()
	current := face.Input
	for _, seg := range path {
		propsRaw, ok := current["properties"]
		if !ok {
			t.Fatalf("face %q schema at %v has no properties", face.ID, path)
		}
		props, ok := propsRaw.(map[string]any)
		if !ok {
			t.Fatalf("face %q schema properties at %v not a map", face.ID, path)
		}
		segRaw, ok := props[seg]
		if !ok {
			t.Fatalf("face %q schema missing property %q (path %v)", face.ID, seg, path)
		}
		segMap, ok := segRaw.(map[string]any)
		if !ok {
			t.Fatalf("face %q property %q not a map", face.ID, seg)
		}
		// Arrays: dive into items; objects: dive into properties directly.
		if itemsRaw, ok := segMap["items"]; ok {
			items, ok := itemsRaw.(map[string]any)
			if !ok {
				t.Fatalf("face %q %q items not a map", face.ID, seg)
			}
			current = items
			continue
		}
		if propsRaw, ok := segMap["properties"]; ok {
			props, ok := propsRaw.(map[string]any)
			if !ok {
				t.Fatalf("face %q %q properties not a map", face.ID, seg)
			}
			current = segMap
			_ = props
			// Re-enter as the object itself on the next loop via properties.
			current = map[string]any{"properties": props}
			continue
		}
		t.Fatalf("face %q property %q is neither array nor object", face.ID, seg)
	}
	propsRaw, ok := current["properties"]
	if !ok {
		t.Fatalf("face %q schema at %v has no properties", face.ID, path)
	}
	props, ok := propsRaw.(map[string]any)
	if !ok {
		t.Fatalf("face %q schema properties at %v not a map", face.ID, path)
	}
	return props
}

func TestScaffoldOutcomeToolAdvertisedOnScaffoldNode(t *testing.T) {
	def := taskHarnessDefinition(t)
	node := findNodeByID(t, def, "test_signatures")
	// CP-67 P-5: the scaffold node declares the face in its flow's tools and
	// its prompt template carries the typed handover instructions.
	face, ok, err := LoadBuiltinToolFace("submit_scaffold_outcome")
	if err != nil || !ok {
		t.Fatalf("LoadBuiltinToolFace(submit_scaffold_outcome): ok=%v err=%v", ok, err)
	}
	if !declaredInFlowTools(def, face.ID) {
		t.Fatalf("task-harness tools list does not declare %q", face.ID)
	}
	prompt := loadPackPrompt(t, node.PromptTemplate)
	for _, want := range []string{"submit_scaffold_outcome", "scaffold_ready", "stubs", "test_suite", "red_tests"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("scaffold prompt %q missing %q", node.PromptTemplate, want)
		}
	}
}

func TestCoderOutcomeToolAdvertisedOnImplementNode(t *testing.T) {
	def := taskHarnessDefinition(t)
	node := findNodeByID(t, def, "implement")
	face, ok, err := LoadBuiltinToolFace("submit_coder_outcome")
	if err != nil || !ok {
		t.Fatalf("LoadBuiltinToolFace(submit_coder_outcome): ok=%v err=%v", ok, err)
	}
	if !declaredInFlowTools(def, face.ID) {
		t.Fatalf("task-harness tools list does not declare %q", face.ID)
	}
	prompt := loadPackPrompt(t, node.PromptTemplate)
	for _, want := range []string{"submit_coder_outcome", "renegotiate_signatures", "batch_signature_requests", "completed"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("coder prompt %q missing %q", node.PromptTemplate, want)
		}
	}
}

// declaredInFlowTools reports whether the flow's tools list names the face file.
func declaredInFlowTools(def FlowDefinition, faceID string) bool {
	// FlowDefinition carries raw tool paths; the face id is the file stem with
	// dashes turned to underscores (submit-scaffold-outcome.yaml).
	want := strings.ReplaceAll(faceID, "_", "-") + ".yaml"
	for _, tool := range def.Tools {
		if strings.HasSuffix(tool, want) {
			return true
		}
	}
	return false
}

func loadPackPrompt(t *testing.T, path string) string {
	t.Helper()
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, p := range pack.Prompts {
		if p.Path == path {
			return p.Contents
		}
	}
	t.Fatalf("pack has no prompt %q", path)
	return ""
}
