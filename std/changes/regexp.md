# std/regexp changes

## 2026-10-22: perf: Go's one-pass matcher for patterns anchored with \A

- Kind: perf
- Commit: 7c1a454e5
- Found by: bench/results/regexp-ascii-bitmap-inline-2026-10-22.md (the one-pass matcher was the last of Go's matchers not ported; anchored row 1.20x Go's instructions)
- Go reference: regexp/onepass.go, regexp/exec.go doOnePass (go1.26.1); onepass_test.go's patterns
- Test: std/native/regexp_test.xo (4,760 anchored cases from Go), internal/llvm/testdata/golden/regexp_table, internal/stdtest corpus regexp; timing row against Go: bench/results/regexp-ascii-bitmap-inline-2026-10-22.md

A pattern starting with `\A` (`^` without `(?m)`) whose every alternation is
decided by the next rune now runs in one pass over the text: no backtracking,
no visited bits, no thread list. Compile checks the program as Go's
compileOnePass does (alternation legs with disjoint rune sets, empty matches
only at `\z`, fewer than 1,000 instructions) and keeps its tables in
`Prog.one`; other patterns run as before. `xo bench --ab` instructions on the
anchored row (short texts, `is_match` and `captures`): 0.87x of the
backtracker, 1.04x Go's; the unanchored rows moved within 0.3%.

## 2026-10-22: perf: the Pike matcher skips empty width contexts it cannot use

- Kind: perf
- Commit: 08c6beb97
- Found by: bench/results/regexp-ascii-bitmap-inline-2026-10-22.md (replace 1.53x Go; profile: add 51%, exec 30%, empty_context and is_word_char 8%)
- Go reference: regexp/exec.go (the context of each position is only needed by empty width instructions)
- Test: internal/llvm/testdata/golden/regexp_table, internal/stdtest corpus regexp, std/native/regexp_test.xo; timing row against Go: bench/results/regexp-ascii-bitmap-inline-2026-10-22.md

The compiled program records whether any instruction is an empty width
assertion (^, $, \b); when none is, the Pike loop no longer computes the
before and after context at every position, and the step copies a thread's
first two group positions without a loop. `xo bench --ab` instructions
against the previous step: find_all 0.968x, replace 0.965x (replace 1.47x Go's
instructions); the backtracker's rows moved within 0.3%; output unchanged.

## 2026-10-22: perf: the Pike matcher's threads without a call or a copy loop

- Kind: perf
- Commit: 6370baa83
- Found by: bench/results/regexp-ascii-bitmap-inline-2026-10-22.md (replace 1.68x Go)
- Go reference: regexp/exec.go add (go1.26.1)
- Test: internal/llvm/testdata/golden/regexp_table, internal/stdtest corpus regexp, std/native/regexp_test.xo; timing row against Go in the result file

