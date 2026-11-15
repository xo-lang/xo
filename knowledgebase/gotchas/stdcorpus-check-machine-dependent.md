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
a file whose only difference is that release (6a8ed7d6f). The six
corpora above that Lang 3 owns were then made deterministic and
independent of the machine (2026-11-15, the lead's follow-up):

- httpurl and mail read Go 1.26.11's url_test.go and message_test.go
  from scripts/stdcorpus/testdata/gosrc (goSrc in mail.go), not GOROOT;
  refresh that copy with a Go toolchain bump.
- protoparse reads its 46 sampled files from
  scripts/stdcorpus/testdata/protoparse;
  `XO_STDCORPUS_PROTO_RESAMPLE=1` samples the module cache again.
- amqp: the oracle rewrites each encoding with every map's entries in
  key byte order (amqporacle/canon.go).
- grpc: grpcref prints the header grpc-go's encoder gives for the
  seeded deadline (a copy of grpcutil.EncodeDuration) after checking the
  copy against each header grpc-go sent on the wire.
- jwt: the oracle signs with a crypto/rand.Reader that reads as one
  repeated byte, under `//go:debug cryptocustomrand=1`. A seeded stream
  is not enough: Go 1.26's custom reader path (crypto/internal/rand
  CustomReader) may read one byte more at random (MaybeReadByte), which
  shifts a stream but not a constant one.

x509create (ECDSA's random nonce) is aws-43's, through the lead. With it
fixed, `go run ./scripts/stdcorpus -check` passes on any machine, and the
full tier's Mac steps run it (gate.sh step corpuscheck).

Pinning the inputs showed a real gap: Go's consumePhrase changed in a
1.26 patch release, and std/mail still skipped CFWS after an encoded
word (std/changes/mail.md, 2026-11-15).

Hit again (2026-11-15, host 3): psloracle panicked there, since
GOPROXY is off and that machine's module cache lacks golang.org/x/net.
psloracle now replaces x/net with scripts/stdcorpus/psloracle/xnet,
x/net v0.60.0's publicsuffix package copied unchanged (with its
LICENSE), so `stdcorpus -check publicsuffix cookiejar` passes with an
empty GOMODCACHE. An oracle that needs a third-party module either
carries the code it needs this way or reads pinned inputs from testdata.
