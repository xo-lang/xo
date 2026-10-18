# xocheck.sh (sourced: . bench/perf/xocheck.sh XO [ROOT]): decision 0264's guard
# for a measurement. It refuses (exit 1) an xo whose build stamp (`xo
# version --json`) does not match the checkout's sources (ROOT, default
# the git top level of the working directory): its source
# key is not `go run ./scripts/xobuild --key`, or it has no stamp (built
# before the stamps, or by hand). XO_BENCH_ANY=1 measures anyway and says
# so. It sets XO_INLINE_STRICT=1 unless the caller set it, so an inliner
# fallback fails the build instead of skewing the numbers, and XO_STAMP
# to a line for the results ("xo 0.21.0 commit c07c2834 key 0bbef18b"
# with ", dirty HASH" or ", NOT THIS CHECKOUT").
_xc_xo=${1:?usage: . bench/perf/xocheck.sh XO}
_xc_here=${2:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}
export XO_INLINE_STRICT=${XO_INLINE_STRICT:-1}
_xc_json=$("$_xc_xo" version --json 2>/dev/null || true)
_xc_field() { printf '%s' "$_xc_json" | sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p"; }
_xc_key=$(_xc_field source_key)
_xc_commit=$(_xc_field commit)
_xc_dirty=$(_xc_field diff_hash)
_xc_want=$(cd "$_xc_here" && go run ./scripts/xobuild --key 2>/dev/null || true)
XO_STAMP="xo $(_xc_field version) commit $(printf '%.8s' "$_xc_commit") key $_xc_key"
[ -z "$_xc_dirty" ] || XO_STAMP="$XO_STAMP, dirty $_xc_dirty"
if [ -z "$_xc_key" ] || [ "$_xc_key" != "$_xc_want" ]; then
	XO_STAMP="$XO_STAMP, NOT THIS CHECKOUT (its key is $_xc_want)"
	if [ "${XO_BENCH_ANY:-}" != 1 ]; then
		echo "xocheck: $_xc_xo is not a build of this checkout's sources: stamp key '${_xc_key}', checkout key '${_xc_want}' (rebuild with go run ./scripts/xobuild, or XO_BENCH_ANY=1 to measure it anyway)" >&2
		exit 1
	fi
	echo "xocheck: XO_BENCH_ANY=1: measuring $_xc_xo although it is not this checkout's build" >&2
fi
export XO_STAMP
unset _xc_xo _xc_here _xc_json _xc_key _xc_commit _xc_dirty _xc_want
