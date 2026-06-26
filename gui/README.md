# GoVi.app — the editor engine embedded in a native macOS application

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
  │  GoVi.app  (Swift / AppKit)                    │
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

This produces `gui/build/GoVi.app`.

## Run

Use the `govi` binary's GUI mode (`govi -g`):

```sh
govi -g file1 file2 ...   # opens the files as tabs in one window
govi -g                   # just launch / focus GoVi.app
govi -g -w file           # block until file's tab/window is closed
```

Multiple files passed together open as **tabs in a single window**. A file that
is already open is focused rather than duplicated.

`govi -g` launches GoVi.app if needed, or hands the files to an **already-running**
instance via the macOS open-documents event (no custom IPC), and brings it to
the front — so it works like a normal command-line editor launcher. Re-opening a
file that's already open just focuses its window. It finds GoVi.app via
`$GOVI_APP`, then next to the `govi` binary, then `gui/build/GoVi.app` in a
checkout, then `/Applications/GoVi.app`.

Or run the app directly:

```sh
open gui/build/GoVi.app --args /path/to/file   # only on a fresh launch
gui/build/GoVi.app/Contents/MacOS/GoVi /path/to/file
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
- **Click-drag** selects a **linear** range in the buffer (character at a time,
  following the file through line wraps), like a terminal. In ex (Q) mode or
  during `:!cmd` output, drag selects in reading order through the transcript.
- **Option-click-drag** selects an **axis-aligned rectangle** of screen cells —
  useful for copying from the status line, gutter, `~` filler, or any block of
  on-screen text.
- **Double-click** selects the word under the pointer; **triple-click** selects
  the whole screen row. In the buffer, word boundaries match the engine's default
  (`DefaultWordBoundary`); on other rows (status, overlay, ex) the same rules are
  applied to the displayed line text.
- **Cmd-C** (and Edit ▸ Copy) copies the screen selection via the system
  pasteboard, regardless of where the text came from.
- **Cmd-X / Cmd-V**, replace-on-type, and Backspace/Delete over a selection work
  only when the selection lies wholly on editable buffer text. A selection that
  includes the status line, line-number gutter, `~` filler, or overlay/ex
  transcript can still be copied, but cut/paste/delete/type-replace beeps and
  does nothing.
- **Cmd-A** selects the whole buffer in normal editing; in overlay or ex (Q)
  mode it selects all visible screen text.
- Vi command keys apply only when nothing is selected (or cancel the selection).

### International text input

Typed text goes through the macOS text-input system (`NSTextInputClient` /
`interpretKeyEvents`), so Option-accented characters and dead keys compose
normally: **Option-o** produces `o`-slash, and **Option-u** then **u** produces
`u`-umlaut (the `¨` shows underlined at the cursor as marked text until the next
key composes it). IMEs work the same way. Control keys (`^F`, `^D`, `^R`, …) are
handled directly so they stay vi commands rather than triggering Cocoa's
built-in Emacs-style key bindings.

These are GUI-layer features (vi has no selection concept). Screen-coordinate
selection and copy are handled in `frontend/grid` (`ScreenRangeText`,
`ApplyScreenSel`, `ScreenToBuffer`, `SelectionBufferRange`) and the bridge
(`GoviSetScreenSelection`, `GoviScreenRangeText`, …). Buffer edits still use
engine primitives in `engine/gui.go` (`MoveCursorTo`, `RangeText`,
`DeleteRange`, `ReplaceSelection*`, `InsertText`) and `grid.Locate` for
click-to-position. The engine core remains free of any selection or clipboard
concept.

## Tested

The engine→grid path the app relies on is covered in pure Go by
`frontend/grid` (composer unit tests and an end-to-end
`TestEngineThroughGrid` that drives a real engine with keystrokes and checks the
composed screen). The Swift layer is a thin renderer over that tested path.
