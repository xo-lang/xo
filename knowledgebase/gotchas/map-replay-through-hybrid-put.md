# A Map replay through the hybrid's put forks again

Status: Observed (2026-10-22, Xo Publish, decision 0291 branching prototype)

## Symptom

With XO_MAP_LOG=branch, a program that wrote a copy of a 3,000-entry Map
3,000 times never finished (internal/llvm/testdata/golden/map_branch,
"allover"). lldb showed xo.maph.goback calling xo.maph.slowput again from
its own replay loop.

## Cause

Going back from a fork to the lists (maph_goback) rebuilds a lists value
from the fork's oldest level, whose lists are shared, then replays the
fork's puts. The replay called `xo.map.put`, the hybrid's put. That put
saw lists of 1024 or more entries held twice, so it forked them again
instead of copying. The result was a log value, so every later put of
the program entered go-back again, with a longer replay each time
(quadratic).

## Fix

A replay inside the hybrid uses the lists' put alone
(maphyb.xo maph_listput, put_body with no hybrid prologue). Its first
put copies the shared lists on write, and the rest write in place. Any
code that rebuilds a hybrid value from shared lists must do the same, or
it re-enters the path it is leaving.
