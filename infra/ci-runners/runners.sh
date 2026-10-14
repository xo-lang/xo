#!/bin/bash
# infra/ci-runners/runners.sh: add, remove, and inspect the CI runners
# (decision 0225, infra/ci-runners/README.md). runners.json is the list;
# Terraform makes the machines; each machine registers itself with GitHub
# at boot.
#
#   runners.sh list                          every runner: machine state, GitHub status, labels
#   runners.sh add NAME TYPE LABELS [AZ]     a new machine (LABELS comma separated, e.g. xo-arm64-big,xo-arm64)
#   runners.sh adopt NAME INSTANCE LABELS    an existing machine, its disk and caches kept (Terraform import)
#   runners.sh remove NAME                   deregister from GitHub, then delete the machine and its disk
#   runners.sh start|stop NAME               by hand (the Lambda starts machines; they stop when idle)
#   runners.sh apply                         terraform plan and apply runners.json as it is
#
# add, adopt, remove, and apply show the plan and ask before applying
# (XO_TF_AUTO=1 applies without asking). The runner name on GitHub is
# xo-ci-NAME. Needs terraform, the AWS CLI for account ACCOUNT_ID, gh
# signed in as a repository admin, and python3.
set -euo pipefail
cd "$(dirname "$0")"
REPO=josephspurrier/xo
export AWS_REGION=${AWS_REGION:-us-east-1}
J=runners.json

die() { echo "runners.sh: $*" >&2; exit 1; }
edit() { python3 -I - "$J" "$@"; }
instance() { terraform output -json runners 2>/dev/null | python3 -I -c 'import json,sys; r=json.load(sys.stdin); print(r.get(sys.argv[1],{}).get("id",""))' "$1"; }
apply() {
  terraform init -input=false >/dev/null
  if [ "${XO_TF_AUTO:-}" = 1 ]; then terraform apply -input=false -auto-approve; else terraform apply -input=false; fi
}

cmd=${1:-list}
shift || true
case "$cmd" in
  list)
    states=$(aws ec2 describe-instances --filters Name=tag:purpose,Values=xo-ci \
      --query 'Reservations[].Instances[].[Tags[?Key==`ci-runner`]|[0].Value,InstanceId,InstanceType,State.Name]' --output text)
    gh_=$(gh api "repos/$REPO/actions/runners?per_page=100" --jq '.runners[]|[.name,.status,(if .busy then "busy" else "idle" end)]|@tsv')
    printf '%-22s %-20s %-13s %-9s %-8s %-5s %s\n' RUNNER INSTANCE TYPE MACHINE GITHUB JOB LABELS
    python3 -I - "$J" "$states" "$gh_" <<'PY'
import json, sys
runners = json.load(open(sys.argv[1]))
ec2 = {l.split("\t")[0]: l.split("\t")[1:] for l in sys.argv[2].splitlines() if l}
gh = {l.split("\t")[0]: l.split("\t")[1:] for l in sys.argv[3].splitlines() if l}
for k, r in sorted(runners.items()):
    n = "xo-ci-" + k
    i, t, s = ec2.get(n, ["-", r["type"], "absent"])
    g, b = gh.get(n, ["absent", "-"])
    print("%-22s %-20s %-13s %-9s %-8s %-5s %s" % (n, i, t, s, g, b if g == "online" else "-", ",".join(r["labels"])))
for n in sorted(set(gh) - {"xo-ci-" + k for k in runners}):
    print("%-22s (on GitHub only: %s; remove with gh api -X DELETE repos/.../actions/runners/ID)" % (n, gh[n][0]))
PY
    ;;
  add)
    [ $# -ge 3 ] || die "usage: add NAME TYPE LABELS [AZ]"
    name=$1 type=$2 labels=$3 az=${4:-us-east-1d}
    arch=$(aws ec2 describe-instance-types --instance-types "$type" --query 'InstanceTypes[0].ProcessorInfo.SupportedArchitectures[0]' --output text)
    [ "$arch" = arm64 ] || arch=amd64
    edit "$name" "$type" "$arch" "$labels" "$az" <<'PY'
import json, sys
p, name, typ, arch, labels, az = sys.argv[1:]
r = json.load(open(p))
if name in r: sys.exit("runners.sh: %s already exists" % name)
r[name] = {"type": typ, "arch": arch, "labels": labels.split(","), "az": az}
json.dump(r, open(p, "w"), indent=2); open(p, "a").write("\n")
PY
    apply
    echo "xo-ci-$name: created stopped or starting; it registers with GitHub at first boot (start it now: runners.sh start $name)"
    ;;
  adopt)
    [ $# -eq 3 ] || die "usage: adopt NAME INSTANCE LABELS"
    name=$1 id=$2 labels=$3
    read -r type az arch < <(aws ec2 describe-instances --instance-ids "$id" \
      --query 'Reservations[0].Instances[0].[InstanceType,Placement.AvailabilityZone,Architecture]' --output text)
    [ "$arch" = arm64 ] || arch=amd64
    edit "$name" "$type" "$arch" "$labels" "$az" "$id" <<'PY'
import json, sys
p, name, typ, arch, labels, az, iid = sys.argv[1:]
r = json.load(open(p))
if name in r: sys.exit("runners.sh: %s already exists" % name)
r[name] = {"type": typ, "arch": arch, "labels": labels.split(","), "az": az, "import_id": iid}
json.dump(r, open(p, "w"), indent=2); open(p, "a").write("\n")
PY
    apply
    ;;
  remove)
    [ $# -eq 1 ] || die "usage: remove NAME"
    name=$1
    python3 -I -c 'import json,sys; sys.exit(0 if sys.argv[2] in json.load(open(sys.argv[1])) else 1)' "$J" "$name" || die "no runner $name in $J"
    rid=$(gh api "repos/$REPO/actions/runners?per_page=100" --jq ".runners[]|select(.name==\"xo-ci-$name\")|.id")
    if [ -n "$rid" ]; then
      busy=$(gh api "repos/$REPO/actions/runners/$rid" --jq .busy)
      [ "$busy" = false ] || die "xo-ci-$name is running a job; try again when it is idle"
    fi
    edit "$name" <<'PY'
import json, sys
p, name = sys.argv[1:]
r = json.load(open(p)); del r[name]
json.dump(r, open(p, "w"), indent=2); open(p, "a").write("\n")
PY
    apply || { git checkout -- "$J" 2>/dev/null; die "apply failed or declined; $J restored"; }
    if [ -n "$rid" ]; then gh api -X DELETE "repos/$REPO/actions/runners/$rid" && echo "xo-ci-$name removed from GitHub"; fi
    ;;
  start|stop)
    [ $# -eq 1 ] || die "usage: $cmd NAME"
    id=$(instance "$1"); [ -n "$id" ] || die "no machine for $1"
    aws ec2 "$cmd-instances" --instance-ids "$id" --query '[StartingInstances,StoppingInstances][]|[0].CurrentState.Name' --output text
    ;;
  apply) apply ;;
  *) die "unknown command $cmd (list, add, adopt, remove, start, stop, apply)" ;;
esac
