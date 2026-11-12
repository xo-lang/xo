# 0617: The std features axis, high group (format specs, HTTP client proxies and cookies, log handlers)

Status: Accepted (owner, 2026-11-12: all three parts, A, B and C, in that order). Proposed by Xo Lang 3, 2026-11-12, at the lead's request: the owner's high group of the std features axis, 2026-11-12).

## Context

The owner's std track (2026-11-12) names a high group of feature gaps,
formatting and the HTTP client first:

1. fmt style width, padding, precision and base formatting;
2. the HTTP client: cookie jar, proxies, streaming response bodies;
3. log handlers: per level sinks, custom destinations;
4. subscribing to other signals (SIGHUP, SIGUSR1);
5. OS information: hostname, pid, rusage, statfs.

Checked against main 26534b74e (the std audit of 2026-11-12,
bench/results/std-audit-2026-11-12.md, and spec/std.md), some are closed
already:

- **4 is done**: `Proc.notify(signals)` with `Signal.Hangup`, `User1`,
  `User2`, `WindowChange`, `Quit`, `Interrupt`, `Terminate` (decision 0407).
- **5 is done**: `Env.hostname()`, `Proc.pid()` (decision 0410),
  `Proc.usage()` (getrusage, decision 0426), `Fs.disk_usage(path)` (statfs).
- **2's streaming is done**: `Client.open(req) -> ClientStream` reads a
  response body as it arrives (spec/std.md 4.5), and `Client.with_redirects`
  (decision 0404) follows redirects.
- **1's operations are done as methods** (decision 0410):
  `Int.to_str_width(width, base, zero, plus)`, `Int.to_str_base`,
  `Float.to_fixed`, `to_exp`, `to_general` (precision, width, zero, plus),
  `Str.pad_left`, `pad_right`, `center`.

What is open:

- **Format specs in interpolation.** `:?` is the only format spec:
  `${x:.2}` is `XO0160` (spec/core.md 1.6). A table row is
  `"${name.pad_right(12)} ${n.to_str_width(8)} ${p.to_fixed(2, width: 7)}"`,
  where Go writes `"%-12s %8d %7.2f"` and Rust `{:<12} {:>8} {:7.2}`.
  Agents writing Go or Rust style reach for the spec first.
- **Proxies.** The Client connects straight to the origin. It reads no
  `HTTP_PROXY`, `HTTPS_PROXY` or `NO_PROXY` and has no explicit proxy
  (audit row 9, `net/http.ProxyFromEnvironment`). Corporate and CI
  networks often require one.
- **The cookie jar is not wired to the Client.** `std/cookiejar.Jar`
  exists (decision 0475): `set_cookies(url, cookies, now)`, `cookies(url,
  now)`. Every caller reads `Set-Cookie` and adds `Cookie` headers by hand,
  including on redirects, where it cannot see the intermediate responses.
  The jar also has no public suffix list (map.txt: `cookiejar.*`), so it
  accepts a cookie for `co.uk` from `example.co.uk`.
