# A gate host on an older Go fails Go backend tests the change never touched

Status: Observed (2026-11-15, gate openenum6 on host 4)

## Symptom

A gate fails only on Go backend checks of modules the change does not
touch: TestStdCorpora go/httpurl (url.Parse of
`mongodb://user:password@host1:1,host2:2,host3:3` answers "invalid port"
where the corpus expects the other answer), TestGoldenGoParity
http_request_guards, FuzzHTTPDiff seed#40. The native backends pass.

## Cause

The Go backend runs Go's own net/url and net/http, so its answers follow the
host's Go release. Host 4 (a new container from `golang:1.26.1-trixie`, set
up 2026-11-15) ran go1.26.1; host 3 moved to go1.26.11 on 2026-11-13
(offload.md), and the corpora and goldens follow go1.26.11
(stdcorpus-check-machine-dependent.md). Copying a host's setup does not copy
an upgraded /usr/local/go.

## Fix

Check `go version` on a new host before it takes gates (an offload.sh job:
`go version`), and install the release the other hosts run, as host 3 did.
Until then, gate elsewhere: `OFFGATE_HOST4=0` keeps offload-gate's old first
host.
