#!/usr/bin/env bash
# Runs on a CI instance from the repository root (~/xo): the CI jobs, each
# with its own cold Go build cache so the times match separate CI runners.
# Prints one "JOB <name> exit=<code> seconds=<n>" line per job; each job's
# output goes to ~/logs/<name>.log. A failing job does not stop the others.
set -uo pipefail
export PATH=/usr/local/go/bin:$PATH CGO_ENABLED=1 GOFLAGS=-buildvcs=false GOTOOLCHAIN=local
cd ~/xo
mkdir -p ~/logs
echo "uname: $(uname -srm)  cpus: $(nproc)  mem: $(free -g | awk '/Mem:/{print $2}') GiB"
go version
clang --version | head -1
# one line: the jobs pass it through `bash -c`, where a newline would end
# the command (infra gotcha, 2026-10-09)
PKGS="$(go list -e ./... | grep -v /runs/ | tr '\n' ' ')"

job() {
	local name="$1"
	shift
	export GOCACHE="$HOME/cache/$name/go-build" TMPDIR="$HOME/cache/$name/tmp"
	rm -rf "$HOME/cache/$name"
	mkdir -p "$GOCACHE" "$TMPDIR"
	local start end code
	start=$(date +%s)
	bash -c "$*" >"$HOME/logs/$name.log" 2>&1
	code=$?
	end=$(date +%s)
	echo "JOB $name exit=$code seconds=$((end - start))"
	rm -rf "$HOME/cache/$name"
}

jobs="${*:-short full race sanitizers}"
for j in $jobs; do
	case "$j" in
	short) job short "go vet $PKGS && go test -count=1 -short -timeout 30m $PKGS" ;;
	full) job full "go test -count=1 -timeout 60m $PKGS" ;;
	race) job race "go test -count=1 -race -timeout 30m ./std/rt" ;;
	sanitizers) job sanitizers "go test -count=1 -v -timeout 30m -run 'Runtime|GoldenThreads|GoldenTasks|Deadlock|ASan|NativeSignal' ./internal/llvm" ;;
	*) echo "unknown job $j" >&2 ;;
	esac
done
