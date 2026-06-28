# Fixing govi to match nvi (handoff from the goterm comparison work)

This note is for the session that will fix govi so it matches nvi. The
divergences below were found by driving govi and nvi through identical input on
identical headless terminals (the `goterm` emulator) and diffing the rendered
screen + cursor. Start here.

## Use the skill
There is a global skill `compare-govi-nvi`. It explains the harness, the
fix->verify loop, and the methodology traps. Read it (or invoke it) before
changing code. The full divergence catalog with exact rules and evidence is
`/Users/claude/src/goterm/DIVERGENCES.md`.

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
place the cursor at the top of the new page (nvi behavior). See the table in
DIVERGENCES.md.

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

## Status: all six fixes landed; the goterm battery is 0 diverged across
editing/motion/search/ex/paging/registers/structure, and `go test ./...` passes.

## Known/accepted (do NOT "fix")
- `^X` hex input: govi accepts 2, 4, or 6 hex digits; nvi only 2. Intentional
  govi extension.

## What already matches (don't worry about these)
Most common commands already match: x/dd/dw/D/J/r/~/cw/yyp, w/b/e/0/$/^/f/t/G/gg/
H/L/50%, /,n,N,?,*, :s/:%s/:m/:t/:N, marks, "a registers, `.` repeat (except the
cw case, which is bug #1), q/@ macros, >>/<</%/==. Full list in DIVERGENCES.md.
