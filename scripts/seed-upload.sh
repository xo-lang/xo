#!/bin/bash
# seed-upload.sh WORKDIR: upload the seed scripts/seed-cut.sh packed in
# WORKDIR/out to the private bucket and make it the newest (decision
# 0216; toolchain.md "Cutting a seed"). Only with the owner's approval of
# the seed, from a machine whose AWS user may write the bucket (the
# development Mac; CI machines may only write dev/).
#
# It checks the five tarballs against their .sha256 and the manifest,
# uploads them, the .sha256 files, and manifest.txt to
# seeds/<seed>/, reads the listing back, then writes seeds/latest.txt
# last (a host whose platform is missing from the newest seed would
# fail), and reads latest.txt back. A dry run's output is refused.
#
# XO_SEED_BUCKET names the bucket (required; CLAUDE.local.md, decision 0249).
set -euo pipefail

die() { echo "seed-upload: $*" >&2; exit 1; }
[ $# -eq 1 ] || die "usage: seed-upload.sh WORKDIR"
out=$1/out
bucket=${XO_SEED_BUCKET:?set XO_SEED_BUCKET to the seed bucket (CLAUDE.local.md)}
[ -s "$out/SEED" ] || die "$out/SEED missing: seed-cut.sh did not finish"
[ ! -e "$out/DRY_RUN" ] || die "$out is a dry run's output"
seed=$(cat "$out/SEED")
echo "$seed" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+-[0-9a-f]{8}$' || die "seed name $seed"
command -v aws >/dev/null || die "needs the aws CLI"

plats="darwin-arm64 darwin-amd64 linux-arm64 linux-amd64 windows-amd64"
for p in $plats; do
  f=$out/$p.tar.gz
  [ -f "$f" ] && [ -f "$f.sha256" ] || die "$p: $f or its .sha256 missing"
  want=$(cut -d' ' -f1 < "$f.sha256")
  got=$(shasum -a 256 "$f" | cut -d' ' -f1)
  [ "$want" = "$got" ] || die "$p: sha256 $got, .sha256 says $want"
  grep -q "^$p $want" "$out/manifest.txt" || die "$p: not in manifest.txt with $want"
done
grep -q "^version ${seed%-*}$" "$out/manifest.txt" || die "manifest.txt is not version ${seed%-*}"
if aws s3 ls "s3://$bucket/seeds/$seed/manifest.txt" >/dev/null 2>&1; then
  die "seeds/$seed exists in the bucket"
fi

for p in $plats; do
  aws s3 cp --only-show-errors "$out/$p.tar.gz" "s3://$bucket/seeds/$seed/$p.tar.gz"
  aws s3 cp --only-show-errors "$out/$p.tar.gz.sha256" "s3://$bucket/seeds/$seed/$p.tar.gz.sha256"
done
aws s3 cp --only-show-errors "$out/manifest.txt" "s3://$bucket/seeds/$seed/manifest.txt"
n=$(aws s3 ls "s3://$bucket/seeds/$seed/" | wc -l | tr -d ' ')
[ "$n" = 11 ] || die "seeds/$seed/ lists $n objects, want 11; latest.txt not changed"

echo "$seed" > "$out/latest.txt"
aws s3 cp --only-show-errors "$out/latest.txt" "s3://$bucket/seeds/latest.txt"
back=$(aws s3 cp --only-show-errors "s3://$bucket/seeds/latest.txt" -)
[ "$back" = "$seed" ] || die "latest.txt reads $back"
echo "SEED UPLOADED: $seed (seeds/latest.txt)"
