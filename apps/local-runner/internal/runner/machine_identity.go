package runner

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type machineIdentityRecord struct {
	MachineID string `json:"machine_id"`
	CreatedAt string `json:"created_at"`
}

func loadOrCreateMachineIdentity(storeDir string) (machineIdentityRecord, error) {
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return machineIdentityRecord{}, err
	}
	path := filepath.Join(storeDir, "machine.json")
	raw, err := os.ReadFile(path)
	if err == nil {
		var record machineIdentityRecord
		if jsonErr := json.Unmarshal(raw, &record); jsonErr == nil && strings.TrimSpace(record.MachineID) != "" {
			return record, nil
		}
	}

	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return machineIdentityRecord{}, err
	}
	record := machineIdentityRecord{
		MachineID: "mch_" + hex.EncodeToString(buf),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return machineIdentityRecord{}, err
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return machineIdentityRecord{}, err
	}
	return record, nil
}
