# Index to `nvi.md` (official Vi/Ex Reference Manual)

`nvi.md` is the OCR/Markdown conversion of the official nvi PDF reference manual
(Keith Bostic, "Vi/Ex Reference Manual", USD:13). Use this index to jump
straight to the authoritative description of a command, option, or feature when
implementing or verifying parity. Line numbers are 1-based into `nvi.md`.

Note: a handful of headings rendered blank in the conversion (the command name
was a punctuation glyph the OCR dropped). The line numbers below point at the
correct heading/body even where the rendered `## ` heading is empty.

## Top-level sections

| # | Section | Line |
|---|---|---|
| 1 | Description | 42 |
| 2 | Additional Features in Nex/Nvi | 54 |
| 3 | Startup Information | 125 |
| 4 | Recovery | 148 |
| 5 | Sizing the Screen | 175 |
| 6 | Character Display | 191 |
| 7 | Multiple Screens | 201 |
| 8 | Tags, Tag Stacks, and Cscope | 224 |
| 9 | Regular Expressions and Replacement Strings | 286 |
| 10 | Scripting Languages | 317 |
| 11 | General Editor Description | 402 |
| 12 | Vi Description | 478 |
| 13 | Vi Commands | 574 |
| 14 | Vi Text Input Commands | 1375 |
| 15 | Ex Addressing | 1442 |
| 16 | Ex Description | 1489 |
| 17 | Ex Commands | 1532 |
| 18 | Set Options | 2118 |
| 19 | Index | 2552 |

### Section 2 — additional nex/nvi features

| Feature | Line |
|---|---|
| Background and foreground screens | 62 |
| Command Editing (cedit) | 66 |
| Displays | 70 |
| Extended Regular Expressions | 74 |
| File Name Completion | 78 |
| Left-right scrolling | 89 |
| Message Catalogs | 93 |
| Incrementing numbers | 97 |
| Scripting languages | 105 |
| Split screens | 109 |
| Tag stacks | 113 |
| Usage information | 117 |
| Word search | 121 |

### Section 12 — vi concepts

| Concept | Line |
|---|---|
| previous context | 501 |
| motion | 505 |
| count | 533 |
| word | 539 |
| bigword | 545 |
| paragraph | 551 |
| section | 562 |
| sentence | 568 |

## Section 13 — Vi commands

| Command | Line |
|---|---|
| `^A` (word search fwd) | 578 |
| `^B` (page back) | 586 |
| `^D` (scroll down) | 595 |
| `^E` (scroll line down) | 601 |
| `^F` (page forward) | 607 |
| `^G` (file info) | 613 |
| `^H` / `h` (left) | 619 / 621 |
| `^J` / `^N` / `j` (down) | 634 / 636 / 638 |
| `^L` / `^R` (repaint) | 646 / 648 |
| `^M` / `+` (next line) | 654 / 656 |
| `^P` / `k` (up) | 664 / 666 |
| `^T` (tag pop) | 674 |
| `^U` (scroll up) | 685 |
| `^W` (switch screen) | 691 |
| `^Y` (scroll line up) | 697 |
| `^Z` (suspend) | 703 |
| `<escape>` | 709 |
| `^]` (tag push) | 713 |
| `^^` (alternate file) | 726 |
| `<space>` / `l` (right) | 732 / 734 |
| `!` (filter) | 742 |
| `#` (increment number) | 763 |
| `$` (eol) | 773 |
| `%` (match) | 786 |
| `&` (repeat subst) | 794 |
| `` ` `` / `'` (mark motions) | 804 / 806 |
| `(` (sentence back) | 816 |
| `)` (sentence fwd) | 829 |
| `,` (reverse f/F/t/T) | 837 |
| `-` (prev line) | 845 |
| `.` (repeat) | 853 |
| `/` `?` `N` (search) | 866 |
| `n` (repeat search) | 868 |
| `;` (repeat f/F/t/T) | 909 |
| `<` (shift left) | 919 |
| `>` (shift right) | 921 |
| `@` (execute buffer) | 930 |
| `A` (append eol) | 938 |
| `B` (back bigword) | 944 |
| `C` (change to eol) | 952 |
| `D` (delete to eol) | 958 |
| `E` (end bigword) | 969 |
| `F` (find back) | 977 |
| `G` (goto line) | 985 |
| `H` (home) | 993 |
| `I` (insert at first nonblank) | 1001 |
| `J` (join) | 1010 |
| `L` (last screen line) | 1018 |
| `M` (middle) | 1026 |
| `O` (open above) | 1036 |
| `P` (put before) | 1047 |
| `Q` (to ex mode) | 1053 |
| `R` (replace mode) | 1059 |
| `S` (substitute line) | 1067 |
| `T` (till back) | 1073 |
| `U` (restore line) | 1081 |
| `W` (fwd bigword) | 1092 |
| `X` (delete back) | 1100 |
| `Y` (yank line) | 1106 |
| `ZZ` (save-quit) | 1112 |
| `[[` (section back) | 1120 |
| `]]` (section fwd) | 1131 |
| `^` (first nonblank) | 1139 |
| `_` (count line, first nonblank) | 1145 |
| `a` (append) | 1153 |
| `b` (back word) | 1159 |
| `c motion` (change) | 1170 |
| `d motion` (delete) | 1176 |
| `e` (end word) | 1182 |
| `f` (find) | 1190 |
| `i` (insert) | 1198 |
| `m` (set mark) | 1209 |
| `o` (open below) | 1217 |
| `p` (put after) | 1225 |
| `r` (replace char) | 1231 |
| `s` (substitute char) | 1239 |
| `t` (till) | 1248 |
| `u` (undo) | 1256 |
| `w` (fwd word) | 1264 |
| `x` (delete char) | 1272 |
| `y motion` (yank) | 1281 |
| `z` (screen position) | 1289 |
| `{` (paragraph back) | 1309 |
| `\|` (column) | 1317 |
| `}` (paragraph fwd) | 1332 |
| `~` (reverse case) | 1340 |
| `~ motion` (tildeop) | 1352 |
| `<interrupt>` | 1360 |

