package flowgate

import (
	"strings"
	"testing"
)

// CP-67 P-2/P-2b (Task-379 / Task-383): canonical AST extraction tests.

const goStubFixture = `package service

import "errors"

type User struct {
	ID string
}

type UserService interface {
	GetUser(id string) (*User, error)
}

func GetUser(id string) (*User, error) {
	return nil, errors.New("not implemented")
}

func CreateUser(name string) (*User, error) {
	panic("not implemented")
}
`

func TestExtractCanonicalSignaturesGo(t *testing.T) {
	sigs, err := ExtractCanonicalSignatures("go", "service.go", []byte(goStubFixture), nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	joined := strings.Join(sigs, "\n")
	for _, want := range []string{
		"func CreateUser(name string) (*User, error)",
		"func GetUser(id string) (*User, error)",
		"type User struct { ID string }",
		"GetUser(id string) (*User, error)", // interface method set member
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("canonical signatures missing %q:\n%s", want, joined)
		}
	}
	// Deterministic order: sorted.
	for i := 1; i < len(sigs); i++ {
		if sigs[i-1] > sigs[i] {
			t.Fatalf("signatures not sorted: %v", sigs)
		}
	}
}

func TestCanonicalSignatureHashDeterministic(t *testing.T) {
	setA := []string{"func B()", "func A()"}
	setB := []string{"func A()", "func B()"}
	if CanonicalSignatureHash(setA) != CanonicalSignatureHash(setB) {
		t.Fatal("same declaration set must hash identically regardless of order")
	}
	if CanonicalSignatureHash(setA) == CanonicalSignatureHash([]string{"func A()", "func B()", "func C()"}) {
		t.Fatal("different declaration sets must hash differently")
	}
	// SHA256 hex is 64 chars.
	if len(CanonicalSignatureHash(setA)) != 64 {
		t.Fatalf("hash length = %d, want 64 hex chars", len(CanonicalSignatureHash(setA)))
	}
}

func TestExtractCanonicalSignaturesKotlinViaLSP(t *testing.T) {
	// LSP-anchored extraction: the mock SymbolSource supplies the declaration
	// ranges (what the real kotlin-language-server would return); the body
	// text is sliced from the provided bytes and shape-matched.
	kotlinSrc := []byte(`package service

data class User(val id: String)

interface UserService {
    fun getUser(id: String): User
}

fun getUser(id: String): User = TODO("not implemented")
`)
	mock := mockSymbolSource{symbols: []DocumentSymbol{
		{Name: "User", Kind: "struct", StartLine: 3, EndLine: 3},
		{Name: "UserService", Kind: "interface", StartLine: 5, EndLine: 7},
		{Name: "getUser", Kind: "function", Detail: "fun getUser(id: String): User", StartLine: 9, EndLine: 9},
	}}
	syms, err := ExtractSymbolInfos("kotlin", "UserService.kt", kotlinSrc, mock)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(syms) < 3 {
		t.Fatalf("expected at least 3 symbols, got %d: %+v", len(syms), syms)
	}
	var sawStruct, sawInterface, sawFunc bool
	for _, s := range syms {
		switch s.Name {
		case "User":
			sawStruct = s.Kind == "struct"
		case "UserService":
			sawInterface = s.Kind == "interface"
		case "getUser":
			sawFunc = s.Kind == "function" && s.BodyShape == BodyStubTODO
		}
	}
	if !sawStruct || !sawInterface || !sawFunc {
		t.Fatalf("kotlin extraction incomplete: struct=%v interface=%v func=%v (%+v)", sawStruct, sawInterface, sawFunc, syms)
	}
}

// mockSymbolSource is the fake LSP the anchored-language tests inject.
type mockSymbolSource struct {
	symbols []DocumentSymbol
	err     error
}

func (m mockSymbolSource) DocumentSymbols(filePath string) ([]DocumentSymbol, error) {
	return m.symbols, m.err
}
