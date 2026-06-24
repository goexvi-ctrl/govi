# Govi.app — the editor engine embedded in a native macOS application

This is the **embeddability proof** for govi: a native macOS (AppKit) application
with the govi editor engine running **in-process**. nvi is *embedded*, not
exec'd — there is no terminal and no child process. The same `engine` package
that backs the terminal frontend drives a Cocoa window here, demonstrating that
the engine has zero terminal/GUI coupling across its `Frontend`/`View` boundary.

This is the foundation for editor features that go beyond a terminal can offer
(spell correction, etc.), which will be layered on top.

## Architecture

```
  ┌──────────────────────────────────────────────┐
  │  Govi.app  (Swift / AppKit)                    │
  │                                                │
  │  GoviView : NSView                             │
  │    keyDown ─────────► GoviKeyRune/KeySpecial   │   C ABI
  │    draw(_:) ◄──────── GoviRowText/Cursor*      │ (libgovi.h)
  └───────────────┬────────────────────────────────┘
                  │  cgo c-archive (libgovi.a)
  ┌───────────────▼────────────────────────────────┐
  │  gui/bridge  (package main, //export)           │
  │    GoviStart/Resize/Key.../Compose/...          │
  │    host implements engine.Frontend              │
  └───────────────┬────────────────────────────────┘
                  │
  ┌───────────────▼────────────────────────────────┐
  │  engine  (no terminal/GUI deps)                 │
  │    Engine.Input(Event) ── drives ── View        │
  │  frontend/grid.Compose(View) ► character grid   │
  └─────────────────────────────────────────────────┘
```

- **`engine`** is untouched: the GUI consumes the exact same `Frontend`/`View`
  contract as the tcell terminal frontend.
- **`frontend/grid`** lays the semantic `View` out into a flat character grid
  (wrapping, gutter, status line, cursor, showmatch) — all the vi presentation
  logic, shared and unit-tested in pure Go. The Swift side is a "dumb terminal"
  that just paints the grid and forwards keys.
- **`gui/bridge`** compiles the engine into a C archive and exposes a small C
  API (see `build/libgovi.h` after a build). The host pulls a fresh grid after
  each input via `GoviCompose` + `GoviRowText`.
- **`gui/macos`** is the Swift/AppKit app: a window, a `GoviView` that renders
  the grid in a monospaced font and translates `NSEvent` key presses into engine
  events, and timers for map/showmatch/recovery just like the terminal `Run`
  loop.

## Build

Requires a Go toolchain and the Xcode command-line tools (`swiftc`, `clang`):

```sh
./gui/build.sh
```

This produces `gui/build/Govi.app`.

## Run

Use the `govi` binary's GUI mode (`govi -g`):

```sh
govi -g file1 file2 ...   # opens the files as tabs in one window
govi -g                   # just launch / focus Govi.app
govi -g -w file           # block until file's tab/window is closed
```

Multiple files passed together open as **tabs in a single window**. A file that
is already open is focused rather than duplicated.

`govi -g` launches Govi.app if needed, or hands the files to an **already-running**
instance via the macOS open-documents event (no custom IPC), and brings it to
the front — so it works like a normal command-line editor launcher. Re-opening a
file that's already open just focuses its window. It finds Govi.app via
`$GOVI_APP`, then next to the `govi` binary, then `gui/build/Govi.app` in a
checkout, then `/Applications/Govi.app`.

Or run the app directly:

```sh
open gui/build/Govi.app --args /path/to/file   # only on a fresh launch
gui/build/Govi.app/Contents/MacOS/Govi /path/to/file
```

Use it like vi: `i` to insert, `Esc`, `:w`, `:q`, `dd`, `/pattern`, etc. All
editing is handled by the embedded engine.

### Ex (Q) mode

Pressing `Q` enters line-oriented ex mode. A terminal leaves the alternate
screen for a scrolling line interface; the GUI equivalent is the window turning
into a scrolling transcript that grows downward — the `:` prompt and the line
you are typing sit just below the previous output (not pinned to the bottom),
and older lines scroll off the top as it fills. `:visual` returns to the editor.

### Windows

