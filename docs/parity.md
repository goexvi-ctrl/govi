# govi vs nvi — feature parity

Tracks what the real **nvi** (1.81.x / the 4.4BSD nex/vi reference) provides
against **govi**, this Go reimplementation. The goal is user-perceptible
bug-for-bug parity; rows are validated against the nvi oracle where marked.

The authoritative behavior spec is the official manual in [`nvi.md`](nvi.md);
[`nvi-index.md`](nvi-index.md) maps every command/option to its line there, so a
parity row can be checked against the source description quickly.

**Status legend**

| Status | Meaning |
|--------|---------|
| ✅ Done | Implemented; matches nvi (✔ = also covered by an oracle conformance test) |
| 🟡 Partial | Implemented with known gaps or simplifications |
| ⚙️ Inert | Recognized/settable but does not yet drive behavior |
| ❌ Not yet | Not implemented |
| — N/A | Out of scope for this port |

---

## Vi command-mode commands

| Command | Description | nvi | govi | Notes |
|---|---|---|---|---|
| `^A` | search forward for word under cursor | yes | ✅ | |
| `^B` `^F` | page up / down | yes | ✅ | |
| `^D` `^U` | scroll down / up half-screen | yes | ✅ | maintains column |
| `^E` `^Y` | scroll one line, cursor fixed | yes | 🟡 | scrolls; cursor-follow simplified |
| `^G` | file information | yes | ✅ | |
| `^H` `h` / `l` `space` | left / right | yes | ✅✔ | |
| `^J` `^N` `j` / `^P` `k` | down / up | yes | ✅✔ | logical-line; maintains display column |
| `^L` `^R` | repaint screen | yes | 🟡 | no-op (frontend repaints every input) |
| `^M` `+` / `-` | next / prev line, first non-blank | yes | ✅ | |
| `^T` `^]` | tag pop / tag push | yes | ✅✔ | ctags `tags` file |
| `^W` | switch screens | yes | ❌ | no split screens |
| `^Z` | suspend | yes | ❌ | |
| `^^` | alternate file | yes | ✅✔ | |
| `:` | ex command line | yes | ✅✔ | |
| `/` `?` `n` `N` | search / repeat | yes | ✅✔ | wrapscan honored |
| `!` | filter through shell | yes | ✅✔ | `!motion` + `:range!cmd` |
| `#` | increment/decrement number | yes | ✅✔ | `#+` `#-` |
| `$` `0` `^` `_` `\|` | line column motions | yes | ✅✔ | `$` sticky to EOL |
| `%` | match bracket | yes | ✅✔ | nests across lines |
| `&` | repeat last substitute | yes | ✅✔ | |
| `` ` `` `'` | marks (exact / line) | yes | ✅✔ | |
| `(` `)` `{` `}` `[[` `]]` | sentence / paragraph / section | yes | ✅✔ | exclusive→linewise operator promotion |
| `;` `,` | repeat / reverse `f F t T` | yes | ✅ | |
| `.` | repeat last change | yes | ✅✔ | with count override |
| `<` `>` | shift left / right | yes | ✅✔ | tab-aware indent |
| `@` | execute register as commands | yes | ✅✔ | |
| `~` | reverse case (count or, with `tildeop`, motion) | yes | ✅✔ | |
| `a A i I o O` | enter insert | yes | ✅✔ | |
| `b B w W e E` | word / WORD motions | yes | ✅✔ | |
| `c C d D s S y Y` | change/delete/subst/yank | yes | ✅✔ | `cw`→`ce` special case |
| `f F t T` | find char in line | yes | ✅✔ | |
| `G H M L` | goto line / screen positions | yes | ✅✔ | |
| `J` | join lines | yes | ✅✔ | |
| `m` | set mark | yes | ✅ | |
| `p P` | put | yes | ✅✔ | char/line-wise, count |
| `Q` | switch to ex mode | yes | 🟡 | basic ex transcript; `:visual` returns |
| `r R` | replace char / replace mode | yes | ✅✔ | |
| `U` | restore line | yes | ✅✔ | undoes the run of changes on the current line |
| `u` | undo (toggle) | yes | ✅✔ | directional `u`/`.` model |
| `x X` | delete char | yes | ✅✔ | |
| `z` | screen positioning (`z↵` `z.` `z-`) | yes | 🟡 | no `z^`/`z+`/count2 |
| `ZZ` `ZQ` | save-quit / quit | yes | ✅ | |

