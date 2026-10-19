# Code signing (macOS)

Signing and notarizing binaries that Xo's own Mach-O linker writes
(compiler/macho, `--ld=macho`). Windows signing is not set up yet.

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
