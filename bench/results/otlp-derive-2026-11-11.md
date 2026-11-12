# std/otlp traces: derive Proto structs against the hand writer (2026-11-11)

Decision 0549 item 3: std/otlp may write its messages as derive Proto
structs only if its timing does not regress. bench/perf/otlp `traces`
encodes 200 spans (OpenTelemetry ids, a parent, kind, four attributes,
two events, an error status every ninth, two scopes) 4,000 times.

linux/arm64 offload host 1, `xo bench --ab --rounds=10`, both built by
one xo (main 3a1a5dc07's compiler) for linux/arm64 --release: A with
main's std (the builder writer, each nested message a Bytes of its own),
B with the trace messages as derive Proto structs marshaled once.

| metric | A | B | B / A (95%) |
|---|---|---|---|
| wall_ms | 472.6 | 410.9 | 0.869 (0.863 to 0.877), better |
| max_rss_kb | 1110 | 1110 | 1.0, within noise |

Both print `traces 912332731` (the same bytes). The generated marshal
sizes each message once and writes into one buffer, where the hand
writer froze a Bytes per KeyValue, AnyValue, Event and Span and copied
it into its parent.

## Metrics (2026-11-11): derive is slower, so metrics export keeps the writer

bench/perf/otlp `metrics` encodes a registry of a counter, a gauge, a
histogram (latency buckets) and a summary, 50 labelled series each,
4,000 times. B wrote the metrics messages (Metric with its data a oneof
enum, NumberDataPoint with as_double a oneof, Histogram and Summary
points) as derive Proto structs; its bytes equal the writer's on the
corpus and on a registry with summaries, function families, a zero gauge
and an empty bucket list (SHA-256, arm64 and llvm). linux/arm64 host 1,
`--ab --rounds=10`, A main 2d26a633c's std:

| workload | A wall | B wall | B / A (95%) |
|---|---|---|---|
| metrics | 205.4 ms | 247.4 ms | 1.205 (1.198 to 1.211), worse |
| traces (unchanged code, control) | 454.2 ms | 449.2 ms | 0.989 (0.984 to 0.994) |

`XO_PROF=cpu`: proto_marshal__MetricMsg is 67% of B, its time in
put_fixed64, put_varint and appends. Like the writer, the generated
marshal makes a Bytes for each nested message (`put_bytes(proto_marshal__X(v))`),
and B adds building the structs: a KeyValueMsg and an AnyValueMsg per
label per point, a List per point. Traces gained because its writer
copied more (a Bytes per KeyValue, AnyValue, Event and Span copied into
each parent).

Next experiment: the generated marshal writes a nested message into its
parent's builder with a reserved length prefix (protobuf-go computes
sizes first; a back-patched one byte prefix moved up when longer is the
cheaper form), so no message is a Bytes of its own; then this row again.

## Derive Proto with a size pass (2026-11-11): metrics move to derive

The generated marshal now does what protobuf-go does: `proto_size__T` sums
the message's size, then `proto_write__T` writes it into one BytesBuilder of
that capacity, each nested message, packed list and map entry in place after
its length (computed by the same size functions), where each was a Bytes of
its own copied into its parent. A is main 3b0fe3824 (old generator, metrics
by the writer, traces by derive); B is the new generator with metrics as the
derive structs of the second table. Same checksums; the otlp corpus and a
registry with summaries hash the same. linux/arm64 host 1, `--ab --rounds=10`:

| workload | A wall | B wall | B / A (95%) |
|---|---|---|---|
| otlp metrics (writer to derive) | 205.6 ms | 175.8 ms | 0.855 (0.851 to 0.863) |
| otlp traces (derive, generator only) | 450.8 ms | 349.8 ms | 0.776 (0.772 to 0.783) |
| protobuf marshal (generator only) | 24.35 ms | 19.43 ms | 0.798 (0.763 to 0.807) |
| protobuf unmarshal (unchanged code) | 22.42 ms | 22.91 ms | 1.022 (1.013 to 1.052) |

