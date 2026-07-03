# govi vs nvi — feature parity

Tracks what **nvi** — Keith Bostic's original 4.4BSD nex/vi (the 1.81.x
reference) — provides, against **govi**, this Go reimplementation. The **nvi**
column always refers to that original nvi.

This matrix is an independent re-derivation of parity, built by reading the
authoritative tables in both source trees:

- **nvi** commands from `nvi/vi/v_cmd.c` (`vikeys[]`), `nvi/ex/ex_cmd.c`
  (`cmds[]`), and options from `nvi/common/options.c` (`optlist[]`).
- **govi** from `engine/vi.go` + `engine/vimotion*.go` (vi dispatch),
  `engine/ex.go` (`exCmds[]`), and `engine/options.go` (`optDefs[]`).

govi ships two frontends over one shared engine, so each gets its own column:

- **govi** — the terminal editor.
- **GoVi.app** — the native macOS GUI (renders the same engine grid).

Editing behavior is identical between the two; the columns diverge only for
frontend-specific features (job control, redraw, GUI-only options).

The authoritative behavior spec is the official manual in [`nvi.md`](nvi.md);
[`nvi-index.md`](nvi-index.md) maps every command/option to its line there, so a
parity row can be checked against the source description quickly.

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
| `^C` | yes | ✅ | ✅ | interrupt. Both frontends cancel a partial `:` line and abort an in-progress search / `:s` / `:g` / `:!` (kills the child) / `:w`. GoVi.app runs engine input on a serial queue, so a ^C reaches `Engine.Interrupt` while a command is running |
| `^D` | yes | ✅ | ✅ | scroll down half screen (sets count) |
| `^E` | yes | 🟡 | 🟡 | scroll down by lines; matches nvi on non-wrapping files, but scrolls by logical line rather than screen row, so wrapped lines differ (GOTERM_DIVERGENCES #44) |
| `^F` | yes | ✅ | ✅ | scroll down by screens |
| `^G` | yes | 🟡 | 🟡 | file status; a message too long for one line lands truncated on govi's status line where nvi pages it into a `+=+=` overlay (info-message pagination, GOTERM_DIVERGENCES "Inconclusive") |
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
| `^\` | yes | 🟡 | 🟡 | switch to ex mode; same ex-screen layout difference as `Q` |
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
| `Q` | yes | 🟡 | 🟡 | switch to ex mode; works, but govi clears to a `:` prompt screen where nvi keeps the buffer text visible (GOTERM_DIVERGENCES #29 cosmetic note) |
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
| `*` | yes | ✅ | ✅ | execute buffer (alias of `@`). Deliberate difference: nvi's bare `:*` carries no default address, so address-taking buffer contents fail ("address of 0"); govi runs the buffer like `:@` |
| `<` | yes | ✅ | ✅ | shift left |
| `=` | yes | ✅ | ✅ | print line number |
| `>` | yes | ✅ | ✅ | shift right |
| `@` | yes | ✅ | ✅ | execute buffer |
| `append` | yes | 🟡 | 🟡 | buffer result correct; nvi enters its scrolled "ex input mode" display, govi keeps the vi screen (GOTERM_DIVERGENCES #28) |
| `abbreviate` | yes | ✅ | ✅ | |
| `args` | yes | 🟡 | 🟡 | list correct; a list too long for one line meets the info-message pagination gap (see `^G`) |
| `bg` | yes | ✅ | ✅ | background current screen |
| `change` | yes | 🟡 | 🟡 | same ex-input display difference as `append` (#28) |
| `cd` | yes | ✅ | ✅ | |
| `chdir` | yes | ✅ | ✅ | |
| `copy` | yes | ✅ | ✅ | |
| `cscope` | yes | ✅ | ✅ | add/find/reset/kill/help + `:display connections` |
| `delete` | yes | ✅ | ✅ | |
| `display` | yes | 🟡 | 🟡 | buffers / connections / screens / tags all answer; `screens` matches, but `buffers` omits nvi's default-buffer row and mode annotations, and `tags` omits the origin stack frame and aligns differently |
| `edit` | yes | ✅ | ✅ | |
| `ex` | yes | ✅ | ✅ | alias of `:edit` (nvi `:ex` = `ex_edit`, not a mode switch); added to `exCmds` |
| `exusage` | yes | ✅ | ✅ | |
| `file` | yes | 🟡 | 🟡 | same long-message pagination caveat as `^G` |
| `fg` | yes | ✅ | ✅ | foreground a backgrounded screen |
| `global` | yes | ✅ | ✅ | |
| `help` | yes | ✅ | ✅ | |
| `insert` | yes | 🟡 | 🟡 | same ex-input display difference as `append` (#28) |
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
| `preserve` | yes | ✅ | ✅ | snapshot goes to `recdir` and survives a clean exit for `-r` recovery; govi writes no companion recover-mail file (nvi's second entry) |
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
| `visual` / `vi` | yes | ✅ | ✅ | ex->vi switch; vi-mode `:vi[sual] file` edits that file (nvi C_VISUAL_VI is `ex_edit`), `:Vi` opens it in a split |
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
| `autowrite` | yes | ✅ | ✅ | honored by the nvi `file_m1` family (`:n`/`:prev`/`:rew`, tag jumps/pop/push, `^^`) and suspend; the historic `:!`-warns-first corner is not wired |
| `backup` | yes | ⚙️ | ⚙️ | |
| `beautify` | yes | ⚙️ | ⚙️ | |
| `cdpath` | yes | ✅ | ✅ | |
| `cedit` | yes | ⚙️ | ⚙️ | colon-line edit char not wired |
| `columns` | yes | ✅ | ✅ | |
| `comment` | yes | ⚙️ | ⚙️ | |
| `directory` | yes | ⚙️ | ⚙️ | temp-file dir not wired; default follows `$TMPDIR` like nvi |
| `edcompatible` | yes | ⚙️ | ⚙️ | |
| `errorbells` | yes | ⚙️ | ⚙️ | govi always signals errors |
| `escapetime` | yes | ⚙️ | ⚙️ | key-timing not wired |
| `exrc` | yes | ✅ | ✅ | |
| `extended` | yes | ⚙️ | ⚙️ | extended-regex toggle not wired |
| `filec` | yes | ✅ | ✅ | file-name completion char; completion verified equal, but govi defaults it to `<tab>` (completion on out of the box) where nvi's default is empty |
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
| `scroll` | yes | ⚙️ | ⚙️ | matches nvi in vi mode anyway: nvi's `^D`/`^U` use `defscroll` (a `^D` count), not this option; nvi reads `scroll` only in ex contexts govi lacks (`:z` sizing, ex `^D`). `:set all` shows the nvi-derived default (window/2) |
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
| `window` | yes | ✅ | ✅ | `:set window=N` resizes the vi map immediately (small-screen growth like `z<count>`) and drives the `^F`/`^B` page distance (`count*window-2`); tracks the terminal on resize unless explicitly set |
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

## 4. Command-line invocation

nvi parses `[-eFlRrSsv] [-c command] [-t tag] [-w size] [file ...]`
(`common/main.c`), plus the historic forms `+cmd` (= `-c cmd`) and a bare `-`
(= `-s`), and keys the startup mode off argv[0]: `ex`/`nex` start in ex mode,
`view`/`nview` start readonly.

| Invocation | nvi | govi | Notes |
|---|---|---|---|
| `file ...` args | yes | ✅ | multiple files build the args list |
| no file | yes | ✅ | throwaway `$TMPDIR/vi.*` buffer, removed on exit |
| argv[0] `ex`/`nex` -> ex mode | yes | ✅ | govi adds the `goex` spelling (symlink/hardlink to the binary) |
| `-e` (ex mode) / `-v` (vi mode) | yes | ✅ | with both, govi lets `-v` win (nvi: last one wins) |
| argv[0] `view`/`nview` -> readonly | yes | ✅ | govi adds the `goview` spelling |
| `-R` readonly | yes | ✅ | sets the functional `readonly` option |
| `-r` recover (list / named file) | yes | ✅ | same two forms |
| `-c command` / `+cmd` | yes | ✅ | runs after the file loads; may exit (`-c wq`); one -c only, like nvi |
| `-t tag` | yes | ✅ | excludes `-r`, like nvi; with file args nvi puts the tag file first in the args list, govi jumps after opening them |
| `-w size` | yes | ✅ | records the size pre-terminal; the first resize clamps and applies it (nvi f_window) |
| `-s` batch/silent (ex only) | yes | ✅ | headless ex script on stdin, no prompts; implied by redirected stdin in ex mode (nvi G_SCRIPTED); errors in vi mode like nvi. Verified against the oracle binary-to-binary by `internal/conformance` TestExBatchBinaryConformance |
| `-S` secure | yes | ✅ | sets the functional `secure` option |
| `-l` lisp + showmatch | yes | ✅ | `showmatch` functional; `lisp` inert in both editors |
| `-F` (no snapshot) | error msg | ✅ | prints the same "no longer supported" warning and continues |
| `-` (= `-s`) historic | yes | ✅ | translated like `+cmd` (nvi v_obsolete) |
| `-n` skip startup files/EXINIT | no | ✅ | govi extension (nvi has no such flag; its `-s` also skips startup as part of batch mode, as does govi's) |
| `-g` / `-G` GUI launch | no | ✅ | govi extensions (GoVi.app); `-G` = launch and wait |

---

## 5. Frontend-specific summary

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
  The 🟡 rows above are display-layer differences (wrapped-line scrolling,
  ex-screen layout, info-message pagination), not behavior gaps.
- **Ex commands:** 71 of 74 in-scope commands done. Genuine gaps: `^D` scroll,
  `:mkexrc`, `:z`. Out of scope: `:open`, `:perl`, `:perldo`, `:script`,
  `:tcl`. `:display` answers all four subcommands but formats `buffers` and
  `tags` differently from nvi.
- **Options:** all 73 shared nvi options are settable; ~31 are functional and ~42
  are inert placeholders (several inert by design: scripting/terminal-optimization
  hints). 8 nvi options are omitted entirely (encoding, a couple
  internal/deprecated flags, and the `w300`/`w1200`/`w9600` baud-rate window
  aliases), so govi's `:set all` is not a literal superset of nvi's. govi adds 4
  options of its own for the GUI/renderer.
- **Command line:** full nvi parity (section 4): every nvi flag including ex
  batch `-s`, the `ex`/`view` program names (plus `goex`/`goview` spellings),
  and the historic `+cmd`/`-` forms. govi adds `-n` (skip startup files) and
  the `-g`/`-G` GUI launchers.
- **Evidence:** every row of this matrix was verified against nvi 1.81.6 through
  the goterm harness (or by source reading where the PTY model cannot drive a
  feature) in the 2026-07 review; see [`parity-review.md`](parity-review.md) for
  the row-by-row evidence and what the review fixed.
