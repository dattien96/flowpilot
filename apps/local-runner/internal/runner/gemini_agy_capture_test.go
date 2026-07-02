package runner

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStripTerminalSequences(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "agy short reply with screen clear and title",
			in:   "\x1b[?25l\x1b[2J\x1b[m\x1b[HOK\r\n\x1b]0;C:\\Users\\dat.nguyen\\AppData\\Local\\agy\\bin\\agy.exe\a\x1b[?25h",
			want: "OK",
		},
		{
			name: "multiline preserved",
			in:   "\x1b[2J\x1b[H1. Apple\r\n2. Banana\r\n3. Orange\r\n\x1b[?25h",
			want: "1. Apple\n2. Banana\n3. Orange",
		},
		{
			name: "plain text untouched",
			in:   "hello world",
			want: "hello world",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimSpace(stripTerminalSequences(tc.in))
			if got != tc.want {
				t.Fatalf("stripTerminalSequences = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsAgyCommand(t *testing.T) {
	if got := isAgyCommand(exec.Command("agy", "--print", "hi")); !got {
		t.Errorf("isAgyCommand(agy) = false, want true")
	}
	if got := isAgyCommand(exec.Command("agy.exe", "--print", "hi")); !got {
		t.Errorf("isAgyCommand(agy.exe) = false, want true")
	}
	if got := isAgyCommand(exec.Command("sh", "-c", "printf ok")); got {
		t.Errorf("isAgyCommand(sh) = true, want false (test stub must use buffered path)")
	}
	if got := isAgyCommand(nil); got {
		t.Errorf("isAgyCommand(nil) = true, want false")
	}
}

// TestGeminiAgyPrintLive exercises the real captureAgyPrint against the actual
// agy binary. It is gated behind FLOWPILOT_AGY_LIVE=1 because it spawns agy and
// makes a live model call. Windows AGY may return empty pipe output; production
// then recovers the assistant response from AGY's persisted conversation DB.
//
//	FLOWPILOT_AGY_LIVE=1 go test ./internal/runner/ -run TestGeminiAgyPrintLive -v
func TestGeminiAgyPrintLive(t *testing.T) {
	if os.Getenv("FLOWPILOT_AGY_LIVE") != "1" {
		t.Skip("set FLOWPILOT_AGY_LIVE=1 to run the live agy capture test")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	args := []string{"--print-timeout", "60s", "--add-dir", cwd, "--sandbox", "--print", "Reply with exactly the word PONG and nothing else."}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := commandContextFn(ctx, geminiBinaryName(), args...)
	cmd.Env = agyFilteredEnv(os.Environ(), nil)
	cmd.Dir = cwd

	stdout, stderr, err := captureAgyPrint(ctx, cmd)
	t.Logf("stdout=%q stderr=%q err=%v", stdout, stderr, err)
	if err != nil {
		t.Fatalf("captureAgyPrint: %v (stderr=%q)", err, stderr)
	}
	if strings.TrimSpace(stdout) == "" && runtime.GOOS != "windows" {
		t.Fatalf("captureAgyPrint returned empty stdout; the response was not captured")
	}
}