## Section 14 — Vi text input commands

| Command | Line |
|---|---|
| `<nul>` (replay input) | 1383 |
| `^D` (autoindent erase) | 1387 |
| `^^D` (erase all indent) | 1393 |
| `0^D` (reset indent) | 1397 |
| `^T` (shift to shiftwidth) | 1401 |
| `<erase>` / `^H` | 1412 |
| `<literal-next>` (`^V`) | 1416 |
| `<escape>` (end input) | 1420 |
| `<line erase>` | 1424 |
| `^W` / `<word erase>` | 1428 / 1430 |
| `^X[0-9A-Fa-f]+` (hex) | 1434 |
| `<interrupt>` | 1438 |

## Section 15/16 — Ex addressing & description

| Topic | Line |
|---|---|
| Ex addressing | 1442 |
| line / range / count | 1502 / 1506 / 1510 |

## Section 17 — Ex commands

Some headings are blank in the conversion; the resolved command name is shown.

| Command | Line |
|---|---|
| `"` (comment) | 1545 |
| `^D` (scroll) | 1549 |
| `!` / `[range]!` (filter) | 1559 / 1561 |
| `[range] nu[mber]` / `[range] #` | 1580 / 1582 |
| `@ buffer` / `* buffer` | 1590 / 1592 |
| `[range] <` (shift left) | 1596 |
| `[line] =` (line number) | 1604 |
| `[range] >` (shift right) | 1610 |
| `ab[brev]` | 1618 |
| `[line] a[ppend][!]` | 1639 |
| `ar[gs]` | 1645 |
| `bg` | 1651 |
| `[range] c[hange][!]` | 1660 |
| `chd[ir][!]` / `cd[!]` | 1666 / 1668 |
| `[range] co[py]` / `[range] t` | 1676 / 1678 |
| `cs[cope]` | 1684 |
| `[range] d[elete]` | 1688 |
| `di[splay]` | 1694 |
| `e[dit][!]` / `ex[!]` | 1707 / 1709 |
| `exu[sage]` | 1721 |
| `f[ile]` | 1727 |
| `fg` | 1733 |
| `[range] g[lobal]` / `[range] v` | 1744 / 1746 |
| `he[lp]` | 1762 |
| `[line] i[nsert][!]` | 1768 |
| `[range] j[oin][!]` | 1774 |
| `[range] l[ist]` | 1791 |
| `map[!]` | 1797 |
| `ma[rk]` / `k` | 1813 / 1815 |
| `[range] m[ove]` | 1821 |
| `mk[exrc][!]` | 1830 |
| `n[ext][!]` | 1836 |
| `o[pen]` | 1844 |
| `pre[serve]` | 1854 |
| `prev[ious][!]` | 1860 |
| `[range] p[rint]` | 1873 |
| `[line] pu[t]` | 1877 |
| `q[uit][!]` | 1883 |
| `[line] r[ead]` | 1891 |
| `rec[over]` | 1899 |
| `res[ize]` | 1905 |
| `rew[ind][!]` | 1916 |
| `se[t]` | 1922 |
| `sh[ell]` | 1930 |
| `so[urce]` | 1936 |
| `[range] s[ubstitute]` / `~` | 1942 / 1944 |
| `su[spend]` / `st[op]` / `^Z` | 1971 |
| `ta[g][!]` | 1977 |
| `tagn[ext][!]` | 1987 |
| `tagp[op][!]` | 2000 |
| `tagp[rev][!]` | 2008 |
| `tagt[op][!]` | 2016 |
| `una[bbrev]` | 2024 |
| `u[ndo]` | 2030 |
| `unm[ap][!]` | 2039 |
| `ve[rsion]` | 2045 |
| `vi[sual]` (ex -> vi) | 2049 |
| `vi[sual][!]` (vi, new screen) | 2055 |
| `viu[sage]` | 2061 |
| `[range] w[rite]` / `wn` / `wq` family | 2067-2073 |
| `[range] x[it][!]` | 2090 |
| `[range] ya[nk]` | 2098 |
| `[line] z` (window) | 2102 |

