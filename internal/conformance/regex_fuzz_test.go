package conformance

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Differential RE fuzzer: govi vs the real nvi binary in ex batch mode
// (nvi -e -s). The oracle is the nvi *editor*, not Spencer's regcomp linked as
// a library -- so re_conv (magic/nomagic, ~, \< \>) and :set extended all run.
//
//	TestRegexBREFuzz  -- default noextended (POSIX BRE)
//	TestRegexEREFuzz  -- :set extended (POSIX ERE)
//
// Controls (optional env):
//
//	GOVI_REGEX_FUZZ_N            trial count (default 300; 40 under -short)
//	GOVI_REGEX_FUZZ_SEED         rng seed (default time-based; printed on failure)
//	GOVI_REGEX_FUZZ_TIMEOUT_MS   per-side timeout (default 3000)
func TestRegexBREFuzz(t *testing.T) { runRegexFuzz(t, false) }
func TestRegexEREFuzz(t *testing.T) { runRegexFuzz(t, true) }

func runRegexFuzz(t *testing.T, extended bool) {
	t.Helper()
	oracle := FindOracle()
	if oracle == "" {
		t.Skip("no nvi oracle found")
	}
	govi, err := GoviBinary()
	if err != nil {
		t.Fatalf("GoviBinary: %v", err)
	}

	n := envInt("GOVI_REGEX_FUZZ_N", 300)
	if testing.Short() && os.Getenv("GOVI_REGEX_FUZZ_N") == "" {
		n = 40
	}
	seed := envInt64("GOVI_REGEX_FUZZ_SEED", time.Now().UnixNano())
	// 3s default: cold binary + RE error paths under load were flaking at 1.5s.
	timeout := time.Duration(envInt("GOVI_REGEX_FUZZ_TIMEOUT_MS", 3000)) * time.Millisecond
	rng := rand.New(rand.NewSource(seed))
	mode := "BRE"
	if extended {
		mode = "ERE"
	}
	t.Logf("%s fuzzer seed=%d trials=%d timeout=%s", mode, seed, n, timeout)

	var diverged int
	const maxReport = 12
	for i := 0; i < n; i++ {
		trial := genRETrial(rng, extended)
		sess := ExSession{Input: trial.input, Commands: trial.cmds}

		want := RunBatchBinaryFull(oracle, sess, timeout)
		got := RunBatchBinaryFull(govi, sess, timeout)

		if diff := reFuzzDiff(want, got); diff != "" {
			diverged++
			if diverged <= maxReport {
				t.Errorf("trial %d (seed=%d) %s\n  cmds %v\n  input %q\n  %s\n  nvi  content=%q ok=%v timeout=%v\n  govi content=%q ok=%v timeout=%v",
					i, seed, trial.note, trial.cmds, trial.input, diff,
					normalizeBuf(want.Content), want.ExitErr == nil, want.TimedOut,
					normalizeBuf(got.Content), got.ExitErr == nil, got.TimedOut)
			}
		}
	}
	if diverged > maxReport {
		t.Errorf("...and %d more divergences (seed=%d)", diverged-maxReport, seed)
	}
	if diverged > 0 {
		t.Fatalf("%d/%d %s fuzzer trials diverged (seed=%d); re-run with GOVI_REGEX_FUZZ_SEED=%d",
			diverged, n, mode, seed, seed)
	}
}

// reFuzzDiff returns a short reason the two outcomes disagree, or "" if they
// match for fuzzer purposes. An empty result means "count as match" (including
// skip-as-ok for known non-actionable cases).
func reFuzzDiff(nvi, govi BatchOutcome) string {
	if isInfraErr(nvi.ExitErr) {
		return "nvi infra: " + nvi.ExitErr.Error()
	}
	if isInfraErr(govi.ExitErr) {
		return "govi infra: " + govi.ExitErr.Error()
	}
	// nvi has crashed on some RE edge cases (e.g. bare $ under extended+ic on
	// an empty buffer: signal: segmentation fault). That is an oracle defect,
	// not a govi divergence -- skip the trial.
	if isOracleCrash(nvi.ExitErr) {
		return ""
	}
	if nvi.TimedOut != govi.TimedOut {
		return fmt.Sprintf("timeout nvi=%v govi=%v", nvi.TimedOut, govi.TimedOut)
	}
	if nvi.TimedOut && govi.TimedOut {
		return ""
	}
	nviOK := nvi.ExitErr == nil
	goviOK := govi.ExitErr == nil
	if nviOK != goviOK {
		return fmt.Sprintf("exit nvi_ok=%v govi_ok=%v nvi_err=%v govi_err=%v",
			nviOK, goviOK, nvi.ExitErr, govi.ExitErr)
	}
	if normalizeBuf(nvi.Content) != normalizeBuf(govi.Content) {
		return "buffer content differs"
	}
	return ""
}

