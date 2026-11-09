# Go's HTTP /alloc baseline at its GC defaults is GC bound

Status: Observed (2026-11-09, Xo Lang 4; bench/perf/jitter on the
linux/arm64 timing host, go1.26.1).

## Symptom

The 0.44.0 release comparison had Xo's /alloc at 32 connections serving
5.99 times Go's requests per second (168,572 against 28,149), with Go's
p99 at 5.5 ms and 118 us of CPU per request, though Go's /hello was
within 1.3x of Xo's.

## Cause

Go's server runs at GOGC=100 with a live heap of a few MB, so its heap
target stays near Go's 4 MB minimum and each /alloc request (a 16 KB
buffer and about 800 small objects) brings the next collection close:
GODEBUG=gctrace=1 counted about 1,250 collections a second (12,600 in
a 10 s run). Same server, same pinning: GOGC=400 94,300 req/s; GOGC=off
with GOMEMLIMIT=1GiB 140,500 req/s at 41 us of CPU and 1 GB of peak RSS;
json.Marshal in place of the hand encoder 23,000.

## Fix / how to avoid

scripts/release-vs-go-http.sh runs Go twice, at its defaults (go) and
with GO_TUNED_ENV (go-tuned, default `GOGC=off GOMEMLIMIT=1GiB`), and
records each server's peak RSS. Quote both: on 2026-11-09 Xo served 5.74
times default Go's /alloc c32 and 1.21 times tuned Go's (1.26 at 256
connections), at 5.6 MB of peak RSS against tuned Go's 1 GB.
