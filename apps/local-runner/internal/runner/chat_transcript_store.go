package runner

// Chat transcript store (CP-59 / SD-26 §5.3, SD26-D-1): the durable
// provider-neutral chat timeline. Local implementation is one NDJSON file per
// chat (O_APPEND writer, 1 MiB-line reader mirroring LoadFlowEvents); the
// Supabase implementation lands with the Task-313 wiring commit
// (workflow_chat_events, unique(chat_id, chat_seq)).

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ChatTranscriptRecord is one durable chat-timeline entry (SD-26 §5.3).
type ChatTranscriptRecord struct {
	ChatID   string          `json:"chatId"`
	ChatSeq  int64           `json:"chatSeq"`
	LegRunID string          `json:"legRunId"`
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload"`
}

// ChatTranscriptStore is the durable chat timeline (SD26-D-1). Append is
// idempotent on (chatId, chatSeq) — duplicates are silently dropped — so
// Task-317 restore replays act as upserts.
type ChatTranscriptStore interface {
	AppendChatRecords(ctx context.Context, recs []ChatTranscriptRecord) error
	ReadChatRecords(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ChatTranscriptRecord, error)
	LatestChatSeq(ctx context.Context, chatID string) (int64, error)
}

// localFileChatTranscriptStore — one NDJSON file per chat at
// <dir>/chats/<chatId>/transcript.ndjson (SD-26 §5.3). Single-process writer;
// appends serialize on the store mutex. Corrupt trailing lines are skipped with
// no error (LoadFlowEvents malformed-line policy).
type localFileChatTranscriptStore struct {
	mu  sync.Mutex
	dir string
}

func newLocalFileChatTranscriptStore(dir string) *localFileChatTranscriptStore {
	return &localFileChatTranscriptStore{dir: dir}
}

// sanitizeChatID keeps only [A-Za-z0-9_-] so a hostile chatId can never escape
// the chats/ directory (path-traversal guard; real ids are cht_<hex>).
func sanitizeChatID(chatID string) string {
	var b strings.Builder
	for _, r := range chatID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

func (l *localFileChatTranscriptStore) chatPath(chatID string) string {
	return filepath.Join(l.dir, "chats", sanitizeChatID(chatID), "transcript.ndjson")
}

func (l *localFileChatTranscriptStore) AppendChatRecords(_ context.Context, recs []ChatTranscriptRecord) error {
	if len(recs) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	chatID := recs[0].ChatID
	existing, err := l.readAll(chatID)
	if err != nil {
		return err
	}
	seen := make(map[int64]struct{}, len(existing))
	for _, rec := range existing {
		seen[rec.ChatSeq] = struct{}{}
	}
	path := l.chatPath(chatID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, rec := range recs {
		if _, dup := seen[rec.ChatSeq]; dup {
			continue
		}
		seen[rec.ChatSeq] = struct{}{}
		line, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return f.Sync()
}

func (l *localFileChatTranscriptStore) ReadChatRecords(_ context.Context, chatID string, afterSeq int64, limit int) ([]ChatTranscriptRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	all, err := l.readAll(chatID)
	if err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ChatSeq < all[j].ChatSeq })
	out := make([]ChatTranscriptRecord, 0, len(all))
	for _, rec := range all {
		if rec.ChatSeq <= afterSeq {
			continue
		}
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, rec)
	}
	return out, nil
}

func (l *localFileChatTranscriptStore) LatestChatSeq(_ context.Context, chatID string) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	all, err := l.readAll(chatID)
	if err != nil {
		return 0, err
	}
	var latest int64
	for _, rec := range all {
		if rec.ChatSeq > latest {
			latest = rec.ChatSeq
		}
	}
	return latest, nil
}

// readAll loads the chat's records from disk. Caller holds l.mu. A missing
// file is an empty transcript, not an error; malformed lines are skipped.
func (l *localFileChatTranscriptStore) readAll(chatID string) ([]ChatTranscriptRecord, error) {
	f, err := os.Open(l.chatPath(chatID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1 MiB lines, LoadFlowEvents policy
	out := make([]ChatTranscriptRecord, 0, 64)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec ChatTranscriptRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue // malformed line: skip, keep the rest
		}
		out = append(out, rec)
	}
	return out, scanner.Err()
}

// chatTranscriptWriter is the service-side serializer (SD-26 §5.3): allocates
// the monotonic per-chat chatSeq and appends through the store under a
// dedicated mutex — never s.mu (chat-store I/O must not extend the service
// lock, SD26-S-2 lock rule). The per-chat counter seeds itself from
// LatestChatSeq on first use so a restart continues the sequence.
type chatTranscriptWriter struct {
	mu    sync.Mutex
	store ChatTranscriptStore
	seq   map[string]int64
}

func newChatTranscriptWriter(store ChatTranscriptStore) *chatTranscriptWriter {
	return &chatTranscriptWriter{store: store, seq: map[string]int64{}}
}

func (w *chatTranscriptWriter) append(ctx context.Context, recs ...ChatTranscriptRecord) error {
	if w == nil || w.store == nil || len(recs) == 0 {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	chatID := recs[0].ChatID
	last, ok := w.seq[chatID]
	if !ok {
		latest, err := w.store.LatestChatSeq(ctx, chatID)
		if err != nil {
			return err
		}
		last = latest
	}
	for i := range recs {
		last++
		recs[i].ChatSeq = last
	}
	w.seq[chatID] = last
	return w.store.AppendChatRecords(ctx, recs)
}
