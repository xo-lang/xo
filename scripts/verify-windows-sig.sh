#!/bin/bash
# verify-windows-sig.sh: check the Authenticode signature of Windows
# executables on any host (knowledgebase/infrastructure/code-signing.md
# "Windows"):
#
#   scripts/verify-windows-sig.sh FILE.exe...
#
# osslsigncode verify against Microsoft's roots in
# scripts/windows-codesign-roots.pem: the file's digest matches the
# signed one, the signer's chain reaches a Microsoft root, and the RFC 3161
# timestamp verifies (so the short lived signing certificate is checked
# at the signing time). Online CRL checks are off (-ignore-crl): the CRL
# fetch is not reliable from a container and adds nothing a release check
# needs. osslsigncode comes from PATH, else a debian:trixie-slim container
# installs it (Homebrew has no macOS 27 bottle; Docker must run). With
# XO_WIN_SIGNER set, the signer's subject must contain it. Prints
# "FILE: ok SIGNER" or "FILE: FAILED" with osslsigncode's lines; exit 1
# when any file fails.
set -uo pipefail
if [ $# -eq 0 ]; then
  echo "usage: verify-windows-sig.sh FILE.exe..." >&2
  exit 2
fi
root=$(cd "$(dirname "$0")/.." && pwd)
roots=$root/scripts/windows-codesign-roots.pem
fails=0
for f in "$@"; do
  abs=$(cd "$(dirname "$f")" && pwd)/$(basename "$f")
  if command -v osslsigncode > /dev/null 2>&1; then
    out=$(osslsigncode verify -CAfile "$roots" -TSA-CAfile "$roots" -ignore-crl -in "$abs" 2>&1)
    st=$?
  else
    out=$(docker run --rm -v "$(dirname "$abs"):/w:ro" -v "$roots:/roots.pem:ro" debian:trixie-slim sh -c \
      'apt-get update -qq > /dev/null && apt-get install -y -qq osslsigncode > /dev/null 2>&1 || { echo "osslsigncode: install failed"; exit 3; }
       osslsigncode verify -CAfile /roots.pem -TSA-CAfile /roots.pem -ignore-crl -in "/w/$1"' sh "$(basename "$abs")" 2>&1)
    st=$?
  fi
  signer=$(printf '%s\n' "$out" | sed -n 's/^[[:space:]]*Subject: //p' | head -1)
  if [ $st -eq 0 ] && printf '%s\n' "$out" | grep -q '^Signature verification: ok' &&
    { [ -z "${XO_WIN_SIGNER:-}" ] || printf '%s' "$signer" | grep -qF "$XO_WIN_SIGNER"; }; then
    echo "$f: ok $signer"
  else
    echo "$f: FAILED"
    printf '%s\n' "$out" | grep -E 'verification|Subject:|Error|Failed|No signature' | sed 's/^/  /'
    fails=$((fails + 1))
  fi
done
[ $fails -eq 0 ]
