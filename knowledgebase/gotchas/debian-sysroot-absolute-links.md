# A Debian sysroot unpacked from .debs has absolute symlinks

Status: Observed (2026-10-15, xo-fa, the linux/amd64 cross build of
decision 0221 stage 2).

## Symptom

`xo build --backend=llvm --target=linux/amd64` with XO_LINUXSYSROOT set
compiles, then lld fails:

    ld.lld: error: .../root/usr/lib/x86_64-linux-gnu/libc.so:5: cannot find
    /lib64/ld-linux-x86-64.so.2 inside .../root

## Cause

Debian's packages ship absolute links (`/lib64/ld-linux-x86-64.so.2 ->
/lib/x86_64-linux-gnu/ld-2.31.so`, the `usr/lib/x86_64-linux-gnu/*.so`
development links to `/lib/...`). Unpacked into a directory they point
at the Mac's own root, where nothing is, and lld follows them before
applying the sysroot.

## Fix

Make them relative after unpacking (knowledgebase/infrastructure/
toolchain.md, "x86_64 cross builds on the Mac"):

    ln -sfn ../lib/x86_64-linux-gnu/ld-2.31.so $R/lib64/ld-linux-x86-64.so.2
    find $R/usr/lib/x86_64-linux-gnu -maxdepth 1 -type l -lname '/lib/*' \
      | while read l; do ln -sfn ../../..$(readlink $l) $l; done

`find $R -type l -lname '/*'` then lists none.