- **Log handlers.** `log.configure(out: Stdio, level, format)` writes every
  record at or above one level to standard output or standard error, as
  text or JSON. A program cannot send errors to standard error and
  everything else to a file, write to a file or socket, or forward records
  (to OpenTelemetry logs, a syslog daemon, a test's buffer).

## Decision

### A. Format specs in interpolation (first)

`${expr:SPEC}` takes a spec after the colon, checked when the program is
checked and lowered to the 0410 methods, so it costs nothing at run time
beyond the method call:

```
SPEC   := [[FILL] ALIGN] [SIGN] ["0"] [WIDTH] ["." PREC] [TYPE] | "?"
ALIGN  := "<" | ">" | "^"
SIGN   := "+"
WIDTH, PREC := a decimal literal
TYPE   := "d" | "x" | "X" | "o" | "b" | "f" | "e" | "g" | "s"
```

- Int: `d` (default), `x`, `X`, `o`, `b`; width, `0` and `+` go to
  `to_str_width`, `X` upper cases it.
- Float: `f` (default when PREC is given), `e`, `g`; to `to_fixed`,
  `to_exp`, `to_general`. Without PREC and TYPE, Float keeps its default
  text (`${x}`) and only pads.
- Any `Display` value (Str included): `s` or none; ALIGN and FILL go to
  `pad_left`, `pad_right`, `center` over the value's text. The default
  alignment is right for numbers and left for text, as in Rust and
  Python.
- `?` stays the Debug text (decision 0296) and takes no other part.
- A spec that does not parse, or a part the type does not take (`.2` on
  an Int, `x` on a Str), is a new diagnostic code (claimed when this is
  accepted), with the forms the type takes. `XO0160` keeps its meaning for
  the other misuses it covers.
- Width and precision are literals. A computed width keeps using the
  methods. Spec text is ASCII, and FILL is one rune.

Spec change: spec/core.md 1.6 (interpolation), spec/std.md 1 (the methods
the specs lower to). Every backend gets it from the checker's lowering. Go
`fmt` parity goldens: each spec against Go's verb.

### B. HTTP client proxies and the cookie jar (second)

1. **Proxies.**
   - `Client.with_proxy(url: http.Url) -> Client` sends every request
     through the proxy.
   - `Client.with_proxy_for(f: fn(http.Url) -> http.Url?) -> Client`
     chooses per request; `None` means connect directly.
   - `http.proxy_from_env(env: Env) -> fn(http.Url) -> http.Url?` follows
     Go's `httpproxy.FromEnvironment`: `HTTP_PROXY`, `HTTPS_PROXY`,
     `NO_PROXY` and the lower case names; NO_PROXY's hosts, domains,
     `*`, ports and CIDR blocks; and no proxy for localhost and loopback
     addresses.
   - Plain HTTP goes to the proxy in absolute form. HTTPS opens a
     `CONNECT` tunnel, then TLS to the origin through it. HTTP/2 goes
     over the tunnel after ALPN.
   - The proxy URL's user and password become `Proxy-Authorization:
     Basic`.
   - `Net.only` must allow the proxy's address. The target is checked as
     well, so a sandbox cannot be bypassed through a proxy.
   - The environment is read only when the program asks (`proxy_from_env`
     takes the `Env` capability). This differs from Go's
     DefaultTransport, which reads it implicitly, because Xo's
     capabilities make each input explicit.
2. **The jar on the client.**
   - `Client.with_jar(jar: cookiejar.Jar, clock: Clock) -> Client`:
     `send`, `get` and `open` add the jar's cookies for each request URL.
   - They store every response's `Set-Cookie` in the jar, including each
     redirect hop's (Go's Client.Jar semantics).
3. **Public suffix list.**
   - A new module, `std/publicsuffix` (Go's golang.org/x/net/publicsuffix,
     the table generated from the Mozilla list at a pinned version, with
     its tests ported), provides `effective_tld_plus_one(domain)`.
   - `cookiejar.new_with(psl: Bool = true)`: `new()` keeps its current
     behaviour; the Client documentation recommends `new_with()`.

### C. Log handlers (third)

(The sketch below is as proposed; what was built is in the amendment
after it: an interface with an effect parameter, not a trait.)

```
trait Handler {
  fn enabled(self, level: log.Level) -> Bool
  fn handle(self, r: log.Record) uses log
}
struct Record { time: Time, level: Level, msg: Str, fields: List[(Str, Debug)] }
fn text_handler(out: Sink, level: Level = Level.Info) -> Handler     // today's Text format
fn json_handler(out: Sink, level: Level = Level.Info) -> Handler     // today's Json format
fn multi(hs: List[Handler]) -> Handler                                // every handler that is enabled
fn level_range(h: Handler, min: Level, max: Level) -> Handler         // per level sinks
fn configure_handler(h: Handler) uses stdio                           // replaces configure's sink
```

- `Sink` is standard output or standard error, an open `File`, a
  connected `Conn`, or a function of the rendered line: `fn(Bytes)`, for
  tests and forwarding.
- `configure(out, level, format)` stays as shorthand for one text or JSON
  handler.
- A handler's errors (a full disk, a closed socket) are counted, not
  raised, as Go's slog drops them; `log.dropped()` reports the count.
- `Logger.with(fields)` attaches its fields to the Record, as now.
- Go's `slog.Handler` has `WithAttrs` and `WithGroup` to pre-render
  fields. These are left out until a profile shows the per-record cost.

### Amendment: the installer pays (owner, 2026-11-12)

B.2 and C install code that runs later, under another effect: a jar's
store runs inside `send` (`uses net.dial`), a handler inside every
`uses log` call. The owner chose, for both, that **the call that
installs the code performs its effects** (spec/core.md 6.3, "Installed
code"):

- **C.** `interface Handler[fx: Effects]` with `fn handle(self, r: Record)
  uses fx`; `configure_handler(h: Handler) uses stdio` takes the
  handler's `fx` implicitly (core 6.2), so the call performs `stdio` and
  `fx`. `interface Sink[fx]` with `stderr_sink`/`stdout_sink` (`stdio`),
  `file_sink` (`fs.write`), `conn_sink` (`net.dial`) and
  `func_sink[fx](f)`. `log.Value` (log/slog's `Value`: `Str`, `Int`,
  `UInt`, `Float`, `Bool`, `Time`, `Dur`, `Any`) gives a handler each
  field as its static type made it, in `Record.values`, beside the text
  `fields`; `Record.time` is the record's time. `uses log` stays the
  effect of logging code: a library's log calls do not change.
- **B.2.** A `CookieStore` interface in std/http (a `cookiejar.Jar`
  satisfies it); `Client.with_jar(store, clock)` performs the store's
  effects and reads the clock where it is called (`uses clock` at the
  call site); `send` stays `uses net.dial`; the store's methods take
  `now` explicitly, as `cookiejar.Jar`'s do.
- The rule also names what was implicit: under `net.dial` and
  `net.listen`, TLS checks certificate expiry against the wall clock
  (std/native/llrt tlsio.xo) and the HTTP server writes the `Date`
  header from it (http1w.xo, tlssrv.xo), with no `uses clock`.
- `xo audit` lists every install site with the effects it charges
  (`installs` in its JSON; cmd/xo TestAuditInstalls).

Alternatives the owner did not take: a `log` effect parameterized by the
handler (every `uses log` signature would change), `with_jar` raising
`send`'s effects (option D: the type of a Client would carry them, and
the 62 `: http.Client` annotations in the repository would break), and
handlers limited to std's sinks (no forwarding).

Order: A, then B, then C. Each lands with its spec text, goldens on every
backend, Go parity tests where Go has the feature, std/changes entries,
and coverage rows.

## Implementation

- A, format specs (2026-11-12, Xo Lang 3): the parser splits the spec
  off the `${...}` text (compiler/parser/fmtspec.xo), the checker picks
  the lowering by the value's type and reports XO0180, and SSA and the Go
  generator lower the checked tree (parser.spec_expr) to decision 0410's
  methods. Golden fmt_specs (every backend; its integer and float rows
  equal go1.26.1's fmt for the same verbs), errorcheck interp_spec_parse
  and interp_spec_misfit; core 1.6, the code table and `xo explain
  XO0180`.

- B, proxies, public suffix list and the jar: see below.

### Implementation of B (Xo Lang 3, 2026-11-12)

- Proxies: `with_proxy`, `with_proxy_for`, and `std/httpproxy`
  (`Config.proxy_func`, `proxy_from_env(env)`), checked against Go's
  ProxyFromEnvironment on 1,920 seeded cases (scripts/stdcorpus
  httpproxy); golden `http_client_proxy`. The Go backend's transports no
  longer read HTTP_PROXY on their own.
- Public suffix list: `std/publicsuffix` (golang.org/x/net v0.60.0's
  list.go and its table, generated by scripts/genpsl), checked against
  x/net on 3,058 domains; `cookiejar.new_with(psl: Bool = true)`
  consults it, checked against Go's cookiejar with x/net's list on 80
  seeded scripts.
- The jar: `http.CookieStore` and `Client.with_jar(store, clock)` as the
  amendment above says; golden `http_client_jar`. On the native backends
  the store's calls are installed with `rt.launder1`/`launder2`, the
  tool std already used for installed callbacks.
  Not ported: Go's redirect rule that drops a request's own Cookie field
  when the jar changed a cookie of the same name.

## Consequences

- Format specs make Go and Rust style table output one expression, with
  types checked. They add a small grammar to interpolation, and a new
  diagnostic code.
- With the client's proxies and jar, std/http's client covers what
  backends behind corporate proxies and session based APIs need.
- The public suffix table adds data (its size measured when it lands) to
  programs that import std/publicsuffix only; others are unchanged.
- Log handlers make std/log usable for services that ship logs, at the
  cost of one trait call per record when a handler is configured. The
  default path stays as fast as today.
- Items 4 and 5 of the owner's list are recorded as closed, with their
  decisions.

## Alternatives

- **A printf style function** (`format("%-12s %8d", name, n)`): an
  untyped argument list checked only at run time, or a checker special
  case for one function. Interpolation with specs keeps every argument
  in place and typed.
- **Methods only (no specs)**: the status quo since decision 0410. It works,
  but it is long and unfamiliar. The specs lower to the same methods, so
  both remain.
- **Read the proxy environment by default**, as Go does: hidden input
  that changes where requests go, which Xo's capability design avoids.
  `with_proxy_for(http.proxy_from_env(os.env))` is one line.
- **A jar inside every Client by default**: cookies would leak between
  unrelated requests of a server that uses one client for many users. Go
  also has none by default.
- **slog's full Handler** (WithAttrs, WithGroup): its pre-rendering is an
  optimisation for allocation heavy loggers. It can be added later
  without breaking handlers.
