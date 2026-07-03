# govi vs nvi -- feature parity (fresh source audit)

## Corrections found (audit 2026-06-30)

Every line re-verified against the actual source (nvi `vi/v_cmd.c`,
`ex/ex_cmd.c`, `common/options.c`; govi `engine/`, `frontend/tcell/`,
`gui/bridge/`, `gui/macos/`), digging into the called functions -- not trusting
prior assertions. Nine inaccuracies were found. Items 1, 6, 7, 8, and 9 have
been corrected in this document, and items 2, 3, 4, and 5 fixed in source. All
nine are now addressed.
Items 6 and 7 also reflect real code gaps (the `print` option is not wired;
`remap`/recursive map expansion is not honored) that are deferred.
A second-round re-audit of the item 3 (^C) fix (items 10-11, below) found a
read-ahead race in the terminal path -- now fixed in source and verified -- and
that the GUI never wired ^C for in-progress interruption -- now fixed in source
too (fix b), build-verified, with on-device runtime QA still pending.

1. **`g`** -- was wrong in ALL THREE columns. nvi `v_cmd.c:407` is `{NULL}`,
   i.e. truly unbound (like `K`/`v`/`V`, which the doc itself lists as unbound).
   govi has no `g` in `commandKey`, `g` is not in `isMotionKey` (`vimotion.go:29`),
   and `editKey` has no `g` case, so a plain `g` hits the default and bells
   ("g isn't a vi command", `vi.go:813`). The "gg/prefix commands" note was
   fictional.
   **Addressed:** removed the standalone `g` row from the Letters table and added
   `g` to the "Not bound in nvi" footnote beneath that table (with the
   `v_cmd.c:407` citation), matching how `K`/`v`/`V`/`^O`/`^_`/`=` are handled.
