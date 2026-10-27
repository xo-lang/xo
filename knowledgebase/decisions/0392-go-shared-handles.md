# 0392: One Xo type per Go type across `use go` bindings

Status: Accepted (owner, 2026-10-27: "Approved, go ahead with 0392";
proposed by xo-lang-4; milestone 0.34.0 criterion 1, Intel tester 0.33.0
b14). Implemented 2026-10-27 (see "As built").

## Context

A binding (internal/gobind) declares a handle for every named Go type its
package's own signatures use, including types from other packages
("foreign handles", `handle.foreign`): `net`'s binding declares its own
`AddrPort` for `netip.AddrPort`, with its own adapter type
`Go_net_AddrPort` in package rt. A program that binds both packages has
two unrelated Xo types for one Go type:

- `net.tcp_addr_from_addr_port(netip.addr_port_from(...))` is XO0401
  (expected `net.AddrPort`, found `netip.AddrPort`), and nothing converts
  between them.
- golang.org/x/text/cases' `upper`, `lower` and `title` take a
  `language.Tag`, which the cases binding declares as `cases.Tag`. No
  function of `cases` returns one, so the functions cannot be called at
  all.
- A foreign type of a foreign type (depth > 0) is skipped outright
  (`catForeign`), so the reach of a binding stops one package away.

Spec core 2.9 and 4.4 say Go values keep Go's semantics; in Go these are
one type.

## Decision

A named Go type is bound once, in the binding of the package that
declares it, and every other binding refers to that one.

1. **References, not copies.** When a binding's signature names a type
   from another bindable package P, the generated declaration file has
   `use go "P"` and writes the type as `p.T` (its local name the
   binding's alias for P). The adapter converts with P's rt type,
   `Go_<P's prefix>T`, which P's adapter defines. Equality, hashing and
   `Debug` (decision 0112's exception, core 2.9, and b15's value rule)
   are P's, so one Go type has one meaning.
2. **Dependencies bind too.** The binding header lists the packages it
   refers to (`// go-deps: net/netip`). The checker loads those
   bindings as it loads the program's own `use go` lines (goimport.xo:
   a missing or stale one is bound the same way, through the Go checker,
   go_miss), so a program that only says `use go "net"` still gets one
   `netip.AddrPort`. Go's import graph is acyclic, so the bindings are.
3. **What stays skipped.** A type from an internal package, or one in a
   package that cannot be bound (cgo only, `XO1101`), is skipped with
   the reason, as today. The depth limit goes: depth only mattered
   because each binding copied its foreign types.
4. **Pins.** A third-party package reached only through `go-deps` needs
   no `use go` of its own; its module is pinned by the `require go` line
   of the module that brought it (one Go module, decision 0356), or, when
   it is another Go module, by the binding workspace's go.sum, recorded in
   xo.lock as the workspace's requirements are today. No new XO1319 for a
   package the program never names.
5. **Format.** Binding format 7 (gobind.Version, GOBIND_VERSION); every
   cached binding is regenerated once.

## Consequences

- b14's program compiles and prints `10.0.0.1:80`; x/text's cases
  functions take `language.Tag`.
- More bindings per program (each referenced package once), each made
  once per module version and cached as today; the Go build links the
  same Go packages as before.
- Generated names change: a foreign handle's Xo name was `AddrPort` in
  `net` and is `netip.AddrPort`; a program that named `net.AddrPort`
  (only possible as a type annotation, since nothing else produced it)
  gets XO0601 with a fix to the home package's name.
- The replay and record classification (decision 0093) is per function
  and unchanged; a handle crossing still marks the call `handle`.

## Alternatives

- **Conversions between the copies** (`net.AddrPort.from(netip.AddrPort)`):
  keeps two types for one, needs a generated function per pair and per
  direction, and x/text's `cases.Tag` still has no source unless every
  foreign type gets a converter from its home type. Rejected: it moves
  the problem to every call site.
- **Aliases only when the home package is also bound** (`type AddrPort =
  netip.AddrPort` when the program has `use go "net/netip"`): the meaning
  of `net`'s binding would depend on the program's other imports, and a
  binding is cached per package. Rejected.
- **Leave it, document the limit**: b14's two examples are common (netip
  through net, language through cases and display); rejected for 0.34.0's
  field bug criterion.

## As built (2026-10-27)

- internal/gobind: a foreign named type is a reference handle
  (`handle.ref`): its Xo name is `<alias>.<name>`, where the name is the
  one the home binding gives it (homeHandleName repeats the home run's
  naming: types in order, a prelude or earlier name prefixed with the
  package's name, then `Go`), and its rt type is `Go_<home prefix><name>`.
  Nothing is declared for it, so equality, hashing and methods are the
  home binding's. A package a program cannot import (`internal`,
  `vendor`) keeps the skip (`catForeign`, with the reason).
- The dependencies are the declaration file's `use go "P" as alias`
  lines (the alias the Go package name, numbered when taken), not a
  separate `go-deps` header: the checker loads them as any `use go`.
  Bind binds each one in the same run (LoadOrBind, recursively), so one
  check finds them all instead of the checker's bounded rebind rounds.
- compiler/check: a binding's own `use go` (Module.go_path set) resolves
  against the binding's project root (Module.go_root) and skips the vouch
  (XO1316) and pin (XO1319) checks, which the program's own use already
  passed.
- Binding format 7.

## Tests (landed with it)

- cmd/xo TestUseGoSharedType (std only): b14's program prints
  `10.0.0.1:80`.
- The spec-conformance golden internal/gen go_shared_type: the netip
  value through net and back, `==` across the two bindings, and a net
  value's AddrPort.
- internal/gobind TestSharedHandles: net's declaration has `use go
  "net/netip" as netip` and takes `netip.AddrPort`, declares no AddrPort
  of its own, and netip is bound in the same run; internal and vendor
  packages are not importable. TestMapping: os/exec's Cmd.process returns
  `os.Process?`.
