# A Go twin whose cost grew with every request ever served

Status: Observed 2026-11-06 (Xo Lang 2, the 4000/s p50 item of decision 0520 phase 1).

## Symptom

`harness load` of stability/notes against its Go twin (bench/size/native/go/notes) at
saturation, 64 closed loop workers, with the map store experiment (30d947457): Xo 103,624
requests a second against Go's 9,016, and Go at 177 us of CPU per request against its 105 at
4000/s and 73 at 1000/s. A Go map server's CPU per request should not triple with load.

## Cause

The twin's `GET /notes` walked every id issued so far (`for id := 1; id <= s.nextID; id++`)
and looked each up in the map, holding the store's mutex. Notes expire and are deleted, so the
live set stays small, but the id counter grows with every create: each list cost O(creates
since start), and the mix lists on 10% of its requests. The faster a target serves, the more
ids it issues and the slower its lists become, and the mutex serializes the rest behind them.
Xo's handler walks the live notes only, so the two apps did different work and every Go row
of bench/results/stability-load-2026-11-05.md (above all its saturation row) measured the
twin's walk. The harness counted only 5xx answers, so nothing else flagged a difference.

## Fix

The twin walks the live notes and sorts them by id (the SQL store's ORDER BY id; output
unchanged). `harness load` also counts 4xx answers per target (`4xx N of M` in load.txt),
since the mix's reads and deletes follow a create's 201 and a target answering creates 4xx
would do far less work.

## How to avoid

When a twin's numbers move with load in a way the language does not explain (CPU per request
rising with the rate), read the twin's handlers for work that grows with history (counters,
logs, id ranges) before comparing. Compare each handler's complexity with the app's, not only
the output.
