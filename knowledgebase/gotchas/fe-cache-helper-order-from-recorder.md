# A front end cache entry replayed Map helpers in the recording build's order

Status: Observed (2026-11-20, tasks #476 and #471).

## Symptom

A build with the caches on differed from the build without them in the
order of two functions in the text, and nothing else: `xo.maph.oflog[T]`
before `xo.maph.slowput[T]`, where the build without the cache has slowput
first. Seen as:

- internal/arm64 TestModuleCheckAuto/programs/sql_conv, "the warm build
  with XO_CHK_CACHE=auto differs from the cache-off build", twice on the
  gate hosts (2026-11-19, 2026-11-20), and 1 cold build in 150 of that
  golden on the Mac (4 of 800 in two more runs, always the same wrong
  file);
- TestWarmBuildSequence, shuffled seed 20777 on host 3 (task #471);
- every time, with no worker involved, for a program edited and built
  again (TestWarmHelperOrder).

## Cause

A body that finds a Map helper already made records it "made whole" in its
entry with the helpers that helper's body calls, so a build where none is
made yet makes them where the body's lowering would (fecache.xo fe_entry).
The entry listed that group sorted by the recording builder's function
indices: the order the recording build happened to make them in. That is
the order a fresh lowering makes them in only when they were all made at
once. A remove on Map[K, V] makes oflog (through xo.maph.slowrem) without
slowput; a later put makes slowput, which calls oflog; a third body that
puts then finds slowput and records "oflog, slowput". Replayed where
neither is made, the entry made oflog first; the build without the cache
makes slowput and then, from its body, oflog.

A speculative front end worker (decision 0637 stage 3) lowers bodies ahead
of their turn in its own copy of the builder, in whatever order they reach
it, so its copy can hold the helpers in such an order when the serial
build never does: the entry it hands the main builder then replays wrong.
That made it one build in 150 or so; XO_FE_SPEC_WAIT=1 and
XO_FE_SPEC_VERIFY=1 both hide it (40 of 40 and 800 of 800 right).

## Fix

Each Map helper keeps the Map helpers its own lowering asked for, in the
order it asked (ssa Func.hreq, filled by Builder.thunk_func, which every
maker calls whether or not the helper is there). fe_entry lists a found
helper's group as the helper, then those requests, each followed by its
own: the order a build that lacks them makes them in. An entry carries
the list beside each function it makes whole (Func.hreq is not in the
function codec), so a helper a replay made has it too. Front end cache
version v20.

## How to find the next one

- Keep both binaries and diff `nm -n` name columns: a pure reordering
  shows as two lines swapped.
- A rare cold-build difference with speculation on: loop the cold build
  (150 to 800 runs, 6 at a time) and compare hashes; then record which
  body's replay made the swapped functions (`b.fe_stats["dbg ..."] = 1`
  lines print under XO_FE_STATS=1).
- An order kept in an entry must be one a fresh lowering reproduces, never
  one read off the recording builder's indices.
