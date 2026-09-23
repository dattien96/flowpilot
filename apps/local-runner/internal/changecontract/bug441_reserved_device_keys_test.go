package changecontract

import "testing"

// BUG-441: NTFS reserves device basenames regardless of extension or letter
// case — "CON.json" still resolves to the console device on Windows. The
// BUG-427 validator only filtered illegal characters and dot segments, so
// StageHeadWrite would happily stage a Head file that cannot portably exist.
func TestBug441StageHeadWriteRejectsWindowsDeviceNames(t *testing.T) {
	reserved := []string{
		"CON", "con", "Con",
		"PRN", "AUX", "NUL", "nul",
		"COM1", "com5", "COM9",
		"LPT1", "lpt7", "LPT9",
		// First dot-segment is the device basename: "con.txt" writes
		// "con.txt.json" which NTFS still treats as CON.
		"con.txt", "COM1.anything",
	}
	dir := t.TempDir()
	for _, key := range reserved {
		if !unsafeHeadFeatureKey(key) {
			t.Fatalf("unsafeHeadFeatureKey(%q) = false, want true (Windows device name)", key)
		}
		if _, _, err := StageHeadWrite(dir, CanonicalHead{FeatureKey: key, BehaviorStatement: "x"}); err == nil {
			t.Fatalf("StageHeadWrite(%q) must reject a reserved device name", key)
		}
		// LoadHead must treat a reserved key as absent, never touch the device path.
		if _, found, err := LoadHead(dir, key); err != nil || found {
			t.Fatalf("LoadHead(%q) = found=%v err=%v, want absent", key, found, err)
		}
	}
}

// Near-miss shapes: names merely similar to device names stay valid.
func TestBug441NonDeviceKeysStillAccepted(t *testing.T) {
	allowed := []string{
		"console", "controller", "com", "com0", "com10", "lpt", "lpt0", "lpt10",
		"auxiliary", "null", "nully", "printer", "calc-core", "feature.con",
	}
	dir := t.TempDir()
	for _, key := range allowed {
		if unsafeHeadFeatureKey(key) {
			t.Fatalf("unsafeHeadFeatureKey(%q) = true, want false", key)
		}
		tmp, final, err := StageHeadWrite(dir, CanonicalHead{FeatureKey: key, BehaviorStatement: "ok"})
		if err != nil {
			t.Fatalf("StageHeadWrite(%q): %v", key, err)
		}
		if err := CommitHeadWrite(tmp, final); err != nil {
			t.Fatalf("CommitHeadWrite(%q): %v", key, err)
		}
	}
}
