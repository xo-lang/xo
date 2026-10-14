# Python text mode reads of want.txt turn `\r` into `\n`

Status: Observed (2026-10-13), fixed in scripts/selfhost-goldens.py (the scripts were deleted on 2026-10-14 for `xo dev goldens`, decision 0209, which reads want files as bytes),
scripts/selfhost-gogoldens.py, and scripts/elf-goldens.py.

Symptom: scripts/selfhost-goldens.py reported 172 of 173 programs
passing on xo1 (both `--backend=arm64` and `--backend=llvm`), the one
failure being internal/llvm/testdata/golden/show_table, while the Go
`xo` and TestGoldenRun passed it. It looked like an xo1 code generation
bug in the Rune or UInt64 text that show_table covers (decision 0193
step 5).

Cause: the harness, not the compiler. show_table prints the rune `'\r'`
raw (`[\r] '\r' 13`; Rune Debug escapes only `\n`, `\t`, `\\`, and
`\'`), so its want.txt holds a carriage return byte. The script read
want.txt with `open(path).read()`, Python's text mode with universal
newlines, which turns that `\r` into `\n`, while the program's output
(decoded from bytes) kept the `\r`. The xo1 binaries' output was byte
for byte equal to want.txt on both backends (checked with the xo1 built
from 795b5754).

Fix: read want.txt with `open(path, encoding='utf-8', newline='')`
(or `'rb'`), as the Go tests read it byte for byte. With the fix,
scripts/selfhost-goldens.py on the xo1 built from 795b5754 (macOS
arm64, 2026-10-13): `--backend=arm64` 173 programs: 173 pass, 0 fail;
`--backend=llvm` 173 programs: 173 pass, 0 fail (before: 172 of 173).
internal/diag's TestScriptsReadWantBytes fails if a script under
scripts/ reads want.txt in text mode again. Before suspecting the
compiler when only a script disagrees with the Go test, diff the raw
bytes (`cmp want-stdout out`) first.
