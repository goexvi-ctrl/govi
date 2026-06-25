# govi vs nvi — feature parity

Tracks what **nvi** — Keith Bostic's original 4.4BSD nex/vi (the 1.81.x
reference) — provides, against **govi**, this Go reimplementation. The **nvi**
column always refers to that original nvi.

govi ships two frontends over one shared engine, so each gets its own column:

- **govi** — the terminal editor.
- **Govi.app** — the native macOS GUI.

Editing behavior is identical between the two (same engine); the columns differ
only for frontend-specific features such as job control, screen repaint, the
shell escape, and the GUI itself. The goal is user-perceptible bug-for-bug
parity with nvi; rows are validated against the nvi oracle where marked.

The authoritative behavior spec is the official manual in [`nvi.md`](nvi.md);
[`nvi-index.md`](nvi-index.md) maps every command/option to its line there, so a
parity row can be checked against the source description quickly.

**Status legend** (per frontend)

| Status | Meaning |
|--------|---------|
| ✅ Done | Implemented; matches nvi (✔ = also covered by an engine oracle conformance test) |
| 🟡 Partial | Implemented with known gaps or simplifications |
| ⚙️ Inert | Recognized/settable but does not yet drive behavior |
| ❌ Not yet | Not implemented |
| — N/A | Not applicable to this frontend, or out of scope for the port |

---

## Vi command-mode commands

| Command | Description | nvi | govi | Govi.app | Notes |
|---|---|---|---|---|---|
| `^A` | search forward for word under cursor | yes | ✅ | ✅ | |
| `^B` `^F` | page up / down | yes | ✅ | ✅ | |
| `^D` `^U` | scroll down / up half-screen | yes | ✅ | ✅ | maintains column |
| `^E` `^Y` | scroll one line, cursor fixed | yes | 🟡 | 🟡 | scrolls; cursor-follow simplified |
| `^G` | file information | yes | ✅ | ✅ | |
| `^H` `h` / `l` `space` | left / right | yes | ✅✔ | ✅✔ | |
| `^J` `^N` `j` / `^P` `k` | down / up | yes | ✅✔ | ✅✔ | logical-line; maintains display column |
| `^L` `^R` | repaint screen | yes | 🟡 | — | terminal: no-op (frontend repaints every input); GUI repaints automatically |
| `^M` `+` / `-` | next / prev line, first non-blank | yes | ✅ | ✅ | |
| `^T` `^]` | tag pop / tag push | yes | ✅✔ | ✅✔ | ctags `tags` file |
| `^W` | switch screens | yes | ❌ | ❌ | no split screens |
| `^Z` | suspend | yes | ✅ | — | terminal job control only; blocked when `secure` |
| `^^` | alternate file | yes | ✅✔ | ✅✔ | |
| `:` | ex command line | yes | ✅✔ | ✅✔ | |
| `/` `?` `n` `N` | search / repeat | yes | ✅✔ | ✅✔ | wrapscan honored |
| `!` | filter through shell | yes | ✅✔ | ✅✔ | `!motion` + `:range!cmd` |
| `#` | increment/decrement number | yes | ✅✔ | ✅✔ | `#+` `#-` |
| `$` `0` `^` `_` `\|` | line column motions | yes | ✅✔ | ✅✔ | `$` sticky to EOL |
| `%` | match bracket | yes | ✅✔ | ✅✔ | nests across lines |
| `&` | repeat last substitute | yes | ✅✔ | ✅✔ | |
| `` ` `` `'` | marks (exact / line) | yes | ✅✔ | ✅✔ | |
| `(` `)` `{` `}` `[[` `]]` | sentence / paragraph / section | yes | ✅✔ | ✅✔ | exclusive→linewise operator promotion |
| `;` `,` | repeat / reverse `f F t T` | yes | ✅ | ✅ | |
| `.` | repeat last change | yes | ✅✔ | ✅✔ | with count override |
| `<` `>` | shift left / right | yes | ✅✔ | ✅✔ | tab-aware indent |
| `@` | execute register as commands | yes | ✅✔ | ✅✔ | |
| `~` | reverse case (count or, with `tildeop`, motion) | yes | ✅✔ | ✅✔ | |
| `a A i I o O` | enter insert | yes | ✅✔ | ✅✔ | |
| `b B w W e E` | word / WORD motions | yes | ✅✔ | ✅✔ | |
| `c C d D s S y Y` | change/delete/subst/yank | yes | ✅✔ | ✅✔ | `cw`→`ce` special case |
| `f F t T` | find char in line | yes | ✅✔ | ✅✔ | |
| `G H M L` | goto line / screen positions | yes | ✅✔ | ✅✔ | |
| `J` | join lines | yes | ✅✔ | ✅✔ | |
| `m` | set mark | yes | ✅ | ✅ | |
| `p P` | put | yes | ✅✔ | ✅✔ | char/line-wise, count |
| `Q` | switch to ex mode | yes | 🟡 | 🟡 | terminal: scrolling line REPL; GUI: bottom-growing transcript; `:visual` returns |
| `r R` | replace char / replace mode | yes | ✅✔ | ✅✔ | |
| `U` | restore line | yes | ✅✔ | ✅✔ | undoes the run of changes on the current line |
| `u` | undo (toggle) | yes | ✅✔ | ✅✔ | directional `u`/`.` model |
| `x X` | delete char | yes | ✅✔ | ✅✔ | |
| `z` | screen positioning (`z↵` `z.` `z-` `[line]z` `z[count]`) | yes | 🟡 | 🟡 | wrap-aware center/bottom; `[line]z[count]` small map (blank below, grows on `j`); `z[count]` types equivalent; no `z^`/`z+` scroll |
| `ZZ` `ZQ` | save-quit / quit | yes | ✅ | ✅ | |
| `<interrupt>` | interrupt current operation | yes | 🟡 | 🟡 | searches/interrupts; not all operations cancellable |

