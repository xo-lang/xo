#!/usr/bin/env bash
# Run the Xo CI matrix on fresh EC2 machines and always tear them down.
#
#   infra/ci-aws/ci.sh [jobs...]     (jobs: short full race sanitizers; default all)
#
# Creates the instances with Terraform, copies the working tree with rsync,
# installs the host's Go version plus clang and lld, runs the jobs on every
# instance in parallel, copies the logs to infra/ci-aws/logs/<timestamp>/,
# then runs `terraform destroy` (also on error or Ctrl-C, via a trap) and
# checks that nothing tagged project=xo-ci is left.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
account=${XO_AWS_ACCOUNT:?set XO_AWS_ACCOUNT to the AWS account ID (CLAUDE.local.md)}
stamp="$(date +%Y%m%d-%H%M%S)"
logs="$here/logs/$stamp"
mkdir -p "$logs"

got="$(aws sts get-caller-identity --query Account --output text)"
if [ "$got" != "$account" ]; then
	echo "wrong AWS account $got (want $account)" >&2
	exit 1
fi

go_version="$(go version | sed -n 's/^go version go\([0-9.]*\) .*/\1/p')"
key="$here/xo-ci-key.pem"

teardown() {
	set +e
	echo "== terraform destroy ($(date -u +%H:%M:%S))"
	terraform -chdir="$here" destroy -auto-approve -input=false >"$logs/destroy.log" 2>&1
	echo "destroy exit=$?"
	rm -f "$key"
	"$here/verify-teardown.sh" | tee "$logs/teardown-verify.txt"
}
trap teardown EXIT

terraform -chdir="$here" init -input=false >/dev/null
echo "== terraform apply ($(date -u +%H:%M:%S))"
terraform -chdir="$here" apply -auto-approve -input=false >"$logs/apply.log" 2>&1 || {
	tail -30 "$logs/apply.log" >&2
	exit 1
}
terraform -chdir="$here" output -json hosts >"$logs/hosts.json"
terraform -chdir="$here" output -json amis >"$logs/amis.json"
cat "$logs/hosts.json" "$logs/amis.json"

ssh_opts=(-i "$key" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null
	-o LogLevel=ERROR -o ServerAliveInterval=30 -o ConnectTimeout=10)

run_host() {
	local name="$1" ip="$2" t0
	shift 2
	local out="$logs/$name"
	mkdir -p "$out"
	t0=$(date +%s)
	for _ in $(seq 60); do
		ssh "${ssh_opts[@]}" "ubuntu@$ip" true 2>/dev/null && break
		sleep 5
	done
	echo "[$name] ssh up after $(($(date +%s) - t0)) s"
	t0=$(date +%s)
	rsync -az --delete -e "ssh ${ssh_opts[*]}" \
		--exclude /runs/ --exclude /.tools/ --exclude /.claude/ --exclude /.git/ \
		--exclude '/infra/ci-aws/.terraform*' --exclude '/infra/ci-aws/*.tfstate*' \
		--exclude /infra/ci-aws/logs/ --exclude /infra/ci-aws/xo-ci-key.pem \
		"$repo/" "ubuntu@$ip:xo/"
	echo "[$name] rsync $(($(date +%s) - t0)) s"
	t0=$(date +%s)
	ssh "${ssh_opts[@]}" "ubuntu@$ip" "bash xo/infra/ci-aws/remote-setup.sh $go_version" >"$out/setup.log" 2>&1 || {
		echo "[$name] setup failed" >&2
		tail -20 "$out/setup.log" >&2
		return 1
	}
	echo "[$name] setup $(($(date +%s) - t0)) s"
	ssh "${ssh_opts[@]}" "ubuntu@$ip" "bash xo/infra/ci-aws/remote-jobs.sh $*" 2>&1 | tee "$out/jobs.txt" | sed "s/^/[$name] /" || true
	rsync -az -e "ssh ${ssh_opts[*]}" "ubuntu@$ip:logs/" "$out/"
}

shift_jobs=("$@")
pids=()
while read -r name ip; do
	run_host "$name" "$ip" "${shift_jobs[@]+"${shift_jobs[@]}"}" &
	pids+=($!)
done < <(python3 -c 'import json,sys; [print(k, v) for k, v in json.load(open(sys.argv[1])).items()]' "$logs/hosts.json")
status=0
for p in "${pids[@]}"; do wait "$p" || status=1; done
echo "== logs in $logs"
exit $status
