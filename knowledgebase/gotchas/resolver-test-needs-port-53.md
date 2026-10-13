# A resolver test needs its own /etc and port 53

Status: Anticipated (2026-10-13, decision 0206 stage 3)

Symptom: a test of the Xo resolver (std/native/llrt/dns.xo) against a
local DNS server cannot point the resolver at the server: resolv.conf's
`nameserver` takes an address and no port (glibc's and Go's resolvers
always ask port 53), binding port 53 needs privilege, and /etc/
resolv.conf, /etc/hosts, and /etc/nsswitch.conf are the host's.

Cause: the resolver reads the system files, as getaddrinfo does; there
is no test hook (and adding one would be a runtime switch only tests
use).

Fix: run the program and the server in a new user, network, and mount
namespace (`unshare -r -n -m`): there the test is root, may bind
127.0.0.1:53, brings `lo` up itself (a new network namespace's loopback
is down: SIOCSIFFLAGS), and bind mounts its files over /etc's
(internal/llvm resolver_linux_test.go). The server is the test binary
itself (TestResolverDNSServer), started in the namespace and stopped by
its PID. Hosts without user namespaces, or without /etc/nsswitch.conf to
mount over, skip the test.
