# Fixing govi to match nvi (handoff from the goterm comparison work)

This note is for the session that will fix govi so it matches nvi. The
divergences below were found by driving govi and nvi through identical input on
identical headless terminals (the `goterm` emulator) and diffing the rendered
screen + cursor. Start here.

## Use the skill
There is a global skill `compare-govi-nvi`. It explains the harness, the
fix->verify loop, and the methodology traps. Read it (or invoke it) before
changing code. This file is the single divergence catalog; the per-finding rules
and evidence are inline below (and the appendix at the end).

## The verify loop (do this after every fix)
The harness tests the BINARY at `/Users/claude/bin/govi`, not the source. After
editing govi you MUST rebuild that binary, then re-run the relevant battery:

```sh
# rebuild the binary the harness tests (skips the slow GUI app target)
cd /Users/claude/src/nvi/govi && go build -o /Users/claude/bin/govi ./cmd/govi

# re-run the battery for what you changed and read the verdicts
cd /Users/claude/src/goterm && go test -run TestDivergeEditing -v . 2>&1 | grep -E 'DIVERGE|match|diverged'
```

A fixed case flips from `DIVERGE` to `match`. For any cursor-only difference,
re-run with `-count=2` to rule out timing noise before declaring it fixed. The
batteries are TestDiverge{Editing,Motion,Search,Ex,Paging,Registers,Structure}.

## Fix order (highest impact first)

### 1. <Esc> burst does not exit insert mode  [FIXED 2026-06-27]
Fixed in frontend/tcell/screen.go (inputKey): tcell's input parser merges a lone
ESC followed by another byte in the same read into an Alt-modified key event
(ESC x -> Alt+x; ESC ^W -> Ctrl-Alt+w; see tcell input.go inpStateEsc). vi has
no Meta bindings, so inputKey splits any Alt-modified rune event back into a
plain Escape followed by the un-Alt'd key, matching nvi. Verified: the editing
battery (incl. the cw `.` case) is now 0/15 diverged. Original report below.

When `<Esc>` is immediately followed by more bytes in the SAME read, govi stays
in insert mode and inserts the trailing bytes as literal text. nvi handles the
burst correctly. This corrupts essentially all scripted/pasted editing, and it
masquerades as many other "divergences," so fixing it first will clear several.

Reproduce (govi shows the bug, nvi does not), buffer "alpha beta gamma":
- `iZ<Esc>x`   govi -> "Zxalpha beta gamma"   nvi -> "alpha beta gamma"
- `cwX<Esc>w.` govi -> "Xw. beta gamma"       nvi -> "X X gamma"
- `cwX<Esc>j0cwY<Esc>` govi -> "Xj0cwY beta gamma" (swallows all until next Esc)

Key clue: delivering `<Esc>` in its OWN write (a pause after it) works. So the
bug is in how govi disambiguates a lone `<Esc>` from `<Esc>` as the prefix of a
Meta/arrow key sequence -- it is most likely a missing or zero key-input timeout
(nvi uses an escape/keytime timeout): when more bytes are already buffered govi
treats ESC+byte as one key instead of "Escape, then byte". Look in govi's key
input / terminal read path for where ESC starts a multi-byte key match and make
a buffered-but-unmatched ESC resolve to a plain Escape.

