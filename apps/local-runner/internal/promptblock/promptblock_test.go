package promptblock

import "testing"

func TestTruncateUTF8ZeroAndNegativeMaxBytesReturnEmptyTruncated(t *testing.T) {
	for _, maxBytes := range []int{0, -1} {
		got, truncated := TruncateUTF8("hello", maxBytes)
		if got != "" || !truncated {
			t.Fatalf("TruncateUTF8(%d) = %q, %v; want \"\", true", maxBytes, got, truncated)
		}
	}
}

func TestTruncateUTF8StopsOnRuneBoundary(t *testing.T) {
	got, truncated := TruncateUTF8("é🙂", 3)
	if got != "é" || !truncated {
		t.Fatalf("TruncateUTF8 boundary = %q, %v; want \"é\", true", got, truncated)
	}
}
