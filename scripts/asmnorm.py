#!/usr/bin/env python3
"""scripts/asmnorm.py FILE: an xo --asm file without source columns: drops the
DWARF sections (line program and inlined calls carry columns) and the column of every
"PATH.xo:LINE:COL" string, so two builds whose sources differ only in
columns compare equal. Prints to stdout."""
import re, sys
skip = False
col = re.compile(r'(\.xo:\d+):\d+')
for l in open(sys.argv[1]):
    s = l.strip()
    if s.startswith('.section') or s in ('.text', '.data'):
        skip = '__DWARF' in s or '.debug_' in s
    if skip:
        continue
    sys.stdout.write(col.sub(r'\1', l))
