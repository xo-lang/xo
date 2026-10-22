#!/bin/sh
# Builds one binary per mode (the mode is the default of `xo bench --ab`'s
# shared arguments, so each variant is its own binary) and compares them:
# format (layout text each call), format_cache (a Mutex held Map from text to
# Layout), format_layout (a lazy const Layout). Run from this directory;
# XO is the xo to use (default xo), OUT a scratch directory.
set -e
XO=${XO:-xo}
OUT=${OUT:?a scratch directory}
for m in format format_cache format_layout; do
  mkdir -p "$OUT/v_$m"
  cp xo.mod "$OUT/v_$m/"
  sed "s/\"none\"/\"$m\"/" main.xo > "$OUT/v_$m/main.xo"
  (cd "$OUT/v_$m" && $XO build -o "$OUT/$m.bin" .)
  /usr/bin/time -l "$OUT/$m.bin" 2>&1 | grep "instructions retired"
done
$XO bench --ab --rounds=7 "$OUT/format.bin" "$OUT/format_cache.bin"
$XO bench --ab --rounds=7 "$OUT/format_cache.bin" "$OUT/format_layout.bin"
