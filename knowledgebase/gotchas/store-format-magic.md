# A store from an older xo faulted instead of failing cleanly

Status: Observed (2026-10-28)

## Symptom

A separately compiled build given a std store (XO_SEPCOMP_STORE) that an
xo from a few hours earlier had written stopped with `fault: index
101636375413 out of range for Bytes of length 27561733 at
macho/lin.xo:90`, not an error naming the store.

## Cause

compiler/macho store.xo's fifth round (50e9dc4b2) changed the symbol
records (fixed 28 byte records, names in the data region) but kept the
file's magic `XOSTORE1`, so store_view took the older file for its own
layout and read offsets out of range.

## Fix

Change STORE_MAGIC with every change of the store's layout (28d3fef6c
made it `XOSTORE2` for the run records' references). The default-on
store (decision 0388) keys its cache entries by xo's identity as well,
so a released xo never reads another's store.
