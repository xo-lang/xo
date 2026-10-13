# A struct of many fields passed by value costs a copy of every word per call

Status: Observed (2026-10-13, decision 0193 step 5)

Symptom: the first tz.xo kept a zone's parsed rules in the native
time.Zone as `rules: Rules{id: Int, d: Option[Data]}` with Data a struct
of seven lists, a nested struct, and six scalars. time.format in a zone
then retired 1.031x the C path's instructions on arm64, and
time.zone(name) 1.744x (bench/perf/text zone_fmt and zone_load, less
setup), though the lookups themselves were faster.

Cause: the native backends pass a struct by value as its flattened
words: the Zone was 28 words, spilled to the stack and copied at every
call that takes a Zone (time.xo passes it through several helpers per
format), with a reference count operation for each list in it. The load
also went through rt.Any (rt.any_get pops from the Any's shared list of
one, which copies the list: an allocation per load).

Fix: a parsed zone is one List[Int] (header, transitions, zones, and
the abbreviations packed eight bytes a word), so Rules is two words and
the C table holds the list itself, marked shared (rttime.tz_find,
tz_keep), with no Any. zone_fmt went to 0.987x and zone_load to 1.001x.
Keep runtime values that travel in other values to a word or two: one
list or handle, not a struct of lists.
