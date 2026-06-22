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

```sh
open gui/build/Govi.app --args /path/to/file
# or directly:
gui/build/Govi.app/Contents/MacOS/Govi /path/to/file
```

Use it like vi: `i` to insert, `Esc`, `:w`, `:q`, `dd`, `/pattern`, etc. All
editing is handled by the embedded engine.

### Mouse and clipboard

In addition to vi keys, the window supports the usual GUI text affordances:

- **Click** to move the cursor.
- **Click-drag** to highlight a selection.
- **Cmd-C / Cmd-X / Cmd-V** (and the Edit menu) copy / cut / paste via the macOS
  system pasteboard; **Cmd-A** selects the whole buffer.
- With a selection active, **typing any character or pasting replaces it** (and
  leaves you in insert mode), and **Backspace/Delete removes it** — standard GUI
  behavior. Vi command keys apply only when nothing is selected.

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