The app is multi-window and multi-tab — every window *and* every tab has its own
embedded engine instance:

- **Cmd-N** opens a new empty window.
- **Cmd-T** (or the tab bar's "+") opens a new empty tab in the current window.
- **Cmd-O** opens one or more files (each in its own window).
- **Cmd-W** (or `:q`) closes the current tab/window; the app quits when the last
  one closes.

Tabbing uses native macOS window tabbing, so dragging tabs between windows,
merging windows, and "Move Tab to New Window" (in the Window menu) all work with
no extra code: each tab is a separate `NSWindow` sharing a tabbing identifier.

This is enabled by the handle-based libgovi API: `GoviStart` returns a handle for
one editor and every call takes it, so windows and tabs are fully independent.

### Spell checking

Standard macOS spell checking (via `NSSpellChecker`):

- Misspelled words on the visible lines get a **red squiggly underline**
  (continuous checking).
- **Right-click / control-click** a word brings up a context menu: spelling
  suggestions plus **Ignore/Learn Spelling** when the word is misspelled, a
  dictionary **Look Up "word"** (the system Dictionary popover), and
  **Cut/Copy/Paste**. The clicked word is selected so those commands act on it.
- **Edit ▸ Spelling ▸ Check Spelling While Typing** toggles it (persisted).

The engine supplies line text and a buffer-position→screen-cell mapping; the
view runs `NSSpellChecker`, draws the underlines, and applies corrections
through the same caret-range primitives as the rest of the GUI. Results are
cached per line text so unchanged lines are not re-checked.

### Settings

**Cmd-,** opens a Settings window. Currently it sets the **text padding** — the
inset in pixels between the window edge and the text (default 3) — persisted in
`UserDefaults` and applied live to all open windows. Padding is a pure rendering
concern in the view layer (it offsets every cell↔pixel conversion); the engine
is unaware of it.

### Mouse and clipboard

In addition to vi keys, the window supports the usual GUI text affordances:

- **Wheel / two-finger scroll** moves the viewport like any windowed app; the
  cursor stays put (and may scroll off-screen) until the next edit or motion.
- **Click** to move the cursor.
- **Click-drag** to highlight a selection.
- **Double-click** selects the word under the cursor; **triple-click** selects
  the whole line. What counts as a "word" is decided by a pluggable function in
  the engine (`SetWordBoundary`); the default groups identifier runes, treats
  punctuation runs as words, and selects whitespace runs. A future language-aware
  rule (e.g. treating `-` as a word character, or tokenizing a specific language)
  drops in there without touching the rest of the editor.
- **Cmd-C / Cmd-X / Cmd-V** (and the Edit menu) copy / cut / paste via the macOS
  system pasteboard; **Cmd-A** selects the whole buffer.
- With a selection active, **typing any character or pasting replaces it** (and
  leaves you in insert mode), and **Backspace/Delete removes it** — standard GUI
  behavior. Vi command keys apply only when nothing is selected.

### International text input

Typed text goes through the macOS text-input system (`NSTextInputClient` /
`interpretKeyEvents`), so Option-accented characters and dead keys compose
normally: **Option-o** produces `o`-slash, and **Option-u** then **u** produces
`u`-umlaut (the `¨` shows underlined at the cursor as marked text until the next
key composes it). IMEs work the same way. Control keys (`^F`, `^D`, `^R`, …) are
handled directly so they stay vi commands rather than triggering Cocoa's
built-in Emacs-style key bindings.

These are GUI-layer features (vi has no selection concept). They are driven by a
few engine primitives in `engine/gui.go` (`MoveCursorTo`, `RangeText`,
`DeleteRange`, `ReplaceSelection*`, `InsertText`) and the cell↔caret mapping in
`frontend/grid` (`Locate`, selection highlighting), all unit-tested in pure Go.
The engine core remains free of any selection or clipboard concept.

## Tested

The engine→grid path the app relies on is covered in pure Go by
`frontend/grid` (composer unit tests and an end-to-end
`TestEngineThroughGrid` that drives a real engine with keystrokes and checks the
composed screen). The Swift layer is a thin renderer over that tested path.
