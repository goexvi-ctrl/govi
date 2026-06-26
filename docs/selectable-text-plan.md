# Plan: Universal screen text selection in GoVi.app

Branch: `selectable-text-plan`

## Problem

Selection in GoVi.app is **buffer-centric**, not **screen-centric**:

1. **Mouse → caret mapping** (`GoviCellToPos` → `grid.Locate`) only covers the editor
   text area. It clamps `y` to `textRows(rows)-1`, so clicks on the status row map to
   the last visible buffer line instead.

2. **Copy** uses `GoviRangeText` → `engine.RangeText`, which reads only from the buffer —
   not from the composed grid.

3. **Highlighting** uses `grid.Selection` with buffer caret positions (`selSpan`), so only
   buffer text can be reverse-video highlighted.

4. **Non-editor layouts** (`:!cmd` overlay, ex/Q transcript, colon-line on the status row)
   are composed separately and never participate in selection. The grid already contains
   all visible text via `GoviRowText`; selection just never reads from it.

## Goal

**Select and copy any visible text** — buffer, status line, `:!cmd` output, ex transcript,
colon prompt — regardless of editor mode.

## Non-goals

- **Cut, paste-over-selection, delete-over-selection, or replace-on-type** when the
  selection touches non-buffer cells (status line, gutter, `~`, overlay, ex transcript) —
  these beep and leave the selection intact.
- **Paste into** status line, overlay, or ex transcript as a distinct editing target.
- **Click-to-position cursor** on non-buffer rows (cursor stays on buffer/input line).
- Changing terminal `govi` behavior (GUI-only).

## Architecture: screen-coordinate selection

Introduce a **screen selection** model parallel to the existing buffer selection. The
composed `grid.Grid` is the source of truth for what appears on screen.

```
┌─────────────────────────────────────────────────┐
│  GoviView (Swift)                                │
│    mouse drag → screen cell (x, y)               │
│    copy → GoviScreenRangeText                    │
│    cut/paste/delete/type → buffer [a,b) only     │
│      (beep if selection touches non-editable)  │
└──────────────────┬──────────────────────────────┘
                   │ C ABI
┌──────────────────▼──────────────────────────────┐
│  gui/bridge                                      │
│    screenSel {active, ax, ay, bx, by}            │
│    GoviSetScreenSelection / GoviScreenRangeText  │
│    GoviSelectionBufferRange → [a,b) or fail   │
└──────────────────┬──────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────────┐
│  frontend/grid                                   │
│    ApplyScreenSelection(g, sel) → reverse video  │
│    ScreenRangeText(g, sel) → clipboard string    │
│    ScreenToBuffer(v, rows, cols, x, y) → Pos,ok│
│    SelectionBufferRange(v, rows, cols, sel)      │
└─────────────────────────────────────────────────┘
```

### Highlighting vs editing

Screen selection **replaces** `GoviSetSelection` / `grid.Selection` for **highlighting
and copy** only. It does **not** eliminate the need for buffer caret ranges when editing.

Today the GUI keeps two parallel stores:

- **Swift** (`selStart` / `selEnd`): used for copy, cut, delete, and replace-on-type via
  `GoviReplaceType` with buffer line/col.
- **Bridge** (`selA` / `selB` → `grid.Selection`): used only for reverse-video highlighting
  in `GoviCompose`.

After this change:

- **Screen coords** drive highlighting (`ApplyScreenSel`) and copy (`ScreenRangeText`).
- **Buffer carets** `[a, b)` are derived from screen endpoints when an edit is attempted, via
  `SelectionBufferRange` / `GoviSelectionBufferRange`. Replace-on-type, cut, paste-over-
  selection, and Backspace/Delete-over-selection all still call the existing engine
  primitives (`GoviReplaceType`, `GoviDeleteRange`, `GoviReplaceText`) with buffer
  positions — either resolved lazily at key/menu time, or cached in Swift when the
  selection is known to lie entirely in the buffer.

`grid.Selection` / `GoviSetSelection` can be removed from the GUI path once screen
selection handles highlighting; the buffer-range derivation is a separate concern tied to
edit operations.

## Implementation plan (4 PRs)

