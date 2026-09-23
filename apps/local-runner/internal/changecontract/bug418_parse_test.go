package changecontract

import (
	"testing"
)

// TestBug418_ParserDropsProseGluedPath pins the malformed legacy contract row
// observed live (grok run-587): a `files:` line where prose was concatenated
// into a path token ("stringutil/reverse.goTask-54LT-B is done…") was
// persisted verbatim into declared_paths[0]. A comma-separated declared path
// containing unquoted whitespace is prose glue, not a path — the entry must
// be dropped, not persisted as a scope member.
func TestBug418_ParserDropsProseGluedPath(t *testing.T) {
	text := "[Change Contract]\n" +
		"feature: calc-core\n" +
		"intent: fix division\n" +
		"files: stringutil/reverse.goTask-54LT-B is done and dusted, stringutil/pad.go\n"
	c, ok := ParseDeclaration(text)
	if !ok {
		t.Fatal("declaration block must parse")
	}
	for _, p := range c.DeclaredPaths {
		if p == "stringutil/reverse.goTask-54LT-B is done and dusted" {
			t.Fatalf("prose-glued path must not persist: %q", p)
		}
	}
	if len(c.DeclaredPaths) != 1 || c.DeclaredPaths[0] != "stringutil/pad.go" {
		t.Fatalf("DeclaredPaths = %v, want only the clean entry", c.DeclaredPaths)
	}
}

// TestBug418_ParserKeepsQuotedSpacedPath guards the legit case the whitespace
// filter must not eat: a real path containing a space survives when quoted.
func TestBug418_ParserKeepsQuotedSpacedPath(t *testing.T) {
	text := "[Change Contract]\n" +
		"feature: demo\n" +
		"files: \"src/my file.go\", src/other.go\n"
	c, ok := ParseDeclaration(text)
	if !ok {
		t.Fatal("declaration block must parse")
	}
	if len(c.DeclaredPaths) != 2 || c.DeclaredPaths[0] != "src/my file.go" || c.DeclaredPaths[1] != "src/other.go" {
		t.Fatalf("DeclaredPaths = %v, want quoted spaced path + plain path", c.DeclaredPaths)
	}
}
