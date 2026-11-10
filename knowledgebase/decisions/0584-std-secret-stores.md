# 0584: Per-OS secret stores: Keyring, Keychain, Dpapi under one `secrets` effect

Status: Proposed (Lang 3, 2026-11-10). Asked for by Azure 1
(`~/xo-lead/notes/aws-43/PROPOSAL-std-0012b-secret-stores.md`), assigned by
the lead; unblocks azidentity/cache, the last Azure data-plane module.
Related: 0004 (effects and capabilities), 0156 (attenuation), 0161 (Proc).

## Context

Go's `azidentity/cache` keeps a token cache in the user's OS secret store:
the Linux kernel keyring (a 32 byte key; the cache is a JWE file), macOS
Keychain (the cache is a generic password), Windows DPAPI (the cache file is
a blob). Std has no way to reach any of them. Programs reach the world only
through capabilities handed down from `main` (core 6.3), so the stores must
be capabilities with test fakes, like `Fs` and `Net`.

Peers: golang.org/x/sys/unix (`KeyctlInt`, `AddKey`, `KeyctlSearch`,
`KeyctlBuffer`), keybase/go-keychain (cgo over Security.framework),
golang.org/x/sys/windows (`CryptProtectData`, `CryptUnprotectData`,
`SHGetKnownFolderPath`).

## Decision

1. **One new effect family, `secrets` with `secrets.read` and
   `secrets.write`** (like `fs`; `uses secrets` means both), added to the
   accepted names (compiler/check/decl.xo:926, the did-you-mean list at :955,
   call.xo:907 `fake_field` "FakeSecrets" => "secrets", the effect table at
   core.md:2637, `xo audit`). Read: `Keyring.user_persistent/search/read`,
   `Keychain.get_generic`, `Dpapi.unprotect`; write: `Keyring.add/unlink`,
   `Keychain.set_generic/delete_generic`, `Dpapi.protect`. It covers all
   three stores: they all read or write the user's secrets, and a function
   that may touch any of them should say so in one word. `xo audit` lists it.
2. **One new capability field, `Os.secrets: Secrets`**, and
   `TestOs.secrets: FakeSecrets`. `Secrets` is an interface with three
   optional members, each present only where the OS has the store:

   ```
   interface Secrets {
     /// Linux kernel keyring; None on other OSes.
     fn keyring(self) -> Keyring?
     /// macOS login keychain; None on other OSes.
     fn keychain(self) -> Keychain?
     /// Windows Data Protection API; None on other OSes.
     fn dpapi(self) -> Dpapi?
     /// Attenuation: a Secrets with only the named stores
     /// (`"keyring"`, `"keychain"`, `"dpapi"`), as Proc.only.
     fn only(self, stores: List[Str]) -> Secrets
   }
   ```

   A grouping field (not three `Os` fields) keeps `Os` from growing per OS
   store. `FakeSecrets` returns all three fakes on every OS, so a test of
   Azure's per-OS code runs everywhere.
