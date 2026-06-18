package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMachineIdentityGeneratedOnce(t *testing.T) {
	dir := t.TempDir()
	first, err := loadOrCreateMachineIdentity(dir)
	if err != nil {
		t.Fatalf("loadOrCreateMachineIdentity() first call failed: %v", err)
	}
	second, err := loadOrCreateMachineIdentity(dir)
	if err != nil {
		t.Fatalf("loadOrCreateMachineIdentity() second call failed: %v", err)
	}
	if first.MachineID == "" {
		t.Fatal("expected non-empty machine id")
	}
	if first.MachineID != second.MachineID {
		t.Fatalf("machine id changed across loads: %q vs %q", first.MachineID, second.MachineID)
	}
	if _, err := os.Stat(filepath.Join(dir, "machine.json")); err != nil {
		t.Fatalf("expected machine.json to exist: %v", err)
	}
}

func TestMachineIdentityDoesNotUsePII(t *testing.T) {
	dir := t.TempDir()
	record, err := loadOrCreateMachineIdentity(dir)
	if err != nil {
		t.Fatalf("loadOrCreateMachineIdentity() failed: %v", err)
	}
	if !strings.HasPrefix(record.MachineID, "mch_") {
		t.Fatalf("machine id = %q, want mch_ prefix", record.MachineID)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "machine.json"))
	if err != nil {
		t.Fatalf("read machine.json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal machine.json: %v", err)
	}
	for _, forbidden := range []string{"hostname", "username", "email", "providerAccountId"} {
		if _, exists := decoded[forbidden]; exists {
			t.Fatalf("machine identity must not contain %q", forbidden)
		}
	}
}
