# std/path changes

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
