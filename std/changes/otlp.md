# std/otlp changes

## 2026-11-17: fix: ExportErr's text on the Go backend for the size limits, partial success and gRPC statuses

- Kind: fix
- Commit: this push
- Found by: task #259 (main's standing TestStdAPICovered gaps: with_compression, with_encoding, with_max_request_size, with_max_response_size and grpc_exporter had no resolved call outside std's own tests; the golden written for them showed the Go backend's empty texts)
- Go reference: OpenTelemetry Go otlptracehttp's request and response size errors, grpc-go's status text (`rpc error: code = CODE desc = MSG`)
- Test: golden otlp_options (internal/llvm and internal/gen: JSON encoding, gzip, both size limits, the gRPC exporter giving up on a refused port), which failed on the Go backend before (it printed nothing for these errors)
- Shipped in: not yet released

On the Go backend `ExportErr.display()` gave "" for RequestTooLarge,
ResponseTooLarge, Partial and Grpc; it now gives the native backends'
texts.

## 2026-11-17: feature: log export (task #184)

- Kind: feature
- Commit: this commit
- Found by: task #184 (stdcov feature gaps: otlp had no log signal)
- Go reference: go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp and otlploggrpc v0.23.0, go.opentelemetry.io/contrib/bridges/otelslog (the slog to OpenTelemetry mapping)
- Test: std/native/otlp_test.xo (two oracle bodies from slog through otelslog and otlploghttp: every value kind, levels, a module's scope, nested and inlined groups; send_logs over OTLP/gRPC retried and with a partial success)
- Shipped in: not yet released

`encode_logs` writes an ExportLogsServiceRequest for std/log records and `Exporter.send_logs` sends it over OTLP/HTTP (`/v1/logs`) or OTLP/gRPC (LogsService/Export), with the exporter's retries, compression, size limits and partial success.

## 2026-11-17: feature: OTLP/gRPC export (task #184)

- Kind: feature
- Commit: this commit
- Found by: task #184 (stdcov gap otlptracehttp.behavior.grpc_transport)
- Go reference: go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.47.0 and otlpmetric/otlpmetricgrpc (client.go, internal/retry: retryable codes, throttle delay from RetryInfo)
- Test: std/native/otlp_test.xo (a std/grpc collector: ok, gzip, partial success, Unavailable retried, ResourceExhausted with and without RetryInfo, InvalidArgument, a missing key, a closed port, the request limit; the retry table per code)
- Shipped in: not yet released

`grpc_exporter` sends traces and metrics as unary Export calls over OTLP/gRPC with the exporter's headers as metadata and gzip when compressing. Retried codes and the RetryInfo delay follow otlptracegrpc; a final status is `ExportErr.Grpc(code, message, tries)` (the code number; `grpc.code_from` gives the `grpc.Code`), displayed as grpc-go's text.

## 2026-11-17: test: differential fuzz against protobuf-go and otlptracehttp's JSON (task #115)

- Kind: test
- Commit: this commit
- Found by: task #115 (testing evidence)
- Go reference: go.opentelemetry.io/proto/otlp v1.11.1 with google.golang.org/protobuf v1.36.12 (deterministic Marshal); otlptracehttp v1.47.0 internal/otlpjson
- Test: internal/stdtest/otlpfuzz_test.go (TestOtlpFuzz; oracle scripts/stdcorpus/otlporacle, Xo side scripts/otlpdiff/xo)
- Shipped in: not yet released

Seeded Resources and 0 to 4 std/trace spans (given, derived and malformed ids, every kind and status rule, HTTP statuses, attributes of every value kind with NaN and the infinities, events, scopes and schema URLs): `encode_traces` and `encode_traces_json` equal the oracle's bytes on seeds 2, 3 and 4 (5,000 cases each) on arm64 and llvm. No difference found.

## 2026-11-17: feature: JSON encoding, gzip, size limits and partial success (task #184)

- Kind: feature
- Commit: this commit
- Found by: task #184 (stdcov feature gaps against otlptracehttp v1.47.0)
- Go reference: go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.47.0 (WithEncoding, WithCompression, WithMaxRequestSize, WithMaxResponseSize, partial success handling)
- Test: std/native/otlp_test.xo (request bodies, partial success, the size errors' text; the oracle JSON expects from scripts/oteloracle)
- Shipped in: not yet released

`encode_traces_json` writes OTLP's JSON mapping byte for byte as Go's otlptracehttp does (checked against its output for the 7 oracle cases); `with_encoding(Encoding.Json)` sends it as `application/json`. `with_compression(Compression.Gzip)` gzips request bodies. `with_max_request_size` and `with_max_response_size` bound the bodies (`RequestTooLarge`, `ResponseTooLarge`; 64 MiB and 4 MiB by default, as Go). A 2xx response's partial success is `ExportErr.Partial`, displayed as Go writes it.

## 2026-11-16: behavior: `exporter` takes a `Rand[rand]` (decision 0652)

- Kind: behavior
- Commit: this push
- Found by: task #141, Rand's effect parameter (decision 0652)
- Go reference: none (Xo's effect system)
- Test: std module tests; golden rand_pure
- Shipped in: not yet released

Rand takes its effect as a parameter now; these hold the OS generator, so they are written with the effect they had before. A `Rand[()]` (a pure one) is still accepted.

## 2026-11-15: feature: `ExportErr` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`ExportErr` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

## 2026-11-11: perf: metrics export encodes derive Proto message structs

- Kind: perf
- Commit: this push
- Found by: decision 0549 item 3, after derive Proto's size pass
- Go reference: go.opentelemetry.io/proto/otlp (metrics.proto), as protobuf-go marshals it
- Timing: bench/results/otlp-derive-2026-11-11.md third table: bench/perf/otlp metrics 0.855 of the hand writer's wall time (linux/arm64 host 1, --ab)
- Test: the otlp corpus on go, arm64, llvm; std/native/otlp_test.xo; a registry with summaries, function families, a zero gauge and an empty bucket list hashes the same as the writer's output
- Shipped in: not yet released

## 2026-11-11: perf: trace export encodes derive Proto message structs

- Kind: perf
- Commit: this push
- Found by: decision 0549 item 3 (derive in std; otlp may use message structs if its timing does not regress)
- Go reference: go.opentelemetry.io/proto/otlp (trace.proto, common.proto, resource.proto), as protobuf-go marshals it
- Timing: bench/results/otlp-derive-2026-11-11.md: bench/perf/otlp traces 0.869 of the hand writer's wall time (linux/arm64 host 1, --ab)
- Test: the otlp corpus (protobuf-go's bytes) on go, arm64, llvm; std/native/otlp_test.xo (the OpenTelemetry Go oracle) on arm64, llvm
- Shipped in: not yet released

`encode_traces` builds the request as derive Proto structs
(ExportTraceServiceRequest down to AnyValue, whose value is a oneof)
and marshals them once, where it wrote each nested message to a Bytes
of its own and copied it into its parent. Output unchanged.

## 2026-11-09: feature: encode_traces writes the OpenTelemetry span fields (decision 0580)

- Kind: feature
- Commit: this push
- Found by: Xo Azure 1 (tracing/azotel)
- Go reference: opentelemetry-proto trace.proto (Span 1 to 15, Event, Status, KeyValue, AnyValue) and the OpenTelemetry Go exporter
- Test: std/native/otlp_test.xo (hand vectors) and the OpenTelemetry Go oracle (otel v1.47.0 through otlptracehttp; Xo Azure 1): std/native/trace_test.xo's generated tests (scripts/oteloracle/gen.py) compare the request bodies of 7 cases (root and child, every kind, every attribute type, attribute overwrite, events, ten status sequences, scopes with versions and schema URLs) byte for byte
- Shipped in: not yet released

`encode_traces` writes a span's own trace, span and parent ids, kind, attributes, events, status, nanosecond times, and groups spans by (scope, version, schema URL) into ScopeSpans (a span with no scope uses the Resource's); a span that leaves them at their defaults encodes as before. A span with its own span id is written as the OpenTelemetry Go SDK writes it: Status always present (empty when unset), `flags` 257 (field 16), an unspecified kind as internal.

## 2026-11-06: feature: std/otlp, OTLP trace and metrics encoders

- Kind: feature
- Commit: this commit
- Found by: decision 0539 (OTLP export of std/trace and std/metrics)
- Go reference: opentelemetry-proto 1.x (trace.proto, metrics.proto, common.proto), checked against google.golang.org/protobuf marshaling the same messages
- Test: testdata/corpus/otlp (trace requests against protobuf-go), std/native/otlp_test.xo
- Shipped in: not yet released

`otlp.encode_traces(res, spans)` and `otlp.encode_metrics(res, registry, start_ms, now_ms)` write the OTLP request bodies
(see spec/std.md section 69). The HTTP exporter with retry follows.

## 2026-11-07: feature: otlp.Exporter, OTLP/HTTP with retry

- Kind: feature
- Commit: this commit
- Found by: decision 0539
- Go reference: the OTLP/HTTP specification (retry on 429, 502, 503, 504 with Retry-After; other statuses are permanent)
- Test: golden otlp_export (go, arm64, llvm)
- Shipped in: not yet released

`otlp.exporter(client, endpoint, res, clock, rand)` with `send_traces` and `send_metrics`, `with_headers` and `with_backoff`.

## 2026-11-07: feature: Retry-After as an HTTP date

- Kind: feature
- Commit: this commit
- Found by: follow-up of decision 0539
- Go reference: OTLP exporter retry rules (opentelemetry-go otlptracehttp: Retry-After seconds only; the date form is RFC 9110)
- Test: std/native/otlp_test.xo (retry-after: seconds, an HTTP date, ignored values)

The exporter waits for the larger of its backoff delay and the server's `Retry-After`, which may now be a number of seconds or an HTTP date (counted from the exporter's clock); a date in the past, a negative number or other text asks for no extra wait.
