# Changelog

Xo stays on 0.x until the owner declares 1.0 (decision 0089). The toolchain
minor matches the spec draft; the full list of language changes for each
release is the "Changes in 0.N" section of `spec/core.md`.

## Unreleased
- `List.filter_map(f)` keeps f's Some values in order; std.md 2 says
  `Str.bytes()` makes no copy.

- `task.deadline(clock)` gives the nearest `with_timeout` deadline the
  current task works under, or None (decision 0430, Go's
  `ctx.Deadline()`); a spawned task keeps the one in force where it was
  spawned.

- std/http trailers (decision 0433): `BodyWriter.set_trailer`, `Response.with_trailer_names`, `BodyReader.trailers`, `ClientStream.trailers` and `Request.with_trailers(names, f)`, as net/http's TrailerPrefix and Request/Response.Trailer; the Go backend sends and reads them, the native backend has the API and its trailers stay empty until its wire part (Xo Lang 3) lands.

- `tls.TlsConn` has the same deadlines (decision 0430). A deadline set on
  the `Conn` before `tls.client` also limits the handshake.
- A linux/amd64 program that links with a C compiler (a `use c` source)
  built from a linux/arm64 machine fails with xo's message ("links with a
  C compiler ... build it on linux/amd64") and a --json report, as from any
  other machine; it handed the program's assembly to the arm64 C compiler,
  which printed assembler errors.
- `Conn` and `ServerConn` have Go's deadlines (decision 0430):
  `set_deadline`, `set_read_deadline` and `set_write_deadline(clock,
  t)`. A read or write past the deadline fails with `Timeout`, and the
  connection stays usable once the deadline moves.

- std/deque: a double ended queue with O(1) pushes and pops at both ends.

- `backoff.retry_notify` calls a function with each error and the next
  delay before it sleeps (cenkalti's RetryNotify).

- An effect variable shared by several callback parameters is the union
  of their arguments' effects (decision 0432): `both(pure_fn, clock_reader)`
  for `fn both[fx: Effects](a: fn() uses fx, b: fn() uses fx)` compiles
  whatever the argument order, and charges the caller the clock; before,
  the first argument fixed the set and the second was XO0701.

- On the Go backend, a stop now waits for WebSocket handlers (and proxied
  upgrades) as it waits for requests, as the native backends did:
  `serve` returned while a handler that had upgraded its connection
  still ran, since net/http's Shutdown does not track a hijacked
  connection (decision 0429).

- FIX (type checking): a function could perform effects it did not
  declare through a struct's effect parameter (a named pure function
  passed first fixed the call's effect variable, and a `Box[clock]`
  argument then passed unchecked); such programs now fail XO0701. A
  function parameter of function type, `if`/`match` branches that are
  functions and a closure's annotated parameter are checked the same
  way. Every program that did not leak an effect still checks.

- An edit build of a native program parses only the std declarations it
  needs (decision 0428): std's record keeps each file's declaration
  ranges, and the check parses one declaration where it needs its tree
  (a generic the SSA builder instantiates, a constant, a default, a
  type's fields) instead of the declaration's whole module. An HTTP
  server edit build on linux/arm64 checks in about 22.5 ms instead of
  27.5 (3 parses instead of 16); xo bench --ab, 150 rounds, the HTTP
  edit build from warm std caches: wall 107.5 to 98.6 ms, 0.917 (0.847
  to 0.976, better), and 0.92 (0.851 to 0.996) on a repeat.
  XO_STD_DECL_PARSE=off parses whole modules as before.
- `xo bench` takes a value flag's value as the next word too (`--rounds 10`, as `--rounds=10`); before, `--rounds 10` failed with "unknown flag --rounds".
- `xo run -o PATH` writes PATH every time: when the run cache served
  an unchanged program, the second run wrote no file there. `xo run -o`
  now builds without the run cache, as `xo test -o` does.
- std's separately compiled objects no longer depend on the directory a
  build ran in: `xo dev stdobj` built std in the caller's working
  directory, so its .debug_str comp_dir slots named that directory's
  view of the build (the temporary build path, or "."), two identical
  builds from different directories linked different bytes, and a deleted
  working directory failed the std cache (gotchas/stdobj-cwd-in-std-bytes.md).
- windows/amd64: a link from std's half (decision 0409) caches the PDB's std part beside the half, named for the half's id: an edit's PDB reads neither std.store.pdb nor std's parts (HTTP edit, Mac cross link: the PDB parts 13 to 4.6 ms, the link 42.5 to 34.9 ms), with the same PDB bytes as a full link. The PDB writer's std part (std's functions' records and line blocks) is cached the same way, std's CodeView types now coming first in a link through the store: with std's CodeView types: an HTTP server edit's PDB 14.5 to 2.3 ms, its link 34.9 to 18.4 ms (57.0 before std's half; Mac cross link).
- A separately compiled Windows build links std from a cached half: std's
  sections as the last full link laid them out, with only the
  relocations an edit can change, so an edit's link no longer reads std's
  38 MB store (an HTTP server's edit link 57 ms to 36 ms in a cross
  build); the output is the same bytes as a full link.
- `xo run` and `xo test` on Windows start the program before its PDB is
  written: the .exe names the PDB by a GUID that does not depend on the
  PDB, so the PDB is made while the program runs and xo waits for it
  before exiting (a small program's link 29 ms to 5 ms on a native
  windows/amd64 host); `xo build` is unchanged, and the files are the
  same bytes either way.
- A push (or any change) inside `Mutex.with(fn(var l) { ... })` no
  longer copies the protected list: the value moves into the closure on
  the native backends and LLVM. 200,000 pushes, one per `with`, took
  31.4G instructions and now take 120M; an error or fault in the
  closure still leaves the value unchanged.
- Native heap: blocks of 16 KiB and larger (size classes 68 to 72) are page aligned and carved
  from 4 MiB aligned page chunks whose first page is a table of their classes and list links
  (decision 0425; std/native/llrt alloc.xo, heapos.xo pchunk and map_aligned, cabi.xo,
  heapprof.xo), so a free block of those classes keeps no resident page for its prefix and links
  and heapos.xo scavenge gives back every page of it (classes 69 and up, as before); 0383's cut stops at the 8 KiB class (CUT_MAX_CLASS 67). Sanitizer and --race builds keep
  the prefixed layout (heapos.pages_on). Self build, cache off: peak memory footprint 7688 to 6855 MB; --ab wall 1.020 (0.999 to 1.030), CPU 1.024.
- Native Windows DNS record lookups (`lookup_mx`, `lookup_txt`,
  `lookup_srv`, `lookup_addr`) work: Xo's own DNS client asks the network
  adapters' DNS servers and reads the Windows hosts file, as Go's pure Go
  resolver does (Go's default asks the system resolver, so Windows' NRPT
  and DNS over HTTPS settings are not consulted; decision 0427); they
  returned "not supported" before.
- A native build that rebuilt nothing hits the binary cache again: the
  per-function cache's spill files (decision 0418) are written to the
  build directory's spill/ subdirectory, which the link's cache key
  skips; in the directory itself a cold build's spills made its key
  differ from an identical warm build's, so the second build linked
  again (TestMachoBinCache).
- The native back end's parts own their functions' SSA bodies and drop
  them as each part is emitted; the module keeps shells with body facts
  (decision 0424). The self build's heap past emit 9452 to 8180 MB and
  its peak 10764 to 9837 MB (footprint 7667 to 7287 MB); the output is
  the same.
- The back-end cache's pack segments hold one LZ4 block per entry and
  are written and read one entry at a time (decision 0418): a build's
  new entries leave the heap as each part finishes instead of at the
  store, and a read decodes only the entries it wants. The pack
  directory's name carries the format, so older xos never read the new
  segments; the first build after the upgrade stores its entries again.
  Self build with the cache on: the cold build's footprint 10428 to
  8371 MB (its cache-off build 7924 to 8350), the second build's
  5809 to 4807 MB and its CPU 21.4 to 9.7 s.
- std/crc32 on x86-64: IEEE (and gzip's and zip's CRC-32) folds 64 bytes
  a step with PCLMULQDQ (on 256 bit registers with VPCLMULQDQ), 0.12 of
  its time, Go's speed (was 8.08x); Castagnoli past 504 bytes runs three crc32q
  streams at once, 0.85x Go's time (was 1.74x).
- std/crc32 on arm64: Castagnoli past 504 bytes runs three crc32cx chains
  at once, 0.48 of its time (about twice Go's speed).

- Native DNS lookups keep /etc/resolv.conf and /etc/hosts in a process
  cache, rechecked every 5 seconds as Go's resolver does: an MX lookup
  runs 0.637 of its former instructions, 1.004x Go's.
- `use go`: Go's functional options (a variadic of an unnamed function type,
  `optFns ...func(*s3.Options)`) bind as a List of Xo functions, so AWS
  SDK v2's `s3.new_from_config` and every s3 client operation bind
  (decision 0422). Binding format 23.
- `use go`: a Go type that implements another package's interface its
  package's signatures name gets an `as_` method for it, so chi's router
  goes where Go takes net/http's Handler
  (`httptest.new_server(r.as_handler())`, decision 0416 addendum).
  Binding format 26.
- `use go`: the `as_` upcasts also cover the exported interfaces of the
  packages a package imports, named by its signatures or not, so AWS SDK
  v2's `credentials.new_static_credentials_provider(...)` goes to
  `aws.Config.set_credentials` as `as_credentials_provider()` (decision
  0416 addendum, widened). Binding format 28.
- `use go`: a package in a nested Go module (aws-sdk-go-v2's
  `service/s3`, its own module inside the root's path) no longer takes the
  enclosing module's `require go` pin: it binds at its own version, `xo
  check` asks for its pin (XO1319), and `xo mod tidy` adds it.
- `use go`: a Go channel given to Xo is a `go.Recv[T]` (decision 0421):
  `recv()`, `try_recv()` and `each(f)` receive what Go sends (fsnotify's
  events, go-redis's pub/sub messages); a waiting receive is a
  cancellation point. Binding format 25.
- Linux edit builds link faster again: a separately compiled ELF link reuses std's half of the last link, std's code, read only data and DWARF already relocated (decision 0420; XO_STD_HALF=off turns it off). The HTTP server's edit link went from 39.1 to 13.8 ms on linux arm64 (against the previous xo) and from 60.1 to 39.3 ms on linux amd64 (against the same layout without the half); the program's code now sits in its own segment after std's.
- darwin edit builds: a program that defines a name std defines weak no longer leaves a std half that a later program could link against without that definition (decision 0409).
- Linux edit builds link faster: a separately compiled ELF link no longer walks std's objects a second time and computes its undefined names once per step (decision 0420). Dropping the second walk took the HTTP server's edit link from 48.6 to 38.3 ms on linux arm64; the binary can keep a few more std bytes (xo: 0.004%).
- std/crc32: Castagnoli runs on arm64's CRC32CX (0.21 of its time, Go's speed);
  std/fnv's 32 bit sums 0.79 of their time.
- `use go`: methods of a named basic type bind as package functions
  taking the value first (`time.month_string(3)` is "March",
  `fs.file_mode_is_dir(m)`), decision 0416. Binding format 20.
- std/crc32 and std/fnv: Go's hash/crc32 (IEEE and Castagnoli,
  `checksum` and `update`) and hash/fnv (the 32, 64 and 128 bit FNV-1
  and FNV-1a sums, and `update` to continue one).
- `use go`: a struct field or package variable of a Go function type binds
  a setter taking an Xo function (`s.set_on_error(fn(msg: Str) { ... })`),
  converted as a callback parameter is, so hooks such as an error handler
  or a check function can be set from Xo. Binding format 17.
- `use go`: a Go pointer to a basic type binds as `T?`, a copy at the
  boundary (decision 0419): options structs with optional fields
  (`opts.set_limit(Some(10))`, `opts.name() ?? "none"`) bind; a parameter
  Go writes through stays skipped. Binding format 21.
- `use go`: go.uber.org/zap binds again: the binding named its generic
  constraint interface `ObjectMarshalerPtr[T]` (`interface{ *T;
  ObjectMarshaler }`) as a type, which Go rejects, so the whole package's
  build failed. A constraint interface, and a function constrained by one
  (zap's `object_values`, `objects`), is skipped now. Binding format 27.
- std/dns: MX, TXT, SRV and reverse (PTR) lookups over a Net, Go's
  LookupMX, LookupTXT, LookupSRV and LookupAddr, with `Resolver{server}`
  to ask one server (decision 0417). Native on Linux and macOS; Windows
  native lookups return `Io` for now.
- `use go`: a Go function value given to Xo (a result such as
  `fnval.adder(2)`, a function field or variable read back) is an Xo
  function `uses go` that calls Go. Binding format 22.
- `use go`: a callback or Go function value with several results returns
  them as an Xo tuple (`fn() -> (Int, Str) ! go.Error`). Binding format 24.
- std/crypto: `ed25519_sign` also takes Go's 64 byte private key (the
  seed, then its public key) and signs without deriving the public key:
  0.55 of the seed form's time, 0.65 of Go's.
- macOS arm64 edit builds link faster: a separately compiled build keeps std's laid-out half in the back-end cache and links only the program against it (decision 0409). The HTTP server's edit link went from 23.7 to 9.9 ms, fib's from 9.4 to 3.3 ms. The program's code is now in its own section (`__TEXT,_ptext`) after std's; lldb, dsymutil, codesign, Xo tracebacks and profiles read it as before. `XO_STD_HALF=off` turns it off; a `--layout` build keeps one `__text`.
- **Fix** (0.34.0): a linux/arm64 build assembled through a stock `cc` (GNU as, or clang with its default processor) failed on `crc32x` ("selected processor does not support" / "instruction requires: crc"): the assembly header declares `.arch_extension crc` beside crypto, aes and sha2.
- std/crypto: Ed25519 signing reduces its scalars with Go's Montgomery
  products instead of bit by bit: sign 0.66 and verify 0.89 of their
  time (sign 1.19x Go's, was 1.81x).
- BREAKING: prelude `FileInfo` gains `is_symlink: Bool` (default
  false): `read_dir` and `walk` report a symbolic link itself (Go's
  Lstat), `stat` follows it. A pattern that destructures FileInfo field
  by field adds the field; struct literals keep working.
- std/backoff: exponential backoff with jitter and `retry`, cenkalti/
  backoff v4's ExponentialBackOff and Retry, over the Clock and Rand it
  is given; `retryable` decides which errors are retried.
- `use go`: Go's `error` outside a final result binds as `go.Error?`
  (`errors.is`, `errors.as_`, error parameters and fields), and sentinel
  error variables as functions (`io.eof()`), so
  `errors.is(Some(e), Some(io.eof()))` matches a Go error (decision
  0416). Binding format 15.
- `use go`: a handle converts to an interface its Go type satisfies with
  `as_<interface>()` (`resp.body().as_reader()`, `buf.as_writer()`,
  `url.as_stringer()`; decision 0416), since Xo does not convert handles
  implicitly; gobind adapter errors quote the Go line. Binding format 14.
- `use go`: Xo functions implement Go interfaces (decision 0416): a
  binding declares a constructor per interface, one function parameter
  per method (`io.writer(write: fn(p: Bytes) -> Int ! go.Error { ... })`,
  `io.reader(read: ...)`, `http.handler(serve_http: ...)`,
  `sort.interface_(len: ..., less: ..., swap: ...)`), so `io.copy`,
  `http.handle` and `sort.sort` take Xo implementations. A Read method's
  function is given the buffer length and returns the bytes. Binding
  format 13.
- `use go` binds Go callbacks that return an error (decision 0416, step
  1): `func(int) error` is an Xo `fn(Int) -> () ! go.Error`, its `Err`
  Go's error (the Go error itself when it came from Go, so `errors.Is`
  works); `go.error(msg)` makes one (Go's `errors.New`). Binding
  format 12.
- `use go` binds generic Go functions (decision 0415, stage 1): a Go
  generic is an Xo generic with its constraints mapped (`any`: none,
  `comparable`: `Eq`, `cmp.Ordered`: `Ord`, `~[]E`: `List[E]`,
  `~map[K]V`: `Map[K, V]`), instantiated by Go at build; an Xo struct or
  List passes through an `any` type parameter untouched; a type argument
  Go must compare with no comparable Go value is `XO1106`. Go's `slices`
  and `maps` stay unbound (the prelude covers them). Generic types are
  stage 2. Bindings an earlier xo cached are made again (binding
  format 10).
- `use go` binds generic Go types (decision 0415, stage 2): a generic
  type is a generic handle (`lru.Cache[K, V]` is `Cache[K: Eq, V]`) with
  its methods and fields bound over its type parameters, and a function
  or method that takes or returns an instance (`*Cache[K, V]`) uses it.
  Instances over a type Go and Xo represent differently (`Int` for Go's
  `int`) and generic types of another package are skipped. Binding
  format 11.
- std/crypto: Ed25519 key generation, signing and verification on Go's
  edwards25519 algorithms (a fixed base table, a variable time double
  multiplication for verify): 0.27, 0.35 and 0.40 of their time.
- std/rate: a token bucket rate limiter, golang.org/x/time/rate's
  Limiter (`allow`, `reserve`, `wait`, `set_limit`, `every`), reading
  the Clock it is given, so tests run on the virtual clock.
- std/toml: `toml.encode(v)` writes a record or map as a TOML document in
  pelletier/go-toml v2's Marshal layout (literal strings where they can
  be, tables and arrays of tables after plain keys), so a program can
  write its configuration file back.
- std/toml: `toml.encode(v)`, a value as a TOML document in
  go-toml v2's Marshal layout (std survey gap 16).
- std/x509: creating certificates and requests and parsing CRLs, as Go's
  crypto/x509: `create_certificate` from a `Template` (self-signed or
  issued by a parent), `self_signed(r, hosts, now)` for development
  certificates (an ECDSA P-256 key, both as PEM), `create_request`,
  `parse_request`, `parse_revocation_list` and their signature checks;
  std/crypto `marshal_private_key` (PKCS #8). Ed25519 and RSA
  certificates and requests are byte for byte Go's (corpus x509create).
- prelude: `create_dir_all_mode(fs, dir, mode)`, create_dir_all with
  the directories it makes given an exact mode (Go's os.MkdirAll with a
  permission, without the umask).
- prelude: `write_atomic(fs, path, data)` replaces a file so a reader or
  a crash sees the old contents or the new, never a part (temporary file,
  sync, rename, directory sync), and `File.sync` (Go's File.Sync). The
  Go backend's MemFs rename keeps the file's mode.
- prelude: `walk_dir(fs, dir, visit)` with `WalkStep` (Continue,
  SkipDir, Stop): Go's filepath.WalkDir over any Fs, returning the
  visited entries.
- std/cli: `float_flag` and `Parsed.float` and `float_opt`, a floating
  point flag in `Float.parse` syntax (Go's flag.Float64).
- `use go` binds Go's `any` (decision 0412): a parameter or result of
  type `any` is a `go.Any`, an opaque Go value; `go.any(x)` makes one
  (the elements of a list literal for `...any` convert implicitly:
  `fmt.sprintf("%d %s", [n, s])`), `as_int()`, `as_str()`, `as_list()`,
  `as_map()` and `as_handle[T]()` read one back, and `go.out[T]()` is a pointer
  a Go function fills (`rows.scan`, `json.unmarshal`). A value with no Go
  value is `XO1106`. Bindings an earlier xo cached are made again
  (binding format 9).
- Fix (`use go`, a Windows tester's bug 18): a Go function that writes
  into a `[]byte` parameter (an `io.Reader`'s `Read(p)`, a
  `Decode(dst, src)`) is skipped, as `use c` skips mutable buffers: an
  Xo `Bytes` is a value, so what Go wrote was lost and the program saw
  zero bytes. Using one is `XO1102` with the reason; a function that
  also returns a `[]byte` (`AppendEncode`) is still bound.
- Fix (`use go`, bug 19): a function or method with a parameter of an
  unexported Go type (chi's `MethodNotAllowedHandler(...methodTyp)`,
  bluemonday's `urlRewriter` callback) is skipped; before, the binding
  named the unexported type and the program's Go build failed ("a code
  generator bug") even when the program never called it. Bindings an
  earlier xo cached are made again (binding format 8).
- Fix (`use go`, bug 20): a build whose bindings were made one at a
  time no longer fails on a Go module requirement the binding workspace
  left out (`go build` runs with `-mod=mod`); a Go module failure shows
  go's message and says that removing `.xo/gobind` binds every package
  again, instead of "a code generator bug".
- **Fix** (0.34.0): a program that uses `scope`, `spawn` or `Chan` failed to build with `--backend=wasm` ("the wasm backend does not support the call `rt.stack_switch`, in sched.fiber_direct"): `Chan` word operations (decision 0353) are native only now. TestSelfhostWasm fails when a golden becomes unsupported that is not in `internal/selfhost/testdata/wasm_unsupported.txt`.
- **Breaking** (decision 0410): the prelude's `Proc` interface gains
  `pid()` and `ppid()`, so a user type that implements `Proc` needs the
  two methods (interfaces have no default bodies, core 2.4); a wrapper
  forwards them to its inner `Proc`.
- **Breaking** (decision 0407): the prelude's `Proc` interface gains
  `notify(signals)`, `stop_notify(c)` and `request_stop()`, so a user
  type that implements `Proc` needs the three methods (interfaces have no
  default bodies, core 2.4); a wrapper forwards them to its inner `Proc`.
- **Breaking** (decision 0410): the prelude's `Env` interface gains `hostname()`, so a user type that implements `Env` needs the method (interfaces have no default bodies, core 2.4); a wrapper forwards it to its inner `Env`.
- macOS edit builds (separate compilation, the default): the binary no
  longer embeds std's DWARF. Xo's own tracebacks read segment __XODW, and
  lldb and dsymutil read std's full DWARF (variables included) through a
  debug map from std's part objects and the program's objects kept in the
  back-end cache (decision 0401). A small program's binary went from 2.03
  to 1.32 MB; the link time is unchanged. A binary copied elsewhere after
  the cache was pruned still prints tracebacks with lines; lldb then has no
  std names or variables, and `dsymutil ./prog` keeps them for good.
- Windows: a path given with backslashes (a path flag, a package
  argument, or a path variable such as XO_STD, XO_SEPCOMP_STORE or
  XO_DUMP_DIR) is read as the `/` form at entry, and a UNC path
  (`\\server\share\x`) keeps its share as its volume; before, some
  variables were taken as relative paths and a UNC root lost a slash.

- **Breaking** (decision 0406): the prelude's `Net` interface gains
  `dial_timeout(addr, d)` and `lookup_host(host)`, so a user type that
  implements `Net` (a fake or wrapper passed where a `Net` is expected)
  needs both methods; interfaces have no default bodies (core 2.4). A
  wrapper forwards them to its inner `Net`; a refusing fake returns
  `Err(NetErr.Denied(addr: ...))` as its `dial` does.
- std/template: Go's html/template as the html mode (decision 0408):
  `parse_html` escapes every action's output for the HTML, attribute,
  URL, JavaScript or CSS context it lands in; `template.html(s)`, `css`,
  `js`, `jsstr`, `url`, `attr` and `srcset` make trusted content that
  passes through in its own context; custom functions through
  `parse(funcs:)` (`TemplateFn`, Go's FuncMap). std/html (spec 39):
  `escape_string` and `unescape_string`, Go's html package.
- **Breaking** (decision 0408): `json.Value` gains the variant
  `Trusted(kind: Str, s: Str)`, content a template trusts in one context
  (`template.html(s)` and its siblings make it; `parse_value` and `decode`
  never do; `encode` writes the plain string). An exhaustive `match` on
  json.Value needs an arm: `xo fix` adds it as a hole (XO0301), or add a
  wildcard.
- Fix (miscompile): two modules whose paths end in the same name
  (app/a/util and app/b/util, used with `as`) had their functions,
  methods and constants share one name, so on the native and wasm
  backends a call to one ran the other's body (LLVM refused the build;
  the Go backend was right). Such a module is now named by its path.
- xo build: when std's store build fails (a std edit only the store's
  full check rejects), every separately compiled build prints one
  warning with the std file:line and builds the whole program; before,
  the fallback was silent. internal/arm64 TestStdStoreBuilds (run under
  -short too) builds the store from scratch.
- std/x509 (new, milestone 0.35): X.509 certificates as Go's
  crypto/x509: `parse` and `parse_pem` give a `Certificate` with its
  names, validity, key, SANs, basic constraints, key usages, key ids and
  every extension; `verify` builds and checks the chains to given roots
  at a given time (Go's Verify, with its error texts);
  `verify_hostname`, `check_signature_from` and `system_roots(fs)` (the
  roots TLS uses). std/tls: `TlsConn.peer_certificates()` and
  `roots_of(certs)`. Spec std.md 41; the corpus x509 compares every
  field, error and verifyTests row with Go's.
- Profile-guided code layout, stage 1 (decision 0403): `xo prof FILE
  --layout=xo.layout` reduces a CPU profile to a layout file, and `xo
  build` and `xo run` order the program's functions by it (the sampled
  ones first by weight, a callee after its caller) when the module root
  has `xo.layout` or `--layout=FILE` names one; `--layout=off` ignores it.
  The file is a source of the build (its hash is in the build info); a
  layout of another target is ignored with one line.
- std/crypto: authenticated encryption and key derivation (milestone
  0.35): `enum Aead { AesGcm, ChaCha20Poly1305 }`, `aead_key(alg, key)`,
  and on an `AeadKey` `seal_random` and `open_random` (a random nonce
  kept with the message, the way to seal unless a protocol counts
  nonces) and `seal` and `open` with an explicit nonce; `hkdf_extract`,
  `hkdf_expand` and `hkdf` over any `Hash` (Go's cipher.AEAD and
  crypto/hkdf; spec std.md 11). Vectors from Go's tests and Wycheproof
  in internal/stdtest/testdata/corpus/aead; bench/perf/aead against Go.
  A separately compiled program's link places the std atoms the file
  names first too (stage 1b, darwin), next to its own hot code.
- Windows: the path variables (XO_STD, XO_MODCACHE, XO_BACKEND_CACHE_DIR,
  the XO_SEPCOMP_STORE and _FACTS paths, XO_STD_STORE, the sysroots,
  XO_BUILD_DIR, XO_GOBIND, XO_GOCACHE and the binding directories) given
  as `C:\x` are taken as `C:/x` when xo starts, as the path flags already
  were (compiler/xo opts.xo path_env): a backslashed one was taken as a
  relative path.
- xo build for windows: the PDB GUID is made from the objects' content,
  not their cache keys, so a build with the cache off, or with separate
  compilation's facts given explicitly, has the same GUID as the default
  build.
- Windows: xo starts its child builds without an `env` program, which a
  Windows host has only under Git Bash: `xo dev stdcache` (std's
  variants for separate compilation) failed in 30 ms, and `--backend=go`
  (and `use go`) could not start the Go build from PowerShell or cmd
  (compiler/xo tools.xo env_command: the variables set on the command).
  Also on Windows: xo started from Git Bash finds itself without the
  `.exe` in argv[0], an `XO_BACKEND_CACHE_DIR` given as `C:\x` is taken
  as absolute, and `xo dev stdobj` takes `C:/x` as absolute (it prefixed
  the working directory to any path not starting with `/`).
- xo build: the first build of a program (which writes the std record)
  and every later one (which load it) have the same build id and bytes;
  the loader hashed empty sources for the std files it had not parsed
  into the build info's `<std>` entry (112 bytes of id strings differed,
  130 on Windows with the PDB GUID; code identical).
- xo build on x86 Linux: a build that assembles through cc (`XO_NOLIBC=0`,
  xotool's seed smoke build, the self build) failed with GNU as ("unknown
  pseudo-op: .arch_extension"): the linux assembly carried the arm64
  crypto directives on every architecture; they are arm64 only now.
  xotool's seed smoke message keeps the assembler's and linker's error
  lines (it said only "cc failed").
- xo: an edit more than a minute after the previous one no longer
  rebuilds std's cached entries twice: the backend cache's identity
  named a std facts entry by its modification time, which the cache's
  own touch moves (gotchas/cache-key-stamped-by-touched-mtime.md).
- Separate compilation (decision 0388) stage 2 is on by default: a
  `xo build` or `xo run` that compiles separately checks the program
  against std's records (XO_STD_EXPORT=1's path) without parsing std;
  XO_STD_EXPORT=0 turns it off, verify compares. The readiness list
  (the full cmd/xo and compiler/check suites, the native goldens on and
  under verify, the fixed point, a new user's first build) is in
  bench/results/sepcomp-edit-stage2-2026-10-28.md.
- xo: the host OS is the OS xo was built for (compiler/hostpath host),
  not a `uname -s` subprocess run four times a build; the run cache's
  miss check on a fib edit drops from 3 to 17 ms to 0 (8 interleaved
  edits against main, a loaded machine).
- Compiler: a loop that stores into a List field of a struct it carries
  (`h.chunks[off] = c`, `s.lanes[l] ^= w`) checks the list's copy on
  write once on the way into the loop, as a loop over a local list
  already did (compiler/ssa rc.xo hoist_reuse, decision 0208 addendum),
  and the loop keeps such a field's length in a register (compiler/arm64
  isel.xo): the store was a count check, a length load and an address
  computation each time. Instructions against the parent's xo (outputs
  equal): flate's hdec_init in its earlier field form (std at
  07a089cec), archive unzip 0.9567 (the hand-hoisted locals, 3de077d62:
  0.9509); SHA3-256 of 1 MiB 0.962, its quiet --ab wall 0.987 (0.981 to
  0.993); archive zip 0.994 (wall within noise); a 512-store fill
  through a struct field 2.403G to 2.299G instructions, a local list's
  2.298G.
- `xo build` and `xo run` compile separately by default on darwin and
  linux, arm64 and amd64, and windows/amd64 (decision 0388 stage 3): std's functions come from std's prebuilt
  variants in the backend cache (its store and facts), so an edit
  compiles and links only the program's own code. An llvm-fib edit
  takes 111.5 ms median against 229.0 with 0.33.0, the HTTP server's
  225.0 against 579.5 (bench/results/sepcomp-edit-fib-2026-10-28.md,
  sepcomp-edit-http-2026-10-28.md). The first build after std changes
  builds the whole program and std's variants in the background.
  --release, --race, a cc link, a program that uses C, and flags or
  variables that change std's code build the whole program as before;
  XO_SEPCOMP_DEFAULT=0 or XO_SEPCOMP=0 turns it off.
- Separate compilation (decision 0388) stage 2, opt in with
  XO_STD_EXPORT=1: the stdx entry carries a header (std's files and
  directories stamped by size and time, the content hashed within 2 s
  of recording) and std's module closure, so a build whose stamps hold
  makes std's modules from the record without reading a std file, and
  a separately compiled build parses only the std files holding a
  generic object (23 of 239), joining other trees on demand; a std body
  outside the facts that meets a pending tree rebuilds the front end
  with every tree. fib's edit checks in 18 ms instead of 33, the HTTP
  server's in 27 instead of 60; the edits take 0.08 and 0.16 s
  (bench/results/sepcomp-edit-stage2-2026-10-28.md).
- Compiler: reference counting insertion (compiler/ssa rc.xo) does the
  same work with fewer copies: the live leaf sets sort by one Int key
  (the paths' ranks, no string compared), the walk's lists are made with
  their capacity and the block's pre-code list is no longer copied on
  every edge, dead field reads go in one worklist pass,. A cold
  self build of xo retires 4.4 percent fewer instructions (2,157 to
  2,062 billion; insert_rc was 24 percent of the build's CPU, its
  largest single cost); output identity against the parent (202 compared,
  0 differing) and the fixed point hold
  (bench/results/insert-rc-2026-10-28.md).
- `xo bench --ab` without --max-load counts a round as loaded from the
  load before its first round plus 3 (at least 6), not a fixed 6, and
  prints that baseline in its load line: an idle machine at 4.4 to 5 lost
  most rounds of multi-threaded workloads.
- amd64: Xo's x86-64 assembler pads every jump (jmp, jcc, call, ret) and
  every cmp/test/add/sub/and/inc/dec a jcc macro-fuses with off 32 byte
  boundaries with NOPs before it, and 32 byte aligns code sections, so
  no jump runs from the legacy decoder on the Intel cores with the JCC
  erratum (Skylake through Comet Lake; decision 0398, compiler/macho
  asm_amd64.xo x86_jump_pad). sort1M on an i7-9750H ran 73 to 90 ms over
  32 placements of the same code (Go 67) depending only on where
  pdq_partition's loops landed; 69 to 72 ms over the same 32 placements (Go 67), every one under the unpadded best of 73; interp 160 to 128 ms
  (bench/results/sort1m-x86-2026-10-28.md). `assemble_elf_amd64_raw` and
  `assemble_macho_amd64_raw` keep clang's bytes for the encoder goldens.
- `xo vet --cost` notes a Str or Bytes slice in a loop read only by index,
  length or equality (XO0913) and a small List made and only read every
  iteration (XO0914); a repo test keeps std free of new ones
  (knowledgebase/std-performance-guide.md).
- Native backends: a loop that dispatches through a jump table (a `match` of
  8 or more arms, decision 0051) gets its latch, header and dispatch copied
  into each arm, so each arm ends in its own table branch (decision 0397,
  compiler/arm64 threadsw.xo; XO_PASSES=-thread_switch turns it off). The
  interp benchmark ran 204.5M to 281.4M cycles depending only on where its
  one `br` landed; threaded it runs 158.4M to 169.1M over the same 16
  placements (bench/results/interp-placement-2026-10-28.md). `XO_ARM64_PAD=K`
  puts K nops before every function label for such placement sweeps
  (decision 0075's knob, in the Xo emitter)
- `xo bench --ab` started by heavy.sh as a gate or a slot run exits 2 at once
  saying to run it as its own HEAVY_CLASS=measure job, where it waited for the
  quiet window that was waiting for it (a 28 minute deadlock, 2026-10-27)
- `use go` with an xo-gobind from another xo (an old one on PATH, or
  XO_GOBIND) is XO1101 naming the helper and both binding formats
  ("xo-gobind at PATH wrote binding format 5, this xo reads 7"), where every
  check said only that no current binding existed
- `use go` keeps working when the installed Go is newer than the one the
  release's xo-gobind was built with (decision 0393; a 0.33.0 tester with Go
  1.27 had every bind panic on its export data): a release carries the
  helper's Go source under lib/xo (go.mod, cmd/xo-gobind, internal/gobind,
  internal/gotool: 96 KB, about 30 KB more in the archive), and xo builds
  the helper with the installed Go once into
  `<user cache>/xo/gobind/<source>-<go version>/` and uses that build. A
  helper that still meets newer export data says so, naming both Go
  versions and the XO_GOBIND workaround, instead of panicking.
- The windows/amd64 linker builds each output section as bytes appended
  whole, with its relocations written over them, where it pushed every input
  byte into a list and wrote every output byte alone; the HTTP server's edit
  719 to 694 ms (the .exe and .pdb byte for byte the same)
- A windows/amd64 edit rebuild no longer rewrites the DWARF slots of every
  part's assembly text: the COFF parts are keyed and assembled with the
  placeholders and the real paths patched into each object, as the Mach-O
  parts are; the HTTP server's edit 795 to 719 ms (the .exe and .pdb byte
  for byte the same)
- `use go`: a Go type two bound packages use is one Xo type (decision 0392):
  `net.tcp_addr_from_addr_port` takes the `netip.AddrPort` netip makes, and
  golang.org/x/text/cases' functions take `language.Tag`, where each binding
  declared its own copy that nothing converted (Intel tester, 0.33.0, b14);
  bindings are regenerated (format 7)
- `==` on a Go value the package hands out by value (`netip.Addr`,
  `language.Tag`) compares by value as Go does, where two equal addresses
  were unequal (pointer identity); a struct the package passes only as `*T`
  (`*url.URL`) still compares by pointer, as Go's `==` on the pointers;
  bindings are regenerated (format 6) (Intel tester, 0.33.0, b15)
- `use go` on Windows finds the shipped bin/xo-gobind.exe (beside xo.exe, a
  release layout, or PATH split on `;`), where it looked for `xo-gobind`
  with no `.exe` and took only `/` paths as directories (Windows tester,
  0.33.0, bug 13)
- `xo mod tidy` on Windows no longer doubles `C:/` paths (`C:/w/C:/w/x`):
  its absolute path check, and six others in the CLI, use the host's path
  rules (Windows tester, 0.33.0, bug 15)
- `use go` of a Go module whose go.mod has a patch-level go line
  (golang.org/x/text: `go 1.26.0`) builds with the Go backend: the generated
  go.mod's go line is at least the binding workspace's, where `go 1.26`
  stopped go build with "updates to go.mod needed" (Intel tester, 0.33.0,
  b16)
- `xo mod tidy` in a fresh clone with a `require go` pin and no binding yet
  stops with XO1320 ("run `xo check` once, then `xo mod tidy` again")
  instead of exiting 0 without the pin's xo.lock lines (Intel tester,
  0.33.0, b17)
- `xo check --no-gc` (and a native build) of a `use go` program reports one
  XO1201 per Go package and file, at its `use go` line with the count of its
  uses, instead of one per Go call (a tester's program had 45)
- Breaking: a `uuid.Uuid` is two Ints (its 128 bits) instead of its canonical
  text (decision 0387; deprecated in 0.33.0 as vet's XO0912). Migration: `u.0`
  becomes `u.display()` (or `"${u}"`); `uuid.Uuid(s)` becomes `uuid.parse(s)?`;
  a pattern `uuid.Uuid(t)` becomes a binding and `t = u.display()`; `u.0 < v.0`
  becomes `u < v` (the bytes' order, as before). The checker names
  `display()` on `u.0`. Against google/uuid: parse 1.18, string 1.02, bytes
  1.75 times Go's time (from 1.23, 0.35 and 4.24).
- A windows/amd64 build writes its PDB faster: the DWARF reader pushes each
  variable and inlined call in place instead of copying its function out and
  back, keeps its abstract origins' records in a list indexed by a map, and
  the module stream is sized first and then written into one buffer, and an
  inline site's ranges are sorted only when out of order and its children
  found once, and a location list's expressions are read in place (one
  that repeats the entry before shares its pieces); the HTTP server's edit
  spends 66 ms in the PDB instead of 105
  (the PDB and the .exe byte for byte the same)
- A windows/amd64 edit rebuild assembles only the parts it changed (their
  objects kept in the back-end cache, the rest assembled on parallel tasks as
  on darwin and linux): the HTTP server's body edit 2783 to 1179 ms, llvm-fib's
  917 to 560 ms
- `xo build` and `xo run` skip inline `test` blocks, as they skip `_test.xo`
  files (decision 0389): a block is not type-checked by a build, so an error in
  it fails `xo check` and `xo test` but not `xo build`, and a build never loads
  std/testing (with its http, sql, log and json imports) for one. `xo test`,
  `xo check`, vet, fix and the language server check them as before.
- AES-GCM on amd64 is about 3.4 times faster: eight blocks a round with AES-NI
  and PCLMULQDQ, as Go's assembly does (16 KiB records on an i7-9750H: sealing
  1.09 and opening 1.02 times Go's time, from 3.67 and 3.49); an AES key's setup
  takes half the time with AES instructions (1.28 times Go's)
- ChaCha20 is about 4 times faster where it runs the portable code (amd64,
  wasm): its block keeps the state in locals instead of lists (ChaCha20-Poly1305
  on an i7-9750H 5.9 times Go's time, from 20.4)
- ChaCha20-Poly1305 on amd64 runs ChaCha20 and Poly1305 in one loop, with AVX2
  where the CPU has it (SSE otherwise) and Poly1305 in 64 bit limbs: sealing
  16 KiB records takes 1.25 times Go's time on an i7-9750H, from 3.10
- ChaCha20 is about 4 times faster where it runs the portable code (wasm): its
  block keeps the state in locals instead of lists; on amd64 it runs SSSE3, two
  blocks at a time, about 10 times faster than before (ChaCha20-Poly1305 on an
  i7-9750H 3.1 times Go's time, from 20.4)
- A native build no longer hashes the 2.1 MB of std/rt's files for its build
  information each time (a memo keyed by their sizes and modified times), and a
  test build's link cache key reuses each object's back-end cache key instead of
  hashing the objects again
- The first edit after a cold build of a mid-sized program (the HTTP server's 35
  parts) regenerates only the edited part instead of all of them: its parts
  are kept from the first build when they are under 128 MiB in all (before, only
  a module of fewer than 16 parts).
- `xo build` and `xo run` load std/testing only when a program file has a test
  block (it imports http, sql, log and json, so every build parsed them): an
  llvm-fib edit rebuild 283 to 244 ms. `xo check`, `xo test`, vet and the language
  server load it as before, and a build still type-checks inline test blocks.
- `--backend=amd64` edit rebuilds for windows reuse every unchanged function
  and part through the back-end cache too, as darwin and linux builds have since
  2026-10-26 (each cache entry carries the Win64 thunks its functions call through)
- An edit rebuild is a few ms faster: the checker's syntax trees and tables are
  released on a task beside the passes after the SSA is built, not on the build's
  thread. The landings and the cleanup per function go through the back-end
  cache (decision 0382, default on; XO_CHAIN_KEYS=0 turns it off), keyed by the
  front end entry each body came from and what its replay set, and the reference
  counting's keys are chained from each body's cleaned-up form: quiet --ab, one
  xo with it off and on, the HTTP server's edit 603.5 to 580.8 ms, llvm-fib's
  263.8 to 259.8 ms.
- `--backend=amd64` edit rebuilds on darwin and linux reuse every unchanged
  function and part, as arm64 builds do (the per-function and part back-end
  cache, lazy inline-cache decode and the deferred inline verification): a body
  edit of the HTTP server for linux/amd64 rebuilds in 676 ms instead of
  1244 ms (arm64: 596 ms), llvm-fib in 290 ms instead of 465 ms, and each
  edited binary is byte for byte a cold build's
  (bench/results/amd64-edit-cache-2026-10-26.md). windows/amd64 keeps building
  every function on an edit for now.
- An unchanged `xo build`, `xo run` or `xo test` (a run cache hit) checks its
  inputs on tasks: the program's own files first in order, so an edit still misses
  at its first file, then the std files (hundreds) in parallel.
- `--race` no longer reports a data race between tasks that only reused one stack
  (the Windows tester's report on v0.32.0: tasks looping only over
  `m.with(fn(var c: Map[Int, Int]) { c[k] = v })` reported WARNING: DATA RACE
  every run; a spawn site that takes a Mutex in a loop runs its tasks one after
  another on one fiber stack, and the finished task's accesses to the
  same stack slot were still in ThreadSanitizer's shadow). A task now starts with
  the shadow of the dead stack below it reset (sanhook.xo dead_stack_reset).
  Checked on darwin and linux/arm64 (the arm64 backend with clang's
  ThreadSanitizer) and on windows/amd64 (CI run 40418639701: 120 of 120 runs,
  the planted race still reported in all 60 of its runs).
- darwin/amd64 builds empty the functions nothing reaches by default, as every
  other native target already did (the hollow pass, decision 0371; it was
  `XO_PASSES=+hollow` until an x86_64 Mach-O binary of it had run: the Intel
  tester's v0.32.0 round ran a hollowed web server through CRUD, 404, load and
  SIGINT). `XO_PASSES=-hollow` turns it off.
- `xo query api std/json` lists std/json's surface outside a module too (it
  printed an empty list: the module was not loaded, and std's declarations
  carry no `pub`); `xo query deps` and `imports` of a std module say std
  packages are not followed instead of "no package among the checked
  modules".
- Every native build is about 10 ms faster to start: the C toolchain's identity
  (`cc --version` and a stat(1) of it, which the run cache's key reads) is
  remembered across builds in the backend cache, keyed by the size and
  modified time of the tool and, on macOS, of the clang xcode-select points to.
- `xo def`, `xo refs`, `xo symbols`, `xo doc NAME` and `xo type` print
  compact text by default, a record a line, also when standard output is
  not a terminal (decision 0386; they printed JSON on one line there:
  `xo symbols --dir=compiler/ssa` was one 540 KB line, now 1,935 lines
  and 173 KB). `xo symbols [--dir=D] NAME` filters by name (`check_*`),
  `--limit=N` cuts symbols and refs, `--doc` adds symbols' doc comments,
  and `--json` prints the same JSON shape broken over lines. Scripts
  that read the JSON pass `--json`.
- An edit rebuild hollows only the changed parts on linux and on amd64 too (it was
  darwin/arm64 only): an ELF part's facts also key on every part's extra labels
  (hextra/ entries). Per edit, hollow on linux/amd64 went from 196 to 86 ms for
  the HTTP server and 51 to 30 ms for llvm-fib; each edit's binary is the file a
  cold build writes on linux/arm64, linux/amd64, darwin/arm64 and darwin/amd64.
  The store log now counts the hfacts and hextra entries a build writes.
- `xo fix` adds a missing `require go` pin once: each fix round's check now
  reads the xo.mod the previous round edited, where it read the file on
  disk and added the pin again every round (25 copies in one run; Intel
  tester b12).
- `xo build -h` lists `--pack-lists` and `--inline-str`; `--inline-str` on a
  build other than native arm64 warns that it has no effect (decision 0370).
- JSON: a number out of Float's or Float32's range fails with Go's words,
  `cannot unmarshal number 1e400 into Float`, in `decode` and `parse_value`
  on every backend (was `expected number`).
- Native amd64 packs lists of sized integers as arm64 and LLVM do (decision 0369, on
  by default): it kept every `UInt8`, `Int16`, `UInt32`, `Rune` or `Float32` element
  in 8 bytes. A 20M `List[UInt8]` on darwin/amd64 peaks at 44 MB instead of 175
  (field bug b13). The x86 loads of signed narrow elements and the equality of packed
  lists, which the change exposed, are fixed with it.
- The back-end cache stores its entries and pack segments LZ4 compressed
  (decision 0365; a port of the LZ4 block format in xo itself,
  std/native/llrt/lz4.xo): about a third of the bytes written (a warm
  llvm-fib's cache 12.1 MB instead of 34.1), for 9 ms more on a warm
  build of llvm-fib with whole-build reuse off and 8 ms on an edit
  rebuild. Compressed files have their own names (`z<KEY>`, `z-<ID>`), so
  older xos miss them rather than misread them. `XO_CACHE_COMPRESS=off`
  stores plain files.
- `xo def`, `xo doc` and `xo symbols` by name, and `xo query` def, api,
  imports and deps, parse and check no function body, and `xo query
  callers` only the bodies that spell the name: at the xo root `xo def`
  takes 0.38 s and 0.38 GB instead of 1.4 s and 2.0 GB, `xo doc` 0.53 s
  and 0.49 GB instead of 2.3 s and 2.7 GB, `xo query callers` 0.38 s
  instead of 1.6 s; in compiler/ 0.24 to 0.37 s instead of 1.1 to 2.0 s,
  with the same answers. A position's lookup (`xo type`, `xo query type`)
  and `xo query effects` still check every body. `XO_LOOKUP_EAGER=1`
  checks every module and body for every lookup.
- A native build of fewer than 4,096 functions is split into parts four times
  smaller, so an edit regenerates and reassembles less: the HTTP server bench's edit
  rebuild 585 ms to 520, its cold build and size unchanged; xo itself keeps its
  parts (smaller ones made its edits 10% slower).
- A split darwin build hollows and assembles part by part through the back-end cache:
  an edit rescans, hollows and assembles only the parts it changed, and the
  undefined-symbol check combines kept per-part facts (`phase hollow` now includes
  assembling the changed parts). Changing a reachable function's body: llvm-fib's
  rebuild about 330 ms to 280, the HTTP server bench's 740 to 590; the binaries are
  byte for byte a cold build's.
- The whole-build cache records the program's own files ahead of std's, so an edit's
  miss stops at the edited file instead of first hashing every std file: an edit
  rebuild of llvm-fib or conc-mutex 260 ms to 250.
- `xo refs` at a root also leaves unparsed and unchecked each function
  body that spells none of the names looked for: at the xo root with
  `--testdata`, `trim_prefix` (350 references) takes 1.35 s and 1.0 GB
  instead of 15.5 s and 5.8 GB, `json.decode` 0.8 s and 0.7 GB, with the
  same references (`XO_REFS_SPELL=off` checks every body).
- `xo check`, `xo vet` and `xo query` over directories holding two modules
  with one module path (copies of a program) check each of them on its own:
  they read only the first module's files under the path, so an error in
  another copy passed the check.
- `xo def`, `xo refs` and `xo symbols` at a root holding two modules with
  one module path check the first and name the others (JSON `same_path`, a
  note) instead of silently reading only the first's files, which lost
  the others' references (the xo checkout's `example.com/notes`, three
  copies).
- `xo refs` at a workspace or repository root checks only the modules whose
  text spells the name looked for (its last segment, as a whole
  identifier): at the xo root a rare name's query takes 0.9 s and 0.9 GB
  instead of 4.4 s and 3.1 GB (with `--testdata`, 1.4 s and 1.2 GB instead
  of 6.5 s and 3.2 GB), with the same references; it also no longer loses
  the references of a module whose path another module under the root
  shares (`example.com/notes` three times in the xo checkout).
  `XO_REFS_SPELL=off` checks every module.
- An edit rebuild whose every part comes from the back-end cache skips hollowing,
  the undefined-symbol check and assembling (since the entry above, per part):
  llvm-fib's and conc-mutex's edit rebuilds take 270 ms instead of 320.
- An edit rebuild whose edit adds no reachable function no longer decodes every
  cached inlining result it does not verify: `inline.verify` about 18 ms to 7.
  With the entry above, llvm-fib's and conc-mutex's edit rebuilds take 260 ms
  instead of 320.
- `xo bench --ab` with no `--max-load` now marks a round loaded when a side
  starts at a 1 minute load of 6 or more, and leaves it out of the
  statistics unless `--keep-loaded` (it does not wait for the load, as
  `--max-load` does). Before, without `--max-load` no round was ever
  loaded, and a run at a steady load of 20 reported "0 loaded". The summary
  gives each side's median load and "loaded: N of M rounds", with a warning
  when the machine was busy through the run (`--json`: `load_note`).
- A front end cache entry of a closure that made a Map type's helper now
  replays in a program where another body made the helper first; such a
  build printed "the front end cache entry ... did not replay" and was
  made again without the cache. `XO_FE_STATS=1` also prints the counts
  of a build the cache dropped (`fe dropped ...`), which name the miss.
- `xo dev edges` exits 1 with the reason when its program does not check: a
  `use go` or `use c` package with no binding stopped the check before names
  resolve and the command wrote only the standard library's references,
  exiting 0, so a per-directory sweep read it as a program with no references
  of its own. The reason names the package and the `.xo/gobind` file `xo
  bind` or a Go check writes.
- The Go backend builds a call of a std function whose default value is a
  call or another expression (`metrics.Registry.histogram`'s `buckets =
  latency_buckets()`), evaluated at each call as on the native backends; it
  was "unsupported std default value".
- arm64: a retain before a call that only borrows the value moves into the paths after
  it, where a release meets it: a list's Str passed to `Duration.parse`, which keeps it
  only in its error, is no longer counted up and down on every successful parse
  (dur_parse 0.94 of the wall time, 0.979 of the instructions).
- arm64: a match arm whose variant holds no heap value drops nothing: the `Ok(d)` arm
  of a `Result[Duration, DurationErr]` called the Result's drop helper on every parse.
  `Duration.parse` loops 0.973 of the instructions, `cut_prefix` and `cut_suffix` 0.80
  and 0.79, `cut` 0.88, JSON ints 0.97.
- `xo bench --ab` on macOS reports cycles, cycles per op and instructions
  per cycle (ipc, higher is better) beside instructions, each with its
  B/A ratio and interval, from `time -l`'s "cycles elapsed" line, so a
  change that runs fewer instructions but more slowly (a lower ipc)
  shows why; on Linux, whose GNU time has neither counter, the text output
  says `n/a` under the table instead of leaving the rows out silently.
- `xo refs A B C` answers several names from one check, in order (JSON
  `{"results": [...]}`, a name with no answer an entry with its error and
  exit status 1), so a sweep pays the program and std check once.
- `xo refs` at a repository root says which `testdata` directories it
  did not search (`"testdata_skipped"`, a `note: not checked` line), and
  `--testdata` searches them, so the program goldens' calls are found; a
  module there whose requirement is not in the module cache is left out
  and named instead of stopping the check. Refs of a std name checks
  only the std and runtime files that spell the name: a rare name's
  query takes 0.08 to 0.4 s instead of about 1.2 s, with the same
  references.
- `var (a, b) = f()`, `var Point{x, y} = p`: `var` takes the irrefutable
  patterns `let` takes, and every name it binds is mutable (decision 0384,
  the owner, 2026-10-26). It was a parse error and a "name not found" for
  every later use of each name; error `XO0179`, which briefly reported it
  with a fix to `let`, is retired. A `let` pattern's name that is assigned
  gets the fix that writes `var`.
- arm64: a compare, add or sub with an immediate of 12 bits shifted left by 12 (0x2000,
  0x3000, 0x10000) takes the immediate (`cmp x, #2, lsl #12`) instead of a constant in
  a register: unicode runecase 0.983 of the instructions, classify and quote 0.995.
- A Map type's cold put paths share one body (decision 0372's stage 0, arm64): the
  lists' put is one cold `xo.maph.putbody`, called by `xo.maph.putrest` (which keeps
  the shared mark) and by a forked hybrid map's replay, so the second copy of the
  body and its `xo.maph.listput` wrapper are gone. A Map type is 848 B smaller in an
  8 type program (2.6% of the text), and a CRUD server 3,976 B (0.28%); copymut runs
  in 0.993 of the wall time and putget in 0.961, with instructions at parity.
- Debug information changes no inlining: in a build that has the DWARF readers, the inliner decides
  for a reader, and for a function only a reader reaches, as a build without debug information would
  (no call site counted inside one, one instruction, never inlined), so both builds inline alike; a
  build without debug information is unchanged, byte for byte. TestDebugInfoSameCode now builds both
  variants with inlining on, so the same-code promise covers inlining.

- arm64: a function's result bounded by one of its parameters is known to its callers (decision
  0376): `i < n` for an n from `strings.space_start(b, lo, b.len())` needs no bounds check.
  compiler/xo itself has 4% fewer bytes index checks and 2.5% fewer list index checks.
- A `std/NAME` module's test file is checked and its calls are built, the prelude's
  included: `xo test --std-module=prelude` reported XO1201 "support is planned" for
  every list literal and every call on a list in it, and XO1201 "the std function
  `NAME.f`" for a call of the module's own function, though the same code compiled in
  a program; only a `std/native/NAME` module's tests worked (decision 0262).
- `xo build --inline-str` (experimental, decision 0370, arm64 and amd64, default off): a Str
  of up to 11 bytes keeps its text in its two words. A list of a million short keys takes
  0.38 of the memory, a `Map[Str, Int]` 0.69, its sort 0.62 (wall 0.77, 0.84 and 0.65);
  Str heavy benches run 1 to 11% more instructions and a release server is 8.4% larger.
- Release binaries keep the function symbols a traceback reads and drop the rest
  (decision 0371's second addendum): a release hello world is 213,337 bytes instead
  of 216,777 and a CRUD server 1.4% smaller, with the same tracebacks. A build with
  no function symbols left (`-s`) now prints
  `traceback: (no function symbols: built with -s, or stripped)` instead of naming
  every frame after the image header.
- Lists of `Int8` to `UInt32`, `Rune` and `Float32` take 1, 2 or 4 bytes an element on the
  native and LLVM backends (decision 0369; `--pack-lists=false` turns it off): 10 million
  `UInt32` and 10 million `UInt8` in lists take 0.27 of the memory and run in 0.53 of the
  time.
- Faster Unicode classification and text splitting on darwin/arm64 and linux/arm64:
  the native inliner counts 23 more runtime operations as leaf instructions, and a
  call in tail position stays a branch instead of pushing its caller over the inline
  limit (decision 0184 addendum): `unicode.is_*` class tests 0.80 of the wall time,
  `map_runes` 0.97, `split` 0.98; xo itself 0.46% larger.
- linux/arm64 binaries are smaller too: the code and data of functions nothing calls
  go, as on darwin/arm64 (decision 0371's ELF addendum): a release hello world is
  283,672 bytes instead of 379,320 (-25%), a CRUD server 3.8% smaller.
- darwin/arm64 binaries also drop the fault-location strings and string literal
  objects that only removed functions used (decision 0371's addendum): a release
  hello world is 299,289 bytes instead of 332,314 (-9.9%), a CRUD server 1.6% smaller.
- Programs with several Map types are smaller on the native and LLVM backends: a
  Map's key-side code is generated once per key type, its values in a list or value
  log of their own (decision 0372). Eight `Map[Str, V]` types take 388,280 bytes of
  __text instead of 444,252; a CRUD server 1.2% less. Map speed and memory are
  unchanged within 3% (quiet `--ab`); `XO_PASSES=-map_core` turns off the lists'
  half.
- A Map used with literal keys (`m["id"] = ...`) no longer gets copies of its put
  and lookup code per key on the native and LLVM backends (decision 0368's addendum:
  a literal clones a callee only when the callee compares it and does not hash or
  store it): a release CRUD server's __text goes from 1,519,088 to 1,467,848 bytes
  (-3.4%), with the closure and compare specializations as fast as before.
- darwin/arm64 binaries no longer carry the code of functions nothing calls that the
  earlier dead function removal had to keep (unused C ABI exports, the helpers only
  they reach): each becomes one `brk #0x484f` after code generation (decision 0371,
  `XO_PASSES=-hollow` turns it off). A release hello world's __text goes from
  235,612 to 164,896 bytes (-30%), a CRUD server's by 3%.
- A keyword where a pattern binds a name (`Some(pub) =>`) is its parse error alone:
  the bindings only the unparsed part read are no longer "never used" (they came
  first and named the wrong lines), a keyword read as a name says so (XO0160, not
  "declarations are only allowed at top level"), and a block ending in a statement
  that did not parse adds no type mismatch.
- A spawned task that loops over a Mutex (every call in its loop is a `Mutex.with`,
  `with_read` or an Atomic method) starts its spawn site small, so a one-shot burst
  of lock-bound tasks runs on the spawner instead of waking a worker per spawn
  (decision 0363): conc `rwmutex` 0.25 and `mutex` 0.65 of the wall time, a
  read-mostly lock 0.57; worker pools, CRUD and the other conc rows within noise.
- JSON text nested deeper than 10,000 levels is an error on the native backends too
  (`invalid character '[' exceeded max depth`, as Go's and the Go backend's).
- LLVM builds write their IR and split modules in a directory of their own and
  remove it after the link (they shared one per split setting, like the native
  path did, and left every `prog*.ll` until the prune); `XO_KEEP_ASM=1` keeps them.
- The build directories (`<user cache>/xo/build`) thin themselves: every file but the
  programs in a `bin/` goes two hours after its last use, and the cap is 6 GiB
  (was 20): 837 directories held 12 GB, mostly part objects, with nothing removed.
- Concurrent `xo build` of one program no longer share the files they link: each
  build writes its assembly and part objects in its own directory under the
  build directory (removed after the link; `XO_KEEP_ASM=1` keeps them in
  place). Before, one build could truncate a part object another was linking
  ("not a 64 bit Mach-O file").
- Decoding a `Time` from JSON on the native backends parses the text in place
  instead of copying each time string first: a list of times decodes in 0.90 of
  the wall time.
- std/json on the native backends decodes and encodes the sized integers
  (Int8, Int16, Int32, UInt8, UInt16, UInt32, UInt64) and Float32; a struct with
  one of them used to fail to build there ("converting `Int` to `Int16`"), with
  the Go backend's range errors and a UInt64 up to 18446744073709551615.
- `Time.parse_rfc3339` and a `Time` decoded from JSON refuse a time outside
  the range of `Time` (1677-09-21 to 2262-04-11) with `None` or the decode
  error "expected RFC 3339 time string", where they returned a wrapped time
  (2300-01-01 came back in the 1700s); native and Go backends agree (decision
  0364).
- The back-end cache's prune removes the empty directories a build target
  leaves (one per build kind on its first build, and most targets are built
  once or twice) after an hour unused instead of a week: one of the 256 shards
  held 1,467 of them on 2026-10-24 (`Policy.empty_idle`, sliced runs).
- `XO_COLD_PACK` is on by default (owner, 2026-10-24; `XO_COLD_PACK=off` writes
  loose files as before). A build's new cache entries of a kind go to the
  shared pack as one segment, fn included (fn_cache_load dropped the shared
  pack fields of its Pack, so fn had been written loose and copied into every
  target's pack), and a build target's pack no longer copies the entries the
  shared pack served. A compiler self-build's cache after three builds: 3,164 MB
  and about 48,000 files before, about 1,800 MB and 126 files now; a cold
  http_server build 2.3 s against 6.7 s on the Mac. (Decision 0351 addendum.)
- The back-end cache's cap counts disk, not bytes: each file takes whole
  4 KiB blocks (prune.Policy.block), so the 16 GiB cap means 16 GiB
  allocated. The cache held about 20 GB on disk under a 16 GiB cap
  because its many small entries (a few hundred bytes each) take a block
  each; the first prune of a full cache after this removes the
  difference (about 2 to 3 GB on the 2026-10-24 cache), least recently
  used first. `XO_BACKEND_CACHE_MAX` is counted the same way.
- A build no longer tries to open cache entries in a shard the cache does
  not have: it lists the kind's shard directories by name only (new
  std/internal dir_names: no stat of any entry, which fs.read_dir does, and
  which made the first form of this, 284b87da9, time out against a 19 GB
  cache). A cold build in an empty cache opens next to nothing; a cache
  that has every shard (the user cache: 23 GB, 256 shards of about 2,000
  entries) opens as before, at no cost: listing each shard to open only its
  keys was measured and dropped (7 ms a shard, 1.8 s a build).

- Every released xo is built by its platform's native backend (decision
  0362): linux/amd64 joins (it was the Go backend; a static binary with
  no C library, which rebuilds itself to the same bytes), and seeds are
  cross built natively. Development builds of xo on x86-64 hosts are
  native too: about 4 times faster to build than LLVM -O0, and the xo
  they make builds 2.5 times faster.

- Windows: the checkout's `xo.exe` (scripts/xobuild) is built natively
  (`--backend=amd64`, as the release's), not with the Go backend, whose
  build of xo spent 540 to 600 s in each program's link (CI's runner);
  builds there are 9.5 to 18 s again (run 40057967924). The CI script
  suite takes `--seed-archive NAME SHA256` to seed xobuild from a
  release archive.

- `xo build` and `xo run` of an unchanged program reuse its binary
  through the run cache `xo test` uses, printing what the build printed
  (a warm build of a small HTTP server 0.54 s to 0.039 s, hello 0.28 s
  to 0.040 s); on by default since 2026-10-23 (the owner's decision),
  `XO_WHOLE_BUILD=off` turns it off (decision 0360). `xo
  test` of a program with a warning is now cached too, the warning
  printed on each hit (it was never stored before). The entries list
  what the build actually read, looked for (an `xo.work` above the
  module, a vendor directory) and the environment variables it read;
  `XO_BACKEND_CACHE=verify` rebuilds on a hit and reports a difference
  as XO1512.

- 0.27.0 (2026-10-23): breaking, a third-party `use go` needs a `require
  go` pin (XO1319, decision 0356) and std's errors print plain messages
  (0358); `xo check` about 2.5x faster warm and `--support` 2.3 to 2.6x;
  warm builds with the module check cache, the shared pack and lazy body
  parsing on by default; `--release` as a preset of per-setting flags
  (0354), `--debug-info=split` (0355), build information in every binary
  (`xo version -m`); native `--seed` and `--explore` (0350); one-word
  channel sends (0353); an x86_64 miscompile of stack-passed parameters
  and the 0.26.0 field reports' fixes (Intel Mac, Windows). Known gaps:
  spawning many short tasks, sorting Strs (1.18x Go), a contended
  RWMutex. The entries below, down to 0.26.0's, are in it.
- xo warns when it is older than the std it runs with (`std/VERSION`):
  `xo: warning: this xo (0.24.0) is older than std at DIR (0.27.0);
  rebuild it`, so a stale `bin/xo` on PATH is noticed.
- `xo doc NAME` and `xo def NAME` run outside a module (a scratch or home
  directory too large to check, or one with no `.xo` file) answer a std or
  prelude name from std, with a note, instead of failing; `--dir` keeps
  the refusal. A name from another language (`String`, `str`, `int`,
  `dict`, `Vec`, `nil`, ...) gets a note naming Xo's type (`Str`, `Int`,
  `Map`, `List`, `Option`).
- `xo check --support` is about 2.5 times faster on a warm run
  (compiler/xo 3.7 s to 1.6 s): it keeps the module check cache and
  builds through the front end cache (decision 0351 addendum).

- darwin programs linked through cc (`use c`, `--ld=cc`) are
  reproducible: two builds of the same sources are byte-identical (ld64
  wrote each object's modification time into the debug map, which the
  UUID and code signature hash; the link now runs with ZERO_AR_DATE=1).

- `xo check` is about 2.5 times faster on a warm run (compiler/xo 1.8 s
  to 0.67 s, a small server 0.59 s to 0.26 s) and peaks a third lower in
  memory: it skips a module unchanged since its last check, replaying
  its diagnostics and warnings, and checks the native runtime's bodies
  only as a build does (decision 0351 addendum). The output is
  unchanged; `XO_CHK_CACHE=off` checks everything.

- `xo build` and `xo run` parse a std or runtime function body only
  when the checker needs it (decision 0351 stage 2, on by default;
  `XO_LAZY_PARSE=off` parses every body): the output is identical, and a
  small program's warm build peaks about 40% lower in memory.

- Native binaries carry their build information, as Go backend ones
  always did, now with every effective setting of `--release`'s preset
  (decision 0354): `xo version -m BINARY` prints it (the xo, backend,
  target, ID, settings, stamps and sources' hashes), a native crash
  report adds a `build:` line, and `xo replay` rebuilds with the
  recorded settings.

- xo.lock records each pinned Go module's go.sum hashes (`go <module>
  <version> h1:...` and its `/go.mod` line; decision 0356): a changed
  module is `XO1304`, a missing line `XO1305` (run `xo mod tidy`).

- **Breaking** (decision 0356): a `use go` of a package outside Go's
  standard library needs its Go module pinned with `require go <module>
  <version>` in xo.mod; without it the use is `XO1319`. Builds used to
  follow the module's latest version, recorded only in the gitignored
  `.xo/`, so they were not reproducible. `xo fix` adds the line with
  the version the binding resolved, and `xo mod tidy` adds missing
  pins. Go's standard library packages need nothing.

- `"${r?:?}"` (the try operator, then the Debug form) parses; it was
  read as Kotlin's elvis `?:` (XO0177 and XO0160), and only
  `"${(r?):?}"` worked.
- `xo query type --dir=DIR FILE:LINE:COL` (and `xo mcp`'s query tool
  with `dir`) finds a relative FILE under DIR; it looked only under the
  working directory.
- `xo serve` with a socket path over the kernel's limit (103 bytes on
  macOS, 107 on Linux) says so and names `--socket` and
  `XO_SERVE_SOCKET`, instead of a bare "bind: invalid argument".
- Spec 6.1 describes the `[module@version]` tag on a dependency's log
  records (decision 0173); it still said "[later]".
- `XO_TRACEBACK=locals` shows a list's first elements for any element
  type (`names = ["a-2", "b"]`; a `List[Str]` or a list of structs
  showed only its length), shows `os = Os` instead of `<unavailable>`,
  and leaves out a local that is not set yet at the fault (`let r =
  f()` faulting in `f`).

- `--debug-info=split` (decision 0355) keeps the DWARF beside the
  program instead of in it: PROG.dSYM on darwin, PROG.debug on linux
  (a GNU build id in both and a `.gnu_debuglink` in the program, as
  `objcopy --only-keep-debug` makes). Tracebacks, `xo dev symbolize`,
  lldb, gdb, llvm-symbolizer and addr2line find it there.
- `xo dev symbolize` no longer prints `NAME+0x0x...` for a pc no table
  names.

- `--release` is a preset (decision 0354): each of its settings has a
  flag of its own in any build (`xo build`, `run`, `test`) that wins
  over the preset whatever the order: `--debug-contracts`, `--dbg`,
  `--error-args`, `--debug-info`, `--profile-support`,
  `--trace-support` (`on|off`, `true|false`, a bare flag is on) and
  `--opt=O0..O3` (LLVM; a usage error on arm64 and amd64).
  `--error-args=off` no longer needs `--release`, and native frames
  print `(...)` for its parameters as the Go backend does.
  `--debug-info=split` is a usage error until decision 0355 lands.
- Every `DIR/...` walk skips `vendor/` directories, with or without
  `modules.txt`, as the go command does (owner, 2026-10-23); vendored
  code is checked only when its directory is named.

- `xo fmt ./...` and `xo parse ./...` skip directories named `testdata`
  and names starting with `.` or `_`, as `check`, `vet`, `test`, `build`
  and `fix` already did, so a deliberately broken directory stays out of
  every `./...` (decision 0210). `fmt ./...` rewrote files under
  `testdata/`.

- A parse error in a parameter list (`r: Int ! E`) no longer cascades:
  the function's parameters and result take any type, so its body and
  its callers report nothing more.
- `xo fix` removes a `?` after an expression that cannot fail (XO0401);
  the XO0601 fix left it.
- `--targets` no longer says "the wasm backend: the wasm backend".
- `scripts/release-verify.sh` accepts Windows ThreadSanitizer's
  "WARNING: DATA RACE"; decision 0273 notes that Windows `--race` works.
- `--race` with `--backend=llvm` links xo's own ThreadSanitizer runtime
  (lib/xo/race) instead of clang's, copied beside the binary on darwin as
  the native backends do: Apple clang 17's runtime stopped at SIGILL on
  an Intel Mac. A race build served from the build cache also copies the
  runtime beside its new output.

- `xo check` checks by the native backends' rules by default, as a build
  does (owner, 2026-10-23): `use c` passes, and `--backend=go` checks by
  the Go backend's. The native support check (XO1201) is `--support`
  (its old name `--no-gc` still works).
- XO1201 says "not available on the native backends (arm64, amd64,
  LLVM)" instead of naming `--no-gc`, which the build may not have been
  given.
- `xo vet` no longer offers to rewrite `x ?? _ => continue` (XO0906)
  into the same text.
- Cross builds link or fail: a darwin program links with Xo's Mach-O
  linker from any host (from Windows or Linux, darwin targets were
  reported built and only their assembly written), and a cross build
  that needs a C compiler (a `use c` source, `--ld=cc`) fails naming
  the target instead of exiting 0 with no binary (`--targets=all`
  skips it with a note). Darwin/amd64 builds that link through cc
  (`use c`, `--ld=cc`) link again (undefined `___tsan_*` symbols), and
  a generic struct's field default that mentions its type parameter
  (`items: List[T] = []`) builds on the native backends.

- `r ?? e => body` (decision 0321) builds when `r` is a plain variable,
  a parameter or a field: it failed on the native backends (XO1201
  converting `()` to `Int`) and the Go backend (`undefined:
  MISSING_VAR_ok`) while `xo check` passed it, and only `(r) ?? e =>`
  worked.

- Warm builds are faster by default (owner, 2026-10-23): the module check
  cache is on (`XO_CHK_CACHE=auto`; a warm build of xo itself 0.885 of
  the wall time with 0.914 of the peak memory), and the cache's shared
  pack (decision 0351; a new program's first build 0.831 of the wall
  time, a warm build in a new directory 0.857). `XO_CHK_CACHE=off` and
  `XO_SHARED_PACK=off` turn them off.

- `xo build --targets=all` skips a target the program cannot support, with
  a note naming it and why (a network program and wasip1/wasm, which has
  no sockets), and exits 0 when the rest built; a target named in
  `--targets` still fails. `--targets=wasip1/wasm` alone now builds with
  the wasm backend (it picked the host's backend and failed naming it).

- A hole interpolated into a string (`"${???}"`) has type `Str`, with the
  values in scope that fit it, instead of `hole of type unknown`.

- `xo test --explore` prints the exact command that reproduces a failure
  on its first seed too, as it did for later seeds (the Go backend said
  only `seed 1`, and `xo test --seed=1` without `--backend=go` passed).

- A test block's `opt?` failure names where the value was missing:
  `error: missing value: find(x) at main.xo:61:11`, as decision 0347
  says and as `expect` failures do, so two `?` of the same text in one
  test are told apart. Every backend.

- `xo test --seed=N` and `--explore=N` run natively on the deterministic
  scheduler (decision 0350, stage 2), as does a test that pins its seed
  (`test "x" seed N`): each test runs with its own seed, else `--seed`'s,
  else 1, and `--explore=N` runs a test under seeds 1 to N until one
  fails, printing how to reproduce (`xo test --seed=S --run=...`) and pin
  it. Before, `--explore` chose the Go backend and native tests ignored
  their seeds. The seeds that expose a race differ from the Go backend's.

- `XO_PROF=cpu` keeps the Xo caller of a sample taken in a C library
  system call (a file's open or read): the profile walk now follows
  the C library's frames above the system call stub back into the
  program, where before such samples showed `open` alone.

- `xo run --seed=N` runs natively on a deterministic scheduler (decision
  0350, stage 1): one thread runs every task, and at each scheduling
  point (channel operations, `select`, `Mutex.with`, `spawn`, `await`,
  `Atomic` operations, `task.is_canceled()`, every wait) a generator
  seeded with N picks the next ready task, so a seed reproduces an
  interleaving. `xo build --seed=N` builds such a program, which takes
  its seed from `XO_SEED` (default 1). Programs built without `--seed`
  are unchanged. Before, `xo run --seed` chose the Go backend.

- Warm builds no longer look up, on every build, the front end cache
  entries of bodies the program never builds (one key per checked body,
  4,146 of a one-line program's 5,373): the build target's pack lists
  the keys found nowhere and skips them. A warm build of a one-line
  program uses about 7% less CPU (decision 0351).

- `XO_CHK_CACHE=auto` skips the check of a module whose closures make a
  generic instance (a Map method first called in a `Mutex.with`
  closure): such a closure now has only its owning function checked
  again, as a body without a front end entry does; before, its whole
  module was checked on every build (bench/perf/http's never skipped).

- `XO_BACKEND_CACHE=verify` no longer reports front end cache entries
  that another program recorded as differing: it compared the modules a
  body was first to build code of and its closure and constant numbers,
  which depend on what the build did before the body and which a replay
  redoes. Builds through the cache were already identical.

- 0.26.0 (2026-10-22): Windows binaries signed (Azure Artifact Signing)
  and native windows/amd64 I/O on completion ports (decision 0324); large
  Maps and Sets branch instead of copying (0291); fault tracebacks by
  default; darwin DWARF embedded (0346); native `--race` without clang;
  xo's memory budget (0349) and the LLVM split default; `xo run DIR` runs
  exactly DIR (0345), build and run skip `_test.xo` (0348), `opt?` in
  tests (0347), and the daylight saving gap fix (Earlier and Later were
  swapped). The entries below, down to 0.25.2's, are in it.
- LLVM builds of large programs split their IR: over 100 MB (xo itself),
  modules of about 50 MB with thin LTO, and the clang jobs sized to
  memory (three quarters of it, less xo's own heap, at 3.5 GB a job).
  The LLVM build of xo, which a single clang took past 20 GB, finishes at
  an 11.1 GB peak (bench/results/llvm-split-default-2026-10-22.md).
  Smaller programs stay one module with full LTO. XO_LLVM_SPLIT and
  XO_LLVM_JOBS still override.

- `XO_CHK_CACHE=auto`: a skipped module whose body calls `xml.encode_value`
  or `xml.decode` and is built from source (its front end cache entry
  gone) builds again; the call reached the build unrewritten and failed
  with XO1201 before the recheck could run (TestModuleCheckAuto
  xml_derive). check_lazy now rewrites the calls of the bodies it checks.

- `xo build -o /dev/null` works again: the hidden stage of an output
  whose directory cannot hold it (/dev) goes to the user cache, and the
  program's bytes are written to the output.

- Native runtime on Linux: an idle worker waiting out a run-next grace
  (50 us) in the poller uses epoll_pwait2 (a nanosecond timeout, Linux
  5.11) instead of epoll_wait, which rounded the wait up to 1 ms; older
  kernels fall back to epoll_wait. HTTP/2 download and CRUD within noise
  (`xo bench --ab`).

- xo stops itself past three quarters of physical memory, at least 16
  GiB (decision 0349's default, owner): a runaway build ends with
  `fatal error: memory budget exceeded` and its phase instead of swapping
  the machine. Builds of xo peak at 9.9 to 11.2 GB counted (18 GiB on a
  24 GB Mac). `XO_MAX_MEM` sets another budget, `off` none.

- The memory budget (decision 0349) counts what the heap uses (large
  blocks by the bytes handed out, not the room a growing block reserves),
  which tracks the resident set: a cold self build of xo peaks at 9.9 GB
  counted against 9.1 GiB RSS. `xo build --phase-times` prints the heap
  and its peak after each phase, and `debug.mem_peak()` is new. xo sets
  no default budget for now.

- The module check cache stays opt in (`XO_CHK_CACHE=auto`): it was
  made the default on 2026-10-22 and reverted the same night, when
  internal/arm64 TestModuleCheckAuto's xml_derive failed its warm build
  on main. scripts/gate.sh runs the TestModuleCheckAuto tests for any
  change to compiler/check or compiler/ssa.

- `xo run --holes` and `xo test --holes` build natively (owner,
  2026-10-22): an evaluated `???` faults with `evaluated a hole \`???\``
  as on the Go backend, so `--holes` no longer needs Go installed.

- `--hybrid` is deprecated (owner, 2026-10-22) and warns on every use:
  since the Go compiler's removal it has been a plain Go backend build.
  Use `--backend=go`, or the native backend (the default). It will be
  removed.

- Native programs print a fault's frames by default (owner,
  2026-10-22): an uncaught fault's one line report is followed by
  `traceback:` and `  FUNCTION (FILE:LINE)` per frame down to main, as a
  hardware fault's report already was. `XO_TRACEBACK=0` turns it off.
  Each fault pays about 0.55 us for its trace; the binary is unchanged.

- Messages: XO1201's note says to build with `--backend=go` (it said
  "build without --no-gc", also on default native builds that passed no
  `--no-gc`); "needs a native backend" names arm64, amd64, and llvm;
  `xo explain XO1504` says no build reports it today.
- scripts/release-verify.sh checks what 0.25.2 shipped broken: `xo build
  --targets=all` gives the six native binaries named NAME-OS-ARCH, `xo
  build --race` with no clang on PATH reports a planted race with its
  main.xo line, the time_civil golden (daylight saving gaps and overlaps)
  matches, a Str view past 2 GiB (internal/arm64 TestStrViewPast2GiB,
  with 5 GB free), and on darwin atos names main.xo from the embedded
  DWARF (run it on the Intel laptop too).

- A memory budget for native programs and for xo (decision 0349, owner):
  `XO_MAX_MEM=12G` (or `512M`, bytes; `off`) stops a program whose heap
  maps more, with status 70, `fatal error: memory budget exceeded: ...`
  and what it was doing (`debug.mem_label`). xo labels its phases, so a
  runaway build stops with the phase named; it sets no default budget yet
  (the first default, half of physical memory, was removed the same day).
  `debug.mem_budget`, `debug.mem_label`, `debug.mem_mapped` and
  `debug.mem_total` are new in std/debug.

- Native runtime: `List.join`, `Bytes.zeroed`-style buffers and
  `to_hex` producing more than 4 GiB fault ("strings: Join output over
  4 GiB", "Str or Bytes longer than 4 GiB") instead of returning a cut
  result.
- Debug information for functions with thousands of overflow checks
  and variables shares one location entry across a run of fault stubs:
  a generated 24,000 line function wrote over 4 GiB of assembly.
- Crash bundles go to `<user cache>/xo/crash` (macOS
  `~/Library/Caches/xo/crash`) instead of the system temporary
  directory, where they were easy to lose (owner, 2026-10-22). The
  newest 100 are kept, the path is printed on standard error as before,
  `XO_CRASH_DIR` still overrides, and `xo clean -cache` leaves them.

- Native runtime: a fault caught by `task.isolate` (an HTTP handler's
  500 included) under `XO_TRACEBACK` no longer leaks its trace: the
  block was released at the wrong address, about 1 KB a fault (200,000
  faults: 203 MB against 2.8 MB).

- `xo build --targets=all` (and any target list mixing architectures)
  builds each target with its own native backend: 0.25.2 failed with
  "the arm64 backend builds for darwin/arm64 and linux/arm64, not
  linux/amd64". With no `--backend`, `all` is the six targets a native
  backend builds; windows/arm64 needs `--backend=go`, whose `all` still
  builds all seven (owner, 2026-10-22). Native builds for several targets also gave
  every binary the same name, so one overwrote another; they are now
  `NAME-OS-ARCH`, as the Go backend's.
- `xo build --targets=windows/amd64 -o app` writes `app.exe` (0.25.2
  wrote `app`, which Windows does not run).
- Native runtime: a `Str` or `Bytes` slice 2 GiB or more into its
  string read the wrong memory (a crash, or zeroed text), and appending
  to such a slice could overwrite the start of the string. A huge
  windows/amd64 build failed in the assembler because xo's own 3.2 GB
  assembly text came out with 1 GB of NUL bytes.
- Fixes from a 0.25.2 field report:
  - The Go backend refuses a program that listens or dials for
    wasip1/wasm, as the wasm backend does (WASI preview1 has no sockets);
    such a server printed "listening", took no port (Go's wasip1 net is
    an in-memory fake), and spun at full CPU. std/rt's listen and dial
    fail there too.
  - A std module used without its `use` (`json.encode` without
    `use std/json`) is XO0601 with the note and fix that add the `use`,
    ahead of members of imported modules that share the name
    (`http.json`).
  - A failed contract prints the values its condition reads inside calls
    and methods too: `ensures(ok(result))` and
    `ensures(result.len() == xs.len())` printed nothing.
  - `xo test --explore`'s failing seed names the exact command:
    `xo test --backend=go --seed=N --run='NAME'` (`--seed` alone runs
    natively, where only Rand is seeded).
  - `xo build .` no longer counts its own files' warnings as outside the
    directories built.
  - `xo edit add-field` makes the new field `pub` when every field of the
    struct is.
  - `xo query holes DIR` takes the directory (`xo query holes .`).
  - `xo run` sends a stop request it gets on to the program as SIGTERM
    when standard input is not a terminal (process managers, IDEs,
    Docker, scripts).

- `xo build` and `xo run` (and `--watch`) no longer read `_test.xo`
  files (decision 0348, owner): a type error in a test file no longer
  fails a build, a helper declared only in a test file is not visible to
  the program, a `use` only tests have is not fetched or bound, and
  `xo run --watch` does not rebuild after a test file changes. `xo test`
  and `xo check` report test files' errors as before.
- New warning `XO1511`: `SITE.read("index.html")`, `read_text` or
  `contains` on an `embed.Dir` constant (or a literal `.sub` of one) with
  a literal path the constant does not hold. It caught a server that
  served a blank page because the key was `static/index.html`; when one
  key ends with the path it is offered as a fix, with a note that
  `SITE.sub("static")` drops the prefix. Paths computed at run time are
  not checked.

- macOS 26's `atos` crashed on every native darwin executable's DWARF,
  so `--race` reports there named no Xo function, file or line: the
  line table's file names are now encoded the way clang encodes them
  (DW_FORM_line_strp). macOS 27's `atos` was not affected.
- `--race` with no `--backend` uses the native backend (arm64 or amd64)
  wherever a prebuilt ThreadSanitizer runtime ships (darwin/arm64,
  darwin/amd64, linux/arm64, linux/amd64, windows/amd64), so it needs
  no clang (owner, 2026-10-22, decision 0273 addendum). In 0.25.2 `xo build --race .`
  picked the LLVM backend and failed without clang, although `--race
  --backend=arm64` worked. Other targets still use LLVM, and a missing
  clang there says no native race runtime ships for the target.
- Native runtime: idle workers no longer loop on a run-next slot they
  may not take yet (its 50 us grace); they wait the grace out in the
  poller or on their futex. An HTTP/2 download server on linux/arm64
  uses 0.664x the CPU (`xo bench --ab`), at Go's rate with 1.21x Go's
  CPU (bench/results/h2-idle-loop-2026-10-22.md).
- `opt.or(x)`, `.unwrap_or(x)`, `.get_or_else(x)`, `.value_or(x)` and
  `.or_else(f)` on an Option stay `XO0601`, now with the note that Xo
  writes `opt ?? x` and a fix that rewrites the call
  (`os.env.get("PORT") ?? "8090"`); `.or_else` with a fn literal gets
  the note only.
- A guide for Go programmers, knowledgebase/guides/option-result-for-go.md:
  Go's `(T, error)` and `(T, bool)` habits against `??`, `?`, `.or_err`,
  `.map_err`, `.ok()`, `.err()`, `r ?? e =>` and `if let`, and the first
  errors (`XO0204`, `XO0205`, `XO0406`, `XO0601`), whose `xo explain`
  text links it.
- Each release archive's README.md starts with getting started for that
  archive: `bin` on the PATH, `xo version`, a three line hello world
  with `xo run .`, then `xo doc NAME`, `xo explain CODE` and the
  website's docs (scripts/release-readme.md).

- `opt?` on an Option directly in a test block fails the test with
  `error: missing value: opt` instead of `XO0204` (decision 0347, owner):
  `let u = store.find(7)?` reads as in an Option function and still
  names the expression that came back empty. Property tests shrink such
  a failure like any other. Helper functions and fn literals keep the
  usual rules; in a `_test.xo` helper, `XO0204` and `XO0205` now note
  that the form is allowed directly in test blocks.

- darwin executables carry their DWARF (decision 0346, owner): Xo's
  Mach-O linker writes it uncompressed into a `__DWARF` segment of the
  executable instead of a `NAME.dSYM` beside it, so a copied, cached,
  renamed or packaged binary keeps its file:line names in crash reports,
  fault tracebacks, the profiler, `--race` reports, atos and lldb.
  `--dsym` writes the bundle too; `-s` leaves both out. Release binaries
  (`xo` itself included) ship with their DWARF: the darwin archive grows
  by about 11 MB (xo's tar.gz 23.2 MB to 34.4 MB). Links through cc (the
  LLVM backend, `--ld=cc`, `use c`) do the same: dsymutil's DWARF is
  moved into the executable after the link and the bundle kept only for
  `--dsym`.
- DWARF fixes on darwin: atos names the first function of each code
  part (xo_f_main was nameless: a marker symbol shared its address), a
  build from outside the module (`xo build DIR`) records the module's
  directory as the compilation directory so lldb opens the right file,
  and `dwarfdump --verify` passes on programs with `List[()]` or fields
  of type `()`.
- `xo clean -cache [-n]` (owner, 2026-10-22), like `go clean -cache`:
  removes every cache under `<user cache>/xo` (build directories, the
  native back end's cache, GOCACHE, test binaries, watch builds, xo dev
  outputs, and the test infrastructure's caches, from one table in
  compiler/prune that a repository test keeps complete) and prints the
  bytes freed per cache; `-n` measures without removing. The module
  cache and the prebuilt seeds stay, each cache's stamp and lock files
  stay, an entry a running build holds the lock of is left, and nothing
  outside `<user cache>/xo` is touched. `xo clean` alone is a usage error.
- A directory argument means that directory alone (decision 0345,
  owner): `xo run` runs exactly the directory it is given, never a
  program found below it (in a parent of two programs `xo run .` ran the
  first it found), and says which subdirectories hold a program; `build`,
  `check`, `test`, `vet`, `doc`, `fmt`, `fix`, `parse`, and `audit` cover
  subdirectories only with `DIR/...`, which stops past 6,000 .xo files or
  60,000 entries (`xo build /...` refuses). A plain directory with no .xo
  file is an error naming `DIR/...`; `xo run FILE.xo` gives the fix `xo
  run DIR`. Write `xo check ./...` where `xo check .` covered the tree.
- Build outputs (the owner's 0.25.2 reports): `-o DIR` naming an existing
  directory writes `DIR/NAME` and `DIR/NAME.dSYM`, as `go build -o`
  does; every final output is built in a hidden `.NAME.tmp-TAG-RAND`
  directory beside it and renamed into place, so no visible file
  appears while a build runs and two builds of one output both succeed;
  a failed build, or one stopped by SIGINT or SIGTERM (exit 130),
  removes it and says why in words (it printed `IsDir(path: ...)` and
  left `OUT.tmp`); `xo run --watch` and
  `xo test --watch` build and keep `last.json` in
  `<user cache>/xo/watch/HASH`, not `.xo/watch` in the project, with the
  debug information named after the binary; `xo init` adds `/.xo/` (the
  `use go` and `use c` binding caches) to `.gitignore`.
- 0.25.2 (2026-10-22), the first binary release: archives for
  darwin/arm64, darwin/amd64 (signed and notarized), linux/arm64,
  linux/amd64 and windows/amd64, each `xo` stamped with the commit it was
  built from (`xo version -v`). 0.25.1 is a tag with no binaries: its
  windows/amd64 xo.exe faulted on every build, because the COFF writer
  wrote a section of 65,535 relocations or more with a wrapped 16 bit count;
  it now writes the extended count (IMAGE_SCN_LNK_NRELOC_OVFL), as LLVM's
  does (3cd6e6d3e). Also since 0.25.0: RSA's secret exponent loop
  (bigmod.exp_into) allocates nothing, so the constant-time checks see no
  select in it at clang -O2; no secret-dependent select was found and
  RSA's output is unchanged (74eb6a305). `json.decode` and `parse_value`
  reject a byte after the value, as RFC 8259 and Go do (f910b349c). Map
  helpers of `Map[K, Bytes]` are named by their types, not by which a build
  made first, which had let a cached caller call one with the other's
  result type (3b8975312). The arm64 inliner no longer inlines a function
  into itself after a callback brings its own call in (a compiler fault on
  generics, 1201c3153).
- A native program on Linux or macOS reports a hardware fault (SIGSEGV,
  SIGBUS, SIGILL, SIGFPE, SIGTRAP: a miscompile, a runtime bug, or C code
  behind `use c`) before the signal's default action ends it, with the
  same exit status and core file: the signal and address, the pc, the
  registers, and the frames named from the program's DWARF (decision
  0343; `XO_CRASH_REPORT=0` turns it off). windows/amd64 programs report
  an unhandled exception the same way (`fatal error:
  EXCEPTION_ACCESS_VIOLATION: access violation (code 0xc0000005, write)
  at address 0x10`), and keep the NTSTATUS as exit code.
- Diagnostics: an unknown method on a Set suggests `add` and `remove`
  (`insert`, `push`, `put`, `discard`, `delete`); `sub`, `add`,
  `elapsed` and the like on a Time or Duration suggest the operators or,
  for `clock.now().sub(t)` and `t.elapsed()`, `clock.since(t)`; other
  languages' method names now open with "did you mean". XO0410 on
  `m[k]?` and `m[k] ?? d` suggests `m.get(k)` (one error, with a fix),
  and on `m[k] += v` for a number or Str element `m.update(k, 0, fn(var
  e) { e += v })` (no new codes).
- Any integer type indexes a List or Bytes: reads, assignments, `get`,
  and slice bounds, and a Str's slice bounds (decision 0331, 5aff68252).
- `x ?? return v`, `?? break`, and `?? continue` on an Option, and the
  Result form `r ?? e => body` for an early error exit, which replaces
  the four line `match` (decision 0321; cec4bfb6f, 937331d29).
- A top-level `const` may call effect-free functions; it is computed
  once, on first use (`const ISO: Layout = time.layout(...)`; XO0415 for
  an effect, XO0416 for a handle in the value; decision 0332, 33288eb37).
- Slices of a Str or Bytes are views of the original, with no copy, and
  `Str.clone` and `Bytes.clone` make an explicit copy (decision 0330;
  d644daae2, 44099405f). `mime.parse_media_type` takes 25.6% fewer
  instructions, `multipart.parse` 16.1%
  (bench/results/views-step3-measured-2026-10-22.md). `url.parse_query`
  and `mime.parse_media_type` copy their tokens when the input is over 4
  KiB, so a kept token does not keep a large input alive: 318 MB to 6 MB
  resident in a probe (bench/results/views-parsers-clone-2026-10-22.md;
  4438a0bf0).
- `time` layouts are named fields (`"{year}-{month}-{day}"`), checked at
  compile time when constant, and `time.layout(text)` gives a `Layout`
  parsed once: native format 2.6 times and parse 2.1 times fewer
  instructions than with a layout string
  (bench/results/time-layout-measured-2026-10-22.md).
- A Mutex release that wakes a writer, on a Mutex `with_read` has taken,
  yields to it (decision 0118 addendum, owner: the default;
  `XO_MUTEX_YIELD=0` turns it off; f367eb970, 0ae49494d): the CRUD
  server on linux/arm64 from 88.6k to 190.8k requests per second on 3
  cores and 70.5k to 136.0k on 6; bench/perf/conc rwmutex costs 19 to 28%
  more CPU.
- `std/debug`: a server's CPU profile, heap profile, and scheduler state
  over HTTP behind an auth function, and `xo prof URL` fetches and
  summarizes one (decision 0325; 0c17dba55, a38859220).
- XO0540, a checker warning: a `Mutex.with` closure that writes its
  `var` value and then has a path that returns `Err`, which undoes the
  write (d4d420f20).
- `--race` on amd64 (darwin and linux) and reports with the call stack
  on every native target (decision 0273 addenda; 64a4f5fb3);
  windows/amd64 builds with it but is not supported until a Windows run
  with threads passes.
- LLVM builds mark only the loops with Float arithmetic for full
  unrolling instead of raising the unroll threshold for the whole module
  (decision 0338, superseding 0333; 8c54d5b9a): nbody keeps its gain
  (2,308 M to 1,160 M instructions) without 12.8% more CPU for xo's own
  LLVM build.
- `xo vet --cost` (decision 0327): XO0907 to XO0911 report costs the
  source does not show (a map copied before a loop, an element counted
  up and down each iteration, a list copied by each iteration's update,
  a large value copied into a closure or Option in a loop, a stored view
  of a parameter's text that keeps the whole input alive; 9b526de4f).

- A function value borrows its arguments (decision 0337): a callback
  that only reads an element (`xs.any(fn(u) { u.name == n })`,
  `filter`, `fold`, a `fn` parameter) no longer counts its heap fields
  up and down per call, except an argument of the function's result
  type (an accumulator) or a `var` closure's value, still handed over
  and updated in place. Callback walks over structs of two strings:
  -34% instructions native, -27% LLVM, -43% and -50% on a list shared by
  two tasks (bench/results/callbacks-0337-2026-10-22.md). `xo vet
  --cost` XO0908 now notes only the callbacks that still take their
  element.

- The back-end cache's objects and parts name no directory, so a build
  in another worktree at the same commit reuses them (xo: 0/117 objects
  before): the arm64 emitter's DWARF names the compile directory and
  std/'s root through .debug_str slots filled as each object leaves the
  compiler. A string literal binding's location list no longer depends
  on the part's other functions (linux/arm64 builds through the cache
  differed from cache-off ones in their DWARF).
- The Go backend compiles a std method call on a value holding a Map
  again (`Request.client_ip`, `Embed.Dir.paths`): since decision 0335 the
  own-marked receiver was not addressable for std/rt's pointer methods.
- `XO_CHK_CACHE=auto` warm builds are the cache-off build's binary
  again: a skipped main module's check entry records whether main is
  named only by its declaration (check cache v6), so main's own Os is
  folded to the runtime's capabilities as on a cold build.
- The back-end cache stores module check entries (chk) only when a
  build reads them (`XO_CHK_CACHE=1|auto` or verify); `--phase-times`
  says `cache chk off` otherwise. Every store appends a line with its
  kind, bytes and code generation key to the cache's `store-log`, so
  the prune's churn can be attributed to new keys or repeats.
- `go run ./scripts/xobuild` builds an older commit (a bisect step) with
  the newest prebuilt seed cut from an ancestor of HEAD, skips a bin/xo
  built from a newer commit, and rejects a built xo that cannot run:
  a commit before seed 0.24.0 got an xo that stopped at `dyld: Symbol
  not found: _xo_x_preempt`.
- `xo dev unwind` and `xo dev debug` name a frame without DWARF in any
  instruction section, not only `__text`: an LLVM build's functions in
  `__xo_text` (decision 0318 stage 4) showed as `?`, and `frame N`
  selected the wrong frame.
- `XO_TRACEBACK=locals` prints an inlined call's variables under its
  `[inlined]` frame instead of the caller's (LLVM printed every frame's
  under main), each line once, a Str missing its length as
  `<unavailable>`, and on arm64 a literal's binding (`let label =
  "order-7"`) with its value instead of `<unavailable>`.
- The back-end cache packs a large module's parts (xo's 650 MB) on its
  build target's second build, not its first: each gate's one-shot
  self-build in a new worktree wrote one no build read (9.3 GB of the
  16 GiB cap, about 27 GB an hour of the prune's churn). A pack none
  of whose entries a build used (after a rebase) is rewritten instead
  of appended to.
- The back-end cache names a build target without its flags' values:
  `--stamp NAME=VALUE` (scripts/xobuild stamps the commit and the
  source key) made every self-build a new target that wrote a 678 MB
  parts pack no later build read. A second stamped self-build now
  reuses 115 of 116 parts (9.5 s against 11.0 s) and leaves half the
  packs (666 against 1,295 MB).
- `xo vet --cost` XO0908 also notes a loop handing struct elements to a
  function value and the prelude's callback walks (`xs.any(fn ...)`,
  `map`, `filter`, `find`, `all`, `fold`) over elements holding heap
  memory, which count each element up and down (3.3 times a `for`
  loop's instructions in a measured walk; decision 0327 addendum).
- The Go backend's Map overwrites an existing key in place while no copy
  shares the map (decision 0335): the generated code marks a map where it
  copies one, as it marks lists. bench/perf putget -39%, update -40%;
  maps that are copied cost what they did.

- `xo vet`'s `XO0906` also notes the four line Result `match`
  (`Ok(v) => v` and `Err(e) => return w`, `break`, or `continue`), with
  the fix `r ?? e => return w` (`Err(_)` gives `_ =>`; decision 0321).

- A code generator failure names the Xo source: the native emitters'
  internal errors and amd64 template failures, the LLVM emitter's
  unsupported constructs, a clang error on the generated IR (mapped from
  the IR line to its function), and the Go generator's errors with no node
  of their own now start `file:line:col: in \`function\`:` (the line of
  the value being lowered, or the function's first; an inlined callee is
  named in its own file). The SSA builder's were already XO1201
  diagnostics at the expression.

- Linux builds with `--backend=llvm` link and run again: since asynchronous
  preemption for LLVM (decision 0318 stage 4) every one failed to link
  with ld.bfd's LLVM plugin and lld ("__start_xo_text can not be
  undefined in a subtraction expression"), and with that fixed crashed at
  start. Checked in Docker on linux/arm64 and linux/amd64 with both
  linkers.

- A by value receiver that is a variable or a field path is read after the
  call's arguments, as a `var` receiver always was (decision 0326):
  `t.size(t.add(v))` sees the item `add` pushed. `xo vet`'s XO0537, which
  reported the old order, is retired.
- Allocation and free on one thread: the frees count (for XO_LEAKCHECK,
  XO_HEAPSTATS, XO_RTSTATS) only while counting is on, the free lists'
  lengths are left to the magazines of a program with threads (recounted
  once when threads start), and a string's capacity is computed instead
  of reread from the heap: each was a load and store chain through the
  heap record on every allocation or free. A loop allocating a 4-byte
  Bytes runs in 20% fewer cycles natively; bench/perf on LLVM 35 of 114
  workloads faster (HMAC 0.84), none slower; arm64 21 faster, two about
  1% slower.
- WebAssembly: `Stdio.write` wrote nothing (its buffer was never flushed in a
  WASI module); it now writes through, in order with print and println.
- A program's own Stdio, Clock, and Rand, and an `Os` built with a
  literal, work on the native backends (decision 0307, complete); the
  native capability values shrank (an Fs from about 66 words to 5);
  `os.tasks` compiles on wasm.
- Raw multi-line strings (decision 0320): `r"""..."""` has no escapes
  and no interpolation and removes common indentation as `"""` does;
  `r#"""..."""#` may contain `"""`.
- HTTP server: a connection marks itself idle and checks whether the
  server stops with atomics instead of the server's lock, twice a
  request (40% fewer contended lock slow paths at 32 connections);
  XO_RTSTATS counts futex calls and slow lock paths by lock.
- main's body runs on a fiber (decision 0286 step 1): a parked main
  moves to an idle worker while main's thread computes, so a program
  that sends work to a computing task no longer has its timers and wakes
  wait for that task (a 20 ms ticker 164 ms late before, 0 now). A ping
  pong between main and one task costs about 65 ns more a hop (two more
  stack switches); spawn and fan out are unchanged. XO_MAIN_FIBER=0 keeps
  main on its thread until 0.24.0 is cut.
- std/http: header fields with repeated names, sent and received each:
  `header_values` and `add_header` on requests and responses, and
  `Response.with_cookie` and `cookies()`. A client response with several
  `Set-Cookie` fields no longer arrives as one joined value.
- std/http: cookies, ported from net/http: `http.Cookie` and its
  `display()` (a Set-Cookie value), `parse_cookie`, `parse_set_cookie`,
  and `Request.cookie(name)`, `cookies()`, and `with_cookie`.
- std/tls: a TLS client on a connection the program dialed
  (`tls.client(conn, host)`), for protocols that upgrade a plaintext
  connection (Postgres's sslmode, SMTP STARTTLS, Redis). It verifies the
  certificate and host name by default; `Verify.ChainOnly` skips the
  name and `Verify.None` only encrypts (`xo vet` warns, XO0902).
- A server whose handlers compute no longer stalls its accept loop: a
  task running on main's thread (main's body, or a task its join runs)
  no longer waits for work that thread took on while the task was parked,
  when another worker is free to take it (decision 0286 step 0): HTTP
  dial to response p99 with 8 computing clients 70 to 13 ms on a Mac.
- `xo build --race --backend=arm64` on macOS: the arm64 backend checks
  races itself (decision 0273), with no clang: it calls
  ThreadSanitizer at the program's loads, stores, and atomics, and Xo's
  linker links ThreadSanitizer's runtime (XO_RACE_RUNTIME names it until
  releases ship one). Linux and amd64 still use --backend=llvm.
- The backend caches survive rebuilding xo: they key on a hash of the
  code generation sources instead of xo's binary, so an xo rebuilt for
  a change elsewhere (the driver, std, tools) builds from a warm cache;
  an arm64 darwin build linked by xo's own linker also uses the binary
  cache.
- The TLS server seals a response without copying it first and takes a
  whole record without slicing it, and HTTP/2 header names are compared
  in words: TLS requests 2.2% and HTTP/2 requests about 2% fewer
  instructions.
- A cold build of a large program spreads its code generation better:
  the parts go to the tasks largest first and a part of large functions
  is cut further (xo's emit 11 to 8 s of wall on 8 tasks), and the
  passes after each function's generation split its text once.
- --race checks the program's tasks, as Go's race detector checks
  goroutines (decision 0273): a race between two tasks on one thread is
  reported now (the runtime's fiber switches used to order them); the
  runtime is left out of the check and says what orders tasks itself
  (spawn and await, scopes, channels, select, mutexes, Atomic), each
  tested both ways (a race reported without it, none with it). The
  runtime's own race tests keep checking the runtime (XO_TSAN_RUNTIME=1).
- XO_HEAP_GUARD=1 when building: the runtime's records end against an
  inaccessible page, so an overrun faults where it happens, and an
  rt.slot_* on a List checks its index (the class of the SV_ROOTS
  crash).
- The native backend is the default on every host: amd64 on x86-64
  hosts (macOS, Linux, Windows) as arm64 on arm64 ones, chosen by the
  targets' architecture, so a default build needs no clang (decision
  0270). Its code is still slower than LLVM's on amd64 (HTTP about 1.15
  times the instructions; on an Intel Mac JSON 1.24, RSA 1.44);
  `--backend=llvm` keeps LLVM's, and `--race` uses it.
- Reference counting insertion sorts each block's live set once, in
  O(n log n), and keys leaves without building text: a cold build of xo
  spends about 18% less CPU (insert_rc 16 to 6.5 s of wall after the
  first step).
- HTTP/2 keeps common header names and values as constants, and AES-GCM
  encrypts only the counter blocks a record needs: HTTP/2 requests 4.6%
  (arm64) and 3.6% (LLVM) fewer instructions, TLS requests 1.3%.
- The files a build target's last build parsed are parsed on tasks
  before the check, and the front end cache also keeps the bodies that
  reuse generic instances made before (xo after an edit: 0.6 to 1.3 s
  less wall, about 8.5 s). A module check cache (an unchanged module's
  bodies checked only when the build needs them, its warnings replayed)
  is opt in, XO_CHK_CACHE=1, until it covers the whole-program checks.
- The back-end cache's prune removes empty directories (one per build
  target) unused for a week.
- Fix: on Windows, `Fs.create_new` over a directory reported `IsDir`
  instead of `Exists`, and over a dangling symbolic link it created the
  link's target instead of reporting `Exists` (seen in the Windows golden
  kit's run of fs_create_new, 2026-10-19).
- The native TLS server keeps a connection's state where only its task
  reads it, instead of sharing the whole state again at every read and
  write: TLS requests 8.2% and HTTP/2 requests 9.2% fewer instructions
  on arm64 (2.9% and 3.2% on LLVM).
- Fix: every native program (the arm64, amd64 and LLVM backends, every
  OS) wrote a pointer past the end of the runtime's services record at
  start, since 67c52321 (2026-10-17, decision 0251's registry of root
  contexts): ctx.xo passed the byte offset SV_ROOTS to rt.slot_*, which
  takes a word index. On Linux, macOS and under Wine the heap word there
  was 0 and the overwrite went unseen; on real Windows the environment
  strings lay there and every program faulted before main (0xC0000005,
  both backends). Regression test std/native/llrt/ctx_test.xo; gotcha
  svc-roots-slot-index.

- The LLVM backend keeps a release's rare paths out of line (a shared
  count's drop, the releases of a fault's cleanup): xo built by LLVM
  51.5 MB to 32.6 MB of code, xo.exe 80.6 MB to 63.7 MB, HTTP and TLS
  unchanged or up to 1.1% fewer instructions.
- The back-end cache keeps the SSA of each function body the front end
  built, keyed by its module's interface and the body's own text: a
  body whose module's view of the program did not change is replayed,
  not built again (xo after an edit: the SSA phase about 2.5 times
  shorter, 1.2 to 1.9 s less CPU). A program using tasks is
  no longer built twice (the SSA phase of xo 3.7 s to 2.7 s cold).
  XO_FE_CACHE=off turns it off.
- Borrow inference runs its rounds on tasks (the same result: the
  greatest fixed point does not depend on the order): after an edit of
  xo, insert_rc.plan 1.1 s to 0.4 s of the wall time; --phase-times
  shows insert_rc.borrows, insert_rc.types, and insert_rc.keys.
- The back-end cache keeps each part of a split build (its functions'
  code, exception tables, and debug sections) under the keys of its
  functions: an unchanged part is read, not generated (xo after an
  edit: 0.8 s less CPU generating parts, the parts' 218 MB of function
  entries no longer read).
- Windows builds with the LLVM backend split into modules link again
  (the release recipe for xo.exe, split 16 with ThinLTO: 294 s on the
  Mac), and at -O0: shared globals get COFF comdats, weak C references
  are alternate names, and calls of functions that only test the build
  target (the runtime's layer()) fold to their constant.
- The back-end cache no longer decodes a function's reference counting
  when its inlining is read from the cache too (the inliner's facts come
  from a small summary kept beside the entry): after an edit of xo,
  0.4 s less CPU decoding and about 0.5 s less user CPU, the wall time
  0.2 to 0.7 s shorter.
- The back-end cache keeps each build target's entries in a few pack
  files besides the per-entry files, read with one open each: after an
  edit of xo, the cache's reads take 0.03 to 0.11 s instead of 0.2 to
  0.5 s per phase, and the build 1.1 to 3 s less system CPU.
- The back-end cache keeps functions with float constants too, and the
  emitter's key comes from the inlining key instead of hashing the
  function again: a one-constant edit of xo takes 14.1 s to 10.7 s of
  CPU.
- The back-end cache's reference counting and inlining entries are
  binary: a one-constant edit of xo takes 16.8 s to 14.1 s of CPU.
- The back-end cache also keeps each function's inlining, under a key
  chained from its reference counting key: a one-constant edit of xo
  rebuilds in 9.5 s (11.1 s before).

- Faster rebuilds after an edit: borrow inference rescans only the
  callers of a function whose flags changed, the back-end cache's entries
  are read in parallel, and the verification after inlining skips
  functions that verified before. A one-constant edit of xo rebuilds in
  11.1 s instead of 14.1 s (17.0 s of CPU instead of 25.2 s), and the
  binary is the same file as a build without the cache.

- macOS x86_64 programs are assembled by Xo's own assembler (objects byte
  identical to clang's) and linked by Xo's own linker: no C toolchain.

- Xo's own x86_64 backend builds macOS x86_64 programs
  (`--backend=amd64 --target=darwin/amd64`; linked by Apple's cc for now).
- Outside text is always valid UTF-8 (decision 0259): HTTP header values,
  files read as text, file names, standard input, child output, the
  environment and arguments, and archive names replace each invalid byte
  with U+FFFD; URL paths and queries keep it as a percent escape. A
  request path with a raw `\x82` used to fault a handler that sliced it,
  and a redirect to such a host ended an HTTP client.

- x86_64 operates on spilled values in place (SHA-3's permutation 26%
  fewer instructions; a TLS handshake with ML-KEM 5%).

- x86_64 multiprecision arithmetic keeps its carries in the flags
  (X25519's field multiply 22% fewer instructions; a TLS handshake 5%).

- Full TLS 1.3 handshakes: -13.9% (arm64) and -13.6% (LLVM)
  instructions on the callgrind benchmark, -22% on linux/amd64: X25519's
  public key from a fixed base table (one ladder instead of two), the
  transcript hashed incrementally, cheaper encoders, and list literals
  allocated at their final size.

- HTTP/2 server: -12.3% (arm64) and -8.2% (LLVM) instructions per
  request on the callgrind benchmark: the request checks compare in
  place, a request's parts are no longer packed and copied out again,
  and the TLS layer copies less per record and per call. HTTP/1.1 over
  TLS -5.7% on arm64.

- darwin/amd64 (LLVM): a fault that task.isolate or a server's handler
  should catch no longer ends the program when the walk passes a frame
  with compact unwind information before a DWARF one (the unwinder's
  frame cache reset the return address register; 13 tests on the Intel
  Mac).
- An addition after a checked multiplication by a constant skips its
  overflow check when the product's range proves it safe (Collatz: 7%
  fewer instructions on x86_64, 4% on arm64).

- HTTP/1.1 responses on the native backends take the Date header's text
  from a cache of the current second shared by the threads (a sequence
  lock in runtime static words 69 to 74), as net/http does: plain HTTP
  -1.1% instructions per request on arm64.

- Maps of up to 5 entries keep no hash table: a put appends without
  hashing or rebuilding, a lookup scans the keys (their lengths first),
  and the sixth put builds the table. Plain HTTP -4.5% (arm64) and -5.3%
  (LLVM) instructions per request (its headers Map); lookups in such a
  map cost 0.88 (4 entries) to 1.00 (5) times the table's, and a lookup
  in a map with a table about 1% more (it checks for the table).
- x86_64 functions make their stack frame only on the paths that need
  it (recursive Fibonacci: 26% fewer instructions, now fewer than
  LLVM's).

- A Map's lists are sized at each rebuild for every entry until the next
  one, so small maps (an HTTP request's headers) do not grow them:
  plain HTTP -1.4% (arm64) and -1.2% (LLVM) instructions per request.
- x86_64 code drops the register copy before most arithmetic (n-body
  23% and collatz 13% fewer instructions).

- Plain HTTP/1.1 servers on the native backends: a request's header
  fields are copied once (not the whole line first), the field lists
  start with room for 8, and `to_upper`/`to_lower` of text already in
  that case return it without a copy: 3.0% (arm64) and 3.7% (LLVM)
  fewer instructions per request (bench/results/http-callgrind-2026-10-17.md).
- x86_64 programs without a C library copy 64 bytes a loop iteration
  (copying a large map: 21% fewer instructions, even with LLVM).

- TLS servers on the native backends no longer copy a connection's state
  at every read and write (rt.any_take moves it out of its holder and
  back): 19% fewer instructions per request on arm64 and 27% on LLVM for
  HTTPS keep-alive, 13% and 19% for HTTP/2
  (bench/results/http-callgrind-2026-10-17.md).

- `xo build --backend=wasm` embeds std/time's zone data as native builds
  do: the TZif data of each constant zone name, only when the program
  looks zones up (hello world and the playground's compiler do not
  grow); a wasm program reads the wall clock through WASI.
- Programs without a C library copy memory with vector loads and
  stores on arm64 and x86_64 (copying a large map: 65% fewer
  instructions on arm64, now even with LLVM; 36% on x86_64). The
  runtime intrinsic behind it, rt.mem_copy_block, was approved by the
  owner on 2026-10-17 (decision 0207).

- WebAssembly builds (WASI) read `Env.program`, `Env.vars` (sorted),
  `Proc.set_grace` and `task.stop_requested` (no signals: never), request
  values (`Any`), and `Fs.case_sensitive` (probed in the working
  directory, not assumed); reading a directory as a file is `IsDir`. A
  program that starts processes fails to build for wasm with "WASI
  preview1 has none", as one that uses sockets does.

- LLVM debug builds show the parts of a parameter or a call's result
  that the code itself reads (a string parameter's length, a map's
  count), where before such values showed as not available.

- arm64: pushing a byte or a word onto a string the code holds alone and
  that has room is inline (a count and capacity check, a store), the
  call only for a shared, literal, or full string: json/enc_strings 3.7%
  fewer instructions, json and text workloads 0.7 to 2%.

- LLVM debug builds describe Str, List, Box, Map, Set, enum, function,
  and handle values to lldb and gdb as arm64 builds do (decision 0252
  stage 4): a list through its object, a struct, enum, or string the
  function builds by its parts; a value that arrives whole (a parameter,
  a call's result) shows as not available rather than with an earlier
  value. A Float32 variable now shows its value (it was read as half of
  the double the register holds). --release and -s builds are unchanged.

- arm64 debug builds describe a variable's word that holds a constant by
  the constant (DW_OP_constu, or DW_OP_addrx of a string literal's
  static object): a literal field that no register keeps (an immortal
  string, which reference counting no longer retains) shows its value in
  lldb and gdb. On macOS a program linked with cc (it links C sources)
  shows the string's length but not its bytes: dsymutil cannot relocate
  a data address in a location list.

- linux/amd64 floating point loops read their operands from memory
  directly (n-body 3% fewer instructions).

- WebAssembly builds (WASI) rename, remove, set modification times,
  create and read symbolic links, and copy files (goldens fs_create_new
  and sys_files_clock now run on wasm). WASI preview1 has no permission
  bits and no advisory locks: `mode`, `set_mode`, and `lock` report
  `Io(msg: "operation not supported (WASI has no file modes or locks)")`
  (after NotFound and the other stat errors), and `set_modified` before
  1970 reports `Io(msg: "WASI has no times before 1970")`. `os.fs.cwd()` under WASI,
  which has no working directory, is rooted at "." (its paths resolve
  against the working directory's preopen, not against `/`).

- WebAssembly builds (WASI) open files as handles: `Fs.open`, `create`,
  `open_append`, and `create_new` with their reads, line reads, writes,
  and close, and `create_dir_all`, `remove_all`, and `create_temp`, with
  the native backends' results (golden fs_handles).

- Files of more than 2 GB can be written and read on macOS (one system
  call refused more than INT_MAX bytes): the native runtime moves at most
  1 GiB a call. Toolchain tests shrink the chunk through std/internal's
  rt_probe "io_chunk" (runtime static word 47).

- Debuggers see Str, List, Map, Set, enum, and function values of arm64
  native builds (decision 0252 stage 4): lldb's `frame variable` shows a
  list's object (count, length, capacity, items), a string's bytes, and
  an enum's variant by name with its fields.
- SHA-256 uses x86-64's SHA-NI instructions when CPUID reports them
  (owner approved, 2026-10-17; decision 0254), on Xo's amd64 backend and
  on LLVM, with the portable code kept for CPUs without them.

- `Fs.create_new(path)`: a handle on a new file, or `Exists(path)` when
  anything is already there (a file, a directory, a symbolic link even
  when it dangles), as O_EXCL; on the native backends (Linux, macOS,
  Windows CREATE_NEW), the Go backend, MemFs, and wasm.
- The LLVM backend's Linux arm64 programs without a C library take the
  same single instruction atomics (one thread: task spawning 34% and
  fan-out 25 to 32% less CPU time).

- Linux arm64 programs linked without libgcc use ARMv8.1's single
  instruction atomics when the CPU has them (XO_LSE=0 turns them off):
  task spawning and fan-out take 21 to 33% less CPU time.

- arm64: a reload of a spilled value that x16 or x17 already holds is
  dropped (13.8% of reloads in the TLS client; fault/defer 10% fewer
  instructions, TLS handshakes 0.7 to 2%).
- The amd64 backend selects x86's own forms for checked multiplies (imul
  and jo), constant offsets (displacements), and logical immediates:
  collatz 13%, iface 24%, hof 18% fewer instructions.

- Debuggers see Int, Float, and Bool locals and parameters of LLVM
  native builds too (decision 0252 stage 3): lldb's `frame variable`
  prints them. -s and release builds carry none of it and cost what they
  did; with debug information LLVM's code can differ, so every golden
  built with `--release --debug-info` is checked to print what its
  `--release` build prints.
- The native runtime's memcpy, memmove, and memset move 32 bytes an
  iteration (no C library builds): copy-heavy programs 35 to 49% fewer
  instructions on amd64, arm64, and LLVM
  (bench/results/amd64-vs-llvm-2026-10-17.md).

- arm64: small fixed-size copies are loads and stores (no memcpy call),
  and a chain of subtractions with borrow is subs and sbcs: P-256 and
  TLS 1.2 handshakes 12% fewer instructions.
- Xo builds for linux/amd64 with its own x86_64 backend and no C
  toolchain: `xo build --backend=amd64` (decision 0254). All 182 native
  goldens pass on linux/amd64, assembled and linked by Xo's own
  assembler and ELF linker.
- The amd64 backend lowers the carry-less multiply and the AES rounds to
  PCLMULQDQ and AES-NI when CPUID has them (decision 0254, stage 3):
  AES-GCM about 15 times faster on linux/amd64.

- ML-KEM in the native TLS client costs less than half as much
  (bench/results/mlkem-client-2026-10-17.md): the hybrid handshake's
  ML-KEM part is 62% smaller on arm64 and 59% on LLVM, and
  encapsulation and decapsulation 28% and 29% smaller.

- arm64: a chain of adds with carry is one instruction per pair with the
  carry in the flags, and each product it adds is made right before its
  pair: P-256's field product saves 1 register pair where it saved 5,
  TLS handshakes 3.1 to 3.3% fewer instructions.
- arm64: a constant that is a logical immediate or a 32 bit movn is one
  instruction, and the inliner counts an add with carry and its carry
  as one, so P-256's field add and subtract inline: bench/perf 0 of 114
  workloads slower, 19 faster (TLS handshakes 1.5 to 2.5% fewer
  instructions, json/parse 3.4%).
- selfhost/macho elflink links x86-64 static executables (decision 0254,
  stage 4; `macho elflink --static`): LLVM-compiled goldens assembled by
  Xo's x86-64 assembler and linked by it pass on linux/amd64 (29 of 29),
  and glibc (dynamic) x86-64 links too (12 of 12).

- P-256's final subtraction takes its first limb as t0 + 1 and its masks
  as (carry - 1): three instructions fewer per field add, subtract and
  multiply (bench/perf/tls p256 on arm64: 1.3% fewer instructions, about
  4% fewer cycles).
- Xo's own x86-64 assembler (decision 0254, stage 1): selfhost/macho
  encodes the amd64 emitter's AT&T text into ELF objects byte for byte
  as clang does (1,953 instruction forms, 3,192 forms of LLVM's own
  output, whole files, and 42 LLVM-generated programs compared with
  clang); `macho asm --amd64`.
- RWMutex: readers a writer's release grants run on the granting worker
  for a short grace (20 us) before another worker may take them
  (decision 0077's addendum): bench/perf/conc rwmutex 90 to 40 ms wall,
  half the CPU; a reader's wait stays bounded (testdata/sched/rwbusy) and
  bench/perf/http p99 is unchanged or better.
- stdio's mode word is read and written with relaxed atomics: a first
  print racing a task's flush was a ThreadSanitizer report
  (TestGoldenTasks/http_wire, 2 of 40 runs under load).
- `xo prof FILE` (decision 0245): a native program under XO_PROF writes a
  raw profile (xo-prof.xoraw: the samples and their locations, named by
  the program) and `xo prof` writes xo-cpu.pprof, xo-heap.pprof and their
  summaries from it; `xo run --profile` and `xo test --profile` do both
  steps as before. The pprof and summary writers left every binary:
  hello 66.6 KiB smaller (633,314 to 565,106 bytes, darwin/arm64).
  XO_PROF_OUT now names the raw file; XO_PROF_HEAP_OUT is gone.
- Smaller frames from the wasm backend (decision 0179's addendum): wide
  parameters by address, wide values retained and released through an
  address, and the cold locals of big functions in a memory frame. The
  playground's compiler is 19.4 MB instead of 29.1, builds the sample in
  1,086 ms instead of 1,654 under Node, and compiles calls nested 40
  deep in Chrome where 10 did, at the same run time.
- Debuggers see local variables of arm64 native builds (decision 0252
  stage 2): Int, Float, Bool, and struct and tuple locals and parameters
  get DWARF types and location lists from the register allocator, so
  lldb's `frame variable` and gdb's `info locals` print them (Str, List,
  Map, and enums come later). Code is the same with and without debug
  information, checked on every golden.

- A scope's join takes its own tasks' queue entries as it runs them
  (decision 0077's addendum), instead of visiting each task a second
  time later: bench/perf/conc spawn at XO_THREADS=1 83.5 to 66.2
  Mcycles on macOS (about 1.18 times rust-ct's cycles a spawn).
- A task whose handle is dropped no longer boxes its value result (it
  cannot be read): spawn 1,835 to 1,620 instructions a spawn on macOS,
  1.12 times rust-ct's (decision 0068's addendum).
- arm64: if-conversion (a branch whose sides only pass values to one
  block, or run up to four instructions that cannot fault, becomes csel,
  csinc, or fcsel), `c >= lo && c <= hi` as one unsigned compare,
  `a && b` and `a || b` as Bool values without branches, and madd/msub
  for a wrapping multiply and add: json floats 0.965 and text lines
  0.941 of the cycles, no workload slower in cycles
  (bench/results/arm64-float-text-2026-10-17.md). Conditions on loaded
  values keep their branches (gotchas/arm64-if-conversion-cost.md).
- `xo dump` and SIGQUIT on native programs (decision 0251): the task tree
  with each task's wait in the Go backend's words and its line (`task 2
  (spawned at main.xo:6): blocked in sleep at main.xo:9`), on standard
  error and in `$XO_DUMP_DIR/xo-dump-<pid>.txt` (mode 0600, never through
  a symlink). The program keeps running; one worker or an idle server
  dumps too (the poller's thread writes it).

- Native contract fuzzing draws small numbers, as the Go backend's does
  (std/rt GenRand.Small): it drew MaxInt and MinInt, so a function that
  overflows only at the extremes failed natively and passed on Go
  (the website tour's deposit; TestContractFuzzSmallInts).

- An `else if` chain is parsed, checked, walked by the flow passes, and
  lowered to SSA in a loop instead of a call per `if`: in the wasm build
  of the compiler (the playground) a 200-branch chain needed more than
  2 MB of stack and now builds in 0.6 MB. The same code on every golden;
  golden else_if_chain (300 branches, a 300-term sum).
- Time zones need no Go: std ships the IANA database (std/tzdata,
  2026e; decision 0094's addendum), which native and Go backend builds
  embed per use as before; scripts/tzdata-update.sh refreshes it per
  release. `--tzdata=system` still reads the operating system's.

- A plain HTTP server links no TLS server and no HTTP/2 (decision 0244,
  owner): they are reached only through what `Server.with_tls` sets up,
  so json_server is 62 percent smaller on arm64 (2.5 MB to 944 KB
  stripped) and 70 percent on LLVM; HTTPS servers are unchanged.

- A native spawn passes its closure as its code and environment, not in
  a list of one, and a task with a () result needs no result list
  (decision 0068's addendum): bench/perf/conc spawn at XO_THREADS=1 on
  macOS 2,250 to 1,843 instructions a spawn (1.27 times rust-ct's,
  from 1.55), about 15% fewer cycles. The task object is zeroed with
  constant-size stores (Linux: 168 instructions a spawn fewer).
- ELF objects from the arm64 backend give each function symbol its
  size (to the next named symbol), so valgrind and perf name Xo
  functions on Linux (callgrind showed ??? for all of them).
- XO_THREADS=1: main's own runtime calls no longer wake the one worker
  for tasks main deferred and will run at its join (decision 0077); on
  Linux spawn-heavy code used two threads for one's work (bench/perf/conc
  spawn: 93 to 36 ms of CPU, 4.7 to 1.8 times rust-ct's).
- Mutex: an unlock no longer signals a waiting writer whose park is
  already signaled and that has not run yet (found with the execution
  trace, decision 0240); bench/perf/conc mutex at the default threads
  31 to 39 Mcycles against 46 to 57 (macOS, three rounds of 15 runs at
  load 17 to 29), rwmutex within noise.
- Contract fuzzing on the native backends (decision 0250): `xo test`
  fuzzes every function with contracts natively, with the Go backend's
  targets, cases, seeds, shrinking, and regression tests (a regression
  written by one backend replays on the other). A 20 contract module:
  the check 0.19 to 0.27 s before (20 skipped), 0.40 to 0.44 s now (20
  fuzzed, 100 cases each), the whole `xo test` 0.98 to 1.02 s.

- An execution trace for native programs (decision 0240): XO_TRACE=FILE
  (`xo run --trace=FILE`, `xo test --trace=FILE`) records spawns, task
  runs, parks with their reason (chan send and recv, select, mutex,
  join, sleep, net, http), wakes and hand offs, deferred and late
  wakes, worker idling, the monitor and the poller; `xo trace FILE`
  writes Chrome trace JSON for ui.perfetto.dev and a summary (time
  parked by reason, runnable latency, top wakers, longest parks). Off,
  a hook is a load and a branch; `xo build --release` leaves the
  recorder out unless `--trace-support`. arm64 branches on a compare
  with 0 as cbz or cbnz.

- The wasm build of the compiler (the playground, decision 0242) is
  28.7 MB instead of 51.7 MB and builds a program in 1.53 s under Node
  instead of 8.5 s: the SSA builder's RC pass runs sequentially in a
  wasm build, so nothing in the module waits and no function needs
  suspension frames. `--annotate` notes why a function uses the
  dispatch loop. The SSA builder's read_var walks single-predecessor
  chains in a loop (the same code on every golden), and `--play build`
  refuses a program that imports std/http before checking the HTTP
  runtime: the new playground downloads 2.13 MB instead of 23.5 MB and
  runs a program in 0.45 s instead of about 2.7 s in Chrome.

- The native profiler on windows/amd64 (decision 0238 "Windows"):
  XO_PROF=cpu runs a sampler thread that suspends each thread whose CPU
  moved and walks it by the image's .pdata and RtlVirtualUnwind;
  XO_PROF=heap walks the allocating thread the same way. Windows links
  keep a COFF symbol table (lld-link /debug:symtab), which names the
  functions, also in a fault's XO_TRACEBACK (which had no frames on
  Windows). Checked under Wine with Box64 only.
- Native is the default backend (decision 0248): `xo build`, `run` and
  `test` with no `--backend` use arm64 on arm64 hosts and LLVM elsewhere.
  `--backend=go` stays; a `use go` program with no `--backend` is a usage
  error naming the package; `--hybrid`, `--holes`, `xo test --explore`,
  `xo run --seed`/`--record` pick the Go backend and `--race` LLVM. A
  native `xo test` says how many contracts it did not fuzz (contract
  fuzzing needs `--backend=go` until native fuzzing lands).

- Native builds check the runtime's function bodies only as the SSA
  builder reaches them (decision 0247): a hello build 947 to 622 ms,
  the 180 goldens 21% faster on arm64, the same binaries and LLVM IR on
  every golden. `xo check --no-gc` still checks every body, and
  TestStdChecks (-short) checks all of std's.
- The checker walks a long chain of operators (`a + b + c + ...`) in a
  loop instead of recursing once per term, and std/native/llrt/tzhost.xo's
  138-term string is one literal: the wasm build of the compiler
  overflowed a browser worker's stack on it.
- Cross builds: `xo build --targets=linux/arm64` on a Mac (or any host)
  now produces a runnable static Linux binary for a program with no C
  library, linked by Xo's ELF linker (decision 0243); `-s` drops the ELF
  linker's local symbols.

- linux/arm64: with no `--ld`, a program with no C library now links with
  Xo's own ELF linker (static, no C toolchain at link time; decision 0243
  step 3); `XO_NOLIBC=0` and `use c` programs keep cc and glibc.

- `xo build --release` leaves the native profiler out unless
  `--profile-support` (decision 0238's addendum, the owner's rule): the
  SSA builders fold prof.supported(), so neither prof.xo's sampler nor
  heapprof.xo is linked; XO_PROF on such a binary prints one line. A
  profile file is created new: O_EXCL, no symbolic link followed, mode
  0600 (internal/llvm TestProfGating). `xo run` and `xo test` help name
  --profile=heap and cpu,heap.
- Linux without C (decision 0243): a Linux program with no `use c` links
  no C library by default, on the arm64 and LLVM backends (XO_NOLIBC=0
  links glibc). selfhost/macho's ELF linker has a static mode (plain
  ET_EXEC, no loader), so `--ld=macho` links these with no C toolchain;
  its assembler gains `:got_lo12:` (a prefix length bug), `msr
  tpidr_el0`, `.size .-SYM` and 4 byte debug pointers. The toolchain's
  own Linux self build keeps the C library until a Linux fixed point
  passes with none.

- The heap profile samples in the heap's counting branch instead of
  switching the program to the threaded heap (decision 0238): with the
  profile on, a single threaded allocation bound program pays 1.22x to
  1.33x (was up to 2.05x); off, the heap is as before the profile
  (bench/results/prof-heap-2026-10-16.md).
- The wasm backend runs the TLS servers, signing, and every crypto
  golden: in a WASI module the native runtime's entropy and environment
  reads go to the wasm runtime's WASI random_get and environ_get
  (decision 0231's addendum, owner 2026-10-16). TestSelfhostWasm: 154
  pass, 26 unsupported, 0 fail of 180 (was 148, 31, 0).

- XO_THREADS=1 (decision 0077's addendum): main's spawns no longer wake
  the one worker per task; main's join or next park runs them, else the
  monitor's tick (about 1 ms), as Go's GOMAXPROCS=1. bench/perf/conc at
  one thread: spawn 0.37x CPU, fanout 0.51x, pingpong 0.69x
  (internal/llvm TestOneThreadSpawnJoin).
- The heap profile off costs about 1% at most (decision 0238): the
  profile's armed bit is bit 1 of a thread's heap word (obj.xo ARMED),
  which the threaded allocation and free already hold, instead of a load
  of the heap record's H_PROF word: against no hooks, tasks 1.009 (was
  1.023), maps 1.010, the rest 1.000 to 1.006
  (bench/results/prof-heap-2026-10-16.md).
- arm64 inliner: the runtime's cold slow paths (the list LLVM marks cold,
  now ssa.cold_fn) are never inlined, and a leaf's constants do not count
  toward its size. arm64 sweep, 151 workloads: 13 faster (atomic
  contended 0.915, ctx budget 0.922, ctx mutex 0.973, float_fmt 0.973),
  1 slower at 1.001. Variants that freed every constant made the
  digests 5 to 8% slower (bench/results/arm64-inline-cost-2026-10-16.md).

- Constants fold in the SSA builder for `*`, `/` and `%` as for `+`
  and `-` (selfhost/ssa expr.xo): `W_DEFER_AT / 8` was a `mov` and an
  `asr` at run time on arm64 (the LLVM backend folded it). A product
  that overflows, a division by zero, and MIN / -1 or MIN % -1 stay the
  checked operation and fault at run time. test/codegen/arith:
  word_index, word_rem, by_zero, fold (now the constant), times3.
- Fix: the native TLS client and server sent records through the raw
  address of a Bytes the compiler had already released (a value is
  released after its last named use, and a raw address is not a use),
  so a send that parked in the event loop could write freed memory.
  An audit of every raw address use in std/native and selfhost
  (bench/results/raw-address-audit-2026-10-16.md) fixed 15 such sites
  (TLS flush, HTTP/2 frames and headers, the HTTP client's key, the
  peer address, process spawn and executable path, the test world's
  environment) with the new `raw.alive` and `raw.alive_str`, which keep
  a value until that point; the rt_addrs golden checks it after
  allocations that would reuse the bytes.

- arm64: four instruction selections for bit tests and atomics. A
  negative mask is a logical immediate (`x & -4` one `and`, was a `mov`
  and an `and`; a negative literal reaches isel as a negation of its
  constant); `(x & C) == 0` or `!= 0` is `tst x, #C` with no `and`; a
  one bit C branches with `tbz`/`tbnz` on that bit (was bit 0 only); a
  relaxed `rt.slot_load` or `slot_store` with a constant index is an
  `ldr`/`str` at the base plus the offset (was an `add` and the access:
  stdio's buffer length). arm64 sweep (bench/perf measure.sh suites but
  the socket ones): 14 faster (json parse 0.965, strings 0.977, ctx
  none 0.858), none slower beyond noise. test/codegen/arith checks the
  three folds. Also: bench/perf gen, glob, report and testworld's
  measure.sh skip the .dSYM's DWARF file when finding the test binary
  (it was found first since the line tables, decision 0239).
- Fix: the DWARF reader (tracebacks, profiles) read the debug sections
  through addresses into a Bytes that was already released, so some
  builds (seen on macOS with LLVM) got wrong or missing lines and inlined
  frames; it now holds the Bytes (gotcha raw-address-outlives-bytes).
  The heap profile names inlined callees too (TestProfHeap's start), and
  TestTraceback and TestTracebackInlined run on LLVM as well as arm64, so
  CI's linux/arm64 runner checks LLVM's tracebacks.

- `--race` on the LLVM backend (decision 0241's addendum): `xo
  build|run|test|asm --backend=llvm --race` marks every function, the
  runtime's too, sanitize_thread and builds with ThreadSanitizer (about
  25 times the time, 12 times the memory: a checking build); arm64 and
  wasm refuse it ("use --backend=llvm"). TestGoldenTasks and the
  runtime tests build with it, and the runtime is clean: plain accesses
  to words other threads update atomically became relaxed atomics or
  acquire/release pairs (pool.xo's deferral words, poll.xo's timer
  flag, stdio.xo's buffer length, obj.xo's count checks, rtstats.xo's
  switch, mutex.xo's reader grant), checked over the 40 golden
  programs that use tasks and the three runtime tests.

- The native HTTP client decodes gzip bodies (std.md 4.5): it asked for
  gzip (Accept-Encoding, as Transport does) but returned the compressed
  bytes; now a gzip body is decoded without Content-Encoding and
  Content-Length on HTTP/1.1 and HTTP/2, as on the Go backend, and a
  request with its own Accept-Encoding or Range gets the body as sent
  (gotcha native-http-client-gzip-body, golden http_gzip).

- Inlined calls in the profiler and tracebacks (decision 0239): dwline.xo
  reads .debug_info's DW_TAG_inlined_subroutine entries (arm64's and
  LLVM's), so pprof shows inlined callees as their own (inline) lines
  and the summary as their own rows, and XO_TRACEBACK prints them marked
  `[inlined]`. arm64 writes nested inlined frames (lldb: inner, then
  outer, both [inlined], then main).

- The last C of the runtime's tests is gone but win_consts.c (decision
  0241): TestCryptoDiff and crypto_diff.c/crypto_ref.c (which checked
  the C hashes decision 0198 had already deleted, not std/crypto's Xo,
  which TestDigestDiff and TestCryptoXoDiff check), xcalls.h and xrec.h,
  and the 20 C ABI exports only the C drivers called.
- The event loop, real-time section, and C library fault tests are Xo
  (decision 0241): evloop.c, rawpoll.c, realtime.c, fault_shared.c,
  fault_shared.ll, and fault_resume.c became internal/llvm/testdata/rt
  evloop, realtime, and fault_shared, verified on darwin/arm64 and
  linux/arm64; std/internal gains rt_queued and rt_allocs (real-time)
  and rt_probe ops (qsort_fault, workers, cpus, inherited).
- arm64: x15 is an allocatable register (the last caller saved choice;
  16 instead of 15). The emitter keeps x16 and x17 as its scratch
  registers; a third spilled operand takes a register free at its
  instruction, a fault stub's recomputed operand any caller saved
  register no argument holds, a switch's spilled index x17, and the
  atomic templates that use w15 declare it (Inst.clobbers). arm64 sweep,
  150 workloads: 10 faster (argon2 0.985, chacha 0.985, p384 0.990, the
  other handshakes 0.995 to 0.998), sysio copy 1.031 in the sweep and a
  tie rerun five times. The forced spill test (XO_ARM64_REGS) passes.

- arm64: `0 - b` of rt.int_wrap is `neg` (ct.mask_bit's mask was
  `mov x9, #0; sub`). Xo Lang 2's finding.

- arm64 inlined frames in the line tables (decision 0239): the inliner
  keeps the callee's own lines and its call site; .debug_info has a
  DW_TAG_inlined_subroutine per run of inlined code, so lldb shows the
  inlined callee as a frame (`scale at main.xo:2:3 [inlined]` over
  `main at main.xo:9:5`). The function cache key holds the lines when
  debug information is on (a line-only change reused stale rows).

- The shared count and heap stress tests are Xo (decision 0241):
  rc_threads.c and alloc_threads.c (876 lines of C) became
  internal/llvm/testdata/rt/rc_threads and alloc_threads, run under
  ThreadSanitizer with the test programs' own code instrumented (xo's
  IR had never been: clang instruments only functions marked
  sanitize_thread). It found the in place string append's count check
  (obj.xo reserve) racing with another thread's release: a relaxed
  atomic load now.
- A native heap profile (decision 0238): XO_PROF=heap (XO_PROF_HEAP_RATE,
  XO_PROF_HEAP_OUT; XO_PROF=cpu,heap for both), `xo run --profile=heap`,
  `xo test --profile=heap` sample allocations every 512 KiB on average
  as Go does, with their frames, and track the sampled blocks' frees;
  at exit pprof with Go's alloc and inuse objects and bytes, and a text
  top 30 by inuse_space and alloc_space; darwin and linux, arm64 and
  LLVM (std/native/llrt/heapprof.xo, internal/llvm TestProfHeap,
  bench/results/prof-heap-2026-10-16.md).
- arm64: an adc chain takes constant operands (made before the chain's
  instruction; only 0 was taken before) and a constant carry in of 1 is
  `cmp xzr, xzr` (no mov and no register; nor does it cost the chain a
  pair). arm64 TLS handshakes: p256 0.968, tls12 0.973, x25519 0.973,
  rsa2048 0.978, ed25519 0.980, p384 0.986 (151 workloads: 7 faster; 4
  slower rows with no function changed). Xo Lang 2's finding (5).

- The task runtime's stress test is Xo (decision 0241):
  std/llrt/testdata/tasks.c (561 lines of C) became
  internal/llvm/testdata/rt/tasks, run under ThreadSanitizer with 1, 3,
  and 8 workers; std/internal gains `rt_probe` and `obj_word` for the
  toolchain's runtime tests.
- The profiler names lines (decision 0239's tables, read back by
  dwline.xo): pprof's Line.line and Function.filename (`go tool pprof
  -list fib` shows the source), and the text summary's hottest line per
  function; leaf samples are no longer charged to the previous function
  when at a function's first instruction. XO_TRACEBACK=1 prints an
  uncaught fault's frames (`  FUNCTION (FILE:LINE)`) after the one line
  report, on every native backend (spec core 8); `xo test` turns it on
  for its tests. TestTraceback, TestProfCPU checks the line.

- macOS: builds linked through cc (the LLVM backend, arm64 `--ld=cc`)
  get PROG.dSYM too: `dsymutil` runs after the link (0.15 s), and an LTO
  link keeps its object for it, so lldb and the profiler see .xo lines
  on every darwin build. A build from the binary cache gets its dSYM
  back (decision 0239).
- With 0.17.0 as the seed floor, what remained only for older compilers'
  code is gone: 42 C ABI exports nothing calls (the C runtime's, not
  the emitters' or the C test drivers'), the second personality
  routine (pers_old) and the `xo_personality` link alias (every emitter
  names `xo_x_personality` since 0.17.0), LLVM's unused `xo_print`
  declarations, and the stale forwarder comments. Proved by building
  from the 0.17.0 seed: fixed point, -short, all arm64 and LLVM goldens.
- `xo dev goldens` prunes its build directories after every run (a day
  unused, then least recently used past 2 GiB, `XO_DEV_BUILD_MAX`):
  xo prunes a build directory only where it chose the location, and
  `bin-arm64/build` had reached 15 GB.
- Scheduler (decision 0077's addendum, owner 2026-10-16): a deferred
  wake is woken by the waker's own next channel, Mutex, or spawn call
  after 50 us (the monitor being the last resort); XO_DEFER_WAKE=0 turns
  deferral off, and a program with a `uses realtime` function never
  defers (the compiler marks it at main's start). Mutex wakes stay
  direct (a deferred grant was measured and rejected). The latency model
  is in the addendum; TestDeferWakeOff, TestDeferredWakeBound (with a
  Mutex case).
- DWARF line tables (decision 0239) on the arm64 and LLVM backends, on
  by default for build, run, and test (`-s` and `xo asm`: none;
  `--release`: none unless `--debug-info`). darwin: PROG.dSYM next to
  the executable from the selfhost/macho link (lldb `b main.xo:9`
  stops there with the source); Linux: .debug_* in the ELF. Sizes in
  bench/results/dwarf-line-tables-2026-10-16.md (TLS client: dSYM 25%,
  linux arm64 +25%, LLVM +39%; compile +3%). SSA values carry their
  statement's line (rc cache v2, arm64 function cache v2).

- Scheduler (decision 0077's addendum, owner 2026-10-16): a channel's
  hand off and a ring's only spawn no longer wake an idle worker; a
  monitor thread (Go's sysmon, deferred wakes only) wakes one after 1 ms
  if the work is still queued, and sleeps when nothing is deferred; idle
  workers no longer spin (XO_SPIN_ROUNDS default 0). pingpong 0.47x CPU,
  rwmutex 0.80x, mutex 0.86x; HTTP p50 and p99 unchanged
  (bench/results/park-wake-2026-10-16.md, internal/llvm
  TestDeferredWakeBound).
- arm64: constant 0 addends of adds, adcs, and adc are xzr (no mov of
  0) and no longer stop an adc chain, and const_int and same_arg see
  through an inlined call's argument and result parameters (a literal 0
  passed to cpu.adc reached it as a block parameter, so it was hoisted
  into a register and the pairing broke). x25519.fe64_mul 173 to 148
  instructions, fe64_sq 113 to 102, nistec.mul4p 172 to 151,
  bigmod.mac_row 149 to 124. arm64 TLS handshakes: rsa2048 0.946,
  tls12 0.951, x25519 0.953, p256 0.956, ed25519 0.961, p384 0.978
  (151 workloads: 8 faster; json enc_small 1.002 and sysio tree 1.007
  with only poll.afd_done changed, Windows code: noise). Xo Lang 2's
  TLS findings (1) and (2).

- arm64: a funnel shift, `shl(a, k) | shr(b, 64 - k)` of rt.int_wrap
  (or `^`), selects extr, also when the shifts reach the `|` through an
  inlined call's continuation parameter; selfhost/macho encodes extr.
  aesgcm.ghash_hw 400 to 388 instructions (6 extr; 6 lsl and 6 lsr
  gone), 31 extr in the TLS client. arm64 sweep, 151 workloads: 4
  faster (httpclient h2big 0.994), testworld 5 1.001 at a 1.001 spread.

- A native CPU profiler (decision 0238): XO_PROF=cpu (XO_PROF_HZ,
  XO_PROF_OUT), `xo run --profile=cpu`, `xo test --profile=cpu` sample
  the program's CPU time with SIGPROF, walk the frames with the fault
  unwinder's tables, and write pprof (`go tool pprof` reads it) and a
  text top 30 at exit; darwin and linux, arm64 and LLVM
  (bench/results/prof-cpu-2026-10-16.md, internal/llvm TestProfCPU).
- darwin/arm64 links with selfhost/macho by default again (cc only with
  `--ld=cc`, `--as=cc` or `use c` sources): the launch stalls that made
  cc the default that morning were an XProtect false positive and a
  shared dev goldens path, both fixed; all 179 goldens pass three macho
  rounds and none matches XProtect. Goldens build 17% faster than with
  cc at 0.5% more bytes (decision 0182 addendum).
- arm64: `a & ~b`, `a | ~b`, and `a ^ ~b` select bic, orn, and eon (the
  complement's mvn is then dead). sha3.keccak 1098 to 998 instructions
  (100 mvn gone; its stack spills unchanged, 216 loads and 75 stores);
  an ML-KEM-768 loop (21 iterations) 0.976 of the instructions on the
  Mac. arm64 sweep, 151 workloads: 3 faster (p384 0.997, ed25519 0.996,
  text replace 0.996), text quote 1.002 with no function of it changed.

- Benchmarks: Rust baselines for bench/perf/llvm (std only) and
  bench/perf/conc (tokio and std threads; run.sh's `rust` and `rust-ct`
  configurations), bench/perf/llvm/rust.sh and rust-docker.sh (Linux
  instruction counts in Docker); results in
  bench/results/rust-baseline-2026-10-16.md.
- arm64 darwin: thread locals without the xo_tlocal_get call. The TLV
  descriptor's thunk is called inline (x0 and lr saved around it; it
  keeps every other register), and its base is reused in the block
  until the next call (a task may resume on another thread after any
  call, so never across one). Mac, instructions retired, XO_THREADS=1,
  5 reps: conc pingpong 0.866, spawn 0.974. Xo lang 4's (b).

- arm64: thread locals inline on Linux (rt.tlocal_get, tlocal_set,
  tlocal_addr: tpidr_el0 plus xo_tlocal's local exec offset, then the
  slot, instead of a call of xo_tlocal_get), and rt.mem_set of zeros
  with a constant length up to 512 bytes as stores of xzr (stp pairs)
  instead of a memset call, on every OS. callgrind linux/arm64,
  XO_THREADS=1: conc pingpong 229.7M to 219.2M instructions (0.954),
  spawn 413.4M to 388.4M (0.939; the C runtime's was 400.7M). Mac
  spawn 0.99; darwin's thread locals still call the TLV getter.

- macOS: the TLS client loads the Security framework with dlopen at its
  first certificate verification instead of every https program linking
  Security and CoreFoundation: an https client's launch drops from 28.5M
  to 14.4M instructions (llvm) and 29.2M to 15.4M (arm64); verification
  results are unchanged (badssl.com's expired, wrong host, self signed
  and untrusted root cases). Needs the 0.17.0 seed (rt.call_c4).
- Tests: FuzzHTTPDiff no longer sits idle for 20 minutes and more in a
  full internal/llvm run: its shared native server, started with the
  first seed input's t, was killed by that input's Lifeline cleanup; the
  servers now live for the fuzz test (f), http1Send stops dialing a
  server that has exited and reports its exit, and testtmp.Lifeline's
  deadline extends to the test's own (gotchas/lifeline-subtest-once.md,
  TestHTTP1SendServerExited).
- The back-end cache (`<user cache>/xo/backend`) holds its 8 GiB cap
  under bursts of builds: entries stay protected for 2 minutes instead
  of 2 hours, and a prune runs whenever an eighth of the cap has been
  stored since the last one (`prune.Policy.burst`, `prune.note_stored`),
  not only hourly; it had grown to 19 GiB in 15 minutes
  (gotchas/backend-cache-burst-past-cap.md, decision 0223 addendum).
- Linux: the auxiliary vector (AT_PHDR for the fault unwinder, AT_HWCAP,
  AT_PAGESZ) is read from memory after the start's environment instead
  of /proc/self/auxv (now a fallback): under valgrind the unwinder found
  no AT_PHDR and a fault escaped task.isolate, and without /proc (a
  chroot, a sandbox) every fault would have (gotchas/linux-auxv-proc.md,
  golden rt_addrs). Fault unwinding on arm64 against pre-zero-C main
  dd33dd13: fault defer, owned and isolate 0.63x, 0.64x and 0.71x wall
  time on the Mac, 0.43x, 0.41x and 0.37x instructions on Linux
  (bench/results/zero-c-perf-sweep-2026-10-15.md).
- arm64: the immortal empty list and zero handle come first in a
  program's data (page offset 0): in selfhost/macho's layout the empty
  list sat at page offset 0x880, and code loading it into x11 then x12
  matched XProtect's MACOS.SOMA.AUENB rule, so macOS blocked 3 of 179
  goldens as malware; now none match the Mac's rules (both linkers,
  both backends). TestEmptyListAtPageStart; TestXProtectRules with yara
  (gotchas/xprotect-false-positive-empty-list.md).
- ML-KEM: the NTT loops on raw coefficients in byte offsets, SHAKE128
  read a block at a time in sample_ntt, and Keccak's lanes as raw memory:
  per CCTV iteration arm64 5.11M to 4.68M instructions, llvm 4.19M to
  3.70M (bench/results/tls-perf-2026-10-15.md).
- AES-GCM: GHASH folds four blocks into one reduction (H^2 to H^4 kept
  with the key) and CTR runs 64 byte chunks by address: a 16 KiB record
  228K to 145K instructions on arm64, 171K to 114K on llvm; bulk TLS
  AES-128-GCM 18.8M to 13.6M a MiB on arm64 (Mbed 14.0M)
  (bench/results/tls-perf-2026-10-15.md step 15).
- Native standard input from a FIFO on macOS sees its end of input: poll(2)
  and kqueue report nothing for a FIFO whose last writer closed, so the
  reader waited forever; stdin.xo now probes a FIFO with a read that does
  not block (gotchas/native-stdin-fifo-eof.md, internal/llvm
  TestStdinFIFO).
- arm64: every spilled operand of one instruction gets its own register.
  Before, a template's fourth spilled use was reloaded into the same
  scratch register as its third, and two spilled definitions both went
  to x16 (add_with_flag's adds and cset, long adc chains): a silent
  wrong value. emit.xo now takes x16, x17, x15, then a register free at
  that instruction, and fails the build when none is left. Templates
  name any number of operands ($u10 and up). `XO_ARM64_REGS=N` (test
  only) limits the allocator so most values spill; TestGoldenRunSpill.

- Tests: internal/testtmp Lifeline: a long-running native child a Go test
  starts (the HTTP parse and fuzz servers, the TLS, TLS matrix, H2 and
  TLS server diff peers, TestGoldenASan's goldens, cmd/xo's notes
  server) runs behind internal/testtmp/lifeline, which kills it when the
  test process is gone however it ended (a pipe only the test holds,
  watched by a separate process; the program keeps its own pid, signals
  and exit status), plus a cleanup kill and a deadline
  (gotchas/test-server-children-orphaned.md).
- nistec.xo: P-384's and P-521's [k]G from a fixed base table made once
  per process on first use (pt_mul_g_n beside P-256's pt_mul_g; the
  table in static words 44 and 45, nistec_gtab.xo, rt.heap_large memory
  for the process's life), constant time as pt_mul: key generation
  and signing 0.34 to 0.45 of the instructions, verification 0.66 to
  0.69, 100 TLS handshakes 0.81 (P-384) and 0.68 (P-521)
  (bench/results/nistec-gtab-2026-10-16.md). bench/perf/tls gains a
  p521 workload. Compiled in, P-521's table had cost every TLS build
  27 s and 2.5 MB.
- A constant List[Int] of 64 or more literals is kept as data and read
  at first use (selfhost/ssa show.xo int_list_data, the native prelude's
  llrt_list_of_le64), not built by code element by element: the P-256
  table's builder was 434 to 524 KB of code; TLS binaries are 13 to 17%
  smaller on both backends (httpclient llvm 2.51 to 2.12 MB, arm64 2.85
  to 2.35 MB) at the same handshake speed.
- darwin/arm64 links with cc by default again when the host has one
  (`--ld=macho` still selects selfhost/macho), a mitigation: some ad hoc
  binaries fail the kernel's signature check (AMFI) and wait at exec on
  a Gatekeeper assessment queued behind pending dialogs; the fix is the
  Developer Tools setting for the terminal
  (gotchas/amfi-gatekeeper-macho-layout.md).
- Allocation counting is off when nothing reads it: alloc.xo counts
  allocations and their bytes (ALLOCS, ABYTES) only when XO_LEAKCHECK,
  XO_HEAPSTATS, or XO_RTSTATS is set (from the first allocation, so their
  reports are unchanged) or after a budget's first stretch (budget.xo reads
  the bytes); FREES stays counted. heapos.xo counting_off at the start,
  counting_on at a budget's mark. bench/perf on both backends: alloc small
  arm64 0.977, text split llvm 0.983, quote arm64 0.978, and most text and
  alloc rows 0.98 to 0.995.
- The unwinder's frame cache is two way set associative (32 sets of 2):
  direct mapped, two hot frames of fault isolate collided after a layout
  change and missed on every lookup (1.14x); fault defer llvm 0.908.
- selfhost/macho's assembler encodes adc, adcs, sbc, and sbcs: since
  cpu.adc's carry chains (dd3b3e2d) and the C runtime's deletion, every
  darwin/arm64 build of a TLS program failed ("unsupported instruction:
  adcs"). scripts/golden-run.sh runs without coreutils' timeout on macOS.
- rt.static_word's table grows from 64 to 256 words on every backend
  (decision 0237; no cost per access). std claims only words 0 to 63
  until a seed whose SSA builder accepts more (TestStaticWords); words
  12-13 and 44-47, the C runtime's leftovers, are free now (poll.xo's
  event loop record moves to word 43).
- XO_NOLIBC=1 (decision 0230's Xo C library): the 68 cabi_libc.xo
  exports nothing calls in such a build are deleted with the libc*.xo
  helpers only they used (736 lines; found from the arm64 Linux assembly
  of every golden, the C runtime's undefined symbols, and the mem*
  calls code generation makes, then run in Docker on linux/arm64 and
  linux/amd64). New std/native/llrt/libcmath.xo: trunc, floor, ceil,
  round and fmod in Xo by the IEEE 754 bits (fmod: musl's exact
  algorithm), the calls LLVM makes for Float's rounding on x86-64
  without SSE4.1 and for `%` of Floats on every target, which no XO_NOLIBC
  build linked before (golden float_libm).
- std/llrt/xo_rt.c is deleted (decision 0234): seed 0.16.0's driver
  compiles no C runtime, so the empty file kept for the 0.15.0 seed goes.
  std/llrt holds only the C tests (testdata).
  With it go the prebuilt runtime packs (decision 0157, superseded):
  `xo rtpack` and `xo dev rtpack`, the runtime object cache, the pack
  lookups of --ld=macho and the ELF linker, the pack part of the binary
  cache keys, and release.sh's pack builds (and its zig requirement).
- Env.only on Windows: the lists admit variable names without ASCII case,
  as Windows (and Go's os) compares them, in both backends (prelude.xo
  env_admits, std/rt attenuate.go): `only(["path"]).get("PATH")` and the
  `Path` Windows sets under `only(["PATH"])` were refused. Spec std.md 3.5;
  golden internal/llvm/testdata/windows/env_case, passing under Wine with
  Box64.
- Windows faults: unwind.xo win_raise compares the frame's own stack
  pointer with the fault boundary (it compared its caller's after
  RtlVirtualUnwind and stopped before main's or a task body's cleanups and
  catches). First run of the Windows fault path, under Wine with Box64:
  24 windows/amd64 fault and task goldens pass (10 failed before; two
  http goldens differ only by Box64's library messages on stdout).
- arm64 emitter: the element callbacks (dupp, dropp, sharep) of a struct
  or tuple count its heap leaves in place, reading each from the element
  (selfhost/arm64 rc.xo elem_helper), instead of loading all its words
  for the struct's helper, which saved them again (past 8 words with a
  frame and a stack copy). std's memfs test (a Map of 9 word nodes,
  shared on every operation): 12.27G to 11.42G instructions on arm64.
- No runtime C (decision 0234): Windows' stack overflow handler is Xo
  (std/native/llrt/entry.xo on_overflow, the cabi.xo export registered
  with AddVectoredExceptionHandler through rt.fn_addr; it flushes
  standard output with flush_try's try lock, writes "fatal error: stack
  overflow" with WriteFile, its count in rt.static_word 61, and ends the
  program with status 2; no allocation). stop.xo's C services record,
  sys_c.xo xo_c_stop_svc and E_OVERFLOW are gone. The driver no longer
  compiles std/llrt/xo_rt.c into a program (link_inputs), xo's own
  linkers no longer take a pack's rt.o, and xo rtpack compiles xo_rt.c
  only if it exists; xo_rt.c is empty and stays only for the 0.15.0
  seed's driver, to be deleted after the next seed. The C tests no
  longer include it. Tested under Wine: internal/llvm/testdata/windows/
  stack_overflow passes for windows/amd64 with Debian's amd64 Wine run by
  Box64 in a native arm64 container (scripts/docker-wine.sh box64, new;
  Rosetta's amd64 emulation cannot run Wine's exception dispatch,
  gotchas/wine-rosetta-no-exceptions.md); windows/arm64 is cross linked
  only.
- Fault speed: std/native/llrt unwind.xo caches each frame's lookup
  (the FDE or compact entry and its rules after the CFA program) by pc,
  64 entries per thread, so a fault unwinding through frames it has seen
  skips read_fde and exec. bench/perf fault (instructions, darwin/arm64):
  defer arm64 1957M to 993M, llvm 1221M to 575M; owned arm64 1128M to
  535M, llvm 705M to 618M; isolate arm64 318M to 187M, llvm 297M to 146M;
  output identical. Against the resweep's pre-zero-C base (arm64 defer
  1.224, owned 1.206, isolate 1.088) about 0.62, 0.57 and 0.64.
- Linux in Docker: compiler-rt's sanitizer runtimes in the image; the
  sanitizer tests (TestSysCTSan, TestGoldenTasks under ThreadSanitizer,
  TestGoldenASan, TestRuntime*, TestDeadlock) run on linux/arm64 with Go
  1.26 and clang 19 (XO_DOCKER_BASE=golang:1.26-trixie, XO_DOCKER_RW=1):
  all pass but fs_ops_edges, whose times, read only mode, and locks a
  directory shared from macOS does not keep. rawpoll.c includes unistd.h.
- Unwinder, x86_64: a landing pad's stack pointer gets the frame's
  DW_CFA_GNU_args_size added (std/native/llrt unwind.xo), as libgcc does:
  a fault through a call whose stack arguments x86_64 LLVM pushed
  (std/cli's command) crashed. Docker linux/amd64 (emulated), all 178
  goldens with the LLVM backend: 175 before, the 3 fault goldens fixed.
  golden-run.sh ends a hung program after XO_GOLDEN_TIMEOUT (600) s.
- The checker treats `File`, `FileLock`, `Child`, and std/internal's
  `SharedLock` as handles (decision 0112 addendum): `==`, `!=`, Map keys,
  Set elements, and `derive Eq` on values holding one are XO0404 on every
  backend. Before, `Result[File, FsErr] == ...` compared by identity on
  the Go backend and stopped at XO1201 natively.
- LLVM backend, linux/arm64: the thread local table is read through
  llvm.threadlocal.address. LLVM had folded its address into callers,
  which kept tpidr_el0 across a park, so a task resumed on another worker
  used the first thread's table and switched to that thread's scheduler
  stack: 19 of 178 goldens crashed in Docker (scopes, channels, net, http,
  timers; an old bug, the 0.14.0 seed's builds too). Docker runs now use
  the host's uid (fs_ops_edges' read-only check fails as root) and
  golden-run.sh keeps a golden's directory name (fs_cwd_path).
- A native test binary's MemFs has every Fs method: `open`, `create`,
  `open_append` (File handles: `read`, `read_line`, `write`, `close`),
  `append`, `copy`, `mode`, `set_mode`, `glob`, `lock`, and `try_lock`
  (a held lock makes another task wait; FileLock `unlock`), with std/rt's
  behaviour and error texts (std/native/testing.xo; the native prelude's
  File and FileLock carry a MemFs number, rt.mem_file and rt.mem_lock;
  selfhost/ssa routes File and FileLock methods in test binaries). No
  XO1201 is left for MemFs. cmd/xo testdata/nogc/same has a test per
  method and testdata/nogc/real real handles under `--real`, each the Go
  backend's report on arm64 and LLVM.
- Runtime C (owner, 2026-10-15): xo_rt.c is Windows' stack overflow
  handler and its services record only. The C tests' setup is a
  constructor in std/llrt/testdata/xcalls.h (XO_RT_NO_MAIN is gone); the
  Winsock, errno and OVERLAPPED_ENTRY asserts are
  std/llrt/testdata/win_consts.c, compiled for windows/amd64 and arm64
  by internal/llvm TestWinConsts with the Windows cross toolchain; the
  Windows headers but windows.h and the #pragma comment(lib) lines are
  gone, and the driver passes the import libraries (kernel32, ws2_32,
  bcrypt, synchronization, ntdll, shell32, crypt32) on a Windows link.
- bench/perf/tls: server workloads (srv-rsa2048, srv-rsa4096,
  srv-p256): the Xo server's instructions per handshake against Go's
  client; RSA 2048 server handshakes 30% to 33% fewer instructions with
  the bigmod change, 4096 43% to 46%
  (bench/results/rsa-bigmod-2026-10-16.md).

- RSA: bigmod.xo's Montgomery product as a double width product (or a
  dedicated square) and Montgomery's reduction in single pass rows on
  cpu.adc: signatures 32% to 46% fewer instructions, verifications 44%
  to 53%, on both backends (bench/perf/rsa, new;
  bench/results/rsa-bigmod-2026-10-16.md); TestRSADiff against
  crypto/rsa for 1024 to 4096 bit keys.

- P-256 handshakes: nistec's final subtraction for P-256 with constant
  limbs, and add4p and sub4p as add with carry chains: 7.7% to 9.0%
  fewer instructions on arm64, 4.2% to 5.3% on LLVM
  (bench/results/tls-arm64-gap-2026-10-16.md).

- RSA and P-521 on arm64: bigmod's multiply and add rows at 7
  instructions a word, the squaring's carries as chains, the
  exponentiation's window selection in registers, and P-521's field
  product on those rows: RSA server handshakes 26% fewer instructions on
  arm64 (2% to 5% on LLVM), P-521 handshakes 35% (18% on LLVM)
  (bench/results/tls-arm64-gap-2026-10-16.md).

- X25519 on arm64: the generated 64 bit field
  (scripts/x25519fe64.py) adds no literal zeros: fe64_mul 148 to 138
  instructions, handshakes 1.5% to 2.4% fewer on arm64
  (bench/results/tls-arm64-gap-2026-10-16.md).

- P-256 and the NIST curves: nistec's final subtraction as one add with
  carry chain and the scalar multiply's constant time table selection
  over the live words only: P-256 handshakes 9.3% fewer instructions on
  arm64 and 6.2% on LLVM (bench/results/tls-arm64-gap-2026-10-16.md,
  with the arm64 against LLVM table of every TLS workload).

- SHA-256 with the CPU's instructions (decision 0231's SHA-256
  section, owner 2026-10-16): cpu.sha256_block, SHA256H, SHA256H2,
  SHA256SU0, and SHA256SU1 on arm64 through both native backends, the
  portable body elsewhere; TLS and std/crypto's sha256, hmac_sha256,
  and PBKDF2 take it. Handshakes 3% to 8% fewer instructions (an ECDSA
  server's 13% to 14%), PBKDF2 4.2 to 4.7 times fewer
  (bench/results/sha256-hw-2026-10-16.md); TestSHA256HWDiff.

- arm64: the cpu.adc chains are back on (the emitter's distinct
  registers for spilled operands, 4bf54541), up to 8 pairs an
  instruction: TLS handshakes 7% to 9% fewer instructions on arm64
  (bench/results/tls-perf-2026-10-15.md).
- arm64: each cpu.adc pair is an instruction template of its own again
  (calls.xo ADC_CHAINS off): a chain of up to ten operands could reload
  a fourth spilled operand into x15 over the third, silently; arm64
  handshakes about 10% slower until the emitter serves longer templates
  (gotchas/arm64-template-scratch-reuse.md, internal/arm64
  TestNoSharedScratchReload).

- TLS: a P-256 squaring of 10 products (nistec.xo sqr4p) and X25519 on
  four 64 bit limbs modulo 2^256 - 38 on adc chains (x25519.xo,
  scripts/x25519fe64.py): the X25519 handshake 7.5M to 7.1M instructions
  on LLVM (10.8M to 10.5M on arm64), RSA-2048 8.4M to 7.9M, TLS 1.2 9.1M
  to 8.4M; TestX25519Diff (bench/results/tls-perf-2026-10-15.md steps 13
  and 14).

- cpu.adc and cpu.adc_carry, add with carry in (decision 0231's
  addendum): adcs chains on arm64 and LLVM; P-256's field product sums
  with them, the X25519 handshake with a P-256 chain 8.8M to 7.5M
  instructions on LLVM and 11.5M to 10.8M on arm64
  (bench/results/tls-perf-2026-10-15.md step 12).

- TLS: P-256's field product reduces by shifts (nistec.xo mul4p,
  generated by scripts/nistecmul4p.py) and P-256's point formulas call
  the 4 limb field functions without dispatch: the X25519 handshake
  with a P-256 chain 10.5M to 8.8M instructions on LLVM (11.5M on
  arm64), RSA-2048 10.4M to 9.3M, TLS 1.2 12.1M to 10.4M; TestNistecDiff
  checks every NIST curve against crypto/ecdsa
  (bench/results/tls-perf-2026-10-15.md step 11).

- LLVM backend: the runtime's slow paths (the pool lock's, the futex
  wake, the heap's reserve and push, budget, mutex, stdio) are noinline
  and cold, and a test block's function is noinline, so neither takes
  the registers of a caller's hot loop: testworld rand against
  pre-zero-C main 1.081 to 1.028, atomic get 1.058 to 0.996, the rest of
  bench/perf within noise (bench/results/llvm-rand-atomic-2026-10-16.md).

- `xo test --no-gc` runs its batches as parallel processes: chunks of
  whole tests (a property test's cases together, one module directory
  each), XO_TEST_JOBS at once, else one per physical core; the report is
  the sequential one (226 comparisons). nogc/prop's fixture 0.46 s to
  0.26 s warm on 12 cores. The Go tests give the xo they run
  XO_TEST_JOBS=2 (internal/xotool TestJobsEnv); the run cache's key
  leaves XO_TEST_JOBS out (decision 0235).

- `xo test --no-gc`: the time limit's watchdog no longer polls (a
  select on the end of the run's output and `after limit`): the 20 ms
  poll made each test process and shrinking replay up to 20 ms slower,
  nogc/prop's fixture 0.92 s to 20.6 s warm, now back under 0.5 s;
  TestNoGCShrinkOverhead guards it (gotchas/polling-watchdog-scope-wait.md).

- `xo test --timeout=D`: the real time each test, property case, and
  shrinking replay may take (default 60s, the Go backend's limit; 0 for
  none, also on the Go backend). Natively a test past it fails with the
  Go backend's message and its log records, its process is killed, and
  the next test runs (decision 0235, spec core 9.2; TestNoGCTimeout).
  The native harness now reads each test process's streams as they come.

- `xo test --no-gc` shrinks a failing property test's input over a
  pipe: one test binary serves the replays (XO_TEST_SERVE), a case not
  smaller than the best skips its body without ending it, and only an
  improving replay starts a new one; nogc/prop's fixture 2.4 s to 0.9 s
  warm, reports unchanged (decision 0235,
  bench/results/xo-test-batched-2026-10-15.md).

- XO_NOLIBC=1: the backends' process entry ran in Docker (linux/arm64
  native, linux/amd64 emulated: hello and five goldens, cross linked on
  the Mac), so std/llrt/xo_nolibc.c's C entry (_start, xo_nolibc_start,
  nl_run) and XO_NOLIBC_C_START are deleted, and with the data symbols
  already the emitters' (decision 0234) the whole file goes: an
  XO_NOLIBC build has no C beyond xo_rt.c. The entry stubs no longer
  store environ and the standard streams (cabi_nolibc.xo nl_start does).
  New scripts/docker-linux.sh (a command in a Debian container with
  clang and lld, either architecture) and scripts/golden-run.sh (a built
  golden program against its want.txt).
- The test binary cache (XO_BIN_CACHE, `<user cache>/xo/testbin`)
  prunes itself to 4 GiB (XO_BIN_CACHE_MAX bytes or XO_CACHE_MAX_GB),
  least recently used first, at most every 2 minutes, keeping only
  entries used in the last 10 minutes (it had grown to 12 GB because
  every entry was younger than the two hour keep of decision 0223);
  selfhost/prune's Policy gains `keep` and mark_used_within.
- Native test binaries' MemFs has symbolic links: `symlink` and
  `read_link`, and `stat`, `exists`, `read`, and `read_text` follow a
  link as the Go backend's MemFs does (std/native/testing.xo). selfhost/check's
  tests build and pass on arm64 and LLVM with the Go backend's report
  (they stopped at XO1201 before); cmd/xo TestNoGCTests compares them,
  and testdata/nogc/same has a symbolic link test.
- Decision 0234, step 2 (after seed 0.15.0): `main` is Xo
  (std/native/llrt/cabi_c.xo, exported under its C name): entry.xo init
  does what xo_rt.c's main and fault_init did (the main heap record,
  fault.xo and ctx.xo, the event loop's record, stop.xo with the C's
  services record through sys_c.xo xo_c_stop_svc), then entry.xo main
  with rt.sym_addr("xo_entry"). xo_rt.c keeps Windows' on_overflow and
  its services record, the Windows headers and asserts, and the C
  tests' constructor (cabi.xo rt_init); cabi.xo's heap_start, rt_start,
  poll_svc, stop_init and entry_main exports and XO_RT_TEST_THREADS are
  gone. Runtime C 343 to 262 lines.
- `xo test --no-gc` starts each test binary directly (os.proc's cmd
  with the directory and environment) instead of through `sh -c 'cd
  ... && exec env ...'`: about 7 ms less per run; nogc/prop's fixture
  5.7 s to 2.4 s, selfhost/lexer with XO_TEST_BATCH=0 4.1 s to 2.0 s;
  reports unchanged (428 old against new comparisons)
  (bench/results/xo-test-batched-2026-10-15.md).

- `xo test` on the native backends keeps a run cache (decision 0236
  item 3, selfhost/xo testcache.xo): with nothing changed (every std
  file, each module directory, xo.mod and xo.lock up the tree, xo
  itself, the toolchain, XO_ variables), the stored test binary and test
  list run without a check or build. examples/notes-service warm: arm64
  4.0 s to 1.5 s, LLVM 6.4 s to 2.6 s; selfhost/prune 1.1 s to 0.1 s.
- `xo test --no-gc` runs tests and property cases in batches, many per
  process (decision 0235, XO_TEST_BATCH): the test binary's main loops
  over the test world's next run and resets the test world between
  runs, and a run the process does not survive runs again alone, so
  reports are unchanged (both modes compared byte for byte over the xo
  unit tests, cmd/xo nogc fixtures, notes-service, the testing goldens
  and bench/xo's tests, on arm64 and LLVM). selfhost/lexer's warm `xo
  test` 4.65 s to 1.24 s (614 processes to 1); plain test packages 4 to
  7 percent faster (bench/results/xo-test-batched-2026-10-15.md).
  XO_TEST_BATCH=0 restores one process per test and per case.

- `xo test` (decision 0236): the LLVM backend builds test binaries at
  -O0 with no LTO unless `XO_LLVM_OPT` names a level, and test binaries
  are cached in the user cache's xo/testbin when `XO_BIN_CACHE` is unset
  (`off` turns it off). examples/notes-service on LLVM: 42.8 s to
  11.9 s cold, 43.2 s to 6.4 s when nothing changed
  (bench/results/xo-test-time-2026-10-15.md).
- Decision 0234, step 1: std/native/llrt/cabi_c.xo exports under exact
  C names in every native program (for `main`, step 2 after seed
  0.15.0); rt.sym_addr("name") gives a C level symbol's address
  (xo_entry, a C function: adrp/add or a GOT load on arm64, a declared
  `@name` on LLVM); an XO_NOLIBC build's environ, stdin, stdout, and
  stderr are defined by the emitters and set by cabi_nolibc.xo nl_start
  (xo_nolibc.c no longer defines them); the XO_NOLIBC printf family
  (four C shells, cabi_libc.xo vsnprintf and vfprintf, libcio.xo's
  formatter) is deleted, as no C of such a build calls it.
- Compiler speed: the compiler's identity for the back-end caches (the
  SHA-256 of the 54 MB xo executable, computed by three phases on every
  build) is memoized in <backend cache>/selfid/ under the hash of the
  executable's path, size, and modified time (selfhost/xo bcache.xo
  self_id). With the add_landings fix, a warm arm64 `xo test` of
  examples/notes-service went from 3.8 s to 2.3 s (insert_rc about 390 to
  280 ms, fn_keys 210 to 107 ms).
- Compiler speed: the SSA pass add_landings fills the rest of a split
  block once, when it reaches it, instead of copying the block's tail at
  every operation that can fault (quadratic in a block's operations):
  examples/notes-service's arm64 test build spent 1,407 ms in it on every
  run, warm or cold, now 32 ms; the generated code is byte identical
  (notes-service, conc_scopes, json_text).
- XO_NOLIBC=1 (Linux, no C library): the process entry is emitted by
  the backends (selfhost/ssa ir.xo entry_stub_arm64 and
  entry_stub_amd64, through the arm64 emitter's assembly and the LLVM
  emitter's module asm, first split module only): _start records the
  initial stack in Xo, stores environ and the standard streams, sets
  tpidr_el0 (arm64), switches to main's 1 GiB stack, and calls
  main(argc, argv, envp), then exit. std/llrt/xo_nolibc.c's C entry
  stays behind XO_NOLIBC_C_START=1 until CI has run the stubs. Also: the
  nolibc flags follow the target architecture (a linux/amd64 LLVM cross
  build from an arm64 Mac took arm64's), and a weak platform call of a
  name the module exports (__errno_location) no longer declares it a
  second time. Cross linked on the Mac for linux/arm64 and linux/amd64
  (hello and five goldens); UNTESTED on linux.
- LLVM builds of one module pick their LTO by optimization level:
  none at `-O0`, full otherwise, `XO_LLVM_LTO=full|thin|none` to
  choose. The LLVM self build of selfhost/xo at O0 takes 129 s instead
  of 1,871 s; O2 keeps full LTO (thin and none lose run time,
  bench/results/llvm-single-lto-2026-10-15.md).
- arm64 emitter: Int operations on constants are folded again after
  inlining (selfhost/arm64 inline.xo fold_consts), so a helper's
  parameter that is a constant at the call (a shift or rotation by
  64 - n) no longer leaves a run-time subtraction whose result is
  hoisted and spilled; the whole bench/perf set on arm64: 14 workloads
  faster (up to 1.6%), none slower beyond noise. Counting one-block
  callees as no block was measured and rejected (41 workloads slower);
  bench/results/arm64-inline-2026-10-15.md, and
  gotchas/arm64-inline-one-level.md corrected (nested inlining works;
  the block limit is what kept such helpers calls).
- Native runtime: xo_rt.c keeps only what is C by design (244 lines,
  with xo_nolibc.c 372 lines of runtime C): the C tests' types,
  placeholders, ev_start and XO_NORETURN moved to
  std/llrt/testdata/xrec.h, and Windows' _setmode calls on the
  standard streams are deleted (Xo writes and reads them with
  WriteFile and ReadFile on their handles, and opens files with
  _O_BINARY, so the C runtime's text mode never applies; UNTESTED on
  windows). The zero-C audit lists the end state and why each piece
  stays.
- Native runtime (after seed 0.14.0): the C only the 0.13.0 seed's code
  called is deleted: xo_heap_large, xo_heap_large_resize and
  xo_heap_release (libcmem.xo's exports), xo_sched_run_task (cabi.xo
  fiber_run_task), xo_flush and xo_set_grace (cabi.xo flush and
  stop_set_grace), xo_rl_copy and xo_rl_grow (obj.xo rl_copy_n and
  rl_grow_n), and the weak definitions of the immortal placeholders
  (every program defines them). The C tests call obj.xo's exports
  through testdata/xcalls.h.
- ML-KEM and SHA-3 (std/native/llrt mlkem.xo, sha3.xo): Keccak in locals
  in Go's in-place form, wrapping field arithmetic, in-place sums of
  products, constant tables read once, lane-wide squeezes. ML-KEM-768
  per TLS handshake: arm64 4.66M to 1.64M instructions, llvm 1.90M to
  1.31M (bench/results/tls-perf-2026-10-15.md); outputs unchanged.
- Native runtime (decision 0233 step 2, the last services): rt.sched_rt
  and xo_rt.c's xo_sched_rt are gone (the empty list a canceled
  operation returns is a `[]` in tasks.xo; op 16 had no caller left).
  entry.xo starts the poller, installs the signal or console handler,
  frees the constants, reads the statistics and sets the heap's mid_max
  itself (rt.fn_addr of cabi.xo's exports, heapos.xo), so the C's
  stop_svc keeps only Windows' on_overflow, and fault_init makes the
  event loop's record. xo_rt.c 348 lines.
- Native emitters, before the 0.14.0 seed (zero-C inventory's emitter
  items): rt.flush and rt.set_grace call cabi.xo's flush and
  stop_set_grace; the LLVM emitter calls obj.xo's rl_copy_n and rl_grow_n
  exports for `reuse` and a full list's growth (no C level, as arm64);
  every program defines the immortal placeholders xo_empty_list and
  xo_zero_handle (arm64 end_data, LLVM globals). xo_rt.c keeps weak
  placeholders and the xo_flush, xo_set_grace, xo_rl_copy and xo_rl_grow
  forwarders for seed 0.13.0's code (delete after 0.14.0), and loses the
  dead Windows xo_personality handler, the unused sanitizer macros and
  XO_ATOMIC_V; the C tests' helpers (rc_*, xo_chunks_now, ev_poke, the
  listener and connection views) moved to testdata/xrec.h.
- Native runtime (decision 0233): a test binary's main starts the test
  world itself (the native prelude's llrt_tw_start, which
  selfhost/ssa testmode.xo calls first), and twrun.xo start puts
  cabi_tw.xo's tw_kick in the test world's record (twflags.xo TW_KICK)
  for pool.xo's clock move, so the C's weak xo_x_tw_start, tw_kick and
  tw_sleep, rt.sched_rt op 2, the services record's word 11, cabi_tw.xo's
  tw_start and tw_sleep, and the dead weak stubs of the network's free
  functions are gone. xo_rt.c's comments are cut to what is still C
  (the runtime they described is Xo), and its includes to what
  xo_nolibc.c needs (the C tests include their own); 1052 to 610 lines
  of std/llrt C.
- Checker: XO0608, a function name two C ABI export modules of the
  native std (std/native/llrt/cabi*.xo, not cabi_libc) both define,
  which would be one symbol xo_x_<name>; reported at check time with
  the other definition as a note, where the assembler or linker failed
  before (gotchas/duplicate-cabi-export-name.md).
- The runtime's start glue leaves the C (decision 0233 step 2): cabi.xo rt_start starts fault.xo and ctx.xo (ThreadSanitizer is found by sanhook.xo, the second personality routine is kept in ctx.xo's SV_PERS_OLD), and cabi.xo fault_raise_msg raises, so xo_rt.c's ctx_init and fault_raise_msg are gone and fault_init is three lines.
- Native runtime (decision 0233): optional features register their own
  end functions when they start (std/native/llrt/atexit.xo): tzhost.xo
  keep registers cabi_time.xo's tz_free, hserve.xo, hclient.xo and
  tlsio.xo register cabi_http.xo's streams_free, twrun.xo start
  registers cabi_tw.xo's tw_end, and entry.xo calls what is registered
  at exit. The C's weak stand-ins of streams_free and tw_end, the
  services record's tz_free word, C main's address passing, and with
  them xo_http.c and xo_std.c (only includes and comments left) are
  deleted; -DXO_TIME selects nothing now, so the runtime pack is format
  5 with one variant (rt.o) and the driver no longer passes the flag.
  The test world's start (services word 11) stays a weak C stand-in
  until the test binary's entry starts it.
- Native runtime (zero-C inventory rank 13): xo_rt.c keeps no copy of
  the task runtime's record layouts: the context, park, node, lock, wait
  list and queue entries, task, budget, channel and select structs and
  their layout asserts are gone (the records are pool.xo's, ctx.xo's,
  tasks.xo's, chan.xo's, selects.xo's and budget.xo's), and raising a
  fault is cabi.xo's fault_raise_cur (the C passes the personality
  routines' addresses); ctx(), cur_get and the C lock helpers are gone.
  The C tests see the records through testdata/xrec.h.
- Native runtime (decision 0233 step 2): Windows' console handler
  (Ctrl-C, Ctrl-Break, closing the console, shutdown) is poll.xo
  on_console (cabi.xo on_console) instead of xo_rt.c's, which goes with
  the C's pointer to the event loop's record. on_overflow stays C by
  design (it runs on the overflowed stack's small reserve). UNTESTED on
  windows; cross linked for windows/amd64 and arm64.
- The heap's last C leaves (zero-C rank 8, decision 0233 step 2): heapos.xo makes the main heap record and the central lists, cabi.xo heap_start and heap_threads replace obj_heap_init and xo_rt_threads_begin, so main_heap, the xo_heap type and its asserts, heap(), heap_slow, hw_slow_c, xo_oom and rt.sched_rt op 20 are gone.
- Native runtime (zero-C inventory rank 10): the runtime's records
  (rt.heap_large, its resize, rt.heap_release) come from libcmem.xo's
  allocator on every build, through a cache per thread, not from the C
  library's malloc; libcmem.xo maps through heapos.xo and locks through
  pool.xo, so it runs on Linux, macOS and Windows. The emitters call
  cabi.xo's exports; xo_rt.c's xo_heap_large and the rest forward for
  seed 0.12.0's code. bench/perf/conc on the Mac, 21 interleaved runs
  (median ms, main against this): spawn 74.1/68.3 LLVM, 70.5/76.9 arm64;
  pmap 92.7/93.1, 73.6/73.2; fanout 29.2/29.5, 30.4/32.9; pingpong
  41.5/46.1, 43.7/46.2 (minimums 33.1/36.4, 40.0/39.3): noise but for
  pingpong's medians, to measure again on a quiet machine. Without the
  per-thread cache pmap took 1.54 times as long (one lock).
- Native runtime (decision 0233 step 2, entry; zero-C inventory ranks 9
  and 15): main's program is entry.xo main (the start, main's threads,
  the end: flush, the stop record's end, statistics, the leak check),
  and C main keeps only the heap's and the faults' setup, Windows'
  binary streams, and one call of cabi.xo's entry_main. run_main is
  entry.xo run_main (cabi.xo's export is main's thread function), and a
  task's entry call is tasks.xo run_entry: both set the unwinding's
  boundary with rt.frame_addr, so xo_rt.c's run_main, run_entry and
  rt.sched_rt op 29 are gone. The weak stand-ins of cabi_http.xo's
  streams_free and cabi_tw.xo's tw_end stay in C (only some programs
  carry the exports): C main passes their addresses to entry_main.
- Native runtime (decision 0233 step 2, poll.xo's services): the park
  wait (cabi.xo poll_park_wait through rt.fn_addr), a context's park
  offset (ctx.xo C_PARK) and the poller thread's start
  (rt.fn_addr("poll_thread")) are Xo; xo_rt.c's pdsvc_wait, park_wait_n
  and pd_svc go (poll.xo's services record is gone: static word 44 is
  unused), with cabi.xo's park_wait_n export.
- Native runtime (zero-C inventory rank 11): the event loop's record is
  made and owned by poll.xo (static word 45); xo_rt.c's ev, xo_pd and
  xo_timer structs, the AFD types and their layout asserts are gone
  (the C keeps a pointer for its signal handlers). The C tests evloop.c
  and rawpoll.c treat descriptors as opaque (testdata/xcalls.h).
- Windows faults (zero-C inventory rank 14, the Windows part): the C
  fault_raise no longer captures a CONTEXT; unwind.xo win_raise captures
  its own (RtlCaptureContext through sys_windows.xo) and walks from
  there, three frames more per raise. UNTESTED on windows.
- Native runtime (decision 0233 step 2, stop and signals): stop.xo arms
  and disarms the grace timer, makes and closes task.stopping's channel
  and names the timer's function through rt.fn_addr of cabi.xo's
  exports (poll_timer_add, poll_timer_del, chan_close, the new
  stop_chan_new, stop_cancel) instead of the C's services record; SIGINT
  and SIGTERM's handler is poll.xo on_signal (cabi.xo on_signal: errno
  kept on macOS, no allocation); task.await runs a claimed task through
  tasks.xo run_claimed directly. xo_rt.c's pdsvc_tadd, pdsvc_tdel,
  stopsvc_chan, stopsvc_close, stop_grace_fire and on_signal go;
  stop_svc keeps only the entry's words (88 on).
- The worker and fiber records, the fiber cache record and the runtime statistics leave the C (decision 0233 step 2, zero-C rank 7): rtstats.xo keeps the statistics (static word 15), ctx.xo makes the fiber cache record, pool.xo sets the statistics hooks itself and reaches oom, deadlock, run_claimed and fiber_map_failed through rt.fn_addr, and rt.sched_run_task calls cabi.xo's fiber_run_task.
- ctx.xo's services record is Xo (decision 0233 step 2): ctx.xo init makes it, and every handle constructor names its free function with rt.fn_addr, so the C record, its initializer, asserts, free function declarations, pool_rec and the weak HTTP and network free stand-ins are gone.
- Native runtime (after seed 0.13.0-d770aa15): the C forwarders only
  older compilers' code called are deleted, std/llrt 2325 to 1967
  lines: the fault raisers and fault_raise, continue, take, msg, loc,
  detail (and xo_http.c's xo_fault_at), string equality and order, the
  number text builders, the entries batches N1 to N3 (scope deadlines,
  stop requests, task.stopping, real-time sections, the network, the
  clock and cancellation checks) with their helpers and the network's
  weak declarations, and the xo_personality forwarder off Windows (the
  unwinder gets xo_x_personality twice there). cabi_net.xo's
  nc_listener_addr and nc_conn_read, which only those forwarders
  called, go too. The C tests call the exports through
  std/llrt/testdata/xcalls.h.
- Native runtime (the event loop): poll.xo checks a context's
  cancellation (budget.xo canceled), signals a park (pool.xo
  park_signal, released for ThreadSanitizer first) and runs a signal's
  shutdown step (stop.xo signal) itself instead of through the C's
  services record; xo_rt.c's pdsvc_canceled, poll_signal_cb,
  poll_stop_cb, stop_signal and park_signal, and cabi.xo's park_signal
  export, go.
- Native runtime (two phase shutdown): stop.xo cancels main's root
  (pool.xo node_cancel), reads the timers' clock and makes the third
  signal's exit (stdio.xo flush, process.xo os_exit(130)) itself instead
  of through the C's services record; xo_rt.c's stopsvc_cancel,
  stopsvc_exit, stopsvc_now, tnow and node_cancel, and cabi.xo's
  node_cancel export, go. Services words 2, 7 and 8 are unused.
- Faults on x86_64 Linux and macOS unwind in Xo (decision 0221 stage
  2): unwind.xo reads x86_64's DWARF registers and macOS's x86_64
  compact unwind, the LLVM emitter writes x86_64 save and resume
  trampolines and names xo_x_personality everywhere, and xo_rt.c's
  libgcc forced unwind and C personality are deleted. The LLVM backend
  cross builds darwin/amd64 on Apple silicon and linux/amd64 from macOS
  with XO_LINUXSYSROOT. UNTESTED on linux/amd64 and darwin/amd64 (the
  fault goldens compile and link for both).
- Native backends (decision 0233, step 1): rt.fn_addr("name"), the
  address of the C ABI export xo_x_<name> as a word, and rt.frame_addr,
  the calling function's frame address (its frame kept, never inlined),
  for std/native only, on arm64 and LLVM (a fault when run on wasm).
  Nothing xo's own build reaches uses them yet, so seed 0.12.0 still
  builds xo; the services records, callbacks, signal handlers and entry
  move onto them after seed 0.13.0. Golden rt_addrs and codegen test
  fnaddr.
- The C allocator entries (xo_alloc, xo_free, xo_resize, alloc_in, free_in, alloc_mt, free_mt, alloc_usable, hw, solo_heap) are gone (zero-C inventory rank 8, first part): the C's strings and lists come from obj.xo's exports, and the C tests allocate through them (xcalls.h t_alloc and friends).
- The C handle code (h_init, h_retain, h_release, h_free, rt_defer, rc_dec_shared) is gone (zero-C inventory rank 5): stop.xo retains and releases its stopping channel itself, and the C tests free through obj.xo's exports.
- Native runtime: sanitizer builds map the heap through the C library's
  mmap (heapos.xo over sanhook.xo heap_map, decided at run time with
  rt.sym_present, as the fiber stacks are), so ThreadSanitizer and
  AddressSanitizer see the heap. xo_rt.c's heap_libc switch never took
  effect (gotchas/c-macro-tested-before-defined.md) and is deleted with
  heapos.xo's W_LIBC arm and its calls of rt.sched_rt ops 11 and 12,
  which the C no longer had. TestSysCTSan checks a ThreadSanitizer
  build's heap mapping.
- Native runtime (zero-C B32 part 5): main's 1 GiB thread is started
  and joined from Xo (pool.xo thread_run: pthreads on Linux and macOS,
  CreateThread on Windows); run_main stays C as the fault boundary.
  std/llrt/xo_win.c is deleted: its Windows headers and Winsock asserts
  are in xo_rt.c's _WIN32 block. UNTESTED on windows (the 17 http, net,
  fs, proc and stdin goldens cross link for both architectures).
- Native runtime (xo_rt.c inventory rank 1 and its dead code): the C
  record types of a scope, a Mutex and an Atomic, which no C read, are
  opaque declarations, and their layout asserts are gone (tasks.xo,
  mutex.xo and atomic_new's constants are the layouts); deadline(),
  pd_wait, and the declarations and weak stubs of xo_x_net_peername and
  xo_x_net_ntop, which nothing called, are deleted with cabi_net.xo's
  net_peername, net_ntop, net_read and net_send. xo_rt.c 2902 to 2816
  lines. The C test tasks.c reads those records at Xo's offsets.
- Native runtime: xo_bytes.c is deleted. Its xo_fault_bytes_index had
  no caller: the arm64 emitter's Bytes index check goes through
  isel.xo fault_entry to cabi.xo's fault_raise_bytes_index export, and
  LLVM calls that export directly, in this compiler and in seed 0.12.0
  alike.
- The test world's record (on, the virtual clock, in_main, the effects made real) moved from the C runtime into twflags.xo (zero-C inventory rank 4): rt.sched_rt op 30 and the record's C readers are gone.
- Windows event loop (zero-C B22's Windows pass, B32 part 4): the
  poller's start (completion port and AFD device, or the WSAPoll
  fallback's socket pair), a socket's registration, close and shutdown,
  the poke, and the WSAPoll loop are Xo (poll.xo; the socket pair is
  sock.xo loopback_pair). xo_rt.c loses about 310 lines and xo_win.c's
  socket shims go (176 lines left). UNTESTED on windows (the 16 http,
  net, fs, proc and stdin goldens cross link for both architectures).
- Native runtime: a started command's record is made in Xo (proc.xo
  spawn through tasks.new_handle, freed by cabi_proc.xo proc_child_drop
  through the services record's SV_PROC_CHILD_FREE); rt.proc_child_new
  is gone, so xo_proc.c's xo_proc_child_new and child_free_x have no
  caller left.
- Native runtime (zero-C small files): xo_proc.c is deleted (its
  xo_proc_child_new lost its last caller with proc.xo's own child
  records), and the Str results of `use c` adapters come from cabi.xo's
  new cffi_str export instead of xo_std.c's xo_cffi_str, so xo_std.c
  loses its helpers too (crypto_ref.c, the test, keeps its own byte
  buffer) and cabi.xo's utf8_encode export goes with them. With
  xo_proc.c went -DXO_PROC, the last flag link.xo io_flags (and xotool
  IOFlags) detected, so both are deleted, and the runtime pack is format
  4 with 2 variants (with and without -DXO_TIME).
- Native backends (generated-code entries): faults (raise, continue,
  take, msg, loc, detail, at) through fault.xo's exports, and a number's
  text (`a + "${n}"`, `"${n}"` for Int and Float) through obj.xo's and
  cabi.xo's, on both backends; the LLVM module head drops the
  declarations of the C fault raisers and number builders. The C entries
  stay for seed 0.12.0's code.
- Build: the prebuilt runtime pack is format 3, with 4 variants (the
  subsets of -DXO_TIME and -DXO_PROC) instead of 16; -DXO_FS and
  -DXO_STDIN, which select nothing since xo_fs.c and xo_stdin.c went,
  are gone from selfhost/xo rt.xo, rtpack.xo and io_flags (and xotool
  IOFlags). The emitters' comments no longer say deleted C entries stay.
  xo_rt.c drops 13 declarations of Xo exports nothing in C calls.
- Native backends (generated-code entries): string equality and order
  are obj.xo's str_eq and str_cmp (rt.mem_cmp) called through their
  exports on both backends; xo_str_eq and xo_str_cmp stay for code of
  the 0.12.0 seed.
- Windows runtime (zero-C B32 part 3): with Mbed TLS gone, xo_win.c's
  shims only its threading and xo_tls.c used (pthread mutexes, condition
  variables, pthread_once, localtime_r) are deleted; xo_win.c is at 331
  lines. UNTESTED on windows (the 15 http, net, fs, proc and stdin
  goldens cross link for both architectures on the Mac).
- Native backends (generated-code entries N1): task.with_timeout's
  deadline and check, proc stop requests and task.stopping, real-time
  section enter and exit, and a connection's write and close and a
  listener's close call their Xo exports (ssa/exports.xo, shared by
  arm64 and LLVM) instead of xo_rt.c's forwarders, which stay for code
  of earlier compilers and can go after the next seed.
- Native backends (generated-code entries N2): Net.listen and dial,
  Listener.accept and addr, Conn.read, and the NetErr's status and
  message call cabi_net.xo's exports (addr, read and the message build
  their List[Str] in netconn.xo; the emitters turn a 0 handle or list
  into the zero handle or empty list) instead of xo_rt.c's forwarders.
  The dead xo_net_dial_timeout and its export (xo_http.c's client, gone
  with the C TLS) are deleted.
- Native backends (generated-code entries N3): rt.clock_now (the test
  world's virtual clock), clock.sleep and sleep_or_cancel, a select's
  `after` arm and task.is_canceled call cabi.xo's exports (over poll.xo
  wall_now, selects.xo after, tasks.xo is_canceled and twrun.xo's
  sleep record) instead of xo_rt.c's forwarders. With N1 and N2, 184
  lines of xo_rt.c can go after the next seed.
- Native runtime (zero-C audit item B06): the last fault entries of
  generated code are Xo exports (cabi.xo fault_raise, fault_continue,
  fault_take, fault_msg, fault_loc, fault_detail over fault.xo); the C
  xo_fault_raise, xo_fault_continue, xo_fault_take, xo_fault_msg,
  xo_fault_loc and xo_fault_detail forward to them for code from seed
  0.12.0 and older. The unwind itself (fault_raise, the Windows context
  capture and the libgcc path of x86_64 with a C library) stays in C.
- Native runtime (zero-C audit end pass, after seed 0.12.0-26641183):
  the C entries no generated code of 0.12.0 or later calls are
  deleted, 4486 to 3832 lines of std/llrt C. Gone: xo_rt.c's
  forwarders of channels, select, Mutex, Atomic.new, tasks, scopes,
  shields, the program's group, budgets, xo_tick, xo_unwinding,
  xo_fault_rethrow, xo_landing, xo_net_dial_timeout, the
  heap's platform calls (heap_big, heap_grow, heap_lock), the counted
  objects' entries (rc_alloc, rc_free, rc_defer, rc_inc, rc_dec,
  rc_claim, rc_mark, rs_concat, rl_new, rl_new_n, rl_share, rl_free,
  h_free, h_zero, rl_empty, rt_live) and decision 0232's seed shims
  (xo_sched_hook, xo_sym_present, the weak sanitizer, pthread, mmap
  and errno names); xo_bytes.c's builders, slices, const_once, the
  Float32 conversions and the -DXO_INTS_C intrinsics; xo_std.c's
  str_slice, str_repeat, rs_append; xo_http.c's xo_any_new and
  xo_any_get; all of xo_fs.c and xo_sys.c, with
  -DXO_FS and -DXO_INTS_C in the drivers (link.xo io_flags, xotool
  IOFlags). The C tests that play generated code call the Xo exports
  through std/llrt/testdata/xcalls.h. xo_set_grace stays: the SSA
  builders still lower Proc.set_grace to it, which no golden reached
  until the new proc_set_grace golden (every backend).
- LLVM backend: the module head no longer declares the C entries
  generated code stopped calling in the entries batches E1 to E3
  (xo_rc_alloc, rc_free, rc_defer, rs_concat, rl_new, rl_new_n,
  rl_empty, rl_share, rl_free, h_free), so their C forwarders can go
  once a seed built from this exists.
- Native backends (generated-code entries E3): the empty list and the
  unwinding placeholders are the addresses of xo_rt.c's xo_empty_list
  and xo_zero_handle in generated code, not calls (xo_rl_empty and
  xo_h_zero stay for code of earlier compilers); alloc small 0.98 on
  arm64, nothing slower.
- Native runtime (zero-C audit item B31, part 20, decision 0202): the
  Xo HTTPS server takes Go 1.26's groups (SecP256r1MLKEM768,
  SecP384r1MLKEM1024 and P-521 next to X25519MLKEM768, X25519, P-256
  and P-384), chosen as Go's server chooses: post-quantum first, then
  a group with a key share. ECDSA P-521 certificates work on TLS 1.3
  (0x0603) and 1.2, and P-224 on TLS 1.2. The interop matrix checks
  them against Go's clients.

- Windows standard input (zero-C B32 part 2): the pump that joins a pipe
  or the console to a loopback socket is Xo (stdin.xo); xo_stdin.c is
  deleted and xo_win.c is at 395 lines. UNTESTED on windows (the 15
  http, net, fs, proc, and stdin goldens cross link for both
  architectures).
- Windows runtime (zero-C B32 part 1): xo_win.c 914 to 484 lines, the
  shims nothing reached deleted; the UTF-8 environment built in Xo
  (process.xo win_env). Windows cross builds now link on the Mac
  (toolchain.md). UNTESTED on windows.
- Native backends (generated-code entries E2): both backends allocate
  and free strings, boxes, and closure environments, and free handles,
  through obj.xo's exports (xo_x_obj_rc_alloc, rc_free, h_free) instead
  of the C entries, which stay for code of earlier compilers. arm64 at or
  under the step before; LLVM string heavy text 1.02
  (bench/results/generated-entries-2026-10-15.md).
- Mbed TLS and fiat-crypto removed (owner approval 2026-10-15; decision
  0202 supersedes 0105 and 0134): third_party/ (Mbed TLS 4.1.1 and
  fiat-crypto, 9 MB), their vendor scripts, std/llrt/xo_tls.c and
  xo_http.c's TLS section, std/native/llrt tlsio_c.xo, tlssrv_c.xo,
  cabi_tls.xo and tlscert.xo's Mbed parts, rthttp.xo's TLS declarations,
  the build glue (selfhost/xo tls.xo: needs_tls, the library's build and
  cache; the link's tls flags; rtpack's libxotls.a and TLS variants, pack
  format 2 with 16 variants; internal/xotool tls.go), the
  `<user cache>/xo/mbedtls` cache, $XO_TLS_CACHE, `xo rtpack --ar`, and
  TestTLSLeaks (Mbed's own allocations). `XO_LLRT_C=tlsio` and `tlssrv`
  now fail as unknown C paths; XO_NOLIBC builds take TLS programs. Needs
  a seed of 0.12.0 or later: the 0.11.0 seed cannot build xo from these
  sources (gotchas/seed-0.11-links-frameworks-only-for-mbed.md).
- Fix: `xo test --no-gc` hung on tests that sleep under the virtual
  clock since B22 remnants batch 2 (0e1771fd): poll.xo read the virtual
  clock at a word index where its constant was a byte offset
  (gotchas/slot-atomics-index-words.md).
- arm64 builds say so when the inlined module does not verify (they go
  on without inlining, slower) instead of falling back silently, and
  fail with XO_INLINE_STRICT=1, which the Go tests set.
- Native runtime performance: a task's run reuses its thread's spare
  context (thread local word 13) instead of a heap block per run
  (tasks.xo run_claimed): ctx spawn 0.98 and 0.96, conc spawn 0.97 and
  0.99 of the step before (arm64, llvm). A new repository test checks
  that no two runtime modules use one thread local word
  (TestThreadLocalWordOwners; the first try collided with sysio.xo's
  word 12).
- AES-GCM: GHASH with the CPU's multiply as a loop over H's words (no
  Gcm passed per block). AES-128-GCM on LLVM 14.1M instructions a MiB
  (Mbed 14.0M; TLS 1.3 13.4M against 13.9M), on arm64 17.7M.
- AES-GCM: the keystream xor written in place into the output, and the
  body read in place for its tag (no copy). AES-128-GCM on LLVM 17.9M to
  15.6M instructions a MiB (Mbed 14.0M), on arm64 26.3M to 21.0M.
- ChaCha20-Poly1305: wrapping arithmetic in the quarter rounds and
  Poly1305's products (no overflow branches), and on arm64 a 32 bit add
  for an add masked to 32 bits. Bulk on LLVM 2652M to 2086M
  instructions for 64 MiB, on arm64 3702M to 2518M.
- Wasm backend: rt.heap_large, rt.heap_release and rt.heap_large_resize
  in native-only arms (sysio.set_err behind layer() == 0) compile to a
  fault when run, as sys6 and fs_set_err do. TestSelfhostWasmFsWrite
  passes again; it had failed since a4ed9cd0
  (gotchas/wasm-rejects-calls-in-dead-native-arms.md).

- AES-GCM: whole block AES intrinsics (cpu.aes_enc10/12/14, the state
  in a register) and one pmull for both halves of a GHASH product on
  arm64. Bulk AES-128-GCM on LLVM 27.3M to 17.9M instructions a MiB
  (Mbed 14.0M), on arm64 37.8M to 26.3M. UNTESTED on linux, x86-64,
  and windows.
- Compiler (selfhost/ssa): `==` and `!=` on two enum variants known at
  build time, such as `target.os == TargetOs.Darwin`, fold to a
  constant on every backend. The arm64 backend used to call the enum's
  equality at run time. test/codegen/enums covers it. sysio.clock_now
  uses `==` again (sysio clock on arm64: 0.93 of main).

- Native runtime performance (the zero-C perf sweep's scheduler paths):
  a context's wait makes no C call outside ThreadSanitizer builds
  (tasks.xo park_wait_n reads the test world's flags from static words),
  ctx.cur inlines again (root behind an out of line function), and the
  output flush before a wait inlines its empty check. ctx budget and
  defer are back to main's instruction counts, sysio chan from 1.17 to
  1.09 of main on arm64 (bench/results/zero-c-perf-sweep-2026-10-15.md).
- arm64 backend: an add or subtract and its carry or borrow
  (std/native/llrt/cpu.xo) become one adds or subs and a cset; the SSA
  builder folds + and - of two Int constants that do not overflow;
  nistec.xo's mul4 and mul6 lose their terms of 0. arm64 TLS handshakes
  12 to 22% fewer instructions (P-256 group 4930M to 3989M for 200).
- Mbed TLS path (XO_LLRT_C=tlssrv, and main's C): http.tls_cert accepts
  an RSA certificate with its PKCS#8 key. TF-PSA-Crypto 1.1.1's PKCS#8
  parse left the RSA public half unset, so mbedtls_pk_check_pair failed;
  xo_tls.c now sets it after the parse. X25519 and X448 PKCS#8 keys are
  an unknown key type, as Go's (gotchas/mbedtls-pkcs8-rsa-check-pair.md).

- Native runtime (decision 0232, the port): the sanitizer fiber hooks,
  ThreadSanitizer's edges, pthread_create on the Linux C library, and
  fiber stacks mapped through libc under ThreadSanitizer moved from
  xo_rt.c to std/native/llrt/sanhook.xo. A sanitizer build is detected
  at run time (rt.sym_present). The C keeps xo_sched_hook as a
  forwarder and xo_<name> shims for the 0.11.0 seed. UNTESTED on linux
  and windows.
- Native runtime performance (the zero-c perf sweep,
  bench/results/zero-c-perf-sweep-2026-10-15.md):
  - std/native/llrt/mutex.xo: held_add has a fast path that calls
    nothing when no Mutex is held yet and there is room for one. The
    XO-F004 message moved into held_fault, and the growth into
    held_grow. held_del searches from the last entry and skips the move
    when only one entry is held. ctx mutex on arm64 went from 1.20x
    main to 1.04x; on LLVM it is 0.86x main.
  - std/native/llrt/sysio.xo: clock_now takes a macOS fast path, written
    with `match` because the arm64 backend does not fold `==` on the
    target enums (gotchas/arm64-target-enum-eq-not-folded.md). sysio
    clock went from 1.08x main to 0.99x on arm64 and 0.96x on LLVM.

- Native backends (zero-C audit, generated-code entries E1): LLVM code
  calls obj.xo's list and string entries through their exports instead of
  the C forwarders (xo_rl_free, rl_share, rc_defer, rl_new_n,
  rs_concat), and both backends call heapos.xo's platform calls and
  anyval.xo's Any directly; the C forwarders stay for code of earlier
  compilers. No bench/perf alloc, text, or conc workload past 5%; LLVM
  alloc strings 0.950 (bench/results/generated-entries-2026-10-15.md).
- std/http (native): a fault in the server's own connection code ends
  that connection only, as net/http recovers per connection: each
  connection's task is isolated (std/native/http.xo
  serve_conn_isolated), logged `ERROR http: panic serving fault=<msg>`,
  the connection closed, the server serving on, nothing live at exit.
  Before, such a fault stopped the server for every client (shown with
  the new test probe). A test hook, XO_HTTP_FAULT_PROBE=1, faults on
  /xo-fault-probe/conn (http1.xo's reader), /frame (h2srv.xo's frame
  handling) and /stream (an HTTP/2 stream's request; the stream is
  reset), on the native and Go backends (std/rt faultProbe: logged the
  same, then http.ErrAbortHandler). Golden http_conn_fault (HTTP/1.1 and
  HTTP/2, every backend, the leak check). main is affected too: with
  main's own bin/xo and std (4be43460), a fault in an `r.wrap`
  middleware leaves the native server answering nobody (checked
  2026-10-15); main has neither fix.
- Tests (zero-C audit B28 follow up): the HTTP parser coverage the C
  drivers had is back, in Xo. TestHTTPParseDiff and FuzzHTTPDiff send
  their corpus and random requests (3,081 inputs) to http1DiffProgram
  built with LLVM and arm64 and compare with net/http in process;
  TestHTTPFuzz mutation fuzzes the HTTP/1.1 server, HTTP/2 frames and
  HPACK over TLS, the client's response reading, and std/json decoding,
  each program run with XO_LEAKCHECK=1 (XO_FUZZ_SECONDS, FUZZ_SEED,
  XO_FUZZ_REPLAY). std/llrt/testdata/http_diff.c, http_fuzz.c and
  http_ref.c deleted.
- std/http: a fault in a middleware (`r.wrap`) is isolated as a
  handler's (500, logged, the server keeps serving) on every backend. It
  was not: the Go backend dropped the connection (net/http's recover),
  and the native server stopped answering anyone
  (gotchas/native-server-unisolated-fault-wedges.md). Found by
  TestHTTPFuzz; golden http_fault has the case.
- Native runtime (zero-C audit item B31, part 19, decision 0202): the
  Xo TLS client offers Go 1.26's groups in its order: X25519MLKEM768,
  SecP256r1MLKEM768, SecP384r1MLKEM1024, X25519, P-256, P-384, P-521
  (key shares for the first and its ECDH part, as Go's).
  - nistec.xo: P-521 (ECDH, ECDSA sign and verify, constant time, nine
    limbs; Go's masking of the top byte of random scalars, hashToNat's
    shift) and P-224 (ECDSA sign and verify, for certificates). The
    workspace's elements widened to 72 bytes; R mod m and m - 2 made
    general. P-521's base point multiplication is the windowed pt_mul,
    as P-384's: a compiled in fixed base table cost every TLS build 27 s
    and 2.5 MB (gotchas/large-const-list-costs-every-build.md).
  - x509.xo: KEY_P521 and KEY_P224 (KEY_EC_OTHER gone); P-521 and P-224
    ECDSA signatures checked (the "P-521 signatures refused" gap closed).
  - The clients: P-521 ECDHE on TLS 1.3 and 1.2, ECDSA P-521 SHA-512
    (0x0603) on TLS 1.3, P-224 and P-521 leaves accepted as Go's.
  - mlkem.xo: ML-KEM-1024 (k 4, du 11, dv 5; ML-KEM-768 unchanged byte
    for byte) and the hybrids by group (SecP256r1MLKEM768 and
    SecP384r1MLKEM1024 put the ECDH part first).
  - Checks: a scratch diff of 40 random cases per curve against Go
    (public keys, ECDH, signatures both ways, hashes of 20 to 70 bytes);
    golden tls_nistec (P-521, P-224), crypto_mlkem (ML-KEM-1024's
    accumulated 100, crypto/mlkem's digest: Go's tests carry no CCTV value
    for 1024; the SecP hybrids) equal to the Go backend; tls13_go replays
    Go 1.26 server traces of P-521 with a P-521 certificate,
    SecP256r1MLKEM768 and SecP384r1MLKEM1024, whole and a byte at a time,
    and TestTLS13ReplayDiff fuzzes them; TestTLSInteropMatrix adds Go
    servers on P-521 (certificate and group) and each hybrid for the Xo
    client. The Xo server's P-521 and hybrids are next (xo-2b).
- Native runtime (zero-C audit item B31, part 18, decision 0202): a
  default https program links no Mbed TLS and no fiat-crypto (nm: 0
  mbedtls, fiat or psa symbols on arm64 and LLVM, http_tls). tlscert.xo
  apple_trust calls the Security and CoreFoundation frameworks directly
  (sys_darwin.xo declarations) instead of xo_tls.c's table, which needed
  Mbed's tls_init; the build links the two frameworks when the generated
  code calls SecTrustEvaluateWithError (selfhost/xo link.xo
  framework_args, ld.xo, internal/xotool FrameworkFlags). The system's
  roots are an Xo record (tlsio.roots_system, static word 31) instead of
  the C's sys_roots. needs_tls (selfhost/xo tls.xo, internal/xotool
  tls.go) compiles -DXO_TLS and links libxotls.a only for the C paths'
  symbols (xo_tls_, xo_cl_tls_: XO_LLRT_C=tlsio or tlssrv). XO_TLS_MLKEM=0
  (spec/std.md section 4, mirroring Go's GODEBUG=tlsmlkem=0, on the
  native and Go backends) makes the client offer no X25519MLKEM768, X25519
  the only key share (a 291 byte ClientHello instead of 1,513), for
  like with like handshake measurements against Mbed; golden
  tls_mlkem_off (tls13.xo client_groups through the tls13.groups op; the
  Go backend's op states the same groups). spec/std.md also documents
  XO_HTTP_MAX_BODY, which it lacked.
- Native runtime (zero-C audit item B31, part 17, decision 0202): the
  Xo TLS 1.3 client offers X25519MLKEM768 (0x11ec) first, as Go 1.26's
  client: groups [X25519MLKEM768, X25519, P-256, P-384], the hybrid key
  share and an X25519 share from the hybrid's own X25519 key, either
  taken from the ServerHello, a HelloRetryRequest for a group already
  shared refused as Go's; the hybrid itself is xo-2b's mlkem.xo
  (ML-KEM-768 and sha3.xo). Go 1.26's server negotiates X25519MLKEM768
  with it (by hand: curve=X25519MLKEM768), as do google.com,
  cloudflare.com, github.com and pq.cloudflareresearch.com. The Go
  reference port does the hybrid with crypto/mlkem; tls13_go replays a
  Go server's X25519MLKEM768 trace (whole and a byte at a time), and
  TestTLS13ReplayDiff fuzzes it too (0 differences).

- Native runtime (decision 0232, items 1 and 2): std/native/sys_c.xo
  calls C library and sanitizer functions on every native OS, with weak
  references and rt.sym_present for symbols a build may not link (arm64:
  .weak_reference/.weak and GOT loads, which selfhost/macho now
  assembles and links; LLVM: extern_weak). On macOS the driver passes
  -Wl,-U for each weak reference. New golden sys_c and test
  TestSysCTSan. UNTESTED on linux and windows.
- Fix (B31 parts 15 and 16): the Xo TLS client left one allocation live
  at exit (http_tls and http_tls_keys on LLVM; xo-2b bisected it to
  165eb6a3): tzrules.load_host kept the host's zone in tzhost's table,
  which only std/time programs free at exit. The offset now comes from
  tzrules.host_offset, which parses without keeping (a zone std/time
  kept is used as it is), and the system roots part 16 parses once
  (static word 31) are freed at exit too (tlsio.roots_free from
  cabi_http.xo's streams_free). TestGolden and TestGoldenRun pass for
  http_tls and http_tls_keys on both TLS paths.

- Native runtime (zero-C audit item B31, part 16, decision 0202): the
  Xo client's system roots off macOS are Xo's: tlscert.xo collects the
  DERs (Linux: $SSL_CERT_FILE or the distribution's bundle, then
  $SSL_CERT_DIR or the default directories, as crypto/x509's
  root_unix.go; Windows: the ROOT store), and tlsio.xo parses them once
  per process with x509.xo into static word 31 (published with a
  compare and swap). The Xo path no longer reads Mbed TLS's chain
  (xo_tls_c_roots_crt removed); macOS keeps the Security framework. The
  Mbed path still loads its roots through tlscert.xo's Mbed loaders.
  UNTESTED on linux and windows (cross built for linux/arm64; Windows
  needs XO_WINSYSROOT).

- Native runtime (zero-C audit item B31, part 15; decision 0193 step
  5's zones): tz.xo splits so that the host's zones link without
  std/time's zone table: tzrules.xo (the rules: TZif parsing, lookup,
  load_host from the host's database), tztable.xo (the program's zone
  data through rttime.tz_table, moved out of tzhost.xo), and tz.xo
  (load over both, as before; std/time names tzrules.Rules). The Xo TLS
  client's expiry text takes Go's time.Local offset from tzrules.xo
  (TZ, the machine's zone, /etc/localtime), so part 14's C accessor
  (xo_tls_c_local_off) is gone again; the TLS 1.2 hand-off passes it too
  (xo-fa's tls12 zone_off). The time goldens (clock_timers, realtime_*,
  sys_files_clock, tz_table) pass.

- TLS 1.2 server (tls12srv.xo): every handshake message is decoded as
  Go's unmarshalHandshakeMessage does for its type before the state
  check (decode_error, not unexpected_message, for a message of the
  wrong type that does not decode), and the ClientHello's
  status_request, extended_master_secret, SCT, early_data, and
  pre_shared_key extensions are as strict as Go's. TestTLSServerReplayDiff
  (tls12, seeds 22 to 24, 16000 cases each): 0 byte differences (was
  11). UNTESTED on linux and windows.
- Native runtime (zero-C audit item B31, part 14, decision 0202): the
  Xo client's certificate expiry text shows the current time in the
  local zone as Go's (time.Local: "current time
  2026-10-15T04:43:17-04:00 is after 2001-01-01T00:00:00Z"; checked with
  TZ unset, UTC and Asia/Kolkata), through x509verify.full_text_zone and
  tls13.Conf.zone_off. The offset comes from xo_tls.c's localtime
  (xo_tls_c_local_off): tz.xo would bring std/time's embedded zone table
  into every https program, so the end pass needs tz.xo's host lookup
  apart from the table before this C goes (done in part 15).

- Native runtime (zero-C audit item B31, part 13, decision 0202): the
  Xo client's custom roots (Client.with_roots) are parsed once, in Xo,
  as Go's AppendCertsFromPEM (x509.xo, so exactly the certificates Go
  takes), into a record the HTTP/2 pool counts; Mbed TLS no longer parses
  them on the Xo path and a connection no longer re-parses them.
  std/native/http.xo asks tlsio.xo (or tlsio_c.xo, XO_LLRT_C=tlsio) for
  roots. The system's roots stay the C's record: macOS verifies through
  the Security framework; on Linux and Windows the Xo client still reads
  them from Mbed's chain (the next step).

- TLS 1.2 server session tickets (tls12srv.xo with tlsticket.xo, RFC
  5077): golden tls12_tickets, crypto/tls's client resuming against the
  Xo server byte for byte (a ticket issued, resumed, resumed a byte at a
  time, and the cases Go declines). UNTESTED on linux and windows.
- TLS performance (decision 0231 parts 2 and 3): AES-GCM uses the CPU's
  AES rounds (AESE/AESMC, AESENC) and carry-less multiply (PMULL,
  PCLMULQDQ) when it has them (std/native/llrt/cpu.xo hw(): macOS arm64,
  Linux AT_HWCAP, x86-64 CPUID), the bitsliced code otherwise. Bulk
  AES-128-GCM on LLVM 138.6M to 27.1M instructions a MiB (arm64 205.0M
  to 38.0M). New golden cpu_prims. UNTESTED on linux, x86-64, and
  windows.
- Native runtime (zero-C audit, scheduler block S2): the worker pool's
  record and its deadlock counts are made by std/native/llrt/ctx.xo at
  start instead of being C statics (xo_rt.c 3,665 to 3,638 lines).
  Decision 0232 (Proposed, parked for the owner) describes the compiler
  feature the sanitizer fiber hooks and the Linux C library services
  would need to leave the C.
- Native runtime (zero-C audit item B31, part 12, decision 0202): Go's
  certificate policy validation in x509verify.xo (policiesValid: RFC
  5280 6.1 as updated by RFC 9618, the policy graph with Go's default
  options), with x509.xo keeping the policies, policy mappings, policy
  constraints and inhibitAnyPolicy, and Verify's order (policies, then
  key usages, then name constraints) and its "x509: no valid chains
  built: N candidate chains with invalid policies" text. This closes part
  11's gap. TestX509VerifyDiff adds random policies, mappings, policy
  constraints and inhibitAnyPolicy: 20,000 chains over two seeds, no
  difference (about 5 percent refused for their policies).

- Native runtime (zero-C audit item B31, post-quantum, decision 0202):
  ML-KEM-768 (FIPS 203) in Xo (std/native/llrt/mlkem.xo, as Go's, with
  the implicit rejection in constant time) over SHA-3 and SHAKE
  (std/native/llrt/sha3.xo). The Xo TLS server negotiates
  X25519MLKEM768, Go's default group, first as Go's server does.
  internal.crypto_op gains the sha3 and mlkem areas (Go's crypto/sha3
  and crypto/mlkem as the reference). Golden crypto_mlkem matches
  FIPS 202's digests, C2SP CCTV's accumulated ML-KEM test and crypto/mlkem;
  TestMLKEMAccumulated runs CCTV's 10,000 iterations; tls13_server
  replays Go 1.26's default client negotiating the hybrid; the interop
  matrix has Go's clients taking it against the Xo servers.
- Native runtime (zero-C audit item B31, part 11, decision 0202): name
  constraints as Go 1.26 checks them (its constraints.go: per
  certificate, sorted and pruned sets of IP, DNS, URI and e-mail
  subtrees searched as Go's, wildcard names against excluded parents,
  full mailboxes, URIs by host), replacing part 9's refusal of every
  IP, e-mail and URI constraint; Go's texts ("x509: a root or
  intermediate certificate is not authorized to sign for this name: IP
  address \"10.0.0.1\" is excluded by constraint \"10.0.0.0/8\"", the
  code 5 text gained Go's "to sign" and detail). x509verify.xo's
  full_text gives a verification's whole Go text (tls13.xo uses it).
  TestX509VerifyDiff (internal/llvm, not -short) verifies random chains
  made by Go (constraints on the root and an intermediate, every kind of
  name on the leaf and the intermediate) with x509verify.xo and Go's
  Verify: 20,000 chains over two seeds, no difference. Known gap, next:
  Go's certificate policy validation (policiesValid) is not done, so a
  chain Go refuses for its policies verifies here (closed by part 12).

- Native runtime (zero-C audit, scheduler block S1): a context's wait
  on its park is Xo (std/native/llrt/tasks.xo park_wait_n: the output
  flush, the budget's pause, the root thread's helping record, the test
  world's counted wait), and a helper's dequeue signals the scope's
  joiner from pool.xo; rt.sched_rt ops 10 and 17, the dead C wait lists
  and helpers, and eight cabi.xo exports only the C called are deleted.
  xo_rt.c 3,809 to 3,665 lines.
- TLS performance (decision 0231 part 1): the carries of the P-256,
  P-384, and RSA field products are CPU flags (std/native/llrt/cpu.xo,
  cmn/cmp and cset on arm64, llvm.uadd.with.overflow on LLVM, a
  constant time Xo body elsewhere and for older xo builds). LLVM
  handshakes: X25519 with a P-256 chain 14.7M to 10.9M instructions,
  P-256 18.6M to 13.4M, P-384 33.6M to 23.7M, RSA-2048 12.3M to 9.4M.
  UNTESTED on linux and windows.
- Native runtime (zero-C audit item B22, remnants batch 2): on Linux and
  macOS the event loop starts in Xo (std/native/llrt/poll.xo ev_start:
  the self pipe, kqueue or epoll, the poller thread), and registers
  descriptors (pd_open), shuts and closes them, wakes the poller, and
  adds and removes timers there; clock.sleep's wait, select's `after`
  timer and task.with_timeout's deadline are Xo too (poll.xo clock_wait,
  timer_fn with FN_SELECT and FN_SCOPE; tasks.xo scope_deadline,
  scope_timed_out). The Linux ThreadSanitizer edge through epoll gets its
  acquire back. xo_rt.c 4,030 to 3,836 lines. Linux untested (written on
  macOS); Windows' start and WSAPoll loop stay C until the Windows pass.
- Native runtime (zero-C audit, xo_rt.c's main thread context): the root
  context of a thread running Xo code outside any task is ctx.xo root
  (the C's ctx forwards; rt_once, rt_init, cur_set, park_init deleted).
  The rest of "Fault unwinding" and the process environment is labelled
  GLUE for the end pass (generated code and seed entry points, Windows
  and libgcc x86_64 paths) in the audit row.
- Native runtime (zero-C audit item B31, part 10, decision 0202): the
  net/url parser of std/http (url_parse, unescape, ports, IPv4 and IPv6
  literals, go_quote) moves from http1.xo into std/native/llrt/neturl.xo,
  unchanged (the http_* and net_* goldens and TestHTTP1ClientDiff and
  TestHTTP1ParseDiff pass), so x509.xo parses a certificate's URI names
  as Go's url.Parse does, with Go 1.26's parseHost texts
  (neturl.go_host_error). TestX509ParseDiff gains twelve URI classes
  (61 in all, no difference). The TLS 1.2 hand-off passes the Security
  framework's table to tls12.xo (xo-fa's apple_svc), so on macOS with the
  system's roots both versions verify as Go.

- Native TLS performance, step 4: Go's point doubling formula and
  inversion by addition chain (P-256's field) or a fixed window (the
  other exponents): handshakes about 8% fewer instructions on LLVM
  (X25519 with a P-256 chain 15.9M to 14.7M, Mbed 5.3M).
- TLS server checks (zero-C audit item B31, decision 0202):
  TestTLSServerReplayDiff fuzzes tls13srv.xo and tls12srv.xo with mutated
  client records against the Go references on LLVM and arm64 (64,000
  cases over four seeds). TestTLSInteropMatrix crosses the Xo and Go
  clients and servers over TLS 1.2 and 1.3, suites, groups, certificate
  keys, ALPN, and resumption against crypto/tls on both ends. Fixes: the
  servers' alerts before version negotiation use record version 0x0301
  as Go's, and session ticket keys use Go's SHA-512 slices (tickets
  interoperate with crypto/tls's, checked by std/rt's TestTicketsAsGo).
- Native TLS 1.2 server: session tickets (RFC 5077, as Go's
  handshake_server.go: the extension, NewSessionTicket after a full
  handshake, the abbreviated handshake with Go's resumption checks;
  tls12srv.Conf ticket_keys, now, Server.resumed; tlsticket.xo seals),
  dormant until tlssrv.xo passes keys. Fuzz fixes: "unexpected EOF" only
  when the stream did not close; no handshake message read while waiting
  for ChangeCipherSpec (server and client).
- Native TLS 1.2 client: tls12.Conf.apple_svc, the chain verified by the
  Security framework on macOS with system roots (tlscert.apple_trust, as
  tls13.xo and Go's systemVerify; bad_certificate for every failure);
  x509verify.error_text for the other verification errors (Go's
  unknown-authority hint).
- Native TLS 1.2 (xo-2b's server differential fuzz, 137 differences to
  0): alerts before the version is agreed use record version 0x0301 as
  crypto/tls (server and client); a ClientHello's server name is
  acknowledged whatever its bytes; the tls12 ops read the peer's bytes as
  one stream and report "unexpected EOF" inside a record; the server op
  takes the three groups' ECDHE keys and uses a hooked signature only
  when it signs the exchange, and its mask covers the ServerKeyExchange
  record (ECDSA signatures vary in length).
- Native runtime (zero-C audit item B31, part 9, decision 0202): X.509
  parsing and chain errors as strict as Go's, with Go's texts.
  std/native/llrt/x509.xo's parser is now a port of Go 1.26's
  crypto/x509 parser.go, over the new cbyte.xo (golang.org/x/crypto's
  cryptobyte reading in Xo: minimal lengths and integers, OID arcs,
  booleans, bit strings, Go's time.Parse rules for UTCTime and
  GeneralizedTime). It covers RDN string types, keys (RSA's NULL
  parameters, P-224 to P-521 points on their curves through bigmod.xo,
  Ed25519, DSA), and every extension Go reads (SAN, name constraints
  with IP, e-mail and URI subtrees, CRL distribution points, policies,
  policy mappings and constraints, AIA, inhibit any policy). x509verify.xo
  picks candidates in Go's order (matching key identifiers first), uses
  Go's alreadyInChain, gives an unknown authority Go's hint ("possibly
  because of ... while trying to verify candidate authority certificate
  ..."), and reports the signature check limit. IP, e-mail and URI name
  constraints refuse a chain whose lower certificates carry such names
  (Go checks them; this is stricter until x509verify.xo does). On macOS
  with the system's roots the Xo client asks the Security framework
  (tlscert.xo apple_trust), as Go does. The first record not looking like
  TLS gets Go's text. TestX509ParseDiff (internal/llvm, not -short)
  diffs x509.xo against Go's ParseCertificate on mutated certificates and
  49 named classes: 150,000 cases, no difference. TestTLS13ReplayDiff now
  fails on any difference in a refused certificate's text: 0 in 20,000.
  Known gaps: a SAN URI is checked as IA5 only (Go also parses it with
  net/url, whose port is in http1.xo; closed by part 10); P-224 and P-521
  signatures are not verified (refused).

- Native TLS 1.2 client: offers the four ECDHE CBC-SHA suites after the
  AEADs, as Go's client does by default (tls12.default_suites, which
  tlsio.xo's combined ClientHello uses too); golden tls12_client adds a
  CBC trace.
- bench/results/tls-perf-2026-10-15.md: a proposal for three compiler
  intrinsics the remaining TLS gap needs (add and subtract with carry,
  AES rounds, carry-less multiply), with measurements.
- Native TLS performance, step 3: P-384's field arithmetic unrolled, and
  RSA's modulus setup by Go's rr chain (12 squarings in place of 4,096
  doublings): P-384 handshakes 56.3M to 36.9M instructions on LLVM (Mbed
  22.9M), RSA-2048 18.5M to 13.1M (Mbed 4.9M).
- Native TLS 1.2 server: the four ECDHE CBC-SHA suites Go's server
  enables by default (c009, c013, c00a, c014), after the AEADs: SHA-1
  and HMAC-SHA1 (std/native/llrt/sha1.xo), AES decryption and CBC
  (aesgcm.xo, BearSSL's bitsliced inverse cipher), and Go's constant
  time CBC record check (extractPadding, the MAC over a constant amount
  of data, one bad_record_mac for every failure). Goldens cbc_prims
  (FIPS 197, SP 800-38A, RFC 2202 against Go) and tls12_cbc (crypto/tls's
  client, records made with the session's keys: padding 255, bad
  padding, bad MAC, short, not whole blocks), byte identical on Go, LLVM
  and arm64. Server side only.
- Native TLS performance, step 2: P-256's field arithmetic unrolled over
  local limbs and a fixed base table for [k]G (nistec_g256.xo, made by
  scripts/nistecg256): an X25519 handshake with a P-256 chain 47.3M to
  15.9M instructions on LLVM (Mbed 5.3M), the P-256 group 62.2M to 20.2M
  (Mbed 12.9M) (bench/results/tls-perf-2026-10-15.md).
- Native runtime (zero-C audit item B31, session resumption, decision
  0202): the Xo TLS server issues session tickets and resumes TLS 1.3
  sessions as Go's crypto/tls does. std/native/llrt/tlsticket.xo holds
  Go's ticket encoding, AES-CTR plus HMAC-SHA256 sealing, and automatic
  key rotation. tls13srv.xo sends a NewSessionTicket after its Finished
  and accepts psk_dhe_ke with a constant time binder check (no 0-RTT).
  Golden tls13_server replays a recorded Go 1.26 client that resumes,
  plus a bad binder, an expired ticket, and a ticket of another key.
  TestTLSDiff's TLS 1.3 resumption probe now equals Go. TLS 1.2 tickets
  come with tls12srv.xo; Xo's client does not resume.
- Native TLS performance, step 1: ChaCha20 on 16 local words with 32
  bit rotates, Poly1305 absorbing the AEAD's pieces in place (no copy of
  the ciphertext): ChaCha20-Poly1305 bulk reads 107M to 41M instructions
  a MiB on LLVM, 173M to 57M on arm64 (bench/results/tls-perf-2026-10-15.md).
- bench/perf/tls: TLS handshakes per group and server key, and bulk
  reads per AEAD, of std/http's client with the Xo TLS against Mbed TLS
  (`XO_LLRT_C=tlsio`), against crypto/tls servers. Baseline in
  bench/results/tls-perf-2026-10-15.md: the Xo handshakes take 3.8 to
  8.9 times Mbed's instructions on LLVM (P-256 field arithmetic about 90%
  of them), bulk AES-GCM about 10 times and ChaCha20-Poly1305 7.8 times.
- Native runtime (zero-C audit item B31, server wiring, decision 0202):
  the HTTP server's TLS is Xo by default. std/native/llrt/tlssrv.xo
  runs tls13srv.xo over the accepted connection's socket with xo_tls.c's
  waits, cancellation, HTTP detection and close_notify. It hands TLS 1.2
  ClientHellos to tls12srv.xo (tls13srv.xo's new HANDOFF state) and picks
  the certificate by server name. http.tls_cert is tls.X509KeyPair in Xo:
  RSA PKCS #1/#8, ECDSA SEC 1/PKCS #8, Ed25519 PKCS #8, with Go's errors
  (golden http_tls_keys equals Go). XO_LLRT_C=tlssrv keeps Mbed TLS
  (tlssrv_c.xo). http_tls passes on both paths. TestH2Diff and
  TestTLSDiff match Go except for the TLS 1.2 CBC suites and session
  resumption (no tickets yet), plus the client-side differences already
  known. UNTESTED on linux and windows.
- Native runtime (B31, TLS 1.2 part 2): the TLS 1.2 server in Xo
  (std/native/llrt/tls12srv.xo, as Go's handshake_server.go): the
  server's suite order (ChaCha20-Poly1305 first when the client prefers
  it), the client's curve and signature scheme order, ECDHE over X25519,
  P-256 and P-384 signed with ECDSA, Ed25519, RSA-PSS or PKCS #1 v1.5,
  the extended master secret, ALPN with Go's h2 fallback, the downgrade
  sentinel when it also speaks TLS 1.3; tls12srv.resume takes over from
  tls13srv.xo's HANDOFF. The golden tls12_server replays crypto/tls's
  client against crypto/tls's own server as the Go reference: RSA-PSS
  and PKCS #1 v1.5 signatures byte for byte, ECDSA through the trace's
  signature and checked when made here, 21 cases on Go, LLVM and arm64.
- Native runtime (zero-C audit item B31, part 8, decision 0202): the Xo
  client negotiates TLS 1.2 and 1.3 as Go's does: one ClientHello
  offering both (Go 1.26's suites, extensions and signature schemes in
  its order), RFC 8446's downgrade sentinel checked, and on a ServerHello
  choosing 1.2 tls13.xo stops (HANDOFF) and tlsio.xo goes on in xo-fa's
  tls12.xo (resume) from that ServerHello; no second try. ALPN offers h2
  and http/1.1 as the C path does. This closes part 7's TLS 1.2 gap: by
  hand on 2026-10-15, badssl.com's TLS 1.2 hosts (tls-v1-2, rsa2048,
  rsa4096, ecc256, ecc384) answer 200 and its expired, wrong host,
  self-signed and untrusted root hosts fail with Go's texts.
  TestTLS13ReplayDiff (internal/llvm, not -short; decision 0202's
  differential fuzzing) feeds the TLS 1.3 client mutated server records
  (bytes flipped, cut, inserted; records dropped, repeated, swapped;
  handshake messages changed under a valid record protection; fed in
  pieces) and diffs the Xo client on LLVM and arm64 against the Go
  reference port: 60,000 cases over three seeds, no difference in what
  the client does; about 19 percent differ only in a refused
  certificate's text (x509.xo parses some certificates Go's parser
  refuses, such as a malformed OID or an invalid PrintableString, and
  Go adds a hint to an unknown authority), the next item to tighten.

- Native runtime (B31, TLS 1.2 version negotiation): tls12.resume takes
  over from tls13.xo when a TLS 1.2 ServerHello answers one ClientHello
  offering TLS 1.3 and 1.2 (as Go's client: no second, 1.2 only hello),
  with RFC 8446's downgrade sentinels refused and the offered signature
  schemes read from that hello. The golden tls12_client adds crypto/tls's
  combined hello against a TLS 1.2 server and both sentinels (24 cases).
- Native runtime (zero-C audit item B31, TLS 1.2 part 1, decision 0202):
  the TLS 1.2 client in Xo (std/native/llrt/tls12.xo, as Go's
  handshake_client.go, key_agreement.go, prf.go and cipher_suites.go):
  ECDHE over X25519, P-256 and P-384, ECDSA, RSA PKCS #1 v1.5, RSA-PSS
  and Ed25519 signatures, the AES-GCM and ChaCha20-Poly1305 suites, the
  extended master secret, ALPN and SNI, an empty certificate when asked
  for one; Go's error texts and alerts. internal.crypto_op's "tls12"
  area; the golden tls12_client replays traces of crypto/tls's server,
  with crypto/tls's own client as the Go reference (std/rt
  internal_tls12.go), over 21 cases on Go, LLVM and arm64. Not on the
  TLS path yet (Xo Lang 2 integrates).
- Native runtime (zero-C audit item B31, part 7, decision 0202): the
  HTTP client's TLS is Xo's. std/native/llrt/tlsio.xo runs tls13.xo's
  client over the connection's socket with xo_tls.c's waits (poll
  descriptor, deadline, cancellation, the 10 s handshake timeout) and
  its texts of a failed handshake; hclient.xo calls it. XO_LLRT_C=tlsio
  selects its C path, tlsio_c.xo, which forwards to xo_tls.c on Mbed TLS
  (decision 0193). The roots stay the C's records: a new
  xo_tls_c_roots_crt hands the Xo client their mbedtls_x509_crt chain,
  whose DERs x509.xo parses. http_tls passes on both paths, arm64 and
  LLVM. Fetched by hand on 2026-10-15 with the Xo path: google.com,
  github.com, proxy.golang.org (which the Mbed path refuses with a host
  name error), cloudflare.com, letsencrypt.org and apple.com.
  Known gaps before merge: the Xo client refuses servers that only
  speak TLS 1.2 (all of badssl.com, for one) until xo-fa's tls12 client
  lands (closed by part 8), and XO_LLRT_C=tlsio restores Mbed;
  macOS's system roots are checked by x509verify.xo, not the Security framework as Go does; a
  certificate's expiry text gives the time in UTC, not the local zone.
  The server's TLS stays Mbed's until xo-2b's tls13srv.xo lands.

- Native runtime (zero-C audit item B31, TLS 1.3 server, decision 0202):
  the TLS 1.3 server in Xo, sans I/O (std/native/llrt/tls13srv.xo, as
  Go's handshake_server_tls13.go: ClientHello checks, Go's choice of
  suite and group, HelloRetryRequest, ALPN, EncryptedExtensions,
  Certificate, CertificateVerify signed with RSA-PSS, ECDSA P-256 or
  P-384, or Ed25519, Finished, KeyUpdate, Go's error texts and alerts)
  over tls13.xo's key schedule and tlsrec.xo's records. internal.crypto_op
  gains the tls13srv area (std/rt/internal_tls13srv.go, the server in Go
  and a pair over Go's crypto/tls); the golden tls13_server replays RFC
  8448 section 3 from the server's side (ServerHello, encrypted flight,
  ticket, data and alert records equal the RFC's, whole and a byte at a
  time), ten failures and choices, three recorded Go 1.26 client
  handshakes, and ten client and server pairs. Not on the TLS path yet
  (Mbed TLS stays selectable).
- Native runtime (zero-C audit item B31, part 6, decision 0202): the
  TLS 1.3 client checked against Go's crypto/tls server. The golden
  tls13_go replays traces recorded from a Go server (HelloRetryRequest
  from X25519 to P-256 with ChaCha20-Poly1305 and ALPN; a
  CertificateRequest answered by an empty Certificate with AES-256-GCM,
  SHA-384 and a P-384 ECDSA server, then a KeyUpdate asking for the
  client's; an Ed25519 server over a P-256 share; fed whole and a byte
  at a time; an expired certificate and no root). The client's records
  equal the trace's, which the Go server accepted. The Go reference is
  now a full port of tls13.xo (std/rt/internal_tls13.go), and the client
  takes fixed random, session id and key hooks. rsa.xo gains
  sign_pss_salt (a fixed salt) for the TLS 1.3 server's RFC 8448 replay.

- Native runtime (zero-C audit, the xo_rt.c memory block): real-time
  sections (core 6.5, decision 0133) are Xo: obj.xo queues and drains
  the deferred frees, std/native/llrt/rtsec.xo enters and leaves the
  sections and faults with XO-F007; xo_rt.c 144 lines deleted, 24
  added. hclient.xo dials through netconn.xo's dial_timeout.
  TestSpecListsEveryFaultCode reads std/native's .xo files too.
  UNTESTED on linux and windows.
- Native runtime fix: a task's context, allocated from the Xo heap
  (8 aligned) since B18 part 3, embedded the unwinder's 16 aligned
  exception object, which UndefinedBehaviorSanitizer reported as a
  misaligned xo_ctx in every program with tasks (TestGoldenASan). The
  exception now sits at the first 16 byte aligned address of an 8
  aligned area in the context (xo_rt.c ctx_exc); no offset moved.
- Native runtime (zero-C audit item B31, part 5, decision 0202): the
  TLS 1.3 client's key exchange over X25519 (x25519.xo), now its default
  share before P-256 and P-384, a low order share refused as
  crypto/ecdh does; tls13.xo exposes its message encoders, the key
  schedule (handshake_secrets, app_secrets), Finished, the signature
  scheme mapping and key shares for the TLS 1.3 server. The golden
  tls13_rfc8448 also replays RFC 8448's trace from the client's X25519
  private key and a low order server share.

- Native runtime (zero-C audit items B22 and B23, remnants batch 1): the
  network is Xo (std/native/llrt/netconn.xo): Net.listen and Net.dial
  (TCP and "unix:PATH"), Listener.accept, addr and close, Conn.read,
  write and close, and the NetErr outcomes, over sock.xo, dns.xo and
  poll.xo. xo_rt.c keeps one line forwarders under the names the
  generated code calls. The C library's network path (XO_LLRT_C=net,
  net.xo, net_c.xo) and the test world's listener exports (tw_listen, tw_ml_*) are
  deleted; xo_rt.c 4,959 to 4,108 lines. Linux and Windows code
  untested (written on macOS); the Winsock shim stays C until the
  Windows pass.
- Native runtime (zero-C audit item B31, part 4, decision 0202): the
  TLS 1.3 client in Xo, sans I/O (std/native/llrt/tls13.xo, as Go's
  handshake_client_tls13.go: ClientHello with compatibility mode,
  ServerHello and HelloRetryRequest checks, the RFC 8446 key schedule
  over hkdf.xo, EncryptedExtensions with ALPN, the server's Certificate
  checked by x509verify.xo, CertificateVerify by x509.check, Finished,
  an empty client Certificate when asked, NewSessionTicket dropped,
  KeyUpdate, Go's error texts and alerts; ECDHE over P-256 and P-384)
  and its record layer (std/native/llrt/tlsrec.xo: AES-128-GCM,
  AES-256-GCM, ChaCha20-Poly1305 records, the nonce, inner content type,
  key updates). internal.crypto_op gains the tls13 area; the golden
  tls13_rfc8448 replays RFC 8448's 1-RTT trace (the client's Finished,
  data and alert records equal the RFC's, fed whole and a byte at a
  time) and six failures against a Go replay. X25519 (x25519.xo) is next;
  not on the TLS path yet (Mbed TLS stays, decision 0193).

- Native runtime (zero-C audit item B15, part 3): the rest of the test
  world is Xo and std/llrt/xo_testing.c is deleted (252 lines): the
  start (twrun.xo start; SIGPIPE in entry.xo), the virtual clock's move
  when every context waits (twrun.xo kick), FakeProc.signal_after's
  timer (poll.xo fire, FN_SIGNAL, on the poller thread or the clock's
  move), and the end's releases for the leak check (twrun.xo end).
  cabi_tw.xo drops five exports no C called. UNTESTED on linux and
  windows.
- Native runtime (zero-C audit, small files, 2026-10-15): a Time's Unix
  nanoseconds are std arithmetic (xo_time_of and xo_time_ns deleted);
  the Fs cancellation check is the native prelude's llrt_fs_canceled;
  on the native targets the SSA builders lower the wasm layer 0 calls
  (rt.fs_*) and rt.stdin_* off Windows to a fault, so xo_fs.c, xo_sys.c,
  and xo_stdin.c's stubs are kept only for the seed; BytesBuilder.write_rune
  is raw.xo push_rune; the LLVM backend calls obj.xo's string builder
  exports and both native backends cabi.xo's const_once, as arm64 did.
  xo_std.c's unused helpers and seven unused cabi.xo exports deleted;
  the C kept for the seed is labeled for the end pass. UNTESTED on linux
  and windows.
- Native runtime (zero-C audit item B28, part 3): the HTTP server's
  registration and connections, streamed bodies (sinks and the streams
  table), request values (the peer's address), and the client's dial,
  TLS start, and close are Xo (std/native/llrt/hconn.xo, hserve.xo,
  hclient.xo); Any values and fault_at are anyval.xo and cabi.xo's, with
  C forwarders kept for generated code. xo_http.c 1,079 to 115 lines
  (the Mbed TLS wrappers, weak free functions, and forwarders).
  UNTESTED on linux and windows.
- Native runtime (zero-C audit item B31, part 3, decision 0202): X.509
  chain verification in Xo (std/native/llrt/x509verify.xo, as Go 1.26's
  Certificate.Verify: chains through the intermediates to a root, roots
  first, each link a signature from a CA allowed to sign certificates,
  validity times, path lengths, unhandled critical extensions, extended
  key usages nested through the chain, DNS name constraints per chain
  after building as Go 1.26 does, the host name through tlsname.xo).
  internal.crypto_op gains the chain area; the golden tls_verify diffs 41
  cases against Go's x509.Verify (outcome and chain). Not on the TLS path
  yet.

- Native runtime (zero-C audit item B31, TLS primitives 4, decision
  0202): X25519 in Xo (std/native/llrt/x25519.xo), RFC 7748, as Go's
  crypto/ecdh X25519, constant time: the Montgomery ladder over all 255
  bits with masked swaps on ed25519.xo's field arithmetic (now shared:
  its field functions are pub, field_workspace gives x25519.xo its own
  slots). A low order point gives all zeros, which the TLS layer must
  reject as Go's ECDH does. crypto_op's "x25519" area; the golden
  crypto_prims checks RFC 7748's 5.2 vectors and 1,000 iterations, 6.1's
  key exchange, low order and non canonical points, and random scalars
  against Go. Not on the TLS path yet.

- Native runtime (zero-C audit item B31, TLS primitives 3, decision
  0202): ChaCha20, Poly1305, and the ChaCha20-Poly1305 AEAD in Xo
  (std/native/llrt/chacha.xo), RFC 8439 (12 byte nonce, 16 byte tag),
  constant time: 32 bit additions, rotations, and xors, and
  poly1305-donna's 26 bit limbs with a masked final reduction; open
  checks the tag with ct.bytes_equal before decrypting. crypto_op's
  "chacha" area (Go's standard library has none, so internal_crypto.go
  writes RFC 8439 out, Poly1305 with math/big); the golden crypto_prims
  checks RFC 8439's 2.3.2, 2.4.2, 2.5.2, 2.6.2, 2.8.2 and appendix A.3's
  Poly1305 edge cases, tag failures, and every length to 200 against Go.
  Not on the TLS path yet.

- Native runtime (zero-C audit item B31, TLS primitives 2, decision
  0202): AES and AES-GCM in Xo (std/native/llrt/aesgcm.xo),
  as Go's crypto/aes and cipher.NewGCM (12 byte nonce, 16 byte tag),
  constant time: BearSSL's bitsliced aes_ct64 (no table indexed by a
  secret) and ghash_ctmul64 GHASH; open checks the tag with
  ct.bytes_equal before decrypting. crypto_op's "aes" area; the golden
  crypto_prims checks FIPS 197 appendix C, the GCM test cases 1 to 4,
  15, 16, tag failures, and every length to 80 against Go. Not on the
  TLS path yet.

- Native runtime (zero-C audit item B31, part 2, decision 0202): P-256
  and P-384 (std/native/llrt/nistec.xo: ECDH and ECDSA signing and
  verification, Montgomery field arithmetic and the complete addition
  formulas of Renes, Costello, and Batina, a masked 4 bit window, as Go's
  nistec), DER (asn1.xo, as cryptobyte), X.509 certificate parsing and
  signature checks (x509.xo, as Go's crypto/x509 parser: names as raw
  DER, validity, keys, SANs, basic and DNS name constraints, key and
  extended key usages, unhandled critical extensions), constant time
  integers modulo any odd modulus (bigmod.xo, as Go's bigmod), and RSA
  (rsa.xo: PKCS #1 v1.5 and PSS verification and signing, PKCS #1 and
  PKCS #8 private keys, CRT with a fault check). internal.crypto_op gains
  the areas p256, p384, rsa, and x509; the goldens tls_nistec, tls_x509
  (under -short), and tls_rsa diff them against Go's crypto/ecdh,
  crypto/ecdsa, crypto/rsa, and crypto/x509 (PKCS #1 v1.5 signatures byte
  for byte). Not on the TLS path yet.

- Native runtime (zero-C audit item B31, TLS primitives 1, decision
  0202): SHA-256, SHA-384, and SHA-512 (std/native/llrt/sha256.xo,
  incremental with clone, SHA-384 and SHA-512 on B08's SHA-512
  compression), HMAC (hmac.xo) and HKDF with TLS 1.3's
  HKDF-Expand-Label (hkdf.xo), as Go's crypto/sha256, crypto/sha512,
  crypto/hmac, and crypto/hkdf. std/internal's crypto_op runs them: one
  dispatcher, an arm per area calling that file's op (std/rt's
  internal_crypto.go is the Go backend's, over Go's crypto packages); the
  golden crypto_prims checks FIPS 180-4, RFC 4231, RFC 5869, and RFC
  8448 vectors and runs under -short. Not on the TLS path yet.

- Native runtime (zero-C audit, next small files batch): file and lock
  handles are records of fsops.xo with Xo free functions (the services
  record's words 128 and 136); collection constants are consts.xo's
  (its table at word 144; xo_const_once and xo_consts_free forward to
  it); Str and Bytes slicing (`s[lo..hi]`) calls the prelude's
  llrt_str_slice_at and llrt_bytes_slice_at. xo_fs.c 133 to 62 lines,
  xo_bytes.c 232 to 200. TestFarOutSlot now checks the rs_push_rune
  call, the last runtime call that returns a Str through a length
  pointer in that program. UNTESTED on linux and windows.

- Native runtime (zero-C audit item B30): the TLS runtime's certificate
  work that needs no TLS state is Xo (std/native/llrt/tlscert.xo):
  http.tls_cert's PEM scan and tls.X509KeyPair's error texts,
  Client.with_roots' certificates, the system's roots (the Security
  framework on macOS, the ROOT store on Windows, crypto/x509's files and
  directories on Linux), the expiry texts in RFC 3339, and macOS's chain
  verification. Mbed TLS calls stay in xo_tls.c behind a services
  record. Linux and Windows code untested (written on macOS).
- Native runtime (zero-C audit, small files batch): standard input's
  waits, registration, raw read, isatty, terminal width, and echo are
  Xo (stdin.xo; Windows keeps its pump in C); the test world's switches
  moved to twflags.xo so sysio.xo and stop.xo read them without the C;
  the outcome of a file operation (status, path, message) lives in
  thread local words 11 and 12 (sysio.xo); getauxval reads
  /proc/self/auxv (linux.xo); the string helpers behind str_byte,
  str_slice, str_join, str_repeat, and bytes_decode are raw.xo's.
  xo_stdin.c 226 to 66 lines, xo_sys.c 64 to 24, xo_bytes.c 272 to 232,
  xo_std.c 226 to 191. UNTESTED on linux and windows.

- Native runtime (zero-C audit item B28, part 2): the HTTP/2 server is
  Xo (std/native/llrt/h2srv.xo): the connection's fiber (the preface,
  the frames and their checks, header blocks through hpack.xo, flow
  control, the stream table, the handler queue and its rapid reset
  limit, GOAWAY and the deadlines), the responses of the stream tasks
  (whole, streamed, the backlog wait, the reset after a handler's fault)
  and the sending (HPACK encoding in send order, DATA within the
  windows). The connection's and the streams' shared state are malloc'd
  records under a lock word; the fiber waits with poll.xo's new
  pd_wait_or_signal. The C keeps the TLS handshake and non-blocking
  reads, the socket's writes, and the stream connections (xo_h2_stream).
  std/llrt/xo_h2.c is deleted (2,068 lines), xo_http.c 1,284 to 1,079;
  a stream's response header is http1.xo's (hdr_*), so the C header
  list, request record, and their accessors went too. Windows' linger
  after a weak cipher GOAWAY is untested on windows.
- Native runtime (zero-C audit item B26, part 2): the program's start
  and end around C main are Xo (std/native/llrt/entry.xo): the
  arguments (UTF-8 from the UTF-16 command line on Windows),
  XO_HEAP_MID_MAX, the choice of the test world or the signal handlers
  and their installation (sigaction on Linux and macOS,
  SetConsoleCtrlHandler, the vectored stack overflow handler and the
  stack guarantee on Windows), XO_LEAKCHECK's and XO_HEAPSTATS' reports,
  and XO_RTSTATS' line. C keeps the handlers themselves, the program
  thread and its fault boundary, and the standard streams' binary mode on
  Windows. Linux and Windows code untested (written on macOS).
- Native runtime (zero-C audit item B28, part 1): the HTTP/1.1 server
  connection is Xo (std/native/llrt/http1.xo Reader, respond, emit,
  stream_end: the input buffer, the request's body, Go's error answers,
  Expect: 100-continue, the response and a stream's chunks over
  http1w.xo); the C keeps the socket and TLS reads and writes, the
  server's registration, and HTTP/2. The C drivers of TestHTTPParseDiff
  and TestHTTPFuzz are skipped until they become Xo programs (end pass).
  xo_http.c 1,909 to 1,265 lines (with B29's client).
- Native runtime (zero-C audit item B22, stage 4): on Windows the event
  loop's descriptor operations and waits are Xo too (poll.xo), and so are
  the AFD polls (pd_arm, their completions) and the poller thread's loop
  over the I/O completion port (poll_main_win); the C keeps opening the
  device, registering a socket, and the WSAPoll fallback. Written on
  macOS and untested on Windows.
- Native runtime (zero-C audit item B25, stage 5): on Windows, Proc.run,
  Cmd.output, run, and spawn, pipelines, and children are Xo too
  (std/native/llrt/proc.xo over windows.xo: CreateProcessW with an
  explicit handle list, exec.LookPath with PATHEXT, the program's pipe
  ends in PIPE_NOWAIT mode polled in short steps instead of the C's
  socket pumps). xo_proc.c's Windows code is deleted (1,431 to 24
  lines). Written on macOS and untested on Windows.
- Native runtime (zero-C audit item B15, part 2): the test world's
  FakeClock (sleeps, advance, set), FakeNet's in memory listeners,
  testing.run_main, FakeProc.signal_after, and os.proc.exit's unwind in a
  test are Xo (std/native/llrt/twrun.xo); xo_testing.c keeps the clock's
  move when every context waits, the signal timer's callback, and
  forwarders (617 to 252 lines).
- Native runtime (zero-C audit item B04, stage (e), decision 0230): in a
  program built with no C library (XO_NOLIBC=1, Linux) the start (the
  environment, the auxiliary vector, the main thread's TLS block and
  stack), getenv, getauxval, vsnprintf and vfprintf, FILE, and
  directories are Xo (std/native/llrt/libcio.xo), and the thread and
  lock work moved from cabi_nolibc.xo to libcthr.xo, which fixes a
  duplicate `xo_x_mutex_lock` (cabi.xo's Mutex[T]) that broke every
  XO_NOLIBC arm64 link; xo_nolibc.c 658 to 128 lines. The malloc state
  and strerror's table share static word 63 (words 30 and 31 freed).
  Written on macOS and untested on Linux.
- Native runtime (zero-C audit item B04, decision 0230): a program built
  with no C library (XO_NOLIBC=1, Linux) takes its C library from Xo,
  exported under the C names (std/native/llrt/cabi_libc.xo): malloc and
  friends (libcmem.xo), the string and memory functions, strerror, the
  strto* family and qsort (libcstr.xo), and the system calls, errno,
  exit, signals, locks, and threads (libcsys.xo); xo_nolibc.c keeps the
  entry, FILE and printf, and directories (1,415 to 658 lines).
  Written on macOS and untested on Linux.
- Native runtime (zero-C audit item B25, stage 4b): Cmd.spawn's
  children (write, close_stdin, read, read_err, read_line, wait, kill,
  and FakeProc's children) are Xo on macOS and Linux
  (std/native/llrt/proc.xo; standard input data is written by the
  child's own waits instead of a thread; Linux UNTESTED). Off Windows,
  xo_proc.c is a 40 line shim (the child record's allocation and free
  function, and faulting stubs the prelude's Windows branch names); the
  rest of it compiles on Windows only (UNTESTED there: the Mac cannot
  build Windows).
- Native runtime (zero-C audit item B23, stage 2a): on Windows the
  network's socket calls go through Xo too (sock.xo's Windows arms over
  ws2_32, Winsock errors mapped to the C runtime's errno; names through
  ws2_32's getaddrinfo in cabi_net.xo, whose errors now read as glibc's
  texts, "Name or service not known"). Written on macOS and untested on
  Windows.
- Native runtime (zero-C audit item B25, stage 4a): Proc.run,
  Cmd.output and run, and pipelines run from Xo on macOS and Linux
  (std/native/llrt/proc.xo run and exec: the pipes, the reads on the
  event loop's poller, standard input written in the same loop instead
  of a thread, the exit polled with waitpid in cancelable steps; Linux
  UNTESTED); the C keeps them for Windows only, and Cmd.spawn's
  children until stage 4b.
- Native runtime (zero-C audit item B29, HTTP/2 client): the client's
  HTTP/2 connections, the pool of idle ones, and the exchange's records
  are Xo (hclient.xo); xo_h2.c keeps only the server.
- Native runtime (zero-C audit item B29, HTTP/1.1): the client's
  connection reads and writes, the request it writes (net/http's
  Request.write with Transport's additions), the reading side of the
  response, and the client's errors are Xo (std/native/llrt/hclient.xo;
  the errors are no longer cut at 511 bytes). UNTESTED on linux and
  windows (the would block errno per OS). Fixed: growing a block the C
  frees lost its first word (raw.c_resize; select arms, a scope's old
  task lists, a context's held locks).
- Native runtime (zero-C audit item B20, part 3): the deadlock check
  (XO-F001) is Xo (pool.xo dl_check, cabi.xo deadlock); in the test
  world with a virtual clock it still records the verdict for the C's
  clock move (tw_kick).
- Native runtime (zero-C audit item B18): running a claimed task is Xo
  (tasks.xo run_claimed: the task's context, from the heap, its budget,
  the end of its run and its scope's counts); only the call of the entry
  function stays C (xo_rt.c run_entry, sched_rt op 29), whose frame is
  the unwinding's boundary.
- Native runtime (zero-C audit item B18): the free functions of tasks,
  scopes, mutexes, channels, selects and Atomics are Xo (tasks.xo,
  mutex.xo, chan.xo, selects.xo), and runtime objects allocate through
  obj.xo's alloc_obj, which refills from the central lists when several
  threads run.
- Native runtime (zero-C audit item B20, part 2): the fiber stacks'
  settings (XO_TASK_STACK_MB, the page size per OS) are Xo
  (poolsetup.xo fibers_setup).
- Native runtime (zero-C audit item B26, part 1): two phase shutdown's
  state machine (core 7.5: the signals counted, the stop request, the
  grace timer, task.stopping's channel, proc.set_grace, main's root
  context, testing.run_main's fresh state) is Xo
  (std/native/llrt/stop.xo); the C keeps the signal and console
  handlers, the entry, and a services record of the C calls it needs.
- Native runtime (zero-C audit item B19, part 3): `select` (its arms,
  the wait over every channel's lock in address order, take) is Xo
  (std/native/llrt/selects.xo), which generated code calls directly.
- Native runtime (zero-C audit item B19, part 2): channels (new, send,
  recv, try_send, try_recv, close, len) are Xo
  (std/native/llrt/chan.xo), which generated code calls directly; a
  waiting context's queue entry is part of the context.
- Native runtime (zero-C audit items B25 stages 2 and 3, B15 part 1):
  commands start from Xo (std/native/llrt/proc.xo: the PATH lookup,
  posix_spawn on macOS, fork and execve on Linux), one Proc.run path for
  every OS, and a test binary's report channel in testworld.xo;
  xo_proc.c and xo_testing.c lose about 400 lines.
- Native runtime (zero-C audit item B19, part 1): Mutex[T] (lock,
  unlock, with_read's shares, the reader handoff, XO-F004's held list)
  is Xo (std/native/llrt/mutex.xo), which generated code calls directly.
- Native runtime (zero-C audit item B29, part 1): the HTTP/2 client's
  exchange is Xo (std/native/llrt/http1.xo h2_exchange, H2X): the
  request's HEADERS and DATA within the server's windows, the server's
  frames (SETTINGS, PING, WINDOW_UPDATE, GOAWAY, RST_STREAM, HEADERS and
  CONTINUATION, DATA), the response's status and header, and Go's error
  texts; the connection's windows, settings, and next stream are part of
  its H2State. The C keeps the socket and TLS reads and writes, the pool,
  and the retry (xo_cl_h2_read, write, end). http_fuzz.c no longer runs
  the client's frame code (it is Xo's).
- Native runtime (zero-C audit item B24): standard input's reading
  (read_line, read_all, lines, read_bytes, the buffer kept across a
  canceled read, the readers' turn) is Xo (std/native/llrt/stdin.xo); the
  C keeps the poller registration, the raw read, and the waits.
- Native runtime (zero-C audit item B28, part 6): a server request's
  url.host (net.SplitHostPort of the Host) and url.port (of an absolute
  target) are Xo (std/native/llrt/http1.xo host_name, host_port); the C
  hands over the raw Host and URL host (xo_hc_host, xo_hc_urlhost), and
  its splitting moved to the test copies in std/llrt/testdata/http_ref.c.
- Native runtime (zero-C audit item B28, part 5): HTTP/2 the SETTINGS
  checks that need no state (the payload's shape, duplicates,
  ENABLE_PUSH, INITIAL_WINDOW, MAX_FRAME_SIZE) are Xo (h2w.xo
  settings_check); a failing setting still fails when the C reaches it,
  after the ones before it apply.
- Native runtime (zero-C audit item B28, part 4): HTTP/2 the server's
  request checks (malformed pseudo fields, 431 for a cut header list,
  the 400 texts of connection specific fields and TE, OPTIONS *) are Xo
  (h2w.xo request_check).
- Native runtime (zero-C audit item B28, part 3): HTTP/2 the checks of
  each received header field (readMetaFrame: values, names, pseudo
  fields after regular ones, the header list's size) and checkPseudos
  are Xo (h2w.xo field_keep, pseudos_ok).
- Native runtime (zero-C audit item B28, part 2): HTTP/2 the server's
  frame checks before it acts on a frame (checkFrameOrder and the
  Framer's parsers: lengths, stream ids, padding, priority) are Xo
  (std/native/llrt/h2w.xo frame_check).
- Native runtime (zero-C audit item B28, part 1): the HTTP/2 frames the
  server and the client write (frame headers, RST_STREAM, WINDOW_UPDATE,
  GOAWAY, SETTINGS, HEADERS and CONTINUATION) and the server's response
  heads (writeChunk's :status, sorted and checked fields, content-type,
  content-length, date) are Xo (std/native/llrt/h2w.xo); a head goes to
  the HPACK encoder as bytes instead of a C header list. xo_h2.c loses
  152 lines and gains 78 (40 of them the weak definitions). A handler
  setting one name twice keeps the values' order (qsort did not).
- Native runtime (zero-C audit item B23, stage 1): on macOS the network's
  socket calls go through Xo (std/native/llrt/sock.xo's darwin arms over
  libSystem; names through libSystem's getaddrinfo in cabi_net.xo), as on
  Linux.
- Native runtime (zero-C audit item B22): the event loop's timer heap and
  the poller thread's loop (kqueue on macOS, epoll on Linux) are Xo
  (std/native/llrt/poll.xo); Windows' loop stays C for now. The event
  loop's mutex and a descriptor's are lock words and the pending signals
  a word; the epoll arms were written on macOS and are untested on Linux.
  A descriptor's waits (with a deadline), close, and free are poll.xo's
  too (pd_wait, pd_close, pd_free), callable from Xo.
- Native runtime (zero-C audit item B25, stage 1): Proc.kill,
  FakeProc.killed, and the environment and working directory a command
  starts from are Xo (std/native/llrt/proc.xo); xo_proc.c loses 106
  lines.
- Native runtime (zero-C audit item B21, part 2): a thread's heap record
  and the list of every record are std/native/llrt/heapos.xo's (a
  mapping, a lock word) instead of calloc and a pthread mutex; the
  unused list of exited threads' records is deleted
  (bench/results/heap-platform-2026-10-14.md).
- Native runtime (zero-C audit item B21, part 1): the heap's 4 MiB chunks
  and its big blocks (above 256 KiB) are mapped from Xo
  (std/native/llrt/heapos.xo: mmap, VirtualAlloc) instead of the C
  library's malloc, with a cache of given back mappings, and the central
  lists' lock is a lock word instead of a pthread mutex. bench/perf
  text, json, alloc, and nested are 0.973 to 1.030 of main's
  instructions, the self build 1.002 and 1.004
  (bench/results/heap-platform-2026-10-14.md).
- Native runtime (zero-C audit item B18, part 2): the spawn and a scope's
  open and close are Xo (std/native/llrt/tasks.xo), which generated code
  calls directly; xo_rt.c loses 145 lines and gains 70. Single threaded
  spawn and await on the Mac are 1.006 to 1.026 of main's instructions.
- Native runtime (zero-C audit item B18, part 1): a task's end, `await`,
  a scope's failed task, `s.cancel()`, and `os.tasks` are Xo
  (std/native/llrt/tasks.xo), which generated code calls directly; a
  context's wait list entry is part of the context. xo_rt.c loses 158
  lines and gains 71; bench/perf/ctx 0.942 to 1.022 on the Mac.
- Native runtime (zero-C audit item B17, decision 0229): the shields of
  deferred code, the unwinding flag, and resource budgets are Xo
  (std/native/llrt/ctx.xo, budget.xo), which generated code calls
  directly; ctx.xo holds the context record's layout as one asserted
  table. xo_rt.c loses 287 lines and gains 87; bench/perf/ctx 0.921 to
  1.029 of main's instructions on the Mac. XO-F005 prints its time as a
  Duration, as the Go backend does.
- std/time on Windows: `time.local` builds the zone as Go's time.Local
  does (transitions for 100 years each side of now, no rule past them,
  Go's abbreviations), so times before 1926 and after 2125 match the Go
  backend; the C before it extended the current rule to every year.
- Native runtime (zero-C audit item B27): std/time's zone data and the
  table of loaded zones are Xo (std/native/llrt/tzhost.xo): the
  program's zone data through a `xo_tz_table` accessor the build writes
  beside it, the host's zone files, and Windows' local zone from
  GetDynamicTimeZoneInformation. std/llrt/xo_time.c (519 lines) is
  deleted.
- Native runtime (zero-C audit item B06, part 3): a fault on Windows
  unwinds through the Xo walker (std/native/llrt/unwind.xo win_raise,
  over RtlVirtualUnwind) instead of xo_rt.c's C walk, which is deleted
  with its LSDA reader. Windows goldens 157 of 157. The walk starts at
  the C fault_raise's captured context, and the unwind tables name the
  Xo personality export on Windows too; fault paths about 0.97 to 1.02
  of the C walk's wall clock time on windows/amd64.
- Native runtime (zero-C audit item B13, stage 3): the string objects'
  appends and builders are Xo (std/native/llrt/obj.xo): `a + b`, and
  the runtime's rs_append, push_byte, push_word, push_words,
  append_bytes, and with_cap. arm64 generated code and std/native call
  them with no C level, and a unique string with room is appended to
  without a call. bench/perf text, json, alloc, and nested are 0.776 to
  1.040 of main's instructions on arm64 and LLVM (json enc_small 0.776,
  text debug 0.801), the self build 0.998 and 0.999
  (bench/results/string-list-objects-2026-10-14.md). The C rs_new stays
  for the C's own string creators.
- bench/perf/sysio/windows-ci.sh runs the sysio Windows measurement on
  CI's Windows runner (`-f suite=script -f runner=xo-windows`).
- `XO_LEAKCHECK=1` on a native program that ends with `os.proc.exit` (or
  whose main returns an error) prints `xo leakcheck: skipped
  (os.proc.exit)` instead of nothing: the report runs only when main
  returns (gotchas/leakcheck-skipped-at-exit.md).
- Native code generation (decision 0228, zero-C audit item B06 part 2):
  a failed check (overflow, division by zero, an index out of range, a
  shift, a Float to Int conversion) and `fault(msg)` call the runtime's
  Xo exports `xo_x_fault_raise_*` directly; the C entry points forward
  to them until the next seed. The fault paths on the Mac are 0.962 to
  1.036 of main's instructions.
- Native runtime (zero-C audit item B16, option D): macOS on x86_64
  makes its file calls from Xo too (std/native/llrt/darwin.xo, through
  libSystem's stat64, lstat64, fstat64, and __getdirentries64, which
  both macOS architectures now use), so the C Fs operations
  (std/llrt/xo_fs.c) and the C fs.read are deleted: 831 lines net.
  macOS figures unchanged (bench/results/fs-ops-2026-10-14.md).
- Native runtime (decision 0227): the C path of the number and time text
  (`XO_LLRT_C=text`, std/native/llrt/text_c.xo and 514 lines of
  std/llrt/xo_std.c) is deleted; the owner accepted Duration text at
  1.112 of the C's instructions on arm64 with 0.925 on LLVM.
  `XO_LLRT_C=text` now fails the build.
- Native runtime (zero-C audit item B06, part 1): a context's fault
  record and the report of an uncaught fault are Xo
  (std/native/llrt/fault.xo); xo_rt.c loses 57 lines and gains 23. The
  fault paths on the Mac are 0.993 to 1.006 of main's instructions. A
  new test raises a fault through the C library's qsort, covering the
  unwinder's shared object lookup on macOS and Linux.
- Native runtime (decision 0221 point 10): a fault on macOS arm64
  unwinds through the Xo walker, which reads the compact unwind of
  `__unwind_info` itself, instead of libunwind. bench/perf/fault is
  0.083 to 0.123 of libunwind's instructions on the Mac. C frames
  between a fault and its catch are passed over on macOS arm64 too.
  darwin/amd64 keeps libunwind until stage 2.
- Native runtime (zero-C audit item B16, part 2): on Windows the Fs
  operations past read, write, and stat (directory names, rename,
  remove, set_modified, file handles, copy, modes, links, locks) call
  kernel32 from Xo (std/native/llrt/windows.xo under fsops.xo), with
  the C path's errors and messages. xo_win.c's POSIX shims for them are
  deleted (260 lines); xo_fs.c's operations stay only for darwin/amd64.
  macOS figures unchanged (bench/results/fs-ops-2026-10-14.md).
- Native runtime (zero-C audit item B02, part 2): the Bytes primitives of
  the system call layers (`bytes_addr`, `bytes_buf`, `bytes_len`) are Xo
  in std/native/llrt/raw.xo, and the per thread system call scratch is
  256 bytes of the thread's heap record (obj.xo `scratch`) in place of a
  C `__thread` buffer; std/llrt/xo_sys.c loses 48 lines net.
- Native code generation (decision 0221 point 7): the unwind tables name
  the runtime's Xo personality export `xo_x_personality` directly, except
  on Windows and x86_64. The C forwarder `xo_personality` stays for
  code built by the 0.11.0 seed and goes after the next seed.
- Native runtime (zero-C audit item B05, decision 0221 stage 3): a fault
  on glibc linux/arm64 unwinds through the Xo unwinder
  (std/native/llrt/unwind.xo) instead of libgcc's forced unwind; shared
  objects' unwind tables are found through ld.so's link_map. On macOS
  the personality routine is Xo over libunwind. C frames between a
  fault and its catch are passed over on Linux arm64 and their cleanups
  no longer run (only the runtime's own C can sit there; `use c` cannot
  call back into Xo). The TSan test builds drop function entry and exit
  instrumentation. bench/perf/fault is 1.001 to 1.009 of the C's
  instructions on the Mac and 0.405 to 0.624 of libgcc's on Linux arm64
  (bench/results/unwinder-glibc-2026-10-14.md).
  glibc linux/amd64 keeps libgcc until stage 2.
- Native runtime: the C the 0.10.0 seed still called is deleted now that
  seed 0.11.0 is out (xo_static_word, xo_ct_barrier and xo_crypto.c,
  the print, exit, env, and rune count forwards; about 200 lines), and
  `Stdio.write` writes through std/native/llrt/stdio.xo instead of the C
  `xo_print` (gotchas/runtime-c-the-seed-still-calls.md).
- Native runtime (zero-C audit item B13, stage 2; decision 0226): the
  list objects (a new list, growth, the copy of a shared list, the
  release of a list's elements) and the drop worklist are Xo
  (std/native/llrt/obj.xo). arm64 generated code calls them directly,
  with no C level: they find the thread's heap themselves, and their
  common cases make no call. The C entry points forward to them for
  LLVM and the seed. bench/perf alloc, nested, text, json and xo are
  0.883 to 1.021 of the C's instructions on arm64 and LLVM, the self
  build 1.000 (bench/results/string-list-objects-2026-10-14.md). The
  string objects stay C.
- Native runtime (zero-C audit item B14): the test world's part 1 is
  Xo (std/native/llrt/testworld.xo): `xo test --real`'s switches, seeded
  Rand, the generators of property tests and testing.gen, captured
  output, FakeEnv, FakeProc.exit_code and on_run, FakeStdio.set_input,
  log records, FakeNet's handlers, failures, and requests, MemFs, and
  the cleanup; std/llrt loses 916 lines and gains 87. Test reports are
  unchanged (TestNoGCTests, TestNoGCTestLeaks on both native backends);
  instructions in bench/results/test-world-2026-10-14.md (every test within
  5 percent of the C, rand 1.006 and gen 0.924 on arm64). The virtual
  clock, in memory listeners, and testing.run_main stay C (B15).
- Compiler: a constant Int expression (`const MIN_INT: Int =
  -9223372036854775807 - 1`, `5 - 1`) is one constant at every use, not
  its operations; testing.gen's generators no longer record their
  choices (only property tests report them).
- arm64 backend (decision 0224): shrink wrapping and tail calls. A
  function makes its frame (frame record, callee saved registers) only
  on the paths that need it, and a call whose results are returned as
  they are is a branch after the epilogue. The self build of xo runs
  0.985 of the instructions; bench/perf 0.963 to 1.004 of before; the
  Xo list objects of B13 move from 1.242 to 1.201 (alloc small) of the
  C's instructions on the Mac (bench/results/shrink-wrapping-2026-10-14.md).
- Tests: `go test -short` over the repository 2.2 times faster (35.0 s
  to 15.7 s on the shared Mac) and `./cmd/xo -short` 4.3 times (24.6 s
  to 5.7 s): every cmd/xo test is parallel (a per test environment for
  the xo processes, cmd/xo/env_test.go), internal/llvm caches its C
  compiles by content (XO_CC_CACHE), internal/gen keeps `use go`
  bindings and generated programs across runs, the harness self test
  runs per method in parallel; scripts/seed-pack.sh takes
  XO_SEED_GZIP_LEVEL (bench/results/short-tests-2026-10-14.md).
- Faster native rebuilds, reference counting per function (decision
  0220 addendum stage C): each function's reference counting is cached
  beside its assembly, keyed by the function before the pass, the types
  it reaches, and the borrowed parameters it calls with; verify checks
  it too. A one-module edit of xo now rebuilds in about 12 s (31 s
  without the cache) and 20 to 25 s of CPU (131 s) on the Mac.
- Faster arm64 rebuilds, per function (decision 0220 addendum stage B):
  each function's assembly is cached in <user cache>/xo/backend under
  the hash of its SSA, the types it reaches, the signatures of the
  functions it names, and the compiler, so a rebuild generates only the
  functions an edit changed; `XO_BACKEND_CACHE=verify` generates every
  function and fails on a cached text that differs, `off` turns the
  cache off, and all three give the same binary. After a one-module
  edit of xo the emit phase drops from 6 to 9.5 s to about 0.2 s and
  the build's CPU time from about 140 s to about 65 s on the Mac.
  --release builds do not use it.
- Faster arm64 rebuilds (decision 0220 addendum stage A): the parts of a
  large program's assembly are cut by their contents and named by their
  first function, and each part's object is cached in
  <user cache>/xo/backend (`XO_BACKEND_CACHE=off` turns it off), so an
  unchanged part is not assembled again: after a one-module edit of xo
  the assemble phase drops from about 6 s to 1.5 s on the Mac, and the
  binary is the same as without the cache
  (bench/results/fast-builds-2026-10-13.md).
- Native runtime (zero-C work item B16, part 1): on Linux and
  darwin/arm64 every Fs operation past read, write, and stat (read_dir,
  walk, rename, remove, remove_all, create_dir_all, create_temp,
  cwd_path, set_modified, file handles, copy, modes, links, and locks,
  with lock's 10 ms polled wait) makes its system calls from Xo
  (std/native/llrt/fsops.xo over linux.xo and darwin.xo); results and
  messages are unchanged, read_line costs 0.68 to 0.82 of the C's
  instructions, the rest within noise
  (bench/results/fs-ops-2026-10-14.md). Windows and darwin/amd64 still
  use the C runtime's xo_fs.c.
- Native runtime (zero-C audit item B13, stage 1): std/http's parser
  and client search text with std/native/llrt/scan.xo (SIMD) instead of
  the C `xo_str_find`, which is deleted; single byte searches scan only
  their range. The stack trace's padding counts runes in Xo. HTTP/1.1
  server 0.961 to 1.013 of the C's instructions
  (bench/results/string-list-objects-2026-10-14.md). The string and
  list objects were ported to Xo and measured, and stay C: on arm64
  they took up to 1.27 times the C's instructions (no shrink wrapping
  in the arm64 emitters); the results file ranks the next steps.
- Native runtime (zero-C audit item B11): the HTTP/2 client's header
  blocks are encoded and decoded by Xo (std/native/llrt/hpack.xo, which
  the server already used); the client's request fields are listed in
  Xo (http1.xo `h2_exchange`) and the C HPACK copy leaves the runtime
  (xo_h2.c 3,606 to 3,056 lines). HTTP/2 client requests measure 0.982
  to 1.009 times the C's instructions
  (bench/results/http2-client-hpack-2026-10-14.md; new h2len and h2big
  workloads in bench/perf/httpclient).
- Native runtime (zero-C audit item B09, decision 0222): a value barrier
  for constant time runtime code (`rt.ct_barrier`, internal: no
  instruction on arm64, an empty asm on LLVM) and constant time masks in
  std/native/llrt/ct.xo; `crypto.equal`, verify_password's comparison,
  and Ed25519 (std/native/llrt/ed25519.xo) are Xo, the same results, the
  C deleted (std/llrt/xo_crypto.c 540 lines to 16). Ed25519 takes 0.944
  to 0.971 of the C's instructions on arm64 and 0.757 to 0.768 on LLVM
  (bench/results/ed25519-consttime-2026-10-14.md) and now builds for
  wasm too. `xo dev codegen` patterns can
  name another module's function with a `// func MOD.NAME` line
  (test/codegen/consttime). Tests: internal/llvm TestConstantTimeLLVM
  (clang -O2 keeps the barriers; no branch or select in the masks) and
  TestCryptoXoDiff (crypto.equal and Ed25519 against Go on both native
  backends).
- xo prunes its caches by age and size again (decision 0223): the build
  directories and the Go backend's cache (whose trims went away with the
  Go driver), the binary cache, and the test harness's xo builds
  (`checkout/<key>`, the newest 4 kept). Entries unused for 24 hours go
  (`XO_CACHE_MAX_AGE`, e.g. `6h`, `0` off), then the least recently used
  past 20 GiB per cache (`XO_CACHE_MAX_GB`); nothing used within two
  hours is removed, and a prune runs at most once an hour.
- Native runtime (zero-C audit item B03, stage 1): standard output's
  64 KiB buffer, its flush, and standard error are Xo
  (std/native/llrt/stdio.xo, process globals in rt.static_word);
  Stdio.print and friends call it through the native prelude, and the C
  runtime flushes through cabi.xo's exports. 1,000,000 println cost
  0.788 (arm64) and 0.859 (LLVM) of the C path's instructions
  (bench/results/stdio-2026-10-14.md). os.proc.exit, env.get, env.vars,
  env.args, and env.program are Xo too (std/native/llrt/process.xo).
- Runtime process globals (zero-C work item B02, decision 0207): an
  internal `rt.static_word(k)` intrinsic (64 zeroed words) in the arm64,
  LLVM, and wasm emitters; std/log's state (log.configure's level,
  format, and output; the dependency attribution of decision 0173 as a
  thread local word; the record time) is Xo, and the 66 line logging
  section of the C runtime is gone. The wasm backend now lowers the
  runtime's thread local words and runs std/log programs.
- Native runtime (zero-C audit item B12): the HTTP/1.1 response writer
  is Xo (std/native/llrt/http1w.xo: status text, DetectContentType, the
  Date header, chunkWriter.writeHeader's rules, the error answers),
  called by the C connection code through cabi_http.xo's exports; HTTP/2
  sniffs and dates through it. 514 lines of xo_http.c deleted; the bytes
  on the wire are unchanged; http workloads 0.976 to 1.001 of the C's
  instructions on HTTP/1.1, HTTP/2 within noise
  (bench/results/http-writer-2026-10-14.md).
- Programs linked with no C library (XO_NOLIBC=1) on linux/arm64 unwind
  faults: deferred code runs and tasks and `task.isolate` catch faults,
  through an unwinder written in Xo (std/native/llrt/unwind.xo:
  .eh_frame_hdr lookup, DWARF CFI interpreter, the personality routine;
  decision 0221). The 12 fault goldens that ended at the fault now pass
  on the arm64 and LLVM backends; xo_nolibc.c loses its unwinder stubs.
- Native runtime (zero-C audit item B10): std/http's client reads
  HTTP/1.1 responses, follows redirects, and parses URLs in Xo
  (std/native/llrt/http1.xo `client_do`, `parse_response`); the C keeps
  the connection, TLS, HTTP/2, and the request's writing. The HTTP/2
  server's request URLs are parsed by Xo too. 1,629 lines of C leave the
  runtime (the C request parser stays as test data for the C fuzzers);
  the client measures 0.953 to 1.001 and the HTTP/2 server 1.006 to
  1.008 times the C's instructions (bench/results/http-client-2026-10-14.md),
  and its answers to 457 responses match the C client's byte for byte.
  Chunked bodies decode faster (5 to 6 percent fewer instructions for
  the server's chunked requests).
- Version 0.10.0 and its prebuilt seeds (decision 0216); the C Rand
  that only the 0.9.0 seed called is deleted (113 lines of xo_crypto.c
  and xo_win.c).
- Smaller native output and faster x86 self builds (decision 0220):
  reference counting releases a dead struct once instead of once per
  heap field (arm64 assembly of xo 17 percent smaller, 73 percent fewer
  calls; LLVM IR 72 percent smaller); `XO_LLVM_OPT` sets clang's level,
  and development builds of xo on x86 use -O0 (xo rebuilds itself in
  1:23 on c7i.8xlarge, was about 42 minutes); parallel jobs default to
  the physical cores and the memory.
- Native runtime (zero-C audit item W1): the scheduler's C fallback,
  `XO_LLRT_C=sched`, is deleted: 1,284 lines of std/llrt/xo_rt.c (its
  fiber switch assembly, thread locals, run queue ring, worker pool,
  parks, fiber stacks, spawn and join copies), std/native/llrt/sched_c.xo,
  `rt.sched_c`, and -DXO_SCHED_C. The scheduler is sched.xo and pool.xo
  only; `XO_LLRT_C=sched` now fails the build. The live C path names
  are net and text.
- Fixes from the Linux arm64 milestone run of 2026-10-14: `--ld=macho`
  links every object of a program whose assembly was split into parts
  (decision 0220; it took only the first and rejected the runtime's
  source); `xo bind` finds or builds the Go binder xo-gobind from the
  std beside xo instead of "std" in the working directory. Tests:
  TestDevGoldensELF builds the linux/arm64 runtime pack it links with
  (`xo dev rtpack`, cached by pack key); TestCrossCompile looks up the
  wasm binary by xo's name, wasip1/wasm (xotool.Target);
  TestSelfhostLLVMEndToEnd compares its IR with the recording only when
  dump replays are on (decision 0217) and keeps its build and output
  check. From the linux/amd64 milestone: internal/llvm's diff tests skip
  the arm64 backend off arm64 hosts (xotool.Arm64Skip), and TestHarness
  builds xo before the Python self test (its 600 s adapter timeout no
  longer includes the build).
- Native runtime (zero-C audit item B08): SHA-512 is Xo
  (std/native/llrt/digest.xo, called by the C Ed25519 through
  cabi_crypto.xo's export; 0.887 of the C's instructions on arm64, 0.641
  on LLVM, bench/results/sha512-rand-2026-10-14.md) and Rand is Xo
  (std/native/llrt/rand.xo over getrandom, getentropy, BCryptGenRandom,
  or the test world's seeded draws, the same values as before). The C
  SHA-512 is deleted; the C Rand functions stay until the seed is a
  release with this change (gotchas/runtime-c-the-seed-still-calls.md).
- The clocks are read from Xo on every OS (work item B01, decision
  0213 item 6): Clock.now on macOS (clock_gettime_nsec_np) and Windows
  (GetSystemTimePreciseAsFileTime), and the monotonic clock of the
  scheduler, timers, and budgets everywhere (the vDSO on Linux,
  libSystem on macOS, QueryPerformanceCounter on Windows), the clock of
  the build chosen once into a word so the hot path tests no target.
  The C runtime's mono_ns, xo_clock_now, and xo_tz_now call the Xo
  exports; xo_win.c's clock_gettime shim is deleted. Clock.now on macOS
  takes 0.969 (arm64) and 0.977 (LLVM) times the C path's instructions,
  on Windows (LLVM) 0.928 to 0.952 times its CPU cycles
  (bench/results/clocks-2026-10-14.md). bench/perf/sysio gains a sleep
  workload.
- Native runtime: the C with no caller is deleted (zero-C audit item
  W0): xo_utf8_valid, xo_str_contains, starts_with, ends_with, split,
  lines, xo_bytes_span, rune_count, and hex (141 lines of std/llrt,
  their std/native/rt.xo declarations), and the -D flags of deleted C
  paths that xo's io_flags still added. A build with an `XO_LLRT_C`
  name that has no C path (no `std/native/llrt/<name>_c.xo`; `all`
  aside) now fails with `XO_LLRT_C=<name>: no C path named <name>`
  instead of silently building the Xo path; the live names are net,
  sched, and text.
- Faster native builds (decision 0220 stage 1): the arm64 backend
  generates a large program's functions in parts on parallel tasks and
  assembles the parts apart before one link; reference counting and the
  inlined module's verification run on tasks too; the runtime flags come
  from one scan of the assembly. xo builds itself in 59.00 s on the
  development Mac (was 337.28 s) and 49.83 s on r8g.8xlarge with
  `XO_EMIT_JOBS=16` (was 496.82 s), the output byte identical for any
  job count. `XO_EMIT_JOBS` sets the tasks (default the cores, at most
  8); `--phase-times` prints the `assemble` phase.
- Developer tooling (decision 0209): hidden `xo dev goldens|errorcheck|
  codegen|dump|rtpack|build-each` in xo. Golden programs are built,
  run, and compared byte for byte by xo itself, in parallel workers,
  largest first, with passes cached by input hash (arm64 set: 49.74 s
  with the Python script, 16.93 s with `-j 8`, 0.21 s cached). Error
  check and codegen pattern suites start in test/ (decision 0217).
  selfhost/macho assembles `svc`, so linux/arm64 `--ld=macho` builds
  work again.
- `xo lsp` answers `textDocument/completion` and
  `textDocument/signatureHelp` (decision 0219): fields and methods
  after `x.` (prelude methods included), variants and statics after a
  type name, public members after a package name, and elsewhere the
  locals in scope, the module's names, imports, prelude names, and
  keywords, with the hover's declaration line and doc comment, ranked
  first when their type is the one expected at the cursor; signature
  help shows the callee and its active parameter. A dangling `.` or an
  open call no longer needs to parse: the server checks a copy patched
  at the cursor.
- std: `Fs.cwd_path()` is the working directory's absolute path from the
  OS (decision 0218), on the Go, arm64, and LLVM backends; a test's MemFs
  gives `/`. The toolchain uses it instead of `$PWD`, so `xo lsp` started
  without a shell (VS Code from the macOS Dock passes `PWD=/`) resolves
  std paths again and Go to Definition into std works.
- Prebuilt seeds (decision 0216 item 1): a checkout with no xo fetches
  the newest seed (release N-1) from a private S3 bucket with the aws
  CLI, checks its sha256, and builds its xo in one hop
  (internal/xotool); a failed fetch is an error naming the cause, and
  the go-compiler-final root and the seed chain run only when asked for
  (`XO_SEED_FALLBACK=chain`, `go run ./scripts/xobuild --from-source`). `scripts/seed-pack.sh XO OUTDIR` packs a built xo as a
  seed. `XO_SEED_BUCKET` and `XO_SEED_URL` choose the location.
- The Go implementation of the compiler is deleted (decision 0215): Go
  cmd/xo, the hidden comparison dumps, and the Go compiler packages
  (lexer, parser, checker, SSA builder, Go, arm64, and LLVM emitters,
  build driver, modules, and the tools), 95,011 lines of non-test Go
  and 18,837 of tests. `xo` is the self hosted toolchain alone; the
  last commit with the Go compiler is tagged `go-compiler-final`. A
  checkout's `xo` is built by an earlier xo (`go run ./scripts/xobuild`;
  with none, by the xo1 of that tag). The Go backend stays (selfhost/gen
  and std/rt), as does `use go` (xo-gobind). The playground's compiler
  half (cmd/xoplay) is gone with it: the site builds it from the tag
  until an Xo replacement exists.
- Native runtime: the C paths of the runtime pieces ported to Xo that
  passed the 2026-10-13 batch 8 full tier are deleted (decision 0198,
  second round): tz, report, show, atomic, fspath, gen, glob, digest,
  ints, rc, alloc, scan, sysio, http1, hpack, and procspec, with their
  `std/native/llrt/<name>_c.xo` modules and their `-DXO_*_C` C: std/llrt
  is 3,223 lines shorter. `XO_LLRT_C` naming one of them now selects
  nothing; only `text`, `sched`, and `net` keep a C path.
- Native backends on Linux: the network's system calls (socket, bind,
  listen, accept4, connect, reads, sends with MSG_NOSIGNAL, socket
  options, names, shutdown, close, and the poller's epoll) are made from
  Xo (std/native/llrt/sock.xo), and host names are resolved by a
  resolver written in Xo (std/native/llrt/dns.xo) instead of glibc's
  getaddrinfo: /etc/nsswitch.conf's files and dns, /etc/hosts,
  /etc/resolv.conf (nameservers, search, ndots, timeout, attempts,
  rotate, single-request, use-vc), A and AAAA over UDP with TCP on
  truncation, and RFC 6724 ordering, following Go's resolver with
  glibc's error texts (decision 0206 stage 3). Its differences from
  glibc are listed at the top of dns.xo. `XO_LLRT_C=net` builds the C
  library path. With `XO_NOLIBC=1`, programs that dial or listen now
  link and run with no C library (std/http still needs it until
  decision 0202).
- Linux: Clock.now calls the kernel's vDSO from Xo instead of the C
  library (0.864 to 0.967 of the C path's instructions,
  bench/results/linux-syscalls-2026-10-13.md); `XO_LLRT_C=sysio` keeps
  the C clock. Static programs (`XO_NOLIBC=1`, decision 0206) take their
  clock, locks, signal handlers' installation, threads, and thread local
  storage layout from Xo, and link again with the arm64 backend (they
  failed on libgcc's atomics helpers). Output is unchanged.
- Fixed: ThreadSanitizer builds on Linux ran out of address space for
  task stacks under load (`fault: out of memory` in conc_stress, not a
  leak); their task stacks are now 2 MiB (XO_TASK_STACK_MB overrides).
  `XO_RTSTATS=1` reports `fiber_live` and `fiber_peak`, and a failing
  task stack mapping says how many stacks were mapped
  (knowledgebase/gotchas/linux-tsan-fiber-stacks.md).
- Native backends on macOS (arm64) and Windows: `os.fs` read, write,
  and stat call libSystem and kernel32 directly from Xo (decision 0213)
  instead of going through the C runtime, with the same errors and
  messages byte for byte; on macOS within the C path's instructions
  (0.562 to 1.013 times, bench/results/platform-calls-2026-10-13.md).
  `XO_LLRT_C=sysio` builds the C path.
- The self-hosted toolchain's Go backend holds the shared lock on the Xo
  Go cache while go build runs, as the Go implementation does, so a
  concurrent cache trim no longer removes entries a build is reading
  (decision 0214). The lock comes from `std/internal`, a module only std
  and the toolchain can import; Xo's public `fs.lock` is unchanged.
- `xo` run with no std library to find (no $XO_STD and no release
  layout beside the binary), or with a std whose prelude lacks a core
  type, now says so (`std library not found: ...`, exit 2) instead of
  faulting in the checker with `prelude type missing: Int`.
- Native backends: the rest of the task scheduler's machinery is Xo
  (std/native/llrt/pool.xo, decision 0212 stage 2): the worker pool
  (waking, idling, stealing, the inject queue), the runtime locks' slow
  paths and their futex waits (libSystem's __ulock, WaitOnAddress,
  futex(2)), worker thread starts (pthread_create from Xo on macOS,
  CreateThread on Windows), task stacks with their guard pages and
  caches, parks, the cancellation tree, a spawn's queueing, and a
  scope's join. Within the 5 percent bar on both backends
  (bench/results/task-switch-2026-10-13.md, Stage 2); `XO_LLRT_C=sched`
  builds the C path. The C runtime's default build is 523 lines shorter.
- Native backends: the task switch is emitted by the compiler (no
  assembly left in the C runtime's default build), and the scheduler's
  core (each worker's run queue, starting, parking, and waking fibers,
  task states) is Xo (std/native/llrt/sched.xo, decision 0212), called
  from the C runtime through C ABI exports; thread local state lives in
  a table the compiler emits. Within the 5 percent bar on both backends
  (bench/results/task-switch-2026-10-13.md); `XO_LLRT_C=sched` builds the
  C path. Fixed on the way: `--as=macho` builds failed on `prfm` (since
  the heap moved to Xo) and on `sxtw` (http programs).
- Native backends: the memory allocator (size classes, free lists,
  chunks, and the magazines that hand freed blocks between threads) is
  Xo (std/native/llrt/alloc.xo, decision 0207 stage 3), called from the
  C runtime through C ABI exports; within 5 percent of the C's
  instructions on both backends and below it on LLVM
  (bench/results/raw-memory-2026-10-13.md, bench/perf/alloc).
  `XO_LLRT_C=alloc` builds the C path.
- Faster list code on the LLVM backend (decision 0211): the compiler
  tells LLVM which lists cannot share storage (a `var` list once made
  unique, a new list, the borrowed parameters, as Xo's value semantics
  guarantee), so it keeps lengths, drops index checks, and vectorizes
  code that writes one list while reading another. Argon2id and
  BLAKE2b in std/crypto are Xo again on the native backends, at 1.043
  times the C's instructions on LLVM and 0.980 on arm64
  (bench/results/llvm-noalias-2026-10-13.md); `XO_LLRT_C=digest` builds
  the C. Output is unchanged.
- Fixed: go commands that ran on the Xo Go cache outside an Xo build
  (`go vet` in tests, the Go binder's `go list`, tool builds) could lose
  cache entries to a concurrent trim and fail with `could not import
  strings (... no such file or directory)`; they now hold the cache's
  shared lock (gotchas/go-commands-outside-build-race-trim.md).
- Native backends: the C runtime's reference count operations (retain,
  release, claim, and marking a value shared between threads) are Xo
  (std/native/llrt/rc.xo, decision 0207 stage 2), called from the C
  through C ABI exports; within 5 percent of the C's instructions on
  both backends (bench/results/raw-memory-2026-10-13.md).
  `XO_LLRT_C=rc` builds the C path. Lists of zeros made by std's
  implementation (deflate's tables, regexp's queue, JSON object key
  tables) take one allocation and a memset instead of a push per
  element. Fixed: the wasm backend reported std code using the raw
  memory intrinsics as unsupported.
- Added: `./...` and `DIR/...` on every command that takes directories
  (check, test, build, vet, doc, fmt, fix, parse, audit). It means DIR,
  which those commands already walk below, so `xo check ./...` equals
  `xo check .`; a `DIR/...` with no `.xo` file under it is an error
  (exit 1) (decision 0210).
- Changed: directories whose name starts with `_` are skipped like
  `testdata` and `.` names, as the go command skips them (decision 0210).
- Fixed: `xo help` lists every command with its purpose, and every
  command takes `-h`/`--help` (usage on stdout, exit 0), also after a
  subcommand (`xo edit rename --help`); `xo` alone and `xo build -h` no
  longer print the internal xo1 usage line or its internal flags; an
  unknown flag exits 2 with the command's usage on stderr.
- The LLVM backend lowers std's internal spin loop hint to the target's
  pause instruction (`yield` on arm64, `pause` on x86-64) instead of
  nothing (decision 0200).
- Windows builds are deterministic: the link passes lld-link `/Brepro`,
  so rebuilding the same sources gives a byte identical `.exe` (the COFF
  time stamp is a hash of the output, not the link time; decision 0197).
- Native backends: more of the C runtime is Xo (decisions 0204, 0207):
  the text of every arithmetic, index, and conversion fault, Int and
  Float text, UTF-8 decoding, white space, Int parsing, Duration and Time
  text used by the runtime itself, `Net.only`'s address checks, and the
  TLS client's and server's name checks, PEM decoding, and alert texts.
  Messages are unchanged byte for byte; TLS name checks now follow Go's
  `net.ParseIP` exactly. Under the hood the C runtime can call runtime
  functions written in Xo, and std's implementation gets raw memory
  intrinsics (internal only). A program built with the arm64 backend is
  about 20 KB larger (LLVM builds are not).
- Added: on Linux, the native backends' os.fs.read, write, and stat
  (and read_dir's and walk's stats) make their system calls from Xo
  (std/native/llrt/linux.xo and sysio.xo, decision 0206) with the C
  runtime's semantics and messages: reads take 0.45 to 0.67 times the
  C path's user instructions on Graviton, the rest within 1 percent
  (bench/results/linux-syscalls-2026-10-13.md). `XO_LLRT_C=sysio`
  builds the C path. Clock.now stays on the vDSO.
- Added: `XO_NOLIBC=1` (experimental, Linux, xo1's arm64 and LLVM
  backends): a static executable with no C library at all (decisions
  0203, 0206 stage 1); a hello world and a file and clock program run in
  an empty root on linux/arm64 and linux/amd64. Faults end such a
  program without running deferred code; sockets past epoll, DNS,
  std/http's TLS, and `use c` still need the C library.
- Faster list code on arm64 (decision 0208): an element access is one
  load or store with an immediate or a scaled register offset (the
  list's data address computed once per loop when the loop cannot move
  the list), bounds checks a guard before an `a || b` proved stay
  removed, 32 by 32 bit products of masked values are `umull` and
  `umaddl`, and a loop that stores into a list it received shared makes
  it unique once before the loop instead of checking at every store
  (all native backends). Argon2id written in Xo went from 2.21 to 1.13
  times the C's instructions on arm64 (1.29 to 1.25 on LLVM); it stays
  C. Output is unchanged.

- Faster on the native backends: Str.split, Str.lines, Str.rune_count,
  Str.find and contains, Bytes.to_hex, Bytes.rune_count, and
  Bytes.to_str (UTF-8 validity) are now Xo in the runtime, scanning 16
  bytes a step with NEON on arm64 and vector compares on LLVM (decision
  0201). They retire from 0.32 to 1.01 times the instructions of the C
  they replace (rune_count about a third); `XO_LLRT_C=scan` builds the
  C path for comparison. Output is unchanged.
- Faster: `Atomic[T]` on the native backends no longer calls the C
  runtime: get, set, swap, and compare_and_swap are one inline atomic
  instruction each (get is a single `ldar` on arm64) and add and max are
  Xo code (std/native/llrt/atomic.xo), 0.60 to 0.94 times the C path's
  instructions (bench/results/atomics-2026-10-13.md; `XO_LLRT_C=atomic`
  restores the C). Under the hood, std's implementation modules get
  atomics with C11 memory orders (decision 0200, internal only).
- **Sized integers are much faster on the native backends, and more of
  the runtime is Xo** (decision 0195's portable group): Int8 to UInt64
  arithmetic, shifts, and conversions are checked inline instead of a C
  call each (5 to 20 times fewer instructions on bench/perf/portable),
  Float32 operations round inline, and the faults, UInt64 division,
  `Time.unix`, the Str Map key hash, Int's checked_ and saturating_
  methods, MemFs.fail_next's patterns, SHA-256, HMAC, PBKDF2,
  verify_password's parsing, and the values of property tests are Xo
  (std/native/llrt/ints.xo, glob.xo, digest.xo, gen.xo). Output is
  unchanged (TestIntsDiff, TestDigestDiff, TestGlobDiff, TestGenDiff
  against the Go backend). Argon2id stays C (its Xo version was 2.07
  times the C's instructions on arm64). `XO_LLRT_C=ints,glob,digest,gen`
  builds the C paths for comparison
  (bench/results/runtime-port-portable-2026-10-13.md).

- Changed: on the native backends, the HTTP/2 server's HPACK (header
  block decoding and encoding), the HTTP/1.1 server's request parsing
  (request line, header fields, request targets and queries, chunked
  bodies and trailers, and std/http's URL helpers), the Fs path walks
  (create_dir_all, remove_all, create_temp, the working directory), and
  the environments and Windows command lines of child processes are Xo
  (std/native/llrt hpack.xo, http1.xo, fspath.xo, procspec.xo; decision
  0193 step 6), as fast as the C or faster (HTTP/1.1 GETs 13 to 16
  percent fewer instructions, HTTP/2 within 2 percent;
  bench/results/runtime-port-step6-2026-10-13.md). `XO_LLRT_C=hpack`,
  `http1`, `fspath`, `procspec` build the C paths for comparison.
- Fixed: `--backend=arm64` (the Go driver and xo1) failed with
  "internal: reads virtual register N before it is written" on a loop
  that clears a flag and breaks, followed by a test of the flag and code
  that goes on from the loop's index; a block entered through a Bool
  join (`a && b`, `a || b`, such a flag) now reuses only the registers
  of the block that dominates it (gotchas/arm64-vreg-read-before-write-break-flag.md).
- Fixed: a chunked request body with a chunk size of 2^63 or more made
  the native HTTP/1.1 server copy a negative length; an IPv6 request
  host with an IPv4 part written with a leading zero is now rejected as
  the Go backend rejects it.
- **The xo1 golden scripts no longer report show_table as failing**:
  scripts/selfhost-goldens.py (and selfhost-gogoldens.py,
  elf-goldens.py) read want.txt in Python's text mode, which turned the
  raw `\r` show_table prints into `\n`, so the golden "differed" on xo1
  although xo1's output matched it byte for byte. They now read it with
  `newline=''`; internal/diag's TestScriptsReadWantBytes keeps it so
  (gotcha python-text-mode-want-cr.md). No compiler change.
- **The C paths of the ported runtime pieces are gone** (decision
  0198): the Unicode operations (`XO_UNICODE=c`, xo_unicode.c), std/json's
  text (`XO_LLRT_C=jsontext`, xo_json.c), the number text
  (`XO_LLRT_C=num`), and the Str operations (`XO_LLRT_C=strings`), 3,373
  lines of std/llrt. Those settings now select nothing; the Xo versions,
  the default since decisions 0180 and 0193, are the only ones. No pack
  object changes (packs never held these paths).
- **xo for Windows can be built by Xo** (decision 0197 stage 7): `xo
  build --backend=llvm --targets=windows/amd64` (or windows/arm64) now
  cross builds from Linux or macOS when `XO_WINSYSROOT` names a Windows
  sysroot (MSVC's C library and the Windows SDK, e.g. made by xwin)
  and an upstream clang with lld-link is used. xo built this way from
  Linux runs on Windows, passes the 145 Windows goldens, and builds
  itself on Windows to the same output. Fixed: on Windows, `xo` started
  by name from cmd.exe did not find its standard library
  (`fault: prelude type missing: Str`).
- Faster: the self hosted compiler (xo1) compiles selfhost/xo 3.6 times
  faster in 37 percent of the memory (588.86 s to 163.83 s, 26.1 GB to
  9.66 GB on the Mac; bench/results/xo1-speed-2026-10-13.md), with the
  same assembly: liveness and dce by worklists over sparse sets, fewer
  allocations per instruction, insert_rc's liveness and borrow sites
  without per pass maps. On the native backends an empty list literal
  `[]` allocates nothing until its first push (decision 0199), and the
  heap prefetches on each allocation from a free list. `XO_HEAPSTATS=1`
  prints a native program's heap by size class at exit.
- Changed: the xo1 that native builds of the development `xo` run
  (switch point A) is cached on its own inputs (selfhost/, the std
  modules it uses, the `xo` version) in `<user cache>/xo/xo1/bin/<key>`,
  so a change to another std module, the C runtime (std/llrt), or
  third_party no longer rebuilds it (5 to 12 minutes); the std beside it
  is still written from the full embedded set.
- The self hosted toolchain runs on x86-64 hosts: on darwin/amd64 and
  linux/amd64, `xo build|run|test --backend=llvm` (and `--no-gc`, whose
  default backend is LLVM there) go through xo1, which `xo` builds with
  the LLVM backend split into 12 modules without LTO (decision 0178
  stage 7, decision 0196). That xo1 passes the LLVM goldens on both
  hosts and rebuilds itself to a byte identical fixed point on both
  with no Go on PATH. scripts/bootstrap-xo.sh picks the
  LLVM backend on x86-64 hosts; scripts/remote-xo1-x86.sh and
  scripts/xo1-host-check.sh run the check on EC2 and the Intel Mac.
- **More of the native runtime is Xo** (decision 0193 step 5): std/time's
  zone rules (TZif parsing, the POSIX TZ footer, the lookup and Go's
  cache) are std/native/llrt/tz.xo, the C keeping only the zone data
  and the table of loaded zones; a Rune's Display and Debug text and a
  UInt64's text (the leaves of the Debug text of collections) are
  show.xo; the report of a property test's case to the harness is
  report.xo. Output is unchanged (TestTZDiff over every zone of Go's
  database, the new goldens show_table and tz_table, TestNoGCTests).
  `XO_LLRT_C=show,tz,report` builds the C paths for comparison
  (bench/results/runtime-port-step5-2026-10-13.md).
- Fixed: in a native test (`xo test --no-gc`, arm64 and llvm), an http
  request to an in memory server sent after a stop request waited in
  the listener's backlog instead of failing, and under load could end
  the test with `XO-F001: deadlock`; it now fails at once, as on the Go
  backend (gotchas/mem-listener-backlog-after-serve.md). Also fixed: on
  a loaded machine the virtual clock of a native test could move twice
  for one idle instant, so a task woken by the first move saw the time
  of the second (gotchas/virtual-clock-double-kick.md).
- **A native build on macOS needs no C compiler** (decision 0182 next
  steps 1 and 2): `xo build --backend=arm64` (and run, test) now
  assembles and links with Xo's own Mach-O assembler and linker when the
  release's runtime pack covers the program and no `use c` source is
  linked; otherwise it uses cc as before. `--ld=cc` keeps cc, `--ld=macho`
  insists on Xo's linker. Linux hosts keep cc for now. Release archives
  carry Go's zone database as `lib/xo/zoneinfo.zip`, which `xo` reads
  before asking `go env GOROOT`, so programs that name time zones build
  with no Go installed. Fixed on the way: a native build for several
  targets of different OSes (`--targets=darwin/arm64,linux/arm64`) gave
  every target the host's predeclared `target`; each OS now gets its
  own; `xo rtpack --status` on linux/amd64 named linux/arm64.
- **`xo` is written in Xo** (decision 0178, switch point B): the release
  archives ship the self hosted toolchain as `bin/xo`, with
  `bin/xo-gobind` (for `use go`) and `lib/xo/` (std, third_party, the
  prebuilt runtime) beside it; keep that layout, `bin/xo` alone does not
  run. The Go `cmd/xo` only builds the first one: scripts/release.sh
  builds it, it builds the darwin/arm64 `xo`, and that `xo` builds the
  runtime packs, the linux/arm64 `xo` (linked by its own ELF linker, no C
  compiler), and the linux/amd64 `xo` (Go backend).
  `scripts/release-verify.sh` checks an archive on its host; cmd/xo's
  suite passes against the archive's `xo` on darwin/arm64 and
  linux/arm64. Fixed on the way: the ELF assembler's `.file`, glibc's
  `stat` family for binaries linked without cc (glibc before 2.33 has no
  `stat` in libc.so.6), runtime flags turned on by symbol names inside
  string literals, the predeclared `target` of a cross build (it was the
  compiler's), the Go backend and `--no-gc` of the self hosted `xo` on
  an amd64 host, and paths printed by `xo serve` on Linux.
- Windows, decision 0197 stages 5 and 6: native programs wait for
  sockets with AFD polls on a completion port (a round trip stays at
  44 us with 4000 idle connections, where WSAPoll took 11.9 ms; set
  `XO_WINPOLL=wsapoll` for the old poller), read standard input from a
  thread so reads can be cancelled, take the local time zone from the
  system setting (as the Go backend), read the environment as UTF-8,
  report a stack overflow as the Go backend does (`fatal error: stack
  overflow`, status 2), and no longer close a socket when closing one of
  hundreds of open files. The LLVM runtime tests and `xo test
  --backend=llvm` run on Windows; `scripts/remote-windows.sh --batch`
  runs the Windows checks of a batch (scripts/windows-batch.sh). Fixed:
  a native test's client waiting for an in memory listener that was
  freed slept forever (test_shutdown deadlocked on Windows).
- Windows paths (decision 0197 stage 4, std.md 3.1 and 3.5): on
  Windows `Fs` takes `\` as a separator, drive paths (`C:\x`) and UNC
  paths (`\\host\share`) as absolute, and a path rooted at `/` on the
  working directory's drive; `Env.program`, `home`, `config_dir`,
  `cache_dir`, and every path an `Fs` returns use `/`. Native builds
  read the home directory from `USERPROFILE` and the user directories
  from `%AppData%` and `%LocalAppData%` on Windows, as the Go backend
  did. Other OSes are unchanged. All 145 LLVM goldens pass on Windows
  built by the self hosted `xo`.
- Windows, decision 0197 stage 2: native programs run processes
  (`Proc.run`, `Cmd`, `Child`, pipelines) on Windows through
  `CreateProcessW` (PATH and PATHEXT lookup, arguments quoted as Go's
  EscapeArg and on line ends, only the three standard handles
  inherited); XO1504 no longer stops any Windows build. On Windows
  `Child.kill`, cancellation, and `Proc.kill(pid, "KILL")` end a process
  with status 137 on both backends (the Go backend's was 1), and other
  signal names are an `Io` error. The self hosted `xo` builds Windows
  programs too (`--backend=llvm` on a windows/amd64 host, and
  `--targets=windows/arm64` cross builds): 143 of 144 LLVM goldens pass
  built by it, and all 144 build for windows/arm64.
- The LLVM backend builds Windows programs (windows/amd64 host, decision
  0197 stage 1): `xo build --backend=llvm` with LLVM's clang
  (x86_64-pc-windows-msvc, lld) and the MSVC build tools. The runtime's
  POSIX use maps onto Win32 and Winsock (std/llrt/xo_win.c): threads,
  tasks on fibers, faults with deferred code, timers, files, sockets,
  HTTP and TLS, standard input. 140 of 144 LLVM goldens pass on Windows
  Server 2025. A program using processes (Proc) stops with the new
  XO1504 (a std API the native runtime does not support on the target
  yet). A Windows host also cross builds the other Windows architecture
  (`--targets=windows/arm64`: 141 of 144 goldens build and link; none
  run yet, for lack of a Windows on Arm host). scripts/remote-windows.sh
  runs a command on, or holds, a temporary Windows EC2 instance.
- The LLVM backend builds and passes the golden suite on darwin/amd64
  (decision 0196): on an Intel Mac with macOS 26.5.2 the whole
  internal/llvm package passes, TestGolden 143 of 143, with no emitter
  or runtime change. `scripts/intel-mac-test.sh` runs a test command for
  a commit on that Mac. The LLVM tests now drop AddressSanitizer where
  it builds but cannot start, as they did for ThreadSanitizer.
- The WebAssembly backend (decision 0179) coalesces each function's
  locals by liveness and lays out reducible functions as structured
  control flow (block, loop, br) instead of the dispatch loop: the self
  hosted checker's module is 8.3 MB (8.9 MB), runs seven goldens under
  Node in about 0.55 s and 340 MB (2.3 s and 2.1 GB), and checks all 144
  LLVM goldens in a Chrome worker (63 before, the rest overflowed the
  worker's stack). `os.fs.write` works in wasm (WASI `path_open` and
  `fd_write`).
- internal/selfhost scales with cores: every native build, Xo process,
  and Go side comparison draws from one pool of `XO_SELFHOST_JOBS`
  slots (default min(GOMAXPROCS, 4); scripts/remote-full.sh passes the
  core count), builds and the heavy runs also from
  `XO_SELFHOST_HEAVY_JOBS` (default one per 8 GiB of memory, 16 GiB in
  the full tier); heavy runs get a process each, backends and
  comparisons run at once, and the full tier prebuilds every component.
  The full tier takes 3,490 s on r8g.8xlarge with 32 jobs (over 3 hours
  before); scripts/remote-full.sh defaults to r8g.8xlarge (256 GiB).
  `-short` no longer samples the heavy runs (175 s to 31 to 52 s on the
  development Mac).
- The arm64 backend (both emitters) lowers a division or remainder by
  an Int constant to a multiply high and shifts instead of `sdiv`
  (decision 0194): faster, with the same faults (only the constants 0
  and -1 can fault). A negative literal is now a constant there (no
  overflow check). Duration text in Xo is still 1.171x of the C on
  arm64 and 1.087x on LLVM in instructions, so it keeps its C path.

- The self hosted `xo` can be bootstrapped without Go:
  `scripts/bootstrap-xo.sh SEED` builds selfhost/xo with a given xo and
  then with itself twice, with no Go on PATH, and checks the fixed point
  (identical assembly and binary); it passed on a clean Linux arm64
  machine with Go deleted (decision 0178 stage 7). Fixed on the way: the
  self hosted driver run with the default relative `--std` could not
  build the TLS library on an empty cache. The selfhost differential
  tests can record their Go side and replay it
  (`XO_SELFHOST_GOLDENS=record|replay`); internal/progen now gives one
  program per seed. The recording is committed as hashes (6 MB:
  internal/selfhost/testdata/gogoldens), per unit, and per function for
  the SSA, LLVM, arm64, and Go generator comparisons, so a replayed
  mismatch names the function; the mutation generators no longer need
  internal/lexer.

- The self hosted `xo` has `xo rtpack` (decision 0178): the prebuilt
  runtime packs scripts/release.sh ships, byte identical to the Go
  command's. Switch point B's gate passes: cmd/xo's whole suite against
  the self hosted binary laid out as a release, 69 of 71 tests passing
  and the two hidden comparison dumps (ssadump, llvmdump) exempt.
- `xo bind --c` is written in Xo (decision 0178 stage 6,
  selfhost/cli/bind): xo1 binds C headers with the same output and
  cache files, byte for byte, as the Go binder. `xo bind` for Go
  packages runs the new Go helper `xo-gobind` (cmd/xo-gobind), which
  stays Go. The self hosted checker now binds a `use go` package or a
  `use c` header missing from the binding cache, and fetches a module
  version missing from the module cache, before checking again, as the
  Go checker does; xo1 runs `xo publish` and prints the API changes of
  `xo get`. `xo publish` in a module of a repository subdirectory under
  a symbolic link (macOS's /var) now finds its tags.
- Linux fixes from the first full suite run on EC2 (Amazon Linux 2023,
  arm64; bench/results/linux-2026-10-12.md): the native runtime
  unregisters a socket or pipe from epoll when it closes it (a child
  process holding the file kept the registration, and the poller then
  touched freed memory) and makes descriptors close-on-exec at creation
  (SOCK_CLOEXEC, accept4, pipe2); HTTP/2 responses with no header fields
  no longer pass NULL to memcpy; Ed25519's constants initialize once
  (pthread_once) instead of racing; ThreadSanitizer builds on Linux use
  8 MiB task stacks; the arm64 backend's binaries are reproducible with
  gcc as `cc` (a fixed `.file`); the bin cache's copy fallback no longer
  makes a parallel exec fail with "text file busy". scripts/remote-full.sh
  gains `--cmd`, `--hold`, and per-package durations at the end of its log.

- The self hosted `xo` (xo1) closes the gaps of decision 0178's switch
  point B outside stage 6: `build`, `run`, and `test` default to the Go
  backend as `xo` does; std is found beside the binary (lib/xo/std of a
  release) through `os.env.program()`, without XO_STD; `xo check
  --no-gc` reports XO1201; `build --ci`, the workspace check of
  `--release`, and native `test --real` work; `xo lsp` answers hover,
  definition, references, rename, formatting, and semantic tokens as the
  Go server does; `xo replay` reads the rebuilt program's build
  information again. The Go `xo` can build xo1 from its embedded sources
  again (the nested selfhost/cli packages were missing).
- The server commands in Xo (decision 0178 stage 5, selfhost/cli/servers):
  xo1 runs `mcp`, `serve`, `dump`, `replay`, and `lsp` (diagnostics,
  quick fixes, symbols) with the Go servers' responses byte for byte,
  and forwards `check` and `query` to a running `xo serve`. Natively,
  `os.env.program()` builds, and removing from both a `Map[Str, Str]`
  and a `Map[Str, Bytes]` in one program no longer fails the SSA
  verifier.
- `os.proc.kill(pid, signal)` sends a signal (by name: `TERM`, `QUIT`,
  ...) to another process, and `os.net.listen` and `dial` take
  `unix:PATH` addresses for Unix domain sockets, on every backend; an
  attenuated `Proc` must list `"kill"`, an attenuated `Net` lists the
  socket as `unix:PATH`; `FakeProc` records signals (`killed()`) and
  `FakeNet` listens on sockets in memory (decision 0190).
- The core commands of `xo` are written in Xo too (decision 0178 stage
  2): `check`, `parse`, `explain`, `version`, `init`, `fmt`, `fix`,
  `vet`, and `doc` run in the self hosted driver xo1 with the same flags,
  exit codes, and output; the Xo formatter writes the same bytes as the
  Go one on every file of the repository. The self hosted checker now
  reports holes (XO0900). Two types of one name in different modules no
  longer share a generated SSA helper (the native build of such a
  program failed).
- The LLVM backend drops the overflow checks that value ranges prove
  (loop counters, binary search midpoints, operations under a branch or
  after an index), tells LLVM the ranges of loop variables, and inlines
  the functions the arm64 backend inlines (decision 0192): the Unicode
  operations written in Xo are now within 5 percent of their C versions
  on LLVM too.
- std/crypto signs and verifies Ed25519 (RFC 8032, decision 0189):
  `crypto.ed25519_public_key(seed)`, `crypto.ed25519_sign(seed, msg)`,
  `crypto.ed25519_verify(public_key, msg, sig)`, with the results of Go's
  crypto/ed25519 on every backend (native builds compute them in C).
- `archive.untar` skips a PAX global header (the one `git archive`
  writes) instead of failing on it, on every backend.
- Native builds of a program whose packages declare types of the same
  name no longer fail in the SSA verifier when both are printed with
  Debug, encoded as JSON, decoded from SQL, or generated in property
  tests (gotchas/ambiguous-type-generated-function-names.md).
- The module commands (`get`, `mod tidy|accept-effects|clean|proxy`,
  `vendor`, `audit`) are written in Xo too (selfhost/cli/modules,
  decision 0178 stage 4); xo1 runs them.
- A native program that asks whether a stream is a terminal
  (`stdout_is_tty`, `width`) without reading standard input links again
  (the runtime's terminal functions were left out).
- `xo query` and `xo edit` are written in Xo too (selfhost/cli/semantic,
  decision 0178 stage 3): `xo1 query|edit` print what the Go tool
  prints (37 of 37 command lines byte identical on cmd/xo's test
  programs; TestQueryCLI and TestEditCLI pass against xo1). The
  definition hashes of `xo query hash` (decision 0125) are byte identical
  to the Go ones on 270 of 270 modules (13,662 definitions).
- Native builds run the self hosted driver (decision 0178 switch point
  A, approved 2026-10-12): `xo build|run|test --backend=arm64|llvm` (and
  `--no-gc`) execute xo1, the driver written in Xo, which `xo` builds
  once from its embedded sources into the user cache (a few minutes the
  first time). Output, diagnostics, JSON, exit codes, and paths are the
  Go driver's; `XO_SELF_DRIVER=0` restores the Go driver. xo1 gained
  `run`, `xo build`'s flags, several directories, the build directory
  and binary cache, split LLVM builds, `--release`, `--stamp`, and the
  check's diagnostics (stage 1).
- `selfhost/macho` also writes ELF: `macho asm --elf` assembles the
  linux/arm64 emitter's assembly into the object `clang -c` writes (byte
  identical on the 159 arm64 goldens and on xo1), and `macho elflink`
  links it with the linux runtime pack into a dynamically linked glibc
  executable (versioned imports, stubs, TLS relaxation,
  `.eh_frame_hdr`). `xo1 build --target=linux/arm64 --ld=macho` builds a
  Linux program from any host with no C toolchain; every golden runs on
  glibc 2.31 and 2.41 (decision 0182 stage 3).
- The native backends no longer count the element of `for x in <place>`
  or an inner list read through an outer one (`a[i][j]`) when nothing
  changes the container while it is in use (decision 0186): on
  bench/perf/nested the read workloads went from 600,041 retains to 1
  (walk) and from 201,001 to 1 (index2) on arm64, allocations unchanged.
- The arm64 backend (and its Xo twin, byte identical) inlines small
  functions that call others and larger ones with at most two call
  sites, and drops the bounds, overflow, and shift checks that value
  bounds, loop conditions (also through `&&` and `||`), and binary
  search midpoints prove (decision 0184). The Unicode runtime in Xo is
  now within 5 percent of the C path on every bench/perf/unicode
  workload on arm64 (39 percent fewer instructions on mixed script case
  mapping, 42 on Rune case mapping).
- `selfhost/macho`, a Mach-O assembler written in Xo, turns the arm64
  emitter's assembly into the same object `cc -c` writes (every section,
  function, and relocation identical on the 158 arm64 goldens and on
  xo1's own assembly; all 158 golden objects byte identical), 6.8 times
  faster; `xo1 build --as=macho` uses it. Its linker links the objects
  with a prebuilt runtime pack into a signed executable, so `xo1 build
  --ld=macho` needs no C compiler at all for programs without `use c`
  sources (decision 0182).
- The WebAssembly backend's modules are much smaller (decision 0179,
  stage 3): the self hosted checker went from 43.2 MB to 8.9 MB and
  runs under Node in 1.9 s and 2.5 GB instead of 10.7 s and 5.9 GB.
  Large `var` parameters are passed by reference and kept in memory,
  programs where nothing waits keep no suspension frames, field
  releases are one call, locals are shared and renumbered.
  `selfhost/wasm --annotate` attributes a module's bytes to SSA opcodes.
- A WebAssembly backend written only in Xo (`selfhost/wasm`, decision
  0179): `xo1 build --backend=wasm` lowers the self hosted SSA to WAT,
  assembles it with an assembler written in Xo, and links a runtime of
  its own over WASI preview1; no C compiler, linker, or Go toolchain.
  71 of the 133 LLVM goldens run under Node with the output of the
  native backends; the rest (tasks, network, files, time, strconv
  quoting, more of the std runtime) fail the build with a clear reason.
- The WebAssembly backend, stage 2 (decision 0179): the Unicode code
  in Xo (decision 0180) compiles to wasm with the program, and the
  backend's runtime adds the text primitives it calls, number and
  duration parsing, and base64; tasks, scopes, channels, select, Mutex,
  Atomic, timers, and budgets run on one thread under a deterministic
  cooperative scheduler (a waiting task's wasm stack is unwound and
  rewound); `rand` uses WASI `random_get`; files are read through WASI
  (`path_open`, `fd_read`, `fd_readdir`): 116 of the 134 LLVM goldens
  pass, and the network fails the build with a clear error. A module
  carries only the runtime functions it reaches (recursion: 7.5 KB, 16.7
  KB before). Values share wasm locals by liveness, so large programs
  build: the self hosted lexer, parser, and checker run as wasm under
  Node with the output of their go backend builds (the checker: a
  40 MB module); `selfhost/wasm --why` lists every construct a program
  needs that the backend lacks.
- The self hosted driver xo1 (`selfhost/xo`, not the released `xo`)
  builds with the LLVM backend (`--backend=llvm`), runs Go backend tests
  (`xo1 test --backend=go`), and builds `use go` programs on the Go
  backend; every golden suite passes through it (decision 0178, which
  proposes the plan for retiring the Go implementation). The Go code
  generator no longer imports a module's package that the generated
  text does not name (go build failed with "imported and not used" on
  selfhost/xo).
- Native builds (arm64, LLVM, `--no-gc`): the Unicode operations of the
  runtime are written in Xo (decision 0180, the runtime pilot):
  `std/native/llrt/unicode.xo` with generated tables
  (`unicode_tables.xo`) does Str and Rune case mapping, `is_letter`,
  `is_digit`, `is_print`, strconv quoting, and std/regexp's `\pL` and
  `\p{Nd}` ranges; the C runtime keeps two bulk byte primitives. Output
  is unchanged on every backend. ASCII case mapping and quoting are
  faster than before; per rune work on arm64 is 1.45 to 1.8 times the
  C's instructions, so `XO_UNICODE=c` at build time still selects the C
  implementation (compiled only then, never in the prebuilt packs).
- More of the native runtime is Xo (decision 0193, which also orders the
  rest of the port): `std/native/llrt/text.xo` does Int.parse, a
  Duration's text and Duration.parse, a Time's RFC 3339 text and
  std/json's Time parse, and the Debug quoting of Str and Bytes;
  `strings.xo` does Str.replace and the trims. Output is unchanged on
  every backend, and the wasm backend runs the Xo versions. Int.parse,
  Duration.parse, Time text, Debug quoting, and replace retire fewer
  instructions than the C did (Time text about a quarter); Duration text
  does not yet meet the 5 percent bar, so `XO_LLRT_C=text,strings` at
  build time selects the C implementations (`XO_UNICODE=c` still works).
- Int and Float text, Float.parse, Bytes.from_hex, and base64 on the
  native backends are Xo (`std/native/llrt/num.xo`, decision 0193 step
  3), checked line by line against Go's strconv on 336,386 inputs. Float
  text retires under 1/50 of the instructions it did (the C ran snprintf
  up to 17 times a Float), Float.parse about 0.28x, Int text 0.73x to
  0.80x, base64 0.31x to 0.39x. Float.parse on the native backends now
  takes what the Go backend takes: hex floats (`0x1p-2`), underscores
  between digits (`1_000`), and any length. `XO_LLRT_C=num` at build time
  selects the C implementations.
- std/json on the native backends is Xo (`std/native/llrt/jsontext.xo`,
  decision 0193 step 4): the parser, the readers the generated decoders
  use, and the text of strings, Floats, and Bytes, checked against the Go
  backend on about 28,000 inputs. Decoding takes 0.41x to 0.96x of the
  instructions it did, encoding strings about 0.9x, structs about 0.12x,
  and Floats about 1/50 (the C ran snprintf up to 17 times a Float).
  std/json now also works on the wasm backend. `XO_LLRT_C=jsontext` at
  build time selects the C implementation.
- A Go backend build with no go command (`$XO_GO` not executable, or no
  `go` on PATH) now fails with XO1503 naming the fix, instead of a
  "code generator bug" message with no detail (decision 0176).
- `xo1 --phase-times` and `XO_PHASE_TIMES=1 xo asmdump` print the wall
  clock of each compiler phase. The self-hosted arm64 emitter's dead code
  elimination and register allocation liveness no longer copy every MIR
  instruction and bit set per pass: xo1 compiles selfhost/xo with 32
  percent fewer instructions and 45 percent fewer cycles, output byte
  identical (bench/results/xo1-perf-2026-10-12.md).
- The self hosted checker (`selfhost/check`) checks `use go` programs
  from the binding the Go toolchain cached, and reports what the Go
  checker reports for escaping scope handles and task groups (XO0802),
  skipped Go names (XO1102), `edition 2026` (XO1315), unvouched escape
  hatches of dependencies (XO1316), and effects beyond the effect lock
  (XO1317) (decision 0177). `xo check` reports XO1102 with the same text
  whether the binding was just made or read from the cache.
- Native builds (arm64, LLVM, `--no-gc`) have `Net.only` and
  `task.with_budget` (decision 0175): a `Net` carries its attenuation
  lists, and dial, listen, each HTTP request and redirect of a client
  made from it are checked (`sql.open` refuses an attenuated `Net`);
  `with_budget` bounds steps, running time, and allocation (counted
  exactly per task) through polls at loop iterations and at the entry of
  the program's functions, in programs that call it. Output is the Go
  backend's (goldens `net_only`, `task_budget`, `test_net_only`); the
  supply-chain replication's run time rows also block natively.
- Go versions (decision 0176): the Go backend needs Go 1.24 or newer
  (std/rt uses crypto/pbkdf2, maphash.Comparable, http.Protocols);
  `xo build`, `xo run`, and `xo test` read `go env GOVERSION` once per
  process and report the new XO1502 for an older go command. The
  generated go.mod and the `use go` binding workspace get
  `go <min(installed, 1.26)>` instead of a fixed `go 1.26`, so an older
  Go is never made to download a newer one. New
  `scripts/go-version-check.sh` runs the version sensitive tests with
  `GOTOOLCHAIN=goX.Y` (no download without `--download`). Every go
  command the tests run uses Xo's capped Go cache (`build.GoEnv`), held
  by the new TestGoCommandsUseXoCache; `go build` in reducediff_test and
  `go tool objdump` in internal/arm64 used the default cache.
- Native backends: an update of a collection inside a list element
  (`g.blocks[i].insts.push(x)`, `m[i][j] = v`, `rows[i].cells[j].g = v`,
  `rows[i].m[k] = v`, `rows[i].text += "x"`) is in place when the outer
  list is unique, instead of copying the inner collection every time
  (decision 0169, `list.take`): 20,000 pushes 63.6 ms to 0.1 ms on
  arm64. A shared outer list still copies (value semantics).
- The Go code generator in Xo (`selfhost/gen`, decision 0167), compared
  with internal/gen through the new hidden `xo gendump`
  (TestSelfhostGen, TestSelfhostGenMutations): every declaration and
  every whole file identical on the repository runs the Xo checker
  checks. The Xo driver `selfhost/xo` gains `--backend=go`: it compiles
  selfhost/check to the same Go as `xo build` and compiles itself to a
  fixed point. The released `xo` is unchanged.
- LLVM builds can split the IR into modules compiled by parallel clang
  processes: `XO_LLVM_SPLIT=n`, `XO_LLVM_JOBS` (default min(cores, 6)),
  `XO_LLVM_LTO=thin|none` (decision 0168). Builds without the variable
  are unchanged (one module, full LTO). The self hosted compiler's
  tests split (12 modules, no LTO): its LLVM build of selfhost/ssa
  takes 512 s instead of 2,394 s.
- The LLVM emitter in Xo (decision 0166, stages 1 to 3):
  `selfhost/llvm` lowers the self hosted SSA module to the LLVM IR
  internal/llvm prints, compared function by function and module by
  module through the new hidden `xo llvmdump` (TestSelfhostLLVM,
  TestSelfhostLLVMMutations); every module both lower is byte
  identical, and the self hosted pipeline lowers selfhost/ssa to IR
  byte identical to internal/llvm's, from which clang builds a working
  binary (TestSelfhostLLVMEndToEnd).
- Spec audit, fifth pass (bench/results/spec-audit-2026-10-11.md): core
  10.1's `xo build` row lists `--stamp`; core 14 no longer calls
  `std/time` Go backend only. New tests: `xo replay` keeps `--stamp`
  values (cmd/xo `TestRecordReplayStamp`); test_cli and test_term in
  `TestNoGCTests`; formats_toml in `TestGenGoldensNative`.
- Native builds (arm64, LLVM, `--no-gc`) run commands like the Go
  backend (decision 0172): `Proc.cmd` with `dir`, `env`, `set_env`,
  `stdin`, `inherit`, `merge_stderr`; `output`, `run`, `spawn` and every
  `Child` method; `Proc.pipeline` with pipefail; and `Proc.only` and
  `Env.only` are enforced natively (`Env.vars` and `Env.home` too).
  File handles (`open`, `create`, `open_append`), `append`, `copy`,
  `mode`/`set_mode`, `symlink`/`read_link`, `glob` with `**`, and
  `lock`/`try_lock` build natively as well, and so does `std/regexp`
  (Go's RE2 parser and Pike VM ported to Xo: the same matches, groups,
  replacements, and errors; Unicode classes other than `\pL`, `\p{Nd}`,
  `\p{Any}`, `\p{ASCII}` are an error natively for now), and
  `std/archive` (Go's compress/flate, gzip, tar, and zip ported to Xo:
  the same bytes and errors; zip64 is an error natively).
- Log attribution and resource budgets (decision 0173). A log record
  written by a dependency's code names it: `[<module>@<version>]` after
  the level in text, `"module"` in JSON (a field of that name cannot
  forge it), `log.Record.module` in tests; `xo audit` lists each
  dependency's log call sites. `task.with_budget(clock, b, f)` with
  `task.budget(cpu:, alloc:, steps:)` runs f under ceilings on running
  time, allocation, and steps and returns `Err(Exceeded(resource))` when
  one is passed; in a program that calls it, generated Go polls at every
  loop iteration and at the entry of functions that call user code
  (Go backend; native XO1201 planned; programs without a budget are
  unchanged). New fault `XO-F008`. The supply-chain replication blocks 12
  of 17 scenarios (was 10 of 15).

- xo1 (`selfhost/xo`, decision 0174) does what internal/build adds for
  native builds, with the same cache keys (shared caches): the TLS
  library of std/http, the zone data of std/time, prebuilt runtime packs
  and the runtime object cache, `use c` inputs, `--target=linux/arm64`
  (assembly), and `xo1 test`. The self compiled compiler passes all 147
  arm64 goldens; `scripts/selfhost-samebin.py` finds its binaries byte
  identical to `xo build --backend=arm64`'s (147 of 147).
  `scripts/selfhost-goldens.py` passes args.txt, env.txt, and stdin.txt.
- `List[T].with_capacity(n)` (decision 0170, every backend): an empty
  list with room for n elements before it grows.
- The `xo <edition>` line of xo.work is validated as xo.mod's `edition`
  line is (decision 0155): `xo 2026` gets the fixable note `XO1315` and
  `xo fix` rewrites it to `xo preview`; any other unknown name is
  `XO1301`.
- The arm64 emitter in Xo (`selfhost/arm64`, decision 0158), compared
  with internal/arm64 piece by piece through the new hidden `xo
  asmdump` (TestSelfhostArm64, TestSelfhostArm64Mutations): every
  function and every whole file identical on the repository runs both
  SSA builders build completely. `selfhost/xo`, a compiler driver all
  in Xo for the switch-over experiment (`scripts/selfhost-switchover.sh`,
  `scripts/selfhost-goldens.py`); the released `xo` is unchanged.
- `xo get` and `xo mod accept-effects` refuse a version with an
  escape hatch (`use c`, impure `use go`) the main module does not
  vouch for (XO1316) before writing xo.mod or xo.lock; `--accept-effects`
  accepts effects, not escape hatches (decision 0156).
- Shell completions (std.md 14.5, decision 0165): `cli.completion(cmd,
  shell)` returns a bash, zsh, or fish script that completes
  subcommands, flags, and choice values from the declaration, on every
  backend. Command names are now limited to letters, digits, `.`, `_`,
  and `-`.
- Version stamping (core 10.1, decision 0165): `xo build --stamp
  NAME=VALUE` (repeatable; also `xo run`) sets `const NAME: Str = "..."`
  of the main module for that build, on every backend, so `--version`
  can print a version set by the build script. A name that is not such
  a constant is the new error XO1501. Stamps are part of the build
  information, so `xo replay` rebuilds the same program.
- Formats (std.md 16 to 19, decision 0164): `std/csv` (`parse`,
  `parse_maps`, `format`; RFC 4180, any separator) on every backend;
  `std/regexp` (`compile`, `must`, `quote`, `is_match`, `find`,
  `find_index`, `find_all`, `captures`, `named`, `replace_all`,
  `replace_all_literal`, `split`; RE2), `std/archive` (`gzip`,
  `gunzip`, `tar`, `untar`, `zip`, `unzip` in memory; entries leaving the
  root are refused), and `std/toml` (`decode[T: Json]`, `to_json`; TOML
  1.0) on the Go backend, XO1201 (planned) on native builds. A std
  parameter's default may be a rune literal.
- `Env.config_dir()` and `cache_dir()` build natively too (native
  prelude; both SSA builders route them there).
- `std/toml` builds natively too (`std/native/toml.xo`, the same JSON
  text and errors as the Go backend).
- Files (std.md 3.1, 3.5, 8.3, decision 0163), Go backend and MemFs:
  streaming handles `os.fs.open(path)`, `create`, `open_append` (a
  `File` with `read`, `read_line`, `write`, `close`); `append`, `copy`
  (keeps permission bits), `mode` and `set_mode`, `symlink` (a sub
  capability refuses a target that escapes it) and `read_link`, `glob`
  (`*`, `?`, `[...]`, `**`), advisory locks `lock` and `try_lock` (a
  `FileLock`, `flock`/`LockFileEx`); `Env.config_dir()` and
  `cache_dir()` (XDG variables, else ~/.config, ~/Library/..., or
  %AppData%). Native builds report them as XO1201 (planned).
- Terminals (std.md 3.6, 15, decision 0162), on every backend:
  `Stdio.stdin_is_tty()`, `stdout_is_tty()`, `stderr_is_tty()`,
  `width()`, `read_secret(prompt)` (a line with the echo off, for
  passwords), `read_bytes(max)` and `read_all_bytes()` (binary input);
  `FakeStdio.set_tty(width)` and `set_input_bytes(data)` for tests; the
  new `std/term` module: `color_enabled(stdio, env)` (NO_COLOR,
  FORCE_COLOR, CLICOLOR_FORCE, TERM=dumb, a terminal), `painter(...)`
  with `bold`, `red`, ... (plain text when color is off), and `strip`.
- Golden programs of internal/llvm and internal/arm64 read `stdin.txt`
  as standard input, as internal/gen's do.
- Richer processes (std.md 3.8, decision 0161), Go backend and test
  fakes: `os.proc.cmd(name, args)` builds a `Cmd` (`dir`, `env(e)` with
  exactly the variables an `Env` shows, so `Env.only` narrows the child,
  `set_env`, `stdin`, `inherit`, `merge_stderr`) and runs it with
  `output()`, `run()` (on the terminal, returns the exit status), or
  `spawn()` (a `Child` with `write`, `close_stdin`, `read`, `read_err`,
  `read_line`, `wait`, `kill`, `pid`); `os.proc.pipeline(cmds)` pipes
  commands together (pipefail status). A child ended by a signal has
  status 128 + n. `Proc.only` applies to every way of running, and a
  `Cmd` keeps the lists of the `Proc` that made it. `FakeProc` answers
  them from `on_run`. Native builds report them as XO1201 (planned).
- `std/cli` (std.md 14, decision 0160), on every backend: command line
  parsing for tools. `cli.command(...)` with `bool_flag`, `str_flag`,
  `int_flag`, `duration_flag`, `choice_flag`, `strs_flag` (repeated),
  `arg`, `rest`, and subcommands; `cli.parse(cmd, args, env)` returns a
  `Parsed` (typed accessors `bool`, `str`, `int`, `duration`, `strs`,
  `_opt` forms, `arg`, `rest`, `command()`) or a `CliErr` (`Help`,
  `Version`, `Usage`); `cli.parse_or_exit(cmd, os.env, os.stdio,
  os.proc)` prints help or version (exit 0) or a usage error (exit 2).
  Flags fall back to variables (`env:`), read through the `Env`.
- Golden programs may set extra environment variables in `env.txt`
  (internal/gen, internal/llvm, internal/arm64); the arm64 golden runner
  passes `args.txt` too.
- Logging is the effect `log` (decision 0156): `std/log`'s `debug`,
  `info`, `warn`, `error` perform `log`, so a `pub` function that logs
  declares `uses log` (inferred for non-pub functions; `log.configure`
  still `stdio`). std's own records stay ambient within std. XO0701 for
  a missing `log` carries a fix, and `xo fix` added it across the repo's
  programs. The effect label list in diagnostics and ceilings includes
  `log`.
- `xo audit [--json] [--no-gc] [<dir>]` (decision 0156): a supply chain
  report per dependency (effects, xo.lock effects state, ceiling, `use c`
  and impure `use go` with their vouches) plus this module's capability
  attenuations, the capabilities it passes into dependencies (attenuated
  or not), and unused vouches.
- Capability attenuation (decision 0156): `os.net.only(hosts)` (exact
  hosts, `*.domain`, `host:port`; checked for raw dials and listens, each
  HTTP request and redirect of a client made from it, and refused for
  `sql.open`), `os.env.only(names)`, and `os.proc.only(cmds)`, on the Go
  backend and its test fakes. `NetErr.Denied(addr)` and
  `ProcErr.Denied(cmd)` are new variants (an exhaustive `match` needs
  the arm). Native builds report `only` as XO1201 (planned).
- Supply chain (decision 0156): an effect lock by default. xo.lock
  records each dependency's effects (`<path> effects <labels|none>`, the
  union of its pub functions' `uses`). `xo get` and `xo mod tidy` record
  them the first time and narrow them; an update that widens them fails
  `xo get`, `xo mod tidy`, and every load with `XO1317` until `xo mod
  accept-effects [<path>...]` or `xo get --accept-effects` records them.
- Supply chain (decision 0156): only the consuming module vouches for a
  dependency's escape hatches. A required module's `use c` lines and its
  `use go` of Go packages that are not pure need a `vouch <module>
  <version> c "<header>" [uses ...]` or `vouch <module> <version> go
  "<package>"` line in the consumer's xo.mod, else `XO1316`; a
  dependency's own `use c ... uses none` no longer counts. A
  dependency's `use c` binding is now written to the main module's
  binding cache (it failed on the read only module cache before).
- Prebuilt runtime (decision 0157): `scripts/release.sh` builds release
  archives (darwin/arm64, linux/arm64, linux/amd64) with the arm64
  backend's C runtime prebuilt in `lib/xo/rtpack/<os>-<arch>/` (every
  variant of std/llrt and the TLS library, byte for byte reproducible;
  linux/arm64 cross compiled with zig cc for glibc 2.31). A native build
  links the pack's object when its key (the runtime sources and flags)
  and checksum match, else compiles the runtime locally as before;
  `XO_RTPACK=<dir>|off` chooses, and the hidden `xo rtpack` builds or
  inspects packs. The first arm64 build of a small program drops from
  1.94 s to 0.13 s (an HTTPS server 4.27 s to 0.28 s). `cc` still
  assembles and links (decision 0150's "no clang" corrected). The zone
  data of std/time is now `xo_tzdata.c`, compiled beside the runtime.
  A failed local runtime compile, and a missing `cc`, name what to
  install.
- Editions (decision 0155): until 1.0 the only edition is `preview`;
  `xo init` writes `edition preview`, and every xo.mod and xo.work of
  the repository says it. `edition 2026` stays an alias of `preview`
  during the 0.x releases: `xo check` gives the fixable note `XO1315` on
  it, and `xo fix` (also `xo fix --edition=preview`) rewrites xo.mod.
  The compatibility promise of core 13 starts with the first frozen
  edition, named at 1.0.
- arm64 backend: enum parameters of 256 bytes or more of direct calls
  are passed by address too (decision 0153); tags and payloads of enums
  in memory are read in place, and large fields and payloads of read
  only memory are passed on by address (the arm64 build of the self
  hosted SSA builder 5 to 9% faster).
- Self hosting: SSA stage 8 (decision 0150): selfhost/ssa runs the
  passes after the build (pruneUnwind, addLandings, cleanup, InsertRC)
  and the verifier exactly as internal/ssa does; the SSA lowering is
  self hosted. `xo ssadump --stage=verify,mutants` prints the
  verifier's verdicts.
- Self hosting: SSA stage 7 (decision 0150): selfhost/ssa builds the
  test binaries of `xo test --no-gc` exactly as internal/ssa does;
  `xo ssadump --test` prints them.
- Self hosting: SSA stage 6 (decision 0150): selfhost/ssa lowers the
  generic intrinsics (JSON codecs, SQL rows, any, cast, status codes)
  and calls of `use c` bindings exactly as internal/ssa does; the Xo
  checker binds `use c` from the binding cache (decision 0152).
- `use c`: two `use c` lines of one header, one with `uses none` and
  one without, no longer share one binding in a process (the second
  line had the first one's effects).
- Self hosting: SSA stage 5 (decision 0150): selfhost/ssa lowers
  scopes, tasks, `await`, channels, `select`, Mutex, Atomic, std/task,
  os.tasks, and the operations that wait exactly as internal/ssa does
  (0 differences, three backends, 1,900 mutations).
- Self hosting: the go build of the Xo SSA builder is 7 to 9% faster
  (`comparable` copied its list of types per recursive call).
- arm64 backend: struct parameters of 256 bytes or more of direct calls
  are passed by address (decision 0151); `var` ones stay in the
  caller's memory, read and updated in place (decision 0148's
  dataflow), non `var` ones are read in place. The arm64 build of the
  self hosted SSA builder is 5.8 times faster (13.2x to 2.3x the Go
  builder) and half the size.
- arm64 backend: register allocation merges live ranges in one pass
  instead of sorting at every assignment (code generation of
  selfhost/ssa 100 s to 60 s).
- Self hosting: SSA stage 2 (decision 0150): selfhost/ssa lowers
  interface values and calls, Error/Display/Debug values, method values,
  `derive Ord` comparisons, and the generated equality and order helpers
  exactly as internal/ssa does (0 differences, three backends).
- Self hosting: SSA stage 3 (decision 0150): selfhost/ssa lowers Map
  and Set (helpers, methods, literals, `m[k] = v`, loops, Debug text)
  and constants of collection types exactly as internal/ssa does.
- Self hosting: SSA stage 4 (decision 0150): selfhost/ssa lowers
  contracts (`requires`, `ensures`, invariants), `defer` with its
  landings and fault continuations, loop bounds, and real-time sections
  exactly as internal/ssa does; stages 1 to 4 of the SSA builder are
  done.
- arm64 backend: a function past 1 MiB of code failed to assemble
  (`b.hs` to a fault stub out of range); its conditional branches now
  go through an unconditional `b` as tbz already did.
- arm64 backend: an overflow fault of `c + x` (constant left) named the
  operands swapped (`6 + 9223372036854775807`); it follows the source
  order, as on the Go and LLVM backends. Found by `progen.Arith`, now
  widened to Float, Str, List[Int], structs, enums with payloads,
  closures, and edge operands (shifts by 63, the minimum divided by -1).
- Go backend: a float `+`, `-`, or `*` of two literals whose result is
  out of range (`2.5e300 * 1e10`) failed to build (a Go constant
  overflow); it is `+Inf` at run time, as on the native backends.
- Tests: TestArithProperty and TestCompileProperty compare a program's
  exit status, all of its standard output, and all of its standard
  error across backends (was the first line of standard error),
  dropping only what native builds legitimately leave out (frames, the
  deadlock's blocked task list, crash bundle paths). bench/nightly.sh
  runs TestArithProperty on 500 programs.
- scripts/reduce-diff.py shrinks a program while two backends run it
  differently (ddmin over lines, then smaller edits; about 40 s for a
  74 line program); TestArithProperty prints the command for a failing
  program.
- Generated Go no longer repeats a `//line` directive on consecutive
  lines (positions are unchanged), declares the unused task `tk` in
  constants whose value does not need it, or ends a function whose body
  already terminates (an `if`/`else` or `switch` returning in every
  branch, by the Go spec's rule) with an unreachable panic. `match`
  emits no `break` after an arm that returns or jumps, declares its
  label only when an arm breaks to it, emits a catch-all arm as a plain
  block (no `Unreachable` after it), and matches an immutable binding in
  place instead of copying it; a `scope` body that returns skips its
  dead epilogue. The generated Go of every golden program now passes
  `go vet` (it had 108 unreachable code reports) and `gofmt -l`
  (TestGeneratedGoVets), and 11 programs keep a snapshot of their Go
  (TestGeneratedGoSnapshot).
- Generated Go: an exhaustive `match` ends in
  `panic(rt.Unreachable(loc))` (Go needs no trailing panic after it),
  statements after one that never completes are not emitted, `x = x` is
  dropped, and the `_ = x` lines Go does not need are removed (191 to 15
  in the snapshot programs; the last ones, `_ = osv` in XoMain and the
  unused trace of a scope's failure, followed on 2026-10-11: the Os
  parameter is `_` when main does not take it, and the trace is `_`
  when the failure becomes a fault). go vet is clean on the examples, the LLVM
  golden programs, the perf benchmarks, and the self hosted compiler too
  (TestGeneratedGoVetsLarge).
- arm64 backend: a conditional branch whose two targets are the same
  block kept its compare out of the output, while the next block reused
  those flags; it branched on stale flags (wrong `if` results after an
  `if` with identical arms). Found by `progen.Arith`, random integer
  programs compared across the Go, LLVM, and arm64 backends
  (TestArithProperty).
- Self hosting (decision 0150): the order of the remaining stages is
  SSA lowering, arm64 emitter, LLVM emitter, Go code generator, and the
  self hosted compiler takes over native builds on arm64 hosts once it
  builds itself to a fixed point there. `selfhost/ssa`, the SSA builder
  in Xo, lowers the core subset (functions, generics, numbers, Str,
  structs, enums, patterns, loops, calls, closures, Option and Result
  with `?` and error frames, Display and Debug text) exactly as
  internal/ssa does: 0 differences on every function it completes over
  the repository and 400 mutations, on the go, arm64, and LLVM builds
  (53% of the Go builder's functions; maps, interfaces, defer, and
  concurrency are later stages). New hidden `xo ssadump [--stage=build,
  unwind,landings,cleanup,rc]`; `xo typedump --native`; the self hosted
  checker gains native mode.
- arm64 backend: the address of a frame slot past 4095 bytes is made
  with a shifted add (the assembler rejected `add x6, sp, #4808`).
- Go backend: `==` on an enum whose variants have no fields compares
  tags instead of boxing both values.
- Spec 5.2: a receiver that is not `var` is evaluated before the
  arguments, like any operand (the behavior of every backend).

- LLVM backend: enums of 256 bytes or more are passed by reference, and
  values in memory nothing writes during the call (by reference
  parameters, their fields and payloads, box contents and list elements
  under a borrowed parameter) are read in place and passed by address
  (decision 0147); `var` parameters passed by reference stay in the
  caller's memory, with fields read and updated there (decision 0148).
  The self hosted checker's LLVM build went from 1.88 times the Go
  checker's time to parity (5.4 s to 2.9 s for 20 checks of selfhost).
- Native backends: an enum parameter is borrowed unless every path
  consumes it, so tree walks pass nodes down without reference count
  traffic (decision 0149).
- The self hosted checker loads modules as internal/check does (decision
  0141): xo.mod, xo.work, xo.lock, the module cache (read, never
  fetched), vendor/, deny rules (XO1312), dependency rules (XO1301 to
  XO1307), and effect ceilings of dependencies (XO1309). Every
  repository run without `use go` or `use c` (610 of 626) is compared
  and identical on the go, arm64, and LLVM builds, and module mutations
  (ceilings, deny rules, versions, lock lines) give no difference. New
  Go goldens for XO1309: `effect_ceiling_work` (xo.work ceilings) and
  `effect_ceiling_vendor` (require ceilings, vendored).
- `cmd/xoplay`, the compiler half of the xolang.dev playground (decision
  0144): `check`, `fmt`, and `gen` as JSON, built for wasip1 with
  `-tags xo_playground`, which leaves the runtime, native, and LLVM runtime
  sources out of the binary. `build.Generate` is the code generation step
  of `xo build` without a build directory or the Go toolchain.
  `xoplay gen --test` generates test binaries, with `xo_nohttp` when they do
  not reach std/http (`gen.ReachesStd`). The playground underlines
  diagnostics, sets `--seed` and `--explore` for tests, downloads brotli
  compressed assets (22.5 MB instead of 33), and is tested in CI before
  every deploy of the site.
- LLVM backend: struct parameters of 256 bytes or more are passed by
  reference in direct calls, `var` ones written back through the
  pointer, instead of field by field (decision 0145); the self hosted
  checker's LLVM build is about 15% faster.
- The self hosted checker reports naming (XO0101, with the rename fix
  that covers every use of a local), time layouts (XO0420, and the
  XO0177 habit of a date pattern from another language), and the
  checker's other XO0177 habits as internal/check does (stage 8 of
  decision 0141): 587 of 609 repository runs are compared whole and
  identical on the go, arm64, and LLVM builds; the 22 left use `use go`
  or `use c` (16) or break dependency rules the port does not have (6).
- The self hosted checker gives every diagnostic the notes and machine
  fixes of internal/check and internal/parser (stage 7 of decision
  0141): edit distance suggestions, habit notes and fixes, effect call
  chains, previous declarations, missing arms and fields as holes;
  `xo typedump` has a `full` section (the `xo check --json` form), and
  every compared run and mutation gives identical JSON on the go, arm64,
  and LLVM builds. `xo astdump` prints notes and fixes too.
- Notes and fixes attached after more diagnostics were reported are no
  longer lost (`xo check --json`, `xo fix`).
- The self hosted parser resynchronizes after a hard error as the Go
  parser does and reports the same errors with the same recovered tree
  (every file and token mutation compared); the self hosted checker now
  checks runs with parse errors too. `xo typedump` no longer panics on a
  use of an unresolved import.
- arm64 backend: spilled values share stack slots when their live
  ranges do not overlap (frames of long functions shrink up to 8 times;
  the self hosted checker's largest went from 28 KiB to 6.4 KiB), and a
  frame past 28 KiB no longer fails to build: such a function addresses
  its far slots through x28 (decision 0146).
- The self hosted checker checks matches for exhaustiveness (stage 6 of
  decision 0141): XO0301 with the same missing patterns, refutable `let`
  and `for` patterns (XO0303), and the XO0305 warning, as internal/check
  does on every repository module (go, arm64, and LLVM backends).
- The self hosted checker applies the value semantics rules (stage 5 of
  decision 0141): `var` places (XO0501, XO0502), overlapping `var`
  arguments (XO0503), moves (XO0510), lost and stored updates (XO0520),
  split critical sections (XO0804), and unused bindings (XO0110,
  XO0201), with the same diagnostics as internal/check on every
  repository module (go, arm64, and LLVM backends).
- The self hosted parser reports the habits of other languages
  (decision 0087) and the other soft errors as internal/parser does,
  with the same messages and spans, and recovers the same tree, so the
  self hosted checker now checks programs written with Go, Rust,
  Python, TypeScript, Kotlin, or Swift habits and gives the same
  diagnostics (the 107 habit fixtures of internal/fix among them).
- The self hosted checker checks generics, effects, and real-time
  functions (`selfhost/check`, stages 3 and 4 of decision 0141): it
  prints the sigs section of `xo typedump` (type parameters, constraints,
  implicit parameters, declared and inferred effects) and reports XO0402,
  comparability (XO0404), XO0701, and XO1401 to XO1405 as internal/check
  does, on every repository module (go, arm64, and LLVM backends). Effect
  ceilings of dependencies (XO1309) are not ported yet.
- The self hosted checker types every expression (`selfhost/check`,
  stage 2 of decision 0141): inference, literals, operators, expected
  types and implicit conversions, calls with named arguments, fields,
  methods, statics, variants, closures, and the Option and Result rules,
  with the same types as internal/check on every repository module it
  accepts (go, arm64, and LLVM backends) and the same diagnostics on
  modules rejected only for names and types. The names section of `xo
  typedump` now also names the fields, methods, statics, variants, and
  parameters (of named arguments) that identifiers resolve to.
- A call reads a `var` receiver or `var` argument after computing its
  arguments (spec 5.2, decision 0142). On the native backends
  `t.get(t.add(7))` ran `get` without the item `add` pushed, and
  `xs.push(f())` dropped what `f` pushed onto `xs`.
- `NAMES[i]`, a constant indexed by a lowercase name, is an index
  (decision 0143); it parsed as type arguments with an effect argument
  and was XO0407.
- The type checker's name resolution written in Xo (`selfhost/check`,
  decision 0141), the first stage of the self hosted checker: the same
  resolution of every identifier as internal/check on every repository
  module the Go checker accepts (on the go, arm64, and LLVM backends),
  and the same diagnostics on modules rejected only for names. Hidden
  `xo typedump [--section=names,types,sigs,diags] [--each] <dirs>`
  prints what the Go checker resolved, a section at a time
  (bench/results/selfhost-checker-2026-10-10.md).
- An assignment computes its value before it reads the binding it
  stores into (spec 5.2, decision 0140). On the native backends
  `t.objs[o].kids = t.add_kids()` dropped what `add_kids` pushed onto
  `t.objs`.
- Native backends: two modules may declare types of the same name; they
  were one type (a field of one was "not available without a garbage
  collector", or the wrong layout was used).
- Go backend: `??` on a nested optional (`opt.ok() ?? None` where `opt`
  is a `T? ! E`) generated Go that did not compile.
- `XO0401` on an `if` without `else` where a value is expected now has
  a note naming the missing `else`; when the expected type came from a
  sibling branch (a closure ending with `if a { n.add(1) } else if b
  { ... }`), a fix discards the branch values with `let _ =`.
- New error `XO0178` (decision 0136): a line that starts with a binary
  operator (`|| b`, `&& c`, `+ d` continuing the line above) gets one
  diagnostic and a fix that moves the operator to the end of the
  previous line. Before, a leading `||` was taken for a Rust closure
  (a wrong XO0177 fix) and cascaded into XO0160 and XO0175.
- `for b in data` over `Bytes` (spec 3.5, decision 0138) yields each
  byte as a `UInt8`; it was `XO0406`. Go's `for _, b := range data`
  gets the fix `for b in data`.
- `while let` (spec 3.5, decision 0137): `while let Some(v) = q.pop() { }`
  loops while a refutable pattern matches; `while let v = opt` is the
  Option shorthand, as for `if let`. Labels, `continue`, and `bound N`
  work; every backend and the self hosted parser.
- Import paths with a `.` or `..` segment (`use example.com/app/./lib`,
  `use ./lib`) are `XO0158`. They were accepted, and since `deny` rules
  compared the path as written while the loader cleaned it, they got
  past `deny` (decision 0095). The loader and `deny` now both use the
  cleaned path, and `XO0158` is the only error such an import gets
  besides a `deny` match.
- Spec: the grammar defines `PathSeg` (ASCII letters, digits, `_`, `-`,
  `.`), matching the lexer; section 4.1 states that dashes outside the
  last segment need nothing (`use github.com/xo-lang/xo/lexer`). No
  behavior change.
- Real-time functions (spec 6.5, decision 0133): `uses realtime` on a
  function makes the checker reject allocation (XO1401), loops without a
  bound (XO1402), calls of functions that are not real-time, function
  values, and interface methods (XO1403), recursion (XO1404), and
  blocking (XO1405). Native builds defer the frees of a real-time
  section to its end (a 256 entry queue per thread, `XO_RTSTATS=1`
  counts) and fault with XO-F007 when a section allocated. Demo
  `examples/realtime-control`: the tick that drops a 20,000 point
  schedule writes its output in about 0.2 us instead of 30 to 114 us
  (bench/results/realtime-2026-10-10.md).
- Loop bounds: `for x in xs bound N { }` and `while c bound N { }` on
  every backend; a constant range longer than N is XO1402, otherwise
  iteration N + 1 faults with XO-F006, and `xo test` fuzzes functions
  with bounded loops.

- Native TLS: ECDSA P-256 signing and verification run on fiat-crypto's
  verified field arithmetic (vendored at a pinned commit,
  `third_party/fiat-crypto`, decision 0134) instead of Mbed TLS's bignum
  code: signatures byte identical, 4.8 times faster (23 us), a full
  HTTPS handshake on the native server takes 251 us of CPU (was 354; Go
  backend 227).
- `bench/perf/http/run.sh` takes `CONFIGS=xo-base-llvm` with
  `XO_BASE=<xo binary>` for before and after runs of the LLVM backend.
- `xo build --hybrid`: loops in Go assembler poll for preemption at every
  back edge (decision 0131), so a long hybrid loop no longer holds up the
  GC's stop the world or other goroutines on its P (11.6 s to 0.058 ms
  during an 11.5 s loop); the hybrid benchmarks are 0 to 7 percent slower
  than before (median 2), still 9 to 31 percent faster than the Go
  backend.
- `xo build --hybrid` compiles loops of programs that start tasks (was
  "no assembly": an RC verifier error on the task runner).
- `bench/perf/hybrid/run.sh` takes `XO_BASE=<xo binary>` to compare two
  compilers' hybrid builds in the same interleaved runs.
- The parser written in Xo (`selfhost/parser`, decision 0127): the whole
  grammar, built with the Go, arm64, and LLVM backends; for every file the
  Go parser accepts it prints the same syntax tree dump, and it rejects
  every file the Go parser rejects (repository, std, spec blocks,
  generated programs, token mutations). Its LLVM build parses 1.3x faster
  than internal/parser (bench/results/selfhost-parser-2026-10-10.md).
- Hidden `xo astdump [--sig] [--bench=N]`: the canonical syntax tree dump.
- arm64: functions with frames up to 28 KiB build (probed a page at a
  time; was a "frame too large" error past 4000 bytes), and values of
  80 or more words no longer fail to assemble (decision 0128).
- The program generator of the compile property test is the package
  internal/progen.
- Native test binaries now report test_once, test_logs, and
  test_shutdown byte for byte as the Go backend (`TestNoGCTests` runs
  them): a fault in a `task.once` function is stored and raised again
  in every caller; `log.configure(os.stdio, ...)` in a test writes
  records to the fake standard error (still kept for `testing.logs()`);
  `os.proc.set_grace` before `testing.run_main` sets that run's grace
  period, and a third signal makes `run_main` return 130 (with
  `exit_code()` `Some(130)`) instead of ending the test binary.
- `==` on `http.Request` works on the native backends (was XO1201): it
  compares the six spec fields and ignores typed request values, as on
  the Go backend (decision 0112 note; internal/llvm golden `request_eq`).
- internal/llvm `TestGolden` runs `fault_defer_chain` and `request_eq`
  under `-short` too.
- Tests for items that had none: native TLS session resumption
  (`TestTLSDiff`, TLS 1.3 and 1.2), decision 0118's reader handoff,
  scope pruning, and magazines (std/llrt/testdata/tasks.c), and decision
  0098's binary trimming (internal/build `TestGoBinaryTrimming`).

## v0.9.0 (2026-10-10)

Spec draft 0.9.

- Module proxies (decision 0126): `XO_PROXY` lists proxy URLs, `direct`
  (git), and `off`, with Go's fallback rules (comma after 404/410, pipe
  after any error; default `direct`), and `XO_NOPROXY` patterns go to
  git. The read only protocol serves `@v/list`, `.mod`, `.zip`, and
  `.lock` per module; `xo mod proxy serve [--fetch] [<cache dir>]`
  serves a module cache and can fill it from git, as a team cache.
  Proxied trees are checked against xo.lock exactly as git fetches are.
- Checksum log (decision 0124): with `XO_SUMDB=<name>+<id>+<key>
  [<url>]`, the first fetch of every module version must match the
  log's record, be in its signed Merkle tree, and extend every tree the
  module cache saw before; otherwise `XO1313` and nothing is cached. An
  unreachable log or malformed settings is `XO1314`; `XO_NOSUMDB` keeps
  private modules out. The default stays `off` (no public log yet).
- Definition hashes (decision 0125, the last step of 0086): `xo query
  hash <name>` prints a content hash per top level definition that
  survives renaming locals and changes with what the definition, or one
  it calls in the module, computes. `xo publish --check` lists the
  definitions (private ones too) changed since the last tag, including
  those changed only through a helper, next to the API comparison.

- std/http speaks HTTP/2 over TLS on every backend (decision 0121):
  `with_tls` servers and `https` clients agree on `h2` by ALPN, plain
  `http` stays HTTP/1.1, and nothing in the API changes. Requests of a
  connection run at once, each as a task; a fault in a streamed body
  resets only its stream. The native backends implement it in C (frames,
  HPACK, flow control) with net/http's limits (250 streams, the header
  list limit with 431, the rapid reset limit) and its error texts, and
  their client keeps HTTP/2 connections for later requests as Go's does.
  Over one connection the native server does 87.8k requests per second
  in the HTTP benchmark, the Go backend 58.2k
  (`bench/results/http2-2026-10-10.md`).
- Native test binaries (`xo test --no-gc`, decision 0120): property tests
  and `testing.gen` work, with the Go backend's generated values for a
  seed, the same failing case, and the same shrunk input in the report.
  A `Set` or `Map` keyed by a sized integer (`Set[Int8]`) now builds
  natively. `xo test --real` gives native tests real capabilities for
  the effects they declare (`fs`, `fs.read`, `fs.write`, `net`, `clock`,
  `env`, `stdio`, `proc`), as on the Go backend.
- Native backends: deferred code that faults no longer skips the rest
  of the cleanup. The deferred code registered before it and that of
  the blocks around it runs, the last fault is reported, as on the Go
  backend (core 3.5, decision 0085 amendment), and a task that caught
  such a fault with `task.isolate` sees cancellation again (it could
  deadlock before).
- LLVM backend (decision 0119): generated code is compiled for the same
  CPU and features as the C runtime (clang's defaults): LSE atomics for
  shared reference counts on darwin/arm64, outline atomics (LSE when the
  CPU has it) on Linux arm64. HTTP benchmark +3.5 percent.
- Native `Mutex` (decision 0118): a writer's release hands the lock to
  the readers waiting, so they no longer wake only to park again: a
  contended readers and writer load (`bench/perf/conc` `rwmutex`, new)
  went from 967 to 131 ms and from 6.3 to 0.25 s of CPU. Long lived
  scopes (the native HTTP server's connections) no longer keep every
  finished task until they close, and the native allocator moves freed
  blocks between threads, so a server's memory stays flat (28 MB instead
  of 295 MB after 300k TLS connections).
- Native HTTPS (decision 0117): full TLS handshakes about 1.6 times as
  fast and at half the CPU (9.7k to 15k per second in the HTTP benchmark,
  Go backend 17k): X25519 key shares through HACL*, faster P-256
  inverses, a spinning lock for Mbed TLS's key store, SHA instructions.
  The native server now sends session tickets as Go's does (TLS 1.3
  resumption with ECDHE, TLS 1.2 tickets; keys rotate daily), so
  returning clients skip the certificate signature. `bench/perf/http`'s
  loadgen has `-resume` (`RESUME=1` in run.sh).
- `xo lsp` (decision 0114): a Language Server Protocol server with
  diagnostics and their fixes as quick fixes, hover (signature, doc, or
  type), definition (into std too), references, rename, document
  symbols, semantic tokens, and formatting. The VS Code extension starts
  it; ```xo fences in Markdown are highlighted.
- VS Code syntax highlighting (decision 0113): `editors/vscode/` registers
  `.xo` files with a TextMate grammar covering spec section 1 (keywords,
  contextual words, interpolation, raw and bytes strings, durations, sets,
  effects in `uses`). Install by symlinking it into `~/.vscode/extensions`.
- `use c "<header>" [link "<lib>"]... [as name] [uses ...]` on the
  native backends (decisions 0093 stage 2 and 0116): bindings generated
  from the header with clang (`xo bind --c`), scalars, Str, Bytes with a
  length, flat structs, and opaque handles at the boundary, the `ffi`
  effect unless the line narrows it, XO1104/XO1105 for headers and names
  that do not bind, XO1201 on the Go backend. Calls are not recorded
  yet (native builds do not record).
- Decision 0111: passing one variable, or overlapping places (`x` and
  `x.f`, `xs[i]` and `xs[j]` unless both indexes are different
  literals, a `var` receiver and an argument naming it), to two `var`
  parameters of one call is `XO0503` on every backend; the native
  backends no longer report XO1201 for it.
- Decision 0112: a value that holds a handle (`Mutex`, `Atomic`, `Chan`,
  `Task`, `Scope`, `Listener`, `Conn`, `task.Group`, the std/http and
  std/sql handles, the test fakes) or a function value is not
  comparable: `==`/`!=`, Map keys, Set elements, `derive Eq`/`Hash`, and
  `T: Eq`/`Hash` arguments are `XO0404` with a note naming the field
  that holds it; `derive Ord` already was. The checker now looks into
  struct fields and enum payloads for `Eq` (it looked only at type
  arguments).
- Native backends (decisions 0107 to 0109): `Proc.run`, `os.tasks` (a
  scope around main and around each test), method values
  (`let f = x.method`), `map_concurrent_until`, base64,
  `Range.to_list`, `Rand.shuffle`, `target` (a compile time constant),
  and in native test binaries MemFs (every method, `fail_next`, case
  sensitivity, `files`), FakeProc `run` and `on_run`, `os.proc.exit`
  inside `testing.run_main`, and `run_main` of a main that cannot fail.
  Fixed natively: a receive arm after an `after` arm lost its value; a
  `?` in a closure printed the function's frame name without
  `.closure`, and one in a `Mutex.with` closure none;
  `FakeClock.advance` returned before the tasks it woke ran; the
  success type of `Err(e)?` was XO1201. One binding passed to two `var`
  parameters is now XO1201 natively (the Go backend shares it, the
  native backends copied it).
- TestCompileProperty (decision 0110): random checker valid programs over
  the optional syntax must build on the Go backend and with `--no-gc`,
  and run alike.
- Fixes from Codex r3: a `select` arm that receives without binding the
  value no longer generates Go that fails to compile; the "propagate with
  ?" fix is offered only where `?` can propagate; help notes for XO0205
  in closures and for passing `TestOs` where `Os` is expected.
- Native backends (`--no-gc`, LLVM and arm64): interface types as values
  (decision 0100): dynamic dispatch, adapters for methods that cannot
  fail, error conversion, interface to interface conversion, Display and
  Debug of the dynamic value, and `==`; `x.debug()` on any value.
- Native backends: newtypes (`Email("x")`, `e.0` read and assigned,
  `Email(p)` patterns, Debug, `==`, Map keys) and list patterns
  (`[]`, `[a, .., z]`). Fixed: an or pattern whose alternatives bind the
  same name (`Leaf(v) | Node(left: v, ..)`) read the first alternative's
  binding natively, a stale value when another alternative matched.
- Native backends (decision 0101): Float32, `Secret[T]` redacted in every
  output path, `Buf[T]`, `<` on `derive Ord` types, `Int32.try` and the
  other sized `try`s, `Int.pow`, `clamp`, `min`/`max` on Str, Set union,
  intersect, difference, `Map.filter`, `List.insert`, `remove_at`,
  `indexed()`, `xs + ys`, std defaults (`crypto.token(r)`,
  `Chan.new()`), `get_or_fault` on any key type, builtin collections
  through interfaces, shorthand field patterns. `==` on a value holding
  a function or runtime handle outside an interface is now XO1201
  natively (it failed in code generation). New test
  TestGenGoldensNative runs 66 internal/gen goldens on Go, LLVM, and
  arm64 and compares them.
- Native backends: `std/path` (std/native/path.xo), `task.semaphore`,
  `task.once` (a fault in its function is not stored natively: the next
  caller runs it again), and `task.map_concurrent`
  (std/native/task.xo); Debug text of `Result[T, Never]` values. Fixed:
  natively, a task whose value is a Result failed its scope when that
  value was an `Err`; shorthand struct patterns `Bag{items}`.
- Native backends (decision 0102): every Fs method and attenuation
  (`sub`, `cwd`, `read_only`, `write_only`, escapes `Denied`),
  `Stdio.write`, and standard input reads (`read_line`, `read_all`,
  `lines`) that a cancellation or a stop request ends with
  `IoErr.Canceled`. Fixed natively: `task.with_timeout` returns
  `TimedOut` when its closure finishes after the deadline without a
  cancellation point, as on the Go backend.
- Native backends: error frames (decision 0104): each `?` records the
  function, location, and parameters lazily, and main's error report
  prints `  at ...` lines identical to the Go backend's.
- Native Fs and standard input code is compiled into a program only when
  it calls them (XO_FS, XO_STDIN; hello world unchanged). Fixed: hybrid
  builds (`--hybrid`) declared a Bool result's argument size rounded up
  (go vet -asmdecl).
- `xo test --no-gc`: `FakeStdio.set_input` feeds the reads; fixed a hang
  after `FakeProc.signal_after` fired (the virtual clock stopped moving).
- Native backends: all of `std/time` (decision 0094; std/native/time.xo
  and std/llrt/xo_time.c, a port of Go's TZif reader and zone lookup):
  zones with the same per use embedded data (or `--tzdata=system`), civil
  time, gaps and overlaps, layouts, strict parsing with the same error
  texts and faults; also `Duration.from_secs`, `scale`, `ceil_seconds`,
  `Time.parse_rfc3339`, and `Time.UNIX_EPOCH` as a value. Programs that
  do not use std/time do not grow. TestTimeDiff compares about 70,000
  lines with the Go backend. Fixed: the arm64 backend failed to assemble
  functions over 8192 instructions (`tbz` range).
- HTTPS in std/http on every backend (std.md 4.6, decisions 0105 and
  0106): `http.tls_cert(cert_pem, key_pem)`, `Server.with_tls(certs)`
  (SNI picks among the certificates), `Client.with_roots(pem)`, and
  `TlsErr`. The native backends (`--no-gc`) speak TLS 1.2 and 1.3 through
  Mbed TLS 4.1.1 (vendored in `third_party/mbedtls`, built once per
  compiler into the user cache, linked only into programs that use
  std/http), verify against the system's roots on macOS and Linux, and
  report Go's error texts; an `https` URL is no longer `XO1201` there.
  std/http speaks HTTP/1.1 only, also over TLS: the Go backend's client no
  longer negotiates HTTP/2. Failed TLS handshakes are not logged.
- `bench/perf/http/run.sh`: `SCHEME=https` serves the Xo configs over TLS
  and `NEWCONN=1` opens a connection (and a handshake) per request.

## v0.8.0 (2026-10-09)

Spec draft 0.8.

- Modules (decision 0086 steps 1 and 2): `require <path> <version>` of Xo
  modules, minimal version selection, git fetch into a read only module
  cache, `xo.lock` hashes verified on every build, `xo get`, `xo mod
  tidy`, `xo vendor`, `xo mod clean`, `--offline`, and `xo.work`
  workspaces (`--workspace=off`). Diagnostics XO1301 to XO1308.
- Codex r2 first run failures: `XO0175` for several match arms on one
  line has a split fix and no longer cascades into a false `XO0301`;
  `.display()` works on every `Display` type (derived, std errors,
  builtins); a closure that never completes infers `Never` instead of
  `XO0402`.
- Effect ceilings and checked semver (decision 0086 steps 3 and 4):
  `require <path> <version> uses none` in xo.mod and `use <dir> uses ...`
  in xo.work (XO1309 at a call above the ceiling), API and effect diffs
  from `xo get` (and `xo get -u`), `xo publish [--check]` against the
  previous tag (XO1310, XO1311; prints the tag command, never pushes),
  `xo init`, `/vN` major version module paths, and `xo serve` noticing a
  new or changed xo.work above the directories.
- `std/time` (decision 0094, Go backend): zones with per use embedded
  data (`--tzdata=system`), civil dates and times with explicit daylight
  saving gaps and overlaps, layouts with named fields checked at compile
  time (XO0420; XO0177 for `{yyyy}`, `%Y`, `2006-01-02` habits).
- Import direction rules (decision 0095): `deny <pkg> -> <pkg>, ...`
  lines in xo.mod with `/...` patterns, checked at every `use` line
  (new code XO1312; a rule naming no package is XO1301), kept by `xo mod
  tidy`; `xo query imports` and `xo query deps` list the `use` lines
  (also in the MCP `query` tool).
- std/time error text (std.md 13): `ZoneErr` and `CivilErr` display as
  sentences (`unknown time zone "Mars/Olympus"`, `2026-03-08T02:30:00
  America/New_York does not exist (skipped by a daylight saving
  change)`) instead of the derived `Skipped(at: ...)` form, matching
  `ParseErr` and `LayoutErr`.
- Newtypes (decision 0097, spec audit gap 4): `e.0` reads the underlying
  value and `Email(s)` destructures it in `let` and `match`.
- `xo check --fix` (decision 0096): applies the unambiguous machine fixes
  (as `xo fix`), reformats, then reports what remains plus an `applied`
  list; the MCP `check` tool takes `fix`, and the bench harness agent
  loop uses it while still scoring first runs as written and after fix.
- Go backend output (decision 0098): hello world 7.9 to 3.6 MB, Go build
  cache growth per program 34 to 2.9 MB, compile memory 286 to 123 MB.
  Binaries carry no DWARF or symbol table (`XO_GO_DWARF=1` keeps them for
  a debugger); programs that do not import std/http do not link net/http.
- Spec audit second pass: `x.debug()` works on every value (std.md 1;
  was XO0601 on concrete types); new tests for single field positional
  variants, `Secret` redaction in Debug and log fields, `target` at run
  time, `Stdio.write`, `Fs.cwd`, `Fs.case_sensitive`, a real `Proc.run`,
  `XO-F005`, `xo test --real`, and byte identical rebuilds (spec 12.1 now
  says "in the same directory").
- Modules in repository subdirectories (decision 0086): a module path
  `github.com/acme/tools/cli` is the directory `cli/` of the repository
  `github.com/acme/tools`, versioned by tags `cli/vX.Y.Z` (as Go);
  fetch, MVS, xo.lock, `xo vendor`, `xo get`, and `xo publish [--check]`
  (prefixed tag command) handle it; nested modules are left out of a
  module's tree and hash.
- Go calls in record and replay (decision 0093 stage 1): `--record`
  records `use go` calls whose arguments and results are values (an
  argument hash and the results) and `xo replay` answers them without
  calling Go; calls with a Go handle or a callback run again and the
  replay reports ``re executed foreign call `<name>` ``; pure std
  packages and xo.mod `replay rerun go <path> [names]` lines are not
  recorded. Binding format v5; traces gain `"foreign": 1`, and older
  traces still replay (every Go call runs again).
- Release builds (decision 0099): `xo build --release` compiles out
  `requires(debug)` and `ensures(debug)` on every backend and reports
  XO1308 for workspace overrides; `--ci` does not imply it.
- Lazy error frames (decision 0092, Go backend): frames keep copies of
  the parameter values and format them, bounded to 64 bytes, only when a
  trace is printed. 1,000,000 errors through 5 frames: 0.93 s to 0.10 s
  with short parameters, 2.78 s to 0.10 s with 90 byte ones; error output
  unchanged. `xo build --release --error-args=false` drops the values.

## v0.7.0 (2026-10-09)

First tagged release. Spec draft 0.7.

- Language: request scoped arenas by default, `use go` packages, two
  phase shutdown, `Mutex.with_read`, `BytesBuilder`, rune and integer
  conversions, native contracts and fault unwinding, habit diagnostics
  (XO0177), `clock.sleep_or_cancel`, cancellable stdin, tuple JSON,
  `Duration.parse` (decisions 0041 to 0088).
- Backends: Go (default), LLVM, native arm64, `--no-gc`, `--hybrid`.
- Toolchain: `xo version`.
- Planned before 1.0: modules and workspaces (0086), `std/time`, native
  TLS, and decisions on the Appendix B questions.
