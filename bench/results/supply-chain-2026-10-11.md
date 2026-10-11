# Supply-chain hardening: Backstabber's Knife Collection replication (2026-10-11)

Decision 0156. This measures how Xo's supply-chain defenses stand against
the attack classes in the Backstabber's Knife Collection taxonomy (Ohm,
Plate, Sykosch, Meier, *Backstabber's Knife Collection: A Review of Open
Source Software Supply Chain Attacks*, DIMVA 2020). The Flix paper
(*"A Flexible, Effect-aware Package Manager"*, reported) replays 51 of
those attacks and blocks 48.

## Method

Each attack class is modelled as a small malicious Xo dependency and a
consumer, built and run with the real toolchain. The harness is
`cmd/xo/TestSupplyChainAttacks` (regenerate the table with
`XO_SC_REPORT=1`). Every "malicious" behaviour is benign and test-only:
it dials a fake host through the test network, reads a fake environment
variable, or writes a MemFs; nothing leaves the test. For each attack the
harness records the first stage Xo blocks it:

- **fetch**: no code runs (no install/build hook);
- **check**: `xo check` rejects it (a ceiling XO1309, an unvouched
  escape hatch XO1316, a top-level statement, ...);
- **lock**: the effect lock rejects an update (XO1317);
- **runtime**: a narrowed capability denies it (`Net.only`, ...);
- **not blocked**: the effect/capability layer does not stop it, recorded
  honestly with the reason.

Two consumer postures appear: *default* (a required dependency, no
ceiling, unattenuated capabilities) and *hardened* (an effect ceiling, or
an attenuated capability). The posture is named in each row.

## Measured result

**10 of 15 scenarios are blocked** (fetch 1, check 7, lock 1, runtime 1;
5 not blocked).

| stage | count |
|---|---|
| fetch | 1 |
| check | 7 |
| lock | 1 |
| runtime | 1 |
| not blocked | 5 |

| attack | class | stage | detail |
|---|---|---|---|
| install-time hook | install hook | fetch | no fetch/install/build hook exists (core 4.2: no code runs at fetch or build setup); a dependency's code runs only when the consumer calls it |
| import-time execution (typosquat trigger) | import trigger | check | a top-level statement in a dependency is a compile error (XO0168); Xo has no module init or import-time execution |
| exfiltrate env over net, ceiling `uses none` | data exfiltration (hardened) | check | XO1309 at the call into the dependency |
| write files / wipe, ceiling `uses none` | sabotage (hardened) | check | XO1309 |
| spawn a process / dropper, ceiling `uses none` | backdoor/dropper (hardened) | check | XO1309 |
| logging exfiltration, ceiling forbids `log` | data exfiltration (hardened) | check | XO1309 (logging is the `log` effect, decision 0156) |
| C binding lies about effects, unvouched | effect laundering | check | the dependency's own `use c ... uses none` does not count; the consumer must vouch (XO1316), and the unvouched binding is charged its worst case (ffi) |
| malicious update adds an effect, with a ceiling | malicious update (hardened) | check | XO1309 after the update |
| malicious update adds a net effect | malicious update (default) | lock | the effect lock recorded v1.0.0 as `effects none`; the update to v1.1.0 adds `net.dial`, so `xo get` fails with XO1317 and writes nothing |
| exfiltrate over net, attenuated capability | data exfiltration (hardened) | runtime | `os.net.only([...])` denies the dial (NetErr.Denied), proven by a consumer test |
| exfiltrate env over net, no ceiling | data exfiltration (default) | not blocked | the effects (`net.dial`, `env`) are visible in the dependency's API and recorded in xo.lock, and `xo audit` surfaces them, but the consumer granted the capabilities and set no ceiling |
| logging exfiltration, `log` allowed | data exfiltration (default) | not blocked | `log` is visible in the API and a ceiling can forbid it, but when the consumer allows `log` the record is still written |
| C binding lies, consumer vouches `uses none` wrongly | effect laundering | not blocked | a vouch is a human review; a consumer that attests `uses none` for C that actually does I/O is not protected (Xo cannot see into C). The vouch records who attested it |
| pure malice: wrong result (swapped payee) | logic / no effect | not blocked | pure computation performs no effect; a wrong return value is not an authority misuse. Contracts and tests are the defense, not the supply-chain layer |
| denial of service: unbounded loop | sabotage / DoS | not blocked | a CPU/time DoS performs no capability effect; the effect and capability layer does not bound compute |

