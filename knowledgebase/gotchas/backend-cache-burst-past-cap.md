# The back-end cache grew past its cap during a burst of builds

Status: Observed (2026-10-16), fixed

## Symptom

`<user cache>/xo/backend` went from about 2 GiB to 19 GiB in 15 minutes
(fn 9.6G, obj 4.5G, rc 5.4G) against its 8 GiB cap, and the Mac fell
below 40 GB free twice. `prune-stamp` showed that a prune had run
(06:28) and removed nothing.

## Cause

The cap applies to the total of fn/, obj/, and rc/, as intended. But
prune.run (decision 0223) never removes an entry used within KEEP, two
hours, and runs at most hourly. At 06:51, 556,157 of the 659,429 entries
were under two hours old, so all the bytes a morning of parallel
builds stored were out of the cap's reach.

## Fix

bcache.xo prunes with keep 2 minutes (a hit refreshes an entry's time
once a minute) and with `Policy.burst` an eighth of the cap: every store
adds its bytes to `DIR/prune-stored` (prune.note_stored), and a prune
runs before the hour once they reach the burst (decision 0223
addendum). selfhost/prune test "a burst of stores prunes before the
interval and keeps the cache near its cap" stores 50 bytes every 10
seconds for an hour against a 1,000 byte cap: over 100 prunes, never
above 1,125 bytes (one prune and 18,000 bytes without burst). A cache
with entries used for long (Go's GOCACHE, the build directories) keeps
the two hours.
