# A value redefined each loop iteration looked used after its consume

Status: Observed and fixed (Xo Lang 2, 2026-10-18).

## Symptom

`var c = match get(cur) { Some(x) => x  None => return }` in a loop shared
every field of the payload (a retain per heap field) and then dropped the
Option, which released them all again: 205 instructions an iteration
for a 7 field struct on arm64. In tlssrv's take_conn this was about 22k
instructions per HTTP/2 request (Xo Lang 3's callgrind).

## Cause

compiler/ssa borrow.xo used_after asks whether a projection's owner is
used after the projection is consumed, by walking the blocks reachable
from the consume. Around a loop the walk reached the owner's own block,
where the next iteration's call made a new Option, and its tag test
counted as a use of this one. So the payload became a borrowed
projection, which is shared when consumed, and the owner stayed alive to
be dropped later.

## Fix

used_after stops at the root's own block (home): a path through it makes
the value anew. The payload stays counted, and rc.xo's move (Func.moved:
a single field payload of an owned enum that dies at it takes the enum's
ownership) removes the share and the drop. The microbenchmark: match and
or_fault 204M to 86M per million (arm64), 122M to 34M (LLVM).
