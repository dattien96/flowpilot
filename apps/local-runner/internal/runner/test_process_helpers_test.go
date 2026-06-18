package runner

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func testShellCommand(ctx context.Context, script string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", script)
}

func shellReadLine() string {
	return "IFS= read -r _ || true\n"
}

func shellOutputLine(line string) string {
	return "printf '%s\\n' " + strconv.Quote(line) + "\n"
}

func shellOutputLines(lines ...string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(shellOutputLine(line))
	}
	return b.String()
}

func seedGoogleDriveProxyReady(t *testing.T, runner *Runner) {
	t.Helper()
	stubGoogleDriveProxyLauncherAvailable(t)
	stubGoogleDriveOAuthTokenRefresh(t)
	if err := runner.ensureSecretStore().Set(googleDriveArtifactSyncClientSecretKey, "artifact-client-secret"); err != nil {
		t.Fatalf("save artifact sync client secret: %v", err)
	}
	if err := runner.ensureSecretStore().Set(googleDriveArtifactSyncPickerAPIKeySecret, "picker-api-key"); err != nil {
		t.Fatalf("save picker api key: %v", err)
	}
	writeSingleProxyArtifactConnection(t, runner, "project-1")
}
