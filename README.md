<img src="icon.png" alt="GoVi Icon" width="128">

# govi

**GoVi** is an Ex/Vi text editor — a modern Go reimplementation of Keith Bostic's **nvi** preserving
learned muscle memory.  **GoVi**'s development focus has been on **vi**.

This repository builds to different **govi** programs:

| Program | What it is |
|---------|------------|
| **`govi`** | Terminal editor (full-screen, like classic vi) |
| **`GoVi.app`** | Native macOS graphical editor; **`govi -g`** opens files in it from the shell |

Both use the same editor engine. If you know vi, you already know govi.

---

## Requirements

- **Go 1.26 or newer** to build either frontend.
- A **Unix-like system** (Linux, macOS, *BSD) for the terminal editor.
- For the macOS GUI (**GoVi.app**): **macOS** with the **Swift toolchain**
  (`swiftc`) from Xcode or the Command Line Tools.

Build from a checkout of the govi repository.

---

## Quick start

### Terminal

**GoVi** for the terminal only requires the Go toolchain:

```sh
cd cmd/govi        # location of package main
go build           # build govi
./govi file.txt    # edit a file
```

It can also be built from the top level directory with make:

```sh
make govi          # build ./govi
./govi file.txt    # edit a file
```

Inside the editor: **`i`** to insert, **Esc** to return to command mode, **`:w`** to
save, **`:q`** to quit. **`:help`** points you at the built-in command lists.

### macOS GUI

