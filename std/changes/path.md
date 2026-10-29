# std/path changes

## 2026-10-29: perf: clean writes a short kept element a byte at a time

- Kind: perf
- Commit: this commit
- Found by: Lang 3 (the std-vs-Go survey's path parts row, 1.24 times Go)
- Go reference: path/path.go Clean (lazybuf writes the result a byte at a time)
- Test: internal/stdtest/testdata/corpus/path on the Go, arm64 and LLVM backends (unchanged)
- Timing: bench/perf/path parts, quiet `xo bench --ab`, 15 rounds, A the parent's std, B this one: wall 0.944 (0.926 to 0.977), instructions 0.946; B against the Go twin 1.09 times its wall (A 1.14); a 8M-pass copy of parts: wall 0.948, instructions 0.928

`clean` copied each kept element of an unclean path with a slice and a `write` (a range check, a count and a drop, and the builder's call each); elements of up to 24 bytes are now written with the builder's inline `write_byte`, and longer ones still with one `write`. Output is unchanged.

## 2026-10-27: perf: clean keeps up to 8 elements in locals, with no list

- Kind: perf
- Commit: this commit
- Found by: path parts at 1.265x Go (measured); a dir-only twin: unclean prefixes at 1.70x Go's cycles (1.28x its instructions), the list of element offsets 21% of an unclean dir (allocation and free); a stub without clean ran parts in 0.508 of the time
- Go reference: path.Clean (lazybuf: one buffer, no list)
- Test: internal/stdtest/testdata/corpus/path (go, arm64, LLVM), the generator adding deep paths for the over-8 form (9 to 12 kept elements, `..` late and early, leading `..`); twincheck's checksums agree on clean, join, parts, match
- Timing: bench/perf/path, quiet --ab, 15 rounds, the parent's std as A: parts 0.927 (0.904 to 0.971), instructions 0.947, cycles 0.918; Xo / Go about 1.17 (estimate: 1.265 x 0.927), from 1.265

clean keeps the kept elements, each its start and end in one word, in
eight locals, and counts their bytes as they come and go, so a path of up
to 8 kept elements at a time allocates only its result; a deeper one is
cleaned by clean_list, the list form before. Three other shapes measured
no gain and were left out: the list packed one word an element (0.988),
no list at all with a backward pass marking kept elements in a bit mask
(0.982; instructions 1.042), and a StrBuilder result, which skips the
UTF-8 check of BytesBuilder's freeze().to_str() (instructions 1.066).

## 2026-10-27: perf: dir cleans its prefix without the trailing slash

- Kind: perf
- Commit: this commit
- Found by: path's parts profile (dir 70%: clean of a prefix ending in `/`, which is never clean, so every dir built its result and validated it as UTF-8)
- Go reference: path.Dir (Clean(path[:lastSlash+1]); Clean drops a trailing `/`)
- Test: internal/stdtest/testdata/corpus/path (go, arm64, LLVM), with dir's edges added to the generator (`a//b`, `//a`, `///a`, `a//`, `/a/`, `a/b//`, `/../a`, `..//a` and more); twincheck's checksums agree on clean, join, parts, match
- Timing: bench/perf/path, quiet --ab, 15 rounds: parts 0.865 of the parent's wall (0.857 to 0.889), instructions 0.844; Xo / Go about 1.38 (estimate: 1.596 x 0.865), from 1.596

dir passed `p[0..i+1]` to clean, a prefix that always ends in `/`, so
is_clean never held and every call built the cleaned text. Clean drops a
trailing `/` of anything longer than `/`, so dir now passes `p[0..i]`
(and `p[0..1]` when the only `/` leads): the same result, and an already
clean directory comes back as a view with nothing allocated. parts stays
over the 1.2x bar: what is left is the other half of dir's prefixes (those
with `.`, `..` or repeated slashes, which clean rebuilds) and the checked
Str slices of split, base and ext.

## 2026-10-27: perf: clean finds the elements once and returns a clean path as it is

- Kind: perf
- Commit: this commit
- Found by: path's parts profile (clean 54%, its byte at a time List[UInt8] buffer 22%)
- Go reference: path.Clean (its result; Go's lazybuf replaced)
- Test: internal/stdtest/testdata/corpus/path (go, arm64, LLVM); twincheck bench/perf/path agrees on clean, join, parts, match
- Timing: bench/perf/path, quiet --ab, 15 rounds: clean 0.850 of the parent's wall (instructions 0.649), 1.016 of Go's (was 1.15); join 0.981 of the parent (instructions 0.703), 1.11 of Go's; parts 1.54 of Go's, unchanged; match 0.975

A path with no empty, `.` or `..` element and no trailing `/` is returned
as it is with nothing allocated (dir's prefixes mostly are); any other is
scanned once into its kept elements, a list of start and end offsets made
at its final size (a list grown by pushes measured slower than the byte
buffer), and written an element at a time. An output as long as the input
is the input. parts stays at 1.54x Go's time: what is left is outside
clean (the slice checks of split, base and ext).

## 2026-10-26: perf: is_match walks the pattern and the name by offset; base cuts once

- Kind: perf
- Commit: this commit
- Found by: Lang 3's twin (bench/perf/path, std-go-timing-audit: match 1.70x, parts 1.54x Go's wall)
- Go reference: path/match.go (Match, scanChunk, matchChunk, getEsc) and path.Base
- Test: internal/stdtest/testdata/corpus/path (go, arm64, LLVM); twincheck bench/perf/path agrees on clean, join, parts, match
- Timing: bench/perf/path, quiet --ab, 15 rounds: match against the parent 0.623 (cycles 0.543), against Go's 0.998; parts 1.02 against the parent, 1.53 against Go's (unchanged within noise); clean 1.15, join 1.17

Go reslices the pattern and the name at every step; each Xo slice is a
view (decision 0330) but pays a range check, a count and a drop, so the
port now carries offsets (get_esc, match_chunk and scan_chunk take
and return positions) and is_match makes no copy at all. base finds its
end and start by index and cuts once, where it cut a copy per trailing
slash. parts stays over Go: its profile is clean (dir calls it), whose
copy buffer is a List[UInt8] pushed a byte at a time, and the slice checks.

## 2026-10-23: feature: split, is_abs and is_match, and a generated corpus

- Kind: feature
- Commit: this commit
- Found by: lead request; Go's path has Split, IsAbs and Match, and the module had neither the functions nor a corpus whose generator is in the repository
- Go reference: path/path.go Split and IsAbs, path/match.go Match, ErrBadPattern, scanChunk, matchChunk and getEsc (go1.26.1)
- Test: internal/stdtest/testdata/corpus/path, generated by scripts/stdcorpus/path.go (every input of the replaced std/native/path_test.xo, Go's cleantests, splittests, jointests, exttests, basetests, dirtests, isAbsTests and matchTests, and seeded random paths and patterns; TestStdCorpora arm64 and llvm, 6 tests each); internal/llvm/testdata/golden/std_path gains split, is_abs and is_match (TestGolden and TestGoldenGoParity std_path pass); std/native/path_test.xo keeps a sample for the arm64 front end cache tests; the fixed point's s2 and s3 assembly and binaries are identical

`split` is Go's, the directory up to and including the last `/` and the
file after it. `is_abs` is `p` starting with `/`. `is_match` is Go's Match,
a shell pattern match over the whole name (`*`, `?`, byte classes and `\`
escapes), ported from path/match.go; it is named `is_match` because `match`
is a keyword, and `PathErr`'s display is hand written Go in std/rt/path.go
as the Go backend's other module displays are. A malformed pattern is `PathErr`, which displays as Go's
"syntax error in pattern" with the pattern, and is reported even when the
name cannot match, as Go does.

The test moves: std/native/path_test.xo was generated by a Go program that
is not in the repository, so it could not be regenerated or checked. Its
3,549 cases (its Go table literals and 600 random paths, and 801 joins)
are kept as the corpus's inputs, so the corpus is a superset of it, and
`go run ./scripts/stdcorpus -check` now covers path.

## 2026-10-21: perf: clean's buffer is bytes, written by index

- Kind: perf
- Commit: f04d9d05
- Found by: Xo Publish's profile of the Go backend (go-backend-path-profile-2: the lazy buffer was a List[Int] grown from empty, 17%)
- Go reference: path.go lazybuf (go1.26.1)
- Test: std/native/path_test.xo; goldens std_path and std06_path

`clean`'s lazy buffer is a `List[UInt8]` made once at the input's length and
written by index, as Go's, and the bytes are compared and copied as bytes
(no widening to Int). 3,000,000 iterations of clean(join) + base + ext +
dir, median of 5 on a busy machine: the Go backend 0.97 s to 0.75 s, native
1.43 s to 1.03 s; the output is the same.

## 2026-10-21: behavior: the Go backend runs path.xo too

- Kind: behavior
- Commit: 09c320f9
- Found by: decision 0312 (std written once, the pilot)
- Go reference: path/path.go (go1.26.1), as ported in std/native/path.xo
- Test: internal/gen/testdata/golden/std06_path; internal/llvm/testdata/golden/std_path (Go parity)

The Go backend compiles std/native/path.xo instead of calling Go's path
package, so every backend runs one implementation. The results are the
same; on the Go backend a path call costs about 2.6 times Go's package.

## 2026-10-21: perf: clean is Go's algorithm natively

- Kind: perf
- Commit: 5cb4ffe1
- Found by: decision 0312's measurement, profiled by Xo Publish (clean was about 80% of a path loop, and its split-and-join algorithm, not the code generator, made most of the 4x against Go)
- Go reference: path/path.go Clean and lazybuf (go1.26.1)
- Test: std/native/path_test.xo (Go's results on path_test.go's strings and 600 random paths); internal/llvm/testdata/golden/std_path

Natively, `clean` is Go's one-pass scan into a lazy buffer. It returns
its input unchanged when nothing needs cleaning, where it used to split
the path into a list and join it again. `join` and `dir` use it, as in
Go. The results are the same.

## 2026-10-21: test: Go's path package on its test strings

- Kind: test
- Commit: 98e4292f
- Found by: the std coverage table (decision 0276: a shipped module without Go's tests)
- Go reference: path Clean, Join, Dir, Base, Ext and path/path_test.go (go1.26.1)
- Test: std/native/path_test.xo (3,549 cases: clean, dir, base, and ext of every string literal of path_test.go and 600 random paths, and 801 joins)

No difference found: the native port gives Go's result on every case.
The Go backend calls Go's path package itself.
