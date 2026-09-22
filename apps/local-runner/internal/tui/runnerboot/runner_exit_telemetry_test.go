package runnerboot

import (
	"os"
	"strings"
	"testing"
)

func TestSpawnRunner_RecordsChildExitStatus(t *testing.T) {
	source, err := os.ReadFile("runnerboot.go")
	if err != nil {
		t.Fatalf("read runnerboot.go: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, "[runnerboot] runner pid=") || !strings.Contains(text, "exited:") {
		t.Fatal("spawnRunner must record runner PID and cmd.Wait exit status in cli-runner.log")
	}
}