## Vi text-input-mode commands

| Command | Description | nvi | govi | Notes |
|---|---|---|---|---|
| `NUL` (`^@`) | replay previous input | yes | ✅ | |
| `^D` / `^T` | autoindent erase / shift to shiftwidth | yes | 🟡 | implemented as line shift left/right |
| `0^D` `^^D` | erase all / reset autoindent | yes | ❌ | |
| `^H` / erase | erase last character | yes | ✅ | |
| `^V` | quote next character | yes | ✅ | |
| `^W` | erase last word | yes | ✅✔ | |
| `^X` | insert hex character code | yes | ✅ | |
| line erase | erase the input line | yes | ❌ | |
| `<esc>` | end input | yes | ✅✔ | |
| autoindent | leading-whitespace carry | yes | ✅✔ | `o`/`O` and `↵` |
| abbreviations | expand on word break | yes | ✅✔ | |

## Ex commands

| Command | nvi | govi | Notes |
|---|---|---|---|
| `:[range]d[elete]` | yes | ✅✔ | buffer + count |
| `:[range]m[ove]` / `:[range]co`/`t` | yes | ✅✔ | |
| `:[range]y[ank]` / `:[line]pu[t]` | yes | ✅✔ | |
| `:[range]j[oin]` | yes | ✅✔ | |
| `:[range]<` `:[range]>` | yes | ✅✔ | |
| `:[range]s[ubstitute]` | yes | ✅✔ | `g` flag, `&`, `\1`-`\9`, `\u\l\U\L\E`, `\n` |
| `:[range]g[lobal]` / `:v` | yes | ✅✔ | |
| `:[line]=` | yes | ✅ | |
| `:[range]p[rint]`/`nu[mber]`/`l[ist]` | yes | ✅ | output via overlay/transcript |
| `:w[rite]` `:wq` `:x[it]` `:q[uit]` | yes | ✅✔ | `!`, modified guard |
| `:r[ead] file` | yes | ✅ | `:r !cmd` ❌ |
| `:[range]!cmd` / `:!cmd` | yes | ✅✔ | |
| `:set` / `:set all` / `:set opt` | yes | ✅✔ | full option registry, grid display |
| `:map` `:map!` `:unmap` | yes | ✅✔ | non-recursive |
| `:ab[breviate]` `:una[bbreviate]` | yes | ✅✔ | |
| `:[range]nu[mber]`/`:#` | yes | ✅ | number-prefixed print |
| `:e[dit]` `:n[ext]` `:prev`/`:N` `:rew[ind]` `:ar[gs]` | yes | ✅✔ | argument list |
| `:f[ile] [name]` | yes | ❌ | show/set current pathname |
| `:ta[g]` | yes | ✅✔ | |
| `:tagn[ext]` `:tagp[rev]` `:tagt[op]` | yes | ❌ | tag-stack walk (vi `^T`/`^]` work) |
| `:vi[sual]` | yes | ✅ | returns from ex mode |
| `:[range]a[ppend]`/`i[nsert]`/`c[hange]` | yes | ❌ | ex input mode |
| `:k`/`:ma` (mark) | yes | ❌ | (vi `m` works) |
| `:u[ndo]` | yes | ❌ | (vi `u` works) |
| `:so[urce]` `:mk[exrc]` | yes | ❌ | exrc scripting |
| `:cd`/`:chdir` | yes | ❌ | |
| `:di[splay] b\|c\|s\|t` | yes | ❌ | buffers/screens/tags inspector |
| `:he[lp]` | yes | ❌ | |
| `:o[pen]` | yes | — | unimplemented in nvi itself |
| `:bg` `:fg` `:res[ize]` | yes | ❌ | needs split screens |
| `:su[spend]`/`:st[op]` | yes | ❌ | job control |
| `:cs[cope]` | yes | — | cscope integration; out of scope |
| `:pre[serve]` `:rec[over]` | yes | ✅ | crash recovery (govi format) |
| `:ve[rsion]` `:viu[sage]` `:exu[sage]` | yes | ❌ | |
| `:@`/`:*` (exec buffer) `:w>>` `:wn` etc. | yes | 🟡 | partial |