2. **`^[`** -- govi/GoVi "cancel partial command" was only partial. `d<ESC>w`
   deleted a word and `5<ESC>x` deleted five chars, so a pending **operator or
   count was not cleared by Escape**; only pending single-char commands
   (`f F t T r m` `` ` `` `' z [ ] @ #`) and insert mode cancelled. nvi's `^[`
   cancels a partial command (silently for an operator/register, with a bell for
   a count-only or idle `^[`; the count is always discarded).
   **Addressed:** added `cancelCommand` (engine/vi.go), invoked when `<ESC>`
   reaches `commandKey`; it clears any pending operator/register/count and bells
   only for the non-partial (count-only or idle) case, matching nvi's `v_cmd`
   esc: handling. Verified against nvi via goterm (`d<ESC>w`, `5<ESC>x`,
   `2d<ESC>w` all match) with an engine regression test.
3. **`^C`** -- note overstated. `interrupt()` originally only cancelled an active
   `:` colon line and rang the bell; it did NOT abort an in-progress
   read/write/search (govi had no cooperative interrupt flag).
   **Addressed (3 stages):** added a cooperative interrupt (engine/interrupt.go).
   A frontend records ^C out of band via `Interrupt()` (atomic flag + buffered
   channel); the engine chooses when to observe it. Stage 1: plumbing + the tcell
   forwarder / GUI wiring. Stage 2: the CPU-bound loops (search, `:s`, `:g`/`:v`)
   poll `Interrupted()` and abort with "Interrupted", keeping partial results.
   Stage 3: blocking external commands (`:!`, `:%!` filter, `:r !`, `w !`) run on
   a goroutine and `select` on `InterruptChan()`, killing the child on ^C; the
   `:w` line loop polls the flag and discards its temp file. (A plain file *read*
   is paged/lazy, so there is no blocking read loop to interrupt.)
4. **`:display`** -- note listed "tags" but govi did not implement it.
   `exDisplay` handled only buffers/screens/connections; `:display tags` fell
   through to the usage error, though nvi's `ex_display` has tags.
   **Addressed:** implemented `displayTags` (engine/screencmds.go) matching nvi's
   `ex_tag_display` layout (numbered, most recent first, current entry marked
   `*`, right-justified 30-col file + tag name). Added a `tag` field to `tagLoc`
   populated at the tag/cscope push sites (engine/tags.go, engine/cscopefind.go)
   and wired `t[ags]` into the `:display` dispatch.
5. **`:ex`** -- note "switch to ex mode" mischaracterized nvi. nvi `:ex`
   (`ex_cmd.c:163` -> `ex_edit`) is "begin editing another file" (an alias of
   `:edit`, `E_NEWSCREEN`; not ex mode). The vi<->ex mode switch is `Q`/`^\`
   (`v_exmode`). So `:ex` was simply a missing alias of govi's existing `:edit`.
   **Addressed:** added `{full: "ex", min: 2, fn: exEdit, newScreen: true}` to
   `exCmds` (engine/ex.go) plus an `exCmdMeta["ex"]` usage entry
   (engine/exusage.go). `:ex`/`:Ex` now behave exactly like `:edit`/`:Edit`;
   `:e`, `:exu`, and `:exusage` still resolve correctly (verified).
6. **`print` (option)** -- was marked functional (✅) but the option value is never
   read anywhere (grep of engine/frontend/gui/cmd finds only the definition, the
   `:print` command, the `[:print:]` regex class, and `:exusage` text). Printable-
   char display is hardcoded, not driven by the option. This is a real code gap
   (the option is not wired), deferred for now.
   **Addressed (docs only):** the option row already carries the accurate ⚙️
   (inert) status and note; removed the inline flag.
7. **`remap` (option note)** -- was backwards. govi maps are NON-remapping:
   `resolveMap` (`maps.go:152-177`) sends each RHS rune through `dispatchRune`
   straight to the mode handler, never back through map expansion, so maps never
   expand recursively (RHS is literal; see `maps.go:13`). The ⚙️ status is
   correct; only the note was wrong. Honoring `remap` (recursive expansion) is a
   real code gap, deferred for now.
   **Addressed (docs only):** the option row already carries the corrected
   non-remapping note; removed the inline flag.
8. **"~40 functional"** -- overstated. Auditing reads, ~31 of the 73 shared
   options are functional and ~42 are inert; the two figures were essentially
   inverted.
   **Addressed:** corrected the Bottom line to "~31 functional / ~42 inert".
9. **"5 nvi options omitted"** -- undercount. nvi also carries `w300`, `w1200`,
   `w9600` (`options.c`, `OPT_NDISP` baud/window aliases) that govi omits too, so
   **8** are omitted, not 5. Because govi omits all of these, "so `:set all`
   matches" is also not literally true -- nvi's `:set all` shows
   `noprint`/`modeline`/encoding that govi lacks.
   **Addressed:** the omit table already lists all 8 (its heading now reads "8"),
   and the Bottom line was corrected to "8 nvi options are omitted" and no longer
   claims `:set all` is a literal match.

### Corrections found (audit round 2, 2026-06-30) -- re-audit of item 3 (^C)

Re-auditing item 3's ^C fix against the *delivery paths* (not just the engine
loops) turned up two further problems:

10. **^C was lost by a read-ahead race (tcell).** `Engine.Input` cleared the
    interrupt on entry (`clearInterrupt` at the top of `Input`). But the terminal
    frontend records a ^C out of band on a *separate* goroutine
    (`frontend/tcell` `forwardInterrupts`) that reads ahead of the main loop (the
    `events` channel is unbuffered). So a ^C typed to abort a command could be set
    *before* the main loop ran the `Input()` that launches that command, and
    `Input()`'s entry clear then discarded it -- the search / `:s` / `:g` / `:!`
    ran to completion with the ^C silently dropped (in command mode the trailing
    ^C key is a no-op, so there was no feedback at all). Reproduced through the
    real `Input()` path; memory-safe (`go test -race` finds no data race -- it is
    a logic race). The tcell `TestForwardInterruptsOutOfBand` already demonstrates
    the read-ahead that triggers it; the stage-2 engine tests missed it by calling
    `exExecute` directly instead of going through `Input()`.
    **Fixed in source (fix a):** `Input()` now clears the interrupt when the
    command *finishes* (deferred `clearInterrupt`), not on entry, so this
    command's interruptible loops observe a ^C set just before it and only a
    leftover interrupt is discarded (nvi's CLR_INTERRUPT). Verified:
    `TestInterruptSetBeforeLaunchingInputIsHonored` drives the read-ahead
    ordering and the substitution now aborts with `Interrupted` (was running to
    completion); `TestInterruptDoesNotLeakToNextCommand` and the existing
    `TestInterruptClearedAtNextInput` confirm a leftover ^C still does not leak
    into the next command; the full engine + tcell suites, the nvi-oracle
    conformance harness, and `go test -race` all pass. See the `^C` row.
11. **^C does nothing in GoVi.app beyond cancelling a colon line.** The GUI's
    interrupt entry point `GoviInterrupt` (`gui/bridge/bridge.go`) -- the only
    call that reaches `Engine.Interrupt` for the GUI -- has NO caller in the Swift
    host (`gui/macos`). Ctrl-C is delivered by `handleControlKey` as
    `GoviKeyRune(handle, 3, 0)`, an ordinary `KeyEvent{Rune:3}` that only cancels
    a colon line (`colonInterrupt`); `Engine.Interrupt` is never called, so
    `Interrupted()` is always false and no search / `:s` / `:g` aborts. Worse,
    engine input runs synchronously on the AppKit main thread, and a blocking
    `:!`/filter waits on `awaitCmd` -> `InterruptChan()`, which the GUI never
    signals: a slow external command *freezes the whole app* with no way to
    interrupt. (Found by static analysis of the Swift + bridge + engine.)
    **Fixed in source (fix b):** `GoviInterrupt` now records the interrupt only
    (`in.eng.Interrupt()`) -- the one engine call safe to make concurrently. The
    Swift host (`GoviView`) runs each window's input on a private serial
    `engineQueue`; for a command that does not finish at once it keeps the main
    thread pumping only key events and forwards a ^C aimed at that window to
    `GoviInterrupt`, so the running search / `:s` / `:g` / `:!` observes it and
    aborts -- the `:!` freeze is gone (the child is killed). Other keys typed
    meanwhile are deferred and re-posted (type-ahead preserved), the map/showmatch
    timer is suspended during the wait, and the shared handle table is now
    mutex-guarded since input runs off the main thread. Verified: the Go bridge
    builds, vets, and passes `go test -race`, and the full GoVi.app bundle
    compiles, links, and signs. On-device runtime QA (interactive ^C, IME, and
    multi-window behavior) is still pending -- it cannot be exercised headlessly
    here.

---

Independent re-derivation of parity, built by reading the authoritative tables in
both source trees rather than trusting the existing `docs/parity.md`:

- **nvi** commands from `nvi/vi/v_cmd.c` (`vikeys[]`), `nvi/ex/ex_cmd.c`
  (`cmds[]`), and options from `nvi/common/options.c` (`optlist[]`).
- **govi** from `engine/vi.go` + `engine/vimotion*.go` (vi dispatch),
  `engine/ex.go` (`exCmds[]`), and `engine/options.go` (`optDefs[]`).

Two frontends share one engine, so each gets its own column:

- **govi** -- the terminal editor.
- **GoVi.app** -- the native macOS GUI (renders the same engine grid).

Editing behavior is identical between the two; the columns diverge only for
frontend-specific features (job control, redraw, GUI-only options).

**Status legend**

| Status | Meaning |
|--------|---------|
| ✅ | Implemented; matches nvi |
| 🟡 | Implemented with a known gap/simplification |
| ⚙️ | Settable/recognized but does not drive behavior (inert) |
| ❌ | Not implemented |
| — | Not applicable to this frontend |

---

## 1. Vi command-mode commands

### Control keys

| Command | nvi | govi | GoVi.app | Notes |
|---|---|---|---|---|
| `^A` | yes | ✅ | ✅ | search forward for cursor word |
| `^B` | yes | ✅ | ✅ | scroll up by screens |
| `^C` | yes | ✅ | ✅ | interrupt. Both frontends cancel a partial `:` line and abort an in-progress search / `:s` / `:g` / `:!` (kills the child) / `:w`. tcell: a read-ahead ^C is honored, not dropped ([AUDIT 10]). GoVi.app: input runs on a serial engine queue so a ^C on the main thread reaches `Engine.Interrupt` and the `:!` freeze is gone ([AUDIT 11]; build-verified, runtime QA pending) |
| `^D` | yes | ✅ | ✅ | scroll down half screen (sets count) |
| `^E` | yes | 🟡 | 🟡 | scroll down by lines; matches nvi on non-wrapping files, but scrolls by logical line rather than screen row, so wrapped lines differ (GOTERM_DIVERGENCES #44) |
| `^F` | yes | ✅ | ✅ | scroll down by screens |
| `^G` | yes | ✅ | ✅ | file status |
| `^H` | yes | ✅ | ✅ | move left; arrives as Backspace (normalizeKey->`h`). A raw Ctrl-`h` rune is unbound |
| `^J` | yes | ✅ | ✅ | move down by lines |
| `^L` | yes | ✅ | — | force full redraw (tcell `Sync()`), recovering a tty another program corrupted. GUI has no tty to corrupt |
| `^M` | yes | ✅ | ✅ | move down to first non-blank |
| `^N` | yes | ✅ | ✅ | move down by lines |
| `^P` | yes | ✅ | ✅ | move up by lines |
| `^R` | yes | ✅ | — | force full redraw (alias of `^L`). GUI has no tty to corrupt |
| `^T` | yes | ✅ | ✅ | tag pop |
| `^U` | yes | ✅ | ✅ | scroll up half screen (sets count) |
| `^V` | yes | ✅ | ✅ | literal character input (insert mode) |
| `^W` | yes | ✅ | ✅ | switch to next split screen |
| `^Y` | yes | 🟡 | 🟡 | scroll up by lines; same wrapped-line granularity gap as `^E` (GOTERM_DIVERGENCES #44) |
| `^Z` | yes | ✅ | — | suspend; GUI has no job control (Suspender unimplemented) |
| `^[` | yes | ✅ | ✅ | escape / cancel partial command (clears a pending operator, register, or count; bells for a count-only or idle `^[`) |
| `^\` | yes | ✅ | ✅ | switch to ex mode |
| `^]` | yes | ✅ | ✅ | tag push for cursor word |
| `^^` | yes | ✅ | ✅ | switch to alternate file |

### Punctuation and digits

| Command | nvi | govi | GoVi.app | Notes |
|---|---|---|---|---|
| `<space>` | yes | ✅ | ✅ | move right by columns |
| `!` | yes | ✅ | ✅ | filter lines through a command (operator) |
| `#` | yes | ✅ | ✅ | increment/decrement number under cursor |
| `$` | yes | ✅ | ✅ | move to last column |
| `%` | yes | ✅ | ✅ | move to match |
| `&` | yes | ✅ | ✅ | repeat last substitution |
| `'` | yes | ✅ | ✅ | move to mark (first non-blank) |
| `(` | yes | ✅ | ✅ | move back sentence |
| `)` | yes | ✅ | ✅ | move forward sentence |
| `+` | yes | ✅ | ✅ | move down to first non-blank |
| `,` | yes | ✅ | ✅ | reverse last F/f/T/t |
| `-` | yes | ✅ | ✅ | move up to first non-blank |
| `.` | yes | ✅ | ✅ | repeat last change |
| `/` | yes | ✅ | ✅ | search forward |
| `0` | yes | ✅ | ✅ | move to first column |
| `:` | yes | ✅ | ✅ | ex command line |
| `;` | yes | ✅ | ✅ | repeat last F/f/T/t |
| `<` | yes | ✅ | ✅ | shift left (operator) |
| `>` | yes | ✅ | ✅ | shift right (operator) |
| `?` | yes | ✅ | ✅ | search backward |
| `@` | yes | ✅ | ✅ | execute buffer |
| `[[` | yes | ✅ | ✅ | move back section |
| `]]` | yes | ✅ | ✅ | move forward section |
| `^` | yes | ✅ | ✅ | move to first non-blank |
| `_` | yes | ✅ | ✅ | move to first non-blank (count-1 lines down) |
| `` ` `` | yes | ✅ | ✅ | move to mark (exact column) |
| `{` | yes | ✅ | ✅ | move back paragraph |
| `\|` | yes | ✅ | ✅ | move to column |
| `}` | yes | ✅ | ✅ | move forward paragraph |
| `~` | yes | ✅ | ✅ | reverse case |

### Letters

| Command | nvi | govi | GoVi.app | Notes |
|---|---|---|---|---|
| `A` | yes | ✅ | ✅ | append at end of line |
| `B` | yes | ✅ | ✅ | move back bigword |
| `C` | yes | ✅ | ✅ | change to end of line |
| `D` | yes | ✅ | ✅ | delete to end of line |
| `E` | yes | ✅ | ✅ | move to end of bigword |
| `F` | yes | ✅ | ✅ | backward char search in line |
| `G` | yes | ✅ | ✅ | move to line |
| `H` | yes | ✅ | ✅ | move to top of screen |
| `I` | yes | ✅ | ✅ | insert before first non-blank |
| `J` | yes | ✅ | ✅ | join lines |
| `L` | yes | ✅ | ✅ | move to bottom of screen |
| `M` | yes | ✅ | ✅ | move to middle of screen |
| `N` | yes | ✅ | ✅ | reverse last search |
| `O` | yes | ✅ | ✅ | open line above |
| `P` | yes | ✅ | ✅ | paste before cursor |
| `Q` | yes | ✅ | ✅ | switch to ex mode |
| `R` | yes | ✅ | ✅ | replace characters |
| `S` | yes | ✅ | ✅ | substitute line(s) |
| `T` | yes | ✅ | ✅ | backward to-char search in line |
| `U` | yes | ✅ | ✅ | restore current line |
| `W` | yes | ✅ | ✅ | move to next bigword |
| `X` | yes | ✅ | ✅ | delete char before cursor |
| `Y` | yes | ✅ | ✅ | yank line |
| `ZZ` | yes | ✅ | ✅ | write (if modified) and exit. `ZQ` is not an nvi command; govi bells |
| `a` | yes | ✅ | ✅ | append after cursor |
| `b` | yes | ✅ | ✅ | move back word |
| `c` | yes | ✅ | ✅ | change to motion (operator) |
| `d` | yes | ✅ | ✅ | delete to motion (operator) |
| `e` | yes | ✅ | ✅ | move to end of word |
| `f` | yes | ✅ | ✅ | forward char search in line |
| `h` | yes | ✅ | ✅ | move left |
| `i` | yes | ✅ | ✅ | insert before cursor |
| `j` | yes | ✅ | ✅ | move down |
| `k` | yes | ✅ | ✅ | move up |
| `l` | yes | ✅ | ✅ | move right |
| `m` | yes | ✅ | ✅ | set mark |
| `n` | yes | ✅ | ✅ | repeat last search |
| `o` | yes | ✅ | ✅ | open line below |
| `p` | yes | ✅ | ✅ | paste after cursor |
| `r` | yes | ✅ | ✅ | replace one char |
| `s` | yes | ✅ | ✅ | substitute char |
| `t` | yes | ✅ | ✅ | forward to-char search in line |
| `u` | yes | ✅ | ✅ | undo (toggles undo/redo like nvi) |
| `w` | yes | ✅ | ✅ | move to next word |
| `x` | yes | ✅ | ✅ | delete char |
| `y` | yes | ✅ | ✅ | yank to motion (operator) |
| `z` | yes | ✅ | ✅ | reposition screen (`z<CR>`/`z.`/`z-`) |

*Not bound in nvi (so absent from govi too, correctly): `g` (`v_cmd.c:407` is
`{NULL}`; govi bells "g isn't a vi command"), `K`, `v`, `V`, `^O`, `^_`, `=`.*

---

## 2. Ex commands

| Command | nvi | govi | GoVi.app | Notes |
|---|---|---|---|---|
| `^D` (scroll) | yes | ❌ | ❌ | ex-mode line scroll; not implemented |
| `!` | yes | ✅ | ✅ | filter/run shell command |
| `#` | yes | ✅ | ✅ | display numbered lines |
| `&` | yes | ✅ | ✅ | repeat substitution |
| `*` | yes | ✅ | ✅ | execute buffer (alias of `@`) |
| `<` | yes | ✅ | ✅ | shift left |
| `=` | yes | ✅ | ✅ | print line number |
| `>` | yes | ✅ | ✅ | shift right |
| `@` | yes | ✅ | ✅ | execute buffer |
| `append` | yes | ✅ | ✅ | |
| `abbreviate` | yes | ✅ | ✅ | |
| `args` | yes | ✅ | ✅ | |
| `bg` | yes | ✅ | ✅ | background current screen |
| `change` | yes | ✅ | ✅ | |
| `cd` | yes | ✅ | ✅ | |
| `chdir` | yes | ✅ | ✅ | |
| `copy` | yes | ✅ | ✅ | |
| `cscope` | yes | ✅ | ✅ | add/find/reset/kill/help + `:display connections` |
| `delete` | yes | ✅ | ✅ | |
| `display` | yes | ✅ | ✅ | buffers / connections / screens / tags |
| `edit` | yes | ✅ | ✅ | |
| `ex` | yes | ✅ | ✅ | alias of `:edit` (nvi `:ex` = `ex_edit`, not a mode switch); added to `exCmds` |
| `exusage` | yes | ✅ | ✅ | |
| `file` | yes | ✅ | ✅ | |
| `fg` | yes | ✅ | ✅ | foreground a backgrounded screen |
| `global` | yes | ✅ | ✅ | |
| `help` | yes | ✅ | ✅ | |
| `insert` | yes | ✅ | ✅ | |
| `join` | yes | ✅ | ✅ | |
| `k` | yes | ✅ | ✅ | set mark (alias of `mark`) |
| `list` | yes | ✅ | ✅ | |
| `move` | yes | ✅ | ✅ | |
| `mark` | yes | ✅ | ✅ | |
| `map` | yes | ✅ | ✅ | |
| `mkexrc` | yes | ❌ | ❌ | write current settings to `.exrc`; not implemented |
| `next` | yes | ✅ | ✅ | |
| `number` | yes | ✅ | ✅ | |
| `open` | yes | ❌ | ❌ | out of scope (also unimplemented in nvi) |
| `perl` | yes | ❌ | ❌ | out of scope (scripting) |
| `perldo` | yes | ❌ | ❌ | out of scope (scripting) |
| `preserve` | yes | ✅ | ✅ | |
| `previous` | yes | ✅ | ✅ | |
| `print` | yes | ✅ | ✅ | |
| `put` | yes | ✅ | ✅ | |
| `quit` | yes | ✅ | ✅ | |
| `read` | yes | ✅ | ✅ | |
| `recover` | yes | ✅ | ✅ | |
| `resize` | yes | ✅ | ✅ | grow/shrink current split |
| `rewind` | yes | ✅ | ✅ | |
| `s` (substitute) | yes | ✅ | ✅ | |
| `script` | yes | ❌ | ❌ | out of scope (scripting windows) |
| `set` | yes | ✅ | ✅ | |
| `shell` | yes | ✅ | ✅ | interactive shell (both frontends implement RunShell) |
| `source` | yes | ✅ | ✅ | |
| `stop` | yes | ✅ | — | job control; terminal only |
| `suspend` | yes | ✅ | — | job control; terminal only |
| `t` (copy) | yes | ✅ | ✅ | |
| `tag` | yes | ✅ | ✅ | |
| `tagnext` | yes | ✅ | ✅ | |
| `tagpop` | yes | ✅ | ✅ | |
| `tagprev` | yes | ✅ | ✅ | |
| `tagtop` | yes | ✅ | ✅ | |
| `tcl` | yes | ❌ | ❌ | out of scope (scripting) |
| `undo` | yes | ✅ | ✅ | |
| `unabbreviate` | yes | ✅ | ✅ | |
| `unmap` | yes | ✅ | ✅ | |
| `v` (vglobal) | yes | ✅ | ✅ | |
| `version` | yes | ✅ | ✅ | |
| `visual` / `vi` | yes | ✅ | ✅ | ex->vi switch, and vi-mode `visual` opens a screen |
| `viusage` | yes | ✅ | ✅ | |
| `vsplit` | yes | ✅ | ✅ | |
| `write` | yes | ✅ | ✅ | |
| `wn` | yes | ✅ | ✅ | write and go to next file |
| `wq` | yes | ✅ | ✅ | |
| `xit` | yes | ✅ | ✅ | |
| `yank` | yes | ✅ | ✅ | |
| `z` | yes | ❌ | ❌ | ex-mode window display around a line; not implemented |
| `~` | yes | ✅ | ✅ | |

**Missing ex commands:** `^D`, `mkexrc`, `z` (three real gaps); plus
`open`, `perl`, `perldo`, `script`, `tcl` (out of scope by design).

---

## 3. Options (`:set`)

Functional = the engine reads the option and it changes behavior. Inert (⚙️) =
settable so `:set all` still matches nvi, but nothing reads it yet. Classified by
auditing reads of each name outside `engine/options.go`.

| Option | nvi | govi | GoVi.app | Notes |
|---|---|---|---|---|
| `altwerase` | yes | ⚙️ | ⚙️ | word-erase variant not wired |
| `autoindent` | yes | ✅ | ✅ | |
| `autoprint` | yes | ⚙️ | ⚙️ | ex auto-print not wired |
| `autowrite` | yes | ✅ | ✅ | |
| `backup` | yes | ⚙️ | ⚙️ | |
| `beautify` | yes | ⚙️ | ⚙️ | |
| `cdpath` | yes | ✅ | ✅ | |
| `cedit` | yes | ⚙️ | ⚙️ | colon-line edit char not wired |
| `columns` | yes | ✅ | ✅ | |
| `comment` | yes | ⚙️ | ⚙️ | |
| `directory` | yes | ⚙️ | ⚙️ | temp-file dir not wired |
| `edcompatible` | yes | ⚙️ | ⚙️ | |
| `errorbells` | yes | ⚙️ | ⚙️ | govi always signals errors |
| `escapetime` | yes | ⚙️ | ⚙️ | key-timing not wired |
| `exrc` | yes | ✅ | ✅ | |
| `extended` | yes | ⚙️ | ⚙️ | extended-regex toggle not wired |
| `filec` | yes | ✅ | ✅ | file-name completion char |
| `flash` | yes | ⚙️ | ⚙️ | visual bell not wired |
| `hardtabs` | yes | ⚙️ | ⚙️ | |
| `iclower` | yes | ⚙️ | ⚙️ | |
| `ignorecase` | yes | ✅ | ✅ | |
| `keytime` | yes | ⚙️ | ⚙️ | |
| `leftright` | yes | ⚙️ | ⚙️ | left-right scroll mode not wired |
| `lines` | yes | ✅ | ✅ | |
| `lisp` | yes | ⚙️ | ⚙️ | out of scope (barely in nvi) |
| `list` | yes | ✅ | ✅ | |
| `lock` | yes | ✅ | ✅ | |
| `magic` | yes | ✅ | ✅ | |
| `matchtime` | yes | ✅ | ✅ | |
| `mesg` | yes | ⚙️ | ⚙️ | |
| `msgcat` | yes | ⚙️ | ⚙️ | message catalogs not used |
| `number` | yes | ✅ | ✅ | |
| `octal` | yes | ⚙️ | ⚙️ | |
| `open` | yes | ⚙️ | ⚙️ | out of scope |
| `optimize` | yes | ⚙️ | ⚙️ | out of scope (terminal draw hint) |
| `paragraphs` | yes | ⚙️ | ⚙️ | `{`/`}` use built-in defaults; custom string ignored |
| `path` | yes | ⚙️ | ⚙️ | |
| `print` | yes | ⚙️ | ⚙️ | option value never read; printable-char display is hardcoded, not driven by this option |
| `prompt` | yes | ⚙️ | ⚙️ | ex `:` prompt always shown |
| `readonly` | yes | ✅ | ✅ | |
| `recdir` | yes | ✅ | ✅ | recovery directory |
| `redraw` | yes | ⚙️ | ⚙️ | out of scope (terminal draw hint) |
| `remap` | yes | ⚙️ | ⚙️ | maps are NON-remapping (RHS sent literally, `maps.go:163`); they never expand recursively |
| `report` | yes | ✅ | ✅ | |
| `ruler` | yes | ✅ | ✅ | |
| `scroll` | yes | ⚙️ | ⚙️ | `^D`/`^U` use computed half-page |
| `searchincr` | yes | ⚙️ | ⚙️ | no incremental search |
| `sections` | yes | ⚙️ | ⚙️ | `[[`/`]]` use built-in defaults |
| `secure` | yes | ✅ | ✅ | |
| `shell` | yes | ✅ | ✅ | |
| `shellmeta` | yes | ✅ | ✅ | |
| `shiftwidth` | yes | ✅ | ✅ | |
| `showmatch` | yes | ✅ | ✅ | |
| `showmode` | yes | ✅ | ✅ | |
| `sidescroll` | yes | ⚙️ | ⚙️ | |
| `slowopen` | yes | ⚙️ | ⚙️ | out of scope |
| `sourceany` | yes | ⚙️ | ⚙️ | |
| `tabstop` | yes | ✅ | ✅ | |
| `taglength` | yes | ✅ | ✅ | |
| `tags` | yes | ✅ | ✅ | |
| `term` | yes | ⚙️ | ⚙️ | frontend owns the terminal type |
| `terse` | yes | ⚙️ | ⚙️ | |
| `tildeop` | yes | ✅ | ✅ | |
| `timeout` | yes | ⚙️ | ⚙️ | |
| `ttywerase` | yes | ⚙️ | ⚙️ | |
| `verbose` | yes | ⚙️ | ⚙️ | |
| `warn` | yes | ⚙️ | ⚙️ | |
| `window` | yes | ✅ | ✅ | |
| `windowname` | yes | ⚙️ | ⚙️ | |
| `wraplen` | yes | ⚙️ | ⚙️ | `wrapmargin` is used instead |
| `wrapmargin` | yes | ✅ | ✅ | |
| `wrapscan` | yes | ✅ | ✅ | |
| `writeany` | yes | ✅ | ✅ | |

### Options nvi has that govi omits entirely  *(8)*

| Option | nvi | govi | GoVi.app | Notes |
|---|---|---|---|---|
| `combined` | yes | ❌ | ❌ | internal combined-char display flag (`OPT_NOSET`) |
| `fileencoding` | yes | ❌ | ❌ | encoding support not ported |
| `inputencoding` | yes | ❌ | ❌ | encoding support not ported |
| `modeline` | yes | ❌ | ❌ | deprecated/off in nvi; not ported |
| `noprint` | yes | ❌ | ❌ | companion to `print`; not ported |
| `w300` | yes | ❌ | ❌ | baud-rate window alias (`OPT_NDISP`); omitted |
| `w1200` | yes | ❌ | ❌ | baud-rate window alias (`OPT_NDISP`); omitted |
| `w9600` | yes | ❌ | ❌ | baud-rate window alias (`OPT_NDISP`); omitted |

### govi-only extension options (not in nvi)

| Option | nvi | govi | GoVi.app | Notes |
|---|---|---|---|---|
| `foreground` | no | ⚙️ | ✅ | GUI text color; inert in terminal |
| `background` | no | ⚙️ | ✅ | GUI background color; inert in terminal |
| `mode` | no | ⚙️ | ✅ | GUI selection mode (terminal/gui/contextual) |
| `refresh` | no | ✅ | — | tcell repaint-throttle interval; GUI drives its own repaint |

---

## 4. Frontend-specific summary

| Feature | govi | GoVi.app | Notes |
|---|---|---|---|
| Split screens (`^W`, `:vsplit`, `:E`, `:resize`) | ✅ | ✅ | multi-pane rendering added to the grid frontend |
| Job control (`^Z`, `:stop`, `:suspend`) | ✅ | — | GUI does not implement `Suspender` |
| Shell escape (`:!`, `:shell`, `!` filter) | ✅ | ✅ | both frontends implement `RunShell` |
| Screen backgrounding (`:bg`/`:fg`) | ✅ | ✅ | engine-level screen management |
| Redraw (`^L`/`^R`) | ✅ | — | terminal forces a full `Sync()` to recover a corrupted tty; ordinary paints still diff to changed cells and coalesce bursts (`refresh`). GUI has no tty to corrupt |

---

## Bottom line

- **Vi command mode:** complete. Every key nvi binds is implemented, including
  `^L`/`^R` (force full redraw via tcell `Sync()`, for recovering a tty another
  program corrupted). Normal editing still uses diffed, burst-coalesced paints.
- **Ex commands:** 71 of 74 in-scope commands done (`:display` now covers all four
  subcommands, including `tags`). Genuine gaps: `^D` scroll, `:mkexrc`, `:z`. Out
  of scope: `:open`, `:perl`, `:perldo`, `:script`, `:tcl`.
- **Options:** all 73 shared nvi options are settable; ~31 are functional and ~42
  are inert placeholders (several inert by design: scripting/terminal-optimization
  hints). 8 nvi options are omitted entirely (encoding, a couple
  internal/deprecated flags, and the `w300`/`w1200`/`w9600` baud-rate window
  aliases), so govi's `:set all` is not a literal superset of nvi's. govi adds 4
  options of its own for the GUI/renderer.
