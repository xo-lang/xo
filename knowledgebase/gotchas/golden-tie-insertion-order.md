# arena_escape `longest` depends on request order

Status: Observed (2026-10-08, two `arena_escape` failures in golden runs
after three concurrent golden programs were added; fixed the same day)

Symptom: `stats {... "longest":"item-110-..."}` instead of `item-147-...`.

Cause: the `GET /stats` handler keeps the first value with the greatest
length while iterating an insertion ordered Map filled by concurrent
requests; several values have the same length, so the winner depends on
which request inserted first.

Fix: the golden program breaks ties by value
(`v.len() > longest.len() || v.len() == longest.len() && v > longest`);
want.txt did not change (`item-147` is also the greatest of the ties).
Any golden output computed from a Map filled by concurrent tasks must not
depend on insertion order.
