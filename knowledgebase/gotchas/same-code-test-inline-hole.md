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
adrp pages and adr offsets normalized, and both variants were built with
`XO_PASSES=-inline`. The hole: the promise "debug information changes no
code you debug" was not tested for inlining. The amd64 test kept inlining on
(it passed).

Fixed 2026-10-25, in the inliner, for the build that has the readers only.
The build still folds `dwline.built()` and prunes its arm at SSA time, so a
build without debug information builds exactly the code it built before: a
new `Module.reader_bodies` (the driver's `debug_info` expression) turns every
rule below off there. A build with debug information, where the readers are
the full bodies, makes the same inlining decisions as one without:

- The build marks a function that folded `dwline.built()` (`Func.reader`, a
  flag bit in the rc cache entry, and read into the lazy shell, since a shell
  has no body to fold again).
- `arm64/inline.xo` computes a release view of the module: the functions
  reachable from `main`, the exports and the entry with the readers as
  leaves. Only the readers' arms reach the rest, so they are what a build
  without debug information does not have; the inliner counts no call site
  in one, costs it one instruction and never inlines it.
- A reader itself costs one instruction and is never inlined, so a caller's
  cost does not depend on the reader's body (a stub in one build, the reader
  in the other), and a helper called from `main` and from a reader counts one
  site, as it does without debug information (debug_interp's main,
  show.append_rune_quote).
- The release view reads the inline summaries' name sets, not
  `m.block_names`: that one is keyed by a facts index that is stale for the
  lazy shells, and hid live functions (fault_unwind's release build grew by
  18 KB).

Measured: 10 goldens built with `--release --debug-info=off` by an xo at
origin/main and by this one are byte-identical files (arith, json_text,
strings, closures, fault_unwind, method_values, maps, big_struct,
float_libm, debug_interp). xo itself built `--release` from one source by
both has the same 60,900,220 byte `__text`, byte for byte (a control build
by the same compiler differs only in the UUID, the signature and 126 bytes
of build noise in `__const`), and the debug build of xo, which is how
xobuild builds it, is 12,152 bytes of `__text` smaller (61,008,016 to
60,995,864) because it now makes the release build's decisions.
TestDebugInfoSameCode now builds both variants with inlining on, over every
golden, and passes, as does TestAmd64DebugInfoSameCode.

The reader's own calls stay calls in a build with debug information, which
costs nothing at run time (they run only on a traceback).
