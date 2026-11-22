# The test clang cache keyed a ThinLTO index by its name

Status: Observed (2026-11-21, Xo Lang 1, task #656).

Symptom: TestSepcompLLVM's json_text and http_tls failed to build with
`ld.lld: error: undefined hidden symbol: xo.dropslow.Str.llvm.6116809274563584145`
(and `xo.drop.Str`, `xo.dropslow.List[Int]`), referenced from the
program's own object, while the same programs built by hand with
`XO_SEPCOMP_LLVM=1` linked. It showed once the test built std's objects
in a cache of its own.

Cause: the tests' clang (internal/xotool clangcache.go) answers a
compile by a key over its arguments and the contents of its input
files. A ThinLTO backend compile (decision 0738: the program's module
against std's bitcode) is given `-fthinlto-index=FILE`, and the key
took that as an argument: the file's name, always
`prog.bc.thinlto.bc`. The index says what the module imports and which
locals are promoted, so the same module against another std has another
object; the cache returned one made against an earlier std, whose
promoted names the present std's objects do not define. The cache is
kept between runs, so the entry came from another commit's run.

Fix: clangKey hashes the index file's contents, and leaves a call whose
index cannot be read alone (TestClangKey).

How to avoid: a new clang option that names a file the output depends
on needs a case in clangKey; an option it does not know is keyed by
its text. A link error that names a symbol no present object could
have produced, in a test only, points at the cache: build by hand with
the real clang (`XO_CLANG=$XO_CLANG_CACHED`) before reading the
compiler. (Both causes were real here: std's linkage also had a bug,
found a day later in other programs: symptom 7 of
llvm-std-objects-roots-and-named-section.md.)
