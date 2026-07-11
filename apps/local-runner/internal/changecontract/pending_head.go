// CP-55 P-5: staging a Canonical Head update at coder-gate-pass time without
// mutating the real .flowpilot/canonical/<feature_key>.json file, so a Flow
// can only ever finalize Canonical mutation at genuine terminal acceptance
// ("done"), never on an intermediate coder pass, a validation retry, or a
// review-loop continue. Mirrors FrozenStore's proven shape (Task-264/265):
// two append-only NDJSON files, a path-keyed mutex serializing writers across
// instances in-process, strict reload, and "logical last-wins status" for
// deciding whether a staged record is still pending, finalized, or abandoned.
package changecontract

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// PendingCanonicalStatusEvent.Status values.
const (
	PendingCanonicalStatusFinalized = "finalized"
	PendingCanonicalStatusAbandoned = "abandoned"
)

// PendingCanonicalRecord is one staged Canonical Head update for a
// (RunID, FeatureKey) pair — the flow-computed CanonicalHead value that WOULD
// be written to disk if this coder pass's turn were the one accepted at
// terminal Flow completion. Head carries the fully-computed update (spec/code
// drift already resolved at stage time); finalizing writes it byte-for-byte,
// it is never recomputed at finalize time.
type PendingCanonicalRecord struct {
	RunID       string        `json:"run_id"`
	FeatureKey  string        `json:"feature_key"`
	CoderStepID string        `json:"coder_step_id,omitempty"`
	Head        CanonicalHead `json:"head"`
	StagedAt    time.Time     `json:"staged_at"`
	// Seq is a monotonically increasing per-store generation number assigned
	// by Stage. It disambiguates "this (run, feature) key was re-staged after
	// a terminal status" from "this key was resolved and is still terminal" —
	// isActiveLocked compares it against the RecordSeq of the latest status
	// event, rather than trusting the latest event's Status in isolation
	// (CP-55 P-5 review finding C-1: without this, a key that is ever
	// finalized/abandoned once could never become pending again, silently
	// dropping every later re-stage for that same run+feature).
	Seq int64 `json:"seq"`
	// Signature is an HMAC-SHA256 tag (hex, first 32 chars) over every other
	// field, minted by Stage when the store was opened with a secret
	// (NewPendingCanonicalStoreWithSecret) — the same durable, workspace-
	// external secret already used to HMAC-bind the FCP prompt marker
	// (flow_context_handoff.go's runMarkerMAC). Exempting this store's own
	// bookkeeping files from gate scope-drift comparison (CP-55 P-8,
	// frozen_scope.go) means their CONTENTS are never otherwise checked before
	// finalize commits Head byte-for-byte to the real Canonical Head file —
	// without this signature, a coder's ordinary file-write tool could append
	// a raw NDJSON line here and have it trusted as if `Stage` had produced it
	// (CP-55 P-8 Claude-agent review, Critical Finding 1). A store opened
	// without a secret (the plain NewPendingCanonicalStore, used throughout
	// this package's and CP-55 P-5/P-8's own tests) never signs or verifies —
	// existing test behavior is unchanged.
	Signature string `json:"signature,omitempty"`
}

