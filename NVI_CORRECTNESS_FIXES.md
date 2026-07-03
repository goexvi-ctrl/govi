# nvi Correctness Fixes: Regression Reference for govi

Compiled from the nvi git log. Each entry is a behavioral or correctness bug fixed
in nvi that a Go reimplementation should independently verify. Build system,
wide-char rendering, and DB backend changes are excluded.

---

## Insert Mode / Text Input

1. **Autoindent + `^^D` abort** (`fad63735`)
   Sequence `:se ai<Enter>i<Tab><Enter>^^D` triggered an abort. The caret state
   was overloaded to track both current state and a "no-change" sentinel
   (`C_NOCHANGE`). These must be tracked separately.

2. **Buffer too small in `v_txt`** (`c9f46804`)
   Copy in `v_txt` used a buffer that was too small; old bug, likely triggered
   by large lines.

3. **`SC_TINPUT` not cleared on error path** (`190a1d89`, NetBSD PR 21797)
   On the error path at the end of `v_txt`, `SC_TINPUT` was not cleared before
   calling `txt_err`, leaving the editor believing it was still in input mode.

4. **Cursor position wrong on partial multi-column character during input**
   (`791d23a2`)
   Cursor column calculation was off when the last character on the line
   occupied a partial cell.

---

## Search / Regex

5. **Smatcher vs. lmatcher selection wrong on 64-bit** (`9afcab2a`)
   The test to decide whether to use the small (bit-vector) matcher compared
   against `sizeof(char *)` instead of `sizeof(int)`. On 64-bit systems where
   pointers are 8 bytes, the smatcher could be selected when it was too small
   for the number of regex states, causing incorrect matching.

6. **`SEARCH_EXTEND` flag not passed to `re_compile`** (`e747b5fb`)
   `search_init` ignored the `SEARCH_EXTEND` flag when calling `re_compile`,
   so functions like `v_searchw` that request ERE syntax regardless of the
   `extended` option were silently using BRE.

7. **Wrong character class in `p_b_cclass`** (`fd5795cc`)
   A logical glitch caused `p_b_cclass` to return the wrong `cclass` entry --
   e.g., returning `[:alpha:]` data when the pattern said `[:alnum:]`.

---

## `^A` Word Search

8. **`^A` idempotency** (`b163b7ce`)
   Repeatedly typing `^A` changed the current keyword when the keyword was a
   single non-word character (it should cycle through occurrences without
   changing the pattern).

9. **`^A` fails when keyword starts with a non-word character** (`9ba2bd88`)
   With `wrapscan` set, placing the cursor over a character like `^` and typing
   `^A` produced "Pattern not found" instead of finding the next occurrence.

10. **Keywords starting with non-word characters built wrong** (`7ff00d30`)
    `v_curword` constructed the pattern for keywords starting with a non-word
    character in a way incompatible with historical tag string search and POSIX.

11. **`^A` word search ignored word-end boundaries** (`bfbd19be`)
    Given `a ab abc` with cursor at start, `^A` then `^A` again would
    incorrectly land on `ab` then `abc`; word-end boundaries were not being
    checked.

---

## Cut / Yank / Delete

12. **Appending to unused cut buffer used wrong cut buffer name** (`53b5b0c8`)
    An `if` clause tested `isupper()` return and assumed it returns exactly `1`.
    On glibc systems where it can return any non-zero value, an empty cut buffer
    `a` would be shown as `A` after `"Ayy`. Repeating `"Ayy` would show the
    bogus buffer twice rather than appending.

---

## Join

13. **`join` didn't conform to POSIX** (`069f378d`)
    The join command's behavior for combining lines and handling trailing
    whitespace did not match the standard.

14. **`join` with single address misbehaved** (`166166e8`)
    `join` needed to distinguish between a 1-address and a 2-address
    invocation; treating a single-address `j` as a 2-address command by
    bumping the count produced incorrect results.

---

## Tags / Cscope

15. **`taglength` option had no effect** (`cda98349`)
    Tags were not being truncated to the length specified by `taglength`.

16. **`ctag_search` missing `SEARCH_PARSE` flag** (`fe58ccc1`)
    Tag pattern searches did not parse the pattern (e.g., anchors `^`/`$`),
    so tag-file patterns that used regex syntax were searched literally.

17. **Cscope: freed memory reuse; `:cs add cscope.out` didn't work** (`9552e695`)
    When cscope died, freed memory was reused. Also `:cs add cscope.out`
    (without `./`) silently failed; only the `./`-prefixed form worked.

---

## Screen Layout / Display

18. **Screen offset of top line could exceed screen count** (`0625ddc2`)
    Operations that change the number of screen-rows a line occupies (e.g.,
    changing `tabstop`) could leave the current screen offset beyond the last
    screen for that line, causing the screen to not be fully repopulated.

19. **`leftright` + `number` options together caused a hang** (`c8480de6`)
    Setting both `leftright` and `number` simultaneously resulted in a very
    long wait (effectively an infinite loop in the screen-map code).

20. **Scrolling broken with `leftright` option** (`f42d9cf4`)
    Scrolling to a line not currently visible in the left/right window was
    handled incorrectly when the `leftright` option was set.