## Vi text-input-mode commands

| Command | Description | nvi | govi | Govi.app | Notes |
|---|---|---|---|---|---|
| `NUL` (`^@`) | replay previous input | yes | ✅ | ✅ | |
| `^D` / `^T` | autoindent erase / shift to shiftwidth | yes | 🟡 | 🟡 | implemented as line shift left/right |
| `0^D` `^^D` | erase all / reset autoindent | yes | ❌ | ❌ | |
| `^H` / erase | erase last character | yes | ✅ | ✅ | |
| `^V` | quote next character | yes | ✅ | ✅ | |
| `^W` | erase last word | yes | ✅✔ | ✅✔ | |
| `^X` | insert hex character code | yes | ✅ | ✅ | |
| line erase | erase the input line | yes | ❌ | ❌ | |
| `<esc>` | end input | yes | ✅✔ | ✅✔ | |
| autoindent | leading-whitespace carry | yes | ✅✔ | ✅✔ | `o`/`O` and `↵` |
| abbreviations | expand on word break | yes | ✅✔ | ✅✔ | |

## Ex commands

| Command | nvi | govi | Govi.app | Notes |
|---|---|---|---|---|
| `:[range]d[elete]` | yes | ✅✔ | ✅✔ | buffer + count |
| `:[range]m[ove]` / `:[range]co`/`t` | yes | ✅✔ | ✅✔ | |
| `:[range]y[ank]` / `:[line]pu[t]` | yes | ✅✔ | ✅✔ | |
| `:[range]j[oin]` | yes | ✅✔ | ✅✔ | |
| `:[range]<` `:[range]>` | yes | ✅✔ | ✅✔ | |
| `:[range]s[ubstitute]` | yes | ✅✔ | ✅✔ | `g` flag, `&`, `\1`-`\9`, `\u\l\U\L\E`, `\n` |
| `:[range]g[lobal]` / `:v` | yes | ✅✔ | ✅✔ | |
| `:[line]=` | yes | ✅ | ✅ | |
| `:[range]p[rint]`/`nu[mber]`/`l[ist]` | yes | ✅ | ✅ | output via overlay/transcript |
| `:w[rite]` `:wq` `:x[it]` `:q[uit]` | yes | ✅✔ | ✅✔ | `!`, `:[range]w !cmd`, dirty guard (incl. insert-mode pending edits); temporary-buffer exit warning |
| `:r[ead] file` `:r !cmd` | yes | ✅✔ | ✅✔ | |
| `:[range]!cmd` / `:!cmd` | yes | ✅✔ | ✅✔ | |
| `:set` / `:set all` / `:set opt` | yes | ✅✔ | ✅✔ | full option registry, grid display |
| `:map` `:map!` `:unmap` | yes | ✅✔ | ✅✔ | non-recursive |
| `:ab[breviate]` `:una[bbreviate]` | yes | ✅✔ | ✅✔ | |
| `:e[dit]` `:n[ext]` `:prev`/`:N` `:rew[ind]` `:ar[gs]` | yes | ✅✔ | ✅✔ | argument list |
| `:f[ile] [name]` | yes | ✅ | ✅ | status line; optional rename sets alternate file |
| `:ta[g]` | yes | ✅✔ | ✅✔ | |
| `:tagn[ext]` `:tagp[rev]` `:tagt[op]` | yes | ❌ | ❌ | tag-stack walk (vi `^T`/`^]` work) |
| `:vi[sual]` | yes | ✅ | ✅ | returns from ex mode |
| `Q` ex (line) mode | yes | ✅ | ✅ | terminal leaves the full screen for a scrolling line REPL (no banner); GUI shows an equivalent bottom-growing scrolling transcript |
| `:[range]a[ppend]`/`i[nsert]`/`c[hange]` | yes | ✅ | ✅ | ex input mode; input ends on a sole `.` (works in ex mode and from the colon line) |
| `:cd`/`:chdir` | yes | ✅ | ✅ | per-tab cwd; GUI also follows tab focus |
| `:so[urce]` | yes | ✅ | ✅ | reads ex commands from a file |
| `:mk[exrc]` | yes | ❌ | ❌ | write current options to an exrc file |
| `:k`/`:ma` (mark) | yes | ❌ | ❌ | (vi `m` works) |
| `:u[ndo]` | yes | ❌ | ❌ | (vi `u` works) |
| `:di[splay] b\|c\|s\|t` | yes | ❌ | ❌ | buffers/screens/tags inspector |
| `:he[lp]` | yes | ✅ | ✅ | points to :viusage / :exusage |
| `:exu[sage] [cmd]` | yes | ✅ | ✅ | lists implemented ex commands |
| `:viu[sage] [key]` | yes | ✅ | ✅ | lists implemented vi keys |
| `:o[pen]` | yes | — | — | non-objective (also unimplemented in nvi); distinct from vi `o` |
| `:bg` `:fg` `:res[ize]` | yes | ❌ | ❌ | needs split screens |
| `:su[spend]`/`:st[op]` | yes | ✅ | — | terminal only; `!` skips autowrite; blocked when `secure` |
| `:cs[cope]` | yes | — | — | cscope integration; out of scope |
| `:pre[serve]` `:rec[over]` | yes | ✅ | ✅ | crash recovery (govi format) |
| `:ve[rsion]` | yes | ✅ | ✅ | git-derived build metadata (`govi-0.1`, date, hash) |
| `:@`/`:*` (exec buffer) `:w>>` `:wn` etc. | yes | 🟡 | 🟡 | partial |
| `:sh[ell]` | yes | ✅ | ❌ | terminal spawns an interactive shell (`tcell` suspend); not implemented in Govi.app; blocked when `secure` |

