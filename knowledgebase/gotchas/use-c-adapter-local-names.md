# `use c`: an adapter local named like a C parameter ("redefinition of 'n'")

Status: Observed (2026-11-11, the scarlet session); fixed 2026-11-11

Symptom: `xo check` passed a `use c` binding, but `xo run` failed at the
C compile of a file in the build cache: `xo_c_clash_....c: error:
redefinition of 'n'`. The header was `struct buf { const void *p; long
long n; }; struct buf get(long long n);`, a Bytes result (core 4.5).

Cause: the binder's C adapters (compiler/cli/bind gen.xo adapter_file)
named their parameters after the C parameters and declared locals with
short names: `r` for every result, `c` for a Str or Bytes result, `v` and
`n` for a Bytes result. A C parameter with one of those names (n, the
length, is common) was declared twice; one named like its own function
(`long long get(long long get)`), or like a type a cast in the adapter
names, would have shadowed it.

Fix: an adapter names parameter N `xo_aN` (its parts `xo_aN_p`,
`xo_aN_n`, `xo_aN_fM`) and its locals `xo_r`, `xo_c`, `xo_v`, `xo_n`, as
it named `xo_len`: no C declaration the binder accepts can reach them.
A failed adapter compile adds a note naming the header and the C
declaration of the function holding the first error (compiler/xo
cffi.xo adapter_note), since the file it points to is generated. Golden
use_c_names: one function per adapter shape, C parameters named r, c, v,
n, xo_len and like their function.

Avoid: generated C that pastes user names next to its own needs a
namespace for its own (an `xo_` prefix), not short names that look
unlikely.
