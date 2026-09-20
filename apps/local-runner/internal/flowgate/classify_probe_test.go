package flowgate

import "testing"

// CP-64 M-2 live-verification regression: the gate-sandbox live run produced
// `missing ',' in argument list` + `FAIL\t<pkg> [setup failed]` (Go load-phase
// parse errors), which the classifier missed — so the reproduce gate fell
// through to the "suite passed" wording instead of the compile-error wording.
// Every observed Go parse-failure shape must classify as a compile failure;
// assertion failures must still not.
func TestClassifySuiteOutput_GoSetupFailedShapes(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "missing comma argument list",
			output: "# cp64test\ncalc_test.go:6:18: missing ',' in argument list\nFAIL\tcp64test [setup failed]\nFAIL\n",
			want:   true,
		},
		{
			name:   "missing comma parameter list",
			output: "# goerr\na_test.go:3:25: missing ',' in parameter list\nFAIL\tgoerr [setup failed]\nFAIL\n",
			want:   true,
		},
		{
			name:   "unbalanced brace EOF",
			output: "# goerr\na_test.go:5:8: expected '}', found 'EOF'\nFAIL\tgoerr [setup failed]\nFAIL\n",
			want:   true,
		},
		{
			name:   "bad package clause",
			output: "# goerr\na_test.go:1:1: expected 'package', found packag\nFAIL\tgoerr [setup failed]\nFAIL\n",
			want:   true,
		},
		{
			name:   "setup failed tag only",
			output: "FAIL\tpkg [setup failed]\nFAIL\n",
			want:   true,
		},
		{
			name:   "assertion failure still not compile",
			output: "=== RUN   TestAdd\n    calc_test.go:9: expected 3 got 4\n--- FAIL: TestAdd (0.00s)\nFAIL\tcp64test\t0.003s\nFAIL\n",
			want:   false,
		},
		{
			name:   "green suite still not compile",
			output: "=== RUN   TestAdd\n--- PASS: TestAdd (0.00s)\nPASS\nok  \tcp64test\t0.002s\n",
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifySuiteOutput("go test -v ./...", tc.output); got != tc.want {
				t.Fatalf("ClassifySuiteOutput(go test) = %v want %v for %q", got, tc.want, tc.output)
			}
		})
	}
	// Same go marker under an unknown/wrapper runner (default branch).
	wrapped := "# goerr\na_test.go:5:8: expected '}', found 'EOF'\nFAIL\tgoerr [setup failed]\nFAIL\n"
	if !ClassifySuiteOutput("./scripts/test.sh", wrapped) {
		t.Fatal("[setup failed] must classify under the default branch too")
	}
}
