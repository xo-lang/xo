# 0203: Linux programs without libc

Status: Accepted (owner, 2026-10-13); not started. Unblocks decision
0195 prerequisite 3 (raw system calls) on Linux.

## Context

Decision 0195 allows calling the platform's own libraries where the
platform requires it. macOS (libSystem) and Windows (kernel32, ntdll,
ws2_32) require it; Linux does not: its system call interface is
stable. The zero-C audit (bench/results/zero-c-audit-2026-10-13.md,
"Owner decisions found", B) asked whether Linux programs keep calling
the system C library (glibc or musl, dynamically) or make raw system
calls like Go does.

## Decision

Without libc. On Linux, native Xo programs make raw system calls and
link statically with no C library, as Go's default does. This
includes, in Xo: thread creation (clone3 or clone), thread local state
setup, futex waits, signals (rt_sigaction with Xo's own restorer),
memory mapping, sockets and files, the clock (the vDSO for
clock_gettime), and a DNS resolver in place of getaddrinfo (resolv.conf,
/etc/hosts, the search and ndots rules, and nsswitch's common cases).
macOS and Windows keep calling their platform libraries.

## Consequences

- One static binary that runs on any Linux of the supported
  architectures, independent of glibc versions (the GLIBC_2.17 stat
  problem of the switch point B flip disappears).
- The DNS resolver must match the system resolver closely enough for
  servers; differences (nsswitch modules such as LDAP or mDNS) are
  documented, as Go documents its pure Go resolver.
- Each piece follows decision 0193's mechanism: the libc path stays
  selectable until the raw path passes the goldens and the bar.
- `use c` programs that link C libraries still need libc (decision D
  of the audit, pending).

## Alternatives

- Call the system C library dynamically on Linux: rejected by the
  owner; it keeps a glibc version dependency and C in the runtime path.
