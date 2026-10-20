# A replayed closure kept the name it was recorded under

Status: Observed (2026-10-20, before the closure relocation landed)

Symptom: with closures cached (decision 0271 step 1b), building several
programs in one cache failed with "arm64 backend: closure of unknown
function `http.call_raw.fn10`" (http_tls, tls_client in
TestFrontCacheAcrossPrograms). Each program alone built.

Cause: a closure is named `${function}.fn${n}` with n the builder's
closure count, so a std body's entry recorded in one program names its
closures by that program's count. The commit renamed them to this
build's numbers, but replaying the closure's own entry assigned the
decoded function, whose name was the recorded one, over the function the
commit had declared. The same program built twice numbers closures the
same way, so the first test did not see it.

Fix: fe_replay keeps the name this build gave the function it commits
into. TestFrontCacheClosures now renumbers (a function made first gains
a closure) and compares with the cache-off build.
