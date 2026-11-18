# A fuzzed DNS reply that points into the ID differs run to run

Status: Observed (2026-11-17, TestDnsFuzz seed 133 case 121)

Symptom: one TestDnsFuzz case differs on some runs and not others (3 of 8
runs agreed), on either backend: std/dns parses the TXT answer, Go's
resolver says "cannot unmarshal DNS message", or the other way round.

Cause: the "byte" change turned an owner name's pointer into one to
offset 44, whose name is a 61 byte label and then a pointer to offset 0:
the message ID. The fake server copies the query's ID, and each resolver
picks its own at random, so whether the name ends there (an ID whose
first byte is 0) or runs on depends on the run. Neither parser is wrong.

Fix: dnfAnswer drops a change that leaves two bytes reading as a pointer
to offset 0 or 1 (dnfReadsID); a name walk reaches the ID only through
one. A fuzz harness whose input comes back with a field the client picks
at random (an ID, a nonce) must keep the answer's meaning independent of it.
