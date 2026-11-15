# Copying a host's setup misses the upgrades made after its source was set up

Status: Observed (2026-11-15)

**Symptom.** Lang 3's openenum6 gate failed on host 4 only, in Go-backend
tests the change does not touch: TestStdCorpora go/httpurl (a multi-host
`url.Parse`), TestGoldenGoParity/http_request_guards and FuzzHTTPDiff seed
40. Host 3 passed the same commit.

**Cause.** Host 4 (2026-11-15) was built from `golang:1.26.1-trixie` with
host 3's files copied over (/opt/llvm, xo-cc, the profile, the seeds, then
/work's gate directories). Host 3 had been moved to go1.26.11 in
/usr/local/go on 2026-11-13, after its own setup, and the copy list was
written from the setup notes, not from the host, so host 4 kept go1.26.1.
The Go-backend tests compare against Go 1.26.11's behavior (net/url and
net/http changed in those patch releases). It was the second gap in host 4's
setup that day (the first: /work's gate directories,
[host-probe-skips-bundle-path.md](host-probe-skips-bundle-path.md)).

**Fix.** go1.26.11 copied from host 3 (`docker cp xo-host3:/usr/local/go - |
docker cp - xo-host4:/usr/local`, the image's tree kept as
/usr/local/go.go1.26.1). To catch the class: scripts/lead/host-toolchain.txt
names the go, clang, node and seed every host must have, and
offload-gate-remote.sh refuses a gate on a host that differs (exit 2,
`HOST-PARITY: ... go: go1.26.1 (want go1.26.11)`). An upgrade changes every
host and that file together. When cloning a host, compare the new one with
the source by running the check, not by a list.
