package lsp

import (
	"sync"
)

// docState tracks one open document's sync version.
type docState struct {
	languageID string
	version    int32
}

// DocumentSyncManager tracks open documents and their LSP versions,
// delegating to Client.DidOpen/DidChange/DidClose with correct numbering.
// Versions start at 1 and increment on every change, per the LSP spec.
type DocumentSyncManager struct {
	mu     sync.Mutex
	client *Client
	docs   map[string]*docState
}

// NewDocumentSyncManager wraps a Client (must be non-nil).
func NewDocumentSyncManager(client *Client) *DocumentSyncManager {
	return &DocumentSyncManager{client: client, docs: make(map[string]*docState)}
}

// OpenDocument sends textDocument/didOpen at version 1. Re-opening an
// already-open document is a no-op (LSP opens a document exactly once).
func (m *DocumentSyncManager) OpenDocument(uri, languageID, content string) error {
	m.mu.Lock()
	if _, ok := m.docs[uri]; ok {
		m.mu.Unlock()
		return nil
	}
	m.docs[uri] = &docState{languageID: languageID, version: 1}
	m.mu.Unlock()
	return m.client.DidOpen(DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: languageID, Version: 1, Text: content},
	})
}

// ChangeDocument sends textDocument/didChange with full content and bumps
// the version. A document that was never opened is opened first using the
// LanguageID guessed from its extension, so callers only need one call.
func (m *DocumentSyncManager) ChangeDocument(uri, newContent string) error {
	m.mu.Lock()
	st, ok := m.docs[uri]
	if !ok {
		m.docs[uri] = &docState{languageID: LanguageID(uri), version: 1}
		m.mu.Unlock()
		return m.client.DidOpen(DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{URI: uri, LanguageID: LanguageID(uri), Version: 1, Text: newContent},
		})
	}
	st.version++
	version := st.version
	m.mu.Unlock()
	return m.client.DidChange(DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uri, Version: version},
		ContentChanges: []ContentChangeEvent{{Text: newContent}},
	})
}

// CloseDocument sends textDocument/didClose and forgets the document.
// Closing an unknown document is a no-op.
func (m *DocumentSyncManager) CloseDocument(uri string) error {
	m.mu.Lock()
	if _, ok := m.docs[uri]; !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.docs, uri)
	m.mu.Unlock()
	return m.client.DidClose(DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
}

// IsOpen reports whether uri is currently tracked.
func (m *DocumentSyncManager) IsOpen(uri string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.docs[uri]
	return ok
}

// Version returns the current sync version of uri, or 0 when unknown.
func (m *DocumentSyncManager) Version(uri string) int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.docs[uri]; ok {
		return st.version
	}
	return 0
}

// OpenCount reports how many documents are tracked (tests/diagnostics).
func (m *DocumentSyncManager) OpenCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.docs)
}
