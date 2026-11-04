# std/json TokenReader and JsonWriter against encoding/json (2026-11-04)

Twin bench/perf/jsontoken (checksums equal, `go run ./scripts/twincheck`),
linux/arm64 host 1, 7 rounds (n=6), times are the harness's (Go first), ratio is Xo over Go.

| workload | Go | Xo | Xo / Go | range |
|---|---|---|---|---|
| none (setup) | 0.647 | 0.352 | 0.544 | 0.413-0.678 |
| read: 40 records, 8,000 times, json.tokens against Decoder.Token (UseNumber) | 882.9 | 205.0 | 0.232 | 0.227-0.249 |
| write: the same listing, JsonWriter calls against json.Marshal of a struct slice | 63.55 | 144.3 | 2.271 | 2.175-2.322 |

The reader is 4.3 times faster than Decoder.Token; the writer is 2.27 times
slower than Marshal.

Open: the writer. Go's Marshal is a compiled struct encoder with no
per call state; the writer pays a BytesBuilder write and a stack update per
call and builds each string through append_body. Next experiment: write
string and key bodies straight into the BytesBuilder (no intermediate Str)
and keep the container state in two Ints.
