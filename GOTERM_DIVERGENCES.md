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

## Status: divergences #1-48 are addressed. #48 (2026-06-30) FIXED: cscope
integration -- `:cscope` (`:cs`) `add`/`find`/`help`/`kill`/`reset`, `:display
c[onnections]`, and `:tagnext`/`:tagprev`/`:tagtop`/`:tagpop`. govi drives real
`cscope -dl` subprocesses and `cs find` results jump like tags (^T returns).
Verified byte-for-byte vs nvi against the database in /Users/claude/src/nvi. See
entry #48.

## Status: divergences #1-47 are addressed. #47 (2026-06-30) FIXED: split
screens -- `^W` switch, capitalized new-screen ex commands (`:E`/`:N`/`:P`/`:Vi`/
`:Tag`), `:vsplit`, `:bg`/`:fg`/`:Fg`, `:resize`, `:display s[creens]`/`b[uffers]`,
per-screen `:q`/`ZZ` close. Terminal frontend renders multi-pane with reverse
status dividers; matches nvi via goterm. GoVi.app (grid composer) now renders
multi-pane too, byte-for-byte with the terminal. See entry #47.

## Status: divergences #1-46 are addressed. #46 (2026-06-29) FIXED: file-name
arguments to `:e`/`:w`/`:r` now do nvi's argv_exp2 expansion (`%`->current,
`#`->alternate, trailing-`*` internal prefix completion, other metachars via the
shell). `:e #` used to open a file literally named `#`. Oracle-verified incl. the
per-command too-many-match errors (`:e` -> Usage, `:w`/`:r` -> "too many file
names"). See entry #46.

## Status: divergences #49-53 (the 2026-07-01 parity-review wave) are FIXED;
#54 (:display tags/buffers format) is OPEN, display-only. The review also
verified every parity.md row through the harness (see govi/docs/parity-review.md)
and fixed a harness artifact: govi's #45 flock made the second-starting editor
read-only when a comparison shared one fixture file, eating the first sent byte
at nvi's "Press any key" prompt -- goterm's runArgs now starts both editors with
EXINIT="set nolock" (locking itself stays covered by TestCoverageLock).

## Status: divergences #1-45 are addressed. #45 (2026-06-28, DATA-SAFETY) FIXED:
implemented the `lock` option (advisory `flock` on a dedicated fd, read-only
fallback when another process -- including nvi -- holds it, re-locked across the
temp+rename write so the lock survives saves) AND the readonly write-guard it
depends on, which was also missing (`:set ro` + `:w` used to write silently).
Cross-process nvi<->govi interop verified both directions.

## Status: divergences #1-44 are addressed. The SIXTH wave (work-items #40-44,
2026-06-28): #40 (vi `z+`/`z^` screen types) FIXED; #41 (`secure` now blocks the
`!` filter path -- the one ungated shell-out) FIXED, with the report's `:source`
claim corrected (nvi does not secure `:source`); #42 FIXED and upgraded -- govi
had NO overwrite guard at all (silent data loss on `:w other-existing-file`), so
nvi's "exists, not written" guard was implemented and `writeany` now bypasses it.
#43 and #44 were RE-SCOPED after verification: the cursor-follow (#44) and static
wrapped rendering (#43) already MATCH nvi on non-wrapping/common cases; the real
residual is a shared architectural gap -- govi's viewport scrolls by logical line
and lacks nvi's screen-row soft map (SMAP) for lines taller than the screen and
for `^E`/`^Y` over wrapped lines. Both are display-only (buffer always correct)
and are tracked together as the line-wrap/soft-map cluster, not small fixes.

## Status: divergences #1-39 are addressed. The FIFTH wave (roster-driven,
2026-06-28) added #37-39, all FIXED: #38 (`:una` abbreviation now resolves to
`:unabbreviate` -- min lowered from 4 to 3), #39 (`:source`/ex parser now strips
a leading `:` from command lines, nvi ex.c behavior), and #37 (the notable
unimplemented ex commands are now implemented: `:undo`, `:@`/`:*` execute a buffer
as EX commands, `:#` = `:number`, `:wn` write-and-next). Two residues under #37
are deferred: `:z`/`:display` are purely informational display commands tangled
with the unresolved info-message pagination (see "Inconclusive"), and the bare
`:*` form replicates nvi's own arcane quirk only partially -- nvi's `:*` carries
no address flag, so an address-sensitive buffer command (e.g. `d`) fails with
"address of 0"; govi runs the buffer like the useful `:@` form (the same class of
faithful-emulation gap as `2G0d(`).

