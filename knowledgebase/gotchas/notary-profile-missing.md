# The notarytool keychain profile can be gone at release time

Status: Observed (2026-11-02, release 0.36.0)

## Symptom

scripts/release.sh builds and signs every archive, then stops at
notarization: `Error: No Keychain password item found for profile:
xo-notary`. 0.35.0 had notarized with the same profile on 2026-10-30.

## Cause

The login keychain was unlocked (other items read fine) but held no
notarytool item of any name. When or how it left was not traced.

## Fix

Check the profile before a release build, so a long build does not end
in this:

    xcrun notarytool history --keychain-profile xo-notary

If it is gone, build with `XO_RELEASE_NO_NOTARIZE=1` (everything signed),
run the verify scripts, and notarize the same signed files later without
re-signing them (re-signing changes the archives and SHA256SUMS): zip the
darwin bin/xo, bin/xo-gobind and race dylibs, `xcrun notarytool submit
ZIP --keychain-profile xo-notary --wait`, require "Accepted", then
`spctl --assess --type open --context context:primary-signature -vv` on
each must say `source=Notarized Developer ID` (code-signing.md). Only the
owner can restore the profile (`xcrun notarytool store-credentials
xo-notary`, interactive).
