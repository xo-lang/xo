# scripts/ci/ci-out.sh: sourced by the suite=script CI scripts (release-smoke.sh,
# windows-race.sh). Actions artifacts carry text only: GitHub's artifact
# egress limit answered 503 on 2026-10-22, after scripts uploaded their
# 43 MB input archive and build products. So inputs and build products
# go to a work directory outside $XO_CI_OUT, and large outputs (crash
# dumps and bundles, binaries kept for debugging, text over 1 MiB) go to
# s3://$XO_SEED_BUCKET/dev/ci-out/$GITHUB_RUN_ID/ (the runner role writes
# dev/*, infra/ci-runners/iam.tf). Nothing expires under dev/: delete a
# run's prefix once read (knowledgebase/infrastructure/ci.md). Its
# PowerShell twin is ci-out.ps1.
#
#   w=$(ci_work NAME)        a fresh directory outside $XO_CI_OUT; the
#                            caller removes it (trap 'rm -rf "${w:?}"' EXIT)
#   ci_offload DIR SUMMARY   every file under DIR that is not text (by
#                            extension) or is text over CI_OUT_MAX bytes
#                            moves to the bucket, a line per file in
#                            SUMMARY (its s3 key and size); outside CI (no
#                            GITHUB_RUN_ID or bucket) files stay and say so

CI_OUT_MAX=${CI_OUT_MAX:-1048576}

ci__np() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }

ci_work() {
  local base=${RUNNER_TEMP:-${TMPDIR:-/tmp}}
  if command -v cygpath >/dev/null; then base=$(cygpath -u "$base"); fi
  mktemp -d "${base%/}/xo-$1.XXXXXX"
}

ci_offload() {
  local dir=$1 sum=$2 f rel size dest n=0
  local run=${GITHUB_RUN_ID:-}
  local bucket=${XO_SEED_BUCKET:-}
  while IFS= read -r f; do
    size=$(wc -c < "$f" | tr -d ' ')
    case "$(basename "$f")" in .*) continue ;; esac
    case "$f" in
      *.txt | *.log | *.out | *.err | *.json | *.md | *.csv)
        [ "$size" -le "$CI_OUT_MAX" ] && continue ;;
    esac
    rel=${f#"$dir"/}
    if [ -z "$run" ] || [ -z "$bucket" ] || ! command -v aws >/dev/null; then
      echo "large output kept locally (no CI bucket): $rel ($size bytes)" >> "$sum"
      continue
    fi
    dest="s3://$bucket/dev/ci-out/$run/$(basename "$dir")/$rel"
    if aws s3 cp --only-show-errors "$(ci__np "$f")" "$dest"; then
      rm -f "$f"
      echo "large output in the bucket: $dest ($size bytes)" >> "$sum"
      n=$((n + 1))
    else
      echo "large output NOT uploaded (aws s3 cp failed), left out of the artifact: $rel ($size bytes)" >> "$sum"
      rm -f "$f"
    fi
  done < <(find "$dir" -type f)
  [ "$n" -gt 0 ] && echo "large outputs: $n under s3://$bucket/dev/ci-out/$run/ (delete the prefix once read)" >> "$sum"
  true
}