### 2. Paging family leaves the viewport fixed  [FIXED 2026-06-27]
Fixed in engine (vi.go ctrlKey + new pageDown/pageUp/scrollDown/scrollUp; screen.go
bottomLine/halfPage; defScroll field reset in engine.go Resize). The six commands
now scroll the viewport per nvi vs_sm_scroll: ^F/^B page by window-2 (cursor to
top/bottom of the new screen), ^D/^U by defscroll=(window+1)/2 (cursor follows
the scroll), ^E/^Y roll the view one line keeping the cursor until it hits an
edge. Verified: paging battery 7/8 -> 1/8 (only goto-off remains, that's #3).
NOTE: rebuild /Users/claude/bin/govi and re-run with `-count=1`; a test run that
overlaps the rebuild will read the stale binary and look unfixed. Original below.

`^F ^B ^D ^U ^E ^Y` move only the cursor in govi; nvi scrolls the viewport.
On a 60-line file at 12 rows (11 text rows), from line 1:
- `^F`: nvi top->010 cursor row 0;  govi top stays 001, cursor row 9.
- `^D`: nvi top->007 cursor row 0;  govi top stays 001, cursor row 5.
nvi pages a full screen by window-2 (=9 here) and a half by window/2; the half
count also differs (nvi 6, govi 5). Implement viewport scrolling for these and
place the cursor at the top of the new page (nvi behavior). The full per-command
table (original buggy govi vs nvi) is in the appendix.

### 3. `20G` (goto off-screen line) viewport placement  [FIXED 2026-06-27]
Fixed by rewriting screen.go scrollToCursor/scrollFull to follow nvi vs_refresh.c
section 6: on-screen = no scroll; within half a text screen of an edge = minimal
scroll to that edge; a farther jump snaps the file boundary to the edge if close,
otherwise centers the target (topForMiddle). Added rowsBetween helper.

### 4. `M` middle-of-screen rounding  [FIXED 2026-06-27]
Fixed in vimotion.go: M now uses `(top+bottom+1)/2` (vs_sm_position P_MIDDLE),
rounding toward the bottom on an even displayed-line count.

### 5. `:Nd` ex-delete cursor column  [FIXED 2026-06-27]
Fixed in excmds.go exDelete: keep the cursor's column (clamped) instead of
jumping to first-non-blank, matching nvi ex_delete (which sets only sp->lno;
:move/:copy explicitly set cno=0, but :d leaves it).

### 6. `:set number` gutter width  [FIXED 2026-06-27]
Fixed in display.go GutterWidth: return a fixed 8 (nvi O_NUMBER_LENGTH,
O_NUMBER_FMT "%7lu ") when numbering is on, instead of the dynamic digits+1.
Updated frontend/grid and frontend/tcell gutter tests to the 8-wide expectation.

## Status: all 13 divergences (#1-13) are FIXED. Every goterm battery is 0
diverged -- the single-command set (editing/motion/search/ex/paging/registers/
structure) and the multi-step session sets (sequences 0/10, advanced 0/18, more
0/18) -- and `go test ./...` passes. The #7-13 group below (found by aggressive
multi-step "session" testing) is the second wave, now cleared.

## Open (found by aggressive multi-step session testing) [2026-06-27]

These were found by `TestDivergeSequences`, `TestDivergeAdvanced`, and
`TestDivergeMore` in goterm/sequence_test.go: realistic sessions that mix `:`
commands, vi commands, external filters, writes, and undo, sent step-by-step with
a settle between (the thing plain unit tests and the one-shot single-command
battery miss). The reported `:w` + `!Ggoimports` + `u` flow now MATCHES nvi; these
are the residue. RECURRING TRAP: a sequence that ends in `u` restores both editors
to the identical original and HIDES a divergence in the operation under test
(#8, #10 were both masked this way); assert the post-operation state.

### 7. `u` does not undo a `:g` (global) command  [FIXED 2026-06-27]
Fixed by making undo.Log Begin/End nest (open is now a depth counter; only the
outermost End commits the group) and wrapping global() in beginChange/endChange,
so a whole :g collapses into one undo group like nvi. Verified: `:g/a/d`+`u` and
`:g/e/s//E/`+`u` both match (post-op and post-undo). Original report below.

Reproduce on sampleLines (alpha beta gamma / the quick brown fox / ...):
- `:g/a/d` then `u`   -- nvi restores all deleted lines; govi leaves them deleted.
- `:g/e/s//E/` then `u` -- nvi reverts the substitutions; govi keeps them.
The `:g` command itself is CORRECT: immediately after `:g/a/d` (or the `:g//s`)
govi's buffer matches nvi exactly. Only the subsequent `u` diverges -- govi's
undo is a no-op (or partial) for a global command. So the global op runs outside
govi's undo machinery: it is not recorded as one undoable change (nvi groups a
whole `:g` into a single undo). Same family as the original write+filter+undo
bug. Look at how `:g` applies its per-line ops vs how `u` walks the undo log.

### 8. `r<CR>` (replace char with a newline) does not split the line  [FIXED 2026-06-27]
Fixed in viedit.go replaceChar: a '\r'/'\n' replacement drops the count target
chars and splits the line (text after moves to a new line, cursor to its col 0),
instead of storing a literal CR. Verified split + undo match nvi. Original below.