### PR 1: Grid screen-selection primitives (Go, unit-tested)

Add to `frontend/grid`:

| Function | Purpose |
|----------|---------|
| `ScreenRangeText(g Grid, a, b Cell) string` | Extract text from composed cells; normalize endpoints; join rows with `\n`; trim trailing spaces per row to match `GoviRowText` behavior |
| `ApplyScreenSel(g *Grid, sel *ScreenSelection)` | Paint `StyleReverse` on every cell in the normalized rectangle |
| `ScreenToBuffer(v View, rows, cols, x, y) (Pos, bool)` | Map a cell to a buffer caret when it lies on buffer text; `ok=false` for gutter, `~` filler, status, overlay, ex transcript |
| `SelectionBufferRange(v View, rows, cols, sel ScreenSelection) (a, b Pos, ok bool)` | Return the buffer caret range `[a, b)` for a screen selection; `ok=false` unless **every** cell in the normalized rectangle maps to buffer text |
| `SelectionFullyEditable(v View, rows, cols, sel ScreenSelection) bool` | Convenience wrapper: true iff `SelectionBufferRange` succeeds |

**Non-editable cells** (selection touching any of these makes `ok=false`):

- Line-number gutter
- `~` filler past end-of-file
- Status / message row (including colon-line input drawn there)
- `:!cmd` / `:viusage` overlay text and its continue prompt
- Ex (Q) transcript lines (the scrolling output; not the current `:` input line)

Integrate `ApplyScreenSel` into `ComposeSel` **after** composition, so highlighting works
in all three layout paths (editor, overlay, ex mode).

Tests in `frontend/grid/grid_test.go`:

- Status line text selectable
- Overlay output + prompt selectable
- Ex transcript lines selectable
- Multi-row selection includes embedded newlines
- Wide-character / gutter cells handled correctly
- `ScreenToBuffer` returns `ok=false` on status row, gutter, and `~` rows
- `SelectionBufferRange` fails when the rectangle spans buffer text and status line
- `SelectionBufferRange` fails when the rectangle includes any gutter or `~` cell

### PR 2: Bridge API (Go c-archive)

Extend `gui/bridge/bridge.go`:

```c
// New exports (names illustrative)
GoviSetScreenSelection(h, active, ax, ay, bx, by)
GoviScreenRangeText(h, ax, ay, bx, by)  // malloc'd; caller frees
GoviScreenToBuffer(h, x, y, &line, &col) → int  // 1 if mappable
GoviSelectionBufferRange(h, &l1, &c1, &l2, &c2) → int  // 1 if current screen sel maps wholly to buffer [l1,c1)-(l2,c2)
```

Changes to `instance`:

- Replace (or supplement) `selActive/selA/selB` with `screenSelActive`, `screenSelA`,
  `screenSelB` as `(x, y)` cell coords.
- `GoviCompose`: call `ApplyScreenSel` after `grid.ComposeSel`.
- Keep `GoviCellToPos` / `GoviRangeText` for backward compatibility during migration; GUI
  will switch to screen APIs.

### PR 3: GoviView mouse + clipboard (Swift)

**Selection tracking** — store drag endpoints as screen cells `(x, y)`, not buffer carets:

- `mouseDown` / `mouseDragged`: call `GoviSetScreenSelection` with normalized rectangle.
- `mouseDown` (single click, no shift): clear selection; if `GoviScreenToBuffer` succeeds,
  move cursor; otherwise leave cursor unchanged.
- Double/triple-click on **any row**: compute word/line range on that row's `GoviRowText`
  string (screen-local boundaries; reuse logic similar to `DefaultWordBoundary` but on the
  row string, not buffer runes).
- Shift-click extend: extend screen selection endpoint (with word/line snapping when
  applicable).

**Clipboard and editing** (all gated by `GoviSelectionBufferRange`):

- `copy()`: always use `GoviScreenRangeText`; works for any visible text.
- `cut()`, `paste()` over a selection, Backspace/Delete over a selection, and
  replace-on-type: call `GoviSelectionBufferRange` first. On success, use the returned
  buffer `[a, b)` with the existing `GoviDeleteRange` / `GoviReplaceText` /
  `GoviReplaceType` paths. On failure, **do nothing and beep** (`NSSound.beep()`).
