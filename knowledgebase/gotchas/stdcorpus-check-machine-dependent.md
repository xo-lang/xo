# `stdcorpus -check` flagged almost every corpus on a Mac with another Go patch release

Status: Observed 2026-11-15 (lang3, the lead's stale yamlfuzz and xmlcodec item)

## Symptom

On the Mac (go1.26.11) a bare `go run ./scripts/stdcorpus -check` called
79 of 91 corpora stale. main's headers said go1.26.1 (81 corpora) or
go1.26.11 (10), so the check failed on either release. xmlcodec, reported
stale by aws-43, differed only in that header.

## Cause

Every corpus header carries `runtime.Version()`, and -check compared the
whole file. Past the header, seven corpora depend on the machine or are
not deterministic at all:

- httpurl and mail read their inputs from GOROOT's net/url and net/mail
  tests, which changed between go1.26.1 and go1.26.11;
- protoparse reads the .proto files of the local Go module cache;
- amqp (Go map order), grpc (timings observed on the wire), jwt and
  x509create (ECDSA's random nonce) differ from one run to the next.

## Fix

-check (and a regeneration) now ignores the Go release in each header
(sameButGoVersion in scripts/stdcorpus/main.go), and a regeneration keeps
a file whose only difference is that release. The seven above still show
as stale on the Mac: check a corpus you changed by name, and for those
seven compare against a second generation, not the committed file.
