# std/norm: what importing it costs (2026-11-03)

Decision 0476 asked for the `__text` and binary size of a program that imports std/norm against one that does not. Measured at the std/norm commit 63e32de25, darwin/arm64, `xo build --release`, `size -m` on the Mach-O.

Program A (no import) prints its argument and its length; program B imports std/norm and prints `norm.normalize(s, norm.Form.Nfc)` and the length:

```xo
// a/main.xo
fn main(os: Os) uses stdio, env {
  let s = os.env.args().get(0) ?? "e"
  os.stdio.println("${s} ${s.len()}")
}
// b/main.xo
use std/norm

fn main(os: Os) uses stdio, env {
  let s = os.env.args().get(0) ?? "e"
  os.stdio.println("${norm.normalize(s, norm.Form.Nfc)} ${s.len()}")
}
```

Each directory holds an `xo.mod` of `module example.com/na` (or `nb`) and `edition preview`. Commands, from each directory:

```sh
XO_STD=$CHECKOUT/std $CHECKOUT/bin/xo build --release -o ../a.bin .
size -m a.bin b.bin
ls -l a.bin b.bin
```

| | A (no import) | B (imports std/norm) | difference |
|---|---|---|---|
| file | 227 KB | 519 KB | +292 KB |
| `__text` | 135,552 | 222,300 | +86,748 |
| `__const` | 7,011 | 206,755 | +199,744 |
| `__cstring` | 1,259 | 2,318 | +1,059 |

The tables are the cost: `std/native/norm.xo` holds the decompositions (2,061 canonical, 3,812 compatibility), the combining classes and 941 composition pairs as `List[Int]` constants, 8 bytes an element. A program that does not import std/norm pays nothing. A 4 byte element list or a packed table would about halve the data. The code size is dominated by the binary searches and the three passes, and is not yet tuned.
