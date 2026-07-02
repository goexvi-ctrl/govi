# parity.md review (2026-07-01) -- evidence, corrections, and fixes

A full row-by-row verification of [`parity.md`](parity.md) against nvi 1.81.6
(`/opt/homebrew/bin/nvi`, the reference the matrix is defined against) through
the goterm headless-terminal harness (`~/src/goterm`). Every vi command, ex
command, and option row was either exercised by an existing battery, exercised
by a new probe written for this review (`goterm/parityreview_test.go`), or --
where the PTY model cannot drive a feature -- verified by reading the source.
Divergence policy for this pass: fix small findings inline, record large ones
in `GOTERM_DIVERGENCES.md`.

## How to re-run the evidence

```sh
cd ~/src/nvi/govi && go build -o ~/bin/govi ./cmd/govi && go test ./...
cd ~/src/goterm && go test -run 'TestDiverge|TestCoverage|TestParity' -v .
```

Every remaining DIVERGE in that run maps to a numbered GOTERM_DIVERGENCES.md
entry or an "expect DIVERGE" probe that pins a documented gap.

## govi bugs found and FIXED (catalog #49-#53)

| # | Finding | Fix |
|---|---------|-----|
| 49 | insert `^T` shifted the line's indent (vim-style); nvi indents at the cursor (txt_dent) | engine/viinsert.go `insertIndent`; conformance cases insert-ctrl-t-midline/-tab |
| 50 | `:vi[sual] file` ignored the file argument; nvi's vi-mode form IS `ex_edit` | engine/exmode.go delegates to exEdit |
| 51 | `window` option was inert (parity.md over-claimed it functional): no map resize, `^F`/`^B` paged by screen rows | engine/{screen,vi,options,engine}.go: f_window clamp, immediate map resize with z-style growth, `count*window-2` paging, f_lines resize tracking, `scroll` display default |
| 52 | `:preserve` snapshot deleted on clean exit (DATA-SAFETY: defeats `vi -r`) | engine/recovery.go `recoverKeep`; TestRecoveryPreservedSurvivesExit |
| 53 | `autowrite` only honored by suspend (parity.md over-claimed): `:n`/`:prev`/`:rew`/tag jumps/`^^` failed instead of writing | engine/exfile.go `checkModified` (nvi file_m1), applied at all 7 guard sites |

Also: `directory` option default now follows `$TMPDIR` like nvi (was `/tmp`),
so `:set all` values line up.

## parity.md corrections (doc was wrong or incomplete)

- `^G`, `:args`, `:file` -> 🟡: behavior right, but a message longer than one
  line meets the info-message pagination gap (nvi pages into a `+=+=` overlay;
  govi truncates on the status line).
