# govi vs the nvi correctness fixes: review findings

Reviews each item in `NVI_CORRECTNESS_FIXES.md` and determines whether govi has
the same defect. Behavioral items were checked by driving govi and nvi through
identical input on identical headless terminals (the goterm harness) and diffing
the rendered screen + cursor. Non-behavioral items (C memory/pointer bugs) and
unimplemented features were determined by source review.

## Methodology note: which nvi is the reference

`/opt/homebrew/bin/nvi` is **nvi 1.81.6**, which PRE-DATES most of the fixes in
`NVI_CORRECTNESS_FIXES.md` -- it is the BUGGY reference for them. The FIXED
reference is the oracle built from this tree (whose git log contains those exact
fix commits): `/Users/claude/src/nvi/build.unix/vi`. All verdicts below compare
govi to the FIXED oracle. (Example: on the #11 word-end case, homebrew nvi gives
the buggy answer and govi gives the fixed one -- comparing only against homebrew
would have been misleading.)

## Verdict summary

| # | Issue | Verdict |
|---|-------|---------|
| 1 | autoindent `^^D` abort | NOT PRESENT (no abort); minor gap: `^^D`/`0^D` not implemented |
| 2 | buffer too small in v_txt | N/A (Go slices) |
| 3 | SC_TINPUT not cleared on error | N/A (no overloaded C flag) |
| 4 | cursor on partial multi-column char | Not reproduced; minor at-rest tab-cursor diff noted |
| 5 | smatcher/lmatcher 64-bit selection | N/A (single backtracking matcher) |
| 6 | SEARCH_EXTEND not passed | N/A (govi `^A` builds BRE-safe patterns); `^A`-on-word OK |
| 7 | wrong cclass in p_b_cclass | NOT PRESENT (each POSIX class maps to its own predicate) |
| 8 | `^A` idempotency (lone non-word char) | **PRESENT** |
| 9 | `^A` fails on non-word keyword | **PRESENT** |
| 10 | keyword starting non-word built wrong | **PRESENT** |
| 11 | `^A` ignored word-end boundaries | NOT PRESENT (govi honors `\<..\>`) |
| 12 | append to cut buffer (isupper) | NOT PRESENT (append works) |
| 13 | join not POSIX (two spaces after .?!) | **PRESENT** |
| 14 | join with single address | NOT PRESENT |
| 15 | `taglength` had no effect | **PRESENT** |
| 16 | ctag_search missing SEARCH_PARSE | NOT PRESENT (tag pattern parsed as regex) |
| 17 | cscope freed memory / `:cs add` | N/A (cscope not implemented) |
| 18 | screen offset of top line > screen count | Basic redraw OK; tall-line case = known soft-map gap (govi #43/#44) |
| 19 | `leftright`+`number` hang | NOT PRESENT (no hang; leftright is a no-op option) |
| 20 | scrolling broken with `leftright` | N/A (leftright scrolling not implemented) |
| 21 | `vi +line` made target the top line | N/A (govi has no `+N`/`+cmd` startup) |
| 22 | vs_line no-draw cursor y | N/A (govi has its own display; no SMAP) |
| 23 | vs_swap used window before init | N/A (no multi-screen) |
| 24 | tty left in ex mode after `q`, multi-screen | N/A (single-screen) |
| 25 | ex->vi confusion with split screens | N/A (single-screen) |
| 26 | CIRCLEQ_INSERT_BEFORE wrong branch | N/A (single-screen) |
| 27 | same file in two screens border case | N/A (single-screen) |
| 28 | tcsetattr EINTR not retried | N/A (Go runtime signal/syscall handling) |
| 29 | OPT_GLOBAL checked in wrong place | N/A (single option table; not observable) |
| 30 | octal option value core dump | NOT PRESENT (base-10 parse, no crash) |
| 31 | abbreviations broken | NOT PRESENT (work) |
| 32 | `@` macro stopped after first run | **PRESENT** (symptom) -- different root cause |
| 33 | `%` expansion in filters/read | **PRESENT** (no `%`/`#` filename expansion) |
| 34 | invalid ex input infinite loop | NOT PRESENT (no hang) |
| 35 | cedit log error killed option | N/A (cedit not implemented) |
| 36 | edited files lacked O_CLOEXEC | NOT PRESENT (Go sets O_CLOEXEC by default) |
| 37 | `-r` listing hardcoded "vi" | N/A (govi prints "govi"; no ex/view aliasing) |
| 38 | wrong pointer freed | N/A (Go GC) |
| 39 | NULL not cast in variadic call | N/A (Go) |
| 40 | replacement string buffer too small | N/A (Go slices) |
| 41 | wide chars treated as blanks | N/A (explicit ASCII blank checks) |
| 42 | `<End>` key not recognized | NOT PRESENT (govi recognizes `<End>`) |

**Confirmed present in govi: #8, #9, #10 (one root cause), #13, #15, #32 (symptom), #33.**

---

## Confirmed present -- details, evidence, fix pointers

### #8 / #9 / #10 -- `^A` cursor-word search over a non-word character

One root cause covers all three. govi's `wordAt` (engine/search.go) skips forward
over ALL non-word runes to the next word, and `searchCurrentWord` always builds
`\<word\>`. nvi's `v_curword` (vi/vi.c) only skips WHITESPACE and always includes
the character under the cursor, so a cursor on a non-word char yields that char as
the keyword; `v_searchw` (vi/v_search.c) then builds an ERE: a non-word keyword
gets `([^[:alnum:]_]|$)` as the rear delimiter (which is what makes repeated `^A`
idempotent, #8), and a leading non-word char is escaped rather than dropped.

Evidence (12x40; `^A` is Ctrl-A):

```
buffer ["foo ^ bar","baz ^ qux"], cursor on first ^ (f^), one ^A:
  fixed-nvi -> (1,4)  (the ^ on line 2)      govi -> (0,6)  (the word "bar")
buffer ["^foo bar","x ^foo y"], cursor on ^ at col0, one ^A:
  fixed-nvi -> (1,2)  (the "^foo" on line 2) govi -> (0,1)  (the "foo" on line 1)
```

govi cannot search for a keyword that is (or starts with) a non-word character;
it silently retargets to the nearest word. Fix in `searchCurrentWord`/`wordAt`:
when the cursor is on a non-word (non-blank) char, make the keyword that single
char (or char + following word run) and build nvi's ERE delimiters.

### #13 -- join does not insert two spaces after `.`, `?`, `!`

POSIX/historic vi join inserts TWO spaces when the preceding line ends in `.`,
`?`, or `!`. govi's `joinLines` (engine/viedit.go) and `exJoin` (engine/excmds.go)
only ever insert a single space. nvi's rule is in ex/ex_join.c (`strchr(".?!", echar)`).

```
["Hello.","World"] + J   -> fixed-nvi "Hello.  World"   govi "Hello. World"
["Stop!","Go"]     + J   -> fixed-nvi "Stop!  Go"        govi "Stop! Go"
:1,2j (same buffers)     -> same divergence
```

(Minor related: govi strips the next line's leading blanks BEFORE testing for a
leading `)`, while nvi tests `p[0]` first -- a rarely-hit edge in the `)` rule.)

Fix: in both join paths, when the first part's last char is in `.?!`, emit two
spaces instead of one.

### #15 -- `taglength` has no effect

`engine/tags.go:lookupTag` matches the tag name with `parts[0] == name` (exact)
and never consults the `taglength` option (defined in options.go but unused).
nvi truncates both the tag and the search key to `taglength` significant chars.

```
tags file has tag "counter"; :set taglength=4 then :tag countXXXX
  fixed-nvi -> jumps to the "func counter" line   govi -> "countXXXX: tag not found"
```

Fix: in `lookupTag`, when `taglength > 0`, compare only the first `taglength`
runes of both the requested name and each tag-file entry.

### #32 -- repeated `@<buffer>` stops working (symptom present; different cause)

The nvi symptom ("@ execution stopped after first run") is reproducible in govi,
but the root cause is different and worth recording because it is a broader
register-correctness bug.

```
buffer ["x","AAAA","BBBB","CCCC","DDDD"], reg a = "x" (linewise) via "ayy, j:
  fixed-nvi  @a @a @a -> AAA/BBB/CCC (each @a deletes a char, moves down)
  govi       @a       -> AAA           (first @a OK)
  govi       @a @a    -> AAA/BBBB...   (second @a does NOT run)
  govi       @a x3    -> line "BBBB@a" (literal "@a" gets typed as text)
```

Root cause (engine/vi.go): `m.reg` is cleared ONLY in `finishCommand`, which is
gated by `!pendingState()`, and `pendingState()` is true whenever `m.reg != 0`.
So after a register-prefixed command (`"ayy`), `m.reg` stays `'a'` indefinitely
(the doubled-operator branch at vi.go ~189 clears op/count but not `m.reg`). The
macro then runs `x`, whose delete writes into the still-selected register `a`,
overwriting the macro with the deleted char (`A`, an append command), so the
SECOND `@a` runs garbage.

Direct (non-macro) confirmation -- `"ayyGx"aP` (yank "yanked" to a; delete a char
on the last line; put a):

```
fixed-nvi -> reg a still "yanked" (a "yanked" line is put)   govi -> reg a clobbered
```

Fix: clear `m.reg` when a command completes regardless of `pendingState` (e.g.
reset it in the doubled-operator branch and in `finishCommand`'s reachable path),
so a register selection does not leak into the next command.

### #33 -- `%` / `#` filename expansion not implemented

govi's shell paths (`engine/shell.go`: exBang, filterLines, readFromCommand,
writeToCommand) use the command argument verbatim. nvi expands `%` to the current
file name and `#` to the alternate file in these arguments.

```
file "buf.txt"; :r !echo %
  fixed-nvi -> inserts the file's path   govi -> inserts the literal "%"
```

Fix: expand `%`/`#` (with `\%` escaping) in the argument before running the shell,
for `:!`, `:[range]!`, `:r !`, `:w !`, and the vi `!` operator.

---

## Notable NOT-PRESENT items (govi already matches fixed nvi)

- **#1 (autoindent `^^D` abort):** govi does NOT abort/crash on
  `:set ai<CR>i<Tab><CR>^^D` (or the `0^D` variant). Both nvis erase the
  autoindent and leave an empty line; govi instead inserts the literal `^`/`0`
  (it does not implement the `^^D`/`0^D` "remove all autoindent" forms). That is a
  minor behavioral gap, NOT the C_NOCHANGE abort the fix addressed.
- **#7 (cclass):** `engine/regex/class.go` maps each `[:name:]` to its own
  predicate; no `[:alnum:]`/`[:alpha:]` mix-up.
- **#11 (`^A` word-end):** govi uses `\<..\>` and lands only on whole-word
  matches -- it does NOT walk onto `ab`/`abc` (verified: matches fixed nvi, while
  homebrew 1.81.6 is the buggy one here).
- **#12, #14, #19, #30, #31, #34, #42, #36:** verified above / by source; govi is
  correct (details in the table).
- **#16 (tag pattern):** govi runs a tag `/pattern/` through the regex engine, so
  `^`/`$` anchors work (an anchored `/^target$/` skips a `xtargetx` decoy line,
  matching fixed nvi).

## N/A -- structurally impossible in a Go reimplementation

- **#2, #38, #39, #40:** fixed-size copy buffers, wrong/early `free`, NULL-cast in
  variadic calls -- none exist under Go's slices + GC.
- **#28 (tcsetattr EINTR):** Go's runtime owns signal delivery and retries
  interrupted syscalls; nvi's hand-rolled tcsetattr retry loop has no analogue.
- **#41 (wide-char isblank):** govi tests blanks with explicit ASCII checks
  (`== ' ' || == '\t'`) / unicode predicates, never a C `isblank()` on a wide
  integer type, so the misclassification cannot recur (not separately exercised
  under the ASCII/ansi harness).
- **#3 (SC_TINPUT on error path):** govi does not overload one input-mode flag as
  both state and a no-change sentinel; the specific leak cannot recur.
- **#36 (O_CLOEXEC):** Go opens files `O_CLOEXEC` by default, so edited files do
  not leak into child processes -- govi gets this fix for free.

## N/A -- feature not implemented in govi

- **#5 (smatcher/lmatcher):** govi's regex is a single backtracking
  continuation-passing matcher (`engine/regex/match.go`); there is no bitvector-vs-
  list matcher chosen by `sizeof`, so the selection bug cannot exist.
- **#17 (cscope):** not implemented.
- **#20 (leftright scrolling), #19 partial:** `leftright` exists only as an option
  flag; there is no horizontal-scroll mode. Setting `leftright`+`number` does not
  hang (#19 verified), and the scroll bug (#20) has no code to be wrong.
- **#21 (`vi +line`):** govi does not parse `+N`/`+command` startup arguments at
  all (it treats `+20` as a filename), so the "+line forced to top" bug is moot.
- **#22, #23, #24, #25, #26, #27 (screen-map / split windows):** govi is
  single-screen with its own display; nvi's SMAP, hidden-screen queue, CIRCLEQ
  window list, and ex/vi multi-screen mode-flag transfer do not exist. (The
  related tall-line/soft-map limitation govi DOES have is tracked in its own
  catalog as #43/#44, not here.)
- **#29 (OPT_GLOBAL):** one option table, one screen -- the per-file/global split
  is not observable.
- **#35 (cedit):** the `cedit` ex-command-edit buffer is not implemented.
- **#37 (`-r` program name):** govi's recovery listing prints "govi" and govi has
  no `ex`/`view` invocation aliasing, so the hardcoded-name bug is moot.

## Minor observation (not one of the 42)

At rest (on file load or right after `:set`), govi draws the cursor on a leading
tab at the tab's LAST cell (e.g. col 7 for tabstop 8) while nvi draws it one cell
further (col 8). It agrees with nvi after any cursor motion, and buffer content is
always correct. This is the cursor-column difference seen in the #18 and #30
probes; it is cosmetic and resolves immediately, and is not the partial-cell input
bug of #4.