## Options

govi carries the full nvi option table (74 options, matching the reference
manual's Set Options section) — all are settable, queryable, and shown by
`:set all`. The ones that drive behavior:

| Option | nvi | govi | Govi.app | Notes |
|---|---|---|---|---|
| `autoindent` (ai) | yes | ✅✔ | ✅✔ | |
| `ignorecase` (ic) | yes | ✅✔ | ✅✔ | search/substitute |
| `magic` | yes | ✅ | ✅ | regex syntax |
| `wrapscan` (ws) | yes | ✅✔ | ✅✔ | search wrap |
| `tabstop` (ts) | yes | ✅✔ | ✅✔ | display + indent |
| `shiftwidth` (sw) | yes | ✅✔ | ✅✔ | `<` `>` and `^T`/`^D` |
| `tildeop` (to) | yes | ✅✔ | ✅✔ | |
| `tags` | yes | ✅✔ | ✅✔ | |
| `number` (nu) | yes | ✅ | ✅ | gutter rendered |
| `list` | yes | ✅ | ✅ | tabs as ^I, controls as ^X, trailing $ |
| `showmatch` (sm) | yes | ✅✔ | ✅✔ | bracket flash on insert (matchtime) |
| `filec` | yes | ✅ | ✅ | file-name completion character on the `:` line |
| `columns`/`lines` | yes | ✅ | ✅ | track terminal / window geometry |
| `shell` | yes | ✅ | ✅ | used by `!` filter and `:shell` |
| `exrc` | yes | ✅ | ✅ | read `./.nexrc`/`./.exrc` at startup (ownership-checked) |
| `foreground`/`background` (fg/bg) | — | ⚙️ | ✅ | per-tab text colors in Govi.app; settable but inert in the terminal |
| `lisp`, `redraw`, `slowopen`/`slow`, `optimize`/`opt` | yes | — | — | non-objectives (see below); settable but never drive behavior |
| `autowrite` (aw) | yes | ❌ | ❌ | auto-write on file/tag/navigation commands |
| `backup` | yes | ❌ | ❌ | backup file path and versioning |
| `lock` | yes | ❌ | ❌ | file locking before write |
| `recdir` | yes | ✅ | ✅ | recovery directory for crash-recovery files |
| `writeany` (wa) | yes | ❌ | ❌ | override ownership checks on write |
| `ruler` | yes | ✅ | ✅ | row/column on status line when no message |
| `showmode` (smd) | yes | ✅ | ✅ | mode indicator on status line; `*` when modified |
| `secure` | yes | 🟡 | 🟡 | blocks `:shell` when set; `!` filters still run |
| `matchtime` (mt) | yes | ✅ | ✅ | showmatch flash duration (tenths of a second) |
| `report` | yes | ⚙️ | ⚙️ | change-report threshold (recognized; used by `:r` line count) |
| `octal` | yes | ⚙️ | ⚙️ | unknown char display format (recognized, inert) |
| all other options | yes | ⚙️ | ⚙️ | recognized/settable, inert |

## Subsystems

| Subsystem | nvi | govi | Govi.app | Notes |
|---|---|---|---|---|
| Large-file editing | recno DB paging | ✅ | ✅ | paged piece-table line store; multi-GB |
| Undo / redo | yes | ✅✔ | ✅✔ | multi-level; nvi directional `u`/`.` |
| Marks | yes | 🟡 | 🟡 | line-granular fixups; intra-line column fixup partial |
| Registers / cut buffers | yes | ✅✔ | ✅✔ | named a-z (A-Z append), numbered 1-9 |
| Regex engine | BRE + extensions | ✅✔ | ✅✔ | backrefs, `\<`/`\>` (incl. Spencer's `[[:<:]]`/`[[:>:]]` word-boundary kludge), intervals, classes; `+?(){}\|` and `\+\?\w\W` literal as in nvi BRE. Pinned by a ~55-case `:s`/`:g` battery vs the oracle. (The Homebrew nvi binary's POSIX `[[:class:]]` is broken — Spencer's source is correct — so govi follows the source, a deliberate divergence from that binary.) |
| Search | yes | ✅✔ | ✅✔ | line-oriented, wrapscan |
| Maps / abbreviations | yes | 🟡 | 🟡 | non-recursive (noremap) |
| Multiple files (arg list) | yes | ✅✔ | ✅✔ | |
| Tags | yes | ✅✔ | ✅✔ | ctags file; tag stack |
| Wide / multibyte display | wchar | ✅✔ | ✅✔ | East Asian width = 2 cols |
| Line wrapping | yes | 🟡 | 🟡 | wraps; no `@`-fill for partial bottom line |
| Left-right scrolling (`leftright`/`sidescroll`) | yes | ❌ | ❌ | always wraps; no horizontal-scroll mode |
| Cursor column maintenance | yes | ✅✔ | ✅✔ | display column, sticky `$` |
| File-name completion | yes | ✅ | ✅ | Tab completion on the `:` line (`filec`); absolute paths + ambiguity bell |
| Command-line editing (`cedit`) | yes | ❌ | ❌ | no ex command-line edit window |
| Embeddable engine boundary | function-pointer table | ✅ | ✅ | Go `Frontend`/`View`; tcell + headless + native GUI frontends |
| Crash recovery (`-r`) | yes | ✅ | ✅ | `govi -r` lists recoverable files; `govi -r file` restores; `:preserve`/`:recover`; govi format (GUI syncs after idle) |
| Startup files (`/etc/vi.exrc`, `~/.nexrc`/`.exrc`, `EXINIT`/`NEXINIT`) | yes | ✅ | ✅ | read at startup unless `-s`; ownership/permission checked; honors `exrc`; `:source` |
| Signals (SIGHUP/SIGTERM/…) | yes | ✅ | — | terminal: trap, restore cooked tty, print signal name; `^\` vi→ex; GUI uses the AppKit lifecycle |
| Split screens / windows (`^W` `:bg`/`:fg`/`:resize`) | yes | ❌ | ❌ | |
| Job control (`^Z` `:suspend`/`:stop`) | yes | ✅ | — | terminal frontend (`tcell`); not Govi.app |
| Cscope integration | yes | — | — | out of scope |
| Message catalogs (i18n) | yes | — | — | English only; out of scope |
| File encodings | iconv | 🟡 | 🟡 | UTF-8 only |
| Perl / Tcl scripting | yes | — | — | non-objective (see below) |
| Ex addressing | yes | ✅✔ | ✅✔ | `.`, `$`, `N`, `'mark`, `/pat/`, `?pat?`, offsets, `%` range |


## Govi.app additions

Govi.app embeds the same engine in a native macOS (AppKit) app, so it adds GUI
affordances the terminal frontend has no place for. These are extras on top of
the shared editor, not nvi-parity items:

| Feature | Notes |
|---|---|
| Native app embedding | engine runs in-process behind a C archive (`gui/bridge`); no terminal, no exec of `govi` |
| Multiple windows / native tabs | Cmd-N, Cmd-T, drag/merge tabs; `Use window tabs` setting |
| Mouse + system clipboard | click to position, drag-select, double/triple-click word/line, Cmd-C/X/V, Cmd-A |
| Spell checking | NSSpellChecker underline, suggestions, Ignore/Learn, Look Up |
| International input | Option/dead keys and IME composition; control keys stay vi commands |
| Per-tab colors | `:set foreground=`/`background=` and Settings defaults |
| Settings window (Cmd-,) | padding, default rows/cols, font + size, colors, open-in tab/window, tabs on/off, unsaved-close warning, title-bar dimensions |
| Font size shortcuts | Cmd-= / Cmd-- adjust the font; the window resizes to keep its rows × cols |
| `govi -g` launcher | open files in a running app (tabs/windows), `-w` to block as `$EDITOR`; no file opens an nvi-style temp buffer |
| Wheel / trackpad scrolling | viewport scrolls like a normal windowed app |

Govi.app is macOS-only: nvi's **Motif** and **GTK** GUI backends are **not
implemented** (and are not planned). nvi's separate-process GUI **IPC** protocol
is a non-objective (below) — govi embeds the engine in-process instead.

## Non-objectives (explicitly out of scope)

These nvi features are deliberately **not** goals of govi. They are listed for
completeness and to keep them from being mistaken for unfinished work (❌). The
options among them remain settable so `:set all` matches nvi, but they will
never drive behavior.

| Feature | Why excluded |
|---|---|
| Tcl/Tk scripting (`engine/10`) | scripting embedding is out of scope |
| Perl scripting | scripting embedding is out of scope |
| nvi GUI IPC protocol | nvi drives a separate GUI process over an IPC channel; govi embeds the engine in-process (`gui/bridge`) for Govi.app instead |
| Motif / GTK GUI backends | govi's GUI is macOS-only (Govi.app); see "Govi.app additions" |
| `lisp` mode | a no-op in nvi itself; nothing to match |
| `redraw` option | terminal-optimization hint; govi repaints every input |
| `slowopen` / `slow` option | slow-terminal drawing mode; irrelevant to govi's renderer |
| `optimize` / `opt` option | terminal-output optimization; irrelevant to govi's renderer |
| Ex `:open` command | unimplemented in nvi too; not the vi-mode `o` command |
| `modelines` option | security hazard; will never be implemented |
| `sourceany` option | security hazard; will never be implemented |
| `mesg` option | terminal messaging control; irrelevant to govi's frontends |
| `iclower` option | case-insensitive fallback; superseded by `ignorecase` |
| `comment` option | skip comment on file open; not implemented |
| `path` option | file search path; not used |
| `window`, `w300`, `w1200`, `w9600` | baud-rate window sizing; irrelevant to modern displays |
| `scroll` option | ex-mode scroll count; not used |
| `searchincr` option | incremental search display; not implemented |
| `terse`, `verbose` options | error verbosity; not applied |
| `hardtabs` option | terminal tab expansion; nvi never sends tabs to terminal |
| `prompt` option | ex prompt display; not yet driving behavior |

---

*Rows marked ✔ are pinned by `internal/conformance` tests that diff govi's engine
against Keith Bostic's nvi binary. Both frontends share that engine, so the mark
applies to govi and Govi.app alike.*
