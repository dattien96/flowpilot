package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Phase 1 (07 plan): the Claude stream-json transport. A `claude` process speaks
// newline-delimited JSON on stdio: user-turn lines + control responses go in on
// stdin; system/assistant/user/stream_event/result/control_request lines come out on
// stdout. Unlike the Codex shared app-server, ONE claude process == ONE session, so
// there is no per-thread routing — every line belongs to the in-flight turn.
//
// Wire shapes are the ASSUMED Claude Code stream-json schema (CLI ~2.1.x); verify
// against the pinned build (07 Appendix A spike). Centralizing them here (+ in
// claude_event_mapper.go / claude_permission_mcp.go) makes a schema drift a localized
// change.

// claudeLine is one parsed stdout frame.
type claudeLine struct {
	Type    string
	Subtype string
	Raw     map[string]any
}

// claudeStream wraps a process's stdio. A single read-loop goroutine parses stdout
// frames into the lines channel; writes (user turns, control replies) are mutex-
// guarded on stdin. The read loop is the SOLE closer of lines (on EOF/error), so a
// process death drains the consumer without a hung turn. stop() lets the consumer
// abandon a still-producing process without leaking the read goroutine.
type claudeStream struct {
	w       io.Writer
	writeMu sync.Mutex

	lines    chan claudeLine
	done     chan struct{}
	stopOnce sync.Once

	errMu    sync.Mutex
	closeErr error
}

func newClaudeStream(w io.Writer) *claudeStream {
	return &claudeStream{w: w, lines: make(chan claudeLine, 256), done: make(chan struct{})}
}

// start launches the read loop, which owns closing lines.
func (s *claudeStream) start(r io.Reader) { go s.readLoop(r) }

func (s *claudeStream) readLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue // skip non-JSON log noise, keep the loop alive
		}
		typ, _ := msg["type"].(string)
		if typ == "" {
			continue
		}
		sub, _ := msg["subtype"].(string)
		select {
		case s.lines <- claudeLine{Type: typ, Subtype: sub, Raw: msg}:
		case <-s.done: // consumer abandoned the stream; stop without leaking
			return
		}
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	s.setCloseErr(fmt.Errorf("claude stream closed: %w", err))
	close(s.lines)
}

// stop signals the read loop to abandon a still-producing process (idempotent).
func (s *claudeStream) stop() { s.stopOnce.Do(func() { close(s.done) }) }

func (s *claudeStream) setCloseErr(err error) {
	s.errMu.Lock()
	if s.closeErr == nil {
		s.closeErr = err
	}
	s.errMu.Unlock()
}

func (s *claudeStream) getCloseErr() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.closeErr
}

func (s *claudeStream) writeJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.w.Write(b)
	return err
}

// writeUserTurn sends a user message on stdin (the turn input).
func (s *claudeStream) writeUserTurn(text string) error {
	return s.writeJSON(map[string]any{
		"type":               "user",
		"message":            map[string]any{"role": "user", "content": text},
		"parent_tool_use_id": nil,
	})
}

// replyControl answers a server→client control_request (permission / ask_user) so the
// engine unblocks. response carries the decision payload (behavior/answer).
func (s *claudeStream) replyControl(requestID any, response map[string]any) error {
	return s.writeJSON(map[string]any{
		"type":     "control_response",
		"response": map[string]any{"request_id": requestID, "response": response},
	})
}
