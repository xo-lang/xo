# 0152: The self hosted checker binds `use c` from the binding cache

Status: Accepted (2026-10-11). Implemented in `selfhost/check/cimport.xo`
(`load_c_import`, `c_skipped`, `c_module_name`), the C fields of
`Module` (objects.xo), `Checker.cbind_dir`, the handle and real-time
rules (infer.xo `is_handle`, realtime.xo `rt_call`), and the SSA
builder's `c_call` (selfhost/ssa json.xo, build_cffi.go). Tests:
TestSelfhostSSA, TestSelfhostChecker on examples/realtime-control and
internal/selfhost/testdata/ssa/stage6c, stage6c_skipped. Part of SSA
stage 6 (decision 0150).

## Context

`use c "<header>"` (decision 0116) is bound by internal/cbind: clang
dumps the header's AST as JSON, the binder maps each function's C
signature to Xo or skips it, and writes two files to the cache
(`<module root>/.xo/cbind`, or `$XO_CBIND_DIR`): an Xo declaration file
(`c_<name>_<hash>.xo`, bodiless like std/*.xo) and a C adapter. The
checker loads the declaration file as a std like module; the SSA
builder calls `rt.<prefix><name>`. Until stage 6 the self hosted checker
reported every run with `use c` (and `use go`) as unsupported, so no
C interop was compared.

Porting the binder means running clang from the Xo checker, parsing its
JSON AST (megabytes for a system header), and mapping C types: a large
piece that the SSA comparison does not need, since the declaration file
is the binder's whole output that the checker reads.

## Decision

- The Xo checker computes the binding exactly as cbind's `locate` does
  (the header's absolute path when it is next to the module, else the
  header as written; link files made absolute, library names kept;
  the effects, `ffi` when the line has no `uses`; the key
  `c:<header>|<links>|<effects>`; the prefix
  `c_<module name>_<first 8 hex digits of sha256(key)>_`) and reads
  `<cache>/<prefix>.xo`. It loads that file as the Go checker does: a
  std like module under the key, in signature mode, with the C fields
  (prefix, header, declaration path, and the skipped names from its
  `// skip: name: why` lines). The checks of the Go checker follow:
  XO1201 on the Go backend, an unknown effect in `uses` (XO1104), a
  missing `link` file (XO1104), an import name that is not valid
  (XO0602), a duplicate name (XO0603), a call of a skipped name
  (XO1105); opaque C structs are handles; calls of C functions are
  allowed in real-time code.
- A declaration file missing from the cache makes the run unsupported
  (`== unsupported use c without the binding of <header> ...`): the
  comparison tests run the Go checker first, which writes it; `xo bind
  --c` writes it too.
- The SSA builder ports build_cffi.go: a call of a bound function is a
  call of `rt.<prefix><name>` with a struct argument split into its
  fields, a struct result read back through `<sym>_r<k>`, and a handle
  result `None` for a NULL pointer.
- The clang step (internal/cbind) stays in Go until the self hosted
  compiler needs to bind headers on its own (the switch-over of
  decision 0150): then it is ported, or the compiler runs `xo bind`.

## Consequences

- Every repository run with `use c` (examples/realtime-control and the
  stage 6 programs) is compared: the checker's output and the SSA build
  stage are identical on the three backends.
- The port found a binder bug: `ffi` and `uses none` lines of one header
  shared a binding in a process (gotchas/cbind-memo-uses-none.md),
  fixed.
- Header names are made module names for ASCII names (cbind's `snake`
  works on Unicode letters); a header with other letters in its name
  would need the Unicode rules.
- `use go` runs stay unsupported: they never build natively (decision
  0116), so the SSA builder has nothing to compare.

## Alternatives

- **Port internal/cbind now**: clang's JSON AST through the Xo JSON
  decoder, type mapping, the adapter generator. More code than the
  checker and builder changes together, and the comparison would not
  change (the declaration file is the same).
- **Find the declaration file by its `// c-header:` line**: no hash
  needed, but two bindings of one header with different links or
  effects carry the same line.
- **Keep `use c` unsupported**: C interop would be the one part of
  stage 6 never compared.