Reproduce: on "alpha beta gamma", `w` to "beta", then `r` then Enter.
- nvi: replaces the `b` with a newline, splitting the line into "alpha " and
  "eta gamma" (cursor to 1,0).
- govi: inserts a literal carriage return as a control char -- the line renders
  "alpha ^M eta gamma" (cursor stays on row 0). `r` with a <CR> argument should
  break the line, not store a CR byte. Check the `r` command's handling of a
  newline/Enter as the replacement character.

### 9. `wrapmargin` (`:set wm=N`) does not auto-wrap inserted text  [FIXED 2026-06-27]
Fixed in viinsert.go maybeWrapMargin (called after each typed rune): when the
cursor's display column passes cols-wrapmargin, break the line at the last blank
before the current word, moving that word to a new line (nvi O_WRAPMARGIN).
Verified the documented 8x30/wm=5 case matches nvi exactly. Original below.

Reproduce on an empty buffer, narrow screen: `:set wm=5` then insert a long line
"the quick brown fox jumped over the lazy dog".
- nvi: inserts a real newline when the cursor crosses the wrap margin, breaking
  the buffer into two lines ("the quick brown fox" / "jumped over the lazy dog").
- govi: inserts one long buffer line; only the terminal wraps it visually. So
  govi does not implement wrapmargin (no newline injected during insert).