The quickest way to get **GoVi** is a prebuilt release. Download the latest
`GoVi-<version>-macos-universal.dmg` (Intel + Apple Silicon, macOS 11+) from the
[releases page](https://github.com/goexvi-ctrl/govi/releases) and open it. Drag
**GoVi.app** onto the **Applications** shortcut, and copy the bundled **`govi`**
terminal tool onto your `PATH`:

```sh
cp "/Volumes/GoVi "*/govi /usr/local/bin/   # or ~/bin
```

The release is signed with a Developer ID and notarized by Apple, so it opens
normally -- no quarantine workaround needed.

Or build it from source:

```sh
make              # builds govi and gui/build/GoVi.app
./govi -g file    # open in GoVi.app (creates the file if missing)
```

Or double-click **GoVi.app**. Use it like vi: **`i`**, **Esc**, **`:w`**, **`:q`**, **`dd`**,
**`/pattern`**, and the rest.

Install to `~/bin`:

```sh
make install      # installs ~/bin/govi and ~/bin/GoVi.app
```

---

## Command-line options (`govi`)

```
govi [-g [-w]] [-r [file]] [-s] [file ...]
```

| Flag | Meaning |
|------|---------|
| **`-g`** | Open the files in **GoVi.app** (macOS) instead of the terminal |
| **`-w`** | With **`-g`**, block until the tabs/windows for *these* files are closed (useful as an `EDITOR`); requires at least one file |
| **`-r`** | List recoverable files (`govi -r`) or recover a named file (`govi -r file`) |
| **`-s`** | Silent startup: do not read startup files or `EXINIT`/`NEXINIT` |
| **`file ...`** | Files to edit. With multiple files, **`:n`** / **`:prev`** move through the argument list |

With **`-g`**, `govi` hands the files to a running **GoVi.app** (or launches it),
forwarding the working directory and startup environment. Set **`GOVI_APP`** if the
app bundle is not in the default search path (next to the `govi` binary,
`gui/build/GoVi.app` in a checkout, or `/Applications/GoVi.app`).

**`govi -g` with no file** opens a *temporary* buffer (backed by a `vi.XXXXXX`
file in the temp directory, like nvi), which is **deleted when its window/tab
closes**. Because the temporary file is discarded, **`:wq`/`:x`/`ZZ`/`:q` warn
("File is a temporary; exit will discard modifications") instead of quitting** —
save your work with **`:w file`** (which adopts that name), or discard it with
**`:q!`** / **`ZQ`**. (govi does not write the temporary file itself; that would
only matter if you had copied its name for use from another process.)

---

## Modes

| Mode | How you get there | What you do |
|------|-------------------|-------------|
| **Command** | Default; **Esc** from insert | Motions, operators, **`:`** ex line, **`/`** search |
| **Insert** | **`i` `a` `I` `A` `o` `O` `c` `C` `s` `S`** … | Type text; **Esc** ends insert |
| **Replace** | **`R`** | Overtype; **Esc** ends |
| **Colon** | **`:`** in command mode | One ex command on the status line |
| **Ex (line)** | **`Q`** in command mode | Line-at-a-time ex editor; **`:visual`** returns to vi |
| **Ex input** | **`:append`**, **`:insert`**, **`:change`** (or from the colon line) | Type lines; a lone **`.`** on a line ends input |

The bottom line of the screen is the **status/message line**. Errors and file
information appear there.

---

## Getting help inside the editor

| Command | Shows |
|---------|-------|
| **`:help`** | Pointers to the usage commands |
| **`:exusage`** | List of all ex commands |
| **`:exusage cmd`** | Detailed usage for one ex command (e.g. **`:exusage substitute`**) |
| **`:viusage`** | List of vi keys (command and insert mode) |
| **`:viusage key`** | Detailed usage for one key (e.g. **`:viusage d`**) |
| **`:version`** | Editor version string |

These are the authoritative references; the summaries below match them.

---

## Vi command-mode keys

> **New to govi?** The reference tables in this and the next two sections mirror
> the built-in **`:viusage`** and **`:exusage`** help. To just start editing,
> follow [Quick start](#quick-start) above, then skip ahead to
> [Options](#options-set), [Startup files](#startup-files), and
> [GoVi.app](#goviapp-macos-gui) — and come back to these tables as a reference.

### Movement

| Key | Action |
|-----|--------|
| **`h`** **`^H`** | Left |
| **`j`** **`^J`** **`^N`** | Down (logical line) |
| **`k`** **`^K`** **`^P`** | Up |
| **`l`** **space** | Right |
| **`w` `b` `e`** | Word forward / backward / end of word |
| **`W` `B` `E`** | WORD (blank-delimited) motions |
| **`0`** | Start of line |
| **`^`** | First non-blank on line |
| **`$`** | End of line |
| **`\|` *n*** | Column *n* |
| **`G`** *n* | Go to line *n* (default: last line) |
| **`H` `M` `L`** | Top / middle / bottom screen line |
| **`+`** **`^M`** | Next line, first non-blank |
| **`-`** | Previous line, first non-blank |
| **`%`** | Matching bracket |
| **`f` `F` `t` `T`** *char* | Find character forward / backward / before / after |
| **`;` `,`** | Repeat / reverse last **`f` `F` `t` `T`** |
| **`(` `)`** | Sentence backward / forward |
| **`{` `}`** | Paragraph backward / forward |
| **`[[` `]]`** | Section backward / forward |
| **`` ` `` `'`** *mark* | To mark (exact / line) |
| **`_`** | First non-blank of last line |

### Scrolling

| Key | Action |
|-----|--------|
| **`^F`** | Page forward |
| **`^B`** | Page backward |
| **`^D`** | Half page down |
| **`^U`** | Half page up |
| **`^E`** | Scroll down one line |
| **`^Y`** | Scroll up one line |
| **`z`** *type* | Reposition screen (**`z.`** **`z-`** **`z^M`**, etc.) |

### Search

| Key | Action |
|-----|--------|
| **`/`** *pat* | Search forward |
| **`?`** *pat* | Search backward |
| **`n`** | Repeat search |
| **`N`** | Repeat search, opposite direction |
| **`^A`** | Search for word under cursor |
| **`&`** | Repeat last substitute |

### Editing operators

| Key | Action |
|-----|--------|
| **`d`** *motion* | Delete |
| **`c`** *motion* | Change (delete, then insert) |
| **`y`** *motion* | Yank |
| **`>` **`<`** *motion* | Shift lines right / left |
| **`~`** *motion* | Toggle case |
| **`!`** *motion* *cmd* | Filter through shell |
| **`dd` `cc` `yy`** | Line delete / change / yank |
| **`D` `C` `Y`** | Delete / change / yank to end of line |
| **`x` `X`** | Delete char under / before cursor |
| **`s` `S`** | Substitute character(s) / whole line |
| **`r`** *char* | Replace character |
| **`R`** | Enter replace mode |
| **`J`** | Join lines |
| **`p` `P`** | Put after / before cursor |
| **`u`** | Undo |
| **`U`** | Undo all changes on current line |
| **`.`** | Repeat last change |
| **`@`** *reg* | Execute register as vi commands |

### Insert entry

| Key | Action |
|-----|--------|
| **`i` `a`** | Insert before / after cursor |
| **`I` `A`** | Insert at first non-blank / end of line |
| **`o` `O`** | Open line below / above |

### Other

| Key | Action |
|-----|--------|
| **`:`** | Ex command line |
| **`m`** *a-z* | Set mark |
| **`#`** **`#+`** **`#-`** | Increment / decrement number at cursor |
| **`^G`** | File information |
| **`^^`** | Alternate file |
| **`^]`** **`^T`** | Tag push / pop (ctags) |
| **`Q`** | Ex (line) mode |
| **`ZZ`** | Write if modified and quit |
| **`ZQ`** | Quit without writing |
| **`^L` `^R`** | Repaint (terminal; GUI repaints automatically) |
| **`^Z`** | Suspend editor (Unix terminal only) |

### Insert-mode keys

| Key | Action |
|-----|--------|
| **Esc** | End insert |
| **`^H`** erase | Delete previous character |
| **`^W`** | Delete previous word |
| **`^U`** | Erase input line (nvi); not implemented in govi insert mode |
| **`^V`** | Quote next character |
| **`^D` `^T`** | Shift left / right (with autoindent) |
| **`^X`** *hex* | Insert a Unicode code point — up to 6 hex digits (2/4/6 for a byte / BMP / astral); ends at 6 digits or a non-hex key |
| **`^@`** (NUL) | Replay previous insertion |
| **`^Z`** | Suspend (leaves insert mode; terminal only) |

With **`autoindent`** (**`:set ai`**), new lines inherit the current line's
leading whitespace.

---

## Ex commands

Ex commands are entered on the **`:`** line or in **ex (Q)** mode. Many accept a
**line address** before the command:

| Address | Meaning |
|---------|---------|
| **`.`** | Current line |
| **`$`** | Last line |
| **`%`** | Entire file |
| **`'a`** | Line with mark *a* |
| **`/pat/`** | Next line matching *pat* |
| **`10`** | Line 10 |
| **`10,20`** | Lines 10 through 20 |
| **`.,$`** | Current line through end |

On the colon line, press **Tab** (the **`filec`** character) to complete the file
name before the cursor — for commands like **`:edit`**, **`:write`**, and
**`:read`**. A unique match fills in (a directory gains a trailing **`/`**); an
ambiguous prefix rings the bell so you can type more.

### Buffer and file

| Command | Summary |
|---------|---------|
| **`:write`** **`[:range] w[rite][!] [file]`** | Write buffer (or range) to file |
| **`:write !cmd`** **`[:range] w[rite] !cmd`** | Pipe the lines (default whole file) to a shell command's input; buffer unchanged |
| **`:wq`** | Write and quit |
| **`:xit`** | Write if modified and quit |
| **`:quit`** **`[:range] q[uit][!]`** | Quit |
| **`:read`** **`:[line] r[ead] file`** | Read file after line |
| **`:read !cmd`** **`:[line] r[ead] !cmd`** | Insert a shell command's output after the line |
| **`:edit`** **`:e[dit][!] [file]`** | Edit a file |
| **`:file`** **`:f[ile] [name]`** | Show or set the buffer name |
| **`:args`** | Display the argument list |
| **`:next`** **`:n[ext][!] [file]`** | Edit next file in args |
| **`:previous`** **`:prev[ious][!]`** | Edit previous file |
| **`:Next`** | Same as **`:previous`** |
| **`:rewind`** **`:rew[ind][!]`** | Edit first file in args |

### Line editing

| Command | Summary |
|---------|---------|
| **`:delete`** **`[:range] d[elete] [buffer]`** | Delete lines |
| **`:yank`** **`[:range] y[ank] [buffer]`** | Yank lines to register |
| **`:put`** **`:[line] pu[t] [buffer]`** | Put buffer after line |
| **`:copy`** **`[:range] co[py] [buffer] address`** | Copy lines |
| **`:t`** **`[:range] t address`** | Copy lines (synonym) |
| **`:move`** **`[:range] m[ove] address`** | Move lines |
| **`:join`** **`[:range] j[oin]`** | Join lines |
| **`:<`** **`[:range] < [count]`** | Shift left |
| **`:>`** **`[:range] > [count]`** | Shift right |
| **`:substitute`** **`[:range] s/pat/repl/[flags]`** | Substitute (see below) |
| **`:&`** | Repeat last substitute |
| **`:global`** **`[:range] g/pat/ cmds`** | Execute on matching lines |
| **`:vglobal`** **`[:range] v/pat/ cmds`** | Execute on non-matching lines |
| **`:append`** **`:[line] a[ppend]`** | Append text after line |
| **`:insert`** **`:[line] i[nsert]`** | Insert text before line |
| **`:change`** **`[:range] c[hange]`** | Change lines |

### Display

| Command | Summary |
|---------|---------|
| **`:print`** **`[:range] p[rint]`** | Print lines |
| **`:number`** **`[:range] n[umber]`** | Print with line numbers |
| **`:list`** **`[:range] l[ist]`** | Print with visible characters |
| **`:=`** **`:[line]=`** | Display line number |

### Options, maps, abbreviations

| Command | Summary |
|---------|---------|
| **`:set`** **`[:se[t] [option[=value]] ...]`** | Show or set options |
| **`:map`** **`:map[!] lhs rhs`** | Map keys in command mode |
| **`:unmap`** **`:unm[ap][!] lhs`** | Unmap keys |
| **`:abbreviate`** **`:ab[breviate] lhs rhs`** | Define abbreviation |
| **`:unabbreviate`** **`:una[bbreviate] lhs`** | Remove abbreviation |
| **`:source`** **`:so[urce] file`** | Read and execute ex commands from a file |

### Shell, tags, recovery

| Command | Summary |
|---------|---------|
| **`:[range] !cmd`** **`:!cmd`** | Filter lines through shell / run shell command |
| **`:shell`** **`:sh[ell]`** | Run an interactive shell |
| **`:tag`** **`:ta[g] tagname`** | Jump to ctags tag |
| **`:preserve`** **`:pre[serve]`** | Flush recovery file |
| **`:recover`** **`:rec[over] [file]`** | Recover from recovery directory |
| **`:suspend`** **`:su[spend][!]`** | Suspend session (terminal) |
| **`:stop`** **`:st[op][!]`** | Same as **`:suspend`** |

### Misc

| Command | Summary |
|---------|---------|
| **`:visual`** **`:vi[sual]`** | Return to vi mode from ex mode |
| **`:help`** **`:he[lp]`** | Help pointers |
| **`:exusage`** **`:exu[sage] [command]`** | Ex command usage |
| **`:viusage`** **`:viu[sage] [key]`** | Vi key usage |
| **`:version`** **`:ve[rsion]`** | Version |

### Substitute

```
:[range] s/pattern/replacement/[flags]
```

| Flag | Meaning |
|------|---------|
| **`g`** | Global — all matches on each line (default: first only) |
| **`c`** | Confirm each replacement (when supported) |

Replacement text supports **`&`** (matched text), **`~`** (previous replacement),
and **`\1`–`\9`** (parenthesized submatches). Case toggles **`\u` `\l` `\U` `\L` `\E`**
and **`\n`** (newline) work as in nvi.

---

## Options (`:set`)

Type **`:set`** to see options that differ from defaults. **`:set all`** lists
every option. Boolean options: **`:set option`** / **`:set nooption`**. Query:
**`:set option?`**. Unique prefixes work (e.g. **`:set tabs=4`** sets **`tabstop`**).

### Options that affect editing

| Option | Abbr | Default | Effect |
|--------|------|---------|--------|
| **autoindent** | ai | off | Carry leading whitespace on new lines |
| **ignorecase** | ic | off | Case-insensitive search/substitute |
| **magic** | | on | Regex metacharacters in patterns |
| **wrapscan** | ws | on | Searches wrap around the file |
| **tabstop** | ts | 8 | Tab width (display and indent) |
| **shiftwidth** | sw | 8 | Indent step for **`<` `>`** and **`^T`/`^D`** |
| **tildeop** | to | off | **`~`** takes a motion |
| **number** | nu | off | Line number gutter |
| **list** | | off | Show tabs as **`^I`**, ends with **`$`**, controls visible |
| **showmatch** | sm | off | Briefly flash matching bracket (see **matchtime**) |
| **matchtime** | | 7 | Showmatch flash duration (tenths of a second) |
| **ruler** | | off | Line,column ruler on the status line (when no message) |
| **showmode** | smd | off | Mode indicator on the status line (**Command**, **Insert**, …); **\*** when modified |
| **tags** | | tags | ctags file path |
| **shell** | sh | `$SHELL` | Shell for **`!`** and **`:shell`** (from `$SHELL`; **`/bin/sh`** if unset) |
| **filec** | | tab | File-name completion trigger on the colon line; empty disables |
| **readonly** | ro | off | Treat buffer as read-only |
| **exrc** | | off | Read **`.exrc`** in the current directory at startup |
| **recdir** | | /var/tmp/vi.recover | Recovery file directory |
| **scroll** | scr | — | Lines scrolled by **`^D`/`^U`** |
| **sections** | sect | NHSHH… | Section boundaries for **`[[` `]]`** |
| **paragraphs** | para | IPLPPPQPP… | Paragraph boundaries for **`{` `}`** |

### GoVi.app display options

These options are stored per tab in the engine. In **GoVi.app** they change the
text and background colors. In the terminal **`govi`** they are settable but do
not change the display.

| Option | Abbr | Values |
|--------|------|--------|
| **foreground** | fg | `#RGB`, `#RRGGBB`, or a color name (e.g. `wheat`, `cornflowerblue`); empty = system default |
| **background** | bg | Same as **foreground** |

Example: **`:set background=wheat foreground=#001122`**

All other nvi options (74 total) are recognized and appear in **`:set all`**. Many
are inert in govi — they exist for compatibility but do not change behavior. See
[`docs/parity.md`](docs/parity.md) for the full parity matrix.

---

## Startup files

Unless you pass **`-s`** to **`govi`**, startup ex commands are read **before** the
file to edit is opened, in this order:

1. **`/etc/vi.exrc`** (must be owned by root or you)
2. **`NEXINIT`** or **`EXINIT`** environment variable (if set, home startup is skipped)
3. **`$HOME/.nexrc`**, or **`$HOME/.exrc`** if `.nexrc` is missing
4. **`./.nexrc`** or **`./.exrc`** in the current directory — only if **`:set exrc`**
   is in effect (from an earlier startup file or default)

Startup files contain **ex** commands, not vi keystrokes. Unsafe startup files
(group-writable or not owned by you) are rejected.

**GoVi.app** follows the same rules when a tab is created. Application **Settings**
defaults for foreground/background are applied first; startup files and **`:set`**
can override them per tab.

Example **`~/.nexrc`**:

```
set autoindent
set shiftwidth=4
set tabstop=4
set background=wheat
```

---

## Registers and marks

| Registers | Use |
|-----------|-----|
| **`a`–`z`** | Named yank/delete buffers |
| **`A`–`Z`** | Append to named buffer |
| **`1`–`9`** | Numbered delete buffers (shift on each delete) |
| **`"`** | Unnamed (last yank/delete) |
| **`.`** | Last change (for **`.`** repeat) |

Marks **`a`–`z`** are set with **`ma`** and used with **`` `a ``** or **`'a`**.

---

## Crash recovery

While a file has unsaved changes, govi maintains a recovery file in the directory
named by the **`recdir`** option (default **`/var/tmp/vi.recover`**).

| Action | Command |
|--------|---------|
| List recoverable sessions | **`govi -r`** |
| Recover a file | **`govi -r filename`** |
| Force recovery sync | **`:preserve`** |
| Recover from inside the editor | **`:recover [file]`** |

Recovery files use govi's own format (not binary nvi recovery). The GUI flushes
recovery data automatically after a short idle period.

---

## GoVi.app (macOS GUI)

### Windows and tabs

Each window and tab is an independent editor session.

| Action | Shortcut / control |
|--------|-------------------|
| New window | **Cmd-N** |
| New tab | **Cmd-T** or tab bar **+** |
| Open files | **Cmd-O** (placement follows Settings) |
| Close tab/window | **Cmd-W** or **`:q`** |
| Quit app | **Cmd-Q** (when last window closes) |

Native macOS tabbing works: drag tabs between windows, merge windows, move a tab
to a new window from the **Window** menu.

### Mouse and clipboard

| Action | How |
|--------|-----|
| Move cursor | Click |
| Select text | Click-drag |
| Select word / line | Double-click / triple-click |
| Scroll | Wheel or two-finger scroll (viewport moves; cursor stays until next edit) |
| Copy / cut / paste | **Cmd-C / Cmd-X / Cmd-V** or Edit menu |
| Select all | **Cmd-A** |
| Replace selection | Type or paste while text is selected |
| Context menu | Right-click or control-click (spelling, Look Up, cut/copy/paste) |

### Spell checking

Misspelled words on visible lines are underlined in red (macOS **NSSpellChecker**).

- Toggle: **Edit → Spelling → Check Spelling While Typing** (persisted)
- Right-click a word for suggestions, Ignore/Learn Spelling, and Look Up

### International input

Option accents, dead keys, and IME composition work through the macOS text-input
system. Control keys (**`^F`**, **`^D`**, etc.) remain vi commands.

### Settings (Cmd-,)

| Setting | Effect |
|---------|--------|
| **Text padding** | Pixel inset between window edge and text (all windows) |
| **Default rows / columns** | Initial window size for new editors |
| **Font / font size** | Monospaced display font (all windows) |
| **Default foreground / background** | Colors for **new tabs** only |
| **Open files in** | New window vs tab of front window |
| **Show rows×columns in title bar** | Live grid size in the window subtitle |
| **Warn before closing unsaved files** | Save/discard prompt on close |

Per-tab colors after a tab is open: **`:set foreground=...`** and
**`:set background=...`**.

### Ex (Q) mode in the GUI

Press **`Q`** for line-oriented ex mode. The window becomes a scrolling
transcript; **`:visual`** returns to the normal editor view.

---

## Building and testing

```sh
make              # govi + GoVi.app
make test         # go test ./...
make clean        # remove build artifacts
```

Developer-oriented notes about the architecture and embedding boundary are in
[`NOTES.md`](NOTES.md). Feature parity with BSD nvi is tracked in
[`docs/parity.md`](docs/parity.md). The full nvi manual is in [`docs/nvi.md`](docs/nvi.md).

---

## Limitations (summary)

govi aims for nvi-compatible editing, not a byte-for-byte clone of every nvi
feature. Notable gaps:

- **No split screens** — no **`^W`**, **`:bg`**, **`:fg`**, **`:resize`**
- **UTF-8** text only
- **No cscope** integration
- **Suspend** (**`^Z`**, **`:suspend`**) — Unix terminal only; not in GoVi.app
- Many legacy options are **settable but inert** (see parity doc)
- **`foreground`** / **`background`** colors — **GoVi.app** only

For day-to-day editing, the vi/ex command set above is fully usable in both
frontends.

---

## Acknowledgements

**govi** is inspired by BSD **nvi** — the 4.4BSD **ex**/**vi** implementation and
reference manual by **Keith Bostic** (University of California, Berkeley). The
editor semantics, command set, and much of the documentation lineage trace to
that work and to **The Regents of the University of California**.

As in [`docs/nvi.md`](docs/nvi.md), credit also belongs to the people who built
the historic **ex**/**vi** line and **nvi** itself, including **Bruce Englar**,
**Peter Kessler**, **Bill Joy**, **Mark Horton**, **Steve Kirkendall**,
**Henry Spencer**, **Ken Arnold**, **Elan Amir**, **George Neville-Neil**,
**Sven Verdoolaege**, and **Rob Mayoff**.

The **govi** program has been written mainly by
[Claude Code](https://claude.com/product/claude-code) and
[Grok Build](https://x.ai).
