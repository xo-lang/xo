# Timing rows against Go for the remaining std modules (2026-11-03)

Follows std-twin-rows-2026-11-03.md. Host 1 (linux/arm64, exclusive lease), xo main 1e1d45cad plus this
change set, `xo bench --ab --rounds=15`, A the Go twin (go build), B the Xo program (`--release`, native
arm64), the twins under bench/perf (new: bench/perf/time; trimchars and strconv now loop 20x longer, since
a 1 to 5 ms run is mostly process start). Ratios are Xo / Go wall with the 95% interval; peak memory is the
max RSS ratio. `none` rows are the setup alone. Not measured: toml, rate, aead, uuid and websocket (their twins
need Go modules from the network, which the host does not fetch), http (a server, tls-h2-vs-go-2026-10-21.md),
and the modules with no twin (log, term, sql, smtp, metrics, ...).

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| bytessearch | index | 0.734 | 0.554 | 0.755 | 0.742 to 0.806 | 1.0 |
| bytessearch | find_any | 5.911 | 4.793 | 0.811 | 0.802 to 0.827 | 1.0 |
| bytessearch | rfind_any | 6.591 | 5.6 | 0.85 | 0.821 to 0.862 | 1.0 |
| bytessearch | contains_any | 5.843 | 4.817 | 0.824 | 0.809 to 0.839 | 1.0 |
| bytessearch | contains_rune | 2.143 | 2.65 | 1.236 | 1.201 to 1.295 | 1.0 |
| bytessearch | cut | 2.045 | 2.174 | 1.063 | 1.024 to 1.078 | 1.0 |
| bytessearch | cut_prefix | 1.06 | 0.884 | 0.834 | 0.781 to 0.861 | 1.0 |
| bytessearch | cut_suffix | 1.058 | 0.911 | 0.862 | 0.823 to 0.879 | 1.0 |
| bytessearch | split_after | 11.05 | 8.73 | 0.79 | 0.769 to 0.8 | 0.566 |
| bytessplit | index | 0.866 | 0.736 | 0.85 | 0.795 to 0.89 | 1.0 |
| bytessplit | split | 14.07 | 10.86 | 0.772 | 0.75 to 0.807 | 0.563 |
| bytessplit | split_n | 5.985 | 4.451 | 0.744 | 0.729 to 0.758 | 0.564 |
| bytessplit | fields | 17.67 | 12.46 | 0.705 | 0.693 to 0.716 | 0.57 |
| bytessplit | trim | 2.109 | 1.962 | 0.93 | 0.898 to 0.964 | 1.0 |
| bytessplit | affix | 1.68 | 1.96 | 1.167 | 1.133 to 1.197 | 1.0 |
| bytestransform | index | 0.949 | 0.774 | 0.816 | 0.789 to 0.844 | 1.0 |
| bytestransform | replace | 12.88 | 10.71 | 0.831 | 0.821 to 0.852 | 0.587 |
| bytestransform | replace_n | 7.057 | 5.286 | 0.749 | 0.741 to 0.766 | 0.581 |
| bytestransform | lower | 14.75 | 8.722 | 0.591 | 0.524 to 0.605 | 0.574 |
| bytestransform | upper | 15.15 | 9.626 | 0.635 | 0.586 to 0.659 | 0.574 |
| bytestransform | fold | 1.688 | 1.881 | 1.114 | 1.08 to 1.152 | 1.0 |
| bytestransform | repeat | 3.768 | 3.782 | 1.004 | 0.978 to 1.028 | 0.572 |
| bytespredicate | index | 0.851 | 0.625 | 0.735 | 0.698 to 0.761 | 1.0 |
| bytespredicate | find_where | 2.852 | 1.15 | 0.403 | 0.382 to 0.426 | 1.0 |
| bytespredicate | rfind_where | 2.448 | 1.242 | 0.507 | 0.485 to 0.523 | 1.0 |
| bytespredicate | contains_where | 8.684 | 3.553 | 0.409 | 0.401 to 0.421 | 1.0 |
| bytespredicate | trim_where | 3.401 | 2.263 | 0.665 | 0.648 to 0.688 | 1.0 |
| bytespredicate | trim_start_where | 2.556 | 1.833 | 0.717 | 0.707 to 0.759 | 1.0 |
| bytespredicate | trim_end_where | 1.369 | 0.73 | 0.533 | 0.515 to 0.562 | 1.0 |
| bytespredicate | split_where | 20.84 | 17.43 | 0.836 | 0.777 to 0.86 | 0.57 |
| bytespredicate | map_runes | 15.98 | 16.47 | 1.03 | 1.021 to 1.046 | 0.577 |
| trimchars | index | 4.982 | 5.542 | 1.112 | 1.104 to 1.124 | 1.0 |
| trimchars | trim | 16.81 | 37.66 | 2.241 | 2.195 to 2.306 | 1.0 |
| trimchars | trim_start | 14.85 | 28.42 | 1.914 | 1.883 to 1.975 | 1.0 |
| trimchars | trim_end | 14.24 | 28.22 | 1.982 | 1.97 to 2.003 | 1.0 |
| strsplit | split | 10.06 | 8.636 | 0.858 | 0.841 to 0.88 | 0.555 |
| strsplit | split_one | 7.691 | 6.045 | 0.786 | 0.774 to 0.802 | 0.571 |
| strsplit | split_multi | 6.39 | 5.131 | 0.803 | 0.774 to 0.82 | 0.58 |
| strsplit | split_absent | 4.771 | 3.551 | 0.744 | 0.726 to 0.76 | 0.741 |
| strsplit | split_empty | 19.45 | 17.81 | 0.916 | 0.89 to 0.94 | 0.563 |
| strsplit | split_after | 7.747 | 7.084 | 0.914 | 0.898 to 0.924 | 0.573 |
| strsplit | lines | 6.953 | 4.939 | 0.71 | 0.688 to 0.731 | 0.572 |
| strsplit | bytes_split | 12.31 | 6.7 | 0.544 | 0.525 to 0.554 | 0.563 |
| strsplit | bytes_fields | 16.51 | 9.952 | 0.603 | 0.584 to 0.619 | 0.565 |
| strsplit | fields | 12.08 | 10.1 | 0.836 | 0.821 to 0.849 | 0.572 |
| strsplit | index | 0.827 | 0.652 | 0.788 | 0.752 to 0.83 | 1.0 |
| unicode | lower | 131.3 | 12.94 | 0.099 | 0.097 to 0.099 | 0.8 |
| unicode | upper | 178.6 | 13.01 | 0.073 | 0.071 to 0.076 | 0.798 |
| unicode | lower_u | 324.9 | 132.7 | 0.409 | 0.406 to 0.41 | 0.798 |
| unicode | upper_u | 328.6 | 135.8 | 0.413 | 0.407 to 0.416 | 0.797 |
| unicode | lower_az | 31.36 | 4.405 | 0.14 | 0.14 to 0.145 | 0.799 |
| unicode | upper_az | 31.6 | 4.559 | 0.144 | 0.142 to 0.148 | 0.799 |
| unicode | eqlower | 997.7 | 337.5 | 0.338 | 0.334 to 0.342 | 0.781 |
| unicode | classify | 20.01 | 6.968 | 0.348 | 0.342 to 0.356 | 1.033 |
| unicode | runecase | 16.31 | 10.02 | 0.614 | 0.605 to 0.628 | 1.034 |
| unicode | classes | 58.47 | 22.22 | 0.38 | 0.373 to 0.383 | 1.034 |
| unicode | latin1 | 1.553 | 2.3 | 1.48 | 1.452 to 1.538 | 1.034 |
| unicode | title | 10.85 | 8.656 | 0.798 | 0.784 to 0.805 | 1.362 |
| unicode | utf8len | 2.35 | 3.44 | 1.464 | 1.372 to 1.493 | 1.343 |
| unicode | quote | 13.6 | 4.829 | 0.355 | 0.351 to 0.365 | 0.811 |
| unicode | indexfunc | 369.2 | 303.4 | 0.822 | 0.815 to 0.826 | 0.873 |
| unicode | trimfunc | 4.371 | 3.136 | 0.717 | 0.689 to 0.738 | 1.034 |
| unicode | fieldsfunc | 20.4 | 11.53 | 0.565 | 0.55 to 0.575 | 0.799 |
| unicode | mapfunc | 209.2 | 125.9 | 0.602 | 0.591 to 0.608 | 1.016 |
| strconv | atoi | 34.5 | 50.39 | 1.46 | 1.449 to 1.489 | 1.0 |
| strconv | itoa | 90.67 | 138.0 | 1.522 | 1.495 to 1.569 | 0.558 |
| strconv | atof | 110.2 | 104.7 | 0.95 | 0.92 to 0.975 | 1.0 |
| strconv | ftoa | 277.8 | 168.2 | 0.605 | 0.6 to 0.61 | 0.572 |
| sort | ints | 62.4 | 59.42 | 0.952 | 0.943 to 0.96 | 0.905 |
| sort | strs | 27.25 | 38.73 | 1.421 | 1.39 to 1.498 | 1.787 |
| sort | floats | 77.54 | 80.72 | 1.041 | 1.033 to 1.046 | 1.434 |
| sort | by | 155.4 | 87.4 | 0.562 | 0.548 to 0.571 | 0.658 |
| regexp | none | 1.007 | 0.649 | 0.644 | 0.622 to 0.679 | 1.0 |
| regexp | compile | 81.23 | 91.62 | 1.128 | 1.12 to 1.142 | 0.611 |
| regexp | match | 49.36 | 61.72 | 1.25 | 1.239 to 1.262 | 1.0 |
| regexp | find_all | 101.7 | 90.41 | 0.889 | 0.881 to 0.901 | 0.627 |
| regexp | replace_all | 77.14 | 81.18 | 1.052 | 1.034 to 1.095 | 0.606 |
| regexp | captures | 10.03 | 9.9 | 0.987 | 0.977 to 1.001 | 1.0 |
| base64 | encode | 52.6 | 30.04 | 0.571 | 0.561 to 0.582 | 0.5 |
| base64 | decode | 36.47 | 31.91 | 0.875 | 0.867 to 0.883 | 0.657 |
| base64 | none | 10.64 | 4.688 | 0.441 | 0.429 to 0.452 | 0.805 |
| base32 | encode | 55.24 | 53.73 | 0.973 | 0.945 to 0.988 | 0.546 |
| base32 | decode | 172.8 | 304.3 | 1.761 | 1.747 to 1.766 | 0.761 |
| base32 | none | 10.66 | 4.646 | 0.436 | 0.424 to 0.441 | 0.804 |
| path | clean | 6.331 | 6.843 | 1.081 | 1.044 to 1.108 | 0.735 |
| path | join | 17.74 | 19.98 | 1.127 | 1.093 to 1.155 | 0.562 |
| path | parts | 6.574 | 9.219 | 1.402 | 1.308 to 1.466 | 0.738 |
| path | match | 5.992 | 5.882 | 0.982 | 0.953 to 1.03 | 1.0 |
| cookie | cookie | 27.19 | 19.88 | 0.731 | 0.719 to 0.764 | 0.634 |
| cookie | setcookie | 27.49 | 41.76 | 1.519 | 1.486 to 1.562 | 0.627 |
| html | none | 0.642 | 0.387 | 0.602 | 0.577 to 0.639 | 1.0 |
| html | escape | 130.5 | 228.5 | 1.751 | 1.724 to 1.775 | 0.602 |
| html | unescape | 304.4 | 196.7 | 0.646 | 0.641 to 0.65 | 0.592 |
| time | none | 0.731 | 0.453 | 0.619 | 0.578 to 0.653 | 1.0 |
| time | format | 36.38 | 197.2 | 5.419 | 5.376 to 5.436 | 0.584 |
| time | parse | 18.66 | 88.29 | 4.731 | 4.696 to 4.777 | 0.598 |
| time | civil | 22.5 | 10.29 | 0.457 | 0.446 to 0.469 | 1.0 |
| time | duration | 15.13 | 10.95 | 0.723 | 0.718 to 0.729 | 1.0 |
| template | none | 0.708 | 0.445 | 0.629 | 0.592 to 0.652 | 1.0 |
| template | parse | 115.2 | 104.0 | 0.903 | 0.886 to 0.915 | 0.641 |
| template | render | 243.6 | 330.1 | 1.355 | 1.348 to 1.394 | 0.633 |
| big | none | 0.787 | 0.466 | 0.592 | 0.57 to 0.627 | 1.0 |
| big | mul | 8.96 | 11.06 | 1.234 | 1.221 to 1.249 | 0.577 |
| big | quorem | 9.84 | 9.755 | 0.991 | 0.976 to 1.011 | 0.578 |
| big | text | 5.323 | 5.9 | 1.108 | 1.101 to 1.124 | 0.584 |
| big | parse | 7.335 | 3.601 | 0.491 | 0.479 to 0.494 | 0.743 |
| big | exp | 58.91 | 53.05 | 0.901 | 0.896 to 0.908 | 1.0 |
| hash | none | 0.844 | 0.513 | 0.608 | 0.587 to 0.633 | 1.0 |
| hash | crc32 | 13.17 | 12.62 | 0.958 | 0.943 to 0.984 | 1.0 |
| hash | crc32c | 13.08 | 4.843 | 0.37 | 0.364 to 0.384 | 1.0 |
| hash | fnv32a | 21.68 | 21.28 | 0.981 | 0.977 to 0.99 | 1.0 |
| hash | fnv64a | 23.04 | 21.95 | 0.953 | 0.951 to 0.963 | 1.0 |
| hash | fnv128a | 151.6 | 124.3 | 0.819 | 0.811 to 0.857 | 0.569 |
| deque | queue | 601.9 | 75.06 | 0.125 | 0.117 to 0.125 | 0.706 |
| deque | both | 319.0 | 53.49 | 0.168 | 0.163 to 0.172 | 0.535 |
| deque | none | 0.587 | 0.357 | 0.608 | 0.578 to 0.639 | 0.998 |
| dns | none | 0.812 | 0.565 | 0.696 | 0.668 to 0.755 | 0.904 |
| dns | mx | 21.9 | 102.0 | 4.657 | 4.575 to 4.707 | 0.557 |
| x509 | none | 1.016 | 0.454 | 0.446 | 0.431 to 0.474 | 1.0 |
| x509 | parse | 456.6 | 370.8 | 0.812 | 0.808 to 0.815 | 0.651 |
| x509 | verify | 679.3 | 662.1 | 0.975 | 0.973 to 0.976 | 0.649 |
| x509 | create | 1105 | 877.1 | 0.794 | 0.793 to 0.796 | 0.672 |
| x509 | crl | 487.6 | 203.5 | 0.417 | 0.415 to 0.428 | 0.658 |
| ed25519 | none | 0.834 | 0.809 | 0.97 | 0.935 to 1.008 | 1.0 |
| ed25519 | keygen | 55.02 | 40.44 | 0.735 | 0.731 to 0.739 | 1.0 |
| ed25519 | sign | 69.84 | 83.19 | 1.191 | 1.185 to 1.192 | 1.0 |
| ed25519 | signkey | 69.43 | 45.0 | 0.648 | 0.643 to 0.651 | 1.0 |
| ed25519 | verify | 148.1 | 112.7 | 0.761 | 0.757 to 0.768 | 1.0 |
| sha3 | none | 3.894 | 2.069 | 0.531 | 0.522 to 0.54 | 1.0 |
| sha3 | big | 75.57 | 77.39 | 1.024 | 1.02 to 1.031 | 1.0 |
| sha3 | small | 59.68 | 71.02 | 1.19 | 1.178 to 1.2 | 1.0 |
| jsonindent | none | 0.648 | 0.381 | 0.587 | 0.548 to 0.619 | 1.0 |
| jsonindent | indent | 185.9 | 116.7 | 0.628 | 0.585 to 0.655 | 0.583 |
| jsonindent | rawfield | 201.9 | 150.8 | 0.747 | 0.732 to 0.758 | 0.575 |
| walkdir | none | 37.95 | 36.58 | 0.964 | 0.933 to 1.001 | 1.0 |
| walkdir | walk | 298.6 | 574.0 | 1.923 | 1.894 to 1.939 | 0.56 |
| time | format_layout | 36.94 | 74.8 | 2.025 | 2.002 to 2.059 | 0.584 |
| time | parse_layout | 18.77 | 35.66 | 1.9 | 1.855 to 1.942 | 0.598 |