- Context menu: Copy always enabled when a selection exists; Cut enabled only when
  `GoviSelectionBufferRange` succeeds.

A selection that **includes any non-editable cell** — status line, `~` filler, gutter line
numbers, overlay text, ex transcript — may still be highlighted and copied, but any operation
that would modify the buffer is rejected with a bell. This matches vi's "invalid operation"
feedback and avoids silently editing the wrong region or ignoring part of the selection.

**Interaction guards**:

- While `PendingOutput` overlay is active: allow select+copy; single click does not move
  buffer cursor; keys still dismiss overlay (existing behavior).
- In ex (Q) mode: allow select+copy on transcript; single click on input line positions
  cursor via existing ex cursor placement.
- Colon mode (`ModeExColon`): status row is the input line — copyable; a selection on the
  colon line fails `SelectionBufferRange` (not buffer text), so paste/delete/type-replace
  over that selection beeps. Ordinary colon-line editing via keys is unchanged.

### PR 4: Tests, docs, polish

- Extend `engine/gui_test.go` or add `gui/bridge/bridge_test.go` (if feasible with cgo) for
  end-to-end: compose overlay → screen select → range text.
- Manual test matrix:

| Scenario | Select | Copy | Cut/paste/delete/type |
|----------|--------|------|----------------------|
| Buffer text only | ✓ | ✓ | ✓ |
| Status line (`file: new file`) | ✓ | ✓ | beep |
| `:!ls` output | ✓ | ✓ | beep |
| `:viusage` pages | ✓ | ✓ | beep |
| Ex (Q) transcript | ✓ | ✓ | beep |
| Colon line (`:w`) | ✓ | ✓ | beep |
| Gutter line numbers | ✓ | ✓ (numbers) | beep |
| `~` empty lines | ✓ | copies `~` | beep |
| Buffer + status (spanning) | ✓ | ✓ | beep |
| Buffer + gutter/`~` (spanning) | ✓ | ✓ | beep |

- Update `gui/README.md` mouse/clipboard section.
- Note fix for `docs/issues.md` item ":!cmd - cannot select output".

## Key design decisions

1. **Screen selection drives highlight + copy.** One code path for display and clipboard;
   mode-agnostic. Buffer caret ranges are derived separately, only when an edit is
   attempted.

2. **Copy always reads the composed grid.** No special cases per mode; what you see is what
   you copy.

3. **Editing operations require a wholly-buffer selection.** `SelectionBufferRange` must
   succeed for every cell in the screen rectangle before cut, paste-over-selection,
   delete-over-selection, or replace-on-type runs. Any selection touching status line,
   gutter, `~` filler, overlay, or ex transcript is rejected with a bell — the selection
   stays highlighted so the user can still copy it.

4. **Word/line double/triple-click on non-buffer rows** uses screen-row text, not
   `GoviWordRange`/`GoviLineRange` (those require buffer positions).

5. **Terminal frontend unaffected.** `grid.Selection` (buffer-based) remains for any future
   use; tcell frontend does not use GUI selection APIs.

## Risks and mitigations

| Risk | Mitigation |
|------|------------|
| Selection includes gutter `~` or line numbers | Copyable; `SelectionBufferRange` fails → edit ops beep |
| Wide chars / wrap boundaries | `ScreenRangeText` reads cells post-compose; same layout as display |
| Partial selection across buffer + status row | Copy works (both regions); edit ops beep |
| User expects type-to-replace on mixed selection | Beep; selection unchanged; user can narrow selection or copy instead |
| `selectAll` in overlay/ex mode | Select all visible screen cells, not whole buffer |
| Performance | Selection is O(selected cells); typical selections are small |

## Estimated effort

| PR | Scope | Size |
|----|-------|------|
| 1 | `frontend/grid` primitives + tests | ~200 lines |
| 2 | `gui/bridge` API + compose integration | ~100 lines |
| 3 | `GoviView.swift` mouse/clipboard | ~150 lines |
| 4 | Tests + docs | ~50 lines |