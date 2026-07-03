# Fix plan: govi defects found vs the nvi correctness fixes

Handoff for a future session. These are the issues from `NVI_CORRECTNESS_FIXES.md`
that govi actually has, with root cause, exact locations, a concrete fix, and the
verification that proves it. Findings detail is in `NVI_CORRECTNESS_REVIEW.md`.

Status markers (per ~/.claude/CLAUDE.md): NEW / STARTED / CODED / TESTED / DONE.

| Item | What | Effort | Status |
|------|------|--------|--------|
| A | #32 named-register leak (also breaks repeated `@`) | small | DONE |
| B | #13 join: two spaces after `.` `?` `!` | small | DONE |
| C | #15 `taglength` ignored | small | DONE |
| D | #33 `%`/`#` filename expansion in shell-outs | medium | DONE |
| E | #8/#9/#10 `^A` on a non-word keyword | medium | DONE |

All five verified via `goterm` `go test -run TestNviFix -v .` (each target case
reports `OK`). Regressions clean: `go test ./...` passes in `nvi/govi`;
`go test -run TestDiverge .` is 0-diverged in `goterm`. The remaining goterm
`TestGoviClearsWithErase` failure is pre-existing (an nvi-oracle tcell rendering
quirk, reproduces with these changes stashed) and unrelated to these fixes.

## How to work (read first)

- The harness tests the BINARY `/Users/claude/bin/govi`, not source. After every
  edit: `cd /Users/claude/src/nvi/govi && go build -o /Users/claude/bin/govi ./cmd/govi`
- Reference editor is the FIXED nvi oracle, NOT homebrew nvi (1.81.6 is too old and
  is buggy for these very fixes): `/Users/claude/src/nvi/build.unix/vi`.
- Verify with the 3-way harness already written:
  `cd /Users/claude/src/goterm && go test -run TestNviFix -v .`
  Each case logs `govi == fixed-nvi (OK)` or `... (POSSIBLE BUG)`. A fix flips its
  case(s) to OK. The cases live in `goterm/nvifixes_probe_test.go`.
- Do not regress: after all fixes, run `go test ./...` in goterm AND the standing
  batteries `go test -run TestDiverge .` (expect 0 diverged), plus `go test ./...`
  in `nvi/govi`.

---

## A. #32 -- named-register selection leaks into the next command  [NEW]

Highest priority: this is a latent register-corruption bug, not just a macro quirk.

**Symptom.** A second `@a`/`@@` does not re-run the buffer (and can type the literal
`@a` into the buffer). More generally, after `"a<cmd>` a later delete clobbers
register a.

**Root cause (engine/vi.go).** `m.reg` is cleared ONLY inside `finishCommand`
(`m.reg = 0`, ~line 158), which is gated in `key()` by `if !m.pendingState()`
(~line 123). `pendingState()` (~line 96) returns true whenever `m.reg != 0`. So once
a register is selected it is never cleared -- it leaks into every following command.
When a macro runs (`@a` dispatches `x`), the leaked `m.reg=='a'` makes the `x` delete
write into register a, overwriting the macro with the deleted char (an `A` = append),
so the next `@a` runs garbage.

**Do NOT "fix" it by dropping `m.reg` from `pendingState()`.** After a bare `"a`
(register chosen, command not yet typed) the fields look identical to "command done"
-- so the gate cannot distinguish them. The register must be cleared at the point a
command CONSUMES it, leaving the gate alone (so a bare `"a` still waits).

**Fix.** Add a helper and clear `m.reg` at each consumption site:

```go
// consumeReg returns the selected register and clears the selection so it does
// not leak into the next command (vi: "x applies to the next command only).
func (m *vimode) consumeReg() rune { r := m.reg; m.reg = 0; return r }
```

Replace the reads at these 4 sites (verified by grep `m\.reg`):
- `engine/vi.go` `startOperator` (~line 469): `m.opReg = m.consumeReg()` (was `= m.reg`).
- `engine/viedit.go` `synthOperator` (~line 497): `reg := m.consumeReg(); m.operate(e, op, reg, mot)`.
- `engine/viedit.go` `synthLineOperator` (~line 506): same pattern.
- `engine/viedit.go` `put` (~line 259): `txt := s.regs.Get(m.consumeReg())`.

