#!/bin/bash
# sign-windows.sh: sign Windows executables with Azure Artifact Signing
# (formerly Trusted Signing) through jsign, and check the signatures
# (knowledgebase/infrastructure/code-signing.md "Windows").
#
#   JSIGN_JAR=PATH/jsign-7.5.jar [JAVA=PATH/java] scripts/sign-windows.sh FILE.exe...
#   ... scripts/sign-windows.sh --check-auth
#
# --check-auth only requests an access token (no signature, nothing
# billed) and exits 0 when the credentials work: release.sh runs it
# before its long build, so a bad secret stops it at once.
#
# Credentials come only from the environment: AZURE_TENANT_ID,
# AZURE_CLIENT_ID, AZURE_CLIENT_SECRET (an app registration with the
# signing role on the profile), AZURE_CODE_SIGNING_ENDPOINT (the
# account's regional endpoint, https://REGION.codesigning.azure.net),
# AZURE_CODE_SIGNING_ACCOUNT, AZURE_CODE_SIGNING_PROFILE. Source them in
# a subshell, for example ( . PATH/TO/azure.env; scripts/sign-windows.sh FILE ).
# The secret and the access token only ever sit in a mode 700 temporary
# directory removed on exit (curl and jsign read them from files), never
# on a command line or in the output. Each signature is billed: sign
# seldom.
set -euo pipefail

check_auth=0
if [ "${1:-}" = --check-auth ]; then
  check_auth=1
  shift
fi
if [ $# -eq 0 ] && [ $check_auth = 0 ]; then
  echo "usage: JSIGN_JAR=... sign-windows.sh FILE.exe... | --check-auth" >&2
  exit 2
fi
if [ ! -f "${JSIGN_JAR:-}" ]; then
  echo "sign-windows.sh: JSIGN_JAR names no file (jsign-7.5.jar, code-signing.md \"The tools\")" >&2
  exit 2
fi
for v in AZURE_TENANT_ID AZURE_CLIENT_ID AZURE_CLIENT_SECRET AZURE_CODE_SIGNING_ENDPOINT AZURE_CODE_SIGNING_ACCOUNT AZURE_CODE_SIGNING_PROFILE JSIGN_JAR; do
  if [ -z "${!v:-}" ]; then
    echo "sign-windows.sh: set $v" >&2
    exit 2
  fi
done
java=${JAVA:-java}
if ! "$java" -version > /dev/null 2>&1; then
  echo "sign-windows.sh: no Java runtime at $java (set JAVA; on macOS /usr/bin/java is a stub until a JDK is installed)" >&2
  exit 2
fi

umask 077
tmp=$(mktemp -d)
trap 'rm -rf "${tmp:?}"' EXIT
chmod 700 "$tmp"

# the token: the client credentials flow, everything curl sends from a
# config file
printf '%s' "$AZURE_CLIENT_SECRET" > "$tmp/secret"
cat > "$tmp/curl.cfg" <<EOF
url = "https://login.microsoftonline.com/$AZURE_TENANT_ID/oauth2/v2.0/token"
data = "grant_type=client_credentials"
data-urlencode = "client_id=$AZURE_CLIENT_ID"
data-urlencode = "client_secret@$tmp/secret"
data-urlencode = "scope=https://codesigning.azure.net/.default"
EOF
curl -sS -K "$tmp/curl.cfg" -o "$tmp/token.json"
rm -f "$tmp/secret" "$tmp/curl.cfg"
if ! python3 -I -c '
import json, sys
d = json.load(open(sys.argv[1]))
if "access_token" not in d:
    sys.stderr.write("sign-windows.sh: no token: %s: %s\n" % (d.get("error", "?"), d.get("error_description", "").split("\n")[0]))
    sys.exit(1)
open(sys.argv[2], "w").write(d["access_token"])
' "$tmp/token.json" "$tmp/token"; then
  exit 1
fi
rm -f "$tmp/token.json"
if [ $check_auth = 1 ]; then
  echo "sign-windows.sh: the credentials give a token"
  exit 0
fi

for f in "$@"; do
  "$java" -jar "$JSIGN_JAR" --storetype TRUSTEDSIGNING \
    --keystore "$AZURE_CODE_SIGNING_ENDPOINT" \
    --storepass "file:$tmp/token" \
    --alias "$AZURE_CODE_SIGNING_ACCOUNT/$AZURE_CODE_SIGNING_PROFILE" \
    --tsaurl http://timestamp.acs.microsoft.com --tsmode RFC3161 \
    "$f"
done
rm -f "$tmp/token"

# the check: the signature's signer and its RFC 3161 timestamp
for f in "$@"; do
  cp "$f" "$tmp/check.exe"
  "$java" -jar "$JSIGN_JAR" extract --format PEM "$tmp/check.exe" > /dev/null
  sig=$(ls "$tmp"/check.exe.sig.pem)
  # every certificate's subject (the signer's, its issuers', the TSA's)
  subject=$(openssl pkcs7 -in "$sig" -print_certs -noout 2>/dev/null | sed -n 's/^subject=//p' | tr '\n' ';' || true)
  # Authenticode's RFC 3161 timestamp: the unsigned attribute 1.3.6.1.4.1.311.3.3.1
  if openssl asn1parse -in "$sig" 2>/dev/null | grep -q "1.3.6.1.4.1.311.3.3.1"; then ts=yes; else ts=no; fi
  echo "$f: certificates ${subject} timestamp $ts"
  rm -f "$tmp/check.exe" "$sig"
  [ "$ts" = yes ] || exit 1
done
