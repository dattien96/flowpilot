package structure

import "testing"

// BUG (live runner.log 2026-09-17): gitnexus cypher answers a zero-row query
// with a bare `[]` array — parseCypherTable used to fail unmarshalling it into
// the {"markdown","error"} envelope, which failed the CP-66 knowledge distill
// bootstrap on every workspace whose index had no Process rows.
func TestParseCypherTableAcceptsBareEmptyArray(t *testing.T) {
	rows, err := parseCypherTable("[]")
	if err != nil {
		t.Fatalf("bare [] must decode as empty output, got error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %v, want empty", rows)
	}
	if rows, err = parseCypherTable("  []\n"); err != nil || len(rows) != 0 {
		t.Fatalf("whitespace-wrapped [] must be empty output too: rows=%v err=%v", rows, err)
	}
}

func TestParseCypherTableEnvelopeStillParses(t *testing.T) {
	const out = "{\n  \"markdown\": \"| p.id | p.label |\\n| --- | --- |\\n| proc_1 | HandleX |\",\n  \"row_count\": 1\n}"
	rows, err := parseCypherTable(out)
	if err != nil {
		t.Fatalf("envelope decode: %v", err)
	}
	if len(rows) != 1 || rows[0][0] != "proc_1" || rows[0][1] != "HandleX" {
		t.Fatalf("rows = %v, want [[proc_1 HandleX]]", rows)
	}
}

func TestParseCypherTableEnvelopeErrorSurfaces(t *testing.T) {
	if _, err := parseCypherTable("{\"error\":\"bad query\"}"); err == nil || err.Error() != "gitnexus cypher: bad query" {
		t.Fatalf("err = %v, want gitnexus cypher: bad query", err)
	}
}
