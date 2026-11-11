# A keychain item prompts when another binary touches it

Status: Observed (2026-11-10, Lang 3, decision 0584 section 8)

## Symptom

cmd/xo TestRealKeychain ran std/native/llrt/userfs_test.xo on arm64 and
then on LLVM against one throwaway keychain. The arm64 run passed; the
LLVM run hung until `xo test`'s 1 minute limit, while the owner's screen
showed a keychain password dialog for the throwaway keychain.

## Cause

An item's access control list trusts the application that created it. The
LLVM test binary updated the `left` item the arm64 binary had added, so the
Security framework asked the user to allow it. Unlocking the keychain with
its password does not change that, and a throwaway keychain is no
exception.

## Fix

One keychain per binary: the harness creates and deletes a fresh keychain
for each backend's run. And no dialog is possible: `keychain.open_at`
(behind userfs `keychain_at`, test only) calls
SecKeychainSetUserInteractionAllowed(false) first and refuses to go on if
SecKeychainGetUserInteractionAllowed still says true, so a call that would
prompt fails at once with errSecInteractionNotAllowed (-25308).