- `:append`/`:insert`/`:change` -> 🟡: buffer result right; nvi's scrolled
  "ex input mode" display differs (catalog #28).
- `Q`, `^\` -> 🟡: the switch works; govi's ex screen clears to a `:` prompt
  where nvi keeps the buffer text (catalog #29 cosmetic note).
- `:display` -> 🟡: all four subcommands answer; `buffers` and `tags` format
  differently from nvi (catalog #54). The old "all four subcommands" ✅ claim
  and the roster's "#37 unimplemented" note were both stale.
- `scroll` note rewritten: nvi's vi-mode `^D`/`^U` use `defscroll` (a `^D`
  count), NOT this option -- govi matches nvi in vi mode anyway; nvi reads
  `scroll` only in ex contexts govi lacks (`:z`, ex `^D`).
- `:*` note: nvi's bare `:*` carries no default address (address-taking buffer
  contents fail); govi deliberately runs the buffer like `:@`.
- `filec` note: mechanism equal, default differs (govi `<tab>`, nvi empty) --
  known/accepted.
- `preserve` note: snapshot survives exit; govi writes no recover-mail file.

## parity.md rows CONFIRMED accurate (spot list)

- All vi punctuation/digit/letter commands and control keys not listed above
  (existing batteries + parity-vikeys probes; 0/11 diverged).
- `^[` cancel of pending operator/count/register: d<ESC>w, 5<ESC>x, 2d<ESC>w,
  "a<ESC>x all match.
- Unbound keys footnote (`g K v V ^O ^_ =`): bell/no-op in both editors.
- Split screens: `:E`, `:vsplit`, `^W`, `:resize`, `:bg`/`:fg`, close -- 7/7
  match (first-ever harness coverage; the goterm coverage manifest previously
  excluded splits as "not implemented" and was updated).
- cscope: `:cs add` + `:cs find g` against the real cscope binary matches.
- Tag stack: `:tagnext`/`:tagprev`/`:tagtop` (multi-match tags file) match.
- `:ex file` == `:edit`; `:cd` + relative `:e`; `:r file`; `:wn` (per-editor
  fixture copies) all match.
- Missing-command rows confirmed missing in govi and present in nvi:
  ex `^D` scroll, `:mkexrc` (nvi writes the file, govi does not), `:z`.
- Options verified functional by behavior probes: ignorecase, magic/nomagic,
  tildeop, wrapscan, window (post-fix), shell (via filter), autowrite
  (post-fix), taglength, tags, ruler, showmode (status-row comparison),
  filec (explicit `:set filec=<tab>`), recdir+preserve (filesystem check),
  readonly+lock+writeany+secure (existing #41/#42/#45 batteries), autoindent,
  list, number, shiftwidth, tabstop, wrapmargin, report/window defaults via
  `:set all`.
- Inert (⚙️) rows honestly inert -- nvi visibly honors what govi ignores:
  paragraphs, sections, extended, remap (recursive maps), leftright.
- `:set all` content: only expected deltas remain -- govi's 4 extension
  options, nvi-only `modeline`/`noprint` (documented omissions), and the
  `filec` default. Layout cannot be byte-identical while the option sets
  differ; the column algorithm is equivalent.
- `^E`/`^Y` wrapped-line gap (#44) reproduced exactly as documented.

## Verified by source reading (not PTY-drivable)

- `^C` interrupt (engine/interrupt.go; also runtime-verified in the prior
  adversarial audit), `^Z`/`:stop`/`:suspend` (engine/suspend.go + tcell
  Suspender), `^L`/`^R` (tcell Sync), `:shell` (RunShell), `:recover` (needs a
  -r restart cycle; engine/recovery.go), quit family (dirty-guard write tests
  cover the exit effects), GoVi.app column deltas (gui/bridge, gui/macos).

## Harness (goterm) changes made by this review

- **Lock-race artifact fixed**: since govi implements nvi's advisory flock
  (#45), any comparison where both editors open the SAME fixture file made the
  second starter read-only behind the other's lock, stuck at a "Press any key"
  prompt that ate the first byte of the test's keys (10 false DIVERGEs across
  the multifile/tags/source/secure batteries; e.g. `:n` decayed to `n` + bell).
  `runArgs` now starts both editors with `EXINIT="set nolock"`, scoped so the
  lock assertions later in the same test still see real locking.
- **New permanent battery** `parityreview_test.go` (~60 probes): vi keys,
  wrapped-line scroll, splits, ex-mode switches, uncovered ex commands,
  option behavior + inertness spot-checks, `:set all`, status-row options,
  `:display` subcommands, filec, cscope, filesystem effects. New helpers:
  `runSepCase` (per-editor fixture copies for probes that WRITE the file),
  `shortDir` (short /tmp paths so path-bearing nvi messages stay on one line
  instead of triggering the pagination overlay), `runStatusCase` (status-row
  comparison), and a "0" prompt-dismissal idiom for probes whose nvi side ends
  at a two-message overlay prompt.
- Coverage manifest updated: splits and cscope moved from "excluded" to
  covered.

## Still open after this review

- Info-message pagination for long/multi-line messages (`^G`, `:args`, `:file`,
  multi-message sequences) -- display subsystem, catalog Inconclusive note.
- Ex input/screen display: #28 (`:a`/`:i`/`:c`), #29 (`Q` screen layout and ex
  autoprint), ex `^D` scroll, `:z`, `:mkexrc`.
- `:display tags`/`buffers` format (#54).
- Insert-mode `^D` ai-scoped dedent + `0^D`/`^^D` (gaps report #6/#7; `^T`
  half is done).
- Wrapped-line rendering cluster (#43/#44, leftright) -- architectural.
- Inert-by-design options (see the ⚙️ rows), recursive maps (`remap`).