21. **`vi +line file` made the target line the top of the screen** (`7b7a25f3`)
    A prior fix to `exf.c` had the side effect of making the line specified by
    `+line` the topmost line on screen; the same bug appeared when switching
    between files.

22. **`vs_line` didn't return cursor y-position on no-draw** (`7c6b85e7`)
    When `vs_line` skipped drawing due to pending ex output, it did not update
    the cursor y-position, causing `vs_paint` to get confused about where the
    cursor was.

23. **`vs_swap` used window before initialization** (`b5ced738`)
    In `vs_sm_fill`, the conversion buffer in `wp` is needed to read lines from
    files. But `vs_swap` called `vs_sm_fill` before initializing `wp`.

---

## Multi-Screen / Split Windows

24. **TTY left in ex mode after `q` with multiple screens** (`72c79701`)
    In ex mode with more than one screen, `:Vi<Enter>Qq<Enter>` returned to vi
    mode visually but left the tty in line-buffered/echo mode. Root cause:
    `cl_screen` used the departing screen's `SC_SCR_EX/VI` flags to decide if
    a mode switch was needed; those flags needed to be transferred to the new
    screen.

25. **Screen confusion going ex->vi with split screens** (`92abd968`)
    Going to ex mode with split screens and then back to vi caused serious
    display confusion because the curses private data on hidden screens was
    stale. Screens must be discarded when moved to the hidden queue, and also
    when going to ex mode.

26. **`CIRCLEQ_INSERT_BEFORE` called in wrong `else` branch in `vs_insert`**
    (`1a75ac3e`)
    The linked-list insertion in `vs_insert` was in the wrong branch of an
    if/else, causing split window ordering to be wrong in certain
    configurations.

27. **Border case when viewing the same file in two screens** (`29612133`)
    `vs_smap.c` had a boundary condition error when two screens displayed the
    same file and their screen maps needed to be independently maintained.

---

## Signal Handling / TTY

28. **`tcsetattr()` EINTR not retried; nvi exited on rapid SIGWINCH** (`914f6937`)
    During a fast series of window-resize signals, `tcsetattr()` could return
    `EINTR`; nvi treated this as a fatal error and exited rather than retrying
    the call.

---

## Options

29. **`OPT_GLOBAL` flag checked in wrong location** (`73aa3e3d`)
    When setting options, the test for the `OPT_GLOBAL` flag was done in the
    wrong place, causing global options to be set per-file or vice versa.

30. **Octal option value caused core dump** (`0e34a6c3`)
    Parsing an octal value for a numeric option (e.g., `0755`) triggered a
    crash.

---

## Abbreviations

31. **Abbreviations broken** (`8126d0f8`)
    Abbreviation expansion was not working correctly.

---

## Ex Commands / Macros

32. **`@` macro execution stopped after first run** (`bf42f055`)
    Repeated execution of a named buffer via `@` was blocked by an incorrect
    buffer-length check that prevented reuse.

33. **`%` expansion in filters/read stopped working** (`b6fb6218`)
    The `%` (current filename) expansion in ex filter commands (`:r !cmd`) and
    `:read` was broken.

34. **Invalid ex input caused infinite loop** (`9c13875b`)
    Certain invalid input to an ex command caused the parser to loop
    indefinitely instead of reporting an error.

35. **`cedit` log error killed the option instead of continuing** (`853b1984`,
    `dc049d47`)
    A failure in `v_ecl_log` (for the `cedit` option, which logs ex commands
    to a buffer) was incorrectly causing the command to be aborted and the
    `cedit` option to be cleared.

---

## File Operations

36. **Edited files lacked close-on-exec flag** (`8c521b71`)
    Files opened for editing were not marked `O_CLOEXEC`. With flock-style
    locking, this caused the file to remain locked for the lifetime of any
    child process spawned by vi (shell commands, filters, etc.).

37. **`-r` recovery listing hardcoded program name as "vi"** (`7911a787`)
    The message printed by `vi -r` (when there were no files to recover)
    hardcoded "vi" instead of using `argv[0]`, so it was wrong when invoked
    as `ex` or `view`.

---

## Memory / Pointer Safety

38. **Wrong pointer freed in allocation path** (`60891280`)
    A code path freed a random/wrong pointer instead of the one that had been
    allocated.

39. **NULL not cast to pointer type in variadic call** (`c67c478d`)
    NULL was passed without a cast to `char *` in a variadic function call. On
    ABIs where `sizeof(char *) != sizeof(int)`, this causes undefined behavior
    (stack corruption).

40. **Replacement string buffer too small** (`3ef5e5e5`)
    Memory allocated for a regex replacement string was too small for the
    result.

---

## Blank / Space Handling

41. **Wide characters incorrectly treated as blanks** (`917d600f`)
    Direct `isblank()` calls on `CHAR_T` values could misidentify some wide
    characters as blanks. Fixed by routing through the `ISBLANK` macro that
    handles the wide-char case.

---

## Keyboard

42. **`<End>` key not recognized on AT-compatible keyboards** (`2fd9183a`)
    The terminfo/termcap entry for the `<End>` key on standard PC keyboards
    was missing from the key map.