// PendingCanonicalStatusEvent is one append-only lifecycle transition for a
// staged record — accumulates over time exactly like ContractStatusEvent
// (preflight.go) does for frozen contracts.
type PendingCanonicalStatusEvent struct {
	RunID      string    `json:"run_id"`
	FeatureKey string    `json:"feature_key"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	At         time.Time `json:"at"`
	// RecordSeq is the Seq of the PendingCanonicalRecord this event resolves —
	// see PendingCanonicalRecord.Seq.
	RecordSeq int64 `json:"record_seq"`
}

func pendingCanonicalKey(runID, featureKey string) string { return runID + "\x00" + featureKey }

var (
	pendingCanonicalLocksMu sync.Mutex
	pendingCanonicalLocks   = map[string]*sync.Mutex{}
)

func pendingCanonicalLockFor(absPath string) *sync.Mutex {
	pendingCanonicalLocksMu.Lock()
	defer pendingCanonicalLocksMu.Unlock()
	if mu, ok := pendingCanonicalLocks[absPath]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	pendingCanonicalLocks[absPath] = mu
	return mu
}

// PendingCanonicalStore persists PendingCanonicalRecord/PendingCanonicalStatusEvent
// to <workspace>/.flowpilot/canonical-pending/{pending_canonical,pending_canonical_events}.ndjson.
// Append + fsync, strict reload (a corrupt line fails NewPendingCanonicalStore
// rather than being silently skipped — a Flow terminal-acceptance guarantee
// must not silently continue on a torn/ambiguous record, same reasoning as
// FrozenStore). Writes serialize across every instance built against the same
// file in this process via a path-keyed mutex.
type PendingCanonicalStore struct {
	mu          *sync.Mutex
	recordsPath string
	eventsPath  string
	byKey       map[string]PendingCanonicalRecord // last staged record per (run,feature)
	statusByKey map[string][]PendingCanonicalStatusEvent
	nextSeq     int64  // next PendingCanonicalRecord.Seq to assign in Stage
	secret      []byte // non-nil only via NewPendingCanonicalStoreWithSecret; see Signature doc
}

// pendingCanonicalStoreDir/RecordsFile/EventsFile are the single source of
// truth for where PendingCanonicalStore writes — PendingCanonicalStoreBookkeepingPaths
// (frozen_scope.go) mirrors FrozenStoreBookkeepingPaths' own idiom so a Flow
// writer's OWN staging write here is exempted from its next gate pass's
// scope-drift comparison, exactly as FrozenStore's two files already are
// (CP-55 P-8 finding: without this, a frozen writer that staged a pending
// Canonical Head update on one gate pass would see that very write reported
// as drift on its next pass — e.g. any validation-retry or review-loop retry
// for a migrated Flow — since the original CA-427 exemption only ever knew
// about FrozenStore's two files, not PendingCanonicalStore's, which did not
// exist yet when that exemption was written).
const (
	pendingCanonicalStoreDir         = "canonical-pending"
	pendingCanonicalStoreRecordsFile = "pending_canonical.ndjson"
	pendingCanonicalStoreEventsFile  = "pending_canonical_events.ndjson"
)

// NewPendingCanonicalStore opens (creating if needed) the pending-canonical
// store rooted at workspace, loading any existing records.
func NewPendingCanonicalStore(workspace string) (*PendingCanonicalStore, error) {
	dir := filepath.Join(workspace, ".flowpilot", pendingCanonicalStoreDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	recordsPath := filepath.Join(dir, pendingCanonicalStoreRecordsFile)
	eventsPath := filepath.Join(dir, pendingCanonicalStoreEventsFile)

	absRecords, err := filepath.Abs(recordsPath)
	if err != nil {
		return nil, err
	}

	s := &PendingCanonicalStore{
		mu:          pendingCanonicalLockFor(absRecords),
		recordsPath: recordsPath,
		eventsPath:  eventsPath,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.resetAndLoadLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

// NewPendingCanonicalStoreWithSecret is NewPendingCanonicalStore plus a
// caller-supplied HMAC secret: every record this store instance stages via
// Stage is signed, and every record it reads (GetPending/ListPendingForRun/
// ListPendingForRunFresh) that fails signature verification is treated as
// absent rather than trusted — closing the forgery gap described on
// PendingCanonicalRecord.Signature. Production gate/finalize/abandon call
// sites (internal/runner/gate_hook.go) always use this constructor with the
// service's own durable markerSecret; every other existing caller (this
// package's own tests, CP-55 P-5/P-8's runner tests reading a staged record
// back directly) keeps using the plain constructor and is unaffected — a nil
// or empty secret here is equivalent to the plain constructor (no signing, no
// verification), so a disk failure initializing the durable secret degrades
// to today's behavior rather than blocking every Flow.
func NewPendingCanonicalStoreWithSecret(workspace string, secret []byte) (*PendingCanonicalStore, error) {
	s, err := NewPendingCanonicalStore(workspace)
	if err != nil {
		return nil, err
	}
	if len(secret) > 0 {
		s.secret = secret
	}
	return s, nil
}

// signPendingCanonicalRecord computes rec's HMAC-SHA256 tag under secret,
// covering every field except Signature itself (which is always cleared
// before marshaling for this purpose, whatever the caller passed in).
func signPendingCanonicalRecord(rec PendingCanonicalRecord, secret []byte) (string, error) {
	rec.Signature = ""
	data, err := json.Marshal(rec)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))[:32], nil
}

// verifyPendingCanonicalRecord reports whether rec.Signature is the correct
// tag for its own other fields under secret.
func verifyPendingCanonicalRecord(rec PendingCanonicalRecord, secret []byte) bool {
	if rec.Signature == "" {
		return false
	}
	want, err := signPendingCanonicalRecord(rec, secret)
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(rec.Signature), []byte(want))
}

// resetAndLoadLocked (re)populates byKey/statusByKey/nextSeq from disk,
// discarding any prior in-memory state. Callers must hold s.mu.
func (s *PendingCanonicalStore) resetAndLoadLocked() error {
	s.byKey = make(map[string]PendingCanonicalRecord)
	s.statusByKey = make(map[string][]PendingCanonicalStatusEvent)
	s.nextSeq = 1
	if err := s.loadRecordsLocked(); err != nil {
		return err
	}
	return s.loadEventsLocked()
}

func (s *PendingCanonicalStore) loadRecordsLocked() error {
	f, err := os.Open(s.recordsPath)
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
		var rec PendingCanonicalRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return fmt.Errorf("changecontract: corrupt pending canonical record at %s:%d: %w", s.recordsPath, lineNo, err)
		}
		s.byKey[pendingCanonicalKey(rec.RunID, rec.FeatureKey)] = rec // last-wins by append order
		if rec.Seq >= s.nextSeq {
			s.nextSeq = rec.Seq + 1
		}
	}
	return scanner.Err()
}

func (s *PendingCanonicalStore) loadEventsLocked() error {
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
		var ev PendingCanonicalStatusEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return fmt.Errorf("changecontract: corrupt pending canonical status event at %s:%d: %w", s.eventsPath, lineNo, err)
		}
		key := pendingCanonicalKey(ev.RunID, ev.FeatureKey)
		s.statusByKey[key] = append(s.statusByKey[key], ev)
	}
	return scanner.Err()
}

// Stage persists rec as the latest pending update for (RunID, FeatureKey),
// overwriting (last-wins) any earlier still-pending stage for the same key —
// a coder that passes the gate more than once for the same feature (e.g.
// across a validation retry loop) simply replaces the earlier staged value,
// it does not accumulate. Idempotent when rec is byte-identical to the
// currently staged record for this key (matches FrozenStore.SaveFrozen's
// idiom) — a duplicate delivery of the same gate-pass event is a no-op, not
// a fresh write.
func (s *PendingCanonicalStore) Stage(rec PendingCanonicalRecord) error {
	if strings.TrimSpace(rec.RunID) == "" || strings.TrimSpace(rec.FeatureKey) == "" {
		return errors.New("changecontract: pending canonical record requires run_id and feature_key")
	}
	// CP-55 P-5 review finding I-5: SaveHead derives its target file from
	// Head.FeatureKey, not from FeatureKey — a mismatch would silently
	// finalize this record's status/tracking under one key while writing a
	// completely different feature's Head file. Fail closed instead.
	if headKey := strings.TrimSpace(rec.Head.FeatureKey); headKey != "" && headKey != rec.FeatureKey {
		return fmt.Errorf("changecontract: pending canonical record feature_key %q does not match head.feature_key %q", rec.FeatureKey, headKey)
	}
	if rec.StagedAt.IsZero() {
		rec.StagedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := pendingCanonicalKey(rec.RunID, rec.FeatureKey)
	if existing, ok := s.byKey[key]; ok && recordsEqual(existing, rec) {
		return nil
	}

	rec.Seq = s.nextSeq
	if len(s.secret) > 0 {
		sig, err := signPendingCanonicalRecord(rec, s.secret)
		if err != nil {
			return err
		}
		rec.Signature = sig
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.recordsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
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

	s.byKey[key] = rec
	s.nextSeq++
	return nil
}

// recordsEqual compares two records ignoring Seq and Signature (both
// internal bookkeeping fields assigned by Stage itself, never part of the
// caller-supplied identity of a duplicate delivery). A marshal error is
// treated as "not equal" only because Stage's own subsequent
// json.Marshal(rec) call surfaces the same error properly — this must never
// be read as "assume no duplicate, write again" in isolation (CP-55 P-5
// review finding M-4).
func recordsEqual(a, b PendingCanonicalRecord) bool {
	a.Seq, b.Seq = 0, 0
	a.Signature, b.Signature = "", ""
	aj, errA := json.Marshal(a)
	bj, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(aj, bj)
}

// GetPending returns the latest staged record for (runID, featureKey) if it
// is still active (no finalized/abandoned status event recorded against it).
func (s *PendingCanonicalStore) GetPending(runID, featureKey string) (PendingCanonicalRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := pendingCanonicalKey(runID, featureKey)
	rec, ok := s.byKey[key]
	if !ok || !s.isActiveLocked(key) {
		return PendingCanonicalRecord{}, false, nil
	}
	return rec, true, nil
}

// ListPendingForRun returns every still-active staged record for runID,
// across every feature key that run staged one for — sorted by FeatureKey
// for deterministic iteration. Reflects only this instance's in-memory state
// as of construction/last Reload — see ListPendingForRunFresh for a
// decision that must observe a concurrent Stage from another instance.
func (s *PendingCanonicalStore) ListPendingForRun(runID string) ([]PendingCanonicalRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listPendingForRunLocked(runID), nil
}

// ListPendingForRunFresh re-reads both NDJSON files from disk and returns the
// still-active staged records for runID, all within a single lock hold.
// ListPendingForRun only reflects this instance's original load — a Stage
// call against a *different* PendingCanonicalStore instance (e.g. a coder
// gate pass racing a Flow's terminal "done" finalize, each of which opens its
// own instance) would otherwise be invisible to a decision based on a stale
// in-memory snapshot (CP-55 P-5 review finding I-1). Callers that are about
// to make an irreversible decision (finalize, abandon) should use this
// instead of ListPendingForRun.
func (s *PendingCanonicalStore) ListPendingForRunFresh(runID string) ([]PendingCanonicalRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.resetAndLoadLocked(); err != nil {
		return nil, err
	}
	return s.listPendingForRunLocked(runID), nil
}

func (s *PendingCanonicalStore) listPendingForRunLocked(runID string) []PendingCanonicalRecord {
	var out []PendingCanonicalRecord
	for key, rec := range s.byKey {
		if rec.RunID != runID {
			continue
		}
		if s.isActiveLocked(key) {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FeatureKey < out[j].FeatureKey })
	return out
}

// isActiveLocked reports whether the currently staged record for key is
// still pending. It compares the record's own Seq against the RecordSeq of
// the latest status event for key, rather than trusting that event's Status
// on its own — otherwise a key that was ever finalized/abandoned could never
// become pending again after a later re-stage (CP-55 P-5 review finding C-1).
func (s *PendingCanonicalStore) isActiveLocked(key string) bool {
	rec, ok := s.byKey[key]
	if !ok {
		return false
	}
	// A secret-bearing store never treats an unsigned/mis-signed record as
	// active — see PendingCanonicalRecord.Signature. A store opened without a
	// secret (s.secret == nil) performs no verification at all, matching
	// every pre-existing caller's behavior unchanged.
	if len(s.secret) > 0 && !verifyPendingCanonicalRecord(rec, s.secret) {
		return false
	}
	events := s.statusByKey[key]
	if len(events) == 0 {
		return true // staged, no status recorded yet for any generation
	}
	last := events[len(events)-1]
	if last.RecordSeq < rec.Seq {
		return true // re-staged since the last recorded status — active again
	}
	switch last.Status {
	case PendingCanonicalStatusFinalized, PendingCanonicalStatusAbandoned:
		return false
	default:
		return true
	}
}

// AppendStatus records a lifecycle transition (finalized/abandoned) for an
// already-staged (runID, featureKey). Idempotent: appending the same
// terminal status again when it is already the latest status is a no-op —
// finalize/abandon retried after a crash must not error on the second call.
func (s *PendingCanonicalStore) AppendStatus(runID, featureKey, status, reason string, at time.Time) error {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(featureKey) == "" {
		return errors.New("changecontract: pending canonical status event requires run_id and feature_key")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := pendingCanonicalKey(runID, featureKey)
	rec, ok := s.byKey[key]
	if !ok {
		return fmt.Errorf("changecontract: no pending canonical record staged for run %q feature %q", runID, featureKey)
	}
	if existing := s.statusByKey[key]; len(existing) > 0 {
		last := existing[len(existing)-1]
		if last.RecordSeq == rec.Seq && last.Status == status {
			return nil // already in this terminal status for this generation — idempotent retry
		}
	}

	ev := PendingCanonicalStatusEvent{RunID: runID, FeatureKey: featureKey, Status: status, Reason: reason, At: at, RecordSeq: rec.Seq}
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

	s.statusByKey[key] = append(s.statusByKey[key], ev)
	return nil
}