### 10. ex search-address ranges are a silent no-op  [FIXED 2026-06-27]
Fixed in ex.go (parseAddrBase now handles `/pat/` and `?pat?`, plus readDelimited)
and search.go (searchAddr, reusing compilePattern/searchFrom; forward skips the
rest of the current line, backward skips it, both wrap per wrapscan; a failed
search sets exParser.err so the command errors instead of silently no-op'ing).
Verified: ex-search-addr / ex-search-range / ex-rel-search all match; no-match
errors like nvi. Original report below.

Found by `TestDivergeMore` (goterm/sequence_test.go).  Ex line addresses given as
a search (`/pat/`) do nothing in govi -- the command is accepted with no error and
the buffer is unchanged; nvi performs the operation.  On sampleLines:
- `:/quick/d`            nvi deletes "the quick brown fox"; govi: no change.
- `:/alpha/,/punct/d`    nvi deletes the 4-line range; govi: no change.
- `:.,/fox/d`            nvi deletes current-line..`/fox/`; govi: no change.
Numeric and symbolic addresses already work (`:2d`, `:2,3d`, `:%s`, `:.,$`), so
the gap is specifically the `/pat/` (and presumably `?pat?`) address parser in the
ex command line.  NOTE: a test that follows the delete with `u` hides this -- undo
restores both editors to the identical original, so they "match"; assert the
post-delete state, not the post-undo state.

### 11. `(` (sentence-back) gets stuck at a sentence boundary  [FIXED 2026-06-27]
Fixed in vimotion2.go: factored sentenceStartScan out of sentenceBackOnce; if the
scanned start isn't strictly before the cursor (i.e. already at a sentence start)
step into the previous sentence and take its start, so repeated `(` keeps moving.
Verified `(` x1-4 from EOL hits col 30/20/10/0, matching nvi. Original below.

On a multi-sentence line ("One fish. Two fish! Red fish? Blue fish."), from EOL:
- one `(`:   both move to col 30 (start of "Blue fish.")  -- match.
- two `(`:   nvi col 20, govi col 30 (govi's 2nd `(` does not move).
- three `(`: nvi col 10, govi col 30 (still stuck).
So govi's `(` moves to the current sentence start but a repeated `(` will not
cross to the PREVIOUS sentence.  `)` (sentence-forward) and `{`/`}` (paragraph)
match.  Look at `(` when the cursor already sits on a sentence boundary.

### 12. insert-mode `^U` does not erase the inserted text  [FIXED 2026-06-27]
Fixed in viinsert.go: wired the (previously dead) insertEnter field in startInsert
and added insertLineErase (^U), erasing from the cursor back to insertEnter.Col on
the current line (else col 0). Verified `ofoo bar`+^U and `Axyz`+^U match nvi.

### 13. replace-mode (`R`) backspace deletes instead of restoring  [FIXED 2026-06-27]
Fixed in viinsert.go: R-mode insertRune now records each overtyped original (or a
noOrig sentinel when typing past EOL) in m.overtyped; insertBackspace in replace
mode pops it and restores the original (or deletes the appended char), stopping at
the insert start. Verified `RXXXX`+2x backspace -> "XXpha beta gamma", matches nvi.

## Inconclusive / needs a cleaner test
- insert-mode `^D` (dedent): nvi's mid-line behavior is arcane -- when ^D is typed
  after other text it can be stored as a literal `^D` rather than dedenting, so a
  naive comparison diverges without either editor being clearly "right".  `^T`
  (indent) works and matches.  Needs a test that uses ^D in proper autoindent
  context (at the start of an auto-indented line) before judging.

## Known/accepted (do NOT "fix")
- `^X` hex input: govi accepts 2, 4, or 6 hex digits; nvi only 2. Intentional
  govi extension.
- Visual mode `v`/`V`: nvi has NO visual mode (it answers "v isn't a vi command").
  govi appears to accept v/V; there is no nvi reference behavior to match, so this
  is out of scope for the comparison, not a divergence to fix.

## What already matches (don't worry about these)
Most common commands already match: x/dd/dw/D/J/r/~/cw/yyp, w/b/e/0/$/^/f/t/G/gg/
H/L/50%, /,n,N,?,*, :s/:%s/:m/:t/:N, marks, "a registers, `.` repeat, q/@ macros,
>>/<</%/==. The full verified-identical list is in the appendix.

## Appendix: detailed evidence (from the original goterm mining)

This is the per-finding evidence captured while mining; kept here so this file is
self-contained. All six findings above are now fixed; the tables below are the
original buggy-govi-vs-nvi behavior that the fixes target.

### Paging family per-command table
60-line numbered file ("001".."060") on a 12-row terminal (11 text rows):

| keys | nvi top / cursor | original govi top / cursor |
|------|------------------|----------------------------|
| `^F` (from line 1)       | top 010, cur row 0  | top 001, cur row 9  |
| `^F^F`                   | top 019, cur row 0  | top 001, cur row 10 |
| `^B` (from end)          | top 041, cur row 10 | top 050, cur row 1  |
| `^D` (from line 1)       | top 007, cur row 0  | top 001, cur row 5  |
| `^U` (from end)          | top 044, cur row 10 | top 050, cur row 5  |
| `^E` (scroll down 1)     | top 002             | top 001 (no scroll) |
| `^Y` (scroll up 1)       | top moves up 1      | no scroll           |

### M middle-of-screen evidence (12x40)
N=5 both row 2; N=6 nvi 3 / govi 2; N=7 both row 3; N=11 both row 5; N=20 both
row 5. Rule: nvi middle = `(top+bottom+1)/2` (rounds toward bottom).

### Verified-identical commands (full list)
- Editing: `x`, `3x`, `X`, `dd`, `2dd`, `D`, `dw`, `d$`, `dG`, `J`, `r`, `~`,
  `cw`, `yy`+`p`, `dd`+`p`.
- Motion: `w`, `3w`, `b`, `e`, `0`, `$`, `^`, `f`, `t`, `G`, `gg`, `H`, `M`, `L`,
  `50%`.
- Search: `/pat`, `/pat`+`n`, `/pat`+`N`, `?pat`, `*`, no-match search.
- Ex: `:Nd`, `:N,Md`, `:s`, `:%s//g`, `:Nm`, `:Nt`, `:N`, `:set number`.
- Registers/repeat: `m`+`` ` ``, `m`+`'`, `"ayy`+`"ap`, `.` after `x`, `.` after
  `dd`, `.` after `cw`, `qa..q`+`@a` macro, `"A` append-register.
- Structure: `>>`, `2>>`, `<<`, `>G`, `%` (paren and brace match), `==`.

### Notes on method
- Cursor-only divergences are re-run with `-count=2` to rule out DSR-response
  timing noise. After a govi rebuild, always re-run with `-count=1`: a cached or
  rebuild-overlapping test run reads the stale binary and looks unfixed.
- Status-line text (differing error/info messages) is treated as cosmetic and
  excluded by dropping the last screen row from the body comparison.