### Section 8 — cscope subcommands

| Command | Line |
|---|---|
| `:cs[cope] f[ind]` | 253 |
| `:cs[cope] h[elp]` | 265 |
| `:display c[onnections]` | 272 |
| `:cs[cope] k[ill] #` | 276 |
| `:cs[cope] r[eset]` | 280 |

### Section 10 — Tcl/Tk scripting API

| Function | Line |
|---|---|
| viAppendLine | 327 |
| viDelLine | 331 |
| viGetLine | 335 |
| viInsertLine | 339 |
| viLastLine | 343 |
| viSetLine | 347 |
| viGetMark | 351 |
| viSetMark | 355 |
| viGetCursor | 359 |
| viSetCursor | 363 |
| viMsg | 367 |
| viEndScreen | 375 |
| viSwitchScreen | 382 |
| viMapKey | 386 |
| viUnmMapKey | 390 |
| viGetOpt | 394 |
| viSetOpt | 398 |

## Section 18 — Set options

| Option | Line |
|---|---|
| altwerase | 2133 |
| autoindent (ai) | 2137 |
| autoprint (ap) | 2147 |
| autowrite (aw) | 2156 |
| backup | 2166 |
| beautify (bf) | 2174 |
| cdpath | 2178 |
| cedit | 2182 |
| columns (co) | 2193 |
| comment | 2197 |
| directory (dir) | 2201 |
| edcompatible (ed) | 2205 |
| escapetime | 2209 |
| errorbells (eb) | 2213 |
| exrc (ex) | 2217 |
| extended | 2221 |
| hardtabs (ht) | 2242 |
| iclower | 2246 |
| ignorecase (ic) | 2250 |
| keytime | 2254 |
| leftright | 2258 |
| lines (li) | 2262 |
| lisp | 2266 |
| list | 2272 |
| lock | 2279 |
| magic | 2283 |
| matchtime | 2287 |
| mesg | 2291 |
| msgcat | 2295 |
| modelines (modeline) | 2303 |
| noprint | 2313 |
| number (nu) | 2322 |
| octal | 2326 |
| open | 2330 |
| optimize (opt) | 2334 |
| paragraphs (para) | 2340 |
| path | 2348 |
| print | 2352 |
| prompt | 2356 |
| readonly (ro) | 2360 |
| recdir | 2369 |
| redraw (re) | 2379 |
| remap | 2385 |
| report | 2389 |
| ruler | 2393 |
| scroll (scr) | 2397 |
| searchincr | 2406 |
| sections (sect) | 2410 |
| secure | 2414 |
| shell (sh) | 2418 |
| shellmeta | 2422 |
| shiftwidth (sw) | 2426 |
| showmatch (sm) | 2430 |
| showmode (smd) | 2434 |
| sidescroll | 2438 |
| slowopen (slow) | 2442 |
| sourceany | 2451 |
| tabstop (ts) | 2457 |
| taglength (tl) | 2461 |
| tags (tag) | 2465 |
| term (ttytype, tty) | 2467 |
| terse | 2471 |
| tildeop (to) | 2475 |
| timeout (to) | 2479 |
| ttywerase | 2483 |
| verbose | 2487 |
| w300 | 2491 |
| w1200 | 2495 |
| w9600 | 2502 |
| warn | 2506 |
| window (w, wi) | 2510 |
| windowname | 2518 |
| wraplen (wl) | 2522 |
| wrapmargin (wm) | 2528 |
| wrapscan (ws) | 2541 |
| writeany (wa) | 2545 |
