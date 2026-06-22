package conformance

import "testing"

// exConformanceCases are ex command scripts whose observable result must match
// the C nvi oracle. Regex commands (:s, :g) arrive in Phase 5.
var exConformanceCases = []struct {
	name  string
	input string
	cmds  []string
}{
	{"delete-range", "a\nb\nc\nd\n", []string{"2,3d"}},
	{"delete-dollar", "a\nb\nc\n", []string{"$d"}},
	{"delete-all", "a\nb\nc\n", []string{"%d"}},
	{"delete-count", "a\nb\nc\nd\ne\n", []string{"2d 2"}},
	{"move-end", "1\n2\n3\n", []string{"1m$"}},
	{"move-top", "1\n2\n3\n", []string{"3m0"}},
	{"move-range", "a\nb\nc\nd\n", []string{"1,2m4"}},
	{"copy-end", "1\n2\n3\n", []string{"1t$"}},
	{"copy-top", "1\n2\n", []string{"2co0"}},
	{"join-range", "a\nb\nc\nd\n", []string{"1,3j"}},
	{"yank-put", "a\nb\nc\n", []string{"1y", "$pu"}},
	{"shift-right", "a\nb\n", []string{"1>"}},
	{"shift-left", "        a\nb\n", []string{"1<"}},
	{"multi", "one\ntwo\nthree\nfour\n", []string{"1m$", "2,3d"}},
}

// TestExConformance pins govi's ex commands to nvi using the headless ex-batch
// oracle (no PTY needed).
func TestExConformance(t *testing.T) {
	oracle := FindOracle()
	if oracle == "" {
		t.Skip("no nvi oracle found")
	}

	for _, tc := range exConformanceCases {
		t.Run(tc.name, func(t *testing.T) {
			sess := ExSession{Input: tc.input, Commands: tc.cmds}

			want, err := RunOracleEx(oracle, sess)
			if err != nil {
				t.Fatalf("oracle: %v", err)
			}
			got, err := RunGoviEx(sess)
			if err != nil {
				t.Fatalf("govi: %v", err)
			}
			if normalizeBuf(got) != normalizeBuf(want) {
				t.Errorf("cmds %v on %q:\n govi %q\n nvi  %q",
					tc.cmds, tc.input, normalizeBuf(got), normalizeBuf(want))
			}
		})
	}
}