(The operator paths carry the register in `m.opReg`, which the doubled-operator and
operator+motion branches already read; clearing `m.reg` after copying is safe.)

**Verify.** `go test -run 'TestNviFix32|TestNviFixRegLeak|TestNviFixes3Way' -v .`
-- `#32 @a x3`, `#32 reg-leak`, and `#12 append` must all be `OK`. Then exercise
`"add`, `"ayyp`, `"aP`, `"ax` by hand in the harness to confirm named registers
still work.

---

## B. #13 -- join must insert two spaces after `. ? !`  [NEW]

**Symptom.** `Hello.` + `World` joins to `Hello. World`; nvi gives `Hello.  World`.
Affects both vi `J` and ex `:j`.

**Root cause.** `engine/viedit.go joinLines` (~line 408) and `engine/excmds.go
exJoin` (~line 164) build the separator as at most a single space; there is no
two-space case. nvi rule (ex/ex_join.c): if the first part's last char is in `.?!`,
insert two spaces, else one (and none if it already ends in blank or the next part
starts with `)`).

**Fix.** In both join builders, after deciding a separator is needed, choose two
spaces when the last non-blank char of the first part is `.`, `?`, or `!`:

```go
sep := []rune{' '}
if n := len(a); n > 0 && (a[n-1] == '.' || a[n-1] == '?' || a[n-1] == '!') {
    sep = []rune{' ', ' '}
}
```

Keep the existing "no separator when first part ends in blank or next part starts
with `)`" guard. Watch the cursor column: vi `J` leaves the cursor on the first
inserted space; with two spaces nvi leaves it on the FIRST of the two (re-check
`joinCol` against the oracle).

Optional nicety (rarely hit): nvi tests the next line's leading char for `)` BEFORE
stripping its leading blanks; govi strips first. Match only if you want exactness.

**Verify.** `go test -run TestNviFixes3Way -v .` -- `#13 J.` and `#13 :j.` flip to OK.
Add a `["foo?","bar"]` and a `["x","y"]` (one-space) check by hand.

---

## C. #15 -- `taglength` has no effect  [NEW]

**Symptom.** `:set taglength=4` then `:tag countXXXX` fails ("tag not found") where
nvi truncates to 4 significant chars and jumps to the `counter` tag.

**Root cause.** `engine/tags.go lookupTag` (~line 124) compares `parts[0] == name`
exactly and never reads the `taglength` option (defined in options.go, unused).

**Fix.** In `lookupTag`, read `tl := e.scr.opts.Int("taglength")`; when `tl > 0`,
compare only the first `tl` runes of both the requested name and each tag entry:

```go
func sig(s string, tl int) string {
    if tl > 0 {
        r := []rune(s)
        if len(r) > tl { return string(r[:tl]) }
    }
    return s
}
// ... if sig(parts[0], tl) == sig(name, tl) { ... }
```

Use runes, not bytes, and apply the truncation to BOTH sides (nvi truncates the tag
and the key). `tl == 0` means "all significant" (no truncation).

**Verify.** `go test -run TestNviFixTags -v .` -- `#15 taglength` flips to OK; `#16
anchored` must stay OK.

---

## D. #33 -- `%` / `#` filename expansion in shell-outs  [NEW]

**Symptom.** `:r !echo %` inserts a literal `%`; nvi inserts the current file name.

**Root cause.** `engine/shell.go` passes the command argument verbatim
(`exBang`, `filterLines`, `readFromCommand`, `writeToCommand`). No `%`/`#`
expansion exists anywhere for shell arguments.

