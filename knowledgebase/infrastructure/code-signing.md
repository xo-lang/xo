# Code signing (macOS and Windows)

Signing and notarizing binaries that Xo's own Mach-O linker writes
(compiler/macho, `--ld=macho`), and Authenticode signing of Windows
executables with Azure Artifact Signing ("Windows" below).

Last verified: 2026-10-18 (the trial below).

## What worked (trial, 2026-10-18)

Four binaries, all linked by compiler/macho: `xo` itself (the compiler at
main 60eaae0b) and a hello world, each for darwin/arm64 (`--backend=arm64`)
and darwin/amd64 (`--backend=amd64 --target=darwin/amd64`, cross built on
the arm64 Mac).

1. `codesign --force --sign "Developer ID Application: NAME (TEAMID)"
   --options runtime --timestamp FILE` replaced the linker's ad hoc,
   linker-signed signature on all four with no change to the linker
   (the layout codesign needs, LC_CODE_SIGNATURE last and __LINKEDIT
   ending the file, was already there). codesign re-hashes in 16 KiB
   pages on arm64 (the linker's signature uses 4 KiB pages: hello's 121
   page hashes became 31).
2. `codesign --verify --strict --verbose=2 FILE`: "valid on disk",
   "satisfies its Designated Requirement" for all four.
3. One notarization submission (a zip of the four, `ditto -c -k
   --keepParent`): Accepted, no issues in `notarytool log`.
4. Ran: on the arm64 Mac the signed hello printed its line and the signed
   `xo` built and ran a program; on the owner's Intel Mac (x86_64, no
   Rosetta on the arm64 Mac) the signed darwin/amd64 hello and
   `xo version` ran.

`scripts/sign-macos.sh` does steps 1 to 3 and the spctl checks.

## The team ID trap

The keychain holds two identities (`security find-identity -v -p
codesigning`): an "Apple Development: NAME (ID)" and the "Developer ID
Application: NAME (TEAMID)". The ID in the Apple Development
certificate's name is that certificate's own identifier, not a team ID;
the team ID is the one in the Developer ID Application name, and it is
the TeamIdentifier `codesign -dvv` prints after signing. Notarization
needs the Developer ID Application identity; an Apple Development
signature is for running on registered devices only.

## Credentials

