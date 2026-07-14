package changecontract

import (
	"reflect"
	"testing"
)

func TestParseDeclarationFullBlock(t *testing.T) {
	text := `Some preamble the AI writes.

[Change Contract]
feature: calc-core
intent: add DivideChecked with explicit error handling
files: calc.go, calc_test.go

I'll now make the change.`

	c, ok := ParseDeclaration(text)
	if !ok {
		t.Fatal("expected ok=true for a full declaration block")
	}
	if c.Confidence != ConfidenceDeclared {
		t.Errorf("Confidence = %q, want %q", c.Confidence, ConfidenceDeclared)
	}
	if c.FeatureKey != "calc-core" {
		t.Errorf("FeatureKey = %q, want calc-core", c.FeatureKey)
	}
	if c.Intent != "add DivideChecked with explicit error handling" {
		t.Errorf("Intent = %q, want the declared intent", c.Intent)
	}
	want := []string{"calc.go", "calc_test.go"}
	if !reflect.DeepEqual(c.DeclaredPaths, want) {
		t.Errorf("DeclaredPaths = %v, want %v", c.DeclaredPaths, want)
	}
}

func TestParseDeclarationFencedBlock(t *testing.T) {
	text := "```change-contract\n[Change Contract]\nfeature: user-id\nintent: fix validation\nfiles: user.go\n```\n"

	c, ok := ParseDeclaration(text)
	if !ok {
		t.Fatal("expected ok=true for a fenced declaration block")
	}
	if c.FeatureKey != "user-id" || c.Intent != "fix validation" {
		t.Errorf("unexpected parse: %+v", c)
	}
	if !reflect.DeepEqual(c.DeclaredPaths, []string{"user.go"}) {
		t.Errorf("DeclaredPaths = %v, want [user.go]", c.DeclaredPaths)
	}
}

func TestParseDeclarationPartialBlockToleratesMissingLines(t *testing.T) {
	text := `[Change Contract]
feature: calc-core

rest of the message`

	c, ok := ParseDeclaration(text)
	if !ok {
		t.Fatal("expected ok=true even for a partial block")
	}
	if c.FeatureKey != "calc-core" {
		t.Errorf("FeatureKey = %q, want calc-core", c.FeatureKey)
	}
	if c.Intent != "" {
		t.Errorf("Intent = %q, want empty (not declared)", c.Intent)
	}
	if c.DeclaredPaths != nil {
		t.Errorf("DeclaredPaths = %v, want nil (not declared)", c.DeclaredPaths)
	}
}

func TestParseDeclarationAbsentReturnsNotOK(t *testing.T) {
	text := "I looked at calc.go and fixed the bug directly, no declaration here."

	c, ok := ParseDeclaration(text)
	if ok {
		t.Fatalf("expected ok=false when no [Change Contract] marker is present, got %+v", c)
	}
}

func TestParseDeclarationCaseInsensitiveMarker(t *testing.T) {
	text := "[change contract]\nfeature: calc-core\n"
	if _, ok := ParseDeclaration(text); !ok {
		t.Fatal("expected the marker match to be case-insensitive")
	}
}