3. **Keyring** (Linux keyrings(7)), exactly the calls Go's azidentity uses:

   ```
   interface Keyring {
     fn user_persistent(self) -> KeyringId ! KeyErr uses secrets
     fn add(self, ring: KeyringId, kind: Str, desc: Str, payload: Bytes) -> KeyId ! KeyErr uses secrets
     fn search(self, ring: KeyringId, kind: Str, desc: Str) -> KeyId ! KeyErr uses secrets
     fn read(self, id: KeyId) -> Bytes ! KeyErr uses secrets
     fn unlink(self, id: KeyId, ring: KeyringId) -> () ! KeyErr uses secrets
   }
   enum KeyErr { NotFound, Expired, Revoked, Denied, Io(msg: Str) }
   ```

   `KeyringId` and `KeyId` are opaque Int newtypes. `user_persistent`:
   `get_keyring_id(USER, create)`; link the session keyring into it, errors
   ignored; `KEYCTL_GET_PERSISTENT(-1, user)`, falling back to the user
   keyring. ENOKEY, EKEYEXPIRED, EKEYREVOKED, ENOENT map to
   NotFound/Expired/Revoked/NotFound; EACCES/EPERM to Denied; the rest to Io
   with the errno text. Payload is passed as given (Go's 33 byte key plus 0
   is Azure's choice).
4. **Keychain** (macOS Security.framework), generic passwords only:

   ```
   interface Keychain {
     fn get_generic(self, service: Str, account: Str) -> Bytes? ! KeychainErr uses secrets
     fn set_generic(self, service: Str, account: Str, data: Bytes) -> () ! KeychainErr uses secrets
     fn delete_generic(self, service: Str, account: Str) -> () ! KeychainErr uses secrets
   }
   struct KeychainErr { status: Int, msg: Str }
   ```

   Not found (-25300) reads as `None`; delete treats -25300 and -25294 as
   success; `set_generic` reads first, then `SecItemAdd` or `SecItemUpdate`,
   as go-keychain. `msg` is `SecCopyErrorMessageString`'s text. No
   `keychain: Str?` path on the trait: a throwaway keychain for tests is a
   constructor of the real implementation (`secrets.keychain_at(path)`,
   `SecKeychainCreate` underneath), visible only to the std test harness,
   so a program cannot be pointed at another keychain.
5. **Dpapi** (Windows):

   ```
   interface Dpapi {
     fn protect(self, data: Bytes, entropy: Bytes = b"") -> Bytes ! DpapiErr uses secrets
     fn unprotect(self, blob: Bytes, entropy: Bytes = b"") -> Bytes ! DpapiErr uses secrets
   }
   struct DpapiErr { code: Int, msg: Str }
   ```

   `CryptProtectData`/`CryptUnprotectData` with no description, no prompt,
   `CRYPTPROTECT_UI_FORBIDDEN`; `LocalFree` on the output. `Env` gains
   `local_app_data() -> Str ! EnvErr uses env`, from
   `SHGetKnownFolderPath(FOLDERID_LocalAppData)` (not the `%LOCALAPPDATA%`
   variable, which Go does not read); `Env.only` does not narrow it (it is
   not a variable), and a `FakeEnv` takes a value.
6. **Fakes** in std/testing.xo: `FakeSecrets`, `FakeKeyring` (a map of rings
   to keys, a counter for ids, ENOKEY semantics), `FakeKeychain` (a map of
   service+account), `FakeDpapi` (a reversible wrap with a magic prefix and
   the entropy checked, not crypto). Each has `fail_next(err)` to inject
   errors, as other fakes.
7. **Backends.**
   - Native linux (arm64, amd64, riscv64): raw syscalls through
     `rt.sys6` (std/native/llrt/libcsys.xo `syscall`): `add_key`, `keyctl`
     (numbers per arch from the syscall table: arm64 217/219, amd64
     248/250, riscv64 217/219). No libc, no keyutils.
   - Native darwin: `use c` style bindings to `SecItemCopyMatching`,
     `SecItemAdd`, `SecItemUpdate`, `SecItemDelete` and the CoreFoundation
     dictionary calls; the Mach-O linker adds
     `/System/Library/Frameworks/Security.framework/Security` and
     `CoreFoundation.framework/CoreFoundation` to `opts.dylibs`
     (compiler/macho/link.xo:59) only when the program reaches a Keychain
     member, so other programs keep one dylib and no flat lookup.
   - Native windows (amd64, arm64): imports of `crypt32.dll`
     (`CryptProtectData`, `CryptUnprotectData`), `kernel32` (`LocalFree`),
     `shell32`/`ole32` (`SHGetKnownFolderPath`, `CoTaskMemFree`), added to
     the PE import table on demand, as the existing kernel32 imports.
   - LLVM backend: the same calls as `declare`s plus a link flag per OS
     (`-framework Security -framework CoreFoundation`; `-lcrypt32 -lshell32
     -lole32`).
   - Go backend (std/rt): `golang.org/x/sys` is not a std dependency, so
     the twin uses `syscall.Syscall` for keyctl on linux, cgo-free calls
     through `syscall.NewLazyDLL` on windows, and on darwin the twin returns
     `None` from `keychain()` (the Go backend is a test and portability
     backend; darwin keychain access needs cgo, which std/rt does not use).
     The corpus runs the Linux and Windows twins; darwin is checked on the
     native backends only. Wasm and WASI: all three are `None`.
   - Where a store is `None`, no code for it is linked.
8. **Safety and testing.** Every unit test uses the fakes. Real stores are
   checked only here: Linux keyctl on the offload hosts (the kernel keyring
   works in a plain login session; a container needs a seccomp profile that
   allows keyctl), each test using a uniquely named key it unlinks, never
   touching existing keys. macOS only against a throwaway keychain created
   in a temp directory and deleted after, never the login keychain (it may
   prompt), and only with the owner's go. Windows DPAPI on the winarm VM
   and the amd64 host, protect/unprotect round trips only, no stored state.
9. **Order of work:** Linux Keyring (native syscalls, Go twin, fake, corpus
   against Go's x/sys behavior on the hosts), then Keychain, then Dpapi.
   Each step: spec/std.md section, std/changes/secrets.md (decision 0276),
   `xo-at-a-glance.md`, README rows.

## Consequences

- A library handed no `Secrets` cannot read a key or cache; `xo audit` shows
  who is handed one; `uses secrets` is visible in signatures.
- `Os` gains a field: code constructing an `Os` by hand (only the runtime
  and `TestOs`) changes; user programs do not.
- The checker's effect-name list grows by one (`secrets`); `Proc.only`-style
  `Secrets.only` makes narrowing a one-liner.
- Darwin programs that use the Keychain link two frameworks (about 2 MB of
  mapped dylibs at run time); others are unchanged.
- The Go twin cannot reach the macOS keychain; documented.

## Alternatives

- Three effects (`keyring`, `keychain`, `dpapi`): finer audit, but three
  names for one idea and three `Os` fields; rejected.
- Three top level `Os` fields: rejected (grows `Os` per OS store).
- One portable `SecretStore` trait (get/set/delete by name) over all three:
  hides real differences (the Linux key is a short secret in a ring, Windows
  stores caller-held blobs, macOS stores named items); azidentity needs each
  OS's exact behavior. A portable layer can sit above these in a later
  library.
- `use c`/`use go` in the Azure repo instead of std: every program using it
  gets the `ffi`/`go` effect and no fakes; rejected.
- Linking libkeyutils for Linux: a dependency for six syscalls; rejected.

## Addendum (2026-11-10): the Os literal

Wiring finding: an `Os{...}` literal must name every field (compiler/ssa
expr.xo `os_lit` reports a missing one; std/native/llrt/userfs.xo `OsFields`
and `UserOs` list them; std/rt gen.go, testing.go and the generated
std_gen_prelude.go rebuild an Os field by field). Adding `Os.secrets` is
therefore **Breaking for a program that builds an Os literal** (the same
class as decision 0426's new Proc methods), and for a user type implementing
`Os`-shaped records. Two ways, to be settled with the lead before code:

- A. Accept it as Breaking in the next minor (CHANGELOG line, migration
  note: add `secrets: os.secrets`). Simple; matches 0426.
- B. Give `Os.secrets` a default so old literals compile (a struct field
  default, as `FileInfo.is_symlink = false`); only possible if `os_lit` and
  `OsFields` fill an omitted interface field with a "no stores" Secrets
  (every member None). More compiler work (Lang 1's file), no break.

Recommendation: B if Lang 1 agrees the `os_lit` change is small, else A.

## Addendum (2026-11-10, Lang 1's checker notes)

- The three declarations are `interface`, not `trait` (XO0177).
- Methods are annotated `uses secrets.read` or `uses secrets.write` per the
  split in decision item 1, not `uses secrets`.
- No effect parameter on Secrets/Keyring, so 0582's widening does not apply.
- Check before code: a default parameter in an interface method
  (`Dpapi.protect(data, entropy: Bytes = b"")`) must work for impls and
  fakes (Proc.cmd's `args = []` does, 0307); if not, two methods.

## Addendum (2026-11-10): staging behind a seed

The prelude declarations (`Keyring`'s `uses secrets.read`) are compiled by
the seed that builds the next xo, and a seed whose checker does not know
`secrets` rejects them (XO0702, found on the first stdgen run of the
secrets2 branch). So the work lands in two steps:

- Stage A (this commit): `secrets`, `secrets.read`, `secrets.write` are
  effect labels (compiler/check decl.xo is_effect_label and its
  did-you-mean list, modfile.xo EFFECT_LABELS, cli/modules publish.xo
  is_label, check/call.xo fake_field `FakeSecrets`), core 6.1's table
  lists them; test/errorcheck/effect_secrets. Nothing in std uses them.
- Stage B (after a seed cut that contains stage A): the prelude's
  `Secrets`, `Keyring`, `KeyErr`, `Os.secrets`, the native and Go
  backends, the fakes and the golden. Done 2026-11-10 except the fakes:
  RealSecrets, RealKeyring and NoSecrets in std/native/llrt/userfs.xo
  (SSA boxes them into the interface: own_os_field, os_no_access,
  secrets_rec; Secrets and Keyring dispatch as ordinary interfaces),
  std/rt/secrets.go and keyring_linux.go, golden secrets_keyring (a real
  round trip on the linux/arm64 host, native and Go). compiler/xo's Os
  wrappers name their own empty Secrets (NoSecretStores) because the
  0.47.0 seed lowers no read of `os.secrets`; a later seed lets them pass
  `os.secrets` on.
- Stage C: the test keyring. Done 2026-11-10 as `TestOs.secrets:
  Secrets` (no FakeSecrets type: Secrets and Keyring are ordinary
  interfaces, so a test needs no control methods; it adds keys through
  the Keyring itself). Native: FakeSecrets and FakeKeyring in
  llrt/userfs.xo over testworld.xo's key vectors (reset per test), chosen
  by own_os_field in a test build; Go: realSecrets.fake, a fakeKeyring per
  newTestOs. cmd/xo TestFakeSecrets on Go and every native backend.
- Stage D: the macOS Keychain and Windows DPAPI members.
