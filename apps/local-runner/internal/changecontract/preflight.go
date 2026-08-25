// CP-55 P-2: the frozen preflight contract model.
//
// This is deliberately a separate lifecycle from Contract/Store in
// contract.go. Contract is Task-184's post-hoc declared-or-inferred capture,
// read by chat-mode context injection and the legacy r-contract/r-scope gate;
// changing its shape or tolerant-load behavior would touch every one of those
// call sites for no reason a Flow preflight needs. PreflightContractDraft /
// FrozenContractRecord instead give CP-55's Flow lifecycle (freeze before
// context, before the coder runs) its own strict, immutable, versioned
// artifact — stored in its own NDJSON files under the same
// .flowpilot/contracts/ directory, never mixed with contracts.ndjson.
package changecontract

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ContractStatusEvent.Status values (CP-55 §3.3 lifecycle).
const (
	ContractStatusFrozen     = "frozen"
	ContractStatusSuperseded = "superseded"
	ContractStatusAccepted   = "accepted"
	ContractStatusAbandoned  = "abandoned"
)

// PreflightContractDraft is what a read-only AI contract planner proposes,
// before Go validates and freezes it. It carries no timestamp, version or id
// — those are assigned only at freeze time, by the runtime, never by the AI.
type PreflightContractDraft struct {
	FeatureKey    string   `json:"feature_key"`
	Intent        string   `json:"intent"`
	DeclaredPaths []string `json:"declared_paths"`
	SourceDocID   string   `json:"source_doc_id,omitempty"`
}

// FrozenContractRecord is one immutable, versioned freeze of a
// PreflightContractDraft, bound to a specific run/coder-step/baseline. It is
// never mutated after SaveFrozen persists it — an amendment is a new record
// with Version+1 and Supersedes set to the prior ContractID.
type FrozenContractRecord struct {
	ContractID       string            `json:"contract_id"`
	Version          int               `json:"version"`
	RunID            string            `json:"run_id"`
	PlannerStepID    string            `json:"planner_step_id,omitempty"`
	CoderStepID      string            `json:"coder_step_id"`
	FeatureKey       string            `json:"feature_key"`
	Intent           string            `json:"intent"`
	DeclaredPaths    []string          `json:"declared_paths"`
	SourceDocID      string            `json:"source_doc_id,omitempty"`
	BaseSHA          string            `json:"base_sha,omitempty"`
	BaselineWorktree map[string]string `json:"baseline_worktree,omitempty"`
	Supersedes       string            `json:"supersedes,omitempty"`
	DeclaredAt       time.Time         `json:"declared_at"`
}

// ContractStatusEvent is one append-only lifecycle transition for a frozen
// contract. Frozen payload and status are separate records by design: the
// payload never changes after freeze, while status accumulates over time
// (frozen -> accepted, or frozen -> superseded, or frozen -> abandoned).
type ContractStatusEvent struct {
	ContractID string    `json:"contract_id"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	At         time.Time `json:"at"`
}

// extractJSONObject extracts the first balanced top-level JSON object from text,
// stripping any surrounding markdown fences (```json ... ```) or conversational prose.
func extractJSONObject(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 {
			if strings.HasPrefix(lines[0], "```") {
				lines = lines[1:]
			}
			if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			trimmed = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	start := strings.IndexByte(trimmed, '{')
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(trimmed); i++ {
		b := trimmed[i]
		if inString {
			if escape {
				escape = false
			} else if b == '\\' {
				escape = true
			} else if b == '"' {
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return trimmed[start : i+1], true
			}
		}
	}
	return "", false
}