## Status: divergences #1-36 are addressed. The FOURTH wave (autonomous, ex+vi
focus, 2026-06-28) #17-36 is now fixed except two explained residues: #21 (the
substitute-flag BEHAVIOR is fixed; only the multi-line usage-message pagination
remains, cosmetic) and #34 (NOT-A-BUG: nvi # is increment, the 0,0 is a
cursor-parking artifact). #32 was reopened as #36 and FIXED: the sentence motions
`(`/`)` now port nvi's character-stream engine and stop on blank-line paragraph
boundaries (the earlier WONT-FIX was wrong). Earlier waves: #1-6 single-command,
#7-13 multi-step "session", #14-16 yank/register/@. After this wave the goterm
batteries report: ex-mine 1/15 (only #21's message display), vi-mine 0/9, and every
other battery (editing/motion/search/ex/paging/registers/structure, sequences/
advanced/more) 0 diverged; `go test ./...` passes. The new ex-output overlay
(vs_msg layout) also resolves the display half of #21/#28/#33 and the
message-pagination note for print commands. One acknowledged corner remains under
#36: `2G0d(` (operator + backward sentence) hits nvi's own arcane m_start.cno
underflow and is left as a faithful-emulation gap.

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

## Open (yank/register/@-execute mining) [2026-06-27]

Found by goterm yank/put/@ probing (the numbered ring, named/append registers,
linewise vs charwise put, count-put, and @ execute-register). Most of this area
MATCHES nvi: the numbered delete ring "1.."9 (incl. that a sub-line delete does
NOT shift it), named "a / append "A registers, the unnamed register, linewise
yank/put and linewise count-put (yy3p), marks, q/@ macro record-replay, @@, and a
counted @ all match. Three real divergences remain (plus two accepted differences
recorded under "Known/accepted").

### 14. charwise p/P leaves the cursor on the wrong end of the put text  [FIXED 2026-06-28]
Fixed in viedit.go: putChars now returns the FIRST inserted position while still
leaving its internal cursor on the last char (so a counted put keeps stacking);
put() applies that first position as the final cursor for a charwise put,
matching nvi vi/v_put.c. Verified yw$p/yw$P land on the first put char. Below.

After a CHARACTERWISE put, nvi places the cursor on the FIRST character of the
inserted text; govi places it on the LAST (vim/POSIX behavior). The body is
identical; only the cursor differs, and only when the put text is >1 char (a
single-char put coincides). On "hello world":
- `yw$p`  -> "hello worldhello "  nvi cursor col 11 (first put char), govi col 16 (last).
- `yw$P`  -> "hello worlhello d"   nvi col 10 (first), govi col 15 (last).
Single-char `ylp` matches (both col 1). Linewise put already matches (cursor to
first non-blank of the first put line). Fix: for a charwise p/P, set the cursor to
the first column of the inserted text, not the last. (Note: govi matches vim/POSIX
here and nvi arguably does not -- but the project target is nvi.)

### 15. `@:` corrupts the buffer  [FIXED 2026-06-28]
Fixed in vi.go execBuffer: it now validates the buffer name (a-z, A-Z, 1-9 only,
per nvi CBNAME) and bells/no-ops for anything else, so `@:` no longer falls back
to the unnamed register. Also added @@ / @* repeat of the last executed buffer
(nvi sp->at_lbuf). Verified `@:` leaves the buffer and cursor unchanged. Below.

`@<x>` executes buffer x as vi input. `:` is not a real buffer name, so nvi treats
`@:` as a no-op (it is NOT vim's "repeat last ex command"). govi instead injects
stray text. After `:d` on regLines (deletes "AAA first", cursor on "BBB second" at
0,0), `@:`:
- nvi:  buffer unchanged ("BBB second"...), cursor 0,0  (no-op).
- govi: "BBB second" -> "BBB secondAA first", cursor 0,18  (garbage inserted).
Make `@:` (and `@` of any undefined/empty buffer) a clean no-op like nvi.

### 16. `@<reg>` of a linewise register drops the trailing-newline cursor move  [FIXED 2026-06-28]
Fixed in vi.go execBuffer: after replaying the buffer's lines, a linewise
register (nvi CB_LMODE) gets a trailing <newline> dispatched, replaying as an
Enter that moves the cursor down -- matching nvi v_at.c (which pushes a newline
after every line, including the last). Verified `"ayyj@a` ends at 3,0. Below.

Executing a yanked LINE as a macro: a linewise register ends in "\n", which nvi
replays as an Enter (cursor moves down a line); govi drops it. On cmdLines (line 1
"3xjdd"), `"ayyj@a` runs 3x / j / dd -- the edits match exactly -- then the
register's trailing newline: nvi cursor ends at 3,0, govi at 2,0. Replay the
trailing newline of a linewise register as a <CR> when executing it with @.

## Open (autonomous mining wave -- EX MODE focus, 2026-06-28)

Mined by goterm probe batteries driving `:` ex commands and comparing the rendered
screen + cursor. Ex mode is materially weaker than vi mode in govi. Findings #17+
below are OPEN. Note on method: ex commands that print (`:p` etc.) scroll output
INTO the body; a single-line print lands only on the dropped status row, so use a
multi-line range to see it. What already MATCHES: `:d`/`:m`/`:t`/`:co` (incl.
cursor line), `:y`+`:pu` content, `:j` content, ranged `:s`, `:&` (repeat subst),
`:=` (single-line, on status row), the `'a` address when the mark was set with vi
`ma`, alternate `:s` delimiters (`#`, `,`), `&`/`\1` backrefs, `\U`/`\u` case
escapes, `\r` split, `^`/`$` anchors, and empty-pattern `:s//repl/` reuse.

### 17. ex display commands `:p` `:l` `:#`/`:nu` print nothing  [FIXED 2026-06-28]
Fixed by routing printRange output through the paged overlay (presentExLines ->
pendingOutput) instead of the status line, and reworking the overlay rendering in
both frontends (tcell renderOutput, grid composeOverlay) to nvi's vs_msg layout:
the buffer stays drawn, and the output is overlaid at the BOTTOM with a "+=+="
divider (vs_divider DIVIDESTR) above it and the continue prompt on the last row.
The overlay page size is rows-1 (it reserves the divider and prompt rows), and the
divider is drawn only on the first page. Verified :1,3p / :1,3l / :1,3# all match.

`:[range]p` (print), `:l` (list, shows `$` at EOL and control chars), and
`:#`/`:nu`/`:number` (print with line numbers) display the addressed lines in nvi
(scrolled into the screen with a `+=+=` continuation separator); govi produces NO
output and just moves the cursor. On subLines (foo.../hello.../aaa...):
- `:1,3p`  nvi shows the 3 lines below a `+=+=` bar; govi: nothing.
- `:1,3l`  nvi shows "foo bar foo baz$" etc; govi: nothing.
- `:1,3#`  nvi shows "     1  foo bar foo baz" etc; govi: nothing.
Core ex output is missing. Implement the print/list/number display commands.

### 18. `:s` trailing count is ignored  [FIXED 2026-06-28]
Fixed in exSubstitute: a trailing count (parsed by splitFlagsCount) sets the range
to [addr2, addr2+count-1] before substituting. Verified :1s/o/0/g 3 changes lines
1-3 (cursor 1,0).

`:[addr]s/pat/rep/flags N` applies the substitution to N lines starting at the
addressed line (nvi/POSIX). govi ignores the count and substitutes only the one
line. `:1s/o/0/g 3` -> nvi changes lines 1-3 (cursor 1,0); govi changes only line
1 (cursor 0,0).

### 19. bare `:s` does not repeat the last substitution  [FIXED 2026-06-28]
Fixed in exSubstitute: an empty argument now repeats the last substitution via
exAmp (default range the current line), like nvi. Verified :s/foo/X/ then :s ->
"X bar X baz".

A bare `:s` (no pattern/repl) repeats the last substitution on the current line in
nvi. `:s/foo/X/` then `:s` -> nvi "X bar X baz" (second foo replaced); govi leaves
"X bar foo baz" (bare `:s` is a no-op). (`:&` already works; this is the bare form.)

### 20. `:~` (repeat subst with last RE) is a no-op  [FIXED 2026-06-28]
Fixed by adding the `~` ex command (exTilde). govi keeps a single lastPattern that
both search and substitute update, so :~ resolves to the same repeat as :& (exAmp).
Verified :s/o/0/ then j:~ changes line 2 (cursor 1,0).

`:~` repeats the last substitution but using the last-used regular expression. nvi
applies it; govi does nothing. `:s/o/0/` then `j:~` -> nvi changes the next line's
first `o`; govi leaves it unchanged. (`:&` repeats with the last subst's own
pattern and DOES work in govi; only `:~` is missing.)

### 21. `:s` with an unsupported flag silently substitutes instead of erroring  [FIXED (behavior) 2026-06-28]
The BEHAVIOR is fixed: splitFlagsCount validates the flags (c/g/r/#/l/p) and an
unknown flag (e.g. n) is a usage error that makes NO change, instead of silently
deleting the matches. The exmine case still reports DIVERGE because nvi paginates
its multi-line *usage message* into the body (with the "+=+=" divider), whereas
govi's error goes to the status line. Matching that residue would require
byte-matching nvi's exact usage-string wording and wrapping -- brittle nvi-string
chasing, and the harness treats status/message text as cosmetic -- so it is left as
the message-pagination gap (see the Inconclusive "EX INFO-MESSAGE PAGINATION" note).

nvi's valid `:s` flags are c/g/r (+ #/l/p); an unknown flag is a usage error and
makes NO change. govi accepts (ignores) the flag and performs the substitution.
`:%s/foo//n` -> nvi prints the s-command usage and changes nothing; govi deletes
the matches (empty replacement, `n` swallowed). [`n` is a vim count-only flag nvi
lacks; either reject it like nvi, or implement count-only -- but do not silently
delete.]

### 22. `:>>`/`:<<` (stacked shift) shifts only one level  [FIXED 2026-06-28]
Fixed in shift(): the repeated shift characters after the command name add levels
(:1>> = 2, :1<<< = 3), passed as the multiplier to shiftLines. Verified :1>> ->
16 leading spaces (2 x shiftwidth 8), cursor 0,16.

In nvi each repeated `>`/`<` adds a shift level: `:1>>` shifts twice, `:1<<`
unshifts twice. govi shifts exactly one level regardless of how many `>`/`<`.
`:1>>` on "foo..." -> nvi 16 leading spaces (2 x shiftwidth 8); govi 8. `:1<<` on
"\t\tabc" -> nvi "abc" (both levels removed); govi "        abc" (one removed).

### 23. ex `:k`/`:mark` commands do not set a mark  [FIXED 2026-06-28]
Fixed by adding the `k` and `mark` ex commands (exMark), which set the named mark on
the addressed line (default current). Verified :2k a then G:'a,$d deletes lines 2-5
(cursor 0,2).

`:2k a` and `:2ma a` (ex mark-set) do not set mark `a` in govi; a later `'a`
address then fails. nvi sets it and `:'a,$d` deletes from line 2 to end; govi
leaves the buffer unchanged. The `'a` ADDRESS itself is fine -- vi `2Gma` then
`:'a,$d` works in govi -- so the gap is specifically the ex `:k`/`:mark` commands.

### 24. ex `:j` (join) leaves the cursor at column 0  [FIXED 2026-06-28]
Fixed in exJoin: it tracks the last join boundary column (the inserted separator,
or the last char of the first part when joined with !) and leaves the cursor there,
like vi J. Verified :1,2j -> cursor 0,15 and :1,2j! -> 0,14.

After `:1,2j` the joined text is identical, but nvi leaves the cursor at the join
column (where the second line was appended, like vi `J`); govi leaves it at column
0. `:1,2j` -> nvi cursor 0,15, govi 0,0 (and `:1,2j!` -> nvi 0,14, govi 0,0).

### 25. ex `:>`/`:<` and `:pu` cursor lands on a different line  [OPEN, cursor-only]
After a shift or ex put, nvi leaves the cursor on the FIRST affected line; govi on
the LAST (which is POSIX: "the last line ... becomes the current line"). `:1,3>` ->
nvi cursor row 0, govi row 2. `:1,2y` then `G:pu` -> nvi on the first put line,
govi on the last. `:m`/`:t`/`:d` cursor lines already match (last line, both). This
is cursor-only and govi matches POSIX; recorded for a decision (match nvi or keep).

### 26. `:g!` deletes matching lines instead of non-matching  [FIXED 2026-06-28]
Fixed in exsearch.go: exGlobal now passes c.force as the invert flag, so :g!/:global!
runs the body on non-matching lines (same as :v). Verified ex-g-bang matches.

`:g!/re/cmd` (and `:global!`) should run cmd on lines NOT matching re. govi
ignores the `!` and behaves like plain `:g` (runs on MATCHING lines). On gLines,
`:g!/red/d` -> nvi keeps the two red lines (deletes the rest); govi deletes the two
red lines (keeps the rest). The synonym `:v/re/d` works correctly in govi, so the
non-match logic exists -- it just is not wired to the `!` on `:g`.

### 27. `:g` mistracks line numbers when its body inserts lines  [FIXED 2026-06-28]
Fixed by tracking each matched line in s.gMarks, which the line-edit primitives
(insertLine/appendLine/deleteLine in screen.go) keep in sync the same way marks
are. global() reads each match's current line from gMarks instead of a single
running delta, so a body command that inserts elsewhere (t$/copy) no longer
mistracks later matches. Verified ex-g-copy matches.

A `:g` whose per-line command ADDS lines (e.g. `t`/`copy`) operates on stale line
numbers for the second and later matches. `:g/red/t$` on gLines (red at lines 1 and
3) -> nvi appends "apple red" then "cherry red"; govi appends "apple red" then
"date brown" (the wrong line). `:g/re/d` (delete), `:g/re/s` (subst), and
`:g/re/m0` (move) all match, so the bug is specific to commands that change the
line count by INSERTING -- nvi marks all matches up front and follows the marks;
govi appears to re-resolve line numbers as the buffer grows. (Minor related: after
`:g/red/m0` the content matches but the final cursor line differs -- nvi line 4,
govi line 1.)

### 28. `:a`/`:i`/`:c` ex text-input: no "ex input mode" display  [OPEN, display; content OK]
The resulting BUFFER is correct in govi (`:2a` then "INSERTED" then "." inserts the
line in the right place). But nvi switches the screen to its line-oriented "ex
input mode" (prints "Entering ex input mode." and scrolls teletype-style), and
stays in that scrolled display afterward; govi performs the edit while keeping the
full-screen vi display. So every `:a`/`:i`/`:c` diverges on the rendered screen
though not on content. This is govi arguably behaving BETTER; recorded as a
faithful-emulation gap for a decision, not a correctness bug.

### 29. ex mode (Q) does not auto-print the current line after editing commands  [OPEN]
In true ex mode (entered with `Q`), ex prints the new current line after a command.
govi does this after a bare address/goto but NOT after editing commands. Driven
step-by-step (a single input burst does not drive nvi's line ex reader -- see the
method note below), in ex mode on gLines:
- `1<CR>` (goto)        nvi prints "apple red"; govi prints "apple red"  -- match.
- `s/apple/X/<CR>`      nvi prints "X red";     govi prints nothing.
- `m$<CR>` (move)       nvi prints the moved line; govi prints nothing.
- `2d<CR>` (delete)     nvi prints the new current line; govi prints nothing.
The edits THEMSELVES are correct in govi (a later `1,$p` shows matching buffers,
and `vi<CR>` returns both to an identical vi screen). The gap is the auto-print:
ex should echo the current line after editing commands, not only after a goto.
(Cosmetic alongside this: `Q` itself -- nvi keeps the buffer text on screen with a
"file: unmodified: line N" banner; govi blanks the screen to just a ":" prompt.)

### 30. vi-mode search with a trailing offset or `;` chain is a no-op  [FIXED 2026-06-28]
Fixed in search.go (searchLine) + engine.go (runSearchLine): a / or ? command line
is parsed for a trailing +N/-N line offset (which makes the move linewise to the
target line's first non-blank) and ;-chaining (re-search from the match, the char
after ; may flip direction). Verified search-offset (3,0) and search-chain (0,4).

A vi-mode `/pat/` search works, but appending a line offset, an end-of-match
offset, or a `;` chain makes govi ignore the search (cursor jumps to 0,0 instead of
the target). nvi supports line offsets and chaining. On subLines:
- `/aaa/+1`  nvi -> line 4 (cursor 3,0); govi -> 0,0 (no move).
- `/one/-1`  nvi -> line 3 (cursor 2,0); govi -> 0,0.
- `/foo/;/bar/`  nvi -> "bar" at 0,4; govi -> 0,0.
Implement the `/pat/+N`, `/pat/-N` line offsets and `;` search chaining for the vi
search command. NOTE: `/pat/e` and `/pat/b` (character offsets) are NOT nvi
features -- nvi rejects them with "Characters after search string, line offset
and/or z command", so do not add those; only line offsets and z/`;`.

### 31. insert mode swallows unhandled control characters  [FIXED 2026-06-28]
Fixed in viinsert.go: the insert-mode control switch now has a default that inserts
the literal control rune (via ctrlRune) instead of discarding it, so ^A/^B/^G/^K/^P
land in the buffer rendered in caret notation. Verified i^A<esc> -> "^Ax" (0,1).

nvi inserts an unhandled control character literally (rendered `^X`); govi drops
it. With buffer "x", `i<ctrl>...<esc>` for ^A/^B/^G/^K/^P all give nvi "^Ax" (etc.,
cursor 0,1) and govi "x" (cursor 0,0, char swallowed). Handled controls (^W ^U ^T
^H ^V ^M ^J ^[ and govi's ^X hex) are unaffected; this is about the REST. nvi's
behavior lets a stray control byte land in the buffer; to match, insert an
unhandled control char literally instead of discarding it.

### 32. sentence motions `(`/`)` skip a blank-line (paragraph) boundary  [FIXED 2026-06-28, see #36]
SUPERSEDED. This was first closed WONT-FIX on the belief that nvi's behavior here
was idiosyncratic, but #36's focused re-probe disproved that: nvi is consistent and
DOES stop on a blank-line paragraph boundary in every blank-crossing case. The
earlier "3G( is a no-op / 3G$( reports an out-of-range row" observations were
test-construction artifacts, not nvi inconsistency. Fixed together with #36 by
porting nvi's character-stream sentence engine; see #36 for the implementation and
the 6-case verification table.

### 33. `:args` does not display the argument list  [OPEN, display]
With two files opened (`govi f1 f2`), `:args` shows the file list with the current
file bracketed in nvi (scrolled into the body); govi shows nothing. Multi-file
editing itself WORKS: `:n` switches to f2, `:rew` back to f1, content matching nvi
exactly. Only the `:args` display is missing. (Same family as the ex-output gap
#17 and the message-pagination note below.)

### 34. `#` (backward word search) is a no-op  [NOT-A-BUG 2026-06-28]
Misdiagnosis: in nvi `#` is v_increment ([count]#[#+-], number increment/decrement),
NOT a backward word search (that is vim). `3G#x` shows nvi's own "Usage: [count]#
+|-|#". The 0,0 vs 2,0 cursor in `3G#` is a harness artifact: `#` waits for its
required +/-/# argument, and nvi parks the displayed cursor at screen-home while
awaiting a char argument (same as `3Gf` and `3Gm`, which also report 0,0). govi's #
increment already matches nvi; there is nothing to fix. Accepted.

`#` searches backward for the word under the cursor; govi does not move. `*`
(forward) agrees with nvi. On ["foo","bar","foo","baz","foo"], `3G#` -> nvi finds
the previous "foo" (cursor 0,0); govi stays at 2,0. Wire `#` to search backward for
the cursor word like `*` does forward.

### 35. operator + search motion (`d/pat`, `y/pat`) corrupts the buffer  [FIXED 2026-06-28]
Fixed in vi.go + search.go: previously a / typed while an operator was pending
aborted the operator and the pattern text ran as commands (d/baz -> b,a,z,<CR>,
splitting the line). Now commandKey defers the pending operator (searchOp), and
runSearchLine applies it as an exclusive characterwise motion from the start to the
match (a line offset makes it linewise). Verified d/baz -> "baz" (cursor 0,0).

Using a `/pat` search as the motion for an operator is broken in govi -- it does
not delete/yank from the cursor to the match; it mangles the line (and can split it
or add blank lines). nvi does it correctly. On "foo bar foo baz":
- `d/baz<CR>`  nvi -> "baz" (deletes "foo bar foo "); govi -> "fz" + a new line
  "oo bar foo baz" (corrupt split).
- `y/baz<CR>P` nvi -> "foo bar foo foo bar foo baz"; govi -> same corruption.
- `d/foo<CR>`  nvi -> "foo baz"; govi -> line unchanged plus two blank lines.
Operator + `f`/`t`/`w` motions all work, so the gap is the search-as-motion path
(related to #30: govi's non-interactive search-command handling is weak). This one
DESTROYS buffer content, so it is high priority.

### 36. RE-OPEN #32: nvi's sentence/blank-line behavior is consistent, not buggy  [FIXED 2026-06-28]
The pushback was correct. Verified by re-probing the real nvi binary with the exact
document and key sequences below (0-based screen rows); govi now matches nvi in all
six cases (and the earlier WONT-FIX of #32 is withdrawn):

| keys     | nvi          | govi before     | govi after   |
|----------|--------------|-----------------|--------------|
| `4G0(`   | row2 (blank) | row0,6 "Two."   | row2 (blank) |
| `1G$)`   | row1 (blank) | row3,0 "Three." | row1 (blank) |
| `1G$))`  | row3,0       | row3,8          | row3,0       |
| `5G0)`   | row5 (blank) | row5 (blank)    | row5 (blank) |
| `7G0(`   | row5 (blank) | row4,0          | row5 (blank) |
| `1G$)))` | row3,8 "F"   | row4,0          | row3,8 "F"   |

FIX: govi's hand-rolled sentence scanner was replaced with a faithful port of nvi's
character-stream cursor and both sentence functions (vi/getc.c cs_*, vi/v_sentence.c
v_sentencef/v_sentenceb) in engine/vimotion2.go. The port models empty lines
(whitespace-only included) as a first-class boundary (CS_EMP) that is its own
sentence stop, which is exactly what the old code missed -- its period-state
whitespace-skip walked straight across blank lines. The nvi VM_LMODE rule (a
sentence motion cuts whole lines when the cursor starts in column 0 and the motion
ended on a line boundary) is carried via a new motion.endFlag and applied in
engine/viedit.go, which keeps `d)`/`y)` line-vs-char behavior matching nvi.

Verification: all ex/vi conformance and goterm batteries pass; vi-mine is now 0/9
diverged (was 2/8). One acknowledged residue is NOT fixed and is nvi's own arcane
edge: `2G0d(` (operator + backward sentence where the cut range underflows
m_start.cno in nvi's own code) deletes an extra line in nvi; govi keeps it. The
PLAIN motions and the well-defined operator cases all match; this single
operator+merge corner is left as a faithful-emulation gap, not chased into nvi's
documented inconsistency.

--- original analysis (retained) ---
#32 (`(`/`)` skip a blank-line paragraph boundary) was closed as WONT-FIX on the
grounds that nvi's "sentence-across-blank" behavior is inconsistent/buggy. A
focused re-test does not support that: across 12 cases -- single blanks, runs of
two blank lines, a line with no ending punctuation, both `(` and `)`, forward and
backward -- nvi was CONSISTENT every time. It treats a blank-line paragraph
boundary as a sentence boundary and stops on it; a run of consecutive blanks
collapses to one stop. govi is the one that varies from nvi: it skips the blank and
lands on the adjacent sentence. Evidence (doc: "One.  Two." / "" / "" /
"Three.  Four." / "no period here" / "" / "Five."):

| keys      | nvi lands         | govi lands        |
|-----------|-------------------|-------------------|
| `4G0(`    | blank (row 2)     | "Two." (0,6)      |
| `1G$)`    | blank (row 1)     | "Three." (3,0)    |
| `1G$))`   | "Three." (3,0)    | "Three." col 8    |
| `5G0)`    | blank (row 5)     | blank (row 5) *   |
| `7G0(`    | blank (row 5)     | "no period" (4,0) |
| `1G$)))`  | "Four." (3,8)     | (4,0)             |

(* the no-punctuation-line case happens to agree.) In every blank-crossing case nvi
stops on the blank and govi does not. This matches the POSIX definition -- "a
sentence is bounded by a '.', '!', or '?' ... or by a paragraph or section
boundary" -- so nvi is spec-correct, and the divergence is govi's. Recommendation:
re-open #32 and make `(`/`)` stop at a blank-line (paragraph) boundary. If there is
a specific nvi inconsistency that motivated the WONT-FIX, capture that exact repro
here; I could not find one. (Note: this is about matching nvi, which is the project
target, regardless of how vim -- which does NOT stop on blank lines -- behaves.)

## Open (roster-driven mining: every :viusage/:exusage command, 2026-06-28)

Used the captured nvi/govi `:viusage` and `:exusage` rosters as a test matrix and
drove every command in both editors (NOT comparing the usage text itself -- that is
just reference). Almost everything matched: vi `^A` (forward cursor-word search),
`[[`/`]]` (sections), `_`, `^N`/`^P`/`^M`/`^J`, `!`-to-motion, and ex `:source`,
`:p`/`:nu`/`:l`, `:cd`, `:file`, plus -- importantly -- `:q`/`:x` on a MODIFIED
buffer correctly REFUSE in govi (no data loss; only the warning's screen rendering
differs, which is the known message-pagination note). The residue is one cluster of
unimplemented ex commands (#37) and one undocumented-but-working vi command (noted
under "Undocumented but functional").

### 37. several ex commands nvi lists in :exusage are no-ops in govi  [FIXED 2026-06-28, :z deferred; :display done in #47]
FIX: implemented the data-affecting and trivial commands. `:undo` (ex.go table +
exUndo, sharing the vi-mode undo/redo direction toggle, nvi ex_undo.c). `:@`/`:*`
(parseName now accepts `@`; exAt executes the named buffer's lines as EX commands,
defaulting to the last-executed buffer for `@@`/`@*`, nvi ex_at.c). `:#` registered
as a synonym for `:number`. `:wn` (exWriteNext: write then advance the arg list).
Verified vs real nvi (goterm coverage): `:@a` matches, `:undo` matches, `:1,3#`
matches, `:wn` writes + advances (data identical; the multi-line info message
renders via the known pagination path). The `:*` bare form is NOT made to match:
nvi's C_STAR table entry has no address flag, so a bare `:*` does not default the
address and an address-sensitive buffer command (`d`) errors with "address of 0"
-- an arcane nvi quirk, like the `2G0d(` underflow, so govi runs the buffer like
`:@`. DEFERRED: `:z` (screenful display) and `:display` (buffer/mark/screen/tag
lists) are purely informational and route through the same info-message display
that cannot be cleanly diffed here yet (see "Inconclusive").

--- original analysis (retained) ---
Driving each by its behavior (not its absence from govi's :exusage) shows these do
nothing in govi while nvi acts:

| command            | nvi                                   | govi        |
|--------------------|---------------------------------------|-------------|
| `:@x` / `:*x`      | execute buffer x as ex commands       | no-op       |
| `:undo`            | undo last change                      | no-op       |
| `:#` (range)       | print with line numbers (= `:number`) | no-op       |
| `:z` (e.g. `:10z`) | display a screenful around a line      | no-op       |
| `:display ...`     | show buffers / marks / screens / tags  | no-op       |
| `:wn`              | write current file AND go to next      | writes only |

Notes / priority: `:undo` and `:@`/`:*` are the notable ones -- govi has the vi-mode
equivalents (`u`, and `@x` executes a buffer as vi KEYS) but not the EX forms (`:@`
executes the buffer as EX COMMANDS, a different and useful thing). Repro for `:@`:
buffer line 1 = "d", `"ayyj:@a` -> nvi deletes the current line (ran "d" as an ex
command), govi leaves it. `:#` is just the `#` synonym for `:number` (`:nu`/`:number`
already work, so this is a one-line alias). `:z`/`:display` are niche display
commands. `:wn` writes correctly but does not advance to the next file. None of
these touch data incorrectly; they simply do nothing.

### 38. `:una` (abbreviation of `:unabbreviate`) is not recognized  [FIXED 2026-06-28]
FIX: lowered the `unabbreviate` entry's `min` from 4 to 3 in the ex command table
(ex.go), matching nvi's `una[bbrev]` and govi's own `:exusage` text. No collision
with `unmap` (different prefix). Verified vs real nvi (goterm cov ex-unabbrev):
`:ab zz zebra` then `:una zz` now removes the abbreviation in both.

--- original analysis (retained) ---
The full command works but its standard abbreviation does not resolve, so the
abbreviation it should remove stays active.
- Repro: `:ab zz zebra<CR>` then `:una zz<CR>` then `ozz <Esc>`.
  - nvi: the line reads `zz ` -- `:una` removed the abbreviation.
  - govi: the line reads `zebra ` -- `:una` did nothing; the abbreviation still fires.
- `:unabbreviate zz` (spelled out) DOES work in govi (the abbrev is removed), and
  `:ab` itself works, so this is purely a missing entry in govi's ex-command
  abbreviation table: `una` -> `unabbreviate`. (Contrast `:unmap`, which resolves
  fine.) Found by the coverage sweep (goterm `TestCoverageEx` -> `ex-unabbrev`).

### 39. `:source` skips lines that begin with a leading `:`  [FIXED 2026-06-28]
FIX: the ex parser (parseEx in ex.go) now strips a run of leading `:` characters
(and surrounding blanks) before parsing the command, matching nvi ex.c ("any
command could have preceding colons"). This covers sourced scripts and inline
idioms like `:g/re/:p`. Verified vs real nvi (goterm cov so-colon): a script of
`:%s/a/A/g` and `:2d` now runs in both.

--- original analysis (retained) ---
nvi tolerates an optional leading `:` on each command in a sourced ex script (a
common idiom); govi silently ignores any such line.
- Repro: source a file whose two lines are `:%s/a/A/g` and `:2d` over buffer
  `alpha/beta/gamma`.
  - nvi: runs both -> `AlphA` / `gAmmA` (line 2 deleted).
  - govi: buffer unchanged (both lines skipped).
- With the SAME commands written WITHOUT the leading colon (`%s/a/A/g`, `2d`),
  govi runs them correctly and matches nvi. So `:source` works; it just does not
  strip a leading `:` from script lines. Found by the coverage sweep (goterm
  `TestCoverageSource`).

## Open (small addressable parity gaps from parity-gaps-report, 2026-06-28)

These are the LOW-EFFORT items from the parity.md gap review whose supporting
machinery already exists in govi. Each was re-verified against real nvi through the
goterm harness on 2026-06-28 (so they are current, not stale). Do them with the
usual verify loop (rebuild /Users/claude/bin/govi, then the named goterm probe).
NOTE: two report items turned out ALREADY DONE on re-check and are NOT listed here
-- ex `:k`/`:ma`/`:mark` set a mark (works; parity.md row was stale) and insert
`^U` line-erase (works) -- so don't re-implement them.

### 40. vi `z` lacks the `z^` and `z+` screen types  [FIXED 2026-06-28]
FIX: split `+` out of the `z<CR>` case and added `^` in screenPosition (vi.go),
threading the vimode through so the bare forms reuse pageDown/pageUp. A bare `z+`
scrolls forward one screen and a bare `z^` backward one screen, each by `t_rows`
(the full text-row count) -- nvi's Z_PLUS/Z_CARAT scroll a full screen, unlike
^F/^B which scroll window-2. `[line]z+` still puts the line at the top (= z<CR>),
`[line]z^` puts it at the bottom. Verified vs real nvi (12x40, 40 lines): `20Gz+`
-> top 026 cursor 0,0 and `20Gz^` -> top 004 cursor row 10, both matching nvi
exactly (and distinct from `^F` top 024 / `^B` top 006). The historic
"previous-screen" off-by-one of the rare `[line]z^` line form is not replicated.

--- original analysis (retained) ---
govi implements `z<CR>` (line to top), `z.` (center), `z-` (bottom), `[line]z`,
and `z[count]`, but ignores the `+`/`^` type modifiers and falls back to
top-positioning.
- Rule (nvi vs_z / ex_z): `z+` displays the NEXT screenful (like one `^F` measured
  from the z line); `z^` displays the PREVIOUS screenful (like one `^B`).
- Repro (12x40, numberedLines(40)): `20Gz+` -> nvi puts 026 at the top row; govi
  leaves 020 at the top. `20Gz^` -> nvi tops at 004; govi tops at 015.
- Fix: add the two type cases to govi's `z` dispatch, reusing the window-2 paging
  math already built for `^F`/`^B` (the #2 paging fix). Small.

### 41. `secure` does not block `!` filters (and `:r!`/`:w!`/`:source`)  [FIXED 2026-06-28]
FIX: added the secure gate to `filterLines` (shell.go), the shared body of the vi
`!` operator and `:[range]!cmd`. That was the one ungated shell-out path:
`:shell`, `:!cmd` (no range), `:r !cmd`, `:w !cmd`, and suspend/stop were ALREADY
gated. Verified vs real nvi: `:set secure` then `:%!sort` now leaves the buffer
UNSORTED in both (filter refused).

CORRECTION to the original report: `:source` is NOT secured in nvi. nvi's
E_SECURE commands (ex_cmd.c) are exactly `!`, `perl`, `perldo`, `script`,
`shell`, `stop`, `suspend`, `tcl`; `:r !`/`:w !` are gated separately via
O_SECURE in ex_read.c/ex_write.c. `:source` carries no secure flag, and verified
against the binary `:set secure` then `:source <script>` still runs the script in
both editors. So govi correctly does NOT gate `:source` (nor does it gate perl/
tcl/script, which govi does not implement at all). govi's message lacks nvi's
trailing period, matching govi's other secure-mode messages (pre-existing style).

--- original analysis (retained) ---
`:set secure` in govi only blocks `:shell`; every other shell-out path still runs.
This is a security-correctness gap, so it ranks above its size.
- Rule (nvi): `secure` disables ALL shell escapes -- `!` filters (`!motion`,
  `:[range]!cmd`), `:read !cmd`, `:write !cmd`, and `:source` of scripts -- in
  addition to `:shell`. (`secure` is also `OPT_NOUNSET`: once on it cannot be
  turned off; govi already honors that.)
- Repro (24x80): `:set secure\r:%!sort\r` -> nvi leaves the buffer UNSORTED
  (filter refused); govi SORTS it (filter ran).
- Fix: route the `!`/`:r !`/`:w !`/`:source` entry points through the same security
  gate that already guards `:shell`, returning nvi's "not allowed in secure mode"
  style error. Small/medium.

### 42. `writeany` (wa) is inert -- write safety check not gated on it  [FIXED 2026-06-28]
CORRECTION + FIX: the premise was wrong -- govi had NO overwrite safety check at
all, so `:w other-existing-file` silently CLOBBERED it (data loss), where nvi
refuses. Implemented nvi's guard (common/exf.c file_write) in exWrite: when the
write is not forced (`:w!`) and `writeany` is off, refuse to write a file that
already exists and is not the buffer's own file (or whose name was changed via
:f), with "exists, not written; use ! to override". This also makes `writeany`
do its job (bypass the guard) and is inherited by :wq/:x/:wn (they call exWrite).
Verified vs real nvi: `:w b` (b exists) refuses and preserves b in both; `:w! b`
and `:set wa` + `:w b` both overwrite in both; a plain self-`:w` still saves.

--- original analysis (retained) ---
govi performs the ownership/permission safety check on `:write` (the `exrc`
ownership machinery proves the check exists) but does not let `writeany` bypass it.
- Rule (nvi): with `set writeany`, a write skips the "file exists / not owner /
  permission" guard that otherwise requires `!`.
- Verify: cannot be a screen diff -- use a file the editor would normally refuse to
  overwrite (e.g. an existing file written without `!`), set `wa`, and confirm the
  write now succeeds without `!`, matching nvi. (Manual / filesystem check, not a
  goterm body diff.)
- Fix: gate the existing write-safety check on the `writeany` option. Small. Pairs
  naturally with the #41 secure work and the autowrite/backup write-path cluster.

### 43. line wrap: no `@`-fill for a too-tall bottom logical line  [OPEN, architectural -- soft-map cluster with #44]
RE-SCOPED after verification 2026-06-28. govi's STATIC wrapped rendering already
matches nvi in the common cases probed (a long line wrapping to several rows at
the bottom shows the same rows and a blank/`~` tail in both; scrolling short
lines past it matches). The `@`-fill did NOT reproduce there. The behavior that
DOES diverge is the same soft-map gap as #44: when a logical line is taller than
the whole screen, nvi keeps prior context and the cursor on its file line at a
screen-row offset, e.g. (12x40, line 2 ~604 chars) `2G` -> nvi keeps "top line"
at row 0 and the huge line from row 1 (cursor 1,0), while govi scrolls the huge
line to row 0 (cursor 0,0); `2Gj` -> nvi cursor row 10, govi 0,0. nvi's `@`
continuation fill is one face of that screen-row (SMAP) machinery.
- Cosmetic/positional only: buffer content is always correct in govi.
- Fix: shares the sub-line viewport offset rework described in #44 (renderer +
  scrollToCursor + paging). Tracked together with #44 and the "Inconclusive:
  LONG-LINE WRAP" note as the line-wrap/soft-map cluster; not a standalone small
  fix.

### 44. `^E`/`^Y` cursor-follow is simplified  [OPEN, architectural -- wrapped lines only; non-wrap verified MATCHING]
RE-SCOPED after verification 2026-06-28. The cursor-follow rule the entry blamed
is actually CORRECT: on a non-wrapping file, govi matches nvi EXACTLY for `^E`
and `^Y` across single steps, counts (`3^E`, `5^E`, `3^Y`), the EOF clamp, the
last line, and column maintenance (`$`, `ll`), including the boundary frame where
the cursor reaches the top/bottom row and then sticks (e.g. 12x40, 40 lines:
`30G` then six `^Y` -> top 025..019, cursor row 5->10 then held at 10 -- identical
in both). So scrollDown/scrollUp are fine.

The REAL residual divergence is granularity on WRAPPED long lines: nvi's `^E`/`^Y`
scroll by SCREEN rows (the soft map, vs_sm_scroll), so one `^E` can put a
wrap-continuation segment of a logical line at the top and keep the cursor on the
same file line at a wrapped column; govi's viewport top is a whole LOGICAL line,
so one `^E` advances to the next file line. Repro (12x40, lines of ~95 chars that
wrap to 3 rows): `8G^E` -> nvi tops on a continuation row (cursor 4,40), govi tops
on the next logical line (cursor 0,0).

Closing this needs a sub-line viewport offset (which wrap segment sits on the top
row) threaded through the renderer, scrollToCursor, and the paging math -- a
substantial change to govi's logical-line viewport model, NOT the "small/medium"
the entry assumed. It belongs with the line-wrap cluster (#43 and the
"Inconclusive: LONG-LINE WRAP" note), so it is left as a known architectural
simplification rather than fixed piecemeal.

### 45. `lock` option not implemented -- no concurrent-edit protection  [FIXED 2026-06-28]
FIX (engine): implemented the `lock` option plus the readonly write-guard it
depends on -- the latter turned out to be MISSING too (a second real gap):
- readonly guard (excmds.go exWrite): `:w` of the buffer's own file is now
  refused when `readonly` is set, unless forced -- "Read-only file, not written;
  use ! to override" (nvi common/exf.c file_write). Previously `:set ro` then
  `:w` silently wrote.
- lock (engine.go Open + lock_unix.go): on opening a file with `lock` on, take a
  non-blocking `flock(LOCK_EX|LOCK_NB)` on a DEDICATED fd (nvi keeps a separate
  lock fd). On EAGAIN/EWOULDBLOCK (held elsewhere) the buffer opens read-only
  with "<file>: already locked, session is read-only"; any other error means
  locking is unsupported and editing proceeds (nvi LOCK_FAILED). The lock is
  released on :e/:n/buffer-close/exit.
- temp+rename re-lock (excmds.go Save -> relockAfterWrite): govi writes via a
  temp file + atomic rename, which orphans the inode the lock was held on, so the
  lock would evaporate on the first :w (nvi writes in place and keeps it). After
  a successful self-write govi re-takes the lock on the new inode.
- A non-unix stub (lock_other.go) treats locking as unsupported.

Verified vs real nvi (cross-process, advisory flock is shared): nvi-then-nvi and
govi-then-govi BOTH make instance 2 read-only and refuse its `:w`; and
nvi<->govi interoperate BOTH directions; `:w!` overrides; a single session's
self-`:w` still writes and KEEPS the lock so a later second instance is still
read-only (matching nvi, not the temp-rename lock-loss). `:set ro` + `:w` now
refuses in both. Full `go test ./...` passes (conformance opens unique temp files,
no lock collisions).

--- original analysis (retained) ---
nvi's `lock` option (default ON) takes an advisory lock on the file being edited;
if the file is already locked (typically because it is open in another editor
window), nvi cannot acquire the lock, warns the user, and opens the file
READ-ONLY so that only the first editor can write it. This prevents the classic
data-loss case: two windows open on one file, and the second window's `:w`
silently clobbers the first window's unsaved changes. govi does not implement it
(parity.md: lock ❌), so a second govi happily edits and overwrites. This is the
same data-safety family as #42 (the overwrite guard) -- treat it as a real
feature, not legacy cruft.

Rule (nvi common/exf.c `file_lock`, called from `file_init`):
- When `O_LOCK` is set, on opening a file for editing nvi attempts a NON-BLOCKING
  exclusive lock (flock `LOCK_EX|LOCK_NB`, falling back to fcntl/lockf).
- LOCK_SUCCESS: hold the lock for the session; release it on file close / switch /
  exit (closing the fd releases an flock).
- LOCK_UNAVAIL (held by another process): set the buffer read-only (nvi's
  `F_RDONLY`) and warn, e.g. "<file>: already locked, session is read-only".
  Writing then requires the usual readonly override (`:w!`).
- LOCK_FAILED (locking unsupported, e.g. some NFS): proceed normally -- graceful
  fallback, no warning, not read-only.
- The lock is advisory and keyed on the file, so it works cross-program: an flock
  held by nvi blocks govi and vice-versa.

Implementation notes for govi:
- Gate on the existing `lock` option (already in the option table; currently
  inert).
- govi likely reads the file into its piece-table and may not keep the original
  fd open. flock is released when its fd closes, so govi must hold a dedicated
  open fd (the file itself, or a lock fd) for the lifetime of the edit session and
  close it on `:e`/`:n`/buffer-close/exit.
- Set the buffer read-only on LOCK_UNAVAIL and emit the warning so the existing
  readonly write-guard (and #42's overwrite guard) then protect the file.
- Apply on the read/edit path; nvi also locks on read/write, but the edit-open
  case is the user-visible one to do first.

Verify (cross-process, oracle = a second nvi):
- Start editor instance 1 on file F (acquires the lock). Start instance 2 on the
  SAME F. Instance 2 must come up read-only with the warning, and a plain `:w` in
  instance 2 must REFUSE (only `:w!` overrides). Capture nvi-then-nvi as the
  reference, then require govi-then-govi to match it, AND nvi-then-govi /
  govi-then-nvi to interoperate (the cross-program advisory lock). A goterm test
  can drive two Terms on one temp file and diff instance 2's status/readonly state
  and whether a no-force `:w` changed the file on disk (cf. the #42 disk-based
  guard in goterm coverage_test.go `TestCoverageRecentFixes`).

## Open (file-name argument expansion, 2026-06-29)

### 46. `:e`/`:w`/`:r` do not expand `%`, `#`, or shell globs in file arguments  [FIXED 2026-06-29]
FIX (engine): file-name arguments now go through nvi's `argv_exp2` expansion
(ex/ex_argv.c) instead of being used verbatim. New `engine/argvexp.go`
(`expandFileArgs` + `globPrefix` + `shellExpand`), wired into `exEdit`
(exfile.go), `exWrite` and `exRead` (excmds.go); stdout-only shell capture
`runShellStdout` added to shell.go; `usageError` helper added to exusage.go.

Originally `:e #` opened a file literally named `#` instead of re-editing the
alternate file; `:w %` / `:r #` likewise took the metacharacter literally, and no
form did glob expansion. Found by user report (`:n` then `:e #`).

Rule (verified against the nvi oracle binary at build.unix/.libs/vi, not just the
source). After `%`->current / `#`->alternate substitution (argv_fexp; the `\`
escape is honored), nvi scans the result for the first `shellmeta` char (default
`` ~{[*?$`'"\ ``) and dispatches three ways:
- no metachar: split on whitespace (argv_exp3).
- a bare TRAILING `*` (sole metachar, last char): INTERNAL filename-PREFIX
  completion (argv_lexp) -- it truncates at the `*` and matches by prefix, NOT a
  real glob, and no shell is forked. Empty result -> "Shell expansion failed".
- any other metachar (`?`, `*.c`, `[`, `\`, ...): fork the `shell` option and
  capture `echo <pattern>`'s stdout (argv_sexp), discarding stderr. Behavior
  therefore tracks the user's shell: e.g. zsh errors on an unmatched glob
  ("Shell expansion failed") while /bin/sh passes it through literally.

Arg-count handling differs by command (also oracle-verified):
- `:e` takes exactly one file; >1 match -> `Usage: ...` (the parser's `f1o`).
- `:w`/`:r` call argv_exp2 internally (ex_write.c:206, ex_read.c:195); >1 match
  -> `<pattern>: expanded into too many file names` (EXM_FILECOUNT), NOT Usage.

Verified: new engine tests (exfile_test.go) cover `:e #`, `:w %`, unique prefix
match, no-match, and too-many for both `:e` (Usage) and `:w` (file-count); all
pass. The `secure` option suppresses the shell-fork path (argv_sexp) only, like
nvi -- internal prefix completion still works under `secure`.

### 48. cscope integration was unimplemented (`:cscope`/`:cs`, `:tagnext`/`:tagprev`/`:tagtop`/`:tagpop`, `:display connections`)  [FIXED 2026-06-30]
FIX (engine): implemented nvi's cscope subsystem (ex/ex_cscope.c) plus the tag
navigation commands it depends on (ex/ex_tag.c ex_tag_next/prev/top/pop).

`:cscope` (abbreviation `:cs`) dispatches to `add`, `find`, `help`, `kill`, and
`reset` (prefix-matched, nvi lookup_ccmd). `add file|dir` starts a real cscope
subprocess -- `cscope -dl -f cscope.out` run in the database directory -- and
talks to it over pipes exactly as nvi does: write "<n><pattern>\n" (n is the
query number, the index of the find-type letter in "sgdct efi"), read
"cscope: <count> lines", that many "<file> <context> <lineno> <pattern>" result
lines, and the ">> " prompt. The CSCOPE_DIRS environment variable is consulted
once (nvi EXTENSION #1); cscope.tpath search paths are honored (EXTENSION #2).

`find c|d|e|f|g|i|s|t pattern` queries every connection and turns the matches
into a tag jump: the source-line pattern is converted to a vi-magic regex with
each blank standing for any run of whitespace/comments (nvi re_cscope_conv's
CSCOPE_RE_SPACE, here `\([[:blank:]]\|/\*\([^*]\|\*/\)*\*/\)*`), the file is
loaded, and the cursor lands on the first non-blank of the match (falling back to
the recorded line number when the file is newer than the database). The current
location is pushed so `^T` returns; multiple matches become the active group that
`:tagnext`/`:tagprev` step through, and `:tagtop`/`:tagpop` unwind the stack.
`:tag` now collects all ctags matches too, so its results are likewise walkable.
`:display c[onnections]` lists the running connections; connections are killed on
:cscope kill / reset and on engine Close.

One environment note: this cscope build reports a failed search as "Unable to
search database" with no count line, going straight to the ">> " prompt (which
has no trailing newline). The reader peeks for that prompt as an alternate
terminator so a zero-match query does not block.

Verified against nvi via goterm (run sequentially so the two editors don't fight
over nvi's per-file edit lock): `:cs find g` definition jumps (centered,
byte-for-byte identical screens), `:cs find c` + `:tagnext` (same match order),
and a no-match query all match nvi exactly. Engine unit tests in
engine/cscope_test.go build a throwaway database with the real cscope binary and
exercise find/definition, multi-match navigation, ^T return, no-connections,
unknown-search-type, no-match, :display connections, kill, and help. (Running
both editors concurrently on the same source tree triggers nvi's "already
locked, session is read-only" pager -- a harness artifact of shared files, not a
divergence; see entry #47's note on the same message.)

### 47. split screens were unimplemented (`^W`, `:E`/`:N`/..., `:vsplit`, `:bg`/`:fg`, `:resize`, `:display screens`)  [FIXED 2026-06-30]
FIX (engine + tcell): implemented nvi's multi-window subsystem (vi/vs_split.c,
vi/v_screen.c, ex/ex_screen.c, ex/ex_display.c, the E_NEWSCREEN path in ex/ex.c).
The Engine now holds a list of displayed screens plus a background queue; `e.scr`
tracks the active one, so existing per-screen code is unchanged. Each screen
carries its own geometry (roff/coff/rows/cols), paged-file handle, argument list,
alternate file, and tag stack; registers and maps are shared, options copied.
New `engine/split.go` + `engine/screencmds.go`; the tcell frontend renders each
screen in its own band with a reverse-video status divider (and a `|` column for
vertical splits).

GUI (GoVi.app) [2026-06-30]: the grid composer (`frontend/grid`, the layout the
GUI bridge pulls) was multi-pane-aware-extended to mirror the tcell `paintScreen`
path: when `View.Split()`, `ComposeSel` iterates `View.Screens()` and lays out
each pane into its `roff/coff/rows/cols` band -- text, gutter, tilde/blank fill,
the reverse-video status/modeline divider, the `|` vsplit column, and the cursor
in the active pane. No C/Swift change was needed: GoVi.app already renders purely
from the composed grid rows + cursor, so it now shows every split pane. Parity is
locked by `TestSplitGridMatchesTcell` (frontend/tcell), which renders the same
engine `View` instant through both frontends and asserts identical rows + cursor
for horizontal and vertical splits; `frontend/grid` adds `TestSplitThroughGrid`
and `TestVsplitThroughGrid`. Previously the GUI drew only the active screen (one
file in the top half).

Commands: `^W` cycles screens; capitalized `:E`/`:N`/`:P`/`:Vi`/`:Tag` open the
target in a new horizontal split; `:vsplit` splits vertically; `:bg`/`:fg`/`:Fg`
background/foreground; `:resize [+-]rows`; `:display s[creens]`/`b[uffers]`;
`:q`/`:wq`/`:x`/`ZZ` close just the active screen until the last one exits.
Removed the divergent `:Next`->previous mapping (oracle: `:N` edits the *next*
file in a new screen).

Verified against nvi via goterm (text + cursor + reverse attributes): split
geometry, `^W`, per-screen `:` commands, editing both sections, three-way splits,
close/join (both axes), `:bg`/`:fg`/`:Fg`, `:resize` grow/shrink, `:display`, and
per-screen arglist navigation all match. Engine unit tests in
`engine/split_test.go`. Known cosmetic edge cases, all on the dropped status row
or only with pathologically long (>80-col) temp paths: the one-keystroke
transient status line truncates a hair differently in a 3-way split, and
`:display screens` wraps a >80-col name one column off. nvi's `:s`/`:d` "N lines
changed" report is still unimplemented (pre-existing, surfaces on a vsplit's
shared bottom status row).

## Parity-review wave (2026-07-01, found by the parityreview_test.go battery)

### 49. insert-mode `^T` shifted the line's indent instead of indenting at the cursor  [FIXED 2026-07-01]
govi's `^T` in input mode adjusted the LINE's leading indent (vim semantics).
nvi (v_txt.c txt_dent, isindent=1) advances the CURSOR's screen column to the
next shiftwidth boundary: any blanks immediately before the cursor are consumed,
then the gap is filled with tabs (a full tabstop each) and trailing spaces,
inserted at the cursor. On "alpha beta gamma" with sw=4, `ia^Tb<Esc>`:
nvi -> "a   balpha..." (cursor 0,4); govi (old) -> "    abalpha..." (0,5).
Fixed in engine/viinsert.go (insertIndent); verified by the goterm coverage
ctrl-t case and two new conformance cases (insert-ctrl-t-midline/-tab) against
the oracle. Insert-mode `^D` keeps the old line-shift model for now (nvi's ^D
is ai-boundary-scoped with 0^D/^^D forms; still open, see the Inconclusive ^D
note).

### 50. `:vi`/`:visual` with a file argument did nothing  [FIXED 2026-07-01]
nvi's vi-mode `:vi[sual][!] [+cmd] [file]` is a second command-table entry
(C_VISUAL_VI) that IS `ex_edit` -- it edits the named file; `:Vi file` opens it
in a split. govi's exVisual ignored the argument entirely (only the bare
ex-mode "return to vi" form worked). Fixed in engine/exmode.go: a non-empty
argument delegates to exEdit. Verified: parityreview "visual-file" now matches.

### 51. `window` option was inert (accepted but drove nothing)  [FIXED 2026-07-01]
parity.md claimed `window` functional; it was read only as a cap for the
`z<count>` map. In nvi, `:set window=N` (f_window-clamped to lines-1) resizes
the vi map immediately -- the screen paints N text rows and grows like a
z[count] small screen -- and `^F`/`^B` page by `count*window - 2`
(v_pagedown/v_pageup), while a geometry change re-derives a default window and
the displayed `scroll` (f_lines). At 24x80 with `:set window=6`, `^F`:
nvi top->005 in a 6-row map; govi (old) paged 21 lines in a full screen.
Fixed across engine/screen.go (windowVal/applyWindowOption), engine/vi.go
(pageOffset, z reset to the window default), engine/options.go (afterOptSet),
engine/engine.go (Resize/relayout). `:set all` now shows nvi-matching
window/scroll values. Verified: parityreview "window" matches; paging battery
0/8; z cases unchanged.

### 52. `:preserve` snapshot was deleted on a clean exit  [FIXED 2026-07-01, DATA-SAFETY]
govi wrote the recovery snapshot on `:preserve` ("File preserved") but a normal
`:q!`/exit removed it with the session's recovery file -- defeating the entire
point of :preserve (recover LATER with `vi -r`). nvi keeps a preserved file
(RCV_PRESERVE). Fixed in engine/recovery.go: :preserve marks the file kept;
removeRecovery detaches instead of deleting it (later edits start a fresh
recovery file). Regression: engine TestRecoveryPreservedSurvivesExit. Note nvi
leaves TWO entries (snapshot + recover-mail file); govi has no recovery-mail
concept and leaves one.

### 53. `autowrite` was not honored by the file-switching commands  [FIXED 2026-07-01]
parity.md claimed `autowrite` functional; only suspend used it. nvi applies it
in file_m1 (common/exf.c): an unforced `:n`/`:prev`/`:rew`, tag jump/push/pop,
or `^^` away from a modified buffer WRITES it (unless readonly -- System V
behavior) instead of failing "No write since last change". Fixed in
engine/exfile.go (checkModified) and applied at all seven guard sites
(exfile.go, tags.go, vicmds2.go). Verified: parityreview "autowrite" matches
with the guard case still failing without aw. The historic autowrite-before-`:!`
corner (ex_bang.c) is not wired; recorded, not fixed.

### 54. `:display tags` / `:display buffers` format differs from nvi  [OPEN, display]
Behavior is right (stack and buffers are tracked; `:display screens` matches
exactly) but the report layout differs. `tags`: nvi prints one frame per stack
entry INCLUDING the origin position (a one-jump stack shows 2 rows) with its
own alignment; govi prints only the jump frames and pads differently.
`buffers`: nvi lists "* a (line mode)" style per-buffer headers plus a
"* default buffer" row; govi prints a compact "a  <content>" line and no
default-buffer row. Both are display-only; needs homebrew-nvi format
reverse-engineering (this tree's ex_tag.c does not match the 1.81.6 binary's
spacing exactly), so recorded rather than fixed.

### 55. `.` dot-repeat buffer was dropped by a file switch  [FIXED 2026-07-06]
The `.` command lost its memory across `:n`/`:e`/tag-jump: after changing a
line in one file and switching to the next (the `govi *.go` workflow), `.` did
nothing. Cause: replaceBuffer (engine/engine.go) recreated the vi state machine
(`e.vi = newVimode()`) on every file switch, discarding the dot replay buffer
(and the f/t char-search repeat and undo direction with it). nvi's `:n` reuses
the same screen and only swaps the underlying file, so the vi-private state
survives -- and even its conservative new-screen path (v_init.c v_screen_copy)
deliberately carries the replay buffer: "User can replay the last input, but
nothing else." Fixed by not recreating vimode in replaceBuffer; the
command-building fields (op, count, pending, inserting) are already idle when a
file switch runs, so nothing transient leaks. Regression: goterm
TestCoverageMultiFile "dot-next" (`dw:n!\r.` -> both show "BBB CCC").

### 56. named collating elements (`[[.tab.]]`, `[[.comma.]]`, ...) were not resolved  [FIXED 2026-07-06]
In a bracket expression, a symbolic collating-element name such as `[[.tab.]]`
or `[[.comma.]]` must resolve to the named character (POSIX; Spencer's regex via
his cname.h character-name table). govi's parseClassElement (engine/regex/
class.go) accepted only a single-character element between the `[.`/`.]` (or
`[=`/`=]`) delimiters and reported "invalid collating element" for any multi-
character name, so `[[.tab.]]` never matched a tab and `[[.comma.]]` never a
comma. This mirrored a real bug nvi itself had: regcomp.c p_b_coll_elem tested
`MEMCMP(...)` (nonzero == "differs") as if it were "matches", so it never found
a name either (nvi fix d8757ff0, 2026-07-06). Fixed by porting the cname.h table
into a `collatingNames` map and looking the name up first; a single character
still stands for itself, and an unknown name is still REG_ECOLLATE. The lookup
serves both `[[.name.]]` and `[[=name=]]` (nvi routes both through
p_b_coll_elem, the latter via p_b_eclass). Verified against the rebuilt oracle:
`:s/[[.tab.]]/T/` + `:s/[[.comma.]]/C/` on "a<tab>b,c" -> both "aTbCc", and an
unknown `[[.nosuch.]]` errors identically. Regression: engine/regex
TestMatch `[[.tab.]]`, `[[.comma.]]x`, `[[.newline.]]`, `[[.space.]-[.tilde.]]`.

## Undocumented but functional (note, not a divergence)
- vi `^\` (switch to ex mode): WORKS in govi -- `^\` then `2d` then `1,$p` executes
  in ex mode and returns cleanly with `vi` -- but `^\` is NOT listed in govi's
  `:viusage` (nvi lists it). So it is a documentation gap in the usage text, while
  the command itself behaves. (govi's ex-mode screen rendering still differs per the
  #28/#29 display notes, but the command is functional.) Worth adding to viusage.

## Inconclusive / needs a cleaner test
- EX INFO-MESSAGE PAGINATION: for multi-line informational/error output (`:r` ->
  "N lines, M characters", `:e` of a modified buffer -> "File modified since last
  complete write...", `:args` list), nvi paginates into the screen body with a
  `+=+=` separator; govi appears to use only the single status line (which the
  harness drops), so these cannot be cleanly diffed here. The underlying ACTIONS
  are correct in govi (`:r` reads the file, `:e` refuses a modified buffer, `:n`/
  `:rew` switch). Worth a dedicated status-line-aware test to judge the messaging.
- LONG-LINE WRAP at a narrow screen: after `3J` on a 40-col terminal the BUFFER is
  identical (verified at 80 cols: both "...aaa bbb aaa ccc", cursor 0,33), but the
  wrapped continuation row renders differently (govi "b aaa ccc"; nvi "aaa bbb aaa
  ccc"). Content is correct; this is a long-logical-line display difference worth a
  dedicated look, not a content bug.
- EX-MODE METHOD: drive true ex mode (Q) ONE command per send with a settle
  between, never as a single byte burst -- nvi's line-oriented ex reader does not
  consume a burst the way the vi key path does, and a burst makes nvi look like it
  ignored the commands (buffer "unmodified") when stepwise it executes them fine.
- ex `:s///c` (confirm flag): interactive; driving it with a fixed burst of `y`
  answers diverges in which matches get confirmed (e.g. the `o` in "four"), but
  that is likely the harness feeding responses faster than the prompt cycles, not
  a clean behavioral finding. Needs a per-prompt send-and-settle test.
- insert-mode `^D` (dedent): nvi's mid-line behavior is arcane -- when ^D is typed
  after other text it can be stored as a literal `^D` rather than dedenting, so a
  naive comparison diverges without either editor being clearly "right".  `^T`
  (indent) works and matches.  Needs a test that uses ^D in proper autoindent
  context (at the start of an auto-indented line) before judging.

## Known/accepted (do NOT "fix")
- `^X` hex input: govi accepts 2, 4, or 6 hex digits; nvi only 2. Intentional
  govi extension.
- `filec` defaults to `<tab>` in govi (colon-line file completion on out of the
  box); nvi's default is empty. The completion mechanism itself matches when
  both editors set the same character (parityreview "filec").
- `"0` (yank register) and `"-` (small-delete register): govi implements both
  (vim); nvi has NEITHER -- `"0p` / `"-p` are no-ops in nvi. govi correctly does
  not shift the numbered ring "1.."9 on a sub-line delete. These are govi
  supersets that never change behavior on registers nvi accepts; accepted.
- `\|` alternation in search/regex: govi supports it (vim/GNU extension); nvi uses
  POSIX BRE which has no `\|`, so `/cat\|dog` matches nothing in nvi. govi superset
  (its regex engine is otherwise a match: `\<`/`\>`, `[...]`, `[^...]`, `^`/`$`,
  `*`, `.`, `\(...\)`, `\{n\}`, nomagic, and ignorecase all agree). Accepted.
- count + CHARWISE put (e.g. `ye3p`, `yw3p`): govi replicates the text cleanly
  (matches vim: "abc" x3 -> "abcabcabc", giving "aabcabcabcbc.def"); nvi produces
  a garbled interleaving ("aaabcabcbcbc.def"). nvi appears buggy here; govi is
  correct -- do NOT match nvi. (Linewise count-put `yy3p` matches.)
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
