package flowgate

import (
	"strings"
	"testing"
)

// CP-67 P-2b (Task-383, B-11): static stub-body whitelist tests.

func TestValidateStubBodiesGo(t *testing.T) {
	src := []byte(`package service

import "errors"

func GetUser(id string) (*User, error) {
	return nil, errors.New("not implemented")
}

func FindUser(id string) (*User, error) {
	panic("not implemented")
}

func Zero() (int, error) {
	return 0, nil
}
`)
	syms, err := ExtractSymbolInfos("go", "service.go", src, nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if v := ValidateStubBodies("go", "service.go", src, nil, syms); len(v) != 0 {
		t.Fatalf("canonical Go stubs must pass the whitelist, got %v", v)
	}
}

func TestValidateStubBodiesGoRejectsNonStubLogic(t *testing.T) {
	src := []byte(`package service

func GetUser(id string) (*User, error) {
	if id == "" {
		return nil, ErrEmpty
	}
	return db.Query(id)
}
`)
	syms, err := ExtractSymbolInfos("go", "service.go", src, nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	v := ValidateStubBodies("go", "service.go", src, nil, syms)
	if len(v) == 0 {
		t.Fatal("control flow inside a stub body must be flagged NonStub")
	}
	if !strings.Contains(v[0].Symbol, "GetUser") {
		t.Fatalf("violation must name the symbol, got %+v", v[0])
	}
}

func TestGoPackageVarAndInitHidingSpots(t *testing.T) {
	src := []byte(`package service

var cache = loadCache()

func init() {
	warmUp()
}

func GetUser(id string) (*User, error) {
	return nil, errors.New("not implemented")
}
`)
	syms, err := ExtractSymbolInfos("go", "service.go", src, nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	v := ValidateStubBodies("go", "service.go", src, nil, syms)
	if len(v) < 2 {
		t.Fatalf("computed var initializer + non-empty init() must both be flagged, got %v", v)
	}
}

func TestExtractCanonicalSignaturesReactViaNode(t *testing.T) {
	src := []byte(`export function UserCard({ id }: Props) {
  throw new Error("not implemented");
}

export const useUser = (id: string) => {
  throw new Error("not implemented");
};
`)
	sigs, err := ExtractCanonicalSignatures("react", "UserCard.tsx", src, nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	// The node path needs the workspace's typescript package; when this dev
	// machine lacks it, the regex fallback still yields usable signatures —
	// but both paths must name the two declared symbols.
	joined := strings.Join(sigs, "\n")
	for _, want := range []string{"UserCard", "useUser"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("react signatures missing %q:\n%s", want, joined)
		}
	}
}

func TestValidateStubBodiesReactViaNode(t *testing.T) {
	src := []byte(`export function UserCard() {
  throw new Error("not implemented");
}
`)
	syms, err := ExtractSymbolInfos("react", "UserCard.tsx", src, nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if v := ValidateStubBodies("react", "UserCard.tsx", src, nil, syms); len(v) != 0 {
		t.Fatalf("not-implemented throw bodies must pass the whitelist, got %v", v)
	}
}

func TestValidateStubBodiesReactRejectsNonStubLogic(t *testing.T) {
	src := []byte(`export function UserCard() {
  const data = fetchUsers();
  return renderList(data);
}
`)
	syms, err := ExtractSymbolInfos("react", "UserCard.tsx", src, nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if v := ValidateStubBodies("react", "UserCard.tsx", src, nil, syms); len(v) == 0 {
		t.Fatalf("multi-statement component body must be flagged NonStub, syms=%+v", syms)
	}
}

func TestValidateStubBodiesKotlinViaLSP(t *testing.T) {
	kotlinSrc := []byte(`fun getUser(id: String): User = TODO("not implemented")

fun findUser(id: String): User {
    throw NotImplementedError("not implemented")
}
`)
	mock := mockSymbolSource{symbols: []DocumentSymbol{
		{Name: "getUser", Kind: "function", Detail: "fun getUser(id: String): User", StartLine: 1, EndLine: 1},
		{Name: "findUser", Kind: "function", Detail: "fun findUser(id: String): User", StartLine: 3, EndLine: 5},
	}}
	syms, err := ExtractSymbolInfos("kotlin", "UserService.kt", kotlinSrc, mock)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if v := ValidateStubBodies("kotlin", "UserService.kt", kotlinSrc, mock, syms); len(v) != 0 {
		t.Fatalf("kotlin TODO/NotImplementedError stubs must pass, got %v", v)
	}
}

func TestExtractCanonicalSignaturesCppViaLSP(t *testing.T) {
	// The tree-sitter exact parser stays behind the `treesitter` build tag
	// (R-6: default build keeps CGO_ENABLED=0); the shipped path is
	// clangd LSP-anchored — same anchor contract, documented deviation.
	cppSrc := []byte(`int getBalance() { return -1; }

User* findUser(int id) { return nullptr; }
`)
	mock := mockSymbolSource{symbols: []DocumentSymbol{
		{Name: "getBalance", Kind: "function", Detail: "int getBalance()", StartLine: 1, EndLine: 1},
		{Name: "findUser", Kind: "function", Detail: "User* findUser(int id)", StartLine: 3, EndLine: 3},
	}}
	syms, err := ExtractSymbolInfos("cpp", "service.cpp", cppSrc, mock)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(syms) < 2 {
		t.Fatalf("expected 2 cpp symbols, got %d", len(syms))
	}
	if v := ValidateStubBodies("cpp", "service.cpp", cppSrc, mock, syms); len(v) != 0 {
		t.Fatalf("cpp zero-sentinel stubs must pass, got %v", v)
	}
}

func TestValidateStubBodiesCpp(t *testing.T) {
	cppSrc := []byte(`void run() {
    throw std::runtime_error("not implemented");
}
`)
	mock := mockSymbolSource{symbols: []DocumentSymbol{
		{Name: "run", Kind: "function", Detail: "void run()", StartLine: 1, EndLine: 3},
	}}
	syms, err := ExtractSymbolInfos("cpp", "service.cpp", cppSrc, mock)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if v := ValidateStubBodies("cpp", "service.cpp", cppSrc, mock, syms); len(v) != 0 {
		t.Fatalf("std::runtime_error stub body must pass, got %v", v)
	}
}

func TestCppMacroBodyHidingDetected(t *testing.T) {
	cppSrc := []byte(`#define RUN_LOGIC(x) { if (x) doA(); doB(); }

int getBalance() { return RUN_LOGIC(1); }
`)
	syms := []SymbolInfo{{Name: "getBalance", Kind: "function", Signature: "int getBalance()", BodyShape: BodyStubReturnZero, Line: 3}}
	v := ValidateStubBodies("cpp", "service.cpp", cppSrc, nil, syms)
	if len(v) == 0 {
		t.Fatal("a multi-statement macro definition must be flagged as a hiding spot")
	}
	if !strings.Contains(v[0].Symbol, "RUN_LOGIC") {
		t.Fatalf("violation must name the macro, got %+v", v[0])
	}
}

func TestCppDeclDefDedupeSignatureHashStable(t *testing.T) {
	decl := SymbolInfo{Name: "foo", Kind: "function", Signature: "int foo(int)", BodyShape: BodyUnverified, Line: 1}
	def := SymbolInfo{Name: "foo", Kind: "function", Signature: "int foo(int)", BodyShape: BodyStubReturnZero, Line: 5}
	deduped := DedupeDeclAndDef([]SymbolInfo{decl, def})
	if len(deduped) != 1 {
		t.Fatalf("decl + def must dedupe to one symbol, got %d", len(deduped))
	}
	if deduped[0].BodyShape != BodyStubReturnZero {
		t.Fatal("dedupe must keep the definition (verified body) row")
	}
}

func TestSignatureHashUnchangedWithBodyShapeOutput(t *testing.T) {
	// B-8.1 compat: BodyShape rides SymbolInfo but never leaks into the
	// canonical strings — the P-2 hash equals the P-2b hash.
	syms := []SymbolInfo{
		{Name: "GetUser", Kind: "function", Signature: "func GetUser(id string) (*User, error)", BodyShape: BodyStubReturnZero, Line: 3},
		{Name: "GetUser", Kind: "function", Signature: "func GetUser(id string) (*User, error)", BodyShape: BodyNonStub, Line: 3},
	}
	a := CanonicalSignatureHash(CanonicalStrings(syms[:1]))
	b := CanonicalSignatureHash(CanonicalStrings(syms[1:]))
	if a != b {
		t.Fatal("BodyShape must not influence CanonicalSignatureHash")
	}
}

func TestStubBodyCacheInvalidation(t *testing.T) {
	c := NewStubBodyCache()
	src := []byte("package a\n")
	if got := c.Get("a.go", src); got != nil {
		t.Fatal("empty cache must miss")
	}
	c.Put("a.go", src, []SymbolInfo{{Name: "A", Signature: "func A()"}})
	if got := c.Get("a.go", src); got == nil || len(got) != 1 {
		t.Fatal("same content must hit")
	}
	changed := []byte("package a\n// edited\n")
	if got := c.Get("a.go", changed); got != nil {
		t.Fatal("changed content must miss (content-hash keyed)")
	}
}

func TestStubBodyUnverifiedFailOpen(t *testing.T) {
	// No LSP for Kotlin: signatures come from the regex pass with Unverified
	// bodies — ValidateStubBodies must NOT flag them (fail-open) while the
	// symbol still carries the unverified marker.
	src := []byte("fun getUser(id: String): User = TODO(\"not implemented\")\n")
	syms, err := ExtractSymbolInfos("kotlin", "UserService.kt", src, nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(syms) == 0 {
		t.Fatal("regex fallback must still produce signature rows")
	}
	if v := ValidateStubBodies("kotlin", "UserService.kt", src, nil, syms); len(v) != 0 {
		t.Fatalf("Unverified bodies must fail open, got %v", v)
	}
}

func TestStubBodyAdaptersDispatchByLanguage(t *testing.T) {
	goSrc := []byte("package a\n\nfunc A() {\n\treturn\n}\n")
	goSyms, err := ExtractSymbolInfos("go", "a.go", goSrc, nil)
	if err != nil {
		t.Fatalf("go dispatch: %v", err)
	}
	if len(goSyms) == 0 || goSyms[0].BodyShape == BodyUnverified {
		t.Fatal("go must dispatch to the exact native parser")
	}
	// Unsupported language errors; supported-but-toolless degrade (react
	// regex fallback, kotlin regex fallback) return Unverified bodies.
	if _, err := ExtractSymbolInfos("ruby", "a.rb", nil, nil); err == nil {
		t.Fatal("unsupported language must error")
	}
	ktSyms, err := ExtractSymbolInfos("kotlin", "A.kt", []byte("fun a() = TODO()\n"), nil)
	if err != nil || len(ktSyms) == 0 || ktSyms[0].BodyShape != BodyUnverified {
		t.Fatalf("kotlin without LSP must degrade to Unverified rows, got %+v err=%v", ktSyms, err)
	}
}