// ParsePreflightDraft parses text as a PreflightContractDraft object.
// Unknown fields are rejected. If the model enclosed the draft in conversational
// prose (e.g. Grok or Claude leading commentary) or markdown code fences,
// the embedded JSON object is automatically extracted and validated.
func ParsePreflightDraft(text string) (PreflightContractDraft, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return PreflightContractDraft{}, errors.New("changecontract: empty preflight draft")
	}
	// Try direct strict decode first.
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var draft PreflightContractDraft
	if err := dec.Decode(&draft); err == nil && !dec.More() {
		return draft, nil
	}
	// If direct strict decode failed, try extracting the embedded JSON object.
	if candidate, ok := extractJSONObject(trimmed); ok {
		dec2 := json.NewDecoder(strings.NewReader(candidate))
		dec2.DisallowUnknownFields()
		var draft2 PreflightContractDraft
		if err := dec2.Decode(&draft2); err == nil {
			return draft2, nil
		}
	}
	// Fall back to returning the original strict decode error for diagnostics.
	dec3 := json.NewDecoder(strings.NewReader(trimmed))
	dec3.DisallowUnknownFields()
	var draft3 PreflightContractDraft
	if err := dec3.Decode(&draft3); err != nil {
		return PreflightContractDraft{}, fmt.Errorf("changecontract: strict preflight draft parse: %w", err)
	}
	if dec3.More() {
		return PreflightContractDraft{}, errors.New("changecontract: preflight draft has trailing content after the JSON value")
	}
	return draft3, nil
}

// ValidatePreflightDraft checks the structural requirements a draft must meet
// before it can be frozen: a feature_key present (and, when knownFeatureKeys
// is non-empty, a member of it), a non-blank intent, and at least one
// declared path that IsConcreteCodeTarget accepts. This is a cheap structural
// gate — the authoritative, workspace-aware normalization (escape checks,
// dedup, sort) happens in NormalizeDeclaredCodePaths at freeze time.
func ValidatePreflightDraft(draft PreflightContractDraft, knownFeatureKeys []string) (PreflightContractDraft, error) {
	fk := strings.TrimSpace(draft.FeatureKey)
	if fk == "" {
		return PreflightContractDraft{}, errors.New("changecontract: preflight draft requires a feature_key")
	}
	if len(knownFeatureKeys) > 0 {
		known := false
		for _, k := range knownFeatureKeys {
			if strings.TrimSpace(k) == fk {
				known = true
				break
			}
		}
		if !known {
			return PreflightContractDraft{}, fmt.Errorf("changecontract: unknown feature_key %q", fk)
		}
	}

	intent := strings.TrimSpace(draft.Intent)
	if intent == "" {
		return PreflightContractDraft{}, errors.New("changecontract: preflight draft requires an intent")
	}

	hasConcrete := false
	for _, p := range draft.DeclaredPaths {
		if IsConcreteCodeTarget(p) {
			hasConcrete = true
			break
		}
	}
	if !hasConcrete {
		return PreflightContractDraft{}, errors.New("changecontract: preflight draft requires at least one concrete declared path")
	}

	return PreflightContractDraft{
		FeatureKey:    fk,
		Intent:        intent,
		DeclaredPaths: draft.DeclaredPaths,
		SourceDocID:   strings.TrimSpace(draft.SourceDocID),
	}, nil
}

