package changecontract

import "testing"

// CP-43 P-1 Plan A: ParseDeclaration must handle prompts/tests that arrive with
// CRLF (\r\n) or bare CR (\r) separators (e.g. NDJSON with \r or Windows
// clipboard). The runner gates on Change Contract; a \r-only block must not
// be treated as absent. Provider-agnostic (pure string parsing).

func TestParseDeclarationCRLF(t *testing.T) {
	text := "[Change Contract]\r\nfeature: calc-core\r\nintent: add guard\r\nfiles: calc.go, calc_test.go\r\nsymbols: Subtract\r\n"
	c, ok := ParseDeclaration(text)
	if !ok {
		t.Fatal("CRLF block must parse")
	}
	if c.FeatureKey != "calc-core" {
		t.Fatalf("feature_key=%q want calc-core", c.FeatureKey)
	}
	if len(c.DeclaredPaths) != 2 || c.DeclaredPaths[0] != "calc.go" {
		t.Fatalf("declared_paths=%v want [calc.go calc_test.go]", c.DeclaredPaths)
	}
}

func TestParseDeclarationBareCR(t *testing.T) {
	// As stored in run-204658 turns NDJSON where prompt uses \r as line sep.
	text := "[Change Contract]\rfeature: calc-core\rintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\rfiles: calc.go, calc_test.go\rsymbols: Subtract"
	c, ok := ParseDeclaration(text)
	if !ok {
		t.Fatalf("bare CR block must parse (run-204658 repro), got not ok")
	}
	if c.FeatureKey != "calc-core" {
		t.Fatalf("feature_key=%q want calc-core", c.FeatureKey)
	}
	if len(c.DeclaredPaths) != 2 {
		t.Fatalf("declared_paths=%v want 2 entries", c.DeclaredPaths)
	}
}

func TestParseDeclarationMixedCRLFAndLF(t *testing.T) {
	text := "[Change Contract]\r\nfeature: calc-core\nintent: mixed\r\nfiles: a.go"
	c, ok := ParseDeclaration(text)
	if !ok {
		t.Fatal("mixed CRLF/LF must parse")
	}
	if c.FeatureKey != "calc-core" {
		t.Fatalf("feature_key=%q", c.FeatureKey)
	}
}