The identity is in the login keychain; codesign used it with no prompt.
Notarization takes an App Store Connect API key from the environment:
`APPLE_API_KEY` (the .p8 file's path), `APPLE_API_KEY_ID`,
`APPLE_API_ISSUER`. Keep them in a file outside the repository and
source it in a subshell only, so the values never stay in a shell or
reach a log:

    ( set -a; . PATH/TO/.env; set +a
      XO_SIGN_IDENTITY="Developer ID Application: NAME (TEAMID)" \
        scripts/sign-macos.sh --notarize FILE... )

Never copy, print, or commit the key or that file, and never write its
path into a tracked file (the repository is public). Every submission
appears in the team's notary history: submit seldom, several binaries in
one zip.

## Releases (2026-10-22)

scripts/release.sh signs the darwin archives' bin/xo, bin/xo-gobind, and
race dylibs with `XO_SIGN_ID` and notarizes them through sign-macos.sh
(toolchain.md "Versions and releases"). For notarization, sign-macos.sh
also takes `XO_NOTARY_PROFILE`, a notarytool keychain profile. The owner
makes it once, interactively:

    xcrun notarytool store-credentials xo-notary

The password or key goes into the login keychain and never passes through
a script or an agent. Pass the identity as its SHA-1 hash, from `security
find-identity -v -p codesigning`, read at run time. The hash and the team
ID stay out of tracked files (decision 0249).

## Verification commands

    codesign -dvv FILE                     # Authority chain, Timestamp, TeamIdentifier, flags=0x10000(runtime)
    codesign --verify --strict --verbose=2 FILE
    spctl --assess --type open --context context:primary-signature -vv FILE
                                           # "accepted, source=Notarized Developer ID" after notarization
    spctl --assess --type execute -vv FILE # a bare executable: "rejected (the code is valid but does
                                           # not seem to be an app)", expected for command line tools

A bare Mach-O cannot carry a stapled ticket (`stapler` works on apps,
disk images, and installer packages); Gatekeeper looks the notarization
up online. The Intel Mac run copied the binaries with scp, which sets no
quarantine attribute, so spctl there checked the signature and the
notarization, not the path of a browser download (a quarantined file is
assessed when first opened).

## Tests

internal/arm64 TestCodesignAcceptsLinkerOutput (macOS, not -short):
links a hello world with `--ld=macho` for darwin/arm64 and darwin/amd64,
checks the linker's ad hoc signature with `codesign --verify --strict`,
re-signs it ad hoc with the hardened runtime (`codesign --force --sign -
--options runtime`), verifies again, and runs the host's binary. It needs
no identity; a Developer ID signature goes through the same codesign
rewrite. A byte appended after the linker's signature makes both the
strict verification and the re-signing fail ("main executable failed
strict validation"), so a linker that left data past __LINKEDIT fails
the test.

## Windows (Authenticode, Azure Artifact Signing)

Last verified: 2026-10-19 (the trial below).

The Azure portal calls the service Artifact Signing (it was Trusted
Signing; the API and jsign still say TRUSTEDSIGNING). A signing account
holds a certificate profile; the publisher identity is validated by
Microsoft, and the service holds the key and renews its short lived
certificates, so there is no key file to keep. The endpoint must match
the account's region (`https://REGION.codesigning.azure.net`, shown on
the account's overview page).

### What worked (trial, 2026-10-19)

Two windows/amd64 binaries from the LLVM backend, linked by lld-link
(cross built on the arm64 Mac, toolchain.md "Windows cross builds on the
Mac"): a hello world, and `xo` itself at main d9f15415 (one module at
O1, `XO_LLVM_OPT=O1 XO_LLVM_LTO=none`, no split: 49.5 min and 5.2 GB
peak RSS on the Mac, 151 MB; split builds do not link for Windows
today, gotchas/windows-llvm-split-link.md).

1. `scripts/sign-windows.sh hello.exe xo.exe`: one token request and
   two signatures (one jsign call each), RFC 3161 timestamps from
   `http://timestamp.acs.microsoft.com`.
2. The signer: CN=Joseph Spurrier, issued by "Microsoft ID Verified CS
   EOC CA 04" under "Microsoft ID Verified Code Signing PCA 2021"; the
   timestamp (the unsigned attribute 1.3.6.1.4.1.311.3.3.1) present on
   both.
3. Structure (no Windows machine needed): the certificate table is 8
   byte aligned and ends the file, the security directory names it, the
   PE checksum is recomputed, and the Authenticode SHA-256 of the image
   (the file without the checksum, the security directory entry, and the
   table) is the digest the signature carries.
4. Ran under Wine with Box64 (scripts/docker-wine.sh box64), unsigned
   and signed alike: hello printed its line, `xo version` printed
   `xo 0.21.0 windows/amd64`, and `xo check` reported a clean module and
   one with two errors.

Not checked yet: Windows' own verification (`signtool verify /pa /v`,
or the file's Properties, Digital Signatures) and SmartScreen; both
need a Windows machine. Next: re-sign Xo's own PE output when the amd64
backend's Windows target and its PE linker land (decision 0254 stage
6c), within the same few signatures.

### The tools

jsign 7.5 (a Java program; https://github.com/ebourg/jsign, the release
jar `jsign-7.5.jar`, SHA-1 f71ce8b6df19371c5cbf084223f142f9f678a7bb,
the same file on GitHub and Maven Central, its PGP signature good from
Emmanuel Bourg, key F513C419E4B9D0AC). It signs PE files on any OS, no
signtool. It needs a Java runtime: on macOS /usr/bin/java is a stub
until a JDK is installed (`brew install openjdk`, then
`JAVA=$(brew --prefix openjdk)/bin/java`). curl, python3 and openssl
do the rest.

### Credentials

An Entra app registration holding the certificate profile signer role
on the profile, through the client credentials flow. Six
variables: `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`,
`AZURE_CODE_SIGNING_ENDPOINT`, `AZURE_CODE_SIGNING_ACCOUNT`,
`AZURE_CODE_SIGNING_PROFILE`. Keep them in a mode 600 file outside the
repository and source it in a subshell only:

    ( set -a; . PATH/TO/azure.env; set +a
      JSIGN_JAR=PATH/jsign-7.5.jar JAVA=PATH/java scripts/sign-windows.sh FILE.exe... )

The script never puts the secret or the access token on a command line
(where ps shows them) or in its output: the secret goes to curl through a
config file (`client_secret@FILE`), the token to jsign as
`--storepass file:FILE`, both in a mode 700 temporary directory removed
on exit. Never print, copy, log, or commit the values, the token, or the
file's path. The account is on the Basic plan (owner, 2026-10-19): a
monthly fee with a monthly signature quota (5,000 on Basic when this was
written), then a small charge per signature; check the Azure portal for
the current figures. A release signs only xo.exe, so the quota is not a
constraint, but each signature is still a billed, owner-approved action:
agents never sign on their own, and a session's permission settings may
refuse the call (the owner then runs it). Rehearse with a self-signed
certificate first (jsign `--keystore KEY.p12 --storetype PKCS12`, the
same output layout). Last verified: 2026-10-19

### Releases (2026-10-22)

scripts/release.sh signs the windows/amd64 archive's `bin/xo.exe` and
`bin/xo-gobind.exe` after the build and before the archive is written
(owner, 2026-10-22: Windows signing in the next release after 0.25.2).

- **Credentials:** the six `AZURE_*` variables in the environment, or
  `XO_AZURE_ENV`, the path of the mode 600 file above. Only the
  subshell that signs reads it. Also `JSIGN_JAR` (jsign-7.5.jar,
  checked against the SHA-1 above) and `JAVA`
  (`$(brew --prefix openjdk)/bin/java`).
- **Early check:** `sign-windows.sh --check-auth` requests a token
  (nothing is signed or billed) before the long build. A bad secret or
  a missing jar stops release.sh at once.
- **Opting out:** `XO_RELEASE_WIN_UNSIGNED=1` builds unsigned Windows
  executables; `XO_RELEASE_UNSIGNED=1` skips all signing.
- **Checks after signing:**
  - sign-windows.sh checks each signature it made (signer, timestamp).
  - scripts/release-verify.sh on a windows archive checks the layout
    and both signatures on any host with scripts/verify-windows-sig.sh.
  - scripts/ci/release-smoke.sh on the Windows runner asks Windows
    itself: `Get-AuthenticodeSignature` must say Valid, with a
    timestamp.
  - `XO_VERIFY_UNSIGNED=1` turns the signature checks into UNSUPPORTED
    for unsigned trial archives.

`scripts/verify-windows-sig.sh FILE.exe...` runs osslsigncode verify
against Microsoft's roots in scripts/windows-codesign-roots.pem: the
digest, the chain, and the RFC 3161 timestamp, so the signing
certificate (valid for days) is checked at the signing time.
`XO_WIN_SIGNER` adds a check of the signer's subject.

- **osslsigncode:** taken from PATH, else a debian:trixie-slim
  container installs it (osslsigncode 2.9). Homebrew has no bottle for
  macOS 27, and building it from source needs cmake from source too.
- **CRLs:** online CRL checks are off (`-ignore-crl`). The fetch failed
  from the container, and without it the timestamp check fell back to
  the current time, so an expired signer certificate failed.
- **Tested** (2026-10-22): a Microsoft-signed vc_redist.x64.exe passes
  (chain to Root 2011, timestamp to Root 2010); 0.25.2's unsigned
  xo.exe fails with "No signature found".
- **Not tested yet:** an Artifact Signing signature (chain to the
  Identity Verification Root 2020) through this script, and the
  Windows smoke's PowerShell check. Both need the first signed release
  build and an owner-approved xo-windows CI run.

### Verification

    scripts/verify-windows-sig.sh FILE.exe                  # any host: osslsigncode, Microsoft's roots
    java -jar jsign-7.5.jar extract --format PEM FILE.exe   # writes FILE.exe.sig.pem
    openssl pkcs7 -in FILE.exe.sig.pem -print_certs -noout  # the signer and its chain
    openssl asn1parse -in FILE.exe.sig.pem | grep 1.3.6.1.4.1.311.3.3.1   # the RFC 3161 timestamp
    signtool verify /pa /v FILE.exe                          # on Windows

internal/llvm TestWindowsCertTableAppends (not -short; needs the Windows
cross toolchain, skips without it): appends a signature shaped
certificate table to an LLVM built windows/amd64 hello the way a signer
does, sets the security directory and the checksum, and checks that
debug/pe still parses it with every section before the table. No signing
service is called.