// isOracleCrash reports an nvi process death that is not a clean non-zero exit
// (RE error / "No match found"). Those are oracle bugs; do not fail govi on them.
func isOracleCrash(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "signal:") || strings.Contains(s, "segmentation fault") ||
		strings.Contains(s, "SIGSEGV") || strings.Contains(s, "SIGABRT") ||
		strings.Contains(s, "killed")
}

// isInfraErr reports errors that are not "editor exited non-zero".
func isInfraErr(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*os.PathError); ok {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "mkdir") || strings.Contains(s, "write") ||
		strings.Contains(s, "build govi")
}

type reTrial struct {
	input string
	cmds  []string
	note  string
}

// genRETrial builds one input buffer + ex script. extended selects ERE
// (:set extended) vs BRE. POSIX named classes [[:alpha:]] are not generated:
// nvi's Spencer mishandles them and govi follows the spec (regex_diff_test.go).
func genRETrial(rng *rand.Rand, extended bool) reTrial {
	var cmds []string
	note := "magic"
	if extended {
		cmds = append(cmds, "set extended")
		note = "extended"
	}
	if rng.Intn(5) == 0 {
		cmds = append(cmds, "set nomagic")
		note += "+nomagic"
	}
	if rng.Intn(4) == 0 {
		cmds = append(cmds, "set ic")
		note += "+ic"
	}

	var pat string
	if extended {
		pat = genEREPattern(rng)
	} else {
		pat = genBREPattern(rng)
	}
	repl := genRepl(rng, pat, extended)
	delim := pickDelim(pat, repl)
	flag := ""
	if rng.Intn(2) == 0 {
		flag = "g"
	}
	cmds = append(cmds, fmt.Sprintf("%%s%c%s%c%s%c%s", delim, pat, delim, repl, delim, flag))

	return reTrial{
		input: genInput(rng),
		cmds:  cmds,
		note:  note,
	}
}

func genInput(rng *rand.Rand) string {
	lines := 1 + rng.Intn(4)
	var b strings.Builder
	for i := 0; i < lines; i++ {
		n := rng.Intn(24)
		for j := 0; j < n; j++ {
			b.WriteByte(inputAlphabet[rng.Intn(len(inputAlphabet))])
		}
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		b.WriteByte('\n')
	}
	return b.String()
}

const inputAlphabet = "abcdeABCDE0123 \t._-+*?|(){}[]^$\\<>"

func genBREPattern(rng *rand.Rand) string {
	if rng.Intn(6) == 0 {
		return randomRun(rng, 1+rng.Intn(8), patternNoiseBRE)
	}
	var b strings.Builder
	pieces := 1 + rng.Intn(5)
	for i := 0; i < pieces; i++ {
		switch rng.Intn(12) {
		case 0:
			b.WriteString(randomRun(rng, 1+rng.Intn(3), "abcdeABCDE0123"))
		case 1:
			b.WriteByte('.')
		case 2:
			b.WriteString(genClass(rng))
		case 3:
			inner := randomRun(rng, 1+rng.Intn(3), "abcde0123.")
			b.WriteString(`\(` + inner + `\)`)
		case 4:
			b.WriteString(`\<`)
			b.WriteString(randomRun(rng, 1+rng.Intn(3), "abcde"))
			if rng.Intn(2) == 0 {
				b.WriteString(`\>`)
			}
		case 5:
			b.WriteByte("abcde."[rng.Intn(6)])
			b.WriteString(genIntervalBRE(rng))
		case 6:
			b.WriteByte("abcde."[rng.Intn(6)])
			b.WriteByte('*')
		case 7:
			if rng.Intn(2) == 0 {
				b.WriteByte('^')
			} else {
				b.WriteByte('$')
			}
		case 8:
			// ERE operators that must stay literal in BRE.
			b.WriteByte("+?|"[rng.Intn(3)])
		case 9:
			b.WriteByte('\\')
			b.WriteByte("+?|(){}.*"[rng.Intn(9)])
		case 10:
			b.WriteString(`\1`)
		default:
			b.WriteString(randomRun(rng, 1+rng.Intn(2), patternNoiseBRE))
		}
	}
	if b.Len() == 0 {
		b.WriteByte('a')
	}
	s := b.String()
	s = strings.ReplaceAll(s, "[:", "[")
	return s
}