`add` copies a thread's first two group positions without a loop and queues the
thread of an ALT's first branch directly when it is a rune instruction. `xo
bench --ab` instructions against the previous step: find_all 0.887x, replace
0.911x (find_all now 0.93x Go's instructions, replace 1.53x); output unchanged.

## 2026-10-22: perf: ASCII bitmaps and inlined hot calls in both matchers

- Kind: perf
- Commit: a5d648ddc
- Found by: bench/results/regexp-first-byte-2026-10-22.md (match 1.87x, replace 1.98x Go)
- Go reference: regexp/backtrack.go (shouldVisit, push) and regexp/exec.go (add), whose calls Go's compiler inlines
- Test: internal/llvm/testdata/golden/regexp_table, internal/stdtest corpus regexp, std/native/regexp_test.xo; timing row against Go: bench/results/regexp-ascii-bitmap-inline-2026-10-22.md

The compiled program keeps a bitmap of the ASCII bytes each rune instruction
accepts and its literal prefix (rebuilt on every call before); the backtracker
appends ALT and capture-restore jobs without a push call and checks the visited
bit inline; the Pike queue's membership test is inline in `add`. `xo bench
--ab` instructions: match 0.567x, find_all 0.807x, replace 0.849x, captures
0.694x (match 1.87x to 1.06x Go's instructions, captures 0.84x Go); output
unchanged.

## 2026-10-22: perf: a first byte filter and one Int per backtrack job

- Kind: perf
- Commit: efc7374be
- Found by: bench/results/regexp-time-csv-vs-go-2026-10-21.md, re-measured in bench/results/regexp-first-byte-2026-10-22.md
- Go reference: regexp/backtrack.go (bitState jobs); the first byte filter is Xo's own (Go's prefix scan only covers literal prefixes)
- Test: internal/llvm/testdata/golden/regexp_table (extended: folds to ASCII, empty matches, assertions), internal/stdtest corpus regexp

The compiled program records the ASCII bytes a match can start with (when no
match is empty); both matchers skip start positions at other ASCII bytes
(non-ASCII bytes are always tried). The backtracker's three job lists are one
list of packed Ints. `xo bench --ab`: match wall 0.757x, find_all 0.911x,
captures 0.636x; replace is flat (+0.9% instructions, wall within noise).

## 2026-10-21: perf: patterns anchored at the start try one position

- Kind: perf
- Commit: (filled in after the push)
- Found by: bench/results/regexp-time-csv-vs-go-2026-10-21.md
- Go reference: regexp/backtrack.go startCond (go1.26.1)
- Test: internal/stdtest/testdata/corpus/regexp (go, arm64, LLVM)

A pattern starting with `^` (without the multiline flag) is tried at position 0
only. Anchored short-text matching is 1.35 times fewer instructions.

## 2026-10-21: perf: literal-prefix scan and the bit-state backtracker

- Kind: perf
- Commit: (filled in after the push)
- Found by: bench/results/regexp-time-csv-vs-go-2026-10-21.md
- Go reference: regexp/backtrack.go, Prog.Prefix (go1.26.1)
- Test: internal/stdtest/testdata/corpus/regexp (go, arm64, LLVM)

Texts shorter than 262,144 divided by the program's length (programs up to 500
instructions) are searched by backtracking with a visited bit per state, as
Go does; a literal prefix of the pattern is searched for when no thread is
alive. `is_match` on short strings is 2.1 times faster than before the
afternoon's work in total (29,759 M to 8,186 M instructions; Go 3,943 M).
Results are the same on the corpus.

## 2026-10-21: test: a Go-generated corpus for regexp

- Kind: test
- Commit: (filled in after the push)
- Found by: the lead's plan for the regexp fast paths (Go's order, each proved by a corpus)
- Go reference: regexp (go1.26.1)
- Test: internal/stdtest/testdata/corpus/regexp (go, arm64, LLVM); scripts/stdcorpus regexp

260 patterns (curated: anchors, classes, repeats, lazy forms, alternation,
groups, flags, folding, empty matches, bad syntax; and seeded random ones)
against 40 texts: whether the pattern compiles, find_index, find_all,
captures, replace_all, replace_all_literal, and split, equal to Go's. It
passes on the current engine on all three backends.

## 2026-10-21: perf: the Pike VM queues are flat and preallocated

- Kind: perf
- Commit: (filled in after the push)
- Found by: XO_PROF=cpu (bench/results/regexp-time-csv-vs-go-2026-10-21.md)
- Go reference: regexp/exec.go queue (sparse set with a dense entry array)
- Test: std module tests native/regexp on arm64 and LLVM

Per match the queues and thread captures are allocated once, not per input
position. match 1.7 times faster, find_all and replace_all 1.9, captures 1.7
(instructions); results are the same.

## 2026-10-21: port: every Unicode class natively

- Kind: port
- Commit: 9212b69b
- Found by: the std coverage table (decision 0276)
- Go reference: regexp/syntax unicodeTable, unicode.Categories, Scripts, FoldCategory, FoldScript, CategoryAliases (go1.26.1, Unicode 15.0.0)
- Test: std/native/regexp_test.xo (1,092 cases: `\p{NAME}`, `\P{NAME}`, and `(?i)[\p{NAME}]` for every category, script, and alias, on the runes at the edges of the class's ranges, against Go's ReplaceAllString), TestUnicodeTables

Native builds had only `\pL`, `\p{Nd}`, `\p{Any}`, and `\p{ASCII}`; any
other class Go knows (`\p{Greek}`, `\pN`, `\p{Lu}`) was an error. They now
have all of them, generated from Go's tables (std/native/llrt/unicode_classes.xo,
about 64 KB more in a program that uses regexp). As in Go, a script
name with an underscore (`\p{Old_Italic}`) is an invalid class: Go's
lookup canonicalizes it to `Olditalic`, which no table has.

## 2026-10-21: test: Go's RE2 search tests

- Kind: test
- Commit: b2ef88de
- Found by: differential test against Go
- Go reference: regexp/testdata/re2-search.txt, regexp Compile, FindStringSubmatchIndex (go1.26.1)
- Test: std/native/regexp_test.xo (1,486 pattern and text pairs: compile errors, the match, every capture)

No difference found: the native engine compiles, rejects, and matches as
Go's regexp on every pair, except patterns with a Unicode class native
builds did not have yet (`\p{Greek}`, decision 0172), which the test
counted apart until the entry above.
