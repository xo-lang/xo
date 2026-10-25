# TestDebugInfoSameCode builds without inlining: a hole

Observed 2026-10-25. Symptom: after 24824095f (`dwline.built()` folds to
the build's debug information), `internal/arm64` TestDebugInfoSameCode failed
for every golden: a build with debug information and one without (`-s`,
`--debug-info=off`) differ by the runtime's DWARF readers (87 KB on the
generics golden), and by what the inliner does with the runtime functions
the readers call.

Cause: the arm64 inliner (compiler/arm64/inline.xo) decides by call-site
counts and callee costs ("few sites", "one site"). In the build with debug
information the DWARF readers are callers of runtime helpers (show.xo,
text.xo); in the build without them they are gone, so a helper that was a
call in one build is inlined into `main` in the other (debug_interp's main,
show.append_rune_quote). The code is the same; the inlining differs.

Fix, and the hole: the test now compares the program's own functions (not
the runtime's, which call the readers: `programFunc`), by body (merged
functions carry hash names), with addresses, branch targets, callee names,
adrp pages and adr offsets normalized, and builds both variants with
`XO_PASSES=-inline`. The hole: the promise "debug information changes no
code you debug" is not tested for inlining. The amd64 test keeps inlining on
(it passes). Remove the hole by folding `dwline.built()` after the inliner
(a late pass that replaces the call with the constant, removes the dead
branch and lets the hollow pass, decision 0371, empty the readers), so both
builds inline from one call graph; queued with Lang 3.