## Options

govi carries the full nvi option table (74 options, matching the reference
manual's Set Options section) — all are settable, queryable, and shown by
`:set all`. The ones that drive behavior:

| Option | nvi | govi | Notes |
|---|---|---|---|
| `autoindent` (ai) | yes | ✅✔ | |
| `ignorecase` (ic) | yes | ✅✔ | search/substitute |
| `magic` | yes | ✅ | regex syntax |
| `wrapscan` (ws) | yes | ✅✔ | search wrap |
| `tabstop` (ts) | yes | ✅✔ | display + indent |
| `shiftwidth` (sw) | yes | ✅✔ | `<` `>` and `^T`/`^D` |
| `tildeop` (to) | yes | ✅✔ | |
| `tags` | yes | ✅✔ | |
| `number` (nu) | yes | ✅ | gutter rendered |
| `list` | yes | 🟡 | settable; display not yet |
| `showmatch` (sm) | yes | ✅✔ | bracket flash on insert (matchtime) |
| `columns`/`lines` | yes | ✅ | track terminal geometry |
| `shell` | yes | ✅ | used by `!` filter |
| all other options | yes | ⚙️ | recognized/settable, inert |

## Subsystems

| Subsystem | nvi | govi | Notes |
|---|---|---|---|
| Large-file editing | recno DB paging | ✅ | paged piece-table line store; multi-GB |
| Undo / redo | yes | ✅✔ | multi-level; nvi directional `u`/`.` |
| Marks | yes | 🟡 | line-granular fixups; intra-line column fixup partial |
| Registers / cut buffers | yes | ✅✔ | named a-z (A-Z append), numbered 1-9 |
| Regex engine | BRE + extensions | ✅✔ | backrefs, `\<`/`\>`, intervals, classes, alternation |
| Search | yes | ✅✔ | line-oriented, wrapscan |
| Maps / abbreviations | yes | 🟡 | non-recursive (noremap) |
| Multiple files (arg list) | yes | ✅✔ | |
| Tags | yes | ✅✔ | ctags file; tag stack |
| Wide / multibyte display | wchar | ✅✔ | East Asian width = 2 cols |
| Line wrapping | yes | 🟡 | wraps; no `@`-fill for partial bottom line |
| Left-right scrolling (`leftright`/`sidescroll`) | yes | ❌ | always wraps; no horizontal-scroll mode |
| Cursor column maintenance | yes | ✅✔ | display column, sticky `$` |
| Word search (`^A`) | yes | ✅✔ | search word under cursor |
| Incrementing numbers (`#`) | yes | ✅✔ | `#+`/`#-` |
| File-name completion | yes | ❌ | no path completion on the `:` line |
| Command-line editing (`cedit`) | yes | ❌ | no ex command-line edit window |
| Embeddable engine boundary | function-pointer table | ✅ | Go `Frontend`/`View`; tcell + headless frontends |
| Crash recovery (`-r`) | yes | ✅ | recovery file in recdir; `:preserve`/`:recover`/`nvi -r`; govi's own format |
| Split screens / windows (`^W` `:bg`/`:fg`/`:resize`) | yes | ❌ | |
| Job control (`^Z` `:suspend`/`:stop`) | yes | ❌ | |
| Cscope integration | yes | — | out of scope |
| Message catalogs (i18n) | yes | — | English only; out of scope |
| File encodings | iconv | 🟡 | UTF-8 only |
| Perl / Tcl scripting | yes | — | out of scope |
| GUI backends (motif/gtk/ipc) | yes | — | boundary ready; no GUI frontend yet |

---

*Rows marked ✔ are pinned by `internal/conformance` tests that diff govi
against the real nvi binary.*
