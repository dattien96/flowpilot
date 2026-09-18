// Package lsp embeds a minimal Language Server Protocol client in the
// FlowPilot Go runner (CP-63). It speaks JSON-RPC 2.0 over stdio to one
// language server process per (workspace, language) and surfaces live
// compiler diagnostics to the agent loop. It intentionally implements only
// the small LSP subset CP-63 needs (initialize, didOpen/didChange/didClose,
// publishDiagnostics) instead of pulling a full LSP framework dependency.
package lsp

// Position is a zero-based line/character offset in a text document (LSP 3.17).
type Position struct {
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
}

// Range is a half-open [Start, End) span in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location pairs a document URI with a range inside it.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextDocumentIdentifier names an already-open document.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentItem is the full content of a newly opened document.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int32  `json:"version"`
	Text       string `json:"text"`
}

// VersionedTextDocumentIdentifier names a document at a known version.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int32  `json:"version"`
}

// ContentChangeEvent carries new document content. Only full-document sync
// is used (Text set, Range nil); range-based deltas are left nil.
type ContentChangeEvent struct {
	Text  string `json:"text"`
	Range *Range `json:"range,omitempty"`
}

// DiagnosticSeverity mirrors the LSP DiagnosticSeverity enumeration.
type DiagnosticSeverity int

const (
	DiagnosticSeverityError       DiagnosticSeverity = 1
	DiagnosticSeverityWarning     DiagnosticSeverity = 2
	DiagnosticSeverityInformation DiagnosticSeverity = 3
	DiagnosticSeverityHint        DiagnosticSeverity = 4
)

// String renders the severity the way agents and logs expect it.
func (s DiagnosticSeverity) String() string {
	switch s {
	case DiagnosticSeverityError:
		return "error"
	case DiagnosticSeverityWarning:
		return "warning"
	case DiagnosticSeverityInformation:
		return "information"
	case DiagnosticSeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

// Diagnostic is a single compiler/analyzer finding in a document.
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Code     any                `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

// PublishDiagnosticsParams is the body of a textDocument/publishDiagnostics
// notification pushed by the server.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     int32        `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// DidOpenTextDocumentParams opens a document on the server.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidChangeTextDocumentParams pushes new content for an open document.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []ContentChangeEvent            `json:"contentChanges"`
}

// DidCloseTextDocumentParams closes a document on the server.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// WorkspaceFolder is a workspace root advertised during initialize.
type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// ClientCapabilities is intentionally minimal: an empty capability set is
// valid per the spec and every server in the CP-63 registry accepts it.
type ClientCapabilities struct{}

// InitializeParams starts the LSP handshake.
type InitializeParams struct {
	ProcessID             *int32             `json:"processId"`
	RootURI               *string            `json:"rootUri,omitempty"`
	WorkspaceFolders      []WorkspaceFolder  `json:"workspaceFolders,omitempty"`
	Capabilities          ClientCapabilities `json:"capabilities"`
	InitializationOptions any                `json:"initializationOptions,omitempty"`
}

// ServerCapabilities keeps only what the runner inspects; unknown fields
// are ignored by encoding/json so servers may advertise freely.
type ServerCapabilities struct {
	TextDocumentSync   any `json:"textDocumentSync,omitempty"`
	DiagnosticProvider any `json:"diagnosticProvider,omitempty"`
	DefinitionProvider any `json:"definitionProvider,omitempty"`
	ReferencesProvider any `json:"referencesProvider,omitempty"`
	HoverProvider      any `json:"hoverProvider,omitempty"`
}

// ServerInfo identifies the language server implementation.
type ServerInfo struct {
	Name    string  `json:"name"`
	Version *string `json:"version,omitempty"`
}

// InitializeResult is the server's handshake answer.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}