Unmarshal's generated code is unchanged; its 2% is the binary's layout (no
code on its path changed). The back-patched length prefix (reserve one byte,
move the body when the length passes 127) was not built: BytesBuilder has no
move or insert, and the size pass already beats the writer. Next experiment:
a size cache per nested message (protobuf-go caches sizes in the message;
here a nested message's size is computed once by its parent's size pass and
again before it is written, so deep trees pay depth times), measured on the
same rows: the next table.

## Nested sizes computed once (2026-11-11)

`XO_PROF=cpu` on the size pass build: proto_size__ functions were about a
third of traces, recomputed at every level. Now the size pass records each
nested message's size in a list in the order the writer meets them
(`proto_sizes__T(v, var sz)`), and the writer reads them with a cursor
(`proto_write__T(b, v, sz, var at)`); a map entry keeps a list of its own,
since the size pass walks the map unsorted and the writer in key order.
A is the size pass build (an unpublished build), B adds the cache. Same checksums;
the protobuf, protowkt, protojson and otlp corpora pass on go, arm64, llvm.
linux/arm64 host 1, `--ab --rounds=10`:

| workload | A wall | B wall | B / A (95%) |
|---|---|---|---|
| otlp metrics | 170.7 ms | 153.2 ms | 0.897 (0.883 to 0.908) |
| otlp traces | 348.2 ms | 297.9 ms | 0.856 (0.853 to 0.859) |
| protobuf marshal | 19.19 ms | 16.33 ms | 0.851 (0.836 to 0.876) |
| protobuf unmarshal (unchanged code) | 22.52 ms | 22.12 ms | 0.982 (0.969 to 1.001), within noise |

Against the hand writer of main 3b0fe3824 (third table's A), the two steps
together are about 0.855 x 0.897 = 0.77 for metrics and 0.776 x 0.856 = 0.66
for traces (products of separate runs, not one --ab).

## Varints and fixed width values as one word (2026-11-12)

std/native/protobuf.xo's `put_varint`, `put_fixed32` and `put_fixed64` append a value with one
`BytesBuilder.write_le` where they pushed a byte at a time. Found with XO_PROF=cpu on host 1 (bench/perf/protobuf
marshal, 100000 passes, 1593 samples): `put_varint` 43% cumulative, `obj_rs_push_byte` 34% and `obj.reserve` 11%;
the size pass (`proto_sizes__Doc`) was 12.7%, which caps what a back-patched length prefix could save.
linux/arm64 host 1, `ab-host.sh` (xo bench --ab, 7 rounds, stopped at 6), A main 591a43ea2, B 8beb0d6e8:

| workload | A wall | B wall | B / A (95%) |
|---|---|---|---|
| protobuf marshal | 17.64 ms | 15.09 ms | 0.856 (0.829 to 0.872) |
| otlp traces | 293.1 ms | 285.0 ms | 0.972 (0.966 to 0.974) |
| otlp metrics | 152.6 ms | 139.8 ms | 0.916 (0.91 to 0.93) |

The protobuf run's B was 9a1f7837b, the same code but for one shift that faulted on 8 byte varints with a high
byte of 128 or more (Xo's `<<` checks overflow; `wrapping_shl` fixes it, a path the bench never reaches).

## Each field's key written with its value (2026-11-12)

derive Proto's writers call `protobuf.put_key_varint`, `put_key_fixed32` and `put_key_bytes` (and a nested
message's `proto_kwrite__T`) with the field's key, so a one byte key and its value or length go out as one word,
where the tag was pushed alone. Found with XO_PROF=cpu at main 99c339dfc (host 1, protobuf marshal, 1501 samples):
`put_varint` 49% cumulative, `cabi.obj_rs_push_byte` 30% (the tags). linux/arm64 host 1, `ab-host.sh`
(xo bench --ab, 7 rounds, stopped at 6), A main 99c339dfc, B f530903ff:

| workload | A wall | B wall | B / A (95%) |
|---|---|---|---|
| protobuf marshal | 15.01 ms | 12.72 ms | 0.847 (0.819 to 0.865) |
| otlp traces | 284.9 ms | 262.8 ms | 0.923 (0.918 to 0.925) |
| otlp metrics | 144.1 ms | 139.7 ms | 0.969 (0.958 to 0.986) |