## Reading the rows

Level rule used for the std-coverage page: Proven when every workload is at most 1.05 of Go's wall, Solid when
every workload is within 2.0, Early above that. Worst workload per bench: see below.

## Over 1.5x, and what was done

- html escape 2.31 to 1.75: the count pass is a table lookup and the result is returned as the Str it is
  (no re-validation of the built bytes). std/changes/html.md.
- time format 7.0 to 5.4 and parse 6.1 to 4.7 with string layouts, which are parsed on every call (51% of the
  profile is layout_items); the same layouts as consts (`time.Layout`, decision 0332) run at format 2.03 and
  parse 1.90. A lone literal is kept as a view and the item list is sized (std/changes/time.md). The rest is
  the layout walk itself; the way to close it is for the compiler to hoist a literal layout argument into a
  const Layout, which is a compiler change not made here.
- trimchars trim 2.24, trim_start 1.91, trim_end 1.98: the profile (100,000,000 trims) is the loop and `drop Str`
  24%: each trimmed result is a view that holds a reference to its text, where Go's substring is free
  (decision 0330's view model). Not changed.
- strconv itoa 1.52 and atoi 1.46, base32 decode 1.76, cookie setcookie 1.52, walkdir walk 1.92 and dns mx 4.66
  (a UDP round trip each, 22 ms): not changed; base32, itoa and the cookie parser are the next candidates.
