#!/bin/bash
# sign-macos.sh: sign macOS executables with a Developer ID, verify them,
# and optionally notarize them (knowledgebase/infrastructure/code-signing.md).
#
#   XO_SIGN_IDENTITY="Developer ID Application: NAME (TEAMID)" \
#     scripts/sign-macos.sh [--notarize] [--zip OUT.zip] FILE...
#
# Each FILE is signed in place with the hardened runtime and a secure
# timestamp (replacing the ad hoc signature Xo's linker writes), then
# checked with codesign --verify --strict. --notarize zips the signed
# files, submits the zip with notarytool and waits, prints the notary log
# when the submission is not accepted, and runs spctl on each file.
# Credentials: XO_NOTARY_PROFILE, a notarytool keychain profile the owner
# made once with `xcrun notarytool store-credentials NAME` (the password
# never passes through this script), else the environment: APPLE_API_KEY
# (the path of the App Store Connect API key, .p8), APPLE_API_KEY_ID,
# APPLE_API_ISSUER.
# Source them in a subshell so they do not stay in your shell, for
# example: ( set -a; . PATH/TO/.env; set +a; scripts/sign-macos.sh --notarize FILE )
# Every submission shows in the team's notary history: submit seldom.
set -euo pipefail

notarize=0
zip=""
while [ $# -gt 0 ]; do
  case "$1" in
    --notarize) notarize=1; shift ;;
    --zip) zip=$2; shift 2 ;;
    --) shift; break ;;
    -*) echo "sign-macos.sh: unknown flag $1" >&2; exit 2 ;;
    *) break ;;
  esac
done
if [ $# -eq 0 ]; then
  echo "usage: XO_SIGN_IDENTITY=... sign-macos.sh [--notarize] [--zip OUT.zip] FILE..." >&2
  exit 2
fi
if [ -z "${XO_SIGN_IDENTITY:-}" ]; then
  echo "sign-macos.sh: set XO_SIGN_IDENTITY to the Developer ID Application identity (security find-identity -v -p codesigning)" >&2
  exit 2
fi

for f in "$@"; do
  codesign --force --sign "$XO_SIGN_IDENTITY" --options runtime --timestamp "$f"
  codesign --verify --strict --verbose=2 "$f"
done

if [ "$notarize" -eq 0 ]; then
  exit 0
fi
if [ -n "${XO_NOTARY_PROFILE:-}" ]; then
  auth=(--keychain-profile "$XO_NOTARY_PROFILE")
else
  for v in APPLE_API_KEY APPLE_API_KEY_ID APPLE_API_ISSUER; do
    if [ -z "${!v:-}" ]; then
      echo "sign-macos.sh: --notarize needs XO_NOTARY_PROFILE, or $v in the environment" >&2
      exit 2
    fi
  done
  auth=(--key "$APPLE_API_KEY" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER")
fi

tmp=$(mktemp -d)
trap 'rm -rf "${tmp:?}"' EXIT
mkdir "$tmp/signed"
for f in "$@"; do
  # two directory levels per file, so two files of the same name can go in
  # one zip (release.sh: xo-V-darwin-arm64/bin/xo and xo-V-darwin-amd64/bin/xo)
  d="$tmp/signed/$(basename "$(dirname "$(dirname "$f")")")/$(basename "$(dirname "$f")")"
  if [ -e "$d/$(basename "$f")" ]; then
    echo "sign-macos.sh: two files would be $d/$(basename "$f") in the zip" >&2
    exit 2
  fi
  mkdir -p "$d"
  cp -p "$f" "$d/"
done
[ -n "$zip" ] || zip=$tmp/signed.zip
ditto -c -k --keepParent "$tmp/signed" "$zip"

xcrun notarytool submit "$zip" "${auth[@]}" --wait --output-format json > "$tmp/submit.json"
id=$(plutil -extract id raw "$tmp/submit.json")
status=$(plutil -extract status raw "$tmp/submit.json")
echo "notarytool: submission $id: $status"
if [ "$status" != "Accepted" ]; then
  xcrun notarytool log "$id" "${auth[@]}"
  exit 1
fi
xcrun notarytool log "$id" "${auth[@]}" > "$tmp/log.json" && grep -E '"(status|issues)"' "$tmp/log.json" || true

for f in "$@"; do
  # a bare executable cannot carry a stapled ticket; Gatekeeper looks the
  # notarization up online
  spctl --assess --type execute -vv "$f" || true
done
