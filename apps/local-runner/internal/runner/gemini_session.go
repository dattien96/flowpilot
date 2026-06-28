package runner

import "sync"

// geminiSessionMap tracks FlowPilot's synthetic per-run session id to Gemini's
// provider-owned ACP session id. The live registry creates a new adapter per turn,
// so this state lives on Runner and is shared across adapter instances. Mapping is
// scoped by Gemini account/scope key so stale ids do not leak across accounts.
type geminiSessionMap struct {
	mu   sync.Mutex
	real map[string]map[string]string
}

func newGeminiSessionMap() *geminiSessionMap {
	return &geminiSessionMap{real: map[string]map[string]string{}}
}

func (m *geminiSessionMap) realSession(scopeKey, fpSessionID string) string {
	if m == nil || scopeKey == "" || fpSessionID == "" {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if scoped, ok := m.real[scopeKey]; ok {
		return scoped[fpSessionID]
	}
	return ""
}

func (m *geminiSessionMap) setRealSession(scopeKey, fpSessionID, geminiSessionID string) {
	if m == nil || scopeKey == "" || fpSessionID == "" || geminiSessionID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	scoped, ok := m.real[scopeKey]
	if !ok {
		scoped = map[string]string{}
		m.real[scopeKey] = scoped
	}
	scoped[fpSessionID] = geminiSessionID
}
