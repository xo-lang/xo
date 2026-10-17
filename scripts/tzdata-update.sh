#!/bin/sh
# Rebuilds std/tzdata/zoneinfo.zip, the IANA time zone database Xo ships
# (decision 0094's addendum, 2026-10-16), from an IANA release:
#
#   scripts/tzdata-update.sh [VERSION]      # e.g. 2026e; default: the latest
#
# As Go's lib/time/update.bash: tzcode and tzdata of VERSION from
# www.iana.org, IANA's own zic built and run (make posix_only, the
# backzone data, zone.tab's zones), then every TZif file stored
# (uncompressed, as the runtime and the build read them) in a zip with
# sorted names and fixed times, so the same VERSION gives the same bytes.
# Writes std/tzdata/zoneinfo.zip and std/tzdata/VERSION (the version and
# the SHA-256 of the two archives). Needs curl, a C compiler, make,
# python3. Run it per release; commit both files.
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
v=${1:-$(curl -fsS https://data.iana.org/time-zones/tzdb/version)}
case $v in
[0-9][0-9][0-9][0-9][a-z]) ;;
*) echo "tzdata-update: bad version $v" >&2; exit 2 ;;
esac
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
base=https://www.iana.org/time-zones/repository/releases
curl -fsS -L -O "$base/tzcode$v.tar.gz"
curl -fsS -L -O "$base/tzdata$v.tar.gz"
code_sum=$(shasum -a 256 "tzcode$v.tar.gz" | cut -d' ' -f1)
data_sum=$(shasum -a 256 "tzdata$v.tar.gz" | cut -d' ' -f1)
mkdir src
(cd src && tar xzf "../tzcode$v.tar.gz" && tar xzf "../tzdata$v.tar.gz")
if ! (cd src && make CFLAGS=-DSTD_INSPIRED AWK=awk TZDIR="$work/zoneinfo" PACKRATDATA=backzone PACKRATLIST=zone.tab posix_only >"$work/make.out" 2>&1); then
	cat "$work/make.out" >&2
	exit 1
fi
python3 - "$work/zoneinfo" "$root/std/tzdata/zoneinfo.zip" <<'PY'
import os, sys, zipfile
src, out = sys.argv[1], sys.argv[2]
names = []
for d, _, files in os.walk(src):
    for f in files:
        p = os.path.join(d, f)
        with open(p, "rb") as fh:
            if fh.read(4) == b"TZif":
                names.append(os.path.relpath(p, src))
names.sort()
tmp = out + ".tmp"
with zipfile.ZipFile(tmp, "w", zipfile.ZIP_STORED) as z:
    for n in names:
        info = zipfile.ZipInfo(n, date_time=(1980, 1, 1, 0, 0, 0))
        info.compress_type = zipfile.ZIP_STORED
        info.external_attr = 0o644 << 16
        with open(os.path.join(src, n), "rb") as fh:
            z.writestr(info, fh.read())
os.replace(tmp, out)
print(len(names), "zones")
PY
printf '%s\ntzcode%s.tar.gz sha256 %s\ntzdata%s.tar.gz sha256 %s\nsource %s\n' "$v" "$v" "$code_sum" "$v" "$data_sum" "$base" > "$root/std/tzdata/VERSION"
ls -l "$root/std/tzdata/zoneinfo.zip"