**Fix.** Add an expander and apply it to the shell-command string before running it:
- `%` -> current file name (`e.scr.name`), `#` -> alternate file name.
- `\%` / `\#` -> literal `%` / `#` (backslash escape; strip the backslash).
- Error if `%`/`#` is used with no current/alternate name (nvi: "No filename to
  substitute for %").

Apply in the vi `!` operator path and the ex `:!`, `:[range]!`, `:r !`, `:w !`
entry points. nvi does this via argv_exp in ex/ex_argv.c; you only need the `%`/`#`
substitution, not full glob/`!!`-history expansion (do those later if wanted).

**Verify.** `go test -run TestNviFixes3Way -v .` -- `#33 r!echo%` flips to OK. Add a
`:!echo \%` check (should stay literal `%`) and `:r !echo %` content match.

---

## E. #8/#9/#10 -- `^A` cursor-word search over a non-word char  [NEW]

One root cause covers all three. Most involved item.

**Symptom.** With the cursor on a non-word char (e.g. `^`), `^A` should search for
that char as a keyword; govi instead skips forward to the nearest WORD.
- on a lone `^`: nvi finds the next `^`; govi jumps to a word.
- on `^foo`: nvi searches `^foo` (keyword starts with the non-word char); govi `foo`.

**Root cause.**
- `engine/search.go wordAt` (~line 255) scans forward over ALL non-word runes to the
  next word, so a non-word char under the cursor is never the keyword.
- `engine/search.go searchCurrentWord` (~line 283) always builds `\<word\>`.

nvi reference: `vi/vi.c v_curword` skips only WHITESPACE, always includes the char
under the cursor, then extends through `inword` chars. `vi/v_search.c v_searchw`
builds the pattern (ERE in C; govi's BRE engine supports the equivalents):
- first char in-word -> prefix `\<` ; else if it is a regex metachar -> prefix `\`.
- append the keyword (escaped).
- last char in-word -> append `\>` ; else (the non-word case) append the rear
  delimiter `\([^[:alnum:]_]\|$\)` (this is what makes repeated `^A` idempotent, #8).

**Fix.**
1. Add a keyword builder matching `v_curword`: from the cursor, skip blanks only; if
   past EOL -> "Cursor not in a word"; the keyword is the char at that position plus
   the following run of word runes. (Keep `wordAt` for tags/`^]`, or branch on
   whether the start char is a word rune.)
2. Rebuild the pattern in `searchCurrentWord`:
   - word keyword: `\<` + escaped + `\>` (current behavior).
   - non-word keyword: escaped-first-char (via `regexEscape`) + escaped-rest, then
     rear delimiter `\([^[:alnum:]_]\|$\)`.
   govi's regex parser already supports `\(` `\|` `\)`, `\<` `\>`, and `[[:alnum:]]`
   (engine/regex/parse.go, class.go), so this compiles via `compilePattern`.
3. `regexEscape` already escapes `. * [ ] ^ $ \`; confirm it covers the metachars
   that can lead a keyword (`^` is escaped -> good).

**Edge cases.** keyword that is a single non-word char must be idempotent under
repeated `^A` (the `\(...\|$\)` delimiter, not `\>`); a keyword like `^foo` must
anchor the `^` literally and end with `\>`. Backward `^A`-equivalent (if any) shares
the builder.

**Verify.** `go test -run TestNviFixes3Way -v .` -- `#8 ^A idem`, `#9 ^A nonword`,
`#10 ^A kw` flip to OK; `#11 ^A wend` must STAY OK (word-end boundaries still hold).
Spot-check `*`/`#`-style word search and `^]` tag-of-word still work.

---

## Not to do / already correct

- Do NOT chase the at-rest cursor-on-tab column (govi draws the cursor on a leading
  tab at the tab's last cell, nvi one cell further). It is cosmetic, resolves after
  any motion, and is not one of the 42 issues.
- #1 `^^D` does not abort (correct); only the `^^D`/`0^D` "remove all autoindent"
  insert forms are unimplemented -- a separate feature, not a correctness fix here.
- Everything else in the table of `NVI_CORRECTNESS_REVIEW.md` marked NOT PRESENT /
  N/A needs no work.

## Done criteria

All of `go test -run TestNviFix -v .` reports `OK`; `go test ./...` passes in both
`goterm` and `nvi/govi`; `go test -run TestDiverge .` stays 0 diverged.