## Reading the five "not blocked"

Two are **out of scope for an effect system** and would be for Flix's too:
pure logic that returns the wrong answer, and a compute/time DoS. Neither
touches a capability, so no effect or ceiling applies; the defenses are
contracts, tests, and (for DoS) a resource budget, which is a separate
mechanism.

Two are **default-posture choices**: a dependency the consumer hands
`os.net`/`os.env` with no ceiling can reach the network, and a dependency
the consumer lets log can log a secret it was given. Both are visible in
the dependency's signatures and in `xo audit`, and both become blocked
the moment the consumer writes a ceiling (`uses none` / without `log`) or
attenuates the capability. They are "not blocked" only because the
consumer declined to harden, not because the system cannot express the
restriction.

One is a **wrong human attestation**: vouching `uses none` for C that
lies. Xo cannot see into C, so a false vouch defeats the check. What Xo
adds over trusting the dependency is that the attestation is the
*consumer's*, per version, and recorded for review (XO1316 forces it);
the dependency can no longer vouch for itself.

## Comparison with Flix (48 of 51), with caveats

- **Not the same corpus.** Flix replays 51 concrete npm/PyPI packages.
  Those payloads are JavaScript and Python; they cannot be run as Xo
  dependencies. This replication models the *behavioural classes* of the
  taxonomy (exfiltration, droppers/backdoors, sabotage, install and
  import triggers, effect laundering, malicious updates, pure logic,
  DoS), not the 51 packages one-for-one. The counts are therefore not
  directly comparable; 10/15 is "classes blocked", not "packages
  blocked".
- **Different threat model split.** The dominant Backstabber class is the
  *malicious update to an existing, already-trusted package*. Against
  that class Xo blocks **by default**: the effect lock (XO1317) fails any
  build whose dependency reaches a wider effect set than the lock
  records, with no ceiling required. Flix's effect-aware manager likewise
  shines here. For *newly adopted* malicious packages, Xo's default
  posture surfaces the reach (signatures, lock, `xo audit`) but blocks
  only once the consumer hardens; this is the deliberate
  capabilities-and-review design (decision 0004), not a gap in
  expressiveness.
- **Install and import triggers** (a large real-world share) are blocked
  structurally and permanently: Xo runs no code at fetch or build, and
  has no module-init or top-level execution, so there is nowhere for an
  install hook or an import-time payload to live.
- **What neither system claims.** Pure-logic tampering and compute DoS
  are outside an effect system. An honest "48 of 51" or "10 of 15" should
  exclude or flag these; here they are the last two rows.

## Next experiments

1. Per-record log attribution (tag each `log` record with the writing
   dependency), so the "log allowed" row becomes traceable even when not
   blocked. Touch `internal/gen` (thread the module into `Log_*` calls)
   and `std/rt/log.go`.
2. A resource budget capability (CPU time / allocation ceiling per task)
   to bring the DoS row into scope; design as a new capability, not an
   effect.
3. Expand the corpus toward the 51 by porting each Flix case's *behaviour*
   (what the payload does) rather than its code, to tighten the
   comparison, and record which Flix cases have no Xo analogue (for
   example, prototype pollution, which Xo's type system precludes).
4. A `vouch`-coverage lint in CI (fail when a dependency gains a new
   escape hatch an existing vouch does not cover), so the wrong-vouch row
   at least cannot drift silently on update.
