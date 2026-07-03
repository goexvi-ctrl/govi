package conformance

import "testing"

// batchOutputCases add scripts with explicit output (:p, :nu, :=) so batch
// stdout conformance covers more than message suppression.
var batchOutputCases = []struct {
	name  string
	input string
	cmds  []string
}{
	{"print-range", "a\nb\nc\n", []string{"1,2p"}},
	{"print-after-subst", "one two\n", []string{"s/one/1/", "1p"}},
	{"line-number", "a\nb\nc\n", []string{"$="}},
}

// TestExBatchBinaryConformance runs the ex conformance scripts through BOTH
// real binaries in batch mode -- `nvi -e -s FILE` vs `govi -e -s FILE`, the
// same script on stdin -- pinning govi's command line and -s batch loop
// (including message suppression) to nvi, not just the engine (which
// TestExConformance covers in-process).
func TestExBatchBinaryConformance(t *testing.T) {
	oracle := FindOracle()
	if oracle == "" {
		t.Skip("no nvi oracle found")
	}
	govi, err := GoviBinary()
	if err != nil {
		t.Fatalf("GoviBinary: %v", err)
	}

	cases := exConformanceCases
	cases = append(cases[:len(cases):len(cases)], batchOutputCases...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := ExSession{Input: tc.input, Commands: tc.cmds}

			wantBuf, wantOut, err := RunBatchBinary(oracle, sess)
			if err != nil {
				t.Fatalf("oracle: %v", err)
			}
			gotBuf, gotOut, err := RunBatchBinary(govi, sess)
			if err != nil {
				t.Fatalf("govi binary: %v", err)
			}
			if normalizeBuf(gotBuf) != normalizeBuf(wantBuf) {
				t.Errorf("cmds %v on %q:\n govi %q\n nvi  %q",
					tc.cmds, tc.input, normalizeBuf(gotBuf), normalizeBuf(wantBuf))
			}
			if gotOut != wantOut {
				t.Errorf("stdout for %v on %q:\n govi %q\n nvi  %q",
					tc.cmds, tc.input, gotOut, wantOut)
			}
		})
	}
}
