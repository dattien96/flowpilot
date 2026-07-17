package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// localDispatchStore is the SD-24 §6.2 single-writer NDJSON commit log.
// Order: validate → append → fsync → then RAM (via memoryDispatchStore afterCommit).
// In practice we mutate RAM under the same mutex after a successful fsync of the line
// that was prepared from the would-be post-state; the memory store's afterCommit
// hook fsyncs before returning success so unpersisted mutations never surface.
type localDispatchStore struct {
	*memoryDispatchStore
	dir      string
	logPath  string
	lockPath string
	lockFile *os.File
	writeMu  sync.Mutex // serializes append+fsync
	tornDrops int
}

// NewLocalDispatchStore opens (or creates) dataDir/dispatch.ndjson with an exclusive
// process lock at dataDir/dispatch.lock. Missing log on a V2 activation is handled
// by callers via GetRunProtocolVersion + empty records → repair_required.
func NewLocalDispatchStore(dataDir string) (*localDispatchStore, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	// Discard leftover compaction temp.
	tmp := filepath.Join(dataDir, "dispatch.ndjson.tmp")
	_ = os.Remove(tmp)

	lockPath := filepath.Join(dataDir, "dispatch.lock")
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := flockExclusive(lf); err != nil {
		_ = lf.Close()
		return nil, fmt.Errorf("%w: %v", ErrDispatchLocked, err)
	}

	s := &localDispatchStore{
		memoryDispatchStore: newMemoryDispatchStore(),
		dir:                 dataDir,
		logPath:             filepath.Join(dataDir, "dispatch.ndjson"),
		lockPath:            lockPath,
		lockFile:            lf,
	}
	// Disk-before-RAM: afterCommit writes the line first; memory already has the
	// mutation — for true disk-before-RAM we write in afterCommit and on failure
	// the caller sees error. Reload uses disk as authority.
	s.afterCommit = s.persistLine
	if err := s.load(); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the process lock.
func (s *localDispatchStore) Close() error {
	if s == nil {
		return nil
	}
	if s.lockFile != nil {
		_ = funlock(s.lockFile)
		err := s.lockFile.Close()
		s.lockFile = nil
		return err
	}
	return nil
}

// TornTailDrops returns how many incomplete trailing lines were discarded on load.
func (s *localDispatchStore) TornTailDrops() int {
	return s.tornDrops
}

func (s *localDispatchStore) persistLine(line dispatchLogLine) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	raw, err := json.Marshal(line)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (s *localDispatchStore) load() error {
	f, err := os.Open(s.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	// Rebuild from scratch.
	s.memoryDispatchStore = newMemoryDispatchStore()
	s.afterCommit = s.persistLine

	br := bufio.NewReader(f)
	var last partialLine
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			// Strip trailing newline for parse; incomplete last line without \n is torn.
			complete := len(line) > 0 && line[len(line)-1] == '\n'
			payload := line
			if complete {
				payload = line[:len(line)-1]
			}
			if !complete {
				// torn tail
				s.tornDrops++
				break
			}
			if len(payload) == 0 {
				continue
			}
			var ll dispatchLogLine
			if jerr := json.Unmarshal(payload, &ll); jerr != nil {
				// treat unparseable as torn/corrupt tail drop
				s.tornDrops++
				break
			}
			s.applyLine(&ll)
			last.ok = true
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	_ = last
	return nil
}

type partialLine struct{ ok bool }

func (s *localDispatchStore) applyLine(ll *dispatchLogLine) {
	if ll == nil {
		return
	}
	if ll.Seq > s.seq {
		s.seq = ll.Seq
	}
	if ll.Activation != nil {
		s.activation[ll.Activation.RunID] = ll.Activation.ProtocolVersion
	}
	if ll.RunStop != nil {
		cp := *ll.RunStop
		s.runStop[cp.RunID] = &cp
	}
	if ll.IntentClear != nil {
		s.clears[intentKeyOf(ll.IntentClear.OwnerRunID, ll.IntentClear.Key, ll.IntentClear.Gen)] = struct{}{}
	}
	if ll.Effect != nil {
		ek := effectKey(ll.Effect.RunID, ll.Effect.TurnID, ll.Effect.EffectKind)
		cp := *ll.Effect
		s.effects[ek] = &cp
	}
	if ll.Release != nil {
		ek := effectKey(ll.Release.RunID, ll.Release.TurnID, releaseEffectKind(ll.Release.DependentRunID))
		cp := *ll.Release
		s.releases[ek] = &cp
	}
	if ll.Repair != nil {
		cp := *ll.Repair
		s.repairs[cp.RunID] = &cp
	}
	if ll.Record != nil {
		k := recKey(ll.Record.RunID, ll.Record.TurnID)
		cp := *ll.Record
		s.records[k] = &cp
	}
	if ll.Envelope != nil {
		// Prefer record's ids; envelope carries them.
		k := recKey(ll.Envelope.RunID, ll.Envelope.TurnID)
		if ll.Record != nil {
			k = recKey(ll.Record.RunID, ll.Record.TurnID)
		}
		cp := *ll.Envelope
		s.envelopes[k] = &cp
	}
	if ll.Successor != nil {
		k := recKey(ll.Successor.RunID, ll.Successor.TurnID)
		cp := *ll.Successor
		s.records[k] = &cp
	}
}

// Compact rewrites the log keeping non-prunable activation/run-stop lines and
// non-terminal / unfinalized records. Crash-safe: temp → fsync → rename → dir sync.
func (s *localDispatchStore) Compact(ctx context.Context, terminalTTL time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	tmp := s.logPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cutoff := s.clock().Add(-terminalTTL)
	// Always write activations + run stops.
	for runID, ver := range s.activation {
		ll := dispatchLogLine{
			Kind: "activation", Seq: s.seq, At: s.clockStr(),
			Activation: &dispatchActivation{RunID: runID, ProtocolVersion: ver},
		}
		if st := s.runStop[runID]; st != nil {
			cp := *st
			ll.RunStop = &cp
		}
		raw, _ := json.Marshal(ll)
		if _, err := f.Write(append(raw, '\n')); err != nil {
			_ = f.Close()
			return err
		}
	}
	for k, r := range s.records {
		keep := !r.State.IsTerminal() || (r.SettleOwed && !r.SettlePhase.IsSettleFinal())
		if !keep && terminalTTL > 0 && r.UpdatedAt != "" {
			if t, e := time.Parse(time.RFC3339Nano, r.UpdatedAt); e == nil && t.Before(cutoff) {
				// prune
				continue
			}
		}
		if !keep && terminalTTL > 0 {
			// still within TTL — keep
		}
		ll := dispatchLogLine{Kind: "record", Seq: s.seq, At: s.clockStr(), Record: r}
		if env := s.envelopes[k]; env != nil {
			ll.Envelope = env
		}
		raw, _ := json.Marshal(ll)
		if _, err := f.Write(append(raw, '\n')); err != nil {
			_ = f.Close()
			return err
		}
	}
	// clears, effects, releases, repairs, resolutions (BUG-289 L1/F-11)
	for ck := range s.clears {
		ll := dispatchLogLine{
			Kind: "clear", Seq: s.seq, At: s.clockStr(),
			IntentClear: &intentClearPayload{OwnerRunID: ck.Owner, Key: ck.Key, Gen: ck.Gen},
		}
		raw, _ := json.Marshal(ll)
		if _, err := f.Write(append(raw, '\n')); err != nil {
			_ = f.Close()
			return err
		}
	}
	for _, eff := range s.effects {
		if eff == nil {
			continue
		}
		ll := dispatchLogLine{Kind: "effect", Seq: s.seq, At: s.clockStr(), Effect: eff}
		raw, _ := json.Marshal(ll)
		if _, err := f.Write(append(raw, '\n')); err != nil {
			_ = f.Close()
			return err
		}
	}
	for _, rel := range s.releases {
		if rel == nil {
			continue
		}
		ll := dispatchLogLine{Kind: "release", Seq: s.seq, At: s.clockStr(), Release: rel}
		raw, _ := json.Marshal(ll)
		if _, err := f.Write(append(raw, '\n')); err != nil {
			_ = f.Close()
			return err
		}
	}
	for _, rep := range s.repairs {
		if rep == nil {
			continue
		}
		ll := dispatchLogLine{Kind: "repair", Seq: s.seq, At: s.clockStr(), Repair: rep}
		raw, _ := json.Marshal(ll)
		if _, err := f.Write(append(raw, '\n')); err != nil {
			_ = f.Close()
			return err
		}
	}
	// resolutions are co-committed with record lines; no separate Kind today.
	// Effects/releases/repairs above close the Compact gap for L1.
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.logPath); err != nil {
		return err
	}
	// Best-effort dir sync.
	if d, err := os.Open(s.dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// MissingLogRepairRequired reports true when activation says V2 but the log file
// is missing or empty of records/activations for that run.
func (s *localDispatchStore) MissingLogRepairRequired(ctx context.Context, runID string) (bool, error) {
	ver, err := s.GetRunProtocolVersion(ctx, runID)
	if err != nil {
		return false, err
	}
	if ver < DispatchProtocolV2 {
		return false, nil
	}
	// Activation present means log had activation — OK.
	// If activation is only in memory from this process's CreatePrepared, OK.
	// Repair when session mirror says V2 but we have no activation and no records.
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.activation[runID]; ok {
		return false, nil
	}
	for _, r := range s.records {
		if r.RunID == runID {
			return false, nil
		}
	}
	return true, nil
}

// Ensure interface compliance.
var _ DispatchStore = (*localDispatchStore)(nil)

func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
