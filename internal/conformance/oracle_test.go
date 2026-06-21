package conformance

import "testing"

// TestOracleExSmoke verifies the ex-batch oracle path end to end. It is the
// foundation the per-command conformance tests build on: once govi can run ex
// commands, a parallel RunGovi will be diffed against RunOracleEx here.
func TestOracleExSmoke(t *testing.T) {
	oracle := FindOracle()
	if oracle == "" {
		t.Skip("no nvi oracle found (set GOVI_NVI_ORACLE or install nvi)")
	}

	cases := []struct {
		name string
		sess ExSession
		want string
	}{
		{
			name: "global substitute",
			sess: ExSession{Input: "one\ntwo\nthree\n", Commands: []string{"%s/o/0/g"}},
			want: "0ne\ntw0\nthree\n",
		},
		{
			name: "delete range",
			sess: ExSession{Input: "a\nb\nc\nd\n", Commands: []string{"2,3d"}},
			want: "a\nd\n",
		},
		{
			name: "move line",
			sess: ExSession{Input: "1\n2\n3\n", Commands: []string{"1m$"}},
			want: "2\n3\n1\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RunOracleEx(oracle, tc.sess)
			if err != nil {
				t.Fatalf("oracle: %v", err)
			}
			if got != tc.want {
				t.Fatalf("oracle output mismatch:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}
