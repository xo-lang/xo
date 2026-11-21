# The std record was written before a check pass had filled the types

Status: Observed (2026-11-20, tiers 79 and 918 on linux/amd64, task #566)

Symptom: golden time_hostile_date failed in the full linux/amd64 run
and passed alone: its server accepted hostile dates ("200 Thursday
9223372036854775807-12-31") and answered "expected object" to
"2026-11-14". XO_FE_CACHE=off or XO_STD_EXPORT=0 gave a good binary;
XO_STD_EXPORT=verify reported no difference. On the Mac it needed std's
variants in the cache (`xo dev stdcache`), then any two builds.

Cause: a std type's own JSON form (`derive Json(via:, to:, from:)`,
decision 0478) is four fields of its object (json_via, json_to,
json_from, json_from_err) that check_std_json_via sets from the
declaration's tree, after the declarations are resolved. The std record
(decision 0388 stage 1a-ii, the default of a separately compiled build)
was encoded before that pass ran, so it held -1 for all four, and a
module made from the record has no tree unless one of its bodies is
built: the pass then found no declarations and the importer derived a
plain codec. Whether the tree was there depended on what the front end
cache already replayed, hence "passes alone". The record's verify mode
compared two records made the same way.

Reach: the std types got their forms on 2026-11-16, after 0.51.1, the
last published release, so no user had it. The 0.53.0 and 0.54.0 seeds
have it: each, with its own std and a warm record, prints "err at $.d:
expected object" from the second build of internal/arm64's
testdata/stdrecord_via (2026-11-20).

Fix: the pass runs for every module of the record before the record's
block ends are taken and it is encoded (compiler/check dump.xo);
EXPORT_VERSION v16. The types the pass makes must be inside the block:
run after the ends were taken, the record "did not relocate" and every
build declared std from source (right output, the record unused).
Test: internal/arm64 TestStdRecordJsonVia (all six std types, a build
from the record against a whole-program build).

Avoid: a field of a declaration that a later pass fills is not in a
record or cache entry written before that pass. When adding such a
field (export.xo encodes every Obj field), find where it is set and
check it is before std_export; a verify mode that rebuilds the same way
cannot see it, only a comparison with a build that does not use the
record.