// ComputeContractID returns a deterministic sha256 identity over every input
// that distinguishes one frozen contract from another: same inputs (including
// map iteration order, which is neutralized by sorting) always yield the same
// id; a version bump or a different baseline always yields a different one.
func ComputeContractID(runID, coderStepID string, version int, draft PreflightContractDraft, baseSHA string, baseline map[string]string) string {
	paths := append([]string(nil), draft.DeclaredPaths...)
	sort.Strings(paths)

	keys := make([]string, 0, len(baseline))
	for k := range baseline {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	baselineParts := make([]string, 0, len(keys))
	for _, k := range keys {
		baselineParts = append(baselineParts, k+"="+baseline[k])
	}

	joined := strings.Join([]string{
		runID,
		coderStepID,
		strconv.Itoa(version),
		strings.TrimSpace(draft.FeatureKey),
		strings.TrimSpace(draft.Intent),
		strings.Join(paths, ","),
		strings.TrimSpace(draft.SourceDocID),
		baseSHA,
		strings.Join(baselineParts, ","),
	}, "\x1f")

	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}

// FreezeContract normalizes draft's declared paths against workspace and
// mints the immutable FrozenContractRecord for it. version must be the
// intended version number for this (runID, coderStepID) — 1 for a first
// freeze, prior+1 for an amendment with supersedes set to the prior
// ContractID. Callers persist the result via FrozenStore.SaveFrozen.
func FreezeContract(
	workspace string,
	runID string,
	plannerStepID string,
	coderStepID string,
	draft PreflightContractDraft,
	baseSHA string,
	baseline map[string]string,
	supersedes string,
	version int,
	now time.Time,
) (FrozenContractRecord, error) {
	if version < 1 {
		version = 1
	}
	normalized, err := NormalizeDeclaredCodePaths(workspace, draft.DeclaredPaths)
	if err != nil {
		return FrozenContractRecord{}, err
	}
	d := draft
	d.DeclaredPaths = normalized

	id := ComputeContractID(runID, coderStepID, version, d, baseSHA, baseline)

	return FrozenContractRecord{
		ContractID:       id,
		Version:          version,
		RunID:            runID,
		PlannerStepID:    plannerStepID,
		CoderStepID:      coderStepID,
		FeatureKey:       d.FeatureKey,
		Intent:           d.Intent,
		DeclaredPaths:    d.DeclaredPaths,
		SourceDocID:      d.SourceDocID,
		BaseSHA:          baseSHA,
		BaselineWorktree: baseline,
		Supersedes:       supersedes,
		DeclaredAt:       now,
	}, nil
}

// frozenStoreLocks path-keys the write mutex across every FrozenStore
// instance in this process (frozen design decision: writes must serialize
// across instances, not just within one instance's own field). Multi-process
// locking is out of scope here — see FrozenStore's doc comment.
var (
	frozenStoreLocksMu sync.Mutex
	frozenStoreLocks   = map[string]*sync.Mutex{}
)

func lockForPath(absPath string) *sync.Mutex {
	frozenStoreLocksMu.Lock()
	defer frozenStoreLocksMu.Unlock()
	if mu, ok := frozenStoreLocks[absPath]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	frozenStoreLocks[absPath] = mu
	return mu
}

// FrozenStore persists FrozenContractRecord/ContractStatusEvent to
// <workspace>/.flowpilot/contracts/{frozen_contracts,frozen_contract_events}.ndjson
// — append + fsync, strict reload (a corrupt line fails NewFrozenStore rather
// than being silently skipped, unlike the tolerant legacy Store). Writes
// serialize across every FrozenStore built against the same file in this
// process via a path-keyed mutex; a second OS process writing concurrently is
// not guarded here — that is an explicit P-3/P-4 prerequisite (the runner
// process boundary, not this store, owns that decision).
type FrozenStore struct {
	mu            *sync.Mutex
	contractsPath string
	eventsPath    string

	byID           map[string]FrozenContractRecord
	versionsByStep map[string][]FrozenContractRecord // key: runID + "\x00" + coderStepID
	statusByID     map[string][]ContractStatusEvent
}

func stepKey(runID, coderStepID string) string { return runID + "\x00" + coderStepID }

// NewFrozenStore opens (creating if needed) the frozen-contract store rooted
// at workspace, loading any existing records. A corrupt or partial line in
// either file fails the open outright — this store fails closed, unlike the
// legacy Store's "tolerate a corrupt line" behavior, since a Flow preflight
// guarantee must not silently continue on a torn/ambiguous record.
func NewFrozenStore(workspace string) (*FrozenStore, error) {
	dir := filepath.Join(workspace, ".flowpilot", "contracts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	contractsPath := filepath.Join(dir, "frozen_contracts.ndjson")
	eventsPath := filepath.Join(dir, "frozen_contract_events.ndjson")

	absContracts, err := filepath.Abs(contractsPath)
	if err != nil {
		return nil, err
	}

	s := &FrozenStore{
		mu:             lockForPath(absContracts),
		contractsPath:  contractsPath,
		eventsPath:     eventsPath,
		byID:           make(map[string]FrozenContractRecord),
		versionsByStep: make(map[string][]FrozenContractRecord),
		statusByID:     make(map[string][]ContractStatusEvent),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadContractsLocked(); err != nil {
		return nil, err
	}
	if err := s.loadEventsLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *FrozenStore) loadContractsLocked() error {
	f, err := os.Open(s.contractsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec FrozenContractRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return fmt.Errorf("changecontract: corrupt frozen contract record at %s:%d: %w", s.contractsPath, lineNo, err)
		}
		s.byID[rec.ContractID] = rec
		key := stepKey(rec.RunID, rec.CoderStepID)
		s.versionsByStep[key] = append(s.versionsByStep[key], rec)
	}
	return scanner.Err()
}

func (s *FrozenStore) loadEventsLocked() error {
	f, err := os.Open(s.eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev ContractStatusEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return fmt.Errorf("changecontract: corrupt contract status event at %s:%d: %w", s.eventsPath, lineNo, err)
		}
		s.statusByID[ev.ContractID] = append(s.statusByID[ev.ContractID], ev)
	}
	return scanner.Err()
}

// SaveFrozen persists rec. Same ContractID with a byte-identical payload is a
// no-op (idempotent under retry/recovery); same ContractID with a different
// payload is rejected — a frozen record's own payload never mutates.
func (s *FrozenStore) SaveFrozen(rec FrozenContractRecord) error {
	if strings.TrimSpace(rec.ContractID) == "" {
		return errors.New("changecontract: frozen contract record requires a contract_id")
	}
	if rec.DeclaredAt.IsZero() {
		rec.DeclaredAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.byID[rec.ContractID]; ok {
		if !frozenRecordsEqual(existing, rec) {
			return fmt.Errorf("changecontract: contract_id %q is already frozen with a different payload", rec.ContractID)
		}
		return nil
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.contractsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}

	s.byID[rec.ContractID] = rec
	key := stepKey(rec.RunID, rec.CoderStepID)
	s.versionsByStep[key] = append(s.versionsByStep[key], rec)
	return nil
}

func frozenRecordsEqual(a, b FrozenContractRecord) bool {
	aj, errA := json.Marshal(a)
	bj, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(aj, bj)
}

// GetFrozenForStep returns the highest-version frozen record for
// (runID, coderStepID) whose latest status event (if any) is neither
// superseded nor abandoned — i.e. the version currently governing that
// coder step. ok=false when no active version exists (never frozen, or every
// version has been superseded/abandoned).
func (s *FrozenStore) GetFrozenForStep(runID, coderStepID string) (FrozenContractRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	versions := append([]FrozenContractRecord(nil), s.versionsByStep[stepKey(runID, coderStepID)]...)
	sort.Slice(versions, func(i, j int) bool { return versions[i].Version < versions[j].Version })
	for i := len(versions) - 1; i >= 0; i-- {
		rec := versions[i]
		if s.isActiveLocked(rec.ContractID) {
			return rec, true, nil
		}
	}
	return FrozenContractRecord{}, false, nil
}

func (s *FrozenStore) isActiveLocked(contractID string) bool {
	events := s.statusByID[contractID]
	if len(events) == 0 {
		return true // frozen, no terminal status recorded yet
	}
	switch events[len(events)-1].Status {
	case ContractStatusSuperseded, ContractStatusAbandoned:
		return false
	default:
		return true
	}
}

// ListVersionsForStep returns every frozen version for (runID, coderStepID),
// version-ascending, including superseded/abandoned ones — for audit.
func (s *FrozenStore) ListVersionsForStep(runID, coderStepID string) ([]FrozenContractRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	versions := append([]FrozenContractRecord(nil), s.versionsByStep[stepKey(runID, coderStepID)]...)
	sort.Slice(versions, func(i, j int) bool { return versions[i].Version < versions[j].Version })
	return versions, nil
}

// ListForRun returns every frozen contract record bound to runID, including
// superseded/abandoned versions. Used by audit (run-147126) to discover a
// declared contract for the run when the live topology may not resolve.
func (s *FrozenStore) ListForRun(runID string) ([]FrozenContractRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []FrozenContractRecord
	for _, rec := range s.byID {
		if rec.RunID == runID {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CoderStepID != out[j].CoderStepID {
			return out[i].CoderStepID < out[j].CoderStepID
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// AppendStatus records a lifecycle transition for an already-frozen
// contractID. Returns an error if contractID was never frozen in this store.
func (s *FrozenStore) AppendStatus(contractID, status, reason string, at time.Time) error {
	if strings.TrimSpace(contractID) == "" {
		return errors.New("changecontract: status event requires a contract_id")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.byID[contractID]; !ok {
		return fmt.Errorf("changecontract: no frozen contract with id %q", contractID)
	}

	ev := ContractStatusEvent{ContractID: contractID, Status: status, Reason: reason, At: at}
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.eventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}

	s.statusByID[contractID] = append(s.statusByID[contractID], ev)
	return nil
}
