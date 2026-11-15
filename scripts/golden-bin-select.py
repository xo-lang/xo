#!/usr/bin/env python3
"""golden-bin-select.py: the goldens whose built binaries differ (decision 0657).

  golden-bin-select.py BASE_OUT HEAD_OUT SRC NAMES_FILE

BASE_OUT/NAME/prog and HEAD_OUT/NAME/prog are one golden's binary built by
the merge base's xo and by the branch's (scripts/gate.sh bin_select: each
with its own std copied to one path, the branch's golden sources at one
path, every cache off). Prints, one a line, the goldens to run:

  - those whose binaries differ once the build-info id is masked (the 24
    hex after `{"id":"` and after `, id `: compiler/xo/binfo.xo hashes every
    source file's digest and the runtime's files into it, so any std edit
    changes it in every binary);
  - those either side failed to build (a compile-error golden's result is
    the compiler's output, not a binary; a golden new on the branch);
  - those whose main.xo uses C (`use c`): their C step records a path
    that differs from build to build (all 8 C-linked goldens differed in
    .debug_line and .rodata at equal sizes with no code change, 2026-11-15:
    ~/xo-lead/notes/lang2/artifact-keyed-tests.md).

The last line on standard error counts each kind.
"""
import os
import re
import sys

ID = re.compile(rb'(\{"id":"|, id )[0-9a-f]{24}')


def load(p):
    with open(p, 'rb') as f:
        return ID.sub(lambda m: m.group(1) + b'0' * 24, f.read())


def uses_c(src, g):
    try:
        with open(os.path.join(src, g, 'main.xo'), encoding='utf-8', errors='replace') as f:
            return re.search(r'^\s*use\s+c\b', f.read(), re.M) is not None
    except OSError:
        return False


def main(argv):
    if len(argv) != 5:
        sys.stderr.write(__doc__)
        return 2
    a, b, src, names = argv[1:]
    with open(names) as f:
        gs = [l.strip() for l in f if l.strip()]
    out, n = [], {'differ': 0, 'failed': 0, 'c': 0, 'same': 0}
    for g in gs:
        pa, pb = os.path.join(a, g, 'prog'), os.path.join(b, g, 'prog')
        if uses_c(src, g):
            n['c'] += 1
            out.append(g)
        elif not (os.path.isfile(pa) and os.path.isfile(pb)):
            n['failed'] += 1
            out.append(g)
        elif load(pa) != load(pb):
            n['differ'] += 1
            out.append(g)
        else:
            n['same'] += 1
    for g in out:
        print(g)
    sys.stderr.write('golden-bin-select: %d of %d: %d differ, %d did not build on a side, %d use C; %d identical\n' % (
        len(out), len(gs), n['differ'], n['failed'], n['c'], n['same']))
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