func genEREPattern(rng *rand.Rand) string {
	if rng.Intn(6) == 0 {
		return randomRun(rng, 1+rng.Intn(8), patternNoiseERE)
	}
	var b strings.Builder
	pieces := 1 + rng.Intn(5)
	for i := 0; i < pieces; i++ {
		switch rng.Intn(14) {
		case 0:
			b.WriteString(randomRun(rng, 1+rng.Intn(3), "abcdeABCDE0123"))
		case 1:
			b.WriteByte('.')
		case 2:
			b.WriteString(genClass(rng))
		case 3:
			// ERE group
			inner := randomRun(rng, 1+rng.Intn(3), "abcde0123.")
			b.WriteString("(" + inner + ")")
		case 4:
			// alternation of two short atoms
			b.WriteString(randomRun(rng, 1+rng.Intn(2), "abcde"))
			b.WriteByte('|')
			b.WriteString(randomRun(rng, 1+rng.Intn(2), "abcde"))
		case 5:
			b.WriteByte("abcde."[rng.Intn(6)])
			b.WriteString(genIntervalERE(rng))
		case 6:
			b.WriteByte("abcde."[rng.Intn(6)])
			b.WriteByte("*+?"[rng.Intn(3)])
		case 7:
			if rng.Intn(2) == 0 {
				b.WriteByte('^')
			} else {
				b.WriteByte('$')
			}
		case 8:
			// word boundary (vi layer; works under extended via re_conv)
			b.WriteString(`\<`)
			b.WriteString(randomRun(rng, 1+rng.Intn(2), "abc"))
			if rng.Intn(2) == 0 {
				b.WriteString(`\>`)
			}
		case 9:
			// BRE group syntax is literal under ERE
			b.WriteString(`\(ab\)`)
		case 10:
			// nested group + alt
			b.WriteString("a(b|c)d")
		case 11:
			// escaped ordinary (including digits: ERE has no backrefs)
			b.WriteByte('\\')
			b.WriteByte("+?|(){}1.*"[rng.Intn(10)])
		case 12:
			b.WriteString("(ab)+")
		default:
			b.WriteString(randomRun(rng, 1+rng.Intn(2), patternNoiseERE))
		}
	}
	if b.Len() == 0 {
		b.WriteByte('a')
	}
	s := b.String()
	s = strings.ReplaceAll(s, "[:", "[")
	return s
}

func genClass(rng *rand.Rand) string {
	var b strings.Builder
	b.WriteByte('[')
	if rng.Intn(4) == 0 {
		b.WriteByte('^')
	}
	n := 1 + rng.Intn(5)
	for i := 0; i < n; i++ {
		if rng.Intn(5) == 0 && i+1 < n {
			lo := byte('a' + rng.Intn(20))
			hi := lo + byte(rng.Intn(5))
			b.WriteByte(lo)
			b.WriteByte('-')
			b.WriteByte(hi)
			i++
			continue
		}
		b.WriteByte("abcdeABCDE0123._+"[rng.Intn(16)])
	}
	b.WriteByte(']')
	return b.String()
}

func genIntervalBRE(rng *rand.Rand) string {
	lo := rng.Intn(4)
	switch rng.Intn(3) {
	case 0:
		return fmt.Sprintf(`\{%d\}`, lo)
	case 1:
		return fmt.Sprintf(`\{%d,\}`, lo)
	default:
		hi := lo + rng.Intn(3)
		return fmt.Sprintf(`\{%d,%d\}`, lo, hi)
	}
}

func genIntervalERE(rng *rand.Rand) string {
	lo := rng.Intn(4)
	switch rng.Intn(3) {
	case 0:
		return fmt.Sprintf(`{%d}`, lo)
	case 1:
		return fmt.Sprintf(`{%d,}`, lo)
	default:
		hi := lo + rng.Intn(3)
		return fmt.Sprintf(`{%d,%d}`, lo, hi)
	}
}

func genRepl(rng *rand.Rand, pat string, extended bool) string {
	switch rng.Intn(6) {
	case 0:
		return ""
	case 1:
		return "X"
	case 2:
		return "&"
	case 3:
		return "X&Y"
	case 4:
		// BRE backref in replacement; ERE groups also number from 1.
		if extended && strings.Contains(pat, "(") {
			return `\1`
		}
		if !extended && strings.Contains(pat, `\(`) {
			return `\1`
		}
		return "Y"
	default:
		return randomRun(rng, 1+rng.Intn(4), "XYZ012-_")
	}
}

const patternNoiseBRE = `abcde0123.*+[?]^$(){}|<>\_-`
const patternNoiseERE = `abcde0123.*+[?]^$(){}|<>\_-`

func randomRun(rng *rand.Rand, n int, alphabet string) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(alphabet[rng.Intn(len(alphabet))])
	}
	return b.String()
}

func pickDelim(pat, repl string) byte {
	for _, d := range []byte("#!@,;:~%") {
		if !strings.ContainsRune(pat, rune(d)) && !strings.ContainsRune(repl, rune(d)) {
			return d
		}
	}
	return '#'
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envInt64(name string, def int64) int64 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}
