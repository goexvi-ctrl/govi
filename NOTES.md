# govi — an embeddable Go rewrite of nvi

`govi` reimplements the BSD `nvi` vi/ex editor in Go with two goals:

1. **User-perceptible bug-for-bug compatibility with nvi.** Same modes, commands,
   key bindings, semantics, and messages. The implementation is free to differ
   (no Berkeley DB, GC instead of manual memory, a fresh regex engine); nvi's
   actual *defects* (manual-refresh artifacts, last-character-in-line glitches)
   are deliberately **not** reproduced.
2. **Embeddability.** The editor core (`engine/`) has no terminal, curses, or GUI
   dependency. Hosts drive it across a small interface and render a semantic
   document model, so the same engine powers a terminal frontend today and a
   graphical application later.

This repository is a **separate git repo** living inside the C `nvi` source tree
(which `.gitignore`s it). The C tree is kept alongside as a behavioral reference
and as the conformance-test oracle.

## Layout

```
engine/            embeddable core — no terminal/GUI imports
  frontend.go      Frontend + View (semantic document model) + ChangeSet
  event.go         input Event sum type
  buffer/          LineStore: line-addressed text storage (in-memory + paged)
  ...              (undo, mark, register, vi, ex, regex, options, seq — see plan)
frontend/tcell/    terminal frontend (renders View to cells)
cmd/govi/          terminal entry point (govi; govi -g launches Govi.app)
internal/conformance/  oracle harness: drives C nvi + govi, diffs observable output
```

## The embedding boundary

The host calls into the engine (`Input`, `Resize`, `Open`, `RunEx`); the engine
calls back through `Frontend.Render(View, ChangeSet)`. The `View` exposes buffer
lines, cursor, mode, viewport, and the message line as data. See
`engine/frontend.go`.

## Status

Early scaffolding. See the phased build plan accompanying this work. Run tests
with `go test ./...`.
