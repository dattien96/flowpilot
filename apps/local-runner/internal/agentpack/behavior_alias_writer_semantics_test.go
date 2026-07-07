package agentpack

import "testing"

// CP-55 P-1 (Task-263): NormalizeBehaviorID regression coverage for the new
// explicit writer/freeze self-mappings. GitNexus flagged NormalizeBehaviorID
// CRITICAL (11 direct callers, 56 impacted symbols, 5 processes, 3 modules)
// before this edit; these tests are the mandatory regression scope that
// pairs with the additive behaviorAliases entries in pack.go. Kept in a new
// file — no pre-existing test is modified.

func TestNormalizeBehaviorIDResolvesNewWriterFreezeSelfMappings(t *testing.T) {
	for _, id := range []string{"agent.code", "contract.freeze"} {
		canonical, ok := NormalizeBehaviorID(id)
		if !ok {
			t.Errorf("NormalizeBehaviorID(%q) ok = false, want true", id)
			continue
		}
		if canonical != id {
			t.Errorf("NormalizeBehaviorID(%q) = %q, want self-mapped %q", id, canonical, id)
		}
	}
}

func TestNormalizeBehaviorIDExistingAliasesUnchanged(t *testing.T) {
	cases := map[string]string{
		"agent.delegate":                        "agent.delegate",
		"coding":                                "agent.delegate",
		"implementation":                        "agent.delegate",
		"code":                                  "agent.delegate",
		"hub.inline":                            "hub.inline",
		"hub.synthesize":                        "hub.inline",
		"context.produce":                       "context.produce",
		"context.deterministic_feature_package": "context.produce",
		"context.deterministic_feature_context": "context.produce",
		"plan":                                  "context.produce",
		"planning":                              "context.produce",
		"design":                                "context.produce",
		"context.render":                        "context.render",
		"context.prompt_handoff":                "context.render",
		"command.validate":                      "command.validate",
		"validation.command":                    "command.validate",
		"validation.summarize":                  "validation.summarize",
		"artifact.audit_draft":                  "artifact.audit_draft",
		"audit.draft":                           "artifact.audit_draft",
		"telegram.notify":                       "telegram.notify",
		"notify.telegram":                       "telegram.notify",
		"hub.notify":                            "hub.notify",
		"notify.hub":                            "hub.notify",
		"flow.control":                          "flow.control",
		"flow.control_tool":                     "flow.control",
		"user.confirm":                          "user.confirm",
	}
	for alias, want := range cases {
		got, ok := NormalizeBehaviorID(alias)
		if !ok {
			t.Errorf("NormalizeBehaviorID(%q) ok = false, want true", alias)
			continue
		}
		if got != want {
			t.Errorf("NormalizeBehaviorID(%q) = %q, want %q", alias, got, want)
		}
	}
}

func TestNormalizeBehaviorIDRejectsUnknownID(t *testing.T) {
	if _, ok := NormalizeBehaviorID("totally.unknown"); ok {
		t.Fatal("expected unknown behavior id to fail normalization")
	}
}

func TestNormalizeBehaviorIDRejectsEmptyID(t *testing.T) {
	if _, ok := NormalizeBehaviorID("   "); ok {
		t.Fatal("expected blank behavior id to fail normalization")
	}
}
